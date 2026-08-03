# BUG-096-001 — Ollama discovery probe targets compose DNS, not the env-wired deployment seam (shared-host false "unreachable")

- **Parent spec:** [096-multi-provider-model-connections](../../spec.md)
- **Severity:** Medium (shared-host deployments only; degrades the SCOPE-04 model-discovery catalog — `local-ollama` is falsely reported unreachable; synthesis/`/ask` is UNAFFECTED)
- **Surface:** `cmd/core/wiring_assistant_openknowledge.go` → `wireSpec096DiscoveryAndDispatch` (SCOPE-07 activation) → `internal/assistant/openknowledge/catalog` Ollama discovery adapter
- **Status:** `blocked` — in-repo fix + hermetic test landed; live deployed-target re-verification gated on a stable host (see state.json `blockedReason`)

## Symptom

On a deployment that consumes Ollama as a shared host capability outside the
product Compose stack, the SCOPE-04 catalog discovery probe reports
`local-ollama: "unreachable"` even though the shared Ollama daemon IS reachable
and the `/ask` synthesis path answers correctly through it. Concrete target
topology belongs in `<knb-repo>`. `GET /v1/agent/model` surfaces the connection with a
`ProviderDiscoveryStatus` of unreachable, so its live-discovered models are
absent from the unified picker catalog.

## Root cause

Two seams represent the same Ollama endpoint in `cfg`, and they diverge on
deployments that consume a shared host capability:

1. **Env-wired seam** — `cfg.OllamaURL` (`OLLAMA_URL` env). The deploy adapter
   in `<knb-repo>/<product>/<deploy-target>` points this at the shared Ollama
   endpoint supplied for that target. Concrete host addressing and selector
   values belong in `<knb-repo>`; the product stack does not start an Ollama
   Compose service in this topology. The `/health` probe and the ML-sidecar
   synthesis path consume this seam, so they work.
2. **096 registry param** — the `local-ollama` connection's `base_url` from the
   product SST remains a development **compose-service DNS name** in the
   build-once bundle; the target adapter does not rewrite it.

`wireSpec096DiscoveryAndDispatch` built the Ollama discovery adapter from seam
#2 (the registry param). On the deployed topology there is no product-owned
Ollama Compose service, so the probe URL resolves to NXDOMAIN → connect failure
→ a typed `StateUnreachable` `ProviderDiscoveryStatus`. Synthesis was fine
because it rides seam #1 (the sidecar's `OLLAMA_URL`) with `api_base` omitted.

## Fix (in-repo, landed this session)

The Ollama **discovery** adapter now resolves its base URL from the env-wired
`cfg.OllamaURL` seam (new `ollamaDiscoveryBaseURL(cfg)`), the SAME seam
`/health` and synthesis already use — NOT the registry param. Fail-loud on an
empty seam; no compose-DNS default (G028 / smackerel-no-defaults). The now-dead
`ollamaConnectionBaseURL` was removed.

## Reproduction (deployed target, pre-fix — read-only)

```
GET /v1/agent/model  (<deploy-target>, core revision <source-revision>)
 → local-ollama: ProviderDiscoveryStatus = unreachable
    (probe target uses a compose-service address → DNS failure because that service is absent from the deployed product stack)
 → yet POST /v1/agent/invoke synthesis via the same host daemon answers 200
```

Because the fix changes the source SHA, the live end-to-end re-verification
(discovery now reports `local-ollama` reachable on `<deploy-target>`)
requires a rebuild + redeploy and is therefore blocked on a stable deployment
host — see
[report.md](report.md) and state.json `blockedReason`.
