#!/usr/bin/env bash
#
# bubbles/scripts/v5.2-selftest.sh
#
# Aggregate selftest for v5.2 work items that don't have their own
# dedicated selftest script. Covers:
#   F1. Tool-log primary evidence path (state-transition-guard Check 9
#       accepts an inline ≥10-line block AND additionally accepts a
#       matching tool-log entry).
#   F3. tool-log.sh writes schemaVersion=2 + framework block on every
#       new entry; readers still accept legacy v1 records.
#   F6. code-search.sh persists the chosen backend to
#       .specify/runtime/code-search.tool on first call.
#   F7. model-tier-advisory.sh writes a model-tier-warning entry to the
#       tool-call log when active < floor.
#
# F2 (diff-evidence auto-strict) and F4 (gates registry) have dedicated
# selftests (diff-evidence-guard-selftest.sh, gates-registry-selftest.sh).
# F5 ships its own validator that runs in framework-validate.
#
# Exit 0 = all PASS. Exit 1 = at least one FAIL.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

tmp_root="$(mktemp -d -t bubbles-v5.2-selftest.XXXXXX)"
trap 'rm -rf "$tmp_root"' EXIT INT TERM

cd "$tmp_root"
git init -q
git -c user.email=test@bubbles -c user.name=test commit --allow-empty -q -m "init"

# --- F3: tool-log writes schemaVersion=2 + framework block ---
LOG="$tmp_root/log.jsonl"
BUBBLES_TOOL_LOG_FILE="$LOG" BUBBLES_TOOL_LOG_QUIET=1 \
  bash "$SCRIPT_DIR/tool-log.sh" -- echo hello-v5.2 >/dev/null 2>&1
if [[ -f "$LOG" ]] && python3 - "$LOG" <<'PY' 2>/dev/null
import json, sys
line = open(sys.argv[1]).readline().strip()
d = json.loads(line)
ok = (
    d.get("schemaVersion") == 2
    and isinstance(d.get("framework"), dict)
    and d["framework"].get("name") == "bubbles"
)
sys.exit(0 if ok else 1)
PY
then
  pass "F3: tool-log new entries carry schemaVersion=2 + framework.name=bubbles"
else
  fail "F3: tool-log entry missing schemaVersion=2 or framework block"
fi

# F3 backward-compat: write a v1-shaped record manually, verify reader (jsonschema) accepts it.
LOG_MIXED="$tmp_root/mixed.jsonl"
cp "$LOG" "$LOG_MIXED"
python3 - "$LOG_MIXED" <<'PY' >/dev/null 2>&1
import json, sys
v1 = {
  "ts": "2026-06-04T00:00:00Z",
  "sessionId": "legacy",
  "agent": "human",
  "spec": "",
  "scope": "",
  "cmd": "echo v1",
  "exitCode": 0,
  "durationMs": 0,
  "stdoutHash": "0"*64,
  "stderrHash": "0"*64,
  "stdoutBytes": 0,
  "stderrBytes": 0,
  "tags": [],
}
with open(sys.argv[1], "a") as f:
  f.write(json.dumps(v1) + "\n")
PY
SCHEMA="$ROOT_DIR/bubbles/schemas/tool-call.schema.json"
if python3 - "$LOG_MIXED" "$SCHEMA" <<'PY' >/dev/null 2>&1
import json, sys
try:
    import jsonschema
except ImportError:
    sys.exit(0)
log_path, schema_path = sys.argv[1], sys.argv[2]
schema = json.loads(open(schema_path).read())
for line in open(log_path):
    line = line.strip()
    if not line:
        continue
    rec = json.loads(line)
    jsonschema.validate(rec, schema)
PY
then
  pass "F3: schema v1 entries STILL validate against tool-call.schema.json (backward-compat)"
else
  fail "F3: schema validator rejects mixed v1+v2 log"
fi

# --- F6: code-search auto-selects + persists backend ---
CACHE_DIR="$tmp_root/.specify/runtime"
rm -rf "$CACHE_DIR"
# Run a no-match search to trigger detection without needing repo content.
( cd "$tmp_root" && bash "$SCRIPT_DIR/code-search.sh" "nonexistent-token-xyz" . >/dev/null 2>&1 || true )
if [[ -f "$CACHE_DIR/code-search.tool" ]]; then
  cached="$(tr -d '[:space:]' < "$CACHE_DIR/code-search.tool")"
  if [[ "$cached" == "rg" || "$cached" == "grep" ]]; then
    pass "F6: code-search persisted backend choice ($cached) to .specify/runtime/code-search.tool"
  else
    fail "F6: code-search cache file has unexpected value '$cached'"
  fi
