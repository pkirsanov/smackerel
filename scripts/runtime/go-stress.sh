#!/usr/bin/env bash
set -euo pipefail

# spec-052 chaos finding: stress tests that may shell out to
# scripts/commands/config.sh require envsubst (gettext-base) which
# is not present in the golang:bookworm base image. Use the shared
# helper so all four go-*.sh wrappers share one envsubst-install
# implementation.
# shellcheck source=scripts/runtime/_ensure_envsubst.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-stress"

workspace_dir="${SMACKEREL_STRESS_WORKSPACE:-/workspace}"
cd "$workspace_dir"

go_run_selector=""

while [[ $# -gt 0 ]]; do
	case "$1" in
		--run)
			if [[ $# -lt 2 ]]; then
				echo "ERROR: --run requires a non-empty regex" >&2
				exit 1
			fi
			if [[ -z "$2" ]]; then
				echo "ERROR: --run requires a non-empty regex" >&2
				exit 1
			fi
			go_run_selector="$2"
			shift 2
			;;
		--run=*)
			go_run_selector="${1#*=}"
			if [[ -z "$go_run_selector" ]]; then
				echo "ERROR: --run requires a non-empty regex" >&2
				exit 1
			fi
			shift
			;;
		*)
			echo "Unknown go-stress option: $1" >&2
			exit 1
			;;
	esac
done

# Stress profile is bounded by the spec NFR (5min duration + warmup).
# Allow generous timeout for the full profile plus one extra cycle.
echo "go-stress: running readiness canary"
go test -tags stress -v -count=1 -timeout 90s -run '^TestStressReadinessCanary_Live$' ./tests/stress/readiness
echo "go-stress: readiness canary passed"

go_test_args=(-tags stress -v -count=1 -timeout 720s)
if [[ -n "$go_run_selector" ]]; then
	echo "go-stress: applying -run selector: $go_run_selector"
	go_test_args+=(-run "$go_run_selector")
fi
go_test_args+=(./tests/stress/...)

go test "${go_test_args[@]}"
