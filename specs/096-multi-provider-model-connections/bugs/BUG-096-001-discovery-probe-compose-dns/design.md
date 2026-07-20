# BUG-096-001 — Design: fix the discovery-adapter base-URL seam

## Root-cause chain (verified)

| # | Fact | Evidence |
|---|------|----------|
| 1 | On self-hosted, knb writes `OLLAMA_URL` **and** `OLLAMA_BASE_URL` = host tailnet IP into the core `app.env` | `knb/smackerel/home-lab/lib/target-env.sh` (`KNB_SHARED_OLLAMA=shared` → `knb_target_env_write_line OLLAMA_URL "$value"`), `params.yaml` `ollama.base_url_host_path` |
| 2 | The Go core reads `cfg.OllamaURL` from `OLLAMA_URL` | `internal/config/config.go:637` `OllamaURL: os.Getenv("OLLAMA_URL")` |
| 3 | `/health` + ML-sidecar synthesis consume the env-wired seam (so they work on self-hosted) | `internal/api/health.go` (`OllamaURL`); synthesis sends `api_base` omitted → sidecar uses its own `OLLAMA_URL` (`internal/assistant/openknowledge/llm/client.go` ChatRequest `APIBase` doc) |
| 4 | The discovery adapter was built from the connection registry `base_url` param (`http://ollama:<port>`, dev compose DNS, baked into the build-once bundle) | pre-fix `cmd/core/wiring_assistant_openknowledge.go` `ollamaConnectionBaseURL(conn)` → `catalog.NewOllamaAdapter(conn.ID, baseURL, …)` |
| 5 | On the single-host topology there is no `ollama` compose service (`enable_ollama: false`, `sharedServices.ollama: shared`) → probe URL = NXDOMAIN → `StateUnreachable` | `knb/smackerel/home-lab/params.yaml` |

**Divergence:** two cfg seams describe the same Ollama endpoint. Seam #1
(`cfg.OllamaURL`) is env-wired and re-pointed per target by knb; seam #2 (the
096 registry `base_url` param) is a fixed dev literal. Discovery used seam #2;
everything else used seam #1.

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
generator, or knb change is needed: the env-wired seam is already correct on
both dev and self-hosted.

## Why the code layer (not the config/bundle layer)

The env-wired seam (`OLLAMA_URL` → `cfg.OllamaURL`) is ALREADY correctly
provisioned on both topologies (dev: compose-service URL; self-hosted: host IP
via knb). `/health` and synthesis already consume it. The single outlier was the
discovery adapter reading the un-re-pointed registry param. The minimal, correct
fix is to make discovery follow the same seam its sibling probes use — a code
change in the wiring, not a new per-env override of the registry param (which
would duplicate the endpoint in a second place and invite drift).

## Test design

Pure hermetic contract on `ollamaDiscoveryBaseURL(cfg)` in package `main`
(`cmd/core`), no DB / network / singletons:

- **Adversarial (core proof):** cfg with `OllamaURL` = host seam AND a
  `local-ollama` connection whose registry `base_url` = compose-DNS literal →
  resolution returns the host seam, and asserts it is NOT the compose-DNS param.
  A regression that re-reads the connection param fails this test.
- **Fail-loud:** `OllamaURL` empty / whitespace → NAMED `OLLAMA_URL` error,
  empty return, no compose-DNS literal in the error.
- **Happy/trim:** a whitespace-padded host seam trims and returns verbatim.

Regression safety: the `internal/config` SST hardcoded-Ollama guard
(`TestSST_NoHardcodedOllamaValues`) scans production `.go` for the forbidden
literal — the fix comment was worded to avoid it (test file is allowlisted).
