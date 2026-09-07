#!/usr/bin/env python3
"""Read and validate authored and planned scenario test references.

The output is one JSON document. Paths are normalized repo-relative POSIX paths.
Authored references also carry one canonical real path and an existence bit.
"""

from __future__ import annotations

import argparse
import json
import ntpath
import os
import posixpath
import re
import stat
import sys
from datetime import date
from typing import Any

CANONICAL_TEST_TYPES = {
    "unit",
    "functional",
    "integration",
    "ui-unit",
    "e2e-api",
    "e2e-ui",
    "stress",
    "load",
}
SCENARIO_ID_RE = re.compile(r"^SCN-[A-Z0-9]+(?:-[A-Z0-9]+)*-[0-9]+$")
CONTROL_RE = re.compile(r"[\x00-\x1f\x7f]")
DRIVE_RE = re.compile(r"^[A-Za-z]:")
PLANNED_SENTINELS = {"planned", "planned-not-authored", "not-authored"}
PLANNED_PATH_SENTINELS = {"__FUTURE_TEST__"}
V2_SCENARIO_FIELDS = {
    "id",
    "scopeRef",
    "title",
    "gherkin",
    "gherkinHash",
    "behaviorClass",
    "changeType",
    "requiredTestType",
    "regressionRequired",
    "lockdown",
    "linkedTests",
    "plannedTests",
    "evidenceRefs",
    "replacedBy",
    "invalidatedBy",
    "tags",
    "riskTier",
    "behaviorTraits",
    "liveProofNotApplicable",
    "obligations",
    "implementationRefs",
    "testMechanism",
    "invariantRefs",
}
V2_SCHEMA_PATH = "bubbles/schemas/scenario-manifest-v2.schema.json"
RFC3339_DATE_TIME = re.compile(
    r"^(?P<year>[0-9]{4})-(?P<month>[0-9]{2})-(?P<day>[0-9]{2})[Tt]"
    r"(?P<hour>[0-9]{2}):(?P<minute>[0-9]{2}):(?P<second>[0-9]{2})"
    r"(?:\.[0-9]+)?(?:[Zz]|(?P<offset_sign>[+-])"
    r"(?P<offset_hour>[0-9]{2}):(?P<offset_minute>[0-9]{2}))$"
)


class ReferenceError(ValueError):
    pass


