#!/usr/bin/env bash
# tool-log-semantic-admission-selftest.sh — IMP-047 S-C (AC13).
#
# The retired rule admitted an exit-zero tool call as DoD evidence when its
# command line shared two non-stopword tokens with a checked DoD item. Two
# tokens is a coincidence, not a proof, and a coincidence presented as evidence
# is the fabrication shape the evidence rules exist to stop.
#
# What replaces it is five bindings, all required:
#   scenario  the item points at a scenario and the receipt is filed under it
#   claim     the receipt's claim COVERS the item, totally, not partially
#   command   the receipt records the literal command that ran
#   revision  the receipt cites the resolved source revision
#   outcome   exit zero, unless the item asserts an expected failure
#
# Every case below removes exactly ONE binding and asserts the admission is
# refused. A test that only proved the happy path would pass against a function
# that admits everything.
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = a dependency is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/state-transition-guard.sh"
BRIDGE="$SCRIPT_DIR/evidence-tool-log-bridge.sh"
SCHEMA="$SCRIPT_DIR/../schemas/tool-call.schema.json"
NAME="tool-log-semantic-admission-selftest"

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

for required in "$GUARD" "$BRIDGE" "$SCHEMA"; do
  [[ -f "$required" ]] || {
    printf '%s: required file not found: %s\n' "$NAME" "$required" >&2
    exit 2
  }
