#!/usr/bin/env bash
# Contract tests for the experience-recall schema and neutral adapter.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEMA="$FRAMEWORK_ROOT/schemas/experience-recall.schema.json"
NONE="$FRAMEWORK_ROOT/adapters/experience-recall/none.sh"
PASS=0
FAIL=0

pass() {
  PASS=$((PASS + 1))
  echo "PASS: $1"
}

fail() {
  FAIL=$((FAIL + 1))
  echo "FAIL: $1"
}

assert_neutral() {
  local verb="$1"
  local expected="$2"
  local output rc
  output="$(bash "$NONE" "$verb" 2>&1)"
  rc=$?
  if [ "$rc" -eq 0 ] && [ "$output" = "$expected" ]; then
    pass "none $verb emits $expected and exits zero"
  else
    fail "none $verb emitted '$output' with exit $rc (expected $expected, exit 0)"
  fi
}

echo "experience-recall-adapter-contract-selftest"

for verb in search export; do
  assert_neutral "$verb" '[]'
done

for verb in read status freshness sync delete capabilities; do
  assert_neutral "$verb" '{}'
done

for verb in search read status freshness sync export delete capabilities; do
  output="$(bash "$NONE" selftest "$verb" 2>&1)"
  rc=$?
  case "$verb" in
    search | export) expected='[]' ;;
    *) expected='{}' ;;
  esac
  if [ "$rc" -eq 0 ] && [ "$output" = "$expected" ]; then
    pass "none selftest $verb declares $expected"
  else
    fail "none selftest $verb emitted '$output' with exit $rc"
  fi
done

bash "$NONE" unknown >/dev/null 2>&1
if [ "$?" -eq 1 ]; then
  pass "none rejects an unknown verb"
else
  fail "none accepted an unknown verb"
fi

bash "$NONE" selftest unknown >/dev/null 2>&1
if [ "$?" -eq 1 ]; then
  pass "none rejects an unknown selftest target"
else
  fail "none accepted an unknown selftest target"
fi

if grep -Eq '(^|[^[:alpha:]])(curl|wget|pip|npm|npx|brew|apt|git[[:space:]]+clone)([^[:alpha:]]|$)' "$NONE"; then
  fail "none contains an installer or network command"
else
  pass "none contains no installer or network command"
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "SKIP: schema assertions (python3 not installed)"
elif ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  echo "SKIP: schema assertions (python jsonschema not installed)"
else
  if python3 - "$SCHEMA" <<'PY'
import copy
import json
import sys

import jsonschema

schema_path = sys.argv[1]
with open(schema_path, encoding="utf-8") as handle:
    schema = json.load(handle)

jsonschema.Draft202012Validator.check_schema(schema)
validator = jsonschema.Draft202012Validator(
    schema,
    format_checker=jsonschema.Draft202012Validator.FORMAT_CHECKER,
)

passed = 0
failed = 0


def check(label, instance, expected_valid):
    global passed, failed
    errors = sorted(validator.iter_errors(instance), key=lambda error: list(error.path))
    valid = not errors
    if valid == expected_valid:
        passed += 1
        print(f"PASS: {label}")
        return
    failed += 1
    detail = "valid" if valid else errors[0].message
    print(f"FAIL: {label} ({detail})")


