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

stress_packages=()
while IFS= read -r package_path; do
	stress_packages+=("$package_path")
done < <(go list -tags stress ./tests/stress/...)

go_stress_package_has_selected_tests() {
	local package_path="$1"
	local found_match=false
	if [[ -z "$go_run_selector" ]]; then
		return 0
	fi
	while IFS= read -r test_name; do
		case "$test_name" in
			Test*)
				found_match=true
				;;
		esac
	done < <(go test -tags stress -list "$go_run_selector" "$package_path")
	if [[ "$found_match" == true ]]; then
		return 0
	fi
	echo "go-stress: skipping workload package $package_path (no tests match selector)"
	return 1
}

ran_package_count=0
for package_path in "${stress_packages[@]}"; do
	if ! go_stress_package_has_selected_tests "$package_path"; then
		continue
	fi
	echo "go-stress: running workload package $package_path"
	go test "${go_test_args[@]}" "$package_path"
	ran_package_count=$((ran_package_count + 1))
done

if [[ "$ran_package_count" -eq 0 ]]; then
	echo "ERROR: go-stress selector matched zero stress packages: $go_run_selector" >&2
	exit 1
fi

echo "go-stress: workload packages passed"
