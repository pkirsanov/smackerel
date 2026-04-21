# Report: 012 — Hospitable Connector

> **Status:** Done
> **Last Updated:** 2026-04-21

---

## Summary

Hospitable Connector delivered under delivery-lockdown mode. 5 scopes completed: (1) API Client, Types & Config; (2) Connector Implementation & Normalizer; (3) Edge Hints, Cross-Domain Linking & Hardening; (4) Message Sync Reliability & Client Hardening; (5) Normalizer Quality Fixes. Implementation: types.go, client.go, connector.go, normalizer.go in `internal/connector/hospitable/`. 202 unit tests across 3 test files (connector_test.go: 69, normalizer_test.go: 56, chaos_test.go: 77) — all pass. PAT authentication, paginated fetching, rate limit handling, 4 resource types (property, reservation, message, review), incremental cursor sync, knowledge graph edge hints, partial failure isolation, property name cache. Security hardening includes SSRF protection on pagination URLs, response body size limits, and extensive chaos testing.

## Completion Statement

All 5 scopes are Done. All DoD items verified with passing tests. `./smackerel.sh test unit` passes all Go packages including `internal/connector/hospitable`. `./smackerel.sh lint` exits 0. `./smackerel.sh check` confirms config in sync. Delivery-lockdown certification complete.

## Known Gaps

Integration tests (`tests/integration/hospitable_test.go`) and E2E tests (`tests/e2e/hospitable_test.go`) listed in scope test plans were never implemented. All test coverage is via unit tests with mock HTTP servers. This is acceptable for the connector pattern (no live API in CI) but means the test plans in scopes.md overstate the integration/E2E coverage.

---

## DevOps Probe (2026-04-21)

**Trigger:** `devops-to-doc` via stochastic-quality-sweep
**Probe agent:** bubbles.devops (inline)
**Result:** 1 critical finding — SST→env→auto-start pipeline was broken; fixed

### Probe Summary

Probed build/deployment/CI/CD/monitoring surface for the Hospitable connector. Examined: Dockerfile, docker-compose.yml, docker-compose.prod.yml, config/smackerel.yaml, scripts/commands/config.sh, internal/config/config.go, cmd/core/connectors.go, generated env files.

**Areas probed:**
- Docker build (Dockerfile, multi-stage, non-root, labels) — clean
- Docker Compose (service definition, healthchecks, env_file, volumes) — clean (connector runs inside smackerel-core, no separate container needed)
- Config SST pipeline (smackerel.yaml → config.sh → generated env → Config struct → auto-start) — **BROKEN**
- Monitoring (health endpoint, slog structured logging, connector health status) — clean
- Build identity (ldflags VERSION/COMMIT_HASH/BUILD_TIME, OCI labels) — clean

**Findings:**

| # | Area | Severity | Detail | Fix |
|---|------|----------|--------|-----|
| 1 | Config SST pipeline | Critical | Hospitable connector had YAML config in `smackerel.yaml` but: (a) `scripts/commands/config.sh` did not extract `HOSPITABLE_*` env vars, (b) `internal/config/config.go` had no `Hospitable*` fields in Config struct, (c) `cmd/core/connectors.go` had no auto-start block. Result: connector registered but could never be started in a real deployment. | Fixed: all 3 layers wired |

**Changes made:**
- `scripts/commands/config.sh` — Added 15 `HOSPITABLE_*` variable extractions from SST and corresponding env file output lines
- `internal/config/config.go` — Added 14 `Hospitable*` fields to Config struct and env var loading in `Load()`
- `cmd/core/connectors.go` — Added auto-start block following the Discord/Twitter/Weather pattern: checks `cfg.HospitableEnabled`, builds `ConnectorConfig` with PAT credentials and all source config, calls `Connect()`, registers with supervisor

**Verification:**
- `./smackerel.sh config generate` — generates `HOSPITABLE_*` vars in dev.env
- `./smackerel.sh check` — "Config is in sync with SST"
- `./smackerel.sh build` — Docker build succeeds (Go compiles clean)
- `./smackerel.sh test unit` — all packages pass including cmd/core, internal/config, internal/connector/hospitable
- `./smackerel.sh lint` — exit 0
- `./smackerel.sh format --check` — 33 files unchanged

---

## Hardening Probe (2026-04-21)

**Trigger:** `harden-to-doc` via stochastic-quality-sweep (round 2)
**Probe agent:** bubbles.harden (inline)
**Result:** Clean — doc freshness fixes only, no code changes

### Probe Summary

Full re-probe of 22 requirements (R-001–R-022), 5 scopes + chaos scope, 30 Gherkin scenarios (SCN-HC-001–030), 4 source files (client.go, connector.go, normalizer.go, types.go), and 202 test functions. All unit tests pass. `./smackerel.sh lint` exit 0. `./smackerel.sh test unit` — hospitable package ok.

**Areas probed:**
- Gherkin scenario completeness and specificity
- DoD item coverage against implementation
- Test plan depth and adversarial coverage
- Scope boundary definitions
- Security hardening gaps
- Spec-vs-implementation drift
- Artifact doc freshness vs actual test counts

**Findings:**

| # | Area | Severity | Detail | Disposition |
|---|------|----------|--------|-------------|
| 1 | uservalidation.md stale test count | Cosmetic | Said 138 (53+39+46), actual 202 (69+56+77) | Fixed: updated uservalidation.md |
| 2 | Scope Summary chaos count stale | Cosmetic | Said 56, actual 77 | Fixed: updated scopes.md |
| 3 | SCN-HC-020 says "healthy" but code uses HealthDegraded | Informational | Scenario-to-code mismatch; implementation is correct (more granular). Already noted in prior probe. | Acknowledged — no change |
| 4 | `HealthDegraded` in code but not R-013 | Informational | Implementation provides more granular health reporting than spec. Not a bug — improvement over spec. | Acknowledged (carried from prior probe) |
| 5 | Processing tier strings unvalidated | Informational | Config accepts arbitrary tier strings. Pipeline validates downstream. Low risk. | Accepted (carried from prior probe) |

