#!/usr/bin/env bash
set -euo pipefail

# compaction-discipline-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/compaction-discipline-guard.sh`
# (Gate G083 — context_compaction_discipline_gate).
#
# Builds a private mktemp Bubbles-repo surface (no edits to the host
# repo), stages fixture scenarios in its `.specify/memory/`
# `bubbles.session.json`, invokes the guard with explicit
# `BUBBLES_REPO_ROOT`, and asserts exit codes plus stdout/stderr
# fingerprints.
#
# Scenarios (matches SCOPE-2 scope.md Gherkin):
#   S0: empty envelopesReceived[]            → exit 0 (sanity check)
#   S1: 50 envelopes, oldest 48 compactedAt=null
#       (eligible-uncompacted-count = 48 > 3)
#                                            → exit 1, stderr "G083"
#                                              + "envelope count threshold"
#   S2: 3 envelopes for spec, oldest 1 raw size ~10 KB compactedAt=null
#       (eligible slice = 1 entry, latest 2 kept raw;
#        uncompacted bytes ~10240 > 8192)    → exit 1, stderr "G083"
#                                              + "size threshold"
#   S3: 4 envelopes, oldest 2 compactedAt set, latest 2 raw kept
#                                            → exit 0, stdout "PASS"
#   S4: malformed session.json               → exit 2
#   S5: envelopes for a DIFFERENT spec       → exit 0 (spec-isolation)
#   S6: 3 envelopes total — eligible slice is empty (latest 2 are raw,
#       so only 1 is eligible AND that 1 is compacted)
#                                            → exit 0
#
# Reference:
#   docs/Framework_Convergence_Health.md

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
GUARD_SCRIPT="$SCRIPT_DIR/compaction-discipline-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "compaction-discipline-guard-selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

# --- Hermetic workspace --------------------------------------------------

WORKSPACE="$(mktemp -d -t bubbles-compaction-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
declare -a FAILED_SCENARIOS=()

