# BUG-038-001: Google Drive provider used http.DefaultClient (no timeout) — a hung endpoint blocks the scan/monitor goroutine indefinitely

**Status:** Resolved (bounded response-header timeout via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Stochastic Quality Sweep Round 17 (parent: stochastic-quality-sweep) — `devops`, parent-expanded
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/038-cloud-drives-integration/`
**Affected surface:** `cmd/core/wiring.go` (provider client construction), `internal/config/drive.go`, `config/smackerel.yaml`, `scripts/commands/config.sh`

## Summary

`cmd/core/wiring.go` passed `http.DefaultClient` to the Google Drive provider's
`ConfigureRuntime`. `http.DefaultClient` has **no timeout**, so a provider
endpoint that accepts a connection but never responds (or stalls before sending
response headers) blocks the calling scan/monitor goroutine indefinitely. Every
other Smackerel connector (discord, weather, twitter, rss, youtube, imap,
hospitable, markets, …) constructs an `http.Client` with an explicit timeout;
the drive provider was the lone exception.

## Mechanism (verified by code reading at repo HEAD)

- `cmd/core/wiring.go` → `googleProvider.ConfigureRuntime(svc.pg.Pool, http.DefaultClient, …)`.
- `internal/drive/google/google.go` → all calls go through `p.client.Do(req)`
  (`ListFolder`, `GetFile`, `PutFile`, `Changes`, OAuth exchange). The provider
  stores the injected client verbatim; `http.DefaultClient.Timeout == 0`.
- A scan/monitor worker calling a hung endpoint blocks on `p.client.Do` with no
  bound (unless the caller's context happens to carry a deadline).

## Impact / Severity rationale (Medium)

- **Reliability:** a single hung Drive endpoint can park a scan/monitor
  goroutine forever, degrading the connector silently with no clear error.
- **Fails closed-ish:** it hangs rather than corrupting data, but recovery
  requires a restart.
- **Not remote-unauthenticated:** triggered by the upstream provider's
  behavior, not an attacker.

## Why a whole-request timeout is the WRONG fix

The Drive provider downloads files up to `drive.limits.max_file_size_bytes`
(100 MiB by default). A blunt `http.Client.Timeout` bounds the ENTIRE request
including body read, so a short whole-request timeout would abort legitimate
large-file downloads mid-stream. The correct knob is a **response-header
timeout** (time-to-first-response-headers) plus dial/TLS-handshake timeouts:
these catch the connect-but-never-respond hang WITHOUT capping body-download
duration. Mid-body stalls remain bounded by the per-call context deadlines the
callers already pass via `http.NewRequestWithContext`.

## Fix (delivered)

1. New fail-loud SST key `drive.providers.google.http_response_header_timeout_seconds`
   (`config/smackerel.yaml`, `scripts/commands/config.sh`, parsed in
   `internal/config/drive.go` via `parsePositiveInt`).
2. `cmd/core/wiring.go` builds the Google provider client with a
   `http.Transport` whose `ResponseHeaderTimeout`, `DialContext` timeout, and
   `TLSHandshakeTimeout` come from the SST value (no whole-request `Timeout`).

## Noted (not fixed here — separate follow-up)

`drive.rate_limits.requests_per_minute` (`DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE`,
600) is SST-declared and parsed into `DriveRateLimitsConfig.RequestsPerMinute`
but is **not yet enforced** by a runtime limiter. Enforcing it is a feature
(a token-bucket/limiter), out of scope for this reliability fix; flagged for
the spec 038 owner. It is not a NO-DEFAULTS violation (no fallback default; the
value is required and parsed) — only unwired.

## Cross-References

- Wiring: `cmd/core/wiring.go`
- Config: `internal/config/drive.go`, `config/smackerel.yaml`, `scripts/commands/config.sh`
- Tests: `internal/config/drive_config_test.go`, `internal/config/validate_test.go`
- Parent spec/design: `../../spec.md`, `../../design.md` §11 (token-refresh deferral context)
