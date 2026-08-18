#!/usr/bin/env bash
# capability-consumer-naming.sh — executable consumers must name AND invoke
# their capability.
#
# Capability: capability-consumer-freshness
#
# Gate G127 companion. IMP-042 SCOPE-13 / COV-15; extended to executable
# reachability by IMP-047 S-E.
#
# G127 already requires every shipped capability to declare a non-empty
# `consumers:` list whose paths exist. That check is one-directional: the ledger
# points at a script, and the script has no idea it was named. A consumer can be
# refactored until it no longer touches the capability at all and the ledger
# keeps claiming the wiring, because the file still exists.
#
# TWO CHECKS LIVE HERE, AND THE SECOND IS THE NEW ONE.
#
#   1. NAMING (unchanged). Every `.sh` listed under a shipped capability must
#      name that capability in its own text, normally a `# Capability: <id>`
#      header line, so the declaration is verifiable from both ends.
#
#   2. EXECUTABLE REACHABILITY (IMP-047 S-E). Naming and file existence are both
#      satisfied by a script that merely MENTIONS the capability in a comment.
#      That is exactly what happened: `framework-validate.sh` was recorded as a
#      consumer of two capabilities while it only SCHEDULED THEIR SELFTESTS, and
#      `compaction-discipline-guard.sh` named its owner surface only inside an
#      `echo` string. Both satisfied every shape test while nothing in
#      production consumed the capability.
#
#      So when a shipped capability declares any PRODUCTION executable consumer
#      (a `.sh` that is not itself a `*-selftest.sh`), at least one of those
#      consumers must INVOKE the owner surface: `bash`/`source`/`exec bash` on
#      the owner path, directly or through a variable that holds it. A path, a
#      comment, an echo string and a scheduled selftest are none of them an
#      invocation.
#
#      A SCHEDULER SELFTEST IS NOT A PRODUCTION CONSUMER. `X-selftest.sh` is
#      excluded from the candidate set, and a validator that only runs
#      `X-selftest.sh` never matches `X.sh`, so scheduling cannot masquerade as
#      consumption.
#
#      A capability that declares NO production executable consumer is reported
#      and counted, not failed: its consumers are agent contracts and registries,
#      which is a different claim, not a false one.
#
# Non-executable consumers (agent markdown, registries, docs) are out of scope
# for both checks: their relationship to a capability is prose, and requiring an
# id string in them would be noise rather than evidence.
#
# Usage:
#   capability-consumer-naming.sh [repo_root]
#
# Exit codes:
#   0  every executable consumer names its capability, and every capability that
#      declares a production executable consumer is really invoked by one
#   1  at least one finding
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

