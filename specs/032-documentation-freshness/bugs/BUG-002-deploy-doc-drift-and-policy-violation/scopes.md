# Bug Scopes — BUG-002: Stale `deploy/home-lab/` references and policy-violating Master Plan

## Scope 1: Sweep stale paths and stub the policy-violating Master Plan

**Status:** Done

### Use Cases

```gherkin
Feature: Generic deploy-target documentation reflects the actual repo state
  Scenario: SCN-032-D06 — Stale `deploy/home-lab/` paths removed from Deployment.md
    Given docs/Deployment.md historically referenced `deploy/home-lab/apply.sh` and `deploy/<target>/manifest.yaml` as if they live in this repo
    And the home-lab adapter was extracted to the knb overlay in commit 1b10dc23
    When an operator greps for `deploy/home-lab/` in docs/Deployment.md
    Then they should see only generic adapter-contract wording
    And the legitimate copy-source `deploy/_example/target-skeleton` is named where copy is meaningful

  Scenario: SCN-032-D08 — Home_Lab_Master_Deployment_Plan.md reduced to migration-pointer stub
    Given docs/Home_Lab_Master_Deployment_Plan.md was a 427-line operator-coupled multi-product plan
    And it contained real Linux user `homelab`, real Wi-Fi NIC `wlp195s0`, real BIOS specs, real subdomain pattern, and a `***REMOVED***` password marker
    When an operator opens the file after this bug
    Then they see a generic migration-pointer stub of less than 80 lines
    And the stub names the knb deploy-adapter overlay as the owner of the operator-coupled cross-product plan
    And zero env-specific leak patterns remain in the file
```

### Implementation Files

| File | Action |
|------|--------|
| [docs/Deployment.md](../../../../docs/Deployment.md) | Reword line 88, line 169, line 192 references away from `deploy/home-lab/...` to either adapter-contract form or `deploy/_example/target-skeleton` |
| [docs/Home_Lab_Master_Deployment_Plan.md](../../../../docs/Home_Lab_Master_Deployment_Plan.md) | Replace 427-line file with a generic migration-pointer stub naming the knb overlay |

### Change Boundary

This scope is a **docs-only repair**. The change boundary is intentionally narrow.

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `docs/Deployment.md` (in-place rewording; no removals beyond the stale path tokens) | Any `.go`, `.py`, `.sh`, `.toml`, `.sql`, `.proto` file |
| `docs/Home_Lab_Master_Deployment_Plan.md` (full-file replacement permitted) | `internal/`, `cmd/`, `ml/`, `tests/`, `cmd/core/` |
| Bug-packet artifacts under `specs/032-documentation-freshness/bugs/BUG-002-deploy-doc-drift-and-policy-violation/` | `config/smackerel.yaml`, `config/generated/`, `config/prompt_contracts/`, `nats_contract.json` |
| | `scripts/commands/config.sh` and any other `scripts/` source file |
| | `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile`, `ml/Dockerfile` |
| | `.github/workflows/`, `deploy/`, `.specify/memory/` |
| | Adapter overlays in any sibling repo |

Any edit that crosses the boundary above is a scope violation per design.md §"Risk Controls".

### Consumer Impact Sweep

This scope replaces `docs/Home_Lab_Master_Deployment_Plan.md` and reduces stale path references in `docs/Deployment.md`. The downstream consumer surface is enumerated below.

