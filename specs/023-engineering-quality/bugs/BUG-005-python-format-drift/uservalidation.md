# User Validation Checklist

## Checklist

- [x] Baseline checklist initialized for BUG-005 python-format-drift

### [Bug Fix] [BUG-005] Python format drift — ruff version pin
- [x] **What:** Pin ruff to a stable version range and reformat 4 drifted Python files
  - **Steps:**
    1. Run `./smackerel.sh format --check` — verify exit code 0
    2. Run `./smackerel.sh test unit --python` — verify all tests pass
    3. Inspect `ml/pyproject.toml` — verify ruff has upper-bound pin
  - **Expected:** Format check passes, all Python tests pass, ruff pin prevents future drift
  - **Verify:** `./smackerel.sh format --check && ./smackerel.sh test unit --python`
  - **Evidence:** report.md#scope-1
  - **Notes:** Bug fix for [BUG-005] — toolchain version pin + reformat, no logic changes

Unchecked items indicate a user-reported regression.
