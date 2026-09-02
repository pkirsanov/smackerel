#!/usr/bin/env bash
# Regression: specs/004-phase3-intelligence/bugs/BUG-004-004-synthesis-persistence-and-health-truth
# Test Plan: T004-C11-COMPOSE-STDIN / SCN-004-004-C11
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$REPO_ROOT/scripts/lib/runtime.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_files_equal() {
  local expected="$1"
  local actual="$2"
  local description="$3"

  if ! cmp -s "$expected" "$actual"; then
    fail "$description differs (expected $(wc -c <"$expected") bytes, got $(wc -c <"$actual") bytes)"
  fi
}

set_fake_docker_logs() {
  local case_name="$1"

  FAKE_DOCKER_ARGV_LOG="$TMP/$case_name.argv"
  FAKE_DOCKER_STDIN_LOG="$TMP/$case_name.stdin"
  export FAKE_DOCKER_ARGV_LOG FAKE_DOCKER_STDIN_LOG
}

write_expected_argv() {
  local output_file="$1"
  shift

  printf '%s\0' "${compose_prefix[@]}" "$@" >"$output_file"
}

mkdir -p "$TMP/bin"
cat >"$TMP/bin/docker" <<'FAKE_DOCKER'
#!/usr/bin/env bash
set -euo pipefail

: "${FAKE_DOCKER_ARGV_LOG:?fake docker argv log is required}"
: "${FAKE_DOCKER_STDIN_LOG:?fake docker stdin log is required}"
printf '%s\0' "$@" >"$FAKE_DOCKER_ARGV_LOG"
cat >"$FAKE_DOCKER_STDIN_LOG"
FAKE_DOCKER
chmod +x "$TMP/bin/docker"
PATH="$TMP/bin:$PATH"
export PATH

env_file="$(smackerel_require_env_file test)"
[[ -r "$env_file" ]] || fail "repo-generated test env is not readable: $env_file"
compose_project="$(smackerel_env_value "$env_file" COMPOSE_PROJECT)"
enable_ollama="$(smackerel_env_value "$env_file" ENABLE_OLLAMA)"
enable_searxng="$(smackerel_env_value "$env_file" ENABLE_SEARXNG)"

compose_prefix=(
  compose
  --project-name "$compose_project"
  --env-file "$env_file"
  -f "$REPO_ROOT/docker-compose.yml"
)
if smackerel_is_truthy "$enable_ollama"; then
  compose_prefix+=(--profile ollama)
fi
if smackerel_is_truthy "$enable_searxng"; then
  compose_prefix+=(--profile searxng)
fi
compose_prefix+=(--profile test)

printf 'piped-first\npiped-second: spaces stay spaces\npiped-final-without-newline' >"$TMP/piped.expected"
set_fake_docker_logs piped
smackerel_compose_stdin_is_terminal() {
  return 1
}
cat "$TMP/piped.expected" | smackerel_compose test exec -T postgres psql --file=-
assert_files_equal "$TMP/piped.expected" "$FAKE_DOCKER_STDIN_LOG" \
  "piped exec stdin"
write_expected_argv "$TMP/piped.argv.expected" exec -T postgres psql --file=-
assert_files_equal "$TMP/piped.argv.expected" "$FAKE_DOCKER_ARGV_LOG" \
  "piped exec argv"
echo "PASS: piped stdin reaches docker compose exec byte-for-byte"
echo "PASS: piped exec argv remains unchanged"

printf 'terminal-classified input must not reach docker' >"$TMP/terminal.input"
set_fake_docker_logs terminal
smackerel_compose_stdin_is_terminal() {
  return 0
}
cat "$TMP/terminal.input" | smackerel_compose test exec core consume-stdin
[[ -f "$FAKE_DOCKER_STDIN_LOG" ]] \
  || fail "terminal-classified exec did not invoke fake docker"
[[ ! -s "$FAKE_DOCKER_STDIN_LOG" ]] \
  || fail "terminal-classified exec did not receive EOF/closed stdin"
write_expected_argv "$TMP/terminal.argv.expected" exec core consume-stdin
assert_files_equal "$TMP/terminal.argv.expected" "$FAKE_DOCKER_ARGV_LOG" \
  "terminal-classified exec argv"
echo "PASS: terminal-classified exec receives EOF from closed stdin"
echo "PASS: terminal-classified exec argv remains unchanged"

printf 'non-exec-first\nnon-exec-final-without-newline' >"$TMP/nonexec.expected"
set_fake_docker_logs nonexec
cat "$TMP/nonexec.expected" | \
  smackerel_compose test run --rm core consume-stdin "argument with spaces"
assert_files_equal "$TMP/nonexec.expected" "$FAKE_DOCKER_STDIN_LOG" \
  "non-exec stdin"
write_expected_argv "$TMP/nonexec.argv.expected" \
  run --rm core consume-stdin "argument with spaces"
assert_files_equal "$TMP/nonexec.argv.expected" "$FAKE_DOCKER_ARGV_LOG" \
  "non-exec argv"
echo "PASS: non-exec command preserves stdin even when classified as terminal"
echo "PASS: non-exec command preserves every argv boundary"

echo "ALL PASS: T004-C11-COMPOSE-STDIN shared helper regression"