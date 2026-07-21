# Bubbles Deployment Target Adapter Instructions

> **Portability:** This file is **project-agnostic**. Copy unchanged across projects.
> Companion to [bubbles-config-sst.instructions.md](bubbles-config-sst.instructions.md) and [bubbles-docker-lifecycle-governance.instructions.md](bubbles-docker-lifecycle-governance.instructions.md).

Use this instruction when creating, modifying, or reviewing anything that runs the project on a real machine, cluster, cloud, or home lab — i.e. anything under `deploy/`, anything that mutates host state (reverse proxies, container runtimes, host firewalls, init systems, mDNS, mesh-VPN identity, hostnames, DNS), or anything in `docs/Deployment*.md`.

## Core Principle: Targets Are Adapters, Not The Project

The project is target-agnostic. A deployment target (`home-lab`, `aws`, `fly`, `gcp`, `local-dev`, `staging-vps`) is an **adapter** that:

1. **Consumes a generated, target-agnostic contract** derived from the project's SST.
2. **Owns ALL target-specific knowledge** (host IPs, FQDNs, mesh-VPN identity, reverse-proxy site files, host-firewall rules, init-system units, cloud-account IDs, region settings).
3. **Is fully idempotent** — re-running the same adapter against the same target MUST converge without manual intervention.
4. **Cleans up after itself** — every adapter ships a teardown that removes ONLY what it created.

Multiple adapters MUST be able to coexist for the same project (e.g., a service running on `home-lab` AND in a `staging-vps`). No adapter may assume it is the only deployment.

## Required Layout

Every project that has a deployment story MUST expose this adapter shape per target. The directory MAY live **in-tree** under the project repo (default for OSS adopters and CI-deployed targets) **OR out-of-tree** under an operator-private location (required when the target leaks operator identity into a public repo — see "Adapter Locality" below).

```
<adapter-root>/
├── contract.yaml                 ← Generated from SST; target-agnostic; DO NOT EDIT (always in-tree at deploy/)
├── README.md                     ← Index of adapters + how to add a new one
└── <target>/                     ← One directory per deployment target
    ├── params.yaml               ← Target-specific values (FQDNs, IPs, TLS dirs, etc.)
    ├── manifest.yaml             ← Mutable deployment pointer (image digest + bundle hash); written by `apply`
    ├── preconditions.sh          ← Verifies target is ready (no-op if all good)
    ├── bootstrap.sh              ← Idempotent install/upgrade
    ├── apply.sh                  ← Pull image by digest, mount bundle, swap manifest pointer
    ├── rollback.sh               ← Pointer-swap to previousManifest (no rebuild)
    ├── teardown.sh               ← Removes ONLY what bootstrap/apply created
    ├── verify.sh                 ← Post-deploy health/smoke checks
    └── README.md                 ← How to use this target (operator-facing)
```

Where `<target>` is a kebab-case, stable identifier (e.g., `home-lab`, `aws-prod`, `fly-staging`, `local-dev`).

## Adapter Locality (Public Repos & Operator Privacy)

Per-target adapters carry **operator-coupled topology** (FQDNs, mesh-VPN identity, reverse-proxy paths, host singletons, cross-project coexistence notes). When a project repo is or will be **public**, that topology MUST NOT live inside the public repo, even with placeholder values, because:

1. Placeholders silently collect real values via copy-paste regressions and stay in git history forever.
2. The mere presence of a `deploy/<my-host>/` directory in a public repo discloses topology-pattern reconnaissance (which proxy, which VPN, which port block) and cross-correlates the operator's other public repos.
3. A generic OSS adopter cannot deploy to the maintainer's specific target anyway — they will author their own.

### Two Locality Modes (BOTH MUST be supported by the CLI)

| Mode | Adapter Root | When To Use |
|------|-------------|-------------|
| **In-tree** (default) | `<repo>/deploy/<target>/` | Generic / shareable targets (e.g., `local-dev`, `_example` skeleton, `fly-staging` template). Safe for public repos. |
| **Out-of-tree** (operator-private) | `${DEPLOY_TARGETS_ROOT}/<project>/<target>/` | Operator-owned targets that name a real host, real VPN, real reverse proxy, or expose cross-project workspace topology. REQUIRED for any operator-owned target if the project repo is or will be public. |

### CLI Resolution Rule (REQUIRED — STRICT, no silent fallback)

The project CLI MUST resolve a target's adapter directory by branching on whether `DEPLOY_TARGETS_ROOT` is set:

1. **If `DEPLOY_TARGETS_ROOT` is set** (operator opted into out-of-tree adapters):
   - If `${DEPLOY_TARGETS_ROOT}/<project>/<target>/params.yaml` exists → use that directory.
   - Else → fail with a clear error listing the attempted out-of-tree path AND the in-tree path the CLI deliberately did NOT consult, plus a hint to either populate the out-of-tree path or unset `DEPLOY_TARGETS_ROOT`.
