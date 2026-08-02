# Bubbles Product Review Snapshot

**Reviewed:** 2026-08-02 | **Repository:** Bubbles | **Baseline:** v7.22.0 at `725b641`
**Type:** standalone diagnostic and corrective roadmap | **Status:** analysis, not certification

## 1. Executive Verdict
Bubbles addresses a real engineering problem: agentic delivery produces persuasive completion claims without a dependable, inspectable link between intent, execution, source state, and outcome. The repository has unusually deep machinery for contracts, evidence, workflow state, validation, and certification.

The product is not yet a dependable assurance control plane. Its strongest mechanisms are incompletely attached, some evidence receipts are structurally inadmissible, several user-facing truth surfaces contradict runtime behavior, and the normal validation path is slow and operationally fragile. Dogfooding by one maintainer across six consumer repositories proves exposure to varied systems, not external demand or customer value.

v7.22.0 materially improves feature-surface reachability, macOS guard integrity, technical-prose discipline, and `doctor` documentation. Those are useful corrections. They do not close the assurance chain: surface reachability remains report-only, certification attachment is still absent from consumer workflows, and receipt admission remains weak.

The corrective verdict is neither "build more governance" nor "just wire up what exists." Bubbles needs a staged convergence: first make the current control path truthful and stable, then expose one mode-aware verdict, strengthen evidence in shadow, attach enforcement gradually, consolidate certification under the existing validate-owned authority, simplify by measured detection parity, and finally prove customer value and retire compensating controls.

## 2. What Bubbles Is Today
### Product inventory
| Surface | Current inventory at v7.22.0 / `725b641` |
|---|---:|
| Agent definitions / prompt shims | 41 / 41 |
| Registered gates | 112 |
| Workflow modes | 15 primitives + 46 persisted compatibility aliases = 61 modes |
| Shell scripts / selftests | 377 / 191 |
| Bash source size | 117,116 lines under `bubbles/scripts/` |
| Shared governance docs / instructions | 49 / 12 |
| Skill directories / recipes | 44 / 74 |
| Managed install files | 757 |

The system includes workflow routing, feature and bug artifacts, gate registries, evidence rules, mode-relative state, certification, release checks, installation and upgrade machinery, adapters, telemetry, selftests, and an MCP surface. This is a substantial assurance framework, not merely a prompt collection.

### MCP and execution surface
The MCP server exposes 12 tool descriptors, 5 static resources, and 2 resource templates. It is not query-only. In particular, `record_evidence` passes arbitrary caller-provided argv through a Bash twin, so it is an open-world, destructive execution capability.

The missing control is not outbound execution in general. It is native lifecycle interception plus server-side execution profiles and grants that distinguish query, evidence, and destructive execution privileges.

### Adoption and certification state
- MCP registration exists in all 7 inspected working copies: the source repository plus 6 consumers.
- Evidence ledgers exist in only 3 of those 7 copies: source, QuantitativeFinance, and Smackerel.
- Each existing ledger has 1 row, for 3 rows total. Of those 3 rows, 0 are simultaneously spec-bound, exit-zero, schema-key-admissible DoD evidence.
- The canonical `state-transition-guard` is attached to the normal push or CI path in 0 of 6 consumers.
- QuantitativeFinance contains an uncalled stale batch wrapper; other matches are one-off invocations or unrelated gates. The accurate attachment denominator is therefore canonical attachment `0/6`, not "nothing exists anywhere."

Across 1,395 packets marked done, `createdAt` is absent from 763 (54.7%), top-level `certifiedAt` from 733 (52.5%), and `certification.status` from 256 (18.4%). This is not solely downstream migration debt: the canonical v3 state template itself omits `createdAt` even though gates use it for grandfathering.

## 3. Is It Solving The Right Problem?
Yes, but the thesis should be corrected.

The weak thesis is that AI models are dishonest and therefore need more rules. That framing encourages permanent compensating gates and confuses model behavior with system assurance.

The stronger product thesis is:

> Agentic delivery lacks a trustworthy completion signal. Bubbles should bind declared intent, source state, execution, machine verdicts, and human acceptance into an inspectable assurance chain.

This reframes Bubbles as a control plane rather than a process methodology. It also sets a higher bar: a receipt can prove that a command ran in a bound environment, but neither a receipt nor a gate alone proves that the product behavior is correct or valuable.

## 4. Intended Users And Jobs
### Intended users
The likely users are lead maintainers and small platform teams coordinating multiple coding agents across several repositories, especially where false closure, hidden rework, or inconsistent controls are expensive.

