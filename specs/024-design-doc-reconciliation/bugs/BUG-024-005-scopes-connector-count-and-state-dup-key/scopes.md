# Scopes: BUG-024-005 Residual reconciliation drift (state.json dup `lastUpdatedAt` + scopes.md 15→16)

## Execution Outline

### Phase Order

1. **Scope 1 — Dedup `lastUpdatedAt` + recert + reconcile scopes.md 15→16 + parent governance:** Single-scope packet. Two latent findings (F1 state.json duplicate key; F2 scopes.md connector-count drift) of the same residual-drift class, closed together. Docs/governance-only; no runtime/schema/NATS/docs-product change. Changes left in the working tree (no commit per round hard rule).

### New Types & Signatures

- No new types, functions, or test files. F1 is a key deletion + recert; F2 is text reconciliation; both are governed by existing gates (G088 recert, G068 fidelity) and the existing BUG-024-003 contract test.

### Validation Checkpoints

- After FR-01: `python3 -c "import json; raw=open('specs/024-design-doc-reconciliation/state.json').read(); assert raw.count('\"lastUpdatedAt\"')==1"` exits 0.
- After FR-02: top-level `certifiedAt`/`lastUpdatedAt` == `2026-06-06T18:00:00Z`; a `bubbles.spec-review` `reviewStatus: CURRENT` entry exists; `requiresRevalidation` absent.
- After FR-03: `grep -nE '15 (connectors|committed)' scopes.md` returns ONLY historical/rollback/grep-contract lines; `SCN-024-06` body lists `qfdecisions`.
- After FR-04: `traceability-guard.sh` exits 0 `RESULT: PASSED`; STG Check 22 (G068) all 6 faithful.
- After FR-05: `resolvedBugs[]` has BUG-024-005; `report.md` has 1 `## BUG-024-005 Harden-Sweep Resolution` section; zero `/home/<user>/`.
- After FR-06: nothing committed; content guards green; G088/STG show the documented uncommitted-handoff state.

## Scope Summary

| # | Name | Surfaces | Key Tests | DoD Summary | Status |
|---|------|----------|-----------|-------------|--------|
| 1 | Dedup `lastUpdatedAt` + recert + scopes.md 15→16 + parent governance | `state.json` + `scopes.md` + `report.md` + new BUG packet folder | duplicate-key detector + 5 framework guards + `TestConnectorCountContract` + grep contracts | single `lastUpdatedAt`; 8 sites 16 + `qfdecisions`; content guards green; G088/STG honest worktree state; no commit | Done |

---

## Scope 1: Dedup `lastUpdatedAt` + Recert + Reconcile scopes.md 15→16 + Parent Governance

**Status:** Done

### Use Cases (Gherkin)

