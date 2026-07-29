# Design: 109 MCP Knowledge Server

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Planning only — no source edited.**
**Requirements source:** [`spec.md`](spec.md) · **Release train:** `next` · **Flag:** `mcpKnowledgeServer`

---

## Design Brief

**Current State.** No MCP server exists (§1). `smackerel-core` serves 517 route
registrations under `internal/api/`, all behind `bearerAuthMiddleware`. `internal/auth`
issues tokens with issuer/subject/jti/iat/nbf/exp/kid and an optional `scope`, but sets
**no audience**, and the `Audience` type in `request_authenticator.go` is unwired to
issuance (F-109-004). `internal/agent/registry.go` holds a general tool registry whose
`SideEffectClass` is a three-value enum and whose `All()` contains 19 non-capabilities —
7 `noop_*` tools plus 12 constant-success handlers in
`internal/recommendation/tools/register.go` (F-109-005). `internal/retrieval/routing`
exists as a handler-free, injectable executor but has **zero live consumers**
(F-109-002).

**Target State.** A new `internal/mcp/` capability layer mounted at `/mcp` on the
existing core listener, carrying its own audience-bound credential and its own
authorizer (D3), its own explicit export manifest (D5 §8.2), and a projection type that
structurally cannot carry `content_raw` (D2). Pull-only (D4). `local-inference` default,
`remote-inference` per-client granted and audited (D1).

**Patterns to Follow.**
- Spec 108 design §7's decoupling contract — different authorizer, different mount point,
  no shared mutable state. This design holds up its half of it.
- `internal/assistant/confirm.Machine`'s Propose → Confirm/Discard → SweepTimeouts
  lifecycle with a `transport` discriminator on the single-flight key. Consumed as a
  domain service (§8).
- `internal/assistant/contracts/refusal.go` + `refusal_test.go` — a closed refusal
  taxonomy with a mechanical test that fails on a banned string. §9A.1's seven tokens get
  the same treatment (§9 T10).
- `internal/metrics`'s `smackerel_*` counter families — extend the namespace with
  `smackerel_mcp_*`, do not fork the registry.

**Patterns to Avoid.**
- Deriving the MCP toolset from `agent.All()` or from `SideEffectClass` (F-109-005). It
  publishes 19 fabricated capabilities and cannot express four orthogonal hints.
- Registering MCP as a fourth assistant `TransportAdapter` (§13 non-goal). It inherits the
  band-LOW capture-acknowledgement path and makes the R-109-UX21 forbidden string
  reachable by construction.
- Calling `internal/api/search.go`'s handler internally. It re-enters
  `bearerAuthMiddleware` (D3 passthrough), and `SearchResult` carries no `source_id`
  (F-109-006).
- Any `${VAR:-…}` / `os.Getenv`-with-default for `mcpKnowledgeServer`
  (`.github/instructions/smackerel-no-defaults.instructions.md`, §15 flag contract).

**Resolved Decisions.** MCP owns its corpus grant (§4 — resolves UX-F-001). Gate order is
audience → toolset∩scope∩data-scope → readiness → egress (§5). Projection safety is a
type property, not a review property (§7). The write plane consumes `confirm.Machine` as a
domain service, never as a transport (§8). Audit gets its own table, not `agent_traces` (§9).

**Open Questions.** None blocking. UX-F-002 (`docs/smackerel.md` §17.1 missing from the §15
table) and UX-F-004/UX-F-005 (editorial) remain **OPEN**; none of them changes a structure
below. The `spec.md` §18 operator gate **closed on 2026-07-29** on the seven product
decisions only and did **not** decide these three, so they are re-routed to `bubbles.analyst`
(the `spec.md` owner) rather than left pointing at a closed gate.

---

## 1. Architecture Placement

**Mount.** `/mcp` on the **existing `smackerel-core` HTTP listener** (§10 Transport).
Stateless Streamable HTTP. **No new port, no new service, no new container.**

**Classification.** A **sibling integration capability** — the §15 wording for
`docs/smackerel.md` §22 — which means all three of these are true:

| It is NOT | Why not |
|---|---|
| an assistant `TransportAdapter` | §13 non-goal. A transport participates in the assistant's band routing and inherits the band-LOW capture-acknowledgement shaping (`canonicalizeSuccessfulCaptureResponse`). That path is precisely what R-109-UX21 forbids on every MCP surface. Registering MCP there would make "saved as an idea" reachable **by construction** and demote a structural invariant to a developer-discipline invariant. |
| a connector | Connectors are **ingress** — they pull foreign artifacts *into* the graph and are the surface that fails to persist `artifacts.source_ref` (BUG-019-003 / F-109-001). MCP is **egress** of projections *out of* the graph. Opposite direction, opposite trust posture, opposite failure modes. |
| a second ingress API | §1: the problem is not "add an API". A foreign tool-calling model is a different consumer class than a first-party client, with different authorization (D3), different content rules (D2), and different enumeration semantics (SCN-109-004). |

**Domain-service consumption (P-SVC).** Every tool handler resolves through an injected
domain-service interface. **No internal HTTP hop.** Rejected explicitly:

- *Internal HTTP call to `internal/api` handlers.* Rejected on three independent grounds:
  (i) it re-enters `bearerAuthMiddleware`, which is exactly the token-passthrough D3
  forbids; (ii) it forces MCP through `SearchResult`, which carries no `source_id`, making
  the §9 source-level egress filter unimplementable (F-109-006); (iii) it doubles the
  serialization boundary and gives `content_raw` a second place to leak from.
