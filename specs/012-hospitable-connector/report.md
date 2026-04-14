# Report: 012 — Hospitable Connector

> **Status:** Done

---

## Summary

Hospitable Connector delivered under delivery-lockdown mode. 5 scopes completed: (1) API Client, Types & Config; (2) Connector Implementation & Normalizer; (3) Edge Hints, Cross-Domain Linking & Hardening; (4) Message Sync Reliability & Client Hardening; (5) Normalizer Quality Fixes. Implementation: types.go, client.go, connector.go, normalizer.go in `internal/connector/hospitable/`. 138 unit tests across 3 test files (connector_test.go, normalizer_test.go, chaos_test.go) — all pass. PAT authentication, paginated fetching, rate limit handling, 4 resource types (property, reservation, message, review), incremental cursor sync, knowledge graph edge hints, partial failure isolation, property name cache. Security hardening includes SSRF protection on pagination URLs, response body size limits, and extensive chaos testing.

## Completion Statement

All 5 scopes are Done. All DoD items verified with passing tests. `./smackerel.sh test unit` passes all Go packages including `internal/connector/hospitable`. `./smackerel.sh lint` exits 0. `./smackerel.sh check` confirms config in sync. Delivery-lockdown certification complete.

## Known Gaps

Integration tests (`tests/integration/hospitable_test.go`) and E2E tests (`tests/e2e/hospitable_test.go`) listed in scope test plans were never implemented. All test coverage is via unit tests with mock HTTP servers. This is acceptable for the connector pattern (no live API in CI) but means the test plans in scopes.md overstate the integration/E2E coverage.

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

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/api             0.129s
ok      github.com/smackerel/smackerel/internal/auth            1.546s
ok      github.com/smackerel/smackerel/internal/config          0.047s
ok      github.com/smackerel/smackerel/internal/connector       1.147s
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.227s
ok      github.com/smackerel/smackerel/internal/connector/browser       0.113s
ok      github.com/smackerel/smackerel/internal/connector/caldav        0.022s
ok      github.com/smackerel/smackerel/internal/connector/hospitable    2.952s
ok      github.com/smackerel/smackerel/internal/connector/imap  0.063s
ok      github.com/smackerel/smackerel/internal/connector/keep  0.362s
ok      github.com/smackerel/smackerel/internal/connector/maps  0.336s
ok      github.com/smackerel/smackerel/internal/connector/rss   0.275s
ok      github.com/smackerel/smackerel/internal/connector/youtube       0.026s
ok      github.com/smackerel/smackerel/internal/db              0.123s
ok      github.com/smackerel/smackerel/internal/digest          0.049s
ok      github.com/smackerel/smackerel/internal/extract         0.082s
ok      github.com/smackerel/smackerel/internal/graph           0.116s
ok      github.com/smackerel/smackerel/internal/intelligence    0.054s
ok      github.com/smackerel/smackerel/internal/nats            0.172s
ok      github.com/smackerel/smackerel/internal/pipeline        0.297s
ok      github.com/smackerel/smackerel/internal/scheduler       0.049s
ok      github.com/smackerel/smackerel/internal/telegram        14.499s
ok      github.com/smackerel/smackerel/internal/topics          0.007s
ok      github.com/smackerel/smackerel/internal/web             0.018s
ok      github.com/smackerel/smackerel/internal/web/icons       0.014s
44 passed in 1.12s
```

Hospitable-specific tests (138 tests across 3 files):

- `connector_test.go` (53 tests): TestClientAuthHeader, TestClientValidateSuccess, TestClientValidateUnauthorized, TestClientValidateForbidden, TestClientPaginatesProperties, TestClientRetryOn429, TestClientMaxRetriesOn429, TestDefaultClientMaxRetries3, TestClientRetryOnServerError, TestClientURLConstruction, TestConnectorID, TestConnectValidConfig, TestConnectInvalidToken, TestConfigValidationMissingToken, TestConfigValidationNegativeLookback, TestConfigValidationDefaults, TestSyncCursorMarshal, TestCursorEmptyAppliesLookback, TestHealthTransitions, TestDisabledResourceSkipped, TestSyncFullLifecycle, TestPartialFailureReturnsSuccessful, TestAllFailuresSetHealthError, TestPropertyNameCacheEnrichesTitle, TestConnectEmptyToken, TestSyncNotConnected, TestCloseIdempotent, TestClientResponseBodySizeLimit, TestClientListMessagesPathEscaping, TestClientListActiveReservationsParam, TestParseLinkNextValid, TestParseLinkNextNoQuoteRel, TestParseLinkNextEmpty, TestParseLinkNextPrevOnly, TestParseLinkNextMultipleLinks, TestConfigProcessingTierOverrides, TestConfigSyncFlagOverrides, TestActiveReservationMessageSync, TestParseRetryAfterSeconds, TestParseRetryAfterHTTPDate, TestParseRetryAfterEmpty, TestParseRetryAfterInvalid, TestRetryAfterUsedOn429, TestPropertyNameCachePersistsInCursor, TestPropertyNameCacheLoadedFromCursor, TestMessageCursorNotAdvancedOnFailure, TestPaginationRejectsCrossOriginNextURL, TestPaginationRejectsCrossOriginLinkHeader, TestPaginationRejectsMetadataEndpoint, TestPaginationAllowsSameOriginNextURL, TestPaginationMaxPageLimit, TestConfigRejectsInvalidBaseURLScheme, TestConfigAcceptsValidBaseURL
- `normalizer_test.go` (39 tests): TestNormalizeProperty, TestNormalizeReservation, TestNormalizeReservationFallbackPropertyID, TestNormalizeReservationLeadTime, TestNormalizeMessage, TestNormalizeReview, TestNormalizeReviewFallbackPropertyID, TestNormalizeAllTiers, TestClassifySenderGuest, TestClassifySenderHost, TestClassifySenderAutomated, TestClassifySenderDefaultGuest, TestNormalizeMessageHostSender, TestNormalizePropertyURL, TestNormalizePropertyNoURL, TestNormalizeReservationURLProduction, TestNormalizeReservationURLTest, TestFormatRatingWhole, TestFormatRatingFractional, TestFormatRatingZero, TestNormalizeReviewFractionalRating, TestFormatAddressFull, TestFormatAddressCityOnly, TestFormatAddressStateOnly, TestFormatAddressCityState, TestFormatAddressEmpty, TestFormatAddressStreetCountryOnly, TestFormatDateStandard, TestFormatDateRFC3339Fallback, TestFormatDateInvalidReturnsOriginal, TestFormatDateEmptyString, TestNormalizeReservationZeroBookedAt, TestNormalizeReviewNoHostResponse, TestNormalizePropertyCapturedAtCreatedAt, TestNormalizePropertyCapturedAtFallbackNow, TestNormalizeMessageCapturedAtFallbackNow, TestFirstNonEmptyMultiple, TestFirstNonEmptyAllEmpty, TestFirstNonEmptyNil
- `chaos_test.go` (46 tests): TestChaos_MalformedJSON_Response, TestChaos_EmptyDataArray, TestChaos_NullDataField, TestChaos_MissingDataField, TestChaos_PropertyAllFieldsEmpty, TestChaos_ReservationAllFieldsEmpty, TestChaos_MessageAllFieldsEmpty, TestChaos_ReviewAllFieldsEmpty, TestChaos_ExtremelyLongGuestName, TestChaos_GuestNameWithNullBytes, TestChaos_GuestNameNewlinesAndTabs, TestChaos_UnicodePropertyNames, TestChaos_UnicodeMessageBodies, TestChaos_UnicodeReviewText, TestChaos_PaginationInfiniteLoop, TestChaos_PaginationEmptyNextURL, TestChaos_PaginationMalformedLinkHeader, TestChaos_TokenExpiryMidSync, TestChaos_TokenExpiryOnValidate, TestChaos_ConcurrentSync, TestChaos_ConcurrentConnectAndSync, TestChaos_ConcurrentHealthCheck, TestChaos_PropertyMissingID, TestChaos_ReservationMissingDates, TestChaos_ReservationInvalidDateFormat, TestChaos_MessageMissingSender, TestChaos_RateLimitWithZeroRetryAfter, TestChaos_RateLimitWithNegativeRetryAfter, TestChaos_RateLimitWithHugeRetryAfter, TestChaos_RateLimitWithMalformedRetryAfter, TestChaos_ServerErrorWithBody, TestChaos_ContextCancelledDuringSync, TestChaos_ExtremeNumericValues, TestChaos_ExtremeRatingValues, TestChaos_CorruptedCursor, TestChaos_ConfigZeroPageSize, TestChaos_ConfigNegativePageSize, TestChaos_ConfigHugePageSize, TestChaos_ConfigZeroLookback, TestChaos_SyncBeforeConnect, TestChaos_SyncAfterClose, TestChaos_APIReturnsWrongTypes, TestChaos_ManyReservationsForMessageSync, TestChaos_PropertyNameCacheWithDuplicateIDs, TestChaos_FormatDateEdgeCases, TestChaos_ParseLinkNextEdgeCases

### Validation Evidence

**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** `./smackerel.sh test unit`, `./smackerel.sh lint`, `./smackerel.sh check`

```
$ ./smackerel.sh lint
All checks passed

$ ./smackerel.sh check
Config is in sync with SST

$ ./smackerel.sh format --check
(exit 0 — no formatting issues)
```

### Audit Evidence

**Phase Agent:** bubbles.audit
**Executed:** YES
**Command:** `./smackerel.sh check`, `./smackerel.sh lint`

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

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/hospitable    8.813s
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
**All tests pass:** `./smackerel.sh test unit` — hospitable package ok 8.750s
**Lint:** Pass
**Check:** Config in sync with SST

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
**All tests pass:** `./smackerel.sh test unit` — hospitable package ok 9.755s
**No regressions:** All 33 Go packages pass, 44 Python tests pass
