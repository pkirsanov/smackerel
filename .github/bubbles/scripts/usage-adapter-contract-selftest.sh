#!/usr/bin/env bash
# bubbles/scripts/usage-adapter-contract-selftest.sh
#
# Hermetic selftest for the host-usage adapter contract (IMP-039 SCOPE-2).
#
# The load-bearing property is the HONESTY half: with no adapter configured, and
# with a configured adapter that finds nothing, `status.measured` MUST be false
# and no token or credit figure may appear. A surface that reported 0 instead of
# `unmeasured` would let a reader mistake "we did not look" for "it was free" —
# which is the IMP-028 mistake this scope was written to avoid repeating.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADAPTERS="$FRAMEWORK_ROOT/adapters/usage"
RESOLVE="$SCRIPT_DIR/usage-resolve.sh"
NAME="usage-adapter-contract-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

expect_exact_usage_error() {
  local root="$1"
  local session_id="$2"
  local label="$3"
  local out rc
  out="$(BUBBLES_USAGE_VSCODE_ROOT="$root" bash "$ADAPTERS/vscode-copilot.sh" session "$session_id" 2>&1)"
  rc=$?
  if [[ "$rc" -eq 2 ]]; then
    ok "$label"
  else
    bad "$label" "rc=$rc out=$out"
  fi
}

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

have_jq=1
command -v jq >/dev/null 2>&1 || have_jq=0

# --- 1. the default adapter exists and is the neutral one --------------------
if [[ -f "$ADAPTERS/none.sh" ]]; then
  ok "none.sh (framework default) is present"
else
  bad "none.sh present" "$ADAPTERS/none.sh missing"
fi

# --- 2. neutral shapes per verb ----------------------------------------------
shape_ok=1
[[ "$(bash "$ADAPTERS/none.sh" requests)" == "[]" ]] || shape_ok=0
[[ "$(bash "$ADAPTERS/none.sh" session)" == "{}" ]] || shape_ok=0
[[ "$(bash "$ADAPTERS/none.sh" capabilities)" == "{}" ]] || shape_ok=0
if [[ "$shape_ok" -eq 1 ]]; then
  ok "none.sh returns the canonical neutral shape per verb"
else
  bad "none.sh neutral shapes" "requests/session/capabilities did not match []/{}/{}"
fi

# --- 3. ADVERSARIAL: 'no data' is distinguishable from 'measured zero' -------
status_none="$(bash "$ADAPTERS/none.sh" status)"
if printf '%s' "$status_none" | grep -q '"measured":false'; then
  ok "none.sh status reports measured:false, not a zero"
else
  bad "none.sh status measured:false" "$status_none"
fi

# --- 4. ADVERSARIAL: the neutral adapter emits NO token or credit figure ------
# If a zero ever leaks out of the default path, every consumer downstream is
# free to render it as a measurement.
if ! printf '%s' "$status_none$(bash "$ADAPTERS/none.sh" session)" |
  grep -qE '"(promptTokens|completionTokens|credits)"'; then
  ok "none.sh emits no token or credit field at all"
else
  bad "none.sh emits no token/credit field" "$status_none"
fi

# --- 5. every verb exits 0 on the default adapter ----------------------------
verb_rc=0
for v in requests session status capabilities; do
  bash "$ADAPTERS/none.sh" "$v" >/dev/null 2>&1 || verb_rc=1
done
if [[ "$verb_rc" -eq 0 ]]; then
  ok "every none.sh verb exits 0"
else
  bad "every none.sh verb exits 0" "a verb returned non-zero"
fi

# --- 6. an unknown verb is an error, not a silent empty ----------------------
bash "$ADAPTERS/none.sh" not-a-verb >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  ok "unknown verb is refused"
else
  bad "unknown verb is refused" "exited 0"
fi

# --- 7. resolver: no config resolves to none ---------------------------------
mkdir -p "$WORK/plain"
out="$(bash "$RESOLVE" --repo-root "$WORK/plain" --names-only 2>&1)"; rc=$?
if [[ "$rc" -eq 0 && "$out" == "adapter=none" ]]; then
  ok "resolver defaults to none with no project config"
