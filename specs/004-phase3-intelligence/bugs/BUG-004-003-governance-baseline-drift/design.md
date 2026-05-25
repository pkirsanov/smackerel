# Design — BUG-004-003 Governance Baseline Drift Closure

> **Bug:** [spec.md](spec.md)
> **Parent Spec:** [../../spec.md](../../spec.md)
> **Parent Design:** [../../design.md](../../design.md)

## Closure Strategy

Artifact-only `validate-to-doc` closure. Zero runtime, config, test,
CI, deploy, framework, or docs files touched. The fix is exclusively
restoring spec→code traceability accuracy after a post-certification
package refactor moved function implementations out of
`internal/intelligence/engine.go` into domain-specific files
(`synthesis.go`, `briefs.go`, `alerts.go`, `alert_producers.go`).

This pattern mirrors three prior precedents under the previous sweep
`sweep-2026-05-24-r10`:

- **R3 — `BUG-020-006-governance-baseline-drift`** (spec 020 Security
  Hardening): legacy strict-mode guard tightening; closed by artifact
  marker restoration at `validate-to-doc` ceiling.
- **R5 — `BUG-014-003-governance-baseline-drift`** (spec 014 Discord
  Connector): same class, larger edit footprint (40 BLOCKs across 7
  finding classes); closed by Categories A-F restorations at
  `validate-to-doc` ceiling.
- **R8 — `BUG-006-005-governance-baseline-drift`** (spec 006 Phase 5
  Advanced): post-certification artifact drift; same `validate-to-doc`
  closure.

The Phase 3 Intelligence runtime is fully functional, certified,
chaos-hardened, security-hardened, and stability-hardened. The runtime
quality is not in dispute. The drift is exclusively in the spec
artifacts' file-path attribution.

## Authoritative File→Symbol Map (Post-Refactor Reality)

Verified by `grep -E "^func |^type [A-Z]" internal/intelligence/*.go`
at HEAD `554e620f9cc0aaeec534b1bd8837d161a864762b`.

### `internal/intelligence/engine.go` (96 lines)

**Types:** `InsightType`, `SynthesisInsight`, `AlertType`,
`AlertStatus`, `Alert`, `Engine`.

**Constants:** `InsightThroughLine`, `InsightContradiction`,
`InsightPattern`, `InsightSerendipity`, `AlertBill`,
`AlertReturnWindow`, `AlertTripPrep`, `AlertRelationship`,
`AlertCommitmentOverdue`, `AlertMeetingBrief`, `AlertPending`,
`AlertDelivered`, `AlertDismissed`, `AlertSnoozed`.

**Functions:** `NewEngine`.

### `internal/intelligence/synthesis.go` (411 lines)

**Types:** `WeeklySynthesis`.

**Functions:** `(*Engine).RunSynthesis`, `synthesisConfidence`,
`(*Engine).GenerateWeeklySynthesis`,
`(*Engine).detectCapturePatterns`, `assembleWeeklySynthesisText`,
`(*Engine).GetLastSynthesisTime`.

### `internal/intelligence/briefs.go` (377 lines)

**Types:** `MeetingBrief`, `AttendeeBrief`, `overdueItem`.

**Functions:** `(*Engine).CheckOverdueCommitments`,
`(*Engine).collectOverdueItems`, `(*Engine).GeneratePreMeetingBriefs`,
`(*Engine).buildAttendeeBrief`, `assembleBriefText`.

### `internal/intelligence/alerts.go` (227 lines)

**Functions:** `(*Engine).CreateAlert`, `(*Engine).DismissAlert`,
`(*Engine).SnoozeAlert`, `(*Engine).GetPendingAlerts`,
`(*Engine).MarkAlertDelivered`, `(*Engine).HasStalePendingAlerts`.

### `internal/intelligence/alert_producers.go` (378 lines)

**Functions:** `(*Engine).ProduceBillAlerts`,
`(*Engine).ProduceTripPrepAlerts`,
`(*Engine).ProduceReturnWindowAlerts`,
`(*Engine).ProduceRelationshipCoolingAlerts`, `clampDay`,
`calendarDaysBetween`.

### `internal/intelligence/resurface.go` (350 lines)

**Functions:** `(*Engine).Resurface`, `serendipityPick`.

### `internal/digest/generator.go` (679 lines)

**Type owning `ActionItem`:** line 64. (The spec 004 Scope 2
evidence currently mis-attributes `ActionItem` to
`internal/intelligence/engine.go`.)

## Edit Plan (3 Categories)

### Category A — `scopes.md` Evidence-Block Function Citations (F1)

Replace the cited file path in each evidence block where a function
is cited but the function does not live in `engine.go`. Constant and
type citations stay as-is when they correctly resolve to `engine.go`.

