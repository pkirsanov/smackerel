#!/usr/bin/env bash
#
# scripts/lint/python-compute-only-guard.sh
#
# Spec 102 SCOPE-102-01 (SCN-102-C1-03 / SCN-102-C1-04) — smackerel-ml
# compute-only invariant guard.
#
# Codifies the invariant that smackerel's Python ML sidecar (ml/**) is
# COMPUTE-ONLY: it reaches product data ONLY over its typed NATS contract wire
# — NEVER a direct datastore driver (postgres/redis/kafka/mongo) and NEVER a
# direct datastore-URL read — AND its container env is the PROJECTED, secret-free
# `ml.env` (deploy/compose.deploy.yml points smackerel-ml.env_file at ./ml.env,
# never ./app.env). Mirrors QF spec 089 Scope C in SHAPE (three checks,
# .allowlist, exit 0/1/2, NO bypass flag) but is smackerel-specific:
#
#   * NATS is the SANCTIONED transport for smackerel's compute-only sidecar, so
#     `nats` / `nats-py` and `NATS_URL` are NOT forbidden (they are how the
#     sidecar legitimately reaches data). QF forbids nats; smackerel does not.
#
# Three checks:
#
#   (a) Dependency scan — no forbidden DATASTORE driver in any
#       ml/pyproject.toml / ml/requirements*.txt: psycopg(2)/asyncpg/sqlalchemy/
#       sqlmodel/redis/aioredis/aiokafka/confluent-kafka/kafka-python/pymongo/
#       motor/databases/tortoise. Matched only in a dependency-spec position
#       (quoted requirement or a requirements-line), so prose that merely
#       mentions a name never trips.
#
#   (b) Infra-URL read scan — no direct os.environ[...] / os.getenv(...) read of
#       DATABASE_URL / POSTGRES_URL / REDIS_URL / RABBITMQ_URL in any ml/**/*.py.
#       (NATS_URL is the sanctioned transport and is intentionally NOT listed.)
#
#   (c) Env-delivery shape (SCN-102-C1-03) — the compute-only sidecar must load
#       the PROJECTED secret-free `ml.env`, NOT the full `app.env`. Asserts the
#       deploy compose's smackerel-ml service declares `env_file: ./ml.env` and
#       does NOT declare `./app.env`; and that config/smackerel.yaml declares the
#       `services.ml.env_allowlist` SST projection surface. This statically
#       catches the SCN-102-C1-03 adversarial case (re-adding env_file:
#       ./app.env to smackerel-ml re-delivers every managed secret). The deep
#       bundle-content assertion (ml.env ∩ SHELL_SECRET_KEYS = ∅) is owned by
#       internal/deploy/bundle_secret_contract_test.go (runs in CI Docker); the
#       tripwire (env_allowlist ∩ SHELL_SECRET_KEYS ⇒ abort) is owned by
#       scripts/commands/config.sh::project_service_env.
#
# Exit codes:
#   0  - clean (compute-only + projected env delivery)
#   1  - violation: a forbidden driver, a direct infra-URL read, or an env
#        delivery that re-exposes app.env / drops the projection
#   2  - bypass flag attempt OR unknown argument OR malformed input
#
# NO bypass flag exists (--skip / --force / --ignore / --no-verify -> exit 2).
#
# Cross-platform: bash 3.2-safe + POSIX grep -E / find / awk, so it runs on
# Linux/WSL (CI ubuntu) and macOS (BSD userland).

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd -P)"
SCAN_ROOT="${PYCO_GUARD_SCAN_ROOT:-${REPO_ROOT}/ml}"
COMPOSE_FILE="${PYCO_GUARD_COMPOSE_FILE:-${REPO_ROOT}/deploy/compose.deploy.yml}"
CONFIG_YAML="${PYCO_GUARD_CONFIG_YAML:-${REPO_ROOT}/config/smackerel.yaml}"

