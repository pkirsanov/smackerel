#!/usr/bin/env bash
#
# bubbles gate-meta.sh — gate registry query helper (v5.1 / M6).
#
# Single read interface for "what does gate Gxxx do?". Wraps the gates:
# block of bubbles/workflows.yaml. Adoption path for v5.2/v6 when the
# registry moves to bubbles/registry/gates.yaml — callers change nothing.
#
# Usage:
#   gate-meta.sh list                     # all defined gate IDs, one per line
#   gate-meta.sh exists Gxxx              # exit 0 if defined, 1 if not
#   gate-meta.sh name Gxxx                # gate name field
#   gate-meta.sh description Gxxx         # gate description field
#   gate-meta.sh json Gxxx                # full record as JSON
#   gate-meta.sh count                    # number of defined gates

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS="$REPO_ROOT/bubbles/workflows.yaml"

[[ -f "$WORKFLOWS" ]] || { echo "gate-meta: workflows.yaml missing at $WORKFLOWS" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "gate-meta: python3 required" >&2
  exit 2
fi

OP="${1:-}"
[[ -z "$OP" ]] && { echo "usage: gate-meta.sh {list|exists|name|description|json|count} [Gxxx]" >&2; exit 2; }

WORKFLOWS="$WORKFLOWS" OP="$OP" GATE="${2:-}" python3 - <<'PY'
import os, re, sys, json

workflows = os.environ['WORKFLOWS']
op = os.environ['OP']
gate = os.environ.get('GATE', '')

# Parse gates: block. Supports `description: "..."` and `description: >- multiline`.
gates = {}
current_id = None
current_field = None
current = {}

def commit_current():
    if current_id:
        gates[current_id] = dict(current)

in_gates = False
buf_field = None
buf_lines = []

def flush_buf():
    global buf_field, buf_lines, current
    if buf_field is not None:
        current[buf_field] = ' '.join(buf_lines).strip()
        buf_field = None
        buf_lines = []

with open(workflows) as f:
    for raw in f:
        line = raw.rstrip('\n')
        if line.startswith('gates:'):
            in_gates = True
            continue
        if not in_gates:
            continue
        # Top-level non-gates section ends the gates block.
        if re.match(r'^[a-zA-Z]', line):
            flush_buf()
            commit_current()
            in_gates = False
            continue
        # New gate entry: '  Gxxx:'
        m = re.match(r'^  (G\d{3}):\s*$', line)
        if m:
            flush_buf()
            commit_current()
            current_id = m.group(1)
            current = {'id': current_id}
            continue
        # Field on same line: '    name: foo_gate' or '    description: text'
        m = re.match(r'^    ([a-zA-Z_]+):\s*(.*)$', line)
        if m:
            flush_buf()
            field, value = m.group(1), m.group(2).strip()
            if value in ('>-', '>', '|', '|-'):
                # Multiline scalar; accumulate continuation lines.
                buf_field = field
                buf_lines = []
            else:
                current[field] = value.strip().strip('"').strip("'")
            continue
        # Continuation of a multiline scalar.
        if buf_field is not None:
            stripped = line.strip()
            if stripped:
                buf_lines.append(stripped)
            continue

# Final flush at EOF if file ended in gates block.
flush_buf()
commit_current()

if op == 'list':
    for gid in sorted(gates.keys()):
        print(gid)
    sys.exit(0)

if op == 'count':
    print(len(gates))
    sys.exit(0)

if op == 'exists':
    sys.exit(0 if gate in gates else 1)

if not gate:
    print(f"gate-meta: {op} requires a gate id (Gxxx)", file=sys.stderr)
    sys.exit(2)

if gate not in gates:
    print(f"gate-meta: undefined gate: {gate}", file=sys.stderr)
    sys.exit(1)

g = gates[gate]
if op == 'name':
    print(g.get('name', ''))
elif op == 'description':
    print(g.get('description', ''))
elif op == 'json':
    print(json.dumps(g, separators=(',', ':')))
else:
    print(f"gate-meta: unknown op: {op}", file=sys.stderr)
    sys.exit(2)
PY
