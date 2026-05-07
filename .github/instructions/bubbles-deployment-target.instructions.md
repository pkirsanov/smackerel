# Bubbles Deployment Target Adapter Instructions

> **Portability:** This file is **project-agnostic**. Copy unchanged across projects.
> Companion to [bubbles-config-sst.instructions.md](bubbles-config-sst.instructions.md) and [bubbles-docker-lifecycle-governance.instructions.md](bubbles-docker-lifecycle-governance.instructions.md).

Use this instruction when creating, modifying, or reviewing anything that runs the project on a real machine, cluster, cloud, or home lab — i.e. anything under `deploy/`, anything that mutates host state (Caddy, Docker daemon, ufw, systemd, mDNS, Tailscale, hostnames, DNS), or anything in `docs/Deployment*.md`.

## Core Principle: Targets Are Adapters, Not The Project

The project is target-agnostic. A deployment target (`home-lab`, `aws`, `fly`, `gcp`, `local-dev`, `staging-vps`) is an **adapter** that:

1. **Consumes a generated, target-agnostic contract** derived from the project's SST.
2. **Owns ALL target-specific knowledge** (host IPs, FQDNs, Tailscale names, Caddy site files, ufw rules, systemd units, cloud-account IDs, region settings).
3. **Is fully idempotent** — re-running the same adapter against the same target MUST converge without manual intervention.
4. **Cleans up after itself** — every adapter ships a teardown that removes ONLY what it created.

Multiple adapters MUST be able to coexist for the same project (e.g., a service running on `home-lab` AND in a `staging-vps`). No adapter may assume it is the only deployment.

## Required Layout

Every project that has a deployment story MUST use this layout:

```
deploy/
├── contract.yaml                 ← Generated from SST; target-agnostic; DO NOT EDIT
├── README.md                     ← Index of adapters + how to add a new one
└── <target>/                     ← One directory per deployment target
    ├── params.yaml               ← Target-specific values (FQDNs, IPs, TLS dirs, etc.)
    ├── preconditions.sh          ← Verifies target is ready (no-op if all good)
    ├── bootstrap.sh              ← Idempotent install/upgrade
    ├── teardown.sh               ← Removes ONLY what bootstrap created
    ├── verify.sh                 ← Post-deploy health/smoke checks
    └── README.md                 ← How to use this target (operator-facing)
```

Where `<target>` is a kebab-case, stable identifier (e.g., `home-lab`, `aws-prod`, `fly-staging`, `local-dev`).

## Required Behavior

- The project's CLI (e.g., `./<project>.sh`) MUST expose a single deployment surface: `deploy <target> <action>`. Actions: `preconditions`, `bootstrap`, `apply`, `rollback`, `verify`, `teardown`, `status`, `manifest`, `params`.
- `deploy/contract.yaml` MUST be regenerated from SST at the start of any deploy action. Drift between contract and SST is a blocking error.
- Adapters MUST read `deploy/contract.yaml` for what to deploy and `deploy/<target>/params.yaml` for where/how to deploy it.
- The `apply` action MUST consume an immutable image digest and a CI-published config bundle hash. No build, no tag-resolution, no fallback.
- The `rollback` action MUST be a pure pointer-swap on `deploy/<target>/manifest.yaml` — no rebuild.
- Bootstrap MUST be idempotent: re-running with no input changes MUST produce zero diffs and exit 0.
- Bootstrap MUST be non-destructive to other adapters' state on the same host (use drop-in directories, namespaced rules, project-prefixed identifiers).
- Teardown MUST be precise: it removes only resources tagged with this adapter's namespace. It MUST NOT touch host singletons that other adapters or the operator depend on.
- Every adapter MUST declare its host singletons usage policy (see "Host Singletons" below).
- Every adapter MUST declare its rollout strategy (`recreate`, `blue-green`, `canary`, `flag-gated`) in `params.yaml`.

## Build-Once Deploy-Many (NON-NEGOTIABLE)

The same git SHA MUST produce one immutable application image. The same image digest is then deployed to every target by pairing it with the matching environment's CI-published config bundle.

