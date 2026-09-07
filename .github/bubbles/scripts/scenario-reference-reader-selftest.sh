#!/usr/bin/env bash
set -uo pipefail
export PYTHONDONTWRITEBYTECODE=1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
READER="$SCRIPT_DIR/scenario-reference-reader.py"
SCHEMA="$SCRIPT_DIR/../schemas/scenario-manifest-v2.schema.json"
WORK=""
WORK_OWNED=false
WORK_IDENTITY=""
path_identity() {
  python3 - "$1" <<'PY'
import os, sys
status = os.stat(sys.argv[1], follow_symlinks=False)
print(f"{status.st_dev}:{status.st_ino}")
PY
}
if [[ -n "${SCENARIO_REFERENCE_READER_SIGNAL_PROBE_WORK:-}" ]]; then
  probe_parent="$SCENARIO_REFERENCE_READER_SIGNAL_PROBE_WORK"
  if [[ ! -d "$probe_parent" || -L "$probe_parent" ]]; then
    printf 'scenario-reference-reader-selftest: signal probe parent must be an existing non-symlink directory\n' >&2
    exit 2
  fi
  WORK="$(mktemp -d "$probe_parent/scenario-reference-reader-selftest.XXXXXX")" || exit 2
else
  WORK="$(mktemp -d)" || exit 2
fi
WORK_OWNED=true
WORK_IDENTITY="$(path_identity "$WORK")" || exit 2
cleanup() {
  if [[ "$WORK_OWNED" == true && -n "$WORK" ]]; then
    if [[ ! -d "$WORK" || -L "$WORK" ]]; then
      printf 'scenario-reference-reader-selftest: refusing cleanup because owned workspace path is absent or is not a non-symlink directory: %s\n' "$WORK" >&2
      WORK_OWNED=false
      return 1
    fi
    local current_identity
    current_identity="$(path_identity "$WORK")" || {
      printf 'scenario-reference-reader-selftest: refusing cleanup because owned workspace identity cannot be read: %s\n' "$WORK" >&2
      WORK_OWNED=false
      return 1
    }
    if [[ "$current_identity" != "$WORK_IDENTITY" ]]; then
      printf 'scenario-reference-reader-selftest: refusing cleanup because owned workspace identity changed: %s\n' "$WORK" >&2
      WORK_OWNED=false
      return 1
    fi
    rm -rf -- "$WORK"
    WORK_OWNED=false
  fi
}
trap cleanup EXIT
trap 'trap - EXIT; cleanup; exit 130' INT
trap 'trap - EXIT; cleanup; exit 143' TERM
if [[ -n "${SCENARIO_REFERENCE_READER_SIGNAL_PROBE_WORK:-}" ]]; then
  mkfifo "$WORK/hold" || exit 2
  exec 9<>"$WORK/hold"
  printf 'READY %s\n' "$WORK"
  read -r _ <&9
  exit 0
fi
mkdir -p "$WORK/repo/tests"
mkdir -p "$WORK/repo/bubbles/schemas"
cp "$SCHEMA" "$WORK/repo/bubbles/schemas/scenario-manifest-v2.schema.json"
printf 'ok\n' > "$WORK/repo/tests/α.spec.ts"

checks=0
failures=0
pass() { checks=$((checks + 1)); printf 'PASS: %s\n' "$1"; }
fail() { checks=$((checks + 1)); failures=$((failures + 1)); printf 'FAIL: %s\n' "$1"; }

run_case() {
  local name="$1" body="$2" expected="$3" needle="$4" output status number
  if [[ "$body" == '{"scenarios"'* ]]; then
    body="{\"schemaVersion\":1,${body#\{}"
  fi
  number=1
  while [[ "$number" -le 27 ]]; do
    body="${body//\"SCN-$number\"/\"SCN-X-$number\"}"
    number=$((number + 1))
  done
  printf '%s\n' "$body" > "$WORK/repo/manifest.json"
  status=0
  output="$(python3 "$READER" "$WORK/repo/manifest.json" --repo-root "$WORK/repo" 2>&1)" || status=$?
  if [[ "$status" -eq "$expected" ]] && printf '%s\n' "$output" | grep -Fq -- "$needle"; then
    pass "$name"
  else
    fail "$name (exit=$status expected=$expected missing=$needle output=$output)"
  fi
}

run_manifest_path_case() {
  local name="$1" manifest_path="$2" expected="$3" needle="$4" output status
  status=0
  output="$(python3 "$READER" "$manifest_path" --repo-root "$WORK/repo" 2>&1)" || status=$?
  if [[ "$status" -eq "$expected" && "$output" == *"$needle"* ]]; then
    pass "$name"
  else
    fail "$name (exit=$status expected=$expected missing=$needle output=$output)"
  fi
}

printf '%s\n' '{"schemaVersion":1,"scenarios":[]}' >"$WORK/repo/regular-manifest.json"
run_manifest_path_case 'regular repository-rooted manifest accepted' \
  "$WORK/repo/regular-manifest.json" 0 '"sourceEnvelope":"schema-version-1"'
ln -s 'regular-manifest.json' "$WORK/repo/contained-manifest.json"
run_manifest_path_case 'contained manifest symlink refused' \
  "$WORK/repo/contained-manifest.json" 2 'cannot parse scenario manifest'
