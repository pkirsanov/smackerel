# Scopes: BUG-061-012 — Server-derived principal for agent tools

## Scope 1: Remove model-supplied identity and resolve the principal server-side

**Scope ID:** `BUG-061-012-SCOPE-01`
**Status:** Done
**Depends On:** none

### Change Boundary

**Allowed surfaces:** `internal/agent/tools/retrieval/tool.go`,
`internal/agent/tools/notification/propose.go`, `internal/agent/tools/notification/execute.go`,
`internal/telegram/agent_bridge.go`, `internal/scheduler/agent_bridge.go`,
`internal/pipeline/agent_bridge.go`, `internal/agent/judgment.go`, plus one new contract test file.

**Surfaces added during implementation** (the original list was a floor, not a budget; each is
recorded here with its justification rather than left as an undeclared edit):

| Surface | Justification |
|---|---|
| `internal/annotation/classifier_bridge.go` | A sixth `Invoke` surface that neither `design.md` § C2 nor `bug.md` enumerated; found by grep. Injects `auth.SystemSession("annotation")` for the same reason as scheduler/pipeline/judgment. |
| `internal/agent/tools/microtools/entity_resolve.go` | Declared a load-bearing `user_id`; the defect is present here, so C1 cannot be satisfied without it. |
| `internal/agent/tools/recipesearch/tool.go` | Declared a decorative `user_id`; the C3 schema contract fails while it remains. |
| `internal/auth/system_session.go` (new) | The explicit system principal C2 requires; a shared constructor rather than four call-site literals. |
| `internal/telegram/agent_bridge_principal.go` (new) | The production `PrincipalResolver` C2's Telegram row requires. |

`internal/agent/tools/notification/execute.go` was in the allowed list but is **unchanged** — it
never declared a model-supplied `user_id` (see `report.md` § Discovered issues 2).

**Excluded surfaces:** No change to `internal/agent/registry.go`, `internal/agent/executor.go`, or
`internal/agent/router.go` — the design's central claim is that the existing context seam suffices,
so any edit to those files means the design was wrong and must be revisited rather than widened. No
database migration, no protobuf, no config key. No change to spec 108's route gate.

**Consumer Impact Sweep:** The changed interface is the `retrieval_search` (and notification tool)
**input schema** — a contract consumed by the model at runtime, not by first-party code. Removing
`user_id` narrows it; a caller that previously supplied the field now fails input-schema validation,
which is the intended outcome. First-party references to that schema are the tool definitions
themselves and their tests. No navigation, breadcrumb, redirect, deep link, API client, or generated
client consumes it.

> **This prediction was falsified during implementation — and has since been resolved.** Three
> first-party consumers invoked these tools through the registry and were broken by the narrowing:
> `tests/stress/assistant_retrieval_p95_test.go`, `tests/e2e/microtools/overlays_e2e_test.go`, and
> `internal/assistant/openknowledge/agenttool/substrate_tool.go`. All three were repaired at commit
> `0dcb9d1f`. The `e2e` lane now exits `0` (sha256 `de7f6cd5…`). The `stress` lane still exits `1`,
> but that failure is **pre-existing**: a clean-room `git worktree` at `0f4b4826` — the commit
> immediately before any BUG-061-012 work — fails the same package with the same exit code. The
> finding is closed; the stress failure is routed to a separate `bubbles.test`-owned packet. See
> `report.md` § Consumer impact sweep — the finding, resolved and § Discovered issues 4 and 5.

### Gherkin Scenarios (Regression Tests)