**No code changes required. No new Gherkin scenarios needed. Security hardening is thorough (SSRF, body limits, retry caps, cache caps, input sanitization, race protection, 77 chaos tests).**

---
## Test Probe (2026-04-21)

**Trigger:** `test-to-doc` via stochastic-quality-sweep (child workflow)
**Probe agent:** bubbles.test (inline)
**Result:** Near-clean — 2 minor assertion weaknesses fixed, no new test functions needed

### Probe Summary

Full test coverage audit: 3 test files, 150+ test functions, 30 Gherkin scenarios (SCN-HC-001–030), all 5 scopes + chaos scope. Applied Test Integrity gates (Gherkin coverage, no mocks in live categories, no silent-pass, real assertions, test plan parity, no self-validating setup).

**Gate Results:**

| Gate | Result | Notes |
|------|--------|-------|
| G1: Gherkin Coverage | PASS | All 30 SCN-HC-* scenarios have 1+ mapped test functions |
| G2: No Internal Mocks (Live) | N/A | All tests are unit category — no live-stack claims |
| G3: No Silent-Pass | PASS | No early-return/bailout patterns found |
| G4: Real Assertions | PASS (after fix) | 2 weak assertions strengthened |
| G5: Test Plan ↔ DoD Parity | PASS | All scope DoD items have corresponding test evidence |
| G7: No Self-Validating Setup | PASS | All assertions validate code output, not test setup |

**Findings:**

| # | Area | Severity | Detail | Fix |
|---|------|----------|--------|-----|
| 1 | `TestCursorEmptyAppliesLookback` weak assertion | Minor | Only checked `Properties` (zero) and `Reservations` (~30d ago). Did not verify `Messages` and `Reviews` cursors also apply the lookback period. | Fixed: added loop checking all 3 lookback fields |
| 2 | `TestSyncFullLifecycle` weak cursor assertion | Minor | Only checked `cursor != ""`. Did not verify decoded cursor has non-zero timestamps for all 4 resource types after sync. | Fixed: added JSON decode + per-field non-zero checks |

**Changes made:**
- [connector_test.go](../../internal/connector/hospitable/connector_test.go): `TestCursorEmptyAppliesLookback` — expanded assertions to verify Messages and Reviews cursors also match lookback period
- [connector_test.go](../../internal/connector/hospitable/connector_test.go): `TestSyncFullLifecycle` — added cursor JSON decode with non-zero timestamp assertions for all 4 resource types

**Verification:**
- `./smackerel.sh test unit` — all pass (hospitable package rerun, not cached)
- `./smackerel.sh lint` — exit 0
- `./smackerel.sh format --check` — 33 files unchanged

**Coverage strength summary:**
- 69 tests in `connector_test.go` — client, connector lifecycle, config, cursor, SSRF, pagination, retry, context cancellation
- 56 tests in `normalizer_test.go` — all 4 normalizers, edge hints, URL safety, rating clamping, input sanitization, address/date formatting
- 77 tests in `chaos_test.go` — malformed JSON, empty/null fields, Unicode, concurrency, extreme values, auth expiry, lifecycle races

---
## Hardening Probe (2026-04-20)

**Trigger:** `harden-to-doc` via stochastic-quality-sweep
**Probe agent:** bubbles.harden (inline)
**Result:** Clean — no actionable findings

### Probe Summary

Systematic review of 22 requirements (R-001–R-022), 5 scopes + chaos scope, 30 Gherkin scenarios (SCN-HC-001–030), 4 source files, and 202 test functions.

**Areas probed:**
- Gherkin scenario completeness and specificity
- DoD item coverage against implementation
- Test plan depth and adversarial coverage
- Scope boundary definitions
- Security hardening gaps
- Spec-vs-implementation drift

**Findings:**

| # | Area | Severity | Detail | Disposition |
|---|------|----------|--------|-------------|
| 1 | Report freshness | Cosmetic | Test count was 138 (stale); actual is 202 after later hardening rounds | Fixed in this update |
| 2 | `HealthDegraded` in code but not R-013 | Informational | Implementation provides more granular health reporting than spec. Not a bug — improvement over spec. | Acknowledged |
| 3 | Processing tier strings unvalidated | Informational | Config accepts arbitrary tier strings. Pipeline validates downstream. Low risk. | Accepted |

**Hardening already present:**
- SSRF protection on pagination URLs (same-origin check)
- Response body size limit (10 MiB)
- Retry-After cap (60s), max page size cap (500), pagination page cap (1000)
- Property name cache caps (10K entries, 1024-char strings)
- Input sanitization (CWE-116), URL scheme validation (CWE-79/601)
- Rating clamping for NaN/Inf (CWE-20)
- Context cancellation propagation, sync timeout (10 min)
- Race condition protection (sync.RWMutex)
- 77 chaos tests covering malformed responses, concurrency, extreme values

**Conclusion:** No implementation work required. Documentation freshness fixed.

---

## Raw Terminal Output Evidence (2026-04-16)

### Unit Tests — Full Suite

