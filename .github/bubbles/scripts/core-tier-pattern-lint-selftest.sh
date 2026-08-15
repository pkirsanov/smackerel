#!/usr/bin/env bash
# bubbles/scripts/core-tier-pattern-lint-selftest.sh
#
# Hermetic selftest for core-tier-pattern-lint.sh (IMP-042 SCOPE-2).
#
# Case 2 is the load-bearing one: a renamed check must turn the lint RED. If it
# does not, the lint is decoration and the silent tier erosion it exists to catch
# still happens.
#
# Usage: bash bubbles/scripts/core-tier-pattern-lint-selftest.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/core-tier-pattern-lint.sh"
NAME="core-tier-pattern-lint-selftest"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

[[ -f "$TARGET" ]] || {
  printf '%s: SKIP (target missing)\n' "$NAME"
  exit 0
}

WORK="$(mktemp -d 2>/dev/null)" || {
  printf '%s: cannot create temp dir\n' "$NAME" >&2
  exit 1
}
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# A miniature validator with the same shape as the real one.
make_validator() {
  local path="$1" alpha_label="$2"
  cat >"$path" <<EOF
#!/usr/bin/env bash
core_check_label() {
  case "\$1" in
    *"Alpha check"* | *"Beta check"*)
      return 0
      ;;
    *) return 1 ;;
  esac
}
run_check "$alpha_label" bash alpha.sh
run_check_self_only "Beta check selftest" bash beta.sh
run_check "Gamma check" \\
  bash gamma.sh
EOF
}

# --- 1. every pattern matches a scheduled check -> green ---------------------
v1="$WORK/ok.sh"
make_validator "$v1" "Alpha check"
out="$(bash "$TARGET" --validator "$v1" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'all 2 core pattern'; then
  ok "a validator whose core patterns all match is green"
else
  bad "matching patterns are green" "exit $rc: $out"
fi

# --- 2. ADVERSARIAL: rename a check and the lint must go RED -----------------
# This is the exact silent erosion the lint exists to catch. Pre-push runs
# --tier=core, so a dead pattern is a guard that quietly stopped blocking.
# The new label must not CONTAIN the old needle, or the substring match still
# succeeds and nothing has actually been renamed.
v2="$WORK/renamed.sh"
make_validator "$v2" "Structural drift probe"
out="$(bash "$TARGET" --validator "$v2" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'DEAD core pattern: "Alpha check"'; then
  ok "renaming a core check turns the lint red and names the dead pattern"
else
  bad "renamed check detected" "exit $rc: $out"
fi

# --- 3. backslash-continued run_check invocations are still seen -------------
# A line-at-a-time reader would miss the continued form and under-report labels.
v3="$WORK/continued.sh"
cat >"$v3" <<'EOF'
#!/usr/bin/env bash
core_check_label() {
  case "$1" in
    *"Gamma check"*)
      return 0
      ;;
    *) return 1 ;;
  esac
}
run_check "Gamma check" \
  bash gamma.sh
EOF
out="$(bash "$TARGET" --validator "$v3" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "a backslash-continued run_check is counted as scheduled"
else
  bad "continued invocation counted" "exit $rc: $out"
fi

# --- 4. an EMPTY core tier is a failure, not a pass --------------------------
# Zero patterns would let every push through while reporting nothing wrong.
v4="$WORK/empty.sh"
cat >"$v4" <<'EOF'
#!/usr/bin/env bash
core_check_label() {
  case "$1" in
    *) return 1 ;;
  esac
}
run_check "Alpha check" bash alpha.sh
EOF
out="$(bash "$TARGET" --validator "$v4" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'ZERO core patterns'; then
  ok "an empty core tier fails loudly instead of passing vacuously"
else
  bad "empty core tier fails" "exit $rc: $out"
fi

# --- 5. no bypass flag exists ------------------------------------------------
out="$(bash "$TARGET" --force 2>&1)"
rc=$?
if [[ "$rc" -eq 2 ]] && printf '%s' "$out" | grep -q 'no bypass'; then
  ok "a bypass-shaped flag is rejected by name"
else
  bad "bypass rejected" "exit $rc: $out"
fi

# --- 6. the REAL validator is currently clean --------------------------------
# Reported as a live check so a dead pattern in the shipped validator is caught
# here rather than at the next rename.
out="$(bash "$TARGET" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "the shipped framework-validate.sh has no dead core pattern"
else
  bad "shipped validator is clean" "$out"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
