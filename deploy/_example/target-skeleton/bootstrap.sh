#!/usr/bin/env bash
# bootstrap.sh — idempotent install/upgrade of host-side dependencies.
#
# Contract:
#   - Re-running with no input changes MUST produce zero diffs and exit 0.
#   - MUST NOT overwrite host singletons (Caddyfile main, daemon.json, default
#     ufw policy). Use drop-ins / namespaced units only.
#   - Every resource created MUST be tagged so teardown.sh can remove ONLY this
#     adapter's resources without touching peer adapters or the operator's host
#     baseline.
#   - MUST NOT prompt interactively. All inputs come from params.yaml and the
#     project's deploy/contract.yaml.
#
# See: .github/instructions/bubbles-deployment-target.instructions.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034
PARAMS_FILE="$SCRIPT_DIR/params.yaml"

# TODO(operator): implement target-specific bootstrap. Examples:
#   - Install (idempotent) docker, your reverse proxy, mesh VPN client
#   - Drop a reverse-proxy site file into params.reverseProxy.dropInDir
#     (NEVER edit the proxy's main config; use `import conf.d/*` then drop in)
#   - Add tagged firewall rules using params.firewall.ruleTag
#   - Install systemd unit(s) named with params.runtime.systemdUnitPrefix
#   - Create namespaced docker network from params.runtime.network
#   - Create namespaced docker volumes with params.runtime.volumePrefix
# Validate after writing:
#   - `<your-proxy> validate` (or equivalent) MUST pass
#   - `docker info` MUST pass
#   - `ufw status verbose` MUST show this adapter's tagged rules

cat >&2 <<'EOM'
bootstrap.sh in deploy/_example/target-skeleton/ is a stub.

This skeleton intentionally fails so that copying it without implementing the
idempotent install/upgrade cannot mutate a host.

Next step:
  cp -r deploy/_example/target-skeleton deploy/<your-target>
  # ...or, for operator-coupled targets in a public repo:
  cp -r deploy/_example/target-skeleton "${DEPLOY_TARGETS_ROOT}/<project>/<your-target>"

Then implement the install/upgrade steps listed in the TODO(operator) block,
keeping every action idempotent and every resource tagged for teardown.
EOM
exit 1
