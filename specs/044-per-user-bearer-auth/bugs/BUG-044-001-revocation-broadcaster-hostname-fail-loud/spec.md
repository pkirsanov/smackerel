# Bug: BUG-044-001 — auth revocation broadcaster falls back to literal `"smackerel-core"` when HOSTNAME is empty

## Classification

- **Type:** runtime wiring defect — silent default at startup env-read site (Gate G028 / no-defaults SST policy violation)
- **Severity:** P2 — MEDIUM (the broadcaster is gated on `cfg.Auth.Enabled && cfg.Auth.RevocationNATSSubject != ""`; production home-lab today does not enable revocation broadcast, so the fallback is a no-op live, but the defect is the bypass of the Gate G028 fail-loud read AND the cross-replica deduplication failure that would silently surface the moment the operator turns on revocation broadcast)
- **Parent Spec:** 044 — per-user-bearer-auth (the spec that introduced the auth revocation broadcaster surface in Scope 02 commit `5f4ceb98`)
- **Workflow Mode:** test-to-doc
- **Status:** Fixed
- **Discovered By:** 2026-05-14 home-lab readiness re-scan (finding HL-RESCAN-008)

## Problem Statement

`cmd/core/wiring.go` lines 243–246 (pre-fix) read the per-replica broadcaster instance identifier via:

```go
instanceID := os.Getenv("HOSTNAME")
if instanceID == "" {
    instanceID = "smackerel-core"
}
```

This is forbidden by the repo-wide no-defaults SST policy (`.github/instructions/smackerel-no-defaults.instructions.md` Gate G028; for Go the `os.Getenv("KEY")` + empty check MUST trigger a loud refusal, never a hidden fallback string). Two contracts collided:

1. **Gate G028 contract:** every env-read site MUST be either `os.Getenv("KEY")` + empty check → fatal/refuse, or a `(value, error)` helper that propagates the empty case as an error. Silent fallback to a literal string is exactly the pattern Gate G028 forbids.
2. **Revocation-broadcast deduplication contract:** the `instanceID` is the broadcaster's identity on the NATS subject. When multiple `smackerel-core` replicas run, each MUST broadcast under a distinct identifier so consumers can deduplicate cross-replica revocation messages. The pre-fix fallback collided every replica's identity to the same literal `"smackerel-core"` string — defeating deduplication the moment more than one replica started.

The pre-fix runtime behavior was:

- `HOSTNAME` set (the canonical Docker container case where the orchestrator injects a unique hostname): broadcaster wired with the unique value. Correct.
- `HOSTNAME` unset or empty (operator runs the binary outside a container, OR the orchestrator forgets to inject `HOSTNAME`, OR the entrypoint clears the env): broadcaster wired with the literal `"smackerel-core"`. **Silently incorrect** for any deployment with more than one `smackerel-core` replica — every replica broadcasts under the same name, so the consumer cannot tell which replica revoked the token, and the deduplication assumption built into the consumer code fails open.

The defect is the bypass of the SST gate AND the silent collision of replica identities. The fix replaces the silent fallback with a fail-loud read of `HOSTNAME` extracted into a small `(value, error)`-returning helper (`resolveBroadcasterInstanceID()`), and lets the broadcaster construction site refuse to wire the broadcaster (and surface a loud `slog.Error` with explicit attribution) when the read fails. The error path matches the existing non-fatal-but-loud handling in the same code block (where `revocation.NewBroadcaster` construction errors and `Subscribe` errors are also non-fatal).

## Detection

| Aspect | Detail |
|---|---|
| Trigger | Home-lab readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-008 |
| Severity | P2 (live home-lab today does not enable revocation broadcast on `cfg.Auth.RevocationNATSSubject != ""`, so the fallback is a no-op there; defect is the bypass of the Gate G028 fail-loud read AND the cross-replica deduplication failure that would silently surface the moment the operator turns on revocation broadcast) |
| Audit method | Searched `cmd/core/` and `internal/` for `os.Getenv` followed by `if .* == ""` + literal-string assignment; found one violation at `cmd/core/wiring.go` line 243 (`instanceID := os.Getenv("HOSTNAME")` followed by `if instanceID == "" { instanceID = "smackerel-core" }`). Cross-referenced `.github/instructions/smackerel-no-defaults.instructions.md` Gate G028 wording: Go must be `os.Getenv` + empty check → fatal, never a silent fallback. Cross-referenced `git log -S 'os.Getenv("HOSTNAME")'` to confirm the pre-fix form was introduced by spec 044 Scope 02 commit `5f4ceb98` (the broadcaster wiring landed there). |

