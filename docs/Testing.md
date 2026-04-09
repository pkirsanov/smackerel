# Smackerel Testing Guide

This guide defines Smackerel's CLI-owned test surface and the isolation rules for runtime testing.

## Current Test Coverage

| Language | Packages/Files | Test Count | Type |
|----------|---------------|------------|------|
| Go | 25 packages | 450+ test functions | Unit (behavioral + structural) |
| Python | 2 test files | 20+ tests | Unit |
| Shell (E2E) | 57 scripts | End-to-end | Live-stack |
| Shell (Stress) | 2 scripts | Stress | Live-stack |

### Key Test Areas

- **Security tests**: auth token validation (placeholder rejection, min length), SSRF URL blocking (private IPs, IPv6, metadata), ILIKE wildcard escaping, config validation (PORT, CRON)
- **Search tests**: temporal intent parsing (8 patterns), search request handling
- **Connector tests**: IMAP tier assignment, CalDAV event parsing, YouTube engagement tiers, RSS/Atom parsing, bookmarks parsing, browser dwell time, Keep takeout parsing + label mapping + qualifiers, Hospitable PAT auth + pagination + rate-limit retry + multi-resource sync + cursor management + sender classification + rating precision
- **Telegram tests**: share capture, forward metadata, conversation assembly (timer/overflow/concurrent), media group assembly
- **Intelligence tests**: synthesis insights, alert lifecycle, resurfacing scoring
- **Pipeline tests**: dedup, processing, embedding format, tier assignment

## Current Validation Surface

The repository now exposes a sanctioned CLI-owned runtime test surface for the foundation scaffold while retaining the Bubbles framework checks for governance work.

| Test type | Command | Required when |
|-----------|---------|---------------|
| Go + Python unit | `./smackerel.sh test unit` | Runtime code changes |
| Integration | `./smackerel.sh test integration` | Runtime lifecycle or health changes |
| End-to-end | `./smackerel.sh test e2e` | Runtime, compose, or config changes |
| Stress smoke | `./smackerel.sh test stress` | Runtime health or lifecycle changes |
| Framework doctor | `bash .github/bubbles/scripts/cli.sh doctor` | Project-owned bootstrap docs change |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | Before claiming bootstrap health |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | Spec or bug artifacts change |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | Traceability-sensitive artifact content changes |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | Managed docs or competitive baseline content changes |

## Current Runtime Test Matrix

The current CLI-owned runtime surface exposes these categories today:

| Test type | Category | Required command |
|-----------|----------|------------------|
| Go unit | `unit` | `./smackerel.sh test unit --go` |
| Python unit | `unit` | `./smackerel.sh test unit --python` |
| Integration | `integration` | `./smackerel.sh test integration` |
| End-to-end API | `e2e-api` | `./smackerel.sh test e2e` |
| End-to-end UI | `e2e-ui` | `./smackerel.sh test e2e` (web UI paths included) |
| Stress | `stress` | `./smackerel.sh test stress` |

### Go Package Coverage

All 25 Go packages have tests:

- `internal/api` — capture, search, health, digest, recent handlers
- `internal/auth` — OAuth2 provider, token exchange
- `internal/config` — validation, placeholder rejection, env parsing
- `internal/connector` — framework, registry, backoff, supervisor
- `internal/connector/bookmarks` — Chrome/Netscape parsing
- `internal/connector/browser` — dwell time, skip list, privacy
- `internal/connector/caldav` — event sync, attendee extraction, tier assignment
- `internal/connector/imap` — email sync, tier qualification, action item extraction
- `internal/connector/keep` — Takeout parsing, normalization, labels, qualifiers
- `internal/connector/hospitable` — PAT auth, pagination, Retry-After, active reservation sync, normalizer (4 types), cursor round-trip, partial failure, sender classification, URL population, rating precision
- `internal/connector/maps` — activity classification, trail detection, GeoJSON
- `internal/connector/rss` — RSS + Atom feed parsing
- `internal/connector/youtube` — video sync, engagement tiers
- `internal/db` — migration system
- `internal/digest` — generation, formatting
- `internal/extract` — readability, SSRF protection, content hashing
- `internal/graph` — similarity, entity, topic, temporal linking
- `internal/intelligence` — synthesis, alerts, commitments, resurfacing
- `internal/nats` — JetStream client, stream management
- `internal/pipeline` — processing, dedup, tier assignment
- `internal/scheduler` — cron scheduling
- `internal/telegram` — bot routing, share capture, forwarding, conversation assembly, media groups, format
- `internal/topics` — lifecycle management
- `internal/web` — handler, search, artifact detail, status
- `internal/web/icons` — SVG validation

## Environment Isolation Rules

### Development State Is Sacred

The persistent development stack exists for manual work only.

- It uses named volumes.
- It must survive CLI restarts.
- It must never be the target for automated E2E, integration, chaos, or validation writes.

### Test State Must Be Disposable

The automated test environment must use ephemeral storage.

- PostgreSQL test data should use project-scoped test volumes that are removed during test cleanup.
- JetStream or queue state used by tests should use test-scoped volumes removed during cleanup.
- Extracted artifact scratch data and temp uploads should be disposable.
- Tests should create uniquely identifiable synthetic fixtures.

### Validation And Chaos Must Be Isolated

Certification, validation, and chaos runs must use isolated runtime state.

- Use a separate Compose project name.
- Use disposable stores.
- Never tear down another active session's runtime implicitly.

## E2E Requirements

Smackerel must adopt the same live-stack standards as the stronger repos.

### Live Stack Only

- `integration`, `e2e-api`, and `e2e-ui` must hit the real running stack.
- Request interception in live categories is forbidden.
- If a test uses interception or canned responses, it must be reclassified out of live categories.

### E2E Uses The Test Stack Only

`./smackerel.sh test e2e` must boot or attach to the ephemeral test stack, never the persistent dev stack.

Required behavior:

- Start disposable test storage.
- Run migrations or schema setup against the test store.
- Seed only synthetic test data.
- Start the runtime against the test environment.
- Execute live-stack E2E coverage.
- Tear down or reset disposable state safely.

### Bug Fixes Need Adversarial Regressions

Every bug fix regression must include at least one case that would fail if the bug returned.

- Tautological fixtures are forbidden.
- Silent-pass bailout logic is forbidden.
- Missing required controls or redirects must fail loudly.

## Verification Standards

Smackerel inherits the Bubbles evidence rules:

- Pass/fail claims require executed commands.
- Test evidence must include raw command output, not summaries.
- Long-running commands must use explicit timeouts.

When new runtime categories are added, update this file, the command registry, and copilot instructions in the same change set so the documented test surface matches reality.