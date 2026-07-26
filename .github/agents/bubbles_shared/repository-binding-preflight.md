## Repository Binding Preflight

This module is the sole shared prompt contract for selecting the work repository.
It defines repository authority only. It does not select workflow modes, choose
work, authorize artifacts, or widen any framework, product, deployment, release,
or specialist ownership boundary.

### Mandatory Ordering

Every repository-sensitive top-level command must obtain one current actionable
`RepositoryBindingDecision` before it reads repository-local session state,
expands a relative target, enumerates `specs/`, selects work, invokes a
repository-owned command, or dispatches a specialist.

The preflight order is fixed:

1. Parse only repository intent, concrete target syntax, and request class.
2. Require the host-supplied `sessionId`, external `sessionControlFile`,
  caller-observed `expectedControlRevision`, and declared `workspaceRoots`
  inventory.
3. Canonicalize and deduplicate eligible Git worktree roots.
4. Reconcile the external session control record and any carried binding packet.
5. Resolve exactly one root using the authority order below.
6. Commit a command-level decision atomically before repository-local work.
7. Emit the operator preflight line and actionable local decision.
8. Only then may a consumer mirror state, discover specs, or dispatch work.

### Required Host Context

On VS Code agent surfaces, materialize this context with the installed
`repository-binding-host-context.sh`, passing the host's per-chat
`{{VSCODE_TARGET_SESSION_LOG}}` value and every declared workspace folder. The
adapter derives a stable opaque session id and private external control path;
it does not select a repository. Other hosts must provide an equivalent explicit
adapter contract or refuse before repository-local work.

- `sessionId` is an opaque identifier supplied by the active interactive host
  session. It must not be derived from a repository file, process ID, CWD,
  prompt location, or host repository metadata.
- `sessionControlFile` is an absolute private path outside every candidate
  repository. Its path is never propagated in work packets.
- `expectedControlRevision` is the integer revision observed by the host adapter:
  `0` when the control record is absent, otherwise the current record revision
  after session/path identity checks. Callers pass it verbatim as
  `--expected-control-revision`; they never infer, increment, or repair it.
- `workspaceRoots` contains only host-declared candidate folders. Inventory
  order is discarded after canonicalization and is never selection authority.

Missing, malformed, non-private, or repository-local host context fails loud.
There is no ambient fallback and no bypass. A control revision changed after the
adapter observation is a compare-and-swap refusal; rerun the adapter to obtain a
fresh observation before retrying.

### Canonical Identity And Eligibility

The canonical repository identity is the physical Git worktree top-level:

1. Enter candidate directories with `cd -P` and obtain `pwd -P`.
2. Resolve Git identity with `git -C <physical-dir> rev-parse --show-toplevel`.
3. Physicalize that top-level again with `cd -P` and `pwd -P`.
4. Deduplicate exact canonical paths. Symlink spellings of one worktree collapse;
   linked Git worktrees remain distinct.
5. Preserve filesystem path bytes and case. Do not use `realpath`, `readlink -f`,
   case folding, workspace order, repository name, or Git common-directory
   identity.

A foundation-eligible root is a Git top-level with either the canonical source
markers (`VERSION`, `install.sh`, `bubbles/scripts/cli.sh`, and `agents/`) or the
installed-framework markers (`.github/bubbles/release-manifest.json` and
`.github/agents/`). Command-specific eligibility and ownership run after binding.

### Closed Authority Order

Resolve command-level authority in this exact order:

1. one valid explicit repository root or exact concrete target;
2. one valid same-session durable work boundary;
3. the sole eligible canonical root in a true single-repository inventory.

The machine authority vocabulary is:

- `explicit-repository-root`
- `concrete-target`
- `resolved-natural-language`
- `durable-work-boundary`
- `single-eligible-repository`
- `scoped-scenario-node`

Prompt source, chat/process/terminal/tool CWD, host `repository` metadata, active
editor, recent files, searches, incidental absolute-path access, timestamps,
workspace order, and scan order are diagnostic-only. They cannot establish,
switch, repair, or override a boundary.

A valid explicit root may intentionally establish, confirm, switch, or repair
current-session malformed/stale/conflicting carried state. Failed explicit
resolution leaves the prior valid boundary byte-for-byte unchanged. Conflicting
or stale inherited authority refuses; recency never chooses a winner.

### Decisions And Scoped Overrides

An actionable command decision carries:

- canonical `repositoryRoot` and safe `repositoryAlias`;
- `sessionId`, `decisionId`, exact `controlRevision`, and the canonical external `controlPathDigest`;
- `authority`, `transition`, and `targetKind`;
- `scopeKind: command`, `scopeId: null`;
- `pathVisibility: local`, `actionable: true`.

Every successful command-level establish, continue, confirm, or switch increments
the external control revision. A child packet is current only when its session,
root, decision ID, and revision exactly match that control record.

An explicit goal-node decision uses `scopeKind: goal-node`, a non-empty node ID,
`authority: scoped-scenario-node`, and `transition: scoped-override`. It never
mutates command-level affinity and cannot be consumed outside its node.

### Operator Output And Refusal

Successful preflight emits one stable first line naming repository, canonical
root, source, and affinity transition before any repository-local operation.
Targetless multi-root ambiguity, stale/malformed/conflicting authority, invalid
explicit targets, and zero eligible roots emit a structured refusal with:

- non-success outcome and stable reason code;
- observed authority state and diagnostic-only signals;
- `requiredInput.field: repositoryRoot` plus canonical-root remediation;
- `affinity: unchanged`;
- `repoLocalSideEffects: zero`.

Refusal never recommends changing CWD, editor focus, prompt source, workspace
order, or recent activity.

### Projection And Privacy

Local actionable packets retain the canonical root for authorized same-session
consumers. A public or committed projection replaces only the root with
`<redacted-local-root>`, sets `pathVisibility: redacted`, and sets
`actionable: false`. A redacted projection cannot authorize discovery, mirror
writes, repository commands, or dispatch. The external control-file path never
appears in either projection.

### Production Interface

The single production owner is `bubbles/scripts/repository-binding.sh`:

- `preflight` resolves and atomically commits command affinity;
- `validate-packet` requires exact current provenance and can emit a redacted
  non-actionable projection;
- `discover-specs` validates the packet before enumerating only
  `<repositoryRoot>/specs`;
- `mirror-session` validates the packet before a post-selection ignored mirror.

The mirror is never preflight authority. Consumers must call or validate this
shared production owner; they must not copy its selection rules into prompts or
infer a repository from their own tool context.