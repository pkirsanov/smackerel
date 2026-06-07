# User Validation: BUG-038-001

**Reported by:** Stochastic Quality Sweep Round 17 (devops lens, parent-expanded)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — `drive.providers.google.http_response_header_timeout_seconds` is required; `loadDriveConfig` fails loud naming the env var when missing.
- [x] AC-2 — the value round-trips into `DriveGoogleProviderConfig.HTTPResponseHeaderTimeoutSeconds` (asserted `== 30`).
- [x] AC-3 — the Google provider client is built with `Transport.ResponseHeaderTimeout` + dial + TLS timeouts from the SST value; no whole-request `Timeout` (large downloads not aborted).
- [x] AC-4 — `./smackerel.sh config generate` resolves the key into the generated env without error.

## Notes

Reliability fix: replaces `http.DefaultClient` (no timeout) with a client that
bounds the connect-but-never-respond hang without capping large-file download
duration. The unused `rate_limits.requests_per_minute` enforcement is a noted
follow-up, not part of this fix.
