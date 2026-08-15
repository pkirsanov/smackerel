#!/usr/bin/env bash
# bubbles/scripts/output-policy-coherence-guard-selftest.sh
#
# Hermetic selftest for output-policy-coherence-guard.sh (IMP-039 SCOPE-1).
#
# The load-bearing property is the adversarial half: cases A1-A3 REINTRODUCE the
# exact contradiction the guard exists to refuse, and case A4-A6 remove the
# anchor the guard compares against. If any of those pass, the guard is a
# tautology and the bounded-retention default is unenforced.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/output-policy-coherence-guard.sh"
NAME="output-policy-coherence-guard-selftest"

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

# Builds a fixture repo root. Every part is the COHERENT shape; individual cases
# then damage exactly one thing, so a failure names one cause.
make_root() {
  local root="$1"
  mkdir -p "$root/agents/bubbles_shared" "$root/.github/instructions" "$root/instructions" "$root/templates"

  cat >"$root/agents/bubbles_shared/evidence-rules.md" <<'ANCHOR'
# Evidence Rules
- Bounded capture is the default evidence shape above 40 lines; produce it with
  `bubbles/scripts/evidence-capture.sh` rather than pasting a transcript.
ANCHOR

  cat >"$root/.github/instructions/terminal-discipline.instructions.md" <<'TD'
---
applyTo: "**"
---
## 2. Never Discard Output; Bound What Re-Enters Context
Run the command unfiltered so every line is produced and captured.
Above 40 lines, route it through `bash bubbles/scripts/evidence-capture.sh`.
TD

  cat >"$root/templates/terminal-discipline.instructions.md.tmpl" <<'TDT'
---
applyTo: "**"
---
Run unfiltered; above 40 lines record via `evidence-capture.sh`.
TDT
}

run_guard() {
  set +e
  GUARD_OUT="$(bash "$TARGET" --root "$1" --quiet 2>&1)"
  GUARD_RC=$?
  set -e
}

# --- P1. the coherent shape passes -------------------------------------------
R="$WORK/p1"; make_root "$R"
run_guard "$R"
if [[ "$GUARD_RC" -eq 0 ]]; then
  ok "P1 coherent surfaces pass"
else
  bad "P1 coherent surfaces pass" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: the exact retired mandate is refused --------------------
R="$WORK/a1"; make_root "$R"
printf '**REQUIRED:** Always capture and display the FULL unfiltered output.\n' \
  >>"$R/.github/instructions/terminal-discipline.instructions.md"
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'COHERENCE'; then
  ok "A1 reintroducing 'FULL unfiltered output' is refused"
else
  bad "A1 reintroducing 'FULL unfiltered output' is refused" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: a paraphrase is refused too -----------------------------
# Wording drift is the realistic reintroduction path; matching one literal string
# would let the defect back in under a synonym.
R="$WORK/a2"; make_root "$R"
printf 'Agents MUST display the entire transcript for every check.\n' \
  >>"$R/instructions/bubbles-example.instructions.md"
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'COHERENCE'; then
  ok "A2 paraphrased unbounded mandate is refused"
else
  bad "A2 paraphrased unbounded mandate is refused" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: universal-quantifier phrasing is refused ----------------
R="$WORK/a3"; make_root "$R"
printf 'Capture untruncated output for every command in the run and keep it.\n' \
  >>"$R/templates/terminal-discipline.instructions.md.tmpl"
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'COHERENCE'; then
  ok "A3 'every command ... untruncated output' is refused"
else
  bad "A3 'every command ... untruncated output' is refused" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: deleting the mechanism from the anchor is refused -------
R="$WORK/a4"; make_root "$R"
cat >"$R/agents/bubbles_shared/evidence-rules.md" <<'ANCHOR'
# Evidence Rules
- Capture only the relevant window above 40 lines.
ANCHOR
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'ANCHOR'; then
  ok "A4 anchor without the capture mechanism is refused"
else
  bad "A4 anchor without the capture mechanism is refused" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: deleting the threshold from the anchor is refused -------
R="$WORK/a5"; make_root "$R"
cat >"$R/agents/bubbles_shared/evidence-rules.md" <<'ANCHOR'
# Evidence Rules
- Use `bubbles/scripts/evidence-capture.sh` when it seems appropriate.
ANCHOR
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'ANCHOR'; then
  ok "A5 anchor without the line threshold is refused"
else
  bad "A5 anchor without the line threshold is refused" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL: removing the anchor file entirely is refused ------------
R="$WORK/a6"; make_root "$R"
rm -f "$R/agents/bubbles_shared/evidence-rules.md"
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'ANCHOR'; then
  ok "A6 missing anchor file is refused, not silently skipped"
else
  bad "A6 missing anchor file is refused, not silently skipped" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- A7. ADVERSARIAL: a bound with no named mechanism is refused --------------
R="$WORK/a7"; make_root "$R"
cat >"$R/.github/instructions/terminal-discipline.instructions.md" <<'TD'
---
applyTo: "**"
---
Keep output bounded above 40 lines.
TD
run_guard "$R"
if [[ "$GUARD_RC" -eq 1 ]] && printf '%s' "$GUARD_OUT" | grep -q 'REACHABLE'; then
  ok "A7 terminal-discipline without the named mechanism is refused"
else
  bad "A7 terminal-discipline without the named mechanism is refused" "rc=$GUARD_RC out=$(printf '%s' "$GUARD_OUT" | tr '\n' '|')"
fi

# --- U1. a root with no instruction surface is a usage error, not a pass ------
R="$WORK/u1"; mkdir -p "$R"
run_guard "$R"
if [[ "$GUARD_RC" -eq 2 ]]; then
  ok "U1 empty root exits 2 rather than reporting coherent"
else
  bad "U1 empty root exits 2 rather than reporting coherent" "rc=$GUARD_RC"
fi

# --- U2. no bypass flag exists -----------------------------------------------
set +e
bypass_out="$(bash "$TARGET" --skip-coherence 2>&1)"; bypass_rc=$?
set -e
if [[ "$bypass_rc" -eq 2 ]] && printf '%s' "$bypass_out" | grep -q 'bypass-shaped'; then
  ok "U2 a bypass-shaped flag is rejected"
else
  bad "U2 a bypass-shaped flag is rejected" "rc=$bypass_rc out=$(printf '%s' "$bypass_out" | tr '\n' '|')"
fi

# --- U3. a nonexistent root is a usage error ---------------------------------
set +e
bash "$TARGET" --root "$WORK/does-not-exist" >/dev/null 2>&1; missing_rc=$?
set -e
if [[ "$missing_rc" -eq 2 ]]; then
  ok "U3 nonexistent root exits 2"
else
  bad "U3 nonexistent root exits 2" "rc=$missing_rc"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
