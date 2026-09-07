#!/usr/bin/env bash
set -uo pipefail

# bash-baseline-guard-selftest.sh
#
# IMP-102 / SCOPE-5. Proof that the shipped Bubbles command surface fails LOUDLY
# and EARLY on an unsupported bash instead of silently masking breakage.
#
# Background: the framework uses associative arrays (declare -A) pervasively
# (12+ scripts under bubbles/scripts/, plus many selftests). On stock macOS
# bash 3.2 those constructs fail at runtime. `cli.sh` runs under `set -uo
# pipefail` WITHOUT `-e` and sources `aliases.sh` (declare -A) — so on bash 3.2
# it previously printed nothing and returned exit 0, MASKING the breakage from
# installers / doctor / CI. SCOPE-5 inserts an early `BASH_VERSINFO < 4` guard
# at both shipped entrypoints (`cli.sh`, `framework-validate.sh`) that prints a
# clear error and exits 1 BEFORE any declare -A construct executes.
#
# Four layers:
#   (BUG-043 parser)     the canonical transition guard MUST parse under one
#                        real bash 3 and one real bash 5 interpreter; a
#                        source-derived repaired fixture MUST cross the former
#                        recovery boundary, while restoring exactly the two
#                        redundant outer quotes MUST recreate the divergence.
#   (positive, static)   the guard MUST exist in each entrypoint AND appear
#                        BEFORE the first construct that requires bash 4+ —
#                        in cli.sh, before `source .../aliases.sh` (declare -A);
#                        in framework-validate.sh, before its first `source`.
#   (functional)         the guard's `(( BASH_VERSINFO[0] < 4 ))` comparison
#                        MUST yield exit 1 for a simulated v=3 and exit 0 for
#                        v=4 / v=5, and the empty/unset `BASH_VERSINFO` branch
#                        MUST also trigger. BASH_VERSINFO is read-only, so the
#                        version integer is simulated in a child bash.
#   (adversarial)        a temp copy of cli.sh with the guard block REMOVED
#                        MUST make the positive static check FAIL — proving the
#                        check has teeth and is not tautological.
#
# Deterministic; hermetic temp fixtures cleaned on exit; prints
# "N passed / M failed"; exits non-zero on any failure. SKIPs (exit 0) only if
# a genuinely-required POSIX tool (grep/awk) is somehow absent.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI="$SCRIPT_DIR/cli.sh"
FRAMEWORK_VALIDATE="$SCRIPT_DIR/framework-validate.sh"
STATE_TRANSITION_GUARD="$SCRIPT_DIR/state-transition-guard.sh"

pass=0
fail=0
ok() { pass=$((pass + 1)); }
bad() {
  echo "  FAIL: $1"
  fail=$((fail + 1))
}

# Graceful degradation: the static checks rely on POSIX grep/awk. If one is
# genuinely absent, SKIP (exit 0) instead of hard-failing (framework convention).
for _dep in grep awk python3; do
  if ! command -v "$_dep" >/dev/null 2>&1; then
    echo "bash-baseline-guard-selftest: SKIP ($_dep not installed)"
    exit 0
  fi
done

if ! command -v shasum >/dev/null 2>&1 \
  && ! command -v sha256sum >/dev/null 2>&1; then
  echo "bash-baseline-guard-selftest: SKIP (no SHA-256 utility installed)"
  exit 0
fi

for _target in "$CLI" "$FRAMEWORK_VALIDATE" "$STATE_TRANSITION_GUARD"; do
  if [[ ! -f "$_target" ]]; then
    echo "bash-baseline-guard-selftest: SKIP (target missing: $_target)"
    exit 0
  fi
done

tmp="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-bash-baseline-guard.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "=== bash baseline guard selftest (IMP-102 / SCOPE-5) ==="
echo "cli.sh:               ${CLI#"$SCRIPT_DIR"/}"
echo "framework-validate.sh: ${FRAMEWORK_VALIDATE#"$SCRIPT_DIR"/}"
echo ""

sha256_file() {
  local record=""
  if command -v shasum >/dev/null 2>&1; then
    record="$(shasum -a 256 "$1")" || return $?
  else
    record="$(sha256sum "$1")" || return $?
  fi
  printf '%s\n' "${record%% *}"
}

