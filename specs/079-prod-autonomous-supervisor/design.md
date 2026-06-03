# Design: 079 Production Autonomous Supervisor

**Status:** draft (analyst-bootstrapped; refinements deferred to `bubbles.design` after operator ratification)
**Companion to:** `spec.md` in this folder

---

## 1. Architecture Options Explored

### Option A — Separate Container, Same Host (RECOMMENDED for ratification)
- Supervisor runs as its own Docker container on the home-lab host (e.g. `smackerel-supervisor`), launched and lifecycle-managed by the **knb deploy adapter** (NOT by `docker-compose.yml` in this repo).
- Network: dedicated bridge network; only egress to Prometheus, NATS metrics endpoint, Postgres read-only role, container runtime socket for healthcheck reads (NOT for exec). No shared volumes with smackerel-core.
- Pros: strong fault isolation; clear ownership boundary (deploy adapter owns it); can be killed without touching prod.
- Cons: requires deploy-adapter authoring; one more container to monitor.

### Option B — Sidecar in smackerel-core Stack (REJECTED)
- Add supervisor as a service in `docker-compose.yml` next to `smackerel-core`.
- Rejected: co-location breaks fault isolation (supervisor crash can take down the network namespace; supervisor noisy-neighbor competes for host resources budgeted for core).

### Option C — Off-Host Control Plane (DEFERRED)
- Supervisor runs on a separate small host (e.g. raspberry-pi) and reaches the home-lab over the tailnet.
- Pros: hardest isolation; supervisor failure cannot affect prod at all.
- Cons: requires additional hardware + tailnet-edge configuration; out-of-scope for a single-host home-lab today. Worth revisiting if framework-promoted to `bubbles.supervisor`.

**Recommended for operator ratification: Option A.** Option C may become the recommended pattern at framework-promotion time.

---

## 2. Telemetry Sources (read-only)

| Source | Surface | Read-only role / credential |
|--------|---------|------------------------------|
| Prometheus | HTTP `/api/v1/query` against home-lab Prometheus | No auth on home-lab; supervisor binds to tailnet IP only |
| Container health | Docker socket (read endpoints only) OR `cAdvisor` HTTP | Socket access is dangerous; prefer cAdvisor. If socket is used, mount read-only and document why |
| NATS lag | NATS monitoring endpoint `/jsz`, `/connz` | Read-only by protocol |
| Postgres slow-query log | Tail `pg_log/*.log` via filesystem mount OR query `pg_stat_statements` with a dedicated read-only role | Dedicated PG role with SELECT on `pg_stat_statements` only |

All telemetry consumption is pull-based by the supervisor. The supervisor does NOT register as a Prometheus scrape target itself (until needed for self-observability — see §7).

---

## 3. Decision Policy (Anomaly → Action)

Evaluation order per detected anomaly:

```
1. Is the candidate's symptom-signature already covered by an open spec
   (status in {in_progress, specs_hardened}) per OpenCoverageIndex?
   → YES: action=defer, write ledger entry, STOP.
2. Has the supervisor filed >=3 bugs for this signature in rolling 24h?
   → YES: action=self-throttle, write ledger entry, STOP.
3. Is now within operator-declared quiet hours AND severity != critical?
   → YES: action=quiesce, queue for 08:00 re-eval, STOP.
4. Does the supervisor hold an unexpired capability token with
   scope="bug-file" and remaining-actions > 0?
   → NO: action=detect-only, write ledger entry, STOP.
   → YES: file bug, decrement token, write ledger entry, STOP.
```

Critical-severity rules (the only ones that can wake the operator) MUST be enumerated in supervisor config with explicit operator sign-off. Default critical set is EMPTY.

---

## 4. Capability Token Model

Tokens are issued by the operator out-of-band (e.g. `bash smackerel-supervisor-cli issue-token --scope=bug-file --ttl=24h --max-actions=5`) and dropped into a host directory the supervisor reads on-the-fly. Tokens are signed; signature verification is mandatory at load time.

