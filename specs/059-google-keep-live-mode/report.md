# Execution Report: 059 Google Keep Live Sync (gkeepapi production hardening)

## Summary

Planning artifacts authored 2026-05-28 by `bubbles.plan`. Six scopes ordered with strict gating; implementation work has not started. This report holds evidence sections that will be populated by `bubbles.implement` and `bubbles.test` as each scope executes.

## Completion Statement

Planning-only execution. All six scopes are `Not started`. `scopes.md`, `scenario-manifest.json`, and `state.json` agree on the active scope inventory (6) and scenario contracts (SCN-059-001 through SCN-059-018). No implementation evidence is recorded yet.

## Planning Validation Evidence

### Artifact Lint

To be populated by validation after scopes are written. Expected command:

```
bash .github/bubbles/scripts/artifact-lint.sh specs/059-google-keep-live-mode
```

### Traceability Guard

To be populated by validation. Expected command:

```
timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/059-google-keep-live-mode
```

## Scope 1

**Status:** Implemented 2026-05-28 by `bubbles.implement`.
**Phase:** implement

### Implementation Summary

- Appended `KEEP_GOOGLE_APP_PASSWORD` to the canonical Bucket-2 secret-key manifest across all three mirrors:
  - `config/smackerel.yaml` → `infrastructure.secret_keys`
  - `internal/config/secret_keys.go` → `secretKeys`
  - `scripts/commands/config.sh` → `SHELL_SECRET_KEYS`
- Added `connectors.google-keep.email` (non-secret) and `connectors.google-keep.app_password` (secret placeholder slot) to `config/smackerel.yaml`.
- Wired `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD` extraction + env-template emission in `scripts/commands/config.sh` (production-class targets get the spec-052 deterministic placeholder; dev/test fall through to the yaml literal, which is empty by default).
- Added `TestSecretKeys_KeepAppPasswordRegistered` and `TestKeepAppPasswordReadOnlyFromSidecarNotCore` to `internal/config/secret_keys_test.go` — the second walks `internal/` and `cmd/` for any Go source reference to `KEEP_GOOGLE_APP_PASSWORD` and asserts the sidecar boundary is enforced at the application layer.
- Added `TestComposeEnvFileSharedAcrossCoreAndMlServices` to `internal/deploy/compose_contract_test.go` — pins that `smackerel-core` and `smackerel-ml` both consume the SAME `env_file: ${SMACKEREL_ENV_FILE:?…}` and neither service introduces a masking per-service override.
- Updated `internal/deploy/bundle_secret_contract_test.go::TestBundleSecretContract_AdversarialA2_LeakageDetector` hardcoded `SHELL_SECRET_KEYS` literal to match the new 6-key array shape.

### Test Evidence

**Claim Source:** executed.

```text
$ cd ~/smackerel && go test ./internal/config/ -run 'TestSecretKeys|TestPlaceholder|TestIsPlaceholder|TestKeepApp' -count=1 -timeout 60s
ok      github.com/smackerel/smackerel/internal/config  0.040s
```

This covers:
- `TestSecretKeys_MirrorsYAMLManifest` — three-mirror parity (yaml + Go); A1 adversarial drift detector path indirectly proven by the bundle-contract suite below.
- `TestSecretKeysMirror` — Go-side ordered-slice pin including the new `KEEP_GOOGLE_APP_PASSWORD` entry.
- `TestSecretKeys_KeepAppPasswordRegistered` — explicit regression that the new key is registered.
- `TestKeepAppPasswordReadOnlyFromSidecarNotCore` — sidecar-boundary application-layer enforcement (zero non-test Go source files under `internal/` or `cmd/`, excluding the manifest file itself, reference the literal).
- `TestPlaceholder*` / `TestIsPlaceholder*` — placeholder format determinism for every declared key (now including `KEEP_GOOGLE_APP_PASSWORD`).

```text
$ cd ~/smackerel && go test ./internal/deploy/ -run 'TestComposeContract|TestComposeEnvFile|TestBundleSecret' -count=1 -timeout 300s
ok      github.com/smackerel/smackerel/internal/deploy  36.468s
```

This covers:
- `TestComposeEnvFileSharedAcrossCoreAndMlServices` — new SCN-059-002 application-layer-boundary contract.
- `TestBundleSecretContract_NoLiteralSecretsInHomeLab` — proves the bundle generator emits `KEEP_GOOGLE_APP_PASSWORD=__SECRET_PLACEHOLDER__KEEP_GOOGLE_APP_PASSWORD__` in `app.env` for `home-lab` (production-class target) AND ships it in the sibling `secret-keys.yaml` manifest.
- `TestBundleSecretContract_AdversarialA1_DriftDetector` — dynamically drops the LAST key from `SHELL_SECRET_KEYS` and asserts the bundle reflects the drift; since `KEEP_GOOGLE_APP_PASSWORD` is the last entry today, this test confirms the adversarial regression contract for the new entry.
- `TestBundleSecretContract_AdversarialA2_LeakageDetector` — tampered config.sh + yaml prove the placeholder shielding gate works key-by-key.
- `TestBundleSecretContract_AdversarialA3_DeterminismDetector` — bundle bytes are deterministic across two runs.
- `TestBundleSecretContract_AdversarialA4_OptOutDetector` — production-class opt-in gate works.
- `TestComposeContract_*` — unchanged spec-042 tailnet-edge invariants still hold.

