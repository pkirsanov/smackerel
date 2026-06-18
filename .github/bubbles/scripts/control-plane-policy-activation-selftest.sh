#!/usr/bin/env bash
set -uo pipefail

# control-plane-policy-activation-selftest.sh
#
# Proof for the control-plane policy-activation fix (gates G055-G060 were
# declared-but-INERT when policySnapshot was absent; G060 was a keyword
# rubber-stamp). Exercises the guard-lib.sh helpers that Check 3A/3D/3E now use:
#   resolve_effective_policy / resolve_effective_policy_source  (Layer 1 — SST precedence)
#   detect_red_green_ordering                                   (Layer 2 — real red->green)
#   policy_spec_grandfathered / policy_snapshot_present         (grandfather clause)
#
# ADVERSARIAL by design: case B is the exact report that PASSED under the old
# keyword grep and MUST now FAIL the hardened ordering check. A tautological
# selftest (all cases satisfy a broken resolver/check) is forbidden.
#
# Hermetic: all fixtures live under a mktemp dir, cleaned on exit; no network,
# no mutation of the real repo.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

CUTOFF="2026-06-18"

pass=0
fail=0

ok() {
  pass=$((pass + 1))
}

bad() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

assert_eq() { # label  actual  expected
  if [[ "$2" == "$3" ]]; then
    ok
  else
    bad "$1 (want '$3', got '$2')"
  fi
}

# Mirror of the hardened Check 3E decision (sans the unchanged exempt path, which
# these fixtures do not exercise): resolve mode -> ordering -> grandfather -> fail.
check3e_verdict() { # state_file  report_file  cutoff  repo_root
  local state="$1" report="$2" cutoff="$3" repo="$4"
  local mode
  mode="$(resolve_effective_policy "$state" tdd mode scenario-first "$repo")"
  if [[ "$mode" != "scenario-first" ]]; then
    echo "skip"
    return
  fi
  if detect_red_green_ordering "$report"; then
    echo "pass"
    return
  fi
  if policy_spec_grandfathered "$state" "$cutoff"; then
    echo "grandfathered"
    return
  fi
  echo "fail"
}

# The exact keyword grep the fix REMOVED — used in case B to prove the old check
# would have matched 'tdd' alone (rubber-stamp) where the new ordering check does not.
old_keyword_grep_matches() { # file
  grep -qiE 'red[[:space:]-]*green|failing targeted|red evidence|green evidence|scenario-first|tdd' "$1"
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-cp-policy-activation.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM

# ── Fixture repo carrying the SST config (defaults.tdd.mode=scenario-first) ──
cfg_repo="$tmp/cfg-repo"
mkdir -p "$cfg_repo/.specify/memory"
cat > "$cfg_repo/.specify/memory/bubbles.config.json" <<'EOF'
{
  "version": 2,
  "defaults": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "scenario-first", "source": "repo-default" },
    "lockdown": { "default": false, "source": "repo-default" },
    "regression": { "immutability": "protected-scenarios", "source": "repo-default" }
  }
}
EOF

# ── Fixture repo with NO SST config (framework-default leg) ──
empty_repo="$tmp/empty-repo"
mkdir -p "$empty_repo"

# ── state.json fixtures ──
# A/E2/E3: no policySnapshot, createdAt AFTER the cutoff → full enforcement.
state_nosnap_post="$tmp/state-nosnap-post.json"
cat > "$state_nosnap_post" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "feature-development",
  "createdAt": "2026-07-01T00:00:00Z"
}
EOF

# D: no policySnapshot, createdAt BEFORE the cutoff → grandfathered.
state_nosnap_pre="$tmp/state-nosnap-pre.json"
cat > "$state_nosnap_pre" <<'EOF'
{
  "version": 3,
  "status": "done",
  "workflowMode": "feature-development",
  "createdAt": "2026-05-01T00:00:00Z"
}
EOF

# E1: policySnapshot present with tdd.mode=off → snapshot wins over config.
state_snap_off="$tmp/state-snap-off.json"
cat > "$state_snap_off" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "feature-development",
  "createdAt": "2026-07-01T00:00:00Z",
  "policySnapshot": {
    "tdd": { "mode": "off", "source": "repo-default" },
    "lockdown": { "default": true, "source": "spec-lockdown" }
  }
}
EOF

# ── report.md fixtures ──
# A/D: no red->green ordering markers at all.
report_no_evidence="$tmp/report-no-evidence.md"
cat > "$report_no_evidence" <<'EOF'
# Report

## Summary

Implemented the export feature and recorded the evidence.
All scopes complete; documentation synchronized.
EOF

# B: ADVERSARIAL — the ONLY tdd-related content is the literal word 'tdd'.
report_keyword_only="$tmp/report-keyword-only.md"
cat > "$report_keyword_only" <<'EOF'
# Report

## Approach

We applied tdd to this change and wired the export handler into the router.
The OKF bundle is serialized from a real datastore query.
EOF

# C: real RED-stage marker on an earlier line, GREEN-stage marker on a later line.
report_ordered="$tmp/report-ordered.md"
cat > "$report_ordered" <<'EOF'
# Report

## TDD Evidence

RED: ran `the targeted test` before the fix
test result: FAILED. 0 passed; 1 failed

Applied the isolation fix to the export handler.

