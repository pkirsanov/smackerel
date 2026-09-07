#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATOR="$SCRIPT_DIR/scenario-manifest-migrate.py"
WORK="$(mktemp -d -t bubbles-scenario-migrate-XXXXXXXX)"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

if ! command -v python3 >/dev/null 2>&1; then
  echo "scenario-manifest-migrate-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  echo "scenario-manifest-migrate-selftest: SKIP (jsonschema not installed)"
  exit 0
fi

duplicate_cases=(
  '{"schemaVersion":1,"schemaVersion":2,"scenarios":[]}'
  '{"schemaVersion":1,"scenarios":[],"scenarios":[]}'
  '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUPLICATE-1","id":"SCN-DUPLICATE-2","title":"Duplicate identity","requiredTestType":"unit"}]}'
  '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUPLICATE-3","title":"Duplicate reference","requiredTestType":"unit","linkedTests":[{"file":"tests/first.py","file":"tests/second.py"}]}]}'
)
duplicate_names=(envelope scenario identity reference)
for duplicate_index in 0 1 2 3; do
  duplicate_file="$WORK/duplicate-${duplicate_names[$duplicate_index]}.json"
  printf '%s\n' "${duplicate_cases[$duplicate_index]}" >"$duplicate_file"
  cp "$duplicate_file" "$duplicate_file.before"
  set +e
  duplicate_output="$(python3 "$MIGRATOR" --write "$duplicate_file" 2>&1)"
  duplicate_rc=$?
  set -e
  [[ "$duplicate_rc" -eq 2 ]] || { echo "FAIL: duplicate ${duplicate_names[$duplicate_index]} member must refuse"; exit 1; }
  [[ "$duplicate_output" == *"duplicate JSON member"* ]] || { echo "FAIL: duplicate ${duplicate_names[$duplicate_index]} refusal is not explicit: $duplicate_output"; exit 1; }
  cmp "$duplicate_file" "$duplicate_file.before"
done
echo "PASS: duplicate envelope, scenario, identity, and reference members refuse before migration without mutation"

cat >"$WORK/identity-whitespace.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"scenarioId":"  SCN-100-018  ","title":"Sole padded identity alias","requiredTestType":"unit","linkedTests":[]}]}
JSON
python3 "$MIGRATOR" --write "$WORK/identity-whitespace.json"
python3 - "$WORK/identity-whitespace.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    scenario = json.load(handle)["scenarios"][0]
assert scenario["id"] == "SCN-100-018"
assert "scenarioId" not in scenario
print("PASS: sole whitespace-bearing scenarioId alias persists as canonical v2 identity")
PY

cat >"$WORK/identity-equivalent.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":" SCN-100-019 ","scenarioId":"SCN-100-019","title":"Equivalent identity aliases","requiredTestType":"unit","linkedTests":[]}]}
JSON
python3 "$MIGRATOR" --write "$WORK/identity-equivalent.json"
python3 - "$WORK/identity-equivalent.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    scenario = json.load(handle)["scenarios"][0]
assert scenario["id"] == "SCN-100-019"
assert "scenarioId" not in scenario
print("PASS: equivalent padded identity aliases agree and emit one canonical id")
PY

cat >"$WORK/identity-conflict.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":" SCN-100-020 ","scenarioId":"SCN-100-021 ","title":"Conflicting normalized identities","requiredTestType":"unit","linkedTests":[]}]}
JSON
cp "$WORK/identity-conflict.json" "$WORK/identity-conflict.before"
set +e
identity_conflict_output="$(python3 "$MIGRATOR" --write "$WORK/identity-conflict.json" 2>&1)"
identity_conflict_rc=$?
set -e
[[ "$identity_conflict_rc" -eq 2 ]] || { echo "FAIL: conflicting post-normalization identity aliases must refuse"; exit 1; }
[[ "$identity_conflict_output" == 'scenario-manifest-migrate: REFUSED scenario[0] has conflicting id, scenarioId values' ]] || { echo "FAIL: normalized identity conflict refusal differs: $identity_conflict_output"; exit 1; }
cmp "$WORK/identity-conflict.json" "$WORK/identity-conflict.before"
echo "PASS: conflicting post-normalization identity aliases refuse without mutation"

invalid_identity_values=('null' 'true' '7' '""' '"   "' '"SCN-invalid-022"' '"SCN-100-022\u0000"')
for invalid_identity in "${invalid_identity_values[@]}"; do
  identity_file="$WORK/identity-invalid-${invalid_identity//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion":1,"scenarios":[{"id":"SCN-100-022","scenarioId":%s,"title":"Invalid identity alias","requiredTestType":"unit","linkedTests":[]}]}' "$invalid_identity" >"$identity_file"
  cp "$identity_file" "$identity_file.before"
  set +e
  python3 "$MIGRATOR" --write "$identity_file"
  identity_rc=$?
  set -e
  [[ "$identity_rc" -eq 2 ]] || { echo "FAIL: present invalid scenarioId alias $invalid_identity must refuse"; exit 1; }
  cmp "$identity_file" "$identity_file.before"
done
echo "PASS: every present empty, non-string, malformed, or control-bearing identity alias fails loud"

