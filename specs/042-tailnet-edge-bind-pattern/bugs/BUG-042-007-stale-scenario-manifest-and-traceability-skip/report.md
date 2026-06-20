# Report: BUG-042-007 — Stale scenario-manifest.json + traceability-guard skip

**Closure date:** 2026-06-17
**Mode:** bugfix-fastlane (planning-artifact reconciliation; zero runtime change)
**Execution model:** parent-expanded-child-mode (planning reconcile by `bubbles.plan`; runtime lacks runSubagent)

---

## Summary

BUG-042-007 is a planning-artifact reconcile against
`specs/042-tailnet-edge-bind-pattern/` resolving two coupled findings routed from a
stochastic-quality-sweep harden probe (Round 33). F1: `scenario-manifest.json` was
pre-supersession stale — `SCN-042-001`'s `then` carried the forbidden
`${HOST_BIND_ADDRESS:-127.0.0.1}:` form, `SCN-042-003` was titled "Compose default
is safe for local runs", and `SCN-042-004/005` titles were shuffled. F2: active
`scopes.md` used `- **SCN-042-NNN - title**` headers so `traceability-guard.sh`'s
`extract_scenarios` found 0 scenarios and the run exited 1 silently under `set -e`,
skipping the G057/G059 manifest cross-check.

The fix reformatted the active `SCN-042-001..006` to the working
`Scenario: SCN-042-NNN — title` gherkin form, relabeled the two stale
HTML-commented duplicate `## Scope N:` headings to `## Superseded Scope N —`,
neutralized the `####` DoD subheadings to bold labels, added per-scenario trace-ID
prefixes to DoD items, and realigned all six manifest entries to the active
fail-loud scopes. The deployment surface (`deploy/compose.deploy.yml` +
`internal/deploy/compose_contract_test.go`) was NOT touched and stays GREEN.

## Completion Statement

BUG-042-007 is **resolved**. `traceability-guard.sh specs/042-tailnet-edge-bind-pattern`
now exits 0 with the G057/G059 manifest cross-check ACTIVE (was exit 1 with the
cross-check skipped); `artifact-lint.sh specs/042-tailnet-edge-bind-pattern`
returns PASSED; and all nine `TestComposeContract_*` functions PASS
(`ok internal/deploy`). No forbidden `${HOST_BIND_ADDRESS:-127.0.0.1}` form was
reintroduced. The parent's `done` status and certification are unchanged. The
bugfix-fastlane workflow terminates in `completed_owned`. Per the parent task,
nothing is committed — the work is left in the working tree.

---

## Implementation Code Diff Evidence

This packet is planning-artifact-only — **no `.go`, `.py`, `.yaml` (config),
`.sh`, `.ts`, `.tsx`, `.sql`, `Dockerfile`, or `.github/workflows/*.yml` files are
touched.** `deploy/compose.deploy.yml` and `internal/deploy/compose_contract_test.go`
are unchanged.

### Code Diff Evidence

```text
$ git status --short deploy/compose.deploy.yml internal/deploy/compose_contract_test.go
$ echo "Exit Code: $?"
Exit Code: 0
$ git status --short specs/042-tailnet-edge-bind-pattern/scopes.md specs/042-tailnet-edge-bind-pattern/scenario-manifest.json specs/042-tailnet-edge-bind-pattern/report.md specs/042-tailnet-edge-bind-pattern/state.json
 M specs/042-tailnet-edge-bind-pattern/scopes.md
 M specs/042-tailnet-edge-bind-pattern/scenario-manifest.json
 M specs/042-tailnet-edge-bind-pattern/report.md
 M specs/042-tailnet-edge-bind-pattern/state.json
?? specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip/
$ ls -1 specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip/ | wc -l
8
```

### Manifest realignment + parent recording verification

```text
$ grep -rn 'HOST_BIND_ADDRESS:-' specs/042-tailnet-edge-bind-pattern/scenario-manifest.json
$ echo "Exit Code: $?"
Exit Code: 1
$ python3 -c "import json;d=json.load(open('specs/042-tailnet-edge-bind-pattern/scenario-manifest.json'));print([(s['scenarioId'],s['title'],s['requiredTestType']) for s in d['scenarios']])"
[('SCN-042-001', 'Backend ports require adapter-provided bind address', 'unit'), ('SCN-042-002', 'Infra services have no host port mapping', 'unit'), ('SCN-042-003', 'Missing bind address fails loud', 'unit'), ('SCN-042-004', 'Explicit loopback is supplied, not defaulted', 'unit'), ('SCN-042-005', 'Operations doc explains infra access without host ports', 'doc-lint'), ('SCN-042-006', 'Copilot guardrail prevents fallback regression', 'doc-lint')]
$ python3 -c "import json;d=json.load(open('specs/042-tailnet-edge-bind-pattern/state.json'));print([b['bugId'] for b in d['resolvedBugs']])"
['BUG-042-001-scope-status-reconciliation', 'BUG-042-007-stale-scenario-manifest-and-traceability-skip']
```