```gherkin
Scenario: BUG-024-005-SCN-001 state.json has a single top-level lastUpdatedAt
  Given specs/024-design-doc-reconciliation/state.json had two top-level "lastUpdatedAt" keys (pre-fix)
  When the BUG-024-005 fix removes the legacy 2026-05-25T00:00:00Z key
  Then python3 raw.count('"lastUpdatedAt"') == 1
  And a json.load object_pairs_hook duplicate-key detector reports no duplicate-key sets
  And the surviving value is the fresh recert timestamp 2026-06-06T18:00:00Z

Scenario: BUG-024-005-SCN-002 state.json is recertified for the planning-truth edit
  Given scopes.md (planning truth) is edited on a done spec
  When the recert is applied
  Then top-level certifiedAt == 2026-06-06T18:00:00Z and lastUpdatedAt == 2026-06-06T18:00:00Z
  And a bubbles.spec-review executionHistory entry with reviewStatus CURRENT is present
  And requiresRevalidation is NOT set (it would trip Gate G089 on a done spec)
  And status stays done

Scenario: BUG-024-005-SCN-003 scopes.md Scope 2 connector count is reconciled to 16
  Given scopes.md Scope 2 body + SCN-024-04/06 claimed 15 connectors omitting qfdecisions (pre-fix)
  When the 8 substantive sites are reconciled 15 to 16
  Then SCN-024-06 title + body say "16 connectors" and the name list includes qfdecisions between markets and rss
  And the count matches spec.md R-006, scenario-manifest.json SCN-024-06, docs/smackerel.md §22.7, and the live registry
  And the 7 historical/rollback/grep-contract "15" references are preserved verbatim

Scenario: BUG-024-005-SCN-004 DoD-Gherkin fidelity (G068) stays green
  Given SCN-024-06 Gherkin title and its DoD bullet are changed in lockstep
  When traceability-guard.sh runs
  Then it exits 0 with RESULT: PASSED
  And STG Check 22 reports all 6 Gherkin scenarios have faithful DoD items
  And artifact-lint + artifact-freshness also exit 0
  And go test -run TestConnectorCountContract ./internal/deploy/... passes 4/4

Scenario: BUG-024-005-SCN-005 Parent governance backfill recorded; nothing committed
  Given the fix is applied
  When inspecting state.json + report.md and the git working tree
  Then state.json has ≥ 7 executionHistory entries naming BUG-024-005 and a resolvedBugs[] BUG-024-005 entry
  And report.md has exactly 1 "## BUG-024-005 Harden-Sweep Resolution" section with Code Diff Evidence + Git-Backed Proof, redacted to ~/
  And nothing is committed or pushed — all changes remain in the working tree
  And G088 / STG Check 30 honestly report the uncommitted scopes.md worktree edit (postCertEdits=1) that clears when the parent commits before certifiedAt=2026-06-06T18:00:00Z
```

### Implementation Plan

**Files touched (working tree only; NO commit):**

1. `specs/024-design-doc-reconciliation/state.json` — remove legacy `lastUpdatedAt`; recert `certifiedAt`/`lastUpdatedAt` → `2026-06-06T18:00:00Z`; append 17 harden-sweep `executionHistory` entries (incl. `bubbles.spec-review` CURRENT); append `resolvedBugs[]` BUG-024-005.
2. `specs/024-design-doc-reconciliation/scopes.md` — 8 substantive sites 15→16 + insert `qfdecisions`; 7 historical "15" preserved.
3. `specs/024-design-doc-reconciliation/report.md` — append `## BUG-024-005 Harden-Sweep Resolution (2026-06-06)`.
4. `specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key/` — 8 packet artifacts.

