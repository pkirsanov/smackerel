#!/usr/bin/env bash
# Hermetic selftest for goal-fidelity-guard.sh (IMP-038 SCOPE-6 / SCOPE-7)
# (Gate G134 — goal_fidelity_gate).
#
# The guard's whole value is that it FAILS on drift a runner would otherwise
# carry to certification. Every case below therefore pairs a green fixture with
# the drifted variant, so a guard that silently stopped checking would fail here
# rather than start passing everything.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/goal-fidelity-guard.sh"
GC="$SCRIPT_DIR/goal-contract.sh"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

if ! command -v jq >/dev/null 2>&1; then
  echo "goal-fidelity-guard-selftest: SKIP (jq not installed)"
  exit 0
fi
[[ -f "$GUARD" ]] || { echo "FAIL: $GUARD not found" >&2; exit 1; }

echo "Running goal-fidelity-guard selftest..."

# expect_rc <label> <want-rc> <command...>
expect_rc() {
  local label="$1" want="$2"; shift 2
  local rc=0
  "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then pass "$label"; else fail "$label (expected exit $want, got $rc)"; fi
}

# expect_finding <label> <finding-token> <command...>
expect_finding() {
  local label="$1" token="$2"; shift 2
  local out rc=0
  out="$("$@" 2>&1)" || rc=$?
  if [[ "$rc" -eq 1 ]] && grep -qF "$token" <<< "$out"; then
    pass "$label"
  else
    fail "$label (rc=$rc, out=$(tr '\n' ' ' <<< "$out"))"
  fi
}

# new_case <name> — a workspace with a frozen contract and a synced spec.
new_case() {
  local d="$TMP_ROOT/$1"
  mkdir -p "$d/spec"
  printf 'Deliver the goal-fidelity guard.\n' > "$d/request.txt"
  echo '{}' > "$d/session.json"
  bash "$GC" freeze \
    --session-file "$d/session.json" \
    --source-request-file "$d/request.txt" \
    --intent "Enforce goal fidelity at planning and completion boundaries" \
    --success-signal "goal-fidelity-guard-selftest exits 0" \
    --hard-constraint "no bypass flag exists" \
    --target "spec=specs/038-goal-fidelity" \
    --repository-root bubbles \
    --spec-target specs/038-goal-fidelity \
    --allowed-path 'bubbles/scripts/**' \
    --runner bubbles.goal \
    --session-id "sess-$1" \
    --repository-alias bubbles >/dev/null 2>&1
  echo '{ "version": 3, "status": "in_progress" }' > "$d/spec/state.json"
  bash "$GC" sync-boundary --session-file "$d/session.json" --state-file "$d/spec/state.json" >/dev/null 2>&1
  bash "$GC" mirror --session-file "$d/session.json" --state-file "$d/spec/state.json" >/dev/null 2>&1
  cat > "$d/spec/spec.md" <<'MD'
# Feature

## Outcome Contract

- **Intent**: Enforce goal fidelity at planning and completion boundaries.
- **Success Signal**: goal-fidelity-guard-selftest exits 0.
- **Hard Constraints**: no bypass flag exists.
- **Failure Condition**: a boundary passes while drift is present.
MD
  cat > "$d/spec/report.md" <<'MD'
# Report

## Summary

Delivered the guard.

## Test Evidence

The declared Success Signal was demonstrated by running the selftest.
MD
  echo "$d"
}

# ── B1 pre-planning ─────────────────────────────────────────────────────────
d1="$(new_case b1)"
expect_rc "T1 pre-planning passes on a frozen, repository-bound contract" 0 \
  bash "$GUARD" --boundary pre-planning --session-file "$d1/session.json"

echo '{}' > "$d1/empty-session.json"
expect_finding "T1b pre-planning refuses a session with no Goal Contract" "GF-1" \
  bash "$GUARD" --boundary pre-planning --session-file "$d1/empty-session.json"

