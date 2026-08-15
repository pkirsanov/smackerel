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
  sess="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/host" bash "$ADAPTERS/vscode-copilot.sh" session 2>&1)"
  got_prompt="$(printf '%s' "$sess" | jq -r '.promptTokens // "none"' 2>/dev/null)"
  got_max="$(printf '%s' "$sess" | jq -r '.maxPromptTokens // "none"' 2>/dev/null)"
  got_n="$(printf '%s' "$sess" | jq -r '.requests // "none"' 2>/dev/null)"
  if [[ "$got_prompt" == "675600" && "$got_max" == "513145" && "$got_n" == "2" ]]; then
    ok "vscode-copilot totals real records without estimating"
  else
    bad "vscode-copilot totals records" "prompt=$got_prompt max=$got_max n=$got_n raw=$sess"
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
  sess="$(BUBBLES_USAGE_VSCODE_ROOT="$WORK/drift" bash "$ADAPTERS/vscode-copilot.sh" session 2>&1)"
  if printf '%s' "$st" | grep -q '"measured":false' && [[ "$(printf '%s' "$sess" | tr -d ' \n')" == "{}" ]]; then
    ok "host schema drift reports unmeasured, never a measured zero"
  else
    bad "schema drift is unmeasured" "status=$st session=$sess"
  fi
else
  ok "vscode-copilot schema-drift case SKIPPED (jq not installed)"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
