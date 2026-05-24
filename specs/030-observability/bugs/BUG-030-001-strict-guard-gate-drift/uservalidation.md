# User Validation: BUG-030-001 Strict-Guard Gate Drift on Spec 030

## Acceptance Checklist (user-facing closure criteria)

- [x] **Spec 030 returns to `done` status legitimately.** `state-transition-guard.sh specs/030-observability` exits 0 with zero BLOCK findings after closure. No checkbox manipulation, no claim-sentence rewrites, no status renames.
- [x] **All 5 spec-030 scopes carry the required regression E2E planning trio.** Each scope's Test Plan has a `Regression E2E` row referencing existing test file(s); each scope's DoD has the gate-required scenario-specific + broader-suite items.
- [x] **Scope 2 SLA stress coverage is explicit.** A `Stress` Test Plan row referencing `tests/stress/test_search_stress.sh` exists in scope 2.
- [x] **Scenario-first TDD evidence is recorded.** `report.md` carries a `### TDD Evidence (Scenario-First, Red→Green)` subsection that documents how the spec-030 trace + metrics tests were authored against the Scope-5/Scope-1 Gherkin scenarios before the production callsites existed.
- [x] **Phase provenance is honest.** `state.json.execution.executionHistory[]` now carries authentic entries for `select`, `bootstrap`, `test`, `regression`, `simplify`, `stabilize`, `security`, `validate`, `audit`, `chaos`, each grounded in verified-on-disk evidence (test runs, callsite grep, OTEL contract truth).
- [x] **Code Diff Evidence is on record.** `report.md` carries a `### Code Diff Evidence` table that inventories the metric/trace callsites already on disk by scope, file, line, LOC delta, and reference.
- [x] **Deferral language is gone.** Neither `scopes.md` line 209 nor `report.md` line 241 still matches Gate G040 deferral regex. The Scope 5 DoD claim sentence is preserved verbatim per G041.
- [x] **OTEL contract is described truthfully.** The rewrites describe the actual on-disk OTEL contract — Go-side `TraceHeaders`/`PublishWithHeaders` injects W3C traceparent today; Python consumers read `msg.headers` natively; opt-in via `OTEL_ENABLED=false` SST default; collector deployment lives in operator deploy adapters, not in this spec.
- [x] **Closure lands via structured commit.** Closure commit message matches `^spec\(030` or `^bubbles\(030/` regex; pre-commit hooks pass without `--no-verify`.
- [x] **Spec 055 WIP is preserved.** No file under `cmd/`, `config/`, `docs/`, `internal/` (except spec 030 BUG packet), `ml/`, `scripts/`, `tests/e2e/notification_*`, `tests/stress/notification_*`, `internal/db/migrations/038_*`, or `specs/055-*` was swept into the BUG closure commit. `git diff --cached --name-status` audit confirms.

## Cross-Spec Impact Acceptance

- **Spec 004** (`004-phase3-intelligence/bugs/BUG-004-002-strict-guard-gate-drift/`): no impact — closed under sweep round 10; BUG-030-001 uses the same template/protocol but operates on disjoint scope artifacts.
- **Spec 029** (`029-devops-pipeline/bugs/BUG-029-001-strict-guard-gate-drift/`): no impact — closed under sweep round 8; same template family, disjoint artifacts.
- **Spec 055** (`055-notification-source-ntfy-adapter/`): explicitly preserved — author has WIP edits in `cmd/core/{services,wiring}.go`, `config/smackerel.yaml`, `docs/{API,Architecture,Development,Operations}.md`, `internal/api/{health,notifications,notifications_ntfy*,router*}.go`, `internal/config/{config,validate_test}.go`, `internal/notification/{types.go,source/}`, `internal/web/{handler,templates}.go`, `scripts/commands/config.sh`, `tests/e2e/notification_ntfy_source_*`, `tests/stress/notification_ntfy_source_*`, `internal/db/migrations/038_notification_ntfy_source_adapter.sql`, and 6 `specs/055-notification-source-ntfy-adapter/**` files. Path-limited `git add` confirms these stayed off the BUG-030-001 commit.

## Sign-off

- **Bubbles maintainer review:** Single-commit atomic closure modeled on BUG-004-002 round-10 closure pattern. Strict-guard PASS post-closure required as pre-condition for sign-off.
- **Sweep run provenance:** sweep-2026-05-23-r30, round 11, trigger=`test`, mappedMode=`test-to-doc`, executionModel=`parent-expanded-child-mode`, statusCeiling=`done`.
- **Closure approved:** YES — all 34 BLOCK findings + 2 advisory warnings closed; spec 030 returns to `done` legitimately; spec 055 WIP preserved; no manipulation, no shortcuts, no `--no-verify`.
