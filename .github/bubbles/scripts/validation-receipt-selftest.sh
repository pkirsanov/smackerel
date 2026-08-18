#!/usr/bin/env bash
#
# validation-receipt-selftest.sh — prove the run receipt cannot certify a tree
# it did not see (IMP-049 / SCOPE-2).
#
# The whole value of the receipt rests on ONE property: every way of being
# uncertain re-runs the validation. So the interesting cases here are not the
# acceptance — they are the refusals, each of which must reach the real
# framework-validate call site inside release-check.sh.
#
# HOW "DID THE SUITE RUN?" IS DECIDED. The fixture stands up a real copy of
# bubbles/scripts under a synthetic repo root and replaces framework-validate.sh
# with a stub that touches a marker file. The assertion is the marker, so it
# reports what actually executed rather than what the output claims. The four
# generator gates release-check also runs are stubbed too: they are not the
# subject here, and the fixture is driven ~20 times.
#
# release-check.sh itself is NOT stubbed. It is the real file, reached through
# its real entry point, making its real decision.
#
# Exit: 0 all cases pass - 1 a case failed - 2 environment unusable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REAL_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LIB="$SCRIPT_DIR/validation-receipt.sh"

if [[ ! -f "$LIB" ]]; then
  echo "validation-receipt-selftest: lib not found: $LIB" >&2
  exit 2
fi
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "validation-receipt-selftest: SKIP (no sha256sum/shasum available)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

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

# --- fixture ---------------------------------------------------------------
FIXTURE="$WORK/repo"
mkdir -p "$FIXTURE/bubbles" "$FIXTURE/tracked"
cp -R "$SCRIPT_DIR" "$FIXTURE/bubbles/scripts"
printf '9.9.9-fixture\n' >"$FIXTURE/VERSION"

# Everything release-check requires to exist, so its verdict in the acceptance
# case is decided by the receipt rather than by fixture skeleton noise.
for required in README.md CHANGELOG.md install.sh \
  docs/CHEATSHEET.md docs/its-not-rocket-appliances.html \
  docs/generated/competitive-capabilities.md docs/generated/issue-status.md \
  docs/guides/AGENT_MANUAL.md docs/guides/INSTALLATION.md \
  docs/guides/CONTROL_PLANE_DESIGN.md docs/guides/CONTROL_PLANE_SCHEMAS.md \
  docs/recipes/framework-ops.md \
  bubbles/capability-ledger.yaml bubbles/action-risk-registry.yaml; do
  mkdir -p "$FIXTURE/$(dirname "$required")"
  printf 'fixture\n' >"$FIXTURE/$required"
done

# Three inventoried files, and a manifest that lists exactly them. A tiny
# inventory keeps the digest assertions readable; the algorithm is indifferent
# to the count.
printf 'alpha\n' >"$FIXTURE/tracked/a.txt"
printf 'beta\n' >"$FIXTURE/tracked/b.txt"
printf 'gamma\n' >"$FIXTURE/tracked/c.txt"
cat >"$FIXTURE/bubbles/release-manifest.json" <<'JSON'
{
  "schemaVersion": 1,
  "version": "9.9.9-fixture",
  "managedFileCount": 2,
  "managedFileChecksums": [
    {"path": "tracked/a.txt", "sha256": "not-read-by-the-digest"},
    {"path": "tracked/b.txt", "sha256": "not-read-by-the-digest"}
  ],
  "sourceOnlyFileCount": 1,
  "sourceOnlyFileChecksums": [
    {"path": "tracked/c.txt", "sha256": "not-read-by-the-digest"}
  ]
}
JSON