else
  bad "resolver defaults to none" "rc=$rc out=$out"
fi

# --- 8. resolver: an explicit adapter resolves ------------------------------
mkdir -p "$WORK/configured/.github"
cat >"$WORK/configured/.github/bubbles-project.yaml" <<'CFG'
usage:
  adapter: vscode-copilot
CFG
out="$(bash "$RESOLVE" --repo-root "$WORK/configured" 2>&1)"; rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'adapter=vscode-copilot'; then
  ok "resolver reads usage.adapter from project config"
else
  bad "resolver reads usage.adapter" "rc=$rc out=$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 9. ADVERSARIAL: a typo fails loud, it does NOT degrade to none ----------
# Silent degradation would make a misconfiguration look like a deliberate
# opt-out, and the operator would never learn the measurement was off.
mkdir -p "$WORK/typo/.github"
cat >"$WORK/typo/.github/bubbles-project.yaml" <<'CFG'
usage:
  adapter: vscode-copilott
CFG
out="$(bash "$RESOLVE" --repo-root "$WORK/typo" 2>&1)"; rc=$?
if [[ "$rc" -eq 1 ]] && ! printf '%s' "$out" | grep -q 'adapter=none'; then
  ok "a misconfigured adapter fails loud instead of degrading to none"
else
  bad "misconfigured adapter fails loud" "rc=$rc out=$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 10. ADVERSARIAL: a path-traversal adapter value is rejected -------------
mkdir -p "$WORK/evil/.github"
cat >"$WORK/evil/.github/bubbles-project.yaml" <<'CFG'
usage:
  adapter: ../../../../bin/sh
CFG
bash "$RESOLVE" --repo-root "$WORK/evil" >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  ok "path-traversal adapter value is rejected"
else
  bad "path-traversal rejected" "resolver accepted it"
fi

# --- 11. reference adapter: absent artifact reports unmeasured ---------------
if [[ "$have_jq" -eq 1 ]]; then
  mkdir -p "$WORK/empty-root"
  st="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/empty-root" bash "$ADAPTERS/vscode-copilot.sh" status 2>&1)"
  if printf '%s' "$st" | grep -q '"measured":false'; then
    ok "vscode-copilot reports measured:false when the artifact is absent"
  else
    bad "vscode-copilot unmeasured on absent artifact" "$st"
  fi
else
  ok "vscode-copilot absent-artifact case SKIPPED (jq not installed)"
fi

# --- 12. reference adapter: normalizes a record in the documented shape ------
# Proves the normalization logic, NOT the host schema: the fixture is written to
# the documented field names. The host owns that schema and can change it, which
# is why status degrades to unmeasured rather than to zero.
if [[ "$have_jq" -eq 1 ]]; then
  mkdir -p "$WORK/host/ws-1/chatSessions"
  cat >"$WORK/host/ws-1/chatSessions/session-a.jsonl" <<'REC'
{"requestId":"r1","timestamp":"2026-08-11T00:00:00Z","modelId":"m-1","promptTokens":162455,"completionTokens":700,"copilotCredits":40.5}
{"requestId":"r2","timestamp":"2026-08-11T00:05:00Z","modelId":"m-1","promptTokens":513145,"completionTokens":900,"copilotCredits":201.545}
REC
  sess="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" session session-a 2>&1)"
  got_prompt="$(printf '%s' "$sess" | jq -r '.promptTokens // "none"' 2>/dev/null)"
  got_max="$(printf '%s' "$sess" | jq -r '.maxPromptTokens // "none"' 2>/dev/null)"
  got_n="$(printf '%s' "$sess" | jq -r '.requests // "none"' 2>/dev/null)"
  got_session="$(printf '%s' "$sess" | jq -r '.sessionId // "none"' 2>/dev/null)"
  got_match="$(printf '%s' "$sess" | jq -r '.identityMatch // "none"' 2>/dev/null)"
  got_artifacts="$(printf '%s' "$sess" | jq -r '.artifactCount // "none"' 2>/dev/null)"
  if [[ "$got_prompt" == "675600" && "$got_max" == "513145" && "$got_n" == "2" &&
    "$got_session" == "session-a" && "$got_match" == "exact" && "$got_artifacts" == "1" ]]; then
    ok "vscode-copilot totals one exact artifact and emits exact identity proof"
  else
    bad "vscode-copilot exact artifact proof" "prompt=$got_prompt max=$got_max n=$got_n session=$got_session match=$got_match artifacts=$got_artifacts raw=$sess"
  fi

  reqs="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" requests session-a 2>&1)"
  req_count="$(printf '%s' "$reqs" | jq 'length' 2>/dev/null)"
  req_prompt_sum="$(printf '%s' "$reqs" | jq 'map(.promptTokens) | add' 2>/dev/null)"
  if [[ "$req_count" == "2" && "$req_prompt_sum" == "675600" ]]; then
    ok "vscode-copilot requests returns only the exact artifact records"
  else
    bad "vscode-copilot exact requests projection" "count=$req_count promptSum=$req_prompt_sum raw=$reqs"
  fi

  st="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" status 2>&1)"
  if printf '%s' "$st" | grep -q '"measured":true'; then
    ok "vscode-copilot reports measured:true once records exist"
  else
    bad "vscode-copilot measured:true with records" "$st"
  fi
