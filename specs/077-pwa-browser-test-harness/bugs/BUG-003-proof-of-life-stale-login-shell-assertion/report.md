# Report: BUG-003 proof_of_life e2e-ui stale 200-branch assertion (served login shell)

## Summary

`web/pwa/tests/proof_of_life.spec.ts` asserted the served `/` document (200 branch) is the "Smackerel" PWA index shell (`title "Smackerel"`, `h1 "Smackerel"`). The REAL served `/` for an unauthenticated browser navigation is the **login shell** — spec 057 (browser-login-redirect) 303-redirects `/` → `/login?next=/`, which Playwright follows, so the served 200 document is `internal/api/admin_ui_static/login.html` (`title "Sign in — Smackerel"`, `h1 "Sign in"`). Fix: update the 200-branch to the ACTUAL live-observed served login-shell identity and correct the docstring. Test-only; zero app/runtime/source changes. Reproduced RED and confirmed GREEN live this session against the disposable `smackerel-test-e2e-ui` stack.

## Discovery

Surfaced during the BUG-077-002 `./smackerel.sh test e2e-ui` run this session and routed out-of-boundary to the parent (`../BUG-002-e2e-ui-login-ratelimit-session-reuse/report.md`: "One unrelated out-of-boundary lane failure (proof_of_life.spec.ts) routed to parent"). The stale assertion has existed since the test was authored (2026-06-02, `4072f30a`) but never executed against a live 200 until spec 100 (F-100-OPT-01/03) let the e2e-ui disposable stack boot.

## Root Cause Analysis

- `page.goto("/")` hits the Go core `baseURL` (`CORE_EXTERNAL_URL`). An unauthenticated browser navigation to `/` is `303`-redirected to `/login?next=/` by the spec-057 browser-login-redirect middleware (`internal/api/auth_browser_redirect.go`; `tests/e2e/auth/browser_login_test.go` "GET / → 303 to /login?next=/"; `specs/057-browser-login-redirect/uservalidation.md` "Browser visit to `/` redirects to `/login?next=/`").
- Playwright follows the `303`, so `response.status()` is the final `200` and the served document is `internal/api/admin_ui_static/login.html`:
  - `<title>Sign in — Smackerel</title>` (line 6)
  - `<h1>Sign in</h1>` (line 11 — single `h1`, rendered OUTSIDE the `{{if not .AuthEnabled}}` conditional, so stable in dev-bypass + auth-enabled modes)
- The disposable e2e-ui stack runs dev-token mode (`AuthConfig.Enabled=false`, per `scripts/runtime/web-e2e-ui.sh`), so `/` resolves to `200` here rather than the production-default `401`.
- The `/` → login redirect is the ratified, intended spec-057 behavior — so `/` serving the login shell is CORRECT. The staleness is entirely in the test's 200-branch expectation. This is test staleness, NOT an app bug.

## Fix Implementation

`web/pwa/tests/proof_of_life.spec.ts` 200-branch updated to the ACTUAL served login-shell identity + docstring corrected to the spec-057 login-shell reality. The `[200, 401]` tolerance is unchanged; exact-match assertions keep the branch adversarial.

## Test Evidence

### repro-before

Before-fix live run — `./smackerel.sh test e2e-ui proof_of_life.spec.ts` (good-neighbor gate CLEAN; disposable stack all containers Healthy). RED, exit 1:

```
=== good-neighbor CLEAN: bringing up e2e-ui + running ONLY proof_of_life (before-fix RED) ===
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy

Running 1 test using 1 worker

  ✘  1 …:1 › proof of life: served / route renders against the test stack (6.7s)

  1) proof_of_life.spec.ts:28:1 › proof of life: served / route renders against the test stack

    Error: Timed out 5000ms waiting for expect(locator).toHaveTitle(expected)

    Locator: locator(':root')
    Expected string: "Smackerel"
    Received string: "Sign in — Smackerel"
    Call log:
      - expect.toHaveTitle with timeout 5000ms
      - waiting for locator(':root')
        9 × locator resolved to <html lang="en">…</html>
          - unexpected value "Sign in — Smackerel"

      44 |
      45 |   if (status === 200) {
    > 46 |     await expect(page).toHaveTitle("Smackerel");
         |                        ^
      47 |     await expect(page.locator("h1")).toHaveText("Smackerel");
      48 |   }
      49 | });
        at <repo-root>/web/pwa/tests/proof_of_life.spec.ts:46:24

  1 failed
    proof_of_life.spec.ts:28:1 › proof of life: served / route renders against the test stack
[web-e2e-ui] Tearing down disposable test stack (project smackerel-test-e2e-ui)...
=== e2e-ui exit code: 1 ===
```

Observed served `/`: **status 200**, **document.title = "Sign in — Smackerel"** (em-dash), title assertion failed first so the h1 assertion (`"Smackerel"`) was not reached. This is the served login shell, matching `internal/api/admin_ui_static/login.html`.

### repro-after

After-fix live run — `./smackerel.sh test e2e-ui proof_of_life.spec.ts` (good-neighbor gate CLEAN; foreign `smackerel-test*` stack from the parallel spec-100 session had cleared; disposable stack all containers Healthy). GREEN, exit 0:

```
=== good-neighbor CLEAN: re-running ONLY proof_of_life (after-fix GREEN) ===
 Container smackerel-test-e2e-ui-smackerel-core-1  Healthy

Running 1 test using 1 worker

  ✓  1 …1 › proof of life: served / route renders against the test stack (552ms)

  1 passed (1.9s)

[web-e2e-ui] Tearing down disposable test stack (project smackerel-test-e2e-ui)...
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test-e2e-ui_default  Removed
=== e2e-ui exit code: 0 ===
```

The corrected 200-branch (`title "Sign in — Smackerel"` + `h1 "Sign in"`) passes against the real served login shell. Teardown removed ONLY the `smackerel-test-e2e-ui` project (own project) — the dev and `smackerel-test` stacks were never touched.

### phase-regression

`regression-quality-guard` (plain + `--bugfix`) on the fixed file — no silent-pass/bailout, adversarial signal, exit 0:

```
=== regression-quality-guard (plain) ===
ℹ️  Scanning web/pwa/tests/proof_of_life.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
PLAIN_EXIT=0

=== regression-quality-guard (--bugfix) ===
ℹ️  Scanning web/pwa/tests/proof_of_life.spec.ts
✅ Adversarial signal detected in web/pwa/tests/proof_of_life.spec.ts
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
  Files with adversarial signals: 1
BUGFIX_EXIT=0
```

The before-fix RED (which FAILED on the mismatched title) is the adversarial proof that the 200-branch actually executes and is not a silent-pass: a blank/error/other served `/` fails the exact-match assertions.

### phase-broader-regression

Full PWA e2e-ui suite discovers/compiles clean after the one-file change (read-only `playwright test --list`; NO stack, no execution of the parallel-owned specs). The isolated change touches zero shared helper/config/fixture, and the after-fix GREEN run already brought the full disposable stack + harness up end-to-end:

```
$ SMACKEREL_BASE_URL=http://127.0.0.1:9 npx playwright test --list
Listing tests:
  auth_login.spec.ts:70:3 › Spec 077 SCOPE-3 — Login flow + CSP smoke › TP-077-03-01 — login page renders form + CSP-clean baseline
  cardrewards_wallet.spec.ts:79:3 › Spec 083 Scope 10 — Card Rewards Wallet › SCN-083-J03 + J01 — add a custom card; wallet lists nickname, type, note, active
  chaos_saga_20260702.spec.ts:78:1 › J1 onboarding + assistant discoverability (SR-01)
  photos_health.spec.ts:31:6 › photo health dashboard renders lifecycle duplicate sensitivity and confidence metrics
  proof_of_life.spec.ts:33:1 › proof of life: served / route renders against the test stack
  qf_decisions_surface.spec.ts:15:3 › QF decision PWA surface › renders search-card contract for QF generic and trust badge cards
  unified_journey.spec.ts:27:3 › Spec 100 — Unified Journey UI Transformation › SCN-100-01/02/09 — the shared app-shell nav cross-links the assistant on the knowledge + card surfaces
Total: 52 tests in 28 files
PLAYWRIGHT_LIST_EXIT=0
```

`proof_of_life.spec.ts` is discovered and the full suite compiles (52 tests, 28 files, exit 0). The parallel-session-owned `auth_login.spec.ts` / `unified_journey.spec.ts` are NOT executed here (good-neighbor + scope isolation).

### change-boundary

Only the one test file changed; zero app/runtime/source or other-spec-test edits:

<!-- bubbles:evidence-legitimacy-skip-begin -->
```
=== git status -s ===
 M web/pwa/tests/proof_of_life.spec.ts

=== git diff --stat web/pwa/tests/proof_of_life.spec.ts ===
 web/pwa/tests/proof_of_life.spec.ts | 27 +++++++++++++++++++--------
 1 file changed, 19 insertions(+), 8 deletions(-)
```
<!-- bubbles:evidence-legitimacy-skip-end -->

### Code Diff Evidence

`git diff` of the fix (docstring + 200-branch assertions only; non-artifact runtime path `web/pwa/tests/proof_of_life.spec.ts`):

<!-- bubbles:evidence-legitimacy-skip-begin -->
```
$ git --no-pager diff web/pwa/tests/proof_of_life.spec.ts
diff --git a/web/pwa/tests/proof_of_life.spec.ts b/web/pwa/tests/proof_of_life.spec.ts
index 643f92bb..8e49d5bf 100644
--- a/web/pwa/tests/proof_of_life.spec.ts
+++ b/web/pwa/tests/proof_of_life.spec.ts
@@ -43,7 +48,13 @@ test("proof of life: served / route renders against the test stack", async ({
   ).toContain(status);

   if (status === 200) {
-    await expect(page).toHaveTitle("Smackerel");
-    await expect(page.locator("h1")).toHaveText("Smackerel");
+    // Spec 057 (browser-login-redirect): an unauthenticated browser
+    // navigation to `/` is 303-redirected to `/login?next=/`, which
+    // Playwright follows, so the served 200 document is the login shell
+    // (internal/api/admin_ui_static/login.html) — NOT the PWA index. These
+    // are the ACTUAL served identities; the exact matches keep the check
+    // adversarial (a blank/error/other-page `/` fails), no silent-pass.
+    await expect(page).toHaveTitle("Sign in — Smackerel");
+    await expect(page.locator("h1")).toHaveText("Sign in");
   }
 });
```
<!-- bubbles:evidence-legitimacy-skip-end -->

