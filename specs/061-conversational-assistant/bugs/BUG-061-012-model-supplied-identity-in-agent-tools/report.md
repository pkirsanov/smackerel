# Report: BUG-061-012 — Server-derived principal for agent tools

### Summary

Filed 2026-08-14 from `docs/Product_Delivery_Plan.md` § P1 hole 2 (Stage 1, Critical). The defect is
verified from source, not inferred from the plan: `retrieval_search` declares `user_id` as a
required, model-filled tool argument and validates only that it is non-empty.

### Scenario-first TDD ordering

The mode forces `forceTddMode: scenario-first`, so the sequence below is the contract rather than a
narrative convenience. The failing proof was captured against pre-fix source first, and only then
was the fix proven against the same tests.

**RED stage — the failing proof, captured first.** The four contract tests were run against the
defect-carrying source reconstructed at `0f4b4826`, with the new tests kept in place:

```text
$ ./smackerel.sh test unit --go --go-run '^(TestToolSchemas_DeclareNoCallerIdentity|TestRetrieval_NoPrincipalFailsClosed|TestRetrieval_GrantRequired|TestRetrieval_ScopesToAuthenticatedPrincipal)$' --verbose
FAIL    github.com/smackerel/smackerel/internal/agent/tools/retrieval   1.267s
FAIL    github.com/smackerel/smackerel/internal/agent/tools/toolscontract      0.102s
exit code: 1
sha256: 4bbe6e6a86a718c9c40329e04078ec03cd339956f135f3c7bce4940fc97987c6
```

**GREEN stage — the same tests now pass** against the fix at `0dcb9d1f`, inside the full unit lane:

```text
$ ./smackerel.sh test unit --go
146 packages, 0 failed
exit code: 0
sha256: 89f74e33a026b24243b14b653e8dbb6e6b391e389e7f00642c9308bbc8e265dc
```

Red before green is what makes these regressions non-vacuous. Full detail, including which files
were reverted to produce the red state and the confirmation that the tree was restored exactly, is
in § Adversarial proof (R4.3).

### Code Diff Evidence

Two commits carry the implementation. Neither touches `internal/agent/registry.go`,
`internal/agent/executor.go`, or `internal/agent/router.go`, which is the design's central claim
holding: the existing context seam sufficed and no agent-package signature changed.

```text
$ git show --stat --oneline 20b0376a
20b0376a fix(BUG-061-012): server-derived principal for agent tools
 internal/agent/judgment.go                         |  13 +-
 internal/agent/judgment_test.go                    |  66 +++++++
 internal/agent/tools/microtools/chaos_065_test.go  |  21 ++-
 internal/agent/tools/microtools/entity_resolve.go  |  41 +++--
 internal/agent/tools/microtools/entity_resolve_test.go  |  81 ++++++--
 internal/agent/tools/notification/notification_test.go  |  82 +++++++--
 internal/agent/tools/notification/propose.go       |  26 ++-
 internal/agent/tools/retrieval/tool.go             |  22 +++
 internal/agent/tools/retrieval/tool_test.go        | 150 ++++++++++++++-
 internal/agent/tools/toolscontract/schema_contract_test.go |  46 +----
 internal/annotation/classifier_bridge.go           |  12 +-
 internal/auth/system_session.go                    |  59 ++++++
 internal/auth/system_session_test.go               |  86 +++++++++
 internal/pipeline/agent_bridge.go                  |   5 +-
 internal/scheduler/agent_bridge.go                 |   9 +-
 internal/telegram/agent_bridge.go                  |  59 ++++++
 internal/telegram/agent_bridge_principal.go        |  88 +++++++++
 internal/telegram/agent_bridge_test.go             | 203 +++++++++++++++++++++
 tests/integration/agent/retrieval_principal_test.go     | 170 +++++++++++++++++
 tests/integration/assistant/entity_resolve_test.go |  98 +++++++++-
 20 files changed, 1223 insertions(+), 114 deletions(-)
exit code: 0
```

