#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG_FILE="$REPO_ROOT/.specify/memory/bubbles.config.json"

require_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"

  if grep -Fq "$needle" <<< "$haystack"; then
    printf 'PASS: %s\n' "$message"
  else
    printf 'FAIL: %s\n' "$message" >&2
    exit 1
  fi
}

restore_profile() {
  local original_profile="$1"
  bash "$SCRIPT_DIR/developer-profile.sh" set "$original_profile" >/dev/null
}

original_profile='delivery'
if [[ -f "$CONFIG_FILE" ]]; then
  configured_profile="$({
    grep -oE '"adoptionProfile"[[:space:]]*:[[:space:]]*"[^"]+"' "$CONFIG_FILE" 2>/dev/null \
      | sed -E 's/.*"([^"]+)"$/\1/'
  } || true)"
  [[ -n "$configured_profile" ]] && original_profile="$configured_profile"
fi

trap 'restore_profile "$original_profile"' EXIT

echo "Running profile-transition selftest..."
echo "Scenario: profile transitions keep guidance explicit while certification invariants stay unchanged."

bash "$SCRIPT_DIR/developer-profile.sh" set foundation >/dev/null
foundation_output="$(bash "$SCRIPT_DIR/developer-profile.sh" show)"
policy_output="$(bash "$SCRIPT_DIR/cli.sh" policy status)"

require_contains "$foundation_output" "Active profile: Foundation (foundation)" "Foundation profile becomes active"
require_contains "$foundation_output" "Governance invariant: full-certification" "Foundation keeps the full-certification invariant"
require_contains "$foundation_output" "docs/guides/CONTROL_PLANE_ADOPTION.md" "Foundation exposes required docs"
require_contains "$foundation_output" "bash bubbles/scripts/cli.sh doctor" "Foundation exposes onboarding-focused next commands"
require_contains "$policy_output" "Adoption profile: Foundation (foundation)" "Policy status reflects the foundation profile"
require_contains "$policy_output" "governanceInvariant = full-certification" "Policy status keeps certification language explicit"

bash "$SCRIPT_DIR/developer-profile.sh" set assured >/dev/null
assured_output="$(bash "$SCRIPT_DIR/developer-profile.sh" show)"
assured_readiness_output="$(bash "$SCRIPT_DIR/repo-readiness.sh" "$REPO_ROOT" --profile assured)"

require_contains "$assured_output" "Active profile: Assured (assured)" "Assured profile becomes active"
require_contains "$assured_output" "Repo-readiness posture: guardrail-forward" "Assured output shows stronger guidance posture"
require_contains "$assured_output" "Scenario contracts remain unchanged when profile guidance changes." "Assured output preserves scenario-contract invariant messaging"
require_contains "$assured_readiness_output" "Profile posture: assured front-loads guardrail visibility, but certification rigor still remains full-strength." "Repo-readiness explains the assured posture"
require_contains "$assured_readiness_output" "Boundary: advisory framework ops only; this does not replace bubbles.validate certification." "Repo-readiness keeps the certification boundary explicit"

bash "$SCRIPT_DIR/developer-profile.sh" set delivery >/dev/null
delivery_output="$(bash "$SCRIPT_DIR/developer-profile.sh" show)"

require_contains "$delivery_output" "Active profile: Delivery (delivery)" "Delivery profile restores as the default repo-local posture"
require_contains "$delivery_output" "docs/guides/WORKFLOW_MODES.md" "Delivery output includes the delivery docs path"

echo "profile-transition selftest passed."