digest = "sha256:" + "a" * 64
timestamp = "2026-08-06T12:00:00Z"
anchor = {
    "relativePath": "improvements/INDEX.md",
    "selector": "#imp-037",
    "contentDigest": digest,
    "observedAt": timestamp,
}
freshness = {
    "contractType": "freshness",
    "state": "fresh",
    "sourceDigest": digest,
    "checkedAt": timestamp,
    "reason": None,
}
lifecycle = {
    "contractType": "lifecycle",
    "state": "admitted",
    "admittedAt": timestamp,
    "supersededAt": None,
    "expiredAt": None,
    "deletedAt": None,
}
provenance = {
    "extractor": "experience-recall-index",
    "extractorVersion": "1",
    "provider": "local-lexical",
    "providerVersion": "1",
    "derivedAt": timestamp,
}
record = {
    "contractType": "record",
    "schemaVersion": 1,
    "recordId": "experience-record-1",
    "kind": "owner-decision",
    "summary": "Owner accepted the provider-neutral recall foundation.",
    "searchableFields": {
        "identifiers": ["IMP-037"],
        "phrases": ["experience recall"],
        "tags": ["recall"],
    },
    "repositoryAlias": "bubbles",
    "specRef": None,
    "scopeRef": "SCOPE-1",
    "scenarioRefs": [],
    "sourceAnchor": anchor,
    "sourceTrust": "owner-approved",
    "recallAuthority": "advisory",
    "freshness": freshness,
    "lifecycle": lifecycle,
    "provenance": provenance,
}
query = {
    "contractType": "query",
    "schemaVersion": 1,
    "text": "experience recall",
    "repositoryAlias": "bubbles",
    "specRef": None,
    "scopeRef": "SCOPE-1",
}
complete_query = copy.deepcopy(query)
complete_query.update(
    {
        "kinds": ["owner-decision"],
        "sourceTrust": ["owner-approved"],
        "lifecycleStates": ["admitted"],
        "freshnessStates": ["fresh"],
        "limit": 5,
    }
)
result = {
    "contractType": "result",
    "schemaVersion": 1,
    "recordId": "experience-record-1",
    "kind": "owner-decision",
    "snippet": "Owner accepted the provider-neutral recall foundation.",
    "repositoryAlias": "bubbles",
    "specRef": None,
    "scopeRef": "SCOPE-1",
    "scenarioRefs": [],
    "sourceAnchor": anchor,
    "sourceTrust": "owner-approved",
    "recallAuthority": "advisory",
    "freshness": freshness,
    "lifecycle": lifecycle,
    "provenance": provenance,
    "score": {
        "exactIdentifier": 1,
        "exactPhrase": 1,
        "tokenOverlap": 2,
        "tagOverlap": 1,
        "total": 5,
    },
}
provider = {
    "contractType": "provider",
    "schemaVersion": 1,
    "adapter": "none",
    "providerVersion": "1",
    "capabilities": {
        "search": "neutral",
        "read": "neutral",
        "status": "neutral",
        "freshness": "neutral",
        "sync": "neutral",
        "export": "neutral",
        "delete": "neutral",
        "capabilities": "neutral",
    },
    "networkAccess": False,
    "automaticInstall": False,
    "defaultBinaryPath": None,
}

check("valid normalized record is accepted", record, True)
check("query without limit is accepted for defaulting", query, True)
check("complete normalized query is accepted", complete_query, True)

minimum_query = copy.deepcopy(query)
minimum_query["limit"] = 1
check("query limit 1 is accepted", minimum_query, True)

bounded_query = copy.deepcopy(query)
bounded_query["limit"] = 20
check("query limit 20 is accepted", bounded_query, True)

unbounded_query = copy.deepcopy(query)
unbounded_query["limit"] = 21
check("query limit 21 is rejected", unbounded_query, False)

zero_query = copy.deepcopy(query)
zero_query["limit"] = 0
check("query limit zero is rejected", zero_query, False)

for field in (
    "repositoryAlias",
    "specRef",
    "scopeRef",
    "sourceAnchor",
    "sourceTrust",
    "recallAuthority",
    "freshness",
    "provenance",
):
    missing = copy.deepcopy(record)
    del missing[field]
    check(f"record missing {field} is rejected", missing, False)

for field in ("repositoryAlias", "specRef", "scopeRef"):
    missing = copy.deepcopy(complete_query)
    del missing[field]
    check(f"query missing {field} is rejected", missing, False)

for field in (
    "repositoryAlias",
    "specRef",
    "scopeRef",
    "sourceAnchor",
    "sourceTrust",
    "recallAuthority",
    "freshness",
    "provenance",
  ):
    missing = copy.deepcopy(result)
    del missing[field]
    check(f"result missing {field} is rejected", missing, False)

unknown_lifecycle = copy.deepcopy(record)
unknown_lifecycle["lifecycle"]["state"] = "forgotten"
check("unknown lifecycle state is rejected", unknown_lifecycle, False)

unknown_freshness = copy.deepcopy(record)
unknown_freshness["freshness"]["state"] = "cached"
check("unknown freshness state is rejected", unknown_freshness, False)

non_advisory_record = copy.deepcopy(record)
non_advisory_record["recallAuthority"] = "authoritative"
check("non-advisory record authority is rejected", non_advisory_record, False)

check("valid normalized result is accepted", result, True)
non_advisory_result = copy.deepcopy(result)
non_advisory_result["recallAuthority"] = "current-source"
check("non-advisory result authority is rejected", non_advisory_result, False)

check("valid freshness contract is accepted", freshness, True)
for state in ("fresh", "stale", "unknown", "disabled"):
    candidate = copy.deepcopy(freshness)
    candidate["state"] = state
    candidate["sourceDigest"] = digest if state in ("fresh", "stale") else None
    check(f"closed freshness state {state} is accepted", candidate, True)

fresh_without_digest = copy.deepcopy(freshness)
fresh_without_digest["sourceDigest"] = None
check("fresh state without a source digest is rejected", fresh_without_digest, False)

check("valid provider contract is accepted", provider, True)