def reject_duplicate_members(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    document: dict[str, Any] = {}
    for key, value in pairs:
        if key in document:
            raise ReferenceError(f"duplicate JSON member {key!r}")
        document[key] = value
    return document


def scenario_identity(
    scenario: dict[str, Any], index: int, strict_v2: bool
) -> tuple[str, dict[str, Any]]:
    if strict_v2 and "scenarioId" in scenario:
        raise ReferenceError(f"scenario[{index}] version 2 does not permit scenarioId")
    values: list[tuple[str, str]] = []
    for key in (("id",) if strict_v2 else ("id", "scenarioId")):
        if key not in scenario:
            continue
        value = scenario[key]
        if not isinstance(value, str):
            raise ReferenceError(f"scenario[{index}] identity alias '{key}' must be a string")
        if CONTROL_RE.search(value):
            raise ReferenceError(f"scenario[{index}] identity alias '{key}' contains a control character")
        normalized = value.strip()
        if not SCENARIO_ID_RE.fullmatch(normalized):
            raise ReferenceError(
                f"scenario[{index}] identity alias '{key}' does not match {SCENARIO_ID_RE.pattern}"
            )
        if strict_v2 and value != normalized:
            raise ReferenceError(
                f"scenario[{index}] version 2 id must not have leading or trailing whitespace"
            )
        values.append((key, normalized))
    if values:
        if len({value for _, value in values}) != 1:
            raise ReferenceError(f"scenario[{index}] has conflicting aliases (id, scenarioId)")
        selected_field, selected_value = values[0]
        return selected_value, {
            "canonicalId": selected_value,
            "authoredField": selected_field,
            "authoredValue": scenario[selected_field],
            "authoredAliases": {
                field: scenario[field]
                for field in ("id", "scenarioId")
                if field in scenario
            },
            "validAuthoredAliases": [
                {"field": field, "value": value} for field, value in values
            ],
        }
    label = "id" if strict_v2 else "id or scenarioId"
    raise ReferenceError(f"scenario[{index}] has no valid {label} matching {SCENARIO_ID_RE.pattern}")


def diagnostic(sid: str, field: str, index: int, message: str) -> ReferenceError:
    return ReferenceError(f"scenario {sid} {field}[{index}]: {message}")


def projected_scope_ref(
    scenario: dict[str, Any], sid: str, strict_v2: bool
) -> str | None:
    if strict_v2:
        legacy_aliases = [field for field in ("scope", "scopeId") if field in scenario]
        if legacy_aliases:
            raise ReferenceError(
                f"scenario {sid}: version 2 does not permit legacy scope alias(es): "
                f"{', '.join(legacy_aliases)}"
            )
        if "scopeRef" not in scenario:
            return None
        value = scenario["scopeRef"]
        if not isinstance(value, str):
            raise ReferenceError(f"scenario {sid}: version 2 scopeRef must be a string")
        if CONTROL_RE.search(value):
            raise ReferenceError(f"scenario {sid}: scopeRef contains a control character")
        normalized = value.strip()
        if not normalized:
            raise ReferenceError(f"scenario {sid}: scopeRef is blank")
        if value != normalized:
            raise ReferenceError(
                f"scenario {sid}: version 2 scopeRef must not have leading or trailing whitespace"
            )
        return value

    present = [
        (field, scenario[field])
        for field in ("scopeRef", "scope", "scopeId")
        if field in scenario
    ]
    if not present:
        return None
    normalized: list[tuple[str, str]] = []
    for field, value in present:
        if isinstance(value, str):
            if CONTROL_RE.search(value):
                raise ReferenceError(f"scenario {sid}: scope alias '{field}' contains a control character")
            projected = value.strip()
            if not projected:
                raise ReferenceError(f"scenario {sid}: scope alias '{field}' is blank")
        elif type(value) is int and value > 0:
            projected = str(value)
        else:
            raise ReferenceError(
                f"scenario {sid}: scope alias '{field}' must be a nonblank string or positive integer"
            )
        normalized.append((field, projected))
    if len({value for _, value in normalized}) != 1:
        names = ", ".join(field for field, _ in normalized)
        raise ReferenceError(f"scenario {sid}: conflicting scope aliases ({names})")
    return normalized[0][1]


def normalize_aliases(
    reference: dict[str, Any], aliases: tuple[str, ...], sid: str, field: str, index: int
) -> str | None:
    present: list[tuple[str, str]] = []
    for alias in aliases:
        if alias not in reference:
            continue
        value = reference[alias]
        if not isinstance(value, str):
            raise diagnostic(sid, field, index, f"alias '{alias}' must be a string")
        if CONTROL_RE.search(value):
            raise diagnostic(sid, field, index, f"alias '{alias}' contains a control character")
        normalized = value.strip()
        if not normalized:
            raise diagnostic(sid, field, index, f"alias '{alias}' is blank")
        present.append((alias, normalized))
    if not present:
        return None
    distinct = {value for _, value in present}
    if len(distinct) != 1:
        names = ", ".join(name for name, _ in present)
        raise diagnostic(sid, field, index, f"conflicting aliases ({names})")
    return present[0][1]


def normalize_path(path: str, sid: str, field: str, index: int) -> str:
    if CONTROL_RE.search(path):
        raise diagnostic(sid, field, index, "path contains a control character")
    if "\\" in path:
        raise diagnostic(sid, field, index, "path must use repository-relative POSIX separators")
    if path.startswith("/") or path.startswith("//"):
        raise diagnostic(sid, field, index, "path must be repository-relative")
    if DRIVE_RE.match(path) or ntpath.splitdrive(path)[0]:
        raise diagnostic(sid, field, index, "Windows drive/device paths are not permitted")
    segments = path.split("/")
    if ".." in segments:
        raise diagnostic(sid, field, index, "path contains lexical traversal")
    if "." in segments:
        raise diagnostic(sid, field, index, "path contains a dot segment")
    if "" in segments:
        raise diagnostic(sid, field, index, "path contains an empty segment")
    normalized = posixpath.normpath(path)
    if normalized in ("", "."):
        raise diagnostic(sid, field, index, "path does not identify a file")
    return normalized


def canonical_repository_root(repo_root: str) -> str:
    root_real = os.path.realpath(repo_root)
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_CLOEXEC", 0)
    try:
        descriptor = os.open(root_real, flags)
    except OSError as exc:
        raise ReferenceError(f"repository root is not an accessible real directory: {exc}") from exc
    try:
        if not stat.S_ISDIR(os.fstat(descriptor).st_mode):
            raise ReferenceError("repository root descriptor is not a directory")
    finally:
        os.close(descriptor)
    return root_real


def open_directory_at(parent_descriptor: int, component: str, display_path: str) -> int:
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_CLOEXEC", 0)
    try:
        descriptor = os.open(component, flags, dir_fd=parent_descriptor)
    except OSError as exc:
        raise OSError(exc.errno, f"cannot open directory component without following links: {display_path}") from exc
    try:
        if not stat.S_ISDIR(os.fstat(descriptor).st_mode):
            raise OSError(f"directory component is not a directory: {display_path}")
        return descriptor
    except BaseException:
        os.close(descriptor)
        raise


def open_relative_regular_file(root_real: str, relative_path: str) -> tuple[int, int]:
    components = relative_path.split("/")
    root_flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    root_flags |= getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_CLOEXEC", 0)
    root_descriptor = os.open(root_real, root_flags)
    parent_descriptor = root_descriptor
    try:
        for offset, component in enumerate(components[:-1]):
            next_descriptor = open_directory_at(
                parent_descriptor, component, "/".join(components[: offset + 1])
            )
            if parent_descriptor != root_descriptor:
                os.close(parent_descriptor)
            parent_descriptor = next_descriptor
        file_flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0)
        file_flags |= getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NONBLOCK", 0)
        file_descriptor = os.open(components[-1], file_flags, dir_fd=parent_descriptor)
        metadata = os.fstat(file_descriptor)
        if not stat.S_ISREG(metadata.st_mode):
            os.close(file_descriptor)
            raise OSError(f"path is not a regular file: {relative_path}")
        return file_descriptor, root_descriptor
    except BaseException:
        os.close(root_descriptor)
        raise
    finally:
        if parent_descriptor != root_descriptor:
            os.close(parent_descriptor)


