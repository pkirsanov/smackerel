# Scopes: 108 Corpus Grant Enforcement

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Planning only — no source edited.**
**Sources:** [`spec.md`](spec.md) · [`design.md`](design.md) · **Release train:** `next`
**Flag introduced:** `corpusGrantEnforcement`

---

## Execution Outline

### Phase Order

1. **Scope 01 — Scope Registration Prerequisite.** Add `corpus` to `auth.RegisteredScopeSurfaces` so the operator can actually mint a token carrying `corpus:read`. Resolves F-108-SURFACE-01 / R-108-PRE1. Blocks every other scope: without it, granting the scope is impossible and every downstream test would be asserting an ungrantable grant.
2. **Scope 02 — Observe-Stage Plumbing.** Fail-loud SST config `auth.corpus_grant_enforcement` → `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT`, the three `smackerel_auth_corpus_grant_*` metrics, and the `corpus_grant_would_deny` structured log fields. The observe middleware is mounted and counts, but **nothing is denied**.
3. **Scope 03 — Gate Mount.** `r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))` on the **sixteen** corpus route groups from `spec.md` §4.2 (Tier A 1–8 + Tier B 9–16, the latter brought in scope by §18 decision 5), mounted only in ENFORCE, honoring the stage machine. Carries the **T8 adversarial route-manifest contract test** and the Tier-B conditional-registration guard.
4. **Scope 04 — Caller Remediation.** The surfaces design.md §5 says break at ENFORCE: PWA/extension daily-user principals and the Telegram bridge (F-108-TELEGRAM-01, whose direction is ratified by §18 decision 3 as **grant derivation**). Also asserts the shared-token / bootstrap bypass is a documented decision, and records that the GuestHost connector credential does **not** receive `corpus:read` (§18 decision 4).
5. **Scope 05 — Docs, Release Train, Flag Bundles.** `docs/Operations.md`, `docs/API.md`, `docs/smackerel.md` §17.2, the `v1`-gate release packet's `docs/releases/v1/features.md`, `config/release-trains.yaml`, `config/feature-flags.next.yaml` (default-ON for the owning train `next`), `config/feature-flags.mvp.yaml` (default-OFF).

### New Types & Signatures

No new authorization primitive is introduced. The following surfaces change:

```go
// internal/auth — EXISTING symbols, no signature change:
//   const GrantGlobalCorpusRead = "corpus:read"
//   func GateGlobalCorpusRead(sess Session) CorpusDecision
//   func RequireScope(required string) func(http.Handler) http.Handler
//   func SessionWithRole(userID, tokenID string, role Role, extraGrants ...string) Session
//        └─ §18 decision 2: the ONLY sanctioned way a daily principal gets corpus:read.
//           dailyUserGrants is NEVER widened.

// internal/auth — Scope 01: registration surface gains one entry
var RegisteredScopeSurfaces = []string{ /* ...existing..., */ "corpus" }

// internal/api — Scope 02: stage-aware observe middleware (new type)
type CorpusGrantGate struct { /* enforce bool; metrics sink; logger */ }
func (g *CorpusGrantGate) Observe(next http.Handler) http.Handler

// cmd/core — Scope 02: fail-loud resolution, no default, aborts on absent/malformed
func resolveCorpusGrantEnforcement(env map[string]string) (bool, error)

// internal/metrics — Scope 02: extends the smackerel_auth_* family
//   smackerel_auth_corpus_grant_would_deny_total{route_group,user_id,session_source}  Counter
//   smackerel_auth_corpus_grant_allowed_total{route_group,session_source}             Counter
//   smackerel_auth_corpus_grant_enforcement_mode                                      Gauge (0|1)

// internal/telegram — Scope 04 (§18 decision 3): the minted scope claim is DERIVED from the
// mapped principal's persisted grant set. The hardcoded `Scopes: []string{"annotation:edit"}`
// at per_user_token.go:201 is REPLACED, not extended.
```

Config keys: `auth.corpus_grant_enforcement` (SST, **no default**) →
`SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` (generated env, `${VAR:?...}` form).

Route-group label set is closed at **sixteen** values (`spec.md` §4.2, §18 decision 5):

- **Tier A (raw corpus retrieval, 1–8):** `search`, `digest`, `recent`, `artifact_detail`,
  `artifact_domain`, `export`, `knowledge`, `context_for`
- **Tier B (corpus-derived Phase-5 intelligence, 9–16):** `expertise`, `learning_paths`,
  `subscriptions`, `serendipity`, `content_fuel`, `quick_references`, `monthly_report`,
  `seasonal_patterns`

### Validation Checkpoints

| After | Gate that catches breakage before the next scope starts |
|---|---|
| Scope 01 | An operator-minted token carrying `corpus:read` round-trips through issuance and validation. If this fails, every later grant assertion is meaningless. |
| Scope 02 | Absent/malformed config **aborts startup** (no silent stage). OBSERVE returns **200** on all sixteen route groups while `..._would_deny_total` increments — proving telemetry works *before* anyone can be denied. |
| Scope 03 | T8 route-manifest set-equality test fails against current `main` (empty-scope principal is allowed today) and passes only once the gate is mounted, across **both tiers**. The Tier-B guard proves the `deps.IntelligenceEngine != nil` conditional cannot make set-equality pass vacuously. Denial parity proves no existence oracle. |
| Scope 04 | Every row of the design.md §5 compatibility matrix is exercised — the "unknown" Telegram row becomes a measured row before the flag can be flipped — **and** the adversarial negative case proves a principal *without* `corpus:read` gains no corpus access through Telegram. |
| Scope 05 | Flag-bundle parity check: `corpusGrantEnforcement` is default-ON in exactly one train and default-OFF in every other; SST key has no default; the retirement contract (§18 decision 6 — flag **and** observe branch deleted together) is recorded. |

### Planning Note — Flag Default Divergence (routed, not resolved here)

`design.md` §4/§9 records `corpusGrantEnforcement: false` in **both**
`config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml` (R-108-FL3).
The repo's mechanically-enforced release-train policy
(`.github/instructions/bubbles-release-trains.instructions.md`, `release-train-guard.sh`)
requires a flag to be default-ON in **exactly one** owning train and default-OFF in every
other. Scope 05 is planned to the enforced policy: **`next` = ON (owning train), `mvp` = OFF**.
This divergence from `design.md` is recorded here rather than silently applied; reconciling
`design.md` §4/§9 is owned by `bubbles.design`, not by this planning packet. Scope 05 DoD
item **DoD-05-06** blocks on that reconciliation.

### Planning Note — Operator Ratification Of `spec.md` Items 7-10 (recorded 2026-07-29)

Items 7-10 of `spec.md` §"Operator Ratification Additions" are now **ratified**. The rulings,
their rationale, and their exact attribution are recorded in `uservalidation.md`
§"Operator ratification — `spec.md` items 7-10". Attribution for all four:
*Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts.
Recorded by bubbles.plan on behalf of the operator.*

| Item | Ruling | Where it binds in this plan |
|---|---|---|
| 7 — coverage bar | All eight route groups must show real traffic **or** carry an explicit `idle-by-design` operator attestation (reason + named principal). `OBSERVE-CLEAN` may not be asserted while any group is silently unobserved. | Scope 04 go/no-go DoD; Scope 05 runbook DoD (`SCN-108-R03` "go/no-go criterion is stated") |
| 8 — admin surface | **Grant-issuance notice only. No grant editor in this spec.** | No scope adds an editor — this ratification **confirms** the existing plan rather than changing it. The notice itself is currently unowned; see the routed gap below. |
| 9 — pre-existing tokens | **Rotate proactively before the ENFORCE flip**, rather than letting grants surface as `unknown`. | Scope 04 implementation plan + DoD (caller remediation already precedes the Scope 05 flag flip) |
| 10 — Register 3 copy | Approved exactly as written; further change is a spec change. | Frozen product language. No plan change; see the routed gap below. |

**What this note deliberately does NOT do.** It does not add metrics, scopes, or Test Plan rows.
Two ratified obligations require work that **no scope in this plan currently owns**, and they are
routed rather than silently absorbed — the same discipline already applied to the flag-default
divergence above:

1. **The coverage denominator.** Ruling 7's option (a) is only assertable once the per-route-group
   **request** counter demanded by `F-108-UX-COVERAGE-01` exists. Scope 02 is planned for exactly
   **three** metrics per `design.md` §4. Adding a fourth from a ratification pass would be planning
   ahead of design. Routed to `bubbles.design`. Until it lands, ruling 7 is satisfiable only via
   option (b), explicit attestation, for every group without an existing traffic signal.
2. **The S7 grant-issuance notice and the Register 3 human copy.** No scope covers the admin-UI
   notice, nor the `COPY-DENY-*` rendering on the PWA, extension, or Telegram surfaces. Scope 03
   covers the **wire** envelope (Register 1) and Scope 04 covers the Telegram **routing** remedy;
   neither covers human-facing copy. Routed to `bubbles.design` → `bubbles.plan` for a plan
   amendment under a delivery-capable mode.

Both are recorded in `state.json.certification.outstandingFindings`.

---

### Planning Note — Operator Ratification Of `spec.md` §18 Items 1-6 (recorded 2026-07-29)

`spec.md` §18 is now a **RATIFIED decision record**; the review gate is CLOSED. All six items
were delegated under the standing instruction *"pick the best option for long term, no
shortcuts."* The decisions, their rationale, and their permanence are recorded in `spec.md` §18;
this table records only **where each one binds in this plan**.

**Two of the six ENLARGE this packet** — decision 3 and decision 5 are both the larger of the
options offered. That growth is reflected below in real scope descriptions, Test Plan rows, and
DoD items. It is **not** absorbed into the existing counts.

