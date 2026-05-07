---
name: bubbles-deployment-target-adapter
description: Enforce per-target deployment adapters that consume an SST-derived contract and own ALL target-specific knowledge. Use when adding or changing how a project is deployed to a real machine, cluster, cloud, or home lab; when authoring `deploy/<target>/` scripts; when reviewing changes that touch host singletons (Caddy, Docker daemon, ufw, systemd, Tailscale, mDNS); when designing CLI surfaces for `deploy <target> <action>`. Triggers include new deployment target proposals, HOME-LAB / cloud / staging deployment work, host-config changes, idempotency reviews, and `docs/Deployment*.md` edits.
---

# Bubbles Deployment Target Adapter

## Goal

Make a project deployable to **multiple, simultaneous targets** (home lab, cloud, staging VPS, dev laptop) without leaking target-specific values into the project's SST or main docs, and without any one target's bootstrap destroying another target's state on a shared host.

The pattern is: **SST → generated contract → per-target adapter**. The contract describes WHAT to deploy (services, ports, volumes, env keys). The adapter describes WHERE and HOW (FQDNs, IPs, TLS dirs, ufw rules, systemd unit names, host-singleton drop-ins).

## Use This Skill When

- Adding a new deployment target (HOME-LAB, AWS, Fly.io, GCP, staging VPS, on-prem K8s).
- Modifying any file under `deploy/`.
- Modifying anything that mutates host state (Caddy, `/etc/docker/daemon.json`, ufw, systemd units, hostnames, Tailscale ACLs).
- Editing or reviewing `docs/Deployment*.md`.
- Reviewing PRs that introduce a new service, persistent volume, public port, or background worker.
- Designing or extending a project CLI's `deploy` subcommand.
- Auditing whether the project still has cross-target leakage in the SST.

## Adapter Lifecycle

```
┌────────────────────────────────┐
│  SST: config/<project>.yaml    │  ← Target-agnostic project config
└────────────────┬───────────────┘
                 │
                 │ ./<project>.sh config generate
                 ▼
┌────────────────────────────────┐
│  deploy/contract.yaml          │  ← Generated; target-agnostic; DO NOT EDIT
│  - services                    │
│  - ports (host & internal)     │
│  - persistent volumes          │
│  - required env keys           │
│  - health endpoints            │
│  - host singleton declarations │
└────────────────┬───────────────┘
                 │
                 ▼
┌────────────────────────────────────────────────────────────────┐
│  deploy/<target>/                                              │
│  - params.yaml       (FQDNs, IPs, TLS dirs, target identity)   │
│  - preconditions.sh  (verify target is ready)                  │
│  - bootstrap.sh      (idempotent install/upgrade)              │
│  - verify.sh         (post-deploy health/smoke)                │
│  - teardown.sh       (precise removal)                         │
│  - README.md         (operator-facing)                         │
└────────────────────────────────────────────────────────────────┘
```

## Required Layout

```
deploy/
├── README.md                  ← Index of adapters + contract overview
├── contract.yaml              ← Generated from SST; DO NOT EDIT
└── <target>/                  ← Per-target adapter directory
    ├── params.yaml            ← Target-specific values
    ├── preconditions.sh       ← Verifies target is ready (idempotent, exit 0 if good)
    ├── bootstrap.sh           ← Idempotent install/upgrade (re-run = no-op)
    ├── verify.sh              ← Post-deploy health/smoke checks
    ├── teardown.sh            ← Removes ONLY what bootstrap created
    └── README.md              ← Operator-facing usage doc
```

## CLI Surface

The project CLI MUST expose a single deployment surface:

```
./<project>.sh deploy <target> <action>

Actions:
  preconditions    Verify the target host is ready (no mutation)
  bootstrap        Idempotent install / upgrade
  verify           Post-deploy health and smoke checks
  teardown         Remove only what bootstrap created
  status           Show current state of this adapter on the target
  params           Print resolved params (params.yaml + contract merge)
  contract         Regenerate deploy/contract.yaml from SST and exit
```

The CLI MUST refuse to run any action other than `preconditions`, `params`, or `contract` if `deploy/contract.yaml` is stale relative to the SST (drift check first).

## Classification Rules

| Value Type | Lives In |
|------------|----------|
| Service list, ports, internal DNS names, env key names, health endpoints, persistent volumes | `deploy/contract.yaml` (generated from SST) |
| Target FQDN, target host IP, Tailscale machine name, target-specific TLS cert dir, target user/group, target storage paths | `deploy/<target>/params.yaml` |
| Caddy site config for target | `deploy/<target>/conf.d/<project>-<target>.caddy` (dropped in by bootstrap) |
| ufw rule set for target | bootstrap script, tagged with `# project=<name> target=<target>` |
| systemd unit names | `<project>-<target>-<purpose>.service` |
| Container names | `${PROJECT}-${TARGET}-${SERVICE}` |
| Network names | `${PROJECT}_${TARGET}_default` |
| Volume names | `${PROJECT}_${TARGET}_${VOLUME}` |

Any value in `deploy/<target>/params.yaml` MUST NOT also live in the SST. Any value in the SST MUST NOT also live in `params.yaml`.

## Host Singleton Pattern

Some host resources permit only one canonical owner per host. Adapters MUST coexist via drop-in / namespace / assert:

| Singleton | Drop-In Pattern | Bootstrap Assertion |
|-----------|----------------|---------------------|
| Caddy | `/etc/caddy/conf.d/<project>-<target>.caddy` (main `Caddyfile` does `import conf.d/*.caddy`) | `caddy validate` |
| Docker daemon.json | Read JSON, deep-merge required keys, write back | Re-merge MUST be a no-op on second run |
| ufw | Tagged rules with `# project=<name> target=<target>`; teardown removes only these | `ufw status numbered` parsed for own tag |
| systemd | Namespaced unit names: `<project>-<target>-<purpose>.service` | `systemctl status` for each own unit |
| Hostname / Tailscale tag | Operator owns; adapter consumes via params.yaml | bootstrap fails if hostname/Tailscale identity mismatch |

The bootstrap script MUST add an explicit assertion step that re-runs the singleton write logic and proves zero changes the second time (idempotency assertion).

## Idempotency Checklist

Every adapter MUST satisfy ALL of these:

```
[ ] First bootstrap on fresh target → healthy deployment
[ ] Second bootstrap immediately after → zero diffs, exit 0
[ ] verify.sh after bootstrap → all health checks pass
[ ] teardown.sh removes all adapter resources
[ ] After teardown, host singletons (Caddy main, daemon.json, ufw default, hostname) unchanged
[ ] After teardown, peer adapters (other targets on same host) unaffected
[ ] Bootstrap after teardown → succeeds with no manual intervention
[ ] Two adapters for different targets coexist on one host without collision
```

## Contract Generation

`deploy/contract.yaml` MUST be generated from the SST by the same generator that produces `.env` files and Compose files. It MUST contain at least:

```yaml
project: <project>
contractVersion: <semver>
generatedAt: <iso8601>
sourceSha: <sha256 of SST>

services:
  - name: gateway
    image: <project>/gateway:<tag>
    healthEndpoint: /health
    ports:
      host: ${PORTS_GATEWAY_HOST}     # Resolved by adapter from contract + params
      internal: 8080
    env:
      required: [DATABASE_URL, JWT_SECRET]
      optional: [LOG_LEVEL]
    persistentVolumes: []

persistentVolumes:
  - name: ${PROJECT}_${TARGET}_postgres_data
    requiredDir: /var/lib/postgresql/data

hostSingletons:
  caddy:
    required: true
    dropInDir: /etc/caddy/conf.d
  ufw:
    required: true
    rules:
      - { proto: tcp, from: any, to: any, port: 443, action: allow }
```

Generated files MUST contain a header marking them as auto-generated and listing the SST source.

## Verification Commands

```bash
# 1. Contract is fresh (no SST drift)
./<project>.sh deploy <target> contract
git diff --exit-code deploy/contract.yaml

# 2. Preconditions pass
./<project>.sh deploy <target> preconditions

# 3. Bootstrap is idempotent (run twice, second run is a no-op)
./<project>.sh deploy <target> bootstrap
./<project>.sh deploy <target> bootstrap   # MUST exit 0 with zero state changes

# 4. Verify passes
./<project>.sh deploy <target> verify

# 5. Teardown is precise (host singletons untouched)
./<project>.sh deploy <target> teardown
diff -q /etc/caddy/Caddyfile.snapshot.before /etc/caddy/Caddyfile     # unchanged
diff -q /etc/docker/daemon.json.snapshot.before /etc/docker/daemon.json  # unchanged
sudo ufw status numbered | grep "project=<project> target=<target>"      # 0 matches

# 6. Bootstrap after teardown succeeds
./<project>.sh deploy <target> bootstrap
./<project>.sh deploy <target> verify
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| Hardcoded HOME-LAB IP `<host-tailnet-ip>` in SST | Target leakage | Move to `deploy/home-lab/params.yaml` |
| `bootstrap.sh` writes `/etc/caddy/Caddyfile` | Destroys peer adapters | Drop file in `conf.d/`, assert main file imports it |
| Container named `gateway` (no namespace) | Collides with peer adapter for another target | `${PROJECT}-${TARGET}-gateway` |
| `teardown.sh` runs `docker system prune -af` | Destroys peer adapters' images | Remove only this adapter's labelled resources |
| `docs/Deployment.md` describes one target step-by-step | New targets force doc rewrite | Main doc explains contract; per-target docs in `deploy/<target>/README.md` |
| Bootstrap requires manual `systemctl daemon-reload` | Not idempotent in CI | Bootstrap runs the reload itself |
| Two adapters share the same volume name | Data corruption when both run on same host | `${PROJECT}_${TARGET}_${VOLUME}` |
| `params.yaml` references a value also in SST | Dual source of truth | Move to one location only |

## Integration With Bubbles Governance

| Bubbles Gate | Adapter Relevance |
|--------------|-------------------|
| Implementation Reality Scan (G028) | Detects target-specific values leaking into source/SST |
| Integration Completeness (G029) | New service must be added to contract AND wired into ≥1 adapter |
| Vertical Slice (G035) | Deployment artifact (Caddy site, systemd unit, etc.) must exist for every target the service is deployed to |
| Pre-Completion Self-Audit | `deploy <target> bootstrap` re-run idempotency proof must be in evidence |
| Docker Lifecycle Governance | Adapter teardown must not destroy peer adapters' state |

## References

- [bubbles-deployment-target.instructions.md](../../instructions/bubbles-deployment-target.instructions.md)
- [bubbles-config-sst skill](../bubbles-config-sst/SKILL.md)
- [bubbles-docker-lifecycle-governance skill](../bubbles-docker-lifecycle-governance/SKILL.md)
- [bubbles-docker-port-standards skill](../bubbles-docker-port-standards/SKILL.md)
- [bubbles-test-environment-isolation skill](../bubbles-test-environment-isolation/SKILL.md)
