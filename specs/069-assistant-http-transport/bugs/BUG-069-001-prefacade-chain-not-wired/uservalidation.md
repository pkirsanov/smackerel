# BUG-069-001 User Validation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

Discovery-phase items (verified by `bubbles.bug` with real read-only commands against the current working tree) are checked `[x]`. Fix-acceptance items are unchecked `[ ]` until the fix owners (`bubbles.implement` → `bubbles.test`) complete them.

### Discovery / Documentation / Root-Cause (this packet — complete)

- [x] DV-01: The defect is real and live — `grep -n 'SetMiddleware\|PreFacadeChain' cmd/core/wiring_assistant_facade.go` shows line 314 installs the identity pass-through `func(next http.Handler) http.Handler { return next }`; `PreFacadeChain` does not appear in the file.
- [x] DV-02: The adapter read is unbounded — `grep -n 'io.ReadAll\|MaxBytesReader' internal/assistant/httpadapter/adapter.go` shows `329: body, err := io.ReadAll(r.Body)` and no `MaxBytesReader` (CWE-400 / CWE-770).
- [x] DV-03: `PreFacadeChain` is wired NOWHERE in production — `grep -rn 'PreFacadeChain' cmd/ internal/api/` returns no matches; it appears only in its definition (`internal/assistant/httpadapter/middleware.go:48`), a doc comment (`late_binding.go:36`), and `tests/`.
- [x] DV-04: The route is default-enabled and bearer-gated — `config/smackerel.yaml:995` `http.enabled: true`; `internal/api/router.go:86` mounts `/api/assistant/turn` inside the `bearerAuthMiddleware` group (router.go:74) under global `Throttle(100)` (router.go:68).
- [x] DV-05: SCOPE-2 is certified `done` despite the gap — `specs/069-assistant-http-transport/state.json` top-level `status: "done"`, `certification.completedScopes` includes `SCOPE-2`, `scopeProgress.done: 7`.
- [x] DV-06: The implementer admitted the placeholder — `cmd/core/wiring_assistant_facade.go:305-313` and `specs/069-assistant-http-transport/report.md:239` both say "SCOPE-2 will replace this pass-through with the real auth/scope/body/rate/CORS chain."
- [x] DV-07: The test gap is proven — `tests/integration/api/assistant_http_auth_test.go:122/136` build a synthetic `mountScope2Route` that hand-wires `PreFacadeChain(cfg)`; neither auth nor limits test calls `api.NewRouter`, so the production wiring is never exercised.
- [x] DV-08: Dev shared-token compatibility confirmed — `internal/auth/scope_middleware.go:71-77` shows `SessionSourceSharedToken` + `SessionSourceBootstrap` pass through `next.ServeHTTP`, so the routed fix does not break dev/test.
- [x] DV-09: The 8-artifact packet exists with substantive content (bug.md, spec.md, design.md, scopes.md, report.md, scenario-manifest.json, uservalidation.md, state.json).
- [x] DV-10: Root cause documented — Five-Whys in bug.md + test-gap analysis in design.md; fix routed (`bubbles.implement` → `bubbles.test`) without being applied.

### Fix Acceptance (owned downstream — pending)

- [ ] AC-01: `cmd/core/wiring_assistant_facade.go:314` installs `httpadapter.PreFacadeChain(transportCfg)`; the identity pass-through is gone (owner: bubbles.implement).
- [ ] AC-02: `grep -rn 'PreFacadeChain' cmd/ internal/api/` returns at least one production match (owner: bubbles.implement).
- [ ] AC-03: Real-router regression test asserts an over-cap body → 413 with no facade call (owner: bubbles.implement).
- [ ] AC-04: Real-router regression test asserts per-user budget exceeded → 429, second user unaffected (owner: bubbles.implement).
- [ ] AC-05: Real-router regression test asserts per-user PASETO without `assistant:turn` → 403; no facade call (owner: bubbles.implement).
- [ ] AC-06: Real-router regression test asserts `SessionSourceSharedToken` within limits → 200/valid envelope (no regression) (owner: bubbles.test).
- [ ] AC-07: Pre-fix RED proof captured — the real-router test FAILS against the identity-wrapper wiring (owner: bubbles.test).
- [ ] AC-08: Post-fix GREEN proof captured — the same test PASSES after the swap; broader assistant E2E/integration suite green (owner: bubbles.test).
- [ ] AC-09: `./smackerel.sh check` exits 0 after the swap; no collateral regressions (owner: bubbles.test).
- [ ] AC-10: `internal/assistant/httpadapter/middleware.go`, `adapter.go`, `late_binding.go`, `config/smackerel.yaml`, `internal/auth/`, and every other `specs/` folder remain unchanged by the fix commit (owner: bubbles.implement / bubbles.test).

## One-To-One Finding Closure Accounting

- **F1 (HIGH, primary — `PreFacadeChain` not wired into the live route):** documented in this packet; to be closed by the one-line `SetMiddleware(PreFacadeChain(transportCfg))` swap (owner: bubbles.implement) + real-router regression test (owner: bubbles.implement → bubbles.test).

A single finding with a single root cause and a single Scope 1; no cherry-picking. This packet is a discovery + documentation deliverable: `bubbles.bug` verified the defect with real commands and routed the fix without performing it.

— bubbles.bug (autonomous), `bugfix-fastlane` mode, stochastic-quality-sweep round R41 (harden-to-doc), 2026-06-16