printf '%s\n' '{"schemaVersion":1,"scenarios":[]}' >"$WORK/outside-manifest.json"
ln -s "$WORK/outside-manifest.json" "$WORK/repo/escaping-manifest.json"
run_manifest_path_case 'escaping manifest symlink refused' \
  "$WORK/repo/escaping-manifest.json" 2 'cannot parse scenario manifest'
mkdir -p "$WORK/outside-manifest-directory"
printf '%s\n' '{"schemaVersion":1,"scenarios":[]}' >"$WORK/outside-manifest-directory/manifest.json"
ln -s "$WORK/outside-manifest-directory" "$WORK/repo/substituted-manifest-directory"
run_manifest_path_case 'intermediate manifest symlink substitution refused' \
  "$WORK/repo/substituted-manifest-directory/manifest.json" 2 'cannot open directory component without following links'
mkfifo "$WORK/repo/fifo-manifest.json"
run_manifest_path_case 'FIFO manifest is refused without blocking' \
  "$WORK/repo/fifo-manifest.json" 2 'path is not a regular file'

run_case 'object missing version refused' '{ "scenarios":[]}' 2 'missing required schemaVersion'
run_case 'schemaVersion bool refused' '{"schemaVersion":true,"scenarios":[]}' 2 'schemaVersion must be an int'
run_case 'schemaVersion float refused' '{"schemaVersion":1.0,"scenarios":[]}' 2 'schemaVersion must be an int'
run_case 'schemaVersion string refused' '{"schemaVersion":"1","scenarios":[]}' 2 'schemaVersion must be an int'
run_case 'schemaVersion null refused' '{"schemaVersion":null,"scenarios":[]}' 2 'schemaVersion must be an int'
run_case 'version 2 unknown top-level field refused by reader' '{"schemaVersion":2,"scenarios":[],"extension":true}' 2 'Additional properties are not allowed'
run_case 'version 2 unknown scenario field refused by reader' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-UNKNOWN-1","title":"closed scenario","requiredTestType":"unit","extension":true}]}' 2 'Additional properties are not allowed'
run_case 'version 2 missing scenario title refused by complete schema' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-MISSING-TITLE-1","requiredTestType":"unit"}]}' 2 "'title' is a required property"
run_case 'version 2 missing requiredTestType refused by complete schema' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-MISSING-TYPE-1","title":"missing type"}]}' 2 "'requiredTestType' is a required property"
run_case 'version 2 malformed nested member refused by complete schema' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-NESTED-TYPE-1","title":"nested type","requiredTestType":"unit","obligations":[{"trait":"coverage","requiredProof":7}]}]}' 2 'requiredProof'
run_case 'version 2 unknown nested field refused by complete schema' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-NESTED-CLOSED-1","title":"nested closed","requiredTestType":"unit","obligations":[{"trait":"coverage","requiredProof":"test","extension":true}]}]}' 2 'Additional properties are not allowed'
run_case 'version 2 malformed hash refused by complete schema' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-HASH-1","title":"bad hash","requiredTestType":"unit","gherkinHash":"sha256:ABC"}]}' 2 'does not match'
run_case 'version 2 noncanonical timestamp refused by complete schema' '{"schemaVersion":2,"generatedAt":"2026-09-01 12:00:00Z","scenarios":[{"id":"SCN-V2-TIME-1","title":"bad time","requiredTestType":"unit"}]}' 2 'is not a'
run_case 'duplicate envelope member refused' '{"schemaVersion":1,"schemaVersion":2,"scenarios":[]}' 2 'duplicate JSON member'
run_case 'duplicate scenarios member refused' '{"schemaVersion":1,"scenarios":[],"scenarios":[]}' 2 'duplicate JSON member'
run_case 'duplicate scenario identity member refused' '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUPLICATE-1","id":"SCN-DUPLICATE-2","linkedTests":[]}]}' 2 'duplicate JSON member'
run_case 'duplicate reference member refused' '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUPLICATE-3","linkedTests":[{"file":"tests/α.spec.ts","file":"tests/other.spec.ts"}]}]}' 2 'duplicate JSON member'

