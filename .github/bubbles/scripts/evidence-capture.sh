#!/usr/bin/env bash
# bubbles/scripts/evidence-capture.sh
#
# Runs a command and emits a compact, verifiable evidence block (IMP-036 SCOPE-6).
#
# WHY THIS EXISTS
# report.md is the single largest artifact in every consuming repo: 79,416 to
# 121,311 lines per repo per 60 days, and specs plus governance account for
# 60-71% of all changed lines against 25-35% for product code. The bulk is
# pasted terminal transcripts.
#
# The >=10-line raw-output rule was written to stop fabricated evidence, and the
# volume was treated as the price. It is not. A transcript proves only that text
# was pasted; it cannot be checked. A hash of the full output CAN be checked, by
# re-running the command and comparing. This form is therefore STRONGER against
# fabrication than the transcript it replaces, at a fraction of the bytes.
#
# It also removes a recurring failure class: pasted transcripts carry absolute
# paths, which trip the secret and PII scanners and have blocked commits.
#
# WHAT DOES NOT CHANGE
# Evidence must still come from real execution in the current session. That rule
# is not the cost and is not relaxed here. This tool exists precisely because it
# runs the command itself, so the recorded exit code and hash cannot be authored
# by hand.
#
# Usage:
#   bash bubbles/scripts/evidence-capture.sh [--label TEXT] [--lines N] -- <command...>
#   bash bubbles/scripts/evidence-capture.sh --verify <sha256> -- <command...>
#
# Exit codes:
#   0 = command succeeded (or --verify matched)
#   1 = command failed (block still emitted; a failure is evidence too)
#   2 = usage error
#   3 = --verify mismatch: the command no longer produces the recorded output

set -uo pipefail

LABEL=""
KEEP=20
VERIFY=""
die_usage() { printf 'evidence-capture: %s\n' "$1" >&2; sed -n '27,31p' "${BASH_SOURCE[0]}" >&2; exit 2; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --label) shift; LABEL="${1:-}" ;;
    --lines) shift; KEEP="${1:-20}" ;;
    --verify) shift; VERIFY="${1:-}" ;;
    -h|--help) sed -n '2,32p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--fake*)
      die_usage "bypass-shaped flag '$1' is not supported; evidence is produced by running the command" ;;
    --) shift; break ;;
    -*) die_usage "unknown flag '$1'" ;;
    *) die_usage "unexpected argument '$1' (put the command after --)" ;;
  esac
  shift
done

[[ $# -gt 0 ]] || die_usage "a command is required after --"
[[ "$KEEP" =~ ^[0-9]+$ ]] || die_usage "--lines must be a non-negative integer"

hash_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 | awk '{print $1}'
  else printf 'unavailable'
  fi
}

tmp="$(mktemp)" || exit 2
trap 'rm -f "$tmp"' EXIT INT TERM

# Interleave stdout and stderr: a runner's failure detail usually arrives on
# stderr, and evidence that drops it is evidence of the wrong thing.
"$@" >"$tmp" 2>&1
rc=$?

total="$(grep -c '' <"$tmp" 2>/dev/null || printf '0')"
digest="$(hash_of <"$tmp")"

if [[ -n "$VERIFY" ]]; then
  if [[ "$digest" == "$VERIFY" ]]; then
    printf '[evidence-capture] VERIFIED - output still hashes to %s\n' "$digest"
    exit 0
  fi
  printf '[evidence-capture] MISMATCH\n' >&2
  printf '  recorded: %s\n' "$VERIFY" >&2
  printf '  observed: %s\n' "$digest" >&2
  printf '  The command no longer produces the recorded output. Either the\n' >&2
  printf '  behaviour changed or the recorded evidence never came from this command.\n' >&2
  exit 3
fi

printf '```\n'
[[ -n "$LABEL" ]] && printf '# %s\n' "$LABEL"
printf '$ %s\n' "$*"
printf 'exit: %s\n' "$rc"
printf 'lines: %s\n' "$total"
printf 'sha256: %s\n' "$digest"
if [[ "$total" -le $((KEEP * 2)) ]]; then
  printf -- '--- output ---\n'
  cat "$tmp"
else
  printf -- '--- first %s ---\n' "$KEEP"
  head -n "$KEEP" "$tmp"
  printf -- '--- omitted %s line(s); sha256 above covers the full output ---\n' "$((total - KEEP * 2))"
  printf -- '--- last %s ---\n' "$KEEP"
  tail -n "$KEEP" "$tmp"
fi
printf '```\n'
printf '<!-- verify: bash bubbles/scripts/evidence-capture.sh --verify %s -- %s -->\n' "$digest" "$*"

exit "$rc"
