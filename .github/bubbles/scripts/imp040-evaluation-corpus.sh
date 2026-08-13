#!/usr/bin/env bash
# bubbles/scripts/imp040-evaluation-corpus.sh
#
# Held-out evaluation corpus for the IMP-040 certification contract (SCOPE-13).
#
# WHAT THIS MEASURES, AND WHY EACH NUMBER MATTERS
#   FALSE ACCEPTANCE — a packet that SHOULD be refused and was not. This is the
#     failure the whole improvement exists to remove; a single one means a gate
#     is decorative.
#   FALSE REJECTION  — a packet that SHOULD pass and was refused. This is the
#     failure that gets gates switched off. A gate with a high false-rejection
#     rate is worse than no gate, because teams route around it and stop reading
#     its output.
#   PLANNING EXPANSION — how many extra declared fields the contract asks for on
#     a legitimate packet. Reported as a count, not a verdict: the cost is real
#     and should be visible rather than argued about.
#   RUNTIME COST — wall-clock seconds for the full corpus.
#
# EIGHT REPOSITORY SHAPES, because the contract must not be accidentally
# JavaScript-shaped. Each shape gets a GREEN packet (must pass) and a RED packet
# (must fail), so every shape tests both directions. A corpus of only-green
# fixtures would report a perfect score for a gate that never fires, and a
# corpus of only-red fixtures would report a perfect score for a gate that
# refuses everything.
#
# Exit codes:
#   0  no false acceptances and no false rejections
#   1  at least one false acceptance or false rejection
#   2  usage error

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="imp040-evaluation-corpus"
VERBOSE=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --verbose) VERBOSE=1; shift ;;
    -h | --help)
      cat <<'EOF'
Usage: imp040-evaluation-corpus.sh [--verbose]

Evaluate the IMP-040 certification contract across eight repository shapes.
Reports false acceptance, false rejection, planning expansion and runtime cost.

Exit: 0 clean | 1 false acceptance or false rejection | 2 usage
EOF
      exit 0 ;;
    --skip* | --force | --ignore*)
      echo "$NAME: '$1' is bypass-shaped and is not supported." >&2
      exit 2 ;;
    *) echo "$NAME: unknown option: $1" >&2; exit 2 ;;
  esac
done

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

false_accept=0
false_reject=0
evaluated=0
declared_fields=0

OBLIGATION="$SCRIPT_DIR/scenario-obligation-lint.sh"
MECHANISM="$SCRIPT_DIR/test-mechanism-lint.sh"

# Emit a packet. $1 dir, $2 manifest JSON.
emit() {
  mkdir -p "$1"
  printf '%s\n' "$2" >"$1/scenario-manifest.json"
}

# $1 shape, $2 expectation (pass|fail), $3 manifest JSON
evaluate() {
  local shape="$1" expect="$2" manifest="$3"
  local dir="$WORK/${shape}-${expect}"
  emit "$dir" "$manifest"
  evaluated=$((evaluated + 1))

  # Count declared contract fields on this packet — the planning-expansion cost.
  local n
  n="$(grep -oE '"(behaviorTraits|obligations|testMechanism|implementationRefs|riskTier)"' \
    "$dir/scenario-manifest.json" 2>/dev/null | wc -l | tr -d ' ')"
  declared_fields=$((declared_fields + n))

  local rc=0
  bash "$OBLIGATION" "$dir" --quiet >/dev/null 2>&1 || rc=1
  bash "$MECHANISM" "$dir" --quiet >/dev/null 2>&1 || rc=1

  if [ "$expect" = "pass" ]; then
    if [ "$rc" -eq 0 ]; then
      [ "$VERBOSE" = "1" ] && printf '  ok    %-28s green packet accepted\n' "$shape"
    else
      false_reject=$((false_reject + 1))
      printf '  FALSE-REJECT %-22s a legitimate packet was refused\n' "$shape"
      bash "$OBLIGATION" "$dir" --quiet 2>&1 | sed 's/^/        /'
      bash "$MECHANISM" "$dir" --quiet 2>&1 | sed 's/^/        /'
    fi
  else
    if [ "$rc" -ne 0 ]; then
      [ "$VERBOSE" = "1" ] && printf '  ok    %-28s red packet refused\n' "$shape"
    else
      false_accept=$((false_accept + 1))
      printf '  FALSE-ACCEPT %-22s a packet that should be refused passed\n' "$shape"
    fi
  fi
}

