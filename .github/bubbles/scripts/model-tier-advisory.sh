#!/usr/bin/env bash
#
# bubbles model-tier-advisory.sh — model capability advisor (v5.1 / M7).
#
# Reads workflows.yaml `modeDefaults.modelFloor` (and per-mode override) and
# compares against the runtime model identifier reported by the host client.
# For phases listed in `modeDefaults.modelFloorEnforcedPhases` (audit, security,
# validate by default) the floor is BLOCKING (v6.1 / S9): `check` exits non-zero
# when the active model is known and below floor. For all other phases it stays
# advisory (warning, exit 0). Unknown model or undeclared floor never blocks.
#
# Usage:
#   model-tier-advisory.sh check [--enforce] --mode <mode> --phase <phase>
#   model-tier-advisory.sh resolve --mode <mode> --phase <phase>   # prints floor
#   model-tier-advisory.sh retirement [--tier <tier>]              # IMP-027/S11
#   model-tier-advisory.sh typed --mode <mode> --phase <phase>
#     --model-class <class> [--model-identity <id> --model-verified]
#
# `retirement` reports which `modelCompensation` gates have met the TIER half
# of their registry `retireWhen` criterion at the given (or active) model tier.
# It never turns a gate off, and it prints the unmet EVIDENCE half every time:
# no harness yet drives a model across the golden-task corpus to produce the
# rates those criteria are written against. See gate-retirement.sh.
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
# BUBBLES_WORKFLOWS_FILE override exists for hermetic selftests and downstream
# repos that relocate workflows.yaml; defaults to the in-tree path.
WORKFLOWS="${BUBBLES_WORKFLOWS_FILE:-$REPO_ROOT/bubbles/workflows.yaml}"
# IMP-058 SCOPE-1 / REG-22: the gate registry, not workflows.yaml, is where
# `classification` and `retireWhen` live. `retirement` reads this file; every
# other operation (check/resolve/typed) keeps reading WORKFLOWS above.
GATES="${BUBBLES_GATES_FILE:-$REPO_ROOT/bubbles/registry/gates.yaml}"

usage() {
  cat >&2 <<'USAGE'
Usage:
  model-tier-advisory.sh check [--enforce] --mode <mode> --phase <phase>
  model-tier-advisory.sh resolve --mode <mode> --phase <phase>
  model-tier-advisory.sh retirement [--tier <tier>]

Reads workflows.yaml model-tier policy and checks whether the active model
(BUBBLES_ACTIVE_MODEL) meets the floor for <mode>/<phase>. BLOCKING (exit 1)
for enforced phases (modeDefaults.modelFloorEnforcedPhases or --enforce) when the
active model is known and below floor; advisory (exit 0) otherwise. Never blocks
when the model is unknown or no floor is declared.

`retirement` reports gate retirement CANDIDACY at a model tier. It is always
advisory and can never retire a gate: the measurement half of every criterion
is unmet because no harness produces those rates yet.
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
OP="$1"; shift
MODE=""
PHASE=""
ENFORCE="0"
TIER=""
MODEL_CLASS="none"
MODEL_IDENTITY=""
MODEL_VERIFIED="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2;;
    --phase) PHASE="$2"; shift 2;;
    --tier) TIER="$2"; shift 2;;
    --enforce) ENFORCE="1"; shift;;
    --model-class) MODEL_CLASS="$2"; shift 2;;
    --model-identity) MODEL_IDENTITY="$2"; shift 2;;
    --model-verified) MODEL_VERIFIED="true"; shift;;
    -h|--help) usage; exit 0;;
    *) usage; exit 2;;
  esac
done

[[ -f "$WORKFLOWS" ]] || { echo "model-tier-advisory: workflows.yaml missing" >&2; exit 2; }

if [[ "$OP" == "typed" ]]; then
    [[ -n "$MODE" && -n "$PHASE" ]] || { usage; exit 2; }
    case "$MODEL_CLASS" in
        none|economy-reasoning|standard-reasoning|high-assurance-reasoning) ;;
        *) echo "model-tier-advisory: unknown model class" >&2; exit 2 ;;
    esac
    if [[ "$MODEL_VERIFIED" == "true" && -z "$MODEL_IDENTITY" ]]; then
        echo "model-tier-advisory: verified identity requires --model-identity" >&2
        exit 2
    fi
    command -v jq >/dev/null 2>&1 || { echo "model-tier-advisory: jq is required for typed output" >&2; exit 2; }
    identity_state="unverified"
    [[ "$MODEL_VERIFIED" == "true" ]] && identity_state="verified"
    material="$(jq -cnS --arg class "$MODEL_CLASS" --arg identity "$MODEL_IDENTITY" --arg state "$identity_state" --arg mode "$MODE" --arg phase "$PHASE" '{contractType:"model-class-decision",mode:$mode,modelClass:$class,modelIdentity:(if $identity == "" then null else $identity end),modelIdentityState:$state,phase:$phase,schemaVersion:1}')"
    if command -v sha256sum >/dev/null 2>&1; then
        digest="$(printf '%s' "$material" | sha256sum | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        digest="$(printf '%s' "$material" | shasum -a 256 | awk '{print $1}')"
    else
        echo "model-tier-advisory: sha256 utility is required" >&2
        exit 2
    fi
    printf '%s' "$material" | jq -cS --arg digest "sha256:$digest" '. + {decisionDigest:$digest}'
    exit 0