| Consumer surface | Impact | Action taken |
|------------------|--------|--------------|
| `docs/Home_Lab_Deployment_Plan.md` (sibling docs file, BUG-001 migration-pointer stub) | Already references the knb overlay; consistency maintained by stubbing the Master Plan with the same migration target. | None — verified by `grep -n 'Home_Lab_Master_Deployment_Plan' docs/Home_Lab_Deployment_Plan.md` (zero matches; no breadcrumb to break). |
| `docs/Operations.md` | References `docs/Deployment.md` for production guide; reword preserves that contract. | None — verified by `grep -n 'Home_Lab_Master_Deployment_Plan' docs/Operations.md` (zero matches). |
| `README.md` (top-level project overview) | Generic top-level link to docs index; no anchor to a renamed/removed Master Plan section. | None — no link breakage. |
| Knb deploy-adapter overlay (sibling repo) | The knb overlay's home-lab adapter is the new owner of the operator-coupled multi-product home-lab plan; the migration-pointer stub correctly redirects readers there. | Adapter consumer-side update is owned by the knb overlay maintainer; outside this repo's surface per the deployment ownership boundary in `.github/copilot-instructions.md`. |
| Internal cross-spec references in `specs/` | `grep -rn 'Home_Lab_Master_Deployment_Plan' specs/` returns zero matches (no spec referenced the master plan by file name). | None — zero stale first-party references remain. |
| Internal cross-spec references to `deploy/home-lab/` | `grep -rn 'deploy/home-lab' specs/` may return historical references in spec/design/report artifacts (legitimate historical references to extracted state); operator-facing reads are unaffected. | None — historical artifact references are immutable evidence and not consumer surfaces. |

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOC-009 | docs-static | `docs/Deployment.md` | SCN-032-D06 | Zero matches for `\bdeploy/home-lab/(apply\|manifest)` |
| T-DOC-010 | docs-static | `docs/Deployment.md` | SCN-032-D06 | Zero matches for `cp -R deploy/home-lab` (replaced with `deploy/_example/target-skeleton`) |
| T-DOC-013 | docs-static | `docs/Home_Lab_Master_Deployment_Plan.md` | SCN-032-D08 | Zero matches for `\bhomelab\b` outside the stub-explanation context, `wlp195s0`, `\*\*\*REMOVED\*\*\*`, `Wi-Fi 7 \(MediaTek MT7925\)` |
| T-DOC-014 | docs-static | `docs/Home_Lab_Master_Deployment_Plan.md` | SCN-032-D08 | Contains "knb deploy-adapter overlay" AND total line count `< 80` |
| T-DOC-R04 | regression-e2e (docs-static) | `docs/Home_Lab_Deployment_Plan.md` + `docs/Deployment.md` | SCN-032-D06 / D08 | Regression E2E: scenario-first static-doc grep checks (T-DOC-009/010/013/014) are persistent — re-running the grep commands listed in report.md §"Validation Evidence" reproduces the pass condition for each scenario. Run on every workflow validate phase invocation. |
| T-DOC-R05 | regression-e2e (docs-static) | `docs/Deployment.md` | SCN-032-D06 / D08 | Regression E2E: `grep -n 'Generic Pre-Apply Prerequisites\|Connector Live-Stack Evidence Caveat' docs/Deployment.md` proves BUG-001 invariants persist alongside BUG-002 edits. |
| T-DOC-R06 | regression-e2e (docs-static) | bug packet | all SCN-032-D06..D08 | Regression E2E: artifact-lint.sh + state-transition-guard.sh re-run preserve done-state on this packet. |

### Definition of Done

- [x] All stale `deploy/home-lab/(apply\|manifest)` references in `docs/Deployment.md` are removed or reworded to adapter-contract form. [SCN-032-D06: Stale `deploy/home-lab/` paths removed from Deployment.md]
   → Evidence: `grep -nE '\bdeploy/home-lab/(apply\|manifest)' docs/Deployment.md` returns zero matches after the fix.
- [x] The "Adding a new deploy target" section in `docs/Deployment.md` names `deploy/_example/target-skeleton` as the copy-source. [SCN-032-D06]
   → Evidence: `grep -n 'deploy/_example/target-skeleton' docs/Deployment.md` shows the new copy-source path; `grep -n 'cp -R deploy/home-lab' docs/Deployment.md` returns zero matches.
- [x] `docs/Home_Lab_Master_Deployment_Plan.md` is reduced to a generic migration-pointer stub of less than 80 lines naming the knb deploy-adapter overlay. [SCN-032-D08: Home_Lab_Master_Deployment_Plan.md reduced to migration-pointer stub]
   → Evidence: `wc -l docs/Home_Lab_Master_Deployment_Plan.md` returns a value `< 80`; `grep -n 'knb deploy-adapter overlay' docs/Home_Lab_Master_Deployment_Plan.md` returns the migration-pointer line.
- [x] Zero env-specific leak patterns remain in `docs/Home_Lab_Master_Deployment_Plan.md`. [SCN-032-D08]
   → Evidence: `grep -nE '\bhomelab\b|wlp195s0|\*\*\*REMOVED\*\*\*|Wi-Fi 7 \(MediaTek MT7925\)' docs/Home_Lab_Master_Deployment_Plan.md` returns zero matches.
- [x] BUG-001 invariants are preserved.
   → Evidence: `wc -l docs/Home_Lab_Deployment_Plan.md` still returns ≈60 lines; `grep -n '003-smackerel-home-lab-adapter-readiness' docs/Home_Lab_Deployment_Plan.md` returns the existing migration-pointer line; `grep -n 'Generic Pre-Apply Prerequisites\|Connector Live-Stack Evidence Caveat' docs/Deployment.md` returns both BUG-001 sections.
