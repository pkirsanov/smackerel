# Smackerel Production Deployment Guide

> **Architecture:** Build-Once Deploy-Many — bubbles framework gate **G074**.
> The same `git SHA` produces immutable artifacts that any environment can consume.
> **CI builds and signs. CI does NOT deploy.** Deploy runs on a different trust
> boundary, invoked by an operator (or a separate workflow with adapter-only credentials).

> **Boundary:** This guide describes the **generic** build-and-publish pipeline
> that the Smackerel repo owns. The artifacts produced here (signed images +
> per-env config bundles) are deployment-target-agnostic. Per-target **final**
> configuration — home-lab and any other concrete environment, including real
> hostnames, real IPs, mesh-VPN identity, reverse-proxy site files, `ufw` rules,
> systemd unit names, secret values, and per-target `manifest.yaml` / `params.yaml`
> — lives in the operator-private deploy-adapter overlay repo, NOT in this repo.
> The formal contract for that split is the adapter-locality rule in
> [`.github/instructions/bubbles-deployment-target.instructions.md`](../.github/instructions/bubbles-deployment-target.instructions.md)
> (see "Adapter owns target-specific knowledge" later in this document for the
> per-adapter responsibilities).

This document is operator-facing. For framework rationale see
[`.github/instructions/bubbles-deployment-target.instructions.md`](../.github/instructions/bubbles-deployment-target.instructions.md)
and [`.github/skills/bubbles-deployment-target-adapter/SKILL.md`](../.github/skills/bubbles-deployment-target-adapter/SKILL.md).

---

## Generic Pre-Apply Prerequisites (Product Contract)

These prerequisites are **product-owned** and apply to ANY deploy target
that intends to run Smackerel in a production-class posture
(`runtime.environment=production` AND/OR `auth.enabled=true`). The
deploy-adapter overlay is responsible for **populating** these values
from the target's secret store; this repo does not embed any real
value. Skipping any prerequisite below causes the runtime to fail loud
at startup with an explicit error per Spec 051's defense-in-depth
contract.

| Required config key (dotted YAML path) | Source / How to populate | Failure mode if missing |
|----------------------------------------|--------------------------|-------------------------|
| `auth.signing.active_private_key` | `smackerel-core auth keygen` (PASETO v4.public, one per target). Surfaced as env var `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`. | `internal/config/loadAuthConfig` rejects the load when `SMACKEREL_ENV=production` AND `auth.enabled=true`. |
| `auth.signing.active_key_id` | Operator-chosen short identifier (e.g. `key-2026-05`). Surfaced as env var `AUTH_SIGNING_ACTIVE_KEY_ID`. | Config-load fails alongside `active_private_key`. |
| `auth.at_rest_hashing_key` | `openssl rand -hex 32`. MUST differ from `active_private_key` (Spec 044 OQ-8). Surfaced as env var `AUTH_AT_REST_HASHING_KEY`. | Config-load fails; runtime additionally rejects the case where the two values are equal. |
| `auth.bootstrap_token` | One-shot secret (`openssl rand -hex 24`). Required only on a fresh production deployment with zero enrolled users. Surfaced as env var `AUTH_BOOTSTRAP_TOKEN`. Cleared after the first user enrolls. | Config-load fails per Spec 051 FR-051-004 / SCN-051-S01. |
| `infrastructure.postgres.password` (non-default) | Generated per-target via the deploy-adapter overlay's secret store. Surfaced as env var `POSTGRES_PASSWORD`. The dev default literal `smackerel` is **rejected** at the SST loader for any `home-lab`-class target AND at the runtime when `SMACKEREL_ENV=production` (Spec 051 FR-051-005, SCN-051-S02 — layered rejection). | The bundle generator and the runtime both refuse to start, naming the field. |

