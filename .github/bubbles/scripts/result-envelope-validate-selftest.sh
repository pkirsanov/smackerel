#!/usr/bin/env bash
#
# bubbles result-envelope-validate-selftest.sh (v6.0 / B3).
#
# Asserts the v6.0 promotion of the envelope validator:
#   1. v5.2 ADVISORY mode still works (--advisory) — exit 0 even when
#      malformed envelopes exist (backwards compatible).
#   2. v6.0 DEFAULT mode (--no-args) blocks on malformed envelopes
#      but still warns on MISSING envelopes (gradual rollout).
#   3. v6.1 STRICT mode (--strict) blocks on missing OR malformed.
#   4. Schema accepts `nextRequiredOwner` as an alias for `nextOwner`
#      when outcome == route_required.
#   5. Schema accepts `blockedReason` as an alias for `blocker.reason`
#      when outcome == blocked.
#   6. Schema accepts additional properties (richer envelope shape).
#   7. A fixture with a deliberately broken `outcome` value FAILS in
#      every mode (truly invalid envelope is always blocking).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
VALIDATOR="$SCRIPT_DIR/result-envelope-validate.sh"
SCHEMA="$REPO_ROOT/bubbles/schemas/result-envelope.schema.json"

[[ -x "$VALIDATOR" ]] || { echo "selftest: missing $VALIDATOR" >&2; exit 2; }
[[ -f "$SCHEMA" ]] || { echo "selftest: missing $SCHEMA" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "result-envelope-validate-selftest: SKIP (python3 not installed)"; exit 0; }
# The validator itself SKIPs gracefully when jsonschema is absent (exit 0). The
# selftest exercises the validator's schema behavior, which cannot run without
# jsonschema — SKIP to match the framework's graceful-degradation convention
# (see model-tier-advisory-selftest.sh).
python3 -c "import jsonschema" 2>/dev/null || { echo "result-envelope-validate-selftest: SKIP (python jsonschema not installed)"; exit 0; }

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

TEST_ROOT_BASE="${HOME}/.cache/bubbles-result-envelope-selftest"
mkdir -p "$TEST_ROOT_BASE"
# A template inside the base directory, not `-p`: the parent-directory flag is
# GNU-only and BSD mktemp rejects it, which took this selftest down on macOS.
TEST_ROOT="$(mktemp -d "$TEST_ROOT_BASE/run.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

# Re-implement run_validator without AGENTS_DIR env override since the
# script resolves it from $REPO_ROOT, not from env. Use a wrapper script.
WRAPPER="$TEST_ROOT/wrapper.sh"
cat > "$WRAPPER" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$1"
shift
exec env BUBBLES_REPO_ROOT="$REPO_ROOT" bash "$SCRIPT_DIR/result-envelope-validate.sh" "$@"
SH

# Simpler approach: invoke the validator directly with REPO_ROOT pointed at
# a synthetic tree that contains both bubbles/schemas/ and agents/.

make_fixture_repo() {
  local label="$1"; shift
  local agents_content_block="$1"; shift   # Full body of the agent file
  local repo="$TEST_ROOT/$label"
  mkdir -p "$repo/agents" "$repo/bubbles/schemas" "$repo/bubbles/scripts"
  cp "$SCHEMA" "$repo/bubbles/schemas/result-envelope.schema.json"
  cp "$VALIDATOR" "$repo/bubbles/scripts/result-envelope-validate.sh"
  chmod +x "$repo/bubbles/scripts/result-envelope-validate.sh"
  cat > "$repo/agents/bubbles.testagent.agent.md" <<EOF
$agents_content_block
EOF
  echo "$repo"
}

run_fixture() {
  local repo="$1"; shift
  set +e
  OUT="$(bash "$repo/bubbles/scripts/result-envelope-validate.sh" "$@" 2>&1)"
  RC=$?
  set -e
}

# Fixture A: agent has a VALID envelope.
ENV_VALID=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "summary": "did the thing"\n}\n```\n'
# Fixture B: agent has NO envelope.
ENV_NONE=$'# Agent without envelope\n\nNo envelope here.\n'
# Fixture C: agent has a MALFORMED envelope (route_required without nextOwner / nextRequiredOwner).
ENV_MALFORMED=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "route_required",\n  "summary": "needs routing but no nextOwner"\n}\n```\n'
# Fixture D: agent uses nextRequiredOwner (alias) for route_required — should pass.
ENV_ALIAS_NRO=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "route_required",\n  "summary": "routing with alias field",\n  "nextRequiredOwner": "bubbles.plan"\n}\n```\n'
# Fixture E: agent uses blockedReason (alias) for blocked outcome.
ENV_ALIAS_BR=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "blocked",\n  "summary": "blocked with alias field",\n  "blockedReason": "missing dep"\n}\n```\n'
# Fixture F: agent has extra (richer) properties — should pass with additionalProperties:true.
ENV_RICHER=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "roleClass": "implementer",\n  "featureDir": "specs/042-foo",\n  "scopeIds": ["01-bar"],\n  "dodItems": ["DOD-01-02"],\n  "artifactsUpdated": ["report.md"]\n}\n```\n'
# Fixture G: agent has invalid outcome value — should ALWAYS fail.
ENV_INVALID=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "not_a_real_outcome"\n}\n```\n'

