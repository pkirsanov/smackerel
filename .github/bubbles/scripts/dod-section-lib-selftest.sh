#!/usr/bin/env bash
set -uo pipefail

# Selftest for bubbles/scripts/dod-section-lib.sh (BUG-026 shared DoD parser).
# Proves the tiered-DoD boundary (F002): a DoD section retains nested tier
# subheadings through depth 6 and ends only at a same-or-shallower heading;
# fenced/commented headings are inert; missing/rowless/read_error are distinct.
#
# Exit codes: 0 = all assertions pass, 1 = a contract failure, 2 = harness error.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/dod-section-lib.sh"

if [[ ! -f "$LIB" ]]; then
  printf 'dod-section-lib-selftest: missing %s\n' "$LIB" >&2
  exit 2
fi
for c in awk mktemp rm; do
  command -v "$c" >/dev/null 2>&1 || { printf 'dod-section-lib-selftest: missing command %s\n' "$c" >&2; exit 2; }
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/dod-section-lib-selftest-XXXXXX")" || { printf 'cannot mktemp\n' >&2; exit 2; }
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }

run_lib() { bash "$LIB" "$1"; }

# --- Fixture 1: tiered DoD (F002) — depth-3 DoD with depth-4 tier subheadings ---
cat >"$WORK/tiered.md" <<'EOF'
## Scope 01
### Definition of Done
#### Core Delivery Items
- [x] Implement the resolver
- [ ] Wire the guard
#### Build Quality Gate
- [x] Selftest green
## Next Scope
- [ ] not part of DoD
EOF
out="$(run_lib "$WORK/tiered.md")"
cb="$(printf '%s\n' "$out" | grep -c '^CHECKBOX')"
if [[ "$cb" -eq 3 ]]; then pass "tiered DoD keeps all 3 checkboxes under depth-4 tiers (F002)"; else fail "tiered DoD checkbox count expected 3, got $cb"; fi
if printf '%s\n' "$out" | grep -qE '^STATUS	rows	3 row'; then pass "tiered DoD STATUS=rows 3"; else fail "tiered DoD STATUS not 'rows 3'"; fi
if printf '%s\n' "$out" | grep -q 'not part of DoD'; then fail "content after shallower heading leaked into DoD"; else pass "content after same-or-shallower heading excluded (boundary correct)"; fi

# --- Fixture 2: no DoD section ---
cat >"$WORK/none.md" <<'EOF'
## Scope
### Test Plan
- some row
EOF
out="$(run_lib "$WORK/none.md")"
if printf '%s\n' "$out" | grep -qE '^STATUS	missing'; then pass "no DoD section -> STATUS missing"; else fail "no DoD section did not yield STATUS missing"; fi

# --- Fixture 3: fenced heading + checkbox are inert ---
cat >"$WORK/fence.md" <<'EOF'
### DoD
```
- [x] fenced not real
```
- [x] real item
EOF
out="$(run_lib "$WORK/fence.md")"
cb="$(printf '%s\n' "$out" | grep -c '^CHECKBOX')"
if [[ "$cb" -eq 1 ]]; then pass "fenced checkbox is inert (only 1 real row)"; else fail "fenced-inert expected 1 checkbox, got $cb"; fi

# --- Fixture 4: rowless DoD ---
cat >"$WORK/rowless.md" <<'EOF'
### Definition of Done
Some prose but no checklist rows.
## After
EOF
out="$(run_lib "$WORK/rowless.md")"
if printf '%s\n' "$out" | grep -qE '^STATUS	rowless'; then pass "DoD with no rows -> STATUS rowless"; else fail "rowless DoD did not yield STATUS rowless"; fi

# --- Fixture 5: HTML-commented DoD heading is inert ---
cat >"$WORK/comment.md" <<'EOF'
<!--
### Definition of Done
- [x] hidden
-->
## Real
EOF
out="$(run_lib "$WORK/comment.md")"
if printf '%s\n' "$out" | grep -qE '^STATUS	missing'; then pass "commented DoD heading is inert -> STATUS missing"; else fail "commented DoD heading was not inert"; fi

# --- Fixture 6: depth-5 heading does NOT start a DoD section ---
cat >"$WORK/deep.md" <<'EOF'
##### Definition of Done
- [x] too deep to start a section
EOF
out="$(run_lib "$WORK/deep.md")"
if printf '%s\n' "$out" | grep -qE '^STATUS	missing'; then pass "depth-5 DoD heading does not start a section"; else fail "depth-5 heading incorrectly started a DoD section"; fi

# --- Fixture 7: read_error on a missing file ---
out="$(run_lib "$WORK/does-not-exist.md")"
if printf '%s\n' "$out" | grep -qE '^STATUS	read_error'; then pass "unreadable file -> STATUS read_error"; else fail "unreadable file did not yield STATUS read_error"; fi

# --- Every parse emits exactly one terminal STATUS ---
sc="$(run_lib "$WORK/tiered.md" | grep -c '^STATUS')"
if [[ "$sc" -eq 1 ]]; then pass "exactly one terminal STATUS record per parse"; else fail "expected exactly 1 STATUS record, got $sc"; fi

printf 'ASSERTIONS=%s PASSED=%s FAILED=%s\n' "$((PASS + FAIL))" "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]] || exit 1
exit 0
