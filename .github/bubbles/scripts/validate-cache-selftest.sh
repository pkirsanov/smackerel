#!/usr/bin/env bash
# validate-cache-selftest.sh — hermetic selftest for the SCOPE-7 result cache.
#
# The cache is only safe because it refuses everything that is not a hermetic
# selftest. These cases pin that refusal: a cache that accepted a live guard
# would report a verdict about a working tree it never inspected, which is the
# exact class of dishonesty the framework's SEC-2 work removed.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/validate-cache.sh"

if [[ ! -f "$LIB" ]]; then
  echo "validate-cache-selftest: lib not found: $LIB" >&2
  exit 2
fi
if ! command -v sha256sum >/dev/null 2>&1; then
  echo "validate-cache-selftest: SKIP (sha256sum not available)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

export BUBBLES_VALIDATE_CACHE_DIR="$WORK/cache"
unset CI
# shellcheck source=bubbles/scripts/validate-cache.sh
source "$LIB"

pass_count=0
fail_count=0

check() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS  $name"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  $name — expected '$expected', got '$actual'"
    fail_count=$((fail_count + 1))
  fi
}

printf '#!/usr/bin/env bash\nexit 0\n' >"$WORK/thing-selftest.sh"
printf '#!/usr/bin/env bash\nexit 0\n' >"$WORK/some-guard.sh"

# --- only hermetic selftests get a key -------------------------------------
key="$(validate_cache_key "$WORK/thing-selftest.sh" 1.0.0 || true)"
check "a selftest yields a cache key" "yes" "$([[ -n "$key" ]] && echo yes || echo no)"

guard_key="$(validate_cache_key "$WORK/some-guard.sh" 1.0.0 || true)"
check "a live guard is REFUSED a cache key" "yes" "$([[ -z "$guard_key" ]] && echo yes || echo no)"

missing_key="$(validate_cache_key "$WORK/absent-selftest.sh" 1.0.0 || true)"
check "a missing script is refused" "yes" "$([[ -z "$missing_key" ]] && echo yes || echo no)"

# --- miss, then store, then hit --------------------------------------------
validate_cache_get "$key" && got=hit || got=miss
check "cold lookup misses" "miss" "$got"

validate_cache_put "$key" 0
validate_cache_get "$key" && got=hit || got=miss
check "after a PASS is stored, lookup hits" "hit" "$got"

# --- failures are never cached ---------------------------------------------
printf '#!/usr/bin/env bash\nexit 1\n' >"$WORK/failing-selftest.sh"
fail_key="$(validate_cache_key "$WORK/failing-selftest.sh" 1.0.0)"
validate_cache_put "$fail_key" 1
validate_cache_get "$fail_key" && got=hit || got=miss
check "a FAILING verdict is never cached" "miss" "$got"

# --- editing the script invalidates the entry -------------------------------
printf '#!/usr/bin/env bash\n# changed\nexit 0\n' >"$WORK/thing-selftest.sh"
new_key="$(validate_cache_key "$WORK/thing-selftest.sh" 1.0.0)"
check "editing the script changes its key" "yes" "$([[ "$new_key" != "$key" ]] && echo yes || echo no)"
validate_cache_get "$new_key" && got=hit || got=miss
check "an edited script misses the cache" "miss" "$got"

# --- a version bump invalidates every entry --------------------------------
bumped="$(validate_cache_key "$WORK/thing-selftest.sh" 2.0.0)"
check "a framework version bump changes the key" "yes" "$([[ "$bumped" != "$new_key" ]] && echo yes || echo no)"

# --- CI never uses the cache ------------------------------------------------
validate_cache_put "$new_key" 0
export CI=true
validate_cache_get "$new_key" && got=hit || got=miss
check "CI bypasses the cache (cold, complete runs)" "miss" "$got"
unset CI

# --- explicit opt-out -------------------------------------------------------
export BUBBLES_VALIDATE_CACHE=0
validate_cache_get "$new_key" && got=hit || got=miss
check "BUBBLES_VALIDATE_CACHE=0 disables lookups" "miss" "$got"
export BUBBLES_VALIDATE_CACHE=1

echo ""
echo "validate-cache selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All validate-cache selftests passed."
exit 0
