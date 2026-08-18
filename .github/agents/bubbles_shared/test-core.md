# Test Core

Purpose: mandatory testing rules for `bubbles.test` and test-facing checks performed by other agents.

## Load By Default
- `critical-requirements.md`
- `test-core.md`
- `test-fidelity.md`
- `e2e-regression.md`
- `consumer-trace.md` when rename/removal work is in scope
- The scope entrypoint plus the tests and implementation under test

## Testing Responsibilities
- Tests validate planned behavior and user/consumer scenarios.
- Fix implementations when tests match the plan; only change planning artifacts before changing tests when the plan is wrong.
- Live-system test labels must match reality.
- Persistent scenario-specific E2E regression coverage is required for changed behavior.

## Required Test Checks
- No proxy tests for required behavior.
- No skip/xfail/disabled required tests.
- Red before green for changed behavior.
- Regression verification after narrow fixes.
- Consumer-facing stale-reference checks for rename/removal work.
- When project config defines `testImpact`, use `bubbles/scripts/test-impact-plan.sh` to choose the narrow-first test order and always-run checks for changed paths, then still execute any required final broad suites.
- When project config defines `traceContracts`, preserve actual trace/log output for configured workflows so validation can run `bubbles/scripts/trace-contract-guard.sh` against evidence rather than predictions.

## Scenario Obligation Matrix (IMP-040 SCOPE-3 / COV-9; authoritative IMP-047 S-D)

A scenario can have a Test Plan row and still have no test of the behavior it
describes: a row proves that SOMETHING was planned, not that the user-visible
path was proven. Coverage is therefore derived from the scenario's BEHAVIOR
TRAITS, not from a row count.

**This matrix is AUTHORITATIVE.** Persistent scenario-specific regression
coverage stays UNIVERSAL — every changed behavior is protected by a test that
survives the change. What became proportionate is the physical test CATEGORY.
Where [critical-requirements.md](critical-requirements.md) and
[e2e-regression.md](e2e-regression.md) previously read as a universal E2E
obligation, they now defer here.

The rows below are a rendering of
[`bubbles/registry/proof-obligations.yaml`](../../bubbles/registry/proof-obligations.yaml),
which `scenario-obligation-lint.sh` reads and enforces under Gate G057. Edit the
registry, not this table.

For each active scenario, `bubbles.plan` records the obligations its traits
imply:

| Behavior trait | Required proof | Live proof |
|---|---|---|
| Pure calculation or validation | Production-unit assertion over transformed output | not required |
| User-visible UI | Visible or accessibility-tree assertion on the current production route | REQUIRED |
| API or wire contract | Real request and externally observable response | REQUIRED |
| Mutable state | Write, read, and persistence round trip | REQUIRED |
| Degraded or unavailable state | Named negative-path assertion with no plausible default | not required |
| Shared consumer or adapter | Producer-consumer parity plus current consumer-surface assertion | REQUIRED |
| Cache, provider, queue, or transport | Declared dependency-path state and live boundary assertion | REQUIRED |
| Responsive or accessible UI | Required viewport and accessibility behavior | REQUIRED |
| SLA-sensitive behavior | Stress or load assertion against the declared threshold | REQUIRED |
| Runtime configuration | Startup or runtime behavior executes the configured value | REQUIRED |
| Documentation, static metadata, non-runtime config | Production-unit or artifact assertion over the declared value | not required |

**Proportionate is not cheaper.** UI, API, mutable state, dependency boundaries
and SLA behavior pay MORE than under the universal-E2E wording: each owes a live
proof. Only pure logic, docs, static metadata and non-runtime configuration pay
less, and they pay less because a live shell around a pure function never proved
anything about the function.

**Synthetic complements, never replaces.** A synthetic fixture may accompany an
applicable live proof. It may not stand in for one. A declared mechanism that
reaches the behavior through a synthetic path where the trait requires a live one
is refused as `LIVE-PROOF-SUBSTITUTED`, and a live-owing trait with no declared
mechanism at all is refused as `LIVE-PROOF-UNDECLARED` — silence is exactly how
the expensive obligation used to get skipped.

**Runtime configuration gets no documentation exemption.** A configured value
that changes what the running system does is runtime behavior wearing a config
file's clothes. Asserting the file parses proves the parser. If the value never
reaches a running system it is `static-metadata` instead, and declaring the
weaker trait does not discharge the stronger one.