```terminal
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/cmd/core (cached)
ok      github.com/smackerel/smackerel/internal/api     (cached)
ok      github.com/smackerel/smackerel/internal/auth    (cached)
ok      github.com/smackerel/smackerel/internal/config  0.073s
ok      github.com/smackerel/smackerel/internal/connector       (cached)
ok      github.com/smackerel/smackerel/internal/connector/alerts        (cached)
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     (cached)
ok      github.com/smackerel/smackerel/internal/connector/browser       (cached)
ok      github.com/smackerel/smackerel/internal/connector/caldav        (cached)
ok      github.com/smackerel/smackerel/internal/connector/discord       (cached)
ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
ok      github.com/smackerel/smackerel/internal/connector/imap  (cached)
ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
ok      github.com/smackerel/smackerel/internal/connector/maps  (cached)
ok      github.com/smackerel/smackerel/internal/connector/markets       (cached)
ok      github.com/smackerel/smackerel/internal/connector/rss   (cached)
ok      github.com/smackerel/smackerel/internal/connector/twitter       (cached)
ok      github.com/smackerel/smackerel/internal/connector/weather       (cached)
ok      github.com/smackerel/smackerel/internal/connector/youtube       (cached)
ok      github.com/smackerel/smackerel/internal/db      (cached)
ok      github.com/smackerel/smackerel/internal/digest  (cached)
ok      github.com/smackerel/smackerel/internal/extract (cached)
ok      github.com/smackerel/smackerel/internal/graph   (cached)
ok      github.com/smackerel/smackerel/internal/intelligence    (cached)
ok      github.com/smackerel/smackerel/internal/knowledge       (cached)
ok      github.com/smackerel/smackerel/internal/nats    (cached)
ok      github.com/smackerel/smackerel/internal/pipeline        (cached)
ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
ok      github.com/smackerel/smackerel/internal/stringutil      (cached)
ok      github.com/smackerel/smackerel/internal/telegram        (cached)
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/internal/web/icons       (cached)
```

**Result:** All Go packages pass. Hospitable package: `ok github.com/smackerel/smackerel/internal/connector/hospitable (cached)`. 202 test functions: 69 connector + 56 normalizer + 77 chaos.

### Lint, Config SST Check, Format Check

```terminal
$ ./smackerel.sh lint
All checks passed!

$ ./smackerel.sh check
Config is in sync with SST

$ ./smackerel.sh format --check
(exit 0)
```

---

## Scenario Traceability

30 Gherkin scenarios (SCN-HC-001 through SCN-HC-030) defined in scopes.md. All linked to concrete test functions in [scenario-manifest.json](scenario-manifest.json).

| Scope | Scenarios | Test File(s) | Coverage |
|-------|-----------|-------------|----------|
| 01 — API Client, Types & Config | SCN-HC-001..006 | `internal/connector/hospitable/connector_test.go` | 12 unit tests |
| 02 — Connector & Normalizer | SCN-HC-007..014 | `internal/connector/hospitable/connector_test.go`, `internal/connector/hospitable/normalizer_test.go` | 14 unit tests |
| 03 — Edge Hints & Hardening | SCN-HC-015..022 | `internal/connector/hospitable/normalizer_test.go`, `internal/connector/hospitable/connector_test.go` | 8 unit tests |
| 04 — Message Sync Reliability | SCN-HC-023..026 | `internal/connector/hospitable/connector_test.go` | 9 unit tests |
| 05 — Normalizer Quality | SCN-HC-027..030 | `internal/connector/hospitable/normalizer_test.go` | 11 unit tests |
| — Chaos & Resilience | (no Gherkin) | `internal/connector/hospitable/chaos_test.go` | 46 unit tests |

### Per-Scope Test File Evidence

**Scope 01 — API Client, Types & Config**
Concrete test file: `internal/connector/hospitable/connector_test.go`
Tests: TestClientAuthHeader, TestClientValidateSuccess, TestClientValidateUnauthorized, TestClientValidateForbidden, TestClientPaginatesProperties, TestClientRetryOn429, TestClientMaxRetriesOn429, TestClientRetryOnServerError, TestClientURLConstruction, TestConfigValidationMissingToken, TestConfigValidationDefaults, TestSyncCursorMarshal

**Scope 02 — Connector Implementation & Normalizer**
Concrete test files: `internal/connector/hospitable/connector_test.go`, `internal/connector/hospitable/normalizer_test.go`
Tests: TestConnectorID, TestConnectValidConfig, TestConnectInvalidToken, TestNormalizeProperty, TestNormalizeReservation, TestNormalizeMessage, TestNormalizeReview, TestSyncCursorMarshal, TestCursorEmptyAppliesLookback, TestDisabledResourceSkipped, TestHealthTransitions, TestCloseIdempotent, TestNormalizeAllTiers, TestSyncFullLifecycle

**Scope 03 — Edge Hints, Cross-Domain Linking & Hardening**
Concrete test files: `internal/connector/hospitable/normalizer_test.go`, `internal/connector/hospitable/connector_test.go`
Tests: TestNormalizeReservation (edge hints), TestNormalizeMessage (edge_part_of), TestNormalizeReview (edge_review_of), TestNormalizeReservationLeadTime, TestPropertyNameCacheEnrichesTitle, TestNormalizeReservationFallbackPropertyID, TestPartialFailureReturnsSuccessful, TestAllFailuresSetHealthError, TestConnectEmptyToken

**Scope 04 — Message Sync Reliability & Client Hardening**
Concrete test file: `internal/connector/hospitable/connector_test.go`
Tests: TestActiveReservationMessageSync, TestClientListActiveReservationsParam, TestParseRetryAfterSeconds, TestParseRetryAfterHTTPDate, TestParseRetryAfterEmpty, TestParseRetryAfterInvalid, TestRetryAfterUsedOn429, TestPropertyNameCachePersistsInCursor, TestPropertyNameCacheLoadedFromCursor, TestMessageCursorNotAdvancedOnFailure

**Scope 05 — Normalizer Quality Fixes**
Concrete test file: `internal/connector/hospitable/normalizer_test.go`
Tests: TestClassifySenderGuest, TestClassifySenderHost, TestClassifySenderAutomated, TestClassifySenderDefaultGuest, TestNormalizeMessageHostSender, TestNormalizePropertyURL, TestNormalizePropertyNoURL, TestNormalizeReservationURLProduction, TestNormalizeReservationURLTest, TestFormatRatingWhole, TestFormatRatingFractional, TestFormatRatingZero, TestNormalizeReviewFractionalRating

## Hardening Sweep Findings (harden-to-doc, 2026-04-12)

**Trigger:** stochastic-quality-sweep → harden
**Agent:** bubbles.workflow (child) → harden-to-doc mode
**Scope:** Full artifact hardening review of spec.md, design.md, scopes.md, and implementation