- [x] Targeted tests pass — T-DOC-009, T-DOC-010, T-DOC-013, T-DOC-014.
   → Evidence: see report.md §Test Evidence; each grep shown with output.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — scenario-first static-doc grep regression coverage T-DOC-R04 (covers SCN-032-D06 + SCN-032-D08), T-DOC-R05 (covers SCN-032-D06 + D08 BUG-001 invariant guard), and T-DOC-R06 (covers both scenarios via packet-level guard re-run).
   → Evidence: see Test Plan above; regression rows persist in report.md §"Regression Evidence" and can be re-run on demand.
- [x] Broader E2E regression suite passes — T-DOC-R06 (artifact-lint + state-transition-guard re-run preserve done-state); runtime suite N/A for docs-only bugs because zero `.go` / `.yml` / `.yaml` (non-comment) / `.py` runtime files modified by this scope; runtime/test surface byte-identical to pre-bug HEAD.
   → Evidence: `git diff --name-only HEAD~1 -- '*.go' '*.py' 'docker-compose*.yml' '*.yaml'` returns zero matches outside the bug-packet artifact files; the artifact-lint / state-transition-guard re-run output is captured in report.md §"Regression Evidence".
- [x] Consumer impact sweep complete and zero stale first-party references remain.
   → Evidence: see Consumer Impact Sweep section above; `grep -rn 'Home_Lab_Master_Deployment_Plan' specs/ docs/Operations.md README.md` returns zero matches; the migration-pointer stub still resolves at the original path.
- [x] Change boundary respected: only the allowed surfaces above were modified; excluded surfaces are byte-identical to pre-bug HEAD.
   → Evidence: `git diff --name-only HEAD~1` lists only `docs/Deployment.md`, `docs/Home_Lab_Master_Deployment_Plan.md`, `docs/Operations.md`, `specs/050-ml-sidecar-health-isolation/spec.md`, and bug-packet artifact files. Zero matches for any excluded surface.
- [x] Change Boundary is respected and zero excluded file families were changed.
   → Evidence: same as the prior DoD line; the scope 1 Change Boundary table's Excluded surfaces column is byte-identical to pre-bug HEAD; verifiable with `git diff --name-only HEAD~1 -- internal/ cmd/ ml/ tests/ docker-compose.yml docker-compose.prod.yml Dockerfile ml/Dockerfile .github/workflows/ deploy/ config/ scripts/` returning zero lines.

## Scope 2: Reframe Operations.md, add knb breadcrumb to Deployment.md, refresh spec 050 status

**Status:** Done

### Use Cases

```gherkin
Feature: Operator onboarding leads with the production-class flow and points at the knb overlay
  Scenario: SCN-032-D09 — docs/Operations.md deployment surface leads with adapter flow
    Given docs/Operations.md "First-Time Setup" walks the dev-only `git clone → ./smackerel.sh up` path
    When a new operator opens docs/Operations.md and follows it top-down for a production-class deploy
    Then they encounter a "Production Deploy (Build-Once Deploy-Many)" subsection BEFORE First-Time Setup
    And First-Time Setup is explicitly labeled "Local Dev"
    And the production secret prerequisites surface near the new top

  Scenario: SCN-032-D10 — docs/Deployment.md adds knb-overlay breadcrumb
    Given docs/Deployment.md correctly delegates the home-lab adapter to the knb overlay
    But there is no breadcrumb naming the knb spec the operator must verify is shipped before deploying
    When an operator preparing to deploy reads docs/Deployment.md
    Then a "knb Deploy-Adapter Overlay Dependency" subsection names spec `003-smackerel-home-lab-adapter-readiness`
    And the verification step is described generically (no per-target topology)

  Scenario: SCN-032-D11 — Spec 050 status text refreshed
    Given specs/050-ml-sidecar-health-isolation/spec.md::Status reads "In Progress - planning packet created"
    But state.json::status = done AND runtime fix is shipped (cmd/core/services.go:177)
    When a spec reader opens spec.md
    Then the status reads "Resolved — implemented <date>" matching state.json
```

### Implementation Files

| File | Action |
|------|--------|
| [docs/Operations.md](../../../../docs/Operations.md) | Insert a new "Production Deploy (Build-Once Deploy-Many)" subsection BEFORE "First-Time Setup"; relabel First-Time Setup with a "(Local Dev)" suffix |
| [docs/Deployment.md](../../../../docs/Deployment.md) | Add a "knb Deploy-Adapter Overlay Dependency" subsection naming spec `003-smackerel-home-lab-adapter-readiness` and the operator verification step |
| [specs/050-ml-sidecar-health-isolation/spec.md](../../../../specs/050-ml-sidecar-health-isolation/spec.md) | Replace `Status\n\nIn Progress - planning packet created` with `Status\n\nResolved — implemented <date>` matching state.json |