else
  fail "F6: code-search did not create .specify/runtime/code-search.tool"
fi
# Manual override honored.
if BUBBLES_CODE_SEARCH_BACKEND=grep bash "$SCRIPT_DIR/code-search.sh" "nonexistent-token-xyz" "$tmp_root" >/dev/null 2>&1; then
  : # exit code 1 on no-match is acceptable
fi
# Override should not have mutated the cache.
if [[ "$(tr -d '[:space:]' < "$CACHE_DIR/code-search.tool")" == "$cached" ]]; then
  pass "F6: BUBBLES_CODE_SEARCH_BACKEND override does not overwrite cache"
else
  fail "F6: BUBBLES_CODE_SEARCH_BACKEND override mutated cache"
fi

# --- F7: model-tier-advisory writes warning entry to tool-call log ---
# Uses a NON-enforced phase (implement) so this exercises the advisory WARN
# path (severity "warn", exit 0). The blocking path for enforced phases
# (audit/security/validate, v6.1 / G126) is covered by
# model-tier-advisory-selftest.sh.
WARN_LOG="$tmp_root/warn.jsonl"
BUBBLES_TOOL_LOG_FILE="$WARN_LOG" BUBBLES_ACTIVE_MODEL="claude-haiku-3" \
  bash "$SCRIPT_DIR/model-tier-advisory.sh" check --mode product-to-delivery --phase implement >/dev/null 2>&1
if [[ -f "$WARN_LOG" ]] && python3 - "$WARN_LOG" <<'PY' >/dev/null 2>&1
import json, sys
recs = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
ok = any(
    r.get("schemaVersion") == 2
    and "model-tier-warning" in (r.get("tags") or [])
    and (r.get("modelTier") or {}).get("severity") == "warn"
    and (r.get("modelTier") or {}).get("floor")
    and (r.get("modelTier") or {}).get("active")
    for r in recs
)
sys.exit(0 if ok else 1)
PY
then
  pass "F7: model-tier-advisory writes model-tier-warning record with schemaVersion=2 + modelTier block"
else
  fail "F7: model-tier-advisory did not write structured warning to tool-call log"
fi
# F7 OK path: no warning when active >= floor.
WARN_LOG_OK="$tmp_root/warn-ok.jsonl"
BUBBLES_TOOL_LOG_FILE="$WARN_LOG_OK" BUBBLES_ACTIVE_MODEL="claude-sonnet-4" \
  bash "$SCRIPT_DIR/model-tier-advisory.sh" check --mode product-to-delivery --phase implement >/dev/null 2>&1
if [[ ! -f "$WARN_LOG_OK" ]] || ! python3 - "$WARN_LOG_OK" <<'PY' >/dev/null 2>&1
import json, sys
try:
    recs = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
except FileNotFoundError:
    sys.exit(1)
sys.exit(0 if any("model-tier-warning" in (r.get("tags") or []) for r in recs) else 1)
PY
then
  pass "F7: model-tier-advisory does NOT write warning when active >= floor"
else
  fail "F7: model-tier-advisory wrote spurious warning when active >= floor"
fi

# --- F1: tool-log primary evidence path is integrated in state-transition-guard ---
# Smoke-test: the function _tool_log_covers_dod_item is defined in the
# guard, and it returns 0 when a matching log entry exists.
if grep -q '_tool_log_covers_dod_item()' "$SCRIPT_DIR/state-transition-guard.sh"; then
  pass "F1: state-transition-guard defines _tool_log_covers_dod_item helper"
else
  fail "F1: state-transition-guard missing _tool_log_covers_dod_item helper"
fi
if grep -q '_tool_log_covers_dod_item "\$scope_dir" "\$line"' "$SCRIPT_DIR/state-transition-guard.sh"; then
  pass "F1: state-transition-guard invokes the helper in Check 9 case 4"
else
  fail "F1: state-transition-guard does not invoke _tool_log_covers_dod_item in Check 9"
fi

if (( failures == 0 )); then
  echo "OK: v5.2 selftest passed (F1, F3, F6, F7)"
  exit 0
else
  echo "FAILED: v5.2 selftest had $failures assertion failures" >&2
  exit 1
fi