**A live proof may be declared not applicable only by NAMING the absent trait.**
`liveProofNotApplicable` carries `absentTrait` and `reason`. An unnamed exemption
is indistinguishable from an omission. An UNKNOWN trait requires review and can
never earn an exemption — a trait the framework does not recognise is a question,
not a discount.

**Derive, do not enumerate.** The matrix is applied per scenario from the traits
that scenario actually has. Attaching every row to every scenario is the failure
mode this replaces, not a safe default: an obligation set that is always the
same carries no information about the scenario, and it trains reviewers to skim
a block that never varies. A pure-calculation scenario owes a production-unit
assertion and nothing else.

**One trait can imply several obligations.** A scenario that renders a cached
value on a route is both "user-visible UI" and "cache/provider/transport", and
owes both proofs — the visible assertion does not discharge the boundary one.

**Migration.** Backfill traits conservatively. Old E2E links stay valid as
regression evidence while traits are backfilled; the lint is inert on a scenario
that declares no traits, so an unbackfilled packet is not retro-broken. Rollback
restores the previous policy text without deleting trait data.

## Test Mechanism Declaration (IMP-040 SCOPE-4 / COV-10)

A test's CATEGORY does not prove its PATH. An `e2e-ui` test can assert against
hidden legacy DOM, or call a render function directly, and still carry the
label. Both shapes bypass the current user path while reporting as end-to-end
coverage. The label states intent; `testMechanism` states mechanism, and unlike
prose it is checkable because the first four fields are closed vocabularies.

Test Plan rows that satisfy scenario coverage declare:

```json
{
  "entrypoint": "production-route",
  "inputOrigin": "synthetic-cache",
  "assertionSurface": "visible-ui",
  "dependencyPath": "cache-only",
  "productionOwners": ["path/to/owner"],
  "negativeControl": "wrong route or changed input fails"
}
```

| Field | Vocabulary |
|---|---|
| `entrypoint` | `production-route`, `production-api`, `production-cli`, `public-function`, `detached-renderer`, `internal-helper` |
| `inputOrigin` | `live-provider`, `ephemeral-real`, `seeded-store`, `synthetic-cache`, `synthetic-fixture`, `recorded-fixture` |
| `assertionSurface` | `visible-ui`, `accessibility-tree`, `http-response`, `persisted-state`, `returned-value`, `hidden-dom`, `internal-state` |
| `dependencyPath` | `not-applicable`, `ephemeral-real`, `same-origin-real`, `external-live`, `synthetic-boundary`, `cache-only` |

`productionOwners` names the production code that computes the asserted result,
as repository-relative paths plus optional symbols when a code-index adapter is
configured. `negativeControl` states the perturbation that makes the test fail:
a test with no stated perturbation has not been shown to be sensitive to what it
claims.

**What the declaration distinguishes.** Synthetic input may prove deterministic
business logic, and a seeded cache may prove cache consumption, but neither
proves live acquisition without a real boundary observation. Hidden DOM may
prove an internal projection but not a visible outcome. A detached renderer call
may prove a renderer unit but not route integration.

`test-mechanism-lint.sh` refuses a declaration that contradicts the scenario's
own `behaviorTraits`. It is inert on any scenario that declares no mechanism, so
it blocks from day one without retro-breaking an existing packet.

**Mixed tests are accepted.** The rules are coherence checks against the
declared trait, not a ban on internal observation. A pure-calculation scenario
asserting a returned value is correct. Only the shape where an internal surface
is offered as the SOLE proof of an external claim is refused.

## Production-Path Fidelity (IMP-040 SCOPE-5 / COV-10)

`regression-quality-guard.sh` rejects a test that reaches or observes the
behavior through an internal door while reporting as end-to-end. The refused
shapes are a detached render call, an internal-DOM or hidden-node read, and a
request interception that answers on the dependency's behalf.

**The rule is two-sided.** A shape is a finding only when the file offers it as
the SOLE proof. A file that also asserts the current visible surface — a role,
a visible locator, an accessibility snapshot — passes, because inspecting
internals *in addition to* proving the outcome is legitimate. A one-sided
version would reject every mixed test, and a guard that rejects correct work is
a guard teams switch off.

