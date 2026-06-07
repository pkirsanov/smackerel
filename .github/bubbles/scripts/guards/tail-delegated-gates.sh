# shellcheck shell=bash
# shellcheck disable=SC2154  # sourced fragment: all referenced vars are set in state-transition-guard.sh's scope before sourcing
# =============================================================================
# guards/tail-delegated-gates.sh  (M4 guard split)
# =============================================================================
# Checks 26-35: the delegated tail gates G085-G095. This fragment is the body
# of the `else` branch of the BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST
# fast-path conditional in state-transition-guard.sh; it is sourced in the same
# shell scope so pass/fail/warn/info, the failures/warnings counters, and
# computed vars ($SCRIPT_DIR, $feature_dir, $script_repo_root, $guard_repo_root,
# fixture_gate_skip, run_guard_in_feature_repo, run_guard_in_script_repo) are
# all in scope exactly as before. Behavior is byte-identical to the previous
# inline blocks.
# =============================================================================

# =============================================================================
# CHECK 26: Framework Dogfood Evidence Enforcement (Gate G085)
# =============================================================================
echo "--- Check 26: Framework Dogfood Evidence Enforcement (Gate G085) ---"
dog_guard="$SCRIPT_DIR/framework-dogfood-guard.sh"
if fixture_gate_skip "framework dogfood evidence enforcement (Gate G085)"; then
  :
elif [[ -x "$dog_guard" ]]; then
  if run_guard_in_script_repo bash "$dog_guard" --repo-root "$script_repo_root" --quiet > /dev/null 2>&1; then
    pass "Framework dogfood evidence contract is satisfied (Gate G085)"
  else
    fail "Framework dogfood evidence contract failed — Gate G085. Run 'bash $dog_guard' for full diagnostic"
    info "Bubbles source requirement: no persistent specs/ tree; use framework validation, selftests, release manifest, and downstream/fixture specs as evidence"
    info "Downstream/fixture requirement: at least one specs/[0-9]*-*/state.json has top-level \"status\": \"done\""
    info "Recipe: docs/recipes/framework-dogfood.md"
    info "Cross-references: G082 (convergence cap), G083 (compaction discipline), G084 (pre-existing deferral)"
  fi
else
  info "framework-dogfood-guard.sh not present at $dog_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 27: Orchestrator Persistence Prompt Lint (Gate G086)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/orchestrator-persistence-lint.sh.
# The guard scans the 4 orchestrator prompt files and rejects language
# that makes continuation depend on a fresh user prompt. Orchestrators
# must default to persistence after non-terminal phases, stopping only
# for convergence achieved, max iterations reached, user requests stop,
# or fundamental impossibility.
echo "--- Check 27: Orchestrator Persistence Prompt Lint (Gate G086) ---"
persistence_guard="$SCRIPT_DIR/orchestrator-persistence-lint.sh"
if fixture_gate_skip "orchestrator persistence prompt lint (Gate G086)"; then
  :
elif [[ -x "$persistence_guard" ]]; then
  if run_guard_in_script_repo bash "$persistence_guard" --root "$script_repo_root" --quiet > /dev/null 2>&1; then
    pass "Orchestrator prompt files satisfy persistence-default lint (Gate G086)"
  else
    fail "Orchestrator persistence prompt lint failed — Gate G086. Run 'bash $persistence_guard' for full diagnostic"
    info "Target files: agents/bubbles.goal.agent.md, agents/bubbles.workflow.agent.md, agents/bubbles.iterate.agent.md, agents/bubbles.sprint.agent.md"
    info "Required default: after non-terminal phases, automatically continue to the next phase"
    info "Stop reasons: convergence achieved, max iterations reached, user requests stop, fundamental impossibility"
  fi
else
  info "orchestrator-persistence-lint.sh not present at $persistence_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 28: Planning Workflow Chain Enforcement (Gate G091)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/planning-workflow-chain-guard.sh.
# Delivery-capable planning/bootstrap/fallback paths MUST preserve the ordered
# canonical chain: bubbles.analyst -> bubbles.ux -> bubbles.design ->
# bubbles.plan. UX is mandatory even for framework/operator/non-UI work;
# non-UI UX defines workflow behavior, status language, blocked envelopes,
# and exception handling.
echo "--- Check 28: Planning Workflow Chain Enforcement (Gate G091) ---"
planning_chain_guard="$SCRIPT_DIR/planning-workflow-chain-guard.sh"
if fixture_gate_skip "planning workflow chain enforcement (Gate G091)"; then
  :
