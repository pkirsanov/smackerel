# Bug Fix Design: [BUG-003] Engine god-object

## Design Brief

- **Current State:** `internal/intelligence/engine.go` is 1256 LOC with 17 methods, 6 types, 15+ constants, and 5 helper functions — all on or associated with the `*Engine` struct. The rest of the `intelligence` package already follows a split convention (`expertise.go`, `subscriptions.go`, `monthly.go`, `resurface.go`, `people.go`, `learning.go`, `lookups.go`).
- **Target State:** `engine.go` contains only the Engine struct, constructor, and package-wide shared types. Method implementations are distributed across 4 domain files following the existing package convention.
- **Patterns to Follow:** The existing package convention: `expertise.go` has expertise methods on `*Engine`, `subscriptions.go` has subscription methods, etc. Exact same pattern — methods on `*Engine` in separate files.
- **Patterns to Avoid:** Do NOT split the Engine struct into separate structs — this would break all consumers, test setup, and scheduler wiring.
- **Resolved Decisions:** Pure file-level reorg within same package; all methods stay on `*Engine`; zero consumer, API, or import changes.
- **Open Questions:** None.

## Root Cause Analysis

### Investigation Summary
Retro hotspot analysis identified `engine.go` (1256 LOC, 17 methods) as the #2 hotspot. The rest of the `intelligence` package already splits by domain: `expertise.go` (250 LOC), `subscriptions.go` (328 LOC), `monthly.go` (498 LOC), `resurface.go` (293 LOC), `people.go` (293 LOC), `learning.go`, `lookups.go`. Only `engine.go` breaks the pattern.

### Root Cause
Organic growth — synthesis, alerts, briefs, and commitments were added to engine.go incrementally during spec 004 and 006 phases. Each feature added methods to the existing file rather than creating new domain files.

### Verified Source Analysis (engine.go, 1256 lines)

**Types defined in engine.go:**
- `InsightType` (string alias) + 4 constants: `InsightThroughLine`, `InsightContradiction`, `InsightPattern`, `InsightSerendipity`
- `SynthesisInsight` struct (8 fields)
- `AlertType` (string alias) + 6 constants: `AlertBill`, `AlertReturnWindow`, `AlertTripPrep`, `AlertRelationship`, `AlertCommitmentOverdue`, `AlertMeetingBrief`
- `AlertStatus` (string alias) + 4 constants: `AlertPending`, `AlertDelivered`, `AlertDismissed`, `AlertSnoozed`
- `Alert` struct (10 fields)
- `Engine` struct (2 fields: `Pool *pgxpool.Pool`, `NATS *smacknats.Client`)
- `overdueItem` struct (4 fields, unexported)
- `MeetingBrief` struct (5 fields)
- `AttendeeBrief` struct (6 fields)
- `WeeklySynthesis` struct (8 fields)
- `WeeklyStats` struct (4 fields)
- `TopicMovement` struct (3 fields)

**Constants:**
- `maxSynthesisTopicGroups = 10`
- `maxTitleLen = 200`, `maxBodyLen = 2000` (inside CreateAlert)
- `maxPendingAlertAgeDays = 7`
- `maxAttendeesPerMeeting = 10` (inside GeneratePreMeetingBriefs)
- `validAlertTypes` map

**Methods on `*Engine` (17 total):**
1. `NewEngine(pool, nats) *Engine` — constructor
2. `RunSynthesis(ctx) ([]SynthesisInsight, error)`
3. `CreateAlert(ctx, *Alert) error`
4. `DismissAlert(ctx, alertID) error`
5. `SnoozeAlert(ctx, alertID, until) error`
6. `GetPendingAlerts(ctx) ([]Alert, error)`
7. `MarkAlertDelivered(ctx, alertID) error`
8. `CheckOverdueCommitments(ctx) error`
9. `collectOverdueItems(ctx) ([]overdueItem, error)` — unexported
10. `GeneratePreMeetingBriefs(ctx) ([]MeetingBrief, error)`
11. `buildAttendeeBrief(ctx, email) AttendeeBrief` — unexported
12. `GenerateWeeklySynthesis(ctx) (*WeeklySynthesis, error)`
13. `detectCapturePatterns(ctx) []string` — unexported
14. `ProduceBillAlerts(ctx) error`
15. `ProduceTripPrepAlerts(ctx) error`
16. `ProduceReturnWindowAlerts(ctx) error`
17. `ProduceRelationshipCoolingAlerts(ctx) error`
18. `GetLastSynthesisTime(ctx) (time.Time, error)`

