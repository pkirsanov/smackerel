# Scopes: 109 MCP Knowledge Server

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Planning only — no source edited.**
**Sources:** [`spec.md`](spec.md) · [`design.md`](design.md) · **Release train:** `next`
**Flag introduced:** `mcpKnowledgeServer`

---

## Execution Outline

### Phase Order

1. **Scope 01 — MCP Foundation.** `internal/mcp/{capability,authz,projection,refusal,audit,transport}` (design §2), the audience-bound credential that closes F-109-004, the independent authorizer with the fixed five-gate order (design §5), the structurally-closed projection type (design §7), the seven-token closed vocabulary and four-field envelope (spec §9A.1/§9A.2), the append-only MCP ledger and the `smackerel_mcp_*` metric family (design §9.1), and the pull-only `/mcp` mount. Blocks every other scope: no toolset can be authorized, projected, refused, or audited until the foundation exists.
2. **Scope 02 — `context` toolset + discovery.** The first three real, service-backed tools, the D5 Tool Inventory as the sole source of exported wire names, plus readiness-filtered, deterministically-ordered, grant-varied `tools/list` (design §5). Proves the registry is explicit and the 19 `agent.All()` non-capabilities never appear.
3. **Scope 03 — `memory-read` over the spec-095 Executor.** The `routing.Executor` seam (design §6), the `corpus` data-scope grant that resolves UX-F-001 (design §4), the SQL source-level egress filter boundary, egress-class enforcement gated **before** the read, honest degradation, and the §9B `resource_link` surface with its opaque-URI constraint. Carries the heaviest adversarial test load.
4. **Scope 04 — `person-context` toolset.** Opt-in grant, no data scope. Exercises the foundation's opt-in path independently of the corpus.
5. **Scope 05 — `graph-read` toolset.** Opt-in grant plus the scope-minimized step-up challenge (Case C), which only becomes observable once two opt-in toolsets coexist.
6. **Scope 06 — `hospitality-read` toolset. BLOCKED.** Registered with `Readiness.Available=false`; specified and testable as *honestly unavailable*, deliverable only after BUG-019-003 persists `artifacts.source_ref`.
7. **Scope 07 — Documentation, Release, and Configuration.** Every `spec.md` §15 row, the `next` train targeting of §17, both feature-flag bundles, the release packet and its `features.md`, the capability ledger surface, and the `docs/Product-Principles.md` alignment note including the §16 honest-discrepancy items.

### New Types & Signatures

```go
// internal/mcp/capability — Scope 01 (design §3). The explicit export manifest.
type ToolID string; type ToolsetID string; type Scope string; type DataScope string
type EgressClass string // "local-inference" | "remote-inference"
type Annotations struct{ ReadOnly, Destructive, Idempotent, OpenWorld bool }
type Readiness   struct{ Available bool; Reason string } // Reason non-empty iff !Available
type Descriptor  struct {
    ID ToolID; Toolset ToolsetID; Input, Output Schema
    RequiredScopes []Scope; RequiredData []DataScope; Egress EgressClass
    Annotations Annotations; Provenance ProvenanceReq
    Readiness func(context.Context) Readiness; Timeout time.Duration
    Handler func(context.Context, Invocation) (Result, error)
}
func Manifest() []Descriptor // slice, never a map — Go randomizes map iteration

// internal/mcp/authz — Scope 01 (design §5). Independent of bearerAuthMiddleware.
func Verify(*http.Request) (Credential, error)          // positive aud equality, not a negative filter
func List(cred Credential, now time.Time) []Descriptor  // pure in (manifest, cred, readiness)

// internal/mcp/projection — Scope 01 (design §7). Closed; cannot express raw content.
type Artifact struct{ /* exactly the §9 returned fields; no map/any/RawMessage/embed */ }
func FromRetrieved(r routing.RetrievedArtifact, sel routing.StrategySelection) Artifact

// internal/mcp/projection — Scope 03 (spec §9B). The ONLY smackerel:// URI constructor.
type OpaqueID string    // newtype over the internal surrogate key — a name/email is a compile error
type ResourceKind int   // closed enum: exactly the five R-109-RES2 templates
func ResourceURI(kind ResourceKind, id OpaqueID) string

// internal/auth — Scope 01 (F-109-004). Wire the existing, unwired Audience type.
//   IssueToken currently sets iss/sub/jti/iat/nbf/exp/kid + optional scope and NO aud.

// internal/mcp — Scope 03 (design §4). A third Grant kind, not a second grant system.
//   GrantKind ∈ { toolset, egress-class, data-scope };  DataScope ∈ { corpus }
//   effective = tools(toolsets) ∩ tools(scope) ∩ tools(data-scopes) ∩ tools(egress)
```

Config key: `mcp.knowledge_server` (SST, **no default**) → `SMACKEREL_MCP_KNOWLEDGE_SERVER`
(generated env, `${VAR:?…}` form). `remote-inference` is deliberately **not** a flag — it is a
per-client registry grant, so no build can enable it globally (spec §15).

Audit ledger columns are closed at eight: `ts`, `client_id`, `toolset`, `tool`, `outcome`,
`egress_class`, `denial_reason`, `trace_token`. `outcome` and `denial_reason` draw only from the
seven-token set: `authorized`, `unauthorized-toolset`, `unauthorized-egress-class`,
`capability-unavailable`, `degraded-fallback`, `refused`, `execution-failed`.

### Validation Checkpoints

| After | Gate that catches breakage before the next scope starts |
|---|---|
| Scope 01 | A real `IssueToken` legacy bearer is **rejected** at `/mcp` (TP-01-02). A negative-form `aud != …` check admits it, so this fails against the implementation a developer writes first. If it passes prematurely, every later authorization assertion is meaningless. |
| Scope 02 | `tools/list` is byte-identical across repeated calls at a pinned readiness state with a manifest large enough that randomized map iteration would reorder it (TP-02-02), and an ungranted name is byte-identical to a never-existing name (TP-02-04). Served names equal the `spec.md` D5 Tool Inventory by two-way set equality and are served in the inventory row order (TP-02-07), so the public wire contract is pinned before any corpus tool exists. No-existence-oracle is proven before any corpus tool exists. |
| Scope 03 | A spy executor records **zero** `Retrieve` calls on an ungranted-egress denial (TP-03-05) — proving gate 4 precedes dispatch rather than post-filtering. Byte-level `content_raw` scan is clean and `smackerel_mcp_projection_raw_content_blocked_total == 0` (TP-03-08). Every emitted `resource_link` URI matches an R-109-RES2 template and carries no PII (TP-03-10) — the envelope leak a payload-only scan cannot see. |
| Scope 04 | An opt-in toolset with **no** data scope authorizes correctly, isolating the §4 corpus term from the toolset term. |
| Scope 05 | The step-up challenge names exactly one scope; a second opt-in toolset now exists, so a catalog leak would be observable (TP-05-03). |
| Scope 06 | Grant is recorded, tools are omitted, invocation is method-not-found, operator surface reports `capability-unavailable` with the BUG-019-003 reason, and **no** `external_ref` is emitted. Proves "never present-but-fake". |
| Scope 07 | Flag-bundle parity: `mcpKnowledgeServer` default-ON in exactly one train (`next`) and default-OFF in every other; SST key has no fallback default. |

### Planning Notes — Findings Carried Forward (routed, not resolved here)

- **F-109-TOOLNAME — CLOSED by `spec.md` D5 Tool Inventory.** The packet previously named
  **toolsets** but never named the individual tools a client sees in `tools/list`; only the five
  `smackerel_mcp_*` Prometheus metrics existed as concrete names. A tool name is a public wire
  contract — clients pin it in config — so it is now planned: D5 carries the normative inventory
  (nine names, four annotation columns, required grants) plus T1 (charset/length/uniqueness), T2
  (row order **is** the `tools/list` ordering contract; may vary per credential, never
  per-connection), T3 (annotations are advisory; server-side authorization enforces) and T4
  (deferred/off toolsets export no names). Planned in **Scope 02** (TP-02-07). `spec.md` §8.2 gained
  `context.get_server_context` so the toolset table and the wire table are two-way consistent.
- **F-109-RESOURCES — CLOSED by `spec.md` §9B.** MCP Resources were neither specified nor
  excluded: D4a deletes resource **subscriptions**, which says nothing about Resources as a read
  primitive, and the packet had zero occurrences of `resource_link`, `smackerel://`, or
  `resources/list`. Silence was the defect. Ratified split: `resource_link` entries and the five
  R-109-RES2 URI templates are **in** scope (a reference is categorically not `content_raw`, so
  this reinforces D2); an enumerable `resources/list` and any whole-graph resource are **permanently
  excluded** (§13) because an enumerable list is a corpus-enumeration oracle that leaks existence and
  volume even when every read is denied. Planned in **Scope 03** (TP-03-10), with R-109-RES3's
  no-PII-in-a-URI constraint enforced by an `OpaqueID` newtype rather than by review.

