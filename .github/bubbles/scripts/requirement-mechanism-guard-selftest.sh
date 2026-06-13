#!/usr/bin/env bash
set -uo pipefail

# requirement-mechanism-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/requirement-mechanism-guard.sh`
# (Gate G097 — requirement_mechanism_correspondence_gate).
#
# Stages disposable spec-tree fixtures under a `mktemp -d` workspace and
# asserts the exit-code contract plus key output tokens for the guard's real
# behaviors: the core "named-mechanism-absent-from-code" BLOCK (the exact
# downstream PKCE-fake shape), synonym-based code evidence clearing it, a
# justification clearing it, grandfathering, not-applicable, and the two
# advisory nudges (#4 negative-assertion, #3 fake-server-as-oracle).
#
# Scenarios:
#   S0   Missing feature dir argument                  → exit 2
#   S0b  Non-existent feature dir path                  → exit 2
#   S1   spec names PKCE, impl file present WITHOUT      → exit 1  (ADVERSARIAL:
#        pkce/code_verifier, no justification, new spec    the downstream fake —
#                                                          fails iff the gate
#                                                          actually correlates
#                                                          requirement to code)
#   S2   spec names PKCE, impl file CONTAINS             → exit 0  (synonym
#        code_verifier (a PKCE synonym)                     evidence clears it)
#   S3   spec names PKCE, impl absent, but a             → exit 0  (disclosure
#        `## Requirement-Mechanism Justifications`          clears it)
#        section discloses the difference
#   S4   spec names no concrete mechanism               → exit 0  (not applicable)
#   S5   same gap as S1 but createdAt before cutoff     → exit 0  (ADVERSARIAL:
#                                                          proves grandfather
#                                                          downgrade to warn)
#   S6   OAuth2 named + present in code, test file has   → exit 0  (#4 nudge
#        NO negative/rejection assertion                    prints, non-blocking)
#   S7   mechanism satisfied; a live-tier integration    → exit 0  (#3 nudge
#        test is backed only by httptest.NewServer          prints, non-blocking)
#   S8   spec names HMAC, impl WITHOUT hmac, new spec    → exit 1  (second
#                                                          mechanism beyond PKCE)
#
# Reference:
#   bubbles/registry/gates.yaml → G097
#   bubbles/scripts/requirement-mechanism-guard.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/requirement-mechanism-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g097-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# Stage a fresh spec dir (under a fake repo root) and emit its absolute path.
# Layout: $WORKSPACE/<name>/specs/097-fixture/ so the guard resolves repo_root
# to $WORKSPACE/<name> via the */specs/* fallback (mktemp is not a git repo).
new_spec_root() {
  local name="$1"
  local root="$WORKSPACE/$name"
  local spec="$root/specs/097-fixture"
  mkdir -p "$spec" "$root/src" "$root/tests"
  printf '%s' "$spec"
}

write_state() {
  local spec="$1" created="$2"
  cat >"$spec/state.json" <<EOF
{
  "version": 3,
  "status": "in_progress",
  "createdAt": "$created"
}
EOF
}

RC=""
OUT=""
run_guard() {
  OUT="$(bash "$GUARD_SCRIPT" "$@" 2>&1)"
  RC=$?
}

# -----------------------------------------------------------------------
# S0: missing feature dir argument → exit 2
# -----------------------------------------------------------------------
run_guard
if [[ "$RC" -eq 2 ]]; then pass "S0 missing feature dir exits 2"; else bad "S0 expected exit 2, got $RC"; fi

# -----------------------------------------------------------------------
# S0b: non-existent feature dir path → exit 2
# -----------------------------------------------------------------------
run_guard "$WORKSPACE/does-not-exist-$$"
if [[ "$RC" -eq 2 ]]; then pass "S0b non-existent feature dir exits 2"; else bad "S0b expected exit 2, got $RC"; fi

# -----------------------------------------------------------------------
# S1: PKCE named, impl present WITHOUT pkce, no justification, new spec → exit 1
# (ADVERSARIAL — the downstream fake shape)
# -----------------------------------------------------------------------
s1="$(new_spec_root s1-pkce-fake)"
s1root="$WORKSPACE/s1-pkce-fake"
write_state "$s1" "2026-06-09"
cat >"$s1/spec.md" <<'EOF'
# OAuth Connector

## Requirements

