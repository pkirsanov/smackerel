#!/usr/bin/env bash
# Deploy-Manifest Assurance Lint (IMP-100 Phase 3 chokes #4/#5 — framework contract half)
# ---------------------------------------------------------------------------
# Generic, TARGET-AGNOSTIC validator for the assurance attestation a signed
# deploy manifest carries (choke #4) and the adapter-preflight refusal it drives
# (choke #5). Given a deploy manifest (e.g. deploy/<target>/manifest.yaml) it:
#   - reads attestations.assurance.{level, profile, evidenceDigest}
#   - validates level ∈ {full, fast, prototype} and evidenceDigest is present
#   - ALWAYS refuses `prototype` (never deployable — the R5 invariant)
#   - when --minimum-assurance is supplied, consults the SHARED decision in
#     assurance-resolve.sh (never re-derives here) and refuses an under-assured
#     level; an optional --risk-class is forwarded so high/unknown risk escalates
#     the floor to `full`.
#
# The CONCRETE deployment adapter (knb-owned per the deployment boundary) CALLS
# this primitive inside its preflight; this framework script owns only the
# generic, host-agnostic decision — exactly like assurance-resolve.sh. It has no
# default surface and is never pointed at a framework file: the caller supplies
# the manifest path (hermetic fixtures in the selftest).
#
# BACKWARD-COMPATIBLE on absence, fail-CLOSED whenever assurance is MANDATORY
# (IMP-047 PD-07):
#   - manifest with no attestations.assurance block → no-op (exit 0)
#     (existing pre-assurance manifests never break)
#   - yq not installed, assurance OPTIONAL              → WARN-and-skip (exit 0)
#   - yq not installed, --require-assurance             → REFUSE (exit 1)
# A PRESENT assurance block is validated in full. There is NO --skip/--force flag.
#
# PD-07, the two defects this file used to carry:
#
#   1. FAIL-OPEN UNDER `require`. The parser gate exited 0 when `yq` was absent
#      EVEN under --require-assurance. A mandatory check that passes because its
#      tool is missing is a false-PASS wearing the word `require`: the caller
#      that asked for mandatory assurance received the same exit code whether
#      the manifest was verified or never read. Absence of a parser is now a
#      refusal on the mandatory path, because "could not check" and "checked and
#      clean" are different answers and only one of them may exit 0.
#
#   2. ANY NON-EMPTY DIGEST PASSED. `evidenceDigest: sha256:deadbeef` satisfied
#      the old presence test, so the field attested nothing — a signed manifest
#      could carry a digest bound to no evidence at all. The digest is now BOUND:
#      it must equal the v1 binding over the achieved level, the source revision,
#      the artifact digest and the receipt root that the manifest itself
#      declares. An arbitrary string no longer verifies, because it no longer
#      names anything.
#
# The binding is defined ONCE, here, and exposed through --compute-digest so a
# producer and this verifier can never drift into two formulas:
#
#   evidenceDigest = sha256( "bubbles-assurance-evidence-v1\n"
#                            <level>\n <sourceRevision>\n
#                            <artifactDigest>\n <receiptRoot>\n )
#
# Usage:
#   bash bubbles/scripts/deploy-manifest-assurance-lint.sh --manifest <path> \
#        [--minimum-assurance <full|fast>] [--risk-class <class>] [--quiet]
#   bash bubbles/scripts/deploy-manifest-assurance-lint.sh --compute-digest \
#        --level <full|fast|prototype> --source-revision <rev> \
#        --artifact-digest <digest> --receipt-root <root>
#
# Exit codes:
#   0  clean / not-applicable / digest computed
#   1  a manifest assurance breach (malformed block, missing or UNBOUND digest,
#      prototype, under-assured vs the supplied floor, or an unavailable parser
#      while --require-assurance is active)
#   2  usage / runtime error
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/assurance-resolve.sh"

MANIFEST=""
MIN_ASSURANCE=""
RISK_CLASS=""
QUIET="false"
REQUIRE_ASSURANCE="false"
COMPUTE_DIGEST="false"
ARG_LEVEL=""
ARG_SOURCE_REVISION=""
ARG_ARTIFACT_DIGEST=""
ARG_RECEIPT_ROOT=""

# sha256 over stdin. macOS ships `shasum`, GNU ships `sha256sum`; neither is
# guaranteed, so the absence of BOTH is reported to the caller rather than
# silently degrading to a weaker comparison.
assurance_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | cut -d' ' -f1
  else
    return 1
  fi
}

