# User Validation — Spec 090 Observability SLO Dogfood

Items are CHECKED `[x]` by default (validated via the session's captured
evidence). Uncheck `[ ]` to report broken behavior.

- [x] Smackerel declares a real `core.health` SLO contract in `.github/bubbles-project.yaml` (targets p99≤200ms, error≤0.1%, availability≥99.9%)
- [x] `scripts/observability/capture-slo.sh` captures REAL per-request latency/status from a live endpoint and emits G100-schema evidence
- [x] The capture tool refuses to fabricate evidence when the endpoint is down (preflight exit 1, no file)
- [x] Real telemetry captured against the live smackerel core `/api/health` MEETS the contract (p99 organically ~26ms, 0% error, 100% availability)
- [x] Bubbles G100 (`observability_slo_evidence_gate`) transitions from no-op to ENFORCED for Smackerel and passes green against the captured evidence
- [x] No bypass flag exists in the capture tool; smackerel `pii-scan.sh` stays green; my files are PII-clean
