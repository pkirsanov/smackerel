# Bug: [BUG-005] Python format drift — 4 files fail ruff format check

## Summary
4 Python files fail `./smackerel.sh format --check` due to ruff version drift. The project specifies `ruff>=0.8.0` in `ml/pyproject.toml` (floor only, no ceiling). The latest ruff (0.15.11) introduces formatting rule changes that reformat 4 committed files differently than the version that originally formatted them.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — format check failing means CI would reject PRs
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Category
toolchain-drift

## Reproduction Steps
1. Run `./smackerel.sh format --check`
2. Observe exit code 1 with "4 files reformatted" output
3. The 4 affected files are listed below

## Expected Behavior
`./smackerel.sh format --check` should exit with code 0 and report no files needing reformatting.

## Actual Behavior
`./smackerel.sh format --check` exits with code 1 and reports 4 files that would be reformatted:
- `ml/app/metrics.py`
- `ml/app/nats_client.py`
- `ml/tests/test_receipt_detection.py`
- `ml/tests/test_receipt_extraction.py`

## Environment
- Service: smackerel-ml (Python ML sidecar)
- File: `ml/pyproject.toml` — ruff version constraint
- Platform: Linux
- Ruff version: latest (0.15.11) vs originally formatted version

## Error Output
```
./smackerel.sh format --check
Would reformat: ml/app/metrics.py
Would reformat: ml/app/nats_client.py
Would reformat: ml/tests/test_receipt_detection.py
Would reformat: ml/tests/test_receipt_extraction.py
4 files would be reformatted
```

## Root Cause
Loose ruff version pin (`>=0.8.0`) in `ml/pyproject.toml` allows major version upgrades with formatting rule changes. The latest ruff (0.15.11) formats certain constructs differently than the version that originally formatted these files, causing the format check to fail.

## Related
- Feature: `specs/023-engineering-quality/`
- Config: `ml/pyproject.toml`

## Deferred Reason
N/A — fixing immediately.
