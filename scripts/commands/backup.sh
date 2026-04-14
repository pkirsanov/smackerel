#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_ENV="dev"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)
      TARGET_ENV="$2"
      shift 2
      ;;
    --env=*)
      TARGET_ENV="${1#*=}"
      shift
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

ENV_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"
[[ -f "$ENV_FILE" ]] || {
  echo "ERROR: Generated env file not found: $ENV_FILE" >&2
  echo "Run: ./smackerel.sh config generate" >&2
  exit 1
}

# Source env file for DB credentials and compose project
set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

COMPOSE_PROJECT="${COMPOSE_PROJECT:?COMPOSE_PROJECT not set in env file}"
POSTGRES_USER="${POSTGRES_USER:?POSTGRES_USER not set in env file}"
POSTGRES_DB="${POSTGRES_DB:?POSTGRES_DB not set in env file}"

BACKUP_DIR="$REPO_ROOT/backups"
mkdir -p "$BACKUP_DIR"

TIMESTAMP="$(date -u +%Y-%m-%d-%H%M%S)"
FILENAME="smackerel-${TIMESTAMP}.sql.gz"
BACKUP_PATH="$BACKUP_DIR/$FILENAME"

CONTAINER_NAME="${COMPOSE_PROJECT}-postgres-1"

# Verify postgres container is running
if ! docker inspect --format='{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null | grep -q true; then
  echo "ERROR: Database container is not running: $CONTAINER_NAME" >&2
  exit 1
fi

echo "Starting backup of database '$POSTGRES_DB'..."

# Run pg_dump inside the container, pipe through gzip.
# Capture stderr so diagnostic errors from pg_dump are visible on failure.
PGDUMP_STDERR_FILE="$(mktemp)"
trap 'rm -f "$PGDUMP_STDERR_FILE"' EXIT
if ! docker exec "$CONTAINER_NAME" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists 2>"$PGDUMP_STDERR_FILE" | gzip > "$BACKUP_PATH"; then
  rm -f "$BACKUP_PATH"
  echo "ERROR: pg_dump failed" >&2
  if [[ -s "$PGDUMP_STDERR_FILE" ]]; then
    echo "pg_dump stderr:" >&2
    cat "$PGDUMP_STDERR_FILE" >&2
  fi
  exit 1
fi

# Show any pg_dump warnings even on success
if [[ -s "$PGDUMP_STDERR_FILE" ]]; then
  echo "pg_dump warnings:" >&2
  cat "$PGDUMP_STDERR_FILE" >&2
fi

# Validate output file is non-empty
FILESIZE="$(stat --format='%s' "$BACKUP_PATH" 2>/dev/null || stat -f '%z' "$BACKUP_PATH" 2>/dev/null || echo 0)"
if [[ "$FILESIZE" -lt 100 ]]; then
  rm -f "$BACKUP_PATH"
  echo "ERROR: Backup file is empty — dump may have failed" >&2
  exit 1
fi

# Validate gzip integrity before reporting success
if ! gunzip -t "$BACKUP_PATH" 2>/dev/null; then
  rm -f "$BACKUP_PATH"
  echo "ERROR: Backup file failed gzip integrity check — dump may be corrupt" >&2
  exit 1
fi

# Human-readable size
if command -v numfmt >/dev/null 2>&1; then
  HUMAN_SIZE="$(numfmt --to=iec "$FILESIZE")"
else
  HUMAN_SIZE="${FILESIZE} bytes"
fi

echo "Backup created: $BACKUP_PATH ($HUMAN_SIZE)"
