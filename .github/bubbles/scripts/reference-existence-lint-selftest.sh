#!/usr/bin/env bash
# reference-existence-lint-selftest.sh — hermetic selftest for
# reference-existence-lint.sh (gate G132).
#
# Builds throwaway markdown fixtures and asserts the lint separates a real
# phantom reference from the many shapes that only LOOK like one: placeholders,
# external schemes, absolute paths, fenced examples, inline code spans, and
# anchors.
#
# ADVERSARIAL CASES (non-tautological — each would fail if detection regressed):
#   A1  a broken and a valid link on the SAME line — proves per-link
#       resolution, not per-line short-circuiting. A regression that stops at
#       the first resolvable target on a line passes every other case here.
#   A2  two fixtures identical except for whether the target file exists —
#       proves the verdict comes from the filesystem, not from the prose.
#   A3  a link AFTER a closed fenced block — proves the fence toggle closes.
#       A regression that treats the opening fence as "skip to end of file"
#       would silently pass every fixture that puts its example first.
#
# No network, no dependency on the live tree.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/reference-existence-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "reference-existence-lint-selftest: lint not found: $LINT" >&2
  exit 2
fi

pass=0
fail=0
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

assert_exit() {
  local expected="$1" label="$2"
  shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS: $label (exit $actual)"
    pass=$((pass + 1))
  else
    echo "FAIL: $label (expected exit $expected, got $actual)"
    fail=$((fail + 1))
  fi
}

assert_reports() {
  local needle="$1" label="$2"
  shift 2
  local output=""
  output="$("$@" 2>&1)" || true
  if printf '%s' "$output" | grep -Fq -- "$needle"; then
    echo "PASS: $label"
    pass=$((pass + 1))
  else
    echo "FAIL: $label (output did not mention '$needle')"
    echo "      output: $output"
    fail=$((fail + 1))
  fi
}

assert_not_reports() {
  local needle="$1" label="$2"
  shift 2
  local output=""
  output="$("$@" 2>&1)" || true
  if printf '%s' "$output" | grep -Fq -- "$needle"; then
    echo "FAIL: $label (output unexpectedly mentioned '$needle')"
    echo "      output: $output"
    fail=$((fail + 1))
  else
    echo "PASS: $label"
    pass=$((pass + 1))
  fi
}

# new_case <name> [block] -> prints the case dir
new_case() {
  local name="$1" blocking="${2:-advisory}"
  local d="$TMP_ROOT/$name"
  mkdir -p "$d/.github"
  if [[ "$blocking" == "block" ]]; then
    printf 'referenceExistenceGuard: block\n' > "$d/.github/bubbles-project.yaml"
  else
    printf 'someOtherKey: value\n' > "$d/.github/bubbles-project.yaml"
  fi
  printf '%s' "$d"
}

# ── Case 1: a resolvable relative link passes (block mode) ────────────────
c1="$(new_case c1 block)"
printf '# t\n\nSee [real](ok.md).\n' > "$c1/a.md"
printf '# ok\n' > "$c1/ok.md"
assert_exit 0 "Case 1: resolvable link passes in block mode" bash "$LINT" "$c1"

# ── Case 2: a dangling link FAILS in block mode ───────────────────────────
c2="$(new_case c2 block)"
printf '# t\n\nSee [gone](does-not-exist.md).\n' > "$c2/a.md"
assert_exit 1 "Case 2: dangling link fails in block mode" bash "$LINT" "$c2"
assert_reports "does-not-exist.md" "Case 2b: finding names the dangling target" bash "$LINT" "$c2"

# ── Case 3: the same dangling link is ADVISORY by default ─────────────────
c3="$(new_case c3 advisory)"
printf '# t\n\nSee [gone](does-not-exist.md).\n' > "$c3/a.md"
assert_exit 0 "Case 3: dangling link is advisory without opt-in" bash "$LINT" "$c3"

# ── Case 4: placeholder / template targets are not claims ─────────────────
c4="$(new_case c4 block)"
printf '# t\n\nSee [spec](specs/<NNN-feature-name>/spec.md) and [x](${VAR}/y.md).\n' > "$c4/a.md"
assert_exit 0 "Case 4: placeholder targets skipped in block mode" bash "$LINT" "$c4"

# ── Case 5: external schemes and absolute paths are skipped ───────────────
c5="$(new_case c5 block)"
printf '# t\n\n[w](https://example.invalid/x.md) [m](mailto:a@b.c) [abs](/etc/nope.md) [anch](#section)\n' > "$c5/a.md"
assert_exit 0 "Case 5: schemes, absolute paths, bare anchors skipped" bash "$LINT" "$c5"

# ── Case 6: links inside a fenced code block are examples, not claims ─────
c6="$(new_case c6 block)"
printf '# t\n\n```\nSee [gone](nope.md).\n```\n' > "$c6/a.md"
assert_exit 0 "Case 6: fenced example link skipped" bash "$LINT" "$c6"

