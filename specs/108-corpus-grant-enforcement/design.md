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
(implementing derivation), not as a *choice*. F-108-SURFACE-01 / R-108-PRE1 (`corpus` not yet in
`auth.RegisteredScopeSurfaces`) was never a direction question and remains a stage-2
prerequisite, planned as Scope 01.

**Updated 2026-08-11 — F-108-UX-ROSTER-01 is DECIDED, see §10.** Derivation's blocking dependency
on server-side grant readability is resolved by design: the issued scope set is recorded on
`auth_tokens` in the same function that mints the token, and read back as "the recorded grant set
of the principal's current standing token". Persisting a `role` on `auth_users` and deriving
through `GrantsForRole` was evaluated and **rejected** — `GrantsForRole` has zero production
callers today, and no role's grant set contains `annotation:edit`, so role derivation would stand
up a second authority vocabulary *and* silently revoke the bridge's only current capability.
Decided is not shipped: §10.11 routes the scoping and the record-before-derive ordering to
`bubbles.plan`, and `SCN-108-F02` stays blocked until the work lands.

**Ratification impact on this design — RECONCILED 2026-08-12 by `bubbles.design`.**
§18 decision 5 brings the eight Phase-5 corpus-derived intelligence endpoints in scope, taking
the gated surface from **8 to 16** route groups (`spec.md` §4.2 Tier B). The planning packet that
recorded the supersession deliberately left the design-owned surfaces alone rather than rewriting
settled design content. Those surfaces are now corrected here, against source rather than against
the spec prose: §2 lists all sixteen route groups, and the §8 T2/T4/T8 count language reads
sixteen. Every count in this file was re-derived from `internal/metrics/auth.go`,
`internal/api/router.go`, and their tests; see §2 for the citations. Nothing was harmonised by
symmetry.

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
`r.Group(func(r chi.Router) { r.Use(deps.bearerAuthMiddleware) ... })`, wrapping the sixteen
corpus route registrations listed in §2. The outer `bearerAuthMiddleware` must run first —
it is what populates the session that `AuthorizeGrant` reads. The gate is mounted *inside*
that group and *outside* the individual `r.Get` / `r.Post` / `r.Route` calls.

**Why the middleware, not the handlers.** The router file is the repo's route manifest.
Two live precedents already prove the pattern in the same function: `annotation:edit` and
`knowledge-graph:read`. Mounting on the group means (a) a new corpus route added inside the
group is gated by construction, (b) the contract test in §8 can assert the manifest
statically, and (c) `RequireScope`'s existing source switch — which lets shared-token and
bootstrap sessions through — applies uniformly instead of being re-implemented sixteen times.

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

Route-group labels are a **closed set of sixteen** values (R-108-O3, widened by `spec.md` §18
decision 5) — never the raw path. The label set is not merely documented as sixteen, it is
sixteen in code: `internal/metrics/auth.go:219-236` declares the sixteen `CorpusRouteGroup`
constants, `:240-257` fixes their canonical Tier A → Tier B order, and
`internal/metrics/corpus_grant_test.go:162-166` fails the build if `CorpusRouteGroups()` returns
any other count. **The label set was widened with the route surface; the two do not diverge.**

The numbering below is the canonical order of `spec.md` §4.2 and `metrics.CorpusRouteGroups()`.
An earlier revision of this table swapped groups 7 and 8 (`knowledge` before `context_for`); that
ordering never matched the ratified spec or the metrics slice and is corrected here.

### Tier A — raw corpus retrieval (groups 1–8)

| # | Route(s) | Method | Handler | `route_group` label | Gated |
|---|---|---|---|---|---|
| 1 | `/api/search` | POST | `deps.SearchHandler` | `search` | YES |
| 2 | `/api/digest` | GET | `deps.DigestHandler` | `digest` | YES |
| 3 | `/api/recent` | GET | `deps.RecentHandler` | `recent` | YES |
| 4 | `/api/artifact/{id}` | GET | `deps.ArtifactDetailHandler` | `artifact_detail` | YES |
| 5 | `/api/artifacts/{id}/domain` | GET | `deps.DomainDataHandler` | `artifact_domain` | YES |
| 6 | `/api/export` | GET | `deps.ExportHandler` | `export` | YES |
| 7 | `/api/context-for` | POST | `deps.ContextHandler.HandleContextFor` | `context_for` | YES |
| 8 | `/api/knowledge/concepts`, `/concepts/{id}`, `/entities`, `/entities/{id}`, `/lint`, `/stats` | GET | `deps.Knowledge*Handler` | `knowledge` | YES |

### Tier B — corpus-derived Phase-5 intelligence (groups 9–16, §18 decision 5)

| # | Route(s) | Method | Handler | `route_group` label | Gated |
|---|---|---|---|---|---|
| 9 | `/api/expertise` | GET | `ExpertiseHandler(deps.IntelligenceEngine)` | `expertise` | YES |
| 10 | `/api/learning-paths` | GET | `LearningPathsHandler(deps.IntelligenceEngine)` | `learning_paths` | YES |
| 11 | `/api/subscriptions` | GET | `SubscriptionsHandler(deps.IntelligenceEngine)` | `subscriptions` | YES |
| 12 | `/api/serendipity` | GET | `SerendipityHandler(deps.IntelligenceEngine)` | `serendipity` | YES |
| 13 | `/api/content-fuel` | GET | `ContentFuelHandler(deps.IntelligenceEngine)` | `content_fuel` | YES |
| 14 | `/api/quick-references` | GET | `QuickReferencesHandler(deps.IntelligenceEngine)` | `quick_references` | YES |
| 15 | `/api/monthly-report` | GET | `MonthlyReportHandler(deps.IntelligenceEngine)` | `monthly_report` | YES |
| 16 | `/api/seasonal-patterns` | GET | `SeasonalPatternsHandler(deps.IntelligenceEngine)` | `seasonal_patterns` | YES |