# The ONE definition of the evidence binding. Producer and verifier both call it.
assurance_evidence_digest() {
  local level="$1" source_revision="$2" artifact_digest="$3" receipt_root="$4"
  local hex
  hex="$(printf 'bubbles-assurance-evidence-v1\n%s\n%s\n%s\n%s\n' \
    "$level" "$source_revision" "$artifact_digest" "$receipt_root" | assurance_sha256)" || return 1
  [[ -n "$hex" ]] || return 1
  printf 'sha256:%s' "$hex"
}

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/deploy-manifest-assurance-lint.sh --manifest <path> [--minimum-assurance <full|fast>] [--risk-class <class>] [--quiet]

Arguments:
  --manifest <path>            Deploy manifest (YAML) to validate.

Optional:
  --minimum-assurance <floor>  Target/train assurance floor (full|fast); when set,
                               an under-assured manifest is refused via assurance-resolve.sh.
  --risk-class <class>         Forwarded to assurance-resolve.sh (high/unknown → floor full).
  --quiet                      Suppress success output.
  --require-assurance          Make assurance MANDATORY: an absent assurance block is a
                               refusal instead of a backward-compatible no-op, and an
                               unavailable YAML parser is a refusal instead of a skip
                               (DEPLOY-101, IMP-047 PD-07).
  -h, --help                   Print this usage and exit.

Digest-producer mode:
  --compute-digest --level <full|fast|prototype> --source-revision <rev> \
                   --artifact-digest <digest> --receipt-root <root>
                               Print the bound evidenceDigest for those inputs and exit 0.
                               This is the SAME formula the verifier applies, so a producer
                               cannot drift from it.

Exit codes:
  0 = clean / not-applicable / digest computed
  1 = manifest assurance breach (malformed, missing or unbound digest, prototype,
      under-assured, or unavailable parser while --require-assurance is active)
  2 = usage / runtime error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --manifest)
      [[ $# -ge 2 ]] || {
        echo "deploy-manifest-assurance-lint: --manifest requires a value" >&2
        exit 2
      }
      MANIFEST="$2"
      shift 2
      ;;
    --minimum-assurance)
      [[ $# -ge 2 ]] || {
        echo "deploy-manifest-assurance-lint: --minimum-assurance requires a value" >&2
        exit 2
      }
      MIN_ASSURANCE="$2"
      shift 2
      ;;
    --risk-class)
      [[ $# -ge 2 ]] || {
        echo "deploy-manifest-assurance-lint: --risk-class requires a value" >&2
        exit 2
      }
      RISK_CLASS="$2"
      shift 2
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --require-assurance)
      REQUIRE_ASSURANCE="true"
      shift
      ;;
    --compute-digest)
      COMPUTE_DIGEST="true"
      shift
      ;;
    --level | --source-revision | --artifact-digest | --receipt-root)
      [[ $# -ge 2 ]] || {
        echo "deploy-manifest-assurance-lint: $1 requires a value" >&2
        exit 2
      }
      case "$1" in
        --level) ARG_LEVEL="$2" ;;
        --source-revision) ARG_SOURCE_REVISION="$2" ;;
        --artifact-digest) ARG_ARTIFACT_DIGEST="$2" ;;
        --receipt-root) ARG_RECEIPT_ROOT="$2" ;;
      esac
      shift 2
      ;;
    --*)
      echo "deploy-manifest-assurance-lint: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "deploy-manifest-assurance-lint: unexpected positional argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$COMPUTE_DIGEST" == "true" ]]; then
  if [[ -z "$ARG_LEVEL" || -z "$ARG_SOURCE_REVISION" || -z "$ARG_ARTIFACT_DIGEST" || -z "$ARG_RECEIPT_ROOT" ]]; then
    echo "deploy-manifest-assurance-lint: --compute-digest requires --level, --source-revision, --artifact-digest and --receipt-root" >&2
    exit 2
  fi
  case "$ARG_LEVEL" in
    full | fast | prototype) ;;
    *)
      echo "deploy-manifest-assurance-lint: --level must be full|fast|prototype (got '$ARG_LEVEL')" >&2
      exit 2
      ;;
  esac
  if ! assurance_evidence_digest "$ARG_LEVEL" "$ARG_SOURCE_REVISION" "$ARG_ARTIFACT_DIGEST" "$ARG_RECEIPT_ROOT"; then
    echo "deploy-manifest-assurance-lint: no sha256 tool (sha256sum/shasum) available to compute the binding" >&2
    exit 2
  fi
  printf '\n'
  exit 0