Landed scoped local commit (`git show --stat`, scoped to the fixed file) — NOT pushed:

<!-- bubbles:evidence-legitimacy-skip-begin -->
```
commit 2b8e145a82d8cd1028e20c681572c57f0cccdd17
subject fix(e2e-ui): BUG-077-003 correct proof_of_life 200-branch to the real served login shell
 web/pwa/tests/proof_of_life.spec.ts | 27 +++++++++++++++++++--------
 1 file changed, 19 insertions(+), 8 deletions(-)
```
<!-- bubbles:evidence-legitimacy-skip-end -->

### Validation Evidence

Independent in-session re-verification: the live e2e-ui `proof_of_life` is GREEN (`1 passed`, exit 0) against the disposable `smackerel-test-e2e-ui` stack (all containers Healthy); the before-fix RED (exit 1, `Received "Sign in — Smackerel"`) proves the RED→GREEN transition; `regression-quality-guard` (plain + `--bugfix`) exits 0 with an adversarial signal; the full PWA suite discovers clean (52 tests, 28 files, exit 0); the real code delta is evidenced in `### Code Diff Evidence` at commit `2b8e145a`; `artifact-lint.sh` exits 0 and `state-transition-guard.sh` exits 0 at `done`. Change Boundary respected: one test file (+19/-8), zero app/runtime/source or other-spec-test edits.

### Audit Evidence

Final governance audit: `bash .github/bubbles/scripts/state-transition-guard.sh <bugdir>` exits 0 — the prior findings (E009 status mirror, Check-8A regression-E2E planning, G068 DoD-Gherkin fidelity, Check-13 artifact-lint Validation/Audit sections + terminal-signal blocks) are all cleared with the evidence above; `artifact-lint.sh <bugdir>` exits 0. Change Boundary respected: `git show 2b8e145a` / `git diff --stat` changed only `web/pwa/tests/proof_of_life.spec.ts` + this bug packet — zero excluded file families. No fabrication: every DoD `[x]` is backed by current-session raw evidence (RED/GREEN e2e-ui + guards + `--list`).

### build-quality

Build Quality Gate — all green this session:

- `regression-quality-guard` (plain + `--bugfix`) on `web/pwa/tests/proof_of_life.spec.ts`: 0 violations, 0 warnings, adversarial signal detected — exit 0 (report.md#phase-regression).
- `bash .github/bubbles/scripts/artifact-lint.sh <bugdir>`: `Artifact lint PASSED.`, exit 0.
- `bash .github/bubbles/scripts/state-transition-guard.sh <bugdir>`: all checks green at `done`, exit 0. The G088 post-certification ordering is satisfied by committing the planning truth first and setting `certifiedAt` after that commit; the fix + packet are scoped LOCAL commits and are NOT pushed.

## Completion Statement

The stale `proof_of_life.spec.ts` 200-branch was corrected to the ACTUAL live-observed served login-shell identity (`title "Sign in — Smackerel"`, `h1 "Sign in"`), the docstring was corrected to the spec-057 login-shell reality, and the `[200, 401]` tolerance + adversarial exact-match nature were preserved. Reproduced RED (exit 1) and confirmed GREEN (exit 0) live this session against the disposable `smackerel-test-e2e-ui` stack. `regression-quality-guard` (plain + `--bugfix`) exit 0. Change Boundary respected: one test file (+19/-8), zero app/runtime/source or other-spec-test edits. This is test staleness — the `/` → `/login?next=/` redirect is ratified spec-057 behavior and was left intact (no app change).

## Files Changed

- `web/pwa/tests/proof_of_life.spec.ts` — docstring corrected to the spec-057 login-shell reality; 200-branch assertions updated from `"Smackerel"`/`"Smackerel"` to the real served `title "Sign in — Smackerel"` + `h1 "Sign in"`.
- `specs/077-pwa-browser-test-harness/bugs/BUG-003-proof-of-life-stale-login-shell-assertion/**` — this bug packet.

## Certification (this session)

- **Before-fix RED:** `./smackerel.sh test e2e-ui proof_of_life.spec.ts` → `1 failed`, exit 1, `Received "Sign in — Smackerel"` (report.md#repro-before).
- **After-fix GREEN:** `./smackerel.sh test e2e-ui proof_of_life.spec.ts` → `1 passed`, exit 0 (report.md#repro-after).
- **Guards:** `regression-quality-guard` plain 0/0 + `--bugfix` adversarial-signal, exit 0 (report.md#phase-regression); `artifact-lint` + `state-transition-guard` exit 0 at `done` (report.md#build-quality).
- **Boundary:** `git diff --stat` = 1 file, +19/-8 (report.md#change-boundary + report.md#code-diff-evidence).