elif [[ -x "$planning_chain_guard" ]]; then
  planning_chain_repo_root="$script_repo_root"
  if bash "$planning_chain_guard" --root "$planning_chain_repo_root" --quiet > /dev/null 2>&1; then
    pass "Planning workflow chain preserves analyst -> ux -> design -> plan (Gate G091)"
  else
    fail "Planning workflow chain guard failed — Gate G091. Run 'bash $planning_chain_guard --root $planning_chain_repo_root' for full diagnostic"
    info "Required chain: bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan"
    info "Targets: workflows.yaml delivery constraints, inline auto-escalations, bootstrapAgents, improvementPreludeProfiles, and prompt/shared fallback prose"
    info "UX is mandatory even for framework/operator/non-UI work; non-UI UX defines workflow behavior, status language, blocked envelopes, and exception handling"
  fi
else
  info "planning-workflow-chain-guard.sh not present at $planning_chain_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 29: Planning Packet Implementation Linkage (Gate G087)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/planning-packet-linkage-guard.sh.
# Hardened planning packets (`state.status == specs_hardened`) must either
# link to a real implementation spec with state.json or classify themselves
# as planning-only with a non-empty justification. If the linked
# implementation spec is done, it must point back with linkedPlanningPacket.
# Archived implementation targets are not valid active implementation links.
echo "--- Check 29: Planning Packet Implementation Linkage (Gate G087) ---"
planning_linkage_guard="$SCRIPT_DIR/planning-packet-linkage-guard.sh"
if fixture_gate_skip "planning packet implementation linkage (Gate G087)"; then
  :
elif [[ -x "$planning_linkage_guard" ]]; then
  if run_guard_in_feature_repo bash "$planning_linkage_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Planning packet implementation linkage is coherent (Gate G087)"
  else
    fail "Planning packet implementation linkage failed — Gate G087. Run 'bash $planning_linkage_guard $feature_dir' for full diagnostic"
    info "specs_hardened packets with planningOnly != true MUST set linkedImplementationSpec to a real spec directory with state.json"
    info "If the linked implementation spec is done, linkedPlanningPacket MUST point back to the planning packet"
    info "planningOnly:true requires a non-empty planningOnlyJustification; archived implementation targets must be relinked or classified planning-only"
  fi
else
  info "planning-packet-linkage-guard.sh not present at $planning_linkage_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 29B: Delivery Implementation Delta (Gate G093)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/delivery-implementation-delta-guard.sh.
# G053 owns Code Diff Evidence shape; G087 owns planning packet linkage; G093
# owns status-ceiling-aware path classification for done-ceiling delivery modes.
echo "--- Check 29B: Delivery Implementation Delta (Gate G093) ---"
delivery_delta_guard="$SCRIPT_DIR/delivery-implementation-delta-guard.sh"
if fixture_gate_skip "delivery implementation delta (Gate G093)"; then
  :
elif [[ -x "$delivery_delta_guard" ]]; then
  if run_guard_in_feature_repo bash "$delivery_delta_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Delivery implementation delta is present or mode ceiling exempts it (Gate G093)"
  else
    fail "Delivery implementation delta guard failed — Gate G093. Run 'bash $delivery_delta_guard $feature_dir' for changed-path classification and owner routing"
    info "Done-ceiling delivery modes MUST show implementation/runtime/config/contract/test/docs delta outside specs/ and .specify/"
    info "Spec-only delivery output must route to implementation/test/docs work, or downgrade to a below-done planning-only workflow governed by G087"
    info "G053 remains the Code Diff Evidence shape check; G093 is the delivery-mode status-level path gate"
  fi
else
  info "delivery-implementation-delta-guard.sh not present at $delivery_delta_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 30: Post-Certification Spec Edit Detection (Gate G088)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/post-cert-spec-edit-guard.sh.