started="$(date +%s)"

echo "============================================================"
echo "  IMP-040 EVALUATION CORPUS (SCOPE-13)"
echo "============================================================"

# --- Shape 1: static browser tool (no build, no runner inventory) -----------
# GREEN: a visible-UI scenario proved on the route it ships on.
evaluate static-browser pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Chart renders on the tool page","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the route"}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-fixture","assertionSurface":"visible-ui","dependencyPath":"not-applicable","productionOwners":["tool.html"],"negativeControl":"an empty dataset renders the empty state"}}]}'
# RED: same scenario proved against hidden DOM.
evaluate static-browser fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Chart renders on the tool page","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the route"}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-fixture","assertionSurface":"hidden-dom","dependencyPath":"not-applicable","productionOwners":["tool.html"],"negativeControl":"an empty dataset renders the empty state"}}]}'

# --- Shape 2: service API project ------------------------------------------
evaluate service-api pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Create order returns 201","requiredTestType":"e2e-api","behaviorTraits":["api-contract"],"obligations":[{"trait":"api-contract","requiredProof":"real request and observable response"}],"testMechanism":{"entrypoint":"production-api","inputOrigin":"ephemeral-real","assertionSurface":"http-response","dependencyPath":"same-origin-real","productionOwners":["src/orders/create.go"],"negativeControl":"a malformed body returns 400"}}]}'
# RED: a wire contract proved by an in-process return value.
evaluate service-api fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Create order returns 201","requiredTestType":"e2e-api","behaviorTraits":["api-contract"],"obligations":[{"trait":"api-contract","requiredProof":"real request and observable response"}],"testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/orders/create.go"],"negativeControl":"a malformed body returns 400"}}]}'

# --- Shape 3: command-line tool (no UI at all) ------------------------------
evaluate cli-tool pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Exit code reflects validation failure","requiredTestType":"functional","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"production-unit assertion over output"}],"testMechanism":{"entrypoint":"production-cli","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["cmd/validate.sh"],"negativeControl":"a valid input exits 0"}}]}'
# RED: declares a trait it never discharges.
evaluate cli-tool fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Exit code reflects validation failure","requiredTestType":"functional","behaviorTraits":["pure-calculation","degraded-state"],"obligations":[{"trait":"pure-calculation","requiredProof":"production-unit assertion over output"}],"testMechanism":{"entrypoint":"production-cli","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["cmd/validate.sh"],"negativeControl":"a valid input exits 0"}}]}'

# --- Shape 4: strongly typed compiled project (high risk) -------------------
evaluate compiled-typed pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Settlement amount is computed from the rate table","requiredTestType":"unit","riskTier":"high","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"production-unit assertion over transformed output"}],"testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/settle.rs"],"negativeControl":"perturbing the rate changes the settlement","negativeControlMechanism":"perturbed-input","negativeControlFallbackReason":"no mutation adapter configured for this repository"}}]}'
# RED: high risk with only adversarial input and no stated fallback.
evaluate compiled-typed fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Settlement amount is computed from the rate table","requiredTestType":"unit","riskTier":"high","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"production-unit assertion over transformed output"}],"testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/settle.rs"],"negativeControl":"perturbing the rate changes the settlement","negativeControlMechanism":"adversarial-input"}}]}'

# --- Shape 5: Python data project (cache-first) -----------------------------
evaluate python-data pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Cached frame renders on the report","requiredTestType":"integration","behaviorTraits":["dependency-path"],"obligations":[{"trait":"dependency-path","requiredProof":"declared dependency state and boundary assertion","satisfiedBy":["cache-case:fresh-no-fetch"]}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-cache","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["src/report.py"],"negativeControl":"an empty cache renders the unavailable state"}}]}'
# RED: a freshness claim proved cache-only.
evaluate python-data fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Stale frame is refreshed from the provider","requiredTestType":"integration","behaviorTraits":["dependency-path"],"obligations":[{"trait":"dependency-path","requiredProof":"declared dependency state and boundary assertion","satisfiedBy":["cache-case:fresh-no-fetch"]}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-cache","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["src/report.py"],"negativeControl":"an empty cache renders the unavailable state"}}]}'

