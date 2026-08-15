#!/usr/bin/env bash
# bubbles/scripts/gate-vintage-selftest.sh
#
# Hermetic selftest for gate-vintage-annotate.sh and gate-vintage-guard.sh
# (IMP-036 SCOPE-8).
#
# The load-bearing properties are the two that keep this from becoming a silent
# excuse mechanism: a gate OLDER than the spec must never be reported as
# grandfathered (case 5), and a spec with no derivable history must report
# nothing rather than guess a vintage (case 7).

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/gate-vintage-guard.sh"
NAME="gate-vintage-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"; [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

command -v git >/dev/null 2>&1 || { printf '%s: SKIP (git not installed)\n' "$NAME"; exit 0; }

WORK="$(mktemp -d 2>/dev/null)" || { printf '%s: cannot create temp dir\n' "$NAME" >&2; exit 1; }
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# A gate registry with one gate older than the spec and one newer.
GATES="$WORK/gates.yaml"
{
  printf 'gates:\n'
  printf '  G001:\n    since: "1.0.0"\n    sinceDate: "2020-01-01"\n    name: old_gate\n'
  printf '  G999:\n    since: "9.9.9"\n    sinceDate: "2099-01-01"\n    name: future_gate\n'
} >"$GATES"

# A git repo whose spec has a real first-commit date.
REPO="$WORK/repo"
mkdir -p "$REPO/specs/001-x"
git -C "$REPO" init -q 2>/dev/null
git -C "$REPO" config user.email t@e && git -C "$REPO" config user.name t
printf '{}\n' >"$REPO/specs/001-x/state.json"
git -C "$REPO" add -A >/dev/null 2>&1
GIT_AUTHOR_DATE="2026-01-01T00:00:00" GIT_COMMITTER_DATE="2026-01-01T00:00:00" \
  git -C "$REPO" commit -q -m init 2>/dev/null

run_guard() { BUBBLES_GATES_FILE="$GATES" bash "$GUARD" "$@" 2>&1; }

# --- 1. spec vintage is derived from git -------------------------------------
out="$(run_guard "$REPO/specs/001-x")"
if printf '%s' "$out" | grep -q 'spec opened          : 2026-01-01'; then
  ok "spec vintage derived from first commit date"
else
  bad "spec vintage derived from git" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 2. a NEWER gate is reported as out of vintage ---------------------------
out="$(run_guard "$REPO/specs/001-x" --failed "G999")"
if printf '%s' "$out" | grep -q 'FAILING but out of vintage: G999'; then
  ok "gate newer than the spec is flagged out of vintage"
else
  bad "newer gate flagged" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 3. ADVERSARIAL: an OLDER gate must NOT be excused -----------------------
# This is the case that keeps the tool from becoming a blanket amnesty.
out="$(run_guard "$REPO/specs/001-x" --failed "G001")"
if printf '%s' "$out" | grep -q 'every failing gate predates this spec' &&
  ! printf '%s' "$out" | grep -q 'G001'; then
  ok "gate older than the spec is NOT excused"
else
  bad "older gate must not be excused" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 4. mixed set reports only the newer one ---------------------------------
out="$(run_guard "$REPO/specs/001-x" --failed "G001 G999")"
if printf '%s' "$out" | grep -q 'out of vintage: G999' &&
  ! printf '%s' "$out" | grep -qE 'out of vintage:.*G001'; then
  ok "mixed failing set reports only the out-of-vintage gate"
else
  bad "mixed set filtering" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 5. a deliberate override pulls a newer gate back IN ---------------------
printf '{"gateVintageOverrides":[{"gate":"G999","owner":"o","reason":"r"}]}\n' \
  >"$REPO/specs/001-x/state.json"
out="$(run_guard "$REPO/specs/001-x" --failed "G999")"
if printf '%s' "$out" | grep -q 'every failing gate predates this spec' &&
  printf '%s' "$out" | grep -q 'deliberate overrides'; then
  ok "override re-applies a newer gate to an older spec"
else
  bad "override honoured" "$(printf '%s' "$out" | tr '\n' '|')"
fi
printf '{}\n' >"$REPO/specs/001-x/state.json"

# --- 6. no git history reports nothing rather than guessing ------------------
NOGIT="$WORK/nogit/specs/002-y"
mkdir -p "$NOGIT"
out="$(run_guard "$NOGIT" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'rather than guessing'; then
  ok "spec with no history reports nothing rather than guessing a vintage"
else
  bad "no-history spec guesses nothing" "rc=$rc: $(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 7. bypass-shaped flags are refused --------------------------------------
set +e
BUBBLES_GATES_FILE="$GATES" bash "$GUARD" "$REPO/specs/001-x" --force >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "bypass-shaped flag refused with exit 2"
else
  bad "bypass flag refused" "exit was $rc"
fi

# --- 8. json shape ------------------------------------------------------------
out="$(run_guard "$REPO/specs/001-x" --json)"
if printf '%s' "$out" | grep -q '"schemaVersion":"gate-vintage/v1"' &&
  printf '%s' "$out" | grep -q '"specOpened":"2026-01-01"'; then
  ok "json report carries schemaVersion and derived spec vintage"
else
  bad "json shape" "$out"
fi

# There is deliberately no live-registry case here. `gate-vintage-annotate.sh
# --check` derives each gate's vintage from git history, so downstream it reads
# the CONSUMING repo's history, where the gates arrived in one install commit,
# and correctly reports stale. Asserting it from this portable selftest failed
# every downstream install for being right. framework-validate already runs that
# exact command as `run_check_self_only "Gate-vintage annotation freshness"`,
# which is the only context where the answer is meaningful. (IMP-042 SCOPE-12.)

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
