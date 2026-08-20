# Gate Coverage Map

> GENERATED — do not edit by hand.
> Regenerate: `bash bubbles/scripts/generate-gate-coverage-map.sh`
> Check drift: `bash bubbles/scripts/generate-gate-coverage-map.sh --check`

This page maps every gate defined in `bubbles/registry/gates.yaml` to the surface(s) that enforce it. It exists so that gates NOT listed in any workflow mode's `requiredGates` are demonstrably enforced elsewhere (state-transition-guard checks, framework-validate selftests/guards, or CI) rather than silently unenforced.

Column meanings:

- **Enforced By (declared)** — the authoritative `enforcedBy` value from `bubbles/registry/gates.yaml`. `guard-check:N` / `script:<path>` / `ci:<workflow>` are mechanical; `mode-required` means a mode demands the gate but no dedicated enforcer implements it; `behavioral:<agent>` is agent-behavior enforcement by design; `unbound` is a genuine coverage gap.
- **# Modes** — how many `modes.yaml` / `workflows.yaml` modes list the gate in `requiredGates`.
- **state-transition-guard** — the guard's labeled `Check N` that names the gate, `ref` when the gate id appears in the guard or a `bubbles/scripts/guards/**` fragment without a labeled check, else `—`.
- **framework-validate scripts** — count of OTHER `bubbles/scripts/**/*.sh` files (selftests, lints, standalone guards run by `framework-validate.sh`) that reference the gate id (the transition guard is excluded — it has its own column).
- **CI** — how a `.github/workflows/*.yml` transitively enforces the gate: `guard` (a workflow runs `state-transition-guard`), `fw-validate` (a workflow runs `framework-validate`), `named` (the gate id appears literally), else `—`.

Detection is limited to these MECHANICAL surfaces. A gate with none of them may still be enforced by AGENT-BEHAVIOR instructions (referenced only under `agents/**`, e.g. a value-first-selection or evidence-rule gate); such gates are surfaced under REVIEW so a maintainer can confirm the behavioral-only enforcement is intentional. This map is advisory — it never blocks on mode coverage completeness.

## Coverage Summary

- Gates defined: **121**
- Declared mechanically enforced (`guard-check:` / `script:` / `ci:`): **84**
- Declared `mode-required` only (a mode requires it; no dedicated mechanical enforcer): **35**
- Declared `behavioral:` (agent-behavior enforcement, by design): **1**
- Declared `unbound` (NO enforcement surface — genuine coverage gap): **1** — G071

Corroborating (grep-derived, advisory) numbers:

- Referenced by ≥1 workflow mode: **65**
- Not referenced by any mode: **56**
  - of those, referenced by state-transition-guard: **36**
  - of those, referenced by a framework-validate script: **52**
  - of those, referenced in CI: **36**

## All Gates