else
  ok "vscode-copilot normalization case SKIPPED (jq not installed)"
  ok "vscode-copilot measured:true case SKIPPED (jq not installed)"
fi

# --- 13. ADVERSARIAL: an artifact WITHOUT the usage fields is unmeasured -----
# The realistic drift: the host renames or drops the field. Reporting 0 there
# would silently claim a free session.
if [[ "$have_jq" -eq 1 ]]; then
  mkdir -p "$WORK/drift/ws-1/chatSessions"
  cat >"$WORK/drift/ws-1/chatSessions/session-b.jsonl" <<'REC'
{"requestId":"r1","timestamp":"2026-08-11T00:00:00Z","modelId":"m-1","tokensIn":162455}
REC
  st="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/drift" bash "$ADAPTERS/vscode-copilot.sh" status 2>&1)"
  sess="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/drift" bash "$ADAPTERS/vscode-copilot.sh" session session-b 2>&1)"
  sess_rc=$?
  if printf '%s' "$st" | grep -q '"measured":false' && [[ "$sess_rc" -eq 2 ]]; then
    ok "host schema drift is globally unmeasured and exact request-like input fails loud"
  else
    bad "schema drift preserves status honesty and exact-input integrity" "status=$st sessionRc=$sess_rc session=$sess"
  fi
else
  ok "vscode-copilot schema-drift case SKIPPED (jq not installed)"
fi

