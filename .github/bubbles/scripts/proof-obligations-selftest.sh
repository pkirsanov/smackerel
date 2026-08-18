#!/usr/bin/env bash
# bubbles/scripts/proof-obligations-selftest.sh
#
# Capability: proportionate-proof
#
# Acceptance corpus for IMP-047 S-D — the trait-derived obligation matrix is
# authoritative and the physical test CATEGORY is proportionate to traits.
#
# WHY A CORPUS RATHER THAN A HAPPY PATH
# This slice DEFINES applicability, so a selftest that exercised one trait would
# be evidence about one trait. The corpus therefore covers EVERY trait in the
# registry plus a valid not-applicable case that NAMES the absent trait, which
# is the only shape an exemption may take.
#
# Every adversarial case PASSES if S-D is reverted: each one is a substitution
# the old universal-E2E wording either permitted outright or could not express.
#
# Exit codes:
#   0 = all cases pass
#   1 = at least one case failed

set -uo pipefail

NAME="proof-obligations-selftest"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/scenario-obligation-lint.sh"
REGISTRY="$SCRIPT_DIR/../registry/proof-obligations.yaml"
SHARED_DIR="$SCRIPT_DIR/../../agents/bubbles_shared"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/$NAME.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

checks=0
failures=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -lt 2 ]] || printf '       %s\n' "$2"
}

for required in "$LINT" "$REGISTRY"; do
  [[ -f "$required" ]] || {
    printf '%s: missing: %s\n' "$NAME" "$required" >&2
    exit 1
  }
done

printf '%s: %s\n' "$NAME" "$REGISTRY"

case_n=0
RC=0
OUT=""
run_case() {
  # $1 = scenario JSON object body (without the surrounding manifest envelope)
  case_n=$((case_n + 1))
  local dir="$WORK/case-$case_n"
  mkdir -p "$dir"
  printf '{"schemaVersion":1,"scenarios":[%s]}\n' "$1" >"$dir/scenario-manifest.json"
  OUT="$(bash "$LINT" "$dir" 2>&1)"
  RC=$?
}

expect_pass() {
  run_case "$2"
  if [[ "$RC" -eq 0 ]]; then
    ok "$1"
  else
    bad "$1" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
}

expect_code() {
  # $1 label, $2 expected finding code, $3 scenario JSON
  run_case "$3"
  if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "$2"; then
    ok "$1"
  else
    bad "$1" "expected $2; rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
}

MECH_UI='"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-fixture","assertionSurface":"visible-ui","dependencyPath":"not-applicable","productionOwners":["src/ui.ts"],"negativeControl":"wrong route renders nothing"}'
MECH_API='"testMechanism":{"entrypoint":"production-api","inputOrigin":"ephemeral-real","assertionSurface":"http-response","dependencyPath":"same-origin-real","productionOwners":["src/api.ts"],"negativeControl":"changed payload returns 400"}'
MECH_STATE='"testMechanism":{"entrypoint":"production-api","inputOrigin":"ephemeral-real","assertionSurface":"persisted-state","dependencyPath":"ephemeral-real","productionOwners":["src/store.ts"],"negativeControl":"skipping the write leaves the row absent"}'

# =============================================================================
# 1. Registry contract shape
# =============================================================================
trait_ids="$(awk '/^traits:/{t=1;next} /^[a-zA-Z]/{t=0} t && /^  - id: /{sub(/^  - id: /,"");print}' "$REGISTRY")"
trait_count="$(printf '%s\n' "$trait_ids" | grep -c .)"
if [[ "$trait_count" -ge 11 ]]; then
  ok "the registry declares $trait_count traits, including runtime-config and static-metadata"
else
  bad "registry trait count" "found $trait_count"
fi

missing_proof=""
while IFS= read -r trait; do
  [[ -n "$trait" ]] || continue
  live="$(awk -v want="$trait" '
    /^traits:/{t=1;next} /^[a-zA-Z]/{t=0}
    t && $0 ~ "^  - id: " want "$" {f=1;next}
    f && /^  - id: /{exit}
    f && /^    liveProof: /{sub(/^    liveProof: /,"");print;exit}
  ' "$REGISTRY")"
  [[ -n "$live" ]] || missing_proof="$missing_proof $trait"
done <<<"$trait_ids"
if [[ -z "$missing_proof" ]]; then
  ok "every trait declares a liveProof disposition — no trait is silently unowned"