| Gate | Name | Enforced By (declared) | # Modes | state-transition-guard | framework-validate scripts | CI |
| --- | --- | --- | --- | --- | --- | --- |
| G001 | artifact_gate | `mode-required` | 55 | — | 18 | — |
| G002 | scope_definition_gate | `mode-required` | 23 | — | 10 | — |
| G003 | test_integrity_gate | `mode-required` | 25 | — | 8 | — |
| G004 | test_execution_gate | `mode-required` | 25 | — | 4 | — |
| G005 | evidence_gate | `mode-required` | 25 | — | 3 | — |
| G006 | docs_sync_gate | `mode-required` | 46 | — | 1 | — |
| G007 | validation_gate | `mode-required` | 48 | — | 2 | — |
| G008 | audit_gate | `mode-required` | 42 | — | 1 | — |
| G009 | chaos_gate | `mode-required` | 26 | — | 4 | — |
| G010 | user_validation_gate | `mode-required` | 31 | — | 2 | — |
| G011 | session_gate | `mode-required` | 55 | — | 1 | — |
| G012 | final_promotion_gate | `mode-required` | 55 | — | — | — |
| G013 | priority_selection_gate | `behavioral:bubbles.workflow` | 3 | — | — | — |
| G014 | bootstrap_readiness_gate | `mode-required` | 18 | — | — | — |
| G015 | scenario_depth_gate | `mode-required` | 24 | — | — | — |
| G016 | gherkin_e2e_coverage_gate | `mode-required` | 24 | — | — | — |
| G018 | dod_completion_gate | `mode-required` | 22 | — | 1 | — |
| G019 | sequential_spec_completion_gate | `mode-required` | 25 | — | 1 | — |
| G020 | cross_agent_verification_gate | `mode-required` | 25 | — | 1 | — |
| G021 | anti_fabrication_gate | `guard-check:12` | 25 | Check 20 | 6 | guard |
| G022 | specialist_completion_gate | `guard-check:6B` | 25 | Check 6B | 8 | guard |
| G023 | state_transition_guard_gate | `mode-required` | 25 | — | — | — |
| G024 | all_scopes_done_before_spec_done_gate | `script:bubbles/scripts/release-delivery-reconciliation-guard.sh` | 25 | ref | 5 | guard |
| G025 | per_dod_item_raw_evidence_gate | `script:bubbles/scripts/release-delivery-reconciliation-guard.sh` | 25 | — | 4 | — |
| G026 | sla_stress_coverage_gate | `guard-check:5A` | 22 | ref | 2 | guard |
| G027 | phase_scope_coherence_gate | `guard-check:15` | 25 | Check 15 | 3 | guard |
| G028 | implementation_reality_scan_gate | `guard-check:16` | 23 | Check 16 | 6 | guard |
| G029 | integration_completeness_gate | `script:bubbles/scripts/capability-consumer-freshness.sh`, `script:bubbles/scripts/release-delivery-reconciliation-guard.sh` | 23 | — | 3 | — |
| G031 | findings_artifact_update_gate | `mode-required` | 9 | — | 1 | — |
| G032 | business_analysis_gate | `mode-required` | 4 | — | 1 | — |
| G033 | design_readiness_gate | `mode-required` | 22 | — | — | — |
| G034 | security_gate | `script:bubbles/scripts/security-gate.sh`, `behavioral:bubbles.security` | 22 | — | 3 | — |
| G035 | vertical_slice_gate | `mode-required` | 22 | — | 1 | — |
| G036 | red_green_traceability_gate | `mode-required` | 2 | — | — | — |
| G037 | scope_size_discipline_gate | `script:bubbles/scripts/scope-context-fit-lint.sh` | 0 | — | 1 | — |
| G038 | failure_recovery_containment_gate | `mode-required` | 0 | — | 1 | — |
| G040 | incomplete_work_language_gate | `guard-check:18` | 25 | Check 18 | 5 | guard |
| G041 | dod_format_integrity_gate | `guard-check:4A`, `guard-check:4B` | 0 | Check 4A, Check 4B | 2 | guard |
| G042 | artifact_ownership_enforcement_gate | `guard-check:3G` | 0 | Check 3G | 3 | guard |
| G043 | consumer_trace_gate | `script:bubbles/scripts/expand-migrate-contract-guard.sh`, `script:bubbles/scripts/guards/planning-checks.sh` | 0 | ref | 2 | guard |
| G044 | comprehensive_regression_gate | `script:bubbles/scripts/domain-model-consistency.sh`, `script:bubbles/scripts/expand-migrate-contract-guard.sh`, `script:bubbles/scripts/regression-baseline-guard.sh` | 25 | — | 8 | — |
| G047 | idor_auth_bypass_gate | `mode-required` | 25 | — | 2 | — |
| G048 | silent_decode_failure_gate | `mode-required` | 25 | — | 2 | — |
| G051 | test_env_dependency_gate | `guard-check:19` | 25 | Check 19 | 2 | guard |
| G052 | artifact_freshness_guard_gate | `guard-check:13A` | 0 | Check 13A | — | guard |
| G053 | implementation_delta_evidence_gate | `guard-check:13B` | 0 | Check 13B | 4 | guard |
| G055 | policy_provenance_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 25 | Check 3A | 4 | guard |
| G056 | validate_certification_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 25 | Check 3H | 1 | guard |
| G057 | scenario_manifest_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 25 | Check 3C | 4 | guard |
| G058 | lockdown_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 1 | Check 3D | — | guard |
| G059 | regression_contract_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 25 | Check 3D | 1 | guard |
| G060 | scenario_tdd_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 3 | Check 3E | 5 | guard |
| G061 | rework_packet_gate | `script:bubbles/scripts/guards/control-plane-checks.sh` | 25 | Check 3F | 1 | guard |
| G063 | concrete_result_gate | `guard-check:3G` | 0 | Check 3G | 2 | guard |
| G064 | workflow_runner_authorization_gate | `guard-check:3H` | 0 | Check 3H | 7 | guard |
| G066 | phase_claim_provenance_gate | `guard-check:6B` | 0 | — | — | — |
| G067 | shared_infrastructure_blast_radius_gate | `script:bubbles/scripts/guards/planning-checks.sh` | 0 | ref | 1 | guard |
| G068 | dod_gherkin_content_fidelity_gate | `guard-check:22` | 0 | Check 22 | 11 | guard |
| G069 | collateral_change_containment_gate | `script:bubbles/scripts/guards/planning-checks.sh` | 0 | ref | 1 | guard |
| G070 | outcome_contract_gate | `script:bubbles/scripts/goal-fidelity-guard.sh`, `script:bubbles/scripts/scenario-compile-lint.sh` | 0 | — | 4 | — |
| G071 | execution_only_validation_gate | `unbound` | 0 | — | 3 | — |
| G072 | evidence_provenance_gate | `guard-check:12` | 0 | Check 40 | 4 | guard |
| G073 | planning_only_source_edit_lockout_gate | `guard-check:3B` | 30 | Check 3B | 6 | guard |
| G074 | workflow_mode_consistency_gate | `guard-check:2B` | 3 | ref | — | guard |
| G075 | scope_index_parity_gate | `guard-check:5B` | 0 | ref | — | guard |
| G076 | phantom_scope_detection_gate | `guard-check:5C` | 0 | ref | — | guard |
| G077 | execution_history_plausibility_gate | `guard-check:7A` | 0 | ref | 1 | guard |
| G078 | batch_promotion_limit_gate | `script:bubbles/scripts/batch-promotion-lint.sh` | 0 | — | 2 | — |
| G079 | impact_aware_validation_plan_gate | `script:bubbles/scripts/test-impact-plan.sh` | 0 | — | 1 | — |
| G080 | trace_contract_evidence_gate | `script:bubbles/scripts/trace-contract-guard.sh` | 0 | — | 2 | — |
| G081 | build_once_deploy_many_integrity_gate | `mode-required` | 3 | — | — | — |
| G082 | convergence_cap_enforcement_gate | `script:bubbles/scripts/convergence-cap-guard-selftest.sh`, `script:bubbles/scripts/convergence-cap-guard.sh` | 0 | Check 23 | 5 | guard |
| G083 | context_compaction_discipline_gate | `script:bubbles/scripts/compaction-discipline-guard-selftest.sh`, `script:bubbles/scripts/compaction-discipline-guard.sh`, `script:bubbles/scripts/context-compactor.sh`, `script:bubbles/scripts/state-snapshot.sh` | 0 | Check 24 | 11 | guard |
| G084 | pre_existing_deferral_block_gate | `script:bubbles/scripts/pre-existing-deferral-guard-selftest.sh`, `script:bubbles/scripts/pre-existing-deferral-guard.sh` | 0 | Check 25 | 2 | guard |
| G085 | framework_dogfood_evidence_gate | `guard-check:26` | 0 | Check 26 | 7 | guard |
| G086 | orchestrator_persistence_lint_gate | `script:bubbles/scripts/orchestrator-persistence-lint-selftest.sh`, `script:bubbles/scripts/orchestrator-persistence-lint.sh` | 0 | Check 27 | 2 | guard |
| G087 | planning_packet_implementation_linkage_gate | `script:bubbles/scripts/planning-packet-linkage-guard-selftest.sh`, `script:bubbles/scripts/planning-packet-linkage-guard.sh` | 0 | Check 29 | 6 | guard |
| G088 | post_certification_spec_edit_gate | `script:bubbles/scripts/post-cert-spec-edit-guard-selftest.sh`, `script:bubbles/scripts/post-cert-spec-edit-guard.sh` | 0 | Check 30 | 5 | guard |
| G089 | inter_spec_dependency_gate | `script:bubbles/scripts/inter-spec-dependency-guard-selftest.sh`, `script:bubbles/scripts/inter-spec-dependency-guard.sh`, `script:bubbles/scripts/inter-spec-dependency-revalidation.sh` | 0 | Check 31 | 7 | guard |
| G090 | retro_convergence_health_evidence_gate | `script:bubbles/scripts/retro-convergence-health-selftest.sh`, `script:bubbles/scripts/retro-convergence-health.sh` | 0 | Check 33 | 6 | guard |
| G091 | planning_workflow_chain_gate | `script:bubbles/scripts/planning-workflow-chain-guard-selftest.sh`, `script:bubbles/scripts/planning-workflow-chain-guard.sh` | 0 | Check 28 | 5 | guard |
| G092 | strict_terminal_status_gate | `script:bubbles/scripts/strict-terminal-status-guard-selftest.sh`, `script:bubbles/scripts/strict-terminal-status-guard.sh` | 0 | Check 32 | 7 | guard |
| G093 | delivery_implementation_delta_gate | `script:bubbles/scripts/delivery-implementation-delta-guard-selftest.sh`, `script:bubbles/scripts/delivery-implementation-delta-guard.sh` | 0 | Check 29B | 3 | guard |
| G094 | capability_foundation_gate | `script:bubbles/scripts/capability-foundation-guard-selftest.sh`, `script:bubbles/scripts/capability-foundation-guard.sh` | 0 | Check 34 | 5 | guard |
| G095 | discovered_issue_disposition_gate | `script:bubbles/scripts/discovered-issue-disposition-guard-selftest.sh`, `script:bubbles/scripts/discovered-issue-disposition-guard.sh` | 0 | Check 35 | 8 | guard |
| G097 | requirement_mechanism_correspondence_gate | `script:bubbles/scripts/requirement-mechanism-guard-selftest.sh`, `script:bubbles/scripts/requirement-mechanism-guard.sh` | 0 | Check 36 | 7 | guard |
| G098 | observability_posture_declared_gate | `script:bubbles/scripts/observability-posture-guard-selftest.sh`, `script:bubbles/scripts/observability-posture-guard.sh` | 0 | Check 37 | 8 | guard |
| G099 | observability_opt_out_freshness_gate | `script:bubbles/scripts/observability-opt-out-guard-selftest.sh`, `script:bubbles/scripts/observability-opt-out-guard.sh` | 0 | Check 38 | 10 | guard |
| G100 | observability_slo_evidence_gate | `script:bubbles/scripts/observability-slo-guard-selftest.sh`, `script:bubbles/scripts/observability-slo-guard.sh` | 0 | Check 39 | 7 | guard |
| G101 | release_delivery_reconciliation_gate | `script:bubbles/scripts/is-terminal-for-mode.sh`, `script:bubbles/scripts/release-delivery-reconciliation-guard-selftest.sh`, `script:bubbles/scripts/release-delivery-reconciliation-guard.sh`, `script:bubbles/scripts/scenario-compile-lint.sh` | 0 | — | 10 | — |
| G110 | release_train_discipline_gate | `script:bubbles/scripts/release-train-guard.sh` | 8 | — | 5 | — |
| G111 | flag_default_off_on_other_trains_gate | `script:bubbles/scripts/release-train-guard.sh` | 5 | — | 2 | — |
| G112 | backup_evidence_required_gate | `mode-required` | 2 | — | — | — |
| G113 | restore_drill_evidence_gate | `mode-required` | 2 | — | — | — |
| G114 | bcdr_evidence_gate | `mode-required` | 1 | — | — | — |
| G115 | env_pollution_isolation_gate | `script:bubbles/scripts/env-pollution-scan.sh` | 3 | — | 4 | — |
| G116 | offsite_backup_required_for_prod_trains_gate | `mode-required` | 1 | — | — | — |
| G117 | audit_trail_immutable_gate | `mode-required` | 12 | — | — | — |
| G118 | backup_retention_declared_gate | `script:bubbles/scripts/release-train-guard.sh` | 3 | — | 2 | — |
| G119 | secret_rotation_recorded_gate | `mode-required` | 2 | — | — | — |
| G120 | pii_classification_declared_gate | `script:bubbles/scripts/release-train-guard.sh` | 2 | — | 2 | — |
| G121 | propagation_policy_declared_gate | `script:bubbles/scripts/propagation-policy-guard.sh` | 3 | — | 2 | — |
| G122 | propagation_validation_required_gate | `script:bubbles/scripts/propagation-policy-guard.sh` | 2 | — | 2 | — |
| G123 | propagation_ledger_recorded_gate | `script:bubbles/scripts/propagation-policy-guard.sh` | 2 | — | 5 | — |
| G124 | incident_severity_declared_gate | `mode-required` | 1 | — | 1 | — |
| G125 | framework_health_evidence_gate | `script:bubbles/scripts/framework-health-evidence-lint.sh`, `script:bubbles/scripts/retro-framework-health.sh` | 1 | — | 4 | — |
| G126 | model_tier_floor_gate | `script:bubbles/scripts/model-tier-advisory-selftest.sh`, `script:bubbles/scripts/model-tier-advisory.sh` | 0 | — | 4 | — |
| G127 | capability_consumer_freshness_gate | `script:bubbles/scripts/capability-consumer-freshness-selftest.sh`, `script:bubbles/scripts/capability-consumer-freshness.sh` | 0 | — | 5 | — |
| G128 | session_cap_enforcement_gate | `script:bubbles/scripts/session-cap-guard-selftest.sh`, `script:bubbles/scripts/session-cap-guard.sh` | 0 | Check 40 | 10 | guard |
| G129 | repository_binding_classification_discovery_conformance_gate | `script:bubbles/scripts/repository-binding-conformance-guard.sh` | 0 | — | 2 | — |
| G130 | domain_invariant_correspondence_gate | `script:bubbles/scripts/domain-invariant-guard-selftest.sh`, `script:bubbles/scripts/domain-invariant-guard.sh` | 0 | Check 41 | 4 | guard |
| G131 | domain_model_consistency_gate | `script:bubbles/scripts/domain-model-consistency-selftest.sh`, `script:bubbles/scripts/domain-model-consistency.sh` | 0 | Check 42 | 5 | guard |
| G132 | reference_existence_gate | `script:bubbles/scripts/reference-existence-lint-selftest.sh`, `script:bubbles/scripts/reference-existence-lint.sh` | 0 | — | 4 | — |
| G133 | collected_test_count_gate | `script:bubbles/scripts/collected-test-count-guard.sh`, `script:bubbles/scripts/collected-test-count-guard-selftest.sh` | 0 | — | 3 | — |
| G134 | goal_fidelity_gate | `script:bubbles/scripts/goal-fidelity-guard.sh`, `script:bubbles/scripts/goal-fidelity-guard-selftest.sh` | 0 | — | 5 | — |
| G135 | autonomy_posture_gate | `script:bubbles/scripts/autonomy-posture-guard.sh`, `script:bubbles/scripts/autonomy-posture-guard-selftest.sh` | 0 | — | 3 | — |
| G136 | human_acceptance_terminal_gate | `script:bubbles/scripts/guards/tail-delegated-gates.sh`, `script:bubbles/scripts/state-transition-guard-selftest.sh`, `script:tests/regression/test_35_human_acceptance_terminal.sh` | 0 | Check 43 | 5 | guard |
| G137 | release_ladder_schema_gate | `script:bubbles/scripts/release-ladder-schema-guard.sh`, `script:bubbles/scripts/release-ladder-schema-guard-selftest.sh` | 0 | — | 3 | — |
| G138 | release_packet_completeness_gate | `script:bubbles/scripts/release-packet-completeness-guard.sh`, `script:bubbles/scripts/release-packet-completeness-guard-selftest.sh` | 0 | — | 3 | — |
| G139 | action_risk_classification_integrity_gate | `script:bubbles/scripts/action-risk-registry-lint.sh`, `script:bubbles/scripts/action-risk-registry-lint-selftest.sh`, `script:bubbles/scripts/pre-tool-risk-gate.sh`, `script:bubbles/scripts/pre-tool-risk-gate-selftest.sh` | 0 | — | 4 | — |
| G140 | phase_name_enum_integrity_gate | `script:bubbles/scripts/phase-name-enum-lint.sh`, `script:bubbles/scripts/phase-name-enum-lint-selftest.sh` | 0 | — | 2 | — |

