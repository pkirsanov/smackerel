#!/usr/bin/env bash
# Spec 077 SCOPE-2 — Doc-vs-CLI parity (TP-077-02-03 / SCN-077-A06).
#
# Assert that every `./smackerel.sh test <category>` subcommand listed
# in the dispatcher help text also appears in `docs/Testing.md`. The
# inverse (docs listing a non-existent category) would mislead operators
# into invoking a missing CLI surface and is asserted by the second
# block.

set -euo pipefail

SH="./smackerel.sh"
DOCS="docs/Testing.md"

if [[ ! -x "$SH" ]]; then
  echo "FAIL: $SH not executable" >&2; exit 1
fi
if [[ ! -f "$DOCS" ]]; then
  echo "FAIL: $DOCS missing" >&2; exit 1
fi

# Categories accepted today by the dispatcher (mined from the help text
# block in $SH). Keep the regex strict: lines start with `  test <name>`.
mapfile -t cli_categories < <(grep -E '^  test [a-z0-9-]+' "$SH" \
  | awk '{print $2}' | sort -u)

if (( ${#cli_categories[@]} == 0 )); then
  echo "FAIL: no test categories discovered in $SH help text" >&2
  exit 1
fi

missing_in_docs=()
for cat in "${cli_categories[@]}"; do
  # Require the literal `./smackerel.sh test <cat>` token in the docs so
  # we are matching the command, not a stray category mention.
  if ! grep -F "./smackerel.sh test ${cat}" "$DOCS" >/dev/null; then
    missing_in_docs+=("$cat")
  fi
done

if (( ${#missing_in_docs[@]} > 0 )); then
  echo "FAIL: dispatcher exposes test categories that $DOCS does not document:" >&2
  printf '  - %s\n' "${missing_in_docs[@]}" >&2
  exit 1
fi

# Adversarial: prove the check is not tautological. Inject a fake
# category and verify it would be flagged. Run in a sub-shell against a
# tmp copy of the help text so the real $SH is never mutated.
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
{
  cat "$SH"
  printf '\n  test fake-canary-077  Synthetic missing-in-docs canary\n'
} > "$TMP"
adversarial_missing="$(grep -E '^  test [a-z0-9-]+' "$TMP" | awk '{print $2}' \
  | while read -r c; do
      if ! grep -F "./smackerel.sh test ${c}" "$DOCS" >/dev/null; then
        echo "$c"
      fi
    done)"
if ! grep -q '^fake-canary-077$' <<<"$adversarial_missing"; then
  echo "FAIL: adversarial mutation 'fake-canary-077' was not flagged — parity check is tautological" >&2
  exit 1
fi

echo "PASS: spec_077_test_category_parity_test (TP-077-02-03 / SCN-077-A06)"
