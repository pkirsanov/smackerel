#!/usr/bin/env bash
# Hermetic selftest for phase-relevance-resolve.sh (IMP-038 SCOPE-5 / GF-4,
# with the SCOPE-7 adversarial cases).
#
# The resolver exists to make ONE registry verdict available to every top-level
# runner. Its danger is the opposite of its purpose: a resolver that skips too
# eagerly deletes assurance silently. Every case below therefore fails if a
# phase would be skipped for a reason the caller never evidenced.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESOLVER="$SCRIPT_DIR/phase-relevance-resolve.sh"
LIVE_MODES="$REPO_ROOT/bubbles/workflows/modes.yaml"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

if ! command -v yq >/dev/null 2>&1; then
  echo "phase-relevance-resolve-selftest: SKIP (yq not installed)"
  exit 0
fi
[[ -f "$RESOLVER" ]] || { echo "FAIL: $RESOLVER not found" >&2; exit 1; }

echo "Running phase-relevance-resolve selftest..."

# field <line-key> <resolver args...>
field() {
  local key="$1"; shift
  bash "$RESOLVER" "$@" 2>/dev/null | sed -n "s/^${key}=//p"
}

# expect <label> <key> <want> <resolver args...>
expect() {
  local label="$1" key="$2" want="$3"; shift 3
  local got
  got="$(field "$key" "$@")"
  if [[ "$got" == "$want" ]]; then
    pass "$label"
  else
    fail "$label (${key}: observed '$got', expected '$want')"
  fi
}

expect_rc() {
  local label="$1" want="$2"; shift 2
  local rc=0
  "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then
    pass "$label"
  else
    fail "$label (expected exit $want, got $rc)"
  fi
}

# ── T1 neverSkip is absolute ────────────────────────────────────────────────
# Read the live list rather than restating it, so adding a neverSkip phase to
# the registry extends this test automatically instead of leaving a gap.
mapfile -t never_skip < <(yq -r '.modes.phaseRelevance.neverSkip // [] | .[]' "$LIVE_MODES")
if [[ "${#never_skip[@]}" -eq 0 ]]; then
  fail "T1 registry declares no neverSkip phases — the safety floor is missing"
else
  never_skip_ok="true"
  for ph in "${never_skip[@]}"; do
    [[ -n "$ph" ]] || continue
    if [[ "$(field verdict --phase "$ph")" != "run" ]]; then
      fail "T1 neverSkip phase '$ph' did not resolve to run"
      never_skip_ok="false"
    fi
    if [[ "$(field rule --phase "$ph")" != "neverSkip" ]]; then
      fail "T1 neverSkip phase '$ph' did not report the neverSkip rule"
      never_skip_ok="false"
    fi
  done
  [[ "$never_skip_ok" == "true" ]] \
    && pass "T1 every registry neverSkip phase (${#never_skip[@]}) resolves to run"
fi

# ── T2 a phase with no rule always runs ─────────────────────────────────────
expect "T2 an undeclared phase resolves to run" verdict run --phase not-a-real-phase
expect "T2b an undeclared phase reports no-rule" rule no-rule --phase not-a-real-phase

# ── T3 fail-safe: an unimplemented skipWhen token must NOT skip ─────────────
# This is the case that decides whether the resolver can quietly delete a phase.
# A new registry rule with no evaluator MUST resolve to run and say so.
unknown_modes="$TMP_ROOT/unknown-token.yaml"
cat > "$unknown_modes" <<'YAML'
modes:
  phaseRelevance:
    enabled: true
    rules:
    - phase: chaos
      skipWhen: some_token_no_evaluator_implements
      reason: "would skip chaos"
    neverSkip:
    - validate
YAML
expect "T3 a skipWhen token with no evaluator resolves to run" verdict run \
  --phase chaos
BUBBLES_MODES_FILE="$unknown_modes"
export BUBBLES_MODES_FILE
expect "T3b an unimplemented token resolves to run, not skip" verdict run --phase chaos
expect "T3c the unimplemented token is named in the rule field" \
  rule "unevaluated:some_token_no_evaluator_implements" --phase chaos
unset BUBBLES_MODES_FILE

