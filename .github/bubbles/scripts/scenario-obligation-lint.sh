#!/usr/bin/env bash
set -euo pipefail

# scenario-obligation-lint.sh
#
# IMP-040 SCOPE-3 / COV-9 — keep the obligation matrix derived, not enumerated.
#
# WHY THIS EXISTS
# Scenario coverage was row-based: a scenario could carry a unit row while its
# user-visible path had no live-system test at all. The obligation matrix fixes
# that by deriving required proof from the scenario's BEHAVIOR TRAITS.
#
# A derived matrix has one characteristic failure mode, and the proposal names
# it directly: attaching every category to every scenario. That is not a safe
# default. An obligation set that never varies carries no information about the
# scenario it is attached to, and it trains a reviewer to skim a block that is
# always identical — which is how row-based coverage failed in the first place.
#
# WHAT IT CHECKS
#   A. TRAIT-COVERED     — every declared trait owes at least one obligation.
#   B. OBLIGATION-ANCHORED — every obligation names a trait the scenario declared.
#   C. NOT-ENUMERATED    — no scenario declares the ENTIRE trait vocabulary.
#   F. LIVE-PROOF        — a trait the registry marks `liveProof: required`
#                          cannot be discharged by a synthetic path, an absent
#                          mechanism, or the wrong test category (IMP-047 S-D).
#                          An exemption must NAME the absent trait.
#   G. ORDERING          — a scenario making a RETURN-TIME claim owes an
#                          assertion sampled at production return plus a
#                          delayed-mutation sentinel (IMP-048 SCOPE-5).
#
# Check C is deliberately narrow. It fires only on the maximal set, which is
# unambiguous: a scenario that is simultaneously pure calculation, user-visible
# UI, an API contract, mutable state, degraded state, a shared consumer, a
# dependency path, responsive/accessible AND SLA-sensitive is a feature, not a
# scenario. A judgement-based threshold ("too many traits") would reject
# legitimate multi-trait scenarios, and a gate that rejects correct work gets
# switched off.
#
# Check F is where the matrix becomes AUTHORITATIVE (IMP-047 S-D). Persistent
# regression stays universal; the physical test CATEGORY becomes proportionate
# to traits. UI, API, mutable state, dependency boundaries and SLA behavior pay
# MORE than under the old universal-E2E wording; pure logic, docs, static
# metadata and non-runtime config pay less, because a live shell around a pure
# function never proved anything about it. Runtime config gets no docs
# exemption. The rows live in bubbles/registry/proof-obligations.yaml.
#
# Check G extends that SAME derivation to ORDERING traits (IMP-048 SCOPE-5 /
# EV-12) rather than standing up a second obligation engine beside it. A
# scenario asserting "no success returns before finality" is satisfiable today
# by a test that calls production, sleeps, then polls until the condition
# becomes true — which passes whether or not production honoured the ordering,
# because the property is sampled AFTER the window in which it could be
# violated. The trigger is textual, exactly like the SCOPE-6 dependency-boundary
# rule, and the phrase lists and token vocabulary are READ from the registry so
# the matrix stays the single authority.
#
# SAFE TO BLOCK ON DAY ONE, unlike the linked-test resolver: these fields are
# new and optional, so the lint is inert on every packet that does not declare
# them. It cannot retro-break an existing manifest.
#
# Exit codes:
#   0  clean, or nothing declared
#   1  finding
#   2  usage error / unparseable manifest

SPEC_DIR=""
QUIET=0

die_usage() {
  printf 'scenario-obligation-lint: %s\n' "$1" >&2
  printf 'usage: scenario-obligation-lint.sh <specDir> [--quiet]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) QUIET=1 ;;
    -h|--help) sed -n '4,38p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported; derive the matrix instead" ;;
    -*) die_usage "unknown option '$1'" ;;
    *) [[ -z "$SPEC_DIR" ]] || die_usage "unexpected argument '$1'"; SPEC_DIR="$1" ;;
  esac
  shift
done

[[ -n "$SPEC_DIR" ]] || die_usage "a spec directory is required"
[[ -d "$SPEC_DIR" ]] || die_usage "spec directory not found: $SPEC_DIR"

MANIFEST="$SPEC_DIR/scenario-manifest.json"
if [[ ! -f "$MANIFEST" ]]; then
  [[ "$QUIET" -eq 1 ]] || printf '[scenario-obligation-lint] NA — no scenario-manifest.json\n'
  exit 0
fi

command -v python3 >/dev/null 2>&1 || die_usage "python3 is required"

