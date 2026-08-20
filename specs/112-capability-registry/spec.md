# Feature: 112 Capability Registry

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** `bubbles.analyst`
**Diagnostic findings addressed:** `D17`, `D18`
**Plan of record:** [`docs/Product_Delivery_Plan.md`](../../docs/Product_Delivery_Plan.md) — Pillar C, Extended scenarios

---

## 1. Problem Statement

Smackerel knows how to do far more than a user can ask it to do, and the gap is not a
capability gap — it is a **reachability** gap.

Twenty-seven scenario contracts are built and shipping in `config/prompt_contracts/`.
Five are declared user-facing in `config/assistant/scenarios.yaml`. The rest are working
machinery with no front door: a user who does not already know the exact page URL or the
exact slash token cannot get to them by asking.

The deeper defect is *why* that gap persists and keeps re-opening. **Reachability is not
described in one place.** It is spread across four independently hand-maintained
inventories, each of which is edited by a different change for a different reason:

| # | Inventory | Location | Maintained by hand |
|---|---|---|---|
| 1 | Assistant intents | [`config/assistant/scenarios.yaml`](../../config/assistant/scenarios.yaml) | yes |
| 2 | Navigation | [`internal/web/appshell.go`](../../internal/web/appshell.go) + [`web/pwa/lib/appnav.js`](../../web/pwa/lib/appnav.js) + [`internal/web/templates.go`](../../internal/web/templates.go) | yes, in three files |
| 3 | Slash aliases | [`internal/assistant/shortcuts.go`](../../internal/assistant/shortcuts.go) | yes |
| 4 | MCP tool list | spec 109, planning-only | not yet built |

Nothing reconciles them. Adding a capability means remembering four edits in four
languages, and forgetting any one of them is silent — no build fails, no test fails, and
the capability is simply unreachable from that surface.

**This is not hypothetical. All three built inventories have already drifted**, and the
drift is recorded in §3 with file and line references. The assistant registry and the
slash table disagree about what `/ask` does. The slash table ships two commands the
assistant registry states do not exist. The two navigation files disagree about three
entries each, while one of them carries a comment asserting it mirrors the other. Every
one of these is the same failure repeated: a fact about a capability written down more
than once, in places that cannot see each other.

**The registry is half-built, not missing — and that is the most important fact in this
document.** `internal/experience/` already contains a generated catalog
(`catalog.gen.json`, schema `smackerel-product-experience/v1`, 20 declared surfaces) with
navigation, renderer, consumer-inventory, state and mutation projections and a validator.
Every surface in it already carries a `capability_id`. The validator already has a
`KnownCapabilities` seam and already emits an `unknown-capability` violation.

What is missing is the thing those `capability_id` values point at. **There is no
capability record anywhere in the repository.** The catalog holds a foreign key to a table
that does not exist, and the only production-shaped test that supplies the capability
universe derives it *from the catalog being validated* — so the check is circular and
cannot fail.

The work is therefore to **extend the existing catalog from surfaces to capabilities** and
generate the four inventories from it. Building a second registry beside `internal/experience/`
would recreate exactly the duplication this feature exists to remove, and is rejected
explicitly in §10.

---

## 2. Outcome Contract

**Intent:** Every capability the system has is described **once**, in one record, and every
place a user could reach that capability from — natural-language intent, navigation, slash
alias, external tool list — is **generated from that record**. A capability that exists is
either reachable by asking for it, or is explicitly and reasonedly declared not to be.
Adding a capability becomes one edit, and forgetting to expose it becomes impossible rather
than silent.

**Success Signal:** A capability added to the registry appears in the assistant intent set,
the navigation projection, the slash alias table and the MCP tool projection **without any
of those four being edited**; and a capability that is enabled but missing an intent
phrase, an authorization policy, a navigation status or an evaluation case **fails a
coverage check** rather than shipping unreachable or unguarded. The daily digest is present
in the guaranteed cross-surface navigation core on every surface that renders navigation.

**Hard Constraints:**
- The existing `internal/experience/` catalog is **extended**, not duplicated. A second
  parallel registry is a failure of this feature, not an implementation detail of it.
- Every reachability surface becomes a **projection**. A hand-maintained list that survives
  beside its generated projection is a defect, not a transitional state.
- Exposure is **declared, never defaulted**. "Not listed" must stop being a way to be
  invisible; every capability carries an explicit exposure decision and a reason when that
  decision is anything other than user-facing.
- Authorization is **per capability and server-derived**. A capability descriptor names the
  authenticated principal and grants it requires; the client never asserts its own
  authority. This consumes spec 108's grant model and does not redefine it.
- No capability may be surfaced whose authorization requirement cannot be enforced. Raising
  reach without raising the guard alongside it is forbidden.
- The registry carries **identity and policy only** — never user content, never a readiness
  fact, matching the content-free discipline the existing catalog already holds.

**Failure Condition:** The registry ships, the four inventories are still edited by hand
"for now", and the registry becomes a fifth thing to keep in sync — strictly worse than
four. Equally a failure: capabilities are surfaced to natural-language dispatch before the
per-capability authorization boundary is enforceable, so the feature's visible outcome is a
wider attack surface. And equally a failure: the coverage check exists but is satisfiable
by a capability set derived from the very artifact under test, reproducing today's circular
validation in a new location.

