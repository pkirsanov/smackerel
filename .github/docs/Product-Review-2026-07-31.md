# Bubbles — Product Review and Convergence Roadmap

**Reviewed:** v7.21.0 · 2026-07-31 · **Type:** diagnostic + corrective roadmap · **Status:** PROPOSED, no framework file mutated

---

## 1. Verdict In One Paragraph

Bubbles solves a real, unserved problem — no competitor can refuse a false "done" — and it has already built the right mechanisms to do it. The failure is not design and not rigour. **It is last-mile attachment: nearly every core mechanism ships complete, installs everywhere, and is then used almost nowhere.** The state-transition guard is wired into 0 of 6 consumer repos. The receipt ledger is registered in 7 of 7 and holds 1 entry per repo across 1,786 packets. The control-plane fields newer gates depend on are absent from 42–55% of certified work. Consequently the headline claim — *"done is a mechanically enforced verdict, not a claim"* — is, in the field, a claim. The corrective program is therefore **activation, not construction**, which makes it far cheaper and faster than the size of the codebase suggests.

---

## 2. What The Product Is

| Dimension | Measured |
|---|---|
| Agents / prompts | 41 / 41 |
| Gates | 112 (73 businessInvariant · 24 modelCompensation · 15 hybrid) |
| Workflow modes | 15 primitives + 46 aliases |
| Scripts | 374 (190 selftests), 115,039 LOC bash |
| Governance prose | 49 shared docs · 12 instruction files · 44 skills · 74 recipes |
| CLI surface | 44 commands |
| Extensibility | adapters (observability, judge, codeindex) · MCP server (1,438 LOC, 12 tools, 7 resources) · project gates via `.github/bubbles-project.yaml` |
| Install footprint | 758 files / 11 MB per repo |
| Field use | 1,786 packets · 6 products · all pinned to 7.21.0 |

---

## 3. Is It The Right Problem?

**Yes — with one reframe needed.**

The stated premise is *"AI agents are untrustworthy by default."* That frames it as **model honesty**, which yields compensations that can never expire — and indeed 24 `modelCompensation` gates exist and **zero have ever retired**.

The durable framing is one level up:

> **An agentic loop produces no trustworthy completion signal.**

A systems problem, not a model problem. It does not dissolve as models improve; it intensifies, because a stronger model produces more convincing claims across more surface.

**Product definition that follows:**

> **Bubbles is the trust layer for agentic delivery. It turns an agent's claim into a receipt a human can verify in seconds.**

---

## 4. Competitive Position

| Product | Answers | Reach | Enforces "done"? |
|---|---|---|---|
| **Spec Kit** | what to build | 124.7k★ · 273 contributors · 30+ agent integrations | ❌ |
| **BMAD-METHOD** | who builds it | 51.3k★ · 160 contributors · 12 agents, 34 workflows | ❌ |
| **Bubbles** | **did it actually get built** | 1 maintainer · 1 *execution* platform (+ MCP serving, + inbound interop from 4) | ✅ by design |

**The edge is real and uncontested.** Both leaders are process/persona toolkits; neither can refuse a completion. Bubbles is the only one treating "done" as a verdict.

**The exposure is equally real.** Claude Code now ships `PreToolUse`, `Stop`, `SubagentStop`, **`TaskCompleted`**, `TeammateIdle` hooks with exit-code-2 blocking, plus prompt/agent hooks and enterprise `allowManagedHooksOnly`. Anthropic's own `TaskCompleted` example runs tests and blocks completion — Bubbles' core proposition as a platform primitive, enforced *inside the loop* where it cannot be skipped.

**The cross-platform story is inbound-only — that is the precise gap.** `install.sh` emits exclusively VS Code surfaces (`.github/agents|bubbles|instructions`); it produces no `.claude/`, `.cursor/`, or `.roo/` artifacts. Interop (`detect|import|apply`) runs the other way: it *imports* Claude Code, Cursor, Cline, and Roo Code config **into** Bubbles. The MCP server serves any MCP host and is registered 7/7. So Bubbles can absorb users from the incumbents and can be *queried* anywhere — but nothing emits an outbound execution surface. Step 6 closes exactly that.

