# BUG-096-001 — Report

## Summary

The SCOPE-04 Ollama **discovery** adapter probed the `local-ollama` connection
registry's `base_url` param (`http://ollama:<port>`, a dev compose-service DNS
name baked into the build-once bundle) instead of the env-wired `cfg.OllamaURL`
seam that `/health` and the ML-sidecar synthesis path use. On the single-host
self-hosted topology (no `ollama` compose service; the daemon is a host
singleton re-pointed by knb via `OLLAMA_URL`), the probe hit NXDOMAIN and
discovery falsely reported `local-ollama: unreachable` while synthesis worked.

The in-repo fix routes the discovery adapter through `ollamaDiscoveryBaseURL(cfg)`
(trimmed `cfg.OllamaURL`, fail-loud on empty, no compose-DNS default) and removes
the now-dead `ollamaConnectionBaseURL`. Three hermetic contract tests (including
the adversarial env-seam-vs-registry-param case) pass; the `internal/config` SST
hardcoded-Ollama guard stays green; the full repo compiles.

## Completion Statement

In-repo scope is complete and proven green this session: the discovery-adapter
base-URL seam is fixed, the dead helper removed, the hermetic contract added and
passing, and the SST guard + full-repo compile confirmed. The remaining item —
live self-hosted re-verification that `GET /v1/agent/model` reports
`local-ollama` reachable — requires a rebuild + redeploy at the fix's source SHA
and is BLOCKED on a stable self-hosted host with good-neighbor rebuild headroom
(the host was memory-oversubscribed by concurrent foreign product load and the
ML sidecar had just been OOM-killed at capture time). This bug is therefore
`blocked`, not `done`; the fix ships in-repo and lands live with the next
096-cohort self-hosted redeploy.

## Test Evidence

### Change surface

```
$ git --no-pager diff --stat
 cmd/core/wiring_assistant_openknowledge.go | 47 ++++++++++++++++++++----------
 1 file changed, 32 insertions(+), 15 deletions(-)
$ git status --porcelain
 M cmd/core/wiring_assistant_openknowledge.go
?? cmd/core/wiring_ollama_discovery_baseurl_test.go
```

### Hermetic contract — 3/3 PASS (verbose)

```
$ ./smackerel.sh test unit --go --go-run 'OllamaDiscoveryBaseURL' --verbose
[go-unit] applying -run selector: OllamaDiscoveryBaseURL
=== RUN   TestOllamaDiscoveryBaseURL_UsesEnvSeamNotRegistryParam
--- PASS: TestOllamaDiscoveryBaseURL_UsesEnvSeamNotRegistryParam (0.00s)
=== RUN   TestOllamaDiscoveryBaseURL_FailsLoudOnEmptySeam
--- PASS: TestOllamaDiscoveryBaseURL_FailsLoudOnEmptySeam (0.00s)
=== RUN   TestOllamaDiscoveryBaseURL_TrimsAndReturnsHostSeam
--- PASS: TestOllamaDiscoveryBaseURL_TrimsAndReturnsHostSeam (0.00s)
PASS
ok      github.com/smackerel/smackerel/cmd/core 0.191s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

### SST hardcoded-Ollama guard stays green after the fix

```
$ ./smackerel.sh test unit --go --go-run 'SST_NoHardcodedOllama|OllamaDiscoveryBaseURL' --verbose
--- PASS: TestSST_NoHardcodedOllamaValues (0.08s)
--- PASS: TestSST_NoHardcodedOllamaValues_Adversarial (0.00s)
--- PASS: TestSST_NoHardcodedOllamaValues_AllowlistAdversarial (0.00s)
ok      github.com/smackerel/smackerel/internal/config  0.098s
[go-unit] go test ./... finished OK
GUARD_EXIT=0
```

### Broader open-knowledge / spec-096 / model-connection regression — compile + run clean

```
$ ./smackerel.sh test unit --go --go-run 'OpenKnowledge|Spec096|ModelConnection|Catalog|Discovery|Ollama|ModelSwitch|Dispatch'
ok      github.com/smackerel/smackerel/cmd/core 0.446s
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog ...
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/llm      ...
ok      github.com/smackerel/smackerel/internal/config  0.098s
[go-unit] go test ./... finished OK
REGRESSION_EXIT=0
```

(The first pre-reword run of this suite surfaced `FAIL TestSST_NoHardcodedOllamaValues`
because the fix's doc comment initially embedded the compose-DNS literal; the
comment was reworded to drop the literal, after which the guard and the full run
went green as shown above.)

### Live self-hosted re-verify — BLOCKED

The live re-probe (`GET /v1/agent/model` on the self-hosted host reporting
`local-ollama` reachable) requires the fix's source SHA to be built and applied
through the knb adapter (8 pre-apply verifications, no bypass). At capture time
the self-hosted host (`<deploy-host>`) was memory-oversubscribed by concurrent foreign
product load and the smackerel ML sidecar had just been OOM-killed; a
good-neighbor rebuild+redeploy was not safe. This item is carried as the
`blocked` remainder in state.json `blockedReason`.

### Validation Evidence

In-repo behavior is validated by the hermetic contract above: the adversarial
case proves discovery resolves the env-wired host seam and NOT the compose-DNS
registry param; the fail-loud case proves an empty seam is a named `OLLAMA_URL`
error with no compose-DNS substitution. Live-stack validation is the `blocked`
remainder.

### Audit Evidence

No production secret, credential, or token is touched by the fix (discovery is
keyless; the change reads `cfg.OllamaURL` only). The smackerel repo stays
generic — the concrete host address lives only in the knb adapter
(`params.yaml`), not in this repo; the hermetic test uses a generic `.invalid`
placeholder hostname in an allowlisted `*_test.go` file. G028 /
smackerel-no-defaults is honored: fail-loud on an empty seam, no compose-DNS
default.