The beachhead and market demand are not yet validated. One maintainer using Bubbles across six repositories is meaningful dogfooding, but it is not evidence that another team will adopt, retain, or pay for the product.

### Jobs and scenarios
- Determine whether a feature is complete for its declared workflow mode.
- Explain exactly why a completion claim is blocked without reading many scripts and artifacts.
- Bind test or build execution to a repository, tree state, environment, and claim.
- Apply the same assurance semantics through CLI, CI, MCP, editor, and agent lifecycle hooks.
- Separate machine-derived assurance from a human decision that the delivered product is acceptable.
- Measure whether controls reduce false closure and rework enough to justify their cost.

### First segment to test
Recruit lead maintainers or small platform teams already using multiple coding agents across several repositories and for whom false completion or repeated remediation is costly. Measure:

1. time-to-trusted-verdict;
2. false closure rate;
3. rework after an agent reports completion;
4. onboarding time;
5. enforcement abandonment;
6. false-positive rate.

## 5. Competitive Position
Official sources were freshly checked by the analyst on 2026-08-02. External product claims are mutable and should be read as dated observations.

| Product | Officially presented focus | Implication for Bubbles |
|---|---|---|
| GitHub Spec Kit | Spec-driven development toolkit and workflow | Strong intent and planning surface; not evidence that Bubbles owns completion assurance exclusively |
| BMAD Method | Agentic agile method with specialized roles and workflows | Strong process and orchestration comparison; Bubbles should not compete on agent or workflow count |
| Claude Code hooks | Lifecycle hooks that can run commands and influence or block behavior | Demonstrates that in-loop enforcement is becoming a host capability, not a Bubbles-only primitive |

Bubbles' differentiation is a hypothesis, not an established exclusive claim: it may offer unusually deep cross-artifact assurance and certification across specifications, scenarios, evidence, workflow state, and release readiness. That hypothesis must be tested through adoption and outcome metrics, not asserted from repository size.

Official sources:

- https://github.com/github/spec-kit
- https://github.com/bmad-code-org/BMAD-METHOD
- https://code.claude.com/docs/en/hooks

## 6. Strengths To Preserve
1. A rich control model linking artifacts, scenarios, gates, evidence, modes, and certification.
2. Clear certification ownership: `bubbles.validate` is the sole certification writer.
3. Mode-relative terminality rather than a single literal status for every workflow.
4. Adversarial selftests and explicit attention to false-positive and anti-fabrication failures.
5. Gate metadata, including 24 `modelCompensation` gates with `retireWhen` criteria.
6. Modular source organization, adapters, an MCP server, and portable downstream installation.
7. Extensive dogfooding across heterogeneous repositories, while keeping its market meaning modest.
8. A willingness to encode uncomfortable findings and inspect the framework's own claims.

## 7. Weaknesses And Root Causes
There is no single root cause. At least six distinct failure classes must be addressed.

### 7.1 Assurance integrity
Receipt composition is not yet trustworthy enough for certification. `tool-log` emits `inputClosure`, but `tool-call.schema.json` is closed and omits that property, so schema-aware admission rejects closure receipts. DoD matching accepts only two or more overlapping tokens, which is too weak for exact claim binding.

The JSONL ledger is mutable and unsigned. Session, spec, scope, agent, and closure fields are caller controlled. The recorded command is a lossy joined string, output hashes do not expose retained inspectable content, and strict freshness rejects stale evidence but not evidence whose freshness is unknown. Receipts improve reviewability; they do not make fabrication impossible or prove behavior correctness.

The existing `assurance-derive.sh` maps caller-supplied booleans to assurance and status. `assurance-certification-check` checks internal consistency, while absent assurance blocks remain backward-compatible. The authority exists, but the facts are not yet derived from admitted evidence and artifacts strongly enough.

### 7.2 Attachment and adoption
Canonical state-transition enforcement is absent from all 6 consumer normal push or CI paths. MCP is registered everywhere, yet only 3 ledgers exist and none of their 3 total rows qualifies as admissible DoD evidence. Installation and capability existence are being mistaken for use.

v7.22.0 adds an Exposure Contract and `surface-reachability-guard.sh`. This is valuable product-surface accounting, but the guard is explicitly report-only and answers a different question: whether shipped value names and reaches a caller surface. It neither attaches nor executes certification guards in consumer workflows. Step 4 should reuse its declared-surface denominator and rollout pattern without treating it as certification enforcement.

The framework needs generated, reusable, repository-owned attachment plus an explicit doctor state. It must adopt advisory-first rollout and measure false positives before blocking all consumers.