- **UX-F-001 — RESOLVED by `design.md` §4.** MCP owns its own corpus-read grant, carried in its own
  credential, as a third `Grant` **kind** (`data-scope`), not a second grant system. Planned in
  **Scope 03**. A corpus denial emits `unauthorized-toolset` to the client (Case A, byte-identical to
  nonexistent) and carries the corpus granularity in the ledger's `denial_reason` column only.
- **UX-F-002 — OPEN.** `docs/smackerel.md` §17.1 (confidence signals) is not in the §15 documentation
  table, yet §9A.5 ties R-109-UX15–UX18 to it. Scope 07 plans the §17.1 row **as a planning decision
  recorded here**, not as a silent spec amendment; `spec.md` §15 is not edited by this packet (G073).
- **UX-F-003 — PARTIALLY RESOLVED (2026-07-29).** Principle numbering. Trust Through Transparency is
  **Principle 8** in `docs/Product-Principles.md`, and there was no Principle 11 — both statements of
  fact, still true. The *principle-gap* half is now **settled by ratified `spec.md` §18 item 7**: the
  document **gains** `Principle 11 — Your Data Stays Yours` at delivery, under **owner sign-off**,
  together with a matching block in its BLOCKING companion enforcement file. Scope 07 carries both as
  named DoD items. This packet still invents nothing and applies nothing.
- **UX-F-004 — OPEN.** §2's Success Signal omits `retrieval_reason` and `retrieval_contract_known`,
  both of which §9 returns and R-109-UX15/UX18 depend on. Scope 03 plans and tests **all six**
  provenance fields. The §2 editorial correction was **not** decided by the §18 gate (which closed
  2026-07-29 on the seven product decisions only) and is re-routed to `bubbles.analyst`.
- **UX-F-005 — OPEN.** D1 calls `remote-inference` "fully coded but default-OFF" while §1 states no
  MCP server exists. Read literally these conflict; the intended meaning is "fully **specified**".
  Scope 03 plans it as fully specified and default-OFF. Ratified §18 item 1 accepted D1's *posture*
  but did **not** correct D1's *prose*, so this stays OPEN and is re-routed to `bubbles.analyst`.
- **F-109-OF2-AMEND — OPEN (new, surfaced by `design.md` §4).** OF-2 now needs a `corpus` data-scope
  grant/revoke step alongside its toolset step, and OF-2 step 4's "operator effective list ==
  client `tools/list`" invariant holds **only if** the operator-side computation runs the same
  four-term intersection. Scope 03 and Scope 07 plan to the amended behavior; amending `spec.md`
  §9A.3 is owned by `bubbles.analyst` under a mode that permits spec edits.

---

## Scope Table

| # | Scope | Surfaces | Depends On | Tests | Status |
|---|---|---|---|---|---|
| 01 | MCP Foundation | `internal/mcp/*`, `internal/auth`, `internal/metrics`, `cmd/core`, `config/` | — | 8 (5 unit, 1 integration, 2 e2e-api) | Not Started |
| 02 | `context` Toolset + Discovery | `internal/mcp/capability`, `internal/mcp/authz` | 01 | 7 (4 unit, 2 integration, 1 e2e-api) | Not Started |
| 03 | `memory-read` + Projection Enforcement | `internal/mcp/*`, `internal/retrieval/routing`, search domain service | 01, 02 | 10 (2 unit, 5 integration, 3 e2e-api) | Not Started |
| 04 | `person-context` Toolset | `internal/mcp/capability`, person/entity service | 01, 02 | 5 (2 unit, 1 integration, 2 e2e-api) | Not Started |
| 05 | `graph-read` Toolset + Step-Up Challenge | `internal/mcp/capability`, `internal/mcp/authz`, graph service | 01, 02, 04 | 5 (2 unit, 1 integration, 2 e2e-api) | Not Started |
| 06 | `hospitality-read` Toolset (**BLOCKED**) | `internal/mcp/capability`, hospitality projection | 01, 02 | 4 (2 unit, 1 integration, 1 e2e-api) | **Blocked** |
| 07 | Docs, Release Packet, Config, Flag Bundles | `docs/`, `docs/releases/`, `config/` | 03, 04, 05 | 6 (3 unit, 1 integration, 2 e2e-api) | Not Started |

Total Test Plan rows: **45** — each with a matching DoD item. Plus **14 standing regression DoD
items** (two per scope: scenario-specific regression coverage and the broader regression suite),
for **59 test DoD items** in total. Every scope carries a persistent scenario-specific
**Regression E2E** row (`TP-01-08`, `TP-02-06`, `TP-03-09`, `TP-04-05`, `TP-05-05`, `TP-06-04`,
`TP-07-06`) so each behavior this feature introduces stays protected after the scope closes.

Canonical commands: `./smackerel.sh test unit` · `./smackerel.sh test integration` · `./smackerel.sh test e2e`

**Deferred, deliberately unplanned here.** SCN-109-013 and SCN-109-014 (write-plane
propose/confirm/expire) belong to the `memory-write` toolset, which D5 sets **off (later)**.
`design.md` §8 settles the `confirm.Machine` mapping now so the deferral does not become a
redesign, but no scope in this packet delivers it. Planning it here would ship an off-by-decision
toolset. Recorded in `scenario-manifest.json` with `plannedIn: null` and status `deferred`.

---

## Scope 01: MCP Foundation — Credential, Authorizer, Projection, Refusal, Audit, Transport

**Status:** Not Started
**Depends On:** — (root scope; blocks 02, 03, 04, 05, 06, 07)
**Foundation:** `foundation: true` — this is the capability foundation (`design.md` § Capability Foundation)
**Resolves:** F-109-004 (missing `aud` claim — conformance blocker)
**Surfaces:** `internal/mcp/{capability,authz,projection,refusal,audit,transport}`, `internal/auth`, `internal/metrics`, `cmd/core`, `config/`

### Use Cases (Gherkin)

**SCN-109-002 — A legacy Smackerel bearer is rejected at `/mcp`**

```gherkin
Given a valid Smackerel bearer token issued for the first-party API
When that token is presented to the /mcp endpoint
Then the request is rejected as not issued for the MCP audience
And the response carries a WWW-Authenticate challenge
And no tool is executed
```

**SCN-109-011 — Sessions are never used for authentication**

```gherkin
Given a client completes initialize successfully
When a subsequent request omits its credential
Then the request is rejected
And no prior session state is used to authorize it
```

**SCN-109-008 — Provider failure is surfaced as a failure**

```gherkin
Given a domain call returns a non-OK agent.Outcome
When the MCP client receives the tool result
Then the result has isError set to true
And the outcome-appropriate cause is preserved in the message
And the result is not shaped like a successful retrieval
```

**SCN-109-009 — Protocol errors and tool errors are distinguished**

```gherkin
Given a client calls a tool that does not exist
Then the server returns a JSON-RPC error
Given a client calls an existing tool whose execution fails
Then the server returns a result with isError true, not a JSON-RPC error
```

**SCN-109-015 — MCP never pushes**

```gherkin
Given an MCP client is connected
When the corpus changes, a digest is generated, or a topic is promoted
Then the server sends no notification, no subscription event, and no tools/list_changed
And the client observes changes only by calling a tool
```

### Implementation Plan

- **Package tree.** Create `internal/mcp/` with exactly the six sub-packages of `design.md` §2, one
  responsibility each. `internal/mcp` imports domain services and `agent.Outcome`; **nothing** in
  `internal/agent` or `internal/assistant` imports `internal/mcp`, so no assistant change can alter
  the MCP export surface. Do **not** extend, wrap, or pass through `internal/agent` (F-109-005) or
  `internal/assistant/openknowledge`.
- **Credential (F-109-004).** Wire the existing, unwired `Audience` type in
  `internal/auth/request_authenticator.go` into `internal/auth/issue.go`'s `IssueToken`. Verify with a
  **positive equality** (`cred.Audience == mcpAudience`), never a negative filter — a token from
  today's issuance path carries **no** `aud` at all and satisfies every negative filter.
- **Authorizer (D3).** `authz.Verify` is wired only into the `/mcp` mount. It never calls
  `bearerAuthMiddleware`, never reads a session, never consults connection state. Gate order is fixed
  and is itself a security property: (1) audience+signature → (2) toolset ∩ credential scope ∩ data
  scope → (3) readiness → (4) egress class → (5) dispatch. Gates 2 and 3 return **no envelope and no
  challenge**; only gate 4 may emit the §9A.2 envelope, scope-minimized.
- **Projection (D2, design §7).** `projection.Artifact` is closed: no `map[string]any`, no
  `json.RawMessage`, no `any`, no `Extra`, no `json:",inline"`, no embedded domain struct, no
  `external_ref`, no `content_raw`. The constructor reads **named** fields and requires a
  `routing.StrategySelection`, which is what makes provenance non-omittable rather than reviewed.
  Add the marshal-time guard incrementing `smackerel_mcp_projection_raw_content_blocked_total` as a
  **detector**, explicitly not as the control.
- **Refusal.** The seven-token closed vocabulary and the fixed four-field envelope
  (`observed:`/`required:`/`remediation:`/`reference:`, exactly that order, one line each). Mirror the
  existing `internal/assistant/contracts/refusal_test.go` precedent: a mechanical scan that fails on a
  banned string. `reference:` may point only at `docs/Operations.md`, `docs/API.md`,
  `docs/Architecture.md`, `docs/smackerel.md` — never at any `specs/` path (R-109-UX5).