done
command -v python3 >/dev/null 2>&1 || {
  printf '%s: python3 is required\n' "$NAME" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-tool-log-admission.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

# The admission function is sourced out of the guard rather than reimplemented,
# because a copy of the logic under test proves only that the copy agrees with
# itself. It is extracted by name so the assertions run the SHIPPING code.
EXTRACT="$TMP_DIR/_tool_log_covers_dod_item.sh"
python3 - "$GUARD" "$EXTRACT" <<'PY'
import sys
src, dest = sys.argv[1], sys.argv[2]
text = open(src, encoding='utf-8').read()
start = text.index('_tool_log_covers_dod_item() {')
end = text.index('\n}\n', start) + len('\n}\n')
open(dest, 'w', encoding='utf-8').write(
    '#!/usr/bin/env bash\nset -uo pipefail\nSCRIPT_DIR="${SCRIPT_DIR:?}"\n' + text[start:end])
PY
[[ -s "$EXTRACT" ]] || {
  printf '%s: could not extract the admission function from the guard\n' "$NAME" >&2
  exit 2
}

# A fixture repo: a real git repo, so the guard resolves a real HEAD revision
# and the source-revision binding is exercised against a genuine value.
REPO="$TMP_DIR/repo"
mkdir -p "$REPO/specs/900-admission" "$REPO/.specify/runtime"
git -C "$REPO" init -q 2>/dev/null
git -C "$REPO" config user.email selftest@example.invalid
git -C "$REPO" config user.name selftest
printf 'fixture\n' > "$REPO/README.md"
git -C "$REPO" add -A >/dev/null 2>&1
git -C "$REPO" -c commit.gpgsign=false commit -qm fixture >/dev/null 2>&1
REV="$(git -C "$REPO" rev-parse --verify HEAD)"
OTHER_REV="$(printf '%040d' 0)"

SPEC_DIR="$REPO/specs/900-admission"
printf '# spec\n' > "$SPEC_DIR/spec.md"
LOG="$REPO/.specify/runtime/tool-calls.jsonl"

DOD='- [x] The coupon multiplier recomputes the checkout total → Receipt: SCN-900-001'
CLAIM='The coupon multiplier recomputes the checkout total'
CMD='npx playwright test checkout'

# Write one receipt. Every argument is explicit so a case can vary exactly one.
write_receipt() {
  # scenarioId claim revision exitCode cmd [spec]
  local sid="$1" claim="$2" rev="$3" exit_code="$4" cmd="$5" spec="${6:-900-admission}"
  SID="$sid" CLAIM="$claim" REV="$rev" EXITC="$exit_code" CMD="$cmd" SPEC="$spec" \
    python3 - "$LOG" <<'PY'
import json, os, sys
record = {
    "schemaVersion": 3,
    "ts": "2026-08-17T09:00:00Z",
    "sessionId": "s-admission",
    "agent": "selftest",
    "spec": os.environ['SPEC'],
    "cmd": os.environ['CMD'],
    "exitCode": int(os.environ['EXITC']),
    "scenarioBinding": {
        "scenarioId": os.environ['SID'],
        "phase": "green",
        "testIdentity": "tests/e2e/checkout.spec.ts::coupon recomputes total",
        "sourceRevision": os.environ['REV'],
        "negativeControl": "drop the coupon multiplier",
    },
}
claim = os.environ['CLAIM']
if claim:
    record["scenarioBinding"]["claim"] = claim
with open(sys.argv[1], 'w', encoding='utf-8') as fh:
    fh.write(json.dumps(record, separators=(',', ':')) + "\n")
PY
}

admits() {
  # Runs the SHIPPING admission function against the fixture.
  SCRIPT_DIR="$SCRIPT_DIR" bash -c '
    source "$1"
    _tool_log_covers_dod_item "$2" "$3"
  ' _ "$EXTRACT" "$SPEC_DIR" "$DOD"
}

expect_refused() {
  local label="$1"
  local rc
  admits
  rc=$?
  if [[ "$rc" -ne 0 ]]; then
    pass "$label"
  else
    fail "$label — it was ADMITTED (exit $rc)"
  fi
}

# ---------------------------------------------------------------------------
# The happy path must work, or every refusal below is vacuous.
# ---------------------------------------------------------------------------
write_receipt "SCN-900-001" "$CLAIM" "$REV" 0 "$CMD"
if admits; then
  pass "a fully bound receipt (scenario, claim, command, revision, outcome) is admitted"
else
  fail "a fully bound receipt was refused — every refusal assertion below would be vacuous"
fi

# ---------------------------------------------------------------------------
# THE RETIRED PATH. This is the case S-C names: a command that shares TOKENS
# with the DoD item but carries no semantic binding at all. Under the old rule
# `checkout` + `total` was two tokens and the item was admitted.
# ---------------------------------------------------------------------------
python3 - "$LOG" <<'PY'
import json, sys
record = {
    "schemaVersion": 2,
    "ts": "2026-08-17T09:00:00Z",
    "sessionId": "s-lexical",
    "spec": "900-admission",
    "cmd": "npx playwright test checkout --reporter total",
    "exitCode": 0,
}
with open(sys.argv[1], 'w', encoding='utf-8') as fh:
    fh.write(json.dumps(record, separators=(',', ':')) + "\n")
PY
expect_refused "token overlap with NO scenarioBinding is refused (the retired lexical path is gone)"

# A binding that exists but names a DIFFERENT scenario is a substitution.
write_receipt "SCN-900-999" "$CLAIM" "$REV" 0 "$CMD"
expect_refused "a receipt filed under a DIFFERENT scenario is refused"

# ---------------------------------------------------------------------------
# ONE BINDING REMOVED PER CASE.
# ---------------------------------------------------------------------------
write_receipt "SCN-900-001" "" "$REV" 0 "$CMD"
expect_refused "claim binding removed: a receipt with no claim is refused"

# Partial coverage. The claim shares tokens with the item but does not cover it,
# which is exactly the shape the two-token rule used to accept.
write_receipt "SCN-900-001" "checkout total" "$REV" 0 "$CMD"
expect_refused "claim binding weakened: a claim that only PARTIALLY covers the item is refused"

write_receipt "SCN-900-001" "$CLAIM" "$OTHER_REV" 0 "$CMD"
expect_refused "revision binding broken: source-revision drift is refused"

write_receipt "SCN-900-001" "$CLAIM" "" 0 "$CMD"
expect_refused "revision binding removed: a receipt with no source revision is refused"

write_receipt "SCN-900-001" "$CLAIM" "$REV" 1 "$CMD"
expect_refused "outcome binding broken: a non-zero exit does not prove a success claim"

write_receipt "SCN-900-001" "$CLAIM" "$REV" 0 ""
expect_refused "command binding removed: a receipt with no command is refused"

write_receipt "SCN-900-001" "$CLAIM" "$REV" 0 "$CMD" "901-other-spec"
expect_refused "spec binding broken: a receipt belonging to another spec is refused"

# ---------------------------------------------------------------------------
# The DoD item must POINT at a scenario. Without a pointer there is no claim of
# coverage to verify, and inferring the pointer would be guessing again.
# ---------------------------------------------------------------------------
write_receipt "SCN-900-001" "$CLAIM" "$REV" 0 "$CMD"
DOD_NO_POINTER='- [x] The coupon multiplier recomputes the checkout total'
if SCRIPT_DIR="$SCRIPT_DIR" bash -c '
  source "$1"
  _tool_log_covers_dod_item "$2" "$3"
' _ "$EXTRACT" "$SPEC_DIR" "$DOD_NO_POINTER"; then
  fail "a DoD item with NO receipt pointer was admitted by an unrelated receipt"
else
  pass "a DoD item with NO receipt pointer is refused even when a bound receipt exists"
fi

# ---------------------------------------------------------------------------
# An expected-FAILURE item is proven by a non-zero exit. Accepting exit 0 for
# both outcomes would make the outcome binding unfalsifiable.
# ---------------------------------------------------------------------------
DOD_FAILING='- [x] A hand-written scenario state is refused by the resolver → Receipt: SCN-900-002'
CLAIM_FAILING='A hand-written scenario state is refused by the resolver'
write_receipt "SCN-900-002" "$CLAIM_FAILING" "$REV" 1 "bash scenario-state-resolve.sh --spec-dir fixture"
if SCRIPT_DIR="$SCRIPT_DIR" bash -c '
  source "$1"
  _tool_log_covers_dod_item "$2" "$3"
' _ "$EXTRACT" "$SPEC_DIR" "$DOD_FAILING"; then
  pass "an expected-failure item is admitted by a NON-ZERO exit"
else
  fail "an expected-failure item was refused despite a non-zero-exit receipt"
fi
write_receipt "SCN-900-002" "$CLAIM_FAILING" "$REV" 0 "bash scenario-state-resolve.sh --spec-dir fixture"
if SCRIPT_DIR="$SCRIPT_DIR" bash -c '
  source "$1"
  _tool_log_covers_dod_item "$2" "$3"
' _ "$EXTRACT" "$SPEC_DIR" "$DOD_FAILING"; then
  fail "an expected-failure item was admitted by an exit-ZERO receipt"
else
  pass "an expected-failure item is NOT admitted by an exit-zero receipt"
fi

# ---------------------------------------------------------------------------
# The bridge must report the same verdict as the guard. Two admission answers
# from two readers of one log is the drift the single-authority rule forbids.
# ---------------------------------------------------------------------------
printf '%s\n' "## Scope 1" "$DOD" > "$SPEC_DIR/scopes.md"
write_receipt "SCN-900-001" "$CLAIM" "$REV" 0 "$CMD"
bridge_out="$(cd "$REPO" && bash "$BRIDGE" "$SPEC_DIR" --log "$LOG" --format json 2>&1)"
if printf '%s' "$bridge_out" | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d["matchedDodItems"] == 1 else 1)' 2>/dev/null; then
  pass "the bridge admits the same fully bound receipt the guard admits"