cat >"$WORK/v1.json" <<'JSON'
{
  "version": 1,
  "featureDir": "specs/042-catalog-assistant",
  "generatedAt": "2026-08-31T12:00:00Z",
  "scenarios": [
    {
      "scenarioId": "SCN-042-001",
      "scope": "01-search",
      "title": "Guest opens search",
      "requiredTestType": "e2e-ui",
      "scopeId": "01-search",
      "linkedTests": [
        "tests/e2e/authored.spec.ts",
        "tests/e2e/fragment.spec.ts#named proof",
        "tests/e2e/multiple.spec.ts#suite#case",
        {"path": "tests/e2e/named.spec.ts", "name": "named proof"},
        {"file": "tests/e2e/titled.spec.ts", "title": "title alias proof"},
        {"path": "tests/e2e/planned.spec.ts", "title": "Planned proof", "type": "e2e-ui", "testState": "planned-not-authored"},
        {"file": "tests/e2e/legacy-planned.spec.ts", "title": "Legacy planned proof", "type": "e2e-ui", "testState": "planned-not-authored"},
        {"file": " tests/e2e/agreed-planned.spec.ts ", "path": "tests/e2e/agreed-planned.spec.ts", "title": "Agreed planned proof", "type": "e2e-ui", "testState": "planned-not-authored"}
      ]
    }
  ]
}
JSON

set +e
python3 "$MIGRATOR" --check "$WORK/v1.json"
check_rc=$?
set -e
[[ "$check_rc" -eq 1 ]] || { echo "FAIL: check mode must report migration required"; exit 1; }
echo "PASS: check mode reports a migratable version 1 manifest"

python3 "$MIGRATOR" --write "$WORK/v1.json"
python3 "$MIGRATOR" --check "$WORK/v1.json"
cp "$WORK/v1.json" "$WORK/v2-before.json"
python3 "$MIGRATOR" --write "$WORK/v1.json"
cmp "$WORK/v1.json" "$WORK/v2-before.json"
echo "PASS: write mode is idempotent and byte-preserving for version 2"

python3 - "$WORK/v1.json" <<'PY'
import os
import stat
import sys
mode = stat.S_IMODE(os.stat(sys.argv[1]).st_mode)
assert mode == 0o644, oct(mode)
print("PASS: migration preserves source permissions")
PY

python3 - "$WORK/v1.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    data = json.load(handle)
scenario = data["scenarios"][0]
assert data["schemaVersion"] == 2
assert data["spec"] == "specs/042-catalog-assistant"
assert scenario["id"] == "SCN-042-001"
assert scenario["scopeRef"] == "01-search"
assert scenario["linkedTests"] == [
    {"file": "tests/e2e/authored.spec.ts", "type": "e2e-ui"},
  {"file": "tests/e2e/fragment.spec.ts", "type": "e2e-ui", "testId": "named proof"},
  {"file": "tests/e2e/multiple.spec.ts", "type": "e2e-ui", "testId": "suite#case"},
    {"file": "tests/e2e/named.spec.ts", "type": "e2e-ui", "testId": "named proof"},
  {"file": "tests/e2e/titled.spec.ts", "type": "e2e-ui", "testId": "title alias proof"},
]
assert scenario["plannedTests"] == [
    {"path": "tests/e2e/planned.spec.ts", "title": "Planned proof", "type": "e2e-ui"},
    {"path": "tests/e2e/legacy-planned.spec.ts", "title": "Legacy planned proof", "type": "e2e-ui"},
    {"path": "tests/e2e/agreed-planned.spec.ts", "title": "Agreed planned proof", "type": "e2e-ui"},
]
print("PASS: aliases and legacy authored/planned references migrate without invented values")
PY

cat >"$WORK/planned-path-conflict.json" <<'JSON'
[
  {
    "id": "SCN-100-023",
    "title": "Conflicting planned path aliases",
    "requiredTestType": "unit",
    "linkedTests": [
      {
        "file": "tests/planned-first.py",
        "path": "tests/planned-second.py",
        "title": "Conflicting planned proof",
        "type": "unit",
        "testState": "planned-not-authored"
      }
    ]
  }
]
JSON
cp "$WORK/planned-path-conflict.json" "$WORK/planned-path-conflict.before"
set +e
planned_conflict_output="$(python3 "$MIGRATOR" --write "$WORK/planned-path-conflict.json" 2>&1)"
planned_conflict_rc=$?
set -e
[[ "$planned_conflict_rc" -eq 2 ]] || { echo "FAIL: conflicting planned file/path aliases must refuse"; exit 1; }
[[ "$planned_conflict_output" == 'scenario-manifest-migrate: REFUSED scenario[0].linkedTests[0] has conflicting path, file values' ]] || { echo "FAIL: planned alias conflict refusal differs: $planned_conflict_output"; exit 1; }
cmp "$WORK/planned-path-conflict.json" "$WORK/planned-path-conflict.before"
echo "PASS: conflicting legacy planned file/path aliases refuse without mutation"

cat >"$WORK/linked-contracts.json" <<'JSON'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-100-008",
      "scopeRef": "SCOPE-8",
      "scopeId": "SCOPE-8",
      "title": "Compatibility aliases agree",
      "requiredTestType": "unit",
      "linkedTests": ["tests/contracts_test.py#contract#nested"],
      "linkedTestContracts": ["tests/contracts_test.py#contract#nested"]
    }
  ]
}
JSON
python3 "$MIGRATOR" --write "$WORK/linked-contracts.json"
python3 - "$WORK/linked-contracts.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    scenario = json.load(handle)["scenarios"][0]
assert scenario["scopeRef"] == "SCOPE-8"
assert scenario["linkedTests"] == [
    {"file": "tests/contracts_test.py", "type": "unit", "testId": "contract#nested"}
]
assert "scopeId" not in scenario
assert "linkedTestContracts" not in scenario
print("PASS: identical scopeId and linkedTestContracts aliases canonicalize losslessly")
PY

