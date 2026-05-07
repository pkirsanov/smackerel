# Report: QF Companion Connector

## Summary

Scope 1 implementation was reconciled against the active QF 063 read contract and validated through the feasible Smackerel runtime test surface. The implementation adds or verifies the `qf-decisions` connector configuration boundary, registry/startup wiring, QF private read client DTO contract, health behavior, and no-publication behavior for schema mismatch.

Only Scope 1 has implementation evidence in this report. No final `done` status is claimed here.

## Planning Inputs

- `spec.md`: Feature specification for Smackerel QF Companion Connector.
- `design.md`: Smackerel connector design for `qf-decisions`.
- Related QF feature: `<home>/quantitativeFinance/specs/063-smackerel-companion-bridge`.
- QF pre-MVP release docs: `<home>/quantitativeFinance/docs/releases/pre-mvp/features.md` and `<home>/quantitativeFinance/docs/releases/pre-mvp/actions.md`.

## Execution Evidence

### Code Diff Evidence

**Claim Source:** executed

Implemented and validated Scope 1 behavior in the existing dirty worktree. The implementation-owned patch in this pass updated the QF DTO contract and tests so the active QF 063 implementation is mirrored: `contract_version` on packet envelopes, string `consent_scope`, string-array `extracted_claims`, required `target_context`, and optional string-array `source_refs` semantics. Existing dirty Scope 1 files in the worktree already included connector registration, config generation, connector/client scaffolding, integration tests, and e2e tests; these were validated without reverting unrelated files.

```text
git status --short
 M .github/bubbles/scripts/implementation-reality-scan.sh
 M cmd/core/connectors.go
 M config/smackerel.yaml
 M internal/config/config.go
 M internal/config/validate_test.go
 M ml/tests/test_ocr.py
 M scripts/commands/config.sh
 M scripts/runtime/python-format.sh
 M scripts/runtime/python-lint.sh
 M scripts/runtime/python-unit.sh
?? internal/connector/qfdecisions/
?? specs/041-qf-companion-connector/
?? tests/e2e/qf_decisions_connector_api_test.go
?? tests/integration/qf_decisions_connector_config_test.go
```

### RED Proof Note

**Claim Source:** interpreted

Before the DTO patch, `./smackerel.sh test unit` failed in `internal/connector/qfdecisions` on the newly added contract assertions: missing `QFDecisionPacketEnvelope.ContractVersion`, `packet.ContractVersion` undefined, and stale `PersonalEvidenceBundle` field types for `ConsentScope` and `ExtractedClaims`. The exact raw terminal resource for this RED run was not recoverable after the conversation context compaction; the GREEN unit evidence below is the executable post-fix proof.

## Test Evidence

### Scope 1 Unit Evidence

**Claim Source:** executed

Command: `./smackerel.sh test unit`

```text
ok      github.com/smackerel/smackerel/cmd/core 0.438s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        (cached)
ok      github.com/smackerel/smackerel/internal/agent   (cached)
ok      github.com/smackerel/smackerel/internal/agent/render    (cached)
ok      github.com/smackerel/smackerel/internal/agent/userreply (cached)
ok      github.com/smackerel/smackerel/internal/annotation      (cached)
ok      github.com/smackerel/smackerel/internal/api     (cached)
ok      github.com/smackerel/smackerel/internal/auth    (cached)
ok      github.com/smackerel/smackerel/internal/config  (cached)
ok      github.com/smackerel/smackerel/internal/connector       (cached)
ok      github.com/smackerel/smackerel/internal/connector/alerts        (cached)
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     (cached)
ok      github.com/smackerel/smackerel/internal/connector/browser       (cached)
ok      github.com/smackerel/smackerel/internal/connector/caldav        (cached)
ok      github.com/smackerel/smackerel/internal/connector/discord       (cached)
ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
ok      github.com/smackerel/smackerel/internal/connector/imap  (cached)
ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
ok      github.com/smackerel/smackerel/internal/connector/maps  (cached)
ok      github.com/smackerel/smackerel/internal/connector/markets       (cached)
ok      github.com/smackerel/smackerel/internal/connector/photos        (cached)
ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   0.131s
409 passed in 18.08s
```

### Scope 1 Integration Evidence

**Claim Source:** executed

Command: `./smackerel.sh test integration`

