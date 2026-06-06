#!/usr/bin/env python3
"""Read-only G088 backfill analysis (OPS-002 tooling).

For each done spec, determine the Gate G088 (post_certification_spec_edit_gate)
remediation class WITHOUT mutating anything:

  - MISSING       : no top-level `certifiedAt` field (G088 short-circuits)
  - STALE         : has `certifiedAt` but a post-cert planning edit exists
  - CLEAN-BACKFILL: faithful historical `certifiedAt` clears the gate (no later
                    planning-truth commit)
  - NEEDS-RECERT  : planning truth (spec.md/design.md/scopes.md) was committed
                    AFTER the faithful historical cert timestamp; a faithful
                    recert requires per-spec review (cannot be rubber-stamped)
  - NO-SOURCE     : no derivable historical cert timestamp at all

The "faithful historical cert timestamp" is sourced, in priority order, from:
  1. latest executionHistory entry with statusAfter == "done" (runEndedAt)
  2. certification.certifiedAt
  3. certification.completedAt
  4. top-level completedAt
  5. lastUpdatedAt

Usage:
  python3 analysis.py specs/001-* specs/002-* ...
  python3 analysis.py            # scans all specs/NNN-* folders

This script writes nothing. It is the evidence generator for the OPS-002
remediation contract; it does NOT perform the recert (that is per-spec,
review-gated work — see report.md).
"""
import glob
import json
import os
import subprocess
import sys
from datetime import datetime


def parse_ts(s):
    if not s:
        return None
    try:
        return datetime.fromisoformat(s.replace("Z", "+00:00"))
    except ValueError:
        return None


def faithful_cert_ts(state):
    best = None
    for entry in state.get("executionHistory", []) or []:
        if entry.get("statusAfter") == "done":
            ts = parse_ts(entry.get("runEndedAt") or entry.get("timestamp"))
            if ts and (best is None or ts > best):
                best = ts
    if best:
        return best, "executionHistory.done"
    cert = state.get("certification", {}) or {}
    for key, label in (("certifiedAt", "certification.certifiedAt"),
                       ("completedAt", "certification.completedAt")):
        ts = parse_ts(cert.get(key))
        if ts:
            return ts, label
    for key in ("completedAt", "lastUpdatedAt"):
        ts = parse_ts(state.get(key))
        if ts:
            return ts, key
    return None, "NONE"


def latest_planning_commit(spec_dir):
    files = [f"{spec_dir}/spec.md", f"{spec_dir}/design.md", f"{spec_dir}/scopes.md"]
    out = subprocess.run(
        ["git", "log", "-1", "--format=%cI", "--", *files],
        capture_output=True, text=True, check=False,
    )
    return parse_ts(out.stdout.strip()) if out.stdout.strip() else None


def main(argv):
    specs = argv or sorted(
        d for d in glob.glob("specs/[0-9]*") if os.path.isdir(d)
    )
    classes = {"CLEAN-BACKFILL": [], "NEEDS-RECERT": [], "NO-SOURCE": [],
               "NOT-DONE": [], "ALREADY-OK": []}
    for spec_dir in specs:
        spec_dir = spec_dir.rstrip("/")
        try:
            state = json.load(open(f"{spec_dir}/state.json"))
        except (OSError, ValueError) as exc:
            print(f"{spec_dir}: ERROR {exc}")
            continue
        if state.get("status") != "done":
            classes["NOT-DONE"].append(spec_dir)
            continue
        if state.get("certifiedAt"):
            classes["ALREADY-OK"].append(spec_dir)
            continue
        cert_ts, src = faithful_cert_ts(state)
        if cert_ts is None:
            classes["NO-SOURCE"].append(spec_dir)
            print(f"{spec_dir}: NO-SOURCE")
            continue
        plan_ts = latest_planning_commit(spec_dir)
        if plan_ts and plan_ts > cert_ts:
            classes["NEEDS-RECERT"].append(spec_dir)
            print(f"{spec_dir}: NEEDS-RECERT cert={cert_ts.date()} ({src}) "
                  f"plan-commit={plan_ts.date()}")
        else:
            classes["CLEAN-BACKFILL"].append((spec_dir, cert_ts.isoformat(), src))
            print(f"{spec_dir}: CLEAN-BACKFILL cert={cert_ts.isoformat()} ({src})")

    print()
    for name in ("CLEAN-BACKFILL", "NEEDS-RECERT", "NO-SOURCE"):
        print(f"{name}: {len(classes[name])}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
