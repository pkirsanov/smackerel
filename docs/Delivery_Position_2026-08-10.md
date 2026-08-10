# Delivery Position — Verified 2026-08-10

Companion to `Product_Delivery_Plan.md`, `Product_Direction_2026-07-31.md`, and
`Release_Schema_Review_2026-08-02.md`. Those three documents were written on
2026-08-02. This note re-measures their claims 8 days and 34 commits later so a
reader knows which numbers still hold.

Every figure below was produced by running a command, not by reading a document.

**Acted on.** `Product_Delivery_Plan.md` has been updated from these findings: it
now carries the re-measured baseline, routes specs 110/111/112 into Stages 4, 6 and
the parallel track, and adds §5 "Execution plan — how we get there".
`Strategy.md` and `Release_Schema_Review_2026-08-02.md` (F1 marked resolved) were
updated to match.

## Headline

**Smackerel's backend is built. The product is not assembled.**

99 specs are `done`. The three specs that constitute a fully functional
smackerel — 105, 106, 107 — are **80 of 537 DoD items complete (15%), and 0 of
those 80 represent shipped pillar behaviour.** Spec 105, the entire connected
knowledge-graph explorer, has not completed a single DoD item.

The constraint is not backend capability. It is the exposure layer.

## Verification Table

| Claim (2026-08-02) | Measured (2026-08-10) | Status |
|---|---|---|
| 109 specs | **112** | +3 new |
| 99 done / 4 hardened / 4 blocked / 2 in_progress | identical | holds |
| — | **3 `not_started`** | new, undocumented in plan |
| 105: 0/139 DoD | 0/139 (0%) | unchanged |
| 106: 41/238 DoD | 41/238 (17%) | unchanged |
| 107: 39/160 DoD | 39/160 (24%) | unchanged |
| 31 PWA pages, 2 with shared nav | 31 / 2 | exact |
| 27 scenario contracts | 27 | exact |
| 20 catalog surfaces | 20 | exact |
| **F1: G110 release-train guard FAILS (exit 1, 7 errors)** | **working tree exit 0; HEAD exit 1, 8 errors** | **see correction** |
| D27: eval suite in no automated lane | still absent from `go-integration.sh` | open |
| D28: scope short-circuit at `scope_middleware.go:71,75` | both cases present | open |
| D25: model-supplied identity in retrieval tool | 4 `user_id` references | open |

## What Changed In 8 Days

**F1 — corrected 2026-08-10, after first publication.**
The first version of this note recorded F1 as resolved by the 34 commits. That was
wrong, and the error is instructive: `release-train-guard.sh` was run against the
**working tree**, which exits 0. Run against `HEAD` alone it still **exits 1 with 8
errors**. The fix exists only as uncommitted local changes to
`config/release-trains.yaml` and six `state.json` files — the in-flight work of
`specs/_ops/OPS-006-local-git-reconciliation` (`in_progress`, `stabilize-to-doc`),
owned by a concurrent session. **No commit fixes F1.**

Errors still present at HEAD:

- train `mvp` has invalid `target_slot 'self-hosted'` (expected `prod|staging|home-lab|none`) — F1a
- six `in_progress` specs carry no `releaseTrain`: `BUG-069-004`, `BUG-061-008`,
  `BUG-061-007`, `BUG-061-006`, `BUG-003-002` (the review's five) plus
  `OPS-006-local-git-reconciliation`, added since the review

The lesson is the same one recommendation #4 of the review already stated, one
level deeper: running the gate is necessary but not sufficient — **it matters
which tree you run it against.** A working-tree pass can be someone else's
uncommitted work.

**Three new specs, correctly routed.** `110-retrieval-quality-foundation` (plan
problem P8, Stage 4), `111-corpus-portability-sensitivity` (parallel track,
D11/D12/D23), `112-capability-registry` (Pillar C, Stage 6). The planning
apparatus is converting the diagnostic into artifacts.

**Where the 34 commits went.** 266 files under `.github/bubbles`, 50 under
`.github/docs`, 38 under `.github/agents` — a framework upgrade. Product work
landed in `061-conversational-assistant` (16 files) and `internal/assistant`
(15), with 13 in `tests/integration`.

**Not moved: the pillars.** Zero DoD items completed across 105/106/107.
**Not moved: every Stage-1 critical.** All three re-verified as open above.

## Position By Pillar

**Pillar A — LLM wiki. Not started.** 0/139. The graph exists in `internal/graph`;
no explorer surface consumes it.

**Pillar B — second brain. Partially planned, not exposed.** 41/238. The decisive
number is 31 PWA pages with 2 sharing navigation: the surfaces exist as isolated
pages, not as a product a user can move through.

**Pillar C — extended scenarios. Planned, gated on the registry.** 39/160, with
`112-capability-registry` now specced. 27 scenario contracts exist; 5 reach a
user. The catalog (20 surfaces) is how the assistant discovers capability, so
the registry is the unlock for the other 22 contracts.

## Honest Read

Stage 0 of 6. The 8-day delta shows planning discipline working — a Critical gate
repaired, three problems converted to specs — and no pillar construction. The
next unit of progress that changes the headline is DoD completion on 105/106/107,
not further specification.

The three Stage-1 criticals (D25, D27, D28) remain open and are each small,
bounded, and independent of pillar work. D27 in particular — wiring the existing
`tests/eval` suite into `go-integration.sh` — is a configuration change guarding
an acceptance gate that currently runs in no automated lane.

## Reproduce

```bash
# G110 — run against BOTH trees; they disagree
bash .github/bubbles/scripts/release-train-guard.sh "$(pwd)"                    # working tree → 0
git stash -u && bash .github/bubbles/scripts/release-train-guard.sh "$(pwd)"    # HEAD alone   → 1
git stash pop
for s in 105-* 106-* 107-*; do
  d="specs/$s/scopes"
  echo "$s $(grep -h '^- \[x\]' $d/*/scope.md | wc -l)/$(cat $d/*/scope.md | grep -cE '^- \[[ x]\]')"
done
grep -n 'tests/eval' scripts/runtime/go-integration.sh                # D27
grep -nE 'SessionSourceSharedToken|SessionSourceBootstrap' internal/auth/scope_middleware.go  # D28
```