```text
$ git show --stat --oneline 0dcb9d1f
0dcb9d1f fix(BUG-061-012): repair the 3 consumers the narrowed tool schemas broke
 internal/agent/judgment_test.go                    |  1 -
 internal/agent/tools/toolscontract/schema_contract_test.go | 61 ++++++++++++----
 internal/assistant/openknowledge/agenttool/substrate_tool.go | 20 ++++---
 tests/e2e/microtools/overlays_e2e_test.go          | 43 +++++++++----
 tests/stress/assistant_retrieval_p95_test.go       | 14 ++++-
 5 files changed, 110 insertions(+), 29 deletions(-)
exit code: 0
```

The excluded families are empty across the whole range:

```text
$ git diff --name-only 0f4b4826..0dcb9d1f -- internal/agent/registry.go internal/agent/executor.go internal/agent/router.go
exit code: 0
$ git diff --name-only 0f4b4826..0dcb9d1f -- 'migrations/*' 'proto/*' 'config/*.yaml'
exit code: 0
```

### Completion Statement

**IMPLEMENTED AND TEST-VERIFIED. NOT YET CERTIFIED.** All eight Test Plan tests exist, execute, and
pass; the adversarial proof required by R4.3 is recorded below; and every lane this bug owns is
green — `lint`, `format`, `unit`, `integration`, and `e2e` all exit `0` against the current tree.
The three first-party consumers that the schema narrowing broke were repaired at `0dcb9d1f`, which
is what moved `e2e` from `1` to `0`. Scope 1 is `Done` and all 16 DoD items are closed with inline
evidence.

**The packet status is `in_progress`, not `done`, and the reason is not cosmetic.**
`bugfix-fastlane` sets `blockOnMissingSpecialistExecution: true` under the `delivery-completion-v1`
transition-audit profile. Six required specialist phases — `regression`, `simplify`, `stabilize`,
`security`, `validate`, `audit` — have never been executed, and Gate G022 treats claiming an
unexecuted phase as fabrication. This report accordingly still owes a `### Validation Evidence` and
an `### Audit Evidence` section, both owned by other agents. Separately, Gate G136 blocks any
terminal transition while `uservalidation.md` carries unchecked human-acceptance items; two remain,
and no agent may check them on the author's behalf without fabricating the acceptance the gate
exists to require. `state.json` records the full pending-gate list.

The `stress` lane exits `1`. The failure is present at baseline: a clean-room `git worktree` at
`0f4b4826` — the commit immediately before any BUG-061-012 work — fails the same package with the
same exit code. That comparison is the whole of the argument, and it is recorded verbatim in
§ Discovered issues 5 along with what was ruled out and what remains unknown. The specific failing
test is **not** identified, and this packet does not claim it is.

Two findings are recorded as **open** rather than resolved, because closing them here would be
false: the Telegram principal resolver is correct and tested but has no production caller, so P1
hole #3 stays open (§ Discovered issues 3); and the `stress` lane above needs an owner decision.

This supersedes both the earlier "PARTIALLY IMPLEMENTED" statement — which described an intermediate
ratchet state that no longer exists — and the "NOT VERIFIABLE AS DONE" statement, which rested on
the `e2e` failure that has since been fixed and on a `stress` failure later shown to be present at
baseline.

### Test Evidence

Every block below was produced by `evidence-capture.sh`. Each `sha256` covers the full output of
that one command and is re-derivable with `--verify`.

**Current tree (`0dcb9d1f`) — the authoritative lane results:**

