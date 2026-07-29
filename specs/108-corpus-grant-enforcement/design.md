# Design: 108 Corpus Grant Enforcement

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Planning only — no source edited.**
**Requirements source:** [`spec.md`](spec.md) · **Release train:** `next`

---

## Design Brief

**Current State.** `internal/auth/browser_session_policy.go` already defines the whole
authorization vocabulary for corpus reads: the grant constant `GrantGlobalCorpusRead =
"corpus:read"`, the decision function `GateGlobalCorpusRead(sess) CorpusDecision`, and the
generic `AuthorizeGrant(sess, required)`. The wildcard `*` is never honored. But
`GateGlobalCorpusRead` is referenced by exactly one file — `internal/api/auth_surface_contract_test.go`
— and by **zero** production routes. In `internal/api/router.go` (~L87–L109) the corpus
surfaces `/api/search`, `/api/digest`, `/api/recent`, `/api/artifact/{id}`,
`/api/artifacts/{id}/domain`, `/api/export`, `/api/knowledge/*` and `/api/context-for` sit
directly under `r.Use(deps.bearerAuthMiddleware)` with no scope check at all. Any
authenticated principal reads the entire global corpus. `dailyUserGrants` is
`[assistant:turn, knowledge-graph:read]` — it deliberately excludes `corpus:read` — so the
grant model already says "daily users may not read the corpus" while the router says
otherwise.

**Target State.** Enforce `corpus:read` exactly as spec 060 designed it, by mounting the
*existing* `auth.RequireScope` chi middleware on the corpus route group. Rollout is
two-stage: **OBSERVE** (evaluate the grant, emit telemetry, allow the request) then
**ENFORCE** (evaluate the grant, deny with a bare 403). Stage selection is a single
fail-loud configuration value with no default.

**Patterns to Follow.**
- `internal/api/router.go` annotations block: `r.Group(...)` + `r.Use(auth.RequireScope("annotation:edit"))`.
- `internal/api/router.go` graph block: `r.Use(auth.RequireScope("knowledge-graph:read"))`.
- `internal/metrics/auth.go` `smackerel_auth_*` counter family (extend it; do not fork it).
- `config/feature-flags.mvp.yaml` / `config/feature-flags.next.yaml` `clientReleaseLaneB`
  precedent for a two-phase, operator-activated, default-OFF-everywhere flag.

**Patterns to Avoid.**
- Per-handler `if !authorized { ... }` checks inside `SearchHandler` / `ExportHandler` /
  the knowledge handlers. That is the shape this repo already rejected for `annotation:edit`;
  it makes the gate invisible to the route manifest and un-auditable, and a new corpus
  handler silently ships ungated.
- Reusing `smackerel_auth_scope_rejected_total` for the observe signal. A would-be denial
  and a real rejection would become indistinguishable, destroying the observation window
  (R-108-O2).
- Any `${VAR:-observe}` / `os.Getenv(k)`-with-default / `unwrap_or` shape for the stage
  value. Forbidden by `.github/instructions/smackerel-no-defaults.instructions.md`.
- Row-level or per-principal corpus partitioning as a "softer" alternative. The corpus is
  one global store; no partitioning exists and inventing one is a different feature.

**Resolved Decisions.**
- Enforcement is a chi middleware on the route group, not a handler-local check.
- `dailyUserGrants` is NOT widened. Ungranted daily users are *supposed* to be denied.
- Stage is resolved once at process start, fail-loud, from SST-generated config.
- Denial is a bare 403 with no body-carried corpus signal.
- The gate is mounted in BOTH stages; the stage only selects deny-vs-count.

**Open Questions.** None blocking design. **Updated 2026-07-29 — `spec.md` §18 is now a RATIFIED
decision record.** F-108-TELEGRAM-01's *direction* is settled: §18 decision 3 ratifies **option
(b)** — derive the Telegram token's scope claim from the mapped principal's **persisted grant
set**; extending the minter's hardcoded list is REJECTED. The finding stays open as *work*
(implementing derivation, and its dependency on F-108-UX-ROSTER-01 grant readability), not as a
*choice*. F-108-SURFACE-01 / R-108-PRE1 (`corpus` not yet in `auth.RegisteredScopeSurfaces`) was
never a direction question and remains a stage-2 prerequisite, planned as Scope 01.

