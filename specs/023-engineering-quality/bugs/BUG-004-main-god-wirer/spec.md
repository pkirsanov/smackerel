# Spec: [BUG-004] main.go god-wirer extraction

## Expected Behavior
`main.go` should contain only the `run()` entrypoint, signal handling, and server lifecycle (~150 LOC). Connector wiring and service construction should be in dedicated files within `cmd/core/`.

## Acceptance Criteria
- [ ] `main.go` is ≤200 LOC
- [ ] Connector wiring extracted to `connectors.go`
- [ ] Service construction extracted to `services.go`
- [ ] All existing tests pass unchanged
- [ ] No behavior change
- [ ] Adding a new connector only touches `connectors.go`
