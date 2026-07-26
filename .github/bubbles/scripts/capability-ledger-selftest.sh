#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

LEDGER_FILE="$ROOT_DIR/bubbles/capability-ledger.yaml"
AGENT_CAPS_FILE="$ROOT_DIR/bubbles/agent-capabilities.yaml"

# --- durable-work-repository-boundary capability contract (IMP-103 S5A) ---
RB_CAPABILITY="durable-work-repository-boundary"
RB_STATE="partial"
RB_RELEASE="unreleased"
RB_OWNER="bubbles/scripts/repository-binding.sh"

# Required evidence provenance. Every entry MUST be declared by the ledger entry
# AND exist on disk. Removing any one is a CAP-RB-EVIDENCE-<id> violation.
RB_REQUIRED_EVIDENCE=(
  bubbles/scripts/repository-binding.sh
  bubbles/scripts/repository-binding-host-context.sh
  bubbles/schemas/repository-binding.schema.json
  agents/bubbles_shared/repository-binding-preflight.md
  bubbles/scripts/repository-binding-selftest.sh
  bubbles/scripts/repository-binding-host-context-selftest.sh
  bubbles/scripts/repository-binding-conformance-guard.sh
  bubbles/scripts/repository-binding-conformance-guard-selftest.sh
  tests/regression/test_repository_binding.sh
  bubbles/registry/gates.yaml
)

# Explicit front-door/shared/runtime consumers that are NOT derivable from
# workflow-mode grants or phase ownership (closed set; changes require a packet
# amendment). Unioned with the mechanically derived agent consumers.
RB_EXPLICIT_CONSUMERS=(
  agents/bubbles.super.agent.md
  agents/bubbles.handoff.agent.md
  agents/bubbles.recap.agent.md
  agents/bubbles.status.agent.md
  agents/bubbles_shared/agent-common.md
  agents/bubbles_shared/operating-baseline.md
  agents/bubbles_shared/repository-binding-preflight.md
  agents/bubbles_shared/workflow-delegation-core.md
  agents/bubbles_shared/workflow-execution-loops.md
  agents/bubbles_shared/workflow-input-bootstrap.md
  agents/bubbles_shared/workflow-phase-engine.md
  agents/bubbles_shared/scenario-compile.md
  skills/bubbles-result-envelope/SKILL.md
  instructions/bubbles-agents.instructions.md
  bubbles/agent-capabilities.yaml
  bubbles/workflows/modes.yaml
  bubbles/schemas/result-envelope.schema.json
  bubbles/scripts/cli.sh
  bubbles/scripts/repository-binding-host-context.sh
  bubbles/scripts/state-snapshot.sh
  bubbles/scripts/context-compactor.sh
  bubbles/scripts/result-envelope-validate.sh
  bubbles/scripts/scenario-compile-lint.sh
  bubbles/scripts/migrate-modes-v5-to-v6.sh
  bubbles/scripts/framework-validate.sh
  bubbles/registry/gates.yaml
)

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

check_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" "$file_path"; then
    pass "$label"
  else
    fail "$label"
  fi
}

# Print the scalar value of <key> inside a two-space capability block.
# Bash 3.2 / awk; no yq, no mapfile.
rb_scalar() {
  local ledger="$1" cap="$2" key="$3"
  awk -v cap="$cap" -v key="$key" '
    $0 == "  " cap ":" { in_cap = 1; next }
    in_cap && /^  [A-Za-z0-9_-]+:[ \t]*$/ { in_cap = 0 }
    in_cap && index($0, "    " key ": ") == 1 {
      value = substr($0, length("    " key ": ") + 1)
      sub(/[ \t]+$/, "", value)
      print value
      exit
    }
  ' "$ledger"
}

