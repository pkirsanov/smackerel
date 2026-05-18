# User Validation: BUG-029-005 — Decouple connector enable-signal from volume-mount-path emptiness; convert the 4 remaining dev-compose `${VAR:-default}` volume-mount substitutions to fail-loud SST

> **Status:** Pending — to be populated after implementation completes and the user (or delegated reviewer) accepts the change.

## Acceptance Criteria

- [ ] AC-1: `scripts/commands/config.sh` resolves the 4 mount-path vars (`BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR`) to a non-empty host path before heredoc emission (shell env > yaml > SST repo-default).
- [ ] AC-2: `docker-compose.yml` converts the 4 volume-mount substitutions to fail-loud `${X:?...}` form with `Gate G028` / `HL-RESCAN-012` / operator-fix-path attribution in the error message.
- [ ] AC-3: `docker-compose.yml` replaces the 4 `${X:+/data/...}` env-override substitutions with bare-literal container paths, matching the existing `AGENT_SCENARIO_DIR: /app/prompt_contracts` architectural-constant pattern. The 11-line deferral comment is replaced with a 5-line fail-loud SST contract comment.
- [ ] AC-4: `cmd/core/connectors.go` drops the redundant `&& cfg.<X> != ""` clause from the bookmarks / browser-history / maps auto-start guards. The boolean enable flag is now the SOLE load-bearing signal (matching the existing twitter pattern).
- [ ] AC-5: `internal/deploy/dev_compose_default_fallback_test.go::devComposeDefaultFallbackAllowlist` is empty. Docstring updated with BUG-029-005 reference. `TestDevComposeContract_AdversarialAllowlistRespected` updated to use synthetic non-allowlisted vars (still proves per-var-not-per-line gating).
- [ ] AC-6: `TestDevComposeContract_FailLoudVolumeMounts` exists, asserts the 4 fail-loud forms with required attribution; 3 adversarial sub-cases prove the test catches regressions to `${X:-...}` / `${X?...}` / bare `${X}` form.
- [ ] AC-7: `TestComposeEnvOverrides_ContainerInternalConstants` exists, asserts the 4 env overrides are bare-literal; 1 adversarial sub-case proves the test catches regression to `${X:+/data/...}` form.
- [ ] AC-8: `TestConnectorStartupGate_BooleanIsSoleLoadBearingSignal` (NEW FILE: `cmd/core/connectors_startup_gate_test.go`) exists, asserts zero `<Flag>Enabled && cfg.<X> != ""` patterns; 1 adversarial sub-case proves the test catches re-introduction.
- [ ] AC-9: `docker compose --env-file config/generated/dev.env -f docker-compose.yml config -q` exits 0 (GREEN). RED proof: `docker compose --env-file /dev/null -f docker-compose.yml config -q` exits non-zero with named-var error.
- [ ] AC-10: `./smackerel.sh test unit --go` PASSes — all pre-existing tests + new tests GREEN.
- [ ] AC-11: 4 `.gitkeep` files force-added with `!data/<dir>/.gitkeep` exception lines in `.gitignore`; fresh-clone bootstrap preserves fail-loud Compose substitution out-of-the-box.

## Reviewer Sign-off

- [ ] All acceptance criteria above are satisfied with evidence in `report.md`.
- [ ] No environment-specific values (real hostnames, IPs, tailnet identifiers, usernames) introduced in any file under this packet.
- [ ] Bubbles validators all PASSED (state-transition-guard, artifact-lint, traceability-guard, regression-baseline-guard).
- [ ] CI green after push.
