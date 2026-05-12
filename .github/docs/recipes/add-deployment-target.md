# Recipe: Add A Deployment Target

> *"New rack, same wiring. Plug it into the same patch panel and the rest of the park stays online."* — Tommy Bean.

---

## The Situation

You already have Build-Once Deploy-Many in place: CI builds one image per service, signs it, generates per-environment config bundles, and publishes everything to a registry. Operators promote a built `<sourceSha>` to a target through a per-target adapter under `deploy/<target>/`.

Now you need to add **another target** — a new home-lab machine, a new cloud environment, a staging VPS, or a customer site — without re-architecting the build pipeline, without leaking target-specific values into the SST, and without breaking the targets you already run.

## Quick Start — Natural Language

```
/bubbles.devops  focus: deployment-target add a new target named <name>
```

If the framing is ambiguous (you are not sure whether this is really a new target or a tweak to an existing one):

```
/bubbles.super  how do I add a deployment target
```

The super resolves the wording into the right `bubbles.devops` invocation and identifies whether existing surfaces (CI build job, SST environments list, promote/rollback scripts) need updates first.

## Prerequisites

This recipe assumes the project already has these in place. If any are missing, fix them first — this recipe is not the entry point for setting up Build-Once Deploy-Many from scratch (see [`build-once-deploy-many.md`](build-once-deploy-many.md) for that).

| Prerequisite | Where It Lives | Why It Matters |
|--------------|----------------|----------------|
| Build-Once Deploy-Many CI workflow | `.github/workflows/build.yml` (or equivalent) | Produces the immutable image digests + per-env config bundles every target consumes |
| At least one existing adapter | `deploy/<existing-target>/` | Becomes the template you copy from — you should never write a new adapter from scratch |
| Skill referenced from copilot-instructions.md | `bubbles-deployment-target-adapter` | Encodes the canonical adapter layout, idempotency checklist, and CI ↔ adapter handshake |
| SST config pipeline | `config/<project>.yaml` + generator | Produces the per-environment config bundles the new target will consume |
| Per-environment list | `environments:` section of the SST | Determines which `<env>` bundle the new target binds to |

If you cannot answer "which existing target is the closest analogue?" — stop and ask. Adapters are inherited, not invented.

## The Steps (Manual Control)

### Step 1 — Define The Target's Environment Family

Pick the `<env>` the new target consumes. Two cases:

| Case | What You Do |
|------|-------------|
| Reusing an existing env (e.g., another `home-lab` machine that runs the same env as the first) | Add the target to the project's target list; do NOT add a new entry under `environments:` |
| New env entirely (e.g., a brand-new `staging` lane separate from `dev` and `prod`) | Add the new entry under `environments:` in `config/<project>.yaml` AND extend the CI matrix so a `<env>-<sourceSha>` config bundle is built every CI run |

Verify after the change:

```bash
./<project>.sh config generate --env <env> --bundle --source-sha <sourceSha>
```

The bundle MUST be byte-identical when regenerated on the same `<sourceSha>`. If it is not, the new env is non-deterministic and must be fixed before adapter work proceeds.

### Step 2 — Scaffold `deploy/<new-target>/`

**Copy the closest existing adapter. Do not write from scratch.**

Required files inside the new adapter directory:

| File | Purpose |
|------|---------|
| `params.yaml` | Target-specific values (FQDN, host IP, registry URL, TLS dir, rollout strategy). Anything in `params.yaml` MUST NOT also live in the SST. |
| `manifest.yaml` | Mutable pointer pair (`image.digest` + `configBundle.hash`). Initially empty; written by `apply.sh`. |
| `apply.sh` | Idempotent: pull image by digest, verify cosign signature, verify bundle hash, swap the manifest pointer, run rollout. |
| `rollback.sh` | Pure pointer-swap to `previousManifest`. Never rebuilds. |
| `verify.sh` | Post-deploy health, smoke, parity checks. |
| `README.md` | Operator-facing doc for this target only. |

After copying, walk every line of every file and replace target-specific identifiers from the source target with the new target's values. Pay special attention to:

- Container/network/volume name prefixes (`${PROJECT}_${TARGET}_*`)
- Caddy drop-in filename (`/etc/caddy/conf.d/<project>-<new-target>.caddy`)
- ufw rule tags (`# project=<project> target=<new-target>`)
- systemd unit names (`<project>-<new-target>-<purpose>.service`)
- Registry URL in `params.yaml` (each target picks one registry)

### Step 3 — Wire The Three Verification Call Sites

Inside the new `apply.sh`, three call sites are non-negotiable. They MUST execute in this order BEFORE the container starts. Reference the `bubbles-deployment-target-adapter` skill's "CI ↔ Adapter Handshake" section for the exact handshake schema.