## Acceptance Criteria

- AC-1: `cmd/core/wiring.go` MUST NOT contain the silent-fallback form `os.Getenv("HOSTNAME")` followed by `if instanceID == "" { instanceID = "smackerel-core" }` (or any equivalent literal-string fallback). The HOSTNAME read MUST go through the new `resolveBroadcasterInstanceID()` helper.
- AC-2: `cmd/core/wiring.go` MUST declare a package-private helper `resolveBroadcasterInstanceID() (string, error)` that returns `(hostname, nil)` when `HOSTNAME` is non-empty and `("", error)` when `HOSTNAME` is empty. The error message MUST name `HOSTNAME`, AND name `HL-RESCAN-008`, AND name `Gate G028`, AND name `spec 044`, AND mention `deduplication`.
- AC-3: When `resolveBroadcasterInstanceID()` returns a non-nil error, the broadcaster construction block MUST refuse to construct the broadcaster, MUST emit a loud `slog.Error` with the error AND the NATS subject, and MUST NOT panic or `os.Exit` (matching the existing non-fatal-but-loud handling pattern in the same block where `revocation.NewBroadcaster` construction errors and `Subscribe` errors are also non-fatal).
- AC-4: When `resolveBroadcasterInstanceID()` returns a non-nil string, the broadcaster construction block MUST proceed to call `revocation.NewBroadcaster` with the resolved `instanceID` exactly as before — no behavioral change on the happy path.
- AC-5: A new `cmd/core/wiring_revocation_test.go` test file MUST exist with at least three test methods: a positive case (non-empty `HOSTNAME` → returns the value, nil error); an empty-set case (`HOSTNAME=""` → returns `""` + non-nil error referencing HL-RESCAN-008 / Gate G028 / spec 044 / deduplication); an unset case (`HOSTNAME` absent → same error shape as empty-set). The empty-set and unset cases are the adversarial regression sub-tests.
- AC-6: RED proof captured: temporarily reverting `resolveBroadcasterInstanceID` to a silent-fallback form (`return "smackerel-core", nil` on empty) MUST cause exactly the empty-set and unset test cases to FAIL with the explicit `id="smackerel-core" err=nil` mismatch message; the positive case MUST continue to PASS. Restoring the fix MUST return all three tests to PASS GREEN.
- AC-7: The existing cmd/core unit test suite (`./smackerel.sh test unit --go` filtered to `cmd/core/...`) MUST continue to PASS unchanged after the fix lands. Zero regression in adjacent code paths.
- AC-8: Generic-only constraint preserved: zero real hostnames, IPs, tailnet identifiers, owner-username tokens, or other operator-specific values introduced into any source file or evidence block. The test fixture's `HOSTNAME` value is a synthetic literal (`smackerel-core-replica-7`) chosen to be obviously non-real.

## Out of Scope

- Editing `internal/auth/revocation/broadcaster.go` (the `Broadcaster` struct's existing constructor signature is preserved; only the wiring-site read of `HOSTNAME` changes).
- Editing the existing call sites of `revocation.NewBroadcaster` in test files (`tests/integration/auth_revocation_test.go`, `tests/integration/auth_chaos_test.go`, `tests/integration/auth_chaos_scope02_test.go`, `tests/integration/auth_chaos_scope03_test.go`) — they all pass synthetic instance IDs and are unaffected by the wiring-site change.
- Editing `cfg.Auth.Enabled`, `cfg.Auth.RevocationNATSSubject`, or any other config gate that controls whether the broadcaster block runs.
- Editing `config/smackerel.yaml` or the SST loader (`scripts/commands/config.sh`) — `HOSTNAME` is a runtime env var injected by the container orchestrator, not an SST-managed value.
- Editing `Dockerfile` or `docker-compose.yml` to set `HOSTNAME` explicitly (Docker already injects `HOSTNAME` for every container by default; the fix is bounded to the runtime read site).
- Editing `cmd/core/helpers.go` `parseFloatEnv` / `parseJSONArrayEnv` / `parseJSONObjectEnv` — those are out-of-scope unused fail-soft helpers closed by [`specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/`](../../../020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/) (HL-RESCAN-014).
- Editing `specs/044-per-user-bearer-auth/spec.md`, `design.md`, `scopes.md`, `state.json`, `report.md`, or `uservalidation.md` (foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope).
- Adding fail-loud reads for OTHER env vars in `cmd/core/wiring.go` (only the `HOSTNAME` read site is in HL-RESCAN-008's scope; other defaults are tracked by HL-RESCAN-012/013/014 separately).
