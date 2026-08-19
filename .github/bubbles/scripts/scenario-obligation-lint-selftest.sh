#!/usr/bin/env bash
# bubbles/scripts/scenario-obligation-lint-selftest.sh
#
# Hermetic selftest for scenario-obligation-lint.sh (IMP-040 SCOPE-3 / COV-9).
#
# The load-bearing case is A4: a scenario that declares the ENTIRE trait
# vocabulary must be refused. That is the failure mode the obligation matrix
# exists to prevent — an always-identical obligation block is exactly as
# uninformative as the row count it replaced.
#
# P1 is its necessary twin: a packet that declares nothing must stay INERT.
# These fields are new, so a lint that demanded them would retro-break every
# existing manifest.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/scenario-obligation-lint.sh"
NAME="scenario-obligation-lint-selftest"

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

make_case() {
  local root="$WORK/$1"
  mkdir -p "$root"
  printf '%s\n' "$2" >"$root/scenario-manifest.json"
  printf '%s' "$root"
}

run_lint() {
  set +e
  OUT="$(bash "$TARGET" "$1" --quiet 2>&1)"
  RC=$?
  set -e
}

# --- P1. a packet declaring nothing stays inert -----------------------------
R="$(make_case p1 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","linkedTests":["tests/a.ts"]}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P1 a manifest with no behaviorTraits is inert"
else
  bad "P1 inert on undeclared" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P2. a coherent derived matrix passes -----------------------------------
# Each live-owing trait NAMES the test that discharges it (IMP-047 S-D). Whether
# that test is really the category it claims is scenario-test-resolve.sh's
# question; this lint only requires that the scenario say how the proof arrives.
R="$(make_case p2 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui","dependency-path"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the production route","satisfiedBy":["t/route.spec.ts"]},{"trait":"dependency-path","requiredProof":"stale-cache boundary observation","satisfiedBy":["t/boundary.spec.ts"]}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P2 a coherent two-trait matrix passes"
else
  bad "P2 coherent matrix" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. scenarios may legitimately differ in trait count -------------------
