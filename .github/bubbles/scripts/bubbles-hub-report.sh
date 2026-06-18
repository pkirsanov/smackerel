#!/usr/bin/env bash
# File: bubbles-hub-report.sh
#
# Read-only governance blast-radius hub report (IMP-014). Composes the framework's
# OWN dependency edges from the authoritative in-repo SSTs/source — never from a
# fuzzy re-derivation — and ranks the most-depended-on nodes (the "hubs"), so the
# blast radius of a change is visible BEFORE the change rather than after a failed
# release-check.
#
# Node kinds + edge sources (every edge is provenance-tagged with its origin):
#   * script        a bubbles/scripts[/guards]/*.sh           in-degree = how many
#                   other scripts/agents reference it  (provenance: script-call)
#   * shared-module a agents/bubbles_shared/*.md              in-degree = how many
#                   agents/scripts include it          (provenance: shared-include)
#   * gate          a Gxxx from bubbles/registry/gates.yaml   in-degree = how many
#                   workflow/script lines reference it (provenance: gate-ref)
#
# In-degree counts DISTINCT SOURCE FILES (so multiple references from one file
# count once) — the truest "how many things depend on this" measure. Deterministic:
# no LLM, no network; identical inputs produce identical output.
#
# Usage: bubbles-hub-report.sh [--top N] [--node <id>] [--format text|json]
#                              [--root <path>] [-h]
#   --top N         show the top N hubs (default 20).
#   --node <id>     print the reverse-dependency set for one node (a script
#                   basename, a bubbles_shared/X.md, or a Gxxx) instead of the
#                   ranking.
#   --format text   human report (default).  --format json   machine-readable.
#   --root <path>   repo root to analyze (default: derived from script location).
#
# Exit: 0 = report emitted (hub findings are INFORMATIONAL — this is not a gate);
#       2 = usage / runtime / malformed-input error. NEVER exits 1.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

TOP=20
NODE=""
FORMAT="text"
ROOT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --top)
      TOP="${2:-}"
      shift 2 || {
        echo "bubbles-hub-report: --top needs a value" >&2
        exit 2
      }
      ;;
    --top=*)
      TOP="${1#*=}"
      shift
      ;;
    --node)
      NODE="${2:-}"
      shift 2 || {
        echo "bubbles-hub-report: --node needs a value" >&2
        exit 2
      }
      ;;
    --node=*)
      NODE="${1#*=}"
      shift
      ;;
    --format)
      FORMAT="${2:-}"
      shift 2 || {
        echo "bubbles-hub-report: --format needs a value" >&2
        exit 2
      }
      ;;
    --format=*)
      FORMAT="${1#*=}"
      shift
      ;;
    --root)
      ROOT="${2:-}"
      shift 2 || {
        echo "bubbles-hub-report: --root needs a value" >&2
        exit 2
      }
      ;;
    --root=*)
      ROOT="${1#*=}"
      shift
      ;;
    -h | --help)
      sed -n '2,33p' "$0"
      exit 0
      ;;
    *)
      echo "bubbles-hub-report: unknown argument '$1'." >&2
      exit 2
      ;;
  esac
done

case "$FORMAT" in
  text | json) ;;
  *)
    echo "bubbles-hub-report: --format must be 'text' or 'json' (got '$FORMAT')." >&2
    exit 2
    ;;
esac
if [[ ! "$TOP" =~ ^[0-9]+$ ]] || [[ "$TOP" -lt 1 ]]; then
  echo "bubbles-hub-report: --top must be a positive integer (got '$TOP')." >&2
  exit 2
fi

if [[ -z "$ROOT" ]]; then
  if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
    ROOT="${SCRIPT_DIR%/bubbles/scripts}"
  else
    ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  fi
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "bubbles-hub-report: python3 unavailable — cannot compose the report." >&2
  exit 2
fi

BUBBLES_HUB_ROOT="$ROOT" BUBBLES_HUB_TOP="$TOP" BUBBLES_HUB_NODE="$NODE" \
  BUBBLES_HUB_FORMAT="$FORMAT" python3 - <<'PYEOF'
import json
import os
import re
import sys

root = os.environ["BUBBLES_HUB_ROOT"]
top = int(os.environ["BUBBLES_HUB_TOP"])
node = os.environ["BUBBLES_HUB_NODE"]
fmt = os.environ["BUBBLES_HUB_FORMAT"]


def rel(p):
    return os.path.relpath(p, root)


# --- Discover known target nodes from the SSTs/source ----------------------
scripts = {}  # basename -> rel path
for sub in ("bubbles/scripts", "bubbles/scripts/guards"):
    d = os.path.join(root, sub)
    if not os.path.isdir(d):
        continue
    for name in sorted(os.listdir(d)):
        if name.endswith(".sh"):
            scripts.setdefault(name, f"{sub}/{name}")

shared = {}  # basename -> rel path
shared_dir = os.path.join(root, "agents", "bubbles_shared")
if os.path.isdir(shared_dir):
    for name in sorted(os.listdir(shared_dir)):
        if name.endswith(".md"):
            shared[name] = f"agents/bubbles_shared/{name}"

gates = set()
gates_yaml = os.path.join(root, "bubbles", "registry", "gates.yaml")
if os.path.isfile(gates_yaml):
    with open(gates_yaml, encoding="utf-8") as fh:
        for line in fh:
            m = re.match(r"^  (G\d{3}):", line)
            if m:
                gates.add(m.group(1))

