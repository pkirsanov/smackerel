#!/usr/bin/env bash
set -euo pipefail

# Spec 099 — dockerized fallback for the resource pre-flight guard.
#
# smackerel.sh prefers a host-native `go run ./cmd/preflight` when a host Go
# toolchain is present (fast, no container). When it is not, the guard runs
# this wrapper via run_go_tooling inside the golang:1.25.10-bookworm container
# (`-v $SCRIPT_DIR:/workspace`, NO --memory cgroup limit). Inside that
# container /proc/meminfo reports HOST MemAvailable (no mem limit) and
# /workspace is a bind mount of the repo, so statfs(/workspace) follows to the
# HOST filesystem backing the repo — both readings are host-correct.
#
# Arg 1 = target env name (dev|test). Required.

TARGET_ENV="${1:?preflight.sh requires a target env name (dev|test)}"
# Arg 2 = threshold profile (heavy|light|ui). Required — selects which SST
# threshold pair the guard enforces (heavy = build/up/full test lanes;
# light = the stores-only integration-light lane; ui = the no-ML PWA browser
# e2e-ui lane, spec 100 F-100-OPT-02). NO-DEFAULTS.
PROFILE="${2:?preflight.sh requires a threshold profile (heavy|light|ui)}"

cd /workspace
exec go run ./cmd/preflight --env "$TARGET_ENV" --repo-root /workspace --profile "$PROFILE"
