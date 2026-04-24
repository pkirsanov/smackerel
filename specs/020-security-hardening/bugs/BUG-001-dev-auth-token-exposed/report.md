# Report: [BUG-001] Dev Auth Token Exposed as Functional Default

**Bug ID:** BUG-020-001
**Feature:** 020-security-hardening
**Created:** 2026-04-19

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Reject default token + change YAML default | Done | `internal/config/config.go:865-874` + `internal/config/validate_test.go:290`; tests pass |

Re-verified 2026-04-24 against committed code: placeholder reject list contains `dev-token-smackerel-2026`, the case-insensitive `dev-token-` prefix rejection is in place, the YAML default is empty, and the regression test covers literal, arbitrary-suffix, and mixed-case tokens.

## Completion Statement

Status: done. Each DoD item in `scopes.md` is now checked with file:line evidence pointing at committed code, and the focused Go test plus the full `./smackerel.sh test unit` run executed in this re-certification pass have been captured below.

## Test Evidence

Focused Go run captured 2026-04-24T07:29:44Z → 07:29:45Z:

```text
$ go test -count=1 -v -run "TestValidate_AuthTokenDevTokenPrefixRejected|TestValidate_AuthTokenAllPlaceholdersRejected" ./internal/config/...
=== RUN   TestValidate_AuthTokenAllPlaceholdersRejected
--- PASS: TestValidate_AuthTokenAllPlaceholdersRejected (0.00s)
=== RUN   TestValidate_AuthTokenDevTokenPrefixRejected
=== RUN   TestValidate_AuthTokenDevTokenPrefixRejected/dev-token-smackerel-2026
=== RUN   TestValidate_AuthTokenDevTokenPrefixRejected/dev-token-anything-here-1234
=== RUN   TestValidate_AuthTokenDevTokenPrefixRejected/Dev-Token-MyProject-9999
--- PASS: TestValidate_AuthTokenDevTokenPrefixRejected (0.00s)
    --- PASS: TestValidate_AuthTokenDevTokenPrefixRejected/dev-token-smackerel-2026 (0.00s)
    --- PASS: TestValidate_AuthTokenDevTokenPrefixRejected/dev-token-anything-here-1234 (0.00s)
    --- PASS: TestValidate_AuthTokenDevTokenPrefixRejected/Dev-Token-MyProject-9999 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

Full suite run via repo CLI captured 2026-04-24:

```text
$ ./smackerel.sh test unit
........................................................................ [ 21%]
........................................................................ [ 43%]
........................................................................ [ 65%]
........................................................................ [ 87%]
..........................................                               [100%]
330 passed, 2 warnings in 11.48s
```

### Validation Evidence

DoD-by-DoD verification 2026-04-24 against committed code:

```text
$ grep -n "dev-token" internal/config/config.go config/smackerel.yaml
internal/config/config.go:865:          "dev-token-smackerel-2026",
internal/config/config.go:872:  // Reject any token starting with "dev-token-" — these are development-only patterns
internal/config/config.go:873:  if strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-") {
internal/config/config.go:874:          return fmt.Errorf("SMACKEREL_AUTH_TOKEN starts with 'dev-token-' which is a development placeholder pattern — generate a secure random token: openssl rand -hex 24")
$ grep -n "auth_token" config/smackerel.yaml
19:  auth_token: "" # REQUIRED: set a secure random token (min 16 chars). Run: openssl rand -hex 24
$ grep -n "DevToken\|dev-token" internal/config/validate_test.go
290:func TestValidate_AuthTokenDevTokenPrefixRejected(t *testing.T) {
293:            "dev-token-smackerel-2026",
294:            "dev-token-anything-here-1234",
302:                            t.Fatalf("dev-token- prefix %q should be rejected", token)
```

All six DoD items map one-to-one to file:line locations above; `go test` PASS lines confirm the runtime behaviour.

### Audit Evidence

Repo-CLI audit hooks executed 2026-04-24T07:30:21Z → 07:30:29Z, plus targeted regression run for the security gate:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 -v -run TestValidate_AuthToken ./internal/config/...
=== RUN   TestValidate_AuthTokenExactly16Chars
--- PASS: TestValidate_AuthTokenExactly16Chars (0.00s)
=== RUN   TestValidate_AuthTokenAllPlaceholdersRejected
--- PASS: TestValidate_AuthTokenAllPlaceholdersRejected (0.00s)
=== RUN   TestValidate_AuthTokenCaseInsensitivePlaceholder
--- PASS: TestValidate_AuthTokenCaseInsensitivePlaceholder (0.00s)
=== RUN   TestValidate_AuthTokenDevTokenPrefixRejected
--- PASS: TestValidate_AuthTokenDevTokenPrefixRejected (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

The SST sync confirms the YAML default change is honoured by generated env files; the focused regression replay confirms no other auth-token tests regressed (4 tests, 0 failures, finished in 0.006s).
