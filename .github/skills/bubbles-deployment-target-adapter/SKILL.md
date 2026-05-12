---
name: bubbles-deployment-target-adapter
description: Enforce per-target deployment adapters that consume an SST-derived contract and own ALL target-specific knowledge. Use when adding or changing how a project is deployed to a real machine, cluster, cloud, or home lab; when authoring `deploy/<target>/` scripts; when reviewing changes that touch host singletons (Caddy, Docker daemon, ufw, systemd, Tailscale, mDNS); when designing CLI surfaces for `deploy <target> <action>`. Triggers include new deployment target proposals, home-lab / cloud / staging deployment work, host-config changes, idempotency reviews, and `docs/Deployment*.md` edits.
---

# Bubbles Deployment Target Adapter

## Goal

Make a project deployable to **multiple, simultaneous targets** (home lab, cloud, staging VPS, dev laptop) without leaking target-specific values into the project's SST or main docs, and without any one target's bootstrap destroying another target's state on a shared host.

The pattern is: **SST → generated contract → per-target adapter**. The contract describes WHAT to deploy (services, ports, volumes, env keys). The adapter describes WHERE and HOW (FQDNs, IPs, TLS dirs, ufw rules, systemd unit names, host-singleton drop-ins).

## Use This Skill When

- Adding a new deployment target (home-lab, AWS, Fly.io, GCP, staging VPS, on-prem K8s).
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
<adapter-root>/                ← Resolved by CLI: out-of-tree (DEPLOY_TARGETS_ROOT) OR in-tree (./deploy/)
├── README.md                  ← Index of adapters + contract overview
├── contract.yaml              ← Generated from SST; ALWAYS in-tree at <repo>/deploy/contract.yaml; DO NOT EDIT
└── <target>/                  ← Per-target adapter directory
    ├── params.yaml            ← Target-specific values
    ├── manifest.yaml          ← Current deployment pointer (image digest, bundle hash); written by `apply`
    ├── preconditions.sh       ← Verifies target is ready (idempotent, exit 0 if good)
    ├── bootstrap.sh           ← Idempotent install/upgrade (re-run = no-op)
    ├── apply.sh               ← Idempotent: pull image digest, mount bundle, swap manifest pointer
    ├── rollback.sh            ← Restore prior manifest pointer (no rebuild)
    ├── verify.sh              ← Post-deploy health/smoke
    ├── teardown.sh            ← Removes ONLY what bootstrap/apply created
    └── README.md              ← Operator-facing
```

`manifest.yaml` is **mutable and target-local** (records the current image digest + config bundle hash deployed to that target). It is the only adapter file the `apply` action edits. `params.yaml` is target-stable.

`contract.yaml` is **always in-tree** under the project repo's `deploy/contract.yaml` (it is generated from SST and is target-agnostic). Per-target adapter directories MAY live in-tree or out-of-tree per the rule below.

## Adapter Locality (Public Repos & Operator Privacy)

Per-target adapters carry **operator-coupled topology** — real FQDNs, real VPN identity, real reverse-proxy paths, host singletons, cross-project coexistence notes. When a project repo is or will be **public**, that topology MUST NOT live inside the public repo, even with placeholder values, because:

1. Placeholders silently collect real values via copy-paste regressions and stay in git history forever.
2. The mere presence of a `deploy/<my-host>/` directory in a public repo discloses topology-pattern reconnaissance (which proxy, which VPN, which port block).
3. Cross-project coexistence notes (e.g., "this host also runs project X, project Y, project Z") cross-correlate the operator's other public repos.
4. A generic OSS adopter cannot deploy to the maintainer's specific target anyway — they will author their own.

### Two Locality Modes

| Mode | Adapter Root | Purpose |
|------|-------------|---------|
| **In-tree** (default) | `<repo>/deploy/<target>/` | Generic / shareable targets: `_example` skeleton, `local-dev`, vendor templates (`fly-staging.template`, `aws-skeleton`). Safe for public repos. |
| **Out-of-tree** (operator-private) | `${DEPLOY_TARGETS_ROOT}/<project>/<target>/` | Operator-owned targets that name a real host, real VPN, real reverse proxy, or expose cross-project workspace topology. REQUIRED when the project repo is or will be public. |

A single operator may keep all of their projects' out-of-tree adapters under one private repo:

```
~/operator-targets/                       ← private git repo, sole operator
├── <project-A>/home-lab/...
├── <project-B>/home-lab/...
├── <project-C>/home-lab/...
└── shared/                               ← cross-project host-singleton coordination
    ├── caddy-port-allocations.md         (which project owns which 4xxxx port)
    ├── ufw-tag-registry.md               (which `# project=...` tags exist)
    └── systemd-unit-namespaces.md        (which `<project>-<target>-<purpose>` units exist)
