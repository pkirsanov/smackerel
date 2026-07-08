# BUG-083-001 — SCN-083-J08 e2e-ui `starred` checkbox 30s timeout on macOS headless

- **Spec / scenario:** 083 Scope 10 — `SCN-083-J08` (manage category names, equivalents, and starred)
- **Severity:** Low (out-of-MVP-dimension; test-harness robustness, not a product defect)
- **Status:** Fixed
- **Surfaced by:** MVP-readiness journey pass, finding **J3**
- **Test:** [web/pwa/tests/cardrewards_categories.spec.ts](../../../../web/pwa/tests/cardrewards_categories.spec.ts)

## Symptom (as reported — finding J3)

The `e2e-ui` lane run was 42 passed / **1 failed** / 9 skipped. The single
failure was `cardrewards_categories.spec.ts` `SCN-083-J08`, which 30s-timed-out
on `page.check('input[name="starred"]')`: the checkbox resolves in the DOM but
`check()` never became click-stable. Browser log showed macOS headless-Chromium
`CVDisplayLinkCreateWithCGDisplay failed` + EGL warnings. Every spec-100 saga
passed; the failure was isolated to this one card-rewards (spec 083) checkbox.

## Root cause — (a) macOS-headless actionability flake (NOT a product bug)

The product side is correct on every axis; the failure is Playwright's
actionability wait on a tiny native checkbox under a broken macOS headless
compositor:

1. **Markup is correct + interactable.** The control is a plain, label-associated
   native checkbox with no `disabled`, no `pointer-events:none`, no overlay, and
   no `appearance:none`-hidden zero-size input:
   `internal/web/cardrewards_templates.go` →
   `<label><input type="checkbox" name="starred"> Starred</label>`.
   The only CSS touching it is the global `*{margin:0;padding:0}` reset and
   `input{color:…}` — nothing that would break hit-testing.
2. **The handler reads + persists `starred`.** `internal/web/cardrewards.go`
   `CategoryUpsert` → `Starred: r.FormValue("starred") == "on"`, persisted via
   `CreateCategoryAlias`. Starring genuinely round-trips.
3. **Same page, same test: `fill()` and `click()` succeed; only `check()`
   fails.** `fill()`/`click()` don't hit-test a ~13px target; `check()` waits for
   stability + a hit-test at the checkbox center, which the broken CVDisplayLink
   compositor never satisfies within the 30s test budget. If the page/markup were
   broken, the fills and the button click would fail too — they don't.
4. **Isolated flake, not deterministic breakage.** 42/1/9 with the one failure on
   the smallest native-checkbox interaction is the classic macOS
   headless-Chromium signature (`CVDisplayLinkCreateWithCGDisplay failed`).

## Fix — minimal test-hardening (correct side: the test), assertion preserved

Replaced each bare `page.check('input[name="starred"]')` with a gated helper
`starCategory(page)` that keeps the interaction a **real** assertion:

```ts
const starred = page.locator('input[name="starred"]');
await expect(starred).toBeVisible();   // real UI presence
await expect(starred).toBeEnabled();   // real interactability
await starred.scrollIntoViewIfNeeded();
await starred.check({ force: true });  // bypass only the flaky stability/hit-test wait
await expect(starred).toBeChecked();   // real: the toggle actually registered
```

Why this is the correct side and not a tautological weakening:

- The product is correct (markup + handler + persistence proven above), so the
  fix belongs in the harness, not the UI.
- `force: true` is **gated** behind `toBeVisible()` + `toBeEnabled()` and
  **followed** by `toBeChecked()`, so a missing / hidden / disabled / non-toggling
  checkbox would still fail. It is not a blind force.
- The definitive `SCN-083-J08` intent — **starring persists** — is untouched: the
  test still submits the form and, after a full page reload, asserts
  `data-starred="true"` on the server-re-rendered row (twice). Remove the star and
  those assertions fail.
- On Linux / evo-x2 the checkbox is click-stable, so `check({ force: true })`
  behaves as a plain `check()` there.

## Evidence

**Pre-fix (reporter, finding J3):** `SCN-083-J08` FAILED — 30s timeout on
`page.check('input[name="starred"]')`; `CVDisplayLinkCreateWithCGDisplay failed`
- EGL warnings; lane 42 passed / 1 failed / 9 skipped.

**Post-fix (executed — `./smackerel.sh test e2e-ui tests/cardrewards_categories.spec.ts`, macOS headless):**

```
Running 1 test using 1 worker
  ✓  1 …s › SCN-083-J08 — manage category names, equivalents, and starred (2.1s)

  1 passed (3.0s)
```

The disposable `smackerel-test-e2e-ui` stack was brought up and fully torn down
(all containers removed, volumes + network removed) by the lane trap.

## Files changed

- [web/pwa/tests/cardrewards_categories.spec.ts](../../../../web/pwa/tests/cardrewards_categories.spec.ts)
  — add `starCategory()` gated-force helper; both `page.check` call sites now use it.
