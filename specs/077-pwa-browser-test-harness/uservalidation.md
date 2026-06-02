# User Validation Checklist

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Spec 077 created as the foundation spec closing ops packet `specs/_ops/F-057-V-001-e2e-ui-harness`
- [x] Three-scope plan agreed (runner foundation + discovery/CI/docs + first real consumer)
- [x] First real consumer is spec 057 SCOPE-4 login flow plus CSP smoke (not a synthetic placeholder)
- [ ] Local `./smackerel.sh test e2e-ui` smoke executed against a fresh test-stack bringup after Scope 1
- [ ] CI `e2e-ui` workflow observed green on a push and on a PR after Scope 2
- [ ] CI `e2e-ui` workflow observed red (and blocking merge) on a deliberately broken PR after Scope 2
- [ ] Spec 057 SCOPE-4 rows 4.1-4.5 reviewed for behavior parity by the Smackerel owner after Scope 3
- [ ] Ops packet `specs/_ops/F-057-V-001-e2e-ui-harness/README.md` marked `routed_resolved` with spec 077 reference after Scope 3 closes
- [ ] Follow-up rework rows opened on spec 073 (TP-073-09) and spec 075 (TP-075-09) to port their deferred Playwright coverage onto the harness

Unchecked items indicate validation pending after implementation completes.
