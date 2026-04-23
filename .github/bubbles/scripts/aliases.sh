#!/usr/bin/env bash
# ────────────────────────────────────────────────────────────────────
# aliases.sh — Sunnyvale alias resolution
# ────────────────────────────────────────────────────────────────────
# Resolves Sunnyvale-themed aliases to canonical Bubbles agent names
# and workflow modes.
#
# Usage (source from another script):
#   source "$(dirname "${BASH_SOURCE[0]}")/aliases.sh"
#   resolve_agent_alias "worst-case-ontario"   → "bubbles.chaos"
#   resolve_mode_alias "bottle-kids"           → "stochastic-quality-sweep"
#
# Or use the lookup + quote:
#   sunnyvale_lookup "decent"
#   → prints: bubbles.status — "DEEEE-CENT!"
# ────────────────────────────────────────────────────────────────────

# ── Agent Aliases ───────────────────────────────────────────────────
# Maps sunnyvale <alias> → bubbles.<agent>

declare -A _AGENT_ALIASES=(
  [pull-the-strings]="bubbles.workflow"
  [under-the-light]="bubbles.grill"
  [private-dancer]="bubbles.grill"
  [worst-case-ontario]="bubbles.chaos"
  [i-am-the-liquor]="bubbles.audit"
  [shit-winds]="bubbles.audit"
  [by-the-book]="bubbles.audit"
  [get-two-birds-stoned]="bubbles.implement"
  [i-got-work-to-do]="bubbles.implement"
  [smokes-lets-go]="bubbles.setup"
  [know-what-im-sayin]="bubbles.docs"
  [somethings-fucky]="bubbles.validate"
  [mans-gotta-eat]="bubbles.validate"
  [way-she-goes]="bubbles.analyst"
  [peanut-butter-and-jam]="bubbles.gaps"
  [safety-always-off]="bubbles.security"
  [decent]="bubbles.status"
  [roll-camera]="bubbles.status"
  [greasy]="bubbles.harden"
  [pave-your-cave]="bubbles.harden"
  [supply-and-command]="bubbles.plan"
  [jim-needs-a-plan]="bubbles.plan"
  [water-under-the-fridge]="bubbles.simplify"
  [keep-the-park-online]="bubbles.devops"
  [have-a-good-one]="bubbles.handoff"
  [skid-row]="bubbles.cinematic-designer"
  [somethings-prowlin]="bubbles.regression"
  [the-super]="bubbles.super"
  [not-how-that-works]="bubbles.test"
  [lets-get-organized]="bubbles.design"
  [whats-going-on-here]="bubbles.clarify"
  [parts-unknown]="bubbles.code-review"
  [whole-show]="bubbles.system-review"
  [nice-kitty]="bubbles.bug"
  [just-fixes]="bubbles.stabilize"
  [used-to-be-a-vet]="bubbles.create-skill"
  [true]="bubbles.commands"
  [ill-do-whatever]="bubbles.iterate"
  [cant-just-slap]="bubbles.ux"
  [catch-me-up]="bubbles.recap"
)

