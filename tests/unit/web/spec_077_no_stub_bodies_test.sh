#!/usr/bin/env bash
# Spec 077 SCOPE-3 — TP-077-03-06 / SCN-077-A08.
#
# Assert zero `expect(true).toBeTruthy()` documentation stubs remain
# under web/pwa/tests/. These bodies were placeholder stubs used by
# spec 073 while the PWA browser harness was not yet wired; spec 077
# SCOPE-3 either replaced them with real driver-based probes or
# deleted them. Any reintroduction would mask a regression.

set -euo pipefail

DIR="web/pwa/tests"

if [[ ! -d "$DIR" ]]; then
  echo "FAIL: $DIR missing" >&2
  exit 1
fi

violations="$(grep -RIn --include='*.spec.ts' -E 'expect\([[:space:]]*true[[:space:]]*\)\.toBeTruthy\(' "$DIR" || true)"

if [[ -n "$violations" ]]; then
  echo "FAIL: spec 077 SCN-077-A08 — found expect(true).toBeTruthy() stub bodies under $DIR:" >&2
  printf '%s\n' "$violations" >&2
  exit 1
fi

# Adversarial: write a temp file containing the forbidden body and
# verify the same grep would flag it. Ensures the regex is not a
# tautology against an empty corpus.
TMP="$(mktemp --suffix=.spec.ts)"
trap 'rm -f "$TMP"' EXIT
cat >"$TMP" <<'EOS'
import { expect, test } from '@playwright/test';
test('adversarial canary', () => {
  expect(true).toBeTruthy();
});
EOS
if ! grep -E 'expect\([[:space:]]*true[[:space:]]*\)\.toBeTruthy\(' "$TMP" >/dev/null; then
  echo "FAIL: adversarial canary not flagged — regex is tautological" >&2
  exit 1
fi

echo "PASS: spec_077_no_stub_bodies_test (TP-077-03-06 / SCN-077-A08)"