**Ratification impact on this design — routed to `bubbles.design`, NOT silently applied here.**
§18 decision 5 brings the eight Phase-5 corpus-derived intelligence endpoints in scope, taking
the gated surface from **8 to 16** route groups (`spec.md` §4.2 Tier B). Two design-owned
surfaces below are therefore stale and are corrected only where they assert the *opposite
direction*; the remaining reconciliation — extending the §2 route-inventory table to sixteen
rows, and the §8 T2/T4/T8 count language — belongs to `bubbles.design`, which owns this file.
`scopes.md` plans to the ratified sixteen-group surface and carries a blocking DoD item for this
reconciliation, exactly as it already does for the flag-default divergence. This planning packet
does not rewrite settled design content.

---

## 1. Enforcement Seam

**Seam:** a single `r.Group(...)` wrapper inside the existing authenticated group in
`internal/api/router.go`, carrying two middlewares in this order:

```
r.Group(func(r chi.Router) {
        r.Use(deps.CorpusGrantGate.Observe)          // stage-aware telemetry, never denies
        r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))  // mounted only in ENFORCE
        ... corpus routes ...
})
```

**Where exactly.** Inside the block opened at `internal/api/router.go` by
`r.Group(func(r chi.Router) { r.Use(deps.bearerAuthMiddleware) ... })`, wrapping the eight
corpus route registrations listed in §2. The outer `bearerAuthMiddleware` must run first —
it is what populates the session that `AuthorizeGrant` reads. The gate is mounted *inside*
that group and *outside* the individual `r.Get` / `r.Post` / `r.Route` calls.

**Why the middleware, not the handlers.** The router file is the repo's route manifest.
Two live precedents already prove the pattern in the same function: `annotation:edit` and
`knowledge-graph:read`. Mounting on the group means (a) a new corpus route added inside the
group is gated by construction, (b) the contract test in §8 can assert the manifest
statically, and (c) `RequireScope`'s existing source switch — which lets shared-token and
bootstrap sessions through — applies uniformly instead of being re-implemented eight times.

**Why not elsewhere.**
- *Not in `bearerAuthMiddleware`.* That middleware serves every authenticated route
  including `/api/capture` and `/api/assistant/turn`; putting a corpus grant check there
  would deny writes and assistant turns.
- *Not in the storage layer.* The corpus is one global store with no per-principal
  partition; a store-level check would have to re-derive the session from context and would
  leak result counts through timing and pagination shape before the decision is made.
- *Not in `cmd/core` wiring.* Wiring decides *whether* a capability exists; this decides
  *who may call it*. Mixing them repeats the fail-soft/authorization confusion already
  untangled in the graph block.

**Reuse of existing symbols.** No new authorization primitive is introduced.
`auth.RequireScope` is the existing middleware; `auth.GrantGlobalCorpusRead` is the existing
constant; `auth.GateGlobalCorpusRead` becomes the decision function the observe middleware
calls, finally giving it a production caller.

---

## 2. Route Inventory

Route-group labels are a **closed set of eight** values (R-108-O3) — never the raw path.

| # | Route(s) | Method | Handler | `route_group` label | Gated |
|---|---|---|---|---|---|
| 1 | `/api/search` | POST | `deps.SearchHandler` | `search` | YES |
| 2 | `/api/digest` | GET | `deps.DigestHandler` | `digest` | YES |
| 3 | `/api/recent` | GET | `deps.RecentHandler` | `recent` | YES |
| 4 | `/api/artifact/{id}` | GET | `deps.ArtifactDetailHandler` | `artifact_detail` | YES |
| 5 | `/api/artifacts/{id}/domain` | GET | `deps.DomainDataHandler` | `artifact_domain` | YES |
| 6 | `/api/export` | GET | `deps.ExportHandler` | `export` | YES |
| 7 | `/api/knowledge/concepts`, `/concepts/{id}`, `/entities`, `/entities/{id}`, `/lint`, `/stats` | GET | `deps.Knowledge*Handler` | `knowledge` | YES |
| 8 | `/api/context-for` | POST | `deps.ContextHandler.HandleContextFor` | `context_for` | YES |

Group 7 is registered via `r.Route("/knowledge", ...)`; the gate attaches to the enclosing
group so all six knowledge endpoints inherit it as one unit.

### Routes deliberately NOT gated

