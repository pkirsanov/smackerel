#!/usr/bin/env bash
set -euo pipefail

cd /workspace

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
go_test_args=(-tags stress -v -count=1 -timeout 720s)
if [[ -n "$go_run_selector" ]]; then
	echo "go-stress: applying -run selector: $go_run_selector"
	go_test_args+=(-run "$go_run_selector")
fi
go_test_args+=(./tests/stress/...)

go test "${go_test_args[@]}"
