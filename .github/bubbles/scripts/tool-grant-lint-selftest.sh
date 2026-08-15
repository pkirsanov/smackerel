#!/usr/bin/env bash
# bubbles/scripts/tool-grant-lint-selftest.sh
#
# Hermetic selftest for tool-grant-lint.sh (IMP-039 SCOPE-5).
#
# Two properties are load-bearing and pull in opposite directions:
#   A1/A5 — an unjustified restricted grant IS reported (the lint has teeth).
#   A2/A3/A4/A6 — a justified or unrestricted grant is NOT reported (the lint is
#                 not a blanket flagger, which would be indistinguishable from
#                 noise and would get switched off).
# A lint that only satisfied one half would be useless in the opposite way.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/tool-grant-lint.sh"
NAME="tool-grant-lint-selftest"

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

make_root() {
  local root="$1"
  mkdir -p "$root/agents" "$root/bubbles/registry"

  cat >"$root/bubbles/registry/tool-grants.yaml" <<'REG'
version: 1
families: [ read, search, edit, agent, todo, web, execute, bubbles, playwright ]
restricted:
  playwright:
    reason: >-
      Live browser automation.
    justifiedByPhases: [ chaos ]
    justifiedByAgents: [ bubbles.chaos ]
  web:
    reason: >-
      Outbound fetch.
    justifiedByPhases: [ analyze ]
    justifiedByAgents: [ bubbles.analyst ]
unrestricted: [ read, search, edit, agent, todo, execute, bubbles ]
REG

  cat >"$root/bubbles/workflows.yaml" <<'WF'
phases:
  chaos:
    owner: bubbles.chaos
    requiredGates: [ G009 ]
  analyze:
    owner: bubbles.probe
    requiredGates: [ G032 ]
  implement:
    owner: bubbles.implement
    requiredGates: [ G002 ]
WF
}

write_agent() {
  local root="$1" name="$2" tools="$3"
  {
    printf -- '---\n'
    printf 'description: fixture agent\n'
    [[ -n "$tools" ]] && printf 'tools: [%s]\n' "$tools"
    printf -- '---\n\nBody.\n'
  } > "$root/agents/$name.agent.md"
}

run_lint() {
  set +e
  LINT_OUT="$(bash "$TARGET" --root "$1" "${@:2}" 2>&1)"
  LINT_RC=$?
  set -e
}

# --- A1. ADVERSARIAL: an unjustified restricted grant is reported ------------
R="$WORK/a1"; make_root "$R"
write_agent "$R" bubbles.router "read, search, todo, bubbles, playwright"
run_lint "$R"
if printf '%s' "$LINT_OUT" | grep -q 'bubbles.router grants restricted family "playwright"'; then
  ok "A1 an unjustified restricted grant is reported"
else
  bad "A1 unjustified grant reported" "rc=$LINT_RC out=$(printf '%s' "$LINT_OUT" | tr '\n' '|')"
fi

# --- A2. an agent named in justifiedByAgents is NOT reported -----------------
R="$WORK/a2"; make_root "$R"
write_agent "$R" bubbles.chaos "read, search, todo, bubbles, playwright"
run_lint "$R"
if [[ "$LINT_RC" -eq 0 ]] && ! printf '%s' "$LINT_OUT" | grep -q 'restricted family'; then
  ok "A2 an agent-justified grant is not reported"
else
  bad "A2 agent-justified grant not reported" "rc=$LINT_RC out=$(printf '%s' "$LINT_OUT" | tr '\n' '|')"
fi

# --- A3. an agent OWNING a justified phase is NOT reported -------------------
# bubbles.probe owns the `analyze` phase, which justifies `web`, but is not in
# justifiedByAgents. Phase ownership alone must be sufficient.
R="$WORK/a3"; make_root "$R"
write_agent "$R" bubbles.probe "read, search, todo, bubbles, web"
run_lint "$R"
if [[ "$LINT_RC" -eq 0 ]] && ! printf '%s' "$LINT_OUT" | grep -q 'restricted family'; then
  ok "A3 a phase-justified grant is not reported"
else
  bad "A3 phase-justified grant not reported" "rc=$LINT_RC out=$(printf '%s' "$LINT_OUT" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: unrestricted families are never reported ---------------
# Guards against the lint degenerating into "flags everything", which would be
# noise rather than signal.
R="$WORK/a4"; make_root "$R"
write_agent "$R" bubbles.router "read, search, edit, agent, todo, execute, bubbles"
run_lint "$R"
if [[ "$LINT_RC" -eq 0 ]] && ! printf '%s' "$LINT_OUT" | grep -q 'restricted family'; then
  ok "A4 unrestricted families are never reported"
else
  bad "A4 unrestricted families not reported" "rc=$LINT_RC out=$(printf '%s' "$LINT_OUT" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: advisory by default, blocking under --strict -----------
# The advisory default is the whole safety argument for this lint. If the
# default ever blocked, a wrong entry in the registry would break every repo.
R="$WORK/a5"; make_root "$R"
write_agent "$R" bubbles.router "read, search, todo, bubbles, web"
run_lint "$R"
advisory_rc="$LINT_RC"
run_lint "$R" --strict
strict_rc="$LINT_RC"
if [[ "$advisory_rc" -eq 0 && "$strict_rc" -eq 1 ]]; then
  ok "A5 advisory exits 0 and --strict exits 1 on the same finding"
else
  bad "A5 advisory/strict split" "advisory=$advisory_rc strict=$strict_rc (want 0 and 1)"
fi

# --- A6. an agent with no tools: line is skipped -----------------------------
# Omitting the key inherits the host default; it grants nothing explicitly, so
# there is no over-grant to report.
R="$WORK/a6"; make_root "$R"
write_agent "$R" bubbles.silent ""
run_lint "$R"
if [[ "$LINT_RC" -eq 0 ]] && printf '%s' "$LINT_OUT" | grep -q '0 agent(s)'; then
  ok "A6 an agent with no tools: line is skipped"
else
  bad "A6 no-tools agent skipped" "rc=$LINT_RC out=$(printf '%s' "$LINT_OUT" | tr '\n' '|')"
fi

# --- U1. a bypass-shaped flag is refused -------------------------------------
set +e
bypass_out="$(bash "$TARGET" --skip-grants 2>&1)"; bypass_rc=$?
set -e
if [[ "$bypass_rc" -eq 2 ]] && printf '%s' "$bypass_out" | grep -q 'bypass-shaped'; then
  ok "U1 a bypass-shaped flag is refused"
else
  bad "U1 bypass-shaped flag refused" "rc=$bypass_rc out=$(printf '%s' "$bypass_out" | tr '\n' '|')"
fi

# --- U2. a missing registry is a usage error, not a pass ---------------------
R="$WORK/u2"; mkdir -p "$R/agents"
set +e
bash "$TARGET" --root "$R" >/dev/null 2>&1; missing_rc=$?
set -e
if [[ "$missing_rc" -eq 2 ]]; then
  ok "U2 a missing registry exits 2 rather than reporting clean"
else
  bad "U2 missing registry exits 2" "rc=$missing_rc"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
