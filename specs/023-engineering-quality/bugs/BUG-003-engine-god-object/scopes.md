# Scopes: [BUG-003] Engine god-object file split

## Scope 1: Split engine.go into domain files
**Status:** [ ] Not started

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: [Bug] Engine file split preserves behavior
  Scenario: All Engine methods remain callable after split
    Given engine.go has been split into synthesis.go, alerts.go, alert_producers.go, briefs.go
    When each method is called with valid inputs
    Then behavior is identical to pre-split (same return values, same DB queries)

  Scenario: engine.go contains only struct and constructor
    Given the split is complete
    When engine.go is inspected
    Then it contains only the Engine struct, NewEngine(), and shared type definitions

  Scenario: All existing engine tests pass unchanged
    Given engine_test.go has not been modified
    When ./smackerel.sh test unit is run
    Then all intelligence package tests pass with zero failures
```

### Implementation Plan
1. Create `internal/intelligence/synthesis.go` — move RunSynthesis, GenerateWeeklySynthesis, detectCapturePatterns, GetLastSynthesisTime, assembleWeeklySynthesisText
2. Create `internal/intelligence/alerts.go` — move CreateAlert, DismissAlert, SnoozeAlert, GetPendingAlerts, MarkAlertDelivered
3. Create `internal/intelligence/alert_producers.go` — move ProduceBillAlerts, ProduceTripPrepAlerts, ProduceReturnWindowAlerts, ProduceRelationshipCoolingAlerts, calendarDaysBetween
4. Create `internal/intelligence/briefs.go` — move GeneratePreMeetingBriefs, buildAttendeeBrief, CheckOverdueCommitments, collectOverdueItems
5. Verify engine.go only has struct + constructor + shared types
6. Verify all tests pass unchanged

### Test Plan
| Type | Label | Description |
|------|-------|-------------|
| Unit | Regression unit | All existing engine_test.go tests pass without modification |
| Unit | Package compile | `go build ./internal/intelligence/...` succeeds |
| Integration | Regression E2E | Full test suite green |

### Definition of Done — 3-Part Validation
- [ ] engine.go is ≤150 LOC
- [ ] 4 new domain files created with correct method placement
- [ ] No import changes in any consumer package
- [ ] All existing tests pass unchanged
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
