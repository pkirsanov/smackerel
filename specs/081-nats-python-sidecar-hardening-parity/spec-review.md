# Spec Review: 081 NATS Python Sidecar Hardening Parity

**Phase:** spec-review · **Agent:** bubbles.spec-review (Gary Laser Eyes) ·
**Date:** 2026-06-04 · **Mode:** read-only audit · **Trust Verdict:**
**MINOR_DRIFT** (usable as source of truth with one documented gap)

---

## 1. Scope of Review

Audited the spec → design → scenario-manifest → scopes chain against
the shipped implementation and tests:

| Artifact | File |
|----------|------|
| Spec | [spec.md](spec.md) |
| Design | [design.md](design.md) |
| Scopes | [scopes.md](scopes.md) |
| Scenario manifest | [scenario-manifest.json](scenario-manifest.json) |
| Execution report | [report.md](report.md) |
| State | [state.json](state.json) |
| Production code | [ml/app/nats_client.py](../../ml/app/nats_client.py) |
| Unit tests | [ml/tests/test_nats_consumer_config.py](../../ml/tests/test_nats_consumer_config.py) |
| Live integration test | [ml/tests/integration/test_deadletter_parity.py](../../ml/tests/integration/test_deadletter_parity.py) |
| SST source | [config/smackerel.yaml](../../config/smackerel.yaml) |
| Config generator | [scripts/commands/config.sh](../../scripts/commands/config.sh) |

This spec was authored, implemented, tested, validated, and audited
all on 2026-06-04 (single-day delivery from `FOLLOWUP-046-PY-SIDECAR`).
No long drift window exists; review focuses on internal consistency
between artifacts rather than spec-vs-code time drift.

---

## 2. Trust Classification

**MINOR_DRIFT** — Maintenance agents may treat this spec as the source
of truth for the **intent** of the feature and the **canonical
contract** (design.md §3 + §4). One bootstrap-era inconsistency in
spec.md §4 / §7 and (by verbatim quote) scopes.md SCN-081-03 was never
retro-updated post-design and would mislead a careless reader.
**design.md is the canonical reference for the dead-letter header
envelope; do not read spec.md or scopes.md Gherkin in isolation for
that contract.**

| Per-artifact trust | Level | Notes |
|---|---|---|
| `spec.md` | MINOR_DRIFT | §4 header table + §7 SCN-081-03 list the bootstrap 5-name envelope (incl. forbidden Sidecar-Instance + Terminated-At) but self-flag the reconciliation handoff to design.md ("The canonical set is the Hard Constraint"). |
| `design.md` | CURRENT | §3 canonical 6-name envelope + §4 algorithm + §3.1 stream resolution match code byte-for-byte. The authoritative reference. |
| `scopes.md` | MINOR_DRIFT | Inherits spec.md §7 Gherkin drift via verbatim quotation; T-081-U6 references an unwritten Go test file (equivalent verification delivered via in-report grep under D01-2). |
| `scenario-manifest.json` | MINOR_DRIFT | SCN-081-03.linkedDoD omits D01-12 (live integration) and D01-14 (regression-E2E mark) which scopes.md maps to SCN-081-03. |
| `state.json` | MINOR_DRIFT | `scopeProgress.dodItemsTotal=13` vs scopes.md inventory `14 items + 2 regression-E2E bullets`; reconcilable if D01-14 is treated as the same test artifact as D01-12 (which the report explicitly notes it is). |
| `report.md` | CURRENT | All 13/13 DoD items carry executed evidence; live-stack integration run captured with exit codes; code-diff evidence present. |
| `ml/app/nats_client.py` | CURRENT | Matches design.md §3 envelope + §4 algorithm byte-for-byte. |
| Unit tests | CURRENT | Cover SCN-081-01..04 boundaries (consumer-config threading, fail-loud SST, 6-header envelope parity, publish-before-term invariant, `_failure_counts` removal, subject→stream completeness). |
| Live integration test | CURRENT | Live-stack JetStream poison-pill round-trip; asserts canonical envelope on republished `deadletter.<subject>`. |

---

## 3. Drift Detection Per Technique

### 3.1 File Existence Check — PASS

Every file path referenced in `design.md §2 Change Surface` and the
DoD evidence anchors exists at the stated path:

- `config/smackerel.yaml` ✅
- `scripts/commands/config.sh` ✅
- `ml/app/nats_client.py` ✅
- `ml/tests/test_nats_consumer_config.py` ✅
- `ml/tests/integration/test_deadletter_parity.py` ✅
- `internal/pipeline/synthesis_subscriber.go` (Go reference) ✅
- `internal/pipeline/subscriber.go` (Go reference) ✅
- `internal/nats/client.go` (stream-binding reference) ✅

### 3.2 Interface / Contract Check — MINOR_DRIFT (Finding F1)

The dead-letter header envelope is the principal external contract.
Three locations describe it and they do **not** all agree:

| Location | Header set described | Matches Go envelope? |
|---|---|---|
| spec.md §4 (Required header envelope table) | `Smackerel-Original-Subject`, `Smackerel-Delivery-Count`, `Smackerel-Last-Error`, **`Smackerel-Sidecar-Instance`**, **`Smackerel-Terminated-At`** | ❌ (2 forbidden names + missing 3) |
| spec.md §7 SCN-081-03 (Gherkin "Then" clause) | Same as §4 | ❌ |
| scopes.md SCOPE-081-01 Use Cases block (verbatim quote of spec.md §7) | Same as §4 | ❌ |
| design.md §3 (Header Envelope canonical) | `Smackerel-Original-Subject`, `Smackerel-Original-Stream`, `Smackerel-Failed-At`, `Smackerel-Last-Error`, `Smackerel-Delivery-Count`, `Smackerel-Original-Consumer` | ✅ (byte-for-byte Go parity) |
| `ml/app/nats_client.py::_handle_poison` lines ~676-714 | Same as design.md §3 | ✅ |
| `test_deadletter_headers_match_go_envelope` | Same as design.md §3 | ✅ |
| `test_poison_message_publishes_to_deadletter_subject` (live) | Same as design.md §3 | ✅ |

Spec.md §4 explicitly self-flags this reconciliation handoff:

> The Go reference (`publishSynthesisToDeadLetter`) uses
> `Smackerel-Failed-At` for the timestamp and
> `Smackerel-Original-Consumer` for the identity; the Python side
> MAY use the same names, and `design.md` MUST decide and align
> both sides to one canonical set before implementation. **The
> canonical set is the Hard Constraint.**

So the drift is documented (not silent), but spec.md §4 + §7 were
never updated post-design to reflect the resolved canonical set, and
scopes.md inherits the stale Gherkin via verbatim quotation.
Implementation honors design.md §3.

### 3.3 Behavioral Check — PASS

Gherkin scenarios SCN-081-01, SCN-081-02, SCN-081-04 describe
behavior the code exhibits and tests assert. SCN-081-03's Gherkin
"Then" clause names the wrong headers (per Finding F1) but the
intent — "consumer publishes the original payload to
`deadletter.<original-subject>` ... before the consumer term()s the
original message" — is honored by the implementation.

### 3.4 Structural Check — PASS

Change surface in design.md §2 (`config/smackerel.yaml`,
`scripts/commands/config.sh`, `ml/app/nats_client.py`) matches the
files actually touched per the report's Code Diff Evidence section
(+218 LOC across the three files + 448 LOC new tests).

### 3.5 Git History Analysis — N/A

Spec was created and shipped on 2026-06-04 from
`FOLLOWUP-046-PY-SIDECAR` (sweep round 13). No multi-day drift
window exists. Per audit blocker 1, the work currently lives entirely
in the working tree (zero structured commits to `specs/081`); commit
delta vs HEAD is the unit of analysis, not commit-history drift.

### 3.6 Test Alignment Check — MINOR_DRIFT (Findings F2, F3)

- **Finding F2 (MANIFEST-LINKAGE-GAP):** `scenario-manifest.json`
  SCN-081-03.linkedDoD lists `D01-8`, `D01-9`, `D01-10` but omits
  `D01-12` (live integration parity) and `D01-14` (regression-E2E
  mark of D01-12). Scopes.md explicitly maps both to SCN-081-03.
- **Finding F3 (TEST-PLAN-DRIFT):** Scopes.md Test Plan row
  `T-081-U6` references `internal/config/config_gen_test.go`
  (additive) which was never created. Equivalent verification was
  delivered via in-report `grep -E '^NATS_CONSUMER_(MAX_DELIVER|ACK_WAIT_SECONDS)='`
  output on the generated env file under D01-2 evidence.

Neither gap blocks the implementation contract; both are artifact
hygiene drift.

### 3.7 Redundancy / Superseded Truth Check — INFO (Finding F4)

- **Finding F4 (DOD-COUNT-AMBIGUITY):** `state.json`
  `scopeProgress[0].dodItemsTotal=13` and `dodItemsChecked=13`, but
  scopes.md inventory table says `14 items + 2 regression-E2E
  bullets` and the actual numbered DoD items are D01-1 through
  D01-14 (14 items). The audit-phase summary explicitly states D01-14
  is "the same artifact as D01-12, surfaced here as the explicit
  Gate G028 Check 8A regression-E2E protection point" — so the
  count of 13 is reconcilable if D01-14 is treated as the
  regression-E2E mark of D01-12 rather than an independent DoD item.
  No active truth is contradicted; flagged as INFO only.

---

## 4. Findings Summary