| Item | Ruling | Where it binds in this plan |
|---|---|---|
| 1 — observation window + "clean" bar | **Coverage-based, not time-only.** ≥14 consecutive OBSERVE days **AND** per-principal × per-route-group coverage across **all sixteen** groups **AND** zero would-deny for principals the operator intends to keep **AND** window resets on new principal enrollment or new client surface. | Scope 04 go/no-go DoD (strengthens the already-ratified item-7 bar from per-group to per-principal × per-group); Scope 05 runbook DoD (`SCN-108-R03`). **Blocked on F-108-COVERAGE-LABEL-01** — see the routed gap below. |
| 2 — which principals need `corpus:read` | **Grant by role; never widen the daily default.** `operatorGrants` already carries it; `dailyUserGrants` stays `[assistant:turn, knowledge-graph:read]` permanently; any daily principal that needs it gets an explicit per-principal `extraGrants` via `auth.SessionWithRole(...)`. | **Confirms** the existing plan rather than changing it — Scope 01 and Scope 04 already forbid widening `dailyUserGrants`, and both Change Boundaries already exclude the grant sets. Decision 2 makes that prohibition permanent rather than scope-local. |
| 3 — F-108-TELEGRAM-01 direction | **Derive Telegram scopes from the mapped principal's persisted grants.** The hardcoded-list extension is REJECTED. | **ENLARGES Scope 04.** `SCN-108-E01` restated, `SCN-108-E04` added as the adversarial negative case, `TP-04-08`/`TP-04-09` added with matching DoD items. Implementation Plan and Change Boundary rewritten off the two-option remedy. |
| 4 — GuestHost connector credential | **NO — it does not get `corpus:read`.** It is an inbound writer, not a corpus reader; its context reads move to the spec-109 MCP `hospitality-read` path under its own audience-bound credential. | Scope 04 Implementation Plan + DoD record the ruling and the accepted consequence: Tier A group 7 (`/api/context-for`) is gated with **no granted external reader** until BUG-019-003 clears. Cross-product coordination is routed to `bubbles.design` on spec 109; this packet does not edit spec 109. |
| 5 — F-108-ADJ-01 scope call | **Gate the Phase-5 intelligence endpoints IN THIS SPEC.** Gated surface goes 8 → **16** route groups. | **ENLARGES Scope 03.** Tier B added to the Implementation Plan, Consumer Impact Sweep, Shared Infrastructure Impact Sweep, and Change Boundary; `SCN-108-G04`/`SCN-108-G05` added; `TP-03-11`/`TP-03-12` added with matching DoD items. Also widens Scope 02's `route_group` label set to sixteen and Scope 05's documentation surface. |
| 6 — flag name and retirement | **Keep `corpusGrantEnforcement`.** Flag dies with its train + 1 cycle; at retirement **both** the flag **and** the observe branch are deleted and enforcement becomes unconditional. `bubbles.train` owns the flip and the retirement. | Scope 05: `SCN-108-R05`, `TP-05-07`, and a matching DoD item make the retirement contract a recorded, testable documentation obligation rather than an implied one. |

**What this note deliberately does NOT do.** It does not invent metrics, and it does not edit
`design.md`. Three obligations arising from the ratification require work **no scope in this
plan currently owns**, and they are routed rather than silently absorbed — the same discipline
already applied to the flag-default divergence above:

1. **The per-principal coverage signal (`F-108-COVERAGE-LABEL-01`, `spec.md` §16).** Decision
   1(b) ratifies coverage as a **per-principal × per-route-group** matrix. The metrics planned
   in `design.md` §4 cannot express it: the would-deny counter carries `user_id` but only fires
   on *denials*, so a **granted** principal's traffic is invisible; and the allowed counter has
   **no `user_id`** at all. Adding a fourth metric (or a `user_id` label to the allowed counter)
   from a ratification pass would be planning ahead of design. Routed to `bubbles.design`. Until
   it lands, decision 1(b) is satisfiable **only** by explicit per-cell operator attestation.
2. **`design.md` §2 route-inventory reconciliation.** `design.md` §2 still tables eight gated
   groups and its §8 T2/T4/T8 rows still say "eight". `spec.md` §4.2 now carries the canonical
   sixteen-group inventory in two tiers, and this plan is written to it. The stale design-side
   count is corrected only where it asserted the *opposite direction* (the Phase-5 "NOT gated"
   row, the Telegram two-option row, the Open Questions note); extending the §2 table itself
   belongs to `bubbles.design`, which owns that file. Blocking DoD item **DoD-03-TIERB-DESIGN**
   in Scope 03.
3. **Server-side grant readability (`F-108-UX-ROSTER-01`).** Decision 3's derivation presumes
   the mapped principal's grants are **readable server-side**; `auth_tokens` has no scopes
   column today. Decision 3 therefore **depends on** that finding, and does not close it.
   Recorded as a blocking dependency in Scope 04.

These three are recorded in `spec.md` §16 and in the affected scope DoD. They are **not** added
to `state.json.certification.outstandingFindings`, because that array is certification-owned and
this ratification pass is explicitly additive-only to `state.json`.

---

## Scope Table

| # | Scope | Surfaces | Depends On | Tests | Status |
|---|---|---|---|---|---|
| 01 | Scope Registration Prerequisite | `internal/auth` | — | 4 (2 unit, 1 integration, 1 e2e-api) | Not Started |
| 02 | Observe-Stage Plumbing | `cmd/core`, `internal/api`, `internal/metrics`, `config/` | 01 | 6 (3 unit, 2 integration, 1 e2e-api) | Not Started |
| 03 | Gate Mount (Tier A + Tier B, 16 route groups) | `internal/api` (router + contract test) | 02 | 12 (1 unit, 7 integration, 3 e2e-api, 1 stress) | Not Started |
| 04 | Caller Remediation (incl. Telegram grant derivation) | Telegram bridge, PWA, extension, shared-token/bootstrap | 03 | 10 (3 unit, 5 integration, 2 e2e-api) | Not Started |
| 05 | Docs, Release Train, Flag Bundles | `docs/`, `docs/releases/`, `config/` | 04 | 7 (4 unit, 1 integration, 2 e2e-api) | Not Started |

Canonical commands: `./smackerel.sh test unit` · `./smackerel.sh test integration` · `./smackerel.sh test e2e` · `./smackerel.sh test stress`

Every scope carries a persistent scenario-specific **Regression E2E** row plus the two
regression DoD items, so each behavior this feature introduces stays protected after the
scope closes. Scopes 03 and 04 additionally carry a Consumer Impact Sweep, a Shared
Infrastructure Impact Sweep, a Change Boundary, and explicit canary coverage.

---

## Scope 01: Scope Registration Prerequisite

**Status:** Not Started
**Depends On:** — (root scope; blocks 02, 03, 04, 05)
**Resolves:** F-108-SURFACE-01 / R-108-PRE1
**Surfaces:** `internal/auth`

### Use Cases (Gherkin)

**SCN-108-P01 — `corpus` is a registered scope surface**

```gherkin
Given the operator inspects the registered scope surfaces
When auth.RegisteredScopeSurfaces is enumerated
Then it contains the surface "corpus"
And the surface maps to the existing grant constant auth.GrantGlobalCorpusRead
```

**SCN-108-P02 — An operator can mint a token carrying `corpus:read`**

```gherkin
Given the surface "corpus" is registered
When the operator issues a principal token whose scope claim includes "corpus:read"
Then token issuance succeeds without an unknown-surface error
And auth.AuthorizeGrant for that session with required "corpus:read" returns authorized
```

### Implementation Plan

- Add `"corpus"` to `auth.RegisteredScopeSurfaces` in `internal/auth`, adjacent to the
  existing surfaces that back `annotation:edit` and `knowledge-graph:read`.
- Do **not** widen `dailyUserGrants`. Design.md "Resolved Decisions" is explicit: ungranted
  daily users are *supposed* to be denied; widening the grant set would defeat the feature.
- Do **not** change `operatorGrants` — it already includes `corpus:read`.
- No route, middleware, metric, or config change in this scope. This scope exists solely so
  the grant is *grantable* before anything can be gated on it.
- Confirm the existing `auth_surface_contract_test.go` surface list is updated in the same
  change so the contract test does not go stale.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-01-01 | unit | `internal/auth/browser_session_policy_test.go` | `RegisteredScopeSurfaces` contains `corpus`; the surface resolves to `GrantGlobalCorpusRead` (SCN-108-P01) | `./smackerel.sh test unit` |