def repository_relative_manifest_path(root_real: str, manifest_path: str) -> str:
    if CONTROL_RE.search(manifest_path):
        raise OSError("manifest path contains a control character")
    if os.path.isabs(manifest_path):
        candidate = os.path.abspath(manifest_path)
    else:
        candidate = os.path.abspath(os.path.join(root_real, manifest_path))
    try:
        contained = os.path.commonpath((root_real, candidate)) == root_real
    except ValueError:
        contained = False
    if not contained:
        raise OSError("manifest path is outside the repository root")
    relative_path = os.path.relpath(candidate, root_real)
    if relative_path in ("", "."):
        raise OSError("manifest path does not identify a file")
    return relative_path


def load_manifest_document(repo_root: str, manifest_path: str) -> Any:
    root_real = canonical_repository_root(repo_root)
    relative_path = repository_relative_manifest_path(root_real, manifest_path)
    descriptor = None
    root_descriptor = None
    try:
        descriptor, root_descriptor = open_relative_regular_file(root_real, relative_path)
        with os.fdopen(os.dup(descriptor), "rb") as handle:
            manifest_bytes = handle.read()
        return json.loads(manifest_bytes, object_pairs_hook=reject_duplicate_members)
    finally:
        if descriptor is not None:
            os.close(descriptor)
        if root_descriptor is not None:
            os.close(root_descriptor)


