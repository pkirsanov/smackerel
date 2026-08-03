#!/usr/bin/env bash
#
# gate-retirement.sh — record and check the retirement criterion for every
# modelCompensation gate (IMP-027 / SCOPE-11: the obsolescence curve).
#
# WHY THIS EXISTS
# ---------------
# SCOPE-2c classified all 112 gates. 24 are `modelCompensation`: they exist
# because models fabricate evidence, batch-check DoD items, and claim phases
# they never ran. Those gates SHOULD get cheaper to carry as models improve —
# but "should" is not a plan. Without a written-down retirement criterion the
# cost of the gate set is permanently pinned to the weakest model the framework
# ever had to survive, because nobody can say what would have to be true to
# turn any single gate off.
#
# This tool makes that criterion explicit, per gate, in the registry:
#
#   retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate,
#                 threshold: 0.005, window: 50 }
#
# Read as: "this gate becomes a candidate for retirement once, on a model of at
# least <minTier>, the measured <metric> stays below <threshold> across
# <window> independent runs."
#
# ─────────────────────────────────────────────────────────────────────────────
# THE MEASUREMENT LOOP DOES NOT EXIST YET — NOTHING RETIRES AUTOMATICALLY
# ─────────────────────────────────────────────────────────────────────────────
# Every `metric` below is a rate observed over MODEL RUNS. The golden-task
# corpus delivered in SCOPE-5 is the SCORER for such runs, but it is not the
# runner: it grades static artifacts with deterministic check types
# (`contains`, `not-contains`, `file-exists`, `executable-oracle`) and never
# invokes a model. So it can tell you whether a delivered artifact is honest;
# it cannot yet tell you how often a given model tier produces a dishonest one.
#
# Until a harness exists that (a) drives a model at a declared tier across the
# corpus, (b) collects each run's output, and (c) aggregates the rate, EVERY
# criterion here is UNMEASURED. Therefore:
#
#   * this tool has NO `retire` subcommand and cannot disable a gate;
#   * `model-tier-advisory.sh retirement` reports CANDIDACY only, and states
#     the unmet evidence precondition every time;
#   * a gate is retired by a human editing the registry, with the measurement
#     attached — never by a tool inferring that models got better.
#
# Recording the criterion is still the valuable half: it converts "someday
# these might be unnecessary" into a falsifiable, reviewable target.
#
# SUBCOMMANDS
#   bind     seed `retireWhen` for modelCompensation gates that lack it
#   lint     fail when a modelCompensation gate has no criterion, when a
#            criterion is malformed, or when a gate that can never retire
#            (businessInvariant / hybrid) carries one
#   report   print the obsolescence curve grouped by tier and metric
#
# Exit codes: 0 ok - 1 findings (lint) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

usage() {
  cat >&2 <<'USAGE'
Usage: gate-retirement.sh [bind|lint|report]

  bind     seed `retireWhen` for modelCompensation gates that lack it
  lint     fail on a missing, malformed, or illegally-placed criterion
  report   print the obsolescence curve

Records WHAT WOULD HAVE TO BE MEASURED before a model-compensation gate could
be retired. It never retires a gate: the measurement loop that would produce
those rates does not exist yet, and no flag here can substitute for it.
USAGE
}

SUBCOMMAND="${1:-lint}"
case "$SUBCOMMAND" in
  bind | lint | report) ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    echo "gate-retirement: unknown subcommand: $SUBCOMMAND (expected bind|lint|report)" >&2
    exit 2
    ;;
esac

# BUBBLES_GATES_FILE override exists for hermetic selftests; defaults in-tree.
GATES="${BUBBLES_GATES_FILE:-$REPO_ROOT/bubbles/registry/gates.yaml}"
if [[ ! -f "$GATES" ]]; then
  echo "gate-retirement: SKIP (gates registry missing)"
  exit 0
fi
# Resolve the managed interpreter before probing (bubbles/scripts/python-env.sh).
_gr_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
[[ -f "$_gr_dir/dependency-posture.sh" ]] && . "$_gr_dir/dependency-posture.sh"
unset _gr_dir

if ! command -v python3 >/dev/null 2>&1 || ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "gate-retirement: SKIP (python3 + PyYAML not installed)"
  exit 0
fi

