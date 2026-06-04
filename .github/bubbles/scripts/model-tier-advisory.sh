#!/usr/bin/env bash
#
# bubbles model-tier-advisory.sh — model capability advisor (v5.1 / M7).
#
# Reads workflows.yaml `modeDefaults.modelFloor` (and per-mode override) and
# compares against the runtime model identifier reported by the host client.
# Emits a warning when the active model is below the floor for the requested
# phase. Advisory in v5.1; v6 (S9) makes it blocking.
#
# Usage:
#   model-tier-advisory.sh check --mode <mode> --phase <phase>
#   model-tier-advisory.sh resolve --mode <mode> --phase <phase>   # prints floor
#
# Environment:
#   BUBBLES_ACTIVE_MODEL    identifier of the model in use (e.g. 'sonnet-4.5',
#                           'opus-4.7', 'gpt-5'). When unset, exits with code 0
#                           and emits a "model-tier: model-unknown" notice.
#
# Tier ranking (low → high):
#   haiku-class  < sonnet-class  < opus-class
#   plus exact identifiers; unknown identifiers are treated as 'sonnet-class'
#   so the advisor is friendly to new releases.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS="$REPO_ROOT/bubbles/workflows.yaml"

usage() {
  cat >&2 <<'USAGE'
Usage:
  model-tier-advisory.sh check --mode <mode> --phase <phase>
  model-tier-advisory.sh resolve --mode <mode> --phase <phase>

Reads workflows.yaml model-tier policy and advises whether the active model
(BUBBLES_ACTIVE_MODEL) meets the floor for <mode>/<phase>. Advisory only.
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
OP="$1"; shift
MODE=""
PHASE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2;;
    --phase) PHASE="$2"; shift 2;;
    -h|--help) usage; exit 0;;
    *) usage; exit 2;;
  esac
done