---

## 3. Evidence Base (verified 2026-08-04 against the working tree)

Every row below was re-read against the repository before being written here. These are
**current-state source observations, not test results.** They establish that the defects
exist; they prove nothing about a fix.

| # | Observation | Location |
|---|---|---|
| E1 | 27 scenario contracts are built and present | `config/prompt_contracts/` — 27 files |
| E2 | Exactly 5 are declared `user_facing: true`: `retrieval_qa`, `weather_query`, `notification_schedule`, `recipe_search`, `open_knowledge` | `config/assistant/scenarios.yaml:26,34,42,56,72` |
| E3 | 17 of the 27 declare `type: "scenario"`, and **all 17 carry a routable `id:`**. The other 10 declare a pipeline type (`domain-extraction` ×3, `query-augment`, `lint-audit`, `ingest-synthesis`, `drive-folder-context`, `drive-classification`, `digest-assembly`, `cross-source-connection`) and **carry no `id:` at all** | `config/prompt_contracts/*.yaml` |
| E4 | One scenario-typed contract, `e2e_ollama_smoke`, is a test harness by its own description ("invoked by `tests/e2e/agent/happy_path_test.go`") | `config/prompt_contracts/e2e-ollama-smoke-v1.yaml` |
| E5 | Therefore the genuinely unsurfaced dispatchable set is **11**, not 22 — see the sharpening note below | derived from E2, E3, E4 |
| E6 | The generated catalog declares 20 surfaces under schema `smackerel-product-experience/v1`, and **every surface carries a `capability_id`** | `internal/experience/catalog.gen.json` |
| E7 | No capability record exists. In the single source of truth, `capability_id` appears **only as an attribute of a surface** — there is no `capabilities:` block | `config/smackerel.yaml:2698` (`product_experience`), `:2706-2934` (the 20 `capability_id` attributes) |
| E8 | The validator already has a `KnownCapabilities` seam and already emits `unknown-capability`; a nil map **disables** the check | `internal/experience/validator.go:99` and its `RouteInventory` doc comment |
| E9 | The only production-shaped test supplies that universe with `iCapsOf(cat)`, which builds the set **from the catalog under validation** — the membership check is circular and cannot fail | `tests/integration/experience/route_inventory_test.go:92-100`, used at `:276` |
| E10 | The catalog package states in its own doc comment that the handwritten navigation authorities "remain ACTIVE and untouched in this slice — the generated catalog is added **ALONGSIDE** them", with cutover deferred | `internal/experience/catalog.go:10` |
| E11 | The shared server navigation partial carries 6 links (assistant, search, knowledge, cards, notifications, settings). **No digest.** | `internal/web/appshell.go:31` |
| E12 | The PWA navigation carries 8 links (assistant, capture, search, cards, connectors, photos, notifications, settings). **No digest.** Its header comment asserts it "mirrors the server-side `app-shell-nav` partial" | `web/pwa/lib/appnav.js:22` (`ITEMS`) |
| E13 | **Those two have already drifted.** `capture`, `connectors` and `photos` exist only in the PWA list; `knowledge` exists only in the server partial. The "mirrors" comment is false as written | E11 + E12 |
| E14 | The server page shell appends three further links — **digest**, topics, status — *after* including the shared partial. So digest is reachable in the server shell while being absent from the shared core the PWA mirrors | `internal/web/templates.go:79-84`; the route exists at `internal/api/router.go:411` |
| E15 | The slash table maps `/ask` to `open_knowledge`, while the assistant registry assigns `/ask` to `retrieval_qa`. **The same token names two different targets in two files** | `internal/assistant/shortcuts.go:47` vs `config/assistant/scenarios.yaml:25-31` |
| E16 | The slash table ships `/recipe` and `/cook`, while the registry records `recipe_search` with `slash_shortcut: ""` and a comment that the set "stays frozen at `/ask`, `/weather`, `/remind`, `/reset`". The registry states the shortcuts do not exist; the code ships them | `internal/assistant/shortcuts.go:50-51` vs `config/assistant/scenarios.yaml:47-57` |
| E17 | 31 first-party PWA pages exist; **2** load the shared navigation (`assistant.html`, `index.html`). The other 29 render no shared navigation at all | `web/pwa/*.html`, `grep` for `appnav.js` |
| E18 | Spec 108 (corpus grant enforcement), which owns the `corpus:read` boundary this feature's authorization requirement depends on, is `specs_hardened` — **planned, not delivered** — and targets the `next` train | `specs/108-corpus-grant-enforcement/state.json` |
| E19 | Spec 109 (MCP) is `specs_hardened`, targets `next`, and explicitly forbids deriving the tool list by passthrough: "no MCP tool is derived by passthrough from `agent.All()`" | `specs/109-mcp-knowledge-server/state.json`; `specs/109-mcp-knowledge-server/spec.md:80-81` |

### Sharpening of `D17` — the unsurfaced count is 11, not 22

`D17` and the plan of record both state that 22 capabilities have no front door, arriving
at 22 as `27 − 5`. That arithmetic is correct, but **a prompt contract is not a capability**,
and the evidence above shows the subtraction mixes two different units.

