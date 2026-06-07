# Report: BUG-038-001 — bound the Google Drive provider client

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07

## Summary

The Google Drive provider was wired with `http.DefaultClient` (no timeout), so a
connect-but-never-respond endpoint blocked the scan/monitor goroutine forever.
The fix adds a fail-loud SST key
`drive.providers.google.http_response_header_timeout_seconds` and builds the
provider client with a `Transport` whose `ResponseHeaderTimeout` + dial + TLS
timeouts derive from it — bounding the hang WITHOUT capping whole-request
duration, so large-file downloads (up to `drive.limits.max_file_size_bytes`)
are not aborted mid-body.

## Root Cause

`cmd/core/wiring.go` passed `http.DefaultClient` (whose `Timeout == 0`) to
`ConfigureRuntime`. Every `p.client.Do(req)` in the provider therefore had no
client-level bound on time-to-response, and a hung endpoint parked the worker.

## Fix

New SST key + a bounded `Transport` in wiring. A whole-request `Timeout` is
deliberately NOT set (it would abort large downloads); `ResponseHeaderTimeout`
catches the hang and per-call context deadlines bound mid-body stalls.

## Test Evidence

### Fail-loud + populate config tests pass against the new required key

```
$ go test -v -count=1 -run 'TestDriveConfigValidationRequiresEverySSTField/DRIVE_PROVIDER_GOOGLE_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS|TestDriveConfigPopulatesEveryField' ./internal/config/
=== RUN   TestDriveConfigValidationRequiresEverySSTField
=== RUN   TestDriveConfigValidationRequiresEverySSTField/DRIVE_PROVIDER_GOOGLE_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS
--- PASS: TestDriveConfigValidationRequiresEverySSTField (0.00s)
=== RUN   TestDriveConfigPopulatesEveryField
--- PASS: TestDriveConfigPopulatesEveryField (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.034s
```

The `...HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS` subtest proves `loadDriveConfig`
fails loud (naming the env var) when the key is empty; `PopulatesEveryField`
asserts the value round-trips as `30`.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/config/ ./cmd/core/
# VET=0
$ git diff --stat
 cmd/core/wiring.go                   | 24 +++++++++++++++++++++++-
 config/smackerel.yaml                |  9 ++++++++-
 internal/config/drive.go             | 15 +++++++++++++++
 internal/config/drive_config_test.go |  4 ++++
 internal/config/validate_test.go     |  1 +
 scripts/commands/config.sh           |  2 ++
 6 files changed, 53 insertions(+), 2 deletions(-)
```

Files changed: `config/smackerel.yaml` + `scripts/commands/config.sh` (new SST
key), `internal/config/drive.go` (struct field + fail-loud parse),
`cmd/core/wiring.go` (bounded `Transport`), `internal/config/drive_config_test.go`
+ `internal/config/validate_test.go` (test wiring). No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Generated ~/smackerel/config/generated/dev.env
$ grep -n 'DRIVE_PROVIDER_GOOGLE_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS' config/generated/dev.env
config/generated/dev.env:123:DRIVE_PROVIDER_GOOGLE_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS=30
$ go test -count=1 ./internal/config/
ok      github.com/smackerel/smackerel/internal/config  31.375s
```

The SST pipeline resolves the new required key end-to-end (config-validate OK;
key present in dev.env), and the full `internal/config` package is green.

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration added)
$ git status --short config/generated/
# (gitignored — generated env not committed)
$ git diff --stat cmd/core/wiring.go
 cmd/core/wiring.go | 24 +++++++++++++++++++++++-
```

The diff is confined to the SST source, the config loader/tests, and the
wiring client construction. No migration, no `.github/bubbles` framework files,
no edits to the parent spec 038 planning artifacts. Generated env files are
gitignored.

## Completion Statement

The Google Drive provider client now bounds connect/header hangs via a fail-loud
SST `http_response_header_timeout_seconds`, while large-file downloads remain
uncapped. The fail-loud + populate config tests lock the required key; the SST
pipeline resolves it; build, vet, and the full `internal/config` package pass
(see Validation Evidence). Scope 1 DoD is complete (9/9). The unused
`rate_limits.requests_per_minute` enforcement is flagged as a separate owner
follow-up. BUG-038-001 is Done.
