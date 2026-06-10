# Design — 082 MVP / evo-x2 Readiness Hardening

## Current Truth (objective research pass, 2026-06-09)

Recon-confirmed facts from the live tree (solution-blind):

- `config/smackerel.yaml:93` → `chat_ids: "510638591"`, `:106` →
  `user_mapping: "510638591:philip"`. `grep -RIn 510638591` returns exactly
  these two lines.
- `internal/config/config.go::validateModelEnvelopes` (≈ line 2105) builds two
  `envelopeBucket`s (ollama, smackerel_ml) and rejects any single
  `profileMiB > bucket.envelopeMiB`. There is **no** sum check.
- `config-validate` (`cmd/config-validate/main.go`) runs `config.Load()` +
  `cfg.Validate()` on the rendered `<env>.env.tmp` BEFORE atomic-promote, so
  the model-envelope validator runs at `./smackerel.sh config generate` time.
- Generated `config/generated/dev.env` distinct interactive ollama set =
  `{gemma3:4b 4096, qwen2.5:0.5b-instruct 1024}` = 5120 MiB (envelope 8 G);
  full distinct set (incl. reasoning/ocr/photos) = 12544 MiB > 8 G.
- `config/generated/test.env` interactive set = `{qwen2.5:0.5b-instruct 1024}`
  (envelope 8 G); `OLLAMA_KEEP_ALIVE=-1`.
- `environments.home-lab` overrides: `ollama_memory_limit: "20G"`,
  `llm_model/ollama_model/ollama_vision_model = gemma4:26b`,
  `agent_provider_default_model/fast_model = llama3.1:8b`,
  `agent_provider_vision_model = gemma4:26b`. Home-lab interactive distinct set
  = `{gemma4:26b 18432, llama3.1:8b 6144}` = 24576 MiB > 20480 MiB.
- `model_memory_profiles`: `gemma4:26b 18432`, `gemma3:4b 4096`,
  `llama3.1:8b 6144`, `deepseek-r1:7b 4864`, `deepseek-ocr:3b 2560`,
  `deepseek-r1:32b 22528`, `gpt-oss:20b 14336`, `nomic-embed-text 768`,
  `qwen2.5:0.5b-instruct 1024`.