note() { printf '[selftest] %s\n' "$*"; }
ok()   { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko()   {
  printf '[selftest] FAIL: %s\n' "$*" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# --- Stage a minimal fake "Bubbles" repo surface inside WORKSPACE --------
#
# We need:
#   <root>/.specify/memory/bubbles.session.json
#
# We do NOT need a workflows.yaml because G083's thresholds are
# framework constants baked into the guard (not declared in
# workflows.yaml). The selftest stages files INSIDE its own mktemp
# workspace via heredocs / printf — this is allowed by
# terminal-discipline policy (heredoc-to-file is forbidden for repo
# files; the workspace here is throwaway and never becomes part of the
# working tree).

stage_repo_root() {
  local root="$1"
  mkdir -p "$root/.specify/memory"
}

write_session_json() {
  local root="$1"
  local payload="$2"
  printf '%s\n' "$payload" > "$root/.specify/memory/bubbles.session.json"
}

# --- Helper: run guard, capture exit + stdout + stderr -------------------

run_guard() {
  local root="$1"
  local spec_dir="$2"
  local stdout_file="$WORKSPACE/stdout.last"
  local stderr_file="$WORKSPACE/stderr.last"

  set +e
  BUBBLES_REPO_ROOT="$root" bash "$GUARD_SCRIPT" "$spec_dir" \
    > "$stdout_file" \
    2> "$stderr_file"
  local rc=$?
  set -e

  printf '%s\n' "$rc" > "$WORKSPACE/exit.last"
}

last_exit()   { cat "$WORKSPACE/exit.last"; }
last_stdout() { cat "$WORKSPACE/stdout.last"; }
last_stderr() { cat "$WORKSPACE/stderr.last"; }

assert_exit() {
  local expected="$1"
  local label="$2"
  local actual
  actual="$(last_exit)"
  if [[ "$actual" != "$expected" ]]; then
    ko "$label: expected exit $expected, got $actual"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 1
  fi
  ok "$label: exit $expected"
}

assert_stdout_contains() {
  local needle="$1"
  local label="$2"
  if ! grep -Fq -- "$needle" "$WORKSPACE/stdout.last"; then
    ko "$label: stdout did not contain '$needle'"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    return 1
  fi
  ok "$label: stdout contains '$needle'"
}

assert_stderr_contains() {
  local needle="$1"
  local label="$2"
  if ! grep -Fq -- "$needle" "$WORKSPACE/stderr.last"; then
    ko "$label: stderr did not contain '$needle'"
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 1
  fi
  ok "$label: stderr contains '$needle'"
}

# =============================================================================
# Scenario S0: empty envelopesReceived[] -> exit 0 (sanity check)
# =============================================================================

note "Scenario S0: empty envelopesReceived[] should pass with exit 0"

S0_ROOT="$WORKSPACE/s0"
stage_repo_root "$S0_ROOT"
write_session_json "$S0_ROOT" '{"envelopesReceived": []}'

run_guard "$S0_ROOT" "specs/900-convergence-fixture"

assert_exit 0 "S0 exit code"
assert_stdout_contains "PASS Gate G083" "S0 PASS marker on stdout"
assert_stdout_contains "total=0" "S0 reports zero envelopes"

# =============================================================================
# Scenario S1: 50 envelopes, oldest 48 compactedAt=null
#              eligible slice = 48 (latest 2 kept raw)
#              uncompacted eligible count = 48 > 3 → fail
# =============================================================================

note "Scenario S1: 50 envelopes, count breach should exit 1"

S1_ROOT="$WORKSPACE/s1"
stage_repo_root "$S1_ROOT"

# Build a JSON array of 50 entries with synthetic timestamps; all 50
# have compactedAt:null and rawSizeBytes:50 (well under size threshold).
S1_JSON="$(
  jq -nc '
    {
      envelopesReceived: [
        range(50)
        | {
            specDir: "specs/900-convergence-fixture",
            agent: "bubbles.workflow",
            receivedAt: ("2026-06-01T10:" + (. + 100 | tostring)[1:] + ":00Z"),
            rawSizeBytes: 50,
            incomingMessage: "x",
            compactedAt: null,
            rawPointer: null
          }
      ]
    }
  '
)"
write_session_json "$S1_ROOT" "$S1_JSON"

run_guard "$S1_ROOT" "specs/900-convergence-fixture"

assert_exit 1 "S1 exit code (count breach)"
assert_stderr_contains "G083" "S1 stderr names Gate G083"
assert_stderr_contains "context_compaction_discipline_gate" "S1 stderr names gate full name"
assert_stderr_contains "envelope count threshold" "S1 stderr names count threshold"
assert_stderr_contains "context-compactor.sh" "S1 stderr names remediation tool"

# =============================================================================
# Scenario S2: 3 envelopes, oldest 1 raw size ~10 KB compactedAt=null
#              eligible slice = 1 (latest 2 kept raw)
#              uncompacted bytes = 10240 > 8192 → fail with size threshold
# =============================================================================

note "Scenario S2: oldest envelope is ~10 KB raw, size breach should exit 1"

S2_ROOT="$WORKSPACE/s2"
stage_repo_root "$S2_ROOT"

# Build the oldest entry's incomingMessage as a 10 KiB string by
# generating a long literal via jq. We use rawSizeBytes:10240
# explicitly so the guard does not have to fall back to string-length.
S2_JSON="$(
  jq -nc '
    {
      envelopesReceived: [
        {
          specDir: "specs/900-convergence-fixture",
          agent: "bubbles.workflow",
          receivedAt: "2026-06-01T10:00:00Z",
          rawSizeBytes: 10240,
          incomingMessage: "fat-envelope",
          compactedAt: null,
          rawPointer: null
        },
        {
          specDir: "specs/900-convergence-fixture",
          agent: "bubbles.workflow",
          receivedAt: "2026-06-01T10:01:00Z",
          rawSizeBytes: 100,
          incomingMessage: "small",
          compactedAt: null,
          rawPointer: null
        },
        {
          specDir: "specs/900-convergence-fixture",
          agent: "bubbles.workflow",
          receivedAt: "2026-06-01T10:02:00Z",
          rawSizeBytes: 100,
          incomingMessage: "small",
          compactedAt: null,
          rawPointer: null
        }
      ]
    }
  '
)"
write_session_json "$S2_ROOT" "$S2_JSON"

run_guard "$S2_ROOT" "specs/900-convergence-fixture"

assert_exit 1 "S2 exit code (size breach)"
assert_stderr_contains "G083" "S2 stderr names Gate G083"
assert_stderr_contains "size threshold" "S2 stderr names size threshold"
assert_stderr_contains "uncompacted eligible bytes" "S2 stderr names bytes metric"

# =============================================================================
# Scenario S3: 4 envelopes, oldest 2 compactedAt set, latest 2 raw kept
#              eligible slice = 2 (both compacted)
#              uncompacted eligible count = 0 → pass
# =============================================================================

note "Scenario S3: 4 envelopes with oldest 2 compacted should exit 0"

