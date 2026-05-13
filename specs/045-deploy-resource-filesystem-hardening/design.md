# Design: Deploy Resource and Filesystem Hardening

## Current Truth

The review identified three hardening gaps: service CPU envelopes are not explicit, the ML model memory posture is not contractually tied to a deployment resource class, and container writable surfaces are not constrained with a read-only root filesystem contract.

This design keeps the product/adapter boundary intact. Smackerel should publish resource and filesystem requirements through its SST and deploy contract surfaces. The target adapter decides how those requirements map to the host.

## Proposed Design

### Resource Envelope

#### SST Schema (`config/smackerel.yaml`)

A new top-level `deploy_resources` block declares CPU and memory envelopes per service. Every key is required (fail-loud SST):

```yaml
deploy_resources:
  postgres:
    cpus: "1.0"
    memory: "1G"
  nats:
    cpus: "0.5"
    memory: "512M"
  smackerel_core:
    cpus: "2.0"
    memory: "1G"
  smackerel_ml:
    cpus: "2.0"
    memory: "3G"
  ollama:
    cpus: "4.0"
    memory: "8G"
```

#### SST Generator (`scripts/commands/config.sh`)

The generator extracts each `deploy_resources.<service>.cpus` and `.memory` value via `required_value` and emits them as env vars (`POSTGRES_CPU_LIMIT`, `POSTGRES_MEMORY_LIMIT`, `NATS_CPU_LIMIT`, `NATS_MEMORY_LIMIT`, `CORE_CPU_LIMIT`, `CORE_MEMORY_LIMIT`, `ML_CPU_LIMIT`, `ML_MEMORY_LIMIT`, `OLLAMA_CPU_LIMIT`, `OLLAMA_MEMORY_LIMIT`) into `config/generated/<env>.env`.

#### Compose Substitution (`deploy/compose.deploy.yml`)

Every `deploy.resources.limits.{cpus,memory}` block uses `${VAR:?error}` fail-loud substitution. Hardcoded literals (`memory: 1G`) are removed and replaced with substituted values from the bundled env file.

### ML Model Envelope

#### SST Schema

```yaml
services:
  ml:
    # ... existing keys ...
    model_memory_profiles:
      "gemma4:26b": 16384       # 16 GB
      "gemma3:9b": 6144
      "deepseek-ocr:3b": 2048
      "deepseek-r1:32b": 22528
      "gpt-oss:20b": 14336
      "nomic-embed-text": 512
```

The active model — sourced from `llm.ollama_model` — is validated at Go core startup against the `ML_MEMORY_LIMIT` envelope. The profile MUST contain an entry for every model name referenced by the runtime configuration.

#### Validation

`internal/config/config.go` Validate() chain adds:

