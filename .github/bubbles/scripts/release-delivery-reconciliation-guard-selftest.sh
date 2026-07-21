#!/usr/bin/env bash
set -uo pipefail

# release-delivery-reconciliation-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/release-delivery-reconciliation-guard.sh`
# (Gate G101 — IMP-006). Stages throwaway repo roots under a temp workspace and
# asserts the guard's exit code for clean + adversarial release-packet shapes.
#
# Scenarios:
#   S0  non-existent repo root                                  → exit 2
#   S1  reconciled mvp packet; required feature done +           → exit 0
#       validate-certified
#   S2  reconciled packet; required feature spec dir MISSING     → exit 1  (ADVERSARIAL:
#       (a downstream "promised-but-unspecced" shape)                       SCOPE-6 replay)
#   S3  reconciled packet; required feature status in_progress   → exit 1  (ADVERSARIAL)
#   S4  reconciled packet; required feature done but 'validate'  → exit 1  (ADVERSARIAL:
#       absent from completed phases (implement self-cert)                  self-certification)
#   S5  reconciled-packet header but ZERO feature annotations    → exit 1  (ADVERSARIAL:
#       (silent-no-op trap)                                                 fail-loud-on-malformed)
#   S6  grandfathered packet (no header), missing spec,          → exit 0  (backward-compat
#       no --require-coverage                                               WARN-only)
#   S7  same grandfathered packet + --require-coverage           → exit 1  (ADVERSARIAL:
#       (scenario/convergence path forces blocking)                        scenario path)
#   S8  reconciled packet; optional/carried/deferred features    → exit 0  (only 'required'
#       with spec=none                                                      is enforced)
#   S9  reconciled packet; required feature BLOCKED w/ reason     → exit 1  (ADVERSARIAL:
#                                                                           honest blocked != delivered)
#   S10 source-repo-shaped root (no docs/releases)               → exit 0  (EXEMPT)
#   S11 reconciled packet; annotation missing 'delivery' field   → exit 1  (ADVERSARIAL: malformed)
#   S12 reconciled packet; required feature delivered_prototype   → exit 1  (assurance invariant:
#       (validate-certified)                                                 prototype never deployable)
#
# Reference: improvements/IMP-006-release-delivery-reconciliation.md

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/release-delivery-reconciliation-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "selftest: guard not executable: $GUARD" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "selftest: jq is required" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-reldeliv-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED=()

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED+=("$1")
}

new_repo() {
  local name="$1"
  local d="$WORKSPACE/$name"
  mkdir -p "$d"
  printf '%s' "$d"
}

# mk_features <repo> <phase> <reconciled true|false> [annotation ...]
mk_features() {
  local repo="$1" phase="$2" reconciled="$3"
  shift 3
  local dir="$repo/docs/releases/$phase"
  mkdir -p "$dir"
  {
    echo "# $phase — features"
    echo ""
    if [[ "$reconciled" == "true" ]]; then
      echo "<!-- bubbles:reconciled-packet schemaVersion=1 phase=$phase -->"
    fi
    local ann
    for ann in "$@"; do
      echo "<!-- $ann -->"
    done
  } >"$dir/features.md"
}