| TP-01-02 | unit | `internal/auth` scope-claim validation test | A scope claim containing `corpus:read` validates without an unknown-surface error; `AuthorizeGrant` returns authorized (SCN-108-P02) | `./smackerel.sh test unit` |
| TP-01-03 | integration | `internal/api` against the ephemeral test stack | A token minted with `corpus:read` round-trips through issuance → bearer auth → session, and the session carries the grant (SCN-108-P02) | `./smackerel.sh test integration` |
| TP-01-04 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-P01 and SCN-108-P02 against the live stack: the `corpus` surface is still registered and a `corpus:read` token still mints and authorizes end-to-end. Fails if the `corpus` surface entry ever stops being registered or stops mapping to `GrantGlobalCorpusRead`; also proves the broader e2e suite shows no green→red drift from this scope | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] `auth.RegisteredScopeSurfaces` includes `corpus`; `dailyUserGrants` and `operatorGrants` are unchanged
- [ ] `TP-01-01` unit test passes — `corpus` surface registered and mapped to `GrantGlobalCorpusRead`
- [ ] `TP-01-02` unit test passes — `corpus:read` scope claim validates and authorizes
- [ ] `TP-01-03` integration test passes — minted `corpus:read` token round-trips to a granted session
- [ ] `internal/api/auth_surface_contract_test.go` surface list updated in the same change (no stale contract)
- [ ] `TP-01-04` regression e2e-api test passes — `corpus` surface registration and `corpus:read` minting are permanently protected against silent removal
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-01-04`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

---

## Scope 02: Observe-Stage Plumbing

**Status:** Not Started
**Depends On:** Scope 01
**Surfaces:** `cmd/core`, `internal/api`, `internal/metrics`, `config/smackerel.yaml`

### Use Cases (Gherkin)

**SCN-108-C03 — Absent enforcement config aborts startup**

```gherkin
Given SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT is absent or empty
When the core process starts
Then startup aborts and the error names SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT
And no stage is silently selected
And no HTTP listener is bound
```

**SCN-108-C05 — Malformed enforcement config aborts startup**

```gherkin
Given SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT is set to a value that is not an accepted boolean
When the core process starts
Then startup aborts and the error names the offending value
And neither OBSERVE nor ENFORCE is selected
```

**SCN-108-O01 — An ungranted request is counted, not denied, in OBSERVE**

```gherkin
Given the enforcement stage is OBSERVE
And a principal whose scope claim does not include "corpus:read"
When that principal requests a corpus route group
Then the response status is 200 and content is returned
And smackerel_auth_corpus_grant_would_deny_total increments for that route_group, user_id, and session_source
And the route_group value is one of the sixteen closed-set values
And a warn log is emitted with event=corpus_grant_would_deny and enforcement_mode=observe
And the log carries no query text and no artifact id
```

**SCN-108-O02 — A granted request is counted as allowed**

```gherkin
Given the enforcement stage is OBSERVE
And a principal whose scope claim includes "corpus:read"
When that principal requests a corpus route group
Then the response status is 200
And smackerel_auth_corpus_grant_allowed_total increments for that route_group and session_source
And smackerel_auth_corpus_grant_would_deny_total does not increment
And smackerel_auth_corpus_grant_enforcement_mode reports 0
```

### Implementation Plan

- **Config (SST, fail-loud).** Declare `auth.corpus_grant_enforcement` in
  `config/smackerel.yaml` with **no default value** (R-108-CFG3, R-108-FL5). `./smackerel.sh
  config generate` emits `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` into
  `config/generated/<env>.env`. Any shell interpolation uses the `${VAR:?...}` form.
  `${VAR:-...}`, `os.Getenv` with a default, and `unwrap_or`-shaped resolution are forbidden
  by `.github/instructions/smackerel-no-defaults.instructions.md`.
- **Resolution.** One resolution point in `cmd/core` wiring, executed once at startup, before
  any listener binds. Absent/empty → abort naming the variable. Non-boolean → abort naming the
  value. No third mode, no per-route override (R-108-FL6).
- **Observe middleware.** New `CorpusGrantGate` in `internal/api` with an `Observe` method
  that calls the existing `auth.GateGlobalCorpusRead(sess)` — finally giving that function a
  production caller — and **never denies**. Mounted in **both** stages; otherwise OBSERVE
  emits nothing.
- **Metrics.** Extend the `smackerel_auth_*` family in `internal/metrics/auth.go` — do not
  fork it and do not reuse `smackerel_auth_scope_rejected_total` for the observe signal
  (R-108-O2). Three additions per design.md §4. `route_group` is a closed **sixteen**-value set
  (`spec.md` §4.2 Tier A + Tier B, per §18 decision 5); raw paths are never a label value
  (R-108-O3/O4). Cardinality stays bounded: sixteen is still a closed set, and `user_id`
  follows the existing precedent at `internal/metrics/auth.go:162`.
- **Coverage signal is NOT planned here (routed).** §18 decision 1(b) ratifies a
  per-principal × per-route-group coverage bar, which these three metrics **cannot express**
  (the would-deny counter only fires on denials; the allowed counter carries no `user_id`).
  That gap is `F-108-COVERAGE-LABEL-01`, routed to `bubbles.design`. This scope does **not**
  invent a fourth metric from a ratification pass.
- **Structured log.** One `warn` per would-be denial: `event=corpus_grant_would_deny`,
  `route_group`, `user_id`, `session_source`, `required_grant=corpus:read`,
  `enforcement_mode=observe`. Answers *who*, never *what they searched for*.
- The observe middleware is wired into the router group in this scope, but
  `auth.RequireScope` is **not** yet mounted — no request can be denied by this scope.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-02-01 | unit | `cmd/core` config resolution test | Absent/empty `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` aborts startup naming the variable; no stage selected (SCN-108-C03, design T3) | `./smackerel.sh test unit` |
| TP-02-02 | unit | `cmd/core` config resolution test | A malformed value aborts startup naming the offending value; no silent fallback to OBSERVE (SCN-108-C05, design T3) | `./smackerel.sh test unit` |
| TP-02-03 | unit | `internal/metrics/auth_test.go` | The three new metrics register in the `smackerel_auth_*` family with the closed **16**-value `route_group` label set (Tier A + Tier B, `spec.md` §4.2); an unknown label value is rejected (design T2, §18 decision 5) | `./smackerel.sh test unit` |
| TP-02-04 | integration | `internal/api` against the ephemeral test stack | OBSERVE: an ungranted principal receives **200** on all sixteen route groups AND `..._corpus_grant_would_deny_total` increments with the correct `route_group`; the warn log carries no query text or artifact id (SCN-108-O01, design T4 observe half) | `./smackerel.sh test integration` |
| TP-02-05 | integration | same | A granted principal receives 200, increments `..._corpus_grant_allowed_total`, does **not** increment the would-deny counter, and `..._enforcement_mode` reports `0` (SCN-108-O02) | `./smackerel.sh test integration` |
| TP-02-06 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-C03, SCN-108-C05, SCN-108-O01 and SCN-108-O02 against the live stack: absent/malformed config still aborts startup, and in OBSERVE an ungranted principal still receives 200 while `..._would_deny_total` still increments. Fails if a silent default is reintroduced or the observe counter is unwired; also proves the broader e2e suite shows no green→red drift | `./smackerel.sh test e2e` |

### Definition of Done

- [ ] `auth.corpus_grant_enforcement` declared in `config/smackerel.yaml` with no default; generated env emits `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT`; no `${VAR:-...}` / `os.Getenv`-with-default shape anywhere in the resolution path
- [ ] Three `smackerel_auth_corpus_grant_*` metrics added to the existing `smackerel_auth_*` family; `smackerel_auth_scope_rejected_total` unchanged and not reused for the observe signal
- [ ] `TP-02-01` unit test passes — absent config aborts startup naming the variable
- [ ] `TP-02-02` unit test passes — malformed config aborts startup naming the value
- [ ] `TP-02-03` unit test passes — metrics register with the closed **16**-value `route_group` label set (Tier A + Tier B)
- [ ] `TP-02-04` integration test passes — OBSERVE returns 200 on all sixteen groups and counts would-be denials
- [ ] `TP-02-05` integration test passes — granted requests count as allowed, would-deny stays flat, mode gauge reports 0
- [ ] Observe middleware is mounted in both stages; no request path can return 403 from this scope
- [ ] `TP-02-06` regression e2e-api test passes — fail-loud config resolution and OBSERVE-stage counting are permanently protected
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-02-06`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Live-category tests emit telemetry tagged `env=test*` only; no write to prod monitoring (R-108-O6, G115)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

---

## Scope 03: Gate Mount

**Status:** Not Started
**Depends On:** Scope 02
**Resolves:** F-108-ADJ-01 (scope call ratified IN SCOPE by `spec.md` §18 decision 5)
**Surfaces:** `internal/api/router.go`, `internal/api/auth_surface_contract_test.go`

> **Scope increase recorded, not absorbed (§18 decision 5, 2026-07-29).** This scope was planned
> against **eight** route groups. Ratification brings the eight corpus-*derived* Phase-5
> intelligence endpoints in scope as **Tier B**, taking the gated surface to **sixteen**. Test
> Plan rows go 10 → 12, scenarios go 5 → 7, and the Implementation Plan, both Impact Sweeps, and
> the Change Boundary below are widened accordingly.

### Use Cases (Gherkin)

**SCN-108-G01 — ENFORCE denies an ungranted principal on every corpus route group**

```gherkin
Given the enforcement stage is ENFORCE
And a principal whose scope claim does not include "corpus:read"
When that principal requests any of the sixteen corpus route groups
Then the response status is 403
And the response body carries no result count, artifact id, artifact title, or domain label
And smackerel_auth_scope_rejected_total increments
And smackerel_auth_corpus_grant_enforcement_mode reports 1
```

**SCN-108-G04 — Tier B corpus-derived intelligence endpoints are gated identically**

```gherkin
Given the enforcement stage is ENFORCE
And the intelligence engine is wired so the Phase-5 endpoints are registered
And a principal whose scope claim does not include "corpus:read"
When that principal requests each of the eight Tier B route groups
Then every response status is 403
And no derived corpus signal is returned by any of them
And the denial body is the same shape as a Tier A denial
And a principal holding "corpus:read" receives 200 from the same eight endpoints
```

**SCN-108-G05 — The Tier B conditional registration cannot make set-equality pass vacuously (adversarial)**

```gherkin
Given the Phase-5 endpoints are registered only when deps.IntelligenceEngine is non-nil
When the route-manifest contract test builds the real router with a NON-NIL intelligence engine
Then the router's mounted corpus group contains all sixteen route groups
And the set equality assertion is evaluated against sixteen, not eight
And the test FAILS if the Tier B routes are registered outside the gated group
And the test FAILS if a nil intelligence engine is substituted to make the assertion trivially satisfiable
```

**SCN-108-D01 — A denial is not an existence oracle**

```gherkin
Given the enforcement stage is ENFORCE
And a principal whose scope claim does not include "corpus:read"
When that principal requests /api/artifact/{id} for an id that exists
And that principal requests /api/artifact/{id} for an id that does not exist
Then both responses are 403
And both responses are byte-identical
And neither response carries a WWW-Authenticate challenge or a retry hint
```

**SCN-108-G02 — Documented bypass sources still pass under ENFORCE**

```gherkin
Given the enforcement stage is ENFORCE
When a shared-token session requests a corpus route group
And a bootstrap session requests a corpus route group
Then both receive a non-403 response per the existing RequireScope source switch
And the bypass is asserted by test rather than assumed
```

**SCN-108-G03 — The gate cannot be silently removed or bypassed (adversarial, design T8)**

```gherkin
Given the real router is constructed through the same constructor production uses, with ENFORCE selected
And a fixture principal whose scope claim is empty
When each of the sixteen canonical corpus route groups is requested
Then every response is 403
And the set of corpus routes mounted under the gated group equals the canonical sixteen-value list exactly
And the test fails if the RequireScope mount is deleted, a corpus route is moved out of the gated group,
    the stage machine falls back to OBSERVE when config is absent, or a seventeenth corpus route is registered ungated
```

**SCN-108-C04 — Rollback to OBSERVE stops denials without a rebuild**

```gherkin
Given the enforcement stage is ENFORCE and ungranted principals are being denied
When the operator sets the train flag back to false, regenerates the config bundle,
     and re-applies the same signed image digest
Then the restarted process resolves the stage as OBSERVE
And previously denied principals receive 200 again
And would-be-denial counting resumes
And no docker build is invoked at any point
```

### Implementation Plan

- Wrap the **sixteen** corpus route registrations from `spec.md` §4.2 (Tier A 1–8, Tier B 9–16)
  in a single `r.Group(...)` **inside** the existing authenticated group opened by
  `r.Group(func(r chi.Router) { r.Use(deps.bearerAuthMiddleware) ... })` in
  `internal/api/router.go` (~L87–L109). Middleware order inside the new group:
  1. `r.Use(deps.CorpusGrantGate.Observe)` — mounted in both stages, never denies.
  2. `r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))` — mounted **only** in ENFORCE.
  The outer `bearerAuthMiddleware` must run first; it populates the session `AuthorizeGrant` reads.
- The gate attaches to the enclosing group, so the six `/api/knowledge/*` endpoints registered
  via `r.Route("/knowledge", ...)` inherit it as one unit.
- **Tier B (§18 decision 5).** The eight Phase-5 intelligence endpoints at
  `internal/api/router.go:238-250` move under the same gated group. They are corpus-*derived*
  reads over the same global store, so they carry the same grant and the same denial shape;
  the tier split is documentation, not a difference in authority.
- **Tier B conditional-registration hazard — handle explicitly, do not assume.** The Phase-5
  block is registered behind `if deps.IntelligenceEngine != nil` (`router.go:239`). Two
  consequences the implementation MUST honor: (a) the gated group must enclose the conditional
  so a non-nil engine cannot register the eight routes *outside* the gate; and (b) the T8
  set-equality assertion must be evaluated against a router built with a **non-nil** engine, or
  it compares sixteen expected groups against a router that only ever registered eight and
  passes vacuously. `SCN-108-G05` / `TP-03-12` exist for exactly this.