# --- Shape 6: project with a custom runner ----------------------------------
evaluate custom-runner pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Scene draws the welcome panel","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the current route"}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["components/WelcomeScene.brs"],"negativeControl":"a missing config draws the fallback panel"}}]}'
# RED: proved by a detached renderer call.
evaluate custom-runner fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Scene draws the welcome panel","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the current route"}],"testMechanism":{"entrypoint":"detached-renderer","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["components/WelcomeScene.brs"],"negativeControl":"a missing config draws the fallback panel"}}]}'

# --- Shape 7: project with no UI (shared library) ---------------------------
evaluate no-ui pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Serializer round-trips the envelope","requiredTestType":"integration","behaviorTraits":["shared-consumer"],"obligations":[{"trait":"shared-consumer","requiredProof":"owner parity plus consumer surface","satisfiedBy":["parity:tests/parity.spec","consumer-surface:tests/consumer.spec"]}],"testMechanism":{"entrypoint":"production-api","inputOrigin":"ephemeral-real","assertionSurface":"http-response","dependencyPath":"same-origin-real","productionOwners":["libs/serde.ts"],"negativeControl":"an unknown field is rejected"}}]}'
# RED: shared consumer with parity only.
evaluate no-ui fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Serializer round-trips the envelope","requiredTestType":"integration","behaviorTraits":["shared-consumer"],"obligations":[{"trait":"shared-consumer","requiredProof":"owner parity plus consumer surface","satisfiedBy":["parity:tests/parity.spec"]}],"testMechanism":{"entrypoint":"production-api","inputOrigin":"ephemeral-real","assertionSurface":"http-response","dependencyPath":"same-origin-real","productionOwners":["libs/serde.ts"],"negativeControl":"an unknown field is rejected"}}]}'

# --- Shape 8: no test-title inventory (adapter = none) ----------------------
# The neutral adapter cannot compare categories, so a legitimate packet must
# still pass. If this shape false-rejects, the contract has become unusable for
# every repository without a runner inventory — which is most of them.
evaluate no-inventory pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Total updates when quantity changes","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the route"}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"accessibility-tree","dependencyPath":"cache-only","productionOwners":["src/cart.tsx"],"negativeControl":"quantity zero hides the total"}}]}'
# RED: mechanism field outside the closed vocabulary.
evaluate no-inventory fail '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Total updates when quantity changes","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the route"}],"testMechanism":{"entrypoint":"whatever-works","inputOrigin":"seeded-store","assertionSurface":"accessibility-tree","dependencyPath":"cache-only","productionOwners":["src/cart.tsx"],"negativeControl":"quantity zero hides the total"}}]}'

# --- Legacy grandfathering ---------------------------------------------------
# A legacy packet declaring NONE of the new fields must pass untouched. If this
# fails, every historical spec in every downstream repo breaks on upgrade.
evaluate legacy-untouched pass '{"schemaVersion":1,"scenarios":[{"id":"SCN-1","title":"Legacy scenario","requiredTestType":"unit"}]}'

elapsed=$(( $(date +%s) - started ))

echo ""
echo "------------------------------------------------------------"
printf '  packets evaluated      : %s\n' "$evaluated"
printf '  FALSE ACCEPTANCE       : %s\n' "$false_accept"
printf '  FALSE REJECTION        : %s\n' "$false_reject"
printf '  planning expansion     : %s declared contract field(s) across the corpus\n' "$declared_fields"
printf '  runtime cost           : %ss wall clock\n' "$elapsed"
echo "------------------------------------------------------------"

if [ "$false_accept" -gt 0 ] || [ "$false_reject" -gt 0 ]; then
  echo "$NAME: FAIL — the contract mis-classified at least one packet."
  exit 1
fi

echo "$NAME: PASS — no false acceptance, no false rejection across 8 repository shapes."
exit 0
