# HOME-LAB Master Deployment Plan

> **Scope:** Cross-project coordination plan for deploying QuantitativeFinance (`qf`), Smackerel (`s`), GuestHost (`gh`), and WanderAide (`wa`) to the GMKtec HOME-LAB home server. Owns hardware/OS/network truth, port allocation, reverse-proxy contract, backup destinations, monitoring catalog, and the rollout schedule.
>
> **Authoritative for the host.** Per-project deployment plans live in each repo (`docs/Home_Lab_Deployment_Plan.md`) and reference this document. The previous `quantitativeFinance/docs/Homelab_Homelab_Setup.md` was merged here and removed from the QF repo.
>
> **Last updated:** 2026-05-07
> **Target first deploy:** Friday 2026-05-08 (Smackerel) / Saturday 2026-05-09 (GuestHost private preview, then QF + WanderAide as experimental sandboxes)

---

## 1. Hardware

| Component | Spec |
|-----------|------|
| CPU | AMD Ryzen AI Max+ 395, 16C/32T, Zen 5, up to 5.1 GHz |
| RAM | 128 GB LPDDR5X-8000 (soldered) |
| iGPU | AMD Radeon 8060S (RDNA 3.5, 40 CUs) |
| NPU | XDNA 2, 50 TOPS |
| Internal SSD | 2 TB NVMe (PCIe 4.0) |
| 2nd M.2 slot | Available (M.2-2280), not populated |
| Ports | 2× USB4 (USB-C), USB-A, HDMI, DisplayPort, 2.5 GbE LAN, SD 4.0 |
| Wireless | Wi-Fi 7 (MediaTek MT7925) + Bluetooth 5.4 |
| Power | 230 W PSU. Idle 6.5–10.5 W, Load ~215 W |
| Noise | 27.5 dB idle, ~47 dB full load |

### BIOS
- iGPU VRAM currently 64 GB (system shows 62 GB usable RAM). **Open task:** reduce to 16 GB to recover ~48 GB system RAM. Requires physical access + reboot.
- Boot order: Ubuntu only. Windows removed.
- GRUB: `TIMEOUT=0`, `TIMEOUT_STYLE=hidden`, `DEFAULT=0`.

---

## 2. Operating System

| Property | Value |
|----------|-------|
| OS | Ubuntu 24.04.4 LTS |
| Kernel | 6.17.0-20-generic |
| Hostname | `home-lab` |
| Timezone | America/Los_Angeles |
| User | `homelab` |
| Sudo | NOPASSWD via `/etc/sudoers.d/homelab` |

### Security baseline
| Setting | Status | Action |
|---------|--------|--------|
| SSH | Key-based auth configured | OK |
| Password | `***REMOVED***` (weak, date) | **MUST change before public exposure** |
| Unattended upgrades | Enabled | OK |
| Firewall (ufw) | Inactive | Add basic policy before any non-Tailscale ingress |

---

## 3. Networking

### Wi-Fi
- Managed by NetworkManager (renderer set in `/etc/netplan/50-cloud-init.yaml`).
- Cloud-init network config disabled.
- Active connection: `PhsRouter-5G` on `wlp195s0`.
- Wi-Fi must remain force-managed: `sudo nmcli device set wlp195s0 managed yes`.

### Tailscale (the only ingress)
| Property | Value |
|----------|-------|
| Tailscale IP | `<host-tailnet-ip>` |
| MagicDNS name | `home-lab` |
| Tailnet domain suffix | `<tailnet-domain>.ts.net` |
| Status | Enabled, auto-start on boot |

### Tailnet devices
| Device | IP | OS | Status |
|--------|----|----|--------|
| home-lab | <host-tailnet-ip> | Linux | Online |
| devbox-wsl (Devbox / WSL) | <devbox-tailnet-ip> | Linux | Online |
| router | <router-tailnet-ip> | Linux | Online |
| pihole | <pihole-tailnet-ip> | Linux | Online |
| iphone | <iphone-tailnet-ip> | iOS | Online |
| macbook-pro | <macbook-tailnet-ip> | macOS | Offline |
| android-tablet | <tablet-tailnet-ip> | Android | Offline |

### SSH access
```
# From Devbox (WSL) — passwordless via key
ssh home-lab

# ~/.ssh/config on Devbox
Host home-lab
    HostName <host-tailnet-ip>
    User homelab
    IdentityFile ~/.ssh/id_ed25519
```