else
  bad "every trait declares liveProof" "missing:$missing_proof"
fi

if grep -q 'id: persistent-regression' "$REGISTRY"; then
  ok "persistent regression is declared UNIVERSAL, retired by no trait"
else
  bad "persistent regression is universal"
fi

# =============================================================================
# 2. Acceptance corpus — one conforming scenario per trait
# =============================================================================

# AC-1. A pure calculation passes with production-unit proof and NO e2e shell.
expect_pass "AC-1 pure-calculation passes with a unit category and no live shell" \
  '{"id":"SCN-001-001","title":"rounds to two places","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"assertion over transformed output","satisfiedBy":["t/round.test.ts"]}]}'

# AC-2. UI on the current production route.
expect_pass "AC-2 user-visible-ui passes with a production-route visible assertion" \
  "{\"id\":\"SCN-001-002\",\"title\":\"list renders\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[\"user-visible-ui\"],\"obligations\":[{\"trait\":\"user-visible-ui\",\"requiredProof\":\"visible assertion\"}],$MECH_UI}"

# AC-3. Real request and response.
expect_pass "AC-3 api-contract passes with a real request and observable response" \
  "{\"id\":\"SCN-001-003\",\"title\":\"create returns 201\",\"requiredTestType\":\"e2e-api\",\"behaviorTraits\":[\"api-contract\"],\"obligations\":[{\"trait\":\"api-contract\",\"requiredProof\":\"real request and response\"}],$MECH_API}"

# AC-4. Write and read round trip.
expect_pass "AC-4 mutable-state passes with a write-and-read round trip" \
  "{\"id\":\"SCN-001-004\",\"title\":\"item persists\",\"requiredTestType\":\"integration\",\"behaviorTraits\":[\"mutable-state\"],\"obligations\":[{\"trait\":\"mutable-state\",\"requiredProof\":\"write, read, persistence round trip\"}],$MECH_STATE}"

expect_pass "AC-5 degraded-state passes with a named negative-path assertion" \
  '{"id":"SCN-001-005","title":"empty state is honest","requiredTestType":"functional","behaviorTraits":["degraded-state"],"obligations":[{"trait":"degraded-state","requiredProof":"named negative-path assertion","satisfiedBy":["t/empty.test.ts"]}]}'

expect_pass "AC-6 shared-consumer passes with parity AND a consumer-surface proof" \
  '{"id":"SCN-001-006","title":"shared formatter","requiredTestType":"e2e-ui","behaviorTraits":["shared-consumer"],"obligations":[{"trait":"shared-consumer","requiredProof":"parity plus consumer surface","satisfiedBy":["parity:t/p.test.ts","consumer-surface:t/c.spec.ts"]}]}'

expect_pass "AC-7 dependency-path passes when it observes a real boundary" \
  '{"id":"SCN-001-007","title":"stale cache refreshes from the provider","requiredTestType":"integration","behaviorTraits":["dependency-path"],"obligations":[{"trait":"dependency-path","requiredProof":"live boundary observation","satisfiedBy":["t/boundary.test.ts","cache-case:stale-paints-before-delta"]}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"live-provider","assertionSurface":"visible-ui","dependencyPath":"external-live","productionOwners":["src/fetch.ts"],"negativeControl":"provider outage surfaces unavailable"}}'

expect_pass "AC-8 responsive-accessible passes with viewport and a11y proof on the route" \
  "{\"id\":\"SCN-001-008\",\"title\":\"nav collapses at 375px\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[\"responsive-accessible\"],\"obligations\":[{\"trait\":\"responsive-accessible\",\"requiredProof\":\"viewport and accessibility behavior\"}],$MECH_UI}"

expect_pass "AC-9 sla-sensitive passes with a stress category" \
  '{"id":"SCN-001-009","title":"p95 under 200ms","requiredTestType":"stress","behaviorTraits":["sla-sensitive"],"obligations":[{"trait":"sla-sensitive","requiredProof":"stress assertion against the threshold","satisfiedBy":["t/p95.stress.ts"]}]}'

expect_pass "AC-10 runtime-config passes when startup executes the configured value" \
  '{"id":"SCN-001-010","title":"retry budget is honoured at startup","requiredTestType":"integration","behaviorTraits":["runtime-config"],"obligations":[{"trait":"runtime-config","requiredProof":"startup executes the configured value"}],"testMechanism":{"entrypoint":"production-cli","inputOrigin":"ephemeral-real","assertionSurface":"http-response","dependencyPath":"same-origin-real","productionOwners":["src/config.ts"],"negativeControl":"a different budget changes the observed retry count"}}'

