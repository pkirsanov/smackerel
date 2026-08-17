#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/continuation-intent-resolve.sh"
checks=0
failures=0

expect_class() {
  local expected="$1"
  local input="$2"
  local actual
  checks=$((checks + 1))
  actual="$(bash "$RESOLVER" "$input")"
  if [[ "$actual" == "$expected" ]]; then
    printf 'PASS: %-9s %s\n' "$expected" "$input"
  else
    printf 'FAIL: %s classified as %s, expected %s\n' "$input" "$actual" "$expected"
    failures=$((failures + 1))
  fi
}

expect_class CONTINUE "continue"
expect_class CONTINUE "continue working on the booking feature"
expect_class CONTINUE "resume spec 042"
expect_class CONTINUE "next"
expect_class CONTINUE "keep going until done"
expect_class CONTINUE "go on with auth"
expect_class CONTINUE "proceed"
expect_class CONTINUE "fix all found from the last sweep"
expect_class CONTINUE "address the rest"
expect_class NEW_WORK "pick the next priority and start it"
expect_class NEW_WORK "start new work"
expect_class NEW_WORK "next-on-the-board"
expect_class OTHER ""
expect_class OTHER "implement the booking feature"
expect_class OTHER "fix tests for the page builder"

printf 'continuation-intent-resolve-selftest: checks=%d failures=%d\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]]