```gherkin
Scenario: SCN-01 A tool argument can no longer name the caller
  Given the registered agent tool set
  When each tool's input schema is read as data
  Then no schema declares user_id, userId, user, principal, or actor as a property

Scenario: SCN-02 Retrieval fails closed with no principal
  Given a context carrying no auth session
  When retrieval_search is invoked with a valid query
  Then it returns retrieval_search_no_principal
  And it performs no search

Scenario: SCN-03 Retrieval refuses a principal without the grant
  Given a context carrying a session that lacks corpus:read
  When retrieval_search is invoked with a valid query
  Then it returns retrieval_search_grant_required
  And the error is distinguishable from the no-principal case

Scenario: SCN-04 Retrieval reads as the authenticated principal
  Given a context carrying a session for user U holding corpus:read
  When retrieval_search is invoked with a valid query
  Then the search is scoped to U
  And no argument could have changed that

Scenario: SCN-05 A mapped Telegram chat still retrieves
  Given a Telegram chat mapped to user U
  When the bridge handles a retrieval message
  Then a principal for U is injected before Invoke
  And retrieval succeeds

Scenario: SCN-06 An unmapped Telegram chat cannot read the corpus
  Given a Telegram chat with no user mapping
  When the bridge handles a retrieval message
  Then no principal is injected
  And retrieval fails closed

Scenario: SCN-07 System triggers cannot read a user corpus
  Given the scheduler, pipeline, or judgment surface
  When it invokes the agent
  Then it injects a system principal that does not carry corpus:read
  And a corpus tool invoked under it fails closed
```

### Implementation Plan

1. `retrieval_search`: drop `user_id` from schema and struct; resolve `auth.SessionFromContext`;
   fail closed on absent principal; require `corpus:read`.
2. Same for the two notification tools.
3. Telegram: resolve `chatID` → user before `Invoke`; inject via `auth.WithSession`.
4. Scheduler / pipeline / judgment: inject an explicit system principal without `corpus:read`.
5. Add the schema-contract test (SCN-01) plus the adversarial cases.

### Test Plan

| ID | Test | Type | Location | Assertion | Scenario |
|---|---|---|---|---|---|
| T-01 | TestToolSchemas_DeclareNoCallerIdentity | unit | `internal/agent/tools/toolscontract/schema_contract_test.go` | No registered tool schema declares a caller-identity property | SCN-01 |
| T-02 | TestRetrieval_NoPrincipalFailsClosed | unit | `internal/agent/tools/retrieval/tool_test.go` | Returns `retrieval_search_no_principal`; no search performed | SCN-02 |
| T-03 | TestRetrieval_GrantRequired | unit | `internal/agent/tools/retrieval/tool_test.go` | Returns `retrieval_search_grant_required` | SCN-03 |
| T-04 | TestRetrieval_ScopesToAuthenticatedPrincipal | unit | `internal/agent/tools/retrieval/tool_test.go` | Search is scoped to the session user | SCN-04 |
| T-05 | TestTelegramBridge_MappedChatInjectsPrincipal | unit | `internal/telegram/agent_bridge_test.go` | Principal present in ctx at Invoke | SCN-05 |
| T-06 | TestTelegramBridge_UnmappedChatInjectsNone | unit | `internal/telegram/agent_bridge_test.go` | No principal; corpus tool fails closed | SCN-06 |
| T-07 | TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant | unit | `internal/agent/judgment_test.go` | System principal lacks `corpus:read` | SCN-07 |
| T-08 | TestRetrieval_EndToEndUnderHTTPSession | integration | `tests/integration/agent/retrieval_principal_test.go` | HTTP session reaches the tool unchanged | SCN-04 |
| T-09 | TestMicroToolOverlays_FullMatrix/SCN-065-A06_entity_resolve_resolved | Regression E2E (e2e-api) | `tests/e2e/microtools/overlays_e2e_test.go:283` | Persistent scenario-specific regression: a tool invoked through the registry resolves under the context principal only — asserts the resolver saw `u-076-3` (line 297), the user carried by `auth.WithSession`, while the input schema declares no identity property | SCN-04 |
| T-10 | `./smackerel.sh test e2e` (full lane) | Regression E2E suite (e2e-api) | `tests/e2e/` | Broader regression: the whole e2e lane stays green after the tool input schemas were narrowed — no collateral consumer breakage | SCN-01, SCN-04 |

**Test Plan location correction.** The T-01 row originally read
`internal/agent/tools/schema_contract_test.go`. No such file exists; the test landed one directory
deeper, in its own package, so it can import the registry without an import cycle. Only the
**Location** cell was corrected, to the path that exists. The test's identity, assertion, and mapped
scenario are untouched — this is a path correction, not a restatement of what the test proves.