else
  fail "the bridge disagreed with the guard on a fully bound receipt"
  printf '  output: %s\n' "$bridge_out"
fi

write_receipt "SCN-900-001" "checkout total" "$REV" 0 "$CMD"
bridge_out="$(cd "$REPO" && bash "$BRIDGE" "$SPEC_DIR" --log "$LOG" --format json 2>&1)"
if printf '%s' "$bridge_out" | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d["matchedDodItems"] == 0 and d["unbound"] else 1)' 2>/dev/null; then
  pass "the bridge refuses a partially covering claim and names it as unbound"
else
  fail "the bridge admitted a partially covering claim"
  printf '  output: %s\n' "$bridge_out"
fi

# ---------------------------------------------------------------------------
# The schema must ACCEPT the binding this admission path requires. A guard that
# demands a field the schema rejects would refuse every honest receipt.
# ---------------------------------------------------------------------------
schema_rc=0
SCHEMA="$SCHEMA" LOG="$LOG" python3 - <<'PY' || schema_rc=$?
import json, os, sys
try:
    from jsonschema import Draft7Validator
except Exception:
    print('  (jsonschema not importable; schema acceptance not asserted)')
    sys.exit(3)
validator = Draft7Validator(json.load(open(os.environ['SCHEMA'])))
for raw in open(os.environ['LOG'], encoding='utf-8'):
    raw = raw.strip()
    if not raw:
        continue
    errors = list(validator.iter_errors(json.loads(raw)))
    if errors:
        print('  schema rejected a bound receipt: %s' % errors[0].message)
        sys.exit(1)
sys.exit(0)
PY
if [[ "$schema_rc" -eq 0 ]]; then
  pass "the tool-call schema accepts a scenarioBinding receipt"
elif [[ "$schema_rc" -eq 3 ]]; then
  pass "schema acceptance skipped (jsonschema unavailable) without failing the suite"
else
  fail "the tool-call schema REJECTS the receipt shape the admission path requires"
fi

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
