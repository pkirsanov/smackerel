# Bug: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Summary

`TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` requires the served photo-library wizard JavaScript to contain the literal `Authorization`, although spec 100 intentionally migrated the PWA to an HttpOnly same-origin session cookie. The production script correctly uses `credentials: "same-origin"`, so the stale test is the sole failing test in the otherwise-green root E2E package.

## Severity

- [ ] Critical
- [ ] High
- [x] Medium - blocks release validation while production behavior remains correct
- [ ] Low

## Status

- [x] Confirmed
- [x] Fixed
- [x] Focused and broader regression verified
- [ ] Validate/audit certified
- [ ] Closed

## Reproduction

1. Run `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e` on the consolidated release candidate.
2. Observe every named E2E subpackage pass while the root package exits nonzero.
3. Run the root package in isolation and observe `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` fail because `photo-library-add.js` lacks the stale `Authorization` literal.
4. Inspect the served script and observe its intentional HttpOnly-cookie contract: `credentials: "same-origin"`.

## Expected Behavior

The E2E test verifies the PWA sends the HttpOnly session cookie using same-origin credentials and rejects an omitted-cookie request mode. It must not require JavaScript-visible bearer credentials.

## Actual Behavior

The test checks for `Authorization`, contradicting the implemented and documented cookie-auth contract.

## Root Cause

The Photos PWA E2E assertion predates spec 100 commit `666073f1`, which moved the PWA to one HttpOnly cookie. The production wizard was updated, but the older static source assertion was not.

## Fix

Replace the stale bearer-header assertion with `credentials: "same-origin"` and add an adversarial assertion that fails if `credentials: "omit"` appears.

## Related

- Parent: `specs/077-pwa-browser-test-harness/`
- Surfaced by: `specs/100-unified-journey-ui-transformation/`
- Test: `tests/e2e/photos_pwa_test.go`
- Runtime source of truth: `web/pwa/photo-library-add.js`