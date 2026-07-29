# Report: 108 Corpus Grant Enforcement

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Release train:** `next`
**Run type:** planning only — no source code, tests, config, or docs were edited.

---

## Summary

This run converted the ratified `spec.md` and `design.md` for corpus grant enforcement into
an executable five-scope plan. It produced `scopes.md`, `uservalidation.md`,
`scenario-manifest.json`, and this report. It did **not** implement anything, did not run
tests, and did not modify `spec.md` or `design.md`.

**What was planned.** Enforcement of the already-defined `corpus:read` grant on the eight
corpus route groups in `internal/api/router.go`, via the already-existing `auth.RequireScope`
middleware, behind a two-stage OBSERVE → ENFORCE rollout selected by a single fail-loud SST
configuration value.

**Scope sequence and rationale.**

| # | Scope | Depends On | Why it sits here |
|---|---|---|---|
| 01 | Scope Registration Prerequisite | — | F-108-SURFACE-01: `corpus` is not in `auth.RegisteredScopeSurfaces`, so the operator literally cannot mint a token carrying `corpus:read`. Every downstream grant assertion is meaningless until this lands. |
| 02 | Observe-Stage Plumbing | 01 | Config + three metrics + structured log. Counts would-be denials while returning 200. Establishes the measurement that answers UC-108-001 *before* anyone can be denied. |
| 03 | Gate Mount | 02 | `auth.RequireScope(auth.GrantGlobalCorpusRead)` on the corpus group, mounted only in ENFORCE. Carries the T8 adversarial route-manifest contract test. |
| 04 | Caller Remediation | 03 | Converts the design.md §5 "unknown" rows into measured rows — notably F-108-TELEGRAM-01 — so the owning-train flag is never flipped while a caller surface is unmeasured. |
| 05 | Docs, Release Train, Flag Bundles | 04 | `docs/Operations.md`, `docs/API.md`, `docs/smackerel.md` §17.2, `config/release-trains.yaml`, and the two flag bundles. Last because the runbook documents behavior the earlier scopes create. |

**Test-plan shape.** 24 Test Plan rows across the five scopes (9 unit, 11 integration,
4 e2e-api), each mapped 1:1 to a Definition-of-Done item so Test Plan ↔ DoD parity holds per
scope. 17 Gherkin scenarios carry stable `SCN-108-*` ids, all listed in
`scenario-manifest.json` with their owning scope.

**Adversarial coverage.** Scope 03 `TP-03-04` implements design.md §8 T8: the route-manifest
contract test builds the real router with ENFORCE selected, drives it with a fixture principal
whose scope claim is **empty**, and asserts 403 on all eight route groups plus set-equality
between the canonical eight and the router's mounted corpus group. Because an empty scope
claim is exactly the input today's ungated router allows, the test is required to fail against
current `main` and to pass only once the gate is mounted. That is what makes it adversarial
rather than tautological.

**Open item routed onward, not resolved here.** `design.md` §4/§9 records
`corpusGrantEnforcement: false` in **both** `config/feature-flags.mvp.yaml` and
`config/feature-flags.next.yaml` (R-108-FL3). The repo's mechanically-enforced release-train
policy requires a flag to be default-ON in exactly one owning train and default-OFF in every
other. Scope 05 is planned to the enforced policy (`next` = ON, `mvp` = OFF) and its
`DoD-05-06` blocks on `bubbles.design` reconciling `design.md`. This packet deliberately did
not edit `design.md` to make the conflict disappear.

---

## Completion Statement

This packet is **complete for its workflow mode** (`product-to-planning`) and stops at its
ceiling, `specs_hardened`.

**Delivered:**

- `scopes.md` — 5 scopes in dependency order, each with Status, Depends On, Gherkin scenarios
  carrying stable `SCN-108-*` ids, an implementation plan, a Test Plan table, and a Definition
  of Done whose test-item count matches its Test Plan row count.
- `uservalidation.md` — `## Checklist` with checked-by-default baseline entries covering
  problem framing, scope decomposition, rollout shape, caller impact, and planning artifacts.
- `scenario-manifest.json` — all 17 `SCN-108-*` scenario contracts with owning scope and
  required test category.
- `state.json` — schema v3, `status: specs_hardened`, `certification.status: specs_hardened`,
  `releaseTrain: next`, `flagsIntroduced: ["corpusGrantEnforcement"]`.
- `report.md` — this file.

