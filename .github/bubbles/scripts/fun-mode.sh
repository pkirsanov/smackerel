#!/usr/bin/env bash
# ────────────────────────────────────────────────────────────────────
# fun-mode.sh — "It ain't rocket appliances."
# ────────────────────────────────────────────────────────────────────
# Sourced by all Bubbles governance scripts to add Sunnyvale-flavored
# commentary when BUBBLES_FUN_MODE=true.
#
# Usage (source from another script):
#   source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"
#
# Then call fun_pass, fun_fail, fun_warn after your normal output,
# or use the event-based fun_message <event> function.
#
# Environment:
#   BUBBLES_FUN_MODE=true   Enable fun messages (default: disabled)
# ────────────────────────────────────────────────────────────────────

_BUBBLES_FUN_MODE="${BUBBLES_FUN_MODE:-false}"

# ── Message catalog ─────────────────────────────────────────────────
# Keyed by event name. Each event has one canonical message.

declare -A _FUN_MESSAGES=(
  # Gate / check results
  [gate_passed]="Decent!"
  [scope_ready]="Looks good, boys."
  [gate_failed]="Something's fucky."
  [fabrication_detected]="That's GREASY, boys. Real greasy."
  [missing_evidence]="Where's your evidence? Shit hawk circling."
  [all_gates_pass]="Way she goes, boys. Way she goes."
  [build_failed]="Holy f***, boys."
  [spec_completed]="DEEEE-CENT!"
  [warnings_found]="The shit winds are coming, Randy."
  [chaos_clean]="Worst case Ontario... nothing broke."
  [regression_clean]="Steve French is purrin'. No regressions, boys."
  [regression_found]="Something's prowlin' around in the code, boys."
  [spec_conflict]="Steve French found another cougar's territory. Two specs, same route."
  [recap]="So basically what happened was..."
  [security_vuln]="Safety... always ON."
  [docs_updated]="Know what I'm sayin'? It's published."
  [deferral_detected]="You can't just NOT do things, Corey!"
  [deferral_blocks_done]="That's NOT gettin' two birds stoned — that's just sayin' you WILL."
  [manipulation_detected]="That's GREASY, boys. You can't just cross things out and say they're done!"
  [format_bypass]="You can't just erase the checkboxes and call it a day, Ricky!"
  [invented_status]="'Deferred — Planned Improvement'?! That's not even a real thing, Julian!"
  [handoff_complete]="Have a good one, boys."
  [gap_found]="This is f***ed. BAAAAM!"
  [bug_located]="That's a nice f***ing kitty right there."
  [build_succeeds]='Knock knock. Who'"'"'s there? A passing build.'
  [milestone_reached]="Freedom 35, boys!"

  # Script lifecycle
  [guard_start]="Alright boys, here's what we're gonna do."
  [guard_blocked]="Boys, we're in the eye of a shiticane."
  [guard_clear]="Passed with flying carpets!"
  [lint_start]="Let's see if this thing's got its grade 10."
  [lint_clean]="Not bad. Not bad at all."
  [lint_dirty]="It's like a tropical earthquake blew through here."
  [dashboard_start]="Let me check on the boys."
  [scan_start]="I got work to do."
  [scan_clean]="It's not rocket appliances — and it's clean."
  [scan_dirty]="Gorilla see, gorilla do. Found copied garbage."
  [audit_start]="Mr. Lahey, I got a confession to make."
  [audit_clean]="The liquor figured it out, Randy."
  [audit_dirty]="I am the liquor, and the liquor says NO."
)

# ── Public API ──────────────────────────────────────────────────────

# Check if fun mode is active
fun_mode_active() {
  [[ "$_BUBBLES_FUN_MODE" == "true" ]]
}

# Print a fun message for a named event (no-op if fun mode is off)
# Usage: fun_message <event_name>
fun_message() {
  fun_mode_active || return 0
  local event="$1"
  local msg="${_FUN_MESSAGES[$event]:-}"
  if [[ -n "$msg" ]]; then
    echo "   🫧 $msg"
  fi
}

# Convenience wrappers for pass/fail/warn with random quips
# These pick from pools of messages for variety.

_FUN_PASS_POOL=(
  "Decent!"
  "Looks good, boys."
  "Way she goes."
  "Not bad. Not bad at all."
  "Passed with flying carpets!"
)

_FUN_FAIL_POOL=(
  "Something's fucky."
  "Holy f***, boys."
  "Boys, we're in the eye of a shiticane."
  "The shit winds are coming, Randy."
)

_FUN_WARN_POOL=(
  "The shit winds are coming, Randy."
  "Worst case Ontario..."
  "That's a bit greasy, boys."
)

# Pick a random element from a bash array
_fun_random_pick() {
  local -n arr=$1
  local len=${#arr[@]}
  echo "${arr[$((RANDOM % len))]}"
}

# Print a random pass quip (no-op if fun mode is off)
fun_pass() {
  fun_mode_active || return 0
  echo "   🫧 $(_fun_random_pick _FUN_PASS_POOL)"
}

# Print a random fail quip (no-op if fun mode is off)
fun_fail() {
  fun_mode_active || return 0
  echo "   🫧 $(_fun_random_pick _FUN_FAIL_POOL)"
}

# Print a random warn quip (no-op if fun mode is off)
fun_warn() {
  fun_mode_active || return 0
  echo "   🫧 $(_fun_random_pick _FUN_WARN_POOL)"
}

# Print the fun mode banner at script start (no-op if fun mode is off)
fun_banner() {
  fun_mode_active || return 0
  echo "   🫧 ────────────────────────────────────────"
  echo "   🫧  BUBBLES FUN MODE: ON"
  echo "   🫧  \"It ain't rocket appliances.\""
  echo "   🫧 ────────────────────────────────────────"
}

# Print a fun summary line at script end (no-op if fun mode is off)
# Usage: fun_summary <pass|fail> [failures_count]
fun_summary() {
  fun_mode_active || return 0
  local result="${1:-pass}"
  local count="${2:-0}"

  if [[ "$result" == "pass" ]]; then
    fun_message all_gates_pass
  elif [[ "$count" -ge 5 ]]; then
    fun_message guard_blocked
  else
    fun_message gate_failed
  fi
}
