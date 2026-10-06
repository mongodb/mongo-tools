package sbom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
)

func TestMergeComponents(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "merged.json")

	a := libraryComponent("pkg:golang/example.com/a@v1", "a")
	b := libraryComponent("pkg:golang/example.com/b@v1", "b")
	c := libraryComponent("pkg:golang/example.com/c@v1", "c")

	// combo 1: root -> {a, b}; a -> {c}
	writeCombo(t, dir, "one.json",
		[]cyclonedx.Dependency{
			{Ref: mainModule().BOMRef, Dependencies: &[]string{a.BOMRef, b.BOMRef}},
			{Ref: a.BOMRef, Dependencies: &[]string{c.BOMRef}},
		},
		[]cyclonedx.Component{a, b},
	)

	// combo 2: root -> {b, c}; b -> {}; c -> {a}; b is re-described with a different name
	bRenamed := b
	bRenamed.Name = "b-renamed"
	writeCombo(t, dir, "two.json",
		[]cyclonedx.Dependency{
			{Ref: mainModule().BOMRef, Dependencies: &[]string{b.BOMRef, c.BOMRef}},
			{Ref: b.BOMRef},
			{Ref: c.BOMRef, Dependencies: &[]string{a.BOMRef}},
		},
		[]cyclonedx.Component{bRenamed, c},
	)

	// Non-JSON files and nested directories must be ignored, even if their contents are not BOMs.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not json"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, "nested", "bad.json"), []byte("{not json"), 0o600),
	)

	require.NoError(t, MergeComponents(dir, modulePrefix, output))
	merged := readMerged(t, output)

	require.Equal(t, cyclonedx.SpecVersion1_6, merged.SpecVersion, "writes a CycloneDX 1.6 BOM")
	require.Equal(
		t,
		mainModule().BOMRef,
		merged.Metadata.Component.BOMRef,
		"promotes the main module to metadata.component",
	)

	require.NotNil(t, merged.Components)
	gotComponents := map[string]string{}
	for _, comp := range *merged.Components {
		gotComponents[comp.BOMRef] = comp.Name
	}
	require.Equal(t, map[string]string{
		a.BOMRef: "a",
		b.BOMRef: "b", // first occurrence wins
		c.BOMRef: "c",
	}, gotComponents, "dedupes components by bom-ref and keeps the first occurrence")
	require.NotContains(
		t,
		gotComponents,
		mainModule().BOMRef,
		"excludes the main module from components",
	)

	require.Equal(t, map[string][]string{
		mainModule().BOMRef: {a.BOMRef, b.BOMRef, c.BOMRef},
		a.BOMRef:            {c.BOMRef},
		b.BOMRef:            nil,
		c.BOMRef:            {a.BOMRef},
	}, dependsOnMap(merged), "unions dependency edges across combos")
}

func TestMergeComponentsExcludesMainModuleFromComponents(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "merged.json")

	a := libraryComponent("pkg:golang/example.com/a@v1", "a")

	// Some SBOM sources list the main module in components[] as well as metadata.component.
	writeCombo(t, dir, "one.json",
		[]cyclonedx.Dependency{
			{Ref: mainModule().BOMRef, Dependencies: &[]string{a.BOMRef}},
		},
		[]cyclonedx.Component{a, *mainModule()},
	)

	require.NoError(t, MergeComponents(dir, modulePrefix, output))
	merged := readMerged(t, output)

	gotComponents := map[string]struct{}{}
	for _, comp := range *merged.Components {
		gotComponents[comp.BOMRef] = struct{}{}
	}
	require.NotContains(
		t,
		gotComponents,
		mainModule().BOMRef,
		"excludes the main module even when it also appears in components[]",
	)
	require.Contains(t, gotComponents, a.BOMRef, "keeps the other components")
}

