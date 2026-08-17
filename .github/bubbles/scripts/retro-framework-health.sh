#!/usr/bin/env bash
# retro-framework-health.sh — framework self-observation analysis.
#
# Reads bubbles' own usage signals and emits an improvement proposal under
# improvements/IMP-NNN-<slug>.md. NEVER mutates bubbles/, agents/, or
# bubbles/workflows.yaml.
#
# WHY THE READERS CHANGED (IMP-047 S-A)
# -------------------------------------
# This report used to rank "top failing gates" out of
# `.specify/runtime/framework-events.jsonl`, a store that carries no gate
# outcomes at all. Per-gate outcomes live in `.specify/runtime/gate-hits.jsonl`,
# written by `gate-hit-log.sh`. The section was not merely empty: it printed a
# confident "no gate failure data" while a populated store sat unread.
#
# Capability freshness had the same shape. It filtered on `lastValidated`, a
# field the current `bubbles/capability-ledger.yaml` schema does not define, so
# the comparison could never select anything and the report always concluded "no
# stale capabilities detected". A filter over an absent field is a false negative
# wearing the word "detected". Nothing in the ledger records WHEN a capability
# was last validated, so this now reports the ledger's real `state` distribution
# and declares validation age UNMEASURED rather than inventing a field to read.
#
# Every section names the STORE it read and the PRODUCER that writes it. A
# reported field whose producer cannot be named is exactly how an unbacked number
# survives. The full producer/reader map is generated separately by
# `generate-telemetry-reader-map.sh`.
#
# Inputs (all read-only):
#   .specify/runtime/gate-hits.jsonl         — per-gate outcomes (gate-hit-log.sh)
#   .specify/runtime/framework-events.jsonl  — agent/workflow events (volume only)
#   .specify/runtime/workflow-runs.json      — per-run records (run-state registry)
#   bubbles/capability-ledger.yaml           — capability state ledger
#   specs/**/scenario-manifest.json          — scenario state counts
#
# Output:
#   improvements/IMP-<NNN>-<slug>.md         — markdown proposal
#
# Exit codes:
#   0 — proposal written (or "no signal" message if inputs absent/empty)
#   1 — hard tooling failure (missing jq/yq/find)
#
# Usage:
#   bash bubbles/scripts/retro-framework-health.sh [repo-root] [--slug <slug>] [--out-dir <dir>]

set -euo pipefail

REPO_ROOT="${1:-.}"
shift || true

SLUG="framework-health-$(date -u +%Y%m%d)"
OUT_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --slug) SLUG="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    -h|--help)
      cat <<'EOF'
retro-framework-health.sh — framework self-observation
Usage: bash retro-framework-health.sh [repo-root] [--slug <slug>] [--out-dir <dir>]
Reads .specify/runtime/gate-hits.jsonl + framework-events.jsonl + workflow-runs.json
     + bubbles/capability-ledger.yaml + specs/**/scenario-manifest.json
Writes improvements/IMP-NNN-<slug>.md (proposal-first; no auto-mutation of framework files).
EOF
      exit 0
      ;;
    *) shift ;;
  esac
done

[[ -z "$OUT_DIR" ]] && OUT_DIR="$REPO_ROOT/improvements"
mkdir -p "$OUT_DIR"

GATE_HITS_FILE="$REPO_ROOT/.specify/runtime/gate-hits.jsonl"
EVENTS_FILE="$REPO_ROOT/.specify/runtime/framework-events.jsonl"
RUNS_FILE="$REPO_ROOT/.specify/runtime/workflow-runs.json"
LEDGER_FILE="$REPO_ROOT/bubbles/capability-ledger.yaml"

# Determine next IMP number
existing="$(find "$OUT_DIR" -maxdepth 1 -name 'IMP-*-*.md' -type f 2>/dev/null | sed -E 's|.*IMP-([0-9]+)-.*|\1|' | sort -n | tail -1 || true)"
if [[ -z "$existing" ]]; then
  next_num=1
else
  next_num=$((existing + 1))
fi
OUT_FILE="$(printf "%s/IMP-%03d-%s.md" "$OUT_DIR" "$next_num" "$SLUG")"

