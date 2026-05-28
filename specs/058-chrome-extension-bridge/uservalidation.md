# User Validation Checklist — 058 Chrome Extension Bridge

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Spec authored covering outcome contract, hard constraints, and clarified NCs (1–5)
- [x] Design authored covering architecture, wire schema, dedup model, auth reuse, build/release, observability, security
- [x] Scopes authored: 5 sequential vertical slices (server endpoint + SST → server dedup → MV3 extension skeleton → build/release wiring → operator docs + devices view)
- [x] Scenario manifest authored with stable SCN-058-001..SCN-058-021 contracts mapped to live tests
- [x] Spec 060 RequireScope dependency confirmed shipped (Scopes 1+2); OQ-DSN-1 resolved upstream

Unchecked items indicate a user-reported regression.
