#!/usr/bin/env bash
# Hermetic selftest for deploy-manifest-assurance-lint.sh
# (IMP-100 Phase 3 chokes #4/#5 — framework contract half; IMP-047 PD-07).
# macOS+WSL portable — no `timeout`.
#
# T19..T24 are the PD-07 adversaries. They exist because the two defects PD-07
# names both LOOKED like passes: a missing parser exited 0 under the word
# `require`, and any non-empty string satisfied `evidenceDigest`. A test that
# only feeds well-formed input can never see either one, so these cases remove
# the tool and forge the digest on purpose.
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

# --- bound digests ------------------------------------------------------------
# Computed through the guard's OWN --compute-digest, so the fixtures cannot drift
# from the verifier's formula. If the two ever disagree, every positive case
# fails loudly instead of quietly asserting a wrong constant.
SRC_REV="a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
ART_DIGEST="sha256:1111111111111111111111111111111111111111111111111111111111111111"
RECEIPT_ROOT="sha256:2222222222222222222222222222222222222222222222222222222222222222"

digest_for() {
  bash "$GUARD" --compute-digest --level "$1" \
    --source-revision "$SRC_REV" --artifact-digest "$ART_DIGEST" --receipt-root "$RECEIPT_ROOT"
}

D_FULL="$(digest_for full)"
D_FAST="$(digest_for fast)"
D_PROTO="$(digest_for prototype)"

if [[ -z "$D_FULL" || "$D_FULL" != sha256:* ]]; then
  echo "deploy-manifest-assurance-lint-selftest: --compute-digest produced no usable digest" >&2
  exit 1
fi

manifest_with() {
  # $1 = level, $2 = digest ("" to omit), $3 = binding|nobinding, $4 = path
  {
    printf 'project: demo\ntarget: home-lab\nimage:\n  digest: sha256:abc\nattestations:\n  assurance:\n'
    printf '    level: %s\n    profile: delivery-completion-v1\n' "$1"
    if [[ "$3" == "binding" ]]; then
      printf '    evidenceBinding:\n      sourceRevision: %s\n      artifactDigest: %s\n      receiptRoot: %s\n' \
        "$SRC_REV" "$ART_DIGEST" "$RECEIPT_ROOT"
    fi
    if [[ -n "$2" ]]; then
      printf '    evidenceDigest: %s\n' "$2"
    fi
  } >"$4"
}

M_NO_BLOCK='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  signature: sig
  sbom: sbom
  provenance: prov'

M_SCALAR_BLOCK='project: demo
target: home-lab
image:
  digest: sha256:abc
attestations:
  assurance: oops'

w() { printf '%s\n' "$2" >"$1"; }
m_no_block="$TMP_ROOT/no_block.yaml"
w "$m_no_block" "$M_NO_BLOCK"
m_scalar="$TMP_ROOT/scalar.yaml"
w "$m_scalar" "$M_SCALAR_BLOCK"

m_full="$TMP_ROOT/full.yaml"
manifest_with full "$D_FULL" binding "$m_full"
m_fast="$TMP_ROOT/fast.yaml"
manifest_with fast "$D_FAST" binding "$m_fast"
m_proto="$TMP_ROOT/proto.yaml"
manifest_with prototype "$D_PROTO" binding "$m_proto"
m_nodigest="$TMP_ROOT/nodigest.yaml"
manifest_with full "" binding "$m_nodigest"
m_badlevel="$TMP_ROOT/badlevel.yaml"
manifest_with gold "$D_FULL" binding "$m_badlevel"
# PD-07 adversary: non-empty and well-shaped, but bound to nothing.
m_forged="$TMP_ROOT/forged.yaml"
manifest_with full "sha256:deadbeef" binding "$m_forged"
# PD-07 adversary: the old shape — a digest with no binding block at all.
m_unbound="$TMP_ROOT/unbound.yaml"
manifest_with full "$D_FULL" nobinding "$m_unbound"
# PD-07 adversary: a correctly computed digest, bound for a DIFFERENT level.
m_levelswap="$TMP_ROOT/levelswap.yaml"
manifest_with full "$D_FAST" binding "$m_levelswap"

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

# T2: full, no floor → shape valid, bound digest, not prototype → clean.
run "T2 full, no floor → clean (exit 0)" 0 --manifest "$m_full"

# T3: fast, no floor → shape valid, bound digest, not prototype → clean.
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

# --- PD-07 adversaries --------------------------------------------------------

# T19: an arbitrary non-empty digest is REFUSED. This is the exact input the old
# presence-only test passed.
run "T19 forged non-empty evidenceDigest → refuse (exit 1)" 1 --manifest "$m_forged"

# T20: a digest with no evidenceBinding block cannot bind anything → refuse.
run "T20 evidenceDigest with no evidenceBinding → refuse (exit 1)" 1 --manifest "$m_unbound"

# T21: a digest legitimately computed for `fast` must not verify a `full`
# manifest. The level is inside the binding, so an attestation cannot be
# promoted by editing one field.
run "T21 digest bound to a different level → refuse (exit 1)" 1 --manifest "$m_levelswap"

# T22/T23: the parser-absence pair. `yq` is removed from PATH by running with a
# PATH that contains only a stub directory holding the interpreter and the few
# coreutils the guard needs — `yq` is deliberately NOT among them. Simulating
# absence by PATH is the only way to exercise the branch on a machine that has
# yq installed, which is every machine that reaches this test.
STUB_BIN="$TMP_ROOT/stub-bin"
mkdir -p "$STUB_BIN"
for tool in bash sh env dirname basename cat cut sed grep sha256sum shasum; do
  tool_path="$(command -v "$tool" 2>/dev/null || true)"
  if [[ -n "$tool_path" ]]; then
    ln -sf "$tool_path" "$STUB_BIN/$tool"
  fi
done
if PATH="$STUB_BIN" command -v yq >/dev/null 2>&1; then
  fail "T22/T23 setup (yq still visible on the stub PATH; absence was not simulated)"
else
  rc=0
  PATH="$STUB_BIN" bash "$GUARD" --manifest "$m_full" --require-assurance >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq 1 ]]; then
    pass "T22 yq absent + --require-assurance → FAIL CLOSED (exit 1)"
  else
    fail "T22 yq absent + --require-assurance → FAIL CLOSED (expected exit 1, got $rc)"
  fi

  rc=0
  PATH="$STUB_BIN" bash "$GUARD" --manifest "$m_full" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]]; then
    pass "T23 yq absent, assurance optional → documented WARN-and-skip (exit 0)"
  else
    fail "T23 yq absent, assurance optional → WARN-and-skip (expected exit 0, got $rc)"
  fi
fi

# T24: --compute-digest is deterministic and level-sensitive, which is what makes
# T21 a real discriminator rather than a coincidence.
if [[ "$D_FULL" == "$(digest_for full)" && "$D_FULL" != "$D_FAST" && "$D_FULL" != "$D_PROTO" ]]; then
  pass "T24 --compute-digest is deterministic and level-sensitive"
else
  fail "T24 --compute-digest is deterministic and level-sensitive"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "deploy-manifest-assurance-lint-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "deploy-manifest-assurance-lint-selftest: all cases passed."
