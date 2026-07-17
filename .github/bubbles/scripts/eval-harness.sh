#!/usr/bin/env bash
#
# eval-harness.sh — versioned golden-task output-quality evaluator.
#
# Bubbles selftests validate framework PROCESS/STRUCTURE. This harness adds the
# missing axis: scoring the QUALITY of a produced artifact (a spec folder, a
# report, a bug fix) against a fixed golden-task rubric, so framework AND model
# upgrades can be measured for quality regression — not just gate-pass.
#
# Version 2 tasks are fail-closed. They require a substantive executable oracle
# or semantic evaluator and use schema-validated evaluator JSON. Operator-owned
# adapters are configured through BUBBLES_EVAL_SEMANTIC and BUBBLES_EVAL_JUDGE;
# task manifests never supply adapter commands.
#
# Usage:
#   eval-harness.sh score --task <task.json> --output <dir>
#   eval-harness.sh run   --suite <dir-of-tasks> --output <dir>   # score all, aggregate
#
# Version 2 check types (in task.json `checks[]`):
#   file-exists   {path}                 +weight if <output>/<path> exists
#   contains      {path, pattern}        +weight if file matches (regex, ci)
#   not-contains  {path, pattern}        +weight if file does NOT match
#   executable-oracle {argv, allowedRoot, timeoutSeconds}
#   semantic-evaluator {rubric, timeoutSeconds}
#
# BUBBLES_EVAL_SEMANTIC is invoked as:
#   <adapter> <output-dir> <task-path> <check-id>
# BUBBLES_EVAL_JUDGE is invoked as:
#   <adapter> <output-dir> <task-path>
# Each adapter emits evaluator-result.schema.json JSON on stdout. Adapter env
# values may also be JSON argv arrays; they are never interpreted by a shell.
#
# Unversioned/version-1 tasks remain runnable during migration, but every v1
# result is explicitly legacy and non-certifying. Legacy gate-pass commands are
# never executed because their string form is an injection surface.
#
# Exit codes:
#   0 — scored and PASSED (ratio >= passThreshold)
#   1 — evaluated but FAILED / ERROR / UNAVAILABLE
#   2 — usage / input error

set -euo pipefail

if ! command -v python3 >/dev/null 2>&1; then
    printf '%s\n' '{"certification":{"eligible":false,"reason":"python3 is required for evaluation","status":"unavailable"},"certified":false,"checks":[],"compatibility":{"legacyGatePass":"disabled-unavailable","unknownOptionalCheckStatus":"unavailable-weight-retained","version1":"supported-legacy-non-certifying"},"deterministicRatio":null,"evaluationErrors":[{"code":"python3-unavailable","message":"python3 is required for evaluation"}],"evaluationStatus":"unavailable","evaluatorResultSchema":"https://bubbles.dev/eval/schemas/evaluator-result.schema.json","inputValid":false,"judge":{"error":{"code":"judge-not-run","message":"evaluation runtime unavailable"},"provenance":null,"required":false,"rubricFindings":[],"score":null,"status":"unavailable","verdict":"not run","weight":0},"legacy":false,"maxScore":null,"passThreshold":null,"passed":false,"qualityCritical":false,"ratio":null,"score":null,"taskId":null,"taskSchema":"https://bubbles.dev/eval/schemas/task-v2.schema.json","taskSchemaVersion":null}'
    exit 1
fi

usage() {
  cat >&2 <<'USAGE'
Usage:
  eval-harness.sh score --task <task.json> --output <dir>
  eval-harness.sh run   --suite <dir-of-tasks> --output <dir>
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
OP="$1"
shift
TASK=""
SUITE=""
OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --task|--suite|--output)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      case "$1" in
        --task) TASK="$2" ;;
        --suite) SUITE="$2" ;;
        --output) OUTPUT="$2" ;;
      esac
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"

OP="$OP" TASK="$TASK" SUITE="$SUITE" OUTPUT="$OUTPUT" \
TASK_SCHEMA="$SCRIPT_DIR/../eval/schemas/task-v2.schema.json" \
EVALUATOR_SCHEMA="$SCRIPT_DIR/../eval/schemas/evaluator-result.schema.json" \
SEMANTIC_ADAPTER="${BUBBLES_EVAL_SEMANTIC:-}" \
JUDGE_ADAPTER="${BUBBLES_EVAL_JUDGE:-}" python3 - <<'PY'
import glob
import json
import math
import os
import re
import subprocess
import sys

op = os.environ["OP"]
output = os.environ.get("OUTPUT", "")
task_schema_path = os.environ["TASK_SCHEMA"]
evaluator_schema_path = os.environ["EVALUATOR_SCHEMA"]
semantic_adapter = os.environ.get("SEMANTIC_ADAPTER", "").strip()
judge_adapter = os.environ.get("JUDGE_ADAPTER", "").strip()