expect_pass "AC-11 static-metadata passes with proportionate artifact proof" \
  '{"id":"SCN-001-011","title":"the catalog lists every recipe","requiredTestType":"unit","behaviorTraits":["static-metadata"],"obligations":[{"trait":"static-metadata","requiredProof":"artifact assertion over the declared value","satisfiedBy":["t/catalog.test.ts"]}]}'

# AC-12. The valid not-applicable case, NAMING the absent trait.
expect_pass "AC-12 a live-proof exemption that NAMES the absent trait is accepted" \
  '{"id":"SCN-001-012","title":"tax rounding","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"assertion over transformed output","satisfiedBy":["t/tax.test.ts"]}],"liveProofNotApplicable":{"absentTrait":"user-visible-ui","reason":"the calculation has no rendered surface; its caller owns display"}}'

# =============================================================================
# 3. Adversarial — each of these PASSES if S-D is reverted
# =============================================================================

expect_code "ADV-1 user-visible-ui asserting only hidden DOM is refused" \
  "LIVE-PROOF-SUBSTITUTED" \
  '{"id":"SCN-002-001","title":"list renders","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"}],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-fixture","assertionSurface":"hidden-dom","dependencyPath":"not-applicable","productionOwners":["src/ui.ts"],"negativeControl":"changed input changes the node"}}'

expect_code "ADV-2 user-visible-ui entered through a detached renderer is refused" \
  "LIVE-PROOF-SUBSTITUTED" \
  '{"id":"SCN-002-002","title":"list renders","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"}],"testMechanism":{"entrypoint":"detached-renderer","inputOrigin":"synthetic-fixture","assertionSurface":"visible-ui","dependencyPath":"not-applicable","productionOwners":["src/ui.ts"],"negativeControl":"changed props change the output"}}'

expect_code "ADV-3 api-contract asserting a returned value is refused" \
  "LIVE-PROOF-SUBSTITUTED" \
  '{"id":"SCN-002-003","title":"create returns 201","requiredTestType":"e2e-api","behaviorTraits":["api-contract"],"obligations":[{"trait":"api-contract","requiredProof":"real request and response"}],"testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/api.ts"],"negativeControl":"bad payload returns an error object"}}'

expect_code "ADV-4 mutable-state asserting an in-memory value is refused" \
  "LIVE-PROOF-SUBSTITUTED" \
  '{"id":"SCN-002-004","title":"item persists","requiredTestType":"integration","behaviorTraits":["mutable-state"],"obligations":[{"trait":"mutable-state","requiredProof":"write, read, persistence round trip"}],"testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"internal-state","dependencyPath":"synthetic-boundary","productionOwners":["src/store.ts"],"negativeControl":"skipping the write leaves the value unset"}}'

expect_code "ADV-5 an SLA claim typed as e2e-ui is refused — stress or load is the proof" \
  "LIVE-PROOF-CATEGORY" \
  "{\"id\":\"SCN-002-005\",\"title\":\"p95 under 200ms\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[\"sla-sensitive\"],\"obligations\":[{\"trait\":\"sla-sensitive\",\"requiredProof\":\"stress assertion\",\"satisfiedBy\":[\"t/p95.spec.ts\"]}],$MECH_UI}"

expect_code "ADV-6 runtime-config proved through an internal helper is refused (no docs exemption)" \
  "LIVE-PROOF-SUBSTITUTED" \
  '{"id":"SCN-002-006","title":"retry budget is honoured","requiredTestType":"unit","behaviorTraits":["runtime-config"],"obligations":[{"trait":"runtime-config","requiredProof":"startup executes the configured value"}],"testMechanism":{"entrypoint":"internal-helper","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/config.ts"],"negativeControl":"a malformed file throws"}}'

expect_code "ADV-7 a live-owing trait with no named proof and no mechanism is refused" \
  "LIVE-PROOF-UNDECLARED" \
  '{"id":"SCN-002-007","title":"list renders","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"}]}'