# Build fixtures.
repo_valid="$(make_fixture_repo valid "$ENV_VALID")"
repo_none="$(make_fixture_repo none "$ENV_NONE")"
repo_malformed="$(make_fixture_repo malformed "$ENV_MALFORMED")"
repo_nro="$(make_fixture_repo nro "$ENV_ALIAS_NRO")"
repo_br="$(make_fixture_repo br "$ENV_ALIAS_BR")"
repo_richer="$(make_fixture_repo richer "$ENV_RICHER")"
repo_invalid="$(make_fixture_repo invalid "$ENV_INVALID")"

# 1. Advisory mode — malformed still passes (exit 0).
run_fixture "$repo_malformed" --advisory
if [[ "$RC" -eq 0 ]]; then
  pass "1. advisory mode (--advisory) exits 0 even with malformed envelope"
else
  fail "1. advisory mode: rc=$RC out=$OUT"
fi

# 2. v6 default — malformed BLOCKS, missing still warns.
run_fixture "$repo_malformed"
if [[ "$RC" -ne 0 ]] && grep -q MALFORMED <<< "$OUT"; then
  pass "2a. v6 default (no flags) blocks on malformed envelope"
else
  fail "2a. v6 default malformed: rc=$RC out=$OUT"
fi
run_fixture "$repo_none"
if [[ "$RC" -eq 0 ]] && grep -q -E 'Advisory|missing' <<< "$OUT"; then
  pass "2b. v6 default still warns (not blocks) on missing envelope"
else
  fail "2b. v6 default missing: rc=$RC out=$OUT"
fi

# 3. Strict mode — missing OR malformed blocks.
run_fixture "$repo_none" --strict
if [[ "$RC" -ne 0 ]]; then
  pass "3a. strict mode blocks on missing envelope"
else
  fail "3a. strict missing: rc=$RC out=$OUT"
fi
run_fixture "$repo_malformed" --strict
if [[ "$RC" -ne 0 ]]; then
  pass "3b. strict mode blocks on malformed envelope"
else
  fail "3b. strict malformed: rc=$RC out=$OUT"
fi

# 4. Schema accepts nextRequiredOwner alias.
run_fixture "$repo_nro"
if [[ "$RC" -eq 0 ]]; then
  pass "4. schema accepts nextRequiredOwner as alias for nextOwner"
else
  fail "4. nextRequiredOwner alias: rc=$RC out=$OUT"
fi

# 5. Schema accepts blockedReason alias.
run_fixture "$repo_br"
if [[ "$RC" -eq 0 ]]; then
  pass "5. schema accepts blockedReason as alias for blocker.reason"
else
  fail "5. blockedReason alias: rc=$RC out=$OUT"
fi

# 6. Schema accepts additional properties.
run_fixture "$repo_richer"
if [[ "$RC" -eq 0 ]]; then
  pass "6. schema accepts richer envelope shape (additionalProperties:true)"
else
  fail "6. richer envelope: rc=$RC out=$OUT"
fi

# 7. Invalid outcome value FAILS in every mode.
run_fixture "$repo_invalid"
if [[ "$RC" -ne 0 ]]; then
  pass "7a. v6 default rejects invalid outcome value"
else
  fail "7a. invalid outcome default: rc=$RC out=$OUT"
fi
run_fixture "$repo_invalid" --strict
if [[ "$RC" -ne 0 ]]; then
  pass "7b. strict rejects invalid outcome value"
else
  fail "7b. invalid outcome strict: rc=$RC out=$OUT"
fi
# advisory still warns but exits 0 — that's by design (advisory never blocks).
run_fixture "$repo_invalid" --advisory
if [[ "$RC" -eq 0 ]] && grep -q MALFORMED <<< "$OUT"; then
  pass "7c. advisory still reports invalid outcome but exits 0 (by design)"
else
  fail "7c. invalid outcome advisory: rc=$RC out=$OUT"
fi

# 8. Valid envelope always passes.
run_fixture "$repo_valid"
if [[ "$RC" -eq 0 ]]; then
  pass "8. valid envelope passes in v6 default mode"
else
  fail "8. valid envelope: rc=$RC out=$OUT"
fi

