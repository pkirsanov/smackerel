#!/usr/bin/env bash
# bubbles/scripts/test-mechanism-lint-selftest.sh
#
# Hermetic selftest for test-mechanism-lint.sh (IMP-040 SCOPE-4 / COV-10).
#
# The load-bearing cases are the COHERENCE ones (A5-A8). Vocabulary and
# completeness catch a careless author; coherence catches the substitution an
# author makes deliberately because the easier test was the one that did not
# prove the claim — asserting hidden DOM for a visible-UI scenario, calling a
# renderer directly instead of the route, or calling seeded cache data proof of
# live acquisition.
#
# P3 and P4 are their guards: the same mechanism values must PASS when the
# scenario's traits do not contradict them, so the rules stay coherence checks
# rather than a blanket ban on internal observation.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/test-mechanism-lint.sh"
NAME="test-mechanism-lint-selftest"

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

# A complete, coherent mechanism used as the base for single-field mutations.
GOOD_MECH='"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["src/render/page.ts"],"negativeControl":"wrong route renders nothing"}'

# --- P1. inert when nothing is declared -------------------------------------
R="$(make_case p1 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui"}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P1 a manifest with no testMechanism is inert"
else
  bad "P1 inert" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P2. a coherent mechanism passes ----------------------------------------
R="$(make_case p2 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"behaviorTraits\":[\"user-visible-ui\"],$GOOD_MECH}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P2 a coherent mechanism passes"
else
  bad "P2 coherent mechanism" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: value outside the closed vocabulary -------------------