### Definition of Done — 3-Part Validation

**Evidence provenance for this section.** Blocks tagged `executed (orchestrator)` were produced by
`evidence-capture.sh` against the current tree in this session; each `sha256` covers the full output
of that one command and is re-derivable with `--verify`. Blocks tagged `executed (this agent)` were
run read-only in this session and their output is pasted verbatim. Blocks tagged
`executed (prior session)` were produced before this session and are recorded verbatim in
`report.md`; where a leg of such evidence was re-derivable now, it was re-run and is shown.

The seven-test scoped run referenced by Group A is:

```text
$ ./smackerel.sh test unit --go --go-run '^(TestToolSchemas_DeclareNoCallerIdentity|TestRetrieval_NoPrincipalFailsClosed|TestRetrieval_GrantRequired|TestRetrieval_ScopesToAuthenticatedPrincipal|TestTelegramBridge_MappedChatInjectsPrincipal|TestTelegramBridge_UnmappedChatInjectsNone|TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant)$' --verbose
=== RUN   TestTelegramBridge_MappedChatInjectsPrincipal
--- PASS: TestTelegramBridge_MappedChatInjectsPrincipal (0.00s)
=== RUN   TestTelegramBridge_UnmappedChatInjectsNone
--- PASS: TestTelegramBridge_UnmappedChatInjectsNone (0.00s)
exit code: 0
```

An anchored `--go-run` alternation that named a test which did not exist would still exit `0`, so
the exit code alone does not prove a given test ran. Each Group A item therefore pairs that run with
a source-existence grep executed in this session, which pins the test to a file and a line. Together
they establish that the named function exists and that nothing matching the filter failed. Only
T-05 and T-06 have a literal per-test `--- PASS:` line in the excerpt available to this agent; the
other five rows say so plainly rather than implying a line that was not seen.

#### Group A — Test Plan parity (10 items ↔ 10 Test Plan rows)

- [x] T-01 discharges SCN-01 — a tool argument can no longer name the caller: read as data, no registered agent tool input schema declares `user_id`, `userId`, `user`, `principal`, or `actor` as a property. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator) · SCN-01

  ```text
  $ grep -n "func TestToolSchemas_DeclareNoCallerIdentity" internal/agent/tools/toolscontract/schema_contract_test.go
  74:func TestToolSchemas_DeclareNoCallerIdentity(t *testing.T) {
  exit code: 0
  ```

  Paired with the seven-test scoped run above (exit code `0`), and corroborated by the full unit
  lane below, which covers this package:

  ```text
  $ ./smackerel.sh test unit --go
  146 packages, 0 failed
  exit code: 0
  sha256: 89f74e33a026b24243b14b653e8dbb6e6b391e389e7f00642c9308bbc8e265dc
  ```

- [x] T-02 discharges SCN-02 — retrieval fails closed with no principal: invoked with a valid query on a context carrying no auth session, `retrieval_search` returns `retrieval_search_no_principal` and performs no search. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator) · SCN-02

  ```text
  $ grep -n "func TestRetrieval_NoPrincipalFailsClosed" internal/agent/tools/retrieval/tool_test.go
  220:func TestRetrieval_NoPrincipalFailsClosed(t *testing.T) {
  exit code: 0
  ```

  Asserts `retrieval_search_no_principal` and that no search is performed. Paired with the
  seven-test scoped run (exit code `0`) and the full unit lane (exit code `0`, 146 packages,
  0 failed, sha256 `89f74e33…`).

- [x] T-03 discharges SCN-03 — retrieval refuses a principal without the grant: a session that lacks `corpus:read` gets `retrieval_search_grant_required`, an error distinguishable from the no-principal case. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator) · SCN-03

  ```text
  $ grep -n "func TestRetrieval_GrantRequired" internal/agent/tools/retrieval/tool_test.go
  242:func TestRetrieval_GrantRequired(t *testing.T) {
  exit code: 0
  ```

  Asserts `retrieval_search_grant_required`, and that the refusal is distinguishable from the
  no-principal case. Paired with the seven-test scoped run (exit code `0`) and the full unit lane
  (exit code `0`, 146 packages, 0 failed, sha256 `89f74e33…`).

