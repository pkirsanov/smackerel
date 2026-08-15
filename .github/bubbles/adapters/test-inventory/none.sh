#!/usr/bin/env bash
# bubbles/adapters/test-inventory/none.sh — no-op test-inventory adapter.
#
# DEFAULT adapter (IMP-040 SCOPE-1). A repository that has not configured a test
# inventory resolves here, so every consumer runs unchanged and the framework
# NEVER gains a dependency on a language-specific test runner.
#
# THE HONESTY RULE THAT DEFINES THIS ADAPTER
# A test inventory answers "which titled tests actually exist, and in what
# category". Only a runner can answer that. With no adapter configured the
# honest answer is `unmeasured` — NOT an empty inventory, because an empty
# inventory is a positive claim that no tests exist, and a resolver reading it
# as such would fail every linked reference in the repository.
#
# The distinction is load-bearing for Gate G057. `scenario-test-resolve.sh`
# degrades to a conservative literal scan of the referenced FILE when this
# adapter is active: that still catches a title that exists nowhere (BUG-030),
# while a category comparison is SKIPPED rather than guessed. Skipping an
# unmeasurable dimension is the same discipline G128 applies to token counts.
#
# Canonical per-verb shapes (validated by
# test-inventory-adapter-contract-selftest.sh):
#   tests                 → JSON map   → neutral empty value: {}
#   status                → JSON map   → {"measured":false,"adapter":"none",...}
#   capabilities          → JSON map   → neutral empty value: {}
#
# `tests` returns the neutral MAP, not a bare `[]`, precisely so it cannot be
# mistaken for a measured-and-empty inventory. A configured adapter returns the
# contract document instead:
#   {"contractVersion":"bubbles-test-inventory/v1","tests":[{id,file,title,
#     category,runner,tags}]}
#
# `status` is the ONE verb that does not return the bare neutral map, because
# "no data" must be distinguishable from "measured zero".

set -euo pipefail

NEUTRAL_STATUS='{"measured":false,"adapter":"none","reason":"no test inventory adapter configured"}'

emit() {
  case "$1" in
    tests | capabilities) echo '{}' ;;
    status) printf '%s\n' "$NEUTRAL_STATUS" ;;
    *) return 1 ;;
  esac
}

VERB="${1:-}"

case "$VERB" in
  tests | status | capabilities)
    emit "$VERB"
    exit 0
    ;;
  selftest)
    # Same shapes through a second entry point so the contract selftest can
    # prove the neutral values are not produced by a divergent code path.
    if emit "${2:-}"; then
      exit 0
    fi
    echo "none.sh: selftest requires a known verb (tests|status|capabilities)" >&2
    exit 2
    ;;
  '' | -h | --help)
    cat <<'EOF'
none.sh — no-op test-inventory adapter (framework default)
Usage: none.sh <verb> [args...]
Verbs: tests (-> {}) | status (-> {"measured":false,...}) |
       capabilities (-> {}) | selftest <verb> (-> canonical neutral shape)
EOF
    exit 0
    ;;
  *)
    echo "none.sh: unknown verb '$VERB' (expected tests|status|capabilities)" >&2
    exit 2
    ;;
esac