R="$(make_case a1 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","testMechanism":{"entrypoint":"somewhere-nice","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'VOCABULARY'; then
  ok "A1 a value outside the closed vocabulary is refused"
else
  bad "A1 bad vocabulary" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: a missing mechanism field -----------------------------
R="$(make_case a2 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","testMechanism":{"entrypoint":"production-route","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "missing required field 'inputOrigin'"; then
  ok "A2 a missing mechanism field is refused and named"
else
  bad "A2 missing field" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A3/A4. ADVERSARIAL: incomplete declarations ----------------------------
R="$(make_case a3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'no productionOwners'; then
  ok "A3 a mechanism naming no productionOwners is refused"
else
  bad "A3 no owners" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case a4 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["a.ts"]}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'no negativeControl'; then
  ok "A4 a mechanism with no negativeControl is refused"
else
  bad "A4 no negative control" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: hidden DOM presented as visible-UI proof --------------
R="$(make_case a5 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"hidden-dom","dependencyPath":"cache-only","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'COHERENCE'; then
  ok "A5 hidden-dom cannot prove a user-visible-ui scenario"
else
  bad "A5 hidden dom" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL: detached renderer for a route scenario ----------------
R="$(make_case a6 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"testMechanism":{"entrypoint":"detached-renderer","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'renderer unit, not route integration'; then
  ok "A6 a detached renderer cannot prove route integration"
else
  bad "A6 detached renderer" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A7. ADVERSARIAL: synthetic input claiming live acquisition -------------
R="$(make_case a7 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"integration","testMechanism":{"entrypoint":"production-api","inputOrigin":"synthetic-cache","assertionSurface":"http-response","dependencyPath":"external-live","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'cannot prove live acquisition'; then
  ok "A7 synthetic input claiming a live dependency path is refused"
else
  bad "A7 synthetic vs live" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A8. ADVERSARIAL: api-contract proved by an internal value --------------
R="$(make_case a8 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-api","behaviorTraits":["api-contract"],"testMechanism":{"entrypoint":"public-function","inputOrigin":"seeded-store","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'externally observable response'; then
  ok "A8 an api-contract proved by a returned value is refused"
else
  bad "A8 api contract" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. hidden-dom is fine when the scenario is NOT user-visible-ui --------
# Guards A5: the rule is coherence with the declared trait, not a ban on
# observing internal state.
R="$(make_case p3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"unit","behaviorTraits":["pure-calculation"],"testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/calc.ts"],"negativeControl":"perturbed input changes the result"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P3 an internal surface passes when the trait does not contradict it"
else
  bad "P3 internal surface allowed" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. a MIXED test passes: internal entry, external assertion ------------
# The proposal accepts mixed tests; only the SOLE-proof shape is rejected.
R="$(make_case p4 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui","dependency-path"],"testMechanism":{"entrypoint":"production-route","inputOrigin":"synthetic-cache","assertionSurface":"accessibility-tree","dependencyPath":"cache-only","productionOwners":["src/page.ts"],"negativeControl":"empty cache renders the unavailable state"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P4 a mixed mechanism with an external assertion passes"
else
  bad "P4 mixed mechanism" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- IMP-040 SCOPE-7: non-vacuity and mutation proof ------------------------
#
# A9-A12 are the rules. P5-P7 are their guards. The important guard is P6: a
# project with no mutation tooling must still be able to ship a high-risk
# scenario by NAMING the fallback. Without that, the tier would be unusable
# outside languages with mutation runners and teams would simply stop declaring
# riskTier, which removes the signal rather than strengthening it.

MECH_CORE='"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"cache-only","productionOwners":["a.ts"]'

# A9. a tier declared with no mechanism.
R="$(make_case a9 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"riskTier\":\"medium\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"empty input refuses\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'NON-VACUITY'; then
  ok "A9 a riskTier with no negativeControlMechanism is refused"
else
  bad "A9 tier without mechanism" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A10. a control weaker than the tier, with no stated reason.
R="$(make_case a10 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"riskTier\":\"high\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"empty input refuses\",\"negativeControlMechanism\":\"adversarial-input\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'NON-VACUITY'; then
  ok "A10 a control weaker than its tier is refused"
else
  bad "A10 weak control" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A11. a control that just restates the scenario title.
R="$(make_case a11 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"Cached price renders\",\"requiredTestType\":\"e2e-ui\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"Cached price renders\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'restates the scenario title'; then
  ok "A11 a control restating the title is refused as a renamed label"
else
  bad "A11 renamed label" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# A12. an out-of-vocabulary tier.
R="$(make_case a12 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"riskTier\":\"critical\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"empty input refuses\",\"negativeControlMechanism\":\"mutation\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'riskTier'; then
  ok "A12 a tier outside low/medium/high is refused"
else
  bad "A12 bad tier" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P5. a control that meets its tier passes.
R="$(make_case p5 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"riskTier\":\"medium\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"perturbing the seeded price changes the rendered total\",\"negativeControlMechanism\":\"perturbed-input\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P5 a control meeting its tier passes"
else
  bad "P5 tier met" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P6. GUARD: a named fallback lets a project without mutation tooling ship.
R="$(make_case p6 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"riskTier\":\"high\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"perturbing the rate changes the total\",\"negativeControlMechanism\":\"perturbed-input\",\"negativeControlFallbackReason\":\"no mutation adapter is configured for this repository\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P6 a named fallback lets a project without mutation tooling ship"
else
  bad "P6 named fallback" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# P7. GUARD: the tier rules stay inert when no riskTier is declared.
R="$(make_case p7 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",\"testMechanism\":{$MECH_CORE,\"negativeControl\":\"empty input refuses\"}}]}")"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P7 the tier rules are inert when no riskTier is declared"
else
  bad "P7 tier inert" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- IMP-040 SCOPE-8: shared-consumer surface substitution ------------------

R="$(make_case a13 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"integration","behaviorTraits":["shared-consumer"],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"hidden-dom","dependencyPath":"not-applicable","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'hidden legacy node'; then
  ok "A13 a hidden node cannot substitute for the consumer surface"
else
  bad "A13 hidden consumer" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case a14 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"integration","behaviorTraits":["shared-consumer"],"testMechanism":{"entrypoint":"detached-renderer","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"not-applicable","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'route that owns rendering'; then
  ok "A14 a manual renderer call cannot substitute for the owning route"
else
  bad "A14 detached consumer" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case p8 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"integration","behaviorTraits":["shared-consumer"],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"visible-ui","dependencyPath":"not-applicable","productionOwners":["a.ts"],"negativeControl":"x"}}]}')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P8 a shared-consumer scenario on the real route and surface passes"
else
  bad "P8 shared consumer ok" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P9/A15. BARE-LIST envelope --------------------------------------------
# Real downstream manifests ship a top-level list. Reading only the object form
# raised AttributeError, which reads as a broken tool rather than a finding.
R="$(make_case p9 '[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui"}]')"
run_lint "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P9 a bare-list manifest envelope is accepted"
else
  bad "P9 bare-list envelope" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case a15 '[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","behaviorTraits":["user-visible-ui"],"testMechanism":{"entrypoint":"production-route","inputOrigin":"seeded-store","assertionSurface":"hidden-dom","dependencyPath":"cache-only","productionOwners":["a.ts"],"negativeControl":"x"}}]')"
run_lint "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'COHERENCE'; then
  ok "A15 a bare-list manifest is still linted, not skipped"
else
  bad "A15 bare-list still validated" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- U1. usage -------------------------------------------------------------
set +e
bash "$TARGET" >/dev/null 2>&1; u1=$?
bash "$TARGET" "$WORK/absent" >/dev/null 2>&1; u2=$?
bypass="$(bash "$TARGET" --skip-mechanism 2>&1)"; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 missing arg, absent dir and a bypass flag all exit 2"
else
  bad "U1 usage" "noarg=$u1 absent=$u2 bypass=$u3"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