jq 'del(.goalContract.approval)' "$d1/session.json" > "$d1/unfrozen.json"
expect_finding "T1c pre-planning refuses a contract with no approval state" "GF-1" \
  bash "$GUARD" --boundary pre-planning --session-file "$d1/unfrozen.json"

# ── B2 post-planning ────────────────────────────────────────────────────────
d2="$(new_case b2)"
expect_rc "T2 post-planning passes on a synced, mirrored spec" 0 \
  bash "$GUARD" --boundary post-planning --session-file "$d2/session.json" --spec-dir "$d2/spec"

jq 'del(.workBoundary)' "$d2/spec/state.json" > "$d2/spec/tmp" && mv "$d2/spec/tmp" "$d2/spec/state.json"
expect_finding "T2b post-planning refuses a spec with no declared workBoundary" "GF-2" \
  bash "$GUARD" --boundary post-planning --session-file "$d2/session.json" --spec-dir "$d2/spec"

d2b="$(new_case b2b)"
jq 'del(.execution.goalContractRef)' "$d2b/spec/state.json" > "$d2b/spec/tmp" && mv "$d2b/spec/tmp" "$d2b/spec/state.json"
expect_finding "T2c post-planning refuses a spec that points at no goal" "GF-1" \
  bash "$GUARD" --boundary post-planning --session-file "$d2b/session.json" --spec-dir "$d2b/spec"

d2c="$(new_case b2c)"
jq '.execution.goalContractRef.goalId = "gc:someone-else:1"' "$d2c/spec/state.json" > "$d2c/spec/tmp" && mv "$d2c/spec/tmp" "$d2c/spec/state.json"
expect_finding "T2d post-planning refuses a spec pointing at a different goal" "GF-1" \
  bash "$GUARD" --boundary post-planning --session-file "$d2c/session.json" --spec-dir "$d2c/spec"

# ── G070 repair: the Outcome Contract presence check that never existed ─────
d3="$(new_case b3)"
expect_rc "T3 post-planning passes on a complete Outcome Contract" 0 \
  bash "$GUARD" --boundary post-planning --session-file "$d3/session.json" --spec-dir "$d3/spec"

d3b="$(new_case b3b)"
printf '# Feature\n\nNo outcome contract here.\n' > "$d3b/spec/spec.md"
expect_finding "T3b a spec.md with NO Outcome Contract is refused (G070 repair)" "G070" \
  bash "$GUARD" --boundary post-planning --session-file "$d3b/session.json" --spec-dir "$d3b/spec"

d3c="$(new_case b3c)"
printf '# Feature\n\n## Outcome Contract\n\n## Next Section\n\ntext\n' > "$d3c/spec/spec.md"
expect_finding "T3c an EMPTY Outcome Contract section is refused, not just a missing one" "G070" \
  bash "$GUARD" --boundary post-planning --session-file "$d3c/session.json" --spec-dir "$d3c/spec"

d3d="$(new_case b3d)"
cat > "$d3d/spec/spec.md" <<'MD'
# Feature

## Outcome Contract

- **Intent**: do the thing.
- **Hard Constraints**: none.
MD
expect_finding "T3d an Outcome Contract with no Success Signal is refused" "Success Signal" \
  bash "$GUARD" --boundary post-planning --session-file "$d3d/session.json" --spec-dir "$d3d/spec"

d3e="$(new_case b3e)"
rm -f "$d3e/spec/spec.md"
expect_finding "T3e a missing spec.md is refused" "G070" \
  bash "$GUARD" --boundary post-planning --session-file "$d3e/session.json" --spec-dir "$d3e/spec"

# ── B3 pre-dispatch ─────────────────────────────────────────────────────────
d4="$(new_case b4)"
bash "$GC" ref --session-file "$d4/session.json" > "$d4/ref.json" 2>/dev/null
expect_rc "T4 pre-dispatch passes for in-boundary mutable work with a valid ref" 0 \
  bash "$GUARD" --boundary pre-dispatch --session-file "$d4/session.json" --spec-dir "$d4/spec" \
    --candidate-repo bubbles --candidate-spec specs/038-goal-fidelity \
    --candidate-path 'bubbles/scripts/x.sh' --ref-file "$d4/ref.json" --mutable