| Line | Scope | Original cite | Corrected cite |
|------|-------|---------------|-----------------|
| 102 | 1 | `engine.go` RunSynthesis | `synthesis.go` RunSynthesis |
| 104 | 1 | `engine.go` RunSynthesis (...) `SynthesisInsight` | `synthesis.go` RunSynthesis generates `SynthesisInsight` (defined in `engine.go`) |
| 106 | 1 | `engine.go` cluster query | `synthesis.go` RunSynthesis cluster query |
| 112 | 1 | `engine.go` RunSynthesis (...) | `synthesis.go` RunSynthesis (...) |
| 120 | 1 | `engine.go` cluster query | `synthesis.go` RunSynthesis cluster query |
| 198 | 2 | `engine.go` CheckOverdueCommitments | `briefs.go` CheckOverdueCommitments |
| 200 | 2 | `engine.go` `ActionItem` model | `internal/digest/generator.go` `ActionItem` model |
| 204 | 2 | `engine.go` CheckOverdueCommitments | `briefs.go` CheckOverdueCommitments |
| 208 | 2 | `engine.go` CheckOverdueCommitments | `briefs.go` CheckOverdueCommitments |
| 214 | 2 | `engine.go` CheckOverdueCommitments | `briefs.go` CheckOverdueCommitments |
| 290 | 3 | `engine.go` AlertMeetingBrief type | `engine.go` AlertMeetingBrief constant + `briefs.go` GeneratePreMeetingBriefs |
| 296 | 3 | `engine.go` MeetingBrief | `briefs.go` MeetingBrief |
| 302 | 3 | `engine.go` MeetingBrief.EventID | `briefs.go` MeetingBrief.EventID |
| 397 | 4 | `engine.go` GetPendingAlerts | `alerts.go` GetPendingAlerts |
| 399 | 4 | `engine.go` DismissAlert (...) CreateAlert | `alerts.go` DismissAlert / SnoozeAlert / GetPendingAlerts / CreateAlert |
| 401 | 4 | `engine.go` CreateAlert | `alerts.go` CreateAlert (AlertBill const in `engine.go`) |
| 403 | 4 | `engine.go` GetPendingAlerts | `alerts.go` GetPendingAlerts |
| 405 | 4 | `engine.go` DismissAlert | `alerts.go` DismissAlert / GetPendingAlerts |
| 485 | 5 | `engine.go` GenerateWeeklySynthesis | `synthesis.go` GenerateWeeklySynthesis |
| 487 | 5 | `engine.go` SynthesisInsight (...) | `synthesis.go` SynthesisInsight (defined in `engine.go`) |
| 495 | 5 | `engine.go` WeeklySynthesis.Patterns | `synthesis.go` WeeklySynthesis.Patterns |
| 587 | 6 | `engine.go` AlertMeetingBrief type integrated | `engine.go` AlertMeetingBrief constant; `briefs.go` GeneratePreMeetingBriefs assembles brief data |

Citations correctly attributed to `engine.go` (NO change required):
108 (InsightContradiction const), 110 (SynthesisInsight struct), 114
(ML sidecar — design statement), 202 (status transition — design
statement), 210 (Alert model + AlertCommitmentOverdue const), 212
(action_item lifecycle — design statement), 389 (AlertBill const),
391 (AlertReturnWindow const), 393 (AlertTripPrep const), 395
(AlertRelationship const), 407 (AlertReturnWindow const), 409
(AlertRelationship const), 411 (AlertTripPrep const).

### Category B — `scopes.md` `Implementation Files` Lists (F2)

Extend the `### Implementation Files` lists for Scopes 1, 2, 3, 4 to
match the post-refactor split.

| Line | Scope | Current list | Add |
|------|-------|--------------|-----|
| 85 | 1 | engine.go, engine_test.go | synthesis.go, synthesis_test.go |
| 179 | 2 | engine.go, engine_test.go | briefs.go, briefs_test.go, internal/digest/generator.go (ActionItem type) |
| 274 | 3 | engine.go, engine_test.go | briefs.go, briefs_test.go |
| 371 | 4 | engine.go, engine_test.go | alerts.go, alerts_test.go, alert_producers.go, alert_producers_test.go |

Scopes 5 (line ~550) and 6 (line 562) lists are already accurate; no
edit.

### Category C — `design.md` `### Intelligence Engine Components` Diagram (F3)

Replace the fabricated 4-subdirectory diagram (lines ~120-145) with
the actual flat layout. Use a fenced code block that mirrors `ls
internal/intelligence/*.go` reality and explicitly notes that Phase
4/5 surface files (annotations, expenses, expertise, learning, lists,
lookups, monthly, people, people_forecast, subscriptions,
vendor_seeds) live in the same directory but are scope of later
specs (006/025/027/028/034/035), not spec 004.

## Verification Strategy

- Run `state-transition-guard.sh specs/004-phase3-intelligence`
  before and after edits. Baseline = 0 BLOCKs + 1 placeholder-paths
  warning (already captured at `/tmp/stg004-baseline.out`). Post-edit
  must remain 0 BLOCKs and warning count must not exceed baseline.
- Run `state-transition-guard.sh
  specs/004-phase3-intelligence/bugs/BUG-004-003-governance-baseline-drift`
  on the bug packet — must show TRANSITION PERMITTED at the
  `validate-to-doc` ceiling.
- Run `artifact-lint.sh specs/004-phase3-intelligence` — must PASS.
- Run `traceability-guard.sh specs/004-phase3-intelligence` — must
  PASS with no regression on scenario coverage.
- Run `git diff --cached --name-status` — staged paths MUST be a
  strict subset of the allowed surfaces enumerated in
  [spec.md AC7](spec.md).
- Run `grep -rn "synthesis/" specs/004-phase3-intelligence/design.md
  | grep -v "synthesis.go"` — should return zero matches after the
  fictitious sub-package references are removed.

## Out Of Scope

- Touching any `internal/`, `cmd/`, `ml/`, `config/`, `tests/`,
  `deploy/`, `docs/`, `.github/workflows/`, `.github/agents/`,
  `.github/bubbles/` paths.
- Updating Phase 4/5 specs (006, 025, 027, 028, 034, 035) that own
  the other intelligence package files.
- Rewriting scenarios, ACs, or DoD wording — only the file-path
  citation portion of evidence blocks is modified.
- Adding new tests or new code coverage.
