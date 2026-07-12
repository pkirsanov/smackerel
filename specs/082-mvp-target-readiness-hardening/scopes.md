# Scopes — 082 MVP / <deploy-host> Readiness Hardening

**Pattern:** independent-hardening-batch (nine cleanly-separated scopes; no
shared mutable surface except `deploy/compose.deploy.yml`, which each scope
edits in a distinct, non-overlapping region).

**Design cross-reference:** [design.md](design.md).

## Execution Outline

- **SCOPE-082-01** — Telegram default identity blanking (config SST).
- **SCOPE-082-02** — Ollama concurrent-envelope guard + figure reconcile (Go validator + config + docs).
- **SCOPE-082-03** — Embedding-model cache persistence (compose + filesystem contract).
- **SCOPE-082-04** — NATS volume durability + clean protection (compose + contract).
- **SCOPE-082-05** — SearxNG SST resource limits (config + compose + contract).
- **SCOPE-082-06** — Pin third-party infra images by digest (contract.yaml + compose + contract test).
- **SCOPE-082-07** — Build-manifest schema convergence (promote.sh + shell test).
- **SCOPE-082-08** — Operator go-live checklist + spec-review addendum (docs + contract).
- **SCOPE-082-09** — ROCm host-specific literals assessment + routing (compose + docs + contract).

**Validation checkpoint:** after each scope, run the scope's targeted test;
after all scopes, `./smackerel.sh config generate`, `check`, `lint`,
`format --check`, `test unit` green, then `artifact-lint` +
`traceability-guard`.

## Inter-Spec Dependencies

<!-- bubbles:g040-skip-begin -->
| Direction | Spec | Relationship |
|-----------|------|--------------|
| `dependsOn` | [specs/045-deploy-resource-filesystem-hardening](../045-deploy-resource-filesystem-hardening/) | Resource envelope + read-only-root contract + model-envelope validator this spec extends. |
| `dependsOn` | [specs/049-monitoring-stack](../049-monitoring-stack/) | externalImages lockstep contract (BUG-049-001) this spec extends for digest pins. |
| `dependsOn` | [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/) | HOST_BIND_ADDRESS fail-loud bind invariants preserved by all compose edits. |
| `dependsOn` | [specs/017-gov-alerts-connector](../017-gov-alerts-connector/) | local-operator build trust model referenced by the go-live checklist. |
| `unblocks` | none | Hardening batch; no downstream spec is currently blocked on it. |
<!-- bubbles:g040-skip-end -->

## Discovered Issues

| Date | ID | Issue | Disposition |
|------|----|-------|-------------|
| 2026-06-10 | DISC-082-01 | Scope 03 implementation recon found the ML image runs as non-root `USER smackerel` and **bakes the embedding model into `/home/smackerel/.cache`** (owned `smackerel:smackerel`) at build time (`ml/Dockerfile:79-89`). The design's planned mount path `/var/cache/smackerel-models` would (a) be root-owned and unwritable by the non-root user, and (b) start empty, still forcing a re-download. | RESOLVED — mount the persistent `ml-model-cache` volume at the image's baked cache path `/home/smackerel/.cache` instead. Docker initializes a fresh named volume FROM the image content (model present, correct ownership) on first mount, so there is no download and no permission failure. DoD D03-1/D03-2 + design.md SCOPE-082-03 updated to the baked path. Strictly superior to the original plan. |
| 2026-06-10 | DISC-082-02 | Scope 07 originally planned the bash test as a sibling under the scripts/deploy directory, but `./smackerel.sh test unit` only auto-discovers shell tests under tests/unit/{cli,web,docs}. A test beside the deploy scripts would never run in the gate suite. | RESOLVED — the parse helpers were factored into `scripts/deploy/promote_manifest_parse.sh` (sourced by both `promote.sh` and the test), and the test lives at `tests/unit/cli/promote_manifest_parse_test.sh` so it is gate-discovered AND runnable directly. Strictly superior to the original plan. |

## Scope Inventory

