#!/usr/bin/env bash
# Hermetic selftest for goal-fidelity-telemetry.sh (IMP-038 SCOPE-7 / GF-5)
# plus the remaining SCOPE-7 adversarial scenarios that have no natural home in
# the per-script selftests.
#
# The telemetry cases exist for ONE property above all: an operator prompt must
# never reach the ledger. That is asserted structurally — there is no free-text
# field — and the refusal cases below fail if one is ever added back.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TELEMETRY="$SCRIPT_DIR/goal-fidelity-telemetry.sh"
GC="$SCRIPT_DIR/goal-contract.sh"
RESOLVER="$SCRIPT_DIR/work-boundary-resolve.sh"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

if ! command -v jq >/dev/null 2>&1; then
  echo "goal-fidelity-telemetry-selftest: SKIP (jq not installed)"
  exit 0
fi
[[ -f "$TELEMETRY" ]] || { echo "FAIL: $TELEMETRY not found" >&2; exit 1; }

echo "Running goal-fidelity-telemetry selftest..."

expect_rc() {
  local label="$1" want="$2"; shift 2
  local rc=0
  "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then pass "$label"; else fail "$label (expected exit $want, got $rc)"; fi
}

repo="$TMP_ROOT/repo"
mkdir -p "$repo"
ledger="$repo/.specify/runtime/framework-events.jsonl"

# ── Recording ───────────────────────────────────────────────────────────────
expect_rc "T1 a phase-relevance event records" 0 \
  bash "$TELEMETRY" --repo-root "$repo" --event phase-relevance \
    --runner bubbles.goal --phase security --verdict skip --rule docs-only

if [[ -f "$ledger" ]] && [[ "$(jq -r 'select(.type == "goal-fidelity.phase-relevance") | .runner' "$ledger")" == "bubbles.goal" ]]; then
  pass "T1b the event is attributed to its deciding runner"
else
  fail "T1b runner attribution missing: $(cat "$ledger" 2>/dev/null)"
fi

for ev in contract-frozen contract-revised expansion-requested expansion-rejected \
          boundary-refusal finding-routed finding-goal-blocking; do
  expect_rc "T2 event '$ev' is accepted" 0 \
    bash "$TELEMETRY" --repo-root "$repo" --event "$ev" --goal-id "gc:sess:1"
done

if [[ "$(wc -l < "$ledger")" -eq 8 ]]; then
  pass "T2b every accepted event appended exactly one line"
else
  fail "T2b expected 8 ledger lines, found $(wc -l < "$ledger")"
fi

if jq -e . "$ledger" >/dev/null 2>&1; then
  pass "T2c every ledger line is valid JSON"
else
  fail "T2c the ledger contains a malformed line"
fi

# The record shape must match the existing framework-events.jsonl convention,
# or the ledger's readers stop working on the new rows.
if [[ "$(jq -r 'select(.type == "goal-fidelity.contract-frozen") | [has("version"), has("type"), has("timestamp")] | all' "$ledger")" == "true" ]]; then
  pass "T2d records carry version, type and timestamp like every other framework event"
else
  fail "T2d record shape diverges from framework-events.jsonl"
fi

# ── The property that matters: no prompt can reach the ledger ───────────────
for free_text in --details --note --reason --message --prompt --text --request; do
  expect_rc "T3 '$free_text' is refused (no free-text field exists)" 2 \
    bash "$TELEMETRY" --repo-root "$repo" --event contract-frozen "$free_text" "the operator typed this"
done

if grep -qE '^\s*--(details|note|reason|message|prompt|text|request)\)\s*[a-z_]+=' "$TELEMETRY"; then
  fail "T3b goal-fidelity-telemetry.sh assigns a free-text field to a variable"
else
  pass "T3b goal-fidelity-telemetry.sh binds no free-text field to a variable"
fi

