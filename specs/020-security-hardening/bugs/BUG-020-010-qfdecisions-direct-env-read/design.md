# Bug Fix Design: [BUG-020-010] QF-decisions callback keystore reads directly from `os.Getenv`

> **STATUS:** Initial design authored by `bubbles.bug` during Phase 2/3 documentation. The implementing agent (`bubbles.design` via `runSubagent` in Phase 3) MUST review, finalize the exact field name and the permissive-vs-strict validation policy, and refine before code edits begin.

## Root Cause Analysis

### Investigation Summary
Code-review finding SEC-2 (P2 SST/security) surfaced one site that bypasses the Config SST single-ingestion-point pattern:

```go
// internal/connector/qfdecisions/callback_keystore.go L137
func LoadCallbackKeystoreFromEnv() (*CallbackKeystore, error) {
    raw := strings.TrimSpace(os.Getenv(CallbackSigningKeysEnvVar))
    if raw == "" {
        return nil, nil
    }
    return LoadCallbackKeystoreFromJSON(raw)
}
```

`CallbackSigningKeysEnvVar = "QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON"` (L32). This is the only `os.Getenv` read in the entire `internal/connector/qfdecisions/` package, and the only known production connector path that bypasses Config.

Confirmation that this is the lone exception:
- `grep -nE 'os\.Getenv' internal/connector/qfdecisions/` returns exactly one match (L137 above).
- The other QF-decisions config values — `QFDecisionsEnabled`, `QFDecisionsBaseURL`, `QFDecisionsCredentialRef`, `QFDecisionsSyncSchedule`, `QFDecisionsPacketVersion`, `QFDecisionsPageSize` — are all Config fields populated at `internal/config/config.go` L588-L591 (string fields via `os.Getenv("QF_DECISIONS_*")`) and L679-L680 (int fields via `mustParseIntEnv`), and all are validated inside `validateQFDecisionsConfig()` at L1766.

### Root Cause
Sin of omission, not commission. When Scope 8 of spec 041 (QF callback HMAC bridge signer) landed, the keystore loader was authored to be self-contained inside the `qfdecisions` package so the connector could ingest its own secret without depending on `internal/config`. The pattern was reasonable for an MVP-stage delivery (and the parser `LoadCallbackKeystoreFromJSON` is already defensive — it rejects empty/malformed/duplicate inputs), but it is incompatible with the SST single-ingestion-point regime hardened by spec 020 and BUG-020-008/009.

The result: a malformed env var is detected at `Connect()` time (L385-L394 of `connector.go`) instead of at the consolidated fail-loud `Validate()` choke point. An operator who typos the env-var name in the deploy adapter would not learn about it until the QF connector tried to start, rather than at boot from the single `Config.Validate()` surface.

### Impact Analysis
- **Affected components:** `internal/connector/qfdecisions/callback_keystore.go`, `internal/connector/qfdecisions/connector.go`, `internal/config/config.go`.
- **Affected runtime:** any deployment with `QF_DECISIONS_ENABLED=true`. A misconfigured env var is detected one step later than the rest of the QF-decisions config surface — diagnostically inconsistent.
- **Affected tests:** the in-tree adversarial test `TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet`, plus 3 integration-test call sites (`qf_callback_signing_test.go` L76, `qf_watch_proposal_test.go` L91/L198) all exercise `LoadCallbackKeystoreFromEnv()` directly. They must be migrated to the new API.
- **Affected operators:** none in steady-state — the env-var name is unchanged; only the runtime ingestion path moves into Config.
- **Affected users:** none.

## Fix Design

### Solution Approach
Three coordinated edits, mirroring the canonical pattern of the surrounding `QFDecisions*` Config fields and the BUG-020-009 thread-through-the-call-site migration:

1. **Decide exact Config field name + validation policy** (BLOCKING design step BEFORE code edits). The implementing agent MUST:
   - Choose the Config field name. Recommended: `QFDecisionsCallbackSigningKeysJSON string` to match the existing `QFDecisions*` PascalCase naming and to make the JSON-string shape explicit.
   - Decide PERMISSIVE vs STRICT validation. Recommended: PERMISSIVE (empty allowed → no keystore, matches today's semantics; non-empty must parse). Document the choice and rationale in this design.md before code edits.
   - Decide whether the `CallbackSigningKeysEnvVar` string constant stays in `callback_keystore.go` (as a documentation reference) or moves to `internal/config` next to the other env-var name strings. Recommended: keep it in `callback_keystore.go` as the canonical name reference, but the package no longer READS it — it only declares it.

2. **Add the Config field and wire it through Load + Validate.**
   - Add `QFDecisionsCallbackSigningKeysJSON string` to the `Config` struct next to the existing `QFDecisions*` fields (around L237-L242 in `internal/config/config.go`).
   - Populate it in `Config.Load()` near L588-L591 via `os.Getenv("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON")` (reuse the existing env-var name — no breaking change for operators or the deploy adapter).
   - Add a validation block in `validateQFDecisionsConfig()` (around L1766+): when non-empty, call `qfdecisions.LoadCallbackKeystoreFromJSON(cfg.QFDecisionsCallbackSigningKeysJSON)` and surface any parse error as a `Validate()` error naming the field. (If circular-import risk between `internal/config` and `internal/connector/qfdecisions` arises, hoist the JSON-schema validation into a local helper in `internal/config` that mirrors the structural checks — but the preferred path is the direct call.)

3. **Replace `LoadCallbackKeystoreFromEnv` with a Config-consuming variant.**
   - Either (a) delete `LoadCallbackKeystoreFromEnv` and call `LoadCallbackKeystoreFromJSON(cfg.QFDecisionsCallbackSigningKeysJSON)` directly at the single call site in `connector.go` L385, or (b) introduce `LoadCallbackKeystoreFromConfig(cfg *config.Config) (*CallbackKeystore, error)` as a thin wrapper that internally calls `LoadCallbackKeystoreFromJSON(cfg.QFDecisionsCallbackSigningKeysJSON)`. Option (a) is simpler and stricter — preferred unless dependency direction forbids it.
   - Plumb `*config.Config` (or just the resolved JSON string) into `Connect()`. The QF-decisions connector already receives a Config; the implementing agent MUST confirm the exact plumbing path during the design step.

4. **Migrate existing tests.**
   - `internal/connector/qfdecisions/callback_keystore_test.go` `TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet` — re-point at the new API. The empty/valid/malformed cases must keep their semantics.
   - `tests/integration/qf_callback_signing_test.go` L76 and `tests/integration/qf_watch_proposal_test.go` L91, L198 — same migration. These tests currently `t.Setenv(qfdecisions.CallbackSigningKeysEnvVar, ...)` then call `LoadCallbackKeystoreFromEnv()`. After the migration, they should either set the field on a test Config and call `LoadCallbackKeystoreFromConfig(cfg)`, or call `LoadCallbackKeystoreFromJSON(rawJSON)` directly.

The implementing agent MUST NOT take shortcuts:
- DO NOT keep `LoadCallbackKeystoreFromEnv()` as a "legacy wrapper" that still calls `os.Getenv`. That re-introduces the bug behind a fig leaf. Remove the env read.
- DO NOT add a silent fallback like `if env := os.Getenv(...); env != "" { return env } else { return cfg.X }`. The Config field is the sole runtime source after the fix.
- DO NOT skip the `Validate()` integration. The boot-time fail-loud check is the actual SST policy fix; without it, the migration is cosmetic.
- DO NOT change the env-var name. The deploy adapter already populates `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON`; renaming it would be an unrelated operator-facing break.
- DO NOT collapse `LoadCallbackKeystoreFromJSON` into the Config validation — keep parse logic in the qfdecisions package; the Config validator merely delegates to it. That respects layering and keeps the parser's existing test coverage in place.

### Affected Files
| File | Change |
|------|--------|
| `internal/config/config.go` | Add 1 string field (`QFDecisionsCallbackSigningKeysJSON`); 1 `os.Getenv` population in `Load()`; 1 validation block in `validateQFDecisionsConfig()` that delegates to `qfdecisions.LoadCallbackKeystoreFromJSON` (or equivalent structural check) when non-empty |
| `internal/connector/qfdecisions/callback_keystore.go` | Delete `LoadCallbackKeystoreFromEnv()` (L119-L142) and the `os.Getenv` read; remove the unused `os` import if it's no longer referenced; optionally add `LoadCallbackKeystoreFromConfig(cfg *config.Config)` as a Config-consuming wrapper |
| `internal/connector/qfdecisions/connector.go` | Update L385 call site to consume the resolved Config field |
| `internal/connector/qfdecisions/callback_keystore_test.go` | Migrate `TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet` to the new API |
| `tests/integration/qf_callback_signing_test.go` | Migrate L76 call site |
| `tests/integration/qf_watch_proposal_test.go` | Migrate L91 and L198 call sites |
| NEW: `internal/connector/qfdecisions/bug020010_config_ingestion_test.go` (or extension of existing keystore test file) | Add the 3 BUG020010 regression tests defined in spec.md EB-6 |

### Alternative Approaches Considered
1. **Keep the env-read inside the qfdecisions package; only add a duplicate `Validate()` check that ALSO reads `os.Getenv`**. Rejected — double-read of the env var is a recipe for skew, and the policy violation (a non-Config secret ingestion point) is unfixed.
2. **Move ALL keystore parsing into `internal/config`**. Rejected — would force `internal/config` to import the qfdecisions domain types (`CallbackSigningKey`, `CallbackKeystore`, time gating) or duplicate them. Keep parsing in the qfdecisions package; let Config delegate.
3. **Introduce a generic "SecretLoader" interface in `internal/config` and refactor every secret through it**. Rejected — scope creep. The bug is specifically the one missed ingestion point; the canonical pattern (a typed Config field populated in `Load()` and validated in the per-feature `validateXxxConfig()` block) is well-established. Use it.
4. **Make the env var REQUIRED when `QFDecisionsEnabled=true`**. Documented as the STRICT alternative in spec.md EB-3 and explicitly left to the implementing agent + reviewer. PERMISSIVE is the recommended default because it preserves the existing "callback signing not configured in this environment" deployment shape that Scope 8 of spec 041 explicitly designed for.

### Regression Test Design
Mirror BUG-020-009's pattern. Three new tests (named per spec.md EB-6) + migration of the four existing call sites.

1. **`TestBUG020010_KeystoreReadsFromConfigNotEnv`** — `t.Setenv` clears the env var (or use `os.Unsetenv`); construct a Config with `QFDecisionsCallbackSigningKeysJSON = "<valid 1-key JSON>"`; call the new `LoadCallbackKeystoreFromConfig(cfg)` (or the direct `LoadCallbackKeystoreFromJSON(cfg.QFDecisionsCallbackSigningKeysJSON)` path); assert keystore is non-nil and `keystore.KeyIDs()` contains the expected key. ADVERSARIAL: if the keystore source were still `os.Getenv`, the empty env would yield a nil keystore and the assertion would fail.

2. **`TestBUG020010_ValidateFailsLoudOnMalformedSigningKeysJSON`** — `cfg.QFDecisionsEnabled = true`; set OTHER required QF-decisions Config fields to valid values; set `cfg.QFDecisionsCallbackSigningKeysJSON = "not-valid-json"`; call `cfg.Validate()`; assert the returned error is non-nil AND its message contains either the field name or the env-var name AND surfaces the underlying parse error from `LoadCallbackKeystoreFromJSON`.

3. **`TestBUG020010_KeystoreEnvVarLiteralRemoved`** — `exec.Command("grep", "-nE", "os\\.Getenv", "internal/connector/qfdecisions/callback_keystore.go")`; assert exit code 1 (no matches). Same shape as the helper-eradication grep test from BUG-020-008/BUG-020-009. Permanent structural guard.

**Adversarial requirement (NON-NEGOTIABLE):** Test #1 MUST unset the env var (`os.Unsetenv` or `t.Setenv("", "")`) BEFORE constructing the Config so that any residual env-read path is exposed. Test #3 makes regression impossible at the literal-pattern level.

**Pre-fix evidence:** All three tests above MUST be authored AND executed BEFORE the migration. They MUST FAIL against `main`:
- Test #1 fails because `cfg.QFDecisionsCallbackSigningKeysJSON` does not exist (BUILD failure) AND `LoadCallbackKeystoreFromConfig` does not exist (BUILD failure) — strongest possible RED.
- Test #2 fails for the same Config-field-undefined reason — BUILD failure RED.
- Test #3 fails because `grep` finds the L137 `os.Getenv` match.

### Rollback Plan
Revert the changes in `internal/config/config.go`, `internal/connector/qfdecisions/callback_keystore.go`, `internal/connector/qfdecisions/connector.go`, and the test migrations. The env-var name is unchanged across the change boundary, so the deploy adapter requires no rollback. No data migration; no schema change; no SST yaml change (the env var is already SST-managed by the deploy adapter, not by `config/smackerel.yaml`).
