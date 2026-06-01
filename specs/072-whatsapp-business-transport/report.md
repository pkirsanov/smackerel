# Report: 072 WhatsApp Business Webhook Adapter

## Summary

Planning-only scaffold created by `bubbles.plan` for the WhatsApp Business transport adapter. The packet defines four sequential scopes, a scenario manifest, and a structured test plan covering SCN-072-A01 through SCN-072-A10.

## Scope Inventory

| Scope | Name | Status |
|---|---|---|
| SCOPE-072-01 | Webhook, Identity, And Fail-Loud Config Foundation | Not Started |
| SCOPE-072-02 | Response Renderer And Capture Acknowledgement | Not Started |
| SCOPE-072-03 | Round-Trip Controls And Idempotent Retries | Not Started |
| SCOPE-072-04 | Independent Disable And Operator Status | Not Started |

## Test Evidence

No implementation, build, lint, runtime, or test evidence is recorded in this planning scaffold. Evidence must be added only after commands execute in the current session and raw output is available.

### bubbles.validate session 2026-06-01 — BLOCKED on repo-wide config-generate gap (NOT spec 072)

**Claim Source:** executed
**Outcome:** Attempted to run TP-072-01/03/05/08/10 against the disposable test stack. The runs could not begin because `./smackerel.sh config generate` (invoked by `./smackerel.sh test integration` and `./smackerel.sh test e2e`) fails for both `dev` and `test` environments with a `[F074-SST-MISSING]` validator error. Spec 072 source/tests are not implicated; the blocker is in the SST→env generator wiring owned by spec 074 (capture-as-fallback policy).

Raw execution:

```
$ ./smackerel.sh config generate
ERROR: [F074-SST-MISSING] missing or invalid required capture_as_fallback configuration: CAPTURE_AS_FALLBACK_DEDUP_WINDOW (env var not set), CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT (env var not set), CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY (env var not set), CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY (env var not set), CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS (env var not set)
exit status 1
ERROR: config-generate-time validation failed for env=dev (see above)

$ ./smackerel.sh config generate --env test
ERROR: [F074-SST-MISSING] missing or invalid required capture_as_fallback configuration: CAPTURE_AS_FALLBACK_DEDUP_WINDOW (env var not set), CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT (env var not set), CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY (env var not set), CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY (env var not set), CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS (env var not set)
exit status 1
ERROR: config-generate-time validation failed for env=test (see above)
```

**Root cause:**
- `config/smackerel.yaml` declares the `capture_as_fallback:` block (lines 933–938) as REQUIRED for spec 074.
- `internal/config/capture_fallback.go` loads `CAPTURE_AS_FALLBACK_*` env vars and emits `[F074-SST-MISSING]` when any are missing; this is invoked by `cmd/config-validate` against the temp env file.
- `scripts/commands/config.sh` (the yaml→env flattener) contains **zero** references to `capture_as_fallback` / `CAPTURE_AS_FALLBACK_*`. The temp env file `config/generated/dev.env.tmp.*` does not contain any `CAPTURE_AS_FALLBACK_*` lines.
- The existing `config/generated/test.env` on disk DOES contain the keys (lines 513–517), but the validator runs against the freshly-generated temp file, so the stale-but-valid disk file does not help.

**Impact:** Every `./smackerel.sh test integration`, `./smackerel.sh test e2e`, `./smackerel.sh check` (which requires up-to-date generated env), and any other path through `smackerel_generate_config` is hard-blocked. This is repo-wide and predates this validate run.

**Spec 072 build verification (executed; non-test-stack):**

```
$ go build ./... ; echo "BUILD_EXIT=$?"
BUILD_EXIT=0
$ go vet ./...  (first 60 lines clean)
VET_EXIT=0
```

Build/vet pass against the in-repo Go sources, confirming spec 072's newly-authored test files (`tests/integration/assistant/whatsapp_webhook_test.go`, `transport_identity_test.go`, `whatsapp_capture_test.go`, `tests/e2e/assistant/whatsapp_signature_e2e_test.go`, `whatsapp_render_e2e_test.go`) compile against the current adapter surface. Their `^func Test...` symbols are confirmed present:

- `TestWhatsAppWebhook_TP_072_01_SignedTextBecomesCanonicalMessage`
- `TestTransportIdentity_TP_072_03_PhoneHashResolvesWithoutRawPhone`
- `TestWhatsAppCapture_TP_072_08_CaptureRouteInvokesCaptureOnce`
- `TestWhatsAppSignatureE2E_TP_072_05_UnsignedNeverReachesFacade`
- `TestWhatsAppRenderE2E_TP_072_10_DisambiguationRendersAsButtons`

**No DoD items flipped.** Per anti-fabrication policy, tests must execute against the disposable test stack before TP-072-01/03/05/08/10 DoD items can be checked. Build-pass alone is insufficient evidence for live-stack `integration`/`e2e-api` DoD claims.

**Routed follow-up:** Fix the `[F074-SST-MISSING]` config-generate gap (owner: spec 074 implementation) before this validate pass can resume. Once `./smackerel.sh config generate --env test` returns RC=0, re-invoke `bubbles.validate` on this scope to execute the five live-stack tests and update DoD.

## Completion Statement

Status remains `in_progress` with `planMaturityOnly=true`. No terminal status or scope completion is claimed by this planning pass.

## Uncertainty Declarations

- Scope DoD items are unchecked because implementation and validation have not executed for this feature.
- Planned test file paths and titles are handoff targets for implementation/test agents; they are not claims that tests already exist.

## Validation Summary

Artifact lint is captured in the invoking agent result envelope for this planning pass.

---

## bubbles.validate session 2026-06-01 (2) — BLOCKED on F061-SST-MISSING regression in WhatsApp validator

**Claim Source:** executed
**Outcome:** `route_required` — implementation regression. Spec 072 source files are now in place and compile, `./smackerel.sh check` is green, and `./smackerel.sh config generate` succeeds. However, `./smackerel.sh test unit` fails with 86 `--- FAIL` records caused by the new WhatsApp SST validator: it requires every `ASSISTANT_TRANSPORTS_WHATSAPP_*` key unconditionally, including when `ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED` is unset or false. This contradicts the spec 072 SCN-072-A06 intent ("enabled with missing access token fails loud") and breaks shared test fixtures across `internal/config`, `internal/deploy`, and `tests/unit/clients` that build assistant config in-memory without the new keys.

### Build Quality Gate (executed)

| Command | Exit | File |
|---|---|---|
| `./smackerel.sh check` | 0 | /tmp/v072-check.log |
| `./smackerel.sh format --check` | 1 (unrelated drift) | /tmp/v072-format.log |
| `./smackerel.sh test unit` | 1 | /tmp/v072-unit.log |
| `bash .github/bubbles/scripts/artifact-lint.sh specs/072-whatsapp-business-transport` | 0 | /tmp/v072-al.log |

### Format-check drift (NOT 072)

`./smackerel.sh format --check` reports 3 unformatted files, none owned by spec 072:

```
internal/agent/tools/microtools/unit_convert.go
internal/assistant/httpadapter/adapter.go
internal/assistant/httpadapter/schema.go
```

`gofmt -l` against only spec 072 files (`internal/whatsapp/**`, `internal/config/assistant_whatsapp_test.go`, `tests/integration/assistant/whatsapp_*.go`, `tests/integration/assistant/transport_*.go`, `tests/e2e/assistant/whatsapp_*.go`, `tests/integration/monitoring/whatsapp_transport_status_test.go`) → EXIT=0, no output. These drifts belong to other in-flight specs (069 HTTP transport surface) and are out-of-scope for 072 certification, but they DO block the repo-wide Build Quality Gate.

### Unit test results

```
$ ./smackerel.sh test unit
...
ok  	github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter  0.052s
FAIL	github.com/smackerel/smackerel/internal/config        13.048s
FAIL	github.com/smackerel/smackerel/internal/deploy        21.982s
FAIL	github.com/smackerel/smackerel/tests/unit/clients      0.007s
FAIL
EXIT=1
```

Total `--- FAIL` records: 86.

**Per-TP outcome:**