def verify_pathname_identity(
    root_real: str, relative_path: str, expected_metadata: os.stat_result
) -> None:
    verification_descriptor = None
    verification_root = None
    try:
        verification_descriptor, verification_root = open_relative_regular_file(
            root_real, relative_path
        )
        observed = os.fstat(verification_descriptor)
        if (observed.st_dev, observed.st_ino) != (
            expected_metadata.st_dev,
            expected_metadata.st_ino,
        ):
            raise OSError("authored pathname identity changed during verification")
    finally:
        if verification_descriptor is not None:
            os.close(verification_descriptor)
        if verification_root is not None:
            os.close(verification_root)


def canonical_authored_path(
    repo_root: str, path: str, sid: str, field: str, index: int
) -> tuple[str, str, dict[str, int]]:
    root_real = canonical_repository_root(repo_root)
    candidate = os.path.abspath(os.path.join(root_real, *path.split("/")))
    try:
        contained = os.path.commonpath((root_real, candidate)) == root_real
    except ValueError:
        contained = False
    if not contained:
        raise diagnostic(sid, field, index, "authored path is outside the repository root")
    descriptor = None
    root_descriptor = None
    try:
        descriptor, root_descriptor = open_relative_regular_file(root_real, path)
        metadata = os.fstat(descriptor)
        verify_pathname_identity(root_real, path, metadata)
    except OSError as exc:
        raise diagnostic(
            sid, field, index,
            f"authored path is not an existing stable regular file: {exc}",
        ) from exc
    finally:
        if descriptor is not None:
            os.close(descriptor)
        if root_descriptor is not None:
            os.close(root_descriptor)
    return candidate, path, {
        "device": metadata.st_dev,
        "inode": metadata.st_ino,
        "mode": stat.S_IMODE(metadata.st_mode),
        "size": metadata.st_size,
        "modifiedNanoseconds": metadata.st_mtime_ns,
    }


def is_rfc3339_date_time(value: Any) -> bool:
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


def load_v2_validator(repo_root: str) -> Any:
    try:
        from jsonschema import Draft7Validator, FormatChecker
        from jsonschema.exceptions import SchemaError
    except ImportError as exc:
        raise ReferenceError("version 2 schema validation requires python jsonschema") from exc

    root_real = canonical_repository_root(repo_root)
    descriptor = None
    root_descriptor = None
    try:
        descriptor, root_descriptor = open_relative_regular_file(root_real, V2_SCHEMA_PATH)
        with os.fdopen(os.dup(descriptor), "r", encoding="utf-8") as handle:
            schema = json.load(handle, object_pairs_hook=reject_duplicate_members)
        metadata = os.fstat(descriptor)
        verify_pathname_identity(root_real, V2_SCHEMA_PATH, metadata)
        Draft7Validator.check_schema(schema)
    except (OSError, ValueError, SchemaError) as exc:
        raise ReferenceError(
            f"version 2 schema is unavailable or invalid under repository root: {exc}"
        ) from exc
    finally:
        if descriptor is not None:
            os.close(descriptor)
        if root_descriptor is not None:
            os.close(root_descriptor)
    checker = FormatChecker()
    checker.checks("date-time")(is_rfc3339_date_time)
    return Draft7Validator(schema, format_checker=checker)


def validate_v2_document(document: dict[str, Any], repo_root: str) -> None:
    validator = load_v2_validator(repo_root)
    errors = sorted(
        validator.iter_errors(document),
        key=lambda error: tuple(str(part) for part in error.absolute_path),
    )
    if not errors:
        return
    first = errors[0]
    location = "/".join(str(part) for part in first.absolute_path) or "<root>"
    raise ReferenceError(f"version 2 validation failed at {location}: {first.message}")