### 7.3 Usability and truth drift
The source-repository `status` and `blocked` CLI paths still mirror mutable top-level state rather than explaining derived certification truth, so they can emit contradictory false-green output. v7.22.0 corrects `doctor` hook documentation, but the status-mirror remedy is only a proposal in IMP-032. README still recommends `bubbles hooks install --all` without limiting it to the framework source repo, conflicting with the scoped framework-ops recipe. Handoff documentation implies stale packet or fresh-chat authority and persistence that the runtime does not provide. The MCP catalog lists 11 of 12 tools and omits `check_observability`, while historical v6 MCP design text still says the server is not implemented.

These are not merely documentation defects. They show that users cannot reliably infer the current control state from the product's own surfaces.

Terminality also needs a unified explanation. A packet is complete-for-mode when its status is `done`, the mode's `statusCeiling`, or a `terminalAlias`. A single why surface must separately report complete-for-mode, achieved assurance, release readiness, and human acceptance.

### 7.4 Operational reliability and performance
Full pre-push runs `framework-validate`, then `release-check`, which runs `framework-validate` again. This is a composition defect, not just a large suite.

Measured telemetry reports framework validation p50 19m39s, p95 30m03s, and max 43m12s across 48 samples. Release check reports p50 23m33s and p95 34m42s across 10 samples.

The execution path also uses a machine-global fail-fast lock; stock macOS has no `flock`. BUG-006 records that pre-push releases this lock between `framework-validate` and `release-check`, so another run can enter and make the second phase fail. BUG-005 records transient full-suite selftest failures that pass standalone and remain root-cause unknown. Both are fail-closed, but they impose long false-failure cycles and erode trust in diagnostics. This review session also encountered repeated lock and ref races. Upgrades overwrite live files before verification and have no rollback transaction. The telemetry analyzer expects fields that producers do not emit.

### 7.5 Complexity and duplicate authority
The installed and contextual surface is large, but arbitrary gate or file-count caps would be the wrong remedy. The source should remain modular while packaging and discovery hide internal machinery from ordinary users.

Skills are lazy discovery and procedure shims. Authoritative policy remains in modules; skill bodies are not the de facto authority. The correct control is typed skills with stable rule IDs, valid authority links and anchors, and generated fragments where repetition is necessary. Natural-language semantic equivalence is not a reliable fidelity test.

### 7.6 Outcome and market measurement
All 24 model-compensation gates have retirement criteria, but zero have retired. The evaluation harness scores supplied outputs; it does not run a controlled model treatment and control over a shared task corpus. Consequently Bubbles cannot yet show causal reduction in false closure, rework, or time-to-trust.

External product and market evidence is not established. Dogfooding provides defect discovery and operational evidence, not beachhead validation.

## 8. Missing Features
- A mode-aware, single explain/verdict kernel with stable reason codes.
- Evidence receipt v3 with exact claim identifiers, canonical argv, environment identity, retained output, tamper evidence, and automatic closure.
- MCP profiles and grants for query, evidence, and destructive execution.
- Generated changed-spec attachment for repository-owned CI and command surfaces.
- Native lifecycle adapters that block unsupported completion without creating a second certification writer.
- Transactional staged upgrades, run-scoped logs, portable concurrent locking, and coherent telemetry fields.
- Field-defined certification-debt and adoption reporting.
- A controlled model-run treatment/control driver and retirement evidence loop.
- Measured packaging that reduces installed and context surface without flattening modular source.

## 9. Current Versus Target Scenarios
| User scenario | Current state | Target state |
|---|---|---|
| Is this work complete? | Multiple commands and potentially contradictory status | One mode-aware explanation with stable reasons |
| Did the required test run? | Markdown evidence or weakly bound mutable receipt | Host or CI attestation bound to exact claim and source state |
| Can an agent finish unsupported work? | Normal consumer paths often do not invoke the canonical guard | Lifecycle adapter blocks and returns the same reason code |
| Is the release ready? | Completion, assurance, and release state can blur together | Release readiness displayed separately |
| Does the user accept the product? | Often conflated with machine completion | Explicit human acceptance remains independent |
| Is enforcement worth its cost? | No controlled outcome measurement | Adoption, latency, false-positive, rework, and retirement dashboard |

## 10. Fundamental Versus Tactical Changes
### Fundamental
- Bind evidence to stable claims and source/environment identity.
- Use one deterministic verdict kernel and preserve validate as sole atomic certification writer.
- Intercept lifecycle completion through transport adapters with shared reason semantics.
- Separate machine assurance, complete-for-mode, release readiness, and human acceptance.
- Make measurement and held-out detection replay prerequisites for simplification and retirement.

