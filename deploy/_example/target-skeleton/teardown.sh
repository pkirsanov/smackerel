#!/usr/bin/env bash
# teardown.sh — remove ONLY what bootstrap.sh and apply.sh created.
#
# Contract (NON-NEGOTIABLE):
#   - MUST NOT touch host singletons (Caddyfile main, daemon.json, default ufw
#     policy, hostname, system DNS).
#   - MUST be safe to run when nothing was deployed (no-op, exit 0).
#   - MUST leave peer adapters' resources on the same host untouched.
#   - After teardown.sh, running bootstrap.sh again MUST succeed without
#     manual intervention.
#
# See: .github/instructions/bubbles-deployment-target.instructions.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034
PARAMS_FILE="$SCRIPT_DIR/params.yaml"

# TODO(operator): implement precise teardown:
#   1. Stop and remove namespaced containers (params.runtime.composeProject)
#   2. Remove namespaced docker network (params.runtime.network)
#   3. Remove namespaced docker volumes matching params.runtime.volumePrefix*
#   4. Disable + remove namespaced systemd units matching
#      params.runtime.systemdUnitPrefix*
#   5. Remove tagged firewall rules carrying params.firewall.ruleTag
#   6. Remove this adapter's reverse-proxy drop-in file from
#      params.reverseProxy.dropInDir
#   7. Reload the reverse proxy (drop-in only, no main config touched)
# Do NOT:
#   - `docker system prune -a` (destroys peer adapters' images)
#   - rm -rf the proxy's main config dir
#   - flush ufw default policy
#   - change hostname / DNS

cat >&2 <<'EOM'
teardown.sh in deploy/_example/target-skeleton/ is a stub.

This skeleton intentionally fails so that copying it without implementing the
precise removal logic cannot accidentally wipe peer adapters' state or host
singletons.

Next step:
  cp -r deploy/_example/target-skeleton deploy/<your-target>
  # ...or, for operator-coupled targets in a public repo:
  cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"

Then implement the steps listed in the TODO(operator) block.
EOM
exit 1
