#!/usr/bin/env bash
# apply.sh — pull image(s) by digest, verify signature + attestations, mount
# the CI-published config bundle by hash, swap manifest.yaml pointer, run the
# rollout strategy declared in params.yaml.
#
# Contract (NON-NEGOTIABLE):
#   - MUST NOT build (`docker build`, `cargo build`, `npm run build`, etc.).
#   - MUST NOT resolve mutable tags at deploy time (no `:latest`, `:main`).
#   - MUST NOT fall back to local build if the registry pull fails.
#   - MUST verify cosign signature (keyless via Sigstore + Rekor) before start.
#   - MUST verify SBOM and SLSA provenance attestations exist.
#   - MUST verify the config bundle hash matches the operator-supplied value.
#   - MUST write manifest.yaml BEFORE starting any container, rotating the
#     prior `current` block into `previousManifest`.
#   - On verify failure: MUST invoke rollback.sh.
#
# See: .github/instructions/bubbles-deployment-target.instructions.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034
PARAMS_FILE="$SCRIPT_DIR/params.yaml"
# shellcheck disable=SC2034
MANIFEST_FILE="$SCRIPT_DIR/manifest.yaml"

# Expected operator-supplied flags (passed by the project's promote helper):
#   --image-<service>=sha256:<digest>   (one per service in deploy/contract.yaml)
#   --config-bundle=<env>-<sourceSha>
#   --source-sha=<sourceSha>

# TODO(operator): implement the apply pipeline:
#   1. Parse --image-* / --config-bundle / --source-sha flags
#   2. For each --image-*: cosign verify against Rekor (keyless), require SBOM
#      attestation, require SLSA provenance attestation
#   3. Pull config bundle by ref AND verify the bundle's sha256 byte-for-byte
#      against build-manifest-<sourceSha>.yaml's configBundles[*].sha256 for
#      this target's environment (BUG-047-001 / DEVOPS-HL-002). The expected
#      hash is operator-supplied via the --config-bundle-sha=<hex> flag (the
#      project's promote helper reads it from the build manifest and passes
#      it through). Refuse to mount on mismatch; refuse to mount if the flag
#      or the manifest field is empty.
#   4. Read current manifest.yaml; stash its `current` block as the new
#      `previousManifest` in memory
#   5. Write new manifest.yaml (current = new digests + bundle ref + bundle
#      sha + sourceSha + ISO-8601 timestamp; previousManifest = stashed block)
#   6. Run the rollout strategy declared in params.rollout.strategy
#   7. Invoke verify.sh; on failure invoke rollback.sh

cat >&2 <<'EOM'
apply.sh in deploy/_example/target-skeleton/ is a stub.

This skeleton intentionally fails so that copying it without implementing the
digest-pinned, signature-verified deploy pipeline cannot ship anything.

Next step:
  cp -r deploy/_example/target-skeleton deploy/<your-target>
  # ...or, for operator-coupled targets in a public repo:
  cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"

Then implement the steps listed in the TODO(operator) block. Build-once-deploy-
many means apply.sh ONLY pulls and verifies — it never builds.
EOM
exit 1