- *A separate MCP process on its own port.* Rejected: it needs its own TLS termination, its
  own reverse-proxy fragment, its own health/readiness surface, and its own deploy unit —
  four new operational surfaces for zero isolation benefit, since it would still read the
  same Postgres with the same credentials.

**Consequence for spec 108.** Spec 108 design §7's decoupling holds symmetrically: the
MCP transport is not registered in `internal/api/router.go`, so it never traverses 108's
`corpus:read` middleware chain, and §4 below declines to consume 108's grant.

---

## 2. Package Layout

New tree under `internal/mcp/`. One responsibility per sub-package.

| Package | Responsibility (one line) |
|---|---|
| `internal/mcp/capability` | The **export registry**: the `Descriptor` type (§3) and the explicit, hand-maintained manifest of the §8.2 toolsets. Sole source of truth for what `/mcp` exports, and the sole place the `spec.md` D5 wire names enter the program. |
| `internal/mcp/authz` | Audience verification, the toolset ∩ scope ∩ data-scope ∩ egress intersection (§5), and the scope-minimized `WWW-Authenticate` challenge. |
| `internal/mcp/projection` | The §9 Projection Contract type and its constructors, plus the single `smackerel://` resource-URI constructor (§7). Structurally incapable of carrying raw content or a PII-bearing URI. |
| `internal/mcp/refusal` | The seven-token closed vocabulary (§9A.1) and the four-field envelope (§9A.2). |
| `internal/mcp/audit` | The append-only MCP ledger writer (§9 here / §11 spec) and the `smackerel_mcp_*` metric family. |
| `internal/mcp/transport` | JSON-RPC over stateless Streamable HTTP: `initialize`, `tools/list`, `tools/call`, `elicitation/create`; emits `resource_link` entries beside the typed payload and registers **no** `resources/list` (§9B.2); the §12 protocol-error-vs-tool-error split; mounting at `/mcp` in `cmd/core` wiring. |

**This is a NEW registry. It MUST NOT extend, wrap, or pass through:**

- **`internal/agent`** — F-109-005 is dispositive. `agent.All()` contains 7 `noop_*`
  bookkeeping tools and 12 handlers in `internal/recommendation/tools/register.go` that
  unconditionally return `{"ok":true}` against a bare `{"type":"object"}` schema. Any
  passthrough — even a filtered one — makes the MCP surface a function of a registry whose
  growth is governed by a different contract, so a nineteenth-plus stub added tomorrow
  ships to foreign agents by default. The failure mode is **fail-open**, which is the wrong
  default for an external boundary (SCN-109-004). `agent.Outcome` is still consumed (§12
  mapping) — that is a *result* type, not a *registry*.
- **`internal/assistant/openknowledge`** — that is the assistant's band-high `/ask` path
  with its own refusal taxonomy (`ErrNoGroundedAnswer`, `StatusUnavailable`) and its own
  citation contract. Reusing it would import a second refusal vocabulary into a surface
  whose vocabulary is closed at seven tokens (R-109-UX1), and would create exactly the
  "parallel refusal-shaping path" the assistant honesty invariant already forbids.

The relationship is **consume, never inherit**: `internal/mcp` imports domain services and
`agent.Outcome`; nothing in `internal/agent` or `internal/assistant` imports
`internal/mcp`, so no assistant change can alter the MCP export surface.

---

## 3. Capability Descriptor Schema

One exported capability is one `Descriptor`. Shape:

```go
package capability

type Descriptor struct {
    ID             ToolID        // public wire name from spec.md D5 Tool Inventory, e.g. "smackerel_recall" — never renamed
    Toolset        ToolsetID     // §8.2 bundle; the grantable unit
    Input          Schema        // typed JSON Schema, closed; no bare {"type":"object"}
    Output         Schema        // typed; memory-read outputs are []projection.Artifact
    RequiredScopes []Scope       // credential scope claims that must all be present
    RequiredData   []DataScope   // §4: memory-read declares {DataScopeCorpus}
    Egress         EgressClass   // minimum class this tool may run at
    Annotations    Annotations   // four orthogonal MCP hints, declared explicitly
    Provenance     ProvenanceReq // ProvenanceRequired forces the §9 retrieval fields
    Readiness      func(context.Context) Readiness
    Timeout        time.Duration // per-tool; no global default
    Handler        func(context.Context, Invocation) (Result, error)
}

type Annotations struct {
    ReadOnly    bool
    Destructive bool
    Idempotent  bool
    OpenWorld   bool
}

type Readiness struct {
    Available bool
    Reason    string // non-empty iff !Available; e.g. the BUG-019-003 reason
}
```

**Why the hints CANNOT be derived from `agent.SideEffectClass`.** Its closed set is
`read | write | external` — three values against four orthogonal booleans (sixteen
reachable states). The mismatch is not theoretical; two concrete collisions from §8.2:

- `memory.propose_capture` is `write` in `SideEffectClass`, but its correct MCP hints are
  `readOnly=false, destructive=false, idempotent=true, openWorld=false`. It is
  **non-destructive** because it only proposes — `confirm.Machine.Confirm` is what commits
  — and **idempotent** because of single-flight (§8). A three-value enum cannot say
  "writes, but neither destroys nor duplicates".
- `memory.search` and `context.get_daily_brief` are both `read`, yet `get_daily_brief` is
  `idempotent=true` while `memory.search` is `idempotent=false` across time, because
  `lifecycle_state` is a first-class Projection field and lifecycle *moves* (Product
  Principle 3, §16). Same `SideEffectClass`, different hints.

Hence F-109-005's resolution: the manifest is explicit. Per the MCP specification, clients
treat annotations as **untrusted hints**, so server-side enforcement (P-AUTHZ, P-EGRESS,
P-PROJ) is mandatory regardless — annotations are a courtesy to the client, never a control.

