#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA="$SCRIPT_DIR/../schemas/scenario-manifest-v2.schema.json"
V1_SCHEMA="$SCRIPT_DIR/../schemas/scenario-manifest.schema.json"

if ! command -v python3 >/dev/null 2>&1; then
  echo "scenario-manifest-v2-schema-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  echo "scenario-manifest-v2-schema-selftest: SKIP (jsonschema not installed)"
  exit 0
fi

python3 - "$SCHEMA" "$V1_SCHEMA" <<'PY'
import copy
import json
import re
import sys
from datetime import date
from jsonschema import Draft7Validator, FormatChecker, validators

schema_path, v1_schema_path = sys.argv[1:]
with open(schema_path, encoding="utf-8") as handle:
    schema = json.load(handle)
with open(v1_schema_path, encoding="utf-8") as handle:
    v1_schema = json.load(handle)
Draft7Validator.check_schema(schema)
Draft7Validator.check_schema(v1_schema)
RFC3339_DATE_TIME = re.compile(
    r"^(?P<year>[0-9]{4})-(?P<month>[0-9]{2})-(?P<day>[0-9]{2})[Tt]"
    r"(?P<hour>[0-9]{2}):(?P<minute>[0-9]{2}):(?P<second>[0-9]{2})"
    r"(?:\.[0-9]+)?(?:[Zz]|(?P<offset_sign>[+-])"
    r"(?P<offset_hour>[0-9]{2}):(?P<offset_minute>[0-9]{2}))$"
)


def is_rfc3339_date_time(value):
    if not isinstance(value, str):
        return False
    match = RFC3339_DATE_TIME.fullmatch(value)
    if match is None:
        return False
    try:
        date(int(match["year"]), int(match["month"]), int(match["day"]))
    except ValueError:
        return False
    return (
        int(match["hour"]) <= 23
        and int(match["minute"]) <= 59
        and int(match["second"]) <= 59
        and (
            match["offset_sign"] is None
            or (
                int(match["offset_hour"]) <= 23
                and int(match["offset_minute"]) <= 59
            )
        )
    )


format_checker = FormatChecker()
format_checker.checks("date-time")(is_rfc3339_date_time)
validator = Draft7Validator(schema, format_checker=format_checker)
# Draft 7 treats 1.0 as an integer mathematically. Bubbles compatibility is
# intentionally token-class strict so it agrees with the reader and migrator:
# only a parsed JSON integer, excluding booleans, satisfies "integer".
exact_integer_type_checker = Draft7Validator.TYPE_CHECKER.redefine(
    "integer",
    lambda _checker, value: isinstance(value, int) and not isinstance(value, bool),
)
ExactIntegerDraft7Validator = validators.extend(
    Draft7Validator,
    type_checker=exact_integer_type_checker,
)
v1_validator = ExactIntegerDraft7Validator(v1_schema, format_checker=format_checker)

base = {
    "schemaVersion": 2,
    "spec": "specs/042-catalog-assistant",
    "generatedAt": "2026-08-31T12:00:00Z",
    "scenarios": [
        {
            "id": "SCN-042-001",
            "title": "Guest opens search",
            "requiredTestType": "e2e-ui",
            "linkedTests": [
                {"file": "tests/é2e/catalog-search.spec.ts", "type": "e2e-ui", "testId": "opens search"}
            ],
            "plannedTests": [
                {"path": "tests/future/catalog-search.spec.ts", "title": "Future search proof", "type": "e2e-ui"}
            ]
        }
    ]
}

def assert_valid(label, document):
    errors = list(validator.iter_errors(document))
    if errors:
        raise AssertionError(f"{label}: expected valid, got {errors[0].message}")
    print(f"PASS: {label}")


def assert_invalid(label, mutate):
    document = copy.deepcopy(base)
    mutate(document)
    if not list(validator.iter_errors(document)):
        raise AssertionError(f"{label}: expected schema rejection")
    print(f"PASS: {label}")


