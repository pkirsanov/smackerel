#!/usr/bin/env bash
#
# bubbles result-envelope-validate.sh
#
# Scans every `agents/*.agent.md` for ```json fenced blocks tagged as
# `result_envelope:` (or `result-envelope:`) and validates each against
# bubbles/schemas/result-envelope.schema.json.
#
# Modes (history):
#   v5.2 / F5: full ADVISORY. Missing or malformed envelopes warn only.
#   v6.0 / B3: malformed envelopes BLOCK. Missing envelopes still WARN
#              (full coverage tracked as v6.1 follow-up — flipping all 40
#              agents at once would block every push without rolling
#              authoring work first).
#
# Usage:
#   result-envelope-validate.sh                  # v6.0 default: malformed
#                                                # blocks, missing warns
#   result-envelope-validate.sh --advisory       # v5.2 behavior: never block
#   result-envelope-validate.sh --strict         # block on missing OR malformed
#                                                # (v6.1+; opt-in until all
#                                                # agents are populated)
#
# Exit codes:
#   0  no blocking findings
#   1  at least one blocking finding for the active mode
#   2  usage error or missing schema

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCHEMA="$REPO_ROOT/bubbles/schemas/result-envelope.schema.json"
AGENTS_DIR="$REPO_ROOT/agents"

MODE="v6-default"  # v6.0 / B3 default: malformed blocks, missing warns.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) MODE="strict"; shift;;
    --advisory) MODE="advisory"; shift;;
    -h|--help)
      sed -n '1,30p' "$0" >&2
      exit 0
      ;;
    *) echo "result-envelope-validate: unknown arg: $1" >&2; exit 2;;
  esac
done

[[ -f "$SCHEMA" ]] || { echo "result-envelope-validate: schema missing at $SCHEMA" >&2; exit 2; }
[[ -d "$AGENTS_DIR" ]] || { echo "result-envelope-validate: agents/ missing at $AGENTS_DIR" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "result-envelope-validate: SKIP (python3 not installed)"
  exit 0
fi

AGENTS_DIR="$AGENTS_DIR" SCHEMA="$SCHEMA" MODE="$MODE" python3 - <<'PY'
import json, os, re, sys
from pathlib import Path

agents_dir = Path(os.environ['AGENTS_DIR'])
schema_path = Path(os.environ['SCHEMA'])
mode = os.environ.get('MODE', 'v6-default')  # advisory | v6-default | strict

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
print(f"  mode: {mode}")

if agents_missing_envelope and mode != "strict":
    print("  Advisory: the following agents do not yet emit a result_envelope JSON block:")
    for name in agents_missing_envelope[:10]:
        print(f"    - {name}")
    if len(agents_missing_envelope) > 10:
        print(f"    ... and {len(agents_missing_envelope) - 10} more")
    if mode == "v6-default":
        print("  Missing envelopes are advisory by default in the current contract; use --strict to make missing envelopes blocking.")

for name, err in malformed_envelopes[:10]:
    print(f"  MALFORMED: {name}: {err}")

# Exit policy:
#   advisory     -> always 0
#   v6-default   -> 1 iff any malformed; missing warns only
#   strict       -> 1 iff any malformed OR missing
if mode == "advisory":
    sys.exit(0)
if mode == "v6-default":
    sys.exit(1 if malformed_envelopes else 0)
# strict
sys.exit(1 if (agents_missing_envelope or malformed_envelopes) else 0)
PY
