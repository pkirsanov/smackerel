#!/usr/bin/env bash
# bubbles/scripts/control-plane-config-merge-selftest.sh
#
# Capability: config-write-preservation
#
# Hermetic selftest for control-plane config preservation (IMP-044 SCOPE-3 / REG-15).
#
# WHY THIS EXISTS
# `save_control_plane_config` renders a FIXED template. Before this guard, every
# key the template did not name was destroyed on the next `policy set` -- silently,
# with a success message. That deletes operator settings and framework blocks
# alike, including the `experienceRecall` block IMP-043 asks operators to add.
#
# Case 2 is the load-bearing one: it disables the merge and requires the keys to
# DISAPPEAR. Without it, this test would pass on a build where the merge does
# nothing at all.
#
# Usage: bash bubbles/scripts/control-plane-config-merge-selftest.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="control-plane-config-merge-selftest"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

command -v python3 >/dev/null 2>&1 || {
  printf '%s: SKIP (python3 not installed)\n' "$NAME"
  exit 0
}
command -v git >/dev/null 2>&1 || {
  printf '%s: SKIP (git not installed)\n' "$NAME"
  exit 0
}
[[ -f "$SCRIPT_DIR/cli.sh" ]] || {
  printf '%s: SKIP (cli.sh not found)\n' "$NAME"
  exit 0
}

WORK="$(cd "$(mktemp -d)" && pwd -P)" || {
  printf '%s: cannot create temp dir\n' "$NAME" >&2
  exit 1
}
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

seed_config() {
  cat >"$1/.specify/memory/bubbles.config.json" <<'CFGEOF'
{
  "version": 2,
  "adoptionProfile": "delivery",
  "experienceRecall": { "adapter": "local-lexical" },
  "operatorCustom": { "keep": "me" },
  "defaults": { "grill": { "mode": "off", "source": "repo-default" } }
}
CFGEOF
}

build_repo() {
  local root="$1"
  mkdir -p "$root/.specify/memory"
  cp -R "$REPO_ROOT/bubbles" "$root/bubbles"
  (cd "$root" && git init -q .)
  seed_config "$root"
}

has_key() {
  KEYFILE="$1" KEYNAME="$2" python3 -c '
import json, os, sys
try:
    with open(os.environ["KEYFILE"]) as fh:
        doc = json.load(fh)
except Exception:
    sys.exit(2)
sys.exit(0 if os.environ["KEYNAME"] in doc else 1)
'
}

grill_mode() {
  KEYFILE="$1" python3 -c '
import json, os
with open(os.environ["KEYFILE"]) as fh:
    doc = json.load(fh)
print(doc.get("defaults", {}).get("grill", {}).get("mode", "<absent>"))
' 2>/dev/null
}

# --- 1. a real mutation applies AND unknown keys survive ---------------------
R1="$WORK/keep"
build_repo "$R1"
(cd "$R1" && bash bubbles/scripts/cli.sh policy set grill.mode on-demand >/dev/null 2>&1)
CFG1="$R1/.specify/memory/bubbles.config.json"
mode1="$(grill_mode "$CFG1")"
if [[ "$mode1" == "on-demand" ]]; then
  ok "the CLI-owned key is actually written (grill.mode = on-demand)"
else
  bad "owned key written" "grill.mode is '$mode1'"
fi

if has_key "$CFG1" experienceRecall && has_key "$CFG1" operatorCustom; then
  ok "framework and operator keys the template does not name both survive"
else
  bad "unknown keys survive" "$(cat "$CFG1" 2>/dev/null | tr '\n' ' ' | head -c 200)"
fi

# --- 2. ADVERSARIAL: disable the merge and the keys MUST disappear -----------
# This is what proves case 1 is testing the merge rather than a coincidence.
R2="$WORK/lose"
build_repo "$R2"
sed -i.bak 's/if \[\[ -f "\$CONTROL_PLANE_CONFIG" \]\]; then/if false; then/' \
  "$R2/bubbles/scripts/cli.sh"
(cd "$R2" && bash bubbles/scripts/cli.sh policy set grill.mode on-demand >/dev/null 2>&1)
CFG2="$R2/.specify/memory/bubbles.config.json"
if has_key "$CFG2" experienceRecall || has_key "$CFG2" operatorCustom; then
  bad "merge is load-bearing" "keys survived even with the merge disabled"
else
  ok "with the merge disabled the keys are destroyed (the guard is load-bearing)"
fi

# --- 3. the merged document is still valid JSON ------------------------------
# A merge that corrupts the file would be worse than the data loss it prevents.
if python3 -c "import json,sys; json.load(open('$CFG1'))" >/dev/null 2>&1; then
  ok "the merged config is still valid JSON"
else
  bad "merged config parses"
fi

# --- 4. repeated mutations stay idempotent for the unknown keys --------------
(cd "$R1" && bash bubbles/scripts/cli.sh policy set grill.mode off >/dev/null 2>&1)
(cd "$R1" && bash bubbles/scripts/cli.sh policy set tdd.mode off >/dev/null 2>&1)
if has_key "$CFG1" experienceRecall && has_key "$CFG1" operatorCustom &&
  [[ "$(grill_mode "$CFG1")" == "off" ]]; then
  ok "unknown keys survive repeated mutations, and the last write still wins"
else
  bad "repeated mutations preserve keys" "grill.mode=$(grill_mode "$CFG1")"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