STATUSES = ("passed", "failed", "error", "unavailable")
KNOWN_V2_CHECKS = (
    "file-exists",
    "contains",
    "not-contains",
    "executable-oracle",
    "semantic-evaluator",
)
SUBSTANTIVE_CHECKS = ("executable-oracle", "semantic-evaluator")
V2_TASK_KEYS = {
    "schemaVersion",
    "taskId",
    "title",
    "rationale",
    "description",
    "passThreshold",
    "judgeWeight",
    "judgeTimeoutSeconds",
    "checks",
}


def reject_constant(value):
    raise ValueError(f"non-finite JSON number: {value}")


def reject_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def load_json(path):
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(
            handle,
            parse_constant=reject_constant,
            object_pairs_hook=reject_duplicate_keys,
        )


def is_number(value):
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False
    try:
        return math.isfinite(value)
    except OverflowError:
        return False


def issue(path, code, message):
    return {"path": path, "code": code, "message": message}


def error_record(code, message, **details):
    record = {"code": code, "message": message}
    record.update(details)
    return record


def rounded(value):
    return round(value, 4) if is_number(value) else None


def safe_relative_path(path):
    if not isinstance(path, str) or not path or "\x00" in path:
        return False
    if os.path.isabs(path):
        return False
    return ".." not in path.replace("\\", "/").split("/")


def is_within(root, candidate):
    try:
        return os.path.commonpath((root, candidate)) == root
    except (TypeError, ValueError):
        return False


def validate_string(value, path, issues, allow_empty=False):
    if not isinstance(value, str) or (not allow_empty and not value):
        issues.append(issue(path, "type", "must be a non-empty string"))


def validate_weight(value, path, issues):
    if not is_number(value) or value < 0:
        issues.append(issue(path, "range", "must be a finite number greater than or equal to 0"))


def validate_timeout(value, path, issues):
    if not is_number(value) or value <= 0 or value > 300:
        issues.append(issue(path, "range", "must be a finite number in (0, 300]"))


def validate_total_weight(checks, issues):
    weights = [
        float(check.get("weight", 1))
        for check in checks
        if isinstance(check, dict) and is_number(check.get("weight", 1))
    ]
    try:
        total = math.fsum(weights)
    except OverflowError:
        total = math.inf
    if not math.isfinite(total):
        issues.append(issue("$.checks", "weight-overflow", "aggregate check weight must be finitely representable"))


def validate_v2_check(check, index, issues):
    path = f"$.checks[{index}]"
    if not isinstance(check, dict):
        issues.append(issue(path, "type", "must be an object"))
        return

    check_type = check.get("type")
    validate_string(check.get("id"), f"{path}.id", issues)
    validate_string(check_type, f"{path}.type", issues)
    required = check.get("required", False)
    if not isinstance(required, bool):
        issues.append(issue(f"{path}.required", "type", "must be a boolean"))
    validate_weight(check.get("weight", 1), f"{path}.weight", issues)

    common = {"id", "type", "required", "weight"}
    if check_type == "file-exists":
        allowed = common | {"path"}
        validate_string(check.get("path"), f"{path}.path", issues)
        if isinstance(check.get("path"), str) and not safe_relative_path(check["path"]):
            issues.append(issue(f"{path}.path", "path", "must stay within the output directory"))
    elif check_type in ("contains", "not-contains"):
        allowed = common | {"path", "pattern"}
        validate_string(check.get("path"), f"{path}.path", issues)
        if isinstance(check.get("path"), str) and not safe_relative_path(check["path"]):
            issues.append(issue(f"{path}.path", "path", "must stay within the output directory"))
        validate_string(check.get("pattern"), f"{path}.pattern", issues, allow_empty=True)
    elif check_type == "executable-oracle":
        allowed = common | {"argv", "allowedRoot", "timeoutSeconds"}
        argv = check.get("argv")
        if not isinstance(argv, list) or not argv:
            issues.append(issue(f"{path}.argv", "type", "must be a non-empty string array"))
        elif any(not isinstance(arg, str) or not arg or "\x00" in arg for arg in argv):
            issues.append(issue(f"{path}.argv", "type", "every argv item must be a non-empty NUL-free string"))
        allowed_root = check.get("allowedRoot")
        validate_string(allowed_root, f"{path}.allowedRoot", issues)
        if isinstance(allowed_root, str) and not safe_relative_path(allowed_root):
            issues.append(issue(f"{path}.allowedRoot", "path", "must be relative to the task directory"))
        validate_timeout(check.get("timeoutSeconds", 30), f"{path}.timeoutSeconds", issues)
    elif check_type == "semantic-evaluator":
        allowed = common | {"rubric", "timeoutSeconds"}
        if "rubric" in check:
            validate_string(check["rubric"], f"{path}.rubric", issues, allow_empty=True)
        validate_timeout(check.get("timeoutSeconds", 30), f"{path}.timeoutSeconds", issues)
    else:
        if required is True:
            issues.append(issue(f"{path}.type", "unknown-required-check", "unknown required checks are invalid"))
        return

    for key in sorted(set(check) - allowed):
        issues.append(issue(f"{path}.{key}", "additional-property", "property is not allowed"))


