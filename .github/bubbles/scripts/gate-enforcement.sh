#!/usr/bin/env bash
#
# gate-enforcement.sh — bind and verify the `enforcedBy` field on every gate
# in bubbles/registry/gates.yaml (IMP-027 / SCOPE-2a).
#
# WHY THIS EXISTS
# ---------------
# docs/generated/gate-coverage-map.md used to be produced by grepping gate IDs
# out of every script. That inference is unreliable in both directions: a gate
# merely MENTIONED in a comment looked enforced (false positive), and a gate
# enforced by a check that never spells its ID looked unenforced (false
# negative, e.g. G018/G035/G078). It also counted `requiredGates` membership —
# a declaration that a mode needs the gate — as if it were mechanical coverage.
#
# The fix is to make enforcement DECLARED rather than guessed. Each gate carries
# an `enforcedBy` list; the coverage map is generated from that field; and this
# script verifies every declaration actually resolves.
#
# A DECLARED ENFORCER MUST ACTUALLY ENFORCE (IMP-049 / SCOPE-5)
# -------------------------------------------------------------
# File existence alone is too weak a resolution test for `script:`. A gate could
# name a real script that has nothing to do with it, and nothing noticed: on the
# tree that introduced this check, 11 of 101 `script:` declarations pointed at a
# file that never mentioned the gate id it was declared to enforce, so the
# binding was hand-maintained prose. `lint` therefore also requires the named
# file to NAME the gate id.
#
# SCOPE, deliberately narrow. This asks ONLY the strict question: a gate that
# ITSELF declares `script:<path>` must have that path name that gate. It does
# NOT grep the script tree to decide whether a gate is enforced somewhere, which
# is a different and unsound measurement (it counts guard-check, behavioral,
# mode-required and CI surfaces as absence). Do not widen it into that.
#
# VOCABULARY (constrained)
#   guard-check:<N>        a labeled check in state-transition-guard.sh
#   script:<path>          a script that enforces the gate
#   ci:<workflow>          a GitHub workflow that enforces it
#   mode-required          declared in a mode's requiredGates, no dedicated
#                          mechanical enforcer  (NOT mechanical coverage)
#   behavioral:<agent>     enforced by agent behavior, by design
#   unbound                NO enforcement surface found. Deliberately visible:
#                          this is the honest coverage backlog, not a pass.
#
# SUBCOMMANDS
#   bind     seed `enforcedBy` for gates that lack it, using the strict rule
#            below. NEVER overwrites an existing value, so hand corrections
#            survive re-runs.
#   lint     verify every declared value resolves (the file exists AND names
#            the gate id, the guard check label exists, the agent exists).
#            Exit 1 on any dangling declaration. This is the check 2a asks for.
#   report   print the distribution and the unbound set.
#
# STRICT DERIVATION RULE (bind only; precedence order)
#   1. description names "state-transition-guard.sh Check <N>"   -> guard-check
#   2. a "# CHECK <N>: ... (Gate <ID>)" banner in the guard        -> guard-check
#   3. description names a `<script>.sh` in backticks             -> script
#   4. description says "Enforced by bubbles.<agent>"             -> behavioral
#   5. a script declares the ID in its first 40 lines, or on a
#      line containing enforc/BLOCKING/gate for/implements        -> script
#   6. the ID appears in any requiredGates/inheritedRequiredGates -> mode-required
#   7. otherwise                                                  -> unbound
#
# Generic runners are NEVER treated as enforcers: framework-validate.sh, cli.sh,
# guard-lib.sh, gate-id-grep.sh, gate-catalog-freshness.sh. They invoke checks;
# they do not implement them.
#
# Exit codes: 0 ok - 1 findings (lint) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

SUBCOMMAND="${1:-lint}"
shift || true

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:?--repo-root requires a path}"
      shift
      ;;
    -h | --help)
      sed -n '2,55p' "$0"
      exit 0
      ;;
    *)
      echo "gate-enforcement: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

case "$SUBCOMMAND" in
  bind | lint | report) ;;
  -h | --help)
    sed -n '2,55p' "$0"
    exit 0
    ;;
  *)
    echo "gate-enforcement: unknown subcommand: $SUBCOMMAND (expected bind|lint|report)" >&2
    exit 2
    ;;
esac

GATES="$REPO_ROOT/bubbles/registry/gates.yaml"
if [[ ! -f "$GATES" ]]; then
  echo "gate-enforcement: SKIP (bubbles/registry/gates.yaml missing)"
  exit 0
