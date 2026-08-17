#!/usr/bin/env bash
#
# bubbles tool-log.sh — structured tool-call evidence recorder (v5.1 / M1).
#
# Wraps an arbitrary command. Streams stdout/stderr through to the caller
# (so existing pipes / TTY behavior are preserved) AND appends a JSONL
# entry to the tool-call log. The log is the v5.1 structured-evidence
# source-of-truth that v5.2's evidence gate will consume directly.
#
# Usage:
#   bash tool-log.sh <command> [args...]
#   bash tool-log.sh -- <command> [args...]    # explicit separator
#
# Environment:
#   BUBBLES_TOOL_LOG_FILE   path to JSONL log (default: .specify/runtime/tool-calls.jsonl)
#   BUBBLES_SESSION_ID      per-session identifier (required; auto-generated if absent)
#   BUBBLES_AGENT_NAME      name of invoking agent (default: 'human')
#   BUBBLES_SPEC            spec slug, if applicable (default: '')
#   BUBBLES_SCOPE           scope identifier, if applicable (default: '')
#   BUBBLES_TOOL_LOG_TAGS   comma-separated tags (e.g. 'test,e2e' or 'build')
#   BUBBLES_TOOL_LOG_INPUTS space/comma-separated input files; each is hashed at
#                           record time into the receipt's inputClosure so a later
#                           change to it can invalidate exactly this receipt
#                           (IMP-024 SCOPE-1/2, consumed by evidence-receipt-check.sh)
#   BUBBLES_TOOL_LOG_QUIET  if '1', suppress the "recorded" footer message
#
#   IMP-047 S-C receipt binding. Supplying BUBBLES_SCENARIO_ID promotes the
#   entry from "a command ran" to "a claim was proven", which is the only shape
#   the Check 9 admission path accepts. All five are recorded together because a
#   partial binding is not weaker evidence — it is evidence for nothing:
#   BUBBLES_SCENARIO_ID        stable SCN-* id this receipt is evidence for
#   BUBBLES_SCENARIO_PHASE     red|implement|green|live|regression|observed
#   BUBBLES_SCENARIO_TEST      test identity (RED and GREEN must cite the same)
#   BUBBLES_SCENARIO_CONTROL   negative control (RED and GREEN must cite the same)
#   BUBBLES_SCENARIO_CLAIM     the claim, in the words of the DoD item it is for
#   BUBBLES_SCENARIO_REVISION  source revision (default: git HEAD)
#   BUBBLES_SCENARIO_IMPL_REFS comma/space separated production paths touched
#
# Exit code: the wrapped command's exit code is preserved.
#
# Design notes:
# - No --no-log bypass flag. Anti-fabrication invariant: every wrap MUST
#   record. Run the command without this wrapper if you don't want logging.
# - We use `tee` + named pipes for hashing without losing live output.
#   Falls back to capture-then-replay if mkfifo is unavailable.

set -uo pipefail

if [[ "${1:-}" == "--" ]]; then
  shift