```

### CLI Resolution Rule (STRICT — no silent fallback)

The project CLI MUST resolve `<adapter-root>/<target>/` by branching on whether `DEPLOY_TARGETS_ROOT` is set. Setting `DEPLOY_TARGETS_ROOT` is an explicit operator opt-in: "all my adapters live out-of-tree now." The CLI MUST honor that opt-in strictly.

1. **If `DEPLOY_TARGETS_ROOT` is set:**
   - If `${DEPLOY_TARGETS_ROOT}/<project>/<target>/params.yaml` exists → use that directory.
   - Else → FAIL with a clear error listing the attempted out-of-tree path AND the in-tree path the CLI deliberately did NOT consult, plus a hint to either populate the out-of-tree path or unset `DEPLOY_TARGETS_ROOT`.
2. **Else (`DEPLOY_TARGETS_ROOT` unset — in-tree-only mode):**
   - If `<repo>/deploy/<target>/params.yaml` exists → use it.
   - Else → FAIL with a clear error listing the attempted in-tree path AND a hint to copy `<repo>/deploy/_example/<target-skeleton>/` as a starting point or set `DEPLOY_TARGETS_ROOT`.

**The CLI MUST NEVER silently fall back from out-of-tree to in-tree.** Falling back to an in-tree leftover after the operator believed they had migrated risks deploying stale state and disclosing previously-private topology that the operator thought was scrubbed. Fail fast.

### What Each Repo Should Contain

| Repo | What MUST exist | What MUST NOT exist (if public) |
|------|----------------|---------------------------------|
| Public product repo | `deploy/contract.yaml`, `deploy/README.md`, `deploy/_example/<target-skeleton>/` (generic), optional in-tree `deploy/local-dev/` (safe), optional vendor templates | Operator-coupled `deploy/home-lab/`, `deploy/<my-host>/`, anything naming a real FQDN/VPN/host |
| Operator-private adapter repo | One subdirectory per project the operator deploys, each containing the full adapter (`params.yaml` + scripts), plus a `shared/` directory for cross-project host-singleton coordination | n/a (private) |

### Public-Repo Safety Checklist

Before publishing a project repo (or merging a change that turns a private repo public):

```bash
# 1. No operator-coupled adapter in tree
test -z "$(find deploy -mindepth 2 -name params.yaml ! -path 'deploy/_example/*' ! -path 'deploy/local-dev/*')"

# 2. No real LAN/VPN IPs anywhere under deploy/
! grep -rE '100\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.|10\.[0-9]+\.[0-9]+\.[0-9]+' deploy/

# 3. No real tailnet names, real hostnames, real proxy site names
! grep -rE '[a-z0-9-]+\.ts\.net' deploy/
! grep -rE '<a-real-host>' deploy/   # replace pattern with operator's known host suffixes

# 4. README documents in-tree vs out-of-tree resolution
grep -q "DEPLOY_TARGETS_ROOT" deploy/README.md

# 5. Git history audited
git log --all -p -- deploy/ | grep -E '<known-operator-pattern>' || echo "OK: no historical leak"
```

If any check fails, the change is BLOCKED until the operator-coupled content is moved out-of-tree (and, if previously committed, the history is scrubbed before publication).

## CLI Surface

The project CLI MUST expose a single deployment surface:

```
./<project>.sh deploy <target> <action>

Actions:
  preconditions    Verify the target host is ready (no mutation)
  bootstrap        Idempotent install / upgrade (uses local build OR registry image)
  apply            Deploy a specific built artifact: --image=<digest> --config-bundle=<hash>
  rollback         Restore previous deployment manifest pointer (no rebuild)
  verify           Post-deploy health and smoke checks
  teardown         Remove only what bootstrap created
  status           Show current state of this adapter on the target
  manifest         Show current deployment manifest pointer (image digest + bundle hash)
  params           Print resolved params (params.yaml + contract merge)
  contract         Regenerate deploy/contract.yaml from SST and exit