**CI workflow MUST:**
- Build a content-addressed image (`sha256:<digest>`) and publish to a registry
- Sign with cosign keyless (Sigstore + Rekor transparency log)
- Attach SBOM attestation (syft or equivalent)
- Attach SLSA provenance attestation
- Generate one config bundle per target environment via the SST pipeline (e.g., `./<project>.sh config generate --env <env> --bundle`) and publish each bundle as an immutable artifact identified by `<env>-<sourceSha>`
- Publish a `build-manifest-<sourceSha>.yaml` listing image digest(s), bundle hashes, and attestation refs
- Stop after publishing — CI MUST NOT SSH to a target, MUST NOT run `apply`, MUST NOT mutate any host state

**Adapter `apply` MUST:**
- Pull image by digest only (no tag resolution at deploy time)
- Verify cosign signature against Rekor before container start
- Verify SBOM and provenance attestations exist
- Pull the config bundle by hash and verify the hash
- Write `deploy/<target>/manifest.yaml` with the new pointer pair before starting any container
- Run the rollout strategy declared in `params.yaml`
- On verify failure: invoke `rollback` (pointer-swap to `previousManifest`)

## Do Not Do

- Put target-specific values (FQDNs, host IPs, Tailscale names, cloud account IDs, region codes) in the project's SST file or any non-adapter location.
- Modify host singletons (e.g., `/etc/caddy/Caddyfile`, `/etc/docker/daemon.json`, monolithic ufw rule sets) by overwriting them. Use drop-ins only.
- Hand-edit `deploy/contract.yaml`. It is generated.
- Reuse the same volume names, container names, network names, or systemd unit names across adapters without a target-specific suffix or namespace.
- Embed deployment-target content into the project's main `docs/Deployment.md`. Main deployment doc explains the contract; per-target docs live under `deploy/<target>/README.md`.
- Create a "primary" target whose presence is required for other targets to work. All adapters must be peers.
- **Use mutable image tags (`:latest`, `:staging-latest`, `:prod-latest`) in any deployment manifest, Compose file, or runtime spec.** Tags are informational; only digests are deployable.
- **Run a build step inside the deploy adapter.** `apply.sh` MUST NOT invoke `docker build`, `docker buildx build`, `cargo build`, `npm run build`, or any compile/bundle step. The build is CI's job.
- **Generate config bundles at deploy time on the target host.** The bundle is a CI artifact consumed by `apply`.
- **Allow CI to deploy.** CI ends at registry push. Deploy is downstream of CI on a different trust boundary.
- **Permit `apply` to fall back to building locally if registry pull fails.** Fail-fast.
- **Rebuild on rollback.** `rollback` is a pointer-swap; no rebuild allowed.

## Host Singletons Policy

Some host resources can only have one canonical configuration (Caddy main config, Docker daemon config, ufw default policy, systemd service of a given name, listening sockets on a port). Adapters MUST handle these via the **drop-in / namespace / assert** pattern:

| Singleton | Forbidden | Required |
|-----------|-----------|----------|
| Caddy main `/etc/caddy/Caddyfile` | Overwriting it | `import conf.d/*.caddy` once, then drop a `conf.d/<project>-<target>.caddy` file |
| Docker `/etc/docker/daemon.json` | Replacing it | Read, deep-merge required keys, write back; assert merge is a no-op on re-run |
| ufw rules | Wiping ruleset | Add tagged comments (`# project=<name> target=<target>`), remove only own tagged rules |
| systemd units | Generic names like `worker.service` | Namespaced names like `<project>-<target>-worker.service` |
| Listening ports | Hardcoded in adapter | Allocated from project's port block (per `bubbles-docker-port-standards`), declared in SST, exposed in `deploy/contract.yaml` |
| Hostnames / mDNS / Tailscale tags | Adapter assigning host-wide identity | Adapter consumes operator-provided host identity via `params.yaml` |

The bootstrap script MUST assert these singletons after writing drop-ins (e.g., `caddy validate`, `docker info`, `ufw status verbose | grep`) and fail loudly on conflict.

## Idempotency Requirements (BLOCKING)

Every adapter MUST satisfy:

1. Running `bootstrap.sh` on a fresh target produces a healthy deployment.
2. Running `bootstrap.sh` again immediately produces zero state changes (no file rewrites, no container recreates, no volume churn).
3. Running `teardown.sh` removes everything `bootstrap.sh` created and leaves the host in the state it was in before bootstrap (drop-ins gone, ufw rules removed, namespaced systemd units disabled and removed, project containers/volumes/networks gone — host singletons left intact).
4. Running `bootstrap.sh` after `teardown.sh` succeeds with no manual intervention.
5. Two adapters for different targets on the same host (e.g., `home-lab` + `staging-vps`) coexist without interfering.

## Verification Commands

```bash
# Verify contract is fresh (no SST drift)
./<project>.sh deploy <target> preconditions

# Verify bootstrap is idempotent (run twice, second run must be a no-op)
./<project>.sh deploy <target> bootstrap
./<project>.sh deploy <target> bootstrap   # MUST converge with zero diffs

# Verify teardown is precise (must not touch host singletons)
./<project>.sh deploy <target> teardown
# Manual: confirm Caddyfile, daemon.json, ufw default policy, hostname unchanged

# Verify health
./<project>.sh deploy <target> verify
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| `home-lab`-specific IP in project SST | Target leakage into project | Move to `deploy/home-lab/params.yaml` |
| Adapter rewrites `/etc/caddy/Caddyfile` | Destroys other adapters' sites | Drop file in `conf.d/`, assert main file imports it |
| Bootstrap creates container `app` (no namespace) | Collides with peer adapter | Use `${PROJECT}-${TARGET}-app` |
| Teardown runs `docker system prune -a` | Destroys peer adapters' images | Remove only this adapter's labelled resources |
| `docs/Deployment.md` describes one specific target step-by-step | New targets force doc rewrite | Main doc describes contract; targets live in `deploy/<target>/README.md` |
| `bootstrap.sh` requires manual prompts | Not idempotent in CI | All inputs from `params.yaml` and contract |
| Adapter assumes only one deployment per host | Blocks multi-target operation | Namespace every host resource |
| **CI workflow runs `./<project>.sh deploy`** | Fuses build with deploy; loses build-once invariant | CI publishes artifacts only; deploy is downstream |
| **Compose / runtime spec uses `image: name:latest`** | Mutable; breaks digest-pinning | `image: name@sha256:<digest>` |
| **`apply.sh` runs `docker build`** | Build is CI's job | `apply.sh` only pulls + verifies + runs |
| **`rollback` rebuilds from prior SHA** | Slow; risks divergence | Pointer-swap to `previousManifest`; no rebuild |
| **`apply` falls back to local build on registry failure** | Defeats build-once invariant | Fail-fast with clear error |
| **CI has SSH credentials to a target host** | Trust boundary violation | CI ends at registry push |
| **Image tag promotion (re-tagging staging → prod)** | Tag-swap loses traceability | Pin by digest in manifest; tags informational only |

## Spec / Plan Authoring Rule

Any feature spec that has a deployment side-effect (a new service, a new background worker, a new public surface, a new persistent store) MUST declare:

1. Which **contract entries** in `deploy/contract.yaml` change.
2. Which **target adapters** need updates and what kind (params change, bootstrap step added, new precondition, etc.).
3. Whether the change requires a **new host singleton** assertion.
4. Whether the change introduces a **new image** that needs build-pipeline coverage (CI build job, signature, SBOM, provenance).
5. Whether the change adds a **new config bundle environment** that the build pipeline must emit.

Specs that introduce target-specific behavior outside `deploy/<target>/` MUST be rejected.
Specs that introduce a new image without updating the build pipeline MUST be rejected.

## References

- [bubbles-config-sst.instructions.md](bubbles-config-sst.instructions.md)
- [bubbles-docker-lifecycle-governance.instructions.md](bubbles-docker-lifecycle-governance.instructions.md)
- [bubbles-docker-port-standards](../skills/bubbles-docker-port-standards/SKILL.md)
- [deployment-target-adapter skill](../skills/bubbles-deployment-target-adapter/SKILL.md)
