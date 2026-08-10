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

# --- Block detection -------------------------------------------------------
#
# The `deploy.key` fixture above is caught by the FILE-EXTENSION check, so it
# never exercised the block scanner. These fixtures use ordinary source
# extensions, which is the only way to reach that code path.
payload="MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDBd0Xm7Qb2ZpVn"

# --- RED: a real key block inside an ordinary source file ------------------
r6="$WORK/r6"
make_repo "$r6"
{
  echo "static KEY: &str = \"$key_begin"
  echo "$payload"
  echo "$key_end\";"
} >"$r6/bubbles/scripts/embedded.sh"
assert "red: key block in an ordinary source file" 1 "$r6"

# --- GREEN: asserting ON the markers is not carrying a key -----------------
#
# Two assertions naming BEGIN and END hold both markers and zero key bytes.
# Requiring only co-occurrence reported this as committed key material.
g6="$WORK/g6"
make_repo "$g6"
{
  echo "assert pem.startswith(b\"$key_begin\")"
  echo "assert pem.rstrip().endswith(b\"$key_end\")"
} >"$g6/docs/test_fixture_smoke.py"
assert "green: test asserting on key markers, no payload" 0 "$g6"

# --- GREEN: an explicitly acknowledged throwaway fixture key ---------------
g7="$WORK/g7"
make_repo "$g7"
{
  echo "const TEST_PEM: &str = concat!("
  echo "    \"$key_begin\", // gitleaks:allow throwaway unit-test key"
  echo "    \"$payload\","
  echo "    \"$key_end\");"
} >"$g7/docs/fixture.rs"
assert "green: annotated throwaway fixture key" 0 "$g7"

# --- RED: the SAME block without the annotation ----------------------------
#
# Mutation proof that the annotation is the discriminator. If this fixture
# were green the exemption would be leaking from the path or the extension,
# and a real committed key would ride through on it.
r7="$WORK/r7"
make_repo "$r7"
{
  echo "const TEST_PEM: &str = concat!("
  echo "    \"$key_begin\","
  echo "    \"$payload\","
  echo "    \"$key_end\");"
} >"$r7/docs/fixture.rs"
assert "red: same block with the annotation removed" 1 "$r7"

# --- RED: a key embedded in a source string literal, carrying \n escapes ---
#
# A PEM pasted into a Rust/JS string literal reads ...QChjVWJ\n\ per line, so
# its body is not a contiguous base64 run and a genuine committed key was
# NOT detected. This fixture fails unless the escape is normalised first.
r8="$WORK/r8"
make_repo "$r8"
{
  echo "const TEST_RSA_PEM: &str = \"$key_begin\\\\n\\\\"
  echo "$payload\\\\n\\\\"
  echo "$key_end\";"
} >"$r8/docs/embedded_pem.rs"
assert "red: key block embedded with escaped newlines" 1 "$r8"

# --- GREEN: annotation trailing an escape on the BEGIN line ----------------
#
# The real-world shape is `"BEGIN...\n", // gitleaks:allow`. Splitting the
# FILE on the escape moves the annotation onto its own line and silently
# revokes the operator's acknowledgement, so an acknowledged fixture key
# started failing. The annotation is matched on the ORIGINAL line.
g10="$WORK/g10"
make_repo "$g10"
{
  echo "const TEST_PEM: &str = concat!("
  echo "    \"$key_begin\\n\", // gitleaks:allow throwaway unit-test RSA key"
  echo "    \"$payload\\n\","
  echo "    \"$key_end\\n\");"
} >"$g10/docs/annotated_escaped.rs"
assert "green: annotation trailing an escape on the BEGIN line" 0 "$g10"

# --- GREEN: a prefix assertion with no closing marker ----------------------
#
# The file names BEGIN and never closes the block. Without requiring closure
# the scan ran to EOF and judged ordinary code below it as the payload — the
# real trigger was a banner comment of '=' characters, which is a contiguous
# base64 run because '=' is padding. A separator is not key material.
g9="$WORK/g9"
make_repo "$g9"
{
  echo "if not pem_bytes.startswith(b\"$key_begin\"):"
  echo "    pytest.fail(\"sidecar did not return a PEM PRIVATE KEY payload\")"
  echo "# ========================================================================"
} >"$g9/docs/prefix_only.py"
assert "green: prefix assertion with no closing marker" 0 "$g9"

# --- GREEN: a value composed at runtime is not a hardcoded credential ------
g8="$WORK/g8"
make_repo "$g8"
printf '#!/usr/bin/env bash\nRUN_TOKEN="depqual-$$-$(date +%%s)"\n' >"$g8/bubbles/scripts/qualify.sh"
assert "green: run identifier built from substitutions" 0 "$g8"

# --- GREEN: a character set is not a credential ----------------------------
g9="$WORK/g9"
make_repo "$g9"
printf "#!/usr/bin/env bash\nTOKEN_CHARS='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'\n" >"$g9/bubbles/scripts/render.sh" # gitleaks:allow
assert "green: alphabet a random token is drawn from" 0 "$g9"

# --- RED: a literal credential that merely CONTAINS a dollar sign ----------
#
# Guards the runtime-composition exclusion from widening into "any value with
# a `$` is fine".
r8="$WORK/r8"
make_repo "$r8"
printf '#!/usr/bin/env bash\nAPI_TOKEN="ghp_literal$value1234"\n' >"$r8/bubbles/scripts/deploy.sh"
assert "red: literal credential containing a bare dollar" 1 "$r8"
# --- GREEN: acknowledged findings in the non-key classes -------------------
#
# The gate already accepts an inline acknowledgement for a committed key
# block. Applying the same standard to the other classes is what lets a repo
# resolve a legitimate finding — an official installer, a hermetic fixture,
# a framework idiom — without patching the gate or switching it off.
g11="$WORK/g11"
make_repo "$g11"
{
  echo "#!/usr/bin/env bash"
  echo "TEST_DB_PASSWORD=\"ephemeral_fixture_pass\" # security-gate:allow container this script creates"
  echo "curl -sSf https://example.invalid/rustup.sh | sh # security-gate:allow official toolchain installer"
  echo "eval \$(compute_paths) # security-gate:allow values produced by a local function"
} >"$g11/bubbles/scripts/acknowledged.sh"
assert "green: acknowledged credential, installer and eval" 0 "$g11"

# --- RED: the SAME three lines without the acknowledgement -----------------
#
# Mutation proof that the annotation is what clears them. If this were green
# the exemption would be leaking from the path, and a real finding would ride
# through on it.
r10="$WORK/r10"
make_repo "$r10"
{
  echo "#!/usr/bin/env bash"
  echo "TEST_DB_PASSWORD=\"ephemeral_fixture_pass\""
  echo "curl -sSf https://example.invalid/rustup.sh | sh"
  echo "eval \$(compute_paths)"
} >"$r10/bubbles/scripts/unacknowledged.sh"
assert "red: same three lines with no acknowledgement" 1 "$r10"
echo ""
echo "security-gate selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All security-gate selftests passed."
exit 0
