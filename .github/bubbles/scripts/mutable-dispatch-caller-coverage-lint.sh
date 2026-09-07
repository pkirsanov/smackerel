#!/usr/bin/env bash
#
# mutable-dispatch-caller-coverage-lint.sh — enumerate every command vector
# that can invoke reference-broker.sh's `dispatch` verb and fail if any
# caller other than mutable-dispatch-gateway.sh reaches it directly
# (IMP-056 SCOPE-6).
#
# WHY THIS EXISTS
# ---------------
# mutable-dispatch-gateway.sh is the ONE place that resolves the dispatch
# adapter, builds the shared broker-owned snapshot, and mints+verifies a
# mutable-dispatch-authorization/v1 record BEFORE the broker ever runs a
# child process. A script that shells out to reference-broker.sh's `dispatch`
# verb directly reopens exactly the authorization gap the gateway exists to
# close — no amount of correct code inside the gateway protects against a
# second, uncoordinated caller of the broker. This lint makes that class of
# regression a structural failure instead of a design intention that quietly
# rots.
#
# WHAT COUNTS AS A CALL
# ----------------------
# A line (or a line immediately followed by the next, to tolerate a wrapped
# argument list) that names reference-broker.sh together with a standalone
# `dispatch` token — either literally, or through a shell variable that was
# itself assigned the broker's path earlier in the same file.
#
# WHAT IS EXEMPT
# ---------------
#   - bubbles/adapters/dispatch/reference-broker.sh itself (the callee).
#   - bubbles/scripts/mutable-dispatch-gateway.sh (the one authorized caller).
#   - any file listed in bubbles/registry/broker-direct-caller-allowlist.txt,
#     each entry requiring a reason comment on the preceding line, matching
#     the selftest-denylist.txt hygiene bar. These are today's adversarial
#     tests of the broker itself (e.g. measured-budget-runtime-v2-selftest.py),
#     which must call the broker directly to test it in isolation.
#
# Exit codes: 0 clean - 1 findings - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
quiet=0

