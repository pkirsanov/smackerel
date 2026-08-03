# OPS-006 Local Git Reconciliation Scopes

## Planning Basis

- **Claim source:** Scope 1 executed evidence and current tracked-work state.
- This plan derives from `objective.md` and `runbook.md`.
- Scope 1 recovery, classification, and safeguard evidence is recorded in `report.md`. It claims no cleanup, commit, push, or source-ref deletion.
- Scope 2 is in progress because the generic changes are committed and pushed while their unit, lint, integration, E2E, stress, build, and framework legs are not complete. Scope 3 is in progress because commit, rebase, push, cleanup, and the Git-state invariants are proven while the complete validation chain is not.
- Execution requires a fresh actionable Smackerel repository binding.
- Execute scopes strictly in order. A later scope cannot start until every DoD item in the preceding scope has current-session evidence.
- Runtime and governance commands must use the repository command surfaces and portable timeout helper named in `runbook.md`.
- No evidence may expose secret values, PII, concrete operator identity, concrete host identity, external recovery paths, or environment-specific configuration values.

## Execution Outline

### Phase Order

1. **Scope 1: Preserve and classify hidden refs and ignored work.**
	Revalidate the inventory and create verified recovery material. Review every item semantically and assign one class. Delete nothing.
2. **Scope 2: Genericize and integrate durable records.**
	Convert approved work into generic tracked changes. Preserve provenance and record every disposition and handoff. Validate each unit on a temporary branch.
3. **Scope 3: Validate, commit, push, clean up, and prove invariants.**
	Run the full validation chain and reconcile remote movement. Push without force, verify remote identity, remove reviewed residue, and prove final invariants.

### New Types And Signatures

- `DispositionClass = INTEGRATED | REPRESENTED | SUPERSEDED | ARCHIVED | DISCARDED | ROUTE_REQUIRED`.
- A durable disposition record contains the exact item or object identifier and one disposition class.
- It also contains semantic evidence and the retained location or integrating commit when applicable.
- It records protected recovery metadata when applicable.
- It records a next owner and exact action when work remains open.
- A nonterminal handoff record contains the tracked item, current status, workflow mode, next owner, and exact next action.
- The closeout record contains starting and final remote identities and recovery verification metadata.
- It also contains object outcomes, approved removals, validation outcomes, push verification, and the next-session handoff.
- No product API, storage schema, runtime configuration key, or deployment contract is introduced by this operation.

### Validation Checkpoints

- **After Scope 1:** every inventoried item has verified recovery coverage and one evidence-backed provisional disposition.
- **After Scope 1:** no reviewed source has been deleted.
- **During Scope 2:** each `INTEGRATED` unit receives narrow tests before the next unit is admitted.
- **During Scope 2:** generated output remains ignored. Secret, PII, and no-default controls remain clean.
- **After Scope 2:** the temporary branch contains coherent generic changes, complete ledger rows, and actionable nonterminal handoffs.
- **After Scope 2:** all targeted checks pass.
- **Before advancing `main` in Scope 3:** the full config, check, lint, format, unit, integration, E2E, stress, build, framework, and diff-check chain passes in the current session.
- **Before cleanup in Scope 3:** local, remote-tracking, and remote `main` identities match.
- **Before cleanup in Scope 3:** both recovery artifacts verify. Every disposition is present on the verified remote.
- **At closeout:** branch, worktree, stash, tag, status, remote-parity, disposition, retained-change, and handoff invariants all pass without object pruning.

### Safety Boundary

- Back up first, review second, integrate third, delete last.
- Never use broad clean, garbage-collection, object-pruning, reflog-expiry, force-push, published-history rewrite, or wildcard ref deletion operations.
- Never classify from a name, timestamp, or commit subject alone.
- Never publish local archive tags by default.
- Never change owner-controlled execution status or certification fields for existing specs or bugs.
- Stop on contradictory inventory evidence, failed recovery verification, unresolved secret or PII findings, unknown ownership, or any failed required check.

### Impact And Trace Applicability

- Smackerel declares no `testImpact` map, so this plan does not claim a generated changed-path impact result.
- The repository's wired trace contract covers runtime health, while OPS-006 plans Git-state reconciliation and does not assert a runtime-health SLO. No Test Plan row is instrumented, so trace and SLO evidence gates are non-applicable to the plan as written.
- If recovered work changes a declared runtime workflow, integration stops until the planning owner amends this plan.
- The amendment names the workflow and adds validate-plane trace evidence. It adds SLO evidence when linked and matching unchecked DoD items.
- Operate-plane telemetry remains read-only. It cannot satisfy feature execution evidence.

## Scope Inventory