before="$(wc -l < "$ledger")"
bash "$TELEMETRY" --repo-root "$repo" --event contract-frozen --details "leak me" >/dev/null 2>&1
if [[ "$(wc -l < "$ledger")" -eq "$before" ]]; then
  pass "T3c a refused free-text call wrote NOTHING to the ledger"
else
  fail "T3c a refused call still appended a row"
fi

# A digest is a hash of the request, so it identifies the goal without
# reproducing it. That distinction is the reason the field is allowed at all.
bash "$TELEMETRY" --repo-root "$repo" --event contract-frozen \
  --digest "sha256:$(printf '%064d' 0)" >/dev/null 2>&1
if grep -q 'sha256:' "$ledger" && ! grep -qi 'operator typed\|leak me' "$ledger"; then
  pass "T3d the ledger holds hashed request identity and no prompt text"
else
  fail "T3d ledger content check failed"
fi

# ── Enum discipline ─────────────────────────────────────────────────────────
expect_rc "T4 an unknown event type is refused" 2 \
  bash "$TELEMETRY" --repo-root "$repo" --event invented-event
expect_rc "T4b an out-of-enum verdict is refused" 2 \
  bash "$TELEMETRY" --repo-root "$repo" --event phase-relevance --verdict maybe
expect_rc "T4c an out-of-enum disposition is refused" 2 \
  bash "$TELEMETRY" --repo-root "$repo" --event boundary-refusal --disposition sideways
expect_rc "T4d an out-of-enum goalImpact is refused" 2 \
  bash "$TELEMETRY" --repo-root "$repo" --event finding-routed --goal-impact whenever
expect_rc "T4e a non-integer revision is refused" 2 \
  bash "$TELEMETRY" --repo-root "$repo" --event contract-revised --revision two
expect_rc "T4f a malformed digest is refused" 2 \
  bash "$TELEMETRY" --repo-root "$repo" --event contract-frozen --digest "not-a-digest"
expect_rc "T4g missing --event is a usage error" 2 bash "$TELEMETRY" --repo-root "$repo"
expect_rc "T4h --help exits 0" 0 bash "$TELEMETRY" --help

# ── Telemetry is observability, never a gate ────────────────────────────────
expect_rc "T5 BUBBLES_TELEMETRY=0 disables recording without failing" 0 \
  env BUBBLES_TELEMETRY=0 bash "$TELEMETRY" --repo-root "$repo" --event contract-frozen

blocked="$TMP_ROOT/blocked"
mkdir -p "$blocked"
: > "$blocked/.specify"   # a FILE where the runtime directory must go
expect_rc "T5b an unwritable runtime path still exits 0 (telemetry never blocks delivery)" 0 \
  bash "$TELEMETRY" --repo-root "$blocked" --event contract-frozen

