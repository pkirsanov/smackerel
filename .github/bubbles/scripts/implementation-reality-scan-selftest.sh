#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCAN_SCRIPT="$SCRIPT_DIR/implementation-reality-scan.sh"
TMPDIR="$(mktemp -d)"
FIXTURE_ROOT="$TMPDIR/fixtures"

trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

run_expect_success() {
  local feature_dir="$1"
  local label="$2"
  local output=""

  if output="$(timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose 2>&1)"; then
    echo "$output"
    pass "$label"
  else
    echo "$output"
    fail "$label"
  fi
}

run_expect_zero_files_failure() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local status=0

  if output="$(timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose 2>&1)"; then
    echo "$output"
    fail "$label"
    return
  else
    status=$?
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
  local status=0

  if output="$(timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose 2>&1)"; then
    echo "$output"
    fail "$label"
    return
  else
    status=$?
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

create_shell_heavy_fixture
create_missing_inventory_fixture
create_go_connector_package_fixture
create_fake_connector_fixture

echo "Running implementation-reality-scan discovery selftest..."
echo "Scenario: shell-heavy fixtures resolve honest implementation inventory."
run_expect_success "$FIXTURE_ROOT/shell-heavy-feature" "Shell-heavy fixture resolves .sh/.yaml/.yml/.json/docs-backed inventory"

echo "Scenario: missing inventories still fail with ZERO_FILES_RESOLVED."
run_expect_zero_files_failure "$FIXTURE_ROOT/missing-inventory-feature" "Missing-inventory fixture fails honestly without shim files"

echo "Scenario: Go connector helper nil returns are not fake when the package has a real transport client."
run_expect_success "$FIXTURE_ROOT/go-connector-package-feature" "Go connector helper return nil lines pass when a sibling client performs external calls"

echo "Scenario: no-op connector still fails external integration authenticity."
run_expect_fake_integration_failure "$FIXTURE_ROOT/fake-connector-feature" "No-op connector without an external call is still flagged as FAKE_INTEGRATION"

if [[ "$failures" -gt 0 ]]; then
  echo "implementation-reality-scan selftest failed with $failures issue(s)."
  exit 1
fi

echo "implementation-reality-scan selftest passed."