python3 - "$GATES" "$SUBCOMMAND" <<'PY'
import re
import sys
from pathlib import Path

import yaml

gates_path = Path(sys.argv[1])
subcommand = sys.argv[2]

VALID_TIERS = ("haiku-class", "sonnet-class", "opus-class")

# Retirement classes. Each is (name, regex signals, criterion). The class is
# derived from the gate's own description so a reviewer can check the call
# rather than trust the tool; `bind` prints the signal that fired.
#
# ANCHOR ON THE ENFORCEMENT SUBJECT, NOT ON THE RATIONALE. Nearly every
# modelCompensation gate mentions "fabricated" somewhere in its justification,
# so matching that word assigns two thirds of the registry to one class and
# tells you nothing. The patterns below quote the thing each gate actually
# INSPECTS (`status column`, `completedScopes`, `route-required`), which is
# what determines the metric you would have to measure to retire it. Order is
# most-specific first.
#
# Threshold and window scale with BLAST RADIUS, not with how annoying the gate
# is. A gate standing between the framework and a fabricated completion claim
# needs a tighter bound over a longer window than one catching a formatting
# bypass, because the failure it prevents is unrecoverable: a false `done` is
# not noticed later, it is BELIEVED later.
CLASSES = [
    (
        "framework-self-evidence",
        [r"framework MUST prove", r"Retrospectives MUST surface",
         r"framework-health", r"dogfood"],
        dict(minTier="opus-class", metric="framework-evidence-omission-rate",
             threshold=0.01, window=30),
    ),
    (
        "process-discipline",
        [r"single git commit", r"push range", r"sessionBudget",
         r"safety caps"],
        dict(minTier="sonnet-class", metric="process-violation-rate",
             threshold=0.02, window=20),
    ),
    (
        "routing-discipline",
        [r"verified by the orchestrator", r"route-required", r"reopen work",
         r"concrete result", r"result shape"],
        dict(minTier="sonnet-class", metric="routing-omission-rate",
             threshold=0.02, window=20),
    ),
    (
        "coverage-completeness",
        [r"stress tests", r"source Gherkin", r"faithfully represent",
         r"latency SLA"],
        dict(minTier="opus-class", metric="coverage-omission-rate",
             threshold=0.02, window=20),
    ),
    (
        "structural-integrity",
        [r"reformatting", r"MUST agree", r"status column", r"terminal status"],
        dict(minTier="opus-class", metric="format-bypass-rate",
             threshold=0.01, window=30),
    ),
    (
        "completion-truth",
        [r"deferral language", r"completedScopes", r"completedPhaseClaims",
         r"phase records claiming", r"phase claims"],
        dict(minTier="opus-class", metric="false-completion-claim-rate",
             threshold=0.005, window=50),
    ),
    (
        "evidence-authenticity",
        [r"evidence is captured", r"block fabricated evidence",
         r"raw terminal output evidence", r"MUST be executed via",
         r"evidence block recording"],
        dict(minTier="opus-class", metric="fabricated-evidence-rate",
             threshold=0.005, window=50),
    ),
]

raw = gates_path.read_text()
data = yaml.safe_load(raw) or {}
gates = {
    gid: meta
    for gid, meta in (data.get("gates") or {}).items()
    if re.fullmatch(r"G\d{3}", str(gid)) and isinstance(meta, dict)
}
gate_ids = sorted(gates)


def is_model_comp(gid):
    return str(gates[gid].get("classification")) == "modelCompensation"


def derive(gid):
    """Return (criterion, class_name, signal) for a modelCompensation gate."""
    meta = gates[gid]
    text = f"{meta.get('name', '')} {meta.get('description', '')}"
    for name, patterns, criterion in CLASSES:
        for p in patterns:
            if re.search(p, text, re.I):
                return criterion, name, p
    # No signal fired. Fall back to the STRICTEST criterion, never the loosest:
    # an unclassified model-compensation gate is one whose failure mode we have
    # not characterised, and guessing "probably harmless" about an
    # uncharacterised failure is the exact move this framework exists to block.
    return (
        dict(minTier="opus-class", metric="fabricated-evidence-rate",
             threshold=0.005, window=50),
        "evidence-authenticity",
        "(no signal; strictest criterion by default)",
    )