def validate_v1_task(task):
    issues = []
    validate_string(task.get("taskId"), "$.taskId", issues)
    threshold = task.get("passThreshold", 0.8)
    if not is_number(threshold) or threshold < 0 or threshold > 1:
        issues.append(issue("$.passThreshold", "range", "must be a finite number in [0, 1]"))
    judge_weight = task.get("judgeWeight", 0)
    if not is_number(judge_weight) or judge_weight < 0 or judge_weight > 1:
        issues.append(issue("$.judgeWeight", "range", "must be a finite number in [0, 1]"))
    if "judgeTimeoutSeconds" in task:
        validate_timeout(task["judgeTimeoutSeconds"], "$.judgeTimeoutSeconds", issues)
    checks = task.get("checks")
    if not isinstance(checks, list) or not checks:
        issues.append(issue("$.checks", "type", "must be a non-empty array"))
        return issues
    for index, check in enumerate(checks):
        path = f"$.checks[{index}]"
        if not isinstance(check, dict):
            issues.append(issue(path, "type", "must be an object"))
            continue
        validate_string(check.get("type"), f"{path}.type", issues)
        if "id" in check:
            validate_string(check["id"], f"{path}.id", issues)
        if "required" in check and not isinstance(check["required"], bool):
            issues.append(issue(f"{path}.required", "type", "must be a boolean"))
        validate_weight(check.get("weight", 1), f"{path}.weight", issues)
        check_type = check.get("type")
        if check_type in ("file-exists", "contains", "not-contains"):
            validate_string(check.get("path"), f"{path}.path", issues)
            if isinstance(check.get("path"), str) and not safe_relative_path(check["path"]):
                issues.append(issue(f"{path}.path", "path", "must stay within the output directory"))
        if check_type in ("contains", "not-contains"):
            validate_string(check.get("pattern"), f"{path}.pattern", issues, allow_empty=True)
        if check_type == "gate-pass" and "command" in check and not isinstance(check["command"], (str, list)):
            issues.append(issue(f"{path}.command", "type", "legacy command must be a string or array"))
    if all(
        not isinstance(check, dict)
        or not is_number(check.get("weight", 1))
        or check.get("weight", 1) == 0
        for check in checks
    ):
        issues.append(issue("$.checks", "positive-weight-required", "at least one check must have positive weight"))
    validate_total_weight(checks, issues)
    return issues


def validate_task(task):
    if not isinstance(task, dict):
        return None, [issue("$", "type", "task must be an object")]

    version = task.get("schemaVersion", 1)
    if isinstance(version, bool) or version not in (1, 2):
        return version, [issue("$.schemaVersion", "unsupported-version", "supported versions are 1 and 2")]
    if version == 1:
        return version, validate_v1_task(task)

    issues = []
    for key in sorted(set(task) - V2_TASK_KEYS):
        issues.append(issue(f"$.{key}", "additional-property", "property is not allowed"))
    for key in ("schemaVersion", "taskId", "checks", "passThreshold"):
        if key not in task:
            issues.append(issue(f"$.{key}", "required", "property is required"))
    validate_string(task.get("taskId"), "$.taskId", issues)
    for key in ("title", "rationale", "description"):
        if key in task and not isinstance(task[key], str):
            issues.append(issue(f"$.{key}", "type", "must be a string"))
    threshold = task.get("passThreshold")
    if not is_number(threshold) or threshold < 0 or threshold > 1:
        issues.append(issue("$.passThreshold", "range", "must be a finite number in [0, 1]"))
    judge_weight = task.get("judgeWeight", 0)
    if not is_number(judge_weight) or judge_weight < 0 or judge_weight > 1:
        issues.append(issue("$.judgeWeight", "range", "must be a finite number in [0, 1]"))
    validate_timeout(task.get("judgeTimeoutSeconds", 30), "$.judgeTimeoutSeconds", issues)

    checks = task.get("checks")
    if not isinstance(checks, list) or not checks:
        issues.append(issue("$.checks", "type", "must be a non-empty array"))
        return version, issues
    for index, check in enumerate(checks):
        validate_v2_check(check, index, issues)

    ids = [check.get("id") for check in checks if isinstance(check, dict) and isinstance(check.get("id"), str)]
    if len(ids) != len(set(ids)):
        issues.append(issue("$.checks", "duplicate-id", "check ids must be unique"))
    if not any(
        isinstance(check, dict)
        and check.get("type") in SUBSTANTIVE_CHECKS
        and check.get("required") is True
        for check in checks
    ):
        issues.append(
            issue(
                "$.checks",
                "substantive-check-required",
                "v2 requires a required executable-oracle or semantic-evaluator check",
            )
        )
    if all(
        not isinstance(check, dict)
        or not is_number(check.get("weight", 1))
        or check.get("weight", 1) == 0
        for check in checks
    ):
        issues.append(issue("$.checks", "positive-weight-required", "at least one check must have positive weight"))
    validate_total_weight(checks, issues)
    return version, issues


