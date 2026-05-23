# User Validation — BUG-049-001 — Prometheus external image contract drift

## Checklist

This bug is a developer-facing contract update with an adversarial regression
test. There is no end-user-visible surface to validate manually.

- [x] Owner reviewed the contract update in `deploy/contract.yaml` and the
      regression test in `internal/deploy/external_images_contract_test.go`.
- [x] Owner confirmed `./smackerel.sh test unit --go` is green in the report.
- [x] Owner accepts the bug as resolved.

## Rationale For Lightweight Validation

The change is:

1. Additive — appends one entry to a documentation YAML.
2. Backward-compatible — the new `profile:` field is ignored by older parsers.
3. Locked by a Go contract test — future regressions are caught by the test
   runner, not by the operator at apply time.

No operator-facing behaviour changes, so no end-to-end run is required.
