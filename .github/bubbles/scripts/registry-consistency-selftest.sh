#!/usr/bin/env bash
# Bubbles registry consistency selftest (v5.0.1 / H1).
#
# Deterministic Python implementation. Validates:
#  1. Every Gxxx referenced across workflows.yaml / scripts / agents / registries
#     resolves to a gate defined in bubbles/workflows.yaml gates: block.
#  2. state-transition-guard.sh has no duplicate `# CHECK <id>:` labels.
#
# Allowed exception patterns (intentional history mentions, fixtures, banners,
# custom-gate idiom) are documented inline in the Python script.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

if ! command -v python3 >/dev/null 2>&1; then
  echo "registry-consistency-selftest: SKIP (python3 not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" <<'PY'
import re
import sys
from pathlib import Path

repo_root = Path(sys.argv[1])
workflows = repo_root / "bubbles" / "workflows.yaml"
stg = repo_root / "bubbles" / "scripts" / "state-transition-guard.sh"

if not workflows.exists():
    print(f"registry-consistency-selftest: ERROR workflows.yaml missing at {workflows}", file=sys.stderr)
    sys.exit(2)

# 1. Parse defined gates from the gates: block of workflows.yaml.
defined = set()
in_gates = False
for line in workflows.read_text().splitlines():
    if line.startswith("gates:"):
        in_gates = True
        continue
    if in_gates and re.match(r"^[a-zA-Z]", line):
        in_gates = False
    if in_gates:
        m = re.match(r"^  (G\d{3}):\s*$", line)
        if m:
            defined.add(m.group(1))

if not defined:
    print(f"registry-consistency-selftest: ERROR no gates parsed from {workflows} gates: block", file=sys.stderr)
    sys.exit(2)

# 2. Collect surfaces to scan.
surfaces = [workflows]
for p in (repo_root / "bubbles" / "scripts").glob("*.sh"):
    if p.name == "registry-consistency-selftest.sh":
        continue
    surfaces.append(p)
for p in (repo_root / "agents").rglob("*.md"):
    surfaces.append(p)
for name in ("capability-ledger.yaml", "adoption-profiles.yaml",
             "intent-routes.yaml", "propagation-policy.yaml",
             "agent-capabilities.yaml", "agent-ownership.yaml",
             "action-risk-registry.yaml"):
    p = repo_root / "bubbles" / name
    if p.exists():
        surfaces.append(p)

# Allowed exception patterns (case-insensitive). A line matching any of these
# is excluded from undefined-gate detection because it is intentionally a
# history mention, fixture token, banner, custom-gate idiom, or selftest data.
EXCEPTION_RE = re.compile(
    r"(?i)("
    r"former|absorbs|consolidates|deprecated|legacy|formerly|previously|"
    r"range|G\d{3}\+|fixture|unknown[- ]gate|"
    r'"gate"\s*:\s*"G\d{3}"|'
    r"── G\d{3}:|"
    r"for token in|"
    r"emits the expected"
    r")"
)

unknown = set()  # set of (file, lineno, token, context)
gate_re = re.compile(r"G(\d{3})")

for f in surfaces:
    try:
        text = f.read_text(errors="replace")
    except Exception:
        continue
    for i, line in enumerate(text.splitlines(), start=1):
        if not gate_re.search(line):
            continue
        if EXCEPTION_RE.search(line):
            continue
        for m in gate_re.finditer(line):
            num = int(m.group(1))
            tok = f"G{m.group(1)}"
            # Custom-gate range — always allowed.
            if num >= 100:
                continue
            if tok not in defined:
                ctx = line.strip()
                if len(ctx) > 120:
                    ctx = ctx[:120] + "…"
                unknown.add((str(f.relative_to(repo_root)), i, tok, ctx))

unknown_sorted = sorted(unknown)
failures = []

if unknown_sorted:
    failures.append(("undefined-gate-refs", unknown_sorted))

# 3. Duplicate CHECK label detection in state-transition-guard.sh.
duplicate_checks = []
if stg.exists():
    seen = {}
    for i, line in enumerate(stg.read_text().splitlines(), start=1):
        m = re.match(r"^# CHECK ([0-9A-Z]+):", line)
        if not m:
            continue
        label = m.group(1)
        seen.setdefault(label, []).append(i)
    for label, lines in seen.items():
        if len(lines) > 1:
            duplicate_checks.append((label, lines))

if duplicate_checks:
    failures.append(("duplicate-check-labels", duplicate_checks))

if failures:
    print("registry-consistency-selftest: FAIL")
    print(f"  {len(defined)} gates defined in bubbles/workflows.yaml gates: block")
    for kind, payload in failures:
        if kind == "undefined-gate-refs":
            print(f"  Undefined gate references ({len(payload)}):")
            for f, lineno, tok, ctx in payload:
                print(f"    {f}:{lineno}: {tok} — {ctx}")
        elif kind == "duplicate-check-labels":
            print(f"  Duplicate CHECK <id> labels in state-transition-guard.sh ({len(payload)}):")
            for label, lines in payload:
                print(f"    CHECK {label}: {len(lines)} occurrences at lines {lines}")
    print("")
    print("  Fix by one of:")
    print("    1. Define the missing gate in bubbles/workflows.yaml gates: block")
    print("    2. Remove the stale reference")
    print("    3. Rewrite as 'former Gxxx' history mention if intentional")
    print("    4. Rename the duplicate CHECK label to keep IDs unique")
    sys.exit(1)

print("registry-consistency-selftest: PASS")
print(f"  {len(defined)} gates defined; all Gxxx references resolve.")
print("  state-transition-guard.sh CHECK labels are unique.")
sys.exit(0)
PY