Ten of the 27 contracts (E3) are pipeline stages — extraction, query augmentation, ingest
synthesis, digest assembly, quality audit, classification. They declare a pipeline `type`
and carry **no routable id**. They are invoked directly by the service that owns them and
were never dispatchable. They are not capabilities lacking a front door; they are internal
steps that correctly have none. One further contract is a test harness (E4).

The honest figure is therefore **11 dispatchable capabilities that carry a routable id, are
not test-only, and have no declared intent**: `alert_timing_evaluate`,
`annotation_classify`, `expertise_classify`, `hospitality_concern_evaluate`,
`recommendation_feedback`, `recommendation_reactive`, `recommendation_watch_evaluate`,
`recommendation_why`, `relationship_cooling_evaluate`, `resurface_evaluate`,
`retrieval_evergreen`.

This **narrows the count and strengthens the finding.** `D17`'s own caveat already says it
"counts declared intents, not observed routing". The important defect was never the size of
the number: it is that **the repository cannot tell you which number is right**, because
nothing records whether a contract is unsurfaced deliberately or by omission. Eleven
capabilities are undeclared, and ten pipeline stages are indistinguishable from them
without reading each file. §8 requires that this become a declared fact rather than an
inference.

### Precision note on `D18`

`D18` states the digest is absent from the guaranteed cross-surface navigation core. E14
confirms this **exactly as stated and no more**: the digest link exists in the server page
shell, so the digest is reachable there; it is absent from the shared partial (E11) and
from the PWA navigation (E12). The finding is about the guaranteed core, not about total
unreachability, and `D18`'s own caveat says so. This spec preserves that distinction and
does not claim the digest is unreachable today.

---

## Domain Capability Model

### 4.1 Primitives

| Primitive | Definition |
|---|---|
| **Capability** | A unit of product function a principal can invoke or receive. The registry's unit of record. Not the same unit as a prompt contract: a capability may bind zero, one, or several contracts. |
| **CapabilityDescriptor** | The single record describing one capability. It is the only place a capability fact is authored. |
| **ExposureClass** | The declared decision about whether a capability is reachable on demand. A closed vocabulary. "Undeclared" is not a member. |
| **ReachabilitySurface** | A place a principal can reach a capability from: the natural-language intent set, the navigation, the alias table, the external tool list. |
| **Projection** | A generated view of the descriptor set onto one reachability surface. Derived, never authored. |
| **AuthorizationRequirement** | The authenticated principal and the grants a capability demands before it will run. |
| **SideEffectClass** | What invoking a capability does to the world: reads, writes, or leaves the system. |
| **ProvenanceRequirement** | Whether the capability's output must cite its sources. |
| **CoverageCheck** | The validation that every enabled capability carries every facet required for it to be reachable and guarded. |

### 4.2 Relationships

- A **Capability** has exactly one **CapabilityDescriptor**. Two records describing one
  capability is the defect this feature removes.
- A **CapabilityDescriptor** declares exactly one **ExposureClass**, one
  **AuthorizationRequirement**, one **SideEffectClass** and one **ProvenanceRequirement**.
- A **CapabilityDescriptor** binds zero or more prompt contracts. The binding is
  many-to-one in both directions and must not be assumed 1:1 (§3, E3).
- Every **ReachabilitySurface** is a **Projection** of the descriptor set. No surface holds
  a capability fact absent from the descriptor.
- The existing catalog's `surface.capability_id` **references a Capability**. Today it
  references nothing (E7); after this feature the reference resolves.
- A **CoverageCheck** evaluates the descriptor set against an **independently derived**
  universe of built capabilities — never against a universe derived from the descriptor set
  itself (E9).

### 4.3 Policies every implementation must obey

- **P1 — One record per capability.** Every capability fact is authored once. A surface
  that carries a capability fact the descriptor does not is a defect.
- **P2 — Every surface is generated.** A hand-maintained inventory that survives beside its
  generated projection is a defect, not a migration phase. Four hand-maintained lists plus
  one registry is worse than four.
- **P3 — Exposure is declared, never defaulted.** Absence from a list must stop being a way
  to be invisible. Any class other than user-facing carries a recorded reason.
- **P4 — Reach never outruns the guard.** A capability may not be surfaced unless its
  authorization requirement is enforceable at the moment of surfacing.
- **P5 — Extend, never duplicate.** The registry extends the existing experience catalog.
  A parallel registry is forbidden — this is `Principle 5 — One Graph, Many Views` applied
  to product surfaces rather than to artifacts.
- **P6 — Coverage is checked against an independent universe.** A check whose expected set
  is derived from the artifact under test cannot fail and is not a check.
- **P7 — The guaranteed core is guaranteed everywhere.** A capability in the navigation
  core appears on every surface that renders navigation, or the core is not a core.
- **P8 — Policy travels with the capability.** Provenance requirement, side-effect class
  and authorization travel in the descriptor, so every surface enforces the same rule
  without re-deriving it.
- **P9 — The registry is content-free.** Identity and policy only. No user content, no
  readiness fact, no session scope — matching the discipline the existing catalog holds.

---

## 5. Actors & Personas

