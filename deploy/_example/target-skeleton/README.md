# `deploy/_example/target-skeleton/` — Adapter Skeleton

This directory is a **skeleton** for a new deploy target. It is intentionally a
stub: every script exits 1 with a `TODO(operator)` message until the operator
copies the directory and fills it in.

The skeleton lives in-tree because it contains zero operator-specific values.
Real adapters that name a real host MUST live out-of-tree under
`${DEPLOY_TARGETS_ROOT}/<project>/<target>/` (see
[`.github/instructions/bubbles-deployment-target.instructions.md`](../../../.github/instructions/bubbles-deployment-target.instructions.md)).

## Use it

1. Copy this directory to a real target name. Two locality modes are supported,
   and the project CLI selects between them strictly based on whether
   `DEPLOY_TARGETS_ROOT` is set.

   - **In-tree** (generic, shareable targets — safe for public repos):

     ```bash
     cp -r deploy/_example/target-skeleton deploy/<your-target>
     ```

   - **Out-of-tree** (operator-coupled targets — required when this repo is or
     will be public):

     ```bash
     mkdir -p "${DEPLOY_TARGETS_ROOT}/<project>"
     cp -r deploy/_example/target-skeleton \
       "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"
     ```

2. Replace every `<placeholder>` in `params.yaml` with real values for the
   target (FQDN, internal address, reverse-proxy choice, container naming
   prefix, registry roots, firewall tag).

3. Implement each script's `TODO(operator)` block, keeping the contract
   documented in the bubbles instruction file:

   | Script              | Responsibility                                              |
   |---------------------|-------------------------------------------------------------|
   | `preconditions.sh`  | Verify the target is ready; no-op when healthy              |
   | `bootstrap.sh`      | Idempotent install/upgrade; drop-ins only; tagged for teardown |
   | `apply.sh`          | Pull-by-digest, cosign verify, bundle verify, swap manifest |
   | `rollback.sh`       | Pure pointer-swap to `previousManifest`; never rebuild       |
   | `verify.sh`         | Post-deploy health and smoke checks (`--max-time` on every curl) |
   | `teardown.sh`       | Remove ONLY namespaced resources; leave host singletons     |

4. Set the executable bit on every `*.sh` file (the in-tree skeleton ships
   with `+x`; the bit is preserved across `cp -r` on Linux/macOS but verify
   after copying):

   ```bash
   chmod +x deploy/<your-target>/*.sh
   ```

## How the project CLI resolves this directory

The project CLI's `deploy <target> <action>` (or `deploy-target <target>`) uses
strict locality resolution — it never silently falls back between modes:

| `DEPLOY_TARGETS_ROOT` | Resolved adapter directory                                       |
|-----------------------|------------------------------------------------------------------|
| **unset**             | `<repo>/deploy/<your-target>/` (in-tree)                         |
| **set**               | `${DEPLOY_TARGETS_ROOT}/<project>/<your-target>/` (out-of-tree)  |

If the resolved directory is missing, the CLI fails with a structured error
listing the path it tried, the path it deliberately did NOT try, and the
opt-in/opt-out hint for `DEPLOY_TARGETS_ROOT`.

## Don't

- Don't commit operator-coupled values (real FQDN, real VPN IP, real proxy
  site name, cross-project workspace notes) to a repo that is or will be
  public. Use `${DEPLOY_TARGETS_ROOT}/<project>/<target>/` instead.
- Don't hand-edit `manifest.yaml`. It is written by `apply.sh` and read by
  `rollback.sh`.
- Don't put target-specific values in the project's SST config file. They
  belong in this adapter's `params.yaml`.
- Don't run `docker build` / `cargo build` / `npm run build` from `apply.sh`.
  Build-once-deploy-many means apply only pulls and verifies.
- Don't rebuild on rollback. `rollback.sh` is a pure pointer-swap.
