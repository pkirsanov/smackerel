# shellcheck shell=bash
# =============================================================================
# guards/tail-convergence-gates.sh  (M4 guard split)
# =============================================================================
# Checks 23-25: convergence cap (G082), compaction discipline (G083), and
# pre-existing deferral block (G084). Sourced by state-transition-guard.sh in
# the same shell scope, so pass/fail/warn/info, the failures/warnings counters,
# and computed vars ($SCRIPT_DIR, $feature_dir, fixture_gate_skip,
# run_guard_in_feature_repo) are all in scope exactly as before extraction.
# Behavior is byte-identical to the previous inline blocks.
# =============================================================================

# =============================================================================
# CHECK 23: Convergence Cap Enforcement (Gate G082)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/convergence-cap-guard.sh.
# The guard reads `.specify/memory/bubbles.session.json` and checks every
# `convergenceLoops[]` entry whose `specDir` matches the spec under
# inspection. If the highest observed `iterationCount` exceeds
# `maxConvergenceIterations` (default 10, from `bubbles/workflows.yaml`),
# the guard exits 1 and this check fails. Missing session.json, missing
# convergenceLoops[], or entries scoped to other specs all pass cleanly.
echo "--- Check 23: Convergence Cap Enforcement (Gate G082) ---"
conv_guard="$SCRIPT_DIR/convergence-cap-guard.sh"
if fixture_gate_skip "convergence cap enforcement (Gate G082)"; then
  :
elif [[ -x "$conv_guard" ]]; then
  if run_guard_in_feature_repo bash "$conv_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Convergence cap not exceeded (Gate G082)"
  else
    fail "Convergence cap exceeded — Gate G082 violation. Run 'bash $conv_guard $feature_dir' for full diagnostic"
    info "maxConvergenceIterations lives in bubbles/workflows.yaml (default 10)"
    info "Orchestrator agents (workflow, goal, iterate, sprint) MUST emit a 'blocked' RESULT-ENVELOPE with finding G082 when the cap is reached"
  fi
else
  info "convergence-cap-guard.sh not present at $conv_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 24: Compaction Discipline Enforcement (Gate G083)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/compaction-discipline-guard.sh.
# The guard reads `.specify/memory/bubbles.session.json`, isolates
# `envelopesReceived[]` entries whose `specDir` matches the spec under
# inspection, sorts by `receivedAt`, drops the latest 2 (kept raw by
# policy), then checks the eligible slice for BOTH `count <= 3` AND
# `cumulative rawSizeBytes <= 8192` UNLESS each over-budget envelope
# carries a `compactedAt` timestamp. Thresholds are framework constants
# (NOT workflows.yaml-configurable). Missing session.json or no
# envelopesReceived[] entries for this spec both pass cleanly.
echo "--- Check 24: Compaction Discipline Enforcement (Gate G083) ---"
comp_guard="$SCRIPT_DIR/compaction-discipline-guard.sh"
if fixture_gate_skip "compaction discipline enforcement (Gate G083)"; then
  :
elif [[ -x "$comp_guard" ]]; then
  if run_guard_in_feature_repo bash "$comp_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Compaction discipline respected (Gate G083)"
  else
    fail "Compaction discipline violation — Gate G083. Run 'bash $comp_guard $feature_dir' for full diagnostic"
    info "Eligible slice (envelopes except latest 2) MUST satisfy count<=3 AND rawSizeBytes<=8192 UNLESS each over-budget envelope has compactedAt"
    info "Orchestrator agents MUST run bubbles/scripts/context-compactor.sh on over-budget envelopes (additively stamps compactedAt) BEFORE the next dispatch"
    info "Thresholds are framework constants; see agents/bubbles_shared/operating-baseline.md → 'Context Compaction Discipline'"
  fi
else
  info "compaction-discipline-guard.sh not present at $comp_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 25: Pre-Existing Deferral Block Enforcement (Gate G084)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/pre-existing-deferral-guard.sh.
# The guard recursively scans every `scope.md` and `report.md` under
# `<feature_dir>/scopes/*/` for two classes of pre-existing deferral
# markers:
#   - Forbidden phrases (case-insensitive substring):
#       "pre-existing failure", "pre-existing test failure",
#       "carried forward", "out of session scope",
#       "previous-session failure", "not introduced by this spec"
#   - Forbidden markers (colon-anchored, case-sensitive):
#       TODO:  FIXME:  HACK:  STUB:
# H2 subsections named `## Superseded Decisions`, `## Historical Notes`,
# and `## Out of Scope` are exempt (allowed to discuss historical
# deferrals for traceability). Inline `...` backticked spans and
# ```fenced code blocks``` are also exempt so the guard never
# self-triggers when the language is used as enumeration prose or
# captured raw terminal output. Any active hit produces exit 1 and
# blocks promotion to `done`.
echo "--- Check 25: Pre-Existing Deferral Block Enforcement (Gate G084) ---"
pre_guard="$SCRIPT_DIR/pre-existing-deferral-guard.sh"
if [[ -x "$pre_guard" ]]; then
  if run_guard_in_feature_repo bash "$pre_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "No active pre-existing-deferral markers in scope.md / report.md (Gate G084)"
  else
    fail "Pre-existing deferral marker detected — Gate G084. Run 'bash $pre_guard $feature_dir' for full diagnostic"
    info "Forbidden phrases: 'pre-existing failure', 'pre-existing test failure', 'carried forward', 'out of session scope', 'previous-session failure', 'not introduced by this spec'"
    info "Forbidden markers (colon-anchored): TODO:  FIXME:  HACK:  STUB:"
    info "Move historical language under '## Superseded Decisions', '## Historical Notes', or '## Out of Scope', OR wrap enumeration prose in inline backticks"
    info "Pre-existing failures MUST be fixed inline; deferring to a follow-up session is forbidden by Gate G084"
  fi
else
  info "pre-existing-deferral-guard.sh not present at $pre_guard; skipping (advisory)"
fi
echo ""