```

The CLI MUST resolve the per-target adapter directory using the strict rule in Adapter Locality:

1. **If `DEPLOY_TARGETS_ROOT` is set** → look ONLY under `${DEPLOY_TARGETS_ROOT}/<project>/<target>/`. If `params.yaml` is missing, FAIL (no silent fallback to in-tree).
2. **Else (`DEPLOY_TARGETS_ROOT` unset)** → look ONLY under `<repo>/deploy/<target>/`. If `params.yaml` is missing, FAIL.

The CLI MUST NEVER silently fall back from out-of-tree to in-tree when `DEPLOY_TARGETS_ROOT` is set.

The CLI MUST refuse to run any action other than `preconditions`, `params`, `contract`, `manifest`, or `rollback` if `deploy/contract.yaml` is stale relative to the SST (drift check first).

The `apply` and `rollback` actions MUST be idempotent: re-running with the same digest+bundle hash MUST be a no-op.

## Classification Rules

| Value Type | Lives In |
|------------|----------|
| Service list, ports, internal DNS names, env key names, health endpoints, persistent volumes | `deploy/contract.yaml` (generated from SST, in-tree) |
| Target FQDN, target host IP, Tailscale machine name, target-specific TLS cert dir, target user/group, target storage paths | `<adapter-root>/<target>/params.yaml` (in-tree for generic targets, out-of-tree for operator-owned targets — see Adapter Locality) |
| Caddy site config for target | `<adapter-root>/<target>/conf.d/<project>-<target>.caddy` (dropped in by bootstrap) |
| ufw rule set for target | bootstrap script, tagged with `# project=<name> target=<target>` |
| systemd unit names | `<project>-<target>-<purpose>.service` |
| Container names | `${PROJECT}-${TARGET}-${SERVICE}` |
| Network names | `${PROJECT}_${TARGET}_default` |
| Volume names | `${PROJECT}_${TARGET}_${VOLUME}` |
| Cross-project host-singleton coordination (port allocations across projects on a shared host, ufw tag registry) | Operator's private out-of-tree `shared/` directory; NEVER any project repo |

Any value in `<adapter-root>/<target>/params.yaml` MUST NOT also live in the SST. Any value in the SST MUST NOT also live in `params.yaml`. Operator-coupled values (real FQDN, real VPN IP, real reverse-proxy site name, cross-project workspace notes) MUST live in an out-of-tree adapter when the project repo is or will be public.

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

## Build-Once Deploy-Many Pattern

The same git SHA MUST produce one immutable application image and a set of per-environment config bundles. The same image digest is then deployed to every target (dev, staging, prod, home-lab, cloud) by pairing it with the matching environment's config bundle.

### Three-Artifact Model

| Artifact | Identity | Mutability | Producer | Consumer |
|----------|----------|-----------|----------|----------|
| **Application image** | `sha256:<digest>` | Immutable | CI (`build` workflow) | `apply` action on every target |
| **Config bundle** | `<env>-<bundle-hash>` (tar.gz of generated env files for one env) | Immutable per (env, sourceSha) | CI (`build` workflow) | `apply` action on the target whose env matches |
| **Deployment manifest** | `deploy/<target>/manifest.yaml` | Mutable (pointer pair) | `apply` action on the target | Adapter scripts (`status`, `verify`, `rollback`) |

### Pipeline Shape

```
┌─────────────────────────────────────────────────────────────────────┐
│ CI (.github/workflows/build.yml on push to main)                    │
│                                                                     │
│  1. test + lint + framework gates                                   │
│  2. docker buildx build                                             │
│       → image: <registry>/<project>/<service>@sha256:<digest>       │
│  3. cosign sign --keyless (Sigstore + Rekor transparency log)       │
│  4. syft attestation → SBOM                                         │
│  5. SLSA provenance attestation                                     │
│  6. for env in <project's environments>:                            │
│       ./<project>.sh config generate --env <env> --bundle           │
│       → config-bundle-<env>-<sourceSha>.tar.gz                      │
│  7. publish image + bundles + attestations to registry              │
│  8. push immutable tags: sha256:<digest>, git-<sha>, build-<run>    │
│  9. CI ENDS HERE — no SSH, no deploy, no host mutation              │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│ Operator (or trust-isolated automation)                             │
│                                                                     │
│  ./<project>.sh deploy <target> apply \                             │
│      --image=sha256:<digest> \                                      │
│      --config-bundle=<env>-<sourceSha>                              │
│                                                                     │
│  Adapter:                                                           │
│   1. Pull image by digest (NEVER by tag)                            │
│   2. Verify cosign signature against Rekor                          │
│   3. Verify SBOM + provenance attestations exist                    │
│   4. Pull config bundle, verify hash                                │
│   5. Write deploy/<target>/manifest.yaml (new pointer pair)         │
│   6. Run rollout strategy (recreate / blue-green / canary)          │
│   7. verify.sh → health, smoke, parity checks                       │
│   8. On failure: rollback.sh → restore prior manifest pointer       │
└─────────────────────────────────────────────────────────────────────┘
```

