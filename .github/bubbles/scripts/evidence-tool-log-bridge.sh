#!/usr/bin/env bash
#
# bubbles evidence-tool-log-bridge.sh — bridge between DoD evidence and
# tool-call log.
#
# History:
#   v5.1 (M2): introduced as advisory matcher. Reports coverage as a
#              confidence signal; never blocks.
#   v5.2 (F1): primary evidence path for `[x]` DoD items. A passing
#              tool-log entry can satisfy the gate alongside the
#              traditional ≥10-line raw markdown evidence path. Both
#              paths preserved.
#   v6.0 (B1): MCP-primary. When the Bubbles MCP server is registered,
#              `query_tool_log` (declared in bubbles/mcp/tools/) wraps
#              this script and surfaces the JSON envelope produced by
#              `--format=json`. The bash twin remains the supported
#              fallback when MCP isn't registered.
#   v6.1 (R2): auto-capture. `bubbles/scripts/tool-capture-shim.sh` (sourceable)
#              routes gate-relevant commands through tool-log.sh so the entries
#              this bridge reads are populated as a ground-truth side effect of
#              running commands, not a manual step. Markdown evidence stays a
#              valid fallback when no tool-log entry exists.
#
# Anti-fabrication is monotonically stronger: existing prose-evidence path
# is preserved; tool-log path is additive proof.
#
# Usage:
#   evidence-tool-log-bridge.sh <spec-dir> [--log <path>] [--format=text|json]
#
# Output:
#   text (default)  Human-readable summary written to stdout.
#   json            Machine-readable envelope with shape:
#                     {
#                       "spec":           "<spec-slug>",
#                       "logPath":        "<absolute path>",
#                       "scopeFiles":     N,
#                       "dodItems":       N,
#                       "toolLogEntries": N,
#                       "matchedDodItems": N,
#                       "coveragePct":    0-100,
#                       "matches":        [{"scopeFile":..., "line":N, "dodBody":..., "cmd":..., "ts":..., "scenarioId":..., "claim":..., "sourceRevision":..., "exitCode":N}, ...],
#                       "unbound":        [{"scopeFile":..., "line":N, "dodBody":..., "reason":...}, ...]
#                     }
#
# Exit codes:
#   0   success (advisory; coverage reported regardless of value)
#   2   bad argument or missing spec dir
#
# The script is intentionally non-blocking. Promotion to a blocking gate
# is the responsibility of state-transition-guard.sh + diff-evidence-guard.sh,
# which read the JSON envelope and decide whether to fail the transition.

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: evidence-tool-log-bridge.sh <spec-dir> [--log <path>] [--format=text|json]

Reports DoD ↔ tool-call log coverage. Text by default; JSON for MCP / programmatic
consumption (`--format=json`).
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
SPEC_DIR="$1"; shift
LOG_OVERRIDE=""
FORMAT="text"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --log) LOG_OVERRIDE="$2"; shift 2;;
    --format) FORMAT="$2"; shift 2;;
    --format=*) FORMAT="${1#--format=}"; shift;;
    -h|--help) usage; exit 0;;
    *) usage; exit 2;;
  esac
done

case "$FORMAT" in
  text|json) ;;
  *) echo "evidence-tool-log-bridge: --format must be 'text' or 'json' (got: $FORMAT)" >&2; exit 2;;
esac

[[ -d "$SPEC_DIR" ]] || { echo "evidence-tool-log-bridge: spec dir missing" >&2; exit 2; }

# IMP-027 SCOPE-4 / SEC-2: this is the RECEIPT BRIDGE. Skipping it silently
# meant receipt-backed evidence simply did not exist for that run.
BUBBLES_DEP_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=bubbles/scripts/dependency-posture.sh
[[ -f "$BUBBLES_DEP_LIB_DIR/dependency-posture.sh" ]] && source "$BUBBLES_DEP_LIB_DIR/dependency-posture.sh"

if ! command -v python3 >/dev/null 2>&1; then
  if declare -F bubbles_require_dep >/dev/null 2>&1; then
    bubbles_require_dep "evidence-tool-log-bridge" "python3 is not installed" || exit 0
  fi
  echo "evidence-tool-log-bridge: SKIP (python3 not installed)"
  exit 0
fi

# Resolve log path. Default: <repo>/.specify/runtime/tool-calls.jsonl.
LOG="$LOG_OVERRIDE"
if [[ -z "$LOG" ]]; then
  REPO_ROOT="$(cd "$SPEC_DIR" && git rev-parse --show-toplevel 2>/dev/null || pwd)"
  LOG="$REPO_ROOT/.specify/runtime/tool-calls.jsonl"
fi

SPEC_SLUG="$(basename "$SPEC_DIR")"

if [[ ! -f "$LOG" ]]; then
  if [[ "$FORMAT" == "json" ]]; then
    printf '{"spec":%s,"logPath":%s,"logPresent":false,"scopeFiles":0,"dodItems":0,"toolLogEntries":0,"matchedDodItems":0,"coveragePct":0,"matches":[],"unbound":[],"note":"no tool-call log found"}\n' \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$SPEC_SLUG")" \
      "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$LOG")"
    exit 0
  fi
  echo "evidence-tool-log-bridge: no tool-call log at $LOG"
  echo "  Advisory: agents can opt into tool-call logging via tool-log.sh wrapper."
  echo "  v6.0 evidence path reads this log via MCP query_tool_log when registered."
  exit 0