- **Audit.** Its own append-only table with the eight closed columns — **not** `agent_traces`, which
  represents band/scenario/LLM invocations with a different outcome enum and would turn OF-4's
  one-filter question into a cross-semantic join. Every disposition other than `authorized` writes a
  row (R-109-UX23). Extend the `smackerel_*` namespace with the five `smackerel_mcp_*` families; do
  not fork the metrics registry.
- **Transport.** JSON-RPC over stateless Streamable HTTP mounted at `/mcp` on the **existing**
  `smackerel-core` listener — no new port, no new service, no new container. Implement `initialize`,
  `tools/list`, `tools/call`, `elicitation/create`, and the §12 protocol-error-vs-tool-error split
  including the `OutcomeInputSchemaViolation` → JSON-RPC-error carve-out. Register **no** notification
  or subscription capability during `initialize` (D4, P-PULL).
- **Not an assistant transport.** Do not register MCP as a fourth `TransportAdapter` (§13 non-goal):
  that inherits the band-LOW `canonicalizeSuccessfulCaptureResponse` path and makes the R-109-UX21
  forbidden string reachable **by construction**.
- **Config.** `mcp.knowledge_server` in the SST with **no default**, generated to
  `SMACKEREL_MCP_KNOWLEDGE_SERVER` in `${VAR:?…}` fail-loud form. No `${VAR:-…}`, no
  `os.Getenv`-with-default (`smackerel-no-defaults.instructions.md`).

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-01-01 | unit | `internal/auth` issuance test | `IssueToken` emits an `aud` claim naming the MCP server; the previously-unwired `Audience` type round-trips through issuance and verification (F-109-004) | `./smackerel.sh test unit` |
| TP-01-02 | unit | `internal/mcp/authz` audience test (**T1**) | A fixture token from the **real** `IssueToken` path — which sets no `aud` — is **rejected** at `/mcp`. Adversarial: a negative-form `aud != "legacy"` check admits it; only positive equality rejects it (SCN-109-002) | `./smackerel.sh test unit` |
| TP-01-03 | unit | `internal/mcp/authz` session test | A request omitting its credential after a successful `initialize` is rejected; no session, connection struct, or `initialize`-time snapshot is consulted (SCN-109-011) | `./smackerel.sh test unit` |
| TP-01-04 | unit | `internal/mcp/projection` reflection test (**T4**) | Reflect over `projection.Artifact`: field set equals §9's closed list by **set equality in both directions**; no field is `map[string]any`/`json.RawMessage`/`any`/an embedded struct; no `content_raw`; no `external_ref`. Adversarial: fails when a field is *added*, which no sample-response test detects | `./smackerel.sh test unit` |
| TP-01-05 | unit | `internal/mcp/refusal` vocabulary test (**T10**) | Table across the full `agent.Outcome` map including the `OutcomeInputSchemaViolation` → JSON-RPC-error carve-out; plus a mechanical scan asserting `"saved as an idea"` and every §9A.1 banned word appear on **no** MCP surface (SCN-109-008, SCN-109-009) | `./smackerel.sh test unit` |
| TP-01-06 | integration | `internal/mcp/audit` against the ephemeral test stack | Every non-`authorized` disposition writes exactly one append-only row carrying a token from the closed seven; `egress_class` is a separate column never folded into `outcome`; no row lands in `agent_traces` (R-109-UX23) | `./smackerel.sh test integration` |
| TP-01-07 | e2e-api | `/mcp` transport test on the ephemeral stack | A full `initialize` → `tools/call` session against the real listener emits **zero** server-initiated messages: no notification, no subscription event, | TP-01-08 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-109-002, SCN-109-008, SCN-109-009, SCN-109-011 and SCN-109-015 against the live stack: a no-`aud` bearer is still rejected, a credential-less post-`initialize` request is still rejected, the refusal vocabulary still contains only the closed seven with no banned word, and a full session still emits zero server-initiated messages. Fails if audience checking is loosened to a negative filter, if session state is ever consulted for authorization, or if a push/notification capability is advertised; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] `internal/mcp/` exists with exactly the six sub-packages of `design.md` §2; no import edge from `internal/agent` or `internal/assistant` into `internal/mcp`
- [ ] `/mcp` is mounted on the existing `smackerel-core` listener — no new port, no new service, no new container; MCP is **not** registered as an assistant `TransportAdapter`
- [ ] `TP-01-01` unit test passes — `IssueToken` emits the MCP `aud` claim (F-109-004 closed)
- [ ] `TP-01-02` unit test passes — legacy no-`aud` bearer rejected by positive equality, not a negative filter
- [ ] `TP-01-03` unit test passes — no session, connection state, or `initialize` snapshot authorizes a request
- [ ] `TP-01-04` unit test passes — `projection.Artifact` field set is closed by two-way set equality; no open field, no `content_raw`, no `external_ref`
- [ ] `TP-01-05` unit test passes — `agent.Outcome` mapping table complete and the banned-string scan is clean
- [ ] `TP-01-06` integration test passes — append-only ledger rows use only the closed seven tokens; `agent_traces` untouched
- [ ] `TP-01-07` e2e-api test passes — zero server-initiated messages across a full session
- [ ] `mcp.knowledge_server` resolves fail-loud with **no** default; absent/malformed value aborts startup
- [ ] `TP-01-08` regression e2e-api test passes — audience binding, per-request authorization, the closed refusal vocabulary, and the no-push transport are permanently protected
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-01-08`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default/fallback introduced

---

## Scope 02: `context` Toolset + Readiness-Filtered Deterministic Discovery

**Status:** Not Started
**Depends On:** Scope 01 (the capability foundation)
**Resolves:** F-109-005 (the general agent registry is not an MCP source of truth)
**Surfaces:** `internal/mcp/capability`, `internal/mcp/authz`, synthesis / lifecycle services

### Use Cases (Gherkin)

**SCN-109-003 — `tools/list` reflects the presented credential**

```gherkin
Given client A holds a credential granting only "context"
And client B holds a credential granting "context" and "memory-read"
When each calls tools/list against the same running server
Then client A sees only context tools
And client B additionally sees memory-read tools
And each list is byte-identical across repeated calls with the same credential
```

*Scope-02 slice:* client A's `context`-only list and its byte-identity are proven here. The
client-B `memory-read` half is proven in Scope 03 (TP-03-02), because `memory-read` does not exist
until then. Planning the whole scenario in Scope 02 would require asserting against an unregistered
toolset.

**SCN-109-004 — No fabricated capability is advertised**

```gherkin
Given the general agent registry contains 7 noop_* tools
And internal/recommendation/tools/register.go registers 12 tools returning {"ok":true}
When any MCP client calls tools/list
Then none of those 19 registrations appear in the result
And every advertised tool is backed by a domain service
```

**SCN-109-P04 — Every served tool name matches the D5 Tool Inventory (traces: `spec.md` D5 Tool Inventory, T1, T2)**

```gherkin
Given spec.md D5 defines the Tool Inventory as the complete set of exportable tool names
When a client whose credential grants every toolset calls tools/list
Then every tool name in the response appears in the Tool Inventory
And every Tool Inventory name whose readiness is available appears in the response
And each name is 1 to 128 characters drawn only from A-Za-z0-9_-. and is unique within the server
And the response order equals the Tool Inventory row order
```

### Implementation Plan

- Register `context.get_daily_brief`, `context.get_active_topics`, and
  `context.get_server_context` as explicit `Descriptor` rows under the D5 wire names
  `smackerel_daily_brief`, `smackerel_active_topics`, and `smackerel_server_context`. The
  first two are backed by the synthesis / lifecycle domain services; `get_server_context`
  is backed by the capability registry and the authorizer and returns real per-credential
  state (granted toolsets, per-tool readiness, server identity, negotiated protocol
  version), never a constant. Default-on, `local-inference`, **no** data scope.
- **Wire names are registered, never derived.** `Descriptor.ID` carries the D5 wire name as
  a literal on the manifest slice. Manifest registration refuses a duplicate name, a name
  outside T1's `A-Za-z0-9_-.` / 1–128-character rule, and any name **absent from the D5
  inventory** — so a tool name cannot enter the wire surface by writing a handler, only by
  amending `spec.md`. No name is derived from `agent.All()`, a struct-field name, or a map
  key. A name is **never renamed**: a rename breaks every client config that pinned it, so
  it is a removal plus an addition, not an edit.
- Declare all four annotation hints **explicitly**. `get_daily_brief` is `idempotent=true`; a
  `SideEffectClass` derivation cannot produce this, because `memory.search` shares its `read` value
  yet is `idempotent=false` across time (`lifecycle_state` moves — Product Principle 3).
- **`List` is a pure function of `(manifest, credential, readiness)`.** The per-request credential is
  the only authorization input: no session, no connection struct, no per-connection cache, no
  `initialize`-time snapshot. This is also what makes revocation effective on the next request with
  no reconnect and no server push (OF-2 step 3, OF-5 step 2, P-PULL).
- **Determinism.** The manifest is a **slice**, never a map — a `range` over a map is the natural
  implementation and the natural bug. Sort by `(toolset ordinal, ToolID)` with a total order and
  serialize from structs with fixed field order.
- **Honest readiness caveat.** `Readiness` is a function of *server* state, not of the credential, so
  the invariant is byte-identity across repeated calls **at a given readiness state**. Tests pin
  readiness explicitly rather than asserting absolute immutability, which would be an overclaim.
- **No-existence-oracle.** An ungranted or unregistered tool name returns JSON-RPC method-not-found
  **byte-identical** to a name that has never existed. No envelope, no `required` scope, no
  "this tool exists but you lack X" (R-109-UX11, R-109-UX12).

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-02-01 | unit | `internal/mcp/capability` manifest test | Both `context.*` descriptors resolve to a real domain-service handler; all four annotation hints are set explicitly and are not derived from `agent.SideEffectClass` | `./smackerel.sh test unit` |
| TP-02-02 | unit | `internal/mcp/authz` determinism test (**T2**) | Repeated `tools/list` calls with the same credential across distinct connections return **byte-identical** JSON. Adversarial fixture: a manifest large enough that Go's randomized map iteration reorders it, so any `range`-over-map implementation fails. Readiness is pinned (SCN-109-003) | `./smackerel.sh test unit` |
| TP-02-03 | unit | `internal/mcp/capability` registry-isolation test | Set-difference assertion: none of the 7 `noop_*` tools and none of the 12 `internal/recommendation/tools/register.go` constant-success handlers appear in the MCP manifest, and the manifest is not derived from `agent.All()` (SCN-109-004) | `./smackerel.sh test unit` |
| TP-02-04 | integration | `internal/mcp` against the ephemeral test stack | Client A (`context` only) invoking an unregistered name and invoking `totally.nonexistent` produce **byte-identical** method-not-found responses. Adversarial: the natural implementation returns a distinct "unauthorized" error, which passes a naive "A is denied" assertion and fails this one (R-109-UX12) | `./smackerel.sh test integration` |
| TP-02-05 | integration | `internal/mcp` against the ephemeral test stack | `context.get_daily_brief` and `context.get_active_topics` return real synthesis/lifecycle output — not a constant — and | TP-02-06 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-109-003 and SCN-109-004 against the live stack: repeated `tools/list` calls on distinct connections stay byte-identical for the same credential, and none of the 19 `agent.All()` non-capabilities ever appears in the served manifest. Fails if discovery regains a `range`-over-map ordering or if the manifest is ever re-derived from the general agent registry; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |
| TP-02-07 | unit | `internal/mcp/capability` tool-name conformance test | The set of `Descriptor.ID` values equals the `spec.md` D5 Tool Inventory by **two-way set equality**; every name satisfies T1 (1–128 chars, `^[A-Za-z0-9_.-]+$`, unique, case-sensitive); and the manifest slice order equals the inventory row order (T2). Adversarial: registration of a well-formed name that is **absent from the inventory** must be refused — the natural implementation accepts any handler a developer registers, which passes a "names are well-formed" test and fails this one (SCN-109-P04) | `./smackerel.sh test unit` |

### Definition of Done

- [ ] All three `context` tools are registered as explicit `Descriptor` rows with all four annotation hints set literally; the manifest is a slice with a total order
- [ ] Every `Descriptor.ID` is the public wire name from the `spec.md` D5 Tool Inventory; registration refuses a duplicate name, an out-of-charset name (T1), and any name absent from the inventory
- [ ] `TP-02-01` unit test passes — descriptors resolve to real domain services, annotations explicit
- [ ] `TP-02-02` unit test passes — byte-identical `tools/list` at a pinned readiness state, adversarial against map iteration
- [ ] `TP-02-03` unit test passes — all 19 `agent.All()` non-capabilities absent; no passthrough
- [ ] `TP-02-04` integration test passes — ungranted and nonexistent names byte-identical
- [ ] `TP-02-05` integration test passes — real service output plus one `authorized` ledger row per call
- [ ] `TP-02-07` unit test passes — served names equal the D5 inventory by two-way set equality, satisfy T1, and are served in the inventory row order
- [ ] `List` consults only the per-request credential — grep-verified absence of session/connection/cache reads on the discovery path
- [ ] `TP-02-06` regression e2e-api test passes — deterministic discovery ordering and registry isolation from `agent.All()` are permanently protected
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-02-06`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default/fallback introduced

