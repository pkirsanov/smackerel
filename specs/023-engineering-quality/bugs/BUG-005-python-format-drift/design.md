# Bug Fix Design: [BUG-005] Python format drift

## Root Cause Analysis

### Investigation Summary
The `ml/pyproject.toml` file specifies `ruff>=0.8.0` as a development dependency. This floor-only pin allows pip/uv to install any ruff version ≥0.8.0, including the latest 0.15.11. Ruff 0.15.x introduces formatting rule changes (parenthesization, trailing comma placement, string quoting) that differ from the version that originally formatted the committed files.

### Root Cause
Loose version pin (`>=0.8.0`) in `ml/pyproject.toml` for the ruff formatter. The pin has no upper bound, so any ruff release with formatting rule changes causes committed files to drift out of compliance.

### Impact Analysis
- **Affected components:** Python ML sidecar formatting — `ml/app/metrics.py`, `ml/app/nats_client.py`, `ml/tests/test_receipt_detection.py`, `ml/tests/test_receipt_extraction.py`
- **Affected data:** None — formatting only, no logic changes
- **Affected users:** All developers — `./smackerel.sh format --check` fails, CI rejects PRs

## Fix Design

### Solution Approach
1. **Pin ruff version range** in `ml/pyproject.toml`: Change `ruff>=0.8.0` to `ruff>=0.15.0,<0.16.0` (or exact pin `ruff==0.15.11`). This allows patch updates within 0.15.x but prevents minor/major version jumps that change formatting rules.
2. **Reformat affected files**: Run `./smackerel.sh format` to apply the pinned version's formatting to the 4 affected files.
3. **Verify**: Run `./smackerel.sh format --check` to confirm exit code 0, and `./smackerel.sh test unit --python` to confirm no test regressions.

### Alternative Approaches Considered
1. **Exact pin (`ruff==0.15.11`)** — Provides maximum stability but requires manual bumps for security patches. Viable but slightly higher maintenance.
2. **Keep loose pin and reformat** — Would fix the immediate issue but leave the door open for future drift. Rejected because it does not address the root cause.

### Affected Files
| File | Change |
|------|--------|
| `ml/pyproject.toml` | Pin ruff version range |
| `ml/app/metrics.py` | Reformat (auto) |
| `ml/app/nats_client.py` | Reformat (auto) |
| `ml/tests/test_receipt_detection.py` | Reformat (auto) |
| `ml/tests/test_receipt_extraction.py` | Reformat (auto) |

### Regression Test Design
- **Pre-fix**: `./smackerel.sh format --check` must fail (exit code 1) with 4 files reported
- **Post-fix**: `./smackerel.sh format --check` must pass (exit code 0) with 0 files reported
- **Adversarial**: Verify that reverting the pyproject.toml pin back to `>=0.8.0` (without reverting file formatting) would still produce a format check failure, confirming the pin is the essential fix component
