# Report: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Summary

The consolidated release candidate's complete E2E run failed only in the root package. Isolation identified one stale Photos PWA source assertion: the test required `Authorization` while the served wizard correctly used the spec-100 HttpOnly same-origin cookie contract.

## Completion Statement

The one-file test repair is implemented and executable RED/GREEN evidence is captured. The packet remains `in_progress`; validate/audit certification is not claimed.

## Test Evidence

### Before Fix: Root Package RED

**Executed:** YES, current session
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e` followed by isolated root-package reproduction
**Exit Code:** 1
**Claim Source:** executed

```text
=== RUN   TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks
--- PASS: TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks (0.02s)
=== RUN   TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce
--- PASS: TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce (0.01s)
=== RUN   TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI
--- PASS: TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI (0.04s)
=== RUN   TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates
--- PASS: TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates (0.01s)
=== RUN   TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI
    photos_pwa_test.go:29: photo-library-add.js missing "Authorization"
--- FAIL: TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI (0.01s)
=== RUN   TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI
--- PASS: TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI (0.01s)
FAIL    github.com/smackerel/smackerel/tests/e2e        221.156s
FAIL: go-e2e (exit=1)
```

### After Fix: Focused Live GREEN

**Executed:** YES, current session
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$'` using the temporary closed root-package diagnostic selector
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying package selector: root
go-e2e: applying -run selector: ^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$
=== RUN   TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI
--- PASS: TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.119s
PASS: go-e2e
Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 to enable tests/e2e/agent/happy_path_test.go)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

### After Fix: Complete Root Package GREEN

**Executed:** YES, current session
**Command:** complete root Go E2E package through the repository CLI's temporary closed diagnostic selector
**Exit Code:** 0
**Claim Source:** executed

```text
=== RUN   TestRecommendationsWeb_RendersAPIBoundResultsAndProvenance
--- PASS: TestRecommendationsWeb_RendersAPIBoundResultsAndProvenance (0.05s)
=== RUN   TestWhyRegression_BS010_NoProviderCall
--- PASS: TestWhyRegression_BS010_NoProviderCall (0.08s)
=== RUN   TestHTTPAdapter_MissingRequiredKey_FailsLoud
--- PASS: TestHTTPAdapter_MissingRequiredKey_FailsLoud (2.52s)
=== RUN   TestSurfacingMetricsExposedOnLiveStack
--- PASS: TestSurfacingMetricsExposedOnLiveStack (0.01s)
=== RUN   TestWeatherAlerts_E2E_FullStack
--- PASS: TestWeatherAlerts_E2E_FullStack (0.01s)
=== RUN   TestWeatherEnrich_E2E_LiveStackRoundTrip
--- PASS: TestWeatherEnrich_E2E_LiveStackRoundTrip (0.69s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        221.468s
PASS: go-e2e
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

## Change Boundary

Production source remains unchanged. The temporary root-package selector used to recover truncated test output was removed before commit. The permanent implementation delta is `tests/e2e/photos_pwa_test.go` plus this bug packet.

## Invocation Audit

No validate or audit specialist result is claimed. Certification remains open.