| Call Site | What It Does | Failure Mode |
|-----------|--------------|--------------|
| `cosign verify` | Verifies the pulled image's signature against Sigstore + Rekor with `--certificate-identity-regexp` and `--certificate-oidc-issuer` for the CI's OIDC issuer | Fail-fast; do not start the container |
| Bundle hash check | Compares the pulled bundle's checksum against the expected hash from the build manifest | Fail-fast; do not extract the bundle |
| Manifest pointer write | After both checks pass, write the new `manifest.yaml` with `previousManifest` populated from the prior pointer | Atomic file write; never partial |

If the existing template adapter has all three in place, your job is to confirm the new one inherited them intact. If it does not, your job is to add them — and to fix the template so the next adapter inherits them too.

### Step 4 — Wire The Promote / Rollback Helpers

If the project ships `scripts/deploy/promote.sh` and `scripts/deploy/rollback.sh`, extend them to recognize `<new-target>`:

| File | What To Change |
|------|----------------|
| `scripts/deploy/promote.sh` | Reads `build-manifest-<sourceSha>.yaml`, resolves digests + bundle ref for the target's env, calls `./<project>.sh deploy <new-target> apply ...`. Add the new target to whatever target-list mechanism the script uses (case statement, allowlist, or directory glob over `deploy/*/`). |
| `scripts/deploy/rollback.sh` | Pure pointer-swap convenience wrapper. Same target-list update as promote. |

If the project does not have these helpers, the operator can call `./<project>.sh deploy <new-target> apply ...` and `./<project>.sh deploy <new-target> rollback` directly. Helpers are optional sugar, not policy.

### Step 5 — Run The Idempotency Checklist

This step is the gate. The new adapter is not done until ALL of these prove out, with terminal output captured as evidence in `report.md`:

```bash
# Apply twice with the same digest+bundle → second run is a no-op
./<project>.sh deploy <new-target> apply \
    --image-<service>=sha256:<digest> \
    --config-bundle=<env>-<sourceSha> \
    --source-sha=<sourceSha>
./<project>.sh deploy <new-target> apply \
    --image-<service>=sha256:<digest> \
    --config-bundle=<env>-<sourceSha> \
    --source-sha=<sourceSha>
# → second run MUST exit 0 with zero state changes

# Rollback after partial apply → cleanly reverts to previousManifest
./<project>.sh deploy <new-target> rollback
./<project>.sh deploy <new-target> manifest | grep -q "<prior-digest>"

# Offline registry → fails loudly, never falls back to local build
# Simulate by pointing the registry URL at an unreachable host in params.yaml
./<project>.sh deploy <new-target> apply ...
# → MUST fail-fast with a clear error; MUST NOT invoke any local build command
```

Skip this step and the adapter is not real — it is a wishlist that has not been proven against its own contract.

## Common Modes

| Intent | Invocation |
|--------|------------|
| Greenfield target on a fresh host | `/bubbles.devops focus: deployment-target add a new target named <name>` |
| Mirror an existing target (second machine, same env) | `/bubbles.devops focus: deployment-target mirror <existing-target> as <new-target>` |
| Migrate an old hand-rolled deploy script to the per-target adapter layout | `/bubbles.devops focus: deployment-target migrate legacy <name> deploy script to G079 adapter layout` |
| Add a target that consumes a brand-new env | `/bubbles.devops focus: deployment-target add target <name> with new env <env>` |

## Idempotency Checklist (Canonical)

These are the most-violated invariants. A new target is not green until ALL of these hold:

- `apply.sh` MUST verify the cosign signature against Rekor BEFORE container start
- `apply.sh` MUST verify the config bundle hash BEFORE extraction
- `apply.sh` MUST NOT contain build commands for any language toolchain (the adapter consumes pre-built artifacts only)
- `apply.sh` MUST NOT have a fallback that builds locally on registry pull failure — failure is loud, not silent
- `rollback.sh` MUST be a pure pointer-swap on `previousManifest` — never rebuilds
- `manifest.yaml` MUST pin every image by `sha256:<digest>` — never `:latest`, branch tags, or env-named tags
- Each target MUST own its OWN `deploy/<target>/manifest.yaml` — never a shared root manifest two targets both write to
- Re-running `apply.sh` with the same `(digest, bundle hash)` MUST be a no-op
- Re-running `bootstrap.sh` (if present) MUST produce zero diffs the second time
- Two CI runs on the same `<sourceSha>` MUST produce byte-identical config bundles for every env

## Forbidden Patterns

What makes the new target invalid. Cite Gate **G079 (Build-Once Deploy-Many Integrity)** when rejecting any of these.

