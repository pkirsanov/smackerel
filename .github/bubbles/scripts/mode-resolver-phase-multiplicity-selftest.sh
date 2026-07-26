#!/usr/bin/env bash
# bubbles/scripts/mode-resolver-phase-multiplicity-selftest.sh
#
# IMP-102 / SCOPE-2 regression selftest.
#
# Proves mode-resolver.sh preserves the MULTIPLICITY of intentionally-repeated
# phases in ordered phase lists (phaseOrder). The prior resolver applied a
# blanket `(.. | select(tag == "!!seq")) |= unique`, which silently deleted the
# SECOND occurrence of a legitimately-repeated phase — e.g. the post-remediation
# certification `validate` in harden-to-doc collapsed 2->1, and the second
# `releases` in idea-to-release-completion vanished entirely. The fix scopes
# dedupe to genuinely set-valued fields (requiredGates, tags) so ordered phase
# lists keep their order AND multiplicity.
#
# ADVERSARIAL / non-tautological: the expected per-mode repeated-phase counts
# are DERIVED FROM SOURCE (bubbles/workflows/modes.yaml) at runtime, never
# hardcoded. Each source-declared duplicate MUST survive resolution with >= its
# source multiplicity. Against the OLD blanket-unique resolver this test FAILS
# (validate/releases collapse 2->1); against the fixed field-scoped-dedupe
# resolver it PASSES. To see the failure, run this selftest against a checkout
# of HEAD:bubbles/scripts/mode-resolver.sh.
#
# Graceful skip when yq is absent (the resolver hard-requires yq v4+).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODES_FILE="${BUBBLES_MODES_FILE:-$ROOT_DIR/bubbles/workflows/modes.yaml}"

selftest_timeout_seconds="${BUBBLES_MODE_MULTIPLICITY_SELFTEST_TIMEOUT_SECONDS:-30}"

# The canonical modes that intentionally declare a repeated phase in phaseOrder
# (a baseline pass plus a post-remediation certification pass, or the two
# idea-to-release-completion `releases` bookends). Membership is asserted
# against source below, so registry drift that drops a duplicate FAILS loudly
# rather than silently making the test tautological.
MODES=(
  harden-to-doc
  gaps-to-doc
  harden-gaps-to-doc
  reconcile-to-doc
  stabilize-to-doc
  improve-existing
  idea-to-release-completion
  stochastic-quality-sweep
)

pass=0
fail=0
pass_msg() {
  pass=$((pass + 1))
  echo "PASS: $1"
}
fail_msg() {
  fail=$((fail + 1))
  echo "FAIL: $1" >&2
}

# Count exact-match occurrences of a phase token in a newline-delimited list on
# stdin. grep -c exits 1 (prints 0) on no match; `|| true` keeps set -e happy.
count_phase() {
  grep -cxF "$1" || true
}

echo "=== mode-resolver phase-multiplicity selftest (IMP-102 / SCOPE-2) ==="

if ! command -v yq >/dev/null 2>&1; then
  echo "mode-resolver-phase-multiplicity-selftest: SKIP (yq not installed)"
  exit 0
fi
if [[ ! -f "$MODES_FILE" ]]; then
  echo "mode-resolver-phase-multiplicity-selftest: SKIP (modes file not found: $MODES_FILE)"
  exit 0
fi

for m in "${MODES[@]}"; do
  # ── Derive the expected repeated-phase counts FROM SOURCE (modes.yaml). ──
  src_phases="$(yq -r ".modes.\"$m\".phaseOrder[]?" "$MODES_FILE" 2>/dev/null || true)"
  if [[ -z "$src_phases" ]]; then
    fail_msg "$m: no phaseOrder found in $MODES_FILE (registry drift?)"
    continue
  fi
  # Phases that source declares >= 2 times — the intentional duplicates.
  dup_phases="$(printf '%s\n' "$src_phases" | sort | uniq -d)"
  if [[ -z "$dup_phases" ]]; then
    fail_msg "$m: source phaseOrder declares NO repeated phase — test premise broken (modes.yaml changed?)"
    continue
  fi

  # ── Resolve the mode through the resolver (grandfather + portable timeout). ──
  set +e
  resolved="$(bubbles_run_with_timeout "$selftest_timeout_seconds" \
    env BUBBLES_MODE_GRANDFATHER=1 "$RESOLVER" "$m" 2>/dev/null)"
  rc=$?
  set -e
  if (( rc != 0 )); then
    fail_msg "$m: resolver exited $rc"
    continue
  fi
  res_phases="$(printf '%s\n' "$resolved" | yq -r '.phaseOrder[]?' 2>/dev/null || true)"
  if [[ -z "$res_phases" ]]; then
    fail_msg "$m: resolved output has no phaseOrder"
    continue
  fi

  # ── Assert each source-duplicated phase survives with >= source multiplicity. ──
  while IFS= read -r ph; do
    [[ -z "$ph" ]] && continue
    exp="$(printf '%s\n' "$src_phases" | count_phase "$ph")"
    got="$(printf '%s\n' "$res_phases" | count_phase "$ph")"
    if (( got >= exp && exp >= 2 )); then
      pass_msg "$m: phase '$ph' multiplicity preserved (source x$exp, resolved x$got)"
    else
      fail_msg "$m: phase '$ph' COLLAPSED (source x$exp, resolved x$got) — blanket-unique regression"
    fi
  done <<< "$dup_phases"
done

# Anti-silent-pass: a run that asserted nothing is itself a failure.
if (( pass == 0 && fail == 0 )); then
  fail_msg "no phase-multiplicity assertions ran (all modes skipped or errored)"
fi

echo ""
echo "mode-resolver-phase-multiplicity-selftest: $pass passed / $fail failed"
if (( fail != 0 )); then
  exit 1
fi
echo "PASS"
