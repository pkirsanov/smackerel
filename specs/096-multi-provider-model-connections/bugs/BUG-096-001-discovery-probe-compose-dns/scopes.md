# BUG-096-001 — Scopes

## Scope 1 — Discovery adapter follows the env-wired Ollama seam

**Status:** In Progress (in-repo fix + hermetic test done; live self-hosted redeploy-verify blocked)
**Depends On:** none
**Owner:** bubbles.devops (fix landed) → self-hosted redeploy-verify (blocked on stable host)

### Gherkin (see spec.md)

- SCN-096-001-A — discovery follows the env-wired host seam on self-hosted
- SCN-096-001-B — empty seam fails loud with no default
- SCN-096-001-C — dev parity preserved

### Implementation plan

1. Add `ollamaDiscoveryBaseURL(cfg)` (trimmed `cfg.OllamaURL`, fail-loud on empty) — DD-1.
2. Use it in `wireSpec096DiscoveryAndDispatch` for the Ollama-kind adapter; remove the dead `ollamaConnectionBaseURL` — DD-1/DD-2.
3. Add the hermetic contract `cmd/core/wiring_ollama_discovery_baseurl_test.go` — DD-3 test design.
4. Reword the fix doc comment so the `internal/config` SST hardcoded-Ollama guard stays green.

### Test Plan

| Test | Category | File | Scenario | Command | Live |
|------|----------|------|----------|---------|------|
| `TestOllamaDiscoveryBaseURL_UsesEnvSeamNotRegistryParam` | unit | `cmd/core/wiring_ollama_discovery_baseurl_test.go` | SCN-096-001-A | `./smackerel.sh test unit --go --go-run OllamaDiscoveryBaseURL` | No |
| `TestOllamaDiscoveryBaseURL_FailsLoudOnEmptySeam` | unit | same | SCN-096-001-B | same | No |
| `TestOllamaDiscoveryBaseURL_TrimsAndReturnsHostSeam` | unit | same | SCN-096-001-C (dev-parity value) | same | No |
| SST guard regression | unit | `internal/config/sst_grep_guard_test.go` | no hardcoded Ollama literal in prod source | `./smackerel.sh test unit --go --go-run SST_NoHardcodedOllama` | No |
| Live self-hosted discovery re-probe | e2e-api | (self-hosted `GET /v1/agent/model`) | SCN-096-001-A live | self-hosted, post-redeploy | Yes (BLOCKED) |

### Definition of Done

- [x] `ollamaDiscoveryBaseURL(cfg)` resolves from `cfg.OllamaURL`, fail-loud on empty, no compose-DNS default; used in the discovery wiring; dead `ollamaConnectionBaseURL` removed → Evidence: [report.md](report.md) → Test Evidence (diff stat + compile)
- [x] Hermetic contract test (3 cases incl. the adversarial env-seam-vs-registry-param case) passes → Evidence: [report.md](report.md) → Test Evidence (verbose PASS lines, `cmd/core ok`)
- [x] `internal/config` SST hardcoded-Ollama guard stays green after the fix → Evidence: [report.md](report.md) → Test Evidence (`TestSST_NoHardcodedOllamaValues` PASS, `internal/config ok`, `GUARD_EXIT=0`)
- [x] Full-repo `go test ./...` compiles with the removal (no dangling references) → Evidence: [report.md](report.md) → Test Evidence (`go test ./... finished OK`, `UNIT_EXIT=0`)
- [ ] Live self-hosted re-verify: after rebuild+redeploy at the fix SHA, `GET /v1/agent/model` reports `local-ollama` reachable → BLOCKED on a stable self-hosted host with good-neighbor rebuild headroom (see state.json `blockedReason`)
