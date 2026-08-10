#!/usr/bin/env bash
#
# autonomy-resolve-selftest.sh — proves the autonomy precedence chain.
#
# The load-bearing assertion is that each layer WINS OVER THE ONE BELOW IT, and
# that the two durable layers (env, project config) resolve without any per-run
# directive. That second property is the whole point of the resolver: it is what
# lets an interrupted session resume at the same posture without the operator
# re-asserting it in the prompt.

set -uo pipefail

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"
RESOLVE="$SCRIPT_DIR/autonomy-resolve.sh"

ISSUES=0
TMPS=()
trap '[[ ${#TMPS[@]} -gt 0 ]] && rm -rf "${TMPS[@]}" 2>/dev/null || true' EXIT INT TERM

pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  ISSUES=$((ISSUES + 1))
}

check() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label"
  else
    fail "$label"
    echo "  expected: $expected"
    echo "  actual:   $actual"
  fi
}

# A repo root with no bubbles-project.yaml, so the config layer is genuinely absent.
BARE_ROOT="$(mktemp -d)"
TMPS+=("$BARE_ROOT")

# A repo root whose project config pins the posture.
CONF_ROOT="$(mktemp -d)"
TMPS+=("$CONF_ROOT")
mkdir -p "$CONF_ROOT/.github"
printf 'autonomy: interactive\n' >"$CONF_ROOT/.github/bubbles-project.yaml"