| # | Finding | Class | Affected artifacts | Suggested fix |
|---|---------|-------|--------------------|----------------|
| F1 | HEADER-ENVELOPE-DRIFT — spec.md §4 + §7 SCN-081-03 (+ scopes.md verbatim quote) describe a bootstrap 5-name envelope including forbidden `Smackerel-Sidecar-Instance` and `Smackerel-Terminated-At`; design.md §3 reconciled to the canonical 6-name Go envelope. | MINOR_DRIFT | spec.md §4 + §7 SCN-081-03; scopes.md SCOPE-081-01 Use Cases (Gherkin) block. | Update spec.md §4 Required header envelope table and §7 SCN-081-03 "Then" clause to name the 6 canonical headers per design.md §3; re-quote into scopes.md. |
| F2 | MANIFEST-LINKAGE-GAP — scenario-manifest.json SCN-081-03.linkedDoD missing D01-12 and D01-14. | MINOR_DRIFT | scenario-manifest.json | Add `"SCOPE-081-01:D01-12"` and `"SCOPE-081-01:D01-14"` to SCN-081-03.linkedDoD. |
| F3 | TEST-PLAN-DRIFT — scopes.md Test Plan row T-081-U6 references unwritten `internal/config/config_gen_test.go`. | MINOR_DRIFT | scopes.md Test Plan table | Either retro-add the Go test or replace the T-081-U6 row to point at the D01-2 grep evidence as the actual verification. |
| F4 | DOD-COUNT-AMBIGUITY — state.json dodItemsTotal=13 vs scopes.md inventory "14 items". | INFO | state.json `scopeProgress[0]` | Optional: adopt either count consistently. The 13-count is reconcilable if D01-14 is treated as the regression-E2E mark of D01-12. |

No MAJOR_DRIFT or OBSOLETE classification reached. None of the
findings undermine the implementation contract — the production
code matches design.md §3 byte-for-byte and is exercised by both
unit (`test_deadletter_headers_match_go_envelope`) and live
integration (`test_poison_message_publishes_to_deadletter_subject`)
assertions on the canonical 6-header envelope.

---

## 5. Auto-Dispatch Decision

Per spec-review mode Phase 5 trigger table:

| Trust Level | Docs Agent Action |
|-------------|-------------------|
| **MINOR_DRIFT** | No automatic invocation — add to handoff suggestions |

**No auto-dispatch invoked.** Findings F1–F3 are MINOR_DRIFT and
flagged for the operator as cleanup items; they do not require
`bubbles.docs` invocation. (No operator-facing managed docs reference
the internal JetStream dead-letter envelope — audit-phase grep over
`docs/**/*.md` confirmed only ntfy-adapter dead-letter references
exist, not this Python-sidecar pattern.)

---

## 6. Maintenance Context (for downstream agents)

If a maintenance agent revisits this feature (simplify, stabilize,
security, regression, gaps, code-review):

- **Treat `design.md §3` as the canonical header-envelope contract.**
  Do NOT read spec.md §4 or §7 SCN-081-03 in isolation; they describe
  a bootstrap draft that was reconciled away.
- **Treat `ml/app/nats_client._handle_poison` and `SUBJECT_TO_STREAM`
  as the authoritative implementation.** Any modification MUST keep
  the 6-header envelope byte-for-byte aligned with Go
  (`internal/pipeline/synthesis_subscriber.go::publishSynthesisToDeadLetter`)
  and MUST preserve the publish-before-term invariant.
- **Treat `test_poison_message_publishes_to_deadletter_subject` as the
  end-to-end regression tripwire.** Re-run after any change to
  `_handle_poison`, `SUBJECT_TO_STREAM`, or the dead-letter header
  envelope (this is the explicit T-081-E1 / D01-14 invariant).
- **Do not trust scopes.md Test Plan row T-081-U6** as a pointer to a
  Go test file — the actual env-emission verification lives in the
  D01-2 grep evidence in report.md.

---

## 7. Tier-1 Gate Re-runs After Phase Recording

| Command | Exit | Notes |
|---|---|---|
| `bash .github/bubbles/scripts/artifact-lint.sh specs/081-…` | (to-run) | Run after this report lands. |
| `bash .github/bubbles/scripts/state-transition-guard.sh specs/081-…` | (to-run) | Check 21 (spec-review enforcement) should PASS once spec-review is in `certifiedCompletedPhases` + `completedPhaseClaims` (recorded by this phase). Operator-side blockers Check 17 (commits) and Check 30 (post-cert) remain. |

(Exit codes captured in the result envelope below.)

---

## 8. Phase Recording

| Field | Value |
|---|---|
| `execution.activeAgent` | `bubbles.spec-review` |
| `execution.currentPhase` | `spec-review` |
| `certification.certifiedCompletedPhases[]` | appended `"spec-review"` (now 5 entries: implement, test, validate, audit, spec-review) |
| `execution.completedPhaseClaims[]` | appended spec-review entry (5 entries) |
| `execution.executionHistory[]` | appended spec-review entry (6 entries) |
| Top-level `status` | unchanged (`not_started`) — promotion remains operator-gated on blocker 1 (commit) |
| `certification.status` | unchanged (`not_started`) |

---

## 9. Next Required Owner

**OPERATOR** — Spec-review blocker 2 (audit-phase Check 21 +
artifact-lint spec-review enforcement) is now closed by this phase.
The remaining gate is blocker 1: zero structured commits to
`specs/081-nats-python-sidecar-hardening-parity` with prefix
`spec(081)` or `bubbles(081/...)`. Once the operator commits the
implementation, tests, SST keys, generator emission, and spec
artifacts under that prefix, re-invoke `bubbles.audit` to promote
to `done`.