fi

if [[ -z "$MANIFEST" ]]; then
  echo "deploy-manifest-assurance-lint: missing required --manifest" >&2
  usage >&2
  exit 2
fi

if [[ -n "$MIN_ASSURANCE" ]]; then
  case "$MIN_ASSURANCE" in
    full | fast) ;;
    *)
      echo "deploy-manifest-assurance-lint: --minimum-assurance must be full|fast (got '$MIN_ASSURANCE')" >&2
      exit 2
      ;;
  esac
fi

if [[ ! -f "$MANIFEST" ]]; then
  echo "deploy-manifest-assurance-lint: manifest not found: $MANIFEST" >&2
  exit 2
fi

info() { [[ "$QUIET" == "true" ]] || echo "[deploy-manifest-assurance-lint] $*"; }
refuse() {
  echo "[deploy-manifest-assurance-lint][REFUSED] $*" >&2
}

# PD-07 defect 1. The parser gate used to exit 0 on a missing `yq` on EVERY
# path, so `--require-assurance` returned the identical exit code whether the
# manifest was verified or never opened. On the MANDATORY path that is a
# false-PASS, so absence of the parser is now a refusal. The optional path keeps
# the documented backward-compatible skip, because there the caller has not
# asked for an assurance decision at all.
if ! command -v yq >/dev/null 2>&1; then
  if [[ "$REQUIRE_ASSURANCE" == "true" ]]; then
    refuse "yq (mikefarah v4) is not installed, so $MANIFEST could NOT be verified, and --require-assurance makes assurance mandatory."
    refuse "A mandatory assurance check must never exit 0 because its parser is missing. Install yq: https://github.com/mikefarah/yq#install"
    exit 1
  fi
  echo "deploy-manifest-assurance-lint: WARN-and-skip — yq not installed; cannot parse $MANIFEST (exit 0; assurance is not mandatory here)." >&2
  exit 0
fi

# Detect the assurance block. mikefarah yq `type` → "!!null" when absent,
# "!!map" when present, a scalar tag (e.g. "!!str") when malformed.
block_type="$(yq -r '.attestations.assurance | type' "$MANIFEST" 2>/dev/null || echo "error")"
if [[ "$block_type" == "error" ]]; then
  echo "deploy-manifest-assurance-lint: cannot parse manifest YAML: $MANIFEST" >&2
  exit 2
fi
if [[ "$block_type" == "!!null" || -z "$block_type" ]]; then
  # IMP-101 SCOPE-9 (DEPLOY-101): an absent assurance block is a backward-compatible
  # no-op UNLESS the operator has activated mandatory assurance via
  # --require-assurance (typically passed by CI/apply after their assurance-migration
  # date). Then a manifest with no assurance block is REFUSED, closing the
  # fail-open window. The migration DATE is the operator's decision, expressed by
  # when they start passing the flag; the framework only provides the seam.
  if [[ "$REQUIRE_ASSURANCE" == "true" ]]; then
    refuse "no attestations.assurance block in $MANIFEST and --require-assurance is set (mandatory assurance active)"
    exit 1
  fi
  info "no attestations.assurance block in $MANIFEST — no-op (backward-compatible)."
  exit 0
fi
if [[ "$block_type" != "!!map" ]]; then
  refuse "attestations.assurance must be a mapping (got '$block_type') in $MANIFEST"
  exit 1
fi

level="$(yq -r '(.attestations.assurance.level // "")' "$MANIFEST" 2>/dev/null || echo "")"
[[ "$level" == "null" ]] && level=""
evidence_digest="$(yq -r '(.attestations.assurance.evidenceDigest // "")' "$MANIFEST" 2>/dev/null || echo "")"
[[ "$evidence_digest" == "null" ]] && evidence_digest=""
binding_source_revision="$(yq -r '(.attestations.assurance.evidenceBinding.sourceRevision // "")' "$MANIFEST" 2>/dev/null || echo "")"
[[ "$binding_source_revision" == "null" ]] && binding_source_revision=""
binding_artifact_digest="$(yq -r '(.attestations.assurance.evidenceBinding.artifactDigest // "")' "$MANIFEST" 2>/dev/null || echo "")"
[[ "$binding_artifact_digest" == "null" ]] && binding_artifact_digest=""
binding_receipt_root="$(yq -r '(.attestations.assurance.evidenceBinding.receiptRoot // "")' "$MANIFEST" 2>/dev/null || echo "")"
[[ "$binding_receipt_root" == "null" ]] && binding_receipt_root=""

