#!/usr/bin/env bash
# rollback.sh — pure pointer-swap to manifest.yaml.previousManifest.
#
# Contract (NON-NEGOTIABLE):
#   - MUST be a pointer-swap, NOT a rebuild.
#   - MUST fail-fast if previousManifest is null.
#   - MUST re-pull previous image digests + bundle from the registry if not
#     cached locally (still digest-pinned, still signature-verified).
#   - MUST run verify.sh after the swap.
#
# See: .github/instructions/bubbles-deployment-target.instructions.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034
MANIFEST_FILE="$SCRIPT_DIR/manifest.yaml"

# TODO(operator): implement pointer-swap rollback:
#   1. Read manifest.yaml
#   2. If previousManifest is null -> exit 1 with a clear "no previous manifest
#      to roll back to" error
#   3. Promote previousManifest to current; demote current to previousManifest
#   4. Re-pull the previous image digests and bundle from the registry
#      (cosign-verify them again — never trust the local cache for security)
#   5. Restart services with the previous artifacts using the rollout strategy
#      declared in params.rollout.strategy
#   6. Invoke verify.sh

cat >&2 <<'EOM'
rollback.sh in deploy/_example/target-skeleton/ is a stub.

This skeleton intentionally fails so that copying it without implementing the
pointer-swap rollback cannot accidentally rebuild on rollback.

Next step:
  cp -r deploy/_example/target-skeleton deploy/<your-target>
  # ...or, for operator-coupled targets in a public repo:
  cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"

Then implement the steps listed in the TODO(operator) block. Rollback is a
pointer-swap, never a rebuild.
EOM
exit 1
