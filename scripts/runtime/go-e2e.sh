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
			echo "Unknown go-e2e option: $1" >&2
			exit 1
			;;
	esac
done

go_test_args=(-tags e2e -v -count=1 -timeout 300s)
if [[ -n "$go_run_selector" ]]; then
	echo "go-e2e: applying -run selector: $go_run_selector"
	go_test_args+=(-run "$go_run_selector")
fi
go_test_args+=(./tests/e2e/...)

go test "${go_test_args[@]}"