R="$(make_case p3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"a","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"assertion over transformed output"}]},{"id":"SCN-001-002","title":"b","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion","satisfiedBy":["t/b.spec.ts"]}]},{"id":"SCN-001-003","title":"c","requiredTestType":"stress","behaviorTraits":["sla-sensitive"],"obligations":[{"trait":"sla-sensitive","requiredProof":"stress assertion against the threshold","satisfiedBy":["t/c.stress.ts"]}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P3 differing per-scenario trait sets pass (derivation, not uniformity)"
else
  bad "P3 differing trait sets" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: a declared trait with no obligation -------------------
R="$(make_case a1 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui","mutable-state"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'TRAIT-COVERED'; then
  ok "A1 a declared trait owing no obligation is refused"
else
  bad "A1 uncovered trait" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: an obligation for an undeclared trait -----------------
R="$(make_case a2 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"},{"trait":"sla-sensitive","requiredProof":"stress"}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'OBLIGATION-ANCHORED'; then
  ok "A2 an obligation naming an undeclared trait is refused"
else
  bad "A2 unanchored obligation" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: an obligation with no stated proof --------------------
R="$(make_case a3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"obligations":[{"trait":"user-visible-ui","requiredProof":"   "}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'TRAIT-COVERED'; then
  ok "A3 an obligation with a blank requiredProof is refused"
else
  bad "A3 blank proof" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: the enumeration anti-pattern --------------------------
# The whole point of SCOPE-3: derived, not enumerated. The trait list is read
# from the registry so this case cannot silently stop being "the entire
# vocabulary" when a trait is added (IMP-047 S-D added two).
REG_TRAITS="$(awk '/^traits:/{t=1;next} /^[a-zA-Z]/{t=0} t && /^  - id: /{sub(/^  - id: /,"");print}' "$SCRIPT_DIR/../registry/proof-obligations.yaml")"
ALL=""
OBS=""
for trait in $REG_TRAITS; do
  [[ -z "$ALL" ]] && ALL="\"$trait\"" || ALL="$ALL,\"$trait\""
  entry="{\"trait\":\"$trait\",\"requiredProof\":\"x\",\"satisfiedBy\":[\"t/x.spec.ts\"]}"
  [[ -z "$OBS" ]] && OBS="$entry" || OBS="$OBS,$entry"
done
R="$(make_case a4 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[$ALL],\"obligations\":[$OBS]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'NOT-ENUMERATED'; then
  ok "A4 a scenario declaring the ENTIRE vocabulary is refused"
else
  bad "A4 enumeration refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. one short of the whole vocabulary is NOT flagged -------------------
# Guards A4 against becoming a judgement threshold. A genuinely multi-trait
# scenario must pass; only the maximal set is unambiguous enumeration.
ALL8='"pure-calculation","user-visible-ui","api-contract","mutable-state","degraded-state","shared-consumer","dependency-path","responsive-accessible"'
OBS8='{"trait":"pure-calculation","requiredProof":"x"},{"trait":"user-visible-ui","requiredProof":"x","satisfiedBy":["t/ui.spec.ts"]},{"trait":"api-contract","requiredProof":"x","satisfiedBy":["t/api.spec.ts"]},{"trait":"mutable-state","requiredProof":"x","satisfiedBy":["t/state.spec.ts"]},{"trait":"degraded-state","requiredProof":"x"},{"trait":"shared-consumer","requiredProof":"x","satisfiedBy":["parity:t/p.spec.ts","consumer-surface:t/c.spec.ts"]},{"trait":"dependency-path","requiredProof":"x","satisfiedBy":["t/boundary.spec.ts"]},{"trait":"responsive-accessible","requiredProof":"x","satisfiedBy":["t/a11y.spec.ts"]}'
R="$(make_case p4 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[$ALL8],\"obligations\":[$OBS8]}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P4 a large but non-maximal trait set passes (A4 is not a judgement threshold)"
else
  bad "P4 large trait set passes" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: a trait outside the vocabulary ------------------------
R="$(make_case a5 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["looks-fine"],"obligations":[{"trait":"looks-fine","requiredProof":"x"}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'UNKNOWN-TRAIT'; then
  ok "A5 a trait outside the vocabulary is refused"
else
  bad "A5 unknown trait" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL: obligations without any declared traits ---------------
R="$(make_case a6 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","obligations":[{"trait":"user-visible-ui","requiredProof":"x"}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'OBLIGATION-ANCHORED'; then
  ok "A6 obligations with no behaviorTraits are refused"
else
  bad "A6 orphan obligations" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- IMP-040 SCOPE-6: dependency-path coverage ------------------------------
#
# A7/A8/A9 are the rules. P5/P6/P7 are their guards: cache-only must stay legal
# for a scenario that is genuinely only about rendering a cached value, and the
# case requirement must not fire on a scenario with no cache-first mechanism.
# Without those guards the check would push authors to declare `external-live`
# on everything, which is the opposite of what it is for.

DEP_OB='"behaviorTraits":["dependency-path"],"obligations":[{"trait":"dependency-path","requiredProof":"boundary assertion"'

# A7. cache-only offered for a freshness claim.
R="$(make_case a7 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Stale cache is refreshed from the provider\",\"requiredTestType\":\"integration\",$DEP_OB,\"satisfiedBy\":[\"cache-case:fresh-no-fetch\"]}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"synthetic-cache\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"cache-only\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'DEPENDENCY-BOUNDARY'; then
  ok "A7 cache-only cannot satisfy a scenario naming freshness"
else
  bad "A7 cache-only vs freshness" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A8. cache-only offered for a fallback claim, named via tags rather than title.
R="$(make_case a8 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Provider result renders\",\"tags\":[\"fallback\"],\"requiredTestType\":\"integration\",$DEP_OB,\"satisfiedBy\":[\"cache-case:fresh-no-fetch\"]}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"synthetic-cache\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"cache-only\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'DEPENDENCY-BOUNDARY'; then
  ok "A8 a boundary claim in tags is caught as well as in the title"
else
  bad "A8 tags boundary" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A9. cache-first scenario naming no case at all.
R="$(make_case a9 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Cached value renders\",\"requiredTestType\":\"integration\",$DEP_OB}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"synthetic-cache\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"cache-only\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'DEPENDENCY-CASE'; then
  ok "A9 a cache-first scenario naming no cache-case is refused"
else
  bad "A9 no cache case" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A10. a mistyped case token is a finding, not a silently uncounted case.
R="$(make_case a10 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Cached value renders\",\"requiredTestType\":\"integration\",$DEP_OB,\"satisfiedBy\":[\"cache-case:fresh-nofetch\"]}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"synthetic-cache\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"cache-only\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'not in the vocabulary'; then
  ok "A10 a mistyped cache-case token is refused"
else
  bad "A10 mistyped case" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P5. cache-only is legal for a scenario that only renders a cached value.
R="$(make_case p5 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Cached price renders on the summary card\",\"requiredTestType\":\"integration\",$DEP_OB,\"satisfiedBy\":[\"cache-case:fresh-no-fetch\"]}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"synthetic-cache\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"cache-only\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P5 cache-only stays legal for a pure cached-render scenario"
else
  bad "P5 cache-only allowed" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P6. a freshness scenario that DOES observe the boundary passes.
R="$(make_case p6 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Stale cache is refreshed from the provider\",\"requiredTestType\":\"integration\",$DEP_OB,\"satisfiedBy\":[\"cache-case:delta-changes-result\"]}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"ephemeral-real\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"same-origin-real\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P6 a freshness scenario observing a real boundary passes"
else
  bad "P6 real boundary" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P7. the case requirement does not fire without a cache-first mechanism.
R="$(make_case p7 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Provider result renders\",\"requiredTestType\":\"integration\",$DEP_OB}],\"testMechanism\":{\"entrypoint\":\"production-route\",\"inputOrigin\":\"ephemeral-real\",\"assertionSurface\":\"visible-ui\",\"dependencyPath\":\"same-origin-real\",\"productionOwners\":[\"a.ts\"],\"negativeControl\":\"x\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P7 the cache-case requirement is scoped to cache-first mechanisms"
else
  bad "P7 non-cache scoped" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- IMP-040 SCOPE-8: shared-consumer parity --------------------------------
#
# A11/A12 are the rule: BOTH halves are owed. P8 is the guard — a scenario that
# declares both passes, so the rule cannot be satisfied by simply never using
# the shared-consumer trait.

SC_HEAD='"behaviorTraits":["shared-consumer"],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"not-applicable","productionOwners":["src/shared/client.ts"],"negativeControl":"x"}'

# A11. only parity declared.
R="$(make_case a11 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$SC_HEAD,\"obligations\":[{\"trait\":\"shared-consumer\",\"requiredProof\":\"parity plus consumer surface\",\"satisfiedBy\":[\"parity:tests/shared/parity.spec.ts\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'consumer-surface'; then
  ok "A11 owner parity alone does not satisfy a shared-consumer scenario"
else
  bad "A11 parity only" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A12. only the consumer surface declared.
R="$(make_case a12 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$SC_HEAD,\"obligations\":[{\"trait\":\"shared-consumer\",\"requiredProof\":\"parity plus consumer surface\",\"satisfiedBy\":[\"consumer-surface:tests/ui/list.spec.ts\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "'parity:'"; then
  ok "A12 a consumer-surface test alone does not satisfy shared-consumer"
else
  bad "A12 surface only" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P8. GUARD: both halves declared passes.
R="$(make_case p8 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$SC_HEAD,\"obligations\":[{\"trait\":\"shared-consumer\",\"requiredProof\":\"parity plus consumer surface\",\"satisfiedBy\":[\"parity:tests/shared/parity.spec.ts\",\"consumer-surface:tests/ui/list.spec.ts\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P8 both parity and consumer-surface declared passes"
else
  bad "P8 both halves" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P9. BARE-LIST envelope is accepted, not a crash --------------------------
# 5 real downstream manifests ship a top-level list instead of
# {"scenarios":[...]}. Reading only the object form raised
# AttributeError: 'list' object has no attribute 'get' — a traceback, which is
# worse than either verdict because it reads as a broken tool rather than a
# finding. Both envelopes carry identical scenario objects.
R="$(make_case p9 '[{"id":"SCN-001-001","title":"t","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"unit assertion"}]}]')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P9 a bare-list manifest envelope is accepted"
else
  bad "P9 bare-list envelope" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A13. the bare-list envelope is still VALIDATED, not waved through -------
