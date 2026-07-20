# BUG-096-001 — Specification: Ollama discovery MUST probe the env-wired host seam

## Expected behavior

**FR-1 — Discovery uses the env-wired Ollama seam.** The SCOPE-04 Ollama
discovery adapter (`GET <base>/api/tags`) MUST resolve its base URL from the
same env-wired seam the `/health` probe and the ML-sidecar synthesis path use
(`cfg.OllamaURL`, sourced from the REQUIRED `OLLAMA_URL` / `llm.ollama_url` SST
value). It MUST NOT read the `local-ollama` connection registry's `base_url`
param, because that param is a fixed dev compose-service DNS name carried
verbatim in the build-once bundle and is not re-pointed per deployment target.

**FR-2 — Fail loud, no compose-DNS default.** When the env-wired seam is empty
/ whitespace, discovery-adapter construction MUST fail loud with a NAMED error
that identifies `OLLAMA_URL`. It MUST NOT substitute a compose-DNS default and
MUST NOT silently fall back to the registry param (G028 /
smackerel-no-defaults).

**FR-3 — Dev parity preserved.** On the dev/compose topology (where Ollama IS a
compose service and `OLLAMA_URL` is the compose-service URL), discovery MUST
continue to probe successfully — the fix changes WHICH cfg field feeds the
probe, not the dev value.

**FR-4 — Synthesis / trust perimeter unchanged.** The fix is confined to the
discovery-adapter base-URL resolution. The `/ask` synthesis dispatch, the
provider-aware dispatch resolver, the 088/089 switch precedence, the per-user
store, and the credential/secret handling are untouched.

## Acceptance scenarios (Gherkin)

```gherkin
Scenario: SCN-096-001-A — discovery follows the env-wired host seam on self-hosted
  Given a self-hosted deployment where OLLAMA_URL points at the host Ollama daemon
    And the local-ollama connection registry base_url is the dev compose-DNS name
  When the SCOPE-04 discovery adapter is constructed
  Then it probes the host daemon (OLLAMA_URL), NOT the compose-DNS registry param
    And local-ollama's installed models appear in the unified catalog as reachable

Scenario: SCN-096-001-B — empty seam fails loud with no default
  Given OLLAMA_URL is empty or whitespace
  When the discovery adapter base URL is resolved
  Then resolution fails with a NAMED error identifying OLLAMA_URL
    And no compose-DNS default is substituted

Scenario: SCN-096-001-C — dev parity preserved
  Given a dev deployment where OLLAMA_URL is the compose-service Ollama URL
  When the discovery adapter is constructed
  Then it probes that compose-service URL and discovery succeeds
```

## Out of scope

- The provider-aware dispatch resolver's Ollama `base_url` param mapping (bare /
  Ollama synthesis stays on the 089 nil-`api_base` path per the existing wiring
  contract; the sidecar's `OLLAMA_URL` is the effective endpoint there).
- Any change to `config/smackerel.yaml`, the config generator, or the knb
  bundle/params (the env-wired seam is already correctly provisioned by knb; the
  defect was purely which cfg field the discovery adapter consumed).
- Hosted-provider (Anthropic/OpenAI/…) discovery, which serves SST-curated
  models and does not probe a live URL.
