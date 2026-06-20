# Smackerel Operations Runbook

This guide covers deployment, daily operations, connector management, troubleshooting, backup/restore, and monitoring for a self-hosted Smackerel instance.

## Terminal Status by Workflow Mode

A spec or bug's `state.json` status is **terminal** (the work is complete for its workflow mode) when ANY of these are true:

1. `status == "done"` — universal terminal status, OR
2. `status == mode.statusCeiling` — the mode's declared completion status, OR
3. `status ∈ mode.terminalAliases` — explicit synonyms declared in [`.github/bubbles/workflows.yaml`](../.github/bubbles/workflows.yaml).

Some Bubbles modes intentionally have ceilings below `done` because the work they describe legitimately stops there. The following Smackerel-relevant modes are terminal at their ceiling — they are **NOT** open backlog items, and re-orchestrating them through `bugfix-fastlane` to force `done` is fake make-work:

| Mode | Terminal status | Meaning |
|------|-----------------|---------|
| `validate-only`, `audit-only`, `validate-to-doc` | `validated` | Certification or audit pass; no new code shipped |
| `docs-only`, `spec-review-to-doc`, `retro-to-review`, `release-planning-to-doc` | `docs_updated` | Documentation maintenance; no source-code changes |
| `spec-scope-hardening`, `product-to-planning` | `specs_hardened` | Spec/design/scopes hardened; no implementation |
| `adapter-readiness-to-packet`, `dark-launch-shipped`, `migration-shipped-pending-cutover` | `delivered_pending_activation` | Implementation + tests + audit shipped; live runtime evidence deferred to operator/cutover/rollout |

To check terminal-for-mode status mechanically, use:

```bash
bash .github/bubbles/scripts/is-terminal-for-mode.sh "$status" "$mode"
# exit 0 = terminal-for-mode; exit 1 = not terminal; exit 2 = error
```

Portfolio sweeps (`spec-dashboard.sh`, `bubbles.status`, `bubbles.recap`, retro tooling) use this helper so ceiling-bound packets are counted as completed work. The mechanical `state-transition-guard.sh` continues to block actual ceiling violations — promotion past a mode's ceiling is forbidden and is not loosened by this rule.

## Deployment

### Production Deploy (Build-Once Deploy-Many)

For a production-class deploy on any target — self-hosted home lab,
staging, or production — **start here, not in First-Time Setup**.

Smackerel uses the Build-Once Deploy-Many architecture: this repo's CI
publishes immutable signed images, SBOM + SLSA attestations, and
per-environment config bundles; a per-target deploy adapter (in-tree
under `deploy/<target>/` for generic targets, or out-of-tree under
`${DEPLOY_TARGETS_ROOT}/smackerel/<target>/` for operator-coupled
targets like home-lab adapters) consumes those artifacts and applies them by
digest.

Read the production guide first:

- [`docs/Deployment.md`](Deployment.md) — full operator workflow,
  artifact contract, adapter contract, deploy-adapter overlay
  dependency, and Generic Pre-Apply Prerequisites
- [`deploy/README.md`](../deploy/README.md) — adapter locality rule
  (in-tree vs out-of-tree) and `DEPLOY_TARGETS_ROOT` resolution
- [`docs/Home_Lab_Deployment_Plan.md`](Home_Lab_Deployment_Plan.md) —
  migration-pointer stub naming the deploy-adapter overlay
  as the owner of the home-lab adapter

Production secret prerequisites are enumerated in
[`docs/Deployment.md`](Deployment.md) §"Generic Pre-Apply
Prerequisites (Product Contract)" — confirm them before invoking any
adapter `apply.sh`.

### Home-Lab Activation Boundary

This product repo proves generic artifact production, status delegation, and
runtime contracts. It does not prove live home-lab activation by itself. A live
home-lab apply requires the operator-private adapter packet:

| Input | Source |
|-------|--------|
| Source SHA | Upstream build manifest for the chosen release. |
| Core and ML image digests | Upstream build manifest, passed as digest-only apply flags. |
| Config bundle ref | `home-lab-<sourceSha>` from the build manifest. |
| Config bundle SHA | Build-manifest `sha256` value for the same bundle. |
| Out-of-tree adapter root | `DEPLOY_TARGETS_ROOT` resolving to `<adapter-root>/smackerel/<target>/`. |
| Concrete params | Adapter `params.yaml`, including target identity, ports, paths, Caddy, and bind address. |
| Encrypted secrets | Adapter-owned SOPS/age ciphertext for the target. |
| Release proof | Cosign, SLSA, SBOM, vulnerability proof, and bundle/source identity receipts. |
| Live approval | Human operator authorization; static docs and fixture tests do not mutate the home-lab host. |

Current KNB home-lab port guidance is `41001` for `smackerel-core` and `41002`
for `smackerel-ml`. Local dev remains on `40001/40002`; do not treat those dev
ports as home-lab activation values.

| Surface | Placeholder-only access path | Home-lab adapter port | Owner boundary |
|---------|------------------------------|-----------------------|----------------|
| Core API and web UI | `https://smk.<tailnet-fqdn>` through host Caddy to `127.0.0.1:41001` | `41001` | KNB adapter writes explicit `HOST_BIND_ADDRESS`; host Caddy owns TLS/tailnet identity. |
| ML sidecar health/devops | `https://smk-ml.<tailnet-fqdn>` through host Caddy to `127.0.0.1:41002` | `41002` | KNB adapter writes route and bind; host-owned `devops_auth` gates access. |
| PostgreSQL | `tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-postgres ...` | No host port | Compose service is internal only. |
| NATS | `tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-nats ...` | No host port | Compose service is internal only. |
| Status | `./smackerel.sh deploy-target <target> status` or adapter `status.sh` | Read-only | Product dispatcher delegates to adapter `status.sh` when executable and otherwise prints a generic read-only fallback. |

### First-Time Setup (Local Dev)

> **Production-class operators:** see Production Deploy (Build-Once
> Deploy-Many) above. The steps below stand up a **dev-only** Smackerel
> stack from a local clone using the `./smackerel.sh up` profile.
> They are NOT a substitute for the signed-artifact, adapter-driven
> production flow.

1. **Clone the repository:**
   ```bash
   git clone <repo-url> && cd smackerel
   ```

2. **Edit configuration:**
   ```bash
   nano config/smackerel.yaml
   ```
   At minimum, set:
   - `runtime.auth_token` — a secure Bearer token (min 16 chars: `openssl rand -hex 24`)
   - `llm.provider`, `llm.model`, `llm.api_key` — your LLM provider credentials
   - `telegram.bot_token`, `telegram.chat_ids` — if using the Telegram bot

3. **Generate runtime config:**
   ```bash
   ./smackerel.sh config generate
   ```
   This renders `config/generated/dev.env` and `config/generated/test.env` from `config/smackerel.yaml`.

4. **Build Docker images:**
   ```bash
   ./smackerel.sh build
   ```

5. **Start the stack:**
   ```bash
   ./smackerel.sh up
   ```

6. **Verify:**
   ```bash
   ./smackerel.sh status
   ```
   All 4 services (postgres, nats, smackerel-core, smackerel-ml) should show as healthy.

### Configuration Changes

After editing `config/smackerel.yaml`, always regenerate and restart:

```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

**Never edit files under `config/generated/` directly.** They are derived artifacts regenerated from `config/smackerel.yaml`.

### Upgrading

```bash
git pull
./smackerel.sh build
./smackerel.sh down && ./smackerel.sh up
```

Database migrations run automatically on startup. The migration runner uses PostgreSQL advisory locks to prevent concurrent migration attempts.

### Pre-built Image Deployment

Tagged releases publish images to GitHub Container Registry (GHCR). To deploy from pre-built images instead of building from source:

1. **Set image override variables** in your environment or `config/smackerel.yaml`:
   ```bash
   export SMACKEREL_CORE_IMAGE=ghcr.io/<owner>/smackerel-core:v1.0.0
   export SMACKEREL_ML_IMAGE=ghcr.io/<owner>/smackerel-ml:v1.0.0
   ```

2. **Pull and start:**
   ```bash
   ./smackerel.sh config generate
   ./smackerel.sh up
   ```
   Compose pulls the pre-built images instead of building from source.

3. **Rollback** to a previous version:
   ```bash
   export SMACKEREL_CORE_IMAGE=ghcr.io/<owner>/smackerel-core:v0.9.0
   export SMACKEREL_ML_IMAGE=ghcr.io/<owner>/smackerel-ml:v0.9.0
   ./smackerel.sh down && ./smackerel.sh up
   ```

When `SMACKEREL_CORE_IMAGE` and `SMACKEREL_ML_IMAGE` are unset (the default), Compose builds from local Dockerfiles as before.

## Stack Lifecycle

| Action | Command |
|--------|---------|
| Start all services | `./smackerel.sh up` |
| Stop all services | `./smackerel.sh down` |
| Stop and remove volumes | `./smackerel.sh down --volumes` |
| Check service health | `./smackerel.sh status` |
| View logs | `./smackerel.sh logs` |
| Rebuild images | `./smackerel.sh build` |
| Backup database | `./smackerel.sh backup` |
| Clean unused Docker resources | `./smackerel.sh clean smart` |
| Full Docker cleanup | `./smackerel.sh clean full` |
| Measure Docker disk usage | `./smackerel.sh clean measure` |

### Health Check Endpoint

```bash
curl http://127.0.0.1:40001/api/health
```

Returns JSON with status for: API, PostgreSQL, NATS, ML sidecar, Telegram bot, Ollama (if enabled), and knowledge layer stats.

### Prometheus Metrics

Available at `http://127.0.0.1:40001/metrics` (unauthenticated, standard Prometheus scrape endpoint).

## DevOps Access on Home-Lab (Tailnet-Edge Pattern)

On home-lab and production deployments, the deploy compose
(`deploy/compose.deploy.yml`) implements the canonical tailnet-edge bind
pattern (see `bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` and
[spec 042](../specs/042-tailnet-edge-bind-pattern/spec.md)). Backend
services bind through fail-loud `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}`
substitution and infra services (Postgres, NATS) have **no host port mapping**.
There is no Compose fallback default: the deploy adapter must write
`HOST_BIND_ADDRESS` explicitly into `app.env` (for example `127.0.0.1` for
loopback or a tailnet bind address for edge fronting). Missing or empty values
abort Docker Compose at substitution time. This section shows the canonical
DevOps access shapes for each.

### HTTP UIs (Pattern P5: Host Caddy on the Tailscale IP)

The `smackerel-core` API and the `smackerel-ml` sidecar are reached via
the host Caddy reverse proxy running on the Tailscale IP. The deploy adapter
deployment adapter writes the Caddy snippet from the canonical Bubbles
template (`bubbles/templates/caddy-tailnet-snippet.caddy.template`); this
repo only ensures the compose is ready.

```bash
# Core API health (HTTPS via host Caddy on the tailnet)
curl --max-time 5 https://smackerel.<host-tailnet-fqdn>/api/health

# ML sidecar health (HTTPS via host Caddy on the tailnet)
curl --max-time 5 https://ml.smackerel.<host-tailnet-fqdn>/health
```

`<host-tailnet-fqdn>` is the host's Tailscale FQDN placeholder. The exact
subdomain shape is owned by the deploy adapter and can be customized per
deployment.

### PostgreSQL (Pattern P1: docker exec over Tailscale SSH)

There is no published Postgres host port. DevOps reaches Postgres via:

```bash
# Interactive psql session (recommended)
tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-postgres \
    psql -U smackerel -d smackerel

# Single-shot query
tailscale ssh <deploy-host> -- docker exec -i smackerel-home-lab-postgres \
    psql -U smackerel -d smackerel -Atqc 'SELECT count(*) FROM artifacts'

# Streaming pg_dump backup (write the dump on the operator's workstation)
tailscale ssh <deploy-host> -- docker exec smackerel-home-lab-postgres \
    pg_dump -U smackerel -d smackerel -Fc | \
    cat > /tmp/smackerel-home-lab.pgdump
```

Container name follows the pattern `smackerel-<env>-postgres` because the
deploy compose's `COMPOSE_PROJECT` env var is set per environment by the
adapter (e.g., `smackerel-home-lab` for the home-lab target).

### NATS (Pattern P1: docker exec over Tailscale SSH)

There is no published NATS client or monitor port. DevOps reaches NATS
via:

```bash
# Subscribe to all subjects (interactive monitoring)
tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-nats \
    nats sub '>'

# Inspect server health (NATS monitor endpoint, in-network)
tailscale ssh <deploy-host> -- docker exec smackerel-home-lab-nats \
    wget -qO- http://localhost:8222/healthz

# List streams
tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-nats \
    nats stream ls
```

### NATS Production Hardening (spec 046)

Spec 046 makes every NATS payload and storage ceiling explicit via SST. The
broker, the ML sidecar reconnect contract, and the Go core's JetStream
stream caps all originate from a single source under
`infrastructure.nats` in [config/smackerel.yaml](../config/smackerel.yaml):

| SST key | Default | Purpose |
|---------|---------|---------|
| `infrastructure.nats.max_payload_bytes` | `8388608` (8 MiB) | Single-message size ceiling. Connectors with larger payloads (e.g., wide-screen OCR uploads) MUST justify a raise. |
| `infrastructure.nats.max_file_store_bytes` | `10737418240` (10 GiB) | JetStream on-disk store ceiling. Sum of per-stream caps must remain comfortably below this. |
| `infrastructure.nats.max_mem_store_bytes` | `1073741824` (1 GiB) | JetStream in-memory store ceiling. |
| `infrastructure.nats.client.reconnect_time_wait_seconds` | `2` | ML sidecar interval between reconnect attempts. |
| `infrastructure.nats.client.max_reconnect_attempts` | `-1` | ML sidecar reconnect ceiling. `-1` means indefinite — the only operationally supported value. |
| `infrastructure.nats.stream_max_bytes[]` | list of 15 entries (≈ 5.6 GiB total) | Per-stream MaxBytes. **Every JetStream stream Smackerel creates MUST have an entry.** |

The values flow:

1. `./smackerel.sh config generate` reads them via `required_value` /
   `required_json_value` in [scripts/commands/config.sh](../scripts/commands/config.sh).
   Missing values abort generation.
2. The generator writes `NATS_MAX_PAYLOAD_BYTES`, `NATS_MAX_FILE_STORE_BYTES`,
   `NATS_MAX_MEM_STORE_BYTES`, `NATS_MAX_RECONNECT_ATTEMPTS`,
   `NATS_RECONNECT_TIME_WAIT_SECONDS`, and `NATS_STREAM_MAX_BYTES_JSON`
   into `config/generated/{env}.env`, and also renders `max_payload`,
   `max_file_store`, `max_memory_store` directives into
   `config/generated/nats.conf`.
3. The Go core's `requiredVars()` chain in
   [internal/config/config.go](../internal/config/config.go) fails loud at
   startup if any of the 6 envelope env vars is missing, malformed, or
   non-positive.
4. `internal/nats.Client.EnsureStreams(ctx, streamCaps)` refuses to start
   if a JetStream stream listed in `AllStreams()` is missing from the cap
   map, or has a non-positive cap. The ML sidecar's `connect()` raises if
   `NATS_MAX_RECONNECT_ATTEMPTS` or `NATS_RECONNECT_TIME_WAIT_SECONDS`
   is missing or not an integer.

#### Tuning the envelope

- **Disk pressure** — lower `infrastructure.nats.max_file_store_bytes` to
  the disk headroom you can actually spare; ensure
  `sum(stream_max_bytes[].bytes) ≤ max_file_store_bytes`.
- **Backpressure** — lower individual entries in `stream_max_bytes[]` to
  apply the pressure where backlog actually builds (typically `ARTIFACTS`
  or `PHOTOS`).
- **Single-payload size** — raise `max_payload_bytes` only when a
  connector legitimately needs it. Larger messages slow JetStream
  propagation.
- **Sidecar reconnect interval** — raise
  `client.reconnect_time_wait_seconds` only when the deployment target
  has known NATS-restart blackout windows longer than the default
  2-second backoff.

After any change, run `./smackerel.sh config generate` then restart the
stack (`./smackerel.sh down && ./smackerel.sh up`) so the new envelope
takes effect.

#### Adding a new JetStream stream

When introducing a new stream to `internal/nats.AllStreams()`:

1. Add a matching `{stream: <NAME>, bytes: <N>}` entry to
   `infrastructure.nats.stream_max_bytes` in `config/smackerel.yaml`.
2. Re-check the sum stays below `max_file_store_bytes`.
3. Run `./smackerel.sh config generate` then `go test ./internal/nats/...`
   — the adversarial unit tests
   (`TestEnsureStreams_MissingStreamCapRejected`) will catch a missing
   entry before runtime.

### Why this pattern

- Closes the host network footgun: no operator or agent can accidentally
  `psql -h 127.0.0.1 -p <pg_host_port>` into the home-lab database from a
  tool that happens to share the env file. The only access path is via
  the audited `tailscale ssh` + `docker exec` chain.
- Reuses the Tailscale identity for authentication. No extra credentials
  to manage for the SSH side.
- Backend HTTP UIs get TLS for free via `tailscale cert`, fronted by the
  host Caddy that the deploy adapter adapter installs.

If a future deployment needs a public-internet-reachable HTTP endpoint
for one of the backends, that is a separate spec. The compose contract
in this repo stays explicit-bind-only: `HOST_BIND_ADDRESS` must be supplied
by the SST-generated env file or by the deploy adapter, and Compose must fail
loudly if it is missing. The deploy adapter is the only surface that decides
what goes on the public NIC.

## Bundle Secret Substitution (spec 052)

