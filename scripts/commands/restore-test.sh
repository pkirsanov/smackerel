#!/usr/bin/env bash
# Spec 048 — Restore drill (`./smackerel.sh backup-restore-test`).
#
# Purpose: prove that a backup artifact written by scripts/commands/backup.sh
# actually restores into a fresh, isolated postgres and yields a database
# containing both the expected schema and connector cursor state. This is
# the FR-048-002 acceptance evidence: a backup that cannot be restored
# does not count.
#
# Contract:
#   - Starts a disposable postgres container on a random high port
#     (no published host port; reach via `docker exec`).
#   - Pipes the supplied backup through `gunzip | psql`.
#   - Asserts that the schema_migrations table is non-empty (so we know
#     the dump contained a real schema, not just an empty database).
#   - Asserts that the sync_state table is reachable (canonical
#     connector cursor store) — FR-048-002 cursor preservation.
#   - Asserts that no secret-shaped substring appears in stdout/stderr
#     (FR-048-003 redaction is preserved through the restore path).
#   - Tears down the disposable container unconditionally via trap.
#
# Exit codes:
#   0 — restore succeeded and assertions passed
#   1 — restore failed OR an assertion failed
#
# This script intentionally does NOT touch the live postgres container.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_ENV="dev"
BACKUP_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)
      TARGET_ENV="$2"; shift 2 ;;
    --env=*)
      TARGET_ENV="${1#*=}"; shift ;;
    --backup-file)
      BACKUP_FILE="$2"; shift 2 ;;
    --backup-file=*)
      BACKUP_FILE="${1#*=}"; shift ;;
    -h|--help)
      cat <<USAGE
Usage: ./smackerel.sh backup-restore-test [--env <env>] [--backup-file <path>]

  --env          Generated env file to source (default: dev).
  --backup-file  Specific backup artifact to restore. Defaults to the
                 newest smackerel-*.sql.gz inside \${BACKUP_LOCAL_DIR}.
USAGE
      exit 0 ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1 ;;
  esac
done

ENV_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"
[[ -f "$ENV_FILE" ]] || {
  echo "ERROR: Generated env file not found: $ENV_FILE" >&2
  echo "Run: ./smackerel.sh config generate" >&2
  exit 1
}

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

POSTGRES_USER="${POSTGRES_USER:?POSTGRES_USER not set in env file}"
POSTGRES_DB="${POSTGRES_DB:?POSTGRES_DB not set in env file}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD not set in env file}"
BACKUP_LOCAL_DIR="${BACKUP_LOCAL_DIR:?BACKUP_LOCAL_DIR not set — run ./smackerel.sh config generate}"

