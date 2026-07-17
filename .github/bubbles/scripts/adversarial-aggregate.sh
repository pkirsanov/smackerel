#!/usr/bin/env bash
#
# adversarial-aggregate.sh — deterministic aggregation for correlated
# adversarial sample results (IMP-020 S2 runtime core).
#
# Every input must satisfy adversarial-sample.schema.json v1. This script uses
# a matching standard-library validator so aggregation does not depend on a
# separately installed JSON Schema package. Sample content is parsed as data;
# it is never passed to a shell or command runner.
#
# Finding identity is sha256 over the canonical JSON array:
#   [category, target, evidenceRef, claim]
# Every unique fingerprint is retained. Per-sample rows and fingerprint lists
# are sorted so input-file order cannot affect output bytes.
#
# Outcomes:
#   agreement-clear     every completed sample is clear with no findings
#   agreement-findings  every completed sample reports the same finding and
#                       blocking-finding fingerprint sets
#   disagreement        completed sample verdict or finding sets differ; the
#                       complete finding union is retained
#   aggregation-error   count, input, schema, provenance, or sample status is
#                       invalid for aggregation
#
# Exit codes:
#   0  valid agreement-clear or agreement-findings
#   1  valid disagreement
#   2  aggregation-error (including Python unavailable)

set -euo pipefail

EXPECTED_SAMPLES=""
EXPECTED_SEEN=0
SAMPLE_FILES=()

usage() {
  cat >&2 <<'USAGE'
Usage: bash bubbles/scripts/adversarial-aggregate.sh --expected-samples <N> <sample-result.json>...

Aggregates exactly N closed-v1 sample results and emits canonical JSON.
Exit 0: agreement | 1: disagreement | 2: aggregation error.
USAGE
}

emit_shell_error() {
  local code="$1" message="$2" expected_json="$3" actual="$4"
  printf '{"actualSamples":%s,"errors":[{"code":"%s","message":"%s","path":""}],"expectedSamples":%s,"findings":[],"kind":"adversarial-sample-aggregate","outcome":"aggregation-error","sampleIds":[],"sampleMatrix":[],"sampleSemantics":"same-runtime-correlated","schemaVersion":1}\n' \
    "$actual" "$code" "$message" "$expected_json"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --expected-samples)
      if [[ "$EXPECTED_SEEN" -eq 1 ]]; then
        echo "adversarial-aggregate: duplicate --expected-samples flag" >&2
        emit_shell_error "duplicate-expected-samples" "expected sample count was provided more than once" "null" "${#SAMPLE_FILES[@]}"
        exit 2
      fi
      EXPECTED_SEEN=1
      shift
      if [[ $# -eq 0 ]]; then
        echo "adversarial-aggregate: --expected-samples requires a value" >&2
        emit_shell_error "missing-expected-samples-value" "expected sample count value is required" "null" "${#SAMPLE_FILES[@]}"
        exit 2
      fi
      EXPECTED_SAMPLES="$1"
      shift
      ;;
    --expected-samples=*)
      if [[ "$EXPECTED_SEEN" -eq 1 ]]; then
        echo "adversarial-aggregate: duplicate --expected-samples flag" >&2
        emit_shell_error "duplicate-expected-samples" "expected sample count was provided more than once" "null" "${#SAMPLE_FILES[@]}"
        exit 2
      fi
      EXPECTED_SEEN=1
      EXPECTED_SAMPLES="${1#*=}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do
        SAMPLE_FILES+=("$1")
        shift
      done
      ;;
    -*)
      echo "adversarial-aggregate: unknown option" >&2
      emit_shell_error "unknown-option" "an unknown option was provided" "null" "${#SAMPLE_FILES[@]}"
      exit 2
      ;;
    *)
      SAMPLE_FILES+=("$1")
      shift
      ;;
  esac
done

if [[ "$EXPECTED_SEEN" -ne 1 ]]; then
  echo "adversarial-aggregate: --expected-samples is required" >&2
  emit_shell_error "missing-expected-samples" "explicit expected sample count is required" "null" "${#SAMPLE_FILES[@]}"
  exit 2
fi

if ! printf '%s' "$EXPECTED_SAMPLES" | grep -qE '^[1-9][0-9]*$'; then
  echo "adversarial-aggregate: invalid expected sample count" >&2
  emit_shell_error "invalid-expected-samples" "expected sample count must be a positive integer" "null" "${#SAMPLE_FILES[@]}"
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "adversarial-aggregate: python3 unavailable" >&2
  emit_shell_error "python3-unavailable" "python3 is required for adversarial aggregation" "$EXPECTED_SAMPLES" "${#SAMPLE_FILES[@]}"
  exit 2
