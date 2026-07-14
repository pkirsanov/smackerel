# Scopes 102 — <deploy-host> Deploy Hardening

> Planned by `bubbles.plan` (invoked by `bubbles.goal` as the planning phase of a
> **no-shortcuts, best-quality** hardening). Four sequential, scope-gated scopes,
> each a single vertical outcome that replaces ONE expedient landed during the
> 2026-07-09 live <deploy-host> MVP-readiness validation of core `639472f7`. Each scope
> carries a Test Plan (Gherkin → concrete test) with **Test Plan ↔ DoD parity**
> and a strict, evidence-backed Definition of Done.
>
> **Cross-repo.** SCOPE-102-01/02/03 land in the **smackerel product repo**
> (`/Users/pkirsanov/Projects/smackerel`); SCOPE-102-02 (ntfy endpoint injection +
> `alertmanager-standup.sh` retirement) and SCOPE-102-04 (backup regression tests)
> also author **apply-ready** code in the **knb self-hosted adapter**
> (`<deployment-owner>/<product>/<target>`). The knb adapter owns the
> concrete host topology (ntfy endpoint value, cross-network attach, manifest
> ownership); the product repo stays target-agnostic ("no env-specific content").
>
> **Anti-fabrication (NON-NEGOTIABLE).** Every DoD item is backed by a REAL
> executed test run with captured evidence (≥10 lines raw output). Every
> regression-preventing test is **adversarial** — it FAILS if the protection is
> removed (secret re-added to the sidecar env; alerting block dropped from the
> bundle; an understated model profile; a silently-succeeding degraded backup).
> No `Exit Code: 0`, no `--- PASS`, no "all tests pass" is written into
> [report.md](report.md) until the command was actually run and its output pasted.
>
> **Out-of-scope operator handoff (honored, NOT planned as in-scope).** The live
> <deploy-host> re-apply (R-102-A), the rollback drill (R-102-B), the knb git
> reconcile/push (R-102-C), and the live `ollama ps` co-residency proof for
> qwen3+gemma4 under ROCm (R-102-D) are operator-gated deploy governance /
> live-host proofs. This plan produces durable, tested, **apply-ready** code; the
> apply itself is a separate operator-authorized step (see § *Out-of-Scope
> Operator Handoff*). No live-host mutation and no faked live-host evidence.
>
> Scenario IDs: `SCN-102-C1-01..05`, `SCN-102-C2-01..04`, `SCN-102-C3-01..05`,
> `SCN-102-C4-01..03` (see [spec.md](spec.md) §4 and
> [scenario-manifest.json](scenario-manifest.json)). Terminal commands use
> `./smackerel.sh` (Docker-only, macOS host) for product tests; knb adapter tests
> run from the Smackerel repo via
> `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh` (knb has no CLI).

---

## Planning Contract Reconciliation (2026-07-10)

This section records planning declarations only. It does not add implementation
evidence, convert an unexecuted check into a pass, or move any scope/spec status.

**Scope-Kind:** contract-only

The in-scope ship-time acceptance boundary is the generated bundle, deploy/config
contracts, request payloads, and hermetic adapter hooks. A fully applied <deploy-host>
system requires operator-authorized host mutation (R-102-A), while ROCm/Ollama
hardware proof requires the real host (R-102-D); both remain operator validation
contracts after repository implementation. This classification exempts live E2E
rows without misclassifying unit, contract, or integration tests as E2E.

### Repository Ownership and Cross-Repo Change Boundary

| Repository owner | Exact in-scope paths | Owned behavior | Explicitly excluded |
| --- | --- | --- | --- |
| `smackerel` product | `.github/workflows/ci.yml`; `Dockerfile`; `cmd/alertmanager-ntfy-bridge/main.go`; `cmd/alertmanager-ntfy-bridge/bridge_test.go`; `cmd/core/wiring_assistant_openknowledge.go`; `config/prometheus/alertmanager.yml`; `config/prometheus/prometheus.yml.tmpl`; `config/smackerel.yaml`; `deploy/compose.deploy.yml`; `deploy/contract.yaml`; `docs/Operations.md`; `internal/backup/status_test.go`; `internal/config/config.go`; `internal/config/validate_ml_envelope_kv_spec102_test.go`; `internal/config/validate_ml_envelope_test.go`; `internal/config/validate_test.go`; `internal/deploy/alertmanager_bundle_contract_test.go`; `internal/deploy/bundle_secret_contract_test.go`; `internal/deploy/compose_contract_test.go`; `internal/deploy/external_images_contract_test.go`; `internal/assistant/openknowledge/llm/client_test.go`; `tests/integration/agent/loop_test.go`; `ml/app/agent.py`; `ml/app/card_categories.py`; `ml/app/domain.py`; `ml/app/drive_classify.py`; `ml/app/intelligence.py`; `ml/app/main.py`; `ml/app/nats_client.py`; `ml/app/ocr.py`; `ml/app/ollama_keepalive.py`; `ml/app/processor.py`; `ml/app/routes/chat.py`; `ml/app/synthesis.py`; `ml/tests/conftest.py`; `ml/tests/test_agent.py`; `ml/tests/test_card_categories.py`; `ml/tests/test_chat_dispatch_parity_spec096.py`; `ml/tests/test_domain.py`; `ml/tests/test_drive_classify.py`; `ml/tests/test_intelligence_handlers.py`; `ml/tests/test_main.py`; `ml/tests/test_nats_client.py`; `ml/tests/test_ocr.py`; `ml/tests/test_ollama_keepalive.py`; `ml/tests/test_processor.py`; `ml/tests/test_synthesis.py`; `scripts/commands/config.sh`; `scripts/lint/python-compute-only-guard.allowlist`; `scripts/lint/python-compute-only-guard.selftest.sh`; `scripts/lint/python-compute-only-guard.sh`; `smackerel.sh` | Generic config generation, bundle/security contracts, Alertmanager routing structure and ntfy message templating, model-envelope validation, Python-only Ollama request construction, typed Go-boundary proof, and product-side schema compatibility | Concrete <deploy-host> topology, operator-private endpoint values, live host mutation, `/srv/backups`, knb manifests, operator git/push actions, and any Go Ollama generation client |
| `knb` deployment overlay | `../knb/smackerel/contract.yaml`; `../<deployment-owner>/<product>/<target>/README.md`; `../<deployment-owner>/<product>/<target>/apply.sh`; `../<deployment-owner>/<product>/<target>/backup.sh`; `../<deployment-owner>/<product>/<target>/params.yaml`; `../<deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh`; `../<deployment-owner>/<product>/<target>/tests/unit/apply_manifest_contract_test.sh`; `../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh`; `../<deployment-owner>/<product>/<target>/tests/unit/backup_status_advance_test.sh`; retired `../<deployment-owner>/<product>/<target>/alertmanager-standup.sh`; retired `../<deployment-owner>/<product>/<target>/alertmanager/alertmanager.yml` | Concrete self-hosted apply/backup behavior, deploy-contract mirror, manifest ownership restoration, and fail-loud/value-safe injection of the operator-private ntfy endpoint | Generic product config, bridge/model logic, product tests, live <deploy-host> re-apply/rollback, and git reconcile/push |

No product path may acquire a concrete ntfy host/topic, <deploy-host> address, or live
backup destination. No knb path may duplicate the product's generic bundle,
security guard, bridge templating, or model-envelope implementation. R-102-A..D
remain post-implementation operator handoffs and are not implementation DoD.

### Coverage Declarations

- **E2E UI coverage:** not applicable. This feature changes no browser, mobile,
  admin, or other interactive UI route, component, navigation target, or user
  workflow; therefore there is no UI scenario or `e2e-ui` test target to add.
- **E2E/API coverage:** no public product API contract is added or changed. The
  executable in-repo boundaries are the generated deploy bundle, compose
  contract, Alertmanager webhook bridge, model request payload, and backup hook;
  their scenario-specific unit/functional/contract checks remain listed per scope.
  The only full deployed-system proofs require <deploy-host> and stay explicitly in
  R-102-A (live re-apply) and R-102-D (live Ollama proof), owned by the operator
  after implementation rather than silently promoted into implementation DoD.
- **Stress coverage:** not applicable. The change introduces no concurrency,
  throughput, queue-capacity, or sustained-load requirement. Model resident-memory
  limits are deterministic configuration validation, and backup/alert behavior is
  failure-path correctness; adversarial unit/functional tests are the
  discriminating checks. Live hardware pressure remains R-102-D.
- **Load coverage:** not applicable for the same reason; no latency or throughput
  acceptance threshold is introduced by this hardening plan.

### Consumer and Change-Boundary Declaration

The affected first-party consumers are: deploy-bundle extraction and Compose;
`smackerel-ml` env loading; `smackerel-core` (which deliberately retains
`app.env`); pre-push and CI lint entry points; Prometheus; Alertmanager; the ntfy
templating bridge; every Python Ollama request builder; config-generation callers;
the backup watcher/status parser; the knb self-hosted apply/backup hooks; and their
contract/unit/functional tests. There are no navigation, breadcrumb, redirect,
generated API client, deep-link, or UI consumers. Each scope's local Change
Boundary remains authoritative; collateral refactors and unrelated cleanup are
excluded.

**Allowed file families:** only the exact Smackerel and knb path families in the
ownership table and each scope-local Change Boundary.

**Excluded surfaces:** live host state, operator secrets, real backup destinations,
unrelated UI/API/QF-companion behavior, and collateral refactors.

---

## Execution Outline

A reviewer can read this ~50-line outline and catch a wrong scope order or a
missing validation checkpoint BEFORE the full plan is implemented.

### Phase Order

1. **SCOPE-102-01 — ML-sidecar compute-only secret isolation (SECURITY, HIGH; `foundation:true`).**
   A reusable per-service env projection (`project_service_env`) ships a
   secret-free `ml.env`; `smackerel-ml` loads `ml.env` not `app.env`; a no-bypass
   `python-compute-only-guard` (forbidden-driver + infra-URL + env-allowlist
   scans) is wired into `test pre-push` + CI; postgres is network-segmented off
   the compute tier. Highest blast-radius reduction → first.
2. **SCOPE-102-02 — Durable Prometheus → Alertmanager → ntfy routing.**
   The `alerting:` block + an `alertmanager` monitoring-profile service + a generic
   routing config + a core-image ntfy templating bridge are folded INTO the product
   bundle so alerting survives every apply; the knb adapter injects the ntfy
   endpoint and the fragile `alertmanager-standup.sh` is retired.
3. **SCOPE-102-03 — Model-envelope correctness + BUG-026-006.**
   SST per-model `num_ctx`/`keep_alive`/`max_loaded_models` + `num_parallel`;
   KV-aware `model_memory_profiles`; a truthful `validateModelEnvelopes`; SST
  request profiles applied fail-loud by the Python compute tier to all 13
  production Ollama inference builders; Go remains the typed validator/router;
  the host-tag `ollama create` hack deleted; BUG-026-006 output-budget/routing
  fix retained and revalidated.