fi
if [[ $# -lt 1 ]]; then
  echo "tool-log: usage: tool-log.sh <command> [args...]" >&2
  exit 2
fi

# Resolve repo root (best effort).
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

LOG_FILE="${BUBBLES_TOOL_LOG_FILE:-$REPO_ROOT/.specify/runtime/tool-calls.jsonl}"
SESSION_ID="${BUBBLES_SESSION_ID:-}"
if [[ -z "$SESSION_ID" ]]; then
  # Auto-generate a session id keyed by pid + timestamp.
  SESSION_ID="auto-$(date -u +%Y%m%dT%H%M%S)-$$"
fi
AGENT_NAME="${BUBBLES_AGENT_NAME:-human}"
SPEC="${BUBBLES_SPEC:-}"
SCOPE="${BUBBLES_SCOPE:-}"
TAGS_RAW="${BUBBLES_TOOL_LOG_TAGS:-}"

mkdir -p "$(dirname "$LOG_FILE")"

# Capture cwd before the wrapped command can change it.
CWD="$(pwd)"

# Capture command line as a single string (best-effort; exact argv shape
# may differ from what a shell parser would produce, but matches what an
# operator typically pastes into evidence).
CMD_STR=""
for arg in "$@"; do
  if [[ -z "$CMD_STR" ]]; then
    CMD_STR="$arg"
  else
    CMD_STR="$CMD_STR $arg"
  fi
done

# Streaming hash via temp files (portable; mkfifo would be faster but is
# OS-dependent — temp file is universal).
STDOUT_CAP="$(mktemp -t bubbles-tool-log-stdout.XXXXXX)"
STDERR_CAP="$(mktemp -t bubbles-tool-log-stderr.XXXXXX)"
cleanup_caps() {
  rm -f "$STDOUT_CAP" "$STDERR_CAP"
}
trap cleanup_caps EXIT INT TERM

# portable-ok: the next line numeric-guards this read and falls back to second-resolution nanoseconds, which is the contract bubbles_now_ms provides.
START_NS="$(date +%s%N 2>/dev/null || echo 0)"
# BSD/macOS date may lack %N (echoes a literal 'N'); fall back to second-res ns.
[[ "$START_NS" =~ ^[0-9]+$ ]] || START_NS="$(( $(date +%s) * 1000000000 ))"

# Run the command, teeing stdout to caller stdout + STDOUT_CAP, stderr to
# caller stderr + STDERR_CAP, preserving exit code.
set +e
"$@" > >(tee "$STDOUT_CAP") 2> >(tee "$STDERR_CAP" >&2)
EXIT_CODE=$?
set -e

# Allow tee processes to flush.
wait 2>/dev/null || true

# portable-ok: numeric-guarded on the next line, as for START_NS above.
END_NS="$(date +%s%N 2>/dev/null || echo 0)"
# BSD/macOS date may lack %N (echoes a literal 'N'); fall back to second-res ns.
[[ "$END_NS" =~ ^[0-9]+$ ]] || END_NS="$(( $(date +%s) * 1000000000 ))"
DURATION_MS=0
if [[ "$START_NS" != "0" && "$END_NS" != "0" ]]; then
  DURATION_MS=$(( (END_NS - START_NS) / 1000000 ))
fi

# Hashes + sizes.
hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    # Fallback: empty hash if no hasher available (still records bytes).
    printf '%064s' '' | tr ' ' '0'
  fi
}

STDOUT_HASH="$(hash_file "$STDOUT_CAP")"
STDERR_HASH="$(hash_file "$STDERR_CAP")"
STDOUT_BYTES="$(wc -c < "$STDOUT_CAP" | tr -d ' ')"
STDERR_BYTES="$(wc -c < "$STDERR_CAP" | tr -d ' ')"

TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# v5.2 schema v2 — framework provenance fields. Resolved best-effort:
# - VERSION file in bubbles repo (when installed in a downstream repo, this
#   resolves to .github/bubbles/.version via the resolver below).
# - Git SHA of the bubbles framework checkout.
FRAMEWORK_VERSION=""
FRAMEWORK_SHA=""
if [[ -f "$REPO_ROOT/.github/bubbles/.version" ]]; then
  FRAMEWORK_VERSION="$(tr -d '[:space:]' < "$REPO_ROOT/.github/bubbles/.version" 2>/dev/null || true)"
elif [[ -f "$REPO_ROOT/VERSION" ]]; then
  FRAMEWORK_VERSION="$(tr -d '[:space:]' < "$REPO_ROOT/VERSION" 2>/dev/null || true)"
fi
if [[ -f "$REPO_ROOT/.github/bubbles/.install-source.json" ]] && command -v python3 >/dev/null 2>&1; then
  FRAMEWORK_SHA="$(python3 -c '
import json, sys
try:
    print(json.load(open(sys.argv[1])).get("sourceGitSha", "") or "")
except Exception:
    pass
' "$REPO_ROOT/.github/bubbles/.install-source.json" 2>/dev/null || true)"
fi
if [[ -z "$FRAMEWORK_SHA" ]] && [[ -d "$REPO_ROOT/.git" ]]; then
  FRAMEWORK_SHA="$(git -C "$REPO_ROOT" rev-parse --verify HEAD 2>/dev/null || true)"
fi

# The revision a scenario receipt is recorded against. A receipt proves what the
# tree WAS, so the resolver refuses it once the tree has moved.
SOURCE_REVISION="$(git -C "$REPO_ROOT" rev-parse --verify HEAD 2>/dev/null || true)"

# JSON encoder — use python3 for correctness (handles special chars in cmd / cwd).
TAGS_RAW="$TAGS_RAW" \
TS="$TS" \
SESSION_ID="$SESSION_ID" \
AGENT_NAME="$AGENT_NAME" \
SPEC="$SPEC" \
SCOPE="$SCOPE" \
CMD_STR="$CMD_STR" \
CWD="$CWD" \
EXIT_CODE="$EXIT_CODE" \
DURATION_MS="$DURATION_MS" \
STDOUT_HASH="$STDOUT_HASH" \
STDERR_HASH="$STDERR_HASH" \
STDOUT_BYTES="$STDOUT_BYTES" \
STDERR_BYTES="$STDERR_BYTES" \
FRAMEWORK_VERSION="$FRAMEWORK_VERSION" \
FRAMEWORK_SHA="$FRAMEWORK_SHA" \
INPUTS_RAW="${BUBBLES_TOOL_LOG_INPUTS:-}" \
SCENARIO_ID="${BUBBLES_SCENARIO_ID:-}" \
SCENARIO_PHASE="${BUBBLES_SCENARIO_PHASE:-}" \
SCENARIO_TEST="${BUBBLES_SCENARIO_TEST:-}" \
SCENARIO_CONTROL="${BUBBLES_SCENARIO_CONTROL:-}" \
SCENARIO_CLAIM="${BUBBLES_SCENARIO_CLAIM:-}" \
SCENARIO_REVISION="${BUBBLES_SCENARIO_REVISION:-$SOURCE_REVISION}" \
SCENARIO_IMPL_REFS="${BUBBLES_SCENARIO_IMPL_REFS:-}" \
python3 - >> "$LOG_FILE" <<'PY'
import json, os, hashlib, re
tags_raw = os.environ.get('TAGS_RAW', '').strip()
tags = [t.strip() for t in tags_raw.split(',') if t.strip()] if tags_raw else []
framework = {"name": "bubbles"}
fv = os.environ.get('FRAMEWORK_VERSION', '').strip()
if fv:
    framework["version"] = fv
fs = os.environ.get('FRAMEWORK_SHA', '').strip()
if fs:
    framework["sourceGitSha"] = fs
# IMP-024 SCOPE-1: input-closure fingerprint. Each declared input file is hashed
# at record time so a later change to it can invalidate exactly this receipt.
inputs_raw = os.environ.get('INPUTS_RAW', '').strip()
input_closure = []
if inputs_raw:
    seen = set()
    for path in re.split(r'[,\s]+', inputs_raw):
        path = path.strip()
        if not path or path in seen:
            continue
        seen.add(path)
        entry = {"path": path}
        try:
            with open(path, 'rb') as fh:
                entry["sha256"] = hashlib.sha256(fh.read()).hexdigest()
        except Exception:
            entry["sha256"] = None  # declared but unreadable at record time
        input_closure.append(entry)
record = {
    "schemaVersion": 2,
    "ts": os.environ['TS'],
    "sessionId": os.environ['SESSION_ID'],
    "agent": os.environ['AGENT_NAME'],
    "spec": os.environ['SPEC'],
    "scope": os.environ['SCOPE'],
    "cmd": os.environ['CMD_STR'],
    "cwd": os.environ['CWD'],
    "exitCode": int(os.environ['EXIT_CODE']),
    "durationMs": int(os.environ['DURATION_MS']),
    "stdoutHash": os.environ['STDOUT_HASH'],
    "stderrHash": os.environ['STDERR_HASH'],
    "stdoutBytes": int(os.environ['STDOUT_BYTES']),
    "stderrBytes": int(os.environ['STDERR_BYTES']),
    "tags": tags,
    "framework": framework,
}
if input_closure:
    record["inputClosure"] = input_closure
# IMP-047 S-C. The binding is recorded ONLY when a scenario is named. It is
# written whole: a caller that names a scenario but omits the test identity or
# the control gets a binding that the resolver REFUSES by name, which is the
# point — a half-binding must be visibly incomplete, never quietly admitted.
scenario_id = os.environ.get('SCENARIO_ID', '').strip()
if scenario_id:
    binding = {
        "scenarioId": scenario_id,
        "phase": os.environ.get('SCENARIO_PHASE', '').strip(),
        "testIdentity": os.environ.get('SCENARIO_TEST', '').strip(),
        "sourceRevision": os.environ.get('SCENARIO_REVISION', '').strip(),
    }
    for key, env in (("negativeControl", 'SCENARIO_CONTROL'), ("claim", 'SCENARIO_CLAIM')):
        value = os.environ.get(env, '').strip()
        if value:
            binding[key] = value
    refs = [r for r in re.split(r'[,\s]+', os.environ.get('SCENARIO_IMPL_REFS', '').strip()) if r]
    if refs:
        binding["implementationRefs"] = refs
    record["schemaVersion"] = 3
    record["scenarioBinding"] = binding
print(json.dumps(record, separators=(',', ':')))
PY

if [[ "${BUBBLES_TOOL_LOG_QUIET:-0}" != "1" ]]; then
  echo "[tool-log] recorded exit=$EXIT_CODE duration=${DURATION_MS}ms → $LOG_FILE" >&2
fi

exit "$EXIT_CODE"
