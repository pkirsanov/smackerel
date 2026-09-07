#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALIDATOR="$SCRIPT_DIR/yaml-schema-validate.sh"
WORK="$(mktemp -d /tmp/bys.XXXXXXXX)"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

if ! command -v python3 >/dev/null 2>&1; then
  echo "yaml-schema-validate-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import yaml, jsonschema' >/dev/null 2>&1; then
  echo "yaml-schema-validate-selftest: SKIP (PyYAML or jsonschema not installed)"
  exit 0
fi

manifest="$WORK/specs/001-dispatch/scenario-manifest.json"
mkdir -p "$(dirname "$manifest")"
mkdir -p "$WORK/bubbles/schemas"
for schema in \
  workflows.schema.json \
  capability-ledger.schema.json \
  adoption-profiles.schema.json \
  tool-trust-registry.schema.json \
  propagation-policy.schema.json \
  scenario-manifest.schema.json \
  scenario-manifest-v2.schema.json; do
  cp "$SCRIPT_DIR/../schemas/$schema" "$WORK/bubbles/schemas/$schema"
done

run_expect_at() {
  local expected_rc="$1"
  local label="$2"
  local root="$3"
  local actual_rc
  set +e
  bash "$VALIDATOR" --repo-root "$root"
  actual_rc=$?
  set -e
  if [[ "$actual_rc" -ne "$expected_rc" ]]; then
    echo "FAIL: $label (expected $expected_rc, got $actual_rc)"
    exit 1
  fi
  echo "PASS: $label"
}

run_expect() {
  run_expect_at "$1" "$2" "$WORK"
}

restore_schema() {
  local schema_name="$1"
  rm -rf "$WORK/bubbles/schemas/$schema_name"
  cp "$SCRIPT_DIR/../schemas/$schema_name" "$WORK/bubbles/schemas/$schema_name"
}

foreign_root="$WORK/foreign-root"
foreign_manifest="$foreign_root/specs/001-foreign/scenario-manifest.json"
mkdir -p "$(dirname "$foreign_manifest")"
printf '%s\n' '[{"scenarioId":"SCN-001-000","title":"Foreign root","requiredTestType":"unit"}]' >"$foreign_manifest"
run_expect_at 1 "foreign repository without target schemas fails closed" "$foreign_root"

cat >"$WORK/bubbles/schemas/scenario-manifest.schema.json" <<'JSON'
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["schemaVersion", "scenarios"],
  "properties": {
    "schemaVersion": {"type": "integer", "const": 99},
    "scenarios": {"type": "array"}
  }
}
JSON
printf '%s\n' '[{"scenarioId":"SCN-001-000","title":"Target authority","requiredTestType":"unit"}]' >"$manifest"
run_expect 1 "target schema disagreement is not replaced by source checkout schema"
cp "$SCRIPT_DIR/../schemas/scenario-manifest.schema.json" "$WORK/bubbles/schemas/scenario-manifest.schema.json"

cat >"$WORK/bubbles/schemas/scenario-manifest.schema.json" <<'JSON'
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "type": "array"
}
JSON
run_expect 1 "duplicate schema member is rejected before validation"
restore_schema scenario-manifest.schema.json

rm "$WORK/bubbles/schemas/scenario-manifest.schema.json"
ln -s scenario-manifest-v2.schema.json "$WORK/bubbles/schemas/scenario-manifest.schema.json"
run_expect 1 "contained schema symlink is rejected"
restore_schema scenario-manifest.schema.json

rm "$WORK/bubbles/schemas/scenario-manifest.schema.json"
ln -s /dev/null "$WORK/bubbles/schemas/scenario-manifest.schema.json"
run_expect 1 "escaping schema symlink is rejected"
restore_schema scenario-manifest.schema.json

rm "$WORK/bubbles/schemas/scenario-manifest.schema.json"
mkdir "$WORK/bubbles/schemas/scenario-manifest.schema.json"
run_expect 1 "schema directory object is rejected"
restore_schema scenario-manifest.schema.json

rm "$WORK/bubbles/schemas/scenario-manifest.schema.json"
mkfifo "$WORK/bubbles/schemas/scenario-manifest.schema.json"
run_expect 1 "schema FIFO is rejected without blocking"
restore_schema scenario-manifest.schema.json

rm "$WORK/bubbles/schemas/scenario-manifest.schema.json"
python3 -c 'import socket, sys; schema_socket = socket.socket(socket.AF_UNIX); schema_socket.bind(sys.argv[1]); schema_socket.close()' \
  "$WORK/bubbles/schemas/scenario-manifest.schema.json"
run_expect 1 "schema Unix-domain socket is rejected without blocking"
restore_schema scenario-manifest.schema.json

mv "$WORK/bubbles/schemas" "$WORK/bubbles/schemas-real"
ln -s schemas-real "$WORK/bubbles/schemas"
run_expect 1 "intermediate schema symlink is rejected"
rm "$WORK/bubbles/schemas"
mv "$WORK/bubbles/schemas-real" "$WORK/bubbles/schemas"

run_expect 0 "regular repository-contained schema objects remain readable"

cat >"$manifest" <<'JSON'
[
  {
    "scenarioId": "SCN-001-001",
    "title": "Legacy array",
    "requiredTestType": "unit",
    "linkedTests": ["tests/legacy_test.py"]
  }
]
JSON
run_expect 0 "legacy array dispatches to version 1 compatibility schema"

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-001-002",
      "title": "Version 1 object",
      "requiredTestType": "unit",
      "linkedTests": ["planned-not-authored"]
    }
  ]
}
JSON
run_expect 0 "schemaVersion 1 dispatches to compatibility schema"