# Print the list items of <key> inside a two-space capability block, one per
# line, with inline "# comments" and trailing whitespace stripped (yq-parity).
rb_list() {
  local ledger="$1" cap="$2" key="$3"
  awk -v cap="$cap" -v key="$key" '
    $0 == "  " cap ":" { in_cap = 1; in_key = 0; next }
    in_cap && /^  [A-Za-z0-9_-]+:[ \t]*$/ { in_cap = 0; in_key = 0 }
    in_cap && $0 == "    " key ":" { in_key = 1; next }
    in_cap && in_key && /^    [A-Za-z0-9_-]+:/ { in_key = 0 }
    in_cap && in_key && /^    - / {
      item = $0
      sub(/^    - /, "", item)
      sub(/[ \t]*#.*$/, "", item)
      sub(/[ \t]+$/, "", item)
      print item
    }
  ' "$ledger"
}

# Direct workflow runners: workflowModeGrants.agents -> agents/<name>.agent.md.
rb_direct_runners() {
  awk '
    /^workflowModeGrants:[ \t]*$/ { in_wmg = 1; next }
    in_wmg && /^[A-Za-z0-9_]/ { in_wmg = 0; in_agents = 0 }
    in_wmg && /^  agents:[ \t]*$/ { in_agents = 1; next }
    in_wmg && in_agents && /^    [A-Za-z0-9._-]+:[ \t]*$/ {
      name = $0
      sub(/^    /, "", name)
      sub(/:[ \t]*$/, "", name)
      print "agents/" name ".agent.md"
    }
  ' "$AGENT_CAPS_FILE"
}

# Phase owners: agents.* with a non-empty ownsPhases -> agents/<name>.agent.md.
rb_phase_owners() {
  awk '
    /^agents:[ \t]*$/ { in_top = 1; next }
    in_top && /^[A-Za-z0-9_]/ && !/^agents:/ { in_top = 0 }
    in_top && /^  [A-Za-z0-9._-]+:[ \t]*$/ {
      cur = $0
      sub(/^  /, "", cur)
      sub(/:[ \t]*$/, "", cur)
      next
    }
    in_top && cur != "" && /^    ownsPhases:/ {
      val = $0
      sub(/^    ownsPhases:[ \t]*/, "", val)
      gsub(/[ \t]/, "", val)
      if (val != "[]" && val != "") {
        print "agents/" cur ".agent.md"
      }
    }
  ' "$AGENT_CAPS_FILE"
}

# Sorted union of direct runners, phase owners, and the explicit closed set.
rb_expected_consumers() {
  {
    rb_direct_runners
    rb_phase_owners
    printf '%s\n' "${RB_EXPLICIT_CONSUMERS[@]}"
  } | LC_ALL=C sort -u
}

# Validate the durable-work-repository-boundary contract against <ledger>.
# Emits CAP-RB-* diagnostics; returns non-zero on any violation.
validate_rb_capability() {
  local ledger="$1"
  local rc=0
  local state release owner ref item ledger_evidence ledger_consumers expected

  if ! grep -qxF "  $RB_CAPABILITY:" "$ledger"; then
    echo "CAP-RB-MISSING: capability '$RB_CAPABILITY' is absent from the ledger"
    return 1
  fi

  state="$(rb_scalar "$ledger" "$RB_CAPABILITY" state)"
  if [[ "$state" != "$RB_STATE" ]]; then
    echo "CAP-RB-STATE: state must be '$RB_STATE' but is '${state:-<empty>}'"
    rc=1
  fi

  release="$(rb_scalar "$ledger" "$RB_CAPABILITY" releaseIntroduced)"
  if [[ "$release" != "$RB_RELEASE" ]]; then
    echo "CAP-RB-RELEASE: releaseIntroduced must be '$RB_RELEASE' but is '${release:-<empty>}'"
    rc=1
  fi

  owner="$(rb_scalar "$ledger" "$RB_CAPABILITY" ownerSurface)"
  if [[ "$owner" != "$RB_OWNER" ]]; then
    echo "CAP-RB-OWNER: ownerSurface must be '$RB_OWNER' but is '${owner:-<empty>}'"
    rc=1
  fi

  ledger_evidence="$(rb_list "$ledger" "$RB_CAPABILITY" evidenceRefs)"
  for ref in "${RB_REQUIRED_EVIDENCE[@]}"; do
    if ! printf '%s\n' "$ledger_evidence" | grep -qxF "$ref"; then
      echo "CAP-RB-EVIDENCE-$ref: required evidence reference missing from the ledger"
      rc=1
    elif [[ ! -f "$ROOT_DIR/$ref" ]]; then
      echo "CAP-RB-EVIDENCE-$ref: evidence path does not exist on disk"
      rc=1
    fi
  done

  expected="$(rb_expected_consumers)"
  ledger_consumers="$(rb_list "$ledger" "$RB_CAPABILITY" consumers | LC_ALL=C sort -u)"

  while IFS= read -r item; do
    [[ -n "$item" ]] || continue
    if ! printf '%s\n' "$ledger_consumers" | grep -qxF "$item"; then
      echo "CAP-RB-CONSUMER-$item: required consumer missing from the ledger"
      rc=1
    fi
  done <<EOF
$expected
EOF

  while IFS= read -r item; do
    [[ -n "$item" ]] || continue
    if ! printf '%s\n' "$expected" | grep -qxF "$item"; then
      echo "CAP-RB-CONSUMER-$item: stale consumer is not in the derived union"
      rc=1
    fi
    if [[ ! -f "$ROOT_DIR/$item" ]]; then
      echo "CAP-RB-CONSUMER-$item: consumer path does not exist on disk"
      rc=1
    fi
  done <<EOF
$ledger_consumers
EOF

  return "$rc"
}

# --- adversarial fixture generators (temporary files only) ---

rb_fixture_remove_capability() {
  local ledger="$1" out="$2"
  awk -v cap="$RB_CAPABILITY" '
    $0 == "  " cap ":" { drop = 1; next }
    drop && /^  [A-Za-z0-9_-]+:[ \t]*$/ { drop = 0 }
    drop { next }
    { print }
  ' "$ledger" >"$out"
}

rb_fixture_set_scalar() {
  local ledger="$1" out="$2" key="$3" from="$4" to="$5"
  awk -v cap="$RB_CAPABILITY" -v key="$key" -v from="$from" -v to="$to" '
    $0 == "  " cap ":" { in_cap = 1; print; next }
    in_cap && /^  [A-Za-z0-9_-]+:[ \t]*$/ { in_cap = 0 }
    in_cap && $0 == "    " key ": " from { print "    " key ": " to; next }
    { print }
  ' "$ledger" >"$out"
}

rb_fixture_remove_listitem() {
  local ledger="$1" out="$2" key="$3" target="$4"
  awk -v cap="$RB_CAPABILITY" -v key="$key" -v target="$target" '
    $0 == "  " cap ":" { in_cap = 1; in_key = 0; print; next }
    in_cap && /^  [A-Za-z0-9_-]+:[ \t]*$/ { in_cap = 0; in_key = 0 }
    in_cap && $0 == "    " key ":" { in_key = 1; print; next }
    in_cap && in_key && /^    [A-Za-z0-9_-]+:/ { in_key = 0 }
    in_cap && in_key && $0 == "    - " target { next }
    { print }
  ' "$ledger" >"$out"
}

rb_make_fixture() {
  local kind="$1" member="${2:-}" out="$3"
  case "$kind" in
    clean) cp "$LEDGER_FILE" "$out" ;;
    missing) rb_fixture_remove_capability "$LEDGER_FILE" "$out" ;;
    state) rb_fixture_set_scalar "$LEDGER_FILE" "$out" state "$RB_STATE" shipped ;;
    release) rb_fixture_set_scalar "$LEDGER_FILE" "$out" releaseIntroduced "$RB_RELEASE" v9.9.9 ;;
    evidence) rb_fixture_remove_listitem "$LEDGER_FILE" "$out" evidenceRefs "$member" ;;
    consumer) rb_fixture_remove_listitem "$LEDGER_FILE" "$out" consumers "$member" ;;
    *)
      echo "unknown fixture kind: $kind" >&2
      return 2
      ;;
  esac
}

