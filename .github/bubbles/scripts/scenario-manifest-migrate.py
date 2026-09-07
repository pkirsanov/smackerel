#!/usr/bin/env python3
"""Migrate losslessly resolvable scenario manifests to schema version 2."""

import argparse
import contextlib
import ctypes
import errno
import fcntl
import hashlib
import json
import os
import re
import secrets
import stat
import sys
import tempfile
from collections import OrderedDict
from datetime import date
from pathlib import Path

try:
    from jsonschema import Draft7Validator, FormatChecker
    from jsonschema.exceptions import SchemaError
except ImportError:
    print("scenario-manifest-migrate: REFUSED jsonschema is not installed", file=sys.stderr)
    sys.exit(2)


TEST_TYPES = {
    "unit",
    "functional",
    "integration",
    "ui-unit",
    "e2e-api",
    "e2e-ui",
    "stress",
    "load",
}
PLANNED_SENTINELS = {"planned", "planned-not-authored", "not-authored"}
CONTROL_RE = re.compile(r"[\x00-\x1f\x7f]")
SCENARIO_ID_RE = re.compile(r"^SCN-[A-Z0-9]+(?:-[A-Z0-9]+)*-[0-9]+$")
TOP_LEVEL_KEYS = {"schemaVersion", "version", "spec", "featureDir", "generatedAt", "scenarios"}
SCENARIO_KEYS = {
    "id",
    "scenarioId",
    "scopeRef",
    "scope",
    "scopeId",
    "title",
    "gherkin",
    "gherkinHash",
    "behaviorClass",
    "changeType",
    "requiredTestType",
    "regressionRequired",
    "lockdown",
    "linkedTests",
    "linkedTestContracts",
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
SCENARIO_FIELD_ORDER = [
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
]


class MigrationRefusal(ValueError):
    """A source manifest cannot be converted without losing or inventing data."""


def refuse(message):
    raise MigrationRefusal(message)


def reject_duplicate_members(pairs):
    document = {}
    for key, value in pairs:
        if key in document:
            refuse(f"duplicate JSON member {key!r}")
        document[key] = value
    return document


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


def format_checker():
    checker = FormatChecker()
    checker.checks("date-time")(is_rfc3339_date_time)
    return checker


def verify_private_directory(directory, label):
    try:
        metadata = directory.lstat()
    except OSError as error:
        refuse(f"cannot inspect {label}: {error}")
    mode = stat.S_IMODE(metadata.st_mode)
    if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISDIR(metadata.st_mode):
        refuse(f"{label} is not a real directory")
    if metadata.st_uid != os.getuid():
        refuse(f"{label} is not owned by the current uid")
    if mode & 0o077:
        refuse(f"{label} permissions are not private: {oct(mode)}")


def lock_directory():
    runtime_value = os.environ.get("XDG_RUNTIME_DIR")
    if runtime_value:
        runtime_directory = Path(runtime_value)
        try:
            verify_private_directory(runtime_directory, "XDG_RUNTIME_DIR")
        except MigrationRefusal:
            pass
        else:
            directory = runtime_directory / "bubbles-scenario-manifest-migrate-locks"
            try:
                directory.mkdir(mode=0o700)
            except FileExistsError:
                pass
            except OSError as error:
                refuse(f"cannot create advisory lock directory: {error}")
            verify_private_directory(directory, "advisory lock directory")
            return directory

    directory = Path(tempfile.gettempdir()) / f"bubbles-scenario-manifest-migrate-{os.getuid()}"
    try:
        directory.mkdir(mode=0o700)
    except FileExistsError:
        pass
    except OSError as error:
        refuse(f"cannot create fallback advisory lock directory: {error}")
    verify_private_directory(directory, "fallback advisory lock directory")
    return directory


def lock_path(path):
    absolute = Path(os.path.abspath(os.fspath(path)))
    canonical_parent = os.path.realpath(os.fspath(absolute.parent))
    canonical = os.path.join(canonical_parent, absolute.name)
    digest = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    return lock_directory() / f"{digest}.lock"


def open_lock_file(advisory_path):
    directory_flags = os.O_RDONLY
    directory_flags |= getattr(os, "O_DIRECTORY", 0)
    directory_flags |= getattr(os, "O_NOFOLLOW", 0)
    directory_flags |= getattr(os, "O_CLOEXEC", 0)
    try:
        directory_descriptor = os.open(advisory_path.parent, directory_flags)
    except OSError as error:
        refuse(f"cannot open advisory lock directory: {error}")
    try:
        directory_metadata = os.fstat(directory_descriptor)
        directory_mode = stat.S_IMODE(directory_metadata.st_mode)
        if not stat.S_ISDIR(directory_metadata.st_mode):
            refuse("advisory lock directory descriptor is not a directory")
        if directory_metadata.st_uid != os.getuid():
            refuse("advisory lock directory descriptor is not owned by the current uid")
        if directory_mode & 0o077:
            refuse(
                "advisory lock directory descriptor permissions are not private: "
                f"{oct(directory_mode)}"
            )

        flags = os.O_RDWR | os.O_CREAT
        flags |= getattr(os, "O_NOFOLLOW", 0)
        flags |= getattr(os, "O_CLOEXEC", 0)
        try:
            descriptor = os.open(advisory_path.name, flags, 0o600, dir_fd=directory_descriptor)
        except OSError as error:
            refuse(f"cannot open advisory lock: {error}")
    finally:
        os.close(directory_descriptor)

    try:
        metadata = os.fstat(descriptor)
        mode = stat.S_IMODE(metadata.st_mode)
        if not stat.S_ISREG(metadata.st_mode):
            refuse("advisory lock is not a regular file")
        if metadata.st_uid != os.getuid():
            refuse("advisory lock is not owned by the current uid")
        if mode & 0o177:
            refuse(f"advisory lock permissions are unsafe: {oct(mode)}")
        os.fchmod(descriptor, 0o600)
        return descriptor
    except BaseException:
        os.close(descriptor)
        raise


@contextlib.contextmanager
def migration_lock(path):
    advisory_path = lock_path(path)
    descriptor = open_lock_file(advisory_path)
    try:
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            refuse("another cooperating migrator holds the advisory lock")
        yield
    finally:
        os.close(descriptor)


def parse_json(source_bytes, label="manifest"):
    try:
        return json.loads(
            source_bytes.decode("utf-8"),
            object_pairs_hook=reject_duplicate_members,
        )
    except UnicodeDecodeError as error:
        refuse(f"{label} is not UTF-8: {error}")
    except json.JSONDecodeError as error:
        refuse(f"invalid {label} JSON at line {error.lineno}, column {error.colno}")


def open_directory_no_follow(path, label):
    absolute = Path(os.path.abspath(os.fspath(path)))
    flags = os.O_RDONLY
    flags |= getattr(os, "O_DIRECTORY", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0)
    flags |= getattr(os, "O_CLOEXEC", 0)
    try:
        descriptor = os.open(os.sep, flags)
    except OSError as error:
        refuse(f"cannot open filesystem root for {label}: {error}")
    try:
        for component in absolute.parts[1:]:
            try:
                next_descriptor = os.open(component, flags, dir_fd=descriptor)
            except OSError as error:
                refuse(f"cannot open {label} without following links: {error}")
            os.close(descriptor)
            descriptor = next_descriptor
        metadata = os.fstat(descriptor)
        if not stat.S_ISDIR(metadata.st_mode):
            refuse(f"{label} is not a directory")
        return descriptor
    except BaseException:
        os.close(descriptor)
        raise


def open_source_directory(path):
    flags = os.O_RDONLY
    flags |= getattr(os, "O_DIRECTORY", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0)
    flags |= getattr(os, "O_CLOEXEC", 0)
    try:
        descriptor = os.open(path.parent, flags)
    except OSError as error:
        refuse(f"cannot open source directory for {path}: {error}")
    try:
        metadata = os.fstat(descriptor)
        if not stat.S_ISDIR(metadata.st_mode):
            refuse(f"source directory for {path} is not a directory")
        return descriptor
    except BaseException:
        os.close(descriptor)
        raise


def read_snapshot(directory_descriptor, source_name, display_path):
    flags = os.O_RDONLY
    flags |= getattr(os, "O_NOFOLLOW", 0)
    flags |= getattr(os, "O_CLOEXEC", 0)
    flags |= getattr(os, "O_NONBLOCK", 0)
    try:
        descriptor = os.open(source_name, flags, dir_fd=directory_descriptor)
    except OSError as error:
        refuse(f"cannot open source {display_path} without following links: {error}")
    try:
        metadata = os.fstat(descriptor)
        if not stat.S_ISREG(metadata.st_mode):
            refuse(f"source {display_path} is not a regular file")
        with os.fdopen(descriptor, "rb") as handle:
            descriptor = None
            source_bytes = handle.read()
    except OSError as error:
        refuse(f"cannot read {display_path}: {error}")
    finally:
        if descriptor is not None:
            os.close(descriptor)
    identity = (metadata.st_dev, metadata.st_ino)
    return source_bytes, identity, stat.S_IMODE(metadata.st_mode)


def load_validator(script_path):
    schema_path = script_path.parent.parent / "schemas" / "scenario-manifest-v2.schema.json"
    directory_descriptor = open_directory_no_follow(
        schema_path.parent, "version 2 schema directory"
    )
    try:
        schema_bytes, _, _ = read_snapshot(
            directory_descriptor, schema_path.name, schema_path
        )
        schema = parse_json(schema_bytes, "version 2 schema")
        Draft7Validator.check_schema(schema)
    except (OSError, json.JSONDecodeError, SchemaError) as error:
        refuse(f"version 2 schema is unavailable or invalid: {error}")
    finally:
        os.close(directory_descriptor)
    return Draft7Validator(schema, format_checker=format_checker())


def validate_v2(document, validator):
    errors = sorted(validator.iter_errors(document), key=lambda error: list(error.absolute_path))
    if not errors:
        return
    first = errors[0]
    location = "/".join(str(part) for part in first.absolute_path) or "<root>"
    refuse(f"version 2 validation failed at {location}: {first.message}")


def choose_alias(record, canonical, alias, context, required=False):
    canonical_value = record.get(canonical)
    alias_value = record.get(alias)
    if canonical_value is not None and alias_value is not None and canonical_value != alias_value:
        refuse(f"{context} has conflicting {canonical} and {alias} values")
    value = canonical_value if canonical_value is not None else alias_value
    if required and value is None:
        refuse(f"{context} is missing {canonical}")
    return value


def normalize_string_aliases(record, aliases, context, required=False):
    present = []
    for name in aliases:
        if name not in record or record[name] is None:
            continue
        value = record[name]
        if not isinstance(value, str):
            refuse(f"{context} alias {name} must be a string")
        normalized = value.strip()
        if not normalized:
            refuse(f"{context} alias {name} is blank")
        present.append((name, normalized))
    if not present:
        if required:
            refuse(f"{context} is missing {aliases[0]}")
        return None
    if len({value for _, value in present}) != 1:
        refuse(f"{context} has conflicting {', '.join(name for name, _ in present)} values")
    return present[0][1]


def normalize_scenario_id_aliases(record, context):
    present = []
    for name in ("id", "scenarioId"):
        if name not in record:
            continue
        value = record[name]
        if not isinstance(value, str):
            refuse(f"{context} alias {name} must be a string")
        if CONTROL_RE.search(value):
            refuse(f"{context} alias {name} contains a control character")
        normalized = value.strip()
        if not normalized:
            refuse(f"{context} alias {name} is blank")
        if SCENARIO_ID_RE.fullmatch(normalized) is None:
            refuse(f"{context} alias {name} is not a canonical scenario identifier")
        present.append((name, normalized))
    if not present:
        refuse(f"{context} is missing id")
    if len({value for _, value in present}) != 1:
        refuse(f"{context} has conflicting {', '.join(name for name, _ in present)} values")
    return present[0][1]


def normalize_scope_aliases(record, context):
    present = []
    for name in ("scopeRef", "scope", "scopeId"):
        if name not in record:
            continue
        value = record[name]
        if isinstance(value, str):
            if CONTROL_RE.search(value):
                refuse(f"{context} alias {name} contains a control character")
            normalized = value.strip()
            if not normalized:
                refuse(f"{context} alias {name} is blank")
        elif type(value) is int and value > 0:
            normalized = str(value)
        else:
            refuse(f"{context} alias {name} must be a nonblank string or positive integer")
        present.append((name, normalized))
    if not present:
        return None
    if len({value for _, value in present}) != 1:
        refuse(f"{context} has conflicting {', '.join(name for name, _ in present)} values")
    return present[0][1]


def require_test_type(value, context):
    if value not in TEST_TYPES:
        refuse(f"{context} is missing a canonical test type")
    return value


def migrate_linked_test(reference, required_type, context):
    if isinstance(reference, str):
        normalized = reference.strip()
        if normalized.lower() in PLANNED_SENTINELS or (
            normalized.startswith("__") and normalized.endswith("__")
        ):
            refuse(f"{context} uses a planned sentinel string")
        if not normalized:
            refuse(f"{context} has a blank authored path")
        if "#" not in normalized:
            return OrderedDict((("file", normalized), ("type", required_type)))
        file_value, test_id = reference.split("#", 1)
        file_value = file_value.strip()
        test_id = test_id.strip()
        if not file_value:
            refuse(f"{context} has a blank authored path before the fragment")
        if not test_id:
            refuse(f"{context} has a blank authored test fragment")
        return OrderedDict(
            (("file", file_value), ("type", required_type), ("testId", test_id))
        )
    if not isinstance(reference, dict):
        refuse(f"{context} is not a string or object")

    test_state = reference.get("testState")
    if test_state is not None:
        if test_state != "planned-not-authored":
            refuse(f"{context} has unsupported testState {test_state!r}")
        allowed = {"file", "path", "title", "type", "testState"}
        extras = set(reference) - allowed
        if extras:
            refuse(f"{context} has fields that version 2 plannedTests cannot preserve: {sorted(extras)}")
        path = normalize_string_aliases(
            reference, ("path", "file"), context, required=True
        )
        title = normalize_string_aliases(reference, ("title",), context, required=True)
        test_type = reference.get("type")
        require_test_type(test_type, context)
        return OrderedDict((("path", path), ("title", title), ("type", test_type)))

    allowed = {"file", "path", "type", "title", "testId", "name"}
    extras = set(reference) - allowed
    if extras:
        refuse(f"{context} has fields that version 2 linkedTests cannot preserve: {sorted(extras)}")
    file_value = normalize_string_aliases(reference, ("file", "path"), context, required=True)
    test_type = reference.get("type", required_type)
    require_test_type(test_type, context)
    test_id = normalize_string_aliases(reference, ("title", "testId", "name"), context)
    migrated = OrderedDict((("file", file_value), ("type", test_type)))
    if test_id is not None:
        migrated["testId"] = test_id
    return migrated


def migrate_planned_test(reference, context):
    if not isinstance(reference, dict):
        refuse(f"{context} must be an object")
    if set(reference) != {"path", "title", "type"}:
        refuse(f"{context} must contain only path, title, and type")
    path = normalize_string_aliases(reference, ("path",), context, required=True)
    title = normalize_string_aliases(reference, ("title",), context, required=True)
    require_test_type(reference.get("type"), context)
    return OrderedDict(
        (("path", path), ("title", title), ("type", reference["type"]))
    )


def migrate_scenario(source, index):
    context = f"scenario[{index}]"
    if not isinstance(source, dict):
        refuse(f"{context} is not an object")
    extras = set(source) - SCENARIO_KEYS
    if extras:
        refuse(f"{context} has fields that version 2 cannot preserve: {sorted(extras)}")

    scenario_id = normalize_scenario_id_aliases(source, context)
    scope_ref = normalize_scope_aliases(source, context)
    required_type = require_test_type(source.get("requiredTestType"), context)
    if not source.get("title"):
        refuse(f"{context} is missing title")

    values = dict(source)
    values["id"] = scenario_id
    values.pop("scenarioId", None)
    values.pop("scope", None)
    values.pop("scopeId", None)
    if scope_ref is not None:
        values["scopeRef"] = scope_ref

    planned_source = source.get("plannedTests", [])
    if not isinstance(planned_source, list):
        refuse(f"{context}.plannedTests must be an array")

    normalized_linked_aliases = []
    for linked_field in ("linkedTests", "linkedTestContracts"):
        if linked_field not in source:
            continue
        linked_source = source[linked_field]
        if not isinstance(linked_source, list):
            refuse(f"{context}.{linked_field} must be an array")
        linked = []
        planned_from_linked = []
        for ref_index, reference in enumerate(linked_source):
            migrated = migrate_linked_test(
                reference,
                required_type,
                f"{context}.{linked_field}[{ref_index}]",
            )
            if "path" in migrated:
                planned_from_linked.append(migrated)
            else:
                linked.append(migrated)
        normalized_linked_aliases.append((linked_field, linked, planned_from_linked))
    if len(normalized_linked_aliases) == 2:
        _, first_linked, first_planned = normalized_linked_aliases[0]
        _, second_linked, second_planned = normalized_linked_aliases[1]
        if first_linked != second_linked or first_planned != second_planned:
            refuse(f"{context} has conflicting linkedTests and linkedTestContracts values")
    if normalized_linked_aliases:
        _, linked, planned_from_linked = normalized_linked_aliases[0]
    else:
        linked = []
        planned_from_linked = []
    values.pop("linkedTestContracts", None)
    if "linkedTests" in source or "linkedTestContracts" in source:
        values["linkedTests"] = linked

    planned = []
    for ref_index, reference in enumerate(planned_source):
        planned.append(migrate_planned_test(reference, f"{context}.plannedTests[{ref_index}]"))
    if "plannedTests" in source or planned_from_linked:
        values["plannedTests"] = planned + planned_from_linked

    migrated_scenario = OrderedDict()
    for field in SCENARIO_FIELD_ORDER:
        if field in values:
            migrated_scenario[field] = values[field]
    return migrated_scenario


def migrate_document(source):
    if isinstance(source, list):
        scenarios = source
        spec = None
        generated_at = None
    elif isinstance(source, dict):
        extras = set(source) - TOP_LEVEL_KEYS
        if extras:
            refuse(f"manifest has fields that version 2 cannot preserve: {sorted(extras)}")
        source_version = source.get("schemaVersion", source.get("version"))
        if type(source_version) is not int or source_version != 1:
            refuse("manifest is neither a legacy array nor a version 1 envelope")
        if "schemaVersion" in source and "version" in source:
            if type(source["schemaVersion"]) is not int or type(source["version"]) is not int:
                refuse("manifest version fields must be integers")
            if source["schemaVersion"] != source["version"]:
                refuse("manifest has conflicting schemaVersion and version values")
        scenarios = source.get("scenarios")
        spec = choose_alias(source, "spec", "featureDir", "manifest")
        generated_at = source.get("generatedAt")
    else:
        refuse("manifest envelope must be an object or legacy array")

    if not isinstance(scenarios, list):
        refuse("manifest scenarios must be an array")
    migrated = OrderedDict((("schemaVersion", 2),))
    if spec is not None:
        migrated["spec"] = spec
    if generated_at is not None:
        migrated["generatedAt"] = generated_at
    migrated["scenarios"] = [migrate_scenario(scenario, index) for index, scenario in enumerate(scenarios)]
    return migrated


def run_pre_replace_test_hook(directory_descriptor, source_name):
    replacement = os.environ.get("_BUBBLES_SCENARIO_MIGRATE_TEST_REPLACEMENT")
    if replacement:
        os.replace(replacement, source_name, dst_dir_fd=directory_descriptor)


def run_final_replace_test_hook(directory_descriptor, source_name):
    replacement = os.environ.get("_BUBBLES_SCENARIO_MIGRATE_TEST_FINAL_REPLACEMENT")
    if replacement:
        os.replace(replacement, source_name, dst_dir_fd=directory_descriptor)


def assert_snapshot_unchanged(
    directory_descriptor, source_name, display_path, source_bytes, source_identity
):
    current_bytes, current_identity, _ = read_snapshot(
        directory_descriptor, source_name, display_path
    )
    if current_identity != source_identity:
        refuse("source identity changed during migration")
    if current_bytes != source_bytes:
        refuse("source bytes changed during migration")


def create_temporary(directory_descriptor, source_name):
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    flags |= getattr(os, "O_NOFOLLOW", 0)
    flags |= getattr(os, "O_CLOEXEC", 0)
    for _ in range(128):
        temporary_name = f".{source_name}.{secrets.token_hex(12)}"
        try:
            descriptor = os.open(
                temporary_name, flags, 0o600, dir_fd=directory_descriptor
            )
            return descriptor, temporary_name
        except FileExistsError:
            continue
        except OSError as error:
            refuse(f"cannot create migration temporary file: {error}")
    refuse("cannot create a unique migration temporary file")


def atomic_exchange(directory_descriptor, first_name, second_name):
    libc = ctypes.CDLL(None, use_errno=True)
    first_bytes = os.fsencode(first_name)
    second_bytes = os.fsencode(second_name)
    if sys.platform.startswith("linux"):
        exchange = getattr(libc, "renameat2", None)
        exchange_flag = 2
    elif sys.platform == "darwin":
        exchange = getattr(libc, "renameatx_np", None)
        exchange_flag = 2
    else:
        exchange = None
        exchange_flag = 0
    if exchange is None:
        refuse("atomic exchange primitive is unavailable; write mode is refused")
    exchange.argtypes = [
        ctypes.c_int,
        ctypes.c_char_p,
        ctypes.c_int,
        ctypes.c_char_p,
        ctypes.c_uint,
    ]
    exchange.restype = ctypes.c_int
    ctypes.set_errno(0)
    result = exchange(
        directory_descriptor,
        first_bytes,
        directory_descriptor,
        second_bytes,
        exchange_flag,
    )
    if result == 0:
        return
    error_number = ctypes.get_errno()
    unavailable_errors = {errno.ENOSYS, errno.EINVAL}
    for name in ("ENOTSUP", "EOPNOTSUPP"):
        value = getattr(errno, name, None)
        if value is not None:
            unavailable_errors.add(value)
    if error_number in unavailable_errors:
        refuse("atomic exchange primitive is unavailable; write mode is refused")
    raise OSError(error_number, os.strerror(error_number))


def write_atomically(
    directory_descriptor,
    source_name,
    display_path,
    document,
    source_bytes,
    source_identity,
    original_mode,
):
    content = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    content_bytes = content.encode("utf-8")
    descriptor, temporary_name = create_temporary(directory_descriptor, source_name)
    cleanup_temporary = True
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as handle:
            handle.write(content)
            handle.flush()
            os.fchmod(handle.fileno(), original_mode)
            os.fsync(handle.fileno())
            replacement_metadata = os.fstat(handle.fileno())
            replacement_identity = (
                replacement_metadata.st_dev,
                replacement_metadata.st_ino,
            )
        run_pre_replace_test_hook(directory_descriptor, source_name)
        assert_snapshot_unchanged(
            directory_descriptor,
            source_name,
            display_path,
            source_bytes,
            source_identity,
        )
        run_final_replace_test_hook(directory_descriptor, source_name)
        atomic_exchange(directory_descriptor, temporary_name, source_name)
        exchanged_bytes, exchanged_identity, _ = read_snapshot(
            directory_descriptor, temporary_name, display_path
        )
        if exchanged_identity != source_identity or exchanged_bytes != source_bytes:
            current_bytes, current_identity, _ = read_snapshot(
                directory_descriptor, source_name, display_path
            )
            if current_identity != replacement_identity or current_bytes != content_bytes:
                cleanup_temporary = False
                refuse(
                    "source changed again after atomic exchange; concurrent bytes remain preserved"
                )
            atomic_exchange(directory_descriptor, temporary_name, source_name)
            rollback_bytes, rollback_identity, _ = read_snapshot(
                directory_descriptor, temporary_name, display_path
            )
            if rollback_identity != replacement_identity or rollback_bytes != content_bytes:
                cleanup_temporary = False
                refuse(
                    "source changed during atomic rollback; concurrent bytes remain preserved "
                    f"at {temporary_name}"
                )
            refuse("source changed during final atomic replacement")
        os.unlink(temporary_name, dir_fd=directory_descriptor)
        cleanup_temporary = False
        os.fsync(directory_descriptor)
    finally:
        if cleanup_temporary:
            try:
                os.unlink(temporary_name, dir_fd=directory_descriptor)
            except FileNotFoundError:
                pass


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true", help="validate and report whether migration is required")
    mode.add_argument("--write", action="store_true", help="atomically replace a losslessly migratable manifest")
    parser.add_argument("manifest", type=Path)
    return parser.parse_args()


def main():
    args = parse_args()
    with migration_lock(args.manifest):
        directory_descriptor = open_source_directory(args.manifest)
        try:
            source_name = args.manifest.name
            source_bytes, source_identity, original_mode = read_snapshot(
                directory_descriptor, source_name, args.manifest
            )
            source = parse_json(source_bytes)
            validator = load_validator(Path(__file__).resolve())

            if (
                isinstance(source, dict)
                and type(source.get("schemaVersion")) is int
                and source.get("schemaVersion") == 2
            ):
                validate_v2(source, validator)
                if args.write:
                    assert_snapshot_unchanged(
                        directory_descriptor,
                        source_name,
                        args.manifest,
                        source_bytes,
                        source_identity,
                    )
                print(f"scenario-manifest-migrate: CURRENT {args.manifest}")
                return 0

            migrated = migrate_document(source)
            validate_v2(migrated, validator)
            if args.check:
                print(f"scenario-manifest-migrate: MIGRATION_REQUIRED {args.manifest}")
                return 1
            write_atomically(
                directory_descriptor,
                source_name,
                args.manifest,
                migrated,
                source_bytes,
                source_identity,
                original_mode,
            )
            print(f"scenario-manifest-migrate: MIGRATED {args.manifest}")
            return 0
        finally:
            os.close(directory_descriptor)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (MigrationRefusal, OSError) as error:
        print(f"scenario-manifest-migrate: REFUSED {error}", file=sys.stderr)
        sys.exit(2)