def validate_evaluator_result(result):
    issues = []
    if not isinstance(result, dict):
        return [issue("$", "type", "evaluator result must be an object")]
    allowed = {"status", "score", "verdict", "rubricFindings", "provenance", "error"}
    for key in sorted(set(result) - allowed):
        issues.append(issue(f"$.{key}", "additional-property", "property is not allowed"))
    for key in ("status", "score", "verdict", "rubricFindings", "provenance"):
        if key not in result:
            issues.append(issue(f"$.{key}", "required", "property is required"))

    status = result.get("status")
    if status not in STATUSES:
        issues.append(issue("$.status", "enum", "must be passed, failed, error, or unavailable"))
    score = result.get("score")
    if status in ("passed", "failed"):
        if not is_number(score) or score < 0 or score > 1:
            issues.append(issue("$.score", "range", "passed/failed evaluator score must be finite and in [0, 1]"))
    elif score is not None:
        issues.append(issue("$.score", "type", "error/unavailable evaluator score must be null"))
    validate_string(result.get("verdict"), "$.verdict", issues)
    findings = result.get("rubricFindings")
    if not isinstance(findings, list) or any(not isinstance(item, str) or not item for item in findings):
        issues.append(issue("$.rubricFindings", "type", "must be an array of non-empty strings"))
    provenance = result.get("provenance")
    if not isinstance(provenance, dict):
        issues.append(issue("$.provenance", "type", "must be an object"))
    else:
        validate_string(provenance.get("adapter"), "$.provenance.adapter", issues)
        validate_string(provenance.get("version"), "$.provenance.version", issues)
        for key in ("provider", "model", "invocationId"):
            if key in provenance:
                validate_string(provenance[key], f"$.provenance.{key}", issues)
    adapter_error = result.get("error")
    if status in ("error", "unavailable"):
        if "error" not in result:
            issues.append(issue("$.error", "required", "error/unavailable evaluator result requires an error object"))
    if "error" in result:
        if not isinstance(adapter_error, dict):
            issues.append(issue("$.error", "type", "must be an object when present"))
        else:
            validate_string(adapter_error.get("code"), "$.error.code", issues)
            validate_string(adapter_error.get("message"), "$.error.message", issues)
    return issues


def base_judge(weight=0, required=False, code="judge-not-requested", message="judgeWeight is zero"):
    return {
        "required": required,
        "weight": weight,
        "status": "unavailable",
        "score": None,
        "verdict": "not run",
        "rubricFindings": [],
        "provenance": None,
        "error": error_record(code, message),
    }


def contract_fields(version):
    return {
        "taskSchemaVersion": version,
        "taskSchema": task_schema.get("$id"),
        "evaluatorResultSchema": evaluator_schema.get("$id"),
    }


def invalid_task_result(task_path, task_id, version, issues):
    legacy = version == 1
    return {
        "taskId": task_id or os.path.basename(task_path),
        **contract_fields(version),
        "inputValid": False,
        "legacy": legacy,
        "qualityCritical": version == 2,
        "evaluationStatus": "error",
        "evaluationErrors": [
            error_record(
                "task-schema-invalid",
                "task definition failed validation before scoring",
                issues=issues,
            )
        ],
        "score": None,
        "maxScore": None,
        "deterministicRatio": None,
        "ratio": None,
        "passThreshold": None,
        "passed": False,
        "certified": False,
        "certification": {
            "eligible": False,
            "status": "legacy-non-certifying" if legacy else "invalid-task",
            "reason": "version 1 tasks cannot certify substantive quality" if legacy else "task schema validation failed",
        },
        "compatibility": compatibility_fields(),
        "checks": [],
        "judge": base_judge(),
    }


