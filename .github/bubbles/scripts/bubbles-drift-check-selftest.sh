#!/usr/bin/env bash
# File: bubbles-drift-check-selftest.sh
#
# Hermetic selftest for bubbles-drift-check.sh. Plants a fixture install root with
# a release-manifest.json + managed files, then proves: a clean tree is IN-SYNC
# (exit 0); a tampered file is DRIFTED (exit 1); a removed file is MISSING
# (exit 1); an extra framework script is an ORPHAN (informational, not a failure);
# --format json is well-formed; a malformed manifest exits 2; and an absent
# optional (opt-in) skill is OPT-OUT (exit 0) when not enabled, MISSING (exit 1)
# when enabled in designLanguages.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DRIFT="$SCRIPT_DIR/bubbles-drift-check.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "bubbles-drift-check-selftest: SKIP (python3 not installed)"
  exit 0
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

sha() { python3 -c "import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],'rb').read()).hexdigest())" "$1"; }

# Build a fixture install root with two managed files + a manifest.
build_root() {
  local root="$1"
  mkdir -p "$root/bubbles/scripts" "$root/agents"
  printf '#!/usr/bin/env bash\necho a\n' >"$root/bubbles/scripts/alpha.sh"
  printf -- '---\nname: probe\n---\nbody\n' >"$root/agents/bubbles.probe.agent.md"
  local h1 h2
  h1="$(sha "$root/bubbles/scripts/alpha.sh")"
  h2="$(sha "$root/agents/bubbles.probe.agent.md")"
  cat >"$root/bubbles/release-manifest.json" <<EOF
{
  "schemaVersion": 1,
  "version": "9.9.9",
  "managedFileCount": 2,
  "managedFileChecksums": [
    { "path": "bubbles/scripts/alpha.sh", "sha256": "$h1" },
    { "path": "agents/bubbles.probe.agent.md", "sha256": "$h2" }
  ]
}
EOF
}

# --- Case 1: clean tree → IN-SYNC, exit 0 -------------------------------------
r1="$work/r1"
build_root "$r1"
set +e
out1="$(bash "$DRIFT" --root "$r1" 2>&1)"
c1=$?
set -e
if [[ "$c1" -eq 0 ]] && grep -q "IN-SYNC" <<<"$out1"; then
  pass "clean tree reports IN-SYNC and exits 0"
else
  fail "clean tree should be IN-SYNC exit 0 (got exit $c1)"
  echo "$out1"
fi

# --- Case 2: tampered file → DRIFTED, exit 1 ----------------------------------
r2="$work/r2"
build_root "$r2"
printf '#!/usr/bin/env bash\necho TAMPERED\n' >"$r2/bubbles/scripts/alpha.sh"
set +e
out2="$(bash "$DRIFT" --root "$r2" 2>&1)"
c2=$?
set -e
if [[ "$c2" -eq 1 ]] && grep -q "DRIFTED  bubbles/scripts/alpha.sh" <<<"$out2"; then
  pass "a tampered managed file is DRIFTED and exits 1"
else
  fail "a tampered file should be DRIFTED exit 1 (got exit $c2)"
  echo "$out2"
fi

# --- Case 3: removed file → MISSING, exit 1 -----------------------------------
r3="$work/r3"
build_root "$r3"
rm -f "$r3/agents/bubbles.probe.agent.md"
set +e
out3="$(bash "$DRIFT" --root "$r3" 2>&1)"
c3=$?
set -e
if [[ "$c3" -eq 1 ]] && grep -q "MISSING  agents/bubbles.probe.agent.md" <<<"$out3"; then
  pass "a removed managed file is MISSING and exits 1"
else
  fail "a removed file should be MISSING exit 1 (got exit $c3)"
  echo "$out3"
fi

# --- Case 4: extra framework script → ORPHAN (informational, still exit 0) ----
r4="$work/r4"
build_root "$r4"
printf '#!/usr/bin/env bash\necho orphan\n' >"$r4/bubbles/scripts/__orphan.sh"
set +e
out4="$(bash "$DRIFT" --root "$r4" 2>&1)"
c4=$?
set -e
if [[ "$c4" -eq 0 ]] && grep -q "ORPHAN   bubbles/scripts/__orphan.sh" <<<"$out4"; then
  pass "an extra framework script is reported ORPHAN without failing (exit 0)"
else
  fail "an orphan should be reported but not fail the check (got exit $c4)"
  echo "$out4"
