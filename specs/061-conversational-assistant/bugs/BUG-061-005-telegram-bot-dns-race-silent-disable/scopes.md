# BUG-061-005 — Scopes

Status: done

---

## Scope 01 — Retry + fail-loud in `cmd/core/wiring.go`

**Status:** Done (shipped in commit `96acf294`, deployed
2026-06-09T15:33:39Z)

**Depends on:** none

**Implementation:** see `design.md` § Solution design. Diff lives in
`cmd/core/wiring.go::startTelegramBotIfConfigured()`.

**Verification (already exercised):**

- `go build ./cmd/core/` — clean
- `go vet ./cmd/core/` — clean
- Live deploy via ci-keyless promote — bot started successfully at
  2026-06-09T15:04:57Z (DNS was healthy; no retries needed in the
  happy path)

**Definition of Done:**

- [x] Retry loop with bounded backoff
- [x] Fail-loud exit on exhaustion
- [x] Committed and pushed to `smackerel/main`
- [x] CI green
- [x] Deployed to <deploy-host>
- [x] Post-deploy verification: bot active
- [ ] Live exercise of the retry path under forced DNS failure
      (deferred to a chaos drill)
- [x] Build Quality Gate: go build + go vet clean
