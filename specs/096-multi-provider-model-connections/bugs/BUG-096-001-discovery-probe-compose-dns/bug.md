# BUG-096-001 — Ollama discovery probe targets compose DNS, not the env-wired host seam (self-hosted false "unreachable")

- **Parent spec:** [096-multi-provider-model-connections](../../spec.md)
- **Severity:** Medium (self-hosted only; degrades the SCOPE-04 model-discovery catalog — `local-ollama` is falsely reported unreachable; synthesis/`/ask` is UNAFFECTED)
- **Surface:** `cmd/core/wiring_assistant_openknowledge.go` → `wireSpec096DiscoveryAndDispatch` (SCOPE-07 activation) → `internal/assistant/openknowledge/catalog` Ollama discovery adapter
- **Status:** `blocked` — in-repo fix + hermetic test landed; live self-hosted redeploy-verify gated on a stable host (see state.json `blockedReason`)

## Symptom

On the single-host **self-hosted** deployment, the SCOPE-04 catalog discovery
probe reports `local-ollama: "unreachable"` even though the shared host Ollama
daemon IS reachable and the `/ask` synthesis path answers correctly through it.
`GET /v1/agent/model` surfaces the local Ollama connection with a
`ProviderDiscoveryStatus` of unreachable, so its live-discovered models are
absent from the unified picker catalog.

## Root cause

Two seams represent the same Ollama endpoint in `cfg`, and they diverge on
self-hosted:

1. **Env-wired seam** — `cfg.OllamaURL` (`OLLAMA_URL` env). The deploy adapter
   (knb `smackerel/home-lab`) re-points this to the **host** Ollama daemon
   (`ollama.base_url_host_path`, the host tailnet IP) because on self-hosted the
   local Ollama daemon is a HOST singleton (`sharedServices.ollama: shared`,
   `enable_ollama: false` — no in-stack `ollama` compose service). The `/health`
   probe and the ML-sidecar synthesis path consume this seam, so they work.
2. **096 registry param** — the `local-ollama` connection's `base_url` param
   (`params: '{"base_url":"http://ollama:<port>"}'` in `config/smackerel.yaml`).
   This is a dev **compose-service DNS name** carried verbatim into the
   build-once bundle; it is NOT re-pointed per target.

`wireSpec096DiscoveryAndDispatch` built the Ollama discovery adapter from seam
#2 (the registry param). On the single-host topology there is no `ollama`
compose service, so the probe URL resolves to NXDOMAIN → connect failure → a
typed `StateUnreachable` `ProviderDiscoveryStatus`. Synthesis was fine because
it rides seam #1 (the sidecar's `OLLAMA_URL`) with `api_base` omitted.

## Fix (in-repo, landed this session)

The Ollama **discovery** adapter now resolves its base URL from the env-wired
`cfg.OllamaURL` seam (new `ollamaDiscoveryBaseURL(cfg)`), the SAME seam
`/health` and synthesis already use — NOT the registry param. Fail-loud on an
empty seam; no compose-DNS default (G028 / smackerel-no-defaults). The now-dead
`ollamaConnectionBaseURL` was removed.

## Reproduction (self-hosted, pre-fix — read-only)

```
GET /v1/agent/model  (self-hosted, core rev a7ce6834fddb)
 → local-ollama: ProviderDiscoveryStatus = unreachable
   (probe target http://ollama:<port>/api/tags → NXDOMAIN on the single-host topology)
 → yet POST /v1/agent/invoke synthesis via the same host daemon answers 200
```

Because the fix changes the source SHA, the live end-to-end re-verification
(discovery now reports `local-ollama` reachable on the self-hosted host)
requires a rebuild + redeploy and is therefore blocked on a stable host — see
[report.md](report.md) and state.json `blockedReason`.
