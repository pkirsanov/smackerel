#!/usr/bin/env bash
# Classify continuation-shaped input before target or work-type extraction.
set -euo pipefail

raw_input="$*"
normalized="$(printf '%s' "$raw_input" |
  tr '[:upper:]' '[:lower:]' |
  sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//; s/[[:space:]]+/ /g; s/[.!?]+$//')"

case "$normalized" in
  pick-next|next-on-the-board|pick\ the\ next\ priority*|start\ the\ next\ priority*|start\ new\ work*)
    printf '%s\n' 'NEW_WORK'
    ;;
  continue|continue\ *|resume|resume\ *|next|next\ please|keep\ going|keep\ going\ *|go\ on|go\ on\ *|proceed|proceed\ *|fix\ all\ found*|fix\ everything\ found*|address\ rest*|address\ the\ rest*|fix\ the\ rest*|resolve\ remaining\ findings*|handle\ remaining\ issues*)
    printf '%s\n' 'CONTINUE'
    ;;
  *)
    printf '%s\n' 'OTHER'
    ;;
esac