# Scopes: BUG-061-012 — Server-derived principal for agent tools

## Scope 1: Remove model-supplied identity and resolve the principal server-side

**Scope ID:** `BUG-061-012-SCOPE-01`
**Status:** Not Started
**Depends On:** none

### Change Boundary

**Allowed surfaces:** `internal/agent/tools/retrieval/tool.go`,
`internal/agent/tools/notification/propose.go`, `internal/agent/tools/notification/execute.go`,
`internal/telegram/agent_bridge.go`, `internal/scheduler/agent_bridge.go`,
`internal/pipeline/agent_bridge.go`, `internal/agent/judgment.go`, plus one new contract test file.

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
| T-01 | TestToolSchemas_DeclareNoCallerIdentity | unit | `internal/agent/tools/schema_contract_test.go` | No registered tool schema declares a caller-identity property | SCN-01 |
| T-02 | TestRetrieval_NoPrincipalFailsClosed | unit | `internal/agent/tools/retrieval/tool_test.go` | Returns `retrieval_search_no_principal`; no search performed | SCN-02 |
| T-03 | TestRetrieval_GrantRequired | unit | `internal/agent/tools/retrieval/tool_test.go` | Returns `retrieval_search_grant_required` | SCN-03 |
| T-04 | TestRetrieval_ScopesToAuthenticatedPrincipal | unit | `internal/agent/tools/retrieval/tool_test.go` | Search is scoped to the session user | SCN-04 |
| T-05 | TestTelegramBridge_MappedChatInjectsPrincipal | unit | `internal/telegram/agent_bridge_test.go` | Principal present in ctx at Invoke | SCN-05 |
| T-06 | TestTelegramBridge_UnmappedChatInjectsNone | unit | `internal/telegram/agent_bridge_test.go` | No principal; corpus tool fails closed | SCN-06 |
| T-07 | TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant | unit | `internal/agent/judgment_test.go` | System principal lacks `corpus:read` | SCN-07 |
| T-08 | TestRetrieval_EndToEndUnderHTTPSession | integration | `tests/integration/agent/retrieval_principal_test.go` | HTTP session reaches the tool unchanged | SCN-04 |

### Definition of Done — 3-Part Validation

#### Group A — Test Plan parity (8 items ↔ 8 Test Plan rows)

- [ ] T-01 executed and passing, with raw output recorded inline
- [ ] T-02 executed and passing, with raw output recorded inline
- [ ] T-03 executed and passing, with raw output recorded inline
- [ ] T-04 executed and passing, with raw output recorded inline
- [ ] T-05 executed and passing, with raw output recorded inline
- [ ] T-06 executed and passing, with raw output recorded inline
- [ ] T-07 executed and passing, with raw output recorded inline
- [ ] T-08 executed and passing, with raw output recorded inline

#### Group B — Bug-fix contract

- [ ] Defect reproduced BEFORE the fix, with raw evidence
- [ ] Adversarial proof: T-01 through T-04 fail against the PRE-fix code (R4.3)
- [ ] `user_id` absent from every agent tool input schema, proven by grep
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Consumer impact sweep completed — zero stale first-party references remain after the tool input schema narrowed
- [ ] Rollback path documented and verified
- [ ] `bug.md` status advanced to Fixed and then Verified

#### Group C — Build Quality Gate (grouped block)

- [ ] Zero warnings across build, lint, and test output; zero deferrals; `./smackerel.sh lint` and `./smackerel.sh format --check` clean; artifact lint exits 0; documentation aligned with delivered behaviour
