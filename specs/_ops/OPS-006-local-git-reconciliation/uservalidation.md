# OPS-006 User Validation

## Evidence Semantics

- Checked baseline items below reflect the operator-supplied inventory recorded in `objective.md`.
- That inventory is marked **interpreted** and was not re-executed or independently certified during planning.
- A checked baseline item is not proof that the condition still holds now.
- Every execution and cleanup outcome remains unchecked until a later bound session performs the work and records current-session evidence.
- Human validation may uncheck any baseline item when newer evidence contradicts it.

## Checklist

### Already-Observed Baseline

- [x] The supplied inventory reported that local `main` and remote-tracking `main` matched at inventory time and that no tracked file was dirty.
- [x] The supplied inventory reported one local branch, one worktree, and no stash.
- [x] The supplied inventory identified eleven local-only archive or backup tags and nine non-equivalent dangling commits requiring semantic review.
- [x] The supplied inventory identified uniquely tagged coordinator handoff content requiring an explicit preserve-or-supersede decision.
- [x] The supplied inventory identified seven ignored temporary Python files, ignored distribution artifacts, stale session files, and stale runtime files.
- [x] The supplied inventory reported tracked nonterminal specs and bugs already present on `main`; these are actionable work, not cleanup residue.

### Final Acceptance Outcomes

#### Preservation And Classification

- [ ] A fresh actionable binding and fresh unfiltered inventory were obtained before reconciliation began.
- [ ] Every supplied and newly discovered local-only item was protected by verified recovery material before mutation.
- [ ] Every tag, tagged-only artifact, dangling commit, temporary Python file, distribution artifact, session file, and runtime file received exactly one class: `INTEGRATED`, `REPRESENTED`, `SUPERSEDED`, `ARCHIVED`, `DISCARDED`, or `ROUTE_REQUIRED`.
- [ ] Every disposition records semantic evidence and the class-required integration, supersession, recovery, or routing details.
- [ ] No secret value, PII, concrete environment detail, or foreign-owned state was exposed or published.

#### Durable Integration And Handoff

- [ ] Every retained change was genericized, tested, committed with provenance, and integrated into the correct ownership boundary.
- [ ] Generated output remained generated and ignored; reproducibility was proven through the repository CLI.
- [ ] Every non-integrated item has a durable pushed disposition and verified recovery metadata.
- [ ] Every tracked nonterminal spec and bug has a pushed status, workflow mode, next owner, and exact next action without owner-controlled status or certification mutation.
- [ ] Consumer and shared-infrastructure impact sweeps found no stale first-party reference or unplanned contract break.

#### Validation, Push, And Cleanup

- [ ] Config generation, compile checks, lint, format, Go unit, Python unit, integration, E2E, stress, build, framework validation, and diff checks all passed in the reconciliation session with zero warnings and no skipped checks.
- [ ] Remote movement was reconciled onto the newer base when necessary, followed by complete revalidation.
- [ ] `main` advanced by fast-forward only and was pushed without force and without publishing local tags.
- [ ] Local `main`, remote-tracking `main`, and the remote `main` head were proven identical after a fresh fetch.
- [ ] Cleanup began only after every disposition was present on the verified remote and both protected recovery artifacts re-verified successfully.
- [ ] Only exact reviewed local tags, ignored items, temporary recovery refs, and the temporary branch were removed; no broad clean or object-pruning command was used.

#### Final Invariants

- [ ] `main` is the only local branch.
- [ ] Exactly one worktree exists.
- [ ] No stash exists.
- [ ] No local tag exists.
- [ ] `git status --porcelain=v1` is empty.
- [ ] `main` and `origin/main` resolve to the same full commit identity.
- [ ] The remote `main` head matches local and remote-tracking `main`.
- [ ] Every supplied dangling commit has a pushed disposition record.
- [ ] Every inventoried ignored work item has a pushed disposition record.
- [ ] Every retained change is committed and present on `origin/main`.
- [ ] Every actionable nonterminal spec and bug has a pushed next-session handoff with a known owner and exact action.
- [ ] Both protected recovery artifacts still verify after cleanup.
- [ ] No object was pruned to make the final inventory appear clean.