# ── T4 fail-safe: a missing input must NOT skip ─────────────────────────────
expect "T4 simplify with no --changed-lines resolves to run" verdict run --phase simplify
expect "T4b the unevaluated rule is named, not silently dropped" \
  rule scope_changed_fewer_than_50_lines --phase simplify
expect "T4c chaos with no changed surface resolves to run" verdict run --phase chaos
expect "T4d stabilize with no --spec-dir resolves to run" verdict run --phase stabilize
expect "T4e security with no changed surface resolves to run" verdict run --phase security
expect "T4f regression with no --spec-dir resolves to run" verdict run --phase regression

# ── T5 line-count evaluator ─────────────────────────────────────────────────
expect "T5 simplify skips below the 50-line threshold" verdict skip --phase simplify --changed-lines 12
expect "T5b simplify runs at exactly the threshold" verdict run --phase simplify --changed-lines 50
expect "T5c simplify runs above the threshold" verdict run --phase simplify --changed-lines 500
expect "T5d simplify runs at zero changed lines only via the rule, not by accident" \
  rule scope_changed_fewer_than_50_lines --phase simplify --changed-lines 0
expect_rc "T5e a non-numeric --changed-lines is a usage error" 2 \
  bash "$RESOLVER" --phase simplify --changed-lines twelve

# ── T6 changed-surface evaluators ───────────────────────────────────────────
docs_only="$TMP_ROOT/docs-only.txt"
printf 'docs/guides/X.md\nREADME.md\nconfig/app.yaml\n' > "$docs_only"
with_code="$TMP_ROOT/with-code.txt"
printf 'docs/guides/X.md\nsrc/main.rs\n' > "$with_code"
ci_change="$TMP_ROOT/ci.txt"
printf '.github/workflows/build.yml\n' > "$ci_change"
infra_change="$TMP_ROOT/infra.txt"
printf 'deploy/compose.deploy.yml\n' > "$infra_change"
auth_change="$TMP_ROOT/auth.txt"
printf 'src/auth/login.rs\n' > "$auth_change"
empty_surface="$TMP_ROOT/empty.txt"
: > "$empty_surface"

expect "T6 chaos skips for a docs/config-only surface" verdict skip \
  --phase chaos --changed-surface-file "$docs_only"
expect "T6b chaos runs as soon as one source file changed" verdict run \
  --phase chaos --changed-surface-file "$with_code"
expect "T6c devops runs for a CI workflow change" verdict run \
  --phase devops --changed-surface-file "$ci_change"
expect "T6d devops runs for a deploy-directory change" verdict run \
  --phase devops --changed-surface-file "$infra_change"
expect "T6e devops skips when nothing infra-shaped changed" verdict skip \
  --phase devops --changed-surface-file "$with_code"
expect "T6f security runs for an auth-path change" verdict run \
  --phase security --changed-surface-file "$auth_change"
expect "T6g security skips for a docs-only surface" verdict skip \
  --phase security --changed-surface-file "$docs_only"
# An empty file declares nothing; that is missing input, not a docs-only scope.
expect "T6h an EMPTY changed-surface file resolves to run, not skip" verdict run \
  --phase chaos --changed-surface-file "$empty_surface"

# ── T7 artifact evaluators ──────────────────────────────────────────────────
sla_spec="$TMP_ROOT/specs/001-sla"
mkdir -p "$sla_spec"
printf '# Feature\n\nThe endpoint must hold a p95 latency under 200ms.\n' > "$sla_spec/spec.md"
clean_spec="$TMP_ROOT/specs/002-clean"
mkdir -p "$clean_spec"
printf '# Feature\n\nA slot machine on a slate background, announced in Slack.\n' > "$clean_spec/spec.md"
ambig_spec="$TMP_ROOT/specs/003-ambiguous"
mkdir -p "$ambig_spec"
printf '# Feature\n\nThe auth model is TBD.\n' > "$ambig_spec/spec.md"

expect "T7 stabilize runs when the spec declares an SLA" verdict run \
  --phase stabilize --spec-dir "$sla_spec"
# The word boundaries on sla/slo are load-bearing; without them these three
# ordinary words read as SLA declarations and stabilize stops skipping.
expect "T7b stabilize still skips for slot/slate/Slack (no false SLA match)" verdict skip \
  --phase stabilize --spec-dir "$clean_spec"