declare -A _AGENT_QUOTES=(
  [pull-the-strings]="Bubbles is pulling all the strings, boys."
  [under-the-light]="Let's get it under the light and see if it survives."
  [private-dancer]="You want answers? Put it under the light."
  [worst-case-ontario]="Worst case Ontario, something breaks."
  [i-am-the-liquor]="I AM the liquor, Randy."
  [shit-winds]="The shit winds are coming."
  [by-the-book]="This is by the book now."
  [get-two-birds-stoned]="Get two birds stoned at once."
  [i-got-work-to-do]="I got work to do."
  [smokes-lets-go]="Smokes, let's go."
  [know-what-im-sayin]="Know what I'm sayin'?"
  [somethings-fucky]="Something's fucky."
  [mans-gotta-eat]="A man's gotta eat, Julian."
  [way-she-goes]="Way she goes, boys."
  [peanut-butter-and-jam]="BAAAAM! Peanut butter and JAAAAM!"
  [safety-always-off]="Safety... always off."
  [decent]="DEEEE-CENT!"
  [roll-camera]="(camera keeps rolling)"
  [greasy]="That's greasy, boys."
  [pave-your-cave]="Why don't you go pave your cave?"
  [supply-and-command]="It's supply and command, Julian."
  [jim-needs-a-plan]="Jim, you need a plan."
  [water-under-the-fridge]="It's all water under the fridge."
  [keep-the-park-online]="Get the rack humming and keep the park online."
  [have-a-good-one]="Here, take this. I gotta go."
  [skid-row]="I was in Skid Row!"
  [somethings-prowlin]="Something's prowlin' around in the code, boys."
  [the-super]="I'm the trailer park supervisor."
  [not-how-that-works]="Dad, that's not how that works."
  [lets-get-organized]="Let's get this organized before anybody breaks it."
  [whats-going-on-here]="What in the f— is going on here?"
  [parts-unknown]="From parts unknown!"
  [whole-show]="Orangie sees everything. He's not dead, he's just... reviewing."
  [nice-kitty]="That's a nice f***ing kitty right there."
  [just-fixes]="... (Bill spots the problem and points at it)"
  [used-to-be-a-vet]="I used to be a vet, you know. I got specialties."
  [true]="True."
  [ill-do-whatever]="I'll do whatever you need, Julian."
  [cant-just-slap]="You can't just slap things together and call it a home, Ricky."
  [catch-me-up]="So basically what happened was..."
)

# ── Agent alias notes (special behavior) ────────────────────────────
declare -A _AGENT_NOTES=(
  [i-am-the-liquor]="Use with --strict flag for maximum enforcement"
  [get-two-birds-stoned]="Runs bubbles.implement then bubbles.test in sequence"
)

# ── Workflow Mode Aliases ───────────────────────────────────────────
# Maps sunnyvale <mode-alias> → canonical workflow mode

declare -A _MODE_ALIASES=(
  [boys-plan]="value-first-e2e-batch"
  [full-send]="full-delivery"
  [clean-and-sober]="full-delivery"
  [no-loose-ends]="full-delivery"
  [keep-the-park-online]="devops-to-doc"
  [strip-it-down]="simplify-to-doc"
  [shit-storm]="chaos-hardening"
  [smash-and-grab]="bugfix-fastlane"
  [randy-put-a-shirt-on]="validate-only"
  [bottle-kids]="stochastic-quality-sweep"
  [conky-says]="harden-gaps-to-doc"
  [freedom-35]="product-to-delivery"
  [gnome-sayin]="docs-only"
  [quick-dirty]="test-to-doc"
  [shit-winds-coming]="harden-to-doc"
  [gut-feeling]="gaps-to-doc"
  [survival-of-the-fitness]="improve-existing"
  [same-lot-new-trailer]="redesign-existing"
  [i-toad-a-so]="reconcile-to-doc"
  [bill-fixes-it]="stabilize-to-doc"
  [open-and-shut]="audit-only"
  [just-watching]="validate-to-doc"
  [smokes-and-setup]="feature-bootstrap"
  [keep-going]="iterate"
  [resume-the-tape]="resume-only"
  [whats-the-big-idea]="product-discovery"
  [harden-up]="spec-scope-hardening"
  [we-broke-it]="chaos-to-doc"
)