run_case 'legacy top-level authored string' '[{"id":"SCN-LEGACY-1","linkedTests":["tests/α.spec.ts"]}]' 0 '"kind":"authored"'
run_case 'canonical planned missing file accepted' '{"schemaVersion":2,"scenarios":[{"id":"SCN-2","title":"planned scenario","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}]}]}' 0 '"exists":false'
run_case 'planned scalar refused with index' '{"schemaVersion":1,"scenarios":[{"id":"SCN-PLAN-3","plannedTests":["tests/future.spec.ts"]}]}' 2 'SCN-PLAN-3 plannedTests[0]'
run_case 'pathless object refused' '{"scenarios":[{"id":"SCN-4","linkedTests":[{"title":"missing path"}]}]}' 2 'SCN-X-4 linkedTests[0]'
run_case 'missing planned title refused' '{"scenarios":[{"id":"SCN-5","plannedTests":[{"path":"tests/future.spec.ts","type":"e2e-ui"}]}]}' 2 'requires nonblank title'
run_case 'invalid planned type refused' '{"scenarios":[{"id":"SCN-6","plannedTests":[{"path":"tests/future.spec.ts","title":"future","type":"browser"}]}]}' 2 "type 'browser' is not canonical"
run_case 'conflicting path aliases refused' '{"scenarios":[{"id":"SCN-7","linkedTests":[{"file":"tests/α.spec.ts","path":"tests/other.spec.ts"}]}]}' 2 'conflicting aliases (file, path)'
run_case 'identical aliases accepted' '{"scenarios":[{"id":"SCN-8","linkedTests":[{"file":"tests/α.spec.ts","path":"tests/α.spec.ts","title":"same","testId":"same","name":"same"}]}]}' 0 '"title":"same"'
run_case 'C0 alias refused' $'{"scenarios":[{"id":"SCN-9","linkedTests":[{"file":"tests/α.spec.ts","title":"bad\\u0001title"}]}]}' 2 'control character'
run_case 'DEL path refused' '{"scenarios":[{"id":"SCN-10","linkedTests":[{"file":"tests/bad\u007f.spec.ts"}]}]}' 2 'control character'
run_case 'scopeRef NUL refused' '{"scenarios":[{"id":"SCN-SCOPE-NUL-1","scopeRef":"SCOPE\u0000ONE","linkedTests":[]}]}' 2 "scope alias 'scopeRef' contains a control character"
run_case 'scope newline refused' '{"scenarios":[{"id":"SCN-SCOPE-NEWLINE-1","scope":"SCOPE\u000aONE","linkedTests":[]}]}' 2 "scope alias 'scope' contains a control character"
run_case 'scopeId tab refused' '{"scenarios":[{"id":"SCN-SCOPE-TAB-1","scopeId":"SCOPE\u0009ONE","linkedTests":[]}]}' 2 "scope alias 'scopeId' contains a control character"
run_case 'scopeRef C0 refused' '{"scenarios":[{"id":"SCN-SCOPE-C0-1","scopeRef":"SCOPE\u0001ONE","linkedTests":[]}]}' 2 "scope alias 'scopeRef' contains a control character"
run_case 'scopeId DEL refused' '{"scenarios":[{"id":"SCN-SCOPE-DEL-1","scopeId":"SCOPE\u007fONE","linkedTests":[]}]}' 2 "scope alias 'scopeId' contains a control character"
run_case 'identical string scope aliases accepted' '{"scenarios":[{"id":"SCN-SCOPE-ALIASES-1","scopeRef":"SCOPE-1","scope":" SCOPE-1 ","scopeId":"SCOPE-1","linkedTests":[]}]}' 0 '"scopeRef":"SCOPE-1"'
run_case 'identical integer scope aliases accepted as string projection' '{"scenarios":[{"id":"SCN-SCOPE-ALIASES-2","scopeRef":7,"scope":7,"scopeId":7,"linkedTests":[]}]}' 0 '"scopeRef":"7"'
run_case 'v1 positive integer scopeRef normalizes to string' '{"schemaVersion":1,"scenarios":[{"id":"SCN-SCOPE-INTEGER-1","scopeRef":7,"linkedTests":[]}]}' 0 '"scopeRef":"7"'
run_case 'v1 mixed string and integer aliases agree after normalization' '{"schemaVersion":1,"scenarios":[{"id":"SCN-SCOPE-MIXED-1","scopeRef":" 7 ","scope":7,"scopeId":"7","linkedTests":[]}]}' 0 '"scopeRef":"7"'
run_case 'v1 padded numeric string remains semantically distinct' '{"schemaVersion":1,"scenarios":[{"id":"SCN-SCOPE-PADDED-1","scopeRef":"07","scope":7,"linkedTests":[]}]}' 2 'conflicting scope aliases (scopeRef, scope)'
for invalid_scope in true false 0 -1 1.0 1.5 null; do
  run_case "v1 scopeRef rejects $invalid_scope" "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-SCOPE-TYPE-1\",\"scopeRef\":$invalid_scope,\"linkedTests\":[]}]}" 2 "scope alias 'scopeRef' must be a nonblank string or positive integer"
done
run_case 'v1 blank scopeRef refused' '{"schemaVersion":1,"scenarios":[{"id":"SCN-SCOPE-BLANK-1","scopeRef":"  ","linkedTests":[]}]}' 2 "scope alias 'scopeRef' is blank"
run_case 'v2 omitted scopeRef accepted' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-1","title":"optional scope","requiredTestType":"unit","linkedTests":[]}]}' 0 '"scopeRef":null'
run_case 'v2 canonical scopeRef projects unchanged' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-2","scopeRef":"SCOPE-2","title":"canonical scope","requiredTestType":"unit","linkedTests":[]}]}' 0 '"scopeRef":"SCOPE-2"'
run_case 'v2 edge-whitespace scopeRef refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-EDGE-2","scopeRef":" SCOPE-2 ","title":"noncanonical scope","requiredTestType":"unit","linkedTests":[]}]}' 2 'does not match'
run_case 'v2 blank scopeRef refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-3","scopeRef":"  ","title":"blank scope","requiredTestType":"unit","linkedTests":[]}]}' 2 'does not match'
run_case 'v2 control scopeRef refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-4","scopeRef":"SCOPE\u0009FOUR","title":"control scope","requiredTestType":"unit","linkedTests":[]}]}' 2 'does not match'
for invalid_scope in 1 1.0 true false null; do
  run_case "v2 scopeRef rejects $invalid_scope" "{\"schemaVersion\":2,\"scenarios\":[{\"id\":\"SCN-V2-SCOPE-TYPE-5\",\"scopeRef\":$invalid_scope,\"title\":\"typed scope\",\"requiredTestType\":\"unit\",\"linkedTests\":[]}] }" 2 'is not of type'
