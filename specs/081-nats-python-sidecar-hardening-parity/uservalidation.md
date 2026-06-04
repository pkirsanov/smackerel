# User Validation Checklist — Spec 081 NATS Python Sidecar Hardening Parity

## Checklist

- [x] Baseline checklist initialized for spec 081 — NATS Python Sidecar Hardening Parity
- [x] spec.md cites spec 022 and spec 046 as substrate; cites FOLLOWUP-046-PY-SIDECAR as origin
- [x] Outcome Contract present with Intent / Success Signal / Hard Constraints / Failure Condition
- [x] Product Principle Alignment present (P5 One Graph, Many Views; P8 Trust Through Transparency)
- [x] Domain Capability Model present (AN5 — second implementation of an existing resilience capability)
- [x] 4 FRs authored (FR-081-001 SST keys, FR-081-002 ConsumerConfig threading, FR-081-003 num_delivered swap, FR-081-004 dead-letter publish)
- [x] 4 Gherkin scenarios authored (SCN-081-01..04) covering all 4 FRs
- [x] scenario-manifest.json links every SCN-081-* to scope, FR(s), and DoD items
- [x] Release Train declared (`next`) with default-off-on-mvp policy
- [x] design.md drafted with 3-file change surface, canonical header envelope, poison-pill algorithm, SST loader pattern, test plan
- [x] scopes.md single scope (SCOPE-081-01) with 11 DoD items and explicit change boundary
- [ ] bubbles.design has reviewed and reconciled Go ↔ Python header envelope names (design §3 alignment)
- [ ] bubbles.implement has shipped the 3-file change in lockstep
- [ ] live-stack integration test `test_poison_message_publishes_to_deadletter_subject` PASS
- [ ] unit tests in `ml/tests/test_nats_consumer_config.py` PASS for all 4 scenarios
- [ ] spec 046 `discoveredIssues.FOLLOWUP-046-PY-SIDECAR` marked resolved with resolutionRefs to this spec