# ── SCOPE-7 scenarios without a natural per-script home ─────────────────────
if [[ -f "$GC" && -f "$RESOLVER" ]]; then
  d="$TMP_ROOT/scenario"
  mkdir -p "$d/spec"
  printf 'Fix the boundary resolver.\n' > "$d/request.txt"
  echo '{}' > "$d/session.json"
  bash "$GC" freeze --session-file "$d/session.json" --source-request-file "$d/request.txt" \
    --intent "Fix the boundary resolver" --success-signal "the resolver refuses an undeclared boundary" \
    --hard-constraint "no new dependency" \
    --target "spec=specs/900-narrow" --repository-root primary-repo \
    --spec-target specs/900-narrow --allowed-path 'bubbles/scripts/**' \
    --runner bubbles.goal --session-id scenario-1 --repository-alias primary-repo >/dev/null 2>&1

  # S1: a planner that bolts on an unrelated adjacent capability is asking for
  # reach it was not granted. Without --approval-note that is an unapproved
  # expansion, which is the shape "while I was in there..." takes.
  rc=0
  bash "$GC" revise --session-file "$d/session.json" \
    --intent "Fix the boundary resolver and add a metrics dashboard" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq 3 ]]; then
    pass "S1 a planner adding an unrelated capability without approval is refused"
  else
    fail "S1 expected refusal exit 3, got $rc"
  fi
  if [[ "$(bash "$GC" read --session-file "$d/session.json" --field .intent 2>/dev/null)" == "Fix the boundary resolver" ]]; then
    pass "S1b the refused expansion left the frozen intent untouched"
  else
    fail "S1b the refused expansion mutated the intent"
  fi

  # S2: an unrelated same-repo bug found by a quality phase is route-same-repo,
  # not something to fix inline in the parent packet.
  echo '{ "version": 3 }' > "$d/spec/state.json"
  bash "$GC" sync-boundary --session-file "$d/session.json" --state-file "$d/spec/state.json" >/dev/null 2>&1
  disp="$(bash "$RESOLVER" --feature-dir "$d/spec" --candidate-repo primary-repo \
    --candidate-spec specs/901-unrelated --strict 2>/dev/null | sed -n 's/^disposition=//p')"
  if [[ "$disp" == "route-same-repo" ]]; then
    pass "S2 an unrelated same-repo finding resolves route-same-repo, not in-boundary"
  else
    fail "S2 expected route-same-repo, got '$disp'"
  fi

  # S3: a pre-IMP-038 spec stays READABLE, and its first mutable run must
  # declare a boundary before editing. Both halves matter — permissiveness for
  # reading was never a licence to edit.
  legacy="$TMP_ROOT/legacy"
  mkdir -p "$legacy"
  echo '{ "version": 3, "status": "done" }' > "$legacy/state.json"
  legacy_read="$(bash "$RESOLVER" --feature-dir "$legacy" --candidate-repo primary-repo 2>/dev/null | sed -n 's/^disposition=//p')"
  rc=0
  bash "$RESOLVER" --feature-dir "$legacy" --candidate-repo primary-repo --strict --require-allowed-paths >/dev/null 2>&1 || rc=$?
  if [[ "$legacy_read" == "in-boundary" && "$rc" -eq 3 ]]; then
    pass "S3 a historical spec stays readable but its first MUTABLE run is refused until a boundary exists"
  else
    fail "S3 legacy read='$legacy_read' (want in-boundary), strict mutable rc=$rc (want 3)"
  fi
  bash "$GC" sync-boundary --session-file "$d/session.json" --state-file "$legacy/state.json" >/dev/null 2>&1
  expect_rc "S3b after syncing a boundary, the same mutable run is authorized" 0 \
    bash "$RESOLVER" --feature-dir "$legacy" --candidate-repo primary-repo \
      --candidate-spec specs/900-narrow --candidate-path 'bubbles/scripts/x.sh' \
      --strict --require-allowed-paths
else
  fail "S1-S3 require goal-contract.sh and work-boundary-resolve.sh"
fi

# S4: the G095 disposition set is unchanged by the goalImpact addition. If a
# value were dropped or renamed, existing filings would stop validating.
schema="$(cd "$SCRIPT_DIR/../.." && pwd)/bubbles/schemas/result-envelope.schema.json"
if [[ -f "$schema" ]]; then
  observed="$(jq -r '.properties.findings.items.properties.disposition.enum | sort | join(",")' "$schema" 2>/dev/null)"
  expected="bug-filed,fixed-in-session,ops-filed,routed,spec-filed,status-adjusted"
  if [[ "$observed" == "$expected" ]]; then
    pass "S4 the G095 disposition set is unchanged after goalImpact was added"
  else
    fail "S4 disposition set drifted: observed '$observed'"
  fi
  impact="$(jq -r '.properties.findings.items.properties.goalImpact.enum | sort | join(",")' "$schema" 2>/dev/null)"
  if [[ "$impact" == "blocking-external,independent,required" ]]; then
    pass "S4b goalImpact carries exactly the three classifications"
  else
    fail "S4b goalImpact enum drifted: '$impact'"
  fi
else
  fail "S4 result-envelope schema not found at $schema"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "goal-fidelity-telemetry-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "goal-fidelity-telemetry-selftest: all cases passed."
