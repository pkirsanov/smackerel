#!/usr/bin/env bash
# Spec 048 — Backup automation entrypoint invoked by `./smackerel.sh backup`.
#
# Product-owned responsibilities (THIS script):
#   - pg_dump | gzip → ${BACKUP_LOCAL_DIR}/smackerel-<ts>.sql.gz
#   - Validate file size + gzip integrity
#   - Apply retention policy (FR-048-001: 7 daily + 4 weekly)
#   - Write status JSON to ${BACKUP_STATUS_FILE} for the Go core's
#     metrics watcher (consumed by SmackerelBackupStale alert)
#   - REDACT secrets — POSTGRES_PASSWORD, SMACKEREL_AUTH_TOKEN, etc.
#     MUST NEVER appear in the status file or stdout/stderr
#
# Target adapter responsibilities (OUT OF SCOPE here):
#   - Scheduling (systemd / cron timer)
#   - Off-host shipping (${BACKUP_DESTINATION_URL}, set by adapter)
#
# Exit codes:
#   0 — backup created and retention applied
#   1 — backup or retention failed (status file records the error class)
#
# SST values are sourced from config/generated/${TARGET_ENV}.env. Missing
# values fail loud here (Gate G028) rather than falling back to defaults.

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

# Source env file for DB credentials, compose project, and backup contract.
# Spec 048 contract: every BACKUP_* key is REQUIRED — Gate G028 fail-loud.
# Postgres credentials are loaded into local variables and never echoed.
set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

COMPOSE_PROJECT="${COMPOSE_PROJECT:?COMPOSE_PROJECT not set in env file}"
POSTGRES_USER="${POSTGRES_USER:?POSTGRES_USER not set in env file}"
POSTGRES_DB="${POSTGRES_DB:?POSTGRES_DB not set in env file}"
# Spec 048 — backup envelope keys. Re-asserting these here means a stale
# env file generated before spec 048 fails loud here instead of silently
# writing artifacts to the wrong directory.
BACKUP_LOCAL_DIR="${BACKUP_LOCAL_DIR:?BACKUP_LOCAL_DIR not set — run ./smackerel.sh config generate}"
BACKUP_STATUS_FILE="${BACKUP_STATUS_FILE:?BACKUP_STATUS_FILE not set — run ./smackerel.sh config generate}"
BACKUP_RETENTION_DAILY="${BACKUP_RETENTION_DAILY:?BACKUP_RETENTION_DAILY not set — run ./smackerel.sh config generate}"
BACKUP_RETENTION_WEEKLY="${BACKUP_RETENTION_WEEKLY:?BACKUP_RETENTION_WEEKLY not set — run ./smackerel.sh config generate}"