case "$BACKUP_LOCAL_DIR" in
  /*) BACKUP_DIR="$BACKUP_LOCAL_DIR" ;;
  *)  BACKUP_DIR="$REPO_ROOT/$BACKUP_LOCAL_DIR" ;;
esac

if [[ -z "$BACKUP_FILE" ]]; then
  # Pick the newest artifact in the backup directory.
  BACKUP_FILE="$(ls -1t "$BACKUP_DIR"/smackerel-*.sql.gz 2>/dev/null | head -n 1 || true)"
fi

if [[ -z "$BACKUP_FILE" || ! -f "$BACKUP_FILE" ]]; then
  echo "ERROR: No backup artifact found." >&2
  echo "Either provide --backup-file <path> or create one via ./smackerel.sh backup." >&2
  exit 1
fi

# Use a disposable container name unrelated to the live compose project.
# The 16-byte hex suffix makes parallel drill runs safe.
SUFFIX="$(openssl rand -hex 8 2>/dev/null || head -c 16 /dev/urandom | xxd -p)"
RESTORE_CONTAINER="smackerel-restore-drill-${SUFFIX}"
RESTORE_VOLUME=""  # tmpfs; no named volume, no persistent state

cleanup() {
  local code=$?
  if docker inspect "$RESTORE_CONTAINER" >/dev/null 2>&1; then
    docker rm -f "$RESTORE_CONTAINER" >/dev/null 2>&1 || true
  fi
  exit $code
}
trap cleanup EXIT INT TERM

echo "Restore drill — container: $RESTORE_CONTAINER"
echo "Restore drill — artifact:  $BACKUP_FILE"

# Bring up a disposable postgres with NO published host port. We will
# reach it via `docker exec`, mirroring the spec 042 tailnet-edge
# pattern (no infra services need host bindings).
docker run -d \
  --name "$RESTORE_CONTAINER" \
  --tmpfs /var/lib/postgresql/data \
  -e POSTGRES_USER="$POSTGRES_USER" \
  -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
  -e POSTGRES_DB="$POSTGRES_DB" \
  --health-cmd="pg_isready -U $POSTGRES_USER -d $POSTGRES_DB" \
  --health-interval=2s \
  --health-timeout=2s \
  --health-retries=30 \
  pgvector/pgvector:pg16 \
  >/dev/null

# Wait for the container to become healthy.
echo -n "Waiting for restore postgres to become healthy "
for _ in $(seq 1 60); do
  status="$(docker inspect --format='{{.State.Health.Status}}' "$RESTORE_CONTAINER" 2>/dev/null || echo unknown)"
  if [[ "$status" == "healthy" ]]; then
    echo " OK"
    break
  fi
  echo -n "."
  sleep 1
done
if [[ "$(docker inspect --format='{{.State.Health.Status}}' "$RESTORE_CONTAINER" 2>/dev/null)" != "healthy" ]]; then
  echo
  echo "ERROR: Restore postgres did not become healthy" >&2
  exit 1
fi

# Pipe the gunzipped backup into psql via docker exec stdin.
echo "Restoring backup..."
RESTORE_LOG="$(mktemp)"
trap 'rm -f "$RESTORE_LOG"; cleanup' EXIT INT TERM
if ! gunzip -c "$BACKUP_FILE" | docker exec -i \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  "$RESTORE_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  >"$RESTORE_LOG" 2>&1; then
  echo "ERROR: psql restore failed. Log tail:" >&2
  tail -n 50 "$RESTORE_LOG" >&2
  exit 1
fi

# FR-048-003 secret redaction — none of the closed-set secret values
# should appear in the restore stdout/stderr (psql can echo SQL
# statements that contain credentials if the dump was malformed).
for var in POSTGRES_PASSWORD SMACKEREL_AUTH_TOKEN TELEGRAM_BOT_TOKEN \
           AUTH_SIGNING_ACTIVE_PRIVATE_KEY AUTH_AT_REST_HASHING_KEY \
           AUTH_BOOTSTRAP_TOKEN LLM_API_KEY; do
  val="${!var:-}"
  if [[ -n "$val" ]] && grep -Fq "$val" "$RESTORE_LOG"; then
    echo "ERROR: Secret-shaped value for $var leaked into restore log" >&2
    exit 1
  fi
done

# FR-048-002 assertion 1: schema_migrations is non-empty so we know
# the dump carried a real schema.
MIG_COUNT="$(docker exec \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  "$RESTORE_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -Atqc 'SELECT COUNT(*) FROM schema_migrations' 2>/dev/null || echo 0)"
if [[ "${MIG_COUNT:-0}" -lt 1 ]]; then
  echo "ERROR: Restored database has no schema_migrations rows" >&2
  exit 1
fi
echo "  schema_migrations rows: $MIG_COUNT"

# FR-048-002 assertion 2: sync_state table is reachable (canonical
# connector cursor store). A non-error count is sufficient; the table
# can legitimately be empty in a freshly-bootstrapped system.
if ! docker exec \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  "$RESTORE_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -Atqc 'SELECT COUNT(*) FROM sync_state' >/dev/null 2>&1; then
  echo "ERROR: sync_state table missing or unreadable after restore" >&2
  exit 1
fi
echo "  sync_state table:        reachable"

# Optional: assert pgvector extension is present (the dump should
# include CREATE EXTENSION vector).
EXT_COUNT="$(docker exec \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  "$RESTORE_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -Atqc "SELECT COUNT(*) FROM pg_extension WHERE extname='vector'" 2>/dev/null || echo 0)"
if [[ "${EXT_COUNT:-0}" -lt 1 ]]; then
  echo "ERROR: pgvector extension missing after restore" >&2
  exit 1
fi
echo "  pgvector extension:      present"

echo
echo "Restore drill PASSED — $(basename "$BACKUP_FILE") restored cleanly."