---

## Scope 03: `memory-read` Over The Spec-095 Executor + Projection Enforcement

**Status:** Not Started
**Depends On:** Scope 01 (the capability foundation), Scope 02
**Resolves:** UX-F-001 (via `design.md` §4); mitigates F-109-002; honors F-109-006
**Surfaces:** `internal/mcp/{capability,authz,projection}`, `internal/retrieval/routing`, the search domain service

### Use Cases (Gherkin)

**SCN-109-001 — Raw content never crosses the MCP boundary**

```gherkin
Given an MCP client with the "memory-read" toolset granted
And the corpus contains an artifact whose content_raw is a phishing email body
When the client calls memory.search and receives results
Then no field in any response contains content_raw
And each result carries only Projection Contract fields
And the phishing body text does not appear anywhere in the response
```

**SCN-109-005 — Remote-inference egress is denied without a grant**

```gherkin
Given a registered MCP client whose declared inference_locality is "remote"
And the operator has not granted remote-inference to that client
When the client calls any memory-read tool
Then the call is denied
And a denial is recorded with reason "remote-inference not granted"
And no corpus projection is returned
```

**SCN-109-006 — Granted remote-inference egress is always audited**

```gherkin
Given the operator has explicitly granted remote-inference to client R
When client R successfully calls memory.search
Then the projection is returned
And exactly one remote-egress audit row is written naming client R, the tool, and the timestamp
```

**SCN-109-007 — Retrieval fallback is reported, not hidden**

```gherkin
Given the routing executor selects a fallback strategy for a query
When the MCP client receives the result
Then retrieval_fell_back is true
And the fallback reason from StrategySelection is present
And the result is not marked isError
```

**SCN-109-P05 — A resource URI never carries PII (traces: `spec.md` §9B R-109-RES2, R-109-RES3, §9B.2)**

```gherkin
Given an artifact whose title and extracted entities contain a person name and an email address
When a client with memory-read and the corpus data scope calls a memory-read tool
Then every emitted resource_link URI matches one of the five R-109-RES2 templates
And no URI contains the person name, the email address, or any external identifier
And no resources/list capability is advertised and no resource-listing method is served
```

### Implementation Plan

- **The seam.** `memory.search`'s handler is a thin adapter over the injected
  `Retrieve(ctx, intent.CompiledIntent, routing.RetrievalRequest) (routing.RetrievalResult, routing.StrategySelection, error)`.
  Flow: natural-language `query` (P2 — no exact tags or dates demanded) → intent compilation →
  `RetrievalRequest` carrying the allowed-source predicate and limit → `Retrieve` → projection mapping.
- **Own the seam; do not wait on spec 095's packets.** MCP depends on the `Executor` **interface** and
  ships its own concrete `cmd/core` wiring for the MCP path. PKT-095-A/B/C are explicitly **not**
  prerequisites, so 109 does not inherit 095's schedule. State plainly in every artifact that MCP is
  spec 095's **first live consumer** (F-109-002) — do not imply 095 was proven in production.
- **Corpus data scope (`design.md` §4, resolves UX-F-001).** Add `data-scope` as a third `GrantKind`
  with exactly one member today (`corpus`). `memory-read` descriptors declare `RequiredData: [corpus]`.
  The authorizer resolves the four-term intersection. A client denied by data scope experiences
  **Case A verbatim** — tools omitted, method-not-found, no envelope, no challenge — because a distinct
  client-visible corpus token would itself be an existence oracle. The operator granularity lives in
  the ledger's `denial_reason` column only.
- **All six provenance fields (UX-F-004).** Map `source_kind`, `retrieval_strategy`,
  `retrieval_reason`, `retrieval_fell_back`, `retrieval_contract_known`, `trace_token` — **all six**,
  mandatory and non-omittable on every result whether or not a fallback occurred. Field *absence* must
  never be how a client infers "no fallback".
- **Egress gated before the read.** Gate 4 runs **before** dispatch and before any retrieval; a denied
  `remote-inference` client causes **zero** corpus reads. The denial is not a post-filter over
  already-materialized results.
- **Source-level filter is SQL, not a post-filter (F-109-006).** The domain service backing
  `memory.search` accepts an allowed-source predicate and applies it in the `WHERE` clause, **before**
  ranking and **before** `LIMIT`. Filtering after `LIMIT` silently shrinks a page and leaks source
  existence through result counts. `internal/mcp` **passes** the predicate; it does not implement the
  filter. Do **not** call `internal/api/search.go`'s handler internally (re-enters
  `bearerAuthMiddleware`; `SearchResult` carries no `source_id`).
- **Degrade honestly.** Fallback → `degraded-fallback`, `isError=false`. A genuine `error` return →
  `execution-failed`, `isError=true`. Never render an unknown routing state as an empty result set;
  "no results found" over a provider error is a forbidden rendering (R-109-UX20).
