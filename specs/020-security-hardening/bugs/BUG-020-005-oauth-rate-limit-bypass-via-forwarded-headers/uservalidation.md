# User Validation: BUG-020-005 — OAuth rate-limit bypass via X-Forwarded-For / X-Real-IP / True-Client-IP

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Acceptance Checklist

- [x] F-SEC-R30-001 closed — the OAuth start/callback per-IP rate limit (R-004) is no longer bypassable by rotating `X-Forwarded-For`, `X-Real-IP`, or `True-Client-IP`. With the SST default (`runtime.trusted_proxies: []`), all forwarded headers are ignored and `r.RemoteAddr` keeps its raw TCP-peer value, so `httprate.LimitByIP` keys on the real network peer.
- [x] Trusted-proxy honor path preserved — when an operator deploys behind a known reverse proxy and populates `runtime.trusted_proxies` with the proxy's CIDR, `X-Forwarded-For` / `X-Real-IP` / `True-Client-IP` are honoured (header precedence: True-Client-IP → X-Real-IP → XFF leftmost), and the per-IP rate limit attributes correctly to the real client.
- [x] Untrusted-peer rejection — when `runtime.trusted_proxies` is non-empty but the connecting TCP peer is NOT in any configured CIDR, forwarded headers are ignored exactly as in the empty-allowlist case.
- [x] Malformed CIDR safety — a typo in `runtime.trusted_proxies` is logged at `ERROR` and dropped; sibling well-formed CIDRs continue to grant trust. A single operator typo cannot silently disable the gate, nor silently grant universal trust.
- [x] Adversarial proof recorded — the two router-level R30 regression tests FAIL when `internal/api/router.go` line 24 is reverted to `r.Use(middleware.RealIP)` and PASS when the gated middleware is restored (transcripts in [report.md](report.md) → "Security R30 adversarial fidelity proof").
- [x] SST compliance (gate G028) — `runtime.trusted_proxies` flows through `config/smackerel.yaml` → `scripts/commands/config.sh` → `config/generated/<env>.env` (as `RUNTIME_TRUSTED_PROXIES`) → `internal/config/config.go` → `cmd/core/wiring.go` → `api.Dependencies`. No `${VAR:-default}` substitution introduced; the production fail-loud surface is the empty-allowlist behaviour of the middleware itself.
- [x] No public-API contract change — only one internal middleware was swapped at the router root; OAuth-handler signature and rate-limit budget are unchanged.
- [x] No DB-schema change.
- [x] All pre-existing `internal/api/...` and `internal/config/...` tests continue to pass.
- [x] `go vet` and `gofmt -l` clean on every touched file.
- [x] Parent spec 020 artifacts updated with security R30 cross-reference (`state.json` execution history + `report.md` section).

## Sign-off

This bug closure is the parent-expanded child workflow execution of `security-to-doc` mode for spec 020 within sweep `sweep-2026-05-23-r30` round 15. The security probe ran inside the same workflow that planned, implemented, tested, validated, and audited the fix in one round. The user acceptance is implicit in the workflow contract: round 15 reaches `completed_owned` only after F-SEC-R30-001 closes with a passing regression test that would fail if the fix were reverted, which the adversarial proof above demonstrates.