[[ -z "$MODE" || -z "$PHASE" ]] && { usage; exit 2; }
[[ -f "$WORKFLOWS" ]] || { echo "model-tier-advisory: workflows.yaml missing" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "model-tier-advisory: SKIP (python3 not installed)"
  exit 0
fi

ACTIVE="${BUBBLES_ACTIVE_MODEL:-}"

WORKFLOWS="$WORKFLOWS" OP="$OP" MODE="$MODE" PHASE="$PHASE" ACTIVE="$ACTIVE" python3 - <<'PY'
import os, sys

try:
    import yaml
except ImportError:
    print("model-tier-advisory: SKIP (PyYAML not installed)")
    sys.exit(0)

workflows = os.environ['WORKFLOWS']
op = os.environ['OP']
mode = os.environ['MODE']
phase = os.environ['PHASE']
active = os.environ.get('ACTIVE', '').strip()

with open(workflows) as f:
    data = yaml.safe_load(f)

# Resolve floor: per-mode-per-phase > per-mode > modeDefaults.
default_floor = (data.get('modeDefaults') or {}).get('modelFloor', {}) or {}
modes = data.get('modes') or {}
mode_block = modes.get(mode) or {}
mode_phase_floor = (mode_block.get('modelFloor') or {}) if isinstance(mode_block.get('modelFloor'), dict) else {}

floor = mode_phase_floor.get(phase) \
     or (mode_block.get('modelFloor') if isinstance(mode_block.get('modelFloor'), str) else None) \
     or default_floor.get(phase) \
     or (default_floor.get('default') if isinstance(default_floor, dict) else None)

# Tier ranking.
TIER_RANK = {
    'haiku-class': 1,
    'sonnet-class': 2,
    'opus-class': 3,
}

def tier_of(model_id: str) -> int:
    if not model_id:
        return 0
    mid = model_id.lower()
    if 'haiku' in mid:
        return TIER_RANK['haiku-class']
    if 'opus' in mid:
        return TIER_RANK['opus-class']
    if 'gpt-5' in mid or 'gpt5' in mid:
        return TIER_RANK['opus-class']
    if 'sonnet' in mid or 'gpt-4' in mid:
        return TIER_RANK['sonnet-class']
    # Unknown: treat as sonnet-class so we don't false-block new releases.
    return TIER_RANK['sonnet-class']

if op == 'resolve':
    print(floor or '')
    sys.exit(0)

if op != 'check':
    print(f"model-tier-advisory: unknown op: {op}", file=sys.stderr)
    sys.exit(2)

if not floor:
    print(f"model-tier: no floor declared for mode={mode} phase={phase}")
    sys.exit(0)

floor_rank = TIER_RANK.get(floor, 0)
if floor_rank == 0:
    print(f"model-tier: unknown floor identifier '{floor}' (mode={mode} phase={phase}) — advisory skipped")
    sys.exit(0)

if not active:
    print(f"model-tier: model-unknown (mode={mode} phase={phase} floor={floor}) — set BUBBLES_ACTIVE_MODEL to enable advisory")
    sys.exit(0)

active_rank = tier_of(active)
if active_rank >= floor_rank:
    print(f"model-tier: OK (mode={mode} phase={phase} floor={floor} active={active})")
    sys.exit(0)

# Below floor — advisory warning.
# v5.2 / F7: also write a durable, auditable entry to the tool-call log
# so warnings survive past the operator's scrollback and are queryable
# alongside command evidence. Schema v2 entry with tag 'model-tier-warning'.
import json, os, subprocess, datetime, hashlib, getpass
warn_msg = f"model-tier: WARN — active model '{active}' is below floor '{floor}' for mode={mode} phase={phase}"
print(warn_msg)
print("  Advisory only in v5.1+; v6 S9 will make this blocking.")
print("  Recommended: re-run this phase with a model at or above the declared floor.")

# Best-effort durable write to tool-call log.
try:
    repo_root = subprocess.check_output(['git', 'rev-parse', '--show-toplevel'], stderr=subprocess.DEVNULL, text=True).strip()
except Exception:
    repo_root = os.getcwd()
log_dir = os.path.join(repo_root, '.specify', 'runtime')
log_path = os.environ.get('BUBBLES_TOOL_LOG_FILE') or os.path.join(log_dir, 'tool-calls.jsonl')
try:
    os.makedirs(os.path.dirname(log_path), exist_ok=True)
    # Framework provenance — same shape tool-log.sh writes.
    framework = {"name": "bubbles"}
    v_file = os.path.join(repo_root, '.github', 'bubbles', '.version')
    if not os.path.exists(v_file):
        v_file = os.path.join(repo_root, 'VERSION')
    if os.path.exists(v_file):
        try:
            framework["version"] = open(v_file).read().strip()
        except Exception:
            pass
    cmd_label = f"model-tier-advisory check --mode {mode} --phase {phase}"
    now_utc = datetime.datetime.now(datetime.timezone.utc)
    record = {
        "schemaVersion": 2,
        "ts": now_utc.strftime('%Y-%m-%dT%H:%M:%SZ'),
        "sessionId": os.environ.get('BUBBLES_SESSION_ID') or f"model-tier-{now_utc.strftime('%Y%m%dT%H%M%S')}-{os.getpid()}",
        "agent": os.environ.get('BUBBLES_AGENT_NAME', 'model-tier-advisory'),
        "spec": os.environ.get('BUBBLES_SPEC', ''),
        "scope": os.environ.get('BUBBLES_SCOPE', ''),
        "cmd": cmd_label,
        "cwd": os.getcwd(),
        "exitCode": 0,
        "durationMs": 0,
        # Hash payload deterministically so identical warnings collapse for analysis.
        "stdoutHash": hashlib.sha256(warn_msg.encode()).hexdigest(),
        "stderrHash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",  # sha256("")
        "stdoutBytes": len(warn_msg),
        "stderrBytes": 0,
        "tags": ["model-tier-warning"],
        "framework": framework,
        "modelTier": {
            "mode": mode,
            "phase": phase,
            "floor": floor,
            "active": active,
            "severity": "warn",
        },
    }
    with open(log_path, 'a') as f:
        f.write(json.dumps(record, separators=(',', ':')) + '\n')
except Exception as e:
    # Non-fatal — advisory should never break a workflow because of log I/O.
    print(f"  (model-tier: tool-log entry skipped: {e})")

sys.exit(0)
PY