| Lane | Command | Exit | sha256 |
|---|---|---|---|
| lint | `./smackerel.sh lint` | `0` | `e60dc02e4b8c160850fd9cda6e2485a5f44a800d8f694a6f5f42b6f62dbe3735` |
| format | `./smackerel.sh format --check` | `0` | `d5e98a0f123acf3d8ee3674ccc338b70f64c26d87431e2f457bb1ed69970c577` |
| unit (full, 146 pkgs) | `./smackerel.sh test unit --go` | `0` | `89f74e33a026b24243b14b653e8dbb6e6b391e389e7f00642c9308bbc8e265dc` |
| integration (full) | `./smackerel.sh test integration` | `0` | `8399bd7b782ca336d58a9f8a35730bd28948cec76d528e7a6ac3a66e3f286725` |
| e2e (full) | `./smackerel.sh test e2e` | `0` | `de7f6cd5d993beb8a8a4e6438fbf2bba37e61e2d0f92297abe188384d5861498` |
| **stress (full)** | `./smackerel.sh test stress` | **`1`** | `229575d80a5de113ef107398a2d8920cb35b31ef6d12f1364278b59e9998014e` |
| T-01..T-07 (scoped) | `./smackerel.sh test unit --go --go-run '^(TestToolSchemas_…\|…\|TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant)$' --verbose` | `0` | recorded inline in `scopes.md` |
| T-08 (scoped) | `./smackerel.sh test integration --go-run '^TestRetrieval_EndToEndUnderHTTPSession$'` | `0` | recorded inline in `scopes.md` |

The `stress` row is the only red lane and is proven pre-existing in § Discovered issues 5.

**Superseded — the earlier capture, kept for provenance.** These were run before the consumer
repairs at `0dcb9d1f` and before the full lanes were exercised; the two red rows are exactly what
the repair commit addressed. They are retained so the transition from red to green is traceable
rather than silently overwritten:

| Lane | Command | Exit | sha256 |
|---|---|---|---|
| unit (full) | `./smackerel.sh test unit --go` | `0` | `814a44fff95419641456216ce2f2e77428a5a262b68f32c071973a50d3604a2a` |
| integration (T-08) | `./smackerel.sh test integration --go-run '^TestRetrieval_EndToEndUnderHTTPSession$'` | `0` | `9e017dd4abf5a406bbdb8a315c40004bdd9f560ac4353449f1cb812c2cd0e440` |
| lint | `./smackerel.sh lint` | `0` | `221aaa2e452506b1f09f58d9d8ce7b3c7581570bf65dda02b2f8ebebc5eece6b` |
| format | `./smackerel.sh format --check` | `0` | `81b6c08647039071dd4036abb69e1063cb663de7b96003e8544ab8b3da9ca5b8` |
| e2e (scoped, pre-repair) | `./smackerel.sh test e2e --go-run '^TestMicroToolOverlays_FullMatrix$'` | `1` | `c47842545b8f9f12f82c74bafa3d71724f8aae50cfc144a26dfc0d917debc3f3` |
| stress (scoped, pre-repair) | `./smackerel.sh test stress --go-run '^TestAssistantRetrievalStressP95$'` | `1` | `a6dedbbe3246f11bbaadd998758c9c481661f8bbb9945395665fde588ed85402` |

Per-test evidence for T-01..T-08 is recorded inline against each DoD item in `scopes.md`.

#### One failure-shaped line in the integration capture is expected

The integration lane surfaces this line:

```text
ERROR: model envelope validation failed (spec 045 FR-045-002) ... bug-045-fixture-llm-20gib
```

It is **not** a defect and must not be chased. It is an intended negative assertion from a
**passing** test in `tests/integration/config_validate_test.go`, which deliberately configures an
oversized model to prove the validator refuses it. The lane exits `0`.

---

## After Fix — What landed

### The affected set was wrong in the delivery plan and in this packet's first filing

The plan named `retrieval`, `notification/propose`, `notification/execute`. The actual set was four
tools, and `notification/execute` was **not** among them:

```text
$ grep -rn '"user_id"' --include='*.go' internal/agent/tools/ | grep -v _test
internal/agent/tools/microtools/entity_resolve.go:153:  "required": ["input", "user_id"],
internal/agent/tools/recipesearch/tool.go:76:  "required": ["query", "user_id"],
internal/agent/tools/retrieval/tool.go:107:  "required": ["query", "user_id"],
internal/agent/tools/notification/propose.go:18:  "required": ["user_id", "what"],
```

They divided on whether the value was *used*, and that division decided the fix:

| Tool | `user_id` before | Consequence |
|---|---|---|
| `retrieval` | validated, then discarded | Decorative. Removal changed no behaviour. |
| `recipesearch` | validated, then discarded | Decorative. Removal changed no behaviour. |
| `microtools/entity_resolve` | `Resolver.Resolve(callCtx, in.UserID, ...)` | Load-bearing — the model chose whose entities resolved. |
| `notification/propose` | `UserID: in.UserID` on the proposal | Load-bearing — the model chose who was notified. |

All four are now fixed. The intermediate ratchet allowlist has been removed, so the contract test
fails on **any** offender rather than on any offender outside a list.

### What is in place now

| Surface | Mechanism |
|---|---|
| `retrieval_search` | `auth.SessionFromContext` → `retrieval_search_no_principal`; then `auth.GateGlobalCorpusRead` → `retrieval_search_grant_required` |
| `microtools/entity_resolve` | `auth.SessionFromContext` → `entity_resolve_no_principal`; resolves under `sess.UserID` |
| `notification/propose` | principal derived server-side, not from the argument |
| `recipesearch` | decorative `user_id` removed from schema |
| Scheduler, pipeline, judgment, annotation | `auth.SystemSession(<component>)` — explicit principal, explicitly empty grant set |
| Telegram | `NewBotPrincipalResolver` reuses the per-user token minter's chat→user and grant-derivation steps |

The two refusals are deliberately distinct: "nobody is here" is an operational defect on a surface,
"this caller may not" is the gate working. Collapsing them would hide the first behind the second.

### Tests that landed beyond the Test Plan

These were delivered but are not Test Plan rows. Crediting them here so the record is complete
rather than leaving real coverage undocumented:

| Test | What it holds |
|---|---|
| `TestTelegramBridge_UnreadableGrantsInjectNoPrincipal` | An unreadable grant read denies rather than degrading to "holds nothing" |
| `TestSystemSession_SourceIsNonEmptySoInjectionIsNotANoOp` | `WithSession` no-ops on an empty `Source`, so an empty constant would silently restore accidental fail-closed |
| `TestSystemSession_*` (3 further cases) | System principal shape: empty-not-nil scopes, `system:` prefix, `IsSystem` |
| `TestEntityResolveIgnoresModelSuppliedUserID` | A smuggled `user_id` argument cannot redirect resolution |
| `TestEntityResolveRejectsUnidentifiedCaller` | Absent principal refuses rather than resolving under an empty user |
| `TestRetrievalSearch_RejectsCallerSuppliedIdentity` | Schema rejects a caller-supplied identity outright |
| `TestPropose_UnidentifiedCaller_Errors` | Notification proposal refuses without a principal |
| `TestPropose_IgnoresModelSuppliedUserID` | A smuggled `user_id` cannot redirect who is notified |

---

## Adversarial proof (R4.3) — the regressions fail against pre-fix code

A regression whose fixtures all satisfy the broken path proves nothing. The four contract tests were
therefore run against the **pre-fix source**, reconstructed by checking the four defect-carrying
files out at the bug-filing commit `0f4b4826` while keeping the new tests.

Reverted files: `retrieval/tool.go`, `recipesearch/tool.go`, `microtools/entity_resolve.go`,
`notification/propose.go`. The revert was confirmed to restore the defect before the run:

```text
$ grep -rn '"user_id"' --include='*.go' internal/agent/tools/ | grep -v _test | grep -v services.go
internal/agent/tools/microtools/entity_resolve.go:153:  "required": ["input", "user_id"],
internal/agent/tools/recipesearch/tool.go:76:  "required": ["query", "user_id"],
internal/agent/tools/retrieval/tool.go:107:  "required": ["query", "user_id"],
internal/agent/tools/notification/propose.go:18:  "required": ["user_id", "what"],
```

```text
# R4.3 ADVERSARIAL: T-01..T-04 against PRE-fix source (0f4b4826)
$ ./smackerel.sh test unit --go --go-run '^(TestToolSchemas_DeclareNoCallerIdentity|TestRetrieval_NoPrincipalFailsClosed|TestRetrieval_GrantRequired|TestRetrieval_ScopesToAuthenticatedPrincipal)$' --verbose
exit: 1
lines: 530
sha256: 4bbe6e6a86a718c9c40329e04078ec03cd339956f135f3c7bce4940fc97987c6
--- failure-shaped lines from the omitted region ---
FAIL
FAIL    github.com/smackerel/smackerel/internal/agent/tools/retrieval   1.267s
FAIL
FAIL    github.com/smackerel/smackerel/internal/agent/tools/toolscontract      0.102s
```