- NC-1: The connector MUST authenticate using OAuth2 with PKCE (code_verifier).
EOF
cat >"$s1/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/connector.go`

## Test Plan

| Type | File |
|------|------|
| integration | `tests/connector_test.go` |
EOF
cat >"$s1root/src/connector.go" <<'EOF'
package connector

// Authenticate attaches the bearer token and calls the upstream API.
func Authenticate(token string) error {
	req.Header.Set("Authorization", "Bearer "+token)
	return doRequest(req)
}
EOF
cat >"$s1root/tests/connector_test.go" <<'EOF'
package connector

func TestAuthenticate(t *testing.T) {
	if err := Authenticate("abc"); err != nil {
		t.Fatal(err)
	}
}
EOF
run_guard "$s1"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "G097 BLOCK"; then
  pass "S1 PKCE named but absent from code BLOCKs (exit 1)"
else
  bad "S1 expected exit 1 with BLOCK, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S2: PKCE named, impl CONTAINS code_verifier (synonym) → exit 0
# -----------------------------------------------------------------------
s2="$(new_spec_root s2-pkce-real)"
s2root="$WORKSPACE/s2-pkce-real"
write_state "$s2" "2026-06-09"
cat >"$s2/spec.md" <<'EOF'
# OAuth Connector

## Requirements

- NC-1: The connector MUST authenticate using PKCE.
EOF
cat >"$s2/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/connector.go`
EOF
cat >"$s2root/src/connector.go" <<'EOF'
package connector

// buildAuthURL implements the PKCE code_verifier / code_challenge exchange.
func buildAuthURL(codeVerifier string) string {
	challenge := sha256Base64URL(codeVerifier)
	return "https://idp/authorize?code_challenge=" + challenge
}
EOF
run_guard "$s2"
if [[ "$RC" -eq 0 ]]; then pass "S2 PKCE with code_verifier evidence passes (exit 0)"; else bad "S2 expected exit 0, got $RC; out=$OUT"; fi

# -----------------------------------------------------------------------
# S3: PKCE named, impl absent, but justification section discloses → exit 0
# -----------------------------------------------------------------------
s3="$(new_spec_root s3-justified)"
s3root="$WORKSPACE/s3-justified"
write_state "$s3" "2026-06-09"
cat >"$s3/spec.md" <<'EOF'
# OAuth Connector

## Requirements

- NC-1: The connector MUST authenticate using OAuth2 with PKCE.

## Requirement-Mechanism Justifications

- PKCE: the upstream IdP does not support PKCE for confidential clients; we use
  the authorization_code grant with a server-side client secret instead. This
  naming difference is intentional and reviewed.
EOF
cat >"$s3/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/connector.go`
EOF
cat >"$s3root/src/connector.go" <<'EOF'
package connector

func exchange(code, clientSecret string) error {
	return postForm("authorization_code", code, clientSecret)
}
EOF
run_guard "$s3"
if [[ "$RC" -eq 0 ]]; then pass "S3 justified naming difference passes (exit 0)"; else bad "S3 expected exit 0, got $RC; out=$OUT"; fi

# -----------------------------------------------------------------------
# S4: no concrete mechanism named → exit 0 (not applicable)
# -----------------------------------------------------------------------
s4="$(new_spec_root s4-na)"
s4root="$WORKSPACE/s4-na"
write_state "$s4" "2026-06-09"
cat >"$s4/spec.md" <<'EOF'
# Pagination Feature

## Requirements

- FR-1: The list endpoint MUST return 25 items per page with a cursor.
EOF
cat >"$s4/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/list.go`
EOF
cat >"$s4root/src/list.go" <<'EOF'
package api
func list(cursor string) {}
EOF
run_guard "$s4"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "not applicable"; then
  pass "S4 no mechanism named is not applicable (exit 0)"
else
  bad "S4 expected exit 0 not-applicable, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S5: same gap as S1 but createdAt before cutoff → exit 0 (grandfathered)
# (ADVERSARIAL — proves the grandfather downgrade)
# -----------------------------------------------------------------------
s5="$(new_spec_root s5-grandfathered)"
s5root="$WORKSPACE/s5-grandfathered"
write_state "$s5" "2026-05-01"
cat >"$s5/spec.md" <<'EOF'
# OAuth Connector

## Requirements

