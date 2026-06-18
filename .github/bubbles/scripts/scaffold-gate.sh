#!/usr/bin/env bash
# File: scaffold-gate.sh
#
# Additive scaffolder for a new Bubbles certification gate. It removes the
# error-prone, hand-walked parts of the new-gate ritual:
#   1. computing the NEXT FREE gate ID (skipping the former burned G096 and the
#      reserved G102–G109 gap), and
#   2. stamping the three NEW skeleton files (guard + hermetic selftest +
#      regression test) with the correct exit-code contract, and
#   3. emitting the precise, copy-pasteable WIRING CHECKLIST for the touchpoints
#      that mutate SHARED registry/guard files.
#
# It is deliberately PURELY ADDITIVE: it creates new files and prints guidance. It
# does NOT auto-edit gates.yaml / workflows.yaml / state-transition-guard.sh /
# framework-validate.sh — programmatic edits to those shared, load-bearing files
# are higher-risk than the value they add, so they stay a guided manual step (run
# `regen-derived.sh` last, per IMP-007). See improvements/IMP-011.
#
# Usage: scaffold-gate.sh <gate_name> [--dry-run]
#   <gate_name>  snake_case, MUST end in `_gate` (e.g. my_new_thing_gate).
#   --dry-run    print the plan (computed ID, files, checklist) WITHOUT writing.
#
# Exit: 0 success / plan printed; 1 refusal (bad name, would clobber, no registry);
#       2 usage.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Repo root: source layout is <root>/bubbles/scripts. Overridable for selftests.
REPO_ROOT="${BUBBLES_SCAFFOLD_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
GATES_FILE="${BUBBLES_SCAFFOLD_GATES_FILE:-$REPO_ROOT/bubbles/registry/gates.yaml}"

usage() {
  sed -n '2,24p' "$0"
}

GATE_NAME=""
DRY_RUN=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    -h | --help)
      usage
      exit 0
      ;;
    -*)
      echo "scaffold-gate: unknown flag '$arg'." >&2
      exit 2
      ;;
    *)
      if [[ -n "$GATE_NAME" ]]; then
        echo "scaffold-gate: only one gate name may be given (got '$GATE_NAME' and '$arg')." >&2
        exit 2
      fi
      GATE_NAME="$arg"
      ;;
  esac
done

[[ -n "$GATE_NAME" ]] || {
  usage
  exit 2
}

# --- Validate the gate name ------------------------------------------------
if [[ ! "$GATE_NAME" =~ ^[a-z][a-z0-9_]*_gate$ ]]; then
  echo "scaffold-gate: gate name must be snake_case ending in '_gate' (got '$GATE_NAME')." >&2
  exit 1
fi

[[ -f "$GATES_FILE" ]] || {
  echo "scaffold-gate: gate registry not found at $GATES_FILE (run from the Bubbles source repo)." >&2
  exit 1
}