usage() {
  cat <<'EOF'
mutable-dispatch-caller-coverage-lint.sh — fail on a broker or gateway bypass

Usage:
  bash bubbles/scripts/mutable-dispatch-caller-coverage-lint.sh [--repo-root <path>] [--quiet]

Exit: 0 clean - 1 findings - 2 usage/environment error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "mutable-dispatch-caller-coverage-lint: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

scan_root="$repo_root/bubbles"
broker_rel="bubbles/adapters/dispatch/reference-broker.sh"
gateway_rel="bubbles/scripts/mutable-dispatch-gateway.sh"
self_rel="bubbles/scripts/mutable-dispatch-caller-coverage-lint.sh"
allowlist_file="$repo_root/bubbles/registry/broker-direct-caller-allowlist.txt"

if [[ ! -d "$scan_root" ]]; then
  echo "mutable-dispatch-caller-coverage-lint: SKIP (bubbles/ not found under $repo_root)"
  exit 0
fi
if [[ ! -f "$repo_root/$broker_rel" ]]; then
  echo "mutable-dispatch-caller-coverage-lint: SKIP ($broker_rel not found)"
  exit 0
fi

# --- Allow-list hygiene: every entry needs a file that exists and a reason -
declare -a allowed=()
findings=0
report() {
  printf 'FINDING: %s: %s\n' "$1" "$2"
  findings=$((findings + 1))
}

if [[ -f "$allowlist_file" ]]; then
  prev_was_comment=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^[[:space:]]*# ]]; then
      prev_was_comment=1
      continue
    fi
    if [[ -z "${line//[[:space:]]/}" ]]; then
      prev_was_comment=0
      continue
    fi
    entry="${line//[[:space:]]/}"
    allowed+=("$entry")
    if [[ ! -f "$repo_root/$entry" ]]; then
      report "allowlist-stale" "$entry is allow-listed but no such file exists (remove the entry)"
    fi
    if [[ "$prev_was_comment" -ne 1 ]]; then
      report "allowlist-unjustified" "$entry is allow-listed with no reason comment on the preceding line"
    fi
    prev_was_comment=0
  done <"$allowlist_file"
fi

is_allowed() {
  local rel="$1"
  local a
  for a in "${allowed[@]+"${allowed[@]}"}"; do
    [[ "$a" == "$rel" ]] && return 0
  done
  return 1
}

# --- Detect a `reference-broker.sh ... dispatch` call site in one file -----
#
# A finding requires the literal broker filename and a standalone `dispatch`
# token within a two-line window (tolerating a wrapped argument list), OR a
# shell variable this same file assigned to the broker's path, later invoked
# with a standalone `dispatch` token. A standalone token is bounded by
# anything other than a letter, digit, underscore or hyphen, so
# `dispatch-launch-state` and `dispatch_intent` do not match.
#
# Pure bash regex (no awk) deliberately: the default `awk` on macOS is BSD
# awk, whose `match()` has no capture-group array — that is a gawk extension
# this repo cannot assume is installed (see macos-portability-guard.sh).
is_dispatch_call() {
  local s="$1"
  # A standalone `dispatch` VERB argument, not the `adapters/dispatch/`
  # directory segment that appears in nearly every adapter path in this
  # tree. A verb argument is bounded by quote/space/paren/comma/string-edge;
  # a path segment is bounded by `/`, which is deliberately excluded from
  # both boundary classes below.
  [[ "$s" =~ (^|[[:space:]\"\'(,])dispatch($|[[:space:]\"\')\,]) ]]
}

scan_file() {
  local f="$1"
  local -a lines=()
  local -a varnames=()
  local i n line window v seen

  mapfile -t lines <"$f"
  n="${#lines[@]}"

  for ((i = 0; i < n; i++)); do
    if [[ "${lines[$i]}" =~ ([A-Za-z_][A-Za-z0-9_]*)=.*reference-broker\.sh ]]; then
      v="${BASH_REMATCH[1]}"
      seen=0
      for existing in "${varnames[@]+"${varnames[@]}"}"; do
        [[ "$existing" == "$v" ]] && seen=1 && break
      done
      [[ "$seen" -eq 0 ]] && varnames+=("$v")
    fi
  done

  for ((i = 0; i < n; i++)); do
    line="${lines[$i]}"
    window="$line"
    if ((i + 1 < n)); then
      window="$window"$'\n'"${lines[$((i + 1))]}"
    fi

    if [[ "$line" == *reference-broker.sh* ]] && is_dispatch_call "$window"; then
      printf '%s:%d: direct literal call to reference-broker.sh with a dispatch token\n' "$f" "$((i + 1))"
    fi

    for v in "${varnames[@]+"${varnames[@]}"}"; do
      if [[ "$window" =~ \$\{?$v\}? ]] && is_dispatch_call "$window"; then
        printf '%s:%d: indirect call to reference-broker.sh via $%s with a dispatch token\n' "$f" "$((i + 1))" "$v"
      fi
    done
  done
}

scanned=0
while IFS= read -r -d '' f; do
  rel="${f#"$repo_root"/}"
  [[ "$rel" == "$broker_rel" ]] && continue
  [[ "$rel" == "$gateway_rel" ]] && continue
  [[ "$rel" == "$self_rel" ]] && continue
  scanned=$((scanned + 1))
  is_allowed_flag=0
  is_allowed "$rel" && is_allowed_flag=1
  while IFS= read -r hit; do
    [[ -n "$hit" ]] || continue
    if [[ "$is_allowed_flag" -eq 1 ]]; then
      continue
    fi
    report "broker-bypass" "$hit"
  done < <(scan_file "$f")
done < <(find "$scan_root" -type f \( -name '*.sh' -o -name '*.py' \) -not -path '*/.git/*' -print0 2>/dev/null)

if [[ "$findings" -gt 0 ]]; then
  echo "[mutable-dispatch-caller-coverage-lint] FAIL — findings: $findings"
  exit 1
fi

[[ "$quiet" -eq 1 ]] || echo "[mutable-dispatch-caller-coverage-lint] OK — $scanned file(s) scanned, no broker or gateway bypass, ${#allowed[@]} allow-listed direct caller(s)"
exit 0
