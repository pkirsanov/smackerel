# Smackerel — HOME-LAB Deployment Plan

> **Scope:** Repo-specific readiness/deployment plan for Smackerel on the HOME-LAB home server.
>
> **Cross-project plan:** [Home_Lab_Master_Deployment_Plan.md](./Home_Lab_Master_Deployment_Plan.md). Smackerel owns the Master plan in this repo.
>
> **Posture for first deploy:** "start using" — the only project intended for daily real use after Friday. Connectors must be set up once and **never lost** (re-syncing data is acceptable, re-authorizing connectors is not).
>
> **Last updated:** 2026-05-07
> **Target deploy slot:** Friday 2026-05-08 afternoon (first deploy on home-lab)

---

## 1. Readiness Verdict

| Area | Status |
|------|--------|
| Build & CLI surface | ✅ Mature (`./smackerel.sh`) |
| Config SSOT (`config/smackerel.yaml` → `config/generated/`) | ✅ |
| Docker compose, named volumes (`postgres-data`, `nats-data`, `ollama-data`) | ✅ |
| Connector framework + 13 connectors specified | ✅ Most connector specs `done` |
| `./smackerel.sh backup` (pg_dump to `backups/`) | ✅ Exists |
| TLS / reverse-proxy spec | ❌ Missing |
| Production deploy mode (`up` is dev/test) | ❌ Missing |
| Backup off-host + retention spec | ❌ Missing |
| Monitoring / alerting spec for home-lab | ❌ Missing (030-observability not done) |
| Resource limits spec | ❌ Missing |
| Secrets management spec (Telegram bot token, LLM API key, auth token) | ❌ Missing |
| QF integration (041-qf-companion-connector) | ⚠ in_progress (Scope 1 implemented, not certified; live runtime evidence still required) |
| WanderAide connector | ❌ Spec does not exist |

**Verdict:** Conditional Go for Friday. Deploy with these guarantees:
1. **Connector persistence:** the `postgres-data`, `nats-data`, and connector-input data volumes are pinned, labeled, and excluded from `clean` operations.
2. **Tailnet-only ingress** (no public TLS this week — closes the Caddy/HTTPS gap until next iteration).
3. **No experimental connector wiring on Friday** beyond the connectors that have `done` specs and are production-ready.

---

## 2. Existing specs/ops needed for first deploy

| Spec | Status | Use for home-lab |
|------|--------|----------------|
| 029-devops-pipeline | done | Build pipeline reused as-is |
| Connector specs (007 keep, 008 telegram, 009 bookmarks, 010 browser-history, 011 maps, 012 hospitable, 013 guesthost, 014 discord, 015 twitter, 016 weather, 017 gov-alerts, 018 financial-markets, 019 connector-wiring) | mostly done | Enable only the subset you intend to use Friday; defer the rest |
| 022-operational-resilience | check status before deploy | Runtime resilience baseline |
| 023-engineering-quality | check status before deploy | Code quality baseline |

---

## 3. New specs/ops needed before first home-lab deploy (gaps)

These are **blocking for "start using"** in the longer run, but the Master plan compensates by treating Friday as Tailnet-only. Open these in this repo:

| Proposed spec | Purpose | Blocking for |
|---------------|---------|--------------|
| `_ops/OPS-HOMELAB-101-homelab-deploy-mode` | Add a Smackerel "homelab" or "prod" mode to `./smackerel.sh up` (named project, `restart: unless-stopped`, host port block `40000–49999`, custom bridge `172.31.0.0/16`, persistent-volume label policy that survives `clean smart`/`clean full`) | Friday deploy |
| `_ops/OPS-HOMELAB-102-secrets-management` | Define how `auth_token`, `llm.api_key`, `telegram.bot_token`, connector `access_token` flow into home-lab without manual file editing; fail-loud on empty | Friday deploy |
| `_ops/OPS-HOMELAB-103-connector-volume-protection` | Pin `postgres-data`, `nats-data`, and connector input dirs (`data/bookmarks-import`, `data/maps-import`, `data/twitter-archive`, `data/browser-history`) under explicit named volumes / bind mounts; document recovery; integrate with backup target | Friday deploy — **directly addresses the user requirement "do not lose connectors after setup"** |
| `_ops/OPS-HOMELAB-104-tls-and-reverse-proxy` | Front Smackerel via host Caddy; no in-stack TLS; plus internal-bind-only `127.0.0.1` checks | Public exposure (post-Friday) |
| `_ops/OPS-HOMELAB-105-backup-and-retention` | Extend the existing `backup` subcommand to push to `/home/homelab/backups/smackerel/`, retention, restore-test command, NATS JetStream stream snapshot | After first deploy, before any real connector data accrues |
| `_ops/OPS-HOMELAB-106-monitoring-alerting-log-retention` | Health endpoints visible to host Uptime Kuma, log rotation policy beyond Docker's 10 MB × 3, error notification channel | After first deploy |
| `_ops/OPS-HOMELAB-107-resource-limits` | Per-service CPU/memory limits + reservations (mirror the QF Spec 060 / GH OPS-097 pattern) | Stability under multi-project load |
| `_ops/OPS-HOMELAB-108-uptime-kuma-monitors-smackerel` | Declare the Smackerel monitor set from Master plan §8 as config-as-code | After first deploy |
| `043-wanderaide-connector` (new feature spec) | Mirror of 013-guesthost-connector for WA | Post-WA-deploy iteration |
| `_ops/OPS-HOMELAB-003-caddy-tailnet-routes` (Master) | Caddy routes for all 4 projects | Public-tailnet exposure |
| `_ops/OPS-HOMELAB-004-host-firewall` (Master) | ufw baseline | Before public exposure |
| `_ops/OPS-HOMELAB-005-host-backups` (Master) | Off-host backup target | Before any data is precious |
| `_ops/OPS-HOMELAB-006-monitoring-catalog` (Master) | Cross-project Uptime Kuma catalog | Friday afternoon |