Tier B carries the same grant and the same denial shape as Tier A. The split is documentation,
not a difference in authority: all sixteen groups compute over the one global corpus, so a
bearer-only Tier B would have left a partial boundary.

**Two registrations are conditional, and both conditionals sit INSIDE the gated group.** Group 7
is registered only when `deps.ContextHandler != nil`, and the whole of Tier B only when
`deps.IntelligenceEngine != nil`. Enclosing the gate in the conditional instead would register
those routes ungated exactly when the dependency is wired — that is, precisely when they serve
corpus content. It is also an assertion hazard: a route-manifest test built with a nil engine
would observe eight gated groups and call the surface complete. See `spec.md` §18 decision 5 and
SCN-108-G05.

Group 8 is registered via `r.Route("/knowledge", ...)`; the gate attaches to the enclosing group
so all six knowledge endpoints inherit it as one unit and a seventh cannot be added ungated.

**Verified against source 2026-08-12.** `internal/api/router.go:131-203` mounts one
`r.Group(...)` carrying `auth.RequireScope(auth.GrantGlobalCorpusRead)` (`:134`, ENFORCE only)
over exactly these sixteen registrations, each with its own
`corpusGate.Observe(metrics.CorpusRouteGroup*)`. The route paths and methods above match the
expected mount manifest in `internal/api/router_corpus_gate_test.go:82-106` row for row. Note
that `spec.md` §4.2 cites pre-implementation `router.go` line numbers (`:89`, `:238-250`) that
the landed code has since moved; the route names, methods, and labels agree exactly, only those
stale line citations do not.

### Routes deliberately NOT gated

| Route | Method | Why not |
|---|---|---|
| `/api/capture` | POST | Write path. Ingest is not a corpus *read*; gating it would break capture for every daily user. |
| `/api/assistant/turn` | POST | Already gated by the `assistant:turn` claim in the `PreFacadeChain` wired in `cmd/core/wiring_assistant_facade.go`. Its corpus access is mediated, not raw. |
| `/api/artifacts/{id}/annotations*`, `/api/annotations`, `/api/artifacts/{id}/tags/{tag}` | POST/GET/DELETE | Already gated by `auth.RequireScope("annotation:edit")`. Annotation bodies are user-authored, not corpus content. |
| `/api/topics`, `/api/people`, `/api/places`, `/api/time`, `/api/graph/edges` | GET | Already gated by `auth.RequireScope("knowledge-graph:read")`, which IS in `dailyUserGrants`. Adding `corpus:read` here would revoke a grant daily users legitimately hold. |
| `/api/bookmarks/import` | POST | Write path. |
| `/api/internal/telegram-message-artifact` | POST/GET | Internal id↔id mapping; returns no corpus content. |
| `/api/expertise` and the other Phase-5 intelligence endpoints | GET | ~~Out of scope per spec §12 Non-Goals~~ — **SUPERSEDED 2026-07-29 by `spec.md` §18 decision 5 (F-108-ADJ-01): these eight endpoints are now IN SCOPE and gated as Tier B.** They compute over the same global corpus, so leaving them bearer-only would have left a partial boundary. They are listed as groups 9–16 in the §2 table above (reconciled 2026-08-12); see `spec.md` §4.2 Tier B for the canonical inventory and `scopes.md` Scope 03 (SCN-108-G04, SCN-108-G05) for the plan. |
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
| `smackerel_auth_corpus_grant_would_deny_total` | Counter | `route_group` (closed set of 16), `user_id`, `session_source` | A request that WOULD be denied under ENFORCE but was allowed under OBSERVE. |
| `smackerel_auth_corpus_grant_allowed_total` | Counter | `route_group`, `user_id`, `session_source` | A request that carried the grant. Gives the denominator, and — with `user_id` — makes a GRANTED principal's traffic attributable, which is what closes the §18 decision 1(b) coverage bar without operator attestation (F-108-COVERAGE-LABEL-01, resolved). |
| `smackerel_auth_corpus_grant_enforcement_mode` | Gauge | none | `0` = OBSERVE, `1` = ENFORCE. Lets a dashboard state the stage without reading config. |
| `smackerel_auth_scope_rejected_total` | Counter (existing) | existing labels | Real 403s in ENFORCE. Unchanged — deliberately NOT reused for the observe signal (R-108-O2). |

Cardinality: `route_group` is closed; `user_id` follows the existing precedent in
`internal/metrics/auth.go` and is bounded by the operator-controlled principal count;
`session_source` is the existing closed session-source enum. No raw path, ever (R-108-O3/O4).

**Coverage query (§18 decision 1(b)).** A `(user_id, route_group)` cell is closed by
observed traffic of EITHER outcome, so the coverage set is the union of the two counters:

```promql
sum by (user_id, route_group) (
    increase(smackerel_auth_corpus_grant_allowed_total[14d])
  or
    increase(smackerel_auth_corpus_grant_would_deny_total[14d])
)
```

