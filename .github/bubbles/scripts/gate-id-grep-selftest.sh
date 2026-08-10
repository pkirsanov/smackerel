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
#                                   (G099 is in the framework range < G900
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
#   - G900+ references in the clean fixture do NOT trigger findings
#     under --strict (project-local custom-gate allowlist works)
#   - the scans still work when the only grep on PATH has neither PCRE (-P)
#     nor the GNU \b word boundary (i.e. stock macOS/BSD grep)
#   - a broken awk makes the gate REFUSE (exit 2) instead of reporting zero
#   - word-boundary and adjacency semantics hold at the edges
#     ("G1234" / "XG123" never match; "G024 G025" is not a duplicate)
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

# --- POSIX-tooling guarantee (adversarial) --------------------------------
#
# The whole point of the awk rewrite is that this gate runs on stock macOS/BSD
# userland, where grep has neither PCRE (-P) nor the GNU \b word boundary. Stage
# a grep that refuses both and run the DUPLICATE fixture through it: the finding
# must still be reported. Asserting on the duplicate fixture (not the clean one)
# is what makes this adversarial — a reintroduced GNU-grep dependency would
# produce an empty scan, which a clean-fixture "exit 0" would happily accept.
#
# This case runs FIRST on purpose: it needs no optional dependency, so it holds
# on every host.

nopcre_root="$TMPDIR/repo-nopcre"
seed_repo "$nopcre_root"
cat > "$nopcre_root/agents/dup-doc.md" <<'EOF'
# dup-doc

This line has a copy-paste regression: G028, G028 in a row.
EOF

stub_dir="$TMPDIR/stub-bin"
mkdir -p "$stub_dir"
real_grep="$(command -v grep)"
cat > "$stub_dir/grep" <<'EOF'
#!/usr/bin/env bash
# Stock BSD/macOS grep simulation: refuses PCRE and the GNU word boundary, the
# two GNU-only constructs this gate must never depend on again.
for a in "$@"; do
  case "$a" in
    -*P*) echo "grep: invalid option -- P" >&2; exit 2 ;;
  esac
  case "$a" in
    *'\b'*) echo "grep: no GNU word-boundary support" >&2; exit 2 ;;
  esac
done
exec "${SELFTEST_REAL_GREP:?SELFTEST_REAL_GREP not set}" "$@"
EOF
chmod +x "$stub_dir/grep"

set +e
nopcre_log="$TMPDIR/nopcre.log"
PATH="$stub_dir:$PATH" SELFTEST_REAL_GREP="$real_grep" \
  bash "$TARGET" --repo-root "$nopcre_root" >"$nopcre_log" 2>&1
nopcre_rc=$?
set -e

if [[ "$nopcre_rc" -eq 1 ]]; then
  pass "grep without -P/\\b still detects the duplicate (exit 1)"
else
  fail "grep without -P/\\b expected exit 1, got $nopcre_rc"
  cat "$nopcre_log"
fi

if grep -Fq "FINDING: duplicate-adjacent:" "$nopcre_log"; then
  pass "grep without -P/\\b still prints the duplicate-adjacent finding"
else
  fail "grep without -P/\\b lost the duplicate-adjacent finding (silent pass)"
  cat "$nopcre_log"
fi

# --- awk fail-loud guard (adversarial) ------------------------------------
#
# awk drives both scans. A missing or broken awk would emit zero rows, and the
# gate would SILENTLY PASS. It must refuse with exit 2 instead. Two shapes:
#   (1) awk absent outright -> the capability probe refuses
#   (2) awk that satisfies the probe but fails on the real scan -> the scan
#       guard refuses (this is the shape a naive `|| true` would swallow)

set +e
noawk_log="$TMPDIR/noawk.log"
BUBBLES_AWK="$TMPDIR/definitely-not-an-awk" \
  bash "$TARGET" --repo-root "$nopcre_root" >"$noawk_log" 2>&1
noawk_rc=$?
set -e

if [[ "$noawk_rc" -eq 2 ]]; then
  pass "missing awk fail-fasts with exit 2 (no silent pass)"
else
  fail "missing awk expected exit 2, got $noawk_rc"
  cat "$noawk_log"
fi

if grep -Fq "requires a working POSIX awk" "$noawk_log"; then
  pass "missing awk prints the awk capability guard message"
else
  fail "missing awk did not print the awk capability guard message"
  cat "$noawk_log"
fi

cat > "$stub_dir/awk-halfbroken" <<'EOF'
#!/usr/bin/env bash
# Satisfies the trivial stdin capability probe, fails on any file-operand scan.
for a in "$@"; do
  if [ -f "$a" ]; then
    echo "awk: simulated scan failure" >&2
    exit 3
  fi
done
cat >/dev/null
echo x
exit 0
EOF
chmod +x "$stub_dir/awk-halfbroken"

set +e
halfawk_log="$TMPDIR/halfawk.log"
BUBBLES_AWK="$stub_dir/awk-halfbroken" \
  bash "$TARGET" --repo-root "$nopcre_root" >"$halfawk_log" 2>&1
halfawk_rc=$?
set -e

if [[ "$halfawk_rc" -eq 2 ]]; then
  pass "awk failing mid-scan fail-fasts with exit 2 (no silent pass)"
