#!/usr/bin/env bash
#
# bubbles generate-gate-coverage-map.sh — gate → enforcing-surface map generator
# (IMP-102 / SCOPE-9).
#
# Canonical sources (all READ-ONLY):
#   - bubbles/registry/gates.yaml          → the set of defined gates (id + name)
#   - bubbles/workflows/modes.yaml         → per-mode `requiredGates` lists
#   - bubbles/workflows.yaml               → per-mode `requiredGates` (union, in
#                                            case the spliced block ever diverges)
#   - bubbles/scripts/state-transition-guard.sh (+ bubbles/scripts/guards/**)
#                                          → guard `--- Check N ---` labels and
#                                            record_passed_gate / record_failed_gate
#   - bubbles/scripts/**/*.sh              → other framework-validate scripts
#                                            (selftests, lints, standalone guards)
#                                            that reference a gate id
#   - .github/workflows/*.yml              → CI entrypoints (framework-validate /
#                                            state-transition-guard) + literal ids
#
# Generated target:
#   docs/generated/gate-coverage-map.md    → each defined gate mapped to the
#                                            surface(s) that enforce it, so the
#                                            gates NOT referenced by any mode's
#                                            requiredGates are demonstrably
#                                            enforced elsewhere (legibility).
#
# This generator is ADVISORY: it never fails because a gate lacks a mode
# reference. It only fails in --check when the committed doc drifts from what the
# current sources would produce (exactly like the other generators).
#
# Usage:
#   generate-gate-coverage-map.sh              # write docs/generated/gate-coverage-map.md
#   generate-gate-coverage-map.sh --check      # exit 0 if committed doc matches;
#                                              # exit 1 if drifted
#   generate-gate-coverage-map.sh --print      # emit regenerated doc to stdout
#
# Exit codes:
#   0 — success (write / check passed / print emitted / graceful SKIP)
#   1 — drift detected in --check mode
#   2 — usage error or missing required input

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GATES="$REPO_ROOT/bubbles/registry/gates.yaml"
OUTPUT="$REPO_ROOT/docs/generated/gate-coverage-map.md"

MODE="write"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) MODE="check"; shift ;;
    --print) MODE="print"; shift ;;
    -h|--help)
      sed -n '2,38p' "$0"
      exit 0
      ;;
    *) echo "generate-gate-coverage-map: unknown arg: $1" >&2; exit 2 ;;
  esac
done

[[ -f "$GATES" ]] || {
  if [[ "$MODE" == "check" ]]; then
    # Downstream repos that predate the registry file won't have it. SKIP
    # rather than FAIL so framework-validate stays green there.
    echo "generate-gate-coverage-map: SKIP (bubbles/registry/gates.yaml missing)"
    exit 0
  fi
  echo "generate-gate-coverage-map: gates registry missing at $GATES" >&2
  exit 2
}