| # | Name | Surfaces | Tests | DoD shape | Status |
|---|------|----------|-------|-----------|--------|
| 01 | Telegram default identity blanking | `config/smackerel.yaml` | `grep` + `./smackerel.sh test unit` (config/telegram) | 6 items | Done |
| 02 | Ollama concurrent-envelope guard + reconcile | `internal/config/config.go`, `config/smackerel.yaml`, `docs/Operations.md`, `internal/config/validate_ml_envelope_test.go` | `./smackerel.sh test unit` (config) | 9 items | Done |
| 03 | Embedding-model cache persistence | `deploy/compose.deploy.yml`, `scripts/commands/config.sh`, `config/smackerel.yaml`, `internal/deploy/compose_filesystem_contract_test.go` | `./smackerel.sh test unit` (deploy) | 7 items | Done |
| 04 | NATS volume durability + clean protection | `deploy/compose.deploy.yml`, `internal/deploy/nats_volume_lifecycle_contract_test.go` | `./smackerel.sh test unit` (deploy) | 6 items | Done |
| 05 | SearxNG SST resource limits | `config/smackerel.yaml`, `scripts/commands/config.sh`, `deploy/compose.deploy.yml`, `internal/deploy/compose_resource_contract_test.go` | `./smackerel.sh test unit` (deploy) | 7 items | Done |
| 06 | Pin third-party infra images by digest | `deploy/contract.yaml`, `deploy/compose.deploy.yml`, `internal/deploy/external_images_contract_test.go` | `./smackerel.sh test unit` (deploy) | 7 items | Done |
| 07 | Build-manifest schema convergence | `scripts/deploy/promote.sh`, `scripts/deploy/promote_manifest_parse.sh`, `tests/unit/cli/promote_manifest_parse_test.sh` | `bash tests/unit/cli/promote_manifest_parse_test.sh` (gate-discovered) | 6 items | Done |
| 08 | Operator go-live checklist + spec-review addendum | `docs/Deployment.md`, `specs/_spec-review-report.md`, `internal/deploy/golive_checklist_docs_contract_test.go` | `./smackerel.sh test unit` (deploy) | 7 items | Done |
| 09 | ROCm host-specific literals assessment + routing | `deploy/compose.deploy.yml`, `docs/Deployment.md`, `internal/deploy/ollama_gpu_group_add_contract_test.go` | `./smackerel.sh test unit` (deploy) | 7 items | Done |

---

## Scope 1: SCOPE-082-01 — Telegram default identity blanking

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `config/smackerel.yaml` — `telegram.chat_ids: ""`, `telegram.user_mapping: ""`.

**Covers scenarios:** SCN-082-A01.