expect_finding "T4b pre-dispatch refuses a foreign repository" "GF-2" \
  bash "$GUARD" --boundary pre-dispatch --session-file "$d4/session.json" --spec-dir "$d4/spec" \
    --candidate-repo other-repo --mutable

expect_finding "T4c pre-dispatch refuses an out-of-boundary path" "GF-2" \
  bash "$GUARD" --boundary pre-dispatch --session-file "$d4/session.json" --spec-dir "$d4/spec" \
    --candidate-repo bubbles --candidate-path 'docs/other.md' --mutable

jq '.sourceRequestDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"' \
  "$d4/ref.json" > "$d4/bad-ref.json"
expect_finding "T4d pre-dispatch refuses a dispatch packet with a substituted digest" "GF-1" \
  bash "$GUARD" --boundary pre-dispatch --session-file "$d4/session.json" --spec-dir "$d4/spec" \
    --candidate-repo bubbles --candidate-path 'bubbles/scripts/x.sh' --ref-file "$d4/bad-ref.json" --mutable

d4b="$(new_case b4b)"
jq 'del(.workBoundary)' "$d4b/spec/state.json" > "$d4b/spec/tmp" && mv "$d4b/spec/tmp" "$d4b/spec/state.json"
expect_finding "T4e pre-dispatch refuses when the spec declares no boundary at all" "GF-2" \
  bash "$GUARD" --boundary pre-dispatch --session-file "$d4b/session.json" --spec-dir "$d4b/spec" \
    --candidate-repo bubbles --mutable

# ── B4 post-finding ─────────────────────────────────────────────────────────
d5="$(new_case b5)"
expect_rc "T5 post-finding passes when every changed path is in-boundary" 0 \
  bash "$GUARD" --boundary post-finding --session-file "$d5/session.json" --spec-dir "$d5/spec" \
    --changed-path 'bubbles/scripts/a.sh' --changed-path 'bubbles/scripts/b.sh'

expect_finding "T5b post-finding refuses an out-of-boundary path changed in the parent packet" "GF-3" \
  bash "$GUARD" --boundary post-finding --session-file "$d5/session.json" --spec-dir "$d5/spec" \
    --changed-path 'bubbles/scripts/a.sh' --changed-path 'docs/unrelated.md'

# ── B5 post-compaction / resume ─────────────────────────────────────────────
d6="$(new_case b6)"
bash "$GC" ref --session-file "$d6/session.json" > "$d6/ref.json" 2>/dev/null
expect_rc "T6 post-compaction passes when the resumed ref still matches" 0 \
  bash "$GUARD" --boundary post-compaction --session-file "$d6/session.json" --ref-file "$d6/ref.json"

jq '.revision = 99' "$d6/ref.json" > "$d6/stale-ref.json"
expect_finding "T6b post-compaction refuses a resumed ref at a different revision" "GF-5" \
  bash "$GUARD" --boundary post-compaction --session-file "$d6/session.json" --ref-file "$d6/stale-ref.json"

jq '.workBoundary.repositoryRoots += ["secondary-repo"]' "$d6/ref.json" > "$d6/widened-ref.json"
expect_finding "T6c post-compaction refuses a resumed ref whose boundary grew" "GF-5" \
  bash "$GUARD" --boundary post-compaction --session-file "$d6/session.json" --ref-file "$d6/widened-ref.json"

# ── B6 pre-certification ────────────────────────────────────────────────────
d7="$(new_case b7)"
expect_rc "T7 pre-certification passes when the report demonstrates the signal" 0 \
  bash "$GUARD" --boundary pre-certification --session-file "$d7/session.json" --spec-dir "$d7/spec"

