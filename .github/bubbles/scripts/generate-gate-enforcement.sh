#!/usr/bin/env bash
#
# generate-gate-enforcement.sh — derive per-gate `enforcedBy` and blocking
# status from evidence and splice them into bubbles/registry/gates.yaml between
# GENERATED markers (IMP-047 S-A).
#
# WHY THIS EXISTS
# ---------------
# Two questions about the gate registry had no trustworthy answer.
#
#   1. "What enforces this gate?"  `enforcedBy` was hand-written, and a
#      hand-written binding drifts. G071 still reads `enforcedBy: [ unbound ]`
#      while the description in the SAME entry names three enforcing agents.
#      G070 carried the identical contradiction until IMP-038 SCOPE-6 repaired
#      it by hand — which is the point: a hand repair fixes one entry and leaves
#      the mechanism that produced it intact.
#
#   2. "How many gates actually block?"  The schema has no `blocking` or
#      `severity` field at all, so the only way to answer was to read 100+
#      enforcing scripts. A framework that cannot state how many of its gates
#      refuse anything cannot argue about retiring any of them.
#
# Both are now DERIVED and emitted into a generated region, so a hand edit inside
# that region fails `--check` instead of quietly becoming a second authority.
#
# WHAT "DERIVED" MEANS HERE, EXACTLY
# ----------------------------------
# `enforcedBy` is derived from `gate-id-grep.sh --emit-refs`, which reports every
# file that references a gate id outside a full-line comment, plus the
# `requiredGates` lists in the workflow registry. Three reference classes are
# treated as bindings:
#
#   script:<path>       a non-selftest script under bubbles/scripts/ that names
#                       the gate id in EXECUTABLE text
#   mode-required       the id appears in a mode's requiredGates list
#   behavioral:<agent>  an agent surface, recorded ONLY when no script binds the
#                       gate — otherwise every gate an agent mentions would list
#                       a dozen behavioral enforcers and the field would stop
#                       meaning anything
#
# Two classes are deliberately NOT bindings, because counting them is how the
# earlier grep-derived coverage map became untrustworthy in both directions:
#
#   documentation   docs/ and instructions/ describe gates; they never enforce
#                   one.
#   selftests       a selftest exercises the enforcer. It is evidence that the
#                   enforcer is tested, not that the selftest enforces the gate.
#                   Several hand-written entries list their selftest sibling, so
#                   the comparison below normalises both sides to real enforcing
#                   scripts rather than reporting that difference as drift.
#
# "Executable text" is load-bearing. A candidate from the scan is CONFIRMED only
# if the id survives a second pass that also drops heredoc bodies — which is
# where `usage()` lives. Without it, `gate-hit-log.sh` binds G001 and G002
# because its own usage string reads `--passed "G001 G002"`, and every registry
# tool binds whatever ids its help text happens to quote. A usage example is not
# an enforcement.
#
# Reference-derivation is still deliberately CONSERVATIVE and is not semantic: a
# script that names a gate id in executable text is recorded as binding it.
# Over-inclusion costs a token in a generated list; under-inclusion hides an
# enforcer.
#
# BLOCKING DERIVATION
# -------------------
# From the enforcing surface's own exit behavior, in this order:
#
#   blocking    a bound script contains a refusing exit (any literal exit whose
#               code is neither 0 nor the usage code 2)
#   blocking    the gate is mode-required or bound to a numbered transition-guard
#               check — the transition guard refuses on a failed required gate
#   advisory    scripts bind the gate but none of them ever exits non-zero
#   unenforced  the registry declares `unbound` and no surface references it
#   unknown     enforcement is behavioral-only, or a declared script is missing,
#               so exit behavior cannot be derived
#
# `unknown` is emitted explicitly rather than guessed. A gate whose blocking
# status cannot be derived must say so; a plausible-looking guess is the failure
# this generator exists to remove.
#
# Usage:
#   generate-gate-enforcement.sh            write the block into gates.yaml
#   generate-gate-enforcement.sh --check    exit 1 if the block is stale or edited
#   generate-gate-enforcement.sh --print    print the computed block
#
# Exit codes: 0 ok - 1 drift (--check) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODE="write"