# --- Source surfaces to scan for references --------------------------------
sources = []  # list of (rel_path, kind)
for sub in ("bubbles/scripts", "bubbles/scripts/guards"):
    d = os.path.join(root, sub)
    if not os.path.isdir(d):
        continue
    for name in sorted(os.listdir(d)):
        if name.endswith(".sh"):
            sources.append((f"{sub}/{name}", "script"))
agents_dir = os.path.join(root, "agents")
if os.path.isdir(agents_dir):
    for name in sorted(os.listdir(agents_dir)):
        if name.endswith(".agent.md"):
            sources.append((f"agents/{name}", "agent"))
    for name in sorted(shared):
        sources.append((shared[name], "shared"))
wf = os.path.join(root, "bubbles", "workflows.yaml")
if os.path.isfile(wf):
    sources.append(("bubbles/workflows.yaml", "workflow"))

script_re = re.compile(r"([A-Za-z0-9][A-Za-z0-9._-]*\.sh)\b")
shared_re = re.compile(r"([A-Za-z0-9][A-Za-z0-9._-]*\.md)\b")
gate_re = re.compile(r"\b(G\d{3})\b")

# edges: target_id -> { provenance -> set(source_rel) }, and a flat list for --node
indeg_sources = {}  # target -> set of distinct source rels
edges = []  # (source_rel, target_id, provenance, lineno)
kind_of = {}  # target -> node kind


def add_edge(src_rel, target, provenance, lineno, kind):
    if target == os.path.basename(src_rel):
        return  # no self-loop
    edges.append((src_rel, target, provenance, lineno))
    indeg_sources.setdefault(target, set()).add(src_rel)
    kind_of[target] = kind


for src_rel, src_kind in sources:
    abspath = os.path.join(root, src_rel)
    try:
        with open(abspath, encoding="utf-8", errors="replace") as fh:
            lines = fh.readlines()
    except OSError:
        continue
    self_base = os.path.basename(src_rel)
    for i, line in enumerate(lines, start=1):
        for m in script_re.finditer(line):
            b = m.group(1)
            if b in scripts and b != self_base:
                add_edge(src_rel, b, "script-call", i, "script")
        for m in shared_re.finditer(line):
            b = m.group(1)
            if b in shared and b != self_base:
                prov = "agent-include" if src_kind in ("agent", "shared") else "shared-include"
                add_edge(src_rel, b, prov, i, "shared-module")
        for m in gate_re.finditer(line):
            g = m.group(1)
            if g in gates:
                prov = "workflow-gate" if src_kind == "workflow" else "gate-ref"
                add_edge(src_rel, g, prov, i, "gate")

# --- --node reverse-dependency mode ----------------------------------------
if node:
    deps = sorted(
        [(s, p, ln) for (s, t, p, ln) in edges if t == node],
        key=lambda x: (x[0], x[2], x[1]),
    )
    if fmt == "json":
        print(json.dumps({
            "node": node,
            "kind": kind_of.get(node),
            "inDegree": len(indeg_sources.get(node, set())),
            "dependents": [
                {"source": s, "provenance": p, "line": ln} for (s, p, ln) in deps
            ],
        }, indent=2))
    else:
        print(f"Reverse dependencies of '{node}' (kind: {kind_of.get(node) or 'unknown'})")
        print(f"  in-degree (distinct source files): {len(indeg_sources.get(node, set()))}")
        if not deps:
            print("  (no dependents found — not referenced by any scanned surface)")
        for s, p, ln in deps:
            print(f"  {s}:{ln}  [{p}]")
    sys.exit(0)

# --- Ranking mode ----------------------------------------------------------
ranked = sorted(
    indeg_sources.items(),
    key=lambda kv: (-len(kv[1]), kv[0]),
)
ranked = ranked[:top]


def provenance_breakdown(target):
    counts = {}
    for (s, t, p, ln) in edges:
        if t == target:
            counts[p] = counts.get(p, 0) + 1
    return counts


if fmt == "json":
    print(json.dumps({
        "root": rel(root) if rel(root) != "." else ".",
        "nodeCounts": {
            "scripts": len(scripts),
            "sharedModules": len(shared),
            "gates": len(gates),
        },
        "edgeCount": len(edges),
        "topHubs": [
            {
                "node": t,
                "kind": kind_of.get(t),
                "inDegree": len(srcs),
                "provenance": provenance_breakdown(t),
            }
            for (t, srcs) in ranked
        ],
    }, indent=2))
else:
    print("Bubbles governance hub report (read-only, SST-derived; informational)")
    print(f"  nodes: {len(scripts)} scripts, {len(shared)} shared-modules, {len(gates)} gates")
    print(f"  edges: {len(edges)} (provenance-tagged)")
    print(f"  top {len(ranked)} hubs by in-degree (distinct source files):")
    print()
    print(f"  {'in-deg':>6}  {'kind':<13}  node")
    print(f"  {'-' * 6}  {'-' * 13}  {'-' * 40}")
    for (t, srcs) in ranked:
        print(f"  {len(srcs):>6}  {(kind_of.get(t) or '?'):<13}  {t}")
    print()
    print("  Use --node <id> for the exact reverse-dependency set of any node.")

sys.exit(0)
PYEOF
