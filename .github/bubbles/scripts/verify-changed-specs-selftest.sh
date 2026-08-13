#!/usr/bin/env bash
# bubbles/scripts/verify-changed-specs-selftest.sh
#
# Hermetic selftest for verify-changed-specs.sh (IMP-040 SCOPE-11 / COV-12).
#
# A2 is the case that justifies the command existing. A diff that touches ONLY
# source changes no spec folder, so a discoverer built on spec paths alone
# reports nothing while certified scenarios go stale. If A2 ever regresses, the
# command has collapsed back into the thing it replaced.
#
# P3 is its guard: a source diff that intersects NO implementationRefs must stay
# silent. A discoverer that returned every spec on every diff would be
# technically safe and practically useless, and would be turned off.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/verify-changed-specs.sh"
NAME="verify-changed-specs-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

# A disposable git repo with one spec whose scenario is certified against
# src/pricing/total.ts.
make_repo() {
  local root="$WORK/$1"
  mkdir -p "$root/specs/001-pricing" "$root/src/pricing" "$root/docs"
  printf 'export const total = 1;\n' >"$root/src/pricing/total.ts"
  printf '# Docs\n' >"$root/docs/README.md"
  cat >"$root/specs/001-pricing/scenario-manifest.json" <<'JSON'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-001-001",
      "title": "Order total renders",
      "requiredTestType": "e2e-ui",
      "evidenceRefs": ["report.md#scn-1"],
      "implementationRefs": ["src/pricing/total.ts"]
    }
  ]
}
JSON
  printf '# Spec\n' >"$root/specs/001-pricing/spec.md"
  (
    cd "$root" || exit 1
    git init -q
    git config user.email "selftest@example.invalid"
    git config user.name "selftest"
    git add -A
    git commit -qm "base"
  ) >/dev/null 2>&1
  printf '%s' "$root"
}

run_vcs() {
  local root="$1"; shift
  set +e
  OUT="$(cd "$root" && bash "$TARGET" --base-ref HEAD~1 --head-ref HEAD "$@" 2>&1)"
  RC=$?
  set -e
}

commit_change() {
  local root="$1" file="$2" content="$3"
  (
    cd "$root" || exit 1
    mkdir -p "$(dirname "$file")"
    printf '%s\n' "$content" >"$file"
    git add -A
    git commit -qm "change $file"
  ) >/dev/null 2>&1
}

# --- A1. a changed planning file is discovered ------------------------------
R="$(make_repo a1)"
commit_change "$R" "specs/001-pricing/spec.md" "# Spec updated"
run_vcs "$R" --list-only
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'specs/001-pricing'; then
  ok "A1 a changed planning file discovers its spec"
else
  bad "A1 planning discovery" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2. THE BLIND SPOT: a SOURCE-ONLY diff still discovers the spec --------
R="$(make_repo a2)"
commit_change "$R" "src/pricing/total.ts" "export const total = 2;"
run_vcs "$R" --list-only
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'specs/001-pricing'; then
  ok "A2 a source-only diff still discovers the impacted certified spec"
else
  bad "A2 source-only discovery (COV-12 blind spot)" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. GUARD: an unrelated diff discovers nothing --------------------------
R="$(make_repo p3)"
commit_change "$R" "docs/README.md" "# Docs updated"
run_vcs "$R" --list-only
if [[ "$RC" -eq 0 ]] && ! printf '%s' "$OUT" | grep -q 'specs/001-pricing'; then
  ok "P3 an unrelated diff discovers no spec"
else
  bad "P3 unrelated diff over-reports" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. an empty range is clean --------------------------------------------
R="$(make_repo p4)"
set +e
OUT="$(cd "$R" && bash "$TARGET" --base-ref HEAD --head-ref HEAD 2>&1)"; RC=$?
set -e
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'no changed files'; then
  ok "P4 an empty diff range exits clean"
else
  bad "P4 empty range" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. gates actually run (not just discovery) ----------------------------
# The spec has no report.md/state.json, so artifact lint must fail and the
# command must surface it as a non-zero result rather than reporting discovery
# and exiting 0.
R="$(make_repo a5)"
commit_change "$R" "src/pricing/total.ts" "export const total = 3;"
run_vcs "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'gate failure'; then
  ok "A5 a discovered spec that fails a gate makes the command exit 1"
else
  bad "A5 gates run" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P6. non-git directory is a usage error, not a silent pass ---------------
mkdir -p "$WORK/notgit"
set +e
OUT="$(cd "$WORK/notgit" && bash "$TARGET" --base-ref HEAD 2>&1)"; RC=$?
set -e
if [[ "$RC" -eq 2 ]]; then
  ok "P6 a non-git directory exits 2 rather than passing silently"
else
  bad "P6 non-git" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7. PORTABILITY: no GNU-only xargs -d ----------------------------------
# `xargs -d` is GNU-only and would break every macOS consumer. Bubbles targets
# bash 3.2 portability, so this is asserted structurally.
#
# Comment lines are STRIPPED before the grep. The script's own comment explains
# why xargs -d is avoided, and a naive grep matches that prose and reports the
# very thing the comment says was avoided. A code-evidence check that reads
# comments is not a code-evidence check.
vcs_code_only="$(grep -vE '^[[:space:]]*#' "$TARGET")"
if ! printf '%s' "$vcs_code_only" | grep -q 'xargs -d'; then
  ok "P7 no GNU-only 'xargs -d' in code (macOS consumers stay supported)"
else
  bad "P7 portability" "verify-changed-specs.sh uses GNU-only 'xargs -d'"
fi

# --- U1. usage ---------------------------------------------------------------
set +e
bash "$TARGET" >/dev/null 2>&1; u1=$?
bypass="$(bash "$TARGET" --no-verify 2>&1)"; u2=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 a missing --base-ref and a bypass flag both exit 2"
else
  bad "U1 usage" "noarg=$u1 bypass=$u2"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
