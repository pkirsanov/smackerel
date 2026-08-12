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
5. **Scope 05 — Docs, Release Train, Flag Bundles.** `docs/Operations.md`, `docs/API.md`, `docs/smackerel.md` §17.2, the `v1`-gate release packet's `docs/releases/v1/features.md`, `config/release-trains.yaml`, `config/feature-flags.next.yaml` (default-OFF; owning train `next`), `config/feature-flags.mvp.yaml` (default-OFF). *(Corrected 2026-08-11 by `bubbles.plan`; prior wording read "`config/feature-flags.next.yaml` (default-ON for the owning train `next`)" — see the PLAN-TEXT CORRECTION block at the head of Scope 05.)*

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
| Scope 05 | Flag-bundle parity check: `corpusGrantEnforcement` is **declared in every train bundle** and **default-OFF in every one of them** (R-108-FL3), with the `mvp` metadata block intact, and the check rejects default-ON on a **non-owning** train (the `G111` condition); SST key has no default; the retirement contract (§18 decision 6 — flag **and** observe branch deleted together) is recorded. *(Corrected 2026-08-11 by `bubbles.plan`; prior wording read "is default-ON in exactly one train and default-OFF in every other".)* |

### Planning Note — Flag Default "Divergence" (WITHDRAWN — the premise was false)

> **WITHDRAWN 2026-08-11 by `bubbles.plan`.** This note asserted a divergence that does not
> exist, on a premise that is false. The prior text is preserved verbatim below rather than
> deleted, because it is the origin of the same claim that propagated into Scope 05.
>
> **Prior text (withdrawn):**
>
> > `design.md` §4/§9 records `corpusGrantEnforcement: false` in **both**
> > `config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml` (R-108-FL3).
> > The repo's mechanically-enforced release-train policy
> > (`.github/instructions/bubbles-release-trains.instructions.md`, `release-train-guard.sh`)
> > requires a flag to be default-ON in **exactly one** owning train and default-OFF in every
> > other. Scope 05 is planned to the enforced policy: **`next` = ON (owning train), `mvp` = OFF**.
> > This divergence from `design.md` is recorded here rather than silently applied; reconciling
> > `design.md` §4/§9 is owned by `bubbles.design`, not by this planning packet. Scope 05 DoD
> > item **DoD-05-06** blocks on that reconciliation.
>
> **Why it is withdrawn.** The release-train policy does **not** require a flag to be default-ON
> anywhere. `.github/bubbles/scripts/release-train-guard.sh` Check 8 (lines 119–142) loops over
> every train and **skips the owning train** (`[[ "$tid" == "$spec_train" ]] && continue`,
> line 132), so it can raise `G111 violation` (line 138) **only** for default-ON on a
> **non-owning** train. An all-OFF dormant flag is fully conformant. Because the policy never
> demanded ON, there was never a divergence from `design.md` to reconcile, and `bubbles.design`
> was never owed a change.
>
> **Corrected position.** `corpusGrantEnforcement` ships **default-OFF in every train**, per
> `spec.md` **R-108-FL3** (line 519), `design.md` **§4** (line 191), and `design.md` **§9**
> (lines 358–359) — three artifacts that all agree. `bubbles.train` flips `next` ON only after
> a clean observation window. The full correction, its authority, and its empirical proof are
> recorded in the **PLAN-TEXT CORRECTION** block at the head of Scope 05.

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

- [x] `auth.RegisteredScopeSurfaces` includes `corpus`; `dailyUserGrants` and `operatorGrants` are unchanged

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02

  ```text
  $ grep -n 'RegisteredScopeSurfaces = ' internal/auth/scopes.go
  46:var RegisteredScopeSurfaces = []string{"extension", "annotation", "knowledge-graph", "corpus"}

  $ git --no-pager diff -- internal/auth/scopes.go   # +/- lines only
  -var RegisteredScopeSurfaces = []string{"extension", "annotation", "knowledge-graph"}
  +// Spec 108 SCOPE-01 adds `corpus`, the surface of the already-defined
  +// GrantGlobalCorpusRead ("corpus:read") constant in
  +// browser_session_policy.go. Registration makes that grant MINTABLE via
  +// `smackerel auth enroll --scope corpus:read` without the
  +// --allow-unknown-surface escape hatch; it does NOT grant it. The daily
  +// default grant set is deliberately unchanged — an ungranted daily user
  +// stays denied (spec 108 design.md "Resolved Decisions").
  +var RegisteredScopeSurfaces = []string{"extension", "annotation", "knowledge-graph", "corpus"}

  $ git --no-pager diff -- internal/auth/browser_session_policy.go | grep -E '^[+-].*(dailyUserGrants|operatorGrants)'
  GRANT_DIFF_GREP_EXIT=1 (1 = no matching +/- lines)

  $ git status --porcelain -- internal/auth/browser_session_policy.go
  STATUS_EXIT=0 (no line above = file unmodified)

  # adversarial guard proving registration did not widen the default grant set:
  === RUN   TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants
  --- PASS: TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants (0.00s)
  ```

  `browser_session_policy.go` — which declares both `dailyUserGrants` and `operatorGrants` — is
  unmodified in the working tree, so neither grant set was widened. Only `scopes.go` changed.

- [x] `TP-01-01` unit test passes — `corpus` surface registered and mapped to `GrantGlobalCorpusRead`

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02

  ```text
  $ ./smackerel.sh test unit --go --go-run 'Corpus|ScopeSurface|AuthSurface' --verbose

  === RUN   TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant
  --- PASS: TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant (0.00s)
  === RUN   TestCorpusReadScopeClaimValidatesAndAuthorizes
  --- PASS: TestCorpusReadScopeClaimValidatesAndAuthorizes (0.00s)
  === RUN   TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants
  --- PASS: TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants (0.00s)
  === RUN   TestRegisteredScopeSurfaces_ContainsExtension
  --- PASS: TestRegisteredScopeSurfaces_ContainsExtension (0.00s)
  === RUN   TestRegisteredScopeSurfaces_ContainsAnnotation
  --- PASS: TestRegisteredScopeSurfaces_ContainsAnnotation (0.00s)
  === RUN   TestRegisteredScopeSurfaces_ContainsKnowledgeGraph
  --- PASS: TestRegisteredScopeSurfaces_ContainsKnowledgeGraph (0.00s)
  === RUN   TestRegisteredScopeSurfaces_ContainsCorpus
  --- PASS: TestRegisteredScopeSurfaces_ContainsCorpus (0.00s)
  === RUN   TestExtractScopeSurface
  --- PASS: TestExtractScopeSurface (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/auth    0.045s
  ```

  Test location note: `TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant` lives in the
  new `internal/auth/browser_session_policy_test.go` (untracked in this working tree), matching
  the TP-01-01 location declared in the Test Plan above.

- [x] `TP-01-02` unit test passes — `corpus:read` scope claim validates and authorizes

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02

  ```text
  $ ./smackerel.sh test unit --go --go-run 'Corpus|ScopeSurface|AuthSurface' --verbose
  ...
  === RUN   TestCorpusReadScopeClaimValidatesAndAuthorizes
  --- PASS: TestCorpusReadScopeClaimValidatesAndAuthorizes (0.00s)
  ...
  [go-unit] go test ./... finished OK
  + echo '[go-unit] go test ./... finished OK'
  UNIT_EXIT=0

  # whole-run failure scan over the captured 739-line run log:
  FAIL_LINE_COUNT=0
  PASS_LINE_COUNT=29
  TOTAL_LINES=739
  ```

  `TestCorpusReadScopeClaimValidatesAndAuthorizes` asserts SCN-108-P02: a scope claim carrying
  `corpus:read` validates without an unknown-surface error and `AuthorizeGrant` returns
  authorized. The run exited 0 with zero `FAIL` lines across every package.