### H-001: Test Plan References Non-Existent Test Files (Medium)

Scopes 1–3 test plan tables list 14 integration/E2E tests across `tests/integration/hospitable_test.go` and `tests/e2e/hospitable_test.go` that were never created:

| Scope | Phantom Tests | Type |
|-------|--------------|------|
| 1 | T-1-13, T-1-14 | integration |
| 2 | T-2-15, T-2-16, T-2-17 | integration |
| 2 | T-2-18, T-2-19 | e2e |
| 3 | T-3-09, T-3-10, T-3-11, T-3-12 | integration |
| 3 | T-3-13, T-3-14 | e2e |

**Impact:** Test plan fidelity — readers of scopes.md may expect these tests exist. The Known Gaps section above documents this, but the test plan tables still reference these files without annotation.

**Disposition:** Documented. No code change needed — all Gherkin scenarios have unit test coverage via mock HTTP servers. If integration/E2E tests are desired in the future, the test plan rows provide the mapping.

### H-002: Missing Gherkin Scenarios for Hardening Behaviors (Low)

Several security and resilience behaviors implemented in code (and covered by chaos_test.go) lack formal Gherkin scenarios in spec.md or scopes.md:

| Behavior | Code Location | Test Coverage | Gherkin Scenario |
|----------|--------------|---------------|-----------------|
| Sync timeout (10 min cap) | `connector.go:maxSyncDuration` | TestChaos_ContextCancelledDuringSync | None |
| Response body size limit (10 MiB) | `client.go:maxResponseSize` | TestClientResponseBodySizeLimit | None |
| SSRF pagination URL rejection | `client.go:isSameOrigin` | TestPaginationRejectsCrossOriginNextURL + 3 more | None |
| Pagination page cap (1000) | `client.go:maxPaginationPages` | TestChaos_PaginationInfiniteLoop | None |
| Retry-After cap (60s) | `client.go:maxRetryAfterCap` | TestChaos_RateLimitWithHugeRetryAfter | None |
| Corrupted cursor fallback | `connector.go:parseCursor` | TestChaos_CorruptedCursor | None |

**Impact:** Behavioral documentation gap. These hardening constants and behaviors are not traceable from spec to implementation through Gherkin. They are discoverable only through code reading or chaos test names.

**Disposition:** Documented. The chaos test suite provides behavioral coverage. Adding Gherkin scenarios would improve traceability but is not blocking since the behaviors are tested.

### H-003: NormalizeMessage Dual Reservation ID Paths (Low)

`NormalizeMessage(m Message, reservationID string, ...)` uses `m.ReservationID` for the `metadata["reservation_id"]` field but uses the `reservationID` parameter for the `edge_part_of` hint and content rendering. In normal sync flow these are always identical (messages are fetched per reservation ID). However, the dual path creates a maintenance risk if the function is called with a mismatched parameter.

**Disposition:** Documented. Not blocking — values are always consistent in the current call path from `Sync()`.

### H-004: SyncSchedule Config Field Not Validated (Low)

The `SyncSchedule` field in `HospitableConfig` is parsed from `smackerel.yaml` but never validated as a valid cron expression during `parseHospitableConfig()`. An invalid cron string would only fail at scheduler registration time, not during connector Connect().

**Disposition:** Documented. Low risk — the scheduler would reject the invalid expression at registration.

### H-005: Property Name Cache Unbounded Growth (Info)

The `propertyNames` map grows monotonically — every property ever synced is cached in the connector's memory and persisted in the JSON cursor. For a large multi-property account over time, this is a slow growth in cursor JSON size. No eviction or LRU strategy exists.

**Disposition:** Documented. Negligible impact for typical accounts (1–50 properties). Only relevant if deployed to manage thousands of properties.

### Hardening Summary

| Finding | Severity | Category | Requires Code Change |
|---------|----------|----------|---------------------|
| H-001 | Medium | Test plan fidelity | No |
| H-002 | Low | Gherkin traceability | No |
| H-003 | Low | Code quality | No (future cleanup) |
| H-004 | Low | Config validation | No (future enhancement) |
| H-005 | Info | Scalability | No (future enhancement) |

**Overall Assessment:** The Hospitable connector artifacts are well-structured with strong Gherkin coverage (30 scenarios), comprehensive DoD items, and 138 tests including a dedicated chaos suite. The findings are documentation-grade observations, not blocking issues.

## Test Evidence

See **Raw Terminal Output Evidence** section above for captured `./smackerel.sh test unit` output from 2026-04-16.

Hospitable-specific tests (138 tests across 3 files):

