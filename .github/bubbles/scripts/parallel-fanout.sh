#!/usr/bin/env bash
#
# parallel-fanout.sh — deterministic reference aggregator + DAG validator for
# the v6.0 / B10 parallel phase fan-out contract (review R8, shipped v6.1).
#
# The parallel-phase DISPATCHER is workflow-agent behavior; THIS script is the
# mechanical reference implementation of the contract's two verifiable pieces:
#
#   aggregate <phase-envelope.json>...   Merge N per-phase result envelopes into
#                                        a single canonical parent envelope,
#                                        honoring the determinism guarantees in
#                                        agents/bubbles_shared/workflow-execution-loops.md
#                                        (stable phase-name ordering, stable
#                                        finding ordering, latest-`at` timestamp,
#                                        failure-preserving outcome). Emits
#                                        canonical JSON (sorted keys) to stdout.
#
#   check-dag <dag.json>                 Given a proposed parallel group, reject
#                                        (exit 1) when any pair violates the DAG:
#                                        a data dependency (one phase reads what
#                                        another writes) or a shared mutable
#                                        write (two phases write the same path
#                                        and are not both idempotent). Exit 0
#                                        when the group is parallel-safe.
#
# Determinism contract (must hold byte-for-byte regardless of input order):
#   1. phases sorted by phase name (alphabetic)
#   2. findings sorted by (specSlug, scopeId, findingId)
#   3. aggregate `at` = MAX individual phase `at`
#   4. outcome = route_required when ANY phase failed/route_required/blocked,
#      accumulating unresolvedFindings from EVERY failed phase; else
#      completed_owned (or completed_diagnostic when all phases are diagnostic)
#
# Exit codes:
#   0 — success (aggregate emitted / DAG is parallel-safe)
#   1 — DAG conflict detected (check-dag) and printed to stderr
#   2 — usage / input error

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  parallel-fanout.sh aggregate <phase-envelope.json> [<phase-envelope.json> ...]
  parallel-fanout.sh check-dag <dag.json>

aggregate: emit a canonical merged parent envelope (deterministic ordering).
check-dag: exit 0 if the parallel group is safe, 1 if a conflict is detected.
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }
OP="$1"; shift

if ! command -v python3 >/dev/null 2>&1; then
  echo "parallel-fanout: SKIP (python3 not installed)" >&2
  exit 0
fi

case "$OP" in
  aggregate)
    [[ $# -lt 1 ]] && { usage; exit 2; }
    OP="aggregate" python3 - "$@" <<'PY'
import json, sys

paths = sys.argv[1:]
phases = []
for p in paths:
    with open(p) as f:
        env = json.load(f)
    phases.append(env)

# Guarantee 1: stable phase ordering by phase name (alphabetic).
def phase_name(e):
    return str(e.get("phase") or e.get("agent") or "")
phases.sort(key=phase_name)

# Guarantee 2: stable finding ordering by (specSlug, scopeId, findingId).
def finding_key(fd):
    return (
        str(fd.get("specSlug", "")),
        str(fd.get("scopeId", "")),
        str(fd.get("findingId", "")),
    )

all_findings = []
unresolved = []
failed_outcomes = {"route_required", "blocked", "failed"}
diagnostic_outcomes = {"completed_diagnostic"}
any_failed = False
all_diagnostic = True

for e in phases:
    outcome = str(e.get("outcome", ""))
    if outcome not in diagnostic_outcomes:
        all_diagnostic = False
    fds = e.get("findings") or []
    for fd in fds:
        all_findings.append(fd)
    if outcome in failed_outcomes:
        any_failed = True
        for fd in fds:
            unresolved.append(fd)

all_findings.sort(key=finding_key)
unresolved.sort(key=finding_key)

# Guarantee 3: aggregate `at` = MAX individual phase `at` (lexical ISO-8601
# compare is correct for Zulu timestamps).
ats = [str(e.get("at", "")) for e in phases if e.get("at")]
agg_at = max(ats) if ats else ""

# Guarantee 4: failure-preserving outcome.
if any_failed:
    outcome = "route_required"
elif all_diagnostic and phases:
    outcome = "completed_diagnostic"
else:
    outcome = "completed_owned"

parent = {
    "schemaVersion": 1,
    "kind": "parallel-fanout-aggregate",
    "outcome": outcome,
    "at": agg_at,
    "phases": [phase_name(e) for e in phases],
    "findings": all_findings,
    "unresolvedFindings": unresolved,
}

# Canonical, deterministic serialization.
sys.stdout.write(json.dumps(parent, sort_keys=True, separators=(",", ":"), ensure_ascii=False))
sys.stdout.write("\n")
PY
    ;;
  check-dag)
    [[ $# -lt 1 ]] && { usage; exit 2; }
    OP="check-dag" python3 - "$1" <<'PY'
import json, sys

with open(sys.argv[1]) as f:
    dag = json.load(f)

phases = dag.get("phases") or []
# Each phase: { "name": str, "reads": [paths], "writes": [paths],
#               "idempotent": bool }
conflicts = []
for i in range(len(phases)):
    for j in range(i + 1, len(phases)):
        a, b = phases[i], phases[j]
        an, bn = a.get("name", f"#{i}"), b.get("name", f"#{j}")
        aw = set(a.get("writes") or [])
        bw = set(b.get("writes") or [])
        ar = set(a.get("reads") or [])
        br = set(b.get("reads") or [])
        # Data dependency: one phase reads what the other writes.
        dep = (ar & bw) | (br & aw)
        if dep:
            conflicts.append(
                f"data-dependency: {an} <-> {bn} on {sorted(dep)}"
            )
        # Shared mutable write: both write the same path and not both idempotent.
        shared = aw & bw
        if shared and not (a.get("idempotent") and b.get("idempotent")):
            conflicts.append(
                f"shared-write: {an} & {bn} both write {sorted(shared)} (not both idempotent)"
            )

if conflicts:
    sys.stderr.write("parallel-fanout: DAG NOT parallel-safe:\n")
    for c in sorted(conflicts):
        sys.stderr.write(f"  - {c}\n")
    sys.exit(1)

print("parallel-fanout: DAG is parallel-safe")
sys.exit(0)
PY
    ;;
  -h|--help)
    usage; exit 0;;
  *)
    echo "parallel-fanout: unknown op: $OP" >&2
    usage
    exit 2;;
esac