# --- IMP-047 S-D: the trait matrix is AUTHORITATIVE -------------------------
#
# The trait vocabulary and every live-proof obligation are READ from
# bubbles/registry/proof-obligations.yaml. They used to be a literal set in this
# file, which meant the prose matrix in test-core.md and the enforced matrix
# here were two answers to one question, free to drift.
#
# Flattened to `traitId|field|value` lines because the registry is a fixed
# shallow shape, so awk is enough and yq stays an optional convenience rather
# than a hard dependency that would make the check unavailable where it matters.
PROOF_REGISTRY_FILE="${BUBBLES_PROOF_OBLIGATIONS_REGISTRY:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../registry/proof-obligations.yaml}"
[[ -f "$PROOF_REGISTRY_FILE" ]] ||
  die_usage "proof-obligation registry not found: $PROOF_REGISTRY_FILE"

PROOF_REGISTRY="$(awk '
  /^traits:/ {t=1; next}
  /^[a-zA-Z]/ {t=0; id=""}
  t && /^  - id: / {id=$0; sub(/^  - id: /, "", id); next}
  t && id != "" && /^    [a-zA-Z]+:/ {
    line=$0
    sub(/^    /, "", line)
    k=line; sub(/:.*/, "", k)
    v=line; sub(/^[a-zA-Z]+:[ ]*/, "", v)
    if (k == "note" || k == "requiredProof") next
    print id "|" k "|" v
  }
' "$PROOF_REGISTRY_FILE")"

[[ -n "$PROOF_REGISTRY" ]] ||
  die_usage "proof-obligation registry declares no traits: $PROOF_REGISTRY_FILE"

# The ordering block is the SAME registry, read with the same flat idiom. A
# second pass rather than a second file: the phrase lists and the token
# vocabulary belong beside the traits they qualify.
ORDERING_REGISTRY="$(awk '
  /^ordering:/ {o=1; next}
  /^[a-zA-Z]/ {o=0}
  o && /^  [a-zA-Z]+:/ {
    line=$0
    sub(/^  /, "", line)
    k=line; sub(/:.*/, "", k)
    v=line; sub(/^[a-zA-Z]+:[ ]*/, "", v)
    if (v == "") next
    print k "|" v
  }
' "$PROOF_REGISTRY_FILE")"

MANIFEST="$MANIFEST" QUIET="$QUIET" PROOF_REGISTRY="$PROOF_REGISTRY" \
  ORDERING_REGISTRY="$ORDERING_REGISTRY" python3 - <<'PY'
import json, os, sys

manifest_path = os.environ["MANIFEST"]
quiet = os.environ.get("QUIET") == "1"

try:
    with open(manifest_path, encoding="utf-8") as fh:
        manifest = json.load(fh)
except (OSError, ValueError) as exc:
    print(f"scenario-obligation-lint: cannot parse {manifest_path}: {exc}", file=sys.stderr)
    sys.exit(2)

VOCABULARY = set()
TRAIT_RULES = {}


def _flow_list(raw):
    raw = raw.strip()
    if raw.startswith("[") and raw.endswith("]"):
        raw = raw[1:-1]
    return [item.strip() for item in raw.split(",") if item.strip()]


# The trait vocabulary and every live-proof rule come from
# bubbles/registry/proof-obligations.yaml (IMP-047 S-D). Nothing is restated
# here: a second copy of the matrix is a second answer, and the prose matrix in
# test-core.md points at the same file for exactly that reason.
for line in os.environ.get("PROOF_REGISTRY", "").splitlines():
    if line.count("|") < 2:
        continue
    trait_id, field, value = line.split("|", 2)
    VOCABULARY.add(trait_id)
    rule = TRAIT_RULES.setdefault(trait_id, {})
    if field in ("requiredEntrypoints", "requiredAssertionSurfaces",
                 "requiredDependencyPaths", "requiredTestTypes"):
        rule[field] = _flow_list(value)
    else:
        rule[field] = value.strip()

MECHANISM_FIELD_FOR = {
    "requiredEntrypoints": "entrypoint",
    "requiredAssertionSurfaces": "assertionSurface",
    "requiredDependencyPaths": "dependencyPath",
}

# --- IMP-048 SCOPE-5: the ordering contract (EV-12) -------------------------
#
# Read from the SAME registry as the traits. Nothing about ordering is spelled
# out here, for the reason check F already established: a second copy of a
# matrix is a second answer.
ORDERING = {}
for line in os.environ.get("ORDERING_REGISTRY", "").splitlines():
    if "|" not in line:
        continue
    key, value = line.split("|", 1)
    ORDERING[key] = _flow_list(value)

