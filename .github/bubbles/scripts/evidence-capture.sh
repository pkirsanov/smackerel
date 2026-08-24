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
#   bash bubbles/scripts/evidence-capture.sh --diagnostic -- <command...>
#   bash bubbles/scripts/evidence-capture.sh --verify <sha256> -- <command...>
#
# --diagnostic is the escalation for the case where a human genuinely needs the
# whole transcript. It is deliberately NOT a second verbosity mode: it is
# per-invocation, it is still bounded by a hard ceiling, and it stamps the block
# so the escalation is visible to a reviewer rather than silent. See
# agents/bubbles_shared/operating-baseline.md -> "Verbosity Posture".
#
# Exit codes:
#   0 = command succeeded (or --verify matched)
#   1 = command failed (block still emitted; a failure is evidence too)
#   2 = usage error
#   3 = --verify mismatch: the command no longer produces the recorded output

set -uo pipefail

# Optional. Only used to lift failure-shaped lines out of the omitted region, so
# a bounded block never hides the line that explains the exit code. Absence
# degrades the block (no failure section), it does not break capture.
if [[ -r "${BASH_SOURCE[0]%/*}/guard-lib.sh" ]]; then
  # shellcheck source=/dev/null
  source "${BASH_SOURCE[0]%/*}/guard-lib.sh"
fi

LABEL=""
KEEP=20
VERIFY=""
DIAGNOSTIC=0
# Even the escalation has a ceiling. "Unbounded on request" is how a bounded
# default erodes back into a transcript paste.
DIAGNOSTIC_MAX_LINES=2000
die_usage() { printf 'evidence-capture: %s\n' "$1" >&2; sed -n '27,31p' "${BASH_SOURCE[0]}" >&2; exit 2; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --label) shift; LABEL="${1:-}" ;;
    --lines) shift; KEEP="${1:-20}" ;;
    --verify) shift; VERIFY="${1:-}" ;;
    --diagnostic) DIAGNOSTIC=1 ;;
    -h|--help) sed -n '2,40p' "${BASH_SOURCE[0]}"; exit 0 ;;
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

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-evidence-capture.XXXXXXXX")" || exit 2
tmp="$tmp_dir/output.log"
: > "$tmp" || exit 2
interrupted_rc=""
child_pid=""
child_group_needs_cleanup=false
child_group_exists() {
  [[ -n "$child_pid" ]] || return 1
  kill -0 -- "-$child_pid" 2>/dev/null || kill -0 "$child_pid" 2>/dev/null
}
signal_child_group() {
  local signal_name="$1"

  [[ -n "$child_pid" ]] || return 0
  kill -s "$signal_name" -- "-$child_pid" 2>/dev/null ||
    kill -s "$signal_name" "$child_pid" 2>/dev/null || true
}
# Invoked indirectly by the EXIT trap.
# shellcheck disable=SC2317
cleanup() {
  if [[ "$child_group_needs_cleanup" == "true" ]] && child_group_exists; then
    signal_child_group KILL
  fi
  rm -rf "$tmp_dir"
}
# Invoked indirectly by the INT/TERM traps.
# shellcheck disable=SC2317
forward_signal() {
  local exit_code="$1"
  local signal_name="$2"

  interrupted_rc="$exit_code"
  child_group_needs_cleanup=true
  signal_child_group "$signal_name"
}
trap cleanup EXIT
trap 'forward_signal 130 INT' INT
trap 'forward_signal 143 TERM' TERM

# Interleave stdout and stderr: a runner's failure detail usually arrives on
# stderr, and evidence that drops it is evidence of the wrong thing.
# Job control gives the background command its own process group. The signal
# traps can therefore stop the complete validator tree instead of killing only
# this wrapper and leaving a lock-holding grandchild behind.
monitor_was_enabled=0
[[ "$-" == *m* ]] && monitor_was_enabled=1
set -m
BUBBLES_EVIDENCE_CAPTURE_OUTPUT_PATH="$tmp" "$@" >"$tmp" 2>&1 &
child_pid=$!
[[ "$monitor_was_enabled" -eq 1 ]] || set +m

rc=0
while true; do
  wait "$child_pid"
  wait_rc=$?
  if ! kill -0 "$child_pid" 2>/dev/null; then
    rc="$wait_rc"
    break
  fi
done
# A direct child can exit while its descendants keep running (for example, a
# nested timeout can stop one shell while a lock-holding grandchild survives).
# Give the remaining process group a graceful shutdown opportunity; the EXIT
# trap supplies a KILL safety net after the evidence block has been emitted.
if child_group_exists; then
  child_group_needs_cleanup=true
  signal_child_group TERM
fi
if [[ -n "$interrupted_rc" ]]; then
  rc="$interrupted_rc"
fi

if [[ ! -f "$tmp" ]]; then
  printf 'evidence-capture: capture output disappeared during command execution: %s\n' "$tmp" >&2
  exit 2
fi

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
if [[ "$DIAGNOSTIC" -eq 1 ]]; then
  # Stamped, not silent: a reviewer can see that bounded retention was waived
  # here and ask whether it needed to be.
  printf 'escalation: diagnostic (bounded retention waived for this invocation)\n'
  if [[ "$total" -gt "$DIAGNOSTIC_MAX_LINES" ]]; then
    printf -- '--- first %s of %s (diagnostic ceiling) ---\n' "$DIAGNOSTIC_MAX_LINES" "$total"
    head -n "$DIAGNOSTIC_MAX_LINES" "$tmp"
    printf -- '--- omitted %s line(s) beyond the diagnostic ceiling; sha256 above covers the full output ---\n' \
      "$((total - DIAGNOSTIC_MAX_LINES))"
  else
    printf -- '--- output ---\n'
    cat "$tmp"
  fi
elif [[ "$total" -le $((KEEP * 2)) ]]; then
  printf -- '--- output ---\n'
  cat "$tmp"
else
  printf -- '--- first %s ---\n' "$KEEP"
  head -n "$KEEP" "$tmp"
  # The omitted region is where a failure line hides. Lifting those lines out is
  # what makes the bounded block safe to prefer over the transcript: the reader
  # still sees why the exit code is what it is.
  if declare -F bubbles_ci_failure_detail >/dev/null 2>&1; then
    mid="$(mktemp)" || mid=""
    if [[ -n "$mid" ]]; then
      awk -v a="$((KEEP + 1))" -v b="$((total - KEEP))" 'NR>=a && NR<=b' <"$tmp" >"$mid"
      fail_lines="$(bubbles_ci_failure_detail "$mid")"
      rm -f "$mid"
      if [[ -n "$fail_lines" ]]; then
        printf -- '--- failure-shaped lines from the omitted region ---\n'
        printf '%s\n' "$fail_lines"
      fi
    fi
  fi
  printf -- '--- omitted %s line(s); sha256 above covers the full output ---\n' "$((total - KEEP * 2))"
  printf -- '--- last %s ---\n' "$KEEP"
  tail -n "$KEEP" "$tmp"
fi
printf '```\n'
printf '<!-- verify: bash bubbles/scripts/evidence-capture.sh --verify %s -- %s -->\n' "$digest" "$*"

exit "$rc"
