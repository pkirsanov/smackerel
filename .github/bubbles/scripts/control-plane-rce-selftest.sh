#!/usr/bin/env bash
set -uo pipefail

# control-plane-rce-selftest.sh
#
# Adversarial proof for IMP-102 SCOPE-4 (RCE elimination): the control-plane
# guard MUST NOT interpolate an un-sanitized spec-directory path into `python3
# -c` SOURCE. Before the fix, guards/control-plane-checks.sh (and its parent
# state-transition-guard.sh) emitted Python that interpolated the spec-directory
# path straight into the source, i.e. open('<state_file>') where <state_file>
# was the un-sanitized directory argument, so a spec directory whose NAME
# contained a single quote + a Python payload made the emitted program execute
# arbitrary code. After the fix the path is passed positionally
# (`python3 -c '...open(sys.argv[1])...' "$state_file"`) or via os.environ, so
# the directory name is inert data.
#
# ADVERSARIAL by design (NON-tautological):
#   Case A creates a spec directory whose NAME carries an
#     os.system("touch $BUBBLES_RCE_SENTINEL")
#   breakout. Under the OLD interpolating guard the sentinel file IS created
#   (arbitrary code executed); under the fixed guard it is NOT — the malicious
#   name is opened as a literal path (which resolves to the real state.json).
#   Case B uses a benign single-quote directory name: under the OLD guard the
#   (now 2>/dev/null-free) control-plane blocks emit a Python SyntaxError; under
#   the fixed guard the quote is inert.
# Each case ALSO asserts the guard actually REACHED the control-plane checks, so
# a guard that bailed out early can never produce a silent false pass.
#
# Hermetic: everything lives under one mktemp root, torn down on exit (including
# any sentinel). No network, no mutation of the real repo. Skips cleanly when
# python3 is unavailable (the vulnerability is python -c specific).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"

pass=0
fail=0

ok() {
  pass=$((pass + 1))
}

bad() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

if ! command -v python3 >/dev/null 2>&1; then
  echo "control-plane-rce-selftest: SKIP (python3 not installed)"
  exit 0
fi
if [[ ! -f "$GUARD_SCRIPT" ]]; then
  echo "control-plane-rce-selftest: SKIP (state-transition-guard.sh not found at $GUARD_SCRIPT)"
  exit 0
fi

tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-cp-rce.XXXXXX")"
# Absolute sentinel path (exported) so the OLD-code os.system breakout would
# create it regardless of the guard's working directory.
export BUBBLES_RCE_SENTINEL="$tmp_root/BUBBLES_RCE_SENTINEL_$$"
# Keep the heavy delegated tail gates out of the fixture run (they have their own
# selftests); we only need the guard to reach the control-plane checks.
export BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=1

cleanup() {
  rm -rf "$tmp_root" 2>/dev/null || true
  # Defensive: remove the sentinel even if a regression made it land elsewhere.
  rm -f "$BUBBLES_RCE_SENTINEL" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Clone the framework surface so the guard resolves its repo root, workflows,
# and capability registries exactly as it would in a real repo.
clone_root="$tmp_root/repo"
mkdir -p "$clone_root/specs"
cp -R "$SCRIPT_DIR/.." "$clone_root/bubbles"
cp -R "$SCRIPT_DIR/../../agents" "$clone_root/agents"
# The transition-contract resolver needs a git work tree + BUBBLES_REPO_ROOT to
# compute the contract digest / target revision (mirrors the transition-guard
# selftest setup); without them the guard exits at contract resolution BEFORE
# the control-plane checks we are exercising.
git -C "$clone_root" init -q >/dev/null 2>&1 || true
git -C "$clone_root" config user.email "rce@selftest.local" >/dev/null 2>&1 || true
git -C "$clone_root" config user.name "rce-selftest" >/dev/null 2>&1 || true
export BUBBLES_REPO_ROOT="$clone_root"

write_fixture() {
  # $1 = absolute spec dir (already created). Minimal-but-coherent docs-only
  # artifact set so the guard evaluates through to the control-plane checks.
  local dir="$1"
  mkdir -p "$dir/tests"
  cat <<'EOF' > "$dir/spec.md"
# RCE Fixture Spec

## Purpose

Minimal docs-only fixture used to drive the transition guard's control-plane
checks against an adversarial spec-directory name.
EOF
  cat <<'EOF' > "$dir/design.md"
# RCE Fixture Design

## Approach

Docs-only workflow so the guard evaluates control-plane state without runtime proof.

## Change Boundary

Only this temporary fixture is evaluated.

## Consumer Impact Sweep

No route, identifier, command, or external consumer changes.

## Shared Infrastructure Impact Sweep

No shared infrastructure or persistent state changes.
EOF
  cat <<'EOF' > "$dir/uservalidation.md"
# User Validation

## Checklist

- [x] Baseline docs-only validation path is available for the fixture.
EOF
  cat <<'EOF' > "$dir/scopes.md"
# Scope 01: RCE Fixture

**Status:** Done

### Definition of Done

- [x] Documentation route metadata recorded consistently -> Evidence: report.md#summary
EOF
  cat <<'EOF' > "$dir/report.md"
# Report

### Summary

Docs-only control-plane RCE selftest fixture.

### Completion Statement

Fixture shaped to reach the control-plane checks.

### Test Evidence

```text
$ echo fixture
fixture
```
EOF
  cat <<'EOF' > "$dir/state.json"
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "autonomous-goal",
  "execution": {
    "completedPhaseClaims": ["test", "validate", "audit", "docs"]
  },
  "certification": {
    "status": "in_progress",
    "certifiedCompletedPhases": ["test", "validate", "audit", "docs"],
    "completedScopes": ["01-rce-fixture"],
    "scopeProgress": [],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    }
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "autonomous-goal"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "test",
      "agent": "bubbles.test",
      "phasesExecuted": ["test"],
      "runStartedAt": "2026-03-27T10:00:00Z",
      "runCompletedAt": "2026-03-27T10:00:47Z",
      "completedAt": "2026-03-27T10:00:47Z"
    },
    {
      "phase": "validate",
      "agent": "bubbles.validate",
      "phasesExecuted": ["validate"],
      "runStartedAt": "2026-03-27T10:01:13Z",
      "runCompletedAt": "2026-03-27T10:02:31Z",
      "completedAt": "2026-03-27T10:02:31Z"
    },
    {
      "phase": "audit",
      "agent": "bubbles.audit",
      "phasesExecuted": ["audit"],
      "runStartedAt": "2026-03-27T10:03:02Z",
      "runCompletedAt": "2026-03-27T10:06:08Z",
      "completedAt": "2026-03-27T10:06:08Z"
    },
    {
      "phase": "docs",
      "agent": "bubbles.docs",
      "phasesExecuted": ["docs"],
      "runStartedAt": "2026-03-27T10:07:19Z",
      "runCompletedAt": "2026-03-27T10:11:44Z",
      "completedAt": "2026-03-27T10:11:44Z"
    }
  ],
  "lastUpdatedAt": "2026-03-27T10:11:44Z"
}
EOF
}