### Tactical
- Correct status, setup, handoff, MCP catalog, and historical design truth drift.
- Repair the receipt schema mismatch and exact matching behavior.
- Remove duplicate full validation from the push composition.
- Introduce run-scoped logs, portable locking, telemetry schema parity, and staged upgrade rollback.
- Generate reusable changed-spec CI and doctor attachment checks.

## 11. Final Target State
Bubbles should become an assurance control plane where intent and outcome contracts link to stable claims; host- or CI-issued tamper-evident attestations bind execution to source and environment; one deterministic verdict kernel explains the result; and `bubbles.validate` atomically certifies it.

CLI, MCP, CI, VS Code, and Claude Code hooks should consume the same facts and reason codes while mapping them to transport-specific outputs and exit behavior. No adapter should become a parallel certification authority.

Machine-derived assurance must answer whether declared evidence and policy conditions are satisfied. Human product acceptance must separately answer whether the delivered behavior is useful, correct in context, and acceptable to ship. Neither result implies the other.

Measurement should drive packaging, simplification, and retirement so the product becomes cheaper as evidence improves rather than accumulating permanent controls.

## 12. Gradual Seven-Step Roadmap
### Step 1 - Truthful and stable control path
**Independent value:** removes known false-green, schema, duplication, concurrency, and upgrade hazards before wider enforcement.

**Actions:** define the threat model and status vocabulary; fix status, setup, handoff, MCP catalog, and stale MCP design text; repair the receipt schema mismatch; run full validation only once per push; use run-scoped logs and portable locks; stage upgrades and atomically switch only after verification.

**Exit criteria:** focused adversarial tests pass; one and only one full validation runs per push; fault injection proves upgrade rollback; concurrent checks pass on declared Linux and macOS hosts without lock or ref collision.

### Step 2 - One mode-aware explain surface with shadow telemetry
**Independent value:** gives operators one fast, truthful answer without changing certification ownership.

**Actions:** build a pure formatter and verdict kernel over current guards and validate facts; expose complete-for-mode, achieved assurance, release readiness, and human acceptance separately; publish field-defined certification debt and adoption telemetry in shadow.

**Exit criteria:** adversarial cases produce no false-green output; on the declared corpus and reference host, warm p95 is at most 2 seconds and cold p99 at most 5 seconds; every displayed field names its source and unknown state.

### Step 3 - Evidence receipt v3 in shadow
**Independent value:** produces inspectable, strongly bound evidence without invalidating existing evidence prematurely.

**Actions:** bind exact claim, `SCN-*`, and DoD IDs; record canonical argv, repository binding, session, tree, dirty state, environment, and toolchain identity; make closure automatic and mandatory; retain inspectable output references; add tamper evidence and concurrent append; define query, evidence, and execution MCP profiles and grants.

**Exit criteria:** 100% of shadow receipts pass their closed schema; known-bad receipts are detected; unknown active evidence fails closed; wrapper overhead stays below 5%; existing evidence remains authoritative until replay demonstrates parity.

### Step 4 - Pilot attachment
**Independent value:** normal repository workflows begin catching unearned transitions with bounded rollout risk.

**Actions:** generate a reusable changed-spec CI or repository-owned command; start advisory, then block in 1 repository, 3 repositories, and finally all 6; add explicit doctor attachment states and publish false-positive telemetry.

**Exit criteria:** canonical guard attachment is 6/6; one-spec p95 is at most 10 seconds and no-change p95 at most 3 seconds on the declared host; the measured false-positive budget is met; an adversarial unearned transition is blocked in every consumer.

### Step 5 - Single certification kernel and lifecycle adapters
**Independent value:** makes certification consistent across hosts while preserving one authority.

**Actions:** keep `bubbles.validate` as sole atomic writer; derive facts directly from artifacts, admitted receipts, audit results, and mode revision rather than caller booleans; let CLI, MCP, CI, VS Code, and Claude hooks share reason codes but map transport-specific exits; keep machine assurance separate from human acceptance.

**Exit criteria:** a lifecycle hook blocks unsupported completion and a valid completion succeeds on at least two declared hosts; every transport reports equivalent reasons; no transport writes certification independently.

### Step 6 - Simplify by measurement
**Independent value:** reduces routine cost and cognitive load without sacrificing defect detection.

