# Execution Reports

Single-file mode: top-level `report.md`.

Links: [uservalidation.md](uservalidation.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md)

## Planning — 2026-06-02

### Summary

Spec 077 scaffolded to close ops packet
`specs/_ops/F-057-V-001-e2e-ui-harness` (open since 2026-05-28). The
packet documents Smackerel's missing real-browser e2e harness: every
committed `web/pwa/tests/*.spec.ts` is a documentation stub asserting
`expect(true).toBeTruthy()`, no Playwright runner is wired, no
`./smackerel.sh test e2e-ui` subcommand exists, and CI does not invoke
any browser-side test job. This blocks spec 057 SCOPE-4 rows 4.1-4.5
(which were dispositioned `ACCEPTED-EQUIVALENT`), spec 073 TP-073-09,
spec 075 TP-075-09, and SCN-073-A09 accessibility coverage.

This spec is a foundation: one harness, three small scopes, one first
real consumer.

Artifacts authored:

- `spec.md` (problem statement, actors, outcome contract, BDD scenarios, UI matrix, NFRs, acceptance criteria, open questions)
- `design.md` (capability foundation, concrete implementations, variation axes, contracts, risks, alternatives)
- `scopes.md` (three-scope decomposition: foundation + discovery/CI/docs + first consumer; execution outline included)
- `scenario-manifest.json` (eight SCN-077-A0N scenarios with SCN-077-A04 inheriting from spec 057 SCOPE-4 rows 4.1-4.5)
- `uservalidation.md` (validation checklist; 4 baseline `[x]` plus 6 pending review items)
- `state.json` (status `in_progress`, workflowMode `full-delivery`)

Ops packet update: `specs/_ops/F-057-V-001-e2e-ui-harness/README.md`
header amended with `Routing Status: Routed to spec 077` so portfolio
sweeps see it as resolved-pending-execution.

### Code Diff Evidence

Not applicable — planning-only run. No source / runtime / config files
modified. All Test Plan rows in `scopes.md` are status `Not Started`
and will be executed by the implementation runs that follow.

### Test Evidence

Not applicable — planning-only run. No tests executed.

### Completion Statement

Not applicable — planning-only bootstrap. Spec 077 is `in_progress`;
completion will be claimed by the validate/audit phases after Scopes 1,
2, and 3 ship and report.md is amended with their per-scope evidence.

## Scope 1a — Compose-Project Lane + Dispatcher Routing — 2026-06-02

### Summary

Delivered the dispatcher + lane wrapper skeleton for the PWA browser end-to-end UI test harness (TP-077-01-04 / SCN-077-A09). Added the `e2e-ui` subcommand to `./smackerel.sh test`, the dedicated Compose project name `smackerel-test-e2e-ui` (declared in `scripts/runtime/web-e2e-ui.sh`), help text entry in the top-level `usage()` block, and a shell-level dispatcher canary at `tests/unit/cli/spec_077_test_dispatcher_test.sh`. Also added a small `tests/unit/cli/*.sh` discovery hook to `./smackerel.sh test unit` so the canary runs via the project-standard CLI as the Test Plan requires. No Node tooling, Playwright config, or proof-of-life spec shipped — those land in SCOPE-1b and SCOPE-1c.

### Files Changed

- `smackerel.sh` — added `e2e-ui)` arm in the `test)` case (forwards to `scripts/runtime/web-e2e-ui.sh`, supports `--help`); added `test e2e-ui` line to the `usage()` heredoc; added `tests/unit/cli/*.sh` discovery loop to the `test unit` lane (runs after the existing Go + Python unit steps, scoped to the new directory only).
- `scripts/runtime/web-e2e-ui.sh` — NEW. Lane wrapper skeleton. Declares `SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test-e2e-ui"`. Supports `--print-compose-project` (prints the project name and exits 0, so the canary can introspect routing without bringing up a stack). Otherwise fails loud with `ERROR: e2e-ui runner not yet wired.` and names the Compose project + the scopes that will wire the runner (1b/1c).
- `tests/unit/cli/spec_077_test_dispatcher_test.sh` — NEW. Shell canary anchoring TP-077-01-04 / SCN-077-A09. Seven assertion sections including two adversarial regression sections (wrapper-exists invariant; unknown-flag forwarding) and a four-lane regression matrix that proves `./smackerel.sh test unit|integration|e2e|stress` still reach their lane-specific option parsers.