d7b="$(new_case b7b)"
printf '# Report\n\n## Summary\n\nDid some work.\n' > "$d7b/spec/report.md"
expect_finding "T7b pre-certification refuses a report that never references the Success Signal" "GF-6" \
  bash "$GUARD" --boundary pre-certification --session-file "$d7b/session.json" --spec-dir "$d7b/spec"

d7c="$(new_case b7c)"
rm -f "$d7c/spec/report.md"
expect_finding "T7c pre-certification refuses a spec with no report.md" "GF-6" \
  bash "$GUARD" --boundary pre-certification --session-file "$d7c/session.json" --spec-dir "$d7c/spec"

d7d="$(new_case b7d)"
cat > "$d7d/spec/spec.md" <<'MD'
# Feature

## Outcome Contract

- **Intent**: do the thing.
- **Success Signal**: the selftest exits 0.
MD
expect_finding "T7d pre-certification refuses an Outcome Contract with no Hard Constraints" "G070" \
  bash "$GUARD" --boundary pre-certification --session-file "$d7d/session.json" --spec-dir "$d7d/spec"

# A revision invalidates certification claims that depended on the prior digest.
d7e="$(new_case b7e)"
bash "$GC" revise --session-file "$d7e/session.json" --approval-note "operator approved a wider intent" \
  --intent "Enforce goal fidelity, and also telemetry" >/dev/null 2>&1
expect_finding "T7e pre-certification refuses a spec certified against a superseded revision" "GF-1" \
  bash "$GUARD" --boundary pre-certification --session-file "$d7e/session.json" --spec-dir "$d7e/spec"

# ── Usage contract ──────────────────────────────────────────────────────────
expect_rc "T8 missing --boundary is a usage error" 2 bash "$GUARD" --session-file "$d1/session.json"
expect_rc "T8b an unknown boundary is a usage error" 2 bash "$GUARD" --boundary nonsense
expect_rc "T8c a missing session file is a usage error" 2 \
  bash "$GUARD" --boundary pre-planning --session-file "$TMP_ROOT/absent.json"
expect_rc "T8d --help exits 0" 0 bash "$GUARD" --help
for bypass in --force --skip --ignore --no-verify --skip-boundary; do
  expect_rc "T8e '$bypass' is not accepted (no bypass exists)" 2 \
    bash "$GUARD" --boundary pre-planning --session-file "$d1/session.json" "$bypass"
done
if grep -qE '^\s*--(force|skip|ignore|no-verify)\)' "$GUARD"; then
  fail "T8f goal-fidelity-guard.sh declares a bypass-shaped flag"
else
  pass "T8f goal-fidelity-guard.sh declares no bypass-shaped flag"
fi

# ── --boundary all ──────────────────────────────────────────────────────────
d9="$(new_case b9)"
bash "$GC" ref --session-file "$d9/session.json" > "$d9/ref.json" 2>/dev/null
expect_rc "T9 --boundary all passes on a fully coherent packet" 0 \
  bash "$GUARD" --boundary all --session-file "$d9/session.json" --spec-dir "$d9/spec" \
    --candidate-repo bubbles --candidate-path 'bubbles/scripts/x.sh' \
    --changed-path 'bubbles/scripts/x.sh' --ref-file "$d9/ref.json" --mutable

d9b="$(new_case b9b)"
bash "$GC" ref --session-file "$d9b/session.json" > "$d9b/ref.json" 2>/dev/null
expect_finding "T9b --boundary all still catches an out-of-boundary changed path" "GF-3" \
  bash "$GUARD" --boundary all --session-file "$d9b/session.json" --spec-dir "$d9b/spec" \
    --candidate-repo bubbles --candidate-path 'bubbles/scripts/x.sh' \
    --changed-path 'docs/unrelated.md' --ref-file "$d9b/ref.json" --mutable

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "goal-fidelity-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "goal-fidelity-guard-selftest: all cases passed."