- `docs/Operations.md` "Model Envelope Sizing" catalog claims `gemma4:26b
  (30720 MiB)`, `deepseek-r1:32b (24576 MiB)`, `gpt-oss:20b (16384 MiB)`,
  `nomic-embed-text (350 MiB)`, `deepseek-ocr:3b (3072 MiB)` — all disagree
  with `config/smackerel.yaml`. The same doc already CLAIMS a sum invariant
  ("sum of resident sizes for models routed to ollama MUST NOT exceed
  OLLAMA_MEMORY_LIMIT_MIB ... The validator rejects any overlay that violates
  this") that the code never enforced.
- `deploy/compose.deploy.yml` `smackerel-ml`: `read_only: true`,
  `tmpfs: [/tmp:size=768m...]`, `HF_HOME=/tmp/hf-cache`,
  `SENTENCE_TRANSFORMERS_HOME=/tmp/st-cache` → model re-download on restart.
- `nats-data` volume labelled `com.smackerel.lifecycle: ephemeral`.
- `searxng` `deploy.resources.limits: { memory: 256M }` literal, no `cpus`.
- `externalImages` + compose literals: `pgvector/pgvector:pg16`,
  `nats:2.10-alpine`, `ollama/ollama:rocm` (mutable tags).
- `internal/deploy/external_images_contract_test.go` locks
  compose-literal == contract-literal byte-for-byte (Check 3).
- `scripts/deploy/promote.sh` awk-parses `images:` LIST + `configBundles:`
  LIST (`- name:`, `- env:`). `scripts/commands/build-home-lab.sh` emits
  `images:` MAP (`smackerel-core: "<ref>"`) + singular `configBundle:` OBJECT.
- `deploy/compose.deploy.yml` ollama: `group_add: ["44","993"]`,
  `HSA_OVERRIDE_GFX_VERSION: "11.5.1"`, `HCC_AMDGPU_TARGET: "gfx1151"`.
- Real digests resolved 2026-06-09 (`docker buildx imagetools inspect`):
  - `pgvector/pgvector:pg16` → `sha256:00ba258a66dac104fd5171074a0084462a64a1369d8513f3d0a634e2f24d15bc`
  - `nats:2.10-alpine` → `sha256:b83efabe3e7def1e0a4a31ec6e078999bb17c80363f881df35edc70fcb6bb927`
  - `ollama/ollama:rocm` → `sha256:e658cf94b88ef88aa0868bc5900e6f83ccf77262ef2ca582601161f865a2b080`

## Design Brief

Nine bounded, independent changes. The only one with non-trivial semantics is
Scope 2 (concurrent-envelope guard); the rest are mechanical contract/config/
doc changes each locked by a test.

---

## 2. Scope designs

### SCOPE-082-01 — Telegram default identity blanking

- `config/smackerel.yaml`: `chat_ids: ""` and `user_mapping: ""`. The existing
  comment block already documents the empty mapping as the dev/test default
  and that production refuses unmapped chats.
- Verify the runtime treats empty `TELEGRAM_CHAT_IDS` as "no recipient":
  inspect `internal/telegram` + `internal/config` parsing. `parseTelegramUserMapping("")`
  already returns `(nil, nil)`. The Telegram bridge is gated by
  `telegram.bot_token` being non-empty; with an empty token and empty chat_ids
  the bridge stays inert. No startup crash.
- Regenerate `config/generated/dev.env` via `./smackerel.sh config generate`.
- Evidence: `grep -RIn 510638591` → 0 lines; `config generate` exit 0;
  `test unit` for telegram/config green.

### SCOPE-082-02 — Ollama concurrent-envelope guard + figure reconcile

**Key decision — the guard sums the distinct INTERACTIVE HOT-PATH models, not
every ollama-bucket model.** Rationale: the per-model individual check already
covers every slot; `config-validate` runs the full validator at generate time,
so a naive sum-all check would reject the *currently-green* dev (12544 > 8192)
and break `config generate`. The models guaranteed co-resident during live
interactive/agent use are the conversational hot path:
`LLM_MODEL`, `OLLAMA_MODEL`, `OLLAMA_VISION_MODEL`,
`AGENT_PROVIDER_DEFAULT_MODEL`, `AGENT_PROVIDER_FAST_MODEL`,
`AGENT_PROVIDER_VISION_MODEL`. On-demand specialists (reasoning, OCR,
photo-intelligence batch) are governed by the individual check + keep-alive
guidance. This subset:
- dev interactive sum 5120 ≤ 8192 ✅ (no false positive)
- test interactive sum 1024 ≤ 8192 ✅
- home-lab interactive sum 24576 > 20480 ❌ → caught.

**Resident gate.** The sum constraint exists *because* keep-alive retains
models. The guard enforces the sum only when keep-alive is "resident":
`OLLAMA_KEEP_ALIVE == "-1"` OR a Go duration ≥ 10 minutes. Short keep-alive
(e.g. `5m`) evicts between sporadic uses, so only the individual check applies.
dev/test/home-lab all use `24h`/`-1` → resident → enforced.

**Implementation** (`internal/config/config.go`):
- Add `OllamaKeepAlive string` to `Config` if absent (read from
  `OLLAMA_KEEP_ALIVE`); add a helper `ollamaKeepAliveResident(raw) bool`.
- In `validateModelEnvelopes`, after the individual loop, compute the distinct
  interactive set (dedupe by model name) and its summed `profileMiB`; if
  `OllamaMemoryLimitMiB > 0` AND resident AND `sum > OllamaMemoryLimitMiB`,
  append a fail-loud error naming the resident set, the sum, and
  `OLLAMA_MEMORY_LIMIT`.
- Reuse the existing combined-error envelope (`spec 045 FR-045-002 | ...`)
  with a new segment `concurrent ollama envelope exceeded: ...`.

**Config fix.** Raise `environments.home-lab.ollama_memory_limit` from `"20G"`
to `"28G"` (24576 interactive sum + ~4 GiB headroom for KV-cache growth; the
evo-x2 host has ~109 GiB). Document the new floor.

**Figure reconcile** (`docs/Operations.md`): correct the catalog to match the
SST authority — `gemma4:26b 18432`, `deepseek-r1:32b 22528`, `gpt-oss:20b
14336`, `nomic-embed-text 768`, `deepseek-ocr:3b 2560` — and add a subsection
documenting the new concurrent-envelope guard + the home-lab `28G` floor.

**Tests** (`internal/config/validate_ml_envelope_test.go`, additive):
- `TestValidate_RejectsOversubscribedInteractiveOllamaSet` — two models each
  individually fitting a 20G envelope but summing to 24576 with resident
  keep-alive → reject. **Adversarial:** this test fails if the sum branch is
  removed.
- `TestValidate_AcceptsFittingInteractiveOllamaSum` — 5120 sum, 8G envelope →
  accept (no false positive).
- `TestValidate_SumGuardRelaxedForNonResidentKeepAlive` — oversubscribed set
  but `OLLAMA_KEEP_ALIVE=5m` → accept (proves the resident gate).

### SCOPE-082-03 — Embedding-model cache persistence

> **DISC-082-01 (2026-06-10, implementation refinement):** The ML image runs
> as non-root `USER smackerel` and **bakes the embedding model into
> `/home/smackerel/.cache`** (owned `smackerel:smackerel`) at build time
> (`ml/Dockerfile:79-89`: `ENV HF_HOME=/home/smackerel/.cache/huggingface`,
> `COPY --from=builder --chown=smackerel:smackerel /opt/hf-cache
> /home/smackerel/.cache`). The deploy compose OVERRIDES `HF_HOME=/tmp/hf-cache`
> onto ephemeral tmpfs, which both ignores the baked-in model AND loses it on
> restart — THAT is the actual re-download cause. Mounting at the originally
> planned `/var/cache/smackerel-models` would be root-owned (unwritable by the
> non-root user) and start empty (still a download). **Corrected approach:**
> mount the persistent `ml-model-cache` volume at the image's baked cache path
> `/home/smackerel/.cache`. Docker initializes a fresh named volume FROM the
> image content (model present, correct `smackerel` ownership) on first mount,
> so there is no download and no permission failure, and read-only root is
> satisfied because writes land on the volume.

- `deploy/compose.deploy.yml` `smackerel-ml`: add a persistent named volume
  `ml-model-cache` mounted at `/home/smackerel/.cache`; point `HF_HOME`
  and `SENTENCE_TRANSFORMERS_HOME` at the `huggingface` / `sentence-transformers`
  subdirectories of that mount (matching the image's baked ENV). Keep
  `read_only: true`; shrink the `/tmp` tmpfs to 128M (now only uvicorn scratch).
- Declare `ml-model-cache` under top-level `volumes:` with
  `name: ${ML_MODEL_CACHE_VOLUME_NAME}` (SST per-env volume name, mirroring
  the postgres/nats/ollama pattern) + a `com.smackerel.lifecycle: persistent`
  label. Emit `ML_MODEL_CACHE_VOLUME_NAME` from `config.sh` per env
  (`smackerel-<env>-ml-model-cache`).
- Update `internal/deploy/compose_filesystem_contract_test.go` so the ML
  read-only-root assertion expects the model-cache mount + the cache-env
  overrides pointing into it (not `/tmp`). Adversarial sub-test: a regression
  pointing `HF_HOME` back at `/tmp` is rejected.

### SCOPE-082-04 — NATS volume durability + clean protection

- `deploy/compose.deploy.yml`: relabel `nats-data`
  `com.smackerel.lifecycle: persistent` (JetStream holds at-least-once
  in-flight capture state). The smackerel.sh clean logic preserves volumes
  whose project/label marks them persistent; the relabel makes
  `clean`/`down` never target `nats-data` for removal on a running home-lab
  stack.
- Add `internal/deploy/nats_volume_lifecycle_contract_test.go`: assert the
  `nats-data` volume's `com.smackerel.lifecycle` label == `persistent`.
  Adversarial: a regression to `ephemeral` is rejected. (Parses the live
  compose, same pattern as the other deploy contract tests.)

### SCOPE-082-05 — SearxNG SST resource limits

- `config/smackerel.yaml` `deploy_resources`: add `searxng: { cpus: "1.0",
  memory: "256M" }` (memory preserves the current literal; cpus is the new
  explicit cap).
- `scripts/commands/config.sh`: emit `SEARXNG_CPU_LIMIT` /
  `SEARXNG_MEMORY_LIMIT` via `required_value deploy_resources.searxng.*`.
- `deploy/compose.deploy.yml` searxng: replace the literal `memory: 256M`
  with `cpus: "${SEARXNG_CPU_LIMIT:?...}"` + `memory:
  "${SEARXNG_MEMORY_LIMIT:?...}"`.
- `internal/deploy/compose_resource_contract_test.go`: extend the resource
  contract assertion to include `searxng` so it's locked exactly like the
  other services. Adversarial: a literal regresses → rejected.

### SCOPE-082-06 — Pin third-party infra images by digest

- `deploy/contract.yaml` `externalImages`: pin
  `pgvector/pgvector:pg16@sha256:00ba…`, `nats:2.10-alpine@sha256:b83e…`,
  `ollama/ollama:rocm@sha256:e658…`. contract.yaml is the deploy SST surface
  for third-party image identity (its file header: "the contract the build
  pipeline produces and the deploy adapter consumes").
- `deploy/compose.deploy.yml`: pin the same three literals byte-for-byte.
- `internal/deploy/external_images_contract_test.go`: the existing Check 3
  already locks byte-for-byte. ADD a new check + adversarial test: every
  literal (non-`${}`) external image whose name is in the production trio
  MUST contain `@sha256:`; a bare-tag regression is rejected.
- The adversarial sub-tests in that file that use synthetic bare-tag images
  (`ollama/ollama:0.23.2`, etc.) stay valid because they construct their own
  in-memory docs and don't read the live files.

### SCOPE-082-07 — Build-manifest schema convergence

- `scripts/deploy/promote.sh`: detect manifest shape and parse both:
  - CI list shape: `images:` list (`- name: smackerel-core` → `ref:`),
    `configBundles:` list (`- env: <env>` → `ref:`/`sha256:`).
  - local-operator map/object shape: `images:` map
    (`smackerel-core: "<ref>"`), `configBundle:` object (`ref:`/`env:`/`sha256:`).
  - Detection: if a top-level `configBundle:` (singular) block exists →
    map/object shape; else list shape. Extraction helpers try list-shape awk
    first, fall back to map/object awk. Fail loud if neither yields a value.
- Add `scripts/deploy/promote_manifest_parse_test.sh` (a bash test using
  fixtures) OR a Go test `internal/deploy/promote_manifest_parse_test.go` that
  drives the extraction helpers against both fixture shapes and asserts equal
  extracted values. Adversarial: a malformed manifest (neither shape) fails.
  Chosen: a self-contained bash test (`bash scripts/deploy/promote_manifest_parse_test.sh`)
  that sources the parse helpers refactored out of promote.sh.

### SCOPE-082-08 — Operator go-live checklist + spec-review addendum

- `docs/Deployment.md`: add a "Go-Live Readiness Checklist" section tying
  together: spec 051 five production secrets; spec 052 L2 knb secret-injection
  dependency; spec 017 local-operator vs CI trust
  (`./smackerel.sh build --target home-lab`); `--profile ollama` /
  `--profile monitoring` / `--profile searxng` enablement; backup +
  restore-drill sequencing before any release-train promote; the
  supervised-canary first apply.
- `specs/_spec-review-report.md`: append a dated addendum (2026-06-09)
  recording portfolio drift (~81 specs; spec 058 now blocked, spec 076 now
  done) WITHOUT rewriting the historical 2026-06-02 body.
- `internal/deploy/golive_checklist_docs_contract_test.go`: assert
  `docs/Deployment.md` contains the checklist anchors (the five secrets,
  spec 051/052/017 references, the three profiles, backup/restore sequencing,
  supervised canary). Adversarial: removing an anchor fails.

### SCOPE-082-09 — ROCm host-specific literals assessment + routing

- **Assessment (recorded here):** `HSA_OVERRIDE_GFX_VERSION=11.5.1` +
  `HCC_AMDGPU_TARGET=gfx1151` describe the *GPU class* (Strix Halo / Radeon
  8060S, gfx1151) that the home-lab profile targets generically — they are a
  reasonable generic default for the known EVO-X2-class accel tier and are NOT
  operator-identifying. They stay, documented.
- `group_add: ["44","993"]` are **host render/video GIDs** — these DO vary per
  host (a different distro/host can assign different render/video GIDs) and are
  exactly the "value that changes when a different operator deploys" the policy
  forbids in the generic compose. **Decision: route them to an adapter-supplied
  fail-loud env.** Replace the literals with
  `group_add: ["${OLLAMA_RENDER_GID:?...}", "${OLLAMA_VIDEO_GID:?...}"]`. The
  deploy adapter MUST set both in `app.env`; missing → compose aborts (Gate
  G028, no silent default).
- Document the contract in `docs/Deployment.md` (adapter MUST emit
  `OLLAMA_RENDER_GID`/`OLLAMA_VIDEO_GID`) and in the compose comment.
- `internal/deploy/ollama_gpu_group_add_contract_test.go`: assert the ollama
  `group_add` entries use the `${OLLAMA_RENDER_GID:?...}` / `${OLLAMA_VIDEO_GID:?...}`
  fail-loud form (no bare numeric literal). Adversarial: a numeric-literal
  regression is rejected. Also assert the gfx env literals remain (generic
  default), pinning the documented decision.

## 6. Test Plan (cross-scope)

| Scope | Test surface | Type | Adversarial? |
|-------|--------------|------|--------------|
| 01 | `grep` + telegram/config unit | unit | n/a |
| 02 | `validate_ml_envelope_test.go` (3 new) | unit | yes (sum-removed fails) |
| 03 | `compose_filesystem_contract_test.go` | unit (contract) | yes (HF_HOME→/tmp fails) |
| 04 | `nats_volume_lifecycle_contract_test.go` | unit (contract) | yes (ephemeral fails) |
| 05 | `compose_resource_contract_test.go` | unit (contract) | yes (literal fails) |
| 06 | `external_images_contract_test.go` | unit (contract) | yes (bare-tag fails) |
| 07 | `promote_manifest_parse_test.sh` | shell test | yes (malformed fails) |
| 08 | `golive_checklist_docs_contract_test.go` | unit (contract) | yes (missing anchor fails) |
| 09 | `ollama_gpu_group_add_contract_test.go` | unit (contract) | yes (numeric literal fails) |

All Go unit/contract tests run under `./smackerel.sh test unit`. SST changes
verified via `./smackerel.sh config generate` (dev + test). Gate suite:
`check`, `lint`, `format --check`, then `artifact-lint` + `traceability-guard`.