4. **SCOPE-102-04 — Backup-adapter durability formalization (knb adapter).**
   Adversarial regression tests LOCK the already-landed F-1/F-2/F-3 fixes for the
   branches not yet covered by the existing `backup_degraded_contract_test.sh`
   (NATS-present-but-capture-fails; root-manifest non-fatal; status advance/hold +
   `internal/backup.Status` schema parity). Test-only + one apply.sh assertion →
   lowest risk → last.

### New Types & Signatures (the C-header view)

- **SCOPE-102-01** — SST `services.ml.env_allowlist: [<exact keys>+<prefix globs>]`
  (`config/smackerel.yaml`); generator `project_service_env(<svc>,<allowlist>) ->
  <svc>.env` = `{k=v ∈ app.env : matches(k,allowlist)} \ (SHELL_SECRET_KEYS ∪
  POSTGRES_*)`, fail-loud if `allowlist ∩ SHELL_SECRET_KEYS ≠ ∅`
  (`scripts/commands/config.sh` ≈ L2878); bundle artifact **`ml.env`**; compose
  `smackerel-ml.env_file: ./ml.env` + top-level `networks: {data-tier,
  compute-tier}` (postgres→data-tier only; core→both; ml/nats/ollama→compute-tier);
  guard `scripts/lint/python-compute-only-guard.sh` + `.allowlist` (exit `0`/`1`/`2`,
  no bypass, `PYCO_GUARD_SCAN_ROOT` default `ml/`).
- **SCOPE-102-02** — `config/prometheus/prometheus.yml.tmpl` static
  `alerting:{alertmanagers:[{static_configs:[{targets:['alertmanager:9093']}]}]}`;
  compose `alertmanager` service (`profiles:[monitoring]`, RO-root, `cap_drop:[ALL]`,
  `no-new-privileges`, fail-loud `ALERTMANAGER_CPU_LIMIT`/`ALERTMANAGER_MEMORY_LIMIT`),
  digest-pinned in `deploy/contract.yaml` `externalImages`; generic
  `config/prometheus/alertmanager.yml` with `webhook_configs.url_file:
  /etc/alertmanager/ntfy_url` (no ntfy literal); bridge `cmd/alertmanager-ntfy-bridge`
  on `${SMACKEREL_CORE_IMAGE}` → `X-Title`/`X-Priority`/`X-Tags` from
  severity+summary, ntfy base from fail-loud `ALERTMANAGER_NTFY_URL`; knb `apply.sh`
  writes the real ntfy URL to the bind-mounted `/etc/alertmanager/ntfy_url` + attaches
  to `self-hosted-ntfy`; **retire** knb `alertmanager-standup.sh` + `alertmanager/alertmanager.yml`.
- **SCOPE-102-03** — existing SST profile
  `services.ml.model_memory_profiles[] = {model,weights_mib,kv_mib_per_1k_ctx,num_ctx}`
  plus request `services.ml.ollama_keep_alive` and daemon
  `infrastructure.ollama.{keep_alive,num_parallel,max_loaded_models}`; existing Go
  `validateModelEnvelopes` resident math `weights + KV(num_ctx,num_parallel)`;
  one shared fail-loud Python request-profile applicator in
  `ml/app/ollama_keepalive.py`, with LiteLLM-kwargs and native-JSON adapters that
  merge `options.num_ctx`, preserve caller options/`think`, and place request
  `keep_alive` at top level. All 13 design-inventoried builders consume it;
  hosted-provider branches receive no Ollama-only fields; Go generation endpoints
  remain forbidden while bounded read-only `/api/tags` probes remain valid.
- **SCOPE-102-04** — knb `tests/unit/backup_*_test.sh` (extend the existing
  `backup_degraded_contract_test.sh` + a new status-advance test) using the present
  seams (`SMACKEREL_DUMP_DIR`, `SMACKEREL_BACKUP_STATUS_FILE`, `NATS_VOLUME_NAME`,
  `SMACKEREL_MANIFEST_FILE`); product `internal/backup/status_test.go` schema-parity
  assertion (`schema_version 1` shape the knb hook writes; `CurrentSchemaVersion`).

### Validation Checkpoints (where breakage is caught before the next scope)

- **After SCOPE-102-01** — `./smackerel.sh config generate` EXIT 0 with a
  secret-free `ml.env`; `internal/deploy` bundle/compose contract tests GREEN
  (ml.env ∩ secrets = ∅; `smackerel-ml.env_file=./ml.env`; postgres unreachable from
  compute tier); the guard PASSES clean AND its adversarial selftest FAILS on a
  re-added secret / forbidden driver / infra-URL read; `./smackerel.sh test pre-push`
  EXIT 0 runs the wired guard. Catches a leaking projection or an unwired guard
  BEFORE alert routing.
- **After SCOPE-102-02** — `alertmanager_bundle_contract_test.go` GREEN (alerting
  block + alertmanager service + `alertmanager.yml` with `url_file`, no ntfy literal);
  bridge unit test GREEN (Alertmanager JSON ⇒ titled/priority ntfy request);
  `./smackerel.sh config generate` + `./smackerel.sh check` EXIT 0. Catches a
  dropped alerting block or a leaked endpoint BEFORE model work.
- **After SCOPE-102-03** — all 13 builder-specific payload tests and the
  production-builder inventory guard are GREEN; invalid profile tables and an
  unprofiled selected model fail before network I/O; LiteLLM/native adapters
  preserve caller options and top-level `keep_alive`; hosted-provider negatives
  carry no Ollama fields; Go typed-boundary and `/api/tags`-only contracts are
  GREEN; the existing KV-validator and BUG-026-006 output-budget tests remain
  GREEN; `./smackerel.sh config generate` + `./smackerel.sh check` EXIT 0.
- **After SCOPE-102-04** —
  `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh` GREEN
  including the new F-1-variant / F-2 / F-3 adversarial branches; product
  `./smackerel.sh test unit` GREEN including the `internal/backup` schema-parity
  assertion. Catches a backup that silently succeeds while degraded.

---

## Scope Ordering Rationale

Ordered by **risk × blast-radius**, dependency-light and each an independent
vertical slice (no horizontal layering):

1. **SCOPE-102-01 first** — the SECURITY-HIGH item; removes the managed-secret set
   from the least-trusted, fastest-churning Python tier. Highest value, self-contained.
2. **SCOPE-102-02 second** — closes the MVP-critical observability loop (alerts have
   no durable delivery path today). Shares the `config.sh` bundle-staging surface and
   `compose.deploy.yml` with SCOPE-102-01 (different regions: 01 adds `ml.env` +
   networks, 02 adds `alertmanager.yml` + the alertmanager service) → 01 lands first,
   02 rebases on it (coordination note below).
3. **SCOPE-102-03 third** — model-envelope truthfulness; independent surface
   (`config/smackerel.yaml` model section + `config.go` + `ml/app` + docs).
4. **SCOPE-102-04 last** — FORMALIZES already-landed knb code with adversarial tests;
   test-only + one apply.sh assertion; zero runtime-logic change → lowest risk.

**Cross-scope coordination.** SCOPE-102-01 and SCOPE-102-02 both extend the
`scripts/commands/config.sh` bundle-staging block (≈ L2878–2965: `STAGE_DIR` writes,
`bundle-manifest.yaml` `files:`, `TAR_FILES`) and `deploy/compose.deploy.yml`. Scope
isolation is preserved by strict gating: SCOPE-102-01 is fully done (checkpoint green)
before SCOPE-102-02 starts, so SCOPE-102-02 extends the post-01 tree. No parallel edits
to the same region.

**Capability-foundation ordering (P4).** [design.md](design.md) now distinguishes
the single-consumer env projection in SCOPE-102-01 from the N≥2 request-profile
foundation in SCOPE-102-03. SCOPE-102-01 remains `foundation:true` for its env
projection. SCOPE-102-03 is also `foundation:true`: within that vertical scope,
the shared request-profile applicator is implemented and canary-tested before the
LiteLLM/native adapters and all 13 builder migrations. No later scope consumes either
foundation, so no new cross-scope overlay dependency is introduced.

---

## Scope Table

| # | Scope | Repo(s) | Surfaces | Required tests (adversarial in **bold**) | DoD headline | Status |
| --- | --- | --- | --- | --- | --- | --- |
| 01 | ML-sidecar compute-only secret isolation `foundation:true` | smackerel | `config/smackerel.yaml`, `scripts/commands/config.sh`, `deploy/compose.deploy.yml`, `scripts/lint/python-compute-only-guard.sh`(+`.allowlist`+selftest), `smackerel.sh` pre-push, `.github/workflows/ci.yml`, `internal/deploy/*_contract_test.go` | functional bundle projection, **guard selftest** (secret/driver/URL/bypass failures), network contract, security F1 credential-suffix regression | `ml.env` excludes managed/data credentials; guard is wired; data/compute tiers are separated | [x] Done |
| 02 | Durable Prometheus→Alertmanager→ntfy routing | smackerel + knb | product bundle/compose/bridge plus exact knb paths in the ownership table | **functional bundle/re-extract/no-literal contracts**, unit bridge templating, security F2 topology contract, security F3 adapter-output hygiene | durable routing and templating contracts; endpoint remains adapter-owned; standup retired | [x] Done |
| 03 | Model-envelope correctness + BUG-026-006 | smackerel | `config/smackerel.yaml`, `internal/config/**`, all 13 Python Ollama request builders + shared request-profile applicator, `docs/Operations.md`, BUG-026-006/007 | **13 builder payload units + inventory guard + fail-loud profile/adapters**, existing KV-math/uncapped/posture and SST output-budget units | every Ollama builder is capped fail-loud; hosted branches stay clean; Go remains typed-only | [~] Implemented and tested; validation pending |
| 04 | Backup-adapter durability formalization | knb (+ product schema parity) | exact adapter tests plus product `internal/backup/status_test.go` | **functional degraded/root-manifest/status regressions**, apply contract, product schema unit, canonical wrapper | F-1/F-2/F-3 are locked; canonical wrapper is green | [~] Implemented and tested; validation pending |

---

## SCOPE-102-01 — ML-sidecar compute-only secret isolation

**Status:** Done (2026-07-09)
**Tags:** `security:high` · `concern:1` · `foundation:true` · `repo:smackerel`
**Depends On:** none (first scope). It ESTABLISHES the reusable per-service env
projection foundation; no later scope depends on it.

### Use Cases (Gherkin)

