#!/usr/bin/env bash
# bubbles/scripts/always-on-instruction-budget-selftest.sh
#
# Hermetic selftest for always-on-instruction-budget.sh (IMP-039 SCOPE-6).
#
# The load-bearing half is adversarial: A1 re-widens a narrowed instruction back
# to applyTo "**", which is the exact regression IMP-036 SCOPE-5 suffered. If
# that case passes, the narrowing is unenforced and will come back.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/always-on-instruction-budget.sh"
NAME="always-on-instruction-budget-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

# $1 root, $2 filename, $3 applyTo value, $4 filler byte count
write_instruction() {
  local root="$1" name="$2" apply="$3" fill="${4:-200}"
  mkdir -p "$root/instructions"
  {
    printf -- '---\n'
    printf 'applyTo: %s\n' "$apply"
    printf -- '---\n\n'
    # Deterministic filler so byte-budget cases are exact rather than incidental.
    awk -v n="$fill" 'BEGIN { for (i = 0; i < n; i++) printf "x" }'
    printf '\n'
  } >"$root/instructions/$name"
}

make_root() {
  local root="$1"
  write_instruction "$root" "bubbles-kernel.instructions.md" '"**"' 1000
  write_instruction "$root" "bubbles-wsl-macos-compatibility.instructions.md" '"**/*.sh"' 1000
  write_instruction "$root" "bubbles-agents.instructions.md" '"**/*.agent.md"' 9000
}

run_guard() {
  set +e
  OUT="$(bash "$TARGET" --root "$1" --quiet 2>&1)"
  RC=$?
  set -e
}

# --- P1. the narrowed shape passes -------------------------------------------
R="$WORK/p1"; make_root "$R"
run_guard "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P1 narrowed surface is within budget"
else
  bad "P1 narrowed surface is within budget" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: re-widening a narrowed instruction is refused ----------
R="$WORK/a1"; make_root "$R"
write_instruction "$R" "bubbles-agents.instructions.md" '"**"' 9000
run_guard "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ALLOWLIST'; then
  ok "A1 re-widening a narrowed instruction to '**' is refused"
else
  bad "A1 re-widening a narrowed instruction to '**' is refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: an allowlisted file growing past the ceiling is refused -
# Catches the other regression shape: the kernel absorbing what narrowing removed.
R="$WORK/a2"; make_root "$R"
write_instruction "$R" "bubbles-kernel.instructions.md" '"**"' 20000
run_guard "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'CEILING'; then
  ok "A2 allowlisted file growing past the ceiling is refused"
else
  bad "A2 allowlisted file growing past the ceiling is refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: losing the kernel entirely is refused ------------------
# A guard that reports "0 bytes always-on" as its best possible score would bless
# deleting the invariants.
R="$WORK/a3"; make_root "$R"
write_instruction "$R" "bubbles-kernel.instructions.md" '"**/*.agent.md"' 1000
run_guard "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ALLOWLIST'; then
  ok "A3 an empty always-on surface is refused, not treated as ideal"
else
  bad "A3 an empty always-on surface is refused, not treated as ideal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: a second file quietly going always-on is refused -------
# The realistic drift is not a rewrite but one added header on a file that was
# correctly narrowed.
R="$WORK/a4"; make_root "$R"
write_instruction "$R" "bubbles-wsl-macos-compatibility.instructions.md" '"**"' 1000
run_guard "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ALLOWLIST'; then
  ok "A4 a narrowed file re-acquiring 'applyTo: \"**\"' is refused"
else
  bad "A4 a narrowed file re-acquiring 'applyTo: \"**\"' is refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- N1. a "**" inside prose is not a directive ------------------------------
# Guards A1 against over-matching: the header is what costs, not the glob text.
R="$WORK/n1"; make_root "$R"
printf '\nUse applyTo: "**" only for true invariants.\n' \
  >>"$R/instructions/bubbles-agents.instructions.md"
run_guard "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "N1 a '**' mentioned in prose does not count as always-on"
else
  bad "N1 a '**' mentioned in prose does not count as always-on" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- U1. no instructions dir is a usage error --------------------------------
set +e
bash "$TARGET" --root "$WORK/nope" >/dev/null 2>&1; rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "U1 missing instructions/ exits 2"
else
  bad "U1 missing instructions/ exits 2" "rc=$rc"
fi

# --- U2. no bypass flag exists -----------------------------------------------
set +e
bypass="$(bash "$TARGET" --force 2>&1)"; rc=$?
set -e
if [[ "$rc" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U2 a bypass-shaped flag is rejected"
else
  bad "U2 a bypass-shaped flag is rejected" "rc=$rc out=$(printf '%s' "$bypass" | tr '\n' '|')"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
