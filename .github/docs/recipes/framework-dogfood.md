# Recipe: Framework Dogfooding (Gate G085)

> **Gate:** G085 - `framework_dogfood_evidence_gate`
> **Guard:** `bubbles/scripts/framework-dogfood-guard.sh`
> **Selftest:** `bubbles/scripts/framework-dogfood-guard-selftest.sh`
> **Durable docs:** `docs/Framework_Convergence_Health.md`, `docs/Spec_Implementation_Alignment.md`

## Rule

The Bubbles source repository MUST NOT contain a persistent `specs/` tree.

Bubbles still dogfoods itself, but its source-repo evidence comes from durable framework surfaces:

- Framework validation: `bash bubbles/scripts/framework-validate.sh`
- Hermetic selftests: `bubbles/scripts/*-selftest.sh`
- Release manifest freshness: `bash bubbles/scripts/generate-release-manifest.sh --check`
- Fixture specs created inside temporary selftest workspaces
- Downstream or external dogfood specs in product repositories that install Bubbles

If a temporary source-repo spec is created during framework work, harvest its durable design and behavior into docs and framework assets, then remove the `specs/` tree before validation and release hygiene proceed.

## Why

The framework source checkout is the upstream product for Bubbles itself. Keeping repo-local execution packets in that checkout creates two competing truth stores: transient spec artifacts and durable framework documentation/assets. The durable truth belongs in docs, scripts, workflows, agents, generated manifests, and release notes.

Downstream repositories are different. They are product repos using Bubbles to deliver product work, so their feature, bug, and ops packets still live under `specs/` according to the normal Bubbles taxonomy.

## G085 Enforcement Model

`framework-dogfood-guard.sh` is source-aware:

| Repository class | Pass condition | Failure condition |
|------------------|----------------|-------------------|
| Bubbles source repository | No persistent `specs/` directory, plus framework validation/release evidence surfaces are present | `specs/` exists, or required evidence surfaces are missing |
| Downstream or hermetic fixture repository | `G085-CURRENT-DONE`: at least one current `specs/[0-9]*-*/state.json` has exact top-level `status: done`; or `G085-FIRST-ADOPTION`: current numbered states exist, current done is zero, local Git history is complete, and every reachable ref contains zero numbered top-level done-state blobs | No current numbered state; reachable historical done evidence with zero current done; malformed state; or unavailable, shallow, partial, malformed, or failed history evidence |

The source-repo pass condition intentionally differs from downstream. Source dogfooding is proved by the framework validating itself and by fixture/downstream specs exercising installed behavior, not by storing Bubbles' own work packet forever.

### Downstream Decision Order

G085 evaluates downstream repositories in a closed order:

1. Parse every current numbered top-level state. Malformed current JSON exits
	`2` with `E085-CURRENT-STATE-MALFORMED`.
2. If one or more current states have exact top-level `status: done`, pass with
	`G085-CURRENT-DONE`. This fast path does not require Git history.
3. If no current numbered state exists, exit `1` with
	`E085-NO-CURRENT-SPEC`. An empty inventory is not first adoption.
4. Otherwise, require the requested path to be the exact physical Git worktree
	root with complete, non-shallow, non-partial local history. Missing or nested
	roots, shallow clones, partial/promisor metadata, failed traversal, and
	malformed historical state exit `2` with distinct `E085-*` integrity codes.
5. Scan commits reachable from every local ref. If any numbered top-level
	historical state blob has exact top-level `status: done`, exit `1` with
	`E085-ESTABLISHED-DONE-REMOVED` and identify its commit and path without
	printing blob content.
6. Only a complete scan with zero historical done blobs passes as
	`G085-FIRST-ADOPTION`, reporting `currentDone=0`, `historicalDone=0`, and
	`historyIntegrity=complete`.

First adoption is derived from existing repository evidence. There is no
adoption flag, install timestamp, cache, environment override, network lookup,
or bypass option. The guard is read-only and does not fetch, checkout, reset,
commit, or mutate refs, the index, worktree, or object database.

## Source-Repo Dogfood Evidence Checklist

Before claiming a Bubbles source change is ready:

1. Run or verify `bash bubbles/scripts/framework-validate.sh`.
2. Run or verify `bash bubbles/scripts/generate-release-manifest.sh --check`.
3. Confirm the relevant guard selftests are wired into `framework-validate.sh`.
4. Confirm durable behavior is documented in `docs/`, `README.md`, shared governance docs, and `CHANGELOG.md` as appropriate.
5. Confirm the Bubbles source repo has no `specs/` tree.

## Temporary Spec Migration

When a temporary Bubbles source spec exists:

1. Read `spec.md`, `design.md`, scopes, reports, and state to identify durable behavior and design decisions.
2. Publish current behavior into managed or explicitly targeted docs.
3. Update framework assets when the implementation itself still carries the stale spec-era rule.
4. Preserve evidence only as release notes or validation output summaries; do not keep the source spec as an archive.
5. Delete the `specs/` tree and rerun validation.

## Failure Output

When the guard is run against the source repo and finds persistent specs, it
emits a G085 violation with the source no-specs requirement and points back to
this recipe. A downstream or fixture repository with no current done spec is
classified by the decision order above; zero current done is not sufficient by
itself for either success or failure.

Source failures remain unchanged. Downstream failures distinguish proven policy
violations from indeterminate evidence: no current numbered state and removed
established done evidence exit `1`; malformed current or historical state and
unavailable, shallow, partial, or failed history exit `2`. Unknown history is
never converted into `historicalDone=0`.

## Cross-References

- `docs/Framework_Convergence_Health.md` - G082-G093 overview and source no-specs rule.
- `docs/Spec_Implementation_Alignment.md` - planning linkage, post-certification edit, dependency, terminal-status, and delivery-delta contracts.
- `bubbles/scripts/framework-dogfood-guard.sh` - source-aware G085 implementation.
- `bubbles/scripts/framework-dogfood-guard-selftest.sh` - hermetic source/downstream fixture matrix.
- `bubbles/scripts/framework-validate.sh` - framework validation surface.
- `bubbles/scripts/generate-release-manifest.sh` - release manifest freshness surface.

## Evidence Provenance

- **Claim Source:** interpreted
- **Interpretation:** This recipe describes the production guard contract. Test
	and release claims require current execution evidence from the focused G085
	checks, framework validation, and release check.
