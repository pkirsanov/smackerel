#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCAN_SCRIPT="$SCRIPT_DIR/implementation-reality-scan.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
TMPDIR="$(mktemp -d)"
FIXTURE_ROOT="$TMPDIR/fixtures"

trap 'rm -rf "$TMPDIR"' EXIT INT TERM

# shellcheck source=/dev/null
source "$GUARD_LIB"

failures=0
RUN_OUTPUT=""
RUN_STATUS=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

assert_output_contains() {
  local expected="$1"
  local label="$2"
  if printf '%s\n' "$RUN_OUTPUT" | grep -Fq -- "$expected"; then
    pass "$label"
  else
    fail "$label (missing: $expected)"
  fi
}

assert_output_not_contains() {
  local forbidden="$1"
  local label="$2"
  if printf '%s\n' "$RUN_OUTPUT" | grep -Fq -- "$forbidden"; then
    fail "$label (unexpected: $forbidden)"
  else
    pass "$label"
  fi
}

run_scan_in_repo() {
  local repo_root="$1"
  local feature_dir="$2"
  local output_file="$TMPDIR/run-scan-in-repo.txt"
  RUN_OUTPUT=""
  RUN_STATUS=0
  if (
    cd "$repo_root" || exit 2
    bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose
  ) >"$output_file" 2>&1; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(cat "$output_file")"
  printf '%s\n' "$RUN_OUTPUT"
}

run_expect_success() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local output_file="$TMPDIR/run-expect-success.txt"

  if bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose >"$output_file" 2>&1; then
    output="$(cat "$output_file")"
    echo "$output"
    pass "$label"
  else
    output="$(cat "$output_file")"
    echo "$output"
    fail "$label"
  fi
}

run_expect_zero_files_failure() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local output_file="$TMPDIR/run-expect-zero-files.txt"
  local status=0

  if bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose >"$output_file" 2>&1; then
    output="$(cat "$output_file")"
    echo "$output"
    fail "$label"
    return
  else
    status=$?
    output="$(cat "$output_file")"
    echo "$output"
  fi

  if [[ "$status" -eq 1 ]] && grep -Fq 'ZERO_FILES_RESOLVED' <<< "$output"; then
    pass "$label"
  else
    fail "$label"
  fi
}

run_expect_fake_integration_failure() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local output_file="$TMPDIR/run-expect-fake-integration.txt"
  local status=0

  if bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose >"$output_file" 2>&1; then
    output="$(cat "$output_file")"
    echo "$output"
    fail "$label"
    return
  else
    status=$?
    output="$(cat "$output_file")"
    echo "$output"
  fi

  if [[ "$status" -eq 1 ]] && grep -Fq 'FAKE_INTEGRATION' <<< "$output"; then
    pass "$label"
  else
    fail "$label"
  fi
}