# --- 14. BUG-037: unscoped, prefix-only, and ambiguous selectors abstain -----
if [[ "$have_jq" -eq 1 ]]; then
  unscoped_session="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" session 2>&1)"
  unscoped_requests="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" requests 2>&1)"
  if [[ "$(printf '%s' "$unscoped_session" | tr -d ' \n')" == "{}" &&
    "$(printf '%s' "$unscoped_requests" | tr -d ' \n')" == "[]" ]]; then
    ok "BUG-037 unscoped requests and session verbs return neutral shapes"
  else
    bad "BUG-037 unscoped adapter calls abstain" "session=$unscoped_session requests=$unscoped_requests"
  fi

  prefix_session="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" session session 2>&1)"
  prefix_requests="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" requests session 2>&1)"
  if [[ "$(printf '%s' "$prefix_session" | tr -d ' \n')" == "{}" &&
    "$(printf '%s' "$prefix_requests" | tr -d ' \n')" == "[]" ]]; then
    ok "BUG-037 prefix-only selectors return neutral shapes"
  else
    bad "BUG-037 prefix-only selector abstains" "session=$prefix_session requests=$prefix_requests"
  fi

  mkdir -p "$WORK/ambiguous/ws-1/chatSessions" "$WORK/ambiguous/ws-2/chatSessions"
  cp "$WORK/host/ws-1/chatSessions/session-a.jsonl" "$WORK/ambiguous/ws-1/chatSessions/session-a.jsonl"
  cp "$WORK/host/ws-1/chatSessions/session-a.jsonl" "$WORK/ambiguous/ws-2/chatSessions/session-a.jsonl"
  ambiguous_session="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/ambiguous" bash "$ADAPTERS/vscode-copilot.sh" session session-a 2>&1)"
  ambiguous_requests="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/ambiguous" bash "$ADAPTERS/vscode-copilot.sh" requests session-a 2>&1)"
  if [[ "$(printf '%s' "$ambiguous_session" | tr -d ' \n')" == "{}" &&
    "$(printf '%s' "$ambiguous_requests" | tr -d ' \n')" == "[]" ]]; then
    ok "BUG-037 duplicate exact artifacts are ambiguous and return neutral shapes"
  else
    bad "BUG-037 duplicate exact artifacts abstain" "session=$ambiguous_session requests=$ambiguous_requests"
  fi

  newline_session=$'session-with-\n-newline'
  mkdir -p "$WORK/newline/ws-1/chatSessions"
  printf '%s\n' '{"requestId":"newline","promptTokens":17}' \
    > "$WORK/newline/ws-1/chatSessions/$newline_session.jsonl"
  newline_out="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/newline" bash "$ADAPTERS/vscode-copilot.sh" session "$newline_session" 2>&1)"
  newline_rc=$?
  if [[ "$newline_rc" -eq 0 ]] &&
    [[ "$(printf '%s' "$newline_out" | jq -r '.sessionId // empty' 2>/dev/null)" == "$newline_session" ]] &&
    [[ "$(printf '%s' "$newline_out" | jq -r '.promptTokens // empty' 2>/dev/null)" == "17" ]]; then
    ok "BUG-037 exact selection preserves newline filename bytes"
  else
    bad "BUG-037 exact selection preserves newline filename bytes" "rc=$newline_rc out=$newline_out"
  fi

  mkdir -p "$WORK/symlink-file/ws-1/chatSessions" "$WORK/symlink-target"
  printf '%s\n' '{"requestId":"outside","promptTokens":19}' > "$WORK/symlink-target/outside.jsonl"
  ln -s "$WORK/symlink-target/outside.jsonl" "$WORK/symlink-file/ws-1/chatSessions/session-link.jsonl"
  expect_exact_usage_error "$WORK/symlink-file" session-link \
    "BUG-037 exact symlink artifacts fail loud"

  mkdir -p "$WORK/symlink-root-target/ws-1/chatSessions"
  printf '%s\n' '{"requestId":"root-link","promptTokens":23}' \
    > "$WORK/symlink-root-target/ws-1/chatSessions/session-root-link.jsonl"
  ln -s "$WORK/symlink-root-target" "$WORK/symlink-root"
  expect_exact_usage_error "$WORK/symlink-root" session-root-link \
    "BUG-037 symlink usage roots fail loud"

  mkdir -p "$WORK/nonregular/ws-1/chatSessions/session-directory.jsonl"
  expect_exact_usage_error "$WORK/nonregular" session-directory \
    "BUG-037 non-regular exact artifacts fail loud"

  mkdir -p "$WORK/unreadable/ws-1/chatSessions"
  printf '%s\n' '{"requestId":"unreadable","promptTokens":29}' \
    > "$WORK/unreadable/ws-1/chatSessions/session-unreadable.jsonl"
  chmod 000 "$WORK/unreadable/ws-1/chatSessions"
  expect_exact_usage_error "$WORK/unreadable" session-unreadable \
    "BUG-037 unreadable traversal fails loud"
  chmod 700 "$WORK/unreadable/ws-1/chatSessions"

  mkdir -p "$WORK/unreadable-file/ws-1/chatSessions"
  printf '%s\n' '{"requestId":"unreadable-file","promptTokens":29}' \
    > "$WORK/unreadable-file/ws-1/chatSessions/session-unreadable-file.jsonl"
  chmod 000 "$WORK/unreadable-file/ws-1/chatSessions/session-unreadable-file.jsonl"
  expect_exact_usage_error "$WORK/unreadable-file" session-unreadable-file \
    "BUG-037 unreadable exact artifacts fail loud"
  chmod 600 "$WORK/unreadable-file/ws-1/chatSessions/session-unreadable-file.jsonl"

  mkdir -p "$WORK/containment-outside/chatSessions" "$WORK/containment"
  printf '%s\n' '{"requestId":"outside-parent","promptTokens":30}' \
    > "$WORK/containment-outside/chatSessions/session-outside-parent.jsonl"
  ln -s "$WORK/containment-outside" "$WORK/containment/ws-link"
  expect_exact_usage_error "$WORK/containment" session-outside-parent \
    "BUG-037 symlinked traversal parents cannot escape containment"

  replacement_dir="$WORK/replacement/ws-1/chatSessions"
  replacement_target="$replacement_dir/session-replaced.jsonl"
  replacement_next="$replacement_dir/session-replaced.next"
  replacement_bin="$WORK/replacement-bin"
  mkdir -p "$replacement_dir"
  printf '%s\n' '{"requestId":"version-a","promptTokens":43}' > "$replacement_target"
  printf '%s\n' '{"requestId":"version-b","promptTokens":47}' > "$replacement_next"
  mkdir -p "$replacement_bin"
  cat > "$replacement_bin/python3" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1##*/}" == "session-state-io.py" && "${2:-}" == "parse-usage" ]]; then
  exec "$USAGE_RACE_REAL_PYTHON" -c 'import importlib.util, os, sys; sys.dont_write_bytecode = True; helper_path = sys.argv[1]; target = os.environ["USAGE_RACE_TARGET"]; replacement = os.environ["USAGE_RACE_REPLACEMENT"]; state = {"triggered": False}; sys.addaudithook(lambda event, arguments: (state.__setitem__("triggered", True), os.replace(replacement, target)) if event == "open" and not state["triggered"] and arguments and isinstance(arguments[0], (str, bytes, os.PathLike)) and os.fsdecode(os.fspath(arguments[0])) == "session-replaced.jsonl" else None); spec = importlib.util.spec_from_file_location("session_state_io_race", helper_path); module = importlib.util.module_from_spec(spec); sys.modules[spec.name] = module; spec.loader.exec_module(module); raise SystemExit(module.main(sys.argv[2:]))' "$@"