# mk_spec <repo> <specpath> <status> [phase ...]   (completedPhases)
mk_spec() {
  local repo="$1" specpath="$2" status="$3"
  shift 3
  local dir="$repo/$specpath"
  mkdir -p "$dir"
  local phases_json="[]"
  if [[ $# -gt 0 ]]; then
    local acc=""
    local p
    for p in "$@"; do acc="$acc\"$p\","; done
    phases_json="[${acc%,}]"
  fi
  cat >"$dir/state.json" <<EOF
{ "version": 3, "specId": "$(basename "$specpath")", "status": "$status", "workflowMode": "full-delivery", "completedPhases": $phases_json }
EOF
}

# mk_spec_blocked <repo> <specpath> <reason>
mk_spec_blocked() {
  local repo="$1" specpath="$2" reason="$3"
  local dir="$repo/$specpath"
  mkdir -p "$dir"
  cat >"$dir/state.json" <<EOF
{ "version": 3, "specId": "$(basename "$specpath")", "status": "blocked", "workflowMode": "full-delivery", "blockedReason": "$reason", "completedPhases": ["plan","design","implement"] }
EOF
}

run_guard() {
  bash "$GUARD" "$@" >/dev/null 2>&1
  RC=$?
}

expect_rc() {
  local want="$1" desc="$2"
  if [[ "$RC" -eq "$want" ]]; then
    pass "$desc (rc=$RC)"
  else
    bad "$desc (want $want, got $RC)"
  fi
}

# ----------------------------------------------------------------------------
# S0 — non-existent repo root → 2
run_guard --repo-root "$WORKSPACE/does-not-exist"
expect_rc 2 "S0 non-existent repo root"

# S1 — reconciled, required feature done + validate-certified → 0
R1="$(new_repo s1)"
mk_features "$R1" mvp true \
  "bubbles:feature id=auth-real spec=specs/074-auth delivery=required"
mk_spec "$R1" specs/074-auth "done" plan design implement test validate audit docs
run_guard --repo-root "$R1" --phase mvp
expect_rc 0 "S1 required feature done+validate-certified"

# S2 — reconciled, required feature spec dir MISSING → 1 (downstream replay, SCOPE-6)
R2="$(new_repo s2)"
mk_features "$R2" mvp true \
  "bubbles:feature id=strategy-agent-runtime spec=specs/075-strategy-agent-runtime delivery=required"
# NOTE: no specs/075-* created — the exact downstream "promised but never specced" shape.
run_guard --repo-root "$R2" --phase mvp
expect_rc 1 "S2 required feature spec MISSING (downstream promised-but-unspecced replay)"

# S3 — reconciled, required feature in_progress → 1
R3="$(new_repo s3)"
mk_features "$R3" mvp true \
  "bubbles:feature id=billing spec=specs/076-billing delivery=required"
mk_spec "$R3" specs/076-billing in_progress plan design implement
run_guard --repo-root "$R3" --phase mvp
expect_rc 1 "S3 required feature in_progress"

# S4 — reconciled, required feature done but validate ABSENT → 1 (self-cert)
R4="$(new_repo s4)"
mk_features "$R4" mvp true \
  "bubbles:feature id=entitlements spec=specs/077-entitlements delivery=required"
mk_spec "$R4" specs/077-entitlements "done" plan design implement test
run_guard --repo-root "$R4" --phase mvp
expect_rc 1 "S4 required feature done but implement-self-certified (validate absent)"

# S5 — reconciled-packet header but ZERO annotations → 1 (silent-no-op trap)
R5="$(new_repo s5)"
mk_features "$R5" mvp true
run_guard --repo-root "$R5" --phase mvp
expect_rc 1 "S5 reconciled packet binds nothing (silent-no-op trap)"

# S6 — grandfathered packet (no header), missing spec, no --require-coverage → 0
R6="$(new_repo s6)"
mk_features "$R6" mvp false \
  "bubbles:feature id=foo spec=specs/078-foo delivery=required"
run_guard --repo-root "$R6" --phase mvp
expect_rc 0 "S6 grandfathered packet (WARN-only, missing spec tolerated)"

# S7 — same grandfathered packet + --require-coverage → 1
run_guard --repo-root "$R6" --phase mvp --require-coverage
expect_rc 1 "S7 grandfathered packet + --require-coverage forces blocking"

# S8 — reconciled, optional/carried/deferred with spec=none → 0
R8="$(new_repo s8)"
mk_features "$R8" mvp true \
  "bubbles:feature id=sso spec=none delivery=deferred-to:v2.0" \
  "bubbles:feature id=market-routes spec=none delivery=carried" \
  "bubbles:feature id=nice-to-have spec=none delivery=optional"
run_guard --repo-root "$R8" --phase mvp
expect_rc 0 "S8 non-required features with spec=none"

# S9 — reconciled, required feature BLOCKED with reason → 1
R9="$(new_repo s9)"
mk_features "$R9" mvp true \
  "bubbles:feature id=bridge spec=specs/063-bridge delivery=required"
mk_spec_blocked "$R9" specs/063-bridge "framework lint heuristic mismatch; operator-actionable"
run_guard --repo-root "$R9" --phase mvp
expect_rc 1 "S9 required feature blocked-with-reason (not delivered)"

# S10 — source-repo-shaped root (no docs/releases) → 0 EXEMPT
R10="$(new_repo s10)"
mkdir -p "$R10/bubbles/scripts" "$R10/agents"
run_guard --repo-root "$R10"
expect_rc 0 "S10 no docs/releases → EXEMPT"

# S11 — reconciled, annotation missing 'delivery' field → 1 (malformed)
R11="$(new_repo s11)"
mk_features "$R11" mvp true \
  "bubbles:feature id=broken spec=specs/079-broken"
mk_spec "$R11" specs/079-broken "done" plan design implement test validate
run_guard --repo-root "$R11" --phase mvp
expect_rc 1 "S11 malformed annotation (missing delivery field)"

# S12 — reconciled, required feature at delivered_prototype, FULLY validate-certified → 1
#       (assurance invariant: prototype tier is NEVER deployable, so it can never
#       satisfy a delivery=required feature — even validate-certified. This LOCKS the
#       explicit refusal so a future prototype-tier mode that declares delivered_prototype
#       terminal cannot silently reconcile a prototype as "delivered" — the deploy hole.)
R12="$(new_repo s12)"
mk_features "$R12" mvp true \
  "bubbles:feature id=proto-only spec=specs/080-proto delivery=required"
mk_spec "$R12" specs/080-proto "delivered_prototype" plan design implement test validate
run_guard --repo-root "$R12" --phase mvp
expect_rc 1 "S12 required feature delivered_prototype is refused (prototype never deployable)"

# ----------------------------------------------------------------------------
echo ""
echo "release-delivery-reconciliation-guard selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  printf '  - %s\n' "${FAILED[@]}" >&2
  exit 1
fi
exit 0
