#!/usr/bin/env bash
#
# Spec 061 SCOPE-01 DoD #4 — assistant SST config-generate functional test.
#
# Sub-test A (happy path):
#   - `./smackerel.sh config generate` exits 0
#   - The resulting config/generated/dev.env contains every ASSISTANT_*
#     env var listed in scripts/commands/config.sh assistant block
#     (25 leaf env vars).
#
# Sub-test B (missing-key fail-loud):
#   - A tampered copy of config/smackerel.yaml with the
#     `borderline_floor:` line removed under the assistant block is
#     passed via `--config` to the SST loader.
#   - The loader exits non-zero.
#   - stderr contains `Missing config key: assistant.borderline_floor`
#     (the canonical fail-loud message emitted by `required_value`
#     in scripts/commands/config.sh).
#
# Output isolation:
#   The smackerel.sh CLI's `config generate` writes
#   $REPO_ROOT/config/generated/<env>.env. To keep the operator's
#   working state untouched, this script backs up any pre-existing
#   dev.env / test.env BEFORE running and restores them after,
#   mirroring tests/config/postgres_dev_default_env_override_test.sh.
#
# Invoked by `./smackerel.sh test integration` via the shell-test
# harness; runnable standalone for ad-hoc verification.

set -uo pipefail

if [[ -z "${REPO_ROOT:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

SMACKEREL_SH="$REPO_ROOT/smackerel.sh"
CONFIG_SH="$REPO_ROOT/scripts/commands/config.sh"
GENERATED_DIR="$REPO_ROOT/config/generated"
SOURCE_YAML="$REPO_ROOT/config/smackerel.yaml"

if [[ ! -f "$SMACKEREL_SH" ]] || [[ ! -f "$CONFIG_SH" ]] || [[ ! -f "$SOURCE_YAML" ]]; then
  echo "FATAL: missing one of smackerel.sh / scripts/commands/config.sh / config/smackerel.yaml under $REPO_ROOT" >&2
  exit 1
fi

# Expected ASSISTANT_* env vars in dev.env (extracted directly from the
# scripts/commands/config.sh assistant block — single source of truth).
EXPECTED_KEYS=(
  ASSISTANT_ENABLED
  ASSISTANT_BORDERLINE_FLOOR
  ASSISTANT_CONTEXT_WINDOW_TURNS
  ASSISTANT_CONTEXT_IDLE_TIMEOUT
  ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL
  ASSISTANT_CONTEXT_STATE_KEY
  ASSISTANT_SOURCES_MAX
  ASSISTANT_BODY_MAX_CHARS
  ASSISTANT_STATUS_MAX_DURATION
  ASSISTANT_DISAMBIGUATE_TIMEOUT
  ASSISTANT_ERROR_CAPTURE_TIMEOUT
  ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM
  ASSISTANT_RATE_LIMIT_WEATHER_RPM
  ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM
  ASSISTANT_SKILLS_RETRIEVAL_ENABLED
  ASSISTANT_SKILLS_RETRIEVAL_TOP_K
  ASSISTANT_SKILLS_WEATHER_ENABLED
  ASSISTANT_SKILLS_WEATHER_PROVIDER
  ASSISTANT_SKILLS_WEATHER_CACHE_TTL
  ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED
  ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT
  ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED
  ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE
  ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS
)

failures=0
SCOPE_TMP="$(mktemp -d)"

# Backup pre-existing generated env files.
BACKUP_DIR="$SCOPE_TMP/backup"
mkdir -p "$BACKUP_DIR"
for f in dev.env test.env; do
  if [[ -r "$GENERATED_DIR/$f" ]]; then
    cp "$GENERATED_DIR/$f" "$BACKUP_DIR/$f" 2>/dev/null || true
  fi
done

restore_backups() {
  for f in dev.env test.env; do
    if [[ -f "$BACKUP_DIR/$f" ]]; then
      cp "$BACKUP_DIR/$f" "$GENERATED_DIR/$f" 2>/dev/null || true
    fi
  done
  rm -rf "$SCOPE_TMP"
}
trap restore_backups EXIT

# ---------------------------------------------------------------------------
# Sub-test A — happy-path config generate emits all ASSISTANT_* vars.
# ---------------------------------------------------------------------------
echo "--- Sub-test A: ./smackerel.sh config generate emits all ASSISTANT_* env vars in dev.env ---"

A_LOG="$SCOPE_TMP/generate.log"
( cd "$REPO_ROOT" && "$SMACKEREL_SH" config generate ) > "$A_LOG" 2>&1
A_EXIT=$?

if [[ "$A_EXIT" -ne 0 ]]; then
  echo "  FAIL: ./smackerel.sh config generate exited $A_EXIT (expected 0)"
  echo "  tail of generate log:"
  tail -20 "$A_LOG" | sed 's|^|    |'
  failures=$((failures + 1))
else
  echo "  OK: ./smackerel.sh config generate exited 0"
fi

if [[ ! -f "$GENERATED_DIR/dev.env" ]]; then
  echo "  FAIL: $GENERATED_DIR/dev.env was not created"
  failures=$((failures + 1))
else
  missing_count=0
  for key in "${EXPECTED_KEYS[@]}"; do
    if ! grep -qE "^${key}=" "$GENERATED_DIR/dev.env"; then
      echo "  FAIL: dev.env missing expected key '$key'"
      missing_count=$((missing_count + 1))
    fi
  done
  if [[ "$missing_count" -eq 0 ]]; then
    echo "  OK: all ${#EXPECTED_KEYS[@]} ASSISTANT_* keys present in dev.env"
  else
    failures=$((failures + missing_count))
  fi
fi

# ---------------------------------------------------------------------------
# Sub-test B — missing required key fails loudly with explicit key path.
# ---------------------------------------------------------------------------
echo "--- Sub-test B: tampered smackerel.yaml (assistant.borderline_floor removed) fails loudly ---"

TAMPERED_YAML="$SCOPE_TMP/smackerel-no-borderline.yaml"
# Delete the line matching `^  borderline_floor:` directly under the
# `assistant:` block. The leading two-space indent is the canonical
# layout in config/smackerel.yaml and the key is unique under that
# indent — a precise enough match for this regression test without
# fragile YAML parsing.
sed -E '/^[[:space:]]{2}borderline_floor:[[:space:]]/d' "$SOURCE_YAML" > "$TAMPERED_YAML"

# Quick adversarial sanity check: the tampered file MUST differ from the
# source (otherwise the sed pattern is wrong and Sub-test B would be
# tautological).
if cmp -s "$SOURCE_YAML" "$TAMPERED_YAML"; then
  echo "  FAIL: tampered YAML is byte-identical to source — sed pattern did not remove borderline_floor"
  failures=$((failures + 1))
else
  echo "  OK: tampered YAML differs from source (sed pattern removed borderline_floor)"
fi

B_LOG="$SCOPE_TMP/missing.log"
( cd "$REPO_ROOT" && bash "$CONFIG_SH" --env test --config "$TAMPERED_YAML" ) > "$B_LOG" 2>&1
B_EXIT=$?

if [[ "$B_EXIT" -eq 0 ]]; then
  echo "  FAIL: loader exited 0 with borderline_floor removed (expected non-zero)"
  echo "  tail of loader log:"
  tail -20 "$B_LOG" | sed 's|^|    |'
  failures=$((failures + 1))
else
  echo "  OK: loader exited $B_EXIT (non-zero, as expected)"
fi

if ! grep -qF 'Missing config key: assistant.borderline_floor' "$B_LOG"; then
  echo "  FAIL: loader output did not contain 'Missing config key: assistant.borderline_floor'"
  echo "  tail of loader log:"
  tail -20 "$B_LOG" | sed 's|^|    |'
  failures=$((failures + 1))
else
  echo "  OK: loader output names the missing key path explicitly"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "PASS: tests/config/assistant_config_generate_test.sh — all sub-tests passed"
  exit 0
else
  echo "FAIL: tests/config/assistant_config_generate_test.sh — $failures sub-test failures"
  exit 1
fi