Cells absent from that result are the ones still needing an operator `idle-by-design`
attestation. Before `allowed_total` carried `user_id` this query was not expressible —
a granted principal that used a route group produced no per-principal series, so it was
indistinguishable from a principal that never called, and every cell for every granted
principal fell to attestation. That was F-108-COVERAGE-LABEL-01.

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
| Telegram bridge | per-user bearer whose scope claim is DERIVED from the mapped principal's persisted grants | **MEASURED 2026-08-13** — same as the principal. The hardcoded `["annotation:edit"]` list is gone (`deriveGrants`, `per_user_token.go`), so the bridge can only narrow the principal's recorded set, never widen it. | **YES** if the principal is a daily user — the same condition as the extension row, no longer a bridge-specific unknown | **RATIFIED 2026-07-29 (`spec.md` §18 decision 3): derive the minted token's scope claim from the mapped principal's persisted grant set.** The earlier two-option framing ("receive a token carrying `corpus:read`" **or** "re-route through `/api/assistant/turn`") is **closed** — both were minter-side or routing-side workarounds that leave authority defined somewhere other than the principal. Derivation is the larger change and the only one consistent with spec 044 Scope 02 and the persisted-grant doctrine. ~~Depends on F-108-UX-ROSTER-01 (grants are not readable server-side today).~~ **The readability dependency is DECIDED 2026-08-11 — see §10: the issued scope set is recorded on `auth_tokens` and read back per principal.** **SHIPPED 2026-08-13:** derivation is implemented and proven by `TP-04-01` (unit), `TP-04-02` (bridge→API integration), `TP-04-08` (hardcoded list gone) and `TP-04-09` (adversarial). Two failure modes are distinguished and both are operator-actionable: a principal holding a delegable non-corpus grant mints successfully and is refused at the corpus route (403, permanent); a principal with no delegable grant aborts at mint with a NAMED condition that identifies the principal to rotate. |
| Internal service-to-service (`/api/context-for`, GuestHost connector) | shared token | Bypasses the scope check per `RequireScope`'s source switch | NO | None — but the bypass MUST be asserted by test (§8) so it is a documented decision, not an accident. **`spec.md` §18 decision 4 (2026-07-29): the GuestHost connector credential does NOT receive `corpus:read`.** Its guest-context reads move to the spec-109 MCP `hospitality-read` path under its own audience-bound credential (spec 109 D3), which is itself blocked on BUG-019-003. Coordination owner: `bubbles.design` on spec 109. |
| Bootstrap session | bootstrap | Bypasses the scope check per `RequireScope`'s source switch | NO | None. |
| Prometheus scrape / orchestrator probes | unauthenticated | n/a | NO | None — `/metrics`, `/readyz`, `/api/health` are ungated. |

The OBSERVE window exists precisely to convert the "unknown" rows above into measured rows
before anyone is denied.

**Status 2026-08-13 — no `unknown` rows remain in this table.** The Telegram bridge was the
only one, and it was unknown for a structural reason rather than a lack of observation: the
minter held a hardcoded scope list, so the bridge's authority was not derivable from any
principal and no amount of watching traffic would have settled it. Decision 3 removed the
list, so the row is now determined by construction and measured by test.

What the OBSERVE window still owes is a different question — not "what authority does this
surface carry" (settled) but "which real principals actually exercise which route groups"
(§18 decision 1(b) coverage). That is answered by the union query in §4, which became
expressible once `allowed_total` gained `user_id` (F-108-COVERAGE-LABEL-01, resolved).

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
| T2 | `unit` | `internal/metrics/corpus_grant_test.go` | The three new metrics register in the `smackerel_auth_*` family with the closed **sixteen-value** `route_group` label set; an unknown label value is rejected. The set is asserted against an independent in-test literal, not derived from `CorpusRouteGroups()`, so adding a seventeenth constant cannot silently update the expectation. |
| T3 | `unit` | `cmd/core` config resolution test | Absent `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` aborts startup naming the variable; a malformed value aborts naming the value; neither silently selects a stage (SCN-108-C03). |
| T4 | `integration` | `internal/api` against the ephemeral test stack | OBSERVE: ungranted principal gets **200** on all sixteen route groups AND `..._corpus_grant_would_deny_total` increments with the right `route_group`. ENFORCE: same principal gets **403** on all sixteen. The fixture MUST wire a non-nil `IntelligenceEngine`, or Tier B is simply absent and the run reports eight-of-eight as a pass. |
| T5 | `integration` | same | Shared-token and bootstrap sessions pass under ENFORCE (documented `RequireScope` source-switch bypass), so the bypass is asserted, not assumed. |
| T6 | `e2e-api` | `./smackerel.sh test e2e` | Full stack, real Postgres: granted operator token reads `/api/search` and `/api/export`; ungranted daily-user token is refused on both; the refusal body contains no artifact id, title, or count. |
| T7 | `e2e-api` | same | Route-group parity: a denied `/api/artifact/{id}` for a real id and for a random id produce byte-identical responses (no existence oracle). |

### T8 — Adversarial: the gate cannot be silently removed (REQUIRED)

Add a **route-manifest contract test** covering the full gated surface. The planned home was
`internal/api/auth_surface_contract_test.go` — the only referent of `GateGlobalCorpusRead` when
this design was written. The landed test is `internal/api/router_corpus_gate_test.go`: the
manifest assertions need the real router and the mount-walking helpers, and the surface-contract
file stays focused on the decision function and the scope-surface registration. Both files exist;
neither duplicates the other.

- Build the real router via the same constructor production uses, with ENFORCE selected.
- Enumerate the sixteen corpus routes from a canonical in-test list (the same sixteen
  `route_group` values as §2, Tier A then Tier B).
- For each, issue a request with a session whose scope claim is empty and assert **403**.
- Assert the **set equality** of "routes the test knows about" against the router's mounted
  corpus group, so adding a seventeenth corpus route without adding it to the list fails the test.