For production-class targets (`home-lab`, `production`, and any future
non-dev/non-test environment), Smackerel uses a 3-layer defense-in-depth
secret pipeline (Spec 052). The bundle generator emits the deterministic
placeholder marker `__SECRET_PLACEHOLDER__<KEY>__` for every managed
secret; the deploy adapter substitutes real values at apply time; the Go
runtime fails loud if any placeholder reaches startup. Full architectural
context: [`docs/Architecture.md` → Secret Boundary (spec 052)](Architecture.md#secret-boundary-spec-052).
Full deployment context: [`docs/Deployment.md` → Bundle Secret Injection (spec 052)](Deployment.md#bundle-secret-injection-spec-052).

This section is the **operator runbook** for the two recurring workflows.

### UC-052-004 Operator Secret Rotation

Rotate any of the four canonical managed secrets (`POSTGRES_PASSWORD`,
`AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, `AUTH_AT_REST_HASHING_KEY`,
`AUTH_BOOTSTRAP_TOKEN`) without a code change or a re-deploy of the
container image.

**Pre-conditions:**

- Operator has age private key for the target's secret store
  (`~/.config/sops/age/keys.txt` on the operator workstation).
- Target host is reachable over Tailscale.
- A current cosign-signed bundle for the target is already in the
  registry (any recent build manifest will do — bundle bytes are
  unchanged because secrets are not in the bundle).

**Procedure:**

1. **Generate the new value.** Use the canonical generator for the key
   class (per `docs/Deployment.md` row table):
   - `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`: rotate via
     `smackerel-core auth keygen` (PASETO v4.public).
   - `AUTH_AT_REST_HASHING_KEY`: `openssl rand -hex 32`.
   - `AUTH_BOOTSTRAP_TOKEN`: `openssl rand -hex 24`.
   - `POSTGRES_PASSWORD`: per the target's password policy.

2. **Update the operator-private secret store.** From the operator
   workstation, edit the encrypted env file with sops in place
   (sops handles age encrypt/decrypt automatically):

   ```bash
   sops <knb-repo>/smackerel/secrets/<target>.enc.env
   # Replace the line for the rotated KEY with the new value.
   # Save and exit — sops re-encrypts in place.
   ```

3. **Re-apply.** From the operator workstation:

   ```bash
   ./smackerel.sh deploy-target <target> apply
   ```

   The adapter:
   - Re-pulls the bundle (sha256 unchanged → no-op pull from cache).
   - Re-decrypts `<target>.enc.env` with the operator's age key.
   - Re-substitutes every `__SECRET_PLACEHOLDER__<KEY>__` in `app.env`.
   - Restarts the container with the new env file.
   - Appends an audit record to `/var/log/smackerel/apply.log` on the
     target with `secrets_substituted=4 placeholders_remaining=0`.

4. **Verify the substitution.** From the operator workstation:

   ```bash
   ./smackerel.sh deploy-target <target> verify
   ```

   This runs the post-apply health check and exercises an authenticated
   path to confirm the new key material is live (relevant only for the
   AUTH_* keys; POSTGRES_PASSWORD rotation is verified by container
   start success).

5. **Rotation audit log.** Record the rotation in the operator-private
   audit log (outside this repo). The rotation is also implicitly
   recorded by:
   - The `sops <target>.enc.env` commit in the deploy adapter overlay
     (sops_lastmodified, sops_version metadata).
   - The `/var/log/smackerel/apply.log` entry on the target host.

**Rollback:** If the new value is wrong, repeat steps 1–4 with the
previous value. The bundle does not need to be regenerated.

**Failure modes:**

| Failure | Fail-loud surface | Recovery |
|---------|-------------------|----------|
| New value left empty in `<target>.enc.env` | L2 adapter pre-flight check (step 7 of `apply.sh`): `assert every declared key has real value in tmpfile` → exits non-zero, container does not start | Re-edit `<target>.enc.env`, re-run `apply` |
| New value accidentally set to placeholder marker | L2 adapter pre-flight check: `non-empty AND not equal to its placeholder marker` → exits non-zero | Re-edit `<target>.enc.env`, re-run `apply` |
| Adapter substitution skipped entirely (e.g. compose `--env-file` missing) | L3 runtime check: `Validate()` returns `<KEY> still equals placeholder marker — adapter substitution failed (spec 052 FR-052-007)` → process exits 1 | Inspect adapter logs; re-run `apply` |

### UC-052-005 Auditor Inspection

Verify, without operator-private credentials, that the deployed
container started with real secret values (not placeholders) and that
no placeholder ever leaks to logs, telemetry, or error paths.

**Pre-conditions:**

- Auditor has `tailscale ssh` access to the target host (read-only via
  Tailscale ACL).
- Auditor does **not** need the age private key — substitution can be
  verified post-hoc through the audit log and the live process state.

**Procedure:**

1. **Inspect the apply audit log.** From the target host:

   ```bash
   tailscale ssh <home-lab-host>
   sudo tail -n 50 /var/log/smackerel/apply.log
   ```

   Each apply emits a single line with the form:

   ```
   <iso8601-timestamp> action=apply target=<target> source_sha=<sha> bundle_sha256=<sha> secrets_substituted=N placeholders_remaining=0
   ```

   `placeholders_remaining=0` is the auditor's confirmation that L2
   substitution succeeded for every declared key. **If
   `placeholders_remaining > 0`, the apply was aborted and no container
   started.**

2. **Inspect the resolved Compose env (live).** From the target host:

   ```bash
   tailscale ssh <home-lab-host>
   sudo docker compose -f <COMPOSE_DIR>/docker-compose.yml config | grep '__SECRET_PLACEHOLDER__'
   ```

   This MUST return zero matches. The adapter's step 9 pre-flight check
   already ran this before container start; this is the auditor's
   independent post-hoc confirmation.

3. **Inspect the live process env (no value extraction).** From the
   target host:

   ```bash
   tailscale ssh <home-lab-host>
   sudo docker exec smackerel-<env>-core sh -c 'env | cut -d= -f1 | sort'
   ```

   Confirm that `POSTGRES_PASSWORD`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`,
   `AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN` (or their
   currently-canonical names per `internal/config/secret_keys.go`) are
   present in the process environment. Auditors MUST NOT extract the
   values themselves; key-name presence is sufficient evidence.

4. **Inspect runtime logs for redaction compliance.** From the target
   host:

   ```bash
   tailscale ssh <home-lab-host>
   sudo docker logs smackerel-<env>-core 2>&1 | grep -E '__SECRET_PLACEHOLDER__|<known-canary-fragment>'
   ```

   This MUST return zero matches. The runtime's redaction contract
   (FR-051-007 extended by FR-052-007) names only the offending KEY in
   error messages, never the value or the placeholder marker. The
   contract test
   `internal/deploy/bundle_secret_contract_test.go::TestBundleSecretContract_AdversarialA2_LeakageDetector`
   asserts this property in CI; this step is the auditor's
   per-deployment confirmation.

5. **Cross-reference with the canonical manifest.** From the target
   host or any clone of the operator's overlay repo:

   ```bash
   git -C <smackerel-repo-clone> show HEAD:internal/config/secret_keys.go | grep -A20 'secretKeys = '
   git -C <smackerel-repo-clone> show HEAD:config/smackerel.yaml | grep -A10 'secret_keys:'
   ```

   The two lists MUST match exactly (same keys, same order). The unit
   test `internal/deploy/bundle_secret_contract_test.go::TestBundleSecretContract_NoLiteralSecretsInHomeLab`
   enforces this in CI; auditors MAY re-verify post-hoc.

**No-credential-required scope.** The full audit can be performed without
the age private key, without sops decryption, and without seeing any
secret value. The audit verifies: substitution happened, no placeholders
remain, no canary leaked to logs, manifest is consistent across surfaces.

## Connector Management

Connectors run on 5-minute sync cycles managed by the connector supervisor. Each connector maintains a cursor in the `sync_state` table for incremental syncing.

### Check Connector Status

```bash
# Via CLI
./smackerel.sh status

# Via API
curl -H "Authorization: Bearer <token>" http://127.0.0.1:40001/api/health
```

The health endpoint reports per-connector status: `healthy`, `syncing`, `error`, or `disconnected`.

### Trigger Immediate Sync

Via the web UI:
1. Open `http://127.0.0.1:40001/settings`
2. Click "Sync Now" next to the connector

Via API:
```bash
curl -X POST -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/settings/connectors/<connector-name>/sync
```

### Enable/Disable a Connector

1. Edit `config/smackerel.yaml` → set `connectors.<name>.enabled: true|false`
2. Regenerate config: `./smackerel.sh config generate`
3. Restart: `./smackerel.sh down && ./smackerel.sh up`

### QF Decisions Connector Operations

The `qf-decisions` connector is governed as a companion read surface for QuantitativeFinance. Current Scope 1 is implemented but not certified complete, and it is limited to explicit configuration, QF bridge read-contract validation, and connector health reporting. It does not yet publish QF artifacts, reset replay cursors, render QF packets, or export `PersonalEvidenceBundle`s.

| Operation | Requirement |
|-----------|-------------|
| Enablement | Keep `connectors.qf-decisions.enabled: false` unless a QF private read endpoint and credential are available and Scope 1 validation is being exercised |
| Required config when enabled | `base_url`, `credential_ref`, `sync_schedule`, `packet_version`, and `page_size` are mandatory; missing or invalid values fail validation |
| Connector health | Schema compatibility failures map to degraded health; authorization, reachability, and other bridge validation failures map to error health |
| Manual sync | A manual sync revalidates the QF read contract and publishes zero artifacts in current Scope 1 |
| Credential rotation | Rotate the QF service credential from `config/smackerel.yaml`, regenerate config, and restart the stack; cursor-preserving overlap behavior belongs to a later spec 041 scope |
| Later-scope operations | Cursor replay, artifact deduplication, packet surfacing, degraded packet rendering, and evidence export must not be treated as operationally available until their corresponding scopes are active and certified |

Smackerel operators must not use this connector to approve trades, alter QF mandates, submit execution requests, or rewrite QF-provided decision content.

### Google Maps Timeline Connector Operations (Spec 011)

The `google-maps-timeline` connector ingests Google Takeout Semantic Location
History exports as activity artifacts (walks, hikes, cycles, drives, transit,
runs), with trail-qualified activities (≥2km walk/hike/run/cycle) producing
enriched trail journal entries, GeoJSON route storage, dedup via
date+location-cluster hash, archive-after-processing, commute/trip detection,
and temporal-spatial linking. The connector is import-only — it makes no
external API calls and reads only the files placed in its configured
`import_dir`. Source code lives at `internal/connector/maps/`; schema lives at
`internal/db/migrations/001_initial_schema.sql:321-339` (location_clusters
table), consolidated from the historical
`internal/db/migrations/archive/009_maps.sql`.

| Operation | Requirement |
|-----------|-------------|
| Enablement | The connector ships disabled (`connectors.google-maps-timeline.enabled: false` in `config/smackerel.yaml`). Enable it only after the operator has populated `import_dir` and granted privacy consent for source `maps` (see "Privacy Consent Opt-In" below). |
| Required config when enabled | `import_dir` (must exist, must be a directory, must NOT be a symlink that can be retargeted), `sync_schedule` (cron string consumed by the supervisor), `watch_interval` (per-cycle scan interval, defaults to `5m` in `config/smackerel.yaml`), `archive_processed` (boolean — when `true`, processed files are moved into the `<import_dir>/archive/` subdirectory), `min_distance_m`, `min_duration_min`, `clustering.location_radius_m`, and the commute/trip/link tuning under the same connector key. All required values must be provided via `config/smackerel.yaml`; missing or zero/negative values fail loudly at `Connect()` time via `parseMapsConfig`. There are no Go-side fallback defaults. |
| Container mount | `deploy/compose.deploy.yml` and `docker-compose.yml` mount the host import directory read-only at `/data/maps-import` via the fail-loud `${MAPS_IMPORT_DIR:?Gate G028 / HL-RESCAN-012 — must be SST-emitted; run ./smackerel.sh config generate or ./smackerel.sh up}:/data/maps-import:ro` form. The deploy adapter MUST set `MAPS_IMPORT_DIR` explicitly in `app.env`; missing or empty values abort Compose at substitution time. |
| Connector health | `HealthDisconnected` (before `Connect()` succeeds), `HealthHealthy` (after `Connect()` and between sync cycles), `HealthSyncing` (during an active `Sync()` call), `HealthError` (import directory unreachable, consent check failed, oversized/unparseable file outside the per-cycle tolerance, or panic during sync — panics are recovered and reflected in health). |
| Observability — sync counts | Inherited from `internal/connector/supervisor.go`: each cycle emits `smackerel_connector_sync_total{connector="google-maps-timeline",status="success|error"}` and the connector-generic `ConnectorSyncFailureRateHigh24h` alert (in `config/prometheus/alerts.yml`) covers it. |
| Observability — structured logs | `slog.Info` on connect (`"google maps timeline connector connected"`), per-cycle (`"google maps timeline sync complete"`), and post-sync (`"post-sync patterns complete"`). `slog.Warn` for oversized files (>50MB warning, >200MB hard skip), unparseable files (skipped and marked processed to prevent retry loops), failed location-cluster inserts, pattern-detection step failures. No PII, no raw coordinates dumped. |
| Secret management | N/A — the connector is file-import only. No API keys, no OAuth, no `MAPS_*_TOKEN`/`MAPS_*_SECRET` env vars in any Compose surface. |
| Rollback | Disable: set `connectors.google-maps-timeline.enabled: false`, regenerate config, restart — the supervisor skips the connector on next cycle. Image rollback: `./smackerel.sh deploy-target <target> rollback` (pointer-swap, no rebuild). Schema rollback is not required — the `location_clusters` table is additive only. |

#### Privacy Consent Opt-In (R-401 Enforcement)

The Maps connector enforces opt-in consent at sync time. Until the operator
records explicit consent in the `privacy_consent` table for source `maps`, each
sync cycle logs `"maps sync skipped: user has not consented to maps data
collection"` and returns zero artifacts with the cursor unchanged. There is no
operator-facing API or UI for this preference yet; opt-in is performed via a
direct database write:

```bash
# Connect to PostgreSQL (password supplied via PGPASSWORD env or ~/.pgpass; never inline)
psql "postgres://smackerel@127.0.0.1:42001/smackerel"

# Grant consent for source 'maps'
INSERT INTO privacy_consent (source_id, consented, consented_at)
VALUES ('maps', TRUE, NOW())
ON CONFLICT (source_id) DO UPDATE SET consented = TRUE, consented_at = NOW();

# Verify
SELECT source_id, consented, consented_at FROM privacy_consent WHERE source_id = 'maps';
```

To revoke consent, set `consented = FALSE` for the same row; the connector
honors the change on its next sync cycle without restart.

#### End-To-End Operator Walkthrough

1. **Export from Google Takeout.** Visit `https://takeout.google.com/`, select
   only the **Location History** product, request a `.zip` export.
2. **Extract the Semantic Location History.** After download, unzip the archive
   and locate the directory `Takeout/Location History (Timeline)/Semantic
   Location History/`. The directory contains per-month folders (e.g.,
   `2026/`) with files named like `2026_APRIL.json`. The connector consumes
   `*.json` files anywhere under `import_dir` (it descends into
   subdirectories).
3. **Place files in `import_dir`.** Copy the JSON files (or the entire
   `Semantic Location History` subtree) into the host directory referenced by
   `MAPS_IMPORT_DIR`. The directory must already exist and be readable by the
   container user. In dev, the conventional path is
   `data/maps-import/` at the repo root (mounted at `/data/maps-import:ro`
   inside the container).
4. **Grant consent (one-time).** Run the SQL `INSERT` above. Skip this step if
   consent is already recorded.
5. **Enable the connector and restart.** Edit `config/smackerel.yaml` and set
   `connectors.google-maps-timeline.enabled: true`. Then run:

   ```bash
   ./smackerel.sh config generate
   ./smackerel.sh down
   ./smackerel.sh up
   ```

6. **Trigger an immediate sync (optional).** The supervisor runs the connector
   on its `sync_schedule` cron, but operators can force a cycle:

   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     http://127.0.0.1:40001/settings/connectors/google-maps-timeline/sync
   ```

7. **Verify ingestion succeeded.** Check connector health, sync counts, and
   container logs:

   ```bash
   # Per-connector health
   curl --max-time 5 -H "Authorization: Bearer <token>" \
     http://127.0.0.1:40001/api/health | jq '.connectors."google-maps-timeline"'

   # Sync counts (expected to increment on success)
   curl --max-time 5 http://127.0.0.1:40001/metrics 2>/dev/null | \
     grep 'smackerel_connector_sync_total{connector="google-maps-timeline"'

   # Container log lines for the most recent sync
   ./smackerel.sh logs core 2>&1 | \
     grep -E 'google maps timeline (connector connected|sync complete)|post-sync patterns complete'
   ```

   On a successful sync the logs include `"google maps timeline sync complete"`
   with `new_files`, `artifacts`, `trail_qualified`, and `errors` fields, plus
   `"post-sync patterns complete"` with `commute_patterns`, `trip_events`, and
   `links_created` fields.

8. **Confirm artifacts are searchable.** Activity artifacts appear under
   source `google-maps-timeline` and are searchable via the standard query
   surface. Trail-qualified hikes (≥2km walk/hike/run/cycle) include the
   `trail_journal` enrichment tier; routes are stored as GeoJSON LineString
   metadata.

#### Cursor And Re-Ingestion

The cursor is a pipe-delimited list of processed filenames pruned to entries
that still exist under `import_dir`. To re-process every file from scratch,
clear the cursor row for this connector (see "Reset a Connector's Sync
Cursor" below) and either remove archived copies or set
`archive_processed: false`. Re-ingestion is safe — the dedup hash on
date+location-cluster prevents duplicate artifacts.

#### Failure Modes And Operator Responses

| Symptom | Likely Cause | Operator Action |
|---------|--------------|-----------------|
| Health stuck on `disconnected` | `import_dir` does not exist, is not a directory, is a symlink that fails `EvalSymlinks`, or `Connect()` was never called. | Verify `MAPS_IMPORT_DIR` host path exists, is a real directory, and the resolved path is reachable by the container user; restart the stack. |
| Health flaps to `error` immediately after a sync | Per-file errors equal artifact count (e.g., every file is unparseable). | Inspect logs for `"permanently unparseable takeout file, skipping"` warnings; remove or fix the offending JSON; the connector marks unparseable files as processed to avoid infinite retry. |
| `sync_count` stays zero, log says `"maps sync skipped: user has not consented to maps data collection"` | `privacy_consent` row missing or `consented = FALSE`. | Apply the consent SQL `INSERT` shown above. |
| Log warns `"skipping oversized takeout file"` | A single JSON file exceeds the 200MB hard limit. | Split the export by month, re-export a smaller window from Takeout, or temporarily raise the limit in source (requires a code change and re-deploy — not configurable today). |
| Archive directory `archive/` not appearing | `archive_processed: false` (the shipped default). | Set `archive_processed: true` if archival is desired; processed files are moved into `<import_dir>/archive/` on a per-cycle basis after successful processing. Filename collisions are resolved by suffixing `_1`, `_2`, etc. |

### Notification Intelligence Operations (Spec 054)

The Notification Intelligence Handler is the source-neutral layer for
operational and personal notification events. Source adapters submit raw events
to the handler; the handler stores raw input, normalizes the event, classifies
severity/domain/intent, correlates incidents, chooses one handling decision,
records suppressions or approvals, and queues redacted output attempts.

The spec 055 ntfy adapter is the concrete source implementation for ntfy
topics. It owns ntfy stream/webhook transport, payload parsing, topic health,
reconnect state, dead-letter records, and replay controls. It still submits
accepted messages only through `SourceEventSink`; classification, incidents,
approvals, safe reactions, and output dispatch remain spec 054 core behavior.

#### Configuration And Startup

Notification intelligence configuration lives in `config/smackerel.yaml`:

| YAML path | Generated env var | Operator note |
|-----------|-------------------|---------------|
| `notification_intelligence.enabled` | `NOTIFICATION_INTELLIGENCE_ENABLED` | Enables the handler and operator API wiring. |
| `notification_intelligence.persistence_threshold` | `NOTIFICATION_PERSISTENCE_THRESHOLD` | Controls persistence-based escalation. |
| `notification_intelligence.escalation_severity` | `NOTIFICATION_ESCALATION_SEVERITY` | Controls severity-based escalation. Valid values are `medium`, `high`, and `critical`. |
| `notification_intelligence.low_confidence_threshold` | `NOTIFICATION_LOW_CONFIDENCE_THRESHOLD` | Low-confidence classifications choose diagnostics instead of fabricated certainty. |
| `notification_intelligence.max_retries` | `NOTIFICATION_MAX_RETRIES` | Bounded retry budget for notification handling/output. |
| `notification_outputs.channels` | `NOTIFICATION_OUTPUT_CHANNELS` | Output channels; the committed runtime uses `dashboard`. |
| `notification_sources.ntfy.instances` | `NTFY_SOURCES_JSON` | JSON array of explicit ntfy source instances consumed at Go startup. |

After editing the YAML, regenerate and restart through the repo CLI:

```bash
./smackerel.sh config generate
./smackerel.sh down
./smackerel.sh up
```

Missing or malformed notification config fails at config generation or at Go
startup. Do not edit `config/generated/*.env` by hand and do not add Compose or
shell fallback syntax for these values.

#### ntfy Source Configuration

Every ntfy source instance is declared under
`notification_sources.ntfy.instances` in `config/smackerel.yaml` and rendered
as `NTFY_SOURCES_JSON` by `./smackerel.sh config generate`. The Go runtime reads
that JSON at startup, registers enabled source instances in
`notification_source_instances`, starts configured adapters, and fails loudly if
the JSON is missing, malformed, or contains an enabled instance with invalid
required fields.

Required instance fields in the current adapter are:

| Field | Purpose |
|-------|---------|
| `source_instance_id` | Stable source identity used by health, raw events, normalized notifications, dead letters, replay attempts, and UI routes. |
| `enabled` | Enables or skips the instance at startup. |
| `source_form` / `transport_mode` | Must both be `stream` or both be `webhook`; the instance does not silently switch modes. |
| `endpoint_url` | Source endpoint used by the transport. For webhook mode in the committed local config, this is the internal core route. |
| `endpoint_ref_name` | Operator-safe endpoint identity shown in status/detail responses. |
| `topics` | Explicit topic allowlist; payloads from unconfigured topics are rejected and can be dead-lettered. |
| `auth_mode` | One of `none`, `bearer_token`, or `basic`. `none` must be explicit to allow zero `secret_ref_names`. |
| `secret_ref_names` | Secret reference names only for credential-backed modes; values never appear in status, logs, dead letters, or UI. |
| `default_domain`, `topic_subjects`, `tag_services`, `tag_intents` | Optional mapping hints. Core classification remains authoritative. |
| `retry_budget`, `initial_delay_seconds`, `max_delay_seconds`, `keepalive_timeout_seconds` | Bounded reconnect policy. All values must be positive and initial delay must not exceed max delay. |
| `lag_degraded_after_seconds`, `lag_disconnected_after_seconds` | Lag thresholds; degraded must be lower than disconnected. |
| `dead_letter_retry_budget`, `max_payload_bytes`, `pressure_threshold_count` | Dead-letter retry, payload ceiling, and pressure thresholds. |
| `display_name`, `endpoint_label`, `config_hash` | Redacted display and drift/audit metadata. |

The built-in HTTP stream transport accepts `auth_mode=none` directly. Credential-backed stream modes require a secret-resolved transport client; otherwise startup or connection fails instead of pretending an unauthenticated connection is valid. Webhook mode uses the authenticated Smackerel API route and the source's explicit metadata.

#### Source Health

Inspect all notification source instances and their latest redacted health:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/sources
```

Expected source states:

| State | Meaning | Operator action |
|-------|---------|-----------------|
| `connected` | Adapter has reported a successful event or check. | No action unless incidents are missing. |
| `degraded` | Transient failures are occurring; retry count and redacted error category should be present. | Inspect source credentials/connectivity outside Smackerel, then regenerate config if references changed. |
| `disconnected` | Auth, configuration, connectivity, or missing-health state prevents normal intake. | Check secret reference names, source instance identity, and source-specific adapter logs. |

Health responses include source type, source instance ID, source form, config
hash, secret reference names, redacted metadata, last event/check timestamps,
retry count, and redacted error text. Secret values are never returned.

#### ntfy Source Detail, Webhook, Reconnect, And Replay

Use the adapter-owned detail endpoint for topic state, lag, last accepted event
proof, and source/output boundary text:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/sources/<source_instance_id>/ntfy
```

For the committed local webhook source, a valid ntfy message-like event is sent
to the webhook route and accepted with `202 Accepted` only after the runtime
webhook receiver is registered:

```bash
curl --max-time 5 -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  --data '{"id":"evt-local-check","event":"message","topic":"home-lab-alerts","title":"Check","message":"source sink path"}' \
  http://127.0.0.1:40001/api/notifications/sources/ntfy-local-webhook/ntfy/webhook
```

The webhook route enforces the configured `max_payload_bytes` ceiling and
configured topics. Malformed JSON returns `invalid_ntfy_webhook_payload` and is
recorded as a dead-letter when the receiver is running. Topic mismatches return
`ntfy_webhook_rejected`.

Reconnect is an operator control that records reconnecting topic state and
degraded source health without creating a notification:

```bash
curl --max-time 5 -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  --data '{}' \
  http://127.0.0.1:40001/api/notifications/sources/<source_instance_id>/ntfy/reconnect
```

Dead-letter inspection supports bounded pagination:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  'http://127.0.0.1:40001/api/notifications/sources/<source_instance_id>/ntfy/dead-letters?limit=50'

curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/sources/<source_instance_id>/ntfy/dead-letters/<dead_letter_id>
```

Replay requires the explicit confirmation value `replay_through_source_sink`:

```bash
curl --max-time 5 -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  --data '{"confirmation":"replay_through_source_sink"}' \
  http://127.0.0.1:40001/api/notifications/sources/<source_instance_id>/ntfy/dead-letters/<dead_letter_id>/replay
```

The first accepted replay reconstructs an eligible source envelope and submits
it through `SourceEventSink`. If the same dead letter is replayed again after a
successful replay, the API returns the existing accepted attempt with
`already_replayed=true` and the same raw event ID; it does not create another
raw event, normalized notification, incident, decision, approval, delivery
attempt, or direct output.

Operator web pages mirror the same information:

| Page | Purpose |
|------|---------|
| `/notifications/sources` | Source list with ntfy rows, redacted auth mode, topic count, config hash, endpoint reference, and dead-letter link. |
| `/notifications/sources/{source_instance_id}` | ntfy health/detail page with topic lag, retry count, possible-gap flag, reconnect action, last accepted event proof, and boundary text. |
| `/notifications/sources/{source_instance_id}/dead-letters` | Dead-letter list with cause, replay status, payload hash, safe preview, and source-sink confirmation note. |
| `/notifications/sources/{source_instance_id}/dead-letters/{dead_letter_id}` | Dead-letter detail and replay confirmation guidance. |

#### Event History And Incidents

Use the event history endpoint for raw-to-normalized audit chains:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/events
```

Inspect one event detail:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/events/<event_id>
```

The detail response links the normalized notification, raw event,
classification, processing decision, and incident when present. Use it to verify
that raw input was stored before normalized output, that source event identity
is `source` or `handler_derived`, and that redaction state is present.

Inspect incidents:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/incidents
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/incidents/<incident_id>
```

Incident states are `observing`, `active`, `diagnosing`, `mitigating`,
`approval_requested`, `escalated`, `suppressed`, or `resolved`. Routine and
low-severity events should usually remain record-only; persistent or severe
events should correlate into one incident instead of producing repeated noise.

#### Suppressions And Quiet Windows

Inspect active or historical suppressions:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/suppressions
```

Suppression kinds are `dedupe`, `maintenance`, `cooldown`, `user_preference`,
`reaction_loop`, `policy`, and `quiet_window`. Suppressions preserve raw event
history; they only explain why user-facing output or action was withheld.

Inspect quiet-window status:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/quiet-windows
```

When no quiet-window records are active, the response is `{"quiet_windows": []}`.
Do not encode quiet-window policy in source adapters; policy belongs in the
core notification configuration and decision layer.

#### Approvals And Safe Reactions

High-blast-radius decisions are represented as `approval_request` decisions.
The core policy refuses destructive automatic actions, runs diagnostics only
when they are read-only, and permits autonomous actions only when the action is
low-risk and allowlisted by policy.

Inspect an approval:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/approvals/<approval_id>
```

Record an approval decision acknowledgement:

```bash
curl --max-time 5 -X POST -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/approvals/<approval_id>/decisions
```

If an event appears to require a destructive reaction, the correct outcome is a
refusal or a user-facing approval path, never automatic execution.

#### Output Delivery

Output channels are separate from source adapters. Dashboard delivery attempts
are visible through:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/outputs
```

Delivery statuses are `queued`, `sent`, `failed`, `withheld`, and
`retry_exhausted`. Each attempt includes the decision ID, incident ID when
present, output channel, destination reference, payload hash, redaction state,
status, redacted error details, attempted timestamp, and completion timestamp.

Use `/api/notifications/summary` for a compact incident/output overview:

```bash
curl --max-time 5 -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/api/notifications/summary
```

#### Redaction Checks

The handler redacts bearer tokens, query-string tokens/API keys/secrets,
password fragments, and `secret-*` / `secret_*` fragments before writing
operator-visible bodies, source metadata, health errors, and output messages.
If a sensitive string appears in a notification response, treat it as a P0 data
exposure: stop sending new source events, preserve the event ID for audit, and
fix the redaction path before reconnecting the source adapter.

#### Troubleshooting

| Symptom | Likely cause | Resolution |
|---------|--------------|------------|
| `missing or invalid required notification configuration` | One of the `NOTIFICATION_*` env vars was not generated or has an invalid value. | Edit `config/smackerel.yaml`, run `./smackerel.sh config generate`, then restart with `./smackerel.sh down` and `./smackerel.sh up`. |
| `ntfy notification sources: ... NTFY_SOURCES_JSON is required` | `notification_sources.ntfy.instances` was not rendered into the generated env file. | Restore the SST key in `config/smackerel.yaml`, run `./smackerel.sh config generate`, and restart. |
| Startup fails with `ntfy source config: ... is required` | An enabled ntfy instance is missing explicit identity, form/mode, endpoint identity, topic set, config hash, redacted metadata, or credential references for a credential-backed mode. | Fix the explicit source instance config in `config/smackerel.yaml`; use `auth_mode: "none"` only when no credential is intentionally required. |
| `/api/notifications/sources` shows `disconnected` with `no_health_report` | Source instance exists but no adapter has reported health yet. | Confirm `NTFY_SOURCES_JSON` contains an enabled instance, restart the core so `StartConfiguredAdapters` runs, and inspect the ntfy detail endpoint. |
| Source health is `degraded` | Transient source failures or rate limits. | Check the source system and credential references; health errors are redacted by category. |
| ntfy source detail shows `stalled`, `possible_gap=true`, or high lag | No event/open/keepalive/check has refreshed the topic before the configured lag threshold. | Inspect source connectivity, then use the reconnect route to record bounded reconnect state. Connected health returns only after a real source check or accepted event. |
| ntfy webhook returns `ntfy_webhook_receiver_unavailable` | The runtime receiver registry is not running or the source instance is not registered. | Restart the core after generating config; confirm the source is enabled and configured with `transport_mode: "webhook"`. |
| ntfy webhook returns `invalid_ntfy_webhook_payload` | Empty, malformed, oversize, or unparsable ntfy JSON payload. | Inspect the matching dead-letter record. Payload previews and causes are redacted; use payload hash and source instance/topic provenance for audit. |
| ntfy webhook returns `ntfy_webhook_rejected` | Payload was valid JSON but rejected by adapter mapping, commonly due to an unconfigured topic or unsupported event shape. | Confirm the topic is present in `notification_sources.ntfy.instances[].topics`; unsupported lifecycle-only events should update health, not create notifications. |
| Dead-letter replay returns `invalid_ntfy_replay_confirmation` | Request did not include the exact confirmation value. | Re-submit with `{"confirmation":"replay_through_source_sink"}`. |
| Dead-letter replay returns `ntfy_replay_failed` | Record is not replay eligible, cannot be parsed/mapped, or the source sink rejected it. | Inspect `ReplayEligible`, `ReplayStatus`, `ErrorKind`, and `ErrorRedacted` on the attempt; replay never bypasses the source sink. |
| Repeating a dead-letter replay returns `already_replayed=true` | The dead letter was already replayed successfully. | Treat the response as an idempotent repeat; use the returned attempt and raw event ID for audit instead of replaying through another path. |
| Events are stored but no output is queued | Event was routine, low severity, suppressed, below thresholds, or confidence selected diagnostics. | Inspect `/api/notifications/events/<event_id>` for classification, decision reason codes, suppressions, and incident state. |
| Repeated alerts appear as one incident | Correlation is working as designed. | Inspect the incident `persistence_count` and source instance IDs to confirm related events are grouped. |
| Approval expected but event was only recorded | Risk level or severity/persistence thresholds did not select `approval_request`. | Inspect decision `threshold_inputs`, `risk_assessment`, and classification confidence in the event detail. |
| Output remains queued | Output channel is not completing delivery or the dispatcher has not sent it yet. | Check `/api/notifications/outputs` for status and redacted errors; verify `notification_outputs.channels` includes the intended channel. |

### Reset a Connector's Sync Cursor

If a connector is stuck or you want to re-sync from scratch, clear its cursor in the database. This requires the stack to be running:

```sql
-- Connect to PostgreSQL (password via PGPASSWORD env or ~/.pgpass; never inline)
-- psql "postgres://smackerel@127.0.0.1:42001/smackerel"

-- View current cursors
SELECT connector_id, cursor, last_sync_at FROM sync_state;

-- Reset a specific connector
UPDATE sync_state SET cursor = '' WHERE connector_id = '<connector-name>';
```

The connector will re-sync from the beginning on its next cycle. Existing artifacts are deduplicated by content hash, so duplicates are not created.

### Import Bookmarks

```bash
# Via API
curl -X POST -H "Authorization: Bearer <token>" \
  -F "file=@bookmarks.json" \
  http://127.0.0.1:40001/api/bookmarks/import
```

Or via the web UI at `http://127.0.0.1:40001/settings` → Import Bookmarks.

### Google Keep live sync

> **Spec reference:** [`specs/059-google-keep-live-mode`](../specs/059-google-keep-live-mode/).
> Cross-references: [`docs/Deployment.md`](Deployment.md) for bundle redeploy
> mechanics, and specs [`051`](../specs/051-bundle-secret-baseline/), [`052`](../specs/052-bundle-secret-substitution/),
> [`054-notification-intelligence-escalation`](../specs/054-notification-intelligence-escalation/) for
> the secret-bundle and notification escalation primitives this runbook reuses.

#### Overview

Live sync polls Google Keep through the Python ML sidecar's `gkeepapi`
bridge over NATS request/reply. `gkeepapi` is an **unofficial** library
that drives Google's internal Keep protocol — it can break at any time
when Google ships an unannounced change. The drift circuit breaker
trips after four consecutive malformed sidecar responses and refuses
all further sync calls until an operator rotates `drift_ack_token`
(see "Recovering from a tripped breaker" below). There is no CLI verb
and no HTTP endpoint that acknowledges drift; rotation is the only
path.

#### Deprecation Path

This entire live-sync surface is provisional. When Google publishes a
stable Keep API, the connector will switch to it; `note_id` continuity
will be preserved across the migration. The fields `warning_acknowledged`,
`drift_ack_token`, and the secret `KEEP_GOOGLE_APP_PASSWORD` will be
retired on the same schedule. Treat any runbook step that mentions
these names as time-limited.

#### Prerequisites

1. A Google account with **2-Step Verification** enabled. Live sync
   requires an App Password, which Google issues only to accounts that
   already have 2SV turned on.
2. Generate an App Password from the Google Account security page
   (look for the "App passwords" entry under 2-Step Verification).
   Treat the generated value as a secret of the same sensitivity as
   the account password itself.

#### Initial enablement

1. Add the App Password to the deploy-overlay sops-encrypted secret
   bundle as `KEEP_GOOGLE_APP_PASSWORD`. The deploy adapter substitutes
   it into the resolved app.env at apply time; the value never lands
   in this repo or in `config/generated/`.
2. Set `KEEP_GOOGLE_EMAIL` in `config/smackerel.yaml` to the account
   email (use a generic placeholder such as `<operator-email>` in any
   documentation you derive from this runbook — never inline a real
   value).
3. Configure the connector block in `config/smackerel.yaml`:

   ```yaml
   connectors:
     google-keep:
       sync_mode: gkeepapi           # or hybrid for Takeout + live
       gkeep_enabled: true
       warning_acknowledged: true    # required acknowledgement
       drift_ack_token: ""           # leave empty until first trip
       poll_interval: 30m            # minimum 15m enforced
   ```

4. Regenerate the env bundle:

   ```bash
   ./smackerel.sh config generate --env <env> --bundle --source-sha <sha>
   ```

5. Deploy to the target:

   ```bash
   ./smackerel.sh deploy-target <target> apply \
       --image-core=sha256:<digest> --image-ml=sha256:<digest> \
       --config-bundle=<env>-<sha> --config-bundle-sha=<sha256-hex>
   ```

6. Verify health:

   ```bash
   ./smackerel.sh status
   ./smackerel.sh logs
   ```

   Look for `gkeepapi sidecar handshake ok` at INFO. A failed handshake
   surfaces the sidecar's verbatim error string and sets the connector
   to `health=error`.

#### Recovering from a tripped breaker

When `smackerel_keep_protocol_drift_detected_total` increments, the
breaker is OPEN and all sync calls return early without contacting the
sidecar.

1. Inspect logs for the drift signature:

   ```bash
   ./smackerel.sh logs | grep keep_protocol_drift
   ```

   The `reason` field tells you whether the drift was schema mismatch,
   transport failure, or a non-auth sidecar error.

2. If the failure is a schema mismatch, consider bumping the
   `gkeepapi` pin in `ml/requirements.txt` to the latest release that
   upstream confirms still works; rebuild the ML image.

3. Rotate `drift_ack_token` in `config/smackerel.yaml` to any new
   non-empty string (`<any-non-empty-string>`):

   ```yaml
   connectors:
     google-keep:
       drift_ack_token: "ack-<incrementing-counter>"
   ```

4. Regenerate the env bundle (`./smackerel.sh config generate --env
   <env> --bundle --source-sha <sha>`).

5. Redeploy using the standard pointer-swap apply (see
   [`docs/Deployment.md`](Deployment.md) — do not duplicate those
   mechanics here):

   ```bash
   ./smackerel.sh deploy-target <target> apply \
       --image-core=sha256:<digest> --image-ml=sha256:<digest> \
       --config-bundle=<env>-<sha> --config-bundle-sha=<sha256-hex>
   ```

6. Verify recovery:

   ```bash
   ./smackerel.sh status
   ./smackerel.sh logs | grep keep_sync_response
   ```

   `Health()` returns to `healthy` after the next successful sync.

Note explicitly: there is no `./smackerel.sh` verb and no HTTP endpoint
that acknowledges drift. Rotation + redeploy is the only path.

#### Rotating the App Password

To rotate the secret without changing connector behaviour, repeat
steps 2 and 5–7 of "Initial enablement": generate a new App Password,
update the sops-encrypted secret bundle, regenerate the env bundle,
redeploy, and verify health.

#### What you must NOT do

- Do not run ad-hoc container-runtime, Python test runner, Go test
  runner, or Python interpreter commands as part of operating this
  connector. Use only `./smackerel.sh`.
- Do not hand-edit `config/generated/*.env`. Those files are derived
  artifacts; the next `./smackerel.sh config generate` will overwrite
  them.
- Do not introduce language-level fallback values for
  `KEEP_GOOGLE_EMAIL` or `KEEP_GOOGLE_APP_PASSWORD`. The repo enforces
  fail-loud SST (see
  [`.github/instructions/smackerel-no-defaults.instructions.md`](../.github/instructions/smackerel-no-defaults.instructions.md)).
- Do not commit a real email, hostname, or App Password to this repo
  or to any spec/doc derived from this runbook. Use generic
  placeholders such as `<operator-email>` and `<any-non-empty-string>`.

## Troubleshooting

### Error Lookup Table

| Error Message | Cause | Resolution |
|---------------|-------|------------|
| `NATS connection refused` | NATS container not running or not healthy | Run `./smackerel.sh up` and wait for health checks. Check `./smackerel.sh logs` for NATS errors |
| `ping database: connection refused` | PostgreSQL not running or not ready | Run `./smackerel.sh up`. PostgreSQL has a 5-second health check interval — wait for it to become healthy |
| `execute migration NNN: ...` | Migration SQL error — schema conflict or corrupt state | Check the specific migration file in `internal/db/migrations/`. Look for conflicting manual schema changes |
| `ML sidecar unhealthy` | Python ML sidecar not ready (120s startup period for model loading) | Wait 2 minutes after startup. Check `./smackerel.sh logs` for ML sidecar errors. Verify LLM API key is set |
| `LLM call failed: timeout` | LLM provider rate limit or network issue | Check `LLM_API_KEY` in config. Verify provider status. For Ollama, ensure the model is downloaded |
| `SMACKEREL_AUTH_TOKEN rejected: known placeholder` | Auth token is still set to the default placeholder value | Set a real token in `config/smackerel.yaml` → `runtime.auth_token` and regenerate config |
| `missing required configuration: ...` | One or more required environment variables not set | Run `./smackerel.sh config generate` to regenerate env files from `config/smackerel.yaml`. Check that all required fields are populated |
| `bookmarks connector not connected: import directory not configured` | Bookmarks connector enabled but import directory doesn't exist | Create `data/bookmarks-import/` or disable the connector |
| `acquire migration lock` | Another instance is running migrations concurrently | Wait for the other instance to finish. If stuck, check for leaked advisory locks in PostgreSQL |
| `401 Unauthorized` | Missing or invalid Bearer token in API request | Include `Authorization: Bearer <token>` header. Token must match `runtime.auth_token` in config |
| `SSRF: blocked request to private IP` | Capture endpoint received a URL pointing to a private/internal IP | This is a security protection. Only public URLs are allowed for capture |
| `token endpoint returned 4xx for google` | OAuth2 token exchange failed — expired or revoked credentials | Re-authorize at `http://127.0.0.1:40001/auth/google/start`. Check client_id and client_secret in config |
| `synthesis subscriber: create consumer` | NATS stream not created or misconfigured | Check NATS config in `config/generated/nats.conf`. Restart NATS: `./smackerel.sh down && ./smackerel.sh up` |

### Checking Logs

```bash
# All services
./smackerel.sh logs

# Specific service (read-only, allowed per terminal discipline)
docker logs smackerel-core 2>&1
docker logs smackerel-ml 2>&1
docker logs smackerel-postgres 2>&1
docker logs smackerel-nats 2>&1
```

### Service Won't Start

1. Check config: `./smackerel.sh check`
2. Regenerate config: `./smackerel.sh config generate`
3. Check logs: `./smackerel.sh logs`
4. Clean and rebuild: `./smackerel.sh clean smart && ./smackerel.sh build && ./smackerel.sh up`

### ML Sidecar Slow to Start

The ML sidecar loads the `all-MiniLM-L6-v2` embedding model on startup. This takes up to 120 seconds (the `start_period` in docker-compose.yml). The core service waits for ML readiness before processing artifacts (configurable via `ml_readiness_timeout_s` in `config/smackerel.yaml`).

### NATS JetStream Issues

Check NATS monitoring dashboard at `http://127.0.0.1:42003` for stream and consumer status. Verify streams exist:

```bash
curl http://127.0.0.1:42003/jsz
```

## Backup & Restore

Smackerel ships a product-owned backup and restore contract (spec 048). The
runtime owns the dump command, retention pruning, and a JSON status file
the metrics watcher republishes as Prometheus gauges; the deploy adapter
overlay owns scheduling (systemd timer or cron) and any off-host shipping
destination (S3, BackBlaze, rclone-to-NFS, etc.).

### Contract Summary

| Concern | Owner | Surface |
|---------|-------|---------|
| Dump command + retention + status file | Smackerel runtime | `./smackerel.sh backup` |
| Disposable restore drill | Smackerel runtime | `./smackerel.sh backup-restore-test` |
| Status JSON schema | Smackerel runtime | `${BACKUP_STATUS_FILE}` (default `./backups/.backup-status.json`) |
| Metrics + alert | Smackerel runtime | `smackerel_backup_last_success_unixtime`, `smackerel_backup_size_bytes`, `smackerel_backup_runs_total{status}`; alert `SmackerelBackupStale` |
| Schedule (timer/cron) | Deploy adapter | `<adapter>/timers/smackerel-backup.timer` |
| Off-host destination | Deploy adapter | `${BACKUP_DESTINATION_URL}` written by adapter to `app.env`; never committed here |

### SST Keys

All five keys are required (`./smackerel.sh config generate` fails loud if any are missing):

| Key | smackerel.yaml path | Default | Meaning |
|-----|---------------------|---------|---------|
| `BACKUP_LOCAL_DIR` | `backup.local_dir` | `./backups` | Directory where artifacts and the status file live. |
| `BACKUP_STATUS_FILE` | `backup.status_file` | `./backups/.backup-status.json` | JSON file written by `backup.sh` and polled by the core watcher. |
| `BACKUP_RETENTION_DAILY` | `backup.retention_daily` | `7` | Distinct-day daily slots (newest of each day kept). |
| `BACKUP_RETENTION_WEEKLY` | `backup.retention_weekly` | `4` | Distinct-ISO-week weekly slots claimed past the daily window. |
| `BACKUP_WATCHER_POLL_SECONDS` | `backup.watcher_poll_seconds` | `60` | Core poll interval for the status file. |

### Retention Policy (FR-048-001)

`./smackerel.sh backup` keeps:

- The newest backup for each of the last `BACKUP_RETENTION_DAILY` distinct calendar days (default 7).
- Then, the newest backup in each of the next `BACKUP_RETENTION_WEEKLY` distinct ISO weeks past the daily cutoff (default 4). Weekly slots never overlap weeks already covered by daily slots — when the history is dense (one artifact per day), the weekly slots advance until a fresh ISO week is found.
- Multiple backups on the same calendar day collapse to one daily slot (the newest); older same-day copies are pruned immediately.

Pure retention logic lives in `internal/backup/retention.go` (unit-tested by `retention_test.go`); the script `scripts/commands/backup.sh` re-implements the same algorithm in Python so cron-only environments without the Go binary still prune correctly.

### Status File Schema

`scripts/commands/backup.sh` writes JSON atomically (`<file>.tmp` → rename) after every run:

```json
{
  "schema_version": 1,
  "last_run_unixtime": 1747169400,
  "last_success_unixtime": 1747169400,
  "last_status": "success",
  "last_duration_ms": 4123,
  "last_size_bytes": 18432123,
  "last_artifact_name": "smackerel-2026-05-13-233000.sql.gz",
  "last_error": ""
}
```

On failure `last_status` becomes `"failed"`, `last_success_unixtime` keeps the prior success value, and `last_error` carries a redacted error string — `POSTGRES_PASSWORD`, `SMACKEREL_AUTH_TOKEN`, `TELEGRAM_BOT_TOKEN`, and other secret env values are scrubbed by `redact_secrets()` before the file is written (FR-048-003).

### Metrics & Alert

`internal/backup.Watcher` polls `${BACKUP_STATUS_FILE}` every `${BACKUP_WATCHER_POLL_SECONDS}` and republishes:

| Metric | Type | Source field |
|--------|------|--------------|
| `smackerel_backup_last_success_unixtime` | Gauge | `last_success_unixtime` |
| `smackerel_backup_size_bytes` | Gauge | `last_size_bytes` |
| `smackerel_backup_runs_total{status="success"\|"failed"}` | Counter | incremented on every new `last_run_unixtime` |

When no status file exists yet, the gauges read 0 — the `SmackerelBackupStale` alert (`config/prometheus/alerts.yml`) fires because `time() - 0 > 90000`, which is the correct behavior on a brand-new host that has never produced a backup.

### Restore Drill (FR-048-002)

```bash
./smackerel.sh backup-restore-test --backup-file backups/smackerel-2026-05-13-233000.sql.gz
```

The drill:

1. Starts a disposable `pgvector/pgvector:pg16` container with `--tmpfs /var/lib/postgresql/data` (no published host port, no named volume).
2. Pipes the gunzipped backup through `psql` inside the container.
3. Asserts `schema_migrations` is non-empty, `sync_state` is reachable (the canonical connector cursor store), and the `vector` extension is present.
4. Scans psql stdout/stderr for any secret-shaped value from the closed redaction set and fails the run if any leaks through.
5. Tears the container down unconditionally on exit (success, failure, or `Ctrl-C`).

If you do not pass `--backup-file`, the drill picks the newest `smackerel-*.sql.gz` in `${BACKUP_LOCAL_DIR}`.

### Operator Workflow

```bash
# Create a fresh backup (writes artifact + updates status file + applies retention).
./smackerel.sh backup

# Verify the artifact actually restores cleanly into a throwaway postgres.
./smackerel.sh backup-restore-test

# Inspect the status file the metrics watcher reads.
cat backups/.backup-status.json

# In Prometheus, the staleness alert fires when no success has landed in 25h.
# Replay the alert query directly:
curl -s 'http://127.0.0.1:42007/api/v1/query?query=(time()%20-%20smackerel_backup_last_success_unixtime)'
```

### Manual / Ad-Hoc Backup Operations

For one-off operator workflows outside the spec 048 automated path (custom
formats, piping to remote storage, partial-table dumps), use the underlying
`pg_dump` directly. The automated path above remains the supported
production contract.

```bash
# Plain SQL (while stack is running)
docker exec smackerel-postgres pg_dump -U smackerel smackerel > backup.sql

# Or compressed custom format
docker exec smackerel-postgres pg_dump -U smackerel -Fc smackerel > backup.dump
```

### PostgreSQL Restore

```bash
# Stop the stack
./smackerel.sh down

# Start only PostgreSQL
docker compose -p smackerel up -d postgres

# Wait for PostgreSQL to be ready, then restore
docker exec -i smackerel-postgres psql -U smackerel smackerel < backup.sql

# Or from compressed backup
docker exec -i smackerel-postgres pg_restore -U smackerel -d smackerel < backup.dump

# Restart full stack
./smackerel.sh up
```

### Volume Backup

Docker named volumes store persistent data. To back up:

```bash
# List volumes
docker volume ls | grep smackerel

# Backup a volume
docker run --rm -v smackerel-dev-postgres-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/postgres-data.tar.gz -C /data .

# Restore a volume
docker run --rm -v smackerel-dev-postgres-data:/data -v $(pwd):/backup alpine \
  sh -c "rm -rf /data/* && tar xzf /backup/postgres-data.tar.gz -C /data"
```

### What to Back Up

| Data | Location | Frequency |
|------|----------|-----------|
| PostgreSQL database | `smackerel-dev-postgres-data` volume | Daily |
| Configuration | `config/smackerel.yaml` | On change |
| Import data | `data/bookmarks-import/`, `data/maps-import/`, `data/twitter-archive/` | On change |
| NATS JetStream | `smackerel-dev-nats-data` volume | Weekly (messages are replayable) |

## Monitoring

### Health Checks

All containers have health checks configured in docker-compose.yml:

| Service | Health Check | Interval | Start Period |
|---------|-------------|----------|-------------|
| PostgreSQL | `pg_isready` | 5s | — |
| NATS | HTTP `/healthz` | 5s | 5s |
| smackerel-core | HTTP `/api/health` | 10s | 15s |
| smackerel-ml | HTTP `/health` | 10s | 120s |
| ollama (optional) | HTTP `/api/tags` | 10s | 30s |

### Key Metrics

#### Go Core (`http://127.0.0.1:40001/metrics`)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_artifacts_ingested_total` | Counter | `source`, `type` | Artifact ingestion by source and type |
| `smackerel_capture_total` | Counter | `source` | Capture request count (telegram, api, extension, pwa) |
| `smackerel_search_latency_seconds` | Histogram | `mode` | Search latency distribution |
| `smackerel_domain_extraction_total` | Counter | `schema`, `status` | Domain extraction attempts |
| `smackerel_connector_sync_total` | Counter | `connector`, `status` | Connector sync success/failure |
| `smackerel_nats_deadletter_total` | Counter | `stream` | Messages routed to dead letter |
| `smackerel_db_connections_active` | Gauge | — | Active database connections |
| `smackerel_digest_generation_total` | Counter | `status` | Digest generation (published, fallback, quiet) |
| `smackerel_intelligence_latency_seconds` | Histogram | `endpoint` | Spec 006 Phase 5 intelligence endpoint latency (buckets 0.05..30s) |
| `smackerel_intelligence_errors_total` | Counter | `endpoint` | Spec 006 Phase 5 intelligence endpoint errors per endpoint |
| `smackerel_alerts_produced_total` | Counter | `type` | Alerts created by producers by type (spec 006 R-504 subscription, relationship-cooling, etc.) |
| `smackerel_alerts_delivered_total` | Counter | `type` | Alerts delivered via Telegram by type (spec 006 R-504 subscription alerts flow through here) |
| `smackerel_alert_delivery_failures_total` | Counter | — | Alert delivery failures (Telegram send or mark-delivered) |

#### Recommendations (Spec 039 Scope 6)

The recommendation runtime exposes eight Prometheus metrics with bounded labels (no `watch_id`, `recommendation_id`, `request_id`, `trace_id`, or `actor_user_id` labels). Per-watch operator visibility is provided by joining the bounded `*_watch_runs_total` metric with the persisted `recommendation_watch_runs` table on `watch_id` — never by embedding the watch id as a Prometheus label.

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_recommendation_provider_requests_total` | Counter | `provider`, `category`, `outcome` | Provider request count by outcome (success, error, timeout, degraded) |
| `smackerel_recommendation_provider_latency_seconds` | Histogram | `provider`, `category` | Provider call latency distribution (buckets 0.05..30s) |
| `smackerel_recommendation_candidates_total` | Counter | `category`, `stage`, `outcome` | Candidate counts at each ranking/dedupe/policy stage |
| `smackerel_recommendation_watch_runs_total` | Counter | `kind`, `outcome` | Watch evaluation runs by kind and outcome |
| `smackerel_recommendation_delivery_total` | Counter | `channel`, `outcome` | Delivery outcomes per channel (telegram, digest, drop) |
| `smackerel_recommendation_suppression_total` | Counter | `reason` | Recommendations suppressed by policy/quiet hours/rate limit |
| `smackerel_recommendation_ranking_confidence_total` | Counter | `confidence` | Distribution of ranking confidence bands |
| `smackerel_recommendation_location_precision_total` | Counter | `requested`, `sent` | Requested vs. sent location precision (privacy reduction) |

**Operator audit view:** `GET /recommendations/watches/{id}` renders a `data-testid="watch-audit-counts"` block sourced from `recommendation_watch_runs` (data-source marker on the section). Use this surface — not Prometheus — for per-watch run counts.

#### Surfacing Metrics (Spec 078)

The cross-surface surfacing controller (`internal/intelligence/surfacing/`) is the single decision point all 7 intelligence producers consult before dispatching a nudge. It exposes 8 bounded-cardinality Prometheus families via `internal/metrics/surfacing.go`. Architecture and contract are documented in [`docs/Architecture.md`](Architecture.md#cross-surface-surfacing-controller-spec-078).

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_surfacing_nudges_delivered_total` | Counter | `producer`, `channel` | Nudges that passed the full pipeline and were delivered |
| `smackerel_surfacing_acted_on_total` | Counter | `producer`, `channel` | Delivered nudges the user acted on (recorded by ack signal) |
| `smackerel_surfacing_false_positive_total` | Counter | `producer`, `channel` | Delivered nudges the user marked as unwanted |
| `smackerel_surfacing_dedupe_total` | Counter | `producer` | Candidates collapsed by the dedupe window |
| `smackerel_surfacing_suppression_total` | Counter | `reason` | Candidates suppressed because the user acked the content key recently |
| `smackerel_surfacing_budget_overrides_total` | Counter | `reason` | Candidates that escalated past budget (p1 + time-critical + `urgent_escalation_enabled`) |
| `smackerel_surfacing_deferred_budget_exhausted_total` | Counter | `producer` | Candidates dropped because the daily nudge budget was exhausted with no escalation |
| `smackerel_surfacing_budget_remaining` | Gauge | — | Remaining slots in the current day's budget |

**Alerting guidance.** Recommended thresholds for operators wiring Prometheus alerts:

- **Budget exhaustion** — alert if `surfacing_deferred_budget_exhausted_total` increases for more than two consecutive days, or if `surfacing_budget_remaining` stays at 0 for > 6 hours. Either signals `surfacing.daily_nudge_budget` is too low for actual load.
- **Dedupe storm** — alert if `rate(surfacing_dedupe_total[15m])` exceeds the same window's `rate(surfacing_nudges_delivered_total[15m])` by more than 5×. Producers are spamming the same `ContentKey` — investigate the offending producer label.
- **Suppression spike** — alert if `rate(surfacing_suppression_total[1h])` jumps > 10× the trailing 24h baseline. Either the user is acknowledging a broad class of nudges (legitimate) or a producer is regressing on relevance.

Cardinality is bounded by the closed `Producer` and `Channel` enums in `internal/intelligence/surfacing/types.go`; adding a new value is a deliberate code change. `ContentKey` is never exposed as a label (PII-safe).



**Log/trace redaction:** All serialized recommendation logs and traces are scanned for forbidden substrings (provider API keys, raw provider payloads, sensitive graph prompt text, raw GPS coordinates) at the persistence boundary via `internal/recommendation/store.AssertRedactSafe`. The unit test `internal/recommendation/store/redact_test.go::TestRecommendationRedaction_NoSecretsOrRawLocationInLogsOrTraces` is the regression guard.

#### ML Sidecar (`http://127.0.0.1:40002/metrics`)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_llm_tokens_used_total` | Counter | `provider`, `model` | LLM token usage per provider/model |
| `smackerel_ml_processing_latency_seconds` | Histogram | `operation` | ML processing latency per operation |

Model label cardinality is bounded: known models pass through, unknown models map to `other`.

### Scheduled Intelligence Jobs (Spec 006 Phase 5)

The Go core scheduler (`internal/scheduler/scheduler.go::scheduleEngineJobs`) registers the following cron-driven jobs that own delivery for spec 006 Phase 5 features. Schedules are hardcoded (only `digest_cron` is operator-configurable via `config/smackerel.yaml::runtime.digest_cron`). Operators verify health by joining the metric counters above (e.g. `smackerel_intelligence_errors_total{endpoint=...}` and `smackerel_alerts_produced_total{type=...}`) with the Go core JSON logs.

| Job | Cron schedule | Spec 006 requirement | Runtime entrypoint | Observation surface |
|-----|---------------|----------------------|--------------------|---------------------|
| `synthesis` | `0 2 * * *` (daily 02:00) | R-501 (expertise mapping), R-502 (learning paths), R-503 (content creation fuel) | `Scheduler.runSynthesisJob` → `engine.RunSynthesis(ctx)` + `engine.CheckOverdueCommitments(ctx)`; 5-minute per-tick timeout | `smackerel_intelligence_latency_seconds{endpoint="synthesis"}`, `smackerel_intelligence_errors_total{endpoint="synthesis"}`, structured log `synthesis complete insights=<N>` (success) or `synthesis failed error=<...>` (error) |
| `resurfacing` | `0 8 * * *` (daily 08:00) | R-505 (serendipity engine) | `Scheduler.runResurfacingJob` → `engine.Resurface(ctx, 5)` + `engine.MarkResurfaced(ctx, ids)`; 2-minute per-tick timeout | `smackerel_intelligence_errors_total{endpoint="resurfacing"}`, structured log `resurfaced artifacts delivered count=<N>` (success) or `resurfacing failed error=<...>` (error) |
| `monthly report` | `0 3 1 * *` (1st of month 03:00) | R-506 (monthly self-knowledge report) | `Scheduler.runMonthlyReportJob` → `engine.GenerateMonthlyReport(ctx)`; 5-minute per-tick timeout | `smackerel_intelligence_errors_total{endpoint="monthly_report"}`, structured logs `monthly report generated month=<...> words=<N>` and `monthly report delivered via Telegram month=<...>` (success) or `monthly report generation failed error=<...>` (error) |
| `subscription detection` | `0 3 * * 1` (Mondays 03:00) | R-504 (subscription tracking) | `Scheduler.runSubscriptionDetectionJob` → `engine.DetectSubscriptions(ctx)`; 2-minute per-tick timeout | `smackerel_alerts_produced_total{type="subscription"}`, `smackerel_alerts_delivered_total{type="subscription"}`, structured log `subscription detection complete detected=<N>` (success) or `subscription detection failed error=<...>` (error) |
| `frequent lookup detection` | `0 4 * * *` (daily 04:00) | R-507 (repeated lookups → quick refs) | `Scheduler.runFrequentLookupsJob` → `engine.DetectFrequentLookups(ctx)` + `engine.CreateQuickReference(...)` (capped at 5 per run) + `engine.PurgeOldSearchLogs(ctx, 60)`; 2-minute per-tick timeout | `smackerel_intelligence_errors_total{endpoint="frequent_lookups"}`, structured logs `frequent lookup detection complete detected=<N>`, `quick reference auto-created concept=<...> lookups=<N>`, `search log purged rows_deleted=<N>` (success) or `frequent lookup detection failed error=<...>` (error) |

**Failure recovery.** All five jobs are stateless re-entrant batch sweeps — re-run safely by waiting for the next cron tick or by restarting the Go core (`./smackerel.sh down && ./smackerel.sh up`). There is no manual trigger HTTP endpoint; missed runs catch up on the next scheduled tick (no backfill). Spec 006 design treats one missed run as an acceptable degradation per the "Knowledge Breathes" lifecycle principle.

**Related shared scheduler jobs.** The scheduler also registers `pre-meeting briefs` (every 5 minutes), `weekly synthesis` (Sundays 16:00), `alert delivery sweep` (every 15 minutes), `daily alert production` (06:00), and `relationship cooling alert production` (Mondays 07:00). Those jobs are owned by the spec 004 intelligence layer and the spec 021 intelligence-delivery layer; see `internal/scheduler/scheduler.go::scheduleEngineJobs` for the live registration list.

### OpenTelemetry Tracing (Opt-in)

Distributed trace context propagation through NATS messages is available but disabled by default. To enable:

1. Set `observability.otel_enabled: true` in `config/smackerel.yaml`
2. Optionally set `observability.otel_exporter_endpoint` to an OTLP gRPC collector (e.g., `http://localhost:4317`)
3. Regenerate config: `./smackerel.sh config generate`
4. Restart: `./smackerel.sh down && ./smackerel.sh up`

When enabled, trace context is propagated via W3C `traceparent` headers in NATS messages between Go core and ML sidecar. When disabled, there is zero overhead.

### NATS Monitoring

The NATS monitoring endpoint at `http://127.0.0.1:42003` provides:
- `/varz` — general server info
- `/connz` — connection details
- `/jsz` — JetStream stream and consumer status
- `/healthz` — health status

## Monitoring Stack

> **Spec 049 — Prometheus profile.** This section is the
> product-owned contract for the bundled Prometheus monitoring
> stack. It documents what Smackerel ships, what dashboards and
> alerts the runtime supports, and where the boundary sits between
> the product repo and the deploy-adapter overlay. Reverse-proxy
> fronting, Alertmanager receivers, Grafana provisioning, and
> TLS termination are deliberately OUT of scope here — they belong
> in the deploy-adapter overlay so the product repo stays
> target-agnostic.

### How To Enable

Prometheus is shipped as an OPT-IN Docker Compose profile. The
runtime stack does NOT include it by default so the bundled image
footprint stays minimal.

```bash
# Enable monitoring alongside the runtime stack
docker compose --profile monitoring up -d

# Inspect Prometheus directly (dev only; bound to 127.0.0.1 per Spec 042)
curl -s http://127.0.0.1:42005/-/healthy
curl -s http://127.0.0.1:42005/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'
```

On home-lab / production deployments the deploy adapter sets
`HOST_BIND_ADDRESS` (typically the tailnet IP) so Prometheus
becomes reachable on the overlay network. The fail-loud
substitution `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set
by deploy adapter}` in `deploy/compose.deploy.yml` makes
`docker compose up` abort at substitution time if the adapter
forgot to set it.

The scrape config rendered from
`config/prometheus/prometheus.yml.tmpl` targets two product-owned
endpoints by compose service name:

| Job | Target | Source |
|-----|--------|--------|
| `smackerel-core` | `smackerel-core:${CORE_CONTAINER_PORT}` | Go core router (chi) — `/metrics` (spec 030) |
| `smackerel-ml` | `smackerel-ml:${ML_CONTAINER_PORT}` | Python FastAPI sidecar — `/metrics` (spec 050) |

### Dashboard Inventory

Dashboard JSON is NOT shipped from this repo (deploy-adapter
overlay responsibility — different operators provision Grafana
differently). The inventory below is the canonical list of
dashboards the runtime metrics support, with the backing metrics
each dashboard MUST be built from. Any future dashboard change
that depends on a NEW metric MUST add instrumentation to
`internal/metrics/` or `ml/app/metrics.py` first; the contract
test `internal/deploy/monitoring_alerts_contract_test.go`
enforces this for alert rules.

| # | Dashboard | Purpose | Backing Metrics |
|---|-----------|---------|-----------------|
| 1 | Service Health | Up/down state for core + ML, restart count | `up{job="smackerel-core"}`, `up{job="smackerel-ml"}`, container restart counters |
| 2 | Ingestion Throughput | Rate of artifact ingestion split by source and type | `rate(smackerel_artifacts_ingested_total[5m])`, `rate(smackerel_capture_total[5m])` |
| 3 | NATS Pressure | Stream depth, ack lag, dead-letter rate | `rate(smackerel_nats_deadletter_total[5m])`, NATS-side `nats_*` metrics from the NATS monitoring port |
| 4 | ML Latency & Pool | Embedding latency, inflight pool utilization, rejection rate | `smackerel_ml_processing_latency_seconds`, `smackerel_ml_embedding_inflight`, `smackerel_ml_embedding_workers_configured`, `smackerel_ml_embedding_rejected_total` |
| 5 | Postgres Pressure | Connection pool saturation, slow query count | `smackerel_db_connections_active`, Postgres-side `pg_*` metrics from a sidecar exporter |
| 6 | Search Latency | Vector vs text fallback latency distribution | `histogram_quantile(0.95, rate(smackerel_search_latency_seconds_bucket[5m]))` split by `mode` |
| 7 | LLM Usage | Token usage and cost proxy by provider/model | `rate(smackerel_llm_tokens_used_total[1h])` split by `provider`, `model` |
| 8 | Alert Delivery | Operator notification pipeline health | `rate(smackerel_alert_delivery_failures_total[15m])` |
| 9 | Connector Sync | Per-connector success/failure trends | `rate(smackerel_connector_sync_total[15m])` split by `connector`, `status` |
| 10 | Domain Extraction | Schema extraction success by domain | `rate(smackerel_domain_extraction_total[15m])` split by `schema`, `status` |
| 11 | LLM Scenario Agent | Invocation outcomes, latency, and tool-call results per scenario (spec 037) | `rate(smackerel_agent_invocations_total[15m])` split by `scenario`, `outcome`; `histogram_quantile(0.95, rate(smackerel_agent_invocation_duration_seconds_bucket[5m]))`; `rate(smackerel_agent_tool_calls_total[15m])` split by `scenario`, `result` |

### Alert Runbook

Alert rules live in `config/prometheus/alerts.yml` and are mounted
read-only at `/etc/prometheus/alerts.yml`. Every rule below is
present in the live file (the contract test
`TestMonitoringAlertsContract_LiveFile` blocks regressions). The
SEVERITY and FIRING ACTION columns are the operator-facing
runbook — receiver routing (Telegram chat ID, PagerDuty key,
email distribution) is deploy-adapter overlay scope.

| Alert | Severity | Backing Metric | Firing Action |
|-------|----------|----------------|---------------|
| `SmackerelCoreUnavailable` | critical | `up{job="smackerel-core"}` | Check container health, inspect `docker logs smackerel-core`, confirm Postgres + NATS healthy. |
| `SmackerelMLUnavailable` | critical | `up{job="smackerel-ml"}` | Inspect ML sidecar logs; Go core falls back to text search but enrichment is paused. |
| `SmackerelIngestionStalled` | warning | `rate(smackerel_artifacts_ingested_total[30m])` | All connectors silent for 30m while core is up — check `smackerel_connector_sync_total` to localise. |
| `SmackerelNATSDeadLetterPressure` | warning | `rate(smackerel_nats_deadletter_total[10m])` | Sustained DLQ traffic — inspect failing stream + ML/LLM availability. |
| `SmackerelDBPoolSaturated` | warning | `smackerel_db_connections_active` | Connection pool ≥ 9/10 for 5m — bump `infrastructure.postgres.max_conns` or hunt slow queries. |
| `SmackerelMLEmbeddingStarvation` | warning | `rate(smackerel_ml_embedding_rejected_total[5m])` | ThreadPoolExecutor rejecting work — raise `services.ml.embedding_workers/queue_max` or scale CPU envelope. |
| `SmackerelAlertDeliveryFailing` | critical | `rate(smackerel_alert_delivery_failures_total[15m])` | Operator notifications failing — check Telegram bot token + chat mapping. |
| `SmackerelBackupStale` | warning | `smackerel_artifacts_ingested_total` | No ingestion for 24h — connectors stuck or backup pipeline silent. |
| `TwitterAPIRateLimitChronicExhaustion` | warning | `max_over_time(smackerel_connector_twitter_api_rate_limit_reset_seconds[15m])` | Twitter API rate-limit reset window above 60s sustained for 30m — verify bearer-token tier, reduce SST `services.connectors.twitter.poll_seconds`, or narrow active source lists. |
| `TwitterAPIRetryStorm` | warning | `rate(smackerel_connector_twitter_api_retries_total[5m])` | Twitter retry rate ≥ 0.2/s for 10m — check connector logs for the failing endpoint/reason, verify Tailscale connectivity to `api.twitter.com`, inspect `smackerel_connector_twitter_api_requests_total{status}` for 5xx / 429 patterns. |
| `SmackerelMLNATSDeadLetterPressure` | warning | `rate(smackerel_ml_nats_deadletter_total[10m])` | ML sidecar (Python) dead-letter pressure on a stream (spec 081 poison-pill routing) — inspect ML sidecar logs and confirm Ollama/LLM availability for SEARCH/SYNTHESIS streams. Companion to `SmackerelNATSDeadLetterPressure`, which only watches the Go core metric. |
| `SmackerelMLNATSDeadLetterPublishFailing` | critical | `rate(smackerel_ml_nats_deadletter_publish_failures_total[10m])` | ML sidecar cannot publish to the DEADLETTER stream — per the spec 081 publish-before-term invariant it nak()s and redelivers the poison message in a loop, so that consumer makes no forward progress. Check the DEADLETTER stream MaxBytes/MaxMsgs and the recurring publish error in ML logs. |
| `SmackerelAgentProviderErrors` | warning | `rate(smackerel_agent_invocations_total{outcome="provider-error"}[10m])` | LLM scenario-agent (spec 037) provider-error rate elevated — the agent's LLM driver (NATS → ML sidecar → Ollama/cloud) is failing. Confirm the ML sidecar is up, check LLM availability, inspect `agent_traces` rows with outcome `provider-error`. |
| `SmackerelAgentInvocationTimeouts` | warning | `rate(smackerel_agent_invocations_total{outcome="timeout"}[15m])` | Agent invocations exceeding their per-scenario wall-clock budget — usually a model too slow for the configured `timeout_ms` on the current hardware tier. Compare warm model latency to the scenario budget, raise the SST timeout, or select a faster provider/model. |
| `SmackerelAgentAllowlistViolations` | warning | `rate(smackerel_agent_tool_calls_total{result="allowlist-violation"}[10m])` | Agent-enforced tool-allowlist rejections on scenario `{{ $labels.scenario }}` — possible active prompt-injection steering the model to off-allowlist (write/external) tools, or a scenario whose allowlist drifted. Inspect `agent_traces`; confirm no write tool ever reached its handler. |
| `SmackerelIntelligenceAlertProductionFailing` | warning | `rate(smackerel_alert_producer_failures_total[15m])` | Phase-3 contextual-alert producers (bill reminders, return-window, trip-prep, relationship-cooling, commitment tracking — spec 004 R-304) are failing to CREATE alerts for type `{{ $labels.type }}` (an LLM timing/cooling judgment error, or a `CreateAlert` DB insert failure). Production-side twin of `SmackerelAlertDeliveryFailing` — the alert is never created, so the user silently misses the deadline. Confirm the ML sidecar is up (`SmackerelMLUnavailable`), check the alerts-table write path / DB health (`SmackerelDBPoolSaturated`), and inspect the core logs for the `alert timing evaluation failed` / `failed to create … alert` warn lines to localise the failing producer. |
| `SmackerelDigestSynthesisDegraded` | warning | `rate(smackerel_digest_generation_total{status="fallback"}[1h])` | The spec 004 daily/weekly synthesis digest (R-302) is failing to publish to the ML LLM generation path (NATS subject `digest.generate`) and the generator is storing plain-text fallback digests WITHOUT LLM synthesis — the `connection discovered` / `patterns noticed` sections collapse to a flat stats list. NATS publish-side failure, so `SmackerelNATSDeadLetterPressure` / the `smackerel-ml-nats` rules do NOT cover it. Confirm the ML sidecar is up (`SmackerelMLUnavailable`), verify NATS connectivity from the core, and inspect the core logs for the `NATS publish failed, generating fallback digest` warn line. |

### Metrics Access Boundary

Per the constitution's "No env-specific content in this repo"
rule, the product repo is target-agnostic and the deploy-adapter
overlay owns all operator-specific exposure decisions.

| Concern | Owner | Notes |
|---------|-------|-------|
| `/metrics` endpoint on `smackerel-core` and `smackerel-ml` | **Product** (this repo) | Unauthenticated by design — accessible only on the compose network; spec 030 |
| Metric names, types, label sets | **Product** (this repo) | Cardinality bounds enforced by `internal/metrics/*_test.go`; tests block label explosions |
| Prometheus scrape config template | **Product** (this repo) | `config/prometheus/prometheus.yml.tmpl`; SST-rendered; T-049-001 contract |
| Alert rule file | **Product** (this repo) | `config/prometheus/alerts.yml`; T-049-004 contract enforces every metric is real |
| Dashboard inventory (this table) | **Product** (this repo) | T-049-005 contract enforces this section exists |
| Prometheus host port binding | **Deploy adapter** | `HOST_BIND_ADDRESS` substitution decides exposure (tailnet IP / loopback) |
| Reverse proxy fronting Prometheus / Grafana | **Deploy adapter** | Caddy/nginx config + TLS — overlay-specific |
| Grafana dashboard JSON provisioning | **Deploy adapter** | Different operators use different Grafana versions/datasource configs |
| Alertmanager receivers (Telegram, PagerDuty, email) | **Deploy adapter** | Receiver credentials are operator-specific secrets |
| Retention beyond the 15-day default | **Deploy adapter** | Configure via overlay or remote-write to long-term storage |

### Static Contract Tests

The monitoring stack is locked down by a family of static contract
tests in `internal/deploy/`. None of them require a running
container — all run in `./smackerel.sh test unit --go`.

| Test | Spec | What it asserts |
|------|------|-----------------|
| `TestMonitoringScrapeContract_LiveTemplate` | FR-049-001 | Both required jobs exist; targets use compose service names; no env-specific content |
| `TestMonitoringRender_LiveTemplate` | FR-049-001/005(b) | Template renders to valid YAML with all substitutions applied |
| `TestMonitoringBindContract_LiveDevCompose` / `LiveDeployCompose` | FR-049-004 | No wildcard binds anywhere in either compose file |
| `TestMonitoringAlertsContract_LiveFile` | FR-049-003 | Every required alert exists; every metric reference is emitted by runtime |
| `TestMonitoringDocsContract_LiveFile` | FR-049-005(e) | This Operations.md section is present and complete |
| `TestComposeContract_LiveFile` (extended) | FR-049-004 (inherits spec 042) | Prometheus port uses fail-loud `${HOST_BIND_ADDRESS:?...}` substitution |
| `TestComposeResourceContract_LiveFile` (extended) | FR-049-005(c) (inherits spec 045) | Prometheus declares `${PROMETHEUS_CPU_LIMIT:?...}` and `${PROMETHEUS_MEMORY_LIMIT:?...}` |
| `TestFilesystemContract_LiveFile` (extended) | FR-049-005(c) (inherits spec 045) | Prometheus is `read_only: true` with `/tmp` as the only tmpfs allowlist entry |

### ML Sidecar Health Isolation (Spec 050)

The ML sidecar isolates CPU-bound embedding work from the FastAPI async
event loop using a dedicated bounded `ThreadPoolExecutor`. This keeps
`/health` responsive even when the model is saturated with embedding
requests, which is what the Go core health probe relies on for its
graceful-degradation behavior (text search fallback when ML is
unreachable).

**SST keys** (required — no defaults, no fallbacks):

| Key | Default in `config/smackerel.yaml` | Purpose |
|-----|-------------------------------------|---------|
| `services.ml.embedding_workers` | `2` | Caps active CPU-bound embedding threads. |
| `services.ml.embedding_queue_max` | `3` | Caps in-flight + queued embedding tasks; excess work is rejected with HTTP 503-equivalent backpressure error. Must be `>= embedding_workers`. |
| `services.ml.health_latency_sla_ms` | `500` | Observable SLA budget for `/health` (milliseconds). |

If any of these keys is missing, empty, non-integer, or non-positive the
sidecar refuses to start (`sys.exit(1)`) with a named ERROR log. To
change a value, edit `config/smackerel.yaml`, regenerate the env file
(`./smackerel.sh config generate`), and restart the stack
(`./smackerel.sh down && ./smackerel.sh up`).

**Prometheus metrics**:

- `smackerel_ml_embedding_workers_configured` (Gauge) — set to
  `embedding_workers` on executor construction.
- `smackerel_ml_embedding_inflight` (Gauge) — current number of
  in-flight embedding tasks.
- `smackerel_ml_embedding_rejected_total` (Counter) — total embedding
  tasks rejected due to `embedding_queue_max` backpressure.

**Tuning guidance**: on hardware with `n` cores reserved for embedding
(see `deploy_resources.smackerel_ml.cpus` in
`config/smackerel.yaml`), set `embedding_workers` to `n`. Set
`embedding_queue_max` to `2 * embedding_workers` for steady-state load
and reduce it to `embedding_workers` for stricter backpressure. The
`health_latency_sla_ms` default of `500` is generous for a healthy
sidecar. This SLA budget is enforced at three layers today:
(1) the dev-time adversarial regression in
`ml/tests/test_embedder.py::test_spec050_health_handler_unblocked_by_busy_executor`
pins median `/health` latency below the budget while the embedding
executor is saturated; (2) the deploy compose container healthcheck
(`deploy/compose.deploy.yml` `smackerel-ml.healthcheck`) probes `/health`
every 10s with a 10s timeout, so docker marks the ML container
unhealthy if a probe exceeds that envelope; and (3) the
`SmackerelMLEmbeddingStarvation` alert in
`config/prometheus/alerts.yml` fires on sustained
`smackerel_ml_embedding_rejected_total` increase, which is the
operator-actionable signal that the bounded executor is shedding work.
There is currently no runtime histogram on `/health` itself; if your
observability platform needs a per-scrape `/health` latency series,
deploy the Prometheus `blackbox_exporter` pointed at the ML sidecar
`/health` URL and alert on `probe_duration_seconds` against
`ML_HEALTH_LATENCY_SLA_MS / 1000`.

## TLS Setup

This section is generic local/self-hosted reverse-proxy guidance for the product
stack. It is not the KNB home-lab activation path. KNB home-lab activation uses
the out-of-tree adapter packet, explicit `HOST_BIND_ADDRESS`, host Caddy, and
adapter ports `41001/41002`.

Smackerel services bind to `127.0.0.1` by default (localhost only). To expose the stack over a network with HTTPS, use a reverse proxy.

### Caddy (Recommended — Automatic HTTPS)

Caddy automatically obtains and renews TLS certificates from Let's Encrypt.

1. [Install Caddy](https://caddyserver.com/docs/install)

2. Create a `Caddyfile`:

```
smackerel.example.com {
    # API and Web UI
    reverse_proxy 127.0.0.1:40001

    # Security headers (Caddy adds most by default)
    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
    }
}
```

3. Start Caddy:
```bash
sudo caddy start
```

Caddy automatically:
- Obtains a Let's Encrypt certificate for your domain
- Redirects HTTP → HTTPS
- Renews certificates before expiry (30 days before)

### nginx (Alternative)

1. Install nginx and certbot:
```bash
sudo apt install nginx certbot python3-certbot-nginx
```

2. Create `/etc/nginx/sites-available/smackerel`:

```nginx
server {
    listen 80;
    server_name smackerel.example.com;

    location / {
        proxy_pass http://127.0.0.1:40001;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support (if needed in future)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

3. Enable the site and obtain a certificate:
```bash
sudo ln -s /etc/nginx/sites-available/smackerel /etc/nginx/sites-enabled/
sudo certbot --nginx -d smackerel.example.com
sudo systemctl reload nginx
```

Certbot automatically configures HTTPS and sets up a renewal cron job.

### Which Ports to Expose

The `40001/40002` values below are product dev/local values. Current KNB
home-lab guidance uses `41001/41002` behind host Caddy and fail-loud
`HOST_BIND_ADDRESS` assignment by the adapter.

| Port | Service | Expose Externally? |
|------|---------|-------------------|
| 40001 | smackerel-core (API + Web UI) | **Yes** — via reverse proxy only |
| 40002 | smackerel-ml (ML sidecar) | **No** — internal only |
| 42001 | PostgreSQL | **No** — internal only |
| 42002 | NATS client | **No** — internal only |
| 42003 | NATS monitoring | **No** — internal only |
| 42004 | Ollama | **No** — internal only |

Only the core API (port 40001) should be exposed through the reverse proxy. All other services must remain on localhost.

### Certificate Renewal

- **Caddy**: Automatic. No action needed.
- **certbot/nginx**: Certbot installs a systemd timer or cron job that runs `certbot renew` twice daily. Verify with:
  ```bash
  sudo certbot renew --dry-run
  ```

### OAuth Callback URL Update

If switching from `http://127.0.0.1:40001` to `https://smackerel.example.com`, update:

1. `config/smackerel.yaml` → `oauth.google.redirect_url`:
   ```yaml
   oauth:
     google:
       redirect_url: "https://smackerel.example.com/auth/google/callback"
   ```
2. Google Cloud Console → Authorized redirect URIs
3. Regenerate config: `./smackerel.sh config generate`
4. Restart: `./smackerel.sh down && ./smackerel.sh up`

## Per-User Bearer Authentication (Spec 044)

Spec 044 introduces a per-user PASETO v4.public bearer-auth subsystem alongside
the legacy single-tenant `runtime.auth_token`. Scope 01 lands the SST surface,
the `internal/auth/` issue/verify/hash/revocation primitives, the DB schema, the
`smackerel-core auth` CLI, and the production-mode startup fail-loud guard.
Scope 02 wires the per-user `bearerAuthMiddleware` onto the API hot path
(`internal/api/router.go`), registers the four admin HTTP endpoints, and closes
three cross-spec body-actor trust-boundary issues in production mode
(MIT-040-S-008 photos mint/reveal, MIT-038-S-003 cloud-drive Connect,
MIT-027-TRACE-001 actor-source segment for user annotations). Web/Telegram
surfaces land at Scope 03; production deprecation of `SMACKEREL_AUTH_TOKEN`
lands at Scope 04.

The full design rationale lives in
[`specs/044-per-user-bearer-auth/spec.md`](../specs/044-per-user-bearer-auth/spec.md)
and [`design.md`](../specs/044-per-user-bearer-auth/design.md). This section is
the operator-facing reference.

### Per-Environment Default

| Environment | `auth.enabled` default | Mode |
|---|---|---|
| `dev` | `false` | Shared `SMACKEREL_AUTH_TOKEN` (legacy). No per-user setup required. |
| `test` | `false` | Shared `SMACKEREL_AUTH_TOKEN` (disposable). No per-user setup required. |
| `home-lab` | `true` | Per-user PASETO required. Operator MUST populate the three secrets below before the runtime starts. |

The per-environment override lives at `environments.<env>.auth_enabled` in
`config/smackerel.yaml`. Production-class deployments (any environment with
`runtime.environment=production` AND `auth.enabled=true`) refuse to start when
required signing or hashing material is missing — see Startup Fail-Loud below.

### Required Secrets For Production-Class Deployments

When `runtime.environment=production` AND `auth.enabled=true`, three secrets MUST
be populated in `config/smackerel.yaml` (or injected by the deploy adapter as
`AUTH_*` env vars) before `./smackerel.sh up`:

| Config key | Env var | Purpose |
|---|---|---|
| `auth.signing.active_private_key` | `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` | PASETO v4.public Ed25519 private key (hex). Signs newly-issued tokens. |
| `auth.signing.active_key_id` | `AUTH_SIGNING_ACTIVE_KEY_ID` | Short identifier embedded in the PASETO footer (e.g. `key-2026-05`). Verifier routes incoming tokens to the active or prior key by this id. |
| `auth.at_rest_hashing_key` | `AUTH_AT_REST_HASHING_KEY` | HMAC-SHA-256 key used to hash issued tokens before persisting them in `auth_tokens.hashed_token`. MUST differ from `auth.signing.active_private_key` (per spec 044 OQ-8 — startup refuses when they match). |

Two additional rotation-related keys are optional during normal operation but
required during a key rotation overlap window:

| Config key | Env var | Purpose |
|---|---|---|
| `auth.signing.prior_public_key` | `AUTH_SIGNING_PRIOR_PUBLIC_KEY` | PASETO v4.public Ed25519 public key (hex) of the previous active key. Verifier uses it to honor in-flight tokens issued before rotation, for the configured grace window. |
| `auth.signing.prior_key_id` | `AUTH_SIGNING_PRIOR_KEY_ID` | Key id of the prior key. The verifier matches an incoming token's footer `kid` against active or prior. |

A one-shot bootstrap secret is also required for the very first user enrollment
on a fresh production deployment:

| Config key | Env var | Purpose |
|---|---|---|
| `auth.bootstrap_token` | `AUTH_BOOTSTRAP_TOKEN` | Required when `auth.enabled=true` AND zero users are enrolled. Consumed once via `smackerel-core auth bootstrap` and then cleared. |

### Startup Fail-Loud

The runtime validates auth secrets at TWO layers and refuses to start when any
required value is missing:

1. **Loader layer** (`internal/config/config.go`) — `loadAuthConfig` rejects
   `token_format != paseto-v4-public`, `rotation_grace_window_hours < 24`,
   `clock_skew_tolerance_seconds` outside `[0, 60]`, and (in production with
   `auth.enabled=true`) empty signing private key, key id, hashing key, OR a
   hashing key that equals the signing private key.
2. **Runtime layer** (`internal/auth/startup.go`) — `ValidateRuntimeAuthStartup`
   re-runs the production-mode invariants from `cmd/core/wiring.go` immediately
   after the legacy `SMACKEREL_AUTH_TOKEN` guard, providing defense in depth if
   future loader changes weaken the loader-side check.

Operators see explicit error messages naming the missing field, e.g.:

```text
AUTH_SIGNING_ACTIVE_PRIVATE_KEY must be set when auth.enabled=true and runtime.environment=production
AUTH_AT_REST_HASHING_KEY must differ from AUTH_SIGNING_ACTIVE_PRIVATE_KEY (spec 044 OQ-8)
```

The same SST validation runs during `./smackerel.sh config generate --env <env>`,
surfacing the missing keys before the env file is written.

### CLI Surface

Scope 01 ships the auth subcommands inside the `smackerel-core` binary; there is
no `./smackerel.sh auth` wrapper at this scope. Operators invoke the CLI inside
the running core container:

```bash
docker exec -it smackerel-<env>-smackerel-core-1 smackerel-core auth <subcommand> [args...]
```

The container project name varies by environment: `smackerel-dev-smackerel-core-1`,
`smackerel-test-smackerel-core-1`, `smackerel-home-lab-smackerel-core-1`. Use
`./smackerel.sh status` to confirm the exact container name.

Available subcommands (per `cmd/core/cmd_auth.go`):

| Subcommand | Purpose |
|---|---|
| `keygen` | Print a fresh PASETO v4.public keypair (hex) plus a suggested `key_id`. Pure stdout; no DB or NATS. Used during rotation. |
| `bootstrap <user-id>` | One-shot first-user enrollment on a fresh deployment. Refuses if any user is already enrolled. Requires `SMACKEREL_BOOTSTRAP_TOKEN` env var matching `auth.bootstrap_token`. |
| `enroll [--notes "..."] <user-id>` | Enroll a new user. Mints the user's first token and prints it to stdout exactly once. |
| `rotate --prior-token-id <id> <user-id>` | Mint a fresh token for an enrolled user. Marks the prior token `rotated`; the prior token remains valid until its natural expiry, bounded by `auth.rotation_grace_window_hours`. |
| `revoke [--reason "..."] <token-id>` | Revoke a token immediately. The cache is updated locally and a NATS event is broadcast on `auth.revocations` so peer instances drop the token within the propagation budget (NFR-AUTH-006 ≤ 60s). |
| `list-users` | Print enrolled users (`user_id`, `enrolled_at`, `enrolled_by`, `status`, `notes`) as a tab-separated table. |

Exit codes: `0` success; `1` command-level failure (DB error, validation error,
missing material); `2` invocation error (missing args, unknown subcommand).

### Key Generation

```bash
docker exec -it smackerel-home-lab-smackerel-core-1 smackerel-core auth keygen
```

Output (placeholder values shown):

```text
# spec 044 — paste these into config/smackerel.yaml under auth.signing
# (rotate auth.signing.prior_public_key + prior_key_id from previous active values first)
active_private_key: "<64-byte hex private key>"
active_public_key:  "<32-byte hex public key>"  # publish for verifier-only consumers
active_key_id:      "key-2026-05"               # short identifier; embed in PASETO footer
```

Capture the output to your secret store (sealed-secrets, vault, deploy-overlay
secret env vars). The CLI never persists the key material; it only prints to
stdout.

### First-User Bootstrap (Fresh Production Deployment)

On a fresh deployment with `auth.enabled=true` AND zero enrolled users:

1. Set `auth.bootstrap_token` to a one-shot secret (e.g. `openssl rand -hex 24`)
   and regenerate config: `./smackerel.sh config generate --env <env>`.
2. Bring up the stack: `./smackerel.sh --env <env> up`.
3. Enroll the first user:

   ```bash
   docker exec -it smackerel-<env>-smackerel-core-1 \
     env SMACKEREL_BOOTSTRAP_TOKEN='<bootstrap-token>' \
     smackerel-core auth bootstrap '<user-id>'
   ```

4. The CLI prints `bootstrap successful — clear auth.bootstrap_token from config now to prevent reuse`,
   the new `user_id`, the new `token_id`, and the wire token (PASETO string).
   Capture the wire token immediately; it is never displayed again.
5. Clear `auth.bootstrap_token` back to the empty string in
   `config/smackerel.yaml`, regenerate config, and restart the stack.

The bootstrap path bypasses the admin-scope check (Scope 01 only allows the
bootstrap subcommand to run when zero users are enrolled).

### Manual Enrollment, Rotation, And Revocation

After bootstrap, use the regular subcommands. Examples (placeholder ids shown):

```bash
# Enroll a second user
docker exec -it smackerel-home-lab-smackerel-core-1 \
  smackerel-core auth enroll --notes 'household co-owner' '<user-id>'

# Rotate a user's token (during planned key rotation or after suspected leakage)
docker exec -it smackerel-home-lab-smackerel-core-1 \
  smackerel-core auth rotate --prior-token-id '<old-token-id>' '<user-id>'

# Revoke a specific token immediately (e.g. after device loss)
docker exec -it smackerel-home-lab-smackerel-core-1 \
  smackerel-core auth revoke --reason 'device-lost' '<token-id>'

# List enrolled users
docker exec -it smackerel-home-lab-smackerel-core-1 \
  smackerel-core auth list-users
```

The minted wire token is printed exactly once at `enroll`/`rotate` time. There
is no recovery path for a lost token — operators MUST capture the value from
stdout into the user's secret store at mint time. The `auth_tokens` row only
stores the HMAC-SHA-256 hash of the wire token under `hashed_token`.

### Scoped Token Enrollment (Spec 060)

Spec 060 amends spec 044 with a PASETO `scope` claim and an
`auth.RequireScope(...)` middleware. The token's wire and DB shapes are
unchanged for legacy spec-044 tokens (no `scope` claim); only operators who
opt in to scoped tokens via `--scope` produce the new wire shape.

The `./smackerel.sh auth ...` wrapper is a thin passthrough to
`smackerel auth ...` inside the running `smackerel-core` container. Args
flow through `"$@"` verbatim — the embedded `,` in
`extension:bookmarks,history` is NEVER split (it belongs to one scope value's
capability list).

When to use `--scope`:

- A surface MUST be wired with `auth.RequireScope(...)` for the scope claim
  to be enforced. Spec 060 ships the primitives only; the wiring lives in the
  consumer spec (e.g. spec 058 Chrome extension bridge wires the
  `extension:bookmarks,history` requirement on its ingest route).
- For routes WITHOUT a `RequireScope` wrap, scoped tokens behave exactly like
  legacy spec-044 tokens — the `scope` claim is parsed into `Session.Scopes`
  but ignored by the handler.

Mint a scoped token (initial enrollment):

```bash
./smackerel.sh auth enroll --notes 'chrome extension on workstation' \
  --scope extension:bookmarks,history <user-id>
```

Repeat `--scope` to accumulate multiple entries; the embedded `,` is part of
the capability list, not a separator between scopes:

```bash
./smackerel.sh auth enroll --scope extension:bookmarks,history \
  --scope extension:other <user-id>
```

Surfaces not yet in the `RegisteredScopeSurfaces` allowlist
(`internal/auth/scopes.go`) require the explicit `--allow-unknown-surface`
escape hatch. The mint emits a structured WARN log naming the unknown
surface; operators MUST land the surface in the allowlist in the same change
set as the spec that introduces it.

Rotation has three modes — pick exactly one:

```bash
# Preserve: parse the prior wire token and re-mint with the SAME scope claim.
./smackerel.sh auth rotate --prior-token-id <old-id> \
  --prior-token <old-wire-token> <user-id>

# Replace: mint with a new explicit scope list. The prior token's scope is
# ignored.
./smackerel.sh auth rotate --prior-token-id <old-id> \
  --scope extension:bookmarks,history <user-id>

# Demote: explicitly mint a legacy spec-044-shape token with no scope claim.
# The single `--scope ""` sentinel is the only way to demote at rotation
# time. Mixing `--scope ""` with non-empty `--scope` values is refused
# (exit 2) so a typo cannot silently lose scope or silently lose the demote
# intent.
./smackerel.sh auth rotate --prior-token-id <old-id> --scope "" <user-id>
```

Rotation refuses to run when neither `--scope` nor `--prior-token` is
supplied (exit 2 with `rotation requires --prior-token <wire> to preserve
scopes, or --scope to set them explicitly`). Per design §7.2, this is an
at-source refusal — the CLI does NOT silently preserve scopes from the
prior-token-id alone, because the on-disk hashed token cannot be parsed back
into a `scope` claim.

Inspect a wire token's parsed claims (`issuer`, `subject`, `jti`, `kid`,
`iat`, `exp`, `scopes`) as JSON. Pure verification path — no DB connect, no
NATS publish:

```bash
./smackerel.sh auth inspect '<wire-token>'
```

Migration notes:

- Existing spec-044 enrolled users do NOT need to re-enroll. Their tokens
  remain valid and continue to validate as `Session.Scopes == nil`. They
  reach scoped endpoints only after a rotation that explicitly mints a
  scoped token (e.g. spec 058's extension ingest route requires
  `extension:bookmarks,history`; a legacy user must rotate into a scoped
  token before their extension can ingest).
- The `auth_scope_rejected_total{required_scope,user_id}` Prometheus counter
  ticks once per 403 `scope_required` response. A non-zero rate on a route
  wired with `RequireScope(...)` after a deploy indicates one or more users
  are still on legacy tokens for that surface.
- The `auth_scope_check_bypassed_total{source}` counter ticks for
  `SessionSourceSharedToken` and `SessionSourceBootstrap` sessions — those
  bypass the scope check by design. In production-class deployments where
  `auth.production_shared_token_fallback_enabled=false`, the
  `source="shared_token"` label should stay at zero.

Scope Registry Maintenance:

- `internal/auth/scopes.go` is the single source of truth for
  `RegisteredScopeSurfaces`, `ScopeNameRegex`, `ValidateScopeName`, and
  `ExtractScopeSurface`. Do not maintain parallel lists.
- Initial spec 060 entry: `["extension"]` (consumed by spec 058
  OQ-DSN-1).
- Adding a new surface is a single-line append reviewed alongside the spec
  that introduces it. Until the entry lands, mints against that surface
  require `--allow-unknown-surface` and emit the WARN log above.

RequireScope endpoint wiring matrix (initial):

| Route | Required Scope | Wired By |
|---|---|---|
| `POST /v1/connectors/extension/ingest` | `extension:bookmarks,history` | spec 058 implementation |

Spec 060 ships ZERO endpoint wiring of `RequireScope(...)` on pre-existing
routes — consumer specs wire their own scope requirements as part of the
same change set that introduces the route.

### Admin HTTP Endpoints

`internal/api/auth_handlers.go` ships parallel admin HTTP endpoints to the CLI.
Scope 02 registers all four routes in `internal/api/router.go` behind
`bearerAuthMiddleware`, so they are reachable on the live API:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/auth/users` | Enroll a user |
| `POST` | `/v1/auth/users/{user_id}/rotate` | Rotate a user's active token |
| `POST` | `/v1/auth/tokens/{token_id}/revoke` | Revoke a specific token |
| `GET` | `/v1/auth/users` | List enrolled users |

Admin scope is enforced by `AuthAdminHandlers.callerIsAdmin`
(`internal/api/auth_handlers.go`):

| Caller's session source | Admin in production? | Admin in dev/test? |
|---|---|---|
| `SessionSourceBootstrap` (one-shot bootstrap session) | Yes (always) | Yes |
| `SessionSourceSharedToken` (`SMACKEREL_AUTH_TOKEN`) | Only when `auth.production_shared_token_fallback_enabled=true` | Yes |
| `SessionSourcePerUserToken` (per-user PASETO) | No (per-user admin allowlist not yet wired) | No |

Until the per-user admin allowlist surface lands in a later scope, operators in
production-class deployments use either the bootstrap session OR (when
`production_shared_token_fallback_enabled=true`) the legacy shared token to call
the admin endpoints. Non-admin callers receive `HTTP 401 FORBIDDEN` with body
`{"error":"FORBIDDEN","message":"admin scope required"}`.

Rotate a user's token (placeholder ids shown):

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"prior_token_id":"<old-token-id>"}' \
  http://127.0.0.1:40001/v1/auth/users/<user-id>/rotate
```

The handler issues a fresh token, persists it via `BearerStore`, and returns the
wire token in the response body. Capture the value immediately — it is never
displayed again.

Revoke a token (placeholder id shown):

```bash
curl -X POST \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"reason":"device-lost"}' \
  http://127.0.0.1:40001/v1/auth/tokens/<token-id>/revoke
```

The handler updates `auth_tokens.status` to `revoked`, refreshes the local
revocation cache, and broadcasts an event on `auth.revocation_nats_subject`
(default `auth.revocations`).

### Token Rotation Grace Window

When a token is rotated (via the CLI `rotate` subcommand or the
`POST /v1/auth/users/{user_id}/rotate` endpoint), the prior token is marked
`rotated` in `auth_tokens` but the verifier continues to honor it until its
recorded `expires_at` passes. Within that window, requests bearing either the
prior token OR the freshly-minted token are admitted; after the window, the
prior token is rejected with `HTTP 401`. The new token admits immediately. The
window is bounded by `auth.rotation_grace_window_hours` (loader floor 24 h).

### Revocation Propagation

Revocations propagate across runtime instances via two paths configured in SST:

| Config key | Default | Purpose |
|---|---|---|
| `auth.revocation_nats_subject` | `auth.revocations` | NATS broadcast subject. Producers publish on revoke; subscribers update their local cache on receive. |
| `auth.revocation_cache_refresh_interval_seconds` | `30` | Periodic DB-poll cadence as the failure-mode backstop when NATS is partitioned. |

In the happy path the broadcast loopback closes the staleness window in well
under one second; the DB-refresh fallback closes it within
`revocation_cache_refresh_interval_seconds`. Worst-case propagation is bounded
by NFR-AUTH-006 (≤ 60 s).

### Production Body / Header Actor-Identity Rejection (Scope 02 MIT closures)

In `runtime.environment=production` with `auth.enabled=true`, the per-user
`bearerAuthMiddleware` derives the actor identity from the verified PASETO
session and rejects any client-supplied actor identifier on three handlers
(closing MIT-040-S-008, MIT-038-S-003, and the actor-source segment of
MIT-027-TRACE-001):

| Handler | Forbidden client surface | Production response |
|---|---|---|
| Photos `MintReveal` (`POST /api/v1/photos/upload`) — `internal/api/photos_upload.go` | Body field `actor_id` | `HTTP 400 actor_id_in_body_forbidden` |
| Photos `MintReveal` | Header `X-Actor-Id` | `HTTP 400 actor_id_in_header_forbidden` |
| Cloud-drive `Connect` (`POST /v1/drives/connections`) — `internal/api/drive_handlers.go` | Body field `owner_user_id` | `HTTP 400 owner_user_id_in_body_forbidden` |
| User annotation create (`POST /api/annotations`) — `internal/api/annotations.go` | Body field `actor_source` | `HTTP 400 {"error":"actor_source in request body is forbidden in production"}` |

In `dev` and `test` (or in production when `auth.enabled=false`), the legacy
ergonomic is preserved — body and header actor identifiers continue to be
honored so existing local-dev scripts and integration tests work unchanged.

Operator action: any production API consumer that previously sent `actor_id`,
`owner_user_id`, or `actor_source` in the request body MUST be updated to omit
those fields. The actor identity is derived from the bearer token claims; no
client-supplied value can override it.

### Observability

`AUTH_TELEMETRY_ENABLED=true` (default) and `AUTH_TELEMETRY_METRIC_PREFIX=smackerel_auth`
(default) reserve the SST surface for the per-user-bearer-auth metric family.
Metric registration lands at Scope 04 (per spec 044 OQ-9 + spec 030 dashboards);
Scopes 01-02 only ship the SST keys.

### Per-User Bearer Auth — Scope 03 (Web Surfaces + Telegram)

Scope 03 extends per-user PASETO authentication onto three caller surfaces — the
PWA web client, the browser extension, and the Telegram bridge — and ships an
operator-facing admin token-management UI behind the existing bearer middleware.
The same `auth.signing.*` and `auth.at_rest_hashing_key` SST surface from Scopes
01-02 covers all four; no new secret material is required at this scope.

#### PWA Cookie-Derived Sessions (`/v1/web/login`)

`POST /v1/web/login` accepts a per-user PASETO token in the request body
(production) or the shared `runtime.auth_token` value (dev/test) and converts it
into an HttpOnly cookie that the browser presents on subsequent same-origin
requests. The route lives outside `bearerAuthMiddleware` (it is the entry point
itself) and is rate-limited to 20 requests per IP per minute via
`httprate.LimitByIP` to absorb credential-stuffing attempts.

Request shape:

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"token":"<per-user PASETO>"}' \
  https://<deploy-host-fqdn>/v1/web/login
```

On success the handler sets the `auth_token` cookie with these attributes
(`internal/api/web_login.go`):

| Attribute | Value | Source |
|---|---|---|
| `HttpOnly` | `true` | unconditional |
| `SameSite` | `Lax` | unconditional |
| `Path` | `/` | unconditional |
| `Secure` | `true` only when `runtime.environment=production` | `strings.EqualFold(env, "production")` |
| `Expires` | matches the verified PASETO `exp` claim | derived from token |

`POST /v1/web/logout` clears the cookie (`MaxAge: -1`). Both endpoints respond
with JSON-only payloads; tokens never appear in URLs or query strings.

The hot-path bearer middleware (`(*Dependencies).bearerAuthMiddleware` in
`internal/api/router.go`) was extended in this scope so that
`extractBearerToken` falls back to the `auth_token` cookie when no
`Authorization: Bearer` header is present. Existing API clients that send the
header continue to work unchanged; only browser callers that previously had no
session path benefit from the cookie fallback.

#### Browser-Friendly Login Entry Point (`GET /login`, spec 057)

Spec 044 shipped the cookie-derived session API but no HTML entry point — a
browser visit to `https://<host>/` with no cookie returned plain `401
Unauthorized` text. Spec 057 closes that gap with two operator-visible
additions on top of the existing wire contract:

1. **`GET /login`** serves a minimal HTML form (single token field +
   submit) that POSTs to the existing `POST /v1/web/login` and then
   redirects to the original `next` path (default `/`). Conforms to the
   existing CSP; works for both production per-user PASETO tokens and the
   dev shared `runtime.auth_token` value.
2. **401 → 303 redirect for browser GETs** — when
   `bearerAuthMiddleware` sees an unauthenticated request with method
   `GET` or `HEAD` AND `Accept: text/html`, it now returns `303 See
   Other` with `Location: /login?next=<requested-path>` instead of
   `401`. API/JSON callers (no `text/html` in `Accept`) keep receiving
   `401` unchanged — spec 044's wire contract is preserved with zero
   regression.

The root `/` UI and the `/login` page both expose `POST /v1/web/logout`
so a browser user can clear their cookie and return to the form. No new
credentials, auth modes, or Caddy-layer changes are introduced; the token
remains the credential and the form is purely a paste target.

#### Browser Extension Per-User PASETO

The browser extension stores its bearer token in
`chrome.storage.local.authToken` and forwards it verbatim as
`Authorization: Bearer <token>` on every request (`web/extension/background.js`
`getConfig()` block). The storage slot is format-agnostic — either a per-user
PASETO produced by the `smackerel-core auth enroll <user-id>` CLI OR the legacy
shared `SMACKEREL_AUTH_TOKEN` value works without any extension code change.

Operator workflow for switching an extension installation to per-user auth:

1. Mint a token for the user with `smackerel-core auth enroll <user-id>`
   (see CLI Surface above).
2. Open the extension popup and paste the wire token into the auth-token
   field; the popup writes it to `chrome.storage.local.authToken`
   atomically (`web/extension/popup/popup.js`).
3. Verify by capturing any URL — the extension makes a `GET
   /v1/photos/connectors` round-trip first to validate the bearer.

See [`web/extension/README.md`](../web/extension/README.md) for the
extension-side transparency contract.

#### Telegram Chat → User Mapping

The Telegram bridge resolves an inbound chat into an enrolled `user_id` via the
`TELEGRAM_USER_MAPPING` env var (sourced from `telegram.user_mapping` in
`config/smackerel.yaml`). Format: comma-separated `<chat_id>:<user_id>` pairs.

```yaml
telegram:
  bot_token: ""
  chat_ids: ""
  user_mapping: "12345:alice,67890:bob"
```

Behavior at the bot's message-handling entry point
(`internal/telegram/bot.go` `safeHandleMessage` line 284 +
`safeHandleCallback` line 251):

| Environment | Mapping state | Behavior |
|---|---|---|
| `production` | chat-id is mapped | Resolve `user_id`, continue handler dispatch |
| `production` | chat-id is NOT mapped | `slog.Warn` + drop message (no internal API call) |
| `production` | mapping is empty | Reject all chats |
| `dev` / `test` | any | Permissive — empty mapping or unmapped chat both proceed |

Operator runbook for adding or removing a mapping:

1. Edit `telegram.user_mapping` in `config/smackerel.yaml` (or the deploy
   adapter's overlay).
2. Regenerate config: `./smackerel.sh config generate`.
3. Restart the stack: `./smackerel.sh down && ./smackerel.sh up`.
4. Confirm the mapping took effect by sending a Telegram message from the
   targeted chat and watching the logs — a mapped chat produces a normal
   capture log line, an unmapped chat in production produces the
   `telegram: drop message from unmapped chat` warning.

The mapping is parsed once at startup (`parseTelegramUserMapping` in
`internal/config/config.go`); there is no hot-reload. Whitespace is tolerated
between pairs and around the colon. Negative chat ids (Telegram supergroups)
are accepted.

##### F02 Closure (Scope 04 shipped)

Spec 044 Scope 04 closes the F02 deferred-finalize-blocker. The Telegram
bot now wires `PerUserTokenMinter`
(`internal/telegram/per_user_token.go`) into every outbound HTTP call via
`Bot.bearerForChat(chatID)` and the `Bot.setBearerHeader(req, chatID)`
helper (see `internal/telegram/bot.go` lines 200–254). Production
deployments with `auth.enabled=true` AND a configured
`auth.signing.active_private_key` activate per-user PASETO minting:
`startTelegramBotIfConfigured` (`cmd/core/wiring.go`) constructs the
minter and calls `tgBot.SetPerUserTokenMinter(minter)` once at startup
(TTL = 5 minutes per design.md §13).

Decision matrix the live bot honors:

| Environment | Chat mapped? | Auth.enabled? | Bearer attached |
|---|---|---|---|
| `production` | Yes | Yes | Fresh per-user PASETO via `MintForChat` |
| `production` | No | Yes | **Request refused** (`ErrNoUserMappingForChat`) — no fallback |
| `production` | Any | No | Shared `runtime.auth_token` (legacy fallback) |
| `dev` / `test` | Yes | Yes | Fresh per-user PASETO via `MintForChat` |
| `dev` / `test` | No | Yes | Shared `runtime.auth_token` (clean dev fallback) |
| `dev` / `test` | Any | No | Shared `runtime.auth_token` |

Closure evidence: in-package unit test
`internal/telegram/bot_wiring_test.go` (8 cases covering the matrix
above), live-stack integration test
`tests/integration/auth_telegram_f02_wiring_test.go`
(`TestF02Wiring_SetPerUserTokenMinter_HappyPath`,
`TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses`), and
the existing Scope 03 e2e suite
`tests/integration/auth_telegram_e2e_test.go`. The
`production_shared_token_fallback_enabled` flag (default `false` per
FR-AUTH-017) is now safe to flip to `false` in any production Telegram
deployment — see "Deprecation Pathway" below for the supervised
sequence.

##### Authentication Metrics (Scope 04)

The runtime exposes seven Prometheus series under the
`smackerel_auth_*` prefix from
[`internal/metrics/auth.go`](../internal/metrics/auth.go) for
operator visibility into the per-user bearer-auth subsystem:

| Metric name | Type | Labels | Surface emitted from |
|---|---|---|---|
| `smackerel_auth_token_issuance_total` | Counter | `source` (`admin_api`, `bootstrap_cli`, `telegram_bridge`) | `internal/api/auth_handlers.go` (HandleEnroll, HandleRotate); `cmd/core/cmd_auth.go` (runAuthEnroll, runAuthRotate, runAuthBootstrap); `internal/telegram/per_user_token.go` (MintForUser) |
| `smackerel_auth_token_rotation_total` | Counter | (none) | `HandleRotate`, `runAuthRotate` |
| `smackerel_auth_token_revocation_total` | Counter | `reason` (closed set: `unspecified`, `compromise`, `rotation`, `offboarding`, `test`, `other`; raw input is bucketed via `NormalizeRevocationReason`) | `HandleRevoke`, `runAuthRevoke` |
| `smackerel_auth_token_validation_latency_seconds` | Histogram | (none); buckets `0.0001..0.1` | `bearerAuthMiddleware` (per-request, includes verify + revocation lookup) |
| `smackerel_auth_token_validation_outcome_total` | Counter | `result` (`accepted`, `rejected_expired`, `rejected_unknown_key`, `rejected_malformed`, `rejected_revoked`), `source` (`header`, `pwa_cookie`, `""`) | `bearerAuthMiddleware` per validation branch |
| `smackerel_auth_legacy_fallback_used_total` | Counter | `environment` (`production`, …) | `bearerAuthMiddleware` Branch 2 (shared-token fallback path) |
| `smackerel_auth_failure_total` | Counter | `reason` (closed set: `missing_token`, `invalid_format`, `paseto_verify_failed`, `revoked`, `shared_token_mismatch`, `auth_not_configured`) | `bearerAuthMiddleware` per 401 branch |

The `result` label values for `*_validation_outcome_total` are mapped
from `auth.VerifyToken` errors via `classifyVerifyError`
(`internal/api/router.go`); the closed set is the contract — operators
can build alerts on these values without parsing free-text error
strings.

Recommended scrape examples:

```promql
# Telegram-bridge mint rate (per-second, 5m window)
rate(smackerel_auth_token_issuance_total{source="telegram_bridge"}[5m])

# Production legacy-fallback usage (alert if > 0 after deprecation flip)
sum(rate(smackerel_auth_legacy_fallback_used_total{environment="production"}[5m]))

# Validation latency p95 over 5m
histogram_quantile(0.95,
  sum(rate(smackerel_auth_token_validation_latency_seconds_bucket[5m]))
  by (le))

# Token revocations bucketed by reason (compromise spikes warrant
# immediate operator review)
sum(increase(smackerel_auth_token_revocation_total[1h])) by (reason)
```

##### Deprecation Pathway — `production_shared_token_fallback_enabled`

The flag `auth.production_shared_token_fallback_enabled` (default
`false` per FR-AUTH-017; verified in `config/smackerel.yaml`) governs
whether `bearerAuthMiddleware` accepts the legacy shared
`runtime.auth_token` in production. With Scope 04 shipped (F02 closure
above + admin UI scoped to per-user PASETO via existing Scope 03 work),
the flag is safe to keep at its default of `false` for all production
deployments.

Operator sequence to retire the fallback in an existing deployment that
currently runs with the flag set to `true`:

1. **Deploy** the new build with all per-user surfaces wired AND
   `production_shared_token_fallback_enabled=true` retained for one
   transition window. Verify via
   `curl http://<host>:<port>/healthz` that the deployment is healthy.
2. **Monitor** `smackerel_auth_legacy_fallback_used_total{environment="production"}`
   for at least one full operator workday after the deploy. If the
   rate is non-zero, identify the caller (the metric labels surface
   environment; the access log surfaces the request path) and migrate
   that caller to a per-user token via the admin UI before flipping
   the flag.
3. **Flip** the flag to `false` in `config/smackerel.yaml`, run
   `./smackerel.sh config generate`, then restart the stack
   (`./smackerel.sh down && ./smackerel.sh up`).
4. **Verify** `smackerel_auth_legacy_fallback_used_total{environment="production"}`
   stays at zero post-flip and that admin/PWA/extension/Telegram
   surfaces all continue to authenticate (look for
   `smackerel_auth_token_validation_outcome_total{result="accepted"}`
   ticking on each surface). The
   `smackerel_auth_failure_total{reason="shared_token_mismatch"}`
   counter MAY tick if a caller still presents the legacy token; the
   401 response is the correct contract.
5. **Rollback** procedure: if step 4 surfaces missed callers, flip
   the flag back to `true` and resume from step 2.

##### Final Scope 04 Audit — End-To-End Migration

This subsection consolidates the operator-facing migration story for
spec 044 across the four shipped scopes plus the supervised
deprecation-flag flip. Operators running an existing single-tenant
Smackerel deployment use this checklist to move to the per-user
bearer-auth posture without downtime, with metric-driven cutover
gates at each step. Cross-references to the per-scope deliverables
live above; this view is the operator-side end-to-end audit.

**Migration sequence (Scope 1 → 2 → 3 → 4 → flag flip).** Each step is
durable and reversible until step 5; nothing destroys the legacy
shared-token surface until the operator explicitly flips
`auth.production_shared_token_fallback_enabled=false` in step 5.

| Step | Scope | What lands | Cutover gate (operator-observable) |
|---|---|---|---|
| 1 | Scope 01 | PASETO v4.public foundation, signing/at-rest keys (3 required env vars), DB migration 033, CLI subcommands, admin HTTP handlers, in-memory revocation cache + NATS broadcaster, startup fail-loud guard. Deploy with `auth.enabled=false` first to run the migration safely; flip to `true` once secrets are confirmed in the deploy overlay. | `./smackerel.sh status` reports healthy; `curl http://<host>:<port>/healthz` returns `200`; no unexpected `auth_not_configured` failures in logs. |
| 2 | Scope 02 | Hot-path `bearerAuthMiddleware` on every API route; admin REST endpoints (`POST /v1/auth/users`, `POST /v1/auth/users/{id}/rotate`, `POST /v1/auth/tokens/{id}/revoke`, `GET /v1/auth/users`); production-mode body / header actor-identity rejection at photos `MintReveal`, drive `Connect`, and annotation create handlers (closes MIT-040-S-008, MIT-038-S-003, MIT-027-TRACE-001 actor-source segment). Flip `auth.enabled=true` AND `auth.production_shared_token_fallback_enabled=true` (the transition setting) in this step. | `smackerel_auth_token_validation_outcome_total{result="accepted"}` ticks for every authenticated route; `smackerel_auth_failure_total{reason="paseto_verify_failed"}` stays low (it ticks on a real misuse); no `actor_id_in_body_forbidden` / `owner_user_id_in_body_forbidden` 400s from a legitimate client (any API consumer presenting body-supplied actor identifiers MUST be migrated to omit those fields per the spec 044 Scope 02 contract). |
| 3 | Scope 03 | PWA `POST /v1/web/login` cookie-derived sessions; browser extension reads `chrome.storage.local.authToken`; Telegram `chat_id → user_id` mapping with production unmapped-chat drop; admin token-management UI at `/admin/auth/tokens` (mint / list / rotate / revoke). | `smackerel_auth_token_issuance_total{source="admin_api"}` ticks on user enrollment; PWA users authenticate via `/v1/web/login` and request the `auth_token` cookie attaches; Telegram bot logs `telegram: drop message from unmapped chat` for any chat not present in `telegram.user_mapping`. |
| 4 | Scope 04 | F02 closure: Telegram bot wires `PerUserTokenMinter.MintForChat` into every outbound HTTP call (production mapped chats mint per-user PASETO; production unmapped chats are refused via `ErrNoUserMappingForChat`); seven-series `smackerel_auth_*` metrics surface; `auth.production_shared_token_fallback_enabled` defaults to `false`. | `smackerel_auth_token_issuance_total{source="telegram_bridge"}` ticks per Telegram-originated capture against a mapped chat; `smackerel_auth_legacy_fallback_used_total{environment="production"}` MAY still tick if any legacy caller is still presenting the shared token (this is the signal that step 5 is not yet safe). |
| 5 | Flag flip | Operator flips `auth.production_shared_token_fallback_enabled=false` in `config/smackerel.yaml`, runs `./smackerel.sh config generate`, then `./smackerel.sh down && ./smackerel.sh up`. | `smackerel_auth_legacy_fallback_used_total{environment="production"}` stays at zero post-flip; `smackerel_auth_failure_total{reason="shared_token_mismatch"}` MAY tick on residual legacy callers (the 401 response is the contract). |

**Metric-based cutover criteria (the gate to flip the flag in step 5).**
The supervised cutover from "shared-token fallback permitted" to
"per-user PASETO only" is gated on the operator confirming the
following three criteria over a representative observation window
(at least one full operator workday after the Scope 04 deploy):

1. `sum(rate(smackerel_auth_legacy_fallback_used_total{environment="production"}[5m]))` is `0` for every 5-minute bucket across the window. A non-zero rate identifies a caller still presenting the legacy `runtime.auth_token`; the access log surfaces the request path, and the caller MUST be migrated to a per-user token via the admin UI before the flag flip.
2. Every active caller surface has at least one `smackerel_auth_token_validation_outcome_total{result="accepted"}` increment per session: PWA users (`source="pwa_cookie"`), browser extension users (`source="header"`), Telegram users (`source="header"` via `telegram_bridge` mints), and admin operators (`source="header"` via the bootstrap or per-user admin token).
3. `histogram_quantile(0.95, sum(rate(smackerel_auth_token_validation_latency_seconds_bucket[5m])) by (le))` is below the NFR-AUTH-001 5 ms p99 hot-path budget (chaos-phase benchmark `BenchmarkAuthChaos_S03_PWACookieDerivedSession_HotPath` recorded ≈1.5 ms/op against a live test stack — the production-class deployment SHOULD see comparable or better numbers given the at-rest hashing and NATS subjects are already warm).

**Rollback path (any step).** Every step before the flag flip is
reversible via the corresponding compose-level revert + restart;
the flag flip itself is reversible by setting
`auth.production_shared_token_fallback_enabled=true` in
`config/smackerel.yaml`, regenerating, and restarting (the
shared-token Branch 2 of `bearerAuthMiddleware` re-activates on
boot and accepts the legacy token while still admitting per-user
PASETO bearers). Operators monitoring step 5 SHOULD plan a
rollback if `smackerel_auth_failure_total{reason="shared_token_mismatch"}`
ticks for any caller they cannot identify and migrate within the
post-flip observation window. No data is lost on rollback —
revocation state, enrolled users, and the at-rest token hashes
all live in PostgreSQL and survive any combination of flag flips.

**Deferred beyond Scope 04 (intentional, NOT blocking).** Two items
remain explicitly deferred per
[`specs/044-per-user-bearer-auth/design.md` §17.3](../specs/044-per-user-bearer-auth/design.md):

- The MIT-027-TRACE-001 NATS-segment closure (annotation pipeline
  derives `actor_source` from session for ALL entry points
  including raw NATS subjects). Scope 04 audit Gate A2 confirmed
  Scope 04 touched ZERO NATS files. The defensive layer at
  `internal/api/annotations.go` (Scope 02 work) covers the API
  entry path AND the NATS-bridged write path that goes through it;
  no security regression. Closure is appropriately deferred to
  spec-level finalize or a future spec.
- The per-user admin allowlist surface (`callerIsAdmin` for
  `SessionSourcePerUserToken`). Currently admin mutations require
  either the bootstrap session OR — when
  `production_shared_token_fallback_enabled=true` — the legacy
  shared token. Per-user admin token authoring is a follow-up
  surface; the page itself loads under any authenticated session
  and the underlying `/v1/auth/*` endpoints continue to enforce
  admin scope at the XHR layer.

#### Admin Token-Management UI (`/admin/auth/tokens`)

Scope 03 ships a single embedded static HTML page that serves as the operator
self-service surface for the Scope 02 `/v1/auth/*` admin endpoints. The page
lives at `internal/api/admin_ui_static/tokens.html` and is served via
`HandleAdminTokensUI` (`internal/api/admin_ui.go`) at `GET
/admin/auth/tokens` behind `bearerAuthMiddleware`.

Three panels are rendered XSS-safe (`textContent` + `appendChild` only):

| Panel | Calls | Purpose |
|---|---|---|
| **Mint a New User** | `POST /v1/auth/users` | Enroll a new user; UI displays the wire token exactly once |
| **Enrolled Users** | `GET /v1/auth/users` + per-row `POST /v1/auth/users/{user_id}/rotate` | List existing enrollments; rotate each user's active token |
| **Revoke a Specific Token** | `POST /v1/auth/tokens/{token_id}/revoke` | Revoke an individual token immediately |

Response headers set by `HandleAdminTokensUI`:

- `Content-Type: text/html; charset=utf-8`
- `Cache-Control: no-store`
- `X-Content-Type-Options: nosniff`
- `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'`

XHR mutations carry `credentials: 'same-origin'` so the existing `auth_token`
cookie (set by `/v1/web/login`) authenticates the call.

Access model — defense in depth at the XHR layer:

1. The page handler enforces ONLY `bearerAuthMiddleware` admit (any
   authenticated session that has a cookie OR `Authorization: Bearer` header
   can load the page).
2. The underlying `/v1/auth/*` admin REST endpoints independently enforce
   admin scope via `AuthAdminHandlers.callerIsAdmin` (per the table in the
   "Admin HTTP Endpoints" section above). A non-admin authenticated session
   that loads the page sees the form chrome, but every admin operation
   returns `HTTP 401 FORBIDDEN admin scope required` from the underlying
   endpoint.

Per-user PASETO sessions (`SessionSourcePerUserToken`) currently do NOT pass
`callerIsAdmin` (the per-user admin allowlist surface is deferred). Operators
on production-class deployments use either the bootstrap session or — when
`production_shared_token_fallback_enabled=true` — the legacy shared token to
exercise the admin UI's mutation panels. The page itself loads under any
authenticated session.

## Expense Tracking Configuration

Expense tracking captures receipts from email, photos, and PDFs, classifies them using a 7-level rule chain, and supports CSV export.

### Enabling Expense Tracking

Set `features.expense_tracking.enabled: true` in `config/smackerel.yaml` and configure:

```yaml
features:
  expense_tracking:
    enabled: true
    categories:
      - groceries
      - dining
      - transport
      - utilities
      - entertainment
      - health
      - travel
      - home
      - other
    business_vendors:
      - "Amazon Web Services"
      - "Google Cloud"
    labels:
      needs_review: "needs-review"
      personal: "personal"
      business: "business"
```

Regenerate config and restart after changes:
```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

The expense API provides 7 endpoints: query, export CSV, correction, classification, suggestions, receipt confirmation, and vendor normalization.

## Meal Planning Configuration

Meal planning provides weekly plans with slot assignment, shopping list generation, and optional CalDAV calendar sync.

### Enabling Meal Planning

Set `features.meal_planning.enabled: true` in `config/smackerel.yaml` and configure:

```yaml
features:
  meal_planning:
    enabled: true
    meal_types:
      - breakfast
      - lunch
      - dinner
      - snack
    meal_times:
      breakfast: "08:00"
      lunch: "12:00"
      dinner: "18:30"
      snack: "15:00"
    default_servings: 2
    caldav:
      enabled: false
      url: ""
      username: ""
      password: ""
      calendar_name: "Meal Plans"
```

The meal plan API provides 12 endpoints for creating, querying, assigning recipes to slots, copying plans, generating shopping lists, and syncing to CalDAV.

## Recipe Features

> Authoritative spec: [`specs/035-recipe-enhancements`](../specs/035-recipe-enhancements/spec.md)
> (Phase A — Foundation: serving scaler + cook mode, certified `done`).
> Phase B (LLM agent + tool registry, Scopes 07–16) is gated on
> [`specs/037-llm-agent-tools`](../specs/037-llm-agent-tools/spec.md) and is
> not yet wired.

### Cook Session Timeout

Cook mode provides a step-by-step Telegram walkthrough for any recipe. Sessions time out after the configured duration (default 2 hours):

```yaml
features:
  recipes:
    cook_session_timeout: "2h"
```

### Serving Scaler

The serving scaler adjusts ingredient quantities for any serving count. Fractions are formatted as kitchen-friendly values (e.g., ½, ¾, ⅓). Scaling is available via the recipe API endpoint and Telegram commands.

### Observability

Recipe operations (serving scaler invocations, cook session lifecycle,
disambiguation flows) do not currently emit dedicated Prometheus metrics in
`internal/metrics/metrics.go`. Operators rely on aggregate `internal/telegram`
and `internal/api` log lines for incident triage. Adding bounded-cardinality
recipe counters is tracked as a follow-up under the spec 035 backlog
(see report.md "Round 17 — devops probe" appendix).

## Troubleshooting — New Features

### Expenses Not Showing Up

| Check | Resolution |
|-------|------------|
| Feature enabled? | Verify `features.expense_tracking.enabled: true` in `config/smackerel.yaml` |
| Config regenerated? | Run `./smackerel.sh config generate` after config changes |
| Receipt extraction working? | Check ML sidecar logs: `docker logs smackerel-ml 2>&1`. The `receipt-extraction-v1` prompt contract must be present in `/app/prompt_contracts/` |
| Vendor not recognized? | Vendor normalization uses an LRU cache with pre-seeded aliases. Unknown vendors appear as "needs-review" until corrected |

### Meal Plan Slots Fail

| Check | Resolution |
|-------|------------|
| Feature enabled? | Verify `features.meal_planning.enabled: true` in `config/smackerel.yaml` |
| Overlapping plans? | Plans with overlapping date ranges are rejected. Check existing plans via `GET /api/meal-plans` |
| Recipe not found? | Slots require a valid recipe artifact ID. Verify the recipe exists in the system |
| CalDAV sync failing? | Check CalDAV URL, credentials, and network connectivity. CalDAV sync is optional and does not block plan creation |

### Cook Mode Timeout

| Check | Resolution |
|-------|------------|
| Session expired? | Default timeout is 2 hours. Adjust `features.recipes.cook_session_timeout` in config |
| No active session? | Start a cook session via Telegram command. Sessions are per-user and per-recipe |
| Bot not responding? | Check Telegram bot token and that the bot is receiving webhook updates |

## Browser Extension

The Smackerel browser extension enables one-click capture of any web page. It supports Chrome (Manifest V3) and Firefox (Manifest V2).

### Chrome Installation

1. Open `chrome://extensions/` in Chrome
2. Enable **Developer mode** (toggle in the top-right corner)
3. Click **Load unpacked**
4. Select the `web/extension/` directory from the Smackerel repository
5. The Smackerel icon appears in the toolbar

### Firefox Installation

1. Open `about:debugging#/runtime/this-firefox` in Firefox
2. Click **Load Temporary Add-on...**
3. Select `web/extension/manifest.firefox.json` from the Smackerel repository
4. The Smackerel icon appears in the toolbar

> **Note:** Firefox temporary add-ons are removed when the browser closes. For persistent installation, use `./smackerel.sh package extension` to create distributable `.zip` files, then install from the packaged file.

### Packaging Extensions for Distribution

To create distributable packages for Chrome and Firefox:

```bash
./smackerel.sh package extension
```

This produces:
- `dist/extension/smackerel-chrome-{version}.zip` — Chrome extension package
- `dist/extension/smackerel-firefox-{version}.zip` — Firefox extension package (with Firefox-specific manifest)

Users can install the Chrome `.zip` by extracting it and loading via **Load unpacked** in `chrome://extensions/`, or by dragging the `.zip` into Chrome. For Firefox, rename the `.zip` to `.xpi` and install from `about:addons`.

### Extension Configuration

After installation, click the Smackerel toolbar icon to open the setup popup:

1. **Server URL** — enter your Smackerel instance URL (e.g., `https://smackerel.example.com` or `http://127.0.0.1:40001` for local dev)
2. **Auth Token** — paste your Bearer auth token (from `runtime.auth_token` in `config/smackerel.yaml`)
3. Click **Test Connection** to verify
4. Click **Save Settings** when the test passes

### Usage

- **Toolbar button:** Click the Smackerel icon → **Save to Smackerel** to capture the current page
- **Context menu:** Right-click any page, link, or image → **Save to Smackerel**
- **Text selection:** Select text on a page → right-click → **Save with selection**
- **Offline queue:** If the server is unreachable, captures are queued locally and synced when connectivity returns

## PWA (Progressive Web App)

The PWA provides a mobile share target so you can send URLs and text to Smackerel from any app's Share menu.

### Installation

1. Open `http://127.0.0.1:40001/pwa/` in a mobile browser (Chrome on Android, Safari on iOS)
   - For HTTPS deployments: `https://smackerel.example.com/pwa/`
2. The browser displays an **Install** prompt (or tap the browser menu → **Add to Home Screen**)
3. Tap **Install** to add Smackerel to your home screen

> **HTTPS required for mobile install:** PWA installation requires HTTPS on mobile browsers. For local development, use `http://127.0.0.1` (localhost is exempt). For network-exposed deployments, set up a reverse proxy with TLS (see the [TLS Setup](#tls-setup) section above).

### Usage

Once installed, Smackerel appears as a share target on your device:

1. In any app (browser, notes, messaging), tap **Share**
2. Select **Smackerel** from the share sheet
3. The URL or text is captured to your Smackerel instance

The PWA uses the Web Share Target API to receive shared content. It posts the shared URL/text to the Smackerel capture endpoint using your configured auth token.

### PWA Troubleshooting

| Check | Resolution |
|-------|------------|
| Install prompt not showing? | Ensure you're on HTTPS (or localhost). Clear browser cache and revisit `/pwa/` |
| Share target not appearing? | The PWA must be installed to the home screen. Reinstall if missing |
| Captures failing? | Verify the Smackerel stack is running and the server URL is reachable from your device |
| Service worker not registering? | Check browser console for errors. The SW scope must match `/pwa/` |

## Chrome Extension Bridge — Sideload Workflow

The Chrome Extension Bridge (spec 058) is a separate Manifest V3 extension at
[`extensions/chrome-bridge/`](../extensions/chrome-bridge/) that streams live
`chrome.bookmarks` and `chrome.history` events into the smackerel-core ingest
endpoint `POST /v1/connectors/extension/ingest`. It is distinct from the
share-only extension under `web/extension/` and ships as its own signed
sideload zip from CI.

> **Authorization model:** the bridge requires a per-user PASETO minted with
> BOTH `extension:bookmarks` AND `extension:history` scopes (AND-semantics,
> spec 060). A legacy spec-044 token without scopes is rejected at the
> server with HTTP 403 `scope_required`.

### Sideload Workflow (Operator)

1. **Pick the build manifest.** Identify the smackerel-core git SHA running
   on your deployment (e.g. from `./smackerel.sh deploy-target home-lab
   verify`), then download the matching `build-manifest-<sourceSha>.yaml`
   from the GitHub Actions `build` workflow run. The manifest's
   `chromeBridge` section names the artifact and its SHA-256.

2. **Download the artifact triple** from the workflow-run artifacts (or, for
   tag-released builds, from the GitHub Release for the SHA):
   - `smackerel-chrome-bridge-<version>-<sha>.zip`
   - `smackerel-chrome-bridge-<version>-<sha>.zip.sha256`
   - `smackerel-chrome-bridge-<version>-<sha>.zip.sig`

3. **Verify the SHA-256** matches the value in
   `build-manifest-<sourceSha>.yaml` → `chromeBridge.zipSha256`:

   ```bash
   sha256sum -c smackerel-chrome-bridge-<version>-<sha>.zip.sha256
   ```

4. **Verify the cosign keyless signature** (Rekor-logged, no local keys
   required). The certificate identity is the GitHub Actions workflow ref
   under the project repository:

   ```bash
   cosign verify-blob \
     --signature smackerel-chrome-bridge-<version>-<sha>.zip.sig \
     --certificate-identity-regexp 'https://github.com/<owner>/smackerel/.github/workflows/build.yml@refs/.*' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     smackerel-chrome-bridge-<version>-<sha>.zip
   ```

5. **Unpack** the zip into a local directory (e.g. `~/chrome-bridge/`).

6. **Load unpacked** in Chrome:
   1. Open `chrome://extensions/`.
   2. Enable **Developer mode** (top-right toggle).
   3. Click **Load unpacked** and select the unpacked directory.
   4. The Smackerel Chrome Bridge entry appears in the extensions list.

7. **Configure the options page.** Click the extension's **Details** →
   **Extension options** and fill in:
   - **Base URL** — your smackerel-core URL (HTTPS in production; for local
     development, `http://127.0.0.1:<core port>`).
   - **Bearer token** — paste the wire token returned from
     `./smackerel.sh auth enroll --scope extension:bookmarks,extension:history`
     (the CLI flags ship with spec 060 Scope 3; until they land, mint the
     token through the admin API and ensure the `scope` claim carries both
     values). The token is masked by default after save.
   - **Source device id** — operator-chosen 1–32 char `[a-z0-9-]` label
     (e.g. `work-laptop`). The options page can auto-generate
     `auto-<uuidv4>`.
   - **Dedup window seconds** — accepted range `[60, 86400]`.
   - **Privacy allow / deny patterns** — up to 64 regex entries each
     (spec 058 OQ-DSN-3 — the cap protects the service-worker per-event
     budget).

8. **Verify by capturing.** Add or remove a bookmark; within ~60 seconds the
   artifact should appear via `GET /v1/artifacts?source=browser-extension`.
   The toolbar badge surfaces the current health:

   | Badge | Meaning | Resolution |
   |-------|---------|------------|
   | (empty) | Healthy and connected | — |
   | `SETUP` | Options page not yet configured | Open options and fill in base URL, token, device id |
   | `AUTH`  | Token rejected (401 or 403) | Mint a fresh token with both required scopes; revoke the prior token id |
   | `DEAD`  | Queue at the 24h dead-letter ceiling | Inspect server logs for the cause (rate-limit, schema reject); after fixing, reopen options and click **Drain queue** |

### Operator Caveats

- **Offline revocation latency (OQ-DSN-2).** Spec 044 propagates per-token
  revocation via NATS within ≤ 60 s for connected callers, but an offline
  extension only learns of a revoked token on its next POST. If you need
  immediate cutoff, disable the extension in `chrome://extensions/` until
  the device reconnects.
- **Privacy filter pattern cap (OQ-DSN-3).** Allow/deny lists are capped at
  64 entries each; exceeding the cap is rejected at options-page save time
  rather than silently truncated.
- **Chrome Sync.** When the operator's Chrome profile syncs bookmarks
  across multiple devices, each device produces a distinct
  `source_device_id` and the server-side dedup tuple treats them as
  separate observations.

### Admin Devices View

`GET /v1/admin/extension/devices` (see
[API.md](API.md#chrome-extension-bridge-ingestion)) aggregates rows from
`raw_ingest_dedup` for the `browser-extension` source and returns one entry
per `(owner_user_id, source_device_id)` pair with the first/last seen
timestamps and 30-day visit count. Admin sessions see every owner; non-admin
sessions see only their own devices.

## Cloud Drives Operations (Spec 038)

The cloud-drives surface (Google Drive provider in scope today) is operated through the `/v1/connectors/drive` and `/v1/drive/*` endpoints, the `DRIVE` NATS stream, and the `drive_*` PostgreSQL tables.

### Enabling A Drive Provider

1. Add OAuth credentials to `config/smackerel.yaml` under `drive.providers.<provider_id>` — required keys include `oauth_client_id`, `oauth_client_secret`, `oauth_redirect_url`, `oauth_base_url`, `api_base_url`, scan/monitor intervals, MIME allow-lists, and sensitivity thresholds. Empty secret values fail-loud at startup; do not rely on env fallbacks.
2. Regenerate config: `./smackerel.sh config generate`.
3. Restart: `./smackerel.sh down && ./smackerel.sh up`.
4. Connect a user account by issuing the OAuth web flow:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:40001/v1/connectors/drive/connect \
     -d '{"provider":"google","owner_user_id":"<uuid>","access_mode":"read_only","scope":{"folder_ids":["<id>"]}}'
   ```
   The handler returns an authorization URL; the user authorizes, and the provider redirects back to `/v1/connectors/drive/oauth/callback?state=…&code=…` which calls `FinalizeConnect`. A row lands in `drive_connections` with `status='healthy'`, the provider-supplied `expires_at`, and the bearer token in `credentials_ref`.

### Drive Connection Health

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/connectors/drive` | List provider catalog and capabilities (provider-neutral). |
| `GET /v1/connectors/drive/connection/{id}` | Inspect a single connection (status, scope, last health reason, expires_at). Returns `404 CONNECTION_NOT_FOUND` for unknown ids. |
| `GET /v1/connectors/drive/connection/{id}/skipped` | Group skipped/blocked files by reason for Screen 4. |

**Token expiry behaviour.** Per spec 038 design.md §2.3 + decision-log A1, only the bearer (access) token is persisted; refresh tokens are intentionally not stored in this scope. When `expires_at` passes, the connection moves out of `healthy`; the user must re-authorize through the OAuth flow above. A dedicated credentials vault is a follow-up scope and MUST move both access and refresh tokens out of `credentials_ref` when it lands.

### Save Rules And Save Service

The Save Rules engine (`/v1/drive/rules`) and Save Service (`/v1/drive/save`) gate every provider write. All endpoints require Bearer auth.

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/drive/rules` / `POST /v1/drive/rules` | List or create Save Rules. |
| `GET/PUT/DELETE /v1/drive/rules/{id}` | Inspect, update, or delete a single rule. |
| `POST /v1/drive/rules/{id}/test` | Screen 8 dry-run — evaluate a rule against a candidate artifact without committing. |
| `GET /v1/drive/rules/audit` | Screen 7 audit feed — first-stable-match outcomes plus all conflicting matches per evaluation. |
| `POST /v1/drive/save` | Submit a save request; the rule engine evaluates, the Save Service routes through the provider writer, and a row lands in `drive_save_requests`. |
| `GET /v1/drive/save/requests` | Recent save requests for Screen 7. |

### Low-Confidence Confirmation

Low-confidence routing decisions and sensitive-content saves are paused at the Save Service and surfaced through Screen 11 and the Telegram numbered-reply path. Both channels share one handler so the exactly-once contract holds.

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/drive/confirmations/{id}` | Fetch the pending confirmation payload. |
| `POST /v1/drive/confirmations/{id}` | Resolve a confirmation (`confirm` / `reject`). The first call wins; subsequent calls return the resolved state. |

### Drive Search And Artifact Detail

Drive content participates in the standard `/api/search` semantic surface. The Drive-specific artifact detail endpoint exposes folder context, version history, save provenance, and skipped/blocked grouping:

```bash
curl -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/v1/drive/artifacts/<artifact-id>
```

### Resetting A Drive Cursor

Drive providers use `drive_cursors(provider_id, connection_id, cursor, valid_until)`. To force a bulk re-scan after a provider cursor invalidation:

```sql
DELETE FROM drive_cursors WHERE connection_id = '<uuid>';
```

The next monitor cycle falls back to a bounded rescan of in-scope folders, computes a delta against `drive_files`, and re-issues only those deltas as change events.

### Drive Database Tables

`drive_connections`, `drive_oauth_states`, `drive_files`, `drive_folders`, `drive_cursors`, `drive_rules`, `drive_save_requests`, `drive_folder_resolutions`, `drive_rule_audit`, `drive_scan_jobs`, `drive_provider_work_queue`, `drive_confirmations`, `drive_share_changes`, plus the consolidated migrations 021/024/030. Backups produced by `./smackerel.sh backup` cover all of them.

### Drive Observability Metrics And Alerts

The provider-neutral drive surface emits two metric families. The first five live in `internal/drive/observability/metrics.go` and are incremented by the scan / extract / save / retrieve services (one counter + one structured log per outcome). The labels are bounded enums — never a connection id or file id.

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_drive_scan_files_total` | Counter | `provider`, `outcome` | Files observed by the scan/monitor pipeline (ok/skipped/blocked/error). |
| `smackerel_drive_extract_files_total` | Counter | `provider`, `outcome` | Files through extraction/classification (ok/skipped/blocked/error). |
| `smackerel_drive_save_attempts_total` | Counter | `provider`, `outcome` | Save-back attempts (ok/skipped/blocked/refused/error). |
| `smackerel_drive_retrieve_decisions_total` | Counter | `provider`, `mode` | Retrieve decisions (bytes/secure_link/provider_link/refused/disambiguate). |
| `smackerel_drive_provider_errors_total` | Counter | `provider`, `work_type` | Provider-side error events (scan/extract/save/retrieve). |

A second set of Scope 6 policy/confirmation counters lives in `internal/metrics/metrics.go`: `smackerel_drive_confirmations_total{status,channel}` (low-confidence confirmation resolutions), `smackerel_drive_policy_decisions_total{surface,decision,sensitivity}` (sensitivity policy verdicts), and `smackerel_drive_rule_conflicts_total{rule_id}` (Save Rule conflict audit per stable-winner rule).

All counters pre-instantiate their bounded label families at container start, so the HELP/TYPE lines are visible at `/metrics` before the first drive operation fires.

**Push-based alerts** (`config/prometheus/alerts.yml`, group `smackerel-drive`, spec 038 round 30 devops sweep `F-038-DEVOPS-001`):

| Alert | Expression (summary) | Fires when |
|-------|----------------------|------------|
| `DriveProviderErrors` | `rate(smackerel_drive_provider_errors_total[10m]) > 0.05` per `(provider, work_type)`, for 10m | The drive provider API is failing or the stored OAuth bearer token expired (only the access token is persisted — design §2.3 — so expiry breaks every drive operation until re-auth). |
| `DriveSaveBackFailing` | `rate(smackerel_drive_save_attempts_total{outcome="error"}[15m]) > 0.05` per `provider`, for 15m | Cross-feature write production (FR-038-004 / FR-038-012 — meal plans, receipts, digests routed to a folder) is failing at the provider boundary (distinct from deliberate `refused`/`blocked`). |
| `DriveRetrieveRefusalSpike` | `rate(smackerel_drive_retrieve_decisions_total{mode="refused"}[10m]) > 0.1` per `provider`, for 15m | The retrieval boundary (FR-038-010 — Telegram/other channels subject to sensitivity + sharing policy) is refusing en masse: a regressed policy blocking legitimate retrievals, or a channel probing the boundary. This is the surface an earlier chaos round hardened against memory exhaustion. |

Alertmanager routing (Telegram, PagerDuty, etc.) is deliberately NOT shipped in this repo — receivers belong to the deploy-adapter overlay so the product repo stays target-agnostic. The `smackerel-drive` group references metrics emitted outside `internal/metrics/`, so the alert-contract allowlist in `internal/deploy/monitoring_alerts_contract_test.go` walks `internal/drive/observability/metrics.go`; a fabricated or renamed drive metric in an alert expression fails that contract at `./smackerel.sh test unit --go`.

## Cloud Photo Libraries Operations (Spec 040)

The cloud-photos surface (Immich and PhotoPrism providers in scope today) is operated through the `/v1/photos/*` endpoints, the `PHOTOS` NATS stream, and the `photo_*` PostgreSQL tables.

### Enabling A Photo Provider

1. Add provider credentials under `photos.providers.<provider>` in `config/smackerel.yaml` (`base_url` + `access_token` for both Immich and PhotoPrism). Empty `access_token` values fail-loud at startup.
2. Regenerate config: `./smackerel.sh config generate`.
3. Restart: `./smackerel.sh down && ./smackerel.sh up`.
4. Test the connection without persisting it:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:40001/v1/photos/connectors/test \
     -d '{"provider":"immich","base_url":"https://immich.example.com","access_token":"<key>"}'
   ```
5. Connect:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:40001/v1/photos/connectors \
     -d '{"provider":"immich","base_url":"https://immich.example.com","access_token":"<key>"}'
   ```

### Photo Provider Endpoints

All `/v1/photos/*` endpoints require Bearer auth.

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/photos/connectors` | List configured photo providers (Immich, PhotoPrism). |
| `POST /v1/photos/connectors` | Create a new provider connection. |
| `POST /v1/photos/connectors/test` | Test credentials without persisting. |
| `GET /v1/photos/connectors/{id}` | Inspect a single connection. |
| `POST /v1/photos/connectors/capabilities/{capability}/exercise` | Exercise a provider capability; unsupported operations return `409 PROVIDER_LIMITATION` with a stable `LimitationCode`. |
| `GET /v1/photos/health` | Aggregate photo health (sync progress, capability limitations). |
| `GET /v1/photos/search?q=<text>` | Semantic photo search; sensitive results omit preview URLs and set `requires_reveal=true`. |
| `GET /v1/photos/{id}` | Fetch a photo record. |
| `GET /v1/photos/{id}/preview?size=thumb\|full` | Sensitive previews return `403 sensitivity_requires_reveal` without a reveal token. |
| `POST /v1/photos/{id}/reveal` | Mint a single-use, actor-bound, TTL + hash-protected reveal token. |
| `POST /v1/photos/upload` | Unified multipart upload pipeline shared by Telegram, the mobile PWA, and web; preserves `source_channel` + `source_ref`. |

### Lifecycle, Duplicates, And Removal Review

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/photos/health/lifecycle` | RAW-to-processed lifecycle dashboard (editor signature, confidence, rationale, review_state). |
| `GET /v1/photos/health/duplicates` | Duplicate clusters (exact, burst, HDR, panorama, near-duplicate, cross-provider). |
| `GET /v1/photos/health/duplicates/{id}` | Inspect a single cluster. |
| `POST /v1/photos/health/duplicates/{id}/best-pick` | Set the best-pick photo for a cluster. |
| `POST /v1/photos/health/duplicates/{id}/resolve` | Resolve a cluster (keep / archive / delete). |
| `GET /v1/photos/health/removal` | Removal-candidate review with reason + confidence + rationale + method, in reversible decision states. |
| `GET /v1/photos/health/quality` | Quality dashboard (Scope 3 placeholder rows). |

### Action Tokens And Destructive Confirmation

Every destructive write (archive, delete, album removal) MUST flow through plan → confirm:

```bash
# Plan a destructive action — returns a scope-hashed PhotoActionToken.
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:40001/v1/photos/actions/plan \
  -d '{"action":"delete","scope":{"photo_ids":["<uuid>"]}}'

# Confirm with the token (and a text-confirmation phrase for delete).
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:40001/v1/photos/actions/confirm \
  -d '{"token":"<photo_action_token>","confirmation":"DELETE"}'
```

`ConfirmedWriter` wraps every `ProviderWriter` so a write cannot fire before confirmation. If the planning scope changes between plan and confirm (scope-hash drift), the confirm call is rejected.

### Capability Taxonomy

Provider capability gaps surface through one taxonomy (`internal/connector/photos/capability_taxonomy.go`) used by:

- API responses (`409 PROVIDER_LIMITATION` envelopes carry a `LimitationCode`).
- Prometheus metrics (`smackerel_photos_capabilities_limited_total{code=…}`).
- The PWA Photo Health dashboard banner strings.

The taxonomy canary integration test asserts those three surfaces stay in sync. When adding a new provider, register limitations through the taxonomy registry — never inline ad-hoc strings.

### Sensitivity Reveal Tokens

Sensitive photo previews are gated server-side. The reveal token lifecycle:

1. Search and detail endpoints return `requires_reveal=true` for sensitive rows and omit preview URLs.
2. Calling `GET /v1/photos/{id}/preview` without a reveal token returns `403 sensitivity_requires_reveal`.
3. The user (or surface) requests a token via `POST /v1/photos/{id}/reveal`.
4. The token is single-use, actor-bound, TTL-bounded, and hash-protected; reuse fails closed.
5. Telegram's `handleFind` substitutes a reveal-required notice for sensitive results so the bot does not auto-deliver sensitive content.

### Photo Database Tables

Migration `025_photo_libraries.sql` plus `029_photo_scope3_lifecycle_dedupe_removal.sql` and `031_photo_scope4_capture_routing_sensitivity.sql` provide: `photo_lifecycle_links`, `photo_clusters`, `photo_cluster_members`, `photo_removal_candidates`, `photo_capabilities`, `photo_sync_state`, `photo_face_links`, `photo_embeddings`, `photo_action_tokens`, `photo_audit_events`, `photo_raw_export_links`, `photo_routing_decisions`, `photo_document_groups`. All are covered by the standard `./smackerel.sh backup` and the disposable test stack.

## Model Envelope Sizing (Spec 045 / BUG-045-001)

Smackerel's local-inference model selection MUST fit two independent memory envelopes:

| Envelope | Source value (`config/smackerel.yaml`) | Default | Bucket members |
|----------|----------------------------------------|---------|----------------|
| Ollama envelope | `deploy_resources.ollama.memory_limit` → `OLLAMA_MEMORY_LIMIT_MIB` | `8G` (8192 MiB) | All `llm.*` model fields + `extract.local.model` + `synthesizer.local.model` + `topics.label.model` + `topics.summary.model` + `recipe.*.local_model` + `meal_plan.*.local_model` (15 ollama-routed model_ref slots) |
| ML-sidecar envelope | `deploy_resources.ml.memory_limit` → `ML_MEMORY_LIMIT_MIB` | `3G` (3072 MiB) | `embedder.local.model` + `ocr.local.model` (2 ml-sidecar-routed model_ref slots) |

### Why two envelopes

The Go core's `internal/config/config.go::validateModelEnvelopes` (post-BUG-045-001) splits the validation into two `envelopeBucket` structures so that the ml-sidecar memory ceiling cannot be exceeded by ollama-routed models and vice versa. Before BUG-045-001 the validator conflated both buckets under `ML_MEMORY_LIMIT`, which let a 30+ GB ollama model pass through dev sandboxes and crash ollama at first inference on deploy targets.

### Default-model rebalance (DD-5)

To restore default startup health on the 8 GiB ollama envelope, BUG-045-001 Scope 3 rebalanced the default-model selection across every ollama-routed model_ref slot:

| Slot | Before | After | Resident size |
|------|--------|-------|---------------|
| `llm.model` (default-mode LLM gateway) | `gemma4:26b` | `gemma3:4b` | 4096 MiB |
| `llm.fast_mode_models[0..3]` | `gemma4:26b` (mixed with `gemma3:4b`) | `gemma3:4b` | 4096 MiB |
| `llm.reasoning_model` | `deepseek-r1:32b` | `deepseek-r1:7b` | 4864 MiB |
| `extract.local.model` | `gemma4:26b` | `gemma3:4b` | 4096 MiB |
| `synthesizer.local.model` | `gemma4:26b` | `gemma3:4b` | 4096 MiB |
| `topics.label.model` | `gemma4:26b` | `gemma3:4b` | 4096 MiB |
| `topics.summary.model` | `gpt-oss:20b` | `gemma3:4b` | 4096 MiB |
| `recipe.import.local_model` | `gemma4:26b` | `gemma3:4b` | 4096 MiB |
| `recipe.enrichment.local_model` | `gemma4:26b` | `gemma3:4b` | 4096 MiB |
| `meal_plan.suggestion.local_model` | `gemma4:26b` | `gemma3:4b` | 4096 MiB |

The ml-sidecar envelope is unchanged: `embedder.local.model` = `nomic-embed-text` (768 MiB) and `ocr.local.model` = `deepseek-ocr:3b` (2560 MiB) both fit the 3 GiB ml-sidecar ceiling.

### Model memory profiles catalog

The `model_memory_profiles` map in `config/smackerel.yaml` declares the resident-size ceiling for every model name a validator may encounter. BUG-045-001 added two new entries to support the rebalance:

| Model name | Resident size (MiB) | Library card |
|------------|---------------------|--------------|
| `gemma3:4b` | 4096 | <https://ollama.com/library/gemma3> |
| `deepseek-r1:7b` | 4864 | <https://ollama.com/library/deepseek-r1> |

Pre-existing entries for `gemma4:26b` (18432 MiB), `deepseek-r1:32b` (22528 MiB), `gpt-oss:20b` (14336 MiB), and all other catalogued models remain in place. They are NOT removed because the operator opt-up path (see below) still needs to validate them. (Spec 082 SCOPE-082-02 reconciled these three figures + the two ml-sidecar figures below to the `config/smackerel.yaml` `model_memory_profiles` SST authority, which had drifted from this doc.)

### Operator opt-up path

An operator with more RAM headroom (e.g., a 24 GiB ollama envelope) can opt up to a heavier default model without editing this repo:

1. In the operator's deploy-adapter overlay, set `deploy_resources.ollama.memory_limit: 24G` (or whatever fits).
2. In the same overlay (or via env var injection at apply time), override the relevant `llm.*` / `topics.*` / `synthesizer.*` / `recipe.*` / `meal_plan.*` model fields with a model name whose `model_memory_profiles` entry fits the new envelope.
3. Run `./smackerel.sh deploy-target <target> apply ...`. The build manifest's `config/smackerel.yaml` is the upstream default; the overlay's resolved values take precedence inside the operator's bundle.

The overlay MUST keep the per-service envelope invariant: the sum of resident sizes for models routed to ollama MUST NOT exceed `OLLAMA_MEMORY_LIMIT_MIB`, and the sum routed to the ml-sidecar MUST NOT exceed `ML_MEMORY_LIMIT_MIB`. The validator rejects any overlay that violates this.

### Pre-emit gate as the structural safety net

The `./smackerel.sh config generate --env <env>` pipeline now runs `cmd/config-validate` against the rendered env file BEFORE atomic-promoting `<env>.env.tmp` → `<env>.env` (DD-2). The atomic-promote sequence (`scripts/commands/config.sh::smackerel_generate_config`):

1. `<env>.env.tmp` is written via heredoc.
2. `chmod 0600 <env>.env.tmp` clamps permissions before validation.
3. `cmd/config-validate --env-file=<env>.env.tmp` exit code is consulted.
4. On exit 0: `mv -f <env>.env.tmp <env>.env` (promote).
5. On non-zero: `rm -f <env>.env.tmp` (no leak; previous `<env>.env` is preserved).

This means an envelope-violating override (whether authored by an operator, an upstream merge, or a runtime experiment) cannot land in `config/generated/<env>.env`. The stack will fail to start with a clear validator error instead of crashing ollama at first inference on the deploy target. The pre-emit gate is honored by every codepath that lands on `smackerel_generate_config` — including `./smackerel.sh check` (Compose render preflight), `./smackerel.sh up` (which runs `config generate` first), and `./smackerel.sh test integration` (which runs `config generate --env test` first).

The integration harness can substitute a precompiled validator binary by setting `SMACKEREL_CONFIG_VALIDATE_BIN` before invoking `smackerel_generate_config`; the default path falls back to `go run ./cmd/config-validate`. The gate also skips when `SHELL_PRODUCTION_CLASS_TARGETS` is empty (i.e., not a production-class generation run).

### Concurrent interactive-set envelope guard (Spec 082 SCOPE-082-02)

The per-model envelope check above proves each configured model fits the
ollama envelope **alone**. It does NOT, by itself, prove that the models the
runtime keeps **co-resident** fit together. Under a resident `OLLAMA_KEEP_ALIVE`
(`-1` = never unload, or any duration ≥ 10 minutes such as the home-lab/prod
`24h`), ollama retains every model it loads, so the distinct interactive
hot-path models are simultaneously resident and their **sum** must also fit
`OLLAMA_MEMORY_LIMIT`. If it does not, Docker OOM-kills the ollama container
into a restart crash-loop the first time the second large model loads.

`internal/config/config.go::validateModelEnvelopes` now sums the distinct
interactive hot-path slots — `LLM_MODEL`, `OLLAMA_MODEL`, `OLLAMA_VISION_MODEL`,
`AGENT_PROVIDER_DEFAULT_MODEL`, `AGENT_PROVIDER_FAST_MODEL`,
`AGENT_PROVIDER_VISION_MODEL` — and fails loud (at `./smackerel.sh config
generate` time, via the pre-emit gate) when that sum exceeds
`OLLAMA_MEMORY_LIMIT` AND keep-alive is resident. On-demand specialists
(reasoning, OCR, photo-intelligence batch) are governed by the per-model
individual check above; operators running sustained concurrent
reasoning+OCR+chat workloads on a long keep-alive should add further headroom
or shorten `OLLAMA_KEEP_ALIVE`.

| Env | Interactive distinct set | Sum (MiB) | `OLLAMA_MEMORY_LIMIT` | Result |
|-----|--------------------------|-----------|-----------------------|--------|
| dev | `gemma3:4b` + `qwen2.5:0.5b-instruct` | 5120 | 8G (8192) | fits |
| test | `qwen2.5:0.5b-instruct` | 1024 | 8G (8192) | fits |
| home-lab | `gemma4:26b` + `llama3.1:8b` | 24576 | **28G (28672)** | fits |

The home-lab `ollama_memory_limit` was raised `20G → 28G` (Spec 082) precisely
because the old 20G floor was sized for `gemma4:26b` alone (18432 MiB) and the
`+ llama3.1:8b` agent default/fast model pushed the resident set to 24576 MiB,
over-subscribing the cgroup. The <deploy-host> host has ~109 GiB RAM, so 28G leaves
~4 GiB headroom for KV-cache growth.

## QF Companion Connector Operations (Spec 041 Scope 5)

The QF companion connector (`internal/connector/qfdecisions`) is a read-only
bridge into the QF (quantitativeFinance) companion surface. Scope 5 hardens
credential rotation, completes the symmetric metric set, locks in the pre-MVP
safety boundary, and rolls out Cross-Product Audit Envelope v1 across every
Smackerel-side QF emission point.

### Credential Rotation Overlap

QF companion credentials rotate via `(*Connector).RotateCredentials`, which
delegates to the pure planner
`internal/connector/qfdecisions/credentials.go::PlanCredentialRotation`. The
operator-facing contract:

- Exactly two active credentials may be supplied; rotation rejects any other
  count with diagnostic `expected_exactly_two_active_credentials`.
- The overlap between the two credentials' `not_before` / `not_after`
  intervals MUST NOT exceed **24 hours**. An overlap beyond 24 hours is
  rejected with diagnostic `credential_overlap_exceeds_24h`.
- The newest valid credential is selected by `not_before` (most recent wins).
- Future-only credentials (whose `not_before` is in the future at rotation
  time) are rejected with diagnostic `credential_not_active`.
- Inverted credential windows (`not_after <= not_before`) are rejected with
  diagnostic `credential_window_inverted`.

### Capability Re-Read On Rotation Start

On every successful rotation, `RotateCredentials` invokes the QF bridge
capability fetch with the newly selected credential **before** any sync,
render, or evidence export call uses it. The `capability_re_read_required`
diagnostic is always present on a successful plan. A failed capability
fetch aborts the rotation and emits an `outcome=error` audit envelope; the
previous credential remains in use until the next successful rotation.

### State Preservation Through Rotation

A successful rotation preserves the following state verbatim:

- `sync_state.sync_cursor` — the QF cursor checkpoint persisted by Scope 2.
- Persisted QF capability response (`capability_response_json`,
  `capability_fetched_at`, `capability_status`) and the
  `EvidenceExportIDs` idempotency record persisted by Scope 4.

Rotation MUST NOT create a new connector identity, clear cursor state,
duplicate evidence exports, or reset revoked / failed export records. The
`sync_cursor_preserved` and `evidence_export_state_preserved` diagnostics
are always present on a successful plan.

### Symmetric `smackerel_qf_*` Metric Reference

Twelve QF-specific metrics are declared and registered exactly once at
process init in `internal/metrics/metrics.go` (declarations at lines
238-388, registrations at lines 395-449). Label parity with QF design 063
is enforced by `internal/metrics/metrics_test.go` and
`internal/connector/qfdecisions/metrics_test.go`.

| Metric | Labels | Emission point |
|--------|--------|----------------|
| `smackerel_qf_packet_ingest_total` | `event_type`, `decision_type`, `approval_state`, `source_surface` | Scope 2 sync per ingested packet |
| `smackerel_qf_packet_validation_failures_total` | `reason` | Scope 2 sync validation failure |
| `smackerel_qf_evidence_export_attempts_total` | `status`, `target_context_type`, `sensitivity_tier` | Scope 4 evidence export attempt |
| `smackerel_qf_cursor_lag_seconds` | (gauge, no labels) | Scope 2 sync per cursor checkpoint |
| `smackerel_qf_action_boundary_attempts_total` | `attempted_action_type` | Scope 5 boundary kick on any rejected QF action attempt (sync diagnostic / render / export) |
| `smackerel_qf_capability_mismatch_total` | `required`, `actual` | Scope 2 capability handshake on mismatch |
| `smackerel_qf_unknown_decision_type_total` | `value` | Scope 2 unknown `decision_type` packet |
| `smackerel_qf_engagement_signal_attempts_total` | `event`, `surface`, `status` | Scope 6 engagement flush — emitted once per `Exporter.flushBatch` outcome (`accepted` on HTTP 201/200 idempotent-repeat, `rejected` on HTTP 4xx, `degraded` on HTTP 5xx or transport timeout after bounded retry budget) plus one `overflow_drop`-event `dropped` emission at enqueue time when the 1024-event buffer evicts the OLDEST entry. Pre-registered by Scope 5; actively emitted by the Scope 6 exporter in `internal/connector/qfdecisions/engagement.go` (transport delivered). |
| `smackerel_qf_evidence_revoked_total` | `reason` | Scope 4 evidence revocation |
| `smackerel_qf_callback_attempts_total` | `action`, `status` | Scope 8 callback attempt — emitted once per `PostCallback` invocation (action vocabulary `{noop, open, unknown}`, status vocabulary `{ok, rejected_v1_deferred, rejected_local, error}`) |
| `smackerel_qf_watch_proposal_attempts_total` | `status` | Scope 9 watch-proposal POST attempt — emitted once per `WatchProposalClient.Propose` invocation (status vocabulary `{rejected_v1_deferred, rejected_local, degraded}`; pre-MVP `accepted` is NEVER emitted because QF rejects every proposal with `WATCH_PROPOSALS_DEFERRED_TO_V1`) |
| `smackerel_qf_deep_link_render_total` | `surface`, `status` | Scope 3 deep-link render |
| `smackerel_qf_trust_object_render_failures_total` | `reason` | Scope 3 trust-object render failure |

Scope 5 also extends the freshness gauge `smackerel_qf_freshness_p95_seconds`
(label: `stage`) with the `render` and `total` stages on top of the existing
`ingest` stage, closing the C-S2-321B-SCOPE-5-RENDER dependency.

### Cross-Product Audit Envelope V1

Every Smackerel-side QF bridge emission point logs a
`qf-decisions: cross_product_audit` record via the structured-logging sink in
`internal/connector/qfdecisions/audit.go::EmitConnectorAuditEnvelope`. The
envelope fields (per QF design 063 mirror):

| Field | Always present? | Source |
|-------|-----------------|--------|
| `audit_envelope_version` | yes | Persisted capability response (`v1` today) |
| `trace_id` | yes | Caller-supplied |
| `actor_ref` | yes | Defaults to `smackerel_connector` when caller-empty |
| `surface` | yes | Defaults to `qf-decisions` when caller-empty |
| `action` | yes | One of the action constants below |
| `outcome` | yes | `ok`, `rejected`, `error`, `degraded`, or domain-specific |
| `reason` | yes | Free-form caller-supplied |
| `ts` | yes | RFC3339 UTC observed timestamp |
| `recorded_at` | yes | RFC3339 UTC matching `ts` |
| `packet_id` | per-event | Set on packet-bound events |
| `export_id` | per-event | Set on evidence-export-bound events |
| `signal_id` | per-event | Set on engagement-signal events |
| `bundle_id` | per-event | Set on evidence-bundle events |
| `target_context_type` | per-event | Set on evidence-export events |
| `sensitivity_tier` | per-event | Set on evidence-export events |

Action constants (`internal/connector/qfdecisions/types.go`):

| Constant | Wire value | Emission point |
|----------|-----------|----------------|
| `AuditActionPacketIngest` | `packet_ingest` | Scope 2 sync per ingested packet |
| `AuditActionEvidenceExportAttempt` | `evidence_export_attempt` | Scope 4 evidence export attempt |
| `AuditActionEvidenceRevocation` | `evidence_revocation` | Scope 4 evidence revocation |
| `AuditActionEngagementSignalFlush` | `engagement_signal_flush` | Scope 6 engagement flush (helper present; transport in Scope 6) |
| `AuditActionCallbackAttempt` | `callback_attempt` | Scope 8 callback attempt — emitted on every `PostCallback` invocation (success, parsed QF rejection, local signature failure, and transport error all log one envelope) |
| `AuditActionWatchProposalAttempt` | `watch_proposal` | Scope 9 watch-proposal attempt — emitted on every `WatchProposalClient.Propose` invocation (parsed QF `WATCH_PROPOSALS_DEFERRED_TO_V1` rejection, local signature failure, and transport error all log one envelope; pre-MVP `outcome` is always `rejected`/`degraded`, never `ok`) |
| `AuditActionDeepLinkRender` | `deep_link_render` | Scope 3 deep-link render |
| `AuditActionCapabilityHandshake` | `capability_handshake` | Scope 2 capability handshake |
| `AuditActionActionBoundaryKick` | `action_boundary_kick` | Scope 5 boundary kick on any rejected QF action attempt |
| `AuditActionCredentialRotation` | `credential_rotation` | Scope 5 rotation start / complete / reject |

The connector audit-log sink (Smackerel-local slog stream) is the canonical
destination for these envelopes today. The QF mirror sink (forwarding
envelopes back to QF's audit ingestion surface) is explicitly reserved
post-MVP and opt-in; no MVP runtime configuration enables it.

### Pre-MVP Safety Boundary

The QF companion connector is **read-only**. The shared safety-boundary
helper `EnforceQFActionBoundary`
(`internal/connector/qfdecisions/boundary.go`) is called from every sync,
render, and evidence-export code path. The following action types are
unconditionally rejected pre-MVP and bump
`smackerel_qf_action_boundary_attempts_total{attempted_action_type}`:

| `attempted_action_type` | Wire value |
|-------------------------|-----------|
| Approval | `approval` |
| Execution | `execution` |
| Mandate change | `mandate_change` |
| EmergencyStop | `emergency_stop` |
| Watch creation | `watch_creation` |
| Watch evaluation | `watch_evaluation` |
| Callback acceptance | `callback_acceptance` |
| QF trust reconstruction | `qf_trust_reconstruction` |

No MVP code path enables any of these actions; no MVP audit-sink wiring
forwards Smackerel envelopes to QF.

### QF Mirror Sink (Reserved Post-MVP)

The QF mirror audit sink is documented as a deliberate post-MVP surface. It
remains opt-in only: no default configuration toggles it on, no SST key
enables it, and no production code path writes to it today. Operators
considering the post-MVP mirror should treat it as a new scope activation
requiring an explicit planning round and explicit operator consent.

## QF Companion Connector Operations (Spec 041 Scope 6)

Scope 6 ships the packet engagement signal exporter — a write-only,
buffered, observability-oriented pipeline that captures user engagement
events ( `opened`, `dwell`, `dismissed`, `snoozed`, `deep_linked`,
`shared`) across the web detail surface, the daily digest surface, and
the Telegram surface, batches them, and flushes them to QF's
`POST /api/private/smackerel/v1/packet-engagement-signals` endpoint via
the Scope 1 QF client transport.

### Capability Gate (Construction Time)

The exporter checks `engagement_signal_supported` on the persisted QF
capability response ONCE, at exporter construction time inside
`(*Connector).Connect`. When the field is `false` the returned exporter
is in a permanently-disabled state: every `Capture` call is a no-op,
no flush worker is started, no metric is emitted, and no audit
envelope is written. Capability is NEVER re-read per `Capture` call;
the only way to flip an exporter from disabled to enabled is to tear
down the connector via `Close` and reconnect after the QF capability
handshake re-runs.

### Consent Gate (Event-Capture Time)

Consent is checked at event-capture time, NOT at flush time. The user's
`engagement_telemetry` privacy preference must be one of:

- `off` / `engagement_telemetry_off` — buffer is bypassed entirely at
  capture time regardless of which surface fires the event;
- `anonymous` / `engagement_telemetry_anonymous` — signals are enqueued
  with `consent_scope=anonymous` and `actor_ref=""` (no PII);
- `pseudonymous` / `engagement_telemetry_pseudonymous` — signals are
  enqueued with `consent_scope=pseudonymous` and `actor_ref` set to the
  caller-supplied opaque pseudonym.

The default consent reader installed at the package level is
fail-closed (`engagement_telemetry_off`). Operators wire a real
consent reader by calling
`qfdecisions.SetEngagementConsentReader(reader qfdecisions.ConsentReader)`
during core process bootstrap.

### Flush Policy (Fixed)

| Knob | Value | Rationale |
|---|---|---|
| Flush interval | 10s | Bounded freshness without sub-second flush storms |
| Flush threshold | 100 events | Batch size that matches the QF endpoint's documented intake budget |
| Buffer capacity | 1024 events | Bounded memory; overflow drops the OLDEST entry to keep recent signals fresh |
| Max retry attempts | 3 | Bounded retry budget; per design.md §Failure Handling |
| Initial backoff | 100ms | First retry delay |
| Max backoff | 2s | Cap on exponential growth |
| Engagement transport timeout | 5s | Per-attempt timeout against the QF endpoint |

The buffer is NOT persisted across process restarts. Engagement signals
in flight at shutdown are flushed best-effort by `Close`; un-flushable
signals are dropped.

### Metric Reference (Scope 6 emissions)

The Scope 6 exporter EMITS samples on the pre-registered Scope 5 vector
`smackerel_qf_engagement_signal_attempts_total{event,surface,status}`.
No new vector is registered. The `reason` label is NOT carried on the
metric (cardinality bound); the QF response reason code is carried only
on the audit envelope.

| Label | Vocabulary |
|---|---|
| `event` | `opened`, `dwell`, `dismissed`, `snoozed`, `deep_linked`, `shared`, `overflow_drop` |
| `surface` | `web`, `digest`, `telegram` |
| `status` | `accepted`, `rejected`, `degraded`, `dropped` |

Per-status emission rules:

- `accepted` — QF responded with HTTP 201 OR HTTP 200 idempotent-repeat;
- `rejected` — QF responded with HTTP 4xx (drop without retry);
- `degraded` — QF responded with HTTP 5xx OR the transport timed out
  after the bounded retry budget exhausted;
- `dropped` — exporter buffer overflowed and the OLDEST entry was
  evicted at enqueue time.

### Audit Envelope Reference (Scope 6 emissions)

Every flush attempt OR overflow-drop emits one Cross-Product Audit
Envelope v1 record via the Scope 5 builder
(`BuildCrossProductAuditEnvelopeV1`) with:

| Field | Value |
|---|---|
| `action` | `engagement_signal_flush` (`AuditActionEngagementSignalFlush`) |
| `outcome` | `ok` (accepted), `rejected` (4xx), `degraded` (5xx or overflow) |
| `reason` | QF response `reason` code for 4xx/5xx; `ENGAGEMENT_BUFFER_OVERFLOW` for overflow; empty for `ok` |
| `surface` | The surface that fired the source events (envelope-level for batch flushes) |
| `actor_ref` | The opaque pseudonym from the consent reader (only set when consent scope is `pseudonymous`) |
| `signal_ids` | The list of signal IDs in the flushed batch |

Idempotent-repeat responses (HTTP 200) increment the `accepted` metric
but MUST NOT emit a duplicate audit envelope beyond the first `ok`
envelope for the same `signal_id` set.

### Write-Only Boundary (Product Principle 10)

The exporter is write-only. Engagement signals are NEVER read back
into local rendering, ranking, digest priority, recommendation
surfaces, or trust metadata. The Smackerel → QF observability channel
is one-way; QF owns interpretation of engagement signals for
calibration, and Smackerel never reflects engagement back onto its own
surfaces. This invariant is enforced at the Change Boundary level
(every excluded file family in `scopes.md` Scope 6 Change Boundary
section names the read-back surfaces explicitly) and at the code level
(`internal/connector/qfdecisions/engagement.go` exposes only
write-path APIs; no `ReadEngagementSignal*` accessor exists).

### Failure Matrix Quick Reference

| QF response | Retry? | Metric `status` | Audit `outcome` | Audit `reason` |
|---|---|---|---|---|
| HTTP 201 | No | `accepted` | `ok` | `""` |
| HTTP 200 idempotent-repeat | No | `accepted` | `ok` (no duplicate envelope) | `""` |
| HTTP 4xx (409 `ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD`, `ENGAGEMENT_PACKET_NOT_FOUND`, `ENGAGEMENT_TRACE_ID_MISMATCH`, `ENGAGEMENT_CONSENT_REQUIRED`, `ENGAGEMENT_DWELL_FIELD_MISMATCH`) | No | `rejected` | `rejected` | QF response `reason` |
| HTTP 5xx | Yes — exponential backoff up to 3 attempts | `degraded` | `degraded` | QF response `reason` if present, else `"transport_failed"` |
| Transport timeout / connection refused | Yes — exponential backoff up to 3 attempts | `degraded` | `degraded` | `"transport_failed"` |
| Buffer overflow (at enqueue time) | N/A | `dropped` | `degraded` | `ENGAGEMENT_BUFFER_OVERFLOW` |

### Operator Hooks

| Hook | API | Purpose |
|---|---|---|
| Install runtime consent reader | `qfdecisions.SetEngagementConsentReader(reader)` | Wire the user's `engagement_telemetry` privacy preference to the global exporter at bootstrap. Fail-closed (`off`) by default. |
| Capture an engagement event | `qfdecisions.CaptureEngagementOpened(ctx, surface, packetID, traceID, actorRef)` | Surface render hooks call this nil-safely; no-ops when no exporter is connected. Follow-up scopes may add additional capture helpers for `dwell`, `dismissed`, `snoozed`, `deep_linked`, `shared` events. |
| Access the connected exporter | `(*Connector).EngagementExporter() *Exporter` | Test/operator-facing accessor for the exporter created at `Connect` time; nil after `Close`. |

### Privacy Preferences Store (Out Of Scope For Spec 041 Scope 6)

Scope 6 wires the consent gate but does NOT introduce a persistent
privacy preferences store. The live core process defaults to
fail-closed (`engagement_telemetry_off`) at process startup. A
follow-up scope owned by a future privacy-settings store activation
will wire user preferences to `SetEngagementConsentReader` at
bootstrap. Until that follow-up scope lands, engagement telemetry is
emitted ONLY by tests that explicitly install a consent reader; the
live system is silent.

## QF Personal Context Read API (Spec 041 Scope 7)

Scope 7 ships the personal-context read API host — a read-only,
consent-token-gated endpoint that lets QF (the only authorized
consumer) pull recent personal-context items for a single
`entity_ref`, strictly bounded by sensitivity ceilings, with a
short-lived consent token that may be redeemed at most five times.

### Route

`GET /api/private/qf/v1/personal-context`

Mounted inside the bearer-auth-gated route group; the route MUST NOT
be reachable without a valid `Authorization: Bearer <token>` header.

Query parameters (all required unless noted):

| Param | Required | Description |
|-------|----------|-------------|
| `entity_ref` | yes | Opaque entity reference (e.g., `user:<id>`). MUST match the token's bound `entity_ref` exactly. |
| `max_sensitivity` | yes | Requested ceiling. MUST be one of `low`, `medium`, `high` (lower-case). |
| `consent_token` | yes | Token id issued by `PersonalContextConsentTokenStore.Issue`. Each call to the route consumes one read against the token, regardless of outcome (rate-limited, capability-disabled, and rejected attempts all count). |
| `requester_id` | no | Defaults to the bearer-auth `auth.UserIDFromContext` value when absent. |

### Response shape (200)

```json
{
  "items": [
    {
      "artifact_id": "...",
      "kind": "note",
      "sensitivity_tier": "low",
      "summary": "...",
      "source_ref": "https://...",
      "captured_at": "2026-05-22T20:19:28Z"
    }
  ],
  "redaction_count": 2,
  "effective_tier": "low",
  "consent_ceiling": "high",
  "user_ceiling": "low",
  "non_influence_warning": "Personal context returned for QF calibration only. Smackerel does not, and MUST NOT, influence QF mandate, watch list, trade approval, or execution decisions.",
  "token_reads_used": 1,
  "token_reads_remaining": 4
}
```

The `non_influence_warning` string is byte-exact and is asserted by
the unit, integration, and e2e tests. Any drift is treated as a
governance break under Principle 10 (QF Companion Boundary) and is
caught by the regression suite.

### Failure matrix

| HTTP status | Error code | Trigger |
|-------------|------------|---------|
| 400 | `missing_consent_token` | `consent_token` query parameter absent or empty |
| 400 | `invalid_max_sensitivity` | `max_sensitivity` not in `{low, medium, high}` |
| 400 | `invalid_entity_ref` | `entity_ref` empty |
| 403 | `PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION` | `entity_ref` or requested tier exceeds what the token was issued for |
| 403 | `PERSONAL_CONTEXT_CONSENT_EXPIRED` | Token past `expires_at` |
| 403 | `PERSONAL_CONTEXT_TOKEN_REVOKED` | Token revoked |
| 403 | `PERSONAL_CONTEXT_TOKEN_NOT_FOUND` | Token id unknown |
| 429 | `PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED` | 6th read against a token issued with the documented 5-read cap (response includes `Retry-After: 900`) |
| 503 | `PERSONAL_CONTEXT_DISABLED_BY_CAPABILITY` | Persisted capability declares `personal_context_pull_supported: false` |

The capability gate fires BEFORE the consent counter is incremented;
the 503 path leaves `reads_used` unchanged. The rate-limit gate
fires AFTER the atomic counter increment, so the disallowed read is
counted and persisted (this is what the SCN-SM-041-027 contract
requires: every attempt MUST be counted).

### Persistent state

Migration `037_qf_personal_context_consent_tokens.sql` creates the
`qf_personal_context_consent_tokens` table:

| Column | Type | Notes |
|--------|------|-------|
| `token_id` | TEXT PRIMARY KEY | Prefixed with `pct_` followed by 64 hex chars |
| `entity_ref` | TEXT NOT NULL | The opaque entity reference the token is bound to |
| `max_sensitivity_tier` | TEXT NOT NULL | CHECK constraint: must be `low`, `medium`, or `high` |
| `requester_id` | TEXT NOT NULL | The issuer/requester id recorded with the token |
| `issued_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `expires_at` | TIMESTAMPTZ NOT NULL | CHECK constraint: must be > `issued_at` |
| `reads_used` | INTEGER NOT NULL DEFAULT 0 | CHECK constraint: must be >= 0 |
| `revoked_at` | TIMESTAMPTZ | Set by `Revoke`; `COALESCE`-protected so revocation is idempotent |

An index on `(expires_at, revoked_at)` supports a future sweep job;
no sweep is wired in Scope 7.

### Token issuance contract

`PersonalContextConsentTokenStore.Issue` refuses any TTL greater than
`PersonalContextConsentMaxTTL` (15 minutes) and refuses any tier
outside `{low, medium, high}`. The maximum reads-per-token is
`PersonalContextConsentMaxReads` (5). Both ceilings are adversarially
asserted by unit tests in
`internal/connector/qfdecisions/personal_context_consent_test.go` so
that a future edit that relaxes them without spec approval fails
first.

### Atomic read counter

`AtomicConsumeRead` performs a single UPDATE that increments
`reads_used` and returns the new row, then validates in this order:

1. revoked → `PERSONAL_CONTEXT_TOKEN_REVOKED` (403)
2. expired → `PERSONAL_CONTEXT_CONSENT_EXPIRED` (403)
3. scope mismatch (wrong `entity_ref`) → `PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION` (403)
4. ceiling raised above token bound → `PERSONAL_CONTEXT_CONSENT_SCOPE_VIOLATION` (403)
5. rate-limit (`reads_used > 5` after increment) → `PERSONAL_CONTEXT_RATE_LIMIT_EXCEEDED` (429)

Because the increment is the first step, EVERY attempt — including
rejected ones — is counted; the integration test
`TestQFPersonalContextRead_AtomicReadCapEnforced_WhenSixthAttemptIsMade`
asserts the persisted counter is exactly 6 after 5 happy reads + 1
rate-limited read.

### Sensitivity filter

Items are pulled by
`PersonalContextSensitivityQuerier.QueryByEntityRef`, which reads
from the `artifacts` table where `metadata->>'entity_ref'` matches
and `metadata->>'sensitivity_tier'` is non-null. The handler then
applies the most-restrictive of (consent ceiling, user ceiling):

- Items with tier strictly above the consent ceiling are NOT
  counted as redactions — they were never in scope.
- Items with tier strictly above the user ceiling but at or below
  the consent ceiling ARE counted in `redaction_count`.
- Items at or below the user ceiling are returned in `items`.

The conservative wired-default user ceiling is `low`
(`personalContextLowCeilingProvider` in `cmd/core/wiring.go`); a
follow-up scope owned by a future privacy-settings store activation
will replace it with a per-user lookup. Misconfigured ceilings
collapse to `low` (the most-restrictive sentinel) via
`PersonalContextTierMinimum`, which is adversarially tested so a
future regression cannot silently widen access.

### Metric

`smackerel_qf_personal_context_reads_total{outcome, sensitivity_tier}` —
one counter, two labels. Outcome vocabulary:

| Outcome | When it fires |
|---------|---------------|
| `ok` | Read succeeded, no redactions |
| `degraded` | Read succeeded with `redaction_count > 0` |
| `rejected` | 4xx outcome (missing token, invalid tier, scope violation, expired, revoked, not found) |
| `rate_limited` | 429 outcome (rate-limit exceeded) |
| `capability_disabled` | 503 outcome (capability declares `personal_context_pull_supported: false`) |
| `unknown` | Fallback for guarded vocabulary drift (never reached in normal operation) |

### Audit envelope

Every attempt — successful, redacted, rejected, rate-limited, or
capability-disabled — emits a Cross-Product Audit Envelope v1 via
`EmitConnectorAuditEnvelope` with `action =
"personal_context_read"`. The envelope is emitted unconditionally
inside `defer`, so even error paths produce one audit record per
read attempt. Audit outcome vocabulary:

| Outcome constant | String |
|------------------|--------|
| `AuditOutcomeOK` | `ok` |
| `AuditOutcomeDegraded` | `degraded` |
| `AuditOutcomeRejected` | `rejected` |
| `AuditOutcomeRateLimited` | `rate_limited` |
| `AuditOutcomeCapabilityDisabled` | `capability_disabled` |

### Operator guidance

- Personal context reads are bounded by the token's `max_sensitivity_tier`
  AND the persisted user privacy ceiling. To raise the user
  ceiling above the conservative `low` default, a future spec
  must add a per-user privacy preferences store (out of scope
  for spec 041 Scope 7).
- Tokens are sweepable via `Revoke(tokenID)`; revocation is
  idempotent. No background sweep is wired in Scope 7; tokens
  past `expires_at` are simply rejected at read time and remain
  in the table until manual cleanup or a future sweep job.
- Capability flips are read live on every attempt (no in-process
  cache). To disable the read path, flip
  `personal_context_pull_supported` to `false` in the persisted
  capability response and the next call returns 503 without
  consuming a token read.

### Non-negotiable boundaries (Principle 10)

- Smackerel MUST NOT use this route to initiate trade approval,
  mandate change, execution, or financial advice. The
  `non_influence_warning` string is byte-exact and is the binding
  reminder on every response.
- The route MUST NOT bypass the bearer-auth gate; the e2e test
  `TestQFPersonalContextRead_LiveHTTP_RequiresBearerAuth` asserts
  this by calling without the header and requiring 401.
- The route MUST NOT widen sensitivity beyond the token's
  `max_sensitivity_tier`; the ordering check is adversarially
  tested with `PersonalContextTierLessOrEqual` so the reversal
  cannot silently grant high-tier access.

## QF Companion Signed Callback Protocol (Spec 041 Scope 8)

Scope 8 ships the signed-callback protocol the connector uses to POST
diagnostic, no-action callbacks back to QF (`POST /api/private/smackerel/v1/callback`).
Every callback envelope is signed with HMAC-SHA256 (lower-case hex) using a
key drawn from an SST-managed JSON keystore. Pre-MVP, every QF response
returns `{"code":"CALLBACK_DEFERRED_TO_V1"}`; Smackerel parses that
rejection deterministically and treats it as `Status = rejected_v1_deferred`
with **zero** local action acceptance — Principle 10 forbids Smackerel from
ever executing a QF action.

### Canonical payload

The signed input is a pipe-delimited string composed in exactly this order:

```text
callback_id|trace_id|packet_id|action|nonce|expires_at|surface
```

Rules (`internal/connector/qfdecisions/callback.go::CallbackCanonicalPayload`):

- No whitespace in any component (no space, tab, CR, LF).
- No trailing pipe.
- No pipe character inside any component.
- `expires_at` is RFC3339 UTC.
- `action` MUST be one of `{noop, open}` pre-MVP. Any other value (in
  particular `approval`, `execution`, `mandate_change`, etc.) is rejected
  locally with reason `MALFORMED_CANONICAL_PAYLOAD` AND triggers the
  pre-existing `EnforceQFActionBoundary` kick (Scope 5 defense-in-depth).
- `surface` MUST be one of `{telegram, web}`.

Any violation aborts before the HMAC step and emits a signature-failure
metric (see below). The signer never reaches the network on a malformed
envelope.

### Keystore (SST env var)

```bash
QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON='[
  {"key_id":"k-2026-05-01","secret":"…32+ bytes…","not_before":"2026-05-01T00:00:00Z"},
  {"key_id":"k-2026-05-15","secret":"…32+ bytes…","not_before":"2026-05-15T00:00:00Z"}
]'
```

Schema (`internal/connector/qfdecisions/callback_keystore.go`):

- The env var holds a JSON array of `{key_id, secret, not_before}` entries.
- `key_id`, `secret`, and `not_before` are all required per entry.
- `key_id` values MUST be unique across the array.
- `not_before` is RFC3339 UTC.
- `SelectActiveKey(now)` chooses the newest key whose `not_before` is not
  after `now` (ties broken by `key_id` ordering). If every key has a
  future `not_before`, the keystore returns `ErrNoActiveCallbackKey` —
  the connector treats this as a **fatal startup error** at
  `Connector.Connect` time and emits a `callback_keystore_no_active_key`
  audit envelope.

Empty / unset env var: the connector still starts; the signer is simply
not wired and the callback path is unavailable in that environment. This
is the documented pre-MVP default.

Malformed env var (invalid JSON, missing fields, duplicate `key_id`, etc.):
fatal startup error with audit reason `callback_keystore_invalid:<err>`.

### Key rotation playbook

1. Generate the new key + `not_before` at least 24h in the future.
2. Add the new entry to the `QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON`
   array (keep the old entry in place during the overlap window).
3. Apply the deploy adapter so the new env var reaches the running
   container; restart the core process. The log line
   `qf-decisions: callback signing keystore loaded keys=N key_ids=[…]`
   confirms the new key is visible.
4. At `now ≥ new_key.not_before`, `SelectActiveKey` switches
   automatically. The old key remains in the keystore and stays
   available for in-flight envelopes that QF may still validate
   against the older `key_id`.
5. Withdraw the old key by removing it from the array and applying
   another deploy. The 24h overlap window is the recommended minimum;
   tightening it requires explicit operator review.

### Signature failure vocabulary

| Reason | Trigger | Behaviour |
|--------|---------|-----------|
| `NO_ACTIVE_KEY` | Keystore is empty OR every key has `not_before > now` at signing time | Abort locally, increment `smackerel_qf_callback_signature_failures_total{reason="NO_ACTIVE_KEY"}`, emit `callback_attempt` audit envelope with `outcome=rejected reason=NO_ACTIVE_KEY`, **no network send** |
| `MALFORMED_CANONICAL_PAYLOAD` | Any canonical-payload rule violation (pipe-in-field, whitespace, unknown action, unknown surface, non-RFC3339 `expires_at`, empty required field) | Same as above with `reason="MALFORMED_CANONICAL_PAYLOAD"` |
| `EXPIRES_AT_OUTSIDE_TOLERANCE` | `expires_at` is more than 60 seconds in the past relative to the signer's wall clock | Same as above with `reason="EXPIRES_AT_OUTSIDE_TOLERANCE"` (clock-skew check is enforced **locally** before signing — Smackerel does not retry the QF round-trip to re-test) |

The Status label emitted on every signature-failure attempt is
`rejected_local`. On every `rejected_local` attempt, the connector
increments BOTH `smackerel_qf_callback_signature_failures_total{reason}`
AND `smackerel_qf_callback_attempts_total{action,status=rejected_local}`.

### Status label vocabulary

| Status | Meaning |
|--------|---------|
| `ok` | QF returned 2xx. **Crucial**: a 2xx response is _not_ a local action acceptance — Smackerel still does nothing with the user-affecting side of the callback (PP10). |
| `rejected_v1_deferred` | QF returned a structured rejection with `code=CALLBACK_DEFERRED_TO_V1`. This is the **expected** pre-MVP status; the connector returns a Go-nil error and surfaces the parsed `RejectionCode` to the caller for observability. |
| `rejected_local` | The signer aborted before any network send. See the failure vocabulary above. |
| `error` | Transport error (network failure, non-2xx unparseable response, etc.). The caller receives a non-nil Go error; **no retry** is attempted — callback is a diagnostic, not a delivery contract. |

### Capability gate

`QFBridgeCapability.callback_signing_supported` is reserved on the wire and
read by the connector during the capability handshake. Pre-MVP it is
documentary only: the connector logs its value
(`qf_capability_callback_signing_supported=…`) at startup but does NOT
condition the signer wiring on it, so an operator-configured keystore
remains the source of truth for the Smackerel side. The flag flips to a
runtime gate when QF lands the v1 callback contract.

### PP10 no-action-accepted guarantee

The callback path:

- Signs and POSTs a diagnostic envelope only.
- NEVER mutates Smackerel state in response to QF's reply (success,
  rejection, or error).
- NEVER retries — neither on QF rejection (e.g., `CALLBACK_DEFERRED_TO_V1`)
  nor on transport failure.
- ALWAYS emits a `callback_attempt` cross-product audit envelope that
  carries `trace_id`, `packet_id`, the canonical `action`, the `surface`,
  the final `status`, and the `reason` (free-form on local failures,
  `CALLBACK_DEFERRED_TO_V1` on the parsed pre-MVP rejection).

These guarantees are unit-tested
(`internal/connector/qfdecisions/callback_test.go`,
`internal/telegram/render/qf_packet_message_test.go`),
integration-tested against the live disposable test stack
(`tests/integration/qf_callback_signing_test.go`), and e2e-tested
through the connector lifecycle against the live core API
(`tests/e2e/qf_callback_signing_test.go`).

## QF Companion Watch Signal Proposal Endpoint (Spec 041 Scope 9, Pre-MVP Design Only)

Scope 9 ships the diagnostic watch-signal proposal client the connector
uses to POST signed pre-MVP watch-signal proposals to the QF Bridge
(`POST /api/private/smackerel/v1/watch-signal-proposals`). Pre-MVP, QF
rejects every proposal with `WATCH_PROPOSALS_DEFERRED_TO_V1` over HTTP
503; the connector parses the rejection without retry. The path is
connector-internal: it has no user-visible affordance on web, digest,
or Telegram, and Smackerel never mutates local watch state in response
to a proposal POST.

### Canonical payload

The signed input is a pipe-delimited string composed in exactly this
order:

```text
trace_id|source|entity_ref|reason|expires_at
```

Rules (`internal/connector/qfdecisions/watch_proposal.go`):

- No whitespace in any component.
- No trailing pipe.
- No pipe character inside any component.
- `expires_at` is RFC3339 UTC.
- `source` MUST be the literal `smackerel_propose` pre-MVP. Any other
  value is rejected locally with reason `MALFORMED_CANONICAL_PAYLOAD`.
- `trace_id` is a UUIDv7 generated client-side per proposal.

### Body shape

The signed envelope POSTed to QF is exactly:

```json
{
  "trace_id": "01970000-0000-7000-8000-000000000031",
  "source": "smackerel_propose",
  "entity_ref": "qf:security:NVDA",
  "reason": "attention_signal_over_threshold",
  "expires_at": "2026-05-23T12:05:00Z",
  "signature": "<64-char lower-case hex HMAC-SHA256>",
  "key_id": "<active Scope 8 key_id>"
}
```

No extra fields. No missing fields.

### Scope 8 signer reuse contract

Scope 9 holds an interface reference to the Scope 8 signer
(`internal/connector/qfdecisions/callback.go`) and the Scope 8 in-process
keystore (`internal/connector/qfdecisions/callback_keystore.go`). Scope
9 does NOT reimplement HMAC-SHA256, key selection, or `key_id` envelope
inclusion. The signature-failure reason vocabulary is aliased to the
Scope 8 vocabulary so any future Scope 8 extension propagates without
a divergent code path:

```go
WatchProposalSignatureFailureNoActiveKey               = CallbackSignatureFailureNoActiveKey
WatchProposalSignatureFailureMalformedCanonicalPayload = CallbackSignatureFailureMalformedCanonicalPayload
WatchProposalSignatureFailureExpiresAtOutsideTolerance = CallbackSignatureFailureExpiresAtOutsideTolerance
```

### Capability gate

`QFBridgeCapability.watch_proposal_supported` is reserved on the wire
and read by the connector during the capability handshake. The pre-MVP
contract is `false`. The connector logs its value
(`qf_capability_watch_proposal_supported=…`) at watch-proposal client
construction time but does NOT treat a `true` value as a runtime
override toggle: the connector continues to expect the
`WATCH_PROPOSALS_DEFERRED_TO_V1` rejection envelope regardless. The
flag flips to a runtime gate when QF lands the v1 watch-proposal
acceptance contract.

### Pre-MVP rejection parsing

Pre-MVP QF responds HTTP 503 with body:

```json
{"code":"WATCH_PROPOSALS_DEFERRED_TO_V1","message":"pre-MVP: bridge does not accept watch proposals"}
```

The connector:

- Parses the response without retrying.
- Returns `Status = rejected_v1_deferred` with the parsed
  `RejectionCode = "WATCH_PROPOSALS_DEFERRED_TO_V1"`.
- Increments
  `smackerel_qf_watch_proposal_attempts_total{status="rejected_v1_deferred"}`.
- Emits a Cross-Product Audit Envelope v1 record with
  `action=watch_proposal`, `outcome=rejected`,
  `reason=WATCH_PROPOSALS_DEFERRED_TO_V1`,
  `target_context_type=<entity_ref>`.

### Status label vocabulary

| Status | Meaning |
|--------|---------|
| `rejected_v1_deferred` | QF returned the structured pre-MVP rejection `WATCH_PROPOSALS_DEFERRED_TO_V1`. This is the expected pre-MVP status. |
| `rejected_local` | The signer aborted before any network send (e.g., `NO_ACTIVE_KEY`, `MALFORMED_CANONICAL_PAYLOAD`, `EXPIRES_AT_OUTSIDE_TOLERANCE`). |
| `degraded` | Transport failure (timeout, network error, malformed QF body) prevented the connector from observing a definitive QF response. |

Pre-MVP `accepted` is NEVER emitted because QF rejects every proposal.

### No-watch-state-mutation guarantee

The watch-proposal client:

- NEVER persists the proposal as accepted in any Smackerel database
  table.
- NEVER mutates QF watch state (no Smackerel-side watch creation, watch
  evaluation, trade approval, mandate change, EmergencyStop, or
  execution).
- NEVER publishes on any NATS subject in response to a proposal POST.
- NEVER retries `WATCH_PROPOSALS_DEFERRED_TO_V1` rejections.

This invariant is enforced at the Change Boundary level
(every excluded file family in `scopes.md` Scope 9 Change Boundary
section names the watch/proposal/qf-state surfaces explicitly) and at
the live-stack level: the integration test
`TestQFWatchProposalPreMVPRejectionParsedAndNoLocalWatchStateMutatedAcrossLiveStack`
snapshots `pg_stat_user_tables.n_tup_ins + n_tup_upd + n_tup_del` for
every `watch_*` / `proposal_*` / `qf_*` relation BEFORE the Propose
call and asserts zero delta AFTER.

### No-user-visible-affordance guarantee

The watch-proposal client is connector-internal. It is callable only
from the connector internal diagnostic path (constructed at
`(*Connector).Connect` time, shut down at `(*Connector).Close` time)
and from the Scope 9 integration test. It is NEVER wired into any
user-visible Smackerel surface (web, daily digest, Telegram). The
unit test
`TestWatchProposalIsNotCallableFromUserVisibleSurfacesAndOnlyFromConnectorDiagnosticPath`
asserts this structurally via an import-graph check. The e2e
adversarial probe
`TestQFWatchProposalPreMVPDeferralRejectionThroughLiveSurfaceWithNoLocalMutationOrUserSurface`
hits `http://smackerel-core:8080/api/private/smackerel/v1/watch-signal-proposals`
and asserts HTTP 4xx (Smackerel core does NOT host the QF endpoint;
the path is only used as a target for outbound POSTs).

### PP10 cross-product boundary

The watch-proposal path is a Smackerel → QF diagnostic POST. It NEVER
initiates trade approval, mandate change, execution, or financial
advice. Smackerel observes attention signals; QF (post-MVP) decides
whether to act on them. Pre-MVP, the rejection envelope makes the
boundary explicit on every attempt.

### Test references

These guarantees are unit-tested
(`internal/connector/qfdecisions/watch_proposal_test.go`),
integration-tested against the live disposable test stack
(`tests/integration/qf_watch_proposal_test.go`), and e2e-tested
through the connector lifecycle against the live core API
(`tests/e2e/qf_watch_proposal_test.go`).

## Assistant Capability (Spec 061)

The Conversational Assistant capability is a transport-agnostic layer
that turns free-text user messages into either an answered turn
(retrieval / weather / notifications / open-knowledge) or a
capture-as-fallback. It is built on the spec 037 LLM Scenario Agent
substrate and presents a single `internal/assistant/facade.go` boundary
to every transport. The capability layer never imports transport
packages; transport adapters implement
`internal/assistant/contracts.TransportAdapter` and own message I/O.
Telegram (see [`docs/Connector_Development.md`](Connector_Development.md))
is one such adapter; the HTTP transport from
[spec 069](../specs/069-assistant-http-transport/) registered under
`Transport="web"` exposes `POST /api/assistant/turn` and is the canonical
programmatic surface used by E2E tests and future frontends (web chat,
Android in-app, WhatsApp Business webhook, devtools).

Related specs layered on top of 061:

| Spec | Concern |
|------|---------|
| [064](../specs/064-open-ended-knowledge-agent/) | Open-ended knowledge agent — terminal scenario that absorbs any NL turn before capture-as-fallback. |
| [065](../specs/065-generic-micro-tools/) | Cross-scenario micro-tools (`location_normalize`, `unit_convert`, `entity_resolve`, `calculator`). |
| [066](../specs/066-legacy-keyword-surface-retirement/) | Retirement of slash commands, `domain_intent.go`, annotation keyword map; configurable NL alias window. |
| [067](../specs/067-intent-driven-policy-enforcement/) | CI guards: scenario-prompt cap, mandatory `principleAlignment`, broadened NO-DEFAULTS, forbidden-keyword guard, compiler-bypass detection. |
| [068](../specs/068-structured-intent-compiler/) | NL → `CompiledIntent` → route runtime contract; runs inside the facade so every transport exercises the same path. |
| [069](../specs/069-assistant-http-transport/) | `POST /api/assistant/turn` HTTP transport adapter. |

### Observability And Alerts

Ten Prometheus metric series cover the capability. Eight live in
[`internal/assistant/metrics/metrics.go`](../internal/assistant/metrics/metrics.go);
the two gate-style counters live with the gates that emit them
(`internal/assistant/metrics/source_assembly.go` and
`internal/assistant/provenance/gate.go`). Every label vocabulary is
closed; `internal/assistant/metrics/labels_test.go` proves no emission
site uses a value outside the published set.

| Metric | Type | Labels | Increment / observation |
|--------|------|--------|--------------------------|
| `smackerel_assistant_facade_turns_total` | CounterVec | `transport`, `outcome` ∈ {`answered`,`captured`,`proposed`,`confirmed`,`discarded`,`error`} | One facade turn observed per transport-outcome pair. |
| `smackerel_assistant_facade_latency_seconds` | HistogramVec | `transport`, `outcome` | Facade enter → response emit latency. Buckets: 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30 s. |
| `smackerel_assistant_router_band_total` | CounterVec | `band` ∈ {`high`,`borderline`,`low`}, `transport` | Three-band post-processor decision per turn. |
| `smackerel_assistant_skill_invocations_total` | CounterVec | `scenario_id`, `outcome` (8 values mirroring spec 037 `InvocationResult.Outcome`), `transport` | One per scenario executor return. `scenario_id` is bounded by the manifest (~10 entries in v1). |
| `smackerel_assistant_capture_fallback_total` | CounterVec | `cause` ∈ {`low_confidence`,`borderline_timeout`,`confirm_discarded`,`confirm_timeout`,`error_offered_capture`,`unresolvable_reference`,`unrouted`,`open_knowledge_no_ground`,`clarify_abandoned`,`compiler_error`}, `transport` | One per capture-as-fallback event, by trigger cause. The last four causes are the spec 074 capture-as-fallback-policy additions (see `internal/assistant/metrics/metrics.go` `AllCaptureFallbackCauses` for the closed vocabulary source-of-truth). |
| `smackerel_assistant_confirm_card_outcomes_total` | CounterVec | `scenario_id`, `outcome` ∈ {`confirmed`,`discarded_user`,`discarded_timeout`}, `transport` | One per terminal confirm-card outcome. |
| `smackerel_assistant_disambiguation_outcomes_total` | CounterVec | `outcome` ∈ {`resolved_user`,`resolved_timeout_capture`,`resolved_non_matching_reply_capture`}, `transport` | One per resolved disambiguation prompt. |
| `smackerel_assistant_active_threads` | GaugeVec | `transport` | Current count of active `assistant_conversations` rows per transport. Refreshed by the periodic context-store ticker. |
| `smackerel_assistant_source_assembly_drops_total` | CounterVec | `scenario_id`, `cause` ∈ {`missing_artifact`,`lookup_error`} | One per cited artifact ID dropped by the source-assembly invariant (graph drift). |
| `smackerel_assistant_provenance_violations_total` | CounterVec | `scenario_id`, `cause` ∈ {`missing_artifact`,`lookup_error`,`fabricated_source`,`dropped_for_quota`} | One per response rewritten to the canonical refusal because a `requires_provenance` scenario returned a body with empty `Sources`. Expected steady-state = 0. |

Operator dashboard: [`deploy/observability/grafana/dashboards/assistant.json`](../deploy/observability/grafana/dashboards/assistant.json)
ships seven panels — Per-transport turn volume + band mix, Per-scenario
success vs failure, Capture-as-fallback rate (by cause), Provenance
violations (alert at >0), Active conversation threads per transport,
Confirm-card outcome distribution, and Source-assembly drops (graph
drift). The dashboard is validated by
`tests/observability/assistant_dashboard_test.go`.

### Service Budgets

The capability layer carries two hard latency budgets (design §8.1)
and two correctness budgets:

| Surface | Budget | Measurement | Last shipped evidence |
|---------|--------|-------------|-----------------------|
| Facade overhead (router + post-processor + context I/O minus skill call) | p95 < 5 ms | `tests/stress/assistant_facade_p95_test.go` (1500 turns × 32 workers; build tag `stress`) | p95 = 327.908 µs (≪ 5 ms); see `specs/061-conversational-assistant/report.md` Round-6 §SCOPE-04 stress evidence. |
| Retrieval skill end-to-end | p95 < 5 s | `tests/stress/assistant_retrieval_p95_test.go` (800 turns × 32 workers, 1–40 ms synthetic upstream tail; build tag `stress`) | p95 = 40.04 ms (≪ 5 s); see Round-17 §SCOPE-06 retrieval-stress evidence. |
| `smackerel_assistant_source_assembly_drops_total` | < 5% of cited IDs per scenario | Sampled from `/metrics` | Steady-state ≈ 0 unless the graph is being repaired or migrated. |
| `smackerel_assistant_provenance_violations_total` | 0 in steady state | Sampled from `/metrics` | Any non-zero rate means the LLM is producing bodies that drop every cited source. |

### Configuration SST

For the operator-facing per-transport key inventory (HTTP, WhatsApp,
assistant Telegram, legacy Telegram bot), see
[`docs/Transport_Configuration.md`](Transport_Configuration.md). That
doc is enforced to stay in lock-step with
`internal/assistant/transportconfig` by `TestRegistry_DocSync`
(SCN-062-A06) under `./smackerel.sh test unit`.

All assistant configuration originates from
[`config/smackerel.yaml`](../config/smackerel.yaml) under the top-level
`assistant:` block (around lines 640–724). It is validated by
[`internal/config/assistant.go`](../internal/config/assistant.go)
(`loadAssistantConfig` + `validateAssistantConfig`) at startup and
emitted into `config/generated/<env>.env` by
[`scripts/commands/config.sh`](../scripts/commands/config.sh). Every key
is REQUIRED; no defaults — startup fails loudly per
[`smackerel-no-defaults`](../.github/instructions/smackerel-no-defaults.instructions.md).
The four validation rules are:

1. Every key resolves to a non-empty value.
2. `assistant.borderline_floor` MUST be strictly greater than
   `agent.routing.confidence_floor` (preserves the borderline band).
3. `assistant.enabled: true` requires at least one
   `assistant.transports.<name>.enabled: true`.
4. `assistant.context.state_key: "user"` (non-recommended) emits a
   startup WARN log; `"transport_user"` is the recommended primary-key
   shape.

### Intent Compiler SST (Spec 068)

The structured intent compiler ([spec 068](../specs/068-structured-intent-compiler/))
lives under `assistant.intent_compiler.*` in
[`config/smackerel.yaml`](../config/smackerel.yaml) and is validated by
[`internal/config/assistant_intent_compiler.go`](../internal/config/assistant_intent_compiler.go)
(`loadAssistantIntentCompilerEnv`). Every key is REQUIRED; the loader
emits one aggregate `F068-SST-MISSING` fail-loud error at startup if any
key is missing, empty, or unparsable — there is no fallback model,
prompt, timeout, or confidence floor.

| Key | Type | Purpose |
|-----|------|---------|
| `assistant.intent_compiler.enabled` | strict bool | Master switch for facade Step 3.5 compilation. `false` keeps the router on raw text + slash shortcuts. |
| `assistant.intent_compiler.model_role` | string | ML-bridge model role the compiler binds to (e.g. `assistant_intent_compiler`). |
| `assistant.intent_compiler.prompt_contract_version` | string | Prompt contract version the ML sidecar must honor (e.g. `intent-compiler-v1`). |
| `assistant.intent_compiler.schema_version` | string | `CompiledIntent` schema version the Go side accepts (e.g. `v1`). |
| `assistant.intent_compiler.timeout_ms` | int ≥ 1 | Per-compile request deadline (ms). |
| `assistant.intent_compiler.confidence_floor` | float [0,1] | Scenario-hint confidence floor; below this the router falls back to similarity ranking. |
| `assistant.intent_compiler.max_context_turns` | int ≥ 0 | Bounded conversation window passed to the compiler. |
| `assistant.intent_compiler.max_output_bytes` | int ≥ 1 | Cap on sidecar response body — defense against runaway LLM output. |
| `assistant.intent_compiler.retry_budget` | int ≥ 0 | Schema-validation retries before declaring `schema_invalid`. |

Operator notes:

- **Fail-loud behavior:** any of the keys above missing or empty in the
  resolved env file aborts `smackerel-core` startup with a single
  aggregated error naming every missing key. There is no soft-fallback;
  do not introduce one via shell defaults (per
  [`smackerel-no-defaults`](../.github/instructions/smackerel-no-defaults.instructions.md)).
- **Operational-command bypass (closed set):** only `/help`, `/status`,
  `/reset`, `/digest`, `/recent`, and `/done` skip compilation. The list
  is owned by
  [`internal/assistant/intent/bypass.go`](../internal/assistant/intent/bypass.go)
  (`OperationalCommands`); every bypass turn is stamped with the trace
  label `operational_command_bypass`.
- **Disabling the compiler:** set
  `assistant.intent_compiler.enabled: false` and regenerate config — the
  facade reverts to slash-shortcut + raw-text routing. All other
  `intent_compiler.*` keys still MUST resolve (NO-DEFAULTS); leave them
  populated.
- **Compiler failure handling:** a compiler error emits the canonical
  capture-as-fallback response without invoking the router (Hard
  Constraint 1). `clarify` and `capture_only` action classes
  short-circuit at facade Steps 3.55 and 3.5; `write` /
  `external_write` side-effect classes are gated at Step 3.6 by
  `intent.RequiresConfirmation` and increment
  `smackerel_assistant_side_effect_blocked_total`.

The full key inventory is enumerated in
[`specs/061-conversational-assistant/scopes.md`](../specs/061-conversational-assistant/scopes.md)
SCOPE-01.

### Confirm-Card Lifecycle

The tool-side notification confirm path is keyed by a ULID
`confirm_ref` and persisted in PostgreSQL so a process restart does not
strand pending confirms:

- Storage: [`internal/db/migrations/043_assistant_confirm_pending.sql`](../internal/db/migrations/043_assistant_confirm_pending.sql)
  defines `assistant_confirm_pending(confirm_ref TEXT PK, payload TEXT,
  expires_at TIMESTAMPTZ)` with an index on `expires_at`.
- Reader / writer: [`internal/agent/tools/notification/pg_confirm_store.go`](../internal/agent/tools/notification/pg_confirm_store.go)
  enforces TTL on every SELECT (`AND expires_at > NOW()`); `Put` requires
  a positive TTL and writes `expires_at = NOW() + ttl`.
- TTL value (SST):
  `assistant.skills.notifications.confirm_timeout` (default
  literal in `config/smackerel.yaml` is `"5m"`). The disambiguation
  prompt TTL is `assistant.disambiguate_timeout` (literal `"2m"`); the
  offer-to-capture-on-error TTL is `assistant.error.capture_timeout`
  (literal `"10s"`). All three are fail-loud REQUIRED values.
- Operator action: confirm via the Telegram inline button → tool
  executes; on TTL expiry the row is filtered out at SELECT time and a
  future sweep job MAY DELETE expired rows.

### Common Failure Modes

| Symptom (metric query) | Likely cause | First-line diagnostic |
|------------------------|--------------|-----------------------|
| `rate(smackerel_assistant_provenance_violations_total{cause="fabricated_source"}[5m]) > 0` | LLM contract drift — the model returned a non-empty body without any cited sources. | Check the prompt contract under `config/prompt_contracts/` for the affected scenario, and verify the `requires_provenance` flag in `config/assistant/skills_manifest.yaml`. |
| `rate(smackerel_assistant_source_assembly_drops_total{cause="missing_artifact"}[5m]) > 0` | Graph store consistency — the LLM cited an artifact that no longer exists (deletion, prune, re-ingest). | Inspect `internal/db/migrations/` for recent artifact-lifecycle migrations; verify the affected artifact rows are still present. |
| `rate(smackerel_assistant_capture_fallback_total{cause="low_confidence"}[10m])` trending up | Router classification degradation — borderline / low confidence band rate rising. | Run the eval harness (see [`docs/Testing.md`](Testing.md) → "Assistant Evaluation Harness") against the current corpus; investigate intent corpus drift. |
| `smackerel_assistant_active_threads{transport=*} > 0` and not decaying | Orphaned thread state — the periodic idle sweeper is not pruning. | Verify `assistant.context.idle_sweep_interval` and `assistant.context.idle_timeout` resolved values; check core logs for sweep-ticker errors. |
| `rate(smackerel_assistant_confirm_card_outcomes_total{outcome="discarded_timeout"}[1h])` > `confirmed` | User-experience drift — confirm TTL too short, or confirm UX hidden behind notifications. | Re-check `assistant.skills.notifications.confirm_timeout` SST value and Telegram delivery health. |

### Recovery Actions

- **Disable the capability** without code change: set
  `assistant.enabled: false` in `config/smackerel.yaml`, regenerate
  config (`./smackerel.sh config generate`), and restart. Telegram
  preserves its pre-spec-061 capture path (BS-001 regression-safe
  fallback).
- **Disable a single skill**: set `assistant.skills.<name>.enabled:
  false` (retrieval / weather / notifications) and regenerate. The
  router will route those intents to capture-as-fallback.
- **Rotate a misbehaving prompt contract**: edit the affected file
  under `config/prompt_contracts/`, `SIGHUP` the core to hot-reload
  (`agent.hot_reload: true`).
- **Recover stalled confirms**: a process restart is safe — pending
  confirms live in `assistant_confirm_pending`; the TTL filter at
  SELECT time excludes expired rows automatically.

### HTTP Transport (Spec 069)

[Spec 069](../specs/069-assistant-http-transport/) adds a second
concrete `contracts.TransportAdapter` registered under
`Transport="web"`, exposing one authenticated route:

```
POST /api/assistant/turn
```

The route lives under the per-user bearer policy (spec 044). Every
inbound request body is translated into an `AssistantMessage`, handed
to the same `Facade.Handle` path that backs Telegram, and the resulting
`AssistantResponse` is rendered back as the HTTP response body. The
facade, scenario executor, structured intent compiler (spec 068), and
router treat the web transport identically to Telegram; only the HTTP
adapter and the audit layer may inspect `AssistantMessage.Transport`.

Operator notes:

- This is the canonical programmatic entrypoint for E2E tests and for
  every future frontend (web chat, Android in-app, WhatsApp Business
  webhook bridge consumers, devtools). Telegram remains supported but is
  no longer the privileged path.
- The route requires a valid per-user PASETO bearer (per spec 044). No
  unauthenticated access is permitted.
- Capture-as-fallback, confirm-card lifecycle, disambiguation prompts,
  provenance enforcement, and the assistant metric series above all
  apply identically to `transport="web"` turns — emissions carry the
  `transport="web"` label rather than `"telegram"`.
- Scope-claim authorization (e.g. an `assistant.turn` scope) and the
  exact enable/disable SST key shape are owned by spec 069 — see the
  spec for the shipped contract before relying on either in operator
  procedures.

### OpenTelemetry Tracing (Spec 061 SCOPE-09a)

The assistant capability ships with an OpenTelemetry SDK substrate that
emits a single root span `assistant.adapter.translate` at every transport
adapter entry. The substrate is gated by three SST keys in
`config/smackerel.yaml` under `assistant.observability:`:

| SST key | Required? | Behaviour |
|---------|-----------|-----------|
| `assistant.observability.otel_enabled` | Yes (bool) | When `false` (production default), the SDK uses a no-op `TracerProvider` — spans are constructed but emit nothing and the network is never touched. When `true`, the OTLP/gRPC exporter is configured against `otel_endpoint`. |
| `assistant.observability.otel_endpoint` | Yes (string; may be empty when `otel_enabled=false`) | The OTLP/gRPC target (e.g. `smackerel-test-jaeger:4317`). MUST be non-empty when `otel_enabled=true`; the loader fails loud with `[F061-SST-MISSING]`-class error otherwise. |
| `assistant.observability.otel_service_name` | Yes (string non-empty) | Recorded as the OTel resource `service.name` attribute on every span. Defaults to `smackerel-core` in the literal yaml. |

When `otel_enabled=true`, `initAssistantTracing` runs a 5 s TCP probe
against the endpoint BEFORE the SDK is constructed; an unreachable
endpoint aborts startup rather than silently buffering spans against a
dead exporter.

To enable tracing in dev, bring up the in-tree `jaegertracing/all-in-one`
sidecar declared in `docker-compose.yml` under the `dev-otel` profile and
override the two non-default SST keys at generation time:

```bash
docker compose --profile dev-otel up -d jaeger    # starts smackerel-test-jaeger
# Then point your env override at the sidecar before regenerating config:
#   ASSISTANT_OBSERVABILITY_OTEL_ENABLED=true
#   ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT=smackerel-test-jaeger:4317
./smackerel.sh up
```

The Jaeger UI is reachable at <http://127.0.0.1:16686> and shows one
`assistant.adapter.translate` span per inbound Telegram update once the
adapter sees traffic. The remaining child spans (8 mandatory + 1
conditional per design §8.3.1.A) land in SCOPE-09b. The test compose
profile also runs the sidecar so SCOPE-09b's tree-shape assertion test
can exercise the live OTLP path; the default `./smackerel.sh up` (no
profile) leaves jaeger stopped and the no-op tracer in effect.

Canonical span attribute set (design §8.3.1.B): `transport`,
`user_id_hashed` (SHA-256 prefix; raw user IDs never appear in span
attributes), `assistant_turn_id`, `scenario_id` (empty at the
pre-routing adapter site), `correlation_id` (Telegram `update_id` for
the telegram transport). End attributes: `status` ∈ {`ok`,`error`,`noop`}
and `error_cause` (empty for `ok`).

#### Full Span Tree (Spec 061 SCOPE-09b)

SCOPE-09b instruments the 8 mandatory + 1 conditional child spans on
top of the SCOPE-09a substrate, completing the design §8.3.1.A facade
subtree. Each inbound Telegram turn that flows through `HandleUpdate`
emits this tree:

```
assistant.adapter.translate              (root; adapter.go::HandleUpdate)
  ├── assistant.facade.handle            (facade.go::Handle)
  │     ├── assistant.context.load       (facade.go::loadContextWithSpan)
  │     ├── assistant.router.classify    (facade.go::routeWithSpan)
  │     ├── assistant.router.band        (facade.go::borderlineWithSpan)
  │     ├── assistant.provenance.check   (facade.go::enforceProvenanceWithSpan; high-band only)
  │     ├── assistant.confirm.persist    (confirm/machine.go::persistProposalWithSpan; CONDITIONAL)
  │     ├── assistant.context.persist    (facade.go::appendTurnAndPersist)
  │     └── assistant.audit.write        (facade.go::writeAudit)
  └── assistant.adapter.render           (adapter.go::HandleUpdate; sibling of facade.handle)
```

Two design §8.3 spans are deliberately deferred to a future spec
because they belong to spec-037 (the agent runtime), not spec-061:
`agent.executor.run` and `agent.tool.<name>.invoke`. Until that
follow-up spec lands, `assistant.router.classify` is the deepest child
along the executor branch and the `agent_trace_id` cross-reference
attribute remains empty.

**Conditional span — `assistant.confirm.persist`.** This span only
emits when the confirm-card propose phase fires (i.e., a scenario with
`confirm_required=true` reaches the propose state in
`internal/assistant/confirm.Machine.Propose`). On every other turn its
absence is correct behavior, not a defect. In v1 the facade does not
yet dispatch through the confirm machine on its high-band path, so
non-confirm turns will not carry this span; once the confirm wiring
lands, the existing `persistProposalWithSpan` site emits without
further code changes.

**Operator commands for live trace inspection (dev/test):**

```bash
# Start the jaeger sidecar (test profile keeps it on by default;
# default `up` leaves the no-op tracer in effect).
docker compose --profile dev-otel up -d jaeger

# Override the two non-default SST keys, then regenerate + restart:
#   ASSISTANT_OBSERVABILITY_OTEL_ENABLED=true
#   ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT=smackerel-test-jaeger:4317
./smackerel.sh config generate
./smackerel.sh up

# Send any inbound Telegram message, then:
#   - Open <http://127.0.0.1:16686>
#   - Pick service `smackerel-core` and operation `assistant.adapter.translate`
#   - The tree above appears with parent/child links and per-span
#     status/error_cause attributes.

# Headless smoke (matches DoD #4 wording):
curl --max-time 5 "http://127.0.0.1:16686/api/traces?service=smackerel-core&limit=1"
```

**Adding a new span for a future skill.** Use the spec 061 SCOPE-09a
helpers verbatim — they enforce the canonical attribute set and the
end-status closed vocabulary:

```go
ctxSpan, span := tracer.StartSpan(ctx, "assistant.<area>.<verb>",
    transport, tracing.HashUserID(userID),
    assistantTurnID, scenarioID, correlationID)
defer tracing.EndSpan(span, "ok", "")
// ... do work using ctxSpan so descendants attach as children ...
```

Cross-references: design §8.3 (span tree), §8.3.1 (ratified
implementation decisions including the deferred spec-037 spans),
§8.3.1.B (canonical attribute set), §8.3.1.C (context.Context
propagation requirement), §8.3.2 (SCOPE-09 split rationale).

## Open-Knowledge Assistant Agent (Spec 064)

The open-knowledge agent extends the spec 061 assistant with a bounded
LLM planner ↔ tool ↔ observation loop that can answer open-domain
questions (factual, computational, current-events) while preserving
the capture-as-fallback invariant and the cite-back verifier
contract. See
[`specs/064-open-ended-knowledge-agent/spec.md`](../specs/064-open-ended-knowledge-agent/spec.md)
for the full outcome contract and refusal taxonomy.

> **Amended by spec 084 (Open-Knowledge Reasoning Loop).** The agent is now
> question-**agnostic**: its system prompt drives a decompose → gather-all-sides
> → reconcile → answer-the-actual-question contract (comparison / why /
> recommendation / multi-hop), replacing the spec-064 anti-multi-hop bias and
> the BUG-064-002 question-type enumeration. The `<CITATIONS>` cite-back
> contract, R1-R4, and the provenance gate are unchanged. Three operational
> deltas:
>
> - **`max_iterations` raised 4 → 6** (5 tool-calling turns + 1 forced-synthesis
>   turn) so a multi-side question can gather every side before answering, plus
>   a lightweight reflect-before-final nudge on the second-to-last iteration.
> - **`per_query_token_budget` raised 64000 → 128000.** `web_search` snippets are
>   re-added to context each iteration, so cumulative tokens grow ~quadratically;
>   128000 keeps a 6-iteration turn off a premature `cap_tokens` refusal. With
>   the zero-cost self-hosted `CostFn` this budget is a pure safety guardrail.
> - **Honest salvage.** When genuine synthesis does not happen and the platform
>   falls back to stitched snippets, the user-visible body is now framed as raw
>   findings ("I searched but couldn't directly answer your question. Here is the
>   most relevant information I found:") rather than presented as a confident
>   verdict. The capped/deduped sources still attach (not a zero-source refusal),
>   and the cite-back / provenance contracts still hold. The genuine
>   cited-synthesis happy path is returned verbatim, unframed.
>
> **Latency ceiling (F-LAT — important for operators).** The live `/ask` path
> uses the facade fast-path (`runOpenKnowledgeDirect`), which invokes the agent
> loop directly with the HTTP request context. The substrate scenario limits
> (`config/prompt_contracts/open_knowledge.yaml` → `limits.timeout_ms` 120000 and
> `limits.per_tool_timeout_ms` 30000) do **NOT** bound the wired reasoning turn —
> they only govern the not-wired substrate fallback, and the substrate loader
> hard-caps `timeout_ms` at 120000. The real ceiling is the HTTP server
> `WriteTimeout` in `cmd/core/main.go`, sized to the worst-case invariant
> `max_iterations × llm_timeout_ms`; spec 084 raised it to `6 × 600s = 3600s`.
> Realistic home-lab (gemma4:26b, resident) reasoning turns are ~40-60s at 6
> iterations (vs ~25-36s at 4); the 3600s ceiling is a slow-CPU-dev backstop. If
> an operator later raises `max_iterations` to 8+, revisit ONLY the `WriteTimeout`
> invariant — not the substrate limits. The deployed model matrix (gemma4:26b
> home-lab / gemma3:4b dev) is unchanged by spec 084.

> **Spec 087 — genuine synthesis (split reasoning model + retry-before-salvage).**
> Spec 084's hypothesis (gemma4:26b reasons well once unshackled) was empirically
> disproven: with the straitjacket removed, gemma4:26b still failed to synthesize
> the pomegranate-comparison class and fell to honest salvage. Spec 087 fixes the
> synthesis turn itself:
>
> - **Split synthesis model (`assistant.open_knowledge.synthesis_model_id`).** The
>   tool-calling GATHER turns keep using `llm_model_id` (gemma — a strong
>   tool-caller). The tools-stripped forced-final SYNTHESIS turn (and its retries)
>   uses `synthesis_model_id` — a reasoning model. Dev default `gemma3:4b`
>   (== `llm_model_id`, no effective split, envelope-safe); home-lab override
>   `assistant_open_knowledge_synthesis_model_id: deepseek-r1:7b`. `deepseek-r1:7b`
>   (4864 MiB) is already in the ollama envelope via `OLLAMA_REASONING_MODEL` and
>   is an on-demand specialist (NOT in the concurrent interactive working-set
>   guard), so it co-resides with gemma4:26b (23296 MiB ≤ 28672 MiB) with no
>   `ollama_memory_limit` change. `deepseek-r1:32b` is the operator opt-up (raise
>   `ollama_memory_limit` first).
> - **`<think>` stripping.** Reasoning models emit a `<think>…</think>`
>   chain-of-thought before the answer. The agent strips it BEFORE citation
>   parsing and cite-back, so it can never reach the user body or become a
>   citation (a fabricated URL inside `<think>` is impossible to cite). No-op for
>   non-reasoning models.
> - **Retry-before-salvage (`assistant.open_knowledge.synthesis_retry_budget`,
>   default `1`).** An empty or ungrounded forced-final synthesis is re-issued with
>   an escalated "write the verdict now, no `<think>`, no preamble" prompt up to
>   `synthesis_retry_budget` times BEFORE the honest snippet salvage fires —
>   rescuing the reasoning model's "thought but did not conclude" failure mode.
>   `0` = no retry = the exact spec-084 salvage timing. Honest salvage remains the
>   genuine-failure fallback (Principle 8): genuine synthesis is what stops it
>   being the default outcome — the honest frame is NOT removed.
> - **`WriteTimeout` updated.** The worst-case `/ask` ceiling is now
>   `(max_iterations + synthesis_retry_budget) × llm_timeout_ms` =
>   `(6 + 1) × 600s = 4200s` (`cmd/core/main.go`). The synthesis turn (+ retry)
>   runs the reasoning model bounded by the same `llm_timeout_ms`, so the envelope
>   stays honest. Realistic home-lab turns (gemma4:26b gather + deepseek-r1:7b
>   synthesis, GPU-resident) complete in ~40-90s. If an operator raises
>   `max_iterations` or `synthesis_retry_budget`, recompute `WriteTimeout`.
>
> All spec-064 trust invariants are preserved verbatim: the cite-back verifier and
> provenance gate run on the post-`<think>`-strip text; capture-as-fallback (the
> Facade) is untouched and inviolable. The decisive home-lab re-verification of the
> pomegranate turn is a separate `bubbles.devops` dispatch (build new signed images
> carrying the synthesis split + `deepseek-r1:7b` model pull + bundle that sets the
> per-environment `synthesis_model_id`).

> **Spec 088 — runtime-switchable synthesis model (per-request, allowlist-gated).**
> The synthesis model can be switched per `/ask` invocation, on BOTH surfaces,
> WITHOUT redeploying — so an operator can A/B `gemma4:26b` vs `deepseek-r1:7b` on
> the synthesis turn live. The switch is a per-request PARAMETER, never an SST
> write (the build-once SST baseline is unchanged; C6).
>
> - **Surfaces (validated identically).**
>   - Telegram: `/ask --model=<id> <question>` — the `--model=` flag is parsed off
>     the front of the `/ask` line and stripped before the question reaches the
>     agent. A baseline `/ask <question>` (no flag) is byte-for-byte the spec-087
>     behaviour.
>   - Web/HTTP: `POST /v1/agent/invoke` with a top-level `"model": "<id>"` field
>     (optional; absent ⇒ baseline). The 200 success envelope always carries the
>     answering `model`.
> - **Allowlist (`assistant.open_knowledge.switchable_models`).** A REQUIRED,
>   non-empty-when-enabled, operator-curated list of models `/ask` may switch the
>   synthesis turn TO. Dev `[gemma3:4b]`; home-lab override
>   `assistant_open_knowledge_switchable_models: [gemma4:26b, deepseek-r1:7b]`.
>   Each entry MUST have a `model_memory_profiles` entry AND co-resident-fit the
>   env `ollama_memory_limit` alongside the gather model (`llm_model_id`) — the
>   SAME co-residence arithmetic spec 087 uses (gather `gemma4:26b` 18432 +
>   synthesis `deepseek-r1:7b` 4864 = 23296 ≤ 28672). This is enforced **fail-loud
>   at config-generation** (`internal/config/config.go::validateModelEnvelopes`):
>   an envelope-busting or un-profiled list aborts `config generate`.
>   `deepseek-r1:32b` is NOT switchable on home-lab (40960 > 28672) — the operator
>   opt-up (raise `ollama_memory_limit` first).
> - **Fail-loud rejection (two reason-codes).** An off-allowlist / un-profiled /
>   over-envelope `--model=` is rejected fail-loud BEFORE any Ollama call — never a
>   silent default, never a backend passthrough. `model_not_allowlisted` (unknown /
>   un-profiled / not-offered) vs `model_over_memory_envelope` (profiled but busts
>   the env envelope — the "raise the envelope first" opt-up). Telegram renders the
>   rejection sentence; HTTP returns **400** with
>   `{status:"rejected", error_code, rejected_model, allowed_models, default_model,
>   message}` — the `message` is the SAME sentence on both surfaces. The rejection
>   does NOT call the agent and does NOT capture-as-answer.
> - **Attribution (`— model:` / envelope `model`).** The answer is attributed to
>   the model of the turn that produced the final text (honest across success /
>   salvage / refuse / early-stop). HTTP carries `model` in the success envelope
>   ALWAYS; Telegram appends a `— model: <id>` footer ONLY when an override was
>   applied (a baseline answer grows no footer — NFR-4 / Principle 6).
> - **`WriteTimeout` UNCHANGED at `4200s`.** A per-request synthesis switch adds no
>   turns and changes neither `max_iterations` nor `synthesis_retry_budget`, so a
>   switched (even slower) synthesis model is bounded by the same `(6+1) × 600s`
>   envelope. A first-class compare-both affordance (deferred, F-COMPARE-LATENCY)
>   would run two passes and double the bound — if it is ever shipped, recompute
>   `WriteTimeout`. The live `gemma4:26b`-vs-`deepseek-r1:7b` A/B is a downstream
>   `bubbles.devops` dispatch.
> - All spec-064/084/087 trust invariants hold under ANY selected model: the
>   override changes WHICH model runs, never the trust perimeter (cite-back,
>   provenance / no-zero-source, capture-as-fallback, `<think>`-strip +
>   retry-before-salvage all run on the turn OUTPUT, model-agnostic).

> **Spec 089 — persistent default + per-user sticky + gather override + prod
> hot-swap (extends spec 088, does NOT amend it).** Spec 088 shipped the
> per-request synthesis switch; spec 089 adds three more selection axes and a
> documented prod hot-swap, all sharing ONE precedence resolver
> (`modelswitch.ResolveEffective`), ONE claim-bound store
> (`user_model_preferences` / `modelpref`), and ONE attribution shape across
> Telegram + web/HTTP (SCN-089-A11 parity).
>
> - **Persistent default (committed SST).** Home-lab promotes the standing
>   synthesis default `deepseek-r1:7b → deepseek-r1:32b`
>   (`environments.home-lab.assistant_open_knowledge_synthesis_model_id`),
>   raises `ollama_memory_limit` `28G → 48G` (so gather `gemma4:26b` 18432 +
>   synthesis `deepseek-r1:32b` 22528 = 40960 ≤ 49152), and adds `deepseek-r1:32b`
>   to `switchable_models` (7b stays the speed escape hatch). The **standing
>   default is now co-residence-checked** at config-generation
>   (`validateModelEnvelopes`), closing the spec-088 gap where only the switchable
>   entries were checked.
> - **Per-user sticky `/model` (claim-bound; persists).** A user sets a sticky
>   `/ask` synthesis model once; it applies to every later `/ask` until changed or
>   reset. Surfaces (identical CRUD via the same store + validator):
>   - Telegram: `/model` (show effective + switchable + default), `/model <id>`
>     (set), `/model default` (reset). NOT an agent run — a per-user CRUD.
>   - Web/HTTP: `GET /v1/agent/model` (show), `PUT /v1/agent/model {"model":"<id>"}`
>     (set), `DELETE /v1/agent/model` (reset). Behind bearer auth; the actor is the
>     PASETO subject (`auth.UserIDFromContext`) — the body NEVER carries a user id,
>     so a spoofed body id is structurally ignored (OWASP A01 / spec 044).
>   - An off-allowlist set is a fail-loud no-op: the rejection is rendered (HTTP
>     400) and the existing preference is UNCHANGED. An orphaned sticky (operator
>     retired the model from `switchable_models`) silently resolves to the SST
>     default + a structured log — it never breaks every `/ask` for that user.
> - **Per-request gather override (`--gather-model=` / `gather_model`).** SEPARATE
>   from `--model=` (synthesis). Re-points the gather/tool turns only, gated by the
>   NEW `assistant.open_knowledge.tool_capable_gather_models` SST set (home-lab
>   `[gemma4:26b, llama3.1:8b]`; the baseline `llm_model_id` MUST be a member). A
>   non-tool-capable gather is refused `model_not_tool_capable` (`rejected_turn:
>   gather`) BEFORE any gather turn runs — `deepseek-r1*` tool-calling is weak,
>   `gemma3:4b` errors `does not support tools`. Per-request only (sticky gather
>   deferred, F-STICKY-GATHER).
> - **Precedence + source (per turn).**
>
>   | Turn | per-request supplied | else sticky supplied | else |
>   |------|----------------------|----------------------|------|
>   | Synthesis | per-request wins · source `this question` | sticky wins · source `your default`; orphaned ⇒ default | SST default · no footer |
>   | Gather | per-request wins · source `this question` | *(sticky gather deferred)* | baseline gather · no footer |
>
>   Telegram footer: single `— model: <id> (<source>)` when only synthesis is
>   non-default; dual `— gather: <g> (<gsrc>) · synth: <s> (<ssrc>)` when a gather
>   override is active; NO footer on a pure system-default answer (NFR-4 /
>   Principle 6). HTTP envelope ALWAYS carries `model` + `model_source` +
>   `gather_model` + `gather_model_source`.
> - **Prod hot-swap (Fork D — ~15s core-recreate, the documented mechanism).**
>   To change the standing default (or any SST model surface) in prod:
>   1. Edit the committed SST (`config/smackerel.yaml`
>      `environments.<env>.assistant_open_knowledge_synthesis_model_id`, etc.).
>   2. `./smackerel.sh config generate --env <env>` — fail-loud (an over-envelope
>      standing default aborts here, naming the model + envelope).
>   3. The operator-private deploy adapter recreates the core service only
>      (`--no-deps`, image digests from the running container, ~15s; the ingestion
>      pipeline is a separate untouched service).
>   4. **Verify** via the boot log
>      `open-knowledge subsystem wired … synthesis_model=<new> …
>      tool_capable_gather_models=… sticky_pref_store=wired` AND a live `/ask`
>      whose envelope reads `model_source: default` with the new `model`.
>   True zero-downtime config-hot-reload is deferred (F-HOTRELOAD) — ~15s of `/ask`
>   unavailability on a deliberate model swap is immaterial for a single-operator
>   self-hosted assistant.
> - **Standing-default footprint headroom.** The `model_memory_profiles`
>   `deepseek-r1:32b=22528 MiB` is a q4-weights + small-ctx ceiling that UNDERSTATES
>   the real KV-cache-dominated footprint (~64 GB at the model's 131072 ctx) at the
>   pipeline's `per_query_token_budget=128000`. The real bound is the Docker
>   `OLLAMA_MEMORY_LIMIT` cgroup cap, which CONSTRAINS ollama's KV-cache: the live
>   A/B at 48G measured 82 GiB used / 26 GiB free, "no pressure", co-resident with
>   ingestion (`docs/experiments/open-knowledge-synthesis-model-ab.md`). The profile
>   is NOT bumped (it is a shared ceiling across the co-residence matrix); an
>   explicit synthesis `num_ctx` bound is the deferred follow-up (F-FOOTPRINT).
> - **`WriteTimeout` UNCHANGED at `4200s`.** The 32b default + a gather override +
>   a sticky preference all only SWAP which model occupies the synthesis or gather
>   seat on the EXISTING turns; none adds a turn or changes `max_iterations` /
>   `synthesis_retry_budget`. The 32b default changes TYPICAL latency (~1.9×), not
>   the MAX. Raising `synthesis_retry_budget` 1→2 (F-RETRYBUDGET) would re-derive
>   `WriteTimeout` to `(6+2)×600s = 4800s` — the explicit reason it is a deferred
>   operator knob, not a silent change.
> - **All spec-064/084/087/088 trust invariants hold under ANY selection** (the
>   resolver changes WHICH model + WHICH turn, never the trust perimeter). The live
>   deepseek-r1:32b standing-default deploy (persist 48G + the 32b default +
>   pull-on-deploy + re-verify) is a SEPARATE downstream `bubbles.devops` dispatch.

### Enabling / Disabling

The subsystem ships disabled. Operator opts in by flipping
`assistant.open_knowledge.enabled: true` in
[`config/smackerel.yaml`](../config/smackerel.yaml), populating the
required keys below, and regenerating + redeploying the per-env
config bundle. Setting `enabled: false` cleanly disables the
scenario; other scenarios continue routing through spec 061 as
before. There is no half-enabled mode — the loader rejects an
enabled config with empty `provider_endpoint`, empty
`llm_model_id`, zero budgets, or empty `tool_allowlist`.

### Provider Choice And Tradeoffs

Exactly one provider is selected via `assistant.open_knowledge.provider`:

| Provider | `provider` value | Hosting | Egress posture | Notes |
|----------|------------------|---------|----------------|-------|
| SearxNG (self-hosted) | `searxng` | In-cluster Docker container behind the `searxng` Compose profile | Loopback-only between Smackerel and the SearxNG container; SearxNG itself reaches upstream engines | Local-first / privacy-preserving. Requires the per-env `searxng_enabled: true` flag and `assistant.open_knowledge.searxng.*` keys populated. No `provider_api_key` required (leave empty string). |
| Brave Search API | `brave` | SaaS | Outbound HTTPS to the Brave API host | Requires non-empty `provider_api_key`. Operator MUST add the Brave API host to `allowed_egress_hosts` and to any network-layer firewall allowlist. |
| Tavily | `tavily` | SaaS | Outbound HTTPS to the Tavily API host | Requires non-empty `provider_api_key`. Same egress allowlist rule applies. |

The spec-064 v1 plan ships SearxNG as the local-first default
recommendation. Brave / Tavily are pluggable behind the same
`WebSearchProvider` contract once the operator accepts the SaaS egress
tradeoff.

### Budget Configuration

All four budgets are REQUIRED and validated at the generator
boundary; the agent terminates the loop and returns
`budget_exhausted` (per-turn) or
`open_knowledge_refusal_total{cause="budget-exhausted-monthly"}`
(monthly) when a budget is exceeded.

| SST key | Scope | Semantics |
|---------|-------|-----------|
| `assistant.open_knowledge.per_query_token_budget` | Per turn | Total LLM tokens (prompt + completion) across all iterations of one turn. |
| `assistant.open_knowledge.per_query_usd_budget` | Per turn | Total estimated USD spend across all iterations of one turn. |
| `assistant.open_knowledge.monthly_budget_usd` | Per deployment | Aggregate monthly USD cap across all users. **NOT a sentinel: `0` means a $0 cap, not "unlimited".** MUST be `> 0` when `enabled: true`. (BUG-064-001) |
| `assistant.open_knowledge.per_user_monthly_budget_usd` | Per user | Per-user monthly USD cap. **`0` is enforced by the SCN-064-A08 pre-flight gate as "no allowance" — the agent refuses EVERY query (`cap_usd`) before any LLM/tool call.** MUST be `> 0` when `enabled: true`. To DISABLE the capability use `enabled: false`, NOT a `0` budget. The default self-hosted deployment runs a zero-cost `CostFn` (local Ollama + searxng), so the configured ceiling never actually binds — it is a guardrail only if a paid provider + real `CostFn` is later wired. (BUG-064-001) |
| `assistant.open_knowledge.max_iterations` | Per turn | Hard cap on planner ↔ tool ↔ observation cycles. |
| `assistant.open_knowledge.llm_timeout_ms` | Per LLM roundtrip | Caps each `/llm/chat` call to the ML sidecar independently. |

### Tool Allowlist

`assistant.open_knowledge.tool_allowlist` is the deny-by-default
operator-controlled list of tool IDs the agent may call. v1 set is
`internal_retrieval`, `web_search`, `unit_convert`, `calculator`
(matching the spec 064 `policySnapshot.v1ToolSet`). Removing
`web_search` produces an internal-knowledge-only deployment; the
agent then returns
`open_knowledge_refusal_total{cause="internal-only-restricted"}` for
any query whose resolution required outbound retrieval. An empty
list disables the subsystem effectively (no tools means no plan can
execute).

### Egress Allowlist

`assistant.open_knowledge.allowed_egress_hosts` is the
application-layer egress gate applied to every outbound HTTP from
the open-knowledge subsystem. Entries MUST be bare hostnames (no
scheme, port, path, or userinfo); wildcards are NOT supported in
v1. The provider's configured `provider_endpoint` host is implicitly
allowed. Empty list = provider endpoint only (deny-by-default per
G028).

Operators using `brave` or `tavily` MUST add the provider's API host
to this list. Operators using `searxng` typically leave this list
empty because all egress flows through the in-cluster SearxNG
container. PKT-020-A asks spec 020 to layer wildcard support plus a
network-layer firewall on top of this application-layer gate; until
then, this is the only egress control specific to the open-knowledge
subsystem.

### Circuit Breaker

The web provider is wrapped in a per-process circuit breaker keyed on
consecutive countable failures (transport unreach, quota exceeded).
All three thresholds are REQUIRED:

| SST key | Default in committed YAML | Meaning |
|---------|---------------------------|---------|
| `assistant.open_knowledge.circuit_breaker.failure_threshold` | `5` | Consecutive failures that trip Closed → Open. |
| `assistant.open_knowledge.circuit_breaker.open_window_seconds` | `60` | Documented Open window for operator dashboards. |
| `assistant.open_knowledge.circuit_breaker.half_open_after_seconds` | `30` | Elapsed time before exactly one HalfOpen probe is allowed. |

When the breaker is Open, the agent short-circuits to
`open_knowledge_refusal_total{cause="provider-unavailable"}` without
calling the provider. PKT-022-A asks spec 022 to review these defaults
against the operational-resilience playbook.

### Refusal Taxonomy

Operators MUST recognise these refusal causes in metrics and logs.
The full closed vocabulary lives in
[`internal/assistant/contracts/refusal.go`](../internal/assistant/contracts/refusal.go)
(`RefusalCause` type, `AllRefusalCauses` slice). The open-knowledge
agent emits one of the following on every non-success turn:

| Cause | Triggered when | First-line check |
|-------|----------------|------------------|
| `budget_exhausted` | Per-turn iteration / token / USD cap hit. | Inspect `open_knowledge_refusal_total{cause="budget-exhausted-turn"}` rate. Tune `per_query_*` budgets up or investigate planner regression. |
| `tool_unavailable` | A required tool returned a hard error or was disabled mid-turn. | Cross-reference with `web_search_provider_errors_total` and tool-side logs; check breaker state. |
| `provider-unavailable` | Circuit breaker Open. | Inspect `web_provider_circuit_state` gauge; wait for the half-open probe or investigate provider quota. |
| `fabricated_source_blocked` | Cite-back verifier rejected the planner's citations (none hash-match the per-turn tool trace). | Inspect `fabricated_source_total` rate. A non-zero rate is a security signal — see security posture below. |
| `internal_only_restricted` | Operator removed `web_search` from `tool_allowlist` but the query needed outbound retrieval. | Expected if the operator chose internal-only mode; otherwise re-add `web_search`. |
| `ambiguous_not_clarified` | Planner could not pick a search target and the disambiguation budget was exhausted. | Inspect the user prompt; consider raising `max_iterations` or improving the user-facing disambiguation hint. |
| `budget-exhausted-monthly` | `monthly_budget_usd` or `per_user_monthly_budget_usd` exceeded. | Operator decision: raise the cap, or accept the deny for the rest of the month. |

Every refusal path persists an `Idea` artifact for the original
prompt (capture-as-fallback invariant per spec.md §3 Hard Constraint 1),
so no user input is lost regardless of cause.

### Metrics And Dashboard

The open-knowledge subsystem emits the metrics named in spec.md §10
under the `open_knowledge_*` and `web_provider_*` prefixes. The
Prometheus dashboard wiring is **pending** under route packet
PKT-049-A (routed to spec 049). Until that lands, operators can
inspect the metrics directly via the core `/metrics` endpoint
(`./smackerel.sh status` shows the endpoint).

### Security Posture

Three layered controls protect the deployment:

1. **Egress allowlist** (`allowed_egress_hosts` + implicit
   `provider_endpoint`) — application-layer host gate. Every
   outbound HTTP from the open-knowledge subsystem is checked before
   the dialler runs.
2. **Cite-back verifier** — mechanical, non-LLM checker that every
   final-answer citation hash-matches a per-turn tool result. The
   LLM is NEVER trusted to attest its own grounding.
   `fabricated_source_total > 0` is the security signal.
3. **Prompt-injection detection metric** — emitted on tool-output
   inspection per spec.md §6.4. A sustained non-zero rate indicates
   either a hostile upstream snippet or a planner regression; both
   warrant investigation.

### Troubleshooting

| Symptom | Likely cause | First-line check |
|---------|--------------|------------------|
| All open-knowledge turns refuse with `budget-exhausted-turn` | `per_query_token_budget` or `per_query_usd_budget` set too low for the configured LLM. | Inspect prior-day refusal rate; raise the budget or pick a cheaper model. |
| Sustained `provider-unavailable` | Circuit breaker Open for longer than `half_open_after_seconds`. | Check provider status; inspect `web_search_provider_errors_total` for the dominant error kind (transport vs quota). |
| `fabricated_source_total > 0` | LLM contract drift, prompt regression, or a hostile snippet causing the planner to confabulate. | Pull the per-turn trace (redacted log); review the prompt contract and the offending snippet. |
| All turns refuse with `internal-only-restricted` | `web_search` removed from `tool_allowlist`. | Re-add `web_search` to the allowlist OR accept the internal-only posture. |
| SearxNG container crash-loops | Missing or malformed `assistant.open_knowledge.searxng.secret_key`. | Verify the key is non-empty in the resolved env; inspect SearxNG container logs. Operators MUST override the committed dev/test placeholder before any real deployment. |
| `open_knowledge_refusal_total{cause="budget-exhausted-monthly"}` spike | Monthly cap hit. | Operator decision: raise `monthly_budget_usd` / `per_user_monthly_budget_usd` or accept the deny. |

### Privacy Note

Enabling `web_search` introduces an outbound data-flow path: the
user's prompt (or a planner-derived query string) is sent to the
configured provider. The SearxNG self-hosted provider is the
local-first mitigation — query strings reach upstream engines via
SearxNG's anonymising proxy rather than directly identifying the
deployment. Brave and Tavily are SaaS; operators choosing them
accept that the provider sees query strings and source IP. Document
this for the deployment's privacy posture.

## Retrieval Routing & Evergreen Signal (spec 095)

Spec 095 adds **retrieval-strategy routing** (intent → read-path strategy
selection) and a **freshness-aware evergreen signal** scored at the ingestion
front door. Both operate over the SINGLE existing pgvector + knowledge-graph +
structured store — there is no parallel index (Principle 5); these keys only
select a read-path strategy and a lifecycle weighting, they never create a new
store. All behaviour is SST-driven from `config/smackerel.yaml`; every key below
is REQUIRED and fail-loud — a missing, empty, or out-of-range value aborts
startup with:

```text
[F095-SST-MISSING] missing or invalid required retrieval configuration: <key>
```

There are NO in-source fallback defaults (Gate G028 / NO-DEFAULTS).

### SST keys (authoritative — mirrors `config/smackerel.yaml`)

```yaml
retrieval:
  routing:
    enabled: true                          # master switch for retrieval-strategy routing
    intent_confidence_threshold: 0.65      # float in (0,1]; below this CompiledIntent confidence the router falls back to vague_recall (R5)
    strategies:
      whole_document_enabled: true         # enable the whole_document strategy (Idea 1a)
      structured_aggregate_enabled: true   # enable the structured_aggregate strategy (Idea 1b)
      vague_recall_enabled: true           # MUST be true — the router's safe fallback cannot be disabled (validator rejects false)
    contracts: { "transcript": [ "whole_document_summary", "vague_recall" ], "meeting": [ "whole_document_summary", "vague_recall" ], "subscription": [ "aggregate_spend", "vague_recall" ], "expense": [ "aggregate_spend", "vague_recall" ], "bill": [ "aggregate_spend", "vague_recall" ], "place": [ "dossier", "vague_recall" ], "trip": [ "dossier", "vague_recall" ] }
  evergreen:
    enabled: true                          # master switch for the evergreen ingestion-front-door signal (Idea 2)
    judgment_source: scenario              # enum: scenario (canonical LLM judgment) | tier_signals (deterministic fallback)
    confidence_floor: 0.60                 # float in [0,1]; operational decision-confidence safety gate (NOT a business cutoff)
    per_tick_budget: 50                    # positive int; per-ingestion-tick evergreen-judgment cap (NFR-2 throughput bound)
    dedup_window_days: 7                   # positive int; re-judge dedup window
    pools:
      synthesis_excludes_low_evergreen: false   # exclude low-evergreen items from the §10 synthesis candidate pool (R12); DEFAULT false = safe additive activation
      digest_excludes_low_evergreen: false      # exclude low-evergreen items from the §12 digest candidate pool (R12); DEFAULT false = safe additive activation
```

#### `retrieval.routing.*`

| Key | Value | Meaning |
|-----|-------|---------|
| `enabled` | `true` | Master switch for retrieval-strategy routing. |
| `intent_confidence_threshold` | `0.65` | Float in (0,1]. Below this `CompiledIntent` confidence the router falls back to `vague_recall` (R5). |
| `strategies.whole_document_enabled` | `true` | Enable the `whole_document` strategy (Idea 1a). |
| `strategies.structured_aggregate_enabled` | `true` | Enable the `structured_aggregate` strategy (Idea 1b). |
| `strategies.vague_recall_enabled` | `true` | MUST be `true` — the router's safe fallback cannot be disabled. Config validation rejects `false` with the named error `RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED (must be true — the router's safe fallback cannot be disabled)`. |
| `contracts` | per-type map (above) | Per-artifact-type admissible query shapes (Idea 3, R7/R8). Closed shape vocabulary: `whole_document_summary` \| `aggregate_spend` \| `dossier` \| `vague_recall`. Any artifact type absent from the map resolves to `[vague_recall]` (R9 fail-safe). Operator-overridable; adding a type is an additive edit with no router change. |

#### `retrieval.evergreen.*`

| Key | Value | Meaning |
|-----|-------|---------|
| `enabled` | `true` | Master switch for the evergreen ingestion-front-door signal (Idea 2). |
| `judgment_source` | `scenario` | Closed enum: `scenario` (canonical LLM `retrieval_evergreen` judgment) or `tier_signals` (deterministic fallback). An unrecognized value is rejected at startup. |
| `confidence_floor` | `0.60` | Float in [0,1]. Operational decision-confidence safety gate — a low-confidence "ephemeral" call is treated conservatively as evergreen (Principle 9, no wrongful exclusion). NOT a business cutoff. |
| `per_tick_budget` | `50` | Positive int. Per-ingestion-tick evergreen-judgment cap (NFR-2 throughput bound). |
| `dedup_window_days` | `7` | Positive int. Re-judge dedup window. |
| `pools.synthesis_excludes_low_evergreen` | `false` | Exclude low-evergreen items from the §10 synthesis candidate pool (R12). DEFAULT `false` = safe additive activation: the synthesis candidate set is byte-for-byte unchanged until the operator opts in. |
| `pools.digest_excludes_low_evergreen` | `false` | Exclude low-evergreen items from the §12 digest candidate pool (R12). DEFAULT `false` = safe additive activation: the digest candidate set is byte-for-byte unchanged until the operator opts in. |

### `judgment_source` fallback behaviour (NFR-2)

The evergreen signal NEVER blocks ingestion or search (R13). The judgment path
degrades gracefully so ingestion always proceeds:

| Condition | Path taken | `evergreen_source` provenance |
|-----------|-----------|-------------------------------|
| `judgment_source: scenario` AND the `retrieval_evergreen` scenario judge is wired AND returns successfully | Scenario decides evergreen ↔ ephemeral; the operational `confidence_floor` then gates whether a low-confidence "ephemeral" call is trusted | `scenario` |
| `judgment_source: scenario` BUT the judge is unavailable (not yet wired) or returns an error | Deterministic `TierSignals` fallback (transient source kinds → ephemeral) so ingestion never blocks | `tier_signals_fallback` |
| `judgment_source: tier_signals` | Deterministic `TierSignals` source used directly | `tier_signals` |

### Persistence & pool exclusion (migration 060)

The evergreen signal is persisted as two ADDITIVE, nullable columns on the
EXISTING `artifacts` table (migration `060_artifact_evergreen_signal.sql`) —
never a parallel store (Principle 5):

| Column | Type | Semantics |
|--------|------|-----------|
| `artifacts.evergreen_score` | `REAL` | Signed score: `>= 0` evergreen, `< 0` ephemeral (magnitude = calibrated confidence), `NULL` = not yet scored. No DB-side default — the app-side `evergreen.Scorer` writes it; a missing scorer leaves it `NULL` (NFR-3 graceful degrade). |
| `artifacts.evergreen_source` | `TEXT` | Judgment provenance (Principle 8 transparency): `scenario`, `tier_signals_fallback`, or `tier_signals`. `NULL` when `evergreen_score` is `NULL`. |

Pool exclusion (when an `*_excludes_low_evergreen` toggle is on) removes
low-evergreen items ONLY from the §10 synthesis and §12 digest candidate pools.
It NEVER hides them from search/retrieval — excluded items stay fully searchable
(R13 / Principle 9). A `NULL` (not-yet-scored) score is **never** excluded; it is
treated as evergreen.

### Enable / roll back pool exclusion

Pool exclusion ships OFF (`false`) for safe additive activation. To turn it on
(or roll it back), follow the same flip-regenerate-restart flow used for
connectors:

1. Edit `config/smackerel.yaml` → set `retrieval.evergreen.pools.synthesis_excludes_low_evergreen` and/or `retrieval.evergreen.pools.digest_excludes_low_evergreen` to `true` (or back to `false` to roll back).
2. Regenerate config: `./smackerel.sh config generate`
3. Restart: `./smackerel.sh down && ./smackerel.sh up`

No rebuild or schema change is required — the toggle is read fail-loud at
startup, and migration 060 is additive-only (manual rollback drops the two
columns; see the migration's rollback footer). Disabling routing or the
evergreen signal entirely follows the same flip-regenerate-restart flow on
`retrieval.routing.enabled` / `retrieval.evergreen.enabled`.