### Manifest File Schema

```yaml
# deploy/<target>/manifest.yaml — written by `apply`, read by all other actions
project: <project>
target: <target>
appliedAt: <iso8601>
appliedBy: <operator or automation identity>
image:
  digest: sha256:<digest>          # Immutable, mandatory
  sourceTag: git-<sha> | build-<run>  # Informational only
configBundle:
  hash: <env>-<sourceSha>
  env: dev | staging | prod | ...
  sourceSha: <SST git sha at build time>
attestations:
  signature: <cosign signature ref>
  sbom: <SBOM artifact ref>
  provenance: <SLSA provenance ref>
previousManifest:
  image: { digest: sha256:<prior> }
  configBundle: { hash: <prior-hash> }
  appliedAt: <iso8601>
rolloutStrategy: recreate | blue-green | canary
```

The `previousManifest` field enables `rollback` to restore the prior pointer without rebuilding.

### Registry Choice

Each target's `params.yaml` declares the registry it pulls from:

```yaml
# deploy/<target>/params.yaml (excerpt)
registry:
  url: registry.example.local:5000   # self-hosted
  # OR: ghcr.io                      # GitHub Container Registry
  # OR: <account>.dkr.ecr.<region>.amazonaws.com
authMethod: oci-token | docker-config | aws-iam | gcp-sa
```

A project MAY ship images to multiple registries from CI. Each target picks one. This supports the "self-hosted for sensitive targets, public registry for non-sensitive" model.

### Verification Steps (Build Side, in CI)

```bash
# Image is content-addressed (digest pinned)
[ -n "$IMAGE_DIGEST" ] && [[ "$IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]

# Cosign signature verifies against Rekor transparency log
cosign verify --certificate-identity-regexp '<expected identity>' \
              --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
              <registry>/<project>/<service>@$IMAGE_DIGEST

# SBOM attestation exists
cosign verify-attestation --type spdxjson \
                          --certificate-identity-regexp '<expected identity>' \
                          --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
                          <registry>/<project>/<service>@$IMAGE_DIGEST

# Same digest is reachable from staging tag and prod tag (build-once invariant)
docker manifest inspect <registry>/<project>/<service>:staging-latest | grep "$IMAGE_DIGEST"
docker manifest inspect <registry>/<project>/<service>:prod-latest    | grep "$IMAGE_DIGEST"
```

### Verification Steps (Deploy Side, in Adapter `apply.sh`)

```bash
# Pull by digest, never by tag
docker pull "${REGISTRY}/${PROJECT}/${SERVICE}@${IMAGE_DIGEST}"

# Verify signature before run
cosign verify --certificate-identity-regexp '<expected>' \
              --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
              "${REGISTRY}/${PROJECT}/${SERVICE}@${IMAGE_DIGEST}"

# Verify config bundle hash
sha256sum "config-bundle-${ENV}-${SOURCE_SHA}.tar.gz" | grep -q "${EXPECTED_HASH}"

# Confirm Compose file or runtime spec pins image by DIGEST not tag
grep -E "image:.*sha256:" docker-compose.runtime.yml
! grep -E "image:.*:latest|image:.*:staging-latest|image:.*:prod-latest" docker-compose.runtime.yml
```

## Rollout Strategy Ladder

Adapters MAY support multiple rollout strategies. The strategy is declared in `params.yaml` and may differ per target. Recommended progression:

| Strategy | When | Mechanism | Downtime |
|----------|------|-----------|----------|
| **recreate** | Now / single-host home-lab | `docker compose down && up -d` per service | 2-30 sec per service |
| **blue-green** | Pre-MVP / multi-instance | Run `<service>-blue` and `<service>-green`; flip Caddy upstream after health-check passes | Zero |
| **canary** | 1.0+ / multi-instance + observability rich | Roll new image to N% of replicas, watch metrics, advance/rollback | Zero |
| **flag-gated rollout** | Dangerous changes regardless of replica count | Image deploys with feature OFF; flag flipped after manual verification | Zero (image side); flag flip is the cutover |

`rollback` semantics by strategy:

| Strategy | Rollback Action |
|----------|----------------|
| recreate | Re-pull `previousManifest.image.digest`, re-mount prior bundle, restart services |
| blue-green | Flip Caddy upstream back to prior color (no image pull required) |
| canary | Reduce new-image replica % to 0, route traffic back to old image |
| flag-gated | Flip flag OFF (image stays deployed) |

The `apply` action MUST refuse a rollout strategy the target's `params.yaml` does not declare.

## CI ↔ Adapter Handshake

The CI build job and the adapter `apply` action must agree on a strict handshake. CI publishes a **build manifest** that adapters consume:

```yaml
# Published as: <registry>/<project>/build-manifests/<sourceSha>.yaml
project: <project>
sourceSha: <git sha at build>
builtAt: <iso8601>
images:
  <service>:
    digest: sha256:<digest>
    registries: [<registry-1>, <registry-2>]    # Where this digest is reachable
configBundles:
  dev:     { hash: dev-<sourceSha>,     artifact: <ref> }
  staging: { hash: staging-<sourceSha>, artifact: <ref> }
  prod:    { hash: prod-<sourceSha>,    artifact: <ref> }
attestations:
  signatureBundle: <ref>
  sbom: <ref>
  provenance: <ref>
```

The `apply` action MUST:
1. Fetch the build manifest by sourceSha or by image digest
2. Verify the requested image digest matches the build manifest entry
3. Verify the requested bundle hash matches the build manifest entry for the env
4. Fail-fast if either is missing or mismatched (no re-build, no fallback)

This handshake is the contract that makes "build once, deploy many" auditable: every deployed artifact is traceable back to a single source SHA + build run.

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

# 4. Apply is digest-pinned and idempotent
./<project>.sh deploy <target> apply --image=sha256:<digest> --config-bundle=<env>-<sourceSha>
./<project>.sh deploy <target> apply --image=sha256:<digest> --config-bundle=<env>-<sourceSha>   # MUST be no-op

# 5. Manifest reflects the applied artifact
./<project>.sh deploy <target> manifest | grep -q "sha256:<digest>"

# 6. Verify passes
./<project>.sh deploy <target> verify

# 7. Rollback restores prior pointer (no rebuild)
./<project>.sh deploy <target> rollback
./<project>.sh deploy <target> manifest | grep -q "<prior-digest>"

# 8. Teardown is precise (host singletons untouched)
./<project>.sh deploy <target> teardown
diff -q /etc/caddy/Caddyfile.snapshot.before /etc/caddy/Caddyfile     # unchanged
diff -q /etc/docker/daemon.json.snapshot.before /etc/docker/daemon.json  # unchanged
sudo ufw status numbered | grep "project=<project> target=<target>"      # 0 matches

