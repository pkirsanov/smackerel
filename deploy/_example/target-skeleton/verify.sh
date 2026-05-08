#!/usr/bin/env bash
# verify.sh — post-deploy health and smoke checks.
#
# Contract:
#   - Exit 0 when the deployed system is healthy end-to-end.
#   - Exit 1 with a structured error otherwise; apply.sh will trigger rollback.
#   - MUST use --max-time on every curl (no infinite waits).
#   - MUST NOT rely on cached state; re-evaluate every check on each invocation.
#
# See: .github/instructions/bubbles-deployment-target.instructions.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034
PARAMS_FILE="$SCRIPT_DIR/params.yaml"

# TODO(operator): implement post-deploy verify. Examples:
#   - HTTP 200 from each public URL the project exposes (curl --max-time 5)
#   - Container `health=healthy` for every namespaced container
#     (`docker inspect --format '{{.State.Health.Status}}' ...`)
#   - Application-level smoke: a known endpoint returns expected response shape
#   - Background workers reporting ready in their structured logs
#   - Reverse-proxy serving the new bundle (cache-busting URL or hash check)

cat >&2 <<'EOM'
verify.sh in deploy/_example/target-skeleton/ is a stub.

This skeleton intentionally fails so that copying it without implementing
real health checks cannot make a failing deploy look healthy.

Next step:
  cp -r deploy/_example/target-skeleton deploy/<your-target>
  # ...or, for operator-coupled targets in a public repo:
  cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"

Then implement the checks listed in the TODO(operator) block.
EOM
exit 1