R="$(make_case a13 '[{"id":"SCN-001-001","title":"t","requiredTestType":"unit","behaviorTraits":["pure-calculation"]}]')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'TRAIT-COVERED'; then
  ok "A13 a bare-list manifest is still linted, not skipped"
else
  bad "A13 bare-list still validated" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- IMP-048 SCOPE-5: the return-time ordering contract (EV-12) -------------
#
# A14-A19 are the rules. P10-P12 are their guards, and P12 is the load-bearing
# one: a scenario making NO ordering claim must be completely unaffected. An
# obligation that quietly taxed every scenario would be switched off, and the
# ordering rule would go with it.
#
# The shape being refused is the S5B false green: a test that calls production,
# sleeps, then polls until the condition becomes true passes whether or not
# production honoured the ordering, because the property is sampled AFTER the
# window in which it could be violated.

ORD_HEAD='"behaviorTraits":["mutable-state"],"obligations":[{"trait":"mutable-state"'

# A14. the claim is made, and the proof itself describes polling.
R="$(make_case a14 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the call returns only after the write is durable; the test polls until the row is visible\",\"satisfiedBy\":[\"ordering:at-return\",\"ordering:sentinel\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-SAMPLED-LATE'; then
  ok "A14 an ordering claim whose proof polls before asserting is refused"