# 9. Bootstrap after teardown succeeds
./<project>.sh deploy <target> bootstrap
./<project>.sh deploy <target> verify
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| Hardcoded host IP (e.g. `<a real Tailscale or LAN IP>`) in SST | Target leakage | Move to `deploy/<target>/params.yaml` |
| `bootstrap.sh` writes `/etc/caddy/Caddyfile` | Destroys peer adapters | Drop file in `conf.d/`, assert main file imports it |
| Container named `gateway` (no namespace) | Collides with peer adapter for another target | `${PROJECT}-${TARGET}-gateway` |
| `teardown.sh` runs `docker system prune -af` | Destroys peer adapters' images | Remove only this adapter's labelled resources |
| `docs/Deployment.md` describes one target step-by-step | New targets force doc rewrite | Main doc explains contract; per-target docs in `deploy/<target>/README.md` |
| Bootstrap requires manual `systemctl daemon-reload` | Not idempotent in CI | Bootstrap runs the reload itself |
| Two adapters share the same volume name | Data corruption when both run on same host | `${PROJECT}_${TARGET}_${VOLUME}` |
| `params.yaml` references a value also in SST | Dual source of truth | Move to one location only |
| **CI workflow runs `./<project>.sh deploy`, builds and deploys in same job** | Conflates build with deploy; same SHA can produce different deployed state | CI publishes artifacts only; deploy is a separate operator/automation step |
| **Compose / runtime spec uses `image: <name>:latest`** | Mutable tag; running container's behavior cannot be tied to a SHA | Pin by digest: `image: <name>@sha256:<digest>` |
| **Compose / runtime spec uses `image: <name>:staging-latest`** | Mutable tag; "promotion" via tag-swap loses traceability | Pin by digest in `manifest.yaml`; tags are informational only |
| **Adapter `bootstrap.sh` runs `docker build`** | Build should happen once in CI, not per-target | `apply.sh` pulls pre-built image by digest |
| **`apply` action falls back to building locally if registry pull fails** | Defeats build-once invariant | Fail-fast; require registry availability; no silent fallback |
| **No image signature verification in `apply.sh`** | Allows tampered images to deploy | `cosign verify` MUST run before container start |
| **Config bundle generated at deploy time on the target** | Target-side generation can diverge from CI-side generation | Config bundle is a CI artifact; adapter consumes it as-is |
| **`rollback` rebuilds from a prior git SHA** | Slow; risks rebuild divergence | Rollback is pure pointer-swap to `previousManifest`; no rebuild |
| **CI workflow needs SSH credentials to a target host** | Trust boundary leak; CI compromise → target compromise | CI ends at registry push; deploy is downstream of CI |
| **Same Compose file used for build and deploy** | Build context bloats deploy spec; image rebuilds on every deploy | Separate `docker-compose.build.yml` (CI only) and `docker-compose.runtime.yml` (deploy) |
| **Operator-coupled adapter (`deploy/<my-host>/`) committed to a public project repo, even with placeholder values** | Topology + identity reconnaissance; placeholders silently collect real values via copy-paste; cross-correlates the operator's other public repos via shared host-singleton notes | Move adapter to `${DEPLOY_TARGETS_ROOT}/<project>/<target>/` (operator-private repo). In-tree `deploy/<target>/` is reserved for generic / shareable targets only (`_example`, `local-dev`, vendor templates) |
| **CLI silently falls back from out-of-tree to in-tree adapter when `DEPLOY_TARGETS_ROOT` is set but the requested target is missing under it** | Operator may have migrated and deleted the in-tree leftover; silent fallback risks deploying a stale adapter | Fail-fast with both attempted paths in the error message |
| **Cross-project host-singleton coordination notes (port allocations, ufw tag registry, systemd unit namespaces) committed to any project repo** | Cross-correlates the operator's projects publicly; encourages tight coupling between project repos | Lives only under operator-private `${DEPLOY_TARGETS_ROOT}/shared/` |

## Integration With Bubbles Governance

| Bubbles Gate | Adapter Relevance |
|--------------|-------------------|
| Implementation Reality Scan (G028) | Detects target-specific values leaking into source/SST |
| Integration Completeness (G029) | New service must be added to contract AND wired into ≥1 adapter |
| Vertical Slice (G035) | Deployment artifact (Caddy site, systemd unit, etc.) must exist for every target the service is deployed to |
| Pre-Completion Self-Audit | `deploy <target> bootstrap` re-run idempotency proof must be in evidence |
| Docker Lifecycle Governance | Adapter teardown must not destroy peer adapters' state |
| **Build-Once-Deploy-Many Integrity (G079)** | Production deployment manifests MUST pin image by digest, MUST have a CI-signed signature, and MUST consume a CI-published config bundle. No mutable tags. No deploy-time rebuilds. |

## References

- [bubbles-deployment-target.instructions.md](../../instructions/bubbles-deployment-target.instructions.md)
- [bubbles-config-sst skill](../bubbles-config-sst/SKILL.md)
- [bubbles-docker-lifecycle-governance skill](../bubbles-docker-lifecycle-governance/SKILL.md)
- [bubbles-docker-port-standards skill](../bubbles-docker-port-standards/SKILL.md)
- [bubbles-test-environment-isolation skill](../bubbles-test-environment-isolation/SKILL.md)
