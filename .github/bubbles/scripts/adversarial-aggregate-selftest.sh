#!/usr/bin/env bash
set -uo pipefail

# adversarial-aggregate-selftest.sh — IMP-020 S2 deterministic aggregation.
#
# Fixtures are generated under a temporary directory and are always parsed as
# JSON data. The cases are intentionally adversarial: any majority-silencing,
# input-order dependence, provenance omission, or shell evaluation must fail.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
AGGREGATE="$SCRIPT_DIR/adversarial-aggregate.sh"
SCHEMA="$SCRIPT_DIR/../eval/schemas/adversarial-sample.schema.json"
BASH_BIN="${BASH:-bash}"

pass=0
fail=0
RUN_STDOUT=""
RUN_STDERR=""
RUN_STATUS=0

tmp="$(mktemp -d "${HOME}/.bubbles-selftest-adversarial-aggregate.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

record_pass() {
  pass=$((pass + 1))
}

record_fail() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

assert_status() { # label expected
  if [[ "$RUN_STATUS" -eq "$2" ]]; then
    record_pass
  else
    record_fail "$1 (expected status $2, got $RUN_STATUS)"
    printf '%s\n' "$RUN_STDOUT" | sed 's/^/    stdout: /'
    printf '%s\n' "$RUN_STDERR" | sed 's/^/    stderr: /'
  fi
}

assert_eq() { # label actual expected
  if [[ "$2" == "$3" ]]; then
    record_pass
  else
    record_fail "$1"
    printf '    expected: %s\n' "$3"
    printf '    actual:   %s\n' "$2"
  fi
}

assert_file_absent() { # label path
  if [[ ! -e "$2" ]]; then
    record_pass
  else
    record_fail "$1 (unexpected path exists: $2)"
  fi
}

if ! command -v python3 >/dev/null 2>&1; then
  echo "FAIL: python3 is required to execute adversarial aggregation checks"
  exit 1
fi
PYTHON_BIN="$(command -v python3)"

cat > "$tmp/generate-fixtures.py" <<'PY'
import copy
import json
import pathlib
import sys


root = pathlib.Path(sys.argv[1])
sentinel = root / "shell-text-executed"

provenance = {
    "runtime": {
        "runtimeId": "vscode.active-runtime",
        "verificationState": "host-verified",
        "evidenceRef": "evidence://runtime/session-1",
    },
    "model": {
        "provider": "fixture-provider",
        "modelId": "fixture-model-v1",
        "verificationState": "self-reported",
        "evidenceRef": "evidence://model/session-1",
    },
    "tools": {
        "inventoryId": "fixture-tools-v1",
        "inventoryHash": "sha256:" + ("a" * 64),
        "verificationState": "host-verified",
        "evidenceRef": "evidence://tools/session-1",
    },
}

finding_alpha = {
    "category": "contract",
    "target": "resolver",
    "evidenceRef": "evidence://finding/alpha",
    "claim": "canonical samples output is required",
    "blocking": True,
}
finding_beta = {
    "category": "provenance",
    "target": "aggregator",
    "evidenceRef": "evidence://finding/beta",
    "claim": "tool inventory must be retained",
    "blocking": False,
}
finding_shared_soft = {
    "category": "provenance",
    "target": "sample-runtime",
    "evidenceRef": "evidence://finding/shared",
    "claim": "runtime identity is not host verified",
    "blocking": False,
}
finding_shared_hard = dict(finding_shared_soft, blocking=True)
finding_outlier = {
    "category": "safety",
    "target": "certification",
    "evidenceRef": "evidence://finding/outlier",
    "claim": "one sample found a blocking counterexample",
    "blocking": True,
}
finding_duplicate = {
    "category": "contract",
    "target": "finding-union",
    "evidenceRef": "evidence://finding/duplicate",
    "claim": "duplicate evidence must collapse by fingerprint",
    "blocking": True,
}
finding_shell = {
    "category": "hostile-text",
    "target": "shell-boundary",
    "evidenceRef": "evidence://finding/shell-text",
    "claim": "$(touch {0}); `touch {0}`; ${{SHOULD_NOT_EXPAND}}; ; touch {0}".format(sentinel),
    "blocking": True,
}


def sample(sample_id, findings=None, status="completed", verdict=None, error=None):
    findings = copy.deepcopy(findings or [])
    if verdict is None:
        verdict = "findings" if findings else "clear"
    result = {
        "schemaVersion": 1,
        "sampleId": sample_id,
        "sampleSemantics": "same-runtime-correlated",
        "status": status,
        "verdict": verdict,
        "invokedAt": "2026-07-11T12:00:00Z",
        "invocationId": "invocation:" + sample_id,
        "provenance": copy.deepcopy(provenance),
        "findings": findings,
    }
    if error is not None:
        result["error"] = error
    return result


