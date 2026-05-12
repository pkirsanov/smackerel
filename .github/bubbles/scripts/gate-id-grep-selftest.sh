#!/usr/bin/env bash
# gate-id-grep-selftest.sh
#
# Hermetic selftest for gate-id-grep.sh.
#
# Stages three synthetic Bubbles-style repos under a temp directory:
#   - clean fixture              -> default + strict both PASS
#   - duplicate-adjacent fixture -> default FAILS, strict FAILS
#   - unknown-G099 fixture       -> default PASSES (no dupes),
#                                   strict FAILS on the unknown ID
#                                   (G099 is in the canonical < G100 range
#                                   but is NOT defined in workflows.yaml)
#
# Asserts:
#   - clean default exits 0 and prints "OK — zero findings"
#   - clean strict  exits 0 and prints "OK — zero findings"
#   - duplicate default exits 1 and prints
#       "FINDING: duplicate-adjacent:" with G028 mentioned
#   - duplicate strict  exits 1 and prints
#       "FINDING: duplicate-adjacent:" with G028 mentioned
#   - unknown   default exits 0 (defaults ignore unknown IDs)
#   - unknown   strict  exits 1 and prints
#       "FINDING: unknown-gate-id:" with G099 mentioned
#   - G100+ references in the clean fixture do NOT trigger findings
#     under --strict (custom-gate allowlist works)
#
# Cleans up on exit.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/gate-id-grep.sh"

if [[ ! -f "$TARGET" ]]; then
  echo "[selftest gate-id-grep] FAIL: target script missing at $TARGET" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

# Helper: build a minimal repo skeleton with workflows.yaml that defines
# G028 and G044 in canonical requiredGates lines.
seed_repo() {
  local root="$1"
  mkdir -p "$root/agents" "$root/instructions" "$root/docs" \
    "$root/bubbles/scripts"
  cat > "$root/bubbles/workflows.yaml" <<'EOF'
gates:
  G028:
    description: implementation reality scan
  G044:
    description: cross-spec regression

workflows:
  delivery-gate-baseline:
    requiredGates: [ G028, G044 ]
EOF
}

# --- Clean fixture --------------------------------------------------------

clean_root="$TMPDIR/repo-clean"
seed_repo "$clean_root"
cat > "$clean_root/agents/clean-doc.md" <<'EOF'
# clean-doc

Mentions canonical gates G028 and G044 in a normal sentence,
plus a custom local gate G100 that should always be allowed.
EOF

set +e
clean_default_log="$TMPDIR/clean-default.log"
bash "$TARGET" --repo-root "$clean_root" >"$clean_default_log" 2>&1
clean_default_rc=$?
set -e

if [[ "$clean_default_rc" -eq 0 ]]; then
  pass "clean default exits 0"
else
  fail "clean default expected exit 0, got $clean_default_rc"
  sed -n '1,40p' "$clean_default_log"
fi

if grep -Fq "OK — zero findings" "$clean_default_log"; then
  pass "clean default reports zero findings"
else
  fail "clean default missing 'OK — zero findings'"
  sed -n '1,40p' "$clean_default_log"
fi

set +e
clean_strict_log="$TMPDIR/clean-strict.log"
bash "$TARGET" --repo-root "$clean_root" --strict >"$clean_strict_log" 2>&1
clean_strict_rc=$?
set -e

if [[ "$clean_strict_rc" -eq 0 ]]; then
  pass "clean strict exits 0 (G100+ allowed)"
else
  fail "clean strict expected exit 0, got $clean_strict_rc"
  sed -n '1,40p' "$clean_strict_log"
fi

if grep -Fq "OK — zero findings" "$clean_strict_log"; then
  pass "clean strict reports zero findings"
else
  fail "clean strict missing 'OK — zero findings'"
  sed -n '1,40p' "$clean_strict_log"
fi

# --- Duplicate-adjacent fixture ------------------------------------------

