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
go_test_args+=(./tests/integration/... ./internal/notification/... ./internal/assistant/... ./internal/cardrewards/... ./tests/eval/...)

# BUG-061-011 — ./tests/eval/... above carries the assistant acceptance gate
# (TestAcceptanceGate_RoutingAccuracyAndCaptureFallback, build tag
# `integration`). Listing the package is only half the contract: a package can
# be selected and still contribute zero executed assertions if the tag stops
# matching, the corpus fails to load, or every case is skipped. The gate
# therefore emits one machine-readable marker line and this lane asserts on it.
#
# The output is tee'd to a temp file OUTSIDE /workspace so the console keeps
# streaming live while the assertion reads the same bytes, and so no untracked
# artifact appears in the repository tree.
gate_marker_prefix="ASSISTANT_ACCEPTANCE_GATE_V1"
gate_output_file="$(mktemp)"
cleanup_gate_output() {
	rm -f "$gate_output_file"
}
trap cleanup_gate_output EXIT

# Capture the whole pipeline status in ONE assignment: a second assignment
# would read a PIPESTATUS already reset by the first. Under `set -e` the `if !`
# form keeps the script alive so the marker check below still runs.
go_test_pipe_status=(0 0)
if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
	go_test_pipe_status=("${PIPESTATUS[@]}")
fi
go_test_rc="${go_test_pipe_status[0]}"
tee_rc="${go_test_pipe_status[1]}"

if [[ "$tee_rc" -ne 0 ]]; then
	echo "ERROR: go-integration: could not capture go test output (tee exit ${tee_rc}); the acceptance-gate assertion cannot be trusted." >&2
	exit 1
fi

gate_marker_check_failed=0
if [[ -z "$go_run_selector" ]]; then # full-lane run: acceptance-gate assertion is ENFORCED
	gate_marker_count=0
	if grep -q "^${gate_marker_prefix} " "$gate_output_file"; then
		gate_marker_count="$(grep -c "^${gate_marker_prefix} " "$gate_output_file")"
	fi

	if [[ "$gate_marker_count" -eq 0 ]]; then
		echo "ERROR: go-integration: the assistant acceptance gate did not run — no ${gate_marker_prefix} line was emitted by TestAcceptanceGate_RoutingAccuracyAndCaptureFallback." >&2
		gate_marker_check_failed=1
	elif [[ "$gate_marker_count" -gt 1 ]]; then
		echo "ERROR: go-integration: ambiguous acceptance-gate result — ${gate_marker_count} ${gate_marker_prefix} lines were emitted by TestAcceptanceGate_RoutingAccuracyAndCaptureFallback; exactly one is required." >&2
		gate_marker_check_failed=1
	else
		gate_marker_line="$(grep -m 1 "^${gate_marker_prefix} " "$gate_output_file")"
		gate_executed_assertions="${gate_marker_line##*executed_assertions=}"
		gate_executed_assertions="${gate_executed_assertions%% *}"
		if [[ ! "$gate_executed_assertions" =~ ^[0-9]+$ ]]; then
			echo "ERROR: go-integration: TestAcceptanceGate_RoutingAccuracyAndCaptureFallback reported a non-numeric executed_assertions value: ${gate_marker_line}" >&2
			gate_marker_check_failed=1
		elif [[ "$gate_executed_assertions" -lt 1 ]]; then
			echo "ERROR: go-integration: TestAcceptanceGate_RoutingAccuracyAndCaptureFallback evaluated nothing (executed_assertions must be >= 1): ${gate_marker_line}" >&2
			gate_marker_check_failed=1
		else
			echo "go-integration: acceptance gate executed ${gate_executed_assertions} assertions."
		fi
	fi
else
	echo "go-integration: NOTICE: acceptance-gate executed-assertion assertion NOT ENFORCED for this run — a focused --run selector (${go_run_selector}) is active. Only a full lane run with no --run selector enforces that TestAcceptanceGate_RoutingAccuracyAndCaptureFallback ran with a non-zero executed-assertion count."
fi

# Neither failure may mask the other: report both, then exit non-zero.
if [[ "$go_test_rc" -ne 0 ]]; then
	echo "ERROR: go-integration: go test failed (exit ${go_test_rc})." >&2
fi
if [[ "$go_test_rc" -ne 0 ]]; then
	exit "$go_test_rc"
fi
if [[ "$gate_marker_check_failed" -ne 0 ]]; then
	exit 1
fi
