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
R="$(make_case p2 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui","dependency-path"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion on the production route"},{"trait":"dependency-path","requiredProof":"stale-cache boundary observation"}]}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P2 a coherent two-trait matrix passes"
else
  bad "P2 coherent matrix" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. scenarios may legitimately differ in trait count -------------------
R="$(make_case p3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"a","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"obligations":[{"trait":"pure-calculation","requiredProof":"assertion over transformed output"}]},{"id":"SCN-001-002","title":"b","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui","sla-sensitive"],"obligations":[{"trait":"user-visible-ui","requiredProof":"visible assertion"},{"trait":"sla-sensitive","requiredProof":"stress assertion against the threshold"}]}]}')"
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
# The whole point of SCOPE-3: derived, not enumerated.
ALL='"pure-calculation","user-visible-ui","api-contract","mutable-state","degraded-state","shared-consumer","dependency-path","responsive-accessible","sla-sensitive"'
OBS='{"trait":"pure-calculation","requiredProof":"x"},{"trait":"user-visible-ui","requiredProof":"x"},{"trait":"api-contract","requiredProof":"x"},{"trait":"mutable-state","requiredProof":"x"},{"trait":"degraded-state","requiredProof":"x"},{"trait":"shared-consumer","requiredProof":"x"},{"trait":"dependency-path","requiredProof":"x"},{"trait":"responsive-accessible","requiredProof":"x"},{"trait":"sla-sensitive","requiredProof":"x"}'
R="$(make_case a4 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[$ALL],\"obligations\":[$OBS]}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'NOT-ENUMERATED'; then
  ok "A4 a scenario declaring the ENTIRE vocabulary is refused"
else
  bad "A4 enumeration refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. eight of nine traits is NOT flagged --------------------------------
# Guards A4 against becoming a judgement threshold. A genuinely multi-trait
# scenario must pass; only the maximal set is unambiguous enumeration.
ALL8='"pure-calculation","user-visible-ui","api-contract","mutable-state","degraded-state","shared-consumer","dependency-path","responsive-accessible"'
OBS8='{"trait":"pure-calculation","requiredProof":"x"},{"trait":"user-visible-ui","requiredProof":"x"},{"trait":"api-contract","requiredProof":"x"},{"trait":"mutable-state","requiredProof":"x"},{"trait":"degraded-state","requiredProof":"x"},{"trait":"shared-consumer","requiredProof":"x","satisfiedBy":["parity:t/p.spec.ts","consumer-surface:t/c.spec.ts"]},{"trait":"dependency-path","requiredProof":"x"},{"trait":"responsive-accessible","requiredProof":"x"}'
R="$(make_case p4 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[$ALL8],\"obligations\":[$OBS8]}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P4 eight of nine traits passes (A4 is not a judgement threshold)"
else
  bad "P4 eight traits pass" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
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
