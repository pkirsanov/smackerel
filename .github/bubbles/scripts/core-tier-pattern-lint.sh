#!/usr/bin/env bash
# bubbles/scripts/core-tier-pattern-lint.sh
#
# Capability: validation-tier-integrity
#
# Fail when a core-tier pattern matches NO scheduled check (IMP-042 SCOPE-2).
#
# WHY THIS EXISTS
# `core_check_label()` in framework-validate.sh decides the core tier by SUBSTRING
# match against check LABELS. Pre-push runs `--tier=core`, so that function is the
# list of checks allowed to block a push.
#
# A substring list keyed on prose has one failure mode and it is silent: rename a
# check and its pattern stops matching. Nothing errors. The tier simply runs one
# fewer check, and a guard that was chosen specifically to block pushes stops
# blocking them while every run still reports success. That is the same shape as
# the unwired-selftest hole COV-2 covered, moved one layer up.
#
# This lint makes that shape loud. It is a stopgap with a stated expiry: SCOPE-2
# replaces label-substring tiering with a typed registry keyed on a stable
# checkId, at which point this lint has nothing left to guard and should go.
#
# Usage:
#   bash bubbles/scripts/core-tier-pattern-lint.sh [--validator PATH]
#
# Exit codes:
#   0 = every core pattern matches at least one scheduled check
#   1 = at least one dead pattern
#   2 = usage error, or the validator could not be read

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALIDATOR="$SCRIPT_DIR/framework-validate.sh"
NAME="core-tier-pattern-lint"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --validator)
      shift
      VALIDATOR="${1:-}"
      ;;
    -h | --help)
      printf 'usage: %s.sh [--validator PATH]\n' "$NAME"
      printf 'Fails when a core-tier pattern matches no scheduled check label.\n'
      exit 0
      ;;
    *)
      printf '%s: unsupported flag "%s". This lint has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
  esac
  shift || true
done

[[ -f "$VALIDATOR" ]] || {
  printf '%s: validator not found: %s\n' "$NAME" "$VALIDATOR" >&2
  exit 2
}

validator_text="$(cat "$VALIDATOR")"

# The quoted needles inside core_check_label()'s case arms, one per line. Only
# the `*"needle"*` shape counts, so the `case "$1" in` line is not mistaken for
# a pattern.
core_patterns="$(printf '%s\n' "$validator_text" | awk '
  /^core_check_label\(\)/ { inside = 1; next }
  inside && /return 0/    { exit }
  inside {
    line = $0
    while (match(line, /\*"[^"]+"\*/)) {
      arm = substr(line, RSTART, RLENGTH)
      print substr(arm, 3, length(arm) - 4)
      line = substr(line, RSTART + RLENGTH)
    }
  }
')"

if [[ -z "$core_patterns" ]]; then
  printf '%s: parsed ZERO core patterns from %s.\n' "$NAME" "$VALIDATOR" >&2
  printf '%s: an empty core tier would let every push through unchecked.\n' "$NAME" >&2
  exit 1
fi

# Every label passed to run_check / run_check_self_only. Backslash-continued
# invocations are reassembled first, for the same reason the scheduled-selftest
# reader does it: a line-at-a-time pass misses the continued form.
scheduled_labels="$(printf '%s\n' "$validator_text" | awk '
  {
    line = $0
    sub(/^[[:space:]]+/, "", line)
    if (pending != "") { line = pending " " line; pending = "" }
    if (line ~ /\\$/) { sub(/\\$/, "", line); pending = line; next }
    if (line !~ /^run_check(_self_only)?[[:space:]]+"/) next
    if (match(line, /"[^"]*"/)) {
      print substr(line, RSTART + 1, RLENGTH - 2)
    }
  }
')"

if [[ -z "$scheduled_labels" ]]; then
  printf '%s: parsed ZERO scheduled check labels from %s.\n' "$NAME" "$VALIDATOR" >&2
  exit 1
fi

label_count="$(printf '%s\n' "$scheduled_labels" | grep -c .)"
pattern_count="$(printf '%s\n' "$core_patterns" | grep -c .)"
dead=0

while IFS= read -r needle; do
  [[ -n "$needle" ]] || continue
  if ! printf '%s\n' "$scheduled_labels" | grep -qF -- "$needle"; then
    printf '%s: DEAD core pattern: "%s"\n' "$NAME" "$needle" >&2
    printf '  It matches no scheduled check, so the core tier silently lost it.\n' >&2
    printf '  Either a check was renamed, or the pattern outlived its check.\n' >&2
    dead=$((dead + 1))
  fi
done <<<"$core_patterns"

if [[ "$dead" -gt 0 ]]; then
  printf '\n%s: %d dead pattern(s) of %d, against %d scheduled checks.\n' \
    "$NAME" "$dead" "$pattern_count" "$label_count" >&2
  printf '%s: pre-push runs --tier=core, so a dead pattern is a guard that stopped blocking.\n' "$NAME" >&2
  exit 1
fi

printf '[%s] OK - all %d core pattern(s) match a scheduled check (%d checks scanned).\n' \
  "$NAME" "$pattern_count" "$label_count"
exit 0
