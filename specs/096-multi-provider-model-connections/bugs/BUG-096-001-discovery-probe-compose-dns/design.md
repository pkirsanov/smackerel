# BUG-096-001 — Design: fix the discovery-adapter base-URL seam

## Root-cause chain (verified)

| # | Fact | Evidence |
|---|------|----------|
| 1 | On `<target>`, the operator-owned deployment adapter writes `OLLAMA_URL` **and** `OLLAMA_BASE_URL` with the target-provided endpoint in the core `app.env` | `<knb-repo>/<product>/<target>/lib/target-env.sh` writes both variables when the shared Ollama capability is selected; `<knb-repo>/<product>/<target>/params.yaml` owns the endpoint value |
| 2 | The Go core reads `cfg.OllamaURL` from `OLLAMA_URL` | `internal/config/config.go:637` `OllamaURL: os.Getenv("OLLAMA_URL")` |
| 3 | `/health` + ML-sidecar synthesis consume the env-wired seam, so they follow the deployed target configuration | `internal/api/health.go` (`OllamaURL`); synthesis sends `api_base` omitted → sidecar uses its own `OLLAMA_URL` (`internal/assistant/openknowledge/llm/client.go` ChatRequest `APIBase` doc) |
| 4 | The discovery adapter was built from the connection registry `base_url` param (a development compose-DNS endpoint baked into the build-once bundle) | pre-fix `cmd/core/wiring_assistant_openknowledge.go` `ollamaConnectionBaseURL(conn)` → `catalog.NewOllamaAdapter(conn.ID, baseURL, …)` |
| 5 | A target that consumes the shared Ollama capability has no product-local `ollama` compose service, so probing the development compose-DNS endpoint returns NXDOMAIN and `StateUnreachable` | `<knb-repo>/<product>/<target>/params.yaml` selects the shared capability |

**Divergence:** two cfg seams describe the same Ollama endpoint. Seam #1
(`cfg.OllamaURL`) is env-wired and re-pointed per target by the operator-owned
deployment adapter; seam #2 (the 096 registry `base_url` param) is a fixed
development compose endpoint. Discovery used seam #2; everything else used seam #1.

## Fix design (DD-1..DD-3)

**DD-1 — Resolve discovery from `cfg.OllamaURL`.** Add
`ollamaDiscoveryBaseURL(cfg *config.Config) (string, error)` returning the
trimmed `cfg.OllamaURL`, fail-loud (named `OLLAMA_URL` error) on empty. Use it
in `wireSpec096DiscoveryAndDispatch` for the Ollama-kind adapter in place of
`ollamaConnectionBaseURL(conn)`.

**DD-2 — Remove the dead helper.** `ollamaConnectionBaseURL` had exactly one
caller (the discovery wiring); after DD-1 it is dead. Remove it to avoid a
dead-code lint finding and to eliminate the compose-DNS seam from the discovery
path entirely.

**DD-3 — Preserve the registry param elsewhere.** The `base_url` param stays a
REQUIRED Ollama registry param (`modelConnectionRequiredParams`) — it still
documents the connection and feeds the dispatch-contract mapping. Only the live
**discovery probe** stops consuming it. No `config/smackerel.yaml`, config
generator, or target-adapter change is needed: the env-wired seam is already
correct in development and on every configured `<target>`.

## Why the code layer (not the config/bundle layer)

The env-wired seam (`OLLAMA_URL` → `cfg.OllamaURL`) is ALREADY correctly
provisioned on both topology classes: development receives the compose-service
URL, while each deployed `<target>` receives the adapter-provided endpoint from
`<knb-repo>/<product>/<target>/...`. `/health` and synthesis already consume it.
The single outlier was the discovery adapter reading the un-re-pointed registry
param. The minimal, correct fix is to make discovery follow the same seam its
sibling probes use — a code change in the wiring, not a new per-environment
override of the registry param, which would duplicate the endpoint in a second
place and invite drift.

## Test design

Pure hermetic contract on `ollamaDiscoveryBaseURL(cfg)` in package `main`
(`cmd/core`), no DB / network / singletons:

- **Adversarial (core proof):** cfg with `OllamaURL` set to a target-wired
  endpoint plus a `local-ollama` connection whose registry
  `base_url` = development compose-DNS endpoint → resolution returns the
  target-wired endpoint and asserts it is NOT the compose-DNS param. A regression
  that re-reads the connection param fails this test.
- **Fail-loud:** `OllamaURL` empty / whitespace → NAMED `OLLAMA_URL` error,
  empty return, no compose-DNS literal in the error.
- **Happy/trim:** a whitespace-padded target-wired endpoint trims and returns
  verbatim.

Regression safety: the `internal/config` SST hardcoded-Ollama guard
(`TestSST_NoHardcodedOllamaValues`) scans production `.go` for the forbidden
literal — the fix comment was worded to avoid it (test file is allowlisted).
