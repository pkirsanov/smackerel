# User Validation: BUG-058-INGEST-SCOPE-403

**Reported by:** Stochastic Quality Sweep Round 16 (harden lens, parent-expanded)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — canonical `"extension:bookmarks,history"` per-user token reaches the ingest handler (not 403) through the real `NewRouter`.
- [x] AC-2 — a per-user token without the canonical scope is still rejected `403`.
- [x] AC-3 — `router_extension_scope_test.go` fails if the gate is reverted to two scopes (adversarial re-RED proven).
- [x] AC-4 — `router.go` wires `auth.RequireScope("extension:bookmarks,history")`; comment matches the spec 060 / spec 058 contract.
- [x] AC-5 — no token-format change, no schema change, no bypass change; full `internal/api` package green (`ok 9.435s`).

## Notes

The defect was a router wiring error (a single comma-joined capability scope
split into two separate scopes), not a flaw in `auth.RequireScope`. The fix
restores the documented single-scope contract and is guarded end-to-end. The
production impact (every real per-user extension token 403'd) is removed.
