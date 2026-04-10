# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

_No scopes have been implemented yet._

---

## Security Audit: 2026-04-10

**Trigger:** stochastic-quality-sweep → security-to-doc
**Scope:** `internal/connector/markets/markets.go`, `internal/connector/markets/markets_test.go`

### Findings

| # | Finding | Severity | OWASP | Remediated |
|---|---------|----------|-------|------------|
| S1 | URL/query-param injection via unsanitized symbol names in `fetchFinnhubQuote` — symbols concatenated directly into URL string | HIGH | A03:2021 Injection | Yes |
| S2 | URL injection via unsanitized coin IDs in `fetchCoinGeckoPrices` — IDs concatenated directly into URL string | HIGH | A03:2021 Injection | Yes |
| S3 | No response body size limit — unbounded `json.Decode` from remote API servers could cause OOM | MEDIUM | A05:2021 Security Misconfiguration | Yes |
| S4 | API key leakage via `fmt.Sprintf` URL construction — token visible in error traces/logs | MEDIUM | A02:2021 Sensitive Data Exposure | Yes |
| S5 | No input validation on symbol/coin ID format — allows special characters through config | MEDIUM | A03:2021 Injection | Yes |
| S6 | Unbounded watchlist size — no maximum on symbols per category | LOW | A05:2021 Security Misconfiguration | Yes |

### Remediations Applied

1. **S1+S2+S4: Safe URL construction** — Replaced `fmt.Sprintf` URL concatenation with `net/url.Parse` + `url.Query().Set()` for both Finnhub and CoinGecko endpoints. Query parameters are now properly encoded and API keys are never part of raw string concatenation.

2. **S3: Response body size limit** — Added `io.LimitReader(resp.Body, maxResponseBodyBytes)` (1MB cap) on all `json.NewDecoder` calls for both Finnhub and CoinGecko responses.

3. **S5: Input validation regexes** — Added `validSymbolRe` (`^[A-Za-z0-9.\-]{1,10}$`) for stock/ETF symbols and `validCoinIDRe` (`^[a-z0-9\-]{1,64}$`) for CoinGecko IDs. Validation runs at config parse time and at fetch time as defense-in-depth.

4. **S6: Watchlist size cap** — Added `maxWatchlistSymbols = 100` per category. `parseMarketsConfig` rejects configs exceeding the limit.

### Tests Added

- `TestParseMarketsConfig_RejectsInjectionSymbol` — 10 injection/malformed symbol cases
- `TestParseMarketsConfig_AcceptsValidSymbols` — 6 valid symbol cases
- `TestParseMarketsConfig_WatchlistSizeLimit` — exceeding 100-symbol cap
- `TestFetchFinnhubQuote_RejectsInvalidSymbol` — 5 adversarial symbol cases at fetch time

### Evidence

- All Go unit tests pass: `./smackerel.sh test unit` — `internal/connector/markets 0.041s`
- `./smackerel.sh check` passes cleanly
