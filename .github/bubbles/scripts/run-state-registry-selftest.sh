#!/usr/bin/env bash
# Hermetic selftest for the workflow-run registry read-modify-write cycle.
#
# THE DEFECT THIS DEFENDS AGAINST
#
# run_state_lines printed any line inside a section that contained a "{". That
# silently assumed the registry was ALWAYS written one-compact-record-per-line,
# an assumption nothing enforced. The moment a registry was pretty-printed — by
# jq, an editor, a formatter, or any other writer — every record's first line
# was a bare "{", the reader returned "{" per record, and write_run_state_registry
# faithfully published "    {," for each one. The next read-modify-write made it
# permanent.
#
# This was not theoretical. It is the observed state of two downstream consumer
# registries found in the field:
#
#   one repository       25 records reduced to a bare  {,
#   another repository   a half record spliced onto a whole one
#
# Both are unparseable, so every consumer of those ledgers — including the
# abandoned-run reaper — is blind on those repositories.
#
# The reader now parses structurally (string state + brace depth), so a record
# is recovered regardless of how the source happened to be formatted.
#
# NOTE ON SCOPE: an earlier draft of this file attributed the corruption to
# concurrent writers sharing a fixed ".tmp" staging path. That staging path was
# genuinely unsafe and is fixed (C1), but it is NOT what produced the corruption
# in the field — a shared staging path could not be made to reproduce it, while
# the formatting round-trip reproduces it on the first try, every time (A1).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI="${BUBBLES_CLI_UNDER_TEST:-$SCRIPT_DIR/cli.sh}"

checks=0
failures=0
ok() { checks=$((checks + 1)); echo "  ok   $1"; }
bad() { checks=$((checks + 1)); failures=$((failures + 1)); echo "  FAIL $1"; echo "       $2"; }