**Package-level helpers (not methods):**
- `synthesisConfidence(artifactCount, sourceCount int) float64`
- `assembleBriefText(brief MeetingBrief) string`
- `assembleWeeklySynthesisText(ws *WeeklySynthesis) string`
- `clampDay(year int, month time.Month, day int) time.Time`
- `calendarDaysBetween(from, to time.Time) int`

### Impact Analysis
- Affected components: `internal/intelligence/engine.go` only (split into 5 files)
- Affected data: none
- Affected users: none
- Risk: **lowest of all three hotspot bugs** — pure file move, all methods stay on `*Engine`, no API changes, no import changes for any consumer

## Fix Design

### Solution Approach
Split engine.go into domain files. All methods remain on `*Engine` — this is purely a file-level reorganization within the same package. No import changes, no consumer changes. No new filenames conflict with existing files in the package.

### Existing Package Files (no conflicts)
```
engine.go          ← being split
expertise.go
learning.go
lookups.go
monthly.go
people.go
resurface.go
subscriptions.go
```

Target names `synthesis.go`, `alerts.go`, `alert_producers.go`, `briefs.go` are all available.

### Target File Layout — Exact Method + Type + Helper Mapping

#### `engine.go` (~30 LOC after split)

**Keeps:**
- `Engine` struct definition
- `NewEngine` constructor

**Imports:**
```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    smacknats "github.com/smackerel/smackerel/internal/nats"
)
```

#### `synthesis.go` (~480 LOC)

**Types moved here:**
- `InsightType` + 4 constants (`InsightThroughLine`, `InsightContradiction`, `InsightPattern`, `InsightSerendipity`)
- `SynthesisInsight` struct
- `maxSynthesisTopicGroups` constant
- `WeeklySynthesis` struct
- `WeeklyStats` struct
- `TopicMovement` struct

**Methods moved here (on `*Engine`):**
- `RunSynthesis(ctx context.Context) ([]SynthesisInsight, error)`
- `GetLastSynthesisTime(ctx context.Context) (time.Time, error)`
- `GenerateWeeklySynthesis(ctx context.Context) (*WeeklySynthesis, error)`
- `detectCapturePatterns(ctx context.Context) []string`

**Helpers moved here:**
- `synthesisConfidence(artifactCount, sourceCount int) float64`
- `assembleWeeklySynthesisText(ws *WeeklySynthesis) string`

**Imports:**
```go
import (
    "context"
    "fmt"
    "log/slog"
    "math"
    "strings"
    "time"

    "github.com/oklog/ulid/v2"
)
```

Note: `ResurfaceCandidate` (used by `assembleWeeklySynthesisText` via `ws.SerendipityPicks`) is already defined in `resurface.go` — no conflict.

#### `alerts.go` (~230 LOC)

**Types moved here:**
- `AlertType` + 6 constants (`AlertBill`, `AlertReturnWindow`, `AlertTripPrep`, `AlertRelationship`, `AlertCommitmentOverdue`, `AlertMeetingBrief`)
- `AlertStatus` + 4 constants (`AlertPending`, `AlertDelivered`, `AlertDismissed`, `AlertSnoozed`)
- `Alert` struct
- `validAlertTypes` map
- `maxPendingAlertAgeDays` constant

**Methods moved here (on `*Engine`):**
- `CreateAlert(ctx context.Context, alert *Alert) error`
- `DismissAlert(ctx context.Context, alertID string) error`
- `SnoozeAlert(ctx context.Context, alertID string, until time.Time) error`
- `GetPendingAlerts(ctx context.Context) ([]Alert, error)`
- `MarkAlertDelivered(ctx context.Context, alertID string) error`