fi
exec "$USAGE_RACE_REAL_PYTHON" "$@"
SH
  chmod 700 "$replacement_bin/python3"
  replacement_out="$(
    USAGE_RACE_REAL_PYTHON="$(command -v python3)"
    export USAGE_RACE_REAL_PYTHON
    export USAGE_RACE_TARGET="$replacement_target"
    export USAGE_RACE_REPLACEMENT="$replacement_next"
    export PATH="$replacement_bin:$PATH"
    BUBBLES_USAGE_VSCODE_ROOT="$WORK/replacement" \
      bash "$ADAPTERS/vscode-copilot.sh" session session-replaced 2>&1
  )"
  replacement_rc=$?
  if [[ "$replacement_rc" -eq 2 ]] && grep -Fq '"version-b"' "$replacement_target"; then
    ok "BUG-037 replacement during exact artifact traversal fails loud"
  else
    bad "BUG-037 replacement during exact artifact traversal fails loud" \
      "adapterRc=$replacement_rc out=$replacement_out"
  fi

  mkdir -p "$WORK/malformed/ws-1/chatSessions"
  cat >"$WORK/malformed/ws-1/chatSessions/session-malformed.jsonl" <<'REC'
{"requestId":"r1","promptTokens":10}
not-json
REC
  expect_exact_usage_error "$WORK/malformed" session-malformed \
    "BUG-037 malformed exact artifacts fail loud"

  mkdir -p "$WORK/mixed/ws-1/chatSessions"
  cat >"$WORK/mixed/ws-1/chatSessions/session-mixed.jsonl" <<'REC'
{"requestId":"valid","promptTokens":31}
{"requestId":"invalid","promptTokens":null}
REC
  expect_exact_usage_error "$WORK/mixed" session-mixed \
    "BUG-037 mixed valid and null prompt-token records fail loud"

  mkdir -p "$WORK/incomplete/ws-1/chatSessions"
  cat >"$WORK/incomplete/ws-1/chatSessions/session-incomplete.jsonl" <<'REC'
{"requestId":"valid","promptTokens":37}
{"requestId":"missing-prompt","completionTokens":5}
REC
  expect_exact_usage_error "$WORK/incomplete" session-incomplete \
    "BUG-037 incomplete request-like records fail loud"

  mkdir -p "$WORK/non-usage/ws-1/chatSessions"
  printf '%s\n' '{"requestId":"not-usage","timestamp":"2026-09-01T00:00:00Z","tokensIn":41}' \
    > "$WORK/non-usage/ws-1/chatSessions/session-non-usage.jsonl"
  non_usage_out="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/non-usage" bash "$ADAPTERS/vscode-copilot.sh" session session-non-usage 2>&1)"
  non_usage_rc=$?
  if [[ "$non_usage_rc" -eq 0 && "$(printf '%s' "$non_usage_out" | tr -d ' \n')" == "{}" ]]; then
    ok "BUG-037 exact artifacts with no request-like object remain neutral"
  else
    bad "BUG-037 exact artifacts with no request-like object remain neutral" "rc=$non_usage_rc out=$non_usage_out"
  fi

  invalid_token_case=0
  for invalid_token in '"42"' '1.5' '-1' 'true'; do
    invalid_token_case=$((invalid_token_case + 1))
    invalid_token_root="$WORK/invalid-token-$invalid_token_case"
    mkdir -p "$invalid_token_root/ws-1/chatSessions"
    printf '{"requestId":"invalid-%s","promptTokens":%s}\n' \
      "$invalid_token_case" "$invalid_token" \
      > "$invalid_token_root/ws-1/chatSessions/session-invalid-token.jsonl"
    expect_exact_usage_error "$invalid_token_root" session-invalid-token \
      "BUG-037 non-integer or negative prompt-token case $invalid_token_case fails loud"
  done
