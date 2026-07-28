#!/usr/bin/env bash
#
# selftest-coverage-lint.sh — prove every selftest in the tree is executed
# (IMP-027 / SCOPE-2b).
#
# WHY THIS EXISTS
# ---------------
# framework-validate.sh enumerated its checks by hand. Adding a selftest and
# adding its `run_check` line are two separate acts, and only the first is
# required to commit — so 8 selftests sat in bubbles/scripts/ executed by
# nothing at all (interop-import, profile-transition, release-train-flag-audit,
# release-train-guard, repo-drift-report, repository-binding-conformance-guard,
# upkeep-calendar, v4.1.0). A selftest that never runs is not coverage; it is a
# file that looks like coverage.
#
# framework-validate now DISCOVERS selftests, so the class is closed by
# construction. This lint guards the remaining hole: the deny-list. It fails
# when the deny-list names a file that no longer exists, or when an entry has
# no stated reason — either of which would let the list decay into a set of
# stale excuses that quietly suppress real checks.
#
# Exit codes: 0 clean - 1 findings - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
quiet=0

usage() {
  cat <<'EOF'
selftest-coverage-lint.sh — prove every selftest is executed

Usage:
  bash bubbles/scripts/selftest-coverage-lint.sh [--repo-root <path>] [--quiet]

Exit: 0 clean - 1 findings - 2 usage/environment error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "selftest-coverage-lint: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

scripts_dir="$repo_root/bubbles/scripts"
validator="$scripts_dir/framework-validate.sh"
denylist="$repo_root/bubbles/registry/selftest-denylist.txt"

if [[ ! -d "$scripts_dir" ]]; then
  echo "selftest-coverage-lint: SKIP (bubbles/scripts not found under $repo_root)"
  exit 0
fi
if [[ ! -f "$validator" ]]; then
  echo "selftest-coverage-lint: SKIP (framework-validate.sh not found)"
  exit 0
fi

findings=0
report() {
  printf 'FINDING: %s: %s\n' "$1" "$2"
  findings=$((findings + 1))
}

# --- The discovery sweep must still exist ----------------------------------
#
# The whole guarantee rests on it. If someone removes the sweep, every
# unenumerated selftest silently stops running again — so assert its presence
# rather than trusting it.
if ! grep -q 'Discovered selftest' "$validator"; then
  report "discovery-sweep-missing" "framework-validate.sh no longer contains the selftest discovery sweep; unenumerated selftests would stop running"
fi

# --- Deny-list hygiene ------------------------------------------------------
declare -a denied=()
if [[ -f "$denylist" ]]; then
  prev_was_comment=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^[[:space:]]*# ]]; then
      prev_was_comment=1
      continue
    fi
    if [[ -z "${line//[[:space:]]/}" ]]; then
      prev_was_comment=0
      continue
    fi
    entry="${line//[[:space:]]/}"
    denied+=("$entry")
    if [[ ! -f "$scripts_dir/$entry" ]]; then
      report "denylist-stale" "$entry is denied but no such selftest exists (remove the entry)"
    fi
    if [[ "$prev_was_comment" -ne 1 ]]; then
      report "denylist-unjustified" "$entry is denied with no reason comment on the preceding line"
    fi
    prev_was_comment=0
  done <"$denylist"
fi

# --- Every selftest is either enumerated, discovered, or denied -------------
#
# Discovery covers anything not enumerated and not denied, so a finding here
# means the sweep is unreachable for that file (e.g. it lives outside the glob).
validator_text="$(cat "$validator")"
total=0
enumerated=0
discovered=0
for path in "$scripts_dir"/*-selftest.sh; do
  [[ -f "$path" ]] || continue
  name="$(basename "$path")"
  total=$((total + 1))

  is_denied=0
  for d in "${denied[@]+"${denied[@]}"}"; do
    [[ "$d" == "$name" ]] && is_denied=1 && break
  done
  if [[ "$is_denied" -eq 1 ]]; then
    continue
  fi

  if [[ "$validator_text" == *"$name"* ]]; then
    enumerated=$((enumerated + 1))
  else
    discovered=$((discovered + 1))
  fi
done

# Selftests nested below bubbles/scripts/ are outside the sweep's glob, so they
# only run if an enumerated check names them explicitly. Report the ones that
# nothing runs.
while IFS= read -r nested; do
  [[ -n "$nested" ]] || continue
  nested_name="$(basename "$nested")"
  if [[ "$validator_text" == *"$nested_name"* ]]; then
    continue
  fi
  is_denied=0
  for d in "${denied[@]+"${denied[@]}"}"; do
    [[ "$d" == "$nested_name" ]] && is_denied=1 && break
  done
  [[ "$is_denied" -eq 1 ]] && continue
  report "outside-discovery-glob" "${nested#"$repo_root"/} is a selftest below bubbles/scripts/, is not named by any enumerated check, and the discovery sweep only globs the top level — nothing runs it"
done < <(find "$scripts_dir" -mindepth 2 -name '*-selftest.sh' -type f 2>/dev/null)

if [[ "$findings" -gt 0 ]]; then
  echo "[selftest-coverage-lint] FAIL — findings: $findings"
  exit 1
fi

[[ "$quiet" -eq 1 ]] || echo "[selftest-coverage-lint] OK — $total selftest(s): $enumerated enumerated, $discovered discovered, ${#denied[@]} denied"
exit 0