MARKER="$WORK/framework-validate-ran"
cat >"$FIXTURE/bubbles/scripts/framework-validate.sh" <<EOF
#!/usr/bin/env bash
# Stub. Its ONLY job is to be observable.
: >"$MARKER"
echo "STUB framework-validate ran"
exit 0
EOF
for stub in generate-capability-ledger-docs.sh generate-framework-stats.sh \
  generate-cheatsheet.sh generate-release-manifest.sh; do
  printf '#!/usr/bin/env bash\nexit 0\n' >"$FIXTURE/bubbles/scripts/$stub"
done
chmod +x "$FIXTURE/bubbles/scripts/framework-validate.sh"

RECEIPT_DIR="$WORK/receipts"
export BUBBLES_VALIDATION_RECEIPT_DIR="$RECEIPT_DIR"
RECEIPT="$RECEIPT_DIR/framework-validate.json"

# shellcheck source=bubbles/scripts/validation-receipt.sh
source "$LIB"

# write_valid_receipt <tier> <verdict> — recorded against the fixture's CURRENT
# content, so a digest mismatch in a case below is caused by that case alone.
write_valid_receipt() {
  validation_receipt_write "$FIXTURE" "$1" "$2" 42 3743 false false
}

# run_release_check [--no-opt-in] — "ran" or "skipped", decided by the marker.
# Also leaves release-check's exit code in rc_exit and its output in rc.out.
rc_exit=0
run_release_check() {
  rm -f "$MARKER"
  if [[ "${1:-}" == "--no-opt-in" ]]; then
    env -u BUBBLES_RELEASE_CHECK_ACCEPT_RECEIPT \
      bash "$FIXTURE/bubbles/scripts/release-check.sh" >"$WORK/rc.out" 2>&1
  else
    BUBBLES_RELEASE_CHECK_ACCEPT_RECEIPT=1 \
      bash "$FIXTURE/bubbles/scripts/release-check.sh" >"$WORK/rc.out" 2>&1
  fi
  rc_exit=$?
  [[ -f "$MARKER" ]] && echo "ran" || echo "skipped"
}

# --- digest behaviour ------------------------------------------------------
d1="$(validation_receipt_tree_digest "$FIXTURE")"
d2="$(validation_receipt_tree_digest "$FIXTURE")"
check "tree digest is deterministic" "yes" "$([[ "$d1" == "$d2" && -n "$d1" ]] && echo yes || echo no)"

printf 'alphaX\n' >"$FIXTURE/tracked/a.txt"
check "one changed inventoried byte changes the digest" "yes" \
  "$([[ "$d1" != "$(validation_receipt_tree_digest "$FIXTURE")" ]] && echo yes || echo no)"
printf 'alpha\n' >"$FIXTURE/tracked/a.txt"

mv "$FIXTURE/tracked/c.txt" "$WORK/c.hidden"
check "a DELETED inventoried file changes the digest" "yes" \
  "$([[ "$d1" != "$(validation_receipt_tree_digest "$FIXTURE")" ]] && echo yes || echo no)"
mv "$WORK/c.hidden" "$FIXTURE/tracked/c.txt"
check "restoring the file restores the digest" "$d1" "$(validation_receipt_tree_digest "$FIXTURE")"

printf 'x\n' >>"$FIXTURE/bubbles/release-manifest.json"
check "editing the manifest itself changes the digest" "yes" \
  "$([[ "$d1" != "$(validation_receipt_tree_digest "$FIXTURE")" ]] && echo yes || echo no)"
sed -i.bak '$d' "$FIXTURE/bubbles/release-manifest.json"
rm -f "$FIXTURE/bubbles/release-manifest.json.bak"
check "reverting the manifest restores the digest" "$d1" "$(validation_receipt_tree_digest "$FIXTURE")"

check "digest is refused when there is no manifest to read" "" \
  "$(validation_receipt_tree_digest "$WORK/definitely-not-a-repo" 2>/dev/null || true)"