cat >"$WORK/whitespace-aliases.json" <<'JSON'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-100-011",
      "scopeRef": " SCOPE-11 ",
      "scope": "SCOPE-11",
      "scopeId": "  SCOPE-11  ",
      "title": "Whitespace aliases agree",
      "requiredTestType": "unit",
      "linkedTests": [
        {"file": " tests/whitespace_test.py ", "path": "tests/whitespace_test.py", "title": " proof name ", "testId": "proof name", "name": "  proof name  "},
        " tests/string_test.py # string proof "
      ],
      "linkedTestContracts": [
        {"file": "tests/whitespace_test.py", "path": " tests/whitespace_test.py ", "title": "proof name", "testId": " proof name ", "name": "proof name"},
        "tests/string_test.py#string proof"
      ],
      "plannedTests": [
        {"path": " tests/future_test.py ", "title": " Future proof ", "type": "unit"}
      ]
    }
  ]
}
JSON
python3 "$MIGRATOR" --write "$WORK/whitespace-aliases.json"
python3 - "$WORK/whitespace-aliases.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    scenario = json.load(handle)["scenarios"][0]
assert scenario["scopeRef"] == "SCOPE-11"
assert scenario["linkedTests"] == [
    {"file": "tests/whitespace_test.py", "type": "unit", "testId": "proof name"},
    {"file": "tests/string_test.py", "type": "unit", "testId": "string proof"},
]
assert scenario["plannedTests"] == [
    {"path": "tests/future_test.py", "title": "Future proof", "type": "unit"}
]
print("PASS: v1 aliases compare and persist normalized semantic values")
PY

cat >"$WORK/integer-scope.json" <<'JSON'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-100-013",
      "scopeRef": 7,
      "scope": " 7 ",
      "scopeId": 7,
      "title": "Integer scope aliases",
      "requiredTestType": "unit",
      "linkedTests": []
    }
  ]
}
JSON
python3 "$MIGRATOR" --write "$WORK/integer-scope.json"
python3 - "$WORK/integer-scope.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    scenario = json.load(handle)["scenarios"][0]
assert scenario["scopeRef"] == "7"
assert "scope" not in scenario and "scopeId" not in scenario
print("PASS: positive integer and matching string scope aliases migrate to one decimal string")
PY

for invalid_scope in true false 0 -1 1.0 1.5 null '[]' '{}'; do
  scope_file="$WORK/scope-invalid-${invalid_scope//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion":1,"scenarios":[{"id":"SCN-100-014","scopeRef":%s,"title":"Invalid scope","requiredTestType":"unit","linkedTests":[]}]}' "$invalid_scope" >"$scope_file"
  cp "$scope_file" "$scope_file.before"
  set +e
  scope_output="$(python3 "$MIGRATOR" --write "$scope_file" 2>&1)"
  scope_rc=$?
  set -e
  [[ "$scope_rc" -eq 2 ]] || { echo "FAIL: invalid scopeRef $invalid_scope must refuse"; exit 1; }
  [[ "$scope_output" == 'scenario-manifest-migrate: REFUSED scenario[0] alias scopeRef must be a nonblank string or positive integer' ]] || { echo "FAIL: invalid scopeRef refusal bytes differ: $scope_output"; exit 1; }
  cmp "$scope_file" "$scope_file.before"
done
echo "PASS: unsupported v1 scopeRef values refuse exactly without mutation"

for escaped_scope in '""' '"   "' '"SCOPE\u0000ONE"' '"SCOPE\u000aONE"' '"SCOPE\u007fONE"'; do
  scope_file="$WORK/scope-string-${escaped_scope//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion":1,"scenarios":[{"id":"SCN-100-015","scopeRef":%s,"title":"Invalid scope string","requiredTestType":"unit","linkedTests":[]}]}' "$escaped_scope" >"$scope_file"
  cp "$scope_file" "$scope_file.before"
  set +e
  python3 "$MIGRATOR" --write "$scope_file"
  scope_rc=$?
  set -e
  [[ "$scope_rc" -eq 2 ]] || { echo "FAIL: invalid scopeRef string $escaped_scope must refuse"; exit 1; }
  cmp "$scope_file" "$scope_file.before"
done
echo "PASS: blank and control-bearing v1 scopeRef strings refuse without mutation"

cat >"$WORK/padded-scope-conflict.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":"SCN-100-016","scopeRef":"07","scope":7,"title":"Padded conflict","requiredTestType":"unit","linkedTests":[]}]}
JSON
cp "$WORK/padded-scope-conflict.json" "$WORK/padded-scope-conflict.before"
set +e
padded_output="$(python3 "$MIGRATOR" --write "$WORK/padded-scope-conflict.json" 2>&1)"
padded_rc=$?
set -e
[[ "$padded_rc" -eq 2 ]] || { echo "FAIL: padded numeric string and integer must conflict"; exit 1; }
[[ "$padded_output" == 'scenario-manifest-migrate: REFUSED scenario[0] has conflicting scopeRef, scope values' ]] || { echo "FAIL: padded conflict refusal bytes differ: $padded_output"; exit 1; }
cmp "$WORK/padded-scope-conflict.json" "$WORK/padded-scope-conflict.before"
echo "PASS: semantic scope comparison preserves significant numeric-string padding"

