#!/usr/bin/env bash
set -euo pipefail

# spec-052 chaos finding: e2e tests that shell out to
# scripts/commands/config.sh (e.g.,
# tests/e2e/drive/drive_foundation_e2e_test.go) require envsubst
# (gettext-base) which is not present in the golang:bookworm base
# image. Use the shared helper so all four go-*.sh wrappers share
# one envsubst-install implementation. Also unblocks
# tests/e2e/qf_decisions_connector_api_test.go's runtime-note
# referenced by spec-041 Scope 2 DoD.
# shellcheck source=scripts/runtime/_ensure_envsubst.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-e2e"

# BUG-075-002 — assistant renderer E2E invokes the checked-in PWA
# renderer with Node. Supply that tool inside this repository-managed
# Docker lane; host Node is neither required nor consulted.
# shellcheck source=scripts/runtime/_ensure_node.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_node.sh"
ensure_node "go-e2e"

cd /workspace

go_run_selector=""
go_package_selector=""

while [[ $# -gt 0 ]]; do
	case "$1" in
		--package)
			if [[ $# -lt 2 || -z "$2" ]]; then
				echo "ERROR: --package requires a non-empty package name" >&2
				exit 1
			fi
			if [[ "$2" != "assistant" ]]; then
				echo "ERROR: unsupported --package value: $2 (allowed: assistant)" >&2
				exit 1
			fi
			go_package_selector="$2"
			shift 2
			;;
		--package=*)
			go_package_selector="${1#*=}"
			if [[ "$go_package_selector" != "assistant" ]]; then
				echo "ERROR: unsupported --package value: ${go_package_selector:-<empty>} (allowed: assistant)" >&2
				exit 1
			fi
			shift
			;;
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

go_test_args=(-p 1 -tags e2e -v -count=1 -timeout 300s)
go_test_packages=(./tests/e2e/...)
if [[ "$go_package_selector" == "assistant" ]]; then
	echo "go-e2e: applying package selector: assistant"
	go_test_packages=(./tests/e2e/assistant)
fi
if [[ -n "$go_run_selector" ]]; then
	echo "go-e2e: applying -run selector: $go_run_selector"
	go_test_args+=(-run "$go_run_selector")
fi
go_test_args+=("${go_test_packages[@]}")

go test "${go_test_args[@]}"
