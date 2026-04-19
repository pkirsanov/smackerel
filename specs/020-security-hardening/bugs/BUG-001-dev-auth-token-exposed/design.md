# Bug Fix Design: [BUG-001] Dev Auth Token Exposed as Functional Default

## Design Brief

- **Current State:** `config/smackerel.yaml` commits `auth_token: "dev-token-smackerel-2026"`. `internal/config/config.go` `Validate()` rejects 6 generic placeholders and enforces ≥16 chars, but the committed default passes both checks.
- **Target State:** `config/smackerel.yaml` uses `auth_token: ""` (empty string). `Validate()` rejects the literal `dev-token-smackerel-2026` and any `dev-token-*` prefix to prevent future variants.
- **Patterns to Follow:** Existing placeholder reject list in `config.go` lines 596–603 uses `strings.EqualFold` for case-insensitive comparison. The SST secrets management pattern uses empty-string placeholders.
- **Patterns to Avoid:** Do not add a regex engine for a simple prefix check. Do not move to a deny-all/allow-list approach that would break existing deployments.
- **Resolved Decisions:** Prefix check uses `strings.HasPrefix(strings.ToLower(...), "dev-token-")` to catch all variants. Empty string remains valid (dev mode).
- **Open Questions:** None.

## Root Cause Analysis

### Investigation Summary

System review (TR-001) identified that `config/smackerel.yaml` commits a guessable, functional auth token as the default value. The 020-security-hardening implementation addressed empty tokens (SEC-021) and generic placeholders, but the project-specific default value was not added to the reject list.

### Root Cause

The placeholder reject list in `Validate()` was populated with generic strings (`changeme`, `test-token`, etc.) but not the actual committed default value from `config/smackerel.yaml`. Since the default is 24 characters, it passes the length check. The config file violated the SST secrets management pattern ("empty-string placeholders are the intended dev pattern") by shipping a functional value.

### Impact Analysis
- Affected components: `internal/config/config.go` (add to reject list), `config/smackerel.yaml` (change default), `internal/config/validate_test.go` (add regression tests)
- Affected data: None
- Affected users: Operators who deploy without changing the default token (they get a clear error instead of silent insecurity)
- Blast radius: Low — config validation change, no runtime behavior change for properly configured deployments

## Fix Design

### Change 1: Extend placeholder reject list (`internal/config/config.go`)

In the `Validate()` method, after the existing placeholder loop (line ~607):

```go
// Reject known placeholder auth tokens — these are guessable defaults
placeholders := []string{
    "development-change-me",
    "changeme",
    "change-me",
    "placeholder",
    "test-token",
    "default",
    "dev-token-smackerel-2026",  // ADD: committed default from smackerel.yaml
}
for _, p := range placeholders {
    if strings.EqualFold(c.AuthToken, p) {
        return fmt.Errorf("SMACKEREL_AUTH_TOKEN is set to a known placeholder value %q — generate a secure random token", c.AuthToken)
    }
}
// ADD: reject any dev-token-* prefix (catches future variants)
if strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-") {
    return fmt.Errorf("SMACKEREL_AUTH_TOKEN starts with 'dev-token-' which is a guessable pattern — generate a secure random token")
}
```

### Change 2: Default to empty string (`config/smackerel.yaml`)

```yaml
# Before:
auth_token: "dev-token-smackerel-2026" # REQUIRED: set a secure random token (min 16 chars). Run: openssl rand -hex 24

# After:
auth_token: "" # REQUIRED: set a secure random token (min 16 chars). Run: openssl rand -hex 24
```

### Change 3: Add regression tests (`internal/config/validate_test.go`)

Add tests that verify:
- The literal `dev-token-smackerel-2026` is rejected
- The prefix `dev-token-anything` is rejected
- Case-insensitive match works (`DEV-TOKEN-SMACKEREL-2026`)
- Valid random tokens still pass
- Empty string still passes (dev mode)

### Scenario-to-Test Mapping

| Scenario | Test | Type |
|----------|------|------|
| Literal default rejected | `TestValidate_CommittedDefaultTokenRejected` | Unit |
| Prefix pattern rejected | `TestValidate_DevTokenPrefixRejected` | Unit |
| Case-insensitive rejection | `TestValidate_DevTokenCaseInsensitive` | Unit |
| Valid token passes | Existing `setRequiredEnv` uses valid token | Unit |
| Empty token passes (dev mode) | Covered by existing startup WARN tests | Unit |
| YAML default is empty | `TestConfigYAML_AuthTokenDefaultEmpty` | Unit |