- **Resource links, reference-not-content (`spec.md` §9B).** `memory.search` returns compact
  Projections **plus** `resource_link` entries — a sibling transport content entry, never a
  Projection field, so §9's closed field set is untouched. Every URI is built by the single
  `projection.ResourceURI(kind, id)` constructor, whose `id` parameter is an `OpaqueID` newtype
  over the internal surrogate key, so a name, an email address, or a raw external identifier is a
  **compile error** rather than a review finding (R-109-RES3). `kind` is a closed enum over exactly
  the five R-109-RES2 templates. Following a link re-enters `tools/call` through all five gates —
  a pointer, never a bypass, and it never dereferences to raw content.
- **No enumeration (§9B.2).** Register **no** `resources/list` handler and advertise no
  resource-listing capability during `initialize`. An enumerable list is a corpus-enumeration
  oracle: it would return, in one call, exactly the existence-and-volume information R-109-UX12
  and F-109-006 spend the whole authorization model withholding. Resources are addressable by
  template, never browsable.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-03-01 | unit | `internal/mcp/projection` mapping test | All **six** provenance fields are populated from `StrategySelection` on every result, fallback or not; the constructor cannot be called without a `StrategySelection` (R-109-UX15, UX-F-004) | `./smackerel.sh test unit` |
| TP-03-02 | unit | `internal/mcp/authz` intersection test (**T12**) | A client granted `memory-read` but **not** the `corpus` data scope experiences Case A exactly (omitted + method-not-found + no envelope), while the ledger row carries `outcome=unauthorized-toolset` with a `denial_reason` naming the corpus scope. Adversarial: the natural implementation either ignores the data scope or emits a distinct client-visible token (`design.md` §4) | `./smackerel.sh test unit` |
| TP-03-03 | integration | `internal/mcp` against the ephemeral test stack (**T3**) | Client A holds `context` only; client B holds `context` + `memory-read` + `corpus`. A's list omits `memory.*`; A invoking `memory.search` is **byte-identical** to A invoking `totally.nonexistent` (SCN-109-003, R-109-UX12) | `./smackerel.sh test integration` |
| TP-03-04 | integration | `internal/mcp` against the ephemeral test stack (**T5**) | Seed an artifact whose `content_raw` is a prompt-injection payload carrying a rare marker; query so it ranks first. The marker appears **nowhere** in the response bytes, and `summary`/`title` are extraction outputs, not substrings of the body (SCN-109-001) | `./smackerel.sh test integration` |
| TP-03-05 | integration | `internal/mcp` against the ephemeral test stack (**T6a**) | Declared-`remote` client with no grant → `unauthorized-egress-class`, §9A.2 envelope, zero projections, `smackerel_mcp_tool_denied_total` increments, **and a spy executor records zero `Retrieve` calls** — proving gate 4 precedes dispatch rather than filtering results (SCN-109-005) | `./smackerel.sh test integration` |
| TP-03-06 | integration | `internal/mcp` against the ephemeral test stack (**T6b**) | With the grant: the projection returns, exactly **one** `egress_class=remote-inference` ledger row is written, and `smackerel_mcp_remote_egress_total` equals the ledger row count (SCN-109-006, OF-4 step 3) | `./smackerel.sh test integration` |
| TP-03-07 | e2e-api | `/mcp` on the ephemeral stack, real Postgres (**T7**) | Force a fallback: `retrieval_fell_back=true`, `retrieval_reason` non-empty, `isError=false`, `outcome=degraded-fallback`. Then force a routing error: `isError=true`, `outcome=execution-failed`, and the response is **not** an empty result set. Adversarial: "no results found" is the natural error rendering and passes a shape-only test (SCN-109-007, R-109-UX20; discharges F-095-E2E-LIVE for the MCP path only) | `./smackerel.sh test e2e` |
| TP-03-08 | e2e-api | `/mcp` on the ephemeral stack (**T8**) | Byte-level `content_raw` scan across every enabled toolset under **both** egress classes; `smackerel_mcp_projection_raw_content_blocked_total == 0` (§2 Success Signal). | TP-03-09 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-109-001, SCN-109-003, SCN-109-005, SCN-109-006 and SCN-109-007 against the live stack: raw `content_raw` still never reaches the wire, an ungranted-egress client still gets zero executor dispatches, a corpus-scope denial still renders as Case A while staying granular in the ledger, and a fallback still renders `isError=false`/`degraded-fallback` rather than an empty result set. Fails if the projection type regains an open field, if egress gating moves after dispatch, or if the source predicate becomes a post-filter; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |
| TP-03-10 | integration | `internal/mcp` against the ephemeral test stack | Seed an artifact whose title and extracted entities carry a rare person name and a rare email address, then scan **every** `resource_link` URI in the response: each matches one of the five R-109-RES2 templates, and neither marker — nor any external identifier — appears in any URI. Also asserts `initialize` advertises no resource-listing capability and `resources/list` is unserved (§9B.2). Adversarial: the natural implementation builds a "human-readable" slug from `title`, which passes a body-only `content_raw` scan (TP-03-04 / TP-03-08) and fails only this one, because the leak is in the envelope rather than the payload (SCN-109-P05) | `./smackerel.sh test integration` |

### Definition of Done

- [ ] `memory.search` and `memory.get_artifact_projection` resolve through the injected `routing.Executor`; no internal HTTP hop and no call into `internal/api/search.go`
- [ ] `data-scope` exists as a third `GrantKind` with member `corpus`; the authorizer evaluates the full four-term intersection
- [ ] The backing domain service applies the allowed-source predicate in SQL **before** ranking and **before** `LIMIT` (F-109-006); no post-filter path exists
- [ ] `TP-03-01` unit test passes — all six provenance fields non-omittable
- [ ] `TP-03-02` unit test passes — corpus data-scope denial is Case A to the client, granular in the ledger
- [ ] `TP-03-03` integration test passes — grant-varied discovery with no existence leak
- [ ] `TP-03-04` integration test passes — injection marker absent from response bytes
- [ ] `TP-03-05` integration test passes — spy executor records **zero** `Retrieve` calls on egress denial
- [ ] `TP-03-06` integration test passes — one remote-egress row per invocation; counter equals row count
- [ ] `TP-03-07` e2e-api test passes — fallback is `isError=false`/`degraded-fallback`; routing error is `isError=true`/`execution-failed` and never an empty result set
- [ ] `TP-03-08` e2e-api test passes — byte-level `content_raw` scan clean; blocked-counter is 0
- [ ] `resource_link` URIs are built only by the single `projection.ResourceURI(kind, id)` constructor whose `id` is an `OpaqueID` newtype and whose `kind` is a closed enum over the five R-109-RES2 templates; no `resources/list` handler is registered and no resource-listing capability is advertised at `initialize` (§9B.2)
- [ ] `TP-03-10` integration test passes — no resource URI carries a person name, an email address, or any external identifier, and every URI matches an R-109-RES2 template (R-109-RES3)
- [ ] `docs`-visible statement recorded for Scope 07: MCP is spec 095's **first live consumer**; F-095-E2E-LIVE is discharged for the MCP path only, and no artifact claims 095 is generally proven
- [ ] `TP-03-09` regression e2e-api test passes — raw-content containment, pre-dispatch egress gating, Case A corpus denial, and honest fallback rendering are permanently protected
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-03-09`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default/fallback introduced

---

## Scope 04: `person-context` Toolset

**Status:** Not Started
**Depends On:** Scope 01 (the capability foundation), Scope 02
**Surfaces:** `internal/mcp/capability`, person/entity domain service

### Use Cases (Gherkin)

**SCN-109-003 (person-context slice) — an opt-in toolset appears only for a credential that holds it**

```gherkin
Given client A holds a credential granting only "context"
And client P holds a credential granting "context" and "person-context"
When each calls tools/list against the same running server
Then client A does not see person.get_context
And client P sees person.get_context
And each list is byte-identical across repeated calls with the same credential
```

**SCN-109-004 (person-context slice) — the advertised person tool is service-backed**

```gherkin
Given client P holds the person-context toolset
When client P calls person.get_context
Then the result is produced by the person/entity domain service
And the result is not a constant-success payload
And no field in the response contains content_raw
```

### Implementation Plan

- Register `person.get_context` as an explicit `Descriptor`: `Toolset: person-context`, opt-in
  (no default grant), `local-inference`, **`RequiredData: []`** — deliberately no data scope. This
  isolates the §4 corpus term from the toolset term, so a regression in either is attributable.
- Declare all four annotation hints explicitly; `openWorld=false` (the person graph is Smackerel-local).
- Resolve through the person/entity domain service directly (P-SVC). No internal HTTP hop, no
  passthrough from `agent.All()`.
- Emit only Projection Contract fields (P-PROJ). Person output reuses `internal/mcp/projection`'s
  closed types; do **not** introduce a parallel, open person payload type — that would reopen the D2
  hole the closed type exists to shut.
- Denial is Case A: omitted from `tools/list`, method-not-found on invocation, no envelope, no
  challenge. `person-context` is never the toolset that first exposes a step-up challenge — that is
  Scope 05, by construction of the gate order.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-04-01 | unit | `internal/mcp/capability` manifest test | `person.get_context` declares `Toolset: person-context`, opt-in default, **empty** `RequiredData`, and all four annotation hints explicitly | `./smackerel.sh test unit` |
| TP-04-02 | unit | `internal/mcp/authz` opt-in test | A credential without the `person-context` grant omits `person.*` from `tools/list`; invocation is method-not-found byte-identical to a never-existing name; the ledger row carries `unauthorized-toolset` | `./smackerel.sh test unit` |
| TP-04-03 | integration | `internal/mcp` against the ephemeral test stack | A granted client receives real person/entity service output — varying with seeded data, not constant — with zero `content_raw` and only Projection Contract fields; one `authorized` ledger row is written | `./smackerel.sh test integration` |
| TP-04-04 | e2e-api | `/mcp` on the ephemeral stack | End-to-end `initialize` → `tools/list` → `tools/call person.get_context` under `local-inference`; | TP-04-05 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-109-003 and SCN-109-004 against the live stack: `person-context` stays opt-in (an ungranted client still sees Case A byte-identical to a nonexistent name), the response still carries only Projection Contract fields, and no `external_ref` is ever emitted. Fails if the toolset silently becomes default-on or if a parallel open payload type is introduced for person output; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] `person.get_context` is registered opt-in with empty `RequiredData` and explicit annotations; it resolves through the person/entity domain service with no HTTP hop
- [ ] Person output reuses the closed `internal/mcp/projection` types; no parallel open payload type is introduced
- [ ] `TP-04-01` unit test passes — descriptor shape correct, data scope deliberately empty
- [ ] `TP-04-02` unit test passes — ungranted client sees Case A, ledger records `unauthorized-toolset`
- [ ] `TP-04-03` integration test passes — real service output, zero `content_raw`, one `authorized` row
- [ ] `TP-04-04` e2e-api test passes — full session clean under byte-level scan, no `external_ref`
- [ ] `TP-04-05` regression e2e-api test passes — opt-in default, closed projection field set, and the never-emitted `external_ref` are permanently protected
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-04-05`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default/fallback introduced

