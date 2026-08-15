#!/usr/bin/env bash
# capability-consumer-naming.sh — executable consumers must name their capability.
#
# Gate G127 companion. IMP-042 SCOPE-13 / COV-15.
#
# G127 already requires every shipped capability to declare a non-empty
# `consumers:` list whose paths exist. That check is one-directional: the ledger
# points at a script, and the script has no idea it was named. A consumer can be
# refactored until it no longer touches the capability at all and the ledger
# keeps claiming the wiring, because the file still exists.
#
# This closes the loop for EXECUTABLE consumers. Every `.sh` listed under a
# shipped capability must name that capability in its own text, normally a
# `# Capability: <id>` header line. The declaration is then verifiable from both
# ends, and deleting the last real use of a capability from a script surfaces
# here instead of silently leaving a false claim in the ledger.
#
# Non-executable consumers (agent markdown, registries, docs) are out of scope:
# their relationship to a capability is prose, and requiring an id string in
# them would be noise rather than evidence.
#
# Usage:
#   capability-consumer-naming.sh [repo_root]
#
# Exit codes:
#   0  every executable consumer of a shipped capability names it
#   1  at least one does not
#   2  the ledger is missing or unreadable

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${1:-$(cd -- "$SCRIPT_DIR/../.." && pwd)}"
LEDGER="$REPO_ROOT/bubbles/capability-ledger.yaml"
LABEL="capability-consumer-naming"

err() { printf '[%s][ERROR] %s\n' "$LABEL" "$*" >&2; }
info() { printf '[%s] %s\n' "$LABEL" "$*"; }

if [[ ! -f "$LEDGER" ]]; then
  err "capability ledger not found: bubbles/capability-ledger.yaml"
  exit 2
fi

# Emit "<capability>\t<consumer>" for shipped capabilities only. The ledger is
# two-space indented under a top-level key, so a single awk pass is enough and
# keeps this runnable under a minimal PATH.
pairs="$(awk '
  /^  [a-z0-9][a-z0-9-]*:[[:space:]]*$/ {
    cap = $1; sub(/:$/, "", cap); state = ""; in_consumers = 0; next
  }
  /^    state:[[:space:]]/ { state = $2; next }
  /^    consumers:[[:space:]]*$/ { in_consumers = 1; next }
  /^    [a-zA-Z]/ { in_consumers = 0; next }
  in_consumers == 1 && /^    - / {
    path = $2
    if (state == "shipped" && cap != "" && path != "") print cap "\t" path
  }
' "$LEDGER")"

checked=0
findings=0
while IFS=$'\t' read -r cap consumer; do
  [[ -n "$cap" && -n "$consumer" ]] || continue
  case "$consumer" in
    *.sh) ;;
    *) continue ;;
  esac
  [[ -f "$REPO_ROOT/$consumer" ]] || continue
  checked=$((checked + 1))
  if ! grep -Fq "$cap" "$REPO_ROOT/$consumer"; then
    err "$consumer is declared a consumer of '$cap' but never names it"
    findings=$((findings + 1))
  fi
done <<<"$pairs"

if [[ "$checked" -eq 0 ]]; then
  err "no executable consumers were parsed from the ledger"
  exit 2
fi

if [[ "$findings" -gt 0 ]]; then
  err "found $findings executable consumer(s) that do not name their capability"
  err "add a '# Capability: <id>' header, or remove the stale consumer entry"
  exit 1
fi

info "OK — all $checked executable consumer(s) name the capability they consume"
exit 0