fi

python3 - "$EXPECTED_SAMPLES" "${SAMPLE_FILES[@]}" <<'PY'
import datetime
import hashlib
import json
import re
import sys


EXPECTED = int(sys.argv[1])
PATHS = sys.argv[2:]
SAMPLE_SEMANTICS = "same-runtime-correlated"
ROOT_KEYS = {
    "schemaVersion",
    "sampleId",
    "sampleSemantics",
    "status",
    "verdict",
    "invokedAt",
    "invocationId",
    "provenance",
    "findings",
    "error",
}
ROOT_REQUIRED = ROOT_KEYS - {"error"}
PROVENANCE_KEYS = {"runtime", "model", "tools"}
RUNTIME_KEYS = {"runtimeId", "verificationState", "evidenceRef"}
MODEL_KEYS = {"provider", "modelId", "verificationState", "evidenceRef"}
TOOL_KEYS = {"inventoryId", "inventoryHash", "verificationState", "evidenceRef"}
FINDING_KEYS = {"category", "target", "evidenceRef", "claim", "blocking"}
ERROR_KEYS = {"code", "message"}
VERIFICATION_STATES = {
    "host-verified",
    "provider-verified",
    "self-reported",
    "unverified",
}
IDENTIFIER = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,159}$")
SHA256 = re.compile(r"^sha256:[0-9a-f]{64}$")


def reject_constant(value):
    raise ValueError(f"non-finite JSON number: {value}")


def add_error(errors, code, path, message):
    errors.append({"code": code, "path": path, "message": message})


def validate_closed_object(value, path, required, allowed, errors):
    if not isinstance(value, dict):
        add_error(errors, "schema-type", path, "expected object")
        return False
    for key in sorted(required - set(value)):
        add_error(errors, "schema-required", f"{path}.{key}", "required property is missing")
    for key in sorted(set(value) - allowed):
        add_error(errors, "schema-additional-property", f"{path}.{key}", "property is not allowed")
    return True


def validate_string(value, path, errors, identifier=False, maximum=4096):
    if not isinstance(value, str):
        add_error(errors, "schema-type", path, "expected string")
        return False
    if not value or len(value) > maximum or "\x00" in value:
        add_error(errors, "schema-string", path, f"expected non-empty string of at most {maximum} characters")
        return False
    if identifier and not IDENTIFIER.fullmatch(value):
        add_error(errors, "schema-identifier", path, "expected portable identifier")
        return False
    return True


def validate_timestamp(value, path, errors):
    if not validate_string(value, path, errors, maximum=128):
        return
    candidate = value[:-1] + "+00:00" if value.endswith("Z") else value
    try:
        parsed = datetime.datetime.fromisoformat(candidate)
    except ValueError:
        add_error(errors, "schema-date-time", path, "expected RFC 3339 date-time")
        return
    if parsed.tzinfo is None:
        add_error(errors, "schema-date-time", path, "date-time must include a timezone")


def validate_verification(value, path, errors):
    if not validate_string(value, path, errors, maximum=32):
        return
    if value not in VERIFICATION_STATES:
        add_error(errors, "schema-enum", path, "unknown verification state")


def validate_provenance(value, path, errors):
    if not validate_closed_object(value, path, PROVENANCE_KEYS, PROVENANCE_KEYS, errors):
        return

    runtime = value.get("runtime")
    runtime_path = f"{path}.runtime"
    if validate_closed_object(runtime, runtime_path, RUNTIME_KEYS, RUNTIME_KEYS, errors):
        validate_string(runtime.get("runtimeId"), f"{runtime_path}.runtimeId", errors, maximum=512)
        validate_verification(runtime.get("verificationState"), f"{runtime_path}.verificationState", errors)
        validate_string(runtime.get("evidenceRef"), f"{runtime_path}.evidenceRef", errors)

    model = value.get("model")
    model_path = f"{path}.model"
    if validate_closed_object(model, model_path, MODEL_KEYS, MODEL_KEYS, errors):
        validate_string(model.get("provider"), f"{model_path}.provider", errors, maximum=512)
        validate_string(model.get("modelId"), f"{model_path}.modelId", errors, maximum=512)
        validate_verification(model.get("verificationState"), f"{model_path}.verificationState", errors)
        validate_string(model.get("evidenceRef"), f"{model_path}.evidenceRef", errors)

    tools = value.get("tools")
    tools_path = f"{path}.tools"
    if validate_closed_object(tools, tools_path, TOOL_KEYS, TOOL_KEYS, errors):
        validate_string(tools.get("inventoryId"), f"{tools_path}.inventoryId", errors, maximum=512)
        inventory_hash = tools.get("inventoryHash")
        if validate_string(inventory_hash, f"{tools_path}.inventoryHash", errors, maximum=71):
            if not SHA256.fullmatch(inventory_hash):
                add_error(errors, "schema-sha256", f"{tools_path}.inventoryHash", "expected sha256:<64 lowercase hex characters>")
        validate_verification(tools.get("verificationState"), f"{tools_path}.verificationState", errors)
        validate_string(tools.get("evidenceRef"), f"{tools_path}.evidenceRef", errors)


