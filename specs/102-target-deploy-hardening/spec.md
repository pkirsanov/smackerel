# Feature: 102 Target Deploy Hardening

**Status:** not_started (spec + design authored; awaiting `bubbles.plan`; ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Release Train:** `mvp`
**Owner Directive (2026-07-09):** During the live target MVP-readiness
validation the core release `639472f7` was deployed and verified, but doing
so applied several EXPEDIENT fixes (a same-file ML env, a fragile post-apply
Alertmanager standup, a host-baked `ollama create` num_ctx override, three
live backup-adapter edits). Replace every expedient with durable, tested,
apply-ready engineering. **No shortcuts, no stopgaps.** The live re-apply,
the rollback drill, and the shared-repo git reconcile/push are operator-gated
and OUT OF SCOPE (see §6).

**Depends On:**
[spec 050 — ML Sidecar Health Isolation](../050-ml-sidecar-health-isolation/spec.md),
[spec 052 — Bundle Secret Injection Contract](../052-bundle-secret-injection-contract/spec.md),
[spec 049 — Monitoring Stack](../049-monitoring-stack/spec.md),
[spec 048 — Backup/Restore Automation](../048-backup-restore-automation/spec.md),
[spec 082 — MVP Target Readiness Hardening](../082-mvp-target-readiness-hardening/spec.md).

**Originates From:** the 2026-07-09 live target MVP-readiness validation of
core `639472f7`. Every finding below is recon-confirmed against the committed
tree of both repos; this spec designs the durable replacement, it does NOT
re-derive the evidence.

**Reuses (product repo):**
[`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml),
[`scripts/commands/config.sh`](../../scripts/commands/config.sh),
[`config/smackerel.yaml`](../../config/smackerel.yaml),
[`internal/config/config.go`](../../internal/config/config.go),
[`config/prometheus/prometheus.yml.tmpl`](../../config/prometheus/prometheus.yml.tmpl),
[`config/prometheus/alerts.yml`](../../config/prometheus/alerts.yml),
[`ml/app/synthesis.py`](../../ml/app/synthesis.py),
[`ml/app/ollama_keepalive.py`](../../ml/app/ollama_keepalive.py),
[`smackerel.sh`](../../smackerel.sh),
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml).

**Reuses (knb self-hosted adapter — cross-repo, apply-ready design only):**
`<deployment-owner>/<product>/<target>/backup.sh`, `<deployment-owner>/<product>/<target>/apply.sh`,
`<deployment-owner>/<product>/<target>/alertmanager/alertmanager.yml`,
`<deployment-owner>/<product>/<target>/alertmanager-standup.sh` (to be RETIRED).

**Canonical guard precedent:**
[`bubbles-isolated-ml-sidecar`](../../.github/skills/bubbles-isolated-ml-sidecar/SKILL.md);
QF spec 089 Scope C (`scripts/lint/python-compute-only-guard.sh`).

---

## 1. Problem / Context

The MVP-readiness validation shipped core `639472f7` to the target host and verified it
live. To get there it applied four classes of expedient fix that trade
durability for speed. Each is a real defect or durability gap in the committed
tree; each becomes one scope.

### Concern 1 — ML-sidecar compute-only secret over-delivery (SECURITY, HIGH)

[`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml) gives
`smackerel-ml` `env_file: ./app.env` — the SAME file `smackerel-core` loads.
So the compute-only Python sidecar's container environment holds every managed
secret in
[`scripts/commands/config.sh::SHELL_SECRET_KEYS`](../../scripts/commands/config.sh)
(`POSTGRES_PASSWORD`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`,
`AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN`, `TELEGRAM_BOT_TOKEN`,
`KEEP_GOOGLE_APP_PASSWORD`, `CARD_REWARDS_GCAL_CREDENTIALS`,
`WEB_REGISTRATION_INVITE_TOKEN`, `LLM_PROVIDER_SECRET_MASTER_KEY`) plus the
`POSTGRES_*` connection parts.

The sidecar holds NO datastore driver and reads NO infra connection URL (the
`bubbles-isolated-ml-sidecar` **code-plane** isolation already PASSES — the
sidecar reaches data only over NATS). But its **env-delivery** over-delivers
credentials, violating the invariant's env-delivery clause: "the generator
[must not] emit the infra URLs / secrets into the sidecar's env." A popped
`pip` dependency in the least-trusted, fastest-churning tier of the system
would find `POSTGRES_PASSWORD` and the auth signing key sitting in
`os.environ`. This is blast-radius the invariant exists to remove.

**Durable target:** a reusable per-service env allowlist so `smackerel-ml`
receives ONLY compute keys and NONE of the datastore/auth secrets, plus a
no-bypass `python-compute-only-guard` wired into `./smackerel.sh test pre-push`
AND CI (forbidden-driver scan + infra-URL-read scan + env-allowlist/secret-
absence assertion), mirroring QF spec 089 Scope C. Network-segmenting
`smackerel-ml` off the postgres-reachable network is designed as defense-in-
depth.

### Concern 2 — Alert routing has no durable delivery path (R-082-C)

The bundled spec-049 Prometheus evaluates the 21 rules in
[`config/prometheus/alerts.yml`](../../config/prometheus/alerts.yml) but the
rendered `prometheus.yml` carries NO `alerting:` block and there is no
Alertmanager in the deploy stack — the rules fire into a void. A LIVE overlay
Alertmanager was stood up out-of-band by `<deployment-owner>/<product>/<target>/alertmanager-standup.sh`,
which **appends** an `alerting:` block to the host's extracted `prometheus.yml`
and SIGHUP-reloads. That script itself documents the fragility: "A subsequent
`apply.sh` re-extracts prometheus.yml from the config bundle and drops the
block, so this script MUST be re-run post-apply." Alerting silently dies on
every deploy until a human re-runs the standup.

**Durable target:** fold the Alertmanager service (under the `monitoring`
compose profile) + the `alerting:`/`alertmanagers:` block INTO the product
deploy compose and config bundle so it survives every apply with zero manual
re-run; deliver alerts to the already-live ntfy `self-hosted-alerts` topic with
message TEMPLATING (title / priority / annotations, not raw Alertmanager JSON).
The operator-private ntfy endpoint stays adapter-owned (no env-specific content
in the product repo). Grafana is deferred (out of scope). The knb
`alertmanager-standup.sh` is retired.

### Concern 3 — Model envelope math does not reflect reality + BUG-026-006

`gemma4:26b`'s uncapped default context (arch `262144` × request parallelism)
predicted ~58 GiB of KV and could not load on the host. The validation baked an
expedient host-side `ollama create gemma4:26b` with `PARAMETER num_ctx 8192` —
a tag-overwrite that lives ONLY on the host, is absent from SST, and is silently
lost on any `ollama pull` / host rebuild.
[`config/smackerel.yaml`](../../config/smackerel.yaml)
`services.ml.model_memory_profiles` states a single static `memory_mib` per
model that understates real KV footprints (e.g. `qwen3:30b-a3b` is configured
at `20480` MiB but measured ~31 GB resident at 32768 context), so
[`internal/config/config.go::validateModelEnvelopes`](../../internal/config/config.go)
validates against a ceiling that does not reflect the running host. qwen3 (text)

+ gemma4 (vision) co-residency was NOT provable on the contended host during the
validation, tying into the 71-95 s synthesis / >30 s domain-extraction latency
that already routed the model-quality root cause of BUG-026-006 to ops.

**Durable target:** make per-model context (`num_ctx`), per-request residency
(`keep_alive`), and daemon-wide loading posture (`max_loaded_models` and
`num_parallel`) CONFIG-DRIVEN (SST, no host-tag hack). The compute-only ML tier
is the sole issuer of Ollama inference requests and MUST apply the SST
`num_ctx` to every profiled inference request as `options.num_ctx`; the typed
service tier validates the model envelope, selects/routes the model, and
requests compute over the existing NATS request/reply or typed sidecar contract
without constructing an Ollama inference request. Recalibrate
`model_memory_profiles` to a KV-aware shape using the real measured footprints;
fix `validateModelEnvelopes` to compute resident memory with real KV math
accounting for `num_parallel`; make a DELIBERATE, documented
co-residency-vs-on-demand-swap decision for qwen3+gemma4; and advance/close the
BUG-026-006 family with the SST output-token-budget + routing fix its ledger
routed to ops.

### Architecture Reconciliation Note (Non-Normative, 2026-07-10)

This note records the implementation evidence used to reconcile the normative
tier contract above; file names and transports are not additional requirements.

+ The current typed service tier is Go. `cmd/core/wiring_agent.go` constructs a
  NATS request/reply driver, and
  `internal/assistant/openknowledge/llm/client.go::Chat` posts a typed
  `ChatRequest` to `<ML_SIDECAR_URL>/llm/chat`. Neither path constructs an
  Ollama `/api/chat` or `/api/generate` inference request.
+ Go owns SST parsing and fail-loud envelope validation in
  `internal/config/config.go`; `cmd/core/wiring_assistant_openknowledge.go`
  converts the KV-aware profiles into resident-memory routing constraints. That
  configuration/routing role does not make Go an Ollama inference client.
+ The current compute-only ML tier is Python. Ollama generation payloads are
  constructed under `ml/app/`; the Spec-102 `num_ctx` implementation resolves
  the per-model value in `ml/app/ollama_keepalive.py` and applies it as
  `options.num_ctx` in `ml/app/processor.py`, `ml/app/synthesis.py`, and
  `ml/app/routes/chat.py`. Protocol-level options such as `keep_alive` and
  structured-extraction `think` remain top-level where the Ollama chat protocol
  requires them.
+ Go does perform read-only Ollama health and model-discovery probes, including
  `GET /api/tags`. Those probes do not run model inference, do not carry
  generation options, and are outside FR-102-C3-2's inference-request contract.

The isolated-sidecar invariant remains unchanged: the ML tier computes and may
call the model daemon, but it holds no datastore credentials or drivers; the
typed service tier owns persistence and requests ML work only through typed
service boundaries.

### Concern 4 — Backup-adapter durability fixes need spec-tracking + regression

Three knb backup-adapter fixes were landed LIVE during the validation and need
formalization with adversarial regression tests so they cannot silently regress
(the knb files were edited AGAIN after those commits — the design re-reads the
current state):

+ **F-1** — `backup.sh` captures the NATS JetStream volume via a docker-mediated
  read (mount the named volume read-only into a throwaway `alpine`, stream a tar
  of its CONTENTS into the operator-owned dump dir). The prior root-`0700` `cp`
  degraded EVERY backup to `warning` when the non-root upkeep engine could not
  read the volume mountpoint.
+ **F-2** — `apply.sh` chowns `manifest.yaml` back to the invoking operator
  (`0644`) after a sudo apply, and the manifest capture in `backup.sh` is
  best-effort/non-fatal. A sudo-apply had left the pointer `root:root 0600`,
  which HARD-FAILED the next scheduled backup under `set -e`.
+ **F-3** — the spec-048 backup-status file is written by `backup.sh` and
  bind-mounted read-only into `smackerel-core` so `internal/backup.Watcher`
  advances `smackerel_backup_last_success_unixtime` for the `SmackerelBackupStale`
  alert.

**Durable target:** spec-track F-1/F-2/F-3 and add adversarial regression tests:
a backup with an unreadable/missing NATS volume must NOT silently succeed (it
must degrade to `warning`, never a clean `success`); a `root:root 0600` manifest
must NOT hard-fail the backup; the status file must advance on a critical-asset
capture and HOLD on a failure.

---

## 2. Goals / Non-Goals

### Goals

+ Replace all four expedient fixes with durable, tested, apply-ready
  engineering in the smackerel product repo and its knb self-hosted adapter.
+ Every regression-preventing test is **adversarial**: it fails if the
  protection is removed (secret re-added to the sidecar env; alerting block
  dropped from the bundle; an understated model profile; a silently-succeeding
  degraded backup).
+ Preserve every existing gate: NO-DEFAULTS / fail-loud SST (Gate G028),
  read-only-root posture (FR-045-003), externalImages lockstep, the spec-045
  resource contract, the spec-042 tailnet-edge bind invariants, "no
  env-specific content", and the isolated-ML-sidecar code-plane isolation.
+ Keep the product repo target-agnostic; concrete host topology (the ntfy
  endpoint, the postgres-network attach, the manifest ownership) stays in the
  knb adapter.

### Non-Goals

+ Grafana provisioning / dashboards (deferred; the alert delivery path is the
  MVP-critical piece).
+ Re-architecting the ML sidecar transport (its NATS-driven compute-only shape
  is retained; only env-delivery is tightened).
+ Any change to the QF-companion boundary or QF packet metadata (Principle 10
  untouched).

## Outcome Contract

**Intent:** Replace each target-readiness expedient with a durable,
configuration-owned contract while keeping model compute isolated from the
state-owning service tier.

**Success Signal:** A regenerated deployment bundle preserves secret isolation
and alert delivery; every profiled Ollama inference request receives its
configured context cap from the compute-only ML tier; model-envelope validation
reflects weights plus KV residency; and degraded backup captures cannot be
reported as clean successes.

**Hard Constraints:** The typed service tier owns persistence, credentials,
model-envelope validation, and routing. The compute-only ML tier holds no
datastore credential or driver and is the only tier that issues Ollama inference
requests. Required runtime configuration fails loud, product artifacts remain
target-agnostic, and concrete self-hosted values remain adapter-owned.

**Failure Condition:** The feature fails even if repository checks are green if
a later apply drops alerting or secret isolation, a profiled inference request
can bypass its SST context cap, the typed service tier gains a direct Ollama
inference path, or a failed critical backup capture advances the success signal.

---

## 3. Product Principle Alignment

This spec is infrastructure hardening; it strengthens the engineering track and
touches product principles 6 and 8 lightly. No principle is violated.

+ **smackerel-no-defaults (fail-loud SST)** — every new runtime value
  (per-service env allowlist, per-model `num_ctx`, per-request `keep_alive`,
  daemon-wide `max_loaded_models`/`num_parallel`, Alertmanager resource limits,
  the ntfy endpoint) is a REQUIRED
  fail-loud SST/adapter value. No `${VAR:-default}`, no language-level fallback.
+ **bubbles-isolated-ml-sidecar** — Concern 1 closes the env-delivery half of
  the invariant the code-plane half already satisfies, and wires the no-bypass
  compute-only guard the invariant prescribes.
+ **bubbles-config-sst** — per-model posture, `num_parallel`, and the env
  allowlist all resolve from the single `config/smackerel.yaml` authority
  through the generated env; the host-tag `ollama create` hack is removed.
+ **bubbles-capability-foundation-design (DE4)** — the one new capability seam is
  the per-service compute-only env projection; it is a parameterized helper
  (`project_service_env`) with a SINGLE concrete consumer (`smackerel-ml`), not a
  new N≥2 provider foundation. The honest DE4 classification is a
  single-implementation justification (see `## Domain Capability Model`). The
  other three concerns harden existing single adapters and introduce no new
  capability seam.
+ **knb adapter ownership boundary** — the product repo owns the generic
  Alertmanager service + routing structure + templating; the knb adapter owns
  the operator-private ntfy endpoint value, the cross-network attach, and the
  manifest ownership fix.
+ **anti-fabrication / real-evidence** — every DoD item is backed by a real
  test run with captured evidence; the co-residency claim that requires a live
  host is designed as an operator-gated proof, never faked (see §6).
+ **Principle 6 (Invisible By Default)** — durable alert routing means the
  system speaks up (ntfy) only when an alert genuinely fires, and stops silently
  swallowing them after every deploy.
+ **Principle 8 (Trust Through Transparency)** — the model-envelope validator
  becomes truthful about real KV footprint instead of validating a fiction;
  degraded backups are recorded, never masked as clean successes.

---

## Domain Capability Model

This feature HARDENS four EXISTING capability surfaces — the knb self-hosted deploy
adapter, the spec-049 monitoring stack, the spec-048 backup adapter, and the
spec-050 ML sidecar. It does NOT introduce a new N≥2 provider / plugin / driver
foundation. The 65 G094 capability-vocabulary trigger hits are EXISTING-infra
wording (the "deploy adapter", the "backup adapter", the FORBIDDEN ML datastore
`driver`s the compute-only guard scans for, the litellm `provider` secret, the
`alertmanager`), not a new pluggable-provider abstraction being added here.

The one genuinely new capability seam is the **per-service compute-only env
projection** introduced by SCOPE-102-01 (FR-102-C1-1/2).

### Single-Capability Justification

The env projection is realized by one parameterized generator helper,
`project_service_env(<service>, <app.env>, <out.env>, <allowlist>)`
([`scripts/commands/config.sh`](../../scripts/commands/config.sh) L604), and it
has EXACTLY ONE concrete consumer today: `smackerel-ml`
([`scripts/commands/config.sh`](../../scripts/commands/config.sh) L3071, driven by
`services.ml.env_allowlist`). `smackerel-core` is deliberately NOT projected — it
is the trusted typed tier that legitimately owns `SHELL_SECRET_KEYS`. The stack
contains exactly one compute-only Python sidecar, so there is exactly one
implementation and no dependent overlay scope. The helper is written parameterized
(service + allowlist as arguments) so a future second sidecar — or a split of the
ML tier — inherits secret-free delivery for free, but this spec does NOT claim,
build, or test a second implementation and does NOT split the design into
foundation + overlay scopes. Per DE4 this is a single-capability,
single-implementation feature; the design-side detail lives in design.md
`### Single-Implementation Justification`.

---

## 4. Scenarios (Gherkin)

Scenario IDs (`SCN-102-*`) are stable contracts for downstream tests.

### Concern 1 — ML-sidecar compute-only secret isolation

**SCN-102-C1-01 — the sidecar env carries no managed secret**

```gherkin
Given a config bundle generated for the self-hosted target
When the smackerel-ml service loads its env from the bundle
Then its environment contains none of the SHELL_SECRET_KEYS
  (POSTGRES_PASSWORD, AUTH_SIGNING_ACTIVE_PRIVATE_KEY,
   AUTH_AT_REST_HASHING_KEY, AUTH_BOOTSTRAP_TOKEN, TELEGRAM_BOT_TOKEN,
   KEEP_GOOGLE_APP_PASSWORD, CARD_REWARDS_GCAL_CREDENTIALS,
   WEB_REGISTRATION_INVITE_TOKEN, LLM_PROVIDER_SECRET_MASTER_KEY)
  And it contains none of the POSTGRES_* connection parts
```

**SCN-102-C1-02 — the sidecar still boots with every compute key it needs**

```gherkin
Given the compute-only env projection for smackerel-ml
When the sidecar starts against that projected env
Then every env var the sidecar actually reads is present
  (NATS_URL, OLLAMA_URL, ML_*, SMACKEREL_ENV, SMACKEREL_AUTH_TOKEN,
   the embedding-model name, PROMPT_CONTRACTS_DIR, HF_HOME,
   SENTENCE_TRANSFORMERS_HOME)
  And startup succeeds with no missing-env fail-loud error
```

**SCN-102-C1-03 — a re-added secret fails the guard (adversarial)**

```gherkin
Given the python-compute-only-guard is wired into pre-push and CI
When a change re-adds env_file: ./app.env to smackerel-ml
  Or a SHELL_SECRET_KEY appears in the smackerel-ml env delivery
Then the guard exits non-zero and names the offending key
  And there is no --skip / --force / --ignore flag that suppresses it
```

**SCN-102-C1-04 — a forbidden driver or infra-URL read fails the guard**

```gherkin
Given the python-compute-only-guard scans the ML Python surface
When a module imports psycopg / asyncpg / sqlalchemy / redis / kafka / pymongo
  Or reads DATABASE_URL / POSTGRES_URL / REDIS_URL / RABBITMQ_URL directly
Then the guard exits non-zero and names the file and line
```

**SCN-102-C1-05 — defense-in-depth: the sidecar cannot reach postgres**

```gherkin
Given the deploy compose segments postgres onto a data-tier network
  that only smackerel-core joins
When smackerel-ml is on the compute-tier network
Then smackerel-ml has no network route to the postgres container
  And smackerel-core retains its route to postgres
```

### Concern 2 — Durable Prometheus → Alertmanager → ntfy routing

**SCN-102-C2-01 — the generated bundle carries the alerting block + service**

```gherkin
Given a config bundle generated with the monitoring profile in mind
When the bundle is extracted
Then prometheus.yml contains an alerting: block targeting alertmanager:9093
  And docker-compose.yml declares an alertmanager service under the
    monitoring profile
  And the bundle contains the alertmanager routing config
```

**SCN-102-C2-02 — alerting survives a re-apply with no manual re-run**

```gherkin
Given a stack applied with the monitoring profile
When apply.sh re-extracts the bundle and restarts the stack
Then the alerting block and the alertmanager service are still present
  And no post-apply standup script is required
```

**SCN-102-C2-03 — a fired alert reaches ntfy as a titled message**

```gherkin
Given the alertmanager routes to the self-hosted-alerts ntfy topic
When an alert fires with a severity label and a summary annotation
Then the ntfy message has a title and a priority derived from the alert
  And it is not the raw Alertmanager JSON body
```

**SCN-102-C2-04 — the ntfy endpoint stays adapter-owned**

```gherkin
Given the product repo alertmanager config
When it is inspected for env-specific content
Then it contains no operator ntfy host, IP, or topic value
  And the real endpoint is injected by the knb adapter at apply time
```

### Concern 3 — Model-envelope correctness + BUG-026-006

**SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked**

```gherkin
Given a per-model num_ctx declared in the configuration authority
  And the typed service tier selects that model and requests ML compute
When the compute-only ML tier constructs the Ollama inference request
Then the inference request carries the SST num_ctx in its options
  And the typed service tier has not issued a direct Ollama inference request
  And no host-side `ollama create ... PARAMETER num_ctx` override is required
```

**SCN-102-C3-02 — the validator uses real KV math (adversarial)**

```gherkin
Given a model profile that understates its measured resident footprint
When validateModelEnvelopes runs at config generation
Then it computes resident = weights + KV(num_ctx, num_parallel)
  And it fails loud naming the model, the required MiB, and the envelope
```

**SCN-102-C3-03 — an uncapped context is refused before it can fail to load**

```gherkin
Given gemma4:26b with its uncapped arch context and the host envelope
When config generation validates the ollama envelope
Then it refuses the uncapped configuration fail-loud
  And it accepts the SST-capped num_ctx that fits the host
```

**SCN-102-C3-04 — co-residency posture is a deliberate SST decision**

```gherkin
Given daemon max_loaded_models and per-request keep_alive configured in SST
When the posture is on-demand-swap (max_loaded_models = 1)
Then the validator does not require the co-resident sum to fit the envelope
When the posture is co-resident (max_loaded_models > 1)
Then the validator requires the co-resident working set to fit the envelope
```

**SCN-102-C3-05 — BUG-026-006 output-budget/routing fix advances**

```gherkin
Given the SST-owned domain/synthesis output-token budget and model routing
When a domain-extraction / synthesis request runs against the configured model
Then the output-token budget is read from SST (not a hardcoded 2000)
  And BUG-026-006 advances toward closure with real before/after evidence
```

### Concern 4 — Backup-adapter durability

**SCN-102-C4-01 — an unreadable NATS volume must not silently succeed**

```gherkin
Given the smackerel backup hook runs
When the NATS JetStream volume is missing or the capture fails
Then the run is recorded as warning (degraded), never a clean success
  And any stale prior nats-data capture is rotated out, not re-shipped as fresh
```

**SCN-102-C4-02 — a root-owned manifest must not hard-fail the backup**

```gherkin
Given a manifest.yaml left root:root 0600 by a sudo apply
When the backup hook attempts to capture the deploy pointer
Then the manifest capture degrades to not-captured (non-fatal)
  And the backup still succeeds on its critical postgres capture
  And apply.sh has chowned the manifest back to the operator so it is readable
```

**SCN-102-C4-03 — the backup-status gauge advances on the critical capture**

```gherkin
Given the F-3 backup-status file bind-mounted into smackerel-core
When a backup captures the postgres dump successfully
Then last_success_unixtime advances and last_status is success
When a backup fails to capture postgres
Then last_success_unixtime HOLDS its prior value and last_status is failed
```

---

## 5. Requirements

### Concern 1

+ **FR-102-C1-1** — `smackerel-ml` MUST receive a compute-only env projection
  (a per-service allowlist) instead of the full `app.env`; the projection MUST
  contain none of `SHELL_SECRET_KEYS` and none of the `POSTGRES_*` connection
  parts.

+ **FR-102-C1-2** — the per-service env allowlist MUST be an SST-declared,
  reviewable set (config authority + generator), designed as a reusable
  per-service projection, with `smackerel-ml` as the first consumer.
+ **FR-102-C1-3** — a `python-compute-only-guard` (forbidden-driver scan +
  infra-URL-read scan + secret-absence/env-allowlist assertion) MUST be wired
  into `./smackerel.sh test pre-push` AND CI, with NO bypass flag.
+ **FR-102-C1-4** — the deploy compose SHOULD segment `postgres` onto a
  data-tier network that only `smackerel-core` joins (defense-in-depth); the
  primary control remains the env allowlist + the guard.

### Concern 2

+ **FR-102-C2-1** — the rendered `prometheus.yml` MUST carry an
  `alerting:`/`alertmanagers:` block targeting the in-stack Alertmanager; it
  MUST be produced by the generator (template) so it is present in every bundle.

+ **FR-102-C2-2** — the deploy compose MUST declare an `alertmanager` service
  under the `monitoring` profile with fail-loud SST resource limits, read-only
  root + tmpfs allowlist, `cap_drop: [ALL]`, `no-new-privileges`.
+ **FR-102-C2-3** — the config bundle MUST include the Alertmanager routing
  config; a contract test MUST assert the generated bundle contains the alerting
  block AND the alertmanager service.
+ **FR-102-C2-4** — alerts MUST be delivered to the ntfy `self-hosted-alerts` topic
  with message TEMPLATING (title / priority / tags from alert labels +
  annotations), not the raw Alertmanager JSON.
+ **FR-102-C2-5** — the operator-private ntfy endpoint MUST be adapter-injected
  (fail-loud); the product repo MUST contain no env-specific ntfy value. The knb
  `alertmanager-standup.sh` MUST be retired.

### Concern 3

+ **FR-102-C3-1** — per-model `num_ctx`, per-request `keep_alive`, and
  daemon-wide `max_loaded_models`/`num_parallel` posture MUST be SST-driven,
  fail-loud, with no host-tag `ollama create` override on the deploy path.

+ **FR-102-C3-2** — the compute-only ML tier MUST be the sole issuer of direct
  Ollama inference requests. Every such request for a model with an SST memory
  profile MUST carry that profile's `num_ctx` as `options.num_ctx`; request-level
  options such as `keep_alive` and `think` MUST be placed where the selected
  Ollama protocol honors them. The typed service tier MUST validate and route
  the model request, then request compute through a typed sidecar boundary; it
  MUST NOT construct or issue a direct Ollama inference request. Read-only
  health and model-discovery probes are not inference requests.
+ **FR-102-C3-3** — `model_memory_profiles` MUST be recalibrated to a KV-aware
  shape using the real measured footprints; `num_parallel` MUST be SST-driven
  (`OLLAMA_NUM_PARALLEL`).
+ **FR-102-C3-4** — `validateModelEnvelopes` MUST compute resident memory as
  `weights + KV(num_ctx, num_parallel)` and fail loud on a real over-envelope
  configuration; the co-residency sum check MUST key off `max_loaded_models`.
+ **FR-102-C3-5** — the qwen3(text)+gemma4(vision) co-residency-vs-swap decision
  MUST be recorded as a deliberate, documented SST posture.
+ **FR-102-C3-6** — the BUG-026-006 output-token budget MUST become SST-owned
  (replacing the hardcoded `2000` in `ml/app/processor.py`/`synthesis.py`) with
  the model-routing fix; BUG-026-006 (and its sibling BUG-026-007) MUST advance
  with real evidence.

### Concern 4

+ **FR-102-C4-1** — a backup with a missing/unreadable NATS volume MUST degrade
  to `warning` and rotate out any stale capture; it MUST NEVER be recorded as a
  clean `success`. (Adversarial regression test.)

+ **FR-102-C4-2** — a `root:root 0600` manifest MUST NOT hard-fail the backup;
  `apply.sh` MUST chown the manifest back to the operator (`0644`). (Adversarial
  regression test.)
+ **FR-102-C4-3** — the spec-048 backup-status file MUST advance
  `last_success_unixtime` on a successful postgres capture and HOLD it on a
  failure; the schema MUST match `internal/backup.Status` (schema_version 1).

---

## 6. Dependencies, Risks & Tracked External Blockers

These are explicitly OUT OF SCOPE and tracked as external, operator-gated
steps — NOT silently absorbed or faked. The design output is durable, tested,
apply-ready code; the actual apply is a separate operator-authorized step.

| # | External blocker | Owner | Why out of scope | Tracked as |
|---|------------------|-------|------------------|-----------|
| a | The live target-host re-apply of this hardening | operator | adapter-driven apply; live host mutation | risk R-102-A |
| b | The rollback drill on the target host | operator | live host mutation | risk R-102-B |
| c | The shared-repo (knb) git reconcile / push | operator | operator-gated deploy governance | risk R-102-C |
| d | Live `ollama ps` co-residency proof for qwen3+gemma4 under ROCm | operator + hardware | needs the real Strix Halo iGPU | risk R-102-D |

**Risks**

+ **R-102-D** — Concern 3 recalibrates `model_memory_profiles` from the
  footprints measured during the 2026-07-09 validation and enforces the KV math
  in-repo; the TRUE co-residency of qwen3+gemma4 under ROCm on the contended
  host is only verifiable on the target host. The validator uses the measured ceilings;
  the live `ollama ps` proof stays an operator-host step (mirrors spec 082
  R-082-A). The SST `max_loaded_models` posture lets the operator pick the safe
  on-demand-swap default until co-residency is proven.
+ **R-102-C** — the knb-adapter changes (Concern 2 ntfy-endpoint injection +
  standup retirement; Concern 4 regression tests) are authored as apply-ready
  code in the knb repo but their reconcile/push and any live effect are
  operator-gated.

---

## 7. Scope Map (for bubbles.plan)

Four cleanly-separated scopes, each independently testable:

| Scope | Concern | Primary surfaces | Adversarial test |
|-------|---------|------------------|------------------|
| SCOPE-102-01 | ML-sidecar compute-only secret isolation | `config.sh` (env projection), `compose.deploy.yml` (ml env + network), `scripts/lint/python-compute-only-guard.sh`, `smackerel.sh` pre-push, `ci.yml` | re-added secret / forbidden driver fails the guard |
| SCOPE-102-02 | Durable Prometheus→Alertmanager→ntfy routing | `prometheus.yml.tmpl`, `compose.deploy.yml` (alertmanager), `config.sh` (bundle), templating bridge, contract tests; knb adapter (ntfy endpoint, retire standup) | bundle missing the alerting block / service fails the contract test |
| SCOPE-102-03 | Model-envelope correctness + BUG-026-006 | `config/smackerel.yaml` (per-model posture + KV-aware profiles), `config.go::validateModelEnvelopes`, typed service-to-sidecar request paths, Python ML-tier Ollama request builders (num_ctx), BUG-026-006/007 advance | understated profile fails the validator |
| SCOPE-102-04 | Backup-adapter durability formalization | knb `backup.sh`, `apply.sh`; product `internal/backup` status contract | degraded backup silently succeeding fails the test |

Each scope's Test Plan and DoD are authored by `bubbles.plan` from this spec +
`design.md`.
