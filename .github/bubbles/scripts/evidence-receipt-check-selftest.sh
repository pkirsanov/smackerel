#!/usr/bin/env bash
# Hermetic selftest for evidence-receipt-check.sh + tool-log.sh inputClosure
# (IMP-100 Phase 2 / IMP-024 SCOPE-1 + SCOPE-2). macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK="$SCRIPT_DIR/evidence-receipt-check.sh"
TOOL_LOG="$SCRIPT_DIR/tool-log.sh"
GUARD="$SCRIPT_DIR/state-transition-guard.sh"
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

# T9: re-running the same evidence identity refreshes a stale receipt. The
# historical row remains append-only in the log but no longer blocks strict
# freshness once a newer current receipt records the new input hash.
refresh_log="$d/refresh.jsonl"
old_hash="$(sha "$d/src.txt")"
printf '{"ts":"2026-07-20T00:00:03Z","cwd":"%s","spec":"spec-a","scope":"scope-a","cmd":"run tests","inputClosure":[{"path":"src.txt","sha256":"%s"}]}\n' \
  "$d" "$old_hash" > "$refresh_log"
printf 'refreshed input\n' > "$d/src.txt"
printf '{"ts":"2026-07-20T00:00:04Z","cwd":"%s","spec":"spec-a","scope":"scope-a","cmd":"run tests","inputClosure":[{"path":"src.txt","sha256":"%s"}]}\n' \
  "$d" "$(sha "$d/src.txt")" >> "$refresh_log"
out="$(bash "$CHECK" --log "$refresh_log" --repo-root "$d" --strict)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" total)" -eq 2 && "$(field "$out" current)" -eq 1 && "$(field "$out" superseded)" -eq 1 && "$(field "$out" valid)" -eq 1 && "$(field "$out" stale)" -eq 0 ]]; then
  pass "T9 fresh rerun supersedes stale receipt with the same evidence identity"
else
  fail "T9 expected one current valid and one superseded receipt (rc=$rc, out=$out)"
fi

# T10: the same command in another scope is a distinct claim. A fresh scope-b
# run must not hide stale evidence still current for scope-a.
scope_log="$d/scope-isolation.jsonl"
printf '{"ts":"2026-07-20T00:00:05Z","cwd":"%s","spec":"spec-a","scope":"scope-a","cmd":"run tests","inputClosure":[{"path":"src.txt","sha256":"%s"}]}\n' \
  "$d" "$old_hash" > "$scope_log"
printf '{"ts":"2026-07-20T00:00:06Z","cwd":"%s","spec":"spec-a","scope":"scope-b","cmd":"run tests","inputClosure":[{"path":"src.txt","sha256":"%s"}]}\n' \
  "$d" "$(sha "$d/src.txt")" >> "$scope_log"
out="$(bash "$CHECK" --log "$scope_log" --repo-root "$d" --strict)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 && "$(field "$out" total)" -eq 2 && "$(field "$out" current)" -eq 2 && "$(field "$out" superseded)" -eq 0 && "$(field "$out" valid)" -eq 1 && "$(field "$out" stale)" -eq 1 ]]; then
  pass "T10 fresh receipt in another scope does not supersede stale evidence"
else
  fail "T10 expected scope-isolated valid=1 stale=1 (rc=$rc, out=$out)"
fi

# BUG-050 SCN-B050-002: once the transition has admitted a receipt, strict
# freshness remains fail-closed. The transition-local projection narrows the
# input set; it must not weaken this checker's verdict for a stale row inside it.
admitted_path="$d/admitted.txt"
printf 'captured input\n' > "$admitted_path"
admitted_hash="$(sha "$admitted_path")"
printf 'changed after capture\n' > "$admitted_path"
admitted_stale_log="$d/admitted-stale.jsonl"
printf '{"schemaVersion":3,"ts":"2026-09-02T08:00:00Z","sessionId":"bug050-stale","spec":"BUG-050","scope":"SCOPE-01","cmd":"bash focused-admitted-stale.sh","exitCode":0,"inputClosure":[{"path":"admitted.txt","sha256":"%s"}],"scenarioBinding":{"scenarioId":"SCN-B050-002","phase":"green","testIdentity":"BUG-050::admitted-stale","sourceRevision":"0000000000000000000000000000000000000001","negativeControl":"change the admitted input closure","claim":"admitted stale receipt blocks"}}\n' \
  "$admitted_hash" > "$admitted_stale_log"
out="$(bash "$CHECK" --log "$admitted_stale_log" --repo-root "$d" --strict)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 && "$(field "$out" current)" -eq 1 && "$(field "$out" stale)" -eq 1 ]] &&
  [[ "$(field "$out" 'staleReceipts[0].reason' 2>/dev/null || true)" == "input hash differs: admitted.txt" ]]; then
  pass "SCN-B050-002 admitted stale receipt remains blocking under strict freshness"