fi

SPEC_DIR="$SPEC_DIR" SPEC_SLUG="$SPEC_SLUG" LOG="$LOG" FORMAT="$FORMAT" \
  SOURCE_REVISION="$(git -C "$SPEC_DIR" rev-parse --verify HEAD 2>/dev/null || true)" \
  python3 - <<'PY'
import json, os, re, sys
from pathlib import Path

spec_dir = Path(os.environ['SPEC_DIR'])
spec_slug = os.environ['SPEC_SLUG']
log_path = Path(os.environ['LOG'])
fmt = os.environ.get('FORMAT', 'text')

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
                dod_items.append((str(sf.relative_to(spec_dir.parent)), i, m.group(1)))
    except Exception:
        continue

# IMP-047 S-C (AC13). SEMANTIC admission, not token overlap.
#
# The retired rule matched a DoD item to any exit-zero command sharing two
# non-stopword tokens with it. Two tokens is a coincidence; this bridge reported
# it as coverage, and the guard admitted it as evidence. The five bindings below
# replace it, and the token path is gone rather than kept alongside.
STOP = {'the', 'and', 'for', 'with', 'this', 'that', 'from', 'into', 'have',
        'has', 'was', 'were', 'are', 'its', 'via', 'per', 'all', 'any', 'each'}
TOK_RE = re.compile(r'[a-zA-Z][a-zA-Z0-9._/-]{2,}')
POINTER_RE = re.compile(r'receipt:\s*([A-Za-z][A-Za-z0-9_.-]*)', re.IGNORECASE)
FAILURE_RE = re.compile(r'\b(fails?|failing|refus\w+|rejects?|blocked)\b', re.IGNORECASE)

def content(text):
    return {t.lower() for t in TOK_RE.findall(text)} - STOP

def claim_subject(body):
    s = re.sub(r'(?:\u2192\s*)?receipt:\s*[A-Za-z][A-Za-z0-9_.-]*', ' ', body, flags=re.IGNORECASE)
    return re.sub(r'(?:\u2192\s*)?evidence:\s*\S+', ' ', s, flags=re.IGNORECASE)

source_revision = os.environ.get('SOURCE_REVISION', '').strip()

matches = []
unbound = []
for sf, ln, body in dod_items:
    pointer = POINTER_RE.search(body)
    if not pointer:
        unbound.append({"scopeFile": sf, "line": ln, "dodBody": body,
                        "reason": "no `Receipt: <scenarioId>` pointer, so no claim of coverage to verify"})
        continue
    wanted = pointer.group(1)
    subject = claim_subject(body)
    item_toks = content(subject)
    expects_failure = bool(FAILURE_RE.search(subject))
    admitted = None
    for e in entries:
        binding = e.get('scenarioBinding')
        if not isinstance(binding, dict):
            continue
        if (binding.get('scenarioId') or '').strip() != wanted:
            continue
        if not (e.get('cmd') or '').strip():
            continue
        rev = (binding.get('sourceRevision') or '').strip()
        if not rev or (source_revision and rev != source_revision):
            continue
        exit_code = e.get('exitCode')
        if not isinstance(exit_code, int):
            continue
        if expects_failure:
            if exit_code == 0:
                continue
        elif exit_code != 0:
            continue
        claim = (binding.get('claim') or '').strip()
        if not claim or (item_toks - content(claim)):
            continue
        admitted = {
            "scopeFile": sf,
            "line": ln,
            "dodBody": body,
            "cmd": e.get('cmd', ''),
            "ts": e.get('ts', ''),
            "scenarioId": wanted,
            "claim": claim,
            "sourceRevision": rev,
            "exitCode": exit_code,
        }
        break
    if admitted:
        matches.append(admitted)
    else:
        unbound.append({"scopeFile": sf, "line": ln, "dodBody": body,
                        "reason": "no receipt for scenario %r satisfies claim, command, revision and outcome binding" % wanted})

matched_count = len(matches)
total = len(dod_items)
pct = (matched_count * 100) // max(total, 1)

if fmt == "json":
    out = {
        "spec": spec_slug,
        "logPath": str(log_path),
        "logPresent": True,
        "scopeFiles": len(scope_files),
        "dodItems": total,
        "toolLogEntries": len(entries),
        "matchedDodItems": matched_count,
        "coveragePct": pct,
        "matches": matches,
        "unbound": unbound,
    }
    print(json.dumps(out, indent=2))
    sys.exit(0)

# text output
print(f"evidence-tool-log-bridge: {spec_slug}")
print(f"  Tool-call entries (spec={spec_slug}): {len(entries)}")
print(f"  DoD [x] items found in {len(scope_files)} scope file(s): {total}")

if not dod_items:
    print("  (no checked DoD items to bridge)")
    sys.exit(0)

if not entries:
    print("  Coverage: 0% — no tool-log entries for this spec.")
    print("  Advisory only; existing prose-evidence path remains valid.")
    sys.exit(0)

print(f"  Coverage: {pct}% ({matched_count}/{total} DoD items have a semantically bound receipt)")
print("  Admission requires all five bindings: scenario, claim, command, source revision, outcome.")
for u in unbound[:10]:
    print(f"  UNBOUND {u['scopeFile']}:{u['line']} — {u['reason']}")
if len(unbound) > 10:
    print(f"  ... and {len(unbound) - 10} more unbound item(s)")
PY
