#!/usr/bin/env bash
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NONE="$SCRIPT_DIR/../adapters/usage/none.sh"
VSCODE="$SCRIPT_DIR/../adapters/usage/vscode-copilot.sh"
REFERENCE="$SCRIPT_DIR/../adapters/usage/reference-test.sh"
RESOLVER="$SCRIPT_DIR/usage-resolve.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/usage-v2-selftest.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM

fail() { echo "usage-adapter-v2-selftest: FAIL: $1" >&2; exit 1; }
pass_count=0
pass() { pass_count=$((pass_count + 1)); echo "ok $pass_count - $1"; }

[[ "$($NONE requests session:any)" == '[]' ]] || fail "none v1 requests changed"
[[ "$($NONE session session:any)" == '{}' ]] || fail "none v1 session changed"
[[ "$($NONE capabilities)" == '{}' ]] || fail "none v1 capabilities changed"
pass "none v1 bytes remain compatible"

description="$($NONE v2 describe)"
[[ "$(printf '%s' "$description" | jq -r '.schemaVersion')" == 2 ]] || fail "none v2 description missing"
[[ "$(printf '%s' "$description" | jq '[.capabilities[] | select(.mode != "unsupported")] | length')" == 0 ]] || fail "none claimed measurement"
pass "none v2 declares every dimension unsupported"

mkdir -p "$tmp/default" "$tmp/typo"
default_result="$($RESOLVER --repo-root "$tmp/default" --contract-major 2)"
printf '%s' "$default_result" | grep -q '^adapter=none$' || fail "default absence changed"
cat >"$tmp/typo/bubbles-project.yaml" <<'EOF'
usage:
  adapter: missing-adapter
EOF
if "$RESOLVER" --repo-root "$tmp/typo" --contract-major 2 >/dev/null 2>&1; then
  fail "configured typo degraded silently"
fi
pass "default absence stays none and configured typo fails loud"

cat >"$tmp/host.jsonl" <<'EOF'
{"hostSessionId":"exact-one","promptTokens":7}
EOF
cat >"$tmp/identity.json" <<EOF
{"artifactPath":"$tmp/host.jsonl","artifactSessionId":"artifact:one","hostInstanceId":"host:one","hostSchemaId":"vscode-copilot-chat-session-v1","hostSessionId":"exact-one","proofDigest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","repositoryDecisionId":"rb:one","sessionIdentityId":"session:one","startedAt":"2026-08-01T00:00:00.000Z","workspaceIdentity":"workspace:one"}
EOF
identity="$($VSCODE v2 identify-session "$tmp/identity.json")"
[[ "$(printf '%s' "$identity" | jq -r '.hostSessionId')" == exact-one ]] || fail "exact identity failed"
pass "one explicit supported host record identifies exactly"

cat >"$tmp/zero.json" <<EOF
$(jq '.hostSessionId="absent"' "$tmp/identity.json")
EOF
if "$VSCODE" v2 identify-session "$tmp/zero.json" >/dev/null 2>&1; then fail "zero-match identity accepted"; fi
cat >>"$tmp/host.jsonl" <<'EOF'
{"hostSessionId":"exact-one","promptTokens":9}
EOF
if "$VSCODE" v2 identify-session "$tmp/identity.json" >/dev/null 2>&1; then fail "multiple-match identity accepted"; fi
cat >"$tmp/drift.json" <<EOF
$(jq '.hostSchemaId="vscode-unknown-v9"' "$tmp/identity.json")
EOF
if "$VSCODE" v2 identify-session "$tmp/drift.json" >/dev/null 2>&1; then fail "schema drift accepted"; fi
pass "zero, multiple, and schema-drift identity inputs refuse"

cat >"$tmp/receipt.json" <<'EOF'
{"finishedAt":"2026-08-01T00:00:01.000Z","intentId":"intent:one","permitId":"permit:one","startedAt":"2026-08-01T00:00:00.000Z","usageReceiptId":"receipt:one"}
EOF
receipt="$($NONE v2 receipt "$tmp/receipt.json")"
[[ "$(printf '%s' "$receipt" | jq -r '.measurementStatus')" == unmeasured ]] || fail "unmeasured became measured zero"
[[ "$(printf '%s' "$receipt" | jq '.measurement | length')" == 0 ]] || fail "none receipt contains measurements"
pass "unmeasured remains explicit and never becomes measured zero"

mkdir -p "$tmp/reference-repo"
cat >"$tmp/usage-authority.json" <<'EOF'
{"contractType":"security-hmac-authority","schemaVersion":1,"purpose":"usage-receipt","authorityId":"authority:usage.selftest","trustRootId":"trust:usage.selftest","keyHex":"2222222222222222222222222222222222222222222222222222222222222222"}
EOF
chmod 600 "$tmp/usage-authority.json"
cat >"$tmp/reference-repo/bubbles-project.yaml" <<'EOF'
usage:
  adapter: reference-test