| Route | Method | Why not |
|---|---|---|
| `/api/capture` | POST | Write path. Ingest is not a corpus *read*; gating it would break capture for every daily user. |
| `/api/assistant/turn` | POST | Already gated by the `assistant:turn` claim in the `PreFacadeChain` wired in `cmd/core/wiring_assistant_facade.go`. Its corpus access is mediated, not raw. |
| `/api/artifacts/{id}/annotations*`, `/api/annotations`, `/api/artifacts/{id}/tags/{tag}` | POST/GET/DELETE | Already gated by `auth.RequireScope("annotation:edit")`. Annotation bodies are user-authored, not corpus content. |
| `/api/topics`, `/api/people`, `/api/places`, `/api/time`, `/api/graph/edges` | GET | Already gated by `auth.RequireScope("knowledge-graph:read")`, which IS in `dailyUserGrants`. Adding `corpus:read` here would revoke a grant daily users legitimately hold. |
| `/api/bookmarks/import` | POST | Write path. |
| `/api/internal/telegram-message-artifact` | POST/GET | Internal id↔id mapping; returns no corpus content. |
| `/api/expertise` and the other Phase-5 intelligence endpoints | GET | ~~Out of scope per spec §12 Non-Goals~~ — **SUPERSEDED 2026-07-29 by `spec.md` §18 decision 5 (F-108-ADJ-01): these eight endpoints are now IN SCOPE and gated as Tier B.** They compute over the same global corpus, so leaving them bearer-only would have left a partial boundary. See `spec.md` §4.2 Tier B for the canonical inventory and `scopes.md` Scope 03 (SCN-108-G04, SCN-108-G05) for the plan. Extending the §2 table above to sixteen rows is routed to `bubbles.design`. |
| `/api/health`, `/metrics`, `/readyz` | GET | Unauthenticated by design; carry no corpus data. |

---

## 3. Denial Semantics

In ENFORCE stage a principal without `corpus:read` receives:

- HTTP **403 Forbidden**.
- The response body is the existing `RequireScope` denial envelope with the scope name only.
  It MUST NOT include: result counts, artifact ids, artifact titles, domain labels, "0 results"
  phrasing, or any existence hint. This mirrors the `CorpusDecision` contract in
  `internal/auth/browser_session_policy.go`, which states a denial "carries NO content, count,
  label, or existence hint".
- **Identical shape for every route group.** `/api/artifact/{id}` for a non-existent id and
  for an existing id must be byte-identical, otherwise the denial becomes an existence oracle.
- **No 404-vs-403 discrimination.** The gate runs before the handler, so a denied principal
  never learns whether the id resolves.
- The wildcard `*` in a token scope claim is NOT honored (`AuthorizeGrant` semantics). A
  wildcard-scoped token is denied exactly like an empty-scoped one.
- No `WWW-Authenticate` challenge and no retry hint — the remedy is an operator grant
  (a token rotation, per F-108-GRANT-MECHANISM-01), not a client-side retry.

---

## 4. Observe → Enforce Stage Machine

### Configuration

| Layer | Name | Value |
|---|---|---|
| Train flag | `corpusGrantEnforcement` | `false` in `config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml` (default-OFF in every train, per R-108-FL3). |
| SST key | `auth.corpus_grant_enforcement` in `config/smackerel.yaml` | Declared with **no default value**. |
| Generated env | `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` | Emitted by `./smackerel.sh config generate` into `config/generated/<env>.env`. |

**Decision:** the value flows through the **SST pipeline** (not read ad hoc from the flag
bundle at runtime). The bundle is the operator's per-train source; `config generate` compiles
it into the env file; the process reads the env var. This keeps one config path and one
fail-loud validation point.

**Fail-loud validation.** Resolution happens once, at startup, in `cmd/core` wiring:

- Absent or empty → the process **aborts** naming `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT`
  (SCN-108-C03).
- Any value other than the two accepted booleans → abort naming the offending value.
- Shell interpolation uses the `${VAR:?...}` form. `${VAR:-...}`, `os.Getenv` with a default,
  and any silent observe-or-enforce choice are forbidden
  (`.github/instructions/smackerel-no-defaults.instructions.md`).
- There is no third mode and no per-route override (R-108-FL6).

### Behavior by stage

| | OBSERVE (`false`) | ENFORCE (`true`) |
|---|---|---|
| `auth.GateGlobalCorpusRead` evaluated | YES | YES |
| `auth.RequireScope` mounted | NO | YES |
| Ungranted request outcome | **200**, content returned | **403**, bare denial |
| Telemetry on ungranted request | `..._corpus_grant_would_deny_total` | `smackerel_auth_scope_rejected_total` |

