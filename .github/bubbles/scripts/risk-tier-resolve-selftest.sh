#!/usr/bin/env bash
# Hermetic selftest for risk-tier-resolve.sh (BFW-01 / IMP-021).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/risk-tier-resolve.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# expect_tier <label> <expected-tier> <resolver-args...>
expect_tier() {
  local label="$1" want="$2"; shift 2
  local out
  out="$("$RESOLVE" "$@" 2>&1)"
  if printf '%s\n' "$out" | grep -qx "tier=$want"; then
    pass "$label"
  else
    fail "$label (got: $(printf '%s' "$out" | grep '^tier=' || true))"
  fi
}

echo "Running risk-tier-resolve selftest..."

# Low-risk: build-free static single-file tool, no triggers → rapid
expect_tier "low-risk build-free static tool -> rapid" "rapid-tool-delivery" \
  --surface "Add a build-free single-file static HTML tool, no backend"

# Each high-risk trigger class escalates to full-delivery:
expect_tier "auth trigger -> full" "full-delivery" --surface "Add a static tool with OAuth login"
expect_tier "payment trigger -> full" "full-delivery" --surface "A build-free tool that processes a Stripe payment"
expect_tier "secret trigger -> full" "full-delivery" --surface "Static page that stores an api-key credential"
expect_tier "PII trigger -> full" "full-delivery" --surface "Isolated tool collecting personal data (GDPR)"
expect_tier "DB migration trigger -> full" "full-delivery" --surface "Static tool plus a database migration alter table"
expect_tier "deploy/infra trigger -> full" "full-delivery" --surface "Buildless tool with a kubernetes deploy"
expect_tier "cross-product proto trigger -> full" "full-delivery" --surface "Static tool that changes a shared protobuf contract"

# Mixed: a positive low-risk signal is present BUT so is a high-risk trigger →
# full-delivery (a user cannot self-label risky work low-risk to shed gates).
expect_tier "low-risk signal + auth trigger -> full (no self-label bypass)" "full-delivery" \
  --surface "self-contained html tool with jwt auth"

# Path-based high-risk trigger:
expect_tier "high-risk changed path (migrations/) -> full" "full-delivery" \
  --surface "static tool" --changed-paths $'index.html\nmigrations/001_users.sql'

# Low-risk changed paths + low-risk signal → rapid
expect_tier "low-risk changed paths + signal -> rapid" "rapid-tool-delivery" \
  --surface "build-free static html tool" --changed-paths $'tool.html\nnotes/tool.md'

# Ambiguous: no positive low-risk signal, no trigger → fail-closed to full
expect_tier "ambiguous (no low-risk signal) -> full (fail-closed)" "full-delivery" \
  --surface "Refactor the internal module"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "risk-tier-resolve-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "risk-tier-resolve-selftest: all cases passed."
