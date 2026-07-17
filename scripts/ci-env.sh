#!/bin/bash
# Sets standard CI environment variables for build and test tasks.
# Requires $EVG_WORKDIR to be set in the environment before sourcing.

# cgo needs a mingw-w64 toolchain and some Windows-specific flags on Windows.
# We also normalize the working directory through cygpath here, since
# Evergreen's workdir expansion on Windows sometimes lacks a drive letter,
# and Go's os/exec refuses to run an executable found via a PATH entry it
# can't prove is absolute.
case "$(PATH=/usr/bin:/bin uname -s)" in
CYGWIN*)
    EVG_WORKDIR="$(cygpath -am "$EVG_WORKDIR")"
    export CGO_CFLAGS="-D_WIN32_WINNT=0x0601 -DNTDDI_VERSION=0x06010000"
    export GOCACHE="C:/windows/temp"
    export PATH="$PATH:/cygdrive/c/mingw-w64/x86_64-4.9.1-posix-seh-rt_v3-rev1/mingw64/bin"
    ;;
esac

# mise has no builds at all for ppc64le or s390x. On those architectures we
# download Go ourselves instead (see install-go-without-mise.sh), so shared
# scripts that need to run go use GO_EXEC_PREFIX rather than hardcoding
# "mise exec go --".
case "$(uname -m)" in
ppc64le | s390x)
    export PATH="${EVG_WORKDIR:?}/.local/share/go-toolchain/go/bin:$PATH"
    export GO_EXEC_PREFIX=""
    # We're managing the toolchain ourselves on these architectures, so don't
    # let Go silently fetch a different version on its own (e.g. because
    # go.mod requires a newer version than what we've provisioned).
    export GOTOOLCHAIN=local
    ;;
*)
    export GO_EXEC_PREFIX="mise exec go --"
    ;;
esac

# We add $HOME to PATH because the evergreen client binary lives at ~/evergreen.
export PATH="${EVG_WORKDIR:?}/.local/bin:$HOME:$PATH"
export MISE_DATA_DIR="${EVG_WORKDIR:?}/.local/share/mise"

# build.go's getPlatform() reads EVG_VARIANT (whenever CI is set) to choose the per-platform build
# tags -- notably "failpoints", which several integration tests depend on
# (e.g. TestMongoDumpTOOLS2498's PauseBeforeDumping failpoint). If EVG_VARIANT is missing the tags
# are dropped and those tests break. Most variants use the build variant name directly; a few
# (e.g. "static") override it via $EVG_PLATFORM, matching the old _set_shell_env logic.
if [ -n "${EVG_PLATFORM:-}" ] || [ -n "${EVG_BUILD_VARIANT:-}" ]; then
    export EVG_VARIANT="${EVG_PLATFORM:-$EVG_BUILD_VARIANT}"
fi

if [ -f /opt/rh/devtoolset-7/enable ]; then
    # shellcheck disable=SC1091
    source /opt/rh/devtoolset-7/enable
fi
