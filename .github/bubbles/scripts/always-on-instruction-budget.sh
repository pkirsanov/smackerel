#!/usr/bin/env bash
set -euo pipefail

# always-on-instruction-budget.sh
#
# IMP-039 SCOPE-6 — bound the always-on instruction surface.
#
# WHY THIS EXISTS
# An instruction with `applyTo: "**"` is re-sent on EVERY request in EVERY
# downstream repository, whatever the request touches. Nothing measured that
# surface, so it regrew silently: IMP-036 SCOPE-5 cut always-on context from 850
# to 545 lines and recorded that for one file "only the applyTo header was the
# cost" — and two years' worth of the largest shipped instructions still carried
# that header when IMP-039 measured them (15,754 B + 4,933 B).
#
# A reduction with no ceiling is a reduction that comes back. This guard is the
# ceiling.
#
# WHAT IT CHECKS
#   A. ALLOWLIST — only the files that are deliberately universal may carry
#                  `applyTo: "**"`. This catches the actual regression shape:
#                  someone widens a narrowed file's header back to "**".
#   B. CEILING   — the total always-on byte count stays under budget. This
#                  catches the other shape: the kernel absorbing everything the
#                  narrowed files used to hold.
#
# SCOPE: framework-shipped instructions only. A downstream repository's own
# always-on instructions are its own governance call and are never measured
# here — which is why framework-validate runs this self-only.
#
# Exit codes:
#   0  within budget
#   1  finding (unexpected always-on file, or over the ceiling)
#   2  usage error / no instruction surface found

# Files permitted to be always-on, and why.
#
#   bubbles-kernel.instructions.md
#     The universal kernel: repository authority, anti-fabrication, evidence
#     integrity, timeouts. These must hold on a request that touches no agent
#     file, no test and no config, so they cannot be conditioned on a glob.
#
# This list is deliberately ONE entry. Every other framework instruction governs
# an identifiable surface and is narrowed to it. Adding an entry here means
# asserting that a rule cannot be conditioned on any glob — which is rare, and
# should be argued rather than assumed.
ALLOWED_ALWAYS_ON=(
  "bubbles-kernel.instructions.md"
)

# Measured at IMP-039 SCOPE-6: the kernel alone, 4,383 B. Headroom is deliberate
# but bounded. Re-widening either narrowed instruction breaches it — the smaller
# of the two, env-pollution at 4,933 B, would already reach 9,316 B — which is
# exactly the regression this ceiling exists to refuse.
CEILING_BYTES=8000

ROOT=""
QUIET=0

die_usage() {
  printf 'always-on-instruction-budget: %s\n' "$1" >&2
  printf 'usage: always-on-instruction-budget.sh [--root DIR] [--quiet]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root) shift; ROOT="${1:-}" ;;
    --quiet) QUIET=1 ;;
    -h|--help) sed -n '4,32p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported; narrow the instruction instead" ;;
    *) die_usage "unknown argument '$1'" ;;
  esac
  shift
done

[[ -n "$ROOT" ]] || ROOT="$(cd "${BASH_SOURCE[0]%/*}/../.." && pwd)"
[[ -d "$ROOT/instructions" ]] || die_usage "no instructions/ directory under '$ROOT'"

is_allowed() {
  local name="$1" a
  for a in "${ALLOWED_ALWAYS_ON[@]}"; do
    [[ "$name" == "$a" ]] && return 0
  done
  return 1
}

findings=0
total=0
listed=""
emit() { printf '%s\n' "$1" >&2; }

while IFS= read -r f; do
  [[ -n "$f" ]] || continue
  # Only the frontmatter header counts. A "**" inside prose is not a directive.
  grep -qE '^applyTo:[[:space:]]*"\*\*"[[:space:]]*$' "$f" || continue

  name="${f##*/}"
  bytes="$(wc -c <"$f" | tr -d '[:space:]')"
  total=$((total + bytes))
  listed="${listed}    ${name} (${bytes} B)"$'\n'

  if ! is_allowed "$name"; then
    emit "FAIL [ALLOWLIST]: '$name' is always-on but not on the allowlist."
    emit "  File: instructions/$name ($bytes B)"
    emit "  applyTo: \"**\" is paid on every request in every downstream repo."
    emit "  Either narrow it to the surfaces its rules govern, or add it to"
    emit "  ALLOWED_ALWAYS_ON with the reason it cannot be conditioned on a glob."
    findings=$((findings + 1))
  fi
done < <(find "$ROOT/instructions" -maxdepth 1 -name '*.instructions.md' -type f | LC_ALL=C sort)

if [[ "$total" -eq 0 ]]; then
  emit "FAIL [ALLOWLIST]: no always-on instruction found."
  emit "  The universal kernel carries invariants that must not become conditional."
  emit "  A surface measuring zero means the kernel lost its applyTo header."
  findings=$((findings + 1))
elif [[ "$total" -gt "$CEILING_BYTES" ]]; then
  emit "FAIL [CEILING]: always-on instruction surface is ${total} B, over the ${CEILING_BYTES} B budget."
  emit "$listed"
  emit "  Every byte here is re-sent on every request. Move whatever can be"
  emit "  conditioned on a file surface into a narrowed instruction."
  findings=$((findings + 1))
fi

if [[ "$findings" -gt 0 ]]; then
  emit ""
  emit "always-on-instruction-budget: $findings finding(s)."
  exit 1
fi

[[ "$QUIET" -eq 1 ]] || printf '[always-on-instruction-budget] OK — %s B always-on, budget %s B\n' "$total" "$CEILING_BYTES"
exit 0
