# Design: BUG-022-002 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [022 spec](../../spec.md) | [022 scopes](../../scopes.md) | [022 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 022 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately described the delivered behavior — backup CLI, DB pool SST plumbing, capture DB-health gate, ML health cache, per-job mutex guards, sequential shutdown, NATS dead-letter routing — but did not embed the `SCN-022-NN` trace ID. The traceability-guard's `scenario_matches_dod` function tries trace-ID equality first and falls back to a fuzzy "≥3 significant words shared" check; for these eight scenarios the DoD wording happened to fall below the threshold (e.g. `SCN-022-01` "Successful database backup" vs DoD "produces a valid, non-empty `.sql.gz` file" — only "backup"-adjacent token overlaps), so the gate fails.

Two ancillary problems accumulated under the same root: (1) `scenario-manifest.json` was never generated for spec 022 (G057/G059), and (2) the Test Plan rows for Scopes 1–4 listed only `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` invocations — those strings contain no `path/with/slashes.ext` pattern, so the guard's `extract_path_candidates` regex returned nothing for 12 of 14 rows. The first existing-file requirement of the trace guard therefore failed even though the underlying `*_test.go` files were already present and passing.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has four parts:

1. **Trace-ID-bearing DoD bullets** added to `scopes.md`:
   - Scope 1 DoD gains four bullets — one each for `SCN-022-01`, `SCN-022-02`, `SCN-022-03`, `SCN-022-04` — with raw `go test` output for `TestValidate_DBPoolConfig_Valid` / `TestValidate_DBMaxConns_Missing` / `TestValidate_DBMinConns_Missing` / `TestValidate_DBMinConns_ExceedsMaxConns` plus source pointers to `scripts/commands/backup.sh:42-100`, `internal/config/validate.go`, and `internal/db/postgres.go::Connect`.
   - Scope 2 DoD gains one bullet for `SCN-022-06` with raw output for `TestCaptureHandler_DBHealthy_ContinuesProcessing` and a source pointer to `internal/api/capture.go::CaptureHandler`.
   - Scope 3 DoD gains one bullet for `SCN-022-11` with raw output for the 13 `TestRun*Job_OverlapGuard` tests + `TestCronConcurrencyGuard_*` tests and a source pointer to `internal/scheduler/scheduler.go::runGuarded` and `internal/scheduler/jobs.go`.
   - Scope 4 DoD gains two bullets — one for `SCN-022-12` (shutdown) and one for `SCN-022-14` (dead-letter) — with raw output for `TestShutdownAll_ParallelSubscriberStop` / `TestStop_CronStopBounded` / `TestStop_WgWaitBounded` and `TestSynthesisDeliveryFailure_RoutesToDeadLetter` / `_BelowMaxDeliver_Naks` / `_PublishFails_Naks`, plus source pointers to `cmd/core/shutdown.go` and `internal/pipeline/synthesis_subscriber.go`.

2. **Test Plan path enrichment** — every Test Plan row in `scopes.md` Scopes 1–4 has its Test column updated to inline the concrete `*_test.go` file path that the guard's `extract_path_candidates` regex can extract. The text retains the original assertion description; the path is appended in the form `internal/<pkg>/<file>_test.go::TestName`. This converts the guard's "Concrete test file references" count from 2/14 to 14/14 without changing the meaning of any assertion.

3. **Scenario manifest** `specs/022-operational-resilience/scenario-manifest.json` is generated covering all 14 `SCN-022-*` scenarios. Each entry has `scenarioId`, `scope`, `requiredTestType`, `linkedTests` (with `file` + `function`), `evidenceRefs` (test + source pointers), and `linkedDoD`.

4. **Report cross-reference** added to `specs/022-operational-resilience/report.md` documenting the bug, the per-scenario classification, and the raw verification evidence with full paths so `report_mentions_path` succeeds.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims — backup CLI delivery, DB pool SST plumbing, capture DB-health gate, ML health cache, 14-mutex cron guard, ~23s sequential shutdown, NATS dead-letter routing are all genuinely delivered and tested — and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. No assertion was relaxed. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (22 failures)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence". The 27 underlying behavior tests for the previously-flagged scenarios (8 unmapped + their adversarial siblings) all PASS pre- and post-fix; running them post-fix confirms no regression.