ORDERING_PREFIX = (ORDERING.get("satisfiedByPrefix") or ["ordering"])[0]
CLAIM_PHRASES = [p.lower() for p in ORDERING.get("claimPhrases", []) if p]
PRECONDITION_PHRASES = [p.lower() for p in ORDERING.get("preconditionPhrases", []) if p]
POLLING_PHRASES = [p.lower() for p in ORDERING.get("pollingPhrases", []) if p]
ORDERING_REQUIRED = [t for t in ORDERING.get("requiredTokens", []) if t]
ORDERING_CONDITIONAL = [t for t in ORDERING.get("conditionalTokens", []) if t]
ORDERING_FORBIDDEN = [t for t in ORDERING.get("forbiddenTokens", []) if t]
ORDERING_VOCAB = set(ORDERING_REQUIRED) | set(ORDERING_CONDITIONAL) | set(ORDERING_FORBIDDEN)
ORDERING_TRAIT = "return-time-ordering"


def ordering_code(token):
    """Derive the failure code from the registry token so the two cannot drift."""
    return "ORDERING-" + token.upper()

# --- IMP-040 SCOPE-6: dependency-path coverage (COV-9, COV-10) --------------
#
# A cached read can satisfy a scenario about rendering a value. It cannot
# satisfy a scenario about FRESHNESS, fallback, retry, transport or delta,
# because those are claims about the boundary and a cache-only test never
# reaches the boundary. The words below are how a scenario says it is making
# that kind of claim.
BOUNDARY_BEHAVIOR_TERMS = (
    "fresh", "stale", "fallback", "retry", "transport", "delta", "refresh",
    "revalidat", "expire", "invalidat", "timeout", "offline", "reconnect",
    "degrad", "outage", "unavailab",
)

# Paths that never observe the boundary.
NON_OBSERVING_PATHS = {"cache-only"}
CACHE_FIRST_PATHS = {"cache-only", "synthetic-boundary"}

# The distinct cache-first cases. Declared as `cache-case:<token>` in an
# obligation's satisfiedBy. The list is closed so a typo is a finding rather
# than a silently uncounted case.
#
# The spec says "require separate cases WHEN APPLICABLE", so this does NOT
# demand all five. Demanding a fixed count would fire on scenarios where a case
# genuinely cannot arise, and a gate that reports impossible work is one authors
# learn to wave through. What it demands is that a cache-first scenario NAME the
# cases it claims, from a vocabulary that cannot absorb a typo.
CACHE_FIRST_CASES = {
    "fresh-no-fetch",
    "stale-paints-before-delta",
    "missing-honestly-unavailable",
    "malformed-rejected",
    "delta-changes-result",
}

# --- IMP-040 SCOPE-8: shared-consumer parity (COV-9, REG-8) -----------------
#
# When a feature publishes through a shared adapter, shell, client, serializer
# or renderer, ONE proof is not enough. Owner parity shows the shared code
# produces the same result for the same input and policy; it says nothing about
# whether the consumer surface a user actually meets still renders it. A
# consumer-surface assertion shows the opposite half. Certifying either alone
# is how a shared change passes while a downstream surface is broken.
SHARED_CONSUMER_PROOFS = {
    "parity": "owner parity over the same input and policy",
    "consumer-surface": "the current externally observable consumer surface",
}

findings = []
declared = 0

# Two manifest envelopes exist in the wild: {"scenarios": [...]} and a BARE
# top-level list of the same scenario objects. The grep-based counter this
# replaced could not tell them apart, so both shipped. Reading only the object
# form crashes on the bare list; refusing the bare list would false-reject real
# specs that were certified with it. The scenario objects are identical — only
# the wrapper differs — so normalise and validate both.
scenarios = manifest.get("scenarios") if isinstance(manifest, dict) else manifest
if not isinstance(scenarios, list):
    scenarios = []

