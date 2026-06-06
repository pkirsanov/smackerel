# Execution Report — BUG-003: `docs/Development.md` inventory drift with no freshness guard

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

DevOps stochastic-quality-sweep Round 1 (parent-expanded `devops-to-doc`) probed
spec 032's managed documentation against on-disk reality and found one concrete
drift plus a systemic gap:

- **Drift (F1):** `internal/scopesdriftguard/` (added 2026-06-05) was absent from
  the `docs/Development.md` Go Packages table — 33 package directories on disk vs
  32 documented, violating spec 032's acceptance criteria.
- **Automation gap (F2):** no mechanical freshness guard existed, so the drift was
  invisible. Spec 032's `design.md` had named this exact CI freshness check as the
  Risk #1 mitigation but never built it.

Both were closed: the package was documented and a fail-loud Go contract test was
added that enforces all three inventories (packages, migrations, prompt contracts)
going forward.

---

## Test Evidence

**Executed:** YES
**Phase Agent:** bubbles.test
**Command:** `./smackerel.sh test unit --go --go-run 'TestDocFreshness' --verbose`

### Before Fix (RED)

The new guard was authored first and run **before** the documentation was
corrected. It reproduced the drift (`internal/scopesdriftguard/` and the new
`internal/docfreshness/` undocumented):

```
$ ./smackerel.sh test unit --go --go-run 'TestDocFreshness' --verbose
=== RUN   TestDocFreshness_AllInternalPackagesDocumented
    doc_freshness_test.go:161: internal/ package freshness: 34 packages on disk, 2 undocumented
    doc_freshness_test.go:163: docs/Development.md is STALE: 2 internal/ package(s) exist on disk but are undocumented: docfreshness, scopesdriftguard
--- FAIL: TestDocFreshness_AllInternalPackagesDocumented (0.00s)
    doc_freshness_test.go:182: migration freshness: 38 migration files on disk, 0 undocumented
--- PASS: TestDocFreshness_AllMigrationsDocumented (0.00s)
    doc_freshness_test.go:203: prompt-contract freshness: 21 contracts on disk, 0 undocumented
--- PASS: TestDocFreshness_AllPromptContractsDocumented (0.00s)
--- PASS: TestDocFreshness_AdversarialUndocumentedItemsDetected (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/docfreshness    0.018s
```

Independent probes confirming the drift:

```
$ find internal -mindepth 1 -maxdepth 1 -type d | wc -l
33
$ grep -c 'scopesdriftguard' docs/Development.md
0
```

(34 vs 33: the guard counts itself — `internal/docfreshness/` — so the
pre-documentation count of undocumented packages is the 2 newly-added guard
packages, `scopesdriftguard` and `docfreshness`; the on-disk count of 33 is the
pre-existing `scopesdriftguard` drift the probe is responsible for.)

### After Fix (GREEN)

Added the two missing Go Packages rows to `docs/Development.md`, then re-ran the
same command — all four tests pass:

```
$ ./smackerel.sh test unit --go --go-run 'TestDocFreshness' --verbose
=== RUN   TestDocFreshness_AllInternalPackagesDocumented
    doc_freshness_test.go:161: internal/ package freshness: 34 packages on disk, 0 undocumented
--- PASS: TestDocFreshness_AllInternalPackagesDocumented (0.00s)
=== RUN   TestDocFreshness_AllMigrationsDocumented
    doc_freshness_test.go:182: migration freshness: 38 migration files on disk, 0 undocumented
--- PASS: TestDocFreshness_AllMigrationsDocumented (0.00s)
=== RUN   TestDocFreshness_AllPromptContractsDocumented
    doc_freshness_test.go:203: prompt-contract freshness: 21 contracts on disk, 0 undocumented
--- PASS: TestDocFreshness_AllPromptContractsDocumented (0.00s)
=== RUN   TestDocFreshness_AdversarialUndocumentedItemsDetected
--- PASS: TestDocFreshness_AdversarialUndocumentedItemsDetected (0.00s)
ok      github.com/smackerel/smackerel/internal/docfreshness    0.020s
```

