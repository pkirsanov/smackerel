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
| Downstream or hermetic fixture repository | At least one `specs/[0-9]*-*/state.json` has top-level `status: done` | Zero numbered specs at `status: done`, malformed state JSON, or missing repo root |

The source-repo pass condition intentionally differs from downstream. Source dogfooding is proved by the framework validating itself and by fixture/downstream specs exercising installed behavior, not by storing Bubbles' own work packet forever.

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

When the guard is run against the source repo and finds persistent specs, it emits a G085 violation with the source no-specs requirement and points back to this recipe. When it is run against a downstream or fixture repo with no done spec, it emits the traditional done-spec evidence violation.

Both failures are real: source repos must remove persistent specs; downstream/fixture repos must provide done-spec evidence.

## Cross-References

- `docs/Framework_Convergence_Health.md` - G082-G093 overview and source no-specs rule.
- `docs/Spec_Implementation_Alignment.md` - planning linkage, post-certification edit, dependency, terminal-status, and delivery-delta contracts.
- `bubbles/scripts/framework-dogfood-guard.sh` - source-aware G085 implementation.
- `bubbles/scripts/framework-dogfood-guard-selftest.sh` - hermetic source/downstream fixture matrix.
- `bubbles/scripts/framework-validate.sh` - framework validation surface.
- `bubbles/scripts/generate-release-manifest.sh` - release manifest freshness surface.

## Evidence Provenance

- **Claim Source:** interpreted
- **Interpretation:** This recipe reflects the implemented source-aware G085 guard and the Bubbles source-repo no-specs rule. Validation is recorded in the migration result envelope.