Both packages carrying T-01..T-04 fail against pre-fix code and pass against the fix. The
regressions are non-vacuous.

The working tree was then restored and confirmed identical to the commit:

```text
$ git checkout HEAD -- <the four files>
$ git status --porcelain -- internal/ tests/
(empty)
```

---

## Rollback — documented and verified

`design.md` § Rollback specifies a single revert: no migration, no persisted state, no config key.
That claim was exercised mechanically during the adversarial run above — production files were moved
back to pre-fix content and forward again with `git checkout` alone, with no schema, data, or
configuration step at either end, and `git status` confirmed an exact restore. Rollback is a revert
of commit `20b0376a`.

---

## Consumer impact sweep — the finding, resolved

The design predicted this sweep would be empty:

> First-party references to that schema are the tool definitions themselves and their tests. No
> navigation, breadcrumb, redirect, deep link, API client, or generated client consumes it.

**That prediction was wrong.** Three first-party consumers existed. All three are now repaired, at
commit `0dcb9d1f`. The prediction is preserved above rather than edited away, because the reason it
was wrong is the durable lesson — see § Discovered issues 4.

**1. `tests/stress/assistant_retrieval_p95_test.go:140`** invoked `retrieval_search` with
`context.Background()` — no session — and a `user_id` argument the schema no longer accepts:

```go
input := json.RawMessage(`{"query":"what about Tailscale ACLs","user_id":"stress-u-1","top_k":5}`)
...
_, err := tool.Handler(ctx, input)   // ctx := context.Background()
```

Every call returned `retrieval_search_no_principal`. **Repaired:** the authenticated session is now
built **once, before the workers start**, so session construction cannot land inside the measured
region and skew the percentile. That detail matters — injecting the session per call would have
fixed the failure and quietly corrupted what the test measures.

**2. `tests/e2e/microtools/overlays_e2e_test.go:258,277`** called `entity_resolve` with a `user_id`
argument and asserted the resolver observed it:

```go
env := callTool(t, microtools.EntityResolveToolName, map[string]any{
        "input": "the lease", "user_id": "u-076-3", "scope": "documents", "top_k": 5,
})
...
if entResolver.lastUser != "u-076-3" { t.Errorf("user-scoping leaked: ...") }
```

The assertion encoded exactly the behaviour this bug removes — that the argument decides the scope.
**Repaired:** `callTool` was split into `callTool` / `callToolAs`, so the identity reaches
`entity_resolve` through the context, which is now the only channel that can carry it. The assertion
still checks that scoping is honoured; it now checks it against the session rather than the argument.

**3. `internal/assistant/openknowledge/agenttool/substrate_tool.go`** — the only **production**
consumer of the three, and the one the sweep had least excuse to miss. It declared a `user_id`
property parsed into `invokeInput.UserID` that the Handler never read: the decorative form of the
same defect. **Repaired:** the property is removed. The caller identity the open-knowledge path
needs already rides the context. Note the boundary that was deliberately *not* crossed — the
scenario input schema in `config/prompt_contracts/open_knowledge.yaml` keeps its `user_id`, because
the facade fills that one server-side from `msg.UserID`. It is a different layer and not a model
argument.

Finding 3 also exposed a hole in T-01 itself: the contract test walked a directory and so covered
8 of 22 registration sites, which is why an openknowledge tool could declare an identity while the
test stayed green. T-01 now derives its file set from the registration call. A contract test that
enumerates by directory tests the directory, not the contract.

**Verification after repair:**

```text
$ ./smackerel.sh test e2e
exit: 0
sha256: de7f6cd5d993beb8a8a4e6438fbf2bba37e61e2d0f92297abe188384d5861498
```

