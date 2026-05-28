# Bug: [BUG-020-010] QF-decisions callback keystore reads directly from `os.Getenv`, bypassing the Config SST single-ingestion-point

## Summary
`internal/connector/qfdecisions/callback_keystore.go` L137 reads the callback HMAC bridge signing keystore directly from the process environment via `os.Getenv(CallbackSigningKeysEnvVar)` — the only known connector that bypasses the Config SST single-ingestion-point pattern. Every other secret in the codebase (DB password, Telegram bot token, LLM API key, OAuth client secrets, QF decisions credential ref, etc.) is funneled through `internal/config.Config.Load()` and validated at boot through `Config.Validate()` / `validateQFDecisionsConfig()`. The QF-decisions callback signing key store is the lone exception.

This is a P2 SST/security finding from the latest code-review pass (SEC-2). The env var (`QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON`) is SST-managed in the sense that the deploy adapter is responsible for populating it, but the runtime ingestion path skips Config and is therefore invisible to `Config.Validate()`. A missing or malformed value is detected late in `Connect()` rather than at the single fail-loud `Validate()` choke point.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — P2 SST/security finding. Not exploitable on its own (the keystore is still parsed defensively in `LoadCallbackKeystoreFromJSON` and signing failures still emit the correct audit envelope), but it directly violates the SST regime that every other secret obeys and is the only known live exception. Every "small" SST exception is a load-bearing reason the regime exists in the first place.
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

## Reproduction Steps
1. Run `grep -nE 'os\.Getenv' internal/connector/qfdecisions/callback_keystore.go`.
2. Observe: `internal/connector/qfdecisions/callback_keystore.go:137:	raw := strings.TrimSpace(os.Getenv(CallbackSigningKeysEnvVar))`.
3. Run `grep -nE 'QFDecisionsCallbackSigningKeysJSON|QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON' internal/config/config.go` and observe ZERO matches — Config has no field for this value and `mustParseIntEnv`/`requiredVars`/`validateQFDecisionsConfig` cannot see it.
4. Compare with the surrounding QF-decisions config surface (`QFDecisionsEnabled`, `QFDecisionsBaseURL`, `QFDecisionsCredentialRef`, `QFDecisionsSyncSchedule`, `QFDecisionsPacketVersion`, `QFDecisionsPageSize`) — all six are Config fields populated in `Config.Load()` and validated in `validateQFDecisionsConfig()` when `QFDecisionsEnabled` is true. The signing-keys JSON is the sole exception.
5. Compare with the SST policy in `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement and `.github/instructions/smackerel-no-defaults.instructions.md`: "ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase. … Every other secret routes through Config."

## Expected Behavior
- The callback signing keystore JSON MUST be ingested through `internal/config.Config` (new field `QFDecisionsCallbackSigningKeysJSON string`) and populated by `Config.Load()` from the env var (same shape as the other `QFDecisions*` string fields).
- `validateQFDecisionsConfig()` MUST validate the field at boot when `QFDecisionsEnabled` is true: a missing value MAY be permitted (matches today's "callback signing not configured in this environment" semantics — see the L119-L128 comment block) OR MAY be required, but a non-empty malformed value MUST fail loud at `Validate()` time with a clear error naming the field, instead of failing later inside `Connect()`.
- The keystore loader MUST accept the JSON string via an explicit parameter and MUST NOT read the process environment. The existing `LoadCallbackKeystoreFromJSON(raw string)` parser is already the correct shape; the env-reading wrapper `LoadCallbackKeystoreFromEnv()` MUST be replaced by a config-reading variant (e.g., `LoadCallbackKeystoreFromConfig(cfg *config.Config)` or pass the string directly into the connector wiring in `Connect()`).
- The literal `os.Getenv(CallbackSigningKeysEnvVar)` MUST be eradicated from the keystore source. The `CallbackSigningKeysEnvVar` constant MAY survive as documentation (mapped to the Config field via `requiredVars`-style registration) OR MAY be relocated to `internal/config` where it lives next to the other env-var names.

## Actual Behavior
- `LoadCallbackKeystoreFromEnv()` reads `os.Getenv(CallbackSigningKeysEnvVar)` directly at L137.
- `Connect()` in `internal/connector/qfdecisions/connector.go` L385 calls `LoadCallbackKeystoreFromEnv()` and surfaces malformed-input errors at that point (rather than at `Validate()` boot time).
- `Config.Validate()` has no awareness of the keystore configuration; a typo in the env-var name in the deploy adapter would not be caught at startup by the consolidated fail-loud `Validate()` surface.

## Environment
- Repo: `smackerel` @ current `main`
- Affected source: `internal/connector/qfdecisions/callback_keystore.go` (L137 — the `os.Getenv` call; also the surrounding `LoadCallbackKeystoreFromEnv` wrapper L119-L142 and the `CallbackSigningKeysEnvVar` constant at L32); `internal/connector/qfdecisions/connector.go` (L385 — the sole non-test caller); `internal/config/config.go` (add field + Load + Validate)
- Affected tests: `internal/connector/qfdecisions/callback_keystore_test.go` (the existing `TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet` adversarial test will need to be re-pointed at the new Config-driven API); `tests/integration/qf_callback_signing_test.go`, `tests/integration/qf_watch_proposal_test.go` (3 call sites that exercise the existing env wrapper — must move to the new API)
- Authoritative policy: `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement; `.github/instructions/smackerel-no-defaults.instructions.md`; `.github/skills/smackerel-no-defaults/SKILL.md`
- Spec association: `specs/020-security-hardening/` (SST regime owner)
- Code-review finding: SEC-2 (P2 SST/security)

