#!/usr/bin/env bash
#
# bundle-cost-report.sh — distance-to-target and a dispatch-weighted cost proxy
# for agent context bundles (IMP-027 / SCOPE-6, COST-1).
#
# WHY THIS EXISTS
# ---------------
# The ratcheting per-agent budget stops bundles GROWING, but it has no notion of
# where they should END UP. Ratchet-only means today's size becomes tomorrow's
# floor: the mechanism institutionalises whatever bloat already exists. An agent
# at 505 KB is "in budget" forever simply because it was 505 KB yesterday.
#
# This adds the missing dimension. Each role class gets a TARGET, and the report
# states the distance to it, so the number is visible instead of implied.
#
# It also reports a cost proxy: bundle_bytes x dispatches. Bundle size alone
# under-states the cost of the orchestrator, which is dispatched far more often
# than any specialist -- the most-invoked agent carrying the largest bundle is
# the actual expense, and neither figure alone shows it.
#
# THIS SCRIPT ONLY REPORTS. It never edits an agent or a budget: the reduction
# it measures is gated on an eval this repo does not yet have (see the R3
# condition in operating-baseline.md), and a reporter that also mutated would
# make it far too easy to chase the number instead of earning it.
#
# Exit codes: 0 always (advisory) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FORMAT="text"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:?--repo-root requires a path}"
      shift
      ;;
    --json)
      FORMAT="json"
      shift
      ;;
    -h | --help)
      sed -n '2,26p' "$0"
      exit 0
      ;;
    *)
      echo "bundle-cost-report: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

AGENTS_DIR="$REPO_ROOT/agents"
[[ -d "$AGENTS_DIR" ]] || AGENTS_DIR="$REPO_ROOT/.github/agents"
if [[ ! -d "$AGENTS_DIR" ]]; then
  echo "bundle-cost-report: SKIP (no agents directory under $REPO_ROOT)"
  exit 0
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "bundle-cost-report: SKIP (python3 not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" "$AGENTS_DIR" "$FORMAT" <<'PY'
import json
import re
import sys
from pathlib import Path

repo_root = Path(sys.argv[1])
agents_dir = Path(sys.argv[2])
fmt = sys.argv[3]

SHARED_RE = re.compile(r"bubbles_shared/([a-z0-9-]+\.md)")

# Role classes and their targets. An orchestrator routes and should carry the
# LEAST reference text; an authoring specialist legitimately carries the most,
# because producing a spec needs the templates in context.
ROLE_TARGETS = [
    ("orchestrator", 160_000, {"workflow", "goal", "sprint", "iterate", "super", "train", "upkeep", "propagate"}),
    ("authoring", 400_000, {"plan", "analyst", "design", "implement", "releases", "ux"}),
    ("specialist", 300_000, set()),  # default
]


def role_of(name: str) -> tuple:
    stem = name.replace("bubbles.", "").replace(".agent.md", "")
    for role, target, members in ROLE_TARGETS:
        if stem in members:
            return role, target
    return ROLE_TARGETS[-1][0], ROLE_TARGETS[-1][1]


def closure_bytes(agent_path: Path) -> int:
    """Static transitive closure over bubbles_shared references."""
    seen = set()
    total = 0
    queue = [agent_path]
    while queue:
        current = queue.pop()
        resolved = current.resolve()
        if resolved in seen or not current.is_file():
            continue
        seen.add(resolved)
        try:
            text = current.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        total += len(text.encode("utf-8"))
        for ref in SHARED_RE.findall(text):
            queue.append(agents_dir / "bubbles_shared" / ref)
    return total


# Dispatch counts, when the runtime has recorded any. Absent = weight 1, which
# keeps the proxy honest rather than inventing traffic that was never observed.
dispatches = {}
events = repo_root / ".specify/runtime/framework-events.jsonl"
if events.is_file():
    try:
        for line in events.read_text(encoding="utf-8").splitlines():
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            agent = rec.get("agent") or rec.get("targetAgent")
            if agent:
                dispatches[str(agent)] = dispatches.get(str(agent), 0) + 1
    except OSError:
        pass

rows = []
for agent_file in sorted(agents_dir.glob("bubbles.*.agent.md")):
    name = agent_file.name
    role, target = role_of(name)
    size = closure_bytes(agent_file)
    stem = name.replace(".agent.md", "")
    count = dispatches.get(stem, 0)
    rows.append(
        {
            "agent": stem,
            "role": role,
            "bytes": size,
            "targetBytes": target,
            "overBy": max(0, size - target),
            "withinTarget": size <= target,
            "dispatches": count,
            # Reachability closure weighted by dispatch. NOT spend: renamed from
            # costProxy because being read as a cost is exactly how IMP-028
            # ended up optimizing a proxy (IMP-039 SCOPE-2).
            "referenceClosureProxy": size * (count if count else 1),
            "dispatchesObserved": count > 0,
        }
    )

rows.sort(key=lambda r: -r["referenceClosureProxy"])

if fmt == "json":
    print(json.dumps({"agents": rows}, indent=2, sort_keys=True))
    sys.exit(0)

over = [r for r in rows if not r["withinTarget"]]
print("Agent bundle cost report (advisory)")
print("")
print(f"  agents measured      : {len(rows)}")
print(f"  within role target   : {len(rows) - len(over)}")
print(f"  over role target     : {len(over)}")
observed = any(r["dispatchesObserved"] for r in rows)
print(f"  dispatch data        : {'observed' if observed else 'none recorded (cost proxy weights all agents equally)'}")
print("")
print(f"  {'agent':<28} {'role':<14} {'bytes':>9} {'target':>9} {'over by':>9}")
for r in rows:
    flag = "" if r["withinTarget"] else "  <-- over"
    print(f"  {r['agent']:<28} {r['role']:<14} {r['bytes']:>9} {r['targetBytes']:>9} {r['overBy']:>9}{flag}")

if over:
    print("")
    print("  These exceed their role target. Reducing an orchestrator by moving")
    print("  authoring modules to phase-local profiles is GATED on a held-out eval")
    print("  showing zero gate-detection regression (operating-baseline.md, R3).")
    print("  Do not rewire an agent reference to chase this number.")
PY