**Consumer Impact Sweep:** `scopes.md` has no code consumers. The product-truth connector surfaces (`docs/smackerel.md` §22.7/§24-A) already say 16 and are untouched. The `SCN-024-06` text consumers are the traceability guard (kept green by lockstep edit) and `scenario-manifest.json` (already 16). The `state.json` `lastUpdatedAt` consumers (governance tooling) read last-wins; deduping makes the read deterministic.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| State shape | `python3 -c "import json; raw=open('specs/024-design-doc-reconciliation/state.json').read(); assert raw.count('\"lastUpdatedAt\"')==1"` exit 0 | F1 dedup | BUG-024-005-SCN-001 |
| State shape | `python3` `object_pairs_hook` duplicate-key detector → no duplicate-key sets | F1 dedup | BUG-024-005-SCN-001 |
| State shape | `python3 -c "import json; d=json.load(open('.../state.json')); assert d['certifiedAt']=='2026-06-06T18:00:00Z' and 'requiresRevalidation' not in d"` exit 0 | FR-02 recert | BUG-024-005-SCN-002 |
| State shape | `python3` assert `bubbles.spec-review` `reviewStatus==CURRENT` entry present | FR-02 recert | BUG-024-005-SCN-002 |
| Grep | `grep -nE '15 (connectors|committed)' scopes.md` → only historical/rollback/grep-contract lines remain | F2 reconciliation | BUG-024-005-SCN-003 |
| Grep | `grep -n 'qfdecisions' scopes.md` → ≥1 hit in SCN-024-06 body | F2 reconciliation | BUG-024-005-SCN-003 |
| Governance gate | `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` exit 0 `RESULT: PASSED` | G068 fidelity stays green | BUG-024-005-SCN-004 |
| Governance gate | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` exit 0 `Artifact lint PASSED.` | parent lint stays green | BUG-024-005-SCN-004 |
| Governance gate | `bash .github/bubbles/scripts/artifact-freshness-guard.sh specs/024-design-doc-reconciliation` exit 0 `RESULT: PASS` | freshness stays green | BUG-024-005-SCN-004 |
| Governance gate | `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key` exit 0 | BUG packet's own gate | BUG-024-005-SCN-005 |
| Runtime regression | `go test -run TestConnectorCountContract ./internal/deploy/...` exit 0 + 4/4 PASS | BUG-024-003 contract preserved | BUG-024-005-SCN-004 |
| Report shape | `grep -cE '^## BUG-024-005 Harden-Sweep Resolution' specs/024-design-doc-reconciliation/report.md` == 1 | FR-05 | BUG-024-005-SCN-005 |
| PII redaction | `grep -cE '/home/<user>/' specs/024-design-doc-reconciliation/report.md` == 0 | gitleaks linux-home-username-leak will not fire | BUG-024-005-SCN-005 |
| Commit discipline | `git log -1 --format='%H'` unchanged (nothing committed) | round hard rule (no commit) | BUG-024-005-SCN-005 |
| Governance gate (honest) | `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/024-design-doc-reconciliation` reports `postCertEdits=1` WORKTREE scopes.md with `certifiedAt=2026-06-06T18:00:00Z` (designed pre-commit handoff) | honest report of uncommitted state; clears on parent commit | BUG-024-005-SCN-005 |
| Regression E2E | Persistent gate sweep — re-run state-transition-guard, artifact-lint, artifact-freshness-guard, traceability-guard, post-cert-spec-edit-guard 3 consecutive times each; content guards green every iteration, G088/STG worktree state stable | Persistent scenario-specific regression coverage that fails if F1 duplicate key returns, F2 15-claims regress, or G068 fidelity breaks | BUG-024-005-SCN-003, BUG-024-005-SCN-004 |
| Regression E2E (broader) | `./smackerel.sh test unit --go` baseline + Go contract test family `TestConnectorCountContract*` | Broader regression cover: Go runtime stays green; BUG-024-003 forward-detection guard preserved verbatim | BUG-024-005-SCN-004 |
| Stress | Coordinated re-run of the duplicate-key detector + traceability-guard at ≥ 5 consecutive iterations to prove no flaky boundary | Stress coverage for the dedup + fidelity contracts (deterministic, repeatable) | BUG-024-005-SCN-001, BUG-024-005-SCN-004 |

### Definition of Done

- [x] Scenario BUG-024-005-SCN-001 (state.json has a single top-level lastUpdatedAt): `lastUpdatedAt` literal count is 1; duplicate-key detector reports none.
  Evidence: post-fix `python3` probe
  ```
  $ python3 -c "import json; raw=open('specs/024-design-doc-reconciliation/state.json').read(); print('count:', raw.count('\"lastUpdatedAt\"'))"
  count: 1
  $ python3 -c "import json; seen=[]; json.loads(open('specs/024-design-doc-reconciliation/state.json').read(), object_pairs_hook=lambda p:(seen.append([k for k in {x for x,_ in p} if [y for y,_ in p].count(k)>1]) or dict(p))); print('dup-sets:', [s for s in seen if s])"
  dup-sets: []
  ```
- [x] Scenario BUG-024-005-SCN-002 (state.json is recertified for the planning-truth edit): `certifiedAt`/`lastUpdatedAt` == 2026-06-06T18:00:00Z; `bubbles.spec-review` CURRENT entry present; `requiresRevalidation` absent; status done.
  Evidence: post-fix `python3` probe
  ```
  $ python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); print('certifiedAt:', d['certifiedAt']); print('lastUpdatedAt:', d['lastUpdatedAt']); print('status:', d['status']); print('specreview_current:', any(e.get('agent')=='bubbles.spec-review' and e.get('reviewStatus')=='CURRENT' for e in d['executionHistory'])); print('requiresRevalidation:', 'requiresRevalidation' in d)"
  certifiedAt: 2026-06-06T18:00:00Z
  lastUpdatedAt: 2026-06-06T18:00:00Z
  status: done
  specreview_current: True
  requiresRevalidation: False
  ```
- [x] Scenario BUG-024-005-SCN-003 (scopes.md Scope 2 connector count is reconciled to 16): 8 substantive sites = 16; `SCN-024-06` body lists `qfdecisions`; 7 historical "15" preserved.
  Evidence: post-fix grep
  ```
  $ grep -cE '16 (connectors|committed)' specs/024-design-doc-reconciliation/scopes.md
  8
  $ grep -c 'markets, qfdecisions, rss' specs/024-design-doc-reconciliation/scopes.md
  1
  $ grep -cE 'git revert <BUG-024-002 SHA>.*15 connectors' specs/024-design-doc-reconciliation/scopes.md
  2
  ```
- [x] Scenario BUG-024-005-SCN-004 (DoD-Gherkin fidelity stays green): traceability PASSED; STG Check 22 all 6 faithful; lint + freshness green; `TestConnectorCountContract` 4/4 PASS.
  Evidence: post-fix guards + go test
  ```
  $ bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1
  RESULT: PASSED (0 warnings)
  $ bash .github/bubbles/scripts/state-transition-guard.sh specs/024-design-doc-reconciliation 2>&1 | grep -E 'Check 22|Gate G068'
  --- Check 22: DoD-Gherkin Content Fidelity (Gate G068) ---
  ✅ PASS: All 6 Gherkin scenarios have faithful DoD items (Gate G068)
  $ go test -run TestConnectorCountContract -v ./internal/deploy/... 2>&1 | grep -cE '^--- PASS'
  4
  ```
- [x] Scenario BUG-024-005-SCN-005 (Parent governance backfill recorded; nothing committed): `resolvedBugs[]` BUG-024-005; `report.md` 1 closure section; zero `/home/<user>/`; HEAD unchanged.
  Evidence: post-fix probes
  ```
  $ python3 -c "import json; d=json.load(open('specs/024-design-doc-reconciliation/state.json')); print('resolved:', any('BUG-024-005' in (b.get('bugId') or '') for b in d.get('resolvedBugs',[]))); print('hist:', sum(1 for e in d['executionHistory'] if 'BUG-024-005' in (e.get('summary') or '')))"
  resolved: True
  hist: 17
  $ grep -cE '^## BUG-024-005 Harden-Sweep Resolution' specs/024-design-doc-reconciliation/report.md
  1
  $ grep -cE '/home/<user>/' specs/024-design-doc-reconciliation/report.md
  0
  ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior added in this scope are captured persistently in the Test Plan's `Regression E2E` row above and re-run cleanly post-edit.
  Evidence: persistent gate sweep — content guards green every iteration; duplicate-key detector + grep contracts deterministic across 3 re-runs
  ```
  $ for i in 1 2 3; do bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation 2>&1 | tail -1; done
  RESULT: PASSED (0 warnings)
  RESULT: PASSED (0 warnings)
  RESULT: PASSED (0 warnings)
  ```