def legacy_sentinel_classification(path: str) -> str | None:
    normalized = path.strip()
    if normalized.lower() in PLANNED_SENTINELS:
        return "legacy-planned-explicit"
    if normalized in PLANNED_PATH_SENTINELS:
        return "legacy-planned-sentinel"
    return None


def reject_unknown_sentinel(path: str, sid: str, field: str, index: int) -> None:
    normalized = path.strip()
    if len(normalized) >= 4 and normalized.startswith("__") and normalized.endswith("__"):
        raise diagnostic(sid, field, index, f"unknown legacy sentinel '{normalized}'")


def parse_reference(
    reference: Any,
    *,
    sid: str,
    field: str,
    index: int,
    repo_root: str,
    planned_field: bool,
    strict_v2: bool,
    scenario_title: str | None,
    required_type: str | None,
) -> dict[str, Any]:
    legacy_planned = False
    if isinstance(reference, str):
        if planned_field or strict_v2:
            message = "canonical planned reference must be an object" if planned_field else "version 2 reference must be an object"
            raise diagnostic(sid, field, index, message)
        if CONTROL_RE.search(reference):
            raise diagnostic(sid, field, index, "reference contains a control character")
        text = reference.strip()
        if not text:
            raise diagnostic(sid, field, index, "reference is blank")
        path_text, separator, title_text = text.partition("#")
        path = path_text.strip()
        title = title_text.strip() if separator else None
        if separator and not title:
            raise diagnostic(sid, field, index, "title after '#' is blank")
        test_type = None
    elif isinstance(reference, dict):
        if strict_v2:
            allowed = {"path", "title", "type"} if planned_field else {"file", "type", "testId"}
            unexpected = sorted(set(reference) - allowed)
            if unexpected:
                raise diagnostic(
                    sid,
                    field,
                    index,
                    f"version 2 reference has unsupported field(s): {', '.join(unexpected)}",
                )
        if strict_v2 and planned_field and "path" not in reference:
            raise diagnostic(sid, field, index, "version 2 planned reference requires the 'path' field")
        if strict_v2 and not planned_field and "file" not in reference:
            raise diagnostic(sid, field, index, "version 2 authored reference requires the 'file' field")
        path = normalize_aliases(reference, ("file", "path"), sid, field, index)
        title = normalize_aliases(reference, ("title", "testId", "name"), sid, field, index)
        test_type = normalize_aliases(reference, ("type",), sid, field, index)
        state = reference.get("testState")
        if state is not None:
            if not isinstance(state, str) or CONTROL_RE.search(state):
                raise diagnostic(sid, field, index, "testState must be a control-free string")
            if state.strip().lower() not in PLANNED_SENTINELS:
                raise diagnostic(sid, field, index, f"unknown testState '{state.strip()}'")
            legacy_planned = True
        if path is None:
            raise diagnostic(sid, field, index, "reference has no file/path alias")
    else:
        raise diagnostic(sid, field, index, "reference must be a string or object")

    if not path:
        raise diagnostic(sid, field, index, "reference path is blank")
    sentinel_classification = legacy_sentinel_classification(path) if not strict_v2 else None
    if sentinel_classification is None:
        reject_unknown_sentinel(path, sid, field, index)
    sentinel = sentinel_classification is not None
    planned = planned_field or legacy_planned or sentinel
    if planned_field and sentinel:
        raise diagnostic(sid, field, index, "canonical planned path cannot use the legacy sentinel")
    if strict_v2 and (legacy_planned or sentinel):
        raise diagnostic(sid, field, index, "version 2 does not permit legacy planned sentinels or testState")
    if planned_field:
        if title is None:
            raise diagnostic(sid, field, index, "canonical planned reference requires nonblank title")
        if test_type is None:
            raise diagnostic(sid, field, index, "canonical planned reference requires nonblank type")
        if test_type not in CANONICAL_TEST_TYPES:
            raise diagnostic(sid, field, index, f"planned type '{test_type}' is not canonical")
    elif planned:
        title = title or scenario_title
        test_type = test_type or required_type
        if title is None:
            raise diagnostic(sid, field, index, "planned reference requires nonblank title")
        if test_type is None or test_type not in CANONICAL_TEST_TYPES:
            raise diagnostic(sid, field, index, "planned reference requires canonical type")
    elif strict_v2:
        if test_type is None:
            raise diagnostic(sid, field, index, "version 2 authored reference requires nonblank type")
        if test_type not in CANONICAL_TEST_TYPES:
            raise diagnostic(sid, field, index, f"authored type '{test_type}' is not canonical")
    if sentinel:
        return {
            "field": field,
            "index": index,
            "kind": "planned",
            "sourceClassification": sentinel_classification or "legacy-test-state",
            "path": path,
            "title": title,
            "type": test_type,
            "sentinel": True,
            "canonicalPath": None,
            "canonicalRelativePath": None,
            "identity": None,
            "exists": False,
        }

    normalized = normalize_path(path, sid, field, index)
    canonical_path = None
    canonical_relative_path = None
    identity = None
    exists = False
    if not planned:
        canonical_path, canonical_relative_path, identity = canonical_authored_path(
            repo_root, normalized, sid, field, index
        )
        exists = True
    return {
        "field": field,
        "index": index,
        "kind": "planned" if planned else "authored",
        "sourceClassification": "canonical-v2" if strict_v2 else (
            "legacy-test-state" if legacy_planned else "legacy-v1"
        ),
        "path": normalized,
        "title": title,
        "type": test_type,
        "sentinel": False,
        "canonicalPath": canonical_path,
        "canonicalRelativePath": canonical_relative_path,
        "identity": identity,
        "exists": exists,
    }