else
  fail "SCN-B050-002 expected one named admitted stale receipt (rc=$rc, out=$out)"
fi

# BUG-050 SCN-B050-005: RED proves historical ordering against its captured
# source. A transition-admitted view may retain that stale closure only when a
# later matching IMPLEMENT receipt exists. Ordinary full-log diagnostics remain
# strict, and the GREEN receipt still uses current bytes.
historical_red_log="$d/historical-red.jsonl"
historical_red_hash="$(sha "$admitted_path")"
printf 'post-red implementation bytes\n' > "$admitted_path"
current_hash="$(sha "$admitted_path")"
printf '{"schemaVersion":3,"ts":"2026-09-02T08:10:00Z","sessionId":"bug050-red","spec":"BUG-050","scope":"SCOPE-01","cmd":"bash focused-red.sh","exitCode":1,"inputClosure":[{"path":"admitted.txt","sha256":"%s"}],"scenarioBinding":{"scenarioId":"SCN-B050-005","phase":"red","testIdentity":"BUG-050::historical-red","sourceRevision":"0000000000000000000000000000000000000001","negativeControl":"restore current-byte equality for historical RED","claim":"historical RED remains ordered proof"}}\n' \
  "$historical_red_hash" > "$historical_red_log"
printf '{"schemaVersion":3,"ts":"2026-09-02T08:11:00Z","sessionId":"bug050-implement","spec":"BUG-050","scope":"SCOPE-01","cmd":"bash focused-implement.sh","exitCode":0,"inputClosure":[{"path":"admitted.txt","sha256":"%s"}],"scenarioBinding":{"scenarioId":"SCN-B050-005","phase":"implement","testIdentity":"BUG-050::historical-red","sourceRevision":"0000000000000000000000000000000000000002","negativeControl":"restore current-byte equality for historical RED","claim":"implementation follows historical RED"}}\n' \
  "$current_hash" >> "$historical_red_log"
printf '{"schemaVersion":3,"ts":"2026-09-02T08:12:00Z","sessionId":"bug050-green","spec":"BUG-050","scope":"SCOPE-01","cmd":"bash focused-green.sh","exitCode":0,"inputClosure":[{"path":"admitted.txt","sha256":"%s"}],"scenarioBinding":{"scenarioId":"SCN-B050-005","phase":"green","testIdentity":"BUG-050::historical-red","sourceRevision":"0000000000000000000000000000000000000002","negativeControl":"restore current-byte equality for historical RED","claim":"current GREEN remains current-byte compatible"}}\n' \
  "$current_hash" >> "$historical_red_log"
out="$(bash "$CHECK" --log "$historical_red_log" --repo-root "$d" --strict)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 && "$(field "$out" stale)" -eq 1 ]]; then
  pass "SCN-B050-005 ordinary full-log freshness still reports the stale RED closure"
else
  fail "SCN-B050-005 ordinary mode unexpectedly relaxed RED freshness (rc=$rc, out=$out)"
fi
out="$(bash "$CHECK" --log "$historical_red_log" --repo-root "$d" --transition-admitted --strict)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" historical)" -eq 1 && "$(field "$out" stale)" -eq 0 && "$(field "$out" valid)" -eq 2 ]]; then
  pass "SCN-B050-005 admitted historical RED survives current-byte drift after matching IMPLEMENT"
else
  fail "SCN-B050-005 admitted historical RED was not preserved (rc=$rc, out=$out)"
fi

if awk '
  /^# CHECK 43:/ { in_check_43 = 1 }
  in_check_43 && /c43_out=.*c43_checker/ && /--log "\$c43_admitted_log"/ && /--transition-admitted/ && /--strict/ {
    freshness_uses_admitted_projection = 1
  }
  in_check_43 && /c43_analysis=.*jq -rs/ { clone_analysis_started = 1 }
  in_check_43 && clone_analysis_started && /"\$c43_admitted_log"/ {
    clone_uses_admitted_projection = 1
  }
  /^# CHECKS 23-25/ { in_check_43 = 0 }
  END {
    exit !(freshness_uses_admitted_projection && clone_uses_admitted_projection)
  }
' "$GUARD"; then
  pass "SCN-B050-001 Check 43 shares transition-admitted receipts across freshness and clone consumers"
else
  fail "SCN-B050-001 Check 43 must pass one transition-admitted projection to freshness and clone consumers"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "evidence-receipt-check-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "evidence-receipt-check-selftest: all cases passed."