# Certified specs (`state.status == done` or legacy read-only
# `done_with_concerns`) must carry
# top-level certifiedAt and must not have later planning-truth commits touching
# spec.md, design.md, scopes.md, scopes/_index.md, or per-scope scope.md files
# unless the spec is demoted, explicitly requires revalidation, or has been
# recertified by current spec review with a newer certifiedAt.
echo "--- Check 30: Post-Certification Spec Edit Detection (Gate G088) ---"
post_cert_guard="$SCRIPT_DIR/post-cert-spec-edit-guard.sh"
if fixture_gate_skip "post-certification spec edit detection (Gate G088)"; then
  :
elif [[ -x "$post_cert_guard" ]]; then
  if run_guard_in_feature_repo bash "$post_cert_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Post-certification planning truth is aligned with certification state (Gate G088)"
  else
    fail "Post-certification spec edit guard failed — Gate G088. Run 'bash $post_cert_guard $feature_dir' for full diagnostic"
    info "Certified specs MUST have top-level certifiedAt and no later planning truth commits"
    info "Tracked files: spec.md, design.md, scopes.md, scopes/_index.md, scopes/*/scope.md"
    info "Remediation: demote status, set requiresRevalidation:true, or complete bubbles.spec-review recertification and update certifiedAt after the edit"
  fi
else
  info "post-cert-spec-edit-guard.sh not present at $post_cert_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 31: Inter-Spec Dependency Enforcement (Gate G089)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/inter-spec-dependency-guard.sh.
# Explicit specDependsOn[] entries must resolve to real specs with stable
# states (done, with legacy read-only done_with_concerns accepted only for
# untouched old specs), unless the current spec has already been flagged with
# requiresRevalidation:true. Cycles are always blocking.
echo "--- Check 31: Inter-Spec Dependency Enforcement (Gate G089) ---"
inter_spec_dependency_guard="$SCRIPT_DIR/inter-spec-dependency-guard.sh"
if fixture_gate_skip "inter-spec dependency enforcement (Gate G089)"; then
  :
elif [[ -x "$inter_spec_dependency_guard" ]]; then
  if run_guard_in_feature_repo bash "$inter_spec_dependency_guard" "$feature_dir" --repo-root "$guard_repo_root" --quiet > /dev/null 2>&1; then
    pass "Inter-spec dependencies are stable or explicitly flagged for revalidation (Gate G089)"
  else
    fail "Inter-spec dependency guard failed — Gate G089. Run 'bash $inter_spec_dependency_guard $feature_dir' for full diagnostic"
    info "Every specDependsOn[] path MUST resolve to a spec directory containing state.json"
    info "Dependency statuses allowed without revalidation: done; legacy read-only done_with_concerns remains compatible only until touched or recertified"
    info "If a dependency is demoted, run inter-spec-dependency-revalidation.sh on that dependency so dependents carry requiresRevalidation:true"
    info "Dependency cycles are always blocking"
  fi
else
  info "inter-spec-dependency-guard.sh not present at $inter_spec_dependency_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 32: Strict Terminal Status Enforcement (Gate G092)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/strict-terminal-status-guard.sh.
# New delivery certification writes may use only `done` or `blocked` as
# terminal statuses. Legacy done_with_concerns remains readable only for old,
# untouched specs until recertification migrates to done plus observations or
# blocked. High/remediation-required observations cannot accompany done.
echo "--- Check 32: Strict Terminal Status Enforcement (Gate G092) ---"
strict_terminal_status_guard="$SCRIPT_DIR/strict-terminal-status-guard.sh"
if fixture_gate_skip "strict terminal status enforcement (Gate G092)"; then
  :
elif [[ -x "$strict_terminal_status_guard" ]]; then
  if run_guard_in_script_repo bash "$strict_terminal_status_guard" "$feature_dir" --repo-root "$script_repo_root" --quiet > /dev/null 2>&1; then
    pass "Terminal certification statuses are strict (Gate G092)"
  else
    fail "Strict terminal status guard failed — Gate G092. Run 'bash $strict_terminal_status_guard $feature_dir' for full diagnostic"
    info "Valid new terminal delivery statuses: done, blocked"
    info "Non-blocking notes belong in observations[] / certification.observations[], not status"
    info "Legacy done_with_concerns is read-only only; touched or recertified specs migrate to done+observations or blocked"
    info "High/remediation-required observations block done"
  fi
else
  info "strict-terminal-status-guard.sh not present at $strict_terminal_status_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 33: Retro Convergence Health Evidence (Gate G090)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/retro-convergence-health.sh.