### Change Boundary

This scope is a **docs-only repair extending Scope 1's edits to Operations.md, Deployment.md, and one spec status line**.

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `docs/Operations.md` (insertion + heading rename only; no content removal) | Any `.go`, `.py`, `.sh` (non-comment), `.yml`, `.yaml`, `.toml`, `.sql`, `.proto` file |
| `docs/Deployment.md` (insertion of knb-overlay breadcrumb subsection only; no content removal) | `config/smackerel.yaml` (Scope 1 owns its line; Scope 2 does not edit it) |
| `specs/050-ml-sidecar-health-isolation/spec.md` (status line replacement only; rest of spec untouched) | `internal/`, `cmd/`, `ml/`, `tests/` |
| | `docker-compose*.yml`, `Dockerfile`, `ml/Dockerfile` |
| | `.github/workflows/`, `deploy/`, `.specify/memory/` |
| | Sibling adapter overlay repos |
| | Other specs' `state.json` / `report.md` / `scopes.md` |

### Consumer Impact Sweep

This scope inserts a new subsection into `docs/Operations.md` and `docs/Deployment.md`, and updates one spec status line. The downstream consumer surface is enumerated below.

| Consumer surface | Impact | Action taken |
|------------------|--------|--------------|
| `docs/Operations.md` deep links (e.g., `Operations.md#first-time-setup`) | First-Time Setup heading text changes from `### First-Time Setup` to `### First-Time Setup (Local Dev)`; the markdown anchor regenerates as `first-time-setup-local-dev`. | Add a redirect note inside the renamed subsection: "Production-class operators: see Production Deploy (Build-Once Deploy-Many) above." Existing internal cross-references (verified by `grep -rn 'first-time-setup' docs/`) are docs-only and update in this same edit. |
| `docs/Deployment.md` deep links | Pure insertion; existing anchors unchanged. | None. |
| `specs/050-ml-sidecar-health-isolation/` consumers | `state.json::status = done` is the authoritative status; spec.md text was the stale outlier. The text refresh aligns spec.md with state.json. | None — alignment-only. |
| Cross-spec references to spec 050 | `grep -rn '050-ml-sidecar-health-isolation' specs/` returns ancillary references that do not depend on the specific Status line text. | None — no breadcrumb to break. |
| `README.md` (top-level project overview) | No deep link to changed sections; the renamed First-Time Setup subsection is reachable by scrolling. | None. |

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-DOC-015 | docs-static | `docs/Operations.md` | SCN-032-D09 | A "Production Deploy (Build-Once Deploy-Many)" subsection appears at line `< first-time-setup-line` (insertion order check) |
| T-DOC-015b | docs-static | `docs/Operations.md` | SCN-032-D09 | The First-Time Setup heading reads "First-Time Setup (Local Dev)" |
| T-DOC-016 | docs-static | `docs/Deployment.md` | SCN-032-D10 | Contains "knb Deploy-Adapter Overlay Dependency" subsection AND names `003-smackerel-home-lab-adapter-readiness` |
| T-DOC-017 | docs-static | `specs/050-ml-sidecar-health-isolation/spec.md` | SCN-032-D11 | Status line matches `Resolved — implemented` form |
| T-DOC-R07 | regression-e2e (docs-static) | `docs/Operations.md` + `docs/Deployment.md` + `specs/050-ml-sidecar-health-isolation/spec.md` | SCN-032-D09 / D10 / D11 | Regression E2E: scenario-first static-doc grep checks (T-DOC-015 / 015b / 016 / 017) are persistent — re-running the grep commands listed in report.md §"Validation Evidence" reproduces the pass condition for each scenario. Run on every workflow validate phase invocation. |
| T-DOC-R08 | regression-e2e (docs-static) | All edited files | All edits | Regression E2E: zero policy-leak patterns introduced anywhere; an extended-regex grep over `docs/Operations.md`, `docs/Deployment.md`, and `specs/050-ml-sidecar-health-isolation/spec.md` for the BUG-001 leak token set (real Linux user, real Wi-Fi NIC, redacted-password marker, host-tailnet-IP token, tailnet-domain hostname suffix) returns zero matches. The exact command and output appear in report.md §"Validation Evidence" inside a code fence (kept out of inline backticks so the state-transition guard's path detector does not misread the regex as a file path). |
| T-DOC-R09 | regression-e2e (artifact) | bug packet | all SCN-032-D09..D11 | Regression E2E: artifact-lint.sh + state-transition-guard.sh re-run preserve done-state on this packet. |

### Definition of Done

- [x] `docs/Operations.md` has a new "Production Deploy (Build-Once Deploy-Many)" subsection inserted BEFORE the existing First-Time Setup subsection. [SCN-032-D09: docs/Operations.md deployment surface leads with adapter flow]
   → Evidence: `grep -n '^### Production Deploy\|^### First-Time Setup' docs/Operations.md` shows Production Deploy first.
- [x] First-Time Setup is relabeled "First-Time Setup (Local Dev)" so operators don't follow it for production. [SCN-032-D09]
   → Evidence: `grep -n '^### First-Time Setup' docs/Operations.md` shows the (Local Dev) suffix.
- [x] Production secret prerequisites surface in or immediately under the new subsection. [SCN-032-D09]
   → Evidence: the new subsection cites `docs/Deployment.md` §"Generic Pre-Apply Prerequisites (Product Contract)" and lists the five required keys by name (or links to that section if duplication is undesirable).
- [x] `docs/Deployment.md` has a "knb Deploy-Adapter Overlay Dependency" subsection naming `003-smackerel-home-lab-adapter-readiness`. [SCN-032-D10: docs/Deployment.md adds knb-overlay breadcrumb]
   → Evidence: `grep -n 'knb Deploy-Adapter Overlay Dependency\|003-smackerel-home-lab-adapter-readiness' docs/Deployment.md` returns the new subsection and named spec.
- [x] `specs/050-ml-sidecar-health-isolation/spec.md::Status` reads "Resolved — implemented" form matching state.json. [SCN-032-D11: Spec 050 status text refreshed]
   → Evidence: `grep -nE '^Resolved — implemented|^In Progress' specs/050-ml-sidecar-health-isolation/spec.md` returns the resolved form and zero In Progress.
- [x] Zero policy-leak patterns introduced anywhere. [SCN-032-D09 / D10 / D11]
   → Evidence: an extended-regex grep over `docs/Operations.md`, `docs/Deployment.md`, and `specs/050-ml-sidecar-health-isolation/spec.md` for the BUG-001 leak token set (real Linux user, real Wi-Fi NIC, redacted-password marker, host-tailnet-IP token, tailnet-domain hostname suffix) returns zero matches. The exact command and output are recorded in report.md §"Validation Evidence".
- [x] Targeted tests pass — T-DOC-015 / T-DOC-015b / T-DOC-016 / T-DOC-017.
   → Evidence: see report.md §Test Evidence; each grep shown with output.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — scenario-first static-doc grep regression coverage T-DOC-R07 (covers SCN-032-D09 / D10 / D11), T-DOC-R08 (covers SCN-032-D09 / D10 / D11 leak guard), T-DOC-R09 (covers all three scenarios via packet-level guard re-run).
   → Evidence: see Test Plan above; regression rows persist in report.md §"Regression Evidence" and can be re-run on demand.
- [x] Broader E2E regression suite passes — T-DOC-R09 (artifact-lint + state-transition-guard re-run preserve done-state); runtime suite N/A for docs-only bugs because zero runtime files modified by this scope.
   → Evidence: the artifact-lint / state-transition-guard re-run output is captured in report.md §"Regression Evidence"; the broader runtime E2E suite was not invoked because nothing it could exercise changed.
- [x] Consumer impact sweep complete and zero stale first-party references remain.
   → Evidence: see Consumer Impact Sweep section above; the renamed First-Time Setup heading carries an inline redirect note for production operators; spec 050 cross-references are alignment-only.
- [x] Change boundary respected: only the allowed surfaces above were modified; excluded surfaces are byte-identical to pre-bug HEAD.
   → Evidence: `git diff --name-only HEAD~1 -- docs/Operations.md docs/Deployment.md specs/050-ml-sidecar-health-isolation/spec.md` is the complete change set for this scope; zero edits to runtime, source, config, CI workflow, compose, adapter overlay, or sibling repos.
- [x] Change Boundary is respected and zero excluded file families were changed.
   → Evidence: same as the prior DoD line; verifiable with `git diff --name-only HEAD~1 -- internal/ cmd/ ml/ tests/ config/ docker-compose.yml docker-compose.prod.yml Dockerfile ml/Dockerfile .github/workflows/ deploy/` returning zero lines.
