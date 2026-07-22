#!/usr/bin/env bash
#
# Hermetic selftest for verify-payload-integrity.sh (IMP-101 SCOPE-8).
#
# Builds a throwaway install target + release manifest under mktemp, then proves
# the verifier: (1) passes a clean payload, (2) hard-fails a missing required
# managed file, (3) hard-fails a corrupted installed file, (4) ignores the
# separate source-only manifest section, (5) preserves agents-only omissions,
# (6) preserves unselected optional-skill omissions but requires opted-in ones,
# (7) treats a missing manifest as advisory, (8) errors on unknown profiles and
# bypass-shaped flags, and (9) resolves default paths. No repo dependency; no
# network; no persistent state.
#
# NOTE: this selftest intentionally uses shell redirection to WRITE its OWN
# throwaway fixtures under a mktemp dir when it RUNS. That is normal script
# behavior (the same pattern every Bubbles *-selftest.sh uses) and is unrelated
# to the agent terminal-discipline rule about authoring workspace files.
#
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY="$SCRIPT_DIR/verify-payload-integrity.sh"

pass_count=0
fail_count=0
pass() {
  printf 'PASS: %s\n' "$1"
  pass_count=$((pass_count + 1))
}
fail() {
  printf 'FAIL: %s\n' "$1"
  fail_count=$((fail_count + 1))
}

if command -v sha256sum >/dev/null 2>&1; then
  sha_of() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha_of() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo "verify-payload-integrity-selftest: SKIP (no sha256 tool available)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# ── Build a fake install target ─────────────────────────────────────
TARGET="$WORK/.github"
mkdir -p "$TARGET/bubbles/scripts" "$TARGET/bubbles/registry" "$TARGET/agents"
printf 'echo one\n' >"$TARGET/bubbles/scripts/one.sh"
printf 'echo two\n' >"$TARGET/bubbles/scripts/two.sh"
printf '# demo agent\n' >"$TARGET/agents/bubbles.demo.agent.md"
printf 'bubbles-optional-design optional\n' >"$TARGET/bubbles/registry/optional-skills.txt"

SHA_ONE="$(sha_of "$TARGET/bubbles/scripts/one.sh")"
SHA_TWO="$(sha_of "$TARGET/bubbles/scripts/two.sh")"
SHA_AGENT="$(sha_of "$TARGET/agents/bubbles.demo.agent.md")"

# Source-only files live in their own section and are intentionally not installed.
write_manifest() {
  {
    printf '{\n'
    printf '  "version": "test",\n'
    printf '  "gitSha": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",\n'
    printf '  "managedFileChecksums": [\n'
    printf '    {"path": "bubbles/scripts/one.sh", "sha256": "%s"},\n' "$SHA_ONE"
    printf '    {"path": "bubbles/scripts/two.sh", "sha256": "%s"},\n' "$SHA_TWO"
    printf '    {"path": "agents/bubbles.demo.agent.md", "sha256": "%s"}\n' "$SHA_AGENT"
    printf '  ],\n'
    printf '  "sourceOnlyFileChecksums": [\n'
    printf '    {"path": "tests/regression/source-only.sh", "sha256": "%s"}\n' "0000000000000000000000000000000000000000000000000000000000000000"
    printf '  ],\n'
    printf '  "docsDigest": "n/a"\n'
    printf '}\n'
  } >"$WORK/manifest.json"
}
write_manifest

# ── Case 1: clean payload verifies (exit 0) ─────────────────────────
if bash "$VERIFY" --target "$TARGET" --manifest "$WORK/manifest.json" --quiet; then
  pass "clean payload verifies (exit 0)"
else
  fail "clean payload should verify (exit 0)"
fi

# ── Case 2: source-only section is outside managed verification ─────
verify_out="$(bash "$VERIFY" --target "$TARGET" --manifest "$WORK/manifest.json" 2>&1 || true)"
if printf '%s' "$verify_out" | grep -q "3 required framework file(s) match"; then
  pass "3 managed files verified; separate source-only section ignored"
else
  fail "expected '3 required framework file(s) match', got: $verify_out"
fi

# ── Case 3: a missing required managed file hard-fails (exit 1) ─────
rm "$TARGET/bubbles/scripts/one.sh"
missing_rc=0
bash "$VERIFY" --target "$TARGET" --manifest "$WORK/manifest.json" --quiet >/dev/null 2>&1 || missing_rc=$?
if [[ "$missing_rc" -eq 1 ]]; then
  pass "missing required managed file hard-fails (exit 1)"
else
  fail "missing required managed file should exit 1, got $missing_rc"
fi
missing_err="$(bash "$VERIFY" --target "$TARGET" --manifest "$WORK/manifest.json" 2>&1 || true)"
if printf '%s' "$missing_err" | grep -q "bubbles/scripts/one.sh" &&
   printf '%s' "$missing_err" | grep -q "required managed file is missing"; then
  pass "missing-file failure names the required managed path"
else
  fail "missing-file failure should name bubbles/scripts/one.sh, got: $missing_err"
fi
printf 'echo one\n' >"$TARGET/bubbles/scripts/one.sh"

