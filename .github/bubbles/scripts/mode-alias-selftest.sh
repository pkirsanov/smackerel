#!/usr/bin/env bash
# Bubbles v6 mode-alias selftest (v6.0 / B4).
#
# Enforces the invariants of bubbles/workflows/aliases.yaml:
#   1. The aliases file parses and exposes a non-empty v5Aliases map.
#   2. Every v5 mode declared in bubbles/workflows.yaml has an entry
#      under v5Aliases (no v5 mode is left behind).
#   3. No v5 mode is referenced under v5Aliases that does not exist in
#      bubbles/workflows.yaml.
#   4. Every primitive used by v5Aliases is one of the 15 canonical
#      v6 primitives listed under v6Primitives.
#   5. Every (primitive, tag-set) tuple is unique across v5Aliases.
#   6. For each v5 mode M, resolving the v6 form (primitive + tags) via
#      mode-resolver.sh --resolve-v6 returns M.
#   7. Resolving each v5 mode AND its matching v6 form produces a
#      byte-identical resolved mode definition.
#   8. Adversarial fixtures fail loudly:
#      a. Unknown v6 primitive -> mode-resolver rejects.
#      b. Unknown tag for known primitive -> mode-resolver rejects.
#      c. Duplicate (primitive, tag) tuple -> mode-resolver rejects.
#
# Replaces nothing; complements mode-resolver-selftest.sh which proves
# template inheritance correctness.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"
ALIASES_FILE="$REPO_ROOT/bubbles/workflows/aliases.yaml"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

[[ -x "$RESOLVER" ]] || { echo "mode-alias-selftest: missing resolver: $RESOLVER" >&2; exit 2; }
[[ -f "$WORKFLOWS_FILE" ]] || { echo "mode-alias-selftest: missing workflows file" >&2; exit 2; }
[[ -f "$ALIASES_FILE" ]] || { echo "mode-alias-selftest: missing aliases file: $ALIASES_FILE" >&2; exit 2; }
command -v yq >/dev/null 2>&1 || { echo "mode-alias-selftest: yq required" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "mode-alias-selftest: python3 required" >&2; exit 2; }

# Cache aliased modes once.
v5_in_workflows="$(bash "$RESOLVER" --list-modes | grep -v '^phaseRelevance$' | sort -u)"
v5_in_aliases="$(yq -r '.v5Aliases | keys[]' "$ALIASES_FILE" | sort -u)"
v6_primitives="$(yq -r '.v6Primitives[]' "$ALIASES_FILE" | sort -u)"

# 1. parse + non-empty
if [[ -n "$v5_in_aliases" ]]; then
  pass "aliases.yaml parses and v5Aliases is non-empty"
else
  fail "aliases.yaml v5Aliases is empty"
fi

# 2. coverage: every v5 mode has an alias entry
missing_in_aliases="$(comm -23 <(echo "$v5_in_workflows") <(echo "$v5_in_aliases"))"
if [[ -z "$missing_in_aliases" ]]; then
  v5_count="$(echo "$v5_in_workflows" | grep -c .)"
  pass "every v5 mode in workflows.yaml has an alias entry ($v5_count)"
else
  fail "v5 modes missing from aliases.yaml:"
  echo "$missing_in_aliases" | sed 's/^/    /'
fi

# 3. inverse: no alias entry refers to an unknown v5 mode
extra_in_aliases="$(comm -13 <(echo "$v5_in_workflows") <(echo "$v5_in_aliases"))"
if [[ -z "$extra_in_aliases" ]]; then
  pass "no alias entry refers to an unknown v5 mode"
else
  fail "alias entries reference unknown v5 modes:"
  echo "$extra_in_aliases" | sed 's/^/    /'
fi

# 4. every primitive is canonical
unknown_primitive="$(yq -r '.v5Aliases | to_entries[] | .value.primitive' "$ALIASES_FILE" | sort -u | comm -23 - <(echo "$v6_primitives"))"
if [[ -z "$unknown_primitive" ]]; then
  pass "every primitive used by v5Aliases is one of the 15 canonical v6 primitives"
else
  fail "alias entries use non-canonical primitives:"
  echo "$unknown_primitive" | sed 's/^/    /'
fi

# 5. every (primitive, tag-set) tuple is unique
dup_tuples="$(bash "$RESOLVER" --list-aliases | awk -F'\t' '{print $2 "\t" $3}' | sort | uniq -d)"
if [[ -z "$dup_tuples" ]]; then
  pass "every (primitive, tag-set) tuple is unique"
else
  fail "duplicate (primitive, tag-set) tuples:"
  echo "$dup_tuples" | sed 's/^/    /'
fi

# 6. each v6 form resolves to its v5 mode
round_trip_failed=0
while IFS=$'\t' read -r v5 prim tags; do
  [[ -z "$v5" ]] && continue
  resolved="$(bash "$RESOLVER" --resolve-v6 "$prim" $tags 2>/dev/null || true)"
  if [[ "$resolved" != "$v5" ]]; then
    fail "v6 form '$prim ${tags}' resolved to '$resolved' (expected '$v5')"
    round_trip_failed=$((round_trip_failed + 1))
  fi
done < <(bash "$RESOLVER" --list-aliases)
if [[ "$round_trip_failed" -eq 0 ]]; then
  alias_count="$(bash "$RESOLVER" --list-aliases | wc -l)"
  pass "every v6 form (primitive + tags) resolves back to its v5 mode ($alias_count round-trips)"
fi

