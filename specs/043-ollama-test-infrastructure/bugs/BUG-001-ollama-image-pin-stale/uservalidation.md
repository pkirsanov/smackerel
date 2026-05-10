# BUG-001 User Validation

> Parent acceptance reference: spec 043 Scope 01 (Config + Compose Foundation) — `infrastructure.ollama.image` and `infrastructure.ollama.test.image` SST keys must produce a pullable image at integration-test time.

## Checklist

- [x] **SCN-BUG-001-001:** Pinned ollama tag (`ollama/ollama:0.23.2`) is currently published on Docker Hub.
  - **Steps:**
    1. `./smackerel.sh --env test config generate`
    2. `grep OLLAMA_IMAGE config/generated/test.env`
    3. `curl -sS -o /dev/null -w "%{http_code}\n" "https://hub.docker.com/v2/repositories/ollama/ollama/tags/0.23.2"`
  - **Expected:** Step 2 prints `OLLAMA_IMAGE=ollama/ollama:0.23.2`. Step 3 prints `200`.
  - **Verify:** `go test -count=1 -tags=integration -v -run 'TestOllamaImagePinIsPublished_LiveTag' ./tests/integration/` exits 0.
  - **Evidence:** [report.md](./report.md#test-evidence)
  - **Notes:** Replaces yanked `ollama/ollama:0.6` pin.

- [x] **SCN-BUG-001-002:** Adversarial — the registry-existence guard returns HTTP 404 for the known-yanked tag `ollama/ollama:0.6`.
  - **Steps:**
    1. `go test -count=1 -tags=integration -v -run 'TestOllamaImagePinIsPublished_AdversarialYankedTag' ./tests/integration/`
  - **Expected:** Test PASSES because the helper returns HTTP 404 for `ollama/ollama:0.6`. Test logs the exact 404 status code returned by Docker Hub.
  - **Verify:** Confirms the live test (`TestOllamaImagePinIsPublished_LiveTag`) is not tautological — it would have caught BUG-001 at the time it was introduced.
  - **Evidence:** [report.md](./report.md#test-evidence)
  - **Notes:** Bug fix for [BUG-001].

- [x] **Operator unblock:** Integration test stack (`./smackerel.sh test integration`) and operator-driven cold-pull lane (`SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e`) can pull the new pin.
  - **Steps:**
    1. `docker pull ollama/ollama:0.23.2` from a clean docker cache.
  - **Expected:** Pull succeeds. Manifest exists.
  - **Verify:** Same SHA digest the Docker Hub tags API reports.
  - **Evidence:** [report.md](./report.md#test-evidence)
  - **Notes:** Live cold-pull verification is operator-driven (~4 GB image, ~minutes); the registry-existence guard is the automated proxy.
