#!/usr/bin/env bash
# Bubbles YAML schema validator (v5.0.1 / H4).
#
# Validates the critical YAML registries against their JSON Schemas:
#   - bubbles/workflows.yaml                 → bubbles/schemas/workflows.schema.json
#   - bubbles/capability-ledger.yaml         → bubbles/schemas/capability-ledger.schema.json
#   - bubbles/adoption-profiles.yaml         → bubbles/schemas/adoption-profiles.schema.json
#
# Requires Python 3 with PyYAML and jsonschema available. If missing,
# emits a single advisory message and exits 0 so the framework still
# validates on minimal hosts.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
case "$SCRIPT_DIR" in
    */.github/bubbles/scripts) REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)" ;;
    *) REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)" ;;
esac

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo-root)
            [[ $# -ge 2 ]] || { echo "yaml-schema-validate: --repo-root requires a value" >&2; exit 2; }
            REPO_ROOT="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: bash bubbles/scripts/yaml-schema-validate.sh [--repo-root PATH]"
            exit 0
            ;;
        *)
            echo "yaml-schema-validate: unknown option: $1" >&2
            exit 2
            ;;
    esac
done

if [[ ! -d "$REPO_ROOT" ]]; then
    echo "yaml-schema-validate: repository root is not a directory: $REPO_ROOT" >&2
    exit 2
fi
REPO_ROOT="$(cd "$REPO_ROOT" && pwd -P)"
if [[ -d "$REPO_ROOT/bubbles" ]]; then
    FRAMEWORK_ROOT="$REPO_ROOT/bubbles"
elif [[ -d "$REPO_ROOT/.github/bubbles" ]]; then
    FRAMEWORK_ROOT="$REPO_ROOT/.github/bubbles"
else
    FRAMEWORK_ROOT="$REPO_ROOT/bubbles"
fi
SCHEMAS_DIR="$FRAMEWORK_ROOT/schemas"

# IMP-027 SCOPE-4 / SEC-2: these were `exit 0`. A schema validator that skips
# reports success for a check that never ran.
# shellcheck source=bubbles/scripts/dependency-posture.sh
[[ -f "$SCRIPT_DIR/dependency-posture.sh" ]] && source "$SCRIPT_DIR/dependency-posture.sh"

if ! command -v python3 >/dev/null 2>&1; then
  if declare -F bubbles_require_dep >/dev/null 2>&1; then
    bubbles_require_dep "yaml-schema-validate" "python3 is not installed" || exit 0
  fi
  echo "yaml-schema-validate: SKIP (python3 not installed)"
  exit 0
fi

if ! python3 -c "import yaml, jsonschema" >/dev/null 2>&1; then
  if declare -F bubbles_require_dep >/dev/null 2>&1; then
    bubbles_require_dep "yaml-schema-validate" "PyYAML or jsonschema is not installed (python3 -m pip install --user pyyaml jsonschema)" || exit 0
  fi
  echo "yaml-schema-validate: SKIP (PyYAML or jsonschema not installed)"
  echo "  Install with: python3 -m pip install --user pyyaml jsonschema"
  exit 0
fi

python3 - "$REPO_ROOT" "$FRAMEWORK_ROOT" "$SCHEMAS_DIR" <<'PY'
import json
import os
import re
import stat
import sys
from datetime import date
from pathlib import Path

import yaml
from jsonschema import Draft7Validator, FormatChecker, validators
from jsonschema.exceptions import SchemaError

repo_root = Path(sys.argv[1])
framework_root = Path(sys.argv[2])
schemas_dir = Path(sys.argv[3])


class DuplicateJsonMemberError(ValueError):
    pass


class SchemaObjectError(ValueError):
    pass


class SchemaObjectMissingError(SchemaObjectError):
    pass


def reject_duplicate_members(pairs):
    document = {}
    for key, value in pairs:
        if key in document:
            raise DuplicateJsonMemberError(f"duplicate JSON member {key!r}")
        document[key] = value
    return document


def load_json(handle):
    return json.load(handle, object_pairs_hook=reject_duplicate_members)


def load_schema_object(path):
    try:
        relative_path = path.relative_to(repo_root)
    except ValueError as error:
        raise SchemaObjectError("schema path escapes repository root") from error

    parts = relative_path.parts
    if not parts or any(part in ("", ".", "..") for part in parts):
        raise SchemaObjectError("schema path is not a normalized repository path")

    required_flags = ("O_DIRECTORY", "O_NOFOLLOW", "O_NONBLOCK")
    missing_flags = [name for name in required_flags if not hasattr(os, name)]
    if missing_flags:
        raise SchemaObjectError(
            f"host lacks safe schema-read flags: {', '.join(missing_flags)}"
        )

    common_flags = os.O_CLOEXEC if hasattr(os, "O_CLOEXEC") else 0
    directory_flags = (
        os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK | common_flags
    )
    file_flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | common_flags
    directory_fd = None
    schema_fd = None
    try:
        directory_fd = os.open(repo_root, directory_flags)
        for component in parts[:-1]:
            next_fd = os.open(component, directory_flags, dir_fd=directory_fd)
            os.close(directory_fd)
            directory_fd = next_fd
        try:
            schema_fd = os.open(parts[-1], file_flags, dir_fd=directory_fd)
        except FileNotFoundError as error:
            raise SchemaObjectMissingError("schema object is absent") from error
        metadata = os.fstat(schema_fd)
        if not stat.S_ISREG(metadata.st_mode):
            raise SchemaObjectError("schema object is not a regular file")
        with os.fdopen(schema_fd, encoding="utf-8") as handle:
            schema_fd = None
            return load_json(handle)
    except SchemaObjectError:
        raise
    except OSError as error:
        raise SchemaObjectError(f"unsafe schema object: {error.strerror}") from error
    finally:
        if schema_fd is not None:
            os.close(schema_fd)
        if directory_fd is not None:
            os.close(directory_fd)


exact_type_checker = Draft7Validator.TYPE_CHECKER.redefine(
    "integer", lambda _checker, instance: type(instance) is int
)
ExactDraft7Validator = validators.extend(
    Draft7Validator, type_checker=exact_type_checker
)

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

# (yaml_path, display_path, schema_filename, optional)
pairs = [
    (framework_root / "workflows.yaml", framework_root.relative_to(repo_root) / "workflows.yaml", "workflows.schema.json", False),
    (framework_root / "capability-ledger.yaml", framework_root.relative_to(repo_root) / "capability-ledger.yaml", "capability-ledger.schema.json", False),
    (framework_root / "adoption-profiles.yaml", framework_root.relative_to(repo_root) / "adoption-profiles.yaml", "adoption-profiles.schema.json", False),
    # IMP-020 S3 / AF-005 — tool-trust registry.
    (framework_root / "tool-trust-registry.yaml", framework_root.relative_to(repo_root) / "tool-trust-registry.yaml", "tool-trust-registry.schema.json", False),
    # v5.1 / M9 additions — present only when project uses these surfaces.
    (repo_root / "propagation-policy.yaml", Path("propagation-policy.yaml"), "propagation-policy.schema.json", True),
    (repo_root / "config/propagation-policy.yaml", Path("config/propagation-policy.yaml"), "propagation-policy.schema.json", True),
]

failures = 0
for yaml_path, yaml_rel, schema_name, optional in pairs:
    schema_path = schemas_dir / schema_name
    if not yaml_path.exists():
        if not optional:
            print(f"yaml-schema-validate: SKIP  {yaml_rel} (not present)")
        continue
    if not schema_path.exists():
        print(f"yaml-schema-validate: FAIL  {yaml_rel} (required target schema absent at {schema_path})")
        failures += 1
        continue
    try:
        with open(yaml_path) as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        print(f"yaml-schema-validate: FAIL  {yaml_rel} — YAML parse error: {e}")
        failures += 1
        continue
    try:
        schema = load_schema_object(schema_path)
        ExactDraft7Validator.check_schema(schema)
        validator = ExactDraft7Validator(schema, format_checker=format_checker)
    except (SchemaObjectError, json.JSONDecodeError, DuplicateJsonMemberError, SchemaError) as e:
        print(f"yaml-schema-validate: FAIL  {schema_name} — schema error: {e}")
        failures += 1
        continue
    errors = sorted(validator.iter_errors(data), key=lambda e: list(e.absolute_path))
    if errors:
        print(f"yaml-schema-validate: FAIL  {yaml_rel} — {len(errors)} validation error(s)")
        for err in errors[:20]:
            loc = "/".join(str(p) for p in err.absolute_path) or "<root>"
            print(f"  {loc}: {err.message[:200]}")
        if len(errors) > 20:
            print(f"  ... {len(errors) - 20} more")
        failures += 1
        continue
    print(f"yaml-schema-validate: PASS  {yaml_rel}")

# Scenario manifests dispatch by envelope and explicit schemaVersion. A legacy
# top-level array uses the version 1 compatibility schema. New output must use
# the strict version 2 object envelope.
scenario_schema_paths = {
    1: schemas_dir / "scenario-manifest.schema.json",
    2: schemas_dir / "scenario-manifest-v2.schema.json",
}
scenario_validators = {}
scenario_schema_failed = False
for version, schema_path in scenario_schema_paths.items():
    try:
        schema = load_schema_object(schema_path)
        ExactDraft7Validator.check_schema(schema)
        scenario_validators[version] = ExactDraft7Validator(schema, format_checker=format_checker)
    except (SchemaObjectError, json.JSONDecodeError, DuplicateJsonMemberError, SchemaError) as e:
        print(f"yaml-schema-validate: FAIL  {schema_path.name} — schema error: {e}")
        failures += 1
        scenario_schema_failed = True

if not scenario_schema_failed:

    found = 0
    failed_here = 0
    for manifest in sorted(repo_root.glob("specs/**/scenario-manifest.json")) if not scenario_schema_failed else []:
        found += 1
        try:
            with open(manifest) as f:
                data = load_json(f)
        except (json.JSONDecodeError, DuplicateJsonMemberError) as e:
            print(f"yaml-schema-validate: FAIL  {manifest.relative_to(repo_root)} — JSON parse error: {e}")
            failures += 1
            failed_here += 1
            continue
        if isinstance(data, list):
            version = 1
            validation_data = {"schemaVersion": 1, "scenarios": data}
        elif isinstance(data, dict):
            version = data.get("schemaVersion")
            validation_data = data
            if type(version) is not int or version not in scenario_validators:
                label = "missing" if version is None else repr(version)
                print(
                    f"yaml-schema-validate: FAIL  {manifest.relative_to(repo_root)}"
                    f" — unsupported schemaVersion {label}"
                )
                failures += 1
                failed_here += 1
                continue
        else:
            print(
                f"yaml-schema-validate: FAIL  {manifest.relative_to(repo_root)}"
                " — unsupported manifest envelope"
            )
            failures += 1
            failed_here += 1
            continue
        validator = scenario_validators[version]
        errs = sorted(validator.iter_errors(validation_data), key=lambda e: list(e.absolute_path))
        if errs:
            print(f"yaml-schema-validate: FAIL  {manifest.relative_to(repo_root)} — {len(errs)} validation error(s)")
            for e in errs[:10]:
                loc = "/".join(str(p) for p in e.absolute_path) or "<root>"
                print(f"  {loc}: {e.message[:200]}")
            failures += 1
            failed_here += 1
    if found and not failed_here:
        print(f"yaml-schema-validate: PASS  specs/**/scenario-manifest.json ({found} file(s))")
    elif not found:
        print("yaml-schema-validate: SKIP  specs/**/scenario-manifest.json (none present)")
else:
    print("yaml-schema-validate: SKIP  specs/**/scenario-manifest.json (schema load failed)")

if failures:
    sys.exit(1)
sys.exit(0)
PY
