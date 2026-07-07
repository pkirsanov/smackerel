#!/usr/bin/env bash
# macos-portability-guard-selftest.sh
#
# Hermetic selftest for macos-portability-guard.sh. Stages throwaway shell-script
# fixtures under a temp dir and asserts the guard's contract:
#
#   - GREEN fixture  (portable helpers/forms + a # portable-ok: pragma, a
#                     pragma-on-the-line-above, a BASH_VERSINFO-guarded mapfile,
#                     and a full-line comment that MENTIONS raw constructs)
#                    -> exit 0 (clean).
#   - RED fixtures   (one per detected class, each reintroducing exactly ONE
#                     GNU/bash-4.x-only construct) -> exit 1, and the guard NAMES
#                     that class in its output.
#   - Self-portability: the guard parses (bash -n); the guard scanning its OWN
#                     source exits 0; and the guard source (comments stripped)
#                     contains no literal GNU-only form.
#
# Uses throwaway /tmp fixtures and cleans them up. Exit 0 = all assertions pass;
# exit 1 = one or more failed. SKIPs (exit 0) only if a genuinely-required POSIX
# tool (awk/grep/find) is somehow absent, per the framework selftest convention.
#
# Origin: bubbles-cross-platform-shell (canonical mechanical enforcement).

# The fixture strings below are DELIBERATELY single-quoted: they are literal
# shell-script CONTENT written into throwaway files that the guard SCANS (never
# executes), so $(...) / $VAR must NOT expand here. Silence the (info-level)
# SC2016 file-wide — expansion is exactly what we do not want.
# shellcheck disable=SC2016
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/macos-portability-guard.sh"

if [[ ! -f "$TARGET" ]]; then
  echo "[selftest macos-portability-guard] FAIL: target script missing at $TARGET" >&2
  exit 1
fi

# Graceful degradation: the guard relies on POSIX awk/grep/find. If one is
# genuinely absent, SKIP (exit 0) instead of hard-failing (framework convention).
for _dep in awk grep find; do
  if ! command -v "$_dep" >/dev/null 2>&1; then
    echo "[selftest macos-portability-guard] SKIP ($_dep not installed)"
    exit 0
  fi
done

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() {
  echo "  FAIL: $1"
  failures=$((failures + 1))
}

# mk <path> <line...> — write a throwaway shell-script fixture (shebang + lines).
mk() {
  local path="$1"
  shift
  printf '%s\n' '#!/usr/bin/env bash' "$@" >"$path"
}

# run_guard <file> — sets GUARD_OUT + GUARD_RC (never aborts the selftest).
run_guard() {
  set +e
  GUARD_OUT="$(bash "$TARGET" "$1" 2>&1)"
  GUARD_RC=$?
  set -e
}

# assert_red <name> <class-label> <fixture-file>
assert_red() {
  local name="$1" class="$2" file="$3"
  run_guard "$file"
  if [[ "$GUARD_RC" -eq 1 ]] && printf '%s\n' "$GUARD_OUT" | grep -Fq "$class"; then
    pass "RED $name -> exit 1 + names '$class'"
  else
    fail "RED $name -> expected exit 1 + '$class', got rc=$GUARD_RC"
    printf '%s\n' "$GUARD_OUT"
  fi
}

echo "== macos-portability-guard selftest =="

# --- GREEN fixture --------------------------------------------------------
# Portable helpers/forms + all three exemption mechanisms. MUST exit 0.
green="$TMPDIR/green.sh"
mk "$green" \
  'bubbles_run_with_timeout 5 sleep 1' \
  'bubbles_sed_inplace "s/a/b/" f.txt' \
  'epoch="$(bubbles_iso_to_epoch "$ts")"' \
  'mt="$(bubbles_file_mtime_epoch f.txt)"' \
  'now="$(bubbles_now_ms)"' \
  'grep -E "[0-9]+" f.txt' \
  'if [[ -n "${VAR+set}" ]]; then :; fi' \
  'if ((BASH_VERSINFO[0] >= 4)); then mapfile -t a < f.txt; fi' \
  'df -P / | awk "NR==2 {print}"' \
  'printf "a b" | paste -sd " " -' \
  'true' \
  'false' \
  'command -v timeout >/dev/null 2>&1 || true' \
  'timeout 5 sleep 1  # portable-ok: docker-internal entrypoint' \
  '# portable-ok: legacy migration path (line-above pragma)' \
  'sed -i "s/x/y/" legacy.txt' \
  '# NOTE historical: this module once used timeout and sed -i and grep -P'

run_guard "$green"
if [[ "$GUARD_RC" -eq 0 ]]; then
  pass "GREEN fixture (portable forms + pragmas + guarded mapfile) -> exit 0"
else
  fail "GREEN fixture expected exit 0, got rc=$GUARD_RC"
  printf '%s\n' "$GUARD_OUT"
fi

# --- RED fixtures: one per detected class ---------------------------------

mk "$TMPDIR/c1.sh" 'timeout 5 sleep 1'
assert_red "class1-raw-timeout" "class-1 raw-timeout" "$TMPDIR/c1.sh"

mk "$TMPDIR/c2.sh" 'sed -i "s/a/b/" f.txt'
assert_red "class2-sed-i" "class-2 in-place-sed" "$TMPDIR/c2.sh"

mk "$TMPDIR/c3.sh" 'e="$(date -d "$ts" +%s)"'
assert_red "class3-date-d" "class-3 date-d-parse" "$TMPDIR/c3.sh"