cat >"$WORK/v2-integer-scope.json" <<'JSON'
{"schemaVersion":2,"scenarios":[{"id":"SCN-100-017","scopeRef":7,"title":"Strict scope","requiredTestType":"unit","linkedTests":[]}]}
JSON
cp "$WORK/v2-integer-scope.json" "$WORK/v2-integer-scope.before"
set +e
v2_scope_output="$(python3 "$MIGRATOR" --write "$WORK/v2-integer-scope.json" 2>&1)"
v2_scope_rc=$?
set -e
[[ "$v2_scope_rc" -eq 2 ]] || { echo "FAIL: strict v2 integer scopeRef must refuse"; exit 1; }
[[ "$v2_scope_output" == *'version 2 validation failed at scenarios/0/scopeRef:'* ]] || { echo "FAIL: strict v2 scope refusal differs: $v2_scope_output"; exit 1; }
cmp "$WORK/v2-integer-scope.json" "$WORK/v2-integer-scope.before"
echo "PASS: strict version 2 integer scopeRef refuses without rewrite"

for fragment_case in 'tests/bare_test.py' 'tests/blank_test.py#' '#missing-path'; do
  fragment_file="$WORK/fragment-${fragment_case//[^A-Za-z0-9]/_}.json"
  printf '[{"id":"SCN-100-009","title":"Fragment case","requiredTestType":"unit","linkedTests":["%s"]}]\n' "$fragment_case" >"$fragment_file"
  cp "$fragment_file" "$fragment_file.before"
  set +e
  python3 "$MIGRATOR" --write "$fragment_file"
  fragment_rc=$?
  set -e
  if [[ "$fragment_case" == "tests/bare_test.py" ]]; then
    [[ "$fragment_rc" -eq 0 ]] || { echo "FAIL: bare path must migrate"; exit 1; }
  else
    [[ "$fragment_rc" -eq 2 ]] || { echo "FAIL: ambiguous fragment $fragment_case must refuse"; exit 1; }
    cmp "$fragment_file" "$fragment_file.before"
  fi
done
echo "PASS: bare paths migrate while blank fragments and paths refuse without mutation"

for conflict_kind in scope contracts normalized-reference; do
  conflict_file="$WORK/conflict-$conflict_kind.json"
  if [[ "$conflict_kind" == "scope" ]]; then
    conflict_fields='"scopeRef":"SCOPE-A","scopeId":"SCOPE-B","linkedTests":[]'
  elif [[ "$conflict_kind" == "contracts" ]]; then
    conflict_fields='"linkedTests":["tests/a.py"],"linkedTestContracts":["tests/b.py"]'
  else
    conflict_fields='"linkedTests":[{"file":" tests/a.py ","path":"tests/b.py"}]'
  fi
  printf '[{"id":"SCN-100-010","title":"Alias conflict","requiredTestType":"unit",%s}]\n' "$conflict_fields" >"$conflict_file"
  cp "$conflict_file" "$conflict_file.before"
  set +e
  python3 "$MIGRATOR" --write "$conflict_file"
  conflict_rc=$?
  set -e
  [[ "$conflict_rc" -eq 2 ]] || { echo "FAIL: conflicting $conflict_kind aliases must refuse"; exit 1; }
  cmp "$conflict_file" "$conflict_file.before"
done
echo "PASS: conflicting compatibility aliases refuse without mutation"

cat >"$WORK/array.json" <<'JSON'
[
  {
    "id": "SCN-100-001",
    "title": "Unicode path remains intact",
    "requiredTestType": "unit",
    "linkedTests": ["tests/日本語/proof_test.py"]
  }
]
JSON
python3 "$MIGRATOR" --write "$WORK/array.json"
python3 "$MIGRATOR" --check "$WORK/array.json"
echo "PASS: legacy array migrates and validates"

cat >"$WORK/sentinel.json" <<'JSON'
[
  {
    "id": "SCN-100-002",
    "title": "Sentinel refuses",
    "requiredTestType": "unit",
    "linkedTests": ["__FUTURE_TEST__"]
  }
]
JSON
cp "$WORK/sentinel.json" "$WORK/sentinel-before.json"
set +e
python3 "$MIGRATOR" --write "$WORK/sentinel.json"
sentinel_rc=$?
set -e
[[ "$sentinel_rc" -eq 2 ]] || { echo "FAIL: sentinel must refuse"; exit 1; }
cmp "$WORK/sentinel.json" "$WORK/sentinel-before.json"
echo "PASS: sentinel refusal leaves source bytes unchanged"

cat >"$WORK/incomplete-planned.json" <<'JSON'
[
  {
    "id": "SCN-100-003",
    "title": "Incomplete plan refuses",
    "requiredTestType": "unit",
    "linkedTests": [
      {"path": "tests/future.py", "type": "unit", "testState": "planned-not-authored"}
    ]
  }
]
JSON
cp "$WORK/incomplete-planned.json" "$WORK/incomplete-before.json"
set +e
python3 "$MIGRATOR" --write "$WORK/incomplete-planned.json"
incomplete_rc=$?
set -e
[[ "$incomplete_rc" -eq 2 ]] || { echo "FAIL: incomplete planned reference must refuse"; exit 1; }
cmp "$WORK/incomplete-planned.json" "$WORK/incomplete-before.json"
echo "PASS: missing planned title refuses without mutation"

