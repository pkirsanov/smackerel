# User Validation: BUG-058-003

**Reported by:** Orchestrator directive — a genuine flake discovered while re-verifying spec 058's `./smackerel.sh test e2e-ext` discharge claim
**Validated:** 2026-06-09

## Acceptance

- [x] AC-1 — `triggerDrain()` waits INSIDE the `sw.evaluate` worker context for `chrome.alarms.create` to be a function before calling it (no cross-process check-then-use gap), then fires the drain alarm.
- [x] AC-2 — the wait is bounded (≤ 5s, 50ms poll) and rejects LOUDLY with a clear MV3-lifecycle error on timeout (no silent hang, no masking).
- [x] AC-3 — `test/e2e/playwright.config.ts` keeps `retries: 0`; the flake is fixed at the race, not papered over with retries.
- [x] AC-4 — `sideload_smoke.spec.ts`'s `expect(badge).toBe("SETUP")` is unchanged; the SETUP-badge contract is not weakened.
- [x] AC-5 — proven non-flaky: 20/20 consecutive separate cold invocations of the isolated `badge matrix` test (≥ 10 required) PASS, and the full `./smackerel.sh test e2e-ext` suite passes 11/11.
- [x] AC-6 — product code untouched: no change to `src/background/index.ts`, `manifest.json`, or any production source.
- [x] AC-7 — all verification ran through the sanctioned `./smackerel.sh test e2e-ext` CLI surface.

## Notes

This is a **test-harness reliability** fix, not a product change. The flake was a
Manifest-V3 service-worker lifecycle race in the `triggerDrain()` Playwright
helper: on a cold service-worker spin-up the permission-gated `chrome.alarms`
binding is transiently `undefined` while the base `chrome` namespace already
exists, so the unguarded `chrome.alarms.create(...)` threw
`Cannot read properties of undefined (reading 'create')` and — with
`retries: 0` — failed the whole suite ~1-in-6 cold runs.

Two deliberate no-shortcut decisions are recorded in `design.md`: (1) the
readiness check is performed **inside** the worker evaluate (closing the
cross-process check-then-use window rather than reopening it with a
Playwright-side two-step); (2) **no Playwright `retries`** were added and the
assertion was **not** weakened — masking a flake is forbidden; the race itself is
fixed.

**Honest scope caveat (does not overclaim):** fixing this flake makes the
`e2e-ext` harness reliable but does **NOT** unblock the parent spec 058. Spec 058
remains `blocked` on its genuinely-irreducible keyless-OIDC `cosign verify-blob`
against a real Rekor transparency log (operator / CI-release-gated; faking it
would be forbidden Rekor pollution). This packet does not touch the parent spec's
`spec.md`/`design.md`/`scopes.md` planning truth.

**Commit ownership:** the worktree is left ready for the orchestrator to review +
commit; this packet does NOT commit. See `report.md` → "Commit-Ordering Note"
for the Gate G088 ordering the committer must honor.