def compatibility_fields():
    return {
        "version1": "supported-legacy-non-certifying",
        "legacyGatePass": "disabled-unavailable",
        "unknownOptionalCheckStatus": "unavailable-weight-retained",
    }


def check_result(check, status, score, error=None, **details):
    weight = float(check.get("weight", 1))
    earned = weight * score if is_number(score) else None
    result = {
        "id": check.get("id", check.get("type", "unknown")),
        "type": check.get("type"),
        "required": check.get("required", False) is True,
        "weight": weight,
        "status": status,
        "ok": status == "passed",
        "score": score,
        "earnedScore": rounded(earned),
        "error": error,
    }
    result.update(details)
    return result


def output_path(out_dir, relative_path):
    root = os.path.realpath(out_dir)
    candidate = os.path.realpath(os.path.join(root, relative_path))
    if not is_within(root, candidate):
        raise ValueError("path escapes output directory")
    return candidate


def parse_adapter_argv(value, role):
    if not value:
        return None, error_record(f"{role}-adapter-missing", f"{role} adapter is not configured")
    if value.lstrip().startswith("["):
        try:
            argv = json.loads(
                value,
                parse_constant=reject_constant,
                object_pairs_hook=reject_duplicate_keys,
            )
        except (TypeError, ValueError, json.JSONDecodeError):
            return None, error_record(f"{role}-adapter-config-invalid", f"{role} adapter argv JSON is invalid")
        if not isinstance(argv, list) or not argv or any(
            not isinstance(arg, str) or not arg or "\x00" in arg for arg in argv
        ):
            return None, error_record(f"{role}-adapter-config-invalid", f"{role} adapter must be a non-empty string argv array")
        return argv, None
    return [value], None