| Actor | Description | Key goals | Reach today |
|---|---|---|---|
| **Daily user** | Uses the product through conversation and the shell | Ask for anything the system can do, in their own words, without knowing its internal name | 5 declared intents; 4 slash tokens; navigation that omits the digest on two of three surfaces |
| **Operator** | Runs the instance and decides what is exposed | Decide, once and visibly, what is reachable and by whom | No single place records this; the decision is spread across four files |
| **External authorized client** | A tool client consuming the corpus through the external tool surface | Discover the capability set it is permitted to call | Tool list does not exist yet (spec 109, planning-only) |
| **Capability author** | Adds or changes a capability | Add a capability once and have it become reachable everywhere it should be | Four edits in four languages; forgetting one is silent |
| **Reviewer** | Reviews a change that adds or exposes a capability | See what a change exposes, to whom, with what authority | No artifact answers this; it must be reconstructed by reading four files |

---

## 6. Use Cases

### UC-112-001 — A user asks for a built capability in plain language
- **Actor:** Daily user
- **Preconditions:** The capability is built and its descriptor declares it user-facing.
- **Main flow:** The user phrases a request in their own words → the request is matched
  against the intent phrasing projected from the descriptor set → the owning capability is
  dispatched → the response carries whatever provenance the descriptor requires.
- **Alternative flow:** No capability matches with sufficient confidence → the user is told
  honestly that nothing matched, and is never silently handed a different capability.
- **Postconditions:** The user reached a capability without knowing its internal id, its
  page, or its slash token.

### UC-112-002 — An author adds a capability
- **Actor:** Capability author
- **Preconditions:** The capability's implementation exists.
- **Main flow:** The author writes one descriptor declaring exposure, intent phrasing,
  owning service, authorization, provenance, side-effect class, navigation status and alias
  → the projections are regenerated → the capability appears on every surface its descriptor
  says it belongs on.
- **Alternative flow:** The descriptor is incomplete → the coverage check fails and names
  the missing facet → nothing ships half-exposed.
- **Postconditions:** No hand edit was made to any reachability surface.

### UC-112-003 — An operator decides what is exposed
- **Actor:** Operator
- **Preconditions:** The registry exists.
- **Main flow:** The operator reads one artifact listing every capability with its exposure
  class, its reason where it is not user-facing, and its authorization requirement → changes
  a decision in one place → the surfaces follow.
- **Postconditions:** The exposure posture of the product is answerable from one record.

### UC-112-004 — A user opens the daily digest from any surface
- **Actor:** Daily user
- **Preconditions:** The digest capability is in the guaranteed navigation core.
- **Main flow:** The user is on any surface that renders navigation → the digest is present
  in that navigation → the user opens it.
- **Postconditions:** The product's primary recurring output is reachable from the shell
  rather than only from the surface that happens to list it.

### UC-112-005 — A reviewer assesses what a change exposes
- **Actor:** Reviewer
- **Preconditions:** A change adds or re-classifies a capability.
- **Main flow:** The reviewer reads the descriptor diff → sees exposure class, principal,
  grants, side-effect class and provenance requirement in one place → judges the exposure.
- **Postconditions:** Exposure review does not require reconstructing state from four files.

---

## 7. Business Scenarios (Gherkin)

### One record, many projections

#### SCN-112-A01 — A capability is described once
```gherkin
Given a capability that is reachable from more than one surface
When its intent phrasing, authorization requirement and provenance requirement are sought
Then all three are found in exactly one record
And no reachability surface carries a capability fact absent from that record
```

#### SCN-112-A02 — Every reachability surface is generated from the record
```gherkin
Given the capability descriptor set
When the assistant intent set, the navigation core, the alias table and the external tool list are produced
Then each is derived from that descriptor set
And none of them is authored or edited by hand
```

#### SCN-112-A03 — Adding a capability requires exactly one edit
```gherkin
Given a newly built capability
When exactly one descriptor is added declaring it user-facing with an intent phrase and an alias
Then it becomes reachable by asking for it in plain language
And it appears in its declared navigation position
And it is callable by its alias
And no reachability surface was edited to achieve this
```

#### SCN-112-A04 — A hand-maintained inventory surviving beside its projection is rejected (boundary)
```gherkin
Given a reachability surface that has a generated projection
When a separately authored list for that same surface is still present and consulted at runtime
Then that condition is reported as a defect
And it is not accepted as a transitional state
```

#### SCN-112-A05 — The existing catalog's capability reference resolves
```gherkin
Given the existing experience catalog whose every surface carries a capability reference
When those references are resolved against the registry
Then each one resolves to exactly one capability descriptor
And a reference that resolves to nothing is reported
```

#### SCN-112-A06 — A second parallel registry is rejected (boundary)
```gherkin
Given the existing experience catalog and its projections
When capability identity or capability policy is authored anywhere outside the registry that extends it
Then that condition is reported as a duplication defect
And the change is refused
```

### Declared exposure

#### SCN-112-B01 — Every built capability carries an explicit exposure decision
```gherkin
Given the set of built capabilities derived independently of the registry
When each is looked up in the registry
Then each carries an exposure class from the closed vocabulary
And no capability is in an undeclared state
```