reached_control_plane() {
  # $1 = guard log. control-plane-checks.sh prints these headers when sourced,
  # which is where the (previously vulnerable) python -c sites live.
  grep -qE 'Check 3[A-H]:|Gate G0(55|56|57|58|59|60|61)' "$1"
}

run_guard() {
  # $1 = spec dir ; $2 = combined-output log file
  rm -f "$BUBBLES_RCE_SENTINEL" 2>/dev/null || true
  bash "$GUARD_SCRIPT" "$1" >"$2" 2>&1 || true
}

# ── Case A: RCE breakout embedded in the spec-directory NAME ────────────────
sq="'"
inj_name="inj${sq}+__import__(\"os\").system(\"touch \$BUBBLES_RCE_SENTINEL\")+${sq}z"
inj_dir="$clone_root/specs/$inj_name"
mkdir -p "$inj_dir"
write_fixture "$inj_dir"
inj_log="$tmp_root/inj.log"
run_guard "$inj_dir" "$inj_log"

if reached_control_plane "$inj_log"; then
  ok
else
  bad "Case A: guard did NOT reach the control-plane checks — cannot prove the vulnerable site was exercised (would be a false pass)"
  echo "--- inj.log (first 80 lines) ---"
  sed -n '1,80p' "$inj_log"
  echo "--- end inj.log ---"
fi

if [[ -e "$BUBBLES_RCE_SENTINEL" ]]; then
  bad "Case A: RCE sentinel WAS created — the adversarial directory name executed code (guard still interpolates the path into python3 -c source)"
else
  ok
fi

if grep -q 'SyntaxError' "$inj_log"; then
  bad "Case A: guard emitted a Python SyntaxError for the adversarial directory name"
else
  ok
fi

# ── Case B: benign single-quote directory name (correctness + no SyntaxError) ─
benign_name="benign${sq}quote-fixture"
benign_dir="$clone_root/specs/$benign_name"
mkdir -p "$benign_dir"
write_fixture "$benign_dir"
benign_log="$tmp_root/benign.log"
run_guard "$benign_dir" "$benign_log"

if reached_control_plane "$benign_log"; then
  ok
else
  bad "Case B: guard did NOT reach the control-plane checks for the benign-quote fixture"
  echo "--- benign.log (first 80 lines) ---"
  sed -n '1,80p' "$benign_log"
  echo "--- end benign.log ---"
fi

if grep -q 'SyntaxError' "$benign_log"; then
  bad "Case B: a benign single-quote directory name produced a Python SyntaxError (path was interpolated into python3 -c source)"
else
  ok
fi

if [[ -e "$BUBBLES_RCE_SENTINEL" ]]; then
  bad "Case B: unexpected sentinel creation"
else
  ok
fi

echo ""
echo "control-plane-rce-selftest: $pass passed / $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