# 9. IMP-038 SCOPE-4 / GF-3 — routing is not resolution.
# Each fixture below is schema-VALID, so it would sail through without the
# accounting check. That is the point: these are the shapes that read as a
# clean, closed result while the work was actually handed to someone else.
ENV_ROUTED_AS_ADDRESSED=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "summary": "claimed closure on a routed finding",\n  "findings": [\n    { "id": "F-1", "severity": "warn", "owner": "bubbles.devops", "summary": "flaky deploy step", "disposition": "routed", "filedArtifact": "specs/_ops/OPS-7/ops.md", "goalImpact": "independent" }\n  ],\n  "addressedFindings": ["F-1"]\n}\n```\n'
ENV_BLOCKING_AS_ADDRESSED=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "summary": "claimed closure on a blocking-external finding",\n  "findings": [\n    { "id": "F-2", "severity": "blocker", "owner": "bubbles.devops", "summary": "upstream contract breaks the success signal", "disposition": "bug-filed", "filedArtifact": "specs/x/bugs/BUG-9/bug.md", "goalImpact": "blocking-external" }\n  ],\n  "addressedFindings": ["F-2"]\n}\n```\n'
ENV_INDEPENDENT_UNFILED=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "summary": "independent finding with nothing filed",\n  "findings": [\n    { "id": "F-3", "severity": "warn", "owner": "bubbles.docs", "summary": "stale guide section", "goalImpact": "independent" }\n  ],\n  "unresolvedFindings": ["F-3"]\n}\n```\n'
ENV_ROUTED_UNRESOLVED_OK=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "route_required",\n  "summary": "routed finding accounted honestly",\n  "nextRequiredOwner": "bubbles.devops",\n  "findings": [\n    { "id": "F-4", "severity": "warn", "owner": "bubbles.devops", "summary": "flaky deploy step", "disposition": "routed", "filedArtifact": "specs/_ops/OPS-7/ops.md", "goalImpact": "independent" }\n  ],\n  "unresolvedFindings": ["F-4"]\n}\n```\n'
ENV_REQUIRED_FIXED_OK=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "summary": "required finding actually fixed",\n  "findings": [\n    { "id": "F-5", "severity": "blocker", "owner": "bubbles.implement", "summary": "in-boundary defect blocking the contract", "disposition": "fixed-in-session", "goalImpact": "required" }\n  ],\n  "addressedFindings": ["F-5"]\n}\n```\n'
ENV_LEGACY_NO_IMPACT=$'# Agent\n\n## RESULT-ENVELOPE\n\n```json\n{\n  "agent": "bubbles.testagent",\n  "outcome": "completed_owned",\n  "summary": "pre-IMP-038 envelope with no goalImpact",\n  "findings": [\n    { "id": "F-6", "severity": "warn", "owner": "bubbles.implement", "summary": "legacy finding shape" }\n  ],\n  "addressedFindings": ["F-6"]\n}\n```\n'

repo_routed_addressed="$(make_fixture_repo routed_addressed "$ENV_ROUTED_AS_ADDRESSED")"
repo_blocking_addressed="$(make_fixture_repo blocking_addressed "$ENV_BLOCKING_AS_ADDRESSED")"
repo_independent_unfiled="$(make_fixture_repo independent_unfiled "$ENV_INDEPENDENT_UNFILED")"
repo_routed_ok="$(make_fixture_repo routed_ok "$ENV_ROUTED_UNRESOLVED_OK")"
repo_required_ok="$(make_fixture_repo required_ok "$ENV_REQUIRED_FIXED_OK")"
repo_legacy="$(make_fixture_repo legacy_no_impact "$ENV_LEGACY_NO_IMPACT")"

run_fixture "$repo_routed_addressed"
if [[ "$RC" -ne 0 ]] && grep -q 'routing hands work to an owner' <<< "$OUT"; then
  pass "9a. a routed finding reported as addressed is refused"
else
  fail "9a. routed-as-addressed: rc=$RC out=$OUT"
fi

run_fixture "$repo_blocking_addressed"
if [[ "$RC" -ne 0 ]] && grep -q 'blocking-external' <<< "$OUT"; then
  pass "9b. a blocking-external finding reported as addressed is refused"
else
  fail "9b. blocking-as-addressed: rc=$RC out=$OUT"
fi

run_fixture "$repo_independent_unfiled"
if [[ "$RC" -ne 0 ]] && grep -q 'names no filedArtifact' <<< "$OUT"; then
  pass "9c. an independent finding with no filed artifact is refused as undischarged"
else
  fail "9c. independent-unfiled: rc=$RC out=$OUT"
fi

run_fixture "$repo_routed_ok"
if [[ "$RC" -eq 0 ]]; then
  pass "9d. a routed finding kept in unresolvedFindings with its artifact passes"
else
  fail "9d. routed-honest: rc=$RC out=$OUT"
fi

run_fixture "$repo_required_ok"
if [[ "$RC" -eq 0 ]]; then
  pass "9e. a required finding genuinely fixed in session passes as addressed"
else
  fail "9e. required-fixed: rc=$RC out=$OUT"
fi

run_fixture "$repo_legacy"
if [[ "$RC" -eq 0 ]]; then
  pass "9f. a pre-IMP-038 finding with no goalImpact still validates (additive)"
else
  fail "9f. legacy finding shape: rc=$RC out=$OUT"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "result-envelope-validate-selftest: FAIL ($failures issue(s))"
  exit 1
fi
echo "result-envelope-validate-selftest: PASS"
exit 0