- Construct the router with a **non-nil** `IntelligenceEngine`. With a nil engine, Tier B is
  never registered, so the walk finds eight gated groups and set equality still holds over the
  routes that exist. The test would pass while eight groups went unexercised.

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
| `docs/API.md` | Add `corpus:read` to the per-endpoint scope column for the sixteen corpus route groups in §2; document the 403 denial envelope and its zero-leakage guarantee; state explicitly which routes are NOT gated and why. |
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

## 10. Server-Side Grant Readability — F-108-UX-ROSTER-01 DECIDED

**Status of the finding: DECIDED 2026-08-11 by `bubbles.design`, the routed owner of the
derivation mechanism (`spec.md` §16 / UX findings table, owner `bubbles.design` → `bubbles.plan`).
Chosen mechanism: record the issued scope set on `auth_tokens` (option (b), token-scoped), NOT a
role column and NOT a per-principal intent column.**

Decided means the mechanism is settled and no further owner decision is required. It does **not**
mean the mechanism has shipped. `scenario-manifest.json` keeps `SCN-108-F02` `blockedBy:
["F-108-UX-ROSTER-01"]` until the work in §10.9 lands, and Scope 04's DoD item stays unchecked
until then. Recording a decision is not recording an implementation.

### 10.1 What the finding actually blocks

`spec.md` §18 decision 3 ratifies that the Telegram bridge derives its minted scope claim from
the mapped principal's persisted grant set. Derivation needs one primitive that does not exist:
**given a `user_id`, return that principal's authoritative grant set, server-side, without
possessing a wire token.**

The grant set exists today in exactly one place — inside issued PASETO tokens. `auth_tokens`
stores `hashed_token`, an HMAC-SHA-256 under `auth.at_rest_hashing_key`, so the claim is not
recoverable from the row. The only read path is `smackerel auth inspect <wire-token>`
(`cmd/core/cmd_auth.go:637`), and `internal/api/admin_ui_static/tokens.html:63-64` states the
wire token is shown once and never displayed again. The primitive is therefore absent, not
merely inconvenient.

### 10.2 Verified current state — three facts that decide the option

Re-verified against the working tree at design time. Each is load-bearing.

| # | Fact | Evidence |
|---|---|---|
| V1 | **The role model has ZERO production callers.** `GrantsForRole` and `SessionWithRole` are referenced only by `browser_session_policy.go` itself and by test files. `RoleDailyUser` / `RoleOperator` never appear in a non-test call site. | `grep -rn 'GrantsForRole\|SessionWithRole' --include='*.go' .` → `browser_session_policy.go` (definition) + `*_test.go` only |
| V2 | **No role's grant set contains `annotation:edit`.** `dailyUserGrants` is `[assistant:turn, knowledge-graph:read]`; `operatorGrants` adds `corpus:read`, `operator:admin`, `operator:model-picker`. `annotation:edit` is in **neither**. | `browser_session_policy.go:54,59-65`; `grep -rn 'annotation:edit' internal/auth/` matches only a doc comment in `scopes.go:36` |
| V3 | **Real production authority is the explicit `--scope` list the operator supplies at issuance**, carried into the PASETO claim and parsed back out by `VerifyAndParse` (`verify.go:280`). `RequestAuthenticator` deliberately performs **no database query on an authenticated request** (`request_authenticator.go:105-108`). | `cmd_auth.go:131-149` → `IssueAndPersistToken` → `IssueToken(Scopes: opts.Scopes)` |

V1 and V3 together reframe the tradeoff the finding was written against. There is no live
`GrantsForRole` authority source for a new store to "stay reconciled with". The live source is
the explicit per-principal scope list, and it is already per-principal and already explicit — it
is simply **write-only**. This design does not add an authority source. It makes the existing one
legible.

### 10.3 Options evaluated

| Option | Shape | Verdict |
|---|---|---|
| **(a)** Persist `role` on `auth_users`, derive via `GrantsForRole` | Role → grant-set lookup | **REJECTED.** Three independent grounds below. |
| **(b-users)** Persist an explicit grant set on `auth_users` | Per-principal intent column | **REJECTED.** Creates the drift the finding warns about. |
| **(b-tokens)** Persist the issued scope set on `auth_tokens` | Per-token record of issuance | **CHOSEN.** |
| **(c1)** Recover grants from the stored token | — | **IMPOSSIBLE.** `hashed_token` is an HMAC; the claim is not recoverable. |
| **(c2)** Expose grants over an internal HTTP endpoint the bridge calls | Indirection over the same read | **REJECTED.** Same database read plus a new endpoint that itself needs authorization. Strictly more surface, no new property. |
| **(c3)** Declare a principal → grants roster in SST config | Config-file authority | **REJECTED.** Puts authority in a deploy artifact. Granting would become a config edit plus a redeploy, contradicting F-108-GRANT-MECHANISM-01, and creates a second authority source outside the token — the shape §18 decision 3 forbids, relocated. |

**Why (a) is rejected.**

1. **It would make `GrantsForRole` a production authority source for the first time (V1).** The
   role model is test-fixture scaffolding today. Adopting it here would stand up a *second*
   authority vocabulary — roles — beside the live one, the explicit `--scope` list written by
   `auth enroll` / `auth rotate`. Two vocabularies drift, which is the exact objection §18
   decision 3 raised against the minter-side list. Option (a) does not avoid that failure; it
   relocates it from the minter to the role table.
2. **It cannot express the grant the bridge needs today (V2).** Deriving from `GrantsForRole`
   yields a Telegram token **without** `annotation:edit`, silently revoking the one capability
   the current hardcoded list exists to provide. Repairing that requires either widening a role
   default — forbidden for `dailyUserGrants` by §18 decision 2, and equally a default-widening
   for `operatorGrants` — or adding a per-principal extras column, which collapses option (a)
   into option (b) while keeping the role table as dead weight.