declare -A _MODE_QUOTES=(
  [boys-plan]="Julian's got a plan. A good plan this time."
  [full-send]="Full send, boys. No half-measures."
  [clean-and-sober]="We're doing this clean and sober."
  [no-loose-ends]="No loose ends. All green or we keep going."
  [strip-it-down]="Cut the nonsense. Keep what actually works."
  [shit-storm]="We're in the eye of a shiticane, Randy."
  [smash-and-grab]="Get in, fix it, get out. Smash and grab."
  [randy-put-a-shirt-on]="Randy, put a shirt on!"
  [bottle-kids]="Bottle kids! Take cover!"
  [conky-says]="Conky says you've got issues."
  [freedom-35]="Freedom 35, boys! The whole deal."
  [gnome-sayin]="Know what I'm sayin'? Publish the truth."
  [quick-dirty]="Quick and dirty, boys."
  [shit-winds-coming]="The shit winds are coming, Randy. Harden up."
  [gut-feeling]="What are ya lookin' at my gut fer?"
  [survival-of-the-fitness]="Survival of the fitness, boys."
  [same-lot-new-trailer]="Same lot, boys. New trailer."
  [i-toad-a-so]="I toad a so. I f***ing toad a so."
  [bill-fixes-it]="... (Bill just shows up and fixes it)"
  [open-and-shut]="Open and shut case."
  [just-watching]="(camera crew just watches)"
  [smokes-and-setup]="Smokes, let's go. Set it up."
  [keep-going]="Keep going, boys. Don't stop."
  [resume-the-tape]="Roll that tape back, boys."
  [whats-the-big-idea]="What's the big idea here, Julian?"
  [harden-up]="Harden up, boys. This has gotta be tight."
  [we-broke-it]="We broke it. Now document what happened."
)

# ── Public API ──────────────────────────────────────────────────────

# Resolve an agent alias to canonical agent name
# Returns empty string if not found
resolve_agent_alias() {
  local alias="$1"
  echo "${_AGENT_ALIASES[$alias]:-}"
}

# Resolve a workflow mode alias to canonical mode name
# Returns empty string if not found
resolve_mode_alias() {
  local alias="$1"
  echo "${_MODE_ALIASES[$alias]:-}"
}

# Get the quote for an agent alias
agent_alias_quote() {
  local alias="$1"
  echo "${_AGENT_QUOTES[$alias]:-}"
}

# Get the quote for a mode alias
mode_alias_quote() {
  local alias="$1"
  echo "${_MODE_QUOTES[$alias]:-}"
}

# Full lookup: try agent first, then mode. Prints formatted result.
# Returns 0 if found, 1 if not found.
sunnyvale_lookup() {
  local alias="$1"
  local agent="${_AGENT_ALIASES[$alias]:-}"
  local mode="${_MODE_ALIASES[$alias]:-}"

  if [[ -n "$agent" ]]; then
    local quote="${_AGENT_QUOTES[$alias]:-}"
    local note="${_AGENT_NOTES[$alias]:-}"
    echo "🫧 $alias → $agent"
    [[ -n "$quote" ]] && echo "   \"$quote\""
    [[ -n "$note" ]] && echo "   Note: $note"
    return 0
  elif [[ -n "$mode" ]]; then
    local quote="${_MODE_QUOTES[$alias]:-}"
    echo "🫧 $alias → workflow mode: $mode"
    [[ -n "$quote" ]] && echo "   \"$quote\""
    return 0
  else
    return 1
  fi
}

# List all aliases in a formatted table
list_all_aliases() {
  echo ""
  echo "🫧 Sunnyvale Agent Aliases"
  echo "──────────────────────────────────────────────────────────────"
  printf "  %-28s %-30s %s\n" "ALIAS" "MAPS TO" "QUOTE"
  printf "  %-28s %-30s %s\n" "─────" "───────" "─────"
  for alias in $(echo "${!_AGENT_ALIASES[@]}" | tr ' ' '\n' | sort); do
    local agent="${_AGENT_ALIASES[$alias]}"
    local quote="${_AGENT_QUOTES[$alias]:-}"
    printf "  %-28s %-30s %s\n" "$alias" "$agent" "\"$quote\""
  done

  echo ""
  echo "🫧 Sunnyvale Workflow Mode Aliases"
  echo "──────────────────────────────────────────────────────────────"
  printf "  %-28s %-30s %s\n" "ALIAS" "MAPS TO" "QUOTE"
  printf "  %-28s %-30s %s\n" "─────" "───────" "─────"
  for alias in $(echo "${!_MODE_ALIASES[@]}" | tr ' ' '\n' | sort); do
    local mode="${_MODE_ALIASES[$alias]}"
    local quote="${_MODE_QUOTES[$alias]:-}"
    printf "  %-28s %-30s %s\n" "$alias" "$mode" "\"$quote\""
  done
  echo ""
}