case "$level" in
  full | fast | prototype) ;;
  "")
    refuse "attestations.assurance.level is required (expected full|fast|prototype) in $MANIFEST"
    exit 1
    ;;
  *)
    refuse "attestations.assurance.level '$level' is invalid (expected full|fast|prototype) in $MANIFEST"
    exit 1
    ;;
esac

# Choke #4: the signed manifest MUST carry an evidence digest for the attestation.
if [[ -z "$evidence_digest" ]]; then
  refuse "attestations.assurance.evidenceDigest is required (a signed manifest must attest its assurance evidence) in $MANIFEST"
  exit 1
fi

# PD-07 defect 2. Presence was the whole test, so `sha256:deadbeef` attested
# nothing. The digest must now BIND: it has to equal the v1 binding over the
# level and the three evidence anchors the manifest itself declares. An
# arbitrary string cannot satisfy that, because it names no source, no artifact
# and no receipt root.
missing_binding=""
[[ -n "$binding_source_revision" ]] || missing_binding+=" sourceRevision"
[[ -n "$binding_artifact_digest" ]] || missing_binding+=" artifactDigest"
[[ -n "$binding_receipt_root" ]] || missing_binding+=" receiptRoot"
if [[ -n "$missing_binding" ]]; then
  refuse "attestations.assurance.evidenceBinding is missing required field(s):${missing_binding} in $MANIFEST"
  refuse "evidenceDigest must BIND source, artifact and receipt root — a bare digest attests nothing."
  exit 1
fi

expected_digest=""
if ! expected_digest="$(assurance_evidence_digest "$level" "$binding_source_revision" "$binding_artifact_digest" "$binding_receipt_root")"; then
  # Fail-closed for the same reason as the parser gate: an assurance decision
  # that could not be computed is not an assurance decision.
  refuse "no sha256 tool (sha256sum/shasum) available to verify the evidence binding in $MANIFEST"
  exit 1
fi

if [[ "$evidence_digest" != "$expected_digest" ]]; then
  refuse "attestations.assurance.evidenceDigest does not bind its declared evidence in $MANIFEST"
  refuse "  declared: $evidence_digest"
  refuse "  bound:    $expected_digest  (level=$level, sourceRevision=$binding_source_revision, artifactDigest=$binding_artifact_digest, receiptRoot=$binding_receipt_root)"
  refuse "Recompute with --compute-digest; an arbitrary non-empty digest is refused."
  exit 1
fi

# The R5 invariant: prototype is NEVER deployable, at any target, regardless of floor.
if [[ "$level" == "prototype" ]]; then
  refuse "attestations.assurance.level 'prototype' is never deployable (R5 invariant) — $MANIFEST"
  exit 1
fi

# Choke #5: when a floor is supplied, consult the shared deploy-eligibility
# decision (never re-derive it here) and refuse an under-assured level.
if [[ -n "$MIN_ASSURANCE" ]]; then
  if [[ ! -x "$RESOLVE" ]]; then
    echo "deploy-manifest-assurance-lint: assurance-resolve.sh not found/executable at $RESOLVE" >&2
    exit 2
  fi
  resolve_out=""
  if [[ -n "$RISK_CLASS" ]]; then
    resolve_out="$(bash "$RESOLVE" --achieved-level "$level" --minimum-assurance "$MIN_ASSURANCE" --risk-class "$RISK_CLASS" 2>/dev/null || true)"
  else
    resolve_out="$(bash "$RESOLVE" --achieved-level "$level" --minimum-assurance "$MIN_ASSURANCE" 2>/dev/null || true)"
  fi
  eligible="$(printf '%s\n' "$resolve_out" | sed -n 's/^deployEligible=//p')"
  reason="$(printf '%s\n' "$resolve_out" | sed -n 's/^reason=//p')"
  if [[ "$eligible" != "true" ]]; then
    refuse "manifest assurance '$level' is NOT deployable at floor '$MIN_ASSURANCE'${RISK_CLASS:+, riskClass=$RISK_CLASS}: ${reason:-below required assurance} — $MANIFEST"
    exit 1
  fi
fi

info "PASSED — manifest assurance $level (evidenceDigest bound to source/artifact/receipt root)${MIN_ASSURANCE:+ meets floor $MIN_ASSURANCE} in $MANIFEST."
exit 0