```text
Preparing disposable test stack...
[+] Running 7/7
 ✔ Network smackerel-test_default             Created                      0.7s 
 ✔ Volume "smackerel-test-postgres-data"      Created                      0.0s 
 ✔ Volume "smackerel-test-nats-data"          Created                      0.0s 
 ✔ Container smackerel-test-nats-1            Healthy                     11.9s 
 ✔ Container smackerel-test-postgres-1        Healthy                     11.9s 
 ✔ Container smackerel-test-smackerel-ml-1    Healthy                     16.3s 
 ✔ Container smackerel-test-smackerel-core-1  Healthy                     16.7s 
{"status":"degraded","version":"dev","commit_hash":"unknown","build_time":"unknown","services":{"alert_delivery":{"status":"up"},"api":{"status":"up","uptime_seconds":2},"connector:qf-decisions":{"status":"disconnected"},"intelligence":{"status":"up"},"ml_sidecar":{"status":"up","model_loaded":true},"nats":{"status":"up"},"postgres":{"status":"up","artifact_count":0}},"knowledge":{"concept_count":0,"entity_count":0,"synthesis_pending":0}}
=== RUN   TestQFDecisionsConnectorConfigRegistryAndHealthIntegration
--- PASS: TestQFDecisionsConnectorConfigRegistryAndHealthIntegration (0.03s)
=== RUN   TestQFDecisionsConnectorSchemaMismatchIntegration
--- PASS: TestQFDecisionsConnectorSchemaMismatchIntegration (0.01s)
=== RUN   TestQFDecisionsConnectorAuthFailureIntegration
--- PASS: TestQFDecisionsConnectorAuthFailureIntegration (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/drive  8.843s
```

### Scope 1 E2E API Evidence

**Claim Source:** executed

Command: `./smackerel.sh test e2e`

```text
=========================================
	Shell E2E Test Results
=========================================
	PASS: test_timeout_process_cleanup.sh
	PASS: test_compose_start.sh
	PASS: test_persistence.sh
	PASS: test_postgres_readiness_gate.sh
	PASS: test_config_fail.sh
	PASS: test_connector_framework.sh
	PASS: test_settings_connectors.sh
	Total:  35
	Passed: 35
	Failed: 0
=========================================
{"status":"degraded","version":"dev","commit_hash":"unknown","build_time":"unknown","services":{"alert_delivery":{"status":"up"},"api":{"status":"up","uptime_seconds":1},"connector:qf-decisions":{"status":"disconnected"},"intelligence":{"status":"up"},"ml_sidecar":{"status":"up","model_loaded":true},"nats":{"status":"up"},"postgres":{"status":"up","artifact_count":0}},"knowledge":{"concept_count":0,"entity_count":0,"synthesis_pending":0}}
=== RUN   TestQFDecisionsConnectorHealthAppearsInLiveAPI
--- PASS: TestQFDecisionsConnectorHealthAppearsInLiveAPI (0.09s)
=== RUN   TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts
--- PASS: TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts (0.11s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        99.673s
PASS: go-e2e
```

## Validation Evidence

### Scope 1 Check Evidence

**Claim Source:** executed

Command: `./smackerel.sh check`

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 4, rejected: 0
scenario-lint: OK
```

### Scope 1 Artifact Lint Evidence

**Claim Source:** executed

Command: `bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector`

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: full-delivery
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
```

### Scope 1 Implementation Reality Evidence

**Claim Source:** executed

Command: `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/041-qf-companion-connector --verbose`

```text
ℹ️  INFO: Scopes yielded 0 files — falling back to design.md for file discovery
⚠️  WARN: Resolved 12 file(s) from design.md fallback — scopes.md should reference these directly
ℹ️  INFO: Resolved 12 implementation file(s) to scan
--- Scan 1: Gateway/Backend Stub Patterns ---
--- Scan 1B: Handler / Endpoint Execution Depth ---
--- Scan 1C: Endpoint Not-Implemented / Placeholder Responses ---
--- Scan 1D: External Integration Authenticity ---
--- Scan 2: Frontend Hardcoded Data Patterns ---
--- Scan 5: Default/Fallback Value Patterns ---
--- Scan 6: Live-System Test Interception ---
--- Scan 7: IDOR / Auth Bypass Detection (Gate G047) ---
--- Scan 8: Silent Decode Failure Detection (Gate G048) ---
Files scanned:  12
Violations:     0
Warnings:       1
🟡 PASSED with 1 warning(s) — manual review advised
```

### Scope 1 Documentation Boundary Evidence

**Claim Source:** executed

Command: `grep -rn "QF remains the system of record\|companion connector" docs/`