# Resolve BACKUP_LOCAL_DIR relative to the repo root when it is not
# absolute. The product default is "./backups"; deploy adapters will
# typically set an absolute path on the target host.
case "$BACKUP_LOCAL_DIR" in
  /*) BACKUP_DIR="$BACKUP_LOCAL_DIR" ;;
  *)  BACKUP_DIR="$REPO_ROOT/$BACKUP_LOCAL_DIR" ;;
esac
case "$BACKUP_STATUS_FILE" in
  /*) STATUS_PATH="$BACKUP_STATUS_FILE" ;;
  *)  STATUS_PATH="$REPO_ROOT/$BACKUP_STATUS_FILE" ;;
esac

mkdir -p "$BACKUP_DIR"
mkdir -p "$(dirname "$STATUS_PATH")"

# Capture the start time so the status file can record duration.
RUN_STARTED_MS="$(date -u +%s%3N)"

TIMESTAMP="$(date -u +%Y-%m-%d-%H%M%S)"
FILENAME="smackerel-${TIMESTAMP}.sql.gz"
BACKUP_PATH="$BACKUP_DIR/$FILENAME"

CONTAINER_NAME="${COMPOSE_PROJECT}-postgres-1"

# Spec 048 FR-048-003 — redaction helper. Strips secret-shaped values
# from any text before it reaches a log line or the status file. The
# closed set of secret env vars below is mirrored from
# internal/backup/status.go forbiddenSecretSubstrings; keep the two in
# sync. The redaction is greedy on the VALUE so partial leaks (e.g.
# "...password=hunter2 failed to authenticate...") still get scrubbed.
redact_secrets() {
  local input="$1"
  local var val
  for var in POSTGRES_PASSWORD SMACKEREL_AUTH_TOKEN TELEGRAM_BOT_TOKEN \
             AUTH_SIGNING_ACTIVE_PRIVATE_KEY AUTH_AT_REST_HASHING_KEY \
             AUTH_BOOTSTRAP_TOKEN LLM_API_KEY HOSPITABLE_ACCESS_TOKEN \
             DISCORD_BOT_TOKEN TWITTER_BEARER_TOKEN \
             GOV_ALERTS_AIRNOW_API_KEY FINANCIAL_MARKETS_FINNHUB_API_KEY \
             FINANCIAL_MARKETS_FRED_API_KEY GUESTHOST_API_KEY; do
    val="${!var:-}"
    if [[ -n "$val" ]]; then
      input="${input//$val/[REDACTED:$var]}"
    fi
  done
  printf '%s' "$input"
}

# write_status writes the JSON status file the Go core's metrics
# watcher reads. Schema MUST match internal/backup.Status. Always
# writes both LastRunUnixtime and LastSuccessUnixtime — the latter is
# the prior success when the current run failed.
PRIOR_LAST_SUCCESS=0
if [[ -f "$STATUS_PATH" ]]; then
  PRIOR_LAST_SUCCESS="$(awk -F'[:,}]' '/last_success_unixtime/ {gsub(/[^0-9]/, "", $2); print $2; exit}' "$STATUS_PATH" 2>/dev/null || echo 0)"
  [[ -z "$PRIOR_LAST_SUCCESS" ]] && PRIOR_LAST_SUCCESS=0
fi

write_status() {
  local status="$1"          # success | failed
  local size_bytes="$2"
  local artifact_name="$3"
  local last_error_raw="${4:-}"
  local now_unix now_ms duration_ms last_success last_error_redacted

  now_unix="$(date -u +%s)"
  now_ms="$(date -u +%s%3N)"
  duration_ms=$(( now_ms - RUN_STARTED_MS ))

  if [[ "$status" == "success" ]]; then
    last_success="$now_unix"
  else
    last_success="$PRIOR_LAST_SUCCESS"
  fi

  # Redact the error string before serialization. Then JSON-escape
  # backslashes and quotes; never trust upstream tools to do this.
  last_error_redacted="$(redact_secrets "$last_error_raw")"
  last_error_redacted="${last_error_redacted//\\/\\\\}"
  last_error_redacted="${last_error_redacted//\"/\\\"}"
  last_error_redacted="${last_error_redacted//$'\n'/\\n}"
  last_error_redacted="${last_error_redacted//$'\r'/}"
  last_error_redacted="${last_error_redacted//$'\t'/\\t}"

  # Write atomically: emit to <path>.tmp then rename. The Go watcher
  # never sees a torn read mid-update.
  local tmp_path="${STATUS_PATH}.tmp"
  cat > "$tmp_path" <<JSON
{
  "schema_version": 1,
  "last_run_unixtime": ${now_unix},
  "last_success_unixtime": ${last_success},
  "last_status": "${status}",
  "last_duration_ms": ${duration_ms},
  "last_size_bytes": ${size_bytes},
  "last_artifact_name": "${artifact_name}",
  "last_error": "${last_error_redacted}"
}
JSON
  chmod 0600 "$tmp_path"
  mv "$tmp_path" "$STATUS_PATH"
}

fail_and_record() {
  local message="$1"
  local redacted
  redacted="$(redact_secrets "$message")"
  echo "ERROR: $redacted" >&2
  write_status failed 0 "" "$message"
  exit 1
}

# Verify postgres container is running.
if ! docker inspect --format='{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null | grep -q true; then
  fail_and_record "Database container is not running: $CONTAINER_NAME"
fi

echo "Starting backup of database '$POSTGRES_DB' (retention: ${BACKUP_RETENTION_DAILY} daily + ${BACKUP_RETENTION_WEEKLY} weekly)..."

# Run pg_dump inside the container, pipe through gzip. Capture stderr
# separately so redaction can scrub it before logging.
PGDUMP_STDERR_FILE="$(mktemp)"
PRUNE_KEEP_FILE="$(mktemp)"
trap 'rm -f "$PGDUMP_STDERR_FILE" "$PRUNE_KEEP_FILE"' EXIT
if ! docker exec "$CONTAINER_NAME" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists 2>"$PGDUMP_STDERR_FILE" | gzip > "$BACKUP_PATH"; then
  rm -f "$BACKUP_PATH"
  err_msg="pg_dump failed"
  if [[ -s "$PGDUMP_STDERR_FILE" ]]; then
    err_msg="$err_msg: $(redact_secrets "$(cat "$PGDUMP_STDERR_FILE")")"
  fi
  fail_and_record "$err_msg"
fi

# Show any pg_dump warnings even on success — redacted.
if [[ -s "$PGDUMP_STDERR_FILE" ]]; then
  echo "pg_dump warnings:" >&2
  redact_secrets "$(cat "$PGDUMP_STDERR_FILE")" >&2
  echo >&2
fi

# Validate output file is non-empty.
FILESIZE="$(stat --format='%s' "$BACKUP_PATH" 2>/dev/null || stat -f '%z' "$BACKUP_PATH" 2>/dev/null || echo 0)"
if [[ "$FILESIZE" -lt 100 ]]; then
  rm -f "$BACKUP_PATH"
  fail_and_record "Backup file is empty (size=$FILESIZE bytes) — dump may have failed"
fi

# Validate gzip integrity before reporting success.
if ! gunzip -t "$BACKUP_PATH" 2>/dev/null; then
  rm -f "$BACKUP_PATH"
  fail_and_record "Backup file failed gzip integrity check — dump may be corrupt"
fi

# Spec 048 FR-048-001 retention pruning. The selection logic mirrors
# internal/backup.SelectKept: walk newest-first, claim one slot per
# distinct calendar day until daily_retention slots fill, then walk
# the remainder claiming one slot per distinct ISO week (skipping the
# weeks already covered by daily slots). The Go unit tests are the
# authoritative contract; this Python re-implements the same
# algorithm so the cron path works with only postgres + python3.

python3 - "$BACKUP_DIR" "$BACKUP_RETENTION_DAILY" "$BACKUP_RETENTION_WEEKLY" "$PRUNE_KEEP_FILE" <<'PY'
import os
import re
import sys
from datetime import datetime, timezone

backup_dir = sys.argv[1]
retention_daily = int(sys.argv[2])
retention_weekly = int(sys.argv[3])
keep_file = sys.argv[4]

NAME_RE = re.compile(r"^smackerel-(\d{4}-\d{2}-\d{2}-\d{6})\.sql\.gz$")

artifacts = []
for entry in os.listdir(backup_dir):
    m = NAME_RE.match(entry)
    if not m:
        continue
    try:
        when = datetime.strptime(m.group(1), "%Y-%m-%d-%H%M%S").replace(tzinfo=timezone.utc)
    except ValueError:
        continue
    artifacts.append((when, entry))

artifacts.sort(key=lambda x: x[0], reverse=True)

kept = set()
seen_days = set()
daily_filled = 0
daily_last_idx = -1
for i, (when, name) in enumerate(artifacts):
    if daily_filled >= retention_daily:
        break
    day_key = when.strftime("%Y-%m-%d")
    if day_key in seen_days:
        continue
    seen_days.add(day_key)
    kept.add(name)
    daily_filled += 1
    daily_last_idx = i

# Weekly slots: walk artifacts AFTER the daily cutoff, skipping weeks
# already covered by daily slots so the two windows never overlap.
if retention_weekly > 0:
    seen_weeks = set()
    for i, (when, _) in enumerate(artifacts):
        if i > daily_last_idx:
            break
        seen_weeks.add(when.strftime("%G-W%V"))
    weekly_filled = 0
    for i in range(daily_last_idx + 1, len(artifacts)):
        if weekly_filled >= retention_weekly:
            break
        when, name = artifacts[i]
        week_key = when.strftime("%G-W%V")
        if week_key in seen_weeks:
            continue
        seen_weeks.add(week_key)
        kept.add(name)
        weekly_filled += 1

with open(keep_file, "w") as fh:
    for name in kept:
        fh.write(name + "\n")
PY

PRUNED_COUNT=0
KEPT_COUNT=0
for f in "$BACKUP_DIR"/smackerel-*.sql.gz; do
  [[ -f "$f" ]] || continue
  name="$(basename "$f")"
  if grep -Fxq "$name" "$PRUNE_KEEP_FILE"; then
    KEPT_COUNT=$(( KEPT_COUNT + 1 ))
  else
    rm -f "$f"
    PRUNED_COUNT=$(( PRUNED_COUNT + 1 ))
  fi
done

# Human-readable size.
if command -v numfmt >/dev/null 2>&1; then
  HUMAN_SIZE="$(numfmt --to=iec "$FILESIZE")"
else
  HUMAN_SIZE="${FILESIZE} bytes"
fi

write_status success "$FILESIZE" "$FILENAME" ""

echo "Backup created: $BACKUP_PATH ($HUMAN_SIZE)"
echo "Retention applied: kept ${KEPT_COUNT} artifacts, pruned ${PRUNED_COUNT}"
echo "Status written: $STATUS_PATH"