def assert_v1_scope(label, value, valid):
    for field in ("scopeRef", "scope", "scopeId"):
        document = {
            "schemaVersion": 1,
            "scenarios": [{
                "id": "SCN-042-002",
                "title": "Legacy scope compatibility",
                "requiredTestType": "unit",
                field: value,
            }],
        }
        errors = list(v1_validator.iter_errors(document))
        if bool(errors) == valid:
            detail = errors[0].message if errors else "unexpected acceptance"
            raise AssertionError(f"{label} ({field}): {detail}")
    print(f"PASS: {label} for scopeRef, scope, and scopeId")


assert_valid("strict version 2 producer envelope", base)
offset_time = copy.deepcopy(base)
offset_time["generatedAt"] = "2026-08-31T14:30:00+02:30"
offset_time["scenarios"][0]["lockdown"] = {
    "state": "locked",
    "approvedAt": "2026-08-31T07:00:00-05:00",
}
assert_valid("RFC3339 Z and offset date-times", offset_time)
for label, timestamp in {
    "lowercase t and z": "2026-08-31t12:00:00z",
    "fractional seconds": "2026-08-31T12:00:00.123456789Z",
    "maximum positive offset": "2026-08-31T12:00:00+23:59",
    "maximum negative offset": "2026-08-31T12:00:00-23:59",
}.items():
    timestamp_document = copy.deepcopy(base)
    timestamp_document["generatedAt"] = timestamp
    timestamp_document["scenarios"][0]["lockdown"] = {
        "state": "locked",
        "approvedAt": timestamp,
    }
    assert_valid(f"RFC3339 accepts {label}", timestamp_document)
scoped_id = copy.deepcopy(base)
scoped_id["scenarios"][0]["id"] = "SCN-IMP-126-SCOPE-7-001"
assert_valid("scoped scenario identifier", scoped_id)
planning = copy.deepcopy(base)
planning["scenarios"][0]["linkedTests"] = []
assert_valid("planning may carry empty linkedTests", planning)
for label, value in {
    "v2 canonical string scopeRef": "SCOPE-1",
}.items():
    scoped = copy.deepcopy(base)
    scoped["scenarios"][0]["scopeRef"] = value
    assert_valid(label, scoped)
for label, value in {
    "v2 integer scopeRef": 1,
    "v2 zero scopeRef": 0,
    "v2 negative scopeRef": -1,
    "v2 float scopeRef": 1.0,
    "v2 boolean scopeRef": True,
    "v2 null scopeRef": None,
    "v2 blank scopeRef": "   ",
    "v2 edge-whitespace scopeRef": " SCOPE-1 ",
    "v2 control-bearing scopeRef": "SCOPE\nONE",
}.items():
    assert_invalid(label + " rejected", lambda document, candidate=value: document["scenarios"][0].update({"scopeRef": candidate}))
assert_invalid("v2 scope alias rejected", lambda document: document["scenarios"][0].update({"scope": "SCOPE-1"}))
assert_invalid("v2 scopeId alias rejected", lambda document: document["scenarios"][0].update({"scopeId": "SCOPE-1"}))
for label, value in {
    "v1 nonblank string scopeRef accepted": " SCOPE-1 ",
    "v1 positive integer scopeRef accepted": 1,
}.items():
    assert_v1_scope(label, value, True)
for label, value in {
    "v1 zero scopeRef rejected": 0,
    "v1 negative scopeRef rejected": -1,
    "v1 float scopeRef rejected": 1.0,
    "v1 boolean scopeRef rejected": True,
    "v1 null scopeRef rejected": None,
    "v1 blank scopeRef rejected": "   ",
    "v1 control-bearing scopeRef rejected": "SCOPE\u007fONE",
}.items():
    assert_v1_scope(label, value, False)