#### SCN-112-B02 — A capability that is not user-facing carries a reason
```gherkin
Given a capability whose exposure class is anything other than user-facing
When its descriptor is read
Then it carries a recorded reason for that classification
And an empty or absent reason is reported as a defect
```

#### SCN-112-B03 — A non-dispatchable pipeline stage is distinguishable from an omission
```gherkin
Given a built unit that is a pipeline stage rather than an invocable capability
When the registry is consulted
Then it is identifiable as structurally non-dispatchable
And it is not counted among capabilities lacking a front door
```

#### SCN-112-B04 — A test-only capability is never user-facing (boundary)
```gherkin
Given a capability whose exposure class is test-only
When the user-facing projections are produced
Then it appears in none of them
And it is still present in the registry with its classification recorded
```

#### SCN-112-B05 — A newly built dispatchable capability defaults to nothing
```gherkin
Given a newly built dispatchable capability with no exposure decision recorded
When the coverage check runs
Then it fails and names the capability
And the capability is neither silently exposed nor silently hidden
```

### The guaranteed navigation core

#### SCN-112-C01 — The digest is in the guaranteed core
```gherkin
Given the guaranteed cross-surface navigation core
When it is read
Then the daily digest is a member of it
```

#### SCN-112-C02 — The core is identical on every surface that renders navigation
```gherkin
Given more than one surface that renders navigation
When each surface's navigation is produced from the registry
Then every member of the guaranteed core is present on each of them
And a member present on one surface and absent from another is reported as drift
```

#### SCN-112-C03 — Two navigation authorities cannot disagree (boundary)
```gherkin
Given two surfaces that each render navigation
When one of them declares that it mirrors the other
Then that claim is verified against the generated core rather than asserted in a comment
And a divergence between them fails rather than passing silently
```

#### SCN-112-C04 — A surface-local addition does not create a hidden core member
```gherkin
Given a surface that appends its own links after the shared navigation
When a capability appears only in that surface-local addition
Then it is reported as absent from the guaranteed core
And it is not treated as cross-surface reachable
```

### Authorization travels with the capability

#### SCN-112-D01 — Every enabled capability names its principal and grants
```gherkin
Given a capability whose exposure class makes it reachable
When its descriptor is read
Then it names the authenticated principal it requires
And it names the grants it requires
```

#### SCN-112-D02 — Authorization is derived on the server
```gherkin
Given a request to invoke a capability
When the required authority is determined
Then it is derived on the server from the descriptor and the authenticated principal
And no authority asserted by the caller is trusted
```

#### SCN-112-D03 — A capability whose guard is unenforceable is not surfaced (boundary)
```gherkin
Given a capability whose descriptor names a grant the running system cannot yet enforce
When the reachability projections are produced
Then the capability is not surfaced
And the reason it was withheld is reported
```

#### SCN-112-D04 — A side-effecting capability is distinguishable from a reading one
```gherkin
Given capabilities that read, capabilities that write, and capabilities that leave the system
When their descriptors are read
Then each declares which of those it does
And a surface that must treat them differently can do so without inspecting the implementation
```

#### SCN-112-D05 — A capability requiring provenance cannot answer without it
```gherkin
Given a capability whose descriptor requires provenance
When it produces an answer carrying no citation
Then the answer is refused or reported honestly
And it is never presented as a grounded result
```

### The coverage check

#### SCN-112-E01 — An enabled capability missing an intent fails the check
```gherkin
Given a capability declared user-facing
When it carries no intent phrasing
Then the coverage check fails and names the capability and the missing facet
```

#### SCN-112-E02 — An enabled capability missing an authorization policy fails the check
```gherkin
Given a capability declared reachable
When it carries no authorization requirement
Then the coverage check fails and names the capability and the missing facet
```

#### SCN-112-E03 — An enabled capability missing a navigation status fails the check
```gherkin
Given a capability declared reachable
When it declares no navigation status
Then the coverage check fails and names the capability and the missing facet
```

#### SCN-112-E04 — An enabled capability missing an evaluation case fails the check
```gherkin
Given a capability declared user-facing
When no evaluation case exercises it
Then the coverage check fails and names the capability and the missing facet
```

#### SCN-112-E05 — The check compares against an independently derived universe (boundary)
```gherkin
Given a built capability that is absent from the registry entirely
When the coverage check runs
Then it fails and names that capability
And the expected universe used by the check is not derived from the registry under test
```

#### SCN-112-E06 — A check that did not run is a failure, not a pass
```gherkin
Given a validation run in which the coverage check did not execute
When the outcome is reported
Then it is reported as a failure
And absence of a result is never recorded as success
```

---

## 8. Requirements

Requirements are behavioural and tech-agnostic. They state what must be true, never how.

### One record, one authority

- **R-112-01** — Every capability MUST be described by exactly one capability descriptor.
- **R-112-02** — A capability descriptor MUST carry: a stable identifier; intent phrasing;
  the owning domain service; the required authenticated principal and grants; the
  provenance requirement; the side-effect class; the navigation projection; the alias
  projection; and the external tool projection.
- **R-112-03** — A capability's stable identifier MUST NOT change when its phrasing,
  navigation position, alias, or exposure class changes.
- **R-112-04** — No reachability surface MAY carry a capability fact that is absent from
  that capability's descriptor.