---

## Test Evidence (Scenario-First Red→Green Proof)

**RED phase (pre-fix): the guard finds 0 scenarios and skips the cross-check.**

```text
$ grep -cE '^[[:space:]]*Scenario( Outline)?:' specs/042-tailnet-edge-bind-pattern/scopes.md
0
$ bash .github/bubbles/scripts/traceability-guard.sh specs/042-tailnet-edge-bind-pattern
--- Scenario Manifest Cross-Check (G057/G059) ---
ℹ️  No scope-defined Gherkin scenarios found — scenario manifest cross-check skipped
ℹ️  Checking traceability for Scope 1: Compose contract + Go unit lint test + SST clarifying comment
$ echo "Exit Code: $?"
Exit Code: 1
```

**GREEN phase (post-fix): 6 scenarios extract, the cross-check is ACTIVE, exit 0.**

```text
$ grep -cE '^[[:space:]]*Scenario( Outline)?:' specs/042-tailnet-edge-bind-pattern/scopes.md
6
$ bash .github/bubbles/scripts/traceability-guard.sh specs/042-tailnet-edge-bind-pattern
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 6 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
✅ scenario-manifest.json records evidenceRefs
--- Gherkin → DoD Content Fidelity (Gate G068) ---
ℹ️  DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped
--- Traceability Summary ---
ℹ️  Scenarios checked: 6
ℹ️  Scenario-to-row mappings: 6
ℹ️  Concrete test file references: 6
ℹ️  Report evidence references: 6
RESULT: PASSED (0 warnings)
$ echo "Exit Code: $?"
Exit Code: 0
```

**Deployment surface intact (nine TestComposeContract_* functions PASS):**

```text
$ ./smackerel.sh test unit --go --go-run 'TestComposeContract' --verbose
--- PASS: TestComposeContract_LiveFile (0.00s)
--- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
--- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
--- PASS: TestComposeContract_AdversarialOllamaLiteralBind (0.00s)
--- PASS: TestComposeContract_AdversarialDefaultFallbackBind (0.00s)
--- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.045s
COMPOSE_TEST_EXIT=0
```

---

## Recertification Guards

### Validation Evidence

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
✅ All 4 scope(s) in scopes.md are marked Done
✅ workflowMode gate satisfied: ### Validation Evidence
✅ workflowMode gate satisfied: ### Audit Evidence
✅ All 115 evidence blocks in report.md contain legitimate terminal output
Artifact lint PASSED.
$ echo "Exit Code: $?"
Exit Code: 0
```

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip
Artifact lint PASSED.
$ echo "Exit Code: $?"
Exit Code: 0
```

### Audit Evidence

```text
$ grep -rn 'HOST_BIND_ADDRESS:-' specs/042-tailnet-edge-bind-pattern/scenario-manifest.json
$ echo "manifest forbidden-form hits: $?"
manifest forbidden-form hits: 1
$ grep -nE '^## Scope [0-9]+:' specs/042-tailnet-edge-bind-pattern/scopes.md
85:## Scope 1: Fail-loud compose contract and mechanical guard
321:## Scope 2: Operator docs and agent guardrails
$ git status --short specs/042-tailnet-edge-bind-pattern/ | grep -vE '^\?\? |^ M ' | wc -l
0
```

Audit verdict: the only `:-127.0.0.1` references remaining in spec 042 are
pre-existing forbidden-labeled evidence and the HTML-commented superseded block
(not in the active gherkin or the realigned manifest — the manifest scan exits 1 /
no match); exactly two active `## Scope N:` headings remain (the stale duplicates
are relabeled `## Superseded Scope N —`); the change set is 100% under
`specs/042-tailnet-edge-bind-pattern/` with `deploy/compose.deploy.yml` and
`internal/deploy/compose_contract_test.go` unchanged; nothing is committed. Spec 042
is internally consistent: `done` + traceability-guard PASSED (cross-check ACTIVE) +
artifact-lint PASSED + nine `TestComposeContract_*` GREEN.
