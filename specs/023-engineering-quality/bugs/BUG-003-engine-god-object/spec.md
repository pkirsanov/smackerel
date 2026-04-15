# Spec: [BUG-003] Engine god-object file split

## Expected Behavior
`engine.go` should contain only the Engine struct, constructor, and shared types. Method implementations should be distributed across domain-specific files following the existing package convention.

## Acceptance Criteria
- [ ] `engine.go` is ≤150 LOC (struct + constructor + shared types)
- [ ] Synthesis methods in `synthesis.go`
- [ ] Alert CRUD methods in `alerts.go`
- [ ] Alert producer methods in `alert_producers.go`
- [ ] Brief/commitment methods in `briefs.go`
- [ ] All existing tests pass unchanged (methods stay on `*Engine`)
- [ ] No behavior change, no API changes, no consumer changes
