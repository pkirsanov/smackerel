# BUG-001 User Validation: Docker Compose Environment Variable Gap

**Parent:** [029-devops-pipeline](../../spec.md)
**Bug:** [bug.md](bug.md)

---

## Validation Criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | All SST-managed env vars reach smackerel-core container | PASS | `env_file: config/generated/dev.env` loads all 140+ vars |
| 2 | All SST-managed env vars reach smackerel-ml container | PASS | Same `env_file:` directive on ML service |
| 3 | Container-internal path overrides preserved | PASS | Volume mount paths use conditional `${VAR:+/container/path}` syntax |
| 4 | No individual SST-managed var declarations remain | PASS | `./smackerel.sh check` drift guard confirms |
| 5 | Future drift prevented by automated guard | PASS | `./smackerel.sh check` fails if `env_file:` removed or SST vars individually declared |
| 6 | No test regressions | PASS | All Go + Python unit tests pass |

## Acceptance

- [x] Bug fix verified — env_file migration eliminates the 52+ variable gap
- [x] Drift guard prevents recurrence
- [x] No regressions in unit tests