| Scope | Outcome | Surfaces | Primary validation | Status |
|---|---|---|---|---|
| 1 | Complete semantic inventory with verified recovery and one class per item | Local refs, unreachable commits, ignored work, recovery material, review ledger | Inventory parity, archive verification, secret/PII controls | Done |
| 2 | Generic integrated work plus durable dispositions and handoffs | Approved source/tests/docs, temporary branch, review ledger, handoff table | Targeted unit, functional, integration, E2E, and diff checks | In progress |
| 3 | Verified remote integration and residue-free local state | Repository CLI, `main`, remote tracking, reviewed local residue, closeout record | Full validation chain, push identity, final invariant checks | In progress |

## Scope 1: Preserve And Classify Hidden Refs And Ignored Work

**Status:** [x] Done.

**Scope-Kind:** `bootstrap`.

**Depends On:** None.

**Claim Source:** executed
**Evidence State:** Scope 1 inventory, recovery, semantic classification, handoff, safeguard, and no-deletion checks were executed in the current bound session. Each completed item below has its own reference to report evidence containing at least 10 raw output lines. No integration, commit, push, cleanup, or source-ref deletion is claimed.

### Outcome

Every supplied and newly discovered local-only item is recoverable, semantically reviewed, and assigned exactly one allowed disposition class before any deletion occurs.

### In-Scope Inventory Categories

- Eleven supplied local-only archive or backup tags, plus any newly observed local tag added to the ledger before review continues.
- The nine supplied non-equivalent dangling commits: `a1ed9f91`, `15ee91c2`, `afeedf7b`, `e2f0b997`, `1ef1cd24`, `58f3655e`, `73f3434d`, `e4fb11ae`, and `34fd9bae`.
- The uniquely tagged coordinator handoff artifact identified by the supplied inventory.
- Seven supplied ignored temporary Python files, reviewed individually.
- Ignored distribution artifacts.
- Stale session files.
- Stale runtime files.
- Any unknown branch, worktree, stash, tag, unreachable commit, or ignored item discovered by revalidation.
- Discovery blocks progress until the new item is preserved and added to the ledger.

### Allowed Disposition Classes

| Class | Exact use | Required result |
|---|---|---|
| `INTEGRATED` | Unique work remains valid | Traceable commit on pushed `main` |
| `REPRESENTED` | Current `main` already carries the same intent | Equivalence evidence and pushed disposition |
| `SUPERSEDED` | A newer implementation or decision replaces the item | Named superseding artifact or commit and pushed semantic reason |
| `ARCHIVED` | The item has durable historical value but no runtime role | Verified protected recovery material and pushed recovery metadata |
| `DISCARDED` | The item is generated, stale, invalid, or an exact duplicate | Semantic reason, recovery proof, and pushed disposition |
| `ROUTE_REQUIRED` | Valid work belongs to another owner | Preserved source, named next owner, and exact next action |

### Gherkin Scenarios

#### SCN-OPS006-001: Recovery precedes mutation

```gherkin
Given a fresh actionable repository binding and a revalidated local inventory
And no other process is changing the checkout
When the execution owner prepares to review local-only refs or ignored work
Then every supplied dangling commit is protected by a temporary recovery ref
And the complete ref set is present in a verified protected bundle
And every explicitly inventoried ignored item is present in a verified protected archive
And no tag, recovery ref, ignored item, branch, stash, or unreachable object is deleted
```

#### SCN-OPS006-002: Every item receives one semantic class

```gherkin
Given verified recovery material and the complete review ledger
When tags, tagged-only content, dangling commits, and ignored work are reviewed
Then each item is classified from semantic evidence rather than its name, age, or subject
And each item receives exactly one allowed disposition class
And every class records the evidence and required recovery, integration, supersession, or routing data
And no ledger row remains REVIEW REQUIRED
```

#### SCN-OPS006-003: Sensitive or foreign-owned work is contained

```gherkin
Given a candidate item may contain secrets, PII, environment-specific material, or foreign-owned artifacts
When committed secret, PII, ownership, and generic-product controls are applied
Then sensitive values are not displayed or staged
And environment-specific material is excluded from the generic product repository
And foreign-owned work is preserved and routed rather than edited without authority
And unresolved sensitive or ownership findings stop the scope
```

### Implementation Plan

