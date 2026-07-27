# Gate Coverage Map

> GENERATED — do not edit by hand.
> Regenerate: `bash bubbles/scripts/generate-gate-coverage-map.sh`
> Check drift: `bash bubbles/scripts/generate-gate-coverage-map.sh --check`

This page maps every gate defined in `bubbles/registry/gates.yaml` to the surface(s) that enforce it. It exists so that gates NOT listed in any workflow mode's `requiredGates` are demonstrably enforced elsewhere (state-transition-guard checks, framework-validate selftests/guards, or CI) rather than silently unenforced.

Column meanings:

- **# Modes** — how many `modes.yaml` / `workflows.yaml` modes list the gate in `requiredGates`.
- **state-transition-guard** — the guard's labeled `Check N` that names the gate, `ref` when the gate id appears in the guard or a `bubbles/scripts/guards/**` fragment without a labeled check, else `—`.
- **framework-validate scripts** — count of OTHER `bubbles/scripts/**/*.sh` files (selftests, lints, standalone guards run by `framework-validate.sh`) that reference the gate id (the transition guard is excluded — it has its own column).
- **CI** — how a `.github/workflows/*.yml` transitively enforces the gate: `guard` (a workflow runs `state-transition-guard`), `fw-validate` (a workflow runs `framework-validate`), `named` (the gate id appears literally), else `—`.

Detection is limited to these MECHANICAL surfaces. A gate with none of them may still be enforced by AGENT-BEHAVIOR instructions (referenced only under `agents/**`, e.g. a value-first-selection or evidence-rule gate); such gates are surfaced under REVIEW so a maintainer can confirm the behavioral-only enforcement is intentional. This map is advisory — it never blocks on mode coverage completeness.

## Coverage Summary

- Gates defined: **112**
- Referenced by ≥1 workflow mode: **65**
- Not referenced by any mode: **47**
  - of those, enforced by state-transition-guard: **35**
  - of those, enforced by a framework-validate script: **38**
  - of those, enforced in CI: **35**
- Gates with NO detected MECHANICAL surface (may be agent-behavior-enforced; REVIEW): **5** — G038, G066, G071, G078, G079

## All Gates

