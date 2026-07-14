# Report 102 — <deploy-host> Deploy Hardening

> **Implementation rework (2026-07-10, SCOPE-102-03).** The design-owned
> 13-builder inventory is now implemented through one shared fail-loud Python
> request-profile foundation. Current-session evidence replaces the invalidated
> partial interpretation. TP-C3-21 now passes through the exact Test Plan command
> (`./smackerel.sh test integration`) and a focused full-stack selector. The broad
> lane exposed two stale config-validation fixtures that still used the pre-spec-102
> `memory_mib` schema and an obsolete model override seam; both test-owned defects
> were repaired and the final unfiltered integration lane exits 0. Only TP-C3-21 is
> closed here; aggregate scope/status/certification items remain non-terminal.
>
> **Anti-fabrication (NON-NEGOTIABLE).** Nothing here is claimed as
> already-passing. No `Exit Code: 0`, no `--- PASS`, no "all tests pass", no
> "Test Evidence" block may be written until the command was actually run and its
> real ≥10-line output pasted. Every regression-preventing test is **adversarial**
> — its evidence MUST show the guard/validator/test FAILING when the protection is
> removed (RED-before) and PASSING when it is present (GREEN-after). Home-path PII
> in any captured transcript MUST be redacted to `~/` before it is written here.
> No secret VALUE is ever pasted (presence/length only).
>
> **Cross-repo.** SCOPE-102-01/02/03 land in the smackerel product repo (tests via
> `./smackerel.sh`); SCOPE-102-02/04 also author apply-ready code in the knb
> `<deployment-owner>/<product>/<target>` adapter (tests from the product repo via
> `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh`).
>
> **Out-of-scope operator handoff (never faked here).** The live <deploy-host> re-apply
> (R-102-A), the rollback drill (R-102-B), the knb git reconcile/push (R-102-C),
> and the live `ollama ps` co-residency proof (R-102-D) are operator-gated. Their
> evidence is captured by the operator on <deploy-host>, NOT in this file.
>
> Planning sources: [scopes.md](scopes.md),
> [scenario-manifest.json](scenario-manifest.json),
> [test-plan.json](test-plan.json), and [uservalidation.md](uservalidation.md).

## Summary

The implementation evidence below records durable work performed after the
2026-07-09 live <deploy-host> MVP-readiness validation of core `639472f7`: (1) a reusable
per-service env projection + a no-bypass compute-only guard so `smackerel-ml`
holds no managed secret; (2) an Alertmanager service + alerting block + ntfy
templating folded into the product bundle; (3) SST model posture, KV-aware
`validateModelEnvelopes`, a shared fail-loud request-profile foundation consumed
by all 13 production Python Ollama builders, typed-only Go boundaries, and the
BUG-026-006 output budget; and (4) adversarial backup-adapter regressions.
SCOPE-102-03 remains In Progress because this surgical test invocation closes only
TP-C3-21 and does not promote its aggregate scenario, Build Quality, scope, status,
or certification items. No live deploy, rollback, reconcile, commit, or push ran.

## Test Evidence

Evidence is captured per scope below. Each block MUST contain the exact command
(via `./smackerel.sh` for the product,
`bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh`
for the knb adapter), the ≥10-line raw output, and the exit code as actually
observed. Adversarial tests show RED-before + GREEN-after.

### SCOPE-102-01 — ML-sidecar compute-only secret isolation