def invoke_adapter(config, arguments, timeout, role):
    argv, config_error = parse_adapter_argv(config, role)
    if config_error:
        status = "unavailable" if config_error["code"] == f"{role}-adapter-missing" else "error"
        return {"status": status, "error": config_error}
    try:
        process = subprocess.run(
            argv + arguments,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired:
        return {
            "status": "error",
            "error": error_record(f"{role}-timeout", f"{role} adapter exceeded its timeout", timeoutSeconds=timeout),
        }
    except FileNotFoundError:
        return {
            "status": "unavailable",
            "error": error_record(f"{role}-adapter-unavailable", f"{role} adapter executable was not found"),
        }
    except (OSError, ValueError) as exc:
        return {
            "status": "unavailable" if getattr(exc, "errno", None) in (2, 13) else "error",
            "error": error_record(f"{role}-adapter-exec-error", f"{role} adapter could not be executed", errno=getattr(exc, "errno", None)),
        }

    if process.returncode != 0:
        return {
            "status": "error",
            "error": error_record(f"{role}-nonzero", f"{role} adapter exited nonzero", exitCode=process.returncode),
        }
    payload = process.stdout.strip()
    if not payload:
        return {
            "status": "error",
            "error": error_record(f"{role}-empty-output", f"{role} adapter emitted no JSON"),
        }
    try:
        result = json.loads(
            payload,
            parse_constant=reject_constant,
            object_pairs_hook=reject_duplicate_keys,
        )
    except (TypeError, ValueError, json.JSONDecodeError):
        return {
            "status": "error",
            "error": error_record(f"{role}-malformed-json", f"{role} adapter output is not valid finite JSON"),
        }
    result_issues = validate_evaluator_result(result)
    if result_issues:
        return {
            "status": "error",
            "error": error_record(f"{role}-schema-invalid", f"{role} adapter output failed evaluator-result validation", issues=result_issues),
        }
    return {"status": "valid", "result": result}


def run_oracle(check, task_path, out_dir):
    task_root = os.path.realpath(os.path.dirname(task_path))
    allowed_root = os.path.realpath(os.path.join(task_root, check["allowedRoot"]))
    details = {
        "execution": {
            "allowedRoot": check["allowedRoot"],
            "argvCount": len(check["argv"]),
            "timeoutSeconds": check.get("timeoutSeconds", 30),
            "shell": False,
        }
    }
    if not is_within(task_root, allowed_root):
        return check_result(
            check,
            "error",
            None,
            error_record("oracle-root-outside-task", "allowedRoot escapes the task directory"),
            **details,
        )
    if not os.path.isdir(allowed_root):
        return check_result(
            check,
            "unavailable",
            None,
            error_record("oracle-root-unavailable", "declared allowedRoot does not exist or is not a directory"),
            **details,
        )

    executable_arg = check["argv"][0]
    executable = os.path.realpath(
        executable_arg if os.path.isabs(executable_arg) else os.path.join(allowed_root, executable_arg)
    )
    if not is_within(allowed_root, executable):
        return check_result(
            check,
            "error",
            None,
            error_record("oracle-executable-outside-root", "oracle executable resolves outside allowedRoot"),
            **details,
        )
    if not os.path.isfile(executable) or not os.access(executable, os.X_OK):
        return check_result(
            check,
            "unavailable",
            None,
            error_record("oracle-executable-unavailable", "oracle executable is missing or not executable"),
            **details,
        )

    argv = [executable] + check["argv"][1:]
    adapter_env = os.environ.copy()
    adapter_env["BUBBLES_EVAL_OUTPUT"] = os.path.realpath(out_dir)
    adapter_env["BUBBLES_EVAL_TASK"] = os.path.realpath(task_path)
    try:
        process = subprocess.run(
            argv,
            cwd=os.path.realpath(out_dir),
            env=adapter_env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=check.get("timeoutSeconds", 30),
            check=False,
        )
    except subprocess.TimeoutExpired:
        return check_result(
            check,
            "error",
            None,
            error_record("oracle-timeout", "oracle exceeded its timeout"),
            **details,
        )
    except (OSError, ValueError) as exc:
        return check_result(
            check,
            "error",
            None,
            error_record("oracle-exec-error", "oracle could not be executed", errno=getattr(exc, "errno", None)),
            **details,
        )

    details["execution"]["exitCode"] = process.returncode
    if process.returncode == 0:
        return check_result(check, "passed", 1.0, None, **details)
    return check_result(
        check,
        "failed",
        0.0,
        error_record("oracle-nonzero", "oracle exited nonzero", exitCode=process.returncode),
        **details,
    )


def run_semantic(check, task_path, out_dir):
    invocation = invoke_adapter(
        semantic_adapter,
        [os.path.realpath(out_dir), os.path.realpath(task_path), check["id"]],
        check.get("timeoutSeconds", 30),
        "semantic",
    )
    if invocation["status"] != "valid":
        return check_result(check, invocation["status"], None, invocation["error"])
    evaluator_result = invocation["result"]
    return check_result(
        check,
        evaluator_result["status"],
        evaluator_result["score"],
        evaluator_result.get("error"),
        verdict=evaluator_result["verdict"],
        rubricFindings=evaluator_result["rubricFindings"],
        provenance=evaluator_result["provenance"],
    )


def run_check(check, task_path, out_dir, version):
    check_type = check.get("type")
    if check_type == "gate-pass":
        return check_result(
            check,
            "unavailable",
            None,
            error_record("legacy-gate-pass-disabled", "legacy gate-pass command strings are not executed"),
        )
    if check_type not in KNOWN_V2_CHECKS:
        status = "error" if check.get("required") is True else "unavailable"
        code = "unknown-required-check" if status == "error" else "unknown-optional-check"
        return check_result(check, status, None, error_record(code, f"unsupported check type: {check_type}"))
    if check_type == "executable-oracle":
        if version != 2:
            return check_result(
                check,
                "unavailable",
                None,
                error_record("v2-check-in-legacy-task", "executable-oracle requires schemaVersion 2"),
            )
        return run_oracle(check, task_path, out_dir)
    if check_type == "semantic-evaluator":
        if version != 2:
            return check_result(
                check,
                "unavailable",
                None,
                error_record("v2-check-in-legacy-task", "semantic-evaluator requires schemaVersion 2"),
            )
        return run_semantic(check, task_path, out_dir)

    try:
        path = output_path(out_dir, check["path"])
    except (OSError, ValueError):
        return check_result(
            check,
            "error",
            None,
            error_record("check-path-invalid", "check path resolves outside the output directory"),
        )
    if check_type == "file-exists":
        return check_result(check, "passed", 1.0) if os.path.exists(path) else check_result(
            check,
            "failed",
            0.0,
            error_record("file-missing", "required artifact path does not exist"),
        )
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as handle:
            body = handle.read()
    except FileNotFoundError:
        return check_result(check, "failed", 0.0, error_record("file-missing", "artifact path does not exist"))
    except OSError as exc:
        return check_result(
            check,
            "error",
            None,
            error_record("file-read-error", "artifact path could not be read", errno=getattr(exc, "errno", None)),
        )
    try:
        present = bool(re.search(check["pattern"], body, re.IGNORECASE | re.MULTILINE))
    except re.error as exc:
        return check_result(
            check,
            "error",
            None,
            error_record("pattern-invalid", "regular expression is invalid", regexError=str(exc)),
        )
    matched = present if check_type == "contains" else not present
    if matched:
        return check_result(check, "passed", 1.0)
    return check_result(check, "failed", 0.0, error_record("pattern-mismatch", "artifact content did not satisfy the check"))


def run_judge(task, task_path, out_dir):
    weight = float(task.get("judgeWeight", 0))
    if weight == 0:
        return base_judge()
    invocation = invoke_adapter(
        judge_adapter,
        [os.path.realpath(out_dir), os.path.realpath(task_path)],
        task.get("judgeTimeoutSeconds", 30),
        "judge",
    )
    if invocation["status"] != "valid":
        result = base_judge(
            weight=weight,
            required=True,
            code=invocation["error"]["code"],
            message=invocation["error"]["message"],
        )
        result["status"] = invocation["status"]
        result["error"] = invocation["error"]
        return result
    evaluator_result = invocation["result"]
    return {
        "required": True,
        "weight": weight,
        "status": evaluator_result["status"],
        "score": evaluator_result["score"],
        "verdict": evaluator_result["verdict"],
        "rubricFindings": evaluator_result["rubricFindings"],
        "provenance": evaluator_result["provenance"],
        "error": evaluator_result.get("error"),
    }


def aggregate_status(reasons):
    statuses = {reason["status"] for reason in reasons}
    for status in ("error", "unavailable", "failed"):
        if status in statuses:
            return status
    return "passed"


def score_task(task_path, out_dir):
    try:
        task = load_json(task_path)
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as exc:
        return invalid_task_result(
            task_path,
            None,
            None,
            [issue("$", "json", f"task JSON could not be loaded: {type(exc).__name__}")],
        )

    version, validation_issues = validate_task(task)
    task_id = task.get("taskId") if isinstance(task, dict) else None
    if validation_issues:
        return invalid_task_result(task_path, task_id, version, validation_issues)

    checks = task["checks"]
    results = [run_check(check, task_path, out_dir, version) for check in checks]
    max_score = sum(float(check.get("weight", 1)) for check in checks)
    scoring_unavailable = any(
        result["weight"] > 0 and result["status"] in ("error", "unavailable")
        for result in results
    )
    got = None if scoring_unavailable else sum(
        result["earnedScore"] for result in results if is_number(result["earnedScore"])
    )
    deterministic_ratio = None if got is None else got / max_score
    judge = run_judge(task, task_path, out_dir)
    judge_weight = float(task.get("judgeWeight", 0))
    if judge_weight > 0 and judge["status"] in ("passed", "failed") and deterministic_ratio is not None:
        ratio = (1 - judge_weight) * deterministic_ratio + judge_weight * judge["score"]
    elif judge_weight > 0:
        ratio = None
    else:
        ratio = deterministic_ratio

    reasons = []
    for result in results:
        if result["required"] and result["status"] != "passed":
            reasons.append(
                {
                    "status": result["status"],
                    "code": f"required-check-{result['status']}",
                    "message": "required check did not pass",
                    "checkId": result["id"],
                }
            )
        elif result["weight"] > 0 and result["status"] in ("error", "unavailable"):
            reasons.append(
                {
                    "status": result["status"],
                    "code": f"weighted-check-{result['status']}",
                    "message": "weighted check could not produce a valid score",
                    "checkId": result["id"],
                }
            )
    if judge_weight > 0 and judge["status"] != "passed":
        reasons.append(
            {
                "status": judge["status"],
                "code": f"required-judge-{judge['status']}",
                "message": "configured weighted judge did not pass",
                "error": judge.get("error"),
            }
        )
    threshold = float(task.get("passThreshold", 0.8))
    if ratio is not None and ratio < threshold:
        reasons.append(
            {
                "status": "failed",
                "code": "ratio-below-threshold",
                "message": "weighted ratio is below passThreshold",
                "ratio": rounded(ratio),
                "passThreshold": threshold,
            }
        )
    evaluation_status = aggregate_status(reasons)
    passed = evaluation_status == "passed"
    legacy = version == 1
    certification = {
        "eligible": passed and not legacy,
        "status": "legacy-non-certifying" if legacy else ("quality-contract-passed" if passed else "not-certified"),
        "reason": (
            "version 1 tasks provide structural diagnostics only and cannot certify substantive quality"
            if legacy
            else "version 2 fail-closed contract satisfied" if passed else "evaluation did not pass"
        ),
    }
    return {
        "taskId": task["taskId"],
        **contract_fields(version),
        "inputValid": True,
        "legacy": legacy,
        "qualityCritical": version == 2,
        "evaluationStatus": evaluation_status,
        "evaluationErrors": reasons,
        "score": rounded(got),
        "maxScore": rounded(max_score),
        "deterministicRatio": rounded(deterministic_ratio),
        "ratio": rounded(ratio),
        "passThreshold": threshold,
        "passed": passed,
        "certified": certification["eligible"],
        "certification": certification,
        "compatibility": compatibility_fields(),
        "checks": results,
        "judge": judge,
    }


def cli_error(code, message):
    return {
        "inputValid": False,
        "evaluationStatus": "error",
        "evaluationErrors": [error_record(code, message)],
        "passed": False,
        "certified": False,
        "certification": {"eligible": False, "status": "input-error", "reason": message},
        "checks": [],
        "judge": base_judge(code="judge-not-run", message="input validation failed"),
    }


def print_json(payload):
    print(json.dumps(payload, sort_keys=True, indent=2, allow_nan=False))


try:
    task_schema = load_json(task_schema_path)
    evaluator_schema = load_json(evaluator_schema_path)
    if task_schema.get("$id") != "https://bubbles.dev/eval/schemas/task-v2.schema.json":
        raise ValueError("unexpected task schema id")
    if evaluator_schema.get("$id") != "https://bubbles.dev/eval/schemas/evaluator-result.schema.json":
        raise ValueError("unexpected evaluator schema id")
except (OSError, TypeError, ValueError, json.JSONDecodeError) as exc:
    print_json(cli_error("schema-contract-unavailable", f"evaluation schema contract could not be loaded: {type(exc).__name__}"))
    sys.exit(2)


if op == "score":
    task_path = os.environ["TASK"]
    if not task_path or not os.path.isfile(task_path):
        print_json(cli_error("task-not-found", "--task must name an existing file"))
        sys.exit(2)
    if not output or not os.path.isdir(output):
        print_json(cli_error("output-not-found", "--output must name an existing directory"))
        sys.exit(2)
    result = score_task(os.path.realpath(task_path), os.path.realpath(output))
    print_json(result)
    if not result["inputValid"]:
        sys.exit(2)
    sys.exit(0 if result["evaluationStatus"] == "passed" else 1)

if op == "run":
    suite = os.environ["SUITE"]
    if not suite or not os.path.isdir(suite):
        print_json(cli_error("suite-not-found", "--suite must name an existing directory"))
        sys.exit(2)
    if not output or not os.path.isdir(output):
        print_json(cli_error("output-not-found", "--output must name an existing directory"))
        sys.exit(2)
    task_paths = sorted(glob.glob(os.path.join(os.path.realpath(suite), "*.json")))
    results = [score_task(task_path, os.path.realpath(output)) for task_path in task_paths]
    passed_count = sum(1 for result in results if result["evaluationStatus"] == "passed")
    invalid_count = sum(1 for result in results if not result["inputValid"])
    certifying_passed = sum(1 for result in results if result.get("certified") is True)
    legacy_passed = sum(
        1
        for result in results
        if result["evaluationStatus"] == "passed" and result.get("legacy") is True
    )
    ratios = [result["ratio"] for result in results if is_number(result.get("ratio"))]
    suite_reasons = []
    if not results:
        suite_reasons.append(
            {"status": "error", "code": "suite-empty", "message": "suite contains no task JSON files"}
        )
    for result in results:
        if result["evaluationStatus"] != "passed":
            suite_reasons.append(
                {
                    "status": result["evaluationStatus"],
                    "code": "task-not-passed",
                    "message": "suite task did not pass",
                    "taskId": result["taskId"],
                }
            )
    suite_status = aggregate_status(suite_reasons)
    all_passed = bool(results) and suite_status == "passed"
    suite_certified = all_passed and certifying_passed == len(results)
    aggregate = {
        "suite": os.path.realpath(suite),
        "evaluationStatus": suite_status,
        "evaluationErrors": suite_reasons,
        "taskCount": len(results),
        "passed": passed_count,
        "failed": len(results) - passed_count,
        "allPassed": all_passed,
        "inputInvalid": invalid_count,
        "certifyingPassed": certifying_passed,
        "legacyPassed": legacy_passed,
        "certified": suite_certified,
        "certification": {
            "eligible": suite_certified,
            "status": (
                "quality-contract-passed"
                if suite_certified
                else "legacy-only-non-certifying" if all_passed and legacy_passed else "not-certified"
            ),
            "reason": (
                "all suite tasks passed the version 2 fail-closed contract"
                if suite_certified
                else "one or more passing tasks use the legacy non-certifying contract"
                if all_passed and legacy_passed
                else "suite did not pass"
            ),
        },
        "meanRatio": rounded(sum(ratios) / len(ratios)) if ratios else None,
        "compatibility": compatibility_fields(),
        "tasks": results,
    }
    print_json(aggregate)
    if invalid_count:
        sys.exit(2)
    sys.exit(0 if all_passed else 1)

print_json(cli_error("unknown-operation", f"unknown operation: {op}"))
sys.exit(2)
PY
