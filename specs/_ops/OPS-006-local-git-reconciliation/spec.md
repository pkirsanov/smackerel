# Spec: OPS-006 Local Git Reconciliation

**Status:** Draft

**Claim Source:** interpreted

This specification derives its requirements from `objective.md` and
`runbook.md`. It does not claim that the reconciliation has run.

## Source Mapping

| Requirement area | Grounding source |
|---|---|
| No-loss reconciliation and durable dispositions | `objective.md` Required Outcomes, Durable Disposition Standard, and Safety Constraints |
| Review order, recovery, integration, and cleanup | `runbook.md` Non-Negotiable Safety Rules and ordered execution phases |
| Final Git-state invariants | `objective.md` Final Invariants and `runbook.md` Final Invariant Check |
| Open spec and bug preservation | `objective.md` Open Work Preservation and `runbook.md` Phase 7 |
| Smackerel and knb ownership boundary | Product deployment boundary policy and Smackerel fail-loud configuration policy |
| Added ignored-work inventory classes | Current OPS-006 authoring request |

## Outcome Contract

**Intent:** Reconcile every Smackerel-local Git object and ignored work item
without losing unique work, provenance, or actionable ownership information.

**Success Signal:** Current `main` contains every retained change and every
durable disposition record. It matches `origin/main` after a non-forced push.

**Hard Constraints:** Preserve recoverability before mutation. Review content
semantically. Keep concrete deployment configuration only in knb. Keep only
target-neutral, fail-loud deployment seams in Smackerel.

**Failure Condition:** The operation fails if any inventoried item lacks a
durable disposition, any unique work becomes unrecoverable, or concrete target
configuration enters Smackerel.

## Domain Capability Model

### Primitives

| Primitive | Meaning |
|---|---|
| Inventory snapshot | A complete view of local reconciliation surfaces at a stated point in the operation |
| Inventory item | One branch, worktree, stash, tag, commit, ignored item, runtime record, spec, or bug requiring classification |
| Recovery record | Verified recovery material and metadata created before a destructive action |
| Disposition record | The semantic decision, evidence, retained location, recovery location, owner, and action for one item |
| Integration record | The trace from recovered source material to its reviewed commit on current `main` |
| Handoff record | A preserved open item with its workflow mode, current status, next owner, and exact next action |

### Lifecycle

Each inventory item moves through these states:

1. `DISCOVERED`
2. `PRESERVED`
3. `REVIEWED`
4. `DISPOSITION_RECORDED`
5. `RESOLVED`

An item cannot skip `PRESERVED` before any destructive action. An item cannot
reach `RESOLVED` until its durable disposition is present on verified remote
`main`.

### Disposition Classes

Each item receives exactly one class.

| Class | Business meaning |
|---|---|
| `INTEGRATED` | Unique and valid work is retained on current `main` |
| `REPRESENTED` | Current `main` already contains the same business intent |
| `SUPERSEDED` | A named, newer artifact or decision replaces the item |
| `ARCHIVED` | The item has historical value but no active product role |
| `DISCARDED` | The item is invalid, stale, duplicate, or reproducibly generated |
| `ROUTE_REQUIRED` | The item remains valid but belongs to another named owner |

### Capability Policies

- One inventory snapshot contains every discovered inventory item.
- One inventory item has one recovery record before destructive change.
- One inventory item has one final disposition record.
- An integrated item links its original provenance to its resulting commit.
- A routed item names its owner and an exact next action.
- Concrete deployment material routes to knb and never enters Smackerel.

## Unresolved Evidence Constraints

- The current inventory has not been executed during specification authoring.
- Historical inventory facts may be stale when runbook execution begins.
- Exact item identities and final dispositions remain execution evidence.
- No recovery artifact, integration result, validation result, or push result is
  claimed by this specification.
- Packet-level artifact lint cannot pass while the intentionally uncreated
  sibling planning artifacts remain absent.

## Actors And Permissions