**`Readiness` is why `hospitality-read` can be specified without being served.** Its
resolver returns `{Available: false, Reason: "<BUG-019-003 reason>"}`. The descriptor is
therefore omitted from `tools/list` and returns method-not-found on invocation (client sees
Case A), while the **operator** surface reports `capability-unavailable` with the reason
(R-109-UX13, SCN-109-012). Availability is a server-state predicate, deliberately **not** a
credential predicate — see the §5 determinism caveat.

**`ProvenanceReq` is how R-109-UX15 stops being a checklist item.** A descriptor marked
`ProvenanceRequired` is refused at manifest-registration time if its `Output` schema does
not include all five §9 retrieval fields, and its results are encoded through a projection
constructor that cannot be built without a `routing.StrategySelection`. Field *absence* can
therefore never be the way a client infers "no fallback".

**Where tool names are registered.** `Descriptor.ID` is the **public wire name** — the
string a client pins in its MCP config and reads back from `tools/list`. The authoritative
list is `spec.md` D5's Tool Inventory; `internal/mcp/capability` is the single place those
names enter the program, as literals on the manifest slice. Registration is validating, not
merely declarative: it refuses a name that is duplicated, that falls outside T1's
`A-Za-z0-9_-.` / 1–128-character rule, or that is **absent from the D5 inventory** — so a
name cannot be introduced by writing a handler, only by amending the spec. Nothing derives a
name from `agent.All()`, from a struct-field name, or from a map key, which is what keeps the
wire surface a reviewed artifact rather than an emergent one. The manifest is a **slice**,
and its order is D5 T2's `(toolset ordinal, name ascending)` — the same fact §5's determinism
rule depends on, so `tools/list` ordering is a property of the registry rather than of
iteration.

## Capability Foundation

`Descriptor` + the registry + `authz` + `projection` + `refusal` + `audit` form the
foundation. Foundation-owned, identical for every capability: audience verification,
grant intersection, egress gating, readiness resolution, projection encoding, the
seven-token vocabulary, the four-field envelope, the `agent.Outcome` → `isError` mapping,
timeout application, and the audit row. A tool author writes a `Handler` and a `Descriptor`
row; the policies of §8.1 (P-PROJ … P-PULL) are applied by the foundation, not re-asserted
per tool.

## Concrete Implementations

| Capability | Backing domain service | Notes |
|---|---|---|
| `context.get_daily_brief`, `context.get_active_topics` | synthesis / lifecycle services | default-on, local, no data scope |
| `context.get_server_context` | capability registry + authorizer (foundation) | default-on, local; pure function of `(manifest, credential, readiness)` — real per-credential state, never a constant |
| `memory.search`, `memory.get_artifact_projection` | `internal/retrieval/routing.Executor` (§6) | requires `DataScopeCorpus` (§4) |
| `person.get_context` | person/entity service | opt-in grant |
| `graph.get_neighbors`, `graph.get_topic` | knowledge-graph service | opt-in grant |
| `hospitality.get_guest_context` | hospitality projection | registered, `Readiness.Available=false` |
| `memory.propose_capture` | `internal/assistant/confirm.Machine` (§8) | deferred toolset, designed now |

### Variation Axes

The foundation above is fixed; every concrete capability differs from every other along
these six axes and **only** these six. A new capability is added by choosing a point on
each axis and writing a `Handler` + `Descriptor` row — never by extending the foundation.

- **Axis 1 — Toolset** — the grantable bundle a capability belongs to: `context`,
  `memory-read`, `person-context`, `graph-read`, `hospitality-read`, or the deferred
  `memory-write`. The toolset, not the tool, is the unit of operator grant and the unit of
  the gate-2 authorization decision (§5).
- **Axis 2 — Backing domain service** — routing executor vs graph service vs person service
  vs confirm machine. Each has a different call signature and a different provenance story.
- **Axis 3 — Authorization surface** — toolset grant only (`context`) vs toolset + data
  scope (`memory-read`) vs toolset + explicit operator grant (`graph-read`,
  `person-context`).
- **Axis 4 — Client egress class** — every capability declares its minimum class;
  `remote-inference` is a per-client grant, never a per-tool default (D1). This axis varies
  per **client**, not per tool, which is why it is enforced at gate 4 rather than baked
  into the descriptor.
- **Axis 5 — Retrieval strategy** — for capabilities that read the corpus, the strategy is
  chosen at request time by `routing.StrategySelection` (§6) rather than fixed by the
  descriptor, and the selection (`Strategy`, `Reason`, `FellBack`, `ContractKnown`,
  `TraceToken()`) is carried into the projection as the six provenance fields. Capabilities
  that do not read the corpus have no point on this axis.
- **Axis 6 — Side-effect posture and readiness** — read-only projection tools vs the
  propose/confirm write tool, which alone needs the `InputRequiredResult` round trip (§8);
  and always-available vs finding-blocked (`hospitality-read` under BUG-019-003), which
  changes discovery output without changing the manifest.

---

## 4. Resolution of UX-F-001 — the corpus grant

**The gap (restated).** D5 and §8.2 gate `memory-read` on "credential **+ corpus grant**",
and UC-109-003's preconditions repeat it. But §8.1's `Grant` primitive binds an `MCPClient`
to a `Toolset` and, separately, to an `EgressClass` — there is no third axis. Meanwhile D3
and §14's decoupling note state that spec 109 does not depend on spec 108, which is the
spec that owns `corpus:read`.