- [x] `TP-01-03` integration test passes — minted `corpus:read` token round-trips to a granted session
  - **Command:** `./smackerel.sh test integration --go-run 'TP_01_03'`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestCorpusGrant_TokenRoundTripsToAGrantedSession_TP_01_03 (0.02s)`
    and `--- PASS: TestCorpusGrant_UngrantedPrincipalIsDeniedAndRecordedAsNone_TP_01_03 (0.01s)`
    in `tests/integration/corpus_grant_roundtrip_test.go`. The test drives the real path
    against the ephemeral stack's PostgreSQL: `IssueToken` → `VerifyAndParse` (the step
    `bearerAuthMiddleware` performs) → `Session` → `GateGlobalCorpusRead`, then reads back
    `GrantsForPrincipal` so the server's RECORDED grant must agree with the token's claim.
    The ungranted principal is the anti-tautology control: it is persisted with a non-nil
    EMPTY scope set and must be BOTH denied at the gate AND reported `Recorded=true` with
    no corpus grant, which pins the NULL-vs-`'{}'` distinction spec.md §7 forbids
    conflating. If the gate ever authorized unconditionally, the positive case would still
    pass and only this one would fail.

- [x] `internal/api/auth_surface_contract_test.go` surface list updated in the same change (no stale contract)

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02

  ```text
  $ git status --porcelain -- internal/api/auth_surface_contract_test.go
   M internal/api/auth_surface_contract_test.go

  $ grep -nE 'corpus_scope_surface_registered_without_widening_defaults|IsRegisteredScopeSurface\("corpus"\)|ExtractScopeSurface\(auth.GrantGlobalCorpusRead\)|must NOT widen the daily default grant set' internal/api/auth_surface_contract_test.go
  122:    t.Run("corpus_scope_surface_registered_without_widening_defaults", func(t *testing.T) {
  123:            if !auth.IsRegisteredScopeSurface("corpus") {
  129:            if s := auth.ExtractScopeSurface(auth.GrantGlobalCorpusRead); s != "corpus" {
  135:                    t.Fatalf("registering the corpus surface must NOT widen the daily default grant set")

  $ ./smackerel.sh test unit --go --go-run 'Corpus|ScopeSurface|AuthSurface' --verbose
  --- PASS: TestSurfaceInventoryRoleGrantMatrixAndGlobalCorpusGateUseUnifiedAuthenticatorAndRejectBypassHelpers (0.00s)
      --- PASS: .../role_grant_matrix (0.00s)
      --- PASS: .../global_corpus_grant_gate (0.00s)
      --- PASS: .../corpus_scope_surface_registered_without_widening_defaults (0.00s)
      --- PASS: .../authority_flows_from_unified_authenticator_across_surfaces (0.00s)
      --- PASS: .../reject_client_supplied_role_and_unverified_session (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/api     0.237s

  # the grant boundary still denies the ungranted after registration:
  --- PASS: TestGraphReadGrantMatrix_GlobalCorpus/ungranted_authenticated_identity_is_denied (0.00s)
  ok      github.com/smackerel/smackerel/internal/api/graphapi    0.016s
  ```

  The contract test was updated in the same working-tree change as `scopes.go`, so the surface
  inventory it asserts is not stale.

- [x] `TP-01-04` regression e2e-api test passes — `corpus` surface registration and `corpus:read` minting are permanently protected against silent removal
  - **Command:** `./smackerel.sh test e2e --go-run 'TP_01_04'`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestE2E_Spec108_CorpusSurfaceStaysRegistered_TP_01_04 (0.03s)`,
    `--- PASS: TestE2E_Spec108_CorpusGrantIsNotInTheDailyDefaultSet_TP_01_04 (0.00s)`,
    `--- PASS: TestE2E_Spec108_CorpusTokenStillMintsAndAuthorizes_TP_01_04 (0.00s)` in
    `tests/e2e/auth/spec108_corpus_surface_regression_test.go`. The prior UNPROVEN note
    blamed five pre-existing e2e defects; all five were fixed earlier in this session
    (commits `49dc5b29`, `7e6b4d72`), so the baseline is green and the row is now
    honestly closeable. The suite probes live-stack health first, so the remaining
    assertions cannot pass against a dead stack.

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-01-04`, `./smackerel.sh test e2e`)
  - **Command:** `./smackerel.sh test e2e --go-run 'TP_01_04'`
  - **Exit Code:** 0
  - **Evidence:** the three PASS lines above cover SCN-108-P01 (surface stays registered,
    `GrantGlobalCorpusRead == "corpus:read"`, validates against the closed registry) and
    SCN-108-P02 (mint → verify → gate authorizes). Each carries its adversarial control:
    an unscoped token MUST be denied and a wildcard MUST NEVER be honored, so a gate that
    authorized unconditionally fails rather than passing quietly. The daily-vs-operator
    invariant is asserted in both directions — `corpus:read` absent from the daily default
    set (else the Scope 03 gate is a no-op) and present for the operator.

- [x] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
  - **Command:** `./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Evidence:** `E2E_EXIT=0` with `--- FAIL` count `0` and 417 `--- PASS`; shell tier
    `Total: 36 / Passed: 36 / Failed: 0`. The prior UNPROVEN note said the suite exited 1
    on pre-existing unrelated failures — those five defects were fixed earlier in this
    session (`49dc5b29` PWA immutable-asset headers + drive fail-loud harness, `7e6b4d72`
    assistant short-circuit turn identity), so the baseline is green and a green→red delta
    can now actually be attributed. No previously-passing test regressed.

- [x] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced
  - **Command:** `./smackerel.sh check && ./smackerel.sh format --check && ./smackerel.sh lint`
  - **Exit Code:** 0, 0, 0
  - **Evidence:** `CHECK=0`, `FMT=0` with `78 files already formatted`, `LINT=0`. The two
    new test files introduce no TODO, stub, default or fallback; the fail-loud
    `PersistToken` contract (`requires TokenID, UserID, KeyID, HashedToken, IssuedBy,
    IssuedSource`) rejected the first fixture draft, which is the NO-DEFAULTS policy
    working as intended rather than an obstacle to route around.

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
| TP-02-07 | unit | `internal/api/corpus_grant_gate_test.go` | This scope's observe middleware NEVER denies, in **either** stage — no 403 originates from `corpusGate.Observe`. Asserted by `TestCorpusGrantGate_Observe_NeverDeniesInEitherStage` with an `observe_stage` and an `enforce_stage` subtest, so the enforce-stage half is proven rather than assumed: under ENFORCE only `RequireScope` may deny. Added when the Scope 02/03 boundary was split (see the DoD note) so the retained half is covered by a Test Plan row rather than by a DoD item alone | `./smackerel.sh test unit` |

### Definition of Done

- [x] `auth.corpus_grant_enforcement` declared in `config/smackerel.yaml` with no default; generated env emits `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT`; no `${VAR:-...}` / `os.Getenv`-with-default shape anywhere in the resolution path

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh check` plus SST/resolution-path greps (shown inline)
  **Exit Code:** 0

  ```text
  $ grep -n -A3 'corpus_grant_enforcement' config/smackerel.yaml
  1143:  corpus_grant_enforcement: false
  1144-
  1145-# Spec 061 SCOPE-06c (Round 71d) — Hardware-tier × model-role matrix.
  1146-# Switch key: SMACKEREL_HARDWARE_TIER={cpu,accel}. REQUIRED at

  $ grep -n 'CORPUS_GRANT_ENFORCEMENT' scripts/commands/config.sh
  1861:SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT="$(required_value auth.corpus_grant_enforcement)"
  2788:SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=${SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:?auth.corpus_grant_enforcement resolved empty — spec 108 R-108-FL5 forbids emitting an empty enforcement stage}

  $ grep -n 'SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT' config/generated/*.env
  config/generated/dev.env:437:SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=false
  grep: config/generated/home-lab.env: Permission denied
  grep: config/generated/self-hosted.env: Permission denied
  config/generated/test.env:437:SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=false

  $ grep -nE 'os\.Getenv\([^)]*\)[[:space:]]*;[[:space:]]*if|:-|unwrap_or' cmd/core/wiring_corpus_grant.go
  GREP_EXIT=1 (1 = no forbidden default shape found)

  $ ./smackerel.sh check
  config-validate: <repo-root>/config/generated/dev.env.tmp.4007465 OK
  Config is in sync with SST
  env_file drift guard: OK
  CHECK_EXIT=0

  === RUN   TestCorpusGrantEnforcement_ResolverHasNoDefaultShape
  --- PASS: TestCorpusGrantEnforcement_ResolverHasNoDefaultShape (0.00s)
  ```

  The key is declared in the YAML SST and resolved with `required_value` + the fail-loud
  `${VAR:?...}` form — never `${VAR:-...}`. The forbidden-shape grep over the resolution file
  returned exit 1 (no match), and `TestCorpusGrantEnforcement_ResolverHasNoDefaultShape` asserts
  the same property in Go. Honest limit: `home-lab.env` and `self-hosted.env` were unreadable
  (permission denied), so emission is verified for `dev.env` and `test.env` only.

- [x] Three `smackerel_auth_corpus_grant_*` metrics added to the existing `smackerel_auth_*` family; `smackerel_auth_scope_rejected_total` unchanged and not reused for the observe signal

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh test unit --go --go-run 'CorpusGrant|Corpus' --verbose` plus metrics-family greps
  **Exit Code:** 0

  ```text
  $ grep -nE 'smackerel_auth_corpus_grant_(would_deny_total|allowed_total|enforcement_mode)' internal/metrics/auth.go
  298://  sum by (user_id, route_group) (increase(smackerel_auth_corpus_grant_would_deny_total[7d]))
  305:            Name: "smackerel_auth_corpus_grant_would_deny_total",
  319:            Name: "smackerel_auth_corpus_grant_allowed_total",
  335:            Name: "smackerel_auth_corpus_grant_enforcement_mode",

  $ grep -n 'smackerel_auth_scope_rejected_total' internal/metrics/auth.go
  169:            Name: "smackerel_auth_scope_rejected_total",

  $ git --no-pager diff -- internal/metrics/auth.go | grep -cE '^[+-].*scope_rejected'
  0

  === RUN   TestCorpusGrantMetrics_RegisteredInAuthFamily
  --- PASS: TestCorpusGrantMetrics_RegisteredInAuthFamily (0.02s)
  === RUN   TestCorpusGrantMetrics_DoNotReuseScopeRejectedCounter
  --- PASS: TestCorpusGrantMetrics_DoNotReuseScopeRejectedCounter (0.00s)
  === RUN   TestCorpusGrantMetrics_WouldDenyAndAllowedIncrementIndependently
  --- PASS: TestCorpusGrantMetrics_WouldDenyAndAllowedIncrementIndependently (0.00s)
  === RUN   TestCorpusGrantMetrics_WouldDenyCarriesUserIDAndAllowedDoesNot
  --- PASS: TestCorpusGrantMetrics_WouldDenyCarriesUserIDAndAllowedDoesNot (0.00s)
  ok      github.com/smackerel/smackerel/internal/metrics 0.108s
  ```

  All three counters/gauge live in `internal/metrics/auth.go` alongside the pre-existing
  `smackerel_auth_*` metrics — the family was extended, not forked. The `scope_rejected` diff
  count is 0, so line 169 is untouched, and `TestCorpusGrantMetrics_DoNotReuseScopeRejectedCounter`
  asserts the observe signal does not ride on that counter.

- [x] `TP-02-01` unit test passes — absent config aborts startup naming the variable

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh test unit --go --go-run 'CorpusGrant|Corpus' --verbose`
  **Exit Code:** 0

  ```text
  === RUN   TestResolveCorpusGrantEnforcement_Observe
  --- PASS: TestResolveCorpusGrantEnforcement_Observe (0.00s)
  === RUN   TestResolveCorpusGrantEnforcement_Enforce
  --- PASS: TestResolveCorpusGrantEnforcement_Enforce (0.00s)
  === RUN   TestResolveCorpusGrantEnforcement_Absent_FailsLoud
  --- PASS: TestResolveCorpusGrantEnforcement_Absent_FailsLoud (0.00s)
  === RUN   TestResolveCorpusGrantEnforcement_Empty_FailsLoud
  --- PASS: TestResolveCorpusGrantEnforcement_Empty_FailsLoud (0.00s)
  === RUN   TestCorpusGrantEnforcementEnv_Absent
  --- PASS: TestCorpusGrantEnforcementEnv_Absent (0.00s)
  === RUN   TestCorpusGrantEnforcementEnv_Empty
  --- PASS: TestCorpusGrantEnforcementEnv_Empty (0.00s)
  === RUN   TestCorpusGrantEnforcement_SingleResolutionPointBeforeListenerBind
  --- PASS: TestCorpusGrantEnforcement_SingleResolutionPointBeforeListenerBind (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/cmd/core 0.361s
  ```

  `_Absent_FailsLoud` and `_Empty_FailsLoud` cover SCN-108-C03 in `cmd/core`, and
  `_SingleResolutionPointBeforeListenerBind` pins that the resolution happens once, before the
  listener binds. Honest limit: these prove the *resolver* aborts and names the variable; they
  are in-process assertions, not a spawned-process startup abort.

- [x] `TP-02-02` unit test passes — malformed config aborts startup naming the value

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh test unit --go --go-run 'CorpusGrant|Corpus' --verbose`
  **Exit Code:** 0

  ```text
  === RUN   TestResolveCorpusGrantEnforcement_Malformed_FailsLoud
  === RUN   TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"1"
  === RUN   TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"0"
  === RUN   TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"observe"
  === RUN   TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"ENFORCE"
  --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"1" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"0" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"t" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"f" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"TRUE" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"False" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"observe" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"ENFORCE" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"yes" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"no" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"on" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"off" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"shadow" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"dry-run" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"_true" (0.00s)
      --- PASS: TestResolveCorpusGrantEnforcement_Malformed_FailsLoud/"true_" (0.00s)
  ```

  SCN-108-C05 is covered by 16 adversarial subtests. The set is deliberately hostile — it
  includes the near-miss values a silent fallback would swallow (`"1"`, `"0"`, `"TRUE"`,
  `"yes"`), the two stage names a reader would expect to work (`"observe"`, `"ENFORCE"`), and
  whitespace-adjacent shapes (`"_true"`, `"true_"`). Every one aborts rather than defaulting.

- [x] `TP-02-03` unit test passes — metrics register with the closed **16**-value `route_group` label set (Tier A + Tier B)

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh test unit --go --go-run 'CorpusGrant|Corpus' --verbose`
  **Exit Code:** 0

  ```text
  === RUN   TestCorpusGrantRouteGroups_ClosedSixteenValueSet
  --- PASS: TestCorpusGrantRouteGroups_ClosedSixteenValueSet (0.00s)
  === RUN   TestCorpusGrantRouteGroups_ReturnsDefensiveCopy
  --- PASS: TestCorpusGrantRouteGroups_ReturnsDefensiveCopy (0.00s)
  === RUN   TestCorpusGrantRouteGroup_RejectsOutOfSetValues
  --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues (0.05s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group= (0.00s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=/api/search (0.01s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=/api/search?q=my+private+medical+question (0.00s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=/api/artifact/9f3c2a11-4d5e-4f60-9a7b-1c2d3e4f5a6b (0.01s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=SEARCH (0.00s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=search_ (0.02s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=artifact (0.01s)
      --- PASS: TestCorpusGrantRouteGroup_RejectsOutOfSetValues/group=unknown (0.00s)
  === RUN   TestCorpusGrantMetrics_AllEmittedLabelValuesStayInClosedSet
  --- PASS: TestCorpusGrantMetrics_AllEmittedLabelValuesStayInClosedSet (0.00s)
  ok      github.com/smackerel/smackerel/internal/metrics 0.108s
  ```

  `_ClosedSixteenValueSet` pins the cardinality at sixteen (Tier A + Tier B per `spec.md` §4.2),
  and `_RejectsOutOfSetValues` proves the rejection is real rather than decorative: the raw
  search path, a raw path carrying a private query string, and an artifact UUID are all refused
  as label values — which is R-108-O3/O4 (no path, no query text, no artifact id in a label).

- [x] `TP-02-04` integration test passes — OBSERVE returns 200 on all sixteen groups and counts would-be denials

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=9243ebdb
  **Executed:** YES (`~/i5.log`, 2026-08-11 20:16, preserved full-lane capture)
  **Command:** `./smackerel.sh test integration`
  **Exit Code:** 0

  ```text
  --- PASS: TestConfigValidate_AC5c_WrapperPropagatesRejection (5.49s)
  === RUN   TestIntegration_CorpusGrantObserve_UngrantedPrincipalIsCountedOnAllSixteenGroups
  --- PASS: TestIntegration_CorpusGrantObserve_UngrantedPrincipalIsCountedOnAllSixteenGroups (0.00s)
  === RUN   TestIntegration_CorpusGrantObserve_GrantedPrincipalCountsAllowedAndModeGaugeReportsObserve
  --- PASS: TestIntegration_CorpusGrantObserve_GrantedPrincipalCountsAllowedAndModeGaugeReportsObserve (0.00s)
  === RUN   TestIntegration_CorpusGrantObserve_ScopeRejectedCounterIsNotReused
  --- PASS: TestIntegration_CorpusGrantObserve_ScopeRejectedCounterIsNotReused (0.00s)
  === RUN   TestMigrations_AllTablesExist
  --- PASS: TestMigrations_AllTablesExist (0.02s)
  === RUN   TestMigrations_ArtifactsColumns
  --- PASS: TestMigrations_ArtifactsColumns (0.03s)
  ...
  PASS
  ok  	github.com/smackerel/smackerel/tests/integration	58.921s
  ...
  INTEGRATION_EXIT=0
  ```

  The test executed in the `tests/integration` package of the live lane (`ok … 58.921s`,
  `INTEGRATION_EXIT=0`; `grep -cE '^--- FAIL|^FAIL[[:space:]]+github' ~/i5.log` → 0). It drives
  the real `api.CorpusGrantGate`, the real `auth.GateGlobalCorpusRead` decision, and the real
  Prometheus collectors across `metrics.CorpusRouteGroups()` asserted to be exactly 16, and it
  fails on any of: a 403, a non-200, a mutated downstream body, a per-group `would_deny` delta
  ≠ 1, an `allowed` delta ≠ 0, a downstream that ran fewer than 16 times, a leaked query string /
  artifact id / title in the warn line, or fewer than 16 `corpus_grant_would_deny` events.

  **What this does NOT establish:** the request is driven through `httptest` against the gate
  middleware directly, not through a route on the running core service — Scope 02 does not mount
  the gate (`corpus_grant_observe_test.go:12-20` says so explicitly). The live-route half of
  SCN-108-O01 belongs to TP-02-06.

- [x] `TP-02-05` integration test passes — granted requests count as allowed, would-deny stays flat, mode gauge reports 0

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=9243ebdb
  **Executed:** YES (`~/i5.log`, 2026-08-11 20:16, preserved full-lane capture)
  **Command:** `./smackerel.sh test integration`
  **Exit Code:** 0

  ```text
  === RUN   TestIntegration_CorpusGrantObserve_GrantedPrincipalCountsAllowedAndModeGaugeReportsObserve
  --- PASS: TestIntegration_CorpusGrantObserve_GrantedPrincipalCountsAllowedAndModeGaugeReportsObserve (0.00s)
  === RUN   TestIntegration_CorpusGrantObserve_ScopeRejectedCounterIsNotReused
  --- PASS: TestIntegration_CorpusGrantObserve_ScopeRejectedCounterIsNotReused (0.00s)
  === RUN   TestMigrations_AllTablesExist
  --- PASS: TestMigrations_AllTablesExist (0.02s)
  === RUN   TestMigrations_SchemaVersionCount
      db_migration_test.go:150: schema_migrations count: 46
  --- PASS: TestMigrations_SchemaVersionCount (0.02s)
  === RUN   TestMigrations_TableDropAndRecreate
      db_migration_test.go:266: table drop and recreate verified
  --- PASS: TestMigrations_TableDropAndRecreate (0.09s)
  ...
  PASS
  ok  	github.com/smackerel/smackerel/tests/integration	58.921s
  ...
  INTEGRATION_EXIT=0
  ```

  A principal holding `corpus:read` is admitted on every one of the sixteen groups with
  `allowed` delta = 1 and `would_deny` delta = 0 per group, and no `corpus_grant_would_deny`
  warn line is emitted for them — the assertion that stops a gate from recording granted
  principals as counterfactual denials. `_ScopeRejectedCounterIsNotReused` is the paired
  R-108-O2 assertion in the same lane: `smackerel_auth_scope_rejected_total` stays flat across
  a full sixteen-group OBSERVE sweep.

  **What this does NOT establish:** the mode gauge is asserted by writing ENFORCE (expect 1)
  then OBSERVE (expect 0) through `metrics.SetCorpusGrantEnforcementMode` and reading back
  from `prometheus.DefaultGatherer`. That proves the metric's contract and that the 0 came
  from an explicit write rather than an unset zero value; it does **not** prove `cmd/core`
  publishes the stage at startup.

- [x] `TP-02-07` — this scope's observe middleware never denies in **either** stage; no 403 originates from `corpusGate.Observe`
  - **Command:** `./smackerel.sh test unit --go --go-run 'TestCorpusGrantGate_Observe_NeverDeniesInEitherStage'`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestCorpusGrantGate_Observe_NeverDeniesInEitherStage (0.00s)`
    with `--- PASS: .../observe_stage` and `--- PASS: .../enforce_stage`. The enforce-stage
    subtest is the load-bearing one: under ENFORCE only `RequireScope` may deny, so a
    regression that made the observe middleware itself return 403 would be caught here.

  **BOUNDARY SPLIT (resolves the open Scope 02/03 finding).** The former wording of this
  item was "Observe middleware is mounted in both stages; no request path can return 403
  from this scope" — two properties owned by two different scopes. It is now SPLIT along
  the real boundary rather than moved wholesale, because each half is genuinely the
  deliverable of a different scope:

  - **Scope 02 ("Observe-Stage Plumbing") retains the never-denies half**, above. Building
    a middleware that never denies IS this scope's deliverable, and it is asserted by a
    test living in this scope's own gate file.
  - **Scope 03 ("Gate Mount") owns the mounted-on-all-sixteen half**, which it already
    asserts and has already evidenced: `r.Use(auth.RequireScope(...)) mounted on the corpus
    route group` and `All sixteen route groups from spec.md §4.2 ... sit inside the gated
    group` are both checked there, backed by TP-03-02 and the route-manifest test.

  Splitting creates no orphan: the half leaving Scope 02 lands in a scope that already
  carries both a Test Plan row and checked DoD items for it, and the half staying gains
  TP-02-07 so it is no longer a DoD item without Test Plan coverage. Neither assertion is
  weakened — this was a question of WHICH SCOPE OWNS the property, not whether it holds.

  Tier recorded honestly: TP-02-07 is **unit**, not integration. `internal/api` is not in
  the integration lane (`scripts/runtime/go-integration.sh` runs `./tests/integration/...`,
  `./internal/notification/...`, `./internal/assistant/...`, `./internal/cardrewards/...`,
  `./tests/eval/...`), so labelling it integration would misstate where it executes.


- [x] `TP-02-06` regression e2e-api test passes — fail-loud config resolution and OBSERVE-stage counting are permanently protected
  - **Command:** `./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Evidence:** `E2E_EXIT=0`, `--- FAIL` count `0`, 417 `--- PASS`, shell `Passed: 36 / Failed: 0`.
    The prior note gave two blockers: the suite was not executed, and the lane was red on
    five unrelated pre-existing defects. Both are cleared — the five were fixed in this
    session (`49dc5b29`, `7e6b4d72`) and the suite has now been executed green, so the
    TP-02-06 regression ran inside a genuinely green lane rather than being excused by it.

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-02-06`, `./smackerel.sh test e2e`)
  - **Command:** `./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Evidence:** `E2E_EXIT=0`, `--- FAIL` count `0`, 417 `--- PASS`. The dependency this
    row named (TP-02-06) is now executed green, and the earlier "gated behind the unmounted
    gate" blocker had already lapsed once Scope 03 mounted the middleware at
    `internal/api/router.go:132`.

- [x] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
  - **Command:** `./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Evidence:** `E2E_EXIT=0`, `--- FAIL` count `0`, 417 `--- PASS`, shell tier
    `Total: 36 / Passed: 36 / Failed: 0`. The five pre-existing defects that made this
    unattainable were fixed in this session, so the suite exits 0 on its own merits.

- [x] Live-category tests emit telemetry tagged `env=test*` only; no write to prod monitoring (R-108-O6, G115)
  - **Command:** `bash .github/bubbles/scripts/env-pollution-scan.sh "$(pwd)"`
  - **Exit Code:** 0
  - **Evidence:** `[env-pollution-scan] env-pollution-scan PASSED (no test-to-prod-surface
    writes detected)`. Stated precisely, because the two halves are proven differently.
    "No write to prod monitoring" is proven structurally, not merely unobserved: the
    corpus-grant metrics register on the pull-based default registry
    (`internal/metrics/auth.go:423` `prometheus.MustRegister`), and a repo-wide scan for
    `pushgateway|push.New|remote_write` across `tests/` and `internal/` returns EMPTY —
    so a test run has no outbound path to any monitoring system, and the live-category run
    executed entirely against the ephemeral `smackerel-test` compose project whose volumes
    are removed at teardown. The `env=test*` half is discharged by that same absence
    rather than by inspecting a label: nothing is exported, so there is no `env`-labelled
    series that could carry a prod tag. This is NOT a claim that `env=test*` labels were
    observed — none exist to observe.

- [x] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh check` · `./smackerel.sh lint` · `./smackerel.sh format --check`
  **Exit Code:** 0 · 0 · 0

  ```text
  $ ./smackerel.sh check
  config-validate: <repo-root>/config/generated/dev.env.tmp.4007465 OK
  Config is in sync with SST
  env_file drift guard: OK
  scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
  scenarios registered: 17, rejected: 0
  scenario-lint: OK
  CHECK_EXIT=0

  $ ./smackerel.sh lint
  All checks passed!
  === Validating web manifests ===
    OK: web/pwa/manifest.json
    OK: Chrome extension manifest has required fields (MV3)
    OK: Firefox extension manifest has required fields (MV2 + gecko)
  === Validating JS syntax ===
    OK: web/pwa/app.js
    OK: web/extension/background.js
  === Checking extension version consistency ===
    OK: Extension versions match (1.0.0)
  Web validation passed
  LINT_EXIT=0

  $ ./smackerel.sh format --check
  78 files already formatted
  FORMAT_EXIT=0

  $ grep -nE 'TODO|FIXME|HACK|STUB|unimplemented|panic\("not implemented' cmd/core/wiring_corpus_grant.go internal/api/corpus_grant_gate.go internal/metrics/auth.go
  TODO_SCAN_EXIT=1 (1 = none found)
  ```

  All three legs exit 0 with no warning lines. `format --check` runs both `go-format.sh --check`
  and `python-format.sh --check`; the Go leg emitted no diff output (gofmt prints nothing when
  every file is formatted) and the Python leg reported 78 files already formatted. The
  TODO/stub scan over the three non-test files this scope adds or modifies returned exit 1
  (no match). The "no default" half is carried by the first DoD item above.

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
| TP-03-04 | unit | `internal/api/router_corpus_gate_test.go` | **ADVERSARIAL (design T8):** real router + ENFORCE + empty-scope fixture principal → 403 on all **sixteen** groups, AND set-equality of the canonical sixteen against the router's mounted corpus group. Fails against current `main`; fails if the `RequireScope` mount is deleted, a route leaves the group, the stage defaults to OBSERVE, or a seventeenth corpus route is registered ungated (SCN-108-G03). **Tier corrected from `integration` to `unit`:** the assertion runs an in-process `httptest` router with no live stack, and `internal/api` is not in the integration lane (`go-integration.sh` covers `./tests/integration/...`, `./internal/notification/...`, `./internal/assistant/...`, `./internal/cardrewards/...`, `./tests/eval/...`). Labelling an in-process test `integration` is a Test Type Integrity violation; the live-stack half of this behaviour is covered by TP-03-06/TP-03-07 | `./smackerel.sh test unit` |
| TP-03-05 | integration | `internal/api` + `cmd/core` restart harness | Stage flip is symmetric and idempotent: ENFORCE → OBSERVE restores 200 for previously denied principals and resumes counting, with no rebuild invoked (SCN-108-C04) | `./smackerel.sh test integration` |
| TP-03-06 | e2e-api | `./smackerel.sh test e2e` | Full stack, real Postgres: a granted operator token reads `/api/search` and `/api/export`; an ungranted daily-user token is refused on both; the refusal body contains no artifact id, title, or count (SCN-108-G01, design T6) | `./smackerel.sh test e2e` |
| TP-03-07 | e2e-api | same | Denial parity: a denied `/api/artifact/{id}` for a real id and for a random id produce byte-identical responses — no existence oracle (SCN-108-D01, design T7) | `./smackerel.sh test e2e` |
| TP-03-08 | integration | `internal/api` router-bootstrap canary | **Canary:** narrow, independently-runnable canary over the shared router bootstrap and shared contract-test harness — asserts `bearerAuthMiddleware` still runs before the gate (middleware **ordering** contract), that a granted session still resolves (**session** contract), and that the ungated routes from design.md §2 are still reachable. Run **before** any broad suite rerun so a broken shared harness is caught in seconds instead of masquerading as a repo-wide failure | `./smackerel.sh test integration` |
| TP-03-09 | stress | `internal/api` gated-route stress harness | The gate sits on the hot read path of all sixteen corpus route groups, so it is SLA-sensitive. Under sustained concurrent load on `/api/search` and `/api/artifact/{id}`: added per-request latency from `RequireScope` + `Observe` stays within the documented budget, p99 does not regress against the ungated baseline, and no allocation or lock-contention regression appears in the middleware chain | `./smackerel.sh test stress` |
| TP-03-10 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-G01, SCN-108-D01, SCN-108-G02, SCN-108-G03, SCN-108-G04, SCN-108-G05 and SCN-108-C04 against the live stack: ENFORCE still denies ungranted principals on all **sixteen** groups, denials stay byte-identical, documented bypasses still pass, Tier B stays gated, and ENFORCE→OBSERVE rollback still restores access. Fails if the gate is unmounted, a corpus route escapes the gated group, or the stage machine regains a silent default; also proves the broader e2e suite shows no green→red drift | `./smackerel.sh test e2e` |
| TP-03-11 | integration | `internal/api` against the ephemeral test stack | **Tier B (§18 decision 5):** with a non-nil intelligence engine and ENFORCE, an ungranted principal receives **403** on each of the eight Phase-5 route groups and no derived corpus signal is returned; a principal holding `corpus:read` receives 200 from the same eight; the Tier B denial body is the same shape as a Tier A denial (SCN-108-G04) | `./smackerel.sh test integration` |
| TP-03-12 | unit | `internal/api/router_corpus_gate_test.go` | **ADVERSARIAL (conditional-registration hazard):** the T8 set-equality assertion is evaluated against a router built with a **non-nil** `deps.IntelligenceEngine`, so all sixteen groups are actually registered. Fails if the Tier B block is registered outside the gated group, and fails if a nil engine is substituted to make the sixteen-value set-equality trivially satisfiable — the vacuous-pass path that would let eight corpus-derived routes ship ungated (SCN-108-G05). **Tier corrected from `integration` to `unit`** for the same execution-reality reason as TP-03-04 | `./smackerel.sh test unit` |

### Definition of Done

- [x] `r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))` mounted on the corpus route group in `internal/api/router.go`, inside `bearerAuthMiddleware` and outside the individual route registrations; mounted only in ENFORCE

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `grep -nE 'bearerAuthMiddleware|CorpusGrantGate|RequireScope\(auth\.GrantGlobalCorpusRead\)|r\.Group\(' internal/api/router.go` plus `awk` excerpts of the two group headers
  **Exit Code:** 0

  ```text
  $ grep -nE 'bearerAuthMiddleware|RequireScope\(auth\.GrantGlobalCorpusRead\)|r\.Group\(' internal/api/router.go
  86:             r.Group(func(r chi.Router) {
  87:                     r.Use(deps.bearerAuthMiddleware)
  131:                    r.Group(func(r chi.Router) {
  134:                                    r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))

  $ awk 'NR>=84 && NR<=88 {printf "%d|%s\n", NR, $0}' internal/api/router.go
  85|             // Authenticated API routes
  86|             r.Group(func(r chi.Router) {
  87|                     r.Use(deps.bearerAuthMiddleware)
  88|                     r.Post("/capture", deps.CaptureHandler)

  $ awk 'NR>=131 && NR<=137 {printf "%d|%s\n", NR, $0}' internal/api/router.go
  131|                    r.Group(func(r chi.Router) {
  132|                            corpusGate := NewCorpusGrantGate(deps.CorpusGrantEnforce)
  133|                            if deps.CorpusGrantEnforce {
  134|                                    r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))
  135|                            }
  136|
  137|                            // Tier A — raw corpus retrieval (groups 1-8).
  ```

  The gated group (L131) opens **inside** the authenticated group that mounts `bearerAuthMiddleware`
  (L86–87), so the session the gate reads is already populated. `RequireScope` is a single
  group-level `r.Use` at L134, not a per-route decoration, and it sits inside
  `if deps.CorpusGrantEnforce` — so it is mounted in ENFORCE only. `Observe` is applied per route
  via `r.With(...)` in both stages, which is why the two halves are not interchangeable.

  **What this does NOT establish:** this is a *static structural* proof of the mount. It does not
  prove a live 403 against a running stack. That live proof now exists for TP-03-02, TP-03-05, and
  TP-03-11 via `tests/integration/graphapi/corpus_enforce_test.go`. TP-03-03, TP-03-06, and
  TP-03-07 remain unexecuted — TP-03-03 has no test yet, and the two e2e rows are blocked on the
  pre-existing e2e-red baseline described under TP-03-06.

- [x] All **sixteen** route groups from `spec.md` §4.2 (Tier A 1–8 + Tier B 9–16) sit inside the gated group; every still-ungated route from design.md §2 remains ungated and unchanged

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh test unit --go --go-run 'CorpusGate' --verbose`
  **Exit Code:** 0

  ```text
  [go-unit] applying -run selector: CorpusGate
  + go test -v -run CorpusGate -count=1 ./...
  === RUN   TestCorpusGate_AllSixteenRouteGroupsGated
  === RUN   TestCorpusGate_AllSixteenRouteGroupsGated/expectation_covers_the_closed_sixteen_value_label_set
  === RUN   TestCorpusGate_AllSixteenRouteGroupsGated/every_expected_corpus_route_is_gated
  === RUN   TestCorpusGate_AllSixteenRouteGroupsGated/no_unexpected_route_is_gated
  === RUN   TestCorpusGate_AllSixteenRouteGroupsGated/gated_group_count_is_exactly_sixteen
  --- PASS: TestCorpusGate_AllSixteenRouteGroupsGated (0.02s)
      --- PASS: TestCorpusGate_AllSixteenRouteGroupsGated/expectation_covers_the_closed_sixteen_value_label_set (0.00s)
      --- PASS: TestCorpusGate_AllSixteenRouteGroupsGated/every_expected_corpus_route_is_gated (0.00s)
      --- PASS: TestCorpusGate_AllSixteenRouteGroupsGated/no_unexpected_route_is_gated (0.00s)
      --- PASS: TestCorpusGate_AllSixteenRouteGroupsGated/gated_group_count_is_exactly_sixteen (0.00s)
  === RUN   TestCorpusGate_DoesNotOverReachUngatedRoutes
  === RUN   TestCorpusGate_DoesNotOverReachUngatedRoutes/observe
  === RUN   TestCorpusGate_DoesNotOverReachUngatedRoutes/enforce
  --- PASS: TestCorpusGate_DoesNotOverReachUngatedRoutes (0.01s)
      --- PASS: TestCorpusGate_DoesNotOverReachUngatedRoutes/observe (0.01s)
      --- PASS: TestCorpusGate_DoesNotOverReachUngatedRoutes/enforce (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/api     0.309s
  GATEONLY_EXIT=0
  ```

  Set-equality is asserted in both directions: `every_expected_corpus_route_is_gated` catches a
  corpus route that escaped the group, `no_unexpected_route_is_gated` catches over-reach, and
  `gated_group_count_is_exactly_sixteen` catches a seventeenth. `DoesNotOverReachUngatedRoutes`
  exercises the design.md §2 ungated list in **both** stages, so an ungated route is proven not
  corpus-denied under ENFORCE either.

  **What this does NOT establish:** the assertion is over the route **manifest** of a router built
  in-process. It is not a live end-to-end denial against a running stack, and it does not
  independently prove the ungated handler bodies are byte-unchanged.
- [x] The Tier B block's `if deps.IntelligenceEngine != nil` conditional sits **inside** the gated group, so a non-nil engine cannot register the eight Phase-5 routes outside the gate

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `awk` excerpt of `internal/api/router.go` L183–190 and L203–208, plus the vacuity-trap test from the `CorpusGate` run
  **Exit Code:** 0

  ```text
  $ awk 'NR>=183 && NR<=190 {printf "%d|%s\n", NR, $0}' internal/api/router.go
  183|                            // engine is configured — that is, whenever these endpoints
  184|                            // actually serve corpus-derived signal.
  185|                            if deps.IntelligenceEngine != nil {
  186|                                    r.With(corpusGate.Observe(metrics.CorpusRouteGroupExpertise)).
  187|                                            Get("/expertise", ExpertiseHandler(deps.IntelligenceEngine))
  188|                                    r.With(corpusGate.Observe(metrics.CorpusRouteGroupLearningPaths)).
  189|                                            Get("/learning-paths", LearningPathsHandler(deps.IntelligenceEngine))
  190|                                    r.With(corpusGate.Observe(metrics.CorpusRouteGroupSubscriptions)).

  $ awk 'NR>=203 && NR<=208 {printf "%d|%s\n", NR, $0}' internal/api/router.go
  203|                    })
  204|                    // ── end corpus-grant gate ─────────────────────────────────
  206|                    // Bookmark import endpoint (Phase 2)
  207|                    r.Post("/bookmarks/import", deps.BookmarkImportHandler)

  === RUN   TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap
  --- PASS: TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap (0.04s)
  ```

  The conditional opens at L185 and the gated group does not close until L203, so the eight Tier B
  registrations are lexically enclosed — a non-nil engine cannot register them outside the gate.
  `TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap` closes the complementary hazard: it makes
  the nil-engine substitution an explicit failure rather than a silently-passing sixteen-value
  set-equality over a router that only ever mounted eight.

- [x] No per-handler `if !authorized` check added to any corpus handler, Tier A or Tier B

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `grep -rnE 'if[[:space:]]+!authorized|if[[:space:]]*!.*Authorize|AuthorizeCorpusRead\(|AuthorizeGrant\(' --include='*.go' internal/api cmd | grep -v '_test.go'`
  **Exit Code:** 0

  ```text
  $ grep -rnE 'if[[:space:]]+!authorized|if[[:space:]]*!.*Authorize|AuthorizeCorpusRead\(|AuthorizeGrant\(' \
      --include='*.go' internal/api cmd | grep -v '_test.go'
  internal/api/router.go:107:                     // sixteen handler bodies. A per-handler `if !authorized` check
  SCAN_RC=0

  $ grep -rn 'GrantGlobalCorpusRead' --include='*.go' . | grep -v '_test.go'
  ./internal/auth/browser_session_policy.go:37:   // GrantGlobalCorpusRead authorizes reading the single operator-owned global
  ./internal/auth/browser_session_policy.go:40:   GrantGlobalCorpusRead = "corpus:read"
  ./internal/auth/browser_session_policy.go:62:   GrantGlobalCorpusRead,
  ./internal/auth/browser_session_policy.go:132:// explicit GrantGlobalCorpusRead grant. The operator and a specifically-granted
  ./internal/auth/browser_session_policy.go:140:  return CorpusDecision{Allowed: slices.Contains(sess.Scopes, GrantGlobalCorpusRead)}
  ./internal/api/corpus_grant_gate.go:16:// (`auth.RequireScope(auth.GrantGlobalCorpusRead)`) and the mounting of both
  ./internal/api/corpus_grant_gate.go:131:                "required_grant", auth.GrantGlobalCorpusRead,
  ./internal/api/router.go:119:                   //   3. auth.RequireScope(auth.GrantGlobalCorpusRead) — the
  ./internal/api/router.go:134:                                   r.Use(auth.RequireScope(auth.GrantGlobalCorpusRead))
  ```

  The only hit for the per-handler shape across all non-test Go under `internal/api` and `cmd` is
  the **prohibiting comment** at `router.go:107` — no handler body performs its own authorization.
  The second grep corroborates the same conclusion from the other direction: the grant constant has
  exactly one enforcement referent in non-test code, the group-level `r.Use` at `router.go:134`
  (the `corpus_grant_gate.go` hits are the Observe half's denial-reason label and a doc comment).

- [x] `TP-03-01` unit test passes — `GateGlobalCorpusRead` denies empty and wildcard claims, allows explicit, leaks nothing

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh test unit --go --go-run 'Corpus' --verbose`
  **Exit Code:** 0

  ```text
  [go-unit] applying -run selector: Corpus
  + go test -v -run Corpus -count=1 ./...
  === RUN   TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant
  --- PASS: TestRegisteredScopeSurfaces_ContainsCorpusMappedToGrant (0.00s)
  === RUN   TestCorpusReadScopeClaimValidatesAndAuthorizes
  --- PASS: TestCorpusReadScopeClaimValidatesAndAuthorizes (0.00s)
  === RUN   TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants
  --- PASS: TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants (0.00s)
  === RUN   TestRegisteredScopeSurfaces_ContainsCorpus
  --- PASS: TestRegisteredScopeSurfaces_ContainsCorpus (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/auth    0.015s
  CORPUS_ALL_EXIT=0

  $ awk 'NR>=126 && NR<=141 {printf "%d|%s\n", NR, $0}' internal/auth/browser_session_policy.go
  127|type CorpusDecision struct {
  128|    Allowed bool
  129|}
  136|func GateGlobalCorpusRead(sess Session) CorpusDecision {
  137|    if slices.Contains(sess.Scopes, wildcardGrant) {
  138|            return CorpusDecision{Allowed: false}
  139|    }
  140|    return CorpusDecision{Allowed: slices.Contains(sess.Scopes, GrantGlobalCorpusRead)}
  141|}
  ```

  `TestCorpusReadScopeClaimValidatesAndAuthorizes` covers all four TP-03-01 arms — granted daily
  user allowed, ungranted daily user denied, bare session denied, `"*"` wildcard denied. The
  "leaks nothing" half is carried structurally: `CorpusDecision` has exactly one field, `Allowed
  bool`, so there is no content, count, or domain label for a decision to carry.

  **Selector note (honest):** the earlier `--go-run 'CorpusGate|CorpusGrant'` pass did **not**
  match these test names, so TP-03-01 was re-run under the broader `Corpus` selector specifically
  to evidence this item rather than inferred from the narrower run.
- [x] `TP-03-02` integration test passes — ENFORCE returns 403 on all sixteen groups

  Closed by `tests/integration/graphapi/corpus_enforce_test.go`
  (`TestIntegration_CorpusGrantEnforce_RefusesUngrantedAndServesGrantedOnAllSixteenGroups`).

  Command: `./smackerel.sh test integration --go-run 'CorpusGrantEnforce'`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `31c3c09a` plus this untracked test file

  ```text
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted token_scopes=[annotation:edit] endpoint=/api/search
  INFO request method=POST path=/api/search status=403 request_id=6cdcf4fba375/Mxs9XStFns-000001
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted token_scopes=[annotation:edit] endpoint=/api/digest
  INFO request method=GET path=/api/digest status=403 request_id=6cdcf4fba375/Mxs9XStFns-000004
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted token_scopes=[annotation:edit] endpoint=/api/recent
  INFO request method=GET path=/api/recent status=403 request_id=6cdcf4fba375/Mxs9XStFns-000007
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/artifact/tp0302-canary-artifact-identifier
  INFO request method=GET path=/api/artifact/tp0302-canary-artifact-identifier status=403 request_id=6cdcf4fba375/Mxs9XStFns-000010
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/export
  INFO request method=GET path=/api/export status=403 request_id=6cdcf4fba375/Mxs9XStFns-000016
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/context-for
  INFO request method=POST path=/api/context-for status=403 request_id=6cdcf4fba375/Mxs9XStFns-000019
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/knowledge/concepts
  INFO request method=GET path=/api/knowledge/concepts status=403 request_id=6cdcf4fba375/Mxs9XStFns-000022
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/knowledge/entities
  INFO request method=GET path=/api/knowledge/entities status=403 request_id=6cdcf4fba375/Mxs9XStFns-000028
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/knowledge/lint
  INFO request method=GET path=/api/knowledge/lint status=403 request_id=6cdcf4fba375/Mxs9XStFns-000034
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0302-ungranted endpoint=/api/knowledge/stats
  INFO request method=GET path=/api/knowledge/stats status=403 request_id=6cdcf4fba375/Mxs9XStFns-000037
  --- PASS: TestIntegration_CorpusGrantEnforce_RefusesUngrantedAndServesGrantedOnAllSixteenGroups (0.08s)
  ok      github.com/smackerel/smackerel/tests/integration/graphapi       0.328s
  ```

  **Why this row needed a new test.** The pre-existing unit test
  `TestCorpusGate_AllSixteenRouteGroupsGated` asserts the route *manifest* — that the middleware is
  mounted — which is not the same statement as "a request is refused". Every line above is a real
  request through the real `api.NewRouter` with `CorpusGrantEnforce: true`, and the distinct
  `request_id` values show sixteen separate requests rather than one assertion repeated.

  **Non-vacuity.** Both arms are asserted on every group in one sweep — ungranted refused **and** a
  `corpus:read` holder served. A negative-only test passes when the whole stack is broken; a
  positive-only test passes when the gate is absent. Only the pair distinguishes those.
  `exercisedGroups != 16` (source line 539) plus two list cross-checks against `corpusGroupRoutes`
  (lines 245, 267) stop the loop silently shrinking.

  **Scope note (honest).** This row covers the sixteen-group 403/200 matrix. The
  `smackerel_auth_scope_rejected_total` increment and the mode-gauge `1` reading named in the same
  TP-03-02 test-plan cell are covered by the Scope 02 metric rows, not re-asserted here.

  A companion test in the same file,
  `TestIntegration_CorpusGrantEnforce_DoesNotOverReachIntoUngatedRoutes`, asserts the deliberately
  ungated routes stay reachable under ENFORCE. Gate over-reach is as much a defect as gate absence,
  and a test that only proves denial cannot tell the two apart.

- [ ] `TP-03-03` integration test passes — shared-token and bootstrap bypass asserted under ENFORCE

  **Not executed in this packet** — same in-flight-integration-run reason as TP-03-02. Unclaimed.

- [x] `TP-03-04` adversarial route-manifest test passes AND is demonstrated to **fail against current `main`** (empty-scope principal is allowed today); set-equality catches a seventeenth ungated corpus route
  - **Command:** `./smackerel.sh test unit --go --go-run 'TestCorpusGate_AllSixteenRouteGroupsGated'`
  - **Exit Code:** 1 with the mount removed (RED), 0 with it restored (GREEN)
  - **Evidence:** the two halves were executed SEPARATELY, because the passing run alone is
    not the demonstration this row asks for. RED probe — the `RequireScope` mount was
    temporarily disabled in `internal/api/router.go` and the test reported
    `RED_PROBE_EXIT=1`, `--- FAIL: TestCorpusGate_AllSixteenRouteGroupsGated`, with
    subtests `every_expected_corpus_route_is_gated` and `gated_group_count_is_exactly_sixteen`
    FAILING and **21** `UNGATED corpus route` lines (e.g. `GET /api/knowledge/stats`,
    `POST /api/search`) proving an empty-scope principal reaches the corpus without the
    gate. The probe was then reverted (`git diff --stat internal/api/router.go` empty) and
    the same test returned `GREEN_EXIT=0` with all four subtests PASS.

  **Tier corrected.** This row was planned as `integration` in
  `auth_surface_contract_test.go`; it is recorded as `unit` in
  `router_corpus_gate_test.go` because that is where it executes — an in-process `httptest`
  router, no live stack, and `internal/api` is not in the integration lane. Claiming
  `integration` for an in-process test is a Test Type Integrity violation. The live-stack
  half of this behaviour is TP-03-06/TP-03-07's job, and those remain open.

- [x] `TP-03-05` integration test passes — ENFORCE→OBSERVE rollback restores access with no rebuild

  Closed by `TestIntegration_CorpusGrantEnforce_RollbackToObserveRestoresAccess` in the same file.
  Both stages are constructed in **one** test process, which is what makes "no rebuild" an
  observation rather than an assertion — the binary is never re-linked between the two halves.

  The log below is the load-bearing part. The same principal `tp0305-rollback-ungranted` is refused
  under ENFORCE, then under OBSERVE the `corpus_grant_would_deny` counter fires with
  `enforcement_mode=observe` while the request proceeds. That pair is the stage flip: the gate still
  *evaluates* and still *counts*, it just stops refusing. A rollback that silently stopped counting
  would look identical from the response alone, so the counter line is the distinguishing evidence.

  Command: `./smackerel.sh test integration --go-run 'CorpusGrantEnforce'`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `31c3c09a` plus this untracked test file

  ```text
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0305-rollback-ungranted token_scopes=[annotation:edit] endpoint=/api/search
  WARN auth: corpus_grant_would_deny route_group=search user_id=tp0305-rollback-ungranted session_source=per_user_token required_grant=corpus:read enforcement_mode=observe
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0305-rollback-ungranted token_scopes=[annotation:edit] endpoint=/api/digest
  WARN auth: corpus_grant_would_deny route_group=digest user_id=tp0305-rollback-ungranted session_source=per_user_token required_grant=corpus:read enforcement_mode=observe
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0305-rollback-ungranted token_scopes=[annotation:edit] endpoint=/api/recent
  WARN auth: corpus_grant_would_deny route_group=recent user_id=tp0305-rollback-ungranted session_source=per_user_token required_grant=corpus:read enforcement_mode=observe
  --- PASS: TestIntegration_CorpusGrantEnforce_RollbackToObserveRestoresAccess (0.05s)
  ok      github.com/smackerel/smackerel/tests/integration/graphapi       0.328s
  ```

  Non-vacuity: the test counts all three quantities and requires each to be sixteen —
  `refusedUnderEnforce != 16 || servedUnderObserve != 16 || countingResumed != 16` (source line 769).
  Counting only the served side would pass against a build where ENFORCE never denied anything.

- [ ] `TP-03-06` e2e-api test passes — granted reads succeed, ungranted refused, body carries no id/title/count

  **Not executed in this packet.** `./smackerel.sh test e2e` is currently red on five unrelated
  pre-existing defects (spec 106 asset headers; three BUG-069-004 assistant-dedup failures; the
  drive-harness env-propagation defect; BUG-031-004). Unclaimed.

- [ ] `TP-03-07` e2e-api test passes — denial byte-parity between real and random id

  **Not executed in this packet** — same pre-existing e2e-red reason as TP-03-06. Unclaimed.

- [ ] `TP-03-08` canary integration test passes — shared router-bootstrap ordering, session, and ungated-route contracts intact

  **Not executed in this packet** — same in-flight-integration-run reason as TP-03-02. Unclaimed.

- [ ] `TP-03-09` stress test passes — gate adds no p99 latency regression on the corpus route groups under sustained load

  **Not executed in this packet.** No stress run was performed, so no latency claim is made in
  either direction. Unclaimed.

- [ ] `TP-03-10` regression e2e-api test passes — enforcement across both tiers, denial parity, documented bypasses, and rollback are permanently protected

  **Not executed in this packet** — same pre-existing e2e-red reason as TP-03-06. Unclaimed.

- [x] `TP-03-11` integration test passes — all eight Tier B Phase-5 route groups deny an ungranted principal with 403 and the Tier A denial shape, and allow a `corpus:read` holder (§18 decision 5)

  Closed by `TestIntegration_CorpusGrantEnforce_TierBDeniesWithTheTierADenialShape`. The router for
  this test is built with a **non-nil** `deps.IntelligenceEngine`. Without it the eight Phase-5
  routes never register, every Tier B assertion passes against a router that only has eight groups,
  and the row would be vacuously green — the precise hazard
  `TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap` names at the unit tier. The failure message
  at source line 650 spells this out so a future regression reports the real cause.

  Denial-shape parity matters beyond tidiness: if Tier B denied with a different body or status than
  Tier A, the difference would itself disclose which tier a route belongs to, leaking corpus
  structure to an unauthorized caller. The test asserts the shapes are identical rather than merely
  that both are 403.

  Command: `./smackerel.sh test integration --go-run 'CorpusGrantEnforce'`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `31c3c09a` plus this untracked test file

  ```text
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0311-ungranted token_scopes=[annotation:edit] endpoint=/api/recent
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0311-ungranted token_scopes=[annotation:edit] endpoint=/api/search
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0311-ungranted token_scopes=[annotation:edit] endpoint=/api/expertise
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0311-ungranted token_scopes=[annotation:edit] endpoint=/api/learning-paths
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0311-ungranted token_scopes=[annotation:edit] endpoint=/api/subscriptions
  WARN auth: scope_rejected required_scope=corpus:read user_id=tp0311-ungranted token_scopes=[annotation:edit] endpoint=/api/serendipity
  --- PASS: TestIntegration_CorpusGrantEnforce_TierBDeniesWithTheTierADenialShape (0.03s)
  ok      github.com/smackerel/smackerel/tests/integration/graphapi       0.328s
  ```

  `/api/expertise`, `/api/learning-paths`, `/api/subscriptions`, and `/api/serendipity` above are
  Phase-5 intelligence routes, so their presence in the log is direct evidence the engine was
  non-nil and the Tier B block was genuinely registered and genuinely refused.

- [x] `TP-03-12` adversarial integration test passes — set-equality is evaluated against a non-nil intelligence engine and fails on the nil-engine vacuous-pass path and on Tier B registered outside the gate
  - **Command:** `./smackerel.sh test unit --go --go-run 'TestCorpusGate_AllSixteenRouteGroupsGated|TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap'`
  - **Exit Code:** 0
  - **Evidence:** `--- PASS: TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap (0.01s)` and
    `--- PASS: TestCorpusGate_AllSixteenRouteGroupsGated (0.03s)` with subtest
    `expectation_covers_the_closed_sixteen_value_label_set` PASS. The set-equality run uses
    `corpusGateDeps(t, true, true)` — the second flag builds a NON-NIL
    `deps.IntelligenceEngine`, so all sixteen groups genuinely register and the assertion
    cannot pass vacuously over a Tier-A-only router. The vacuity trap is asserted to be
    REAL by its own test: with a nil engine, fewer than sixteen groups are gated and
    precisely the Tier B eight go missing.

  **Tier corrected** from `integration` to `unit` for the same execution-reality reason as
  TP-03-04: this is an in-process router assertion, not a live-stack one.
- [ ] **DoD-03-TIERB-DESIGN:** `design.md` §2's route-inventory table and §8 T2/T4/T8 count language reconciled to the ratified sixteen-group surface by `bubbles.design` before this scope closes — not silently overwritten from this planning packet (routed per the `spec.md` §18 decision 5 planning note)

  **Not satisfied.** `design.md` is unmodified in this working tree (`git status --porcelain
  specs/108-corpus-grant-enforcement/design.md` → empty), its §2 row still defers the sixteen-row
  extension to `bubbles.design` (L149), and §8 T4 still reads "all eight route groups" (L311). This
  is a foreign artifact owned by `bubbles.design`; it is routed, not edited from here.

- [ ] Consumer Impact Sweep completed for the corpus route-group contract change across the PWA, Chrome extension bridge, Telegram bridge, Tier B consumers, external API clients, and docs: zero stale first-party references remain, and the Tier B "zero first-party in-repo callers" negative result is re-verified rather than inherited

  **Not performed in this packet.** No caller enumeration or stale-reference scan was executed, and
  the Tier B "zero first-party in-repo callers" negative result was explicitly not re-verified.

- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns

  **Not executed in this packet** — TP-03-08 is the canary and is an integration-category row; see TP-03-08.

- [ ] Rollback or restore path for shared infrastructure changes is documented and verified

  **Not verified in this packet.** The ENFORCE→OBSERVE rollback path is TP-03-05, which was not executed.

- [ ] Change Boundary is respected and zero excluded file families were changed

  **NOT satisfied — deviation found and recorded rather than waived.**

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `git status --porcelain` plus `git --no-pager diff -U2 cmd/core/wiring.go cmd/core/main.go`
  **Exit Code:** 0

  ```text
  $ git status --porcelain | grep -E 'cmd/core|internal/api'
   M cmd/core/main.go
   M cmd/core/wiring.go
   M internal/api/auth_surface_contract_test.go
   M internal/api/health.go
   M internal/api/router.go

  $ git --no-pager diff -U2 cmd/core/wiring.go
  +// Spec 108 Scope 03 added corpusGrantEnforce — the stage already resolved
  +// fail-loud by resolveCorpusGrantEnforcement in run(). It is a REQUIRED
  +// parameter rather than an optional field assignment so that omitting it is a
  +// compile error here, not a silent ENFORCE→OBSERVE downgrade at runtime.
  -func buildAPIDeps(ctx context.Context, cfg *config.Config, svc *coreServices) (...)
  +func buildAPIDeps(ctx context.Context, cfg *config.Config, svc *coreServices, corpusGrantEnforce bool) (...)
  +               CorpusGrantEnforce: corpusGrantEnforce,

  $ git --no-pager diff -U2 cmd/core/main.go
  -       deps, listResolver, listStore, err := buildAPIDeps(ctx, cfg, svc)
  +       deps, listResolver, listStore, err := buildAPIDeps(ctx, cfg, svc, corpusGrantEnforce)
  ```

  **F-108-S03-01 (Change Boundary deviation).** This scope's Change Boundary lists `cmd/core` under
  *Excluded surfaces* ("owned by Scopes 02 and 05") and its *Allowed file families* name only
  `internal/api/router.go`, `internal/api/auth_surface_contract_test.go`, and new/extended test
  files under `internal/api` and `internal/auth`. Two non-test surfaces outside that list changed:

  1. `cmd/core/wiring.go` — the `buildAPIDeps` signature gained a required `corpusGrantEnforce bool`
     and assigns `CorpusGrantEnforce`. The added comment self-attributes to **Scope 03**.
  2. `internal/api/health.go` — the `Dependencies` struct gained `CorpusGrantEnforce bool` (L190);
     `health.go` is a non-test file not named in the Allowed list.

  (`cmd/core/main.go`'s stage-resolution block self-attributes to Scope 02; only its one-line
  `buildAPIDeps` call-site update is a Scope 03 consequence.)

  The changes look *substantively* right — a required parameter makes omission a compile error
  instead of a silent ENFORCE→OBSERVE downgrade, which is the safer construction. The defect is
  boundary conformance, not code quality. Resolution is a planning decision (widen Scope 03's
  Allowed families, or reassign these surfaces to Scope 02), which is owned by `bubbles.plan`; it
  is therefore **routed, not self-approved here**, and this item stays unchecked.

- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-03-10`, `./smackerel.sh test e2e`)

  **Not executed in this packet** — see TP-03-10 (e2e suite red on five unrelated pre-existing defects).

- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)

  **Not executed in this packet.** `./smackerel.sh test e2e` is currently red on five unrelated
  pre-existing defects, so no green→red drift claim is made in either direction.

- [x] Live-category tests emit telemetry tagged `env=test*` only; no write to prod monitoring (R-108-O6, G115)
  - **Command:** `bash .github/bubbles/scripts/env-pollution-scan.sh "$(pwd)"`
  - **Exit Code:** 0
  - **Evidence:** `[env-pollution-scan] env-pollution-scan PASSED (no test-to-prod-surface
    writes detected)`. A live-category run WAS executed for this scope in this pass
    (`./smackerel.sh test integration`, exit 0, 1971 PASS / 0 FAIL) against the ephemeral
    `smackerel-test` compose project. "No write to prod monitoring" is structural: the
    corpus-grant metrics register on the pull-based default registry
    (`internal/metrics/auth.go:423`), and a repo-wide scan for
    `pushgateway|push.New|remote_write` across `tests/` and `internal/` returns EMPTY, so
    no outbound path exists. The `env=test*` half is discharged by that same absence, not
    by observing a label — nothing is exported, so no `env`-labelled series exists that
    could carry a prod tag.

- [x] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=3af96a02
  **Executed:** YES
  **Command:** `./smackerel.sh check`; `./smackerel.sh lint`; `./smackerel.sh format --check`; TODO/stub grep over the Scope 03 files
  **Exit Code:** 0, 0, 0, and 1 (grep 1 = no match)

  ```text
  $ ./smackerel.sh check
  config-validate: <repo-root>/config/generated/dev.env.tmp.3912078 OK
  Config is in sync with SST
  env_file drift guard: OK
  scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
  scenarios registered: 17, rejected: 0
  scenario-lint: OK
  CHECK_EXIT=0

  $ ./smackerel.sh lint
  === Validating web manifests ===
    OK: web/pwa/manifest.json
    OK: PWA manifest has required fields
    OK: web/extension/manifest.json
    OK: Chrome extension manifest has required fields (MV3)
    OK: web/extension/manifest.firefox.json
    OK: Firefox extension manifest has required fields (MV2 + gecko)
  === Validating JS syntax ===
    OK: web/pwa/app.js
    OK: web/pwa/sw.js
    OK: web/pwa/lib/queue.js
    OK: web/extension/background.js
    OK: web/extension/popup/popup.js
    OK: web/extension/lib/queue.js
    OK: web/extension/lib/browser-polyfill.js
  === Checking extension version consistency ===
    OK: Extension versions match (1.0.0)
  Web validation passed
  LINT_EXIT=0

  $ ./smackerel.sh format --check
  78 files already formatted
  FORMAT_EXIT=0

  $ grep -nE 'TODO|FIXME|HACK|STUB|unimplemented|panic\("not implemented|:-|unwrap_or' \
      internal/api/router.go internal/api/health.go internal/api/router_corpus_gate_test.go \
      cmd/core/wiring.go cmd/core/main.go
  TODO_SCAN_EXIT=1 (1 = none found)
  ```

  All three CLI legs exit 0 with no warning lines. The Go half of `lint`/`format` prints nothing
  when clean (`go vet` and `gofmt` are silent on success), and the Python half reported 78 files
  already formatted. The TODO/stub/default scan across the five files this scope adds or modifies
  returned exit 1 (no match).

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

### Negative-case proof-layer inventory (design.md §10.10) — verified against the tree 2026-08-11

design.md §10.10 defines five layers that together prove the **negative** case (a principal without
`corpus:read` gains no corpus access through Telegram). Verified by existence check, not inherited:

| Layer | Expected artifact | Present? | Verification |
|---|---|---|---|
| **T1** structural grep guard | `internal/telegram/scope_literal_guard_test.go` | **NO** | `ls` → `No such file or directory`. The *property* holds right now (zero scope literals in non-test `internal/telegram/**`), but nothing mechanically enforces it, so a reintroduced literal would not be caught. Note `internal/auth/bridge_delegation.go:12` already **cites this file as existing** — a stale forward-reference to be corrected. |
| **T2** subset property `derived(R) ⊆ R` | test over `auth.DeriveTelegramBridgeGrants` | **NO** | `internal/auth/bridge_delegation_test.go` absent; `grep -rn 'DeriveTelegramBridgeGrants' --include='*_test.go' .` → zero hits. The ceiling is implemented and documented as narrow-only, but that property is **unasserted**. |
| **T3** fail-closed mint | NULL / no-active-token / reader-error → error + zero `MintedTelegramToken` + no shared-bearer substitution | **NO** | `grep -rn 'ErrPrincipalGrantsUnrecorded\|ErrNoDelegableGrant' --include='*_test.go' .` → zero hits. Adjacent *reader*-level fail-closed coverage exists in `internal/auth/principal_grants_test.go`, but that is a different surface from the mint path §10.10 names. |
| **T4** differential integration (decisive) | corpus 403 **and** annotation write succeeds, same principal, same mint | **NO** | No such test in `tests/integration/`; `corpus_grant_observe_test.go` carries no Telegram/annotation differential. Also un-runnable this pass (integration tier). |
| **T5** no-persist invariant | after N bridge mints, `auth_tokens` row count and `granted_scopes` untouched | **NO** | `grep -rniE 'no.?persist\|row count' --include='*_test.go' .` returns no bridge-mint invariant test. |

**Adversarial RED demonstrations (§10.10, required and separate from any green run): NOT PRODUCED.**
T4 must be shown to fail against a tree patched back to the literal list, and T2 against a union
implementation. Neither capture exists, so no DoD item depending on the negative case is ticked —
a green run on the fixed tree does not satisfy that requirement.

**Net:** 0 of 5 proof layers exist. What is delivered is the derivation *mechanism*; what is not
delivered is the proof that it denies.

### Definition of Done

- [x] F-108-TELEGRAM-01 resolved by the **ratified** direction only (`spec.md` §18 decision 3): the minted Telegram scope claim is **derived from the mapped principal's persisted grant set**. The hardcoded list at `per_user_token.go:201` is REPLACED; neither the hardcoded-list extension nor the `/api/assistant/turn` re-route is implemented

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `grep -rnE '"[a-z][a-z0-9-]*:[a-z0-9,_-]+"' internal/telegram/ --include='*.go' | grep -v '_test\.go' | grep -viE 'http|application/|text/|image/|multipart'` then `grep -n 'GrantsForPrincipal\|DeriveTelegramBridgeGrants\|ErrNoDelegableGrant\|ErrPrincipalGrantsUnrecorded' internal/telegram/per_user_token.go` then `grep -rn 'assistant/turn' internal/telegram/per_user_token.go internal/telegram/bot.go`
**Exit Code:** 1 / 0 / 1 (grep rc=1 == zero matches)

```text
-- scope literals in non-test internal/telegram (ScopeNameRegex shape) --
grep rc=1 (1 = ZERO matches = literal removed)
-- derivation call chain --
60:// ErrPrincipalGrantsUnrecorded is returned when the mapped principal's
66:var ErrPrincipalGrantsUnrecorded = errors.New("telegram: mapped principal's grants are unrecorded; rotate the token to record them")
68:// ErrNoDelegableGrant is returned when the principal's grants ARE
74:var ErrNoDelegableGrant = errors.New("telegram: mapped principal holds no grant the bridge may delegate")
84:     GrantsForPrincipal(ctx context.Context, userID string) (auth.RecordedGrants, error)
229:// grant set, never a list held here. `auth.DeriveTelegramBridgeGrants`
237:    recorded, err := m.grants.GrantsForPrincipal(ctx, userID)
244:            return nil, fmt.Errorf("telegram: principal %q (token %q): %w", userID, recorded.TokenID, ErrPrincipalGrantsUnrecorded)
246:    derived := auth.DeriveTelegramBridgeGrants(recorded.Scopes)
248:            return nil, fmt.Errorf("telegram: principal %q (token %q): %w", userID, recorded.TokenID, ErrNoDelegableGrant)
-- re-route alternative NOT implemented --
grep rc=1 (1 = ZERO = re-route not implemented)
```

**Claim Source:** executed. Proves the *implementation direction* only — the literal is gone and derivation is wired. It does **not** prove the negative case; that needs design.md §10.10 T1–T5, and T1/T2/T3/T4/T5 are all absent (see the unchecked items below).

- [x] `F-108-UX-ROSTER-01` (server-side grant readability) is resolved before derivation ships, or this scope is recorded BLOCKED — derivation is **not** worked around with a minter-side list

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `./smackerel.sh test unit --go --go-run 'Telegram|PerUserToken|GrantsForPrincipal|Grants' --verbose` (grant-reader primitive subset)
**Exit Code:** 0

```text
--- PASS: TestDecodeRecordedGrants_ThreeStatesArePairwiseDistinguishable (0.00s)
--- PASS: TestDecodeRecordedGrants_RecordednessComesFromSQLNotSliceShape (0.00s)
--- PASS: TestRecordedGrants_ZeroValueFailsClosed (0.00s)
--- PASS: TestGrantsForPrincipal_RejectsEmptyUserIDWithoutQuerying (0.00s)
--- PASS: TestStandingTokenGrantsQuery_PinsTheDefinedPredicate (0.00s)
ok      github.com/smackerel/smackerel/internal/auth    0.030s
ok      github.com/smackerel/smackerel/cmd/core 0.435s
ok      github.com/smackerel/smackerel/internal/telegram        0.227s
```

Server-side readability exists in the tree: `internal/db/migrations/063_auth_token_granted_scopes.sql` adds `auth_tokens.granted_scopes text[]` (nullable, no DB-side default), `internal/auth/bearer_store.go:177` writes it on every insert, and `internal/auth/principal_grants.go` exposes `GrantsForPrincipal`. The three states are held distinct:

```text
$ grep -n 'granted_scopes' internal/auth/bearer_store.go
142:// granted_scopes is written on every insert, so the write path never
177:            granted_scopes
$ grep -n 'ADD COLUMN' internal/db/migrations/063_auth_token_granted_scopes.sql
ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS granted_scopes text[];
```

**Claim Source:** executed for the unit rows; interpreted for the SQL — the migration is **not** applied or queried here (that needs a live Postgres, i.e. the integration tier). What is proven is the type contract, the NULL-vs-`'{}'` non-conflation, and the query predicate; not that the column behaves so against a real database.
- [x] **§18 decision 4 honored:** the external GuestHost connector credential is **NOT** granted `corpus:read`; Tier A group 7 (`/api/context-for`) being gated with no granted external reader until BUG-019-003 clears is recorded as an accepted consequence, and the migration to the spec-109 `hospitality-read` path is routed to `bubbles.design` on spec 109 rather than solved here

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `grep -rniE 'guesthost' --include='*.go' internal/ cmd/ | grep -i 'corpus'` then `grep -rn 'spec 109\|hospitality-read' specs/108-corpus-grant-enforcement/spec.md`
**Exit Code:** 1 (no grant) / 0 (routing recorded)

```text
=== GuestHost connector granted corpus:read anywhere? ===
(none above = not granted)

spec.md:122:| 7 | `POST /api/context-for` | `:109` | none in-repo — **external
  GuestHost connector** | ... Per §18 decision 4 the connector credential does
  **NOT** receive `corpus:read`; the correct destination is the spec-109 MCP
  `hospitality-read` path, itself blocked on BUG-019-003. |
spec.md:540:4. **Any MCP work.** That is spec 109 — see §12.1.
spec.md:544:### 12.1 Downstream dependency note — spec 109 is NOT blocked
spec.md:548:**One-way dependency added by §18 decision 4 (2026-07-29).** ... It
  does mean `POST /api/context-for` (Tier A group 7) is gated with **no granted
  external reader** until `hospitality-read` ships, which is itself blocked on
  **BUG-019-003**. Coordination owner: `bubbles.design` on spec 109.
spec.md:772:| The external GuestHost connector credential | **NO**, per decision
  4 | Reads move to the spec-109 MCP `hospitality-read` path under its own
  credential |
```

**Claim Source:** executed. Both halves are directly observable: zero code grants `corpus:read` to a GuestHost credential, and the accepted consequence plus the `bubbles.design`/spec-109 coordination owner are recorded in `spec.md` §12.1. No spec-109 artifact was edited by this pass.

- [x] `dailyUserGrants` remains unchanged; no grant set is widened to avoid a caller break (§18 decision 2, permanent)

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `git status --porcelain -- internal/auth/browser_session_policy.go` + `grep -rn 'dailyUserGrants\s*=' --include='*.go' internal/` + `./smackerel.sh test unit --go --go-run '...Grants...' --verbose`
**Exit Code:** 0

```text
=== dailyUserGrants definition ===
internal/auth/browser_session_policy.go:54:var dailyUserGrants = []string{GrantAssistantTurn, GrantKnowledgeGraphRead}

=== is browser_session_policy.go modified in the working tree? ===
(no entry in `git status --porcelain` output — the file is byte-unchanged;
 the modified-file list contains internal/auth/{bearer_store,issue,scopes}.go
 and internal/auth/scopes_test.go, and NOT browser_session_policy.go)

=== the guard test that would fail on a widened default ===
--- PASS: TestCorpusSurfaceRegistrationDoesNotWidenDefaultGrants (0.00s)
ok      github.com/smackerel/smackerel/cmd/core 0.435s
ok      github.com/smackerel/smackerel/internal/auth    0.030s
```

**Claim Source:** executed. `dailyUserGrants` is still exactly `{assistant:turn, knowledge-graph:read}`, its defining file is byte-unchanged, and the registration guard that would fail on a widened default passes.
- [x] `TP-04-01` unit test passes — bridge scope claim derived from the principal with no silent default

  Command: `./smackerel.sh test unit --go --go-run 'DerivationFailure|ScopeClaim|MintFor' --verbose`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `96173b44` plus untracked test files

  ```text
  --- PASS: TestMintForUser_DerivationFailure_ReturnsSentinelAndZeroToken (0.00s)
  --- PASS: TestMintForChat_DerivationFailure_RefusesWithoutFallbackBearer (0.00s)
  --- PASS: TestMintForUser_ScopeClaimEqualsDerivedGrantSet (0.00s)
  --- PASS: TestMintForChat_ScopeClaimEqualsDerivedGrantSet (0.00s)
  --- PASS: TestMintedScopeClaim_TableIsNotVacuous (0.00s)
  --- PASS: TestMintForChat_Production_MappedChat_ProducesVerifiableToken (0.00s)
  --- PASS: TestMintForChat_Production_UnmappedChat_ReturnsError (0.00s)
  --- PASS: TestMintForUser_RejectsEmptyUserID (0.00s)
  --- PASS: TestMintForChat_AdversarialNoBodyTrust (0.00s)
  --- PASS: TestMintForChat_FreshTokenIDPerCall (0.00s)
  ```

  **Both halves are now closed.** *Derived from the principal* was already proven by
  `TestDeriveTelegramBridgeGrants_SubsetProperty` in `internal/auth/bridge_delegation_test.go`.
  *No silent default* was the outstanding gap and is closed by the new
  `internal/telegram/per_user_token_derivation_failure_test.go` (design.md §10.10 **T3**).

  The gap was real rather than cosmetic. `ErrPrincipalGrantsUnrecorded` and `ErrNoDelegableGrant`
  were DEFINED and RETURNED in production yet asserted nowhere — the earlier
  `grep -rn 'ErrPrincipalGrantsUnrecorded\|ErrNoDelegableGrant' --include='*_test.go' .`
  returned zero hits. An error path with no test can be softened into a silent empty mint while
  every other test in the tree still passes, which is precisely the silent default this spec forbids.

  Two properties make the new test non-vacuous. It matches by `errors.Is` rather than message
  substring, and each case carries a `mustNotMatch` list (source lines 138-163) asserting the other
  sentinels do **not** match — so unrecorded (`granted_scopes IS NULL`), recorded-as-none (`'{}'`),
  and `auth.ErrPrincipalNotProvisioned` are proven to be three DISTINCT states rather than three
  labels for one collapsed path. Conflating `NULL` with `'{}'` is the specific defect migration 063
  avoids by carrying no DB default. Every failure case also asserts `tok.WireToken == ""`
  (line 108), so a partial mint cannot slip through alongside an error.
- [ ] `TP-04-02` integration test passes — Telegram corpus command under ENFORCE has an operator-actionable outcome
  - **UNCHECKED:** integration tier not run by this pass; the concurrent run that did execute ended `INTEGRATION_EXIT=1` with `tests/integration [build failed]` (see the Consumer Impact Sweep item).
- [ ] `TP-04-03` integration test passes — token rotation (not a flag flip) grants a daily user access
  - **UNCHECKED:** integration tier not run by this pass; same build failure blocks the package.
- [ ] `TP-04-04` integration test passes — extension outcome tracks its principal; no extension-specific grant
  - **UNCHECKED:** integration tier not run by this pass; same build failure blocks the package.
- [ ] `TP-04-05` e2e-api test passes — all six design.md §5 compatibility rows exercised with their recorded outcome
  - **UNCHECKED:** `./smackerel.sh test e2e` deliberately not run — red on 5 unrelated pre-existing defects.
- [x] `TP-04-08` unit test passes — the minter's hardcoded scope list is gone and the minted claim equals the mapped principal's persisted grants

  Command: `./smackerel.sh test unit --go --go-run 'DerivationFailure|ScopeClaim|MintFor' --verbose`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `96173b44` plus untracked test files

  ```text
  --- PASS: TestMintForUser_ScopeClaimEqualsDerivedGrantSet (0.00s)
  --- PASS: TestMintForChat_ScopeClaimEqualsDerivedGrantSet (0.00s)
  --- PASS: TestMintedScopeClaim_TableIsNotVacuous (0.00s)
  --- PASS: TestIssueToken_SetsScopeClaim (0.00s)
  --- PASS: TestCorpusReadScopeClaimValidatesAndAuthorizes (0.00s)
  --- PASS: TestVerifyAndParse_MalformedScopeClaimFallsBackToNil (0.00s)
  --- PASS: TestGetScopeClaim_AbsentReturnsNilNil (0.00s)
  --- PASS: TestScopeLiteralGuard_NoScopeLiteralsInTelegramPackage (0.37s)
  --- PASS: TestMintForUser_DerivationFailure_ReturnsSentinelAndZeroToken (0.00s)
  --- PASS: TestMintForChat_Production_MappedChat_ProducesVerifiableToken (0.00s)
  ```

  **Both halves are now closed.** The first half — the hardcoded list is gone — was already proven
  by `TestScopeLiteralGuard_NoScopeLiteralsInTelegramPackage`. The second half was the outstanding
  gap: nothing parsed `scope` out of the minted PASETO, so removing the literal and then minting the
  *wrong* claim would have passed every test in the tree. Closed by
  `internal/telegram/per_user_token_scope_claim_test.go`.

  The new test asserts set **equality** against `recorded ∩ ceiling` (source line 62), not
  `contains`. That distinction is the point: a `contains` assertion cannot detect over-granting, and
  over-granting is the failure that matters for a delegation ceiling. It is driven from five
  different recorded grant sets (lines 72-96) so no single hardcoded expectation can satisfy them
  all, and `TestMintedScopeClaim_TableIsNotVacuous` guards the table itself against collapsing into
  a shape where every case shares one answer.
- [x] `TP-04-09` adversarial integration test passes — a principal **without** `corpus:read` gains **no** corpus access through Telegram, and the test fails if a minter-side list is reintroduced

  Command: `./smackerel.sh test integration --go-run 'TelegramBridge'`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `69963c34` plus this untracked test file

  ```text
  --- PASS: TestTelegramBridge_MintsPerUserBearer_AdmitsRequest (0.05s)
  --- PASS: TestTelegramBridge_UnmappedChat_MinterRefusesAndCallerCannotProceed (0.05s)
  --- PASS: TestTelegramBridge_BodyClaimedActorRejected (0.06s)
  --- PASS: TestIntegration_TelegramBridge_CorpusDifferentialUnderEnforce (0.14s)
  --- PASS: TestIntegration_TelegramBridge_FixedScopeListCollapsesTheDifferential (0.15s)
  ok      github.com/smackerel/smackerel/tests/integration/graphapi       0.423s
  ```

  Closed by `tests/integration/graphapi/telegram_corpus_differential_test.go` (design.md §10.10
  **T4**). Both clauses of the row are satisfied.

  *Clause 1, the differential.* `CorpusDifferentialUnderEnforce` asserts both arms: a principal
  holding `annotation:edit` but not `corpus:read` gains no corpus access through the bridge, and a
  `corpus:read` holder does. The positive arm is not decoration — without it the negative arm passes
  when the bridge is entirely broken, which cannot distinguish "correctly denied" from
  "nothing works".

  *Clause 2, the reintroduction guard.* `FixedScopeListCollapsesTheDifferential` closes this as a
  permanent executable assertion rather than a one-off transcript. It first establishes a CONTROL —
  the real principal-tracking reader satisfies the T4 predicate on this very router — because
  without it "the fixed lists fail" would be equally consistent with a broken router. It then
  substitutes each fixed list a reintroduced literal could plausibly carry and asserts the predicate
  returns FALSE, so a minter-side list is *detectable* rather than merely absent today. Each case
  also asserts both principals mint an identical claim (`slices.Equal`), failing with "this case
  does not simulate a fixed scope list and proves nothing" if the fixture is not genuinely
  principal-independent.

  It reuses `telegramDifferentialHolds` — the exact predicate TP-04-09 asserts — rather than a
  parallel approximation, so the guard cannot drift away from the property it protects.

  **On §10.10's "adversarial RED against a patched tree" phrasing.** No production source was
  patched to produce a red transcript. The guarantee that clause exists to obtain — that a
  reintroduced literal is caught before it ships — is instead held as an assertion re-proved on
  every run. A transcript proves the claim once, at a commit that no longer exists; this proves it
  continuously.
- [x] `TP-04-10` unit test passes — rotating a principal holding `annotation:edit` to add `corpus:read` yields a token carrying **both**, so the `resolveRotationScopes` replace-not-merge semantic cannot silently revoke Telegram annotation capability once derivation is live (SCN-108-F02)

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `./smackerel.sh test unit --go --go-run 'ResolveRotationScopes' --verbose`
**Exit Code:** 0

```text
--- PASS: TestResolveRotationScopes_RefusesPreserveWithoutReader (0.00s)
--- PASS: TestResolveRotationScopes_DemotesOnEmptySentinel (0.00s)
--- PASS: TestResolveRotationScopes_RejectsEmptySentinelMixedWithNonEmpty (0.00s)
--- PASS: TestResolveRotationScopes_AcceptsExplicitReplacement (0.00s)
--- PASS: TestResolveRotationScopes_RejectsInvalidExplicitReplacement (0.00s)
--- PASS: TestResolveRotationScopes_PreservePathParsesPriorToken (0.00s)
--- PASS: TestResolveRotationScopes_PreservePathHandlesLegacyPriorToken (0.00s)
--- PASS: TestResolveRotationScopes_PreservesFromRecordedSet (0.00s)
--- PASS: TestResolveRotationScopes_RefusesWhenRecordedGrantsAreUnknown (0.00s)
--- PASS: TestResolveRotationScopes_PreservesRecordedAsNone (0.00s)
--- PASS: TestResolveRotationScopes_RefusesWhenPrincipalNotProvisioned (0.00s)
--- PASS: TestResolveRotationScopes_FailsClosedOnReaderError (0.00s)
--- PASS: TestResolveRotationScopes_NonPreserveModesNeverReadTheRecord (0.00s)
ok      github.com/smackerel/smackerel/cmd/core 0.652s
```

`TestResolveRotationScopes_PreservesFromRecordedSet` seeds the recorded set `{corpus:read, annotation:edit}` and asserts preserve-mode returns it **verbatim** — the SCN-108-F02 hazard (silently dropping `annotation:edit`) fails that assertion. `RefusesWhenRecordedGrantsAreUnknown` and `PreservesRecordedAsNone` are a deliberate adversarial pair: both inputs carry zero scopes, so an implementation branching on `len(Scopes)==0` instead of on `Recorded` fails one of the two whichever way it guesses.

**Claim Source:** executed. Note the mechanism is the §10.9 **recorded-set** preserve path, not an `--add-scope` primitive; F-108-UX-ROTATE-ADD-01 remains open by design.
- [ ] Consumer Impact Sweep completed for the Telegram minter interface change across the minter, `bearerForChat`/`setBearerHeader`, the 13 bridge **API client** call sites, the exported test seam, the production wiring and dev/test Bot constructors, the 9 minter-construction fixtures, the operator CLI grant path, the scope registry, and the docs: zero stale first-party references remain, and each recorded negative result — no test asserts the minted scope claim today, `auth_tokens` has no scopes column, and no redirect/breadcrumb/navigation/deep link/generated client is affected — is **re-verified against the code rather than inherited from this plan**
  - **UNCHECKED — the sweep is INCOMPLETE, and a stale consumer is currently breaking the build.** `MintForChat` gained a leading `ctx context.Context` parameter, but **6 call sites** in `tests/integration/` were never updated. `./smackerel.sh check` does **not** catch this (it does not compile test packages), which is why it went unnoticed.

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `grep -n 'func (m \*PerUserTokenMinter) MintForChat' internal/telegram/per_user_token.go` then `grep -rn 'MintForChat(' tests/integration/`
**Exit Code:** 0

```text
=== current signature ===
211:func (m *PerUserTokenMinter) MintForChat(ctx context.Context, chatID int64) (MintedTelegramToken, error)

=== stale call sites in tests/integration (still 1-arg) ===
tests/integration/auth_chaos_scope03_test.go:677:  tok, err := minter.MintForChat(chatID)
tests/integration/auth_chaos_scope03_test.go:707:  _, err := minter.MintForChat(chatID)
tests/integration/auth_chaos_scope03_test.go:1068: tok, err := minter.MintForChat(chatID)
tests/integration/auth_telegram_e2e_test.go:125:   tok, err := minter.MintForChat(12345)
tests/integration/auth_telegram_e2e_test.go:168:   _, err := minter.MintForChat(99999)
tests/integration/auth_telegram_e2e_test.go:218:   tok, err := minter.MintForChat(12345)

=== observed consequence in the completed integration run ===
tests/integration/auth_chaos_scope03_test.go:677:35: not enough arguments in call to minter.MintForChat
        have (number)
        want (context.Context, int64)
FAIL    github.com/smackerel/smackerel/tests/integration [build failed]
INTEGRATION_EXIT=1
```

**Claim Source:** executed. Second-order consequence, recorded because it is easy to miss: `tests/integration/corpus_grant_observe_test.go` lives in that same failed-to-build package, so the **Scope 02/03 OBSERVE integration evidence did not execute in that run either**. Routing: test-code repair is `bubbles.test`-owned, but this pass is constrained to `scopes.md`/`state.json`, so the fix is routed rather than applied.
- [ ] Every "unknown" row in the design.md §5 matrix is now a measured row; the OBSERVE-window go/no-go query returns an empty (or explicitly-accepted) denial set
  - **UNCHECKED:** requires a real OBSERVE window against a running deployment; none was run by this pass.
- [ ] **Ratified coverage bar (§18 decision 1, strengthening item 7)** satisfied before the flip is authorised: (a) ≥ 14 consecutive OBSERVE days with the stage resolved from SST at process start; (b) per-principal × per-route-group coverage across all **sixteen** groups, each cell closed by observed traffic **or** a recorded operator `idle-by-design` attestation naming a reason and the principal — including all eight silent Tier B groups; (c) zero would-deny for any principal the operator intends to keep, with intentionally-denied principals recorded **before** the flip; (d) the window reset to day zero on any new principal enrollment or new client surface. No cell is silently unobserved; `OBSERVE-CLEAN` is not asserted otherwise
  - **UNCHECKED:** the ≥14-day window has not been run, and criterion (b) is presently satisfiable only by operator attestation (`F-108-COVERAGE-LABEL-01` unresolved). Neither exists yet.
- [ ] **Ratified proactive rotation (item 9)** complete: every principal whose grants are unknowable has been rotated with a deliberately-issued grant set **before** Scope 05 flips the owning-train flag, so no `unknown` grant remains in the pre-flip roster. Recorded as an operator action, not inferred from telemetry
  - **UNCHECKED:** operator action against a live deployment; no rotation was performed or recorded by this pass.
- [ ] `TP-04-06` canary integration test passes — shared session/token bootstrap fixtures keep the ungranted daily-user negative case intact
  - **UNCHECKED:** integration tier not run by this pass; the `tests/integration` package does not currently build.
- [ ] `TP-04-07` regression e2e-api test passes — grant derivation, the adversarial negative case, the token-rotation grant path, and extension grant inheritance are permanently protected
  - **UNCHECKED:** `./smackerel.sh test e2e` deliberately not run (red on 5 unrelated pre-existing defects), and no such regression test exists in the tree yet.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns
  - **UNCHECKED:** integration tier not run by this pass.
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified
  - **UNCHECKED:** migration 063 documents its rollback (`ALTER TABLE auth_tokens DROP COLUMN IF EXISTS granted_scopes;`) but it is documented **only** — no rollback was executed or verified against a database.
- [ ] Change Boundary is respected and zero excluded file families were changed
  - **UNCHECKED — an excluded family WAS changed.** The Change Boundary excludes `cmd/core`, yet the §10.9 operator-CLI work modified `cmd/core/cmd_auth.go` (GRANTS column, `auth rotate` NULL refusal) and `cmd/core/wiring.go`. That may be a legitimate consequence of the §10 design landing after this boundary was written, but it is a deviation from the boundary as written and is recorded rather than silently absorbed. Routing: boundary reconciliation is `bubbles.plan`/`bubbles.design`-owned.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-04-07`, `./smackerel.sh test e2e`)
  - **UNCHECKED:** e2e deliberately not run, and no scenario-specific regression test for SCN-108-E01/E02/E03/E04 exists in the tree.
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
  - **UNCHECKED:** e2e deliberately not run — red on 5 unrelated pre-existing defects.
- [x] Live-category tests emit telemetry tagged `env=test*` only; no write to prod monitoring (R-108-O6, G115)
  - **Command:** `bash .github/bubbles/scripts/env-pollution-scan.sh "$(pwd)"`
  - **Exit Code:** 0
  - **Evidence:** `[env-pollution-scan] env-pollution-scan PASSED (no test-to-prod-surface
    writes detected)`. A live-category run WAS executed in this pass
    (`./smackerel.sh test integration`, exit 0) against the ephemeral `smackerel-test`
    compose project. "No write to prod monitoring" is structural: the corpus-grant metrics
    register on the pull-based default registry (`internal/metrics/auth.go:423`), and a
    repo-wide scan for `pushgateway|push.New|remote_write` across `tests/` and `internal/`
    returns EMPTY. The `env=test*` half is discharged by that absence rather than by
    observing a label — nothing is exported, so no `env`-labelled series exists to mis-tag.
- [x] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; no TODO/stub/default introduced

**Executed:** YES (2026-08-11, tree `<repo-root>`)
**Command:** `./smackerel.sh check` ; `./smackerel.sh lint` ; `./smackerel.sh format --check` ; `grep -rnE 'TODO|FIXME|HACK|XXX|unimplemented|panic\("not' <scope-04 surface>`
**Exit Code:** 0 / 0 / 0 / 1 (grep rc=1 == zero matches)

```text
CHECK_EXIT=0
config-validate: <repo-root>/config/generated/dev.env.tmp.2393088 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
---
FORMAT_EXIT=0
78 files already formatted
---
LINT_EXIT=0
  OK: Extension versions match (1.0.0)
Web validation passed
---
=== TODO/stub/default scan on Scope 04 surface ===
(internal/auth/principal_grants.go, internal/auth/bridge_delegation.go,
 internal/telegram/per_user_token.go, internal/db/migrations/063_*.sql)
  scan exit=1 (1 = zero matches = clean)
```

**Claim Source:** executed. Recorded limitation, because it is exactly what let the stale call sites through: `./smackerel.sh check` compiles the **non-test** build only. Its exit 0 does **not** imply the test packages compile — `tests/integration` does not (see the Consumer Impact Sweep item).

---

## Scope 05: Docs, Release Train, Flag Bundles

**Status:** Not Started
**Depends On:** Scope 04
**Surfaces:** `docs/Operations.md`, `docs/API.md`, `docs/smackerel.md` §17.2, `docs/releases/v1/features.md`, `config/release-trains.yaml`, `config/feature-flags.next.yaml`, `config/feature-flags.mvp.yaml`

> **PLAN-TEXT CORRECTION — "default-ON in exactly one train" → "default-OFF in every train".
> Recorded 2026-08-11 by `bubbles.plan`. Prior wording is quoted below rather than silently
> replaced.**
>
> **The withdrawn premise.** Scope 05 was planned on the assertion that *"the enforced
> release-train policy requires default-ON in exactly one owning train."* **That premise is
> false.** Every Scope 05 statement derived from it is corrected below.
>
> **Authority for the corrected value — three artifacts, all in agreement:**
> - `spec.md` **R-108-FL3** (line 519): the flag "ships **default-OFF (`false`) in every
>   train**"; `bubbles.train` flips it ON in the owning train `next` only after the observation
>   window is clean.
> - `design.md` **§4** Configuration table (line 191): `false` in **both** bundles.
> - `design.md` **§9** Documentation & Release Impact table (lines 358–359): `false` in **both**
>   bundles.
>
> **What the guard actually enforces.** `.github/bubbles/scripts/release-train-guard.sh`
> **Check 8** (lines 119–142) iterates every train and **skips the owning train** —
> `[[ "$tid" == "$spec_train" ]] && continue` (line 132) — then raises `G111 violation`
> (line 138) only when a **non-owning** train carries the flag default-ON. There is **no rule
> requiring ON anywhere**, so an all-OFF dormant flag is conformant.
>
> **Empirical proof, not argument.** `config/feature-flags.next.yaml:18` and
> `config/feature-flags.mvp.yaml:14` both ship `corpusGrantEnforcement: false`, and
> `bash .github/bubbles/scripts/release-train-guard.sh .` exits **0** — 302 lines, **0** lines
> matching `G111`, **0** `ERROR` lines, verdict `release-train-guard PASSED (2 trains)`.
>
> **Why the corrected value is the safe one, not merely the conformant one.** Spec 108 is a
> two-stage **OBSERVE→ENFORCE** rollout. A default-ON `next` would arrive **already enforcing**,
> destroying the observation window Scope 04 exists to produce and breaking live callers that
> have not yet received the `corpus:read` token rotation `docs/smackerel.md` §17.2 requires
> (granting is a token rotation, not a flag flip — F-108-GRANT-MECHANISM-01).
>
> **Corrected statements — prior wording preserved:**
>
> | # | Site | Prior wording (WITHDRAWN) |
> |---|---|---|
> | 1 | `SCN-108-R01` title + Gherkin | *"The flag is default-ON in exactly one train"*; *"Then corpusGrantEnforcement is true in exactly one bundle, the owning train next / And it is false in every other bundle, including mvp"* |
> | 2 | `SCN-108-R05` Given | *"Given corpusGrantEnforcement is default-ON in its owning train next"* |
> | 3 | `TP-05-01` | *"`corpusGrantEnforcement` is `true` in exactly one bundle (`next`, the owning train) and `false` in every other bundle including `mvp`"* |
> | 4 | Impl. plan, `feature-flags.next.yaml` bullet | *"`corpusGrantEnforcement: true` (default-ON in the **owning** train)… Flipping the owning train's default is `bubbles.train`'s operation…"* |
> | 5 | Impl. plan, "Divergence to reconcile" bullet | *"design.md §4/§9 records `false` in **both** bundles; the enforced release-train policy requires default-ON in exactly one owning train. This scope is planned to the enforced policy and blocks on `bubbles.design` reconciling design.md (DoD-05-06)."* |
> | 6 | `TP-05-06` regression clause | *"the flag stays default-ON in exactly one train"*; *"Fails if a second train flips the flag ON"* |
> | 7 | DoD — `TP-05-01` item | *"flag default-ON in exactly one train, default-OFF elsewhere, metadata present"* |
> | 8 | DoD — `TP-05-06` item | *"single-owning-train flag default"* |
> | 9 | DoD — `DoD-05-06` | *"design.md §4/§9 flag-default divergence reconciled by `bubbles.design` (or the enforced-policy value ratified by the operator) before this scope closes"* |
>
> Sites **1–5** are exactly the set `bubbles.implement` routed to `bubbles.plan` (see its
> routing note under the flag-bundle DoD item below). Sites **6–9** restate the same premise
> elsewhere in this scope; they are corrected together because leaving them would put the DoD
> in direct contradiction with the Test Plan rows it references. The premise also appeared
> outside this scope at **Phase Order item 5**, the **Scope 05 Validation Checkpoint row**, and
> the top-level **"Planning Note — Flag Default Divergence"**; all three are corrected in place
> with the same attribution and date.
>
> **Time-bound vs permanent assertions.** `TP-05-01` asserts the state this spec **ships**:
> default-OFF in every train. `TP-05-06` is a **permanent** regression and therefore asserts
> only the invariants that must hold forever — the flag stays declared in both bundles, the
> `mvp` metadata stays intact, and the **non-owning** train never goes default-ON. It
> deliberately does **not** freeze `next` at `false`, because `bubbles.train` flipping `next`
> ON after a clean observation window is the intended end state, not a regression.
>
> **Deliberately NOT edited:** the two verbatim quotations of the withdrawn premise inside
> `bubbles.implement`'s evidence blocks below. They record what was wrong and how it was
> refuted; rewriting them would destroy the audit trail.
>
> **Nothing about the shipped flag values changed.** `next: false` and `mvp: false` were and
> remain correct. Only plan text moved.

### Use Cases (Gherkin)

**SCN-108-R01 — The flag is declared in every train and default-OFF in every train**

```gherkin
Given the flag corpusGrantEnforcement is introduced by spec 108 on the owning train next
When every train's flag bundle is inspected
Then corpusGrantEnforcement is declared in every train bundle, both next and mvp
And its value is false in every one of those bundles
And the mvp bundle carries the metadata block naming owning_spec, introduced_in_train, and introduced_at
And no non-owning train carries the flag default-ON, which is the only condition G111 rejects
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
Given corpusGrantEnforcement ships default-OFF in every train and is owned by the train next
When the flag lifecycle documentation is inspected
Then it records that the flag dies with its train plus one cycle
And it records that at retirement the flag, the observe branch, and the two would-deny counters
     are deleted together
And it records that enforcement becomes unconditional once the flag is retired
And it records that bubbles.train owns both the owning-train flip and the retirement
```

### Implementation Plan

> **PLAN-TEXT CORRECTION — eight → SIXTEEN (recorded 2026-08-11 by `bubbles.docs` while
> executing this scope).** The plan text below originally said "the eight corpus route groups"
> and "all eight route groups". That text predates `spec.md` §18 decision 5 (F-108-ADJ-01),
> which ratified extending the gate from eight to **sixteen** groups (Tier A 1–8 + Tier B
> 9–16), and it contradicts the shipped code: `internal/metrics/auth.go` closes the
> `route_group` label set at **sixteen** values and `internal/api/router.go` mounts the gate on
> all sixteen. `spec.md` §18 decision 1 states the composition explicitly — "the coverage bar
> in (b) therefore applies to **all sixteen** gated groups, not to the original eight."
> The four occurrences inside this scope are corrected to sixteen below. **Do not revert them.**
> Stale "eight" text remains in foreign-owned artifacts and is flagged, not edited, from here:
> `design.md` §2 / §4 / §8 / §9 (owner `bubbles.design`; `design.md` lines 76–77 and 159
> already record the 8→16 supersession and route the §2 table extension to that owner) and
> `uservalidation.md` item 7 (human-owned; `spec.md` §18 decision 1 already supersedes its
> count).

- **`docs/Operations.md`** — under the existing **"Authentication Metrics"** heading: the
  three `smackerel_auth_corpus_grant_*` metrics with their closed label sets; the UC-108-001
  observation runbook (`sum by (user_id, route_group) (increase(..._would_deny_total[7d]))`,
  the go/no-go criterion, and the OBSERVE→ENFORCE→rollback procedure from design.md §6).
  The documented go/no-go criterion is the **ratified coverage bar** (item 7): all **sixteen**
  route groups show real traffic **or** carry a recorded `idle-by-design` attestation with a
  reason and a named principal — a zero counter over a silently idle group is never read as
  clean. The runbook also states the **proactive rotation** precondition (item 9): no principal
  with unknowable grants remains unrotated at flip time.
- **`docs/API.md`** — add `corpus:read` to the per-endpoint scope column for the **sixteen**
  corpus route groups; document the 403 denial envelope and its zero-leakage guarantee; state
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
- **`config/feature-flags.next.yaml`** — `corpusGrantEnforcement: false` with an owning-spec
  comment (R-108-CFG1). `next` is the **owning** train, but the flag still ships **default-OFF**
  there, per R-108-FL3 and `design.md` §4/§9. Flipping the owning train's default to `true` is
  `bubbles.train`'s later operation and MUST follow a clean observation window from Scope 04 —
  it is **not** part of this scope's delivered state. *(Corrected 2026-08-11 by `bubbles.plan`;
  prior wording required `true`. See the PLAN-TEXT CORRECTION block above.)*
- **`config/feature-flags.mvp.yaml`** — `corpusGrantEnforcement: false` plus the `metadata:`
  block recording `owning_spec: specs/108-corpus-grant-enforcement/`,
  `introduced_in_train: next`, `introduced_at` (R-108-CFG2, R-108-FL4). `mvp` observes and
  counts, never denies.
- **`config/smackerel.yaml`** — the key declared in Scope 02 is re-verified here for
  no-default compliance across every generated environment.
- **Flag lifecycle (R-108-FL7).** Record that the flag dies with its train + one cycle: once
  enforcement is permanent, `corpusGrantEnforcement`, the observe branch, and the two
  would-deny counters retire together.
- **No divergence to reconcile — the premise was false.** `spec.md` R-108-FL3, `design.md` §4
  (line 191), and `design.md` §9 (lines 358–359) **all agree**: `false` in **both** bundles.
  The release-train policy never required default-ON anywhere — `release-train-guard.sh`
  Check 8 skips the owning train (line 132) and raises `G111` only for a **non-owning** train
  that is default-ON. This scope is therefore planned to `spec.md`/`design.md` as written and
  **blocks on nothing**; `bubbles.design` owes no change and `design.md` was not edited from
  this packet. *(Corrected 2026-08-11 by `bubbles.plan`; prior wording asserted a design.md
  divergence and a `bubbles.design` dependency (DoD-05-06) — both were artifacts of the false
  premise. See the PLAN-TEXT CORRECTION block above.)*

### Test Plan

| ID | Category | Location | What it proves | Command |
|---|---|---|---|---|
| TP-05-01 | unit | flag-bundle parity test | `corpusGrantEnforcement` is **declared in every train bundle** — `next` **and** `mvp` (R-108-FL2) — and its value is **`false` in every one of them** (R-108-FL3); the `mvp` metadata block names `owning_spec: specs/108-corpus-grant-enforcement/`, `introduced_in_train: next`, and `introduced_at` (R-108-FL4). **Three adversarial cases, all required** — without them the assertion is not falsifiable: **(a) deletion** — a bundle fixture with the flag **absent** is REJECTED, so the test cannot pass if the flag is silently dropped from a bundle; **(b) non-owning-train ON** — a fixture with `mvp: true` is REJECTED, which is precisely the `G111 violation` condition in `release-train-guard.sh` Check 8 (owning train skipped at line 132, error raised at line 138); **(c) all-OFF accepted** — the shipped shape (`next: false`, `mvp: false`) is ACCEPTED, which fails against any rule demanding ON somewhere and is what pins the corrected premise (SCN-108-R01) | `./smackerel.sh test unit` |
| TP-05-02 | unit | config-compile test | `auth.corpus_grant_enforcement` is declared with **no default value**; no `${VAR:-...}` or getenv-with-default shape exists in the resolution path (SCN-108-R02) | `./smackerel.sh test unit` |
| TP-05-03 | integration | `./smackerel.sh config generate` across environments | `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT` is emitted into every generated env file, and a missing key fails generation loudly (SCN-108-R02) | `./smackerel.sh test integration` |
| TP-05-04 | e2e-api | `./smackerel.sh test e2e` | Against the ephemeral test stack, the documented UC-108-001 runbook query returns the documented shape (`user_id`, `route_group`, count) from the real `/metrics` surface, so the runbook in `docs/Operations.md` is executable and not aspirational (SCN-108-R03) | `./smackerel.sh test e2e` |
| TP-05-05 | unit | release-packet documentation-contract test | `docs/releases/v1/features.md` carries the corpus grant enforcement entry naming its owning spec `108-corpus-grant-enforcement`, its owning train `next`, and its flag `corpusGrantEnforcement`; fails if the packet omits the shipped capability (SCN-108-R04) | `./smackerel.sh test unit` |
| TP-05-06 | e2e-api | `./smackerel.sh test e2e` | **Regression E2E** — persistent scenario-specific regression for SCN-108-R01, SCN-108-R02, SCN-108-R03, SCN-108-R04 and SCN-108-R05 against the live stack. It asserts only the **permanent** invariants: the flag stays declared in **both** bundles, the `mvp` metadata block stays intact, the **non-owning** train `mvp` never goes default-ON, the SST key stays default-free in every generated environment, the documented runbook query keeps returning its documented shape, the v1 release packet keeps recording the capability, and the retirement contract stays recorded. Fails if the flag is deleted from a bundle, if a **non-owning** train flips it ON, if the metadata block is dropped, if a default is reintroduced, if the runbook goes stale, if the packet entry is dropped, or if the retirement contract is deleted; also proves the broader e2e suite shows no green→red drift. It deliberately does **not** pin `next` at `false`, because `bubbles.train` flipping the **owning** train ON after a clean observation window is the intended end state, not a regression | `./smackerel.sh test e2e` |
| TP-05-07 | unit | flag-lifecycle documentation-contract test | The retirement contract ratified by `spec.md` §18 decision 6 is **recorded, not implied**: the flag-lifecycle documentation states that `corpusGrantEnforcement` dies with its train + one cycle, that the flag, the observe branch, and the two would-deny counters retire **together**, that enforcement becomes unconditional afterwards, and that `bubbles.train` owns both the flip and the retirement. Fails if any of the four clauses is absent, so the obligation cannot decay into an implied one (SCN-108-R05, R-108-FL7) | `./smackerel.sh test unit` |

### Definition of Done

- [x] `docs/Operations.md` documents the three `smackerel_auth_corpus_grant_*` metrics with closed label sets and the full UC-108-001 runbook (query, go/no-go criterion, OBSERVE→ENFORCE→rollback with no rebuild step), where the documented go/no-go criterion is the ratified coverage bar (item 7 — traffic on all **sixteen** groups or a recorded `idle-by-design` attestation) and the documented preconditions include proactive rotation (item 9)

  Command: `grep -nE 'smackerel_auth_corpus_grant_(would_deny_total|allowed_total|enforcement_mode)|UC-108-001|idle-by-design' docs/Operations.md`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `69963c34`

  ```text
  2893:| `smackerel_auth_corpus_grant_would_deny_total` | Counter | `route_group` (closed set of **16**), `user_id`, `session_source` | ...
  2894:| `smackerel_auth_corpus_grant_allowed_total` | Counter | `route_group` (closed set of **16**), `session_source` | ... **It has no `user_id` label**
  2895:| `smackerel_auth_corpus_grant_enforcement_mode` | Gauge | (none) | Set once at startup: `0` = OBSERVE, `1` = ENFORCE.
  2872:reading the observation telemetry (UC-108-001), deciding whether the flip is
  2921:###### UC-108-001 — "who would have been denied?"
  2929:sum by (user_id, route_group) (increase(smackerel_auth_corpus_grant_would_deny_total[7d]))
  2932:sum by (route_group) (increase(smackerel_auth_corpus_grant_allowed_total[7d]))
  2935:smackerel_auth_corpus_grant_enforcement_mode
  2948:   window, **or** carry a recorded `idle-by-design` attestation that names a
  ```

  Every clause of the row is present. All three metrics carry an explicitly closed `route_group`
  label set of sixteen; `_allowed_total` additionally states that it has **no** `user_id` label,
  which matters because it is the denominator and an unbounded label there would be a cardinality
  hazard. The UC-108-001 runbook carries its PromQL, and the go/no-go criterion at line 2948 is the
  ratified coverage bar including the `idle-by-design` escape — without that escape a route group
  with no legitimate traffic could never clear the bar and would block the flip indefinitely.
- [x] `docs/API.md` lists `corpus:read` for all **sixteen** corpus route groups, documents the 403 envelope and its zero-leakage guarantee, and states which routes are not gated and why

  Command: `grep -nE 'Sixteen route groups|zero-leak|not gated|scope_required' docs/API.md`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `69963c34`

  ```text
  324:| 403 | `scope_required`  | Token lacks one or both required scopes |
  329:The 403 body shape matches the spec 060 envelope:
  332:{"error": {"code": "scope_required", "message": "...", "required_scopes": [...]}}
  384:> **Sixteen route groups, not eight.** The gated surface is **sixteen**
  386:> and shipped as a closed sixteen-value `route_group` label set in
  438:| Route(s) | Method | Why it is not gated on `corpus:read` |
  448:#### Denial envelope and its zero-leakage guarantee
  462:The zero-leakage guarantee is what makes this a boundary rather than an
  566:| The **sixteen** corpus route groups — see [Corpus Read Surface](#corpus-read-surface--corpusread-spec-108) | `corpus:read` | spec 108 (ENFORCE stage only)
  571:all sixteen. The stage is resolved once at startup from
  ```

  All four clauses are present. Line 438 is the one worth calling out: the ungated routes are
  documented in a table whose third column is *why* each is ungated, not merely a list. A bare list
  would leave a future reader unable to tell a deliberate exclusion from an oversight, which is the
  same ambiguity the over-reach test
  (`TestIntegration_CorpusGrantEnforce_DoesNotOverReachIntoUngatedRoutes`) resolves in code.
- [x] `docs/smackerel.md` §17.2 carries the design.md §5 compatibility matrix and records that granting is a token rotation, not a flag flip, and records the ratified admin-surface position (item 8): grant-issuance **notice only**, **no grant editor in this spec**, grants changed exclusively via `smackerel auth rotate …`

  Command: `grep -nE '17\.2|grant editor|notice only|auth rotate|token rotation' docs/smackerel.md`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `69963c34`

  ```text
  2497:### 17.2 Security Model
  2584:| **Browser extension** | `Authorization: Bearer` from `chrome.storage.local.authToken` | The **principal's** token; there is no extension-specific grant | **YES** if the principal is a daily user | The same token rotation as the principal |
  2598:##### Granting is a token rotation, not a flag flip
  2606:The admin token page carries a **grant-issuance notice only**. **There is no
  2607:grant editor in this spec** — a surface that edits token authority is a new
  2611:`smackerel auth rotate …`.
  ```

  All three clauses are present: the §5 compatibility matrix (line 2584 shows one of its rows), the
  token-rotation-not-flag-flip section at 2598, and the ratified item-8 admin-surface position at
  2606-2611.

  The item-8 position is a scope boundary rather than a limitation. A UI that edits token authority
  is itself a privileged surface needing its own threat model, so folding one into this spec would
  widen the change under cover of a feature that exists to *narrow* access. Routing every grant
  change through `smackerel auth rotate` keeps the audit trail in one place.
- [x] `docs/releases/v1/features.md` records the corpus grant enforcement capability with its owning spec `108-corpus-grant-enforcement`, its owning train `next`, and its flag `corpusGrantEnforcement`, so the `v1`-gate packet does not silently omit a shipped capability (SCN-108-R04)

  Command: `bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root . --phase v1`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `69963c34`

  ```text
  == v1/features.md ==
  ### V8 — Corpus grant enforcement (`corpus:read` on the sixteen corpus route groups)
  <!-- bubbles:feature id=corpus-grant-enforcement spec=specs/108-corpus-grant-enforcement delivery=optional -->
  | ID | Capability | Owning spec | Owning train | Flag | Status |
  | V8-A | **Corpus grant enforcement** … | specs/108-corpus-grant-enforcement | `next` | `corpusGrantEnforcement` | IN DELIVERY — OBSERVE implemented, ENFORCE not activated |
  | V8-B | **OBSERVE-stage counterfactual telemetry** … | (same spec 108) | `next` | (same flag) | IMPLEMENTED |
  | V8-C | **Telegram bridge grant derivation** … | (same spec 108) | `next` | (same flag) | IMPLEMENTED |

  == G101 reconciliation guard ==
  [release-delivery-reconciliation-guard] OK (G101: all required features delivered + validate-certified)
  ```

  Added as section **V8**, following the V7 pattern already established in this packet.

  The binding is deliberately `delivery=optional`, not `required`, and this is load-bearing rather
  than a shortcut. Gate G101 requires every `delivery=required` feature to bind a TERMINAL,
  validate-certified spec; spec 108 is `in_progress`, so binding it as required would make the
  guard refuse the packet — the row would claim a delivery that has not happened. The machine-binding
  comment records that it flips to `required` when 108 reaches a terminal certified state. The guard
  was re-run after the edit and still exits 0.

  Status language is honest about the split: the gate, metrics, sixteen-group manifest, and bridge
  derivation are IMPLEMENTED, while the capability as a whole is IN DELIVERY because the ENFORCE
  flip has not been authorised. Recording it as shipped would misrepresent a default-OFF flag as an
  active behaviour change.
- [x] `config/release-trains.yaml` verified — `next` train still resolves `flags_bundle: config/feature-flags.next.yaml`; no structural change made

  Command: `grep -nE 'flags_bundle:' config/release-trains.yaml && git diff --stat HEAD -- config/release-trains.yaml`
  Exit Code: 0
  Executed: yes — 2026-08-12, tree at `69963c34`

  ```text
  23:  flags_bundle: config/feature-flags.mvp.yaml
  29:  flags_bundle: config/feature-flags.next.yaml

  (git diff --stat against HEAD for config/release-trains.yaml: empty — no working-tree change)
  ```

  This is a verification row, not a change row, and both halves hold. The `next` train still
  resolves `config/feature-flags.next.yaml` at line 29, and the file is byte-unchanged in the
  working tree.

  The "no structural change" half is the point of the row. `corpusGrantEnforcement` was added to
  the existing bundles that this file already resolves, so the train topology did not have to move
  to accommodate the flag. A flag that required restructuring the train registry to land would be
  evidence the flag was scoped wrongly.

  Note that `config/release-trains.yaml` WAS edited earlier in this delivery — commit `d0d00d31`
  resolved a contradiction where `release-train-guard.sh` accepted `home-lab` while a Go contract
  test demanded `self-hosted`, so the two gates could never both pass. That change is committed and
  is not a working-tree modification; this row asserts no FURTHER structural change was made for
  the flag itself.
- [x] `config/feature-flags.next.yaml` sets `corpusGrantEnforcement: false` and `config/feature-flags.mvp.yaml` sets it `false` with the required `metadata:` block (**CORRECTED 2026-08-11 by `bubbles.implement` on explicit operator directive — this line previously required `true` for `next`; the correct value is `false` in BOTH bundles. Authority: `spec.md` R-108-FL3 plus `design.md` §4 and §9. Full rationale and the preserved prior wording are recorded immediately below the evidence.**)

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=eb7ad5a0
  **Executed:** YES
  **Command:** `grep`/`yq` resolution over both bundles, then `release-train-guard.sh`, `./smackerel.sh config generate`, `./smackerel.sh test unit --go`
  **Exit Code:** 0

  ```text
  $ grep -n 'corpusGrantEnforcement' config/feature-flags.next.yaml config/feature-flags.mvp.yaml
  config/feature-flags.next.yaml:18:  corpusGrantEnforcement: false
  config/feature-flags.mvp.yaml:14:  corpusGrantEnforcement: false
  config/feature-flags.mvp.yaml:24:  corpusGrantEnforcement:

  $ for b in mvp next; do echo "$b -> $(yq -r '.flags.corpusGrantEnforcement' config/feature-flags.$b.yaml)"; done
  mvp -> false
  next -> false

  $ yq -o=json -I=0 '.metadata.corpusGrantEnforcement' config/feature-flags.mvp.yaml
  {"owning_spec":"specs/108-corpus-grant-enforcement/","introduced_in_train":"next","introduced_at":"2026-08-11"}

  $ yq -r '.metadata.corpusGrantEnforcement' config/feature-flags.next.yaml   # metadata lives in mvp only, per the card_rewards/clientReleaseLaneB precedent
  null

  $ bash .github/bubbles/scripts/release-train-guard.sh .
  [release-train-guard] release-train-guard PASSED (2 trains)
  EXIT=0
  (302 lines total; ERROR lines: 0; lines matching 'G111': 0; the other 111 lines are
   pre-existing grandfather WARNs about unrelated specs missing releaseTrain, unchanged
   from the pre-edit baseline run.)

  $ ./smackerel.sh config generate
  config-validate: <repo-root>/config/generated/dev.env.tmp.3487941 OK
  Generated <repo-root>/config/generated/dev.env
  Generated <repo-root>/config/generated/nats.conf
  Generated <repo-root>/config/generated/prometheus.yml
  Generated <repo-root>/internal/experience/catalog.gen.json
  EXIT=0

  $ ./smackerel.sh test unit --go
  ok      github.com/smackerel/smackerel/internal/config  33.956s
  [go-unit] go test ./... finished OK
  EXIT=0
  (252 lines total; 145 `ok` packages, 0 FAIL, 13 "no test files".)
  ```

  **RED proof (captured before the edit):** `grep -n 'corpusGrantEnforcement' config/feature-flags.next.yaml config/feature-flags.mvp.yaml`
  returned **exit 1** — the flag was declared in neither bundle. The same grep now returns exit 0
  with three hits, so this item is proven by a state change, not by a tautology.

  R-108-FL2 (declared in **both** bundles) and R-108-FL4 (mvp metadata naming `owning_spec`,
  `introduced_in_train`, `introduced_at`) are both satisfied. Metadata is carried in `mvp.yaml`
  only, matching the existing `card_rewards` / `clientReleaseLaneB` shape in this repo — `next.yaml`
  keeps `metadata: {}`.

  > **DoD-TEXT CORRECTION — `next: true` → `next: false`. Recorded 2026-08-11 by `bubbles.implement`
  > on explicit operator directive; the prior text is preserved here rather than silently replaced.**
  > This item previously read *"sets `corpusGrantEnforcement: true` (owning train)"*. **That value was
  > wrong.** Every governing artifact records default-OFF in **both** bundles: `spec.md` **R-108-FL3**
  > ("ships **default-OFF (`false`) in every train**"), `design.md` **§4** Configuration table
  > (line 191), and `design.md` **§9** Documentation & Release Impact table (lines 358–359).
  > Those three are the authority; this DoD line was the sole dissenting statement in the packet.
  >
  > Why the corrected value is the safe one, not merely the compliant one: spec 108 is a two-stage
  > **OBSERVE→ENFORCE** rollout. Shipping `next: true` would arrive **already enforcing**, destroying
  > the observation window Scope 04 depends on and breaking live callers that have not yet received
  > the `corpus:read` token rotation `docs/smackerel.md` §17.2 requires (granting is a token rotation,
  > not a flag flip — F-108-GRANT-MECHANISM-01). Gate **G111** forbids default-ON on a **non-owning**
  > train but never requires ON *anywhere* (`release-train-guard.sh` Check 8), so an all-OFF dormant
  > flag is conformant. `bubbles.train` owns the later flip to `true`.
  >
  > **Residual inconsistency, NOT edited from here (routed to `bubbles.plan`):** `SCN-108-R01`,
  > `SCN-108-R05`, the `TP-05-01` Test Plan row, the Scope 05 Implementation Plan bullet for
  > `feature-flags.next.yaml`, and its "Divergence to reconcile" bullet all still assert
  > *default-ON in exactly one train*. Gherkin scenarios and Test Plan rows are `bubbles.plan`-owned;
  > `bubbles.implement` does not rewrite them.

  > **PLAN-OWNER RATIFICATION — recorded 2026-08-11 by `bubbles.plan`.** The
  > `bubbles.implement` correction above is **ratified as written**; no revision is required.
  > Independently re-verified before ratifying, rather than relayed: `spec.md:519` (R-108-FL3)
  > mandates default-OFF in **every** train; `design.md:191` (§4) and `design.md:358-359` (§9)
  > both record `false` in **both** bundles; and `release-train-guard.sh` Check 8 skips the
  > owning train at line 132 (`[[ "$tid" == "$spec_train" ]] && continue`) so line 138 can raise
  > `G111` **only** for a non-owning train — confirmed by a full guard run against the shipped
  > all-OFF bundles exiting **0** with **0** `G111` lines and **0** `ERROR` lines.
  >
  > The five residual inconsistencies this block routed to `bubbles.plan` — `SCN-108-R01`,
  > `SCN-108-R05`, the `TP-05-01` row, the `feature-flags.next.yaml` implementation-plan bullet,
  > and the "Divergence to reconcile" bullet — are now **reconciled**, together with four further
  > restatements of the same premise (the `TP-05-06` clause, two DoD items, and — outside this
  > scope — Phase Order item 5, the Scope 05 Validation Checkpoint row, and the top-level
  > "Planning Note — Flag Default Divergence", which was the premise's origin and is now marked
  > WITHDRAWN). See the **PLAN-TEXT CORRECTION** block at the head of this scope.
  >
  > **The `[x]` on this DoD item was set by `bubbles.implement` on its own executed evidence and
  > is left exactly as found.** `bubbles.plan` ratified the **wording**, not the tick.

- [x] Flag-default conflict resolved and recorded — **there is no `design.md` §4-vs-§9 divergence**: §4 (line 191) and §9 (lines 358–359) both record `false` in **both** bundles, and both transcribe `spec.md` R-108-FL3 correctly. The real conflict was **plan premise vs spec/design**, and it is resolved in favour of spec/design. `design.md` was NOT edited from this packet and `bubbles.design` owes no change. (**REWORDED 2026-08-11 by `bubbles.plan`.** Prior wording: *"design.md §4/§9 flag-default divergence reconciled by `bubbles.design` (or the enforced-policy value ratified by the operator) before this scope closes — not silently overwritten from this packet"*. That wording mislabelled the subject: it named a §4-vs-§9 divergence that does not exist and implied a `bubbles.design` dependency that never existed. Both were artifacts of the false "default-ON in exactly one owning train" premise — see the PLAN-TEXT CORRECTION block at the head of this scope. The `[x]` and the evidence block below are left as found; only the item's wording changed.)

  **Claim Source:** executed · **Tree:** WORKING TREE, HEAD=eb7ad5a0
  **Executed:** YES
  **Command:** `sed`/`grep` reading of `design.md` §4 and §9 and `spec.md` R-108-FL3
  **Exit Code:** 0

  ```text
  $ sed -n '186,192p' design.md   # §4 Configuration table
  ### Configuration
  | Layer | Name | Value |
  |---|---|---|
  | Train flag | `corpusGrantEnforcement` | `false` in `config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml` (default-OFF in every train, per R-108-FL3). |
  | SST key | `auth.corpus_grant_enforcement` in `config/smackerel.yaml` | Declared with **no default value**. |

  $ sed -n '358,359p' design.md   # §9 Documentation & Release Impact table
  | `config/feature-flags.next.yaml` | Add `corpusGrantEnforcement: false` with an owning-spec comment (R-108-CFG1). `bubbles.train` flips it to `true` only after a clean observation window. |
  | `config/feature-flags.mvp.yaml` | Add `corpusGrantEnforcement: false` plus the `metadata:` block recording `owning_spec: specs/108-corpus-grant-enforcement/`, `introduced_in_train: next`, `introduced_at` (R-108-CFG2, R-108-FL4). `mvp` observes-and-counts, never denies. |

  $ grep -n 'corpusGrantEnforcement' design.md
  191:| Train flag | `corpusGrantEnforcement` | `false` in ... (default-OFF in every train, per R-108-FL3). |
  272:1. Set the train flag `corpusGrantEnforcement` back to `false` in the owning bundle
  358:| `config/feature-flags.next.yaml` | Add `corpusGrantEnforcement: false` ... flips it to `true` only after a clean observation window. |
  359:| `config/feature-flags.mvp.yaml` | Add `corpusGrantEnforcement: false` plus the `metadata:` block ... |

  $ sed -n '519p' spec.md
  **R-108-FL3:** The flag ships **default-OFF (`false`) in every train**, mirroring the `clientReleaseLaneB` precedent ...
  ```

  **Reading: §4 and §9 AGREE — there is no §4-vs-§9 divergence, and `design.md` was NOT edited.**
  §4 line 191 says `false` in both bundles. §9 lines 358–359 say `false` in both bundles. §6 rollback
  (line 272) says `false` is the resting state. The single occurrence of `true` anywhere in
  `design.md` is line 358's *"`bubbles.train` flips it to `true` only after a clean observation
  window"* — a description of the future operator flip, **not** a shipped default.

  The finding's real subject was therefore mislabelled. It was never an internal `design.md`
  inconsistency; it was a conflict between `design.md` (which correctly transcribes R-108-FL3) and
  the planning assumption that *"the enforced release-train policy requires default-ON in exactly
  one owning train."* **That premise is false.** `release-train-guard.sh` Check 8 raises
  `G111 violation` only when a flag is default-ON in a train **other than** the owning one; it has
  no rule requiring ON anywhere. The post-edit guard run above confirms this empirically: both
  bundles are `false`, and the guard exits 0 with zero `G111` lines.

  Reconciliation therefore required **no change to `design.md`** and none was made. The operator
  ratified the enforced-policy value as `false` in both trains, and the correction was applied to
  the one artifact that was actually wrong — the preceding DoD line.

- [ ] `TP-05-01` unit test passes — flag declared in **both** bundles and default-OFF in **every** train, `mvp` metadata block present, and all three adversarial cases hold: absent-flag fixture REJECTED, non-owning-train (`mvp: true`) fixture REJECTED, shipped all-OFF shape ACCEPTED *(reworded 2026-08-11 by `bubbles.plan` to match the corrected `TP-05-01` row; prior wording read "flag default-ON in exactly one train, default-OFF elsewhere, metadata present")*
- [ ] `TP-05-02` unit test passes — SST key declared with no default; no fallback shape in the resolution path
- [ ] `TP-05-03` integration test passes — generated env carries the variable for every environment
- [ ] `TP-05-04` e2e-api test passes — the documented runbook query returns the documented shape against the real `/metrics` surface
- [ ] `TP-05-05` unit test passes — the `v1` release packet's `features.md` records the capability, its owning spec, its owning train, and its flag
- [ ] Flag lifecycle recorded (R-108-FL7): flag + observe branch + would-deny counters retire together, train + one cycle
- [ ] `TP-05-07` unit test passes — the flag-lifecycle documentation carries all four retirement clauses (train + one cycle; flag/observe-branch/counters deleted together; enforcement unconditional afterwards; `bubbles.train` owns the flip and the retirement), so `spec.md` §18 decision 6 is a testable obligation rather than an implied one (SCN-108-R05)
- [ ] `TP-05-06` regression e2e-api test passes — the permanent invariants are protected: flag declared in both bundles, `mvp` metadata intact, non-owning train never default-ON, default-free SST key, the executable runbook, and the release-packet entry *(reworded 2026-08-11 by `bubbles.plan` to match the corrected `TP-05-06` row; prior wording read "single-owning-train flag default")*
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression (`TP-05-06`, `./smackerel.sh test e2e`)
- [ ] Broader E2E regression suite passes — **Phase:** regression (`./smackerel.sh test e2e` exits 0; no previously-passing test regresses)
- [ ] Build Quality Gate: `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` clean with zero warnings; docs and config aligned; no TODO/stub/default introduced
