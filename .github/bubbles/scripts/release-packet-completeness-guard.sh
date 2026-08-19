#!/usr/bin/env bash
# release-packet-completeness-guard.sh — enforces the "no fewer" half of the
# release-packet contract (Gate G138, IMP-050).
#
# agents/bubbles.releases.agent.md states "Exactly 8 docs per phase, no more and
# no fewer". The "no more" half was already mechanical: release-packet-location-
# guard.sh rejects a non-canonical doc inside a packet directory. The "no fewer"
# half had no enforcement anywhere, so a phase could reach "all required features
# delivered, all gates green" with no recorded monetization posture, no scaling
# thresholds, and no operational-readiness statement. Gate G101 could not see it
# either: an annotation-driven gate reads features.md, and a document that is
# absent contributes no annotations to fail on.
#
# This guard is deliberately SEPARATE from the location guard. Folding a
# completeness assertion into a script named "location" would send a reader
# debugging a completeness failure into a file that claims to be about placement.
#
# Detection rule (set-shaped, per IMP-050 SCOPE-2 requirement 1):
#   - Enumerate each docs/releases/<phase>/ directory.
#   - Treat that directory as a packet if it holds at least one canonical doc.
#     A directory holding none is not a partially-authored packet; it is some
#     other directory (assets, images) and is left alone.
#   - Report EACH absent canonical doc by name. A count alone would tell an
#     operator that something is missing without telling them which commercial
#     or operational surface is unrecorded, which is the specific observed harm.
#
# A partially-authored packet is NOT a legitimate resting state (IMP-050
# SCOPE-1). The releases agent's `docs:` argument restricts which docs a run
# refreshes; it is not an authoring path that licenses a permanent partial
# packet. No "under construction" escape is offered here on purpose: an
# undeclared, unbounded exemption would repeat the unvalidated-boolean precedent
# recorded in IMP-049 EV-14.
#
# Exit 0 = every packet complete (or no packets present). Exit 1 = at least one
# packet is missing at least one canonical doc. Exit 2 = usage/environment error.
# No --skip / --force / --ignore flag exists by design.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

# shellcheck source=./release-packet-docs-lib.sh
. "$SCRIPT_DIR/release-packet-docs-lib.sh"

REPO_ROOT="${1:-.}"

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "[release-packet-completeness-guard][ERROR] repo root not found: $REPO_ROOT" >&2
  exit 2
fi

REPO_ROOT_ABS="$(cd "$REPO_ROOT" && pwd -P)"
RELEASES_DIR="$REPO_ROOT_ABS/docs/releases"

# A repository with no release packets at all is clean, not incomplete.
if [[ ! -d "$RELEASES_DIR" ]]; then
  echo "[release-packet-completeness-guard] OK — no docs/releases/ directory; nothing to check"
  exit 0
fi

incomplete_phases=0
phases_checked=0
report=""

while IFS= read -r phase_dir; do
  [[ -z "$phase_dir" ]] && continue

  present=0
  missing=""
  for doc in "${RELEASE_PACKET_DOCS[@]}"; do
    if [[ -f "$phase_dir/$doc" ]]; then
      present=$((present + 1))
    else
      missing="$missing $doc"
    fi
  done

  # Not a packet directory — holds none of the canonical eight.
  [[ "$present" -eq 0 ]] && continue

  phases_checked=$((phases_checked + 1))
  [[ -z "$missing" ]] && continue

  incomplete_phases=$((incomplete_phases + 1))
  phase_name="$(basename "$phase_dir")"
  report="${report}
  docs/releases/${phase_name}/ — ${present}/8 present, missing:"
  for doc in $missing; do
    report="${report}
      - ${doc}"
  done
done < <(find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d | LC_ALL=C sort)

if [[ "$incomplete_phases" -eq 0 ]]; then
  echo "[release-packet-completeness-guard] OK — ${phases_checked} release packet(s) hold all 8 canonical docs"
  exit 0
fi

echo "[release-packet-completeness-guard][FAIL] ${incomplete_phases} of ${phases_checked} release packet(s) are incomplete:" >&2
printf '%s\n' "$report" >&2
cat >&2 <<EOF

The release-packet contract (agents/bubbles.releases.agent.md) is "exactly 8 docs
per phase, no more and no fewer". A phase missing a doc can reach "all required
features delivered, all gates green" with that surface unrecorded.

Remediation: author the named doc(s) via bubbles.releases, or remove the phase
directory if the phase was abandoned. There is no skip flag; a partially
authored packet is not a legitimate resting state.
EOF
exit 1