- **No per-handler checks.** Do not add `if !authorized { ... }` inside `SearchHandler`,
  `ExportHandler`, the knowledge handlers, or any `internal/api/intelligence.go` handler — that
  shape is invisible to the route manifest, un-auditable, and lets a new corpus handler ship
  ungated.
- Leave the deliberately-ungated routes from design.md §2 alone: `/api/capture`,
  `/api/assistant/turn`, the `annotation:edit` group, the `knowledge-graph:read` group,
  `/api/bookmarks/import`, `/api/internal/telegram-message-artifact`, `/api/health`, `/metrics`,
  `/readyz`. **Note:** `/api/expertise` and the other Phase-5 endpoints were previously on that
  ungated list; §18 decision 5 moves them to Tier B. Reconciling `design.md` §2's table is
  routed to `bubbles.design` (DoD-03-TIERB-DESIGN) and is **not** silently edited from here.
- Denial semantics per design.md §3: bare 403, existing `RequireScope` envelope carrying the
  scope name only, identical shape for every route group, no 404-vs-403 discrimination,
  wildcard `*` not honored, no `WWW-Authenticate` and no retry hint.
- **T8 lands here.** Extend `internal/api/auth_surface_contract_test.go` — today the only
  referent of `GateGlobalCorpusRead` — with the route-manifest contract test described in
  design.md §8. Its fixture principal carries an **empty** scope claim, which is exactly the
  input today's ungated router allows, so the test **fails against current `main`** and passes
  only once the gate is mounted. That is what makes it adversarial rather than tautological.

### Consumer Impact Sweep

This scope **moves sixteen existing route registrations** out of the flat authenticated group and
under a new gated group, and **replaces** the effective access contract of those endpoints
(previously: any authenticated principal; after: `corpus:read` holders only). No URL string
changes, but the *contract* behind each path changes, so every first-party consumer of those
paths must be traced before the flag can be flipped. The sweep is a read-only enumeration in
this scope; the actual caller remediation is Scope 04.

**Affected consumer surfaces (enumerated, not sampled):**

| Consumer surface | How it reaches the corpus paths | What must be checked |
|---|---|---|
| **PWA** (`internal/web`) | Same-origin browser session → `/api/search`, `/api/digest`, `/api/recent`, `/api/artifact/{id}`, `/api/export`, `/api/knowledge/*`, `/api/context-for` | Every fetch call site, plus **navigation** entries and in-app **deep link** targets that land on a corpus view; a 403 must render an operator-actionable state, not a blank panel or a silent empty list |
| **Chrome extension bridge** (`extensions/chrome-bridge`) | Bearer token inherited from the principal → capture/lookup calls that read corpus paths | The extension's **API client** call sites; it has no grant of its own and must track its principal exactly |
| **Telegram bridge** | Service token → corpus commands proxied to the same paths | The bridge token's scope claim (F-108-TELEGRAM-01); measured in OBSERVE, remediated in Scope 04 by **grant derivation** (§18 decision 3) |
| **Tier B Phase-5 consumers** (§18 decision 5) | Direct bearer calls to `/api/expertise`, `/learning-paths`, `/subscriptions`, `/serendipity`, `/content-fuel`, `/quick-references`, `/monthly-report`, `/seasonal-patterns` | **Zero first-party in-repo callers** — verified by grep over `web/pwa/`, `extensions/`, `internal/telegram/`. That is a *recorded negative result*, and it cuts both ways: the caller-break radius is near zero, **but** the observation window will be silent for all eight, which is exactly the falsely-clean signal decision 1 forbids reading as safety. These eight must be closed by per-cell attestation in Scope 04, never by a zero counter |
| **External / MCP API clients** | Direct bearer calls to any of the sixteen groups | Documented in `docs/API.md` as requiring `corpus:read`; there is no **generated client** to regenerate, which is itself an assertion to verify rather than assume |
| **Docs and runbooks** | `docs/API.md`, `docs/Operations.md`, `docs/smackerel.md` §17.2 | **Stale-reference** scan: no doc may still describe any of the sixteen endpoints as reachable by any authenticated principal |

There is **no redirect** and no breadcrumb rewrite in this scope — paths are unchanged — and that
negative result is recorded here explicitly so a later reader does not re-open the question.

### Shared Infrastructure Impact Sweep

The gate is mounted on the **shared router bootstrap** in `internal/api/router.go` and is asserted
by the **shared test harness** in `internal/api/auth_surface_contract_test.go`. Both are
high-fan-out surfaces: the router constructor is the common bootstrap every API test builds its
fixture stack from, and the contract test file is the shared harness that today is the only
referent of `GateGlobalCorpusRead`. A defect here does not fail one test; it fails every API test
at once and hides the real signal.

**Blast radius and downstream contract surfaces:**

- **Middleware ordering contract** — `bearerAuthMiddleware` must run *before* the gate, because the
  gate reads the session it populates. Inverting the ordering yields a uniform 403 for every
  principal, including granted ones.
- **Session contract** — every existing API test fixture that builds a session now flows through an
  additional middleware; fixtures that construct a session with an empty scope claim (previously
  harmless) will begin receiving 403.
- **Role/grant context contract** — the shared-token and bootstrap source switch inside
  `RequireScope` is a downstream contract that must keep behaving identically (`TP-03-03`).
- **Stage/timing contract** — the gate is mounted only in ENFORCE, so the router constructor now has
  two shapes; any test that assumes one shape becomes stage-dependent.
- **Conditional-registration contract (Tier B)** — the router constructor now has a *third* axis:
  `deps.IntelligenceEngine` nil vs. non-nil. Any fixture that builds the router with a nil engine
  silently drops eight of the sixteen gated routes. That is a shared-harness trap, not a Tier-B
  detail: a set-equality assertion run against such a fixture passes while production ships the
  eight routes ungated (`TP-03-12`).
- **Storage contract** — unchanged. No fixture storage, migration, or seed path is touched by this
  scope, and that negative result is asserted rather than assumed.

**Canary before broad rerun:** `TP-03-08` runs the narrow router-bootstrap canary first. If the
ordering or session contract is broken, the canary fails in seconds and the full suite is not run
against a known-bad harness.

### Change Boundary

This scope is a **contract repair** on shared routing infrastructure, so its blast radius is
contained by an explicit boundary. Collateral cleanup is opt-in and out of scope here.

**Allowed file families:**

- `internal/api/router.go` — the new gated `r.Group(...)`, its two `r.Use(...)` lines, and the
  relocation of the Tier A and Tier B route registrations into it
- `internal/api/auth_surface_contract_test.go` — the T8 route-manifest contract test
- New/extended test files under `internal/api` and `internal/auth` named in the Test Plan

**Excluded surfaces (must remain byte-unchanged by this scope):**

- Every corpus handler body — `SearchHandler`, `ExportHandler`, the knowledge handlers, and every
  Tier B handler in `internal/api/intelligence.go`: no per-handler `if !authorized` check may be
  added, and no handler logic may change. This scope **moves route registrations**, it does not
  touch handler bodies
- `internal/auth/*.go` grant definitions — `dailyUserGrants`, `operatorGrants`,
  `RegisteredScopeSurfaces` (owned by Scope 01; `dailyUserGrants` is permanently frozen by
  §18 decision 2)
- The deliberately-ungated routes from design.md §2, **minus** the Phase-5 block that §18
  decision 5 moved to Tier B
- `design.md` itself — the §2 route-inventory reconciliation is routed to `bubbles.design`
  (DoD-03-TIERB-DESIGN), never silently overwritten from this packet
- `internal/metrics`, `cmd/core`, `config/`, `docs/` — owned by Scopes 02 and 05
- The Telegram bridge, PWA, and extension caller code — owned by Scope 04

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-03-01 | unit | `internal/auth/browser_session_policy_test.go` | `GateGlobalCorpusRead` denies an empty scope claim, denies a `*` wildcard claim, allows an explicit `corpus:read` claim, and returns a `CorpusDecision` carrying no content/count/label (design T1) | `./smackerel.sh test unit` |
| TP-03-02 | integration | `internal/api` against the ephemeral test stack | ENFORCE: an ungranted principal receives **403** on all sixteen route groups; `smackerel_auth_scope_rejected_total` increments; mode gauge reports `1` (SCN-108-G01, design T4 enforce half) | `./smackerel.sh test integration` |
| TP-03-03 | integration | same | Shared-token and bootstrap sessions pass under ENFORCE via the documented `RequireScope` source switch — the bypass is asserted, not assumed (SCN-108-G02, design T5) | `./smackerel.sh test integration` |
| TP-03-04 | integration | `internal/api/auth_surface_contract_test.go` | **ADVERSARIAL (design T8):** real router + ENFORCE + empty-scope fixture principal → 403 on all **sixteen** groups, AND set-equality of the canonical sixteen against the router's mounted corpus group. Fails against current `main`; fails if the `RequireScope` mount is deleted, a route leaves the group, the stage defaults to OBSERVE, or a seventeenth corpus route is registered ungated (SCN-108-G03) | `./smackerel.sh test integration` |
| TP-03-05 | integration | `internal/api` + `cmd/core` restart harness | Stage flip is symmetric and idempotent: ENFORCE → OBSERVE restores 200 for previously denied principals and resumes counting, with no rebuild invoked (SCN-108-C04) | `./smackerel.sh test integration` |
| TP-03-06 | e2e-api | `./smackerel.sh test e2e` | Full stack, real Postgres: a granted operator token reads `/api/search` and `/api/export`; an ungranted daily-user token is refused on both; the refusal body contains no artifact id, title, or count (SCN-108-G01, design T6) | `./smackerel.sh test e2e` |
| TP-03-07 | e2e-api | same | Denial parity: a denied `/api/artifact/{id}` for a real id and for a random id produce byte-identical responses — no existence oracle (SCN-108-D01, design T7) | `./smackerel.sh test e2e` |
| TP-03-08 | integration | `internal/api` router-bootstrap canary | **Canary:** narrow, independently-runnable canary over the shared router bootstrap and shared contract-test harness — asserts `bearerAuthMiddleware` still runs before the gate (middleware **ordering** contract), that a granted session still resolves (**session** contract), and that the ungated routes from design.md §2 are still reachable. Run **before** any broad suite rerun so a broken shared harness is caught in seconds instead of masquerading as a repo-wide failure | `./smackerel.sh test integration` |
| TP-03-09 | stress | `internal/api` gated-route stress harness | The gate sits on the hot read path of all sixteen corpus route groups, so it is SLA-sensitive. Under sustained concurrent load on `/api/search` and `/api/artifact/{id}`: added per-request latency from `RequireScope` + `Observe` stays within the documented budget, p99 does not regress against the ungated baseline, and no allocation or lock-contention regression appears in the middleware chain | `./smackerel.sh test stress` |
| TP-03-10 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-G01, SCN-108-D01, SCN-108-G02, SCN-108-G03, SCN-108-G04, SCN-108-G05 and SCN-108-C04 against the live stack: ENFORCE still denies ungranted principals on all **sixteen** groups, denials stay byte-identical, documented bypasses still pass, Tier B stays gated, and ENFORCE→OBSERVE rollback still restores access. Fails if the gate is unmounted, a corpus route escapes the gated group, or the stage machine regains a silent default; also proves the broader e2e suite shows no green→red drift | `./smackerel.sh test e2e` |
| TP-03-11 | integration | `internal/api` against the ephemeral test stack | **Tier B (§18 decision 5):** with a non-nil intelligence engine and ENFORCE, an ungranted principal receives **403** on each of the eight Phase-5 route groups and no derived corpus signal is returned; a principal holding `corpus:read` receives 200 from the same eight; the Tier B denial body is the same shape as a Tier A denial (SCN-108-G04) | `./smackerel.sh test integration` |
| TP-03-12 | integration | `internal/api/auth_surface_contract_test.go` | **ADVERSARIAL (conditional-registration hazard):** the T8 set-equality assertion is evaluated against a router built with a **non-nil** `deps.IntelligenceEngine`, so all sixteen groups are actually registered. Fails if the Tier B block is registered outside the gated group, and fails if a nil engine is substituted to make the sixteen-value set-equality trivially satisfiable — the vacuous-pass path that would let eight corpus-derived routes ship ungated (SCN-108-G05) | `./smackerel.sh test integration` |