fi

# --- Case 5: --format json is well-formed -------------------------------------
r5="$work/r5"
build_root "$r5"
printf '#!/usr/bin/env bash\necho TAMPERED\n' >"$r5/bubbles/scripts/alpha.sh"
set +e
out5="$(bash "$DRIFT" --root "$r5" --format json 2>&1)"
c5=$?
set -e
if [[ "$c5" -eq 1 ]] && python3 -c "import json,sys;d=json.loads(sys.stdin.read());assert d['status']=='drift';assert 'bubbles/scripts/alpha.sh' in d['drifted']" <<<"$out5" 2>/dev/null; then
  pass "--format json emits well-formed drift output and exits 1"
else
  fail "--format json should be well-formed drift JSON exit 1 (got exit $c5)"
  echo "$out5"
fi

# --- Case 6: malformed manifest → exit 2 --------------------------------------
r6="$work/r6"
mkdir -p "$r6/bubbles"
printf 'not json{' >"$r6/bubbles/release-manifest.json"
set +e
bash "$DRIFT" --root "$r6" >/dev/null 2>&1
c6=$?
set -e
[[ "$c6" -eq 2 ]] \
  && pass "a malformed manifest exits 2" \
  || fail "a malformed manifest should exit 2 (got exit $c6)"

# --- Case 7: no manifest → exit 0 (nothing to check) --------------------------
r7="$work/r7"
mkdir -p "$r7/bubbles"
set +e
bash "$DRIFT" --root "$r7" >/dev/null 2>&1
c7=$?
set -e
[[ "$c7" -eq 0 ]] \
  && pass "a repo with no manifest exits 0 (nothing to check)" \
  || fail "no manifest should exit 0 (got exit $c7)"

# Fixture: alpha.sh present + in-sync; an optional skill recorded in the manifest
# but NOT planted on disk; the optional-skills registry lists it.
build_optional_root() {
  local root="$1"
  mkdir -p "$root/bubbles/scripts" "$root/bubbles/registry"
  printf '#!/usr/bin/env bash\necho a\n' >"$root/bubbles/scripts/alpha.sh"
  printf 'bubbles-cinematic-design cinematic\n' >"$root/bubbles/registry/optional-skills.txt"
  local h1
  h1="$(sha "$root/bubbles/scripts/alpha.sh")"
  cat >"$root/bubbles/release-manifest.json" <<EOF
{
  "schemaVersion": 1,
  "version": "9.9.9",
  "managedFileCount": 2,
  "managedFileChecksums": [
    { "path": "bubbles/scripts/alpha.sh", "sha256": "$h1" },
    { "path": "skills/bubbles-cinematic-design/SKILL.md", "sha256": "0000000000000000000000000000000000000000000000000000000000000000" }
  ]
}
EOF
}

# --- Case 8: absent optional skill, NOT enabled → OPT-OUT, exit 0 -------------
r8="$work/r8"
build_optional_root "$r8"
set +e
out8="$(bash "$DRIFT" --root "$r8" 2>&1)"
c8=$?
set -e
if [[ "$c8" -eq 0 ]] \
  && grep -q "OPT-OUT  skills/bubbles-cinematic-design/SKILL.md" <<<"$out8" \
  && ! grep -q "MISSING  skills/bubbles-cinematic-design" <<<"$out8"; then
  pass "an absent optional skill not enabled in designLanguages is OPT-OUT (exit 0), not MISSING"
else
  fail "absent opted-out optional skill should be OPT-OUT exit 0 (got exit $c8)"
  echo "$out8"
fi

# --- Case 9: absent optional skill, ENABLED → MISSING, exit 1 -----------------
r9="$work/r9"
build_optional_root "$r9"
printf 'designLanguages:\n  enabled: [cinematic]\n  default: cinematic\n' >"$r9/bubbles-project.yaml"
set +e
out9="$(bash "$DRIFT" --root "$r9" 2>&1)"
c9=$?
set -e
if [[ "$c9" -eq 1 ]] && grep -q "MISSING  skills/bubbles-cinematic-design/SKILL.md" <<<"$out9"; then
  pass "an absent optional skill that IS enabled is MISSING and exits 1"
else
  fail "absent enabled optional skill should be MISSING exit 1 (got exit $c9)"
  echo "$out9"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[bubbles-drift-check-selftest] OK"
else
  echo "[bubbles-drift-check-selftest] $failures failed"
  exit 1
fi
