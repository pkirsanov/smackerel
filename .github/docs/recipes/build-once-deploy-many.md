# Recipe: Build-Once Deploy-Many

> *"Build it once, sign it, and deploy the same digest everywhere it needs to live."*

Use this when a project ships container images to **multiple environments** (dev, staging, prod, home-lab, cloud, on-prem) and you want a single git SHA → one immutable image → many deployments.

This recipe links the work owned by `bubbles.devops` (build pipeline + adapter) and `bubbles.releases` (deployment.md release packet doc + Phase Overview accuracy).

## Why

Independent build-then-deploy-per-target leaks problems:
- "Works in staging" depends on the staging build, not the staging tested artifact
- Mutable tags (`:latest`, `staging-latest`) drift; the same tag points to different bytes over time
- CI/deploy fusion forces CI to hold production credentials
- Hand-built bundles drift; staging config differs from prod config without traceability

Build-Once Deploy-Many fixes this with the **three-artifact model**:

| Artifact | Identity | Mutability | Producer | Consumer |
|----------|----------|-----------|----------|----------|
| Application image | `sha256:<digest>` | Immutable | CI build job | Adapter `apply` on every target |
| Config bundle | `<env>-<bundle-hash>` (one per env, deterministic) | Immutable per `(env, sourceSha)` | CI build job | Adapter `apply` on the matching env's target |
| Deployment manifest | `deploy/<target>/manifest.yaml` (image digest + bundle hash pointer pair) | Mutable (operator-controlled) | Adapter `apply` action on the target | Adapter `status`, `verify`, `rollback` |

Gate **G079 (Build-Once Deploy-Many Integrity)** enforces this — advisory in the framework, blocking in opted-in product repos.

## Pipeline Shape

```
┌──────────────────────────────────────────────────────────────┐
│ CI (.github/workflows/build.yml)                             │
│  1. test + lint + framework gates                            │
│  2. docker buildx build → image @sha256:<digest>             │
│  3. cosign sign --keyless (Sigstore + Rekor)                 │
│  4. syft → SBOM attestation                                  │
│  5. SLSA build-provenance attestation                        │
│  6. Trivy CRITICAL+HIGH gate                                 │
│  7. for each env:                                            │
│       ./<project>.sh config generate --env <env> --bundle    │
│         --source-sha <SHA>                                   │
│       → config-bundle-<env>-<SHA>.tar.gz                     │
│  8. publish image + bundles + attestations to ghcr           │
│  9. write build-manifest-<sourceSha>.yaml (CI artifact)      │
│ 10. CI ENDS HERE — no SSH, no apply, no host mutation        │
└──────────────────────────────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────┐
│ Operator (or trust-isolated automation)                      │
│  scripts/deploy/promote.sh \                                 │
│      --target <target> \                                     │
│      --build-manifest build-manifest-<sourceSha>.yaml        │
│                                                              │
│  → resolves digests + bundle ref for the target's env        │
│  → calls ./<project>.sh deploy <target> apply ...            │
│                                                              │
│  Adapter `apply.sh`:                                         │
│   1. Pull image by digest (NEVER by tag)                     │
│   2. Verify cosign signature against Rekor                   │
│   3. Verify SBOM + SLSA provenance attestations              │
│   4. Pull config bundle + verify hash                        │
│   5. Write deploy/<target>/manifest.yaml (new pointer)       │
│   6. Run rollout (recreate / blue-green / canary)            │
│   7. verify.sh → health, smoke, parity                       │
│   8. On failure → rollback.sh (pointer-swap, no rebuild)     │
└──────────────────────────────────────────────────────────────┘
```

## Operator Commands

```bash
# Build (CI) — automatic on push to main
# Produces: ghcr.io/<owner>/<project>/<service>@sha256:<digest>
#           ghcr.io/<owner>/<project>-config-bundles:<env>-<sourceSha>
#           build-manifest-<sourceSha>.yaml

# Promote a built SHA to a target (operator)
bash scripts/deploy/promote.sh \
    --target <target> \
    --build-manifest build-manifest-<sourceSha>.yaml

# Or apply directly with explicit digests/bundle
./<project>.sh deploy <target> apply \
    --image-<service-1>=sha256:<digest-1> \
    --image-<service-2>=sha256:<digest-2> \
    --config-bundle=<env>-<sourceSha> \
    --source-sha=<sourceSha>

# Verify after apply
./<project>.sh deploy <target> verify

# Rollback (pointer-swap only — does NOT rebuild)
./<project>.sh deploy <target> rollback
# Or via convenience wrapper
bash scripts/deploy/rollback.sh --target <target>

# Inspect current pointer
./<project>.sh deploy <target> manifest

# Show resolved params (params.yaml + contract merge)
./<project>.sh deploy <target> params
```

## Forbidden Patterns