done
run_case 'v2 legacy scope alias refused even when null' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-ALIAS-6","scope":null,"title":"legacy scope","requiredTestType":"unit","linkedTests":[]}]}' 2 'Additional properties are not allowed'
run_case 'v2 legacy scopeId alias refused even when agreeing' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-SCOPE-ALIAS-7","scopeRef":"SCOPE-7","scopeId":"SCOPE-7","title":"legacy scope id","requiredTestType":"unit","linkedTests":[]}]}' 2 'Additional properties are not allowed'
run_case 'POSIX traversal refused' '{"scenarios":[{"id":"SCN-11","plannedTests":[{"path":"tests/../future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 'lexical traversal'
run_case 'backslash traversal refused' '{"scenarios":[{"id":"SCN-12","plannedTests":[{"path":"tests\\..\\future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 'POSIX separators'
run_case 'drive-relative refused' '{"scenarios":[{"id":"SCN-13","plannedTests":[{"path":"C:future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 'Windows drive/device'
run_case 'drive-absolute refused' '{"scenarios":[{"id":"SCN-14","plannedTests":[{"path":"C:/future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 'Windows drive/device'
run_case 'UNC refused' '{"scenarios":[{"id":"SCN-15","plannedTests":[{"path":"\\\\server\\share\\future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 'POSIX separators'
run_case 'rooted backslash refused' '{"scenarios":[{"id":"SCN-16","plannedTests":[{"path":"\\future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 'POSIX separators'
device_path_json='{"scenarios":[{"id":"SCN-17","plannedTests":[{"path":"\\\\?\\'
device_path_json+='C'
device_path_json+=':\\future.spec.ts","title":"future","type":"e2e-ui"}]}]}'
run_case 'device path refused' "$device_path_json" 2 'POSIX separators'

mkdir -p "$WORK/outside"
printf 'escape\n' > "$WORK/outside/escape.spec.ts"
ln -s "$WORK/outside/escape.spec.ts" "$WORK/repo/tests/escape.spec.ts"
run_case 'authored symlink escape refused' '{"scenarios":[{"id":"SCN-18","linkedTests":["tests/escape.spec.ts"]}]}' 2 'stable regular file'
mkdir -p "$WORK/outside/escaped-tests"
printf 'escape through directory\n' >"$WORK/outside/escaped-tests/proof.spec.ts"
ln -s "$WORK/outside/escaped-tests" "$WORK/repo/escaped-tests"
run_case 'authored intermediate symlink escape refused' '{"scenarios":[{"id":"SCN-ESCAPE-DIR-18","linkedTests":["escaped-tests/proof.spec.ts"]}]}' 2 'stable regular file'
ln -s 'α.spec.ts' "$WORK/repo/tests/contained.spec.ts"
run_case 'final-component authored symlink refused' '{"scenarios":[{"id":"SCN-19","linkedTests":["tests/contained.spec.ts"]}]}' 2 'stable regular file'
run_case 'malformed secondary v1 identity alias refused' '{"scenarios":[{"id":"SCN-20","scenarioId":"SCN-OTHER","linkedTests":[]}]}' 2 "identity alias 'scenarioId' does not match"
run_case 'conflicting valid scenario id aliases refused' '{"schemaVersion":1,"scenarios":[{"id":"SCN-FIRST-20","scenarioId":"SCN-OTHER-20","linkedTests":[]}]}' 2 'conflicting aliases (id, scenarioId)'
run_case 'malformed primary v1 identity alias prevents fallback' '{"schemaVersion":1,"scenarios":[{"id":"invalid","scenarioId":" SCN-FALLBACK-20 ","linkedTests":[]}]}' 2 "identity alias 'id' does not match"
run_case 'v1 identity aliases agree after whitespace normalization' '{"schemaVersion":1,"scenarios":[{"id":" SCN-NORMALIZED-20 ","scenarioId":"SCN-NORMALIZED-20","linkedTests":[]}]}' 0 '"scenarioId":"SCN-NORMALIZED-20"'
run_case 'one valid v1 scenarioId alias is accepted as fallback identity' '{"schemaVersion":1,"scenarios":[{"scenarioId":" SCN-FALLBACK-20 ","linkedTests":[]}]}' 0 '"scenarioId":"SCN-FALLBACK-20"'
run_case 'duplicate effective IDs refused' '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUP-20"},{"scenarioId":"SCN-DUP-20"}]}' 2 'duplicate effective scenario id at index 1'
run_case 'control character in scenario id refused' '{"scenarios":[{"id":"SCN-21\u0009BAD","linkedTests":[]}]}' 2 "identity alias 'id' contains a control character"
run_case 'conflicting authored field aliases refused' '{"scenarios":[{"id":"SCN-22","linkedTests":["tests/α.spec.ts"],"linkedTestContracts":["tests/other.spec.ts"]}]}' 2 'conflicting aliases (linkedTests, linkedTestContracts)'
run_case 'identical authored field aliases deduplicated' '{"scenarios":[{"id":"SCN-23","linkedTests":["tests/α.spec.ts"],"linkedTestContracts":["tests/α.spec.ts"]}]}' 0 '"references":[{"field":"linkedTests","index":0'
run_case 'version 2 authored scalar refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-1","title":"scalar","requiredTestType":"unit","linkedTests":["tests/α.spec.ts"]}]}' 2 'is not of type'
run_case 'version 2 scenarioId alias refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-STRICT-1","scenarioId":"SCN-V2-LEGACY-1","title":"strict","requiredTestType":"unit","linkedTests":[]}]}' 2 'Additional properties are not allowed'
run_case 'version 2 id whitespace refused by reader' '{"schemaVersion":2,"scenarios":[{"id":" SCN-V2-STRICT-2 ","title":"strict","requiredTestType":"unit","linkedTests":[]}]}' 2 'does not match'
run_case 'version 2 planned file alias refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-2","title":"planned alias","requiredTestType":"e2e-ui","plannedTests":[{"file":"tests/future.spec.ts","title":"future","type":"e2e-ui"}]}]}' 2 "'path' is a required property"
run_case 'version 2 authored type required' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-3","title":"authored type","requiredTestType":"unit","linkedTests":[{"file":"tests/α.spec.ts"}]}]}' 2 "'type' is a required property"
run_case 'unsupported schema version refused' '{"schemaVersion":3,"scenarios":[]}' 2 'unsupported scenario manifest schemaVersion'
run_case 'dot segment refused' '{"scenarios":[{"id":"SCN-24","linkedTests":["tests/./α.spec.ts"]}]}' 2 'dot segment'
run_case 'empty segment refused' '{"scenarios":[{"id":"SCN-25","linkedTests":["tests//α.spec.ts"]}]}' 2 'empty segment'
run_case 'version 2 authored legacy alias refused' '{"schemaVersion":2,"scenarios":[{"id":"SCN-V2-4","title":"legacy alias","requiredTestType":"e2e-ui","linkedTests":[{"file":"tests/α.spec.ts","title":"legacy","type":"e2e-ui"}]}]}' 2 'Additional properties are not allowed'
run_case 'unknown sentinel-shaped name refused' '{"scenarios":[{"id":"SCN-26","title":"future","requiredTestType":"unit","linkedTests":["__NOT_A_SENTINEL__"]}]}' 2 "unknown legacy sentinel '__NOT_A_SENTINEL__'"
for sentinel in planned planned-not-authored not-authored; do
  run_case "explicit planned sentinel $sentinel" "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-SENTINEL-26\",\"title\":\"future\",\"requiredTestType\":\"unit\",\"linkedTests\":[\"$sentinel\"]}]}" 0 '"sourceClassification":"legacy-planned-explicit"'