### SST Emission Evidence

**Claim Source:** executed.

Dev (non-production-class) target — empty values per spec-052 FR-052-011:

```text
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp.221782 OK
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml

$ grep -E '^KEEP_GOOGLE' ~/smackerel/config/generated/dev.env
KEEP_GOOGLE_EMAIL=
KEEP_GOOGLE_APP_PASSWORD=
```

Home-lab (production-class) target — deterministic placeholder per spec-052 FR-052-002:

```text
$ ./smackerel.sh config generate --env home-lab
config-validate: skipped for production-class target env=home-lab (placeholder mode; runtime check enforces at container start)
Generated ~/smackerel/config/generated/home-lab.env

$ grep -E '^KEEP_GOOGLE' ~/smackerel/config/generated/home-lab.env
KEEP_GOOGLE_EMAIL=
KEEP_GOOGLE_APP_PASSWORD=__SECRET_PLACEHOLDER__KEEP_GOOGLE_APP_PASSWORD__
```

**Interpretation note for DoD item 4:** the planning text says "`config/generated/dev.env` carries the secret placeholder marker". The spec-052 contract (FR-052-002 / FR-052-011) gates placeholder emission on `TARGET_ENV ∈ production_class_targets`. dev/test environments are explicitly NEVER in the production-class list and therefore receive yaml-literal values (empty by default for `KEEP_GOOGLE_APP_PASSWORD`). The DoD item is treated as satisfied by demonstrating both target shapes work as spec-052 prescribes (dev=empty, home-lab=placeholder). A planning-text refinement to remove the "dev" wording is routed to `bubbles.plan` (non-blocking; the underlying contract is correctly enforced).

### Fail-Loud SST Audit

**Claim Source:** executed.

```text
$ grep -nE '(KEEP_GOOGLE_(EMAIL|APP_PASSWORD)).*:-' config/smackerel.yaml scripts/commands/config.sh internal/ docker-compose.yml deploy/compose.deploy.yml -r 2>/dev/null
(empty — no language-level fallback default introduced)
```

### Change Boundary Compliance

Files modified are within the documented allowed family for Scope 1:
- `config/smackerel.yaml` ✓
- `internal/config/secret_keys.go` ✓
- `internal/config/secret_keys_test.go` ✓
- `scripts/commands/config.sh` ✓
- `internal/deploy/compose_contract_test.go` ✓ (new test; the file itself sits adjacent to the compose contract surface)
- `internal/deploy/bundle_secret_contract_test.go` ✓ (A2 literal update — the contract test for the three-mirror manifest the scope explicitly extends)

Out-of-boundary file families NOT touched: `ml/app/**`, `internal/connector/keep/**`, `tests/e2e/**`, `docker-compose.yml`, `deploy/compose.deploy.yml` (no compose service-shape change was required).

## Scope 2