# invokes_surface <consumer-file> <owner-basename>
#
# True only when the owner surface appears in an INVOCATION POSITION on a line
# that is not a full-line comment. Variable indirection counts, because
# `GUARD="$SCRIPT_DIR/x.sh"` followed by `bash "$GUARD"` is the normal shape;
# a bare mention inside a string does not, because that is prose that happens to
# live in code.
invokes_surface() {
  local file="$1" owner="$2"
  [[ -f "$file" ]] || return 1
  LC_ALL=C awk -v owner="$owner" '
    function is_invocation(s, tok,   idx) {
      idx = index(s, tok)
      if (idx == 0) return 0
      return 1
    }
    /^[ \t]*#/ { next }
    {
      line = $0
      # Direct: bash/sh/source/. followed by a path ending in the owner name.
      if (line ~ ("(^|[^A-Za-z0-9_])(bash|sh|source|exec[ \t]+bash|[.])[ \t]+[\"'"'"']?[A-Za-z0-9_${}/.-]*" owner)) {
        found = 1
        next
      }
      # Indirect step 1: a variable that holds the owner path.
      if (match(line, /^[ \t]*[A-Za-z_][A-Za-z0-9_]*=/) && index(line, owner) > 0) {
        name = line
        sub(/^[ \t]*/, "", name)
        sub(/=.*$/, "", name)
        holder[name] = 1
      }
      # Indirect step 2: that variable used as a command.
      for (name in holder) {
        if (line ~ ("(^|[^A-Za-z0-9_])(bash|sh|source|exec[ \t]+bash|[.])[ \t]+[\"'"'"']?[$][{]?" name "[}]?")) found = 1
      }
    }
    END { exit(found ? 0 : 1) }
  ' "$file"
}

# Emit "<capability>\t<owner-surface>\t<consumer>" for shipped capabilities only.
# The ledger is two-space indented under a top-level key, so a single awk pass is
# enough and keeps this runnable under a minimal PATH.
pairs="$(awk '
  /^  [a-z0-9][a-z0-9-]*:[[:space:]]*$/ {
    cap = $1; sub(/:$/, "", cap); state = ""; owner = ""; in_consumers = 0; next
  }
  /^    state:[[:space:]]/ { state = $2; next }
  /^    ownerSurface:[[:space:]]/ { owner = $2; next }
  /^    consumers:[[:space:]]*$/ { in_consumers = 1; next }
  /^    [a-zA-Z]/ { in_consumers = 0; next }
  in_consumers == 1 && /^    - / {
    path = $2
    if (state == "shipped" && cap != "" && path != "") print cap "\t" (owner == "" ? "-" : owner) "\t" path
  }
' "$LEDGER")"

checked=0
findings=0
declare -A cap_owner=()
declare -A cap_candidates=()
declare -A cap_invoked=()

while IFS=$'\t' read -r cap owner consumer; do
  [[ -n "$cap" && -n "$consumer" ]] || continue
  cap_owner["$cap"]="$owner"
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

  # Executable reachability accounting. Selftests are excluded from the
  # candidate set by name: a suite that exercises a surface is not a production
  # consumer of it.
  case "$consumer" in
    *-selftest.sh) continue ;;
  esac
  [[ "$owner" == *.sh ]] || continue
  cap_candidates["$cap"]="${cap_candidates[$cap]:-0}"
  cap_candidates["$cap"]=$((cap_candidates[$cap] + 1))
  if invokes_surface "$REPO_ROOT/$consumer" "$(basename "$owner")"; then
    cap_invoked["$cap"]="$consumer"
  fi
done <<<"$pairs"

if [[ "$checked" -eq 0 ]]; then
  err "no executable consumers were parsed from the ledger"
  exit 2
fi

unreachable=0
no_executable_consumer=0
for cap in "${!cap_owner[@]}"; do
  owner="${cap_owner[$cap]}"
  [[ "$owner" == *.sh ]] || continue
  candidates="${cap_candidates[$cap]:-0}"
  if [[ "$candidates" -eq 0 ]]; then
    no_executable_consumer=$((no_executable_consumer + 1))
    info "NOTE: shipped capability '$cap' declares no production executable consumer; its consumers are contracts, not code."
    continue
  fi
  if [[ -z "${cap_invoked[$cap]:-}" ]]; then
    err "UNREACHABLE: shipped capability '$cap' declares $candidates production executable consumer(s),"
    err "             but none of them INVOKES its owner surface $owner."
    err "             A declared path, a comment, an echo string and a scheduled selftest are not invocations."
    err "             Wire a real caller, or remove the consumer entry that claims wiring it does not have."
    findings=$((findings + 1))
    unreachable=$((unreachable + 1))
  fi
done

if [[ "$findings" -gt 0 ]]; then
  err "found $findings capability-consumer finding(s) ($unreachable unreachable owner surface(s))"
  err "add a '# Capability: <id>' header, wire a real invocation, or remove the stale consumer entry"
  exit 1
fi

info "OK — all $checked executable consumer(s) name the capability they consume; every shipped capability with a production executable consumer is really invoked by one ($no_executable_consumer contract-only capability/capabilities noted)"
exit 0