- NC-1: The connector MUST authenticate using OAuth2 with PKCE.
EOF
cat >"$s5/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/connector.go`
EOF
cat >"$s5root/src/connector.go" <<'EOF'
package connector
func Authenticate(token string) error { return nil }
EOF
run_guard "$s5"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "grandfathered\|DOWNGRADED"; then
  pass "S5 pre-cutoff spec is grandfathered to warning (exit 0)"
else
  bad "S5 expected exit 0 grandfathered, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S6: OAuth2 satisfied in code, test lacks negative assertion → exit 0 + #4 nudge
# -----------------------------------------------------------------------
s6="$(new_spec_root s6-neg-nudge)"
s6root="$WORKSPACE/s6-neg-nudge"
write_state "$s6" "2026-06-09"
cat >"$s6/spec.md" <<'EOF'
# OAuth Connector

## Requirements

- NC-1: The connector MUST authenticate using OAuth2.
EOF
cat >"$s6/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/connector.go`

## Test Plan

| Type | File |
|------|------|
| integration | `tests/connector_test.go` |
EOF
cat >"$s6root/src/connector.go" <<'EOF'
package connector
// uses oauth2 authorization_code flow
func auth() { useOAuth2() }
EOF
cat >"$s6root/tests/connector_test.go" <<'EOF'
package connector
func TestHappyPath(t *testing.T) {
	if err := auth(); err != nil { t.Fatal(err) }
}
EOF
run_guard "$s6"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "NUDGE" && printf '%s' "$OUT" | grep -qi "negative/rejection"; then
  pass "S6 satisfied mechanism with no negative assertion emits #4 nudge, non-blocking (exit 0)"
else
  bad "S6 expected exit 0 with negative-assertion nudge, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S7: mechanism satisfied; live-tier test backed by httptest.NewServer → #3 nudge
# -----------------------------------------------------------------------
s7="$(new_spec_root s7-fakeserver-nudge)"
s7root="$WORKSPACE/s7-fakeserver-nudge"
write_state "$s7" "2026-06-09"
cat >"$s7/spec.md" <<'EOF'
# OAuth Connector

## Requirements

- NC-1: The connector MUST authenticate using HMAC request signing.
EOF
cat >"$s7/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/connector.go`

## Test Plan

| Type | File |
|------|------|
| e2e-api | `tests/connector_test.go` |
EOF
cat >"$s7root/src/connector.go" <<'EOF'
package connector
// signs each request with hmac-sha256
func sign() { computeHMAC() }
EOF
cat >"$s7root/tests/connector_test.go" <<'EOF'
package connector
import "net/http/httptest"
func TestAgainstFake(t *testing.T) {
	srv := httptest.NewServer(handler)
	defer srv.Close()
	callAgainst(srv.URL)
}
EOF
run_guard "$s7"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "in-process fake server"; then
  pass "S7 live-tier test backed by httptest.NewServer emits #3 nudge, non-blocking (exit 0)"
else
  bad "S7 expected exit 0 with fake-server nudge, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S8: HMAC named, impl WITHOUT hmac, new spec → exit 1 (second mechanism)
# -----------------------------------------------------------------------
s8="$(new_spec_root s8-hmac-gap)"
s8root="$WORKSPACE/s8-hmac-gap"
write_state "$s8" "2026-06-09"
cat >"$s8/spec.md" <<'EOF'
# Webhook Receiver

## Requirements

- NC-1: Inbound webhooks MUST be verified with an HMAC signature.
EOF
cat >"$s8/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/webhook.go`
EOF
cat >"$s8root/src/webhook.go" <<'EOF'
package webhook
// TODO verify signature
func receive(body []byte) { process(body) }
EOF
run_guard "$s8"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "HMAC"; then
  pass "S8 HMAC named but absent from code BLOCKs (exit 1)"
else
  bad "S8 expected exit 1 for HMAC gap, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# Verdict
# -----------------------------------------------------------------------
echo
echo "============================================================"
echo "  requirement-mechanism-guard selftest verdict"
echo "    passed assertions: $PASS_COUNT"
echo "    failed assertions: $FAIL_COUNT"
echo "============================================================"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  printf '  FAILED: %s\n' "${FAILED_SCENARIOS[@]}" >&2
  echo "requirement-mechanism-guard-selftest: FAILED" >&2
  exit 1
fi
echo "requirement-mechanism-guard-selftest: PASSED"
exit 0