EOF
reference_resolution="$(BUBBLES_USAGE_REFERENCE_TEST=enabled "$RESOLVER" --repo-root "$tmp/reference-repo" --contract-major 2)"
printf '%s' "$reference_resolution" | grep -q '^adapter=reference-test$' || fail "reference adapter did not negotiate through resolver"
reference_description="$($REFERENCE v2 describe)"
[[ "$(printf '%s' "$reference_description" | jq -r '.adapterId')" == reference-test ]] || fail "reference adapter description missing"
if "$REFERENCE" receipt "$tmp/receipt.json" >/dev/null 2>&1; then fail "reference adapter enabled without explicit test control"; fi
reference_identity="$(BUBBLES_USAGE_REFERENCE_TEST=enabled "$REFERENCE" v2 identify-session "$tmp/identity.json")"
[[ "$(printf '%s' "$reference_identity" | jq -r '.adapterId')" == reference-test ]] || fail "reference identity was not adapter-originated"
cat >"$tmp/reference-quote.json" <<'EOF'
{"actionDigest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","attemptId":"attempt:one","budgetId":"budget:one","dimensionSetDigest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","epochId":"epoch:one","expiresAt":"2026-08-01T00:10:00.000Z","goalId":"goal:one","intentId":"intent:one","maximums":[{"amount":1,"currency":null,"dimension":"modelRequestCount","scale":null,"unit":"requests"}],"negotiationId":"negotiation:one","occurrenceId":"occurrence:one","quoteId":"quote:one","quotedAt":"2026-08-01T00:00:00.000Z","ruleDigest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","sessionIdentityId":"session:one"}
EOF
reference_quote="$(BUBBLES_USAGE_REFERENCE_TEST=enabled "$REFERENCE" v2 quote "$tmp/reference-quote.json")"
[[ "$(printf '%s' "$reference_quote" | jq -r '.adapterId')" == reference-test ]] || fail "reference quote was not adapter-originated"
cat >"$tmp/reference-receipt.json" <<'EOF'
{"actionDigest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","attemptId":"attempt:one","budgetId":"budget:one","childExitCode":0,"consumptionId":"consumption:one","epochId":"epoch:one","finishedAt":"2026-08-01T00:00:01.000Z","goalId":"goal:one","intentId":"intent:one","measurement":[{"amount":1,"currency":null,"dimension":"modelRequestCount","scale":null,"unit":"requests"}],"monotonicFinishedNs":1,"monotonicStartedNs":0,"occurrenceId":"occurrence:one","permitId":"permit:one","sessionIdentityId":"session:one","startedAt":"2026-08-01T00:00:00.000Z"}
EOF
reference_receipt="$(BUBBLES_USAGE_REFERENCE_TEST=enabled "$REFERENCE" v2 receipt "$tmp/reference-receipt.json")"
[[ "$(printf '%s' "$reference_receipt" | jq -r '.measurementStatus')" == measured ]] || fail "reference receipt was not measured"
printf '%s' "$reference_receipt" >"$tmp/reference-receipt-record.json"
jq -cS '{receipt:.,verifiedAt:"2026-08-01T00:00:02.000Z"}' "$tmp/reference-receipt-record.json" >"$tmp/reference-verification-input.json"
signature="$(python3 "$SCRIPT_DIR/security-authority.py" sign usage-receipt "$tmp/usage-authority.json" "$tmp/reference-verification-input.json")"
jq -cS --arg authenticator "$(printf '%s' "$signature" | jq -r '.authenticator')" '. + {authenticator:$authenticator}' "$tmp/reference-verification-input.json" >"$tmp/reference-verification-authenticated.json"
reference_verification="$(BUBBLES_USAGE_REFERENCE_TEST=enabled BUBBLES_USAGE_REFERENCE_AUTHORITY="$tmp/usage-authority.json" "$REFERENCE" v2 verify-receipt "$tmp/reference-verification-authenticated.json")"
[[ "$(printf '%s' "$reference_verification" | jq -r '.verdict')" == valid ]] || fail "reference verification was not valid"
forged="$(jq -cS '.authenticator="hmac-sha256:0000000000000000000000000000000000000000000000000000000000000000"' "$tmp/reference-verification-authenticated.json")"
printf '%s' "$forged" >"$tmp/reference-verification-forged.json"
if BUBBLES_USAGE_REFERENCE_TEST=enabled BUBBLES_USAGE_REFERENCE_AUTHORITY="$tmp/usage-authority.json" "$REFERENCE" v2 verify-receipt "$tmp/reference-verification-forged.json" >/dev/null 2>&1; then fail "forged receipt authenticator accepted"; fi
pass "reference lifecycle is explicit, disabled by default, and adapter-originated"

if "$NONE" v2 quote "$tmp/reference-receipt.json" >/dev/null 2>&1; then fail "none adapter became enforcement eligible"; fi
if "$VSCODE" v2 quote "$tmp/reference-receipt.json" >/dev/null 2>&1; then fail "vscode adapter became enforcement eligible"; fi
pass "none and vscode remain enforcement-ineligible negative controls"

if grep -Eq -- '--(skip|force|ignore|insecure|no-verify)' "$NONE" "$VSCODE" "$REFERENCE" "$RESOLVER"; then fail "bypass flag found"; fi
pass "v2 surfaces expose no bypass flags"
echo "usage-adapter-v2-selftest: PASS ($pass_count checks)"