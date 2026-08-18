#!/usr/bin/env bash
set -euo pipefail

# generate-report-template.sh
#
# IMP-047 S-B — generate the report.md template from bubbles/registry/report-sections.yaml.
#
# WHY THIS EXISTS
# The shipped report.md template contained zero occurrences of `Completion
# Statement`, `Validation Evidence`, `Audit Evidence`, or `Chaos Evidence`, while
# `artifact-lint.sh` required all four. The framework shipped a template that
# failed its own lint, then shipped `report-section-autofix.sh` to inject the
# missing headings after the fact.
#
# A generated template cannot disagree with its schema. This script rewrites the
# block between the GENERATED markers in feature-templates.md from the registry,
# so a report.md authored from the canonical template passes artifact-lint on
# FIRST WRITE, in any workflow mode, with no autofix step.
#
# Writes nothing unless the rendered block differs. --check reports drift and
# exits non-zero without writing, matching the regen-derived.sh convention.
#
# Exit codes:
#   0  in sync (or written)
#   1  drift under --check, or a malformed target
#   2  usage error / unreadable registry

MODE="write"
REPO_ROOT=""

die_usage() {
  printf 'generate-report-template: %s\n' "$1" >&2
  printf 'usage: generate-report-template.sh [--check] [--repo-root DIR]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) MODE="check" ;;
    --repo-root) shift; REPO_ROOT="${1:-}" ;;
    -h|--help) sed -n '4,26p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported" ;;
    *) die_usage "unknown option '$1'" ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi
[[ -d "$REPO_ROOT" ]] || die_usage "repo root not found: $REPO_ROOT"

REGISTRY="$REPO_ROOT/bubbles/registry/report-sections.yaml"
TARGET="$REPO_ROOT/agents/bubbles_shared/feature-templates.md"

[[ -f "$REGISTRY" ]] || die_usage "registry not found: $REGISTRY"
[[ -f "$TARGET" ]] || die_usage "target not found: $TARGET"
command -v python3 >/dev/null 2>&1 || die_usage "python3 is required"

REGISTRY="$REGISTRY" TARGET="$TARGET" MODE="$MODE" python3 - <<'PY'
import os, re, sys

registry_path = os.environ["REGISTRY"]
target_path = os.environ["TARGET"]
mode = os.environ["MODE"]

START = "<!-- GENERATED:REPORT_TEMPLATE_START — do not edit by hand; run bubbles/scripts/generate-report-template.sh -->"
END = "<!-- GENERATED:REPORT_TEMPLATE_END -->"


def load_registry(path):
    """Minimal reader for the exact shape report-sections.yaml declares.

    PyYAML is deliberately not required: every other framework generator runs on
    a stock interpreter, and adding an optional dependency to the ONE authority
    that the template depends on would reintroduce the failure class PD-04 just
    removed — a check that silently degrades when its tool is absent.
    """
    sections = {}          # id -> {heading, body, acceptShallow}
    order = {"alwaysRequired": [], "templateOnly": [], "promotionSections": []}
    group = None
    current_id = None
    body_lines = None
    body_indent = None

    with open(path, encoding="utf-8") as fh:
        raw = fh.read().split("\n")

    for line in raw:
        stripped = line.strip()

        if body_lines is not None:
            if stripped == "" or line.startswith(body_indent):
                body_lines.append(line[len(body_indent):] if line.startswith(body_indent) else "")
                continue
            sections[current_id]["body"] = "\n".join(body_lines).rstrip("\n")
            body_lines = None
            body_indent = None

        m = re.match(r"^(alwaysRequired|templateOnly|promotionSections):\s*$", line)
        if m:
            group = m.group(1)
            current_id = None
            continue

        if group is None:
            continue

        if re.match(r"^[A-Za-z]", line):
            group = None
            current_id = None
            continue

        m = re.match(r"^\s+-\s+id:\s*(\S+)\s*$", line)
        if m:
            current_id = m.group(1)
            sections[current_id] = {"heading": None, "body": "", "acceptShallow": False}
            order[group].append(current_id)
            continue

        if current_id is None:
            continue

        m = re.match(r"^\s+heading:\s*(.+?)\s*$", line)
        if m:
            sections[current_id]["heading"] = m.group(1)
            continue

        m = re.match(r"^\s+acceptShallow:\s*true\s*$", line)
        if m:
            sections[current_id]["acceptShallow"] = True
            continue

        m = re.match(r"^(\s+)body:\s*\|\s*$", line)
        if m:
            body_lines = []
            body_indent = m.group(1) + "  "
            continue

    if body_lines is not None:
        sections[current_id]["body"] = "\n".join(body_lines).rstrip("\n")

    return sections, order


sections, order = load_registry(registry_path)

emitted = order["alwaysRequired"][:1] + order["templateOnly"] + order["alwaysRequired"][1:] \
    + order["promotionSections"]
# Summary first, then the diff evidence, then the always-required remainder, then
# the promotion sections. Ordering is presentational; no reader depends on it.
seen = []
for sid in emitted:
    if sid not in seen:
        seen.append(sid)

missing = [sid for sid in seen if not sections.get(sid, {}).get("heading")]
if missing:
    print(f"generate-report-template: registry section(s) without a heading: {missing}",
          file=sys.stderr)
    sys.exit(2)

lines = [START, "", "```markdown", "# Execution Reports", "",
         "Single-file mode: use top-level `report.md`.",
         "Per-scope mode: use `scopes/NN-name/report.md` for each scope.", "",
         "Links: [uservalidation.md](uservalidation.md)", "",
         "## Scope: [scope-name] - [YYYY-MM-DD HH:MM]", ""]

for sid in seen:
    sec = sections[sid]
    lines.append(f"### {sec['heading']}")
    body = sec["body"].rstrip("\n")
    if body:
        lines.append(body)
    lines.append("")

lines.append("```")
lines.append("")
lines.append("Every section above is emitted by "
             "`bubbles/scripts/generate-report-template.sh` from "
             "[`bubbles/registry/report-sections.yaml`](../../bubbles/registry/report-sections.yaml), "
             "which is the same authority `artifact-lint.sh` and "
             "`state-transition-guard.sh` read. A report authored from this "
             "template satisfies the section checks on first write; there is no "
             "autofix step and no second list to keep in sync.")
lines.append("")
lines.append(END)

block = "\n".join(lines)

with open(target_path, encoding="utf-8") as fh:
    current = fh.read()

if START not in current or END not in current:
    print("generate-report-template: feature-templates.md is missing the GENERATED "
          "markers; add them around the report.md template block", file=sys.stderr)
    sys.exit(1)

start_at = current.index(START)
end_at = current.index(END) + len(END)
updated = current[:start_at] + block + current[end_at:]

if updated == current:
    print("[generate-report-template] OK — feature-templates.md report block is in sync")
    sys.exit(0)

if mode == "check":
    print("generate-report-template: DRIFT — the report.md template block does not "
          "match bubbles/registry/report-sections.yaml; run "
          "bubbles/scripts/generate-report-template.sh", file=sys.stderr)
    sys.exit(1)

with open(target_path, "w", encoding="utf-8") as fh:
    fh.write(updated)
print("[generate-report-template] WROTE — feature-templates.md report block regenerated "
      "from bubbles/registry/report-sections.yaml")
sys.exit(0)
PY