| Actor | Goals | Permission boundary |
|---|---|---|
| Reconciliation operator | Preserve, classify, integrate, and reconcile local work | May mutate operational Git state only during separately authorized runbook execution |
| Smackerel artifact owner | Decide whether recovered product work remains valid | May change only artifacts within the owner's declared boundary |
| knb deployment owner | Retain and apply concrete deployment configuration | Owns all target-specific deployment values and assets |
| Spec or bug owner | Continue valid nonterminal work | Owns status, planning, and certification changes for that item |
| Repository maintainer | Accept reviewed reconciliation commits | Allows only non-forced advancement of current `main` |

## Use Cases

### UC-OPS-006-001: Inventory Local Work

- **Actor:** Reconciliation operator
- **Preconditions:** The run has separate authorization and a fresh repository
  binding.
- **Main Flow:**
  1. Capture each required inventory class.
  2. Compare the result with the historical baseline.
  3. Add every newly discovered item to the review ledger.
- **Alternative Flows:** Stop mutation when another process changes the
  checkout or the inventory contradicts the baseline.
- **Postconditions:** Every discovered item has a unique ledger entry.

### UC-OPS-006-002: Preserve And Classify An Item

- **Actor:** Reconciliation operator
- **Preconditions:** The item appears in the current inventory.
- **Main Flow:**
  1. Create and verify recovery material.
  2. Check ownership and sensitivity.
  3. Review content, provenance, dependencies, and current relevance.
  4. Assign exactly one disposition class.
- **Alternative Flows:** Stop work on the item when recovery fails, ownership is
  unknown, or sensitive content lacks safe handling.
- **Postconditions:** The item remains recoverable and has an evidence-backed
  disposition proposal.

### UC-OPS-006-003: Integrate Or Route Valid Work

- **Actor:** Smackerel artifact owner
- **Preconditions:** Review proves that the item contains valid unique work.
- **Main Flow:**
  1. Confirm the owning artifact boundary.
  2. Integrate Smackerel-owned behavior with provenance.
  3. Route foreign-owned behavior to its named owner.
  4. Validate integrated behavior through the Smackerel CLI.
- **Alternative Flows:** Classify the item as represented or superseded when
  current tracked work already carries its intent.
- **Postconditions:** Valid work is either retained on `main` or preserved with
  an exact owner action.

### UC-OPS-006-004: Route Concrete Deployment Configuration

- **Actor:** knb deployment owner
- **Preconditions:** Recovered material contains concrete deployment
  configuration.
- **Main Flow:**
  1. Keep concrete values out of Smackerel.
  2. Preserve the source material in protected recovery storage.
  3. Record the knb owner and exact transfer action.
  4. Retain only target-neutral, fail-loud seams in Smackerel.
- **Alternative Flows:** Split mixed content by ownership while preserving
  shared provenance.
- **Postconditions:** Concrete deployment configuration belongs only to knb.

### UC-OPS-006-005: Preserve Open Specs And Bugs

- **Actor:** Spec or bug owner
- **Preconditions:** A tracked spec or bug is nonterminal under its workflow
  mode.
- **Main Flow:**
  1. Record its path, status, and workflow mode.
  2. Keep owner-controlled state unchanged.
  3. Name the next owner and exact next action.
- **Alternative Flows:** Route contradictory or stale ownership metadata for
  owner review without changing it.
- **Postconditions:** Open work remains actionable on `main`.

## Expected Behavior

### EB-001: Revalidate The Inventory

The operation MUST create a fresh inventory before changing refs, ignored
files, or tracked content. The supplied baseline is context, not current-state
proof.

The inventory MUST cover every class listed in the Inventory Coverage section.
Newly discovered items MUST enter the same review and disposition process.

### EB-002: Preserve Before Mutation

The operation MUST create verified recovery material before deleting a ref,
removing an ignored item, or changing the convenient reachability of an object.

Failed or incomplete recovery verification MUST stop destructive work.

### EB-003: Review Semantically

The operation MUST classify each item from its content, relationships, current
relevance, and overlap with `main`. Names, timestamps, and commit subjects alone
MUST NOT determine disposition.

Related commits MUST be reviewed in dependency order. Mixed-content items MUST
be separated by ownership without losing provenance.

### EB-004: Record Durable Dispositions

