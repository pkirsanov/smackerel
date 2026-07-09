#!/usr/bin/env bash
set -uo pipefail

# observability-slo-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/observability-slo-guard.sh`
# (Gate G100 — observability_slo_evidence_gate).
#
# Stages throwaway repo surfaces, each carrying:
#   * `.github/bubbles-project.yaml`  (posture + slos contract + workflow slo: link)
#   * `specs/<f>/scopes.md`           (a Test Plan row declaring observabilityWorkflow)
#   * `.specify/runtime/observability/<workflow>.slo.json`  (captured metric evidence)
# and asserts the guard's exit codes + message fingerprints across every state.
#
# NOTE on workspace location: the reference `yq` is frequently snap-confined
# (strict confinement cannot read `/tmp`). Like the posture/opt-out guard
# selftests, this selftest stages its hermetic workspace UNDER $HOME so snap-yq
# can read the fixtures. Staging files inside a throwaway $HOME workspace is
# allowed by terminal-discipline policy (the workspace never becomes part of the
# working tree).
#
# Cases (matches IMP-001 SCOPE-4 T4.6):
#   within-target            → exit 0, captured evidence within target
#   breached                 → exit 1, SLO BREACH (latency over target)
#   opted-out                → exit 0, clean no-op
#   no-observabilityWorkflow → exit 0, no-op (wired + slo: link but no instrumented scope)
#   missing-evidence         → exit 1, gap (instrumented workflow, no evidence file)
#   malformed-json           → exit 1, fail loud (unparseable JSON) before numeric compare
#   malformed-no-observed    → exit 1, fail loud (missing `observed` block) before compare
#   wrong-workflow           → exit 1, fail loud (evidence for a different workflow)
#   missing-parser           → exit 1, FAIL CLOSED (PATH stripped of jq/yq)
#   framework-repo-exempt    → exit 0, EXEMPT (no-runtime), no enforcement
#
# SPEC-ATTRIBUTION SCOPING (Gate G100 refinement — mirrors G090) — --spec-dir
# scopes instrumentation attribution to ONE transitioning spec's own artifacts:
#   spec-scoped-teeth        → exit 1, an instrumented-with-slo spec with absent
#                              evidence STILL BLOCKS under its own --spec-dir
#   spec-scoped-false-block  → exit 0, a spec declaring NO observabilityWorkflow
#                              no-ops under --spec-dir EVEN WHEN an unrelated spec
#                              instruments a workflow whose evidence is absent
#   spec-scoped-backcompat   → exit 1, the SAME fixture with NO --spec-dir
#                              (repo-wide) still BLOCKS (standalone/CI teeth kept)
#
# ADVERSARIAL-OBSERVABILITY (R2-G) — a trace/SLO check is only trusted if it
# would FAIL when instrumentation regresses. Two adversarial proofs:
#   adversarial-breach        → the SAME evidence that PASSES within target, once
#                               an observed value regresses past the target, FAILS.
#   adversarial-missing-attr  → an observed block that DROPS a contract-declared
#                               metric FAILS (a removed measurement cannot prove
#                               the SLO met), not a silent skip.
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
GUARD="$SCRIPT_DIR/observability-slo-guard.sh"
FIXTURES="$SCRIPT_DIR/../tests/fixtures/observability"
BASH_BIN="$(command -v bash)"

if [[ ! -x "$GUARD" ]]; then
  echo "observability-slo-guard-selftest: guard not executable: $GUARD" >&2
  exit 2
fi

# Both parsers are required to exercise the numeric cases. If either is missing,
# SKIP (exit 0) rather than fail — the missing-parser fail-closed case is proven
# separately by stripping PATH, which does not need a real parser present.
if ! command -v yq >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
  echo "SKIP: observability-slo-guard-selftest (yq and/or jq not installed)"
  exit 0
fi

WORKSPACE="$(mktemp -d "${HOME}/.bubbles-selftest-obs-slo.XXXXXX")"
cleanup() { rm -rf "$WORKSPACE"; }
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0

