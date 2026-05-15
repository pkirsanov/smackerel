# Smackerel Operations Runbook

This guide covers deployment, daily operations, connector management, troubleshooting, backup/restore, and monitoring for a self-hosted Smackerel instance.

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

`<host-tailnet-fqdn>` is the host's Tailscale FQDN (e.g.,
`<deploy-host-fqdn>`). The exact subdomain shape is owned by the deploy adapter
adapter and can be customized per deployment.

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

### Reset a Connector's Sync Cursor

If a connector is stuck or you want to re-sync from scratch, clear its cursor in the database. This requires the stack to be running:

```sql
-- Connect to PostgreSQL
-- psql "postgres://smackerel:smackerel@127.0.0.1:42001/smackerel"

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

**Log/trace redaction:** All serialized recommendation logs and traces are scanned for forbidden substrings (provider API keys, raw provider payloads, sensitive graph prompt text, raw GPS coordinates) at the persistence boundary via `internal/recommendation/store.AssertRedactSafe`. The unit test `internal/recommendation/store/redact_test.go::TestRecommendationRedaction_NoSecretsOrRawLocationInLogsOrTraces` is the regression guard.

#### ML Sidecar (`http://127.0.0.1:40002/metrics`)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_llm_tokens_used_total` | Counter | `provider`, `model` | LLM token usage per provider/model |
| `smackerel_ml_processing_latency_seconds` | Histogram | `operation` | ML processing latency per operation |

Model label cardinality is bounded: known models pass through, unknown models map to `other`.

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
sidecar; if your observability platform should alert on sustained
breaches, point your alert at `histogram_quantile(0.95,
rate(smackerel_ml_request_latency_seconds_bucket{endpoint="/health"}[5m]))`
and compare against the SLA value.

## TLS Setup

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
     env SMACKEREL_BOOTSTRAP_TOKEN='<bootstrap secret>' \
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
