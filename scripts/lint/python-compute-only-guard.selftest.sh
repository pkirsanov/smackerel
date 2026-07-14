#!/usr/bin/env bash
#
# scripts/lint/python-compute-only-guard.selftest.sh
#
# Spec 102 SCOPE-102-01 — adversarial selftest for the smackerel-ml
# compute-only guard (SCN-102-C1-03 / SCN-102-C1-04). Proves the guard has
# BITE: it PASSES on the live tree and FAILS (exit 1) when the compute-only
# invariant is violated — a forbidden datastore driver, a direct datastore-URL
# read, a re-added env_file: ./app.env, or a dropped env_allowlist — and rejects
# every bypass flag (exit 2). Also proves NATS is the sanctioned transport (a
# nats-py dependency does NOT trip the guard).
#
# Hermetic: every adversarial case runs against a temp fixture (a scan root, a
# tampered compose copy, or a tampered config copy) via the guard's env-var
# override seams. The live repo files are NEVER mutated.
#
# Exit 0 iff every case behaves as expected; exit 1 otherwise.
#
# Cross-platform: bash 3.2-safe + POSIX. Run directly:
#   bash scripts/lint/python-compute-only-guard.selftest.sh

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd -P)"
GUARD="${SCRIPT_DIR}/python-compute-only-guard.sh"
LIVE_ML="${REPO_ROOT}/ml"
LIVE_COMPOSE="${REPO_ROOT}/deploy/compose.deploy.yml"
LIVE_CONFIG="${REPO_ROOT}/config/smackerel.yaml"

pass=0
fail=0
ok() {
  printf 'SELFTEST OK   - %s\n' "$*"
  pass=$((pass + 1))
}
bad() {
  printf 'SELFTEST FAIL - %s\n' "$*" >&2
  fail=$((fail + 1))
}