```text
$ grep -n 'user_id' tests/stress/assistant_retrieval_p95_test.go \
    tests/e2e/microtools/overlays_e2e_test.go \
    internal/assistant/openknowledge/agenttool/substrate_tool.go
tests/e2e/microtools/overlays_e2e_test.go:118:// the entity_resolve handler and records the user_id it received so
tests/e2e/microtools/overlays_e2e_test.go:120:// BUG-061-012 that user_id originates in the authenticated session on
internal/assistant/openknowledge/agenttool/substrate_tool.go:68:// BUG-061-012: a `user_id` property was declared here and parsed into
internal/assistant/openknowledge/agenttool/substrate_tool.go:74:// config/prompt_contracts/open_knowledge.yaml keeps its `user_id`
```

Every surviving occurrence is a comment explaining the removal. Not one is a live argument. The
sweep is closed.

---

## Discovered issues

### 1. A sixth `Invoke` surface existed that no artifact named

`design.md` § C2 enumerates four surfaces (HTTP, Telegram, and "Scheduler, Pipeline, Judgment"), and
`bug.md` lists five callers of `Invoke`. `internal/annotation/classifier_bridge.go` was in neither,
and was found by grep rather than by the artifacts. It is now wired with
`auth.SystemSession("annotation")` alongside the other server-initiated surfaces. It has been added
to the `scopes.md` Change Boundary.

An enumeration produced by hand is only as complete as the reading that produced it; the schema
contract test is the durable protection, because it derives the tool set from source instead of
restating it.

### 2. `notification/execute.go` never had a `user_id`

The Test Plan and `docs/Product_Delivery_Plan.md` both imply `notification/execute` carried a
model-supplied identity. It did not, at the bug-filing commit or now:

```text
$ git show 0f4b4826:internal/agent/tools/notification/execute.go | grep -n 'user_id\|UserID'
88:             "user:"+envelope.UserID,
```

That single reference reads `UserID` off the `payloadEnvelope` round-tripped from the ConfirmStore —
server-held state, never a model argument. The file was correctly left unchanged. The record is
corrected here rather than inventing work to match it.

### 3. The Telegram fix is real, tested, and **dormant** — P1 hole #3 stays open

`telegram.NewAgentBridge` has **no production caller**. It is constructed only in tests:

```text
$ grep -rn 'NewAgentBridge' --include='*.go' . | grep -v '/\.git/'
./internal/agent/bridge.go:16://     hands to api.AgentInvokeHandler.Runner and telegram.NewAgentBridge,
./internal/telegram/agent_bridge.go:83:// NewAgentBridge constructs the bridge. Both arguments are required;
./internal/telegram/agent_bridge.go:86:func NewAgentBridge(runner AgentRunner, sender AgentSender) (*AgentBridge, error) {
./internal/telegram/agent_bridge_test.go:87:    bridge, err := NewAgentBridge(runner, &discardSender{})
./tests/e2e/agent/bs014_never_invent_test.go:221:       br, err := telegram.NewAgentBridge(runner, sender)
./tests/e2e/agent/telegram_replies_test.go:75:  br, err := telegram.NewAgentBridge(runner, sender)
```

`NewBotPrincipalResolver` likewise appears only in its own definition and in
`agent_bridge_test.go:92`. The file header records that wiring the bridge into the bot router is
scope-10 work which has not landed.

The **live** Telegram path is `assistant_adapter → facade`:

```text
$ grep -rn 'NewBotChatResolver' --include='*.go' cmd/
cmd/core/wiring_assistant_facade.go:328:                ResolveUser:     telegram.NewBotChatResolver(tgBot),
```

That path resolves a user for the assistant but never calls `auth.WithSession` — confirmed by the
non-test `WithSession` call-site list, which contains the HTTP authenticator, the four system
surfaces, and `agent_bridge.go`, but nothing under `cmd/core` or `internal/assistant`.

So the resolver added by this fix is correct and covered by T-05/T-06, but **nothing in production
constructs it**. A reader who concluded from this packet that the live Telegram surface is now
principal-bound would be wrong. **P1 hole #3 remains open** and needs the scope-10 router wiring
before any such claim can be made.

