#!/usr/bin/env bash
#
# gate-telemetry-capability-lint.sh — refuse a preventionEvidence criterion on
# a gate that cannot structurally emit the telemetry it depends on
# (IMP-058 SCOPE-3 / COV-24).
#
# WHY THIS EXISTS
# ----------------
# SCOPE-2 added `preventionEvidence: { minRuns, prevented, sourceClass }` as a
# second, satisfiable retirement criterion, read from gate-hit-log.sh's
# product telemetry. That telemetry has exactly one production producer today:
# state-transition-guard.sh's `pass`/`fail` helpers extract any `GNNN`
# substring from their own message text (see record_gate_ids_from_message)
# and feed it into the per-run bubbles_gate_hit_append call. A gate whose
# enforcement never reaches one of those tagged pass/fail calls can never
# accumulate a record, no matter how many times it is evaluated -- its
# preventionEvidence criterion would be permanently, silently unsatisfiable.
#
# The SCOPE-3 investigation found three distinct reasons a gate can be in
# that state, and this lint distinguishes them rather than treating "no
# telemetry" as one uniform problem:
#
#   1. Enforced by state-transition-guard.sh, but no pass/fail call in that
#      file mentions the gate's own id. FIXABLE with a message-text edit
#      (this is what SCOPE-3 fixed for G026, G063, G072, G074).
#   2. Enforced only by a DIFFERENT script (e.g. framework-validate.sh,
#      batch-promotion-lint.sh). framework-validate.sh's own check loop does
#      not call gate-hit-log.sh append anywhere -- confirmed by source
#      inspection, not assumption -- so no message-text edit inside that
#      other script would help; the gap is architectural (a second
#      integration point does not exist yet).
#   3. Enforced only behaviorally (an agent instruction file, no script at
#      all). There is no execution point to hook a telemetry call into
#      without inventing a self-report mechanism, which is exactly the kind
#      of unverified claim this framework's evidence rules refuse to trust.
#
# This lint enforces the one thing SCOPE-3 can responsibly guarantee today:
# a gate in class 2 or 3 must never be allowed to declare preventionEvidence,
# because doing so would create a criterion that reads as measurable but can
# never be satisfied. It does NOT require every modelCompensation gate to be
# telemetry-capable -- classes 2 and 3 are real, open, and larger than a
# single-session fix; see improvements/IMP-058-*.md SCOPE-3 for the full
# gate-by-gate classification and what each remaining gap would require.
#
# Usage: gate-telemetry-capability-lint.sh
# Exit codes: 0 clean - 1 findings - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GATES="${BUBBLES_GATES_FILE:-$REPO_ROOT/bubbles/registry/gates.yaml}"
GUARD="${BUBBLES_GUARD_FILE:-$REPO_ROOT/bubbles/scripts/state-transition-guard.sh}"

if [[ ! -f "$GATES" ]]; then
  echo "gate-telemetry-capability-lint: SKIP (gates registry missing)"
  exit 0
fi

if ! command -v python3 >/dev/null 2>&1 || ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "gate-telemetry-capability-lint: SKIP (python3 + PyYAML not installed)"
  exit 0
fi

# state-transition-guard.sh `source`s its guards/*.sh fragments unconditionally
# (the M4 split -- "sourced in this shell scope so behavior is byte-identical"
# per that file's own comments), so a pass/fail call inside a fragment reaches
# the SAME record_gate_ids_from_message/bubbles_gate_hit_append pipeline as one
# written directly in state-transition-guard.sh. Derive the fragment list from
# the guard's own `source "$SCRIPT_DIR/guards/X.sh"` lines rather than globbing
# the directory, so a fragment the guard does not actually source is not
# silently credited as reachable.
GUARD_DIR="$(cd "$(dirname "$GUARD")" && pwd)"
GUARD_FRAGMENTS="$( { grep -oE 'source "\$SCRIPT_DIR/guards/[A-Za-z0-9._-]+\.sh"' "$GUARD" 2>/dev/null \
  | sed -E 's#.*guards/([A-Za-z0-9._-]+\.sh)".*#'"$GUARD_DIR"'/guards/\1#'; } || true)"

python3 - "$GATES" "$GUARD" "$GUARD_FRAGMENTS" <<'PY'
import re
import sys
from pathlib import Path

import yaml

gates_path = Path(sys.argv[1])
guard_path = Path(sys.argv[2])
fragment_paths = [Path(p) for p in sys.argv[3].split("\n") if p.strip()]

data = yaml.safe_load(gates_path.read_text()) or {}
gates = data.get("gates") or {}
derived = ((data.get("gateEnforcement") or {}).get("derived")) or {}

guard_lines = []
for p in [guard_path] + fragment_paths:
    if p.is_file():
        guard_lines.extend(p.read_text().split("\n"))

def tagged_in_guard(gid):
    """A pass/fail call (string possibly spanning to the next line's closing
    quote) that mentions this exact gate id, mirroring
    record_gate_ids_from_message's own (G\\d{3}) extraction."""
    for i, line in enumerate(guard_lines):
        if not re.match(r'^\s*(pass|fail)\s+"', line):
            continue
        buf = line
        j = i
        while buf.count('"') < 2 and j < len(guard_lines) - 1:
            j += 1
            buf += " " + guard_lines[j]
        if re.search(rf'\b{gid}\b', buf):
            return True
    return False

findings = []
checked = 0
for gid in sorted(k for k in gates if re.fullmatch(r"G\d{3}", str(k))):
    meta = gates[gid]
    if not isinstance(meta, dict) or str(meta.get("classification")) != "modelCompensation":
        continue
    if not isinstance(meta.get("preventionEvidence"), dict):
        continue
    checked += 1
    enforced_by = ((derived.get(gid) or {}).get("enforcedBy")) or []
    # Both the guard itself and any of its unconditionally-sourced
    # guards/*.sh fragments reach the same telemetry pipeline (see the
    # fragment-derivation comment above), so either counts as guard-reachable.
    guard_enforced = any(
        e == "script:bubbles/scripts/state-transition-guard.sh"
        or re.fullmatch(r"script:bubbles/scripts/guards/[A-Za-z0-9._-]+\.sh", e)
        for e in enforced_by
    )
    capable = guard_enforced and tagged_in_guard(gid)
    if not capable:
        reason = ("not enforced by state-transition-guard.sh (enforced by a "
                   "different script or behaviorally only -- no telemetry "
                   "integration point exists there)"
                   if not guard_enforced else
                   "enforced by state-transition-guard.sh but no pass/fail "
                   "call in that file mentions this gate id")
        findings.append(f"FINDING: telemetry-incapable-criterion: {gid} declares "
                         f"preventionEvidence but {reason}")

for line in findings:
    print(line)

if findings:
    print(f"[gate-telemetry-capability-lint] FAIL — {len(findings)} gate(s) "
          f"declare preventionEvidence without a working telemetry path")
    sys.exit(1)

print(f"[gate-telemetry-capability-lint] OK — all {checked} gate(s) declaring "
      f"preventionEvidence have a verified telemetry path in "
      f"state-transition-guard.sh")
sys.exit(0)
PY
