#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/trace-contract-guard.sh"
TMP_BASE="${TMPDIR:-$HOME/.cache}"
mkdir -p "$TMP_BASE"
TMP_DIR="$(mktemp -d -p "$TMP_BASE" bubbles-trace-contract.XXXXXX)"
failures=0

cleanup() {
  if (( failures == 0 )) && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "Preserving selftest workspace: $TMP_DIR" >&2
  fi
}
trap cleanup EXIT

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

contract_file="$TMP_DIR/bubbles-project.yaml"
trace_good="$TMP_DIR/trace-good.txt"
trace_missing="$TMP_DIR/trace-missing.txt"
trace_redflag="$TMP_DIR/trace-redflag.txt"
trace_optional="$TMP_DIR/trace-optional.txt"

cat > "$contract_file" <<'YAML'
traceContracts:
  workflows:
    booking.create:
      requiredSpans:
        - name: http.request
          attributes:
            - trace_id
            - booking.id
      requiredAttributes:
        - tenant.id
      requiredInvariants:
        - booking emitted exactly one confirmation event
      redFlags:
        error:
          - Missing trace_id
        warning:
          - slow span
YAML

cat > "$trace_good" <<'EOF'
SPAN http.request
ATTR trace_id=abc
ATTR booking.id=booking-123
ATTR tenant.id=tenant-1
INVARIANT booking emitted exactly one confirmation event
EOF

cat > "$trace_missing" <<'EOF'
SPAN http.request
ATTR trace_id=abc
ATTR tenant.id=tenant-1
EOF

cat > "$trace_redflag" <<'EOF'
SPAN http.request
ATTR trace_id=abc
ATTR booking.id=booking-123
ATTR tenant.id=tenant-1
INVARIANT booking emitted exactly one confirmation event
ERROR Missing trace_id
EOF

cat > "$trace_optional" <<'EOF'
SPAN nothing-required
EOF

if "$GUARD" --contract "$contract_file" --workflow booking.create --trace-output "$trace_good" >/dev/null; then
  pass "valid trace evidence satisfies required spans, attributes, and invariants"
else
  fail "valid trace evidence satisfies required spans, attributes, and invariants"
fi

if "$GUARD" --contract "$contract_file" --workflow booking.create --trace-output "$trace_missing" >/dev/null 2>&1; then
  fail "missing required attribute fails the guard"
else
  pass "missing required attribute fails the guard"
fi

if "$GUARD" --contract "$contract_file" --workflow booking.create --trace-output "$trace_redflag" >/dev/null 2>&1; then
  fail "error red flag fails the guard"
else
  pass "error red flag fails the guard"
fi

if "$GUARD" --contract "$TMP_DIR/no-such.yaml" --trace-output "$trace_optional" >/dev/null; then
  pass "missing optional contract is non-blocking without --require-config"
else
  fail "missing optional contract is non-blocking without --require-config"
fi

if "$GUARD" --require-config --contract "$TMP_DIR/no-such.yaml" --trace-output "$trace_optional" >/dev/null 2>&1; then
  fail "--require-config fails when traceContracts are absent"
else
  pass "--require-config fails when traceContracts are absent"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "trace-contract-guard selftest failed with $failures issue(s)."
  exit 1
fi

echo "trace-contract-guard selftest passed."