usage() {
  sed -n '2,70p' "$0"
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
      echo "generate-gate-enforcement: unknown argument: $1" >&2
      echo "  This generator has no --skip/--force/--ignore." >&2
      exit 2
      ;;
  esac
done

GATES="$REPO_ROOT/bubbles/registry/gates.yaml"
GREP_TOOL="$REPO_ROOT/bubbles/scripts/gate-id-grep.sh"

if [[ ! -f "$GATES" ]]; then
  echo "generate-gate-enforcement: SKIP (bubbles/registry/gates.yaml missing)"
  exit 0
fi
if [[ ! -f "$GREP_TOOL" ]]; then
  echo "generate-gate-enforcement: gate-id-grep.sh missing at $GREP_TOOL" >&2
  echo "  Refusing to emit a binding set derived from no evidence." >&2
  exit 2
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "generate-gate-enforcement: SKIP (python3 not installed)"
  exit 0
fi

REFS_FILE="$(mktemp)"
trap 'rm -f "$REFS_FILE"' EXIT INT TERM

if ! bash "$GREP_TOOL" --repo-root "$REPO_ROOT" --emit-refs >"$REFS_FILE"; then
  echo "generate-gate-enforcement: reference scan failed." >&2
  echo "  Refusing to derive bindings from an incomplete scan." >&2
  exit 2
fi

python3 - "$REPO_ROOT" "$MODE" "$REFS_FILE" <<'PY'
import re
import sys
from pathlib import Path

repo_root = Path(sys.argv[1])
mode = sys.argv[2]
refs_file = Path(sys.argv[3])

gates_path = repo_root / "bubbles/registry/gates.yaml"
gates_text = gates_path.read_text(encoding="utf-8")

START = "# GENERATED:GATE_ENFORCEMENT_START"
END = "# GENERATED:GATE_ENFORCEMENT_END"

# Read the registry with line-oriented parsing rather than a YAML loader: this
# generator must run in a checkout that may not have PyYAML, and the two fields
# it needs are unambiguous at the line level.
body_lines = gates_text.splitlines()
generated_start = next((i for i, l in enumerate(body_lines) if l.startswith(START)), None)
generated_end = next((i for i, l in enumerate(body_lines) if l.startswith(END)), None)
scan_upper = generated_start if generated_start is not None else len(body_lines)

gate_ids = []
declared = {}
current = None
for line in body_lines[:scan_upper]:
    m = re.match(r"^  (G\d{3}):\s*$", line)
    if m:
        current = m.group(1)
        gate_ids.append(current)
        declared[current] = []
        continue
    if current is None:
        continue
    m = re.match(r"^    enforcedBy:\s*\[(.*)\]\s*$", line)
    if m:
        declared[current] = [t.strip() for t in m.group(1).split(",") if t.strip()]

if not gate_ids:
    print("generate-gate-enforcement: no gate ids found in the registry", file=sys.stderr)
    sys.exit(2)

# --- reference evidence -----------------------------------------------------
refs = {}
for row in refs_file.read_text(encoding="utf-8").splitlines():
    if "\t" not in row:
        continue
    gid, path = row.split("\t", 1)
    refs.setdefault(gid, set()).add(path)

# --- mode-required evidence -------------------------------------------------
mode_required = set()
for rel in ("bubbles/workflows.yaml", "bubbles/workflows/modes.yaml"):
    p = repo_root / rel
    if not p.is_file():
        continue
    for line in p.read_text(encoding="utf-8").splitlines():
        if "requiredGates" not in line and "delivery-gate-baseline" not in line:
            continue
        mode_required.update(re.findall(r"\bG\d{3}\b", line))

# --- does a script refuse? --------------------------------------------------
# A refusing exit is any literal `exit N` whose code is neither 0 (success) nor
# 2 (the framework's usage/environment code). `exit "$rc"` and `exit $?` are
# propagations, so the script refuses whenever whatever it ran refused.
REFUSING_EXIT = re.compile(r"\bexit\s+(?:\"?\$\{?[A-Za-z_?][^\"\s]*\}?\"?|(\d+))")
script_refuses_cache = {}


def script_refuses(rel_path):
    if rel_path in script_refuses_cache:
        return script_refuses_cache[rel_path]
    p = repo_root / rel_path
    if not p.is_file():
        script_refuses_cache[rel_path] = None
        return None
    verdict = False
    for m in REFUSING_EXIT.finditer(p.read_text(encoding="utf-8", errors="replace")):
        code = m.group(1)
        if code is None:
            verdict = True
            break
        if code not in ("0", "2"):
            verdict = True
            break
    script_refuses_cache[rel_path] = verdict
    return verdict


def agent_token(rel_path):
    name = Path(rel_path).name
    if name.startswith("bubbles.") and name.endswith(".agent.md"):
        return "behavioral:" + name[: -len(".agent.md")]
    return "behavioral:" + rel_path


# --- confirm a candidate binding against executable text --------------------
#
# The scan already dropped full-line comments. This second pass also drops
# heredoc bodies, which is where `usage()` text lives. A gate id quoted in a
# help string is documentation, not enforcement.
HEREDOC_OPEN = re.compile(r"<<-?\s*[\"']?([A-Za-z_][A-Za-z0-9_]*)[\"']?")
executable_ids_cache = {}


def executable_gate_ids(rel_path):
    if rel_path in executable_ids_cache:
        return executable_ids_cache[rel_path]
    p = repo_root / rel_path
    if not p.is_file():
        executable_ids_cache[rel_path] = set()
        return executable_ids_cache[rel_path]
    found = set()
    terminator = None
    for raw in p.read_text(encoding="utf-8", errors="replace").splitlines():
        if terminator is not None:
            if raw.strip() == terminator:
                terminator = None
            continue
        stripped = raw.lstrip()
        if stripped.startswith("#"):
            continue
        found.update(re.findall(r"\bG\d{3}\b", raw))
        m = HEREDOC_OPEN.search(raw)
        if m:
            terminator = m.group(1)
    executable_ids_cache[rel_path] = found
    return found


records = []
for gid in gate_ids:
    seen = refs.get(gid, set())
    scripts = sorted(
        p for p in seen
        if p.startswith("bubbles/scripts/")
        and p.endswith(".sh")
        and not p.endswith("-selftest.sh")
        and gid in executable_gate_ids(p)
    )
    agents = sorted(p for p in seen if p.startswith("agents/"))

    derived = []
    if gid in mode_required:
        derived.append("mode-required")
    derived += ["script:" + p for p in scripts]
    if not scripts:
        # Behavioral enforcement is recorded only when nothing mechanical binds
        # the gate. This is the case that resolves G071 from evidence.
        derived += sorted({agent_token(p) for p in agents})

    declared_tokens = declared.get(gid, [])
    declared_scripts = sorted(
        t[len("script:"):] for t in declared_tokens
        if t.startswith("script:") and not t.endswith("-selftest.sh")
    )
    has_guard_check = any(t.startswith("guard-check:") for t in declared_tokens)

    # --- blocking ---
    missing_declared = [p for p in declared_scripts if not (repo_root / p).is_file()]
    refusing = [p for p in scripts if script_refuses(p)]
    if refusing:
        blocking, basis = "blocking", "script-exit-nonzero"
    elif gid in mode_required:
        blocking, basis = "blocking", "mode-required-transition-refusal"
    elif has_guard_check:
        blocking, basis = "blocking", "guard-check-transition-refusal"
    elif missing_declared:
        blocking, basis = "unknown", "declared-script-absent"
    elif scripts:
        blocking, basis = "advisory", "script-exit-zero-only"
    elif agents:
        blocking, basis = "unknown", "behavioral-only-no-derivable-exit"
    elif declared_tokens == ["unbound"]:
        blocking, basis = "unenforced", "declared-unbound-and-unreferenced"
    else:
        blocking, basis = "unknown", "no-enforcing-surface-found"

    # --- agreement between the hand-written field and the evidence ---
    if declared_tokens == ["unbound"] and derived:
        agreement = "contradiction"
    elif not declared_tokens:
        agreement = "undeclared"
    elif declared_scripts and sorted(scripts) == declared_scripts:
        agreement = "agrees"
    elif not declared_scripts and not scripts:
        agreement = "agrees"
    else:
        agreement = "divergent"

    records.append((gid, derived, blocking, basis, agreement, declared_tokens))


def render_list(items):
    return "[ " + ", ".join(items) + " ]" if items else "[]"


lines = [
    START + " — do not edit by hand; run bubbles/scripts/generate-gate-enforcement.sh",
    "#",
    "# Derived from gate-id-grep.sh --emit-refs plus the workflow registry's",
    "# requiredGates lists. `enforcedBy` here is EVIDENCE; the per-entry field",
    "# above is the hand-written declaration, and `agreement` states whether the",
    "# two match. `blocking` is derived from the enforcing surface's exit",
    "# behaviour and is `unknown` whenever it genuinely cannot be derived.",
    "#",
    "# Entries sit under `derived:` so they are indented one level deeper than the",
    "# registry's own `  Gxxx:` keys. Several readers count gates with a",
    "# two-space-anchored pattern, and a sibling block at that indent would double",
    "# every count in the framework.",
    "gateEnforcement:",
    "  derived:",
]
for gid, derived, blocking, basis, agreement, declared_tokens in records:
    entry = (
        f"    {gid}: {{ enforcedBy: {render_list(derived)}, "
        f"blocking: {blocking}, blockingBasis: {basis}, agreement: {agreement}"
    )
    if agreement in ("contradiction", "divergent", "undeclared"):
        entry += f", declaredEnforcedBy: {render_list(declared_tokens)}"
    entry += " }"
    lines.append(entry)

counts = {}
for _, _, blocking, _, _, _ in records:
    counts[blocking] = counts.get(blocking, 0) + 1
summary = ", ".join(f"{k} {counts[k]}" for k in sorted(counts))
disagreements = sum(1 for r in records if r[4] in ("contradiction", "divergent", "undeclared"))
lines.append("#")
lines.append(f"# {len(records)} gates: {summary}. {disagreements} disagree with their declaration.")
lines.append(END)

block = "\n".join(lines) + "\n"

if mode == "print":
    sys.stdout.write(block)
    sys.exit(0)

if generated_start is not None and generated_end is not None:
    head = "\n".join(body_lines[:generated_start])
    tail = "\n".join(body_lines[generated_end + 1:])
    desired = (head + "\n" if head else "") + block + (tail + "\n" if tail.strip() else "")
    current_block = "\n".join(body_lines[generated_start:generated_end + 1]) + "\n"
elif generated_start is None and generated_end is None:
    desired = gates_text.rstrip("\n") + "\n\n" + block
    current_block = None
else:
    print("generate-gate-enforcement: only one GENERATED marker is present in gates.yaml", file=sys.stderr)
    sys.exit(2)

if mode == "check":
    if current_block is None:
        print("generate-gate-enforcement: DRIFT — the generated enforcement block is absent")
        print("  Run: bash bubbles/scripts/generate-gate-enforcement.sh")
        sys.exit(1)
    if current_block != block:
        print("generate-gate-enforcement: DRIFT — the generated enforcement block is stale or hand-edited")
        print("  Run: bash bubbles/scripts/generate-gate-enforcement.sh")
        sys.exit(1)
    print(f"generate-gate-enforcement: block is current ({len(records)} gates: {summary})")
    sys.exit(0)

if current_block == block:
    print(f"generate-gate-enforcement: no change ({len(records)} gates: {summary})")
    sys.exit(0)

gates_path.write_text(desired, encoding="utf-8")
print(f"generate-gate-enforcement: wrote {len(records)} gates ({summary}); {disagreements} disagree with their declaration")
sys.exit(0)
PY
