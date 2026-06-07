# Scopes: BUG-038-001

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).

## Scope 1 — Bound the Google Drive provider client (fail-loud SST timeout)

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] SST key `drive.providers.google.http_response_header_timeout_seconds` added to `config/smackerel.yaml` and generated via `required_value` in `scripts/commands/config.sh` (assignment + heredoc emit)
      → Evidence: report.md `### Code Diff Evidence` + `### Validation Evidence` (`config generate` OK)
- [x] `loadDriveConfig` parses the key fail-loud (naming `DRIVE_PROVIDER_GOOGLE_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS`) into `DriveGoogleProviderConfig.HTTPResponseHeaderTimeoutSeconds`
      → Evidence: report.md `## Test Evidence` (`TestDriveConfigValidationRequiresEverySSTField/...HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS` PASS)
- [x] The key round-trips into the typed config (asserted `== 30`)
      → Evidence: report.md `## Test Evidence` (`TestDriveConfigPopulatesEveryField` PASS)
- [x] `cmd/core/wiring.go` builds the Google provider client with `Transport.ResponseHeaderTimeout` + dial + TLS-handshake timeouts from the SST value; NO whole-request `Timeout` (large downloads not aborted)
      → Evidence: report.md `### Code Diff Evidence` (wiring.go diff; BUILD=0; VET=0)
- [x] `./smackerel.sh config generate` resolves the key into the generated env without error
      → Evidence: report.md `### Validation Evidence` (`config-validate ... OK`; key present in dev.env)
- [x] `go build ./...`, `go vet`, drive config tests green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-038-TIMEOUT-01/02` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage for the new behavior — the fail-loud + populate config tests lock the required key; they fail if the key is dropped or mistyped
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression suite passes — the full `internal/config` package runs green with the new required key wired into `setRequiredEnv` and `driveSSTKeys`
      → Evidence: report.md `### Validation Evidence` (`ok ... internal/config`)

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-038-TIMEOUT-01 | TestDriveConfigValidationRequiresEverySSTField (timeout key subtest) | internal/config/drive_config_test.go | unit (fail-loud) | SCN-038-TIMEOUT-01 |
| T-038-TIMEOUT-02 | TestDriveConfigPopulatesEveryField | internal/config/drive_config_test.go | unit (populate) | SCN-038-TIMEOUT-02 |

### Non-Goals

- Enforcing `drive.rate_limits.requests_per_minute` (limiter feature; owner follow-up).
- OAuth token refresh (design.md §11 deferral).