else
  ok "BUG-037 exact-selector cases SKIPPED (jq not installed)"
  ok "BUG-037 prefix-selector case SKIPPED (jq not installed)"
  ok "BUG-037 ambiguous-selector case SKIPPED (jq not installed)"
  ok "BUG-037 newline filename case SKIPPED (jq not installed)"
  ok "BUG-037 exact symlink case SKIPPED (jq not installed)"
  ok "BUG-037 symlink root case SKIPPED (jq not installed)"
  ok "BUG-037 non-regular exact artifact case SKIPPED (jq not installed)"
  ok "BUG-037 unreadable traversal case SKIPPED (jq not installed)"
  ok "BUG-037 unreadable exact artifact case SKIPPED (jq not installed)"
  ok "BUG-037 containment case SKIPPED (jq not installed)"
  ok "BUG-037 replacement race case SKIPPED (jq not installed)"
  ok "BUG-037 malformed exact-artifact case SKIPPED (jq not installed)"
  ok "BUG-037 mixed token case SKIPPED (jq not installed)"
  ok "BUG-037 incomplete token case SKIPPED (jq not installed)"
  ok "BUG-037 non-usage artifact case SKIPPED (jq not installed)"
  ok "BUG-037 string prompt-token case SKIPPED (jq not installed)"
  ok "BUG-037 fractional prompt-token case SKIPPED (jq not installed)"
  ok "BUG-037 negative prompt-token case SKIPPED (jq not installed)"
  ok "BUG-037 boolean prompt-token case SKIPPED (jq not installed)"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