---

## Scope 05: `graph-read` Toolset + Scope-Minimized Step-Up Challenge

**Status:** Not Started
**Depends On:** Scope 01 (the capability foundation), Scope 02, Scope 04
**Surfaces:** `internal/mcp/capability`, `internal/mcp/authz`, knowledge-graph domain service

### Use Cases (Gherkin)

**SCN-109-010 — Scope challenges disclose only what is missing**

```gherkin
Given a client authorized for "context" calls a graph-read tool
When the authorizer denies the call
Then the WWW-Authenticate challenge names only the missing graph-read scope
And no other scope name appears in the response
```

**SCN-109-003 (graph-read slice) — graph tools appear only for a credential that holds the grant**

```gherkin
Given client G holds a credential granting "context" and "graph-read"
When client G calls tools/list
Then graph.get_neighbors and graph.get_topic are present
And a client holding only "context" sees neither
And each list is byte-identical across repeated calls with the same credential
```

### Implementation Plan

- Register `graph.get_neighbors` and `graph.get_topic` as explicit `Descriptor` rows: `Toolset:
  graph-read`, opt-in, `local-inference`, empty `RequiredData`.
- **`lifecycle_state` is a first-class Projection field** (Product Principle 3), so graph results
  expose lifecycle rather than a flat snapshot. Consequently `graph.*` annotations declare
  `idempotent=false` across time — the same explicit-hint requirement that `SideEffectClass` cannot
  express.
- **Step-up challenge (Case C).** Depends on Scope 04 so that at least two opt-in toolsets coexist:
  with only one, a "names exactly one scope" assertion is trivially satisfied and cannot detect a
  catalog leak. The challenge names **only** `scope="graph-read"` — never
  `scope="context memory-read person-context graph-read …"`. The full catalog is never published in
  any response.
- **Case C applies only where the denial is legitimately visible to the client.** It must not apply
  to Case A or Case B, where the correct response is method-not-found with **no challenge at all**.
  Getting this backwards converts the challenge into an existence oracle.
- Resolve through the knowledge-graph domain service (P-SVC). No new search backend, no parallel
  index (Product Principle 5 — a new *view*, not a parallel store).

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-05-01 | unit | `internal/mcp/capability` manifest test | Both `graph.*` descriptors declare `Toolset: graph-read`, opt-in, empty `RequiredData`, `idempotent=false`, and all four hints explicitly | `./smackerel.sh test unit` |
| TP-05-02 | unit | `internal/mcp/authz` gate-order test | A gate-2/gate-3 denial returns **no** challenge and no envelope, while a gate-4 denial does. Adversarial: an implementation that emits a challenge on every denial passes a naive "challenge present" test and fails this one | `./smackerel.sh test unit` |
| TP-05-03 | integration | `internal/mcp` against the ephemeral test stack | A `context`-only client denied on a legitimately-visible `graph-read` gate receives a `WWW-Authenticate` naming exactly one scope; a full-catalog scan asserts no other registered scope name (`memory-read`, `person-context`, `corpus`, `hospitality-read`) appears anywhere in the response bytes (SCN-109-010) | `./smackerel.sh test integration` |
| TP-05-04 | e2e-api | `/mcp` on the ephemeral stack | A granted client's `graph.get_neighbors` returns real graph-service output carrying `lifecycle_state`; | TP-05-05 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-109-003 and SCN-109-010 against the live stack: a step-up challenge still names exactly one scope and no other registered scope name appears anywhere in the response bytes, and gate-2/gate-3 denials still carry no challenge and no envelope. Fails if the challenge is ever broadened into a scope catalogue or emitted on every denial — either of which turns the challenge into a scope-enumeration oracle; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] Both `graph.*` tools registered opt-in with explicit annotations; they resolve through the knowledge-graph domain service — no new search backend, no parallel index
- [ ] Case C is reachable **only** for gate-4 denials; gates 2 and 3 return method-not-found with no challenge and no envelope
- [ ] `TP-05-01` unit test passes — descriptor shape and explicit hints correct
- [ ] `TP-05-02` unit test passes — challenge emitted only at gate 4
- [ ] `TP-05-03` integration test passes — challenge names exactly one scope; no other scope name anywhere in the response
- [ ] `TP-05-04` e2e-api test passes — real graph output with `lifecycle_state`, clean byte scan, one `authorized` row
- [ ] `TP-05-05` regression e2e-api test passes — scope-minimized challenge and gate-4-only challenge emission are permanently protected against becoming a scope-enumeration oracle
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-05-05`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default/fallback introduced

---

## Scope 06: `hospitality-read` Toolset — **BLOCKED**

**Status:** Blocked
**Depends On:** Scope 01 (the capability foundation), Scope 02
**Blocked By:** `BUG-019-003` — `artifacts.source_ref` is never persisted by the connector front door
(`specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted/`, status `specs_hardened`)
**Blocking Finding:** F-109-001 (`spec.md` §14) · **Job blocked:** J4 (STR host ops agent)
**Surfaces:** `internal/mcp/capability`, hospitality projection service

**Blocked Reason.** `hospitality.get_guest_context` requires a **stable external reference per
artifact** so a guest-context projection can cite the record it came from. `artifacts.source_ref` is
never persisted, so no such reference exists. `spec.md` §9's external-reference honesty rule forbids
synthesizing one: emitting a reconstructed or guessed `external_ref` is a fabrication. Per `spec.md`
§18 item 5 (**ratified 2026-07-29**) the operator's decision is that this toolset **waits** rather
than shipping a fabricated
reference. The scope is therefore specified, registered, and testable **as honestly unavailable**;
serving it is deliverable only after BUG-019-003 is fixed. This scope does **not** block Scope 07.

### Use Cases (Gherkin)

**SCN-109-012 — `hospitality-read` is honestly unavailable**

```gherkin
Given BUG-019-003 is unresolved and artifacts.source_ref is never persisted
When the operator grants hospitality-read to a client
Then the toolset is reported unavailable with the BUG-019-003 reason
And no guest-context tool appears in tools/list
And no fabricated external reference is emitted
```

### Implementation Plan

- Register `hospitality.get_guest_context` in the manifest with a `Readiness` resolver returning
  `{Available: false, Reason: "<BUG-019-003 reason>"}`. Registration is deliberate: it is what lets
  the operator surface report *why* a grant they made has no effect.
- **Readiness is a server-state predicate, deliberately not a credential predicate.** The descriptor
  is omitted from `tools/list` and returns method-not-found on invocation, so the **client**
  experiences Case B ≡ Case A exactly. Never present-but-fake (§12).
- **The honesty is owed to the operator, not to the client (R-109-UX13).** The operator surface
  reports `capability-unavailable` with the blocking reason, rendered through the §9A.2 four-field
  envelope — `reference:` points at `docs/API.md` — "MCP toolset availability", never at a `specs/` path.
- The grant itself is still **recorded** (`absent → granted`) even though the toolset is not served,
  so the operator's intent survives the eventual BUG-019-003 fix without a re-grant.
- Emit **no** `external_ref` anywhere on this path. `deep_link` (Smackerel-local) remains the only
  navigational field.
- **Do not** implement the guest-context retrieval body in this scope. Writing a handler that cannot
  be served would be a stub against a blocked dependency.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-06-01 | unit | `internal/mcp/capability` readiness test (**T9a**) | The descriptor is registered with `Readiness.Available=false` and a **non-empty** `Reason` naming BUG-019-003; the manifest-registration check rejects an unavailable descriptor carrying an empty reason | `./smackerel.sh test unit` |
| TP-06-02 | unit | `internal/mcp/authz` discovery test (**T9b**) | With the grant recorded, `tools/list` omits `hospitality.*` and invocation returns method-not-found byte-identical to a never-existing name. Adversarial: "present but returning an error" is the easy implementation and violates §12's never-present-but-fake rule (SCN-109-012) | `./smackerel.sh test unit` |
| TP-06-03 | integration | `internal/mcp` against the ephemeral test stack | Granting the toolset records the grant (`absent → granted`) **and** the operator surface reports `capability-unavailable` through the four-field envelope with the BUG-019-003 reason and a `docs/` reference; | TP-06-04 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-109-012 against the live stack: `hospitality.*` stays omitted from `tools/list` even for a granted client, invocation stays method-not-found byte-identical to a never-existing name, and **no** `external_ref` appears on any MCP response. Fails the moment the toolset is quietly flipped to present-but-erroring, or a synthesized external reference is emitted before BUG-019-003 persists `artifacts.source_ref`; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] `hospitality.get_guest_context` is registered with `Readiness.Available=false` and a non-empty BUG-019-003 reason; **no** retrieval handler body is implemented against the blocked dependency
- [ ] The grant is recorded even while the toolset is unserved, so the operator's intent survives the eventual fix without a re-grant
- [ ] `TP-06-01` unit test passes — unavailable descriptor requires a non-empty reason
- [ ] `TP-06-02` unit test passes — omitted from `tools/list`; method-not-found byte-identical to nonexistent
- [ ] `TP-06-03` integration test passes — operator sees `capability-unavailable` with the reason; no `external_ref` on any response
- [ ] `docs/API.md` — "MCP toolset availability" carries the honest unavailability statement (delivered in Scope 07)
- [ ] `TP-06-04` regression e2e-api test passes — honest unavailability (omitted, method-not-found, zero `external_ref`) is permanently protected against a quiet flip to present-but-erroring
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-06-04`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default/fallback introduced
- [ ] **Serving** `hospitality.get_guest_context` remains out of this scope until BUG-019-003 persists `artifacts.source_ref` (F-109-001, `spec.md` §18 item 5)