Every item MUST have a durable disposition record before local cleanup. The
record MUST identify the item, classification, evidence, retained result,
recovery location when applicable, owner, and exact next action.

The record becomes durable only after it exists on verified remote `main`.

### EB-005: Integrate Retained Work

Valid unique work MUST enter current `main` through traceable commits. The
integration MUST preserve source provenance and pass the applicable Smackerel
CLI validation.

Generated outputs MUST remain ignored when tracked sources reproduce them.

### EB-006: Protect Sensitive Material

Candidate work MUST be checked for secrets, personal data, and concrete target
configuration before display or integration. Records MUST contain only
redacted finding classes and safe recovery metadata.

Sensitive values MUST remain outside commits, evidence, logs, and chat output.

### EB-007: Preserve Open Specs And Bugs

Nonterminal specs and bugs MUST remain tracked on `main`. Reconciliation MUST
NOT alter their owner-controlled status, execution claims, or certification
fields.

Each open item MUST receive a handoff record with its current status, workflow
mode, next owner, and exact next action.

### EB-008: Respect Product And Deployment Ownership

All concrete deployment configuration MUST live in knb. Smackerel MUST retain
only target-neutral contracts, required configuration seams, and fail-loud
validation.

Recovered concrete deployment material MUST be preserved and routed to the knb
deployment owner. It MUST NOT be normalized into Smackerel documentation,
examples, tests, or configuration.

### EB-009: Reconcile Against Current Remote State

The operation MUST integrate against current `origin/main`. Remote movement
MUST trigger reconciliation and renewed validation.

The operation MUST NOT overwrite newer remote history. It MUST NOT force-push,
rewrite published history, or publish local archive tags.

### EB-010: Clean Up Last

The operation MAY remove reviewed local residue only after recovery verifies,
all dispositions are durable, retained work is pushed, and remote identity is
confirmed.

The operation MUST NOT prune unreachable objects to manufacture a clean
inventory result.

### EB-011: Separate Planning From Execution

Creating this specification MUST NOT execute reconciliation. It MUST NOT
commit, push, mutate refs, remove ignored files, or change runtime state.

Runbook execution requires separate authorization and a fresh repository
binding.

## Inventory Coverage

| Inventory class | Required review |
|---|---|
| Branches | Enumerate local branches, upstream relationships, divergence, and unique reachable work |
| Worktrees | Enumerate every worktree, associated branch or detached state, lock state, and active use |
| Stashes | Review every stash, including staged, unstaged, and untracked content represented by it |
| Local tags | Review tag type, target object, unique reachable history, tagged-only content, and purpose |
| Dangling commits | Review metadata, parent graph, changed paths, semantic diff, dependencies, and overlap with current `main` |
| Ignored local config and generated state | Review provenance, sensitivity, ownership, current references, and reproducibility |
| Ignored provider migration scripts | Review unique behavior, provider ownership, current relevance, tests, and secret or personal-data risk |
| Dist outputs | Identify tracked generating sources, reproducibility, current consumers, and whether any unique content exists |
| Stale runtime records | Identify the owning process, active references, lifecycle command, recovery need, and safe removal method |
| Open specs and bugs | Record path, mode-aware terminal state, workflow mode, next owner, and exact next action |

## No-Loss Semantics

1. **Complete coverage:** Every item in the initial and refreshed inventories
   MUST appear in the disposition ledger.
2. **Recovery first:** Verified recovery MUST exist before any destructive
   action affects an item.
3. **Semantic review:** A filename, age, tag prefix, or generated-looking shape
   MUST NOT prove disposability.
4. **Provenance continuity:** Integrated work MUST retain a trace to its source
   object or ignored item.
5. **Durable decision:** A local note or external backup alone MUST NOT satisfy
   the disposition requirement.
6. **Sensitive-data containment:** Recovery MUST NOT cause secret, personal, or
   target-specific data to enter Smackerel or public evidence.
7. **Explicit discard proof:** `DISCARDED` requires a semantic reason and
   verified recovery or reproducibility evidence.
8. **No cleanup by pruning:** Object pruning MUST NOT substitute for review,
   disposition, or recovery.