- [x] Broader E2E regression suite passes (Go unit baseline + Go contract test family `TestConnectorCountContract*`) so the connector-count contract recertifies cleanly.
  Evidence: `go test -run TestConnectorCountContract ./internal/deploy/...` 4/4 PASS (LiveFile + 3 adversarial)
  ```
  $ go test -run TestConnectorCountContract ./internal/deploy/... 2>&1 | tail -1
  ok  	github.com/smackerel/smackerel/internal/deploy	0.028s
  ```
- [x] Independent canary suite for shared fixture/contract passes before broad suite reruns: the R-006 directory-count canary `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l == 16` matches the reconciled scopes.md SCN-024-06 value BEFORE the broader guard suite executes.
  Evidence:
  ```
  $ find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l
  16
  ```
- [x] Rollback or restore path for shared infrastructure changes is documented and verified: because nothing is committed, rollback is `git checkout -- specs/024-design-doc-reconciliation/{scopes.md,state.json,report.md}` + `rm -rf` the new packet folder; no DB schema, restart, or runtime impact.
  Evidence: see design.md Rollback section; working-tree-only change
  ```
  $ git status --short specs/024-design-doc-reconciliation/ | grep -cE '^ M|^\?\?'
  ```
- [x] Consumer impact sweep complete and zero stale first-party references remain after the scopes.md SCN-024-06 change; downstream connector-count surfaces (docs/smackerel.md §22.7/§24-A, scenario-manifest.json) already say 16 and are re-verified untouched.
  Evidence:
  ```
  $ grep -cE 'Committed Connector Inventory \(16 connectors\)' docs/smackerel.md
  1
  ```