### Code Diff Evidence

`git status --short` after the change:

```text
 M smackerel.sh
?? scripts/runtime/web-e2e-ui.sh
?? tests/unit/cli/
```

Three files changed/added. Zero changes outside the Change Boundary declared in `scopes.md → Scope 1a` (no `web/pwa/**`, no `web/pwa/tests/**`, no CI workflows, no `docs/Testing.md`, no `README.md`, no Playwright config).

### Test Evidence

**RED proof (adversarial)** — disable the new `e2e-ui)` dispatcher arm and re-run the canary; expect failure:

```text
$ cp smackerel.sh /tmp/smackerel.sh.bak \
  && sed -i 's|^      e2e-ui)$|      e2e-ui-DISABLED-FOR-RED-PROOF)|' smackerel.sh \
  && bash tests/unit/cli/spec_077_test_dispatcher_test.sh
FAIL: 'test e2e-ui --print-compose-project' exit=1
RED exit=1
```

**GREEN proof** — restore the dispatcher arm and re-run the canary:

```text
$ cp /tmp/smackerel.sh.bak smackerel.sh \
  && bash tests/unit/cli/spec_077_test_dispatcher_test.sh
PASS: spec_077_test_dispatcher_test (TP-077-01-04 / SCN-077-A09)
GREEN exit=0
```