| Forbidden | Why | Use Instead |
|-----------|-----|-------------|
| Mutable image tag in `manifest.yaml` (`:latest`, `:main`, branch names) | Loses digest pinning | Pin `image: <registry>/<project>/<service>@sha256:<digest>` |
| CI workflow performing `ssh`/`scp`/`rsync`/`apply` | Fuses build with deploy; wrong trust boundary | CI publishes artifacts only; operator runs apply |
| Adapter `apply.sh` invoking `docker build`/`cargo build`/`npm run build` | Defeats build-once invariant | Pull pre-built image by digest from registry |
| Adapter falling back to local build on registry pull failure | Silent fallback masks supply-chain failures | Fail-fast with clear error |
| Missing `cosign verify` before container start | Allows tampered images to run | `cosign verify --certificate-identity-regexp ... @${IMAGE_DIGEST}` |
| Missing config bundle hash verification | Allows tampered config to deploy | `sha256sum bundle | grep -q "${EXPECTED_HASH}"` |
| `rollback.sh` rebuilding instead of pointer-swap | Slow, non-idempotent | Restore `previousManifest` pointer pair |
| Target-side bundle generation | Bundle becomes deploy-time non-deterministic | Bundle is a CI build artifact, immutable per `(env, sourceSha)` |
| Plaintext secrets in config bundle | Secrets in artifact registry | Use injected env vars / sealed secrets at host |
| Two targets sharing one `deploy/manifest.yaml` | Prevents independent rollback | Each target owns `deploy/<target>/manifest.yaml` |

## Workflow Routing

| Work | Mode / Agent |
|------|--------------|
| Author or modify `deploy/<target>/` adapter | `/bubbles.devops focus: deployment-target` (or `/bubbles.workflow mode: devops-to-doc`) |
| Wire up CI build pipeline + cosign/SBOM/SLSA | `/bubbles.devops focus: ci-cd` |
| Add `scripts/deploy/promote.sh` / `rollback.sh` | `/bubbles.devops focus: release-automation` |
| Add config bundle generation to SST | `/bubbles.devops focus: config-sst` |
| Update the deployment.md doc inside a phase release packet | `/bubbles.workflow mode: release-planning-to-doc <phase> mode: refresh` (Sonny edits the deployment.md packet doc) |
| Audit a deployment surface for G079 violations | `/bubbles.security` (supply-chain section) and `/bubbles.regression` (deployment regression scan) |
| Detect deployment regressions | `/bubbles.regression` (looks for digest drift, bundle drift, mutable-tag reintroduction, removed cosign calls) |

## Verification Checklist

Use this before merging any change that touches `deploy/`, `.github/workflows/build.yml`, `config/<project>.yaml`, or `scripts/deploy/`:

- [ ] Image is pinned by `sha256:<digest>` in every deployment manifest — no mutable tags
- [ ] CI workflow contains zero `ssh`/`scp`/`rsync`/`apply.sh` invocations under job steps (a `no-ssh-guard` job is acceptable when it greps the workflow source itself)
- [ ] `apply.sh` calls `cosign verify` with `--certificate-identity-regexp` and `--certificate-oidc-issuer` BEFORE container start
- [ ] `apply.sh` verifies `sha256sum` of the pulled bundle against the expected hash
- [ ] `rollback.sh` is pointer-swap only — `grep -E 'docker build|cargo build|npm run build' rollback.sh` returns nothing
- [ ] Each deployment target owns its own `deploy/<target>/manifest.yaml`
- [ ] Re-running `apply.sh` with the same digest+bundle is a no-op
- [ ] Re-running `bootstrap.sh` produces zero diffs the second time
- [ ] Two CI runs on the same source SHA produce byte-identical config bundles
- [ ] No plaintext secret values inside any committed config bundle artifact (only `${VAR}` placeholders)
- [ ] Trivy CRITICAL+HIGH gate is active in CI (no `continue-on-error` or `--severity LOW` bypass)

## Companion Assets

- Skill: [`bubbles-deployment-target-adapter`](../../skills/bubbles-deployment-target-adapter/SKILL.md) — full adapter pattern, idempotency checklist, CI ↔ adapter handshake, anti-patterns table
- Skill: [`bubbles-config-sst`](../../skills/bubbles-config-sst/SKILL.md) — Config Bundle Artifact section
- Skill: [`bubbles-docker-port-standards`](../../skills/bubbles-docker-port-standards/SKILL.md) — 10k Rule + Dual-URL Standard
- Skill: [`bubbles-docker-lifecycle-governance`](../../skills/bubbles-docker-lifecycle-governance/SKILL.md) — cleanup, freshness, volume safety
- Skill: [`bubbles-test-environment-isolation`](../../skills/bubbles-test-environment-isolation/SKILL.md) — ephemeral test storage
- Instructions: [`bubbles-deployment-target.instructions.md`](../../instructions/bubbles-deployment-target.instructions.md)
- Instructions: [`bubbles-config-sst.instructions.md`](../../instructions/bubbles-config-sst.instructions.md)
- Instructions: [`bubbles-docker-lifecycle-governance.instructions.md`](../../instructions/bubbles-docker-lifecycle-governance.instructions.md)
- Instructions: [`bubbles-docker-ports.instructions.md`](../../instructions/bubbles-docker-ports.instructions.md)
- Instructions: [`bubbles-test-environment-isolation.instructions.md`](../../instructions/bubbles-test-environment-isolation.instructions.md)
- State Gate: **G079 (Build-Once Deploy-Many Integrity)** — `agents/bubbles_shared/state-gates.md`
- Related recipe: [DevOps Work](devops-work.md) — focused devops execution lane
- Related recipe: [Release Planning](release-planning.md) — Sonny's release packet authoring (deployment.md packet doc)
