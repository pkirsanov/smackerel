# BUG-096-001 — User Validation

In-repo items are checked `[x]` (verified with real commands this session, raw
evidence in [report.md](report.md)). The live-stack item is unchecked `[ ]`
until the fix is redeployed and re-probed on a stable self-hosted host.

## Checklist

- [x] Ollama discovery resolves its probe URL from the env-wired `cfg.OllamaURL` seam (not the compose-DNS registry param)
- [x] An empty `OLLAMA_URL` fails loud with a named error — no compose-DNS default (G028)
- [x] Dev parity preserved: a well-formed seam is trimmed and returned verbatim
- [x] The dead `ollamaConnectionBaseURL` helper is removed and the full repo still compiles
- [x] The `internal/config` SST hardcoded-Ollama guard stays green after the fix
- [ ] Live self-hosted: `GET /v1/agent/model` reports `local-ollama` reachable after rebuild+redeploy (BLOCKED on a stable host — see state.json `blockedReason`)