fi
# Resolve the managed interpreter before probing (bubbles/scripts/python-env.sh).
_ge_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
[[ -f "$_ge_dir/dependency-posture.sh" ]] && . "$_ge_dir/dependency-posture.sh"
unset _ge_dir

if ! command -v python3 >/dev/null 2>&1; then
  echo "gate-enforcement: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "gate-enforcement: SKIP (PyYAML not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" "$SUBCOMMAND" <<'PY'
import re
import sys
from pathlib import Path

import yaml

repo_root = Path(sys.argv[1])
subcommand = sys.argv[2]

GATES = repo_root / "bubbles/registry/gates.yaml"
GUARD = repo_root / "bubbles/scripts/state-transition-guard.sh"
SCRIPTS_DIR = repo_root / "bubbles/scripts"
AGENTS_DIR = repo_root / "agents"

GATE_RE = re.compile(r"\bG\d{3}\b")
GENERIC = {
    "framework-validate.sh",
    "cli.sh",
    "guard-lib.sh",
    "gate-id-grep.sh",
    "gate-catalog-freshness.sh",
    "gate-enforcement.sh",
    "gate-enforcement-selftest.sh",
}

raw = GATES.read_text()
data = yaml.safe_load(raw) or {}
gates = {
    gid: meta
    for gid, meta in (data.get("gates") or {}).items()
    if re.fullmatch(r"G\d{3}", str(gid)) and isinstance(meta, dict)
}
gate_ids = sorted(gates)


def guard_check_labels():
    """Map gate id -> {check labels} from the transition guard."""
    out = {}
    if not GUARD.is_file():
        return out
    banner = re.compile(r"(?:---\s*Check|#\s*CHECK)\s+([0-9]+[A-Za-z]?)\b")
    for line in GUARD.read_text().splitlines():
        m = banner.search(line)
        if not m:
            continue
        for gid in set(GATE_RE.findall(line)):
            out.setdefault(gid, set()).add(m.group(1))
    return out


def declaring_scripts():
    """Map gate id -> {script basenames} that DECLARE the gate, not merely mention it."""
    out = {}
    verb = re.compile(r"enforc|BLOCKING|gate for|implements", re.I)
    for sf in sorted(SCRIPTS_DIR.rglob("*.sh")):
        if sf.name in GENERIC or sf.resolve() == GUARD.resolve():
            continue
        try:
            lines = sf.read_text().splitlines()
        except (OSError, UnicodeDecodeError):
            continue
        ids = set(GATE_RE.findall("\n".join(lines[:40])))
        for ln in lines:
            if verb.search(ln):
                ids |= set(GATE_RE.findall(ln))
        for gid in ids:
            out.setdefault(gid, set()).add(sf.name)
    return out

def mode_required_ids():
    out = set()
    for rel in ("bubbles/workflows/modes.yaml", "bubbles/workflows.yaml"):
        p = repo_root / rel
        if not p.exists():
            continue
        text = p.read_text()
        for key in ("requiredGates", "inheritedRequiredGates"):
            for m in re.finditer(key + r":\s*\[([^\]]*)\]", text):
                out |= set(GATE_RE.findall(m.group(1)))
        try:
            d = yaml.safe_load(text) or {}
        except yaml.YAMLError:
            continue
        for cfg in (d.get("modes") or {}).values():
            if isinstance(cfg, dict):
                for k in ("requiredGates", "inheritedRequiredGates"):
                    for gid in cfg.get(k) or []:
                        out.add(str(gid))
    return out


checks = guard_check_labels()
decl = declaring_scripts()
required = mode_required_ids()

# basename -> repo-relative path. Resolved RECURSIVELY: several enforcers were
# extracted into bubbles/scripts/guards/, so a flat lookup produced dangling
# `script:` declarations for gates whose enforcer very much exists.
script_paths = {}
for p in sorted(SCRIPTS_DIR.rglob("*.sh")):
    script_paths.setdefault(p.name, p.relative_to(repo_root).as_posix())
all_scripts = set(script_paths)
GUARD_TEXT = GUARD.read_text() if GUARD.is_file() else ""


def guard_label_exists(label):
    return bool(
        re.search(r"(?:---\s*Check|#\s*CHECK)\s+" + re.escape(label) + r"\b", GUARD_TEXT)
    )


def derive(gid):
    meta = gates[gid]
    desc = str(meta.get("description") or "")

    # A description may name a check label that was since renamed or extracted
    # into bubbles/scripts/guards/. Only trust it if the label still exists.
    m = re.search(r"state-transition-guard\.sh`?\s+Check\s+([0-9]+[A-Za-z]?)", desc)
    if m and guard_label_exists(m.group(1)):
        return ["guard-check:" + m.group(1)]

    if gid in checks:
        return ["guard-check:" + c for c in sorted(checks[gid])]

    named = [
        Path(x).name
        for x in re.findall(r"`([a-zA-Z0-9_./-]+\.sh)`", desc)
    ]
    named = sorted({x for x in named if x in all_scripts and x not in GENERIC})
    if named:
        return ["script:" + script_paths[x] for x in named]

    m = re.search(r"[Ee]nforced by `?(bubbles\.[a-z-]+)", desc)
    if m:
        return ["behavioral:" + m.group(1)]

    found = sorted(
        {
            x
            for x in decl.get(gid, set())
            if not x.endswith("-selftest.sh") and x not in GENERIC
        }
    )
    if found:
        return ["script:" + script_paths[x] for x in found]

    if gid in required:
        return ["mode-required"]

    return ["unbound"]


def declared(gid):
    v = gates[gid].get("enforcedBy")
    if v is None:
        return None
    return [str(v)] if isinstance(v, str) else [str(x) for x in v]


# --------------------------------------------------------------------------
if subcommand == "bind":
    lines = raw.splitlines(keepends=True)
    inserts = {}  # line index (0-based) after which to insert -> rendered block
    current = None
    for i, line in enumerate(lines):
        m = re.match(r"^  (G\d{3}):\s*$", line)
        if m:
            current = m.group(1)
            continue
        if current and re.match(r"^  \S", line):
            current = None
        if current and re.match(r"^    name:", line):
            if declared(current) is None:
                inserts[i] = current
            current = None

    if not inserts:
        print("gate-enforcement bind: every gate already declares enforcedBy (no change)")
        sys.exit(0)

    out = []
    for i, line in enumerate(lines):
        out.append(line)
        if i in inserts:
            gid = inserts[i]
            vals = derive(gid)
            out.append("    enforcedBy: [" + ", ".join(vals) + "]\n")
    GATES.write_text("".join(out))
    print(f"gate-enforcement bind: seeded enforcedBy for {len(inserts)} gate(s)")
    sys.exit(0)

# --------------------------------------------------------------------------
findings = 0


def report_finding(kind, msg):
    global findings
    print(f"FINDING: {kind}: {msg}")
    findings += 1


dist = {}
unbound = []
for gid in gate_ids:
    vals = declared(gid)
    if vals is None:
        report_finding("enforcedBy-missing", f"{gid} does not declare enforcedBy")
        continue
    for v in vals:
        kind = v.split(":", 1)[0]
        dist[kind] = dist.get(kind, 0) + 1
        if kind == "unbound":
            unbound.append(gid)
        elif kind == "guard-check":
            label = v.split(":", 1)[1]
            if not GUARD.is_file():
                report_finding("guard-missing", f"{gid} declares {v} but the guard script is absent")
            elif not re.search(
                r"(?:---\s*Check|#\s*CHECK)\s+" + re.escape(label) + r"\b",
                GUARD.read_text(),
            ):
                report_finding("guard-check-unresolved", f"{gid} declares {v} but no such check label exists")
        elif kind == "script":
            rel = v.split(":", 1)[1]
            target = repo_root / rel
            if not target.is_file():
                report_finding("script-unresolved", f"{gid} declares {v} but that file does not exist")
            elif gid not in target.read_text(errors="replace"):
                report_finding(
                    "script-gate-id-absent",
                    f"{gid} declares {v} but that file never names {gid} "
                    f"(add the gate id to the script, or drop the declaration)",
                )
        elif kind == "behavioral":
            agent = v.split(":", 1)[1]
            if AGENTS_DIR.is_dir() and not (AGENTS_DIR / f"{agent}.agent.md").is_file():
                report_finding("agent-unresolved", f"{gid} declares {v} but that agent file does not exist")
        elif kind not in ("mode-required", "ci"):
            report_finding("vocabulary-invalid", f"{gid} declares '{v}' which is outside the constrained vocabulary")

if subcommand == "report":
    print("gate enforcement distribution:")
    for k in sorted(dist):
        print(f"  {k}: {dist[k]}")
    if unbound:
        print(f"  unbound gates: {' '.join(sorted(set(unbound)))}")

if findings:
    print(f"[gate-enforcement] FAIL — findings: {findings}")
    sys.exit(1)

print(f"[gate-enforcement] OK — {len(gate_ids)} gate(s) declare a resolvable enforcedBy")
sys.exit(0)
PY
