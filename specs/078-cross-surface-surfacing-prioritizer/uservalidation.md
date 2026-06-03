# User Validation — Cross-Surface Surfacing Prioritizer

**Status:** Auto-acknowledged — adopt-existing infrastructure with no user-facing change.

## Checklist

- [x] Existing surfacing controller code (`internal/intelligence/surfacing/`) adopted under spec 078 ownership without behavior change.
- [x] Unit and e2e tests for the adopted code remain green on disposable stack (3/3 e2e PASS per `report.md#certification--2026-06-03`).
- [x] 8 `surfacing_*` Prometheus metric families exposed on the live `/metrics` endpoint.
- [x] No user-facing UX, copy, or notification surface changes introduced by this adoption.
- [x] No operator action required — adoption is pure governance bookkeeping over already-shipped code.

This spec formalizes already-shipped infrastructure
(`internal/intelligence/surfacing/`) that is invisible to end users by
design (product principle 6). The user-facing effects — bounded daily
nudge volume, no duplicate cross-channel deliveries, no re-pestering
after acknowledgement — are observed through downstream consumer specs
(025 alert-delivery, 054 digest-producer) which carry their own user
validation.

If a future change makes any surfacing decision user-visible (e.g., an
operator UI for tuning the budget), this section MUST be populated
before that change can be certified.