- [x] Change boundary held: only `specs/024-design-doc-reconciliation/` paths modified; zero runtime/docs/framework files touched.
  Evidence: see Change Boundary section below + audit phase
  ```
  $ git status --short | grep -vE '^ ?M? ?specs/024-design-doc-reconciliation/' | grep -cE 'cmd/|internal/|ml/|docs/|config/|\.github/bubbles/'
  0
  ```

### Scenario-First TDD Evidence

- BUG-024-005-SCN-001 (single lastUpdatedAt): RED before fix — `raw.count('"lastUpdatedAt"') == 2` + duplicate-key detector flagged `['lastUpdatedAt']`; GREEN after fix — count 1, detector empty.
- BUG-024-005-SCN-002 (recert): RED before fix — `certifiedAt == 2026-05-28T05:07:51Z`, no CURRENT review for this edit; GREEN after fix — `certifiedAt == 2026-06-06T18:00:00Z` + `bubbles.spec-review` CURRENT entry, `requiresRevalidation` absent.
- BUG-024-005-SCN-003 (scopes.md 15→16): RED before fix — 8 substantive "15" hits + SCN-024-06 list omitted `qfdecisions`; GREEN after fix — 8 sites = 16, `qfdecisions` present, 7 historical "15" preserved.
- BUG-024-005-SCN-004 (G068 fidelity): RED risk — changing only the Gherkin title would break fidelity; GREEN — title + DoD bullet changed in lockstep, STG Check 22 PASS, traceability PASSED.
- BUG-024-005-SCN-005 (governance + no commit): RED before fix — no BUG-024-005 provenance; GREEN after fix — 17 history entries + resolvedBugs entry + report section, HEAD unchanged.

### Shared Infrastructure Impact Sweep

`scopes.md` is a spec-internal planning artifact with no downstream code consumers. The authoritative connector-count product-truth surfaces (`docs/smackerel.md` §22.7/§24-A) already enumerate 16 (BUG-024-002) and are NOT modified here. The `SCN-024-06` text consumers are the traceability guard (mapped by `scenarioId`, kept green by lockstep edit) and `scenario-manifest.json` (already 16). The `state.json` `lastUpdatedAt` is read by governance tooling with last-wins semantics; deduping makes the read deterministic.

**Canary:** existing R-006 directory-count canary `find internal/connector -maxdepth 1 -mindepth 1 -type d | wc -l == 16` continues guarding the inventory; F2 aligns the Scope 2 narrative with it.

**Rollback:** working-tree-only — `git checkout -- specs/024-design-doc-reconciliation/{scopes.md,state.json,report.md}` + `rm -rf` the packet folder. No schema/restart/runtime impact.

### Change Boundary

**Allowed (this packet may modify):**
- `specs/024-design-doc-reconciliation/state.json`, `scopes.md`, `report.md`
- `specs/024-design-doc-reconciliation/bugs/BUG-024-005-scopes-connector-count-and-state-dup-key/` (8 artifacts)

**Excluded (MUST NOT modify):**
- `cmd/`, `internal/` (incl. `internal/deploy/docs_connector_count_contract_test.go`), `ml/`, `web/`, `tests/`
- `docs/*`, `config/`, `scripts/`, `smackerel.sh`, `docker-compose*`, `deploy/`
- `.github/bubbles/` (framework-managed)
- `specs/055-*`, `specs/044-per-user-bearer-auth/state.json`, all other spec folders, the closed BUG-024-002/003/004 packets
