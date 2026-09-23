#!/bin/bash

# Generates a fresh CycloneDX SBOM for mongo-tools and writes it to the path given via --output.
#
# This intentionally never writes to a fixed, well-known path in the repo: there is no
# developer-maintained, checked-in SBOM anymore (see TOOLS-3768). Callers are:
#   - scripts/upload-sbom.sh: generates to a temp file, then `silkbomb upload`s it.
#   - scripts/generate-release-sbom-snapshot.sh: generates straight to ssdlc/<tag>.bom.json for the
#     release's historical record.

set -e
set -o pipefail
set -x

readonly CYCLONEDX_GOMOD_VERSION="v1.12.0"

OUTPUT=""
while [[ $# -gt 0 ]]; do
    case "$1" in
    --output)
        OUTPUT="$2"
        shift 2
        ;;
    *)
        echo "generate-sbom.sh: unrecognized argument: $1" >&2
        exit 1
        ;;
    esac
done
if [ -z "$OUTPUT" ]; then
    echo "usage: generate-sbom.sh --output <path>" >&2
    exit 1
fi

BINARY_NAMES="$(mise exec go -- go run release/release.go print-binary-names)"
OS_ARCH_COMBOS="$(mise exec go -- go run release/release.go print-os-arch-combos)"
MODULE_PATH="$(mise exec go -- go list -m)"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# Install cyclonedx-gomod once, as a native binary, so we can invoke it directly below with
# GOOS/GOARCH set as env vars per target combo. Running it via `go run pkg@version` with those
# vars set doesn't work: `go run` would cross-compile cyclonedx-gomod itself for the target
# instead of just using GOOS/GOARCH as build-constraint context for its own analysis, producing a
# non-native binary that can't even execute on this host.
GOBIN="$WORKDIR/bin" mise exec go -- go install "github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@${CYCLONEDX_GOMOD_VERSION}"
CYCLONEDX_GOMOD="$WORKDIR/bin/cyclonedx-gomod"

mkdir "$WORKDIR/combos"

# For every (binary, OS/arch) combination, run cyclonedx-gomod's "app" mode, which evaluates each
# binary's actual build constraints -- so platform-gated dependencies (e.g. Windows-only or
# Linux-only packages) are captured precisely -- without needing to compile anything. Unlike
# `cyclonedx-gomod mod` (a whole-module scan), "app" mode doesn't miss dependencies that are only
# reachable deep inside a vendored package's own imports: `mod` mode misses
# github.com/google/uuid, a real dependency of the Azure auth code path, because it doesn't
# evaluate build constraints the way a real build does.
#
# Each combo's own output also carries a real, multi-level dependency graph (each component's
# `dependsOn` lists what *that* component actually imports, not just "the app depends on
# everything") -- keep every combo file around so `merge-sbom-components` (release/release.go) can
# merge that graph properly, rather than flattening it away.
#
# shellcheck disable=SC2086 # we intentionally don't quote `$OS_ARCH_COMBOS`/`$BINARY_NAMES` so we
# split on the whitespace.
for c in $OS_ARCH_COMBOS; do
    os="$(echo $c | cut -f1 -d/)"
    arch="$(echo $c | cut -f2 -d/)"
    for name in $BINARY_NAMES; do
        GOOS="$os" GOARCH="$arch" "$CYCLONEDX_GOMOD" app \
            -std -short-purls -json -output-version 1.6 \
            -main "${name}/main" -output "$WORKDIR/combos/${name}-${os}-${arch}.json" .
    done
done

# Merges every combo's components (deduped by bom-ref) and dependency graph (edges unioned across
# combos) into one BOM, using cyclonedx-go directly rather than jq text manipulation -- see
# mergeSBOMComponents in release/release.go for the merge rules (including how the main module's
# own self-referencing component is promoted to metadata.component).
mise exec go -- go run release/release.go merge-sbom-components \
    --dir "$WORKDIR/combos" \
    --module-prefix "pkg:golang/${MODULE_PATH}@" \
    --output "$WORKDIR/merged-sbom.json"

./scripts/authenticate-devprod-platforms-ecr.sh

OUTPUT_DIR="$(dirname "$OUTPUT")"
OUTPUT_BASE="$(basename "$OUTPUT")"
mkdir -p "$OUTPUT_DIR"

# The arguments to the silkbomb program start at "update". We mount $WORKDIR and $OUTPUT_DIR
# separately from each other, writing directly to the caller's requested output path.
podman run \
    --rm \
    --platform linux/amd64 \
    -v "${WORKDIR}":/workdir \
    -v "$(cd "$OUTPUT_DIR" && pwd)":/output \
    901841024863.dkr.ecr.us-east-1.amazonaws.com/release-infrastructure/silkbomb:2.0 \
    update \
    --sbom-in /workdir/merged-sbom.json \
    --sbom-out "/output/${OUTPUT_BASE}" \
    --select-licenses \
    --schema-version 1.6