```text
docs/smackerel.md:114:Smackerel may act as a companion surface for QuantitativeFinance (QF), but not as a financial-decision system. QF remains the system of record for intents, scenarios, decision packets, approval state, mandates, execution attempts, calibration, and provenance. Smackerel can ingest QF decision artifacts, preserve their trust metadata, surface them in digest/search/Web/Telegram experiences, and export personal context back to QF as a consent-scoped evidence bundle.
docs/smackerel.md:121:| Actions | No trade approval, mandate change, execution, or financial advice in the pre-MVP companion connector |
docs/Connector_Development.md:30:The QF Decisions connector is a companion connector, not a markets connector and not a recommendation engine. Its job is to ingest QF-owned decision artifacts and preserve their authority boundary inside Smackerel.
docs/Development.md:51:- QF companion connector (`qf-decisions`) from `specs/041-qf-companion-connector/`
```

### State Transition Guard Evidence

**Claim Source:** executed

Command: `bash .github/bubbles/scripts/state-transition-guard.sh specs/041-qf-companion-connector`

```text
--- Check 3B: Validate Certification State (Gate G056) ---
✅ PASS: state.json contains certification block
✅ PASS: Top-level status matches certification.status (in_progress)
✅ PASS: certification block records certifiedCompletedPhases
🔴 BLOCK: certification block missing scopeProgress (Gate G056)
--- Check 4: DoD Completion (Zero Unchecked) ---
ℹ️  INFO: DoD items total: 60 (checked: 10, unchecked: 50)
🔴 BLOCK: Resolved scope artifacts have 50 UNCHECKED DoD items — ALL must be [x] for 'done'
--- Check 9: DoD Evidence Presence ---
✅ PASS: All 10 checked DoD items across resolved scope files have evidence blocks
--- Check 13: Artifact Lint ---
✅ PASS: Artifact lint passes (exit 0)
--- Check 16: Implementation Reality Scan (Gate G028) ---
✅ PASS: Implementation reality scan passed — no stub/fake/hardcoded data patterns detected
--- Check 18: Deferral Language Scan (Gate G036) ---
🔴 BLOCK: Scope artifact contains 7 deferral language hit(s): scopes.md — SPEC CANNOT BE DONE WITH DEFERRED WORK (Gate G040)
TRANSITION BLOCKED: 38 failure(s), 2 warning(s)
state.json status MUST NOT be set to 'done'.
```

Because this guard failed before any `state.json` write, `state.json` was left unchanged by this implementation pass. Certification/state artifacts are owned by validation and planning agents.

## Plan Reconciliation Notes (2026-05-03)

A non-implementation `bubbles.plan` reconciliation pass was performed (mode: reconcile) to fold the 2026-05-03 cross-repo design deltas into `scopes.md` only. No tests were run, no DoD items were checked, no scope status was changed, and no runtime source, `spec.md`, `design.md`, or `uservalidation.md` was modified.

Reconciled scopes:

- **Scope 2 (Cursor sync, normalization, and storage):** response-level `next_cursor` is the canonical advancement value persisted in `sync_state.sync_cursor`; per-event `QFDecisionEvent.cursor` is diagnostic-only. Content-type normalization preserves `analysis_note` variants as `qf/decision-packet` with `Metadata.decision_subtype = "analysis_note"`; no other `qf/...` content type is introduced pre-MVP.
- **Scope 4 (Personal evidence bundle export):** tightened the `PersonalEvidenceBundle` required field set; `source_refs` is optional; explicit field-set parity with QF spec 063 (a Smackerel-locally-valid bundle must also pass QF import validation).
- **Scope 5 (Safety boundaries, observability, docs, tests):** added explicit reserved-schema handling per the design's "Reserved Schemas (Not Implemented Pre-MVP)" subsection — `QFApprovalAction` is normalized to `qf/approval-request` with `Metadata.reserved = true` and excluded from search/digest/recommendation/evidence-builder surfaces; any inbound `QFWatchSignal` payload is recorded as a diagnostic log only and never alters connector, packet, digest, or Telegram state.

## Final Plan Review Notes (2026-05-03)

A final non-implementation `bubbles.plan` review pass was performed after the prior plan reconciliation. The active scopes already matched the core cross-repo design, but a few outline/scenario rows still used shorthand evidence-bundle wording. This pass tightened `scopes.md` only so the Validation Checkpoints, Scope 1 DTO contract, Scope 4 scenarios, Scope 4 test titles, and Scope 4 DoD all name the canonical `PersonalEvidenceBundle` field set, keep `source_refs` optional, and avoid treating optional external references as required.

No tests were run, no DoD items were checked, no scope status was changed, and no runtime source, `spec.md`, `design.md`, or `uservalidation.md` was modified.

## Completion Statement

Status: in_progress. Scope 1 implementation and feasible validation evidence are recorded, but final certification is not claimed. `state.json` is unchanged because the transition guard did not permit state writes before certification/planning-owned artifact issues are corrected.
