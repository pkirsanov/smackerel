#!/usr/bin/env bash
# Hermetic selftest for deploy-manifest-assurance-lint.sh
# (IMP-100 Phase 3 chokes #4/#5 — framework contract half).
# macOS+WSL portable — no `timeout`; yq-gated (the guard consumes yq).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/deploy-manifest-assurance-lint.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v yq >/dev/null 2>&1; then
  echo "deploy-manifest-assurance-lint-selftest: SKIP (yq not installed)"
  exit 0
fi

# --- manifest fixtures --------------------------------------------------------
M_NO_BLOCK='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  signature: sig
  sbom: sbom
  provenance: prov'

M_FULL='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance:
    level: full
    profile: delivery-completion-v1
    evidenceDigest: sha256:deadbeef'

M_FAST='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance:
    level: fast
    profile: delivery-completion-fast-v1
    evidenceDigest: sha256:cafef00d'

M_PROTOTYPE='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance:
    level: prototype
    profile: delivery-completion-v1
    evidenceDigest: sha256:0badf00d'

M_MISSING_DIGEST='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance:
    level: full
    profile: delivery-completion-v1'

M_BAD_LEVEL='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance:
    level: gold
    profile: x
    evidenceDigest: sha256:abc'

M_SCALAR_BLOCK='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance: oops'

w() { printf '%s\n' "$2" > "$1"; }
m_no_block="$TMP_ROOT/no_block.yaml"
w "$m_no_block" "$M_NO_BLOCK"
m_full="$TMP_ROOT/full.yaml"
w "$m_full" "$M_FULL"
m_fast="$TMP_ROOT/fast.yaml"
w "$m_fast" "$M_FAST"
m_proto="$TMP_ROOT/proto.yaml"
w "$m_proto" "$M_PROTOTYPE"
m_nodigest="$TMP_ROOT/nodigest.yaml"
w "$m_nodigest" "$M_MISSING_DIGEST"
m_badlevel="$TMP_ROOT/badlevel.yaml"
w "$m_badlevel" "$M_BAD_LEVEL"
m_scalar="$TMP_ROOT/scalar.yaml"
w "$m_scalar" "$M_SCALAR_BLOCK"

run() {
  local label="$1" exp="$2"
  shift 2
  local rc=0
  bash "$GUARD" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running deploy-manifest-assurance-lint selftest..."

# T1: no assurance block → no-op (backward-compatible).
run "T1 no assurance block → no-op (exit 0)" 0 --manifest "$m_no_block"

# T2: full, no floor → shape valid, not prototype → clean.
run "T2 full, no floor → clean (exit 0)" 0 --manifest "$m_full"

# T3: fast, no floor → shape valid, not prototype → clean.
run "T3 fast, no floor → clean (exit 0)" 0 --manifest "$m_fast"

# T4: prototype, no floor → ALWAYS refuse (R5 invariant).
run "T4 prototype, no floor → refuse (exit 1)" 1 --manifest "$m_proto"

# T5: full at floor=full → deployable.
run "T5 full at floor=full → deployable (exit 0)" 0 --manifest "$m_full" --minimum-assurance full

# T6: fast at floor=full → under-assured refuse.
run "T6 fast at floor=full → refuse (exit 1)" 1 --manifest "$m_fast" --minimum-assurance full

# T7: fast at floor=fast → deployable.
run "T7 fast at floor=fast → deployable (exit 0)" 0 --manifest "$m_fast" --minimum-assurance fast

# T8: prototype at floor=fast → refuse (never deployable).
run "T8 prototype at floor=fast → refuse (exit 1)" 1 --manifest "$m_proto" --minimum-assurance fast

# T9: level present but evidenceDigest missing → refuse.
run "T9 missing evidenceDigest → refuse (exit 1)" 1 --manifest "$m_nodigest"

# T10: invalid level token → refuse.
run "T10 invalid level 'gold' → refuse (exit 1)" 1 --manifest "$m_badlevel"

# T11: assurance block is a scalar, not a map → refuse.
run "T11 scalar assurance block → refuse (exit 1)" 1 --manifest "$m_scalar"

# T12: fast at floor=fast but riskClass=high → escalate to full → refuse.
run "T12 fast at floor=fast + riskClass=high → refuse (exit 1)" 1 --manifest "$m_fast" --minimum-assurance fast --risk-class high

# T13: full at floor=fast → deployable (full always meets floor).
run "T13 full at floor=fast → deployable (exit 0)" 0 --manifest "$m_full" --minimum-assurance fast

# T14: manifest not found → usage/runtime error.
run "T14 manifest not found → error (exit 2)" 2 --manifest "$TMP_ROOT/does_not_exist.yaml"

# T15: invalid --minimum-assurance value → usage error.
run "T15 invalid --minimum-assurance 'prototype' → error (exit 2)" 2 --manifest "$m_full" --minimum-assurance prototype

# T16: missing --manifest → usage error.
run "T16 missing --manifest → error (exit 2)" 2 --minimum-assurance full

# T17 (IMP-101 SCOPE-9): absent block + --require-assurance → refuse (mandatory).
run "T17 no block + --require-assurance → refuse (exit 1)" 1 --manifest "$m_no_block" --require-assurance

# T18 (IMP-101 SCOPE-9): present block + --require-assurance → still deployable.
run "T18 full block + --require-assurance → deployable (exit 0)" 0 --manifest "$m_full" --require-assurance

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "deploy-manifest-assurance-lint-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "deploy-manifest-assurance-lint-selftest: all cases passed."