# run_guard <expected_rc> <needle_or_-> <label> -- <env assignments...>
# Runs the guard with the given env overrides, asserts the exit code and (if
# needle != "-") that the combined output contains the needle.
run_guard() {
  local want_rc="$1" needle="$2" label="$3"
  shift 3
  [[ "${1:-}" == "--" ]] && shift
  local out rc
  out="$(env "$@" bash "$GUARD" 2>&1)"
  rc=$?
  if [[ "$rc" -ne "$want_rc" ]]; then
    bad "${label}: expected exit ${want_rc}, got ${rc}"
    printf '  --- guard output ---\n%s\n  --- end ---\n' "$out" >&2
    return
  fi
  if [[ "$needle" != "-" ]] && ! printf '%s' "$out" | grep -qF "$needle"; then
    bad "${label}: exit ${rc} correct but output did not mention %q" "$needle"
    printf '  --- guard output ---\n%s\n  --- end ---\n' "$out" >&2
    return
  fi
  ok "${label} (exit ${rc}${needle:+, mentions \"$needle\"})"
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

# ── Case 0: clean live tree PASSES ───────────────────────────────────────────
run_guard 0 "clean" "clean live tree passes" -- \
  "PYCO_GUARD_SCAN_ROOT=${LIVE_ML}" \
  "PYCO_GUARD_COMPOSE_FILE=${LIVE_COMPOSE}" \
  "PYCO_GUARD_CONFIG_YAML=${LIVE_CONFIG}"

# ── Case 1: forbidden datastore driver in requirements FAILS (SCN-102-C1-04) ──
DRIVER_ROOT="${TMP_ROOT}/driver"
mkdir -p "$DRIVER_ROOT"
{
  echo "# adversarial fixture"
  echo "psycopg2==2.9.9"
} >"${DRIVER_ROOT}/requirements.txt"
run_guard 1 "psycopg2" "forbidden datastore driver trips the guard" -- \
  "PYCO_GUARD_SCAN_ROOT=${DRIVER_ROOT}" \
  "PYCO_GUARD_COMPOSE_FILE=${LIVE_COMPOSE}" \
  "PYCO_GUARD_CONFIG_YAML=${LIVE_CONFIG}"

# ── Case 2: direct datastore-URL read in *.py FAILS (SCN-102-C1-04) ───────────
URL_ROOT="${TMP_ROOT}/url"
mkdir -p "$URL_ROOT"
{
  echo "import os"
  echo 'DB = os.environ["DATABASE_URL"]'
} >"${URL_ROOT}/leak.py"
run_guard 1 "DATABASE_URL" "direct datastore-URL read trips the guard" -- \
  "PYCO_GUARD_SCAN_ROOT=${URL_ROOT}" \
  "PYCO_GUARD_COMPOSE_FILE=${LIVE_COMPOSE}" \
  "PYCO_GUARD_CONFIG_YAML=${LIVE_CONFIG}"

# ── Case 3: re-added env_file: ./app.env on smackerel-ml FAILS (SCN-102-C1-03) ─
BAD_COMPOSE="${TMP_ROOT}/compose-app-env.yml"
# Rewrite the smackerel-ml env_file from ./ml.env back to ./app.env (the exact
# regression this scope removes). Only the ml service's projected env_file line
# is changed; app.env stays for smackerel-core.
awk '
  /^  smackerel-ml:[[:space:]]*$/ { inblk=1 }
  inblk && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ && !/^  smackerel-ml:/ { inblk=0 }
  inblk && /^[[:space:]]*-[[:space:]]*\.\/ml\.env[[:space:]]*$/ { sub(/\.\/ml\.env/, "./app.env") }
  { print }
' "$LIVE_COMPOSE" >"$BAD_COMPOSE"
run_guard 1 "app.env" "re-added env_file: ./app.env trips the guard" -- \
  "PYCO_GUARD_SCAN_ROOT=${LIVE_ML}" \
  "PYCO_GUARD_COMPOSE_FILE=${BAD_COMPOSE}" \
  "PYCO_GUARD_CONFIG_YAML=${LIVE_CONFIG}"

# ── Case 4: dropped env_allowlist SST surface FAILS (SCN-102-C1-03) ───────────
BAD_CONFIG="${TMP_ROOT}/config-no-allowlist.yaml"
# Remove the `env_allowlist:` key line so the projection SST surface is gone.
grep -vE '^[[:space:]]*env_allowlist:[[:space:]]*$' "$LIVE_CONFIG" >"$BAD_CONFIG"
run_guard 1 "env_allowlist" "dropped env_allowlist trips the guard" -- \
  "PYCO_GUARD_SCAN_ROOT=${LIVE_ML}" \
  "PYCO_GUARD_COMPOSE_FILE=${LIVE_COMPOSE}" \
  "PYCO_GUARD_CONFIG_YAML=${BAD_CONFIG}"

# ── Case 5: NATS is the SANCTIONED transport — nats-py does NOT trip ──────────
NATS_ROOT="${TMP_ROOT}/nats"
mkdir -p "$NATS_ROOT"
{
  echo "# nats-py is the sanctioned compute-only transport for smackerel-ml"
  echo "nats-py==2.9.0"
} >"${NATS_ROOT}/requirements.txt"
{
  echo "import os"
  echo 'NATS = os.environ["NATS_URL"]  # sanctioned wire'
} >"${NATS_ROOT}/wire.py"
run_guard 0 "clean" "nats-py + NATS_URL do NOT trip the guard (sanctioned wire)" -- \
  "PYCO_GUARD_SCAN_ROOT=${NATS_ROOT}" \
  "PYCO_GUARD_COMPOSE_FILE=${LIVE_COMPOSE}" \
  "PYCO_GUARD_CONFIG_YAML=${LIVE_CONFIG}"

# ── Case 6: every bypass flag exits 2 (no bypass) ────────────────────────────
for flag in --skip --force --ignore --no-verify; do
  out="$(env "PYCO_GUARD_SCAN_ROOT=${LIVE_ML}" bash "$GUARD" "$flag" 2>&1)"
  rc=$?
  if [[ "$rc" -eq 2 ]] && printf '%s' "$out" | grep -qF "bypass flag"; then
    ok "bypass flag ${flag} exits 2 (no bypass)"
  else
    bad "bypass flag ${flag}: expected exit 2 + 'bypass flag', got exit ${rc}"
    printf '  --- guard output ---\n%s\n  --- end ---\n' "$out" >&2
  fi
done

printf '\npython-compute-only-guard selftest: %d passed, %d failed\n' "$pass" "$fail"
[[ "$fail" -eq 0 ]] || exit 1
exit 0