**Status:** DONE (2026-07-09). All DoD items backed by real executed evidence below.
Env-constrained note: a LIVE `smackerel-ml` container start against `ml.env` is on
the self-hosted apply path (operator-gated R-102-A); the in-scope boot-safety proof is
the contract test asserting every fail-loud startup key the sidecar reads
(`ml/app/main.py::_check_required_config` + `nats_client` subscribe) is present in the
projected `ml.env` (so the sidecar's `sys.exit(1)` missing-config gate cannot fire).

Cross-repo: smackerel product only (no knb changes in this scope).

#### Evidence 1 — SCN-102-C1-01/02: projected `ml.env` is secret-free AND retains every compute key

`internal/deploy/bundle_secret_contract_test.go` (runs a REAL `bash config.sh --env
self-hosted --bundle`, extracts `ml.env` from the generated tarball, asserts absence of
every `config.SecretKeys()` + `POSTGRES_*` + `DATABASE_URL`, and presence of every
fail-loud startup key):

```
$ ./smackerel.sh test unit --go --go-run 'TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102|TestMLEnv_ContainsEveryComputeKey_Spec102' --verbose
=== RUN   TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102
    bundle_secret_contract_test.go:860: SCN-102-C1-01 OK — projected ml.env carries no managed secret / POSTGRES_* / DATABASE_URL. Projected keys (86):
        AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING
        ... (86 compute-only keys: AGENT_*, EMBEDDING_MODEL, ENABLE_OLLAMA,
            KEEP_GOOGLE_EMAIL, LLM_API_KEY, LLM_MODEL, LLM_PROVIDER, ML_*, NATS_*,
            OLLAMA_*, PHOTOS_INTELLIGENCE_EMBED_MODEL, PROMPT_CONTRACTS_DIR,
            SMACKEREL_AUTH_TOKEN, SMACKEREL_ENV) ...
--- PASS: TestMLEnv_ExcludesManagedSecretsAndPostgres_Spec102 (2.70s)
=== RUN   TestMLEnv_ContainsEveryComputeKey_Spec102
--- PASS: TestMLEnv_ContainsEveryComputeKey_Spec102 (1.26s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  4.130s
```

Direct bundle extraction confirms the secret-absence out of band (host bundle stage runs
under Docker/GNU-tar in CI; the projection body itself is portable and the Go test proves
it in Docker):

```
$ SMACKEREL_HARDWARE_TIER=cpu bash scripts/commands/config.sh --env self-hosted --bundle ... (projected ml.env dumped) ; \
  grep -E '^(POSTGRES_PASSWORD|AUTH_SIGNING_ACTIVE_PRIVATE_KEY|AUTH_AT_REST_HASHING_KEY|AUTH_BOOTSTRAP_TOKEN|TELEGRAM_BOT_TOKEN|KEEP_GOOGLE_APP_PASSWORD|CARD_REWARDS_GCAL_CREDENTIALS|WEB_REGISTRATION_INVITE_TOKEN|LLM_PROVIDER_SECRET_MASTER_KEY|POSTGRES_|DATABASE_URL)' ml.env
(no match — the 9 SHELL_SECRET_KEYS, all POSTGRES_* parts, and DATABASE_URL are ABSENT)
```

The projected 86 keys cover every fail-loud startup key: `NATS_URL, LLM_PROVIDER,
LLM_MODEL, OLLAMA_URL, ML_PROCESSING_DEGRADED_FALLBACK_ENABLED, SMACKEREL_ENV,
ML_LOG_LEVEL, ML_EMBEDDING_WORKERS, ML_EMBEDDING_QUEUE_MAX, ML_HEALTH_LATENCY_SLA_MS,
ML_OLLAMA_KEEP_ALIVE, ML_STRUCTURED_EXTRACTION_THINKING, SMACKEREL_AUTH_TOKEN,
NATS_MAX_RECONNECT_ATTEMPTS, NATS_RECONNECT_TIME_WAIT_SECONDS, NATS_CONSUMER_MAX_DELIVER,
NATS_CONSUMER_ACK_WAIT_SECONDS` — so the sidecar's boot-time fail-loud check cannot fire.

**Claim Source:** executed.

**Google Keep sidecar-bridge note (surfaced during implementation — for bubbles.security):**
`ml/app/keep_bridge.py` reads `KEEP_GOOGLE_APP_PASSWORD` (a SHELL_SECRET_KEY) LAZILY, only
when a `keep.sync.request` arrives AND `connectors.google-keep.gkeep_enabled` is true. It is
DISABLED by default (`gkeep_enabled: false`; self-hosted does not override it), and the read is
lazy (registration at boot does not read it; the handshake degrades gracefully returning
`"KEEP_GOOGLE_APP_PASSWORD is required"`). The design's SCN-102-C1-01 deliberately lists
`KEEP_GOOGLE_APP_PASSWORD` in the must-be-absent set, so it is projected OUT
(secure-by-default; the most sensitive secret — a Google account app-password — leaves the
least-trusted tier). Consequence: enabling Keep live-sync on the isolated sidecar becomes an
explicit operator security decision that would trip the allowlist∩secret tripwire — a
conscious carve-out, not a silent regression. `CARD_REWARDS_GCAL_CREDENTIALS` is NOT read by
the sidecar (core-only), so projecting it out is a pure win.

#### Evidence 2 — SCN-102-C1-03 tripwire: allowlist ∩ SHELL_SECRET_KEYS fails loud (adversarial)

`project_service_env` refuses an `env_allowlist` that intersects a managed secret — both an
EXACT secret key and a PREFIX glob covering one:

```
$ ./smackerel.sh test unit --go --go-run 'TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102'
=== RUN   TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102
=== RUN   TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/exact_secret_key
=== RUN   TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/prefix_glob_covering_a_secret
--- PASS: TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102 (2.54s)
    --- PASS: TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/exact_secret_key (…)
    --- PASS: TestMLEnv_AllowlistIntersectsSecret_FailsLoud_Spec102/prefix_glob_covering_a_secret (…)
```

The test injects `POSTGRES_PASSWORD` (exact) and `AUTH_*` (prefix) into `env_allowlist` and
asserts config generation exits non-zero AND names `TRIPWIRE` + the offending secret. Removing
the tripwire makes the test fail. **Claim Source:** executed.

#### Evidence 3 — SCN-102-C1-03/04 guard: clean run + adversarial selftest (10/10) + no bypass

```
$ bash scripts/lint/python-compute-only-guard.sh ; echo "GUARD_RC=$?"
python-compute-only-guard: OK - dependency scan: no forbidden datastore driver in 2 dependency file(s) under ml (nats-py is the sanctioned transport)
python-compute-only-guard: OK - infra-URL scan: no direct DATABASE_URL/POSTGRES_URL/REDIS_URL/RABBITMQ_URL read in *.py (NATS_URL is the sanctioned wire)
python-compute-only-guard: OK - env-delivery shape: smackerel-ml loads the projected ./ml.env (not ./app.env); services.ml.env_allowlist is declared
python-compute-only-guard: clean - smackerel Python (ml) is compute-only: no datastore driver, no direct datastore-URL read, secret-free projected env delivery.
GUARD_RC=0

$ bash scripts/lint/python-compute-only-guard.selftest.sh ; echo "SELFTEST_RC=$?"
SELFTEST OK   - clean live tree passes (exit 0, mentions "clean")
SELFTEST OK   - forbidden datastore driver trips the guard (exit 1, mentions "psycopg2")
SELFTEST OK   - direct datastore-URL read trips the guard (exit 1, mentions "DATABASE_URL")
SELFTEST OK   - re-added env_file: ./app.env trips the guard (exit 1, mentions "app.env")
SELFTEST OK   - dropped env_allowlist trips the guard (exit 1, mentions "env_allowlist")
SELFTEST OK   - nats-py + NATS_URL do NOT trip the guard (sanctioned wire) (exit 0, mentions "clean")
SELFTEST OK   - bypass flag --skip exits 2 (no bypass)
SELFTEST OK   - bypass flag --force exits 2 (no bypass)
SELFTEST OK   - bypass flag --ignore exits 2 (no bypass)
SELFTEST OK   - bypass flag --no-verify exits 2 (no bypass)

python-compute-only-guard selftest: 10 passed, 0 failed
SELFTEST_RC=0
```

Note smackerel-specific: `nats-py` / `NATS_URL` are the SANCTIONED typed contract wire (the
sidecar's only data path), so they are deliberately NOT forbidden (unlike QF spec 089).
**Claim Source:** executed.

#### Evidence 4 — guard wired no-bypass into `./smackerel.sh test pre-push` (+ CI step)

```
$ ./smackerel.sh test pre-push ; echo "PREPUSH_RC=$?"
Running macOS/WSL portability guard (host-run operator surface: smackerel.sh + scripts/)...
...
macOS/WSL portability guard: OK
Running python-compute-only guard (smackerel-ml compute-only invariant)...
python-compute-only-guard: OK - dependency scan: no forbidden datastore driver in 2 dependency file(s) under ml (nats-py is the sanctioned transport)
python-compute-only-guard: OK - infra-URL scan: no direct DATABASE_URL/POSTGRES_URL/REDIS_URL/RABBITMQ_URL read in *.py (NATS_URL is the sanctioned wire)
python-compute-only-guard: OK - env-delivery shape: smackerel-ml loads the projected ./ml.env (not ./app.env); services.ml.env_allowlist is declared
python-compute-only-guard: clean - ...
python-compute-only guard: OK
PREPUSH_RC=0
```

CI wiring: `.github/workflows/ci.yml` `lint-and-test` job — dedicated step
`Compute-only ML sidecar guard (spec 102)` → `bash scripts/lint/python-compute-only-guard.sh`.
No `--skip/--force/--ignore/--no-verify` exists (each exits 2, proven above). **Claim Source:** executed.

#### Evidence 5 — SCN-102-C1-05 network segmentation: postgres unreachable from the compute tier

`internal/deploy/compose_contract_test.go` asserts the live `deploy/compose.deploy.yml` puts
postgres on `data-tier` ONLY, `smackerel-ml` on `compute-tier` (NOT data-tier), and
`smackerel-core` on BOTH; two adversarial fixtures (ml-on-data-tier / postgres-on-compute-tier)
prove the assertion has bite:

```
=== RUN   TestNetworkSegmentation_MLCannotReachPostgres_Spec102
    compose_contract_test.go:…: contract OK: postgres is data-tier only; smackerel-ml is compute-tier only (no postgres route); smackerel-core straddles both tiers
--- PASS: TestNetworkSegmentation_MLCannotReachPostgres_Spec102 (0.00s)
=== RUN   TestNetworkSegmentation_AdversarialMLOnDataTier_Spec102
--- PASS: TestNetworkSegmentation_AdversarialMLOnDataTier_Spec102 (0.00s)
=== RUN   TestNetworkSegmentation_AdversarialPostgresOnComputeTier_Spec102
--- PASS: TestNetworkSegmentation_AdversarialPostgresOnComputeTier_Spec102 (0.00s)
```

**Claim Source:** executed.

#### Evidence 6 — no regression (deploy + config contract suites + `./smackerel.sh check`)

```
$ ./smackerel.sh --env=dev config generate ; echo "cfg_rc=$?"          # cfg_rc=0
$ ./smackerel.sh test unit --go --go-run 'Compose|Bundle|MLEnv|Network|Monitoring|ExternalImages|Envsubst|SecretKeys|Prometheus|Searxng'
ok      github.com/smackerel/smackerel/internal/config  0.068s
ok      github.com/smackerel/smackerel/internal/deploy  15.648s
...
$ ./smackerel.sh check ; echo "CHECK_RC=$?"
config-validate: .../config/generated/dev.env.tmp.71000 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_RC=0
```

Consumer Impact Sweep: `deploy/compose.deploy.yml` smackerel-ml now loads `./ml.env`;
`smackerel-core` keeps `./app.env`; zero stale deploy-path `smackerel-ml`+`app.env`
references (`deploy/contract.yaml` untouched — it is image-only). The DEV `docker-compose.yml`
keeps its shared `${SMACKEREL_ENV_FILE}` env-file (dev has no managed secrets — inline dev
values per FR-052-011); `TestComposeEnvFileSharedAcrossCoreAndMlServices` stays green.
smackerel-no-defaults honored: `services.ml.env_allowlist` is REQUIRED fail-loud (Gate G028,
no `${VAR:-default}`). **Claim Source:** executed.

**Files changed (smackerel):** `config/smackerel.yaml` (env_allowlist),
`scripts/commands/config.sh` (`project_service_env` + `_env_allowlist_match` + bundle staging

+ manifest + TAR_FILES), `deploy/compose.deploy.yml` (ml `env_file: ./ml.env` + data/compute
network segmentation), `scripts/lint/python-compute-only-guard.sh` (+`.allowlist` +
`.selftest.sh`), `smackerel.sh` (pre-push wiring), `.github/workflows/ci.yml` (CI step),
`internal/deploy/bundle_secret_contract_test.go` (ml.env tests + tripwire),
`internal/deploy/compose_contract_test.go` (network segmentation tests).

### SCOPE-102-02 — Durable Prometheus → Alertmanager → ntfy routing

**Status:** DONE (2026-07-09). All DoD items backed by real executed evidence below.

**Resolved architecture (documented for bubbles.validate).** The design's SCOPE-102-02
points 4/5 are slightly loose about where the ntfy endpoint lives; the correct, real
templating topology is `Prometheus → Alertmanager → alertmanager-ntfy-bridge → ntfy`.
The product `config/prometheus/alertmanager.yml` routes via `url_file:
/etc/alertmanager/ntfy_url` (NO endpoint literal — SCN-102-C2-04); the bundle renders
that file to the GENERIC in-stack bridge address (`http://alertmanager-ntfy-bridge:9099/`,
a compose service name, not operator-private); the bridge holds the OPERATOR-PRIVATE
ntfy endpoint via `ALERTMANAGER_NTFY_URL`, injected by knb `apply.sh` from
`params.yaml alerting.ntfy_url`. This is the only topology that yields real templating
(X-Title/X-Priority/X-Tags) AND keeps the product repo endpoint-free.

**Bridge decision.** The templating bridge is self-contained stdlib HTTP (`cmd/alertmanager-ntfy-bridge`)
built into the ALREADY-pinned core image (no new external image to pin/sign — design
OQ-102-1 default). It does NOT reuse `internal/notification/source/ntfy` (that package
RECEIVES ntfy webhooks; the bridge PUBLISHES to ntfy — a different direction).

Cross-repo: smackerel product (all product surfaces) + knb `<deployment-owner>/<product>/<target>` adapter
(ntfy injection + standup retirement).

#### Evidence 1 — SCN-102-C2-03: bridge templates an Alertmanager payload into a titled/priority ntfy message (real HTTP round-trip)

`cmd/alertmanager-ntfy-bridge/bridge_test.go` POSTs a REAL Alertmanager webhook-v4 JSON to
the REAL bridge handler, which makes a REAL HTTP POST to a stub ntfy sink (httptest), and
asserts the captured X-Title/X-Priority/X-Tags + that the body is the human summary, NOT
the raw JSON:

```
$ ./smackerel.sh test unit --go --go-run 'TestBridge_' --verbose
=== RUN   TestBridge_TitledPriorityNtfyRequest_Spec102
--- PASS: TestBridge_TitledPriorityNtfyRequest_Spec102 (0.01s)
=== RUN   TestBridge_SeverityPriorityMapping_Spec102
=== RUN   TestBridge_SeverityPriorityMapping_Spec102/severity=critical
=== RUN   TestBridge_SeverityPriorityMapping_Spec102/severity=warning
=== RUN   TestBridge_SeverityPriorityMapping_Spec102/severity=info
=== RUN   TestBridge_SeverityPriorityMapping_Spec102/severity=
--- PASS: TestBridge_SeverityPriorityMapping_Spec102 (0.00s)
    --- PASS: TestBridge_SeverityPriorityMapping_Spec102/severity=critical (0.00s)
    --- PASS: TestBridge_SeverityPriorityMapping_Spec102/severity=warning (0.00s)
    --- PASS: TestBridge_SeverityPriorityMapping_Spec102/severity=info (0.00s)
    --- PASS: TestBridge_SeverityPriorityMapping_Spec102/severity= (0.00s)
=== RUN   TestBridge_ResolvedIsLowPriority_Spec102
--- PASS: TestBridge_ResolvedIsLowPriority_Spec102 (0.00s)
=== RUN   TestBridge_NewNtfyRequestHeaders_Spec102
--- PASS: TestBridge_NewNtfyRequestHeaders_Spec102 (0.00s)
ok      github.com/smackerel/smackerel/cmd/alertmanager-ntfy-bridge     0.015s
```

The primary test asserts (critical firing alert) `X-Priority=5`, `X-Tags` contains
`rotating_light`+`backup`, `X-Title` contains `FIRING`+`SmackerelBackupStale`, body is the
summary, AND adversarially that the body is NOT the raw JSON blob (no `"version"`/`"alerts"`/
leading `{`). Env-constrained note: the full monitoring-profile CONTAINER stack (real
prom/alertmanager container → bridge → ntfy container) is the operator-gated apply path
(R-102-A) and requires a core-image rebuild with the bridge; the in-scope, repo-CLI,
real-execution equivalent is this httptest end-to-end (real bridge handler + real HTTP POST
to a test ntfy sink, env=test). **Claim Source:** executed.

#### Evidence 2 — SCN-102-C2-01/02/04: the generated bundle carries the alerting block + services + url_file (survives re-extract), no ntfy literal

`internal/deploy/alertmanager_bundle_contract_test.go` generates a REAL self-hosted bundle via
`bash config.sh --bundle`, extracts it, and asserts the alerting wiring:

```
$ ./smackerel.sh test unit --go --go-run 'TestBundle_CarriesAlertingBlockAndService_Spec102|TestBundle_AlertingSurvivesReExtract_Spec102|TestAlertmanagerConfig_NoNtfyLiteral_UsesUrlFile_Spec102|TestBundle_AdversarialMissingAlertingBlock_Spec102|TestBundle_AdversarialMissingAlertmanagerService_Spec102|TestAlertmanagerConfig_AdversarialNtfyLiteral_Spec102' --verbose
=== RUN   TestBundle_CarriesAlertingBlockAndService_Spec102
    alertmanager_bundle_contract_test.go:237: SCN-102-C2-01 OK — bundle carries: prometheus.yml alerting -> alertmanager:9093; alertmanager + alertmanager-ntfy-bridge services (profiles:[monitoring]); alertmanager.yml + generic bridge url_file
--- PASS: TestBundle_CarriesAlertingBlockAndService_Spec102 (2.48s)
=== RUN   TestBundle_AlertingSurvivesReExtract_Spec102
    alertmanager_bundle_contract_test.go:288: SCN-102-C2-02 OK — two consecutive bundle extractions both carry the alerting block + alertmanager service (no post-apply standup required)
--- PASS: TestBundle_AlertingSurvivesReExtract_Spec102 (2.73s)
=== RUN   TestAlertmanagerConfig_NoNtfyLiteral_UsesUrlFile_Spec102
--- PASS: TestAlertmanagerConfig_NoNtfyLiteral_UsesUrlFile_Spec102 (2.63s)
=== RUN   TestBundle_AdversarialMissingAlertingBlock_Spec102
    alertmanager_bundle_contract_test.go:342: adversarial OK — missing alerting block rejected: contract violation: prometheus.yml has NO alerting.alertmanagers block — the 21 alert rules would fire into a void (R-082-C)...
--- PASS: TestBundle_AdversarialMissingAlertingBlock_Spec102 (0.00s)
=== RUN   TestBundle_AdversarialMissingAlertmanagerService_Spec102
    alertmanager_bundle_contract_test.go:357: adversarial OK — missing alertmanager service rejected...
    alertmanager_bundle_contract_test.go:372: adversarial OK — missing bridge service rejected: ...ntfy would receive the raw Alertmanager JSON instead of titled/priority messages (SCN-102-C2-03)
--- PASS: TestBundle_AdversarialMissingAlertmanagerService_Spec102 (0.00s)
=== RUN   TestAlertmanagerConfig_AdversarialNtfyLiteral_Spec102
    alertmanager_bundle_contract_test.go:391: adversarial OK — inline ntfy literal rejected: ...receiver "ntfy-self-hosted-alerts" uses an inline `url: http://self-hosted-ntfy:8080/self-hosted-alerts`...
--- PASS: TestAlertmanagerConfig_AdversarialNtfyLiteral_Spec102 (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  5.232s
```

Adversarial RED-proof captured DURING implementation: the no-literal check initially FAILED
against my first-draft `alertmanager.yml` because it embedded the ntfy TOPIC in the receiver
name (`ntfy-self-hosted-alerts`) — `contract violation: alertmanager.yml contains the
operator-private ntfy literal "self-hosted-alerts"`. Renaming the receiver to the generic
`ntfy-bridge` + purging the topic from comments made it GREEN, proving the check has real
bite (a topic leak in the product repo is rejected). **Claim Source:** executed.

#### Evidence 3 — SCN-102-C2-01: config generate renders the alerting block + emits ALERTMANAGER_* SST; check EXIT 0

```
$ ./smackerel.sh --env=dev config generate
config-validate: .../config/generated/dev.env.tmp.36793 OK
Generated .../config/generated/dev.env
Generated .../config/generated/nats.conf
Generated .../config/generated/prometheus.yml

$ grep -nA4 'alerting:' config/generated/prometheus.yml
49:alerting:
50-  alertmanagers:
51-    - static_configs:
52-        - targets:
53-            - "alertmanager:9093"

$ grep -nE 'ALERTMANAGER_' config/generated/dev.env
663:ALERTMANAGER_IMAGE=prom/alertmanager:v0.28.0@sha256:d5155cfac40a6d9250ffc97c19db2c5e190c7bc57c6b67125c94903358f8c7d8
664:ALERTMANAGER_CONTAINER_PORT=9093
665:ALERTMANAGER_BRIDGE_LISTEN_PORT=9099
666:ALERTMANAGER_CPU_LIMIT=0.5
667:ALERTMANAGER_MEMORY_LIMIT=128M
668:ALERTMANAGER_BRIDGE_CPU_LIMIT=0.25
669:ALERTMANAGER_BRIDGE_MEMORY_LIMIT=64M

$ ./smackerel.sh check
config-validate: .../config/generated/dev.env.tmp.39420 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_RC=0
```

The alertmanager image is digest-pinned (byte-lockstep with `deploy/contract.yaml`
externalImages, locked by `external_images_contract_test.go`); the three-way port parity
(SST `monitoring.alertmanager.container_port` == prometheus alerting target `9093` == compose
`--web.listen-address=:${ALERTMANAGER_CONTAINER_PORT}`) is asserted by the bundle contract test.
`smackerel-no-defaults` honored: every new key is REQUIRED fail-loud (Gate G028); the
operator-private `ALERTMANAGER_NTFY_URL` is NOT emitted here (adapter-injected). **Claim Source:** executed.

#### Evidence 4 — no regression (full internal/deploy + internal/config packages) + guards clean

```
$ ./smackerel.sh test unit --go --go-run 'Compose|Monitoring|ExternalImages|Alertmanager|Bundle|Prometheus|Resource|Filesystem|Network|Envsubst|Searxng|Ollama|Nats|Clients|Dockerfile|SecretKeys|MLEnv'
ok      github.com/smackerel/smackerel/internal/config  1.445s
ok      github.com/smackerel/smackerel/internal/deploy  22.890s

$ bash scripts/lint/python-compute-only-guard.sh ; echo "GUARD_RC=$?"
python-compute-only-guard: clean - smackerel Python (ml) is compute-only ...
GUARD_RC=0

$ bash .github/bubbles/scripts/macos-portability-guard.sh smackerel.sh scripts/
...
PASS: the scanned surface is WSL+macOS portable.
```

A RED-during-implementation note: adding the alerting block first tripped
`TestMonitoringScrapeContract_LiveTemplate`/`TestMonitoringRender_*` because my block COMMENT
contained a literal `${ALERTMANAGER_CONTAINER_PORT}` that the template's envsubst-allowlist
scanner (which scans the whole file, comments included) flagged as an unknown var. Rewording
the comment to drop the `${}` syntax (the target itself is the literal `alertmanager:9093`,
parity-locked) made all three GREEN — proving those guards have bite. **Claim Source:** executed.

#### Evidence 5 — knb adapter (apply-ready): ntfy injection + network attach + standup retirement; contract tests green

`<deployment-owner>/<product>/<target>/apply.sh` now injects `ALERTMANAGER_NTFY_URL` from `params.yaml
alerting.ntfy_url` into `app.env` on every apply (fail-loud, mirroring HOST_BIND_ADDRESS /
OLLAMA_RENDER_GID) and, post-compose-up, attaches the `alertmanager-ntfy-bridge` container to
`params.yaml alerting.ntfy_network` (idempotent, best-effort-with-loud-warning). The fragile
`alertmanager-standup.sh` + its overlay `alertmanager/alertmanager.yml` are RETIRED. Consumer
Impact Sweep: the ONLY remaining `alertmanager-standup` reference is the retirement-explaining
prose in README.md; `adapter_inventory_test.sh` never required the standup.

```
$ rm -f <deployment-owner>/<product>/<target>/alertmanager-standup.sh <deployment-owner>/<product>/<target>/alertmanager/alertmanager.yml
$ grep -rniE 'alertmanager-standup|alertmanager/alertmanager\.yml' <deployment-owner>/<product>/<target>/
CLEAN: no dangling references in <deployment-owner>/<product>/<target>

$ shellcheck -x <deployment-owner>/<product>/<target>/apply.sh   # (clean, exit 0)

$ for t in readme_contract_test.sh adapter_inventory_test.sh apply_manifest_contract_test.sh apply_audit_schema_test.sh; do bash <deployment-owner>/<product>/<target>/tests/unit/$t; done
readme_contract_test.sh OK
adapter_inventory_test.sh OK
apply_manifest_contract_test.sh OK
apply_audit_schema_test: PASSED=22 FAILED=0
```

Env-constrained: the LIVE apply of the ntfy injection + network attach is the operator-gated
<deploy-host> re-apply (R-102-A) + knb reconcile/push (R-102-C) — apply-ready, not run live here.
Environmental failure dispositioned in the `## Discovered Issues` ledger below (DI-102-01):
`tests/unit/shred-and-remove.bats` test 7 fails on the macOS dev host (BSD `dd` vs GNU `dd` —
a spec-001 secret-removal test in a file this scope did NOT touch); it blocks `run-tests.sh`
under `set -e`, so the shell contract tests were run directly (all green above).
**Claim Source:** executed.

**Files changed (smackerel):** `config/smackerel.yaml` (monitoring.alertmanager + deploy_resources.alertmanager[_ntfy_bridge]),
`scripts/commands/config.sh` (ALERTMANAGER_* resolution + emission + bundle staging of alertmanager.yml + rendered url_file),
`config/prometheus/prometheus.yml.tmpl` (static alerting block), `config/prometheus/alertmanager.yml` (NEW, generic, url_file),
`deploy/compose.deploy.yml` (alertmanager + alertmanager-ntfy-bridge services), `deploy/contract.yaml` (alertmanager externalImage),
`cmd/alertmanager-ntfy-bridge/{main.go,bridge_test.go}` (NEW bridge + tests), `Dockerfile` (build bridge into core image),
`internal/deploy/alertmanager_bundle_contract_test.go` (NEW), `internal/deploy/external_images_contract_test.go` (bridge in projectBuiltServices).
**Files changed (knb):** `<deployment-owner>/<product>/<target>/apply.sh` (ntfy injection + network attach), `<deployment-owner>/<product>/<target>/params.yaml` (alerting section),
`<deployment-owner>/<product>/<target>/README.md` (posture update); RETIRED `<deployment-owner>/<product>/<target>/alertmanager-standup.sh` + `<deployment-owner>/<product>/<target>/alertmanager/alertmanager.yml`.

### SCOPE-102-03 — Model-envelope correctness + BUG-026-006

**Status:** IN PROGRESS. Production and unit/contract implementation is 13/13;
TP-C3-21 is GREEN through the exact full integration command and a focused
full-stack selector. Aggregate SCOPE-102-03 completion remains outside this
surgical invocation.

**Reconciled architecture (design-owned authority).** (a) `num_ctx` is folded
into each `model_memory_profiles` entry (`{model, weights_mib, kv_mib_per_1k_ctx, num_ctx}`)
— a single-source-per-model shape rather than a separate `model_posture` list — read by
the Go validator and the Python request-profile applicator. (b) The `ollama create
num_ctx` "hack" was NEVER in the committed tree (it was a live-host tag-overwrite on <deploy-host>);
the durable replacement is per-request SST profile application. (c) Go validates/routes
through typed NATS and `/llm/chat` boundaries and retains bounded read-only `/api/tags`
probes; Python alone constructs Ollama inference requests. (d) All 13 Python
production Ollama-capable builders across 12 files now consume the shared
request-profile applicator. No production Ollama model is intentionally unprofiled.

Cross-repo: smackerel product only.

#### SCOPE-102-03 Rework Evidence

The shared foundation in `ml/app/ollama_keepalive.py` owns
`OllamaProfileConfigError`, `OllamaRequestProfile`,
`parse_ollama_model_profiles`, `load_ollama_model_profiles`,
`resolve_ollama_request_profile`, `apply_ollama_profile_to_litellm`, and
`apply_ollama_profile_to_native_json`. Ollama routes require a unique normalized
profile, positive integer `num_ctx`, and positive duration/integer `keep_alive`.
The adapters copy caller payloads, merge `options.num_ctx`, preserve other
options/`think`/tools/format/budgets/determinism, and place `keep_alive` at the
protocol top level. Hosted providers bypass the adapters unchanged.

| # | Builder | Required direct payload proof |
| --- | --- | --- |
| 1 | `agent.handle_invoke` | selected `options.num_ctx`; top-level `keep_alive`; determinism options preserved |
| 2 | `card_categories.extract_card_categories` | selected `num_ctx`; top-level `think` and `keep_alive` preserved |
| 3 | `domain._do_domain_extract` | selected `num_ctx`; existing top-level `keep_alive` preserved |
| 4 | `drive_classify.classify_drive_file` | selected `num_ctx`; top-level `think`/`keep_alive` preserved |
| 5 | `intelligence.SynthesisGenerator.generate` | native JSON nested `options.num_ctx`; top-level `keep_alive`; no hardcoded model fallback |
| 6 | `main._warmup_domain_model` | startup request is profile-capped |
| 7 | `nats_client._handle_search_rerank` | selected `num_ctx`; `think`/`keep_alive` preserved |
| 8 | `nats_client._handle_digest_generate` | Ollama chat route; nested `num_ctx`; top-level `keep_alive` |
| 9 | `ocr._do_ocr` | native JSON nested `num_ctx`; top-level `keep_alive` |
| 10 | `processor.process_content` | direct payload proof; existing options and output budget preserved |
| 11 | `routes.chat._dispatch_ollama` | typed selected-model profile plus top-level `keep_alive` |
| 12 | `synthesis.handle_extract` | existing direct proof reruns after applicator centralization |
| 13 | `synthesis.handle_crosssource` | selected `num_ctx`; top-level `keep_alive` preserved |

All 13 rows above have direct production-function payload tests and pass in the
current Python suite. TP-C3-14 additionally AST-discovers exactly these 13 provider
calls and requires the correct profile adapter to occur before each call. Missing,
malformed, duplicate, non-positive, and unprofiled selections fail before network
I/O; hosted branches carry no Ollama-only fields. Exact test names and commands are
in [scopes.md](scopes.md) and [test-plan.json](test-plan.json).

**Phase:** implement. **Claim Source:** executed.

#### Rework Evidence 1 - RED then GREEN for non-positive keep_alive

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 1 (RED), then 0 after the shared resolver fix
**Output:**

```text
___________ test_resolve_keep_alive_rejects_non_positive_spec102[0] ____________
E       Failed: DID NOT RAISE RuntimeError
___________ test_resolve_keep_alive_rejects_non_positive_spec102[-1] ___________
E       Failed: DID NOT RAISE RuntimeError
___________ test_resolve_keep_alive_rejects_non_positive_spec102[0s] ___________
E       Failed: DID NOT RAISE RuntimeError
__________ test_resolve_keep_alive_rejects_non_positive_spec102[-30m] __________
E       Failed: DID NOT RAISE RuntimeError
FAILED ml/tests/test_ollama_keepalive.py::test_resolve_keep_alive_rejects_non_positive_spec102[0]
FAILED ml/tests/test_ollama_keepalive.py::test_resolve_keep_alive_rejects_non_positive_spec102[-1]
FAILED ml/tests/test_ollama_keepalive.py::test_resolve_keep_alive_rejects_non_positive_spec102[0s]
FAILED ml/tests/test_ollama_keepalive.py::test_resolve_keep_alive_rejects_non_positive_spec102[-30m]
4 failed, 586 passed, 2 skipped in 97.96s (0:01:37)
```

**Result:** PASS. The RED isolated exactly the four adversarial values; the
shared resolver now rejects them before payload construction.
**Phase:** implement. **Claim Source:** executed.

#### Rework Evidence 2 - TP-C3-01..20 and TP-C3-27 Python suite

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 0
**Output:**

```text
+ echo '[py-unit] pip install OK; starting pytest ml/tests'
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 12%]
.......................................s................................ [ 24%]
........................................................................ [ 36%]
........................................................................ [ 48%]
........................................................................ [ 60%]
........................................................................ [ 72%]
........................................................................ [ 84%]
........................................................................ [ 96%]
..................                                                       [100%]
592 passed, 2 skipped in 21.18s
[py-unit] pytest ml/tests finished OK
+ echo '[py-unit] pytest ml/tests finished OK'
```

**Result:** PASS. The final post-lint tree includes all 13 direct builder tests,
the exact AST inventory guard, startup and dynamic zero-I/O failures,
LiteLLM/native merge, both hosted-provider negatives, and BUG-026-006 output
budget assertions. **Phase:** implement. **Claim Source:** executed.

#### Rework Evidence 3 - SCN-102-C3-02/03/04 typed Go and KV contracts

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestSpec102Go|TestValidateModelEnvelopes_(KVMathFailsUnderstated|RefusesUncappedAcceptsCapped|CoResidentSumGatedByMaxLoaded)_Spec102' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestSpec102GoUsesTypedMLBoundary
  client_test.go:217: typed boundary: method=POST path=/llm/chat fields=4
  client_test.go:238: typed boundary: Go received the sidecar response without importing Ollama response semantics
--- PASS: TestSpec102GoUsesTypedMLBoundary (0.02s)
=== RUN   TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly
  client_test.go:319: read-only Ollama probe: internal/assistant/openknowledge/catalog/adapter.go -> GET /api/tags
  client_test.go:319: read-only Ollama probe: internal/api/health.go -> GET /api/tags
  client_test.go:319: read-only Ollama probe: internal/api/model_connections_probe.go -> GET /api/tags
--- PASS: TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly (1.98s)
=== RUN   TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102
--- PASS: TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102
--- PASS: TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102
--- PASS: TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.034s
[go-unit] go test ./... finished OK
```

+ **SCN-102-C3-02 (adversarial):** a model whose weights (6144) fit alone but whose
  weights+KV (@ num_ctx 8192, num_parallel 4 = 8241) exceed the 8192 envelope is REFUSED
  naming `kv-heavy`, `OLLAMA_MEMORY_LIMIT`, `envelope exceeded`, `KV`, `num_ctx=8192`;
  removing the KV term (kv_per_1k=0) makes the SAME weights FIT — proving the KV term (not a
  static ceiling) is what triggers the rejection.
+ **SCN-102-C3-03:** gemma4:26b's uncapped 262144 context (KV ≈ 268 GiB) is refused; the
  SST-capped num_ctx=8192 (resident 24772 < 49152) is accepted.
+ **SCN-102-C3-04:** the same over-sum config (40960 > 28672) is ACCEPTED at
  `max_loaded_models=1` (on-demand swap: per-model fit only) and REFUSED at
  `max_loaded_models=2` (co-resident: working-set sum required) — the posture gate.

TP-C3-22/23 additionally prove that Go emits typed POST `/llm/chat` requests and
that its three direct Ollama owners are GET `/api/tags` only; AST scanning rejects
any production Go `/api/chat` or `/api/generate` literal. **Phase:** implement.
**Claim Source:** executed.

#### Rework Evidence 4 - TP-C3-21 exact full-stack closure

**Executed:** YES (current session)
**Command:** `./smackerel.sh test integration --go-run 'TestSpec102NATSDriverUsesTypedBoundaryWithoutOllamaProtocol'`
**Exit Code:** 0
**Output:**

```text
[go-integration] gettext-base install OK
go-integration: applying -run selector: TestSpec102NATSDriverUsesTypedBoundaryWithoutOllamaProtocol
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.037s [no tests to run]
=== RUN   TestSpec102NATSDriverUsesTypedBoundaryWithoutOllamaProtocol
  loop_test.go:283: typed NATS boundary: subject=agent.invoke.request.spec102_typed_boundary.<unique> fields=12 provider=ollama model=gemma4:26b
--- PASS: TestSpec102NATSDriverUsesTypedBoundaryWithoutOllamaProtocol (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/agent  0.061s
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/integration/annotation     0.010s [no tests to run]
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
```

**Result:** PASS. The planned Go selector executes against the full disposable
stack and real ephemeral NATS; the observed typed request has 12 fields and no
Ollama protocol fields. **Phase:** test.
**Claim Source:** executed.

**Executed:** YES (current session, final test tree)
**Command:** `./smackerel.sh test integration`
**Exit Code:** 0
**Output:**

```text
--- PASS: TestCardRewardsStore_CreateReadUserCard_B01 (0.01s)
=== RUN   TestCardRewardsStore_CreateCustomCard_B04
--- PASS: TestCardRewardsStore_CreateCustomCard_B04 (0.00s)
=== RUN   TestCardRewardsStore_SharedLimitOffer_B05
--- PASS: TestCardRewardsStore_SharedLimitOffer_B05 (0.00s)
=== RUN   TestCardRewardsStore_TieredSelection_B06
--- PASS: TestCardRewardsStore_TieredSelection_B06 (0.01s)
=== RUN   TestCardRewardsStore_CascadeDelete_B07
--- PASS: TestCardRewardsStore_CascadeDelete_B07 (0.01s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     0.318s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
Container smackerel-test-smackerel-core-1 Removed
Container smackerel-test-postgres-1 Removed
Container smackerel-test-smackerel-ml-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
```

**Result:** PASS. This is the literal Test Plan command. The complete Go
integration lane exits 0 and its exit cleanup removes the disposable stack.
**Phase:** test. **Claim Source:** executed.

**Broad-lane defect diagnosis and fix.** After image acquisition was warmed through
the canonical `./smackerel.sh --env test up` / `down --volumes` lifecycle, the first
unfiltered test run reached Go and exited 1. The failing test was
`TestConfigValidate_AC5c_BinaryRejectsOversizedModel`: its synthetic profile still
used legacy `memory_mib`, which spec 102 now parses as `weights_mib=0`, so validation
failed at schema loading instead of the intended envelope assertion. Its sibling
wrapper fixture also changed top-level `llm.model`, which no longer controls the
test environment, and silently skipped on YAML drift. The test-only repair:

+ expresses synthetic profiles as `{weights_mib, kv_mib_per_1k_ctx, num_ctx}`;
+ injects `environments.test.{llm_model,ollama_model}`, the current resolver seam;
+ replaces stale-shape/generated-config skips with fail-loud assertions.
+ makes the shared real-NATS helper fail loud when `NATS_URL` is absent, so
  TP-C3-21 cannot silently pass without the canonical live stack.

Both repaired tests pass together and prove the original adversarial contract:
the 20 GiB fixture is rejected against `OLLAMA_MEMORY_LIMIT=8G` at both binary and
repo-wrapper surfaces. No production/deploy code changed.
**Claim Source:** executed.

#### Rework Evidence 5 - SCN-102-C3-05 BUG-026-006 output budget

`ml/tests/test_processor.py::test_output_budget_read_from_sst_not_hardcoded_spec102` sets
`ML_DOMAIN_OUTPUT_TOKEN_BUDGET=1234` (distinct from 2000) and asserts
`max_tokens==1234` and `!=2000` (a re-hardcoded 2000 flips it red). It passed in
the current 592-test Python run above. BEFORE/AFTER:

```
BEFORE:  ml/app/processor.py:147   "max_tokens": 2000,
         ml/app/synthesis.py:176   token_budget = contract.get("token_budget", 2000)
AFTER:   ml/app/processor.py       "max_tokens": resolve_domain_output_token_budget(),
         ml/app/synthesis.py       token_budget = contract.get("token_budget", resolve_domain_output_token_budget())
```

The budget is SST-owned (`services.ml.domain_output_token_budget` → `ML_DOMAIN_OUTPUT_TOKEN_BUDGET`,
fail-loud, boot-checked by `ml/app/main.py::_check_required_config`, default raised 2000→4096).
BUG-026-006 state advanced with this before/after evidence (`advancedBy` entry); BUG-026-007
carries a complementary advance note (num_ctx SST-driving). Both remain live-verify-gated
(R-102-A/D). **Claim Source:** executed.

#### Rework Evidence 6 - config, check, lint, and feature guards

**Executed:** YES (current session)
**Command:** `./smackerel.sh config generate && ./smackerel.sh check; exit_code=$?; echo "EXIT_CODE=$exit_code"; exit "$exit_code"`
**Exit Code:** 0
**Output:**

```text
config-validate: ~/Projects/smackerel/config/generated/dev.env.tmp.48459 OK
Generated ~/Projects/smackerel/config/generated/dev.env
Generated ~/Projects/smackerel/config/generated/nats.conf
Generated ~/Projects/smackerel/config/generated/prometheus.yml
config-validate: ~/Projects/smackerel/config/generated/dev.env.tmp.54078 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
EXIT_CODE=0
```

**Executed:** YES (current session)
**Command:** `./smackerel.sh lint`
**Exit Code:** 0
**Output:**

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ Found DoD section in scopes.md
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
============================================================
  BUBBLES TRACEABILITY GUARD
  Feature: ~/Projects/smackerel/specs/102-<deploy-host>-deploy-hardening
============================================================
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 34 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
--- Gherkin → DoD Content Fidelity (Gate G068) ---
✅ scopes.md scenario maps to DoD item: SCN-102-C3-02 — the validator uses real KV math (adversarial)
✅ scopes.md scenario maps to DoD item: SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked
ℹ️  DoD fidelity: 8 scenarios checked, 8 mapped to DoD, 0 unmapped
ℹ️  Scenarios checked: 8
ℹ️  Scenario-to-row mappings: 8
ℹ️  Concrete test file references: 8
ℹ️  Report evidence references: 8
RESULT: PASSED (0 warnings)
```

**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
ℹ️  INFO: Scopes yielded 0 files — falling back to design.md for file discovery
⚠️  WARN: Resolved 49 file(s) from design.md fallback — scopes.md should reference these directly
ℹ️  INFO: Resolved 49 implementation file(s) to scan
--- Scan 1: Gateway/Backend Stub Patterns ---
--- Scan 1B: Handler / Endpoint Execution Depth ---
--- Scan 1C: Endpoint Not-Implemented / Placeholder Responses ---
--- Scan 1D: External Integration Authenticity ---
--- Scan 2: Frontend Hardcoded Data Patterns ---
--- Scan 3: Frontend API Call Absence ---
--- Scan 4: Prohibited Simulation Helpers in Production ---
--- Scan 5: Default/Fallback Value Patterns ---
--- Scan 6: Live-System Test Interception ---
  Files scanned:  49
  Violations:     0
  Warnings:       1
🟡 PASSED with 1 warning(s) — manual review advised
```

**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh ml/tests/test_ollama_keepalive.py ml/tests/test_main.py ml/tests/test_chat_dispatch_parity_spec096.py internal/assistant/openknowledge/llm/client_test.go tests/integration/agent/loop_test.go`
**Exit Code:** 0
**Output:**

```text
============================================================
  BUBBLES REGRESSION QUALITY GUARD
  Repo: ~/Projects/smackerel
  Timestamp: 2026-07-10T20:15:45Z
  Bugfix mode: false
============================================================
ℹ️  Scanning ml/tests/test_ollama_keepalive.py
ℹ️  Scanning ml/tests/test_main.py
ℹ️  Scanning ml/tests/test_chat_dispatch_parity_spec096.py
ℹ️  Scanning internal/assistant/openknowledge/llm/client_test.go
ℹ️  Scanning tests/integration/agent/loop_test.go
============================================================
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 5
============================================================
```

**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 1
**Output:**

```text
--- Check 3F: Transition And Rework Packets (Gate G061) ---
✅ PASS: state.json transitionRequests queue is empty
🔴 BLOCK: state.json still contains non-empty reworkQueue entries — open rework remains (Gate G061)
--- Check 4: DoD Completion (Zero Unchecked) ---
ℹ️  INFO: DoD items total: 66 (checked: 62, unchecked: 4)
🔴 BLOCK: Resolved scope artifacts have 4 UNCHECKED DoD items — ALL must be [x] for 'done'
   → scopes.md: - [ ] SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked.
   → scopes.md: - [ ] TP-C3-21 — the Go NATS typed-boundary contract passes.
   → scopes.md: - [ ] Build Quality Gate passes: `./smackerel.sh config generate`, focused Python/Go
   → scopes.md: - [ ] `bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh`
--- Check 16: Implementation Reality Scan (Gate G028) ---
✅ PASS: Implementation reality scan passed — no stub/fake/hardcoded data patterns detected
--- Check 18: Deferral Language Scan (Gate G040) ---
✅ PASS: Zero deferral language found in scope and report artifacts (Gate G040)
--- Check 34: Capability Foundation Enforcement (Gate G094) ---
✅ PASS: Capability foundation requirements are satisfied, not applicable, or grandfathered (Gate G094)
--- Check 35: Discovered-Issue Disposition (Gate G095) ---
✅ PASS: Discovered-issue disposition clean — no unfiled deferrals (Gate G095)
🔴 TRANSITION BLOCKED: 19 failure(s), 2 warning(s)
state.json status MUST NOT be set to 'done'.
```

**Result:** FAIL as expected for a non-terminal feature. No status or
certification field was promoted. **Phase:** implement. **Claim Source:** executed.

**Result:** PASS. `git diff --check` exited clean for the exact SCOPE-102-03
source/test boundary. Artifact lint and traceability guard passed;
implementation reality scanned 49 files with zero violations and one existing
design-fallback discovery warning. **Phase:** implement. **Claim Source:** executed.

**Current rework files:** all 12 inventoried production modules plus
`ml/app/ollama_keepalive.py`; their 12 planned Python test files;
`internal/assistant/openknowledge/llm/client_test.go`;
`tests/integration/agent/loop_test.go`; and the existing
`internal/config/validate_ml_envelope_kv_spec102_test.go`. No excluded source
family, live host, deploy adapter, secret value, or git history was changed.

#### Security Rework Evidence (2026-07-10)

**Finding disposition.** All three routed security findings are addressed in
implementation and regression tests. TP-C3-21 now also has an exact
full-integration-lane verdict; this surgical invocation does not promote the
aggregate SCOPE-102-03 completion items.

| Finding | Disposition | Implemented protection |
| --- | --- | --- |
| SEC-102-RR-01 | Addressed | Go `providerDispatchContracts` and Python `validate_provider_dispatch_controls` admit only provider-owned endpoint/parameter keys; unsupported controls reject before LiteLLM. |
| SEC-102-RR-02 | Addressed | `dispatch_litellm`, `dispatch_ollama_native_json_async`, and `dispatch_ollama_native_json` own production network primitives; Ollama requires a resolved `OllamaRequestProfile`; hosted dispatch passes `profile=None`; alias/discard mutations fail the structural guard. |
| SEC-102-RR-03 | Addressed | `OllamaProfileConfigError` carries category plus structural context only; supplied values are absent from exception text, startup/warmup/chat/agent logs, HTTP details, and agent envelopes. |

**13-builder abstraction accounting.** LiteLLM dispatcher (11):
`agent.handle_invoke`, `card_categories.extract_card_categories`,
`domain._do_domain_extract`, `drive_classify.classify_drive_file`,
`main._warmup_domain_model`, `nats_client._handle_search_rerank`,
`nats_client._handle_digest_generate`, `processor.process_content`,
`routes.chat._dispatch_ollama`, `synthesis.handle_extract`, and
`synthesis.handle_crosssource` (11 LiteLLM builders). Native dispatcher (2):
`intelligence._call_llm` uses the async native adapter and
`ocr.extract_text_ollama` uses the synchronous native adapter. Total: 13/13.
The hosted `_dispatch_hosted` branch shares `dispatch_litellm` with
`profile=None` and never resolves or emits Ollama profile fields.

##### Security RED proofs

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102' --verbose`
**Exit Code:** 1 (before Go allowlist implementation)
**Output:**

```text
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/internal/assistant/intent/policyguard   0.009s [no tests to run]
# github.com/smackerel/smackerel/internal/assistant/openknowledge/llm [github.com/smackerel/smackerel/internal/assistant/openknowledge/llm.test]
internal/assistant/openknowledge/llm/dispatch_resolver_test.go:328:60: undefined: RejectInvalidConnectionParams
internal/assistant/openknowledge/llm/dispatch_resolver_test.go:329:60: undefined: RejectInvalidConnectionParams
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics 0.017s [no tests to run]
FAIL    github.com/smackerel/smackerel/internal/assistant/openknowledge/llm [build failed]
```

**Result:** RED. The typed invalid-connection-param refusal did not exist.
**Phase:** implement. **Claim Source:** executed.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 1 (before Python allowlists)
**Output:**

```text
FAILED ml/tests/test_chat_dispatch_hosted_spec096.py::test_hosted_provider_params_block_injected_controls_before_litellm[api_base]
FAILED ml/tests/test_chat_dispatch_hosted_spec096.py::test_hosted_provider_params_block_injected_controls_before_litellm[options]
FAILED ml/tests/test_chat_dispatch_hosted_spec096.py::test_hosted_provider_params_block_injected_controls_before_litellm[keep_alive]
FAILED ml/tests/test_chat_dispatch_hosted_spec096.py::test_hosted_provider_params_block_injected_controls_before_litellm[extra_headers]
FAILED ml/tests/test_chat_dispatch_hosted_spec096.py::test_hosted_provider_params_block_injected_controls_before_litellm[timeout]
FAILED ml/tests/test_chat_dispatch_hosted_spec096.py::test_anthropic_api_base_is_rejected_before_litellm
=========================== short test summary info ============================
E       Failed: DID NOT RAISE any of (ValidationError, HTTPException)
E       Failed: DID NOT RAISE any of (ValidationError, HTTPException)
E       Failed: DID NOT RAISE any of (ValidationError, HTTPException)
6 failed, 592 passed, 2 skipped in 22.61s
```

**Result:** RED. Arbitrary hosted controls reached the external dispatch path.
**Phase:** implement. **Claim Source:** executed.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 1 (after replacing the line-order guard, before dispatcher migration)
**Output:**

```text
E       AssertionError: direct Ollama network primitive outside foundation: [('agent.py', 'handle_invoke', 'litellm.acompletion', 260),
('agent.py', 'handle_invoke', 'completion_fn', 343),
('card_categories.py', 'extract_card_categories', 'litellm.acompletion', 196),
('domain.py', '_do_domain_extract', 'litellm.acompletion', 158),
('drive_classify.py', 'classify_drive_file', 'litellm.acompletion', 80),
('intelligence.py', '_call_llm', 'post:/api/generate', 57),
('main.py', '_warmup_domain_model', 'litellm.acompletion', 300),
('nats_client.py', '_handle_search_rerank', 'litellm.acompletion', 942),
('nats_client.py', '_handle_digest_generate', 'litellm.acompletion', 1075),
('ocr.py', 'extract_text_ollama', 'post:/api/generate', 115),
('processor.py', 'process_content', 'litellm.acompletion', 179),
('routes/chat.py', '_dispatch_ollama', 'litellm.acompletion', 216),
('routes/chat.py', '_dispatch_hosted', 'litellm.acompletion', 385),
('synthesis.py', 'handle_extract', 'litellm.acompletion', 223),
('synthesis.py', 'handle_crosssource', 'litellm.acompletion', 364)]
3 failed, 597 passed, 2 skipped in 18.48s
```

**Result:** RED. The structural guard found every direct primitive before migration.
**Phase:** implement. **Claim Source:** executed.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 1 (before supplied-value redaction)
**Output:**

```text
E           app.ollama_keepalive.OllamaProfileConfigError: ML_OLLAMA_KEEP_ALIVE must be a positive Ollama duration or integer such as '30m', '1h30m', or '1800'; got 'SENTINEL-AGENT-PROFILE-SECRET-RR03'
FAILED ml/tests/test_agent.py::test_profile_error_response_envelope_redacts_supplied_value_security102
FAILED ml/tests/test_chat_dispatch_parity_spec096.py::test_ollama_profile_error_redacts_supplied_value_from_http_and_logs
FAILED ml/tests/test_main.py::test_startup_profile_error_log_redacts_supplied_value_security102
FAILED ml/tests/test_ollama_keepalive.py::test_profile_errors_redact_supplied_values_security102
Captured log call:
ERROR smackerel-ml.openknowledge.chat open_knowledge live dispatch failed unexpectedly
app.ollama_keepalive.OllamaProfileConfigError: ML_OLLAMA_KEEP_ALIVE must be a positive Ollama duration or integer such as '30m', '1h30m', or '1800'; got 'SENTINEL-CHAT-PROFILE-SECRET-RR03'
Captured log call:
ERROR smackerel-ml Invalid Ollama request profile configuration: ML_MODEL_MEMORY_PROFILES_JSON entry 'llama3.2' num_ctx must be a positive integer, got 'SENTINEL-STARTUP-PROFILE-SECRET-RR03'
4 failed, 601 passed, 2 skipped in 9.55s
```

**Result:** RED. Exception text, logs, and envelopes exposed supplied values.
**Phase:** implement. **Claim Source:** executed.

##### Final Python security and builder suite

**Executed:** YES (current session, after final lint fixes)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 0
**Output:**

```text
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.....................................................s.................. [ 23%]
........................................................................ [ 35%]
........................................................................ [ 46%]
........................................................................ [ 58%]
........................................................................ [ 70%]
........................................................................ [ 82%]
........................................................................ [ 93%]
......................................                                   [100%]
612 passed, 2 skipped in 15.09s
[py-unit] pytest ml/tests finished OK
```

**Result:** PASS. This includes provider allowlists, 13/13 builder payloads,
structural direct-network rejection, alias/discard mutations, mandatory resolved
profiles, hosted bypass, sentinel redaction, tools/options/think/determinism/budget
regressions, and all other Python unit tests. **Phase:** implement.
**Claim Source:** executed.

##### Focused Go security, boundary, and KV suite

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102|TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102|TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096|TestDispatch_SecretNeverInLogsOrErrors_Spec096|TestSpec102Go|TestValidateModelEnvelopes_(KVMathFailsUnderstated|RefusesUncappedAcceptsCapped|CoResidentSumGatedByMaxLoaded)_Spec102' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102/legitimate_openai_params_survive
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102/reject_injected_api_base
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102/reject_injected_options
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102/reject_injected_keep_alive
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102/reject_injected_extra_headers
=== RUN   TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102/reject_injected_timeout
--- PASS: TestDispatchResolver_ProviderParamsAreClosedPerProvider_Security102 (0.00s)
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102/ollama_base_url
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102/anthropic_has_no_non_secret_routing_params
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102/openai_base_url_and_org
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102/azure_endpoint_version_and_deployment
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102/google_vertex_project_and_location
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102/bedrock_region
--- PASS: TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102 (0.00s)
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096
--- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096 (0.00s)
=== RUN   TestDispatch_SecretNeverInLogsOrErrors_Spec096
--- PASS: TestDispatch_SecretNeverInLogsOrErrors_Spec096 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/llm    2.868s
=== RUN   TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102
--- PASS: TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102
--- PASS: TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102
--- PASS: TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.105s
```

**Result:** PASS. Legitimate Ollama, Anthropic, OpenAI, Azure Foundry, Google,
and Bedrock controls survive; five injected controls reject without value
leakage; existing no-fallback, credential safety, typed sidecar, tags-only, and
KV contracts remain green. **Phase:** implement.
**Claim Source:** executed.

##### Format, check, lint, and compute-only gates

**Executed:** YES (current session)
**Command:** `./smackerel.sh format --check`
**Exit Code:** 0
**Output:**

```text
Building editable for smackerel-ml (pyproject.toml): started
Building editable for smackerel-ml (pyproject.toml): finished with status 'done'
Successfully built smackerel-ml
Installing collected packages: websockets, uvloop, typing-extensions, ruff, rpds-py,
pyyaml, python-dotenv, pypdf, pygments, prometheus-client, pluggy, packaging,
nats-py, iniconfig, idna, httptools, h11, click, certifi, attrs, annotated-types,
annotated-doc, uvicorn, typing-inspection, referencing, pytest, pydantic-core,
httpcore, anyio, watchfiles, starlette, pydantic, jsonschema-specifications,
httpx, pydantic-settings, jsonschema, fastapi, smackerel-ml
Successfully installed smackerel-ml-0.1.0
74 files already formatted
```

**Result:** PASS. The initial run named only
`internal/deploy/alertmanager_bundle_contract_test.go`; `./smackerel.sh format`
repaired the Go formatting and Ruff reported nine Python files reformatted in
the existing dirty tree. The final standalone check is clean.
**Phase:** implement. **Claim Source:** executed.

**Executed:** YES (current session)
**Command:** `./smackerel.sh format --check && ./smackerel.sh check && ./smackerel.sh lint`
**Exit Code:** 0
**Output:**

```text
74 files already formatted
config-validate: config/generated/dev.env.tmp.21556 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

**Result:** PASS. **Phase:** implement. **Claim Source:** executed.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test pre-push`
**Exit Code:** 0
**Output:**

```text
Running macOS/WSL portability guard (host-run operator surface: smackerel.sh + scripts/)...
== macOS portability guard -- scanning 42 file(s) ==
ok   class-1 raw-timeout: none
ok   class-2 in-place-sed: none
ok   class-3 date-d-parse: none
ok   class-4 stat-c-mtime: none
ok   class-5 readlink-f-absolutize: none
ok   class-6 grep-pcre: none
ok   class-7 bracket-v-isset: none
ok   class-8 mapfile-readarray: none
ok   class-9 mktemp-suffix: none
ok   class-10 df-output: none
ok   class-11 bin-true-false: none
ok   class-12 paste-no-stdin-operand: none
ok   class-13 date-nanoseconds: none
PASS: the scanned surface is WSL+macOS portable.
macOS/WSL portability guard: OK
Running python-compute-only guard (smackerel-ml compute-only invariant)...
python-compute-only-guard: OK - dependency scan: no forbidden datastore driver in 2 dependency file(s) under ml (nats-py is the sanctioned transport)
python-compute-only-guard: OK - infra-URL scan: no direct DATABASE_URL/POSTGRES_URL/REDIS_URL/RABBITMQ_URL read in *.py (NATS_URL is the sanctioned wire)
python-compute-only-guard: OK - env-delivery shape: smackerel-ml loads the projected ./ml.env (not ./app.env); services.ml.env_allowlist is declared
python-compute-only-guard: clean - smackerel Python (ml) is compute-only: no datastore driver, no direct datastore-URL read, secret-free projected env delivery.
python-compute-only guard: OK
```

**Result:** PASS. **Phase:** implement. **Claim Source:** executed.

##### Feature guards

**Executed:** YES (current session)
**Commands:** artifact lint, traceability guard, implementation reality scan,
and regression-quality guard over the 18 security/builder/boundary/KV test files.
**Exit Codes:** 0 / 0 / 0 / 0
**Output:**

```text
Artifact lint PASSED.
✅ scenario-manifest.json covers 34 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
ℹ️  Scenarios checked: 8
ℹ️  Scenario-to-row mappings: 8
ℹ️  Concrete test file references: 8
ℹ️  Report evidence references: 8
RESULT: PASSED (0 warnings)
IMPLEMENTATION REALITY SCAN RESULT
Files scanned: 49
Violations: 0
Warnings: 1
PASSED with 1 warning(s) - manual review advised
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 18
```

**Result:** PASS. The implementation-reality warning is the existing
design.md-fallback file discovery warning; it reports zero implementation
violations. **Phase:** implement. **Claim Source:** executed.

**User validation status.** No item in `uservalidation.md` was changed. Its
new-acceptance boxes are intentionally unchecked and owned by planning/human
validation; this implementation run does not self-certify them.

#### Current Security Finding Closure (2026-07-11)

The two current SCOPE-102-03 security findings are closed in implementation and
adversarial regression tests. This section records execution evidence only; it
does not certify the scope or promote feature status.
**Phase:** implement. **Claim Source:** executed.

| Finding | Disposition | Protection |
| --- | --- | --- |
| OCR profile failures entered the transport fallback, cached Tesseract output, and returned `status=ok` | Addressed | `extract_text_ollama` re-raises `OllamaProfileConfigError`; `handle_ocr_request` returns a category-only error before network/cache. Missing, unprofiled, and invalid profiles assert zero network, zero cache, non-ok status, and sentinel absence. Ordinary transport failure still falls back to and caches Tesseract. |
| Structural guard missed module aliases, bound methods, extra builders, and mixed nominal/direct dispatch | Addressed | The AST guard tracks symbol provenance through module/function lexical scopes, imports, assignments, HTTP client construction, bound methods, and endpoint aliases. The discovered profiled-builder key set must equal `_EXPECTED_PROFILED_BUILDERS` exactly. Four requested mutations plus the existing alias/discard mutations are rejected. |

##### RED Proof

**Executed:** YES (in current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 1
**Output:**

```text
WARNING  smackerel-ml.ocr:ocr.py:123 Ollama vision OCR failed: ML_MODEL_MEMORY_PROFILES_JSON key=document expected=required non-empty profile list category=missing_document
WARNING  smackerel-ml.ocr:ocr.py:123 Ollama vision OCR failed: ML_MODEL_MEMORY_PROFILES_JSON key=model model=<redacted> expected=exactly one matching profile entry category=missing_model_profile
WARNING  smackerel-ml.ocr:ocr.py:123 Ollama vision OCR failed: ML_MODEL_MEMORY_PROFILES_JSON entry=0 key=num_ctx model=<redacted> expected=positive integer category=invalid_num_ctx
FAILED ml/tests/test_ocr.py::TestExtractTextOllama::test_profile_config_failure_is_category_only_and_never_cached_security102[missing-missing_document]
FAILED ml/tests/test_ocr.py::TestExtractTextOllama::test_profile_config_failure_is_category_only_and_never_cached_security102[unprofiled-missing_model_profile]
FAILED ml/tests/test_ocr.py::TestExtractTextOllama::test_profile_config_failure_is_category_only_and_never_cached_security102[invalid-invalid_num_ctx]
FAILED ml/tests/test_ollama_keepalive.py::test_structural_guard_rejects_module_level_imported_alias_spec102
FAILED ml/tests/test_ollama_keepalive.py::test_structural_guard_rejects_bound_native_post_alias_spec102
FAILED ml/tests/test_ollama_keepalive.py::test_structural_guard_rejects_extra_dispatch_builder_spec102
FAILED ml/tests/test_ollama_keepalive.py::test_structural_guard_rejects_nominal_dispatch_plus_direct_alias_spec102
=========================== short test summary info ============================
7 failed, 613 passed, 2 skipped in 6.69s
```

**Result:** FAIL as expected. All seven failures map to the two requested
findings; no unrelated Python test failed. **Phase:** implement.
**Claim Source:** executed.

##### Final Python Unit Suite

**Executed:** YES (in current session, formatted final tree)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 0
**Output:**

```text
+ echo '[py-unit] pip install OK; starting pytest ml/tests'
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.....................................................s.................. [ 23%]
........................................................................ [ 34%]
........................................................................ [ 46%]
........................................................................ [ 57%]
........................................................................ [ 69%]
........................................................................ [ 81%]
........................................................................ [ 92%]
..............................................                           [100%]
620 passed, 2 skipped in 6.08s
[py-unit] pytest ml/tests finished OK
```

**Result:** PASS. This includes the three OCR profile variants, transport
fallback control, exact 13-builder inventory, all requested alias/builder
mutations, prior direct/discard mutations, and the full existing Python unit
suite. **Phase:** implement. **Claim Source:** executed.

##### Focused Go, KV, And Provider Contracts

**Executed:** YES (in current session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestSpec102Go|TestValidateModelEnvelopes_(KVMathFailsUnderstated|RefusesUncappedAcceptsCapped|CoResidentSumGatedByMaxLoaded)_Spec102|ProviderDispatch' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestSpec102GoUsesTypedMLBoundary
  client_test.go:217: typed boundary: method=POST path=/llm/chat fields=4
  client_test.go:238: typed boundary: Go received the sidecar response without importing Ollama response semantics
--- PASS: TestSpec102GoUsesTypedMLBoundary (0.02s)
=== RUN   TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly
  client_test.go:319: read-only Ollama probe: internal/api/health.go -> GET /api/tags
  client_test.go:319: read-only Ollama probe: internal/api/model_connections_probe.go -> GET /api/tags
  client_test.go:319: read-only Ollama probe: internal/assistant/openknowledge/catalog/adapter.go -> GET /api/tags
--- PASS: TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly (1.59s)
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102
--- PASS: TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102 (0.00s)
=== RUN   TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102
--- PASS: TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102
--- PASS: TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102
--- PASS: TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102 (0.00s)
PASS
[go-unit] go test ./... finished OK
```

**Result:** PASS. Typed boundaries, read-only Ollama probes, all six legitimate
provider-control contracts, and the three KV-envelope regressions remain green.
**Phase:** implement. **Claim Source:** executed.

##### Build Quality And Feature Gates

**Executed:** YES (in current session)
**Commands:** `./smackerel.sh format --check`; `./smackerel.sh config generate`;
`./smackerel.sh check`; `./smackerel.sh lint`; `./smackerel.sh test pre-push`;
artifact lint; traceability guard; implementation-reality scan; regression-quality
guard over `ml/tests/test_ocr.py` and `ml/tests/test_ollama_keepalive.py`.
**Exit Codes:** 0 / 0 / 0 / 0 / 0 / 0 / 0 / 0 / 0
**Output:**

```text
74 files already formatted
config-validate: ~/Projects/smackerel/config/generated/dev.env.tmp.17303 OK
Generated ~/Projects/smackerel/config/generated/dev.env
Generated ~/Projects/smackerel/config/generated/nats.conf
Generated ~/Projects/smackerel/config/generated/prometheus.yml
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
Web validation passed
PASS: the scanned surface is WSL+macOS portable.
python-compute-only-guard: clean - smackerel Python (ml) is compute-only: no datastore driver, no direct datastore-URL read, secret-free projected env delivery.
Artifact lint PASSED.
RESULT: PASSED (0 warnings)
Files scanned:  49
Violations:     0
Warnings:       1
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 2
```

**Result:** PASS. The implementation-reality scan's one advisory is file
discovery (`scopes.md` yielded no machine-resolved paths, so it scanned 49 paths
from `design.md`); it reports zero implementation violations. `git diff --check`
also exits 0. **Phase:** implement. **Claim Source:** executed.

##### Current-Session OCR Reverification (2026-07-10)

This is evidence-only revalidation of the existing SCOPE-102-03 OCR security
change. It does not change scope status, certification, deployment state, or git
history. The canonical Python lane collects the three parametrized profile-error
cases and the ordinary transport-fallback control in `ml/tests/test_ocr.py`.

**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --python`
**Exit Code:** 0
**Output:**

```text
+ cd /workspace
+ echo '[py-unit] starting pip install -e ./ml[dev]'
[py-unit] starting pip install -e ./ml[dev]
+ PIP_DISABLE_PIP_VERSION_CHECK=1
+ PIP_ROOT_USER_ACTION=ignore
+ python -m pip install --no-cache-dir -e './ml[dev]'
Successfully built smackerel-ml
+ echo '[py-unit] pip install OK; starting pytest ml/tests'
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.....................................................s.................. [ 23%]
........................................................................ [ 34%]
........................................................................ [ 46%]
........................................................................ [ 57%]
........................................................................ [ 69%]
........................................................................ [ 81%]
........................................................................ [ 92%]
..............................................                           [100%]
620 passed, 2 skipped in 14.08s
+ echo '[py-unit] pytest ml/tests finished OK'
[py-unit] pytest ml/tests finished OK
```

**Result:** PASS. The executed lane covers the OCR assertions for zero Ollama
network calls before profile validation, zero cache writes, a non-`ok`
category-only response, sentinel absence from exception/log/response surfaces,
and preservation of the ordinary transport-failure Tesseract fallback/cache path.
**Phase:** implement. **Claim Source:** executed for the suite verdict;
interpreted for the mapping from the collected test file to those assertions.

**Uncertainty Declaration:** `./smackerel.sh test unit --python` has no Python
file/name selector. A host-side `PYTEST_ADDOPTS=-k ...` attempt was not forwarded
through `run_python_tooling` into Docker and therefore reran the same full suite
(`620 passed, 2 skipped`); the VS Code test adapter reported `No tests found` for
the file. No separate focused-pass result is claimed. The full canonical lane is
the executed OCR proof.

**Executed:** YES (current session)
**Commands:** `./smackerel.sh format --check`; `./smackerel.sh check`;
`./smackerel.sh lint`
**Exit Codes:** 0 / 0 / 0
**Output:**

```text
$ ./smackerel.sh format --check
74 files already formatted
$ ./smackerel.sh check
config-validate: ~/Projects/smackerel/config/generated/dev.env.tmp.99455 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
$ ./smackerel.sh lint
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

**Result:** PASS. Formatting, SST/config and scenario consistency, Go/Python
lint, and web validation are clean on the same tree. The home path in the config
check line is normalized to `~/` per evidence PII policy. **Phase:** implement.
**Claim Source:** executed.

**Finding closure:** addressed findings = OCR fail-loud/profile-envelope;
scope-aware exact structural guard. Unresolved findings from this assignment =
none. SCOPE-102-03 is implementation-complete and ready for `bubbles.validate`,
but remains In Progress because this invocation does not certify. No deploy,
commit, push, live-host mutation, or certification occurred.

### SCOPE-102-04 — Backup-adapter durability formalization

**Status:** DONE (2026-07-09). All DoD items backed by real executed evidence below.

**Resolved architecture (documented for bubbles.validate).** The F-1/F-2/F-3 backup
durability code already landed in `<deployment-owner>/<product>/<target>/backup.sh` (spec-048/OPS-RDY-04)
and `<deployment-owner>/<product>/<target>/apply.sh` (F-2 manifest chown-back); this scope is a _test-only_
formalization that locks that behavior behind adversarial regression tests plus one
cross-repo schema-parity assertion. All tests are hermetic — stubbed `docker`/`restic`/`du`
on a sandbox PATH and `SMACKEREL_*` seams; no `/srv/backups`, no real host volume, no
network (env-pollution-isolation clean). Fixing the SCOPE-102-02 alertmanager externalImage
also required mirroring it into the knb `smackerel/contract.yaml` (drift closed — evidence 5).

Cross-repo: knb self-hosted adapter (tests + contract mirror) + smackerel product
(`internal/backup/status_test.go` schema-parity assertion).

#### Evidence 1 — SCN-102-C4-01/02: degraded ≠ success, root-manifest non-fatal (real backup.sh run)

`tests/unit/backup_degraded_contract_test.sh` EXECUTES `backup.sh` against hermetic stubs:

```
$ bash <deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh
PASS: scenario A: hook exits 0 (degraded ships what it captured)
PASS: scenario A: emits META: status=warning (engine downgrades outcome to warning)
PASS: scenario A: postgres_captured=false + nats_captured=false recorded
PASS: scenario A: stale postgres-*.sql.gz rotated out of dump dir
PASS: scenario A: stale nats-data rotated out of dump dir
PASS: scenario A: live deploy pointer captured into dump set (OPS-RDY-04)
PASS: scenario B: NO status=warning on a healthy run (degrade is conditional, not always-on)
PASS: scenario B: pg + NATS + manifest all captured; dump dir holds fresh artifacts
==> SCN-102-C4-01 scenario C: NATS present + docker-mediated capture FAILS => DEGRADED
[WARN] NATS volume 'smackerel-self-hosted-nats-data' present but docker-mediated capture FAILED — JetStream state NOT captured this run (rotated out partial nats-data; run DEGRADED)
PASS: scenario C (F-1/SCN-102-C4-01): NATS present + capture FAILS => nats_captured=false + status=warning; partial nats-data rotated out; NOT a clean success
PASS: scenario D (F-2/SCN-102-C4-02): unreadable manifest => manifest_captured=false WITHOUT status=warning; run stays a CLEAN success on its critical captures
backup_degraded_contract_test.sh OK
```

+ **Scenario C (F-1, adversarial):** the NATS volume is PRESENT but the docker-mediated
  JetStream tar capture returns non-zero. The run records `nats_captured=false` AND emits
  `status=warning` (degraded), rotating out the partial `nats-data` — it is NOT allowed to
  masquerade as a clean success. A regression that swallowed the capture failure and shipped
  `status=success` would flip this assertion.
+ **Scenario D (F-2, adversarial):** a chmod-000 (unreadable) manifest under a non-root run
  yields `manifest_captured=false` WITHOUT `status=warning` — the manifest pointer is a
  _nice-to-have_ snapshot, so its absence must NOT downgrade a run whose critical (pg) captures
  succeeded. A regression that treated the manifest as critical would raise a false warning here.
+ Scenario B was also repaired for macOS (its docker `run` stub now emulates the JetStream tar
  copy: `run) exec tar -C "$NATS_SRC" -cf - . ;;`) — the pre-existing BSD-tar gap that made
  scenario B silently skip is closed.

#### Evidence 2 — SCN-102-C4-03: status advances on success, HOLDS on failure (real backup.sh run)

`tests/unit/backup_status_advance_test.sh` (NEW) drives `backup.sh` with the
`SMACKEREL_BACKUP_STATUS_FILE` seam across two runs, seeding a known-old prior success:

```
$ bash <deployment-owner>/<product>/<target>/tests/unit/backup_status_advance_test.sh
--- status after run 1 ---
{
  "schema_version": 1,
  "last_run_unixtime": 1783669740,
  "last_success_unixtime": 1783669740,
  "last_status": "success",
  ...
}
PASS: run 1 (F-3): postgres captured => last_status=success, last_success_unixtime advanced 1 -> 1783669740, schema_version=1
--- status after run 2 ---
{
  ...
  "last_success_unixtime": 1783669740,
  "last_status": "failed",
  ...
}
PASS: run 2 (F-3/SCN-102-C4-03): postgres skipped => last_status=failed, last_success_unixtime HELD at 1783669740 (freshness gauge NOT advanced by a failed backup)
backup_status_advance_test.sh OK
```

+ **Run 1 (success):** postgres captured ⇒ `last_status=success` and `last_success_unixtime`
  ADVANCES from the seeded `1` to the run time (`1783669740`).
+ **Run 2 (failure, adversarial):** postgres container DOWN ⇒ `last_status=failed` and
  `last_success_unixtime` HOLDS its prior value (`1783669740`). This is the exact
  false-confidence guard: a failed backup that advanced the freshness gauge would silently
  reset `SmackerelBackupStale` — the test FAILS if run 2's `last_success` moves.

#### Evidence 3 — SCN-102-C4-02: apply.sh manifest chown-back locked (source contract + adversarial fixture)

`tests/unit/apply_manifest_contract_test.sh` (extended) asserts the `$SUDO_USER`-gated
chown-back + chmod 0644 is present, correctly ordered after the pointer commit, and
required (adversarial fixture #2 strips it and proves the contract catches the regression):

```
$ bash <deployment-owner>/<product>/<target>/tests/unit/apply_manifest_contract_test.sh
PASS: apply.sh contains: if [[ -n "${SUDO_USER:-}" ]]; then
PASS: apply.sh contains: chown "$SUDO_USER:$SUDO_USER" "$MANIFEST" 2>/dev/null || true
PASS: apply.sh contains: chmod 0644 "$MANIFEST"
PASS: order: 'mv -f "$NEW_MANIFEST" "$MANIFEST"' before 'chown "$SUDO_USER:$SUDO_USER" "$MANIFEST" 2>/dev/null || true'
PASS: order: 'chown "$SUDO_USER:$SUDO_USER" "$MANIFEST" 2>/dev/null || true' before 'chmod 0644 "$MANIFEST"'
PASS: adversarial fixture proves manifest chown-back (F-2) is required
apply_manifest_contract_test.sh OK
```

Rationale (from apply.sh F-2 block): apply.sh runs under sudo, so the freshly-committed
`$MANIFEST` is root:root 0600; the non-root <operator> upkeep backup hook copies THIS pointer into
its dump set (OPS-RDY-04) and, under `set -e`, HARD-FAILS every scheduled backup if it cannot
read it. The manifest holds no secrets, so chown it back to the invoking operator + 0644.

#### Evidence 4 — SCN-102-C4-03: cross-repo schema_version 1 field-set parity (product internal/backup)

`internal/backup/status_test.go` (extended) embeds a verbatim replica of the knb
`backup.sh::write_backup_status` heredoc and asserts it parses via `LoadStatus` at
`CurrentSchemaVersion`, and that the writer's key SET equals the `backup.Status` json-tag set:

```
$ ./smackerel.sh test unit --go --go-run 'TestLoadStatus|TestWatcher|TestRetention|TestMarshalStatus' --verbose
--- PASS: TestRetentionPolicy_Validate (0.00s)
--- PASS: TestLoadStatus_RoundTrip (0.00s)
--- PASS: TestLoadStatus_MissingFile (0.00s)
--- PASS: TestLoadStatus_RejectsZeroSchemaVersion (0.00s)
--- PASS: TestLoadStatus_RejectsUnknownStatus (0.00s)
--- PASS: TestLoadStatus_RejectsSecretSubstrings (0.00s)
--- PASS: TestWatcher_PollIdempotentAndMonotonic (0.01s)
--- PASS: TestWatcher_PollMissingFile (0.00s)
--- PASS: TestWatcher_NilSinkPanics (0.00s)
--- PASS: TestLoadStatus_KnbAdapterSchemaParity (0.00s)
ok      github.com/smackerel/smackerel/internal/backup  0.006s
```

`TestLoadStatus_KnbAdapterSchemaParity` is the new drift-lock: if either repo changes the
on-disk shape (knb heredoc adds/drops a key, or `backup.Status` renames/drops a json tag)
the key-set comparison fails until both sides are reconciled.

#### Evidence 5 — no regression: contract-mirror drift closed + full knb shell-unit tier

The SCOPE-102-02 alertmanager externalImage had to be mirrored into the knb
`smackerel/contract.yaml` (the adapter-side mirror of the upstream deploy contract). Before
the mirror edit `contract_drift_test.sh` correctly FAILED (`missing-from-mirror:
externalImages.alertmanager.*`); after mirroring it passes. Full 22-file shell-unit tier:

```
$ for t in <deployment-owner>/<product>/<target>/tests/unit/*_test.sh; do bash "$t" ...; done
OK   rc=0  adapter_inventory_test.sh
OK   rc=0  apply_audit_schema_test.sh
OK   rc=0  apply_manifest_contract_test.sh
OK   rc=0  backup_degraded_contract_test.sh
OK   rc=0  backup_status_advance_test.sh
OK   rc=0  backup_timer_contract_test.sh
OK   rc=0  contract_drift_test.sh
OK   rc=0  edge_monitoring_contract_test.sh
OK   rc=0  effective_env_decrypt_convergence_test.sh
OK   rc=0  forbidden_patterns_test.sh
OK   rc=0  operator_home_resolver_test.sh
OK   rc=0  operator_home_wiring_test.sh
OK   rc=0  preconditions_contract_test.sh
OK   rc=0  preconditions_local_operator_contract_test.sh
OK   rc=0  readme_contract_test.sh
OK   rc=0  rollback_manifest_contract_test.sh
OK   rc=0  same_release_noop_host_env_reconcile_contract_test.sh
OK   rc=0  secrets_contract_test.sh
OK   rc=0  telegram_user_mapping_emission_contract_test.sh
OK   rc=0  trivy_predicate_alignment_test.sh
OK   rc=0  verify_caddy_topology_contract_test.sh
FAIL rc=1  verify_digest_contract_test.sh
```

**Pre-existing environmental failure (NOT a spec-102 regression, honestly recorded):**
`verify_digest_contract_test.sh` fails at the CORE HTTP HEALTH CHECK stage
(`ERROR: core health check failed at http://127.0.0.1:41001/api/health?strict=true`) —
`verify.sh` performs a REAL localhost health probe that is not served in the local macOS
test env, so it exits BEFORE reaching the digest-drift logic the test asserts on. `verify.sh`
and its `scope04-fixtures.bash` helper are UNMODIFIED by this feature (`git status` confirms
neither is in the change set), and this test belongs to spec-011 SR-S-004, not spec 102. It is
the same class of pre-existing macOS/BSD environmental gap as `shred-and-remove.bats` test 7
(BSD `dd`), which makes `run-tests.sh` abort under `set -e` before the shell-unit tier — hence
the shell-unit tests are run directly (documented pattern, also used for SCOPE-102-02). Both
belong to bubbles.validate / a Linux CI run for green confirmation; neither is introduced or
worsened by spec 102.

**Files changed (SCOPE-102-04):**
knb `<deployment-owner>/<product>/<target>/tests/unit/backup_degraded_contract_test.sh` (scenarios B fix + C/D),
knb `<deployment-owner>/<product>/<target>/tests/unit/backup_status_advance_test.sh` (NEW),
knb `<deployment-owner>/<product>/<target>/tests/unit/apply_manifest_contract_test.sh` (chown-back assertions + adversarial fixture #2),
knb `smackerel/contract.yaml` (alertmanager externalImage mirror — SCOPE-102-02 drift close),
product `internal/backup/status_test.go` (cross-repo schema-parity test + `encoding/json` import).

### Code Diff Evidence

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `git status --short -- specs/102-<deploy-host>-deploy-hardening scripts/commands/config.sh internal/deploy deploy/compose.deploy.yml cmd/alertmanager-ntfy-bridge ml/app/ollama_keepalive.py ml/tests/test_ollama_keepalive.py ml/tests/test_processor.py && git -C ../knb status --short -- <deployment-owner>/<product>/<target>/apply.sh <deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh`
**Exit Code:** 0
**Output:**

```bash
git status --short -- specs/102-<deploy-host>-deploy-hardening scripts/commands/config.sh internal/deploy deploy/compose.deploy.yml cmd/alertmanager-ntfy-bridge ml/app/ollama_keepalive.py ml/tests/test_ollama_keepalive.py ml/tests/test_processor.py && git -C ../knb status --short -- <deployment-owner>/<product>/<target>/apply.sh <deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh
```

```text
 M deploy/compose.deploy.yml
 M internal/deploy/bundle_secret_contract_test.go
 M internal/deploy/compose_contract_test.go
 M internal/deploy/external_images_contract_test.go
 M ml/app/ollama_keepalive.py
 M ml/tests/test_ollama_keepalive.py
 M ml/tests/test_processor.py
 M scripts/commands/config.sh
?? cmd/alertmanager-ntfy-bridge/
?? internal/deploy/alertmanager_bundle_contract_test.go
?? specs/102-<deploy-host>-deploy-hardening/
 M <deployment-owner>/<product>/<target>/apply.sh
?? <deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh
```

**Result:** PASS. The inventory is scoped to the two repos and remediation surfaces; the
preceding `git diff --check` run also exited cleanly for the same explicit file set.

## Security Remediation Evidence (2026-07-10)

This section records the current-session closure proof for security findings F1-F3 from the
post-implementation security review. It does not certify the spec or change the top-level
state. **Phase:** implement. **Claim Source:** executed.

### F1 — Future Same-Prefix Credentials Fail Loud

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Commands:** `cd ~/Projects/smackerel && ./smackerel.sh test unit --go --go-run 'TestMLEnv_CredentialSuffixRequiresSanction_Spec102' --verbose` (before and after)
**Exit Codes:** 1 (RED before guard), 0 (GREEN after final case-insensitive guard)
**Output:**

```text
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/future_same-prefix_credential_is_rejected
  bundle_secret_contract_test.go:1007: SECURITY F1 REGRESSION: generator accepted OLLAMA_CLOUD_TOKEN through the OLLAMA_* projection prefix; future unregistered credentials must fail loud
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/sanctioned_LLM_API_key_remains_projected
--- FAIL: TestMLEnv_CredentialSuffixRequiresSanction_Spec102 (6.57s)
  --- FAIL: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/future_same-prefix_credential_is_rejected (4.77s)
  --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/sanctioned_LLM_API_key_remains_projected (1.80s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  6.606s
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/uppercase_suffix
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/mixed-case_suffix
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102/sanctioned_LLM_API_key_remains_projected
--- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102 (8.10s)
  --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/uppercase_suffix (5.00s)
  --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/mixed-case_suffix (1.44s)
  --- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102/sanctioned_LLM_API_key_remains_projected (1.66s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  8.124s
[go-unit] go test ./... finished OK
```

**Result:** PASS. Projected keys matching the routed case-insensitive credential suffix
family now fail loud unless explicitly sanctioned. `LLM_API_KEY` and the exact,
startup-required inter-service `SMACKEREL_AUTH_TOKEN` are the reviewed compute credentials.

### F2 — Monitoring Ingress Isolated From ML

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Commands:** `./smackerel.sh test unit --go --go-run 'TestCompose_MonitoringIngressIsolatedFromML_Spec102' --verbose` (before and after)
**Exit Codes:** 1 (RED before `monitoring-tier`), 0 (GREEN after topology change)
**Output:**

```text
=== RUN   TestCompose_MonitoringIngressIsolatedFromML_Spec102
  alertmanager_bundle_contract_test.go:295: SECURITY F2: live deploy compose: contract violation: root networks has no monitoring-tier — unauthenticated monitoring ingress cannot be isolated from the ML compute tier
--- FAIL: TestCompose_MonitoringIngressIsolatedFromML_Spec102 (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.025s
=== RUN   TestCompose_MonitoringIngressIsolatedFromML_Spec102
  alertmanager_bundle_contract_test.go:325: adversarial OK — shared ML/monitoring ingress tier rejected: contract violation: root networks has no monitoring-tier — unauthenticated monitoring ingress cannot be isolated from the ML compute tier
--- PASS: TestCompose_MonitoringIngressIsolatedFromML_Spec102 (14.50s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  14.548s
[go-unit] go test ./... finished OK
```

**Result:** PASS. ML remains compute-only; Alertmanager and the unauthenticated bridge are
monitoring-only; Prometheus alone spans compute and monitoring to preserve scrape and alert
delivery paths. The assertion passes against source and a freshly generated bundle.

### F3 — Operator-Private URL Is Never Printed

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Commands:** `cd ~/Projects/knb && bash <deployment-owner>/<product>/<target>/tests/unit/alertmanager_ntfy_output_hygiene_test.sh` (before and after)
**Exit Codes:** 1 (RED before presence-only logging), 0 (GREEN after)
**Output:**

```text
[1/5] apply.sh exists
[2/5] adversarial value-bearing status output is rejected
[3/5] presence-only status output is accepted
[4/5] live adapter status output is value-safe
FAIL: ALERTMANAGER_NTFY_URL write confirmation expands the operator-private URL
[1/5] apply.sh exists
[2/5] adversarial value-bearing status output is rejected
[3/5] presence-only status output is accepted
[4/5] live adapter status output is value-safe
[5/5] contract complete
PASS: apply.sh confirms ALERTMANAGER_NTFY_URL emission without printing its value
```

**Result:** PASS. Resolution remains fail loud and the protected `app.env` write remains
unchanged; stdout now confirms only that the value was written and redacted.

### Complete Spec-102 Go Regression

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `cd ~/Projects/smackerel && ./smackerel.sh test unit --go --go-run 'Spec102' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestBundle_CarriesAlertingBlockAndService_Spec102
--- PASS: TestBundle_CarriesAlertingBlockAndService_Spec102 (27.62s)
=== RUN   TestCompose_MonitoringIngressIsolatedFromML_Spec102
--- PASS: TestCompose_MonitoringIngressIsolatedFromML_Spec102 (2.99s)
=== RUN   TestBundle_AlertingSurvivesReExtract_Spec102
--- PASS: TestBundle_AlertingSurvivesReExtract_Spec102 (5.40s)
=== RUN   TestMLEnv_CredentialSuffixRequiresSanction_Spec102
--- PASS: TestMLEnv_CredentialSuffixRequiresSanction_Spec102 (3.72s)
=== RUN   TestNetworkSegmentation_MLCannotReachPostgres_Spec102
--- PASS: TestNetworkSegmentation_MLCannotReachPostgres_Spec102 (0.00s)
=== RUN   TestNetworkSegmentation_AdversarialMLOnDataTier_Spec102
--- PASS: TestNetworkSegmentation_AdversarialMLOnDataTier_Spec102 (0.00s)
=== RUN   TestNetworkSegmentation_AdversarialPostgresOnComputeTier_Spec102
--- PASS: TestNetworkSegmentation_AdversarialPostgresOnComputeTier_Spec102 (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  50.900s
[go-unit] go test ./... finished OK
```

**Result:** PASS. The complete current Spec-102 Go selector exits cleanly after all three
security changes.

### Current knb Suite Status

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `cd ~/Projects/knb && bash <deployment-owner>/<product>/<target>/tests/run-tests.sh`
**Exit Code:** 1
**Output:**

```text
==> bats unit suite
ciphertext-only-content.bats
 ✓ ciphertext-only: file content is non-empty
 ✓ ciphertext-only: content contains at least one ENC[AES256_GCM,...] block
 ✓ ciphertext-only: sops MAC metadata present
 ✓ ciphertext-only: sops recipient metadata present
 ✓ ciphertext-only: zero literal KEY=value plaintext lines (NFR-001)
 ✓ ciphertext-only: source label captured for evidence
   inspected source: HEAD
shred-and-remove.bats
 ✗ shred_and_remove removes files with shred and with dd fallback
   `[[ "$(cat "$DD_LOG")" == *"dd-called"* ]]' failed
   with_shred_file_exists=no
   fallback_file_exists=no
   shred_log=shred-called
   dd_log=
   assertion_linux_shred_path_executed=true
   assertion_portable_dd_fallback_executed=true
   assertion_files_unlinked_in_both_paths=true
13 tests, 1 failure
```

**Result:** FAIL. The unchanged macOS `dd` fallback fixture still stops the canonical wrapper
before shell-unit execution. A separate current-session all-shell-unit run executed every
`*_test.sh`: all remediation contracts passed, while the unchanged
`verify_digest_contract_test.sh` remained the sole failure because its real localhost core
health probe exits before digest-drift assertions. The existing SCOPE-102-04 wrapper DoD
therefore remains unchecked; no green result is claimed.

### Evidence-Linkage Artifact Lint

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/102-<deploy-host>-deploy-hardening
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
```

**Result:** PASS. The SCOPE-102-02 and SCOPE-102-04 meta-DoD evidence links are now
parser-recognized; no unchecked DoD item was changed and no certification field was written.

### Canonical Product Lint

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `./smackerel.sh lint`
**Exit Code:** 0
**Output:**

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
```

**Result:** PASS. Ruff and the product web-manifest and JavaScript checks are clean after
the formatting-only test corrections.

### Superseded Pre-Rework Python Unit Record

The pre-reconciliation Python run did not contain the 13-builder matrix and is
not active SCOPE-102-03 completion evidence. Current-session Python evidence is
[SCOPE-102-03 Rework Evidence 2](#rework-evidence-2---tp-c3-0120-and-tp-c3-27-python-suite),
which records the final `592 passed, 2 skipped` tree.

### No-Defaults Implementation Reality Scan

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
ℹ️  INFO: Scopes yielded 0 files — falling back to design.md for file discovery
⚠️  WARN: Resolved 23 file(s) from design.md fallback — scopes.md should reference these directly
ℹ️  INFO: Resolved 23 implementation file(s) to scan
--- Scan 1: Gateway/Backend Stub Patterns ---
--- Scan 1B: Handler / Endpoint Execution Depth ---
--- Scan 1C: Endpoint Not-Implemented / Placeholder Responses ---
--- Scan 1D: External Integration Authenticity ---
--- Scan 2: Frontend Hardcoded Data Patterns ---
--- Scan 2B: Sensitive Client Storage ---
--- Scan 3: Frontend API Call Absence ---
--- Scan 4: Prohibited Simulation Helpers in Production ---
--- Scan 5: Default/Fallback Value Patterns ---
--- Scan 6: Live-System Test Interception ---
--- Scan 7: IDOR / Auth Bypass Detection (Gate G047) ---
--- Scan 8: Silent Decode Failure Detection (Gate G048) ---
============================================================
  IMPLEMENTATION REALITY SCAN RESULT
============================================================
  Files scanned:  23
  Violations:     0
  Warnings:       1
🟡 PASSED with 1 warning(s) — manual review advised
```

**Result:** PASS with one discovery warning. The scanner finds zero default, stub, fake-data,
test-interception, auth-bypass, or silent-decode violations after the no-defaults repair.

### Final State-Transition Guard

**Phase:** implement. **Claim Source:** executed.
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/102-<deploy-host>-deploy-hardening; exit_code=$?; echo "EXIT_CODE=$exit_code"; [[ "$exit_code" -eq 0 ]]`
**Exit Code:** 1
**Output:**

```text
--- Check 34: Capability Foundation Enforcement (Gate G094) ---
✅ PASS: Capability foundation requirements are satisfied, not applicable, or grandfathered (Gate G094)
--- Check 35: Discovered-Issue Disposition (Gate G095) ---
✅ PASS: Discovered-issue disposition clean — no unfiled deferrals (Gate G095)
--- Check 36: Requirement-Mechanism Correspondence (Gate G097) ---
✅ PASS: Requirement-mechanism correspondence satisfied, disclosed, not applicable, or grandfathered (Gate G097)
--- Check 37: Observability Posture Declared (Gate G098) ---
✅ PASS: Observability posture is declared & well-formed, undeclared+warn, or EXEMPT (Gate G098)
--- Check 38: Observability Opt-Out Freshness (Gate G099) ---
✅ PASS: Observability opt-out is recorded & well-formed (or not opted-out / EXEMPT) (Gate G099)
--- Check 39: Observability SLO Evidence (Gate G100) ---
✅ PASS: Observability SLO evidence meets the contract, or the gate no-ops (not wired / no instrumented slo workflow / non-adopter / not-attributed-to-this-spec / EXEMPT) (Gate G100)
============================================================
  TRANSITION GUARD VERDICT
============================================================
🔴 TRANSITION BLOCKED: 46 failure(s), 2 warning(s)
state.json status MUST NOT be set to 'done'.
Fix ALL blocking failures above before attempting promotion.
🔍 Running project-defined gates from .github/bubbles-project.yaml...
EXIT_CODE=1
```

**Result:** FAIL. This is the required honest non-terminal verdict. The same run confirms
that canonical scope status (G041), all 34 checked DoD evidence links, git-backed delta
evidence (G053), implementation reality (G028), and discovered-issue disposition (G095)
pass. The remaining blockers are grouped in DI-102-01 through DI-102-04 below.

## Planning Contract Reconciliation Evidence (2026-07-10)

This section is owned by `bubbles.plan` and records only commands executed during the
current planning reconciliation. It does not replace implementation evidence or certify a
status transition.

### Artifact Inventory And Linkage

**Phase:** plan. **Claim Source:** executed.
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ uservalidation checklist has checked-by-default entries
✅ All checklist bullet items use checkbox syntax
✅ Top-level status matches certification.status
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

### Scenario, Test, And DoD Traceability

**Phase:** plan. **Claim Source:** executed.
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Output:**

```text
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 34 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
--- Gherkin → DoD Content Fidelity (Gate G068) ---
✅ scopes.md scenario maps to DoD item: SCN-102-C1-01 — the sidecar env carries no managed secret
✅ scopes.md scenario maps to DoD item: SCN-102-C1-03 — a re-added secret fails the guard (adversarial)
✅ scopes.md scenario maps to DoD item: SCN-102-C2-02 — alerting survives a re-apply with no manual re-run
✅ scopes.md scenario maps to DoD item: SCN-102-C2-03 — a fired alert reaches ntfy as a titled message
✅ scopes.md scenario maps to DoD item: SCN-102-C3-02 — the validator uses real KV math (adversarial)
✅ scopes.md scenario maps to DoD item: SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked
✅ scopes.md scenario maps to DoD item: SCN-102-C4-01 — an unreadable NATS volume must not silently succeed
✅ scopes.md scenario maps to DoD item: SCN-102-C4-02 — a root-owned manifest must not hard-fail the backup
ℹ️  DoD fidelity: 8 scenarios checked, 8 mapped to DoD, 0 unmapped
ℹ️  Scenarios checked: 8
ℹ️  Scenario-to-row mappings: 8
ℹ️  Concrete test file references: 8
ℹ️  Report evidence references: 8
RESULT: PASSED (0 warnings)
```

### Capability, Coverage, Consumer, And Boundary Guards

**Phase:** plan. **Claim Source:** executed.
**Commands:**
`bash .github/bubbles/scripts/capability-foundation-guard.sh specs/102-<deploy-host>-deploy-hardening`
and `bash .github/bubbles/scripts/state-transition-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** G094=0; state-transition=1. The transition refusal is expected and preserved;
the planning-check window below is green.
**Output (dedicated G094 output followed by the planning-check window from the full guard):**

```text
$ bash .github/bubbles/scripts/capability-foundation-guard.sh specs/102-<deploy-host>-deploy-hardening
capability-foundation-guard: Gate G094 applies: triggerHits=83 concreteImplementationEntries=0
capability-foundation-guard: spec.md contains Domain Capability Model
capability-foundation-guard: design.md contains non-empty Single-Implementation Justification
capability-foundation-guard: UX primitive check not applicable: screenCount=0 uiReuseHits=0
capability-foundation-guard: PASS Gate G094 - capability foundation requirements satisfied
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/102-<deploy-host>-deploy-hardening
--- Check 5A: SLA Stress Coverage ---
✅ PASS: SLA-sensitive scope includes stress coverage: scopes.md
--- Check 8A: Scenario-Specific Regression E2E Coverage ---
ℹ️  INFO: Scope-Kind 'contract-only' for scopes.md — E2E regression rows not required (v4.1.0 scopeKinds opt-out)
--- Check 8B: Consumer Trace Planning For Renames/Removals ---
✅ PASS: Scope includes Consumer Impact Sweep section: scopes.md
✅ PASS: Scope DoD includes consumer impact sweep completion item: scopes.md
✅ PASS: Scope lists affected consumer surfaces for rename/removal work: scopes.md
--- Check 8C: Shared Infrastructure Blast-Radius Planning ---
ℹ️  INFO: No shared fixture/bootstrap scope patterns detected — blast-radius planning check not applicable
--- Check 8D: Change Boundary Containment ---
✅ PASS: Scope includes Change Boundary section: scopes.md
✅ PASS: Scope DoD includes change-boundary containment item: scopes.md
✅ PASS: Scope enumerates allowed and excluded surfaces for the change boundary: scopes.md
--- Check 18: Deferral Language Scan (Gate G040) ---
✅ PASS: Zero deferral language found in scope and report artifacts (Gate G040)
--- Check 22: DoD-Gherkin Content Fidelity (Gate G068) ---
✅ PASS: All 8 Gherkin scenarios have faithful DoD items (Gate G068)
--- Check 34: Capability Foundation Enforcement (Gate G094) ---
✅ PASS: Capability foundation requirements are satisfied, not applicable, or grandfathered (Gate G094)
🔴 TRANSITION BLOCKED: 20 failure(s), 2 warning(s)
state.json status MUST NOT be set to 'done'.
```

**Result:** Planning-contract checks pass. The transition refusal is retained because the
canonical knb wrapper, specialist phase ledger, implement provenance, and validate-owned
certification state are not closed.

## SCOPE-102-03 Planning Reconciliation Validation

This evidence proves the planning handoff is coherent; it does not prove the
reopened implementation behavior.

### Artifact And Traceability Guards

**Phase:** plan
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Claim Source:** executed

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Top-level status matches certification.status
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

**Phase:** plan
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0
**Claim Source:** executed

```text
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 34 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
--- Gherkin → DoD Content Fidelity (Gate G068) ---
✅ scopes.md scenario maps to DoD item: SCN-102-C3-02 — the validator uses real KV math (adversarial)
✅ scopes.md scenario maps to DoD item: SCN-102-C3-01 — per-model num_ctx is SST-driven, not host-baked
ℹ️  DoD fidelity: 8 scenarios checked, 8 mapped to DoD, 0 unmapped
--- Traceability Summary ---
ℹ️  Scenarios checked: 8
ℹ️  Scenario-to-row mappings: 8
ℹ️  Concrete test file references: 8
ℹ️  Report evidence references: 8
ℹ️  DoD fidelity scenarios: 8 (mapped: 8, unmapped: 0)
RESULT: PASSED (0 warnings)
```

### Capability And Planning Guards

**Phase:** plan
**Commands:**
`bash .github/bubbles/scripts/capability-foundation-guard.sh specs/102-<deploy-host>-deploy-hardening`
`bash .github/bubbles/scripts/planning-workflow-chain-guard.sh --root /Users/pkirsanov/Projects/smackerel`
`bash .github/bubbles/scripts/planning-packet-linkage-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 0 for each command
**Claim Source:** executed

```text
capability-foundation-guard: Gate G094 applies: triggerHits=121 concreteImplementationEntries=18
capability-foundation-guard: spec.md contains Domain Capability Model
capability-foundation-guard: design.md contains non-empty Single-Implementation Justification
capability-foundation-guard: UX primitive check not applicable: screenCount=0 uiReuseHits=0
capability-foundation-guard: PASS Gate G094 - capability foundation requirements satisfied
planning-workflow-chain-guard: deliveryCapableModes=27 bootstrapChainsChecked=3 promptFilesScanned=47 root=/Users/pkirsanov/Projects/smackerel
PASS Gate G091 (planning_workflow_chain_gate) - ordered planning chain valid: bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan
planning-packet-linkage-guard: PASS Gate G087 (planning_packet_implementation_linkage_gate) - spec=specs/102-<deploy-host>-deploy-hardening status=not_started planningOnly=false
```

### Structured Parity And Reopened State

**Phase:** plan
**Command:** read-only `grep -c` and `jq -r` parity query over `scopes.md`, `test-plan.json`, `scenario-manifest.json`, and `state.json`
**Exit Code:** 0
**Claim Source:** executed

```text
27
27
structuredTests=27 status=in_progress
builders=13 linkedTests=23 verificationState=reopened_pending_implementation
completedScopes=SCOPE-102-01,SCOPE-102-02 reworkScope=SCOPE-102-03 reworkOwner=bubbles.implement currentScope=SCOPE-102-03 nextAgent=bubbles.implement certificationCompleted=0
```

The first two lines are the Markdown TP-C3 row count and unchecked TP-C3 DoD
count. `certificationCompleted=0` is preserved from the validate-owned block; this
planning pass did not invent certification for scopes 01/02/04.

### Consolidated Transition Verdict

**Phase:** plan
**Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/102-<deploy-host>-deploy-hardening`
**Exit Code:** 1
**Claim Source:** executed

```text
--- Check 3F: Transition And Rework Packets (Gate G061) ---
✅ PASS: state.json transitionRequests queue is empty
🔴 BLOCK: state.json still contains non-empty reworkQueue entries — open rework remains (Gate G061)
--- Check 4: DoD Completion (Zero Unchecked) ---
ℹ️  INFO: DoD items total: 66 (checked: 32, unchecked: 34)
🔴 BLOCK: Resolved scope artifacts have 34 UNCHECKED DoD items — ALL must be [x] for 'done'
--- Check 5: Scope Status Cross-Reference ---
ℹ️  INFO: Resolved scopes: total=4, Done=2, In Progress=2, Not Started=0, Blocked=0
--- Check 8: Test File Existence ---
✅ PASS: Test file exists: tests/integration/agent/loop_test.go
✅ PASS: Test file exists: internal/assistant/openknowledge/llm/client_test.go
--- Check 16: Implementation Reality Scan (Gate G028) ---
🔴 BLOCK: Implementation reality scan found 3 source code violation(s) — STUB/FAKE DATA DETECTED (Gate G028)
🔴 VIOLATION [DEFAULT_FALLBACK] ml/app/main.py:83
🔴 VIOLATION [DEFAULT_FALLBACK] ml/app/main.py:95
🔴 VIOLATION [DEFAULT_FALLBACK] ml/app/main.py:107
--- Check 22: DoD-Gherkin Content Fidelity (Gate G068) ---
✅ PASS: All 8 Gherkin scenarios have faithful DoD items (Gate G068)
--- Check 34: Capability Foundation Enforcement (Gate G094) ---
✅ PASS: Capability foundation requirements are satisfied, not applicable, or grandfathered (Gate G094)
🔴 TRANSITION BLOCKED: 22 failure(s), 2 warning(s)
state.json status MUST NOT be set to 'done'.
```

This is the intended honest handoff verdict: implementation and certification are
open, while the planning-specific gates pass and every planned test file resolves.

## Discovered Issues

Per Gate G095, every issue observed during this work has an explicit disposition.
DI-102-01/02 are pre-existing macOS/BSD environmental test failures in files this
feature did not modify. DI-102-05 is closed by the current full-integration run.

### DI-102-01 — macOS `dd` Fixture

**Date:** 2026-07-10. **Source:** SCOPE-102-02 and SCOPE-102-04 Evidence 5.
**Disposition:** routed to **bubbles.validate**.

`tests/unit/shred-and-remove.bats` test 7 fails on macOS because its portable fallback
fixture expects GNU `dd`; `run-tests.sh` therefore exits under `set -e` before the
shell-unit tier. This spec-001 test is unmodified by spec 102 and passes on GNU/Linux.
The direct 22-file shell-unit run in SCOPE-102-04 Evidence 5 provides additional local
proof: 21/22 files pass. Linux-CI confirmation of the canonical wrapper remains required.

### DI-102-02 — Live Core Health Prerequisite

**Date:** 2026-07-10. **Source:** SCOPE-102-04 Evidence 5.
**Disposition:** routed to **bubbles.validate**.

`tests/unit/verify_digest_contract_test.sh` exits at the real core health probe
(`127.0.0.1:41001/api/health`) before reaching its digest-drift assertions because no core
service is listening in the local test environment. The spec-011 `verify.sh` and
`scope04-fixtures.bash` surfaces are unmodified by spec 102. Linux-CI or live-stack
confirmation remains required; the SCOPE-102-02 contract-mirror fix is a separate file.

### DI-102-03 — Planning Contract Drift (CLOSED 2026-07-10)

**Date:** 2026-07-10. **Source:** planning reconciliation guard runs above.
**Disposition:** addressed by **bubbles.plan**.

Closed with explicit contract-only E2E and stress/load declarations; exact Smackerel/knb
paths and ownership; F1/F2/F3 security-test linkage; canonical non-live test categories;
consumer and change-boundary declarations; zero G040 hits; faithful G068 DoD text; a
reconciled scenario manifest; and `test-plan.json`. Artifact lint, traceability, G094, and
all state-transition planning checks pass. No execution result was synthesized.

### DI-102-04 — Phase And Certification Ledger Incomplete (OPEN, FOREIGN-OWNED)

**Date:** 2026-07-10. **Source:** final state-transition guard.
**Disposition:** owned by the historical implement invocation/orchestrator, the remaining
full-delivery specialists, and **bubbles.validate**.

Exact remaining state defects: `certification.certifiedCompletedPhases` and
`certification.scopeProgress` are absent; `certification.completedScopes` is empty; eleven
full-delivery specialist phase records are absent; and the historical `implement` objects in
`execution.completedPhaseClaims` have no matching implement/orchestrator execution-history
entry, so G022 reports phase impersonation. This planning run appended only its own
`bubbles.plan` history entry. It did not backfill implement provenance or write
`certification.*`.

### DI-102-05 — Full Integration Ollama Image Pull (CLOSED 2026-07-11)

**Date:** 2026-07-10. **Source:** SCOPE-102-03 Rework Evidence 4.
**Disposition:** resolved by canonical dependency warm-up followed by the literal
Test Plan command; no product/deploy code change.

Early attempts expired during the multi-gigabyte Ollama pull or transient Docker
Hub TLS handshakes. `./smackerel.sh --env test up` completed that acquisition
through the canonical disposable-stack lifecycle, and `down --volumes` restored a
clean baseline. The subsequent exact `./smackerel.sh test integration` run reached
all Go tests and exposed stale test-only config fixtures. After the fixture repair,
the final exact run exits 0; focused TP-C3-21 full-stack evidence also exits 0.

## Completion Statement

**TP-C3-21 CLOSED (2026-07-11).** The exact full integration Test Plan command and
focused full-stack selector both exit 0. TP-C3-21 alone is checked; SCN-102-C3-01,
the Build Quality Gate, scope Done, top-level status, and certification remain
unchanged by request. Validate readiness is still blocked by the pre-existing
DI-102-01/02/04 and other unchecked aggregate DoD items. No live <deploy-host> apply,
rollback, git reconcile, commit, push, status promotion, or certification occurred.

## Independent Deep Validation Evidence (2026-07-11)

This section records only commands independently executed by `bubbles.validate`
against the current dirty trees in this session. R-102-A/B/C/D were not executed;
no live deploy, rollback, commit, or push occurred.

### Scope 04 Canonical knb Wrapper

**Phase:** validate
**Command:** `cd /Users/pkirsanov/Projects/smackerel && bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
upstream-ci-grep.bats
 ✓ upstream ci grep: smackerel workflows contain no host-secret sensitive references
   workflow_file_count_before=6
   grep_status=1
   workflow_file_count_after=6
   read_only_boundary=upstream_workflow_hashes_unchanged
113 tests, 0 failures
==> shell functional suite
apply-rollback-manifest-idempotency.bats
6 tests, 0 failures
==> status/verify fixture: status remains read-only and reports drift
PASS: status.sh reports digest and contract drift in read-only mode
==> status/verify fixture: verify blocks the same digest drift
verify OK (stack health)
PASS: verify.sh fails on the digest drift that status.sh reports
status_verify_fixture_test.sh OK
```

**Result:** PASS. The prior macOS `dd` fixture and localhost-health blockers no
longer prevent the canonical wrapper from completing. Scope 04's exact broad
adapter command is green in this session.

### Scope 03 Python And Focused Go Contracts

**Phase:** validate
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh test unit --python`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.....................................................s.................. [ 23%]
........................................................................ [ 34%]
........................................................................ [ 46%]
........................................................................ [ 57%]
........................................................................ [ 69%]
........................................................................ [ 81%]
........................................................................ [ 92%]
..............................................                           [100%]
620 passed, 2 skipped in 45.20s
[py-unit] pytest ml/tests finished OK
SCOPE03_PYTHON_EXIT=0
```

**Result:** PASS for the Spec-102 Python matrix. The two suite skips are the
existing opt-in live-Ollama/runtime-dependency paths, not linked Spec-102 test
rows; they are reported rather than converted to zero skips.

**Phase:** validate
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh test unit --go --go-run 'TestSpec102Go|TestValidateModelEnvelopes_(KVMathFailsUnderstated|RefusesUncappedAcceptsCapped|CoResidentSumGatedByMaxLoaded)_Spec102|ProviderDispatch|TestLoadStatus_KnbAdapterSchemaParity' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
=== RUN   TestSpec102GoUsesTypedMLBoundary
--- PASS: TestSpec102GoUsesTypedMLBoundary (0.30s)
=== RUN   TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly
--- PASS: TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly (8.36s)
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102
--- PASS: TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102 (0.00s)
=== RUN   TestLoadStatus_KnbAdapterSchemaParity
--- PASS: TestLoadStatus_KnbAdapterSchemaParity (0.04s)
=== RUN   TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102
--- PASS: TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102
--- PASS: TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102
--- PASS: TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102 (0.00s)
[go-unit] go test ./... finished OK
SCOPE03_04_GO_EXIT=0
```

**Result:** PASS. The typed Go boundary, read-only Ollama probes, provider
controls, KV-envelope cases, and cross-repo backup status schema all executed.

### Build And Exact Integration Lane

**Phase:** validate
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh build`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
Smackerel pre-flight resource check: OK
  RAM  available: 9708 MB (required >= 6000 MB)
  Disk available: 8205271 MB / 8013.0 GB (required >= 15 GB)
[+] Building 545.3s (49/49) FINISHED
 => [smackerel-core builder 7/8] RUN if [ -n "${GO_BUILD_TAGS}" ]; then  154.9s
 => [smackerel-ml] exporting to image                                    530.2s
 => [smackerel-core builder 8/8] RUN CGO_ENABLED=0 GOOS=linux go build -  14.4s
 => [smackerel-core] exporting to image                                    8.3s
[+] build 2/2
 ✔ Image smackerel-smackerel-ml   Built                                   545.4s
 ✔ Image smackerel-smackerel-core Built                                   545.4s
BUILD_EXIT=0
```

**Result:** PASS. Both current product images built. Two integration attempts
before this build exited 124 while Docker copied/exported the ML image and did
not reach tests; both invoked disposable-stack teardown and are not counted as
test passes.

**Phase:** validate
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh test integration`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
--- PASS: TestCardRewardsPipelineLivePG_ReRunIdempotent_I06 (0.10s)
--- PASS: TestRecommendLivePG_PerCategoryGeneration_G06 (0.02s)
--- PASS: TestRecommendLivePG_StarredOverridePreserved_G07 (0.03s)
--- PASS: TestReconcileLivePG_IdempotentUpsert_F07 (0.02s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     1.771s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
Container smackerel-test-smackerel-core-1 Removed
Container smackerel-test-smackerel-ml-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
SCOPE03_INTEGRATION_FINAL_EXIT=0
```

**Result:** PASS. The literal TP-C3-21 command completed against the disposable
stack and removed its containers, volumes, and network.

## Test Phase Closure (2026-07-11)

This section is the `bubbles.test` execution ledger for the current standalone
test phase. It supersedes earlier test-phase uncertainty only; it does not alter
planning-owned scope text, `test-plan.json`, top-level scope completion,
`requiresRevalidation`, or validate-owned `certification.*`. No source, deploy,
live-host, commit, or push operation was performed.

### Python Unit Matrix

**Phase:** test
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh test unit --python`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
+ echo '[py-unit] pip install OK; starting pytest ml/tests'
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.....................................................s.................. [ 23%]
........................................................................ [ 34%]
........................................................................ [ 46%]
........................................................................ [ 57%]
........................................................................ [ 69%]
........................................................................ [ 81%]
........................................................................ [ 92%]
..............................................                           [100%]
620 passed, 2 skipped in 95.99s (0:01:35)
+ echo '[py-unit] pytest ml/tests finished OK'
[py-unit] pytest ml/tests finished OK
PYTHON_EXIT=0
```

**Result:** PASS for the selected Spec-102 Python rows. The broad Python lane
reports two skips rather than zero; the separate selected-file integrity scan
below found zero skip/only markers in the 19 Spec-102-selected files. This
section does not convert the broad-suite skip count to zero.

### Focused Go Boundary, Provider, KV, And Schema Matrix

**Phase:** test
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh test unit --go --go-run 'TestSpec102Go|TestValidateModelEnvelopes_(KVMathFailsUnderstated|RefusesUncappedAcceptsCapped|CoResidentSumGatedByMaxLoaded)_Spec102|ProviderDispatch|TestLoadStatus_KnbAdapterSchemaParity' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
=== RUN   TestSpec102GoUsesTypedMLBoundary
--- PASS: TestSpec102GoUsesTypedMLBoundary (0.04s)
=== RUN   TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly
--- PASS: TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly (5.76s)
=== RUN   TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102
--- PASS: TestProviderDispatchParams_LegitimateProviderContractsSurvive_Security102 (0.00s)
=== RUN   TestLoadStatus_KnbAdapterSchemaParity
--- PASS: TestLoadStatus_KnbAdapterSchemaParity (0.02s)
=== RUN   TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102
--- PASS: TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102
--- PASS: TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102 (0.00s)
=== RUN   TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102
--- PASS: TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102 (0.00s)
[go-unit] go test ./... finished OK
FOCUSED_GO_EXIT=0
```

**Result:** PASS. The typed Go sidecar boundary, read-only Ollama endpoint
inventory, legitimate provider contracts, KV-aware envelope cases, and knb
backup-status schema parity all executed by name.

### Exact Integration Test Plan Command

**Phase:** test
**Command:** `cd /Users/pkirsanov/Projects/smackerel && ./smackerel.sh test integration`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
--- PASS: TestCardRewardsPipelineLivePG_ReRunIdempotent_I06 (0.18s)
--- PASS: TestRecommendLivePG_PerCategoryGeneration_G06 (0.05s)
--- PASS: TestRecommendLivePG_StarredOverridePreserved_G07 (0.04s)
--- PASS: TestReconcileLivePG_IdempotentUpsert_F07 (0.05s)
PASS
ok      github.com/smackerel/smackerel/internal/cardrewards     1.451s
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
Container smackerel-test-smackerel-core-1 Removed
Container smackerel-test-smackerel-ml-1 Removed
Container smackerel-test-nats-1 Removed
Network smackerel-test_default Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-ollama-data Removed
INTEGRATION_EXIT=0
```

**Result:** PASS. The literal TP-C3-21 command completed against the disposable
stack and its teardown removed the test containers, network, and named volumes.
An earlier launch attempt from the wrong working directory exited 127 before
the CLI started and is not counted as test evidence.

### Canonical knb Adapter Wrapper

**Phase:** test
**Command:** `cd /Users/pkirsanov/Projects/smackerel && bash ../<deployment-owner>/<product>/<target>/tests/run-tests.sh`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
upstream-ci-grep.bats
 ✓ upstream ci grep: smackerel workflows contain no host-secret sensitive references
   workflow_file_count_before=6
   grep_status=1
   workflow_file_count_after=6
   read_only_boundary=upstream_workflow_hashes_unchanged
113 tests, 0 failures
==> shell functional suite
apply-rollback-manifest-idempotency.bats
6 tests, 0 failures
==> status/verify fixture: status remains read-only and reports drift
PASS: status.sh reports digest and contract drift in read-only mode
==> status/verify fixture: verify blocks the same digest drift
verify OK (stack health)
PASS: verify.sh fails on the digest drift that status.sh reports
status_verify_fixture_test.sh OK
KNB_WRAPPER_EXIT=0
```

**Result:** PASS. The exact Scope-102-04 wrapper completed with the two aggregate
test counts green and the read-only status/verify drift fixture green.

### Test Integrity And Regression Quality

There is no installed `test-integrity-guard.sh`. The canonical
`bubbles-test-integrity` skill defines the executable constituent checks used
here: selected-file skip/only scanning, live-category interception scanning,
scenario traceability, and `regression-quality-guard.sh`.

**Phase:** test
**Commands:** selected-file skip/only scan; live integration interception scan;
`bash .github/bubbles/scripts/traceability-guard.sh specs/102-<deploy-host>-deploy-hardening`;
`bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix <19 selected test files>`
**Exit Codes:** 0 / 0 / 0 / 0
**Claim Source:** executed
**Output:**

```text
TEST_INTEGRITY_SKIP_SCAN=PASS matches=0 files=19
TEST_INTEGRITY_LIVE_MOCK_SCAN=PASS matches=0 files=2
TEST_INTEGRITY_CONSTITUENT_EXIT=0
✅ scenario-manifest.json covers 34 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
ℹ️  DoD fidelity: 8 scenarios checked, 8 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 19
Files with adversarial signals: 16
REGRESSION_QUALITY_EXIT=0
```

**Result:** PASS. The selected tests contain no skip/only marker, the live
integration files contain no interception/mock pattern, all parsed scenarios
map to DoD, and bugfix-strength regression quality reports zero violations.

### Test Phase Verdict And Ownership Boundary

**Test phase verdict:** `TESTED` for the contract-only Spec-102 Test Plan. The
Python matrix, focused Go matrix, literal integration command, canonical knb
wrapper, selected-file integrity checks, traceability guard, and regression
quality guard all have current-session zero exits. No trace/SLO artifact is due:
the project posture is wired, but no Spec-102 Test Plan row declares an
`observabilityWorkflow`.

Planning-owned registry text still describes SCOPE-102-03 and SCOPE-102-04 as
In Progress, and `test-plan.json` still marks the canonical wrapper blocked.
Those fields are not writable by `bubbles.test`. Top-level `completedScopes`,
`requiresRevalidation`, and all `certification.*` fields also remain unchanged;
their reconciliation belongs to the registry owners after this execution claim.