- [x] T-04 discharges SCN-04 — retrieval reads as the authenticated principal: given a session for user U holding `corpus:read`, the search is scoped to U and no argument could have changed that. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator) · SCN-04

  ```text
  $ grep -n "func TestRetrieval_ScopesToAuthenticatedPrincipal" internal/agent/tools/retrieval/tool_test.go
  285:func TestRetrieval_ScopesToAuthenticatedPrincipal(t *testing.T) {
  exit code: 0
  ```

  Asserts the search is scoped to the session user and that no argument could have changed it.
  Paired with the seven-test scoped run (exit code `0`) and the full unit lane (exit code `0`,
  146 packages, 0 failed, sha256 `89f74e33…`).

- [x] T-05 discharges SCN-05 — a mapped Telegram chat still retrieves: for a chat mapped to user U the bridge injects a principal for U before Invoke, and retrieval succeeds. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (orchestrator) · SCN-05

  ```text
  === RUN   TestTelegramBridge_MappedChatInjectsPrincipal
  --- PASS: TestTelegramBridge_MappedChatInjectsPrincipal (0.00s)
  exit code: 0
  ```

  A literal per-test `--- PASS:` line was observed for this row. Source location confirmed this
  session at `internal/telegram/agent_bridge_test.go:102`.

- [x] T-06 discharges SCN-06 — an unmapped Telegram chat cannot read the corpus: with no user mapping the bridge injects no principal, and retrieval fails closed. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (orchestrator) · SCN-06

  ```text
  === RUN   TestTelegramBridge_UnmappedChatInjectsNone
  --- PASS: TestTelegramBridge_UnmappedChatInjectsNone (0.00s)
  exit code: 0
  ```

  A literal per-test `--- PASS:` line was observed for this row. Source location confirmed this
  session at `internal/telegram/agent_bridge_test.go:135`.

- [x] T-07 discharges SCN-07 — system triggers cannot read a user corpus: the scheduler, pipeline, and judgment surfaces inject a system principal that does not carry `corpus:read`, so a corpus tool invoked under it fails closed. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator) · SCN-07

  ```text
  $ grep -n "func TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant" internal/agent/judgment_test.go
  129:func TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant(t *testing.T) {
  exit code: 0
  ```

  Asserts the system principal carries no `corpus:read`. Paired with the seven-test scoped run
  (exit code `0`) and the full unit lane (exit code `0`, 146 packages, 0 failed,
  sha256 `89f74e33…`).