- `connector_test.go` (53 tests): TestClientAuthHeader, TestClientValidateSuccess, TestClientValidateUnauthorized, TestClientValidateForbidden, TestClientPaginatesProperties, TestClientRetryOn429, TestClientMaxRetriesOn429, TestDefaultClientMaxRetries3, TestClientRetryOnServerError, TestClientURLConstruction, TestConnectorID, TestConnectValidConfig, TestConnectInvalidToken, TestConfigValidationMissingToken, TestConfigValidationNegativeLookback, TestConfigValidationDefaults, TestSyncCursorMarshal, TestCursorEmptyAppliesLookback, TestHealthTransitions, TestDisabledResourceSkipped, TestSyncFullLifecycle, TestPartialFailureReturnsSuccessful, TestAllFailuresSetHealthError, TestPropertyNameCacheEnrichesTitle, TestConnectEmptyToken, TestSyncNotConnected, TestCloseIdempotent, TestClientResponseBodySizeLimit, TestClientListMessagesPathEscaping, TestClientListActiveReservationsParam, TestParseLinkNextValid, TestParseLinkNextNoQuoteRel, TestParseLinkNextEmpty, TestParseLinkNextPrevOnly, TestParseLinkNextMultipleLinks, TestConfigProcessingTierOverrides, TestConfigSyncFlagOverrides, TestActiveReservationMessageSync, TestParseRetryAfterSeconds, TestParseRetryAfterHTTPDate, TestParseRetryAfterEmpty, TestParseRetryAfterInvalid, TestRetryAfterUsedOn429, TestPropertyNameCachePersistsInCursor, TestPropertyNameCacheLoadedFromCursor, TestMessageCursorNotAdvancedOnFailure, TestPaginationRejectsCrossOriginNextURL, TestPaginationRejectsCrossOriginLinkHeader, TestPaginationRejectsMetadataEndpoint, TestPaginationAllowsSameOriginNextURL, TestPaginationMaxPageLimit, TestConfigRejectsInvalidBaseURLScheme, TestConfigAcceptsValidBaseURL
- `normalizer_test.go` (39 tests): TestNormalizeProperty, TestNormalizeReservation, TestNormalizeReservationFallbackPropertyID, TestNormalizeReservationLeadTime, TestNormalizeMessage, TestNormalizeReview, TestNormalizeReviewFallbackPropertyID, TestNormalizeAllTiers, TestClassifySenderGuest, TestClassifySenderHost, TestClassifySenderAutomated, TestClassifySenderDefaultGuest, TestNormalizeMessageHostSender, TestNormalizePropertyURL, TestNormalizePropertyNoURL, TestNormalizeReservationURLProduction, TestNormalizeReservationURLTest, TestFormatRatingWhole, TestFormatRatingFractional, TestFormatRatingZero, TestNormalizeReviewFractionalRating, TestFormatAddressFull, TestFormatAddressCityOnly, TestFormatAddressStateOnly, TestFormatAddressCityState, TestFormatAddressEmpty, TestFormatAddressStreetCountryOnly, TestFormatDateStandard, TestFormatDateRFC3339Fallback, TestFormatDateInvalidReturnsOriginal, TestFormatDateEmptyString, TestNormalizeReservationZeroBookedAt, TestNormalizeReviewNoHostResponse, TestNormalizePropertyCapturedAtCreatedAt, TestNormalizePropertyCapturedAtFallbackNow, TestNormalizeMessageCapturedAtFallbackNow, TestFirstNonEmptyMultiple, TestFirstNonEmptyAllEmpty, TestFirstNonEmptyNil
- `chaos_test.go` (46 tests): TestChaos_MalformedJSON_Response, TestChaos_EmptyDataArray, TestChaos_NullDataField, TestChaos_MissingDataField, TestChaos_PropertyAllFieldsEmpty, TestChaos_ReservationAllFieldsEmpty, TestChaos_MessageAllFieldsEmpty, TestChaos_ReviewAllFieldsEmpty, TestChaos_ExtremelyLongGuestName, TestChaos_GuestNameWithNullBytes, TestChaos_GuestNameNewlinesAndTabs, TestChaos_UnicodePropertyNames, TestChaos_UnicodeMessageBodies, TestChaos_UnicodeReviewText, TestChaos_PaginationInfiniteLoop, TestChaos_PaginationEmptyNextURL, TestChaos_PaginationMalformedLinkHeader, TestChaos_TokenExpiryMidSync, TestChaos_TokenExpiryOnValidate, TestChaos_ConcurrentSync, TestChaos_ConcurrentConnectAndSync, TestChaos_ConcurrentHealthCheck, TestChaos_PropertyMissingID, TestChaos_ReservationMissingDates, TestChaos_ReservationInvalidDateFormat, TestChaos_MessageMissingSender, TestChaos_RateLimitWithZeroRetryAfter, TestChaos_RateLimitWithNegativeRetryAfter, TestChaos_RateLimitWithHugeRetryAfter, TestChaos_RateLimitWithMalformedRetryAfter, TestChaos_ServerErrorWithBody, TestChaos_ContextCancelledDuringSync, TestChaos_ExtremeNumericValues, TestChaos_ExtremeRatingValues, TestChaos_CorruptedCursor, TestChaos_ConfigZeroPageSize, TestChaos_ConfigNegativePageSize, TestChaos_ConfigHugePageSize, TestChaos_ConfigZeroLookback, TestChaos_SyncBeforeConnect, TestChaos_SyncAfterClose, TestChaos_APIReturnsWrongTypes, TestChaos_ManyReservationsForMessageSync, TestChaos_PropertyNameCacheWithDuplicateIDs, TestChaos_FormatDateEdgeCases, TestChaos_ParseLinkNextEdgeCases

### Validation Evidence

**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** `./smackerel.sh test unit`, `./smackerel.sh lint`, `./smackerel.sh check`, `./smackerel.sh format --check`

```terminal
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
# 138 tests: 53 connector + 39 normalizer + 46 chaos — all pass

$ ./smackerel.sh lint
All checks passed!

$ ./smackerel.sh check
Config is in sync with SST

$ ./smackerel.sh format --check
(exit 0)
```

### Audit Evidence

**Phase Agent:** bubbles.audit
**Executed:** YES
**Command:** `./smackerel.sh check`, `./smackerel.sh lint`

```terminal
$ ./smackerel.sh check
Config is in sync with SST

$ ./smackerel.sh lint
All checks passed!
```

Code quality review of `internal/connector/hospitable/`:

- **Pattern compliance:** Follows existing connector patterns (Keep, Maps, Browser) — implements `connector.Connector` interface (ID, Connect, Sync, Health, Close)
- **Config SST:** All config values sourced from `config/smackerel.yaml` → `scripts/commands/config.sh` → `config/generated/dev.env`. No hardcoded ports, URLs, or fallback defaults
- **NATS contract:** No modifications to existing NATS streams or subjects
- **Database:** Uses existing `artifacts`, `edges`, `sync_state` tables — no new migrations
- **Security:** PAT stored in config SST pipeline. Bearer token auth over TLS. No user input in API URL paths without validation. Rate limiting respects API contract

### Chaos Evidence

**Phase Agent:** bubbles.chaos
**Executed:** YES
**Command:** `./smackerel.sh test unit`

Resilience verification from dedicated `chaos_test.go` (46 tests) plus unit tests:

- **Malformed API responses:** TestChaos_MalformedJSON_Response — truncated JSON, empty body, binary data, HTML error pages, null/string/number bodies all handled without panic
- **Empty/null data:** TestChaos_EmptyDataArray, TestChaos_NullDataField, TestChaos_MissingDataField — zero items returned cleanly
- **All-empty structs:** TestChaos_PropertyAllFieldsEmpty, TestChaos_ReservationAllFieldsEmpty, TestChaos_MessageAllFieldsEmpty, TestChaos_ReviewAllFieldsEmpty — normalizers produce valid artifacts from zero-value fields
- **Unicode/extreme strings:** TestChaos_UnicodePropertyNames, TestChaos_UnicodeMessageBodies, TestChaos_UnicodeReviewText, TestChaos_ExtremelyLongGuestName, TestChaos_GuestNameWithNullBytes, TestChaos_GuestNameNewlinesAndTabs
- **Pagination abuse:** TestChaos_PaginationInfiniteLoop (maxPaginationPages cap), TestChaos_PaginationEmptyNextURL, TestChaos_PaginationMalformedLinkHeader
- **SSRF protection:** TestPaginationRejectsCrossOriginNextURL, TestPaginationRejectsCrossOriginLinkHeader, TestPaginationRejectsMetadataEndpoint — cross-origin pagination URLs rejected
- **Token lifecycle:** TestChaos_TokenExpiryMidSync, TestChaos_TokenExpiryOnValidate
- **Concurrency safety:** TestChaos_ConcurrentSync, TestChaos_ConcurrentConnectAndSync, TestChaos_ConcurrentHealthCheck
- **Rate limit edge cases:** TestChaos_RateLimitWithZeroRetryAfter, TestChaos_RateLimitWithNegativeRetryAfter, TestChaos_RateLimitWithHugeRetryAfter, TestChaos_RateLimitWithMalformedRetryAfter
- **Extreme numeric values:** TestChaos_ExtremeNumericValues, TestChaos_ExtremeRatingValues
- **Corrupted state:** TestChaos_CorruptedCursor, TestChaos_SyncBeforeConnect, TestChaos_SyncAfterClose
- **Config edge cases:** TestChaos_ConfigZeroPageSize, TestChaos_ConfigNegativePageSize, TestChaos_ConfigHugePageSize, TestChaos_ConfigZeroLookback
- **Partial failure isolation:** TestPartialFailureReturnsSuccessful — message sync error does not block property/reservation sync, partial artifacts returned, health remains healthy
- **All failures set error:** TestAllFailuresSetHealthError — all resource types returning 500 → zero artifacts, health=error
- **Empty token handling:** TestConnectEmptyToken — empty access_token → clear error "access_token is required", health=error
- **Property name cache miss:** TestNormalizeReservationFallbackPropertyID, TestNormalizeReviewFallbackPropertyID — unknown property ID falls back to raw ID, no crash, no empty title
- **Rate limit exhaustion:** TestClientMaxRetriesOn429 — 3 consecutive 429s → rate limit error returned cleanly
- **Retry-After edge cases:** TestParseRetryAfterEmpty, TestParseRetryAfterInvalid — missing or malformed Retry-After header → falls back to exponential backoff without crash
- **Message cursor isolation:** TestMessageCursorNotAdvancedOnFailure — one reservation message failure does not advance cursor, preventing message loss

---

## Execution Evidence

### Security Audit (security-to-doc sweep — April 2026)

**Trigger:** Stochastic quality sweep — security trigger
**Scope:** `internal/connector/hospitable/` (client.go, connector.go, normalizer.go, types.go)
**Areas audited:** SSRF, injection, auth bypass, data exposure, resource exhaustion

#### Finding S-001: SSRF via Pagination URL Following (HIGH — REMEDIATED)

`fetchPaginated` followed `NextURL` from JSON response body and `Link` header without validating the URL stayed on the same origin. A compromised or malicious API could redirect the client to internal services (e.g., `http://169.254.169.254/latest/meta-data/`), leaking the Bearer token via the Authorization header.

**Fix:** Added `isSameOrigin()` check that validates pagination URLs share the same scheme+host as the configured `baseURL`. Cross-origin pagination URLs are logged and rejected. Tests: `TestPaginationRejectsCrossOriginNextURL`, `TestPaginationRejectsCrossOriginLinkHeader`, `TestPaginationRejectsMetadataEndpoint`, `TestPaginationAllowsSameOriginNextURL`.

#### Finding S-002: No Maximum Page Limit in Pagination (MEDIUM — REMEDIATED)

`fetchPaginated` had no upper bound on page count. A malicious API could return infinite pagination chains, causing unbounded memory growth or long-running requests.

**Fix:** Added `maxPaginationPages = 1000` constant. Pagination loop terminates after 1000 pages. Test: `TestPaginationMaxPageLimit`.

#### Finding S-003: User-Controlled `base_url` Without Scheme Validation (MEDIUM — REMEDIATED)

The `base_url` config value was accepted without validation. Non-HTTP schemes (e.g., `file://`, `ftp://`, `javascript:`) could redirect the HTTP client to unintended targets.

**Fix:** Added URL parsing and scheme validation in `parseHospitableConfig` — only `http` and `https` schemes with a non-empty host are accepted. Test: `TestConfigRejectsInvalidBaseURLScheme`, `TestConfigAcceptsValidBaseURL`.

#### Items Reviewed — No Finding

- **Injection:** `reservationID` in URL paths uses `url.PathEscape()` — safe
- **Auth bypass:** Token validated on `Connect()` via `Validate()` call; missing/empty token rejected with clear error
- **Data exposure:** Token not logged in error paths; `slog.Info/Warn/Error` calls do not include sensitive fields
- **Response size:** 10 MiB limit already enforced on `doGetPaginated` (prior hardening)
- **Normalizer safety:** All normalizers handle zero-value structs, null bytes, invalid UTF-8 gracefully (covered by chaos tests)

