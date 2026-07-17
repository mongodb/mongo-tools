#!/usr/bin/env bash

# `evergreen validate` exits 0 even when it reports warnings, and it always writes to stdout, not
# stderr. We want to fail on any warning except the specific pre-existing ones listed below, but
# precious's ignore-stderr can't do that: it only looks at stderr, and it's all-or-nothing (matching
# one pattern suppresses the whole output, not just that line). So we do the line-by-line filtering
# here instead.
#
# Each accepted warning is matched exactly, by task name, rather than by a wildcard "defined but
# not used" pattern. A new unused-task warning is a real problem to fix or explicitly allow here,
# not something to pass silently just because it looks like the others.

set -o pipefail

TEMP_FILE=$(mktemp "${TMPDIR:-/tmp}/evergreen-validate.tmp.XXXXXXXXXX")
trap 'rm -f "$TEMP_FILE"' EXIT

# The tee lets us capture the output for filtering below while still printing it to the console, so
# it shows up when precious is run in verbose mode.
evergreen validate --file common.yml -p mongo-tools 2>&1 | tee "$TEMP_FILE"
STATUS=${PIPESTATUS[0]}

if [ "$STATUS" -ne 0 ]; then
    exit "$STATUS"
fi

if grep --quiet "is valid with warnings" "$TEMP_FILE"; then
    while IFS= read -r line; do
        case "$line" in
        "WARNING: task 'commit-queue-workaround' defined but not used by any variants; consider using or disabling")
            continue
            ;;
        "WARNING: task 't_resmoke_setup' defined but not used by any variants; consider using or disabling")
            continue
            ;;
        "WARNING: task 'generate_mongodump_fuzz_tasks' defined but not used by any variants; consider using or disabling")
            continue
            ;;
        *"is valid with warnings")
            continue
            ;;
        *)
            exit 1
            ;;
        esac
    done <"$TEMP_FILE"
fi