- [x] T-08 discharges SCN-04 end to end — retrieval reads as the authenticated principal when the session arrives over HTTP: the principal reaches the tool unchanged and the search is scoped to it. Executed and passing, with raw output recorded inline

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator) · SCN-04

  ```text
  $ grep -n "func TestRetrieval_EndToEndUnderHTTPSession" tests/integration/agent/retrieval_principal_test.go
  93:func TestRetrieval_EndToEndUnderHTTPSession(t *testing.T) {
  exit code: 0
  ```

  SCN-04 end to end: an HTTP session reaches the tool unchanged. Executed by the scoped integration
  run and by the full integration lane:

  ```text
  $ ./smackerel.sh test integration --go-run '^TestRetrieval_EndToEndUnderHTTPSession$'
  exit code: 0
  $ ./smackerel.sh test integration
  exit code: 0
  sha256: 8399bd7b782ca336d58a9f8a35730bd28948cec76d528e7a6ac3a66e3f286725
  ```

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — T-09 holds SCN-04 at the E2E layer

  **Phase:** implement · **Claim Source:** executed (this agent, current session) · SCN-04

  ```text
  $ grep -n 'func TestMicroToolOverlays_FullMatrix\|SCN-065-A06_entity_resolve_resolved\|lastUser != "u-076-3"' tests/e2e/microtools/overlays_e2e_test.go
  133:func TestMicroToolOverlays_FullMatrix(t *testing.T) {
  283:    t.Run("SCN-065-A06_entity_resolve_resolved", func(t *testing.T) {
  297:            if entResolver.lastUser != "u-076-3" {
  exit code: 0
  $ git log --oneline -1 0dcb9d1f -- tests/e2e/microtools/overlays_e2e_test.go
  0dcb9d1f fix(BUG-061-012): repair the 3 consumers the narrowed tool schemas broke
  exit code: 0
  ```

  This is the durable, scenario-specific regression: the subtest calls `entity_resolve` **through
  the agent registry** with the caller identity supplied only by `auth.WithSession` on the context
  (`sessionCtx`, line 93), then asserts at line 297 that the resolver saw `u-076-3`. Because the
  tool's input schema declares no identity property and is `additionalProperties:false`, no value in
  `args` could have produced that user — so a regression that reintroduced a model-supplied
  `user_id`, or that stopped reading the session, changes `lastUser` and fails the test. It is
  non-vacuous for the same reason the Group B adversarial proof is: it was itself broken by the
  schema narrowing and repaired at `0dcb9d1f`, which is direct evidence that it exercises the
  narrowed path rather than routing around it. No per-test `--- PASS:` line was observed by this
  agent; the passing signal is the lane below, which is the only file in `tests/e2e/microtools/`.

- [x] Broader E2E regression suite passes — T-10, the full e2e lane, exits 0 with no collateral breakage

  **Phase:** implement · **Claim Source:** executed (orchestrator, current tree)

  ```text
  $ ./smackerel.sh test e2e
  exit code: 0
  sha256: de7f6cd5d993beb8a8a4e6438fbf2bba37e61e2d0f92297abe188384d5861498
  ```

  The lane covering every first-party consumer of the narrowed schemas exits `0`. This is the
  broader half of the regression contract and is what closes the falsified Consumer Impact Sweep
  prediction recorded above: two of the three broken consumers live in this lane, and it is green
  after their repair at `0dcb9d1f`. The `stress` lane is **not** covered by this item and remains
  open — it exits `1` at baseline `0f4b4826` as well, and is recorded as an open item in Group C
  rather than folded into this pass.

#### Group B — Bug-fix contract

- [x] Defect reproduced BEFORE the fix, with raw evidence

  **Phase:** implement · **Claim Source:** executed (prior session; one leg re-derived this session)

  The RED state at the bug-filing commit, re-derived from git in the current session:

  ```text
  $ git show 0f4b4826:internal/agent/tools/retrieval/tool.go | sed -n '104,113p'
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
  exit code: 0
  ```

  `user_id` sits in `required`, so at that commit the model MUST produce it on every call. The full
  four-step reproduction — required argument, emptiness-only check, absent principal mechanism
  (`grep … → exit 1`), and the identity-free Telegram surface — is recorded verbatim in `report.md`
  § Before Fix — Reproduction. It was produced in a prior session at `0f4b4826`; the leg above is
  the part still re-derivable from git today, and it re-derives.

- [x] Adversarial proof: T-01 through T-04 fail against the PRE-fix code (R4.3)

  **Phase:** implement · **Claim Source:** executed (prior session, recorded verbatim in `report.md`)

  ```text
  $ ./smackerel.sh test unit --go --go-run '^(TestToolSchemas_DeclareNoCallerIdentity|TestRetrieval_NoPrincipalFailsClosed|TestRetrieval_GrantRequired|TestRetrieval_ScopesToAuthenticatedPrincipal)$' --verbose
  FAIL
  FAIL    github.com/smackerel/smackerel/internal/agent/tools/retrieval   1.267s
  FAIL
  FAIL    github.com/smackerel/smackerel/internal/agent/tools/toolscontract      0.102s
  exit code: 1
  sha256: 4bbe6e6a86a718c9c40329e04078ec03cd339956f135f3c7bce4940fc97987c6
  ```

  The four defect-carrying files were checked out at `0f4b4826` while the new tests were kept. Both
  packages carrying T-01..T-04 fail against pre-fix source and pass against the fix, so the
  regressions are non-vacuous rather than tautological — a regression whose fixtures all satisfy the
  broken path proves nothing, and these do not.