Re-verified in the current session: `grep -n 'NewBotChatResolver' cmd/core/wiring_assistant_facade.go`
still returns the single line `328: ResolveUser: telegram.NewBotChatResolver(tgBot),`. Nothing about
this finding has changed, and it is recorded here as **open**, not resolved.

### 4. The Consumer Impact Sweep was wrong, and the reason is reusable

The sweep predicted no first-party consumers of the narrowed schemas. Three existed:

| Consumer | Lane | Kind |
|---|---|---|
| `tests/stress/assistant_retrieval_p95_test.go` | `stress` | test |
| `tests/e2e/microtools/overlays_e2e_test.go` | `e2e` | test |
| `internal/assistant/openknowledge/agenttool/substrate_tool.go` | `e2e` | **production** |

All three are repaired at `0dcb9d1f`; the `e2e` lane now exits `0`. The mechanical detail of each
repair is in § Consumer impact sweep — the finding, resolved.

The reason the prediction failed is worth more than the fact that it did. **Every one of the three
sat in `stress` or `e2e` — and neither lane had been run at the moment the prediction was recorded.**
The sweep was performed against the lanes that were already green. A sweep that consults only the
lanes you already run confirms whatever you already believe; it cannot do anything else, because the
evidence it draws on is selected by the same assumption it is meant to test. It did not fail through
carelessness. It failed structurally.

The concrete correction is that a sweep following a **contract narrowing** must enumerate consumers
from source across the whole repository and then run the lanes that exercise them, in that order.
The third finding makes the point sharply: `substrate_tool.go` is production code, not a test, and a
sweep restricted to green lanes missed it as surely as it missed the other two.

### 5. The `stress` lane fails, and the failure is pre-existing

`./smackerel.sh test stress` exits `1` on the current tree
(sha256 `229575d80a5de113ef107398a2d8920cb35b31ef6d12f1364278b59e9998014e`). It is **not** caused by
this bug. The proof is a clean-room comparison in a `git worktree` checked out at `0f4b4826` — the
commit immediately before any BUG-061-012 work — run against the same lane:

```text
baseline worktree @ 0f4b4826
FAIL    github.com/smackerel/smackerel/tests/stress     380.987s
exit: 1

current tree @ 0dcb9d1f
FAIL    github.com/smackerel/smackerel/tests/stress     366.733s
exit: 1
```

The same package fails the same way on a tree containing none of this work. A baseline is the only
thing that can settle attribution here, and it settles it: the failure predates the bug and does not
gate it.

**What was ruled out.** Each of these is a negative result, recorded so the next reader does not
repeat the search:

- **Not the tests themselves.** Every stress test passes in isolation — two scoped batches were run
  across the package, both exit `0`. So no individual test is broken on its own terms.
- **Not shared fixture setup.** There is no `TestMain` in `tests/stress`; re-confirmed this session
  by `grep -rn "func TestMain" tests/stress/` → exit `1`, no matches. So there is no package-level
  setup or teardown to blame.
- **Not visible in the captured output.** No `--- FAIL:` line appears anywhere in the untruncated
  tail of the capture — only the package-level `FAIL` summary line. See the tooling limitation below
  for why.

**What is NOT known: which test fails.** This packet does **not** identify the failing test, and no
statement here should be read as though it does. What the three negatives jointly describe is a
failure that appears only in a full-package run and disappears under scoped runs — cross-test
interference, resource contention under concurrency, or a timing threshold that only a full run
reaches. Naming the mechanism would require running the package to completion with per-test output
preserved, which was not done.

> **Citation withheld.** This shape is sometimes filed against a recorded residue class. A search of
> this repository (`specs/`, `docs/`, `.github/bubbles/`) found no residue-class register matching
> that description, so the pattern is characterised on its own evidence above rather than cited to a
> register that may not exist here. Do not add such a citation without first confirming the target.

**Routing.** This is a `bubbles.test`-owned investigation into a lane this bug does not own, and it
needs the full-run reproduction that the negatives above scope out. It belongs in its own packet
rather than inside this one, because folding an unrelated failure that predates this work into a
security fix would either hold the fix open indefinitely or, worse, produce a false claim that the
lane was repaired.

