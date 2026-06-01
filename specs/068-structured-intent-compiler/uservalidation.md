# User Validation — Spec 068 Structured Intent Compiler

Links: [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] Baseline checklist initialized for the planning packet.
- [x] Scope ordering places compiler schema/config/trace foundation before read, write, and guard overlays.
- [x] Planned UX covers clarification, trace inspection, and side-effect confirmation from compiled intent fields.
- [x] Scope 1 HTTP-route e2e for SCN-068-A06 and SCN-068-A07 is deferred to spec 069 wire-up (Smackerel has no assistant HTTP ingress until spec 069 ships); the scenarios remain authored in this spec.
- [x] Scopes 2, 3, and 4 mirror the Scope 1a split: HTTP-route e2e for SCN-068-A01, SCN-068-A02, SCN-068-A03, SCN-068-A04, SCN-068-A05, and SCN-068-A09 is deferred to spec 069 wire-up, and in-process `Facade.Handle` integration tests provide the in-scope coverage that can land now. SCN-068-A08 (raw-route bypass guard) keeps live guard + policy-guard e2e coverage in this spec because it is source-scanning policy behavior, not transport-bound.
- [x] No owner-reported regression is recorded in this planning pass.

## Owner Sign-Off

Owner sign-off occurs after delivery evidence exists in [report.md](report.md).