for alias in scopeRef scope scopeId; do
  printf '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-010","title":"Integral alias","requiredTestType":"unit","%s":1}]}\n' "$alias" >"$manifest"
  run_expect 0 "version 1 compatibility alias $alias accepts exact JSON integer 1"
  for invalid_alias_value in 1.0 true false; do
    printf '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-011","title":"Non-integer alias","requiredTestType":"unit","%s":%s}]}\n' "$alias" "$invalid_alias_value" >"$manifest"
    run_expect 1 "version 1 compatibility alias $alias rejects $invalid_alias_value"
  done
done

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 2,
  "scenarios": [
    {
      "id": "SCN-001-003",
      "title": "Version 2 object",
      "requiredTestType": "e2e-api",
      "linkedTests": [],
      "plannedTests": [
        {"path": "tests/api/proof.py", "title": "API proof", "type": "e2e-api"}
      ]
    }
  ]
}
JSON
run_expect 0 "schemaVersion 2 dispatches to strict producer schema"

duplicate_cases=(
  '{"schemaVersion":1,"schemaVersion":2,"scenarios":[]}'
  '{"schemaVersion":1,"scenarios":[],"scenarios":[]}'
  '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUPLICATE-1","id":"SCN-DUPLICATE-2","title":"Duplicate identity","requiredTestType":"unit"}]}'
  '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUPLICATE-3","title":"Duplicate reference","requiredTestType":"unit","linkedTests":[{"file":"tests/first.py","file":"tests/second.py"}]}]}'
)
duplicate_names=(envelope scenario identity reference)
for duplicate_index in 0 1 2 3; do
  printf '%s\n' "${duplicate_cases[$duplicate_index]}" >"$manifest"
  run_expect 1 "duplicate ${duplicate_names[$duplicate_index]} member fails before schema projection"
done

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 2,
  "generatedAt": "2026-08-31T12:00:00Z",
  "scenarios": [
    {
      "id": "SCN-IMP-126-SCOPE-7-003",
      "title": "Scoped identifier",
      "requiredTestType": "unit"
    }
  ]
}
JSON
run_expect 0 "version 2 accepts scoped scenario identifiers and valid date-times"

for valid_timestamp in \
  '2026-08-31T12:00:00Z' \
  '2026-08-31t12:00:00z' \
  '2026-08-31T12:00:00.123456789Z' \
  '2026-08-31T12:00:00+23:59' \
  '2026-08-31T12:00:00-23:59'; do
  printf '{"schemaVersion":2,"generatedAt":"%s","scenarios":[]}\n' "$valid_timestamp" >"$manifest"
  run_expect 0 "dispatcher accepts canonical RFC3339-subset timestamp $valid_timestamp"
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
  printf '{"schemaVersion":2,"generatedAt":"%s","scenarios":[]}\n' "$invalid_timestamp" >"$manifest"
  run_expect 1 "dispatcher rejects timestamp outside canonical RFC3339 subset $invalid_timestamp"
done

cat >"$manifest" <<'JSON'
{
  "scenarios": []
}
JSON
run_expect 1 "missing schemaVersion fails"

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 3,
  "scenarios": []
}
JSON
run_expect 1 "unknown schemaVersion fails"

for invalid_version in true false 1.0 '"1"' null; do
  printf '{"schemaVersion": %s, "scenarios": []}\n' "$invalid_version" >"$manifest"
  run_expect 1 "non-integer schemaVersion $invalid_version fails"
done

cat >"$manifest" <<'JSON'
"scenario-manifest"
JSON
run_expect 1 "scalar envelope fails"

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 2,
  "scenarios": [
    {
      "scenarioId": "SCN-001-004",
      "title": "Alias is not version 2",
      "requiredTestType": "unit"
    }
  ]
}
JSON
run_expect 1 "version 2 rejects version 1 aliases"

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 2,
  "generatedAt": "not-a-date-time",
  "scenarios": [
    {
      "id": "SCN-001-005",
      "title": "Invalid date-time",
      "requiredTestType": "unit"
    }
  ]
}
JSON
run_expect 1 "version 2 rejects invalid date-times through the dispatcher"

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 2,
  "scenarios": [
    {
      "id": "SCN-invalid-006",
      "title": "Invalid identifier",
      "requiredTestType": "unit"
    }
  ]
}
JSON
run_expect 1 "version 2 rejects lowercase scenario identifiers through the dispatcher"

cat >"$manifest" <<'JSON'
{
  "schemaVersion": 2,
  "scenarios": [
    {
      "id": "SCN-001-007",
      "title": "Unsafe path",
      "requiredTestType": "unit",
      "linkedTests": [{"file": "C:/tests/proof.py", "type": "unit"}]
    }
  ]
}
JSON
run_expect 1 "version 2 rejects unsafe drive paths through the dispatcher"

for whitespace_path in ' tests/proof.py' 'tests/proof.py '; do
  printf '{"schemaVersion":2,"scenarios":[{"id":"SCN-001-008","title":"Non-normalized path","requiredTestType":"unit","linkedTests":[{"file":"%s","type":"unit"}]}]}\n' "$whitespace_path" >"$manifest"
  run_expect 1 "version 2 rejects repository path edge whitespace"
done

echo "yaml-schema-validate-selftest: PASS"
