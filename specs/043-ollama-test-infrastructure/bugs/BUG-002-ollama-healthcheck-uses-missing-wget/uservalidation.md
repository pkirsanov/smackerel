# BUG-002 User Validation

> Parent acceptance reference: spec 043 Scope 01 (Config + Compose Foundation) — the compose-side ollama service must report `Up (healthy)` so `docker compose up -d --wait` succeeds and the integration runner can proceed.

## Checklist

- [x] **SCN-BUG-002-001:** ollama container reports `Up (healthy)` within `start_period + interval × retries` seconds when the test stack comes up.
  - **Steps:**
    1. `./smackerel.sh --env test config generate`
    2. `./smackerel.sh --env test up`
    3. `docker ps --filter name=smackerel-test-ollama --format '{{.Names}}\t{{.Status}}'`
  - **Expected:** Step 2 exits 0 (no `--wait-timeout` exit-124). Step 3 prints `smackerel-test-ollama-1\tUp <duration> (healthy)`.
  - **Verify:** `go test -count=1 -tags=integration -v -run 'TestOllamaHealthcheck_LiveFiles' ./tests/integration/` exits 0.
  - **Evidence:** [report.md](./report.md#test-evidence)
  - **Notes:** Replaces the broken `wget`-based healthcheck (binary not in `ollama/ollama:0.23.2` image after BUG-001 image bump).

- [x] **SCN-BUG-002-002:** Adversarial — the healthcheck-binary guard rejects any compose snippet whose ollama healthcheck calls `wget` or `curl` (binaries not in the pinned image), with an error that names the offending binary AND the image.
  - **Steps:**
    1. `go test -count=1 -tags=integration -v -run 'TestOllamaHealthcheck_Adversarial' ./tests/integration/`
  - **Expected:** All three adversarial tests PASS. Each rejection error mentions the forbidden binary (`wget` or `curl`) and the image (`ollama/ollama:0.23.2`).
  - **Verify:** Confirms the live test (`TestOllamaHealthcheck_LiveFiles`) is not tautological — it would have caught BUG-002 at the time it was introduced.
  - **Evidence:** [report.md](./report.md#test-evidence)
  - **Notes:** Bug fix for [BUG-002].

- [x] **Operator unblock:** Integration test stack (`./smackerel.sh test integration`) reaches the Go test runner instead of failing at the `docker compose up -d --wait` step.
  - **Steps:**
    1. `./smackerel.sh test integration`
  - **Expected:** Stack-up phase succeeds (no `container smackerel-test-ollama-1 is unhealthy / EXIT=124`). Go integration tests proceed; pass/fail status reflects test logic, not infra liveness.
  - **Verify:** Re-run end-to-end after fix — exit code from the runner reflects the actual test results, not infra timeout.
  - **Evidence:** [report.md](./report.md#test-evidence)
  - **Notes:** Unblocks spec 044 validate phase and every downstream spec that uses the integration lane.