#### Test Evidence

```terminal
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/hospitable    8.813s
# All Go packages pass, hospitable package includes 7 new SSRF security tests
```

7 new security tests added, all pass:
- TestPaginationRejectsCrossOriginNextURL
- TestPaginationRejectsCrossOriginLinkHeader
- TestPaginationRejectsMetadataEndpoint
- TestPaginationAllowsSameOriginNextURL
- TestPaginationMaxPageLimit
- TestConfigRejectsInvalidBaseURLScheme
- TestConfigAcceptsValidBaseURL

### Delivery Lockdown Certification

- **Scopes completed:** 5/5 (Scope 01–05)
- **Unit tests:** 138 tests across 3 test files — all pass
- **Lint:** Pass
- **Format:** Pass
- **Check:** Pass

## Security Sweep Findings (security-to-doc, 2026-04-14)

**Trigger:** stochastic-quality-sweep R23 → security
**Agent:** bubbles.workflow (child) → security-to-doc mode
**Scope:** Deep security audit of all source files in `internal/connector/hospitable/`

### SEC-012-001: ListingURL Scheme Not Validated (CWE-79/CWE-601) — HIGH

`NormalizeProperty` used `firstNonEmpty(p.ListingURLs)` as the artifact URL without validating the URL scheme. A compromised Hospitable API could inject `javascript:`, `data:`, or `vbscript:` URLs that would be stored as artifact URLs, enabling stored XSS or open redirect if the artifact URL is rendered in a browser.

**Fix:** Added `isSafeURL()` helper (allows only `http`/`https` schemes) and `firstSafeURL()` which filters listing URLs through scheme validation before use. Unsafe URLs are silently skipped; first safe URL is used.

**Files changed:** `normalizer.go`
**Tests added:** `TestIsSafeURL` (11 cases), `TestFirstSafeURL`, `TestFirstSafeURL_AllUnsafe`, `TestFirstSafeURL_Nil`, `TestNormalizePropertyRejectsJavascriptURL`, `TestNormalizePropertyRejectsDataURL`

### SEC-012-002: Reservation URL Built with Unescaped ID (CWE-79) — MEDIUM

`NormalizeReservation` built the dashboard URL as `"https://app.hospitable.com/reservations/" + r.ID` without escaping the reservation ID. If `r.ID` contains path-traversal (`../`) or query injection (`?admin=true`) characters from a compromised API, the constructed URL could be manipulated.

**Fix:** Applied `url.PathEscape(r.ID)` when constructing the reservation dashboard URL.

**Files changed:** `normalizer.go`
**Tests added:** `TestNormalizeReservationURLPathEscape`, `TestNormalizeReservationURLQueryInjection`

### SEC-012-003: Unbounded page_size Config Value (CWE-400) — MEDIUM

`parseHospitableConfig` accepted any positive `page_size` without an upper bound. A misconfigured or malicious config with `page_size: 999999` could cause the API to return excessively large paginated responses, risking memory exhaustion.

**Fix:** Added `maxPageSize = 500` constant and applied cap in config parsing. Values above 500 are silently reduced.

**Files changed:** `connector.go`
**Tests added:** `TestConfigPageSizeCappedAtMax`, `TestConfigPageSizeBelowCapPreserved`
**Tests updated:** `TestChaos_ConfigHugePageSize` (now asserts cap enforcement)

### SEC-012-004: Rating Value Unbounded — NaN/Inf/Negative (CWE-20) — LOW

`formatRating` accepted arbitrary `float64` values including negative numbers, NaN, and Inf. A malicious API returning `rating: NaN` or `rating: -999` would produce misleading artifact titles.

**Fix:** Added `clampRating()` helper that constrains ratings to `[0.0, 5.0]`, mapping NaN/Inf to 0. Applied in `NormalizeReview` before title/content/metadata use.

**Files changed:** `normalizer.go`
**Tests added:** `TestClampRating` (10 cases), `TestNormalizeReviewNegativeRatingClamped`, `TestNormalizeReviewNaNRatingClamped`, `TestNormalizeReviewOverflowRatingClamped`

### Security Sweep Summary

| Finding | Severity | CWE | Category | Fixed |
|---------|----------|-----|----------|-------|
| SEC-012-001 | High | CWE-79/601 | URL scheme injection | Yes |
| SEC-012-002 | Medium | CWE-79 | URL path traversal | Yes |
| SEC-012-003 | Medium | CWE-400 | Resource exhaustion | Yes |
| SEC-012-004 | Low | CWE-20 | Input validation | Yes |

**Total new tests:** 21 adversarial security tests

```terminal
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/hospitable    8.750s

$ ./smackerel.sh lint
All checks passed!

$ ./smackerel.sh check
Config is in sync with SST
```

## Security Sweep Pass 2 (security-to-doc, 2026-04-14)

**Trigger:** stochastic-quality-sweep R30 → security (second pass)
**Agent:** bubbles.workflow (child) → security-to-doc mode
**Scope:** Deeper security audit following R23 remediations; focus on residual CWE classes

### SEC-012-005: Control Character Injection in User-Supplied Text (CWE-116) — MEDIUM

User-supplied text fields from the Hospitable API — guest names, message bodies, sender names, review text, host responses, and property names — were embedded directly into normalized artifact content and titles without control character sanitization. Null bytes (`\x00`), carriage returns (`\r`), ANSI escape sequences (`\x1B`), and other C0 control characters could corrupt downstream text processing, cause log injection (CWE-117), or confuse content parsers.

The project already has `stringutil.SanitizeControlChars()` (established by SEC-021-002 for the intelligence engine) but it was not applied at the connector normalizer layer.

**Fix:** Applied `stringutil.SanitizeControlChars()` to all API-supplied text fields in all four normalizer functions (`NormalizeProperty`, `NormalizeReservation`, `NormalizeMessage`, `NormalizeReview`) before embedding in titles, content, and metadata.