The observe middleware is mounted in **both** stages — otherwise OBSERVE emits nothing.

### Metrics (extend `smackerel_auth_*` in `internal/metrics/auth.go`; no parallel family)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `smackerel_auth_corpus_grant_would_deny_total` | Counter | `route_group` (closed set of 8), `user_id`, `session_source` | A request that WOULD be denied under ENFORCE but was allowed under OBSERVE. |
| `smackerel_auth_corpus_grant_allowed_total` | Counter | `route_group`, `session_source` | A request that carried the grant. Gives the denominator. |
| `smackerel_auth_corpus_grant_enforcement_mode` | Gauge | none | `0` = OBSERVE, `1` = ENFORCE. Lets a dashboard state the stage without reading config. |
| `smackerel_auth_scope_rejected_total` | Counter (existing) | existing labels | Real 403s in ENFORCE. Unchanged — deliberately NOT reused for the observe signal (R-108-O2). |

Cardinality: `route_group` is closed; `user_id` follows the existing precedent in
`internal/metrics/auth.go` and is bounded by the operator-controlled principal count;
`session_source` is the existing closed session-source enum. No raw path, ever (R-108-O3/O4).

### Structured log fields

Emitted once per would-be denial in OBSERVE at `warn`:
`event=corpus_grant_would_deny`, `route_group`, `user_id`, `session_source`,
`required_grant=corpus:read`, `enforcement_mode=observe`. No query text, no artifact ids —
the log answers *who*, not *what they searched for*.

### "Who would have been denied?" (UC-108-001)

The operator reads `sum by (user_id, route_group) (increase(smackerel_auth_corpus_grant_would_deny_total[7d]))`.
A non-empty result set is exactly the list of principals that must be granted `corpus:read`
(or accepted as intentionally denied) before the stage flips. An empty result over a full
observation window is the go/no-go signal for ENFORCE. `docs/Operations.md` carries the
runbook (R-108-O5).

---

## 5. Caller Compatibility

| Surface | Session source | Holds `corpus:read` today | Breaks at ENFORCE | Operator action |
|---|---|---|---|---|
| PWA / web browser session (daily user) | per-user bearer | NO — `dailyUserGrants` excludes it | **YES** — search, recent, digest, artifact detail, export, knowledge all 403 | Rotate the principal's token with `corpus:read` added to the scope claim (F-108-GRANT-MECHANISM-01: granting is a token rotation, not a flag flip). Requires R-108-PRE1 first. |
| PWA / web browser session (operator) | per-user bearer | YES — `operatorGrants` includes it | NO | None. |
| Browser extension | per-user bearer, same grant set as its principal | Same as the principal | **YES** if the principal is a daily user | Same token rotation. The extension consumes the principal's token; there is no extension-specific grant. |
| Telegram bridge | bridge token | **Unknown / provably insufficient** — F-108-TELEGRAM-01 | **YES, blocking** | **RATIFIED 2026-07-29 (`spec.md` §18 decision 3): derive the minted token's scope claim from the mapped principal's persisted grant set.** The earlier two-option framing ("receive a token carrying `corpus:read`" **or** "re-route through `/api/assistant/turn`") is **closed** — both were minter-side or routing-side workarounds that leave authority defined somewhere other than the principal. Derivation is the larger change and the only one consistent with spec 044 Scope 02 and the persisted-grant doctrine. Depends on F-108-UX-ROSTER-01 (grants are not readable server-side today). Still a stage-2 prerequisite, not a design gap. |
| Internal service-to-service (`/api/context-for`, GuestHost connector) | shared token | Bypasses the scope check per `RequireScope`'s source switch | NO | None — but the bypass MUST be asserted by test (§8) so it is a documented decision, not an accident. **`spec.md` §18 decision 4 (2026-07-29): the GuestHost connector credential does NOT receive `corpus:read`.** Its guest-context reads move to the spec-109 MCP `hospitality-read` path under its own audience-bound credential (spec 109 D3), which is itself blocked on BUG-019-003. Coordination owner: `bubbles.design` on spec 109. |
| Bootstrap session | bootstrap | Bypasses the scope check per `RequireScope`'s source switch | NO | None. |
| Prometheus scrape / orchestrator probes | unauthenticated | n/a | NO | None — `/metrics`, `/readyz`, `/api/health` are ungated. |

The OBSERVE window exists precisely to convert the "unknown" rows above into measured rows
before anyone is denied.

