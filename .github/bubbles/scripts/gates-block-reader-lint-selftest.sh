#!/usr/bin/env bash
# bubbles/scripts/gates-block-reader-lint-selftest.sh
#
# Hermetic selftest for gates-block-reader-lint.sh (IMP-042 SCOPE-13).
#
# THE DEFECT THIS DEFENDS AGAINST
#
# The generated `gates:` block in workflows.yaml was deleted on the strength of a
# reader inventory built by looking for scripts that parse gate DEFINITIONS. Every
# script that instead greps the same file for a gate NAME was missed, three
# regressions shipped, and the removal was reverted.
#
# The load-bearing case here is case 3: a script that references workflows.yaml
# and names a gate id which ALSO appears outside the block must NOT be reported.
# Without it, the lint degrades into "mentions a gate id somewhere", the reader
# set becomes noise, and an inventory nobody believes is worse than none.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/gates-block-reader-lint.sh"

checks=0
failures=0
ok() {
  checks=$((checks + 1))
  echo "  ok   $1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  echo "  FAIL $1"
  echo "       $2"
}

[[ -f "$LINT" ]] || {
  echo "FAIL: $LINT not found" >&2
  exit 1
}

# `pwd -P` because /var and /tmp are symlinks on some platforms and the lint
# strips a physical prefix.
FIXTURE_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
cleanup() { rm -rf "$FIXTURE_ROOT"; }
trap cleanup EXIT INT TERM

build_fixture() {
  local root="$1"
  rm -rf "$root"
  mkdir -p "$root/bubbles/scripts" "$root/bubbles/registry"

  # G901 and fixture_only_gate exist ONLY inside the block; G902 also appears in
  # a per-mode requiredGates list, so it survives the block's deletion.
  cat >"$root/bubbles/workflows.yaml" <<'YAML'
defaultMode: full-delivery

gates:
  G901:
    name: fixture_only_gate
    description: declared solely inside the generated block
  G902:
    name: fixture_shared_gate
    description: also referenced by a mode
modes:
  demo:
    requiredGates:
      - G902
YAML

  cat >"$root/bubbles/scripts/reader.sh" <<'SH'
#!/usr/bin/env bash
# Reads a gate that exists only inside the generated block.
grep -q 'G901' bubbles/workflows.yaml
SH

  cat >"$root/bubbles/scripts/nonreader.sh" <<'SH'
#!/usr/bin/env bash
# Reads workflows.yaml, but for a gate the mode surface also declares.
grep -q 'G902' bubbles/workflows.yaml
SH

  cat >"$root/bubbles/scripts/unrelated.sh" <<'SH'
#!/usr/bin/env bash
# Names a block-exclusive gate but never reads the file.
echo 'G901 is documented elsewhere'
SH

  # Prose ABOUT the block is not a dependency on it. This is not hypothetical:
  # the framework-validate.sh check that wires this lint became the lint's own
  # first finding, because a comment naming workflows.yaml plus an unrelated gate
  # id elsewhere in the file was enough to qualify.
  cat >"$root/bubbles/scripts/documented.sh" <<'SH'
#!/usr/bin/env bash
# Describes the generated block in bubbles/workflows.yaml and mentions G901
# purely as an example of what lives there.
echo 'no gate lookup happens here'
SH
}

build_fixture "$FIXTURE_ROOT/repo"
REPO="$FIXTURE_ROOT/repo"

# --- 1. discovery is exact -------------------------------------------------
listed="$(bash "$LINT" --repo-root "$REPO" --list 2>/dev/null)"
if [[ "$listed" == "bubbles/scripts/reader.sh" ]]; then
  ok "1 the block-exclusive reader is the only script discovered"
else
  bad "1 discovery is exact" "listed: $(printf '%s' "$listed" | tr '\n' ' ')"
fi

# --- 2. a script naming a surviving gate id is NOT a reader ----------------
if ! grep -q 'nonreader.sh' <<<"$listed"; then
  ok "2 a gate id that also lives outside the block is not evidence of a dependency"
else
  bad "2 non-exclusive token ignored" "nonreader.sh was reported as a reader"
fi

# --- 3. a script that never reads the file is NOT a reader ----------------
if ! grep -q 'unrelated.sh' <<<"$listed"; then
  ok "3 naming a gate without reading workflows.yaml is not a dependency"
else
  bad "3 file reference required" "unrelated.sh was reported as a reader"
