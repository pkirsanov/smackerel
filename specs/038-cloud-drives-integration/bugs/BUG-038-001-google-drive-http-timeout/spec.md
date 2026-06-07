# Spec: BUG-038-001 — Google Drive provider must bound connect/header hangs

## Expected Behavior

The Google Drive provider's HTTP client MUST bound the time spent waiting for a
connection and response headers, so a hung or unresponsive provider endpoint
cannot block a scan/monitor goroutine indefinitely. The bound MUST come from a
fail-loud SST value (no hidden default) and MUST NOT cap whole-request duration
(large-file downloads up to `drive.limits.max_file_size_bytes` must complete).

## Actual Behavior

`cmd/core/wiring.go` passed `http.DefaultClient` (no timeout) to the provider,
so a hung endpoint blocked indefinitely. See `bug.md` → "Mechanism".

## Acceptance Criteria

1. **AC-1 (SST key, fail-loud):** `drive.providers.google.http_response_header_timeout_seconds`
   is a required SST value; `loadDriveConfig` fails loud (naming the env var)
   when it is missing or non-positive.
2. **AC-2 (key populates):** the value round-trips into
   `DriveGoogleProviderConfig.HTTPResponseHeaderTimeoutSeconds`.
3. **AC-3 (client bounded):** the Google provider client is built with a
   `Transport` whose `ResponseHeaderTimeout`, dial, and TLS-handshake timeouts
   derive from the SST value; the whole-request `Timeout` is NOT set (large
   downloads are not aborted).
4. **AC-4 (pipeline):** `./smackerel.sh config generate` resolves the new key
   into the generated env without error.

## Out of Scope

- Enforcing `drive.rate_limits.requests_per_minute` (a limiter feature; flagged
  for the spec 038 owner in `bug.md`).
- OAuth token-refresh (design.md §11 deferral — separate concern).
- Per-call context deadlines (callers already pass them).

## Cross-References

- Bug detail + fix + the why-not-whole-request-timeout rationale: `bug.md`
- Parent spec/design: `../../spec.md`, `../../design.md`
