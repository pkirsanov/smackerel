#!/usr/bin/env bash
#
# bundle-cost-report-selftest.sh — hermetic selftest for bundle-cost-report.sh
#
# Proves the reporter classifies role targets correctly, computes the transitive
# closure (not just the agent file), weights the cost proxy by observed
# dispatches, and NEVER mutates the agent tree it measures.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/bundle-cost-report.sh"
PASS=0
FAIL=0

if ! command -v python3 >/dev/null 2>&1; then
  echo "bundle-cost-report-selftest: SKIP (python3 not installed)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

ok() {
  PASS=$((PASS + 1))
  echo "  PASS: $1"
}
bad() {
  FAIL=$((FAIL + 1))
  echo "  FAIL: $1"
}

# ---- fixture -------------------------------------------------------------
mk_repo() {
  local root="$1"
  rm -rf "$root"
  mkdir -p "$root/agents/bubbles_shared"
  printf 'x%.0s' $(seq 1 5000) >"$root/agents/bubbles_shared/heavy.md"
  printf 'y%.0s' $(seq 1 100) >"$root/agents/bubbles_shared/light.md"
}

REPO="$WORK/repo"
mk_repo "$REPO"

# An orchestrator that references the heavy shared module.
{
  echo "# workflow"
  echo "See bubbles_shared/heavy.md for reference."
} >"$REPO/agents/bubbles.workflow.agent.md"

# A specialist that references only the light module.
{
  echo "# docs"
  echo "See bubbles_shared/light.md."
} >"$REPO/agents/bubbles.docs.agent.md"

run_json() {
  bash "$GUARD" --repo-root "$REPO" --json 2>/dev/null
}

# ---- Case 1: runs clean and emits valid JSON ------------------------------
OUT="$(run_json)"
RC=$?
if [[ $RC -eq 0 ]]; then
  ok "exits 0 (advisory reporter never blocks)"
else
  bad "expected exit 0, got $RC"
fi
if echo "$OUT" | python3 -c 'import json,sys; json.load(sys.stdin)' 2>/dev/null; then
  ok "emits valid JSON under --json"
else
  bad "JSON output did not parse"
fi

field() {
  echo "$OUT" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for a in d['agents']:
    if a['agent']=='$1':
        print(a['$2']); break
" 2>/dev/null
}

# ---- Case 2: role classification -----------------------------------------
if [[ "$(field bubbles.workflow role)" == "orchestrator" ]]; then
  ok "workflow classified as orchestrator"
else
  bad "workflow role misclassified"
fi
if [[ "$(field bubbles.docs role)" == "specialist" ]]; then
  ok "docs classified as specialist"
else
  bad "docs role misclassified"
fi

# ---- Case 3: orchestrator target is the strictest ------------------------
WT="$(field bubbles.workflow targetBytes)"
DT="$(field bubbles.docs targetBytes)"
if [[ "$WT" -lt "$DT" ]]; then
  ok "orchestrator target ($WT) stricter than specialist ($DT)"
else
  bad "orchestrator target not stricter than specialist"
fi

# ---- Case 4: closure includes referenced modules, not just the agent -----
WB="$(field bubbles.workflow bytes)"
if [[ "$WB" -gt 5000 ]]; then
  ok "closure includes the referenced heavy module ($WB bytes > 5000)"
else
  bad "closure appears to omit referenced modules (got $WB)"
fi
DB="$(field bubbles.docs bytes)"
if [[ "$DB" -lt 1000 ]]; then
  ok "light-reference agent stays small ($DB bytes)"
else
  bad "light agent unexpectedly large ($DB)"
fi

# ---- Case 5: ADVERSARIAL — a bloated agent must be reported over target --
# Regression guard: if closure walking silently broke and returned only the
# agent file's own size, this agent would fall UNDER target and the check
# would wrongly report a healthy repo.
printf 'z%.0s' $(seq 1 200000) >"$REPO/agents/bubbles_shared/huge.md"
{
  echo "# iterate"
  echo "See bubbles_shared/huge.md."
} >"$REPO/agents/bubbles.iterate.agent.md"
OUT="$(run_json)"
if [[ "$(field bubbles.iterate withinTarget)" == "False" ]]; then
  ok "adversarial: bloated orchestrator reported OVER target"
else
  bad "adversarial: bloated orchestrator NOT flagged (closure walk broken?)"
fi
OVER="$(field bubbles.iterate overBy)"
if [[ "$OVER" -gt 0 ]]; then
  ok "adversarial: overBy reports a positive distance ($OVER)"
else
  bad "adversarial: overBy was not positive"
fi

# ---- Case 6: ADVERSARIAL — under-target agent must NOT be flagged --------
# Proves the check discriminates instead of flagging everything.
if [[ "$(field bubbles.docs withinTarget)" == "True" ]]; then
  ok "adversarial: small agent correctly NOT flagged"
else
  bad "adversarial: small agent wrongly flagged (check flags everything)"
fi

# ---- Case 7: cost proxy weights by observed dispatches -------------------
BASE="$(field bubbles.docs costProxy)"
mkdir -p "$REPO/.specify/runtime"
{
  echo '{"agent":"bubbles.docs"}'
  echo '{"agent":"bubbles.docs"}'
  echo '{"agent":"bubbles.docs"}'
} >"$REPO/.specify/runtime/framework-events.jsonl"
OUT="$(run_json)"
WEIGHTED="$(field bubbles.docs costProxy)"
if [[ "$WEIGHTED" -gt "$BASE" ]]; then
  ok "cost proxy rises with observed dispatches ($BASE -> $WEIGHTED)"
else
  bad "cost proxy ignored dispatch counts ($BASE -> $WEIGHTED)"
fi
if [[ "$(field bubbles.docs dispatches)" == "3" ]]; then
  ok "dispatch count read from the runtime event log"
else
  bad "dispatch count not read correctly"
fi

# ---- Case 8: reporter never mutates the tree it measures -----------------
BEFORE="$(find "$REPO/agents" -type f -exec sha256sum {} \; | sort | sha256sum)"
bash "$GUARD" --repo-root "$REPO" >/dev/null 2>&1
AFTER="$(find "$REPO/agents" -type f -exec sha256sum {} \; | sort | sha256sum)"
if [[ "$BEFORE" == "$AFTER" ]]; then
  ok "reporter did not mutate any agent file"
else
  bad "reporter MUTATED the agent tree (must be read-only)"
fi

# ---- Case 9: missing agents dir degrades gracefully ----------------------
EMPTY="$WORK/empty"
mkdir -p "$EMPTY"
if bash "$GUARD" --repo-root "$EMPTY" 2>&1 | grep -q "SKIP"; then
  ok "SKIPs cleanly when no agents directory exists"
else
  bad "did not SKIP on a repo with no agents directory"
fi

# ---- Case 10: unknown flag is rejected -----------------------------------
if ! bash "$GUARD" --definitely-not-a-flag >/dev/null 2>&1; then
  ok "rejects an unknown flag"
else
  bad "accepted an unknown flag"
fi

# ---- Case 11: no bypass flag exists --------------------------------------
if ! grep -qE '\-\-(skip|force|ignore|no-verify|bypass)' "$GUARD"; then
  ok "exposes no --skip/--force/--ignore bypass"
else
  bad "a bypass flag is present"
fi

echo ""
echo "bundle-cost-report-selftest: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]] || exit 1