if ! command -v python3 >/dev/null 2>&1; then
  echo "generate-gate-coverage-map: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "generate-gate-coverage-map: SKIP (PyYAML not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" "$OUTPUT" "$MODE" <<'PY'
import re
import sys
from pathlib import Path

import yaml

repo_root = Path(sys.argv[1])
output_path = Path(sys.argv[2])
mode = sys.argv[3]

output_rel = output_path.relative_to(repo_root).as_posix()
self_names = {"generate-gate-coverage-map.sh", "generate-gate-coverage-map-selftest.sh"}
GATE_RE = re.compile(r"\bG\d{3}\b")


def load_yaml(rel):
    path = repo_root / rel
    if not path.exists():
        return {}
    try:
        return yaml.safe_load(path.read_text()) or {}
    except yaml.YAMLError:
        return {}


# 1. Defined gates (id -> name), sorted.
gates_data = load_yaml("bubbles/registry/gates.yaml")
gates = {}
for gid, meta in (gates_data.get("gates") or {}).items():
    if not re.fullmatch(r"G\d{3}", str(gid)):
        continue
    name = ""
    if isinstance(meta, dict):
        name = str(meta.get("name") or "")
    gates[gid] = name
gate_ids = sorted(gates)

# 2. Mode requiredGates (union of modes.yaml + workflows.yaml).
gate_modes = {gid: set() for gid in gates}
for rel in ("bubbles/workflows/modes.yaml", "bubbles/workflows.yaml"):
    data = load_yaml(rel)
    modes = data.get("modes") or {}
    if not isinstance(modes, dict):
        continue
    for mode_name, cfg in modes.items():
        if not isinstance(cfg, dict):
            continue
        req = cfg.get("requiredGates") or []
        if not isinstance(req, list):
            continue
        for gid in req:
            gid = str(gid)
            if gid in gate_modes:
                gate_modes[gid].add(str(mode_name))

# 3. state-transition-guard: labeled `--- Check N ---` + any reference.
guard_checks = {gid: set() for gid in gates}   # gid -> {"3B", "4A", ...}
guard_referenced = {gid: False for gid in gates}
guard_files = [repo_root / "bubbles/scripts/state-transition-guard.sh"]
guards_dir = repo_root / "bubbles/scripts/guards"
if guards_dir.is_dir():
    guard_files.extend(sorted(p for p in guards_dir.rglob("*") if p.is_file()))
check_label_re = re.compile(r"---\s*Check\s+([0-9]+[A-Za-z]?)\b")
for gf in guard_files:
    if not gf.is_file():
        continue
    try:
        text = gf.read_text()
    except (OSError, UnicodeDecodeError):
        continue
    for line in text.splitlines():
        ids_here = set(GATE_RE.findall(line))
        if not ids_here:
            continue
        for gid in ids_here:
            if gid in guard_referenced:
                guard_referenced[gid] = True
        m = check_label_re.search(line)
        if m:
            check_id = m.group(1)
            for gid in ids_here:
                if gid in guard_checks:
                    guard_checks[gid].add(check_id)

# 4. Other framework-validate scripts (selftests / lints / standalone guards),
#    excluding the transition guard + guards/ (their own column) and self.
guard_file_set = {gf.resolve() for gf in guard_files}
fw_scripts = {gid: set() for gid in gates}     # gid -> {basename, ...}
scripts_dir = repo_root / "bubbles/scripts"
for sf in sorted(scripts_dir.rglob("*.sh")):
    if not sf.is_file():
        continue
    if sf.name in self_names:
        continue
    if sf.resolve() in guard_file_set:
        continue
    try:
        text = sf.read_text()
    except (OSError, UnicodeDecodeError):
        continue
    ids_here = set(GATE_RE.findall(text))
    for gid in ids_here:
        if gid in fw_scripts:
            fw_scripts[gid].add(sf.name)

# 5. CI: which entrypoints the workflows run + literal gate ids.
ci_runs_guard = False
ci_runs_fw_validate = False
ci_literal = {gid: False for gid in gates}
wf_dir = repo_root / ".github/workflows"
if wf_dir.is_dir():
    for wf in sorted(wf_dir.glob("*.yml")) + sorted(wf_dir.glob("*.yaml")):
        try:
            text = wf.read_text()
        except (OSError, UnicodeDecodeError):
            continue
        if "state-transition-guard" in text:
            ci_runs_guard = True
        if "framework-validate" in text:
            ci_runs_fw_validate = True
        for gid in GATE_RE.findall(text):
            if gid in ci_literal:
                ci_literal[gid] = True


def guard_cell(gid):
    checks = sorted(guard_checks[gid], key=lambda c: (int(re.match(r"\d+", c).group()), c))
    if checks:
        return ", ".join("Check " + c for c in checks)
    if guard_referenced[gid]:
        return "ref"
    return "—"


def fw_cell(gid):
    n = len(fw_scripts[gid])
    return str(n) if n else "—"


def ci_cell(gid):
    parts = []
    if ci_literal[gid]:
        parts.append("named")
    if guard_referenced[gid] and ci_runs_guard:
        parts.append("guard")
    if fw_scripts[gid] and ci_runs_fw_validate:
        parts.append("fw-validate")
    return ", ".join(dict.fromkeys(parts)) if parts else "—"


def has_any_surface(gid):
    return bool(
        gate_modes[gid]
        or guard_referenced[gid]
        or fw_scripts[gid]
        or ci_literal[gid]
    )


# --- Summary numbers ---
total = len(gate_ids)
with_mode = [g for g in gate_ids if gate_modes[g]]
no_mode = [g for g in gate_ids if not gate_modes[g]]
no_mode_guard = [g for g in no_mode if guard_referenced[g]]
no_mode_fw = [g for g in no_mode if fw_scripts[g]]
no_mode_ci = [g for g in no_mode if ci_cell(g) != "—"]
no_surface = [g for g in gate_ids if not has_any_surface(g)]


def md_escape(text):
    return text.replace("|", "\\|")


lines = []
lines.append("# Gate Coverage Map")
lines.append("")
lines.append("> GENERATED — do not edit by hand.")
lines.append("> Regenerate: `bash bubbles/scripts/generate-gate-coverage-map.sh`")
lines.append("> Check drift: `bash bubbles/scripts/generate-gate-coverage-map.sh --check`")
lines.append("")
lines.append(
    "This page maps every gate defined in `bubbles/registry/gates.yaml` to the "
    "surface(s) that enforce it. It exists so that gates NOT listed in any "
    "workflow mode's `requiredGates` are demonstrably enforced elsewhere "
    "(state-transition-guard checks, framework-validate selftests/guards, or CI) "
    "rather than silently unenforced."
)
lines.append("")
lines.append("Column meanings:")
lines.append("")
lines.append("- **# Modes** — how many `modes.yaml` / `workflows.yaml` modes list the gate in `requiredGates`.")
lines.append("- **state-transition-guard** — the guard's labeled `Check N` that names the gate, `ref` when the gate id appears in the guard or a `bubbles/scripts/guards/**` fragment without a labeled check, else `—`.")
lines.append("- **framework-validate scripts** — count of OTHER `bubbles/scripts/**/*.sh` files (selftests, lints, standalone guards run by `framework-validate.sh`) that reference the gate id (the transition guard is excluded — it has its own column).")
lines.append(
    "- **CI** — how a `.github/workflows/*.yml` transitively enforces the gate: "
    "`guard` (a workflow runs `state-transition-guard`), `fw-validate` (a workflow "
    "runs `framework-validate`), `named` (the gate id appears literally), else `—`."
)
lines.append("")
lines.append(
    "Detection is limited to these MECHANICAL surfaces. A gate with none of them "
    "may still be enforced by AGENT-BEHAVIOR instructions (referenced only under "
    "`agents/**`, e.g. a value-first-selection or evidence-rule gate); such gates "
    "are surfaced under REVIEW so a maintainer can confirm the behavioral-only "
    "enforcement is intentional. This map is advisory — it never blocks on mode "
    "coverage completeness."
)
lines.append("")
lines.append("## Coverage Summary")
lines.append("")
lines.append(f"- Gates defined: **{total}**")
lines.append(f"- Referenced by ≥1 workflow mode: **{len(with_mode)}**")
lines.append(f"- Not referenced by any mode: **{len(no_mode)}**")
lines.append(f"  - of those, enforced by state-transition-guard: **{len(no_mode_guard)}**")
lines.append(f"  - of those, enforced by a framework-validate script: **{len(no_mode_fw)}**")
lines.append(f"  - of those, enforced in CI: **{len(no_mode_ci)}**")
if no_surface:
    lines.append(f"- Gates with NO detected MECHANICAL surface (may be agent-behavior-enforced; REVIEW): **{len(no_surface)}** — {', '.join(no_surface)}")
else:
    lines.append("- Gates with NO detected mechanical surface: **0**")
lines.append("")
lines.append("## All Gates")
lines.append("")
lines.append("| Gate | Name | # Modes | state-transition-guard | framework-validate scripts | CI |")
lines.append("| --- | --- | --- | --- | --- | --- |")
for gid in gate_ids:
    name = md_escape(gates[gid]) or "—"
    lines.append(
        f"| {gid} | {name} | {len(gate_modes[gid])} | {guard_cell(gid)} | {fw_cell(gid)} | {ci_cell(gid)} |"
    )
lines.append("")
lines.append("## Gates Not Referenced By Any Mode")
lines.append("")
lines.append(
    "These gates are intentionally enforced OUTSIDE the mode `requiredGates` "
    "lists. Each row names the concrete non-mode surface(s) that enforce it; a "
    "row reaching the final column with no surfaces would be a genuine gap."
)
lines.append("")
if no_mode:
    lines.append("| Gate | Name | state-transition-guard | framework-validate scripts | CI | Enforcing script files |")
    lines.append("| --- | --- | --- | --- | --- | --- |")
    for gid in no_mode:
        name = md_escape(gates[gid]) or "—"
        scripts_sorted = sorted(fw_scripts[gid])
        if len(scripts_sorted) > 6:
            shown = ", ".join(scripts_sorted[:6]) + f", +{len(scripts_sorted) - 6} more"
        elif scripts_sorted:
            shown = ", ".join(scripts_sorted)
        else:
            shown = "—"
        lines.append(
            f"| {gid} | {name} | {guard_cell(gid)} | {fw_cell(gid)} | {ci_cell(gid)} | {shown} |"
        )
else:
    lines.append("_Every defined gate is referenced by at least one workflow mode._")
lines.append("")

new_content = "\n".join(lines) + "\n"

if mode == "print":
    sys.stdout.write(new_content)
    sys.exit(0)

if mode == "check":
    if not output_path.exists():
        print(f"generate-gate-coverage-map: DRIFT — {output_rel} does not exist", file=sys.stderr)
        print("  Run: bash bubbles/scripts/generate-gate-coverage-map.sh", file=sys.stderr)
        sys.exit(1)
    current = output_path.read_text()
    if current == new_content:
        print(f"generate-gate-coverage-map: {output_rel} is in sync ({total} gates mapped)")
        sys.exit(0)
    print(f"generate-gate-coverage-map: DRIFT — {output_rel} is stale", file=sys.stderr)
    print("  Run: bash bubbles/scripts/generate-gate-coverage-map.sh", file=sys.stderr)
    sys.exit(1)

# write mode
if output_path.exists() and output_path.read_text() == new_content:
    print(f"generate-gate-coverage-map: no change ({total} gates mapped)")
    sys.exit(0)
output_path.parent.mkdir(parents=True, exist_ok=True)
output_path.write_text(new_content)
print(f"generate-gate-coverage-map: wrote {output_rel} ({total} gates mapped)")
PY
