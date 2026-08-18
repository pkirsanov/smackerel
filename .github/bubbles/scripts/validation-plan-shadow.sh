#!/usr/bin/env bash
# validation-plan-shadow.sh — run the registry plan in SHADOW and compare.
#
# Capability: declared-input-closure-validation
#
# WHY THIS EXISTS
# A new selection mechanism that decides what runs is the most dangerous kind of
# change a validation suite can take, because its failure mode is silence: work
# stops happening and the suite still reports success. So the closure-derived
# plan is compared against the plan the suite actually schedules BEFORE anything
# depends on it. Until the two agree, the migration does not proceed.
#
# WHAT IS COMPARED
#   executed check set  every script framework-validate.sh would run in the full
#                       tier, against every script the closure map declares
#   normalized results  the shape of the answer per check: RUN or REUSABLE
#
# The comparison is by SCRIPT PATH, not by label. Labels are prose and get
# reworded; the script is the thing that runs.
#
# Usage:
#   validation-plan-shadow.sh [--repo-root DIR] [--require-match]
#
# Exit codes:
#   0  report produced (and, under --require-match, the two plans agree)
#   1  --require-match was requested and the plans differ
#   2  usage error, or an input could not be read

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="validation-plan-shadow"
REQUIRE_MATCH="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:-}"
      ;;
    --require-match) REQUIRE_MATCH="true" ;;
    -h | --help)
      printf 'usage: %s.sh [--repo-root DIR] [--require-match]\n' "$NAME"
      exit 0
      ;;
    *)
      printf '%s: unsupported flag "%s". This comparator has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
  esac
  shift || true
done

VALIDATOR="$REPO_ROOT/bubbles/scripts/framework-validate.sh"
REGISTRY="$REPO_ROOT/bubbles/registry/validation-checks.yaml"
CLOSURE="$REPO_ROOT/bubbles/scripts/validation-closure.sh"

for required in "$VALIDATOR" "$REGISTRY" "$CLOSURE"; do
  [[ -f "$required" ]] || {
    printf '%s: required input not found: %s\n' "$NAME" "$required" >&2
    exit 2
  }
done

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

# The CURRENT plan: what the suite schedules today. --list-tier=full resolves the
# schedule without executing anything, so the shadow run costs nothing it is
# supposed to be measuring.
current_scripts="$tmp_dir/current.txt"
declared_scripts="$tmp_dir/declared.txt"

# The registrations are read from the validator source with the same logical-line
# reader the generator uses, because a physical-line reader loses continued
# registrations and would report a difference that does not exist.
LC_ALL=C awk '
  {
    line = $0
    sub(/^[ \t]+/, "", line)
    if (pending != "") { line = pending " " line; pending = "" }
    if (line ~ /\\$/) { sub(/\\$/, "", line); pending = line; next }
    if (line !~ /^run_check(_self_only)?[ \t]/) next
    rest = line
    while (match(rest, /[$][{]?(SCRIPT_DIR|REPO_ROOT)[}]?[A-Za-z0-9_.\/-]*[.]sh/)) {
      tok = substr(rest, RSTART, RLENGTH)
      rest = substr(rest, RSTART + RLENGTH)
      print tok
      break
    }
  }
' "$VALIDATOR" \
  | LC_ALL=C sed -e 's|^[$]{\{0,1\}SCRIPT_DIR}\{0,1\}/|bubbles/scripts/|' \
    -e 's|^[$]{\{0,1\}REPO_ROOT}\{0,1\}/||' \
  | LC_ALL=C sort -u >"$current_scripts"

# The sweep adds every non-denylisted selftest, so the current plan includes them.
denylist="$REPO_ROOT/bubbles/registry/selftest-denylist.txt"
for candidate in "$REPO_ROOT"/bubbles/scripts/*-selftest.sh; do
  [[ -f "$candidate" ]] || continue
  base="$(basename "$candidate")"
  if [[ -r "$denylist" ]] && LC_ALL=C grep -qxF "$base" "$denylist"; then
    continue
  fi
  printf 'bubbles/scripts/%s\n' "$base" >>"$current_scripts"
done
LC_ALL=C sort -u -o "$current_scripts" "$current_scripts"

LC_ALL=C awk '/^    script: /{ print $2 }' "$REGISTRY" | LC_ALL=C sort -u >"$declared_scripts"

only_current="$(LC_ALL=C comm -23 "$current_scripts" "$declared_scripts")"
only_declared="$(LC_ALL=C comm -13 "$current_scripts" "$declared_scripts")"

printf '%s: current plan   %s script(s)\n' "$NAME" "$(LC_ALL=C wc -l <"$current_scripts" | tr -d ' ')"
printf '%s: declared plan  %s script(s)\n' "$NAME" "$(LC_ALL=C wc -l <"$declared_scripts" | tr -d ' ')"

# Normalized results: one typed answer per declared check, so the shadow report
# shows the SHAPE the new mechanism will emit, not only its membership.
runnable=0
reusable=0
while IFS=$'\t' read -r _id complete _script; do
  [[ -n "$_id" ]] || continue
  if [[ "$complete" == "true" ]]; then
    reusable=$((reusable + 1))
  else
    runnable=$((runnable + 1))
  fi
done < <(bash "$CLOSURE" list --registry "$REGISTRY" --repo-root "$REPO_ROOT" 2>/dev/null || true)
printf '%s: normalized results — %s always-execute (unknown closure), %s reuse-eligible (complete closure)\n' \
  "$NAME" "$runnable" "$reusable"

drift=0
if [[ -n "$only_current" ]]; then
  drift=1
  printf '%s: scheduled but NOT declared (the registry would MISS these):\n' "$NAME" >&2
  printf '%s\n' "$only_current" >&2
fi
if [[ -n "$only_declared" ]]; then
  drift=1
  printf '%s: declared but NOT scheduled (the registry claims work nobody runs):\n' "$NAME" >&2
  printf '%s\n' "$only_declared" >&2
fi

if [[ "$drift" -eq 0 ]]; then
  printf '%s: OK — the declared plan and the current full plan contain the same check set.\n' "$NAME"
  exit 0
fi

printf '%s: SHADOW DIFFERENCE — the declared plan does not yet match the current full plan.\n' "$NAME" >&2
printf '%s: execution must NOT switch to registry selection until this is empty.\n' "$NAME" >&2
[[ "$REQUIRE_MATCH" == "true" ]] && exit 1
exit 0