def validate_finding(value, path, errors):
    if not validate_closed_object(value, path, FINDING_KEYS, FINDING_KEYS, errors):
        return
    validate_string(value.get("category"), f"{path}.category", errors)
    validate_string(value.get("target"), f"{path}.target", errors)
    validate_string(value.get("evidenceRef"), f"{path}.evidenceRef", errors)
    validate_string(value.get("claim"), f"{path}.claim", errors)
    if type(value.get("blocking")) is not bool:
        add_error(errors, "schema-type", f"{path}.blocking", "expected boolean")


def validate_error(value, path, errors):
    if not validate_closed_object(value, path, ERROR_KEYS, ERROR_KEYS, errors):
        return
    validate_string(value.get("code"), f"{path}.code", errors, identifier=True, maximum=160)
    validate_string(value.get("message"), f"{path}.message", errors)


def validate_sample(sample, path, errors):
    if not validate_closed_object(sample, path, ROOT_REQUIRED, ROOT_KEYS, errors):
        return
    if sample.get("schemaVersion") != 1 or type(sample.get("schemaVersion")) is not int:
        add_error(errors, "schema-version", f"{path}.schemaVersion", "expected integer 1")
    validate_string(sample.get("sampleId"), f"{path}.sampleId", errors, identifier=True, maximum=160)
    if sample.get("sampleSemantics") != SAMPLE_SEMANTICS:
        add_error(errors, "schema-semantics", f"{path}.sampleSemantics", f"expected {SAMPLE_SEMANTICS}")

    status = sample.get("status")
    verdict = sample.get("verdict")
    if status not in {"completed", "error", "unavailable"}:
        add_error(errors, "schema-enum", f"{path}.status", "unknown sample status")
    if verdict not in {"clear", "findings", "error", "unavailable"}:
        add_error(errors, "schema-enum", f"{path}.verdict", "unknown sample verdict")
    expected_verdicts = {
        "completed": {"clear", "findings"},
        "error": {"error"},
        "unavailable": {"unavailable"},
    }
    if status in expected_verdicts and verdict not in expected_verdicts[status]:
        add_error(errors, "schema-status-verdict", f"{path}.verdict", "verdict does not match status")

    validate_timestamp(sample.get("invokedAt"), f"{path}.invokedAt", errors)
    validate_string(sample.get("invocationId"), f"{path}.invocationId", errors, identifier=True, maximum=160)
    validate_provenance(sample.get("provenance"), f"{path}.provenance", errors)

    findings = sample.get("findings")
    if not isinstance(findings, list):
        add_error(errors, "schema-type", f"{path}.findings", "expected array")
        findings = []
    for index, finding in enumerate(findings):
        validate_finding(finding, f"{path}.findings[{index}]", errors)
    if verdict == "clear" and findings:
        add_error(errors, "schema-clear-findings", f"{path}.findings", "clear verdict cannot contain findings")
    if verdict == "findings" and not findings:
        add_error(errors, "schema-findings-empty", f"{path}.findings", "findings verdict requires at least one finding")

    if status == "completed" and "error" in sample:
        add_error(errors, "schema-completed-error", f"{path}.error", "completed sample cannot contain error")
    if status in {"error", "unavailable"}:
        if "error" not in sample:
            add_error(errors, "schema-required", f"{path}.error", "non-completed sample requires error details")
        else:
            validate_error(sample.get("error"), f"{path}.error", errors)
    elif "error" in sample:
        validate_error(sample.get("error"), f"{path}.error", errors)