---

## 6. Rollback

Returning to OBSERVE requires **no rebuild and no code change**:

1. Set the train flag `corpusGrantEnforcement` back to `false` in the owning bundle
   (`config/feature-flags.next.yaml`).
2. Regenerate the config bundle for the target env (`./smackerel.sh config generate`).
3. Re-apply the **same signed image digest** with the new bundle. The deploy adapter's
   pointer-swap path applies; `docker build` is never invoked.

Denials stop immediately on the restarted process and counting resumes (SCN-108-C04).

**Why the stage is resolved once at startup rather than hot-reloaded.** A hot-reload path
must answer "what do I do when the reloaded value is absent or malformed?" — and every
answer is a silent default, which is forbidden. Fail-loud-at-startup is the only shape
compatible with `smackerel-no-defaults`. The cost is a process restart on rollback; the
restart is the same pointer-swap the deploy adapter already performs, so the operator's
rollback is one config change plus a restart of an unchanged artifact.

**Blast radius of a bad flip.** Worst case is a corpus-wide 403 for daily users. No data is
mutated, no migration runs, nothing is destroyed — the flip is symmetric and idempotent.

---

## 7. Decoupling From Spec 109 (MCP)

Spec 109 is **not blocked by, and not coupled to, this design**:

- **Different authorizer.** 109 carries its own authorizer and an audience-bound credential.
  It does not consult `auth.RequireScope` or `GateGlobalCorpusRead`.
- **Different mount point.** The seam introduced here is a chi middleware inside
  `internal/api/router.go`'s authenticated `/api` group. The MCP transport is not registered
  in that router, so it never traverses this middleware chain.
- **No shared mutable state.** The gate reads the session already placed on the request
  context by `bearerAuthMiddleware` and writes only counters. It adds no package-level state
  and changes no exported signature in `internal/auth`.
- **No new dependency edge.** `internal/api` already imports `internal/auth`; this design
  adds no import that 109 would inherit.
- **Grant-set independence.** `dailyUserGrants` and `operatorGrants` are unchanged, so 109's
  credential model is unaffected regardless of which stage is active.

Consequence: 109 may proceed in parallel. If 109 later chooses to reuse `corpus:read` as its
audience scope, that is 109's decision and requires no change here.

---

## 8. Test Strategy

| # | Category | Location | What it proves |
|---|---|---|---|
| T1 | `unit` | `internal/auth/browser_session_policy_test.go` | `GateGlobalCorpusRead` denies an empty scope claim, denies a `*` wildcard claim, allows an explicit `corpus:read` claim, and returns a `CorpusDecision` carrying no content/count/label. |
| T2 | `unit` | `internal/metrics/auth_test.go` | The three new metrics register in the `smackerel_auth_*` family with the closed 8-value `route_group` label set; an unknown label value is rejected. |
| T3 | `unit` | `cmd/core` config resolution test | Absent `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` aborts startup naming the variable; a malformed value aborts naming the value; neither silently selects a stage (SCN-108-C03). |
| T4 | `integration` | `internal/api` against the ephemeral test stack | OBSERVE: ungranted principal gets **200** on all eight route groups AND `..._corpus_grant_would_deny_total` increments with the right `route_group`. ENFORCE: same principal gets **403** on all eight. |
| T5 | `integration` | same | Shared-token and bootstrap sessions pass under ENFORCE (documented `RequireScope` source-switch bypass), so the bypass is asserted, not assumed. |
| T6 | `e2e-api` | `./smackerel.sh test e2e` | Full stack, real Postgres: granted operator token reads `/api/search` and `/api/export`; ungranted daily-user token is refused on both; the refusal body contains no artifact id, title, or count. |
| T7 | `e2e-api` | same | Route-group parity: a denied `/api/artifact/{id}` for a real id and for a random id produce byte-identical responses (no existence oracle). |

### T8 — Adversarial: the gate cannot be silently removed (REQUIRED)

Extend `internal/api/auth_surface_contract_test.go` — today the only referent of
`GateGlobalCorpusRead` — with a **route-manifest contract test**:

- Build the real router via the same constructor production uses, with ENFORCE selected.
- Enumerate the eight corpus routes from a canonical in-test list (the same eight
  `route_group` values as §2).
- For each, issue a request with a session whose scope claim is empty and assert **403**.
- Assert the **set equality** of "routes the test knows about" against the router's mounted
  corpus group, so adding a ninth corpus route without adding it to the list fails the test.

