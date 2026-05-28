# Bug: [BUG-020-007] Spec 020 prescribes literal 127.0.0.1 host binds — superseded by spec 042 but no pointer recorded

## Summary
`specs/020-security-hardening` still prescribes literal `127.0.0.1:HOST_PORT` host-bind syntax and a "100% of services bind to 127.0.0.1" acceptance criterion. Spec `042-tailnet-edge-bind-pattern` explicitly supersedes that prescription (deploy compose now requires the fail-loud `${HOST_BIND_ADDRESS:?...}` form per `.github/instructions/smackerel-no-defaults.instructions.md` and gate G028, and infra services have no host `ports:` block at all). Spec 020 carries no `supersededBy` pointer to spec 042, and the affected sections contain no inline supersession note. This is artifact-only documentation drift; the deployed code already matches spec 042.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — stale governance doc that contradicts current binding pattern and gate G028; misleads any reader (human or agent) writing new deploy/compose work
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced — evidence is the spec text itself)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Read `specs/020-security-hardening/spec.md` L48–52, L101–109, L269 — prescribes literal `127.0.0.1:HOST_PORT` binds and "100% of services bind to 127.0.0.1" as a success metric.
2. Read `specs/020-security-hardening/design.md` L5–10 — same literal-loopback pattern.
3. Read `specs/042-tailnet-edge-bind-pattern/spec.md` L17, L104, L255 — explicitly supersedes that prescription.
4. Read `specs/020-security-hardening/state.json` — no `supersededBy` field referencing `042-tailnet-edge-bind-pattern`.
5. Inspect `deploy/compose.deploy.yml` — already uses `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` for `smackerel-core` and `smackerel-ml`; `postgres` and `nats` have no `ports:` block. Code matches spec 042, contradicting spec 020.

## Expected Behavior
- `specs/020-security-hardening/state.json` carries a `supersededBy: "042-tailnet-edge-bind-pattern"` pointer (in whichever field the v3 control-plane schema supports for cross-spec supersession of an in-place section).
- `specs/020-security-hardening/spec.md` and `specs/020-security-hardening/design.md` open with an inline supersession note at the top of the affected sections (or at the top of the document) directing the reader to spec 042 for the canonical bind-address pattern.
- The L269 success-metric row "Host port binding — 100% of services bind to 127.0.0.1" is updated or annotated to reflect the spec-042 invariant (fail-loud `${HOST_BIND_ADDRESS:?...}` for `smackerel-core` / `smackerel-ml`; no host `ports:` block for `postgres` / `nats`).

## Actual Behavior
- Spec 020 still reads as if literal `127.0.0.1:HOST_PORT` is the current target pattern.
- No supersession pointer in `state.json` and no inline note in `spec.md` / `design.md`.
- Acceptance criterion at L269 is stale.

## Environment
- Repo: `smackerel` @ current `main`
- Affected artifacts: `specs/020-security-hardening/spec.md`, `specs/020-security-hardening/design.md`, `specs/020-security-hardening/state.json`
- Authoritative successor: `specs/042-tailnet-edge-bind-pattern/`
- Authoritative policy: `.github/instructions/smackerel-no-defaults.instructions.md`, gate G028
- Code reality (already matches 042): `deploy/compose.deploy.yml`

## Error Output
```
specs/020-security-hardening/spec.md:48-52   prescribes literal 127.0.0.1:HOST_PORT binds
specs/020-security-hardening/spec.md:101-109 same pattern reiterated
specs/020-security-hardening/spec.md:269     success metric "100% of services bind to 127.0.0.1"
specs/020-security-hardening/design.md:5-10  same literal-loopback pattern
specs/020-security-hardening/state.json      no supersededBy: "042-tailnet-edge-bind-pattern"
specs/042-tailnet-edge-bind-pattern/spec.md:17,104,255  explicitly supersedes spec 020 bind prescription
```

## Root Cause (filled after analysis)
Spec 042 shipped the tailnet-edge bind pattern and reversed the spec-020 literal-loopback prescription, but the supersession was not back-propagated into spec 020's artifacts: no state.json pointer was added, and the spec 020 prose was not annotated. Surfaced by spec-review P0-2.

## Related
- Feature: `specs/020-security-hardening/`
- Supersedes from: `specs/042-tailnet-edge-bind-pattern/`
- Policy: `.github/instructions/smackerel-no-defaults.instructions.md`
- Gate: G028 (NO-DEFAULTS / fail-loud SST)
- Compose contract test: `internal/deploy/compose_contract_test.go`