# --- Gate outcomes, from the store that actually carries them ---------------
#
# Firing and prevention are separate facts (IMP-047 S-A). A gate that fired and
# permitted is not the same as one that never fired, and only PREVENTION is a
# valid basis for retiring a gate, so the three populations are reported
# separately instead of collapsed into one "failures" count.
gate_prevented="- (no gate prevented a transition in .specify/runtime/gate-hits.jsonl)"
gate_fired_only="- (no gate recorded as fired without preventing)"
gate_never_fired="- (no gate recorded as credited without evaluation)"
if [[ -f "$GATE_HITS_FILE" ]]; then
  gate_summary="$(awk '
    {
      gate=""; outcome=""; firedf=""; preventedf=""; estatus="";
      if (match($0, /"gate":"[^"]*"/)) { gate = substr($0, RSTART+8, RLENGTH-9) }
      if (gate == "") next
      if (match($0, /"outcome":"[^"]*"/)) { outcome = substr($0, RSTART+11, RLENGTH-12) }
      if (match($0, /"exitStatus":"[^"]*"/)) { estatus = substr($0, RSTART+14, RLENGTH-15) }
      if (match($0, /"fired":(true|false)/)) { firedf = substr($0, RSTART+8, RLENGTH-8) }
      if (match($0, /"prevented":(true|false)/)) { preventedf = substr($0, RSTART+12, RLENGTH-12) }
      # Records predating the split derive both values from their own fields.
      if (firedf == "") {
        firedf = "true"
        preventedf = (outcome == "fail" && estatus != "" && estatus != "0") ? "true" : "false"
      }
      if (firedf == "true") fired[gate]++; else notfired[gate]++
      if (preventedf == "true") prevented[gate]++
      seen[gate] = 1
    }
    END { for (g in seen) printf "%s\t%d\t%d\t%d\n", g, fired[g]+0, notfired[g]+0, prevented[g]+0 }
  ' "$GATE_HITS_FILE" 2>/dev/null || true)"
  if [[ -n "$gate_summary" ]]; then
    rows="$(printf '%s\n' "$gate_summary" | awk -F'\t' '$4 > 0 { printf "%d\t%s\n", $4, $1 }' | sort -rn | head -5 | awk -F'\t' '{printf "- %s (%d prevented transition(s))\n", $2, $1}' || true)"
    [[ -n "$rows" ]] && gate_prevented="$rows"
    rows="$(printf '%s\n' "$gate_summary" | awk -F'\t' '$2 > 0 && $4 == 0 { printf "%d\t%s\n", $2, $1 }' | sort -rn | head -5 | awk -F'\t' '{printf "- %s (fired %d time(s), prevented nothing)\n", $2, $1}' || true)"
    [[ -n "$rows" ]] && gate_fired_only="$rows"
    rows="$(printf '%s\n' "$gate_summary" | awk -F'\t' '$2 == 0 { printf "%d\t%s\n", $3, $1 }' | sort -rn | head -5 | awk -F'\t' '{printf "- %s (credited %d time(s) without being evaluated)\n", $2, $1}' || true)"
    [[ -n "$rows" ]] && gate_never_fired="$rows"
  fi
fi

# Unsuccessful runs from workflow-runs.json. The run-state file is an OBJECT
# holding activeRuns/recentRuns arrays whose records carry status/result and a
# command -- NOT a top-level array of records carrying mode/outcome. The old
# filter raised "Cannot index number with string" on every run, and 2>/dev/null
# swallowed it, so this section reported "no data" while 22 active and 12 failed
# runs sat in the file. A health report that cannot see failures is worse than
# no health report.
stalled_modes=""
if [[ -f "$RUNS_FILE" ]] && command -v jq >/dev/null 2>&1; then
  stalled_modes="$(jq -r '(.activeRuns[]?, .recentRuns[]?) | select((.result // "pending") != "success") | .command // "unknown"' "$RUNS_FILE" 2>/dev/null \
    | sort | uniq -c | sort -rn | head -3 | awk '{printf "- %s (%d run(s) not completed successfully)\n", $2, $1}' || true)"
fi
[[ -z "$stalled_modes" ]] && stalled_modes="- (no unsuccessful or pending run data found in workflow-runs.json)"

# --- Capability ledger, using fields the schema actually defines -------------
capability_states="- (capability ledger missing or unreadable)"
capability_partial="- (no capability outside the shipped state)"
if [[ -f "$LEDGER_FILE" ]] && command -v yq >/dev/null 2>&1; then
  states="$(yq -r '.capabilities // {} | to_entries | .[] | .value.state // "unstated"' "$LEDGER_FILE" 2>/dev/null \
    | sort | uniq -c | sort -rn | awk '{printf "- %s: %d capability(ies)\n", $2, $1}' || true)"
  [[ -n "$states" ]] && capability_states="$states"
  partial="$(yq -r '.capabilities // {} | to_entries | .[] | select((.value.state // "unstated") != "shipped") | "- " + .key + " (state: " + (.value.state // "unstated") + ", introduced: " + (.value.releaseIntroduced // "unstated") + ")"' "$LEDGER_FILE" 2>/dev/null | head -10 || true)"
  [[ -n "$partial" ]] && capability_partial="$partial"
fi

# --- Scenario state counts ---------------------------------------------------
#
# Progress is a count of scenario STATES. Gate outcomes are never substituted:
# a gate can refuse a transition but cannot advance a scenario, so counting
# green gates would measure the guard rather than the delivery.
scenario_counts="- (no scenario-manifest.json found under specs/)"
scenario_total=0
if [[ -d "$REPO_ROOT/specs" ]] && command -v jq >/dev/null 2>&1; then
  manifest_list="$(find "$REPO_ROOT/specs" -maxdepth 3 -type f -name scenario-manifest.json 2>/dev/null | LC_ALL=C sort || true)"
  if [[ -n "$manifest_list" ]]; then
    scenario_total="$(printf '%s\n' "$manifest_list" | grep -c '[^[:space:]]' || true)"
    counted="$(printf '%s\n' "$manifest_list" | while IFS= read -r manifest; do
      [[ -n "$manifest" ]] || continue
      jq -r '(.scenarios // [])[] | (.state // "unstated")' "$manifest" 2>/dev/null || true
    done | sort | uniq -c | sort -rn | awk '{printf "- %s: %d scenario(s)\n", $2, $1}' || true)"
    if [[ -n "$counted" ]]; then
      scenario_counts="$counted"
    else
      scenario_counts="- UNMEASURED: $scenario_total manifest(s) record no scenario states. Gate outcomes are NOT substituted for them."
    fi
  fi
fi

# --- Signal volume, per store ------------------------------------------------
event_count=0
[[ -f "$EVENTS_FILE" ]] && event_count="$(wc -l < "$EVENTS_FILE" 2>/dev/null | tr -d ' ' || echo 0)"
gate_hit_count=0
[[ -f "$GATE_HITS_FILE" ]] && gate_hit_count="$(wc -l < "$GATE_HITS_FILE" 2>/dev/null | tr -d ' ' || echo 0)"

# Write the proposal
cat > "$OUT_FILE" <<EOF
# IMP-$(printf "%03d" "$next_num") — $SLUG

**Type:** Framework improvement proposal (auto-generated by \`bubbles.retro target: framework\`)
**Status:** PROPOSED
**Date:** $(date -u +%FT%TZ)
**Auto-mutation:** NONE. This is a proposal only. Human reviews and decides whether to spec the change.

## Signal Volume

Every figure below names the store it came from and the surface that writes it.
A number whose producer cannot be named is unbacked, and is reported as such.

| Signal | Store | Producer | Volume |
|---|---|---|---|
| Gate outcomes | \`.specify/runtime/gate-hits.jsonl\` | \`bubbles/scripts/gate-hit-log.sh\` | $gate_hit_count record(s) |
| Framework events | \`.specify/runtime/framework-events.jsonl\` | \`bubbles/scripts/goal-fidelity-telemetry.sh\` | $event_count record(s) |
| Workflow runs | \`.specify/runtime/workflow-runs.json\` | run-state registry | see below |
| Capability states | \`bubbles/capability-ledger.yaml\` | hand-maintained ledger | see below |
| Scenario states | \`specs/**/scenario-manifest.json\` | planning artifacts | $scenario_total manifest(s) |

The complete producer/reader map is generated at
\`docs/generated/telemetry-reader-map.md\`.

## Diagnosis

### Gates that prevented a transition

Store: \`.specify/runtime/gate-hits.jsonl\`. Producer: \`bubbles/scripts/gate-hit-log.sh\`.
Prevention is the only valid basis for retiring a gate.

$gate_prevented

### Gates that fired but prevented nothing

These are retirement CANDIDATES, not decisions.

$gate_fired_only

### Gates credited without being evaluated

These never fired. A gate that has not been exercised is not evidence of
uselessness and must not be read as one.

$gate_never_fired

### Stalled / non-completed workflow modes

Store: \`.specify/runtime/workflow-runs.json\`.

$stalled_modes

### Capability ledger state distribution

Store: \`bubbles/capability-ledger.yaml\`. Fields read: \`state\`, \`releaseIntroduced\`.

$capability_states

**Capability validation age: UNMEASURED.** The ledger schema records no
validation timestamp, so no honest staleness threshold can be applied. Do not
report a freshness figure until a field that records one exists.

Capabilities outside the \`shipped\` state:

$capability_partial

### Scenario state counts

Store: \`specs/**/scenario-manifest.json\`.

Progress is a count of scenario states. Gate outcomes are NOT progress: a gate
can refuse a transition, but it cannot advance a scenario, so counting green
gates would measure the guard rather than the delivery.

$scenario_counts

## Recommended next step

If any of the above signals warrant a framework change:

1. Open a new spec under \`specs/NNN-<slug>/\` describing the proposed change.
2. Use \`/bubbles.workflow full-delivery\` to plan + implement + validate the change.
3. Reference this IMP file in the spec's \`design.md\` Diagnosis section.

If the signals are healthy and no change is warranted, mark this proposal as \`status: noop\` and archive.

## Provenance

- Generated by: \`bubbles/scripts/retro-framework-health.sh\`
- Input sources: \`.specify/runtime/gate-hits.jsonl\`, \`.specify/runtime/framework-events.jsonl\`, \`.specify/runtime/workflow-runs.json\`, \`bubbles/capability-ledger.yaml\`, \`specs/**/scenario-manifest.json\`
- Mutation: ZERO framework files modified by this script.

EOF

echo "[retro-framework-health] Wrote $OUT_FILE"
exit 0