**Decision: (a) — MCP owns its own corpus-read grant, carried in its own credential.**

Concretely, §8.1's `Grant` primitive gains a third **kind**, not a second grant system:

```
GrantKind ∈ { toolset, egress-class, data-scope }
DataScope ∈ { corpus }          // exactly one member today
```

`memory-read`'s descriptors declare `RequiredData: [corpus]`. The MCP authorizer resolves
`§10`'s intersection with one added term:

```
effective_tools = tools(granted_toolsets)
                ∩ tools(credential_scope)
                ∩ tools(granted_data_scopes)
                ∩ tools(allowed_egress_classes(client))
```

**Why (a) and not (b) — four reasons, each sufficient.**

1. **D3 is not survivable under (b).** Spec 108's `corpus:read` is a scope claim on the
   **legacy first-party bearer**, evaluated by `auth.RequireScope` inside
   `internal/api/router.go`. For MCP to consume it, MCP would have to read authorization
   state out of a credential model whose issuance path it does not own — which is the
   substance, if not the letter, of "MCP servers MUST NOT accept any tokens that were not
   explicitly issued for the MCP server."
2. **Expressiveness mismatch that cannot be patched.** Spec 108's grant mechanism is a
   **token rotation** per principal (F-108-GRANT-MECHANISM-01). MCP grants are **per
   client** — that is the entire shape of §8.1's `Grant` and of D1's per-client egress
   model. One operator will run several MCP clients under one identity; a principal-level
   grant cannot express "client A may read the corpus, client B may not". Option (b) does
   not merely couple the specs, it loses a distinction the product requires.
3. **(b) imports a rollout stage into a security boundary.** Spec 108 ships OBSERVE →
   ENFORCE behind a fail-loud stage flag; under OBSERVE, `corpus:read` is evaluated but
   **not enforced**. An MCP client consuming that grant would read the entire corpus with
   no grant at all for the whole observation window — a silent hole created purely by
   reuse. D3's stated payoff is that MCP is "secure-by-construction on day one"; (b)
   forfeits it.
4. **Blast radius is wrong under (b).** Revoking corpus access from one misbehaving MCP
   client would require rotating the operator's principal token, simultaneously breaking
   the PWA, the browser extension, and the Telegram bridge (spec 108 design §5). Under (a),
   revocation is a single MCP registry row and is effective on the client's next request.

