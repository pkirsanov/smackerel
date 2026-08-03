# Ops Packet: [OPS-006] Local Git Reconciliation

## Repository Context

This packet applies to the Smackerel repository at `<smackerel-repo>`.
Establish a fresh actionable repository binding outside committed artifacts
before execution.

## Objective

Reconcile every Smackerel-local Git object and ignored work item into current
`main` or a durable, reviewed disposition.

Retain valuable work with its provenance. Record why superseded or disposable
work does not belong on `main`.

Commit and push every retained change and disposition record. Finish on clean
`main`, synchronized with `origin/main`.

Preserve nonterminal specifications and bugs as an actionable next-session
handoff. Do not change their owner-controlled status or certification fields.

## Authorization Boundary

This packet defines a later reconciliation operation. Authoring this packet
creates only `objective.md` and `runbook.md`.

Packet authoring does not commit, push, change refs, or alter ignored files.
Runbook execution requires a fresh repository-binding decision.

The execution owner may change operational Git state under this runbook. The
execution owner must route foreign artifact changes to their named owners.

## Supplied Inventory Baseline

**Claim Source:** interpreted

The operator supplied these facts from an executed inventory. Packet authoring
did not re-execute or independently certify them.

| Surface | Supplied fact |
|---|---|
| Main parity | `main` and `origin/main` both resolve to `29e46260` |
| Tracked worktree state | No dirty tracked files |
| Local branches | No branch beyond `main` |
| Worktrees | One worktree |
| Stashes | No stashes |
| Local-only tags | 11 archive or backup tags |
| Dangling commits | Nine non-equivalent commits require semantic review |
| Tagged unique content | `HANDOFF-second-brain-coordinator.md` exists uniquely through tagged history |
| Ignored Python work | Seven `.copilot-temp` Python files |
| Ignored build output | Ignored `dist` artifacts exist |
| Ignored operational residue | Stale session and runtime files exist |
| Tracked open work | Nonterminal specifications and bugs already exist on `main` |

The nine supplied dangling commit IDs are:

1. `a1ed9f91`
2. `15ee91c2`
3. `afeedf7b`
4. `e2f0b997`
5. `1ef1cd24`
6. `58f3655e`
7. `73f3434d`
8. `e4fb11ae`
9. `34fd9bae`

## Required Outcomes

1. Review every local tag before deleting any tag.
2. Review every supplied dangling commit before deleting its recovery ref.
3. Preserve or supersede `HANDOFF-second-brain-coordinator.md` explicitly.
4. Review every ignored work item before removing it.
5. Integrate valuable unique work through traceable commits on current `main`.
6. Record every non-integrated item's reason and recovery location in `runbook.md`.
7. Validate all integrated work through the Smackerel repository CLI.
8. Push without force and verify the remote commit identity.
9. Record nonterminal specs and bugs with a next owner and next action.
10. Satisfy every final invariant in this packet.

## Durable Disposition Standard

A disposition is durable only when the pushed `runbook.md` records:

- the exact item or object ID
- the semantic classification
- the evidence used for that classification
- the retained path or integrating commit, when applicable
- the encrypted or access-controlled recovery location, when applicable
- the next owner and action, when work remains open

An external backup alone is not a disposition. An unpushed local note is not a
disposition.

## Safety Constraints

- Create and verify recovery material before changing any local ref.
- Review content semantically before deleting its final convenient reference.
- Never use `git clean -fdx`, `git gc`, or an object-pruning command.
- Never force-push or rewrite published `main` history.
- Never print secret values, credentials, or private data during review.
- Keep recovery artifacts outside the repository with restrictive permissions.
- Do not publish local archive tags unless an explicit owner decision requires it.
- Use explicit paths and values. Do not add configuration defaults.
- Use `./smackerel.sh` for runtime build, lint, format, and test operations.
- Preserve framework-managed files and owner-controlled certification fields.

## Open Work Preservation

Tracked nonterminal specs and bugs are not reconciliation residue. Keep them on
`main` and include them in the next-session handoff table in `runbook.md`.

Classify terminal state with the committed mode-aware helper. Do not infer
terminal state from the literal status value alone.

Each open item needs its path, status, workflow mode, next owner, and next
action. Unknown ownership is a routing finding, not permission to edit.

## Final Invariants

The operation is complete only when all conditions hold:

- `main` is the only local branch.
- Exactly one worktree exists.
- No stash exists.
- No local tag exists.
- `git status --porcelain=v1` is empty.
- `main` and `origin/main` resolve to the same commit.
- Every supplied dangling commit has a pushed disposition record.
- Every inventoried ignored work item has a pushed disposition record.
- Every retained change is committed and present on `origin/main`.
- Actionable nonterminal specs and bugs have a pushed next-session handoff.

Unreachable objects may remain until Git expires them normally. Do not prune
objects to make an inventory command appear clean.

## Out Of Scope

- Product behavior changes unrelated to recovered local work
- Certification or status transitions for existing specs and bugs
- Remote tag deletion
- Deployment or production runtime changes
- Force-pushes, remote history rewrites, and object pruning