## Gates Not Referenced By Any Mode

These gates are intentionally enforced OUTSIDE the mode `requiredGates` lists. Each row names the concrete non-mode surface(s) that enforce it; a row reaching the final column with no surfaces would be a genuine gap.

| Gate | Name | state-transition-guard | framework-validate scripts | CI | Enforcing script files |
| --- | --- | --- | --- | --- | --- |
| G037 | scope_size_discipline_gate | — | 1 | — | scope-context-fit-lint.sh |
| G038 | failure_recovery_containment_gate | — | 1 | — | gate-id-grep.sh |
| G041 | dod_format_integrity_gate | Check 4A, Check 4B | 2 | guard | evidence-admission-hardening-selftest.sh, v4.1.0-selftest.sh |
| G042 | artifact_ownership_enforcement_gate | Check 3G | 3 | guard | agent-ownership-lint.sh, gate-bands-selftest.sh, gate-hit-log-selftest.sh |
| G043 | consumer_trace_gate | ref | 2 | guard | expand-migrate-contract-guard.sh, imp021-interaction-contracts-selftest.sh |
| G052 | artifact_freshness_guard_gate | Check 13A | — | guard | — |
| G053 | implementation_delta_evidence_gate | Check 13B | 4 | guard | delivery-implementation-delta-guard-selftest.sh, delivery-implementation-delta-guard.sh, state-certification-reconcile.sh, state-transition-guard-selftest.sh |
| G063 | concrete_result_gate | Check 3G | 2 | guard | agent-ownership-lint.sh, state-transition-guard-selftest.sh |
| G064 | workflow_runner_authorization_gate | Check 3H | 7 | guard | agent-ownership-lint.sh, cli.sh, framework-validate.sh, scenario-compile-lint-selftest.sh, scenario-compile-lint.sh, state-transition-guard-selftest.sh, +1 more |
| G066 | phase_claim_provenance_gate | — | — | — | — |
| G067 | shared_infrastructure_blast_radius_gate | ref | 1 | guard | imp021-interaction-contracts-selftest.sh |
| G068 | dod_gherkin_content_fidelity_gate | Check 22 | 11 | guard | audit-result-contract-lint-selftest.sh, capability-freshness-selftest.sh, dod-section-lib.sh, evidence-admission-hardening-selftest.sh, scenario-match-lib-selftest.sh, scenario-match-lib.sh, +5 more |
| G069 | collateral_change_containment_gate | ref | 1 | guard | imp021-interaction-contracts-selftest.sh |
| G070 | outcome_contract_gate | — | 4 | — | generate-gate-enforcement.sh, goal-fidelity-guard-selftest.sh, goal-fidelity-guard.sh, scenario-compile-lint.sh |
| G071 | execution_only_validation_gate | — | 3 | — | gate-id-grep.sh, generate-gate-enforcement-selftest.sh, generate-gate-enforcement.sh |
| G072 | evidence_provenance_gate | Check 40 | 4 | guard | claim-source-lint.sh, domain-model-consistency.sh, framework-validate.sh, gate-strength-lint.sh |
| G075 | scope_index_parity_gate | ref | — | guard | — |
| G076 | phantom_scope_detection_gate | ref | — | guard | — |
| G077 | execution_history_plausibility_gate | ref | 1 | guard | retro-framework-health-selftest.sh |
| G078 | batch_promotion_limit_gate | — | 2 | — | batch-promotion-lint.sh, gate-enforcement.sh |
| G079 | impact_aware_validation_plan_gate | — | 1 | — | test-impact-plan.sh |
| G080 | trace_contract_evidence_gate | — | 2 | — | observability-check.sh, trace-contract-guard.sh |
| G082 | convergence_cap_enforcement_gate | Check 23 | 5 | guard | convergence-cap-guard-selftest.sh, convergence-cap-guard.sh, session-cap-guard-selftest.sh, session-cap-guard.sh, state-snapshot.sh |
| G083 | context_compaction_discipline_gate | Check 24 | 11 | guard | cli.sh, compaction-discipline-guard-selftest.sh, compaction-discipline-guard.sh, context-compactor.sh, framework-validate.sh, session-liveness-selftest.sh, +5 more |
| G084 | pre_existing_deferral_block_gate | Check 25 | 2 | guard | pre-existing-deferral-guard-selftest.sh, pre-existing-deferral-guard.sh |
| G085 | framework_dogfood_evidence_gate | Check 26 | 7 | guard | bug-packet-selftest.sh, evidence-admission-hardening-selftest.sh, framework-dogfood-guard-selftest.sh, framework-dogfood-guard.sh, observability-slo-guard.sh, scaffold-gate.sh, +1 more |
| G086 | orchestrator_persistence_lint_gate | Check 27 | 2 | guard | orchestrator-persistence-lint-selftest.sh, orchestrator-persistence-lint.sh |
| G087 | planning_packet_implementation_linkage_gate | Check 29 | 6 | guard | audit-result-contract-lint-selftest.sh, delivery-implementation-delta-guard-selftest.sh, delivery-implementation-delta-guard.sh, planning-packet-linkage-guard-selftest.sh, planning-packet-linkage-guard.sh, state-transition-guard-selftest.sh |
| G088 | post_certification_spec_edit_gate | Check 30 | 5 | guard | post-cert-spec-edit-guard-selftest.sh, post-cert-spec-edit-guard.sh, retro-framework-health-selftest.sh, strict-terminal-status-guard-selftest.sh, verify-changed-specs.sh |
| G089 | inter_spec_dependency_gate | Check 31 | 7 | guard | inter-spec-dependency-guard-selftest.sh, inter-spec-dependency-guard.sh, inter-spec-dependency-revalidation.sh, repo-drift-report-selftest.sh, repo-drift-report.sh, retro-framework-health-selftest.sh, +1 more |
| G090 | retro_convergence_health_evidence_gate | Check 33 | 6 | guard | observability-slo-guard-selftest.sh, observability-slo-guard.sh, retro-convergence-health-selftest.sh, retro-convergence-health.sh, scaffold-gate-selftest.sh, v4.1.0-selftest.sh |
| G091 | planning_workflow_chain_gate | Check 28 | 5 | guard | audit-result-contract-lint-selftest.sh, mode-resolver-selftest.sh, planning-workflow-chain-guard-selftest.sh, planning-workflow-chain-guard.sh, state-transition-guard-selftest.sh |
| G092 | strict_terminal_status_gate | Check 32 | 7 | guard | inter-spec-dependency-guard-selftest.sh, inter-spec-dependency-guard.sh, post-cert-spec-edit-guard-selftest.sh, post-cert-spec-edit-guard.sh, state-transition-guard-selftest.sh, strict-terminal-status-guard-selftest.sh, +1 more |
| G093 | delivery_implementation_delta_gate | Check 29B | 3 | guard | delivery-implementation-delta-guard-selftest.sh, delivery-implementation-delta-guard.sh, state-transition-guard-selftest.sh |
| G094 | capability_foundation_gate | Check 34 | 5 | guard | capability-foundation-guard-selftest.sh, capability-foundation-guard.sh, guard-lib.sh, rapid-tool-delivery-mode-selftest.sh, release-manifest-selftest.sh |
| G095 | discovered_issue_disposition_gate | Check 35 | 8 | guard | discovered-issue-disposition-guard-selftest.sh, discovered-issue-disposition-guard.sh, evidence-admission-hardening-selftest.sh, framework-validate.sh, gate-bands.sh, goal-fidelity-telemetry-selftest.sh, +2 more |
| G097 | requirement_mechanism_correspondence_gate | Check 36 | 7 | guard | capability-consumer-freshness.sh, domain-invariant-guard.sh, framework-validate.sh, release-delivery-reconciliation-guard.sh, requirement-mechanism-guard-selftest.sh, requirement-mechanism-guard.sh, +1 more |
| G098 | observability_posture_declared_gate | Check 37 | 8 | guard | cli.sh, framework-validate.sh, gate-hit-log-selftest.sh, observability-check.sh, observability-opt-out-guard.sh, observability-posture-guard-selftest.sh, +2 more |
| G099 | observability_opt_out_freshness_gate | Check 38 | 10 | guard | cli.sh, framework-validate.sh, gate-hit-log-selftest.sh, gate-id-grep-selftest.sh, observability-opt-out-guard-selftest.sh, observability-opt-out-guard.sh, +4 more |
| G100 | observability_slo_evidence_gate | Check 39 | 7 | guard | framework-validate.sh, gate-bands-selftest.sh, gate-bands.sh, observability-check.sh, observability-slo-guard-selftest.sh, observability-slo-guard.sh, +1 more |
| G101 | release_delivery_reconciliation_gate | — | 10 | — | adversarial-resolve.sh, framework-validate.sh, is-terminal-for-mode.sh, release-delivery-reconciliation-guard-selftest.sh, release-delivery-reconciliation-guard.sh, release-ladder-schema-guard.sh, +4 more |
| G126 | model_tier_floor_gate | — | 4 | — | framework-validate.sh, model-tier-advisory-selftest.sh, model-tier-advisory.sh, v5.2-selftest.sh |
| G127 | capability_consumer_freshness_gate | — | 5 | — | capability-consumer-freshness-selftest.sh, capability-consumer-freshness.sh, capability-consumer-naming.sh, framework-validate.sh, scaffold-gate-selftest.sh |
| G128 | session_cap_enforcement_gate | Check 40 | 10 | guard | cli.sh, framework-validate.sh, rapid-tool-delivery-mode-selftest.sh, risk-tier-resolve.sh, scaffold-gate-selftest.sh, session-cap-guard-selftest.sh, +4 more |
| G129 | repository_binding_classification_discovery_conformance_gate | — | 2 | — | framework-validate.sh, repository-binding-conformance-guard.sh |
| G130 | domain_invariant_correspondence_gate | Check 41 | 4 | guard | domain-invariant-guard-selftest.sh, domain-invariant-guard.sh, domain-model-consistency.sh, framework-validate.sh |
| G131 | domain_model_consistency_gate | Check 42 | 5 | guard | domain-model-consistency-selftest.sh, domain-model-consistency.sh, framework-validate.sh, gate-bands-selftest.sh, gate-bands.sh |
| G132 | reference_existence_gate | — | 4 | — | framework-validate.sh, generate-release-manifest.sh, reference-existence-lint-selftest.sh, reference-existence-lint.sh |
| G133 | collected_test_count_gate | — | 3 | — | collected-test-count-guard-selftest.sh, collected-test-count-guard.sh, gate-vintage-guard.sh |
| G134 | goal_fidelity_gate | — | 5 | — | framework-validate.sh, goal-boundary-receipt-selftest.sh, goal-boundary-receipt.sh, goal-fidelity-guard-selftest.sh, goal-fidelity-guard.sh |
| G135 | autonomy_posture_gate | — | 3 | — | autonomy-posture-guard-selftest.sh, autonomy-posture-guard.sh, framework-validate.sh |
| G136 | human_acceptance_terminal_gate | Check 43 | 5 | guard | acceptance-authority-lib.sh, acceptance-authority-selftest.sh, artifact-lint.sh, framework-validate.sh, state-transition-guard-selftest.sh |
| G137 | release_ladder_schema_gate | — | 3 | — | framework-validate.sh, release-ladder-schema-guard-selftest.sh, release-ladder-schema-guard.sh |
| G138 | release_packet_completeness_gate | — | 3 | — | framework-validate.sh, release-packet-completeness-guard-selftest.sh, release-packet-completeness-guard.sh |
| G139 | action_risk_classification_integrity_gate | — | 4 | — | action-risk-registry-lint-selftest.sh, action-risk-registry-lint.sh, pre-tool-risk-gate-selftest.sh, pre-tool-risk-gate.sh |
| G140 | phase_name_enum_integrity_gate | — | 2 | — | phase-name-enum-lint-selftest.sh, phase-name-enum-lint.sh |