done
run_case 'future-test sentinel source classification' '{"schemaVersion":1,"scenarios":[{"id":"SCN-FUTURE-26","title":"future","requiredTestType":"unit","linkedTests":["__FUTURE_TEST__"]}]}' 0 '"sourceClassification":"legacy-planned-sentinel"'
run_case 'planned testState source classification' '{"schemaVersion":1,"scenarios":[{"id":"SCN-STATE-26","linkedTests":[{"path":"tests/future.ts","title":"future","type":"unit","testState":"planned-not-authored"}]}]}' 0 '"sourceClassification":"legacy-test-state"'
run_case 'unknown historical testState refused' '{"scenarios":[{"id":"SCN-27","linkedTests":[{"file":"tests/α.spec.ts","testState":"implemented"}]}]}' 2 "unknown testState 'implemented'"

v2_projection='{"schemaVersion":2,"scenarios":[{"id":"SCN-PROJECTION-1","scopeRef":"SCOPE-1","title":"Projection","gherkin":{"given":"state","when":"action","then":"result"},"behaviorClass":"ui","changeType":"changed","requiredTestType":"e2e-ui","regressionRequired":true,"linkedTests":[{"file":"tests/α.spec.ts","type":"e2e-ui","testId":"works"}],"plannedTests":[{"path":"tests/future.ts","title":"future","type":"e2e-ui"}],"evidenceRefs":["report.md#proof"],"implementationRefs":["src/a.ts"],"invariantRefs":["INV-1"],"obligations":[],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-fixture","assertionSurface":"visible-ui","dependencyPath":"not-applicable"}}]}'
v1_object_projection='{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-PROJECTION-2","scope":"02-consumer","title":"Legacy object","requiredTestType":"unit","linkedTests":["tests/α.spec.ts#legacy works"],"evidenceRefs":["report.md#legacy"]}]}'
v1_array_projection='[{"id":"SCN-PROJECTION-3","scopeId":"3","title":"Legacy array","requiredTestType":"unit","linkedTests":[{"file":"tests/α.spec.ts","title":"array works","type":"unit"}]}]'
printf '%s\n' "$v2_projection" >"$WORK/repo/manifest.json"
file_output="$(python3 "$READER" "$WORK/repo/manifest.json" --repo-root "$WORK/repo")"
stdin_output="$(printf '%s\n' "$v2_projection" | python3 "$READER" --stdin --repo-root "$WORK/repo")"
v1_object_output="$(printf '%s\n' "$v1_object_projection" | python3 "$READER" --stdin --repo-root "$WORK/repo")"
v1_array_output="$(printf '%s\n' "$v1_array_projection" | python3 "$READER" --stdin --repo-root "$WORK/repo")"
if [[ "$file_output" == "$stdin_output" ]] && printf '%s\n%s\n%s\n' "$file_output" "$v1_object_output" "$v1_array_output" | python3 -c '
import json, sys
docs = [json.loads(line) for line in sys.stdin]
assert [(d["manifestSchemaVersion"], d["sourceEnvelope"]) for d in docs] == [(2, "schema-version-2"), (1, "schema-version-1"), (1, "legacy-top-level-array")]
expected_scopes = ["SCOPE-1", "02-consumer", "3"]
expected_ids = ["SCN-PROJECTION-1", "SCN-PROJECTION-2", "SCN-PROJECTION-3"]
expected_fields = ["id", "scenarioId", "id"]
for document, scope_ref, scenario_id, authored_field in zip(docs, expected_scopes, expected_ids, expected_fields):
    assert list(document) == ["projectionVersion", "manifestSchemaVersion", "sourceEnvelope", "scenarios"]
    scenario = document["scenarios"][0]
    assert list(scenario) == ["scenarioId", "scenarioIndex", "scopeRef", "title", "requiredTestType", "evidenceRefs", "implementationRefs", "invariantRefs", "obligations", "testMechanism", "authoredIdentity", "metadata", "references"]
    assert scenario["scenarioId"] == scenario_id and scenario["scopeRef"] == scope_ref
    assert scenario["metadata"]["scopeRef"] == scope_ref
    identity = scenario["authoredIdentity"]
    assert identity["canonicalId"] == scenario_id and identity["authoredField"] == authored_field
    assert identity["authoredAliases"] == {authored_field: scenario_id}
    authored = scenario["references"][0]
    assert list(authored) == ["field", "index", "kind", "sourceClassification", "path", "title", "type", "sentinel", "canonicalPath", "canonicalRelativePath", "identity", "exists"]
    assert authored["canonicalRelativePath"] == "tests/α.spec.ts" and authored["exists"] is True
    assert set(authored["identity"]) == {"device", "inode", "mode", "size", "modifiedNanoseconds"}