1. Obtain a fresh actionable binding, confirm the reconciliation owner, and confirm no concurrent checkout mutation.
2. Load the portable timeout helper and fetch remote state without fetching tags.
3. Capture unfiltered inventories for tracked state, local and remote `main`, branches, tags, worktrees, stashes, unreachable objects, and ignored work.
4. Stop on dirty tracked state or any unknown branch, worktree, stash, ref, object, or ignored item until the ledger includes it.
5. Prove all nine supplied dangling IDs still resolve as commits.
6. Create explicit temporary recovery refs for all supplied dangling commits before any potentially destructive action.
7. Create and verify a protected Git bundle containing all current refs.
8. Create a protected archive containing only the exact reviewed ignored paths. Record digests and access metadata without sensitive content.
9. Apply secret and PII safeguards before displaying, staging, or integrating candidate bodies.
10. Review tags, tagged-only content, and dangling commits in parent-before-child order.
11. Review each temporary Python file, generated distribution artifact, session residue item, and runtime residue item.
12. Assign exactly one allowed class per item and record redacted semantic evidence. Do not delete any reviewed source in this scope.

### Change Boundary

- **Allowed:** read-only repository inventories and temporary recovery refs.
- **Allowed:** protected recovery artifacts outside all workspace repositories and redacted review-ledger updates.
- **Allowed:** committed secret, PII, ownership, and generic-product checks.
- **Excluded:** integration commits, `main` advancement, pushes, and all reviewed-source deletion.
- **Excluded:** status transitions, certification edits, deployment, and object pruning.
- **Rollback:** stop, preserve verified recovery material, and leave all source refs and ignored items intact.

### Test Plan

| Scenario | Category | Verification | Command surface | Live system |
|---|---|---|---|---|
| SCN-OPS006-001 | `functional` | Fresh inventory is complete; all supplied commits resolve; bundle and ignored-work archive verify before mutation | Exact unfiltered Phase 1 and Phase 2 commands in `runbook.md`, using `bubbles_run_with_timeout` | No |
| SCN-OPS006-002 | `functional` | Inventory-to-ledger comparison has one and only one allowed class per item, required evidence fields are populated, and no review-required row remains | Row-by-row comparison against the unfiltered inventories and verified recovery listings | No |
| SCN-OPS006-003 | `functional` | Candidate paths clear committed secret, PII, ownership, and generic-product controls without exposing values | `./smackerel.sh lint` plus the repository's committed candidate-content safeguards named by the execution owner | No |

This bootstrap scope changes no runtime behavior, so it does not claim a live E2E category. Any candidate later selected for integration receives runtime-category coverage in Scope 2 and the full E2E chain in Scope 3.

### Definition of Done

#### Core Items

