# Code-Index Onboarding (Smackerel)

**Status:** ONBOARDED 2026-07-29. `codeIndex.adapter: codegraph` is set in
`.github/bubbles-project.yaml` and a local index exists. The seam is
ADVISORY-ONLY — no gate consumes it and it is never a blocking verdict.
Reversal is at the end of this file.

**Measured index (2026-07-29):** 2,331 files → 40,009 nodes / 114,711 edges,
~132 MB. `routes` returns 444 entries.

**Every clone must rebuild.** `.codegraph/` is gitignored, so the index is NOT
distributed. A fresh clone resolves `adapter=codegraph` but has no index, and the
adapter then exits 1 ("could not look") rather than returning a misleading `[]`.
Run the `codegraph init` step below after cloning.

**Framework contract:** [`bubbles-code-index-adapter`](../.github/skills/bubbles-code-index-adapter/SKILL.md)
(skill, vendored) — the generic doctrine, the 5-verb contract, and the rules for
consuming derived facts. This document is the Smackerel-specific part only.

---

## Fit verdict: ADOPT

Measured composition (`git ls-files`):

| Language | Files | Provider support |
|----------|-------|------------------|
| Go | 2,022 | Full |
| Shell | 541 | **Not parsed** by `codegraph` |
| Python (`ml/`) | 78 | Full |
| TypeScript | 70 | Full |

The Go core runtime and the Python ML sidecar are both fully supported. The
shell files are operator tooling (`./smackerel.sh` and friends), not a surface
any contract gate reasons about.

---

## What it would close in THIS repo

**1. No impact-aware validation, against the largest test suite in the
portfolio.** No `testImpact:` map in `.github/bubbles-project.yaml`, so every
change runs the full suite against **1,256 Go test files**. This is the single
biggest available saving of any repo here. `affected` derives the reachable
subset from the real import graph rather than a hand-maintained list.

**2. The isolated-ML-sidecar invariant is grep-enforced.** The framework rule
([`bubbles-isolated-ml-sidecar`](../.github/skills/bubbles-isolated-ml-sidecar/SKILL.md))
says the Python tier is compute-only and must never hold datastore credentials.
Today that is checked by pattern-matching imports. With 78 Python files indexed,
`impact` answers the transitive question — *does anything reachable from the
sidecar touch a datastore symbol* — instead of the surface question of whether a
specific driver name appears in a specific file.

**3. No API-contract audit.** 204 files register routes; there is no
audit/openapi script in `scripts/`. `routes` gives a derived inventory to diff
against, which is exactly the check the sibling QF repo has (and whose scope was
found to be too narrow — see QF BUG-002). Smackerel has no such check to narrow.

---

## Onboarding steps

```bash
# 1. Install the provider in an isolated prefix (never global)
npm install --prefix ~/.cache/codegraph-eval @colbymchenry/codegraph

# 2. .gitignore already carries `.codegraph/` — verify before indexing
grep -n 'codegraph' .gitignore

# 3. Build the index with provider telemetry OFF
cd /path/to/smackerel
CODEGRAPH_TELEMETRY=0 DO_NOT_TRACK=1 CODEGRAPH_NO_DAEMON=1 \
  ~/.cache/codegraph-eval/node_modules/.bin/codegraph init

# 4. Opt in
#    .github/bubbles-project.yaml:
#      codeIndex:
#        adapter: codegraph

# 5. Verify
bash .github/bubbles/scripts/codeindex-resolve.sh --repo-root . --names-only
bash .github/bubbles/adapters/codeindex/codegraph.sh status
```

**WSL note:** keep the checkout on the Linux-native filesystem. Under `/mnt/c`
SQLite WAL cannot be enabled and reads block on writes.

---

## Reversal

```bash
~/.cache/codegraph-eval/node_modules/.bin/codegraph uninit --force
rm -rf ~/.cache/codegraph-eval
# set codeIndex.adapter back to none (or remove the block)
```

---

## Constraints

- **Advisory only.** Never put a third-party index behind a blocking gate. The
  sidecar-isolation rule keeps its existing enforcement; the index adds reach,
  not authority.
- **`[]` is not exit 1.** Exit 0 with `[]` means "indexed, found nothing". Exit 1
  means "could not look". Conflating them reports clean for an unindexed repo.
- **Shell is unparsed.** `./smackerel.sh` and `scripts/**` are invisible to the
  index. Any check over operator tooling stays grep-based.
- **Not for agent context.** It does not reduce prompt-bundle cost.
- **NO-DEFAULTS still applies.** Provider env vars are analysis-tool config, not
  SST runtime config — they must not enter `config/smackerel.yaml`.
- **Terminal discipline still applies.** Provider commands do not replace
  `./smackerel.sh` for any build, test, or deploy operation.
