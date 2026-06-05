#!/usr/bin/env bash
#
# bubbles/scripts/gates-registry-selftest.sh
#
# Selftest for v5.2 / F4: bubbles/registry/gates.yaml ↔ workflows.yaml
# gates block round-trip.
#
# Asserts:
#   T1. Registry file exists.
#   T2. workflows.yaml gates block is in sync with the registry
#       (--check returns 0).
#   T3. Round-trip is stable: writing then re-checking produces no change.
#   T4. Drift detection works: mutating the registry copy makes --check fail.
#   T5. gate-meta.sh count equals the number of `Gxxx:` entries in the registry.
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
REGISTRY="$ROOT_DIR/bubbles/registry/gates.yaml"
WORKFLOWS="$ROOT_DIR/bubbles/workflows.yaml"
GEN="$SCRIPT_DIR/generate-gates-block.sh"
GATE_META="$SCRIPT_DIR/gate-meta.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

# --- T1: registry exists ---
if [[ -f "$REGISTRY" ]]; then
  pass "T1: registry file exists at bubbles/registry/gates.yaml"
else
  # In downstream repos that installed v5.2.0 (before the v5.2.1 installer
  # fix), the registry file may not exist. Emit SKIP and exit 0 so the
  # selftest doesn't block downstream pre-push hooks until they re-run
  # install.sh.
  echo "SKIP: T1: bubbles/registry/gates.yaml is missing (re-run install.sh to upgrade past v5.2.0 installer gap)"
  exit 0
fi

# --- T2: --check is currently in sync ---
if bash "$GEN" --check >/dev/null 2>&1; then
  pass "T2: workflows.yaml gates block matches registry (--check exit 0)"
else
  fail "T2: workflows.yaml gates block does NOT match registry — run generate-gates-block.sh"
fi

# --- T3: round-trip stability (write into a copy, --check passes) ---
tmp_root="$(mktemp -d -t bubbles-gates-registry-selftest.XXXXXX)"
trap 'rm -rf "$tmp_root"' EXIT INT TERM
# Mirror enough of the repo so the generator can resolve paths from a
# scripts/ dir.
mkdir -p "$tmp_root/bubbles/scripts" "$tmp_root/bubbles/registry"
cp "$REGISTRY" "$tmp_root/bubbles/registry/gates.yaml"
cp "$WORKFLOWS" "$tmp_root/bubbles/workflows.yaml"
cp "$GEN" "$tmp_root/bubbles/scripts/generate-gates-block.sh"
chmod +x "$tmp_root/bubbles/scripts/generate-gates-block.sh"

# First write (should be no-op since input already in sync).
out1="$(bash "$tmp_root/bubbles/scripts/generate-gates-block.sh" 2>&1 || true)"
if echo "$out1" | grep -q "no change\|updated"; then
  pass "T3a: generator runs without error on synced input"
else
  fail "T3a: generator output unexpected: $out1"
fi
# --check after write should still pass.
if bash "$tmp_root/bubbles/scripts/generate-gates-block.sh" --check >/dev/null 2>&1; then
  pass "T3b: round-trip stable (--check exit 0 after write)"
else
  fail "T3b: round-trip unstable — write produced drift"
fi

# --- T4: drift detection works ---
echo "" >> "$tmp_root/bubbles/registry/gates.yaml"
echo "  G999_test_only:" >> "$tmp_root/bubbles/registry/gates.yaml"
echo "    name: synthetic_gate_for_selftest" >> "$tmp_root/bubbles/registry/gates.yaml"
if bash "$tmp_root/bubbles/scripts/generate-gates-block.sh" --check >/dev/null 2>&1; then
  fail "T4: drift NOT detected after mutating the registry (FALSE-NEGATIVE)"
else
  pass "T4: drift detected after mutating registry (--check exit 1)"
fi

# --- T5: gate-meta count matches registry Gxxx entries ---
registry_gate_count="$(grep -cE '^  G[0-9]{3}:$' "$REGISTRY" || true)"
gate_meta_count="$(bash "$GATE_META" count 2>/dev/null || echo 0)"
if [[ "$registry_gate_count" == "$gate_meta_count" ]]; then
  pass "T5: gate-meta.sh count ($gate_meta_count) matches registry entries ($registry_gate_count)"
else
  fail "T5: gate-meta.sh count ($gate_meta_count) DIFFERS from registry entries ($registry_gate_count)"
fi

if (( failures == 0 )); then
  echo "OK: gates-registry-selftest passed ($registry_gate_count gates in registry)"
  exit 0
else
  echo "FAILED: gates-registry-selftest had $failures assertion failures" >&2
  exit 1
fi