def load_manifest(
    path: str | None, use_stdin: bool, repo_root: str
) -> tuple[list[dict[str, Any]], bool, int, str]:
    try:
        if use_stdin:
            document = json.load(sys.stdin, object_pairs_hook=reject_duplicate_members)
        else:
            assert path is not None
            document = load_manifest_document(repo_root, path)
    except (OSError, ValueError) as exc:
        raise ReferenceError(f"cannot parse scenario manifest: {exc}") from exc
    strict_v2 = False
    if isinstance(document, dict):
        if "schemaVersion" not in document:
            raise ReferenceError("scenario manifest object is missing required schemaVersion")
        version = document["schemaVersion"]
        if type(version) is not int:
            raise ReferenceError(
                f"scenario manifest schemaVersion must be an int, got {type(version).__name__}"
            )
        if version not in (1, 2):
            raise ReferenceError(f"unsupported scenario manifest schemaVersion: {version!r}")
        strict_v2 = version == 2
        if strict_v2:
            validate_v2_document(document, repo_root)
        scenarios = document.get("scenarios")
        envelope = f"schema-version-{version}"
    else:
        scenarios = document
        version = 1
        envelope = "legacy-top-level-array"
    if not isinstance(scenarios, list):
        raise ReferenceError("scenario manifest must be an object with scenarios[] or a legacy top-level array")
    return scenarios, strict_v2, version, envelope


