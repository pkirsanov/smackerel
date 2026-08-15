#!/usr/bin/env bash
set -euo pipefail

# output-policy-coherence-guard.sh
#
# IMP-039 SCOPE-1 — refuse the output-policy contradiction.
#
# WHY THIS EXISTS
# The framework shipped two rules about command output that disagreed, and the
# always-on one mandated the expensive branch:
#
#   - agents/bubbles_shared/evidence-rules.md mandates BOUNDED capture above 40
#     lines and windowed reads above 100 lines.
#   - templates/terminal-discipline.instructions.md.tmpl, which every downstream
#     repo installs with `applyTo: "**"`, mandated displaying the whole
#     unfiltered output of every command.
#
# Every agent therefore paid the expensive branch on every request while a
# cheaper and STRONGER form was already shipped in evidence-capture.sh. The
# measurement behind the change: one live session carried 1,295,690 tool-result
# tokens, 49.7% of 2,606,430 prompt tokens, for 8,931 completion tokens.
#
# This guard exists so the contradiction cannot silently return. It is the same
# regression-shape argument evidence-capture-selftest.sh already makes for
# --verify: the mechanism is only trustworthy if reintroducing the defect fails.
#
# WHAT IT CHECKS
#   A. ANCHOR      — evidence-rules.md still mandates bounded capture. Without
#                    this the guard would be a tautology: delete the anchor and
#                    every later check has nothing to contradict.
#   B. COHERENCE   — no scanned instruction surface reasserts an unbounded
#                    display mandate.
#   C. REACHABLE   — the terminal-discipline surfaces name the replacement
#                    mechanism, so the bounded path exists where the rule is
#                    stated rather than only in a skill the agent may not load.
#
# Exit codes:
#   0  coherent
#   1  finding (contradiction, missing anchor, or unnamed replacement)
#   2  usage error / no scannable surface found
#
# Usage:
#   bash bubbles/scripts/output-policy-coherence-guard.sh [--root DIR] [--quiet]
#
# --root defaults to the repository root inferred from this script's location.
# The selftest points it at hermetic fixtures.

ROOT=""
QUIET=0

die_usage() {
  printf 'output-policy-coherence-guard: %s\n' "$1" >&2
  printf 'usage: output-policy-coherence-guard.sh [--root DIR] [--quiet]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root) shift; ROOT="${1:-}" ;;
    --quiet) QUIET=1 ;;
    -h|--help) sed -n '4,45p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported; fix the contradiction instead" ;;
    *) die_usage "unknown argument '$1'" ;;
  esac
  shift
done