**Imports:**
```go
import (
    "context"
    "fmt"
    "log/slog"
    "strings"
    "time"

    "github.com/oklog/ulid/v2"
    "github.com/smackerel/smackerel/internal/stringutil"
)
```

#### `alert_producers.go` (~310 LOC)

**Methods moved here (on `*Engine`):**
- `ProduceBillAlerts(ctx context.Context) error`
- `ProduceTripPrepAlerts(ctx context.Context) error`
- `ProduceReturnWindowAlerts(ctx context.Context) error`
- `ProduceRelationshipCoolingAlerts(ctx context.Context) error`

**Helpers moved here:**
- `clampDay(year int, month time.Month, day int) time.Time`
- `calendarDaysBetween(from, to time.Time) int`

Note: `calendarDaysBetween` is also used by `CheckOverdueCommitments` (in briefs.go). Since both files are in the same package, this is fine — Go resolves symbols within the package regardless of file.

**Imports:**
```go
import (
    "context"
    "fmt"
    "log/slog"
    "time"
)
```

#### `briefs.go` (~300 LOC)

**Types moved here:**
- `overdueItem` struct (unexported)
- `MeetingBrief` struct
- `AttendeeBrief` struct

**Methods moved here (on `*Engine`):**
- `GeneratePreMeetingBriefs(ctx context.Context) ([]MeetingBrief, error)`
- `buildAttendeeBrief(ctx context.Context, email string) AttendeeBrief`
- `CheckOverdueCommitments(ctx context.Context) error`
- `collectOverdueItems(ctx context.Context) ([]overdueItem, error)`

**Helpers moved here:**
- `assembleBriefText(brief MeetingBrief) string`

**Imports:**
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strings"
    "time"

    "github.com/smackerel/smackerel/internal/stringutil"
)
```

### Test Impact Analysis

**Zero test breakage.** All test files in the package use `package intelligence` and reference types/methods by unqualified name. Since Go resolves symbols package-wide regardless of which file defines them, moving methods between files within the same package has zero effect on compilation or test behavior.

**Existing test file inventory:**
| Test File | Tests Types/Methods From | Impact |
|-----------|-------------------------|--------|
| `engine_test.go` | `InsightType`, `AlertType`, `AlertStatus`, `Alert`, `SynthesisInsight`, `MeetingBrief`, `WeeklySynthesis`, `NewEngine`, `RunSynthesis`, `CheckOverdueCommitments`, `CreateAlert`, `GeneratePreMeetingBriefs`, `assembleBriefText`, `assembleWeeklySynthesisText` | **None** — all symbols still in `package intelligence` |
| `expertise_test.go` | Expertise methods | None |
| `subscriptions_test.go` | Subscription methods | None |
| `monthly_test.go` | Monthly methods | None |
| `resurface_test.go` | Resurface methods | None |
| `people_test.go` | People methods | None |
| `lookups_test.go` | Lookup methods | None |
| `learning_test.go` | Learning methods | None |

**No new test files are required.** Optionally, tests could be reorganized to match source files (e.g., synthesis tests in `synthesis_test.go`), but this is a separate cleanup — not part of this bug fix.

### Verification Checklist (for implementer)

1. After splitting, run `wc -l internal/intelligence/engine.go` — must be ≤150 LOC
2. Run `./smackerel.sh test unit` — all tests must pass unchanged
3. Run `grep "^import" internal/intelligence/engine.go` — must NOT import `stringutil`, `encoding/json`, `math`, or `ulid`
4. Run `grep "^func (e \*Engine)" internal/intelligence/engine.go | wc -l` — must be 0 (no methods remain in engine.go)
5. Verify no duplicate function/type definitions: `grep -rn "^func\|^type\|^const\|^var" internal/intelligence/*.go | sort`

### Alternative Approaches Considered
1. **Split Engine into separate structs** — Rejected: breaks all consumers, test setup, and the scheduler wiring. Not justified for a god-file that's otherwise well-factored.
2. **Leave as-is, add file headers** — Rejected: does not fix the hotspot or coupling metrics.
3. **Move tests to match source files** — Deferred: optional cleanup, not part of the bug fix scope.