- **R-112-05** — The registry MUST extend the existing experience catalog rather than
  standing beside it. Capability identity and capability policy MUST NOT be authored in a
  second location.
- **R-112-06** — The existing catalog's per-surface capability reference MUST resolve to a
  capability descriptor, and an unresolvable reference MUST be reported.
- **R-112-07** — The registry MUST carry identity and policy only, and MUST NOT carry user
  content, readiness facts, or session scope.

### Generation, not maintenance

- **R-112-08** — The natural-language intent set, the navigation core, the alias table and
  the external tool list MUST each be generated from the descriptor set.
- **R-112-09** — After a surface's projection exists, the previously hand-maintained
  inventory for that surface MUST be retired, not left active beside it.
- **R-112-10** — Adding, removing or re-classifying a capability MUST require editing only
  its descriptor.
- **R-112-11** — A divergence between two surfaces that are both projections of the same
  descriptor set MUST be detectable and MUST fail rather than pass silently.
- **R-112-12** — A claim by one surface that it mirrors another MUST be verified against
  the generated core rather than asserted in prose.

### Declared exposure

- **R-112-13** — Every built capability MUST carry an exposure class drawn from a closed
  vocabulary, and "undeclared" MUST NOT be a valid state.
- **R-112-14** — Any exposure class other than user-facing MUST carry a recorded reason.
- **R-112-15** — A unit that is structurally non-dispatchable MUST be identifiable as such,
  so it is distinguishable from a capability that was omitted by accident.
- **R-112-16** — A test-only capability MUST NOT appear in any user-facing projection.
- **R-112-17** — A newly built dispatchable capability with no recorded exposure decision
  MUST fail validation rather than defaulting to exposed or to hidden.
- **R-112-18** — The set of user-facing capabilities MUST be expandable to every
  dispatchable capability for which no reasoned exclusion is recorded.

### The guaranteed navigation core

- **R-112-19** — The daily digest MUST be a member of the guaranteed cross-surface
  navigation core.
- **R-112-20** — Every member of the guaranteed core MUST be present on every surface that
  renders navigation.
- **R-112-21** — A capability present only in a surface-local navigation addition MUST be
  reported as absent from the guaranteed core and MUST NOT be treated as cross-surface
  reachable.
- **R-112-22** — A surface that renders no navigation MUST be identifiable, so the set of
  surfaces on which the core is not guaranteed is a known quantity rather than an unknown
  one.

### Authorization

- **R-112-23** — Every capability whose exposure class makes it reachable MUST name the
  authenticated principal and the grants it requires.
- **R-112-24** — Authorization MUST be derived on the server from the descriptor and the
  authenticated principal. Authority asserted by the caller MUST NOT be trusted.
- **R-112-25** — A capability whose declared grant cannot be enforced by the running system
  MUST NOT be surfaced, and the reason for withholding it MUST be reported.
- **R-112-26** — The grant vocabulary MUST be consumed from the owning specification and
  MUST NOT be redefined here.
- **R-112-27** — Every capability MUST declare its side-effect class, so a surface can
  treat reading, writing and externally-visible capabilities differently without inspecting
  the implementation.
- **R-112-28** — Every capability MUST declare its provenance requirement, and a capability
  that requires provenance MUST NOT present an uncited answer as grounded.

### Coverage

- **R-112-29** — A coverage check MUST fail when an enabled capability lacks an intent
  phrase, an authorization requirement, a navigation status, or an evaluation case.
- **R-112-30** — The coverage check MUST report the specific capability and the specific
  missing facet, not an aggregate count.
- **R-112-31** — The universe the coverage check compares against MUST be derived
  independently of the registry under test.
- **R-112-32** — A capability that exists in the built system but is absent from the
  registry entirely MUST fail the coverage check.
- **R-112-33** — A coverage check that did not execute MUST be reported as a failure, never
  as a pass.

---

## 9. Non-Functional Requirements

- **NFR-112-01 — Determinism.** The same descriptor set MUST produce byte-identical
  projections, so a regenerated projection is reviewable as a diff.
- **NFR-112-02 — Reviewability.** A change to what the product exposes MUST be legible as a
  descriptor diff, without reconstructing state from multiple files.
- **NFR-112-03 — Failure legibility.** A coverage failure MUST name the capability and the
  missing facet precisely enough to act on without further investigation.
- **NFR-112-04 — Ordering stability.** Projected ordering MUST be stable across
  regenerations, so navigation does not reshuffle for unrelated reasons.
- **NFR-112-05 — Honest copy.** No surface produced from the registry may describe a
  capability as available when its guard is unenforceable or its exposure is withheld.
- **NFR-112-06 — No content leakage.** A projection MUST NOT reveal the existence of a
  capability a principal is not permitted to know about.

---

## 10. Non-Goals

1. **A second registry.** Building a new capability registry beside `internal/experience/`
   is explicitly rejected. It would recreate the duplication this feature exists to remove
   and would violate `Principle 5 — One Graph, Many Views`. The existing catalog is
   extended.
2. **Defining the grant vocabulary.** `corpus:read` and its enforcement belong to spec 108.
   This feature consumes that vocabulary and does not redefine, widen, or fork it.
