#!/usr/bin/env bash
set -euo pipefail

# test-mechanism-lint.sh
#
# IMP-040 SCOPE-4 / COV-10 — a test's CATEGORY does not prove its PATH.
#
# WHY THIS EXISTS
# An `e2e-ui` test can assert against hidden legacy DOM, or invoke a render
# function directly, and still carry the label. Both shapes bypass the current
# user path while reporting as end-to-end coverage. The label is a claim about
# intent; `testMechanism` is a claim about mechanism, and unlike prose it is
# checkable because the first four fields are closed vocabularies.
#
# WHAT IT CHECKS
#   A. VOCABULARY   — the four mechanism fields hold declared values only.
#   B. COMPLETENESS — a declared mechanism states its productionOwners and its
#                     negativeControl. A test with no stated perturbation has
#                     not been shown to be sensitive to what it claims.
#   C. COHERENCE    — the declared mechanism does not contradict the scenario's
#                     own behaviorTraits. These are the "distinguishes valid
#                     claims" rules from the proposal:
#                       hidden-dom / internal-state cannot prove user-visible-ui
#                       detached-renderer cannot prove route integration
#                       synthetic input cannot prove live acquisition
#
# Check C is what gives this teeth. A and B are shape checks a careless author
# trips by accident; C catches the substitution an author makes deliberately
# because the test was easier to write that way.
#
#   E. EXECUTED    — IMP-048 SCOPE-4 / EV-11. A and B..D check that a
#                    DECLARATION exists. A `riskTier: high` scenario declaring
#                    `negativeControlMechanism: mutation` also owes proof the
#                    mutation RAN, which this file cannot answer because the
#                    answer lives in an execution receipt rather than in the
#                    manifest. Delegated to `mutation-receipt.sh check` so the
#                    declaration rules and the execution rules stay one
#                    implementation each. Inert unless the repository has
#                    configured `mutationExecution:`, which no repository does
#                    by default.
#
# SAFE TO BLOCK on day one: testMechanism is a new optional field, so the lint
# is inert on every packet that does not declare one.
#
# Exit codes:
#   0  clean, or nothing declared
#   1  finding
#   2  usage error / unparseable manifest

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_DIR=""
REPO_ROOT="$PWD"
QUIET=0

die_usage() {
  printf 'test-mechanism-lint: %s\n' "$1" >&2
  printf 'usage: test-mechanism-lint.sh <specDir> [--repo-root PATH] [--quiet]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) QUIET=1 ;;
    --repo-root)
      [[ $# -ge 2 ]] || die_usage "--repo-root requires a value"
      REPO_ROOT="$2"; shift ;;
    -h|--help) sed -n '4,48p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported; declare the real mechanism instead" ;;
    -*) die_usage "unknown option '$1'" ;;
    *) [[ -z "$SPEC_DIR" ]] || die_usage "unexpected argument '$1'"; SPEC_DIR="$1" ;;
  esac
  shift
done

[[ -n "$SPEC_DIR" ]] || die_usage "a spec directory is required"
[[ -d "$SPEC_DIR" ]] || die_usage "spec directory not found: $SPEC_DIR"

[[ -d "$REPO_ROOT" ]] || die_usage "repo root not found: $REPO_ROOT"

MANIFEST="$SPEC_DIR/scenario-manifest.json"
if [[ ! -f "$MANIFEST" ]]; then
  [[ "$QUIET" -eq 1 ]] || printf '[test-mechanism-lint] NA — no scenario-manifest.json\n'
  exit 0
fi

command -v python3 >/dev/null 2>&1 || die_usage "python3 is required"

REFERENCE_READER="$SCRIPT_DIR/scenario-reference-reader.py"
[[ -f "$REFERENCE_READER" ]] || die_usage "scenario-reference-reader.py is required"
manifest_state() {
    python3 - "$1" <<'PY'
import hashlib
import os
import stat
import sys

path = sys.argv[1]
try:
        before = os.lstat(path)
        if stat.S_ISLNK(before.st_mode) or not stat.S_ISREG(before.st_mode):
                raise OSError("pathname is not a regular non-symlink file")
        with open(path, "rb") as fh:
                data = fh.read()
                opened = os.fstat(fh.fileno())
        after = os.lstat(path)
except OSError as exc:
        print(f"test-mechanism-lint: cannot establish manifest identity: {exc}", file=sys.stderr)
        sys.exit(2)

identity = (before.st_dev, before.st_ino)
if identity != (opened.st_dev, opened.st_ino) or identity != (after.st_dev, after.st_ino):
        print("test-mechanism-lint: manifest pathname identity changed while it was read", file=sys.stderr)
        sys.exit(2)
print(f"{before.st_dev}:{before.st_ino}:{hashlib.sha256(data).hexdigest()}")
PY
}