create_shell_heavy_fixture() {
  local feature_dir="$FIXTURE_ROOT/shell-heavy-feature"
  mkdir -p "$feature_dir/scripts" "$feature_dir/config" "$feature_dir/docs"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Shell Heavy Fixture

## Scope 1: Inventory Discovery

### Implementation Files

- \`$feature_dir/scripts/validate.sh\`
- \`$feature_dir/config/service.yaml\`
- \`$feature_dir/config/service.yml\`
- \`$feature_dir/config/schema.json\`
- \`$feature_dir/docs/operator.md\`
EOF

  cat > "$feature_dir/scripts/validate.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "fixture validation complete"
EOF

  cat > "$feature_dir/config/service.yaml" <<'EOF'
service: shell-heavy
mode: explicit
EOF

  cat > "$feature_dir/config/service.yml" <<'EOF'
service: shell-heavy-yml
mode: explicit
EOF

  cat > "$feature_dir/config/schema.json" <<'EOF'
{"service":"shell-heavy","mode":"explicit"}
EOF

  cat > "$feature_dir/docs/operator.md" <<'EOF'
# Operator Notes

This fixture proves non-code implementation inventories are still resolved.
EOF
}

create_missing_inventory_fixture() {
  local feature_dir="$FIXTURE_ROOT/missing-inventory-feature"
  mkdir -p "$feature_dir"

  cat > "$feature_dir/scopes.md" <<'EOF'
# Scopes: Missing Inventory Fixture

## Scope 1: Missing Inventory

This scope intentionally has no backtick-wrapped implementation file paths.
EOF
}

create_go_connector_package_fixture() {
  local feature_dir="$FIXTURE_ROOT/go-connector-package-feature"
  local package_dir="$feature_dir/internal/connector/honest"
  mkdir -p "$package_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Go Connector Package Fixture

## Scope 1: Honest Connector Helpers

### Implementation Files

- \`$package_dir/client.go\`
- \`$package_dir/capability.go\`
- \`$package_dir/normalizer.go\`
EOF

  cat > "$package_dir/client.go" <<'EOF'
package honest

import (
	"context"
	"net/http"
)

type Client struct {
	httpClient *http.Client
}

func (c *Client) Fetch(ctx context.Context, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
EOF

  cat > "$package_dir/capability.go" <<'EOF'
package honest

import "fmt"

func ValidateCapability(version string) error {
	if version == "" {
		return fmt.Errorf("capability version is required")
	}
	return nil
}
EOF

  cat > "$package_dir/normalizer.go" <<'EOF'
package honest

type Artifact struct {
	ID string
}

type DegradedDiagnostic struct {
	Reason string
}

func Normalize(raw string) (*Artifact, *DegradedDiagnostic) {
	if raw == "" {
		return nil, &DegradedDiagnostic{Reason: "missing trusted artifact"}
	}
	return &Artifact{ID: raw}, nil
}
EOF
}

create_fake_connector_fixture() {
  local feature_dir="$FIXTURE_ROOT/fake-connector-feature"
  local package_dir="$feature_dir/internal/connector/external"
  mkdir -p "$package_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Fake Connector Fixture

## Scope 1: No-op Connector

### Implementation Files

- \`$package_dir/connector.go\`
EOF

  cat > "$package_dir/connector.go" <<'EOF'
package external

type Connector struct{}

func (c *Connector) Sync() error {
	return nil
}
EOF
}

create_sensitive_storage_fixture() {
  SENSITIVE_REPO="$FIXTURE_ROOT/sensitive-storage-repo"
  SENSITIVE_FEATURE="$SENSITIVE_REPO/specs/001-sensitive-storage"
  SENSITIVE_SOURCE="$SENSITIVE_REPO/src/provider-client.js"
  SENSITIVE_DART_SOURCE="$SENSITIVE_REPO/src/provider-preferences.dart"
  SENSITIVE_CONFIG="$SENSITIVE_REPO/.github/bubbles-project.yaml"
  mkdir -p "$SENSITIVE_FEATURE" "$(dirname "$SENSITIVE_SOURCE")" "$(dirname "$SENSITIVE_CONFIG")"

  cat > "$SENSITIVE_FEATURE/scopes.md" <<'EOF'
# Scope 1: Sensitive Storage Selftest

### Implementation Files

- `src/provider-client.js`
- `src/provider-preferences.dart`
EOF

  cat > "$SENSITIVE_SOURCE" <<'EOF'
const KEY = "marketProvider:twelvedata:apiKey";
const KEY_ALIAS = KEY;
const UNKNOWN_KEY = "marketProvider:unknown-vendor:apiKey";
const CACHE_KEY = "marketCache:latest";
localStorage.setItem(KEY_ALIAS, providerCredential);
sessionStorage.setItem(KEY, providerCredential);
sessionStorage.setItem(UNKNOWN_KEY, providerCredential);
sessionStorage.setItem(`marketProvider:${provider}:apiKey`, providerCredential);
localStorage.setItem(KEY, providerCredential);
sessionStorage.setItem(KEY, authBearerToken);
localStorage.setItem(CACHE_KEY, marketSnapshot); // auth token and payment secret are comments only
localStorage.removeItem("legacyAuthToken");
const beforeScrub = { apiKey: providerCredential, price: 42 };
localStorage.setItem("marketCache:before", JSON.stringify(beforeScrub));
const afterScrub = { apiKey: providerCredential, authToken: authBearerToken, price: 42 };
delete afterScrub.apiKey;
delete afterScrub.authToken;
localStorage.setItem("marketCache:after", JSON.stringify(afterScrub));
indexedDB.open("authCredentialDatabase");
SharedPreferences.putString("refreshToken", refreshToken);
AsyncStorage.multiSet("paymentCard", paymentCardNumber);
const transaction = providerDatabase.transaction("credentials", "readwrite");
const credentialStore = transaction.objectStore("credentials");
credentialStore.put(providerCredential, KEY);
EOF

  cat > "$SENSITIVE_DART_SOURCE" <<'EOF'
Future<void> persistProviderCredential(
  SharedPreferences preferences,
  String providerCredential,
) async {
  await preferences.setString(
    "marketProvider:twelvedata:apiKey",
    providerCredential,
  );
}
EOF

  write_sensitive_valid_config
}

write_sensitive_valid_config() {
  cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage: sessionStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
EOF
}

assert_sensitive_invalid_config() {
  local label="$1"
  run_scan_in_repo "$SENSITIVE_REPO" "$SENSITIVE_FEATURE"
  if [[ "$RUN_STATUS" -eq 1 ]]; then
    pass "$label blocks"
  else
    fail "$label blocks (expected exit 1, got $RUN_STATUS)"
  fi
  assert_output_contains "reason=SENSITIVE_STORAGE_CONFIG_INVALID" "$label reports config integrity"
}

create_shell_heavy_fixture
create_missing_inventory_fixture
create_go_connector_package_fixture
create_fake_connector_fixture
create_sensitive_storage_fixture

echo "Running implementation-reality-scan discovery selftest..."
echo "Scenario: shell-heavy fixtures resolve honest implementation inventory."
run_expect_success "$FIXTURE_ROOT/shell-heavy-feature" "Shell-heavy fixture resolves .sh/.yaml/.yml/.json/docs-backed inventory"

echo "Scenario: missing inventories still fail with ZERO_FILES_RESOLVED."
run_expect_zero_files_failure "$FIXTURE_ROOT/missing-inventory-feature" "Missing-inventory fixture fails honestly without shim files"

echo "Scenario: Go connector helper nil returns are not fake when the package has a real transport client."
run_expect_success "$FIXTURE_ROOT/go-connector-package-feature" "Go connector helper return nil lines pass when a sibling client performs external calls"

echo "Scenario: no-op connector still fails external integration authenticity."
run_expect_fake_integration_failure "$FIXTURE_ROOT/fake-connector-feature" "No-op connector without an external call is still flagged as FAKE_INTEGRATION"

echo "Scenario: semantic Scan 2B distinguishes storage operations and exact session classification."
run_scan_in_repo "$SENSITIVE_REPO" "$SENSITIVE_FEATURE"
if [[ "$RUN_STATUS" -eq 1 ]]; then
  pass "Sensitive storage matrix retains blocking findings"
else
  fail "Sensitive storage matrix retains blocking findings (expected exit 1, got $RUN_STATUS)"
fi
assert_output_contains "reason=DURABLE_CREDENTIAL_STORAGE storage=localStorage operation=persist key=marketProvider:twelvedata:apiKey provider=twelvedata" "Literal and alias-resolved durable credentials are blocked"
assert_output_not_contains "src/provider-client.js:6" "Exact configured session credential is allowed"
assert_output_contains "reason=SESSION_PROVIDER_UNKNOWN" "Unknown session provider is blocked distinctly"
assert_output_contains "reason=SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED" "Dynamic session provider is blocked unresolved"
assert_output_contains "reason=FORBIDDEN_SECRET_CLASS storage=sessionStorage" "High-trust session material cannot use approval"
assert_output_not_contains "src/provider-client.js:11" "Inline comment vocabulary does not taint cache"
assert_output_not_contains "src/provider-client.js:12" "removeItem remains cleanup"
assert_output_contains "src/provider-client.js:14" "Credential object before scrub remains blocking"
assert_output_not_contains "src/provider-client.js:18" "Proven scrubbed rewrite remains clear"
assert_output_contains "storage=indexedDB operation=read" "IndexedDB credential access remains covered"
assert_output_contains "storage=SharedPreferences operation=persist" "SharedPreferences credential persistence remains covered"
assert_output_contains "storage=AsyncStorage operation=persist" "AsyncStorage credential persistence remains covered"
assert_output_contains "storage=indexedDB operation=persist key=marketProvider:twelvedata:apiKey" "IndexedDB object-store credential persistence remains covered"
assert_output_contains "src/provider-preferences.dart" "SharedPreferences instance credential persistence remains covered"

echo "Scenario: sensitive storage project configuration fails closed."
cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: ../src/*.js
        storage: sessionStorage
        key: marketProvider:*:apiKey
        provider: '*'
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
EOF
assert_sensitive_invalid_config "Traversal and wildcard approval"

cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage: sessionStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
      - path: src/provider-client.js
        storage: sessionStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
EOF
assert_sensitive_invalid_config "Duplicate approval tuple"

cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    unknownField: true
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage: localStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: auth-token
        privilege: high
        lifetime: durable
EOF
assert_sensitive_invalid_config "Unknown field and enum values"

cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage sessionStorage
        key: marketProvider:twelvedata:apiKey
EOF
assert_sensitive_invalid_config "Malformed sensitive storage YAML"

write_sensitive_valid_config
NO_PARSER_PATH="$TMPDIR/no-parser-path"
mkdir -p "$NO_PARSER_PATH"
for tool_name in awk basename cat cut dirname find grep head sed sort tr wc; do
  tool_path="$(command -v "$tool_name" 2>/dev/null || true)"
  [[ -z "$tool_path" ]] || ln -s "$tool_path" "$NO_PARSER_PATH/$tool_name"
done
parser_output=""
parser_status=0
if parser_output="$(
  cd "$SENSITIVE_REPO" || exit 2
  env -i PATH="$NO_PARSER_PATH" /bin/bash "$SCAN_SCRIPT" "$SENSITIVE_FEATURE" --verbose 2>&1
)"; then
  parser_status=0
else
  parser_status=$?
fi
printf '%s\n' "$parser_output"
if [[ "$parser_status" -eq 1 ]] && printf '%s\n' "$parser_output" | grep -Fq 'reason=SENSITIVE_STORAGE_CONFIG_INVALID'; then
  pass "Parser-unavailable configured approval fails closed"
else
  fail "Parser-unavailable configured approval fails closed"
fi

echo "Scenario: portable watchdog preserves exit 124 without GNU coreutils."
NO_TIMEOUT_PATH="$TMPDIR/no-timeout-path"
mkdir -p "$NO_TIMEOUT_PATH"
ln -s "$(command -v sleep)" "$NO_TIMEOUT_PATH/sleep"
portable_timeout_status=0
if PATH="$NO_TIMEOUT_PATH" bubbles_run_with_timeout 1 /bin/sleep 5; then
  portable_timeout_status=0
else
  portable_timeout_status=$?
fi
if [[ "$portable_timeout_status" -eq 124 ]]; then
  echo "PORTABLE_WATCHDOG_FALLBACK=124"
  pass "Portable watchdog preserves exit 124"
else
  echo "PORTABLE_WATCHDOG_FALLBACK=$portable_timeout_status"
  fail "Portable watchdog preserves exit 124"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "implementation-reality-scan selftest failed with $failures issue(s)."
  exit 1
fi

echo "implementation-reality-scan selftest passed."