---

## Scope 07: Documentation, Release Packet, Configuration, And Flag Bundles

**Status:** Not Started
**Depends On:** Scope 03, Scope 04, Scope 05 (all transitively on the Scope 01 capability foundation)
**Sources:** `spec.md` §15 (documentation / configuration / flag contract), §16 (principle alignment), §17 (release train)
**Surfaces:** `docs/`, `docs/releases/`, `config/`

**Scope note.** Scope 06 is **not** a dependency: its documentation obligation is the honest
*unavailability* statement, which is writable today and is exactly what `spec.md` §14 requires.

### Use Cases (Gherkin)

Scenarios below are **planning-derived** (`SCN-109-P01…P03`): `spec.md` §7 authors no documentation
or release scenario, and this packet may not edit `spec.md` (G073). Each names the exact `spec.md`
requirement row it traces to. `SCN-109-004` is reused verbatim because the documentation obligation
is the same no-fabricated-capability invariant, expressed on the docs surface.

**SCN-109-P01 — Every `spec.md` §15 documentation row is satisfied (traces: §15 Documentation table)**

```gherkin
Given spec.md section 15 lists the required documentation changes
When the documentation surface is inspected after this scope
Then docs/smackerel.md 17.2 carries the MCP boundary, the D2 injection-containment rationale, and the D3 no-token-passthrough rule
And docs/smackerel.md 21 carries an MCP row recording Fabric.so parity plus the D1/D2 differentiation
And docs/smackerel.md 22 lists MCP as a sibling integration capability, explicitly not a connector and not an assistant transport
And docs/API.md, docs/Operations.md, docs/Architecture.md, and docs/INVESTOR_OVERVIEW.md each carry their required section
```

**SCN-109-P02 — The flag is default-ON in exactly one train (traces: §15 Flag contract, §17)**

```gherkin
Given config/release-trains.yaml declares the train "next"
When the feature-flag bundles are inspected
Then config/feature-flags.next.yaml sets mcpKnowledgeServer true
And config/feature-flags.mvp.yaml sets mcpKnowledgeServer false
And no other train sets mcpKnowledgeServer true
And the SST key mcp.knowledge_server carries no fallback default
```

**SCN-109-P03 — Every documented claim about egress is honest (traces: D1, §15, §16)**

```gherkin
Given D1 states Smackerel cannot verify a client's declared inference locality
When any document mentions remote-inference egress
Then it states that the control is an operator declaration, administrative and auditable, not cryptographic
And no document claims technical enforcement of inference locality
And the local-first narrative states MCP ships and local-first is preserved by D1 plus D2
```

**SCN-109-004 (documentation slice) — No fabricated capability is documented**

```gherkin
Given hospitality-read is registered but unserved under BUG-019-003
And memory-write is off by decision D5
When the capability ledger and the release packet features list are inspected
Then hospitality-read is recorded as specified-not-delivered with the BUG-019-003 reason
And memory-write is recorded as specified-not-delivered by decision, not as shipped
And no document claims a capability the running server does not serve
```

### Implementation Plan

- **`docs/smackerel.md` §17.2 (Trust & Security).** Add the MCP boundary: the D2 injection-containment
  rationale, why "content is data, not instructions" does **not** transfer to a foreign tool-calling
  agent that never inherits Smackerel's own system-prompt defense, and the D3 no-token-passthrough rule.
- **`docs/smackerel.md` §21 (Competitive Landscape).** Add an **MCP row**. Fabric.so already ships MCP
  (§21.1 strength), so this is explicitly a **parity** move; the **differentiating** move is D1+D2 —
  provably local-first by default and never emitting raw content. Annotate honestly: the D1 control is
  an operator declaration, **not verified**. Record that the row this actually strengthens is
  "Local-first / own your data", and that shipping MCP without D1+D2 would have made the §21.4 UVP false.
- **`docs/smackerel.md` §22 (Connector Ecosystem & Reuse).** Add MCP to the integration inventory as a
  **sibling integration capability** — explicitly **not** a connector (connectors are ingress and are
  the surface that fails to persist `source_ref`; MCP is egress) and **not** an assistant transport
  (§13 non-goal).
- **`docs/smackerel.md` §17.1 (UX-F-002).** Add the confidence-signal tie for R-109-UX15–UX18. §15's
  table omits this row; adding it is recorded here as a **planning decision**, and the `spec.md` §15
  amendment is routed to `bubbles.analyst` rather than silently applied.
- **`docs/API.md`.** Document `/mcp`: stateless Streamable HTTP transport, the credential + audience
  requirement, the §8.2 toolset inventory, the Projection Contract field list including the
  never-emitted set, the §12 failure-semantics table, the scope-challenge behavior, and an
  "MCP toolset availability" section carrying the `hospitality-read` honest-unavailability statement.
