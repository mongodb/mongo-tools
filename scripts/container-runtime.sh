#!/bin/bash

# Prints the name of the container runtime to use for pulling/running images (podman or docker),
# preferring podman when both are present since that's what our existing garasign signing flow
# (scripts/sign_artifacts.sh) already assumes. Callers capture this via command substitution --
# e.g. `RUNTIME="$(./scripts/container-runtime.sh)"` -- since silkbomb runs identically under
# either runtime.

if command -v podman >/dev/null 2>&1; then
    echo podman
elif command -v docker >/dev/null 2>&1; then
    echo docker
else
    echo "container-runtime.sh: neither podman nor docker found on PATH" >&2
    exit 1
fi