refuse_manifest_change() {
    local phase="$1" observed="$2" required="$3"
    printf '[test-mechanism-lint] REFUSED — scenario manifest changed during validation\n' >&2
    printf '  phase: %s\n' "$phase" >&2
    printf '  observed: %s\n' "$observed" >&2
    printf '  required: %s\n' "$required" >&2
    exit 2
}

require_manifest_state() {
    local path="$1" expected="$2" phase="$3" observed
    observed="$(manifest_state "$path" 2>&1)" || refuse_manifest_change "$phase" "$observed" "$expected"
    [[ "$observed" == "$expected" ]] || refuse_manifest_change "$phase" "$observed" "$expected"
}

BASELINE_STATE="$(manifest_state "$MANIFEST")" || exit 2
SNAPSHOT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/test-mechanism-manifest.XXXXXXXX")" || die_usage "cannot create stable manifest snapshot"
SNAPSHOT_MANIFEST="$SNAPSHOT_DIR/scenario-manifest.json"
cp "$MANIFEST" "$SNAPSHOT_MANIFEST" || die_usage "cannot copy scenario manifest into stable snapshot"
SNAPSHOT_STATE="$(manifest_state "$SNAPSHOT_MANIFEST")" || exit 2
[[ "${SNAPSHOT_STATE##*:}" == "${BASELINE_STATE##*:}" ]] || refuse_manifest_change "snapshot-capture" "$SNAPSHOT_STATE" "$BASELINE_STATE"
require_manifest_state "$MANIFEST" "$BASELINE_STATE" "snapshot-capture"
PROJECTION_FILE="$(mktemp "${TMPDIR:-/tmp}/test-mechanism-lint.XXXXXXXX")" || die_usage "cannot create scenario projection"
cleanup() { rm -f "$PROJECTION_FILE"; rm -rf "$SNAPSHOT_DIR"; }
trap cleanup EXIT INT TERM
if ! python3 "$REFERENCE_READER" --stdin --repo-root "$REPO_ROOT" <"$SNAPSHOT_MANIFEST" >"$PROJECTION_FILE"; then
  exit 2
fi
require_manifest_state "$SNAPSHOT_MANIFEST" "$SNAPSHOT_STATE" "after-canonical-projection"
require_manifest_state "$MANIFEST" "$BASELINE_STATE" "after-canonical-projection"

PROJECTION_FILE="$PROJECTION_FILE" QUIET="$QUIET" python3 - <<'PY'
import json, os, sys

projection_path = os.environ["PROJECTION_FILE"]
quiet = os.environ.get("QUIET") == "1"

try:
    with open(projection_path, encoding="utf-8") as fh:
        projection = json.load(fh)
except (OSError, ValueError) as exc:
    print(f"test-mechanism-lint: cannot parse scenario reference projection: {exc}", file=sys.stderr)
    sys.exit(2)

VOCAB = {
    "entrypoint": {"production-route", "production-api", "production-cli",
                   "public-function", "detached-renderer", "internal-helper"},
    "inputOrigin": {"live-provider", "ephemeral-real", "seeded-store",
                    "synthetic-cache", "synthetic-fixture", "recorded-fixture"},
    "assertionSurface": {"visible-ui", "accessibility-tree", "http-response",
                         "persisted-state", "returned-value", "hidden-dom", "internal-state"},
    "dependencyPath": {"not-applicable", "ephemeral-real", "same-origin-real",
                       "external-live", "synthetic-boundary", "cache-only"},
}

# An externally observable surface. hidden-dom and internal-state are absent
# deliberately: they observe an internal projection, not what a user sees.
EXTERNAL_SURFACES = {"visible-ui", "accessibility-tree"}
SYNTHETIC_INPUTS = {"synthetic-cache", "synthetic-fixture", "recorded-fixture", "seeded-store"}
LIVE_PATHS = {"external-live", "live-provider"}

findings = []
declared = 0

scenarios = projection.get("scenarios") if isinstance(projection, dict) else None
if not isinstance(scenarios, list):
    print("test-mechanism-lint: scenario reference projection has no scenarios array", file=sys.stderr)
    sys.exit(2)