if [[ -z "$ROOT" ]]; then
  ROOT="$(cd "${BASH_SOURCE[0]%/*}/../.." && pwd)"
fi
[[ -d "$ROOT" ]] || die_usage "root '$ROOT' is not a directory"

ANCHOR_FILE="$ROOT/agents/bubbles_shared/evidence-rules.md"

# A bounded-capture mandate in the anchor. Both halves are required: naming the
# mechanism without a threshold leaves the default ambiguous, and a threshold
# without the mechanism leaves an agent with a bound it cannot act on.
ANCHOR_RE='evidence-capture'
ANCHOR_THRESHOLD_RE='40[[:space:]]+lines'

# An unbounded display mandate. POSIX ERE only (BSD grep must accept it).
# The quantifier can precede or follow the phrase, so both orders are matched:
# a one-directional pattern let "untruncated output for every command" through.
UNBOUNDED_RE='full[[:space:]]+unfiltered[[:space:]]+output'
UNBOUNDED_RE="$UNBOUNDED_RE|display[[:space:]]+(the[[:space:]]+)?(entire|complete|whole|full)[[:space:]]+(unfiltered[[:space:]]+)?(output|transcript)"
UNBOUNDED_RE="$UNBOUNDED_RE|(always|every[[:space:]]+command|all[[:space:]]+commands)[^.]*(unfiltered|untruncated)[^.]*output"
UNBOUNDED_RE="$UNBOUNDED_RE|(unfiltered|untruncated)[[:space:]]+output[^.]*(always|every[[:space:]]+command|all[[:space:]]+commands)"

findings=0
emit() { printf '%s\n' "$1" >&2; }

# --- Scan surface -----------------------------------------------------------
# Instruction files are the only always-on surface, which is exactly why a rule
# stated here is paid on every request and must be bounded.
surfaces=()
while IFS= read -r f; do
  [[ -n "$f" ]] && surfaces+=("$f")
done < <(
  {
    [[ -f "$ROOT/templates/terminal-discipline.instructions.md.tmpl" ]] &&
      printf '%s\n' "$ROOT/templates/terminal-discipline.instructions.md.tmpl"
    find "$ROOT/.github/instructions" "$ROOT/instructions" \
      -maxdepth 1 -name '*.instructions.md' -type f 2>/dev/null
  } | LC_ALL=C sort -u
)

if [[ ${#surfaces[@]} -eq 0 ]]; then
  die_usage "no instruction surface found under '$ROOT'"
fi

# --- Check A: the anchor still mandates bounded capture ---------------------
if [[ ! -f "$ANCHOR_FILE" ]]; then
  emit "FAIL [ANCHOR]: agents/bubbles_shared/evidence-rules.md is missing."
  emit "  Without the bounded-capture mandate this guard cannot detect a contradiction."
  findings=$((findings + 1))
else
  anchor_missing=""
  grep -qiE "$ANCHOR_RE" "$ANCHOR_FILE" || anchor_missing="the evidence-capture mechanism"
  grep -qiE "$ANCHOR_THRESHOLD_RE" "$ANCHOR_FILE" ||
    anchor_missing="${anchor_missing:+$anchor_missing and }the line threshold"
  if [[ -n "$anchor_missing" ]]; then
    emit "FAIL [ANCHOR]: evidence-rules.md no longer states the bounded-capture default."
    emit "  File: agents/bubbles_shared/evidence-rules.md"
    emit "  Missing: $anchor_missing"
    emit "  Restore it, or this guard silently passes while the expensive default returns."
    findings=$((findings + 1))
  fi
fi

# --- Check B: no surface reasserts an unbounded display mandate -------------
for f in "${surfaces[@]}"; do
  while IFS= read -r hit; do
    [[ -z "$hit" ]] && continue
    emit "FAIL [COHERENCE]: unbounded output-display mandate in an always-on instruction."
    emit "  File: ${f#"$ROOT"/}"
    emit "  Line: $hit"
    emit "  evidence-rules.md mandates bounded capture above 40 lines. An always-on"
    emit "  instruction that mandates displaying everything contradicts it and is paid"
    emit "  on every request. State what the command must PRODUCE separately from what"
    emit "  re-enters context, and point at evidence-capture.sh for the bounded form."
    findings=$((findings + 1))
  done < <(grep -niE "$UNBOUNDED_RE" "$f" 2>/dev/null | cut -c1-200 || true)
done

# --- Check C: the replacement mechanism is named where the rule is stated ---
for f in "${surfaces[@]}"; do
  case "${f##*/}" in
    terminal-discipline.instructions.md|terminal-discipline.instructions.md.tmpl) ;;
    *) continue ;;
  esac
  if ! grep -qF 'evidence-capture.sh' "$f"; then
    emit "FAIL [REACHABLE]: terminal-discipline states the output rule without naming the bounded form."
    emit "  File: ${f#"$ROOT"/}"
    emit "  Add the evidence-capture.sh invocation next to the rule. A bound with no"
    emit "  named mechanism is guidance an agent cannot act on."
    findings=$((findings + 1))
  fi
done

if [[ "$findings" -gt 0 ]]; then
  emit ""
  emit "output-policy-coherence-guard: $findings finding(s)."
  exit 1
fi

[[ "$QUIET" -eq 1 ]] || printf '[output-policy-coherence-guard] OK — %s surface(s) coherent with evidence-rules.md\n' "${#surfaces[@]}"
exit 0