This test fails if any of the following happens: `r.Use(auth.RequireScope(...))` is deleted
from the group, a corpus route is moved out of the gated group, the stage machine defaults to
OBSERVE when the config is absent, or a new corpus route is registered ungated. It is
adversarial rather than tautological because its fixture principal has an **empty** scope
claim — the exact input that passes today's ungated router — so it fails against `main` as it
exists now and passes only once the gate is mounted.

**Test isolation.** All live-category tests run on the ephemeral test stack and emit
telemetry tagged `env=test*` only (R-108-O6, G115). No test writes to prod monitoring.

---

## 9. Documentation & Release Impact

| File | Required change |
|---|---|
| `docs/Operations.md` | Under the existing **"Authentication Metrics"** heading: document the three new `smackerel_auth_corpus_grant_*` metrics with their closed label sets, plus the UC-108-001 observation runbook (the `sum by (user_id, route_group)` query, the go/no-go criterion, and the OBSERVE→ENFORCE→rollback procedure from §6). |
| `docs/API.md` | Add `corpus:read` to the per-endpoint scope column for the eight corpus route groups in §2; document the 403 denial envelope and its zero-leakage guarantee; state explicitly which routes are NOT gated and why. |
| `docs/smackerel.md` §17.2 | Update the caller-surface table with the §5 compatibility matrix: which surfaces break at ENFORCE and what the operator must issue. Record that granting is a token rotation, not a flag flip. |
| `config/release-trains.yaml` | No structural change — this spec targets the existing `next` train (`phase: active`, `target_slot: staging`). Confirm `flags_bundle: config/feature-flags.next.yaml` still resolves. |
| `config/feature-flags.next.yaml` | Add `corpusGrantEnforcement: false` with an owning-spec comment (R-108-CFG1). `bubbles.train` flips it to `true` only after a clean observation window. |
| `config/feature-flags.mvp.yaml` | Add `corpusGrantEnforcement: false` plus the `metadata:` block recording `owning_spec: specs/108-corpus-grant-enforcement/`, `introduced_in_train: next`, `introduced_at` (R-108-CFG2, R-108-FL4). `mvp` observes-and-counts, never denies. |
| `config/smackerel.yaml` | Declare `auth.corpus_grant_enforcement` with **no default value**, fail-loud on absence (R-108-CFG3, R-108-FL5). |

Flag lifecycle (R-108-FL7): the flag dies with its train + one cycle. Once enforcement is
permanent, `corpusGrantEnforcement`, the observe branch, and the two would-deny counters are
retired together. `state.json` carries `releaseTrain: "next"` and
`flagsIntroduced: ["corpusGrantEnforcement"]`.

---

## Capability Model

### Single-Implementation Justification

This design introduces exactly one implementation of one capability — "gate a route group on
a named grant" — and deliberately does not build a foundation abstraction. The reason is
concrete: the abstraction **already exists** as `auth.RequireScope`, and it already has two
live implementations in the same router (`annotation:edit`, `knowledge-graph:read`).
`corpus:read` is the third caller of an existing extension point, not a new axis. Building a
"corpus authorization framework" on top would be a second abstraction over a one-line
middleware mount, with no second variation axis to justify it. The only genuinely new element
is the observe→enforce stage machine, and it has exactly one mode selector, one config key,
and no provider variation.

---

## Complexity Tracking

| Deviation from simplest viable approach | Simpler alternative considered | Why rejected |
|---|---|---|
| Two-stage OBSERVE→ENFORCE rollout instead of flipping enforcement on directly | Mount `auth.RequireScope("corpus:read")` and ship it | The denial set is currently **unknown** — F-108-TELEGRAM-01 proves at least one surface breaks, and the PWA/extension surfaces break for every daily user. Shipping straight to ENFORCE means discovering the blast radius in production. The observe stage is the minimum instrumentation that answers UC-108-001 before anyone is denied. |
| Three new metrics rather than reusing `smackerel_auth_scope_rejected_total` | Reuse the existing rejection counter | R-108-O2: a would-be denial and a real rejection would be indistinguishable, which destroys the only signal the observe stage exists to produce. |
| Stage resolved at startup rather than hot-reloadable | Hot-reload the stage from config | A hot-reload path must define behavior when the reloaded value is absent or malformed; every answer is a silent default, forbidden by `smackerel-no-defaults`. Startup-only resolution keeps one fail-loud validation point. |