9. **Stop on uncertainty:** Contradictory evidence or unknown ownership MUST
   stop mutation for the affected item.

## Product And knb Boundary

### Smackerel Retains

- Target-neutral deployment contracts and schemas.
- Required configuration keys and environment-variable seams.
- Placeholder-only examples that reveal no concrete target.
- Validation that rejects missing or empty required deployment inputs.
- Generic build artifacts and product behavior.

Smackerel MUST NOT introduce defaults or fallback values for required
deployment inputs. Missing input MUST fail loudly.

### knb Retains

- All concrete deployment configuration.
- Target and host identities.
- Network addresses, names, ports, listeners, and deployment order.
- Concrete parameters, manifests, adapters, and target runbooks.
- Reverse-proxy, firewall, init-system, and host-singleton configuration.
- Real secret values in their approved encrypted form.

### Recovered Mixed Content

When one recovered item contains both generic product work and concrete
deployment configuration, the operation MUST preserve both parts. Generic work
may enter Smackerel after review. Concrete deployment work MUST route to knb.

The disposition record MUST preserve their shared provenance without copying
concrete values into Smackerel.

## Acceptance Scenarios

### SCN-OPS-006-001: Fresh Inventory Covers Every Class

Given a separately authorized reconciliation run.
And a supplied historical inventory baseline.
When the operator captures the current local inventory.
Then every required inventory class is represented.
And every newly discovered item enters the review ledger.
And no local mutation occurs before the inventory is complete.

### SCN-OPS-006-002: Recovery Failure Stops Destructive Work

Given an inventoried item that could become harder to recover.
When its recovery material cannot be verified.
Then the item remains unchanged.
And its disposition remains unresolved.
And the operation reports the blocking recovery failure.

### SCN-OPS-006-003: Equivalent Work Is Represented, Not Duplicated

Given a local item whose business intent may already exist on current `main`.
When semantic comparison proves equivalent behavior and coverage.
Then the item receives `REPRESENTED`.
And the record names the current artifact or commit that carries the intent.
And no duplicate implementation enters `main`.

### SCN-OPS-006-004: Unique Work Is Integrated With Provenance

Given a preserved item with valid unique behavior.
When the owning reviewer accepts the behavior.
Then the behavior enters current `main` through a traceable commit.
And applicable Smackerel CLI validation succeeds.
And the disposition links source provenance to the integrating commit.

### SCN-OPS-006-005: Concrete Deployment Material Routes To knb

Given recovered material containing concrete deployment configuration.
When the item is classified by ownership.
Then the concrete material does not enter Smackerel.
And it remains recoverable for the knb deployment owner.
And the disposition names that owner and an exact action.

### SCN-OPS-006-006: Required Smackerel Seam Fails Loudly

Given retained Smackerel code that needs a deployment-supplied value.
When the required value is missing or empty.
Then validation rejects the configuration.
And no default or fallback target value is substituted.

### SCN-OPS-006-007: Ignored Provider Migration Work Is Reviewed

Given an ignored provider migration script.
When the operator reviews its behavior, ownership, tests, and sensitivity.
Then valid unique behavior is integrated or routed to its owner.
And duplicate, invalid, or superseded behavior receives evidence for that class.
And the ignored source remains recoverable until its disposition is durable.

### SCN-OPS-006-008: Generated Output Is Removed Only When Reproducible

Given an ignored generated or dist output.
When tracked sources reproduce its required behavior through the repository CLI.
Then the output may receive `DISCARDED`.
And the disposition names the generating source and validation evidence.
And unique output that cannot be reproduced remains preserved.

### SCN-OPS-006-009: Runtime Record Is Not Removed While Active

Given a stale-looking runtime record.
When its owning process or lifecycle reference remains active.
Then the record is not removed.
And the item remains preserved for owner review.

### SCN-OPS-006-010: Open Work Keeps Owner-Controlled State

Given a tracked nonterminal spec or bug.
When the reconciliation inventory reaches that item.
Then its status and certification fields remain unchanged.
And its mode-aware state is recorded.
And the handoff names a next owner and exact action.

### SCN-OPS-006-011: Remote Movement Does Not Get Overwritten

