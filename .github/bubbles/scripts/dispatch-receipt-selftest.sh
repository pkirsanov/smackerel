#!/usr/bin/env bash
# dispatch-receipt-selftest.sh — IMP-048 SCOPE-2 (HO-4).
#
# Every assertion runs the SHIPPING script against a real fixture repository and
# reads the outcome back out of the JSONL store the script actually wrote. A
# mechanism that is only ever asked what it would do has never been shown to do
# it.
#
# Covered:
#   default off        an unconfigured repository is a clean no-op: zero
#                      records, no store file, exit 0
#   closed class set   each of the six failure classes plus the one success
#                      class is recorded with its own prescribed action
#   no false advance   the phase is refused for EVERY non-envelope class, one
#                      case per class, and permitted only for a valid envelope
#   retry identity     a transport retry reuses the SAME occurrence id and the
#                      SAME packet digest -- one occurrence, two attempts
#   escalation         a second identical transport failure becomes
#                      REPEAT_INFRASTRUCTURE and STOPS
#   per-class budgets  an infrastructure retry does not touch the
#                      defect-remediation budget, and vice versa
#   earned receipts    a receipt asserted without a dispatch -- absent packet,
#                      forged digest, self-declared escalation, envelope status
#                      that does not support the claim -- is REFUSED and writes
#                      nothing
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the script under test is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$SCRIPT_DIR/dispatch-receipt.sh"
NAME="dispatch-receipt-selftest"
STORE_REL=".specify/runtime/dispatch-receipts.jsonl"

passes=0
failures=0
pass() {
  passes=$((passes + 1))
  printf 'PASS: %s\n' "$1"
}
fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

