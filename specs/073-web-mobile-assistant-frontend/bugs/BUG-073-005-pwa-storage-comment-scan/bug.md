# BUG-073-005 - Served PWA storage scan flags policy comments as executable access

**Status:** Confirmed - reproduction and fix in progress
**Severity:** Medium - false-positive live E2E blocks the assistant package
**Spec:** 073-web-mobile-assistant-frontend
**Discovered:** 2026-07-19 during serialized broad `./smackerel.sh test e2e`

## Summary

`TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09`
searches the entire served `assistant.js` text with `strings.Contains`. The file's
header comments explicitly say `localStorage`, `sessionStorage`, and other auth
storage APIs are forbidden, so the live test reports a violation even though no
executable access exists. The dedicated unit storage guard already strips line
comments before evaluating the same policy.

## Reproduction

```bash
SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run 'TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09' --verbose
```

## Expected Behavior

- Policy comments may name forbidden browser APIs without failing the served
  route E2E.
- Executable references to forbidden browser storage APIs still fail both the
  dedicated guard and the served-source E2E.
- Both tests use one reusable, comment-aware JavaScript source inspection
  helper rather than divergent ad hoc string replacement.
- The live test continues to fetch the real served asset from the disposable
  stack and validates the production route/wiring.

## Actual Behavior

The live test scans raw source, so the documentation comment itself trips the
forbidden-token check. This is a stale test implementation, not evidence of a
production privacy breach.

## Impact

Broad assistant E2E is red and a real future storage access could be harder to
reason about because two policy checks currently interpret source differently.

## Security Classification

Current inspection finds a false positive, not real credential persistence.
The fix must retain adversarial coverage proving executable storage access is
detected.