| TP | Scenario | Cat | Result | Notes |
|---|---|---|---|---|
| TP-072-02 | SCN-072-A02 | unit | ✅ PASS | `internal/whatsapp/assistant_adapter` package green (0.052s) |
| TP-072-04 | SCN-072-A06 | unit | ❌ FAIL | New WhatsApp validator gates every key on its own existence rather than on `WHATSAPP_ENABLED=true`; sub-cases like `TestAssistantHTTPTransportConfigRequiresEverySSTKey/enabled_missing` see WhatsApp errors first |
| TP-072-06 | SCN-072-A03 | unit | ✅ PASS | render golden tests in `internal/whatsapp/assistant_adapter` |
| TP-072-07 | SCN-072-A04 | unit | ✅ PASS | render fallback golden tests in `internal/whatsapp/assistant_adapter` |
| TP-072-09 | SCN-072-A09 | unit | ✅ PASS | template policy test in `internal/whatsapp/assistant_adapter` |
| TP-072-01,03,05,08,10,11,12,13,14 | live | int/e2e | ⚪ NOT RUN | Unit gate failure already blocks Build Quality Gate; not run to avoid 30+ min on a known-blocked path |

**Sample failing assertion (from `/tmp/v072-unit.log`):**

```
assistant_http_transport_test.go:114: error must contain "ASSISTANT_TRANSPORTS_HTTP_ENABLED"; got: [F061-SST-MISSING] missing or invalid required assistant configuration:
  ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED, ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH,
  ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID, ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID,
  ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF, ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF,
  ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF, ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF,
  ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL, ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION,
  ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE, ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS
```

The HTTP-transport test was checking that with HTTP enabled and one HTTP key missing the error names that HTTP key; instead it received WhatsApp's complaint because WhatsApp keys are now demanded before HTTP cross-field checks even run.

**Affected failing test groups (sample, not exhaustive):**

- `TestAssistantHTTPTransportConfig*` (HTTP transport contract regression — was passing, broken by WhatsApp validator order)
- `TestLoadAssistantConfig_*` (Weather, Otel happy-path / permissive-but-required)
- `TestBUG020008/009/010_*` (regression suite for prior assistant SST bugs)
- `TestRuntimeConfig_S004_*` (auth-token warnings)
- `TestDriveConfig*` (Drive OAuth secret loading)
- `TestErrorPaths_NeverEcho*` (signature key / bootstrap token error redaction)
- `TestBundleSecretContract_AdversarialA[1-4]_*` and `TestBundleSecretContract_NoLiteralSecretsInHomeLab` in `internal/deploy` (bundle generation regression)
- `tests/unit/clients` (entire package)

**Note on test.env:** SST-generated `config/generated/test.env` DOES contain all 12 `ASSISTANT_TRANSPORTS_WHATSAPP_*` keys with `ENABLED=false` plus placeholder refs. The live test stack would therefore start. The regression is strictly in-process validators invoked by Go unit tests that construct assistant config from custom env maps and do not (and should not have to) include the new WhatsApp keys.

### Artifact lint

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/072-whatsapp-business-transport
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
... (25 ✅ lines, 0 failures)
✅ state.json planMaturityOnly=true is not claiming delivery-done status
```

### DoD disposition

No DoD items flipped. All scope DoD entries remain `[ ]`. Per anti-fabrication policy, only `TP-072-02`, `TP-072-06`, `TP-072-07`, `TP-072-09` (the four unit-only rows scoped to `internal/whatsapp/assistant_adapter`) currently have executed-pass evidence; their DoD items still require the broader SCN gates and a clean Build Quality Gate before they may be checked.

### Routing

- **Owner:** `bubbles.implement` (or `bubbles.bug` if the project elects to file a regression).
- **Required fix (technical, not prescriptive):** make `ASSISTANT_TRANSPORTS_WHATSAPP_*` SST keys conditionally required on `ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED=true`, mirroring the spec 072 SCN-072-A06 intent and matching the established pattern of other transports. This restores the in-memory contract used by `internal/config`, `internal/deploy`, and `tests/unit/clients` test fixtures without requiring those packages to know about WhatsApp.
- **After fix:** re-run `./smackerel.sh test unit`, then this validate pass for the 14 TP-072 rows (5 unit, 6 integration, 5 e2e-api) plus repeat Build Quality Gate.
- **Also referenced (separate owner):** the 3 `format --check` failures in `internal/agent/tools/microtools/unit_convert.go` and `internal/assistant/httpadapter/{adapter,schema}.go` belong to other in-flight spec work (likely 069 / micro-tools); they block the repo-wide Build Quality Gate and must be cleared before 072 can certify.

### Status

Status remains `in_progress`. `certification.status` remains `in_progress`. `planMaturityOnly` remains `true` until implementation regression is resolved.