Projects extend the vocabularies through the existing configuration boundary:
`scans.regressionQuality.pathSubstitutionPatterns` and
`scans.regressionQuality.currentSurfacePatterns`.

**Not covered here, deliberately.** "A seeded value asserted unchanged after a
pass-through" is a tautology that cannot be recognised from syntax — deciding it
needs a judgement about whether production code transformed the value, and any
pattern guessing at it would fire on every legitimate round-trip assertion. That
case belongs to the non-vacuity work, where perturbing the input answers it by
experiment rather than by text matching.

## Dependency-Path Coverage (IMP-040 SCOPE-6 / COV-9, COV-10)

A cached read can satisfy a scenario about *rendering* a value. It cannot
satisfy a scenario about **freshness, fallback, retry, transport or delta**,
because those are claims about the boundary and a cache-only test never reaches
the boundary. When a scenario's title or tags name that kind of behavior, a
`cache-only` dependency path is refused: the test must observe the named
boundary.

For cache-first behavior the distinct cases are:

| Case token | Behavior |
|---|---|
| `fresh-no-fetch` | Fresh cache serves with no fetch |
| `stale-paints-before-delta` | Stale but meaningful cache paints before the delta completes |
| `missing-honestly-unavailable` | Missing cache stays honestly unavailable until data arrives |
| `malformed-rejected` | Malformed or rejected data |
| `delta-changes-result` | Delta completion changes the owning result |

A cache-first scenario names the cases it claims as `cache-case:<token>` in an
obligation's `satisfiedBy`. The vocabulary is closed, so a typo is a finding
rather than a silently uncounted case.

**Cases are named, not counted.** The requirement is that a cache-first scenario
declare the cases that apply, not that it declare all five. Demanding a fixed
count would report work that cannot arise for a given scenario, and a gate that
reports impossible work is one authors learn to wave through.

Live means the real validate-plane dependency, never production infrastructure;
environment-isolation rules continue to apply.

## Non-Vacuity and Mutation Proof (IMP-040 SCOPE-7 / COV-11)

Every new scenario contract owes one negative control: the perturbation that
makes the test fail. A test with no stated control has not been shown to be
sensitive to the behavior it claims to prove.

`riskTier` sets how strong that control must be:

| `riskTier` | Minimum `negativeControlMechanism` | Meaning |
|---|---|---|
| `low` | `adversarial-input` | Adversarial input or a missing selector proves the assertion fails |
| `medium` | `perturbed-input` | Perturb one input, require a specified output change or refusal |
| `high` | `mutation` | Bounded mutation against the owning branch or predicate |

The control must run through the production path, and it must not duplicate the
positive fixture under a renamed label — a `negativeControl` that restates the
scenario title verbatim is refused.

**Tier the scenarios that would actually hurt.** A uniform tier across every
scenario carries no information, the same way a uniform obligation set does.

**A project without mutation tooling is not blocked.** Mutation execution is a
project adapter (`mutationExecution:` in project config, default `none`), never
a hardcoded language runner — Bubbles supports eight languages and hardcoding
one engine would make the strongest control unreachable in most of them. A
high-risk scenario may declare a weaker mechanism when it also declares
`negativeControlFallbackReason`. The point is that a deliberate fallback stays
distinguishable from a silent downgrade; the resolver refuses a *broken* adapter
config loudly for the same reason, so a misspelling cannot buy that exemption.

## Shared-Consumer Parity (IMP-040 SCOPE-8 / COV-9, REG-8)

When a feature publishes through a shared adapter, shell, client, serializer or
renderer, **one proof is not enough**. A `shared-consumer` scenario owes both,
declared in its obligation's `satisfiedBy`:

| Prefix | Proof |
|---|---|
| `parity:` | Owner parity over the same input and policy |
| `consumer-surface:` | A test of the current externally observable consumer surface |

Parity shows the shared code produces the same result for the same input; it
says nothing about whether the surface a user actually meets still renders it.
Certifying either half alone is how a shared change passes while a downstream
surface is broken.

An attached hidden legacy node cannot substitute for the visible current
surface, and a manual renderer invocation cannot substitute for the route that
owns rendering — a `shared-consumer` scenario asserting `hidden-dom` /
`internal-state`, or entering through a `detached-renderer`, is refused.

