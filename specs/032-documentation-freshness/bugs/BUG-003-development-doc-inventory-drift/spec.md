# BUG-003 — `docs/Development.md` inventory drift (undocumented package) with no freshness guard

> **Parent feature:** [../../spec.md](../../spec.md) — 032 Documentation Freshness
> **Bug ID:** BUG-003-development-doc-inventory-drift
> **Severity:** MEDIUM (managed-doc correctness; no runtime impact)
> **Status:** Resolved — fix delivered 2026-06-06 (DevOps stochastic-quality-sweep Round 1)

---

## Summary

Spec 032's acceptance criteria require `docs/Development.md` to list **every** Go
package under `internal/`, **every** migration on disk, and **every** prompt
contract on disk. A DevOps probe on 2026-06-06 found that `docs/Development.md`
had drifted: the test-only package `internal/scopesdriftguard/` (added
2026-06-05) was absent from the Go Packages table — **33 package directories on
disk vs 32 documented**.

The deeper defect is systemic: spec 032's own `design.md` named a "CI freshness
check comparing documented packages to `go list ./...`" as the **Risk #1
mitigation** ("Docs go stale again after initial update"), but it was left
*optional/deferred*. With no mechanical guard, the inventory drifted invisibly —
exactly the failure mode the spec exists to prevent. The migration (38) and
prompt-contract (21) inventories were still fresh, but nothing guaranteed they
would stay that way.

## Observed vs Expected

| | Observed (before fix) | Expected |
|---|---|---|
| `internal/` packages on disk | 33 | — |
| `internal/` packages in `docs/Development.md` | 32 (`internal/scopesdriftguard/` missing) | 33 (all documented) |
| Automated freshness guard | none | a mechanical guard fails CI / the Go unit suite on any future drift |

## Expected Behavior Specification

1. `docs/Development.md` MUST reference every top-level `internal/` directory that
   contains Go source anywhere in its tree (the spec 032 freshness probe is
   `find internal -mindepth 1 -maxdepth 1 -type d`).
2. `docs/Development.md` MUST reference every `internal/db/migrations/*.sql` file
   and every `config/prompt_contracts/*.yaml` file.
3. A mechanical, fail-loud guard MUST detect any future drift in (1) and (2) so
   that adding a package/migration/contract without documenting it fails the Go
   unit suite and CI — turning the design's deferred "Risk #1 mitigation" into a
   live control.

## Reproduction Steps

1. `find internal -mindepth 1 -maxdepth 1 -type d | wc -l` → 33 on disk.
2. `grep -c 'scopesdriftguard' docs/Development.md` → 0 (undocumented).
3. With the new guard present, `./smackerel.sh test unit --go --go-run 'TestDocFreshness'`
   fails with `STALE: ... undocumented: docfreshness, scopesdriftguard`.

Full raw before/after captures are in [report.md](report.md).

## Root Cause

See [design.md](design.md). In short: documentation inventories were maintained
by hand with no mechanical guard, so a newly added package silently violated
spec 032's acceptance criteria. The spec's intended CI freshness check was never
implemented.

## Fix Summary

1. Document the missing `internal/scopesdriftguard/` package (and the new guard
   package `internal/docfreshness/`) in the `docs/Development.md` Go Packages table.
2. Add `internal/docfreshness/doc_freshness_test.go` — a fail-loud contract test
   that asserts every `internal/` package, every migration, and every prompt
   contract is documented in `docs/Development.md`, with an adversarial
   anti-tautology case. It runs under `./smackerel.sh test unit --go` and CI with
   no new CLI/CI surface.