cat >"$WORK/extension.json" <<'JSON'
[
  {
    "id": "SCN-100-004",
    "title": "Unknown extension refuses",
    "requiredTestType": "unit",
    "vendorExtension": true
  }
]
JSON
cp "$WORK/extension.json" "$WORK/extension-before.json"
set +e
python3 "$MIGRATOR" --write "$WORK/extension.json"
extension_rc=$?
set -e
[[ "$extension_rc" -eq 2 ]] || { echo "FAIL: unpreservable extension must refuse"; exit 1; }
cmp "$WORK/extension.json" "$WORK/extension-before.json"
echo "PASS: unpreservable extension refuses rather than dropping data"

for invalid_version in true false 1.0 '"1"' null 3; do
  version_file="$WORK/version-${invalid_version//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion": %s, "scenarios": []}\n' "$invalid_version" >"$version_file"
  cp "$version_file" "$version_file.before"
  set +e
  python3 "$MIGRATOR" --write "$version_file"
  version_rc=$?
  set -e
  [[ "$version_rc" -eq 2 ]] || { echo "FAIL: invalid version $invalid_version must refuse"; exit 1; }
  cmp "$version_file" "$version_file.before"
  echo "PASS: invalid version $invalid_version refuses without mutation"
done

cat >"$WORK/invalid-contract.json" <<'JSON'
{
  "schemaVersion": 1,
  "generatedAt": "not-a-date-time",
  "scenarios": [
    {
      "id": "SCN-invalid-001",
      "title": "Invalid contract",
      "requiredTestType": "unit"
    }
  ]
}
JSON
cp "$WORK/invalid-contract.json" "$WORK/invalid-contract.before"
set +e
python3 "$MIGRATOR" --write "$WORK/invalid-contract.json"
invalid_contract_rc=$?
set -e
[[ "$invalid_contract_rc" -eq 2 ]] || { echo "FAIL: invalid ID/date-time must refuse"; exit 1; }
cmp "$WORK/invalid-contract.json" "$WORK/invalid-contract.before"
echo "PASS: invalid identifier and date-time refuse without mutation"

for valid_timestamp in \
  '2026-08-31T12:00:00Z' \
  '2026-08-31t12:00:00z' \
  '2026-08-31T12:00:00.123456789Z' \
  '2026-08-31T12:00:00+23:59' \
  '2026-08-31T12:00:00-23:59'; do
  timestamp_file="$WORK/timestamp-valid-${valid_timestamp//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion":1,"generatedAt":"%s","scenarios":[]}\n' "$valid_timestamp" >"$timestamp_file"
  python3 "$MIGRATOR" --write "$timestamp_file"
done
for invalid_timestamp in \
  '20260831T120000Z' \
  '2026-08-31' \
  '2026-08-31T12:00:00' \
  '2026-08-31T12:00:00+24:00' \
  '2026-08-31T12:00:00-12:60' \
  '2026-02-29T12:00:00Z' \
  '2026-08-31T24:00:00Z' \
  '2026-08-31T12:60:00Z' \
  '1990-12-31T23:59:60Z' \
  '2026-08-31T12:00:61Z'; do
  timestamp_file="$WORK/timestamp-invalid-${invalid_timestamp//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion":1,"generatedAt":"%s","scenarios":[]}\n' "$invalid_timestamp" >"$timestamp_file"
  cp "$timestamp_file" "$timestamp_file.before"
  set +e
  python3 "$MIGRATOR" --write "$timestamp_file"
  timestamp_rc=$?
  set -e
  [[ "$timestamp_rc" -eq 2 ]] || { echo "FAIL: timestamp outside canonical RFC3339 subset must refuse: $invalid_timestamp"; exit 1; }
  cmp "$timestamp_file" "$timestamp_file.before"
done
echo "PASS: migration enforces the canonical leap-second-free RFC3339 subset"

for whitespace_path in ' tests/current.py' 'tests/current.py '; do
  v2_path_file="$WORK/v2-whitespace-${whitespace_path//[^A-Za-z0-9]/_}.json"
  printf '{"schemaVersion":2,"scenarios":[{"id":"SCN-100-012","title":"Strict path","requiredTestType":"unit","linkedTests":[{"file":"%s","type":"unit"}]}]}\n' "$whitespace_path" >"$v2_path_file"
  cp "$v2_path_file" "$v2_path_file.before"
  set +e
  python3 "$MIGRATOR" --write "$v2_path_file"
  v2_path_rc=$?
  set -e
  [[ "$v2_path_rc" -eq 2 ]] || { echo "FAIL: strict v2 edge-whitespace path must refuse"; exit 1; }
  cmp "$v2_path_file" "$v2_path_file.before"
done
echo "PASS: strict version 2 rejects non-normalized repository paths without mutation"