2. **Else** (`DEPLOY_TARGETS_ROOT` unset — in-tree-only mode):
   - If `<repo>/deploy/<target>/params.yaml` exists → use it.
   - Else → fail with a clear error listing the attempted in-tree path AND a hint to create `<repo>/deploy/_example/<target-skeleton>/` as a starting point or set `DEPLOY_TARGETS_ROOT`.

The CLI MUST NEVER silently fall back from out-of-tree to in-tree when `DEPLOY_TARGETS_ROOT` is set. Setting `DEPLOY_TARGETS_ROOT` is an explicit operator opt-in: "all my adapters live out-of-tree now". Falling back to an in-tree leftover after the operator believed they had migrated risks deploying stale state and disclosing previously-private topology that the operator thought was scrubbed.

### Public-Repo Safety Checklist

Before publishing a project repo (or merging a change that turns a private repo public), verify:

- `<repo>/deploy/<target>/` contains ONLY generic targets (`_example`, `local-dev`, vendor templates) OR is empty except for `contract.yaml` + `README.md`.
- No file under `<repo>/deploy/` references: a real LAN/VPN IP, a real mesh-VPN node FQDN, a real reverse-proxy site filename that names this operator's host, a real init-system unit pattern that names this operator's host, or other workspace projects' names by short-name.
- `<repo>/deploy/README.md` documents the in-tree vs out-of-tree resolution rule and points operators at `DEPLOY_TARGETS_ROOT`.
- Git history is audited for previously-committed operator-coupled values (use `git log -p -- deploy/`); any sensitive history MUST be scrubbed before the repo goes public.

## Required Behavior