for scenario in scenarios:
    if not isinstance(scenario, dict):
        continue
    sid = scenario.get("id") or scenario.get("scenarioId") or "<unidentified-scenario>"
    traits = scenario.get("behaviorTraits")
    obligations = scenario.get("obligations")

    if not isinstance(traits, list) or not traits:
        # Nothing declared: the lint is inert rather than demanding the new
        # fields from packets written before they existed.
        if isinstance(obligations, list) and obligations:
            findings.append((sid, "OBLIGATION-ANCHORED",
                             "obligations are declared but behaviorTraits is empty; "
                             "an obligation must name the trait that implies it"))
        continue

    declared += 1
    trait_set = {t for t in traits if isinstance(t, str)}

    unknown = sorted(trait_set - VOCABULARY)
    if unknown:
        findings.append((sid, "UNKNOWN-TRAIT",
                         f"not in the obligation vocabulary: {', '.join(unknown)}"))

    if trait_set >= VOCABULARY:
        findings.append((sid, "NOT-ENUMERATED",
                         "declares the ENTIRE trait vocabulary. The matrix is derived "
                         "per scenario from the traits it actually has; a set that never "
                         "varies carries no information about the scenario"))

    obligation_traits = set()
    satisfied_by_trait = {}
    if isinstance(obligations, list):
        for ob in obligations:
            if not isinstance(ob, dict):
                continue
            trait = ob.get("trait")
            proof = ob.get("requiredProof")
            if not isinstance(trait, str) or not trait:
                findings.append((sid, "OBLIGATION-ANCHORED", "an obligation declares no trait"))
                continue
            obligation_traits.add(trait)
            named = [e for e in (ob.get("satisfiedBy") or [])
                     if isinstance(e, str) and e.strip()]
            if named:
                satisfied_by_trait.setdefault(trait, []).extend(named)
            if not isinstance(proof, str) or not proof.strip():
                findings.append((sid, "TRAIT-COVERED",
                                 f"obligation for '{trait}' states no requiredProof"))
            if trait not in trait_set:
                findings.append((sid, "OBLIGATION-ANCHORED",
                                 f"obligation names trait '{trait}' which the scenario does not declare"))

    uncovered = sorted(trait_set - obligation_traits)
    if uncovered:
        findings.append((sid, "TRAIT-COVERED",
                         f"declared trait(s) with no obligation: {', '.join(uncovered)}"))

    # --- D. dependency-path coverage (SCOPE-6) ------------------------------
    mech = scenario.get("testMechanism")
    dependency = mech.get("dependencyPath") if isinstance(mech, dict) else None

    # Every cache-case token declared anywhere on this scenario.
    declared_cases = set()
    malformed_cases = set()
    if isinstance(obligations, list):
        for ob in obligations:
            if not isinstance(ob, dict):
                continue
            for entry in ob.get("satisfiedBy") or []:
                if not isinstance(entry, str) or not entry.startswith("cache-case:"):
                    continue
                token = entry.split(":", 1)[1].strip()
                if token in CACHE_FIRST_CASES:
                    declared_cases.add(token)
                else:
                    malformed_cases.add(token)

    if malformed_cases:
        findings.append((sid, "DEPENDENCY-CASE",
                         f"cache-case token(s) not in the vocabulary: "
                         f"{', '.join(sorted(malformed_cases))}. Valid: "
                         f"{', '.join(sorted(CACHE_FIRST_CASES))}"))

    if dependency in NON_OBSERVING_PATHS:
        haystack = " ".join(
            [str(scenario.get("title") or "")]
            + [t for t in (scenario.get("tags") or []) if isinstance(t, str)]
        ).lower()
        named = sorted({term for term in BOUNDARY_BEHAVIOR_TERMS if term in haystack})
        if named:
            findings.append((sid, "DEPENDENCY-BOUNDARY",
                             f"scenario names boundary behavior ({', '.join(named)}) but "
                             f"dependencyPath is '{dependency}'. A cache-only test never "
                             "reaches the boundary it claims to prove; observe the named "
                             "boundary instead"))

    if dependency in CACHE_FIRST_PATHS and "dependency-path" in trait_set and not declared_cases:
        findings.append((sid, "DEPENDENCY-CASE",
                         f"cache-first scenario (dependencyPath '{dependency}') names no "
                         "cache-case in any obligation's satisfiedBy. Declare the applicable "
                         f"case(s) as 'cache-case:<token>' from: "
                         f"{', '.join(sorted(CACHE_FIRST_CASES))}"))

    # --- E. shared-consumer parity (SCOPE-8) --------------------------------
    if "shared-consumer" in trait_set:
        shared_proofs = set()
        for ob in obligations if isinstance(obligations, list) else []:
            if not isinstance(ob, dict) or ob.get("trait") != "shared-consumer":
                continue
            for entry in ob.get("satisfiedBy") or []:
                if not isinstance(entry, str):
                    continue
                for kind in SHARED_CONSUMER_PROOFS:
                    if entry.startswith(kind + ":"):
                        shared_proofs.add(kind)

        missing_proofs = sorted(set(SHARED_CONSUMER_PROOFS) - shared_proofs)
        if missing_proofs:
            detail = "; ".join(f"'{k}:' — {SHARED_CONSUMER_PROOFS[k]}" for k in missing_proofs)
            findings.append((sid, "SHARED-CONSUMER",
                             "a shared-consumer scenario owes BOTH proofs; the "
                             f"shared-consumer obligation names no {detail}"))

        if isinstance(mech, dict) and not (mech.get("productionOwners") or []):
            findings.append((sid, "SHARED-CONSUMER",
                             "a shared-consumer scenario names no productionOwners. The "
                             "controlling code path must be recorded as repository-relative "
                             "paths when no code-index adapter is configured"))

    # --- G. return-time ordering (IMP-048 SCOPE-5 / EV-12) ------------------
    #
    # Triggered by the scenario's OWN words. An ordering claim announces itself
    # in the obligation's requiredProof ("returns only after", "cannot return
    # before", "remains held until"), and a scenario may also declare the trait
    # outright. A scenario doing NEITHER is untouched: ordinary scenarios carry
    # no new burden, which is the condition under which a check like this stays
    # switched on.
    proof_text = " ".join(
        ob.get("requiredProof") for ob in (obligations if isinstance(obligations, list) else [])
        if isinstance(ob, dict) and isinstance(ob.get("requiredProof"), str)
    ).lower()
    claim_hits = [p for p in CLAIM_PHRASES if p in proof_text]

    if claim_hits or ORDERING_TRAIT in trait_set:
        trigger = ", ".join(f"'{p}'" for p in claim_hits) or f"the '{ORDERING_TRAIT}' trait"

        ordering_tokens = set()
        malformed_tokens = set()
        for ob in obligations if isinstance(obligations, list) else []:
            if not isinstance(ob, dict):
                continue
            for entry in ob.get("satisfiedBy") or []:
                if not isinstance(entry, str) or not entry.startswith(ORDERING_PREFIX + ":"):
                    continue
                token = entry.split(":", 1)[1].strip()
                if token in ORDERING_VOCAB:
                    ordering_tokens.add(token)
                else:
                    malformed_tokens.add(token)

        if malformed_tokens:
            findings.append((sid, "ORDERING-TOKEN",
                             f"ordering token(s) not in the vocabulary: "
                             f"{', '.join(sorted(malformed_tokens))}. Valid: "
                             f"{', '.join(sorted(ORDERING_VOCAB))}"))

        # THE FALSE-GREEN SHAPE. A test that sleeps and then polls until the
        # condition becomes true passes whether or not production honoured the
        # ordering, because the property is sampled after the window in which it
        # could be violated. It proves eventual convergence, not the contract.
        late_tokens = sorted(ordering_tokens & set(ORDERING_FORBIDDEN))
        late_phrases = sorted({p for p in POLLING_PHRASES if p in proof_text})
        if late_tokens or late_phrases:
            named = ", ".join(f"'{t}'" for t in late_tokens + late_phrases)
            findings.append((sid, "ORDERING-SAMPLED-LATE",
                             f"the scenario makes an ordering claim ({trigger}) but its proof "
                             f"samples the asserted state after the window ({named}). A test that "
                             "polls until the condition becomes true proves eventual convergence, "
                             "not the ordering contract; sample AT production return instead"))

        for token in ORDERING_REQUIRED:
            if token in ordering_tokens:
                continue
            findings.append((sid, ordering_code(token),
                             f"the scenario makes an ordering claim ({trigger}) but no obligation "
                             f"names '{ORDERING_PREFIX}:{token}' in satisfiedBy. The owed proof is "
                             "an assertion sampled AT production return with no pre-assertion sleep "
                             "or poll, PLUS a delayed-mutation sentinel proven unchanged after a "
                             f"bounded observation. Tokens: {', '.join(sorted(ORDERING_VOCAB))}"))

        # CONDITIONAL. Only a contract that NAMES a precondition owes the
        # attempt ledger, so an ordinary ordering claim is not charged for it.
        precondition_hits = sorted({p for p in PRECONDITION_PHRASES if p in proof_text})
        if precondition_hits:
            for token in ORDERING_CONDITIONAL:
                if token in ordering_tokens:
                    continue
                findings.append((sid, ordering_code(token),
                                 f"the scenario names a precondition ({', '.join(precondition_hits)}) "
                                 f"but no obligation names '{ORDERING_PREFIX}:{token}'. Record every "
                                 "attempt and show each was preceded by the required observation"))

    # --- F. live-proof obligations (IMP-047 S-D) ----------------------------
    #
    # Persistent regression stays universal; the CATEGORY becomes proportionate.
    # A trait whose registry row says `liveProof: required` cannot be discharged
    # by a synthetic test — synthetic may COMPLEMENT it, never replace it. A
    # trait whose row says `not-required` owes proportionate proof and pays for
    # no live shell, which is the half that stops a pure calculation buying a
    # fake E2E.
    #
    # An absent declaration is a finding rather than silence. The whole point of
    # making the matrix authoritative is that "no mechanism declared" stops
    # meaning "no obligation".
    na = scenario.get("liveProofNotApplicable")
    if na is not None:
        na_absent = na.get("absentTrait") if isinstance(na, dict) else None
        na_reason = na.get("reason") if isinstance(na, dict) else None
        if not isinstance(na_absent, str) or not na_absent.strip() \
                or not isinstance(na_reason, str) or not na_reason.strip():
            findings.append((sid, "NA-MALFORMED",
                             "liveProofNotApplicable must name an absentTrait AND a reason. "
                             "An unnamed exemption is indistinguishable from an omission"))
        elif na_absent not in VOCABULARY:
            findings.append((sid, "NA-MALFORMED",
                             f"liveProofNotApplicable names '{na_absent}', which is not a "
                             "trait in the proof-obligation registry"))
        elif na_absent in trait_set:
            findings.append((sid, "NA-CONTRADICTS-TRAIT",
                             f"liveProofNotApplicable names '{na_absent}' while the scenario "
                             "declares it. A declared trait owes its proof"))
        elif unknown:
            findings.append((sid, "NA-UNKNOWN-TRAIT",
                             f"a scenario carrying unknown trait(s) ({', '.join(unknown)}) "
                             "cannot claim a live-proof exemption. An unknown trait requires "
                             "review; it never buys a discount"))

    for trait in sorted(trait_set & VOCABULARY):
        rule = TRAIT_RULES.get(trait, {})
        if rule.get("liveProof") != "required":
            continue

        required_types = rule.get("requiredTestTypes")
        if required_types:
            actual_type = scenario.get("requiredTestType")
            if actual_type not in required_types:
                findings.append((sid, "LIVE-PROOF-CATEGORY",
                                 f"trait '{trait}' owes {' or '.join(required_types)} proof "
                                 f"but requiredTestType is '{actual_type}'"))

        # A trait whose obligation NAMES the test that discharges it has said
        # how the proof is obtained. Whether that test is really the category it
        # claims is scenario-test-resolve.sh's question, not this lint's — two
        # checks answering one question is the drift this slice removes.
        if satisfied_by_trait.get(trait):
            continue

        mech_rules = {k: v for k, v in rule.items() if k in MECHANISM_FIELD_FOR and v}
        if not mech_rules:
            continue

        if not isinstance(mech, dict) or not mech:
            findings.append((sid, "LIVE-PROOF-UNDECLARED",
                             f"trait '{trait}' owes live proof but the scenario names no "
                             "satisfiedBy test and declares no testMechanism. Name the test "
                             "that supplies the live proof, declare the mechanism, or declare "
                             "liveProofNotApplicable naming the absent trait"))
            continue

        for key, allowed in mech_rules.items():
            field = MECHANISM_FIELD_FOR[key]
            actual = mech.get(field)
            if actual not in allowed:
                findings.append((sid, "LIVE-PROOF-SUBSTITUTED",
                                 f"trait '{trait}' requires {field} in "
                                 f"({', '.join(allowed)}) but the declared mechanism is its "
                                 f"only named proof and uses '{actual}'. A synthetic path may "
                                 "complement live proof; it cannot replace it"))

if findings:
    print("scenario-obligation-lint: FAIL — obligation matrix is not coherent (COV-9)", file=sys.stderr)
    for sid, code, detail in findings:
        print(f"  {code}: {sid}", file=sys.stderr)
        print(f"    {detail}", file=sys.stderr)
    print("", file=sys.stderr)
    print(f"scenario-obligation-lint: {len(findings)} finding(s).", file=sys.stderr)
    sys.exit(1)

if not quiet:
    if declared:
        print(f"[scenario-obligation-lint] OK — {declared} scenario(s) with a coherent derived obligation matrix")
    else:
        print("[scenario-obligation-lint] OK — no behaviorTraits declared (inert)")
sys.exit(0)
PY
