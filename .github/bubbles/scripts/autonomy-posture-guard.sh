#!/usr/bin/env bash
#
# autonomy-posture-guard.sh — Gate G135.
#
# IMP-039 shipped a posture dial that is resolvable (SCOPE-1), a corrected enum
# (SCOPE-2), an opt-in `unattended` (SCOPE-3), a never-suppressible floor
# (SCOPE-4), a decision ledger (SCOPE-5) and posture persistence (SCOPE-6).
# Each is only as good as its NEXT edit: an enum value added without teaching
# the resolver, or a floor item quietly deleted, reintroduces exactly the AUT-2
# class of defect the spec existed to remove — a declaration that describes
# behavior the implementation does not have.
#
# This gate asserts the surface stays internally consistent:
#
#   1. The never-suppressible floor exists and still enumerates its items.
#   2. The workflows.yaml enum and the resolver's VALID_AUTONOMY agree exactly.
#   3. `unattended` with an unbounded budget is refused with its named code.
#   4. The resolver still refuses every bypass flag.
#
# Exit codes: 0 consistent | 1 findings (one per line) | 2 usage.
# There is NO --skip/--force/--ignore. A posture that cannot satisfy the floor
# is changed, never exempted.
#
# Portable awk only: POSIX 2-arg match/substr. Do NOT introduce the GNU 3-arg
# match($0, /re/, arr) form; BSD/macOS awk rejects it and fails silently.

set -uo pipefail

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

REPO_ROOT_ARG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/autonomy-posture-guard.sh [--repo-root <dir>]

Gate G135. Asserts the IMP-039 autonomy posture surface is internally
consistent: the floor exists, the enum and the resolver agree, `unattended`
stays bounded, and no bypass flag exists.

Options:
  --repo-root <dir>   Repo root to check (default: the repo containing this script).
  -h, --help          Print this usage and exit 0.

Exit codes: 0 consistent | 1 findings | 2 usage.
There is NO --skip/--force/--ignore bypass.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || { echo "autonomy-posture-guard: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --skip | --force | --ignore | --no-verify)
      echo "autonomy-posture-guard: $1 is not a supported flag. A posture that cannot satisfy the floor is changed, never exempted." >&2
      exit 2
      ;;
    *)
      echo "autonomy-posture-guard: unknown flag: $1" >&2
      exit 2
      ;;
  esac
done

if [[ -n "$REPO_ROOT_ARG" ]]; then
  REPO_ROOT="$REPO_ROOT_ARG"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd)"
fi

FLOOR_FILE="$REPO_ROOT/agents/bubbles_shared/critical-requirements.md"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"
RESOLVER="$REPO_ROOT/bubbles/scripts/autonomy-resolve.sh"

FINDINGS=0
finding() {
  echo "autonomy-posture-guard: $1"
  FINDINGS=$((FINDINGS + 1))
}

# --- 1. The never-suppressible floor still exists, with its items ------------
if [[ ! -f "$FLOOR_FILE" ]]; then
  finding "the floor file is missing: agents/bubbles_shared/critical-requirements.md"
elif ! grep -q '^## Autonomy Floor' "$FLOOR_FILE"; then
  finding "critical-requirements.md no longer carries an '## Autonomy Floor' section"
else
  floor_items="$(awk '
    /^## Autonomy Floor/ { inf = 1; next }
    inf && /^## / { inf = 0 }
    inf && /^[0-9]+\. / { n++ }
    END { print n + 0 }
  ' "$FLOOR_FILE")"
  if [[ "$floor_items" -lt 6 ]]; then
    finding "the Autonomy Floor enumerates $floor_items item(s); it must retain at least its 6 (anti-fabrication, destructive actions, secrets, action-node approval, pre-push, legitimate blocked)"
  fi
fi

# --- 2. The enum and the resolver agree ---------------------------------------
if [[ ! -f "$WORKFLOWS_FILE" ]]; then
  finding "bubbles/workflows.yaml is missing"
elif [[ ! -f "$RESOLVER" ]]; then
  finding "bubbles/scripts/autonomy-resolve.sh is missing; the dial would be prose-only again (AUT-1)"
else
  enum_values="$(awk '
    /^    autonomy:$/ { ina = 1; next }
    ina && /^    [a-zA-Z]/ { ina = 0 }
    ina && /^      values:$/ { inv = 1; next }
    inv && /^      [a-zA-Z]/ { inv = 0 }
    inv && /^        [a-z]+:/ {
      line = $0
      sub(/^ +/, "", line)
      sub(/:.*$/, "", line)
      print line
    }
  ' "$WORKFLOWS_FILE" | LC_ALL=C sort | tr '\n' ' ')"

  resolver_values="$(sed -n 's/^VALID_AUTONOMY="\(.*\)"$/\1/p' "$RESOLVER" |
    tr ' ' '\n' | grep -v '^$' | LC_ALL=C sort | tr '\n' ' ')"

  if [[ -z "$enum_values" ]]; then
    finding "could not read any autonomy values from workflows.yaml"
  elif [[ -z "$resolver_values" ]]; then
    finding "could not read VALID_AUTONOMY from autonomy-resolve.sh"
  elif [[ "$enum_values" != "$resolver_values" ]]; then
    finding "enum/resolver drift — workflows.yaml declares [${enum_values% }] but the resolver accepts [${resolver_values% }]"
  fi
fi

# --- 3. `unattended` stays bounded, and says so by name -----------------------
if [[ -x "$RESOLVER" || -f "$RESOLVER" ]]; then
  unbounded_out="$(bash "$RESOLVER" --autonomy unattended --session-budget unbounded 2>&1 >/dev/null)"
  unbounded_rc=$?
  if [[ "$unbounded_rc" -ne 3 ]]; then
    finding "unattended with an unbounded budget exited $unbounded_rc; it must be refused with exit 3"
  fi
  if ! printf '%s' "$unbounded_out" | grep -q 'E039-UNATTENDED-UNBOUNDED'; then
    finding "the unbounded refusal no longer names E039-UNATTENDED-UNBOUNDED; an unnamed refusal is not actionable"
  fi

  bounded_rc=0
  bash "$RESOLVER" --autonomy unattended --session-budget bounded >/dev/null 2>&1 || bounded_rc=$?
  if [[ "$bounded_rc" -ne 0 ]]; then
    finding "unattended with a bounded budget exited $bounded_rc; it must resolve"
  fi
fi

# --- 4. No bypass flag survives on the resolver -------------------------------
if [[ -f "$RESOLVER" ]]; then
  for flag in --skip --force --ignore --no-verify; do
    rc=0
    bash "$RESOLVER" "$flag" >/dev/null 2>&1 || rc=$?
    if [[ "$rc" -ne 2 ]]; then
      finding "the resolver accepted bypass flag $flag (exit $rc); the posture must never waive verification"
    fi
  done
fi

if [[ "$FINDINGS" -eq 0 ]]; then
  echo "autonomy-posture-guard: OK — floor intact, enum and resolver agree, unattended stays bounded, no bypass."
  exit 0
fi

echo "autonomy-posture-guard: $FINDINGS finding(s)." >&2
exit 1