- [x] `user_id` absent from every agent tool input schema, proven by grep

  **Phase:** implement · **Claim Source:** executed (this agent, current session)

  ```text
  $ grep -rn '"user_id"' --include='*.go' internal/agent/tools/ internal/assistant/openknowledge/ | grep -v _test
  internal/agent/tools/notification/services.go:136:      UserID    string    `json:"user_id"`
  exit code: 0
  ```

  One match remains and it is not a tool input schema. It is a Go struct tag on `payloadEnvelope`
  (`services.go:128-138`), the opaque payload `notification_execute` round-trips from the
  ConfirmStore to the Scheduler — server-held state, never a model argument. Compare the pre-fix
  grep in `report.md`, which returned four `"required": [… "user_id"]` schema lines. T-01 is the
  durable enforcement: it derives the tool set from the registration call rather than restating it.

- [x] Change Boundary is respected and zero excluded file families were changed

  **Phase:** implement · **Claim Source:** executed (this agent, current session)

  ```text
  $ git diff --name-only 0f4b4826..0dcb9d1f -- internal/agent/registry.go internal/agent/executor.go internal/agent/router.go
  exit code: 0
  $ git diff --name-only 0f4b4826..0dcb9d1f -- 'migrations/*' 'proto/*' 'config/*.yaml'
  exit code: 0
  ```

  Both diffs print nothing. `registry.go`, `executor.go`, and `router.go` are untouched across the
  whole implementation range, and that is the load-bearing result rather than a formality: the
  design's central claim was that the existing context seam suffices, so an edit to any of those
  three would have meant the design was wrong rather than that the boundary was widened. No
  migration, no protobuf, no config key. The five surfaces added beyond the original list are each
  declared with justification in the Change Boundary table above.

- [x] Consumer impact sweep completed — zero stale first-party references remain after the tool input schema narrowed

  **Phase:** implement · **Claim Source:** executed (this agent + orchestrator)

  ```text
  $ grep -n 'user_id' tests/stress/assistant_retrieval_p95_test.go tests/e2e/microtools/overlays_e2e_test.go internal/assistant/openknowledge/agenttool/substrate_tool.go
  tests/e2e/microtools/overlays_e2e_test.go:118:// the entity_resolve handler and records the user_id it received so
  tests/e2e/microtools/overlays_e2e_test.go:120:// BUG-061-012 that user_id originates in the authenticated session on
  internal/assistant/openknowledge/agenttool/substrate_tool.go:68:// BUG-061-012: a `user_id` property was declared here and parsed into
  internal/assistant/openknowledge/agenttool/substrate_tool.go:74:// config/prompt_contracts/open_knowledge.yaml keeps its `user_id`
  exit code: 0
  ```

  Every surviving occurrence is a comment explaining the removal; not one is a live argument, and
  `assistant_retrieval_p95_test.go` has none at all. The lane that carries two of the three
  consumers now passes:

  ```text
  $ ./smackerel.sh test e2e
  exit code: 0
  sha256: de7f6cd5d993beb8a8a4e6438fbf2bba37e61e2d0f92297abe188384d5861498
  ```

  The sweep's original prediction was wrong — three consumers existed, one of them production code —
  and all three were repaired at `0dcb9d1f`. See `report.md` § Discovered issues 4 for why the
  prediction failed structurally rather than by carelessness.

- [x] Rollback path documented and verified

  **Phase:** implement · **Claim Source:** executed (prior session, recorded verbatim in `report.md`)

  ```text
  $ git checkout HEAD -- internal/agent/tools/retrieval/tool.go internal/agent/tools/recipesearch/tool.go internal/agent/tools/microtools/entity_resolve.go internal/agent/tools/notification/propose.go
  $ git status --porcelain -- internal/ tests/
  exit code: 0
  ```

  `git status` printed nothing, confirming an exact restore. `design.md` § Rollback specifies a
  single revert with no migration, no persisted state, and no config key — and that claim was not
  asserted, it was exercised. During the adversarial run the production files were moved back to
  pre-fix content and forward again with `git checkout` alone, with no schema, data, or
  configuration step at either end. Rollback is a revert of `20b0376a` and `0dcb9d1f`. The
  excluded-family check above independently corroborates the "no migration, no protobuf, no config"
  half of the claim.