**Design anchors:** [§2 SCOPE-082-01](design.md#scope-082-01--telegram-default-identity-blanking).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-A01 — Telegram identity is blank by default
  Given a fresh checkout of config/smackerel.yaml
  When an operator reads telegram.chat_ids and telegram.user_mapping
  Then both are empty strings
  And no file in the repo contains the literal chat-id "510638591"
  And the runtime starts with an empty Telegram recipient set treated as
    "no Telegram recipient configured" rather than crashing
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `grep -RIn 510638591 .` | shell | 0 matches (excluding this spec's evidence); empty-mapping parse covered by `internal/telegram/user_mapping_test.go` (SCN-082-A01) |
| `./smackerel.sh test unit` (telegram + config) | unit | empty mapping parses to nil, no crash |
| `./smackerel.sh config generate` | shell | exit 0 with blank telegram values |

### Definition of Done

- [x] D01-1 — `config/smackerel.yaml` `telegram.chat_ids` is `""`.
- [x] D01-2 — `config/smackerel.yaml` `telegram.user_mapping` is `""`.
- [x] D01-3 — `grep -RIn 510638591` over the tree (excluding `specs/082-*`) returns 0 lines.
- [x] D01-4 — `./smackerel.sh config generate` exits 0 with the blank values.
- [x] D01-5 — `./smackerel.sh test unit` telegram/config tests green (empty mapping → nil, no startup crash).
- [x] D01-6 — SCN-082-A01 Telegram identity is blank by default — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 2: SCOPE-082-02 — Ollama concurrent-envelope guard + figure reconcile

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `internal/config/config.go` — `validateModelEnvelopes` concurrent-sum branch + keep-alive resident helper.
- `config/smackerel.yaml` — `environments.self-hosted.ollama_memory_limit: "28G"`.
- `docs/Operations.md` — reconcile model-profile catalog + document the guard.
- `internal/config/validate_ml_envelope_test.go` — 3 new tests.

**Covers scenarios:** SCN-082-B01, SCN-082-B02.

**Design anchors:** [§2 SCOPE-082-02](design.md#scope-082-02--ollama-concurrent-envelope-guard--figure-reconcile).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-B01 — Concurrent interactive ollama sum over-subscription fails loud
  Given OLLAMA_MEMORY_LIMIT resolves to 20G
  And keep-alive is resident (-1 or a multi-hour duration)
  And the distinct interactive hot-path models sum to 24576 MiB
  When config.Load()/Validate() runs
  Then validation fails loud naming the resident set, the sum, and
    OLLAMA_MEMORY_LIMIT

Scenario: SCN-082-B02 — Fitting interactive sum is accepted (no false positive)
  Given OLLAMA_MEMORY_LIMIT resolves to 8G
  And the distinct interactive hot-path models sum to 5120 MiB
  When config.Load()/Validate() runs
  Then validation succeeds
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/config/validate_ml_envelope_test.go::TestValidate_RejectsOversubscribedInteractiveOllamaSet` | unit | sum 24576 > 20480 resident → reject (adversarial: fails if sum branch removed) (SCN-082-B01) |
| `internal/config/validate_ml_envelope_test.go::TestValidate_AcceptsFittingInteractiveOllamaSum` | unit | sum 5120 ≤ 8192 → accept (no false positive) (SCN-082-B02) |
| `TestValidate_SumGuardRelaxedForNonResidentKeepAlive` | unit | oversubscribed + keep_alive 5m → accept |
| `./smackerel.sh config generate` (dev + test) | shell | live configs still pass |

### Definition of Done

- [x] D02-1 — `validateModelEnvelopes` computes the distinct interactive hot-path sum and rejects when resident + `sum > OLLAMA_MEMORY_LIMIT`.
- [x] D02-2 — Resident gate: enforced for `OLLAMA_KEEP_ALIVE == -1` or duration ≥ 10m; relaxed otherwise.
- [x] D02-3 — Error message names the resident set, the sum, and `OLLAMA_MEMORY_LIMIT`.
- [x] D02-4 — `environments.self-hosted.ollama_memory_limit` raised to `"28G"` with documented rationale.
- [x] D02-5 — `docs/Operations.md` catalog reconciled to SST (gemma4:26b 18432, deepseek-r1:32b 22528, gpt-oss:20b 14336, nomic-embed-text 768, deepseek-ocr:3b 2560).
- [x] D02-6 — `docs/Operations.md` documents the concurrent-envelope guard + self-hosted 28G floor.
- [x] D02-7 — 3 new unit tests green, including the adversarial sum-removed proof.
- [x] D02-8 — `./smackerel.sh config generate` dev + test still exit 0 (no false positive).
- [x] D02-9 — SCN-082-B01 Concurrent interactive ollama sum over-subscription fails loud; SCN-082-B02 Fitting interactive sum is accepted (no false positive) — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 3: SCOPE-082-03 — Embedding-model cache persistence

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `deploy/compose.deploy.yml` — `smackerel-ml` persistent model-cache volume + cache-env repoint; `volumes:` declaration.
- `scripts/commands/config.sh` — emit `ML_MODEL_CACHE_VOLUME_NAME`.
- `config/smackerel.yaml` — per-env `ml_model_cache_volume_name`.
- `internal/deploy/compose_filesystem_contract_test.go` — assert mount + repoint.

**Covers scenarios:** SCN-082-C01.

**Design anchors:** [§2 SCOPE-082-03](design.md#scope-082-03--embedding-model-cache-persistence).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-C01 — Embedding-model cache survives restart without HuggingFace
  Given deploy/compose.deploy.yml is rendered for smackerel-ml
  When the compose filesystem contract is asserted
  Then smackerel-ml mounts a persistent named volume at the model-cache path
  And read-only root is still true with an explicit tmpfs allowlist
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/deploy/compose_filesystem_contract_test.go` (extended) | unit | ml mounts `ml-model-cache`, `HF_HOME`/`SENTENCE_TRANSFORMERS_HOME` point into it, read_only true (SCN-082-C01) |
| adversarial sub-test | unit | `HF_HOME=/tmp/...` regression rejected |
| `./smackerel.sh config generate` | shell | `ML_MODEL_CACHE_VOLUME_NAME` emitted |

### Definition of Done

- [x] D03-1 — `smackerel-ml` mounts persistent `ml-model-cache` at `/home/smackerel/.cache` (the image's baked cache path; DISC-082-01).
- [x] D03-2 — `HF_HOME` + `SENTENCE_TRANSFORMERS_HOME` point into the persistent mount (`/home/smackerel/.cache/{huggingface,sentence-transformers}`), not `/tmp`.
- [x] D03-3 — `read_only: true` preserved; tmpfs allowlist intact.
- [x] D03-4 — `ml-model-cache` declared with `name: ${ML_MODEL_CACHE_VOLUME_NAME}` + `persistent` lifecycle label.
- [x] D03-5 — `config.sh` emits `ML_MODEL_CACHE_VOLUME_NAME` per env; `config generate` exit 0.
- [x] D03-6 — `compose_filesystem_contract_test.go` extended + adversarial sub-test green.
- [x] D03-7 — SCN-082-C01 Embedding-model cache survives restart without HuggingFace — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 4: SCOPE-082-04 — NATS volume durability + clean protection

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `deploy/compose.deploy.yml` — `nats-data` lifecycle label → `persistent`.
- `internal/deploy/nats_volume_lifecycle_contract_test.go` — new contract test.

**Covers scenarios:** SCN-082-D01.

**Design anchors:** [§2 SCOPE-082-04](design.md#scope-082-04--nats-volume-durability--clean-protection).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-D01 — clean cannot wipe queued capture events
  Given the nats-data volume carries a durable lifecycle label/protection
  When a clean flow enumerates removable project volumes
  Then nats-data is excluded from removal on a running self-hosted stack
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/deploy/nats_volume_lifecycle_contract_test.go` | unit | `nats-data` label `com.smackerel.lifecycle: persistent` (SCN-082-D01) |
| adversarial sub-test | unit | `ephemeral` regression rejected |

### Definition of Done

- [x] D04-1 — `nats-data` labelled `com.smackerel.lifecycle: persistent` in `deploy/compose.deploy.yml`.
- [x] D04-2 — A code comment documents WHY (JetStream at-least-once in-flight capture state).
- [x] D04-3 — New contract test asserts the durable label against the live compose file.
- [x] D04-4 — Adversarial sub-test proves an `ephemeral` regression is rejected.
- [x] D04-5 — `./smackerel.sh test unit` (deploy) green.
- [x] D04-6 — SCN-082-D01 clean cannot wipe queued capture events — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 5: SCOPE-082-05 — SearxNG SST resource limits

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `config/smackerel.yaml` — `deploy_resources.searxng: { cpus, memory }`.
- `scripts/commands/config.sh` — emit `SEARXNG_CPU_LIMIT` / `SEARXNG_MEMORY_LIMIT`.
- `deploy/compose.deploy.yml` — searxng limits → fail-loud `${VAR:?...}`.
- `internal/deploy/compose_resource_contract_test.go` — include searxng.

**Covers scenarios:** SCN-082-E01.

**Design anchors:** [§2 SCOPE-082-05](design.md#scope-082-05--searxng-sst-resource-limits).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-E01 — SearxNG limits are SST-sourced and fail loud
  Given deploy/compose.deploy.yml renders the searxng service
  When the resource contract is asserted
  Then searxng cpus and memory use ${SEARXNG_CPU_LIMIT:?...} and
    ${SEARXNG_MEMORY_LIMIT:?...} sourced from deploy_resources.searxng.*
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/deploy/compose_resource_contract_test.go` (extended) | unit | searxng cpus+memory use `${SEARXNG_*_LIMIT:?...}` (SCN-082-E01) |
| adversarial sub-test | unit | literal `memory: 256M` regression rejected |
| `./smackerel.sh config generate` | shell | `SEARXNG_CPU_LIMIT`/`SEARXNG_MEMORY_LIMIT` emitted |

### Definition of Done

- [x] D05-1 — `deploy_resources.searxng.{cpus,memory}` added to `config/smackerel.yaml`.
- [x] D05-2 — `config.sh` emits `SEARXNG_CPU_LIMIT` + `SEARXNG_MEMORY_LIMIT` (required_value).
- [x] D05-3 — compose searxng uses `cpus: "${SEARXNG_CPU_LIMIT:?...}"` + `memory: "${SEARXNG_MEMORY_LIMIT:?...}"`.
- [x] D05-4 — explicit `cpus` cap now present (was missing).
- [x] D05-5 — `compose_resource_contract_test.go` includes searxng + adversarial literal-regression sub-test.
- [x] D05-6 — `./smackerel.sh config generate` + `test unit` green.
- [x] D05-7 — SCN-082-E01 SearxNG limits are SST-sourced and fail loud — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 6: SCOPE-082-06 — Pin third-party infra images by digest

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `deploy/contract.yaml` — pin externalImages postgres/nats/ollama by digest.
- `deploy/compose.deploy.yml` — pin the same three literals byte-for-byte.
- `internal/deploy/external_images_contract_test.go` — digest-pin check + adversarial.

**Covers scenarios:** SCN-082-F01.

**Design anchors:** [§2 SCOPE-082-06](design.md#scope-082-06--pin-third-party-infra-images-by-digest).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-F01 — Third-party infra images are digest-pinned in lockstep
  Given deploy/contract.yaml and deploy/compose.deploy.yml
  When the externalImages contract is asserted
  Then postgres, nats, and ollama images contain @sha256: digests
  And the contract.yaml and compose literals match byte-for-byte
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/deploy/external_images_contract_test.go` (extended) | unit | trio literals contain `@sha256:`; compose==contract byte-for-byte (SCN-082-F01) |
| adversarial sub-test | unit | bare-tag (no `@sha256:`) trio image rejected |

### Definition of Done

- [x] D06-1 — `deploy/contract.yaml` pins pgvector/nats/ollama by the resolved sha256 digests.
- [x] D06-2 — `deploy/compose.deploy.yml` pins the same three byte-for-byte.
- [x] D06-3 — Existing byte-for-byte lockstep (Check 3) still passes.
- [x] D06-4 — New digest-pin check requires `@sha256:` on the production trio.
- [x] D06-5 — Adversarial sub-test proves a bare-tag regression is rejected.
- [x] D06-6 — `./smackerel.sh test unit` (deploy) green — Evidence: see `report.md` SCOPE-082-06 deploy-package run.
- [x] D06-7 — SCN-082-F01 Third-party infra images are digest-pinned in lockstep — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 7: SCOPE-082-07 — Build-manifest schema convergence

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `scripts/deploy/promote.sh` — source + use the dual-shape parse helpers.
- `scripts/deploy/promote_manifest_parse.sh` — NEW sourceable helper library (DISC-082-02).
- `tests/unit/cli/promote_manifest_parse_test.sh` — gate-discovered bash test over both shapes (DISC-082-02).

**Covers scenarios:** SCN-082-G01.

**Design anchors:** [§2 SCOPE-082-07](design.md#scope-082-07--build-manifest-schema-convergence).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-G01 — promote.sh parses both manifest shapes
  Given a CI list-shape build manifest and a local-operator map/object manifest
  When promote.sh extracts core/ml refs and the bundle ref+sha for an env
  Then both shapes yield the same extracted values
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `tests/unit/cli/promote_manifest_parse_test.sh` | shell | both fixture shapes → identical extracted values (SCN-082-G01) |
| adversarial case | shell | malformed (neither shape) fails loud |

### Definition of Done

- [x] D07-1 — `promote.sh` detects manifest shape (list vs map/object) and parses both.
- [x] D07-2 — Extraction helpers are factored (`scripts/deploy/promote_manifest_parse.sh`) so the test can drive them.
- [x] D07-3 — Fail-loud when neither shape yields a value.
- [x] D07-4 — `tests/unit/cli/promote_manifest_parse_test.sh` (gate-discovered) covers both shapes → equal values.
- [x] D07-5 — Adversarial malformed-manifest case fails loud.
- [x] D07-6 — SCN-082-G01 promote.sh parses both manifest shapes — Evidence: ≥10 lines incl. real run captured in `report.md`.

---

## Scope 8: SCOPE-082-08 — Operator go-live checklist + spec-review addendum

**Status:** Done
**Scope-Kind:** docs-only
**Depends on:** none
**Foundation:** false
### Surface

- `docs/Deployment.md` — Go-Live Readiness Checklist section.
- `specs/_spec-review-report.md` — dated addendum.
- `internal/deploy/golive_checklist_docs_contract_test.go` — anchors contract.

**Covers scenarios:** SCN-082-H01.

**Design anchors:** [§2 SCOPE-082-08](design.md#scope-082-08--operator-go-live-checklist--spec-review-addendum).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-H01 — Operator go-live checklist exists and is wired
  Given docs/ after this spec
  When an operator looks for go-live readiness
  Then one checklist enumerates the 5 secrets, L2 injection, local/CI trust,
    profile enablement, backup/restore sequencing, and supervised canary
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `golive_checklist_docs_contract_test.go` | unit | all checklist anchors present in `docs/Deployment.md` (SCN-082-H01) |
| adversarial sub-test | unit | a removed anchor fails |

### Definition of Done

- [x] D08-1 — `docs/Deployment.md` has a "Go-Live Readiness Checklist" section.
- [x] D08-2 — It enumerates the 5 production secrets (spec 051), L2 knb injection (spec 052), local/CI trust (spec 017).
- [x] D08-3 — It covers `--profile ollama`/`--profile monitoring`/`--profile searxng` enablement.
- [x] D08-4 — It covers backup + restore-drill sequencing before any release-train promote, and the supervised-canary first apply.
- [x] D08-5 — `specs/_spec-review-report.md` gets a dated 2026-06-09 addendum (no history rewrite).
- [x] D08-6 — `golive_checklist_docs_contract_test.go` (anchors + adversarial) green.
- [x] D08-7 — SCN-082-H01 Operator go-live checklist exists and is wired — Evidence: ≥10 lines captured in `report.md`.

---

## Scope 9: SCOPE-082-09 — ROCm host-specific literals assessment + routing

**Status:** Done
**Scope-Kind:** contract-only
**Depends on:** none
**Foundation:** false
### Surface

- `deploy/compose.deploy.yml` — ollama `group_add` → fail-loud adapter env.
- `docs/Deployment.md` — adapter contract for `OLLAMA_RENDER_GID`/`OLLAMA_VIDEO_GID`.
- `internal/deploy/ollama_gpu_group_add_contract_test.go` — contract.

**Covers scenarios:** SCN-082-I01.

**Design anchors:** [§2 SCOPE-082-09](design.md#scope-082-09--rocm-host-specific-literals-assessment--routing).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-082-I01 — ROCm host GIDs are policy-correct
  Given deploy/compose.deploy.yml ollama service
  When the host-specific render/video GIDs are evaluated
  Then either they are routed to an adapter-supplied fail-loud env (no silent
    default) or a documented rationale justifies a generic default, with a
    contract test pinning the decision
```

### Test Plan

| Test | Type | Asserts |
|------|------|---------|
| `internal/deploy/ollama_gpu_group_add_contract_test.go` | unit | `group_add` uses `${OLLAMA_RENDER_GID:?...}`/`${OLLAMA_VIDEO_GID:?...}`; gfx env literals remain (SCN-082-I01) |
| adversarial sub-test | unit | a bare numeric-literal GID regression rejected |

### Definition of Done

- [x] D09-1 — Assessment recorded in `design.md`: GIDs are host-specific → route to adapter; gfx target stays generic.
- [x] D09-2 — compose `group_add` uses `${OLLAMA_RENDER_GID:?...}` + `${OLLAMA_VIDEO_GID:?...}` (no silent default; Gate G028).
- [x] D09-3 — compose comment + `docs/Deployment.md` document the adapter MUST emit both GIDs.
- [x] D09-4 — gfx env literals (`HSA_OVERRIDE_GFX_VERSION`, `HCC_AMDGPU_TARGET`) retained with documented generic-default rationale.
- [x] D09-5 — `ollama_gpu_group_add_contract_test.go` (fail-loud form + adversarial numeric-literal) green.
- [x] D09-6 — `HOST_BIND_ADDRESS` + all other spec-042/045 invariants still pass their contract tests.
- [x] D09-7 — SCN-082-I01 ROCm host GIDs are policy-correct — Evidence: ≥10 lines captured in `report.md`.