fi

# Resolve the managed interpreter before probing, so a provisioned environment
# satisfies the import even when PATH's python3 does not. See
# bubbles/scripts/python-env.sh. No-op when unprovisioned.
_mta_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
[[ -f "$_mta_dir/dependency-posture.sh" ]] && . "$_mta_dir/dependency-posture.sh"
unset _mta_dir

if ! command -v python3 >/dev/null 2>&1; then
  echo "model-tier-advisory: SKIP (python3 not installed)"
  exit 0
fi

# IMP-027 / SCOPE-11 — obsolescence curve. Handled before the mode/phase
# requirement below because retirement candidacy is a property of the gate
# registry and the model tier, not of any single mode/phase.
if [[ "$OP" == "retirement" ]]; then
  # BUBBLES_GATE_HIT_ROOT override exists for hermetic selftests, mirroring
  # BUBBLES_GATES_FILE/BUBBLES_WORKFLOWS_FILE above: it lets a fixture point
  # gate-hit-log.sh at synthetic telemetry instead of the real repo's
  # .specify/runtime/gate-hits.jsonl.
  GATES="$GATES" TIER="${TIER:-${BUBBLES_ACTIVE_MODEL:-}}" \
    REPO_ROOT="${BUBBLES_GATE_HIT_ROOT:-$REPO_ROOT}" \
    GATE_HIT_LOG="$SCRIPT_DIR/gate-hit-log.sh" python3 - <<'PY'
import json
import os
import subprocess
import sys

try:
    import yaml
except ImportError:
    print("model-tier-advisory: SKIP (PyYAML not installed)")
    sys.exit(0)

with open(os.environ['GATES']) as f:
    data = yaml.safe_load(f) or {}

TIER_RANK = {'haiku-class': 1, 'sonnet-class': 2, 'opus-class': 3}


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
    return TIER_RANK['sonnet-class']


declared = (os.environ.get('TIER') or '').strip()
rank = tier_of(declared)

gates = {
    gid: meta
    for gid, meta in (data.get('gates') or {}).items()
    if isinstance(meta, dict) and str(meta.get('classification')) == 'modelCompensation'
}

eligible, blocked, uncharted = [], [], []
for gid in sorted(gates):
    crit = gates[gid].get('retireWhen')
    if not isinstance(crit, dict) or 'minTier' not in crit:
        uncharted.append(gid)
        continue
    need = TIER_RANK.get(str(crit['minTier']), 0)
    row = (gid, crit)
    if rank and rank >= need:
        eligible.append(row)
    else:
        blocked.append(row)

if not declared:
    print("model-tier retirement: model-unknown — set BUBBLES_ACTIVE_MODEL or "
          "pass --tier to evaluate the tier half of each criterion")
else:
    print(f"model-tier retirement: evaluating at tier '{declared}'")
print(f"  modelCompensation gates: {len(gates)}")

if uncharted:
    print(f"  NO CRITERION RECORDED ({len(uncharted)}): {', '.join(uncharted)}")
    print("    These carry unbounded cost in time. Run: gate-retirement.sh bind")

if declared:
    print(f"  tier precondition MET ({len(eligible)}): "
          + (', '.join(g for g, _ in eligible) if eligible else "none"))
    print(f"  tier precondition NOT met ({len(blocked)}): "
          + (', '.join(f"{g}(needs {c['minTier']})" for g, c in blocked)
             if blocked else "none"))

print("")
print("  NOTHING IS RETIRED BY THIS REPORT. Each RATE criterion above has two")
print("  halves and only the TIER half is evaluated here.")
print("  The EVIDENCE half is UNMET for every rate criterion without exception:")
print("  retiring a gate on a rate requires that rate measured below its")
print("  threshold across its window of real model runs, and no harness")
print("  produces those rates yet. The golden-task corpus scores a delivered")
print("  artifact; it does not drive a model, so it cannot report how often a")
print("  tier produces a dishonest one. Turning a gate off on tier eligibility")
print("  alone would substitute 'the model is probably better now' for a")
print("  measurement — the exact move the rate criterion exists to prevent.")
print("  See the preventionEvidence section below for the one evidence form")
print("  this report CAN evaluate today.")