expect "T7c interrogate runs when the spec carries an ambiguity marker" verdict run \
  --phase interrogate --spec-dir "$ambig_spec"

grill_off="$TMP_ROOT/grill-off.json"
printf '{"executionOptions":{"grillMode":"off"}}' > "$grill_off"
grill_on="$TMP_ROOT/grill-on.json"
printf '{"executionOptions":{"grillMode":"on-demand"}}' > "$grill_on"
expect "T7d interrogate skips a clean spec under grillMode off" verdict skip \
  --phase interrogate --spec-dir "$clean_spec" --session-file "$grill_off"
expect "T7e interrogate still runs on an ambiguous spec even with grillMode on-demand" verdict run \
  --phase interrogate --spec-dir "$ambig_spec" --session-file "$grill_on"

echo '{"version":3}' > "$clean_spec/state.json"
expect "T7f regression runs when a prior spec exists" verdict run \
  --phase regression --spec-dir "$sla_spec"
rm -f "$clean_spec/state.json" "$ambig_spec/state.json"
expect "T7g regression skips when no prior spec exists" verdict skip \
  --phase regression --spec-dir "$sla_spec"

# ── T8 one verdict for every runner (SCOPE-7) ───────────────────────────────
# The defect this resolver removes is four runners reaching four verdicts for
# one scope. --runner is recorded for audit and MUST NOT change the decision.
runner_ok="true"
for ph in simplify chaos security devops stabilize regression interrogate validate audit; do
  base=""
  for rn in bubbles.goal bubbles.sprint bubbles.iterate bubbles.workflow; do
    v="$(field verdict --phase "$ph" --runner "$rn" \
      --changed-surface-file "$with_code" --changed-lines 12 \
      --spec-dir "$sla_spec" --session-file "$grill_off")"
    if [[ -z "$base" ]]; then
      base="$v"
    elif [[ "$v" != "$base" ]]; then
      fail "T8 runner '$rn' disagreed on phase '$ph': '$v' vs '$base'"
      runner_ok="false"
    fi
  done
done
[[ "$runner_ok" == "true" ]] \
  && pass "T8 goal, sprint, iterate, and workflow receive identical verdicts for the same scope"

# The runner must still be visible in the reason, or a skip cannot be audited.
if field reason --phase simplify --changed-lines 12 --runner bubbles.sprint | grep -q 'bubbles.sprint'; then
  pass "T8b the deciding runner is recorded in the reason for audit"
else
  fail "T8b the runner must be recorded in the reason"
fi

# ── T8c published guidance may not out-run the wired runners (SCOPE-7) ──────
# WORKFLOW_MODES.md previously claimed that "the active authorized runner"
# applies smart phase routing while only ONE runner had ever been given the
# contract. This case fails if the published claim ever again names a runner
# that cannot actually reach the resolver's obligation.
#
# Reachability is what matters, not a literal mention: a runner is wired when
# its agent file loads a shared doc that carries the mandate. bubbles.workflow
# reaches it through workflow-phase-engine.md; goal, sprint and iterate reach
# it through operating-baseline.md. A runner that loads neither is unwired even
# if the guide names it.
claim_doc="$REPO_ROOT/docs/guides/WORKFLOW_MODES.md"
if [[ -f "$claim_doc" ]]; then
  mandate_docs=()
  for shared in operating-baseline workflow-phase-engine; do
    f="$REPO_ROOT/agents/bubbles_shared/$shared.md"
    [[ -f "$f" ]] && grep -q 'phase-relevance-resolve.sh' "$f" && mandate_docs+=("$shared")
  done
  claim_ok="true"
  if [[ "${#mandate_docs[@]}" -eq 0 ]]; then
    fail "T8c no shared doc carries the phase-relevance mandate, so no runner is wired"
    claim_ok="false"
  fi
  for rn in bubbles.goal bubbles.sprint bubbles.iterate bubbles.workflow; do
    agent_file="$REPO_ROOT/agents/$rn.agent.md"
    claimed="no"; grep -q "$rn" "$claim_doc" && claimed="yes"
    wired="no"
    if [[ -f "$agent_file" ]]; then
      for shared in "${mandate_docs[@]}"; do
        grep -q "bubbles_shared/$shared.md" "$agent_file" && { wired="yes"; break; }
      done
    fi
    if [[ "$claimed" == "yes" && "$wired" == "no" ]]; then
      fail "T8c WORKFLOW_MODES.md claims $rn applies phase relevance, but $rn loads no shared doc carrying the mandate"
      claim_ok="false"
    fi
    if [[ "$wired" == "no" ]]; then
      fail "T8c $rn is an authorized top-level runner but cannot reach the phase-relevance mandate"
      claim_ok="false"
    fi
  done
  [[ "$claim_ok" == "true" ]] \
    && pass "T8c every runner the guide names can actually reach the resolver mandate"