# --- Compute the next free gate ID -----------------------------------------
# Bands/holes (see agents/bubbles_shared/quality-gates.md): the former G096 is
# BURNED (legacy id, never reused), and G102–G109 is a RESERVED GAP. The next id
# is max(existing)+1 advanced past any burned id or the reserved gap.
bubbles_next_gate_id() {
  local gates_file="$1"
  local max=0 n
  while IFS= read -r n; do
    n=$((10#$n))
    ((n > max)) && max=$n
  done < <(grep -oE '^  G[0-9]{3}:' "$gates_file" | grep -oE '[0-9]{3}')
  local cand=$((max + 1))
  while [[ "$cand" -eq 96 ]] || { [[ "$cand" -ge 102 ]] && [[ "$cand" -le 109 ]]; }; do
    cand=$((cand + 1))
  done
  printf 'G%03d' "$cand"
}

# --- Compute the next regression test number -------------------------------
bubbles_next_regression_nn() {
  local dir="$REPO_ROOT/tests/regression"
  local max=0 n
  if [[ -d "$dir" ]]; then
    while IFS= read -r n; do
      n=$((10#$n))
      ((n > max)) && max=$n
    done < <(find "$dir" -maxdepth 1 -name 'test_*.sh' -exec basename {} \; 2>/dev/null | grep -oE '^test_[0-9]+' | grep -oE '[0-9]+')
  fi
  printf '%02d' $((max + 1))
}

GATE_ID="$(bubbles_next_gate_id "$GATES_FILE")"
# Base slug: strip trailing _gate, hyphenate.
BASE="$(printf '%s' "${GATE_NAME%_gate}" | tr '_' '-')"
NN="$(bubbles_next_regression_nn)"

GUARD="bubbles/scripts/${BASE}-guard.sh"
SELFTEST="bubbles/scripts/${BASE}-guard-selftest.sh"
REGRESSION="tests/regression/test_${NN}_${BASE}.sh"

echo "scaffold-gate plan"
echo "  gate id      : $GATE_ID"
echo "  gate name    : $GATE_NAME"
echo "  guard        : $GUARD"
echo "  selftest     : $SELFTEST"
echo "  regression   : $REGRESSION"
echo

# Refuse to clobber any existing target.
for rel in "$GUARD" "$SELFTEST" "$REGRESSION"; do
  if [[ -e "$REPO_ROOT/$rel" ]]; then
    echo "scaffold-gate: refusing to clobber existing file: $rel" >&2
    exit 1
  fi
done

write_file() {
  local rel="$1" body="$2"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] would create $rel"
    return 0
  fi
  mkdir -p "$REPO_ROOT/$(dirname "$rel")"
  printf '%s' "$body" >"$REPO_ROOT/$rel"
  chmod +x "$REPO_ROOT/$rel"
  echo "created $rel"
}

guard_body="#!/usr/bin/env bash
set -euo pipefail
#
# ${BASE}-guard.sh
#
# Gate ${GATE_ID} — ${GATE_NAME}.
#
# TODO(${GATE_ID}): one-paragraph rationale — what failure mode this blocks and why
# a per-spec gate could not already catch it.
#
# Exit: 0 = clean / not-applicable / grandfathered; 1 = finding (BLOCKING);
#       2 = usage.

if [[ \"\${1:-}\" == \"-h\" || \"\${1:-}\" == \"--help\" ]]; then
  sed -n '2,12p' \"\$0\"
  exit 0
fi

# TODO(${GATE_ID}): implement the real check. Replace the placeholder below.
# Keep it fail-loud (no silent skips) and provide an actionable message on a
# finding. No --skip/--force/--ignore bypass.
echo \"${GATE_ID} ${GATE_NAME}: TODO implement check\" >&2
exit 1
"

selftest_body="#!/usr/bin/env bash
set -euo pipefail
#
# ${BASE}-guard-selftest.sh — hermetic selftest for Gate ${GATE_ID} (${GATE_NAME}).
#
# Build mktemp fixtures and assert BOTH directions:
#   * a CLEAN fixture → guard exits 0;
#   * an ADVERSARIAL fixture that violates the rule → guard exits 1
#     (would FAIL if the bug were reintroduced — no tautological pass).

SCRIPT_DIR=\"\$(cd \"\$(dirname \"\${BASH_SOURCE[0]}\")\" && pwd)\"
GUARD=\"\$SCRIPT_DIR/${BASE}-guard.sh\"
work=\"\$(mktemp -d)\"
trap 'rm -rf \"\$work\"' EXIT
failures=0
pass() { echo \"PASS: \$1\"; }
fail() {
  echo \"FAIL: \$1\"
  failures=\$((failures + 1))
}

# TODO(${GATE_ID}): build a CLEAN fixture and assert exit 0.
# TODO(${GATE_ID}): build an ADVERSARIAL fixture and assert exit 1.

if [[ \"\$failures\" -eq 0 ]]; then
  echo \"[${BASE}-guard-selftest] OK\"
else
  echo \"[${BASE}-guard-selftest] \$failures failed\"
  exit 1
fi
"

regression_body="#!/usr/bin/env bash
set -euo pipefail
#
# test_${NN}_${BASE}.sh — persistent regression for Gate ${GATE_ID} (${GATE_NAME}).
#
# Re-runs the guard's hermetic selftest so a future change that re-breaks the
# gate is caught by the regression suite, not only by framework-validate.

SCRIPT_DIR=\"\$(cd \"\$(dirname \"\${BASH_SOURCE[0]}\")\" && pwd)\"
exec bash \"\$SCRIPT_DIR/../../bubbles/scripts/${BASE}-guard-selftest.sh\"
"

write_file "$GUARD" "$guard_body"
write_file "$SELFTEST" "$selftest_body"
write_file "$REGRESSION" "$regression_body"

cat <<CHECKLIST

────────────────────────────────────────────────────────────────────────────
Remaining MANUAL wiring (touches SHARED registry/guard files — done by hand,
then verified). Each line is the touchpoint; mirror an existing recent gate.

  [ ] bubbles/registry/gates.yaml
        add a '  ${GATE_ID}:' entry (name: ${GATE_NAME}, BLOCKING prose with
        enforced-by/exits/selftest/regression), then regenerate the workflows
        gates block:
          bash bubbles/scripts/generate-gates-block.sh && \\
          bash bubbles/scripts/generate-gates-block.sh --check
  [ ] bubbles/scripts/guards/tail-delegated-gates.sh
        add a CHECK block that runs the guard in the feature repo (if delegated).
  [ ] bubbles/scripts/state-transition-guard.sh
        bump the two 'Checks NN / G085-G0NN' range labels (+ the tail fragment
        header) to include ${GATE_ID} (if delegated).
  [ ] bubbles/scripts/framework-validate.sh
        add:  run_check \"${GATE_NAME} selftest\" bash \"\\\$SCRIPT_DIR/${BASE}-guard-selftest.sh\"
  [ ] skills/bubbles-quality-gates-catalog/SKILL.md
        add a quick-ref row for ${GATE_ID}; bump the catalog ceiling note.
  [ ] agents/bubbles_shared/quality-gates.md
        add the ${GATE_ID} rationale row; update the range note.
  [ ] VERSION  → MINOR bump (new gate).   CHANGELOG.md → new top '## vX.Y.0' entry.
  [ ] Regenerate ALL derived artifacts in dependency order (IMP-007):
          bash bubbles/scripts/regen-derived.sh
  [ ] Validate:
          bash bubbles/scripts/framework-validate.sh
          bash bubbles/scripts/release-check.sh
────────────────────────────────────────────────────────────────────────────
CHECKLIST

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "(dry-run: no files were written)"
fi