# IMP-058 SCOPE-2 / COV-24 — the second criterion form. A gate may declare
# `preventionEvidence: { minRuns, prevented, sourceClass }` alongside (never
# instead of) its rate criterion. Unlike the rate, this is satisfied directly
# from gate-hit-log.sh's own product-run telemetry, so it is decidable today.
# A gate's PREVENTION history alone (no minRuns needed) proves it is earning
# its cost; only the "never prevented" case needs a run-count floor to tell
# "not yet observed enough" apart from "observed plenty, never mattered".
earning, candidate, unmeasured = [], [], []
hit_by_gate = {}
gate_hit_log = os.environ.get('GATE_HIT_LOG', '')
if gate_hit_log and os.path.isfile(gate_hit_log):
    try:
        proc = subprocess.run(
            [gate_hit_log, 'report', '--json', '--repo-root',
             os.environ.get('REPO_ROOT', '')],
            capture_output=True, text=True, timeout=30,
        )
        hits = json.loads(proc.stdout or '{}')
        for row in hits.get('gates') or []:
            gid = row.get('gate')
            if gid:
                hit_by_gate[gid] = row
    except (OSError, subprocess.SubprocessError, ValueError):
        hit_by_gate = {}

for gid in sorted(gates):
    row = hit_by_gate.get(gid)
    prevented = int(row.get('prevented', 0)) if row else 0
    fired = int(row.get('fired', 0)) if row else 0
    pe = gates[gid].get('preventionEvidence')
    if prevented >= 1:
        earning.append((gid, fired, prevented))
    elif isinstance(pe, dict) and 'minRuns' in pe and fired >= int(pe['minRuns']):
        candidate.append((gid, fired, prevented))
    else:
        unmeasured.append((gid, fired, prevented))

print("")
print("  preventionEvidence report (IMP-058 SCOPE-2 / COV-24) — decidable from")
print("  gate-hit-log.sh product telemetry, no model run required:")
print(f"  EARNING ({len(earning)}) — prevented at least once, cost is proven: "
      + (', '.join(f"{g}(prevented={p}/{f})" for g, f, p in earning)
         if earning else "none"))
print(f"  CANDIDATE ({len(candidate)}) — fired >= declared minRuns, never "
      "prevented: "
      + (', '.join(f"{g}(fired={f})" for g, f, p in candidate)
         if candidate else "none"))
print(f"  UNMEASURED ({len(unmeasured)}) — no telemetry, or below the "
      "declared minRuns, or no preventionEvidence declared: "
      + (', '.join(g for g, f, p in unmeasured) if unmeasured else "none"))
print("")
print("  CANDIDATE is not retirement. It means the owner can now make an")
print("  informed call instead of waiting on an unmeasurable rate — a gate")
print("  judged load-bearing for reasons the hit log cannot see stays,")
print("  regardless of its record count.")
sys.exit(0)
PY
  exit 0
fi

[[ -z "$MODE" || -z "$PHASE" ]] && { usage; exit 2; }

ACTIVE="${BUBBLES_ACTIVE_MODEL:-}"

WORKFLOWS="$WORKFLOWS" OP="$OP" MODE="$MODE" PHASE="$PHASE" ACTIVE="$ACTIVE" FORCE_ENFORCE="$ENFORCE" python3 - <<'PY'
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

# v6.1 (S9 / R4): which phases enforce the floor as BLOCKING vs advisory.
enforced_phases = set((data.get('modeDefaults') or {}).get('modelFloorEnforcedPhases') or [])
force_enforce = os.environ.get('FORCE_ENFORCE', '0') == '1'
enforce = force_enforce or (phase in enforced_phases)

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

# Below floor.
# For enforced phases (modeDefaults.modelFloorEnforcedPhases or --enforce) this
# is BLOCKING (exit 1); otherwise advisory (exit 0). v5.2 / F7: write a durable,
# auditable entry to the tool-call log either way so the signal survives past
# the operator's scrollback and is queryable alongside command evidence.
import json, os, subprocess, datetime, hashlib, getpass
severity = "blocked" if enforce else "warn"
level = "BLOCKED" if enforce else "WARN"
warn_msg = f"model-tier: {level} — active model '{active}' is below floor '{floor}' for mode={mode} phase={phase}"
print(warn_msg)
if enforce:
    print("  BLOCKING (v6.1 / S9 / G126): this phase requires a model at or above the declared floor.")
else:
    print(f"  Advisory: phase '{phase}' is not in modeDefaults.modelFloorEnforcedPhases.")
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
        "exitCode": (1 if enforce else 0),
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
            "severity": severity,
            "enforced": enforce,
        },
    }
    with open(log_path, 'a') as f:
        f.write(json.dumps(record, separators=(',', ':')) + '\n')
except Exception as e:
    # Non-fatal — advisory should never break a workflow because of log I/O.
    print(f"  (model-tier: tool-log entry skipped: {e})")

sys.exit(1 if enforce else 0)
PY
