#!/usr/bin/env bash
# shellcheck disable=SC2016 # Patterns intentionally contain literal Markdown backticks.
#
# docs-wording-advisory.sh — exact documentation wording, reported not enforced
# (IMP-049 / SCOPE-6 / COST-1).
#
# WHY THIS EXISTS
# ---------------
# Several blocking selftests asserted a literal English sentence, a generated
# Markdown table row, or generated HTML markup. Rewording a sentence or
# regenerating a card therefore broke a release gate, which taxed exactly the
# documentation edits the framework most wants to encourage.
#
# Those assertions were NOT deleted. The owning selftest now asserts the
# STRUCTURE the contract actually depends on -- a mode key, a CLI path, a
# heading id, the surface tokens a sentence enumerates -- and the exact wording
# moved here, where drift is REPORTED and never blocks.
#
# The split rule, applied per assertion:
#   moved here      the subject is presentation: generated markup, a rendered
#                   table row, sentence order/punctuation/conjunctions
#   left blocking   the subject is an interface: a gate id, a script name, a
#                   mode key, a flag, a schema field, an exit code, a status
#                   token, a CLI path, a frontmatter field
#
# This lint is ADVISORY BY CONSTRUCTION: it always exits 0. A drifted line is a
# prompt to re-check the doc or to update this table, never a failed build. If
# a claim here ever becomes a real interface, move it back to its selftest.
#
# Usage:
#   docs-wording-advisory.sh [--repo-root <path>] [--quiet]
#
# Exit codes:
#   0  always (advisory)
#   2  usage error

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
LABEL="docs-wording-advisory"
quiet=0

usage() {
  cat <<'EOF'
docs-wording-advisory.sh — exact documentation wording, reported not enforced

Usage:
  bash bubbles/scripts/docs-wording-advisory.sh [--repo-root <path>] [--quiet]

Exit: 0 always (advisory) - 2 usage error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "$LABEL: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Entries are consecutive triples: <path relative to repo root> <ERE pattern>
# <origin selftest> <claim>. The pattern is the VERBATIM expression the blocking
# selftest used before IMP-049 SCOPE-6, so nothing about the wording claim was
# weakened in transit.
CLAIMS=(
  # --- generated artifacts: rendered by generate-cheatsheet.sh from
  # bubbles/cheatsheet/modes.json, whose freshness that generator already owns.
  'docs/its-not-rocket-appliances.html'
  'wf-name">full-delivery<'
  'workflow-surface-selftest.sh'
  'HTML cheat sheet renders the full-delivery workflow card'

  'docs/CHEATSHEET.md'
  '\| `full-delivery` \| full-send \|'
  'workflow-surface-selftest.sh'
  'Cheatsheet renders the full-delivery/full-send alias row'

  # --- presentation copy in the visual cheat sheet
  'docs/its-not-rocket-appliances.html'
  'Lease the lot|Same stack, same lease|Runtime doctor'
  'workflow-surface-selftest.sh'
  'HTML cheat sheet carries the runtime TPB vocabulary'

  # --- exact heading title (the phase id itself stays blocking)
  'agents/bubbles.workflow.agent.md'
  'Phase 0\.95: Full-Delivery Convergence Loop'
  'workflow-surface-selftest.sh'
  'Workflow agent titles Phase 0.95 the Full-Delivery Convergence Loop'

  # --- exact English sentences (the enumerated surfaces stay blocking)
  'docs/recipes/ask-the-super-first.md'
  'runtime lease conflicts|reuse the validation stack if it is compatible'
  'workflow-surface-selftest.sh'
  'Super recipe phrases the runtime coordination guidance'

  'prompts/bubbles.super.prompt.md'
  'framework validation, release hygiene, run-state and event diagnostics, repo-readiness'
  'super-surface-selftest.sh'
  'Super prompt phrases its framework-ops scope'

  'prompts/bubbles.super.prompt.md'
  'agents, workflow modes, recipes, skills, instructions, CLI commands, run-state, framework events, and risk classes'
  'super-surface-selftest.sh'
  'Super prompt phrases the live-surface discovery list'

  'docs/guides/AGENT_MANUAL.md'
  'recipe, skill, instruction, risk, and runtime surfaces'
  'super-surface-selftest.sh'
  'Agent manual phrases the super discovery breadth'

  # --- Markdown table rows / list glue (the CLI paths stay blocking)
  'agents/bubbles.super.agent.md'
  'Source framework repo \| `bash bubbles/scripts/cli\.sh \.\.\.`'
  'super-surface-selftest.sh'
  'Super agent renders the source-repo CLI table row'

  'agents/bubbles.super.agent.md'
  'Downstream installed repo \| `bash \.github/bubbles/scripts/cli\.sh \.\.\.`'
  'super-surface-selftest.sh'
  'Super agent renders the downstream CLI table row'

  'docs/recipes/ask-the-super-first.md'
  'source framework repo: `bash bubbles/scripts/cli\.sh \.\.\.`'
  'super-surface-selftest.sh'
  'Super recipe renders the source-repo CLI bullet'

  'docs/recipes/ask-the-super-first.md'
  'downstream installed repo: `bash \.github/bubbles/scripts/cli\.sh \.\.\.`'
  'super-surface-selftest.sh'
  'Super recipe renders the downstream CLI bullet'

  'README.md'
  'returns a completion recap and an unstarted next-priority candidate'
  'continuation-routing-selftest.sh'
  'README phrases the completed-state boundary'

  'docs/recipes/resume-work.md'
  'tries to resume the active workflow context first'
  'continuation-routing-selftest.sh'
  'Resume recipe phrases active-workflow resume precedence'

  'docs/recipes/resume-work.md'
  'Recap may show one next-priority candidate as `not started`; it does not start that item'
  'continuation-routing-selftest.sh'
  'Resume recipe phrases terminal recap without execution'
)

present=0
drifted=0
absent_file=0

report() {
  [[ "$quiet" -eq 1 ]] && return 0
  printf '[%s] %s\n' "$LABEL" "$*"
}

i=0
while [[ "$i" -lt "${#CLAIMS[@]}" ]]; do
  rel_path="${CLAIMS[$i]}"
  pattern="${CLAIMS[$((i + 1))]}"
  origin="${CLAIMS[$((i + 2))]}"
  claim="${CLAIMS[$((i + 3))]}"
  i=$((i + 4))

  abs_path="$REPO_ROOT/$rel_path"
  if [[ ! -f "$abs_path" ]]; then
    absent_file=$((absent_file + 1))
    report "NO-FILE   $rel_path — $claim (moved from $origin)"
    continue
  fi
  if grep -Eq -- "$pattern" "$abs_path"; then
    present=$((present + 1))
    report "PRESENT   $rel_path — $claim"
  else
    drifted=$((drifted + 1))
    # Always surfaced, even under --quiet: drift is the reason to run this.
    printf '[%s] DRIFTED   %s — %s\n' "$LABEL" "$rel_path" "$claim"
    printf '[%s]           expected wording: %s\n' "$LABEL" "$pattern"
    printf '[%s]           moved from: %s (advisory — not a build failure)\n' "$LABEL" "$origin"
  fi
done

printf '[%s] %d wording claim(s): %d present, %d drifted, %d file-absent — advisory, exit 0\n' \
  "$LABEL" "$(((${#CLAIMS[@]}) / 4))" "$present" "$drifted" "$absent_file"

exit 0