else
  fail "awk failing mid-scan expected exit 2, got $halfawk_rc"
  cat "$halfawk_log"
fi

if grep -Fq "scan failed (awk returned non-zero)" "$halfawk_log"; then
  pass "awk failing mid-scan prints the scan-failure guard message"
else
  fail "awk failing mid-scan did not print the scan-failure guard message"
  cat "$halfawk_log"
fi

# --- Clean fixture --------------------------------------------------------

clean_root="$TMPDIR/repo-clean"
seed_repo "$clean_root"
cat > "$clean_root/agents/clean-doc.md" <<'EOF'
# clean-doc

Mentions canonical gates G028 and G044 in a normal sentence,
plus a project-local custom gate G900 that should always be allowed.
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
  pass "clean strict exits 0 (G900+ allowed)"
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
(G099 is in the framework range < G900 so it is NOT auto-allowed).
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
   && grep -Fq "G099" "$unk_strict_log"; then  # fixture token G099 is intentionally unknown
  pass "unknown strict reports unknown-gate-id G099"  # fixture: G099 is a synthetic unknown gate
else
  fail "unknown strict missing unknown-gate-id G099 finding"
  sed -n '1,40p' "$unk_strict_log"
fi

# --- Word-boundary and adjacency edges ------------------------------------
#
# These pin the semantics the awk scanner emulates without GNU \b. The fixture's
# canonical set is only G028/G044, so every other id here surfaces as an unknown
# under --strict — which is how "did scan 2 emit this token at all?" becomes
# observable. "G123" must NEVER appear, because the only two places it could
# come from are the over-long "G1234" and the prefixed "XG123".

edge_root="$TMPDIR/repo-edge"
seed_repo "$edge_root"
cat > "$edge_root/agents/edge-doc.md" <<'EOF'
DUPSPACE G024 G024 end
DUPCOMMA G024, G024 end
DISTINCT G024 G025 end
OVERLONG G1234 end
PREFIXED XG123 end
TRIPLE G026 G027 G029 end
DOTSEP G044. G044 end
EOF

set +e
edge_default_log="$TMPDIR/edge-default.log"
bash "$TARGET" --repo-root "$edge_root" >"$edge_default_log" 2>&1
edge_default_rc=$?
set -e

if [[ "$edge_default_rc" -eq 1 ]]; then
  pass "edge default exits 1 (duplicates present)"
else
  fail "edge default expected exit 1, got $edge_default_rc"
  cat "$edge_default_log"
fi

if grep -Fq "DUPSPACE" "$edge_default_log"; then
  pass "edge: 'G024 G024' is reported as duplicate-adjacent"
else
  fail "edge: 'G024 G024' was NOT reported as duplicate-adjacent"
  cat "$edge_default_log"
fi

if grep -Fq "DUPCOMMA" "$edge_default_log"; then
  pass "edge: 'G024, G024' is reported as duplicate-adjacent"
else
  fail "edge: 'G024, G024' was NOT reported as duplicate-adjacent"
  cat "$edge_default_log"
fi

if grep -Fq "DISTINCT" "$edge_default_log"; then
  fail "edge: 'G024 G025' was wrongly reported as duplicate-adjacent"
  cat "$edge_default_log"
else
  pass "edge: 'G024 G025' is NOT a duplicate"
fi

if grep -Fq "DOTSEP" "$edge_default_log"; then
  fail "edge: 'G044. G044' was wrongly reported (separator is not [ ,]+)"
  cat "$edge_default_log"
else
  pass "edge: 'G044. G044' is NOT a duplicate (separator must be spaces/commas)"
fi

if grep -Fq "duplicate-adjacent: 2" "$edge_default_log"; then
  pass "edge: exactly 2 duplicate-adjacent lines reported"
else
  fail "edge: expected exactly 2 duplicate-adjacent lines"
  cat "$edge_default_log"
fi

set +e
edge_strict_log="$TMPDIR/edge-strict.log"
bash "$TARGET" --repo-root "$edge_root" --strict >"$edge_strict_log" 2>&1
edge_strict_rc=$?
set -e

if [[ "$edge_strict_rc" -eq 1 ]]; then
  pass "edge strict exits 1"
else
  fail "edge strict expected exit 1, got $edge_strict_rc"
  cat "$edge_strict_log"
fi

if grep -Fq "G123" "$edge_strict_log"; then
  fail "edge: 'G1234'/'XG123' wrongly matched as G123 (word boundary broken)"
  cat "$edge_strict_log"
else
  pass "edge: 'G1234' and 'XG123' never match as G123"
fi

# `grep -o` semantics: one row per OCCURRENCE, so the TRIPLE line (fixture line
# 6) must contribute exactly three reference rows, not one.
triple_rows="$(grep -c 'edge-doc\.md:6:' "$edge_strict_log" || true)"
if [[ "$triple_rows" -eq 3 ]] \
   && grep -Fq "G026" "$edge_strict_log" \
   && grep -Fq "G027" "$edge_strict_log" \
   && grep -Fq "G029" "$edge_strict_log"; then
  pass "edge: a line with three ids yields exactly three reference rows"
else
  fail "edge: expected 3 reference rows from the TRIPLE line, got $triple_rows"
  cat "$edge_strict_log"
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
