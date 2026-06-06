#!/usr/bin/env python3
"""OPS-002 per-spec post-cert commit classifier (read-only).

For each `done` spec that fails Gate G088, list the commits that touched its
planning truth (spec.md / design.md / scopes.md) AFTER the spec's faithful
historical certification timestamp, and classify whether ALL of them belong to
the known benign documentation-reconciliation campaign.

A spec whose post-cert planning edits are ALL benign-campaign commits can be
faithfully recertified: the review consists of confirming the edit provenance
(documentation reconciliation that did not change requirement/scenario/DoD
semantics). A spec with ANY non-campaign post-cert planning commit is flagged
SUBSTANTIVE and requires deeper per-spec review.

The benign campaign is identified by commit-subject patterns (drift cleanup,
ratchet, OPS banner sweep, scopes-drift ratchet test, migration-evidence
reconciliation, the 20-round stochastic sweep governance commits, and the
release-planning packet). Writes nothing.

Usage:
  python3 classify.py            # all done specs
  python3 classify.py specs/NNN-... ...
"""
import glob
import json
import os
import re
import subprocess
import sys
from datetime import datetime

# Commit-subject patterns for the benign documentation-reconciliation campaign.
BENIGN_SUBJECT_PATTERNS = [
    re.compile(r"drift cleanup", re.I),
    re.compile(r"\bratchet\b", re.I),
    re.compile(r"sweep spec\.md status banners", re.I),
    re.compile(r"OPS-001", re.I),
    re.compile(r"scopes-drift", re.I),
    re.compile(r"reconcile stale migration", re.I),
    re.compile(r"migration-\d+ evidence", re.I),
    re.compile(r"stochastic[- ]quality[- ]sweep", re.I),
    re.compile(r"\bsweep round\b", re.I),
    re.compile(r"sweep-r\d+", re.I),
    re.compile(r"re-certify after", re.I),
    re.compile(r"recertify after", re.I),
    re.compile(r"de-backtick never-authored", re.I),
    re.compile(r"close sweep bug-ceremony", re.I),
    re.compile(r"governance recerts", re.I),
    re.compile(r"OPS-002", re.I),
    re.compile(r"backfill top-level certifiedAt", re.I),
]


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
        return best
    cert = state.get("certification", {}) or {}
    for key in ("certifiedAt", "completedAt"):
        ts = parse_ts(cert.get(key))
        if ts:
            return ts
    for key in ("completedAt", "lastUpdatedAt"):
        ts = parse_ts(state.get(key))
        if ts:
            return ts
    return None


def post_cert_commits(spec_dir, since_ts):
    files = [f"{spec_dir}/spec.md", f"{spec_dir}/design.md", f"{spec_dir}/scopes.md"]
    out = subprocess.run(
        ["git", "log", f"--since={since_ts.isoformat()}", "--format=%h\t%s",
         "--", *files],
        capture_output=True, text=True, check=False,
    )
    rows = []
    for line in out.stdout.splitlines():
        if "\t" in line:
            sha, subj = line.split("\t", 1)
            rows.append((sha, subj))
    return rows


def is_benign(subject):
    return any(p.search(subject) for p in BENIGN_SUBJECT_PATTERNS)


def main(argv):
    specs = argv or sorted(d for d in glob.glob("specs/[0-9]*") if os.path.isdir(d))
    benign_only = []
    substantive = []
    no_source = []
    for spec_dir in specs:
        spec_dir = spec_dir.rstrip("/")
        try:
            state = json.load(open(f"{spec_dir}/state.json"))
        except (OSError, ValueError):
            continue
        if state.get("status") != "done":
            continue
        # Already G088-clean?
        g = subprocess.run(
            ["bash", ".github/bubbles/scripts/post-cert-spec-edit-guard.sh",
             spec_dir, "--quiet"],
            capture_output=True, text=True, check=False,
        )
        if g.returncode == 0:
            continue
        cert_ts = faithful_cert_ts(state)
        if cert_ts is None:
            no_source.append(spec_dir)
            print(f"{spec_dir}: NO-SOURCE")
            continue
        commits = post_cert_commits(spec_dir, cert_ts)
        nonbenign = [(s, j) for (s, j) in commits if not is_benign(j)]
        if not commits:
            # G088 fails but no post-cert planning commit => pure missing field.
            benign_only.append((spec_dir, cert_ts, []))
            print(f"{spec_dir}: MISSING-FIELD-ONLY (no post-cert planning commit)")
        elif not nonbenign:
            benign_only.append((spec_dir, cert_ts, commits))
            print(f"{spec_dir}: BENIGN-ONLY ({len(commits)} post-cert commits, all campaign)")
        else:
            substantive.append((spec_dir, nonbenign))
            print(f"{spec_dir}: SUBSTANTIVE ({len(nonbenign)} non-campaign commits)")
            for sha, subj in nonbenign[:6]:
                print(f"    - {sha} {subj}")

    print()
    print(f"BENIGN-ONLY (faithfully recertifiable): {len(benign_only)}")
    print(f"SUBSTANTIVE (needs deeper review): {len(substantive)}")
    print(f"NO-SOURCE: {len(no_source)}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