for scenario in scenarios:
    if not isinstance(scenario, dict):
        print("test-mechanism-lint: scenario reference projection contains a non-object scenario", file=sys.stderr)
        sys.exit(2)
    sid = scenario.get("scenarioId") or "<unidentified-scenario>"
    metadata = scenario.get("metadata")
    if not isinstance(metadata, dict):
        print(f"test-mechanism-lint: scenario {sid} projection has no metadata object", file=sys.stderr)
        sys.exit(2)
    mech = scenario.get("testMechanism")
    if not isinstance(mech, dict) or not mech:
        continue
    declared += 1

    traits = metadata.get("behaviorTraits")
    trait_set = {t for t in traits if isinstance(t, str)} if isinstance(traits, list) else set()

    # --- A. vocabulary ------------------------------------------------------
    for field, allowed in VOCAB.items():
        value = mech.get(field)
        if value is None:
            findings.append((sid, "VOCABULARY", f"testMechanism is missing required field '{field}'"))
            continue
        if not isinstance(value, str) or value not in allowed:
            findings.append((sid, "VOCABULARY",
                             f"testMechanism.{field} = '{value}' is not in the closed vocabulary "
                             f"({', '.join(sorted(allowed))})"))

    # --- B. completeness ----------------------------------------------------
    owners = mech.get("productionOwners")
    if not isinstance(owners, list) or not [o for o in owners if isinstance(o, str) and o.strip()]:
        findings.append((sid, "COMPLETENESS",
                         "testMechanism declares no productionOwners; the production code that "
                         "computes the asserted result must be named"))
    negative = mech.get("negativeControl")
    if not isinstance(negative, str) or not negative.strip():
        findings.append((sid, "COMPLETENESS",
                         "testMechanism declares no negativeControl; a test with no stated "
                         "perturbation has not been shown to be sensitive to what it claims"))

    entrypoint = mech.get("entrypoint")
    surface = mech.get("assertionSurface")
    origin = mech.get("inputOrigin")
    dependency = mech.get("dependencyPath")

    # --- C. coherence with the scenario's own traits ------------------------
    if "user-visible-ui" in trait_set:
        if surface in VOCAB["assertionSurface"] and surface not in EXTERNAL_SURFACES:
            findings.append((sid, "COHERENCE",
                             f"scenario declares user-visible-ui but asserts against '{surface}'. "
                             "That proves an internal projection, not a visible outcome"))
        if entrypoint == "detached-renderer":
            findings.append((sid, "COHERENCE",
                             "scenario declares user-visible-ui but enters through a detached "
                             "renderer. That proves a renderer unit, not route integration"))
        if entrypoint == "internal-helper":
            findings.append((sid, "COHERENCE",
                             "scenario declares user-visible-ui but enters through an internal "
                             "helper, bypassing the path a user takes"))

    if origin in SYNTHETIC_INPUTS and dependency in LIVE_PATHS:
        findings.append((sid, "COHERENCE",
                         f"inputOrigin '{origin}' is synthetic but dependencyPath claims "
                         f"'{dependency}'. Synthetic input cannot prove live acquisition "
                         "without a real boundary observation"))

    if "api-contract" in trait_set and surface in {"internal-state", "returned-value"}:
        findings.append((sid, "COHERENCE",
                         f"scenario declares api-contract but asserts against '{surface}'. "
                         "A wire contract needs an externally observable response"))

    # SCOPE-8. The shared-consumer half that a shared change most often breaks
    # is the CONSUMER surface, so an internal observation cannot stand in for
    # it: an attached hidden legacy node is not the visible current surface, and
    # a manual renderer call is not the route that owns rendering.
    if "shared-consumer" in trait_set:
        if surface in {"hidden-dom", "internal-state"}:
            findings.append((sid, "COHERENCE",
                             f"scenario declares shared-consumer but asserts against "
                             f"'{surface}'. A hidden legacy node cannot substitute for the "
                             "visible current consumer surface"))
        if entrypoint == "detached-renderer":
            findings.append((sid, "COHERENCE",
                             "scenario declares shared-consumer but enters through a "
                             "detached renderer. A manual renderer invocation cannot "
                             "substitute for the route that owns rendering"))

    # --- D. non-vacuity: the negative control matches the risk (SCOPE-7) ----
    # Ranked weakest to strongest. A negative control that is weaker than the
    # stakes has not shown the test is sensitive to what it claims.
    RANK = {"adversarial-input": 1, "perturbed-input": 2, "mutation": 3}
    TIER_MINIMUM = {"low": 1, "medium": 2, "high": 3}

    tier = metadata.get("riskTier")
    ncm = mech.get("negativeControlMechanism")
    fallback = mech.get("negativeControlFallbackReason")

    if tier is not None and tier not in TIER_MINIMUM:
        findings.append((sid, "VOCABULARY",
                         f"riskTier = '{tier}' is not one of low, medium, high"))
    elif tier is not None:
        if ncm is None:
            findings.append((sid, "NON-VACUITY",
                             f"scenario declares riskTier '{tier}' but no "
                             "negativeControlMechanism. The tier sets how strong the "
                             "control must be, so it has to say which one it used"))
        elif ncm not in RANK:
            findings.append((sid, "VOCABULARY",
                             f"negativeControlMechanism = '{ncm}' is not one of "
                             f"{', '.join(sorted(RANK, key=RANK.get))}"))
        elif RANK[ncm] < TIER_MINIMUM[tier]:
            # Not blocked outright: a project with no mutation tooling is
            # expected to use a weaker control. It just has to SAY SO, so a
            # deliberate fallback is distinguishable from a silent downgrade.
            if not isinstance(fallback, str) or not fallback.strip():
                findings.append((sid, "NON-VACUITY",
                                 f"riskTier '{tier}' needs at least "
                                 f"'{sorted(RANK, key=RANK.get)[TIER_MINIMUM[tier] - 1]}' "
                                 f"but the control is '{ncm}'. Strengthen it, or state a "
                                 "negativeControlFallbackReason naming why it cannot be"))

    # A control that merely restates the scenario is the renamed-label shape the
    # proposal calls out: it duplicates the positive fixture instead of
    # perturbing anything.
    title = scenario.get("title")
    if isinstance(negative, str) and isinstance(title, str):
        norm = lambda s: " ".join(s.lower().split())
        if norm(negative) and norm(negative) == norm(title):
            findings.append((sid, "NON-VACUITY",
                             "negativeControl restates the scenario title verbatim. A "
                             "control has to name a perturbation, not relabel the "
                             "positive case"))

