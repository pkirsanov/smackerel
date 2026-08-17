#!/usr/bin/env bash
set -euo pipefail

# report-sections-resolve.sh
#
# IMP-047 S-B — the ONE reader of bubbles/registry/report-sections.yaml.
#
# WHY THIS EXISTS
# `artifact-lint.sh` and `state-transition-guard.sh` each carried their own
# hand-maintained copy of the report-section lists, and `report-section-autofix.sh`
# carried a third. Three copies, three answers, and a template that matched none
# of them. Rather than have each script grow its own parser (which would recreate
# the drift in parser form), both CALL this resolver and consume its flat
# key=value output. `artifact-lint.sh` deliberately sources no sibling library,
# so this is invoked as a subprocess, not sourced.
#
# Output lines (stable, greppable, one fact per line):
#   always=<heading>|<shallowOk>            required in every report.md
#   enforceStatus=<s1> <s2> ...             statuses that activate mode gates
#   mode=<workflowMode>|<h1>;<h2>;...       promotion sections for that mode
#   strictMode=<workflowMode>               mode requiring populated sections
#   strictStatus=<status>                   status requiring populated sections
#   strict=<heading>|<ownerAgent>           a section that must be populated
#
# There is NO fallback list. When the registry is missing or unreadable this
# exits non-zero and prints nothing, so a caller cannot silently degrade to an
# empty requirement set — an empty requirement set is a false-PASS, which is the
# same defect class as IMP-047 PD-04.
#
# python3, not awk: the registry needs nested-block parsing, and the portable way
# to express that in awk needs 3-argument match(), which BSD awk does not have
# (macos-portability-guard class-16). python3 is already a hard dependency of the
# sibling resolvers.
#
# Exit codes:
#   0  resolved
#   2  usage error / registry missing / registry unparseable

REGISTRY=""

die_usage() {
  printf 'report-sections-resolve: %s\n' "$1" >&2
  printf 'usage: report-sections-resolve.sh [--registry FILE]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --registry) shift; REGISTRY="${1:-}" ;;
    -h|--help) sed -n '4,37p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported" ;;
    *) die_usage "unknown option '$1'" ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -n "$REGISTRY" ]] || REGISTRY="$SCRIPT_DIR/../registry/report-sections.yaml"
[[ -f "$REGISTRY" ]] || die_usage "registry not found: $REGISTRY"
command -v python3 >/dev/null 2>&1 || die_usage "python3 is required to read the registry"

REGISTRY="$REGISTRY" python3 - <<'PY'
import os, re, sys

path = os.environ["REGISTRY"]
with open(path, encoding="utf-8") as fh:
    lines = fh.read().split("\n")

group = None
always = []              # (heading, shallowOk)
promo_order = []
promo_owner = {}         # heading -> owner
promo_id = {}            # section id -> heading
cur_id = None
cur_heading = None
cur_shallow = False
enforce_status = []
mode_sections = []       # (mode, [section ids])
cur_group_ids = None
in_modes = False
strict_mode = None
strict_status = None
strict_ids = []


def bracket_list(text):
    inner = text[text.index("[") + 1:text.rindex("]")]
    return [item.strip() for item in inner.split(",") if item.strip()]


def flush_always():
    global cur_heading, cur_shallow, cur_id
    if cur_heading:
        always.append((cur_heading, cur_shallow))
    cur_heading, cur_shallow, cur_id = None, False, None


def flush_promo():
    global cur_heading, cur_id
    if cur_heading:
        promo_order.append(cur_heading)
        promo_owner.setdefault(cur_heading, "")
        if cur_id:
            promo_id[cur_id] = cur_heading
    cur_heading, cur_id = None, None


for raw in lines:
    if re.match(r"^[A-Za-z]", raw):
        if group == "always":
            flush_always()
        elif group == "promo":
            flush_promo()
        head = raw.split(":", 1)[0]
        group = {"alwaysRequired": "always",
                 "promotionSections": "promo",
                 "modeRequired": "modeReq",
                 "strictPopulated": "strict"}.get(head)
        in_modes = False
        continue

    if group is None:
        continue

    item = re.match(r"^\s+-\s+id:\s*(\S+)\s*$", raw)

    if group == "always":
        if item:
            flush_always()
            cur_id = item.group(1)
            continue
        m = re.match(r"^\s+heading:\s*(.+?)\s*$", raw)
        if m:
            cur_heading = m.group(1)
            continue
        if re.match(r"^\s+acceptShallow:\s*true\s*$", raw):
            cur_shallow = True
        continue

    if group == "promo":
        if item:
            flush_promo()
            cur_id = item.group(1)
            continue
        m = re.match(r"^\s+heading:\s*(.+?)\s*$", raw)
        if m:
            cur_heading = m.group(1)
            continue
        m = re.match(r"^\s+owner:\s*(\S+)\s*$", raw)
        if m and cur_heading:
            promo_owner[cur_heading] = m.group(1)
        continue

    if group == "modeReq":
        if re.match(r"^\s+enforceWhenStatusIn:\s*\[.*\]\s*$", raw):
            enforce_status = bracket_list(raw)
            continue
        if re.match(r"^\s+sections:\s*\[.*\]\s*$", raw):
            cur_group_ids = bracket_list(raw)
            in_modes = False
            continue
        if re.match(r"^\s+modes:\s*$", raw):
            in_modes = True
            continue
        m = re.match(r"^\s+-\s+([A-Za-z0-9._-]+)\s*$", raw)
        if in_modes and m and cur_group_ids is not None:
            mode_sections.append((m.group(1), cur_group_ids))
        continue

    if group == "strict":
        m = re.match(r"^\s+whenWorkflowMode:\s*(\S+)\s*$", raw)
        if m:
            strict_mode = m.group(1)
            continue
        m = re.match(r"^\s+whenStatus:\s*(\S+)\s*$", raw)
        if m:
            strict_status = m.group(1)
            continue
        if re.match(r"^\s+sections:\s*\[.*\]\s*$", raw):
            strict_ids = bracket_list(raw)
        continue

if group == "always":
    flush_always()
elif group == "promo":
    flush_promo()

if not always or not promo_order or not enforce_status or not mode_sections:
    print(f"report-sections-resolve: {path} is missing a required block "
          "(alwaysRequired / promotionSections / modeRequired / its mode list)",
          file=sys.stderr)
    sys.exit(2)

unknown = [sid for _, ids in mode_sections for sid in ids if sid not in promo_id]
unknown += [sid for sid in strict_ids if sid not in promo_id]
if unknown:
    print(f"report-sections-resolve: {path} references undeclared section id(s): "
          f"{sorted(set(unknown))}", file=sys.stderr)
    sys.exit(2)

out = []
for heading, shallow in always:
    out.append(f"always={heading}|{'yes' if shallow else 'no'}")
out.append("enforceStatus=" + " ".join(enforce_status))
for mode, ids in mode_sections:
    out.append(f"mode={mode}|" + ";".join(promo_id[sid] for sid in ids))
if strict_mode:
    out.append(f"strictMode={strict_mode}")
if strict_status:
    out.append(f"strictStatus={strict_status}")
for sid in strict_ids:
    heading = promo_id[sid]
    out.append(f"strict={heading}|{promo_owner.get(heading, '')}")

print("\n".join(out))
PY