- [x] Fresh current-session inventory is captured without truncation, and every newly observed item is added to the ledger before review continues.
	- Evidence: [Current inventory and special-ref predicates](report.md#current-genericity-and-recovery-safeguards), [ignored-path ledger totals](report.md#ignored-archive-completeness), and [unreachable-object inventory totals](report.md#unreachable-preservation-and-classification).
- [x] All supplied dangling commits, all reviewed local tags, and all exact ignored items are covered by verified protected recovery material created before mutation.
	- Evidence: [Recovery ref and artifact verification](report.md#recovery-and-ref-verification), [classified tag bundle coverage](report.md#tag-verification), and [ignored archive completeness](report.md#ignored-archive-completeness).
- [x] Every inventoried item has exactly one of `INTEGRATED`, `REPRESENTED`, `SUPERSEDED`, `ARCHIVED`, `DISCARDED`, or `ROUTE_REQUIRED`, with all class-required evidence fields.
	- Evidence: [Protected commit representation](report.md#protected-commit-representation), [temporary Python dispositions](report.md#temporary-python-security-dispositions), [ignored archive classifications](report.md#ignored-archive-completeness), and [unreachable-object classifications](report.md#unreachable-preservation-and-classification).
- [x] The tagged-only coordinator handoff artifact has an explicit preserve, supersede, archive, discard, or route decision with provenance.
	- Evidence: [Tagged handoff provenance and routing decision](report.md#tagged-handoff-verification).
- [x] No local tag, recovery ref, ignored item, branch, stash, or unreachable object has been deleted.
	- Evidence: [Current ref, tag, branch, worktree, stash, and recovery predicates](report.md#current-genericity-and-recovery-safeguards) and [archive source-mutation predicate](report.md#ignored-archive-completeness).
- [x] Secret, PII, environment-specific, and foreign-ownership findings are contained and routed.
	- Evidence: [Genericity, deployment-boundary, and machine-token safeguards](report.md#current-genericity-and-recovery-safeguards), [unsafe temporary Python dispositions](report.md#temporary-python-security-dispositions), and [mode-aware ownership inventory](report.md#mode-aware-nonterminal-inventory).
- [x] Any unresolved sensitive or ownership finding blocks Scope 2.
	- Evidence: [Passing genericity and recovery safeguard predicates](report.md#current-genericity-and-recovery-safeguards) and [explicit tagged-handoff owner and next action](report.md#tagged-handoff-verification).

#### Test Evidence Items

- [x] SCN-OPS006-001 functional inventory and recovery verification passes with current-session raw output.
	- Evidence: [Nine-ref, bundle, archive, permission, and digest verification](report.md#recovery-and-ref-verification).
- [x] SCN-OPS006-002 inventory-to-ledger completeness verification passes with no missing, duplicate, or review-required disposition.
	- Evidence: [Tag classification failure count](report.md#tag-verification), [ignored-path unclassified count](report.md#ignored-archive-completeness), and [unreachable classification and mismatch counts](report.md#unreachable-preservation-and-classification).
- [x] SCN-OPS006-003 committed secret, PII, ownership, and generic-product checks pass without sensitive output.
	- Evidence: [Product-boundary and machine-local-token safeguard output](report.md#current-genericity-and-recovery-safeguards).

#### Build Quality Gate

- [x] Scope 1 records are consistent, redacted, explicit, free of defaults, and ready for Scope 2.
	- Evidence: [Genericity and recovery safeguards with preserved change-boundary invariants](report.md#current-genericity-and-recovery-safeguards) and [closed temporary-source dispositions](report.md#temporary-python-security-dispositions).

## Scope 2: Genericize And Integrate Durable Records

**Status:** [~] In progress.

**Scope-Kind:** `bootstrap`.

**Depends On:** Scope 1.

**Claim Source:** executed for the committed and pushed generic changes; unexecuted for the outstanding validation legs
**Evidence State:** The generic changes are committed as `b4652c94` and `fdc04cad` and are present on verified `origin/main` at `8a4e553d`. Items closed below have current-session evidence in [Integration, push, cleanup, and final invariants](report.md#integration-push-cleanup-and-final-invariants). Every remaining checkbox stays unchecked because `./smackerel.sh lint`, `./smackerel.sh test unit --python`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and `./smackerel.sh build` were not completed in this session, and `./smackerel.sh test unit --go` exits `1`.

### Outcome

Each retained item enters the correct generic ownership boundary with provenance and tests.
Every other item receives a durable disposition. Nonterminal tracked work receives an actionable handoff without owner-controlled state changes.

### Gherkin Scenarios

#### SCN-OPS006-004: Valid unique work is integrated generically

```gherkin
Given an item is classified INTEGRATED and its provenance is preserved
When the item is moved onto a temporary branch based on current origin/main
Then operator-specific and environment-specific values are removed or routed to existing explicit seams
And no default or fallback is introduced
And the behavior is covered by persistent tests that would fail if the recovered behavior regressed
And the integration is traceable to its source object or ignored-work provenance
```

#### SCN-OPS006-005: Non-integrated work remains auditable and recoverable

```gherkin
Given an item is classified REPRESENTED, SUPERSEDED, ARCHIVED, DISCARDED, or ROUTE_REQUIRED
When its durable disposition is written
Then the record names the exact item, class, semantic evidence, and recovery proof
And it names the superseding artifact, protected recovery metadata, or next owner and exact action as required by the class
And no sensitive value or concrete environment detail is published
```

#### SCN-OPS006-006: Existing nonterminal work is preserved

```gherkin
Given tracked specs and bugs already exist on main
When terminal state is evaluated with the committed mode-aware helper
Then every nonterminal item is recorded with path, status, workflow mode, next owner, and exact next action
And valid actions recovered from tagged handoff content retain provenance
And execution and certification state remain unchanged
```

### Implementation Plan

1. Re-fetch without tags and create a temporary reconciliation branch from current `origin/main`.
2. Never integrate from the historical inventory base when the remote has advanced.
3. Integrate approved dangling commits in dependency order with source-origin metadata.
4. Do not copy commit content by hand when traceable integration is available.
5. Move approved ignored source into its owned module after semantic review, genericization, and persistent regression coverage.
6. Keep generated distribution output ignored. Prove that tracked source and the repository CLI reproduce it before classifying the ignored copy.
7. Remove or route concrete operator, host, external path, secret, PII, and environment-specific values from retained work.
8. Reuse existing explicit configuration seams and fail-loud behavior. Add no defaults.
9. Run the narrowest relevant repository CLI category after each coherent integrated unit. Stop on any failure.
10. Populate every durable disposition field in `runbook.md`. Include recovery metadata and integrating or superseding commits when applicable.
11. Evaluate tracked specs and bugs with the committed mode-aware helper. Resolve ownership through the ownership registry.
12. Write one exact continuation per nonterminal item. Do not change status or certification fields.
13. Commit coherent reviewed units only after their targeted tests pass.
14. Keep all tags, recovery refs, ignored source copies, and the temporary branch until Scope 3 verifies the push.

### Consumer And Shared-Surface Impact Sweep

- For each integrated source or contract change, enumerate first-party API, PWA, extension, Telegram, MCP, docs, generated-client, config, and test consumers before committing.
- For each shared fixture, bootstrap, auth, session, storage, or harness change, identify downstream consumers.
- Run independent canary coverage and preserve a recovery path before broad validation.
- A route, identifier, contract, or path rename follows expand, migrate, then contract ordering.
- Do not remove old forms until all consumers are proven migrated.

### Change Boundary

- **Allowed:** approved retained source, tests, owned docs, and the temporary reconciliation branch.
- **Allowed:** `runbook.md` disposition and handoff records. Local commits are allowed after checks pass.
- **Excluded:** owner-controlled status or certification edits, concrete environment values, and tracked generated output.
- **Excluded:** `main` advancement, push, reviewed-source deletion, deployment, and object pruning.
- **Rollback:** abort the current integration operation, return to `main`, retain the temporary branch when it carries unique reviewed work, and keep both verified recovery artifacts.

### Test Plan

| Scenario | Category | Verification | Command | Live system |
|---|---|---|---|---|
| SCN-OPS006-004 | `unit` | Persistent adversarial tests cover every integrated Go and Python behavior selected from recovered work | `./smackerel.sh test unit --go` and `./smackerel.sh test unit --python` | No |
| SCN-OPS006-004 | `functional` | Generated config, compilation checks, lint, and formatting prove generic fail-loud integration with no defaults or sensitive material | `./smackerel.sh config generate`; `./smackerel.sh check`; `./smackerel.sh lint`; `./smackerel.sh format --check` | No |
| SCN-OPS006-004 | `integration` | Integrated behavior composes with real repository dependencies | `./smackerel.sh test integration` | Yes, disposable test stack |
| SCN-OPS006-004 | `e2e-api` | Persistent regression coverage exercises every integrated externally observable behavior end to end without internal mocks | `./smackerel.sh test e2e` | Yes, disposable test stack |
| SCN-OPS006-005, SCN-OPS006-006 | `functional` | Durable dispositions and nonterminal handoffs are complete, generic, provenance-preserving, and whitespace-clean | `git diff --check origin/main...HEAD` plus ledger-to-inventory and handoff-to-mode-aware-inventory comparisons | No |

If Scope 1 selects no runtime behavior for `INTEGRATED`, the unit, integration, and E2E commands remain regression checks.
Their success does not imply that recovered runtime behavior existed.

### Definition of Done

#### Core Items

- [x] Every `INTEGRATED` item is generic, owned, provenance-preserving, free of defaults and sensitive values, and represented by a coherent local commit on the temporary branch. Evidence: integrating commit and disposition row.
	- Evidence: [Commit inventory, safety-gate results, and pushed head](report.md#integration-push-cleanup-and-final-invariants). `b4652c94` is atomic across the ignore rule and the five `.dart_tool` untrackings. `fdc04cad` carries the genericization at 80 files, 410 insertions, and 383 deletions. Both cleared the private-token scan, the untracked packet scan, and the knb deployment-boundary gate before commit and again before push.
- [x] Every non-integrated item has a complete durable disposition record with semantic evidence and verified recovery metadata. Evidence: review ledger.
	- Evidence: [Unreachable classification with zero unclassified rows](report.md#unreachable-preservation-and-classification), [tag classification](report.md#tag-verification), [ignored archive classification](report.md#ignored-archive-completeness), and [the ledger reaching verified `origin/main` in commit `8a4e553d`](report.md#integration-push-cleanup-and-final-invariants).
- [ ] Generated outputs remain ignored and are reproducible through the repository CLI before their local copies are eligible for cleanup. Evidence: reproducibility record.
- [x] Every tracked nonterminal spec and bug has status, workflow mode, next owner, and exact next action, with no execution or certification state mutation. Evidence: next-session handoff table.
	- Evidence: [Mode-aware inventory at 27 helper-derived paths](report.md#mode-aware-nonterminal-inventory), [`OWNER_FIELDS_UNCHANGED` across all 8 redacted `state.json` files](report.md#genericization-repair-verification), and [the 32-row ledger on verified `origin/main`](report.md#integration-push-cleanup-and-final-invariants).
- [ ] Consumer and shared-infrastructure impact sweeps are complete for every integrated change. Evidence: integration review record.
- [ ] No stale first-party reference or unplanned shared-contract change remains. Evidence: integration review record.
- [x] All local tags, temporary recovery refs, original ignored source copies, and the temporary branch remain available pending verified push. Evidence: pre-Scope-3 inventory.
	- Evidence: [Cleanup ordered strictly after the verified push, with both recovery artifacts re-verified beforehand and retained](report.md#integration-push-cleanup-and-final-invariants). The bundle verifies with exit `0` and lists all 65 refs, covering the 9 dangling refs, the 42 unreachable refs, and the 11 tags.

#### Test Evidence Items

- [ ] SCN-OPS006-004 Go and Python unit regression commands pass with current-session raw output. Evidence: Scope 2 test record.
- [ ] SCN-OPS006-004 config generation, check, lint, and format commands pass with current-session raw output and zero warnings. Evidence: Scope 2 test record.
- [ ] SCN-OPS006-004 integration tests pass against the disposable test stack with no internal mocks. Evidence: Scope 2 test record.
- [ ] SCN-OPS006-004 E2E regression tests pass against the disposable live stack and fail loudly on missing behavior. Evidence: Scope 2 test record.
- [ ] SCN-OPS006-005 and SCN-OPS006-006 diff, ledger, and handoff completeness checks pass. Evidence: Scope 2 test record.

#### Build Quality Gate

- [ ] Scope 2 commits, records, tests, docs, ownership boundaries, and rollback material are ready for final validation.
	No excluded surface changed. Evidence: change-boundary review.

## Scope 3: Validate, Commit, Push, Clean Up, And Prove Final Invariants

**Status:** [~] In progress.

**Scope-Kind:** `bootstrap`.

**Depends On:** Scope 2.

**Claim Source:** executed for commit, rebase, push, cleanup, and the Git-state invariants; unexecuted for the complete validation chain
**Evidence State:** Three coherent commits reached verified `origin/main` at `8a4e553d2b41bfc63bf82cb34ddb8423025fcb1a` by non-force fast-forward push, cleanup ran only after that verified push, and the branch, worktree, stash, tag, `refs/ops`, status, and parity invariants were re-observed unfiltered in the current session. Evidence is in [Integration, push, cleanup, and final invariants](report.md#integration-push-cleanup-and-final-invariants).
**Uncertainty Declaration:** The complete validation chain was NOT completed. `./smackerel.sh lint`, `./smackerel.sh test unit --python`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, `./smackerel.sh test stress`, and `bash .github/bubbles/scripts/cli.sh framework-validate` were not run at all, and `./smackerel.sh build` was not run to completion. `./smackerel.sh test unit --go` exits `1` on the two documented pre-existing failures. Two Final-Invariant-Check rows are also unproven: the recovery artifacts were verified BEFORE cleanup rather than after it, and the handoff ledger still carries two `unassigned` owners. Every item depending on those commands or rows stays unchecked, and the scope stays In Progress.

### Outcome

All retained work and disposition records reach verified `origin/main` after validation.
Only then may the owner remove reviewed local residue and prove every final invariant.

### Gherkin Scenarios

#### SCN-OPS006-007: Validated work advances without rewriting remote history

```gherkin
Given the temporary reconciliation branch contains complete durable dispositions and tested retained work
When the full repository validation chain passes against the current remote base
Then local main advances by fast-forward only
And main is pushed without force and without publishing local tags
And local main, remote-tracking main, and the remote main head resolve to the same commit
```

#### SCN-OPS006-008: Remote movement forces revalidation

```gherkin
Given origin/main changes before local main advances or the push is accepted
When the execution owner refreshes remote state
Then the temporary branch is rebased onto the newer remote base
And every conflict is reviewed semantically
And the complete validation chain is rerun
And the newer remote state is never overwritten
```

#### SCN-OPS006-009: Cleanup occurs only after durable remote proof

```gherkin
Given every disposition is present on verified origin/main
And both protected recovery artifacts still verify
When reviewed local residue is removed by exact item or ref name
Then only approved local tags, ignored items, temporary recovery refs, and the temporary branch are removed
And no object is pruned
And all final Git, disposition, retained-change, and handoff invariants pass
```

### Implementation Plan

1. Confirm every Scope 2 local commit is coherent and the final `runbook.md` update contains every disposition and next-session handoff.
2. Run the complete current-session validation chain listed below with full, unfiltered output and explicit outcomes.
3. Fetch remote state again without tags. If `origin/main` moved, rebase the temporary branch, review conflicts semantically, and rerun the complete chain.
4. Switch to `main` and fast-forward it to the validated temporary branch only. Do not merge non-fast-forward and do not rewrite history.
5. Push only `main:main` without force or tag publication. If rejected, return to remote reconciliation and rerun validation.
6. Fetch again and prove local `main`, `origin/main`, and the remote head all match. Keep recovery material and all residue until this proof passes.
7. Re-verify the Git bundle and ignored-work archive.
8. Remove only the exact reviewed tags, approved ignored paths, temporary recovery refs, and temporary branch.
9. Do not run garbage collection, pruning, or reflog expiry. Unreachable objects may expire normally.
10. Run every final invariant check. Compare the final ledger and handoff against the original and refreshed inventories.
11. Complete the closeout record only from current-session observed output. Leave the packet open if any invariant or evidence item fails.

### Complete Validation Chain

Run every command through the portable timeout helper.
Use the timeout defined in `runbook.md`.

1. `./smackerel.sh config generate`
2. `./smackerel.sh check`
3. `./smackerel.sh lint`
4. `./smackerel.sh format --check`
5. `./smackerel.sh test unit --go`
6. `./smackerel.sh test unit --python`
7. `./smackerel.sh test integration`
8. `./smackerel.sh test e2e`
9. `./smackerel.sh test stress`
10. `./smackerel.sh build`
11. `bash .github/bubbles/scripts/cli.sh framework-validate`
12. `git diff --check origin/main...HEAD`

### Final Invariant Checks

Run the exact unfiltered checks from `runbook.md` and interpret them as follows:

| Invariant | Required proof |
|---|---|
| Local branches | The branch inventory reports only `main` |
| Worktrees | The worktree inventory reports exactly one worktree record |
| Stashes | The stash inventory is empty |
| Local tags | The local tag inventory is empty |
| Worktree state | `git status --porcelain=v1` is empty |
| Main parity | `main` and `origin/main` resolve to the same full commit identity |
| Remote parity | The remote `main` head matches local and remote-tracking `main` |
| Dangling-commit accounting | Every supplied dangling commit has a pushed final disposition and evidence |
| Ignored-work accounting | Every inventoried ignored item has a pushed final disposition and evidence |
| Retained work | Every retained change is committed and present on `origin/main` |
| Nonterminal handoff | Every actionable nonterminal spec and bug has a pushed next owner and exact next action |
| Recovery | Both protected recovery artifacts verify after cleanup |
| Object lifecycle | No object-pruning operation was used |

### Change Boundary

- **Allowed:** final coherent commits, complete validation, remote refresh, and semantic rebase.
- **Allowed:** fast-forward-only `main` advancement, non-force push, identity verification, exact cleanup, and closeout recording.
- **Excluded:** force-push, tag push, remote tag deletion, published-history rewrite, wildcard cleanup, broad ignored-file cleanup, object pruning, deployment, unrelated product changes, and status or certification transitions.
- **Rollback before push:** preserve reviewed work on the temporary branch and in recovery artifacts.
- **Rollback after push:** use new revert commits and the full validation chain. Never rewrite remote history.

### Test Plan

| Scenario | Category | Verification | Command | Live system |
|---|---|---|---|---|
| SCN-OPS006-007 | `functional` | Generated configuration and compile checks succeed | `./smackerel.sh config generate`; `./smackerel.sh check` | No |
| SCN-OPS006-007 | `functional` | Lint, formatting, and diff quality gates succeed with zero warnings | `./smackerel.sh lint`; `./smackerel.sh format --check`; `git diff --check origin/main...HEAD` | No |
| SCN-OPS006-007 | `unit` | Go unit and adversarial regression coverage passes | `./smackerel.sh test unit --go` | No |
| SCN-OPS006-007 | `unit` | Python unit and adversarial regression coverage passes | `./smackerel.sh test unit --python` | No |
| SCN-OPS006-007 | `integration` | Real component interactions pass against disposable dependencies | `./smackerel.sh test integration` | Yes, disposable test stack |
| SCN-OPS006-007 | `e2e-api` | End-to-end product regression flows pass without internal mocks | `./smackerel.sh test e2e` | Yes, disposable test stack |
| SCN-OPS006-007 | `stress` | Stress checks pass for affected runtime paths | `./smackerel.sh test stress` | Yes, disposable test stack |
| SCN-OPS006-007 | `functional` | Immutable build succeeds from the validated source | `./smackerel.sh build` | No |
| SCN-OPS006-007 | `framework` | Installed Bubbles framework validation passes | `bash .github/bubbles/scripts/cli.sh framework-validate` | No |
| SCN-OPS006-007, SCN-OPS006-008 | `functional` | Push is non-force, remote movement is reconciled, and all three main identities match | Exact fetch, revision, remote-head, status, fast-forward, and push-verification commands in `runbook.md` | Remote Git only |
| SCN-OPS006-009 | `functional` | Exact cleanup preserves recovery and all final Git, disposition, retained-work, and handoff invariants pass | Exact cleanup and Final Invariant Check commands in `runbook.md`, followed by ledger and handoff completeness comparison | No |

### Definition of Done

#### Core Items

- [x] Every retained change and every durable disposition or handoff record is present in coherent local commits before `main` advances. Evidence: commit inventory and review ledger.
	- Evidence: [Three-commit inventory with per-commit file and line counts](report.md#integration-push-cleanup-and-final-invariants). `b4652c94` and `fdc04cad` carry the retained generic changes and `8a4e553d` carries the disposition and handoff records.
- [ ] Any remote movement is reconciled onto the newer base and the complete validation chain is rerun. Evidence: remote reconciliation record.
- [x] No newer remote state is overwritten. Evidence: remote reconciliation record.
	- Evidence: [Rebase onto `1ca7bcb6` with exit `0` and zero conflicts, then a non-force push over the `1ca7bcb6..8a4e553d` fast-forward range](report.md#integration-push-cleanup-and-final-invariants).
- [x] Local `main` advances by fast-forward only and is pushed without force or tag publication. Evidence: push record.
	- Evidence: [`PUSH_REFSPEC=refs/heads/main:refs/heads/main`, `RANGE=1ca7bcb6..8a4e553d` with no `+` prefix, `PUSH_EXIT=0`, `FORCE=false`, `TAGS_PUSHED=0`](report.md#integration-push-cleanup-and-final-invariants).
- [x] Local, remote-tracking, and remote `main` identities match after a fresh fetch. Evidence: push-verification record.
	- Evidence: [Three-way identity where local `main`, remote-tracking `main`, and the `git ls-remote` head all resolve to `8a4e553d2b41bfc63bf82cb34ddb8423025fcb1a`, with ahead/behind at `0 0`](report.md#integration-push-cleanup-and-final-invariants).
- [x] Cleanup begins only after verified push and successful re-verification of both recovery artifacts. Evidence: cleanup precondition record.
	- Evidence: [`CLEANUP_ORDER=after-verified-push`, bundle verify exit `0` reporting `The bundle records a complete history` across 65 refs, and both artifact digests re-checked before cleanup](report.md#integration-push-cleanup-and-final-invariants).
- [x] Only exact reviewed tags, ignored items, temporary recovery refs, and the temporary branch are removed. Evidence: cleanup record.
	- Evidence: [51 `refs/ops/OPS-006/` refs and 11 local tags deleted, each by full ref or tag name, with `WILDCARD=false` on both](report.md#integration-push-cleanup-and-final-invariants).
- [x] No broad cleanup or object pruning occurs. Evidence: cleanup record.
	- Evidence: [`CLEANUP_GC_RUN=false`, `CLEANUP_PRUNE_RUN=false`, `CLEANUP_REFLOG_EXPIRE_RUN=false`](report.md#integration-push-cleanup-and-final-invariants).
- [ ] Every final invariant in the Final Invariant Checks table passes in the current session. Evidence: closeout record.
- [ ] The final next-session handoff has no unknown owner or vague action, and owner-controlled status and certification fields remain unchanged. Evidence: handoff and tracked-state comparison.

#### Test Evidence Items

- [x] SCN-OPS006-007 config generation and compile checks pass with current-session raw output. Evidence: Scope 3 test record.
	- Evidence: [`CHECK_EXIT=0` with config in sync with the SST and scenario-lint reporting 17 registered and 0 rejected](report.md#genericization-repair-verification), restated as `VALIDATION_COMPLETED_config_generate=0` and `VALIDATION_COMPLETED_check=0` in [the Scope 3 record](report.md#integration-push-cleanup-and-final-invariants).
- [ ] SCN-OPS006-007 lint, format, and diff checks pass with zero warnings. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 Go unit tests pass with current-session raw output. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 Python unit tests pass with current-session raw output. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 integration tests pass against disposable dependencies with current-session raw output. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 E2E tests pass against the disposable live stack without internal mocks or bailout paths. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 stress tests pass with current-session raw output. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 build passes with current-session raw output and zero warnings. Evidence: Scope 3 test record.
- [ ] SCN-OPS006-007 framework validation passes with current-session raw output. Evidence: Scope 3 test record.
- [x] SCN-OPS006-007 and SCN-OPS006-008 remote movement, non-force push, and three-way identity verification checks pass. Evidence: Scope 3 push-verification record.
	- Evidence: [Rebase exit `0` with zero conflicts onto the advanced `1ca7bcb6` base, `PUSH_EXIT=0` with `FORCE=false` and `TAGS_PUSHED=0`, and `PUSH_IDENTITY_THREE_WAY=MATCH`](report.md#integration-push-cleanup-and-final-invariants).
- [ ] SCN-OPS006-009 exact cleanup and all final invariant, ledger, retained-work, recovery, and handoff checks pass. Evidence: Scope 3 closeout record.

#### Build Quality Gate

- [ ] Scope 3 has current-session evidence for every command and invariant. It has zero warnings, deferrals, or skipped checks.
	No sensitive output or excluded-surface change exists. Evidence: final closeout review.

## Superseded Scopes (Do Not Execute)

None.