[[ -f "$SUT" ]] || {
  printf '%s: script under test not found: %s\n' "$NAME" "$SUT" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-dispatch-receipt.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

# --- fixture helpers -------------------------------------------------------

repo_seq=0
REPO=""
# Sets the global REPO. NOT called through a command substitution: that would
# run in a subshell, repo_seq would never advance, and every "fresh" fixture
# would silently be the same directory carrying the previous test's records.
new_repo() {
  local adapter="${1:-jsonl}"
  repo_seq=$((repo_seq + 1))
  REPO="$TMP_DIR/repo$repo_seq"
  mkdir -p "$REPO/.github"
  printf 'packet for repo%s\n' "$repo_seq" > "$REPO/packet.txt"
  printf 'a different packet for repo%s\n' "$repo_seq" > "$REPO/other-packet.txt"
  if [[ "$adapter" != "unset" ]]; then
    printf 'dispatchReceipts:\n  adapter: %s\n' "$adapter" > "$REPO/.github/bubbles-project.yaml"
  fi
}

LAST_OUT=""
LAST_RC=0
record() {
  local repo="$1"
  shift
  LAST_OUT="$(bash "$SUT" record --repo-root "$repo" "$@" 2>&1)"
  LAST_RC=$?
}
status_of() {
  local repo="$1"
  shift
  LAST_OUT="$(bash "$SUT" status --repo-root "$repo" "$@" 2>&1)"
  LAST_RC=$?
}

# Read one key=value line out of the captured output. A herestring, never a
# discarding pipe: `... | grep -q` on unbounded input under pipefail is the
# BUG-009 SIGPIPE race.
kv() {
  awk -v k="$1" -F= '$1 == k { print substr($0, length(k) + 2); exit }' <<< "$LAST_OUT"
}

store_path() { printf '%s/%s' "$1" "$STORE_REL"; }

store_count() {
  local f
  f="$(store_path "$1")"
  if [[ -f "$f" ]]; then
    awk 'END { print NR + 0 }' "$f"
  else
    printf '0'
  fi
}

# Count records carrying BOTH a dispatchStatus and an action, so "recorded" and
# "recorded with the right handling" are one assertion rather than two hopes.
store_class_action_count() {
  local f
  f="$(store_path "$1")"
  if [[ ! -f "$f" ]]; then
    printf '0'
    return 0
  fi
  awk -v c="\"dispatchStatus\":\"$2\"" -v a="\"action\":\"$3\"" '
    index($0, c) > 0 && index($0, a) > 0 { n++ }
    END { print n + 0 }
  ' "$f"
}

store_field_values() {
  local f
  f="$(store_path "$1")"
  [[ -f "$f" ]] || return 0
  awk -v pat="\"$2\":\"" '
    {
      if (index($0, pat) > 0) {
        p = index($0, pat) + length(pat)
        rest = substr($0, p)
        q = index(rest, "\"")
        print substr(rest, 1, q - 1)
      }
    }
  ' "$f"
}

distinct_count() { sort -u <<< "$1" | awk 'NF { n++ } END { print n + 0 }'; }

assert_eq() {
  if [[ "$2" == "$3" ]]; then
    pass "$1"
  else
    fail "$1 (expected '$3', got '$2')"
  fi
}

# ---------------------------------------------------------------------------
# DEFAULT OFF. An unconfigured repository behaves exactly as it does today:
# nothing is recorded, nothing is enforced, and the exit status is clean.
# ---------------------------------------------------------------------------
new_repo unset
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status NO_RESULT
assert_eq "unconfigured repo: record exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: adapter is none" "$(kv adapter)" "none"
assert_eq "unconfigured repo: receipt skipped" "$(kv receipt)" "skipped"
assert_eq "unconfigured repo: zero records written" "$(store_count "$REPO")" "0"
if [[ -e "$REPO/.specify" ]]; then
  fail "unconfigured repo: no runtime directory is created"
else
  pass "unconfigured repo: no runtime directory is created"
fi
status_of "$REPO"
assert_eq "unconfigured repo: status exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: status reports zero records" "$(kv records)" "0"

# A repository that explicitly opts out is the same no-op as one that never
# configured the capability.
new_repo none
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status TRANSPORT_TERMINATED
assert_eq "explicit adapter=none: record exits 0" "$LAST_RC" "0"
assert_eq "explicit adapter=none: zero records written" "$(store_count "$REPO")" "0"

# A configured-but-unknown adapter fails LOUD rather than degrading to none: a
# typo must not silently disable the control.
new_repo bogus
LAST_OUT="$(bash "$SUT" resolve --repo-root "$REPO" 2>&1)"
LAST_RC=$?
assert_eq "unknown adapter fails loud" "$LAST_RC" "1"

# ---------------------------------------------------------------------------
# CLOSED CLASS SET. Each class is recorded with its own action, and the phase
# advances on exactly one of them.
# ---------------------------------------------------------------------------
new_repo jsonl

check_class() {
  local class="$1" expect_action="$2" expect_rc="$3" expect_advance="$4"
  shift 4
  local occ="occ-${class}#1"
  record "$REPO" --occurrence-id "$occ" --agent bubbles.implement \
    --packet-file "$REPO/packet.txt" --dispatch-status "$class" "$@"
  assert_eq "$class: exit status" "$LAST_RC" "$expect_rc"
  assert_eq "$class: prescribed action" "$(kv action)" "$expect_action"
  assert_eq "$class: advance verdict" "$(kv advance)" "$expect_advance"
  assert_eq "$class: recorded with its action" \
    "$(store_class_action_count "$REPO" "$class" "$expect_action")" "1"
  status_of "$REPO" --occurrence-id "$occ"
  assert_eq "$class: status advancePermitted" "$(kv advancePermitted)" \
    "$([[ "$expect_advance" == "permitted" ]] && printf 'true' || printf 'false')"
}

check_class TRANSPORT_TERMINATED retry-identical-once 1 refused
check_class NO_RESULT retry-identical-once 1 refused
check_class NARRATIVE_ONLY envelope-only-recovery 1 refused
check_class TIMEOUT resume-at-unresolved-leaf 1 refused
check_class ENVELOPE_FAILURE route-to-fix-loop 1 refused --result-envelope-status blocked
check_class ENVELOPE_OK advance 0 permitted --result-envelope-status completed_owned

# The sixth failure class is DERIVED, so it is exercised through escalation
# rather than by declaration (see the escalation block below).

# ---------------------------------------------------------------------------
# RETRY IDENTITY. A transport retry is another ATTEMPT at one occurrence, not a
# new occurrence. A fresh id on retry is exactly how a resultless dispatch
# disappears from the ledger.
# ---------------------------------------------------------------------------
new_repo jsonl
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status TRANSPORT_TERMINATED
FIRST_DIGEST="$(kv packetDigest)"
assert_eq "retry identity: first attempt is attempt 1" "$(kv attemptId)" "1"
assert_eq "retry identity: retry reuses the occurrence id" "$(kv retryOccurrenceId)" "implement#1"
assert_eq "retry identity: retry reuses the packet digest" "$(kv retryPacketDigest)" "$FIRST_DIGEST"

# The ONE permitted retry, dispatched with the identical packet, returning a
# real envelope this time.
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status ENVELOPE_OK \
  --result-envelope-status completed_owned
assert_eq "retry identity: the retry is attempt 2" "$(kv attemptId)" "2"
assert_eq "retry identity: the retry carries the same digest" "$(kv packetDigest)" "$FIRST_DIGEST"
assert_eq "retry identity: the retry advances the phase" "$LAST_RC" "0"
assert_eq "retry identity: exactly one occurrence id in the store" \
  "$(distinct_count "$(store_field_values "$REPO" occurrenceId)")" "1"
assert_eq "retry identity: exactly one packet digest in the store" \
  "$(distinct_count "$(store_field_values "$REPO" packetDigest)")" "1"
assert_eq "retry identity: two attempts recorded" "$(store_count "$REPO")" "2"

# ---------------------------------------------------------------------------
# ESCALATION. A second identical transport failure is not another retry.
# ---------------------------------------------------------------------------
new_repo jsonl
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status TRANSPORT_TERMINATED
assert_eq "escalation: first transport failure is not terminal" "$(kv terminal)" "false"
assert_eq "escalation: first transport failure exits 1" "$LAST_RC" "1"

record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status TRANSPORT_TERMINATED
assert_eq "escalation: second identical failure is REPEAT_INFRASTRUCTURE" \
  "$(kv dispatchStatus)" "REPEAT_INFRASTRUCTURE"
assert_eq "escalation: escalatedFrom names the original class" \
  "$(kv escalatedFrom)" "TRANSPORT_TERMINATED"
assert_eq "escalation: action is the typed infrastructure stop" \
  "$(kv action)" "stop-typed-infrastructure-blocked"
assert_eq "escalation: the phase is still refused" "$(kv advance)" "refused"
assert_eq "escalation: the run is terminal" "$(kv terminal)" "true"
assert_eq "escalation: terminal stop exits 3" "$LAST_RC" "3"
assert_eq "escalation: REPEAT_INFRASTRUCTURE recorded with its action" \
  "$(store_class_action_count "$REPO" REPEAT_INFRASTRUCTURE stop-typed-infrastructure-blocked)" "1"

# A DIFFERENT packet on the same occurrence is a different dispatch, so it is a
# first failure rather than a repeat. Escalating on the occurrence id alone
# would convert a genuine new attempt into a false infrastructure stop.
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/other-packet.txt" --dispatch-status TRANSPORT_TERMINATED
assert_eq "escalation: a different packet is not a repeat" \
  "$(kv dispatchStatus)" "TRANSPORT_TERMINATED"

# ---------------------------------------------------------------------------
# PER-CLASS BUDGETS. An infrastructure fault must not spend the allowance kept
# for genuine defect remediation. Both counters are asserted independently.
# ---------------------------------------------------------------------------
new_repo jsonl
record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status TRANSPORT_TERMINATED
status_of "$REPO" --occurrence-id 'implement#1'
assert_eq "budgets: infrastructure consumed after a transport failure" \
  "$(kv budget.infrastructure.consumed)" "1"
assert_eq "budgets: defect-remediation UNTOUCHED by a transport failure" \
  "$(kv budget.defect-remediation.consumed)" "0"
assert_eq "budgets: envelope-recovery UNTOUCHED by a transport failure" \
  "$(kv budget.envelope-recovery.consumed)" "0"

record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/other-packet.txt" --dispatch-status ENVELOPE_FAILURE \
  --result-envelope-status blocked
status_of "$REPO" --occurrence-id 'implement#1'
assert_eq "budgets: defect-remediation consumed by a real envelope failure" \
  "$(kv budget.defect-remediation.consumed)" "1"
assert_eq "budgets: infrastructure UNCHANGED by a defect failure" \
  "$(kv budget.infrastructure.consumed)" "1"
assert_eq "budgets: defect-remediation limit is not the infrastructure limit" \
  "$(kv budget.defect-remediation.limit)" "2"
assert_eq "budgets: defect-remediation not exhausted at one failure" \
  "$(kv budget.defect-remediation.exhausted)" "false"

record "$REPO" --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status NARRATIVE_ONLY
status_of "$REPO" --occurrence-id 'implement#1'
assert_eq "budgets: envelope-recovery consumed by a narrative-only reply" \
  "$(kv budget.envelope-recovery.consumed)" "1"
assert_eq "budgets: infrastructure still unchanged" "$(kv budget.infrastructure.consumed)" "1"
assert_eq "budgets: defect-remediation still unchanged" "$(kv budget.defect-remediation.consumed)" "1"

# ---------------------------------------------------------------------------
# ADVERSARIAL. A receipt is EARNED by dispatching, never by asserting it. Every
# case below must be refused AND must write nothing.
# ---------------------------------------------------------------------------
new_repo jsonl

refuse_case() {
  local label="$1"
  shift
  local before after
  before="$(store_count "$REPO")"
  record "$REPO" "$@"
  after="$(store_count "$REPO")"
  if [[ "$LAST_RC" -ne 2 ]]; then
    fail "$label refused with exit 2 (got $LAST_RC)"
  else
    pass "$label refused with exit 2"
  fi
  assert_eq "$label wrote no record" "$after" "$before"
}

refuse_case "receipt with no dispatched packet is" \
  --occurrence-id 'implement#1' --agent bubbles.implement --dispatch-status ENVELOPE_OK \
  --result-envelope-status completed

refuse_case "receipt naming a packet that does not exist is" \
  --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/no-such-packet.txt" --dispatch-status ENVELOPE_OK \
  --result-envelope-status completed

refuse_case "receipt with a forged packet digest is" \
  --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --packet-digest 0000000000000000000000000000000000000000000000000000000000000000 \
  --dispatch-status ENVELOPE_OK --result-envelope-status completed

refuse_case "receipt with a digest borrowed from another packet is" \
  --occurrence-id 'implement#1' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" \
  --packet-digest "$(bash "$SUT" record --repo-root "$REPO" --occurrence-id 'probe#1' \
    --agent bubbles.implement --packet-file "$REPO/other-packet.txt" \
    --dispatch-status TIMEOUT 2>/dev/null | awk -F= '$1 == "packetDigest" { print $2; exit }')" \
  --dispatch-status ENVELOPE_OK --result-envelope-status completed

refuse_case "self-declared REPEAT_INFRASTRUCTURE is" \
  --occurrence-id 'implement#2' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status REPEAT_INFRASTRUCTURE

refuse_case "an advance claimed with no envelope status is" \
  --occurrence-id 'implement#3' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status ENVELOPE_OK

refuse_case "an advance claimed on a blocked envelope is" \
  --occurrence-id 'implement#4' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status ENVELOPE_OK \
  --result-envelope-status blocked

refuse_case "an envelope failure claiming a success outcome is" \
  --occurrence-id 'implement#5' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status ENVELOPE_FAILURE \
  --result-envelope-status completed_owned

refuse_case "an unknown dispatch class is" \
  --occurrence-id 'implement#6' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status SOMEWHAT_FAILED

refuse_case "a bypass flag is" \
  --occurrence-id 'implement#7' --agent bubbles.implement \
  --packet-file "$REPO/packet.txt" --dispatch-status NO_RESULT --force

# ---------------------------------------------------------------------------
printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