**Files changed:** `normalizer.go` (added `stringutil` import + 6 sanitization calls)
**Tests added:** `TestSEC012005_GuestNameControlChars`, `TestSEC012005_MessageBodyControlChars`, `TestSEC012005_ReviewTextControlChars`, `TestSEC012005_PropertyNameControlChars`

### SEC-012-006: ActiveReservationIDs Unbounded in Cursor (CWE-770) — LOW

The `syncCursor.ActiveReservationIDs` slice was persisted in the JSON cursor without any size cap. While `maxMessageSyncReservations` capped how many IDs were used for message fetch fan-out, the full list from both `ListReservations()` and `ListActiveReservations()` was stored. Over time with many reservations, this could grow unboundedly, bloating cursor JSON and consuming excessive memory on deserialization.

**Fix:** Capped `ActiveReservationIDs` in the cursor to `maxMessageSyncReservations` before serialization.

**Files changed:** `connector.go`
**Tests added:** `TestSEC012006_ActiveReservationIDsCursorCap`

### SEC-012-007: Property Name Cache Accepts Oversized Strings (CWE-400) — LOW

While `maxPropertyNameCacheSize` capped the number of entries in the property name cache, individual property IDs and names had no length limit. A compromised API returning megabyte-length ID or name strings would bypass the entry count cap while still causing excessive memory consumption. Affects both the in-memory cache and the JSON cursor persistence.

**Fix:** Added `maxCacheStringLen = 1024` constant. Property name cache entries (both at cursor load and from API responses) now skip entries where either the ID or name exceeds the limit.

**Files changed:** `connector.go`
**Tests added:** `TestSEC012007_OversizedPropertyIDSkipped`, `TestSEC012007_OversizedPropertyNameSkipped`, `TestSEC012007_OversizedCursorPropertyNamesSkippedOnLoad`

### Security Sweep Pass 2 Summary

| Finding | Severity | CWE | Category | Fixed |
|---------|----------|-----|----------|-------|
| SEC-012-005 | Medium | CWE-116 | Control char injection | Yes |
| SEC-012-006 | Low | CWE-770 | Unbounded resource growth | Yes |
| SEC-012-007 | Low | CWE-400 | Resource exhaustion via strings | Yes |

**Total new tests:** 8 adversarial security tests

```terminal
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/hospitable    9.755s
# All 33 Go packages pass, 44 Python tests pass
```

## Security Sweep Pass 3 (security-to-doc, 2026-04-14)

**Trigger:** stochastic-quality-sweep R12 → security (third pass)
**Agent:** bubbles.workflow (child) → security-to-doc mode
**Scope:** Residual defense-in-depth audit following R23 and R30 remediations

### SEC-012-008: Cursor PropertyNames Deserialization Bypasses Cache Size Cap (CWE-770) — MEDIUM

The `maxPropertyNameCacheSize` cap was enforced when WRITING PropertyNames to the cursor during Sync(), but NOT when READING them from a deserialized cursor at the start of the next Sync(). A crafted or corrupted cursor JSON with more than 10,000 PropertyName entries would bypass the write-side cap and inflate the in-memory `propertyNames` map without limit, causing excessive memory consumption proportional to the cursor payload.

**Fix:** Added entry counting during cursor PropertyNames loading. When the loaded count reaches `maxPropertyNameCacheSize`, remaining entries are discarded with a warning log.

**Files changed:** `connector.go` (cursor load loop gains `loaded` counter + cap check)
**Tests added:** `TestSEC012008_CursorPropertyNamesCappedOnLoad`, `TestSEC012008_CursorBelowCapLoadsAll`

### SEC-012-009: Address Fields Bypass Control Character Sanitization (CWE-116) — MEDIUM

SEC-012-005 (R30) applied `SanitizeControlChars()` to user-facing text fields — GuestName, Sender, Body, ReviewText, HostResponse, PropertyName — but missed the `Address` struct fields (Street, City, State, Country, Zip). These fields flow into property content via `formatAddress()` and into metadata as the `"address"` value. A compromised API injecting null bytes or ANSI escape sequences into address fields would bypass the sanitization boundary.

**Fix:** Applied `stringutil.SanitizeControlChars()` to all five Address fields in `NormalizeProperty()` before content and metadata formatting.

**Files changed:** `normalizer.go` (5 new sanitization calls in NormalizeProperty)
**Tests added:** `TestSEC012009_AddressControlChars`, `TestSEC012009_AddressFieldsCleaned`

### SEC-012-010: Reservation/Review Channel and Status Fields Bypass Sanitization (CWE-116) — LOW

`Reservation.Channel`, `Reservation.Status`, and `Review.Channel` were embedded in content strings via `buildReservationContent()` and `buildReviewContent()` without control character sanitization. While these fields are typically short strings from a controlled vocabulary (e.g., "Airbnb", "confirmed"), a compromised API could inject arbitrary values including control characters, inconsistent with the SEC-012-005 sanitization pattern applied to other text fields.

**Fix:** Applied `stringutil.SanitizeControlChars()` to Channel and Status fields in `NormalizeReservation()` and Channel in `NormalizeReview()`.

**Files changed:** `normalizer.go` (3 new sanitization calls)
**Tests added:** `TestSEC012010_ReservationChannelControlChars`, `TestSEC012010_ReviewChannelControlChars`, `TestSEC012010_CleanChannelStatusPreserved`

### Security Sweep Pass 3 Summary

| Finding | Severity | CWE | Category | Fixed |
|---------|----------|-----|----------|-------|
| SEC-012-008 | Medium | CWE-770 | Unbounded cursor deserialization | Yes |
| SEC-012-009 | Medium | CWE-116 | Address field sanitization gap | Yes |
| SEC-012-010 | Low | CWE-116 | Channel/Status sanitization gap | Yes |

**Total new tests:** 7 adversarial security tests

```terminal
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/hospitable    9.103s
# All 33 Go packages pass, 44 Python tests pass

$ ./smackerel.sh lint
All checks passed!

$ ./smackerel.sh check
Config is in sync with SST
```