# Forbidden DATASTORE-driver package names (compute-only violation). NATS is the
# sanctioned transport and is deliberately absent.
FORBIDDEN='psycopg2|psycopg|asyncpg|sqlalchemy|sqlmodel|aioredis|redis|aiokafka|confluent[-_]kafka|kafka-python|pymongo|motor|databases|tortoise'
# A forbidden name in a dependency-spec position: quoted requirement ("name>=x",
# "name[extra]", "name") OR a requirements.txt line (name==x / bare name). The
# trailing boundary is a requirement terminator, so prose ("redis-like cache")
# never matches.
DEP_PATTERN="[\"'](${FORBIDDEN})([<>=!~;,[]|[\"'])|^[[:space:]]*(${FORBIDDEN})([<>=!~;,[]|[[:space:]]|\$)"
# Direct datastore-URL env read (NATS_URL intentionally excluded — sanctioned wire).
INFRA_URL_PATTERN='os\.(environ\[|getenv\()[[:space:]]*["'"'"'](DATABASE_URL|POSTGRES_URL|REDIS_URL|RABBITMQ_URL)["'"'"']'

usage() {
  cat <<'USAGE_EOF'
Usage: bash scripts/lint/python-compute-only-guard.sh [--help|-h]

Spec 102 SCOPE-102-01 — asserts the smackerel Python ML sidecar (ml/**) is
compute-only: no forbidden datastore-driver dependency, no direct datastore-URL
read, and a secret-free projected env delivery (smackerel-ml loads ./ml.env,
never ./app.env).

Exit codes:
  0 = clean   1 = violation   2 = bypass flag / unknown arg / malformed input

NO --skip / --force / --ignore / --no-verify flag exists.
Env:
  PYCO_GUARD_SCAN_ROOT     Python surface to scan   (default: ml/)
  PYCO_GUARD_COMPOSE_FILE  deploy compose to check  (default: deploy/compose.deploy.yml)
  PYCO_GUARD_CONFIG_YAML   SST config to check      (default: config/smackerel.yaml)
USAGE_EOF
}

for arg in "$@"; do
  case "$arg" in
    --skip | --force | --ignore | --no-verify)
      printf 'python-compute-only-guard: ERROR - bypass flag %q is forbidden by repo policy.\n' "$arg" >&2
      exit 2
      ;;
  esac
done

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf 'python-compute-only-guard: ERROR - unknown argument %q\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$SCAN_ROOT" ]]; then
  printf 'python-compute-only-guard: FATAL - scan root not found: %s\n' "$SCAN_ROOT" >&2
  exit 2
fi

fail=0
err() {
  printf 'python-compute-only-guard: FAIL - %s\n' "$*" >&2
  fail=1
}
info() { printf 'python-compute-only-guard: %s\n' "$*"; }

# Allowlist: repo-relative path globs to EXCLUDE from the file scans (empty by default).
ALLOWLIST_FILE="${SCRIPT_DIR}/python-compute-only-guard.allowlist"
ALLOWLIST_PATTERNS=()
if [[ -f "$ALLOWLIST_FILE" ]]; then
  while IFS= read -r line; do
    line="${line%%#*}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -n "$line" ]] && ALLOWLIST_PATTERNS+=("$line")
  done <"$ALLOWLIST_FILE"
fi
is_allowlisted() {
  local path="$1" pattern
  for pattern in ${ALLOWLIST_PATTERNS[@]+"${ALLOWLIST_PATTERNS[@]}"}; do
    # shellcheck disable=SC2053  # intentional glob match, not equality
    [[ "$path" == $pattern ]] && return 0
  done
  return 1
}

# ── 1. Dependency scan (pyproject.toml / requirements*.txt) ──────────────────
dep_files=0
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  is_allowlisted "${f#"$REPO_ROOT"/}" && continue
  dep_files=$((dep_files + 1))
  if hits="$(grep -InE "$DEP_PATTERN" "$f" 2>/dev/null)"; then
    while IFS= read -r line; do
      [[ -n "$line" ]] && err "forbidden datastore-driver dependency in ${f#"$REPO_ROOT"/}:${line}"
    done <<<"$hits"
  fi
done <<<"$(find "$SCAN_ROOT" -type f \( -name 'pyproject.toml' -o -name 'requirements*.txt' \) 2>/dev/null | sort)"
[[ "$fail" -eq 0 ]] && info "OK - dependency scan: no forbidden datastore driver in ${dep_files} dependency file(s) under ${SCAN_ROOT#"$REPO_ROOT"/} (nats-py is the sanctioned transport)"

# ── 2. Infra-URL read scan (*.py) ────────────────────────────────────────────
url_fail_before="$fail"
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  is_allowlisted "${f#"$REPO_ROOT"/}" && continue
  if hits="$(grep -InE "$INFRA_URL_PATTERN" "$f" 2>/dev/null)"; then
    while IFS= read -r line; do
      [[ -n "$line" ]] && err "direct datastore-URL read in ${f#"$REPO_ROOT"/}:${line} (the compute-only sidecar must reach data via the typed NATS wire, not a direct DB/cache/queue URL)"
    done <<<"$hits"
  fi
done <<<"$(find "$SCAN_ROOT" -type f -name '*.py' 2>/dev/null | sort)"
[[ "$fail" -eq "$url_fail_before" ]] && info "OK - infra-URL scan: no direct DATABASE_URL/POSTGRES_URL/REDIS_URL/RABBITMQ_URL read in *.py (NATS_URL is the sanctioned wire)"

# ── 3. Env-delivery shape (SCN-102-C1-03) ────────────────────────────────────
# The compute-only sidecar MUST load the projected secret-free ml.env, not the
# full app.env. Extract the smackerel-ml service block from the deploy compose
# and assert its env_file is ./ml.env (and NOT ./app.env). Also assert the SST
# projection surface (services.ml.env_allowlist) is declared.
env_fail_before="$fail"
if [[ ! -f "$COMPOSE_FILE" ]]; then
  err "deploy compose not found: ${COMPOSE_FILE#"$REPO_ROOT"/}"
elif [[ ! -f "$CONFIG_YAML" ]]; then
  err "SST config not found: ${CONFIG_YAML#"$REPO_ROOT"/}"
else
  # Extract the smackerel-ml service block: lines from `^  smackerel-ml:` up to
  # (but not including) the next top-level service key `^  <name>:`.
  ml_block="$(awk '
    /^  smackerel-ml:[[:space:]]*$/ { inblk=1; next }
    inblk && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { inblk=0 }
    inblk { print }
  ' "$COMPOSE_FILE")"
  if [[ -z "$ml_block" ]]; then
    err "could not locate the smackerel-ml service block in ${COMPOSE_FILE#"$REPO_ROOT"/}"
  else
    if printf '%s\n' "$ml_block" | grep -qE '^[[:space:]]*-[[:space:]]*\./app\.env[[:space:]]*$'; then
      err "smackerel-ml declares env_file ./app.env in ${COMPOSE_FILE#"$REPO_ROOT"/} — the compute-only sidecar MUST load the projected secret-free ./ml.env, never the full ./app.env (SCN-102-C1-03; re-adding app.env re-delivers every managed secret)"
    fi
    if ! printf '%s\n' "$ml_block" | grep -qE '^[[:space:]]*-[[:space:]]*\./ml\.env[[:space:]]*$'; then
      err "smackerel-ml does NOT declare env_file ./ml.env in ${COMPOSE_FILE#"$REPO_ROOT"/} — the compute-only projection (spec 102 SCOPE-102-01) is not wired"
    fi
  fi
  if ! grep -qE '^[[:space:]]*env_allowlist:[[:space:]]*$' "$CONFIG_YAML"; then
    err "services.ml.env_allowlist is not declared in ${CONFIG_YAML#"$REPO_ROOT"/} — the SST projection surface (spec 102 SCOPE-102-01) is missing"
  fi
fi
[[ "$fail" -eq "$env_fail_before" ]] && info "OK - env-delivery shape: smackerel-ml loads the projected ./ml.env (not ./app.env); services.ml.env_allowlist is declared"

if [[ "$fail" -ne 0 ]]; then
  printf 'python-compute-only-guard: RESULT - FAILED (smackerel-ml is not compute-only / not projected).\n' >&2
  exit 1
fi

info "clean - smackerel Python (${SCAN_ROOT#"$REPO_ROOT"/}) is compute-only: no datastore driver, no direct datastore-URL read, secret-free projected env delivery."
exit 0