3. **Role granularity is strictly coarser than the authority the CLI already issues.** An
   operator can already mint `extension:bookmarks,history` for one principal. No role expresses
   that. A role column could not round-trip the grant sets the system issues today.

**Why (b-users) is rejected.** A per-principal grant column expresses *intent*, which is a
different quantity from *what the principal's token actually carries*. The two diverge the moment
intent is edited without a rotation, producing a "granted but not effective" state and an
unanswerable question about which one authorizes. That ambiguity is precisely what the reviewer
of this finding warned against, and it is why the record belongs on the token, not the principal.

**Why (b-tokens) is chosen.** Recording the scope set on the token row is a **record of
issuance**, not an intent store. It is written in the same function that mints the token, from
the same value that becomes the claim, so it cannot disagree with the token by construction. It
adds legibility and no authority. It is also the continuation the schema author already planned:
`033_auth_per_user_bearer.sql:19-20` says "per-user permission scopes … deferred to later scopes
/ specs".

### 10.4 Mechanism

**Schema (migration `063_auth_token_granted_scopes.sql`):**

```sql
ALTER TABLE auth_tokens ADD COLUMN granted_scopes text[];
```

Nullable, **no default**. The nullability carries meaning and the three states are distinct:

| Value | Meaning | Produced by |
|---|---|---|
| `NULL` | **Unknown.** The token was issued before grant recording existed. | The migration only. Never by the write path. |
| `'{}'` | **Recorded as none.** The token was issued with no scope claim. | An issuance that supplied no `--scope`, or the demote sentinel `--scope ""`. |
| `'{corpus:read,…}'` | **Recorded set.** Exactly the claim in the token. | An issuance with explicit scopes. |

Conflating `NULL` with `'{}'` would assert that a pre-existing token was issued with no grants.
That is a claim about authority that nobody made. `spec.md` S7 already forbids the UI equivalent
("rendering a guess, a default, or an empty set would be fabricated authority state"); the same
prohibition applies to the column that feeds it. A `NOT NULL DEFAULT '{}'` column would commit
exactly that error at migration time for every existing row.

**Write invariant — one point, atomic by construction.** There are exactly three token-issuance
call sites in the tree:

| Call site | Path | Writes an `auth_tokens` row | Records `granted_scopes` |
|---|---|---|---|
| `cmd/core/cmd_auth.go:133` (CLI enroll / rotate / bootstrap) | `IssueAndPersistToken` | YES | YES |
| `internal/api/auth_handlers.go:299` (admin API mint) | `IssueAndPersistToken` | YES | YES, as `'{}'` — the admin API has no scope parameter (F-108-UX-ADMINUI-01) |
| `internal/telegram/per_user_token.go:187` (bridge mint) | `IssueToken` only | **NO** | **NO** |

`IssueAndPersistToken` (`internal/auth/issue.go:226`) already holds both halves: it passes
`opts.Scopes` to `IssueToken` and then calls `store.PersistToken`. Threading the same
`opts.Scopes` into `PersistTokenParams` makes the record and the claim the same value from the
same variable in one function. Drift is not prevented by discipline; it is unrepresentable.

**The bridge MUST NOT persist.** `MintForUser` calls `IssueToken` directly and never touches
`auth_tokens`. That is what stops a feedback loop in which a derived, ceiling-narrowed bridge
token is later read back as the principal's standing grant set and ratchets authority downward on
every message. The property holds today by construction; §10.10 T5 asserts it so a later refactor
cannot silently break it.

**Read (new `BearerStore` method):**

```go
// RecordedGrants is the server-side answer to "what does this principal hold?"
type RecordedGrants struct {
    TokenID  string
    Scopes   []string // meaningful only when Recorded is true
    Recorded bool     // false ⇒ granted_scopes IS NULL (issued before recording)
}

// GrantsForPrincipal returns the recorded grant set of the principal's current
// standing token. Returns ErrPrincipalNotProvisioned when no such token exists.
func (s *BearerStore) GrantsForPrincipal(ctx context.Context, userID string) (RecordedGrants, error)
```

**"Current standing token" is defined, not inferred:** the row for `user_id` with
`status = 'active'` and `expires_at > now()`, ordered `issued_at DESC, token_id DESC`, limit 1.
Rotation moves the prior token to `'rotated'`, so the newest active row is the operator's latest
issuance. Expired and revoked rows are excluded, so a principal whose token lapsed holds nothing
and derivation fails closed.

No new index. `ix_auth_tokens_user_id` already covers the predicate, and the read runs once per
inbound Telegram message rather than once per HTTP request. Adding an index for an
operator-scale table with no measured pressure would be speculation.

### 10.5 Migration and backfill

**Backfill is `NULL` for every existing row. No value is invented.**

The alternatives were considered and rejected:

- Backfill `'{annotation:edit}'` to preserve today's Telegram behavior. This asserts a grant no
  operator issued. It is fabricated authority state.
- Backfill from `dailyUserGrants`. This both fabricates and widens, and it is a default.
- Backfill `'{}'`. This asserts "issued with no grants" for tokens that demonstrably carry
  grants.

**What an already-enrolled principal therefore gets: nothing, explicitly marked unknown.** That
is safe because unknown fails closed everywhere it is consumed — derivation refuses to mint,
`list-users` prints `unknown`, and S7 renders `── unknown ──` exactly as `spec.md` already
requires. The remedy is already ratified: `uservalidation.md` item 9 requires proactive rotation
of every unknowable principal **before** the ENFORCE flip, with the operator issuing a deliberate
grant set rather than recovering an unreadable one. This design supplies the column that makes
"which principals are still unknown?" a query instead of a guess.