v2 = docs[0]["scenarios"][0]
assert [r["kind"] for r in v2["references"]] == ["authored", "planned"]
assert v2["evidenceRefs"] == ["report.md#proof"] and v2["implementationRefs"] == ["src/a.ts"] and v2["invariantRefs"] == ["INV-1"]
assert v2["metadata"]["gherkin"] == {"given":"state", "when":"action", "then":"result"}
assert v2["metadata"]["behaviorClass"] == "ui" and v2["metadata"]["changeType"] == "changed" and v2["metadata"]["regressionRequired"] is True
# This is the exact root-level lookup used by the scope consumer.
assert [scenario.get("scopeRef") for scenario in (d["scenarios"][0] for d in docs)] == expected_scopes
'; then
  pass 'complete v1 object, v1 array, and v2 projection equality/order with scope consumer compatibility'
else
  fail 'complete v1 object, v1 array, and v2 projection equality/order with scope consumer compatibility'
fi

printf '%s\n' '{"schemaVersion":2,"scenarios":[{"id":"SCN-PARTIAL-1","title":"first","requiredTestType":"unit","linkedTests":[]},{"id":"SCN-PARTIAL-2","title":"second","requiredTestType":"unit","linkedTests":[7]}]}' >"$WORK/repo/manifest.json"
status=0
python3 "$READER" "$WORK/repo/manifest.json" --repo-root "$WORK/repo" >"$WORK/stdout" 2>"$WORK/stderr" || status=$?
if [[ "$status" -eq 2 && ! -s "$WORK/stdout" ]] && grep -Fq 'linkedTests/0' "$WORK/stderr"; then
  pass 'malformed later scenario emits no partial projection'
else
  fail 'malformed later scenario emits no partial projection'
fi

