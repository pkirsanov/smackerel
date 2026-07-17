#!/usr/bin/env bash
# Hermetic selftest for repo-binding-preflight.sh (BFW-05 / IMP-025).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFLIGHT="$SCRIPT_DIR/repo-binding-preflight.sh"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

echo "Running repo-binding-preflight selftest..."

# Target repo dir named "app-alpha" -> slug "app-alpha".
d="$TMP_ROOT/app-alpha"; mkdir -p "$d"

# ── T1: matching agent-source slug -> pass (exit 0)
out="$("$PREFLIGHT" --repo-root "$d" --agent-source app-alpha 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'matches target repo'; then
  pass "T1 matching agent source passes (exit 0)"
else
  fail "T1 matching agent source should pass (rc=$rc)"
fi

# ── T2: foreign agent-source slug -> refuse (exit 1), naming both repos
out="$("$PREFLIGHT" --repo-root "$d" --agent-source app-beta 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s\n' "$out" | grep -q 'BINDING MISMATCH' && printf '%s\n' "$out" | grep -q 'app-beta' && printf '%s\n' "$out" | grep -q 'app-alpha'; then
  pass "T2 foreign agent source is refused, naming both repos (exit 1)"
else
  fail "T2 foreign agent source should be refused naming both (rc=$rc)"
fi

# ── T3: canonical framework-source -> pass (exit 0)
out="$("$PREFLIGHT" --repo-root "$d" --canonical-source 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'canonical'; then
  pass "T3 canonical framework-source work passes (exit 0)"
else
  fail "T3 canonical source should pass (rc=$rc)"
fi

# ── T4: .install-source.json targetRepoSlug matching -> pass (marker auto-detect)
mkdir -p "$d/.github/bubbles"
printf '{ "installedVersion": "7.19.2", "targetRepoSlug": "app-alpha" }\n' > "$d/.github/bubbles/.install-source.json"
out="$("$PREFLIGHT" --repo-root "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'matches target repo'; then
  pass "T4 matching targetRepoSlug marker passes (auto-detect)"
else
  fail "T4 matching targetRepoSlug marker should pass (rc=$rc)"
fi

# ── T5: .install-source.json targetRepoSlug foreign -> refuse (exit 1)
printf '{ "installedVersion": "7.19.2", "targetRepoSlug": "app-beta" }\n' > "$d/.github/bubbles/.install-source.json"
out="$("$PREFLIGHT" --repo-root "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s\n' "$out" | grep -q 'BINDING MISMATCH' && printf '%s\n' "$out" | grep -q 'targetRepoSlug'; then
  pass "T5 foreign targetRepoSlug marker is refused (exit 1)"
else
  fail "T5 foreign targetRepoSlug marker should be refused (rc=$rc)"
fi

# ── T6: no marker + no --agent-source -> advisory (exit 0, remediation, non-blocking)
d2="$TMP_ROOT/no-marker-repo"; mkdir -p "$d2"
out="$("$PREFLIGHT" --repo-root "$d2" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'ADVISORY'; then
  pass "T6 no marker + no agent-source is advisory (exit 0, remediation)"
else
  fail "T6 no marker should be advisory not blocking (rc=$rc)"
fi

# ── T7: repo-slug derivation matches install.sh (dir "App-Alpha" -> "app-alpha")
d3="$TMP_ROOT/App-Alpha"; mkdir -p "$d3"
out="$("$PREFLIGHT" --repo-root "$d3" --agent-source app-alpha 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'matches target repo'; then
  pass "T7 repo-slug derivation matches install.sh (case/punct sanitized)"
else
  fail "T7 slug derivation should match install.sh (rc=$rc)"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "repo-binding-preflight-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "repo-binding-preflight-selftest: all cases passed."