# ── Case 7: inline code spans are sample text ─────────────────────────────
c7="$(new_case c7 block)"
printf '# t\n\nWrite it as `[label](target.md)` in your doc.\n' > "$c7/a.md"
assert_exit 0 "Case 7: inline code span link skipped" bash "$LINT" "$c7"

# ── Case 8: ref-ok escape hatch ───────────────────────────────────────────
c8="$(new_case c8 block)"
printf '# t\n\nSee [gone](nope.md). ref-ok:documents a downstream install path\n' > "$c8/a.md"
assert_exit 0 "Case 8: ref-ok exemption honored" bash "$LINT" "$c8"

# ── Case 9: anchors are stripped before resolution ────────────────────────
c9="$(new_case c9 block)"
printf '# t\n\nSee [sec](ok.md#some-heading).\n' > "$c9/a.md"
printf '# ok\n' > "$c9/ok.md"
assert_exit 0 "Case 9: anchor stripped, base path resolves" bash "$LINT" "$c9"

# ── Case 10: a directory target resolves ──────────────────────────────────
c10="$(new_case c10 block)"
mkdir -p "$c10/sub"
printf '# t\n\nSee [dir](sub).\n' > "$c10/a.md"
assert_exit 0 "Case 10: directory target resolves" bash "$LINT" "$c10"

# ── Case 11: parent-relative traversal resolves ───────────────────────────
c11="$(new_case c11 block)"
mkdir -p "$c11/deep"
printf '# root\n' > "$c11/root.md"
printf '# t\n\nSee [up](../root.md).\n' > "$c11/deep/a.md"
assert_exit 0 "Case 11: ../ traversal resolves" bash "$LINT" "$c11"

# ── Case 12: percent-encoded space is decoded before resolution ───────────
c12="$(new_case c12 block)"
printf '# t\n\nSee [sp](My%%20File.md).\n' > "$c12/a.md"
printf '# f\n' > "$c12/My File.md"
assert_exit 0 "Case 12: %%20 decoded before resolution" bash "$LINT" "$c12"

# ── Case 13: usage errors ─────────────────────────────────────────────────
assert_exit 2 "Case 13: no scan surface is a usage error" bash "$LINT"
assert_exit 2 "Case 13b: missing scan path is a usage error" bash "$LINT" "$TMP_ROOT/definitely-absent"
assert_exit 2 "Case 13c: unknown flag is a usage error" bash "$LINT" --force "$TMP_ROOT"

# ── Case 14: a single file may be scanned directly ────────────────────────
c14="$(new_case c14 block)"
printf '# t\n\nSee [gone](nope.md).\n' > "$c14/a.md"
assert_exit 1 "Case 14: single-file surface is scannable" bash "$LINT" "$c14/a.md"

# ── ADVERSARIAL A1: broken + valid link on the SAME line ──────────────────
# A regression that stops after the first resolvable target on a line would
# pass every case above and silently miss this.
a1="$(new_case a1 block)"
printf '# t\n\nSee [ok](ok.md) and also [gone](missing-sibling.md) here.\n' > "$a1/a.md"
printf '# ok\n' > "$a1/ok.md"
assert_exit 1 "A1: broken link beside a valid one on one line is caught" bash "$LINT" "$a1"
assert_reports "missing-sibling.md" "A1b: the broken target is the one reported" bash "$LINT" "$a1"
assert_not_reports "broken relative link target: ok.md" "A1c: the valid target is not reported" bash "$LINT" "$a1"

# ── ADVERSARIAL A2: identical prose, verdict differs only by filesystem ───
# Proves the verdict is read from disk, not pattern-matched from the text.
a2y="$(new_case a2-present block)"
printf '# t\n\nSee [same](twin.md).\n' > "$a2y/a.md"
printf '# twin\n' > "$a2y/twin.md"
assert_exit 0 "A2: identical prose PASSES when the target exists" bash "$LINT" "$a2y"

a2n="$(new_case a2-absent block)"
printf '# t\n\nSee [same](twin.md).\n' > "$a2n/a.md"
assert_exit 1 "A2b: identical prose FAILS when the target is absent" bash "$LINT" "$a2n"

# ── ADVERSARIAL A3: a real link AFTER a closed fenced block ───────────────
# A regression that treats the opening fence as "skip the rest of the file"
# would pass Case 6 and silently stop checking every doc that opens with an
# example.
a3="$(new_case a3 block)"
printf '# t\n\n```\n[example](whatever.md)\n```\n\nNow a real one: [gone](after-fence.md).\n' > "$a3/a.md"
assert_exit 1 "A3: link after a closed fence is still checked" bash "$LINT" "$a3"
assert_reports "after-fence.md" "A3b: the post-fence target is reported" bash "$LINT" "$a3"

# ── Case 15: a surface with no markdown files is clean, not an error ──────
c15="$(new_case c15 block)"
printf 'not markdown\n' > "$c15/a.txt"
assert_exit 0 "Case 15: surface without markdown is clean" bash "$LINT" "$c15"

echo
echo "reference-existence-lint selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
