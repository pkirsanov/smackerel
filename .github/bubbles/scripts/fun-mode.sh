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

_fun_message_for_event() {
  case "$1" in
    gate_passed) printf '%s\n' "Decent!" ;;
    scope_ready) printf '%s\n' "Looks good, boys." ;;
    gate_failed) printf '%s\n' "Something's fucky." ;;
    fabrication_detected) printf '%s\n' "That's GREASY, boys. Real greasy." ;;
    missing_evidence) printf '%s\n' "Where's your evidence? Shit hawk circling." ;;
    all_gates_pass) printf '%s\n' "Way she goes, boys. Way she goes." ;;
    build_failed) printf '%s\n' "Holy f***, boys." ;;
    spec_completed) printf '%s\n' "DEEEE-CENT!" ;;
    warnings_found) printf '%s\n' "The shit winds are coming, Randy." ;;
    chaos_clean) printf '%s\n' "Worst case Ontario... nothing broke." ;;
    regression_clean) printf '%s\n' "Steve French is purrin'. No regressions, boys." ;;
    regression_found) printf '%s\n' "Something's prowlin' around in the code, boys." ;;
    spec_conflict) printf '%s\n' "Steve French found another cougar's territory. Two specs, same route." ;;
    recap) printf '%s\n' "So basically what happened was..." ;;
    security_vuln) printf '%s\n' "Safety... always ON." ;;
    docs_updated) printf '%s\n' "Know what I'm sayin'? It's published." ;;
    deferral_detected) printf '%s\n' "You can't just NOT do things, Corey!" ;;
    deferral_blocks_done) printf '%s\n' "That's NOT gettin' two birds stoned — that's just sayin' you WILL." ;;
    manipulation_detected) printf '%s\n' "That's GREASY, boys. You can't just cross things out and say they're done!" ;;
    format_bypass) printf '%s\n' "You can't just erase the checkboxes and call it a day, Ricky!" ;;
    invented_status) printf '%s\n' "'Deferred — Planned Improvement'?! That's not even a real thing, Julian!" ;;
    handoff_complete) printf '%s\n' "Have a good one, boys." ;;
    gap_found) printf '%s\n' "This is f***ed. BAAAAM!" ;;
    bug_located) printf '%s\n' "That's a nice f***ing kitty right there." ;;
    build_succeeds) printf '%s\n' "Knock knock. Who's there? A passing build." ;;
    milestone_reached) printf '%s\n' "Freedom 35, boys!" ;;
    guard_start) printf '%s\n' "Alright boys, here's what we're gonna do." ;;
    guard_blocked) printf '%s\n' "Boys, we're in the eye of a shiticane." ;;
    guard_clear) printf '%s\n' "Passed with flying carpets!" ;;
    lint_start) printf '%s\n' "Let's see if this thing's got its grade 10." ;;
    lint_clean) printf '%s\n' "Not bad. Not bad at all." ;;
    lint_dirty) printf '%s\n' "It's like a tropical earthquake blew through here." ;;
    dashboard_start) printf '%s\n' "Let me check on the boys." ;;
    scan_start) printf '%s\n' "I got work to do." ;;
    scan_clean) printf '%s\n' "It's not rocket appliances — and it's clean." ;;
    scan_dirty) printf '%s\n' "Gorilla see, gorilla do. Found copied garbage." ;;
    audit_start) printf '%s\n' "Mr. Lahey, I got a confession to make." ;;
    audit_clean) printf '%s\n' "The liquor figured it out, Randy." ;;
    audit_dirty) printf '%s\n' "I am the liquor, and the liquor says NO." ;;
  esac
}

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
  local msg
  msg="$(_fun_message_for_event "$event")"
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

# Pick a random element from the provided arguments
_fun_random_pick() {
  local len=$#
  local offset
  [[ "$len" -gt 0 ]] || return 0
  offset=$((RANDOM % len))
  shift "$offset"
  echo "$1"
}

# Print a random pass quip (no-op if fun mode is off)
fun_pass() {
  fun_mode_active || return 0
  echo "   🫧 $(_fun_random_pick "${_FUN_PASS_POOL[@]}")"
}

# Print a random fail quip (no-op if fun mode is off)
fun_fail() {
  fun_mode_active || return 0
  echo "   🫧 $(_fun_random_pick "${_FUN_FAIL_POOL[@]}")"
}

# Print a random warn quip (no-op if fun mode is off)
fun_warn() {
  fun_mode_active || return 0
  echo "   🫧 $(_fun_random_pick "${_FUN_WARN_POOL[@]}")"
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
