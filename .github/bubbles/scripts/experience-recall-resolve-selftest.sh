#!/usr/bin/env bash
# Hermetic contract tests for experience-recall adapter resolution.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/experience-recall-resolve.sh"
ADAPTERS="$(cd "$SCRIPT_DIR/.." && pwd)/adapters/experience-recall"
PASS=0
FAIL=0
WORK="$(mktemp -d)"

cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

pass() {
  PASS=$((PASS + 1))
  echo "PASS: $1"
}

fail() {
  FAIL=$((FAIL + 1))
  echo "FAIL: $1"
}

assert_exit() {
  if [ "$2" -eq "$3" ]; then
    pass "$1"
  else
    fail "$1 (expected exit $3, got $2)"
  fi
}

assert_contains() {
  case "$3" in
    *"$2"*) pass "$1" ;;
    *) fail "$1 (expected '$2', got: $3)" ;;
  esac
}

new_repo() {
  local repo="$WORK/$1"
  mkdir -p "$repo/.github"
  echo "$repo"
}

write_config() {
  printf '%s' "$2" >"$1/.github/bubbles-project.yaml"
}

write_root_config() {
  printf '%s' "$2" >"$1/bubbles-project.yaml"
}

echo "experience-recall-resolve-selftest"

repo="$(new_repo absent)"
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "absent config exits zero" "$rc" 0
assert_contains "absent config resolves none" "adapter=none" "$output"
assert_contains "absent config resolves the neutral script" "adapterPath=$ADAPTERS/none.sh" "$output"

repo="$(new_repo unrelated)"
write_config "$repo" 'observability:
  adapter: prometheus
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
assert_contains "unrelated config resolves none" "adapter=none" "$output"

repo="$(new_repo explicit-none)"
write_config "$repo" 'experienceRecall:
  adapter: none
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
assert_contains "explicit none resolves none" "adapter=none" "$output"

repo="$(new_repo root-config)"
write_root_config "$repo" 'experienceRecall:
  adapter: none
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
assert_contains "root project config resolves none" "adapter=none" "$output"

repo="$(new_repo config-precedence)"
write_root_config "$repo" 'experienceRecall:
  adapter: missing-provider
'
write_config "$repo" 'experienceRecall:
  adapter: none
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit ".github project config takes precedence" "$rc" 0
assert_contains "lower-priority root adapter is ignored" "adapter=none" "$output"

repo="$(new_repo quoted-none)"
write_config "$repo" 'experienceRecall:
  adapter: "none"
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
assert_contains "quoted none is normalized" "adapter=none" "$output"

repo="$(new_repo block-scope)"
write_config "$repo" 'observability:
  adapter: missing-provider
experienceRecall:
  adapter: none
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
assert_contains "foreign adapter key is ignored" "adapter=none" "$output"

repo="$(new_repo commented)"
write_config "$repo" 'experienceRecall:
  # adapter: missing-provider
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
assert_contains "commented adapter is ignored" "adapter=none" "$output"

repo="$(new_repo traversal)"
write_config "$repo" 'experienceRecall:
  adapter: ../../../etc/passwd
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "path traversal fails loud" "$rc" 1
assert_contains "path traversal reports invalid token" "invalid experienceRecall.adapter" "$output"

repo="$(new_repo uppercase)"
write_config "$repo" 'experienceRecall:
  adapter: None
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "uppercase provider fails loud" "$rc" 1

repo="$(new_repo leading-hyphen)"
write_config "$repo" 'experienceRecall:
  adapter: -none
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "leading-hyphen provider fails loud" "$rc" 1

repo="$(new_repo shell-meta)"
write_config "$repo" 'experienceRecall:
  adapter: none;touch
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "shell-metacharacter provider fails loud" "$rc" 1

repo="$(new_repo unknown)"
write_config "$repo" 'experienceRecall:
  adapter: missing-provider
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "unknown provider fails loud" "$rc" 1
assert_contains "unknown provider names the missing adapter" "has no adapter" "$output"

repo="$(new_repo local-lexical)"
write_config "$repo" 'experienceRecall:
  adapter: local-lexical
'
output="$(bash "$RESOLVE" --repo-root "$repo" 2>&1)"
rc=$?
assert_exit "local lexical provider resolves when explicitly configured" "$rc" 0
assert_contains "local lexical provider resolves its shipped adapter" "adapterPath=$ADAPTERS/local-lexical.sh" "$output"

repo="$(new_repo names-only)"
output="$(bash "$RESOLVE" --repo-root "$repo" --names-only 2>&1)"
if [ "$output" = "adapter=none" ]; then
  pass "names-only emits only the adapter"
else
  fail "names-only emitted unexpected fields: $output"
fi

if grep -Eq '(^|[^[:alpha:]])(curl|wget|pip|npm|npx|brew|apt|git[[:space:]]+clone)([^[:alpha:]]|$)' \
  "$RESOLVE" "$ADAPTERS/none.sh"; then
  fail "resolver or neutral adapter contains an installer or network command"
else
  pass "resolver and neutral adapter contain no installer or network command"
fi

echo "experience-recall-resolve-selftest: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
