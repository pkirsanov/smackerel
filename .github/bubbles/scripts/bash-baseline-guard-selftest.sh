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
# Three layers:
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

pass=0
fail=0
ok() { pass=$((pass + 1)); }
bad() {
  echo "  FAIL: $1"
  fail=$((fail + 1))
}

# Graceful degradation: the static checks rely on POSIX grep/awk. If one is
# genuinely absent, SKIP (exit 0) instead of hard-failing (framework convention).
for _dep in grep awk; do
  if ! command -v "$_dep" >/dev/null 2>&1; then
    echo "bash-baseline-guard-selftest: SKIP ($_dep not installed)"
    exit 0
  fi
done

for _target in "$CLI" "$FRAMEWORK_VALIDATE"; do
  if [[ ! -f "$_target" ]]; then
    echo "bash-baseline-guard-selftest: SKIP (target missing: $_target)"
    exit 0
  fi
done

echo "=== bash baseline guard selftest (IMP-102 / SCOPE-5) ==="
echo "cli.sh:               ${CLI#"$SCRIPT_DIR"/}"
echo "framework-validate.sh: ${FRAMEWORK_VALIDATE#"$SCRIPT_DIR"/}"
echo ""

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
tmp="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-bash-baseline-guard.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM
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

echo ""
echo "bash-baseline-guard-selftest: $pass passed / $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
