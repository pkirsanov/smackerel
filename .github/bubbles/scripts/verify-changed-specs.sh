#!/usr/bin/env bash
# bubbles/scripts/verify-changed-specs.sh
#
# Downstream changed-spec verification (IMP-040 SCOPE-11 / COV-12, BUG-031).
#
# WHY THIS EXISTS
# Downstream repositories each hand-rolled their own idea of "check the specs I
# touched", or skipped it. Measured across the six consumer repos on 2026-08-12:
# five carry a pre-push hook, exactly ONE of them invokes any Bubbles guard, and
# one carries no hook at all. So the certification gates existed and simply were
# not reached — which is indistinguishable, from the outside, from having no
# gates.
#
# This is ONE generic command so a downstream repo wires a single line instead
# of reimplementing the discovery logic, because a reimplementation is what
# drifts.
#
# WHAT IT DISCOVERS, AND WHY BOTH HALVES ARE NEEDED
#   1. CHANGED PLANNING FILES — spec directories the diff touched directly.
#   2. IMPACTED CERTIFIED SCENARIOS — specs the diff did NOT touch, whose
#      implementationRefs intersect the changed SOURCE (SCOPE-9). A source-only
#      diff touches no spec folder, so half 1 alone reports nothing while
#      certified scenarios silently go stale. That is the COV-12 blind spot.
#
# WHAT IT RUNS per discovered spec: artifact lint (G010), the state transition
# guard's control-plane checks including scenario manifest resolution (G057),
# Test Plan parity, and the traceability guard (G088) when present. Each is
# invoked as the existing script; this command is a DISCOVERER and a DISPATCHER,
# never a second implementation of a check.
#
# Exit codes:
#   0  nothing to verify, or every discovered spec passed
#   1  at least one discovered spec failed a gate
#   2  usage error / not a git repository

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_REF=""
HEAD_REF="HEAD"
REPO_ROOT="$PWD"
SPECS_DIR="specs"
LIST_ONLY=0

usage() {
  cat <<'EOF'
Usage: verify-changed-specs.sh --base-ref <ref> [--head-ref <ref>] [options]

Discover the specs a diff affects — directly OR through implementationRefs —
and run the certification gates on each.

Options:
  --base-ref REF     Diff base (required)
  --head-ref REF     Diff head (default: HEAD)
  --repo-root PATH   Repository to inspect (default: $PWD)
  --specs-dir PATH   Spec root, repo-relative (default: specs)
  --list-only        Print the discovered spec dirs and exit 0
  -h, --help         Show this help

Exit: 0 clean/nothing | 1 a discovered spec failed a gate | 2 usage
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-ref)  [ "$#" -ge 2 ] || { echo "verify-changed-specs: --base-ref requires a value" >&2; exit 2; }; BASE_REF="$2"; shift 2 ;;
    --head-ref)  [ "$#" -ge 2 ] || { echo "verify-changed-specs: --head-ref requires a value" >&2; exit 2; }; HEAD_REF="$2"; shift 2 ;;
    --repo-root) [ "$#" -ge 2 ] || { echo "verify-changed-specs: --repo-root requires a value" >&2; exit 2; }; REPO_ROOT="$2"; shift 2 ;;
    --specs-dir) [ "$#" -ge 2 ] || { echo "verify-changed-specs: --specs-dir requires a value" >&2; exit 2; }; SPECS_DIR="$2"; shift 2 ;;
    --list-only) LIST_ONLY=1; shift ;;
    -h | --help) usage; exit 0 ;;
    --skip* | --force | --ignore* | --no-verify)
      echo "verify-changed-specs: '$1' is bypass-shaped and is not supported." >&2
      echo "  Unreached gates are the exact failure this closes; there is no opt-out." >&2
      exit 2 ;;
    *) echo "verify-changed-specs: unknown option: $1" >&2; exit 2 ;;
  esac
done

[ -n "$BASE_REF" ] || { usage >&2; exit 2; }
[ -d "$REPO_ROOT" ] || { echo "verify-changed-specs: repo root not found: $REPO_ROOT" >&2; exit 2; }
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

cd "$REPO_ROOT" || exit 2
git rev-parse --git-dir >/dev/null 2>&1 || {
  echo "verify-changed-specs: not a git repository: $REPO_ROOT" >&2
  exit 2
}

CHANGED="$(git diff --name-only "$BASE_REF" "$HEAD_REF" 2>/dev/null)" || {
  echo "verify-changed-specs: cannot diff '$BASE_REF'..'$HEAD_REF'" >&2
  exit 2
}