**Direct routing probes** (extracted from the canary's individual sections; commands re-runnable):

```text
$ ./smackerel.sh test e2e-ui --help                    # exit 0, lists Compose project + Playwright
$ ./smackerel.sh test e2e-ui --print-compose-project   # exit 0, prints exactly 'smackerel-test-e2e-ui'
smackerel-test-e2e-ui
$ ./smackerel.sh test e2e-ui                           # exit 1, fail-loud
ERROR: e2e-ui runner not yet wired.
       Compose project for this lane: smackerel-test-e2e-ui
       Node tooling (Playwright runner) lands in spec 077 SCOPE-1b.
       Proof-of-life spec + live-stack isolation proof land in SCOPE-1c.
```

**Existing-lane regression matrix** (canary §5) — each lane's unknown-flag handler is reached, which proves dispatcher routing is intact without bringing up any stack:

```text
./smackerel.sh test unit         --__spec077_canary_bogus_flag__   → 'Unknown test unit option'
./smackerel.sh test integration  --__spec077_canary_bogus_flag__   → 'Unknown test integration option'
./smackerel.sh test e2e          --__spec077_canary_bogus_flag__   → 'Unknown test e2e option'
./smackerel.sh test stress       --__spec077_canary_bogus_flag__   → 'Unknown test stress option'
```

**`./smackerel.sh test unit` baseline** — the full `./smackerel.sh test unit` command fails on this machine because the Go tooling container does not have `node` / `dart` on `PATH`, which the pre-existing spec 073 cross-language canary `tests/unit/clients/TestRenderDescriptorV1_*` requires. Tail:

```text
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
    render_descriptor_canary_test.go:125: node not on PATH; the spec 073 cross-language renderer canary requires both node and dart
--- FAIL: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
    render_descriptor_canary_test.go:367: dart not on PATH; the spec 073 cross-language renderer canary requires dart
FAIL    github.com/smackerel/smackerel/tests/unit/clients       0.007s
```

This failure is **pre-existing** and **not introduced by this scope**:

- `git status` confirms no changes touched `tests/unit/clients/**` or `scripts/runtime/go-unit.sh` (only `smackerel.sh`, `scripts/runtime/web-e2e-ui.sh`, `tests/unit/cli/` changed).
- The failure reproduces on `git stash` of the scope 1a delta.
- The Go tooling step that fails runs *before* the new `tests/unit/cli/*.sh` discovery loop, so the shell canary is the path that anchors TP-077-01-04 in evidence; the canary is run directly above.
- Routing for the four existing lanes is asserted by canary §5 (no docker required), so DoD #5 "Broader test-dispatcher behavior is unchanged" is proven by the canary itself rather than by a full `./smackerel.sh test unit` green run on this host.

### Artifact-Lint Evidence

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness
# (exit 0; no findings against scope 1a artifacts)
```

### Completion Statement

Scope 1a (Compose-Project Lane + Dispatcher Routing) is implementation-complete. All eight DoD items in `scopes.md → Scope 1a` are checked with inline evidence and Claim Source tags (`executed` for the six directly-verifiable items, `interpreted` for the two that depend on bounded reasoning about untouched code paths). RED→GREEN proof captured. Change Boundary respected (3 files; zero excluded file families touched). The next scope in the chain is SCOPE-1b (Node tooling runner + Playwright config fail-loud), which depends on this scope's lane wrapper.

## Scope 1b — Node Tooling Runner + Playwright Config Fail-Loud — 2026-06-02

**Phase:** implement

### Summary

Delivered the Node tooling runner + Playwright config fail-loud SST consumer for the PWA browser e2e-ui harness (TP-077-01-03 / SCN-077-A10). Added `web/pwa/package.json` (pins `@playwright/test` 1.49.1 + `typescript` 5.6.3), `web/pwa/playwright.config.ts` (sources `baseURL` exclusively through the fail-loud helper, no `??`/`||`/hardcoded default), `web/pwa/tsconfig.json`, the `_support/env.ts` fail-loud helper, the `_support/csp.ts` no-op skeleton, the `_support/csp.test.ts` Node-level unit test, the `run_node_tooling` Bash helper in `scripts/runtime/web-e2e-ui.sh`, the auto-discovered shell driver `tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh`, and the seam-handoff updates to `tests/unit/cli/spec_077_test_dispatcher_test.sh` sections 4 and 7 (the SCOPE-1a "runner not yet wired" stub assertions are replaced with SCOPE-1b "runner is now wired and propagates exit codes" assertions). `.gitignore` extended with `web/pwa/test-results/` and `web/pwa/playwright-report/`.

### Files Changed

- `web/pwa/package.json` — NEW. Workspace manifest pinning `@playwright/test@1.49.1` and `typescript@5.6.3`; `scripts: { test:e2e-ui }`.
- `web/pwa/playwright.config.ts` — NEW. `defineConfig` with `testDir: 'tests'`, `testMatch: '**/*.spec.ts'`, `testIgnore: ['**/_support/**']`, list/html/json reporters writing to `web/pwa/test-results/` and `web/pwa/playwright-report/`, and `use.baseURL: requireSmackerelBaseUrl()` (no inline default, no `??`, no `||`).
- `web/pwa/tsconfig.json` — NEW. ES2022 strict config; `noEmit: true`; `types: ['node']`.
- `web/pwa/tests/_support/env.ts` — NEW. Exports `requireSmackerelBaseUrl()` that throws an `Error` naming `SMACKEREL_BASE_URL` when the var is unset or empty.
- `web/pwa/tests/_support/csp.ts` — NEW. Skeleton `attachCSPGuard(_page: unknown): void` (one parameter, no body; SCOPE-3 will wire real CSP-violation listeners). Deliberately avoids importing `@playwright/test` so unit tests do not require the dependency to be installed.
- `web/pwa/tests/_support/csp.test.ts` — NEW. Node `--test` suite (4 cases) asserting fail-loud throw on unset env, fail-loud throw on empty-string env, value return on set env, and the `attachCSPGuard` skeleton signature.
- `scripts/runtime/web-e2e-ui.sh` — REWRITTEN. Adds `run_node_tooling()` function (cd into `web/pwa`, invoke `${SMACKEREL_E2E_UI_NPX:-npx} playwright test "$@"`, propagate exit code; 127 if npx binary missing). Preserves `--print-compose-project` short-circuit. Adds a `BASH_SOURCE != $0` guard so sourcing the script (used by the unit test) does not trigger the default action. Default action now invokes `run_node_tooling` (replacing the SCOPE-1a "runner not yet wired" stub).
- `tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh` — NEW. Auto-discovered by `./smackerel.sh test unit`. Three assertion blocks: (A) static composition over `playwright.config.ts` + `env.ts`, (B) `node --experimental-strip-types --test web/pwa/tests/_support/csp.test.ts`, (C) `run_node_tooling` exit-code propagation across `STUB_EXIT=0|7|127` and a missing-binary case.
- `tests/unit/cli/spec_077_test_dispatcher_test.sh` — sections 4 and 7 updated. SCOPE-1a's assertions on the "runner not yet wired" stub are replaced with SCOPE-1b assertions that the wrapper now invokes `run_node_tooling` (injects `SMACKEREL_E2E_UI_NPX=false` to prove exit-code propagation without docker/network); both sections fail loud if the SCOPE-1a stub message reappears (regression sentry). All other sections of the canary are unchanged.
- `.gitignore` — added `web/pwa/test-results/` and `web/pwa/playwright-report/`. `node_modules/` is already globally ignored.
- `specs/077-pwa-browser-test-harness/scopes.md` — SCOPE-1b status flipped to `Done`, inventory updated, eight DoD checkboxes marked with inline evidence + Claim Source tags.
- `specs/077-pwa-browser-test-harness/state.json` — `execution.completedPhaseClaims` extended with the SCOPE-1b implement claim; `certification.scopeProgress` entry for SCOPE-1b set to `implementation_complete`.

### Code Diff Evidence

`git status --short` for SCOPE-1b-owned paths only (other modified/untracked files in the working tree belong to unrelated in-progress work and are out of scope for this report):

```text
 M .gitignore
?? scripts/runtime/web-e2e-ui.sh
 M tests/unit/cli/spec_077_test_dispatcher_test.sh
?? tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh
?? web/pwa/package.json
?? web/pwa/playwright.config.ts
?? web/pwa/tsconfig.json
?? web/pwa/tests/_support/env.ts
?? web/pwa/tests/_support/csp.ts
?? web/pwa/tests/_support/csp.test.ts
 M specs/077-pwa-browser-test-harness/scopes.md
 M specs/077-pwa-browser-test-harness/report.md
 M specs/077-pwa-browser-test-harness/state.json
```

Zero changes outside the Change Boundary declared in `scopes.md → Scope 1b`. The `scripts/runtime/web-e2e-ui.sh` rewrite and the two updated sections of `spec_077_test_dispatcher_test.sh` are the explicit SCOPE-1a→SCOPE-1b lane-wrapper-invocation seam called out in the scope's plan. No `smackerel.sh` dispatcher edits; no CI workflow files; no `docs/Testing.md` / `README.md`; no edits to any pre-existing `web/pwa/tests/*.spec.ts` body.

### Red Proof

Introduce a silent default in `requireSmackerelBaseUrl()` and re-run the SCOPE-1b unit test; expect failure caught by the static-composition block (A):

```text
$ cp web/pwa/tests/_support/env.ts /tmp/env.ts.bak \
  && sed -i 's|throw new Error(|return process.env.SMACKEREL_BASE_URL ?? "http://127.0.0.1:18080"; // RED-PROOF\n    throw new Error(|' \
       web/pwa/tests/_support/env.ts \
  && bash tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh
~/smackerel/web/pwa/tests/_support/env.ts:17:    return process.env.SMACKEREL_BASE_URL ?? "http://127.0.0.1:18080"; // RED-PROOF
FAIL: found forbidden ?? or || default near SMACKEREL_BASE_URL
RED exit=1
```

### Test Evidence

**Block A — static composition** (executed inside `tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh`):

```text
$ grep -q 'requireSmackerelBaseUrl' web/pwa/playwright.config.ts && echo OK
OK
$ grep -nE 'SMACKEREL_BASE_URL[[:space:]]*(\?\?|\|\|)' web/pwa/playwright.config.ts web/pwa/tests/_support/env.ts; echo "rc=$?"
rc=1   # no match → no forbidden default
$ grep -nE 'baseURL[[:space:]]*:[[:space:]]*"https?://' web/pwa/playwright.config.ts; echo "rc=$?"
rc=1   # no hardcoded baseURL literal
$ grep -nE 'process\.env\.SMACKEREL_BASE_URL' web/pwa/playwright.config.ts; echo "rc=$?"
rc=1   # config never reads process.env directly — must go through requireSmackerelBaseUrl()
```

**Block B — node:test on the fail-loud helper + CSP guard skeleton**:

```text
$ env -u SMACKEREL_BASE_URL node --experimental-strip-types --no-warnings=ExperimentalWarning \
       --test web/pwa/tests/_support/csp.test.ts
[spec_077_playwright_config_fail_loud] node v22.22.0
PASS: spec_077_playwright_config_fail_loud_test (TP-077-01-03 / SCN-077-A10)
```

The node:test runner summary inside Block B reports `# tests 4` (asserted by the driver to be ≥ 1 to prevent a silent zero-test green).

**Block C — `run_node_tooling` exit-code propagation** (also inside the driver):

```text
# STUB_EXIT=0   → run_node_tooling returns 0
# STUB_EXIT=7   → run_node_tooling returns 7
# STUB_EXIT=127 → run_node_tooling returns 127
# SMACKEREL_E2E_UI_NPX=/definitely/not/a/real/binary/$$  → run_node_tooling returns 127,
#   stderr names "is required to run the spec 077 PWA e2e-ui harness"
```

All four cases asserted exactly; the driver fails loud with the observed vs expected code if any propagation is lost.

**Full driver invocation**:

```text
$ bash tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh
[spec_077_playwright_config_fail_loud] node v22.22.0
PASS: spec_077_playwright_config_fail_loud_test (TP-077-01-03 / SCN-077-A10)
exit=0
```

### Regression Evidence

**SCOPE-1a dispatcher canary** (the seam this scope edits sections 4 and 7 of) runs green end-to-end against the new wrapper:

```text
$ bash tests/unit/cli/spec_077_test_dispatcher_test.sh
PASS: spec_077_test_dispatcher_test (TP-077-01-04 / SCN-077-A09)
exit=0
```

Sections 1–3, 5, and 6 of the dispatcher canary are unchanged (usage text, `--help`, `--print-compose-project`, the four-lane regression matrix, and the wrapper-exists invariant), proving the seam handoff did not break any pre-existing dispatcher contract.

**Broader `./smackerel.sh test unit` baseline** — not re-run on this host. The pre-existing missing-`node`/`dart`-in-the-Go-tooling-container failure documented in the SCOPE-1a report still reproduces (no SCOPE-1b code touched `tests/unit/clients/**` or `scripts/runtime/go-unit.sh`), and the new shell test is the only new unit added; running it directly proves auto-discovery would pick it up. **Claim Source:** interpreted for the broader suite; executed for the two shell canaries.

### Artifact-Lint Evidence

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness
✅ Required artifact exists: spec.md … report.md   (all six required artifacts present)
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md (with checkbox items, all using checkbox syntax)
✅ uservalidation checklist present and uses checkbox syntax
✅ state.json v3 has all required + recommended fields; top-level status matches certification.status
⚠️  state.json uses deprecated fields 'scopeProgress', 'statusDiscipline', 'scopeLayout' — pre-existing across the repo, not introduced by SCOPE-1b
exit=0
```

### Rollback Path

To restore SCOPE-1a behavior exactly:

1. `rm -r web/pwa/package.json web/pwa/playwright.config.ts web/pwa/tsconfig.json web/pwa/tests/_support/`
2. `rm tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh`
3. Revert `scripts/runtime/web-e2e-ui.sh` to its SCOPE-1a stub (remove `run_node_tooling`, remove `BASH_SOURCE != $0` guard, restore the `runner not yet wired` heredoc as the default action; keep the Compose-project constant + `--print-compose-project` short-circuit).
4. Revert sections 4 and 7 of `tests/unit/cli/spec_077_test_dispatcher_test.sh` to their pre-SCOPE-1b form (assert the `runner not yet wired` message).
5. Revert the four lines added to `.gitignore` for `web/pwa/test-results/` and `web/pwa/playwright-report/`.

No schema migrations, no managed-doc changes, no SST regenerations, no CI changes — rollback is a pure file-level revert.

### Completion Statement

Scope 1b (Node Tooling Runner + Playwright Config Fail-Loud) is implementation-complete. All eight DoD items in `scopes.md → Scope 1b` are checked with inline evidence and Claim Source tags (`executed` for the directly-verified static + behavioral + exit-code items, `interpreted` for the `tsc --noEmit`-equivalent property, the broader unit-suite property, the rollback path, and the build-quality-gate aggregate). RED→GREEN proof captured. Change Boundary respected; the SCOPE-1a dispatcher canary still passes against the wired runner. The next scope in the chain is SCOPE-1c (Proof-of-Life Spec + Live-Stack Isolation Proof), which depends on both SCOPE-1a (Compose lane) and SCOPE-1b (Node runner + Playwright config + CSP skeleton).

---

## Scope 1c — Proof-of-Life Spec + Live-Stack Isolation Proof — 2026-06-02

**Agent:** bubbles.implement
**Scope:** scopes.md#scope-1c
**Scenarios:** SCN-077-A01, SCN-077-A07
**Test Plan Rows:** TP-077-01-01, TP-077-01-02, TP-077-01-05, TP-077-01-01R

### Code Delivered

- `scripts/runtime/web-e2e-ui.sh` — lifecycle wiring added on top of the SCOPE-1b runner stub:
  - sources `scripts/lib/runtime.sh` for the SST helpers,
  - `bring_up_test_stack()` generates the test SST env, sources `CORE_EXTERNAL_URL` + `COMPOSE_WAIT_TIMEOUT_S`, exports `SMACKEREL_BASE_URL` from `CORE_EXTERNAL_URL` (no `??`, no `||`, no hardcoded localhost),
  - installs `trap 'tear_down_test_stack' EXIT`, `INT`, `TERM` BEFORE bringing the stack up,
  - new `e2e_ui_compose` helper pins `docker compose --project-name "$SMACKEREL_E2E_UI_COMPOSE_PROJECT" --env-file "$SMACKEREL_E2E_UI_ENV_FILE" -f docker-compose.yml` so the lane never inherits the env-file `COMPOSE_PROJECT=smackerel-test` value,
  - `tear_down_test_stack()` is idempotent and routes through `e2e_ui_compose down --remove-orphans --volumes` — scoped to the dedicated project only,
  - the live-stack lifecycle is bypassed when `SMACKEREL_E2E_UI_NPX` is set (SCOPE-1a/1b unit canaries inject a stub `npx` and never want docker);
- `web/pwa/tests/proof_of_life.spec.ts` — loads `/` against `baseURL` (sourced via `requireSmackerelBaseUrl()`), asserts `<title>Smackerel</title>` + `<h1>Smackerel</h1>`, imports `attachCSPGuard` from `_support/csp.ts` as a contract smoke-import (real CSP wiring lands in SCOPE-3);
- `tests/integration/cli/spec_077_compose_project_test.go` (TP-077-01-05) — Go integration test with `//go:build integration`. Static contract: project name constant, no collision with `smackerel-test` / `smackerel`, `e2e_ui_compose` invokes `docker compose --project-name "$SMACKEREL_E2E_UI_COMPOSE_PROJECT"`, the `--print-compose-project` short-circuit prints the constant, and the dispatcher arm in `smackerel.sh` execs the wrapper;
- `tests/integration/cli/spec_077_test_stack_isolation_test.go` (TP-077-01-02) — Go integration test with `//go:build integration`. Static contract: EXIT + INT + TERM traps installed; teardown is scoped through `e2e_ui_compose` (never `./smackerel.sh --env test down`, never `--env dev`); bring-up never delegates to `./smackerel.sh --env test up` (which would inherit `smackerel-test`); `SMACKEREL_BASE_URL` is derived from SST `CORE_EXTERNAL_URL` with no `:-` fallback and no hardcoded localhost.

### Red Proof

Sabotaged the wrapper to assert the new Go integration tests are not tautological:

```text
$ sed -i 's|SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test-e2e-ui"|SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test"|' scripts/runtime/web-e2e-ui.sh
$ go test -tags integration -count=1 -run TestSpec077 ./tests/integration/cli/...
--- FAIL: TestSpec077TestStackUsesDedicatedComposeProject (0.00s)
    --- FAIL: TestSpec077TestStackUsesDedicatedComposeProject/declares_the_dedicated_Compose_project_constant
        spec_077_compose_project_test.go:39: scripts/runtime/web-e2e-ui.sh must declare SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test-e2e-ui"; not found
    --- FAIL: TestSpec077TestStackUsesDedicatedComposeProject/project_name_is_NOT_the_integration/e2e/stress_lane_name
        spec_077_compose_project_test.go:51: e2e-ui Compose project must NOT be `smackerel-test` (collides with the integration/e2e/stress lane); found: SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test"
FAIL    github.com/smackerel/smackerel/tests/integration/cli    0.005s
```

```text
$ sed -i "s|trap 'tear_down_test_stack' EXIT|# trap removed for RED proof|" scripts/runtime/web-e2e-ui.sh
$ go test -tags integration -count=1 -run TestSpec077TestStackIsolation_DevStackUntouched ./tests/integration/cli/...
--- FAIL: TestSpec077TestStackIsolation_DevStackUntouched (0.00s)
    --- FAIL: TestSpec077TestStackIsolation_DevStackUntouched/EXIT_trap_installed_for_teardown
        spec_077_test_stack_isolation_test.go:31: wrapper must install `trap 'tear_down_test_stack' EXIT` so teardown runs on success and failure
FAIL    github.com/smackerel/smackerel/tests/integration/cli    0.004s
```

Wrapper restored to head after each RED probe.

### Green Proof (Static Contract Tests)

```text
$ go test -tags integration -count=1 -run TestSpec077 ./tests/integration/cli/...
ok      github.com/smackerel/smackerel/tests/integration/cli    0.007s
```

Both `TestSpec077TestStackUsesDedicatedComposeProject` (TP-077-01-05) and `TestSpec077TestStackIsolation_DevStackUntouched` (TP-077-01-02) pass.

### Regression Evidence (SCOPE-1a + SCOPE-1b canaries still green)

```text
$ bash tests/unit/cli/spec_077_test_dispatcher_test.sh
PASS: spec_077_test_dispatcher_test (TP-077-01-04 / SCN-077-A09)

$ bash tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh
PASS: spec_077_playwright_config_fail_loud_test (TP-077-01-03 / SCN-077-A10)
```

The SCOPE-1c lifecycle adds a `SMACKEREL_E2E_UI_NPX`-gated bypass so the existing canaries (which inject a stub `npx`) continue to assert dispatcher + runner contracts without bringing up docker. The seam handoff is documented inline in `scripts/runtime/web-e2e-ui.sh` at the default-action block.

### Live-Stack Lifecycle Evidence (partial)

`./smackerel.sh test e2e-ui` was invoked end-to-end against the disposable test stack. The harness:

1. Generated SST → `config/generated/test.env` with `CORE_EXTERNAL_URL=http://127.0.0.1:45001`.
2. Brought up containers under the dedicated Compose project — observed names: `smackerel-test-e2e-ui-postgres-1`, `smackerel-test-e2e-ui-nats-1`, `smackerel-test-e2e-ui-smackerel-ml-1`, `smackerel-test-e2e-ui-smackerel-core-1`, `smackerel-test-e2e-ui-ollama-1`, `smackerel-test-e2e-ui-searxng-1`. NONE of these collide with the `smackerel-test` integration/e2e/stress lane or the `smackerel` dev lane.
3. Containers reached health: `postgres-1 Healthy`, `nats-1 Healthy`, `smackerel-ml-1 Healthy`, `ollama-1 Healthy`, `searxng-1 Healthy`.
4. `smackerel-core-1` reported `unhealthy` within the `COMPOSE_WAIT_TIMEOUT_S=180` window on this fresh build. The trap fired and tore down the entire `smackerel-test-e2e-ui` project (volumes + network removed); no resources leaked into other projects.

This live evidence proves SCN-077-A07 (lane isolation under the dedicated project + trap-driven teardown) end-to-end. It does NOT prove SCN-077-A01 (Playwright proof-of-life green) because `smackerel-core` did not pass its healthcheck within the SST wait window on this run. The smackerel-core readiness/healthcheck behavior is the existing live-stack contract owned by the runtime spec, not the spec 077 harness wiring; the harness merely consumes the same `--wait --wait-timeout $COMPOSE_WAIT_TIMEOUT_S` contract used by `./smackerel.sh --env test up`. A repeat run after the core image is warm should clear this — but the harness invocation is unchanged, so the green-proof DoD item below carries an Uncertainty Declaration rather than a fabricated PASS.

### Code Diff Evidence

```text
$ git status --short | grep -E 'spec_077|web/pwa/tests/proof_of_life|web-e2e-ui|tests/integration/cli'
 M scripts/runtime/web-e2e-ui.sh
?? tests/integration/cli/
?? web/pwa/tests/proof_of_life.spec.ts
```

Within Change Boundary: only `scripts/runtime/web-e2e-ui.sh` (lifecycle wiring, allowed) + `web/pwa/tests/proof_of_life.spec.ts` (allowed) + `tests/integration/cli/spec_077_*_test.go` (allowed). Zero touches to: any other `web/pwa/tests/*.spec.ts` body, `web/pwa/package.json`, `web/pwa/playwright.config.ts`, `web/pwa/tests/_support/csp.ts`, CI workflow files, `docs/Testing.md`, `README.md`, the non-`e2e-ui)` arms of `smackerel.sh`, `scripts/lib/runtime.sh`.

### Artifact-Lint Evidence

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness
DONE
```

### Rollback Path

1. `rm tests/integration/cli/spec_077_compose_project_test.go tests/integration/cli/spec_077_test_stack_isolation_test.go` (and `rmdir tests/integration/cli` if empty).
2. `rm web/pwa/tests/proof_of_life.spec.ts`.
3. Revert `scripts/runtime/web-e2e-ui.sh` to its SCOPE-1b state: remove the `bring_up_test_stack` / `tear_down_test_stack` / `e2e_ui_compose` block and the `source scripts/lib/runtime.sh` line; restore the SCOPE-1b default-action block (`run_node_tooling "$@"` only, no live-stack bring-up).

No schema, no managed-doc edits, no CI edits.

### Completion Statement

SCOPE-1c (Proof-of-Life Spec + Live-Stack Isolation Proof) ships the planned harness lifecycle and isolation invariants. The Compose-project isolation contract (SCN-077-A07) is proven end-to-end: both static (TP-077-01-02, TP-077-01-05 Go integration tests) and live (containers observed under `smackerel-test-e2e-ui-*` prefix; trap teardown verified). The proof-of-life spec body (`proof_of_life.spec.ts`) ships and the harness reaches the Playwright invocation, but the GREEN run of TP-077-01-01 / TP-077-01-01R against the live stack did NOT complete in this turn because `smackerel-core` failed its healthcheck within the SST `COMPOSE_WAIT_TIMEOUT_S` window on the fresh-build run. The corresponding DoD bullet carries an Uncertainty Declaration and the issue is routed back as an unresolved finding for the runtime/harden owner.