### ⚠ Tailscale ↔ Docker bridge interference (BLOCKING for live monitoring)
This is reproduced and documented in QF as `BUG-CHAOS-060-003`. On a Tailscale-active host, container-to-container traffic on the default Docker bridge `172.18.0.0/16` can time out at L3/L4 even when the bridge looks healthy. This must be mitigated host-side **before** any project depends on inter-container Prometheus scraping or cross-stack HTTP.

Mitigations (apply in order; first that resolves it wins):
1. `/etc/docker/daemon.json` → `{"dns": ["172.18.0.1"], "dns-search": []}` to suppress Tailscale-injected container DNS.
2. Pin custom subnets per project network outside Tailscale-routed ranges (use `172.30.0.0/16`+ blocks reserved per project below).
3. Run Tailscale with `--accept-routes=false` on the host.
4. Document the residual incompatibility in the affected project runbook.

Owner: see [Cross-cutting gaps](#9-cross-cutting-gaps-no-spec-yet).

---

## 4. Storage — LVM Layout

Volume group: `vg0` (1.86 TiB total)

| Logical Volume | Size | Mount Point | Filesystem | Purpose |
|----------------|------|-------------|------------|---------|
| lv-root | 80 GB | `/` | ext4 | OS, packages |
| lv-var | 200 GB | `/var` | ext4 | Logs, apt cache |
| lv-docker | 600 GB | `/var/lib/docker` | ext4 | Docker images, containers, volumes |
| lv-home | 300 GB | `/home` | ext4 | User home, git repos, workspaces |
| lv-ollama | 300 GB | `/opt/ollama` | ext4 | Ollama models (host-level Ollama; Smackerel uses its own containerized Ollama by default) |
| lv-swap | 16 GB | swap | swap | Swap (LVM partition, not file) |
| **Free** | **410 GB** | — | — | Unallocated, available for `lvextend` |

All `fstab` entries use `/dev/disk/by-id/dm-uuid-LVM-...`.

### Current utilization (April 2026)
| Mount | Used | Avail |
|-------|------|-------|
| `/` | 12 GB | 64 GB |
| `/var` | 7.1 GB | 179 GB |
| `/var/lib/docker` | 1.5 MB | 560 GB |
| `/home` | 325 MB | 279 GB |
| `/opt/ollama` | 4.6 GB | 275 GB |

### Backup mount points (one per project, on `lv-home`)
| Path | Owner | Purpose |
|------|-------|---------|
| `/home/homelab/backups/qf/` | qf | QF Postgres/timescale + config |
| `/home/homelab/backups/smackerel/` | s | Postgres + NATS JetStream snapshots + connector OAuth + ingest data |
| `/home/homelab/backups/guesthost/` | gh | Postgres + uploads + seed |
| `/home/homelab/backups/wanderaide/` | wa | Postgres + Infisical-resolved env snapshots |

Off-host copy is **not** configured yet — see [§9](#9-cross-cutting-gaps-no-spec-yet) and the Storage Expansion Plan.

---

## 5. Existing Host Services (already running)

These are platform-level services. Project containers must not reuse their ports or container names.

### Docker Engine
| Property | Value |
|----------|-------|
| Version | 29.4.0 |
| Compose | v5.1.3 |
| Data root | `/var/lib/docker` (lv-docker, 600 GB) |
| Log rotation | `max-size: 10m`, `max-file: 3` (`/etc/docker/daemon.json`) |
| Weekly prune | Sunday 3am (`/etc/cron.d/docker-prune`) — images/containers > 7 days |
| User groups | `homelab` in `docker`, `render`, `video` |

### Ollama (host-level)
| Property | Value |
|----------|-------|
| Version | 0.21.0 |
| GPU | AMD ROCm (auto-detected Radeon 8060S) |
| Models directory | `/opt/ollama/models` |
| Listen address | `0.0.0.0:11434` |
| Systemd | `ollama.service` (enabled) |
| Models | `llama3.1:8b` |

> Smackerel ships its own containerized Ollama on a different host port. The host-level Ollama on `:11434` is shared and may be reused by any project for local LLM inference if a single shared model cache is desired.

### Caddy (reverse proxy)
| Property | Value |
|----------|-------|
| Version | 2.11.2 |
| Config | `/etc/caddy/Caddyfile` |
| Systemd | `caddy.service` (enabled) |

Current routes:
```
:80   { redir / /immich permanent }
:2284 { reverse_proxy localhost:2283 }
:11435{ reverse_proxy localhost:11434 }
```
**Will be extended** with project routes — see [§7 Reverse-proxy contract](#7-caddy-reverse-proxy-contract).

### Other infrastructure
| Service | Web UI / Port | Notes |
|---------|---------------|-------|
| Immich | `http://<host-tailnet-ip>:2283` | Library `/home/homelab/immich/library` |
| Uptime Kuma | `http://<host-tailnet-ip>:3001` | Will host all project monitors — see [§8](#8-uptime-kuma-monitor-catalog) |
| Portainer | `https://<host-tailnet-ip>:9443` | Endpoint = local Docker socket |
| Pi-hole / Unbound | Tailnet | Off-host |

### Reserved/used host ports (do not reassign)
| Port | Service |
|------|---------|
| 22 | SSH |
| 80 | Caddy redirect |
| 2283 | Immich Web |
| 2284 | Caddy → Immich proxy |
| 3001 | Uptime Kuma |
| 9443 | Portainer (HTTPS) |
| 11434 | Ollama API |
| 11435 | Caddy → Ollama proxy |

---

## 6. Project Port Allocation (NON-NEGOTIABLE)

Each project owns a 10k host port block. Per-project compose files must bind only inside their own block. This avoids collisions with each other and with the host services in §5.

| Project | Host port block | Docker bridge subnet (custom, to avoid Tailscale interference) | Compose project label |
|---------|-----------------|----------------------------------------------------------------|------------------------|
| QuantitativeFinance | `30000–39999` (already documented in repo CLI) | `172.30.0.0/16` | `quantitativefinance` |
| Smackerel | `40000–49999` | `172.31.0.0/16` | `smackerel` |
| GuestHost | `10000–19999` (already used by repo CLI) | `172.32.0.0/16` | `guesthost` |
| WanderAide | `20000–29999` (existing range) | `172.33.0.0/16` | `wanderaide` |

Rules:
- External/host URLs MUST use `127.0.0.1` (not `localhost`).
- Internal URLs MUST use service DNS names + container ports.
- Standard ports (`5432`, `6379`, `9092`, etc.) MUST NOT be mapped to the host directly. Each repo CLI is the source of truth for its mapping.

---

## 7. Caddy Reverse-Proxy Contract

The host's Caddy is the only public-tailnet entry point for HTTPS. Each project gets:
1. One MagicDNS hostname under `*.home-lab.<tailnet-domain>.ts.net` (Tailscale auto-issues TLS).
2. One Caddy route block referencing the project's host port.

Initial routes to add (planned, not yet applied):
```
qf.home-lab.<tailnet-domain>.ts.net {
    reverse_proxy 127.0.0.1:30050
}
smackerel.home-lab.<tailnet-domain>.ts.net {
    reverse_proxy 127.0.0.1:40080
}
gh.home-lab.<tailnet-domain>.ts.net {
    reverse_proxy 127.0.0.1:10050
}
wa.home-lab.<tailnet-domain>.ts.net {
    reverse_proxy 127.0.0.1:20050
}
```

> Exact ports are placeholder; each per-repo plan declares the canonical UI port. Caddy edits go through this Master plan (PR-reviewed) — no project edits `/etc/caddy/Caddyfile` directly.

---

## 8. Uptime Kuma Monitor Catalog

Today Uptime Kuma watches Immich, Ollama, Caddy, Portainer, Pi-hole, Unbound. Add the following per-project monitors as part of each project's first deploy:

| Project | Monitor | Type | Target |
|---------|---------|------|--------|
| qf | gateway health | HTTP | `http://host.docker.internal:30001/health` |
| qf | dashboard | HTTP | `http://host.docker.internal:30050/` |
| s | smackerel-core health | HTTP | `http://host.docker.internal:40081/health` |
| s | smackerel-ml health | HTTP | `http://host.docker.internal:40082/health` |
| s | postgres reachable | TCP | `host.docker.internal:40432` |
| s | nats reachable | TCP | `host.docker.internal:40222` |
| gh | backend health | HTTP | `http://host.docker.internal:10001/health` |
| gh | dashboard | HTTP | `http://host.docker.internal:10050/` |
| wa | gateway health | HTTP | `http://host.docker.internal:20001/health` |
| wa | admin portal | HTTP | `http://host.docker.internal:20050/` |

Each monitor needs the per-project Tailnet hostname listed in §7 added as a second probe once Caddy routes are live (catches TLS-cert and proxy issues).

---

## 9. Cross-Cutting Gaps (no spec yet)

These are required for the four-project rollout but are not owned by any single repo. Track as new specs in this Master plan; create the spec in the repo where the work executes.

| Gap | Owner repo for spec | Why blocking | Required before |
|-----|---------------------|--------------|-----------------|
| **Tailscale ↔ Docker bridge mitigation runbook** | `quantitativeFinance/specs/_ops/OPS-HOMELAB-002-tailscale-docker-bridge` | BUG-CHAOS-060-003 will recur on home-lab; Prometheus/Caddy scrapes must work cross-container | Any monitoring-dependent deploy |
| **Caddy reverse-proxy + TLS hostname contract** | `smackerel/specs/_ops/OPS-HOMELAB-003-caddy-tailnet-routes` | Need a single Caddyfile owner; today's Caddyfile only proxies Immich+Ollama | Any externally-reachable project |
| **Host firewall (ufw) baseline** | `smackerel/specs/_ops/OPS-HOMELAB-004-host-firewall` | ufw is inactive; only allow SSH + tailscale | Before public exposure |
| **Backup destination + off-host copy** | `smackerel/specs/_ops/OPS-HOMELAB-005-host-backups` | Local-only backups violate the DR principle | Before GH public preview |
| **Uptime Kuma monitor catalog spec** | `smackerel/specs/_ops/OPS-HOMELAB-006-monitoring-catalog` | Locks the monitors above into config-as-code | Friday afternoon |
| **Per-project secret bootstrap on home-lab** | each repo (project-specific) | No agreed mechanism for shipping `secrets.yaml` / Infisical bootstrap to home-lab | First deploy of each project |
| **Manual host hardening checklist** | this doc (§13) | Password change, BIOS VRAM, ufw, key copy | Before public exposure |

---

## 10. Deployment Schedule (Fri/Sat)

Today is Thu 2026-05-07. The schedule below assumes a half-day Friday and a full Saturday for soak/integration.

### Fri 2026-05-08 (morning) — host prep
1. Apply Tailscale ↔ Docker bridge mitigation (§3 step 1 first; verify).
2. Create `/home/homelab/backups/{qf,smackerel,guesthost,wanderaide}/`.
3. Add the 4 project Caddy hostnames placeholders (without port rules) so MagicDNS warms.
4. Confirm Docker prune + log rotation are functional with empty project state.

### Fri 2026-05-08 (afternoon) — first deploy: Smackerel
- Why first: closest to "near-final" (per repo plan), and it preserves connectors immediately so we never have to redo OAuth setup later.
- Per-project plan: [smackerel/docs/Home_Lab_Deployment_Plan.md](./Home_Lab_Deployment_Plan.md)
- Done = Smackerel containerized stack up, Tailnet route live, Uptime Kuma green, first connector authorized end-to-end.

### Fri evening / Sat AM 2026-05-09 — second deploy: GuestHost private preview
- Why second: needs backup posture proven before any real-data entry; the user softened the real-data requirement so we may seed sample data first.
- Per-project plan: [../../guestHost/docs/Home_Lab_Deployment_Plan.md](../../guestHost/docs/Home_Lab_Deployment_Plan.md)
- Done = Backend + dashboard reachable via Tailnet, backups running on schedule with restore tested, admin login works.

### Sat afternoon — third deploy: QuantitativeFinance (experimental sandbox only)
- Why third: bigger surface, no live trading needed for first deploy, integration with Smackerel comes later.
- Per-project plan: [../../quantitativeFinance/docs/Home_Lab_Deployment_Plan.md](../../quantitativeFinance/docs/Home_Lab_Deployment_Plan.md)
- Done = Gateway + dashboard reachable, simulation paths work, no live order flow enabled.

### Sat evening — fourth deploy: WanderAide (experimental sandbox only)
- Why fourth: most in-progress security/auth work; deploy as internal-only and use it to validate the proto-only contract on a real host.
- Per-project plan: [../../wanderaide/docs/Home_Lab_Deployment_Plan.md](../../wanderaide/docs/Home_Lab_Deployment_Plan.md)
- Done = Backend services up on Tailnet only, admin portal reachable, no public exposure.

### Sun 2026-05-10 — soak day
- Watch Uptime Kuma 24h.
- Restore-test a Smackerel + GH backup.
- Iterate on per-project gap backlog.

---

## 11. Iteration Backlog (post-cutover)

Add features only after the Friday/Saturday deploy is green and stable. Each project's per-repo plan owns its own backlog. Cross-cutting items live here:

1. Off-host backup target (Phase 1 USB4 NVMe or HDD RAID — see §14).
2. Shared `secrets-bootstrap` runbook + script.
3. UPS + `nut` integration (graceful shutdown of Docker stacks on power loss).
4. Switch from MagicDNS-only TLS to a real domain + ACME if any project ever needs public access.

---

## 12. Auto-Start Services (systemd enabled)

| Service | Status |
|---------|--------|
| `ssh` | enabled |
| `ollama` | enabled |
| `docker` | enabled |
| `caddy` | enabled |
| `tailscaled` | enabled |
| `NetworkManager` | enabled |

Docker containers with `restart: unless-stopped` auto-start with Docker.

---

## 13. Manual Host Tasks (open)

These are real-world tasks that block hardening but are out of scope for any single repo CLI:

- [ ] Change `homelab` password (`passwd`).
- [ ] BIOS: reduce iGPU VRAM to 16 GB.
- [ ] `ssh-copy-id homelab@<host-tailnet-ip>` from macOS.
- [ ] Enable + configure ufw to allow only SSH + tailscale (`tailscale0`).
- [ ] Apply the Tailscale ↔ Docker bridge mitigation (§3).
- [ ] Phase 1 storage purchase: USB4 NVMe enclosure + 4 TB NVMe.
- [ ] Phase 2 storage purchase: 2-bay USB-C HDD enclosure + 2× 8 TB NAS HDD.
- [ ] UPS purchase (600–850 VA with USB monitoring) + `nut`.

---

## 14. Storage Expansion Plan (Future)

### Phase 1 — USB4 NVMe Enclosure (~$200–280)
| Item | Purpose |
|------|---------|
| USB4 NVMe enclosure (ACASIS or ADATA SE920) | 40 Gbps, PCIe 4.0 ×4 |
| 4 TB NVMe (WD Black SN770 or Crucial P3 Plus) | Fast external storage |

Mounted at `/mnt/fast/` with `nofail` in fstab.

| Path | Purpose |
|------|---------|
| `/mnt/fast/ollama-models` | LLM models (moved from lv-ollama) |
| `/mnt/fast/immich-ml` | Immich ML cache + thumbnails |
| `/mnt/fast/dev-docker` | Dev/build Docker volumes |
| `/mnt/fast/scratch` | Build artifacts, cargo target dirs |

After migration: remove `lv-ollama` (300 GB) → 710 GB free in vg0.

### Phase 2 — USB-C HDD RAID Enclosure (~$350–500)
| Item | Purpose |
|------|---------|
| 2-bay USB-C enclosure (Sabrent / TerraMaster D2-310) | RAID-1 mirror |
| 2× 8 TB Seagate IronWolf NAS HDD | 24/7 rated, redundant |

Mounted at `/mnt/media/` with `nofail` in fstab.

| Path | Purpose |
|------|---------|
| `/mnt/media/immich-library` | Photos & videos (bulk) |
| `/mnt/media/backups` | **Project DB backups (off internal SSD)** |
| `/mnt/media/archive` | Cold storage |

### Workload isolation strategy
| Workload | Storage | Docker isolation |
|----------|---------|------------------|
| Production project services | Internal NVMe (lv-docker) | Resource reservations (cpus, memory) |
| Dev/build | USB4 SSD (/mnt/fast/dev-docker) | Resource limits, lower blkio weight |
| Media bulk | HDD RAID (/mnt/media/) | Volume mount only |
| LLM models | USB4 SSD (/mnt/fast/ollama-models) | Loaded into RAM, disk idle after |

---

## 15. Maintenance

| Task | Schedule | Config |
|------|----------|--------|
| Security updates | Daily (auto) | `/etc/apt/apt.conf.d/20auto-upgrades` |
| Docker prune | Sun 3am | `/etc/cron.d/docker-prune` |
| Docker log rotation | Continuous | `/etc/docker/daemon.json` (10 MB × 3) |

---

## 16. Cross-References

- Smackerel repo plan: [Home_Lab_Deployment_Plan.md](./Home_Lab_Deployment_Plan.md)
- QuantitativeFinance repo plan: [../../quantitativeFinance/docs/Home_Lab_Deployment_Plan.md](../../quantitativeFinance/docs/Home_Lab_Deployment_Plan.md)
- GuestHost repo plan: [../../guestHost/docs/Home_Lab_Deployment_Plan.md](../../guestHost/docs/Home_Lab_Deployment_Plan.md)
- WanderAide repo plan: [../../wanderaide/docs/Home_Lab_Deployment_Plan.md](../../wanderaide/docs/Home_Lab_Deployment_Plan.md)
- QF↔Smackerel integration spec: [../../quantitativeFinance/specs/063-smackerel-companion-bridge/](../../quantitativeFinance/specs/063-smackerel-companion-bridge/)
- Smackerel↔QF connector spec: [../specs/041-qf-companion-connector/](../specs/041-qf-companion-connector/)
- Smackerel↔GH connector spec (done): [../specs/013-guesthost-connector/](../specs/013-guesthost-connector/)