### Definition of Done

- [ ] `r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))` mounted on the corpus route group in `internal/api/router.go`, inside `bearerAuthMiddleware` and outside the individual route registrations; mounted only in ENFORCE
- [ ] All **sixteen** route groups from `spec.md` §4.2 (Tier A 1–8 + Tier B 9–16) sit inside the gated group; every still-ungated route from design.md §2 remains ungated and unchanged
- [ ] The Tier B block's `if deps.IntelligenceEngine != nil` conditional sits **inside** the gated group, so a non-nil engine cannot register the eight Phase-5 routes outside the gate
- [ ] No per-handler `if !authorized` check added to any corpus handler, Tier A or Tier B
- [ ] `TP-03-01` unit test passes — `GateGlobalCorpusRead` denies empty and wildcard claims, allows explicit, leaks nothing
- [ ] `TP-03-02` integration test passes — ENFORCE returns 403 on all sixteen groups
- [ ] `TP-03-03` integration test passes — shared-token and bootstrap bypass asserted under ENFORCE
- [ ] `TP-03-04` adversarial route-manifest test passes AND is demonstrated to **fail against current `main`** (empty-scope principal is allowed today); set-equality catches a seventeenth ungated corpus route
- [ ] `TP-03-05` integration test passes — ENFORCE→OBSERVE rollback restores access with no rebuild
- [ ] `TP-03-06` e2e-api test passes — granted reads succeed, ungranted refused, body carries no id/title/count
- [ ] `TP-03-07` e2e-api test passes — denial byte-parity between real and random id
- [ ] `TP-03-08` canary integration test passes — shared router-bootstrap ordering, session, and ungated-route contracts intact
- [ ] `TP-03-09` stress test passes — gate adds no p99 latency regression on the corpus route groups under sustained load
- [ ] `TP-03-10` regression e2e-api test passes — enforcement across both tiers, denial parity, documented bypasses, and rollback are permanently protected
- [ ] `TP-03-11` integration test passes — all eight Tier B Phase-5 route groups deny an ungranted principal with 403 and the Tier A denial shape, and allow a `corpus:read` holder (§18 decision 5)
- [ ] `TP-03-12` adversarial integration test passes — set-equality is evaluated against a non-nil intelligence engine and fails on the nil-engine vacuous-pass path and on Tier B registered outside the gate
- [ ] **DoD-03-TIERB-DESIGN:** `design.md` §2's route-inventory table and §8 T2/T4/T8 count language reconciled to the ratified sixteen-group surface by `bubbles.design` before this scope closes — not silently overwritten from this planning packet (routed per the `spec.md` §18 decision 5 planning note)
- [ ] Consumer Impact Sweep completed for the corpus route-group contract change across the PWA, Chrome extension bridge, Telegram bridge, Tier B consumers, external API clients, and docs: zero stale first-party references remain, and the Tier B "zero first-party in-repo callers" negative result is re-verified rather than inherited
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-03-10`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Live-category tests emit telemetry tagged `env=test*` only; no write to prod monitoring (R-108-O6, G115)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

---

## Scope 04: Caller Remediation

**Status:** Not Started
**Depends On:** Scope 03
**Resolves:** F-108-TELEGRAM-01 (stage-2 blocking prerequisite; direction ratified by `spec.md` §18 decision 3)
**Depends on (external, unresolved):** F-108-UX-ROSTER-01 — grants are not readable server-side today
**Surfaces:** Telegram bridge, PWA / web browser session, browser extension, shared-token and bootstrap callers

> **Scope increase recorded, not absorbed (§18 decision 3, 2026-07-29).** This scope was planned
> around a **two-option** Telegram remedy (grant the bridge token `corpus:read`, **or** re-route
> its corpus commands through `/api/assistant/turn`). Ratification **closes both** and selects a
> third, larger direction: **derive the minted scope claim from the mapped principal's persisted
> grant set**. Test Plan rows go 7 → 9, scenarios go 3 → 4, and the Implementation Plan, Shared
> Infrastructure Impact Sweep, and Change Boundary below are rewritten off the closed framing.

### Use Cases (Gherkin)

**SCN-108-E01 — Telegram bridge corpus command under enforcement**

```gherkin
Given the enforcement stage is ENFORCE
And a mapped Telegram chat whose principal holds "corpus:read" in its persisted grant set
When the user issues a search, digest, recent, or knowledge command through the bridge
Then the minted per-user token carries "corpus:read" because it was DERIVED from that principal's grants
And the command succeeds
And no minter-side hardcoded scope list determined the outcome
```

**SCN-108-E04 — Telegram authority comes from the principal, not the minter (adversarial)**

```gherkin
Given the enforcement stage is ENFORCE
And a mapped Telegram chat whose principal does NOT hold "corpus:read" in its persisted grant set
When the user issues a corpus command through the bridge
Then the minted per-user token does NOT carry "corpus:read"
And the corpus command is refused with 403
And the refusal is rendered as an operator-actionable permanent condition, never as a transient retry
And the test FAILS if the minter reintroduces a hardcoded scope list that grants corpus access
    to every mapped chat regardless of the principal's persisted grants