- The project's CLI (e.g., `./<project>.sh`) MUST expose a single deployment surface: `deploy <target> <action>`. Actions: `preconditions`, `bootstrap`, `apply`, `rollback`, `verify`, `teardown`, `status`, `manifest`, `params`.
- The CLI MUST resolve the adapter directory using the rule in "Adapter Locality" above (out-of-tree first if `DEPLOY_TARGETS_ROOT` is set; in-tree fallback only when `DEPLOY_TARGETS_ROOT` is unset).
- `deploy/contract.yaml` MUST be regenerated from SST at the start of any deploy action. Drift between contract and SST is a blocking error.
- Adapters MUST read `deploy/contract.yaml` for what to deploy and the resolved adapter directory's `params.yaml` for where/how to deploy it.
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
- **Attest the achieved assurance level** (`certification.assurance.level` ∈ `full`|`fast`|`prototype`) plus the certifying evidence digest into `build-manifest-<sourceSha>.yaml` and the per-target `manifest.yaml` `attestations.assurance` block (IMP-100 R5 choke #4)
- Stop after publishing — CI MUST NOT SSH to a target, MUST NOT run `apply`, MUST NOT mutate any host state

**Adapter `apply` MUST:**
- Pull image by digest only (no tag resolution at deploy time)
- Verify cosign signature against Rekor before container start
- Verify SBOM and provenance attestations exist
- **Verify the assurance attestation BEFORE starting any container** (IMP-100 R5 choke #5) via `bubbles/scripts/deploy-manifest-assurance-lint.sh --manifest deploy/<target>/manifest.yaml --minimum-assurance <target-floor> [--risk-class <class>]`: ALWAYS refuse `prototype` (never deployable at any target); refuse an under-assured level for the target floor (the lint consumes the shared decision in `assurance-resolve.sh`). A manifest with no `attestations.assurance` block is a backward-compatible no-op. The generic decision is framework-owned; the concrete floor + wiring are adapter-owned.
- Pull the config bundle by hash and verify the hash
- Write `deploy/<target>/manifest.yaml` with the new pointer pair before starting any container
- Run the rollout strategy declared in `params.yaml`
- On verify failure: invoke `rollback` (pointer-swap to `previousManifest`)

## Do Not Do

- Put target-specific values (FQDNs, host IPs, mesh-VPN identity, cloud account IDs, region codes) in the project's SST file or any non-adapter location.
- Modify host singletons (e.g., reverse-proxy main config, container-runtime daemon config, monolithic host-firewall rule sets) by overwriting them. Use drop-ins only.
- Hand-edit `deploy/contract.yaml`. It is generated.
- Reuse the same volume names, container names, network names, or init-system unit names across adapters without a target-specific suffix or namespace.
- Embed deployment-target content into the project's main `docs/Deployment.md`. Main deployment doc explains the contract; per-target docs live under `deploy/<target>/README.md`.
- Create a "primary" target whose presence is required for other targets to work. All adapters must be peers.
- **Use mutable image tags (`:latest`, `:staging-latest`, `:prod-latest`) in any deployment manifest, Compose file, or runtime spec.** Tags are informational; only digests are deployable.
- **Run a build step inside the deploy adapter.** `apply.sh` MUST NOT invoke `docker build`, `docker buildx build`, `cargo build`, `npm run build`, or any compile/bundle step. The build is CI's job.
- **Generate config bundles at deploy time on the target host.** The bundle is a CI artifact consumed by `apply`.
- **Allow CI to deploy.** CI ends at registry push. Deploy is downstream of CI on a different trust boundary.
- **Permit `apply` to fall back to building locally if registry pull fails.** Fail-fast.
- **Rebuild on rollback.** `rollback` is a pointer-swap; no rebuild allowed.
- **Commit operator-coupled adapter content to a public (or to-be-public) repo.** Operator-owned home-lab / personal-VPS / private-cloud adapters MUST live out-of-tree under `${DEPLOY_TARGETS_ROOT}/<project>/<target>/`. The in-tree `deploy/<target>/` directory is reserved for generic / shareable targets (`_example`, `local-dev`, vendor templates). See "Adapter Locality" above.
- **Mix multiple operators' adapters under one in-tree `deploy/<target>/`.** Each operator owns their own out-of-tree adapter root; the project repo never names a specific operator's deployment.

## Host Singletons Policy

Some host resources can only have one canonical configuration (reverse-proxy main config, container-runtime daemon config, host-firewall default policy, init-system service of a given name, listening sockets on a port). Adapters MUST handle these via the **drop-in / namespace / assert** pattern:

| Singleton | Forbidden | Required |
|-----------|-----------|----------|
| Reverse-proxy main config | Overwriting it | Main config imports a drop-in directory once; adapter drops a per-adapter snippet (`<project>-<target>.<ext>`) into that directory |
| Container-runtime daemon config | Replacing it | Read, deep-merge required keys, write back; assert merge is a no-op on re-run |
| Host-firewall rules | Wiping ruleset | Add tagged comments (`# project=<name> target=<target>`), remove only own tagged rules |
| Init-system units | Generic names like `worker.service` | Namespaced names like `<project>-<target>-worker.service` |
| Listening ports | Hardcoded in adapter | Allocated from project's port block (per `bubbles-docker-port-standards`), declared in SST, exposed in `deploy/contract.yaml` |
| Hostnames / mDNS / mesh-VPN identity | Adapter assigning host-wide identity | Adapter consumes operator-provided host identity via `params.yaml` |

The bootstrap script MUST assert these singletons after writing drop-ins (e.g., reverse-proxy validate, container-runtime info, firewall status) and fail loudly on conflict.

### TLS Issuance Contract

Every adapter that publishes an HTTPS surface MUST declare ONE TLS issuance strategy in its `README.md` and implement it consistently in `bootstrap.sh` (issuance) and `teardown.sh` (cleanup). The framework is agnostic to which strategy is chosen — public-CA ACME, private-CA ACME, mesh-VPN-issued certs, operator-supplied PEM files, or any other mechanism are all acceptable as long as the strategy is documented, idempotent, and bounded to the adapter's own scope. The framework's only requirements are:

- The chosen issuance mechanism MUST be reachable from the target's network position (e.g., a public ACME challenge cannot complete against a host that is not publicly reachable).
- If the chosen mechanism is non-renewing, `bootstrap.sh` MUST install a renewal job (cron, systemd timer, or equivalent) and a reload trigger for any reverse proxy that consumes the issued certs.
- `teardown.sh` MUST remove only this adapter's cert files and renewal job; shared cert directories owned by peer adapters MUST NOT be deleted.
- Concrete TLS-strategy patterns (e.g., a private-network deployment using mesh-issued certs and a host reverse proxy) belong in a project-specific or overlay-specific skill, NOT in this framework instruction.

## Idempotency Requirements (BLOCKING)

Every adapter MUST satisfy:

1. Running `bootstrap.sh` on a fresh target produces a healthy deployment.
2. Running `bootstrap.sh` again immediately produces zero state changes (no file rewrites, no container recreates, no volume churn).
3. Running `teardown.sh` removes everything `bootstrap.sh` created and leaves the host in the state it was in before bootstrap (drop-ins gone, host-firewall rules removed, namespaced init-system units disabled and removed, project containers/volumes/networks gone — host singletons left intact).
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
# Manual: confirm reverse-proxy main config, container-runtime daemon config, host-firewall default policy, and hostname are unchanged

# Verify health
./<project>.sh deploy <target> verify
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| `home-lab`-specific IP in project SST | Target leakage into project | Move to `deploy/home-lab/params.yaml` |
| Adapter rewrites the reverse-proxy main config | Destroys other adapters' sites | Drop file in the reverse-proxy drop-in directory, assert main config imports it |
| Bootstrap creates container `app` (no namespace) | Collides with peer adapter | Use `${PROJECT}-${TARGET}-app` |
| Teardown runs container-runtime system-wide prune | Destroys peer adapters' images | Remove only this adapter's labelled resources |
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
| **Operator-coupled adapter (real FQDN, real VPN IP, real reverse-proxy site, cross-project workspace notes) committed to a public repo** | Topology + identity reconnaissance; placeholders silently collect real values via copy-paste regressions; cross-correlates operator's other public repos | Move adapter to `${DEPLOY_TARGETS_ROOT}/<project>/<target>/`; in-tree `deploy/<target>/` reserved for generic / shareable targets only |
| **CLI silently falls back from out-of-tree to in-tree adapter when `DEPLOY_TARGETS_ROOT` is set but target is missing** | Risks deploying a stale in-tree leftover after operator thought they had migrated | Fail-fast with clear error listing both attempted paths |

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