1. Parse `ML_MEMORY_LIMIT` (e.g. `3G` → 3072 MiB)
2. For each configured model name (`OLLAMA_MODEL`, `OLLAMA_VISION_MODEL`, `OLLAMA_OCR_MODEL`, `OLLAMA_REASONING_MODEL`, `OLLAMA_FAST_MODEL`, `EMBEDDING_MODEL`), look up its required-memory profile.
3. If a profile is missing for any used model, fail-loud (the operator MUST declare every model's memory needs).
4. If any used model's required memory exceeds the ML envelope, fail-loud naming the model + required + envelope MiB.

The profile and envelope reach Go via two new env vars: `ML_MEMORY_LIMIT` and `ML_MODEL_MEMORY_PROFILES_JSON` (JSON object emitted by `config.sh` from `services.ml.model_memory_profiles`).

### Filesystem Envelope

#### Read-only Root Set

| Service        | `read_only: true` | Rationale                                              |
|----------------|------------------|--------------------------------------------------------|
| `smackerel-core` | yes              | Stateless Go binary; no persistent root writes         |
| `smackerel-ml`   | yes              | Stateless FastAPI sidecar; needs tmpfs for HF cache    |
| `ollama`         | yes              | Model store lives in named volume `ollama-data`        |
| `postgres`       | NO               | pg writes WAL/control files outside `/var/lib/postgresql/data` |
| `nats`           | NO               | NATS server JetStream writes need broader root access  |

Postgres and NATS exemption is documented inside the compose file and enforced by the contract test allowlist.

#### Tmpfs Mounts (read-only services only)

| Service        | tmpfs mount(s)                                  | Notes |
|----------------|------------------------------------------------|-------|
| `smackerel-core` | `/tmp` (size: 64M)                             | Stateless Go binary; tmp space for log buffering only. |
| `smackerel-ml`   | `/tmp` (size: 768M)                            | Single tmpfs covers `/tmp` AND HuggingFace cache (compose env overrides `HF_HOME=/tmp/hf-cache`, `SENTENCE_TRANSFORMERS_HOME=/tmp/st-cache` so the existing tmpfs absorbs both). 768M sized for sentence-transformers + nomic-embed-text. |
| `ollama`         | `/tmp` (size: 64M); `/.ollama_tmp` (size: 64M) | Persistent model store stays on `ollama-data` named volume. |

The named volume `ollama-data:/root/.ollama` is preserved as a writable named-volume mount (not tmpfs) so model downloads survive restarts.

#### Compose Contract Test (`internal/deploy/compose_contract_test.go`)

A new assertion function `assertResourceAndFilesystemContract(yamlBytes)` checks:

1. Every contract service has non-empty `deploy.resources.limits.cpus` and `deploy.resources.limits.memory`. Hardcoded literal numeric forms still pass (the contract is "the value exists"), but the generated values must be present.
2. Every read-only-root service (`smackerel-core`, `smackerel-ml`, `ollama`) has `read_only: true`.
3. Postgres and NATS MUST NOT have `read_only: true` (regression guard against accidentally locking out stateful services).
4. For each read-only service, every entry in `tmpfs:` MUST be in the allowed-writable allowlist for that service.
5. All Spec 042 invariants remain enforced by the existing assertion (re-run for safety).

Adversarial sub-tests construct mutated compose docs that should each fail the contract:
- A service missing `cpus` limit
- A service missing `memory` limit
- `smackerel-core` missing `read_only: true`
- `postgres` accidentally set `read_only: true`
- `smackerel-ml` introducing an unauthorized writable mount

### Spec 051 Coordination (Secret Loading Under read-only-root)

Spec 051 added defense-in-depth secret loading via env vars and SST-resolved `app.env`. Read-only root does NOT regress this because:
- `env_file: ./app.env` is READ-only on container side (compose mounts the file readable, no write needed).
- All secret consumers (auth_token, llm.api_key, telegram.bot_token) read from process env — no filesystem writes required.
- The `prompt_contracts/` mount is `:ro`; ML sidecar never writes to it.

The `internal/config/secrets.go` dev-default rejection (FR-051-005) is unaffected; it only reads env vars.

### Spec 042 Coordination (Tailnet-Edge Bind Pattern)

The existing `assertComposeContract()` function MUST continue to pass unchanged. The new `assertResourceAndFilesystemContract()` is additive — it does not touch port mappings or `network_mode`. The existing live-file test continues to enforce the HOST_BIND_ADDRESS fail-loud form on smackerel-core / smackerel-ml and the no-host-port invariant on postgres / nats.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-045-001 | unit/config | Validate `deploy_resources.*` SST values are required (missing key → fail-loud at config generate). |
| T-045-002 | contract | `internal/deploy/compose_contract_test.go` extension: every contract service has `deploy.resources.limits.cpus` and `memory`. |
| T-045-003 | unit/config | `internal/config/config.go` `Validate()` rejects ML model whose required memory exceeds `ML_MEMORY_LIMIT`. |
| T-045-004 | contract | Every read-only-root-required service declares `read_only: true`; postgres/nats are NOT read-only. |
| T-045-005 | contract | Read-only services' tmpfs mounts match the allowlist. |
| T-045-006 | adversarial | Mutated compose fixtures (missing limit, missing read_only, unauthorized tmpfs) MUST fail the contract assertion. |
| T-045-007 | live-stack smoke | `./smackerel.sh up` brings the stack up under hardened limits and `/api/health` returns 200. |
| T-045-008 | artifact | Artifact lint passes for this packet. |

## Risk Controls

- Keep writable stateful-service data paths explicit and untouched by read-only-root tightening.
- Preserve generated config ownership: no hand-edited generated files.
- Treat model-envelope failure as startup validation, not runtime degradation.