GREEN: ran `the targeted test` after the fix
test result: ok. 1 passed; 0 failed
EOF

echo "=== control-plane policy-activation selftest ==="

# ── Case A: ACTIVATION-FROM-CONFIG ──────────────────────────────────────────
# No policySnapshot, but the SST config declares tdd.mode=scenario-first, and a
# post-cutoff createdAt → the setting activates from the SST and Check-3E FAILS.
a_mode="$(resolve_effective_policy "$state_nosnap_post" tdd mode scenario-first "$cfg_repo")"
a_source="$(resolve_effective_policy_source "$state_nosnap_post" tdd mode scenario-first "$cfg_repo")"
assert_eq "A: tdd mode activates from SST config" "$a_mode" "scenario-first"
assert_eq "A: provenance is repo-default (SST is the source of record)" "$a_source" "repo-default"
a_verdict="$(check3e_verdict "$state_nosnap_post" "$report_no_evidence" "$CUTOFF" "$cfg_repo")"
assert_eq "A: Check-3E activates and FAILS (no red->green, post-cutoff, no snapshot)" "$a_verdict" "fail"

# ── Case B: ADVERSARIAL KEYWORD-ONLY ────────────────────────────────────────
# The report whose only tdd content is the word 'tdd' MUST fail hardened G060,
# while it WOULD have matched the old keyword grep (proving the rubber-stamp).
if old_keyword_grep_matches "$report_keyword_only"; then
  ok
else
  bad "B: old keyword grep should have matched 'tdd' (rubber-stamp baseline)"
fi
if detect_red_green_ordering "$report_keyword_only"; then
  bad "B: hardened G060 ordering check MUST NOT pass on keyword-only 'tdd'"
else
  ok
fi
b_verdict="$(check3e_verdict "$state_nosnap_post" "$report_keyword_only" "$CUTOFF" "$cfg_repo")"
assert_eq "B: Check-3E verdict on keyword-only report is FAIL" "$b_verdict" "fail"

# ── Case C: RED->GREEN ORDERING PASSES ──────────────────────────────────────
if detect_red_green_ordering "$report_ordered"; then
  ok
else
  bad "C: red-stage-then-green-stage ordering MUST pass hardened G060"
fi
c_verdict="$(check3e_verdict "$state_nosnap_post" "$report_ordered" "$CUTOFF" "$cfg_repo")"
assert_eq "C: Check-3E verdict on ordered report is PASS" "$c_verdict" "pass"

# ── Case D: GRANDFATHER ─────────────────────────────────────────────────────
# Pre-cutoff createdAt, no policySnapshot, no red->green → downgraded, not a fail.
if policy_spec_grandfathered "$state_nosnap_pre" "$CUTOFF"; then
  ok
else
  bad "D: pre-cutoff snapshot-less spec MUST be grandfathered"
fi
if policy_spec_grandfathered "$state_nosnap_post" "$CUTOFF"; then
  bad "D: post-cutoff snapshot-less spec MUST NOT be grandfathered"
else
  ok
fi
if policy_spec_grandfathered "$state_snap_off" "$CUTOFF"; then
  bad "D: snapshot-bearing spec MUST NOT be grandfathered (full enforcement)"
else
  ok
fi
d_verdict="$(check3e_verdict "$state_nosnap_pre" "$report_no_evidence" "$CUTOFF" "$cfg_repo")"
assert_eq "D: Check-3E verdict on grandfathered spec is downgraded (not fail)" "$d_verdict" "grandfathered"

# ── Case E: PRECEDENCE (snapshot > config > framework default) ───────────────
# E1: snapshot tdd.mode=off overrides config scenario-first.
e1_mode="$(resolve_effective_policy "$state_snap_off" tdd mode scenario-first "$cfg_repo")"
e1_source="$(resolve_effective_policy_source "$state_snap_off" tdd mode scenario-first "$cfg_repo")"
assert_eq "E1: snapshot value wins over config" "$e1_mode" "off"
assert_eq "E1: provenance is snapshot" "$e1_source" "snapshot"

# E2: no snapshot → config value wins over framework default.
e2_mode="$(resolve_effective_policy "$state_nosnap_post" tdd mode off-default "$cfg_repo")"
e2_source="$(resolve_effective_policy_source "$state_nosnap_post" tdd mode off-default "$cfg_repo")"
assert_eq "E2: config value wins over framework default" "$e2_mode" "scenario-first"
assert_eq "E2: provenance is repo-default" "$e2_source" "repo-default"

# E3: no snapshot, no config → framework default.
e3_mode="$(resolve_effective_policy "$state_nosnap_post" tdd mode fallback-mode "$empty_repo")"
e3_source="$(resolve_effective_policy_source "$state_nosnap_post" tdd mode fallback-mode "$empty_repo")"
assert_eq "E3: framework default used when no snapshot and no config" "$e3_mode" "fallback-mode"
assert_eq "E3: provenance is framework-default" "$e3_source" "framework-default"

# E (bonus): boolean config value normalizes to a lowercase string token.
e_bool="$(resolve_effective_policy "$state_nosnap_post" lockdown default true "$cfg_repo")"
assert_eq "E: boolean config default normalizes to 'false'" "$e_bool" "false"

echo ""
echo "control-plane-policy-activation-selftest: $pass passed / $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
