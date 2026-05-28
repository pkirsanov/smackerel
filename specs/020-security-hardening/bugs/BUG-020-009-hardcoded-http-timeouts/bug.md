# Bug: [BUG-020-009] Hardcoded HTTP client timeouts violate SST config-driven runtime values

## Summary
Two production HTTP clients are constructed with hardcoded `time.Duration` literals instead of being driven by SST config:

- `internal/connector/markets/markets.go` L158 — `httpClient: &http.Client{Timeout: 10 * time.Second}` (financial-markets connector)
- `internal/auth/oauth.go` L117 — `client := &http.Client{Timeout: 15 * time.Second}` (generic OAuth2 token-endpoint client)

These literals are runtime values that govern external-call latency tolerance. Per `.github/copilot-instructions.md` → SST Zero-Defaults Enforcement and `.github/instructions/smackerel-no-defaults.instructions.md`, ALL configuration values MUST originate from `config/smackerel.yaml`; ZERO hardcoded ports, URLs, hostnames, OR fallback defaults anywhere in the codebase. A hardcoded timeout cannot be tuned per-environment, per-deployment, or per-incident without a rebuild, and it hides the value from `Config.Validate()`'s fail-loud surface.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — promoted from P2 in the latest code review. Not exploitable on its own but it is a direct policy violation of the SST regime and a load-bearing reason that regime exists (every "small" hardcoded value is one more thing operators cannot tune without a code edit).
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

## Reproduction Steps
1. Run `grep -nE 'Timeout: *[0-9]+ *\* *time\.Second' internal/connector/markets/markets.go internal/auth/oauth.go`.
2. Observe two matches:
   - `internal/connector/markets/markets.go:158: httpClient: &http.Client{Timeout: 10 * time.Second},`
   - `internal/auth/oauth.go:117: client := &http.Client{Timeout: 15 * time.Second}`
3. Compare against `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement: "ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase."
4. Inspect `Config.Validate()` in `internal/config/config.go` and confirm NEITHER value is in the required-keys collector — there is no way for an operator to make either value explicit.

## Expected Behavior
- Both HTTP-client timeouts MUST be sourced from `config/smackerel.yaml` and threaded through `internal/config.Config`.
- The new SST keys MUST be registered with `Config.Validate()` so a missing or unparseable value aborts boot with a consolidated, fail-loud error naming the offending key (same pattern as BUG-020-008).
- Both call-sites MUST construct their `http.Client` from the resolved config field; no `time.Second` literal may remain at the call-site.
- `config/smackerel.yaml` MUST carry an explicit non-empty value for each new key. No `:-` shell fallback, no `os.Getenv("KEY", "default")`, no in-Go default.

## Actual Behavior
- Both clients are constructed with literal `time.Duration` values at compile time.
- Operators cannot adjust either timeout without a rebuild.
- `Config.Validate()` is unaware of the values; a typo in any future env var would not be caught at boot.

## Environment
- Repo: `smackerel` @ current `main`
- Affected source: `internal/connector/markets/markets.go` (L158); `internal/auth/oauth.go` (L117); `internal/config/config.go` (Load + Validate); `config/smackerel.yaml` (connectors.financial-markets section; auth section — see design.md for exact key path)
- Authoritative policy: `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement
- Spec association: `specs/020-security-hardening/` (SST / NO-DEFAULTS regime owner)
- Code-review finding: H-4 (originally P2, promoted to actionable)
- Note: The user request named the yaml paths as `connectors.markets.http_timeout_seconds` and `auth.oauth.http_timeout_seconds`. The actual section in `config/smackerel.yaml` is `connectors.financial-markets` (L452), and there is no existing `auth.oauth` sub-section under `auth:` (L566). The implementing agent MUST choose the exact yaml path that matches the existing section names and document the choice in `design.md` before code edits begin. The user's intent — "introduce SST keys ... thread through Config ... remove hardcoded literals" — is unambiguous.

## Error Output
```
$ grep -nE 'Timeout: *[0-9]+ *\* *time\.Second' internal/connector/markets/markets.go internal/auth/oauth.go
internal/connector/markets/markets.go:158:		httpClient:       &http.Client{Timeout: 10 * time.Second},
internal/auth/oauth.go:117:	client := &http.Client{Timeout: 15 * time.Second}
```

## Root Cause (filled after analysis)
See design.md.

## Related
- Feature: `specs/020-security-hardening/`
- Sibling bug: `specs/020-security-hardening/bugs/BUG-020-008-parseintenv-silent-defaults/` (same SST NO-DEFAULTS regime; same fail-loud `Validate()` integration pattern is the canonical reference)
- Policy: `.github/instructions/smackerel-no-defaults.instructions.md`, `.github/copilot-instructions.md` § SST Zero-Defaults Enforcement
- Code-review finding: H-4 (P2 promoted to actionable)
