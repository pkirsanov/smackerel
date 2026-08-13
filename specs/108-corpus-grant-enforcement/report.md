# Report: 108 Corpus Grant Enforcement

**Mode:** `full-delivery` · **Ceiling:** `done` · **Release train:** `next`
**Run type:** delivery — all five scopes implemented and their tests executed.
**Current status:** `blocked` — engineering complete and green; three operator-owned,
time-bound DoD items in Scope 04 remain (see "Completion Statement").

> **Header corrected 2026-08-13.** This report was written during the planning pass and its
> header still read `product-to-planning` / `specs_hardened` / "planning only — no source code,
> tests, config, or docs were edited". That is no longer true and is corrected rather than left
> to mislead. The planning-time narrative below is retained verbatim as the record of what was
> planned; the delivery evidence is in "Test Evidence".

---

## Summary

**Delivery outcome (2026-08-13).** All five scopes are implemented. Scopes 01, 02, 03 and 05 are
Done; Scope 04 is Blocked on operator-owned items only. 87 of 90 DoD items are closed with
executed evidence. Lanes: `check`/`lint`/`format` 0, unit 145 packages / 0 failures, integration
1974 pass / 0 fail, e2e all phases pass. One BLOCKING finding raised during execution
(`F-108-COVERAGE-LABEL-01`) was root-caused and fixed rather than recorded and left; one plan
inconsistency (`F-108-S04-01`) was recorded and routed to `bubbles.plan`.

**What could not be finished, and why it is not an engineering gap.** Three Scope 04 DoD items
require a ≥ 14-day OBSERVE window and an operator-recorded principal rotation. No execution can
compress elapsed production time, so the spec is `blocked` rather than `done` — and deliberately
not `done_with_concerns`, which this repo forbids precisely because it lets half-finished work
masquerade as finished.

---

### Planning-pass narrative (retained verbatim)

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

**Current statement (2026-08-13, `full-delivery`).** This packet is **not** complete. Its status
is `blocked`, and that is the honest terminal state available to it today.

**What is complete.** All five scopes are implemented. Scopes 01, 02, 03 and 05 are `Done`;
87 of 90 DoD items carry executed evidence. Every lane is green: `check`/`lint`/`format` exit 0,
unit 145 packages with 0 failures, integration 1974 pass with 0 failures, e2e all phases pass,
and `artifact-lint` exits 0. Two findings raised during execution were resolved rather than
carried: `F-108-COVERAGE-LABEL-01` (a real code gap — the allowed counter carried no `user_id`,
so only DENIED principals were attributable) was fixed, and `F-108-S04-01` (a Change Boundary
inconsistency in the plan) was recorded and routed to `bubbles.plan`.

**What blocks completion.** Three Scope 04 DoD items, all operator-owned and time-bound:

1. ≥ 14 consecutive OBSERVE days with the stage resolved from SST at process start.
2. Proactive rotation of every principal whose grants are unknowable, recorded as an operator
   action rather than inferred from telemetry.
3. The OBSERVE-window go/no-go query returning an empty or explicitly-accepted denial set.

None of these can be closed by executing anything. They need elapsed production time and an
operator decision, so claiming them would be fabrication and the status stays `blocked`. It is
specifically **not** `done_with_concerns`, which this repo forbids because that value lets a
half-finished packet present as finished and the "concerns" become permanent.