- **`docs/Operations.md`.** Operator runbook covering OF-1…OF-5: register a client, issue an
  audience-bound credential, grant/revoke toolsets, **grant/revoke the `corpus` data scope**
  (F-109-OF2-AMEND — OF-2 needs this step; without it the operator hits "I granted it and nothing
  happened"), grant/revoke `remote-inference` with the mandatory explicit per-`client_id` confirmation
  (R-109-UX8), read the MCP audit ledger, and interpret the five §11 metrics. **No flow may require
  hand-editing a config file** — that is an explicit §2 Failure Condition.
- **`docs/Architecture.md`.** Place the MCP capability layer: mounted at `/mcp` on `smackerel-core`,
  consuming domain services directly, no internal HTTP hop, no assistant `TransportAdapter`, its own
  authorizer and its own append-only ledger.
- **`docs/INVESTOR_OVERVIEW.md`.** Update the local-first moat narrative: MCP is shipped **and**
  local-first is preserved by D1 + D2. State the D1 limitation honestly in the same paragraph.
- **Capability ledger surface.** Record MCP with its honest delivery state: `context`, `memory-read`,
  `person-context`, `graph-read` delivered; `hospitality-read` **specified, not delivered**
  (BUG-019-003); `memory-write` **specified, not delivered** (D5 decision); `assistant`, `operations`
  off; `external` **permanently off**. If no capability-ledger file exists in this repo, record the
  same inventory in `docs/API.md` — "MCP toolset availability" and state that placement in the release
  packet, rather than inventing a new ledger surface in a planning-only packet.
- **Release packet.** Under `docs/releases/` for the `next` train: add the MCP entry and update the
  packet's `features.md` with the flag name, the owning train, the honest delivery state above, and the
  `remote-inference` egress opt-in. Do **not** claim `hospitality-read` or `memory-write` as shipped.
- **`config/release-trains.yaml`.** Confirm `next` is a declared train and record spec 109's targeting.
- **`config/feature-flags.next.yaml`.** `mcpKnowledgeServer: true` — owning train, default-ON.
- **`config/feature-flags.mvp.yaml`.** `mcpKnowledgeServer: false` — default-OFF.
  Per the mechanically-enforced release-train policy a flag is default-ON in **exactly one** train.
  `remote-inference` is deliberately **not** a flag: it is a per-client registry grant, so no build can
  enable it globally.
- **`docs/Product-Principles.md` alignment note.** Record the §16 alignment against **Principle 8 —
  Trust Through Transparency** (UX-F-003: there is no pre-existing Principle 11), plus P2, P3, P4, P5,
  P6, P10. Carry the §16 principle-gap note in its **ratified** form: the commissioning brief cited
  "Principle 9 — Own your data", Principle 9 is actually "Design For Restart, Not Perfection", and no
  *existing* numbered principle states data ownership — that commitment is carried today by
  Constitution C1, `docs/smackerel.md` §21.4, and `docs/INVESTOR_OVERVIEW.md`.
- **`docs/Product-Principles.md` NEW Principle 11 — OWNER SIGN-OFF REQUIRED.** Ratified `spec.md` §18
  item 7 (2026-07-29) decides the gap **closes by addition**: the document gains **Principle 11 — Your
  Data Stays Yours**, using the paragraph ratified in §18 item 7 verbatim, in the house format
  (`## Principle 11 — Your Data Stays Yours` + `**Status**: Ratified <date>`), placed after Principle 10
  and before `## Surfacing Process`. Constitution C1 is **not** the sole carrier.
  **This amends an owner-ratified document** (`docs/Product-Principles.md` is stamped "Ratified
  2026-06-03" and states edits go through the normal product-principles change process), so it MUST NOT
  be applied by an agent without recorded owner sign-off.
  Because that document's companion enforcement file
  `.github/instructions/product-principles.instructions.md` is declared **BLOCKING** and carries a
  per-principle enforcement block for each of P1–P10, the same change set MUST add a matching
  **Principle 11** enforcement block — otherwise the product's single biggest differentiator ships as
  an unenforced principle, which is exactly the "claim with no enforcement track" gap §18 item 7
  exists to close.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-07-01 | unit | `internal/config` flag-bundle parity test | `mcpKnowledgeServer` is `true` in `config/feature-flags.next.yaml`, `false` in `config/feature-flags.mvp.yaml`, and `true` in **exactly one** train across every declared train in `config/release-trains.yaml` (SCN-109-P02) | `./smackerel.sh test unit` |
| TP-07-02 | unit | `cmd/core` config-resolution test | `mcp.knowledge_server` resolves fail-loud: absent or malformed aborts startup with the named error. Adversarial: a `${VAR:-…}` or `os.Getenv`-with-default implementation silently boots and fails this test (SCN-109-P02) | `./smackerel.sh test unit` |
| TP-07-03 | unit | `internal/docs` documentation-contract test | Every `spec.md` §15 target file exists and carries its required section anchor — `docs/smackerel.md` §17.1/§17.2/§21/§22, `docs/API.md` `/mcp` + "MCP toolset availability", `docs/Operations.md` MCP runbook, `docs/Architecture.md`, `docs/INVESTOR_OVERVIEW.md`, `docs/Product-Principles.md` alignment note, and the `next` release packet's `features.md`. **Also** (ratified §18 item 7): `docs/Product-Principles.md` carries a `## Principle 11 — Your Data Stays Yours` heading with a `**Status**: Ratified` stamp, **and** `.github/instructions/product-principles.instructions.md` carries a matching Principle 11 enforcement block. Adversarial: adding the principle to the principles document but **not** to the BLOCKING companion enforcement file fails this test — an unenforced principle is the gap, not the fix (SCN-109-P01) | `./smackerel.sh test unit` |
| TP-07-04 | integration | doc/registry consistency check on the ephemeral stack | The toolsets documented as **delivered** equal the toolsets the running server actually serves, by two-way set equality; `hospitality-read` and `memory-write` appear as specified-not-delivered with their reasons and appear in **neither** served set. Adversarial: a docs-only assertion passes while the server serves a different set (SCN-109-004) | `./smackerel.sh test integration` |
| TP-07-05 | e2e-api | `/mcp` on the ephemeral stack with the `next` bundle | With `config/feature-flags.next.yaml` applied, `/mcp` is mounted and `initialize` succeeds; with the `mvp` bundle applied, `/mcp` is not served. | `./smackerel.sh test e2e` |
| TP-07-06 | e2e-api | `/mcp` regression suite on the ephemeral stack | **Regression E2E** — persistent scenario-specific regression for SCN-109-004, SCN-109-P01, SCN-109-P02 and SCN-109-P03 against the live stack: the documented-delivered toolset set still equals the served set by two-way equality, `hospitality-read` and `memory-write` still appear in neither served set, and the flag still gates `/mcp` per train with no default. Fails if documentation drifts ahead of what the server actually serves — the exact dishonesty this feature exists to prevent; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] `docs/smackerel.md` §17.2 carries the MCP boundary: D2 injection-containment rationale, why the "content is data, not instructions" defense does not transfer to a foreign agent, and the D3 no-token-passthrough rule
- [ ] `docs/smackerel.md` §21 carries an MCP row: Fabric.so parity + D1/D2 differentiation + the honest "operator declaration, not verified" annotation
- [ ] `docs/smackerel.md` §22 lists MCP as a **sibling integration capability**, explicitly not a connector and not an assistant transport
- [ ] `docs/smackerel.md` §17.1 carries the confidence-signal tie for R-109-UX15–UX18 (UX-F-002 recorded as a planning decision; the `spec.md` §15 amendment is routed, not silently applied)
- [ ] `docs/API.md` documents `/mcp`: transport, credential + audience, toolset inventory, Projection Contract incl. the never-emitted set, failure-semantics table, scope-challenge behavior, and "MCP toolset availability"
- [ ] `docs/Operations.md` carries the OF-1…OF-5 runbook **including the `corpus` data-scope grant/revoke step** (F-109-OF2-AMEND) and the mandatory explicit per-`client_id` `remote-inference` confirmation; no flow requires hand-editing a config file
- [ ] `docs/Architecture.md` places the MCP capability layer: `/mcp` on `smackerel-core`, direct domain-service consumption, no HTTP hop, no assistant `TransportAdapter`
- [ ] `docs/INVESTOR_OVERVIEW.md` records MCP shipped **and** local-first preserved by D1+D2, with the D1 limitation stated honestly in the same passage
- [ ] The capability-ledger surface records the honest delivery state (`hospitality-read` and `memory-write` specified-not-delivered with reasons); if no ledger file exists, the same inventory lands in `docs/API.md` and the placement is stated in the release packet
- [ ] The `next` release packet under `docs/releases/` and its `features.md` carry the MCP entry, the flag name, the owning train, the honest delivery state, and the `remote-inference` egress opt-in
- [ ] `config/release-trains.yaml` confirms `next` is declared and records spec 109's targeting
- [ ] `docs/Product-Principles.md` alignment note cites **Principle 8** (UX-F-003) and carries the §16 principle-gap note in its **ratified** form — recorded as resolved by §18 item 7, not as an open owner decision
- [ ] **OWNER SIGN-OFF REQUIRED (amends an owner-ratified document).** `docs/Product-Principles.md` gains **`## Principle 11 — Your Data Stays Yours`** with a `**Status**: Ratified <date>` stamp, using the paragraph ratified in `spec.md` §18 item 7 verbatim, placed after Principle 10 and before `## Surfacing Process`. That document is stamped "Ratified 2026-06-03" and states edits go through the normal product-principles change process, so **no agent may apply this without recorded owner sign-off**; absent sign-off this scope is **Blocked on this item**, never silently skipped or downgraded
- [ ] **OWNER SIGN-OFF REQUIRED (same change set).** `.github/instructions/product-principles.instructions.md` — the **BLOCKING** companion enforcement file, which carries a per-principle enforcement block for each of P1–P10 — gains a matching **Principle 11** enforcement block. Shipping the principle without its enforcement block would leave the product's single biggest differentiator unenforced, which is the exact "product claim with no enforcement track" gap §18 item 7 exists to close
- [ ] `TP-07-01` unit test passes — flag default-ON in exactly one train, default-OFF elsewhere
- [ ] `TP-07-02` unit test passes — `mcp.knowledge_server` fail-loud with no fallback default
- [ ] `TP-07-03` unit test passes — every §15 target file and section anchor present
- [ ] `TP-07-04` integration test passes — documented-delivered set equals served set by two-way equality
- [ ] `TP-07-05` e2e-api test passes — the flag actually gates the `/mcp` surface per train
- [ ] `TP-07-06` regression e2e-api test passes — documented-delivered equals served by two-way equality, and per-train flag gating is permanently protected against documentation drifting ahead of the served surface
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-07-06`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh config generate` clean with zero warnings; no TODO/stub/default/fallback introduced