| Gate | Name | # Modes | state-transition-guard | framework-validate scripts | CI |
| --- | --- | --- | --- | --- | --- |
| G001 | artifact_gate | 55 | — | 5 | — |
| G002 | scope_definition_gate | 23 | — | 2 | — |
| G003 | test_integrity_gate | 25 | — | 2 | — |
| G004 | test_execution_gate | 25 | — | 1 | — |
| G005 | evidence_gate | 25 | — | 1 | — |
| G006 | docs_sync_gate | 46 | — | — | — |
| G007 | validation_gate | 48 | — | — | — |
| G008 | audit_gate | 42 | — | — | — |
| G009 | chaos_gate | 26 | — | 1 | — |
| G010 | user_validation_gate | 31 | — | — | — |
| G011 | session_gate | 55 | — | — | — |
| G012 | final_promotion_gate | 55 | — | — | — |
| G013 | priority_selection_gate | 3 | — | — | — |
| G014 | bootstrap_readiness_gate | 18 | — | — | — |
| G015 | scenario_depth_gate | 24 | — | — | — |
| G016 | gherkin_e2e_coverage_gate | 24 | — | — | — |
| G018 | dod_completion_gate | 22 | — | — | — |
| G019 | sequential_spec_completion_gate | 25 | — | — | — |
| G020 | cross_agent_verification_gate | 25 | — | 1 | — |
| G021 | anti_fabrication_gate | 25 | Check 20 | 4 | guard |
| G022 | specialist_completion_gate | 25 | Check 6B | 5 | guard |
| G023 | state_transition_guard_gate | 25 | — | — | — |
| G024 | all_scopes_done_before_spec_done_gate | 25 | ref | 3 | guard |
| G025 | per_dod_item_raw_evidence_gate | 25 | — | 2 | — |
| G026 | sla_stress_coverage_gate | 22 | ref | — | guard |
| G027 | phase_scope_coherence_gate | 25 | Check 15 | 1 | guard |
| G028 | implementation_reality_scan_gate | 23 | Check 16 | 5 | guard |
| G029 | integration_completeness_gate | 23 | — | 2 | — |
| G031 | findings_artifact_update_gate | 9 | — | 1 | — |
| G032 | business_analysis_gate | 4 | — | — | — |
| G033 | design_readiness_gate | 22 | — | — | — |
| G034 | security_gate | 22 | — | — | — |
| G035 | vertical_slice_gate | 22 | — | — | — |
| G036 | red_green_traceability_gate | 2 | — | — | — |
| G037 | scope_size_discipline_gate | 0 | — | 1 | — |
| G038 | failure_recovery_containment_gate | 0 | — | — | — |
| G040 | incomplete_work_language_gate | 25 | Check 18 | 5 | guard |
| G041 | dod_format_integrity_gate | 0 | Check 4A, Check 4B | 2 | guard |
| G042 | artifact_ownership_enforcement_gate | 0 | Check 3G | 1 | guard |
| G043 | consumer_trace_gate | 0 | ref | 2 | guard |
| G044 | comprehensive_regression_gate | 25 | — | 8 | — |
| G047 | idor_auth_bypass_gate | 25 | — | 2 | — |
| G048 | silent_decode_failure_gate | 25 | — | 2 | — |
| G051 | test_env_dependency_gate | 25 | Check 19 | 2 | guard |
| G052 | artifact_freshness_guard_gate | 0 | Check 13A | — | guard |
| G053 | implementation_delta_evidence_gate | 0 | Check 13B | 3 | guard |
| G055 | policy_provenance_gate | 25 | Check 3A | 3 | guard |
| G056 | validate_certification_gate | 25 | Check 3H | 1 | guard |
| G057 | scenario_manifest_gate | 25 | Check 3C | 1 | guard |
| G058 | lockdown_gate | 1 | Check 3D | — | guard |
| G059 | regression_contract_gate | 25 | Check 3D | 1 | guard |
| G060 | scenario_tdd_gate | 3 | Check 3E | 4 | guard |
| G061 | rework_packet_gate | 25 | Check 3F | — | guard |
| G063 | concrete_result_gate | 0 | Check 3G | 2 | guard |
| G064 | workflow_runner_authorization_gate | 0 | Check 3H | 6 | guard |
| G066 | phase_claim_provenance_gate | 0 | — | — | — |
| G067 | shared_infrastructure_blast_radius_gate | 0 | ref | 1 | guard |
| G068 | dod_gherkin_content_fidelity_gate | 0 | Check 22 | 7 | guard |
| G069 | collateral_change_containment_gate | 0 | ref | 1 | guard |
| G070 | outcome_contract_gate | 0 | — | 1 | — |
| G071 | execution_only_validation_gate | 0 | — | — | — |
| G072 | evidence_provenance_gate | 0 | Check 40 | 4 | guard |
| G073 | planning_only_source_edit_lockout_gate | 30 | Check 3B | 6 | guard |
| G074 | workflow_mode_consistency_gate | 3 | ref | — | guard |
| G075 | scope_index_parity_gate | 0 | ref | — | guard |
| G076 | phantom_scope_detection_gate | 0 | ref | — | guard |
| G077 | execution_history_plausibility_gate | 0 | ref | — | guard |
| G078 | batch_promotion_limit_gate | 0 | — | — | — |
| G079 | impact_aware_validation_plan_gate | 0 | — | — | — |
| G080 | trace_contract_evidence_gate | 0 | — | 1 | — |
| G081 | build_once_deploy_many_integrity_gate | 3 | — | — | — |
| G082 | convergence_cap_enforcement_gate | 0 | Check 23 | 5 | guard |
| G083 | context_compaction_discipline_gate | 0 | Check 24 | 3 | guard |
| G084 | pre_existing_deferral_block_gate | 0 | Check 25 | 2 | guard |
| G085 | framework_dogfood_evidence_gate | 0 | Check 26 | 6 | guard |
| G086 | orchestrator_persistence_lint_gate | 0 | Check 27 | 2 | guard |
| G087 | planning_packet_implementation_linkage_gate | 0 | Check 29 | 6 | guard |
| G088 | post_certification_spec_edit_gate | 0 | Check 30 | 3 | guard |
| G089 | inter_spec_dependency_gate | 0 | Check 31 | 6 | guard |
| G090 | retro_convergence_health_evidence_gate | 0 | Check 33 | 6 | guard |
| G091 | planning_workflow_chain_gate | 0 | Check 28 | 5 | guard |
| G092 | strict_terminal_status_gate | 0 | Check 32 | 7 | guard |
| G093 | delivery_implementation_delta_gate | 0 | Check 29B | 3 | guard |
| G094 | capability_foundation_gate | 0 | Check 34 | 5 | guard |
| G095 | discovered_issue_disposition_gate | 0 | Check 35 | 6 | guard |
| G097 | requirement_mechanism_correspondence_gate | 0 | Check 36 | 7 | guard |
| G098 | observability_posture_declared_gate | 0 | Check 37 | 7 | guard |
| G099 | observability_opt_out_freshness_gate | 0 | Check 38 | 9 | guard |
| G100 | observability_slo_evidence_gate | 0 | Check 39 | 5 | guard |
| G101 | release_delivery_reconciliation_gate | 0 | — | 7 | — |
| G110 | release_train_discipline_gate | 8 | — | 1 | — |
| G111 | flag_default_off_on_other_trains_gate | 5 | — | 2 | — |
| G112 | backup_evidence_required_gate | 2 | — | — | — |
| G113 | restore_drill_evidence_gate | 2 | — | — | — |
| G114 | bcdr_evidence_gate | 1 | — | — | — |
| G115 | env_pollution_isolation_gate | 3 | — | 4 | — |
| G116 | offsite_backup_required_for_prod_trains_gate | 1 | — | — | — |
| G117 | audit_trail_immutable_gate | 12 | — | — | — |
| G118 | backup_retention_declared_gate | 3 | — | 2 | — |
| G119 | secret_rotation_recorded_gate | 2 | — | — | — |
| G120 | pii_classification_declared_gate | 2 | — | 2 | — |
| G121 | propagation_policy_declared_gate | 3 | — | 2 | — |
| G122 | propagation_validation_required_gate | 2 | — | 2 | — |
| G123 | propagation_ledger_recorded_gate | 2 | — | 3 | — |
| G124 | incident_severity_declared_gate | 1 | — | 1 | — |
| G125 | framework_health_evidence_gate | 1 | — | — | — |
| G126 | model_tier_floor_gate | 0 | — | 4 | — |
| G127 | capability_consumer_freshness_gate | 0 | — | 4 | — |
| G128 | session_cap_enforcement_gate | 0 | Check 40 | 7 | guard |
| G129 | repository_binding_classification_discovery_conformance_gate | 0 | — | 1 | — |
| G130 | domain_invariant_correspondence_gate | 0 | Check 41 | 4 | guard |
| G131 | domain_model_consistency_gate | 0 | Check 42 | 3 | guard |