cat >"$WORK/source-target.json" <<'JSON'
{"schemaVersion":1,"scenarios":[]}
JSON
cp "$WORK/source-target.json" "$WORK/source-target.before"
ln -s "$WORK/source-target.json" "$WORK/source-symlink.json"
set +e
source_symlink_output="$(python3 "$MIGRATOR" --write "$WORK/source-symlink.json" 2>&1)"
source_symlink_rc=$?
set -e
[[ "$source_symlink_rc" -eq 2 ]] || { echo "FAIL: symlink source must refuse"; exit 1; }
[[ "$source_symlink_output" == *"without following links"* ]] || { echo "FAIL: symlink source refusal is not explicit: $source_symlink_output"; exit 1; }
cmp "$WORK/source-target.json" "$WORK/source-target.before"
[[ -L "$WORK/source-symlink.json" ]] || { echo "FAIL: source symlink was replaced"; exit 1; }
echo "PASS: source symlink refuses without reading, replacing, or mutating its target"

mkdir "$WORK/source-directory.json"
mkfifo "$WORK/source-fifo.json"
for nonregular_source in "$WORK/source-directory.json" "$WORK/source-fifo.json"; do
  set +e
  nonregular_output="$(python3 "$MIGRATOR" --write "$nonregular_source" 2>&1)"
  nonregular_rc=$?
  set -e
  [[ "$nonregular_rc" -eq 2 ]] || { echo "FAIL: non-regular source must refuse: $nonregular_source"; exit 1; }
  [[ "$nonregular_output" == *"is not a regular file"* ]] || { echo "FAIL: non-regular source refusal is not explicit: $nonregular_output"; exit 1; }
done
[[ -d "$WORK/source-directory.json" ]] || { echo "FAIL: source directory was replaced"; exit 1; }
[[ -p "$WORK/source-fifo.json" ]] || { echo "FAIL: source FIFO was replaced"; exit 1; }
echo "PASS: directory and FIFO sources refuse without blocking or replacement"

mkdir -m 700 "$WORK/runtime"
XDG_RUNTIME_DIR="$WORK/runtime" python3 - "$MIGRATOR" "$WORK/v1.json" <<'PY'
import importlib.util
import os
import stat
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
lock = module.lock_path(module.Path(sys.argv[2]))
assert lock.parent == module.Path(os.environ["XDG_RUNTIME_DIR"]) / "bubbles-scenario-manifest-migrate-locks"
with module.migration_lock(module.Path(sys.argv[2])):
  metadata = lock.lstat()
  assert stat.S_ISREG(metadata.st_mode)
  assert metadata.st_uid == os.getuid()
  assert stat.S_IMODE(metadata.st_mode) == 0o600
print("PASS: trustworthy XDG runtime directory and private regular lock are used")
PY

mkdir -m 700 "$WORK/real-runtime"
ln -s "$WORK/real-runtime" "$WORK/runtime-symlink"
XDG_RUNTIME_DIR="$WORK/runtime-symlink" TMPDIR="$WORK" python3 - "$MIGRATOR" "$WORK/v1.json" <<'PY'
import importlib.util
import os
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
directory = module.lock_directory()
assert directory == module.Path(os.environ["TMPDIR"]) / f"bubbles-scenario-manifest-migrate-{os.getuid()}"
print("PASS: symlink XDG runtime directory is rejected in favor of secure fallback")
PY

mkdir -m 700 "$WORK/directory-symlink-runtime" "$WORK/directory-symlink-target"
ln -s "$WORK/directory-symlink-target" "$WORK/directory-symlink-runtime/bubbles-scenario-manifest-migrate-locks"
set +e
XDG_RUNTIME_DIR="$WORK/directory-symlink-runtime" python3 "$MIGRATOR" --check "$WORK/v1.json"
directory_symlink_rc=$?
set -e
[[ "$directory_symlink_rc" -eq 2 ]] || { echo "FAIL: symlink advisory lock directory must refuse"; exit 1; }
echo "PASS: symlink advisory lock directory refuses"

mkdir -m 700 "$WORK/hostile-temp"
mkdir -m 0770 "$WORK/hostile-temp/bubbles-scenario-manifest-migrate-$(id -u)"
set +e
TMPDIR="$WORK/hostile-temp" XDG_RUNTIME_DIR='' python3 "$MIGRATOR" --check "$WORK/v1.json"
hostile_directory_rc=$?
set -e
[[ "$hostile_directory_rc" -eq 2 ]] || { echo "FAIL: group-accessible fallback lock directory must refuse"; exit 1; }
echo "PASS: precreated fallback lock directory with hostile mode refuses"

mkdir -m 700 "$WORK/symlink-runtime"
XDG_RUNTIME_DIR="$WORK/symlink-runtime" python3 - "$MIGRATOR" "$WORK/v1.json" <<'PY'
import importlib.util
import os
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
lock = module.lock_path(module.Path(sys.argv[2]))
target = module.Path(os.environ["XDG_RUNTIME_DIR"]) / "target"
target.write_text("do not follow", encoding="utf-8")
lock.symlink_to(target)
try:
  with module.migration_lock(module.Path(sys.argv[2])):
    raise AssertionError("symlink lock was accepted")
except module.MigrationRefusal as error:
  assert "cannot open advisory lock" in str(error), error
assert target.read_text(encoding="utf-8") == "do not follow"
print("PASS: symlink advisory lock refuses without touching its target")
PY