if findings:
    print("test-mechanism-lint: FAIL — declared mechanism does not support the claim (COV-10)", file=sys.stderr)
    for sid, code, detail in findings:
        print(f"  {code}: {sid}", file=sys.stderr)
        print(f"    {detail}", file=sys.stderr)
    print("", file=sys.stderr)
    print(f"test-mechanism-lint: {len(findings)} finding(s).", file=sys.stderr)
    sys.exit(1)

if not quiet:
    if declared:
        print(f"[test-mechanism-lint] OK — {declared} declared mechanism(s) coherent with their scenario traits")
    else:
        print("[test-mechanism-lint] OK — no testMechanism declared (inert)")
sys.exit(0)
PY

# --- E. the declared mutation actually RAN (IMP-048 SCOPE-4 / EV-11) -------
#
# Delegated, never duplicated. This file owns the DECLARATION; the receipt store
# owns the EXECUTION, and a second reader of that store would be a second answer
# to one question. Guarded on presence so a partial installation degrades to
# today's behaviour rather than to a broken tool.
COMPANION="$SCRIPT_DIR/mutation-receipt.sh"
if [[ -f "$COMPANION" ]]; then
    companion_rc=0
    require_manifest_state "$SNAPSHOT_MANIFEST" "$SNAPSHOT_STATE" "before-mutation-receipt"
    require_manifest_state "$MANIFEST" "$BASELINE_STATE" "before-mutation-receipt"
  if [[ "$QUIET" -eq 1 ]]; then
        bash "$COMPANION" check --spec-dir "$SNAPSHOT_DIR" --repo-root "$REPO_ROOT" --quiet || companion_rc=$?
  else
        bash "$COMPANION" check --spec-dir "$SNAPSHOT_DIR" --repo-root "$REPO_ROOT" || companion_rc=$?
  fi
    require_manifest_state "$SNAPSHOT_MANIFEST" "$SNAPSHOT_STATE" "after-mutation-receipt"
    require_manifest_state "$MANIFEST" "$BASELINE_STATE" "after-mutation-receipt"
    [[ "$companion_rc" -eq 0 ]] || exit "$companion_rc"
fi
exit 0
