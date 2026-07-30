#!/usr/bin/env bash
# bubbles/scripts/test-impact-shadow-selftest.sh
#
# Selftest for test-impact-shadow.sh.
#
# The property under test is UNUSUAL: this script's correctness is mostly about
# what it MUST NOT do. A test-impact tool that quietly becomes authoritative is
# the exact failure the shadow design exists to prevent, so the assertions below
# are deliberately weighted toward refusal and degradation, not happy-path math.
#
# Every case is constructed to FAIL if the corresponding property regresses:
#   T1/T2  fail if degradation stops being honest (a missing index or a `none`
#          adapter must NEVER yield a subset).
#   T3     fails if the tool ever reports gating:true.
#   T4     fails if a bypass-shaped flag is accepted.
#   T5     fails if an exit code ever encodes "safe to skip".

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/test-impact-shadow.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

PASS=0
FAIL=0
ok()  { echo "  ✅ $1"; PASS=$((PASS + 1)); }
bad() { echo "  ❌ $1"; FAIL=$((FAIL + 1)); }

echo "test-impact-shadow-selftest"
echo ""

if ! command -v python3 >/dev/null 2>&1; then
  echo "  SKIP (python3 not installed)"
  exit 0
fi

# --- fixture: a repo whose adapter resolves to `none` ------------------------
FIX_NONE="$WORK/repo-none"
mkdir -p "$FIX_NONE/.github/bubbles/scripts" "$FIX_NONE/.github/bubbles/adapters/codeindex"
cp "$SCRIPT_DIR/codeindex-resolve.sh" "$FIX_NONE/.github/bubbles/scripts/" 2>/dev/null
cp "$SCRIPT_DIR/../adapters/codeindex/none.sh" "$FIX_NONE/.github/bubbles/adapters/codeindex/" 2>/dev/null
( cd "$FIX_NONE" && git init -q . && git config user.email t@t && git config user.name t &&
  echo hi > a.txt && git add -A && git commit -qm init && echo changed > a.txt ) >/dev/null 2>&1

# --- T1: `none` adapter must degrade, never produce a subset -----------------
out="$(bash "$TARGET" --repo-root "$FIX_NONE" 2>&1)"; rc=$?
if [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q 'DEGRADED'; then
  ok "T1 adapter=none degrades honestly (exit 0, no subset)"
else
  bad "T1 adapter=none did not degrade (rc=$rc): ${out:0:80}"
fi

# T1b: the degraded report must NOT claim a subset was derived.
if printf '%s' "$out" | grep -qi 'would skip'; then
  bad "T1b degraded output claimed a skip figure"
else
  ok "T1b degraded output claims no skip figure"
fi

# --- T2: a repo with no resolver at all must degrade -------------------------
FIX_BARE="$WORK/repo-bare"
mkdir -p "$FIX_BARE"
( cd "$FIX_BARE" && git init -q . ) >/dev/null 2>&1
out="$(bash "$TARGET" --repo-root "$FIX_BARE" 2>&1)"; rc=$?
if [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q 'DEGRADED'; then
  ok "T2 no resolver degrades honestly"
else
  bad "T2 no resolver did not degrade (rc=$rc)"
fi

# --- T3: ADVERSARIAL — JSON must never report gating:true --------------------
out="$(bash "$TARGET" --repo-root "$FIX_NONE" --json 2>&1)"
verdict="$(printf '%s' "$out" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    print("NOTJSON"); raise SystemExit
if d.get("gating") is True:
    print("GATING_TRUE")
elif d.get("mode") != "shadow":
    print("MODE_NOT_SHADOW")
else:
    print("OK")
' 2>/dev/null || echo NOTJSON)"
case "$verdict" in
  OK)      ok "T3 ADVERSARIAL: JSON is mode=shadow and never gating:true" ;;
  NOTJSON) bad "T3 JSON output was not parseable" ;;
  *)       bad "T3 ADVERSARIAL FAILED: $verdict" ;;
esac

# --- T4: ADVERSARIAL — bypass-shaped flags rejected --------------------------
bypass_ok=1
for flag in --skip --force --gate --authoritative --apply; do
  bash "$TARGET" "$flag" >/dev/null 2>&1
  [ $? -eq 2 ] || bypass_ok=0
done
[ "$bypass_ok" -eq 1 ] &&
  ok "T4 ADVERSARIAL: bypass/gating-shaped flags all exit 2" ||
  bad "T4 a bypass/gating-shaped flag was accepted"

# --- T5: ADVERSARIAL — no exit code may encode "safe to skip" ----------------
# The tool has exactly two documented exits: 0 (report produced) and 2 (usage).
# If a third ever appears it risks being read as a go/no-go signal.
bash "$TARGET" --repo-root "$FIX_NONE" >/dev/null 2>&1;  rc_none=$?
bash "$TARGET" --repo-root "$FIX_BARE" >/dev/null 2>&1;  rc_bare=$?
bash "$TARGET" --nonsense            >/dev/null 2>&1;    rc_bad=$?
if [ "$rc_none" -eq 0 ] && [ "$rc_bare" -eq 0 ] && [ "$rc_bad" -eq 2 ]; then
  ok "T5 ADVERSARIAL: exit codes are only 0 (report) / 2 (usage)"
