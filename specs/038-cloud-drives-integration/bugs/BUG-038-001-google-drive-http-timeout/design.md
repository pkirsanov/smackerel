# Design: BUG-038-001

## Problem

`cmd/core/wiring.go` injected `http.DefaultClient` (no timeout) into the Google
Drive provider, so a connect-but-never-respond endpoint hung the scan/monitor
goroutine forever. See `bug.md` for the verified mechanism and the
why-not-whole-request-timeout rationale.

## Change

1. **SST key.** Add `drive.providers.google.http_response_header_timeout_seconds`
   to `config/smackerel.yaml` (dev value 30), generate it via `required_value`
   in `scripts/commands/config.sh`, and parse it fail-loud in
   `internal/config/drive.go` (`parsePositiveInt`, naming the env var) into
   `DriveGoogleProviderConfig.HTTPResponseHeaderTimeoutSeconds`.
2. **Client construction.** In `cmd/core/wiring.go`, build the Google provider
   client with:

   - `Transport.ResponseHeaderTimeout = N s` — bounds time-to-response-headers
     (catches the hang);
   - `Transport.DialContext` (net.Dialer Timeout = N s) and
     `TLSHandshakeTimeout = N s` — bound connection establishment;
   - `IdleConnTimeout`, `MaxIdleConns`, `MaxIdleConnsPerHost` — sensible pool
     tuning (hardcoded, matching the discord client precedent);
   - **no** whole-request `http.Client.Timeout` — large downloads must not be
     aborted mid-body.

## Why this shape

- `ResponseHeaderTimeout` is the precise knob for a client that does both small
  metadata calls and large body downloads: it bounds the common hang
  (server accepts, never responds) without limiting body-read time.
- Mid-body stalls remain bounded by the per-call context deadlines the callers
  already pass via `http.NewRequestWithContext`.
- Consistent with every other connector having an explicit client timeout;
  the drive provider was the lone `http.DefaultClient` exception.

## Schema / Blast Radius

- `config/smackerel.yaml`, `scripts/commands/config.sh` — new SST key (2 lines
  in config.sh: assignment + heredoc emit).
- `internal/config/drive.go` — struct field + fail-loud parse.
- `cmd/core/wiring.go` — provider client construction (uses already-imported
  `net`, `net/http`, `time`).
- `internal/config/drive_config_test.go`, `internal/config/validate_test.go` —
  test wiring for the new required key.
- No production-code behavior change beyond the bounded client; no schema
  migration.

## Alternatives Considered

- **`http.Client{Timeout: N}` (whole request).** Rejected: aborts legitimate
  large-file downloads (up to 100 MiB) mid-body.
- **No SST key, hardcode the timeout.** Rejected: violates SST NO-DEFAULTS; the
  timeout must be operator-tunable per environment.
- **Enforce the rate limit here too.** Deferred: a limiter is a feature, not
  part of this reliability fix (flagged for the owner in `bug.md`).
