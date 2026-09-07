#!/usr/bin/env bash
# mutable-dispatch-caller-coverage-lint-selftest.sh — hermetic selftest.
#
# Builds throwaway trees under mktemp. Each red fixture proves a specific way
# a broker-or-gateway bypass could slip past the lint; each green fixture
# proves a legitimate shape is not falsely flagged.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/mutable-dispatch-caller-coverage-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "mutable-dispatch-caller-coverage-lint-selftest: lint not found: $LINT" >&2
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

# make_repo <dir> — a minimal broker + gateway, no other callers.
make_repo() {
  local d="$1"
  mkdir -p "$d/bubbles/adapters/dispatch" "$d/bubbles/scripts" "$d/bubbles/registry"
  cat >"$d/bubbles/adapters/dispatch/reference-broker.sh" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  dispatch) echo "would dispatch" ;;
  *) exit 2 ;;
esac
EOF
  cat >"$d/bubbles/scripts/mutable-dispatch-gateway.sh" <<'EOF'
#!/usr/bin/env bash
BROKER="$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh"
bash "$BROKER" dispatch --store-root "$X"
EOF
}

# --- GREEN: only the broker and the gateway exist ---------------------------
g1="$WORK/g1"
make_repo "$g1"
assert "green: broker + gateway only, no other caller" 0 "$g1"

# --- GREEN: a path merely NAMES the dispatch adapter directory -------------
# `adapters/dispatch/` is a directory segment shared by every dispatch
# adapter reference in the tree; it must never be mistaken for the verb.
g2="$WORK/g2"
make_repo "$g2"
cat >"$g2/bubbles/scripts/bypass-flag-scan.sh" <<'EOF'
#!/usr/bin/env bash
if grep -Eq -- '--(skip|force)' "$SCRIPT_DIR/../adapters/dispatch/none.sh" "$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh"; then
  exit 1
fi
EOF
assert "green: dispatch-adapter directory path is not a verb call" 0 "$g2"

# --- GREEN: a fixture-copying loop stages the broker file, never runs it ---
g3="$WORK/g3"
make_repo "$g3"
cat >"$g3/bubbles/scripts/wiring-fixture-selftest.sh" <<'EOF'
#!/usr/bin/env bash
for adapter_file in none.sh reference-broker.sh; do
  cp "$SCRIPT_DIR/../adapters/dispatch/$adapter_file" "$FIXTURE/bubbles/adapters/dispatch/$adapter_file"
done
EOF
assert "green: fixture-copy loop naming the broker file is not a call" 0 "$g3"

# --- GREEN: an allow-listed direct caller with a reason ---------------------
g4="$WORK/g4"
make_repo "$g4"
cat >"$g4/bubbles/scripts/broker-adversarial-selftest.sh" <<'EOF'
#!/usr/bin/env bash
bash "$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh" dispatch --store-root "$X"
EOF
cat >"$g4/bubbles/registry/broker-direct-caller-allowlist.txt" <<'EOF'
# adversarial test of the broker in isolation
bubbles/scripts/broker-adversarial-selftest.sh
EOF
assert "green: allow-listed direct caller with a reason" 0 "$g4"

# --- RED: a new script calls the broker's dispatch verb directly -----------
r1="$WORK/r1"
make_repo "$r1"
cat >"$r1/bubbles/scripts/evil-direct.sh" <<'EOF'
#!/usr/bin/env bash
bash "$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh" dispatch --store-root "$X"
EOF
assert "red: direct literal broker-dispatch bypass" 1 "$r1"

# --- RED: a new script resolves the broker path into a variable first ------
r2="$WORK/r2"
make_repo "$r2"
cat >"$r2/bubbles/scripts/evil-indirect.sh" <<'EOF'
#!/usr/bin/env bash
BROKER="$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh"
bash "$BROKER" dispatch --store-root "$X"
EOF
assert "red: indirect variable broker-dispatch bypass" 1 "$r2"

# --- RED: allow-list entry with no reason comment ---------------------------
r3="$WORK/r3"
make_repo "$r3"
cat >"$r3/bubbles/scripts/evil-direct.sh" <<'EOF'
#!/usr/bin/env bash
bash "$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh" dispatch --store-root "$X"
EOF
printf 'bubbles/scripts/evil-direct.sh\n' >"$r3/bubbles/registry/broker-direct-caller-allowlist.txt"
assert "red: unjustified allow-list entry" 1 "$r3"

# --- RED: allow-list entry naming a file that no longer exists --------------
r4="$WORK/r4"
make_repo "$r4"
cat >"$r4/bubbles/registry/broker-direct-caller-allowlist.txt" <<'EOF'
# long gone
bubbles/scripts/does-not-exist.sh
EOF
assert "red: stale allow-list entry" 1 "$r4"

echo ""
echo "mutable-dispatch-caller-coverage-lint selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All mutable-dispatch-caller-coverage-lint selftests passed."
exit 0