if [ -z "$CHANGED" ]; then
  echo "[verify-changed-specs] OK — no changed files between $BASE_REF and $HEAD_REF"
  exit 0
fi

# --- Half 1: spec directories the diff touched directly ---------------------
directly_changed=""
while IFS= read -r path; do
  case "$path" in
    "$SPECS_DIR"/*)
      # specs/<name>/... -> specs/<name>
      rest="${path#"$SPECS_DIR"/}"
      name="${rest%%/*}"
      [ -n "$name" ] && directly_changed="$directly_changed$SPECS_DIR/$name"$'\n'
      ;;
  esac
done <<EOF
$CHANGED
EOF
directly_changed="$(printf '%s' "$directly_changed" | sort -u | sed '/^$/d')"

# --- Half 2: specs whose implementationRefs intersect the changed source ----
# Every spec is asked, not just the touched ones — that is the whole point.
#
# Paths go in on STDIN via the resolver's --changed-from, NOT through xargs:
# `xargs -d` is GNU-only and would break every macOS consumer, and Bubbles
# targets bash 3.2 portability.
impacted=""
impact_resolver="$SCRIPT_DIR/scenario-impact-resolve.sh"
if [ -x "$impact_resolver" ] && [ -d "$SPECS_DIR" ]; then
  for manifest in "$SPECS_DIR"/*/scenario-manifest.json; do
    [ -f "$manifest" ] || continue
    spec_dir="$(dirname "$manifest")"
    if ! printf '%s\n' "$CHANGED" | bash "$impact_resolver" "$spec_dir" --changed-from - --quiet >/dev/null 2>&1; then
      impacted="$impacted$spec_dir"$'\n'
    fi
  done
fi
impacted="$(printf '%s' "$impacted" | sort -u | sed '/^$/d')"

ALL_SPECS="$(printf '%s\n%s\n' "$directly_changed" "$impacted" | sort -u | sed '/^$/d')"

if [ -z "$ALL_SPECS" ]; then
  echo "[verify-changed-specs] OK — no spec was changed or impacted by $BASE_REF..$HEAD_REF"
  exit 0
fi

echo "============================================================"
echo "  BUBBLES CHANGED-SPEC VERIFICATION (COV-12)"
echo "  Repo:  $REPO_ROOT"
echo "  Range: $BASE_REF..$HEAD_REF"
echo "============================================================"
[ -n "$directly_changed" ] && printf 'Changed planning files in:\n%s\n' "$directly_changed"
[ -n "$impacted" ] && printf 'Impacted certified scenarios in:\n%s\n' "$impacted"
echo ""

if [ "$LIST_ONLY" = "1" ]; then
  printf '%s\n' "$ALL_SPECS"
  exit 0
fi

failures=0
run_gate() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then
    printf '  ok   %s\n' "$label"
  else
    printf '  FAIL %s\n' "$label"
    printf '       re-run: %s\n' "$*"
    failures=$((failures + 1))
  fi
}

while IFS= read -r spec_dir; do
  [ -n "$spec_dir" ] || continue
  echo "--- $spec_dir ---"

  [ -x "$SCRIPT_DIR/artifact-lint.sh" ] &&
    run_gate "artifact lint (G010)" bash "$SCRIPT_DIR/artifact-lint.sh" "$spec_dir"

  [ -x "$SCRIPT_DIR/traceability-guard.sh" ] &&
    run_gate "traceability / Test Plan parity (G088)" bash "$SCRIPT_DIR/traceability-guard.sh" "$spec_dir"

  [ -x "$SCRIPT_DIR/scenario-test-resolve.sh" ] &&
    run_gate "scenario linked-test resolution (G057)" bash "$SCRIPT_DIR/scenario-test-resolve.sh" "$spec_dir" --quiet

  [ -x "$SCRIPT_DIR/scenario-obligation-lint.sh" ] &&
    run_gate "scenario obligation matrix (G057)" bash "$SCRIPT_DIR/scenario-obligation-lint.sh" "$spec_dir" --quiet

  [ -x "$SCRIPT_DIR/test-mechanism-lint.sh" ] &&
    run_gate "test mechanism coherence (G057)" bash "$SCRIPT_DIR/test-mechanism-lint.sh" "$spec_dir" --quiet

  echo ""
done <<EOF
$ALL_SPECS
EOF

echo "============================================================"
if [ "$failures" -gt 0 ]; then
  echo "  RESULT: $failures gate failure(s) across changed/impacted specs"
  echo "============================================================"
  exit 1
fi
echo "  RESULT: all changed/impacted specs pass"
echo "============================================================"
exit 0