def fmt(c):
    return ("{ minTier: %s, metric: %s, threshold: %s, window: %d }"
            % (c["minTier"], c["metric"], c["threshold"], c["window"]))


if subcommand == "bind":
    lines = raw.splitlines(keepends=True)
    inserts = {}
    current = None
    for i, line in enumerate(lines):
        m = re.match(r"^  (G\d{3}):\s*$", line)
        if m:
            current = m.group(1)
            continue
        if current and re.match(r"^  \S", line):
            current = None
        if current and re.match(r"^    classification:", line):
            if is_model_comp(current) and gates[current].get("retireWhen") is None:
                inserts[i] = current
            current = None

    if not inserts:
        print("gate-retirement bind: every modelCompensation gate already "
              "declares retireWhen (no change)")
        sys.exit(0)

    out = []
    for i, line in enumerate(lines):
        out.append(line)
        if i in inserts:
            gid = inserts[i]
            criterion, cls, signal = derive(gid)
            out.append(f"    retireWhen: {fmt(criterion)}\n")
            print(f"  {gid}: {cls}  (signal: {signal})")
    gates_path.write_text("".join(out))
    print(f"gate-retirement bind: seeded retireWhen for {len(inserts)} gate(s)")
    sys.exit(0)

findings = 0
curve = {}
for gid in gate_ids:
    criterion = gates[gid].get("retireWhen")
    model_comp = is_model_comp(gid)

    if not model_comp:
        if criterion is not None:
            cls = gates[gid].get("classification")
            print(f"FINDING: retirement-illegal: {gid} is '{cls}' and can never "
                  f"retire, but declares retireWhen")
            findings += 1
        continue

    if criterion is None:
        print(f"FINDING: retirement-missing: {gid} is modelCompensation but "
              f"declares no retireWhen (its cost is unbounded in time)")
        findings += 1
        continue
    if not isinstance(criterion, dict):
        print(f"FINDING: retirement-malformed: {gid} retireWhen must be a "
              f"mapping, got {type(criterion).__name__}")
        findings += 1
        continue

    missing = [k for k in ("minTier", "metric", "threshold", "window")
               if k not in criterion]
    if missing:
        print(f"FINDING: retirement-malformed: {gid} retireWhen lacks "
              f"{', '.join(missing)}")
        findings += 1
        continue
    if str(criterion["minTier"]) not in VALID_TIERS:
        print(f"FINDING: retirement-malformed: {gid} minTier "
              f"'{criterion['minTier']}' is not one of {', '.join(VALID_TIERS)}")
        findings += 1
        continue
    try:
        threshold = float(criterion["threshold"])
        window = int(criterion["window"])
    except (TypeError, ValueError):
        print(f"FINDING: retirement-malformed: {gid} threshold/window are not "
              f"numeric")
        findings += 1
        continue
    if not (0 < threshold < 1):
        print(f"FINDING: retirement-malformed: {gid} threshold {threshold} is "
              f"not a rate in (0,1)")
        findings += 1
        continue
    if window < 1:
        print(f"FINDING: retirement-malformed: {gid} window {window} must be "
              f"at least 1 run")
        findings += 1
        continue

    key = (str(criterion["minTier"]), str(criterion["metric"]),
           threshold, window)
    curve.setdefault(key, []).append(gid)

if subcommand == "report":
    total = sum(len(v) for v in curve.values())
    print(f"gate obsolescence curve — {total} modelCompensation gate(s) with a "
          f"recorded retirement criterion:")
    for (tier, metric, threshold, window) in sorted(curve):
        members = curve[(tier, metric, threshold, window)]
        pct = threshold * 100
        print(f"  {metric} < {pct:g}% over {window} runs at >= {tier}"
              f"  ({len(members)}): {', '.join(members)}")
    print("")
    print("  MEASURED: none. No harness drives a model across the golden-task")
    print("  corpus and aggregates these rates, so every criterion above is")
    print("  currently UNMET on its evidence precondition. Retirement is a")
    print("  human edit backed by that measurement, never a tool inference.")

if findings:
    print(f"[gate-retirement] FAIL — findings: {findings}")
    sys.exit(1)

covered = sum(len(v) for v in curve.values())
print(f"[gate-retirement] OK — all {covered} modelCompensation gate(s) declare "
      f"a retirement criterion")
sys.exit(0)
PY