else
  bad "A14 polling proof" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A15. the same shape declared as a token rather than described in prose.
R="$(make_case a15 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the call returns only after the write is durable\",\"satisfiedBy\":[\"ordering:at-return\",\"ordering:sentinel\",\"ordering:poll-until\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-SAMPLED-LATE'; then
  ok "A15 a declared late-sampling token is refused, not silently counted"
else
  bad "A15 late token" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A16. an ordering claim naming no ordering proof at all.
R="$(make_case a16 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the handle cannot return before the lease is released\",\"satisfiedBy\":[\"tests/order.spec.ts\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-AT-RETURN'; then
  ok "A16 an ordering claim with no at-return assertion is refused"
else
  bad "A16 no at-return" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A17. the sentinel is owed separately: sampling at return shows the value at
# one instant, the sentinel shows nothing mutated it afterwards.
R="$(make_case a17 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the handle cannot return before the lease is released\",\"satisfiedBy\":[\"ordering:at-return\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-SENTINEL'; then
  ok "A17 a missing delayed-mutation sentinel is refused"
else
  bad "A17 no sentinel" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A18. the contract NAMES a precondition, so every attempt owes an ordering
# proof that it was preceded by the observation.
R="$(make_case a18 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the control is written only after an observed quorum and remains held until finality\",\"satisfiedBy\":[\"ordering:at-return\",\"ordering:sentinel\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-PRECONDITION-ATTEMPTS'; then
  ok "A18 a named precondition owes a recorded-attempt proof"
else
  bad "A18 precondition attempts" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A19. a mistyped ordering token is a finding, not a silently uncounted proof.
R="$(make_case a19 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the call returns only after the write is durable\",\"satisfiedBy\":[\"ordering:at-retrn\",\"ordering:sentinel\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-TOKEN'; then
  ok "A19 a mistyped ordering token is refused"
else
  bad "A19 mistyped ordering token" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P10. the SAME scenario asserting AT return, with a sentinel and no