**Deliberately NOT delivered (out of mode):**

- No source, test, config, or documentation file was created or modified. Gate G073 / Check 3B
  prohibits source edits from this mode.
- No scope is marked `Done`; every scope Status is `Not Started`.
- No DoD item is checked; every item is `- [ ]`.
- No execution evidence exists, because nothing was executed. See **Test Evidence** below.

**Next owner.** `bubbles.implement` executes Scope 01 first. Scope 01 must reach `Done` with
its own recorded evidence before Scope 02 begins; the plan is sequential and scope-gated.

**Prerequisite before Scope 05 can flip the owning-train flag:** F-108-TELEGRAM-01 must be
resolved in Scope 04, and the OBSERVE-window go/no-go query must return an empty or explicitly
accepted denial set.

---

## Test Evidence

**No test evidence exists for this packet, and none is claimed.**

This is a planning-only run at ceiling `specs_hardened`. No implementation exists yet, so
there is nothing to execute and no output to record. No command was run: no build, no test, no
lint, no guard, no format check. Recording a passing result here would be fabrication under the
Anti-Fabrication Policy, and none is recorded.

### What will be captured, and by which scope

Evidence is captured at implementation time by the scope that owns the change, recorded inline
under the corresponding Definition-of-Done item as raw terminal output (≥10 lines) with the
exact command and exit code.

| Scope | Category | Command | Evidence to be captured |
|---|---|---|---|
| 01 | unit | `./smackerel.sh test unit` | `TP-01-01`, `TP-01-02` — `corpus` surface registered; `corpus:read` claim validates and authorizes |
| 01 | integration | `./smackerel.sh test integration` | `TP-01-03` — minted `corpus:read` token round-trips to a granted session |
| 02 | unit | `./smackerel.sh test unit` | `TP-02-01`, `TP-02-02`, `TP-02-03` — absent and malformed config abort startup by name; three metrics register with the closed `route_group` label set |
| 02 | integration | `./smackerel.sh test integration` | `TP-02-04`, `TP-02-05` — OBSERVE returns 200 on all eight groups while would-deny increments; granted requests count as allowed |
| 03 | unit | `./smackerel.sh test unit` | `TP-03-01` — `GateGlobalCorpusRead` denies empty and wildcard claims, allows explicit, leaks nothing |
| 03 | integration | `./smackerel.sh test integration` | `TP-03-02`, `TP-03-03`, `TP-03-04`, `TP-03-05` — ENFORCE 403s on all eight groups; documented bypass asserted; **T8 adversarial manifest test, including a recorded failing run against current `main`**; rollback restores access with no rebuild |
| 03 | e2e-api | `./smackerel.sh test e2e` | `TP-03-06`, `TP-03-07` — granted reads succeed and ungranted are refused with no id/title/count; denial byte-parity between a real and a random id |
| 04 | unit | `./smackerel.sh test unit` | `TP-04-01` — bridge token scope resolved without a silent default |
| 04 | integration | `./smackerel.sh test integration` | `TP-04-02`, `TP-04-03`, `TP-04-04` — Telegram command has an operator-actionable outcome; token rotation grants a daily user; extension tracks its principal |
| 04 | e2e-api | `./smackerel.sh test e2e` | `TP-04-05` — all six design.md §5 compatibility rows exercised with their recorded outcome |
| 05 | unit | `./smackerel.sh test unit` | `TP-05-01`, `TP-05-02` — flag default-ON in exactly one train; SST key declared with no default |
| 05 | integration | `./smackerel.sh test integration` | `TP-05-03` — generated env carries `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` for every environment |
| 05 | e2e-api | `./smackerel.sh test e2e` | `TP-05-04` — the documented UC-108-001 runbook query returns the documented shape against the real `/metrics` surface |

### Evidence rules that apply at implementation time

- Every live-category test (`integration`, `e2e-api`) runs on the **ephemeral test stack** and
  emits telemetry tagged `env=test*` only. No test writes to prod monitoring (R-108-O6, G115).
- `TP-03-04` is not satisfied by a passing run alone. The adversarial property requires a
  recorded run showing the test **failing** against the pre-gate router, alongside the passing
  run after the gate is mounted. A test that passes both before and after the fix is
  tautological and does not close the DoD item.
- Evidence is recorded inline beneath its DoD item as raw terminal output with command and
  exit code. Summaries such as "all tests pass" do not satisfy the evidence standard.
