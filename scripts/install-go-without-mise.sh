#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
# shellcheck disable=SC1091
source "$SCRIPT_DIR/functions.sh"

# mise has no builds at all for ppc64le or s390x, so on those architectures we manage Go ourselves.
# This repo's Evergreen hosts have historically had a Go toolchain at /opt/golang/go<version> (see
# the old set_goenv.sh), not necessarily on PATH. We check there, and whatever's already on PATH,
# for a Go matching what we pin in mise.toml; failing that, we download it directly from Google's Go
# distribution (the same source mise's own core:go backend uses).
#
# Either way, we end up with a symlink or extracted toolchain at $go_dir/go/bin/go, so
# scripts/ci-env.sh only ever needs to check that one fixed location.

# We're managing the toolchain ourselves here, so don't let Go silently fetch a different version on
# its own (e.g. because go.mod requires a newer version than whatever go we find) - that would
# defeat the whole point of pinning a version and checking for it explicitly.
export GOTOOLCHAIN=local

arch=$(uname -m)
want_go_version=$(grep -E '^go = ' "$REPO_ROOT/mise.toml" | sed -E 's/^go = "(.*)"$/\1/')

go_dir="${EVG_WORKDIR:?}/.local/share/go-toolchain"
rm -rf "$go_dir"
mkdir -p "$go_dir"

for candidate in "$(command -v go 2>/dev/null)" /opt/golang/go*/bin/go; do
    # `grep -F` treats the version as a literal string so the dots in e.g. "1.25.11" don't match any
    # character, and the trailing space requires an exact version rather than a loose prefix match
    # (e.g. "1.25.1" incorrectly matching within "1.25.11").
    if [ -x "$candidate" ] && "$candidate" version | grep -qF "go${want_go_version} "; then
        echo "using existing go $want_go_version at $candidate"
        ln -s "$(dirname "$(dirname "$candidate")")" "$go_dir/go"
        "$go_dir/go/bin/go" version
        exit 0
    fi
done

echo "no system go matching $want_go_version found for $arch; downloading it"
tmp_tarball=$(mktemp --suffix=.tar.gz)
trap 'rm -f "$tmp_tarball"' EXIT
retry curl --fail --location --silent --show-error \
    --output "$tmp_tarball" \
    "https://dl.google.com/go/go${want_go_version}.linux-${arch}.tar.gz"

tar -C "$go_dir" -xzf "$tmp_tarball"

"$go_dir/go/bin/go" version
