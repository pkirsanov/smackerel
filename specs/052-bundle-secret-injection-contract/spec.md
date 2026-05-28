# Feature: 052 Bundle Secret Injection Contract

**Status:** Done (certified per state.json)

## Status

Not Started — analyst-bootstrapped greenfield spec. Workflow has not advanced beyond the analyze phase. Routing decision (which structural option to adopt) is owned by the operator and the design phase.

## Problem Statement

The CI workflow `.github/workflows/build.yml` builds three configuration bundles per source SHA (`dev`, `test`, `home-lab`) via `./smackerel.sh config generate --env <env> --bundle ...` so the Build-Once Deploy-Many invariant (bubbles G074) holds. Spec 051's `FR-051-005` correctly added a defense-in-depth gate at the SST loader (`scripts/commands/config.sh`) that REJECTS dev-default Postgres passwords for non-dev SST targets. The two policies collide for the `home-lab` matrix leg: CI has no real production secret to inject, so `infrastructure.postgres.password=smackerel` (the local-dev value in `config/smackerel.yaml`) flows through the loader, the gate fires, and the `build-bundles (home-lab)` job exits 1.

This was masked behind the spec 047 Trivy failure until R13 turned Trivy green (run `25881131646`, head `b14742c4`); the surfaced finding `F-047-B` in [specs/047-ci-image-vulnerability-gate/report.md](../047-ci-image-vulnerability-gate/report.md) recorded the exact failure shape and routed it here.

The architectural conflict is real and structural — both invariants are correct; the contract for how secrets cross the CI → bundle → adapter boundary is missing. Today the live system has THREE incompatible expectations:

1. **Spec 051 hard constraint:** "No literal secret values may be committed" AND "the SST loader must reject dev-default Postgres passwords for non-dev targets". Both correct.
2. **Spec 051 hard constraint:** "Target-specific secret injection remains a target adapter responsibility". Also correct — the adapter (`knb/smackerel/home-lab/apply.sh`) decrypts `knb/smackerel/secrets/home-lab.enc.env` via `sops` and supplies it as the SECOND `--env-file` to `docker compose up`, which overrides anything in the bundle's `app.env`.
3. **bubbles G074 hard constraint:** CI MUST produce one immutable, signed, OCI-published bundle per environment per source SHA, with byte-for-byte determinism, BEFORE the operator deploys; CI MUST NOT have access to real production secrets and MUST NOT SSH or call adapter scripts.

(1) and (2) are inconsistent at the moment when the home-lab bundle is FIRST GENERATED in CI — the SST loader fails before any bundle bytes exist, so (3) cannot complete either. The bundle's value is also semantically meaningless at apply time because the adapter overrides it; we are blocking on a value that is going to be discarded.

This is why IT IS a real, deeper, structural feature gap — not a one-line fix. Whatever option is chosen materially shapes how every future SST-managed secret (LLM API keys, OAuth tokens, payment tokens, mesh-VPN identity, Telegram/Discord bot tokens) crosses the same boundary.

## Outcome Contract

**Intent:** Define a single, machine-checkable contract for how SST-managed secret values cross the CI → bundle → adapter → runtime boundary, such that (a) CI can produce a deterministic per-env bundle for `home-lab` (and any future production-class target) without any real production secret being known to CI, (b) the bundle published to OCI never contains a real production secret value, (c) the deploy adapter is REQUIRED to inject real secrets at apply time and FAILS LOUD if any required secret is missing or unsubstituted, and (d) spec 051's existing dev-default rejection continues to fire for actual local-dev attempts.

**Success Signal:** Pushing to `main` causes the entire CI matrix (`dev`, `test`, `home-lab`) to go green and `publish-build-manifest` to write a `build-manifest-<sourceSha>.yaml` containing all three `configBundles[*]` entries with valid sha256 digests. An auditor can pull the published `home-lab` bundle from `ghcr.io/pkirsanov/smackerel-config-bundles:home-lab-<sourceSha>`, extract `app.env`, and observe that NO key declared as a "secret" in the SST manifest contains a real value. An operator can run `bash knb/smackerel/home-lab/apply.sh ...` against that bundle and the resulting containers boot with the real secrets sourced from `knb/smackerel/secrets/home-lab.enc.env` (via `sops`). A regression that removes the placeholder substitution OR reintroduces a real value into the published bundle is caught by an adversarial test that fails the CI run BEFORE the bundle is pushed to the registry.

**Hard Constraints:**

- The Build-Once Deploy-Many invariant (bubbles G074) MUST be preserved end-to-end. The CI matrix `[ dev, test, home-lab ]` MUST continue to build all three bundles deterministically and publish all three to OCI.
- CI MUST NOT have access to any real production secret value at any time. CI MUST NOT SSH, deploy, or call any adapter script.
- Spec 051's `FR-051-005` defense-in-depth gate MUST continue to reject the literal local-dev Postgres password (`smackerel`) when an actual operator attempts to use it for a real deployment. This spec MUST NOT weaken or bypass that gate.
- Spec 051's `FR-051-007` log-redaction guarantee MUST hold for every new error path introduced by this spec.
- Target-specific secret injection MUST remain an adapter (`knb/smackerel/home-lab/apply.sh`) responsibility. This spec MUST NOT add a real-secret-emission path to the smackerel CI workflow.
- The `auth.bootstrap_token` semantics from spec 051 `FR-051-004` (always-required-in-production until operator explicitly clears) MUST be preserved.
- The dev and test bundles MUST continue to ship usable inline values for local dev convenience (the dev/test SST surface is allowed to contain the dev-default values it always has).
- Determinism: the same `(sourceSha, env, smackerel.yaml content)` MUST produce the same bundle bytes (the same sha256). A non-deterministic placeholder (e.g., one derived from `date +%s`) is forbidden.

**Failure Condition:**

Any of the following makes this feature a failure even if all tests pass:

1. The published `home-lab` config bundle contains a real production secret value.
2. The published `home-lab` config bundle contains the literal local-dev Postgres password `smackerel`, or any other value from `internal/config/secrets.go::DevDBPasswords`.
3. The deploy adapter `knb/smackerel/home-lab/apply.sh` runs `docker compose up` against an unsubstituted bundle and Smackerel boots with a placeholder value as a real credential.
4. CI gains the ability to read a real production secret (whether via GitHub Actions secrets, OIDC-federated identity to a secret manager, or any other path).
5. A regression that disables the spec 051 dev-default rejection for actual operator workflows is silently shipped.
6. A new SST-managed secret added in a future spec is shipped without being declared in the SST secret-key manifest, causing it to leak into bundles.

## Reconciliation With Parent Finding (F-047-B)

The surfaced finding `F-047-B` in [specs/047-ci-image-vulnerability-gate/report.md](../047-ci-image-vulnerability-gate/report.md#surfaced-findings-out-of-scope-for-spec-047) recorded:

> Job 76061941351 on run 25881130287 (head_sha b14742c4) failed at step "Generate config bundle for home-lab" with exit 1. Local reproduction:
> ```
> $ TARGET_ENV=home-lab ./smackerel.sh config generate --env home-lab --bundle
> ERROR: infrastructure.postgres.password is a known dev-default value —
> refusing to generate config for TARGET_ENV=home-lab (spec 051 FR-051-005).
> ```
> Resolution requires an operator decision (define `CI_BUNDLE_POSTGRES_PASSWORD` GitHub Actions secret + env override on bundle-generate step, OR drop home-lab from the CI matrix and have the deploy adapter generate the bundle on the target). This is squarely a spec 051 follow-up and does not block spec 047 R13 close-out.

Spec 052 takes ownership of that decision and the structural fix. Spec 047 stays closed; spec 051 stays closed and is NOT reopened. The defense-in-depth gate spec 051 added is correct as written; spec 052 builds the missing bundle-secret-injection contract on top of it so the gate stops blocking CI without ever weakening the gate itself.

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Release Engineer (Operator) | Maintains the smackerel monorepo. Pushes to `main`, expects all CI matrix legs to go green and the build-manifest to publish for every commit. | Reproducible green CI for every push. Zero special action required to ship a normal change. | Push to `main`, manage GitHub Actions secrets, edit `config/smackerel.yaml`. |
| Deploy Operator | Runs `bash knb/smackerel/home-lab/apply.sh ...` from a workstation OR target host with the operator age key. Owns the encrypted secrets at rest in `knb/smackerel/secrets/home-lab.enc.env`. | Apply a CI-published bundle to the home-lab target with real secrets injected. Be able to rotate any secret without rebuilding bundles. | Decrypt `knb/smackerel/secrets/home-lab.enc.env` via `sops`, run adapter scripts, restart containers. |
| Smackerel Core Service (consumer) | The Go core process that reads `POSTGRES_PASSWORD`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, etc. at startup. Runs in production with `runtime.environment=production`. | Receive real production credentials via the env-file pipeline. Refuse to start if any required secret is missing or matches a dev-default. | Read process env vars; refuse to start otherwise. |
| Smackerel CI Workflow (Producer) | `.github/workflows/build.yml` — produces immutable signed bundles + build manifest. Runs on `ubuntu-latest`, has GHCR push rights, has cosign keyless OIDC, has Trivy. | Build every bundle in the matrix deterministically. Publish to OCI. Stop at registry push. | Push to `ghcr.io/pkirsanov/smackerel-config-bundles`, sign via cosign keyless, upload SBOM and provenance. Has NO production secret access by design. |
| Security Auditor | Pulls a published bundle by digest, extracts `app.env`, verifies it contains no real secret value. Reads `internal/config/secrets.go::DevDBPasswords` and the SST secret-key manifest to know which keys to inspect. | Prove via bundle inspection that the supply chain does not leak real secrets. | Read-only on OCI registry, source repo. |
| Future Spec Author | Writes a new spec that adds a new SST-managed secret (e.g., a new connector API key). | Be able to declare the new key as a "secret" in one place, have the entire CI → bundle → adapter → runtime pipeline route it correctly without per-key plumbing. | Edit `config/smackerel.yaml`, the SST secret-key manifest, and the new spec's artifacts. |

## Use Cases

### UC-052-001: CI builds a deterministic home-lab bundle without operator-provided secrets

- **Actor:** Smackerel CI Workflow
- **Preconditions:** A push to `main` (or a tag matching `v*`) has triggered `build.yml`. The `build-images` job has produced the core and ml image digests. No GitHub Actions secret of any kind for production credentials exists or is consulted.
- **Main Flow:**
  1. The `build-bundles` matrix leg for `env=home-lab` checks out the source at the resolved `sourceSha`.
  2. The leg invokes `./smackerel.sh config generate --env home-lab --bundle --output-dir dist/config-bundles/`.
  3. The SST loader resolves every key in `config/smackerel.yaml`. For every key declared as a "secret" in the SST secret-key manifest AND for which `TARGET_ENV` is `home-lab`, the loader emits a placeholder marker (e.g., `__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__`) in `app.env` instead of the literal yaml value.
  4. The loader validates that NO real secret value appears in the staged bundle.
  5. The bundle is packaged into `config-bundle-home-lab-<sourceSha>.tar.gz` deterministically.
- **Alternative Flows:** If any key declared as a "secret" in the SST secret-key manifest is MISSING from `config/smackerel.yaml`, the loader fails loud naming the key and pointing to the manifest entry.
- **Postconditions:** `dist/config-bundles/config-bundle-home-lab-<sourceSha>.tar.gz` exists and contains `app.env` with placeholder markers in place of every secret value. The bundle's sha256 matches a second invocation of the same command (determinism).

### UC-052-002: Adapter applies the home-lab bundle with real secrets injected at apply time

- **Actor:** Deploy Operator
- **Preconditions:** A CI run for `<sourceSha>` has published `ghcr.io/pkirsanov/smackerel-config-bundles:home-lab-<sourceSha>` and a `build-manifest-<sourceSha>.yaml` declaring the bundle's expected sha256. The operator has the age private key. `knb/smackerel/secrets/home-lab.enc.env` exists, is sops-encrypted to the operator age key, and contains a real value for every key declared as a "secret" in the SST secret-key manifest.
- **Main Flow:**
  1. Operator runs `bash knb/smackerel/home-lab/apply.sh --image-core=sha256:... --image-ml=sha256:... --config-bundle=home-lab-<sourceSha> --source-sha=<sourceSha>`.
  2. Adapter pulls the bundle by digest, verifies the sha256 against the build manifest, and verifies the cosign signature.
  3. Adapter extracts the bundle into `$COMPOSE_DIR`. Bundle's `app.env` contains placeholder markers.
  4. Adapter decrypts `knb/smackerel/secrets/home-lab.enc.env` via `sops` into a `chmod 0600` tmpfile.
  5. Adapter validates that the decrypted secrets file contains a real (non-empty, non-placeholder) value for EVERY placeholder key in `app.env`.
  6. Adapter invokes `docker compose up --env-file app.env --env-file <tmpfile>`, which causes the secrets file (second `--env-file`) to override every placeholder in `app.env`.
  7. Smackerel core boots with real production credentials. `internal/config/config.go::Validate()` and `internal/auth/startup.go::ValidateRuntimeAuthStartup` see real values, not placeholders, and pass.
- **Alternative Flows:** Step 5 fails loud if any required secret is missing OR equals a placeholder marker. Step 7 fails loud if any value is still a placeholder marker (defense-in-depth runtime check).
- **Postconditions:** Containers run with real production credentials. The adapter audit log line in `/var/log/knb-apply.log` records `secrets_decrypted=true` and `outcome=success`.

### UC-052-003: Adapter detects and refuses a bundle whose secret keys were not properly substituted

- **Actor:** Deploy Operator
- **Preconditions:** A bundle exists. The operator's secrets file is incomplete OR the bundle was tampered with to remove the placeholder markers and inject real-looking values.
- **Main Flow:**
  1. Adapter extracts the bundle.
  2. Adapter decrypts the secrets file.
  3. Adapter computes the set of "expected secret keys" from the bundle's SST secret-key manifest section.
  4. Adapter validates: for every key in the expected set, (a) the key MUST appear as a placeholder in `app.env`, AND (b) the key MUST appear with a real value in the decrypted secrets file.
  5. If either check fails, adapter exits non-zero, no container starts, and the audit log records `outcome=failure` with the offending key name (NEVER the value).
- **Postconditions:** No container starts. The operator can fix the gap and retry.

### UC-052-004: Operator rotates a production secret without rebuilding bundles

- **Actor:** Deploy Operator
- **Preconditions:** A production secret has been compromised or is due for routine rotation.
- **Main Flow:**
  1. Operator decrypts `knb/smackerel/secrets/home-lab.enc.env` via `sops`.
  2. Operator updates the value for the affected key.
  3. Operator re-encrypts via `sops -e -i`.
  4. Operator commits the encrypted file to the knb repo.
  5. Operator re-runs `apply.sh` with the SAME `--config-bundle=` argument as before.
  6. Adapter substitutes the new value at apply time.
  7. Smackerel core restarts with the new credential.
- **Postconditions:** New credential is live. No bundle was rebuilt. No CI run was triggered. The bundle's sha256 in the manifest is unchanged.

### UC-052-005: Auditor inspects a published bundle and confirms zero secret values

- **Actor:** Security Auditor
- **Preconditions:** A bundle has been published to OCI for some `<sourceSha>`.
- **Main Flow:**
  1. Auditor pulls the bundle by digest from `ghcr.io/pkirsanov/smackerel-config-bundles:home-lab-<sourceSha>`.
  2. Auditor verifies the cosign signature against the public Rekor log.
  3. Auditor extracts the bundle and reads `app.env`.
  4. Auditor reads the SST secret-key manifest (shipped with the bundle as `secret-keys.yaml` or similar) to know which keys to inspect.
  5. Auditor confirms that EVERY key in the manifest appears in `app.env` with a placeholder marker (not a real value).
  6. Auditor confirms that NO key NOT in the manifest contains a value that matches `internal/config/secrets.go::DevDBPasswords` (no accidental dev-default leakage either).
- **Postconditions:** The auditor has a documented, repeatable, signature-verified proof that the supply chain does not leak real secrets via published bundles.

## Business Scenarios

```gherkin
Scenario: BS-052-001 home-lab bundle generation succeeds in CI without operator-provided secrets
  Given the SST secret-key manifest declares POSTGRES_PASSWORD as a secret
  And TARGET_ENV=home-lab
  And no real Postgres password is set in the CI environment
  When the CI build-bundles matrix leg for env=home-lab runs ./smackerel.sh config generate --env home-lab --bundle
  Then the loader exits 0
  And dist/config-bundles/config-bundle-home-lab-<sourceSha>.tar.gz exists
  And the bundle's app.env contains a placeholder marker for POSTGRES_PASSWORD
  And the bundle's app.env contains NO literal value from internal/config/secrets.go::DevDBPasswords
  And the verify-determinism step (second generate, hash compare) reports identical sha256

Scenario: BS-052-002 published home-lab bundle does NOT contain a real production password
  Given a CI run has published config-bundle-home-lab-<sourceSha>.tar.gz to OCI
  When an auditor pulls the bundle by digest, extracts app.env, and greps for the key POSTGRES_PASSWORD
  Then the value is a placeholder marker (e.g., __SECRET_PLACEHOLDER__POSTGRES_PASSWORD__)
  And the value is NOT in internal/config/secrets.go::DevDBPasswords
  And the value is NOT a real production credential

Scenario: BS-052-003 Adapter substitutes all placeholder secrets before docker compose up
  Given a published home-lab bundle exists for sourceSha S
  And the operator has knb/smackerel/secrets/home-lab.enc.env with real values for every secret key declared in the bundle's SST secret-key manifest
  When the operator runs `bash knb/smackerel/home-lab/apply.sh --image-core=... --image-ml=... --config-bundle=home-lab-S --source-sha=S`
  Then the adapter decrypts the secrets file
  And the adapter passes app.env AND the decrypted secrets file as two --env-file args to docker compose up (in that order)
  And Smackerel core boots successfully
  And `internal/config/config.go::Validate()` does NOT fire the dev-default rejection (the real value is present, not the placeholder)

Scenario: BS-052-004 Adapter fails loud if any placeholder remains in app.env at apply time
  Given a published home-lab bundle exists
  And the operator's decrypted secrets file is missing POSTGRES_PASSWORD (the operator forgot to add it)
  When the operator runs apply.sh
  Then the adapter exits non-zero BEFORE invoking docker compose up
  And no container starts
  And the error message names "POSTGRES_PASSWORD" without printing any value
  And /var/log/knb-apply.log records outcome=failure with the missing key name

Scenario: BS-052-005 A regression that removes placeholder substitution is caught by an adversarial test
  Given the SST loader has a bug that causes it to emit the literal yaml value for POSTGRES_PASSWORD on home-lab
  When the contract test internal/deploy/bundle_secret_contract_test.go runs ./smackerel.sh config generate --env home-lab --bundle and inspects the bundle's app.env
  Then the test fails with a clear message naming POSTGRES_PASSWORD as a leaked secret
  And the regression is blocked at PR review time

Scenario: BS-052-006 spec 051 dev-default DB password rejection continues to fire for actual local-dev attempts
  Given an operator has set POSTGRES_PASSWORD=smackerel via env override (mistakenly thinking they are providing a real password)
  And TARGET_ENV=home-lab
  And the operator is running ./smackerel.sh config generate --env home-lab --bundle locally (NOT in CI)
  When the SST loader runs
  Then the spec 051 FR-051-005 dev-default rejection fires
  And the error message names "infrastructure.postgres.password" without printing the value
  And no bundle is generated

Scenario: BS-052-007 Operator rotates a secret without rebuilding the bundle
  Given Smackerel has been deployed with config-bundle=home-lab-S
  When the operator updates POSTGRES_PASSWORD in knb/smackerel/secrets/home-lab.enc.env
  And the operator re-runs apply.sh with the same --config-bundle=home-lab-S argument
  Then the adapter substitutes the new value
  And Smackerel core restarts with the new credential
  And no CI run is triggered
  And the bundle's sha256 in build-manifest-S.yaml is unchanged

Scenario: BS-052-008 A new secret added by a future spec automatically routes through the contract
  Given a future spec author adds a new key SOMETHING_API_KEY to config/smackerel.yaml AND declares it in the SST secret-key manifest
  When CI runs `./smackerel.sh config generate --env home-lab --bundle`
  Then the loader emits a placeholder marker for SOMETHING_API_KEY in app.env
  And the adapter validates that SOMETHING_API_KEY appears in the decrypted secrets file at apply time
  And no per-key plumbing was added to the CI workflow, the adapter, or the runtime
```

## Edge Cases & Adversarial Scenarios

| Edge Case | Concern | Required Behavior |
|-----------|---------|-------------------|
| Placeholder marker is itself a real password elsewhere | An attacker observes the placeholder format, sets that exact string as a real secret on a victim deployment | Placeholder format MUST be unique enough that it cannot reasonably collide (UUID-like or sourceSha-derived). Must NEVER be permitted as a real value (runtime adversarial test). |
| Operator's secrets file shipped on wrong target | Operator runs adapter on `dev` host with `home-lab` bundle (or vice versa) | Adapter MUST verify the bundle's environment label matches the secrets file's environment label. Mismatch = fail loud. |
| Partial substitution (some placeholders substituted, some not) | A bug in the secrets file means only a subset of placeholders are replaced | Adapter MUST scan `app.env` AFTER substitution and confirm zero placeholder markers remain. Defense-in-depth: runtime ALSO refuses to start if any required env var still equals a placeholder. |
| Other secret-shaped values in `smackerel.yaml` that aren't yet declared | Connector API keys, OAuth tokens, bot tokens are currently empty strings in the yaml; they ship as empty in the bundle | Spec 052 MUST declare which keys are "secrets" via the SST secret-key manifest. ALL such keys MUST be in the placeholder pipeline. Empty-string ergonomics for "feature disabled" must be preserved (a missing connector API key with `connector.enabled=false` is fine; with `connector.enabled=true` it must fail loud). |
| Dev/test bundles regressing to use placeholders | A naive implementation rewrites all bundles to use placeholders, breaking local dev | Spec 052 MUST scope the placeholder behavior to `TARGET_ENV ∈ { non-dev, non-test }` (currently just `home-lab`; future production-class targets opt in explicitly). dev/test bundles MUST continue to ship inline values for local dev convenience. |
| Determinism break via timestamp/random in placeholder | Placeholder is non-deterministic, breaking bundle byte-identity guarantee | Placeholder MUST be a static, deterministic literal (e.g., `__SECRET_PLACEHOLDER__<KEY>__`) — derivable purely from the key name. |
| Auditor cannot enumerate "secrets" without source access | The bundle ships placeholders but the auditor doesn't know which keys SHOULD be placeholders | Bundle MUST ship the SST secret-key manifest as a sibling file (e.g., `secret-keys.yaml`) so an auditor can verify completeness of the placeholder set without reading the source repo. |
| Adapter age key compromise vs CI compromise | The trust boundary is the operator workstation/age key, not CI | Bundle MUST contain ZERO real secret values regardless of CI compromise scenario. CI compromise can swap an image digest (caught by cosign verify on the adapter side) but cannot leak production secrets because they were never in CI. |

## Competitive Analysis

This is internal supply-chain infrastructure, not a customer-facing feature. The relevant "competitors" are canonical patterns from the GitOps / SLSA / supply-chain-security space.

| Pattern | How It Handles Bundle/Manifest Secrets | Smackerel Today | Spec 052 Direction |
|---------|----------------------------------------|------------------|---------------------|
| Helm + Sealed Secrets (Bitnami) | Manifest contains `SealedSecret` CRDs encrypted by an in-cluster controller key. Manifest itself can be public. | Not applicable — different shape | Aligned: bundle contains placeholders; secrets exist sealed elsewhere. |
| ArgoCD + Vault Plugin | Manifest contains `<vault:path/to/secret>` references. ArgoCD plugin substitutes at sync time. | Not applicable — different shape | Aligned: bundle contains references (placeholders); adapter substitutes at apply time. |
| Kustomize + secretGenerator | Secrets generated at apply time from source files; never in source control. | Not applicable — different shape | Aligned: secrets file (sops-encrypted) is the source; never inlined into bundle. |
| Skaffold / Cap'n Proto | Bundle contains `${ENV_VAR}` references; deploy-time substitution by env var. | Currently bundle contains LITERAL values | Aligned: shift bundle from "literal value" to "reference"; adapter resolves at apply time. |
| SLSA Level 3+ supply chain | Build artifacts contain ONLY the things the build process can produce; secrets are explicitly out-of-band. | Currently bundle inlines yaml values, including local-dev secret-shaped strings | Aligned: bundle becomes pure structural artifact; secrets injected out-of-band by adapter. |
| **Smackerel today (the outlier)** | **Bundle contains literal values from `smackerel.yaml`; SST gate refuses dev-default for production targets; CI matrix breaks because there is no place to provide a real value at build time without giving CI access to production secrets.** | **N/A** | **Spec 052 brings smackerel into alignment.** |

The industry consensus is unambiguous: **secrets are NEVER in the build artifact**. Smackerel's current architecture (literal values inlined into bundle at SST-loader time) is the structural outlier. Adopting the placeholder + adapter-substitution contract brings smackerel into alignment with every mainstream GitOps and SLSA-conformant supply chain pattern, without sacrificing the Build-Once Deploy-Many invariant or spec 051's defense-in-depth gate.

## Platform Direction & Market Trends

### Industry Trends

| Trend | Status | Relevance | Impact on Smackerel |
|-------|--------|-----------|----------------------|
| SLSA Level 3+ supply-chain security demands secrets are NOT in build artifacts | Established (SLSA v1.0 ratified 2026; major OSS projects converging) | High | Smackerel already adopts cosign + Rekor (spec 047). Bringing bundle artifact in line is the next consistent step. |
| GitOps secret-as-reference pattern (Sealed Secrets, External Secrets Operator, Vault plugin) | Growing (default in Kubernetes ecosystems; spreading to Compose/non-K8s) | High | Establishes the "placeholder in bundle, substitute at apply" pattern as table stakes. |
| OCI artifact signing extending beyond images to config bundles | Growing (cosign + ORAS standardising) | High | Smackerel already does this via spec 047. Spec 052 must preserve sign-and-publish for the placeholder bundle. |
| Reproducible builds (Bazel, Nix, source date epoch) | Established | Medium | Bundle determinism requirement carries through; placeholder substitution must be deterministic. |
| "Secrets-as-references" SST manifest pattern | Emerging (cdk8s, Pulumi, Crossplane converging on declarative secret references) | Medium | A first-class SST secret-key manifest in Smackerel positions the project to absorb new secret types without ad-hoc plumbing. |
| Mandatory SBOM + provenance for every artifact (executive orders, EU CRA) | Established (regulatory pressure increasing) | Medium | Spec 047 already covers images. Bundle SBOM/provenance is the logical next step (Smackerel does ship SBOM for bundles via cosign attest) — no new work for spec 052 but the secret-free-bundle invariant makes audit results cleaner. |

### Strategic Opportunities

| Opportunity | Type | Priority | Rationale |
|-------------|------|----------|-----------|
| Unblock CI green for `home-lab` matrix leg | Table Stakes | P0 | Currently blocking every push to main. Until this is fixed, the spec 047 vulnerability gate, the spec 049 monitoring stack, and every future CI-validated artifact ship under a partial-failure regime. |
| First-class SST secret-key manifest | Differentiator | P1 | Single source of truth for "which keys are secrets" routes ALL future secrets (LLM keys, OAuth tokens, payment tokens, mesh-VPN identity) through one contract. Each new connector spec drops one declaration into the manifest instead of adding ad-hoc plumbing across CI, adapter, and runtime. |
| Adversarial bundle-inspection test in CI | Table Stakes | P0 | Without this, any future regression that re-inlines a real value into the bundle ships silently. Spec 052 MUST land this test or it has not delivered. |
| Auditor-facing bundle inspection workflow | Differentiator | P2 | Document and ship the operator/auditor procedure for verifying a published bundle contains no secrets. Differentiates Smackerel from projects that talk about supply-chain security without making it auditable end-to-end. |
| Decouple secret rotation from bundle rebuild | Table Stakes | P0 | Operators MUST be able to rotate a credential without a CI run. The placeholder + adapter-substitution pattern delivers this for free. (Today's literal-value bundle implies that any rotation requires a new bundle and therefore a new CI run, which is operationally untenable.) |

### Recommendations

1. **Immediate (this work cycle):** Adopt the SST secret-key manifest + placeholder substitution pattern (IP-052-001). Land the adversarial bundle-inspection test in the same change set. Update `knb/smackerel/home-lab/apply.sh` to perform the substitution and the post-substitution scan.
2. **Near-term (next work cycle):** Backfill all existing secret-shaped values in `smackerel.yaml` (connector API keys, OAuth tokens, bot tokens) into the SST secret-key manifest. Today they ship as empty strings, which is harmless but inconsistent. Routing them through the manifest gives a uniform contract and prepares for future connectors that have non-optional credentials.
3. **Strategic (6+ months):** Extend the same contract to the `wanderaide`, `guesthost`, and `quantitativefinance` deploy adapters in the `knb` repo. Spec 052 establishes the pattern in smackerel; the same shape applies to every overlay product. Cross-product alignment reduces operator cognitive load and audit surface area.

## Improvement Proposals

### IP-052-001: SST-managed secret-key manifest + placeholder substitution at apply time ⭐ Recommended

- **Impact:** High
- **Effort:** L (≈ 3 scopes: SST loader emit-mode + secret-key manifest, adapter substitution + validation, runtime adversarial defense-in-depth + bundle inspection contract test)
- **Competitive Advantage:** Brings Smackerel into alignment with SLSA Level 3+ and the GitOps secrets-as-reference industry consensus. Decouples secret rotation from bundle rebuild. Provides a single SST-owned source of truth for "what is a secret" that every future spec consumes for free.
- **Actors Affected:** Release Engineer (CI green), Deploy Operator (apply.sh now substitutes), Smackerel Core Service (defense-in-depth runtime check), Auditor (bundle inspection workflow), Future Spec Author (one declaration to add a new secret).
- **Business Scenarios:** BS-052-001, BS-052-002, BS-052-003, BS-052-004, BS-052-005, BS-052-006, BS-052-007, BS-052-008
- **Sketch:**
  - New surface: `infrastructure.secret_keys` (or similar) in `config/smackerel.yaml` declaring the canonical set of secret keys (e.g., `POSTGRES_PASSWORD`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, `AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN`, plus any connector tokens currently empty).
  - New surface: `internal/config/secret_keys.go` with a Go-side mirror of the manifest plus a parallel grep-friendly mirror in `scripts/commands/config.sh` (defense-in-depth duplicate, pattern already established by spec 051's `DevDBPasswords`).
  - SST loader change: when `TARGET_ENV ∈ { home-lab, ...future production-class targets }` AND a key is in the secret-key manifest, emit `__SECRET_PLACEHOLDER__<KEY>__` instead of the literal yaml value.
  - Bundle layout change: include `secret-keys.yaml` as a sibling file in the bundle so the adapter (and auditors) can enumerate the expected secret set from the bundle alone.
  - Adapter change: `knb/smackerel/home-lab/apply.sh` reads `secret-keys.yaml` from the extracted bundle, validates that every declared key has a placeholder in `app.env` AND a real value in the decrypted secrets file, then performs the existing two-`--env-file` `docker compose up` (no change to that step).
  - Runtime change: `internal/config/config.go::Validate()` and `internal/auth/startup.go::ValidateRuntimeAuthStartup` add a defense-in-depth check that refuses to start if any env var STILL equals a placeholder marker (catches adapter regression that breaks substitution).
  - Contract test: `internal/deploy/bundle_secret_contract_test.go` runs `./smackerel.sh config generate --env home-lab --bundle` and asserts every key in the secret-key manifest appears as a placeholder in the bundle's `app.env` and zero secret-key values from `internal/config/secrets.go::DevDBPasswords` appear anywhere.

### IP-052-002: CI secret env override (quick patch) ⚠ Stop-gap only

- **Impact:** Medium (unblocks CI green) / Low (does not deliver any structural improvement)
- **Effort:** S (≈ 1 scope: add GitHub Actions secret + env override on the bundle-generate step + tests + docs)
- **Competitive Advantage:** None — perpetuates the architectural outlier.
- **Actors Affected:** Release Engineer (immediate CI green), Auditor (still sees a real-shaped value in bundle, even if it's a CI-controlled placeholder).
- **Business Scenarios:** Solves BS-052-001 partially. Does NOT solve BS-052-002 (the CI secret value DOES end up in the published bundle, even if it isn't a real production credential). Does NOT solve BS-052-007 (rotation still requires CI-secret update). Does NOT solve BS-052-008 (every new secret needs another GitHub Actions secret).
- **Sketch:**
  - Add `CI_BUNDLE_POSTGRES_PASSWORD` GitHub Actions secret with a long-random value not in `DevDBPasswords`.
  - On the `build-bundles` matrix step, set `env: { POSTGRES_PASSWORD: ${{ secrets.CI_BUNDLE_POSTGRES_PASSWORD }} }`.
  - The SST loader's `required_value` already prefers env over yaml; the dev-default rejection passes.
  - Bundle contains the CI secret value, which is overridden by the adapter at apply time (bundle value semantically meaningless).
- **Risk:** Quietly normalises "CI knows a real-shaped secret" as the workaround. Auditor cannot prove the bundle has no real secret value because the CI secret IS a value (it's just not the production value). If exfiltrated, must be treated as compromised. Future secrets each demand their own GitHub Actions secret with this same shape.
- **Recommendation:** Use only if the operator wants a same-day unblock and accepts the trade-off explicitly. Spec 052 should still pursue IP-052-001 in a separate cycle even if IP-052-002 lands first.

### IP-052-003: Drop home-lab from the CI matrix; adapter generates bundle at apply time ⚠ Compromises Build-Once Deploy-Many

- **Impact:** Medium (unblocks CI green) / Negative (compromises Build-Once Deploy-Many guarantee)
- **Effort:** M (≈ 2 scopes: remove home-lab from matrix, restructure adapter to call `./smackerel.sh config generate` on target with full smackerel CLI installed)
- **Competitive Advantage:** None — moves bundle generation OFF the trust boundary that bubbles G074 was designed to enforce.
- **Actors Affected:** Release Engineer (smaller CI matrix), Deploy Operator (now needs full smackerel CLI on target host), Auditor (cannot verify a published bundle for home-lab because none exists).
- **Business Scenarios:** Solves BS-052-001 (vacuously — there is no CI generation step to fail). Does NOT solve BS-052-002 (no published bundle to inspect). Solves BS-052-007 (bundle is regenerated on every apply with whatever secrets are present). VIOLATES bubbles G074 explicitly.
- **Risk:** Loses the cosign-signed, sha256-verified, deterministic bundle for the most security-sensitive target. Loses the operator's ability to pin to any historical sourceSha and apply the same bundle bytes. Increases the attack surface on the target host (full smackerel CLI runs in production environment).
- **Recommendation:** Reject. Listed for completeness because it appears in `F-047-B`'s "OR drop home-lab from the CI matrix" alternative; spec 052 explicitly recommends against it.

### IP-052-004: Hardening — backfill all existing connector secrets into the manifest

- **Impact:** Medium
- **Effort:** S (depends on IP-052-001 landing first; then ≈ 1 scope to inventory and route)
- **Competitive Advantage:** Eliminates the inconsistency where some secret-shaped keys (Postgres password, auth keys) are routed through a contract and others (LLM API keys, OAuth tokens, bot tokens) are shipped as empty strings.
- **Actors Affected:** Future Spec Author (one consistent contract for any new secret), Auditor (uniform inspection surface).
- **Business Scenarios:** BS-052-008
- **Sketch:** After IP-052-001 establishes the manifest pattern, file a follow-up spec to enumerate the existing empty-string secret keys (`LLM_API_KEY`, `TELEGRAM_BOT_TOKEN`, `DISCORD_BOT_TOKEN`, `HOSPITABLE_ACCESS_TOKEN`, `TWITTER_BEARER_TOKEN`, `GUESTHOST_API_KEY`, `FINANCIAL_MARKETS_FINNHUB_API_KEY`, `FINANCIAL_MARKETS_FRED_API_KEY`, `GOV_ALERTS_AIRNOW_API_KEY`, `QF_DECISIONS_CREDENTIAL_REF`) and route them all through the manifest. Many are gated by `<connector>.enabled=false` so the empty value is harmless today; after migration, an enabled connector with a missing secret fails loud at the same boundary as `POSTGRES_PASSWORD`.

### IP-052-005: Cross-product alignment (knb-wide pattern)

- **Impact:** High (cross-product) / Medium (per-product)
- **Effort:** L (≈ 3 specs across `wanderaide`, `guesthost`, `quantitativefinance`)
- **Competitive Advantage:** Same contract across all four overlay products in the `knb` ecosystem reduces operator cognitive load and audit surface. A single inspection workflow covers every product's bundle.
- **Actors Affected:** Deploy Operator (one mental model across products), Auditor (one inspection workflow).
- **Business Scenarios:** BS-052-008 generalised to all products.
- **Recommendation:** Out of scope for spec 052 (smackerel-only). File as cross-product follow-up after IP-052-001 lands and proves out the pattern.

## UI Scenario Matrix

**N/A.** This is a pure CI / build / deploy infrastructure feature with no user-facing UI surface. Operator-facing surfaces are limited to CLI invocation (`./smackerel.sh config generate`, `bash apply.sh`), CI logs, and audit-log lines — none of which constitute a UI flow.

## Non-Functional Requirements

- **Determinism:** The bundle generation MUST be byte-identical across two invocations with the same `(sourceSha, env, smackerel.yaml content, secret-key manifest)`. The placeholder format MUST be derived purely from key name (no timestamp, no random).
- **Performance:** Adapter substitution + post-substitution scan SHOULD complete in under 5 seconds on the home-lab target host (a small Ed25519 / sops decrypt + a sub-second grep over `app.env` + a sub-second startup of `docker compose up`).
- **Security:**
  - Zero real secret values in any committed file (smackerel repo OR knb repo plaintext OR any CI workflow file). Real values exist only in `knb/smackerel/secrets/home-lab.enc.env` (sops-encrypted to operator age key) and as decrypted values in a `chmod 0600` tmpfile during apply.
  - Spec 051 `FR-051-007` log-redaction MUST hold for every new error path. No placeholder marker AND no real value MAY appear in any error message; only the offending KEY name.
  - Cosign signature verification on the bundle MUST pass before extraction (already enforced; no change).
  - Bundle sha256 verification against `build-manifest-<sourceSha>.yaml` MUST pass before extraction (already enforced; no change).
- **Auditability:** Every apply MUST append a one-line audit record to `/var/log/knb-apply.log` with `secrets_decrypted=true|false`, `decryption_ms=<n>`, `secrets_sha256=<n>`, `outcome=success|failure` (already enforced; no change).
- **Operator Ergonomics:**
  - Secret rotation: updating a value in `home-lab.enc.env` and re-running `apply.sh` MUST be sufficient. NO CI run, NO bundle rebuild required.
  - Adding a new secret: a future spec author edits `config/smackerel.yaml` AND the SST secret-key manifest in one place; CI, adapter, and runtime route the new key automatically with no per-key plumbing.
- **Backwards Compatibility:**
  - Spec 051 `FR-051-005` dev-default Postgres password rejection MUST continue to fire for actual local-dev attempts (BS-052-006).
  - Spec 044 PASETO v4 (Ed25519) auth contract MUST be unchanged.
  - The dev and test bundles MUST continue to ship inline values (no behavior change for local dev workflow).
- **Defense-in-Depth (mandatory layer count):** The contract MUST be enforced at THREE layers:
  1. **SST-loader layer** (build time): emits placeholders for non-dev/test targets.
  2. **Adapter layer** (apply time): validates substitution completeness.
  3. **Runtime layer** (startup time): refuses to start if any required env var still equals a placeholder marker.

## Product Principle Alignment

This spec supports:

- **Principle 8 (Trust Through Transparency):** Makes the secret-injection contract explicit, machine-checkable, and end-to-end auditable. An operator or auditor can verify, from the published bundle alone, that no production secret was ever in the supply chain.
- **Principle 6 (Invisible By Default, Felt Not Heard):** Failure modes (missing secret, partial substitution, placeholder-as-real-value) ALL fail loud at the earliest possible boundary. No silent broken-deployment surprises.
- **Principle 5 (One Graph, Many Views) generalised to deployment:** A single SST-owned secret-key manifest is the source of truth; CI, adapter, and runtime all consume it. No parallel definitions to drift.

It also reinforces spec 051's principle alignment (defense-in-depth, no fallback secret values) by extending the gate to the bundle artifact layer that spec 051 explicitly left to the adapter.

## Non-Goals

- Building a new secret manager. The existing `sops` + `age` + `knb/smackerel/secrets/home-lab.enc.env` workflow is the source of real secret values; this spec does not replace it.
- Storing target-specific secret values in the smackerel repo (still forbidden; reinforced).
- Implementing target adapter secret injection from scratch (the adapter already does it; this spec changes WHEN the adapter intervenes — at apply time after extraction — and adds a substitution-completeness validation step).
- Changing the spec 044 PASETO v4 (Ed25519) token format, key rotation contract, or revocation propagation.
- Changing the spec 051 dev-default Postgres password rejection. The check stays exactly as written.
- Changing the spec 047 Trivy CRITICAL/HIGH vulnerability gate.
- Cross-product alignment (`wanderaide`, `guesthost`, `quantitativefinance` deploy adapters in `knb`). That is a separate cross-product effort (IP-052-005) that spec 052 enables but does not deliver.
- Reopening spec 051. Spec 051's contract is correct as written; this spec is downstream from it, not a re-litigation.
- Migrating existing connector empty-string secrets into the manifest (IP-052-004). Out of scope; deliverable in a follow-up spec.
- Changing the CI trust boundary. CI continues to have ZERO production-secret access.

## Requirements (Draft — for design phase to refine)

The following draft requirements are provided to seed the design phase. Final FR numbering and wording is owned by `bubbles.design` and `bubbles.plan`.

### Bundle Secret-Key Manifest

- **FR-052-001 (draft):** A canonical SST-owned secret-key manifest MUST exist at a single source of truth in `config/smackerel.yaml` (or a sibling YAML referenced from it) declaring the set of keys whose values must be substituted at apply time rather than inlined into the bundle. The Go-side mirror lives in `internal/config/secret_keys.go`; the shell-side mirror lives in `scripts/commands/config.sh`. Drift between the three MUST be caught by a contract test.

### CI Bundle Generation

- **FR-052-002 (draft):** When `TARGET_ENV` is non-dev/non-test (currently `home-lab`; any future production-class target by explicit declaration) AND a key appears in the secret-key manifest, the SST loader MUST emit `__SECRET_PLACEHOLDER__<KEY>__` in the generated `app.env` instead of the literal yaml value.
- **FR-052-003 (draft):** The bundle MUST include a `secret-keys.yaml` sibling file enumerating the expected secret keys for the target environment, so the adapter and auditors can verify completeness from the bundle alone.
- **FR-052-004 (draft):** Bundle generation MUST remain deterministic: the same `(sourceSha, env, smackerel.yaml content, secret-key manifest)` MUST produce the same sha256.

### Adapter Substitution

- **FR-052-005 (draft):** `knb/smackerel/home-lab/apply.sh` MUST read `secret-keys.yaml` from the extracted bundle, validate that every declared key has a placeholder in `app.env` AND a real (non-empty, non-placeholder) value in the decrypted secrets file, and fail loud on any mismatch BEFORE invoking `docker compose up`.
- **FR-052-006 (draft):** The adapter MUST scan `app.env` AFTER substitution (or equivalently, validate that the second `--env-file` correctly overrides every placeholder) and fail loud if any placeholder marker remains.

### Runtime Defense-in-Depth

- **FR-052-007 (draft):** `internal/config/config.go::Validate()` MUST refuse to start if any required env var equals a placeholder marker. This is the third-layer defense-in-depth: even if the adapter has a bug that silently passes through unsubstituted values, the runtime catches it.

### Auditability

- **FR-052-008 (draft):** A contract test (`internal/deploy/bundle_secret_contract_test.go` or equivalent) MUST run `./smackerel.sh config generate --env home-lab --bundle`, extract the result, and assert: (a) every key in the secret-key manifest appears as a placeholder in `app.env`, (b) no value from `internal/config/secrets.go::DevDBPasswords` appears anywhere in the bundle, (c) the placeholder format is the canonical `__SECRET_PLACEHOLDER__<KEY>__` shape.
- **FR-052-009 (draft):** Spec 051 `FR-051-007` log-redaction MUST extend to every new error path introduced by this spec. Adversarial test coverage MUST prove no placeholder marker AND no real value appears in any error message.

### Backwards Compatibility

- **FR-052-010 (draft):** Spec 051 `FR-051-005` dev-default Postgres password rejection MUST continue to fire for actual operator workflows (`POSTGRES_PASSWORD=smackerel` set via env override on a non-CI invocation). The dev-default rejection and the placeholder emission are different code paths and MUST coexist.
- **FR-052-011 (draft):** dev and test bundles MUST continue to ship inline values for local dev convenience. The placeholder emission is scoped to non-dev/non-test SST targets only.

## Open Questions (For Design Phase)

| OQ | Question | Owner | Routing |
|----|----------|-------|---------|
| OQ-052-01 | Should the SST secret-key manifest be a top-level YAML key (`secret_keys: [POSTGRES_PASSWORD, ...]`) or a per-section flag (`infrastructure.postgres.password.secret: true`)? | Design | bubbles.design |
| OQ-052-02 | Should the placeholder format be `__SECRET_PLACEHOLDER__<KEY>__` or include a unique-per-bundle nonce derived from `sourceSha`? Trade-off: nonce makes accidental collision impossible but couples bundle determinism to source SHA in a more visible way. | Design | bubbles.design |
| OQ-052-03 | Should the adapter substitution be done by the existing two-`--env-file` `docker compose up` (relies on Compose's own override semantics) or by an explicit pre-substitution step that produces a single `app.env.resolved` with placeholders replaced? Trade-off: explicit substitution gives the adapter scan-after surface; implicit relies on Compose. | Design | bubbles.design |
| OQ-052-04 | Should the contract test live in `internal/deploy/` (Go test surface) or as a shell test under `scripts/`? Spec 051 used both surfaces; spec 052 should pick consistently. | Design | bubbles.design |
| OQ-052-05 | When a future production-class target is added (e.g., `cloud-staging`), should it opt into placeholder emission via a manifest declaration, or should the loader default to placeholder for ANY non-{dev,test} target? Trade-off: opt-in is explicit but easier to forget; default-on is safer but couples adding a new target to the spec 052 contract. | Design | bubbles.design |
| OQ-052-06 | Should the contract additionally cover bundle-time provenance (e.g., sign the `secret-keys.yaml` shipping in the bundle so an adversarial swap is caught by signature verification rather than by content inspection)? | Design | bubbles.design |

## Dependencies & Sequencing

- **Hard dependency on:** Spec 044 (PASETO auth contract — unchanged), Spec 047 R13 (Trivy gate is green so this gap is now visible), Spec 051 (defense-in-depth gate exists and is what spec 052 builds on).
- **Hard dependency from:** Any future spec that adds a new SST-managed secret (e.g., a future connector spec that needs an API key for an enabled connector). Those specs become much smaller once spec 052 lands.
- **Touches but does not modify:** Spec 049 (monitoring stack) — uses the same bundle generation pipeline; placeholder emission must not break Prometheus / alerts files which contain no secrets.
- **Out-of-scope crossreferences:** Spec 045 (deploy resource limits), Spec 046 (NATS hardening), Spec 048 (backup automation), Spec 050 (ML sidecar isolation) — none of these touch secret values; spec 052 has no overlap with them.