```

**SCN-108-E02 — Daily-user principal is remediated by token rotation, not a flag flip**

```gherkin
Given a PWA daily-user principal whose scope claim excludes "corpus:read"
And the enforcement stage is ENFORCE
When the principal requests a corpus route group
Then the response is 403
And when the operator rotates that principal's token with "corpus:read" added to the scope claim
Then the same request returns 200
And no feature flag was changed to achieve the grant
```

**SCN-108-E03 — The browser extension inherits its principal's grant**

```gherkin
Given the browser extension consumes the principal's bearer token
And the enforcement stage is ENFORCE
When the principal holds "corpus:read"
Then extension corpus requests succeed
And when the principal does not hold "corpus:read"
Then extension corpus requests receive the same 403 as the PWA
And no extension-specific grant exists or is introduced
```

### Implementation Plan

- **Telegram bridge (F-108-TELEGRAM-01, blocking) — RATIFIED direction: grant derivation.**
  `spec.md` §18 decision 3 selects option (b): the bridge's minted per-user PASETO derives its
  scope claim from the **mapped principal's persisted grant set**. The hardcoded
  `Scopes: []string{"annotation:edit"}` at `internal/telegram/per_user_token.go:201` is
  **REPLACED, not extended**. Option (a) (extend the hardcoded list) and the re-route-through-
  `/api/assistant/turn` alternative are both **CLOSED** — do not reopen either, and do not widen
  `dailyUserGrants` to paper over the change.
  - *Why the larger change:* the hardcoded list locates authority at the **minter** rather than
    the **principal**, contradicting spec 044 Scope 02 and the `browser_session_policy.go`
    persisted-grant doctrine. Extending it would entrench a second, divergent authority source
    that drifts and must eventually be unwound at higher cost.
  - *Correctness is a NEGATIVE case.* Deriving is only correct if a principal **without**
    `corpus:read` gains **no** corpus access through Telegram. A hardcoded-list implementation
    would pass a naive "Telegram works" test and fail this one — `SCN-108-E04` / `TP-04-09` exist
    for exactly that reason.
  - *Blocking dependency:* derivation presumes the mapped principal's grants are **readable
    server-side**. `F-108-UX-ROSTER-01` records that `auth_tokens` has no scopes column today.
    This scope **depends on** that finding; it does not close it. If it is unresolved when this
    scope is picked up, the scope is BLOCKED, not worked around with a minter-side list.
  - The OBSERVE-window readout
    (`sum by (user_id, route_group) (increase(smackerel_auth_corpus_grant_would_deny_total[7d]))`)
    still measures the bridge's real denial set — it now informs *which principals need a grant*,
    not *which remedy to pick*.
- **GuestHost connector (§18 decision 4) — NO grant.** The external GuestHost connector
  credential does **not** receive `corpus:read`. It is an inbound writer, not a corpus reader;
  its guest-context reads belong on the spec-109 MCP `hospitality-read` path under that path's
  own audience-bound credential (spec 109 **D3**). Accepted consequence, recorded rather than
  papered over: Tier A group 7 (`POST /api/context-for`) is gated with **no granted external
  reader** until `hospitality-read` ships, which is itself blocked on **BUG-019-003**. Cross-
  product coordination is owned by `bubbles.design` on spec 109; this scope does **not** edit
  spec 109 and does **not** invent an interim grant.
- **PWA / extension.** No code change to the grant model. The remedy is a **token rotation**
  per F-108-GRANT-MECHANISM-01: the operator re-issues the principal's token with
  `corpus:read` added to the scope claim, via the existing per-principal `extraGrants` seam
  `auth.SessionWithRole(...)` (§18 decision 2). This requires Scope 01. Record the procedure so
  Scope 05 can document it.
- **Shared token / bootstrap.** No change. The `RequireScope` source switch already lets these
  through. Scope 03 `TP-03-03` asserts it; this scope confirms the decision is recorded in the
  compatibility matrix rather than discovered later as an accident.
- **Prometheus / orchestrator probes.** No change — `/metrics`, `/readyz`, `/api/health` are
  ungated by design.
- Close every "unknown" row in the design.md §5 matrix with a measured row before Scope 05
  can flip the owning-train flag to ON.
- **Proactive rotation of pre-existing tokens (ratified item 9, `uservalidation.md`).** Any
  principal whose grants are unknowable — every token minted before grant persistence, which
  S7 must render as `── unknown ──` — is **rotated proactively before the ENFORCE flip**, not
  left to surface as `unknown` at flip time. The operator **issues a deliberate grant set** per
  principal rather than attempting to recover an unreadable one; that is what makes the pre-flip
  roster determinate by construction and what sidesteps the replace-not-add hazard
  (F-108-UX-ROTATE-ADD-01) for this migration. This does **not** close F-108-UX-ROSTER-01:
  grants remain unreadable server-side afterwards, so SCN-108-F02 stays at risk for any token
  that is not proactively rotated.
- **Coverage attestation for the go/no-go (ratified item 7, strengthened by §18 decision 1).** The
  OBSERVE-window readout is judged against the ratified bar, which is now the **conjunction** of
  four criteria: (a) ≥ **14 consecutive days** in OBSERVE with the stage resolved from SST at
  process start; (b) **per-principal × per-route-group coverage** across **all sixteen** groups —
  every enrolled `per_user_token` principal observed at least once in every group; (c) **zero**
  would-deny events attributable to a principal the operator intends to keep, with any
  intentionally-denied principal recorded **before** the flip; and (d) the window **RESETS to day
  zero** on any new principal enrollment or any new client surface.
  - Item 7's `idle-by-design` escape survives, but applies **per matrix cell** rather than per
    group: a cell is satisfied by observed traffic **or** by an explicit operator attestation
    naming a reason and the principal. Cells that can never be exercised (the GuestHost
    connector will never call `/api/search`) are closed by attestation, never by inference.
  - **All eight Tier B groups have zero first-party in-repo callers**, so they will be silent for
    the entire window. Their cells MUST be closed by attestation; a zero counter over a silent
    group is never read as clean.
  - Because `F-108-COVERAGE-LABEL-01` is unresolved (the planned metrics cannot express
    per-principal coverage), criterion (b) is presently satisfiable **only** by per-cell
    attestation. Attestation is a recorded operator decision, never an inferred one.
  - A silently unobserved cell blocks `OBSERVE-CLEAN`.

### Consumer Impact Sweep

§18 decision 3 makes this scope an **interface change**, not just a caller repair: the Telegram
per-user token minter stops sourcing its scope claim from a literal and starts **deriving** it
from the mapped principal's persisted grant set. `Scopes: []string{"annotation:edit"}`
(`internal/telegram/per_user_token.go:201`) is **removed**, and `PerUserTokenMinter`
(`per_user_token.go:55-62` — today only `bot`, `signingKey`, `keyID`, `issuer`, `ttl`, `now`)
plus `PerUserTokenMinterOptions` (`:65-93`) gain a **new inbound grant-read dependency** they do
not have today. Every construction site and every consumer of the minted claim must therefore be
traced before the flag can be flipped. The sweep below is a **read-only enumeration verified
against the code at plan time**; the remediation itself is this scope's implementation work.

**Affected consumer surfaces (enumerated and code-verified, not sampled):**

| Consumer surface | How it reaches the changed interface | What must be checked |
|---|---|---|
| **Minter itself** — `internal/telegram/per_user_token.go` | `MintForUser` (`:179`) builds the claim; `MintForChat` (`:160`) resolves chat → user then delegates to it | The literal at `:201` is REPLACED, not extended; an unresolvable principal **aborts the mint** rather than falling back to a literal; `MintedTelegramToken` (`:132-141`) stays transient (the bot persists nothing) |
| **`bearerForChat` / `setBearerHeader`** — `internal/telegram/bot.go:298` / `:320` | `bearerForChat` is the **only** caller of `MintForChat` (`bot.go:302`); `setBearerHeader` (`:321`) is the only caller of `bearerForChat` | The nil-minter branch (`:299`) falls back to the shared `b.authToken` — a **different authority source** that derivation does NOT cover. That split must stay explicit, or dev/test silently proves nothing about the production path |
| **Bridge **API client** call sites** — 13 `setBearerHeader(req, …)` calls across 8 files: `bot.go:956, 1040, 1139, 1198, 1439, 1499`; `mapping.go:35, 64`; `knowledge.go:28`; `list.go:329`; `annotation.go:214`; `photo_upload.go:151`; `recipe_commands.go:517` | Each attaches the derived bearer to an internal API request | Corpus-reaching calls — `/api/search` (`bot.go:174`, `recipe_commands.go:476`), `/api/digest` (`bot.go:175`), `/api/recent` (`bot.go:176`, `recipe_commands.go:177`), `/api/knowledge` (`bot.go:178`, `knowledge.go:28`) — begin receiving 403 for an ungranted principal and must render an operator-actionable message. **Non-corpus** calls MUST keep working: `/api/internal/telegram-message-artifact` (`mapping.go:28,56`) and `/api/artifacts/{id}/annotations` (`annotation.go:178`) are ungated per design.md §2 and depend on `annotation:edit` surviving derivation |
| **Exported test seam** — `internal/telegram/test_helpers.go:52` `SetBearerHeaderForTest` (`:62`) | Wraps the unexported `setBearerHeader` for out-of-package Telegram tests | Any out-of-package test binding through this seam inherits the new authority source |
| **Production wiring** — `cmd/core/wiring.go:767` | The **only** production `telegram.NewPerUserTokenMinter(...)` construction; `SetPerUserTokenMinter` (`bot.go:238`) applied at `wiring.go:783` | A new required option must be threaded here; a nil/zero grant reader must fail construction loudly rather than degrade to a literal |
| **Dev/test Bot constructor** — `internal/telegram/bot_webhook_test_mode.go:37` `NewBotForWebhookTestMode` | Builds a non-production `*Bot` with corpus URLs (`:51-55`) and **no** `tokenMinter`, so it exercises the shared-token path | Recorded so a green webhook-test-mode run is never read as evidence that derivation works |
| **Minter construction fixtures** | `internal/telegram/per_user_token_test.go:28, 56, 69, 149, 172` and `internal/telegram/bot_wiring_test.go:95, 137, 175, 243` — 9 sites | Every site constructs today's 6-field options struct and must be updated deliberately, never by supplying a permissive stub grant reader "to make tests compile" |
| **Absent fixture — recorded negative result** | `grep -rn 'annotation:edit' internal/telegram/` returns **only** `per_user_token.go:196, 199, 201` (the doc comment and the literal); `grep -rn 'Scopes' internal/telegram/*_test.go` returns **nothing** | **No test asserts the minted scope claim today.** The surface this sweep expected to find does not exist, so the current suite would stay green if derivation silently regressed to a literal. That absence is precisely why `TP-04-08` and `TP-04-09` are required, and it is stated here rather than assumed away |
| **Operator CLI grant path** — `cmd/core/cmd_auth.go` | `runAuthEnroll` (`:162`), `runAuthRotate` (`:255`), `runAuthListUsers` (`:385`), `runAuthInspect` (`:632`), `validateScopeFlags` (`:557`), `resolveRotationScopes` (`:596`) | This is the surface that WRITES the grants derivation will READ, so it becomes an upstream dependency of Telegram behavior for the first time. `resolveRotationScopes` **replaces** rather than merges (`:596-620`): rotating a principal to add `corpus:read` without naming the full list silently drops `annotation:edit` and would silently revoke Telegram annotation capability once derivation is live (`SCN-108-F02`, `TP-04-10`) |
| **Scope registry** — `internal/auth/scopes.go:39` | `RegisteredScopeSurfaces = []string{"extension", "annotation", "knowledge-graph"}` — `corpus` is **absent**, so `validateScopeFlags` rejects `corpus:read` without `--allow-unknown-surface` | Scope 01 adds the surface; the closed-set tests at `internal/auth/scopes_test.go:37, 53, 71` must gain a `corpus` case in that same change set |
| **Grant persistence — verified absent** | `auth_tokens` (`internal/db/migrations/033_auth_per_user_bearer.sql:37-60`) carries `token_id, user_id, key_id, issued_at, expires_at, hashed_token, status, rotated_from_token_id, issued_by, issued_source` | There is **no scopes column and no server-side grant read path**, verified here rather than inherited. Derivation cannot ship until `F-108-UX-ROSTER-01` provides one; that is the blocking dependency in this scope's DoD, not a detail to route around |
| **Docs — stale-reference scan** | `docs/smackerel.md` §17.2, `docs/Operations.md`, `docs/API.md` | No doc may still state that the Telegram bridge carries a fixed `annotation:edit` claim, or that granting is a flag flip rather than a token rotation. Doc edits are owned by Scope 05; this sweep only records the stale set |

**Recorded negative results (so a later reader does not re-open them):** no URL or path changes
occur in this scope, so there is **no redirect** to add, **no breadcrumb** or **navigation**
entry to rewrite, and **no deep link** to retarget. The Telegram bridge hand-builds its requests
from `b.baseURL` (`bot.go:174-178`, `mapping.go:28`, `recipe_commands.go:539/552`), so there is
**no generated client** to regenerate for this surface either. Each of these is an assertion to
re-verify at implementation time, not an assumption.

### Shared Infrastructure Impact Sweep

Remediating callers means changing the **shared authentication bootstrap** that every first-party
surface inherits: the principal token-minting path, the Telegram bridge's service-token bootstrap,
and the **shared test fixtures** that mint sessions for the PWA, extension, and bridge suites. These
are common test-infrastructure surfaces — a single wrong fixture scope claim silently re-grants
every ungranted principal and makes the whole feature look like it works when it does not.

**Blast radius and downstream contract surfaces:**

- **Session/token bootstrap contract** — adding `corpus:read` to a principal's scope claim is a
  token rotation, so any shared fixture that hardcodes a scope claim must be updated deliberately,
  never widened "to make tests pass."
- **Role/grant context contract** — the daily-user vs. operator distinction is exactly what this
  feature enforces. A fixture that grants `corpus:read` to the *daily-user* fixture destroys the
  negative case that `TP-04-03` **and `TP-04-09`** depend on.
- **Bridge bootstrap contract (rewritten by §18 decision 3)** — the Telegram bridge's per-user
  token is minted in a different path from user principals, and derivation makes that path read
  the mapped principal's persisted grants. Two consequences: the bridge mint path now has a
  **new inbound dependency** on grant readability (`F-108-UX-ROSTER-01`), and a change to the
  user-principal grant path can now change bridge behavior where previously it could not.
  Changing one must not implicitly change the other in an unreviewed direction.
- **Ordering/timing contract** — the OBSERVE measurement window must complete *before* the remedy is
  chosen; choosing early makes the measured matrix rows fictional.
- **Storage contract** — unchanged. No fixture storage or seed path is modified, asserted rather
  than assumed.

**Canary before broad rerun:** `TP-04-06` runs the narrow session/token bootstrap canary first, so a
broken shared fixture is caught before the full suite is run against it.

### Change Boundary

This scope repairs callers only. Grant-model changes are explicitly out of bounds.

**Allowed file families:**

- The Telegram bridge's per-user token minting path (`internal/telegram/per_user_token.go`) —
  replacing the hardcoded scope list with derivation from the mapped principal's persisted grants
- Shared test fixtures that mint principal, extension, and bridge sessions
- New/extended test files named in the Test Plan

**Excluded surfaces (must remain byte-unchanged by this scope):**

- `dailyUserGrants` and `operatorGrants` — widening a grant set to avoid a caller break is the
  precise failure mode this feature exists to prevent, and §18 decision 2 makes the prohibition
  permanent
- Any minter-side hardcoded scope list — reintroducing one anywhere is a §18 decision 3 violation,
  not an implementation shortcut
- The `/api/assistant/turn` routing path — the re-route alternative is CLOSED; do not implement it
- The external GuestHost connector credential — §18 decision 4 forbids granting it `corpus:read`;
  its migration is spec 109's, not this scope's
- Spec 109's artifacts — this packet does not edit them
- `internal/api/router.go` gate mount — owned by Scope 03
- `internal/metrics`, `cmd/core`, `config/` — owned by Scopes 02 and 05
- `docs/` — owned by Scope 05

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-04-01 | unit | Telegram bridge token/scope derivation test | The minted token's scope claim is **derived** from the mapped principal's persisted grant set with no silent default: an unresolvable principal aborts the mint rather than falling back to a hardcoded list (SCN-108-E01, §18 decision 3) | `./smackerel.sh test unit` |
| TP-04-02 | integration | bridge → API against the ephemeral test stack | Under ENFORCE a Telegram corpus command completes for a principal whose persisted grants include `corpus:read`; the failure mode for a principal without it is an operator-actionable outcome, never an unexplained 403 (SCN-108-E01) | `./smackerel.sh test integration` |
| TP-04-03 | integration | `internal/api` against the ephemeral test stack | A daily-user principal is denied under ENFORCE, and the **same** principal succeeds after a token rotation adding `corpus:read` — with no feature-flag change (SCN-108-E02) | `./smackerel.sh test integration` |
| TP-04-04 | integration | same | The browser extension's outcome tracks its principal exactly: granted → 200, ungranted → the same 403 as the PWA; no extension-specific grant exists (SCN-108-E03) | `./smackerel.sh test integration` |
| TP-04-05 | e2e-api | `./smackerel.sh test e2e` | Every row of the design.md §5 compatibility matrix is exercised end-to-end — daily user, operator, extension, Telegram bridge, shared token, bootstrap — with the recorded break/no-break outcome for each | `./smackerel.sh test e2e` |
| TP-04-06 | integration | shared session/token bootstrap canary | **Canary:** narrow, independently-runnable canary over the shared session-minting fixtures and the bridge token bootstrap — asserts the daily-user fixture still resolves **without** `corpus:read` (the negative case `TP-04-03` depends on), the operator fixture still resolves **with** it, and the bridge token bootstrap is independent of the user-principal path (**role** and **session** contracts). Run **before** any broad suite rerun so a silently-widened fixture cannot make the whole feature look green | `./smackerel.sh test integration` |
| TP-04-07 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-E01, SCN-108-E02, SCN-108-E03 and SCN-108-E04 against the live stack: the Telegram token still derives its authority from the mapped principal, a principal without `corpus:read` still gains no corpus access through Telegram, a daily user is still granted by token rotation rather than a flag flip, and the extension still tracks its principal exactly. Fails if a grant set is later widened, a minter-side scope list is reintroduced, or an extension-specific grant is introduced; also proves the broader e2e suite shows no green→red drift | `./smackerel.sh test e2e` |
| TP-04-08 | unit | `internal/telegram/per_user_token_test.go` | The hardcoded `Scopes: []string{"annotation:edit"}` list at `per_user_token.go:201` is **gone**: the minted claim equals the mapped principal's persisted grant set for a principal holding `annotation:edit` + `corpus:read`, and equals it for a principal holding only `annotation:edit`. Asserts the authority source is the principal, not a literal in the minter (SCN-108-E01, §18 decision 3) | `./smackerel.sh test unit` |
| TP-04-09 | integration | bridge → API against the ephemeral test stack | **ADVERSARIAL negative case:** a mapped Telegram chat whose principal does **NOT** hold `corpus:read` issues a corpus command under ENFORCE → the minted token does not carry the grant, the command is refused 403, and the refusal renders as a permanent operator-actionable condition rather than a transient retry. **Fails if any minter-side hardcoded list grants corpus access to every mapped chat** — the exact shortcut §18 decision 3 rejects, which a naive "Telegram works" test would pass (SCN-108-E04) | `./smackerel.sh test integration` |
| TP-04-10 | unit | `cmd/core/cmd_auth.go` rotation-semantics test | **Consumer Impact Sweep follow-through.** `resolveRotationScopes` (`cmd_auth.go:596`) **replaces** rather than merges, so rotating a principal to add `corpus:read` without naming the full list silently drops `annotation:edit` — which, once derivation is live, silently revokes the principal's Telegram annotation capability. Proves the sweep's rotation contract: rotating a principal holding `annotation:edit` to add `corpus:read` yields a token carrying **both**, and the replace-not-merge semantic is asserted rather than discovered in production (SCN-108-F02, F-108-GRANT-MECHANISM-01) | `./smackerel.sh test unit` |

### Definition of Done

- [ ] F-108-TELEGRAM-01 resolved by the **ratified** direction only (`spec.md` §18 decision 3): the minted Telegram scope claim is **derived from the mapped principal's persisted grant set**. The hardcoded list at `per_user_token.go:201` is REPLACED; neither the hardcoded-list extension nor the `/api/assistant/turn` re-route is implemented
- [ ] `F-108-UX-ROSTER-01` (server-side grant readability) is resolved before derivation ships, or this scope is recorded BLOCKED — derivation is **not** worked around with a minter-side list
- [ ] **§18 decision 4 honored:** the external GuestHost connector credential is **NOT** granted `corpus:read`; Tier A group 7 (`/api/context-for`) being gated with no granted external reader until BUG-019-003 clears is recorded as an accepted consequence, and the migration to the spec-109 `hospitality-read` path is routed to `bubbles.design` on spec 109 rather than solved here
- [ ] `dailyUserGrants` remains unchanged; no grant set is widened to avoid a caller break (§18 decision 2, permanent)
- [ ] `TP-04-01` unit test passes — bridge scope claim derived from the principal with no silent default
- [ ] `TP-04-02` integration test passes — Telegram corpus command under ENFORCE has an operator-actionable outcome
- [ ] `TP-04-03` integration test passes — token rotation (not a flag flip) grants a daily user access
- [ ] `TP-04-04` integration test passes — extension outcome tracks its principal; no extension-specific grant
- [ ] `TP-04-05` e2e-api test passes — all six design.md §5 compatibility rows exercised with their recorded outcome
- [ ] `TP-04-08` unit test passes — the minter's hardcoded scope list is gone and the minted claim equals the mapped principal's persisted grants
- [ ] `TP-04-09` adversarial integration test passes — a principal **without** `corpus:read` gains **no** corpus access through Telegram, and the test fails if a minter-side list is reintroduced
- [ ] `TP-04-10` unit test passes — rotating a principal holding `annotation:edit` to add `corpus:read` yields a token carrying **both**, so the `resolveRotationScopes` replace-not-merge semantic cannot silently revoke Telegram annotation capability once derivation is live (SCN-108-F02)
- [ ] Consumer Impact Sweep completed for the Telegram minter interface change across the minter, `bearerForChat`/`setBearerHeader`, the 13 bridge **API client** call sites, the exported test seam, the production wiring and dev/test Bot constructors, the 9 minter-construction fixtures, the operator CLI grant path, the scope registry, and the docs: zero stale first-party references remain, and each recorded negative result — no test asserts the minted scope claim today, `auth_tokens` has no scopes column, and no redirect/breadcrumb/navigation/deep link/generated client is affected — is **re-verified against the code rather than inherited from this plan**
- [ ] Every "unknown" row in the design.md §5 matrix is now a measured row; the OBSERVE-window go/no-go query returns an empty (or explicitly-accepted) denial set
- [ ] **Ratified coverage bar (§18 decision 1, strengthening item 7)** satisfied before the flip is authorised: (a) ≥ 14 consecutive OBSERVE days with the stage resolved from SST at process start; (b) per-principal × per-route-group coverage across all **sixteen** groups, each cell closed by observed traffic **or** a recorded operator `idle-by-design` attestation naming a reason and the principal — including all eight silent Tier B groups; (c) zero would-deny for any principal the operator intends to keep, with intentionally-denied principals recorded **before** the flip; (d) the window reset to day zero on any new principal enrollment or new client surface. No cell is silently unobserved; `OBSERVE-CLEAN` is not asserted otherwise
- [ ] **Ratified proactive rotation (item 9)** complete: every principal whose grants are unknowable has been rotated with a deliberately-issued grant set **before** Scope 05 flips the owning-train flag, so no `unknown` grant remains in the pre-flip roster. Recorded as an operator action, not inferred from telemetry
- [ ] `TP-04-06` canary integration test passes — shared session/token bootstrap fixtures keep the ungranted daily-user negative case intact
- [ ] `TP-04-07` regression e2e-api test passes — grant derivation, the adversarial negative case, the token-rotation grant path, and extension grant inheritance are permanently protected
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-04-07`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Live-category tests emit telemetry tagged `env=test*` only; no write to prod monitoring (R-108-O6, G115)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

---

## Scope 05: Docs, Release Train, Flag Bundles

**Status:** Not Started
**Depends On:** Scope 04
**Surfaces:** `docs/Operations.md`, `docs/API.md`, `docs/smackerel.md` §17.2, `docs/releases/v1/features.md`, `config/release-trains.yaml`, `config/feature-flags.next.yaml`, `config/feature-flags.mvp.yaml`

### Use Cases (Gherkin)

**SCN-108-R01 — The flag is default-ON in exactly one train**

```gherkin
Given the flag corpusGrantEnforcement is declared
When every train's flag bundle is inspected
Then corpusGrantEnforcement is true in exactly one bundle, the owning train next
And it is false in every other bundle, including mvp
And the mvp bundle carries the metadata block naming owning_spec, introduced_in_train, and introduced_at
And the release-train guard reports zero violations
```

**SCN-108-R02 — The SST key has no default and reaches every environment**

```gherkin
Given auth.corpus_grant_enforcement is declared in config/smackerel.yaml
When ./smackerel.sh config generate runs for each environment
Then SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT is present in every generated env file
And config/smackerel.yaml declares the key with no default value
And no ${VAR:-default} or getenv-with-default shape appears in the resolution path
```

**SCN-108-R03 — The operator runbook answers "who would have been denied?"**

```gherkin
Given docs/Operations.md documents the corpus grant rollout
When the operator follows the UC-108-001 runbook
Then the documented query sum by (user_id, route_group) (increase(smackerel_auth_corpus_grant_would_deny_total[7d]))
     returns the principals that must be granted corpus:read
And the go/no-go criterion for flipping to ENFORCE is stated
And the OBSERVE to ENFORCE to rollback procedure is stated with no rebuild step
```

**SCN-108-R04 — The release packet records this capability**

```gherkin
Given the next train is the promotion candidate feeding the v1 gate packet
When docs/releases/v1/features.md is inspected
Then it carries an entry for corpus grant enforcement
And that entry names the owning spec 108-corpus-grant-enforcement
And that entry names the owning train next
And that entry names the flag corpusGrantEnforcement
And the packet therefore does not silently omit a shipped capability
```

**SCN-108-R05 — The flag retirement contract is recorded, not implied**

```gherkin
Given corpusGrantEnforcement is default-ON in its owning train next
When the flag lifecycle documentation is inspected
Then it records that the flag dies with its train plus one cycle
And it records that at retirement the flag, the observe branch, and the two would-deny counters
     are deleted together
And it records that enforcement becomes unconditional once the flag is retired
And it records that bubbles.train owns both the owning-train flip and the retirement
```

### Implementation Plan

- **`docs/Operations.md`** — under the existing **"Authentication Metrics"** heading: the
  three `smackerel_auth_corpus_grant_*` metrics with their closed label sets; the UC-108-001
  observation runbook (`sum by (user_id, route_group) (increase(..._would_deny_total[7d]))`,
  the go/no-go criterion, and the OBSERVE→ENFORCE→rollback procedure from design.md §6).
  The documented go/no-go criterion is the **ratified coverage bar** (item 7): all eight route
  groups show real traffic **or** carry a recorded `idle-by-design` attestation with a reason
  and a named principal — a zero counter over a silently idle group is never read as clean.
  The runbook also states the **proactive rotation** precondition (item 9): no principal with
  unknowable grants remains unrotated at flip time.
- **`docs/API.md`** — add `corpus:read` to the per-endpoint scope column for the eight corpus
  route groups; document the 403 denial envelope and its zero-leakage guarantee; state
  explicitly which routes are **not** gated and why.
- **`docs/smackerel.md` §17.2** — update the caller-surface table with the design.md §5
  compatibility matrix, including which surfaces break at ENFORCE and what the operator must
  issue. Record that granting is a **token rotation**, not a flag flip. Record the ratified
  admin-surface position (item 8): the token page carries the **grant-issuance notice only**;
  there is **no grant editor in this spec**, and grants are therefore changed exclusively via
  `smackerel auth rotate …`. Record the proactive-rotation precondition (item 9) as operator
  procedure rather than as an implied step.
- **`docs/releases/v1/features.md`** — the `next` train is the promotion candidate feeding the
  **v1 gate** packet (`docs/INVESTOR_OVERVIEW.md`), so the capability is recorded in that
  packet's feature list: the corpus grant enforcement entry, its owning spec
  `108-corpus-grant-enforcement`, its owning train `next`, and its flag
  `corpusGrantEnforcement`. A shipped capability absent from the packet is a silent omission,
  not a documentation nicety.
- **`config/release-trains.yaml`** — no structural change; confirm the existing `next` train
  (`phase: active`, `target_slot: staging`) still resolves `flags_bundle:
  config/feature-flags.next.yaml`.
- **`config/feature-flags.next.yaml`** — `corpusGrantEnforcement: true` (default-ON in the
  **owning** train) with an owning-spec comment (R-108-CFG1). Flipping the owning train's
  default is `bubbles.train`'s operation and MUST follow a clean observation window from
  Scope 04.
- **`config/feature-flags.mvp.yaml`** — `corpusGrantEnforcement: false` plus the `metadata:`
  block recording `owning_spec: specs/108-corpus-grant-enforcement/`,
  `introduced_in_train: next`, `introduced_at` (R-108-CFG2, R-108-FL4). `mvp` observes and
  counts, never denies.
- **`config/smackerel.yaml`** — the key declared in Scope 02 is re-verified here for
  no-default compliance across every generated environment.
- **Flag lifecycle (R-108-FL7).** Record that the flag dies with its train + one cycle: once
  enforcement is permanent, `corpusGrantEnforcement`, the observe branch, and the two
  would-deny counters retire together.
- **Divergence to reconcile.** design.md §4/§9 records `false` in **both** bundles; the
  enforced release-train policy requires default-ON in exactly one owning train. This scope is
  planned to the enforced policy and blocks on `bubbles.design` reconciling design.md
  (DoD-05-06). Do **not** silently edit design.md from this packet.

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-05-01 | unit | flag-bundle parity test | `corpusGrantEnforcement` is `true` in exactly one bundle (`next`, the owning train) and `false` in every other bundle including `mvp`; the `mvp` metadata block names `owning_spec`, `introduced_in_train`, `introduced_at` (SCN-108-R01) | `./smackerel.sh test unit` |
| TP-05-02 | unit | config-compile test | `auth.corpus_grant_enforcement` is declared with **no default value**; no `${VAR:-...}` or getenv-with-default shape exists in the resolution path (SCN-108-R02) | `./smackerel.sh test unit` |
| TP-05-03 | integration | `./smackerel.sh config generate` across environments | `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` is emitted into every generated env file, and a missing key fails generation loudly (SCN-108-R02) | `./smackerel.sh test integration` |
| TP-05-04 | e2e-api | `./smackerel.sh test e2e` | Against the ephemeral test stack, the documented UC-108-001 runbook query returns the documented shape (`user_id`, `route_group`, count) from the real `/metrics` surface, so the runbook in `docs/Operations.md` is executable and not aspirational (SCN-108-R03) | `./smackerel.sh test e2e` |
| TP-05-05 | unit | release-packet documentation-contract test | `docs/releases/v1/features.md` carries the corpus grant enforcement entry naming its owning spec `108-corpus-grant-enforcement`, its owning train `next`, and its flag `corpusGrantEnforcement`; fails if the packet omits the shipped capability (SCN-108-R04) | `./smackerel.sh test unit` |
| TP-05-06 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-R01, SCN-108-R02, SCN-108-R03, SCN-108-R04 and SCN-108-R05 against the live stack: the flag stays default-ON in exactly one train, the SST key stays default-free in every generated environment, the documented runbook query keeps returning its documented shape, the v1 release packet keeps recording the capability, and the retirement contract stays recorded. Fails if a second train flips the flag ON, a default is reintroduced, the runbook goes stale, the packet entry is dropped, or the retirement contract is deleted; also proves the broader e2e suite shows no green→red drift | `./smackerel.sh test e2e` |
| TP-05-07 | unit | flag-lifecycle documentation-contract test | The retirement contract ratified by `spec.md` §18 decision 6 is **recorded, not implied**: the flag-lifecycle documentation states that `corpusGrantEnforcement` dies with its train + one cycle, that the flag, the observe branch, and the two would-deny counters retire **together**, that enforcement becomes unconditional afterwards, and that `bubbles.train` owns both the flip and the retirement. Fails if any of the four clauses is absent, so the obligation cannot decay into an implied one (SCN-108-R05, R-108-FL7) | `./smackerel.sh test unit` |

### Definition of Done

- [ ] `docs/Operations.md` documents the three `smackerel_auth_corpus_grant_*` metrics with closed label sets and the full UC-108-001 runbook (query, go/no-go criterion, OBSERVE→ENFORCE→rollback with no rebuild step), where the documented go/no-go criterion is the ratified coverage bar (item 7 — traffic on all eight groups or a recorded `idle-by-design` attestation) and the documented preconditions include proactive rotation (item 9)
- [ ] `docs/API.md` lists `corpus:read` for all eight corpus route groups, documents the 403 envelope and its zero-leakage guarantee, and states which routes are not gated and why
- [ ] `docs/smackerel.md` §17.2 carries the design.md §5 compatibility matrix and records that granting is a token rotation, not a flag flip, and records the ratified admin-surface position (item 8): grant-issuance **notice only**, **no grant editor in this spec**, grants changed exclusively via `smackerel auth rotate …`
- [ ] `docs/releases/v1/features.md` records the corpus grant enforcement capability with its owning spec `108-corpus-grant-enforcement`, its owning train `next`, and its flag `corpusGrantEnforcement`, so the `v1`-gate packet does not silently omit a shipped capability (SCN-108-R04)
- [ ] `config/release-trains.yaml` verified — `next` train still resolves `flags_bundle: config/feature-flags.next.yaml`; no structural change made
- [ ] `config/feature-flags.next.yaml` sets `corpusGrantEnforcement: true` (owning train) and `config/feature-flags.mvp.yaml` sets it `false` with the required `metadata:` block
- [ ] design.md §4/§9 flag-default divergence reconciled by `bubbles.design` (or the enforced-policy value ratified by the operator) before this scope closes — not silently overwritten from this packet
- [ ] `TP-05-01` unit test passes — flag default-ON in exactly one train, default-OFF elsewhere, metadata present
- [ ] `TP-05-02` unit test passes — SST key declared with no default; no fallback shape in the resolution path
- [ ] `TP-05-03` integration test passes — generated env carries the variable for every environment
- [ ] `TP-05-04` e2e-api test passes — the documented runbook query returns the documented shape against the real `/metrics` surface
- [ ] `TP-05-05` unit test passes — the `v1` release packet's `features.md` records the capability, its owning spec, its owning train, and its flag
- [ ] Flag lifecycle recorded (R-108-FL7): flag + observe branch + would-deny counters retire together, train + one cycle
- [ ] `TP-05-07` unit test passes — the flag-lifecycle documentation carries all four retirement clauses (train + one cycle; flag/observe-branch/counters deleted together; enforcement unconditional afterwards; `bubbles.train` owns the flip and the retirement), so `spec.md` §18 decision 6 is a testable obligation rather than an implied one (SCN-108-R05)
- [ ] `TP-05-06` regression e2e-api test passes — single-owning-train flag default, default-free SST key, the executable runbook, and the release-packet entry are permanently protected
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-05-06`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; docs and config aligned; no TODO/stub/default introduced