# Regenerate capability projections from the canonical ledger into an isolated
# temporary root and prove the generator is internally consistent there. The
# canonical generated docs are reconciled in S5B, so the S5A stage must not
# assert their by-design-stale evidence projection.
rb_hermetic_generation_ok() {
  local tmp_root rc=0
  tmp_root="$(mktemp -d)"
  mkdir -p "$tmp_root/bubbles" "$tmp_root/docs/issues" "$tmp_root/docs/generated"
  cp "$ROOT_DIR/bubbles/capability-ledger.yaml" "$tmp_root/bubbles/"
  cp "$ROOT_DIR/bubbles/interop-registry.yaml" "$tmp_root/bubbles/"
  cp "$ROOT_DIR/README.md" "$tmp_root/"
  cp "$ROOT_DIR"/docs/issues/*.md "$tmp_root/docs/issues/" 2>/dev/null || true
  if ! BUBBLES_REPO_ROOT="$tmp_root" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" >/dev/null 2>&1; then
    rc=1
  elif ! BUBBLES_REPO_ROOT="$tmp_root" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check >/dev/null 2>&1; then
    rc=1
  fi
  rm -rf "$tmp_root"
  return "$rc"
}

check_consumers_exist() {
  local capability="$1" label="$2"
  local consumer count=0 consumers

  consumers="$(rb_list "$LEDGER_FILE" "$capability" consumers)"
  if [[ -z "$consumers" ]]; then
    fail "$label declares at least one consumer"
    return 0
  fi

  while IFS= read -r consumer; do
    [[ -n "$consumer" ]] || continue
    count=$((count + 1))
    if [[ -e "$ROOT_DIR/$consumer" ]]; then
      pass "$label consumer path exists: $consumer"
    else
      fail "$label consumer path is missing: $consumer"
    fi
  done <<EOF
$consumers
EOF
  pass "$label declares $count consumer path(s)"
}

# Build a mutated fixture, run the validator, and require it to fail with the
# expected diagnostic token.
expect_rb_violation() {
  local kind="$1" member="$2" token="$3" label="$4"
  local fixture out
  fixture="$(mktemp)"
  rb_make_fixture "$kind" "$member" "$fixture"
  if out="$(validate_rb_capability "$fixture" 2>&1)"; then
    fail "$token adversarial fixture must fail ($label)"
  elif printf '%s\n' "$out" | grep -qF "$token"; then
    pass "$token adversarial fixture rejected ($label)"
  else
    fail "$token adversarial fixture failed for the wrong reason ($label)"
  fi
  rm -f "$fixture"
}

# Optional standalone entrypoints used for red/green demonstration.
case "${1:-}" in
  --rb-validate)
    rb_rc=0
    validate_rb_capability "${2:-$LEDGER_FILE}" || rb_rc=$?
    exit "$rb_rc"
    ;;
  --rb-emit-fixture)
    fixture_path="$(mktemp)"
    if rb_make_fixture "${2:-}" "${3:-}" "$fixture_path"; then
      printf '%s\n' "$fixture_path"
      exit 0
    fi
    rm -f "$fixture_path"
    exit 2
    ;;
esac

echo "Running capability-ledger selftest..."
echo "Scenario: ledger-backed competitive docs stay aligned with the source-of-truth registry."

if rb_hermetic_generation_ok; then
  pass "Capability ledger generates consistent surfaces from the source ledger (hermetic)"
else
  fail "Capability ledger generates consistent surfaces from the source ledger (hermetic)"
fi

check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  workflow-orchestration:$' "Ledger defines workflow orchestration capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  supported-interop-apply:$' "Ledger defines supported interop apply capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  session-aware-runtime-coordination:$' "Ledger defines runtime coordination capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  orchestrator-context-compaction:$' "Ledger defines orchestrator context compaction capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  per-turn-state-snapshot:$' "Ledger defines per-turn state snapshot capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  linter-on-edit-gate:$' "Ledger defines linter-on-edit gate capability"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '^State summary: [0-9]+ shipped, [0-9]+ partial, [0-9]+ proposed\.$' "Generated capability guide exposes generated state summary"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  workflow-runner-authorization:$' "Ledger defines workflow runner authorization capability"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Workflow orchestration \| shipped \|' "Generated capability guide includes shipped workflow orchestration row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Workflow runner authorization \| shipped \|' "Generated capability guide includes workflow runner authorization row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Supported interop apply \| shipped \|' "Generated capability guide includes shipped supported interop apply row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Session-aware runtime coordination \| shipped \|' "Generated capability guide includes shipped runtime coordination row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Orchestrator context compaction \| shipped \|' "Generated capability guide includes shipped orchestrator context compaction row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Per-turn state snapshot \| shipped \|' "Generated capability guide includes shipped per-turn state snapshot row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Linter-on-edit gate \| shipped \|' "Generated capability guide includes shipped linter-on-edit gate row"
check_pattern "$ROOT_DIR/docs/generated/issue-status.md" '^Tracked gaps: 2 issue-backed capabilities\.$' "Generated issue status guide counts tracked gaps from the ledger"
check_pattern "$ROOT_DIR/docs/generated/interop-migration-matrix.md" '\| Claude Code \| markdown \|' "Generated migration matrix is refreshed from the interop registry"

check_consumers_exist "observability-adapter-contract" "Observability adapter contract"
check_consumers_exist "observability-posture-and-slo-gates" "Observability posture/SLO gates"

# --- durable-work-repository-boundary enforcement (IMP-103 S5A) ---
# The clean canonical ledger satisfies the full capability contract.
if validate_rb_capability "$LEDGER_FILE" >/dev/null 2>&1; then
  pass "durable-work-repository-boundary clean ledger satisfies the capability contract"
else
  fail "durable-work-repository-boundary clean ledger satisfies the capability contract"
  validate_rb_capability "$LEDGER_FILE" || true
fi

# Adversarial: omission/mutation fixtures each fail for their named reason.
expect_rb_violation missing "" CAP-RB-MISSING "capability removed"
expect_rb_violation state "" CAP-RB-STATE "state is not partial"
expect_rb_violation release "" CAP-RB-RELEASE "releaseIntroduced is not unreleased"

# Every required evidence reference removed in turn.
for rb_ref in "${RB_REQUIRED_EVIDENCE[@]}"; do
  expect_rb_violation evidence "$rb_ref" "CAP-RB-EVIDENCE-$rb_ref" "evidence removed: $rb_ref"
done

# Every derived + explicit consumer removed in turn.
while IFS= read -r rb_consumer; do
  [[ -n "$rb_consumer" ]] || continue
  expect_rb_violation consumer "$rb_consumer" "CAP-RB-CONSUMER-$rb_consumer" "consumer removed: $rb_consumer"
done <<EOF
$(rb_expected_consumers)
EOF

if [[ "$failures" -gt 0 ]]; then
  echo "capability-ledger selftest failed with $failures issue(s)."
  exit 1
fi

echo "capability-ledger selftest passed."