**Sequencing constraint — record before derive (NEW, and load-bearing).** `annotation:edit` is in
no role default (V2) and is not backfilled. If the minter switches to derivation before the
mapped Telegram principals have recorded grant sets, every bridge annotation write starts failing
with 403 — in OBSERVE stage, where the corpus gate does not deny but the always-on
`annotation:edit` gate does. Enforcement staging does **not** protect this. The two changes must
land in this order:

1. Column, write path, read path, and the `list-users` grants column.
2. Rotate every mapped Telegram principal so their standing token records a deliberate set.
3. Only then switch the minter from the literal to derivation.

`bubbles.plan` owns turning that ordering into scope boundaries (§10.11).

**Rollback.** `DROP COLUMN granted_scopes`, recorded in the migration footer in the same style as
`033`. Nothing reads the column until step 3, and no existing behavior depends on it, so the
column is additive and independently revertible.

### 10.6 Authority: token, database, or intersection

Unambiguous answer, stated per moment so it cannot be read three ways.

| Moment | Authority | Database read? |
|---|---|---|
| **Request-time authorization** (`/api/search`, every gated route) | **The presented token's scope claim. Unchanged.** `AuthorizeGrant` and `GateGlobalCorpusRead` read `Session.Scopes`, populated by `VerifyAndParse`. | **No.** Preserved deliberately — `request_authenticator.go:105-108` records that principal state is an issuance-time policy specifically so no query runs per authenticated request. |
| **Issuance** (`auth enroll` / `rotate`, admin API) | The operator's explicit input. | Write only. The row is a projection of the issuance, not an independent opinion. |
| **Delegated re-issuance** (bridge per-message mint) | **The recorded grant set of the principal's current standing token**, narrowed by the delegation ceiling (§10.7). | **Yes.** This is the one new read. |

**The database is never consulted to authorize a request, and the token is never consulted to
mint a delegation.** They cannot conflict, because they are not two opinions about one question —
the row is written from the same value that becomes the claim, in the same function
(`IssueAndPersistToken`), for the same token.

There is **no intersection at request time**. Introducing one would require a per-request
database read and would contradict an explicit, documented architectural decision. There **is**
an intersection at mint time, but it is between the recorded set and the ceiling, not between the
database and a token.

One consequence worth stating because it is a genuine improvement rather than a compromise: a
standing bearer token keeps its grants until rotation (F-108-GRANT-MECHANISM-01), but a bridge
token is re-derived on every message under a 5-minute TTL (`cmd/core/wiring.go:780`). Narrowing
or revoking a principal's grants therefore takes effect on the bridge within one message, and
fully within the TTL. The delegated path is **more** responsive to grant changes than the direct
path, not less.

### 10.7 Derivation contract for the Telegram bridge

**Reader seam.** `PerUserTokenMinterOptions` gains a required `PrincipalGrantReader`, a
consumer-side interface in `internal/telegram` satisfied by `*auth.BearerStore`:

```go
type PrincipalGrantReader interface {
    GrantsForPrincipal(ctx context.Context, userID string) (auth.RecordedGrants, error)
}
```

`deps.BearerStore` is already constructed at `cmd/core/wiring.go:488` and is in scope at the
minter construction site (`wiring.go:767`), so no new dependency edge is created. A nil reader
**fails minter construction**; it does not degrade.

**Derivation rule:**

```
derived = recorded ∩ TelegramBridgeDelegableGrants
```

**The ceiling narrows; it never confers.** `TelegramBridgeDelegableGrants` is a declared closed
set in `internal/auth`, beside the grant vocabulary rather than in the bridge, so
`internal/telegram` holds no scope literal at all. Its members are the grants required by the
gated internal routes the bridge actually calls:

| Grant | Bridge call site |
|---|---|
| `corpus:read` | `/api/search`, `/api/digest`, `/api/recent`, `/api/knowledge` (`bot.go:174-178`, `recipe_commands.go:476`, `knowledge.go:28`) |
| `annotation:edit` | `/api/artifacts/{id}/annotations` (`annotation.go:178`) |

Enumerated, not sampled: the bridge's other internal calls — `/api/capture`, `/api/health`,
`/api/lists`, `/api/expenses`, `/api/internal/telegram-message-artifact`, `/v1/photos/upload` —
carry no `RequireScope` in `internal/api/router.go`, and the assistant is reached in-process
through `assistant_wiring.go`, not over HTTP, so `assistant:turn` is not required.

**Why a narrowing list is not the shape §18 decision 3 rejected.** The rejected shape is a
minter-side list that **confers** authority the principal may not hold; that is what "defines
authority at the minter rather than the principal" means. A ceiling only **withholds** authority
the principal does hold. `derived ⊆ recorded` for every input, so a ceiling cannot make an
ungranted principal granted and cannot affect the negative case in either direction. The
distinction is mechanical, and §10.10 T2 asserts it as a property rather than trusting the
reading.

**Why the ceiling is worth its cost.** A bridge token is minted **without the principal
presenting a credential** — the bridge synthesizes authority from a chat mapping. Chat mapping is
a weaker binding than token possession: a compromised Telegram account, or one mis-entered
mapping, yields whatever the mapped principal holds. Without a ceiling, mapping an operator's
chat mints tokens carrying `operator:admin` and `operator:model-picker` to serve a `/find`. The
ceiling bounds that blast radius to the two capabilities the bridge exercises. A capability
missing from the ceiling fails closed and surfaces as a 403 on the new command, which is a
visible, correct failure rather than a silent one.

**Anti-drift.** A contract test derives the required set from the bridge's own call sites plus the
router manifest and fails if the declared ceiling omits one. The ceiling is a declared constant
for greppability, with a mechanical check so it cannot rot.