def fingerprint(finding):
    identity = [
        finding["category"],
        finding["target"],
        finding["evidenceRef"],
        finding["claim"],
    ]
    payload = json.dumps(identity, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return "sha256:" + hashlib.sha256(payload).hexdigest()


errors = []
samples = []
if len(PATHS) != EXPECTED:
    add_error(
        errors,
        "sample-count-mismatch",
        "",
        f"expected {EXPECTED} sample files but received {len(PATHS)}",
    )

for index, path in enumerate(PATHS):
    location = f"inputs[{index}]"
    try:
        with open(path, "r", encoding="utf-8") as handle:
            sample = json.load(handle, parse_constant=reject_constant)
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError) as exc:
        add_error(errors, "sample-input-invalid", location, f"cannot load sample result: {exc}")
        continue
    before = len(errors)
    validate_sample(sample, location, errors)
    if len(errors) == before:
        samples.append(sample)

sample_ids = [sample["sampleId"] for sample in samples]
seen_ids = set()
for sample_id in sorted(sample_ids):
    if sample_id in seen_ids:
        add_error(errors, "duplicate-sample-id", "sampleId", f"duplicate sample ID: {sample_id}")
    seen_ids.add(sample_id)

invocation_ids = [sample["invocationId"] for sample in samples]
seen_invocation_ids = set()
for invocation_id in sorted(invocation_ids):
    if invocation_id in seen_invocation_ids:
        add_error(
            errors,
            "duplicate-invocation-id",
            "invocationId",
            f"duplicate invocation ID: {invocation_id}",
        )
    seen_invocation_ids.add(invocation_id)

union = {}
matrix = []
for sample in sorted(samples, key=lambda item: item["sampleId"]):
    sample_fingerprints = {}
    for finding in sample["findings"]:
        value = fingerprint(finding)
        prior = sample_fingerprints.get(value)
        if prior is not None and prior != finding["blocking"]:
            add_error(
                errors,
                "sample-finding-blocking-conflict",
                sample["sampleId"],
                f"finding {value} has conflicting blocking values within one sample",
            )
        sample_fingerprints[value] = bool(finding["blocking"]) or bool(prior)
        entry = union.get(value)
        if entry is None:
            entry = {
                "fingerprint": value,
                "category": finding["category"],
                "target": finding["target"],
                "evidenceRef": finding["evidenceRef"],
                "claim": finding["claim"],
                "blocking": bool(finding["blocking"]),
                "sampleIds": set(),
            }
            union[value] = entry
        entry["blocking"] = entry["blocking"] or bool(finding["blocking"])
        entry["sampleIds"].add(sample["sampleId"])

    all_fingerprints = sorted(sample_fingerprints)
    blocking_fingerprints = sorted(
        value for value, is_blocking in sample_fingerprints.items() if is_blocking
    )
    matrix.append(
        {
            "sampleId": sample["sampleId"],
            "status": sample["status"],
            "verdict": sample["verdict"],
            "invokedAt": sample["invokedAt"],
            "invocationId": sample["invocationId"],
            "provenance": sample["provenance"],
            "findingFingerprints": all_fingerprints,
            "blockingFindingFingerprints": blocking_fingerprints,
        }
    )

for sample in samples:
    if sample["status"] != "completed":
        add_error(
            errors,
            "sample-not-completed",
            sample["sampleId"],
            f"sample status is {sample['status']}",
        )

findings = []
for value in sorted(union):
    entry = union[value]
    entry["sampleIds"] = sorted(entry["sampleIds"])
    findings.append(entry)

errors.sort(key=lambda item: (item["path"], item["code"], item["message"]))
if errors:
    outcome = "aggregation-error"
    exit_code = 2
else:
    verdicts = {row["verdict"] for row in matrix}
    finding_sets = {tuple(row["findingFingerprints"]) for row in matrix}
    blocking_sets = {tuple(row["blockingFindingFingerprints"]) for row in matrix}
    if verdicts == {"clear"} and finding_sets == {()}:
        outcome = "agreement-clear"
        exit_code = 0
    elif verdicts == {"findings"} and len(finding_sets) == 1 and len(blocking_sets) == 1:
        outcome = "agreement-findings"
        exit_code = 0
    else:
        outcome = "disagreement"
        exit_code = 1

result = {
    "schemaVersion": 1,
    "kind": "adversarial-sample-aggregate",
    "outcome": outcome,
    "sampleSemantics": SAMPLE_SEMANTICS,
    "expectedSamples": EXPECTED,
    "actualSamples": len(PATHS),
    "sampleIds": sorted(sample_ids),
    "findings": findings,
    "sampleMatrix": matrix,
    "errors": errors,
}
sys.stdout.write(json.dumps(result, sort_keys=True, separators=(",", ":"), ensure_ascii=False))
sys.stdout.write("\n")
sys.exit(exit_code)
PY