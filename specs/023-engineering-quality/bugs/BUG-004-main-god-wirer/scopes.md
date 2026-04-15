# Scopes: [BUG-004] main.go god-wirer extraction

## Scope 1: Extract connector wiring and service construction
**Status:** [ ] Not started

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: [Bug] main.go extraction preserves startup behavior
  Scenario: Application starts successfully after extraction
    Given main.go has been split into main.go, connectors.go, services.go
    When the application is built and started
    Then all connectors register and the server serves requests identically

  Scenario: main.go contains only lifecycle code
    Given the extraction is complete
    When main.go is inspected
    Then it contains only run(), signal handling, and server start/stop

  Scenario: All existing main_test.go tests pass unchanged
    Given main_test.go has not been modified
    When ./smackerel.sh test unit is run
    Then all cmd/core tests pass with zero failures
```

### Implementation Plan
1. Create `cmd/core/connectors.go` — extract connector instantiation + config parsing + registration
2. Create `cmd/core/services.go` — extract DB, NATS, pipeline, scheduler, web handler construction
3. Slim `cmd/core/main.go` to lifecycle-only code
4. Verify build + all tests pass

### Test Plan
| Type | Label | Description |
|------|-------|-------------|
| Unit | Regression unit | All existing cmd/core tests pass unchanged |
| Unit | Build | `go build ./cmd/core/...` succeeds |
| Integration | Regression E2E | Full build + check + test cycle green |

### Definition of Done — 3-Part Validation
- [ ] main.go is ≤200 LOC
- [ ] connectors.go and services.go created with correct function placement
- [ ] All existing tests pass unchanged
- [ ] `./smackerel.sh build` succeeds
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