dup_root="$TMPDIR/repo-dup"
seed_repo "$dup_root"
cat > "$dup_root/agents/dup-doc.md" <<'EOF'
# dup-doc

This line has a copy-paste regression: G028, G028 in a row.
EOF

set +e
dup_default_log="$TMPDIR/dup-default.log"
bash "$TARGET" --repo-root "$dup_root" >"$dup_default_log" 2>&1
dup_default_rc=$?
set -e

if [[ "$dup_default_rc" -eq 1 ]]; then
  pass "duplicate default exits 1"
else
  fail "duplicate default expected exit 1, got $dup_default_rc"
  sed -n '1,40p' "$dup_default_log"
fi

if grep -Fq "FINDING: duplicate-adjacent:" "$dup_default_log" \
   && grep -Fq "G028" "$dup_default_log"; then
  pass "duplicate default reports duplicate-adjacent G028"
else
  fail "duplicate default missing duplicate-adjacent G028 finding"
  sed -n '1,40p' "$dup_default_log"
fi

set +e
dup_strict_log="$TMPDIR/dup-strict.log"
bash "$TARGET" --repo-root "$dup_root" --strict >"$dup_strict_log" 2>&1
dup_strict_rc=$?
set -e

if [[ "$dup_strict_rc" -eq 1 ]]; then
  pass "duplicate strict exits 1"
else
  fail "duplicate strict expected exit 1, got $dup_strict_rc"
  sed -n '1,40p' "$dup_strict_log"
fi

if grep -Fq "FINDING: duplicate-adjacent:" "$dup_strict_log" \
   && grep -Fq "G028" "$dup_strict_log"; then
  pass "duplicate strict still reports the duplicate-adjacent finding"
else
  fail "duplicate strict missing duplicate-adjacent G028 finding"
  sed -n '1,40p' "$dup_strict_log"
fi

# --- Unknown-G099 fixture -------------------------------------------------

unk_root="$TMPDIR/repo-unknown"
seed_repo "$unk_root"
cat > "$unk_root/instructions/unknown-doc.instructions.md" <<'EOF'
# unknown-doc

References an unknown gate ID G099 that is NOT in workflows.yaml
(G099 is in the canonical < G100 range so it is NOT auto-allowed).
Also references canonical G028 normally.
EOF

set +e
unk_default_log="$TMPDIR/unk-default.log"
bash "$TARGET" --repo-root "$unk_root" >"$unk_default_log" 2>&1
unk_default_rc=$?
set -e

if [[ "$unk_default_rc" -eq 0 ]]; then
  pass "unknown default exits 0 (defaults ignore unknowns)"
else
  fail "unknown default expected exit 0, got $unk_default_rc"
  sed -n '1,40p' "$unk_default_log"
fi

if grep -Fq "OK — zero findings" "$unk_default_log"; then
  pass "unknown default reports zero findings"
else
  fail "unknown default missing 'OK — zero findings'"
  sed -n '1,40p' "$unk_default_log"
fi

set +e
unk_strict_log="$TMPDIR/unk-strict.log"
bash "$TARGET" --repo-root "$unk_root" --strict >"$unk_strict_log" 2>&1
unk_strict_rc=$?
set -e

if [[ "$unk_strict_rc" -eq 1 ]]; then
  pass "unknown strict exits 1"
else
  fail "unknown strict expected exit 1, got $unk_strict_rc"
  sed -n '1,40p' "$unk_strict_log"
fi

if grep -Fq "FINDING: unknown-gate-id:" "$unk_strict_log" \
   && grep -Fq "G099" "$unk_strict_log"; then
  pass "unknown strict reports unknown-gate-id G099"
else
  fail "unknown strict missing unknown-gate-id G099 finding"
  sed -n '1,40p' "$unk_strict_log"
fi

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest gate-id-grep] OK"
  exit 0
else
  echo "[selftest gate-id-grep] FAIL — $failures assertion(s) failed"
  exit 1
fi
