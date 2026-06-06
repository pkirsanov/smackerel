# Design — BUG-003: `docs/Development.md` inventory drift with no freshness guard

> **Bug:** [spec.md](spec.md) | **Parent design:** [../../design.md](../../design.md)
> **Author:** bubbles.workflow (DevOps stochastic-quality-sweep Round 1, parent-expanded `devops-to-doc`)
> **Date:** 2026-06-06
> **Status:** Implemented

---

## Root Cause Analysis

Spec 032 ("Documentation Freshness") set acceptance criteria that
`docs/Development.md` list **every** Go package under `internal/`, **every**
migration, and **every** prompt contract. Those inventories are maintained by
hand. The spec's `design.md` Risk table identified the obvious failure mode:

> | 1 | Docs go stale again after initial update | CI freshness check (spec 029 coordination) |

and the testing-strategy table listed an **optional** "docs-freshness check
comparing documented packages to `go list ./...`". That control was never built.

Consequence: when `internal/scopesdriftguard/` was added on 2026-06-05, nothing
forced its documentation. By 2026-06-06 the Go Packages table listed 32 of the 33
package directories — a silent acceptance-criteria violation. The migration (38)
and prompt-contract (21) tables happened to still be current, but only by manual
diligence; the same drift could recur for any of the three inventories.

This is a **doc-freshness automation gap** (a DevOps finding): the documentation
the spec is responsible for has no mechanical staleness control.

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D-001 | Remediate the concrete drift by documenting `internal/scopesdriftguard/` (and the new guard package `internal/docfreshness/`) in the Go Packages table. | Restores spec 032 acceptance-criteria compliance for the current tree. |
| D-002 | Deliver the deferred "Risk #1 mitigation" as a Go contract test, not a new CLI subcommand or CI-workflow edit. | Mirrors the repo's existing guard pattern (`internal/scopesdriftguard/`, `internal/deploy/compose_contract_test.go`). It runs automatically under `./smackerel.sh test unit --go` and CI with **zero** new CLI/CI/compose surface — lowest blast radius, no shared-infra change. |
| D-003 | Package-detection semantics match the spec 032 probe (`find internal -mindepth 1 -maxdepth 1 -type d`), restricted to directories containing Go source anywhere in their tree. | Includes test-only guard packages and container packages whose Go lives only in subdirectories (`internal/whatsapp/`). Avoids false negatives. |
| D-004 | Membership test is substring containment of `internal/<pkg>/`, the migration filename, and the contract filename in `docs/Development.md`. | Matches the table-row form (`` `internal/<pkg>/` ``) already used by every documented entry; simple, deterministic, no false-pass for the real inventories. |
| D-005 | Include an adversarial anti-tautology test that builds a synthetic repo whose `Development.md` documents nothing, asserts each scan reports exactly its synthetic missing item, and proves the green path passes once documented. | Required by the bug-template adversarial-regression rule; proves the guard is not vacuous and would fail if the bug were reintroduced. |
| D-006 | No fallback/default values anywhere; the guard fails loud (test failure with the exact missing item) on drift. | Honors the Smackerel NO-DEFAULTS / fail-loud SST policy. |

## Fix Design

### Part A — Remediate current drift (`docs/Development.md`)

Add two rows to the Go Packages table:

- `` `internal/scopesdriftguard/` `` — test-only scopes-path drift ratchet (the package that triggered this bug).
- `` `internal/docfreshness/` `` — the new freshness guard package itself (self-consistent: the guard requires its own package to be documented).

### Part B — Mechanical guard (`internal/docfreshness/doc_freshness_test.go`)

A new test-only package `docfreshness` with:

| Test | Asserts |
|------|---------|
| `TestDocFreshness_AllInternalPackagesDocumented` | every `internal/` package directory (Go-source-bearing) appears in `docs/Development.md` |
| `TestDocFreshness_AllMigrationsDocumented` | every `internal/db/migrations/*.sql` filename appears in `docs/Development.md` |
| `TestDocFreshness_AllPromptContractsDocumented` | every `config/prompt_contracts/*.yaml` filename appears in `docs/Development.md` |
| `TestDocFreshness_AdversarialUndocumentedItemsDetected` | against a synthetic repo, each scan reports exactly its synthetic missing item AND a documented item is reported present (anti-tautology + green-path) |

Repo root is resolved from the test file location via `runtime.Caller(0)` (two
parents up), matching the existing `scopesdriftguard` pattern, so the test is
working-directory independent.

## Change Boundary

| Allowed surfaces | Excluded surfaces |
|------------------|-------------------|
| `internal/docfreshness/doc_freshness_test.go` (new test-only file) | Any runtime `.go` (non-test), `.py`, `.sql`, `.proto` |
| `docs/Development.md` (Go Packages table: +2 rows, no removals) | `config/`, `docker-compose*.yml`, `Dockerfile`, `ml/Dockerfile` |
| Bug-packet artifacts under this folder | `.github/bubbles/**`, `.github/workflows/**`, `scripts/commands/build-home-lab.sh` (in-progress external work) |
| Parent `report.md` (DevOps-sweep evidence breadcrumb only) | Parent `spec.md` / `design.md` / `scopes.md` (no planning-truth edit → no recert needed) |

## Consumer Impact Sweep

| Consumer surface | Impact | Action |
|------------------|--------|--------|
| `./smackerel.sh test unit --go` + CI (`go test ./...`) | Gains 4 new fast file-reading tests in `internal/docfreshness`. | None — passing on this tree; no runtime deps, no network, no DB. |
| `docs/Development.md` readers | Two new Go Packages rows; existing anchors and rows unchanged. | None — additive. |
| Future package/migration/contract authors | Adding one without documenting it now fails the Go unit suite. | Intended new control; documented in the new package's doc comment and the table row. |
| `internal/scopesdriftguard/` | Unrelated guard; not modified. | None. |

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Go contract test (red→green) | guard fails before the doc rows are added, passes after | [report.md](report.md) Before/After |
| Adversarial regression | synthetic undocumented item detected; green path passes | `TestDocFreshness_AdversarialUndocumentedItemsDetected` |
| Format/vet/lint | `gofmt -l` clean, `go vet ./...` clean, web validation clean | [report.md](report.md) Audit Evidence |