mkdir -m 700 "$WORK/mode-runtime"
XDG_RUNTIME_DIR="$WORK/mode-runtime" python3 - "$MIGRATOR" "$WORK/v1.json" <<'PY'
import importlib.util
import os
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
lock = module.lock_path(module.Path(sys.argv[2]))
lock.write_text("", encoding="utf-8")
lock.chmod(0o660)
try:
  with module.migration_lock(module.Path(sys.argv[2])):
    raise AssertionError("unsafe lock mode was accepted")
except module.MigrationRefusal as error:
  assert "permissions are unsafe" in str(error), error
print("PASS: precreated advisory lock with hostile mode refuses")
PY

mkdir -m 700 "$WORK/owner-runtime"
XDG_RUNTIME_DIR="$WORK/owner-runtime" python3 - "$MIGRATOR" "$WORK/v1.json" <<'PY'
import importlib.util
import os
import sys
from unittest import mock

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
real_fstat = module.os.fstat


def hostile_owner(descriptor):
  metadata = real_fstat(descriptor)
  if not module.stat.S_ISREG(metadata.st_mode):
    return metadata
  values = list(metadata)
  values[4] = os.getuid() + 1
  return os.stat_result(values)


with mock.patch.object(module.os, "fstat", side_effect=hostile_owner):
  try:
    with module.migration_lock(module.Path(sys.argv[2])):
      raise AssertionError("foreign-owned lock was accepted")
  except module.MigrationRefusal as error:
    assert "not owned by the current uid" in str(error), error
print("PASS: precreated advisory lock with foreign owner refuses where testable")
PY

XDG_RUNTIME_DIR="$WORK/runtime" python3 - "$MIGRATOR" "$WORK/v1.json" <<'PY'
import importlib.util
import subprocess
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
with module.migration_lock(module.Path(sys.argv[2])):
    result = subprocess.run(
        [sys.executable, sys.argv[1], "--check", sys.argv[2]],
        capture_output=True,
        text=True,
        timeout=10,
    )
assert result.returncode == 2, (result.returncode, result.stdout, result.stderr)
assert "another cooperating migrator holds the advisory lock" in result.stderr
print("PASS: advisory lock contention deterministically refuses a cooperating migrator")
PY

mkdir -p "$WORK/alias-source/inner"
cat >"$WORK/alias-source/inner/manifest.json" <<'JSON'
{"schemaVersion":1,"scenarios":[]}
JSON
ln -s "$WORK/alias-source" "$WORK/alias-source-link"
XDG_RUNTIME_DIR="$WORK/runtime" python3 - "$MIGRATOR" \
  "$WORK/alias-source/inner/manifest.json" \
  "$WORK/alias-source-link/inner/manifest.json" <<'PY'
import importlib.util
import os
import subprocess
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
real_path = module.Path(sys.argv[2])
alias_path = module.Path(sys.argv[3])
assert module.lock_path(real_path) == module.lock_path(alias_path)
with module.migration_lock(real_path):
  result = subprocess.run(
    [sys.executable, sys.argv[1], "--write", os.fspath(alias_path)],
    capture_output=True,
    text=True,
    timeout=10,
  )
assert result.returncode == 2, (result.returncode, result.stdout, result.stderr)
assert "another cooperating migrator holds the advisory lock" in result.stderr
with open(real_path, encoding="utf-8") as handle:
  assert handle.read() == '{"schemaVersion":1,"scenarios":[]}\n'
print("PASS: intermediate directory aliases share one migration lock identity")
PY

cat >"$WORK/race.json" <<'JSON'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-IMP-126-SCOPE-7-009",
      "title": "Concurrent replacement is preserved",
      "requiredTestType": "unit"
    }
  ]
}
JSON
cat >"$WORK/race-replacement.json" <<'JSON'
{"schemaVersion":1,"scenarios":[]}
JSON
set +e
_BUBBLES_SCENARIO_MIGRATE_TEST_REPLACEMENT="$WORK/race-replacement.json" \
  python3 "$MIGRATOR" --write "$WORK/race.json"
race_rc=$?
set -e
[[ "$race_rc" -eq 2 ]] || { echo "FAIL: concurrent replacement must refuse"; exit 1; }
printf '{"schemaVersion":1,"scenarios":[]}\n' >"$WORK/race-expected.json"
cmp "$WORK/race.json" "$WORK/race-expected.json"
echo "PASS: concurrent replacement refuses and preserves the concurrent writer's bytes"

cat >"$WORK/race-symlink.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":"SCN-IMP-126-SCOPE-7-010","title":"Concurrent symlink replacement is preserved","requiredTestType":"unit"}]}
JSON
cat >"$WORK/race-symlink-target.json" <<'JSON'
{"schemaVersion":1,"scenarios":[]}
JSON
cp "$WORK/race-symlink-target.json" "$WORK/race-symlink-target.before"
ln -s "$WORK/race-symlink-target.json" "$WORK/race-symlink-replacement.json"
set +e
_BUBBLES_SCENARIO_MIGRATE_TEST_REPLACEMENT="$WORK/race-symlink-replacement.json" \
  python3 "$MIGRATOR" --write "$WORK/race-symlink.json"
race_symlink_rc=$?
set -e
[[ "$race_symlink_rc" -eq 2 ]] || { echo "FAIL: concurrent symlink replacement must refuse"; exit 1; }
[[ -L "$WORK/race-symlink.json" ]] || { echo "FAIL: concurrent source symlink was overwritten"; exit 1; }
cmp "$WORK/race-symlink-target.json" "$WORK/race-symlink-target.before"
echo "PASS: concurrent symlink replacement refuses without following or overwriting its target"