Primary outcome — the compute-only sidecar holds no managed secret, and the guard
makes that non-regressible. Contract scenarios `SCN-102-C1-01..05` (full text in
[spec.md](spec.md) §4).

```gherkin
Scenario: SCN-102-C1-01 — the sidecar env carries no managed secret
  Given a config bundle generated for the self-hosted target
  When the smackerel-ml service loads its env from the bundle
  Then its environment contains none of the SHELL_SECRET_KEYS
    (POSTGRES_PASSWORD, AUTH_SIGNING_ACTIVE_PRIVATE_KEY, AUTH_AT_REST_HASHING_KEY,
     AUTH_BOOTSTRAP_TOKEN, TELEGRAM_BOT_TOKEN, KEEP_GOOGLE_APP_PASSWORD,
     CARD_REWARDS_GCAL_CREDENTIALS, WEB_REGISTRATION_INVITE_TOKEN,
     LLM_PROVIDER_SECRET_MASTER_KEY)
    And it contains none of the POSTGRES_* connection parts
```

```gherkin
Scenario: SCN-102-C1-03 — a re-added secret fails the guard (adversarial)
  Given the python-compute-only-guard is wired into pre-push and CI
  When a change re-adds env_file: ./app.env to smackerel-ml
    Or a SHELL_SECRET_KEY appears in the smackerel-ml env delivery
  Then the guard exits non-zero and names the offending key
    And there is no --skip / --force / --ignore flag that suppresses it
```

Also in this scope (mapped in the Test Plan): `SCN-102-C1-02` (sidecar still boots
with every compute key), `SCN-102-C1-04` (forbidden driver / infra-URL read fails the
guard), `SCN-102-C1-05` (defense-in-depth: postgres unreachable from smackerel-ml).

### Implementation Plan

Cited against the recon in [design.md](design.md) → *SCOPE-102-01*.

1. **Env-projection foundation** — implement `project_service_env(<svc>,<allowlist>)`
   in [`scripts/commands/config.sh`](../../scripts/commands/config.sh) in the
   bundle-staging block (≈ L2878, next to the `app.env` write). Output =
   `{k=v ∈ app.env : matches(k,allowlist)} \ (SHELL_SECRET_KEYS ∪ POSTGRES_*)`.
   ALWAYS hard-subtract `SHELL_SECRET_KEYS` (≈ L510-521) + `POSTGRES_*` regardless of
   the declared allowlist; **fail loud** if the declared allowlist intersects
   `SHELL_SECRET_KEYS` (the tripwire). Stage `ml.env` into `STAGE_DIR`, add it to
   `bundle-manifest.yaml` `files:` and the `TAR_FILES` argv, `chmod 0644`.
2. **SST allowlist** — add `services.ml.env_allowlist` to
   [`config/smackerel.yaml`](../../config/smackerel.yaml) =
   `{NATS_URL, OLLAMA_URL, SMACKEREL_ENV, SMACKEREL_AUTH_TOKEN, PROMPT_CONTRACTS_DIR,
   HF_HOME, SENTENCE_TRANSFORMERS_HOME, EMBEDDING_MODEL,
   PHOTOS_INTELLIGENCE_EMBED_MODEL}` + prefixes `{ML_*, OLLAMA_*}`. REQUIRED,
   fail-loud (Gate G028). No `${VAR:-default}`.