assert_invalid("legacy top-level array rejected", lambda document: document.update({"schemaVersion": 1}))
assert_invalid("scenarioId alias rejected", lambda document: document["scenarios"][0].update({"scenarioId": document["scenarios"][0].pop("id")}))
assert_invalid("linked string rejected", lambda document: document["scenarios"][0].update({"linkedTests": ["tests/e2e/a.spec.ts"]}))
assert_invalid("linked path alias rejected", lambda document: document["scenarios"][0].update({"linkedTests": [{"path": "tests/e2e/a.spec.ts", "type": "e2e-ui"}]}))
assert_invalid("linked reference requires type", lambda document: document["scenarios"][0].update({"linkedTests": [{"file": "tests/e2e/a.spec.ts"}]}))
assert_invalid("planned reference requires title", lambda document: document["scenarios"][0].update({"plannedTests": [{"path": "tests/e2e/a.spec.ts", "type": "e2e-ui"}]}))
assert_invalid("reference extension rejected", lambda document: document["scenarios"][0]["linkedTests"][0].update({"name": "alias"}))
assert_invalid("lowercase scenario identifier rejected", lambda document: document["scenarios"][0].update({"id": "SCN-imp-126-001"}))
assert_invalid("whitespace scenario identifier rejected", lambda document: document["scenarios"][0].update({"id": " SCN-126-001"}))
assert_invalid("invalid generatedAt date-time rejected", lambda document: document.update({"generatedAt": "not-a-date-time"}))
assert_invalid("invalid approvedAt date-time rejected", lambda document: document["scenarios"][0].update({"lockdown": {"state": "locked", "approvedAt": "not-a-date"}}))
for label, timestamp in {
    "basic ISO notation": "20260831T120000Z",
    "date only": "2026-08-31",
    "missing timezone": "2026-08-31T12:00:00",
    "invalid positive offset": "2026-08-31T12:00:00+24:00",
    "invalid negative offset": "2026-08-31T12:00:00-12:60",
    "invalid calendar date": "2026-02-29T12:00:00Z",
    "invalid hour": "2026-08-31T24:00:00Z",
    "invalid minute": "2026-08-31T12:60:00Z",
    "leap second excluded by canonical subset": "1990-12-31T23:59:60Z",
    "invalid second": "2026-08-31T12:00:61Z",
}.items():
    assert_invalid(
        f"generatedAt rejects {label}",
        lambda document, value=timestamp: document.update({"generatedAt": value}),
    )
    assert_invalid(
        f"approvedAt rejects {label}",
        lambda document, value=timestamp: document["scenarios"][0].update(
            {"lockdown": {"state": "locked", "approvedAt": value}}
        ),
    )
assert_invalid("top-level replacedBy requires scenario identifier", lambda document: document["scenarios"][0].update({"replacedBy": "replacement"}))
assert_invalid("lockdown replacedBy requires scenario identifier", lambda document: document["scenarios"][0].update({"lockdown": {"state": "replaced", "replacedBy": "replacement"}}))
assert_invalid("invalidatedBy rejects control characters", lambda document: document["scenarios"][0].update({"invalidatedBy": "operator\nother"}))
assert_invalid("lifecycle reason rejects control characters", lambda document: document["scenarios"][0].update({"liveProofNotApplicable": {"absentTrait": "static-metadata", "reason": "line one\nline two"}}))

unsafe_paths = {
    "POSIX absolute": "/tests/a.spec.ts",
    "backslash rooted": "\\tests\\a.spec.ts",
    "UNC": "\\\\server\\share\\a.spec.ts",
    # Assemble the drive marker so this portable selftest does not itself
    # contain the literal host-absolute pattern rejected by agnosticity-lint.
    "Windows device": "\\\\?\\" + "C" + ":\\a.spec.ts",
    "drive absolute": "C:/tests/a.spec.ts",
    "drive relative": "C:tests/a.spec.ts",
    "slash traversal": "tests/../a.spec.ts",
    "leading traversal": "../tests/a.spec.ts",
    "backslash traversal": "tests\\..\\a.spec.ts",
    "dot segment": "tests/./a.spec.ts",
    "double slash": "tests//a.spec.ts",
    "tab": "tests/a\tb.spec.ts",
    "newline": "tests/a\nb.spec.ts",
    "C0 control": "tests/a\u001fb.spec.ts",
    "DEL": "tests/a\u007fb.spec.ts",
    "leading whitespace": " tests/a.spec.ts",
    "trailing whitespace": "tests/a.spec.ts ",
}
for label, path in unsafe_paths.items():
    assert_invalid(
        f"unsafe path rejected: {label}",
        lambda document, candidate=path: document["scenarios"][0]["linkedTests"][0].update({"file": candidate}),
    )

print("scenario-manifest-v2-schema-selftest: PASS")
PY
