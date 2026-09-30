package sbom

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CycloneDX/cyclonedx-go"
)

// MergeComponents merges every per-combo CycloneDX JSON file in the given dir into a single BOM
// and writes it to output. Each input file is expected to be cyclonedx-gomod's "app" mode output
// for one (binary, OS/arch) combination: a full BOM describing that combination's own main module
// (whose purl starts with modulePrefix) and its dependencies, complete with a real dependency graph
// (each component's own "dependsOn" edges, not just "the app depends on everything").
//
// Components are deduped by bom-ref, keeping the first occurrence - a purl fully identifies a
// module version, so it doesn't matter which combo discovered it first. Dependency edges are
// unioned across combos, since the same component can gain additional edges under different build
// constraints. cyclonedx-gomod's "app" mode describes the main module itself only in
// metadata.component, not as a components[] entry; that's promoted to the merged BOM's
// metadata.component here too. Its own dependencies[] entry (listing its direct dependencies) is
// kept rather than dropped: Silkbomb expects the root component to have one.
func MergeComponents(dir, modulePrefix, output string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %q: %w", dir, err)
	}

	components := map[string]cyclonedx.Component{}
	dependsOn := map[string]map[string]struct{}{}
	var rootComponent *cyclonedx.Component

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open %q: %w", filePath, err)
		}

		var bom cyclonedx.BOM
		err = cyclonedx.NewBOMDecoder(f, cyclonedx.BOMFileFormatJSON).Decode(&bom)
		f.Close()
		if err != nil {
			return fmt.Errorf("failed to decode %q: %w", filePath, err)
		}

		// cyclonedx-gomod's "app" mode describes the module being analyzed (the main module itself)
		// in metadata.component, not as an entry in components[] - so the root is found here, not
		// by filtering components by purl prefix.
		if rootComponent == nil && bom.Metadata != nil && bom.Metadata.Component != nil &&
			strings.HasPrefix(bom.Metadata.Component.PackageURL, modulePrefix) {
			root := *bom.Metadata.Component
			rootComponent = &root
		}

		if bom.Components != nil {
			for _, c := range *bom.Components {
				// Defensive: some SBOM sources do list the main module as one of its own
				// components. Guard against that here too, even though cyclonedx-gomod's "app" mode
				// doesn't.
				if strings.HasPrefix(c.PackageURL, modulePrefix) {
					continue
				}
				if _, ok := components[c.BOMRef]; !ok {
					components[c.BOMRef] = c
				}
			}
		}

		// Unlike components[], the root's own entry in dependencies[] is kept: Silkbomb expects the
		// BOM's root component to have a dependencies[] entry listing its direct dependencies (it
		// warns "the Dependency Graph is incomplete" otherwise), and cyclonedx-gomod's own output
		// already provides exactly that for each combo.
		if bom.Dependencies != nil {
			for _, d := range *bom.Dependencies {
				edges, ok := dependsOn[d.Ref]
				if !ok {
					edges = map[string]struct{}{}
					dependsOn[d.Ref] = edges
				}
				if d.Dependencies != nil {
					for _, dep := range *d.Dependencies {
						edges[dep] = struct{}{}
					}
				}
			}
		}
	}

	if rootComponent == nil {
		return fmt.Errorf(
			"no component with purl prefix %q found in %q; can't identify the main module's own component",
			modulePrefix,
			dir,
		)
	}
	if len(components) == 0 {
		return fmt.Errorf("no components found in %q", dir)
	}

	finalComponents := make([]cyclonedx.Component, 0, len(components))
	for _, c := range components {
		finalComponents = append(finalComponents, c)
	}
	sort.Slice(finalComponents, func(i, j int) bool {
		return finalComponents[i].BOMRef < finalComponents[j].BOMRef
	})

	finalDependencies := make([]cyclonedx.Dependency, 0, len(dependsOn))
	for ref, edges := range dependsOn {
		dep := cyclonedx.Dependency{Ref: ref}
		if len(edges) > 0 {
			deps := make([]string, 0, len(edges))
			for d := range edges {
				deps = append(deps, d)
			}
			sort.Strings(deps)
			dep.Dependencies = &deps
		}
		finalDependencies = append(finalDependencies, dep)
	}
	sort.Slice(finalDependencies, func(i, j int) bool {
		return finalDependencies[i].Ref < finalDependencies[j].Ref
	})

	bom := cyclonedx.NewBOM()
	bom.SpecVersion = cyclonedx.SpecVersion1_6
	bom.Metadata = &cyclonedx.Metadata{Component: rootComponent}
	bom.Components = &finalComponents
	bom.Dependencies = &finalDependencies

	outFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create %q: %w", output, err)
	}
	defer outFile.Close()

	encoder := cyclonedx.NewBOMEncoder(outFile, cyclonedx.BOMFileFormatJSON)
	encoder.SetPretty(true)
	if err := encoder.Encode(bom); err != nil {
		return fmt.Errorf("failed to write merged SBOM to %q: %w", output, err)
	}

	return nil
}