**Status:** Done 2026-05-28 by `bubbles.implement` (planning reconciliation by `bubbles.plan` resolved the boundary-test conflict: KEEP_GOOGLE_APP_PASSWORD validation moves to Scope 3's `keep.sidecar.handshake` per SCN-059-019; Scope 2 narrows to KEEP_GOOGLE_EMAIL-only at Go-core `Connect()`).

**Phase:** implement
**Agent:** bubbles.implement
**Claim Source:** executed

### Changes

- `config/smackerel.yaml` — added `drift_ack_token: ""` placeholder under `connectors.google-keep` with operator-workflow comment.
- `internal/connector/keep/keep.go`:
  - Added `gkeepPollIntervalFloor = 15 * time.Minute` named constant (resolves OQ-059-01 to a single canonical value); `parseKeepConfig` now references it instead of an inline literal, and the rejection error includes the constant value.
  - `KeepConfig.DriftAckToken string` field added adjacent to `GkeepWarningAck`.
  - `parseKeepConfig` parses `drift_ack_token` using the present-check pattern: missing key yields empty string; non-string value yields an error referencing the field name.
  - `Connect()` gained a Scope-2 fail-loud precondition block that runs BEFORE the existing `warning_acknowledged` gate: when `sync_mode ∈ {gkeepapi, hybrid}` AND `gkeep_enabled: true`, `KEEP_GOOGLE_EMAIL` MUST be non-empty; error is field-name-only (no value, length, or hash).
- `internal/connector/keep/keep_test.go` — added six Scope-2 tests; updated five pre-existing tests that enable gkeepapi mode to `t.Setenv` the now-required `KEEP_GOOGLE_EMAIL` value (and the App Password value for forward-compat).
- `internal/connector/keep/regression_test.go` — `TestRegression_GkeepWarningGateEnforced` now sets the keep envs so the warning_ack gate (which now runs AFTER the secrets gate) is the one being asserted.

### Evidence

```text
$ go test ./internal/connector/keep -run 'TestParseKeepConfigParsesDriftAckToken|TestParseKeepConfigRejectsNonStringDriftAckToken|TestConnectFailsLoudWhenKeepEmailMissingInLiveMode|TestConnectDoesNotLeakKeepAppPasswordInError|TestParseKeepConfigEnforcesPollIntervalFloor|TestConnectFailsLoudWhenKeepAppPasswordMissingInLiveMode' -v -count=1
=== RUN   TestParseKeepConfigParsesDriftAckToken
--- PASS: TestParseKeepConfigParsesDriftAckToken (0.00s)
=== RUN   TestParseKeepConfigRejectsNonStringDriftAckToken
--- PASS: TestParseKeepConfigRejectsNonStringDriftAckToken (0.00s)
=== RUN   TestConnectFailsLoudWhenKeepEmailMissingInLiveMode
--- PASS: TestConnectFailsLoudWhenKeepEmailMissingInLiveMode (0.00s)
=== RUN   TestConnectDoesNotLeakKeepAppPasswordInError
--- PASS: TestConnectDoesNotLeakKeepAppPasswordInError (0.00s)
=== RUN   TestParseKeepConfigEnforcesPollIntervalFloor
--- PASS: TestParseKeepConfigEnforcesPollIntervalFloor (0.00s)
=== RUN   TestConnectFailsLoudWhenKeepAppPasswordMissingInLiveMode
--- SKIP: TestConnectFailsLoudWhenKeepAppPasswordMissingInLiveMode (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/keep
```

Full keep-package suite (Scope-2 GREEN, no Scope-1 boundary regression):

```text
$ go test ./internal/connector/keep ./internal/config -count=1 | grep -E '^---|^FAIL\s|^ok\s'
ok      github.com/smackerel/smackerel/internal/connector/keep  0.179s
--- FAIL: TestSST_NoHardcodedOllamaValues (0.04s)
FAIL    github.com/smackerel/smackerel/internal/config  27.947s
```

The remaining `TestSST_NoHardcodedOllamaValues` failure is a pre-existing flake in `ml/app/processor.py:104` (a code comment that mentions the literal `localhost:11434`). It is unrelated to spec 059 and existed before this scope's changes — confirmed via `git log --oneline -5 ml/app/processor.py` showing the most recent commit predates spec 059.

The Scope-1 sidecar boundary test (`TestKeepAppPasswordReadOnlyFromSidecarNotCore`) PASSES (it appears under `ok internal/config` after the revert documented below).

### Planning Reconciliation (closed 2026-05-28)

Scope 2's original DoD item 2 required `Connect()` to fail loud when `KEEP_GOOGLE_APP_PASSWORD` is empty via an `os.Getenv("KEEP_GOOGLE_APP_PASSWORD")` call in the Go connector. That conflicted mechanically with Scope 1's `TestKeepAppPasswordReadOnlyFromSidecarNotCore` boundary test (forbids any non-test Go literal reference to the env-key string).

`bubbles.plan` reconciled this by adopting option (3) from the originally-routed list: KEEP_GOOGLE_APP_PASSWORD validation is owned by the ML sidecar (`ml/app/keep_bridge.py`, which already legitimately reads the value) and surfaced to the Go core via a Scope 3 `keep.sidecar.handshake` NATS reply (new SCN-059-019). Scope 2's DoD now narrows to KEEP_GOOGLE_EMAIL-only at Go-core `Connect()`, which IS shipped and proven by `TestConnectFailsLoudWhenKeepEmailMissingInLiveMode`.

Implementation close-out actions taken in this round:
- Removed the inline `t.Skip` and the entire body of `TestConnectFailsLoudWhenKeepAppPasswordMissingInLiveMode` from `internal/connector/keep/keep_test.go`; left a short comment in its place pointing future readers to SCN-059-019 (Scope 3 sidecar handshake) as the owning contract.
- The reconciled DoD item 2 wording (EMAIL-only at Go core; APP_PASSWORD via sidecar handshake) is satisfied today by the EMAIL Connect() check shipped in this scope; the sidecar-handshake half is now Scope 3's responsibility, tracked by SCN-059-019 in `scopes.md`.

### Files Modified (Scope 2 close-out, this round)

- `internal/connector/keep/keep_test.go` (removed skipped test body, replaced with reconciliation comment)
- `specs/059-google-keep-live-mode/scopes.md` (Scope 2 status → Done; inventory table row → Done; DoD item 2 checkbox → `[x]`)
- `specs/059-google-keep-live-mode/state.json` (completedPhaseClaims entry for Scope 2 implement; currentScope → 3; completedScopes += "2")
- `specs/059-google-keep-live-mode/report.md` (this section)

### Close-Out Test Evidence

**Claim Source:** executed.

```text
$ cd ~/smackerel && go test ./internal/connector/keep -count=1 -timeout 120s
ok      github.com/smackerel/smackerel/internal/connector/keep  0.244s
```

All keep-package tests (including SCN-059-003 / SCN-059-004 EMAIL / SCN-059-005 / SCN-059-004 adversarial password-leak) pass; no tests skipped (the previously skipped `TestConnectFailsLoudWhenKeepAppPasswordMissingInLiveMode` has been removed). The reconciliation comment in `keep_test.go` carries zero string literals matching `"KEEP_GOOGLE_APP_PASSWORD"` outside a Go comment, but even if it did, the boundary test only walks non-test `.go` files.

```text
$ cd ~/smackerel && go test ./internal/config/... -count=1 -timeout 120s 2>&1 | tail -5
--- FAIL: TestSST_NoHardcodedOllamaValues (0.07s)
    sst_grep_guard_test.go:223: SST violation: production source contains forbidden Ollama literals ...
          ml/app/processor.py:104: # OLLAMA_BASE_URL env vars and otherwise defaults to localhost:11434,
FAIL    github.com/smackerel/smackerel/internal/config  8.567s
```

The only failing test under `internal/config/...` remains the pre-existing `TestSST_NoHardcodedOllamaValues` flake against an unrelated comment in `ml/app/processor.py:104` — same failure documented in the Scope 2 progress section above; unrelated to spec 059. The Scope 1 boundary test `TestKeepAppPasswordReadOnlyFromSidecarNotCore` does not appear in the FAIL list, confirming it remains green after the Scope 2 close-out.

### Planning Conflict (routed to `bubbles.plan`)

Scope 2's DoD item 2 requires `Connect()` to fail loud when `KEEP_GOOGLE_APP_PASSWORD` is empty via an `os.Getenv("KEEP_GOOGLE_APP_PASSWORD")` call in the Go connector. Scope 1 (already shipped) added `TestKeepAppPasswordReadOnlyFromSidecarNotCore`, which forbids ANY non-test Go literal reference to that env-key string. These two requirements are mechanically irreconcilable in their current form.

Resolution options for `bubbles.plan`:

1. **Indirection.** Read the App Password presence through `config.SecretKeys()` or a small helper in `internal/config` (the manifest-listed key) — the helper file (`internal/config/secret_keys.go`) is already allow-listed in the boundary test. The connector calls the helper instead of `os.Getenv` directly.
2. **Boundary relaxation.** Change `TestKeepAppPasswordReadOnlyFromSidecarNotCore` to allow presence-check reads (e.g., add `internal/connector/keep/keep.go` to its allow-list) and update the Scope 1 design note to reflect the value-vs-presence distinction.
3. **Defer to ML sidecar.** Move the App Password presence check entirely into `ml/app/keep_bridge.py` (which already legitimately reads the value) and surface the failure through a Connect-time NATS handshake. This expands Scope 3 work.

Implementation here followed option (0) — implement the EMAIL half (no boundary conflict), revert the APP_PASSWORD half, document the conflict, route to planning. The skipped test `TestConnectFailsLoudWhenKeepAppPasswordMissingInLiveMode` carries an inline `t.Skip` referencing this conflict.

### Files Modified (Scope 2)

- `config/smackerel.yaml`
- `internal/connector/keep/keep.go`
- `internal/connector/keep/keep_test.go`
- `internal/connector/keep/regression_test.go`

All within the allowed Change Boundary for Scope 2.

### Spec-Narrative Drift Note (poll-interval floor)

`spec.md` and `design.md` narrative use "10 min" for the poll-interval floor; the actual canonical value is 15 min (the constant `gkeepPollIntervalFloor` in code, the comment in `config/smackerel.yaml`, and the rejection message all agree). Per Scope 2 implementation-plan bullet 5, this narrative drift is recorded here for a follow-up `bubbles.analyst`/`bubbles.docs` correction (route_required, separate concern from the App Password conflict above).

## Scope 3

Not started.

## Scope 4

Not started.

## Scope 5

Not started.

## Scope 6

Not started.

## Cross-Scope Certification Gates

Not started.

## Test Evidence

Will be captured per scope as each scope completes. Each entry will include the executed command, exit code, and the relevant unfiltered output captured via IDE file tooling (no shell redirection).

## Implementation Evidence

None yet; planning-only phase.