# pre-assertion sleep or poll, passes. Without this the rule would be a ban on
# ordering claims rather than a contract for proving them.
R="$(make_case p10 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"the call returns only after the write is durable\",\"satisfiedBy\":[\"tests/order.spec.ts\",\"ordering:at-return\",\"ordering:sentinel\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P10 an ordering claim proved at return with a sentinel passes"
else
  bad "P10 ordering satisfied" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P11. the trait route reaches the same obligation as the phrase route, so a
# scenario cannot escape by declaring the trait and writing bland proof prose.
R="$(make_case p11 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",\"behaviorTraits\":[\"return-time-ordering\"],\"obligations\":[{\"trait\":\"return-time-ordering\",\"requiredProof\":\"x\",\"satisfiedBy\":[\"tests/order.spec.ts\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'ORDERING-AT-RETURN'; then
  ok "P11 the declared ordering trait owes the same proof as the phrase"
else
  bad "P11 trait route" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case p11b "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",\"behaviorTraits\":[\"return-time-ordering\"],\"obligations\":[{\"trait\":\"return-time-ordering\",\"requiredProof\":\"x\",\"satisfiedBy\":[\"ordering:at-return\",\"ordering:sentinel\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P11b the declared ordering trait is dischargeable"
else
  bad "P11b trait route satisfied" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P12. GUARD, and the load-bearing one: a scenario making NO ordering claim
# carries NO new burden. Asserted explicitly rather than inferred from the exit
# code, because an ordering finding on an ordinary scenario is the failure mode
# that would get this check switched off.
R="$(make_case p12 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"write, read and persistence round trip\",\"satisfiedBy\":[\"tests/state.spec.ts\"]}]}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]] && [[ "$OUT" != *ORDERING* ]]; then
  ok "P12 a scenario with NO ordering claim is completely unaffected"
else
  bad "P12 no new burden" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P13. GUARD: the registry is the AUTHORITY, not a second copy of a list that
# also lives in the lint. Proved by CHANGING the registry and showing the lint's
# behaviour follows: a grep for a phrase cannot tell an illustrative comment
# apart from an enforced list, but a swapped vocabulary can.
ALT_REG="$WORK/alt-proof-obligations.yaml"
awk '/^  claimPhrases:/ { print "  claimPhrases: [flibberty gibbet]"; next } { print }' \
  "$SCRIPT_DIR/../registry/proof-obligations.yaml" > "$ALT_REG"

ORD_BODY='"requiredProof":"the call returns only after the write is durable","satisfiedBy":["tests/order.spec.ts"]}]}]}'
R="$(make_case p13a "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,$ORD_BODY")"
set +e
ALT_OUT="$(BUBBLES_PROOF_OBLIGATIONS_REGISTRY="$ALT_REG" bash "$TARGET" "$R" --quiet 2>&1)"
ALT_RC=$?
set -e

R="$(make_case p13b "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"integration\",$ORD_HEAD,\"requiredProof\":\"flibberty gibbet is honoured\",\"satisfiedBy\":[\"tests/order.spec.ts\"]}]}]}")"
set +e
ALT2_OUT="$(BUBBLES_PROOF_OBLIGATIONS_REGISTRY="$ALT_REG" bash "$TARGET" "$R" --quiet 2>&1)"
ALT2_RC=$?
set -e

if [[ "$ALT_RC" -eq 0 && "$ALT_OUT" != *ORDERING* ]] &&
  [[ "$ALT2_RC" -eq 1 ]] && printf '%s' "$ALT2_OUT" | grep -q 'ORDERING-AT-RETURN'; then
  ok "P13 the ordering phrase list is READ from the registry, not restated in the lint"
else
  bad "P13 registry authority" "original=$ALT_RC swapped=$ALT2_RC"
fi

# --- U1. usage -------------------------------------------------------------
set +e
bash "$TARGET" >/dev/null 2>&1; u1=$?
bash "$TARGET" "$WORK/absent" >/dev/null 2>&1; u2=$?
bypass="$(bash "$TARGET" --force 2>&1)"; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 missing arg, absent dir and a bypass flag all exit 2"
else
  bad "U1 usage" "noarg=$u1 absent=$u2 bypass=$u3"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