**Actions:** normalize mode authority and generate aliases, specialist surfaces, and managed-doc fragments; hide internal agents from ordinary selection; make skills thin, typed shims with stable rule IDs and checked authority anchors; remove duplicate scans, prose, and predicates only after held-out replay.

**Exit criteria:** held-out detection shows zero loss; core validation p95 is at most 60 seconds; full validation p95 is at most 5 minutes and max at most 10 minutes on the declared machine; install bytes and time, context bytes, duplicate scans, and false positives all have measured budgets. No arbitrary gate or file-count cap is used.

### Step 7 - Prove value and retire
**Independent value:** establishes whether Bubbles helps external teams and starts reducing compensating controls on evidence.

**Actions:** run controlled model treatment and control over an external pilot corpus; report customer time-to-trusted-verdict, false closure, rework, onboarding, abandonment, and false positives; require 300 independent zero-trigger opportunities before inferring a rate below 1%; retain 100% held-out detection; publish adoption, latency, and value dashboards.

**Exit criteria:** external pilot results are reproducible; the opportunity denominator and treatment assignment are inspectable; the first qualifying `modelCompensation` gate is retired without held-out detection loss; the dashboard exposes both product value and control cost.

## 13. Anti-Drift Contract
1. One verdict kernel owns reason semantics; transport adapters only translate them.
2. `bubbles.validate` remains the sole certification writer.
3. Machine assurance, complete-for-mode, release readiness, and human acceptance remain separate fields.
4. Every load-bearing rule has a stable ID, typed authority link, valid anchor, and executable or explicitly human-owned enforcement class.
5. Skills remain thin discovery and procedure shims; generated fragments prevent duplicated policy prose.
6. Every new mechanism ships with adoption, latency, false-positive, and detection-replay instrumentation.
7. Unknown active evidence fails closed; legacy evidence has an explicit, measured compatibility policy.
8. Simplification requires held-out zero detection loss, not aesthetic preference or count targets.
9. Performance budgets name corpus, machine, cache state, and percentile.
10. Install and context reduction target bytes, time, duplicate scans, and discoverability while preserving modular source.
11. Upgrade changes are staged, verified, atomic, and rollback-capable before live files change.
12. External differentiation remains a tested hypothesis until customer evidence supports it.

## 14. Evidence And Limitations
### Independently rederived measurements
**Claim Source:** executed in the current review session, with source inventory and affected findings refreshed against v7.22.0 at `725b641`; raw command transcripts are not embedded in this standalone snapshot.

The session independently reran the product inventory, receipt schema and admission checks, the 1,395-packet state-field sweep, the MCP registration and ledger census, the canonical attachment scan, and related count-bearing inspections. These results support the diagnostic findings but are not portable raw evidence blocks.

### Repository-derived baseline measurements
**Claim Source:** interpreted from repository-resident telemetry and registry or state surfaces at v7.22.0 / `725b641`.
**Interpretation:** The validation latency percentiles and model-compensation retirement count are review findings derived from those sources; their raw derivation output is not reproduced here, so they are not certification evidence.

### Static code inspection
**Claim Source:** interpreted from current-session source inspection, refreshed for the v7.22.0 delta through `725b641`, with concrete implementation paths used to form the findings.
**Interpretation:** The inspected paths support structural findings and defect explanations; they do not independently prove runtime behavior correctness.

This category covers MCP capabilities, receipt composition and schema admission, validate-owned certification, mode-relative terminality, CLI and documentation truth drift, validation composition, locking and upgrade behavior, telemetry field mismatch, skill authority, and evaluation-harness scope. Static inspection demonstrates structure and defects; it does not prove runtime behavior correctness.

### Focused subagent tests
**Claim Source:** executed by the analyst's focused diagnostic probes; raw outputs are not reproduced in this standalone snapshot.

Those probes support narrow defect findings only. They are not evidence that the entire framework passes or that the roadmap works.

### Mutable external sources
**Claim Source:** official external pages fetched by the analyst on 2026-08-02.

The Spec Kit, BMAD Method, and Claude Code observations may change after that date. No star or contributor counts are used, and no exclusivity claim is inferred from the comparison.

### Limitations
- Full framework validation and the replacement snapshot's pre-push check were not run during this publication pass. No validation-pass claim is made.
- External product demand, willingness to adopt, willingness to pay, and causal outcome improvement remain unproven.
- The historical 9-of-12 GuestHost sample is omitted because its sample list and selection rule were not persisted, so it is not reproducible.
- Governance-to-product byte ratios are omitted because no stable, rederived definition was available.
- Roadmap targets are proposals. They become facts only after implementation and executed measurement.