The planner identifies the controlling code path, using the code-index adapter
when one is configured. Without an index it records explicit repository-relative
owner paths in `productionOwners`, which a shared-consumer scenario must have.

## Source-to-Scenario Impact (IMP-040 SCOPE-9 / REG-8)

Changed-spec validation asks which specs a diff touched, and answers it from
spec-folder paths. A diff that changes only SOURCE therefore looks like it
touched no spec at all, and every scenario certified against that source stays
certified on evidence that no longer describes the code. The blind spot is worst
where it matters most: a shared consumer, whose single edit invalidates
scenarios across many specs at once.

`implementationRefs` closes it. Each scenario records the code path that owns its
asserted result plus the consumer surfaces that render it. `scenario-impact-resolve.sh`
marks any CERTIFIED scenario whose refs intersect the diff for revalidation,
regardless of which spec folder the diff touched.

```bash
git diff --name-only <base>..HEAD \
  | bash bubbles/scripts/scenario-impact-resolve.sh specs/<NNN-feature> --changed-from -
```

A ref may name a file, a file plus a symbol (`path#symbol`), or a directory
(`path/`, matching everything beneath). Symbol suffixes are stripped before
comparison: without a code index the framework cannot tell which symbol a diff
touched, and pretending otherwise would UNDER-report — the failure direction
that leaves stale certification standing. **Over-reporting is the safe direction
here.** A scenario flagged unnecessarily costs a re-run; a scenario missed keeps
a false certification.

Only certified scenarios are flagged. An uncertified one has no claim to
invalidate.

**Ownership is declared, never inferred.** Refs come from the code-index adapter
when one is configured, and otherwise from explicit repository-relative paths the
planner records. The resolver matches only against those declared refs — it does
not guess ownership from a filename resemblance.

## Human Acceptance Is Terminal (IMP-040 SCOPE-10 / EV-8, Gate G136)

A terminal transition fails on **any** unchecked item in `uservalidation.md`.

Artifact lint requires the checklist to carry at least one checked `[x]` and
never rejects an unchecked one, so one checked plus five unchecked passes lint
and the spec reaches a terminal status with five behaviors no human accepted.

Lint is the wrong place to repair that. Lint also runs during **planning**, where
a checked-by-default template is legitimate — the template records what *will* be
accepted, it does not claim a human already ran it. Tightening lint would either
break planning or force it to fabricate acceptance up front. The terminal
transition is the moment the claim stops being provisional, so that is where the
check belongs.

The gate runs only when the target status is `done`. A ceiling-bound mode
(`validate-only`, `docs-only`, `spec-scope-hardening`, ...) is not claiming human
acceptance of delivered behavior, and an open checklist is the correct state for
it. Only the `## Checklist` section is parsed; a `[ ]` under `## Notes` is
ignored.

**The guard prints the item and never changes it.** Checking a box on the
author's behalf would fabricate exactly the human acceptance the gate exists to
require. Either a human accepts the behavior and checks it, or the item is a real
regression and the spec is not done.

## Changed-Spec Verification (IMP-040 SCOPE-11 / COV-12)

One generic command replaces per-repo reimplementations of "check the specs I
touched":

```bash
bubbles verify-changed-specs --base-ref <base-ref> [--head-ref <head-ref>]
```

It discovers **both** halves and runs the gates on each discovered spec —
artifact lint (G010), traceability and Test Plan parity (G088), and the scenario
contract checks (G057):

1. **Changed planning files** — spec directories the diff touched directly.
2. **Impacted certified scenarios** — specs the diff did *not* touch, whose
   `implementationRefs` intersect the changed source.

Half 2 is why the command exists. A source-only diff touches no spec folder, so
discovery built on spec paths alone reports nothing while certified scenarios go
stale.

**Wiring is the repository's job, and it is the part that actually fails.**
Measured across the six consumer repos on 2026-08-12: five carry a pre-push hook,
exactly one invokes any Bubbles guard, and one carries no hook at all. Gates that
are never reached are indistinguishable from gates that do not exist. The command
has no bypass flag; what varies between repos is whether it is invoked at all.

## References
- `evidence-rules.md`
- `state-gates.md`
