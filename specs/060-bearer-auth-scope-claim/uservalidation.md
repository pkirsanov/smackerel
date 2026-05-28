# User Validation: 060 Bearer Auth Scope Claim & RequireScope Middleware

## Checklist

- [x] The plan extends the spec 044 PASETO claim set with an OPTIONAL `scope: []string` claim and threads it through `ParsedToken.Scopes` → `Session.Scopes` without modifying spec 044's signature/footer/revocation contracts.
- [x] The plan ships `auth.RequireScope(required ...string)` middleware with AND semantics (OQ-DSN-AMD-1), dev/test + bootstrap pass-through (OQ-DSN-AMD-2), and 403 `scope_required` body shape — with the rejection metric labelled by the FIRST missing required scope to bound label cardinality.
- [x] The plan creates `internal/auth/scopes.go` as the single canonical registry for `RegisteredScopeSurfaces`, `ScopeNameRegex`, `ValidateScopeName`, and `ExtractScopeSurface` (OQ-DSN-AMD-3) with zero new SST keys and zero DB schema changes.
- [x] The plan extends the `auth enroll` and `auth rotate` CLI surface with repeatable `--scope` (via `flag.Func` to avoid collision with capability-internal commas like `extension:bookmarks,history`), `--allow-unknown-surface` escape hatch, `--prior-token <wire>` for rotation preserve, and `--scope ""` demote sentinel; adds `auth inspect`.
- [x] The plan resolves design §7.4 in favor of a `./smackerel.sh auth` passthrough wrapper that forwards `$@` verbatim through the smackerel-core container, propagates exit codes, and aligns the operator surface with the rest of the `./smackerel.sh` CLI.
- [x] The plan ships BS-002 as the headline adversarial regression test that fails loudly if `getScopeClaim` ever falls back to treating missing claim as `[]string{"*"}` or any wildcard; the test contains no bailout `if err != nil { return }` early-exits.
- [x] The plan preserves backward compatibility as a hard invariant: legacy spec-044 tokens (no `scope` claim) MUST continue to authenticate against every endpoint NOT wired with `RequireScope`; the BS-004 regression test against the live router proves this.
- [x] The plan adds `auth_scope_rejected_total{required_scope,user_id}` and `auth_scope_check_bypassed_total{source}` counter vectors with closed-set label cardinality unit tests modelled on `TestAuthValidationOutcome_AcceptsClosedSetLabels`.
- [x] The plan honors SST zero-defaults (no `${VAR:-default}`, no `os.Getenv("KEY","default")`), terminal discipline (all commands via `./smackerel.sh`), and no env-specific content in any committed artifact (generic placeholders only).
- [x] The plan explicitly ships ZERO endpoint wiring of `RequireScope` on pre-existing endpoints (spec 058 wires its own), per design §9.
- [x] The plan unblocks spec 058 by delivering `auth.Session.Scopes`, `auth.RequireScope(...)`, the `--scope` enrollment surface, and the documented contract before spec 058's `/v1/connectors/extension/ingest` handler implementation begins.

## User-Reported Regressions

No user-reported regressions are open for this planning phase.
