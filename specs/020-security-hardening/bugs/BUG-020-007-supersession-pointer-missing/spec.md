# Spec: [BUG-020-007] Spec 020 must record supersession by spec 042 for host-bind pattern

## Expected Behavior

### EB-1: state.json carries supersession pointer
`specs/020-security-hardening/state.json` MUST include a `supersededBy` reference to `042-tailnet-edge-bind-pattern` scoped to the host-bind / loopback-binding prescription. The exact field placement follows whatever the v3 control-plane state schema supports for partial-section supersession (top-level `supersededBy`, a `supersessions` array, or an equivalent provenance entry); the bug-fix scope MUST pick a schema-valid form and apply it.

### EB-2: spec.md carries inline supersession note
`specs/020-security-hardening/spec.md` MUST open the affected sections (at minimum the "Attack Surface: Network Exposure" section at L48–52 and the "Success Metrics" row at L269) with an inline note directing the reader to `specs/042-tailnet-edge-bind-pattern/` as the current canonical bind-address pattern. A document-level supersession banner at the top of `spec.md` is acceptable if it explicitly names the affected sections.

### EB-3: design.md carries inline supersession note
`specs/020-security-hardening/design.md` MUST carry the same inline supersession note at L5–10 (or at the top of the document) pointing to spec 042.

### EB-4: L269 acceptance criterion reflects spec-042 invariant
The L269 success-metric row currently reading "Host port binding — 100% of services bind to 127.0.0.1" MUST be updated OR annotated such that a reader cannot conclude that literal `127.0.0.1:HOST_PORT` is still the target. The annotation MUST name:
- the fail-loud `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` form for `smackerel-core` and `smackerel-ml`, AND
- the "no host `ports:` block" invariant for `postgres` and `nats`.

### EB-5: No runtime code changes
This bug is artifact-only. The fix MUST NOT touch any file outside `specs/020-security-hardening/`. Code already matches spec 042 (`deploy/compose.deploy.yml`, `internal/deploy/compose_contract_test.go`).

## Acceptance Criteria
1. `grep -n 'supersededBy' specs/020-security-hardening/state.json` returns a line that names `042-tailnet-edge-bind-pattern` (or equivalent schema-supported pointer).
2. `grep -n '042-tailnet-edge-bind-pattern' specs/020-security-hardening/spec.md` returns at least one match.
3. `grep -n '042-tailnet-edge-bind-pattern' specs/020-security-hardening/design.md` returns at least one match.
4. The L269 success-metric row no longer claims unqualified `127.0.0.1` binding for all services; it either references spec 042 or is rewritten to match the spec-042 invariant.
5. `git diff --name-only` for the fix touches only files under `specs/020-security-hardening/`.
6. `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening` passes.
7. `bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing` passes.

## Out of Scope
- Re-running any production security-hardening test suite.
- Editing `deploy/compose.deploy.yml`, `internal/deploy/compose_contract_test.go`, or any other code or compose file.
- Editing spec 042 (it is authoritative; spec 020 is the artifact that must be updated to reflect supersession).