mk "$TMPDIR/c4.sh" 'm="$(stat -c %Y /etc/hosts)"'
assert_red "class4-stat-c" "class-4 stat-c-mtime" "$TMPDIR/c4.sh"

mk "$TMPDIR/c5.sh" 'p="$(readlink -f "$HOME")"'
assert_red "class5-readlink-f" "class-5 readlink-f-absolutize" "$TMPDIR/c5.sh"

mk "$TMPDIR/c6.sh" 'grep -P "[0-9]+" /etc/hosts'
assert_red "class6-grep-P" "class-6 grep-pcre" "$TMPDIR/c6.sh"

mk "$TMPDIR/c7.sh" 'if [[ -v MYVAR ]]; then :; fi'
assert_red "class7-isset" "class-7 bracket-v-isset" "$TMPDIR/c7.sh"

mk "$TMPDIR/c8.sh" 'mapfile -t arr < <(printf "x\n")'
assert_red "class8-mapfile" "class-8 mapfile-readarray" "$TMPDIR/c8.sh"

mk "$TMPDIR/c9.sh" 'f="$(mktemp --suffix=.yaml)"'
assert_red "class9-mktemp-suffix" "class-9 mktemp-suffix" "$TMPDIR/c9.sh"

mk "$TMPDIR/c10.sh" 'df --output=pcent /'
assert_red "class10-df-output" "class-10 df-output" "$TMPDIR/c10.sh"

mk "$TMPDIR/c11.sh" '/bin/true'
assert_red "class11-bin-true" "class-11 bin-true-false" "$TMPDIR/c11.sh"

mk "$TMPDIR/c12.sh" 'printf "a\nb\n" | paste -sd " "'
assert_red "class12-paste" "class-12 paste-no-stdin-operand" "$TMPDIR/c12.sh"

mk "$TMPDIR/c13.sh" 'now="$(date +%s%N)"'
assert_red "class13-date-ns" "class-13 date-nanoseconds" "$TMPDIR/c13.sh"

# --- Directory-surface + PORTABILITY_SCAN_PATHS wiring --------------------

dir_surface="$TMPDIR/surface"
mkdir -p "$dir_surface/nested"
mk "$dir_surface/clean.sh" 'echo ok'
mk "$dir_surface/nested/dirty.sh" 'grep -P "x" f'
run_guard "$dir_surface"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s\n' "$GUARD_OUT" | grep -Fq "class-6 grep-pcre"; then
  pass "directory surface recurses into *.sh (nested dirty file caught)"
else
  fail "directory surface expected exit 1 + class-6, got rc=$GUARD_RC"
  printf '%s\n' "$GUARD_OUT"
fi

set +e
env_out="$(PORTABILITY_SCAN_PATHS="$TMPDIR/c1.sh" bash "$TARGET" 2>&1)"
env_rc=$?
set -e
if [[ "$env_rc" -eq 1 ]] && printf '%s\n' "$env_out" | grep -Fq "class-1 raw-timeout"; then
  pass "PORTABILITY_SCAN_PATHS env surface is honored"
else
  fail "PORTABILITY_SCAN_PATHS env surface expected exit 1 + class-1, got rc=$env_rc"
  printf '%s\n' "$env_out"
fi

# --- Usage errors ---------------------------------------------------------

set +e
bash "$TARGET" >/dev/null 2>&1
noarg_rc=$?
bash "$TARGET" "$TMPDIR/does-not-exist.sh" >/dev/null 2>&1
badpath_rc=$?
set -e
if [[ "$noarg_rc" -eq 2 ]]; then
  pass "no-surface invocation exits 2 (usage)"
else
  fail "no-surface invocation expected exit 2, got $noarg_rc"
fi
if [[ "$badpath_rc" -eq 2 ]]; then
  pass "missing-path invocation exits 2 (usage)"
else
  fail "missing-path invocation expected exit 2, got $badpath_rc"
fi

# --- Guard self-portability ----------------------------------------------

if bash -n "$TARGET" 2>/dev/null; then
  pass "guard parses (bash -n)"
else
  fail "guard failed bash -n parse"
fi

run_guard "$TARGET"
if [[ "$GUARD_RC" -eq 0 ]]; then
  pass "guard is self-portable (guard scans its own source -> exit 0)"
else
  fail "guard scanning its own source expected exit 0, got rc=$GUARD_RC"
  printf '%s\n' "$GUARD_OUT"
fi

# Comment-stripped literal grep for GNU-only forms (real spaces, not [[:space:]])
# so the guard's own [[:space:]] pattern strings never false-match. 'timeout' is
# intentionally omitted (it is a substring of the helper names).
code_only="$(grep -vE '^[[:space:]]*#' "$TARGET" || true)"
gnu_forms='sed -i|date -d|stat -c|readlink -f|grep -[a-zA-Z]*P|df --output|/bin/(true|false)|mktemp --suffix|date [+]%s%N|\[\[ -v '
gnu_hits="$(printf '%s\n' "$code_only" | grep -nE "$gnu_forms" 2>/dev/null || true)"
if [[ -z "$gnu_hits" ]]; then
  pass "guard source (comments stripped) has no literal GNU-only form"
else
  fail "guard source contains a literal GNU-only form:"
  printf '%s\n' "$gnu_hits"
fi

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest macos-portability-guard] OK — all assertions passed."
  exit 0
fi
echo "[selftest macos-portability-guard] FAIL — $failures assertion(s) failed." >&2
exit 1
