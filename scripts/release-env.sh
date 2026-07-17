#!/bin/bash
# Applies release-specific env var overrides. Requires EVG_TRIGGERED_BY_TAG,
# FAKE_TAG_FOR_RELEASE_TESTING, EVG_VARIANT, and PLATFORM_OVERRIDE to be set
# in the environment (via env:) before sourcing.

if [ -n "$FAKE_TAG_FOR_RELEASE_TESTING" ]; then
    export EVG_TRIGGERED_BY_TAG="$FAKE_TAG_FOR_RELEASE_TESTING"
    export IS_FAKE_TAG=1
fi

if [ -n "$PLATFORM_OVERRIDE" ]; then
    export EVG_VARIANT="$PLATFORM_OVERRIDE"
fi
