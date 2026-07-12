# Feature: 079 Production Autonomous Supervisor

**Status:** in_progress (analyst-owned authoring; ceiling = `specs_hardened`)
**Workflow Mode:** `spec-scope-hardening`
**Release Train:** `next` (post-MVP capability; MUST NOT ship on `mvp`)
**Planning Only:** true — implementation deferred until operator ratifies the safety model in `uservalidation.md`.

**Owner Directive (2026-06-03):**
> "bubbles agent running/monitoring/fixing service" on the production self-hosted.

**Depends On (read-only):**
- Deploy adapter overlay at sibling repo `<deploy-adapter-repo>/<deployment-owner>/<product>/<target>/` (operator-private; supervisor runs OUTSIDE this repo's smackerel-core stack)
- `bubbles.upkeep` (calendar-driven hygiene) — supervisor is the event-driven complement, NOT a replacement
- `bubbles.goal` (convergence loop) — optionally invoked for proposed fixes
- Prometheus scrape config + container health + NATS lag metrics + Postgres slow-query log already published by the self-hosted stack

**Unblocks:** nothing — this spec sits at `in_progress` until operator review.

---

## 1. Problem Statement

The self-hosted Smackerel deployment is the operator's daily production. Today, anomalies (SLO burn, scheduler stalls, ML sidecar latency drift, NATS lag, error spikes) are detected only when the operator notices them — typically hours late. The operator wants a Bubbles-agent supervisor that:

- Continuously reads prod telemetry (read-only by default).
- Detects anomalies against declared SLOs and steady-state baselines.
- Files bugs under the owning spec's `bugs/BUG-NNN-*` folder with full reproduction evidence.
- Optionally invokes `bubbles.goal` to ship a fix, gated by an operator-issued capability token.

The risk profile is severe: a misbehaving supervisor on prod can become the noisy neighbor, corrupt the spec corpus with spurious bugs, push bad fixes, or exfiltrate user data. This spec captures the safety model the operator MUST ratify before any line of supervisor code is written.

---

## 2. Outcome Contract

**Intent:** A read-only-by-default supervisor process runs on the self-hosted host (outside the smackerel-core stack), watches declared SLOs + telemetry, and emits append-only evidence packets. Write actions (bug filing, fix dispatch) are gated by operator-issued capability tokens with explicit scope and TTL. The supervisor never modifies prod config, never touches the knb manifest, never pushes to git from prod, and never takes the stack down.

**Success Signal:** With the supervisor running for 30 consecutive days on a quiet self-hosted, the operator's `specs/<spec>/bugs/` directory contains zero spurious BUG-NNN folders, the supervisor's own resource footprint stays under its declared budget, and every real anomaly (verified post-hoc) produced a bug packet within its declared detection window.

**Hard Constraints:**
- Supervisor process MUST NOT share a Docker network, host PID namespace, or volume mount with `smackerel-core`, `smackerel-ml`, `postgres`, or `nats`.
- Supervisor MUST NOT hold any credential capable of mutating `config/smackerel.yaml`, `config/release-trains.yaml`, `config/feature-flags.*.yaml`, knb-side `manifest.yaml`, or the prod git remote.
- Bug filing and fix dispatch MUST require an unexpired operator-issued capability token bound to (scope, TTL, max-actions).
- Every supervisor action MUST be recorded in an append-only ledger; corrections are new entries, never edits.
- Supervisor MUST defer when an open spec already covers the symptom (read `specs/*/state.json` first; do not double-file).

**Failure Condition:** The supervisor produces more operator workload than it removes (spurious bugs, alert fatigue, fixes that conflict with planned work, or a single autonomous action that touches forbidden surfaces).

---

## 3. Product Principle Alignment

This spec is bound by `.github/instructions/product-principles.instructions.md` (ratified 2026-06-03; BLOCKING).

| Principle | Alignment | Evidence |
|-----------|-----------|----------|
| **6 — Invisible by Default, Felt Not Heard** | The supervisor's entire UX is anti-notification. It does NOT page the operator on every anomaly. It writes durable, queryable evidence; the operator pulls when they want to look. System-initiated notifications honor the < 3/week budget (per design doc §1.4) and MUST clear the actionability bar. A status-update prompt ("we filed N bugs this week") is FORBIDDEN. | SCN-079-A03 (quiet hours), §6 Decision Policy in design.md |
| **9 — Design for Restart, Not Perfection** | When the operator returns after time away, the supervisor MUST NOT present a backlog screen or an "X unread anomalies" counter. The default view is "ask the supervisor what mattered while you were away" — a single synthesized digest, not a queue. Unread counters that punish absence are FORBIDDEN. | SCN-079-A06 (capability token expiry → read-only continues), §4 in spec |
| 1 — Observe First, Ask Second | Supervisor infers anomalies from telemetry; never blocks on user input to classify. | §5 Decision Policy |
| 8 — Trust Through Transparency | Every supervisor-authored bug MUST include source-attributed evidence (Prometheus query, log excerpt with timestamp + container id, metric snapshot). | Hard Constraint above |

**Cross-product Principle 10 (QF Companion Boundary):** Not applicable — the supervisor never proposes financial actions.

---

## 4. Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Operator | Sole human owner of the self-hosted; reviews supervisor output and issues capability tokens | Reduce time-to-detect for prod anomalies without becoming a notification target | Issues/revokes capability tokens; merges proposed fixes; transitions this spec's status |
| Supervisor (default) | Read-only Bubbles-agent process running outside the smackerel-core stack | Detect, classify, and record anomalies | Read prod telemetry (Prometheus, container health, NATS lag, slow-query log); read `specs/*/state.json` + `bugs/`; write to its own append-only ledger |
| Supervisor (write-scoped) | Same process holding an unexpired capability token | File a bug; dispatch a `bubbles.goal` run for a specific anomaly | Bounded by token scope + TTL + max-actions; never gains shell, never gains git push, never gains config mutation |
| Existing `bubbles.upkeep` (Treena) | Calendar-driven hygiene agent | Run scheduled backup/restore-drill/BCDR/patch tasks | Distinct from supervisor; supervisor MUST NOT trigger upkeep tasks |

**Boundary with `bubbles.upkeep`:** `bubbles.upkeep` is calendar-driven (recurring hygiene on a fixed cadence — backup, restore-drill, BCDR, secret rotation, flag cleanup). The supervisor is event-driven (anomaly detection on continuous telemetry). They MUST NOT overlap: upkeep does not consume real-time telemetry; supervisor does not run scheduled tasks. If both want to act on the same surface, upkeep wins by precedence and supervisor defers.

---

## 5. Use Cases

### UC-079-001: Detect Anomaly and File Bug (write-scoped)
- **Actor:** Supervisor (write-scoped)
- **Preconditions:** Operator-issued capability token unexpired with scope `bug-file`, max-actions ≥ 1; anomaly detected against a declared SLO; no open spec covers the symptom.
- **Main Flow:**
  1. Supervisor classifies the anomaly (rule-based + telemetry signatures).
  2. Supervisor reads `specs/*/state.json` to find the owning spec and confirm no open coverage.
  3. Supervisor authors `specs/<owning-spec>/bugs/BUG-NNN-<slug>/` with reproduction evidence (Prometheus snapshot, log excerpt, metric chart, classification rule id).
  4. Supervisor decrements the token's max-actions counter and writes the action to its append-only ledger.
- **Postconditions:** Bug folder exists; token counter decremented; ledger entry appended; no notification sent (operator pulls when ready).

### UC-079-002: Detect Anomaly, No Token → Read-Only Path
- **Actor:** Supervisor (default)
- **Main Flow:** Supervisor detects anomaly → writes to its own ledger → DOES NOT file a bug → DOES NOT notify. Operator discovers it next time they check the supervisor's read-only digest surface.
- **Postconditions:** Zero foreign-artifact mutation; one ledger entry.

### UC-079-003: Supervisor Defers to Active Spec
- **Actor:** Supervisor
- **Main Flow:** Anomaly detected → supervisor scans `specs/*/state.json` for `status ∈ {in_progress, specs_hardened}` whose `## Outcome Contract` or scenario manifest covers the symptom → if found, supervisor records a deference ledger entry pointing to the active spec and takes NO action.
- **Postconditions:** No duplicate bug; ledger entry preserves the deference decision.

---

## 6. Business Scenarios (Gherkin)

### SCN-079-A01: Anomaly detected with valid write token → bug filed
```gherkin
Given the supervisor holds an unexpired capability token with scope "bug-file" and max-actions=3
And Prometheus shows core_http_5xx rate > SLO burn-rate threshold for 10 consecutive minutes
And no open spec under specs/*/state.json covers the symptom
When the supervisor's classification loop fires
Then a new folder specs/<owning-spec>/bugs/BUG-NNN-<slug>/ is created containing spec.md, scenario-manifest.json placeholder, and an evidence/ subdir with the Prometheus snapshot + log excerpt
And the supervisor's append-only ledger gains one entry with action=bug-file, token-id=<id>, remaining-actions=2
And no notification is sent to the operator
```

### SCN-079-A02: Anomaly detected without write token → read-only only
```gherkin
Given the supervisor has no capability token (or all tokens expired)
And an SLO burn-rate anomaly is detected
When the classification loop fires
Then the supervisor writes one ledger entry with action=detect-only, reason="no-write-token"
And no bug folder is created
And no file outside the supervisor's own ledger surface is modified
```

### SCN-079-A03: Supervisor respects operator quiet hours
```gherkin
Given the operator has declared quiet-hours 22:00-08:00 local in supervisor config
And an anomaly is detected at 23:30 local that does NOT trip the "critical" classification rule
When the classification loop completes
Then no bug is filed during quiet hours regardless of token state
And the anomaly is queued for re-evaluation at 08:00 with the original detection timestamp preserved in the ledger
And no notification is sent
```

### SCN-079-A04: Supervisor defers to an active spec
```gherkin
Given an open spec specs/NNN-<name>/state.json has status="in_progress"
And that spec's scenario-manifest.json includes a scenario covering "ML sidecar latency drift > 500ms p99"
When the supervisor detects ML sidecar p99 latency > 500ms for 5 minutes
Then the supervisor writes a ledger entry action=defer, target-spec=specs/NNN-<name>
And no bug is filed
And no fix is dispatched
```

### SCN-079-A05: Supervisor detects its own noise and self-throttles
```gherkin
Given the supervisor has filed N>=3 bugs against the same symptom-signature within a rolling 24h window
When the supervisor evaluates the next matching anomaly
Then the supervisor self-throttles: writes a ledger entry action=self-throttle, signature=<sig>, suppressed-until=<ts+24h>
And no further bug is filed against that signature for 24h
And the supervisor's own resource budget (CPU, memory, disk-writes) remains under the declared cap recorded in supervisor config
```

### SCN-079-A06: Capability token expiry revokes write access mid-flight
```gherkin
Given the supervisor holds a capability token with TTL expiring at T
And at T+1s the supervisor's classification loop produces a candidate bug
When the supervisor attempts to write the bug folder
Then the write is refused before any file is created
And the supervisor writes one ledger entry action=write-refused, reason="token-expired", token-id=<id>
And the supervisor continues running in read-only mode
And no partial bug folder is left on disk
```

---

## 7. Competitive Analysis

| Capability | Smackerel supervisor (proposed) | PagerDuty / Opsgenie | Datadog Watchdog | Sentry alerts |
|-----------|---------------------------------|-----------------------|-------------------|----------------|
| Detection | Telemetry + Bubbles-agent classification | Rule-based + on-call rotation | ML anomaly detection | Error stream rules |
| Response | File bugs into spec corpus; optional fix dispatch | Page humans | Page humans + dashboards | Page humans |
| Trust boundary | Read-only by default; capability-token-gated writes | Full integration access | Full read of metrics; no remediation | Read errors; no remediation |
| Operator-felt load | < 3 notifications/week target (Principle 6) | High (paging) | Medium (configurable) | Medium |
| Remediation | Bubbles convergence loop (proposal-only, human-merge) | None (humans fix) | None | None |

**Edge:** No incumbent files bugs into a spec-corpus structure with full Bubbles evidence, and none offer optional convergence-loop fix proposals with operator-controlled merge. The capability-token write boundary is also distinctive.

---

## 8. Platform Direction & Market Trends

### Industry Trends
| Trend | Status | Relevance | Impact on Product |
|-------|--------|-----------|-------------------|
| LLM-driven SRE / "AI Ops" agents | Growing | High | Validates the supervisor pattern; competitors are headed toward proposal-only auto-remediation |
| Capability-based agent security (least-privilege tokens with TTL + scope) | Emerging | High | Aligns with the spec's safety model; supervisor is a poster child for the pattern |
| Append-only audit ledgers for autonomous actions | Established (SOX/PCI) | Medium | Operator hygiene; required for after-the-fact review |
| Quiet-hours / digest-style operator UX (anti-paging) | Growing | High | Direct Principle 6 alignment |

### Strategic Opportunities
| Opportunity | Type | Priority | Rationale |
|-------------|------|----------|-----------|
| Promote supervisor pattern to bubbles framework foundation (capability-first design) | Differentiator | High | Sibling products (guesthost, wanderaide, quantitativeFinance) share the same anti-paging operator profile; a shared `bubbles.supervisor` agent would amortize the safety-model investment |
| Per-spec "open coverage" index queryable by the supervisor | Table Stakes | High | Without it, deference logic (SCN-079-A04) cannot work; adjacent value for `bubbles.plan` and `bubbles.status` |

**Cross-Product Reuse Decision (Capability-First, AN5):** The supervisor pattern is GENERAL. Spec 079 is a *product-specific instance* whose safety-model ratification will become the input to a follow-up framework-level promotion. Capability primitives (capability-token issuance, append-only ledger, deference-to-active-spec query, quiet-hours engine) MUST be authored with no Smackerel-specific assumptions so they can be lifted into `bubbles/` without rework. See `### Domain Capability Model` below.

---

## 9. Domain Capability Model (AN5)

The supervisor introduces a new capability that two or more products (Smackerel, guesthost, wanderaide, QF) will want. Define the primitives provider-neutrally.

**Domain primitives:**
- `TelemetrySource` — read-only stream of metrics, logs, container health, or query stats. Lifecycle: `connected | degraded | disconnected`.
- `AnomalyClassification` — `(signature, severity, source-ref, evidence-bundle, detected-at)`. Lifecycle: `candidate → confirmed → deferred | filed | suppressed`.
- `CapabilityToken` — `(id, scope, ttl, max-actions, issued-by, issued-at)`. Lifecycle: `active → exhausted | expired | revoked`.
- `SupervisorLedgerEntry` — append-only `(ts, action, signature, token-id?, target-ref?, reason?)`. No update lifecycle (immutable).
- `OpenCoverageIndex` — read-only view over active specs/bugs answering "is symptom S already owned by an open artifact?"

**Relationships:** `TelemetrySource → AnomalyClassification → {SupervisorLedgerEntry, BugProposal?}`; every write transition consumes `CapabilityToken`; deference consults `OpenCoverageIndex`.

**Business policies every concrete implementation MUST obey:**
- Default state holds zero write capability.
- Tokens are bounded by all three of {scope, TTL, max-actions}; absence of any bound is a config error and the token MUST be refused at load.
- Ledger is append-only; corrections are new entries pointing to the original.
- Deference precedence: open-spec coverage > self-throttle > quiet-hours > write.

**Provider-/screen-/class-neutral behavior vocabulary:** `detect`, `classify`, `defer`, `file`, `dispatch`, `revoke`, `throttle`, `quiesce`.

---

## 10. Non-Functional Requirements

- **Resource budget (self-imposed):** supervisor process MUST stay under operator-declared CPU/memory/disk-write caps; SCN-079-A05 trips if breached.
- **Detection window:** declared per anomaly class in supervisor config; default 10 minutes for SLO-burn class.
- **Notification budget:** < 3 system-initiated notifications per week (Principle 6, design doc §1.4).
- **Isolation:** supervisor runs in a separate container/host (NOT inside `smackerel-core` stack — co-location breaks fault isolation).
- **Reversibility:** every supervisor write is reversible: bug folders are operator-deletable; ledger entries can be corrected by new appends.
- **Telemetry-only access:** supervisor process has zero credentials capable of mutating `config/`, knb manifest, or git remote.

---

## 11. Operator Review Required

**RATIFIED 2026-06-03 (principle-derived autonomous ratification).** See `uservalidation.md` for the per-dimension operator notes and the 5 open-question directions. Status transitioned `in_progress → specs_hardened`. Downstream Bubbles agents (`bubbles.design`, `bubbles.plan`) may now pick up the spec via normal workflow gates; implementation remains DoD-gated. The 7-dimension list below remains the authoritative requirements record.

| # | Safety Dimension | Operator Ratification Required |
|---|------------------|--------------------------------|
| 1 | **Trust boundary** — supervisor is read-only by default; writes require operator-issued capability token with scope + TTL + max-actions | Confirm token issuance/revocation workflow + storage location |
| 2 | **Blast radius** — NEVER auto-deploy; proposals only, human merge; bug-filing is append-only (reversible) | Confirm proposal-only stance + reversibility guarantee |
| 3 | **Where it runs** — separate container/host outside the smackerel-core stack; deploy-adapter-owned, configured via knb's bcdr/upkeep harness | Confirm host/container placement and whose adapter owns the lifecycle |
| 4 | **Forbidden surfaces** — MUST NOT modify `config/smackerel.yaml`, knb `manifest.yaml`, push to git from prod, take stack down, exfiltrate user data | Confirm forbidden list is complete |
| 5 | **Failure modes** — noisy-neighbor self-detection (SCN-079-A05), stale spec knowledge mitigation, conflict-with-planned-work mitigation (SCN-079-A04 deference) | Confirm failure-mode coverage + acceptable residual risk |
| 6 | **Boundary with `bubbles.upkeep`** — supervisor is event-driven; upkeep is calendar-driven; they MUST NOT overlap; upkeep wins precedence | Confirm boundary; reject if any overlap planned |
| 7 | **Cross-product reuse decision** — supervisor pattern is a candidate for promotion to bubbles framework foundation (`bubbles.supervisor`); spec 079 is the product-specific instance feeding the foundation | Confirm framework-promotion intent OR reject and re-scope as product-only |

**Ratification mechanism:** Operator edits `uservalidation.md`, checks the 7 boxes with notes, then transitions `state.json.status` from `in_progress` to `specs_hardened`. Only then is `bubbles.design` permitted to author `design.md` refinements and `bubbles.plan` permitted to author `scopes.md`.

**Implementation is deferred until operator ratifies the safety model in `uservalidation.md`.**

---

## 12. Out of Scope (Explicit Non-Goals)

- Any line of supervisor implementation code (deferred).
- `scopes.md` authoring (owned by `bubbles.plan`, blocked until operator ratification).
- Real <deploy-host> hostname, IP, or tailnet identifiers (operator binds these in the knb deploy-adapter overlay; this repo stays generic — see `.github/copilot-instructions.md` "No Env-Specific Content").
- Integration with paging providers (PagerDuty/Opsgenie) — supervisor is anti-paging by design.
- Modifying `bubbles.upkeep` — out of scope; coexistence boundary defined above.