if python3 - "$READER" "$WORK/repo" <<'PY'
import importlib.util, json, os, pathlib, sys
reader_path, repo = sys.argv[1:]
spec = importlib.util.spec_from_file_location("reader", reader_path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
manifest = pathlib.Path(repo) / "mutation.json"
manifest.write_text(json.dumps({"schemaVersion": 1, "scenarios": [{"id": "SCN-MUTATION-1", "title": "future", "requiredTestType": "unit", "linkedTests": ["__FUTURE_TEST__"]}]}), encoding="utf-8")
before = sorted(str(p.relative_to(repo)) for p in pathlib.Path(repo).rglob("*"))
projected = module.read_references(str(manifest), repo)
after = sorted(str(p.relative_to(repo)) for p in pathlib.Path(repo).rglob("*"))
assert before == after and projected["scenarios"][0]["references"][0]["kind"] == "planned"
PY
then
  pass 'planned-reference mutation-sensitive probe'
else
  fail 'planned-reference mutation-sensitive probe'
fi

if python3 - "$READER" "$WORK/repo" <<'PY'
import importlib.util, json, os, pathlib, sys
reader_path, repo_text = sys.argv[1:]
repo = pathlib.Path(repo_text)
spec = importlib.util.spec_from_file_location("reader", reader_path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
authored = repo / "tests" / "identity.spec.ts"
authored.write_text("authored identity\n", encoding="utf-8")
manifest = repo / "identity.json"
manifest.write_text(json.dumps({"schemaVersion": 2, "scenarios": [{"id": "SCN-IDENTITY-1", "title": "identity", "requiredTestType": "unit", "linkedTests": [{"file": "tests/identity.spec.ts", "type": "unit", "testId": "identity"}]}]}), encoding="utf-8")
projected = module.read_references(str(manifest), str(repo))
reference = projected["scenarios"][0]["references"][0]
recorded = reference["identity"]
initial = os.stat(authored)
assert recorded == {
    "device": initial.st_dev,
    "inode": initial.st_ino,
    "mode": initial.st_mode & 0o7777,
    "size": initial.st_size,
    "modifiedNanoseconds": initial.st_mtime_ns,
}
authored.write_text("mutated authored identity with a different size\n", encoding="utf-8")
mutated = os.stat(authored)
mutated_identity = {
    "device": mutated.st_dev,
    "inode": mutated.st_ino,
    "mode": mutated.st_mode & 0o7777,
    "size": mutated.st_size,
    "modifiedNanoseconds": mutated.st_mtime_ns,
}
assert mutated_identity != recorded
replacement = repo / "tests" / "identity.replacement"
replacement.write_text("replacement\n", encoding="utf-8")
os.replace(replacement, authored)
replaced = os.stat(authored)
replaced_identity = {
    "device": replaced.st_dev,
    "inode": replaced.st_ino,
    "mode": replaced.st_mode & 0o7777,
    "size": replaced.st_size,
    "modifiedNanoseconds": replaced.st_mtime_ns,
}
assert replaced_identity != recorded and replaced_identity["inode"] != recorded["inode"]
PY
then
  pass 'authored identity values expose post-projection mutation and replacement'
else
  fail 'authored identity values expose post-projection mutation and replacement'
fi

if python3 - "$READER" "$WORK/repo" <<'PY'
import importlib.util, json, os, pathlib, sys
reader_path, repo_text = sys.argv[1:]
repo = pathlib.Path(repo_text)
spec = importlib.util.spec_from_file_location("reader_race", reader_path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
authored = repo / "tests" / "race.spec.ts"
authored.write_text("original\n", encoding="utf-8")
replacement = repo / "tests" / "race.replacement"
replacement.write_text("replacement\n", encoding="utf-8")
manifest = repo / "race.json"
manifest.write_text(json.dumps({"schemaVersion": 1, "scenarios": [{"id": "SCN-RACE-1", "linkedTests": ["tests/race.spec.ts"]}]}), encoding="utf-8")
real_verify = module.verify_pathname_identity
def replace_then_verify(root_real, relative_path, expected_metadata):
    os.replace(replacement, authored)
    return real_verify(root_real, relative_path, expected_metadata)
module.verify_pathname_identity = replace_then_verify
try:
    module.read_references(str(manifest), str(repo))
except module.ReferenceError as error:
    assert "pathname identity changed" in str(error), error
else:
    raise AssertionError("pathname replacement was accepted")
PY
then
  pass 'authored pathname replacement race is refused before projection returns'
else
  fail 'authored pathname replacement race is refused before projection returns'
fi

mkdir -p "$WORK/rogue/bubbles/schemas"
printf '%s\n' '{"type":"object"}' >"$WORK/rogue/bubbles/schemas/scenario-manifest-v2.schema.json"
printf '%s\n' '{"schemaVersion":2,"scenarios":[{"id":"SCN-ROOT-AUTHORITY-1","requiredTestType":"unit"}]}' >"$WORK/repo/root-authority.json"
root_authority_status=0
root_authority_output="$(cd "$WORK/rogue" && python3 "$READER" "$WORK/repo/root-authority.json" --repo-root "$WORK/repo" 2>&1)" || root_authority_status=$?
if [[ "$root_authority_status" -eq 2 && "$root_authority_output" == *"'title' is a required property"* ]]; then
  pass 'version 2 schema authority follows declared repository root rather than cwd'
else
  fail "version 2 schema authority follows declared repository root rather than cwd (exit=$root_authority_status output=$root_authority_output)"
fi

run_probe_cleanup_case() {
  local case_name="$1" expected_status="$2" probe_parent
  probe_parent="$WORK/probe-$case_name-parent"
  mkdir -p "$probe_parent"
  printf 'preserve me\n' >"$probe_parent/sentinel"
  python3 - "$0" "$case_name" "$expected_status" "$probe_parent" <<'PY'
import os, pathlib, select, shutil, signal, stat, subprocess, sys
selftest, case_name, expected_text, parent_text = sys.argv[1:]
parent = pathlib.Path(parent_text)
sentinel = parent / "sentinel"
parked = parent / "parked-owned-child"
symlink_target = parent / "caller-symlink-target"
child = None
owned_identity = None
environment = os.environ.copy()
environment["SCENARIO_REFERENCE_READER_SIGNAL_PROBE_WORK"] = parent_text
process = subprocess.Popen(
    ["bash", selftest],
    env=environment,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    text=True,
)
try:
    assert process.stdout is not None
    ready, _, _ = select.select([process.stdout], [], [], 5)
    assert ready, "signal probe did not become ready"
    line = process.stdout.readline().rstrip("\n")
    prefix = f"READY {parent_text}/scenario-reference-reader-selftest."
    assert line.startswith(prefix), line
    child = pathlib.Path(line.removeprefix("READY "))
    assert child.parent == parent and child.is_dir()
    owned_identity = (child.stat().st_dev, child.stat().st_ino)
    assert sentinel.read_text(encoding="utf-8") == "preserve me\n"
    if case_name == "NORMAL":
        with (child / "hold").open("w", encoding="utf-8") as hold:
            hold.write("continue\n")
    elif case_name == "REPLACED":
      child.rename(parked)
      child.mkdir()
      (child / "caller-content").write_text("do not delete replacement\n", encoding="utf-8")
      process.send_signal(signal.SIGTERM)
    elif case_name == "SYMLINK":
      child.rename(parked)
      symlink_target.mkdir()
      (symlink_target / "caller-content").write_text("do not follow symlink\n", encoding="utf-8")
      child.symlink_to(symlink_target, target_is_directory=True)
      process.send_signal(signal.SIGTERM)
    elif case_name == "ABSENT":
      child.rename(parked)
      process.send_signal(signal.SIGTERM)
    else:
        process.send_signal(getattr(signal, case_name))
    stdout, stderr = process.communicate(timeout=5)
    assert process.returncode == int(expected_text), (process.returncode, stdout, stderr)
    assert parent.is_dir()
    assert sentinel.read_text(encoding="utf-8") == "preserve me\n"
    if case_name == "REPLACED":
      assert child.is_dir() and not child.is_symlink()
      assert (child / "caller-content").read_text(encoding="utf-8") == "do not delete replacement\n"
      assert (parked.stat().st_dev, parked.stat().st_ino) == owned_identity
      assert "refusing cleanup because owned workspace identity changed" in stderr
    elif case_name == "SYMLINK":
      assert child.is_symlink()
      assert (symlink_target / "caller-content").read_text(encoding="utf-8") == "do not follow symlink\n"
      assert (parked.stat().st_dev, parked.stat().st_ino) == owned_identity
      assert "refusing cleanup because owned workspace path is absent or is not a non-symlink directory" in stderr
    elif case_name == "ABSENT":
      assert not child.exists() and not child.is_symlink()
      assert (parked.stat().st_dev, parked.stat().st_ino) == owned_identity
      assert "refusing cleanup because owned workspace path is absent or is not a non-symlink directory" in stderr
    else:
      assert not child.exists(), child
finally:
    if process.poll() is None:
        process.kill()
        process.wait()
    if child is not None and child.is_symlink():
      child.unlink()
    elif child is not None and child.exists() and child.parent == parent:
      shutil.rmtree(child)
    if parked.exists() and parked.parent == parent:
      parked_status = parked.stat()
      assert stat.S_ISDIR(parked_status.st_mode)
      if owned_identity is not None:
        assert (parked_status.st_dev, parked_status.st_ino) == owned_identity
      shutil.rmtree(parked)
    if symlink_target.exists() and symlink_target.parent == parent:
      shutil.rmtree(symlink_target)
PY
}

if run_probe_cleanup_case SIGINT 130; then
  pass 'SIGINT exits 130 and removes active workspace'
else
  fail 'SIGINT exits 130 and removes active workspace'
fi
if run_probe_cleanup_case SIGTERM 143; then
  pass 'SIGTERM exits 143 and removes active workspace'
else
  fail 'SIGTERM exits 143 and removes active workspace'
fi
if run_probe_cleanup_case NORMAL 0; then
  pass 'normal completion preserves probe parent and removes owned child'
else
  fail 'normal completion preserves probe parent and removes owned child'
fi

replacement_cases_ok=true
run_probe_cleanup_case REPLACED 143 || replacement_cases_ok=false
run_probe_cleanup_case SYMLINK 143 || replacement_cases_ok=false
run_probe_cleanup_case ABSENT 143 || replacement_cases_ok=false

refusal_parent="$WORK/refusal-parent"
mkdir -p "$refusal_parent"
printf 'preserve me\n' >"$refusal_parent/sentinel"
printf 'not a directory\n' >"$refusal_parent/unusable"
refusal_status=0
SCENARIO_REFERENCE_READER_SIGNAL_PROBE_WORK="$refusal_parent/unusable" bash "$0" >"$refusal_parent/stdout" 2>"$refusal_parent/stderr" || refusal_status=$?
if [[ "$replacement_cases_ok" == true \
  && "$refusal_status" -eq 2 \
  && -d "$refusal_parent" \
  && "$(cat "$refusal_parent/sentinel")" == 'preserve me' \
  && -f "$refusal_parent/unusable" \
  && "$(find "$refusal_parent" -maxdepth 1 -type d -name 'scenario-reference-reader-selftest.*' -print)" == '' \
  && "$(cat "$refusal_parent/stderr")" == 'scenario-reference-reader-selftest: signal probe parent must be an existing non-symlink directory' ]]; then
  pass 'unsafe parent and replaced owned paths are refused without deleting caller content'
else
  fail "unsafe parent and replaced owned path refusal preserves caller content (exit=$refusal_status replacements=$replacement_cases_ok)"
fi

printf 'scenario-reference-reader-selftest: %s checks, %s failures\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]]