command -v jq >/dev/null 2>&1 || { echo "run-state-registry-selftest: SKIP (jq not installed)"; exit 0; }
[[ -f "$CLI" ]] || { echo "FAIL: $CLI not found" >&2; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
REG="$TMP/.specify/runtime/workflow-runs.json"
mkdir -p "$(dirname "$REG")"

# cli.sh ends in `main "$@"`; neutralising it makes the file sourceable so the
# reader and writer can be driven directly instead of through a full command.
SOURCEABLE="$TMP/cli-sourceable.sh"
sed 's|^main "\$@"$|: # entrypoint disabled for direct function tests|' "$CLI" > "$SOURCEABLE"

read_section() { # read_section <src> <section>
  ( set +u
    # shellcheck disable=SC1090
    source "$1" >/dev/null 2>&1
    # shellcheck disable=SC2034  # consumed by the sourced cli.sh functions
    CONTROL_PLANE_RUN_STATE_FILE="$REG"
    run_state_lines "$2" ) 2>/dev/null
}

round_trip() { # round_trip <src> -> echoes "valid <n>" or "corrupt <line>"
  ( set +u
    # shellcheck disable=SC1090
    source "$1" >/dev/null 2>&1
    # shellcheck disable=SC2034  # consumed by the sourced cli.sh functions
    CONTROL_PLANE_RUN_STATE_FILE="$REG"
    write_run_state_registry "$(run_state_lines activeRuns)" "$(run_state_lines recentRuns)"
  ) >/dev/null 2>&1
  if jq empty "$REG" >/dev/null 2>&1; then
    printf 'valid %s' "$(jq '.activeRuns | length' "$REG")"
  else
    printf 'corrupt %s' "$(sed -n '4p' "$REG" | tr -d ' ')"
  fi
}

# --- R1. THE REGRESSION: a pretty-printed registry must survive -------------
jq -n '{version:1,activeRuns:[{runId:"a",status:"active"},{runId:"b",status:"active"}],recentRuns:[]}' > "$REG"
verdict="$(round_trip "$SOURCEABLE")"
if [[ "$verdict" == "valid 2" ]]; then
  ok "R1 a pretty-printed registry survives a read-modify-write with both records"
else
  bad "R1 pretty-printed registry round-trip" "got '$verdict' (want 'valid 2')"
fi

# --- R2. the ordinary compact shape is unchanged ---------------------------
printf '{\n  "version": 1,\n  "activeRuns": [\n    {"runId":"c","status":"active"},\n    {"runId":"d","status":"active"}\n  ],\n  "recentRuns": [\n  ]\n}\n' > "$REG"
verdict="$(round_trip "$SOURCEABLE")"
if [[ "$verdict" == "valid 2" ]]; then
  ok "R2 the normal one-record-per-line shape still round-trips"
else
  bad "R2 compact registry round-trip" "got '$verdict' (want 'valid 2')"
fi

# --- R3. a whole document on one line (jq -c) ------------------------------
# The old reader could not read this at all: with the section header and the
# records on the same line, its `next` skipped every record.
jq -c -n '{version:1,activeRuns:[{runId:"e",status:"active"}],recentRuns:[{runId:"f",status:"completed"}]}' > "$REG"
active="$(read_section "$SOURCEABLE" activeRuns)"
recent="$(read_section "$SOURCEABLE" recentRuns)"
if [[ "$(printf '%s' "$active" | grep -c .)" == "1" && "$(printf '%s' "$recent" | grep -c .)" == "1" ]]; then
  ok "R3 a single-line document is read correctly for both sections"
else
  bad "R3 single-line document" "active='$active' recent='$recent'"
fi

# --- R4. structure inside string values must not be misread ----------------
# Brace counting is only safe if it respects string state; a worktree path
# containing braces would otherwise end a record early.
jq -n '{version:1,activeRuns:[{runId:"g",worktree:"/tmp/a {weird} path/x"}],recentRuns:[]}' > "$REG"
active="$(read_section "$SOURCEABLE" activeRuns)"
if [[ "$(printf '%s' "$active" | jq -r '.worktree' 2>/dev/null)" == "/tmp/a {weird} path/x" ]]; then
  ok "R4 braces and spaces inside a string value are preserved"
else
  bad "R4 braces inside strings" "got '$active'"
fi

# --- A1. ADVERSARIAL: the OLD reader must fail R1 --------------------------
# Without this, R1 could pass for reasons unrelated to the fix. This rebuilds
# the previous line-based reader and shows it produces the exact field
# signature — "{," — on the very first round-trip.
OLD="$TMP/cli-old-reader.sh"
awk '
  /^run_state_lines\(\) \{$/ { skip = 1 }
  skip && /^write_run_state_registry\(\) \{$/ {
    skip = 0
    print "run_state_lines() {"
    print "  local section=\"$1\""
    print "  ensure_run_state_registry"
    print "  awk -v target=\"$section\" \x27"
    print "    $0 ~ \"\\\"\" target \"\\\"[[:space:]]*:[[:space:]]*\\\\[\" { in_target = 1; next }"
    print "    in_target && /^[[:space:]]*\\]/ { exit }"
    print "    in_target && /\\{/ {"
    print "      gsub(/^[[:space:]]+/, \"\", $0)"
    print "      sub(/,[[:space:]]*$/, \"\", $0)"
    print "      print $0"
    print "    }"
    print "  \x27 \"$CONTROL_PLANE_RUN_STATE_FILE\""
    print "}"
    print ""
  }
  !skip { print }
' "$SOURCEABLE" > "$OLD"

if ! grep -q 'in_target && /\\{/' "$OLD"; then
  bad "A1 negative control" "could not reconstruct the previous line-based reader"
else
  jq -n '{version:1,activeRuns:[{runId:"a",status:"active"},{runId:"b",status:"active"}],recentRuns:[]}' > "$REG"
  old_verdict="$(round_trip "$OLD")"
  if [[ "$old_verdict" == corrupt* ]]; then
    ok "A1 the previous line-based reader DOES corrupt the same input ($old_verdict) — R1 is load-bearing"
  else
    bad "A1 negative control did not reproduce" \
      "old reader returned '$old_verdict'; R1 proves nothing if the old reader also survives"
  fi
fi

# --- C1. staging path must remain per-writer -------------------------------
# Separate concern from the formatting defect above, and a real one: a shared
# "$FILE.tmp" is not a staging file but a shared mutable buffer. This is a
# structural check because it must hold deterministically on every machine.
if grep -q 'tmp_file="\$(mktemp' "$CLI"; then
  ok "C1 the staging path is allocated by mktemp (cannot collide between writers)"
else
  bad "C1 staging path is per-writer" \
    "expected 'tmp_file=\"\$(mktemp ...' in $CLI — a fixed \".tmp\" is shared between concurrent writers"
fi

echo
if [[ "$failures" -eq 0 ]]; then
  echo "run-state-registry-selftest: PASS ($checks checks)"
  exit 0
fi
echo "run-state-registry-selftest: FAIL ($failures/$checks checks failed)" >&2
exit 1
