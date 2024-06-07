#!/usr/bin/env bash

set -o errexit

golines_flags="--max-len 100 --no-reformat-tags"

if [ "$1" == "--lint" ]; then
    # shellcheck disable=SC2086
    OUTPUT=$(golines \
        $golines_flags \
        --dry-run \
        "${@:2}")
    if [ -n "$OUTPUT" ]; then
        echo "$OUTPUT"
        exit 1
    fi
else
    # shellcheck disable=SC2086
    golines \
        $golines_flags \
        --write-output \
        "${@:1}"
fi