---

## 4. Connector preservation contract (NON-NEGOTIABLE)

The user's hard requirement: connectors set up on home-lab must survive everything except a deliberate wipe.

| Asset | Lives in | Protection |
|-------|----------|------------|
| Connector OAuth tokens / API keys | Postgres `oauth_tokens`-like tables | Named volume `postgres-data` with explicit `name:` (already in compose) |
| Connector cursor state, dedup hashes | Postgres tables + NATS JetStream | `postgres-data` + `nats-data` named volumes |
| Read-only ingestion sources | `./data/bookmarks-import`, `./data/maps-import`, `./data/browser-history`, `./data/twitter-archive` | Bind mounts; back up nightly |
| Connector configuration | `config/smackerel.yaml` + `config/generated/` | Source-controlled; secrets via OPS-HOMELAB-102 |
| Prompt contracts | `config/prompt_contracts/` | Source-controlled |

Rules to encode in OPS-HOMELAB-103:
1. `./smackerel.sh down` MUST NOT pass `--volumes`.
2. `./smackerel.sh clean smart` MUST keep `postgres-data` + `nats-data` (already documented; verify in the spec).
3. `./smackerel.sh clean full` MUST require explicit confirmation token on home-lab.
4. Volume names MUST be project-namespaced via the `${POSTGRES_VOLUME_NAME}` env vars (already wired via compose).

---

## 5. SQL consolidation task (single init script)

Current state: `internal/db/migrations/` has 15 active SQL files (`001_initial_schema` + 018–031) plus a 17-file `archive/`.

**Proposed work:**
- New spec: `_ops/OPS-HOMELAB-110-init-schema-consolidation`.
- Action: collapse the 15 active migrations into a new `internal/db/migrations/001_initial_schema.sql` (regenerated), move 018–031 into `internal/db/migrations/archive/` with a header pointing at the new baseline, document consolidation date.
- Constraint: run `pg_dump --schema-only` against a database produced by the current sequential apply, regenerate, diff — must be byte-equivalent (excluding object create order).
- Out of scope for Friday. Schedule for week of 2026-05-12 (after first deploy is stable, before second iteration's migrations are added).

---

## 6. Worktree freeze and release commit

- The earlier weekend audit showed e2e was passing on the latest test run.
- Before Friday: pick deploy commit on `main`, create branch `release/homelab-2026-05-08`, push.
- Verify with: `./smackerel.sh check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` — all must be clean on the deploy commit.

---

## 7. First-deploy checklist (Friday afternoon)

1. ✅ Apply Master plan §3 Tailscale ↔ Docker bridge mitigation (host-side, before any container starts on home-lab).
2. ✅ Provision `/home/homelab/backups/smackerel/` and `/home/homelab/smackerel/data/` for connector inputs.
3. ✅ Land OPS-HOMELAB-101 + OPS-HOMELAB-102 + OPS-HOMELAB-103 (minimum viable subset).
4. ✅ `./smackerel.sh config generate` for the homelab env.
5. ✅ `./smackerel.sh build` on the deploy commit.
6. ✅ Copy resolved env to home-lab via OPS-HOMELAB-102 mechanism.
7. ✅ `./smackerel.sh up` on home-lab.
8. ✅ `./smackerel.sh status` + `./smackerel.sh logs` clean.
9. ✅ Authorize at least one read-only connector end-to-end (suggested: bookmarks or browser-history — no OAuth round-trip needed).
10. ✅ Authorize one OAuth connector (suggested: telegram bot capture, since it's the daily-driver path).
11. ✅ Trigger `./smackerel.sh backup`; verify file lands in `/home/homelab/backups/smackerel/`.
12. ✅ Restore-test the backup into a throwaway Postgres on home-lab; verify connector tokens decrypt.
13. ✅ Add Uptime Kuma monitors from Master plan §8.
14. ✅ Add Caddy route placeholder; do not enable until OPS-HOMELAB-104 lands.

---

## 8. Post-deploy iteration backlog

| Item | Priority | Notes |
|------|----------|-------|
| OPS-HOMELAB-104 — TLS via host Caddy | P1 | Required before any non-Tailscale device accesses Smackerel |
| OPS-HOMELAB-105 — backup retention + off-host copy | P1 | Local-only backups violate DR |
| OPS-HOMELAB-106 — monitoring/alerting/log retention | P1 | Telegram alert channel preferred |
| OPS-HOMELAB-107 — resource limits | P2 | Once GH/QF/WA also run on the host |
| 030-observability spec completion | P2 | Brings dashboards online for Smackerel |
| 041-qf-companion-connector Scope 2-5 | P2 | Once QF Spec 063 Scope 2 unblocks |
| New spec: `043-wanderaide-connector` | P2 | Mirror of 013-guesthost-connector |
| OPS-HOMELAB-110 — single init schema | P2 | Week of 2026-05-12 |
| Public TLS / real DNS | P3 | Only if smackerel ever needs to be reachable off-tailnet |

---

## 9. Hard "no-go" items for Friday deploy

- ❌ No public-internet exposure (Tailnet only until OPS-HOMELAB-104 + Master OPS-HOMELAB-004 land).
- ❌ No `--volumes` passed to `./smackerel.sh down` on home-lab (would destroy connectors).
- ❌ No experimental connector wiring (only those with `done` specs).
- ❌ No re-using dev secrets — every secret is generated fresh for home-lab.
- ❌ No `./smackerel.sh clean full` on home-lab without an explicit confirmation token.
