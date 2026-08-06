#!/usr/bin/env bash
# bubbles/scripts/evidence-capture-selftest.sh
#
# Hermetic selftest for evidence-capture.sh (IMP-036 SCOPE-6).
#
# The load-bearing property is case 5: --verify must FAIL when the command's
# output changes. If a recorded hash cannot detect drift, the compact form is
# weaker than the transcript it replaces and must not ship.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/evidence-capture.sh"
NAME="evidence-capture-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"; [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

# --- 1. records command, exit code and hash ----------------------------------
out="$(bash "$TARGET" --label "demo" -- printf 'a\nb\nc\n' 2>&1)"
if printf '%s' "$out" | grep -q '^exit: 0$' &&
  printf '%s' "$out" | grep -q '^lines: 3$' &&
  printf '%s' "$out" | grep -qE '^sha256: [0-9a-f]{64}$'; then
  ok "records command, exit code, line count and a sha256"
else
  bad "records the basics" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 2. short output is shown in full ----------------------------------------
if printf '%s' "$out" | grep -q -- '--- output ---' &&
  printf '%s' "$out" | grep -qx 'b'; then
  ok "short output is emitted in full, not truncated"
else
  bad "short output shown in full" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 3. long output is head/tail trimmed with an explicit omission note -------
long="$(bash "$TARGET" --lines 2 -- seq 1 50 2>&1)"
if printf '%s' "$long" | grep -q -- '--- first 2 ---' &&
  printf '%s' "$long" | grep -q 'omitted 46 line(s)' &&
  printf '%s' "$long" | grep -q -- '--- last 2 ---'; then
  ok "long output is trimmed and states how many lines were omitted"
else
  bad "long output trimmed" "$(printf '%s' "$long" | tr '\n' '|')"
fi

# --- 4. a FAILING command still produces evidence and propagates its code -----
set +e
fail_out="$(bash "$TARGET" -- sh -c 'echo boom; exit 7' 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 7 ]] && printf '%s' "$fail_out" | grep -q '^exit: 7$' &&
  printf '%s' "$fail_out" | grep -qx 'boom'; then
  ok "failing command still emits evidence and propagates exit 7"
else
  bad "failing command evidence" "rc=$rc $(printf '%s' "$fail_out" | tr '\n' '|')"
fi

# --- 5. ADVERSARIAL: --verify must DETECT changed output ---------------------
# Without this, the hash is decoration and the compact form would be weaker than
# a transcript.
digest="$(bash "$TARGET" -- printf 'stable\n' 2>&1 | awk '/^sha256: /{print $2}')"
set +e
bash "$TARGET" --verify "$digest" -- printf 'stable\n' >/dev/null 2>&1
same_rc=$?
bash "$TARGET" --verify "$digest" -- printf 'CHANGED\n' >/dev/null 2>&1
diff_rc=$?
set -e
if [[ "$same_rc" -eq 0 && "$diff_rc" -eq 3 ]]; then
  ok "--verify passes on identical output and FAILS (3) when it changes"
else
  bad "--verify detects drift" "same=$same_rc changed=$diff_rc (want 0 and 3)"
fi

# --- 6. stderr is captured, not dropped --------------------------------------
err_out="$(bash "$TARGET" -- sh -c 'echo to-stderr >&2' 2>&1)"
if printf '%s' "$err_out" | grep -qx 'to-stderr'; then
  ok "stderr is interleaved into the evidence, not discarded"
else
  bad "stderr captured" "$(printf '%s' "$err_out" | tr '\n' '|')"
fi

# --- 7. bypass-shaped flags are refused --------------------------------------
set +e
bash "$TARGET" --fake -- true >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "bypass-shaped flag refused with exit 2"
else
  bad "bypass flag refused" "exit was $rc"
fi

# --- 8. a missing command is a usage error -----------------------------------
set +e
bash "$TARGET" --label x >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "no command after -- is a usage error"
else
  bad "missing command is usage error" "exit was $rc"
fi

# --- 9. emits a re-runnable verify hint --------------------------------------
if printf '%s' "$out" | grep -q '<!-- verify: bash bubbles/scripts/evidence-capture.sh --verify'; then
  ok "block carries a re-runnable verify command"
else
  bad "verify hint emitted" "$(printf '%s' "$out" | tr '\n' '|')"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
