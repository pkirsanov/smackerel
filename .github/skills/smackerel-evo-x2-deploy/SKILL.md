---
name: smackerel-evo-x2-deploy
description: Smackerel-specific pointers for BUILDING and DEPLOYING Smackerel to the evo-x2 home-lab target under the local-operator trust model. Use when an agent is asked to build/sign/deploy Smackerel to evo-x2, or needs the on-host checkout path, image names, required build-env, and deploy command. The SHARED operator environment — evo-x2 host identity, every age/cosign/sops/ghcr secret+key LOCATION + value-safe load-method, build-tool prerequisites, and the full build→sign→deploy procedure — is documented ONCE in the knb repo skill knb-evo-x2-deploy; this skill only adds Smackerel's substitutions. Locations + procedures ONLY, never secret values.
---

# Smackerel → evo-x2 home-lab deploy

**Do not re-discover the operator environment each session.** The canonical,
product-agnostic runbook lives in the **knb** repo and covers everything shared
across products: the evo-x2 host identity (`phwp@100.106.147.105`, on-target knb
checkout `/home/phwp/knb`, operator `pkirsanov`), every secret/key **LOCATION** +
value-safe load-method (age key, cosign private key + passphrase store, ghcr push
auth), the build-tool prerequisites (docker / cosign / syft / trivy / oras / go —
and how to install `trivy`/`go` when absent), and the full build→sign→promote flow.

> **Canonical runbook:** open the `knb` workspace folder →
> `knb/.github/skills/knb-evo-x2-deploy/SKILL.md`.
> **Secret hygiene (NON-NEGOTIABLE):** follow `knb-secret-terminal-hygiene` — every
> secret is a path + key-name + load-command, **never a value**. Load into an env
> var; never echo it.

## Smackerel substitutions for the knb runbook

| knb runbook placeholder | Smackerel value |
|---|---|
| on-target product checkout | `/home/phwp/smackerel` |
| product CLI | `./smackerel.sh` |
| build + sign command | `./smackerel.sh build --target home-lab` |
| deploy (canonical, from a workstation) | `./smackerel.sh deploy home-lab` |
| deploy (manual on-host promote) | `promote.sh --target home-lab --product smackerel --operator=pkirsanov --local-build-manifest <path>` |
| built images | `smackerel-core`, `smackerel-ml` (contract: `registry.core`, `registry.ml`) |
| local build manifest lands in | `/home/phwp/smackerel/dist/local-build-manifests/local-build-manifest-<sha>.yaml` (+ `.sig`) |
| deploy order on evo-x2 | **first** (Smackerel proves host secret / Caddy / release-proof path) |

## Smackerel-specific build-env (REQUIRED)

In addition to the cosign env from the knb runbook (`OPERATOR_COSIGN_KEY`,
`OPERATOR_COSIGN_PUBKEY`, `COSIGN_PASSWORD` loaded from the SOPS passphrase store),
Smackerel's inner build **requires the hardware tier** or it fails
`F061-HARDWARE-TIER-MISSING`:

```bash
export SMACKEREL_HARDWARE_TIER=accel   # home-lab = accel (the evo-x2 GPU tier), matches CI's home-lab→accel mapping
```

Set it in the MAIN shell session — not inside a backgrounded `&` subshell, or it
won't persist into the build.

## Smackerel-specific notes

- **Observability posture is `bundled`** (spec 032): Smackerel brings its OWN
  spec-049 Prometheus on the zero-manual apply path with **no adapter `--profile`
  flag** — the home-lab config bundle carries `COMPOSE_PROFILES=searxng,monitoring`,
  so `prometheus` starts Day-1. Grafana + Alertmanager are NOT in the deploy compose
  (tracked operator dependency R-082-C); evaluated alert rules currently fire into
  the void until R-082-C or the shared spec-014 stack lands.
- **`config generate` first.** The generated `config/generated/env.sh` is gitignored,
  so a `git pull`/`git checkout` does NOT refresh it — run `./smackerel.sh config generate`
  before building on an updated checkout or the build fails loud on a missing key.
- **Trivy CRITICAL/HIGH gate** blocks the build on any fixable HIGH/CRITICAL CVE in
  BOTH the Go tree and the ML sidecar's `ml/requirements.txt` — bump the dependency
  and push to `main`; there is no gate skip (the same finding fails CI too).
- **Post-deploy `core: degraded` / "Invalid JSON from LLM"** under evo-x2 host
  contention is an LLM-quality artifact, not a deploy fault — confirm containers are
  healthy and digests match `manifest.yaml` (`verify.sh` does this) before chasing it.

## Canonical sources (read for depth)

- `knb/.github/skills/knb-evo-x2-deploy/SKILL.md` — the shared operator runbook (host, secrets, tools, build+deploy).
- `knb/smackerel/home-lab/README.md` — the knb-owned Smackerel adapter contract (params, rollout, observability posture).
- `knb/.github/skills/knb-secret-terminal-hygiene/SKILL.md` — value-safe secret handling in the shell.
