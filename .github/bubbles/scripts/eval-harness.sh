#!/usr/bin/env bash
#
# eval-harness.sh — golden-task output-quality evaluator (review R11, v6.1).
#
# Bubbles selftests validate framework PROCESS/STRUCTURE. This harness adds the
# missing axis: scoring the QUALITY of a produced artifact (a spec folder, a
# report, a bug fix) against a fixed golden-task rubric, so framework AND model
# upgrades can be measured for quality regression — not just gate-pass.
#
# Deterministic rubric checks are first-class. An optional LLM-as-judge can be
# plugged via BUBBLES_EVAL_JUDGE (a command that reads the output path and emits
# a 0..1 score on stdout); when set, its score is blended in at the task's
# `judgeWeight`. With no judge, scoring is 100% deterministic and reproducible.
#
# Usage:
#   eval-harness.sh score --task <task.json> --output <dir>
#   eval-harness.sh run   --suite <dir-of-tasks> --output <dir>   # score all, aggregate
#
# Check types (in task.json `checks[]`):
#   file-exists   {path}                 +weight if <output>/<path> exists
#   contains      {path, pattern}        +weight if file matches (regex, ci)
#   not-contains  {path, pattern}        +weight if file does NOT match
#   gate-pass     {command}              +weight if `command` exits 0
#
# Output: JSON {taskId, score, maxScore, ratio, passed, checks:[...]} to stdout.
#
# Exit codes:
#   0 — scored and PASSED (ratio >= passThreshold)
#   1 — scored and FAILED (ratio <  passThreshold)
#   2 — usage / input error

set -euo pipefail

if ! command -v python3 >/dev/null 2>&1; then
  echo "eval-harness: SKIP (python3 not installed)" >&2
  exit 0
fi

usage() {
  cat >&2 <<'USAGE'
Usage:
  eval-harness.sh score --task <task.json> --output <dir>
  eval-harness.sh run   --suite <dir-of-tasks> --output <dir>
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
OP="$1"; shift
TASK="" ; SUITE="" ; OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --task) TASK="$2"; shift 2;;
    --suite) SUITE="$2"; shift 2;;
    --output) OUTPUT="$2"; shift 2;;
    -h|--help) usage; exit 0;;
    *) usage; exit 2;;
  esac
done

OP="$OP" TASK="$TASK" SUITE="$SUITE" OUTPUT="$OUTPUT" \
JUDGE="${BUBBLES_EVAL_JUDGE:-}" python3 - <<'PY'
import json, os, re, subprocess, sys, glob

op = os.environ["OP"]
output = os.environ.get("OUTPUT", "")
judge = os.environ.get("JUDGE", "").strip()

def read(path):
    try:
        with open(path, errors="replace") as f:
            return f.read()
    except Exception:
        return None

def run_check(chk, out_dir):
    ctype = chk.get("type")
    weight = float(chk.get("weight", 1))
    cid = chk.get("id", ctype)
    ok = False
    if ctype == "file-exists":
        ok = os.path.exists(os.path.join(out_dir, chk["path"]))
    elif ctype in ("contains", "not-contains"):
        body = read(os.path.join(out_dir, chk["path"]))
        present = bool(body is not None and re.search(chk["pattern"], body, re.I | re.M))
        ok = present if ctype == "contains" else (body is not None and not present)
    elif ctype == "gate-pass":
        try:
            rc = subprocess.run(chk["command"], shell=True, cwd=out_dir,
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode
            ok = (rc == 0)
        except Exception:
            ok = False
    else:
        return {"id": cid, "type": ctype, "ok": False, "weight": weight, "error": "unknown-check-type"}
    return {"id": cid, "type": ctype, "ok": ok, "weight": weight}

def score_task(task_path, out_dir):
    task = json.loads(read(task_path) or "{}")
    checks = task.get("checks", [])
    results = [run_check(c, out_dir) for c in checks]
    max_score = sum(float(c.get("weight", 1)) for c in checks) or 1.0
    got = sum(r["weight"] for r in results if r["ok"])
    ratio = got / max_score

    judge_weight = float(task.get("judgeWeight", 0))
    if judge and judge_weight > 0:
        try:
            proc = subprocess.run([judge, out_dir], capture_output=True, text=True)
            jscore = float(proc.stdout.strip() or "0")
            jscore = max(0.0, min(1.0, jscore))
            ratio = (1 - judge_weight) * ratio + judge_weight * jscore
        except Exception:
            pass

    threshold = float(task.get("passThreshold", 0.8))
    passed = ratio >= threshold
    return {
        "taskId": task.get("taskId", os.path.basename(task_path)),
        "score": round(got, 4),
        "maxScore": round(max_score, 4),
        "ratio": round(ratio, 4),
        "passThreshold": threshold,
        "passed": passed,
        "checks": results,
    }

if op == "score":
    task_path = os.environ["TASK"]
    if not task_path or not os.path.exists(task_path):
        print("eval-harness: --task not found", file=sys.stderr); sys.exit(2)
    if not output or not os.path.isdir(output):
        print("eval-harness: --output dir not found", file=sys.stderr); sys.exit(2)
    res = score_task(task_path, output)
    print(json.dumps(res, sort_keys=True, indent=2))
    sys.exit(0 if res["passed"] else 1)

elif op == "run":
    suite = os.environ["SUITE"]
    if not suite or not os.path.isdir(suite):
        print("eval-harness: --suite dir not found", file=sys.stderr); sys.exit(2)
    if not output or not os.path.isdir(output):
        print("eval-harness: --output dir not found", file=sys.stderr); sys.exit(2)
    tasks = sorted(glob.glob(os.path.join(suite, "*.json")))
    results = [score_task(t, output) for t in tasks]
    passed_n = sum(1 for r in results if r["passed"])
    agg = {
        "suite": suite,
        "taskCount": len(results),
        "passed": passed_n,
        "failed": len(results) - passed_n,
        "meanRatio": round(sum(r["ratio"] for r in results) / len(results), 4) if results else 0,
        "tasks": results,
    }
    print(json.dumps(agg, sort_keys=True, indent=2))
    sys.exit(0 if passed_n == len(results) and results else 1)

else:
    print(f"eval-harness: unknown op: {op}", file=sys.stderr); sys.exit(2)
PY