### 6. Tooling limitation: `--- FAIL:` lines are dropped from every captured evidence block

This is the direct reason the failing stress test could not be named from captured output.
`bubbles_ci_failure_detail` in `.github/bubbles/scripts/guard-lib.sh:386` selects failure-shaped
lines with this expression:

```bash
local _re="^[[:space:]]*(FAIL|ERROR|AssertionError|Traceback|not ok|✗|❌|(${_tools}):)|(${_phrases})"
```

The anchor allows leading **whitespace** only. Go's per-test failure line begins with three hyphens:

```text
--- FAIL: TestSomething (1.23s)
```

`---` is not whitespace, so the line never matches, and every Go per-test failure is filtered out of
the captured block. What survives is only the package-level summary that Go prints flush-left:

```text
FAIL    github.com/smackerel/smackerel/tests/stress     366.733s
```

The consequence is precise and worth naming: a captured Go failure block can tell you **which
package** failed but never **which test**. That is the whole distance between "the stress lane is
red" and "this test is red", and it is exactly the distance this packet could not close.

`guard-lib.sh` is **framework-owned** (`.github/bubbles/`), a downstream-managed install artifact.
It is recorded here and deliberately **not edited** — a local patch would be silently reverted by
the next framework refresh, which is worse than the current, documented gap. The fix belongs
upstream, as an added alternative in the shape-1 branch of the same expression.


---

## Before Fix — Reproduction

### TREE

HEAD `0f4b4826`, working tree clean on every path named below.

### STEP 1 — the model is asked for the identity

```text
$ sed -n '104,113p' internal/agent/tools/retrieval/tool.go
var inputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "user_id"],
  "properties": {
    "query":   {"type": "string", "minLength": 1},
    "user_id": {"type": "string", "minLength": 1},
    "top_k":   {"type": "integer", "minimum": 1, "maximum": 50}
  }
}`)
```

`user_id` is in `required`, so the model MUST produce it on every call.

### STEP 2 — the server checks only that it is non-empty

```text
$ sed -n '178,184p' internal/agent/tools/retrieval/tool.go
        if in.UserID == "" {
                return nil, errors.New("retrieval_search_missing_user_id")
        }
        if in.Query == "" {
                return nil, errors.New("retrieval_search_empty_query")
        }
```

No comparison against an authenticated principal. No grant required.

### STEP 3 — no principal mechanism exists

```text
$ grep -rn 'AuthenticatedPrincipal\|PrincipalFromContext\|WithPrincipal' --include='*.go' internal/
$ echo "exit=$?"
exit=1
```

Exit `1` is the finding: the concept is absent, so there is nothing for the tool to have consulted.

### STEP 4 — the Telegram surface carries no identity into the agent

```text
$ sed -n '84,94p' internal/telegram/agent_bridge.go
func (b *AgentBridge) Handle(ctx context.Context, chatID int64, text string) (*agent.InvocationResult, error) {
        ...
        env := agent.IntentEnvelope{
                Source:   "telegram",
                RawInput: text,
        }
        result, decision := b.Runner.Invoke(ctx, env)
```

`chatID` is never resolved to a user. `IntentEnvelope` (`internal/agent/router.go:40-52`) has no
identity field. On this surface the model's `user_id` is therefore the **only** identity in play —
which is why this is filed S1 rather than S2.

### Reading of the transcript

Steps 1 and 2 establish that the tool trusts an argument. Step 3 establishes that it had no
alternative to trust. Step 4 establishes that on at least one live surface there is no other
identity anywhere in the call. The three together are the defect; none alone would be.

### What this packet does NOT claim

- It makes **no** claim about spec 108's corpus-grant route gate. That is a different boundary,
  mid-rollout behind a ratified OBSERVE window, and nothing here makes it more or less enforced.
- It makes no claim that the defect has been exploited.
- It does not assert the fix size beyond the seven files named in the Change Boundary; that estimate
  is derived from the five `Invoke` call sites plus three tool files, and the implementing agent
  should treat it as a floor rather than a budget.
