#!/usr/bin/env bash
#
# bubbles evidence-tool-log-bridge.sh — bridge between DoD evidence and
# tool-call log (v5.1 / M2 — advisory phase).
#
# For each `[x]` DoD item in scopes.md (or per-scope scope.md), check whether
# the spec's tool-calls.jsonl contains an entry with matching:
#   - sessionId or spec field present
#   - exitCode == 0 (or matches expected pattern)
#   - cmd substring overlapping with what the DoD claims
#
# In v5.1 this is ADVISORY ONLY: it reports coverage as a confidence signal,
# not a blocker. v5.2 promotes the matcher to a primary evidence path —
# at which point a DoD item with a passing tool-log entry no longer requires
# inline ≥10-line raw output (the tool-log is structurally stronger).
#
# Anti-fabrication is monotonically stronger: existing prose-evidence path
# is preserved; tool-log path is additive proof.
#
# Usage:
#   evidence-tool-log-bridge.sh <spec-dir> [--log <path>]

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: evidence-tool-log-bridge.sh <spec-dir> [--log <path>]

Reports DoD ↔ tool-call log coverage. Advisory only in v5.1.
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
SPEC_DIR="$1"; shift
LOG_OVERRIDE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --log) LOG_OVERRIDE="$2"; shift 2;;
    -h|--help) usage; exit 0;;
    *) usage; exit 2;;
  esac
done

[[ -d "$SPEC_DIR" ]] || { echo "evidence-tool-log-bridge: spec dir missing" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "evidence-tool-log-bridge: SKIP (python3 not installed)"
  exit 0
fi

# Resolve log path. Default: <repo>/.specify/runtime/tool-calls.jsonl.
LOG="$LOG_OVERRIDE"
if [[ -z "$LOG" ]]; then
  REPO_ROOT="$(cd "$SPEC_DIR" && git rev-parse --show-toplevel 2>/dev/null || pwd)"
  LOG="$REPO_ROOT/.specify/runtime/tool-calls.jsonl"
fi

if [[ ! -f "$LOG" ]]; then
  echo "evidence-tool-log-bridge: no tool-call log at $LOG"
  echo "  Advisory: agents can opt into tool-call logging via tool-log.sh wrapper."
  echo "  v5.2 evidence gate will read this log as primary structured evidence."
  exit 0
fi

SPEC_SLUG="$(basename "$SPEC_DIR")"

SPEC_DIR="$SPEC_DIR" SPEC_SLUG="$SPEC_SLUG" LOG="$LOG" python3 - <<'PY'
import json, os, re
from pathlib import Path

spec_dir = Path(os.environ['SPEC_DIR'])
spec_slug = os.environ['SPEC_SLUG']
log_path = Path(os.environ['LOG'])

# Load tool-call entries for this spec.
entries = []
for raw in log_path.read_text(errors='replace').splitlines():
    raw = raw.strip()
    if not raw:
        continue
    try:
        d = json.loads(raw)
    except json.JSONDecodeError:
        continue
    spec_field = d.get('spec', '') or ''
    if spec_field == spec_slug or spec_slug.split('-', 1)[0] in spec_field:
        entries.append(d)

# Collect DoD [x] items from scope files.
scope_files = []
if (spec_dir / "scopes.md").exists():
    scope_files.append(spec_dir / "scopes.md")
for p in spec_dir.glob("scopes/*/scope.md"):
    scope_files.append(p)

CHECKED_RE = re.compile(r'^- \[x\] (.*)$')
dod_items = []
for sf in scope_files:
    try:
        for i, line in enumerate(sf.read_text(errors='replace').splitlines(), start=1):
            m = CHECKED_RE.match(line)
            if m:
                dod_items.append((sf.relative_to(spec_dir.parent), i, m.group(1)))
    except Exception:
        continue

print(f"evidence-tool-log-bridge: {spec_slug}")
print(f"  Tool-call entries (spec={spec_slug}): {len(entries)}")
print(f"  DoD [x] items found in {len(scope_files)} scope file(s): {len(dod_items)}")

if not dod_items:
    print("  (no checked DoD items to bridge)")
    raise SystemExit(0)

if not entries:
    print("  Coverage: 0% — no tool-log entries for this spec.")
    print("  Advisory only in v5.1; existing prose-evidence path remains valid.")
    raise SystemExit(0)

# Heuristic matching: DoD body contains tokens from cmd field.
def tokens(s):
    return set(re.findall(r'[a-zA-Z][a-zA-Z0-9._/-]+', s.lower()))

matched_dod = 0
for sf, ln, body in dod_items:
    body_toks = tokens(body)
    # Require ≥2 token overlap (excluding common words) to count as match.
    for e in entries:
        cmd_toks = tokens(e.get('cmd', ''))
        overlap = body_toks & cmd_toks - {'a', 'the', 'and', 'or', 'for', 'is', 'in', 'of', 'to', 'with'}
        if len(overlap) >= 2 and e.get('exitCode') == 0:
            matched_dod += 1
            break

pct = (matched_dod * 100) // max(len(dod_items), 1)
print(f"  Coverage: {pct}% ({matched_dod}/{len(dod_items)} DoD items have a matching tool-log entry)")
print("  Advisory in v5.1; v5.2 evidence gate makes this primary.")
PY
