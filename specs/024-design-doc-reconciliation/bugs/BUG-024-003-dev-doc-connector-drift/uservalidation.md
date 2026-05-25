# BUG-024-003 User Validation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] AC-01: `docs/Development.md` L31 reads `- 16 passive connectors (...QF Decisions companion via spec 041 read-only packet flow)` with exactly 16 parenthetical items; `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits.
- [x] AC-02: `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` returns ≥ 1; QF Decisions is enumerated in the L31 parenthetical list.
- [x] AC-03: `specs/024-design-doc-reconciliation/spec.md` R-006 reconciled from `the 15 implemented connectors` to `the 16 implemented connectors`; R-006 bulleted list contains 16 entries with `qfdecisions` as the 16th entry preserving Principle 10 boundary text `no financial advice generation`.
- [x] AC-04: `specs/024-design-doc-reconciliation/spec.md` R-PRD-011 last bullet reads `16 committed connectors`; AC-5 reads `match the 16 committed connectors`; problem statement L9 / hard constraints L23 / goals L33 / UC-003 L84 / scenarios L219 all read `16` (not `15`).
- [x] AC-05: `grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` returns 0 hits; internal parity with BS-004 (L119-121) restored.
- [x] AC-06: `internal/deploy/docs_connector_count_contract_test.go` created with `assertConnectorCountContract` pure function + `TestConnectorCountContract_LiveFile` + 3 adversarial sub-tests (`AdversarialConnectorsGoLow`, `AdversarialSmackerelMdHigh`, `AdversarialDevelopmentMdLow`).
- [x] AC-07: `./smackerel.sh test unit --go --go-run TestConnectorCountContract --verbose` exits 0 with all 4 PASS lines; full `internal/deploy` suite (~21s, 24 tests including the 4 new) green.
- [x] AC-08: 3 adversarial sub-tests prove the assertion is non-tautological — each synthetically violates exactly one of the 3 surfaces and the contract function returns the expected `contract violation:` error with precise diagnostics naming the disagreeing counts.
- [x] AC-09: Spec 024 `status` remains `done`. `certification.completedScopes` and `certification.certifiedCompletedPhases` for the original 2 scopes preserved unchanged. `resolvedBugs[]` extended with BUG-024-003 entry.
- [x] AC-10: No runtime code, schema, NATS topology, web template, prompt contract, Telegram command, deploy script, compose file, or `smackerel.yaml` value is modified. `git diff --stat` shows only `docs/Development.md` + `specs/024-design-doc-reconciliation/` paths + `internal/deploy/docs_connector_count_contract_test.go`.
- [x] AC-11: BUG-024-003 packet's own gates pass: `state-transition-guard.sh` + `artifact-lint.sh` + `traceability-guard.sh` + `artifact-freshness-guard.sh` on `specs/024-design-doc-reconciliation/bugs/BUG-024-003-dev-doc-connector-drift/` all exit 0.
- [x] AC-12: Bug `state.json::status` is `resolved` with `executionHistory[]` containing complete provenance for `bug`, `analyze`, `design`, `plan`, `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `chaos`, `validate`, `audit`, `docs`, `finalize` phases (15 entries).
- [x] AC-13: Single commit with subject prefix `bubbles(024/bug-024-003):` satisfies Check 17 structured commit gate.
- [x] AC-14: Path-limited `git add` discipline confirmed via `git diff --cached --name-status`. Zero files from `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/`, `internal/connector/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, or `ml/` swept in.
- [x] AC-15: `git push origin main` succeeded without `--no-verify`; pre-push hook ran `./smackerel.sh test pre-push` and validated the new HEAD SHA cleanly.
- [x] AC-16: Sweep ledger `.specify/memory/sweep-2026-05-24-r10.json` round 9 entry appended (local-only, NOT in the commit) preserving R1-R8 entries; status: `completed_owned`; executionModel: `parent-expanded-child-mode`; findings/findingsClosedThisRound/bugsSpawned/bugId/bugFinalStatus/guardsClean populated.

## One-To-One Finding Closure Accounting

- **F1 (HIGH, docs-runtime-drift) closed:** Iteration 1 (docs/Development.md L31 single-line edit) — `grep -nE '15 passive connectors' docs/Development.md` returns 0 hits post-edit.
- **F2 (MEDIUM, missing-forward-detection) closed:** Iteration 3 (new `internal/deploy/docs_connector_count_contract_test.go` — pure function + live test + 3 adversarial sub-tests) — `./smackerel.sh test unit --go --go-run TestConnectorCountContract` exits 0; adversarial sub-tests prove the assertion fires against any synthetic drift.
- **F3 (LOW, artifact-internal-inconsistency) closed:** Iteration 2 (specs/024-design-doc-reconciliation/spec.md 8 edits across problem statement / hard constraints / goals / UC-003 / scenarios / R-PRD-011 / R-006 intro+list / AC-5) — `grep -nE 'the 15 implemented connectors|the 15 committed connectors|match the 15 committed connectors' specs/024-design-doc-reconciliation/spec.md` returns 0 hits post-edit.

The packet honors the chaos-hardening NON-NEGOTIABLE governance:
- Single atomic commit prefixed `bubbles(024/bug-024-003):` lands all changes (Check 17 clean).
- Path-limited `git add` discipline confirmed; zero stray files from `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, `cmd/`, `internal/connector/`, `internal/api/`, `internal/config/`, `internal/web/`, `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `.github/bubbles/`, `ml/`.
- Parent spec 024 `state.json` and `report.md` extended with chaos-round closure evidence; sweep ledger R9 appended preserving R1-R8.
- `git push origin main` succeeded without `--no-verify`; pre-push hook validated cleanly.

I accept the closure and authorize sweep round 9 to be marked `completed_owned` with this packet as the recorded evidence.

— bubbles.workflow (autonomous), parent-expanded `chaos-hardening` mode, sweep-2026-05-24-r10 round 9, 2026-05-25