# Retrospectives must compute convergence health from session data. More than
# 2 combined recap/handoff invocations is a P0 convergence regression and
# fails the gate with slo=failed.
echo "--- Check 33: Retro Convergence Health Evidence (Gate G090) ---"
retro_convergence_health="$SCRIPT_DIR/retro-convergence-health.sh"
if fixture_gate_skip "retro convergence health evidence (Gate G090)"; then
  :
elif [[ -f "$retro_convergence_health" ]]; then
  retro_repo_root="$script_repo_root"
  if bash "$retro_convergence_health" "$feature_dir" --repo-root "$retro_repo_root" --schema full > /dev/null 2>&1; then
    pass "Retro convergence health SLO is pass/degraded (Gate G090)"
  else
    fail "Retro convergence health failed — Gate G090. Run 'bash $retro_convergence_health $feature_dir --repo-root $retro_repo_root' for full diagnostic"
    info "Required retro schema: convergenceHealth: {recapCount, handoffCount, summarizeHistoryCount, turnCount, slo}"
    info "P0 regression threshold: recapCount + handoffCount MUST be <= 2"
    info "Snapshot completeness threshold: snapshotCompleteness MUST be 1.0"
  fi
else
  info "retro-convergence-health.sh not present at $retro_convergence_health; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 34: Capability Foundation Enforcement (Gate G094)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/capability-foundation-guard.sh.
# New specs that trigger capability-first proportionality must model the
# domain capability, technical foundation, concrete implementations,
# variation axes, UI primitives where applicable, and foundation-before-
# overlay scope ordering. Older specs are grandfathered by state.json
# createdAt so framework upgrades do not retroactively block closed work.
echo "--- Check 34: Capability Foundation Enforcement (Gate G094) ---"
capability_foundation_guard="$SCRIPT_DIR/capability-foundation-guard.sh"
if [[ -x "$capability_foundation_guard" ]]; then
  if run_guard_in_feature_repo bash "$capability_foundation_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Capability foundation requirements are satisfied, not applicable, or grandfathered (Gate G094)"
  else
    fail "Capability foundation guard failed — Gate G094. Run 'bash $capability_foundation_guard $feature_dir' for full diagnostic"
    info "Proportionality triggers: new capability, N>=2 implementation/provider/component/variant, adapter/provider/strategy/plugin/channel/driver/connector/variant language, or shared surfaces"
    info "Required sections: spec.md Domain Capability Model or Single-Capability Justification; design.md Capability Foundation / Concrete Implementations / Variation Axes or Single-Implementation Justification"
    info "When multiple screens share UI behavior, spec.md must include UI Primitives or Single-Screen Justification"
    info "When design splits foundation and concrete implementations, scopes must tag foundation:true and overlay scopes must Depends On the foundation"
  fi
else
  info "capability-foundation-guard.sh not present at $capability_foundation_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 35: Discovered-Issue Disposition (Gate G095)
# =============================================================================
# Every issue an agent observes during work MUST have an explicit disposition.
# "Pre-existing", "unrelated", "out of scope", "known issue", "skipping",
# "will fix later", "not my session" without a filed BUG/spec/ops/routed
# disposition is forbidden and counts as fabrication.
echo "--- Check 35: Discovered-Issue Disposition (Gate G095) ---"
discovered_issue_guard="$SCRIPT_DIR/discovered-issue-disposition-guard.sh"
if [[ -x "$discovered_issue_guard" ]]; then
  if bash "$discovered_issue_guard" "$feature_dir" > /dev/null 2>&1; then
    pass "Discovered-issue disposition clean — no unfiled deferrals (Gate G095)"
  else
    fail "Discovered-issue disposition guard failed — Gate G095. Run 'bash $discovered_issue_guard $feature_dir' for full diagnostic"
    info "Remediation: for every forbidden deferral phrase, either cite a concrete artifact (BUG-NNN, TR-NNN, spec path, ops URL) in the same paragraph, OR add a row to '## Discovered Issues' in report.md dated today with disposition + reference"
    info "See agents/bubbles_shared/operating-baseline.md → 'Discovered-Issue Disposition' for the disposition table"
  fi
else
  info "discovered-issue-disposition-guard.sh not present at $discovered_issue_guard; skipping (advisory)"
fi
echo ""