| Forbidden | Why It Breaks G079 | Use Instead |
|-----------|--------------------|-------------|
| Mutable image tag in the new target's `manifest.yaml` (`:latest`, `:main`, env-named tags) | Loses digest pinning; same tag drifts to different bytes over time | Pin `image: <registry>/<project>/<service>@sha256:<digest>` |
| New target's `apply.sh` invokes a build command for any language toolchain | Defeats the build-once invariant; the deployed bytes are not the CI-tested bytes | Pull pre-built image by digest from registry |
| New target's `apply.sh` falls back to a local build on registry pull failure | Silent fallback masks supply-chain failures and registry outages | Fail-fast with a clear error |
| New target skips `cosign verify` "to make local testing easier" | Allows tampered images to run on the new host | Verify against Rekor before container start, every time |
| New target skips bundle hash verification | Allows tampered config to deploy to the new host | Verify `sha256sum` against the build-manifest hash before extraction |
| New target's `rollback.sh` rebuilds from a prior `<sourceSha>` | Slow; reintroduces build-divergence risk during incident response | Restore `previousManifest` pointer pair only |
| New target shares `deploy/manifest.yaml` with another target | Prevents independent rollback; one target's apply clobbers the other's pointer | Each target owns `deploy/<new-target>/manifest.yaml` |
| Plaintext secrets in the new target's `params.yaml` or in any committed config bundle artifact | Secrets in the registry; secrets in git history | Use injected env vars / sealed secrets resolved at the host |
| Target-specific values bleeding into `config/<project>.yaml` | Cross-target leakage in the SST; adding the third target requires editing the SST again | Move target-specific values into `deploy/<new-target>/params.yaml` |
| CI workflow now SSHes into the new target after build | Fuses build with deploy; CI compromise becomes target compromise | CI ends at registry push; operator (or trust-isolated automation) runs `apply` |

## Verification

Operator commands the new target MUST support after Step 5:

```bash
./<project>.sh deploy <new-target> preconditions
./<project>.sh deploy <new-target> apply \
    --image-<service>=sha256:<digest> \
    --config-bundle=<env>-<sourceSha> \
    --source-sha=<sourceSha>
./<project>.sh deploy <new-target> verify
./<project>.sh deploy <new-target> manifest
./<project>.sh deploy <new-target> rollback
./<project>.sh deploy <new-target> teardown
```

Regression-detection grep snippets — run these against the new adapter before merging (taken from `bubbles.regression` Step 5):

```bash
# Mutable-tag reintroduction
grep -nE 'image:.*:latest|image:.*:main|image:.*-latest' deploy/<new-target>/

# Apply-side rebuild reintroduced (negative example — MUST return zero)
grep -nE 'docker[[:space:]]build|cargo[[:space:]]build|npm[[:space:]]run[[:space:]]build|go[[:space:]]build' deploy/<new-target>/apply.sh

# Apply-side fallback reintroduced (negative example — MUST return zero)
grep -nE '\|\| docker[[:space:]]build|on.*fail.*build|fallback' deploy/<new-target>/apply.sh

# Cosign verification removed (MUST find at least one call)
grep -n 'cosign verify' deploy/<new-target>/apply.sh

# Bundle hash verification (MUST find at least one call)
grep -nE 'sha256sum|EXPECTED_HASH' deploy/<new-target>/apply.sh

# Rollback now rebuilds (negative example — MUST return zero)
grep -nE 'docker[[:space:]]build|cargo[[:space:]]build' deploy/<new-target>/rollback.sh

# Manifest collision check (count MUST equal one per target, never a single shared root)
find deploy -name 'manifest.yaml'
```

Capture the raw output of each command in `report.md` per the per-DoD-item evidence rule.

## Related Recipes

- [`build-once-deploy-many.md`](build-once-deploy-many.md) — Full pipeline shape, three-artifact model, operator commands
- [`devops-work.md`](devops-work.md) — Focused devops execution lane and `bubbles.devops` focus values
- [`framework-ops.md`](framework-ops.md) — When the framework itself is in scope rather than a project's deploy

## Related Skills

- [`bubbles-deployment-target-adapter`](../../skills/bubbles-deployment-target-adapter/SKILL.md) — Adapter pattern, idempotency checklist, CI ↔ adapter handshake, anti-patterns table
- [`bubbles-config-sst`](../../skills/bubbles-config-sst/SKILL.md) — Config bundle artifact section; ensures the new target's `<env>` bundle is deterministic
- [`bubbles-docker-lifecycle-governance`](../../skills/bubbles-docker-lifecycle-governance/SKILL.md) — Cleanup policy, freshness verification, persistent volume safety on the new host

## Related Instructions

- [`bubbles-deployment-target.instructions.md`](../../instructions/bubbles-deployment-target.instructions.md) — Companion enforcement instructions for deployment targets
- [`bubbles-config-sst.instructions.md`](../../instructions/bubbles-config-sst.instructions.md) — Companion enforcement for config SST

## Related Gate

- **G079 (Build-Once Deploy-Many Integrity)** — `agents/bubbles_shared/state-gates.md`
