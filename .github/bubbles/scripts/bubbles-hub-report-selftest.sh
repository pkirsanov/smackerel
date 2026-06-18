#!/usr/bin/env bash
# File: bubbles-hub-report-selftest.sh
#
# Hermetic selftest for bubbles-hub-report.sh. Plants a tiny fixture repo with a
# KNOWN edge set (scripts that source a common script, agents that include a
# shared module, a workflows.yaml that references a gate) and proves: the degree
# ranking is correct, the --node reverse-dependency closure is exact and
# provenance-tagged, --format json is stable, exit 0 on a clean fixture, and a
# usage error exits 2 (never exit 1).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HUB="$SCRIPT_DIR/bubbles-hub-report.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "bubbles-hub-report-selftest: SKIP (python3 not installed)"
  exit 0
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# --- Build a fixture repo with a known governance graph -----------------------
# hub.sh is sourced by a.sh, b.sh, c.sh → in-degree 3 (the clear top script hub).
# shared-hub.md is included by two agents → in-degree 2.
# G500 is referenced by workflows.yaml + one script → in-degree 2.
root="$work/repo"
mkdir -p "$root/bubbles/scripts" "$root/bubbles/registry" "$root/agents/bubbles_shared"

printf '#!/usr/bin/env bash\n: hub\n' >"$root/bubbles/scripts/hub.sh"
for s in a b c; do
  printf '#!/usr/bin/env bash\nsource "$SCRIPT_DIR/hub.sh"\n' >"$root/bubbles/scripts/$s.sh"
done
# one script references gate G500
printf '#!/usr/bin/env bash\n# enforces G500 here\nsource "$SCRIPT_DIR/hub.sh"\n' >"$root/bubbles/scripts/d.sh"

printf -- '# shared hub module\n' >"$root/agents/bubbles_shared/shared-hub.md"
printf -- '# lonely module\n' >"$root/agents/bubbles_shared/lonely.md"
for ag in one two; do
  printf -- '---\nname: %s\n---\nstandard_rules: see bubbles_shared/shared-hub.md\n' "$ag" \
    >"$root/agents/bubbles.$ag.agent.md"
done

cat >"$root/bubbles/registry/gates.yaml" <<'EOF'
gates:
  G500:
    name: fixture_probe_gate
  G501:
    name: other_probe_gate
EOF

cat >"$root/bubbles/workflows.yaml" <<'EOF'
modes:
  fixture-mode:
    gates:
      - G500 # referenced by a mode
EOF

# --- Case 1: ranking — hub.sh is the top SCRIPT hub with in-degree 4 -----------
set +e
out1="$(bash "$HUB" --root "$root" --top 10 2>&1)"
c1=$?
set -e
# a,b,c,d all source hub.sh → in-degree 4
if [[ "$c1" -eq 0 ]] && grep -qE '^[[:space:]]+4[[:space:]]+script[[:space:]]+hub\.sh' <<<"$out1"; then
  pass "hub.sh ranks with in-degree 4 (sourced by a/b/c/d) and exit 0"
else
  fail "hub.sh should rank with in-degree 4 (got exit $c1)"
  echo "$out1"
fi
if grep -qE '^[[:space:]]+2[[:space:]]+shared-module[[:space:]]+shared-hub\.md' <<<"$out1"; then
  pass "shared-hub.md ranks with in-degree 2 (included by two agents)"
else
  fail "shared-hub.md should rank with in-degree 2"
  echo "$out1"
fi
if grep -qE '^[[:space:]]+2[[:space:]]+gate[[:space:]]+G500' <<<"$out1"; then
  pass "G500 ranks with in-degree 2 (workflows.yaml + d.sh)"
else
  fail "G500 should rank with in-degree 2"
  echo "$out1"
fi

# --- Case 2: --node reverse-deps are exact + provenance-tagged -----------------
set +e
out2="$(bash "$HUB" --root "$root" --node hub.sh 2>&1)"
c2=$?
set -e
if [[ "$c2" -eq 0 ]] \
  && grep -q "in-degree (distinct source files): 4" <<<"$out2" \
  && grep -q "bubbles/scripts/a.sh:2  \[script-call\]" <<<"$out2"; then
  pass "--node hub.sh prints the exact provenance-tagged reverse-dependency set"
else
  fail "--node hub.sh reverse-deps wrong (got exit $c2)"
  echo "$out2"
fi

# --- Case 3: --format json is stable ------------------------------------------
set +e
out3="$(bash "$HUB" --root "$root" --top 5 --format json 2>&1)"
c3=$?
set -e
if [[ "$c3" -eq 0 ]] && python3 -c "
import json,sys
d=json.loads(sys.stdin.read())
top={h['node']:h['inDegree'] for h in d['topHubs']}
assert top.get('hub.sh')==4, top
assert top.get('shared-hub.md')==2, top
assert top.get('G500')==2, top
assert d['nodeCounts']['gates']==2
" <<<"$out3" 2>/dev/null; then
  pass "--format json emits a stable, correct hub ranking"
else
  fail "--format json shape/contents wrong (got exit $c3)"
  echo "$out3"
fi

# --- Case 4: a node with no dependents reports in-degree 0 --------------------
set +e
out4="$(bash "$HUB" --root "$root" --node lonely.md 2>&1)"
c4=$?
set -e
if [[ "$c4" -eq 0 ]] && grep -q "in-degree (distinct source files): 0" <<<"$out4"; then
  pass "an unreferenced node reports in-degree 0 (no fabricated edges)"
else
  fail "lonely.md should have in-degree 0 (got exit $c4)"
  echo "$out4"
fi

# --- Case 5: usage error exits 2 (never 1) ------------------------------------
set +e
bash "$HUB" --root "$root" --format xml >/dev/null 2>&1
c5=$?
set -e
[[ "$c5" -eq 2 ]] \
  && pass "an invalid --format exits 2 (usage), never 1" \
  || fail "invalid --format should exit 2 (got exit $c5)"

# --- Case 6: --top must be a positive integer ---------------------------------
set +e
bash "$HUB" --root "$root" --top 0 >/dev/null 2>&1
c6=$?
set -e
[[ "$c6" -eq 2 ]] \
  && pass "--top 0 is rejected with exit 2" \
  || fail "--top 0 should exit 2 (got exit $c6)"

if [[ "$failures" -eq 0 ]]; then
  echo "[bubbles-hub-report-selftest] OK"
else
  echo "[bubbles-hub-report-selftest] $failures failed"
  exit 1
fi