**Strategic implication:** stop competing on process surface (41 agents / 62 modes / 74 recipes vs Spec Kit's 7 commands). Compete only on verifiable completion.

---

## 5. Strengths To Preserve

1. **Gate classification with retirement criteria** — `businessInvariant` vs `modelCompensation` + `retireWhen`. Encodes *why* a rule exists and what would remove it. Not present in any competitor.
2. **Institutional honesty** — the repo publishes its own disconfirmations in-tree (bundle premise "measured FALSE"; IMP-028 closed "Cost: real. Benefit: zero"; a retirement tool whose header states its measurement loop does not exist). Rare, and marketable.
3. **Adversarial regression requirement** — a bug fix's test must fail if the bug returns; tautological and silent-pass patterns mechanically detected.
4. **Artifact ownership + routing** — backed by a manifest and a blocking gate.
5. **Receipt architecture** — `record_evidence` → `tool-calls.jsonl` → consumed by the guard. Correct design, already built.
6. **Real dogfooding** — 1,786 packets across 6 heterogeneous stacks, single version.
7. **Migration on-ramp** — inbound interop from Claude Code, Cursor, Cline, and Roo Code, review-gated, writing only into project-owned targets. A real path off the incumbents that neither competitor offers in reverse.
8. **Skills as a working delivery channel** — 44 skills (5,102 lines) whose descriptions load into agent context and whose bodies carry the actual rules, not pointers. This is the one governance channel that demonstrably reaches agents.

---

## 6. Weaknesses — All One Root Cause

Every weakness below is the same failure: **a complete mechanism with no forcing function.**

| # | Mechanism | Built? | Installed? | Used? |
|---|---|---|---|---|
| W1 | Certification guards (`state-transition-guard`, `artifact-lint`, `reality-scan`) | ✅ | **unattached in 6/6 repos by _any_ path** — not git hook, not project CLI, not CI | ~never |
| W2 | Receipt ledger (`record_evidence` → `tool-calls.jsonl`) | ✅ guard consumes it | MCP registered **7/7** | **1 entry** in 2/6 repos across 1,786 packets |
| W3 | v3 control plane (`createdAt` / `certifiedAt` / `certification.status`) | ✅ | — | missing on **55% / 42% / 18%** of 1,395 done packets |
| W4 | Gate retirement (`retireWhen` on all 24) | ✅ criteria declared | — | **0 retired**; metrics in 1/6 repos; harness does not exist |
| W5 | Shared governance prose (49 docs, 7,369 lines) | ✅ authoritative | linked, not inlined | **provably not read** — confirmed for `bubbles.goal` and `bubbles.workflow` |
| W6 | Skill↔module fidelity (44 skills restate the rules agents actually follow) | ✅ skills reach agents | — | **nothing verifies them against the authoritative docs** |

**On W1 — what "unattached" means precisely.** Bubbles-managed *git hooks* are **framework-source-only by design**: `doctor` states a downstream repo without them is "reported, never flagged." That is a deliberate boundary, not a defect. The defect is that downstream repos never attached the guards through the surface they *do* own either. Measured across all six: the only Bubbles scripts in any enforcement path are `pii-scan` (3 repos, git hook) and `macos-portability-guard` (2 repos, CLI). wanderaide's sole `state-transition-guard` reference is a one-off Python script outside any hook. **Zero repos invoke the certification guards anywhere.**

**On W5/W6 — the de facto governance is the unguarded one.** Two channels carry the same rules. The 7,369 lines of shared docs are authoritative but unreachable. The 5,102 lines of skills are reachable — descriptions sit in agent context, bodies restate the rules in full — and the framework built this deliberately (v4.0+ "Skills-First Discovery Layer"). So skills are what agents actually follow. Yet `bubbles/scripts/` contains `skill-description-load.sh` and `skill-evolution.sh` and **no consistency check whatsoever**: if a skill's restatement drifts from its authoritative module, agents follow the drifted skill and no gate notices. The rules with the most reach have the least verification.

**Consequences, measured:**

- **9 of 12** sampled `done` packets in guestHost fail a check that has existed since **day one** — certified without running the specialist chain their own declared mode requires. The gate is correct; it was never in the path.
- The guard silently falls back to the markdown evidence rail when no ledger exists, so the ≥10-line heuristic stays load-bearing and fabrication detection remains **heuristic instead of structural**.
- 31% of gates (35/112) are `mode-required` only — prose, no script.
- Governance-to-product byte ratio reaches **3.32×** (knb) and **2.95×** (research-lab).

**Why W2 has near-zero adoption — the precise chain:**

1. `tool-log.sh` exists and works.
2. It is **absent from the 44-command CLI surface** — an agent obeying terminal-discipline rules never encounters it.
3. It is documented in `evidence-rules.md`, `validation-profiles.md`, `feature-templates.md`, `project-config-contract.md` — **all behind markdown links agents provably do not read.**
4. Only **1 of 41** agent definitions mentions it inline.

---

## 7. Missing Features

| Gap | Impact |
|---|---|
| **Outcome measurement harness** | 190 selftests measure conformance; 13 golden tasks score artifacts and never invoke a model. Nothing can answer *"does this governance help?"* — so no gate can retire. The `judge` adapter (`ollama.sh`) is the seed of this and is unused for it. |
| **A single "why is this not done?" answer** | `dod`, `blocked`, `lint`, `guard`, `scan`, `audit-done` each answer a fragment across 44 commands. No consolidated verdict. |
| **Enforcement-rank declaration per rule** | Nothing records whether a rule *can* be mechanically enforced, so prose accumulates silently. |
| **Adoption telemetry** | The framework cannot see that its own mechanisms are unused. Every W-finding above required manual archaeology. |
| **Skill↔module fidelity check** | 44 skills restate the authoritative rules and are the channel agents actually read; nothing compares the two. A silent drift changes agent behaviour with zero signal. |
| **Outbound execution surfaces** | Interop is inbound-only and MCP is query-only; no target emits runnable agent/hook surfaces for Claude Code, Cursor, Cline, or Roo. |

---

## 8. User Scenarios — Current vs Target

| Scenario | Today | Target |
|---|---|---|
| "Is this feature actually done?" | Run `guard`, read long check output, hope hooks ran | `bubbles why <spec>` — verdict + exact missing condition + next command, <2s |
| "Prove this test passed" | Paste ≥10 lines of terminal output; guard applies fabrication heuristics | Receipt ID resolves to argv + exit code + output hash; fabrication structurally impossible |
| "Agent marks work done" | Agent writes `status: done`; nothing necessarily checks | Agent *proposes*; only the deriver *grants*; unearned completion blocked in-loop |
| "Onboard a new repo" | 758 files, then guards silently never invoked | One CLI; `doctor` reports `UNATTACHED` until a repo-owned surface runs them |
| "Is the framework worth its cost?" | Unanswerable | Per-gate trigger rate by model tier; gates retire on evidence |

---

## 9. Target State

**Core rule:** `done` is not written — it is **derived**. An agent may *propose* completion; only the deriver may *grant* it. A hand-edited `done` without a resolving derivation is rejected structurally, not heuristically.

**Core object:** the receipt that already exists — argv, exit code, duration, stdout/stderr hashes, spec, scope — append-only in `tool-calls.jsonl`.

**Core surface:**

```
$ bubbles why specs/042-catalog-assistant

  specs/042-catalog-assistant — NOT DONE (3 of 4 conditions met)

  ✓ all 6 scopes Done                          derived from artifacts
  ✓ every DoD item has a supporting receipt    31/31 receipts, exit 0
  ✓ delivery delta outside specs/              14 source files changed
  ✗ scope 04 DoD item 3 claims "e2e passes"    no receipt found

    Next:  bubbles run --spec specs/042 -- npm run test:e2e
```

**Enforcement ranking** — every rule declares the lowest rank that can hold it; rank 4–5 requires owner sign-off:

| Rank | Attachment | Skippable | Target |
|---|---|---|---|
| 1 | Runtime hook (`PreToolUse` / `Stop` / `TaskCompleted`) or MCP | No | majority |
| 2 | CI | No | high |
| 3 | Git hook | By policy only | all repos |
| 4 | Script the agent invokes | Yes | rare |
| 5 | Prose behind a link | Trivially | none |

**Target numbers:**

| Dimension | Today | Target |
|---|---|---|
| Framework gates | 112 | ≤ 20 |
| Prose-only gates | 35 (31%) | 0 |
| Certification guards attached (any repo-owned surface) | 0/6 | 6/6, or `doctor` fails |
| Receipts per delivered scope | ~0 | ≥ 1 per execution DoD item |
| Files installed per repo | 758 | ≤ 20 |
| `done` written by an agent | always | never |
| Outcome measurement | none | per-gate rate by model tier |

---

## 10. Roadmap — Seven Steps

Ordered by dependency. **Each step delivers value alone and is worth doing even if the next never happens.** No step is complete without its exit test. Steps 1–3 activate mechanisms that already exist and carry most of the value.

### Step 1 · Attach the guard that already exists
**Value:** the 112 gates you already built begin firing. Highest value-per-effort change available.

Bubbles-managed git hooks are **framework-source-only** and must stay that way — do not install them downstream. Attach through the surface each repo owns instead:

- Publish one documented invocation each repo wires into its **own** pre-push path or CI (`wanderaide.sh test pre-push`, `quantitativefinance.sh dev lint`, a CI job — whatever that repo already runs).
- Ship a drop-in CI job template for repos without a suitable CLI hook.
- `bubbles doctor` gains an **attachment check**: does *any* repo-owned surface invoke the certification guards? Report `ATTACHED` / `UNATTACHED` per repo. Keep it advisory while adoption lands, then make `UNATTACHED` fail. This is distinct from the existing Hook Health section, which correctly reports source-only git hooks and must not start flagging downstream repos.

**Exit test:** for every consumer repo, a repo-owned surface invokes `state-transition-guard`; flipping a spec to `done` with an unchecked DoD item is refused by that repo's normal push path. `doctor` reports `ATTACHED` for all six.

### Step 2 · Make the receipt path the default evidence rail
**Value:** fabrication becomes structurally impossible rather than heuristically detected. Retires the ≥10-line guessing game.

- Expose `tool-log.sh` as a first-class CLI verb: `bubbles run --spec <s> -- <cmd>`.
- Carry it on a channel that reaches agents — a skill and/or the agent bodies themselves. A linked shared doc does not count (W5).
- Guard **prefers** receipts; the markdown rail becomes an explicit fallback, reported as degraded.

**Exit test:** a delivered scope produces ≥1 receipt per execution DoD item; a DoD item asserting an execution outcome with no receipt is refused; detection does not regress on a held-out known-bad set.

### Step 3 · Publish the certification-debt number
**Value:** the operator learns, for the first time, what share of "done" is real. The number is the product — nothing is auto-fixed.

- Run guard + lint across all 1,395 `done` packets; classify each failure as `under-certified`, `unrecorded`, or `missing-control-plane`.
- Backfill `createdAt` / `certifiedAt` from git history where derivable; label the rest `legacy-unverifiable` — honest, not silently passing.

**Exit test:** the report reproduces the measured baseline (9/12 guestHost sample; 764 / 592 / 256 missing fields) from one command, and becomes a tracked metric that may only decrease.

### Step 4 · Derive the verdict
**Value:** eliminates the entire failure class this review found. An agent can no longer write `done`.

- `bubbles certify <spec>` derives status from receipts + artifacts and writes a derivation record.
- Agents write `proposedStatus`; the guard rejects any `status: done` lacking a derivation whose receipts still resolve.

**Exit test:** manually setting `status: done` exits 1 naming the missing derivation.

### Step 5 · Collapse the gate set — and the second prose channel
**Value:** faster validation, smaller install, far less to read and maintain — and the rules agents actually follow finally become the verified ones.

Classify all 112 gates as **ledger-derivable** (delete script, keep query) · **project-specific** (move to `.github/bubbles-project.yaml`) · **taste** (delete) · **irreducible** (keep, target ≤ 20).

Resolve W6 in the same pass. Each skill declares the authoritative module it restates; a `skill-fidelity` check fails when a skill asserts a rule its module does not contain, or omits one the module marks non-negotiable. Where a shared doc exists only to be restated by a skill, collapse the two — one source, one reachable channel.

**Exit test:** ≤ 20 framework gates; `framework-validate` wall-clock measurably lower; **zero detection loss** replaying the Step 3 corpus — that clause is the safety property, do not proceed without it. Every governance-critical skill names its module and passes `skill-fidelity`; a deliberately drifted skill fails it.

### Step 6 · Attach at the loop, on more than one platform
**Value:** enforcement moves inside the agentic loop where it cannot be skipped, and Bubbles stops depending on the one platform lacking the primitive.

Emit the ≤20 gates from one source into three targets: Claude Code hooks (`PreToolUse` for risk class, `Stop`/`TaskCompleted` for completion), CI jobs, git hooks. Promote the existing MCP server to the primary cross-host attachment. This is the step that makes the platform story bidirectional: interop already *imports* from four competitors, but nothing yet *emits* a runnable surface back out.

**Exit test:** in a live session a `TaskCompleted` hook blocks an unearned completion and the reason reaches the model — same guard, same exit code, three attachment points, one source.

### Step 7 · Measure, then retire
**Value:** the retirement curve finally starts; the framework can answer whether its governance helps.

- Build the harness that drives a model at a declared tier across the corpus and aggregates per-gate trigger rates (reuse the `judge` adapter).
- Enable metrics in all 6 repos (currently 1).
- Retire the first `modelCompensation` gates meeting their declared criteria.
- Ship as a versioned CLI (≤20 files), retiring `.manifest` / `.checksums` / `framework-write-guard` as a category.

**Exit test:** a rate report exists for ≥1 tier over ≥50 runs; ≥1 gate retires on evidence; install footprint ≤20 files; a repo upgrades in one command with zero manifest reconciliation.

---

## 11. Final View — After Step 7

**Operator experience.** You describe an outcome. Agents work. When one claims completion, the runtime asks the ledger whether the claim is supported — inside the loop, before the turn can end. If not, the agent is told which claim lacks a receipt and keeps working. You are not consulted, because nothing ambiguous happened. When you want status, one command answers in under two seconds with the exact next command. When it says `DONE`, that is a derived verdict backed by receipts you can inspect individually.

**Engineering shape.** A versioned CLI of ~20 files. An append-only receipt ledger. ~20 irreducible gates expressed as queries over it, emitted to three attachment points from one source. Project rules live in projects. Load-bearing governance reaches agents through the skills channel — the one that demonstrably works — with a mechanical fidelity check binding each skill to its authoritative module; reference material is labelled as such and is much shorter.

**Economics.** Every remaining `modelCompensation` gate carries a measured failure rate and a retirement date. As models improve the framework gets **cheaper**, automatically, on evidence. The cost curve bends down for the first time. `businessInvariant` gates never move, because they never should.

**Position.** Spec Kit tells you what to build. BMAD tells you who builds it. Bubbles is the only thing that can tell you it actually got built — and prove it.

**Why drift stops.** Drift happened because gates were the only lever an unattached framework could pull, and pulling it was free. After Step 5 there is a ceiling; after Step 7 there is a price. Adding a gate costs you a gate, or a measurement.

---

## 12. Anti-Drift Contract

Adopt at Step 1; enforce from Step 5.

| Rule | Mechanism |
|---|---|
| Gate ceiling | hard cap — add one, retire one, or supply a measured rate |
| Enforcement rank declared per rule | rank 4–5 requires owner sign-off |
| No new gate while `doctor` fails anywhere | blocking check |
| Certification debt may only decrease | tracked metric from Step 3 |
| Every evidence rule is a ledger query | non-queryable rules rejected |
| `modelCompensation` gates are dated | evaluated against the packet's certification date, never retroactively |
| Load-bearing rules reach agents via a channel that works | skills (description-matched) or `applyTo: "**"` — never a bare markdown link |
| Every skill is bound to its authoritative module | mechanical fidelity check; a drifted restatement fails |
| **New mechanisms ship with an adoption metric** | a capability with no usage telemetry is not done |

The last row is the one that would have prevented every weakness in §6.

---

## 13. Evidence

**Claim Source:** executed.

Measured this session: repository-binding preflight; `git log -S` history traces on `state-transition-guard.sh` and `required-specialists.yaml`; `artifact-lint.sh` against 13 guestHost `done` packets (direct exit code `1`); `jq` field sweep across 1,395 `done` packets in 6 repos; hook-content inspection across 6 repos; `tool-calls.jsonl` and MCP registration inventory; `cli.sh --help` surface enumeration; MCP server and tool-descriptor inspection; `lint-budget`; `gate-retirement.sh`; `model-tier-advisory.sh retirement`; gate-registry counts; `generate-release-manifest.sh --check` (in a clean worktree); `pii-scan.sh`; `agnosticity-lint.sh`; `install.sh` emission-target enumeration; `interop-registry.yaml` source/applyTarget inventory; skill inventory with governance-coverage and line-count comparison against `agents/bubbles_shared/`; `bubbles/scripts/` search for any skill↔module consistency check (**none exists** — the basis for W6).

**Framework status:** `framework-validate` + `release-check` ran to completion via the pre-push hook on both commits in this session and passed; neither push used a bypass.

**Interpretation:** §1–§8 are analysis over those measurements. §9–§12 are proposals; none is implemented.
