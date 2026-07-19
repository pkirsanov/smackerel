# User Validation: BUG-031-009 Dockerized Go E2E runner interruption leak

## Checklist

### Interruption cleanup

- [x] **What:** Interrupting a Dockerized Go E2E run stops its exact runner before dependencies are torn down.
  - **Steps:** Run the controlled nested interruption regression.
  - **Expected:** The runner container is absent before project-stack teardown and no post-teardown package failures are emitted.
  - **Verify:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`
  - **Evidence:** `report.md#test-evidence`

### Scoped ownership

- [x] **What:** Cleanup removes only the interrupted invocation's runner container.
  - **Steps:** Keep a nonmatching labeled canary container alive during the cleanup test.
  - **Expected:** Exact-label runner is removed; canary remains until test-owned cleanup.
  - **Verify:** Run the adversarial cleanup regression.
  - **Evidence:** `report.md#test-evidence`