All three inventory checks pass (34 packages, 38 migrations, 21 contracts — 0
undocumented) and the adversarial anti-tautology test passes.

---

## Regression Evidence

**Executed:** YES
**Phase Agent:** bubbles.test / bubbles.regression

The adversarial anti-tautology test `TestDocFreshness_AdversarialUndocumentedItemsDetected`
is the persistent regression. It builds a synthetic repo whose `docs/Development.md`
documents nothing, then asserts:

- the package scan reports exactly `[ghostpkg]` missing,
- the migration scan reports exactly `[099_ghost.sql]` missing,
- the contract scan reports exactly `[ghost-extraction-v1.yaml]` missing,
- and a doc that documents all three reports zero missing (green path).

It is part of the `internal/docfreshness` package and runs on every
`go test ./...`; it would fail if the guard were neutered or the bug reintroduced
(a new package added without a doc row). Result for this round: PASS (the
`ok internal/docfreshness` line above includes it).

---

## Audit Evidence

**Executed:** YES
**Phase Agent:** bubbles.audit

```
$ ./smackerel.sh format --check ; echo exit $?
exit 0
$ ./smackerel.sh lint ; echo exit $?
... All checks passed!            (ruff, Python)
=== Validating web manifests ===  (all OK)
Web validation passed
exit 0
```

`gofmt -l` clean for the new file, `go vet ./...` clean (lint exit 0), web
validation clean. The only changed Go file is the new test (alongside the two
documentation rows):

```
$ git --no-pager status --short -- internal/docfreshness/ docs/Development.md
 M docs/Development.md
?? internal/docfreshness/
```

### Code Diff Evidence

**Executed:** YES
**Phase Agent:** bubbles.audit
**Command:** `git status --short` / `git diff --stat`

The delivery is one new test-only Go source file plus two `docs/Development.md`
Go Packages rows — zero runtime/source/config files changed:

```
$ git status --short -- internal/docfreshness/ docs/Development.md
 M docs/Development.md
?? internal/docfreshness/
$ git diff --stat -- docs/Development.md
 docs/Development.md | 2 ++
 1 file changed, 2 insertions(+)
$ wc -l internal/docfreshness/doc_freshness_test.go
281 internal/docfreshness/doc_freshness_test.go
```

New file (untracked): `internal/docfreshness/doc_freshness_test.go` — 281 lines,
package `docfreshness`. The two `docs/Development.md` insertions are the
`internal/scopesdriftguard/` and `internal/docfreshness/` Go Packages rows; the
full added hunk is shown by `git diff -- docs/Development.md`.

---

## Validation Evidence

**Executed:** YES
**Phase Agent:** bubbles.validate

- F1 closed: `docs/Development.md` now documents all 33 on-disk `internal/`
  packages (34 including the new guard package) — acceptance-criteria restored.
- F2 closed: the guard runs under the Go unit suite and CI with no new CLI/CI
  surface; any future drift in packages, migrations, or contracts fails loud.
- Migration (38) and prompt-contract (21) inventories verified already fresh and
  now mechanically protected.

---

## DevOps / Chaos / Simplify / Stabilize / Security Notes

| Phase | Outcome |
|-------|---------|
| devops | Probe identified the inventory drift + the missing freshness control; both remediated this round. |
| chaos | The adversarial synthetic-repo test is the chaos probe — it perturbs the inventory/doc relationship and asserts detection. PASS. |
| simplify | No simplification opportunity — the guard is a single ~230-line test file with shared pure helpers; no duplication introduced. |
| stabilize | No runtime instability surface — the change is a test-only file plus documentation rows; product runtime byte-identical. |
| security | No secrets, no network, no PII; the guard only reads repo files. `gofmt`/`go vet` clean. |

---

## Completion Statement

Both findings closed one-to-one. `internal/scopesdriftguard/` and the new
`internal/docfreshness/` package are documented in `docs/Development.md`; the
`internal/docfreshness` contract test enforces all three managed inventories.
Red→green reproduced with real output; adversarial regression green; format,
vet, and web validation clean. No commit performed — changes left in the working
tree for the parent batch-commit per the sweep contract.