def read_references(
    manifest_path: str | None, repo_root: str, use_stdin: bool = False
) -> dict[str, Any]:
    result: list[dict[str, Any]] = []
    scenarios, strict_v2, version, envelope = load_manifest(
        manifest_path, use_stdin, repo_root
    )
    seen: set[str] = set()
    for scenario_index, scenario in enumerate(scenarios):
        if not isinstance(scenario, dict):
            raise ReferenceError(f"scenario <scenario[{scenario_index}]>: scenario entry must be an object")
        if strict_v2:
            unexpected = sorted(set(scenario) - V2_SCENARIO_FIELDS)
            if unexpected:
                raise ReferenceError(
                    f"scenario[{scenario_index}] version 2 has unsupported field(s): {', '.join(unexpected)}"
                )
        sid, identity = scenario_identity(scenario, scenario_index, strict_v2)
        if sid in seen:
            raise ReferenceError(
                f"scenario {sid}: duplicate effective scenario id at index {scenario_index}"
            )
        seen.add(sid)
        if strict_v2 and "linkedTestContracts" in scenario:
            raise ReferenceError(f"scenario {sid}: version 2 does not permit linkedTestContracts")
        references: list[dict[str, Any]] = []
        authored_field = None
        if "linkedTests" in scenario and "linkedTestContracts" in scenario:
            if scenario["linkedTests"] != scenario["linkedTestContracts"]:
                raise ReferenceError(
                    f"scenario {sid}: conflicting aliases (linkedTests, linkedTestContracts)"
                )
            authored_field = "linkedTests"
        elif "linkedTests" in scenario:
            authored_field = "linkedTests"
        elif "linkedTestContracts" in scenario:
            authored_field = "linkedTestContracts"
        fields = ([authored_field] if authored_field else []) + (
            ["plannedTests"] if "plannedTests" in scenario else []
        )
        for field in fields:
            assert field is not None
            if field not in scenario:
                continue
            values = scenario[field]
            if not isinstance(values, list):
                raise ReferenceError(f"scenario {sid} {field}: field must be an array")
            for index, reference in enumerate(values):
                references.append(
                    parse_reference(
                        reference,
                        sid=sid,
                        field=field,
                        index=index,
                        repo_root=repo_root,
                        planned_field=field == "plannedTests",
                        strict_v2=strict_v2,
                        scenario_title=(
                            scenario.get("title") if isinstance(scenario.get("title"), str) else None
                        ),
                        required_type=(
                            scenario.get("requiredTestType")
                            if isinstance(scenario.get("requiredTestType"), str) else None
                        ),
                    )
                )
        scope_ref = projected_scope_ref(scenario, sid, strict_v2)
        metadata = {"scopeRef": scope_ref}
        metadata.update(
            (key, value)
            for key, value in scenario.items()
            if key not in {
                "id", "scenarioId", "scopeRef", "scope", "scopeId",
                "linkedTests", "linkedTestContracts", "plannedTests",
            }
        )
        for key, default in (
            ("title", None),
            ("requiredTestType", None),
            ("evidenceRefs", []),
            ("implementationRefs", []),
            ("invariantRefs", []),
            ("obligations", None),
            ("testMechanism", None),
        ):
            metadata.setdefault(key, default)
        result.append(
            {
                "scenarioId": sid,
                "scenarioIndex": scenario_index,
                "scopeRef": scope_ref,
                "title": metadata["title"],
                "requiredTestType": metadata["requiredTestType"],
                "evidenceRefs": metadata["evidenceRefs"],
                "implementationRefs": metadata["implementationRefs"],
                "invariantRefs": metadata["invariantRefs"],
                "obligations": metadata["obligations"],
                "testMechanism": metadata["testMechanism"],
                "authoredIdentity": identity,
                "metadata": metadata,
                "references": references,
            }
        )
    return {
        "projectionVersion": 1,
        "manifestSchemaVersion": version,
        "sourceEnvelope": envelope,
        "scenarios": result,
    }


def main() -> int:
    parser = argparse.ArgumentParser(add_help=False)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("manifest", nargs="?")
    source.add_argument("--stdin", action="store_true")
    parser.add_argument("--repo-root", required=True)
    parser.add_argument("-h", "--help", action="help")
    args = parser.parse_args()
    try:
        payload = read_references(args.manifest, args.repo_root, args.stdin)
    except ReferenceError as exc:
        print(f"scenario-reference-reader: {exc}", file=sys.stderr)
        return 2
    json.dump(payload, sys.stdout, ensure_ascii=False, separators=(",", ":"))
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
