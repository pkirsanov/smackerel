# Spec: [BUG-020-010] QF-decisions callback keystore MUST be ingested via Config SST

## Expected Behavior

### EB-1: Direct `os.Getenv` read eradicated from the keystore source
After the fix, `internal/connector/qfdecisions/callback_keystore.go` MUST contain zero `os.Getenv` calls. The `LoadCallbackKeystoreFromEnv()` wrapper (L119-L142) MUST be either removed or rewritten to take its input via an explicit parameter (e.g., `LoadCallbackKeystoreFromConfig(cfg *config.Config) (*CallbackKeystore, error)`) so the package no longer touches the process environment.

### EB-2: New SST Config field carries the JSON string
A new field MUST be added to `internal/config.Config`:

| Suggested env var | Suggested Config field | Suggested type |
|-------------------|------------------------|----------------|
| `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON` (existing — reuse) | `QFDecisionsCallbackSigningKeysJSON` | `string` |

The implementing agent MAY refine the exact field name to match neighboring `QFDecisions*` Go naming conventions, provided EB-1, EB-3, EB-4, and EB-5 are all satisfied. The chosen names MUST be documented in `design.md` BEFORE code edits begin.

`Config.Load()` MUST populate the new field from the existing env var via the same pattern used by the other `QFDecisions*` string fields at L588-L591 (`os.Getenv("QF_DECISIONS_*")`). This preserves the existing single deploy-adapter contract (the deploy adapter already sets this env var today; no operator-facing change).

### EB-3: Boot-time validation via the existing fail-loud path
`validateQFDecisionsConfig()` (currently at `internal/config/config.go` L1766) MUST validate the new field whenever `QFDecisionsEnabled` is true. Validation rules:

- **Permissive default (RECOMMENDED — matches today's semantics):** Empty/unset is permitted (means "callback signing not configured in this environment" — preserves the L119-L128 comment-block invariant in `callback_keystore.go`). Non-empty MUST be parseable by `LoadCallbackKeystoreFromJSON`; a malformed non-empty value MUST produce a `Validate()` error naming the field.
- **Strict alternative:** If the implementing agent and reviewer decide that callback signing MUST always be configured when `QFDecisionsEnabled` is true, the field becomes required and an empty value also fails `Validate()`. This decision MUST be documented in `design.md` with the rationale.

Either way, the malformed-non-empty case MUST be caught at `Validate()` time, not deferred to `Connect()`.

### EB-4: `Connect()` consumes the resolved Config field
The single non-test call site at `internal/connector/qfdecisions/connector.go` L385 MUST be updated to consume the resolved Config field (either by calling the new `LoadCallbackKeystoreFromConfig(cfg)` or by passing `cfg.QFDecisionsCallbackSigningKeysJSON` into the existing `LoadCallbackKeystoreFromJSON(raw)`). The `Connect()` method MUST receive the Config (or its relevant field) via dependency injection — `Connect()` MUST NOT call `os.Getenv` or read the env-var name directly.

### EB-5: Existing tests are migrated, not deleted
The existing adversarial test `TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet` at `internal/connector/qfdecisions/callback_keystore_test.go:125` and the 3 integration-test call sites in `tests/integration/qf_callback_signing_test.go` (L76) and `tests/integration/qf_watch_proposal_test.go` (L91, L198) MUST be migrated to the new Config-driven API. The semantics they encode (empty→nil keystore, valid→keystore, malformed→error) MUST be preserved in the new test surface.

### EB-6: New regression tests assert Config integration
The fix MUST add at least three NEW regression tests:

1. `TestBUG020010_KeystoreReadsFromConfigNotEnv` — constructs the connector with `cfg.QFDecisionsCallbackSigningKeysJSON = "<valid JSON>"` while `os.Unsetenv("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON")` ensures the env is empty. Asserts the resolved keystore is non-nil and contains the expected key IDs. Adversarial: if the keystore were still reading from `os.Getenv`, the empty env would yield a nil keystore and the assertion would fail.
2. `TestBUG020010_ValidateFailsLoudOnMalformedSigningKeysJSON` — sets `cfg.QFDecisionsEnabled = true` and `cfg.QFDecisionsCallbackSigningKeysJSON = "not-valid-json"`, calls `cfg.Validate()`, asserts the returned error names the `QFDecisionsCallbackSigningKeysJSON` field (or its env-var name) and surfaces the underlying parse error.
3. `TestBUG020010_KeystoreEnvVarLiteralRemoved` — guard test: `grep -nE 'os\.Getenv' internal/connector/qfdecisions/callback_keystore.go` exits with code 1 (no matches). Encoded as a `t.Run` that shells out via `exec.Command` (same pattern as the helper-eradication grep tests already established by BUG-020-008 and BUG-020-009).

## Acceptance Criteria
1. `grep -nE 'os\.Getenv' internal/connector/qfdecisions/callback_keystore.go` returns zero matches (exit 1).
2. `grep -nE 'QFDecisionsCallbackSigningKeysJSON' internal/config/config.go` returns at least: 1 field declaration + 1 `Load()` population + 1 `validateQFDecisionsConfig` guard.
3. The three new tests above are present, named with the `BUG020010` prefix, and PASS post-fix.
4. The three new tests (or at least #1 and #2) MUST FAIL against the pre-fix tree — either as build failures (the new Config field does not exist) or as runtime assertion failures (the keystore reads from env, not Config). Build-failure RED is acceptable and is the strongest form.
5. The existing test `TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet` is migrated (not deleted) to the new API and PASSES.
6. The 3 integration-test call sites in `tests/integration/qf_callback_signing_test.go` and `tests/integration/qf_watch_proposal_test.go` are migrated and PASS.
7. `./smackerel.sh build` passes after the change.
8. `./smackerel.sh test unit` passes after the change (zero collateral regressions).
9. `go test -tags=integration ./tests/integration/ -run TestQFCallback -count=1` passes after the change (zero collateral regressions in the existing QF callback signing integration suite).
10. `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-010-qfdecisions-direct-env-read` passes.