## Error Output
```
$ grep -nE 'os\.Getenv' internal/connector/qfdecisions/callback_keystore.go
internal/connector/qfdecisions/callback_keystore.go:137:	raw := strings.TrimSpace(os.Getenv(CallbackSigningKeysEnvVar))

$ grep -nE 'QFDecisionsCallbackSigningKeysJSON|QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON' internal/config/config.go
(zero matches — the env var is invisible to Config)

$ grep -nE 'os\.Getenv\(' internal/connector/qfdecisions/ -r
internal/connector/qfdecisions/callback_keystore.go:137:	raw := strings.TrimSpace(os.Getenv(CallbackSigningKeysEnvVar))
(this is the ONLY os.Getenv read in the entire qf-decisions connector package)
```

## Root Cause (filled after analysis)
See `design.md`. Initial hypothesis: when Scope 8 of spec 041 (QF callback signing) landed, the keystore loader was authored to be self-contained (env-read inside the qfdecisions package) so the connector could load its own secret without an extra plumbing dependency on `internal/config`. The pattern was correct for an MVP-stage delivery but is incompatible with the SST single-ingestion-point regime enforced by spec 020 (NO-DEFAULTS, fail-loud `Validate()`). Every other `QFDecisions*` field on `Config` was added in the same window and routed through the canonical path; the signing-keys JSON was simply missed.

## Related
- Feature: `specs/020-security-hardening/` (SST regime owner)
- Originating feature: `specs/041-qf-companion-connector/` Scope 8 (SCN-SM-041-028 — HMAC bridge callback signer)
- Sibling SST bugs in spec 020 (canonical fail-loud / SST plumbing patterns):
  - `BUG-020-002-ml-auth-token-module-import-fail-loud` (fail-loud at module init)
  - `BUG-020-004-ml-nats-client-auth-token-fail-loud-read` (fail-loud env read)
  - `BUG-020-008-parseintenv-silent-defaults` (canonical `mustParseIntEnv` + `intLoadErrs` pattern)
  - `BUG-020-009-hardcoded-http-timeouts` (canonical config-thread-through-the-call-site migration)
- Policy: `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement; `.github/instructions/smackerel-no-defaults.instructions.md`
- Code-review finding: SEC-2 (P2 SST/security)