## Gates Not Referenced By Any Mode

These gates are intentionally enforced OUTSIDE the mode `requiredGates` lists. Each row names the concrete non-mode surface(s) that enforce it; a row reaching the final column with no surfaces would be a genuine gap.

| Gate | Name | state-transition-guard | framework-validate scripts | CI | Enforcing script files |
| --- | --- | --- | --- | --- | --- |
| G037 | scope_size_discipline_gate | — | 1 | — | scope-context-fit-lint.sh |
| G038 | failure_recovery_containment_gate | — | — | — | — |
| G041 | dod_format_integrity_gate | Check 4A, Check 4B | 2 | guard | evidence-admission-hardening-selftest.sh, v4.1.0-selftest.sh |
| G042 | artifact_ownership_enforcement_gate | Check 3G | 1 | guard | agent-ownership-lint.sh |
| G043 | consumer_trace_gate | ref | 2 | guard | expand-migrate-contract-guard.sh, imp021-interaction-contracts-selftest.sh |
| G052 | artifact_freshness_guard_gate | Check 13A | — | guard | — |
| G053 | implementation_delta_evidence_gate | Check 13B | 3 | guard | delivery-implementation-delta-guard-selftest.sh, delivery-implementation-delta-guard.sh, state-transition-guard-selftest.sh |
| G063 | concrete_result_gate | Check 3G | 2 | guard | agent-ownership-lint.sh, state-transition-guard-selftest.sh |
| G064 | workflow_runner_authorization_gate | Check 3H | 6 | guard | agent-ownership-lint.sh, framework-validate.sh, scenario-compile-lint-selftest.sh, scenario-compile-lint.sh, state-transition-guard-selftest.sh, workflow-runner-grants-lint.sh |
| G066 | phase_claim_provenance_gate | — | — | — | — |
| G067 | shared_infrastructure_blast_radius_gate | ref | 1 | guard | imp021-interaction-contracts-selftest.sh |
| G068 | dod_gherkin_content_fidelity_gate | Check 22 | 7 | guard | audit-result-contract-lint-selftest.sh, capability-freshness-selftest.sh, dod-section-lib.sh, evidence-admission-hardening-selftest.sh, state-transition-guard-selftest.sh, state-transition-required-specialists-selftest.sh, +1 more |
| G069 | collateral_change_containment_gate | ref | 1 | guard | imp021-interaction-contracts-selftest.sh |
| G070 | outcome_contract_gate | — | 1 | — | scenario-compile-lint.sh |
| G071 | execution_only_validation_gate | — | — | — | — |
| G072 | evidence_provenance_gate | Check 40 | 4 | guard | claim-source-lint.sh, domain-model-consistency.sh, framework-validate.sh, gate-strength-lint.sh |
| G075 | scope_index_parity_gate | ref | — | guard | — |
| G076 | phantom_scope_detection_gate | ref | — | guard | — |
| G077 | execution_history_plausibility_gate | ref | — | guard | — |
| G078 | batch_promotion_limit_gate | — | — | — | — |
| G079 | impact_aware_validation_plan_gate | — | — | — | — |
| G080 | trace_contract_evidence_gate | — | 1 | — | observability-check.sh |
| G082 | convergence_cap_enforcement_gate | Check 23 | 5 | guard | convergence-cap-guard-selftest.sh, convergence-cap-guard.sh, session-cap-guard-selftest.sh, session-cap-guard.sh, state-snapshot.sh |
| G083 | context_compaction_discipline_gate | Check 24 | 3 | guard | compaction-discipline-guard-selftest.sh, compaction-discipline-guard.sh, context-compactor.sh |
| G084 | pre_existing_deferral_block_gate | Check 25 | 2 | guard | pre-existing-deferral-guard-selftest.sh, pre-existing-deferral-guard.sh |
| G085 | framework_dogfood_evidence_gate | Check 26 | 6 | guard | evidence-admission-hardening-selftest.sh, framework-dogfood-guard-selftest.sh, framework-dogfood-guard.sh, observability-slo-guard.sh, scaffold-gate.sh, state-transition-guard-selftest.sh |
| G086 | orchestrator_persistence_lint_gate | Check 27 | 2 | guard | orchestrator-persistence-lint-selftest.sh, orchestrator-persistence-lint.sh |
| G087 | planning_packet_implementation_linkage_gate | Check 29 | 6 | guard | audit-result-contract-lint-selftest.sh, delivery-implementation-delta-guard-selftest.sh, delivery-implementation-delta-guard.sh, planning-packet-linkage-guard-selftest.sh, planning-packet-linkage-guard.sh, state-transition-guard-selftest.sh |
| G088 | post_certification_spec_edit_gate | Check 30 | 3 | guard | post-cert-spec-edit-guard-selftest.sh, post-cert-spec-edit-guard.sh, strict-terminal-status-guard-selftest.sh |
| G089 | inter_spec_dependency_gate | Check 31 | 6 | guard | inter-spec-dependency-guard-selftest.sh, inter-spec-dependency-guard.sh, inter-spec-dependency-revalidation.sh, repo-drift-report-selftest.sh, repo-drift-report.sh, strict-terminal-status-guard-selftest.sh |
| G090 | retro_convergence_health_evidence_gate | Check 33 | 6 | guard | observability-slo-guard-selftest.sh, observability-slo-guard.sh, retro-convergence-health-selftest.sh, retro-convergence-health.sh, scaffold-gate-selftest.sh, v4.1.0-selftest.sh |
| G091 | planning_workflow_chain_gate | Check 28 | 5 | guard | audit-result-contract-lint-selftest.sh, mode-resolver-selftest.sh, planning-workflow-chain-guard-selftest.sh, planning-workflow-chain-guard.sh, state-transition-guard-selftest.sh |
| G092 | strict_terminal_status_gate | Check 32 | 7 | guard | inter-spec-dependency-guard-selftest.sh, inter-spec-dependency-guard.sh, post-cert-spec-edit-guard-selftest.sh, post-cert-spec-edit-guard.sh, state-transition-guard-selftest.sh, strict-terminal-status-guard-selftest.sh, +1 more |
| G093 | delivery_implementation_delta_gate | Check 29B | 3 | guard | delivery-implementation-delta-guard-selftest.sh, delivery-implementation-delta-guard.sh, state-transition-guard-selftest.sh |
| G094 | capability_foundation_gate | Check 34 | 5 | guard | capability-foundation-guard-selftest.sh, capability-foundation-guard.sh, guard-lib.sh, rapid-tool-delivery-mode-selftest.sh, release-manifest-selftest.sh |
| G095 | discovered_issue_disposition_gate | Check 35 | 6 | guard | discovered-issue-disposition-guard-selftest.sh, discovered-issue-disposition-guard.sh, evidence-admission-hardening-selftest.sh, framework-validate.sh, scaffold-gate-selftest.sh, state-transition-guard-selftest.sh |
| G097 | requirement_mechanism_correspondence_gate | Check 36 | 7 | guard | capability-consumer-freshness.sh, domain-invariant-guard.sh, framework-validate.sh, release-delivery-reconciliation-guard.sh, requirement-mechanism-guard-selftest.sh, requirement-mechanism-guard.sh, +1 more |
| G098 | observability_posture_declared_gate | Check 37 | 7 | guard | cli.sh, framework-validate.sh, observability-check.sh, observability-opt-out-guard.sh, observability-posture-guard-selftest.sh, observability-posture-guard.sh, +1 more |
| G099 | observability_opt_out_freshness_gate | Check 38 | 9 | guard | cli.sh, framework-validate.sh, gate-id-grep-selftest.sh, observability-opt-out-guard-selftest.sh, observability-opt-out-guard.sh, observability-posture-guard-selftest.sh, +3 more |
| G100 | observability_slo_evidence_gate | Check 39 | 5 | guard | framework-validate.sh, observability-check.sh, observability-slo-guard-selftest.sh, observability-slo-guard.sh, scaffold-gate-selftest.sh |
| G101 | release_delivery_reconciliation_gate | — | 7 | — | adversarial-resolve.sh, framework-validate.sh, release-delivery-reconciliation-guard-selftest.sh, release-delivery-reconciliation-guard.sh, scaffold-gate-selftest.sh, scenario-compile-lint-selftest.sh, +1 more |
| G126 | model_tier_floor_gate | — | 4 | — | framework-validate.sh, model-tier-advisory-selftest.sh, model-tier-advisory.sh, v5.2-selftest.sh |
| G127 | capability_consumer_freshness_gate | — | 4 | — | capability-consumer-freshness-selftest.sh, capability-consumer-freshness.sh, framework-validate.sh, scaffold-gate-selftest.sh |
| G128 | session_cap_enforcement_gate | Check 40 | 7 | guard | framework-validate.sh, rapid-tool-delivery-mode-selftest.sh, risk-tier-resolve.sh, scaffold-gate-selftest.sh, session-cap-guard-selftest.sh, session-cap-guard.sh, +1 more |
| G129 | repository_binding_classification_discovery_conformance_gate | — | 1 | — | framework-validate.sh |
| G130 | domain_invariant_correspondence_gate | Check 41 | 4 | guard | domain-invariant-guard-selftest.sh, domain-invariant-guard.sh, domain-model-consistency.sh, framework-validate.sh |
| G131 | domain_model_consistency_gate | Check 42 | 3 | guard | domain-model-consistency-selftest.sh, domain-model-consistency.sh, framework-validate.sh |

