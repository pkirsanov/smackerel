#!/usr/bin/env bash
# preconditions.sh — verify this target is ready to receive a deploy.
#
# Contract:
#   - MUST exit 0 when everything is already in order (no-op on a healthy host).
#   - MUST exit 1 with a structured, actionable error otherwise.
#   - MUST NOT mutate host state.
#
# See: .github/instructions/bubbles-deployment-target.instructions.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034
PARAMS_FILE="$SCRIPT_DIR/params.yaml"

# TODO(operator): implement target-specific readiness checks. Examples:
#   - SSH reachability to params.host.fqdn (with timeout)
#   - Required binaries on host (docker, your reverse proxy, jq, cosign)
#   - Operator-supplied secrets present (env vars / sealed secrets)
#   - Firewall ingress allowed to the reverse-proxy port
#   - Disk free / inode budget on the volume root
#   - Container runtime (docker / podman) responsive
#   - mDNS / VPN / DNS resolution for params.host.fqdn

cat >&2 <<'EOM'
preconditions.sh in deploy/_example/target-skeleton/ is a stub.

This skeleton intentionally fails so that copying it without filling in real
checks cannot accidentally pass a deploy gate.

Next step:
  cp -r deploy/_example/target-skeleton deploy/<your-target>
  # ...or, for operator-coupled targets in a public repo:
  cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"

Then implement the readiness checks listed in the TODO(operator) block above.
EOM
exit 1
