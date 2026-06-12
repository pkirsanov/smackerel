# User Validation: BUG-031-008 CI integration job stabilization

This bug is validated when the smackerel `CI` workflow `integration` job is GREEN on
`origin/main`. Items are unchecked while the fix is in progress; each is checked with
evidence once the corresponding cluster is GREEN against the real stack (and the overall
integration job is green post-push).

## Checklist

- [x] Bug packet consolidates the 4 CI integration-job clusters under spec 031 (031 owns live-stack testing); the prior BUG-031-001 and BUG-045-002 are both `done` and do not cover these four tests, so this is NEW work (scoping fact, true at filing time)
- [ ] C1: `./smackerel.sh --env test auth` and `... auth not-a-real-subcommand` return exit 2 with the expected banner through the non-interactive caller (`TestCLIAuthPassthrough_*` GREEN)
- [ ] C2: the assistant transport-hint live-stack tests pass (or the cause is honestly classified and handed back with exact repro + next owner if a real CI-resource decision is required)
- [ ] C3a: the micro-tools registry canary reflects shipped reality (four concrete micro-tools registered) and passes
- [ ] C3b: the weather scenario advertises `location_normalize` (+ `weather_lookup`); the prompt stays ≥40% shrunk; scenario-lint stays green
- [ ] The CI `integration` job is GREEN on `origin/main` after the fast-forward push (or the remaining clusters are documented with exact repro + next owner)
- [ ] spec-083 (`specs/083-card-rewards-companion/*`, `internal/cardrewards/*`, and the listed ml/test/docs files) is untouched by this bug's change set
