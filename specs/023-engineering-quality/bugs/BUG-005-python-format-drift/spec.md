# Feature: [BUG-005] Fix Python format drift from ruff version pin

## Problem Statement
The Python ML sidecar's ruff dependency uses a floor-only pin (`>=0.8.0`) that allows arbitrary major version upgrades. When ruff's formatting rules change across versions, committed files drift out of compliance, causing `./smackerel.sh format --check` to fail. This blocks CI and PR acceptance.

## Outcome Contract
**Intent:** Pin the ruff version to a stable range and reformat affected files so the format check passes consistently.
**Success Signal:** `./smackerel.sh format --check` exits 0 with zero files needing reformatting, and `./smackerel.sh test unit --python` still passes.
**Hard Constraints:** No Python test behavior may change. Only formatting and the version pin are modified.
**Failure Condition:** Format check still fails after the fix, or the version pin is still loose enough to allow future formatting drift.

## Goals
- Pin ruff to a version range that prevents future formatting drift
- Reformat the 4 affected files to comply with the pinned version
- Restore `./smackerel.sh format --check` to passing state

## Non-Goals
- Changing ruff configuration rules or adding new lint rules
- Upgrading other Python dependencies
- Modifying Python source logic

## Requirements
- `ml/pyproject.toml` must pin ruff to a range that prevents minor/major version drift (e.g., `ruff>=0.15.0,<0.16.0` or exact pin)
- All 4 affected files must be reformatted to match the pinned version's output
- `./smackerel.sh format --check` must exit 0
- `./smackerel.sh test unit --python` must pass with zero failures
- No other Python files should be affected by the reformat

## User Scenarios (Gherkin)

```gherkin
Scenario: Format check passes after ruff pin and reformat
  Given ruff is pinned to a stable version range in ml/pyproject.toml
  And the 4 affected files have been reformatted
  When ./smackerel.sh format --check is run
  Then it exits with code 0 and reports no files needing reformatting

Scenario: Python unit tests pass after reformat
  Given the 4 affected files have been reformatted
  When ./smackerel.sh test unit --python is run
  Then all tests pass with zero failures

Scenario: No unrelated files reformatted
  Given ruff is pinned to the new version range
  When ./smackerel.sh format is run
  Then only the 4 known affected files are changed
```

## Acceptance Criteria
- `./smackerel.sh format --check` exits 0
- `./smackerel.sh test unit --python` exits 0
- `ml/pyproject.toml` contains a ruff version pin with an upper bound
- Only `ml/app/metrics.py`, `ml/app/nats_client.py`, `ml/tests/test_receipt_detection.py`, `ml/tests/test_receipt_extraction.py` are reformatted
