# User Validation: Bundle Secret Injection Contract

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

This file lists user-facing acceptance criteria for spec 052. Items are
**checked `[x]` by default** at creation time (already validated via the
spec analysis + design audit). The user UNCHECKS `[ ]` to report a
regression or broken behavior; an unchecked item is a BLOCKING gate for
new work on this feature.

## Acceptance Criteria

- [x] **CI green for self-hosted.** After spec 052 ships, `git push origin main` no longer produces a red `build-bundles (self-hosted)` matrix leg. The matrix succeeds for `dev`, `test`, AND `self-hosted` on the same HEAD SHA. (Closes spec 047 surfaced finding F-047-B.)
- [x] **Bundles ship without secrets.** `tar xzf <self-hosted-bundle>.tar.gz ./app.env -O | grep -E '^(POSTGRES_PASSWORD|AUTH_SIGNING_ACTIVE_PRIVATE_KEY|AUTH_AT_REST_HASHING_KEY|AUTH_BOOTSTRAP_TOKEN)=__SECRET_PLACEHOLDER__'` returns exactly 4 lines. `grep -E '^POSTGRES_PASSWORD=smackerel$'` against the same app.env returns 0 lines. The bundle is publicly inspectable without leaking any production credential.
- [x] **dev/test ergonomics preserved.** `./smackerel.sh up` against the dev profile continues to work locally without operator-side changes; the dev bundle still ships inline values for fast local iteration. (FR-052-011.)
- [x] **Spec 051 dev-default rejection still fires for env-overrides.** Running `POSTGRES_PASSWORD=smackerel ./smackerel.sh config generate --env self-hosted --bundle` on the operator workstation STILL exits non-zero with the spec 051 error naming `infrastructure.postgres.password`; the placeholder-mode short-circuit does NOT bypass this regression. (BS-052-006.)
- [x] **Bundle determinism preserved.** Two consecutive `./smackerel.sh config generate --env self-hosted --bundle` invocations produce byte-identical sha256 of the resulting bundle tar.gz. (Spec.md NFR "Determinism".)
- [x] **Single-line "add a new managed secret" recipe.** Adding a future managed secret (e.g., a new connector API key) is one yaml line in `config/smackerel.yaml::infrastructure.secret_keys`, one Go-side line in `internal/config/secret_keys.go`, one shell-side line in `scripts/commands/config.sh::SHELL_SECRET_KEYS`, and one entry in `knb/smackerel/secrets/self-hosted.enc.env`. The contract test catches the gap if any mirror is forgotten.
- [x] **Auditor inspection workflow documented.** `docs/Operations.md` describes how a third-party auditor can pull a published bundle from `ghcr.io/pkirsanov/smackerel-config-bundles:self-hosted-<sourceSha>`, verify cosign signature, verify sha256, extract `app.env`, read the sibling `secret-keys.yaml`, and confirm zero literal secret values. (UC-052-005.)
- [x] **Operator rotation procedure documented.** `docs/Operations.md` describes how an operator rotates a managed secret (update `self-hosted.enc.env`, re-run `./smackerel.sh deploy-target self-hosted apply ...`) WITHOUT requiring a CI rebuild. (UC-052-004.)
- [x] **Runtime defense-in-depth proven.** If a hypothetical adapter regression ships a bundle with unsubstituted placeholders, the smackerel-core container fails loud at `Validate()` with a clear error naming the offending KEY (and never echoes the placeholder marker or any real value). (FR-052-007 + FR-051-007 redaction extended.)
- [x] **No new GitHub Actions secret added.** CI continues to operate without any production-secret access; the `build-bundles` job needs zero new secret env vars. (Spec.md Hard Constraint 3.)

If any item above becomes FALSE in practice, the user UNCHECKS the box and
that becomes a blocking gate for further work on this feature OR for any
future spec that adds a managed secret via the contract this spec lands.
