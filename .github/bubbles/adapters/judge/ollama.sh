#!/usr/bin/env bash
# bubbles/adapters/judge/ollama.sh — model judge adapter backed by an Ollama daemon.
#
# Invoked by eval-harness.sh as:  ollama.sh <out_dir> <task_path>
# Emits ONE evaluator-result JSON object on stdout. Contract enforced by
# validate_evaluator_result() in bubbles/scripts/eval-harness.sh:
#   status  passed|failed|error|unavailable   score  [0,1], MUST be null unless passed/failed
#   verdict non-empty string                  rubricFindings  array of non-empty strings
#   provenance  object, adapter+version REQUIRED
#   error   REQUIRED object when status is error/unavailable
#
# FAIL-CLOSED: every failure path emits error/unavailable with score null. This
# adapter never emits a passing score it did not obtain from the model.
#
# There is deliberately NO none.sh sibling here. For optional-enrichment adapters
# (observability, codeindex) a `none` default returns a neutral empty value so
# consumers skip gracefully; a judge is REQUIRED SCORING whenever judgeWeight > 0,
# so a neutral default would silently downgrade required scoring to a skip.
# Absence must stay judge-adapter-missing.
#
# Config (env; no hardcoded endpoint — this surface is project-agnostic):
#   BUBBLES_EVAL_JUDGE_URL     REQUIRED, base URL of the Ollama daemon
#   BUBBLES_EVAL_JUDGE_MODEL   default qwen3:30b-a3b
#   BUBBLES_EVAL_JUDGE_THINK   "true"/"false", default false

set -euo pipefail

ADAPTER_VERSION="1.0.0"

emit_failure() {
  BUBBLES_JUDGE_STATUS="$1" BUBBLES_JUDGE_CODE="$2" BUBBLES_JUDGE_MESSAGE="$3" \
  BUBBLES_JUDGE_VERSION="$ADAPTER_VERSION" python3 - <<'PY'
import json, os
message = os.environ["BUBBLES_JUDGE_MESSAGE"]
print(json.dumps({
    "status": os.environ["BUBBLES_JUDGE_STATUS"],
    "score": None,
    "verdict": message,
    "rubricFindings": [message],
    "provenance": {"adapter": "ollama", "version": os.environ["BUBBLES_JUDGE_VERSION"]},
    "error": {"code": os.environ["BUBBLES_JUDGE_CODE"], "message": message},
}, sort_keys=True))
PY
  exit 0
}

case "${1:-}" in
  -h | --help)
    cat >&2 <<'EOF'
ollama.sh — model judge adapter (Ollama backend)
Usage: ollama.sh <out_dir> <task_path>
Env:   BUBBLES_EVAL_JUDGE_URL (required), BUBBLES_EVAL_JUDGE_MODEL, BUBBLES_EVAL_JUDGE_THINK
EOF
    exit 0
    ;;
esac

OUT_DIR="${1:-}"
TASK_PATH="${2:-}"

[ -n "$OUT_DIR" ] || emit_failure "error" "judge-usage" "out_dir argument is required"
[ -n "$TASK_PATH" ] || emit_failure "error" "judge-usage" "task_path argument is required"
[ -d "$OUT_DIR" ] || emit_failure "error" "judge-out-dir-missing" "out_dir does not exist or is not a directory"
[ -r "$TASK_PATH" ] || emit_failure "error" "judge-task-unreadable" "task_path does not exist or is not readable"
[ -n "${BUBBLES_EVAL_JUDGE_URL:-}" ] || emit_failure "unavailable" "judge-url-unset" "BUBBLES_EVAL_JUDGE_URL is not set; no judge endpoint configured"
command -v python3 >/dev/null 2>&1 || emit_failure "error" "judge-python-missing" "python3 is required by this adapter"
command -v curl >/dev/null 2>&1 || emit_failure "error" "judge-curl-missing" "curl is required by this adapter"

BUBBLES_JUDGE_OUT_DIR="$OUT_DIR" \
BUBBLES_JUDGE_TASK_PATH="$TASK_PATH" \
BUBBLES_JUDGE_URL="$BUBBLES_EVAL_JUDGE_URL" \
BUBBLES_JUDGE_MODEL="${BUBBLES_EVAL_JUDGE_MODEL:-qwen3:30b-a3b}" \
BUBBLES_JUDGE_THINK="${BUBBLES_EVAL_JUDGE_THINK:-false}" \
BUBBLES_JUDGE_VERSION="$ADAPTER_VERSION" \
python3 - <<'PY'
import json, os, subprocess, sys, uuid

VERSION = os.environ["BUBBLES_JUDGE_VERSION"]
MODEL = os.environ["BUBBLES_JUDGE_MODEL"]
MAX_FILES = 12
MAX_BYTES_PER_FILE = 4000


def emit(status, verdict, findings, score=None, error=None, invocation_id=None):
    provenance = {"adapter": "ollama", "version": VERSION, "provider": "ollama", "model": MODEL}
    if invocation_id:
        provenance["invocationId"] = invocation_id
    result = {
        "status": status,
        "score": score,
        "verdict": verdict,
        "rubricFindings": findings,
        "provenance": provenance,
    }
    if error is not None:
        result["error"] = error
    print(json.dumps(result, sort_keys=True))
    sys.exit(0)