func TestMergeComponentsNoRoot(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "merged.json")

	bom := cyclonedx.NewBOM()
	bom.SpecVersion = cyclonedx.SpecVersion1_6
	bom.Metadata = &cyclonedx.Metadata{Component: &cyclonedx.Component{
		Type:       cyclonedx.ComponentTypeApplication,
		BOMRef:     "pkg:golang/other@v1",
		Name:       "other",
		PackageURL: "pkg:golang/other@v1",
	}}
	comps := []cyclonedx.Component{libraryComponent("pkg:golang/example.com/a@v1", "a")}
	bom.Components = &comps
	writeBOM(t, dir, "one.json", bom)

	err := MergeComponents(dir, modulePrefix, output)
	require.ErrorContains(
		t,
		err,
		"no component with purl prefix",
		"errors when the main module can't be identified",
	)
}

func TestMergeComponentsNoComponents(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "merged.json")

	writeCombo(t, dir, "one.json", nil, []cyclonedx.Component{})

	err := MergeComponents(dir, modulePrefix, output)
	require.ErrorContains(
		t,
		err,
		"no components found",
		"errors when the input BOMs have no dependency components",
	)
}

func TestMergeComponentsMissingDir(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	err := MergeComponents(
		filepath.Join(t.TempDir(), "does-not-exist"),
		modulePrefix,
		filepath.Join(t.TempDir(), "merged.json"),
	)
	require.ErrorContains(
		t,
		err,
		"failed to read directory",
		"errors when the input directory can't be read",
	)
}

func TestMergeComponentsMalformedJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dir := t.TempDir()
	output := filepath.Join(t.TempDir(), "merged.json")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o600))

	err := MergeComponents(dir, modulePrefix, output)
	require.ErrorContains(
		t,
		err,
		"failed to decode",
		"errors when an input file is not valid CycloneDX JSON",
	)
}

const modulePrefix = "pkg:golang/example.com/mod@"

func writeCombo(
	t *testing.T,
	dir, name string,
	deps []cyclonedx.Dependency,
	comps []cyclonedx.Component,
) {
	t.Helper()

	bom := cyclonedx.NewBOM()
	bom.SpecVersion = cyclonedx.SpecVersion1_6
	bom.Metadata = &cyclonedx.Metadata{Component: mainModule()}
	bom.Components = &comps
	bom.Dependencies = &deps
	writeBOM(t, dir, name, bom)
}

func writeBOM(t *testing.T, dir, name string, bom *cyclonedx.BOM) {
	t.Helper()

	f, err := os.Create(filepath.Join(dir, name))
	require.NoError(t, err)
	defer f.Close()

	require.NoError(t, cyclonedx.NewBOMEncoder(f, cyclonedx.BOMFileFormatJSON).Encode(bom))
}

func readMerged(t *testing.T, path string) *cyclonedx.BOM {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var bom cyclonedx.BOM
	require.NoError(t, cyclonedx.NewBOMDecoder(f, cyclonedx.BOMFileFormatJSON).Decode(&bom))
	return &bom
}

func dependsOnMap(bom *cyclonedx.BOM) map[string][]string {
	out := map[string][]string{}
	if bom.Dependencies == nil {
		return out
	}
	for _, d := range *bom.Dependencies {
		if d.Dependencies != nil {
			out[d.Ref] = *d.Dependencies
		} else {
			out[d.Ref] = nil
		}
	}
	return out
}

func mainModule() *cyclonedx.Component {
	return &cyclonedx.Component{
		Type:       cyclonedx.ComponentTypeApplication,
		BOMRef:     modulePrefix + "v1.0.0",
		Name:       "example.com/mod",
		Version:    "v1.0.0",
		PackageURL: modulePrefix + "v1.0.0",
	}
}

func libraryComponent(ref, name string) cyclonedx.Component {
	return cyclonedx.Component{
		Type:       cyclonedx.ComponentTypeLibrary,
		BOMRef:     ref,
		Name:       name,
		PackageURL: ref,
	}
}
