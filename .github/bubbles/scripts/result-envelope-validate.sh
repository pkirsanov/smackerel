#!/usr/bin/env bash
#
# bubbles result-envelope-validate.sh (v5.2 / F5).
#
# Scans every `agents/*.agent.md` for ```json fenced blocks tagged as
# `result_envelope:` (or `result-envelope:`) and validates each against
# bubbles/schemas/result-envelope.schema.json.
#
# v5.2: ADVISORY. Missing block in an agent file → warn, do not fail.
#       Malformed block → warn, do not fail. v6 F5 promotion makes both
#       blocking.
#
# Usage:
#   result-envelope-validate.sh           # scan whole repo, exit 0 always
#                                         # (advisory)
#   result-envelope-validate.sh --strict  # exit 1 on any failure
#
# Exit codes:
#   0  advisory mode (default), or strict mode with no findings
#   1  strict mode + at least one envelope is malformed/missing
#   2  usage error or missing schema

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCHEMA="$REPO_ROOT/bubbles/schemas/result-envelope.schema.json"
AGENTS_DIR="$REPO_ROOT/agents"

STRICT=0
if [[ "${1:-}" == "--strict" ]]; then STRICT=1; fi

[[ -f "$SCHEMA" ]] || { echo "result-envelope-validate: schema missing at $SCHEMA" >&2; exit 2; }
[[ -d "$AGENTS_DIR" ]] || { echo "result-envelope-validate: agents/ missing at $AGENTS_DIR" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "result-envelope-validate: SKIP (python3 not installed)"
  exit 0
fi

AGENTS_DIR="$AGENTS_DIR" SCHEMA="$SCHEMA" STRICT="$STRICT" python3 - <<'PY'
import json, os, re, sys
from pathlib import Path

agents_dir = Path(os.environ['AGENTS_DIR'])
schema_path = Path(os.environ['SCHEMA'])
strict = int(os.environ['STRICT']) == 1

try:
    import jsonschema
except Exception:
    print("result-envelope-validate: SKIP (python jsonschema not installed)")
    sys.exit(0)

schema = json.loads(schema_path.read_text())

# Match a fenced block that looks like an envelope. Two acceptable shapes:
#   1. ```json result_envelope:        ```  ... ```
#   2. <!-- result_envelope --> ```json ... ```
#   3. ```jsonc with first non-blank line "// result_envelope"
# Cheap regex over option (1) (most common); future shapes can be added.
ENVELOPE_RE = re.compile(
    r'```(?:json[c5]?|jsonc)\s+(?:result[_-]envelope:?)\s*\n(.*?)\n```',
    re.DOTALL | re.IGNORECASE,
)
# Bare ```json fenced block under a heading "## Result Envelope" or
# "## RESULT-ENVELOPE" inside the prose. We accept a small lookbehind window.
BARE_ENVELOPE_RE = re.compile(
    r'(?:^|\n)#{1,4}\s+result[_\-\s]?envelope\b[^\n]*\n+```(?:json[c5]?|jsonc)\s*\n(.*?)\n```',
    re.DOTALL | re.IGNORECASE,
)

total_agents = 0
agents_with_envelope = 0
agents_missing_envelope = []
malformed_envelopes = []  # list of (path, error_text)

for p in sorted(agents_dir.glob('*.agent.md')):
    total_agents += 1
    text = p.read_text(errors='replace')
    matches = []
    for m in ENVELOPE_RE.finditer(text):
        matches.append(m.group(1))
    for m in BARE_ENVELOPE_RE.finditer(text):
        matches.append(m.group(1))
    if not matches:
        agents_missing_envelope.append(p.name)
        continue
    agents_with_envelope += 1
    for raw in matches:
        try:
            doc = json.loads(raw)
        except json.JSONDecodeError as e:
            malformed_envelopes.append((p.name, f"JSON parse error: {e}"))
            continue
        try:
            jsonschema.validate(doc, schema)
        except jsonschema.ValidationError as e:
            malformed_envelopes.append((p.name, f"Schema error: {e.message} at {list(e.path)}"))

# Report.
print(f"result-envelope-validate: scanned {total_agents} agent file(s)")
print(f"  with valid envelope: {agents_with_envelope}")
print(f"  missing envelope: {len(agents_missing_envelope)}")
print(f"  malformed envelope(s): {len(malformed_envelopes)}")

if agents_missing_envelope and not strict:
    print("  Advisory (v5.2): the following agents do not yet emit a result_envelope JSON block:")
    for name in agents_missing_envelope[:10]:
        print(f"    - {name}")
    if len(agents_missing_envelope) > 10:
        print(f"    ... and {len(agents_missing_envelope) - 10} more")
    print("  v6 will make this blocking. No action required yet.")

for name, err in malformed_envelopes[:10]:
    print(f"  MALFORMED: {name}: {err}")

# v5.2 advisory mode: always exit 0 unless --strict.
if strict and (agents_missing_envelope or malformed_envelopes):
    sys.exit(1)
sys.exit(0)
PY
