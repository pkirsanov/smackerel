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
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCHEMAS_DIR="$REPO_ROOT/bubbles/schemas"

if ! command -v python3 >/dev/null 2>&1; then
  echo "yaml-schema-validate: SKIP (python3 not installed)"
  exit 0
fi

if ! python3 -c "import yaml, jsonschema" >/dev/null 2>&1; then
  echo "yaml-schema-validate: SKIP (PyYAML or jsonschema not installed)"
  echo "  Install with: python3 -m pip install --user pyyaml jsonschema"
  exit 0
fi

python3 - "$REPO_ROOT" "$SCHEMAS_DIR" <<'PY'
import json
import sys
from pathlib import Path

import yaml
from jsonschema import Draft7Validator
from jsonschema.exceptions import SchemaError

repo_root = Path(sys.argv[1])
schemas_dir = Path(sys.argv[2])

# (yaml_path_rel, schema_filename, optional)
pairs = [
    ("bubbles/workflows.yaml", "workflows.schema.json", False),
    ("bubbles/capability-ledger.yaml", "capability-ledger.schema.json", False),
    ("bubbles/adoption-profiles.yaml", "adoption-profiles.schema.json", False),
    # v5.1 / M9 additions — present only when project uses these surfaces.
    ("propagation-policy.yaml", "propagation-policy.schema.json", True),
    ("config/propagation-policy.yaml", "propagation-policy.schema.json", True),
]

failures = 0
for yaml_rel, schema_name, optional in pairs:
    yaml_path = repo_root / yaml_rel
    schema_path = schemas_dir / schema_name
    if not yaml_path.exists():
        if not optional:
            print(f"yaml-schema-validate: SKIP  {yaml_rel} (not present)")
        continue
    if not schema_path.exists():
        print(f"yaml-schema-validate: SKIP  {yaml_rel} (no schema at {schema_path})")
        continue
    try:
        with open(yaml_path) as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        print(f"yaml-schema-validate: FAIL  {yaml_rel} — YAML parse error: {e}")
        failures += 1
        continue
    with open(schema_path) as f:
        schema = json.load(f)
    try:
        validator = Draft7Validator(schema)
    except SchemaError as e:
        print(f"yaml-schema-validate: FAIL  {schema_name} — schema error: {e.message}")
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

# v5.1 / M9: scenario-manifest.json — repo-wide scan under specs/**.
scenario_schema = schemas_dir / "scenario-manifest.schema.json"
if scenario_schema.exists():
    with open(scenario_schema) as f:
        schema = json.load(f)
    validator = Draft7Validator(schema)
    found = 0
    failed_here = 0
    for manifest in repo_root.glob("specs/*/scenario-manifest.json"):
        found += 1
        try:
            with open(manifest) as f:
                data = json.load(f)
        except json.JSONDecodeError as e:
            print(f"yaml-schema-validate: FAIL  {manifest.relative_to(repo_root)} — JSON parse error: {e}")
            failures += 1
            failed_here += 1
            continue
        errs = list(validator.iter_errors(data))
        if errs:
            print(f"yaml-schema-validate: FAIL  {manifest.relative_to(repo_root)} — {len(errs)} validation error(s)")
            for e in errs[:10]:
                loc = "/".join(str(p) for p in e.absolute_path) or "<root>"
                print(f"  {loc}: {e.message[:200]}")
            failures += 1
            failed_here += 1
    if found and not failed_here:
        print(f"yaml-schema-validate: PASS  specs/*/scenario-manifest.json ({found} file(s))")
    elif not found:
        print("yaml-schema-validate: SKIP  specs/*/scenario-manifest.json (none present)")

if failures:
    sys.exit(1)
sys.exit(0)
PY