else
  bad "T5 unexpected exit codes: none=$rc_none bare=$rc_bare bad=$rc_bad"
fi

# --- T6: the tool must not execute or mutate anything ------------------------
before="$(cd "$FIX_NONE" && git status --porcelain | sort)"
bash "$TARGET" --repo-root "$FIX_NONE" >/dev/null 2>&1
after="$(cd "$FIX_NONE" && git status --porcelain | sort)"
[ "$before" = "$after" ] &&
  ok "T6 leaves the working tree untouched" ||
  bad "T6 MUTATED the working tree"

# --- capability-awareness fixtures -------------------------------------------
# Verb support is NOT uniform across providers. A provider that cannot REPORT
# freshness is not the same as an index that IS stale, and a provider without
# `affected` has not "returned nothing". These fixtures pin that distinction:
# they FAIL if the consumer regresses to running the sync/re-check dance and
# then blaming the index for a limitation of the provider.
make_stub_repo() {
  local dir="$1"
  mkdir -p "$dir/.github/bubbles/scripts" "$dir/.github/bubbles/adapters/codeindex"
  cp "$SCRIPT_DIR/codeindex-resolve.sh" "$dir/.github/bubbles/scripts/"
  printf 'codeIndex:\n  adapter: codebase-memory\n' >"$dir/.github/bubbles-project.yaml"
  cat >"$dir/.github/bubbles/adapters/codeindex/codebase-memory.sh" <<'STUB'
#!/usr/bin/env bash
# Stub provider — selftest use only. Capability levels come from the
# environment so one stub covers several support matrices.
f="${STUB_FRESHNESS:-native}"
a="${STUB_AFFECTED:-native}"
case "$1" in
  capabilities) printf '{"symbols":"native","impact":"native","affected":"%s","routes":"native","indexed":"native","status":"native","freshness":"%s","sync":"native"}\n' "$a" "$f" ;;
  freshness)    [ "$f" = "unsupported" ] && exit 2; echo '{}' ;;
  sync|status)  echo '{}' ;;
  indexed)      echo '[{"path":"a.txt","nodeCount":3}]' ;;
  affected)     [ "$a" = "unsupported" ] && exit 1; echo '["t/thing_test.sh"]' ;;
  *)            echo '[]' ;;
esac
exit 0
STUB
  chmod +x "$dir/.github/bubbles/adapters/codeindex/codebase-memory.sh"
  ( cd "$dir" && git init -q . && git config user.email t@t && git config user.name t &&
    echo hi >a.txt && mkdir -p t && echo test >t/thing_test.sh &&
    git add -A && git commit -qm init && echo changed >a.txt ) >/dev/null 2>&1
}

FIX_CAP="$WORK/repo-cap"
make_stub_repo "$FIX_CAP"

# --- T7: an unsupported verb is a capability gap, not a malfunction ----------
out="$(STUB_FRESHNESS=unsupported STUB_AFFECTED=unsupported bash "$TARGET" --repo-root "$FIX_CAP" 2>&1)"; rc=$?
if [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -q "does not support 'affected'"; then
  ok "T7 unsupported 'affected' is named as a capability gap"
else
  bad "T7 did not name the capability gap (rc=$rc): ${out:0:120}"
fi

# T7b: ADVERSARIAL — it must not blame the INDEX for a PROVIDER limitation.
if printf '%s' "$out" | grep -qi 'still STALE'; then
  bad "T7b blamed the index ('still STALE') for an unreportable freshness verb"
else
  ok "T7b ADVERSARIAL: never blames the index for an unreportable freshness"
fi

# --- T8: unverifiable freshness is disclosed, never silently claimed ---------
out="$(STUB_FRESHNESS=unsupported STUB_AFFECTED=native bash "$TARGET" --repo-root "$FIX_CAP" --json 2>&1)"
verdict="$(printf '%s' "$out" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    print("NOTJSON"); raise SystemExit
print("OK" if d.get("degraded") is False and d.get("freshnessVerified") is False else "BAD:%s" % d)
' 2>/dev/null)"
if [ "$verdict" = "OK" ]; then
  ok "T8 discloses freshnessVerified:false instead of implying a check ran"
else
  bad "T8 freshness honesty regressed: $verdict"
fi

# T8b: ADVERSARIAL — a provider that CAN report freshness must still say true,
# otherwise T8 would pass by hardcoding false.
out="$(STUB_FRESHNESS=native STUB_AFFECTED=native bash "$TARGET" --repo-root "$FIX_CAP" --json 2>&1)"
verdict="$(printf '%s' "$out" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    print("NOTJSON"); raise SystemExit
print("OK" if d.get("freshnessVerified") is True else "BAD:%s" % d)
' 2>/dev/null)"
if [ "$verdict" = "OK" ]; then
  ok "T8b ADVERSARIAL: a real freshness check still reports verified:true"
else
  bad "T8b freshnessVerified is hardcoded, not measured: $verdict"
fi

echo ""
echo "  passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ] || exit 1
echo "✅ test-impact-shadow-selftest: all assertions passed"
exit 0
