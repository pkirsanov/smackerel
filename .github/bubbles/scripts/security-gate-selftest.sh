#!/usr/bin/env bash
# security-gate-selftest.sh — hermetic selftest for G034's enforcer.
#
# G034 previously had no enforcer at all, so every check here is proving a
# capability the framework did not have. Each red fixture plants exactly one
# class of violation.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE="$SCRIPT_DIR/security-gate.sh"

if [[ ! -f "$GATE" ]]; then
  echo "security-gate-selftest: gate not found: $GATE" >&2
  exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

pass_count=0
fail_count=0

assert() {
  local name="$1" expected="$2" repo="$3" actual=0 output=""
  output="$(bash "$GATE" --repo-root "$repo" 2>&1)" || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS  $name (exit $actual)"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  $name — expected exit $expected, got $actual"
    echo "      output: $output"
    fail_count=$((fail_count + 1))
  fi
}

make_repo() {
  local d="$1"
  mkdir -p "$d/bubbles/scripts" "$d/docs"
  printf '#!/usr/bin/env bash\necho hello\n' >"$d/bubbles/scripts/ordinary.sh"
  printf '# docs\n' >"$d/docs/readme.md"
}

# --- GREEN: a clean tree ----------------------------------------------------
g1="$WORK/g1"
make_repo "$g1"
assert "green: clean tree" 0 "$g1"

# The fixtures below need private-key MARKERS. Assembling them at runtime keeps
# the literal out of this file, so the repo's own secret scanner does not flag
# the test that exists to catch committed keys.
key_begin="-----BEGIN RSA PRIVATE"" KEY-----"
key_end="-----END RSA PRIVATE"" KEY-----"

# --- RED: committed private key --------------------------------------------
r1="$WORK/r1"
make_repo "$r1"
{
  echo "$key_begin"
  echo "MIIEowIBAAKCAQEAxxxx"
  echo "$key_end"
} >"$r1/deploy.key"
assert "red: committed private key" 1 "$r1"

# --- GREEN: a doc that DESCRIBES the key marker is not a key ---------------
g2="$WORK/g2"
make_repo "$g2"
printf 'Never commit a `%s` block.\n' "$key_begin" >"$g2/docs/policy.md"
assert "green: policy doc mentioning the marker" 0 "$g2"

# --- RED: inline credential in a script ------------------------------------
r2="$WORK/r2"
make_repo "$r2"
printf '#!/usr/bin/env bash\nAPI_TOKEN="ghp_realtokenvalue1234"\n' >"$r2/bubbles/scripts/deploy.sh"
assert "red: inline credential literal" 1 "$r2"

# --- GREEN: env indirection is the correct pattern -------------------------
g3="$WORK/g3"
make_repo "$g3"
printf '#!/usr/bin/env bash\nAPI_TOKEN="$UPSTREAM_TOKEN"\n' >"$g3/bubbles/scripts/deploy.sh"
assert "green: credential read from the environment" 0 "$g3"

# --- GREEN: a selftest fixture token is synthetic --------------------------
g4="$WORK/g4"
make_repo "$g4"
printf '#!/usr/bin/env bash\nTOKEN="selftest-token-123"\n' >"$g4/bubbles/scripts/thing-selftest.sh"
assert "green: synthetic token in a selftest fixture" 0 "$g4"

# --- RED: curl piped to shell inside a script ------------------------------
r3="$WORK/r3"
make_repo "$r3"
printf '#!/usr/bin/env bash\ncurl -fsSL https://example.com/x.sh | bash\n' >"$r3/bubbles/scripts/bootstrap.sh"
assert "red: silent curl-pipe-shell in a script" 1 "$r3"

# --- GREEN: printing the command is guidance, not execution ----------------
g5="$WORK/g5"
make_repo "$g5"
printf '#!/usr/bin/env bash\necho "Install: curl -fsSL https://example.com/i.sh | bash"\n' >"$g5/bubbles/scripts/help.sh"
assert "green: install one-liner printed for the operator" 0 "$g5"

# --- RED: eval on command substitution -------------------------------------
r4="$WORK/r4"
make_repo "$r4"
printf '#!/usr/bin/env bash\neval "$(cat /tmp/untrusted)"\n' >"$r4/bubbles/scripts/run.sh"
assert "red: eval on command substitution" 1 "$r4"

# --- RED: world-writable tracked file --------------------------------------
r5="$WORK/r5"
make_repo "$r5"
printf 'data\n' >"$r5/docs/open.txt"
chmod 666 "$r5/docs/open.txt"
assert "red: world-writable file" 1 "$r5"

echo ""
echo "security-gate selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All security-gate selftests passed."
exit 0
