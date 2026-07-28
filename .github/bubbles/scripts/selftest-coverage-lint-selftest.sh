#!/usr/bin/env bash
# selftest-coverage-lint-selftest.sh — hermetic selftest.
#
# Builds throwaway trees under mktemp. Each red fixture proves a specific way
# the "every selftest runs" guarantee could silently break.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/selftest-coverage-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "selftest-coverage-lint-selftest: lint not found: $LINT" >&2
  exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

pass_count=0
fail_count=0

assert() {
  local name="$1" expected="$2" repo="$3" actual=0 output=""
  output="$(bash "$LINT" --repo-root "$repo" 2>&1)" || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS  $name (exit $actual)"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  $name — expected exit $expected, got $actual"
    echo "      output: $output"
    fail_count=$((fail_count + 1))
  fi
}

# make_repo <dir> [--no-sweep]
make_repo() {
  local d="$1" sweep="${2:-with-sweep}"
  mkdir -p "$d/bubbles/scripts" "$d/bubbles/registry"
  if [[ "$sweep" == "with-sweep" ]]; then
    cat >"$d/bubbles/scripts/framework-validate.sh" <<'EOF'
#!/usr/bin/env bash
run_check "Enumerated" bash "$SCRIPT_DIR/alpha-selftest.sh"
run_check "Discovered selftest: $selftest_name" bash "$selftest_path"
EOF
  else
    cat >"$d/bubbles/scripts/framework-validate.sh" <<'EOF'
#!/usr/bin/env bash
run_check "Enumerated" bash "$SCRIPT_DIR/alpha-selftest.sh"
EOF
  fi
  printf '#!/usr/bin/env bash\nexit 0\n' >"$d/bubbles/scripts/alpha-selftest.sh"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$d/bubbles/scripts/beta-selftest.sh"
  printf '# no entries\n' >"$d/bubbles/registry/selftest-denylist.txt"
}

# --- GREEN: sweep present, nothing denied -----------------------------------
g1="$WORK/g1"
make_repo "$g1"
assert "green: enumerated + discovered, sweep present" 0 "$g1"

# --- GREEN: a denied selftest that exists and carries a reason --------------
g2="$WORK/g2"
make_repo "$g2"
cat >"$g2/bubbles/registry/selftest-denylist.txt" <<'EOF'
# needs a live host session
beta-selftest.sh
EOF
assert "green: justified deny-list entry" 0 "$g2"

# --- GREEN: nested selftest that IS enumerated ------------------------------
g3="$WORK/g3"
make_repo "$g3"
mkdir -p "$g3/bubbles/scripts/hooks"
printf '#!/usr/bin/env bash\nexit 0\n' >"$g3/bubbles/scripts/hooks/nested-selftest.sh"
cat >>"$g3/bubbles/scripts/framework-validate.sh" <<'EOF'
run_check "Nested" bash "$SCRIPT_DIR/hooks/nested-selftest.sh"
EOF
assert "green: nested selftest named by an enumerated check" 0 "$g3"

# --- RED: the discovery sweep was removed -----------------------------------
r1="$WORK/r1"
make_repo "$r1" no-sweep
assert "red: discovery sweep removed" 1 "$r1"

# --- RED: deny-list names a file that does not exist ------------------------
r2="$WORK/r2"
make_repo "$r2"
cat >"$r2/bubbles/registry/selftest-denylist.txt" <<'EOF'
# this one was deleted long ago
gamma-selftest.sh
EOF
assert "red: stale deny-list entry" 1 "$r2"

# --- RED: deny-list entry with no stated reason -----------------------------
r3="$WORK/r3"
make_repo "$r3"
printf 'beta-selftest.sh\n' >"$r3/bubbles/registry/selftest-denylist.txt"
assert "red: unjustified deny-list entry" 1 "$r3"

# --- RED: nested selftest nothing runs --------------------------------------
r4="$WORK/r4"
make_repo "$r4"
mkdir -p "$r4/bubbles/scripts/hooks"
printf '#!/usr/bin/env bash\nexit 0\n' >"$r4/bubbles/scripts/hooks/orphan-selftest.sh"
assert "red: nested selftest outside the glob and unenumerated" 1 "$r4"

echo ""
echo "selftest-coverage-lint selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All selftest-coverage-lint selftests passed."
exit 0