### 10.8 Failure modes — all fail closed, none fall back

| Condition | Outcome |
|---|---|
| Database unreachable at mint time | Mint returns a typed error. No token is produced. |
| No active unexpired token for the principal (`ErrPrincipalNotProvisioned`) | Mint aborts. Distinct telemetry label: the operator must provision this principal. |
| `granted_scopes IS NULL` (pre-migration token) | Mint aborts. Distinct label: unknown, rotate to record. |
| `granted_scopes = '{}'` (deliberate demotion) | Mint aborts. Distinct label: deliberately ungranted. A zero-scope bridge token could do nothing anyway. |
| `recorded ∩ ceiling = ∅` | Mint aborts, same as above. |

**No fallback to the shared bearer — this is the hazard, stated explicitly.** `bearerForChat`
(`internal/telegram/bot.go:298`) already contains a branch that falls back to `b.authToken` when
the minter is nil, which is the dev/test path. Routing a grant-read **error** into that branch
would substitute a different authority source at the exact moment authority could not be
determined. That is a fail-open bug wearing the costume of resilience, and it would silently
undo spec 044 Scope 04. The nil-minter branch stays exactly as it is — a construction-time
condition, never an error-handling path.

**Wiring must fail loud.** `cmd/core/wiring.go:783-789` currently logs a warning and continues
when minter construction fails, leaving production Telegram on the shared bearer. Once the minter
carries the authority derivation, that warn-and-continue is a silent bypass of the whole
mechanism. In production with `auth.enabled`, a minter construction failure MUST abort the
process, consistent with the `os.Exit(1)` already used for Telegram initialization failure
twenty lines above (`wiring.go:762`).

**User-facing rendering.** Every abort reaches the reply site as a **typed** condition, not a
collapsed string, so the bridge renders an operator-actionable permanent state. This is the same
requirement F-108-UX-TELEGRAM-COPY-01 already raises against `handleFind`'s
`? Search failed. Try again in a moment.` (`bot.go:852`). Telling a permanently-ungranted user to
retry is the dishonesty class BUG-061-008/009 ratified against.

### 10.9 Operator surface — what `smackerel auth` must change

| Command | Change | Why |
|---|---|---|
| `auth enroll --scope …` | **No CLI surface change.** The recording happens inside `IssueAndPersistToken`. | The flag already carries the operator's explicit intent; only persistence was missing. |
| `auth list-users` | **Add a `GRANTS` column**, printing the recorded set, or the literal `unknown` for `NULL`. | This is the finding's literal requirement: read a principal's grants without possessing the wire token. Header is `USER_ID  ENROLLED_AT  ENROLLED_BY  STATUS  NOTES` today (`cmd_auth.go:418`). |
| `auth rotate` preserve mode | **Preserve from the recorded set instead of requiring `--prior-token`.** When `granted_scopes IS NULL`, refuse with a message naming the principal — never guess. | `resolveRotationScopes` (`cmd_auth.go:596-620`) can only preserve by parsing a wire token the operator no longer has. This is what makes SCN-108-F02 achievable. |
| `auth inspect` | No change. | Still useful when the operator does hold a wire token. |
| Admin REST / `tokens.html` | No change **here**. | Owned by F-108-UX-ADMINUI-01. Recorded consequence: admin-API mints record `'{}'` because the endpoint has no scope parameter, so a principal whose latest token came from the admin API has no delegable authority until re-issued from the CLI. Honest and actionable, not a silent trap. |

### 10.10 How the negative case is proven

The correctness case for derivation is negative: a principal **without** `corpus:read` must gain
**no** corpus access through Telegram. A test that only asserts "Telegram still works" passes
against a hardcoded list and is therefore worthless here. Five layers, ordered strongest first.

| # | Layer | What it proves | Why it is not vacuous |
|---|---|---|---|
| **T1** | **Structural grep guard** — `internal/telegram/**` contains no string matching `ScopeNameRegex` (`^[a-z][a-z0-9-]*:[a-z0-9,_-]+$`). Precedent: `internal/auth/sst_grep_guard_test.go`. | A minter-side grant list **cannot exist** in the package. | It constrains the source, not a behavior, so no fixture choice can make it pass falsely. It is the only layer that survives an author who is actively trying to reintroduce the shortcut. |
| **T2** | **Subset property test** — for a table of recorded sets `R` (including sets with and without `corpus:read`, empty, and unknown), assert `derived(R) ⊆ R`. | The ceiling narrows and never confers. | Includes an `R` without `corpus:read` and asserts absence in the output. A conferring implementation fails on that row. |
| **T3** | **Fail-closed unit test** — `NULL`, no-active-token, and reader-error inputs each return an error, a zero-valued `MintedTelegramToken`, and **no** shared-bearer substitution. | Absent grant data denies rather than defaults. | Asserts the returned token is zero, so an implementation that mints a scopeless-but-valid token fails; asserts no fallback bearer, so wiring the error into `bearerForChat`'s nil-minter branch fails. |
| **T4** | **Differential integration test (the decisive one)** — one mapped chat, one principal whose recorded set is exactly `{annotation:edit}`. Under ENFORCE: the corpus command is refused **403**, and in the **same test with the same mint**, the annotation write **succeeds**. | Authority tracks the principal per capability. | This is the anti-vacuity design. A regression to a hardcoded `{annotation:edit, corpus:read}` list fails the negative arm. A derivation that drops everything, or a bridge that is simply broken, fails the positive arm. Neither "everything works" nor "everything is broken" can pass. A single-arm negative test would pass in the broken case, which is exactly why the arms are paired on one principal. |
| **T5** | **No-persist invariant test** — after N bridge mints, `auth_tokens` row count for the principal is unchanged and `granted_scopes` is untouched. | A derived token never becomes the next derivation's input. | Fails the moment a refactor points the bridge at `IssueAndPersistToken`, which would ratchet authority down on every message. |