fi

# --- 3b. prose about the block is not a dependency on it -------------------
if ! grep -q 'documented.sh' <<<"$listed"; then
  ok "3b a comment describing the block is not a dependency on it"
else
  bad "3b comments ignored" "documented.sh was reported as a reader"
fi

# --- 4. seed then check is green ------------------------------------------
bash "$LINT" --repo-root "$REPO" --seed >/dev/null 2>&1
seed_rc=$?
bash "$LINT" --repo-root "$REPO" >/dev/null 2>&1
check_rc=$?
if [[ "$seed_rc" -eq 0 && "$check_rc" -eq 0 ]]; then
  ok "4 a seeded inventory passes its own check"
else
  bad "4 seed then check" "seed=$seed_rc check=$check_rc"
fi

# --- 5. a NEW undeclared reader fails and is named -------------------------
cat >"$REPO/bubbles/scripts/reader2.sh" <<'SH'
#!/usr/bin/env bash
# A second dependency introduced after the inventory was declared.
grep -q 'fixture_only_gate' bubbles/workflows.yaml
SH
out5="$(bash "$LINT" --repo-root "$REPO" 2>&1)"
rc5=$?
if [[ "$rc5" -eq 1 ]] && grep -q 'UNDECLARED' <<<"$out5" && grep -q 'reader2.sh' <<<"$out5"; then
  ok "5 an undeclared reader fails the lint and is named"
else
  bad "5 undeclared reader refused" "rc=$rc5 out=$(tr '\n' ' ' <<<"$out5")"
fi
rm -f "$REPO/bubbles/scripts/reader2.sh"

# --- 6. a declaration that stopped being true fails ------------------------
# This is what forces the inventory to SHRINK as readers are repointed.
cat >"$REPO/bubbles/scripts/reader.sh" <<'SH'
#!/usr/bin/env bash
# Repointed at the canonical registry.
grep -q 'G901' bubbles/registry/gates.yaml
SH
out6="$(bash "$LINT" --repo-root "$REPO" 2>&1)"
rc6=$?
if [[ "$rc6" -eq 1 ]] && grep -q 'no longer depends' <<<"$out6"; then
  ok "6 a declared reader that was repointed must leave the inventory"
else
  bad "6 stale declaration refused" "rc=$rc6 out=$(tr '\n' ' ' <<<"$out6")"
fi

# --- 7. a declared file that no longer exists fails ------------------------
rm -f "$REPO/bubbles/scripts/reader.sh"
out7="$(bash "$LINT" --repo-root "$REPO" 2>&1)"
rc7=$?
if [[ "$rc7" -eq 1 ]] && grep -q 'does not exist' <<<"$out7"; then
  ok "7 a declared reader that was deleted fails the lint"
else
  bad "7 missing declared file refused" "rc=$rc7 out=$(tr '\n' ' ' <<<"$out7")"
fi

# --- 8. with the block gone the precondition is reported as met ------------
build_fixture "$FIXTURE_ROOT/repo2"
REPO2="$FIXTURE_ROOT/repo2"
cat >"$REPO2/bubbles/workflows.yaml" <<'YAML'
defaultMode: full-delivery

modes:
  demo:
    requiredGates:
      - G902
YAML
bash "$LINT" --repo-root "$REPO2" --seed >/dev/null 2>&1
out8="$(bash "$LINT" --repo-root "$REPO2" 2>&1)"
rc8=$?
if [[ "$rc8" -eq 0 ]] && grep -q 'removal precondition is met' <<<"$out8"; then
  ok "8 an empty inventory reports the SCOPE-13 removal precondition as met"
else
  bad "8 empty inventory reports readiness" "rc=$rc8 out=$(tr '\n' ' ' <<<"$out8")"
fi

# --- 9. a missing inventory is a usage error, not a silent pass ------------
build_fixture "$FIXTURE_ROOT/repo3"
out9="$(bash "$LINT" --repo-root "$FIXTURE_ROOT/repo3" 2>&1)"
rc9=$?
if [[ "$rc9" -eq 2 ]] && grep -q 'inventory missing' <<<"$out9"; then
  ok "9 a missing inventory refuses instead of reporting success"
else
  bad "9 missing inventory refused" "rc=$rc9 out=$(tr '\n' ' ' <<<"$out9")"
fi

printf 'gates-block-reader-lint-selftest: %s check(s), %s failure(s)\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