expect_code "ADV-8 an exemption naming a trait the scenario DECLARES is refused" \
  "NA-CONTRADICTS-TRAIT" \
  "{\"id\":\"SCN-002-008\",\"title\":\"list renders\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[\"user-visible-ui\"],\"obligations\":[{\"trait\":\"user-visible-ui\",\"requiredProof\":\"visible assertion\"}],$MECH_UI,\"liveProofNotApplicable\":{\"absentTrait\":\"user-visible-ui\",\"reason\":\"it is only a small change\"}}"

expect_code "ADV-9 an exemption with no reason is refused" \
  "NA-MALFORMED" \
  '{"id":"SCN-002-009","title":"tax rounding","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"x","satisfiedBy":["t/tax.test.ts"]}],"liveProofNotApplicable":{"absentTrait":"user-visible-ui"}}'

expect_code "ADV-10 an exemption naming a trait outside the registry is refused" \
  "NA-MALFORMED" \
  '{"id":"SCN-002-010","title":"tax rounding","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"x","satisfiedBy":["t/tax.test.ts"]}],"liveProofNotApplicable":{"absentTrait":"probably-fine","reason":"seems unrelated"}}'

expect_code "ADV-11 an UNKNOWN trait cannot earn a live-proof exemption" \
  "NA-UNKNOWN-TRAIT" \
  '{"id":"SCN-002-011","title":"whatever","requiredTestType":"unit","behaviorTraits":["mostly-cosmetic"],"obligations":[{"trait":"mostly-cosmetic","requiredProof":"x","satisfiedBy":["t/x.test.ts"]}],"liveProofNotApplicable":{"absentTrait":"user-visible-ui","reason":"nothing renders"}}'

# =============================================================================
# 4. The lint READS the registry rather than restating it
# =============================================================================
alt_reg="$WORK/alt-proof-obligations.yaml"
sed -e '/^  - id: user-visible-ui$/,/^  - id: api-contract$/ s/^    liveProof: required$/    liveProof: not-required/' \
  "$REGISTRY" >"$alt_reg"
undeclared='{"id":"SCN-003-001","title":"list renders","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"}]}'
alt_dir="$WORK/alt-case"
mkdir -p "$alt_dir"
printf '{"schemaVersion":1,"scenarios":[%s]}\n' "$undeclared" >"$alt_dir/scenario-manifest.json"
alt_out="$(BUBBLES_PROOF_OBLIGATIONS_REGISTRY="$alt_reg" bash "$LINT" "$alt_dir" 2>&1)"
alt_rc=$?
default_out="$(bash "$LINT" "$alt_dir" 2>&1)"
default_rc=$?
# A clean exit alone would also be produced by a lint that stopped emitting the
# finding for an unrelated reason, so the ABSENCE of the finding is asserted too.
alt_undeclared=absent
if printf '%s' "$alt_out" | grep -q 'LIVE-PROOF-UNDECLARED'; then
  alt_undeclared=present
fi
if [[ "$alt_rc" -eq 0 && "$alt_undeclared" == "absent" && "$default_rc" -eq 1 ]] &&
  printf '%s' "$default_out" | grep -q 'LIVE-PROOF-UNDECLARED'; then
  ok "flipping liveProof in the registry changes the lint (single source, not a copy)"
else
  bad "lint reads the registry" "alt rc=$alt_rc alt LIVE-PROOF-UNDECLARED=$alt_undeclared default rc=$default_rc"
fi

# =============================================================================
# 5. The prose surfaces are reconciled, not merely edited
# =============================================================================
crit="$SHARED_DIR/critical-requirements.md"
e2e="$SHARED_DIR/e2e-regression.md"

if [[ -f "$crit" ]] && grep -q 'PERSISTENT REGRESSION test' "$crit" &&
  ! grep -q 'scenario-specific E2E regression test' "$crit"; then
  ok "critical-requirements.md keeps regression universal and drops the universal-E2E wording"
else
  bad "critical-requirements.md reconciled" "$(grep -n 'E2E' "$crit" 2>/dev/null | tr '\n' '|')"
fi

if [[ -f "$e2e" ]] && grep -q 'persistent scenario-specific regression coverage. This is UNIVERSAL' "$e2e" &&
  grep -q 'proof-obligations.yaml' "$e2e"; then
  ok "e2e-regression.md defers the CATEGORY question to the trait matrix"
else
  bad "e2e-regression.md reconciled" "$(grep -n 'E2E regression coverage' "$e2e" 2>/dev/null | tr '\n' '|')"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
