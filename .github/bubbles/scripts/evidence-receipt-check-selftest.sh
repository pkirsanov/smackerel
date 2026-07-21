#!/usr/bin/env bash
# Hermetic selftest for evidence-receipt-check.sh + tool-log.sh inputClosure
# (IMP-100 Phase 2 / IMP-024 SCOPE-1 + SCOPE-2). macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK="$SCRIPT_DIR/evidence-receipt-check.sh"
TOOL_LOG="$SCRIPT_DIR/tool-log.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "evidence-receipt-check-selftest: SKIP (jq not installed)"
  exit 0
fi

sha() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}
# field <json-output> <key> — extract an integer field from the summary JSON.
field() { printf '%s' "$1" | jq -r ".$2"; }

echo "Running evidence-receipt-check selftest..."

# ── Setup: a repo dir with an input file + a JSONL log referencing it (valid).
d="$TMP_ROOT/repo"
mkdir -p "$d"
printf 'hello world\n' > "$d/src.txt"
h="$(sha "$d/src.txt")"
log="$d/tool-calls.jsonl"
printf '{"ts":"2026-07-20T00:00:00Z","cmd":"run tests","inputClosure":[{"path":"src.txt","sha256":"%s"}]}\n' "$h" > "$log"

# T1: inputs unchanged → valid=1, stale=0, exit 0.
out="$(bash "$CHECK" --log "$log" --repo-root "$d")"
rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" valid)" -eq 1 && "$(field "$out" stale)" -eq 0 && "$(field "$out" withClosure)" -eq 1 ]]; then
  pass "T1 unchanged inputs → valid=1 stale=0 (exit 0)"
else
  fail "T1 expected valid=1 stale=0 (rc=$rc, out=$out)"
fi

# T2: an input file changed on disk → stale=1; --strict → exit 1.
printf 'hello CHANGED\n' > "$d/src.txt"
out="$(bash "$CHECK" --log "$log" --repo-root "$d")" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" stale)" -eq 1 && "$(field "$out" valid)" -eq 0 ]]; then
  pass "T2 changed input (hash differs) → stale=1 (exit 0 non-strict)"
else
  fail "T2 expected stale=1 (rc=$rc, out=$out)"
fi
bash "$CHECK" --log "$log" --repo-root "$d" --strict >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]]; then pass "T2b --strict with stale → exit 1"; else fail "T2b --strict should exit 1 (rc=$rc)"; fi

# T3: restore file (valid again) but name it in --changed → stale via targeted invalidation.
printf 'hello world\n' > "$d/src.txt" # restore original hash
out="$(bash "$CHECK" --log "$log" --repo-root "$d" --changed src.txt)"
if [[ "$(field "$out" stale)" -eq 1 ]]; then
  pass "T3 --changed names the input → stale=1 (targeted invalidation)"
else
  fail "T3 expected stale=1 via --changed (out=$out)"
fi

# T3b: an UNRELATED changed file invalidates nothing (the receipt's closure does not intersect).
out="$(bash "$CHECK" --log "$log" --repo-root "$d" --changed some/other/file.txt)"
if [[ "$(field "$out" stale)" -eq 0 && "$(field "$out" valid)" -eq 1 ]]; then
  pass "T3b unrelated --changed file → valid=1 stale=0 (no over-invalidation)"
else
  fail "T3b unrelated change should invalidate nothing (out=$out)"
fi

# T4: a receipt with NO inputClosure → unknown (conservative), not valid.
log2="$d/no-closure.jsonl"
printf '{"ts":"2026-07-20T00:00:01Z","cmd":"legacy run","stdoutHash":"abc"}\n' > "$log2"
out="$(bash "$CHECK" --log "$log2" --repo-root "$d")"
if [[ "$(field "$out" unknown)" -eq 1 && "$(field "$out" valid)" -eq 0 && "$(field "$out" withClosure)" -eq 0 ]]; then
  pass "T4 receipt without inputClosure → unknown=1 (conservative)"
else
  fail "T4 expected unknown=1 (out=$out)"
fi

# T5: missing --log → usage error.
bash "$CHECK" --repo-root "$d" >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 2 ]]; then pass "T5 missing --log → exit 2"; else fail "T5 expected exit 2 (rc=$rc)"; fi

# T6: log not found → runtime error.
bash "$CHECK" --log "$d/does-not-exist.jsonl" >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 2 ]]; then pass "T6 log not found → exit 2"; else fail "T6 expected exit 2 (rc=$rc)"; fi

# T7: integration — tool-log.sh actually records inputClosure when inputs are declared.
if command -v python3 >/dev/null 2>&1; then
  wd="$TMP_ROOT/tl"
  mkdir -p "$wd"
  printf 'input-content\n' > "$wd/in.txt"
  intlog="$wd/tool-calls.jsonl"
  ( cd "$wd" && BUBBLES_TOOL_LOG_FILE="$intlog" BUBBLES_TOOL_LOG_INPUTS="in.txt" BUBBLES_TOOL_LOG_QUIET=1 \
      bash "$TOOL_LOG" -- echo "ran" >/dev/null 2>&1 )
  if [[ -f "$intlog" ]] && jq -e '.inputClosure[0].path == "in.txt" and (.inputClosure[0].sha256 | length) == 64' "$intlog" >/dev/null 2>&1; then
    pass "T7 tool-log.sh records inputClosure with a 64-hex sha256"
  else
    fail "T7 tool-log.sh should record inputClosure (log=$(cat "$intlog" 2>/dev/null))"
  fi
  # And that receipt is VALID against the unchanged input.
  out="$(bash "$CHECK" --log "$intlog" --repo-root "$wd")"
  if [[ "$(field "$out" valid)" -eq 1 ]]; then
    pass "T7b tool-log receipt is valid against unchanged input"
  else
    fail "T7b tool-log receipt should be valid (out=$out)"
  fi
else
  pass "T7 SKIP (python3 unavailable) — tool-log integration"
  pass "T7b SKIP (python3 unavailable)"
fi

# T8: --strict with only valid receipts → exit 0.
printf '{"ts":"2026-07-20T00:00:02Z","cmd":"ok","inputClosure":[{"path":"src.txt","sha256":"%s"}]}\n' "$(sha "$d/src.txt")" > "$d/valid.jsonl"
bash "$CHECK" --log "$d/valid.jsonl" --repo-root "$d" --strict >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]]; then pass "T8 --strict all-valid → exit 0"; else fail "T8 expected exit 0 (rc=$rc)"; fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "evidence-receipt-check-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "evidence-receipt-check-selftest: all cases passed."