def fail(status, code, message, invocation_id=None):
    emit(status, message, [message], None, {"code": code, "message": message}, invocation_id)


out_dir = os.environ["BUBBLES_JUDGE_OUT_DIR"]
task_path = os.environ["BUBBLES_JUDGE_TASK_PATH"]
base_url = os.environ["BUBBLES_JUDGE_URL"].rstrip("/")
think = os.environ["BUBBLES_JUDGE_THINK"].strip().lower() == "true"
invocation_id = str(uuid.uuid4())

try:
    with open(task_path, "r", encoding="utf-8") as handle:
        task = json.load(handle)
except (OSError, ValueError) as exc:
    fail("error", "judge-task-unparseable", f"task JSON could not be loaded: {type(exc).__name__}")

if not isinstance(task, dict):
    fail("error", "judge-task-unparseable", "task JSON is not an object")

# Bounded so a large out_dir cannot exhaust the context window or the timeout.
artifacts = []
for root, _dirs, files in os.walk(out_dir):
    for name in sorted(files):
        path = os.path.join(root, name)
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as handle:
                body = handle.read(MAX_BYTES_PER_FILE)
        except OSError:
            continue
        artifacts.append(f"--- FILE: {os.path.relpath(path, out_dir)} ---\n{body}")
        if len(artifacts) >= MAX_FILES:
            break
    if len(artifacts) >= MAX_FILES:
        break

if not artifacts:
    fail("failed", "judge-no-artifacts", "no readable artifacts were produced under out_dir")

rubric = task.get("rationale") or task.get("title") or task.get("taskId") or "unspecified task"

prompt = (
    "You are a strict evaluator grading an AI agent's output. Judge ONLY what is "
    "present in the artifacts. Absence of evidence is a failure, never a pass.\n\n"
    f"TASK UNDER REVIEW:\n{rubric}\n\n"
    "ARTIFACTS PRODUCED BY THE AGENT:\n" + "\n\n".join(artifacts) +
    "\n\nReply with ONLY a JSON object, no prose, exactly:\n"
    '{"status":"passed"|"failed","score":<number 0.0-1.0>,'
    '"verdict":"<one sentence>","rubricFindings":["<finding>", ...]}\n'
    'Use status "failed" and a low score when the artifacts claim success without '
    "supporting evidence."
)

payload = {
    "model": MODEL,
    "prompt": prompt,
    "format": "json",
    "stream": False,
    "think": think,
    "options": {"num_predict": 512, "temperature": 0},
}

try:
    # Below the harness judgeTimeoutSeconds ceiling of 300, which kills us first anyway.
    completed = subprocess.run(
        ["curl", "-s", "--fail-with-body", "--max-time", "290",
         "-H", "Content-Type: application/json", "--data-binary", "@-",
         f"{base_url}/api/generate"],
        input=json.dumps(payload), capture_output=True, text=True, check=False,
    )
except OSError as exc:
    fail("error", "judge-invoke-failed", f"curl could not be executed: {type(exc).__name__}", invocation_id)

if completed.returncode != 0:
    fail("unavailable", "judge-endpoint-unreachable",
         f"judge endpoint returned curl exit {completed.returncode}", invocation_id)

try:
    envelope = json.loads(completed.stdout)
except ValueError:
    fail("error", "judge-transport-malformed", "judge endpoint did not return JSON", invocation_id)

# Thinking models put the JSON body in .thinking and leave .response empty.
body = (envelope.get("response") or "").strip() or (envelope.get("thinking") or "").strip()
if not body:
    fail("error", "judge-empty-response", "judge returned neither response nor thinking content", invocation_id)

try:
    parsed = json.loads(body)
except ValueError:
    fail("error", "judge-malformed-json", "judge output was not parseable JSON", invocation_id)

if not isinstance(parsed, dict):
    fail("error", "judge-malformed-json", "judge output was not a JSON object", invocation_id)

status = parsed.get("status")
if status not in ("passed", "failed"):
    fail("error", "judge-bad-status", f"judge returned unsupported status {status!r}", invocation_id)

score = parsed.get("score")
if isinstance(score, bool) or not isinstance(score, (int, float)) or not (0 <= score <= 1):
    fail("error", "judge-bad-score", "judge score was not a number in [0,1]", invocation_id)

verdict = parsed.get("verdict")
if not isinstance(verdict, str) or not verdict.strip():
    fail("error", "judge-bad-verdict", "judge verdict was missing or empty", invocation_id)

findings_raw = parsed.get("rubricFindings")
if not isinstance(findings_raw, list):
    fail("error", "judge-bad-findings", "judge rubricFindings was not an array", invocation_id)

findings = [str(item).strip() for item in findings_raw if str(item).strip()] or [verdict.strip()]
emit(status, verdict.strip(), findings, float(score), None, invocation_id)
PY