3. **Compose** — change `smackerel-ml.env_file` from `- ./app.env` to `- ./ml.env`
   in [`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml); keep its explicit
   `environment:` block. `smackerel-core` is unchanged (keeps `./app.env`; it is the
   trusted typed tier).
4. **Guard** — add [`scripts/lint/python-compute-only-guard.sh`](../../scripts/lint/python-compute-only-guard.sh)
   - `.allowlist`, mirroring QF spec-089 Scope C
   (`/Users/pkirsanov/Projects/QuantitativeFinance/scripts/lint/python-compute-only-guard.sh`):
   bash-3.2 + POSIX; exit `0` clean / `1` violation / `2` bypass-or-malformed;
   `--skip|--force|--ignore|--no-verify ⇒ exit 2`; `PYCO_GUARD_SCAN_ROOT` default
   `ml/`. Three checks: (a) forbidden-driver scan
   (`psycopg|asyncpg|sqlalchemy|redis|aioredis|kafka|confluent-kafka|pymongo|motor`
   in dependency-spec position of `ml/pyproject.toml`/`requirements*.txt`);
   (b) infra-URL-read scan (`os.environ[...]`/`os.getenv(...)` of
   `DATABASE_URL|POSTGRES_URL|REDIS_URL|RABBITMQ_URL` in `ml/**/*.py`);
   (c) env-allowlist/secret-absence assertion (generate a self-hosted bundle, extract
   `ml.env`, assert `ml.env ∩ SHELL_SECRET_KEYS = ∅` and compose
   `smackerel-ml.env_file = ./ml.env`). Add an adversarial selftest fixture
   (`scripts/lint/python-compute-only-guard.selftest.sh`) proving each check trips.
5. **Wiring (no bypass)** — add the guard to the `test pre-push` arm in
   [`smackerel.sh`](../../smackerel.sh) (≈ L2200, next to the macOS-portability guard:
   `bash "$SCRIPT_DIR/scripts/lint/python-compute-only-guard.sh" || exit 1`) AND to CI
   in [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) (the `lint-and-test`
   job; either fold into `./smackerel.sh lint` or add a dedicated host-side step — the
   guard is a pure bash/grep/find static gate).
6. **Defense-in-depth network** — add top-level `networks: {data-tier, compute-tier}`
   to `deploy/compose.deploy.yml`; `postgres` joins ONLY `data-tier`; `smackerel-core`
   joins BOTH; `smackerel-ml`, `nats`, `ollama` join `compute-tier`. Lock with a
   compose contract-test assertion.

**Consumer Impact Sweep** (env_file contract change `app.env → ml.env`): update
[`internal/deploy/compose_contract_test.go`](../../internal/deploy/compose_contract_test.go)
and [`internal/deploy/bundle_secret_contract_test.go`](../../internal/deploy/bundle_secret_contract_test.go)
to assert `smackerel-ml` loads `ml.env`; scan for any other reference to
`smackerel-ml` + `app.env` (compose, contract tests, docs, `deploy/contract.yaml`).
`smackerel-core`'s `app.env` is intentionally untouched.

**Shared Infrastructure Impact Sweep** (`project_service_env` is a protected reusable
foundation): blast radius = the `config.sh` bundle-staging block (every bundle consumer)

- the compose network topology (every service). **Canary tests** = the bundle
secret-absence assertion + the compose network contract test (run BEFORE any broad
suite). **Rollback** = revert `smackerel-ml.env_file` to `./app.env` and drop the
`networks:` block (documented one-line reversions; the projection code is additive).

**Change Boundary** — allowed: `config/smackerel.yaml` (ml.env_allowlist),
`scripts/commands/config.sh` (projection), `deploy/compose.deploy.yml` (ml env_file +
networks), `scripts/lint/python-compute-only-guard.sh`(+`.allowlist`+selftest),
`smackerel.sh` (pre-push arm), `.github/workflows/ci.yml`, `internal/deploy/*_contract_test.go`.
Excluded (must remain untouched): `smackerel-core`'s `app.env` delivery, the ML
transport (NATS-driven compute-only shape), any auth/postgres runtime code.

### Test Plan

**Scope-Kind contract:** no in-scope live E2E row is claimed. The generated-env
tests prove the sidecar's fail-loud startup preconditions; the actual deployed
container boot remains operator validation R-102-A.

| Scenario | Type / Category | Exact test | Command | Live System | Assertion / regression |
| --- | --- | --- | --- | --- | --- |
| SCN-102-C1-01 | functional | `internal/deploy/bundle_secret_contract_test.go::TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102` | `./smackerel.sh test unit --go --go-run 'TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102' --verbose` | No | Generated `ml.env` excludes every managed secret and `POSTGRES_*`; re-adding one fails. |
| SCN-102-C1-02 | functional | `internal/deploy/bundle_secret_contract_test.go::TestMLEnv_ContainsEveryComputeKey_Spec102` | `./smackerel.sh test unit --go --go-run 'TestMLEnv_ContainsEveryComputeKey_Spec102' --verbose` | No | Generated `ml.env` contains every fail-loud startup key; deployed boot is R-102-A. |
| SCN-102-C1-03 | unit | `scripts/lint/python-compute-only-guard.selftest.sh` | `bash scripts/lint/python-compute-only-guard.selftest.sh` | No | Re-added `app.env`/secret and every bypass-shaped flag fail adversarially. |
| SCN-102-C1-04 | unit | `scripts/lint/python-compute-only-guard.selftest.sh` | `bash scripts/lint/python-compute-only-guard.selftest.sh` | No | Forbidden driver or direct datastore URL read fails and names the source. |
| SCN-102-C1-05 | contract | `internal/deploy/compose_contract_test.go::TestNetworkSegmentation_MLCannotReachPostgres_Spec102` | `./smackerel.sh test unit --go --go-run 'TestNetworkSegmentation_' --verbose` | No | ML and postgres occupy disjoint tiers while core reaches both; adversarial topology cases fail. |
| SCN-102-C1-01/C1-03 (security F1) | functional | `internal/deploy/bundle_secret_contract_test.go::TestMLEnv_CredentialSuffixRequiresSanction_Spec102` | `./smackerel.sh test unit --go --go-run 'TestMLEnv_CredentialSuffixRequiresSanction_Spec102' --verbose` | No | A future same-prefix credential is rejected case-insensitively unless explicitly sanctioned. |

**Impact-aware validation (G079).** `.github/bubbles-project.yaml` defines no
`testImpact` map, so no impact-plan is generated automatically; changed-path seed for a
later regeneration: `config/smackerel.yaml`, `scripts/commands/config.sh`,
`deploy/compose.deploy.yml`, `scripts/lint/python-compute-only-guard.sh`, `smackerel.sh`,
`.github/workflows/ci.yml`, `internal/deploy/**`. **Trace contract (G080):** this scope
declares no `observabilityWorkflow`; G080/G100 no-op.

### Definition of Done (Test Plan ↔ DoD parity)

- [x] SCN-102-C1-01 — the sidecar env carries no managed secret:
  `./smackerel.sh config generate` EXITs 0 and the generated self-hosted bundle's
  `ml.env` contains NONE of `SHELL_SECRET_KEYS` and NONE of the `POSTGRES_*` connection
  parts (SCN-102-C1-01) — proven by a real `bundle_secret_contract_test.go` run
  (evidence pasted). [report.md](report.md) §SCOPE-102-01 Evidence 1.
- [x] The generator **fails loud** if `services.ml.env_allowlist` intersects
  `SHELL_SECRET_KEYS` (tripwire) — proven by a real negative-config test run
  (`TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102`, exact + prefix cases).
  Security F1 extends the same contract: future same-prefix credential suffixes fail
  case-insensitively unless explicitly sanctioned
  (`TestMLEnv_CredentialSuffixRequiresSanction_Spec102`). Evidence 2 and
  [report.md](report.md#f1--future-same-prefix-credentials-fail-loud).
- [x] `smackerel-ml` boots against the projected `ml.env` with every compute var
  present and no missing-env fail-loud error (SCN-102-C1-02) — proven by
  `TestMLEnv_ContainsEveryComputeKey_Spec102` asserting every fail-loud startup key
  (`_check_required_config` + nats_client subscribe) is present. Evidence 1. (Live
  container start is on the self-hosted apply path — operator-gated R-102-A.)
- [x] SCN-102-C1-03 — a re-added secret fails the guard (adversarial):
  `scripts/lint/python-compute-only-guard.sh` PASSES clean on the current tree AND
  its adversarial selftest FAILS on a re-added secret, a forbidden driver, and a direct
  infra-URL read (SCN-102-C1-03/04) — real selftest evidence (10/10);
  `--skip/--force/--ignore/--no-verify` each exit 2. Evidence 3.
- [x] The guard is wired into `./smackerel.sh test pre-push` (EXIT 0 real run) AND into
  CI (`.github/workflows/ci.yml` dedicated step), with NO bypass flag. Evidence 4.
- [x] `deploy/compose.deploy.yml` segments `postgres` onto `data-tier`; a compose
  contract test proves `smackerel-ml` cannot reach `postgres` while `smackerel-core` can
  (SCN-102-C1-05) — real `compose_contract_test.go` evidence (+2 adversarial). Evidence 5.
- [x] Consumer Impact Sweep complete; zero stale first-party references remain for
  `smackerel-ml` + `app.env` in the deploy path; `smackerel-core` `app.env` is
  unchanged. Evidence 6.
- [x] No regression: existing `internal/deploy` contract tests + `./smackerel.sh check`
  EXIT 0. Evidence 6.
- [x] `smackerel-no-defaults` / config-SST honored (no `${VAR:-default}`, no
  language-level fallback for the new allowlist — REQUIRED fail-loud, Gate G028). Evidence 6.
- [x] [report.md](report.md) §SCOPE-102-01 carries the real ≥10-line evidence for each
  item above; anti-fabrication respected.

The following checked rows are a single-file-layout fidelity index for G068. They
link to the later scopes' existing DoD/evidence and add no execution claim:

- [x] SCN-102-C2-02 — alerting survives a re-apply with no manual re-run.
  Evidence: [report.md](report.md) §SCOPE-102-02 and the matching SCOPE-102-02 DoD.
- [x] SCN-102-C2-03 — a fired alert reaches ntfy as a titled message.
  Evidence: [report.md](report.md) §SCOPE-102-02 and the matching SCOPE-102-02 DoD.
- [x] SCN-102-C3-02 — the validator uses real KV math (adversarial).
  Evidence: [report.md](report.md) §SCOPE-102-03 and the matching SCOPE-102-03 DoD.
- [x] SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked.
  Planning-fidelity index only: the active behavior contract is reopened under
  SCOPE-102-03; this row does not claim the 13-builder implementation is complete.
  Evidence: current-session `traceability-guard.sh` mapped all 8 parsed scenarios,
  including SCN-102-C3-01, with `RESULT: PASSED (0 warnings)`; see
  [report.md](report.md#scope-102-03-planning-reconciliation-validation).
  **Phase:** plan. **Claim Source:** executed.
- [x] SCN-102-C4-01 — an unreadable NATS volume must not silently succeed.
  Evidence: [report.md](report.md) §SCOPE-102-04 and the matching SCOPE-102-04 DoD.
- [x] SCN-102-C4-02 — a root-owned manifest must not hard-fail the backup.
  Evidence: [report.md](report.md) §SCOPE-102-04 and the matching SCOPE-102-04 DoD.

> Evidence: see [report.md](report.md) §SCOPE-102-01 (Evidence 1–6) — all commands executed
> via `./smackerel.sh` / `bash scripts/lint/...`; raw ≥10-line outputs pasted (bundle
> secret-absence + 86 compute keys, tripwire exact+prefix, guard selftest 10/10, pre-push
> EXIT 0, network segmentation +2 adversarial, deploy+config regression + `check` EXIT 0).

---

## SCOPE-102-02 — Durable Prometheus → Alertmanager → ntfy routing

**Status:** Done (2026-07-09)
**Tags:** `concern:2` · `repo:smackerel+knb` · `observability`
**Depends On:** SCOPE-102-01 done (shares the `config.sh` bundle-staging block +
`compose.deploy.yml`; rebases on the post-01 tree).

### Use Cases (Gherkin)

Primary outcome — alerting survives every apply with zero manual re-run, and a fired
alert reaches ntfy as a titled message. Contract scenarios `SCN-102-C2-01..04`
([spec.md](spec.md) §4).

```gherkin
Scenario: SCN-102-C2-02 — alerting survives a re-apply with no manual re-run
  Given a stack applied with the monitoring profile
  When apply.sh re-extracts the bundle and restarts the stack
  Then the alerting block and the alertmanager service are still present
    And no post-apply standup script is required
```

```gherkin
Scenario: SCN-102-C2-03 — a fired alert reaches ntfy as a titled message
  Given the alertmanager routes to the self-hosted-alerts ntfy topic
  When an alert fires with a severity label and a summary annotation
  Then the ntfy message has a title and a priority derived from the alert
    And it is not the raw Alertmanager JSON body
```

Also mapped: `SCN-102-C2-01` (bundle carries the alerting block + alertmanager service),
`SCN-102-C2-04` (ntfy endpoint stays adapter-owned; product repo has no ntfy literal).

### Implementation Plan

Cited against [design.md](design.md) → *SCOPE-102-02*.

1. **Alerting block** — append a static `alerting:` block targeting `alertmanager:9093`
   to [`config/prometheus/prometheus.yml.tmpl`](../../config/prometheus/prometheus.yml.tmpl)
   so every `envsubst` render (≈ `config.sh` L2796-2825) and therefore every bundle
   carries it.
2. **Alertmanager service** — add an `alertmanager` service to
   [`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml) under
   `profiles: [monitoring]`, mirroring the sibling `prometheus` posture: read-only root
   - `/tmp` tmpfs, `cap_drop: [ALL]`, `no-new-privileges`, fail-loud
   `ALERTMANAGER_CPU_LIMIT`/`ALERTMANAGER_MEMORY_LIMIT` from
   `deploy_resources.alertmanager.*`, no host port. Pin the image by digest in
   [`deploy/contract.yaml`](../../deploy/contract.yaml) `externalImages` (byte-lockstep
   with `external_images_contract_test.go`).
3. **Generic routing config** — add `config/prometheus/alertmanager.yml` (product-owned,
   generic): port the knb overlay route tree / `group_by` / `inhibit_rules`, but replace
   the operator-private endpoint with `webhook_configs.url_file: /etc/alertmanager/ntfy_url`.
   No ntfy host/topic literal → "no env-specific content" holds. Stage into the bundle
   next to `prometheus.yml`/`alerts.yml` (`config.sh` ≈ L2882-2965; add to
   `bundle-manifest.yaml` + `TAR_FILES`).
4. **Templating bridge** — add `cmd/alertmanager-ntfy-bridge` built into the EXISTING
   core binary/image, exposed as a `monitoring`-profiled compose service on
   `${SMACKEREL_CORE_IMAGE}` (no new external image to pin/sign — OQ-102-1 default). It
   receives the Alertmanager webhook JSON and republishes to ntfy with
   `X-Title`/`X-Priority`/`X-Tags` from the alert `severity` label + `summary`/
   `description` annotations. ntfy base URL read fail-loud from `ALERTMANAGER_NTFY_URL`
   (adapter-injected). Reuse the spec-055 ntfy-publish helper as the templating foundation.
5. **knb adapter (apply-ready)** — in `../<deployment-owner>/<product>/<target>/apply.sh`: write the real
   ntfy base URL to the host file bind-mounted at `/etc/alertmanager/ntfy_url` (fail-loud;
   adapter-owned env-specific value), attach the alertmanager/bridge container(s) to the
  `self-hosted-ntfy` network (idempotent). **Retire**
  `../<deployment-owner>/<product>/<target>/alertmanager-standup.sh` and
  `../<deployment-owner>/<product>/<target>/alertmanager/alertmanager.yml` (superseded by the bundled
   generic config).
6. **Contract test** — add `internal/deploy/alertmanager_bundle_contract_test.go`:
   generate a bundle and assert `prometheus.yml` contains `alertmanager:9093`;
   `docker-compose.yml` declares the `alertmanager` service under `profiles: [monitoring]`;
   the bundle contains `alertmanager.yml` with a `url_file` and NO ntfy literal.

**Consumer Impact Sweep** (retirement of `alertmanager-standup.sh` + `alertmanager/alertmanager.yml`):
enumerate + clean every reference in the knb adapter — `README.md`,
`ACTIVATION_PACKET.md`, any script/doc invoking the standup, `apply.sh` post-apply hooks.
Zero dangling references to the retired standup.

**Shared Infrastructure Impact Sweep** (product `config.sh` bundle staging +
`prometheus.yml.tmpl` are shared by all monitoring consumers): blast radius = every
generated bundle + the monitoring profile. **Canary** = `alertmanager_bundle_contract_test.go`

- the existing `monitoring_*_contract_test.go` (run first). **Rollback** = the
alertmanager service + alerting block are additive under the `monitoring` profile (off by
default); reverting is dropping the block + service (documented). The knb standup file is
retired only after the bundled path is proven green.

**Change Boundary** — allowed: `config/prometheus/prometheus.yml.tmpl`,
`deploy/compose.deploy.yml`, `deploy/contract.yaml`, `config/prometheus/alertmanager.yml`,
`cmd/alertmanager-ntfy-bridge/**`, `scripts/commands/config.sh` (bundle staging region),
`internal/deploy/alertmanager_bundle_contract_test.go`;
`../knb/smackerel/contract.yaml`; `../<deployment-owner>/<product>/<target>/{README.md,apply.sh,params.yaml}`;
`../<deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh`; and the
two retired knb files. Excluded: Grafana provisioning (unchanged by this spec), the 21 rules in
`alerts.yml` (unchanged),
the operator-private ntfy VALUE in the product repo (must never appear).

### Test Plan

**Scope-Kind contract:** no in-scope live E2E row is claimed. The bridge test uses
real bridge code and HTTP against an external-boundary test sink, so it remains a
unit test; full container routing and the real ntfy destination remain R-102-A/C.

| Scenario | Type / Category | Exact test | Command | Live System | Assertion / regression |
| --- | --- | --- | --- | --- | --- |
| SCN-102-C2-01 | functional | `internal/deploy/alertmanager_bundle_contract_test.go::TestBundle_CarriesAlertingBlockAndService_Spec102` | `./smackerel.sh test unit --go --go-run 'TestBundle_CarriesAlertingBlockAndService_Spec102' --verbose` | No | Generated bundle carries the alerting block, Alertmanager, bridge, and generic routing file; omission fails adversarially. |
| SCN-102-C2-02 | functional | `internal/deploy/alertmanager_bundle_contract_test.go::TestBundle_AlertingSurvivesReExtract_Spec102` | `./smackerel.sh test unit --go --go-run 'TestBundle_AlertingSurvivesReExtract_Spec102' --verbose` | No | Consecutive generated bundles retain durable routing without the retired standup. |
| SCN-102-C2-03 | unit | `cmd/alertmanager-ntfy-bridge/bridge_test.go::TestBridge_TitledPriorityNtfyRequest_Spec102` | `./smackerel.sh test unit --go --go-run 'TestBridge_' --verbose` | No | Bridge code emits title/priority/tags and a human body, not raw Alertmanager JSON. |
| SCN-102-C2-04 | functional | `internal/deploy/alertmanager_bundle_contract_test.go::TestAlertmanagerConfig_NoNtfyLiteral_UsesUrlFile_Spec102` | `./smackerel.sh test unit --go --go-run 'TestAlertmanagerConfig_' --verbose` | No | Product bundle carries only the generic bridge target; an operator ntfy literal fails adversarially. |
| SCN-102-C1-05/C2-01 (security F2) | contract | `internal/deploy/alertmanager_bundle_contract_test.go::TestCompose_MonitoringIngressIsolatedFromML_Spec102` | `./smackerel.sh test unit --go --go-run 'TestCompose_MonitoringIngressIsolatedFromML_Spec102' --verbose` | No | Monitoring ingress and ML compute tiers are disjoint; Prometheus alone spans both. |
| SCN-102-C2-04 (security F3) | functional | `../<deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh` | `bash ../<deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh` | No | Adapter status output confirms presence only and never expands the operator-private URL. |

**Impact-aware validation (G079).** No `testImpact` map defined; changed-path seed:
`config/prometheus/**`, `deploy/compose.deploy.yml`, `deploy/contract.yaml`,
`cmd/alertmanager-ntfy-bridge/**`, `scripts/commands/config.sh`, `internal/deploy/**`;
knb `../knb/smackerel/contract.yaml`,
`../<deployment-owner>/<product>/<target>/{README.md,apply.sh,params.yaml,alertmanager-standup.sh,alertmanager/**,tests/unit/alertmanager_ntfy_output_hygiene_test.sh}`.
**Trace contract (G080):** this scope completes the operate-plane alert *delivery* path
but declares no new `observabilityWorkflow`; G080/G100 remain no-op (proof is the bundle
contract + bridge test + operator-gated live delivery), per env-pollution isolation
(test telemetry stays `env=test`).

### Definition of Done (Test Plan ↔ DoD parity)

- [x] A generated bundle's `prometheus.yml` carries an `alerting:` block targeting
  `alertmanager:9093` (SCN-102-C2-01) — real `alertmanager_bundle_contract_test.go`
  evidence. [report.md](report.md) §SCOPE-102-02 Evidence 2/3.
- [x] The deploy compose declares an `alertmanager` service under `profiles:[monitoring]`
  with fail-loud SST resource limits, read-only root + tmpfs, `cap_drop:[ALL]`,
  `no-new-privileges`; the image is digest-pinned in `deploy/contract.yaml` — real
  contract-test evidence (bundle contract + `external_images_contract_test.go` green).
  Security F2 also requires the monitoring ingress tier to remain disjoint from the ML
  compute tier (`TestCompose_MonitoringIngressIsolatedFromML_Spec102`). Evidence 2/3/4
  and [report.md](report.md#f2--monitoring-ingress-isolated-from-ml).
- [x] SCN-102-C2-02 — alerting survives a re-apply with no manual re-run: a bundle
  re-extract retains the durable route with no post-apply standup —
  real re-extract test evidence (`TestBundle_AlertingSurvivesReExtract_Spec102`). Evidence 2.
- [x] SCN-102-C2-03 — a fired alert reaches ntfy as a titled message: the templating
  bridge turns an Alertmanager payload into a titled/priority ntfy message — real
  `bridge_test.go` evidence (real HTTP round-trip to a test
  ntfy sink, `env=test`). The full monitoring-profile CONTAINER stack is the operator-gated
  apply path (R-102-A, needs a core-image rebuild); the in-scope repo-CLI real-execution
  equivalent is the httptest end-to-end. Evidence 1.
- [x] The product `alertmanager.yml` contains a `url_file` and NO env-specific ntfy value
  (SCN-102-C2-04); the knb `apply.sh` injects the real endpoint fail-loud (`ALERTMANAGER_NTFY_URL`
  from `params.yaml alerting.ntfy_url`) — real no-literal test evidence (adversarial RED
  captured: a topic leak in a receiver name was rejected). Security F3 additionally
  requires value-safe adapter output, locked by
  `../<deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh`.
  Evidence 2/5 and [report.md](report.md#f3--operator-private-url-is-never-printed).
- [x] `../<deployment-owner>/<product>/<target>/alertmanager-standup.sh` and
  `../<deployment-owner>/<product>/<target>/alertmanager/alertmanager.yml` are RETIRED; Consumer
  Impact Sweep shows zero dangling references (README updated to the
  retirement posture; `adapter_inventory_test.sh` never required the standup). Evidence 5.
- [x] No regression: existing `monitoring_*_contract_test.go` + `./smackerel.sh check`
  EXIT 0; the `monitoring` profile is off by default (no always-up alertmanager). Evidence 3/4.
- [x] `smackerel-no-defaults` / "no env-specific content" honored (no ntfy literal, no
  `${VAR:-default}`; `ALERTMANAGER_NTFY_URL` adapter-injected). Evidence 3.
- [x] [report.md](report.md) §SCOPE-102-02 carries the real ≥10-line evidence per item.
  Evidence: [report.md](report.md#security-remediation-evidence-2026-07-10) records the
  current-session F2 RED/GREEN proof and complete Spec-102 regression output.
  **Phase:** implement. **Claim Source:** executed.

---

## SCOPE-102-03 — Model-envelope correctness + BUG-026-006

**Status:** Implemented and tested; validation pending (2026-07-11)
**Tags:** `concern:3` · `repo:smackerel` · `bug:BUG-026-006` · `foundation:true`
**Depends On:** SCOPE-102-02 done (independent surface; sequenced for scope isolation).

**Implementation progress (2026-07-11 current security closure).** The two
remaining security findings are addressed in product source and adversarial tests.
OCR profile configuration failures now bypass the ordinary transport fallback and
return only `ollama_profile_config_error` plus the category, before network or cache
write. The structural guard now resolves imports, assignments, client instances,
bound methods, and endpoint aliases across module/function scopes, and requires
exact equality between discovered profiled builders and the 13-row inventory.
Current-session Python, focused Go/KV/provider, format, config, check, lint,
pre-push, artifact, traceability, implementation-reality, and regression-quality
checks pass. The implementation and test registries are complete; validation
remains pending, and this planning reconciliation does not certify the scope. Evidence:
[report.md](report.md#current-security-finding-closure-2026-07-11).
**Phase:** implement. **Claim Source:** executed.

### Use Cases (Gherkin)

Primary outcome — the validator is truthful about real KV footprint, and per-model
`num_ctx` is SST-driven (the host-tag `ollama create` hack is deleted). Contract
scenarios `SCN-102-C3-01..05` ([spec.md](spec.md) §4).

```gherkin
Scenario: SCN-102-C3-02 — the validator uses real KV math (adversarial)
  Given a model profile that understates its measured resident footprint
  When validateModelEnvelopes runs at config generation
  Then it computes resident = weights + KV(num_ctx, num_parallel)
    And it fails loud naming the model, the required MiB, and the envelope
```

```gherkin
Scenario: SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked
  Given a per-model num_ctx declared in the configuration authority
    And the typed service tier selects that model and requests ML compute
  When the compute-only ML tier constructs the Ollama inference request
  Then the inference request carries the SST num_ctx in its options
    And the typed service tier has not issued a direct Ollama inference request
    And no host-side `ollama create ... PARAMETER num_ctx` override is required
```

Also mapped: `SCN-102-C3-03` (uncapped context refused before it can fail to load),
`SCN-102-C3-04` (co-residency posture is a deliberate SST decision keyed off
`max_loaded_models`), `SCN-102-C3-05` (BUG-026-006 output-budget/routing fix advances).

### Implementation Plan

Cited against [design.md](design.md) → *SCOPE-102-03*.

1. **Preserve the existing SST and Go envelope contracts.** Keep the required
  `services.ml.model_memory_profiles[]` shape
  `{model,weights_mib,kv_mib_per_1k_ctx,num_ctx}`, request-level
  `services.ml.ollama_keep_alive`, daemon-wide
  `infrastructure.ollama.{keep_alive,num_parallel,max_loaded_models}`, KV-aware
  `validateModelEnvelopes`, and `max_loaded_models: 1` on-demand posture. Every
  production Ollama model selected by runtime routes has exactly one positive
  `num_ctx`; the deploy path contains no host-side `ollama create` override.
2. **Build one fail-loud request-profile applicator.** In
  [`ml/app/ollama_keepalive.py`](../../ml/app/ollama_keepalive.py), replace the
  fail-open `resolve_ollama_num_ctx` behavior with one shared applicator plus
  LiteLLM-kwargs and native-Ollama-JSON payload adapters. Missing, malformed, or
  non-list profile data; duplicate normalized model entries; non-positive
  `num_ctx`; and an unprofiled selected model all raise a named configuration
  error before network I/O. The applicator merges `options.num_ctx`, preserves
  caller-owned options and top-level `think`, and places SST request
  `keep_alive` at the protocol top level. Daemon controls are never request fields.
3. **Validate at startup and request selection.** Extend
  `ml/app/main.py::_check_required_config` so an effective Ollama provider requires
  and validates `ML_MODEL_MEMORY_PROFILES_JSON` before startup network I/O. Repeat
  the selected-model lookup during request construction because typed requests can
  select a model dynamically after startup. Hosted providers bypass the Ollama
  applicator and receive no Ollama-only options.
4. **Migrate the exact 13-builder inventory.** Apply the shared contract to:
  `agent.handle_invoke`; `card_categories.extract_card_categories`;
  `domain._do_domain_extract`; `drive_classify.classify_drive_file`;
  `intelligence.SynthesisGenerator.generate → _call_llm` (native JSON);
  `main._warmup_domain_model`; `nats_client._handle_search_rerank`;
  `nats_client._handle_digest_generate`; `ocr._do_ocr → extract_text_ollama`
  (native JSON); `processor.process_content`; `routes.chat._dispatch_ollama`;
  `synthesis.handle_extract`; and `synthesis.handle_crosssource`. Remove the
  `model or "llama3"` fallback in `intelligence.py`. Preserve provider prefixes,
  tools, response formats, token budgets, `think`, and determinism options.
5. **Make inventory coverage mechanical.** Add a coverage-matrix/AST guard that
  discovers every production `litellm.acompletion`, direct `/api/generate`, and
  injected production `completion_fn` under `ml/app/`; assert the discovered set
  equals the 13-row inventory and that every registered production builder invokes
  real request construction with its selected profile. A newly added or bypassing
  builder fails the guard.
6. **Lock the typed Go boundary.** Preserve Go as the SST parser, envelope
  validator, model selector/router, NATS typed-request producer, and typed
  `/llm/chat` client. Add contracts proving direct Go Ollama calls are bounded
  read-only `GET /api/tags` probes and that Go has no `/api/chat` or
  `/api/generate` inference client.
7. **Preserve BUG-026-006 output-budget coverage.** Retain and rerun the existing
  SST-owned output-token-budget and routing regressions in
  `ml/tests/test_processor.py` and `ml/tests/test_synthesis.py`; centralizing
  context profiles must not weaken those tests or conflate context capacity with
  output-token budget.

**Consumer Impact Sweep** (shared request-profile adoption): enumerate every
production `litellm.acompletion`, injected production `completion_fn`, direct
`/api/generate`, provider branch, selected-model source, and Go Ollama endpoint.
Keep the exact 13-row inventory in lockstep with builder-specific tests. Verify typed
NATS and `/llm/chat` consumers remain free of Ollama generation fields; bounded Go
`/api/tags` probes remain valid. Re-scan every `model_memory_profiles` reader and
the existing BUG-026-006 output-budget consumers.

**Shared Infrastructure Impact Sweep** (`ml/app/ollama_keepalive.py` becomes a
protected high-fan-out request contract): blast radius = sidecar startup plus all 13
builders across NATS, typed chat, warmup, extraction, OCR, rerank, digest, agent,
and intelligence flows. **Independent canaries** = applicator fail-loud/merge tests,
one LiteLLM builder, one native builder, hosted negatives, and the Go tags-only
boundary before the complete matrix. **Rollback/restore** = revert the entire
SCOPE-102-03 implementation change as one unit before deployment; a mixed tree where
some builders use the applicator and others fall back to daemon defaults is invalid.

**Change Boundary** — allowed production files:
`config/smackerel.yaml`; `internal/config/config.go`;
`cmd/core/wiring_assistant_openknowledge.go`; `ml/app/agent.py`;
`ml/app/card_categories.py`; `ml/app/domain.py`; `ml/app/drive_classify.py`;
`ml/app/intelligence.py`; `ml/app/main.py`; `ml/app/nats_client.py`;
`ml/app/ocr.py`; `ml/app/ollama_keepalive.py`; `ml/app/processor.py`;
`ml/app/routes/chat.py`; `ml/app/synthesis.py`; and `docs/Operations.md`.
Allowed tests: `internal/config/validate_ml_envelope_kv_spec102_test.go` plus
existing adjacent config tests; `tests/integration/agent/loop_test.go`;
`internal/assistant/openknowledge/llm/client_test.go`;
`ml/tests/test_agent.py`; `test_card_categories.py`;
`test_chat_dispatch_parity_spec096.py`; `test_domain.py`;
`test_drive_classify.py`; `test_intelligence_handlers.py`; `test_main.py`;
`test_nats_client.py`; `test_ocr.py`; `test_ollama_keepalive.py`;
`test_processor.py`; and `test_synthesis.py`. Excluded: Go production inference
clients; typed-boundary schema/transport redesign; spec-087/088 model-switch
behavior; QF-companion behavior; auth, persistence, UI, alerting, backup, live
deploy, completed SCOPE-102-01/02/04 surfaces, and collateral cleanup.

### Test Plan

**Scope-Kind contract:** `contract-only`. The true inference/co-residency proof on
the Strix Halo iGPU remains operator-gated R-102-D. In-scope tests execute each
real production request-construction function while replacing only the external
provider network boundary, then assert on the payload produced by production code.

| ID | Scenario | Type / Category | Exact test | Command | Live System | Assertion / regression |
| --- | --- | --- | --- | --- | --- | --- |
| TP-C3-01 | SCN-102-C3-01 | unit | `ml/tests/test_agent.py::test_handle_invoke_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | `agent.handle_invoke` emits selected `options.num_ctx`, top-level `keep_alive`, and preserves determinism options. |
| TP-C3-02 | SCN-102-C3-01 | unit | `ml/tests/test_card_categories.py::test_extract_card_categories_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Card-category extraction retains `think` and receives the selected profile. |
| TP-C3-03 | SCN-102-C3-01 | unit | `ml/tests/test_domain.py::test_do_domain_extract_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Domain extraction adds selected `num_ctx` while retaining top-level `keep_alive`. |
| TP-C3-04 | SCN-102-C3-01 | unit | `ml/tests/test_drive_classify.py::test_classify_drive_file_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Drive classification retains `think` and receives profile fields. |
| TP-C3-05 | SCN-102-C3-01 | unit | `ml/tests/test_intelligence_handlers.py::test_synthesis_generator_generate_applies_native_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Native `/api/generate` JSON carries nested `options.num_ctx` and top-level `keep_alive`; no hardcoded model fallback survives. |
| TP-C3-06 | SCN-102-C3-01 | unit | `ml/tests/test_main.py::test_warmup_domain_model_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Startup warmup emits a capped request instead of an uncapped probe. |
| TP-C3-07 | SCN-102-C3-01 | unit | `ml/tests/test_nats_client.py::test_handle_search_rerank_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Search rerank retains `think` and receives profile fields. |
| TP-C3-08 | SCN-102-C3-01 | unit | `ml/tests/test_nats_client.py::test_handle_digest_generate_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Digest generation uses the Ollama chat route with top-level `keep_alive`, not a legacy uncapped route. |
| TP-C3-09 | SCN-102-C3-01 | unit | `ml/tests/test_ocr.py::test_do_ocr_applies_native_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | OCR native JSON carries nested `options.num_ctx` and top-level `keep_alive`. |
| TP-C3-10 | SCN-102-C3-01 | unit | `ml/tests/test_processor.py::test_process_content_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Existing compliant processor path gains direct payload proof and preserves caller options. |
| TP-C3-11 | SCN-102-C3-01 | unit | `ml/tests/test_chat_dispatch_parity_spec096.py::test_dispatch_ollama_applies_profile_spec102` | `./smackerel.sh test unit --python` | No | Typed chat-selected model receives `options.num_ctx` plus top-level `keep_alive`. |
| TP-C3-12 | SCN-102-C3-01 | unit | `ml/tests/test_synthesis.py::test_handle_extract_threads_sst_num_ctx_spec102` | `./smackerel.sh test unit --python` | No | Existing direct extract proof remains and preserves `think`, format, and options. |
| TP-C3-13 | SCN-102-C3-01 | unit | `ml/tests/test_synthesis.py::test_handle_crosssource_applies_ollama_profile_spec102` | `./smackerel.sh test unit --python` | No | Cross-source synthesis gains selected `num_ctx` without losing top-level `keep_alive`. |
| TP-C3-14 | SCN-102-C3-01 | contract | `ml/tests/test_ollama_keepalive.py::test_production_ollama_builder_inventory_is_complete_spec102` | `./smackerel.sh test unit --python` | No | Discovery equals the exact 13-row inventory and rejects a new unregistered or bypassing production builder. |
| TP-C3-15 | SCN-102-C3-01/C3-03 | unit | `ml/tests/test_main.py::test_startup_rejects_invalid_model_profiles_spec102` | `./smackerel.sh test unit --python` | No | Missing, malformed, duplicate, and non-positive profile data fail at startup before network I/O. |
| TP-C3-16 | SCN-102-C3-01/C3-03 | unit | `ml/tests/test_ollama_keepalive.py::test_unprofiled_selected_ollama_model_fails_before_dispatch_spec102` | `./smackerel.sh test unit --python` | No | A dynamic unprofiled selected model raises before either payload adapter invokes the network boundary. |
| TP-C3-17 | SCN-102-C3-01 | unit | `ml/tests/test_ollama_keepalive.py::test_litellm_adapter_merges_options_and_keep_alive_spec102` | `./smackerel.sh test unit --python` | No | LiteLLM adapter preserves `think`, tools, format, token budget, determinism, and existing options; `keep_alive` is top-level. |
| TP-C3-18 | SCN-102-C3-01 | unit | `ml/tests/test_ollama_keepalive.py::test_native_json_adapter_merges_options_and_keep_alive_spec102` | `./smackerel.sh test unit --python` | No | Native adapter preserves caller options while nesting only `num_ctx`; `keep_alive` remains top-level. |
| TP-C3-19 | SCN-102-C3-01 | unit | `ml/tests/test_chat_dispatch_parity_spec096.py::test_hosted_dispatch_carries_no_ollama_profile_options_spec102` | `./smackerel.sh test unit --python` | No | Hosted chat dispatch contains neither Ollama `options.num_ctx` nor `keep_alive`. |
| TP-C3-20 | SCN-102-C3-01 | unit | `ml/tests/test_intelligence_handlers.py::test_hosted_intelligence_carries_no_ollama_profile_options_spec102` | `./smackerel.sh test unit --python` | No | Hosted intelligence dispatch contains no Ollama-only fields. |
| TP-C3-21 | SCN-102-C3-01 | integration | `tests/integration/agent/loop_test.go::TestSpec102NATSDriverUsesTypedBoundaryWithoutOllamaProtocol` | `./smackerel.sh test integration` | Yes (`env=test`) | Against real NATS, the Go driver emits typed turns without Ollama generation fields. |
| TP-C3-22 | SCN-102-C3-01 | contract | `internal/assistant/openknowledge/llm/client_test.go::TestSpec102GoUsesTypedMLBoundary` | `./smackerel.sh test unit --go --go-run 'TestSpec102GoUsesTypedMLBoundary' --verbose` | No | Go uses typed `/llm/chat`; it does not construct an Ollama payload. |
| TP-C3-23 | SCN-102-C3-01 | contract | `internal/assistant/openknowledge/llm/client_test.go::TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly` | `./smackerel.sh test unit --go --go-run 'TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly' --verbose` | No | Direct Go Ollama URLs are bounded `GET /api/tags`; `/api/chat` and `/api/generate` fail the contract. |
| TP-C3-24 | SCN-102-C3-02 | unit | `internal/config/validate_ml_envelope_kv_spec102_test.go::TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102` | `./smackerel.sh test unit --go --go-run 'TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102' --verbose` | No | Existing adversarial weights+KV rejection remains green. |
| TP-C3-25 | SCN-102-C3-03 | unit | `internal/config/validate_ml_envelope_kv_spec102_test.go::TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102` | `./smackerel.sh test unit --go --go-run 'TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102' --verbose` | No | Existing uncapped refusal and SST-capped acceptance remain green. |
| TP-C3-26 | SCN-102-C3-04 | unit | `internal/config/validate_ml_envelope_kv_spec102_test.go::TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102` | `./smackerel.sh test unit --go --go-run 'TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102' --verbose` | No | Existing posture-gated working-set validation remains green. |
| TP-C3-27 | SCN-102-C3-05 | unit | `ml/tests/test_processor.py::test_output_budget_read_from_sst_not_hardcoded_spec102` plus existing synthesis budget/routing tests | `./smackerel.sh test unit --python` | No | Existing BUG-026-006 SST output-budget/routing regressions remain intact. |

**Impact-aware validation (G079).** No `testImpact` map defined; changed-path seed:
`config/smackerel.yaml`, `internal/{agent,assistant,config}/**`, all exact
Python source/test paths in the Change Boundary, and `docs/Operations.md`.
**Trace contract (G080):** no new
`observabilityWorkflow`; no-op.

### Definition of Done (Test Plan ↔ DoD parity)

> **TP-C3-21 closure (2026-07-11).** The literal Test Plan command
> `./smackerel.sh test integration` exits 0 on the final test tree. A focused
> full-stack run of `TestSpec102NATSDriverUsesTypedBoundaryWithoutOllamaProtocol`
> also exits 0 and logs the 12-field typed NATS request. The broad lane initially
> exposed stale test-only config fixtures using the pre-spec-102 `memory_mib`
> schema and an obsolete model override seam; those fixtures now use the typed
> profile schema/current test-env overrides and fail loud rather than skip. The
> shared real-NATS helper also fails loud when `NATS_URL` is absent.
> The test invocation closed TP-C3-21; this later planning reconciliation
> synchronizes the planner-owned aggregate status while leaving certification
> unchanged for `bubbles.validate`.

- [x] SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked. Every one of the 13 production Python Ollama inference builders emits its
  selected profile's `options.num_ctx` through the shared fail-loud applicator;
  Go issues only typed compute requests and bounded read-only `/api/tags` probes;
  no host-side `ollama create ... PARAMETER num_ctx` override is required.
  Current-session evidence: [report.md](report.md#current-security-finding-closure-2026-07-11).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-01 — `agent.handle_invoke` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-02 — `extract_card_categories` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-03 — `_do_domain_extract` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-04 — `classify_drive_file` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-05 — `SynthesisGenerator.generate → _call_llm` native payload coverage passes with no model fallback. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-06 — `_warmup_domain_model` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-07 — `_handle_search_rerank` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-08 — `_handle_digest_generate` capped-route coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-09 — `ocr._do_ocr → extract_text_ollama` native payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-10 — `process_content` direct payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-11 — `_dispatch_ollama` typed selected-model coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-12 — `handle_extract` direct payload coverage reruns green after foundation changes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-13 — `handle_crosssource` payload coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-14 — the production-builder inventory guard proves exactly 13 registered builders and rejects bypasses. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-15 — startup rejects missing/malformed/duplicate/non-positive profile data before network I/O. Evidence:
  [report.md](report.md#rework-evidence-1---red-then-green-for-non-positive-keep_alive) and
  [Rework Evidence 2](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-16 — request construction rejects an unprofiled selected Ollama model before network I/O. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-17 — LiteLLM option-merge and top-level `keep_alive` coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-18 — native JSON option-merge and top-level `keep_alive` coverage passes. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-19 — hosted chat dispatch carries no Ollama-only fields. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-20 — hosted intelligence dispatch carries no Ollama-only fields. Evidence:
  [report.md](report.md#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-21 — the Go NATS typed-boundary contract passes.
  The exact Test Plan command and focused full-stack selector both exit 0 against
  real ephemeral NATS; the final broad lane also includes the repaired typed-profile
  fixture regressions, contains no skip marker in the touched tests, and cleans up
  all disposable containers/volumes.
  [Evidence](report.md#rework-evidence-4---tp-c3-21-exact-full-stack-closure).
  **Phase:** test. **Claim Source:** executed.
- [x] TP-C3-22 — the Go `/llm/chat` typed-boundary contract passes. Evidence:
  [report.md](report.md#rework-evidence-3---scn-102-c3-020304-typed-go-and-kv-contracts).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-23 — the Go direct-endpoint guard permits only bounded read-only `/api/tags`. Evidence:
  [report.md](report.md#rework-evidence-3---scn-102-c3-020304-typed-go-and-kv-contracts).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-24 — the existing adversarial KV-math validator test reruns green. Evidence:
  [report.md](report.md#rework-evidence-3---scn-102-c3-020304-typed-go-and-kv-contracts).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-25 — the existing uncapped-refusal/capped-acceptance test reruns green. Evidence:
  [report.md](report.md#rework-evidence-3---scn-102-c3-020304-typed-go-and-kv-contracts).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-26 — the existing `max_loaded_models` posture test reruns green. Evidence:
  [report.md](report.md#rework-evidence-3---scn-102-c3-020304-typed-go-and-kv-contracts).
  **Phase:** implement. **Claim Source:** executed.
- [x] TP-C3-27 — the existing BUG-026-006 SST output-budget/routing tests rerun green. Evidence:
  [report.md](report.md#rework-evidence-5---scn-102-c3-05-bug-026-006-output-budget).
  **Phase:** implement. **Claim Source:** executed.
- [x] All 13 builders use the shared fail-loud request-profile applicator; no
  per-builder silent omission, daemon-default fallback, or hardcoded model fallback
  remains. Evidence: [report.md](report.md#scope-102-03-rework-evidence) 13/13
  accounting plus TP-C3-14. **Phase:** implement. **Claim Source:** executed.
- [x] Shared Infrastructure Impact Sweep canaries pass before the complete matrix,
  and Change Boundary review shows zero excluded source families changed. Evidence:
  RED/GREEN foundation canary, final Python/Go matrices, and scoped `git diff --check`
  in [report.md](report.md#scope-102-03-rework-evidence).
  **Phase:** implement. **Claim Source:** executed.
- [x] `smackerel-no-defaults`, isolated-ML-sidecar, and config-SST policies hold:
  required profiles fail loud, Go never issues inference, and hosted providers receive
  no Ollama fields. Evidence: TP-C3-15..20/22/23 and the zero-violation reality
  scan in [report.md](report.md#scope-102-03-rework-evidence).
  **Phase:** implement. **Claim Source:** executed.
- [x] Build Quality Gate passes: `./smackerel.sh config generate`, focused Python/Go
  tests, `./smackerel.sh check`, and the implementation workflow's broader regression
  command have current-session evidence. Format, lint, pre-push, artifact,
  traceability, implementation-reality, and regression-quality gates also pass;
  the reality scan reports zero violations and one file-discovery advisory.
  [Evidence](report.md#current-security-finding-closure-2026-07-11).
  **Phase:** implement. **Claim Source:** executed.
- [x] [report.md](report.md) §SCOPE-102-03 Rework Evidence contains raw,
  provenance-tagged evidence for every checked item. TP-C3-21 evidence is now
  appended; all planner-owned completion items are checked and validation remains
  pending. **Phase:** test.
  **Claim Source:** executed.

---

## SCOPE-102-04 — Backup-adapter durability formalization

**Status:** Implemented and tested; validation pending (2026-07-11)
**Planning status basis:** all in-scope DoD items are checked, and the test-phase
closure records the canonical knb wrapper at `113 tests, 0 failures`, `6 tests,
0 failures`, plus `status_verify_fixture_test.sh OK`. Validation remains pending;
this planning reconciliation does not certify the scope.
**Tags:** `concern:4` · `repo:knb` (+ product schema parity) · `test-formalization`
**Depends On:** SCOPE-102-03 done (independent knb-adapter surface; sequenced last as
lowest-risk test-only work).

> **Re-read note (2026-07-09).** The current
> `../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` ALREADY locks
> **F-1 (NATS volume MISSING ⇒ degraded, stale rotation)**, **OPS-RDY-04 manifest
> capture**, and a **healthy control**. This scope therefore ADDS ONLY the branches not
> yet covered — it does NOT duplicate them. The F-1/F-2/F-3 runtime code in
> `backup.sh`/`apply.sh` is already correct; this scope FORMALIZES it with adversarial
> regression tests + one apply.sh assertion + a product-side schema-parity test.

### Use Cases (Gherkin)

Primary outcome — a degraded backup can NEVER silently succeed, and a root-owned manifest
can NEVER hard-fail the backup. Contract scenarios `SCN-102-C4-01..03` ([spec.md](spec.md) §4).

```gherkin
Scenario: SCN-102-C4-01 — an unreadable NATS volume must not silently succeed
  Given the smackerel backup hook runs
  When the NATS JetStream volume is present but the docker-mediated capture fails
  Then the run is recorded as warning (degraded), never a clean success
    And any stale prior nats-data capture is rotated out, not re-shipped as fresh
```

```gherkin
Scenario: SCN-102-C4-02 — a root-owned manifest must not hard-fail the backup
  Given a manifest.yaml left root:root 0600 (unreadable) by a sudo apply
  When the backup hook attempts to capture the deploy pointer
  Then the manifest capture degrades to not-captured (non-fatal, WITHOUT backup_degraded)
    And the backup still succeeds on its critical postgres capture
    And apply.sh has chowned the manifest back to the operator (0644) so it is readable
```

Also mapped: `SCN-102-C4-03` (backup-status gauge advances on the critical postgres
capture and HOLDS on a failure; JSON matches `internal/backup.Status` schema_version 1).

### Implementation Plan

Cited against [design.md](design.md) → *SCOPE-102-04* and the re-read of the current
`../<deployment-owner>/<product>/<target>/{backup.sh,apply.sh}`. Test harness: `bats` + shell under
`../<deployment-owner>/<product>/<target>/tests/` (`run-tests.sh`), using the present seams
`SMACKEREL_DUMP_DIR`, `SMACKEREL_BACKUP_STATUS_FILE`, `NATS_VOLUME_NAME`,
`SMACKEREL_MANIFEST_FILE`, `SMACKEREL_PG_CONTAINER`.

1. **F-1 NATS-present-but-capture-fails (SCN-102-C4-01, NEW branch).** Extend
  `../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` (or a sibling) with a scenario where
   `docker volume inspect "$NATS_VOLUME"` SUCCEEDS but the `docker run … | tar -x`
   docker-mediated read FAILS. Assert `backup.sh` (a) rotates out the partial
   `nats-data`, (b) emits `META: nats_captured=false` + `META: status=warning`, and
   (c) does NOT record a clean success. (The existing test covers only the volume-MISSING
   branch; this covers the present-but-capture-failed branch at `backup.sh` ≈ L118-124.)
2. **F-2 root-manifest non-fatal (SCN-102-C4-02, NEW).** Add a scenario with a healthy
   pg+NATS capture but an UNREADABLE `SMACKEREL_MANIFEST_FILE` (simulated `root:root 0600`
   / unreadable path). Assert `backup.sh` degrades to `META: manifest_captured=false`
   WITHOUT flipping `backup_degraded` — i.e. NO `META: status=warning`, the run stays a
   clean success on its critical captures. Separately, add an `apply.sh` unit assertion
   that the `$SUDO_USER`-gated `chown "$SUDO_USER:$SUDO_USER" "$MANIFEST"; chmod 0644
   "$MANIFEST"` runs (apply.sh ≈ L1690-1697) so the pointer is operator-readable —
  extend `../<deployment-owner>/<product>/<target>/tests/unit/apply_manifest_contract_test.sh`.
3. **F-3 status advance/hold + schema parity (SCN-102-C4-03, NEW).** Add
  `../<deployment-owner>/<product>/<target>/tests/unit/backup_status_advance_test.sh` driving `backup.sh` with
   `SMACKEREL_BACKUP_STATUS_FILE` set: (a) postgres captured ⇒ `last_status=success`,
   `last_success_unixtime` ADVANCES; (b) postgres skipped ⇒ `last_status=failed`,
   `last_success_unixtime` HOLDS the prior value. Assert the JSON shape matches
   `internal/backup.Status` schema_version 1. On the product side, extend
   [`internal/backup/status_test.go`](../../internal/backup/status_test.go) to assert the
   exact field set the knb hook writes (`schema_version`, `last_run_unixtime`,
   `last_success_unixtime`, `last_status`, `last_duration_ms`, `last_size_bytes`,
   `last_artifact_name`, `last_error`) parses into `backup.Status` with
   `CurrentSchemaVersion == 1` — keeping the two repos in lockstep.

**Consumer Impact Sweep** — none (test-only + additive assertions). No route/contract/
identifier rename.

**Shared Infrastructure Impact Sweep** (the upkeep `backup.sh` hook gates every scheduled
backup): blast radius = every backup run. **Canary** = the new adversarial branches +
the existing `backup_degraded_contract_test.sh` (run via `run-tests.sh` first).
**Rollback** = the tests are additive; no runtime behavior changes (the F-1/F-2/F-3 code
is already landed and correct).

**Change Boundary** — allowed: `../<deployment-owner>/<product>/<target>/tests/**` (extend +
`backup_status_advance_test.sh`), the `apply.sh` chown-back assertion, product
`internal/backup/status_test.go`. Excluded (must remain untouched): the runtime logic of
`backup.sh`/`apply.sh` (already correct — this scope LOCKS it, it does not re-write it);
any live `/srv/backups` path or real host volume (tests use hermetic stubs + seams only,
per env-pollution isolation).

### Test Plan

**Scope-Kind contract:** no live backup is claimed. These are functional tests of
the real hook with hermetic external-command stubs; live volumes, `/srv/backups`,
and restic remain operator validation R-102-A/C.

| Scenario | Type / Category | Exact test | Command | Live System | Assertion / regression |
| --- | --- | --- | --- | --- | --- |
| SCN-102-C4-01 | functional | `../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` | `bash ../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` | No | NATS capture failure records warning, removes partial data, and cannot report clean success. |
| SCN-102-C4-02 | functional | `../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` | `bash ../<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` | No | Unreadable manifest remains non-fatal when critical captures succeed. |
| SCN-102-C4-02 | contract | `../<deployment-owner>/<product>/<target>/tests/unit/apply_manifest_contract_test.sh` | `bash ../<deployment-owner>/<product>/<target>/tests/unit/apply_manifest_contract_test.sh` | No | Apply contract preserves ordered operator chown plus mode `0644`. |
| SCN-102-C4-03 | functional | `../<deployment-owner>/<product>/<target>/tests/unit/backup_status_advance_test.sh` | `bash ../<deployment-owner>/<product>/<target>/tests/unit/backup_status_advance_test.sh` | No | Critical capture advances freshness; failure holds it. |
| SCN-102-C4-03 | unit | `internal/backup/status_test.go::TestLoadStatus_KnbAdapterSchemaParity` | `./smackerel.sh test unit --go --go-run 'TestLoadStatus_KnbAdapterSchemaParity' --verbose` | No | Adapter JSON and product parser retain schema-version and field-set parity. |
| SCOPE-102-04 broad adapter regression | functional | `../<deployment-owner>/<product>/<target>/tests/run-tests.sh` | `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh` | No | Canonical knb wrapper exits zero across all suites. |

**Impact-aware validation (G079).** No `testImpact` map; changed-path seed:
knb `../<deployment-owner>/<product>/<target>/tests/**`, knb `../<deployment-owner>/<product>/<target>/apply.sh`, product
`internal/backup/status_test.go`. **Trace contract (G080):** no new `observabilityWorkflow`;
no-op. **Env-pollution isolation (G115):** tests use hermetic stubs + `SMACKEREL_*` seams
only — no `/srv/backups`, no prod monitoring, no real host volume.

### Definition of Done (Test Plan ↔ DoD parity)

- [x] SCN-102-C4-01 — an unreadable NATS volume must not silently succeed: a NATS
  volume PRESENT but with a FAILING docker-mediated capture causes `backup.sh` to
  rotate out the partial `nats-data`, record `nats_captured=false` + `status=warning`,
  and does NOT report a clean success (SCN-102-C4-01) — real bash-harness evidence.
  (report.md §SCOPE-102-04 Evidence 1, scenario C — `backup_degraded_contract_test.sh` PASS.)
- [x] SCN-102-C4-02 — a root-owned manifest must not hard-fail the backup: an
  UNREADABLE (`root:root 0600`) manifest with a healthy pg/NATS capture causes
  `manifest_captured=false` WITHOUT `backup_degraded`; the run stays a clean success
  (SCN-102-C4-02) — real bash-harness evidence.
  (report.md §SCOPE-102-04 Evidence 1, scenario D — `backup_degraded_contract_test.sh` PASS.)
- [x] `apply.sh` chowns the manifest back to `$SUDO_USER` + `0644` (SCN-102-C4-02) — real
  `apply_manifest_contract_test.sh` evidence.
  (report.md §SCOPE-102-04 Evidence 3 — source contract + adversarial fixture #2 PASS.)
- [x] The backup-status file ADVANCES `last_success_unixtime` on a postgres capture and
  HOLDS it on a failure (SCN-102-C4-03) — real `backup_status_advance_test.sh` evidence.
  (report.md §SCOPE-102-04 Evidence 2 — run 1 advances 1→ts, run 2 HOLDS. PASS.)
- [x] The knb-written status JSON matches `internal/backup.Status` schema_version 1 —
  real product `internal/backup/status_test.go` evidence (two-repo lockstep).
  (report.md §SCOPE-102-04 Evidence 4 — `TestLoadStatus_KnbAdapterSchemaParity` PASS.)
- [x] `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh` (from the Smackerel repo
  root) EXITs 0 with
  ALL suites green including the new adversarial branches; the existing
  `backup_degraded_contract_test.sh` still passes.
  Current-session evidence: [report.md](report.md#test-phase-closure-2026-07-11)
  records EXIT 0 with `113 tests, 0 failures`, `6 tests, 0 failures`, and
  `status_verify_fixture_test.sh OK`.
  **Phase:** test. **Claim Source:** executed.
- [x] Change Boundary is respected and zero excluded file families were changed:
  no runtime-logic change to `backup.sh`/`apply.sh` (test-only + assertions).
  (This scope touched only `tests/**` + the `smackerel/contract.yaml` mirror; `git status`
  shows no backup.sh change and no apply.sh runtime-logic change in SCOPE-102-04.)
- [x] [report.md](report.md) §SCOPE-102-04 carries the real ≥10-line evidence per item.
  Evidence: [report.md](report.md#test-phase-closure-2026-07-11) records the
  current-session canonical-wrapper closure. **Phase:** test. **Claim Source:** executed.

---

## Out-of-Scope Operator Handoff (honored, NOT in-scope work)

Per [spec.md](spec.md) §6, these are operator-gated live-host / deploy-governance steps.
This plan delivers durable, tested, **apply-ready** code; the following are the
post-implementation operator handoff, tracked as external blockers — never faked in-repo:

| # | Handoff step | Risk | Owner | Gate |
| --- | --- | --- | --- | --- |
| a | Live <deploy-host> re-apply of this hardening (all four scopes) | R-102-A | operator | adapter-driven apply; live host mutation |
| b | Rollback drill on <deploy-host> | R-102-B | operator | live host mutation |
| c | knb git reconcile / push of the adapter changes (SCOPE-102-02/04) | R-102-C | operator | operator-gated deploy governance |
| d | Live `ollama ps` co-residency proof for qwen3+gemma4 under ROCm | R-102-D | operator + hardware | needs the real Strix Halo iGPU |

The in-repo deliverable for (d) is the KV-aware validator + the SST `max_loaded_models`
posture (default `1`, on-demand swap); the operator flips to co-resident only once
`ollama ps` proves the fit on <deploy-host>.

---

## Planning Notes

- **Impact-aware validation (G079).** `.github/bubbles-project.yaml` defines no
  `testImpact` map, so `test-impact-plan.sh` produces no first-pass categories; each
  scope's Test Plan lists the exact changed-path seed used for this implemented tree.
- **Trace-contract evidence (G080/G100).** The only declared `observabilityWorkflow` is
  `core.health` (unrelated to these four scopes). This feature declares NO new
  observability workflow, so G080/G100 no-op for every scope. SCOPE-102-02 completes the
  operate-plane alert *delivery* path but does not add a trace workflow; its proof is the
  bundle contract + bridge test + operator-gated live delivery.
- **Terminal discipline.** Product tests run via `./smackerel.sh` (Docker-only, macOS
  host); knb adapter tests run from the product repo via
  `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh` (knb has no CLI — adapter
  scripts/tests are invoked directly). Full unfiltered output, no
  redirection-to-file, no secret values echoed.
- **Recommended continuation:** `/bubbles.workflow specs/102-<deploy-host>-deploy-hardening mode: full-delivery`
  → execute SCOPE-102-01 → 02 → 03 → 04 in order, gating each on its Validation Checkpoint.
