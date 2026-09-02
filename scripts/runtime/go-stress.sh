#!/usr/bin/env bash
set -euo pipefail

go_stress_output_matches() {
	local output_file="$1"
	local pattern="$2"
	local grep_rc
	if grep -Eq "$pattern" "$output_file"; then
		return 0
	else
		grep_rc=$?
	fi
	if [[ "$grep_rc" -eq 1 ]]; then
		return 1
	fi
	echo "ERROR: go-stress: output classification could not be trusted (grep exit $grep_rc)." >&2
	return "$grep_rc"
}

go_stress_classify_output() {
	local output_file="$1"
	if [[ ! -r "$output_file" ]]; then
		echo "ERROR: go-stress: output classification could not be trusted because the capture is unreadable." >&2
		return 1
	fi

	local -a patterns
	local -a classes
	patterns=(
		'^[[:space:]]*--- SKIP:'
		'^[[:space:]]*([[:alnum:]_.-]+=[^"[:space:]]+[[:space:]]+)*level=WARN([[:space:]]|$)'
		'^[[:space:]]*(time=)?[0-9]{4}[-/][0-9]{2}[-/][0-9]{2}[T ][0-9]{2}:[0-9]{2}:[0-9]{2}[^[:space:]]*[[:space:]]+WARN([[:space:]]|$)'
		'^[[:space:]]*[{][^\\{}]*"level"[[:space:]]*:[[:space:]]*"WARN"([[:space:]]*[,}])'
	)
	classes=(
		"go-test-skip"
		"runtime-warning-level"
		"runtime-warning-timestamp"
		"runtime-warning-json"
	)

	local index
	local match_rc
	for ((index = 0; index < ${#patterns[@]}; index++)); do
		if go_stress_output_matches "$output_file" "${patterns[$index]}"; then
			printf '%s\n' "${classes[$index]}"
			return 0
		else
			match_rc=$?
		fi
		if [[ "$match_rc" -ne 1 ]]; then
			return "$match_rc"
		fi
	done
	printf '%s\n' "clean"
}

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
	return 0
fi

# spec-052 chaos finding: stress tests that may shell out to
# scripts/commands/config.sh require envsubst (gettext-base) which
# is not present in the golang:bookworm base image. Use the shared
# helper so all four go-*.sh wrappers share one envsubst-install
# implementation.
# shellcheck source=scripts/runtime/_ensure_envsubst.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-stress"

go_stress_output_file=""
go_stress_list_output_file=""
go_stress_cleanup_outputs() {
	if [[ -n "$go_stress_output_file" ]]; then
		rm -f "$go_stress_output_file"
		go_stress_output_file=""
	fi
	if [[ -n "$go_stress_list_output_file" ]]; then
		rm -f "$go_stress_list_output_file"
		go_stress_list_output_file=""
	fi
}
trap go_stress_cleanup_outputs EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

go_stress_run_checked_package() {
	local package_path="$1"
	shift
	go_stress_cleanup_outputs
	if ! go_stress_output_file="$(mktemp)"; then
		echo "ERROR: go-stress: package $package_path could not allocate an output capture file; test output classification cannot run." >&2
		return 1
	fi

	local -a go_test_pipe_status
	local go_test_rc
	local tee_rc
	go_test_pipe_status=(0 0)
	# Capture PIPESTATUS in one assignment. Any intervening command would reset it.
	if ! go test "$@" 2>&1 | tee "$go_stress_output_file"; then
		go_test_pipe_status=("${PIPESTATUS[@]}")
	fi
	go_test_rc="${go_test_pipe_status[0]}"
	tee_rc="${go_test_pipe_status[1]}"

	if [[ "$tee_rc" -ne 0 ]]; then
		echo "ERROR: go-stress: package $package_path output capture failed (tee exit $tee_rc); skip and warning classification cannot be trusted." >&2
	fi
	if [[ "$go_test_rc" -ne 0 ]]; then
		go_stress_cleanup_outputs
		return "$go_test_rc"
	fi
	if [[ "$tee_rc" -ne 0 ]]; then
		go_stress_cleanup_outputs
		return 1
	fi

	local output_class
	local classifier_rc
	if output_class="$(go_stress_classify_output "$go_stress_output_file")"; then
		classifier_rc=0
	else
		classifier_rc=$?
		echo "ERROR: go-stress: package $package_path output classification failed (classifier exit $classifier_rc)." >&2
		go_stress_cleanup_outputs
		return "$classifier_rc"
	fi
	case "$output_class" in
		clean)
			;;
		go-test-skip)
			echo "ERROR: go-stress: package $package_path emitted forbidden output class go-test-skip (--- SKIP:); remove the skip and make the stress assertion execute." >&2
			go_stress_cleanup_outputs
			return 1
			;;
		runtime-warning-level)
			echo "ERROR: go-stress: package $package_path emitted forbidden output class runtime-warning-level (level=WARN); fix the warning-producing runtime path before stress can pass." >&2
			go_stress_cleanup_outputs
			return 1
			;;
		runtime-warning-timestamp)
			echo "ERROR: go-stress: package $package_path emitted forbidden output class runtime-warning-timestamp (timestamp followed by WARN); fix the warning-producing runtime path before stress can pass." >&2
			go_stress_cleanup_outputs
			return 1
			;;
		runtime-warning-json)
			echo "ERROR: go-stress: package $package_path emitted forbidden output class runtime-warning-json (direct JSON level WARN); fix the warning-producing runtime path before stress can pass." >&2
			go_stress_cleanup_outputs
			return 1
			;;
		*)
			echo "ERROR: go-stress: package $package_path output classification failed with class $output_class; inspect capture-file creation and classifier integrity." >&2
			go_stress_cleanup_outputs
			return 1
			;;
	esac

	go_stress_cleanup_outputs
}

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
go_stress_run_checked_package "./tests/stress/readiness" -tags stress -v -count=1 -timeout 90s -run '^TestStressReadinessCanary_Live$' ./tests/stress/readiness
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
	go_stress_selected_package=false
	local found_match=false
	if [[ -z "$go_run_selector" ]]; then
		go_stress_selected_package=true
		return 0
	fi
	go_stress_cleanup_outputs
	if ! go_stress_list_output_file="$(mktemp)"; then
		echo "ERROR: go-stress: package $package_path could not allocate a test-list capture file; selector discovery cannot run." >&2
		return 1
	fi

	local list_rc
	if go test -tags stress -list "$go_run_selector" "$package_path" >"$go_stress_list_output_file" 2>&1; then
		list_rc=0
	else
		list_rc=$?
	fi
	if [[ "$list_rc" -ne 0 ]]; then
		local diagnostic_line=""
		while IFS= read -r diagnostic_line || [[ -n "$diagnostic_line" ]]; do
			printf '%s\n' "$diagnostic_line" >&2
		done < "$go_stress_list_output_file"
		echo "ERROR: go-stress: package $package_path test discovery failed (go test -list exit $list_rc); selector result is unavailable." >&2
		go_stress_cleanup_outputs
		return "$list_rc"
	fi
	while IFS= read -r test_name; do
		case "$test_name" in
			Test*)
				found_match=true
				;;
		esac
	done < "$go_stress_list_output_file"
	go_stress_cleanup_outputs
	if [[ "$found_match" == true ]]; then
		go_stress_selected_package=true
		return 0
	fi
	echo "go-stress: skipping workload package $package_path (no tests match selector)"
	return 0
}

go_stress_selected_package=false
ran_package_count=0
for package_path in "${stress_packages[@]}"; do
	if go_stress_package_has_selected_tests "$package_path"; then
		selection_rc=0
	else
		selection_rc=$?
		exit "$selection_rc"
	fi
	if [[ "$go_stress_selected_package" != true ]]; then
		continue
	fi
	echo "go-stress: running workload package $package_path"
	go_stress_run_checked_package "$package_path" "${go_test_args[@]}" "$package_path"
	ran_package_count=$((ran_package_count + 1))
done

if [[ "$ran_package_count" -eq 0 ]]; then
	echo "ERROR: go-stress selector matched zero stress packages: $go_run_selector" >&2
	exit 1
fi

echo "go-stress: workload packages passed"
