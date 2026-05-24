#!/usr/bin/env bash
set -euo pipefail

# spec-052 chaos finding: integration tests that shell out to
# scripts/commands/config.sh (e.g.,
# tests/integration/config_validate_test.go,
# tests/integration/ollama_config_contract_test.go,
# tests/integration/drive/drive_config_contract_test.go) require
# envsubst (gettext-base) which is not present in the
# golang:bookworm base image. Use the shared helper so all four
# go-*.sh wrappers share one envsubst-install implementation.
# shellcheck source=scripts/runtime/_ensure_envsubst.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-integration"

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
			echo "Unknown go-integration option: $1" >&2
			exit 1
			;;
	esac
done

go_test_args=(-p 1 -tags integration -v -count=1 -timeout 300s)
if [[ -n "$go_run_selector" ]]; then
	echo "go-integration: applying -run selector: $go_run_selector"
	go_test_args+=(-run "$go_run_selector")
fi
go_test_args+=(./tests/integration/... ./internal/notification/...)

go test "${go_test_args[@]}"
