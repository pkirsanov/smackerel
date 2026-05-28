# User Validation: BUG-061-001 BS-001 webhook cold-start poll budget

## Checklist

### [Bug Fix] BUG-061-001 BS-001 webhook e2e tolerates cold-stack Ollama load
- [x] **What:** `tests/e2e/test_telegram_assistant_bs001.sh` ROW-1 passes on both cold and warm test stacks.
  - **Steps:**
    1. `./smackerel.sh --env test down --volumes && ./smackerel.sh --env test up`
    2. `bash tests/e2e/test_telegram_assistant_bs001.sh`
  - **Expected:** All three rows (ROW-1 happy path, ROW-2 wrong-secret 401, ROW-3 missing-header 401) PASS.
  - **Verify:** terminal exit code 0 + three `PASS:` lines.
  - **Evidence:** report.md → Post-Fix Verification — Cold Stack
  - **Notes:** Bug fix for BUG-061-001; production code path unchanged.
