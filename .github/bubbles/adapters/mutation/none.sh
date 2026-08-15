#!/usr/bin/env bash
# bubbles/adapters/mutation/none.sh — no-op mutation-execution adapter.
#
# DEFAULT adapter (IMP-040 SCOPE-7). A repository that has not configured
# mutation tooling resolves here, so the framework NEVER gains a dependency on a
# language-specific mutation runner. Bubbles supports bash, Rust, Go, Python,
# TypeScript, Dart, Scala and BrightScript repositories; hardcoding any one
# mutation engine would make the strongest negative control available to some
# languages and unreachable in others.
#
# THE HONESTY RULE THAT DEFINES THIS ADAPTER
# A mutation run answers "if the owning predicate were wrong, would this test
# notice". Only a runner can answer that. With no adapter configured the honest
# answer is `unmeasured` — NOT a survival rate of zero, and NOT a pass. A zero
# would be a positive claim that no mutant survived, which is exactly the
# unearned confidence COV-11 exists to remove.
#
# This is why `test-mechanism-lint.sh` does not simply refuse a high-risk
# scenario that lacks `negativeControlMechanism: mutation`. It accepts a weaker
# control when the scenario names a `negativeControlFallbackReason`. A project
# without tooling stays honest by SAYING it has none, rather than either lying
# about a mutation run or being unable to declare risk at all.
#
# Canonical per-verb shapes (validated by
# mutation-adapter-contract-selftest.sh):
#   run                   → JSON map   → neutral empty value: {}
#   status                → JSON map   → {"measured":false,"adapter":"none",...}
#   capabilities          → JSON map   → neutral empty value: {}
#
# `run` returns the neutral MAP, not a bare `[]` and not a survival count,
# precisely so it cannot be mistaken for a measured result. A configured adapter
# returns the contract document instead:
#   {"contractVersion":"bubbles-mutation/v1","mutants":[{id,file,line,operator,
#     status}],"survived":<int>,"killed":<int>}
#
# `status` is the ONE verb that does not return the bare neutral map, because
# "no data" must be distinguishable from "measured zero survivors".

set -euo pipefail

NEUTRAL_STATUS='{"measured":false,"adapter":"none","reason":"no mutation adapter configured"}'

emit() {
  case "$1" in
    run | capabilities) echo '{}' ;;
    status) printf '%s\n' "$NEUTRAL_STATUS" ;;
    *) return 1 ;;
  esac
}

VERB="${1:-}"

case "$VERB" in
  run | status | capabilities)
    emit "$VERB"
    exit 0
    ;;
  selftest)
    # Same shapes through a second entry point so the contract selftest can
    # prove the neutral values are not produced by a divergent code path.
    if emit "${2:-}"; then
      exit 0
    fi
    echo "none.sh: selftest requires a known verb (run|status|capabilities)" >&2
    exit 2
    ;;
  '' | -h | --help)
    cat <<'EOF'
none.sh — no-op mutation-execution adapter (framework default)
Usage: none.sh <verb> [args...]
Verbs: run (-> {}) | status (-> {"measured":false,...}) |
       capabilities (-> {}) | selftest <verb> (-> canonical neutral shape)
EOF
    exit 0
    ;;
  *)
    echo "none.sh: unknown verb '$VERB' (expected run|status|capabilities)" >&2
    exit 2
    ;;
esac