3. **Building the external tool surface.** Spec 109 owns the tool server. This feature
   supplies the per-capability projection it consumes; it does not implement the server.
4. **Deciding each capability's exposure.** This feature requires that every capability
   carry a declared class and a reason. Which class each of the eleven receives is an
   operator and design decision, recorded per capability, not fixed here.
5. **Redesigning navigation information architecture.** This feature makes the core
   generated, guaranteed and inclusive of the digest. Rethinking the hierarchy is separate.
6. **Retrofitting shared navigation onto all 29 island pages.** That composition work
   belongs to the shell specifications. This feature makes the absence measurable and names
   the core those pages must eventually render.
7. **Changing how any capability is implemented.** The registry describes capabilities; it
   does not alter their behaviour.

---

## 11. Open Findings (routed, not resolved here)

| ID | Severity | Finding | Owner |
|---|---|---|---|
| `F-112-FLAG-01` | **BLOCKING** | No feature flag is declared, so `flagsIntroduced` is deliberately empty. A cutover that replaces live navigation and widens natural-language reach warrants a flag, but gate G111 requires an introduced flag to be default-OFF in every non-owning train's bundle, which means editing `config/feature-flags.mvp.yaml` — an artifact owned by `bubbles.train` and outside this authoring run's permitted surface. Naming a flag here without the backing bundle entries would be a declaration with no enforcement. | `bubbles.train` |
| `F-112-108-01` | **BLOCKING** | R-112-23 through R-112-26 depend on spec 108's `corpus:read` grant model. Spec 108 is `specs_hardened` — planned, not delivered (E18). Surfacing capabilities before that boundary is enforceable violates P4. The ordering between the two specs is a real dependency, not a preference. | `bubbles.plan` |
| `F-112-UNIT-01` | **BLOCKING** | The registry's unit is the capability; the built artifacts are prompt contracts, and the mapping is not 1:1 (E3, §3 sharpening). The rule that decides what constitutes one capability, and how contracts bind to capabilities, is undecided. Getting it wrong reproduces the original defect at a new granularity. | `bubbles.design` |
| `F-112-UNIVERSE-01` | **BLOCKING** | R-112-31 requires the coverage check to compare against an independently derived universe. How that universe is derived — and how it avoids the circularity already present at `route_inventory_test.go:92-100` (E9) — is undecided. Without this the check cannot fail and is decorative. | `bubbles.design` |
| `F-112-109-01` | HIGH | Spec 109 explicitly forbids deriving the tool list by passthrough (E19). The external tool projection must therefore be a per-capability declared projection, not a blanket export of the capability set. Reconciling R-112-08 with that constraint is a design decision. | `bubbles.design` (with spec 109) |
| `F-112-CUTOVER-01` | HIGH | Three navigation authorities are live and already divergent (E11–E14), and the catalog package records that their cutover was deferred to a later scope of the shell specification (E10). Whether this feature performs that cutover or hands it to the shell spec is undecided, and R-112-09 cannot be satisfied by whichever spec does not. | `bubbles.plan` |
| `F-112-ALIAS-01` | HIGH | The alias table and the assistant registry currently contradict each other on `/ask`, `/recipe` and `/cook` (E15, E16). Generating both from one descriptor forces a decision about which existing behaviour is correct. That decision changes shipped user-visible behaviour for at least one token and cannot be made silently by a generator. | `bubbles.design` |
| `F-112-ISLANDS-01` | MEDIUM | 29 of 31 first-party PWA pages render no shared navigation (E17). R-112-20 is satisfiable in principle by a core that reaches only surfaces which render navigation at all, which would make the guarantee vacuous on most pages. The relationship between this feature's guarantee and the island pages must be stated rather than assumed. | `bubbles.design` (with the shell spec) |
| `F-112-TEMPLATE-01` | LOW | The authoring brief noted that `.specify/templates/spec-template.md` is expected to govern spec structure. **That file does not exist in this repository** — `.specify/` contains only `memory/`, `metrics/` and `runtime/`; there is no `templates/` directory. This spec follows the structure of the adjacent sibling spec together with the Bubbles BDD scenario contract. Either the template should be installed or the instruction corrected. | operator via `bubbles.plan` |
| `F-112-110-01` | LOW | Sibling spec `110-retrieval-quality-foundation` has no `state.json`, so it carries no `releaseTrain` or `flagsIntroduced` declaration. This was observed while reading siblings for consistency and is **not modified here** (that spec is outside this run's permitted surface), but it is recorded because gate G110 covers train declaration across specs. | `bubbles.plan` |

---

## Product Principle Alignment

Cited from [`docs/Product-Principles.md`](../../docs/Product-Principles.md) by number and
exact name.

| Principle | How this feature serves it |
|---|---|
| **Principle 2 — Vague In, Precise Out** | The principle holds that a user who cannot find what they need loses confidence, and that requiring exact names is a regression. Today reaching eleven built capabilities requires knowing an exact page or an exact slash token. Generating the intent set from the descriptors is what lets a user ask in their own words instead. |
| **Principle 5 — One Graph, Many Views** | This is the governing principle. It states that views are "projections, not separate stores" and that a new parallel store "is rejected — extend the existing graph". This feature applies that rule to product surfaces: one descriptor set, four projections, and an explicit rejection of a second registry (§10 non-goal 1). It is also the principle the current four hand-maintained inventories violate. |
| **Principle 7 — Small, Frequent, Actionable Output** | The digest is the product's primary recurring output and the principle's subject. R-112-19 puts it in the guaranteed core so the output the principle is about is reachable from the shell rather than from whichever surface happens to list it. |
| **Principle 8 — Trust Through Transparency** | The principle requires that output without source attribution is rejected. R-112-28 makes the provenance requirement a declared property of the capability, so every surface enforces the same rule rather than each re-deriving it — and a capability that requires provenance cannot present an uncited answer as grounded. |
| **Principle 11 — Local-First Data Ownership** (ratified 2026-07-29, BLOCKING) | The principle requires that where a capability lets an authorized external client read the corpus, access is "an explicit, per-client, audited operator grant — never a default, never a build-time switch, never silent". R-112-23 through R-112-26 make the grant a declared per-capability property, and R-112-25 refuses to surface a capability whose grant cannot be enforced. The external tool projection is bound by this: exposing a capability to an external client is a grant decision, not a generation side effect. |
| **Principle 6 — Invisible By Default, Felt Not Heard** | Recorded as a **tension, not an alignment.** This feature raises how much the product surfaces. The principle constrains interruption, not discoverability, so making a capability *findable when asked for* does not conflict with it — but any capability whose descriptor introduces a notification or a proactive prompt remains bound by the principle's actionability bar and its budget. The registry must not become a route around that bar. |

---

## Release Train

**Declared train: `next`** (`target_slot: staging`, per
[`config/release-trains.yaml`](../../config/release-trains.yaml)).

The two valid trains are `mvp` (`target_slot: prod`) and `next` (`target_slot: staging`).

### Why `next` and not `mvp`

1. **This raises reach, and the guard that should bound it is not delivered.** Surfacing the
   eleven currently-undeclared dispatchable capabilities makes them invocable by anyone who
   can talk to the assistant. The per-capability authorization that bounds that
   (`corpus:read`, spec 108) is `specs_hardened`, not delivered, and targets `next` (E18).
   Shipping the exposure on the live train while its boundary lives on the other train would
   widen reach ahead of its guard — precisely what P4 forbids.
2. **Its two hard dependencies are both on `next`.** Spec 108 (grants) and spec 109 (the
   external tool surface) are both `specs_hardened` on `next`. A registry on `mvp` would
   project into a tool list that does not exist on that train, and would declare grants that
   train does not enforce.
3. **It replaces live navigation on a running shell.** Three navigation authorities are
   active and already divergent (E11–E14). Cutting them over to a generated projection is a
   change with real regression surface on every page a user touches; proving it on staging
   before the live host is the correct order.
4. **The sibling precedent is consistent.** Spec 111 chose `next` on the same reasoning —
   an enforcement-adjacent change whose dependency sits on `next` — and spec 108 states in
   its own words that a security-posture change "MUST NOT ship on `mvp`".

### Behaviour on the `mvp` train

On `mvp` this capability is **default-off**: the registry drives no surface, the three
existing hand-maintained navigation authorities and the existing slash table remain in
force unchanged, and no capability's exposure class changes. The `mvp` train therefore
continues to expose five user-facing intents and continues to omit the digest from the
shared navigation core until promotion.

**That cost is stated rather than obscured.** Choosing `next` means the live train keeps the
reachability gap that `D17` and `D18` describe for longer. That is the accepted trade: the
alternative is surfacing capabilities on the live host before their authorization boundary
can be enforced, which trades a discoverability gap for a security one.

### Flag declaration

`flagsIntroduced` is **deliberately empty**, not omitted. The default-off behaviour above
is the behaviour a flag would implement, but gate G111 requires an introduced flag to be
default-OFF in every non-owning train's bundle, which requires editing
`config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml` — both owned by
`bubbles.train` and outside this authoring run's permitted surface. Declaring a flag name
here without those bundle entries would produce a declaration that reads as enforced and is
not. The declaration is routed to `bubbles.train` as `F-112-FLAG-01`, marked BLOCKING, and
must be resolved before delivery scopes begin.

---

## Exposure Contract

| Capability | Surface class | Surface id | Status | Plan |
|---|---|---|---|---|
| Capability descriptor set | internal | the registry extending `internal/experience/` | planned | this spec; delivery gated on `F-112-UNIT-01` |
| Assistant intent projection | internal | generated replacement for `config/assistant/scenarios.yaml` | planned | this spec, R-112-08 |
| Navigation core projection | uiRoute | generated cross-surface navigation core, digest included | planned | this spec, R-112-08 and R-112-19 |
| Alias projection | internal | generated replacement for the slash alias table | planned | this spec, R-112-08; contradiction resolution gated on `F-112-ALIAS-01` |
| External tool projection | internal | per-capability projection consumed by the tool surface | planned | spec 109; this spec supplies the projection only |
| Capability coverage check | internal | consumed by the repository's validation surface | planned | this spec, R-112-29 through R-112-33 |

No capability in this feature is delivered. Every row is `planned`, and each names the spec
or the finding that gates it. There are no `delivered` rows to reconcile, because no
implementation exists.