network_provider = copy.deepcopy(provider)
network_provider["networkAccess"] = True
check("provider network access is rejected", network_provider, False)

installing_provider = copy.deepcopy(provider)
installing_provider["automaticInstall"] = True
check("provider automatic install is rejected", installing_provider, False)

binary_provider = copy.deepcopy(provider)
binary_provider["defaultBinaryPath"] = "/usr/bin/recall"
check("provider default binary path is rejected", binary_provider, False)

for state, timestamp_field in (
    ("admitted", None),
    ("superseded", "supersededAt"),
    ("expired", "expiredAt"),
    ("deleted", "deletedAt"),
):
    candidate = copy.deepcopy(lifecycle)
    candidate["state"] = state
    if timestamp_field:
        candidate[timestamp_field] = timestamp
    check(f"closed lifecycle state {state} is accepted", candidate, True)

unknown_standalone = copy.deepcopy(lifecycle)
unknown_standalone["state"] = "archived"
check("standalone unknown lifecycle is rejected", unknown_standalone, False)

for unsafe_path in (
    "/etc/passwd",
    "../outside.json",
    "improvements/../outside.json",
):
    candidate = copy.deepcopy(record)
    candidate["sourceAnchor"]["relativePath"] = unsafe_path
    check(f"unsafe source path {unsafe_path!r} is rejected", candidate, False)

malformed_anchor_digest = copy.deepcopy(record)
malformed_anchor_digest["sourceAnchor"]["contentDigest"] = "sha256:abc"
check("malformed source-anchor digest is rejected", malformed_anchor_digest, False)

malformed_freshness_digest = copy.deepcopy(freshness)
malformed_freshness_digest["sourceDigest"] = "sha256:" + "A" * 64
check("malformed freshness digest is rejected", malformed_freshness_digest, False)


def with_extra(instance, path):
    candidate = copy.deepcopy(instance)
    target = candidate
    for segment in path:
        target = target[segment]
    target["unexpected"] = True
    return candidate


for label, instance, path in (
    ("record", record, ()),
    ("record source anchor", record, ("sourceAnchor",)),
    ("record searchable fields", record, ("searchableFields",)),
    ("record freshness", record, ("freshness",)),
    ("record lifecycle", record, ("lifecycle",)),
    ("record provenance", record, ("provenance",)),
    ("query", complete_query, ()),
    ("result", result, ()),
    ("result score", result, ("score",)),
    ("provider", provider, ()),
    ("provider capabilities", provider, ("capabilities",)),
):
    check(f"closed {label} rejects an extra field", with_extra(instance, path), False)

defs = schema["$defs"]
query_limit = defs["query"]["properties"]["limit"]
closed_lifecycle = defs["lifecycle"]["properties"]["state"]["enum"]
required_metadata = set(defs["recordMetadata"]["required"])
required_fields = {
    "repositoryAlias",
    "scopeRef",
    "sourceAnchor",
    "sourceTrust",
    "freshness",
    "provenance",
}
if query_limit.get("default") == 5 and query_limit.get("maximum") == 20:
    passed += 1
    print("PASS: query contract pins default 5 and maximum 20")
else:
    failed += 1
    print("FAIL: query contract default/maximum drifted")

if closed_lifecycle == ["admitted", "superseded", "expired", "deleted"]:
    passed += 1
    print("PASS: lifecycle enum is closed and ordered")
else:
    failed += 1
    print(f"FAIL: lifecycle enum drifted: {closed_lifecycle}")

if defs["recallAuthority"].get("const") == "advisory":
    passed += 1
    print("PASS: recall authority is fixed to advisory")
else:
    failed += 1
    print("FAIL: recall authority is not fixed to advisory")

if required_fields.issubset(required_metadata):
    passed += 1
    print("PASS: source, repository, scope, trust, freshness, and provenance are required")
else:
    failed += 1
    print(f"FAIL: required metadata missing: {sorted(required_fields - required_metadata)}")

provider_properties = defs["provider"]["properties"]
if (
    provider_properties["networkAccess"].get("const") is False
    and provider_properties["automaticInstall"].get("const") is False
    and provider_properties["defaultBinaryPath"].get("type") == "null"
):
    passed += 1
    print("PASS: provider contract forbids network, automatic install, and a default binary path")
else:
    failed += 1
    print("FAIL: provider dependency-safety contract drifted")

print(f"schema assertions: {passed} passed, {failed} failed")
sys.exit(1 if failed else 0)
PY
  then
    pass "schema validates all positive and adversarial fixtures"
  else
    fail "schema fixture validation failed"
  fi
fi

echo "experience-recall-adapter-contract-selftest: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