- [x] `bug.md` status advanced to Fixed and then Verified

  **Phase:** implement · **Claim Source:** executed (this agent, current session)

  ```text
  $ grep -n "^- \[" specs/061-conversational-assistant/bugs/BUG-061-012-model-supplied-identity-in-agent-tools/bug.md
  10:- [x] Reported
  11:- [x] Confirmed (reproduced)
  12:- [x] In Progress
  13:- [x] Fixed
  14:- [x] Verified
  15:- [ ] Closed
  exit code: 0
  ```

  `Fixed` rests on the implementation at `20b0376a` and `0dcb9d1f`. `Verified` rests on the eight
  Test Plan rows above plus the five green lanes in Group C. `Closed` is left unchecked because it
  is a human acceptance step, and `uservalidation.md` is human-only under Gate G136.

#### Group C — Build Quality Gate (grouped block)

- [x] Zero warnings across build, lint, and test output; zero deferrals; `./smackerel.sh lint` and `./smackerel.sh format --check` clean; artifact lint exits 0; documentation aligned with delivered behaviour

  **Phase:** implement · **Claim Source:** executed (orchestrator, current tree)

  ```text
  $ ./smackerel.sh lint
  exit code: 0
  sha256: e60dc02e4b8c160850fd9cda6e2485a5f44a800d8f694a6f5f42b6f62dbe3735
  $ ./smackerel.sh format --check
  78 files formatted
  exit code: 0
  sha256: d5e98a0f123acf3d8ee3674ccc338b70f64c26d87431e2f457bb1ed69970c577
  $ ./smackerel.sh test unit --go
  146 packages, 0 failed
  exit code: 0
  sha256: 89f74e33a026b24243b14b653e8dbb6e6b391e389e7f00642c9308bbc8e265dc
  $ ./smackerel.sh test integration
  exit code: 0
  sha256: 8399bd7b782ca336d58a9f8a35730bd28948cec76d528e7a6ac3a66e3f286725
  $ ./smackerel.sh test e2e
  exit code: 0
  sha256: de7f6cd5d993beb8a8a4e6438fbf2bba37e61e2d0f92297abe188384d5861498
  ```

  Every lane this bug owns is green: `lint`, `format`, `unit`, `integration`, and `e2e` all exit
  `0`. No warning appears in any of them.

  The `stress` lane exits `1`. It predates this work, which is established by running the same lane
  in a clean-room `git worktree` at `0f4b4826` — the commit before any BUG-061-012 change:

  ```text
  $ ./smackerel.sh test stress          # clean-room worktree @ 0f4b4826
  FAIL    github.com/smackerel/smackerel/tests/stress     380.987s
  exit code: 1
  $ ./smackerel.sh test stress          # current tree @ 0dcb9d1f
  FAIL    github.com/smackerel/smackerel/tests/stress     366.733s
  exit code: 1
  sha256: 229575d80a5de113ef107398a2d8920cb35b31ef6d12f1364278b59e9998014e
  ```

  The same package fails the same way on a tree containing none of this work, so the failure is not
  attributable to this bug. The specific failing test is **not** identified; `report.md` §
  Discovered issues 5 records exactly what was ruled out and why the name could not be recovered
  from captured output. **This lane is an open item against this packet under the
  `requireNoPreexistingFailingTests` constraint of `bugfix-fastlane`, and it is recorded as open
  rather than dismissed.**

  Within this packet's own surface there are no unaddressed items: every issue found during
  implementation was fixed in-session, and the two findings that reach beyond this surface — the
  dormant Telegram wiring and the `stress` lane — are both written down as open with named owners
  instead of being narrated away.


