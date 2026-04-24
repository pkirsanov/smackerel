# Execution Reports

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Pin ruff version and reformat affected files — Done

### Summary
The Python ML sidecar's `ruff` dependency had a floor-only pin (`>=0.8.0`) that allowed arbitrary major version upgrades. When ruff's formatting rules changed across versions, four committed files (`ml/app/metrics.py`, `ml/app/nats_client.py`, `ml/tests/test_receipt_detection.py`, `ml/tests/test_receipt_extraction.py`) drifted out of compliance and broke `./smackerel.sh format --check`. Commit c6e3dca (2026-04-20) pinned ruff to `>=0.15.0,<0.16.0` and reformatted the four files. This session (2026-04-24) verified the fix at HEAD with real captured commands.

### Completion Statement
All 19 DoD items in `scopes.md` (12 Core + 7 Build Quality) are checked with inline `**Evidence:**` blocks captured this session from real terminal output, plus historical git evidence for pre-fix items where re-capture would require source revert (forbidden by the verification-only hard rule). Scope 1 status promoted from `In Progress` to `Done`. State promoted from `in_progress` to `done`.

### Test Evidence

**Command:** `./smackerel.sh test unit` (Python pytest portion, captured 2026-04-24)

```
$ ./smackerel.sh test unit
Successfully installed annotated-doc-0.0.4 ... ruff-0.15.11 ...
........................................................................ [ 21%]
........................................................................ [ 43%]
........................................................................ [ 65%]
........................................................................ [ 87%]
..........................................                               [100%]
=============================== warnings summary ===============================
tests/test_ocr.py::TestLRUEviction::test_evicts_oldest_when_exceeding_max
  /workspace/ml/app/ocr.py:139: RuntimeWarning: coroutine 'AsyncMockMixin._execute_mock_call' was never awaited
330 passed, 2 warnings in 11.94s
Exit Code: 0
```

**Command:** `./smackerel.sh format --check`

```
$ ./smackerel.sh format --check
Collecting ruff<0.16.0,>=0.15.0 (from smackerel-ml==0.1.0)
  Downloading ruff-0.15.11-py3-none-manylinux_2_17_x86_64.manylinux2014_x86_64.whl.metadata (26 kB)
Successfully installed ruff-0.15.11 ...
39 files left unchanged
Exit Code: 0
```

**Command:** `grep -n "ruff" ml/pyproject.toml`

```
$ grep -n "ruff" ml/pyproject.toml
30:    "ruff>=0.15.0,<0.16.0",
42:[tool.ruff]
46:[tool.ruff.lint]
Exit Code: 0
```

### Validation Evidence

**Command:** `git show --stat c6e3dca` filtered for ml Python files — confirms only the 4 documented files plus pyproject.toml were touched (no unrelated reformat)

```
$ git show --stat c6e3dca | grep -E "ml/(app|tests|pyproject)"
 ml/app/metrics.py                                  |  19 +-
 ml/app/nats_client.py                              |   8 +-
 ml/pyproject.toml                                  |   2 +-
 ml/tests/test_receipt_detection.py                 |  46 ++--
 ml/tests/test_receipt_extraction.py                |   8 +-
Exit Code: 0
```

**Command:** `./smackerel.sh format --check` — fix is structurally enforced (pip resolves ruff<0.16.0 ruling out drift-causing upgrades)

```
$ ./smackerel.sh format --check 2>&1 | grep -E "Collecting ruff|left unchanged"
Collecting ruff<0.16.0,>=0.15.0 (from smackerel-ml==0.1.0)
39 files left unchanged
Exit Code: 0
```

### Audit Evidence

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-005-python-format-drift`

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-005-python-format-drift
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Detected state.json status: done
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
✅ Workflow mode 'bugfix-fastlane' allows status 'done'
✅ All 1 scope(s) in scopes.md are marked Done
✅ Required specialist phase 'implement' found in execution/certification phase records
✅ Required specialist phase 'test' found in execution/certification phase records
✅ Required specialist phase 'validate' found in execution/certification phase records
✅ Required specialist phase 'audit' found in execution/certification phase records
✅ Phase-scope coherence verified (Gate G027)
Artifact lint PASSED.
Exit Code: 0
```

**Command:** `git log -1 --format='%h %ai %s' c6e3dca` — fix commit metadata

```
$ git log -1 --format='%h %ai %s' c6e3dca
c6e3dca 2026-04-20 05:12:13 +0000 fix(023): BUG-005 pin ruff version + BUG-006 test auth token provisioning
Exit Code: 0
```

### Verification Notes

- The "Pre-fix regression test FAILS" DoD item was evidenced via historical git log + commit message, not by reverting source. The user's hard rule for this verification session forbids modifying `pyproject.toml` (or any source), so the original failing `format --check` output cannot be re-captured here. The post-fix `format --check` returning EXIT=0 with "39 files left unchanged" is the proof that the bug is resolved.
- The "Adversarial regression case" is structurally enforced by the `<0.16.0` upper bound rather than by a code-level test. pip resolution this session confirmed the bound is active (`ruff-0.15.11` installed, NOT 0.16+).

## Re-Promotion Note (2026-04-24)

The earlier 2026-04-20 promotion to `done` was demoted to `in_progress` because the prior `report.md` was a stub ("Pending implementation") with no specialist phase records or evidence blocks beyond the bare `documentation` phase. This session captured real terminal output for every DoD item against the current HEAD, where the ruff pin and the four reformatted files have been in place since commit c6e3dca (2026-04-20). The 2026-04-24 promotion replaces the stub Pending content with command-backed evidence per the bugfix-fastlane workflow.