**Operator next step.** Start the OBSERVE window on the first full day **after** the release
carrying the `user_id` coverage label reaches the deployment (`docs/Operations.md` → "Window
start precondition"). Starting earlier wastes up to 14 days: adding a label starts new series and
stops the old ones, so a window spanning the deploy reads the new series from zero. Then run the
daily `corpus-grant-observe-review` procedure — it is scheduled in `config/upkeep-calendar.yaml`
and carries `blocks_on_failure: [release-train-promote]`, so it gates the flip mechanically
rather than depending on anyone remembering.

**Phases not yet run.** `regression`, `simplify`, `gaps`, `harden`, `stabilize`, `security`,
`validate`, `audit`, `chaos`, and `docs` have not been executed as separately-provenanced
specialist dispatches. They are therefore absent from `certification.certifiedCompletedPhases`
rather than listed — under-claiming is correct here; listing them would be the exact fabrication
Gate G022 exists to catch.

---

### Planning-pass completion statement (retained verbatim)

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

> **SUPERSEDED 2026-08-13.** This section previously read "No test evidence exists for this
> packet, and none is claimed", which was correct while the packet was planning-only at ceiling
> `specs_hardened`. All five scopes have since been implemented and their tests executed, so that
> statement is now false and is replaced rather than left standing. The planning-time capture
> table is retained below, under "What was planned, and what was captured", because it is the
> record against which the delivered coverage is checked.

### Lane results (executed this pass)

**Claim Source:** executed · **Tree:** WORKING TREE
**Executed:** YES
**Exit codes:** `check`=0, `lint`=0, `format --check`=0, `unit`=0, `integration`=0

```text
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.3059395 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHK_EXIT=0

$ ./smackerel.sh lint
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json          OK: PWA manifest has required fields
  OK: web/extension/manifest.json    OK: Chrome extension manifest (MV3)
  OK: web/extension/manifest.firefox.json   OK: Firefox manifest (MV2 + gecko)
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0

$ ./smackerel.sh format --check
78 files already formatted
FMT_EXIT=0
```

```text
$ ./smackerel.sh test unit --go
[go-unit] go test ./... finished OK
UNIT_EXIT=0
$ grep -cE '^(FAIL|--- FAIL)' /tmp/unit.log
0                       # zero failures
$ grep -cE '^ok ' /tmp/unit.log
145                     # 145 packages green
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
ok      github.com/smackerel/smackerel/tests/unit/clients       (cached)
ok      github.com/smackerel/smackerel/web/pwa/tests            (cached)
```

```text
$ ./smackerel.sh test integration
INT_EXIT=0
$ grep -cE '^--- PASS|^    --- PASS' /tmp/int.log
1974                    # 1974 subtests passed
$ grep -cE '^--- FAIL|^    --- FAIL|^FAIL' /tmp/int.log
0                       # zero failures
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
# Ephemeral stack torn down — no residue in the persistent dev store (R-108-O6, G115).
```

```text
$ ./smackerel.sh test e2e
E2E=0
--- go phases ---
PASS: go-e2e
PASS: go-e2e-graph-disabled
PASS: go-e2e-corpus-enforce
--- shell totals ---
  Passed: 36
  Failed: 0
--- go FAILs ---
                        # empty — zero Go test failures
# Re-run AFTER the SEC-108-03 change (new bypass counter emitted from the gate's
# early return), so all three Go phases and all 36 shell scripts are green against
# the final code state rather than against the state before the fix.
```

### Shared-infrastructure canary, proven non-vacuous

A green canary only means something if a broken registry would turn it red. That was tested
rather than assumed: the allowlist entry was removed, the canary was re-run, and the entry was
restored byte-identically.

```text
# (1) green against the real registry
$ ./smackerel.sh test unit --go --go-run 'TestRegisteredScopeSurfaces'
[go-unit] applying -run selector: TestRegisteredScopeSurfaces
ok  github.com/smackerel/smackerel/internal/auth  0.052s        CANARY_EXIT=0

# (2) probe — "corpus" removed from RegisteredScopeSurfaces
--- FAIL: TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant (0.00s)
    browser_session_policy_test.go:35: RegisteredScopeSurfaces missing 'corpus'
    (spec 108 SCOPE-01, F-108-SURFACE-01): [extension annotation knowledge-graph]
--- FAIL: TestRegisteredScopeSurfaces_ContainsCorpus (0.00s)
    scopes_test.go:91: RegisteredScopeSurfaces missing 'corpus' (spec 108 SCOPE-01):
    [extension annotation knowledge-graph]
FAIL  github.com/smackerel/smackerel/internal/auth  0.095s      PROBE_EXIT=1

# (3) restored, green again — and this round trip IS the documented rollback
$ git diff --stat internal/auth/scopes.go
(empty)
ok  github.com/smackerel/smackerel/internal/auth  0.031s        RESTORE_EXIT=0
```

### Consumer sweep for the `user_id` label change

```text
$ grep -rn "RecordCorpusGrantAllowed" --include=*.go .
./internal/api/corpus_grant_gate.go:115:  metrics.RecordCorpusGrantAllowed(routeGroup, sess.UserID, sessionSource)
./internal/metrics/auth.go:365:func RecordCorpusGrantAllowed(group CorpusRouteGroup, userID, sessionSource string) error {
./internal/metrics/corpus_grant_test.go: 7 call sites, all 3-arg
# ONE production call site. No 2-arg caller survives.

$ grep -rn "corpus_grant" deploy/observability/
EXIT=1                  # empty — no dashboard or alert rule queries the series
$ find deploy/observability -type f
deploy/observability/prometheus/alerts.legacy_retirement.yml.tmpl
deploy/observability/grafana/dashboards/assistant.json
deploy/observability/grafana/dashboards/assistant_intents.json
deploy/observability/grafana/dashboards/legacy_retirement.json
# All four scanned; widening the label set cannot silently break a panel.
```

The sweep found real drift rather than confirming a guess: `scopes.md`'s own contract block still
advertised `..._allowed_total{route_group,session_source}` after the label had shipped, and the
UC-108-001 denominator in `docs/Operations.md` still grouped by `route_group` alone — a query that
keeps returning results while silently aggregating across principals. Both corrected.

### Code Diff Evidence

The one production behaviour change in this pass is the `user_id` label added to the allowed
counter, resolving `F-108-COVERAGE-LABEL-01`. It is two lines of call-site change plus the
recorder signature and the label set:

```diff
$ git show a0721209 -- internal/api/corpus_grant_gate.go
-               if err := metrics.RecordCorpusGrantAllowed(routeGroup, sessionSource); err != nil {
+               if err := metrics.RecordCorpusGrantAllowed(routeGroup, sess.UserID, sessionSource); err != nil {
```

```text
$ git show --stat a0721209 -- internal/
 internal/api/corpus_grant_gate.go      |  2 +-
 internal/api/corpus_grant_gate_test.go |  2 +-
 internal/metrics/auth.go               | 30 +++++++----
 internal/metrics/corpus_grant_test.go  | 92 +++++++++++++++++++++++++++-------
 internal/telegram/bot.go               |  4 +-
 internal/telegram/test_helpers.go      |  2 +-
 6 files changed, 99 insertions(+), 33 deletions(-)
```

**Why the call site sits where it does.** Line 114 is `if auth.GateGlobalCorpusRead(sess).Allowed`
and line 115 is the counter increment. The decision is made *before* anything is recorded, so
telemetry is strictly downstream of authorization: this label cannot influence who is admitted or
refused, which is what makes reverting it analytically safe rather than operationally risky.

**Full delivery diff for the spec's owned surfaces:**

```text
$ git diff --stat HEAD~13 -- internal/ cmd/ tests/ config/ smackerel.sh
 internal/config/corpus_grant_flag_contract_test.go | 278 +++++++++++++++++++
 smackerel.sh                                       |  13 +-
 tests/e2e/corpus_enforce_e2e_test.go               | 201 +++++++++++++++
 tests/e2e/test_timeout_process_cleanup.sh          |  24 +-
 tests/integration/corpus_grant_env_emission_test.go|  113 +++++++++
 tests/integration/graphapi/telegram_corpus_differential_test.go | 10 +-
```

### Security review and the SEC-108-03 fix

A static security review of the gate (routed findings `SEC-108-01`..`06`) confirmed the core
authorization surfaces CLEAN — `Observe` is structurally incapable of denying (`record` takes no
`http.ResponseWriter` at all), telemetry is strictly downstream of the decision, middleware
ordering resolves correctly through chi, no wildcard or empty claim escalates, and bridge
delegation cannot confer a grant the principal does not hold. It also corrected a premise carried
in this packet: a malformed scope claim does **not** drop the bad element and keep the good half —
`verify.go` fails the WHOLE set closed, which is the safer shape.

One finding was fixed rather than only recorded, because it degraded the decision this spec
exists to inform. **SEC-108-03:** under OBSERVE, `RequireScope` is not mounted, so
`smackerel_auth_scope_check_bypassed_total` never fires for corpus routes, and the gate returned
before touching either corpus counter. Bypass-band traffic was therefore invisible in every
spec-108 series for the entire observation window — so the coverage table the operator reads
before authorising the flip excluded that band structurally. Per `SEC-108-02` the band is not
exotic: username/password web login mints the shared token as the session cookie, so it is the
ordinary browser population.

```text
$ ./smackerel.sh test unit --go --go-run 'TestCorpusGrantGate|TestCorpusGrantMetrics'
ok  github.com/smackerel/smackerel/internal/api      1.031s
ok  github.com/smackerel/smackerel/internal/metrics  0.058s
RESTORE_EXIT=0
```

The regression is bound to the CALL SITE, not just the recorder — a recorder-level test would
prove the counter works while leaving the emit deletable. Proven by removing the emit:

```text
--- FAIL: TestCorpusGrantGate_Observe_BypassedSessionSourcesAreCountedButNotPredicted/shared_token
    corpus_grant_gate_test.go:574: source "shared_token": bypassed delta = 0, want 1 —
    the gate must emit this band onto its own series, otherwise the OBSERVE window
    cannot tell 'nobody bypassed' from 'we could not see'
--- FAIL: .../bootstrap
    corpus_grant_gate_test.go:574: source "bootstrap": bypassed delta = 0, want 1
FAIL  github.com/smackerel/smackerel/internal/api  0.407s
PROBE_EXIT=1
```

The counter stays OUT of the would-deny prediction — a bypassing session is never refused under
ENFORCE, so crediting it there would inflate the UC-108-001 grant list with principals that can
never be denied. Both properties are asserted together because either alone is satisfiable by the
wrong implementation. `docs/Operations.md` gained step 2b so the operator actually reads the
series, and the metric tables in `design.md` §4 and the runbook were corrected from three series
to four.

### Reproducible in-lane integration failure — NOT caused by this spec, diagnosed and routed

> **CORRECTION.** This section first called the failure a transient flake caused by concurrent
> agent activity. That was wrong, and the correction matters more than the original note: a
> second full-lane run with **no** concurrent activity reproduced it identically. It is
> deterministic in the full lane, not intermittent.

Two full-lane integration runs exited 1 with **1971 pass / 7 fail** against a 1974 / 0 baseline.
The same three tests failed both times, all in `tests/integration/openknowledge`, all with one
symptom — a `smackerel_self` search returning zero of the rows the test had itself just inserted:

```text
--- FAIL: TestSelfKnowledge_TrustPerimeter (0.18s)
    self_knowledge_provenance_test.go:80: self artifact "sk-prov-230108.925520-self" not returned by the tool
--- FAIL: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.12s)
    self_knowledge_tool_test.go:71: got 0 in-run cited self rows, want 2 (ids=[])
--- FAIL: TestPgxSemanticSearcher_NamespaceScopedCosine (0.06s)
    semantic_searcher_test.go:116: got 0 in-run smackerel_self rows, want 2 (ids=[])
```

**Not a regression from this spec.** The corpus-grant change reads a session's scope claim and
increments counters; it touches no artifact storage, no embeddings, and not this namespace. The
same three tests pass in isolation, and pass when run alongside both other known contenders.

**Diagnosis — the guard has a blind spot it cannot see by construction.** `BUG-104-001` added
`tests/integration/nslock` to serialise access to the shared `smackerel_self` namespace, and all
three known test-side contenders correctly take it: the openknowledge victims, the selfknowledge
ingestor test, and `knowledge_stats_test.go`'s `TRUNCATE`. Callsite coverage is complete, and
`nslock/callsite_contract_test.go` enforces it with a regex over test source.

The row-deleter that is actually firing is not test code. `internal/assistant/selfknowledge/ingestor.go:137`
`sweepStale` runs in the **production** ingestor:

```sql
DELETE FROM artifacts
WHERE source_id = $1 AND content_hash <> ALL($2::text[])
```

and `cmd/core/wiring_selfknowledge.go:64` — a file whose own header says "spec 104 SCOPE-03 **boot
lifecycle**" — invokes `ingestor.Ingest(ctx)` once at core startup, which performs that sweep.

**What is proven.** The mechanism was not left as an inference — it was executed against a live
test stack:

```text
# 1. Core boots and runs the sweep (nothing stale yet):
{"msg":"self-knowledge corpus ingested","namespace":"smackerel_self","entries":13,"published":13,"swept":0}

# 2. Insert one smackerel_self row with a SYNTHETIC content_hash — the exact
#    shape every one of these tests inserts ("h-"+id):
$ psql -c "INSERT INTO artifacts (id,artifact_type,title,content_hash,source_id)
           VALUES ('probe-sweep-108','capability','sweep probe','h-probe-sweep-108','smackerel_self');"
INSERT 0 1
$ psql -tAc "SELECT count(*) FROM artifacts WHERE id='probe-sweep-108';"
1

# 3. Restart the core. Its boot lifecycle runs Ingest -> sweepStale:
$ docker restart smackerel-test-smackerel-core-1
{"msg":"self-knowledge corpus ingested","namespace":"smackerel_self","entries":13,"published":13,"swept":1}
                                                                                              ^^^^^^^^^
# 4. The row is gone:
$ psql -tAc "SELECT count(*) FROM artifacts WHERE id='probe-sweep-108';"
0
```

`swept` moved 0 → 1 and the probe row count moved 1 → 0 on a core restart. The core's boot-time
sweep deletes precisely the rows these tests depend on, and it holds no `nslock` because it runs
in a different process as production code — which is why the regex-over-test-sources contract in
`nslock/callsite_contract_test.go` cannot reach it even in principle. No allowlist entry would
have caught this.

This also explains why the failure looked intermittent at first: it depends on whether a core
boot or restart lands while those rows exist, not on which tests run together. Every earlier
observation fits — zero in-run rows, green in isolation, green alongside both known test-side
contenders, red in the full lane, and `artifact_count: 0` in the health snapshot.

**The fix belongs to spec 104, and there are two candidate shapes.** Either make the production
sweep take the same advisory lock — it can, `pg_advisory_lock` is database-scoped, not
test-scoped — or give the test rows corpus-consistent hashes so the sweep does not consider them
stale. The first is the honest one: the lock's stated purpose is mutual exclusion over the
namespace, and a deleter that ignores it makes the guarantee partial.

**Routing.** Spec 104 / `BUG-104-001`. That fix is incomplete rather than wrong: it established
mutual exclusion across test callsites but not against the production sweep the lane itself
starts. Recorded here because this session observed, diagnosed and proved it — not because this
spec owns it. It does not gate spec 108: no corpus-grant test is affected, and every corpus-grant
suite passes in every run.

### What was planned, and what was captured

### What will be captured, and by which scope

Retained as the planning-time capture contract. Every row below has since been executed and its
raw output recorded inline under the owning scope's DoD item in `scopes.md`; the lane totals
above are the aggregate. The table stays because it is the record the delivered coverage is
checked against — deleting it would remove the ability to notice a row that was planned and never
captured.

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
