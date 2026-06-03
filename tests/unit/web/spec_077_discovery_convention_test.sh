#!/usr/bin/env bash
# Spec 077 SCOPE-2 — Discovery convention pin (TP-077-02-01 / SCN-077-A02).
#
# Asserts that web/pwa/playwright.config.ts continues to set
# `testDir: "tests"` and `testMatch: "**/*.spec.ts"`. Together these
# two values are the entire contract for "drop a .spec.ts under
# web/pwa/tests/ and it is auto-picked-up". Silently changing either
# would orphan future specs without a build error, so this canary
# fails loud if the convention drifts.

set -euo pipefail

CONFIG="web/pwa/playwright.config.ts"

if [[ ! -f "$CONFIG" ]]; then
  echo "FAIL: $CONFIG not found" >&2
  exit 1
fi

# Allow either single- or double-quoted forms; the contract is the value.
if ! grep -E 'testDir:[[:space:]]*["'"'"']tests["'"'"']' "$CONFIG" >/dev/null; then
  echo "FAIL: $CONFIG must declare testDir: \"tests\" (spec 077 SCN-077-A02)" >&2
  grep -n "testDir" "$CONFIG" >&2 || true
  exit 1
fi

if ! grep -E 'testMatch:[[:space:]]*["'"'"']\*\*/\*\.spec\.ts["'"'"']' "$CONFIG" >/dev/null; then
  echo "FAIL: $CONFIG must declare testMatch: \"**/*.spec.ts\" (spec 077 SCN-077-A02)" >&2
  grep -n "testMatch" "$CONFIG" >&2 || true
  exit 1
fi

# Adversarial sub-test: prove the canary would fail if either pin
# were broken. Run on a temp copy so we never mutate the real file.
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
sed 's/testDir:[[:space:]]*"tests"/testDir: "elsewhere"/' "$CONFIG" >"$TMP/broken.ts"
if grep -E 'testDir:[[:space:]]*["'"'"']tests["'"'"']' "$TMP/broken.ts" >/dev/null; then
  echo "FAIL: adversarial mutation did not change the testDir value — canary is tautological" >&2
  exit 1
fi

echo "PASS: spec_077_discovery_convention_test (TP-077-02-01 / SCN-077-A02)"