# --- PROOF 1: matching receipt + pass verdict is consumed ------------------
write_valid_receipt full pass
check "PROOF 1 — matching pass receipt at tier=full: suite does NOT run" "skipped" "$(run_release_check)"
check "PROOF 1 — release-check still passes overall" "0" "$rc_exit"
check "PROOF 1 — the decision is reported to the operator" "yes" \
  "$(grep -q 'receipt: accepted' "$WORK/rc.out" && echo yes || echo no)"

# --- PROOF 2: one changed managed byte re-runs the suite -------------------
printf 'beta-mutated\n' >"$FIXTURE/tracked/b.txt"
check "PROOF 2 — one changed inventoried byte: suite RUNS" "ran" "$(run_release_check)"
check "PROOF 2 — the refusal names the digest mismatch" "yes" \
  "$(grep -q 'tree digest' "$WORK/rc.out" && echo yes || echo no)"
printf 'beta\n' >"$FIXTURE/tracked/b.txt"
check "PROOF 2 — reverting the byte makes the same receipt consumable again" "skipped" "$(run_release_check)"

# --- PROOF 3: verdict=fail re-runs the suite -------------------------------
write_valid_receipt full fail
check "PROOF 3 — verdict=fail: suite RUNS" "ran" "$(run_release_check)"

# --- PROOF 4: tier=core does not satisfy tier=full -------------------------
write_valid_receipt core pass
check "PROOF 4 — tier=core when full is required: suite RUNS" "ran" "$(run_release_check)"

# --- PROOF 5: missing or corrupt receipt re-runs the suite -----------------
rm -f "$RECEIPT"
check "PROOF 5a — missing receipt: suite RUNS" "ran" "$(run_release_check)"

mkdir -p "$RECEIPT_DIR"
printf '{ this is not json' >"$RECEIPT"
check "PROOF 5b — corrupt receipt: suite RUNS" "ran" "$(run_release_check)"

write_valid_receipt full pass
sed -i.bak 's/"treeDigest"/"treeDigestRenamed"/' "$RECEIPT"
rm -f "$RECEIPT.bak"
check "PROOF 5c — receipt missing a required field: suite RUNS" "ran" "$(run_release_check)"

# --- the default posture is unchanged --------------------------------------
write_valid_receipt full pass
check "DEFAULT — a perfectly valid receipt is IGNORED without the opt-in" "ran" "$(run_release_check --no-opt-in)"
check "DEFAULT — the reason says reuse is opt-in" "yes" \
  "$(grep -q 'reuse is opt-in' "$WORK/rc.out" && echo yes || echo no)"

# --- the remaining refusal preconditions -----------------------------------
mutate_receipt() { # <sed-expression> <case-name>
  write_valid_receipt full pass
  sed -i.bak "$1" "$RECEIPT"
  rm -f "$RECEIPT.bak"
  check "$2" "ran" "$(run_release_check)"
}
mutate_receipt 's/"schemaVersion": 1,/"schemaVersion": 2,/' "an unknown schemaVersion is refused"
mutate_receipt 's/"producer": "framework-validate.sh"/"producer": "someone-else.sh"/' "a foreign producer is refused"
mutate_receipt 's/"changedOnly": false/"changedOnly": true/' "a --changed-only run is refused (it did not execute every check)"
mutate_receipt 's/"cacheEnabled": false/"cacheEnabled": true/' "a cache-assisted run is refused (some verdicts were reused)"
mutate_receipt 's/"recordedAtEpoch": [0-9]*/"recordedAtEpoch": 1000000000/' "an expired receipt is refused"
mutate_receipt 's/"toolchainDigest": "[a-f0-9]*"/"toolchainDigest": "0000000000000000000000000000000000000000000000000000000000000000"/' "a different toolchain is refused"

write_valid_receipt full pass
printf '9.9.10-fixture\n' >"$FIXTURE/VERSION"
check "a framework version bump is refused" "ran" "$(run_release_check)"
printf '9.9.9-fixture\n' >"$FIXTURE/VERSION"