These five values MUST be populated by the deploy-adapter overlay
before `apply` is invoked. The product repo's bundle generator
(`./smackerel.sh config generate --env <env> --bundle`) emits the
deterministic placeholder marker `__SECRET_PLACEHOLDER__<KEY>__` for
every managed key on production-class targets (`home-lab`,
`production`); dev/test bundles continue to ship literal values
inline. Per Bubbles gate G074 and Spec 052, secrets MUST NOT live in
the bundle — substitution is the deploy adapter's responsibility,
verified at runtime by Layer 3 (FR-052-007). See
[Bundle Secret Injection](#bundle-secret-injection-spec-052) below for
the full 3-layer contract and the operator workflow.

### Bundle Secret Injection (spec 052)

For production-class targets (`home-lab`, `production`, and any future
non-dev/non-test environment), Smackerel's secrets pipeline implements a
three-layer defense-in-depth contract defined in Spec 052
([`specs/052-bundle-secret-injection-contract/`](../specs/052-bundle-secret-injection-contract/)).
Each layer catches a missing secret on its own; the three layers together
make it impossible for a process to start with a placeholder value masquerading
as a secret.

**Canonical managed-secret manifest** (single source of truth: `infrastructure.secret_keys`
in `config/smackerel.yaml`; mirrored in `internal/config/secret_keys.go::secretKeys`
and the shell mirror in `scripts/commands/config.sh`; drift caught by
`internal/deploy/bundle_secret_contract_test.go`):

| Key | Surfaced as env var | Consumer |
|-----|---------------------|----------|
| `POSTGRES_PASSWORD` | `POSTGRES_PASSWORD` | DATABASE_URL credential component |
| `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` | `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` | PASETO v4.public signing material |
| `AUTH_AT_REST_HASHING_KEY` | `AUTH_AT_REST_HASHING_KEY` | At-rest hashing of revocation entries |
| `AUTH_BOOTSTRAP_TOKEN` | `AUTH_BOOTSTRAP_TOKEN` | One-time per-user enrollment seed |

**Layer 1 — SST loader** (`./smackerel.sh config generate --env <env> --bundle`):

For production-class targets, the bundle generator emits the marker
`__SECRET_PLACEHOLDER__<KEY>__` for every managed key (FR-052-001..006).
The marker format is **deterministic** (FR-052-002): no nonce, no timestamp,
no source-SHA mixing. Identical inputs produce byte-identical bundle bytes
(verified by `internal/deploy/bundle_secret_contract_test.go::TestBundleSecretContract_AdversarialA3_DeterminismDetector`
re-running the bundle generator and comparing sha256 digests). The bundle
also carries a `secret-keys.yaml` manifest declaring exactly which keys the
adapter MUST substitute (FR-052-005).

For `dev` and `test` environments, the bundle continues to ship literal
secret values inline (FR-052-011) — there is no placeholder mode for local
development. The placeholder mode is opt-in **by environment**, never by flag.

**Layer 2 — Deploy adapter** (`<adapter-root>/smackerel/<target>/apply.sh`):

The per-target adapter is the single layer responsible for substituting
real secret values into the bundle. The adapter:

1. Pulls the bundle by sha256 digest from the OCI registry.
2. Reads `secret-keys.yaml` to discover which keys need substitution.
3. Decrypts the operator-private secret store (sops/age, e.g.
   `<knb-repo>/smackerel/secrets/<target>.enc.env`).
4. For each declared key, replaces every occurrence of
   `__SECRET_PLACEHOLDER__<KEY>__` in the bundle's `app.env` with
   the real value.
5. Writes one marker-free effective `app.env` to the target host with
  `chmod 0600`. The adapter must not pass a second secret env file to
  Compose as an active deploy contract.
6. Logs safe metadata only: declared key names, substituted counts,
  placeholder remaining count, encrypted-file identity, cleanup status,
  and outcome. It never logs secret values.
7. Starts the container against the substituted env file.

The substitution is the adapter's contract; the product repo does not ship
operator-coupled home-lab adapter code. For operator-private overlays, the
adapter root is resolved by `DEPLOY_TARGETS_ROOT`. The KNB Smackerel home-lab
adapter also writes an explicit `HOST_BIND_ADDRESS` value into `app.env`; deploy
Compose then fails loudly with
`${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` if the
value is absent. The `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback form is
historical/superseded, forbidden for Smackerel deploy surfaces, and is not
active deploy guidance.

The same injection mechanism owns home-lab's **model selection and Ollama
memory envelope**. The deploy adapter's per-target `params.yaml` carries a
`model_selection:` block; `apply.sh` appends the resolved model env vars (the
`LLM_MODEL`, `OLLAMA_*_MODEL`, `AGENT_PROVIDER_*_MODEL`,
`ASSISTANT_OPEN_KNOWLEDGE_*` and `PHOTOS_INTELLIGENCE_*_MODEL` fields) plus
`OLLAMA_MEMORY_LIMIT` into `app.env` after the generated values
(last-occurrence-wins — the same pattern as `HOST_BIND_ADDRESS`). The product
repo's bundle ships only the generic commodity base (identical to dev), so the
operator's real, hardware-specific model set and envelope never live in this
repo. The Go core's `validateModelEnvelopes` check then validates the
adapter-injected selection against the adapter-injected envelope at container
start. See [`docs/Operations.md`](Operations.md) “Model Envelope Sizing.”

**Layer 3 — Go runtime** (`config.Validate()` + `auth.ValidateRuntimeAuthStartup()`):

At process startup, the runtime checks every managed key against its
exact placeholder marker (FR-052-007). Implementation:

- `internal/config/config.go::Validate()` iterates `SecretKeys()` and rejects
  any key whose resolved value equals `IsPlaceholder(value)` exactly.
  POSTGRES_PASSWORD is read from the parsed DATABASE_URL credential
  component; AUTH_* keys are read from `os.Getenv(key)` because
  `loadAuthConfig()` runs after `Validate()` in `Load()`.
- `internal/auth/startup.go::ValidateRuntimeAuthStartup()` is the
  wiring-time second pass. It checks `cfg.SigningActivePrivateKey` and
  `cfg.AtRestHashingKey` against the placeholder format
  (`__SECRET_PLACEHOLDER__<KEY>__`) inlined as constants, with a parity
  test (`internal/auth/startup_placeholder_test.go::TestValidateRuntimeAuthStartup_PlaceholderFormatParity`)
  that drift-detects any mismatch with the canonical
  `internal/config/secret_keys.go::Placeholder()` format.

The error message format is fixed: `<KEY> still equals placeholder marker — adapter substitution failed (spec 052 FR-052-007)`.
The error names the offending KEY only — the placeholder marker literal
is **never** echoed in error messages, logs, or telemetry (FR-051-007
redaction contract extended). The leakage detector test
(`internal/deploy/bundle_secret_contract_test.go::TestBundleSecretContract_AdversarialA2_LeakageDetector`)
asserts no canary value appears in any error path.

**Operator workflow — adding a new managed secret:**

1. Add the KEY to `infrastructure.secret_keys` in `config/smackerel.yaml`.
2. Add the same KEY (same order) to `secretKeys` in `internal/config/secret_keys.go`.
3. Add the same KEY (same order) to the shell mirror in `scripts/commands/config.sh`.
4. Wire the runtime consumer to read the resolved value from `os.Getenv(key)`
   or from a `Config` struct field populated by `Load()`.
5. If the key is consumed by `internal/auth/startup.go`, extend the
   placeholder check there too (mirror the existing
   AUTH_SIGNING_ACTIVE_PRIVATE_KEY / AUTH_AT_REST_HASHING_KEY pattern).
6. Provide the real value through the deploy adapter overlay
  (`<adapter-root>/smackerel/secrets/<target>.enc.env` for sops/age targets;
  inline in `.env.secrets` for local dev only).
7. Run `./smackerel.sh test unit --go` — the contract test
   (`internal/deploy/bundle_secret_contract_test.go::TestBundleSecretContract_NoLiteralSecretsInHomeLab`)
   catches yaml↔Go↔shell drift before merge.

**Operator workflow — rotating a managed secret:**

1. Update the value in `<adapter-root>/smackerel/secrets/<target>.enc.env`
   (sops/age encrypt: `sops --encrypt --in-place ...`).
2. Re-run `./smackerel.sh deploy-target <target> apply` — the adapter
   re-pulls the bundle (unchanged sha256 because secrets are not in the
   bundle), re-reads the secret store, re-substitutes, and restarts
   the container with the new value.
3. Verify via `<knb-repo>/smackerel/<target>/verify.sh` and the apply
   audit log on the target.

**Auditor inspection:** The full operator runbook for inspecting placeholder
substitution success and the runtime fail-loud path lives in
[Operations.md → "Bundle Secret Substitution (spec 052)"](Operations.md#bundle-secret-substitution-spec-052).

### Connector Live-Stack Evidence Caveat

Connector spec status fields (`spec.md::Status`,
`scopes.md::Status`, `state.json::status`) reflect the certification
of the connector's product-side runtime contract — handler wiring,
config validation, normalizer behavior, error taxonomy, unit + static
coverage. Connector status does **NOT** by itself guarantee that the
connector has been exercised end-to-end against the live external
provider on every target.

| Evidence class | What it proves | Where it lives |
|----------------|---------------|----------------|
| Unit / static | Pure-Go behavior, error taxonomy, normalizer correctness, fail-loud config validation. | `internal/connector/<provider>/*_test.go` |
| Integration (test stack) | The connector compiles into the runtime, the SST keys validate, the handler wires into the connector framework. | `tests/integration/<provider>_*` |
| Live-stack (e2e against real external provider on the target) | The connector authenticates against the real provider (OAuth round-trip or API-key flow), pulls real artifacts, and writes them through the live runtime stack on a specific target. | Per-target operator validation, **not** asserted by Smackerel CI. The deploy-adapter overlay records this on a per-target basis. |

Operators MUST treat connector "done" status as a proof of the first
two evidence classes only. The live-stack class MUST be re-verified
on every new target as part of the deploy-adapter overlay's
target-readiness checklist. The concrete overlay artifact path is recorded in
the operator-private deploy-adapter overlay for that target. This product repo deliberately does NOT host a
target-coupled "connector live-stack readiness" checklist — that
material would entangle product-side and target-side evidence and is
the kind of mix that BUG-001 (`specs/032-documentation-freshness/bugs/BUG-001-home-lab-readiness-plan-stale/`)
removed.

---

## Three artifacts produced per source SHA

| Artifact | Identifier | Mutable? | Producer |
|----------|-----------|----------|----------|
| Application image (`smackerel-core`) | `ghcr.io/pkirsanov/smackerel-core@sha256:<digest>` | No (immutable) | `.github/workflows/build.yml` (CI, when enabled) — or `./smackerel.sh build --target <target>` (local-operator; see "Local-operator (no cloud CI)" below) |
| Application image (`smackerel-ml`)   | `ghcr.io/pkirsanov/smackerel-ml@sha256:<digest>`   | No (immutable) | `.github/workflows/build.yml` (CI, when enabled) — or `./smackerel.sh build --target <target>` (local-operator; see "Local-operator (no cloud CI)" below) |
| Config bundle (per env)              | `ghcr.io/pkirsanov/smackerel-config-bundles:<env>-<sourceSha>` | No (immutable, deterministic) | `./smackerel.sh config generate --env <env> --bundle` |
| Build manifest                       | `build-manifest-<sourceSha>.yaml`                  | No (immutable) | `.github/workflows/build.yml` (CI, when enabled) — or `./smackerel.sh build --target <target>` (local-operator; emits `local-build-manifest-<sourceSha>.yaml`) |
| Deployment manifest (per target)     | `<adapter-root>/<target>/manifest.yaml`            | **Yes** (pointer)              | The per-target deploy adapter (in-tree under `deploy/<target>/` for generic targets, out-of-tree under `${DEPLOY_TARGETS_ROOT}/smackerel/<target>/` for operator-coupled targets like home-lab adapters). Adapter actions are invoked via `./smackerel.sh deploy-target <target> <action>`; the dispatcher (`scripts/commands/deploy_target.sh`) delegates mutating and verification actions to adapter scripts and delegates `status` to adapter `status.sh` when executable. |

Image tags like `:latest`, `:main`, `:staging-latest` MUST NOT be used in any
deployment manifest. Adapters consume images by digest only.

---

## CI pipeline (`.github/workflows/build.yml`)

> **Reference producer — operator-toggleable.** The GitHub CI workflow below is
> the *reference* producer of the immutable artifacts above, not the only one. An
> operator may disable the GitHub Actions workflows and produce the same
> immutable, cosign-signed, digest-addressed `smackerel-core` / `smackerel-ml`
> images and per-env config bundles — plus a `local-build-manifest-<sourceSha>.yaml`
> — via the in-repo local-operator path documented in "Local-operator (no cloud
> CI)" below. The binding contract is the **artifact shape** (Build-Once
> Deploy-Many), not the specific producer; `scripts/deploy/promote.sh` consumes
> either manifest identically.

```text
git push (main / tag) → tests → buildx → trivy CRITICAL/HIGH scan (FAILS workflow on findings)
                              → cosign keyless sign (Sigstore + Rekor)
                              → syft SBOM attestation
                              → SLSA provenance attestation
                              → for env in (dev, test, home-lab):
                                    ./smackerel.sh config generate --env $env --bundle
                                    determinism check (regenerate, compare sha256)
                                    oras push bundle → registry
                              → publish build-manifest-<sourceSha>.yaml
                              → END (no deploy)
```

The CI workflow has **no SSH key**, **no host credentials**, **no `apply` invocation**.
It cannot mutate any deploy target.

### Server-only build manifest on non-release pushes (Spec 098)

The mobile-client build is **decoupled** from the server deploy manifest. A
missing Android signing secret can never block a home-lab SERVER deploy.

- **Non-release push (e.g. push to `main`):** the `build-clients` job is
  **skipped** (it requires the operator-private `ANDROID_KEYSTORE_BASE64` secret
  and has no unsigned fallback — spec 085 FR-CBR-007). `publish-build-manifest`
  still runs once the server-side jobs (`build-images`, `build-bundles`,
  `build-chrome-bridge`) succeed, and publishes a **server-only**
  `build-manifest-<sourceSha>.yaml`: `smackerel-core` + `smackerel-ml` images +
  per-env config bundles + chrome-bridge. The **android platform is NOT
  contracted** — there is no `clients.artifacts[]` block. This is the same
  clients-absent shape `./smackerel.sh build --target home-lab` already emits
  (`dist/local-build-manifests/local-build-manifest-<sourceSha>.yaml`), and
  `scripts/deploy/promote.sh` promotes it through the identical core + ml +
  bundle path (it never reads a `clients:` block).
- **Tagged release (`refs/tags/v*`) or explicit `build_clients: true`
  `workflow_dispatch`:** `build-clients` runs and the manifest pins the AAB +
  APK by `sha256` under `clients.artifacts[]` exactly as spec 085 requires
  (Build-Once-Deploy-Many client integrity is preserved). A `build-clients`
  **failure** on a release still blocks the manifest.

A static contract test
(`internal/deploy/build_workflow_clients_contract_test.go`) drift-protects this:
`build-clients` must be release-gated, `publish-build-manifest` must tolerate a
**skipped** client build, and the clients-block steps must be **success-gated**
on `build-clients`. Adversarial sub-tests prove a non-release manifest is
accepted WITHOUT android digests AND that a release manifest WITHOUT android
digests is rejected.

**knb-side expectation (operator-private adapter — out of this repo's scope):** a
server-only manifest must **not contract the android platform**, so the knb
conformance gate check (c) `E025-CLIENT-MANIFEST-NO-DIGEST` (which fail-closes
when a *contracted* android platform has *no digest*) has nothing to fail-close
on. This matches the `local-build-manifest`, which carries no `clients:` block at
all. The knb deploy-adapter overlay owns this behavior; this repo only guarantees
the manifest shape above.

### Vulnerability Gate (Spec 047 — Deployability Prerequisite)

**Every image (`smackerel-core`, `smackerel-ml`) is scanned by Trivy
BEFORE cosign signing, SBOM attestation, bundle generation, or
build-manifest publication.** If Trivy reports any **fixable**
CRITICAL or HIGH finding (severity `CRITICAL,HIGH`, `exit-code: '1'`,
`ignore-unfixed: true`), the workflow fails immediately and no signed
deployable artifact is produced for that source SHA.

**Threshold tuning (spec 047 design.md §"Threshold Tuning:
ignore-unfixed Policy"):** the gate blocks deployment ONLY on
CRITICAL/HIGH CVEs that have an upstream fix available. Advisory CVEs
in the base images (`debian:bookworm-slim`, `python:3.12-slim`) that
have no upstream fix yet are still surfaced in the SARIF artifact for
operator visibility, but they do NOT block deploy. This matches the
"Risk Controls" policy in the design doc: *"Do not treat advisory or
informational findings as deploy blockers."*

The build manifest carries a `vulnerabilityScan` attestation block
naming the scanner, severity threshold (`severityThreshold:
CRITICAL,HIGH`), the gate-blocking criterion (`gateBlocksOn:
CRITICAL,HIGH-with-upstream-fix`), the threshold-tuning declaration
(`ignoreUnfixed: true`), the rationale (`ignoreUnfixedRationale:
"..."`), and the workflow-artifact ID of the SARIF/JSON scan reports
(`trivy-scan-reports-<sourceSha>`). Operators MUST NOT promote a
build manifest to a deploy target unless this attestation is present
AND the `ignoreUnfixed: true` declaration is consistent with the
workflow's actual `ignore-unfixed` flag value.

A static contract test
(`internal/deploy/build_workflow_vuln_gate_contract_test.go`) enforces
that:

- every image in the build matrix has a matching Trivy scan step,
- the scan runs **before** the first cosign sign step,
- severity is `CRITICAL,HIGH` and exit-code is `'1'`,
- **every Trivy step sets `ignore-unfixed: true`** (block on FIXABLE
  CRITICAL/HIGH only — flipping to `false` is rejected),
- the build manifest carries the full vulnerabilityScan attestation
  block including `gateBlocksOn: CRITICAL,HIGH-with-upstream-fix`,
  `ignoreUnfixed: true`, and the `ignoreUnfixedRationale` field (all
  three FR-047-003 deployability policy fields are drift-protected).

Adding a new image to the matrix without a corresponding scan step,
or flipping `ignore-unfixed` back to `false` on either Trivy step, or
omitting the `ignoreUnfixed: true` line or the `ignoreUnfixedRationale`
field from the build-manifest heredoc — any of these fails the contract
test and blocks merge. Matrix coverage AND threshold-tuning policy
cannot drift silently.

---

## Home-Lab Activation Packet

Home-lab deployability in this product repo is non-live and static until the
operator-private adapter supplies target values and release artifacts. Before a
live home-lab apply, the operator-owned packet must contain all of the following:

| Input | Owner | Purpose |
|-------|-------|---------|
| Source SHA | Upstream build manifest | Ties images, config bundle, attestations, and target manifest to one immutable source revision. |
| Core and ML image digests | Upstream build manifest | Consumed as `--image-core=sha256:<digest>` and `--image-ml=sha256:<digest>`; mutable tags are not accepted. |
| Config bundle ref | Upstream build manifest | Uses the `home-lab-<sourceSha>` bundle identity for the target environment. |
| Config bundle SHA | Upstream build manifest | Passed as `--config-bundle-sha=<sha256-hex>` so the adapter verifies bundle bytes before extraction. |
| Out-of-tree adapter root | Operator environment | `DEPLOY_TARGETS_ROOT` points the product CLI at `<adapter-root>/smackerel/<target>/` for operator-coupled targets. |
| Concrete params | Deploy adapter overlay | Hostname, tailnet placeholders, bind address, Caddy paths, ports, install paths, and rollout strategy are adapter-owned. |
| Encrypted secrets | Deploy adapter overlay | SOPS/age ciphertext supplies managed secret values and first-user bootstrap material without committing plaintext. |
| Release proof | Upstream registry/build artifacts | Cosign signature, SLSA provenance, SBOM, vulnerability proof, bundle hash, and source identity checks. |
| Live approval | Human operator | Static/docs/fixture readiness does not mutate the home-lab host or authorize live activation. |

Current KNB home-lab adapter guidance uses `41001` for `smackerel-core` and
`41002` for `smackerel-ml`. The product dev environment still uses `40001` and
`40002`; do not copy those dev ports into home-lab activation runbooks.

## Operator workflow

```bash
# 1) Pick a release: locate the build-manifest.yaml from the CI run on the desired commit
gh run download <run-id> --name build-manifest-<sourceSha> --dir /tmp/sm-release

# 2) Promote to a target (resolves digests + bundle ref from the manifest, calls apply)
bash scripts/deploy/promote.sh --target home-lab --build-manifest /tmp/sm-release/build-manifest.yaml

# 2b) Or apply directly with explicit digests
./smackerel.sh deploy-target home-lab apply \
  --image-core=sha256:<core-image-digest> \
  --image-ml=sha256:<ml-image-digest> \
  --config-bundle=home-lab-<sourceSha> \
  --config-bundle-sha=<sha256-hex> \
  --source-sha=<sourceSha>

# 3) Verify
./smackerel.sh deploy-target home-lab verify

# 4) On regression, pure pointer-swap rollback (NEVER rebuilds)
./smackerel.sh deploy-target home-lab rollback
```

> BUG-047-001 / DEVOPS-HL-002 — `--config-bundle-sha` is the operator's
> source of truth for adapter-side bundle hash verification. The build
> workflow emits the value as `configBundles[*].sha256` in
> `build-manifest-<sourceSha>.yaml`; `scripts/deploy/promote.sh` reads it
> from there automatically. When invoking `apply` directly, copy the
> `sha256:` field for the target environment out of the build manifest.
> Without this flag the adapter cannot verify the bundle hash, collapsing
> the bundle-tamper defense-in-depth gate.

---

## Deploy-Adapter Overlay Dependency

Some deploy targets shipped by Smackerel are **operator-coupled** —
they require a real hostname, a real Tailscale tailnet identity, real
reverse-proxy site files, real ufw rules, real systemd unit names, and
real backup destinations to actually apply against a host. Per the
deployment ownership boundary recorded in
[`.github/copilot-instructions.md`](../.github/copilot-instructions.md),
that operator-coupled content does NOT live in this product repo.
Instead, this product repo ships only the generic adapter contract,
the `deploy/_example/target-skeleton`, and the strict
`DEPLOY_TARGETS_ROOT` resolution rule in
[`scripts/commands/deploy_target.sh`](../scripts/commands/deploy_target.sh).

The operator-coupled adapter implementations live in the operator-private
deploy-adapter overlay repo alongside any host-specific topology they bind.
Today the home-lab target is the canonical example.

### Operator verification step

Before invoking `./smackerel.sh deploy-target <target> apply` for an
operator-coupled target, the operator MUST:

1. Confirm the deploy-adapter overlay's adapter-readiness artifact for
  the target is shipped and certified (see that overlay's own README for
  the canonical path).
2. Confirm `DEPLOY_TARGETS_ROOT` is set in the operator's environment
   per the resolution rule in
   [`scripts/commands/deploy_target.sh`](../scripts/commands/deploy_target.sh)
   so the dispatcher resolves the out-of-tree adapter without silent
   fallback.
3. Confirm the Generic Pre-Apply Prerequisites below are satisfied;
  the deploy-adapter overlay MUST NOT mask or work around any of them.

The verification step is described generically here. This product
repo names no real hostnames, no real IPs, no real tailnet
identifiers, no real reverse-proxy site files, no real systemd unit
names, and no real secret values. The deploy-adapter overlay owns those.

---

## Adapter contract (per bubbles G074)

Each per-target adapter's `apply.sh` MUST (regardless of whether the
adapter lives in-tree under `deploy/<target>/` or out-of-tree under
`${DEPLOY_TARGETS_ROOT}/smackerel/<target>/` per the locality rule in
[`deploy/README.md`](../deploy/README.md#adapter-locality-in-tree-vs-out-of-tree)):

1. Reject any image reference not of form `<repo>@sha256:<digest>`
2. Pull both images by digest
3. Verify cosign signature + transparency-log entry against the configured
   identity/issuer (`signing.cosignIdentity`, `signing.cosignIssuer` in `params.yaml`)
4. Pull the config bundle by `<env>-<sourceSha>` tag and verify its sha256
5. Render the target runtime env, including explicit `HOST_BIND_ADDRESS`, and
  fail before Compose if any required value is missing or still a placeholder
6. Stage the new pointer while preserving the prior pointer in
  `previousManifest`; the home-lab KNB adapter commits the pointer only after
  staged verification succeeds so late failures cannot look like a clean deploy
7. Run the rollout strategy declared in `params.yaml` (home-lab adapters usually
  declare `recreate`; `blue-green` is also available)
8. On verify failure, leave the committed pointer truthful, reconcile runtime
  state or record explicit drift, and never rebuild

Each adapter's `rollback.sh` MUST:

- Restore the `previousManifest` pointer (atomic swap)
- NEVER invoke any build step
- Fail explicitly if `previousManifest` is null (no prior release to roll back to)

---

## Adding a new deploy target

This repo ships a fully-stubbed adapter skeleton at
[`deploy/_example/target-skeleton/`](../deploy/_example/target-skeleton/).
Use it as the copy source. Choose in-tree or out-of-tree per the locality
rule in [`deploy/README.md`](../deploy/README.md#adapter-locality-in-tree-vs-out-of-tree):

```bash
# In-tree (generic, shareable, safe for public repos):
cp -R deploy/_example/target-skeleton deploy/<new-target>

# Out-of-tree (operator-coupled, public-repo-safe; e.g. how home-lab
# adapters are provided via an operator-private deploy-adapter overlay):
mkdir -p "${DEPLOY_TARGETS_ROOT}/smackerel"
cp -R deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/smackerel/<new-target>"
```

Then:

1. Edit `<adapter-root>/<new-target>/params.yaml` with target-specific knobs (rollout
   strategy, hostnames, replica counts, host paths)
2. Edit each script for target-specific differences (e.g., k8s vs docker compose)
3. Add the env name to `deploy/contract.yaml` `configBundles.environments` and to
   the matrix in `.github/workflows/build.yml`
4. The CLI auto-discovers the new target on the next `./smackerel.sh deploy-target` run
   (in-tree adapters are auto-discovered; out-of-tree adapters require
   `DEPLOY_TARGETS_ROOT` to be set per the strict resolution rule in
   [`scripts/commands/deploy_target.sh`](../scripts/commands/deploy_target.sh))

## Status delegation

`./smackerel.sh deploy-target <target> status` uses the same strict adapter
resolution rule as `apply`, `verify`, and `rollback`. When the resolved adapter
contains an executable `status.sh`, the product dispatcher transfers control to
that script so the adapter can report target-specific manifest, runtime, health,
Caddy, and contract drift without product-side target logic.

If `status.sh` is absent or not executable, the product dispatcher does not
guess target state and does not mutate anything. It prints an
`adapter status script unavailable` notice, identifies the resolved adapter
directory, and then attempts only a generic read-only Docker container summary
for the product/target Compose project. Operators should treat that fallback as
an adapter-status gap, not as deploy readiness proof.

---

## Forbidden patterns (G074)

| Pattern | Why it's blocked |
|---------|------------------|
| Mutable image tag in `manifest.yaml` (`:latest`, `:main`, branch names) | Defeats reproducibility + rollback |
| CI workflow performing `apply`/`deploy`/`ssh` | Wrong trust boundary |
| Adapter `apply.sh` invoking `docker build`, `cargo build`, `npm run build` | Defeats build-once |
| Adapter falling back to local build on registry pull failure | Defeats build-once |
| Missing cosign verification before container start | Allows unsigned/tampered images |
| Missing bundle hash verification | Allows tampered config |
| `rollback.sh` rebuilding instead of pointer-swap | Defeats fast rollback |
| Target-side bundle generation | Defeats reproducibility |
| Plaintext secrets in bundle | Use injected env vars / sealed secrets |
| Non-deterministic bundle | Two CI runs on same SHA produce different bundles |
| Two targets sharing one `manifest.yaml` | Each target owns its own pointer |

---

# Reverse-proxy and operational concerns

The remainder of this guide covers production deployment concerns: TLS termination, auth token management, Docker Compose overrides, and HTTPS requirements for webhooks and OAuth.

## Reverse Proxy Setup for TLS

This section is generic self-hosted/local-dev guidance for a product-owned
stack. It is not the KNB home-lab activation path. KNB home-lab activation uses
the out-of-tree adapter packet above, explicit `HOST_BIND_ADDRESS`, and current
adapter ports `41001` and `41002` behind host Caddy.

Smackerel services bind to `127.0.0.1` by default. For production, terminate TLS at a reverse proxy and forward to the core service on port `40001`.

**Only expose port `40001` (smackerel-core).** All other services (ML sidecar, PostgreSQL, NATS, Ollama) must remain on localhost.

### Caddy (Recommended — Automatic HTTPS)

Caddy automatically obtains and renews TLS certificates from Let's Encrypt.

1. [Install Caddy](https://caddyserver.com/docs/install)

2. Create a `Caddyfile`:

```
smackerel.example.com {
    reverse_proxy 127.0.0.1:40001

    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }
}
```

3. Start Caddy:
```bash
sudo caddy start
```

Caddy automatically:
- Obtains a Let's Encrypt certificate for your domain
- Redirects HTTP → HTTPS
- Renews certificates before expiry

### nginx + certbot

1. Install nginx and certbot:
```bash
sudo apt install nginx certbot python3-certbot-nginx
```

2. Create `/etc/nginx/sites-available/smackerel`:

```nginx
server {
    listen 80;
    server_name smackerel.example.com;

    location / {
        proxy_pass http://127.0.0.1:40001;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support (future)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

3. Enable and obtain a certificate:
```bash
sudo ln -s /etc/nginx/sites-available/smackerel /etc/nginx/sites-enabled/
sudo certbot --nginx -d smackerel.example.com
sudo systemctl reload nginx
```

Certbot configures HTTPS automatically and installs a renewal timer.

### Certificate Renewal

- **Caddy**: Automatic — no action needed.
- **certbot/nginx**: Verify the renewal timer:
  ```bash
  sudo certbot renew --dry-run
  ```

## Auth Token Generation

Generate a cryptographically secure auth token (minimum 16 characters):

```bash
openssl rand -hex 24
```

Set it in `config/smackerel.yaml`:

```yaml
runtime:
  auth_token: "your-generated-token-here"
```

**Rules:**
- Known placeholder values like `development-change-me` are rejected at startup
- Tokens shorter than 16 characters are rejected
- Token comparison uses constant-time comparison (`subtle.ConstantTimeCompare`)
- Rotate tokens by updating config, regenerating, and restarting

After changing the token:
```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

## Per-User Bearer Auth (Spec 044) — Production Posture

Spec 044 introduces a per-user PASETO v4.public bearer-auth subsystem alongside
the legacy `runtime.auth_token`. The per-environment default and the operator
runbook (key generation, bootstrap, enrollment, rotation, revocation) live in
[Operations.md](Operations.md#per-user-bearer-authentication-spec-044).
This section is the deploy-time checklist.

When deploying to a target where `auth.enabled=true` (the home-lab default;
optional per-target override for production rollouts), the deploy adapter MUST
inject the spec 044 secrets via the standard secret-injection mechanism. They
are NEVER committed in the build's per-env config bundle — the bundle treats
them as empty-string placeholders and the deploy adapter overlays the real
values at apply time.

Required `AUTH_*` env vars (target-specific):

| Env var | Source | Required when |
|---|---|---|
| `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` | `smackerel-core auth keygen` (one per target) | `auth.enabled=true` AND `runtime.environment=production` |
| `AUTH_SIGNING_ACTIVE_KEY_ID` | Operator-chosen short identifier (e.g. `key-2026-05`) | `auth.enabled=true` AND `runtime.environment=production` |
| `AUTH_AT_REST_HASHING_KEY` | `openssl rand -hex 32` (must differ from signing key) | `auth.enabled=true` AND `runtime.environment=production` |
| `AUTH_SIGNING_PRIOR_PUBLIC_KEY` | Previous active public key (hex) | Only during a key rotation overlap window |
| `AUTH_SIGNING_PRIOR_KEY_ID` | Previous active key id | Only during a key rotation overlap window |
| `AUTH_BOOTSTRAP_TOKEN` | One-shot secret (`openssl rand -hex 24`); cleared after first user enrolls | Fresh production deployment with zero enrolled users |

Pre-`apply` checklist for any target with `auth.enabled=true`:

1. Confirm the target's bundle reports the three required keys as empty
   placeholders (per `bubbles G074` — secrets MUST NOT live in the bundle).
2. Confirm the deploy adapter overlay populates `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`,
   `AUTH_SIGNING_ACTIVE_KEY_ID`, and `AUTH_AT_REST_HASHING_KEY` from the
   target's secret store before invoking the runtime.
3. For a fresh target, set `AUTH_BOOTSTRAP_TOKEN` in the overlay, run the
   bootstrap flow per Operations.md, then remove the bootstrap secret from the
   overlay and re-`apply`.
4. The runtime fails loud at startup if any required value is missing or if the
   hashing key equals the signing key (spec 044 OQ-8). Operators see explicit
   error messages naming each missing field; recovery is to populate the secret
   and re-`apply`.

Forbidden:

- Committing real `AUTH_SIGNING_*` or `AUTH_AT_REST_HASHING_KEY` values into
  `config/smackerel.yaml` or any file under `config/generated/`.
- Reusing the signing private key as the at-rest hashing key (rejected at
  startup per OQ-8).
- Leaving `AUTH_BOOTSTRAP_TOKEN` populated in the deploy overlay after the
  first user has been enrolled (the runbook clears it).

### Spec 051 Defense-In-Depth Contract

Spec 051 (`specs/051-deployment-secret-auth-contract/`) ratifies the deployment
secret contract that this section describes and adds two cross-cutting hard
gates that fire even if a future operator skips a step above:

1. **Layered secret rejection.** The dev-default Postgres password
   (`infrastructure.postgres.password: smackerel`) is refused at BOTH the SST
   loader (`scripts/commands/config.sh` for `TARGET_ENV=home-lab` and any
   future production-class target) AND the runtime (`internal/config/Validate`
   when `SMACKEREL_ENV=production`). Either layer alone protects the
   deployment; both layers together give defense-in-depth (FR-051-005,
   SCN-051-S02).
2. **Log-redaction guarantee.** Every error path that could plausibly receive
   a secret value is covered by the security-static adversarial test
   `internal/config/log_redaction_test.go`. The test seeds canary substrings
   and asserts no canary appears in any returned error. Adding a new secret
   env var to `loadAuthConfig` or `Validate` MUST extend this test to cover
   the new error path (FR-051-007, SCN-051-S03).

The canonical key names in the table above are also pinned by
`internal/config/docs_required_keys_test.go`: every name in this section MUST
appear at least once in this doc and in [Operations.md](Operations.md), and
the pre-spec-044 HMAC-based auth aliases listed in the test's
`forbiddenAuthAliases` slice MUST NOT appear anywhere. The test runs on every
`./smackerel.sh test unit` invocation and prevents silent contract drift
(FR-051-006).

### API-Consumer Migration (Scope 02)

A target that flips `auth_enabled=true` for the first time gains the per-user
`bearerAuthMiddleware` on the API hot path. Two consumer-visible changes
follow:

1. **Bearer-token transition.** API callers MUST present a per-user PASETO
   token issued via the bootstrap / enroll flow (or, when
   `auth.production_shared_token_fallback_enabled=true`, the legacy shared
   `SMACKEREL_AUTH_TOKEN`). The middleware verifies the token statelessly with
   no DB roundtrip per request, attaches the resolved `Session` to the request
   context, and returns `HTTP 401` on failure.
2. **Body-supplied actor identifiers are rejected.** In production mode, the
   photos `MintReveal`, cloud-drive `Connect`, and user-annotation create
   handlers reject any client-supplied actor identifier in the request body or
   headers (closing MIT-040-S-008, MIT-038-S-003, MIT-027-TRACE-001
   actor-source segment). See the operator-side error-code table in
   [Operations.md](Operations.md#production-body--header-actor-identity-rejection-scope-02-mit-closures).
   API consumers that previously sent `actor_id`, `owner_user_id`, or
   `actor_source` MUST be updated to omit those fields before the target flip
   — the actor identity is derived from the bearer-token claims and no
   client-supplied value can override it.

In `dev` and `test` (or in production while `auth.enabled=false`), all three
handlers continue to honor body-supplied actor identifiers and the
`X-Actor-Id` header, so existing local-dev consumers and integration fixtures
do not need to be changed before the flip.

### API-Consumer Migration (Scope 03)

Scope 03 extends per-user PASETO authentication onto the PWA, browser
extension, and Telegram bridge, plus an admin token-management UI. Each
surface has a distinct migration step for production targets where
`auth.enabled=true`.

1. **PWA users — clear browser state and re-authenticate.** Existing PWA
   sessions backed by a stored `SMACKEREL_AUTH_TOKEN` (in `localStorage` or
   the legacy cookie) MUST be cleared and the user must re-authenticate via
   the new `POST /v1/web/login` endpoint. The login handler converts a
   per-user PASETO into an `auth_token` cookie marked `HttpOnly +
   SameSite=Lax + Path=/` and (in production) `Secure`. End users who keep
   the existing token in localStorage will see authenticated requests
   continue to work unchanged in `dev` / `test`; in production once the
   shared-token fallback is disabled the cookie path is the only working
   browser auth surface. See
   [Operations.md](Operations.md#pwa-cookie-derived-sessions-v1weblogin) for
   the full request shape and cookie attribute table.

2. **Browser extension users — install per-user tokens.** The extension
   storage slot `chrome.storage.local.authToken`
   (`web/extension/background.js`) accepts EITHER a per-user PASETO
   (production) OR the legacy shared `SMACKEREL_AUTH_TOKEN` (dev/test). To
   migrate an installation:

    - On the server: mint a token for the user with `smackerel-core auth
      enroll <user-id>` (see Operations.md "CLI Surface" for the docker exec
      form).
    - On the client: open the extension popup, paste the wire token into
      the auth-token input, and click save. The popup writes the value to
      `chrome.storage.local.authToken` atomically; subsequent capture
      requests carry it as `Authorization: Bearer <token>` with no further
      code change. Operators MAY also write `chrome.storage.local.authToken`
      directly via Chrome DevTools for bulk rollouts.

3. **Operators — populate Telegram chat → user mapping before flip.** Any
   production target that intends to use the Telegram bridge with per-user
   attribution MUST populate `telegram.user_mapping` in
   `config/smackerel.yaml` (or the deploy adapter overlay's
   `TELEGRAM_USER_MAPPING` env var) before flipping `auth_enabled=true`.
   Format: `<chat_id>:<user_id>` pairs, comma-separated. Production with an
   unmapped chat drops the message at the bot's entry point (`slog.Warn` +
   no internal API call); production with empty mapping rejects all chats.
   Dev / test tolerate empty mapping. Steps:

    - Edit `telegram.user_mapping` in `config/smackerel.yaml` (or the deploy
      overlay).
    - `./smackerel.sh config generate` to refresh `<env>.env` with the new
      `TELEGRAM_USER_MAPPING` value.
    - Restart the stack so the bot reloads its mapping (the parser is
      startup-only).

4. **Admin operators — exercise the token-management UI behind admin
   bearer.** The admin token-management UI is reachable at `GET
   /admin/auth/tokens` (`internal/api/admin_ui.go`) behind
   `bearerAuthMiddleware`. The page exposes three panels — Mint a New
   User, Enrolled Users (with per-row Rotate), and Revoke a Specific
   Token — that drive the existing Scope 02 `/v1/auth/*` admin REST
   endpoints. Per the admin-scope rule (see
   [Operations.md](Operations.md#admin-token-management-ui-adminauthtokens)),
   per-user PASETO sessions do NOT yet pass `callerIsAdmin`, so admin
   mutations require either the bootstrap session or — when
   `production_shared_token_fallback_enabled=true` — the legacy shared
   token. The page itself loads under any authenticated session.

#### Telegram Per-User Attribution Wiring (F02 Scope 04 — shipped)

The library `internal/telegram/per_user_token.go` (`PerUserTokenMinter`) is
shipped, unit-tested, and integration-tested in isolation. Spec 044
Scope 04 closes the F02 deferred-finalize-blocker by wiring `MintForChat`
into the bot's outbound HTTP calls via `Bot.bearerForChat(chatID)` and
the `Bot.setBearerHeader(req, chatID)` helper
(`internal/telegram/bot.go` lines 200–254). Production wiring is
performed by `startTelegramBotIfConfigured` (`cmd/core/wiring.go`) when
`auth.enabled=true` AND `auth.signing.active_private_key` is configured;
the minter is constructed once with TTL = 5 minutes and attached via
`tgBot.SetPerUserTokenMinter(minter)` before `Start`.

Operator implication for any production Telegram deployment:

| Setting | Behavior |
|---|---|
| `auth_enabled=true` AND `production_shared_token_fallback_enabled=true` | **Working** — bot mints per-user PASETO for mapped chats; production unmapped chats are refused (no fallback to shared bearer); legacy callers presenting `runtime.auth_token` still authenticate during the transition window |
| `auth_enabled=true` AND `production_shared_token_fallback_enabled=false` (recommended default) | **Working** — bot mints per-user PASETO for mapped chats; production unmapped chats are refused; legacy callers presenting `runtime.auth_token` are rejected with 401 |

Closure evidence:
[`internal/telegram/bot_wiring_test.go`](../internal/telegram/bot_wiring_test.go)
(8 unit cases),
[`tests/integration/auth_telegram_f02_wiring_test.go`](../tests/integration/auth_telegram_f02_wiring_test.go)
(`TestF02Wiring_SetPerUserTokenMinter_HappyPath`,
`TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses`),
[`tests/integration/auth_telegram_e2e_test.go`](../tests/integration/auth_telegram_e2e_test.go)
(Scope 03 e2e). Operator deprecation sequence for the shared-token
fallback flag is documented in
[Operations.md → "Deprecation Pathway"](Operations.md#deprecation-pathway--production_shared_token_fallback_enabled).

## Go-Live Readiness Checklist (evo-x2 / home-lab)

> **Spec 082 SCOPE-082-08.** This is the single consolidated operator
> checklist for a FIRST real deployment of the Smackerel MVP to a
> production-class home-lab target (evo-x2). It ties together the secret
> prerequisites (spec 051), the bundle secret-injection contract (spec 052),
> the local-operator vs CI trust decision (spec 017), Compose profile
> enablement, backup/restore-drill sequencing, and the supervised-canary
> first apply. Work top to bottom; every gate below is fail-closed by design
> (the system refuses to start insecurely rather than starting insecurely).

### 1. Build & trust model (spec 017 — local-operator vs CI)

- [ ] Decide the build path. **CI (keyless, recommended):** the `build.yml`
      workflow produces cosign-keyless-signed images + SBOM + SLSA provenance.
      **Local-operator (no cloud CI):** run `./smackerel.sh build --target home-lab`
      which docker-builds, Trivy-gates, pushes to ghcr, signs with the operator
      cosign key, attaches an SBOM, generates the deterministic `accel`-tier
      bundle, and emits `local-build-manifest-<sha>.yaml` (`trustModel: local-operator`).
- [ ] Do **NOT** raw `docker build` + `docker compose up` on the host — that
      bypasses the signed-supply-chain contract (no cosign, no bundle hash).
- [ ] Confirm the knb home-lab adapter `preconditions.sh`/`apply.sh` accepts the
      chosen `trustModel` (the local-operator path uses an operator pubkey and
      omits SLSA; keep adapter cosign-verify ON, pointed at the operator key).

### 2. Production secrets (spec 051 — five required, fail-loud)

The deploy adapter MUST populate these in its encrypted store; the runtime
fails loud at startup if any is missing, empty, or a dev default:

- [ ] `POSTGRES_PASSWORD` — non-default (the dev literal `smackerel` is rejected for home-lab).
- [ ] `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` — PASETO v4.public signing key (`smackerel-core auth keygen`).
- [ ] `AUTH_SIGNING_ACTIVE_KEY_ID` — operator-chosen key id (e.g. `key-2026-06`).
- [ ] `AUTH_AT_REST_HASHING_KEY` — `openssl rand -hex 32`; MUST differ from the signing key.
- [ ] `AUTH_BOOTSTRAP_TOKEN` — `openssl rand -hex 24`; one-shot, cleared after first user enrolls.

### 3. Bundle secret injection (spec 052 — L2 is knb-side)

- [ ] Confirm the knb-side **L2 secret-injection adapter** PR is landed. L1
      (placeholder emit) + L3 (runtime fail-loud) are implemented in THIS repo;
      L2 (substituting `__SECRET_PLACEHOLDER__<KEY>__` from the sops/age store)
      lives in the knb adapter and is a HARD pre-go-live dependency.
- [ ] Dry-run `apply.sh` against the published bundle and assert **0 remaining
      placeholders** before the real apply.
- [ ] Confirm the adapter writes `HOST_BIND_ADDRESS`, `OLLAMA_RENDER_GID`,
      `OLLAMA_VIDEO_GID`, and all `*_CPU_LIMIT`/`*_MEMORY_LIMIT` values into
      `app.env` (Compose fails loud at substitution time if any is missing).
- [ ] Confirm the adapter injects the `model_selection` model env vars
      (`LLM_MODEL`, `OLLAMA_*_MODEL`, `AGENT_PROVIDER_*_MODEL`,
      `ASSISTANT_OPEN_KNOWLEDGE_*`, `PHOTOS_INTELLIGENCE_*_MODEL`) + the matching
      `OLLAMA_MEMORY_LIMIT` into `app.env`. The in-repo bundle ships the
      commodity base; the Go core's `validateModelEnvelopes` fails loud at
      container start if the injected model set busts the injected envelope.

### 4. Compose profile enablement (the "it deployed but X is dead" footguns)

- [ ] Ollama on evo-x2 is a **shared host daemon**, NOT an in-stack container,
      so **`--profile ollama` is NOT used on evo-x2**. The knb overlay sets
      `sharedServices.ollama: shared` with
      `base_url_host_path: http://<host>:11434`; the adapter wires
      `OLLAMA_BASE_URL` to that host daemon, the generated `home-lab.env` sets
      `ENABLE_OLLAMA=false`, and the ollama profile is absent from
      `COMPOSE_PROFILES` — no in-stack ollama container starts. The
      smackerel-ml sidecar + assistant reach the host daemon directly. (Only a
      target that BUNDLES ollama — `sharedServices.ollama: bundled` — enables
      `--profile ollama` to run an in-stack container.)
  - [ ] Confirm the **host** ollama daemon has ROCm device access (`/dev/kfd` +
        `/dev/dri`) and the correct render/video GIDs
        (`getent group render` / `getent group video`), and that the
        `params.yaml ollama.models` are present (`ollama list`). This is host
        daemon setup, not in-stack container config.
  - [ ] Keep `OLLAMA_KEEP_ALIVE` resident on the host daemon (`-1` or a
        duration ≥ 10m): the `validateModelEnvelopes` co-residence OOM guard is
        enforced only while keep-alive keeps the interactive hot-path models
        resident. With a short/zero keep-alive the sum guard relaxes and
        avoiding co-residence OOM becomes the operator's responsibility.
- [ ] `monitoring` profile — **auto-enabled on home-lab; NO manual `--profile`
      step** (DEVOPS-RDY-01). `config/smackerel.yaml`
      `environments.home-lab.observability_bundled: true` makes `config.sh`
      append `monitoring` to `COMPOSE_PROFILES` in the generated bundle env,
      and the knb adapter's `docker compose --env-file app.env up` inherits
      it — so the spec-049 Prometheus (metric scrape + in-Prometheus
      `alerts.yml` alert-rule evaluation) starts Day-1 on the zero-manual
      apply path. Matches the knb selector
      `smackerel/home-lab/params.yaml sharedServices.observability: bundled`.
      **Grafana dashboards + Alertmanager routing are NOT bundled here** —
      they come from the shared host observability stack (spec 014) when live,
      or a knb-side stand-up (spec 082 R-082-C). See *Monitoring Profile*
      below.
- [ ] `--profile searxng` — enable only if the open-knowledge agent is on.

### 5. Backup, restore-drill & promote sequencing

- [ ] Sequence the first go-live as an initial **apply** (or knb promote), NOT a
      release-train **promote**. On a fresh host no backup or restore-drill has
      ever run, so the promote gates **G112** (backup-freshness) and **G113**
      (restore-drill currency) cannot pass yet.
- [ ] After the stack is live, wire the knb backup scheduler and run ≥1 backup +
      ≥1 restore-drill (recording ledger entries) BEFORE the first promote.
- [ ] Decide off-host backup (USB/cloud) vs accepting the documented RAID5-only,
      no-offsite risk (`offsite_required: false` → G116 WARN, non-blocking).

### 6. Supervised canary first apply

- [ ] Run the first `apply` as a **supervised canary** with `verify.sh` + health
      checks + a rollback rehearsal — the full adapter → bundle → secret-injection
      → runtime path has never been exercised end-to-end against a real target.
- [ ] Validate accel inference on the host: `rocminfo` confirms the gfx target,
      `ollama ps` shows the interactive working set resident without OOM under a
      concurrent chat+OCR+synthesis load, and an accel-tier scenario returns
      within budget — BEFORE enrolling real users.
- [ ] Keep all evo-x2-specific topology/secrets in the out-of-tree knb overlay;
      never add real hostnames, IPs, tailnet identifiers, or secret values to
      this repo (pii-scan/gitleaks gates defend this).

## Docker Compose Production Overrides

Create a `docker-compose.prod.yml` for production-specific settings:

```yaml
services:
  smackerel-core:
    restart: always
    environment:
      - SMACKEREL_LOG_LEVEL=warn
    deploy:
      resources:
        limits:
          memory: 512M

  smackerel-ml:
    restart: always
    deploy:
      resources:
        limits:
          memory: 3G

  postgres:
    restart: always
    deploy:
      resources:
        limits:
          memory: 1G

  nats:
    restart: always
    deploy:
      resources:
        limits:
          memory: 512M
```

Use with the base Compose file:
```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

**Production considerations:**
- Increase PostgreSQL memory limit for larger datasets
- Increase ML sidecar memory if using larger embedding models
- Set `restart: always` so services recover from crashes
- Use Docker volumes on fast storage (SSD) for PostgreSQL data
- Back up PostgreSQL daily via the spec 048 contract (see [Operations Runbook → Backup & Restore](Operations.md#backup--restore))

### Spec 048 — Deploy Adapter Backup Contract

The Smackerel runtime owns the dump, retention, status file, and restore drill. The deploy adapter overlay owns scheduling and off-host shipping. Adapter responsibilities:

| Responsibility | Adapter contract |
|----------------|------------------|
| Daily timer / cron | Install a systemd timer (or equivalent) that invokes `./smackerel.sh backup` on the host. Cadence MUST be at least daily so the 25h `SmackerelBackupStale` alert window stays satisfied. |
| Off-host destination | Set `BACKUP_DESTINATION_URL` in `app.env` (the adapter-written env file). The destination string is opaque to Smackerel — the adapter chooses S3, BackBlaze, NFS, rclone, etc. Never commit the real URL in this repo. |
| Off-host shipping job | After `./smackerel.sh backup` succeeds, the adapter ships `${BACKUP_LOCAL_DIR}/smackerel-*.sql.gz` to `${BACKUP_DESTINATION_URL}`. Smackerel does not invoke the shipping job; the adapter chains it after the dump. |
| Restore drill cadence | The adapter SHOULD invoke `./smackerel.sh backup-restore-test` at least weekly so a silent failure to produce a restorable artifact surfaces inside the alert window. |
| Bind mounts | If the adapter pins `BACKUP_LOCAL_DIR` to an absolute path on the host (recommended in production), bind-mount that path into the `smackerel-core` container so the watcher can read the status file. |

The adapter MUST NOT override the retention slot counts (`BACKUP_RETENTION_DAILY=7`, `BACKUP_RETENTION_WEEKLY=4`) per Product Principle 9 — those are part of the product contract and changing them is a spec-level decision, not an adapter knob.

## Telegram Webhook HTTPS Requirement

Telegram Bot API requires HTTPS for webhooks. When deploying with a public domain:

1. Set up TLS via the reverse proxy (Caddy or nginx — see above)
2. Telegram will use long polling by default. Webhook mode requires an HTTPS URL:
   - The bot connects outbound to Telegram's API servers, so long polling works without HTTPS
   - If you switch to webhook mode, the callback URL **must** be HTTPS
3. Ensure your domain's TLS certificate is valid and trusted (Let's Encrypt certificates work)

The default Smackerel configuration uses long polling, which works behind a firewall without exposing any ports to the internet. Webhook mode is only needed if you require lower latency for bot responses.

## OAuth Callback URL HTTPS Requirement

OAuth2 providers (Google) require HTTPS callback URLs in production. When switching from localhost to a public domain:

1. Update `config/smackerel.yaml`:
   ```yaml
   oauth:
     google:
       redirect_url: "https://smackerel.example.com/auth/google/callback"
   ```

2. Update the authorized redirect URI in Google Cloud Console to match

3. Regenerate config and restart:
   ```bash
   ./smackerel.sh config generate
   ./smackerel.sh down && ./smackerel.sh up
   ```

**Rules:**
- Google requires HTTPS for production redirect URIs (localhost exemption only applies to `http://127.0.0.1`)
- The redirect URL in config must exactly match the URL registered in Google Cloud Console
- After updating, existing OAuth tokens remain valid — only new authorization flows use the updated URL

## Port Exposure Summary

The `40001/40002` rows below describe the product dev/self-hosted local port
allocation from `config/smackerel.yaml`. They are not active KNB home-lab
activation guidance. The current KNB Smackerel home-lab adapter uses `41001` for
core and `41002` for ML, with `HOST_BIND_ADDRESS` set explicitly by the adapter
and Compose fail-loud if it is missing.

| Port | Service | Expose via reverse proxy? |
|------|---------|--------------------------|
| 40001 | smackerel-core (API + Web UI) | **Yes** |
| 40002 | smackerel-ml (ML sidecar) | **No** — internal only |
| 42001 | PostgreSQL | **No** — internal only |
| 42002 | NATS client | **No** — internal only |
| 42003 | NATS monitoring | **No** — internal only |
| 42004 | Ollama | **No** — internal only |
| 42005 | Prometheus (monitoring profile, dev) | **No** — operator-only on dev; deploy adapter chooses overlay exposure |

## Monitoring Profile (Spec 049 + DEVOPS-RDY-01)

Smackerel ships a self-contained Prometheus monitoring stack as a Docker
Compose profile. Its activation is driven by the fail-loud SST key
`environments.<env>.observability_bundled` in `config/smackerel.yaml`
(no silent default — every environment MUST declare it, per
smackerel-no-defaults / Gate G028):

| Env | `observability_bundled` | Prometheus start | Mechanism |
|-----|-------------------------|------------------|-----------|
| `dev` | `false` | opt-in | operator passes `--profile monitoring` (below) |
| `test` | `false` | opt-in | the integration fixture enables the profile |
| `home-lab` | `true` | **Day-1, automatic** | `config.sh` appends `monitoring` to `COMPOSE_PROFILES` in the bundle env; the knb adapter's `docker compose --env-file app.env up` inherits it — **no manual `--profile` step** |

On `home-lab` the spec-049 Prometheus (metric scrape + in-Prometheus
`alerts.yml` alert-rule evaluation) therefore starts on the zero-manual
apply path, matching the knb selector
`smackerel/home-lab/params.yaml sharedServices.observability: bundled`.

**Honest scope — what is NOT bundled here.** `deploy/compose.deploy.yml`
defines ONLY the `prometheus` service; there is no `grafana` or
`alertmanager` service. So "bundled observability" today means
**Prometheus metric scrape + in-Prometheus alert-rule evaluation** — NOT
Grafana dashboards and NOT Alertmanager notification routing. Those come
from the shared host observability stack (spec 014) when live, or a
knb-side stand-up (spec 082 R-082-C).

For `dev`/`test` (or any environment with `observability_bundled: false`),
start Prometheus opt-in:

```bash
# On the deploy host (after the deploy adapter has extracted the
# bundle and written app.env)
docker compose --env-file app.env -f compose.deploy.yml \
  --profile monitoring up -d
```

### What This Repo Ships

| Artifact | Source | Purpose |
|----------|--------|---------|
| Prometheus scrape config template | `config/prometheus/prometheus.yml.tmpl` | envsubst-rendered to `config/generated/prometheus.yml` and bundled into the deploy artifact |
| Alert rules | `config/prometheus/alerts.yml` | Committed as-is, mounted read-only at `/etc/prometheus/alerts.yml` |
| Prometheus service definition | `deploy/compose.deploy.yml::services.prometheus` | Inherits spec 042 fail-loud bind + spec 045 read-only + resource envelope |
| Dashboard inventory | `docs/Operations.md::Monitoring Stack` | Names the 10 dashboards the runtime metrics support |
| Alert runbook | `docs/Operations.md::Alert Runbook` | One row per alert: name, severity, firing action |
| SST keys | `config/smackerel.yaml::monitoring.prometheus.* + environments.<env>.prometheus_*` | Single source of truth for image, port, retention, intervals |
| External image pin | `deploy/contract.yaml::externalImages[name=prometheus]` | Canonical pin list for adapter overlays. `prom/prometheus:v2.55.1` is profile-gated; only required when `--profile monitoring` is enabled. Drift between this list and `deploy/compose.deploy.yml` is locked by `internal/deploy/external_images_contract_test.go` (BUG-049-001). |

### What The Deploy Adapter Owns

Anything operator-specific is out of scope for this repo. The deploy
adapter overlay MUST provide:

- `HOST_BIND_ADDRESS` substitution value (tailnet IP or loopback)
- Reverse-proxy / TLS fronting for the Prometheus UI (if needed)
- Alertmanager configuration (receivers: Telegram, PagerDuty, email)
- Grafana provisioning (datasource config + dashboard JSON)
- Retention beyond the SST default (remote-write, long-term storage)
- Secret rotation for any Alertmanager receivers it adds

### Verifying The Profile

```bash
# After enabling the profile, confirm Prometheus is up and scraping
curl -s http://${HOST_BIND_ADDRESS}:${PROMETHEUS_HOST_PORT}/-/healthy
curl -s http://${HOST_BIND_ADDRESS}:${PROMETHEUS_HOST_PORT}/api/v1/targets \
  | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Expected: both `smackerel-core` and `smackerel-ml` targets show
# health: "up". If a target is `down`, the failure surfaces in
# Prometheus's logs (`docker logs <prometheus-container>`).
```

### Disabling The Profile

```bash
# Stop only the prometheus service, leaving the runtime stack
docker compose --env-file app.env -f compose.deploy.yml \
  --profile monitoring stop prometheus

# Or omit the --profile flag entirely on next `up` — Compose will
# only start services in the default profile set.
```

The named volume `${PROMETHEUS_VOLUME_NAME}` (e.g.
`smackerel-home-lab-prometheus-data`) survives `down`/`up` cycles
so historical metrics persist for the configured
`PROMETHEUS_RETENTION_DAYS` (default 15 days).

## Spec 064 Deployment Notes (Open-Knowledge Agent)

The open-knowledge agent ships disabled in the committed
[`config/smackerel.yaml`](../config/smackerel.yaml). Operator opt-in
requires populating the SST keys below in the per-env bundle and
redeploying. Full operator surface is in
[`docs/Operations.md`](Operations.md#open-knowledge-assistant-agent-spec-064).

### New SST Keys (Under `assistant.open_knowledge.*`)

All keys are REQUIRED at the generator boundary even when
`enabled: false` (Gate G028, smackerel-no-defaults):

| Key | Type | Secret | Notes |
|-----|------|--------|-------|
| `enabled` | bool | no | Strict `"true"` / `"false"`; rejects `"1"` / `"0"`. |
| `provider` | enum | no | `"searxng"` \| `"brave"` \| `"tavily"`. |
| `provider_endpoint` | string | no | Empty string permitted when `enabled: false`. |
| `provider_api_key` | string | **YES** | Empty permitted for `searxng`; non-empty required for `brave` / `tavily` when enabled. Operator injects via the spec 052 secret-injection path; never commit. |
| `llm_model_id` | string | no | Model id served by the ML sidecar `/llm/chat` route. |
| `llm_timeout_ms` | int | no | Per LLM roundtrip; > 0 when enabled. |
| `max_iterations` | int | no | Hard cap on planner ↔ tool cycles; > 0 when enabled. |
| `per_query_token_budget` | int | no | Per-turn token budget; > 0 when enabled. |
| `per_query_usd_budget` | float | no | Per-turn USD budget; > 0 when enabled. |
| `monthly_budget_usd` | float | no | Aggregate monthly cap; ≥ 0 (explicit). |
| `per_user_monthly_budget_usd` | float | no | Per-user monthly cap; ≥ 0. |
| `tool_allowlist` | string[] | no | Deny-by-default tool id list; non-empty when enabled. v1 set: `internal_retrieval`, `web_search`, `unit_convert`, `calculator`. |
| `web_snippet_cache_enabled` | bool | no | Strict bool. |
| `allowed_egress_hosts` | string[] | no | Application-layer egress allowlist. Provider host is implicit. Bare hostnames only; no wildcards in v1. |
| `circuit_breaker.failure_threshold` | int | no | Consecutive failures that trip Open; > 0 when enabled. |
| `circuit_breaker.open_window_seconds` | int | no | Documented Open window; > 0 when enabled. |
| `circuit_breaker.half_open_after_seconds` | int | no | Elapsed time before HalfOpen probe; > 0 when enabled. |
| `searxng.image` | string | no | Pinned upstream tag; never `:latest`. |
| `searxng.container_port` | int | no | SearxNG default listen port inside the container. |
| `searxng.secret_key` | string | **YES** | Non-empty; entrypoint substitutes into `settings.yml`. Operators MUST override the committed dev/test placeholder. |
| `searxng.base_url` | string | no | SearxNG `base_url`; in-cluster JSON API use. |

Per-env keys under `environments.<env>.*`:

| Key | Type | Notes |
|-----|------|-------|
| `searxng_enabled` | bool | Flips the `searxng` Compose profile for that env. |
| `searxng_host_port` | int | Host port for ad-hoc dev inspection (loopback bind only). |
| `searxng_bind_address` | string | Host bind address; loopback for dev/test by convention. |

### Egress Implication

Selecting `brave` or `tavily` introduces outbound HTTPS from the core
runtime to the provider's API host. The application-layer
`allowed_egress_hosts` gate enforces the host allowlist inside the
process, but operators MUST ALSO whitelist the provider host at the
network layer (firewall, egress proxy, or VPN policy) per the
spec 020 defense-in-depth posture. PKT-020-A asks spec 020 to add
wildcard support plus a network-layer firewall on top of this
application-layer gate; until then, the network-layer rule is an
operator responsibility.

### Rollback Path

To cleanly disable the subsystem in production:

1. Set `assistant.open_knowledge.enabled: false` in the source
   `config/smackerel.yaml`.
2. Re-run `./smackerel.sh config generate --env <env> --bundle
   --source-sha <sha>` to emit a new immutable bundle.
3. Promote the new bundle via
   `bash scripts/deploy/promote.sh --target <target> --build-manifest <path>`
   (the bundle hash changes; the image digests do not).
4. The subsystem disables on next start with no impact on other
   scenarios — the loader skips registration and the spec 048
   manifest re-resolves with capture-as-fallback in the slot
   open-knowledge previously occupied.

No data migration is required either direction; the subsystem holds
no persistent state of its own (per-turn `ToolResultStore` is
in-process only).

## Client Binary Lane (Spec 085 — knb spec 025 conformance)

smackerel's native Flutter Android client (`clients/mobile/assistant`) is a
first-class, immutable, digest-pinned, cosign-keyless-provenance artifact —
exactly like the two container images — under the canonical, mechanically
enforced knb spec 025 "Client Binary Release & Delivery Pattern".

### What This Repo Ships

- `deploy/contract.yaml` declares a top-level `clients:` group (android only;
  `kind: [aab, apk]`, `provenance: cosign-keyless`, `laneB: false`). iOS is
  reserved for a future spec (no `clients/mobile/assistant/ios/` app target
  exists). Drift is locked by `internal/deploy/clients_contract_test.go`.
- `.github/workflows/build.yml` `build-clients` job builds the AAB + APK **once
  per `sourceSha`** (Build-Once Deploy-Many), reproducibly (`SOURCE_DATE_EPOCH`
  = commit time + commit-derived version, never wall-clock), distribution-signs
  with the operator-private upload keystore (env-ref only), cosign-keyless
  provenance-signs each artifact (Rekor), and pushes to
  `ghcr.io/<owner>/smackerel-clients` addressed by `sha256`. It **stops at
  registry push** (no SSH/apply). `publish-build-manifest` pins the digests in
  `build-manifest-<sourceSha>.yaml` under `clients.artifacts[]` via the
  fail-closed emitter `scripts/deploy/client-manifest-clients-block.sh`.
- The distribution signing material (Android upload keystore + passwords/alias)
  is **operator-private** (GitHub Actions secrets): `ANDROID_KEYSTORE_BASE64`,
  `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`. No
  raw key file or inline password literal lives in the repo.

### Two Lanes

- **Lane A (evo-x2 self-host)** — the EXISTING knb home-lab adapter consumes the
  signed android artifact by digest (pull → cosign-verify → byte-check sha256 →
  serve → audit; rollback is a pointer-swap). smackerel never builds clients in
  an adapter and never calls the knb Lane-A lib directly.
- **Lane B (Play Store)** — CODED but **default-OFF** behind the
  `clientReleaseLaneB` flag (declared `false` in every train bundle). The
  `.github/workflows/client-release-laneb.yml` lane is `workflow_dispatch`-only
  (never auto-runs) and reads the flag from an env var with no fallback default.
  Activation = `bubbles.train` flips the flag ON in exactly one owning train AND
  the operator sets the `CLIENT_RELEASE_LANE_B` repository variable to `true`.

### Conformance Gate

smackerel's pre-push hook (`scripts/git-hooks/pre-push`) and a CI safety-net
workflow (`.github/workflows/client-binary-conformance.yml`) invoke the knb gate
`scripts/lint/client-binary-conformance.sh --repo <smackerel-root>`. A conformance
regression (e.g. removing the contract `clients:` block) is refused with the
matching `E025-CLIENT-*` code. There is no `--skip`/`--force`/`--insecure`/
`--no-verify` bypass (C3).