# 7. byte-identical resolution between v5 invocation and v6 invocation.
#    Step 6 already proves round-trip name resolution. This step proves the
#    YAML bytes are the same. Resolving every mode through both invocation
#    paths makes the selftest dominated by yq fork overhead (~10s per pair
#    × 55 modes = ~10 min). Default mode covers a representative slice; set
#    BUBBLES_MODE_ALIAS_FULL_PARITY=1 to cover every alias (used by release
#    gating and by maintainers debugging resolver drift).
if [[ "${BUBBLES_MODE_ALIAS_FULL_PARITY:-0}" == "1" ]]; then
  parity_subset="$(bash "$RESOLVER" --list-aliases)"
  parity_label="all $(echo "$parity_subset" | wc -l)"
else
  # Cover one entry per primitive plus the 3 selftestExpectations examples.
  # bash awk: one row per unique primitive (column 2). 15 max + expectations.
  parity_subset="$(bash "$RESOLVER" --list-aliases | awk -F'\t' '!seen[$2]++ {print}')"
  parity_label="$(echo "$parity_subset" | wc -l) (one per primitive; full set under BUBBLES_MODE_ALIAS_FULL_PARITY=1)"
fi
parity_failed=0
parity_checked=0
while IFS=$'\t' read -r v5 prim tags; do
  [[ -z "$v5" ]] && continue
  # v7: the v5-NAME invocation is the grandfathered/persisted path (bare v5
  # names are rejected only for new operator input). Grandfather it here so the
  # byte-for-byte parity against the v6-form path still verifies that both
  # invocations resolve to the same mode definition.
  v5_resolved="$(BUBBLES_MODE_GRANDFATHER=1 bash "$RESOLVER" "$v5" 2>/dev/null)"
  # shellcheck disable=SC2086
  v6_resolved="$(bash "$RESOLVER" "$prim" $tags 2>/dev/null)"
  if [[ "$v5_resolved" != "$v6_resolved" ]]; then
    fail "resolution drift for '$v5' vs v6 form '$prim ${tags}'"
    parity_failed=$((parity_failed + 1))
  fi
  parity_checked=$((parity_checked + 1))
done <<< "$parity_subset"
if [[ "$parity_failed" -eq 0 ]]; then
  pass "resolved-mode definitions match byte-for-byte between v5 and v6 invocation ($parity_label pairs)"
fi

# 8a. adversarial: unknown v6 primitive is rejected
if bash "$RESOLVER" --resolve-v6 nonexistent-primitive action:foo 2>/dev/null; then
  fail "adversarial: resolver accepted unknown v6 primitive"
else
  pass "adversarial: resolver rejects unknown v6 primitive"
fi

# 8b. adversarial: unknown tag for known primitive is rejected
if bash "$RESOLVER" --resolve-v6 ship action:bogus 2>/dev/null; then
  fail "adversarial: resolver accepted unknown tag for known primitive"
else
  pass "adversarial: resolver rejects unknown tag for known primitive"
fi

# 8c. adversarial: duplicate (primitive, tag) tuple is rejected by an
# alternate aliases file passed via BUBBLES_WORKFLOW_ALIASES_FILE.
# yq runs under snap on some platforms and cannot read /tmp; use $HOME.
_alias_fixture_base="${HOME}/.cache/bubbles-mode-alias-selftest"
mkdir -p "$_alias_fixture_base"
# BSD/macOS mktemp lacks GNU `--suffix`; create the temp file then rename to add
# the .yaml extension (portable across GNU + BSD).
tmp_alias="$(mktemp -p "$_alias_fixture_base")"
mv "$tmp_alias" "${tmp_alias}.yaml"
tmp_alias="${tmp_alias}.yaml"
trap 'rm -f "$tmp_alias"' EXIT
cat > "$tmp_alias" <<EOF
version: 1
v6Primitives: [ship]
v5Aliases:
  alpha-mode:
    primitive: ship
    tags: {action: promote}
    description: First alias.
  beta-mode:
    primitive: ship
    tags: {action: promote}
    description: Duplicate tuple — should be rejected at resolution time.
EOF
out="$(BUBBLES_WORKFLOW_ALIASES_FILE="$tmp_alias" bash "$RESOLVER" --resolve-v6 ship action:promote 2>&1 || true)"
if echo "$out" | grep -q 'ambiguous'; then
  pass "adversarial: resolver rejects duplicate (primitive, tag-set) tuple"
else
  fail "adversarial: resolver did NOT reject duplicate tuple (got: $out)"
fi

# 9. selftestExpectations sanity: every documented example resolves.
expectation_count="$(yq -r '.selftestExpectations | length // 0' "$ALIASES_FILE")"
if [[ "$expectation_count" -gt 0 ]]; then
  expectation_failures=0
  while IFS=$'\t' read -r input expect_v5; do
    [[ -z "$input" ]] && continue
    # shellcheck disable=SC2086
    actual="$(bash "$RESOLVER" --resolve-v6 $input 2>/dev/null || true)"
    if [[ "$actual" != "$expect_v5" ]]; then
      fail "expectations: '$input' resolved to '$actual' (expected '$expect_v5')"
      expectation_failures=$((expectation_failures + 1))
    fi
  done < <(yq -r '.selftestExpectations[] | [.input, .expectV5Mode] | @tsv' "$ALIASES_FILE")
  if [[ "$expectation_failures" -eq 0 ]]; then
    pass "every selftestExpectations row resolves to the documented v5 mode ($expectation_count rows)"
  fi
else
  pass "selftestExpectations is empty (skipped)"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "mode-alias-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "mode-alias-selftest: PASS"
exit 0