**Rejected alternative, recorded:** folding the corpus grant *into* the `memory-read`
toolset grant (i.e. declaring D5's "+ corpus grant" redundant). Rejected because it
silently deletes a gate the spec states twice, and because a future second data scope
(e.g. attachments, photos) would then have no axis to attach to — the model would have to
be widened later under pressure rather than now under design.

**Consequence for OF-2.** OF-2's observable outcomes change in exactly two places, and this
is the operator-visible cost of (a):

- **OF-2 step 1 is no longer sufficient for `memory-read`.** Granting the toolset to a
  client that lacks the `corpus` data-scope grant records the toolset grant, and the
  client's next `tools/list` **still omits** `memory.*`. The operator surface must therefore
  show the *effective* result, not the grant they just made — otherwise the operator gets
  the same "I granted it and nothing happened" confusion that OF-2 step 2 already handles
  for `hospitality-read`. OF-2 needs a `corpus` data-scope grant/revoke step alongside its
  toolset step.
- **OF-2 step 4's invariant is preserved but only conditionally.** "The operator's effective
  toolset list is identical to what the client's `tools/list` returns" holds **only if** the
  operator-side computation runs the same four-term intersection above, including the
  data-scope term. Computing it from toolset grants alone would break the invariant on
  day one.

This is recorded here as a design consequence and routed to `bubbles.plan` for the §9A.3
OF-2 amendment; **spec.md is not edited by this design** (mode `product-to-planning`;
`spec.md` is `bubbles.analyst`-owned, so amending it is that owner's call, not this design's).

**Refusal token emitted on a corpus denial: `unauthorized-toolset`.**

This is not a compromise, it is the required answer. R-109-UX1 closes the vocabulary at
seven tokens and forbids coining a new one at a call site; a new disposition is added by
amending §9A.1's table, which this design may not do. But more importantly, a *distinct*
client-visible corpus token would itself be an **existence oracle**: it would tell a client
"`memory.search` exists and you would be allowed it but for the corpus scope" — exactly what
R-109-UX12 forbids by requiring the withheld and the nonexistent responses to be
byte-identical. A client denied by data scope therefore experiences §9A.4 **Case A**
verbatim: tools omitted from `tools/list`, method-not-found on invocation, no envelope, no
challenge.

The granularity the operator needs is not lost — it lives in the ledger's separate
**`denial_reason`** column (§11), which distinguishes a missing toolset grant from a
missing `corpus` data scope without widening anything the client can observe. Honesty is
owed to the operator, not to the client (R-109-UX13).

---

## 5. Authorization & Egress Enforcement

**Credential verification is independent of `bearerAuthMiddleware` (D3).**
`internal/mcp/authz` exposes `Verify(*http.Request) (Credential, error)` and is wired only
into the `/mcp` mount. It never calls `bearerAuthMiddleware`, never reads a session, and
never consults connection state (SCN-109-011).

**The audience predicate is positive, and that is load-bearing.** The check is
`cred.Audience == mcpAudience` — a required equality, **not** a negative filter such as
`aud != "legacy"`. A token from today's `IssueToken` carries **no** `aud` at all
(F-109-004), so it satisfies every negative filter and fails only a positive one. Fail-closed
is the only correct default for an external boundary, and §9's T1 is written specifically to
kill the negative-form implementation. Wiring `Audience` into `IssueToken` and verifying it
at `/mcp` is a conformance blocker in scope for this feature (§10, F-109-004).

**Gate order — fixed, and the order is itself a security property:**

| # | Gate | Failure disposition | Client sees |
|---|---|---|---|
| 1 | Audience + signature | HTTP rejection + `WWW-Authenticate` | challenge; no tool executes (SCN-109-002) |
| 2 | Toolset ∩ credential scope ∩ data scope (§4) | `unauthorized-toolset` | tool absent from list; method-not-found on invoke; **no envelope** |
| 3 | Readiness | `capability-unavailable` | identical to gate 2 (Case B ≡ Case A for the client) |
| 4 | Egress class | `unauthorized-egress-class` | §9A.2 envelope, scope-minimized |
| 5 | Dispatch to handler | `execution-failed` / `degraded-fallback` | §12 mapping |

**Why egress is gate 4 and not gate 2.** Gate 4 is the only gate that produces a
client-visible envelope, because §9A.2's table permits an envelope only when the tool is
*legitimately visible to this credential* and denied on a later gate. If egress ran before
the toolset/data-scope gate, a client with no `memory-read` grant at all could receive an
`unauthorized-egress-class` envelope naming `memory.search` — leaking the tool's existence
and violating R-109-UX12. Gates 2 and 3 must therefore fully precede gate 4.

**Why egress is checked before dispatch and before any read.** A denied `remote-inference`
client must cause **zero** corpus retrieval; the denial cannot be a post-filter over results
that were already materialized. §9's T6 asserts this with a counting executor, not by
inspecting the response.

**Egress semantics.** Each `MCPClient` carries an operator-declared `inference_locality`.
`remote-inference` is denied unless a per-client grant exists; when granted, every
invocation writes a remote-egress row and increments
`smackerel_mcp_remote_egress_total` (D1, SCN-109-005/006). Smackerel **cannot verify** the
declaration — the control is administrative and auditable, not cryptographic, and every
document that mentions it must say exactly that (D1, §15).

**`tools/list` filtering.**

```go
func List(cred Credential, now time.Time) []Descriptor   // pure in (manifest, cred, readiness)
```

- **Per-request credential is the only authorization input.** The MCP specification permits
  the tool list to vary by *authorization*; it does not permit it to vary by *connection*.
  The filter therefore reads the credential presented on **this** request and nothing else:
  no session, no connection struct, no per-connection cache, no `initialize`-time snapshot.
  This is also what makes revocation effective on the next request with no reconnect and no
  server push (OF-2 step 3, OF-5 step 2, D4 P-PULL).
- **Determinism.** The manifest is a slice, never a map — Go randomizes map iteration, so a
  `range` over a map is the natural implementation and the natural bug. Output is sorted by
  `(toolset ordinal, ToolID)` with a total order, and serialized from structs with fixed
  field order, so repeated calls with the same credential are **byte-identical**
  (SCN-109-003, P-DETERMINISTIC).
- **Honest caveat.** `Readiness` is a function of *server* state, not of the credential. If
  BUG-019-003 were fixed and `hospitality-read` became available mid-process, the list
  would change for an unchanged credential. The invariant is byte-identity across repeated
  calls *at a given readiness state*; overclaiming absolute immutability would be false, so
  the test (§9 T2) pins readiness explicitly rather than asserting it can never move.

**Scope minimization.** A gate-4 denial returns `WWW-Authenticate` naming **only** the
missing scope (`scope="graph-read"`). The catalog is never published in any response
(§10, R-109-UX14, SCN-109-010). Gates 2 and 3 return no challenge at all.

---

## 6. Read Plane Over Spec 095

**The call.** `memory.search`'s handler is a thin adapter over the injected interface:

```go
Retrieve(ctx, intent.CompiledIntent, routing.RetrievalRequest)
    (routing.RetrievalResult, routing.StrategySelection, error)
```

Flow: natural-language `query` (P2 — vague in, precise out; no exact tags or dates
demanded) → intent compilation to `intent.CompiledIntent` → `RetrievalRequest` carrying the
allowed-source predicate (§7) and the limit → `Retrieve` → §9 projection mapping:

| Projection field | Source |
|---|---|
| `source_kind` | `RetrievalResult.RetrievedSource{Kind}` |
| `retrieval_strategy` | `StrategySelection.Strategy` |
| `retrieval_reason` | `StrategySelection.Reason` |
| `retrieval_fell_back` | `StrategySelection.FellBack` |
| `retrieval_contract_known` | `StrategySelection.ContractKnown` |
| `trace_token` | `StrategySelection.TraceToken()` |

**MCP is spec 095's first live consumer — stated, not hidden (F-109-002).**
`internal/retrieval/routing` is the right substrate (handler-free, injectable, an explicit
`StrategySelection` return), but its request-path integration is deferred as PKT-095-A /
PKT-095-B / PKT-095-C and its live end-to-end coverage is deferred as F-095-E2E-LIVE. This
design does **not** assume spec 095 was proven in production.

**Mitigation — three parts, all funded inside this feature (§18 item 6):**

1. **Own the seam; do not wait on the packets.** MCP depends on the `Executor` *interface*,
   and this feature ships its own concrete `cmd/core` wiring for the MCP path. PKT-095-A/B/C
   remain 095's work for 095's surfaces; if they land later they replace that wiring. They
   are explicitly **not** a prerequisite for 109, so 109 does not inherit 095's schedule.
2. **Fund the live coverage here.** An `e2e-api` test on the ephemeral stack (real Postgres,
   real retrieval path) exercises `Retrieve` end-to-end through `/mcp` (§9 T7). This
   discharges F-095-E2E-LIVE **for the MCP path specifically**; it is not a claim that 095
   is generally proven, and no document may state otherwise.
3. **Degrade honestly rather than hard-fail.** A fallback is `degraded-fallback` with
   `isError=false` (R-109-UX17, UC-109-004). A genuine `error` return is `execution-failed`
   with `isError=true` (R-109-UX19). The layer never renders an unknown routing state as an
   empty result set (R-109-UX20) — "no results found" over a provider error is a forbidden
   rendering.

**Rejected alternative.** Calling `internal/api/search.go`'s `SearchHandler` internally —
rejected on the three grounds in §1, of which F-109-006 (`SearchResult` carries no
`source_id`) is by itself disqualifying.

---

## 7. Projection Enforcement

The D2 invariant — `content_raw` is never returned over `/mcp`, in any toolset, at any
egress class — is enforced **structurally**. Developer discipline is not a control.

**Mechanism 1 (primary): a type that cannot express raw content.**

```go
package projection

type Artifact struct {                 // closed: exactly the §9 returned fields
    ArtifactID  string   `json:"artifact_id"`
    Kind        string   `json:"kind"`
    Title       string   `json:"title"`
    Summary     string   `json:"summary"`      // extraction output, never the body
    Entities    []Entity `json:"entities"`
    Topics      []Topic  `json:"topics"`
    // … lifecycle_state, created_at, observed_at, the six retrieval fields, deep_link
}

func FromRetrieved(r routing.RetrievedArtifact, sel routing.StrategySelection) Artifact
```

Four properties make it structural, not aspirational:

- **No open field.** No `map[string]any`, no `json.RawMessage`, no `any`, no `Extra`, no
  `json:",inline"`.
- **No embedding of a domain type.** The constructor reads *named* fields and assigns them.
  A field added to the domain artifact row tomorrow therefore cannot appear on the wire —
  the compiler does not carry it across, and no reflection copies it.
- **The constructor requires `StrategySelection`.** A memory-read projection literally
  cannot be built without provenance, which is what makes R-109-UX15's "mandatory and
  non-omittable" true rather than reviewed.
- **`external_ref` is not a field.** It is absent from the type, so a reconstructed or
  guessed external reference is not merely forbidden by policy — there is nowhere to put it
  (§9 external-reference honesty rule, F-109-001).

Emitting raw content therefore requires **adding a field to this type**: a visible,
reviewable, test-breaking change (§9 T4 asserts field-set equality in both directions), not
an accident in a handler.

**Mechanism 2: one typed wire path.** Memory-read handler signatures return
`[]projection.Artifact`, and `transport` marshals that type — `Result.Content` for
memory-read is typed, not `any`. A handler cannot physically return a domain row.

**Where `resource_link` is emitted (spec §9B).** A resource link is a **sibling content
entry** to the typed payload, never a Projection field, so mechanism 1's closed type is
untouched. The emission path is deliberately narrow:

1. `internal/mcp/projection` owns the **only** URI constructor —
   `func ResourceURI(kind ResourceKind, id OpaqueID) string`. Its `id` parameter type is
   `OpaqueID`, a newtype over the internal surrogate key, so a display name, an email
   address, a meeting title, or a raw external identifier is a **compile error**, not a
   review finding. That is the same structural stance mechanism 1 takes for `content_raw`:
   the constraint is in the type, and R-109-RES3 is therefore enforced rather than
   remembered. `ResourceKind` is a closed enum over exactly the five R-109-RES2 templates,
   so an unlisted URI shape has no way to be built.
2. `FromRetrieved` fills the Projection and, from the **same** already-opaque
   `ArtifactID` it just assigned, derives the link. No second lookup, no join back to a
   domain row — nothing PII-bearing is ever in scope at the point the URI is built.
3. `internal/mcp/transport` appends those links to the tool result beside the typed
   `[]projection.Artifact`. It **registers no `resources/list` handler** and advertises no
   resource-listing capability during `initialize` (§9B.2), so the enumeration oracle is
   absent by construction rather than denied at request time.
4. Following a link re-enters `tools/call` through all five §5 gates. A link is a pointer,
   never a bypass, and it never dereferences to raw content — the thing it resolves to is
   the same `projection.Artifact` mechanism 1 already closed.

This is why resource links **strengthen** P-PROJ instead of straining it: returning a
reference is what removes the pressure to inline a body in the first place.

**Mechanism 3 (detector, not control): the marshal-time guard.** A guard scans the
serialized payload for a `content_raw` key and increments
`smackerel_mcp_projection_raw_content_blocked_total` (§11), which **MUST always be 0**;
non-zero is a P0. This exists to detect a breach of mechanism 1, not to substitute for it.
If it ever fires, the correct response is to fix the type, not to rely on the guard.

**Source-level egress filter — SQL, never a post-filter.**

Two independent reasons, per §9 and F-109-006:

1. **It is not expressible as a post-filter today.** `internal/api/search.go`'s
   `SearchFilters` has no `source_id` and `SearchResult` does not carry one, so there is
   nothing to filter on after the fact.
2. **A post-filter would still be wrong even after adding `source_id`.** Filtering after
   ranking and after `LIMIT` silently shrinks a page, so a denied source leaks its existence
   through result counts and pagination shape — the same existence-oracle failure spec 108
   design §3 rules out for 403-vs-404.

Therefore the domain service backing `memory.search` accepts an allowed-source predicate and
applies it in the `WHERE` clause, **before** ranking and **before** `LIMIT`. §9 states this
explicitly as "a required capability of the service, not of the MCP layer"; this design
honors that boundary — `internal/mcp` passes the predicate, it does not implement the filter.

---

## 8. Write Plane (deferred toolset, designed now)

`memory-write` is off (D5) and ships later; the mapping is settled now so the deferral does
not become a redesign.

**Mapping onto the existing `internal/assistant/confirm.Machine`:**

| MCP | `confirm.Machine` |
|---|---|
| `tools/call memory.propose_capture` | `Propose(ctx, Proposal{UserID, Transport: "mcp", …}) → (confirm_ref, ExpiresAt)` |
| `InputRequiredResult` / `elicitation/create` carrying `requestState` | the round trip that carries `confirm_ref` back to the operator |
| client returns the operator's decision with the same `requestState` | `Confirm(ctx, userID, "mcp", confirm_ref)` or `Discard(…)` |
| no decision before `ExpiresAt` | `SweepTimeouts` discards; a later confirm fails honestly (SCN-109-014) |

`requestState` carries **only** the `confirm_ref`. The pending payload stays server-side —
the client is a courier for a decision, never a holder of the write. That also means a
compromised client cannot replay a payload, only a reference that single-flight has already
consumed.

**Consumed as a DOMAIN SERVICE; `transport="mcp"` is a keying discriminator — explicitly
NOT an assistant transport registration.**

- `transport` already exists as a discriminator on the single-flight key
  `(user_id, transport, confirm_ref)` (UC-109-006 postcondition). Using `"mcp"` gives MCP
  proposals their own namespace, so an MCP confirm can never collide with a Telegram or PWA
  confirm for the same user.
- Registering MCP as a fourth `TransportAdapter` is a §13 non-goal and would be actively
  harmful: a transport participates in assistant band routing and inherits
  `canonicalizeSuccessfulCaptureResponse`'s band-LOW capture-acknowledgement path. That is
  the "saved as an idea" response class R-109-UX21 forbids on **every** MCP surface, at
  every toolset, at every egress class. Consuming the state machine directly takes the
  lifecycle **without** the response-shaping layer, so the forbidden string is unreachable
  by construction rather than by rule.

**Single-flight gives lost-response-retry safety for free.** MCP is stateless Streamable
HTTP: a client that loses the response to a `Confirm` cannot know whether the write
committed, and its only recourse is to retry. Because `Confirm` is race-safe single-flight
on `(user_id, "mcp", confirm_ref)`, the retry observes the already-confirmed state and
returns the same disposition instead of double-writing. Consequences:

- `memory.propose_capture`'s `idempotentHint` is **true** despite `readOnlyHint` being
  false (§3) — the exact combination `SideEffectClass` cannot express.
- **No MCP-specific idempotency-key mechanism is needed.** Inventing one would be a second
  dedupe system layered over a working one.

**Audit.** Write-path invocations write the §9 MCP ledger row **and** additionally reuse
`confirm.Machine`'s existing audit writer, so propose / confirm / discard / timeout land on
the same trail as every other transport (§11). MCP forks the *invocation* ledger, never the
*write* trail — otherwise a write's history would depend on which client made it.

---

## 9. Audit & Test Design

### 9.1 The MCP audit record

Its own append-only table. One row per invocation, plus rows for grant and revocation
decisions (R-109-UX9, OF-3 step 3, OF-5 step 1).

| Field | Notes |
|---|---|
| `ts` | the ledger owns time; the §9A.2 envelope must not carry timestamps (R-109-UX6) |
| `client_id` | the `MCPClient` id; a re-issued identity is a **new** id (OF-5 step 4) |
| `toolset` | §8.2 bundle |
| `tool` | stable `ToolID` |
| `outcome` | the closed seven-token set of §9A.1 — no synonyms, no case variants (R-109-UX1) |
| `egress_class` | `local-inference` \| `remote-inference` — a separate column, never folded into `outcome` |
| `denial_reason` | nullable; carries the §4 corpus-vs-toolset granularity the client must not see |
| `trace_token` | matches the token the client received, so a bad answer resolves to a strategy selection (OF-4 step 5) |

**Why not `agent_traces`.** That table represents **scenario / LLM invocations** from the
assistant's band pipeline. An MCP path has no band, no scenario id, and no model call of its
own — it is a tool invocation against a domain service. Reusing it would (i) pollute
assistant analytics with non-assistant traffic, (ii) force the seven-token vocabulary into a
schema built around a different outcome enum, and (iii) make OF-4's question — *"did my
corpus ever go to a third-party model?"* — a join across unrelated semantics instead of one
filter on `egress_class`. R-109-UX10 requires the ledger to answer with a row set, not a
computed verdict; that only works if the rows are homogeneous.

**Refusal is never silent (R-109-UX23):** every disposition other than `authorized` writes a
row. A request that produced no row and no response is an incident.

### 9.2 Test design (all adversarial; every one fails against the naive implementation)

| # | Category | What it proves — and why it is not tautological |
|---|---|---|
| **T1** | `unit` | **Legacy bearer rejected at `/mcp`** (SCN-109-002). Fixture is a token from the real `IssueToken` path, which sets **no** `aud`. A negative-form check (`aud != …`) admits it; only the positive equality rejects it. The test fails against the negative-form implementation, which is the one a developer writes first. |
| **T2** | `unit` | **`tools/list` connection-invariance + deterministic order** (SCN-109-003). Same credential across repeated calls and across distinct connections → byte-identical JSON. Adversarial fixture: a manifest large enough that Go's randomized map iteration reorders it, so any `range`-over-map implementation fails. Readiness is pinned so the §5 caveat is not silently relied on. |
| **T3** | `integration` | **Grant-varied discovery with no existence leak** (R-109-UX12, SCN-109-003/010). Client A holds `context` only; B holds `context` + `memory-read` + `corpus`. A's list omits `memory.*`; A invoking `memory.search` returns a JSON-RPC method-not-found **byte-identical** to A invoking `totally.nonexistent`. Adversarial because the natural implementation returns a distinct "unauthorized" error, which passes a naive "A is denied" assertion and fails this one. |
| **T4** | `unit` | **Raw-content escape asserted on the projection TYPE.** Reflect over `projection.Artifact`: field set equals §9's closed list by **set equality in both directions**; no field is `map[string]any` / `json.RawMessage` / `any` / an embedded struct; no field named `content_raw`; no `external_ref`. Adversarial because it fails when someone *adds* a field — which no sample-response test can detect. |
| **T5** | `integration` | **Injection containment** (SCN-109-001, D2 reason 2). Seed an artifact whose `content_raw` is a prompt-injection payload carrying a rare marker. Query so it ranks first. Assert the marker appears nowhere in the response bytes and that `summary` / `title` are extraction outputs, not substrings of the body. |
| **T6** | `integration` | **Egress-class enforcement, gated before the read** (SCN-109-005/006). Declared-`remote` client with no grant → `unauthorized-egress-class`, §9A.2 envelope, zero projections, `smackerel_mcp_tool_denied_total` increments, **and a spy executor records zero `Retrieve` calls** — proving gate 4 precedes dispatch rather than filtering results. Then grant it: projection returns, exactly one `egress_class=remote-inference` row, and `smackerel_mcp_remote_egress_total` equals the ledger row count (OF-4 step 3). |
| **T7** | `e2e-api` | **Spec 095 honest degradation, live** (F-109-002 mitigation 2; F-095-E2E-LIVE discharge for this path). Real stack, real Postgres. Force a fallback: `retrieval_fell_back=true`, `retrieval_reason` non-empty, `isError=false`, `outcome=degraded-fallback`. Then force a routing error: `isError=true`, `outcome=execution-failed`, and the response is **not** an empty result set. Adversarial because "no results found" is the natural error rendering and passes a shape-only test (R-109-UX20). |
| **T8** | `e2e-api` | **Byte-level `content_raw` scan** across every enabled toolset under both egress classes; `smackerel_mcp_projection_raw_content_blocked_total == 0` (§2 Success Signal). Complements T4 — T4 guards the type, T8 guards the wire. |
| **T9** | `unit` | **`hospitality-read` honest unavailability** (SCN-109-012, F-109-001). Grant it: the grant is recorded, `tools/list` omits `hospitality.*`, invocation returns method-not-found, the **operator** surface reports `capability-unavailable` with the BUG-019-003 reason, and no projection carries `external_ref`. Adversarial because "present but returning an error" is the easy implementation and violates §12's never-present-but-fake rule. |
| **T10** | `unit` | **Failure honesty + closed vocabulary** (R-109-UX19/21, §12). Table across the full `agent.Outcome` map including the `OutcomeInputSchemaViolation` → JSON-RPC-error carve-out (SCN-109-009); plus a mechanical scan asserting `"saved as an idea"` and every §9A.1 banned word appear on no MCP surface. Mirrors the existing `internal/assistant/contracts/refusal_test.go` precedent. |
| **T11** | `unit` | **Write single-flight retry safety** (SCN-109-013/014). Propose → Confirm → Confirm again on the same `(user_id,"mcp",confirm_ref)`: exactly one write, identical disposition both times. Then a `SweepTimeouts`-discarded ref → `refused`, `isError=true`, no write. Adversarial because a non-single-flight implementation passes the first confirm and double-writes on the retry. |
| **T12** | `unit` | **Corpus data-scope gate** (§4). A client granted `memory-read` but **not** the `corpus` data scope experiences Case A exactly (omitted + method-not-found + no envelope), while the ledger row carries `outcome=unauthorized-toolset` with a `denial_reason` naming the corpus scope. Adversarial because the natural implementation either ignores the data scope entirely or emits a distinct client-visible token. |

**Test isolation.** Every live-category test (`integration`, `e2e-api`) runs on the ephemeral
test stack with disposable storage and emits telemetry tagged `env=test*` only — no writes to
prod monitoring, prod backup paths, or knb manifests (G115).

---

## Complexity Tracking

| Deviation from the simplest viable approach | Simpler alternative considered | Why rejected |
|---|---|---|
| A new `internal/mcp/` registry instead of reusing `internal/agent` | Filter `agent.All()` down to real tools | Fail-open: F-109-005's 19 non-capabilities grow under a different contract, so a new stub ships to foreign agents by default (SCN-109-004). |
| A third `Grant` kind (`data-scope`) rather than two | Fold the corpus grant into the `memory-read` toolset grant | Silently deletes a gate D5 and §8.2 both state, and leaves a future second data scope no axis to attach to (§4). |
| A dedicated audit table rather than `agent_traces` | Reuse the existing trace table | Different semantics (no band, no scenario, no LLM turn) and it turns OF-4's one-filter question into a cross-semantic join (§9.1). |
| Consuming `confirm.Machine` directly rather than registering a transport | Register MCP as a fourth `TransportAdapter` | Inherits the band-LOW capture-acknowledgement path, making the R-109-UX21 forbidden string reachable by construction (§8). |
| A closed projection struct with an explicit constructor | Marshal the domain row and omit `content_raw` via a struct tag | Omission-by-tag is a review property; a field added upstream tomorrow ships silently. The closed type makes the leak a compile/test failure (§7). |
