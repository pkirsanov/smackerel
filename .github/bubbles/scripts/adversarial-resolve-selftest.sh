#!/usr/bin/env bash
set -uo pipefail

# adversarial-resolve-selftest.sh — IMP-002 SCOPE-0 control-plane resolver.
#
# ADVERSARIAL by design: includes precedence cases that FAIL if the
# directive > env > config > default ordering is broken, and validation cases
# that FAIL if invalid mode/passes/teeth are silently accepted. A tautological
# selftest (all cases satisfy a broken resolver) is forbidden.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/adversarial-resolve.sh"

pass=0
fail=0

# Stage the hermetic workspace UNDER $HOME so a snap-confined `yq` can read the
# fixture config (strict snap confinement cannot read /tmp). Mirrors the shipped
# observability-endpoint-resolve-selftest.sh pattern.
tmp="$(mktemp -d "${HOME}/.bubbles-selftest-adversarial.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

empty_repo="$tmp/empty"
mkdir -p "$empty_repo/.github"

cfg_repo="$tmp/cfg"
mkdir -p "$cfg_repo/.github"
printf 'adversarial:\n  mode: auto\n  passes: 5\n  teeth: blocking\n' \
  > "$cfg_repo/.github/bubbles-project.yaml"

assert_line() { # label  output  expected_exact_line
  if printf '%s\n' "$2" | grep -qx -- "$3"; then
    pass=$((pass + 1))
  else
    echo "FAIL: $1 (want line: $3)"
    printf '%s\n' "$2" | sed 's/^/    /'
    fail=$((fail + 1))
  fi
}

# --- 1. Zero-config → OFF by default ---
out="$(env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" 2>/dev/null)"
assert_line "default mode off" "$out" "mode=off"
assert_line "default passes 1" "$out" "passes=1"
assert_line "default teeth warn" "$out" "teeth=warn"
assert_line "default source" "$out" "source=default"

# --- 2. Env sets auto ---
out="$(env -i PATH="$PATH" BUBBLES_ADVERSARIAL=auto bash "$RESOLVE" --repo-root "$empty_repo" 2>/dev/null)"
assert_line "env auto mode" "$out" "mode=auto"
assert_line "env auto source" "$out" "source=env"

# --- 3. ADVERSARIAL: directive OFF beats env ON + config AUTO ---
out="$(env -i PATH="$PATH" BUBBLES_ADVERSARIAL=on bash "$RESOLVE" --repo-root "$cfg_repo" --mode off 2>/dev/null)"
assert_line "directive beats env+config (mode)" "$out" "mode=off"
assert_line "directive beats env+config (source)" "$out" "source=directive"

# --- 4. Env beats config ---
out="$(env -i PATH="$PATH" BUBBLES_ADVERSARIAL=on bash "$RESOLVE" --repo-root "$cfg_repo" 2>/dev/null)"
assert_line "env beats config (mode)" "$out" "mode=on"
assert_line "env beats config (source)" "$out" "source=env"

# --- 5. Config layer used when no env/directive (requires yq) ---
if command -v yq >/dev/null 2>&1; then
  out="$(env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$cfg_repo" 2>/dev/null)"
  assert_line "config mode" "$out" "mode=auto"
  assert_line "config passes" "$out" "passes=5"
  assert_line "config teeth" "$out" "teeth=blocking"
  assert_line "config source" "$out" "source=config"
else
  echo "SKIP: config-layer cases (yq not installed)"
fi

# --- 6. Directive string token parse ---
out="$(env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" --directive "adversarial: on passes: 3 teeth: blocking" 2>/dev/null)"
assert_line "directive str mode" "$out" "mode=on"
assert_line "directive str passes" "$out" "passes=3"
assert_line "directive str teeth" "$out" "teeth=blocking"

# --- 7-10. Validation: invalid values exit 1 (no silent accept) ---
if env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" --mode bogus >/dev/null 2>&1; then
  echo "FAIL: invalid mode silently accepted"; fail=$((fail + 1))
else
  pass=$((pass + 1))
fi
if env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" --passes 0 >/dev/null 2>&1; then
  echo "FAIL: invalid passes=0 silently accepted"; fail=$((fail + 1))
else
  pass=$((pass + 1))
fi
if env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" --passes abc >/dev/null 2>&1; then
  echo "FAIL: invalid passes=abc silently accepted"; fail=$((fail + 1))
else
  pass=$((pass + 1))
fi
if env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" --teeth nope >/dev/null 2>&1; then
  echo "FAIL: invalid teeth silently accepted"; fail=$((fail + 1))
else
  pass=$((pass + 1))
fi

# --- 11. No bypass flag: --force is a usage error (exit 2) ---
if env -i PATH="$PATH" bash "$RESOLVE" --repo-root "$empty_repo" --force >/dev/null 2>&1; then
  echo "FAIL: --force was accepted (no bypass flag may exist)"; fail=$((fail + 1))
else
  pass=$((pass + 1))
fi

echo "adversarial-resolve-selftest: $pass passed, $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
