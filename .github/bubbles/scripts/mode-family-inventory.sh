#!/usr/bin/env bash
#
# mode-family-inventory.sh — per-family mode inventory + structural validator
# (review R5, v6.1).
#
# v6.1 physically split the mode registry out of workflows.yaml into the
# dedicated bubbles/workflows/modes.yaml file (true split, no duplication). This
# tool operates on that registry and ENFORCES the structural invariant the split
# relies on: every mode in modes.yaml maps to EXACTLY ONE v6 primitive family
# declared in bubbles/workflows/aliases.yaml. A new mode without a canonical
# primitive mapping fails framework-validate.
#
# Usage:
#   mode-family-inventory.sh                # print family -> [modes] inventory
#   mode-family-inventory.sh --check        # exit 1 if any mode is unmapped,
#                                           # maps to >1 family, or maps to a
#                                           # non-canonical primitive
#   mode-family-inventory.sh --family <p>   # print only modes in primitive <p>
#
# Exit codes:
#   0 — inventory printed / check passed
#   1 — check failed (structural invariant violated)
#   2 — usage / missing input

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS="${BUBBLES_WORKFLOWS_FILE:-$REPO_ROOT/bubbles/workflows.yaml}"
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml.
# Parse them from there unless the workflows file still embeds an inline modes:
# block (fixtures / pre-split repos).
MODES="$(dirname "$WORKFLOWS")/workflows/modes.yaml"
if grep -qE '^modes:' "$WORKFLOWS" 2>/dev/null || [[ ! -f "$MODES" ]]; then
  MODES="$WORKFLOWS"
fi
ALIASES="${BUBBLES_ALIASES_FILE:-$REPO_ROOT/bubbles/workflows/aliases.yaml}"

MODE="print"
FAMILY_FILTER=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) MODE="check"; shift;;
    --family) MODE="family"; FAMILY_FILTER="${2:?--family requires a primitive}"; shift 2;;
    -h|--help)
      sed -n '2,22p' "$0"; exit 0;;
    *) echo "mode-family-inventory: unknown arg: $1" >&2; exit 2;;
  esac
done

[[ -f "$WORKFLOWS" ]] || { echo "mode-family-inventory: workflows.yaml missing at $WORKFLOWS" >&2; exit 2; }
[[ -f "$ALIASES" ]] || { echo "mode-family-inventory: aliases.yaml missing at $ALIASES" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "mode-family-inventory: SKIP (python3 not installed)"
  exit 0
fi

WORKFLOWS="$MODES" ALIASES="$ALIASES" MODE="$MODE" FAMILY_FILTER="$FAMILY_FILTER" python3 - <<'PY'
import os, re, sys

workflows = os.environ["WORKFLOWS"]
aliases = os.environ["ALIASES"]
mode = os.environ["MODE"]
family_filter = os.environ.get("FAMILY_FILTER", "")

# --- Parse the canonical 15 primitives + v5->primitive map from aliases.yaml.
# Pure-line parser (no yq/PyYAML dependency) matching mode-resolver conventions.
primitives = []
in_prims = False
alias_primitive = {}   # v5 mode -> primitive
in_aliases = False
current_mode = None
with open(aliases) as f:
    for raw in f:
        line = raw.rstrip("\n")
        if line.startswith("v6Primitives:"):
            in_prims = True; in_aliases = False; continue
        if line.startswith("v5Aliases:"):
            in_aliases = True; in_prims = False; continue
        if in_prims:
            m = re.match(r"^- (\S+)", line)
            if m:
                primitives.append(m.group(1))
            elif re.match(r"^[A-Za-z]", line):
                in_prims = False
        if in_aliases:
            # mode key: two-space indent, ends with ':'
            mk = re.match(r"^  ([a-z][a-z0-9-]*):\s*$", line)
            if mk:
                current_mode = mk.group(1)
                continue
            pm = re.match(r"^    primitive:\s*(\S+)", line)
            if pm and current_mode:
                alias_primitive[current_mode] = pm.group(1)

primitive_set = set(primitives)

# --- Parse the 55 defined modes from workflows.yaml modes: block.
defined_modes = []
in_modes = False
with open(workflows) as f:
    lines = f.read().splitlines()
for i, line in enumerate(lines):
    if line.startswith("modes:"):
        in_modes = True; continue
    if in_modes and re.match(r"^[A-Za-z]", line):
        in_modes = False
    if in_modes:
        mm = re.match(r"^  ([a-z][a-z0-9-]*):\s*$", line)
        if mm:
            # Confirm it's a real mode (next non-blank indented line is description/template-ish).
            defined_modes.append(mm.group(1))

# --- Build family -> modes, and detect violations.
from collections import defaultdict
family = defaultdict(list)
unmapped = []
noncanonical = []
for mname in defined_modes:
    prim = alias_primitive.get(mname)
    if prim is None:
        unmapped.append(mname)
        continue
    if prim not in primitive_set:
        noncanonical.append((mname, prim))
        continue
    family[prim].append(mname)

if mode == "check":
    problems = []
    if unmapped:
        problems.append(f"{len(unmapped)} mode(s) in workflows.yaml have no v6 primitive in aliases.yaml: {sorted(unmapped)}")
    if noncanonical:
        problems.append(f"{len(noncanonical)} mode(s) map to a non-canonical primitive: {noncanonical}")
    # Every primitive that has aliases should be one of the canonical 15 (already checked).
    if problems:
        print("mode-family-inventory: FAIL")
        for p in problems:
            print(f"  - {p}")
        sys.exit(1)
    total = sum(len(v) for v in family.values())
    print(f"mode-family-inventory: PASS — {total} modes across {len(family)} families; "
          f"every mode maps to exactly one canonical v6 primitive.")
    sys.exit(0)

if mode == "family":
    modes = sorted(family.get(family_filter, []))
    if not modes:
        print(f"(no modes in family '{family_filter}')")
        sys.exit(0)
    for m in modes:
        print(m)
    sys.exit(0)

# Default: print the full inventory in canonical primitive order.
for prim in primitives:
    modes = sorted(family.get(prim, []))
    print(f"{prim} ({len(modes)}):")
    for m in modes:
        print(f"  - {m}")
if unmapped:
    print(f"UNMAPPED ({len(unmapped)}):")
    for m in sorted(unmapped):
        print(f"  - {m}")
PY