resolve() { env -u BUBBLES_AUTONOMY bash "$RESOLVE" "$@" 2>/dev/null; }
posture() { resolve "$@" | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p'; }
layer() { resolve "$@" | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY_SOURCE=//p'; }

echo "Running autonomy-resolve selftest..."
echo "Scenario: the posture dial resolves from a documented precedence chain and survives a restart."

# --- Layer 4: framework default ---
check "Layer 4: no input resolves to the framework default" "full" "$(posture --repo-root "$BARE_ROOT")"
check "Layer 4: default names itself as the winning layer" "framework-default" "$(layer --repo-root "$BARE_ROOT")"

# --- Layer 3: project config beats default ---
if command -v yq >/dev/null 2>&1; then
  check "Layer 3: project config beats the framework default" "interactive" "$(posture --repo-root "$CONF_ROOT")"
  check "Layer 3: names project-config as the winning layer" "project-config" "$(layer --repo-root "$CONF_ROOT")"
else
  echo "SKIP: layer-3 config assertions require yq (absent); env/directive/default layers still asserted below."
fi

# --- Layer 2: env beats project config AND default ---
check "Layer 2: env beats the framework default" "guarded" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"
check "Layer 2: names env as the winning layer" "env" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY_SOURCE=//p')"
if command -v yq >/dev/null 2>&1; then
  check "Layer 2: env beats the project config" "guarded" \
    "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --repo-root "$CONF_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"
fi

# --- Layer 1: directive beats everything ---
check "Layer 1: --autonomy beats env" "interactive" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --autonomy interactive --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"
check "Layer 1: names directive as the winning layer" "directive" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --autonomy interactive --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY_SOURCE=//p')"
check "Layer 1: an autonomy: token inside --directive is extracted" "guarded" \
  "$(posture --directive 'mode: full-delivery autonomy:guarded specs: x' --repo-root "$BARE_ROOT")"
check "Layer 1: explicit --autonomy overrides the --directive token" "interactive" \
  "$(posture --autonomy interactive --directive 'autonomy:guarded' --repo-root "$BARE_ROOT")"

# --- Durability: the restart property the resolver exists to provide ---
check "Durability: env alone resolves with NO per-run directive" "interactive" \
  "$(BUBBLES_AUTONOMY=interactive bash "$RESOLVE" --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"

# --- Invalid values exit 1 ---
resolve --autonomy bogus --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid --autonomy exits 1" "1" "$?"
BUBBLES_AUTONOMY=bogus bash "$RESOLVE" --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid BUBBLES_AUTONOMY exits 1" "1" "$?"
resolve --directive 'autonomy:bogus' --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid autonomy token in --directive exits 1" "1" "$?"

if command -v yq >/dev/null 2>&1; then
  BAD_ROOT="$(mktemp -d)"
  TMPS+=("$BAD_ROOT")
  mkdir -p "$BAD_ROOT/.github"
  printf 'autonomy: bogus\n' >"$BAD_ROOT/.github/bubbles-project.yaml"
  resolve --repo-root "$BAD_ROOT" >/dev/null 2>&1
  check "Invalid autonomy in project config exits 1" "1" "$?"
fi

# --- Usage errors exit 2 ---
resolve --autonomy --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "--autonomy with no value exits 2" "2" "$?"
resolve --autonomy full --autonomy guarded --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Duplicate --autonomy exits 2" "2" "$?"
resolve --bogus-flag >/dev/null 2>&1
check "Unknown flag exits 2" "2" "$?"
resolve --format yaml --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid --format exits 2" "2" "$?"

# --- No bypass exists (the posture never waives verification) ---
for flag in --skip --force --ignore --no-verify; do
  resolve "$flag" >/dev/null 2>&1
  check "Bypass flag $flag is refused (exit 2)" "2" "$?"
done

# --- Help + json shape ---
resolve --help >/dev/null 2>&1
check "--help exits 0" "0" "$?"
help_out="$(bash "$RESOLVE" --help 2>/dev/null)"
if printf '%s' "$help_out" | grep -q '^Usage:'; then
  pass "--help prints a Usage banner"
else
  fail "--help should print a Usage banner"
fi
check "--format json emits a single-line record" '{"autonomy":"full","source":"framework-default"}' \
  "$(posture_json=$(resolve --format json --repo-root "$BARE_ROOT"); printf '%s' "$posture_json")"

# --- SCOPE-3: `unattended` is opt-in AND may not be unbounded ---
check "unattended resolves when the budget is bounded" "unattended" \
  "$(posture --autonomy unattended --session-budget bounded)"

resolve --autonomy unattended --session-budget bounded >/dev/null 2>&1
check "unattended with a bounded budget exits 0" "0" "$?"

resolve --autonomy unattended --session-budget unbounded >/dev/null 2>&1
check "unattended with an unbounded budget is refused (exit 3)" "3" "$?"

unbounded_err="$(env -u BUBBLES_AUTONOMY bash "$RESOLVE" --autonomy unattended --session-budget unbounded 2>&1 >/dev/null)"
if printf '%s' "$unbounded_err" | grep -q 'E039-UNATTENDED-UNBOUNDED'; then
  pass "The unbounded refusal names its code (E039-UNATTENDED-UNBOUNDED)"
else
  fail "The unbounded refusal must name E039-UNATTENDED-UNBOUNDED"
fi

# With no override the budget state is read from the session file.
BUDGET_ROOT="$(mktemp -d)"
TMPS+=("$BUDGET_ROOT")
mkdir -p "$BUDGET_ROOT/.specify/memory"
printf '%s\n' '{"sessionBudget":{"maxToolCalls":250,"maxWallClockMinutes":null}}' >"$BUDGET_ROOT/.specify/memory/bubbles.session.json"
resolve --autonomy unattended --repo-root "$BUDGET_ROOT" >/dev/null 2>&1
check "unattended is allowed when the session file carries a numeric cap" "0" "$?"

printf '%s\n' '{"sessionBudget":{"maxToolCalls":null,"maxWallClockMinutes":null}}' >"$BUDGET_ROOT/.specify/memory/bubbles.session.json"
resolve --autonomy unattended --repo-root "$BUDGET_ROOT" >/dev/null 2>&1
check "unattended is refused when every session cap is null" "3" "$?"

resolve --autonomy unattended --session-budget bogus >/dev/null 2>&1
check "--session-budget with a bogus value is a usage error (exit 2)" "2" "$?"

# Adding a fourth value must not shift the default, and must not constrain the others.
check "the framework default is still full, not unattended" "full" "$(posture --repo-root "$BARE_ROOT")"

resolve --autonomy full --session-budget unbounded --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "full is unaffected by an unbounded budget (exit 0)" "0" "$?"

echo
if [[ $ISSUES -eq 0 ]]; then
  echo "autonomy-resolve selftest passed."
  exit 0
fi
echo "autonomy-resolve selftest failed with $ISSUES issue(s)."
exit 1
