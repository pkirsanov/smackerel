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
# BACKWARD-COMPATIBLE + fail-open on absence, fail-closed on a real breach:
#   - manifest with no attestations.assurance block → no-op (exit 0)
#     (existing pre-assurance manifests never break)
#   - yq not installed                              → WARN-and-skip (exit 0)
# A PRESENT assurance block is validated in full. There is NO --skip/--force flag.
#
# Usage:
#   bash bubbles/scripts/deploy-manifest-assurance-lint.sh --manifest <path> \
#        [--minimum-assurance <full|fast>] [--risk-class <class>] [--quiet]
#
# Exit codes:
#   0  clean / not-applicable
#   1  a manifest assurance breach (malformed block, missing digest, prototype,
#      or under-assured vs the supplied floor)
#   2  usage / runtime error
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/assurance-resolve.sh"

MANIFEST=""
MIN_ASSURANCE=""
RISK_CLASS=""
QUIET="false"

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
  -h, --help                   Print this usage and exit.

Exit codes:
  0 = clean / not-applicable
  1 = manifest assurance breach (malformed, missing digest, prototype, or under-assured)
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

if ! command -v yq >/dev/null 2>&1; then
  echo "deploy-manifest-assurance-lint: WARN-and-skip — yq not installed; cannot parse $MANIFEST (exit 0)." >&2
  exit 0
fi

info() { [[ "$QUIET" == "true" ]] || echo "[deploy-manifest-assurance-lint] $*"; }
refuse() {
  echo "[deploy-manifest-assurance-lint][REFUSED] $*" >&2
}

# Detect the assurance block. mikefarah yq `type` → "!!null" when absent,
# "!!map" when present, a scalar tag (e.g. "!!str") when malformed.
block_type="$(yq -r '.attestations.assurance | type' "$MANIFEST" 2>/dev/null || echo "error")"
if [[ "$block_type" == "error" ]]; then
  echo "deploy-manifest-assurance-lint: cannot parse manifest YAML: $MANIFEST" >&2
  exit 2
fi
if [[ "$block_type" == "!!null" || -z "$block_type" ]]; then
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

info "PASSED — manifest assurance $level (evidenceDigest present)${MIN_ASSURANCE:+ meets floor $MIN_ASSURANCE} in $MANIFEST."
exit 0