mkdir -p "$WORK/schema-fixture/bubbles/scripts" "$WORK/schema-fixture/bubbles/schemas"
cp "$SCRIPT_DIR/../schemas/scenario-manifest-v2.schema.json" \
  "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
python3 - "$MIGRATOR" "$WORK/schema-fixture/bubbles/scripts/scenario-manifest-migrate.py" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
validator = module.load_validator(module.Path(sys.argv[2]))
assert validator.is_valid({"schemaVersion": 2, "scenarios": []})
print("PASS: descriptor-rooted schema loading accepts an ordinary regular schema")
PY

cat >"$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json" <<'JSON'
{"$schema":"http://json-schema.org/draft-07/schema#","$schema":"http://json-schema.org/draft-07/schema#","type":"object"}
JSON
python3 - "$MIGRATOR" "$WORK/schema-fixture/bubbles/scripts/scenario-manifest-migrate.py" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
try:
  module.load_validator(module.Path(sys.argv[2]))
  raise AssertionError("duplicate schema member was accepted")
except module.MigrationRefusal as error:
  assert "duplicate JSON member '$schema'" in str(error), error
print("PASS: duplicate schema members refuse before schema validation")
PY

rm "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
cat >"$WORK/schema-fixture/schema-target.json" <<'JSON'
{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}
JSON
ln -s "$WORK/schema-fixture/schema-target.json" \
  "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
python3 - "$MIGRATOR" "$WORK/schema-fixture/bubbles/scripts/scenario-manifest-migrate.py" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
try:
  module.load_validator(module.Path(sys.argv[2]))
  raise AssertionError("schema symlink was accepted")
except module.MigrationRefusal as error:
  assert "without following links" in str(error), error
print("PASS: schema symlink refuses without following its target")
PY

rm "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
mkfifo "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
python3 - "$MIGRATOR" "$WORK/schema-fixture/bubbles/scripts/scenario-manifest-migrate.py" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
try:
  module.load_validator(module.Path(sys.argv[2]))
  raise AssertionError("schema FIFO was accepted")
except module.MigrationRefusal as error:
  assert "is not a regular file" in str(error), error
print("PASS: non-regular schema object refuses without blocking")
PY

rm "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
cp "$SCRIPT_DIR/../schemas/scenario-manifest-v2.schema.json" \
  "$WORK/schema-fixture/bubbles/schemas/scenario-manifest-v2.schema.json"
ln -s "$WORK/schema-fixture/bubbles" "$WORK/schema-fixture/bubbles-link"
python3 - "$MIGRATOR" "$WORK/schema-fixture/bubbles-link/scripts/scenario-manifest-migrate.py" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
try:
  module.load_validator(module.Path(sys.argv[2]))
  raise AssertionError("intermediate schema path symlink was accepted")
except module.MigrationRefusal as error:
  assert "without following links" in str(error), error
print("PASS: intermediate schema path substitution refuses under descriptor-rooted traversal")
PY

cat >"$WORK/exchange-success.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":"SCN-ATOMIC-EXCHANGE-1","title":"Atomic exchange succeeds","requiredTestType":"unit"}]}
JSON
python3 "$MIGRATOR" --write "$WORK/exchange-success.json"
python3 "$MIGRATOR" --check "$WORK/exchange-success.json"
echo "PASS: ordinary write migration succeeds through the atomic exchange path"

cat >"$WORK/final-race.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":"SCN-FINAL-RACE-1","title":"Final replacement race is preserved","requiredTestType":"unit"}]}
JSON
cat >"$WORK/final-race-replacement.json" <<'JSON'
{"schemaVersion":1,"scenarios":[{"id":"SCN-CONCURRENT-WRITER-1","title":"Concurrent writer bytes","requiredTestType":"unit"}]}
JSON
cp "$WORK/final-race-replacement.json" "$WORK/final-race-expected.json"
set +e
_BUBBLES_SCENARIO_MIGRATE_TEST_FINAL_REPLACEMENT="$WORK/final-race-replacement.json" \
  python3 "$MIGRATOR" --write "$WORK/final-race.json"
final_race_rc=$?
set -e
[[ "$final_race_rc" -eq 2 ]] || { echo "FAIL: final replacement interval race must refuse"; exit 1; }
cmp "$WORK/final-race.json" "$WORK/final-race-expected.json"
echo "PASS: final validation-to-replacement race refuses without overwriting concurrent new bytes"

python3 - "$MIGRATOR" "$WORK" <<'PY'
import importlib.util
import os
import sys
from unittest import mock

spec = importlib.util.spec_from_file_location("scenario_manifest_migrate", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
directory_descriptor = os.open(sys.argv[2], os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
try:
  with mock.patch.object(module.sys, "platform", "unsupported"):
    try:
      module.atomic_exchange(directory_descriptor, "exchange-success.json", "v1.json")
      raise AssertionError("missing atomic exchange primitive was accepted")
    except module.MigrationRefusal as error:
      assert "write mode is refused" in str(error), error
finally:
  os.close(directory_descriptor)
print("PASS: unavailable atomic exchange primitive fails closed instead of weakening write safety")
PY

echo "scenario-manifest-migrate-selftest: PASS"