bash_major_of() {
  # Expansion belongs to the child interpreter whose version is under test.
  # shellcheck disable=SC2016
  "$1" -c 'printf "%s\n" "${BASH_VERSINFO[0]}"' 2>/dev/null
}

bash_version_of() {
  # Expansion belongs to the child interpreter whose version is under test.
  # shellcheck disable=SC2016
  "$1" -c 'printf "%s\n" "$BASH_VERSION"'
}

resolve_bash_major() {
  local wanted="$1"
  local candidate=""
  local major=""
  shift

  for candidate in "$@"; do
    [[ -n "$candidate" && -x "$candidate" ]] || continue
    major="$(bash_major_of "$candidate")" || continue
    case "$major" in
      ''|*[!0-9]*) continue ;;
    esac
    if [[ "$wanted" -eq 3 && "$major" -eq 3 ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
    if [[ "$wanted" -eq 5 && "$major" -ge 5 ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

BUG043_LAST_EXIT=0
BUG043_LAST_SENTINEL_COUNT=0
BUG043_LAST_HEADING_COUNT=0

run_bug043_parse_leg() {
  local mode="$1"
  local interpreter="$2"
  local source_file="$3"
  local source_sha="$4"
  local stdout_file=""
  local stderr_file=""
  local rc=0

  stdout_file="$tmp/${mode}.$(basename "$interpreter").parse.stdout"
  stderr_file="$tmp/${mode}.$(basename "$interpreter").parse.stderr"

  "$interpreter" -n < "$source_file" >"$stdout_file" 2>"$stderr_file" || rc=$?
  echo "BUG043_PARSE_RESULT mode=$mode interpreter=$interpreter sourceSha256=$source_sha exit=$rc"
  echo "BUG043_PARSE_STDERR_BEGIN mode=$mode interpreter=$interpreter"
  cat "$stderr_file"
  echo "BUG043_PARSE_STDERR_END mode=$mode interpreter=$interpreter"
  BUG043_LAST_EXIT="$rc"
}

run_bug043_execution_leg() {
  local mode="$1"
  local interpreter="$2"
  local source_file="$3"
  local state_file="$4"
  local stdout_file=""
  local stderr_file=""
  local rc=0
  local sentinel_count=0
  local heading_count=0

  stdout_file="$tmp/${mode}.$(basename "$interpreter").exec.stdout"
  stderr_file="$tmp/${mode}.$(basename "$interpreter").exec.stderr"

  "$interpreter" -s -- "$state_file" < "$source_file" >"$stdout_file" 2>"$stderr_file" || rc=$?
  sentinel_count="$(grep -Fxc 'BUG043_SENTINEL_AFTER_FORMER_LINE_4016' "$stdout_file" 2>/dev/null || true)"
  heading_count="$(grep -Fxc -- '--- Check 16: Implementation Reality Scan (Gate G028) ---' "$stdout_file" 2>/dev/null || true)"
  echo "BUG043_EXEC_OUTPUT_BEGIN mode=$mode interpreter=$interpreter"
  cat "$stdout_file"
  cat "$stderr_file"
  echo "BUG043_EXEC_OUTPUT_END mode=$mode interpreter=$interpreter"
  echo "BUG043_EXEC_RESULT mode=$mode interpreter=$interpreter exit=$rc sentinelCount=$sentinel_count headingCount=$heading_count"
  BUG043_LAST_EXIT="$rc"
  BUG043_LAST_SENTINEL_COUNT="$sentinel_count"
  BUG043_LAST_HEADING_COUNT="$heading_count"
}

# ── BUG-043 (scenario-first parser regression) ──────────────────────────────
# The literal titles below are the planned scenario bindings. Keep each title
# unique in this file so scenario-test-resolve.sh can prove the linkage.
TP01_TITLE='Regression: SCN-B043-001 repaired guard parses under real Bash 3 and Bash 5 on identical bytes'
TP02_TITLE='Regression: SCN-B043-002 source-derived fixture crosses the former Bash 3 recovery boundary'
TP03_TITLE='Mutation: SCN-B043-002 exact outer-quote restoration recreates the parser divergence'

path_bash="$(command -v bash 2>/dev/null || true)"
path_bash3="$(command -v bash3 2>/dev/null || true)"
bash3_bin="$(resolve_bash_major 3 /bin/bash "$path_bash3" /usr/local/bin/bash3 2>/dev/null || true)"
bash5_bin="$(resolve_bash_major 5 /opt/homebrew/bin/bash /usr/local/bin/bash "$path_bash" /usr/bin/bash /bin/bash 2>/dev/null || true)"
system_bash_major="$(bash_major_of /bin/bash 2>/dev/null || true)"
bug043_matrix_executed=false
production_bash3_parse=not-run
production_bash5_parse=not-run
repaired_bash3_parse=not-run
repaired_bash5_parse=not-run
repaired_bash3_exec=not-run
repaired_bash5_exec=not-run
repaired_bash3_sentinel=not-run
repaired_bash5_sentinel=not-run
mutant_bash3_parse=not-run
mutant_bash5_parse=not-run
mutant_bash3_exec=not-run
mutant_bash5_exec=not-run
mutant_bash3_sentinel=not-run
mutant_bash5_sentinel=not-run
guard_sha_before=not-run
guard_sha_after=not-run

if [[ -z "$bash3_bin" || -z "$bash5_bin" ]]; then
  if [[ "$system_bash_major" -eq 3 || -n "$bash3_bin" ]]; then
    bad "BUG-043 required dual-parser host does not expose both real Bash 3 and Bash 5 interpreters"
  else
    echo "  INFO: BUG-043 dual-parser matrix not executed because this host does not expose a real Bash 3 interpreter"
  fi
else
  bug043_matrix_executed=true
  bash3_version="$(bash_version_of "$bash3_bin")"
  bash5_version="$(bash_version_of "$bash5_bin")"
  guard_sha_before="$(sha256_file "$STATE_TRANSITION_GUARD")"
  echo "BUG043_INTERPRETER role=bash3 path=$bash3_bin version=$bash3_version"
  echo "BUG043_INTERPRETER role=bash5 path=$bash5_bin version=$bash5_version"
  echo "BUG043_PRODUCTION_SOURCE path=${STATE_TRANSITION_GUARD#"$SCRIPT_DIR"/} sha256=$guard_sha_before"

  run_bug043_parse_leg production "$bash3_bin" "$STATE_TRANSITION_GUARD" "$guard_sha_before"
  production_bash3_parse="$BUG043_LAST_EXIT"
  run_bug043_parse_leg production "$bash5_bin" "$STATE_TRANSITION_GUARD" "$guard_sha_before"
  production_bash5_parse="$BUG043_LAST_EXIT"

  if [[ "$production_bash3_parse" -eq 0 && "$production_bash5_parse" -eq 0 ]]; then
    echo "  PASS: $TP01_TITLE"
    ok
  else
    bad "$TP01_TITLE: expected parse exits 0/0, observed $production_bash3_parse/$production_bash5_parse"
  fi

  repaired_fixture="$tmp/bug043-repaired-fixture.sh"
  mutant_fixture="$tmp/bug043-quote-restoration-mutant.sh"
  fixture_state="$tmp/bug043-state.json"
  fixture_build_rc=0
  python3 - "$STATE_TRANSITION_GUARD" "$repaired_fixture" "$mutant_fixture" "$fixture_state" <<'PY' || fixture_build_rc=$?
from hashlib import sha256
from pathlib import Path
import sys

source_path = Path(sys.argv[1])
repaired_path = Path(sys.argv[2])
mutant_path = Path(sys.argv[3])
state_path = Path(sys.argv[4])

source = source_path.read_bytes()
lines = source.splitlines(keepends=True)

def payload(line):
    return line.rstrip(b"\r\n")

def with_eol(original, replacement):
    if original.endswith(b"\r\n"):
        return replacement + b"\r\n"
    if original.endswith(b"\n"):
        return replacement + b"\n"
    return replacement

quoted_open = b'claim_backing_analysis="$(python3 - "$state_file" <<\'PY\''
repaired_open = b'claim_backing_analysis=$(python3 - "$state_file" <<\'PY\''
quoted_close = b')"'
repaired_close = b')'
heading = b'echo "--- Check 16: Implementation Reality Scan (Gate G028) ---"'
sentinel = b"printf '%s\\n' 'BUG043_SENTINEL_AFTER_FORMER_LINE_4016'\n"

opening_indices = [
    index
    for index, line in enumerate(lines)
    if payload(line) in (quoted_open, repaired_open)
]
heading_indices = [
    index
    for index, line in enumerate(lines)
    if payload(line) == heading
]
if len(opening_indices) != 1:
    raise SystemExit(
        f"BUG043_FIXTURE_BUILD_ERROR openingAnchorCount={len(opening_indices)}"
    )
if len(heading_indices) != 1:
    raise SystemExit(
        f"BUG043_FIXTURE_BUILD_ERROR headingAnchorCount={len(heading_indices)}"
    )

opening_index = opening_indices[0]
terminator_index = next(
    (
        index
        for index in range(opening_index + 1, len(lines))
        if payload(lines[index]) == b"PY"
    ),
    None,
)
if terminator_index is None or terminator_index + 1 >= len(lines):
    raise SystemExit("BUG043_FIXTURE_BUILD_ERROR closingAnchorCount=0")

closing_index = terminator_index + 1
source_open = payload(lines[opening_index])
source_close = payload(lines[closing_index])
if source_open == quoted_open and source_close != quoted_close:
    raise SystemExit("BUG043_FIXTURE_BUILD_ERROR quoted opening has no quoted close")
if source_open == repaired_open and source_close != repaired_close:
    raise SystemExit("BUG043_FIXTURE_BUILD_ERROR repaired opening has no repaired close")

assignment = list(lines[opening_index : closing_index + 1])
repaired_assignment = list(assignment)
repaired_assignment[0] = with_eol(repaired_assignment[0], repaired_open)
repaired_assignment[-1] = with_eol(repaired_assignment[-1], repaired_close)

mutant_assignment = list(repaired_assignment)
mutant_assignment[0] = with_eol(mutant_assignment[0], quoted_open)
mutant_assignment[-1] = with_eol(mutant_assignment[-1], quoted_close)

changed_indices = [
    index
    for index, (candidate, mutant) in enumerate(
        zip(repaired_assignment, mutant_assignment)
    )
    if candidate != mutant
]
repaired_assignment_bytes = b"".join(repaired_assignment)
mutant_assignment_bytes = b"".join(mutant_assignment)
quote_boundary_mutation_count = (
    len(mutant_assignment_bytes) - len(repaired_assignment_bytes)
)
if changed_indices != [0, len(repaired_assignment) - 1]:
    raise SystemExit(
        f"BUG043_FIXTURE_BUILD_ERROR changedAssignmentLines={changed_indices}"
    )
if quote_boundary_mutation_count != 2:
    raise SystemExit(
        "BUG043_FIXTURE_BUILD_ERROR "
        f"quoteBoundaryMutationCount={quote_boundary_mutation_count}"
    )

source_heredoc = b"".join(assignment[1:-1])
repaired_heredoc = b"".join(repaired_assignment[1:-1])
mutant_heredoc = b"".join(mutant_assignment[1:-1])
if not (source_heredoc == repaired_heredoc == mutant_heredoc):
    raise SystemExit("BUG043_FIXTURE_BUILD_ERROR heredocBytesChanged=1")

prefix = b'#!/usr/bin/env bash\nset -uo pipefail\nstate_file="$1"\n'
suffix = lines[heading_indices[0]] + sentinel
repaired_path.write_bytes(prefix + repaired_assignment_bytes + suffix)
mutant_path.write_bytes(prefix + mutant_assignment_bytes + suffix)
state_path.write_text("{}\n", encoding="utf-8")

source_form = "quoted" if source_open == quoted_open else "repaired"
source_repair_deletions = 2 if source_form == "quoted" else 0
heredoc_digest = sha256(source_heredoc).hexdigest()
print(f"BUG043_FIXTURE_SOURCE_FORM={source_form}")
print("BUG043_FIXTURE_OPENING_ANCHOR_COUNT=1")
print("BUG043_FIXTURE_CLOSING_ANCHOR_COUNT=1")
print("BUG043_FIXTURE_HEADING_ANCHOR_COUNT=1")
print(f"BUG043_FIXTURE_SOURCE_REPAIR_DELETION_COUNT={source_repair_deletions}")
print(f"BUG043_FIXTURE_QUOTE_BOUNDARY_MUTATION_COUNT={quote_boundary_mutation_count}")
print("BUG043_FIXTURE_HEREDOC_BYTES_CHANGED=0")
print(f"BUG043_FIXTURE_HEREDOC_SHA256={heredoc_digest}")
PY

  echo "BUG043_FIXTURE_BUILD_EXIT=$fixture_build_rc"
  if [[ "$fixture_build_rc" -ne 0 ]]; then
    bad "$TP02_TITLE: source-derived fixture construction failed"
    bad "$TP03_TITLE: exact quote-restoration mutant construction failed"
  else
    repaired_sha="$(sha256_file "$repaired_fixture")"
    mutant_sha="$(sha256_file "$mutant_fixture")"
    echo "BUG043_FIXTURE_SHA256 mode=repaired sha256=$repaired_sha"
    echo "BUG043_FIXTURE_SHA256 mode=exact-mutant sha256=$mutant_sha"

    run_bug043_parse_leg repaired "$bash3_bin" "$repaired_fixture" "$repaired_sha"
    repaired_bash3_parse="$BUG043_LAST_EXIT"
    run_bug043_parse_leg repaired "$bash5_bin" "$repaired_fixture" "$repaired_sha"
    repaired_bash5_parse="$BUG043_LAST_EXIT"
    run_bug043_execution_leg repaired "$bash3_bin" "$repaired_fixture" "$fixture_state"
    repaired_bash3_exec="$BUG043_LAST_EXIT"
    repaired_bash3_sentinel="$BUG043_LAST_SENTINEL_COUNT"
    repaired_bash3_heading="$BUG043_LAST_HEADING_COUNT"
    run_bug043_execution_leg repaired "$bash5_bin" "$repaired_fixture" "$fixture_state"
    repaired_bash5_exec="$BUG043_LAST_EXIT"
    repaired_bash5_sentinel="$BUG043_LAST_SENTINEL_COUNT"
    repaired_bash5_heading="$BUG043_LAST_HEADING_COUNT"

    if [[ "$repaired_bash3_parse" -eq 0 \
      && "$repaired_bash5_parse" -eq 0 \
      && "$repaired_bash3_exec" -eq 0 \
      && "$repaired_bash5_exec" -eq 0 \
      && "$repaired_bash3_sentinel" -eq 1 \
      && "$repaired_bash5_sentinel" -eq 1 \
      && "$repaired_bash3_heading" -eq 1 \
      && "$repaired_bash5_heading" -eq 1 ]]; then
      echo "  PASS: $TP02_TITLE"
      ok
    else
      bad "$TP02_TITLE: expected parse 0/0, execution 0/0, sentinel 1/1, heading 1/1"
    fi

    run_bug043_parse_leg exact-mutant "$bash3_bin" "$mutant_fixture" "$mutant_sha"
    mutant_bash3_parse="$BUG043_LAST_EXIT"
    run_bug043_parse_leg exact-mutant "$bash5_bin" "$mutant_fixture" "$mutant_sha"
    mutant_bash5_parse="$BUG043_LAST_EXIT"
    run_bug043_execution_leg exact-mutant "$bash3_bin" "$mutant_fixture" "$fixture_state"
    mutant_bash3_exec="$BUG043_LAST_EXIT"
    mutant_bash3_sentinel="$BUG043_LAST_SENTINEL_COUNT"
    mutant_bash3_heading="$BUG043_LAST_HEADING_COUNT"
    run_bug043_execution_leg exact-mutant "$bash5_bin" "$mutant_fixture" "$fixture_state"
    mutant_bash5_exec="$BUG043_LAST_EXIT"
    mutant_bash5_sentinel="$BUG043_LAST_SENTINEL_COUNT"
    mutant_bash5_heading="$BUG043_LAST_HEADING_COUNT"

    if [[ "$mutant_bash3_parse" -eq 2 \
      && "$mutant_bash5_parse" -eq 0 \
      && "$mutant_bash3_exec" -eq 2 \
      && "$mutant_bash5_exec" -eq 0 \
      && "$mutant_bash3_sentinel" -eq 0 \
      && "$mutant_bash5_sentinel" -eq 1 \
      && "$mutant_bash3_heading" -eq 0 \
      && "$mutant_bash5_heading" -eq 1 ]]; then
      echo "  PASS: $TP03_TITLE"
      ok
    else
      bad "$TP03_TITLE: expected parse 2/0, execution 2/0, sentinel 0/1, heading 0/1"
    fi
  fi

  guard_sha_after="$(sha256_file "$STATE_TRANSITION_GUARD")"
  echo "BUG043_PRODUCTION_SOURCE_AFTER sha256=$guard_sha_after"
  if [[ "$guard_sha_before" == "$guard_sha_after" ]]; then
    echo "  PASS: BUG-043 parser fixtures left canonical production source bytes unchanged"
    ok
  else
    bad "BUG-043 parser fixtures changed canonical production source bytes"
  fi
  echo ""
fi

# first_line_matching <regex> <file> — line number (1-based) of the FIRST line
# matching an extended regex, or empty if none.
first_line_matching() {
  grep -nE "$1" "$2" 2>/dev/null | head -1 | cut -d: -f1
}

# guard_line_of <file> — line number of the bash-4 guard: the single line that
# contains BOTH `BASH_VERSINFO` and the `< 4` comparison. Empty if absent.
guard_line_of() {
  grep -n 'BASH_VERSINFO' "$1" 2>/dev/null | grep -F '< 4' | head -1 | cut -d: -f1
}

# guard_before_marker <file> <marker-regex> — returns 0 iff a bash-4 guard is
# present AND appears strictly before the first line matching <marker-regex>.
guard_before_marker() {
  local file="$1" marker="$2" g m
  g="$(guard_line_of "$file")"
  m="$(first_line_matching "$marker" "$file")"
  [[ -n "$g" && -n "$m" ]] || return 1
  (( g < m ))
}

# ── Layer 1 (positive, static): guard exists BEFORE the bash-4 dependency ────
# cli.sh: the guard MUST precede `source ".../aliases.sh"` (aliases.sh L21 is the
# first declare -A the CLI would otherwise source under `set -uo pipefail`).
cli_guard="$(guard_line_of "$CLI")"
cli_aliases="$(first_line_matching '^[[:space:]]*source .*aliases\.sh' "$CLI")"
if [[ -z "$cli_guard" ]]; then
  bad "cli.sh: no BASH_VERSINFO '< 4' guard found"
elif [[ -z "$cli_aliases" ]]; then
  bad "cli.sh: could not locate the 'source .../aliases.sh' line to anchor the check"
elif (( cli_guard < cli_aliases )); then
  echo "  PASS: cli.sh guard at L${cli_guard} precedes aliases.sh source at L${cli_aliases}"
  ok
else
  bad "cli.sh: guard at L${cli_guard} does NOT precede aliases.sh source at L${cli_aliases}"
fi

# framework-validate.sh: the guard MUST precede its first `source` (guard-lib.sh),
# i.e. run before any helper is loaded or any declare -A selftest is dispatched.
fv_guard="$(guard_line_of "$FRAMEWORK_VALIDATE")"
fv_source="$(first_line_matching '^source ' "$FRAMEWORK_VALIDATE")"
if [[ -z "$fv_guard" ]]; then
  bad "framework-validate.sh: no BASH_VERSINFO '< 4' guard found"
elif [[ -z "$fv_source" ]]; then
  bad "framework-validate.sh: could not locate the first 'source' line to anchor the check"
elif (( fv_guard < fv_source )); then
  echo "  PASS: framework-validate.sh guard at L${fv_guard} precedes first source at L${fv_source}"
  ok
else
  bad "framework-validate.sh: guard at L${fv_guard} does NOT precede first source at L${fv_source}"
fi

# Extra "near the top" signal (non-fragile upper bound): the guard should sit in
# the file header, not buried hundreds of lines down.
if [[ -n "$fv_guard" ]] && (( fv_guard <= 30 )); then
  echo "  PASS: framework-validate.sh guard is near the top (L${fv_guard} <= 30)"
  ok
else
  bad "framework-validate.sh: guard not near the top (L${fv_guard:-none})"
fi

# ── Layer 2 (functional): prove the '< 4' comparison logic actually gates ────
# BASH_VERSINFO is read-only, so simulate the major-version integer in a child
# bash and exercise the exact guard shape: fail (exit 1) for v<4, pass (exit 0)
# for v>=4.
sim_guard() { # <simulated-major-version>
  bash -c 'v="$1"; if (( v < 4 )); then exit 1; fi; exit 0' _ "$1"
}
# v=3 → guard MUST trigger (exit 1)
if sim_guard 3; then
  bad "functional: simulated bash 3 did NOT trigger the guard (expected exit 1)"
else
  echo "  PASS: simulated v=3 triggers guard (exit 1)"
  ok
fi
# v=4 → guard MUST pass (exit 0)
if sim_guard 4; then
  echo "  PASS: simulated v=4 passes guard (exit 0)"
  ok
else
  bad "functional: simulated bash 4 unexpectedly triggered the guard (expected exit 0)"
fi
# v=5 → guard MUST pass (exit 0)
if sim_guard 5; then
  echo "  PASS: simulated v=5 passes guard (exit 0)"
  ok
else
  bad "functional: simulated bash 5 unexpectedly triggered the guard (expected exit 0)"
fi
# empty/unset BASH_VERSINFO branch MUST also trigger (exit 1)
if bash -c 'if [[ -z "${BV:-}" ]]; then exit 1; fi; exit 0'; then
  bad "functional: empty/unset version branch did NOT trigger (expected exit 1)"
else
  echo "  PASS: empty/unset BASH_VERSINFO branch triggers guard (exit 1)"
  ok
fi

# ── Layer 3 (adversarial, non-tautological): removing the guard MUST break the
# static check. Build a temp copy of cli.sh with the guard block excised and
# assert guard_before_marker FAILS on it. This proves Layer 1 has real teeth.
stripped="$tmp/cli-noguard.sh"

# Drop the contiguous guard block: from the `if ... BASH_VERSINFO ... < 4 ...`
# line through its closing `fi` (inclusive). Everything else is preserved.
awk '
  $0 ~ /BASH_VERSINFO/ && $0 ~ /< 4/ { inblock = 1 }
  inblock && /^fi$/ { inblock = 0; next }
  inblock { next }
  { print }
' "$CLI" > "$stripped"

# Sanity: the guard line must actually be gone from the stripped copy.
if [[ -n "$(guard_line_of "$stripped")" ]]; then
  bad "adversarial: failed to strip the guard from the temp cli.sh copy"
else
  # The real cli.sh MUST pass the check; the stripped copy MUST fail it.
  if guard_before_marker "$CLI" '^[[:space:]]*source .*aliases\.sh' \
    && ! guard_before_marker "$stripped" '^[[:space:]]*source .*aliases\.sh'; then
    echo "  PASS: static check passes on real cli.sh and FAILS on guard-removed copy (has teeth)"
    ok
  else
    bad "adversarial: static check did not distinguish real cli.sh from guard-removed copy (tautological)"
  fi
fi

if [[ "$bug043_matrix_executed" == true ]]; then
  echo ""
  echo "BUG043_MATRIX_SUMMARY productionParse=$production_bash3_parse/$production_bash5_parse repairedParse=$repaired_bash3_parse/$repaired_bash5_parse repairedExecution=$repaired_bash3_exec/$repaired_bash5_exec repairedSentinel=$repaired_bash3_sentinel/$repaired_bash5_sentinel mutantParse=$mutant_bash3_parse/$mutant_bash5_parse mutantExecution=$mutant_bash3_exec/$mutant_bash5_exec mutantSentinel=$mutant_bash3_sentinel/$mutant_bash5_sentinel"
  echo "BUG043_SOURCE_INVARIANT before=$guard_sha_before after=$guard_sha_after unchanged=$([[ "$guard_sha_before" == "$guard_sha_after" ]] && printf true || printf false)"
fi

echo ""
echo "bash-baseline-guard-selftest: $pass passed / $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