**Adversarial demonstration required, not optional.** T4 must be shown to **fail** against a tree
patched back to the literal list, and T2 must be shown to fail against a union implementation.
Neither is proven by a green run on the fixed tree. These are pre-fix red demonstrations, and
they are separate evidence from the passing run.

### 10.11 What this does NOT close, and what is routed onward

Stated so a later reader does not over-read the decision.

- **F-108-UX-ROTATE-ADD-01 stays open.** §10.9 removes its documented blocker — the operator can
  now read the current list, so the R-108-DOC2 "type the full list" rule becomes performable, and
  preserve-mode no longer needs a wire token. Whether an additive `--add-scope` primitive lands is
  that finding's call, not this one's. Merging it here would be silent absorption.
- **F-108-UX-ADMINUI-01 stays open.** This design supplies the read the grant chip needs and the
  exact `unknown` semantics S7 requires. It does not build the editor.
- **§18 decision 2 is untouched.** `dailyUserGrants` is not read, not widened, and not referenced
  by this mechanism. `TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants` is unaffected —
  derivation reads a recorded per-token set, never a role default.
- **Scope 04's Change Boundary does not contain this work.** Scope 04 admits the Telegram minter
  path and test fixtures. This mechanism adds a migration, a `BearerStore` write and read, CLI
  output and rotate-preserve changes, and a wiring fail-loud change — none of which fit. It also
  imposes the record-before-derive ordering in §10.5, which is a sequencing constraint across
  scopes rather than inside one. **Routed to `bubbles.plan`:** give the mechanism its own scope
  ordered before Scope 04, or widen Scope 04's boundary explicitly and record the ordering in its
  DoD. `bubbles.design` does not edit scope boundaries.
- **`scenario-manifest.json` is unchanged.** `SCN-108-F02` remains `blockedBy:
  ["F-108-UX-ROSTER-01"]` because the scenario is blocked by the *work*, which has not shipped.
  Clearing it now would convert a decision into a false claim of delivery.

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

**§10 addendum (2026-08-11).** The `PrincipalGrantReader` seam introduced in §10.7 is a
consumer-side interface with exactly **one** production implementation, `*auth.BearerStore`, plus
a test fake. It is a Go testability seam, not a variation axis: there is no second grant store, no
pluggable provider, and no configuration that selects between implementations. Building a
foundation abstraction over one database read would be an abstraction with nothing to vary. If a
second authority store is ever proposed, that proposal is the trigger for capability-foundation
work — and it would first have to supersede §10.6, which states that the record is a projection of
issuance rather than an independent source.

---

## Complexity Tracking

| Deviation from simplest viable approach | Simpler alternative considered | Why rejected |
|---|---|---|
| Two-stage OBSERVE→ENFORCE rollout instead of flipping enforcement on directly | Mount `auth.RequireScope("corpus:read")` and ship it | The denial set is currently **unknown** — F-108-TELEGRAM-01 proves at least one surface breaks, and the PWA/extension surfaces break for every daily user. Shipping straight to ENFORCE means discovering the blast radius in production. The observe stage is the minimum instrumentation that answers UC-108-001 before anyone is denied. |
| Three new metrics rather than reusing `smackerel_auth_scope_rejected_total` | Reuse the existing rejection counter | R-108-O2: a would-be denial and a real rejection would be indistinguishable, which destroys the only signal the observe stage exists to produce. |
| Stage resolved at startup rather than hot-reloadable | Hot-reload the stage from config | A hot-reload path must define behavior when the reloaded value is absent or malformed; every answer is a silent default, forbidden by `smackerel-no-defaults`. Startup-only resolution keeps one fail-loud validation point. |
| §10: record the issued scope set on `auth_tokens` rather than deriving grants from a `role` column on `auth_users` | Persist `role`, derive via the existing `GrantsForRole` | `GrantsForRole` has **zero production callers** (§10.2 V1), so adopting it would stand up a second authority vocabulary beside the live one — the explicit `--scope` list — which is the drift §18 decision 3 rejected, relocated from the minter to the role table. It also cannot express `annotation:edit` (V2), which no role grants, so role derivation would silently revoke the bridge's only current capability. |
| §10: nullable `granted_scopes` with three distinct states | `NOT NULL DEFAULT '{}'` | A default backfills every pre-existing row as "issued with no grants", asserting an authority fact nobody established. `spec.md` S7 already forbids the UI equivalent; the same prohibition applies to the column that feeds it. The three-state column is the only shape that can say *unknown*. |
| §10: delegation ceiling `derived = recorded ∩ TelegramBridgeDelegableGrants` | `derived = recorded` — no ceiling | A bridge token is minted **without the principal presenting a credential**; authority is synthesized from a chat mapping, which is a weaker binding than token possession. Without a ceiling, mapping an operator's chat mints tokens carrying `operator:admin` to serve a `/find`. The ceiling only withholds (`derived ⊆ recorded` always), so it cannot confer authority and cannot affect the negative case — asserted as a property by §10.10 T2, not left to reading. |
| §10: minter construction failure aborts the process in production | Keep the existing warn-and-continue at `wiring.go:783-789` | Once the minter carries the authority derivation, warn-and-continue leaves production Telegram on the shared bearer — a silent bypass of the entire mechanism and of spec 044 Scope 04. Fail-loud matches the `os.Exit(1)` already used twenty lines above for Telegram init failure. |