ok() { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko() { printf '[selftest] FAIL: %s\n' "$*" >&2; FAIL_COUNT=$((FAIL_COUNT + 1)); }

# Stage a repo's project config from a shipped fixture.
stage_config() {
  local root="$1" fixture="$2"
  mkdir -p "$root/.github"
  cp "$FIXTURES/$fixture" "$root/.github/bubbles-project.yaml"
}

# Mark a workflow as instrumented by writing a scope Test Plan row that declares
# observabilityWorkflow (this is what `workflow_is_instrumented` greps for).
stage_instrumented() {
  local root="$1" wf="$2"
  mkdir -p "$root/specs/001-demo"
  printf '%s\n' \
    '# Demo scope' \
    '' \
    '| Test | Category | Workflow | Proof |' \
    '|------|----------|----------|-------|' \
    "| SLO under load | stress | ${wf} | observabilityWorkflow: ${wf} |" \
    > "$root/specs/001-demo/scopes.md"
}

# Mark a workflow instrumented under a SPECIFIC spec dir (spec-attribution cases).
stage_instrumented_in() {
  local root="$1" spec="$2" wf="$3"
  mkdir -p "$root/$spec"
  printf '%s\n' \
    '# Demo scope' \
    '' \
    '| Test | Category | Workflow | Proof |' \
    '|------|----------|----------|-------|' \
    "| SLO under load | stress | ${wf} | observabilityWorkflow: ${wf} |" \
    > "$root/$spec/scopes.md"
}

# Write captured SLO evidence for <workflow> by transforming the canonical clean
# fixture through a jq filter (so adversarial cases derive from the SAME bytes
# that pass the within-target case). Empty filter == verbatim copy.
write_evidence() {
  local root="$1" wf="$2" filter="${3:-.}"
  local dir="$root/.specify/runtime/observability"
  mkdir -p "$dir"
  jq "$filter" "$FIXTURES/slo-evidence.json" > "$dir/${wf}.slo.json"
}

# Write raw (non-jq) evidence bytes verbatim (for the unparseable-JSON case).
write_evidence_raw() {
  local root="$1" wf="$2" bytes="$3"
  local dir="$root/.specify/runtime/observability"
  mkdir -p "$dir"
  printf '%s' "$bytes" > "$dir/${wf}.slo.json"
}

RC=""; OUT=""
run_guard() {
  local root="$1"
  local of="$WORKSPACE/out.last"
  "$BASH_BIN" "$GUARD" --repo-root "$root" >"$of" 2>&1
  RC=$?
  OUT="$(cat "$of")"
}
# Run the guard scoped to a specific transitioning spec dir (--spec-dir), the
# shape state-transition-guard's Check 39 uses.
run_guard_spec_scoped() {
  local root="$1" spec="$2"
  local of="$WORKSPACE/out.last"
  "$BASH_BIN" "$GUARD" --repo-root "$root" --spec-dir "$spec" >"$of" 2>&1
  RC=$?
  OUT="$(cat "$of")"
}
# Run the guard with a PATH stripped of jq/yq (proves FAIL CLOSED).
run_guard_no_parser() {
  local root="$1"
  local empty="$WORKSPACE/emptybin"
  mkdir -p "$empty"
  local of="$WORKSPACE/out.last"
  PATH="$empty" "$BASH_BIN" "$GUARD" --repo-root "$root" >"$of" 2>&1
  RC=$?
  OUT="$(cat "$of")"
}

assert_exit() {
  local want="$1" label="$2"
  if [[ "$RC" == "$want" ]]; then ok "$label (exit $RC)"; else
    ko "$label: expected exit $want, got $RC"; printf '  --- output ---\n%s\n' "$OUT" >&2
  fi
}
assert_contains() {
  local needle="$1" label="$2"
  if grep -qiF -- "$needle" <<<"$OUT"; then ok "$label (contains '$needle')"; else
    ko "$label: output missing '$needle'"; printf '  --- output ---\n%s\n' "$OUT" >&2
  fi
}

WF="booking.create"

# --- within-target: wired + instrumented + valid evidence within target ---
R="$WORKSPACE/within-target"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.'
run_guard "$R"; assert_exit 0 "within-target"; assert_contains "within target" "within-target message"

# --- breached: observed latency over target → BLOCK ----------------------
R="$WORKSPACE/breached"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.observed.latencyP99Ms = 80'
run_guard "$R"; assert_exit 1 "breached blocks when wired"; assert_contains "SLO BREACH" "breach message"; assert_contains "latencyP99Ms" "breach names metric"

# --- opted-out: clean no-op ---------------------------------------------
R="$WORKSPACE/opted-out"
stage_config "$R" posture-opted-out-fresh.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.observed.latencyP99Ms = 999'   # breaching, but posture!=wired
run_guard "$R"; assert_exit 0 "opted-out no-op"; assert_contains "no-op" "opted-out message"

# --- no-observabilityWorkflow: wired + slo: link but NO instrumented scope ---
R="$WORKSPACE/no-instrumented"
stage_config "$R" posture-wired.yaml
mkdir -p "$R/specs/001-demo"
printf '# scope with no observability workflow row\n' > "$R/specs/001-demo/scopes.md"
write_evidence "$R" "$WF" '.observed.latencyP99Ms = 999'   # breaching, but not instrumented
run_guard "$R"; assert_exit 0 "no observabilityWorkflow no-op"; assert_contains "no instrumented" "no-instrumented message"

# --- missing-evidence: instrumented workflow with NO evidence file → gap ---
R="$WORKSPACE/missing-evidence"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
# deliberately do NOT write the evidence file
run_guard "$R"; assert_exit 1 "missing evidence blocks"; assert_contains "NO captured SLO evidence" "missing-evidence message"

# --- malformed-json: unparseable bytes → fail loud before numeric compare ---
R="$WORKSPACE/malformed-json"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence_raw "$R" "$WF" 'this is { not ] valid JSON'
run_guard "$R"; assert_exit 1 "malformed JSON blocks"; assert_contains "malformed JSON" "malformed-json message"

# --- malformed-no-observed: shipped fixture missing `observed` → fail loud ---
R="$WORKSPACE/malformed-no-observed"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
mkdir -p "$R/.specify/runtime/observability"
cp "$FIXTURES/slo-evidence-malformed.invalid.json" "$R/.specify/runtime/observability/${WF}.slo.json"
run_guard "$R"; assert_exit 1 "malformed (no observed) blocks"; assert_contains "missing the required 'observed'" "no-observed message"

# --- wrong-workflow: evidence captured for a DIFFERENT workflow → fail loud ---
R="$WORKSPACE/wrong-workflow"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.workflow = "checkout.flow"'
run_guard "$R"; assert_exit 1 "wrong-workflow blocks"; assert_contains "Wrong-workflow evidence is rejected" "wrong-workflow message"

# --- missing-parser: PATH stripped → FAIL CLOSED (blocking gate) ---------
R="$WORKSPACE/missing-parser"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.'
run_guard_no_parser "$R"; assert_exit 1 "missing parser fails closed"; assert_contains "install jq" "missing-parser message"

# --- non-adopter + missing-parser: NO observability config → no-op (NOT blocked) ---
# Safety property for wiring G100 into the universal done-gate: a repo that
# never adopted observability MUST no-op even when jq/yq (and even all of PATH)
# are absent. The parser-free builtin opt-in pre-check guarantees a non-adopter
# is never fail-closed-blocked. (A) no config file at all; (B) a config with no
# `observability:` key.
R="$WORKSPACE/nonadopter-no-config"
mkdir -p "$R/.github"
# deliberately NO bubbles-project.yaml
run_guard_no_parser "$R"; assert_exit 0 "non-adopter (no config) + no parser → no-op"; assert_contains "no-op" "non-adopter-no-config message"

R="$WORKSPACE/nonadopter-no-obs-block"
mkdir -p "$R/.github"
printf 'scans:\n  idor:\n    handlerFilePatterns: handler\n' > "$R/.github/bubbles-project.yaml"
run_guard_no_parser "$R"; assert_exit 0 "non-adopter (no observability block) + no parser → no-op"; assert_contains "no traceContracts.observability block" "non-adopter-no-obs-block message"

R="$WORKSPACE/nonadopter-comment-only-observability"
mkdir -p "$R/.github"
printf '# observability: not adopted here\nscans:\n  idor:\n    handlerFilePatterns: handler\n' > "$R/.github/bubbles-project.yaml"
run_guard_no_parser "$R"; assert_exit 0 "non-adopter (comment-only observability mention) + no parser → no-op"; assert_contains "no traceContracts.observability block" "non-adopter-comment-only message"

R="$WORKSPACE/nonadopter-unrelated-observability-key"
mkdir -p "$R/.github"
printf 'not_observability: true\nscans:\n  idor:\n    handlerFilePatterns: handler\n' > "$R/.github/bubbles-project.yaml"
run_guard_no_parser "$R"; assert_exit 0 "non-adopter (unrelated observability key) + no parser → no-op"; assert_contains "no traceContracts.observability block" "non-adopter-unrelated-key message"

# --- framework-repo-exempt: VERSION + install.sh + bubbles/scripts → EXEMPT ---
R="$WORKSPACE/fwrepo"
mkdir -p "$R/bubbles/scripts" "$R/.github"
printf '9.9.9\n' > "$R/VERSION"
printf '#!/usr/bin/env bash\necho installer\n' > "$R/install.sh"
cp "$FIXTURES/posture-wired.yaml" "$R/.github/bubbles-project.yaml"
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.observed.latencyP99Ms = 999'   # breaching, but exempt short-circuits
run_guard "$R"; assert_exit 0 "framework-repo exempt"; assert_contains "EXEMPT" "framework-exempt message"

# --- ADVERSARIAL (R2-G): same evidence passes, then a regressed value FAILS ---
# Prove the guard is NOT a rubber stamp: the within-target evidence above passed
# (exit 0); a single regressed observed value on that SAME contract must FAIL.
R="$WORKSPACE/adversarial-breach"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" '.'                              # identical to within-target
run_guard "$R"; assert_exit 0 "adversarial baseline passes (identical to within-target)"
write_evidence "$R" "$WF" '.observed.errorRatePct = 5.0'  # regress error rate 0.02 → 5.0 (target 0.1)
run_guard "$R"; assert_exit 1 "adversarial: regressed observed value now FAILS"; assert_contains "errorRatePct" "adversarial breach names metric"

# --- ADVERSARIAL (R2-G): a removed contract-declared metric FAILS ---------
# A dropped measurement (instrumentation regression) cannot prove the SLO met.
R="$WORKSPACE/adversarial-missing-attr"
stage_config "$R" posture-wired.yaml
stage_instrumented "$R" "$WF"
write_evidence "$R" "$WF" 'del(.observed.latencyP99Ms)'   # drop a contract-declared metric
run_guard "$R"; assert_exit 1 "adversarial: removed required metric FAILS"; assert_contains "MISSING" "adversarial missing-metric message"

# --- G100 REFINEMENT (spec-attribution scoping, mirrors G090) -------------
# (a) TEETH PRESERVED: an instrumented-with-slo spec with ABSENT evidence still
#     BLOCKS under its OWN --spec-dir (the SLO gate keeps teeth for the spec).
R="$WORKSPACE/spec-scoped-teeth"
stage_config "$R" posture-wired.yaml
stage_instrumented_in "$R" "specs/010-instrumented" "$WF"
# deliberately do NOT write the evidence file
run_guard_spec_scoped "$R" "specs/010-instrumented"
assert_exit 1 "spec-scoped: instrumented spec with missing evidence still BLOCKS"
assert_contains "NO captured SLO evidence" "spec-scoped teeth message"

# (b) FALSE-BLOCK FIXED: a spec declaring NO observabilityWorkflow no-ops under
#     --spec-dir EVEN WHEN an UNRELATED spec in the same fixture instruments a
#     workflow whose evidence is absent (the BUG-005 scenario). The same fixture
#     is reused by (c) to prove the repo-wide path is unchanged.
R="$WORKSPACE/spec-scoped-attribution"
stage_config "$R" posture-wired.yaml
stage_instrumented_in "$R" "specs/021-other" "$WF"   # unrelated instrumented sibling, NO evidence
mkdir -p "$R/specs/020-no-obs"
printf '# tooling fix — zero observability instrumentation\n' > "$R/specs/020-no-obs/scopes.md"
run_guard_spec_scoped "$R" "specs/020-no-obs"
assert_exit 0 "spec-scoped: no-observabilityWorkflow spec no-ops despite unrelated instrumented sibling with absent evidence"
assert_contains "not applicable to this spec" "spec-scoped false-block-fixed message"

# (c) BACKWARD-COMPAT: the SAME fixture with NO --spec-dir (repo-wide) still
#     BLOCKS — the unrelated instrumented sibling's absent evidence is enforced,
#     proving standalone/CI teeth are preserved.
run_guard "$R"
assert_exit 1 "repo-wide (no --spec-dir): instrumented sibling with absent evidence still BLOCKS"
assert_contains "NO captured SLO evidence" "backward-compat teeth message"

echo ""
echo "observability-slo-guard-selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if (( FAIL_COUNT == 0 )); then
  echo "observability-slo-guard selftest passed."
  exit 0
else
  echo "observability-slo-guard selftest FAILED." >&2
  exit 1
fi
