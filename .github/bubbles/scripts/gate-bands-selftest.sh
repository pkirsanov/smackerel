#!/usr/bin/env bash
# gate-bands-selftest.sh — hermetic selftest.
#
# Proves the band computation is correct (including the fragmented case that
# the hand-written prose got wrong) and that --check actually detects drift.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL="$SCRIPT_DIR/gate-bands.sh"

if [[ ! -f "$TOOL" ]]; then
  echo "gate-bands-selftest: tool not found: $TOOL" >&2
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "gate-bands-selftest: SKIP (python3 not installed)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

pass_count=0
fail_count=0

check() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo "PASS  $name"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  $name"
    echo "      expected: $expected"
    echo "      actual:   $actual"
    fail_count=$((fail_count + 1))
  fi
}

# make_repo <dir> <gate-ids...>
make_repo() {
  local d="$1"
  shift
  mkdir -p "$d/bubbles/registry" "$d/docs/recipes"
  {
    echo "gates:"
    for gid in "$@"; do
      echo "  $gid:"
      echo "    name: some_gate"
    done
  } >"$d/bubbles/registry/gates.yaml"
  cat >"$d/bubbles/workflows.yaml" <<'EOF'
# GENERATED:GATE_BANDS_START
# stale
# GENERATED:GATE_BANDS_END
modes: {}
EOF
  cat >"$d/docs/recipes/custom-gates.md" <<'EOF'
<!-- GENERATED:GATE_BANDS_START -->
stale
<!-- GENERATED:GATE_BANDS_END -->
EOF
}

# --- contiguous run ---------------------------------------------------------
r1="$WORK/r1"
make_repo "$r1" G001 G002 G003
check "contiguous ids collapse to one band" "G001-G003" "$(bash "$TOOL" --print --repo-root "$r1")"

# --- fragmented set (the case the hand-written prose got wrong) -------------
r2="$WORK/r2"
make_repo "$r2" G001 G002 G005 G009 G010
check "fragmented ids enumerate every run" "G001-G002 plus G005 plus G009-G010" "$(bash "$TOOL" --print --repo-root "$r2")"

# --- single id --------------------------------------------------------------
r3="$WORK/r3"
make_repo "$r3" G042
check "a lone id renders without a range" "G042" "$(bash "$TOOL" --print --repo-root "$r3")"

# --- drift detection --------------------------------------------------------
r4="$WORK/r4"
make_repo "$r4" G001 G002
rc=0
bash "$TOOL" --check --repo-root "$r4" >/dev/null 2>&1 || rc=$?
check "check detects a stale band string" "1" "$rc"

bash "$TOOL" --repo-root "$r4" >/dev/null 2>&1
rc=0
bash "$TOOL" --check --repo-root "$r4" >/dev/null 2>&1 || rc=$?
check "check is green after a write" "0" "$rc"

# --- both surfaces are actually updated -------------------------------------
if grep -q 'G001-G002' "$r4/bubbles/workflows.yaml"; then
  echo "PASS  workflows.yaml band string was written"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  workflows.yaml band string was not written"
  fail_count=$((fail_count + 1))
fi
if grep -q 'G001-G002' "$r4/docs/recipes/custom-gates.md"; then
  echo "PASS  custom-gates.md band string was written"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  custom-gates.md band string was not written"
  fail_count=$((fail_count + 1))
fi

# --- the custom-gate band must never overlap the framework band -------------
#
# This is the collision SCOPE-2d fixes: idRange advertised G100+ while the
# framework occupies G110-G131.
repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
if [[ -f "$repo_root/bubbles/workflows.yaml" ]]; then
  id_range="$(grep -E '^\s+idRange:' "$repo_root/bubbles/workflows.yaml" | head -1 | sed -E 's/.*idRange:\s*//')"
  max_gate="$(grep -oE '^  G[0-9]{3}:' "$repo_root/bubbles/registry/gates.yaml" | grep -oE '[0-9]{3}' | sort -n | tail -1)"
  range_start="$(printf '%s' "$id_range" | grep -oE '[0-9]+' | head -1)"
  if [[ -n "$range_start" && -n "$max_gate" ]] && ((10#$range_start > 10#$max_gate)); then
    echo "PASS  customGatesDiscovery.idRange ($id_range) starts above the highest framework gate (G$max_gate)"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  customGatesDiscovery.idRange ($id_range) collides with the framework band (highest framework gate G$max_gate)"
    fail_count=$((fail_count + 1))
  fi
fi

echo ""
echo "gate-bands selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All gate-bands selftests passed."
exit 0