# ── Case 4: a corrupted installed file hard-fails (exit 1) ──────────
printf 'echo two CORRUPTED\n' >"$TARGET/bubbles/scripts/two.sh"
corrupt_rc=0
bash "$VERIFY" --target "$TARGET" --manifest "$WORK/manifest.json" --quiet >/dev/null 2>&1 || corrupt_rc=$?
if [[ "$corrupt_rc" -eq 1 ]]; then
  pass "corrupted installed file hard-fails (exit 1)"
else
  fail "corrupted installed file should exit 1, got $corrupt_rc"
fi
# Confirm the failure names the offending file on stderr.
corrupt_err="$(bash "$VERIFY" --target "$TARGET" --manifest "$WORK/manifest.json" 2>&1 || true)"
if printf '%s' "$corrupt_err" | grep -q "bubbles/scripts/two.sh"; then
  pass "failure output names the mismatched file"
else
  fail "failure output should name bubbles/scripts/two.sh, got: $corrupt_err"
fi
printf 'echo two\n' >"$TARGET/bubbles/scripts/two.sh" # restore

# ── Case 5: agents-only omissions are explicit and narrow ──────────
cat >"$WORK/profile-manifest.json" <<'EOF'
{
  "managedFileChecksums": [
    {"path": "instructions/bubbles-demo.instructions.md", "sha256": "0000000000000000000000000000000000000000000000000000000000000000"},
    {"path": "skills/bubbles-demo/SKILL.md", "sha256": "0000000000000000000000000000000000000000000000000000000000000000"}
  ]
}
EOF
if bash "$VERIFY" --target "$TARGET" --manifest "$WORK/profile-manifest.json" --install-profile agents-only --quiet; then
  pass "agents-only profile permits absent instructions and skills"
else
  fail "agents-only profile should permit absent instructions and skills"
fi
full_profile_rc=0
bash "$VERIFY" --target "$TARGET" --manifest "$WORK/profile-manifest.json" --quiet >/dev/null 2>&1 || full_profile_rc=$?
if [[ "$full_profile_rc" -eq 1 ]]; then
  pass "full profile still requires ordinary instructions and skills"
else
  fail "full profile should require ordinary instructions and skills, got $full_profile_rc"
fi

# ── Case 6: optional skill omission depends on explicit opt-in ──────
cat >"$WORK/optional-manifest.json" <<'EOF'
{
  "managedFileChecksums": [
    {"path": "skills/bubbles-optional-design/SKILL.md", "sha256": "0000000000000000000000000000000000000000000000000000000000000000"}
  ]
}
EOF
if bash "$VERIFY" --target "$TARGET" --manifest "$WORK/optional-manifest.json" --quiet; then
  pass "unselected optional skill may be absent"
else
  fail "unselected optional skill should be allowed to remain absent"
fi
cat >"$TARGET/bubbles-project.yaml" <<'EOF'
designLanguages:
  enabled:
    - optional
EOF
optional_opted_in_rc=0
bash "$VERIFY" --target "$TARGET" --manifest "$WORK/optional-manifest.json" --quiet >/dev/null 2>&1 || optional_opted_in_rc=$?
if [[ "$optional_opted_in_rc" -eq 1 ]]; then
  pass "opted-in optional skill becomes required"
else
  fail "opted-in optional skill should be required, got $optional_opted_in_rc"
fi
rm "$TARGET/bubbles-project.yaml"

# ── Case 7: missing manifest is advisory (exit 0) ──────────────────
if bash "$VERIFY" --target "$TARGET" --manifest "$WORK/does-not-exist.json" --quiet; then
  pass "missing manifest is advisory (exit 0)"
else
  fail "missing manifest should be advisory (exit 0)"
fi

# ── Case 8: unknown profile and bypass-shaped flag error (exit 2) ──
profile_rc=0
bash "$VERIFY" --install-profile partial >/dev/null 2>&1 || profile_rc=$?
if [[ "$profile_rc" -eq 2 ]]; then
  pass "unknown install profile errors (exit 2)"
else
  fail "unknown install profile should exit 2, got $profile_rc"
fi
bypass_rc=0
bash "$VERIFY" --skip-verification >/dev/null 2>&1 || bypass_rc=$?
if [[ "$bypass_rc" -eq 2 ]]; then
  pass "unknown/bypass flag errors (exit 2)"
else
  fail "unknown flag should exit 2, got $bypass_rc"
fi

# ── Case 9: default manifest path resolves under the target ────────
cp "$WORK/manifest.json" "$TARGET/bubbles/release-manifest.json"
if (cd "$WORK" && bash "$VERIFY" --target ".github" --quiet); then
  pass "default manifest path (<target>/bubbles/release-manifest.json) resolves"
else
  fail "default manifest path should resolve and verify"
fi

# ── Case 10: default target (.github) when run from install root ───
# Prove the verifier defaults TARGET_DIR to .github with no --target flag.
if (cd "$WORK" && bash "$VERIFY" --quiet); then
  pass "default target (.github) resolves with no --target flag"
else
  fail "default target (.github) should resolve and verify"
fi

echo "---"
echo "verify-payload-integrity-selftest: ${pass_count} passed, ${fail_count} failed"
[[ "$fail_count" -eq 0 ]]