Required fields:
- `id` (uuid)
- `scope` (enum: `bug-file`, `fix-dispatch`)
- `ttl` (RFC3339 expiry)
- `max-actions` (int)
- `issued-by` (operator identity)
- `issued-at` (RFC3339)
- `sig` (operator-key signature over the above)

Missing any bound (scope, ttl, max-actions) → REFUSE at load. Tokens that hit `max-actions=0` or `ttl expired` are removed from the token store; supervisor falls back to read-only.

Revocation: operator deletes the token file. Supervisor re-reads the token directory on every action attempt; no caching of "I previously had a token."

---

## 5. Append-Only Ledger

- Location: outside this repo and outside the smackerel-core volume tree. Lives on the supervisor host at a path declared by the knb deploy adapter (e.g. `/srv/supervisor/ledger.jsonl`).
- Format: JSONL, one entry per line, no rewrites.
- Required fields per entry: `ts`, `action`, `signature`, `token_id?`, `target_ref?`, `reason?`, `evidence_path?`.
- Rotation: operator-owned (knb adapter handles rotation alongside backup-tier-1).
- Verification: a periodic supervisor self-check appends an `action=ledger-integrity` entry with hash of prior entries.

---

## 6. Boundary With Existing Agents

| Agent | Cadence | Surface | Boundary with supervisor |
|-------|---------|---------|---------------------------|
| `bubbles.upkeep` (Treena) | Calendar-driven | Backup, restore-drill, BCDR, patch, secret-rotation, flag-cleanup, compliance-sweep | Supervisor NEVER triggers upkeep tasks; upkeep NEVER consumes real-time telemetry. If both want to act, upkeep wins. |
| `bubbles.goal` | On-demand convergence loop | Closes specs/bugs to terminal status | Supervisor MAY dispatch `bubbles.goal` for a filed bug only with a `fix-dispatch`-scoped token. The convergence run itself happens OUTSIDE prod (against the repo, not against the live stack). |
| `bubbles.plan` / `bubbles.implement` / etc. | Operator-initiated | Spec authoring + delivery | Supervisor is a producer of bugs they consume; never the reverse. |

---

## 7. Self-Observability

The supervisor itself MUST be observable so the operator can detect when it has become the problem:
- Supervisor exposes Prometheus metrics on a tailnet-bound port: classification rate, ledger-append rate, token-load count, deference count, self-throttle count, action-refused count, CPU/memory/disk-write gauges.
- Operator's existing home-lab Prometheus scrapes the supervisor.
- If supervisor metrics breach declared self-budget, supervisor self-throttles AND writes a ledger entry; it does NOT page.

---

## 8. Build-Once Deploy-Many Alignment

Per `.github/copilot-instructions.md` "Build-Once Deploy-Many":
- The supervisor container image is built by CI and pushed to ghcr by digest (e.g. `ghcr.io/<owner>/smackerel-supervisor@sha256:<d>`).
- The knb deploy adapter owns the lifecycle script (`apply.sh`, `rollback.sh`, `verify.sh`) — NOT this repo.
- This repo provides the source + Dockerfile + CI build only. No env-specific values here; the adapter overlay binds host + token-store path + ledger path + telemetry endpoints.

---

## 9. Open Architecture Questions (for `bubbles.design` post-ratification)

1. Token signing key management: operator-laptop-resident vs hardware-key-resident? (operator preference)
2. Should bug filing happen via a git push from supervisor host (REJECTED — forbidden) or via a deploy-adapter-mediated PR opener that runs OFF the prod host? (deferred)
3. OpenCoverageIndex implementation: live scan of `specs/*/state.json` over a read-only repo mount, or pre-computed snapshot delivered by CI? (perf vs freshness tradeoff)
4. Quiet-hours definition: single window, or per-day schedule? (operator preference)
5. Framework promotion: timing for lifting capability-token + ledger + OpenCoverageIndex into `bubbles/` after spec 079 ships? (post-ratification decision)