Given `origin/main` changes during reconciliation.
When the operator prepares retained work for integration.
Then the work is reconciled onto the newer remote base.
And affected validation runs again.
And the push does not rewrite remote history.

### SCN-OPS-006-012: Closure Requires Every Invariant

Given all reviewed changes and dispositions are present on remote `main`.
When the operator runs the final invariant check.
Then only `main` remains as a local branch.
And exactly one worktree remains.
And no stash or local tag remains.
And the tracked worktree is clean.
And local and remote `main` identify the same commit.
And every inventory item has a durable disposition.

## Acceptance Criteria

1. A fresh inventory accounts for every required inventory class.
2. Every discovered item has exactly one final disposition class.
3. Every destructive action is preceded by verified recovery for its item.
4. Every retained change has traceable provenance and exists on remote `main`.
5. Every non-integrated item has semantic evidence and safe recovery metadata.
6. Every local tag and dangling commit has an individual reviewed disposition.
7. Every ignored local config item, generated item, provider migration script,
   and dist output has an individual reviewed disposition.
8. Every stale runtime record has owner, activity, lifecycle, and recovery
   evidence before removal.
9. Every open spec and bug retains owner-controlled state and has an actionable
   handoff.
10. Smackerel contains no concrete deployment configuration after integration.
11. All concrete deployment configuration is preserved for, or retained in,
    knb.
12. Every required Smackerel deployment seam rejects missing or empty input
    without a default or fallback.
13. Applicable Smackerel CLI validation passes in the execution session.
14. No secret value, personal data, or concrete target value appears in commits,
    disposition records, evidence, or chat output.
15. No force-push, published-history rewrite, remote tag deletion, or object
    pruning occurs.
16. The final local state has only `main`, one worktree, no stash, no local tag,
    and no tracked worktree change.
17. Local `main`, remote-tracking `main`, and remote `main` identify the same
    commit after push verification.

## Invariants

- **INV-001 No loss:** No inventoried item becomes unrecoverable before its
  durable disposition exists.
- **INV-002 Complete accounting:** Every discovered item has exactly one ledger
  row and one final disposition.
- **INV-003 Backup first:** Recovery verification precedes ref deletion and
  ignored-item removal.
- **INV-004 Current base:** Integration uses current remote `main`, never a stale
  supplied baseline.
- **INV-005 Provenance:** Every integrated result traces to its recovered source.
- **INV-006 Product boundary:** Smackerel remains generic and target-neutral.
- **INV-007 Deployment ownership:** All concrete deployment configuration lives
  in knb.
- **INV-008 Fail loud:** Required Smackerel deployment seams have no defaults or
  fallback target values.
- **INV-009 Confidentiality:** Secrets, personal data, and concrete target values
  never enter committed or conversational evidence.
- **INV-010 Artifact ownership:** Reconciliation does not change foreign-owned
  planning, status, or certification content.
- **INV-011 History safety:** The operation never force-pushes, rewrites
  published history, or prunes objects.
- **INV-012 Completion honesty:** The packet remains nonterminal until every
  acceptance criterion has current-session execution evidence.

## Exposure Contract

| Capability | Surface class | Surface id | Status | Plan |
|---|---|---|---|---|
| Local Git reconciliation | internal | `runbook.md` | planned | An authorized operations owner executes OPS-006 after a fresh repository binding |

## Non-Functional Requirements

- **Auditability:** A reviewer can trace every discovered item to one disposition
  and its evidence.
- **Recoverability:** Recovery material remains verifiable until remote
  dispositions and retained work are confirmed.
- **Confidentiality:** Reviews and records expose no sensitive content.
- **Idempotence:** Rechecking an already reconciled repository creates no new
  product change.
- **Portability:** Requirements use repository roles and generic ownership, not
  machine-specific values.
- **Determinism:** The same inventory and evidence produce the same coverage and
  invariant results.

## Non-Goals

- Product behavior changes unrelated to recovered work.
- Status or certification transitions for existing specs and bugs.
- Remote tag deletion.
- Production deployment or runtime mutation.
- Published-history rewriting.
- Object pruning.
- Reconciliation execution during specification authoring.