# --- invalidation ----------------------------------------------------------
write_valid_receipt full pass
validation_receipt_invalidate "$FIXTURE"
check "invalidate removes the receipt" "yes" "$([[ ! -e "$RECEIPT" ]] && echo yes || echo no)"
validation_receipt_invalidate "$FIXTURE" >/dev/null 2>&1
check "invalidate on an absent receipt succeeds" "0" "$?"
check "an invalidated receipt is not consumed" "ran" "$(run_release_check)"

# --- a stubbed sibling must not be able to end the consumer -----------------
# `source` runs in the caller's shell, so a truncated or stubbed
# validation-receipt.sh that merely exits would terminate release-check with
# whatever status it chose — a silent 0 that certified nothing. This is not
# hypothetical: tests/regression/test_28 stages exactly such a stub for every
# $SCRIPT_DIR sibling it does not recognise, and it caught this defect.
write_valid_receipt full pass
cp "$FIXTURE/bubbles/scripts/validation-receipt.sh" "$WORK/receipt-lib.real"
printf '#!/usr/bin/env bash\nexit 0\n' >"$FIXTURE/bubbles/scripts/validation-receipt.sh"
check "a STUBBED receipt lib does not end release-check — the suite RUNS" "ran" "$(run_release_check)"
cp "$WORK/receipt-lib.real" "$FIXTURE/bubbles/scripts/validation-receipt.sh"
check "restoring the real lib restores consumption" "skipped" "$(run_release_check)"

# --- the REAL framework-validate, on its no-execution path -----------------
# --list-tier executes no check, so it must neither write a receipt (it validated
# nothing) nor destroy one (it judged nothing). Both halves are asserted against
# the real script, not the stub; the listing path exits before the first check,
# so it costs well under a second.
if [[ -f "$REAL_ROOT/bubbles/scripts/framework-validate.sh" ]]; then
  real_dir="$WORK/real-receipts"
  mkdir -p "$real_dir"
  printf '{"sentinel": true}\n' >"$real_dir/framework-validate.json"
  BUBBLES_VALIDATION_RECEIPT_DIR="$real_dir" \
    bash "$REAL_ROOT/bubbles/scripts/framework-validate.sh" --list-tier=full >"$WORK/lt.out" 2>&1
  check "real framework-validate --list-tier exits 0" "0" "$?"
  check "real framework-validate --list-tier leaves an existing receipt untouched" "yes" \
    "$(grep -q '"sentinel": true' "$real_dir/framework-validate.json" 2>/dev/null && echo yes || echo no)"

  # The real validator's WRITE path cannot be executed here: one measured full
  # run is 3743s across 338 checks, and no selftest may spend that. What IS
  # checkable without running it is that the three call sites exist, so deleting
  # one is caught here rather than discovered as a receipt that never appears.
  fv_src="$(<"$REAL_ROOT/bubbles/scripts/framework-validate.sh")"
  check "framework-validate invalidates the previous receipt before its first check" "yes" \
    "$([[ "$fv_src" == *'validation_receipt_invalidate "$REPO_ROOT"'* ]] && echo yes || echo no)"
  check "framework-validate records a pass verdict" "yes" \
    "$([[ "$fv_src" == *'fv_write_receipt pass'* ]] && echo yes || echo no)"
  check "framework-validate records a fail verdict" "yes" \
    "$([[ "$fv_src" == *'fv_write_receipt fail'* ]] && echo yes || echo no)"

  # The digest over the REAL inventory (~1000 entries), so the algorithm is
  # exercised at production scale rather than only against the 3-file fixture.
  real_d1="$(validation_receipt_tree_digest "$REAL_ROOT")"
  check "the real repository yields a digest" "yes" "$([[ -n "$real_d1" ]] && echo yes || echo no)"
  check "the real digest is stable across two reads" "$real_d1" "$(validation_receipt_tree_digest "$REAL_ROOT")"
fi

echo
echo "validation-receipt-selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]]