S3_ROOT="$WORKSPACE/s3"
stage_repo_root "$S3_ROOT"
write_session_json "$S3_ROOT" '{
  "envelopesReceived": [
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:00:00Z",
      "rawSizeBytes": 200,
      "incomingMessage": "old-1",
      "compactedAt": "2026-06-01T10:00:30Z",
      "rawPointer": null
    },
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:01:00Z",
      "rawSizeBytes": 200,
      "incomingMessage": "old-2",
      "compactedAt": "2026-06-01T10:01:30Z",
      "rawPointer": null
    },
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:02:00Z",
      "rawSizeBytes": 200,
      "incomingMessage": "latest-1-raw",
      "compactedAt": null,
      "rawPointer": null
    },
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:03:00Z",
      "rawSizeBytes": 200,
      "incomingMessage": "latest-2-raw",
      "compactedAt": null,
      "rawPointer": null
    }
  ]
}'

run_guard "$S3_ROOT" "specs/900-convergence-fixture"

assert_exit 0 "S3 exit code (compliant compaction)"
assert_stdout_contains "PASS Gate G083" "S3 PASS marker on stdout"
assert_stdout_contains "eligible=2" "S3 reports eligible=2"
assert_stdout_contains "uncompacted=0" "S3 reports uncompacted=0"

# =============================================================================
# Scenario S4: malformed session.json -> exit 2
# =============================================================================

note "Scenario S4: malformed session.json should exit 2"

S4_ROOT="$WORKSPACE/s4"
stage_repo_root "$S4_ROOT"
write_session_json "$S4_ROOT" '{"envelopesReceived": ['

run_guard "$S4_ROOT" "specs/900-convergence-fixture"

assert_exit 2 "S4 exit code (malformed JSON)"
assert_stderr_contains "compaction-discipline-guard" "S4 stderr has diagnostic prefix"
assert_stderr_contains "not valid JSON" "S4 stderr names malformed-JSON condition"

# =============================================================================
# Scenario S5: envelopes for a DIFFERENT spec → exit 0 (spec-isolation)
# =============================================================================

note "Scenario S5: envelopesReceived[] for a different spec should NOT trip the guard"

S5_ROOT="$WORKSPACE/s5"
stage_repo_root "$S5_ROOT"

# 20 entries all for a DIFFERENT spec — none should be counted against
# the queried specDir.
S5_JSON="$(
  jq -nc '
    {
      envelopesReceived: [
        range(20)
        | {
            specDir: "specs/999-other-spec",
            agent: "bubbles.workflow",
            receivedAt: ("2026-06-01T10:" + (. + 100 | tostring)[1:] + ":00Z"),
            rawSizeBytes: 50,
            incomingMessage: "x",
            compactedAt: null,
            rawPointer: null
          }
      ]
    }
  '
)"
write_session_json "$S5_ROOT" "$S5_JSON"

run_guard "$S5_ROOT" "specs/900-convergence-fixture"

assert_exit 0 "S5 exit code (other-spec entries isolated)"
assert_stdout_contains "total=0" "S5 ignores entries for non-matching specDir"

# =============================================================================
# Scenario S6: 3 envelopes, eligible slice = 1, that 1 is compacted
#              uncompacted eligible count = 0 → pass
# =============================================================================

note "Scenario S6: 3 envelopes with oldest compacted (latest 2 raw kept) should exit 0"

S6_ROOT="$WORKSPACE/s6"
stage_repo_root "$S6_ROOT"
write_session_json "$S6_ROOT" '{
  "envelopesReceived": [
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:00:00Z",
      "rawSizeBytes": 300,
      "incomingMessage": "oldest-compacted",
      "compactedAt": "2026-06-01T10:00:15Z",
      "rawPointer": null
    },
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:01:00Z",
      "rawSizeBytes": 300,
      "incomingMessage": "latest-1-raw",
      "compactedAt": null,
      "rawPointer": null
    },
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "receivedAt": "2026-06-01T10:02:00Z",
      "rawSizeBytes": 300,
      "incomingMessage": "latest-2-raw",
      "compactedAt": null,
      "rawPointer": null
    }
  ]
}'

run_guard "$S6_ROOT" "specs/900-convergence-fixture"

assert_exit 0 "S6 exit code (3 envelopes, eligible slice fully compacted)"
assert_stdout_contains "PASS Gate G083" "S6 PASS marker on stdout"
assert_stdout_contains "eligible=1" "S6 reports eligible=1"
assert_stdout_contains "uncompacted=0" "S6 reports uncompacted=0"

# =============================================================================
# Final verdict
# =============================================================================

echo ""
echo "============================================================"
echo "  COMPACTION-DISCIPLINE-GUARD SELFTEST VERDICT"
echo "============================================================"
printf 'Passed assertions: %d\n' "$PASS_COUNT"
printf 'Failed assertions: %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo ""
  echo "FAILED scenarios:"
  for s in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $s"
  done
  exit 1
fi

echo ""
echo "🟢 compaction-discipline-guard-selftest: ALL SCENARIOS PASS"
exit 0
