#!/usr/bin/env bash
#
# generate-telemetry-reader-map.sh — derive the telemetry producer/reader map
# from declared stores and path constants, and emit it as a generated document
# (IMP-047 S-A).
#
# WHY THIS EXISTS
# ---------------
# Framework health reported fields nobody produced. `retro-framework-health.sh`
# ranked "top failing gates" out of `framework-events.jsonl`, a store that
# carries no gate outcomes at all, while the per-gate outcomes sat unread in
# `gate-hits.jsonl`. The report was not wrong about the data; it was reading a
# store that could never answer the question. Nothing connected a reported field
# to the thing that writes it, so that mismatch was invisible.
#
# The map is DERIVED rather than hand-authored, because a hand-written map is a
# fourth guess about dependencies and the reason this was broken is that the
# association was assumed rather than established.
#
# WHAT IS DERIVED, AND FROM WHAT
# ------------------------------
#   stores     every `storageFile:` declared in the workflow registry, plus every
#              `.specify/runtime|metrics/<file>` path constant appearing in a
#              framework script. A store that exists only in code is still a
#              store; omitting it is how an unreadable plane stays invisible.
#   producer   the registry's own `producer:` field, CONFIRMED against the named
#              script actually containing the store path. A declared producer
#              that does not reference its own store is reported, not trusted.
#   readers    every other script containing the store path.
#
# VALUE CLASSES (closed set)
#   direct      a confirmed producer writes the store and the path is readable
#   derived     the registry names a non-store source (for example values
#               computed from state.json) rather than a file this map can confirm
#   unmeasured  the store is declared but no script references its path, so
#               nothing produces or reads it
#
# `unmeasured` is emitted explicitly. A declared store with no producer is the
# exact shape of a measurement nobody takes, and it must be visible rather than
# absent from the table.
#
# Usage:
#   generate-telemetry-reader-map.sh            write the generated document
#   generate-telemetry-reader-map.sh --check    exit 1 if the document is stale
#   generate-telemetry-reader-map.sh --print    print the computed document body
#
# Exit codes: 0 ok - 1 drift (--check) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODE="write"

usage() {
  sed -n '2,45p' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      MODE="check"
      shift
      ;;
    --print)
      MODE="print"
      shift
      ;;
    --repo-root)
      shift
      REPO_ROOT="${1:?--repo-root requires a path}"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "generate-telemetry-reader-map: unknown argument: $1" >&2
      echo "  This generator has no --skip/--force/--ignore." >&2
      exit 2
      ;;
  esac
done

if [[ ! -f "$REPO_ROOT/bubbles/workflows.yaml" ]]; then
  echo "generate-telemetry-reader-map: SKIP (bubbles/workflows.yaml missing)"
  exit 0
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "generate-telemetry-reader-map: SKIP (python3 not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" "$MODE" <<'PY'
import re
import sys
from pathlib import Path

repo_root = Path(sys.argv[1])
mode = sys.argv[2]

OUT_REL = "docs/generated/telemetry-reader-map.md"
START = "<!-- GENERATED:TELEMETRY_READER_MAP_START"
END = "<!-- GENERATED:TELEMETRY_READER_MAP_END -->"

workflows = repo_root / "bubbles/workflows.yaml"
text = workflows.read_text(encoding="utf-8")

# --- declared stores, producers and sources ---------------------------------
#
# Association is by enclosing block: the nearest preceding key at a lower indent.
# That is exact for this registry's shape and needs no YAML engine, so the
# generator runs in a checkout without PyYAML.
declared = {}
block_stack = []
current_block = None
for raw in text.splitlines():
    if not raw.strip() or raw.lstrip().startswith("#"):
        continue
    indent = len(raw) - len(raw.lstrip())
    m = re.match(r"^\s*([A-Za-z][A-Za-z0-9_]*):\s*$", raw)
    if m:
        while block_stack and block_stack[-1][0] >= indent:
            block_stack.pop()
        block_stack.append((indent, m.group(1)))
        current_block = m.group(1)
        continue
    m = re.match(r"^\s*(storageFile|producer|source|reportCommand):\s*(.+?)\s*$", raw)
    if not m or current_block is None:
        continue
    while block_stack and block_stack[-1][0] >= indent:
        block_stack.pop()
    owner = block_stack[-1][1] if block_stack else current_block
    value = m.group(2).strip().strip('"').strip("'")
    declared.setdefault(owner, {})[m.group(1)] = value

# --- path constants appearing in framework scripts --------------------------
#
# The final character may not be a dot: a store named at the end of a prose
# sentence would otherwise be captured with the sentence's full stop and appear
# as a second, non-existent store.
STORE_RE = re.compile(r"\.specify/(?:runtime|metrics)/[A-Za-z0-9._-]*[A-Za-z0-9_-]")
script_paths = sorted(
    p for p in (repo_root / "bubbles/scripts").rglob("*.sh") if p.is_file()
)
refs = {}
for p in script_paths:
    rel = str(p.relative_to(repo_root))
    body = p.read_text(encoding="utf-8", errors="replace")
    for store in set(STORE_RE.findall(body)):
        refs.setdefault(store, set()).add(rel)