def write(name, value):
    (root / name).write_text(
        json.dumps(value, ensure_ascii=False, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )


write("clear-a.json", sample("clear-a"))
write("clear-b.json", sample("clear-b"))
write("clear-c.json", sample("clear-c"))
write("same-findings-a.json", sample("same-findings-a", [finding_alpha, finding_beta]))
write("same-findings-b.json", sample("same-findings-b", [finding_beta, finding_alpha]))
write("blocking-soft.json", sample("blocking-soft", [finding_shared_soft]))
write("blocking-hard.json", sample("blocking-hard", [finding_shared_hard]))
write("outlier-blocking.json", sample("outlier-blocking", [finding_outlier]))
write("duplicate-a.json", sample("duplicate-a", [finding_duplicate, finding_duplicate]))
write("duplicate-b.json", sample("duplicate-b", [finding_duplicate]))
write("shell-shaped.json", sample("shell-shaped", [finding_shell]))

duplicate_id_a = sample("duplicate-id")
duplicate_id_b = sample("duplicate-id")
duplicate_id_b["invocationId"] = "invocation:duplicate-id:second"
write("duplicate-id-a.json", duplicate_id_a)
write("duplicate-id-b.json", duplicate_id_b)

duplicate_invocation_a = sample("duplicate-invocation-a")
duplicate_invocation_b = sample("duplicate-invocation-b")
duplicate_invocation_a["invocationId"] = "same-runtime-invocation-01"
duplicate_invocation_b["invocationId"] = "same-runtime-invocation-01"
write("duplicate-invocation-a.json", duplicate_invocation_a)
write("duplicate-invocation-b.json", duplicate_invocation_b)

unknown_field = sample("unknown-field")
unknown_field["majorityHint"] = "clear"
write("unknown-field.json", unknown_field)

wrong_semantics = sample("wrong-semantics")
wrong_semantics["sampleSemantics"] = "independent"
write("wrong-semantics.json", wrong_semantics)

missing_runtime = sample("missing-runtime")
del missing_runtime["provenance"]["runtime"]
write("missing-runtime.json", missing_runtime)

invalid_runtime = sample("invalid-runtime")
invalid_runtime["provenance"]["runtime"]["verificationState"] = "guessed"
write("invalid-runtime.json", invalid_runtime)

missing_model = sample("missing-model")
del missing_model["provenance"]["model"]
write("missing-model.json", missing_model)

invalid_model = sample("invalid-model")
invalid_model["provenance"]["model"]["modelId"] = ""
write("invalid-model.json", invalid_model)

missing_tools = sample("missing-tools")
del missing_tools["provenance"]["tools"]
write("missing-tools.json", missing_tools)

invalid_tools = sample("invalid-tools")
invalid_tools["provenance"]["tools"]["inventoryHash"] = "sha256:not-a-digest"
write("invalid-tools.json", invalid_tools)

incomplete = sample(
    "incomplete-sample",
    status="incomplete",
    verdict="error",
    error={"code": "still-running", "message": "sample did not reach a terminal status"},
)
write("incomplete.json", incomplete)

error_sample = sample(
    "error-sample",
    status="error",
    verdict="error",
    error={"code": "runtime-error", "message": "sample execution failed"},
)
write("status-error.json", error_sample)

unavailable_sample = sample(
    "unavailable-sample",
    status="unavailable",
    verdict="unavailable",
    error={"code": "runtime-unavailable", "message": "sample runtime was unavailable"},
)
write("status-unavailable.json", unavailable_sample)

(root / "malformed.json").write_text('{"schemaVersion":', encoding="utf-8")
PY

cat > "$tmp/json-assert.py" <<'PY'
import json
import os
import re
import sys


data = json.loads(os.environ["AGG_JSON"])
check = sys.argv[1]
args = sys.argv[2:]


def resolve(path):
    value = data
    if not path:
        return value
    for part in path.split("."):
        value = value[int(part)] if isinstance(value, list) else value[part]
    return value


def fail(message):
    print(message, file=sys.stderr)
    raise SystemExit(1)


if check == "value":
    actual = resolve(args[0])
    expected = json.loads(args[1])
    if actual != expected:
        fail(f"{args[0]} expected {expected!r}, got {actual!r}")
elif check == "length":
    actual = len(resolve(args[0]))
    expected = int(args[1])
    if actual != expected:
        fail(f"len({args[0]}) expected {expected}, got {actual}")
elif check == "error-code":
    codes = [entry.get("code") for entry in data.get("errors", [])]
    if args[0] not in codes:
        fail(f"error code {args[0]!r} not present in {codes!r}")
elif check == "error-path":
    paths = [entry.get("path", "") for entry in data.get("errors", [])]
    if not any(args[0] in path for path in paths):
        fail(f"error path containing {args[0]!r} not present in {paths!r}")
elif check == "canonical-order":
    sample_ids = data.get("sampleIds", [])
    if sample_ids != sorted(set(sample_ids)):
        fail(f"sampleIds are not sorted and unique: {sample_ids!r}")
    matrix_ids = [row["sampleId"] for row in data.get("sampleMatrix", [])]
    if matrix_ids != sorted(matrix_ids):
        fail(f"sampleMatrix is not sorted: {matrix_ids!r}")
    finding_ids = [finding["fingerprint"] for finding in data.get("findings", [])]
    if finding_ids != sorted(set(finding_ids)):
        fail(f"findings are not sorted and unique: {finding_ids!r}")
    for finding in data.get("findings", []):
        ids = finding["sampleIds"]
        if ids != sorted(set(ids)):
            fail(f"finding sampleIds are not sorted and unique: {ids!r}")
    for row in data.get("sampleMatrix", []):
        for key in ("findingFingerprints", "blockingFindingFingerprints"):
            values = row[key]
            if values != sorted(set(values)):
                fail(f"{row['sampleId']} {key} is not sorted and unique: {values!r}")
elif check == "finding-claims":
    actual = sorted(finding["claim"] for finding in data.get("findings", []))
    expected = sorted(args)
    if actual != expected:
        fail(f"finding claims expected {expected!r}, got {actual!r}")
elif check == "fingerprint-format":
    fingerprints = [finding["fingerprint"] for finding in data.get("findings", [])]
    if not fingerprints:
        fail("expected at least one fingerprint")
    pattern = re.compile(r"^sha256:[0-9a-f]{64}$")
    invalid = [value for value in fingerprints if not pattern.fullmatch(value)]
    if invalid:
        fail(f"invalid fingerprints: {invalid!r}")
elif check == "no-terms":
    terms = [term.lower() for term in args]
    hits = []

    def walk(value, path=""):
        if isinstance(value, dict):
            for key, child in value.items():
                key_path = f"{path}.{key}" if path else key
                lowered = key.lower()
                for term in terms:
                    if term in lowered:
                        hits.append(key_path)
                walk(child, key_path)
        elif isinstance(value, list):
            for index, child in enumerate(value):
                walk(child, f"{path}[{index}]")
        elif isinstance(value, str):
            lowered = value.lower()
            for term in terms:
                if term in lowered:
                    hits.append(f"{path}={value}")

    walk(data)
    if hits:
        fail(f"forbidden terms found: {hits!r}")
else:
    fail(f"unknown JSON assertion: {check}")
PY

cat > "$tmp/schema-check.py" <<'PY'
import json
import pathlib
import sys

import jsonschema


schema_path = pathlib.Path(sys.argv[1])
sample_path = pathlib.Path(sys.argv[2])
expectation = sys.argv[3]

schema = json.loads(schema_path.read_text(encoding="utf-8"))
try:
    sample = json.loads(sample_path.read_text(encoding="utf-8"))
except (OSError, UnicodeError, json.JSONDecodeError) as exc:
    if expectation == "invalid":
        raise SystemExit(0)
    print(f"clean sample could not be loaded: {exc}", file=sys.stderr)
    raise SystemExit(1)

validator = jsonschema.Draft202012Validator(
    schema,
    format_checker=jsonschema.FormatChecker(),
)
errors = sorted(validator.iter_errors(sample), key=lambda error: list(error.absolute_path))
if expectation == "valid" and errors:
    for error in errors:
        print(f"unexpected schema error at {list(error.absolute_path)}: {error.message}", file=sys.stderr)
    raise SystemExit(1)
if expectation == "invalid" and not errors:
    print("invalid mutation unexpectedly satisfied the schema", file=sys.stderr)
    raise SystemExit(1)
PY

if ! "$PYTHON_BIN" "$tmp/generate-fixtures.py" "$tmp"; then
  echo "FAIL: could not generate hermetic adversarial sample fixtures"
  exit 1
fi

run_aggregate() {
  local stderr_file="$tmp/aggregate.stderr"
  RUN_STDOUT=""
  RUN_STDERR=""
  RUN_STATUS=0
  if RUN_STDOUT="$("$BASH_BIN" "$AGGREGATE" "$@" 2>"$stderr_file")"; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_STDERR="$(cat "$stderr_file")"
}

assert_json() { # label check [arguments...]
  local label="$1"
  shift
  if AGG_JSON="$RUN_STDOUT" "$PYTHON_BIN" "$tmp/json-assert.py" "$@"; then
    record_pass
  else
    record_fail "$label"
    printf '%s\n' "$RUN_STDOUT" | sed 's/^/    /'
  fi
}

assert_json_value() { # label path expected-json
  assert_json "$1" value "$2" "$3"
}

assert_json_length() { # label path expected-length
  assert_json "$1" length "$2" "$3"
}

assert_aggregation_error_case() { # label file error-code error-path-fragment
  local label="$1"
  run_aggregate --expected-samples 1 "$2"
  assert_status "$label exits with aggregation error" 2
  assert_json_value "$label reports aggregation-error" outcome '"aggregation-error"'
  assert_json "$label reports $3" error-code "$3"
  assert_json "$label identifies $4" error-path "$4"
}

assert_schema_case() { # label file valid|invalid
  if "$PYTHON_BIN" "$tmp/schema-check.py" "$SCHEMA" "$2" "$3"; then
    record_pass
  else
    record_fail "$1"
  fi
}

# 1. Unanimous clear is agreement-clear with a sorted, empty union.
run_aggregate --expected-samples 3 "$tmp/clear-c.json" "$tmp/clear-a.json" "$tmp/clear-b.json"
assert_status "unanimous clear exits zero" 0
assert_json_value "unanimous clear outcome" outcome '"agreement-clear"'
assert_json_value "unanimous clear semantics" sampleSemantics '"same-runtime-correlated"'
assert_json_value "unanimous clear expected count" expectedSamples '3'
assert_json_value "unanimous clear actual count" actualSamples '3'
assert_json_value "unanimous clear sample IDs" sampleIds '["clear-a","clear-b","clear-c"]'
assert_json_length "unanimous clear has no findings" findings 0
assert_json_length "unanimous clear has no errors" errors 0
assert_json "unanimous clear canonical ordering" canonical-order

# 2. Same findings agree regardless of file order or finding order; bytes match.
run_aggregate --expected-samples 2 "$tmp/same-findings-a.json" "$tmp/same-findings-b.json"
assert_status "same findings exits zero" 0
assert_json_value "same findings outcome" outcome '"agreement-findings"'
assert_json_length "same findings retains both unique findings" findings 2
assert_json "same findings retains complete claim set" finding-claims \
  "canonical samples output is required" \
  "tool inventory must be retained"
assert_json "same findings fingerprints are stable SHA-256 values" fingerprint-format
assert_json "same findings canonical ordering" canonical-order
assert_json "agreement output has no majority or independence labels" no-terms \
  majority independent independence vote consensus ensemble
ordered_findings_output="$RUN_STDOUT"

run_aggregate --expected-samples 2 "$tmp/same-findings-b.json" "$tmp/same-findings-a.json"
assert_status "reversed same findings exits zero" 0
assert_eq "reversed file and finding order is byte-stable" "$RUN_STDOUT" "$ordered_findings_output"

# 3. A clear/findings verdict split is disagreement and preserves the union.
run_aggregate --expected-samples 2 "$tmp/clear-a.json" "$tmp/same-findings-a.json"
assert_status "verdict disagreement exits one" 1
assert_json_value "verdict disagreement outcome" outcome '"disagreement"'
assert_json_length "verdict disagreement keeps every unique finding" findings 2
assert_json "verdict disagreement keeps complete claim set" finding-claims \
  "canonical samples output is required" \
  "tool inventory must be retained"
assert_json "verdict disagreement canonical ordering" canonical-order
assert_json "disagreement output has no majority or independence labels" no-terms \
  majority independent independence vote consensus ensemble

# 4. Equal finding identities with different blocking sets must disagree.
run_aggregate --expected-samples 2 "$tmp/blocking-soft.json" "$tmp/blocking-hard.json"
assert_status "blocking-set disagreement exits one" 1
assert_json_value "blocking-set disagreement outcome" outcome '"disagreement"'
assert_json_length "blocking-set disagreement dedupes identity" findings 1
assert_json_value "blocking-set disagreement preserves blocking union" findings.0.blocking 'true'
assert_json_value "blocking-set disagreement records both sample IDs" findings.0.sampleIds \
  '["blocking-hard","blocking-soft"]'
assert_json "blocking-set disagreement canonical ordering" canonical-order

# 5. Two clear samples cannot outvote one blocking finding.
run_aggregate --expected-samples 3 "$tmp/clear-a.json" "$tmp/clear-b.json" "$tmp/outlier-blocking.json"
assert_status "two-clear-one-blocking exits one" 1
assert_json_value "two-clear-one-blocking is disagreement" outcome '"disagreement"'
assert_json_length "two-clear-one-blocking preserves finding union" findings 1
assert_json_value "two-clear-one-blocking retains blocking flag" findings.0.blocking 'true'
assert_json_value "two-clear-one-blocking retains finding source" findings.0.sampleIds \
  '["outlier-blocking"]'
assert_json "two-clear-one-blocking has no majority path" no-terms \
  majority independent independence vote consensus ensemble

# 6. Duplicate findings collapse to one stable fingerprint and sorted sample IDs.
run_aggregate --expected-samples 2 "$tmp/duplicate-a.json" "$tmp/duplicate-b.json"
assert_status "duplicate finding aggregation exits zero" 0
assert_json_value "duplicate finding outcome" outcome '"agreement-findings"'
assert_json_length "duplicate finding union has one entry" findings 1
assert_json_value "duplicate finding records unique sample IDs" findings.0.sampleIds \
  '["duplicate-a","duplicate-b"]'
assert_json_length "duplicate finding first matrix row has one fingerprint" \
  sampleMatrix.0.findingFingerprints 1
assert_json_length "duplicate finding second matrix row has one fingerprint" \
  sampleMatrix.1.findingFingerprints 1
assert_json "duplicate finding fingerprint format" fingerprint-format
duplicate_output="$RUN_STDOUT"

run_aggregate --expected-samples 2 "$tmp/duplicate-b.json" "$tmp/duplicate-a.json"
assert_status "reversed duplicate inputs exit zero" 0
assert_eq "duplicate fingerprint and sample IDs are byte-stable" "$RUN_STDOUT" "$duplicate_output"

# 7. Input/count/schema failures always return aggregation-error.
run_aggregate --expected-samples 3 "$tmp/clear-a.json" "$tmp/clear-b.json"
assert_status "exact count mismatch exits two" 2
assert_json_value "exact count mismatch outcome" outcome '"aggregation-error"'
assert_json_value "exact count mismatch expected count" expectedSamples '3'
assert_json_value "exact count mismatch actual count" actualSamples '2'
assert_json "exact count mismatch error code" error-code sample-count-mismatch

run_aggregate --expected-samples 2 "$tmp/duplicate-id-a.json" "$tmp/duplicate-id-b.json"
assert_status "two-file duplicate sample ID exits two" 2
assert_json_value "two-file duplicate sample ID outcome" outcome '"aggregation-error"'
assert_json "two-file duplicate sample ID error code" error-code duplicate-sample-id
assert_json "two-file duplicate sample ID error path" error-path sampleId
assert_json_length "two-file duplicate sample ID reports one distinct error" errors 1
assert_json_value "two-file duplicate sample ID remains distinct" errors.0.code '"duplicate-sample-id"'

run_aggregate --expected-samples 2 \
  "$tmp/duplicate-invocation-a.json" \
  "$tmp/duplicate-invocation-b.json"
assert_status "two-file duplicate invocation ID exits two" 2
assert_json_value "two-file duplicate invocation ID outcome" outcome '"aggregation-error"'
assert_json "two-file duplicate invocation ID error code" error-code duplicate-invocation-id
assert_json "two-file duplicate invocation ID error path" error-path invocationId
assert_json_value "two-file duplicate invocation ID retains both sample IDs" sampleIds \
  '["duplicate-invocation-a","duplicate-invocation-b"]'
assert_json_length "two-file duplicate invocation ID retains both matrix rows" sampleMatrix 2
assert_json_value "two-file duplicate invocation ID retains first matrix row" \
  sampleMatrix.0.sampleId '"duplicate-invocation-a"'
assert_json_value "two-file duplicate invocation ID retains second matrix row" \
  sampleMatrix.1.sampleId '"duplicate-invocation-b"'
assert_json_length "two-file duplicate invocation ID has zero findings" findings 0
duplicate_invocation_output="$RUN_STDOUT"

run_aggregate --expected-samples 2 \
  "$tmp/duplicate-invocation-b.json" \
  "$tmp/duplicate-invocation-a.json"
assert_status "reversed duplicate invocation inputs exit two" 2
assert_eq "reversed duplicate invocation output is byte-stable" \
  "$RUN_STDOUT" "$duplicate_invocation_output"

assert_aggregation_error_case \
  "malformed JSON" \
  "$tmp/malformed.json" \
  sample-input-invalid \
  'inputs[0]'
assert_aggregation_error_case \
  "unknown root field" \
  "$tmp/unknown-field.json" \
  schema-additional-property \
  majorityHint
assert_aggregation_error_case \
  "wrong sample semantics" \
  "$tmp/wrong-semantics.json" \
  schema-semantics \
  sampleSemantics

# 8. Runtime, model, and tool provenance are each required and validated.
assert_aggregation_error_case \
  "missing runtime provenance" \
  "$tmp/missing-runtime.json" \
  schema-required \
  provenance.runtime
assert_aggregation_error_case \
  "invalid runtime provenance" \
  "$tmp/invalid-runtime.json" \
  schema-enum \
  provenance.runtime.verificationState
assert_aggregation_error_case \
  "missing model provenance" \
  "$tmp/missing-model.json" \
  schema-required \
  provenance.model
assert_aggregation_error_case \
  "invalid model provenance" \
  "$tmp/invalid-model.json" \
  schema-string \
  provenance.model.modelId
assert_aggregation_error_case \
  "missing tool provenance" \
  "$tmp/missing-tools.json" \
  schema-required \
  provenance.tools
assert_aggregation_error_case \
  "invalid tool provenance" \
  "$tmp/invalid-tools.json" \
  schema-sha256 \
  provenance.tools.inventoryHash

# 9. Incomplete, error, and unavailable samples can never aggregate as agreement.
assert_aggregation_error_case \
  "incomplete sample" \
  "$tmp/incomplete.json" \
  schema-enum \
  status
assert_aggregation_error_case \
  "error sample" \
  "$tmp/status-error.json" \
  sample-not-completed \
  error-sample
assert_aggregation_error_case \
  "unavailable sample" \
  "$tmp/status-unavailable.json" \
  sample-not-completed \
  unavailable-sample

# 10. Shell-shaped finding data remains inert while surviving the JSON round trip.
shell_sentinel="$tmp/shell-text-executed"
run_aggregate --expected-samples 1 "$tmp/shell-shaped.json"
assert_status "shell-shaped finding exits zero" 0
assert_json_value "shell-shaped finding outcome" outcome '"agreement-findings"'
assert_json_length "shell-shaped finding retained once" findings 1
assert_json_value "shell-shaped finding text retained as data" findings.0.claim \
  "\"\$(touch $shell_sentinel); \`touch $shell_sentinel\`; \${SHOULD_NOT_EXPAND}; ; touch $shell_sentinel\""
assert_file_absent "shell-shaped finding did not execute" "$shell_sentinel"

# 11. A Python-free PATH fails closed with structured aggregation-error JSON.
no_python_path="$tmp/no-python-path"
mkdir -p "$no_python_path"
ln -s "$(command -v grep)" "$no_python_path/grep"
missing_python_stderr="$tmp/missing-python.stderr"
if RUN_STDOUT="$(env -i PATH="$no_python_path" "$BASH_BIN" "$AGGREGATE" \
  --expected-samples 1 "$tmp/clear-a.json" 2>"$missing_python_stderr")"; then
  RUN_STATUS=0
else
  RUN_STATUS=$?
fi
RUN_STDERR="$(cat "$missing_python_stderr")"
assert_status "missing Python exits two" 2
assert_json_value "missing Python outcome" outcome '"aggregation-error"'
assert_json "missing Python error code" error-code python3-unavailable

# 12. Optional jsonschema confirms clean shape and rejects invalid mutations.
# The runtime's manual validator above always runs; only this optional engine may skip.
if "$PYTHON_BIN" -c 'import jsonschema' >/dev/null 2>&1; then
  schema_clean_count=0
  for clean_sample in \
    "$tmp/clear-a.json" \
    "$tmp/duplicate-invocation-a.json" \
    "$tmp/duplicate-invocation-b.json" \
    "$tmp/same-findings-a.json" \
    "$tmp/status-error.json" \
    "$tmp/status-unavailable.json" \
    "$tmp/shell-shaped.json"; do
    assert_schema_case "jsonschema accepts $clean_sample" "$clean_sample" valid
    schema_clean_count=$((schema_clean_count + 1))
  done

  schema_invalid_count=0
  for invalid_sample in \
    "$tmp/unknown-field.json" \
    "$tmp/wrong-semantics.json" \
    "$tmp/missing-runtime.json" \
    "$tmp/invalid-runtime.json" \
    "$tmp/missing-model.json" \
    "$tmp/invalid-model.json" \
    "$tmp/missing-tools.json" \
    "$tmp/invalid-tools.json" \
    "$tmp/incomplete.json"; do
    assert_schema_case "jsonschema rejects $invalid_sample" "$invalid_sample" invalid
    schema_invalid_count=$((schema_invalid_count + 1))
  done
  echo "jsonschema validation: $schema_clean_count clean accepted, $schema_invalid_count invalid rejected"
else
  echo "SKIP: jsonschema validation (optional Python module not installed)"
fi

# 13. Active current-contract surfaces keep honest correlated-sample terminology.
# Historical snapshots are intentionally excluded; current generated public
# outputs are explicit active surfaces. Every exception is justified on the
# same physical line as the otherwise forbidden term.
active_source_surfaces=(
  "agents/bubbles.redteam.agent.md"
  "prompts/bubbles.redteam.prompt.md"
  "agents/bubbles.super.agent.md"
  "agents/bubbles_shared/agent-common.md"
  "bubbles/workflows.yaml"
  "skills/bubbles-workflow-mode-resolution/SKILL.md"
  "docs/recipes/adversarial-verification.md"
  "docs/recipes/cross-model-review.md"
  "docs/guides/AGENT_MANUAL.md"
  "docs/guides/WORKFLOW_MODES.md"
  "docs/recipes/README.md"
  "docs/CATALOG.md"
  "bubbles/cheatsheet/vocabulary.json"
  "docs/CHEATSHEET.md"
  "docs/its-not-rocket-appliances.html"
)

source_scan_output=""
source_scan_output_file="$tmp/active-source-scan.out"

# Keep this heredoc out of command substitution: Apple Bash 3.2 parses
# backticks in nested heredoc payloads as shell syntax.
"$PYTHON_BIN" - "$REPO_ROOT" "${active_source_surfaces[@]}" >"$source_scan_output_file" 2>&1 <<'PY'
import pathlib
import re
import sys


root = pathlib.Path(sys.argv[1])
relative_paths = sys.argv[2:]

permitted_phrase_patterns = [
  re.compile(pattern, re.IGNORECASE)
  for pattern in (
    r"\bmust\s+not\s+(?:be\s+)?(?:described|called|labeled|labelled|treated|presented|advertised|claimed|used)\b",
    r"\bnot\s+(?:an?\s+)?(?:evidence\s+of\s+)?(?:model\s+or\s+tool\s+)?(?:independent|independence|vot(?:e|es|ed|ing)|consensus|ensemble|cross[- ]model|external[- ]model|executable|implemented|enabled|supported|available|working)\b",
    r"\b(?:cross[- ]model|external[- ]model|independen(?:t|ce)|vot(?:e|es|ed|ing)|consensus|ensemble|crossModelReview|BUBBLES_ADVERSARIAL_PASSES|passes)\b.{0,60}\b(?:is|are|was|were|remains?|does|do|can)\s+(?:not|unsupported|unavailable|deprecated)\b",
    r"\b(?:unsupported|unavailable|deprecated|historical)\s+(?:compatibility\s+|former\s+|legacy\s+|current\s+)?(?:cross[- ]model|external[- ]model|independen(?:t|ce)|vot(?:e|es|ed|ing)|consensus|ensemble|crossModelReview|BUBBLES_ADVERSARIAL_PASSES|passes)\b",
    r"\b(?:cross[- ]model|external[- ]model|independen(?:t|ce)|vot(?:e|es|ed|ing)|consensus|ensemble|crossModelReview|BUBBLES_ADVERSARIAL_PASSES|passes)\b\s*(?::|—|-)\s*(?:unsupported|unavailable|deprecated|historical)\b",
    r"\bno\s+verified(?:\s+external)?(?:\s+(?:provider/model|model/provider|provider|model))?(?:\s+adapter)?\b",
    r"\bdifferent\s+(?:model|provider)(?:\s+or\s+(?:model|provider)|/provider|/model)?\b.{0,120}\bis\s+(?:therefore\s+)?unsupported\b",
    r"\bdifferent\s+(?:model|provider)(?:/provider|/model)?\b.{0,160}\bno\s+verified\s+external(?:\s+(?:provider/model|model/provider|provider|model))?\s+adapter\b",
    r"\b(?:multi[- ]AI(?:\s+second\s+opinion)?|(?:two|2)\s+(?:different\s+)?AIs?)\b(?:\s+(?:capability|execution|review|claim|wording|language|prose))?\s+(?:is|are|was|were|remains?)\s+(?:unsupported|unavailable|deprecated|historical|not\s+(?:supported|available|implemented|executable))\b",
    r"\b(?:unsupported|unavailable|deprecated|historical)\s+(?:multi[- ]AI(?:\s+second\s+opinion)?|(?:two|2)\s+(?:different\s+)?AIs?)\s+(?:capability|claim|wording|language|prose)\b",
    r"\b(?:multi[- ]AI(?:\s+second\s+opinion)?|(?:two|2)\s+(?:different\s+)?AIs?)\b\s*(?::|—|-)\s*(?:unsupported|unavailable|deprecated|historical)\b",
    r"\b(?:cannot|never)\s+(?:be\s+|provide\s+|establish\s+|prove\s+|claim\s+|run\s+|execute\s+)?(?:an?\s+)?(?:independen(?:t|ce)|vot(?:e|es|ed|ing)|consensus|ensemble|cross[- ]model|external[- ]model|different[- ](?:model|provider))\b",
    r"\bcannot\s+(?:currently\s+)?verify\s+(?:an?\s+)?different\s+(?:provider/model|model/provider|provider|model)\s+invocation\b",
    r"\brather\s+than\b.{0,120}\b(?:independen(?:t|ce)|vot(?:e|es|ed|ing)|consensus|ensemble|cross[- ]model|external[- ]model)\b",
    r"\bcross[- ]model\s+review\s*:\s*(?:unsupported|unavailable|deprecated|historical)\b",
    r"\bcross[- ]model\s+review\b.{0,80}\bmigration\s+note\b",
    r"\bmigration\s+note\b.{0,80}\bcross[- ]model\s+review\b",
    r"\bexternal\s+(?:provider|model|review)(?:/model|/provider)?\s+adapter\b.{0,80}\bwould\s+need\b",
    r"\b(?:cross[- ]model|external[- ]model|independen(?:t|ce)|vot(?:e|es|ed|ing)|consensus|ensemble|crossModelReview|BUBBLES_ADVERSARIAL_PASSES|passes)\b.{0,80}\b(?:historical|deprecated)\s+(?:only|compatibility|reference|discussion|syntax|alias|claim|recipe|state)\b",
  )
]

claim_patterns = [
  (
    "same-runtime samples advertised as independent",
    re.compile(
      r"(?:\bsame[- ]runtime\b|\bsamples?\b).{0,120}\bindependen(?:t|ce)\b"
      r"|\bindependen(?:t|ce)\b.{0,120}(?:\bsame[- ]runtime\b|\bsamples?\b)",
      re.IGNORECASE,
    ),
  ),
  (
    "independent validator/reviewer claim",
    re.compile(
      r"\bindependent(?:ly)?\s+(?:validator|reviewer|verification|review|evaluation|check|sample)s?\b"
      r"|\bindependent\s+validations?\b"
      r"|\b(?:validator|reviewer|validation|verification|review|evaluation|check|sample)s?\b.{0,40}\bindependent\b",
      re.IGNORECASE,
    ),
  ),
  ("voting claim", re.compile(r"\bvot(?:e|es|ed|ing|er|ers)\b", re.IGNORECASE)),
  ("consensus claim", re.compile(r"\bconsensus\b", re.IGNORECASE)),
  ("ensemble claim", re.compile(r"\bensembles?\b", re.IGNORECASE)),
  ("cross-model execution claim", re.compile(r"\bcross[- ]model\b", re.IGNORECASE)),
  (
    "external-model execution claim",
    re.compile(
      r"\bexternal[- ]model\b"
      r"|\bexternal\s+(?:provider|model|review|reviewer|validator)(?:/model|/provider)?(?:\s+(?:adapter|execution|invocation|review|validator))?\b",
      re.IGNORECASE,
    ),
  ),
  (
    "different-model execution claim",
    re.compile(r"\b(?:different|another)\s+(?:model|provider)(?:/provider)?\b", re.IGNORECASE),
  ),
  (
    "stale multi-AI/two-AI claim",
    re.compile(
      r"\bmulti[- ]AI\b|\b(?:two|2)\s+(?:different\s+)?AIs?\b",
      re.IGNORECASE,
    ),
  ),
]

syntax_patterns = [
  ("crossModelReview command/config syntax", re.compile(r"\bcrossModelReview\b")),
  ("BUBBLES_ADVERSARIAL_PASSES environment syntax", re.compile(r"\bBUBBLES_ADVERSARIAL_PASSES\b")),
  ("--passes command syntax", re.compile(r"(?<![A-Za-z0-9_-])--passes(?:\s|=|`|$)")),
  (
    "passes config/directive syntax",
    re.compile(r"(?<![A-Za-z0-9_])passes\s*:\s*\S", re.IGNORECASE),
  ),
  ("passes output syntax", re.compile(r"(?<![A-Za-z0-9_])passes\s*=\s*\S", re.IGNORECASE)),
  ("adversarial.passes config syntax", re.compile(r"\badversarial\.passes\b", re.IGNORECASE)),
]

workflow_cross_model_patterns = [
  re.compile(r"^\s*crossModelReview\s*:"),
  re.compile(r"^\s*-\s*crossModelReview\b"),
  re.compile(
    r"^\s*[A-Za-z0-9_-]*(?:command|action|invoke|workflow|mode)[A-Za-z0-9_-]*\s*:.*\bcrossModelReview\b",
    re.IGNORECASE,
  ),
]


def has_nearby_qualifier(line, match):
  clause_start = 0
  clause_end = len(line)
  for separator in (";", "|", ". ", " — "):
    prior = line.rfind(separator, 0, match.start())
    if prior >= 0:
      clause_start = max(clause_start, prior + len(separator))
    following = line.find(separator, match.end())
    if following >= 0:
      clause_end = min(clause_end, following)

  clause = line[clause_start:clause_end]
  return any(pattern.search(clause) for pattern in permitted_phrase_patterns)


def scan_lines(relative_path, lines):
  findings = []
  for line_number, line in enumerate(lines, start=1):
    for label, pattern in claim_patterns:
      for match in pattern.finditer(line):
        if not has_nearby_qualifier(line, match):
          findings.append((relative_path, line_number, label, line.rstrip()))

    for label, pattern in syntax_patterns:
      for match in pattern.finditer(line):
        if (
          relative_path == "bubbles/workflows.yaml"
          and label.startswith("crossModelReview")
          and any(pattern.search(line) for pattern in workflow_cross_model_patterns)
        ):
          findings.append((relative_path, line_number, label, line.rstrip()))
        elif not has_nearby_qualifier(line, match):
          findings.append((relative_path, line_number, label, line.rstrip()))
  return findings


classifier_cases = [
  (
    "a disclaimer on another line cannot whitelist a false claim",
    [
      "Cross-model review is unsupported.",
      "Same-runtime samples are independent validators.",
    ],
    True,
  ),
  ("voting ensemble advertising is rejected", ["Samples use a voting ensemble."], True),
  ("consensus advertising is rejected", ["The samples reach consensus."], True),
  (
    "unrelated same-line negation cannot whitelist a false claim",
    ["This is not a release; same-runtime samples are independent validators."],
    True,
  ),
  (
    "unrelated same-line unsupported prose cannot whitelist a false claim",
    ["The old CLI is unsupported; samples use a voting ensemble."],
    True,
  ),
  (
    "unrelated same-clause MUST NOT cannot whitelist a false claim",
    ["This MUST NOT be skipped, and same-runtime samples are independent validators."],
    True,
  ),
  (
    "unrelated same-clause unsupported cannot whitelist cross-model advertising",
    ["The old CLI is unsupported and cross-model execution runs automatically."],
    True,
  ),
  (
    "unrelated same-clause historical cannot whitelist independence advertising",
    ["Historical logs exist while same-runtime samples are independent validators."],
    True,
  ),
  (
    "unrelated same-clause deprecated cannot whitelist ensemble advertising",
    ["A deprecated alias exists and samples use a voting ensemble."],
    True,
  ),
  ("working cross-model advertising is rejected", ["Cross-model execution runs automatically."], True),
  ("working external-model advertising is rejected", ["An external model adapter runs each review."], True),
  ("Multi-AI second opinion advertising is rejected", ["Get a Multi-AI second opinion."], True),
  ("two different AIs advertising is rejected", ["Two different AIs review the result."], True),
  (
    "unrelated historical prose cannot whitelist two-AI advertising",
    ["Historical logs remain available while two different AIs review the result."],
    True,
  ),
  ("crossModelReview config is rejected", ["crossModelReview: enabled"], True),
  ("legacy environment syntax is rejected", ["BUBBLES_ADVERSARIAL_PASSES=3"], True),
  ("legacy command syntax is rejected", ["--passes 3"], True),
  ("legacy passes config syntax is rejected", ["passes: 3"], True),
  ("legacy passes output syntax is rejected", ["passes=3"], True),
  ("unrelated independent scopes are allowed", ["Independent scopes remain valid planning units."], False),
  (
    "same-line prohibition is allowed",
    ["Samples MUST NOT be described as independent validators, voting, consensus, or ensemble."],
    False,
  ),
  ("unsupported cross-model prose is allowed", ["Cross-model execution is unsupported."], False),
  ("deprecated passes syntax is allowed", ["`passes: 3` is deprecated compatibility syntax."], False),
  ("unavailable external adapter prose is allowed", ["No verified external model adapter exists."], False),
  (
    "unsupported Multi-AI capability prose is allowed",
    ["The Multi-AI second opinion capability is unsupported."],
    False,
  ),
  (
    "historical two-AI claim prose is allowed",
    ["The two different AIs claim is historical."],
    False,
  ),
]

classifier_errors = []
for label, lines, should_find in classifier_cases:
  found = bool(scan_lines("fixture.md", lines))
  if found != should_find:
    classifier_errors.append(f"{label}: expected finding={should_find}, got {found}")

if classifier_errors:
  print("active-source terminology classifier self-check failed:", file=sys.stderr)
  for error in classifier_errors:
    print(f"  {error}", file=sys.stderr)
  raise SystemExit(1)

surface_text = {}
missing = []
for relative_path in relative_paths:
  path = root / relative_path
  if not path.is_file():
    missing.append(relative_path)
    continue
  surface_text[relative_path] = path.read_text(encoding="utf-8")

if missing:
  print("active-source terminology scan is missing required surfaces:", file=sys.stderr)
  for relative_path in missing:
    print(f"  {relative_path}", file=sys.stderr)
  raise SystemExit(1)

violations = []
for relative_path, text in surface_text.items():
  violations.extend(scan_lines(relative_path, text.splitlines()))

if violations:
  print("active-source terminology violations:", file=sys.stderr)
  for relative_path, line_number, label, line in violations:
    print(f"  {relative_path}:{line_number}: {label}: {line}", file=sys.stderr)
  raise SystemExit(1)

canonical_text = re.sub(r"\s+", " ", surface_text["agents/bubbles_shared/agent-common.md"])
public_text = re.sub(r"\s+", " ", surface_text["docs/recipes/adversarial-verification.md"])
external_text = re.sub(r"\s+", " ", surface_text["docs/recipes/cross-model-review.md"])

generated_surface_paths = (
  "bubbles/cheatsheet/vocabulary.json",
  "docs/CHEATSHEET.md",
  "docs/its-not-rocket-appliances.html",
)
generated_positive_markers = (
  ("same-runtime-correlated", re.compile(r"same-runtime-correlated")),
  ("samples: 3", re.compile(r"\bsamples\s*:\s*3\b")),
  ("mode/samples/teeth", re.compile(r"\bmode/samples/teeth\b")),
  ("BUBBLES_ADVERSARIAL_SAMPLES", re.compile(r"\bBUBBLES_ADVERSARIAL_SAMPLES\b")),
  (
    "deterministic union/disagreement aggregation",
    re.compile(r"\bdeterministic\s+union/disagreement\s+aggregation\b"),
  ),
)

generated_marker_errors = []
for relative_path in generated_surface_paths:
  text = surface_text[relative_path]
  for label, pattern in generated_positive_markers:
    if not pattern.search(text):
      generated_marker_errors.append(f"{relative_path}: {label}")

if generated_marker_errors:
  print("generated-surface positive contract markers missing:", file=sys.stderr)
  for error in generated_marker_errors:
    print(f"  {error}", file=sys.stderr)
  raise SystemExit(1)

optional_tag_sections = {}
markdown_optional_lines = [
  line
  for line in surface_text["docs/CHEATSHEET.md"].splitlines()
  if re.match(r"^\*\*Optional execution tags:\*\*", line, re.IGNORECASE)
]
if len(markdown_optional_lines) == 1:
  optional_tag_sections["docs/CHEATSHEET.md"] = markdown_optional_lines[0]

html_text = surface_text["docs/its-not-rocket-appliances.html"]
html_optional_headings = list(
  re.finditer(r"<h3[^>]*>[^<]*Optional Execution Tags</h3>", html_text, re.IGNORECASE)
)
if len(html_optional_headings) == 1:
  section_start = html_optional_headings[0].start()
  next_heading = re.search(r"<h3\b", html_text[html_optional_headings[0].end():], re.IGNORECASE)
  section_end = (
    html_optional_headings[0].end() + next_heading.start()
    if next_heading
    else len(html_text)
  )
  optional_tag_sections["docs/its-not-rocket-appliances.html"] = html_text[section_start:section_end]

optional_tag_errors = []
for relative_path in ("docs/CHEATSHEET.md", "docs/its-not-rocket-appliances.html"):
  section = optional_tag_sections.get(relative_path)
  if section is None:
    optional_tag_errors.append(f"{relative_path}: optional execution tags section missing or ambiguous")
  elif re.search(r"\bcrossModelReview\b", section):
    optional_tag_errors.append(f"{relative_path}: optional execution tags still contain crossModelReview")

if optional_tag_errors:
  print("generated-surface optional-tag contract violations:", file=sys.stderr)
  for error in optional_tag_errors:
    print(f"  {error}", file=sys.stderr)
  raise SystemExit(1)

positive_markers = [
  ("samples", canonical_text, re.compile(r"\bsamples\b", re.IGNORECASE)),
  ("same-runtime-correlated", canonical_text, re.compile(r"same-runtime-correlated")),
  ("adversarial-sample.schema.json", canonical_text, re.compile(r"adversarial-sample\.schema\.json")),
  ("adversarial-aggregate.sh", canonical_text, re.compile(r"adversarial-aggregate\.sh")),
  ("agreement-clear", canonical_text + " " + public_text, re.compile(r"\bagreement-clear\b")),
  ("agreement-findings", canonical_text + " " + public_text, re.compile(r"\bagreement-findings\b")),
  ("disagreement", canonical_text + " " + public_text, re.compile(r"\bdisagreement\b")),
  ("aggregation-error", canonical_text + " " + public_text, re.compile(r"\baggregation-error\b")),
  (
    "exact-N actual invocation contract",
    canonical_text + " " + public_text,
    re.compile(
      r"exactly\s+`?N`?\s+separate\s+(?:times|invocations).{0,240}actual\s+invocation"
      r"|N\s+actual\s+(?:redteam\s+)?invocations"
      r"|N\s+requires\s+N\s+actual\s+top-level\s+invocations",
      re.IGNORECASE,
    ),
  ),
  (
    "unsupported external model statement",
    external_text + " " + canonical_text,
    re.compile(
      r"(?:unsupported|unavailable|no\s+verified).{0,180}(?:external|different).{0,120}(?:model|provider|adapter)"
      r"|(?:external|different).{0,180}(?:model|provider|adapter).{0,120}(?:unsupported|unavailable|no\s+verified)",
      re.IGNORECASE,
    ),
  ),
]

missing_markers = [label for label, text, pattern in positive_markers if not pattern.search(text)]
if missing_markers:
  print("active-source positive contract markers missing:", file=sys.stderr)
  for label in missing_markers:
    print(f"  {label}", file=sys.stderr)
  raise SystemExit(1)

print(f"active-source terminology classifier: {len(classifier_cases)} adversarial cases passed")
print(f"active-source terminology scan: {len(surface_text)} current-contract surfaces clean")
print(
  "generated-surface positive assertions: "
  f"{len(generated_surface_paths) * len(generated_positive_markers)} passed"
)
print(f"generated-surface optional-tag assertions: {len(optional_tag_sections)} passed")
for label, _, _ in positive_markers:
  print(f"positive contract marker: {label}")
PY
source_scan_status=$?
source_scan_output="$(cat "$source_scan_output_file")"

if [[ "$source_scan_status" -eq 0 ]]; then
  record_pass
  printf '%s\n' "$source_scan_output"
else
  record_fail "active current-contract source terminology is honest and complete"
  printf '%s\n' "$source_scan_output" | sed 's/^/    /'
fi

echo "adversarial-aggregate-selftest: $pass passed, $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"