else
  fail "T8c $claim_doc not found — the published claim cannot be checked"
fi

# ── T9 the registry is the source, not this script ──────────────────────────
# Flipping enabled:false in a substitute registry must make every phase run.
disabled_modes="$TMP_ROOT/disabled.yaml"
cat > "$disabled_modes" <<'YAML'
modes:
  phaseRelevance:
    enabled: false
    rules:
    - phase: simplify
      skipWhen: scope_changed_fewer_than_50_lines
      reason: "would skip"
    neverSkip: []
YAML
BUBBLES_MODES_FILE="$disabled_modes"
export BUBBLES_MODES_FILE
expect "T9 a disabled registry makes every phase run" verdict run \
  --phase simplify --changed-lines 1
unset BUBBLES_MODES_FILE

# A rule moved to a different phase must follow the registry, not a hardcoded
# assumption about which phase owns which token.
moved_modes="$TMP_ROOT/moved.yaml"
cat > "$moved_modes" <<'YAML'
modes:
  phaseRelevance:
    enabled: true
    rules:
    - phase: docs
      skipWhen: scope_changed_fewer_than_50_lines
      reason: "registry says docs, not simplify"
    neverSkip: []
YAML
BUBBLES_MODES_FILE="$moved_modes"
export BUBBLES_MODES_FILE
expect "T9b a rule reassigned to another phase follows the registry" verdict skip \
  --phase docs --changed-lines 3
expect "T9c the phase that lost the rule now runs" verdict run \
  --phase simplify --changed-lines 3
expect "T9d the registry's own reason text is returned, not a restated one" \
  reason "registry says docs, not simplify" --phase docs --changed-lines 3
unset BUBBLES_MODES_FILE

# neverSkip must beat a rule that would otherwise fire — proven with a registry
# that declares BOTH for the same phase.
conflict_modes="$TMP_ROOT/conflict.yaml"
cat > "$conflict_modes" <<'YAML'
modes:
  phaseRelevance:
    enabled: true
    rules:
    - phase: audit
      skipWhen: scope_changed_fewer_than_50_lines
      reason: "a rule that must lose to neverSkip"
    neverSkip:
    - audit
YAML
BUBBLES_MODES_FILE="$conflict_modes"
export BUBBLES_MODES_FILE
expect "T10 neverSkip beats a rule that would otherwise fire" verdict run \
  --phase audit --changed-lines 1
expect "T10b the winning rule is reported as neverSkip" rule neverSkip \
  --phase audit --changed-lines 1
unset BUBBLES_MODES_FILE

# ── T11 usage contract ──────────────────────────────────────────────────────
expect_rc "T11 missing --phase is a usage error" 2 bash "$RESOLVER"
expect_rc "T11b an unreadable registry is a usage error" 2 \
  env BUBBLES_MODES_FILE="$TMP_ROOT/absent.yaml" bash "$RESOLVER" --phase simplify
expect_rc "T11c --help exits 0" 0 bash "$RESOLVER" --help
for bypass in --force --skip --ignore --no-verify --skip-phase; do
  expect_rc "T11d '$bypass' is not accepted (no bypass exists)" 2 \
    bash "$RESOLVER" --phase audit "$bypass"
done
if grep -Eq -- '--(force|skip-phase|ignore|no-verify)\)' "$RESOLVER"; then
  fail "T11e the resolver declares a bypass-shaped flag"
else
  pass "T11e the resolver declares no bypass-shaped flag"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "phase-relevance-resolve-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "phase-relevance-resolve-selftest: all cases passed."