# --- assemble one row per store ---------------------------------------------
stores = {}
for owner, fields in declared.items():
    store = fields.get("storageFile")
    if store:
        stores.setdefault(store, {"owners": [], "producer": "", "source": "", "report": ""})
        stores[store]["owners"].append(owner)
        if fields.get("producer"):
            stores[store]["producer"] = fields["producer"]
        if fields.get("reportCommand"):
            stores[store]["report"] = fields["reportCommand"]
    elif fields.get("source"):
        key = "(derived) " + fields["source"]
        stores.setdefault(key, {"owners": [], "producer": "", "source": fields["source"], "report": ""})
        stores[key]["owners"].append(owner)

for store in refs:
    stores.setdefault(store, {"owners": [], "producer": "", "source": "", "report": ""})

rows = []
for store in sorted(stores):
    info = stores[store]
    referencing = sorted(refs.get(store, set()))
    producer = info["producer"]
    producer_confirmed = bool(producer) and producer in referencing
    if info["source"] and not referencing:
        value_class = "derived"
        producer_cell = info["source"]
    elif producer_confirmed:
        value_class = "direct"
        producer_cell = producer
    elif referencing:
        # Nothing declared a producer, but scripts do carry the path. The
        # producer is whichever of them writes it; this map does not guess which,
        # so the store is direct with an undeclared producer.
        value_class = "direct"
        producer_cell = "undeclared"
    else:
        value_class = "unmeasured"
        producer_cell = producer if producer else "none"
    readers = [r for r in referencing if r != producer]
    rows.append((store, producer_cell, readers, value_class, info["owners"], info["report"]))

lines = [
    "# Telemetry Producer/Reader Map",
    "",
    "GENERATED — do not edit by hand. Run `bash bubbles/scripts/generate-telemetry-reader-map.sh`.",
    "",
    "Every telemetry store the framework declares or references, with the surface",
    "that produces it and the surfaces that read it. A reported field whose store",
    "has no producer is a measurement nobody takes; this table exists so that case",
    "is visible instead of absent.",
    "",
    "| Store | Producer | Readers | Value class | Declared by |",
    "|---|---|---|---|---|",
]
for store, producer_cell, readers, value_class, owners, _report in rows:
    reader_cell = "<br>".join(f"`{r}`" for r in readers) if readers else "—"
    owner_cell = ", ".join(f"`{o}`" for o in sorted(set(owners))) if owners else "code only"
    producer_fmt = "—" if producer_cell in ("none",) else f"`{producer_cell}`"
    lines.append(f"| `{store}` | {producer_fmt} | {reader_cell} | {value_class} | {owner_cell} |")

counts = {}
for _, _, _, value_class, _, _ in rows:
    counts[value_class] = counts.get(value_class, 0) + 1
summary = ", ".join(f"{k} {counts[k]}" for k in sorted(counts))
lines += [
    "",
    f"{len(rows)} store(s): {summary}.",
    "",
    "`direct` — a script carries the store path and produces or reads it.",
    "`derived` — the registry names a non-store source rather than a file.",
    "`unmeasured` — declared but no surface references the path: nothing produces",
    "or reads it, so any field reported from it is unbacked.",
]

body = "\n".join(lines) + "\n"
document = (
    START + " — run bash bubbles/scripts/generate-telemetry-reader-map.sh -->\n"
    + body
    + END + "\n"
)

if mode == "print":
    sys.stdout.write(document)
    sys.exit(0)

out_path = repo_root / OUT_REL
current = out_path.read_text(encoding="utf-8") if out_path.is_file() else None

if mode == "check":
    if current is None:
        print(f"generate-telemetry-reader-map: DRIFT — {OUT_REL} is missing")
        print("  Run: bash bubbles/scripts/generate-telemetry-reader-map.sh")
        sys.exit(1)
    if current != document:
        print(f"generate-telemetry-reader-map: DRIFT — {OUT_REL} is stale or hand-edited")
        print("  Run: bash bubbles/scripts/generate-telemetry-reader-map.sh")
        sys.exit(1)
    print(f"generate-telemetry-reader-map: {OUT_REL} is current ({len(rows)} stores: {summary})")
    sys.exit(0)

if current == document:
    print(f"generate-telemetry-reader-map: no change ({len(rows)} stores: {summary})")
    sys.exit(0)

out_path.parent.mkdir(parents=True, exist_ok=True)
out_path.write_text(document, encoding="utf-8")
print(f"generate-telemetry-reader-map: wrote {OUT_REL} ({len(rows)} stores: {summary})")
sys.exit(0)
PY
