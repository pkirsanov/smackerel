#!/usr/bin/env bash
# receipt-identity-selftest.sh — BUG-033 regression surface for Check 43.
#
# WHY THIS EXISTS SEPARATELY FROM state-transition-guard-selftest.sh
# The guard selftest drives the WHOLE guard, which is the right end-to-end
# proof and the wrong feedback loop: one BUG-033 assertion costs a full
# multi-hundred-case run. Check 43's decision is a single self-contained jq
# program, so this sibling extracts THAT PROGRAM FROM THE GUARD SOURCE and
# drives it against receipt fixtures directly.
#
# Extraction, not re-implementation, is the load-bearing choice. A second copy
# of the identity rules would pass while the guard regressed — which is the
# exact class of defect BUG-033 is. The extraction therefore fails loudly if the
# guard's shape changes, rather than silently testing nothing.
#
# The end-to-end cases stay in state-transition-guard-selftest.sh. This file is
# the microscope, that one is the field trial.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the guard program could not be extracted, or jq is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
NAME="receipt-identity-selftest"

passes=0
failures=0
pass() {
  passes=$((passes + 1))
  printf 'PASS: %s\n' "$1"
}
fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

command -v jq >/dev/null 2>&1 || {
  printf '%s: jq is required\n' "$NAME" >&2
  exit 2
}
[[ -f "$GUARD_SCRIPT" ]] || {
  printf '%s: guard not found: %s\n' "$NAME" "$GUARD_SCRIPT" >&2
  exit 2
}

EMPTY_SHA="$(grep -oE 'c43_empty_stdout_sha256="[0-9a-f]{64}"' "$GUARD_SCRIPT" | grep -oE '[0-9a-f]{64}' | head -1 || true)"
if [[ "$EMPTY_SHA" != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]]; then
  printf '%s: empty-stdout constant not extractable from the guard (got %s)\n' "$NAME" "${EMPTY_SHA:-<none>}" >&2
  exit 2
fi

# The full Check 43 program: everything between the jq invocation line and its
# terminating quote. This is what the guard actually runs.
PROGRAM="$(awk '
  /c43_analysis="\$\(jq -rs/ { grab = 1; next }
  grab && /^[[:space:]]*'"'"' "\$c43_log"/ { exit }
  grab { print }
' "$GUARD_SCRIPT")"
if [[ -z "$PROGRAM" ]] || ! printf '%s' "$PROGRAM" | grep -qF 'deterministic_siblings'; then
  printf '%s: could not extract the Check 43 jq program from the guard (shape changed)\n' "$NAME" >&2
  exit 2
fi

# The definitions alone, so an assertion can probe one identity function without
# routing through the whole clone/sibling classification.
DEFS="$(printf '%s\n' "$PROGRAM" | awk '
  /^[[:space:]]*map\(select\(\(\.stdoutHash/ { exit }
  { print }
')"
if ! printf '%s' "$DEFS" | grep -qF 'def command_family:'; then
  printf '%s: could not extract the Check 43 identity definitions from the guard\n' "$NAME" >&2
  exit 2
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-receipt-identity.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

NONEMPTY="9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43"

write_log() {
  local path="$1"
  shift
  local line
  : > "$path"
  for line in "$@"; do
    printf '%s\n' "$line" >> "$path"
  done
}

analyze() {
  jq -rs --arg empty_sha "$EMPTY_SHA" "$PROGRAM" "$1" 2>&1
}
clone_count() {
  printf '%s' "$1" | jq -r '.clones | length' 2>/dev/null || printf 'ERR'
}
sibling_count() {
  printf '%s' "$1" | jq -r '.siblings | length' 2>/dev/null || printf 'ERR'
}

# ---------------------------------------------------------------------------
# BUG-033 facet 1 — target distinctness measured PER RECEIPT.
#
# A validator is routinely re-run over one subject, so an honest log repeats
# that subject. Measuring `target_identity` once per RECEIPT makes 9 honest runs
# over 2 specs produce 9 values with 2 distinct entries, and the distinctness
# test then fails ON SHAPE ALONE — before any question of forgery is asked.
# ---------------------------------------------------------------------------
rerun_log="$TMP_DIR/facet1-rerun.jsonl"
write_log "$rerun_log" \
  "{\"ts\":\"2026-08-16T09:00:01Z\",\"sessionId\":\"rr-a1\",\"spec\":\"specs/alpha\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":101,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:02Z\",\"sessionId\":\"rr-a2\",\"spec\":\"specs/alpha\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":102,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:03Z\",\"sessionId\":\"rr-a3\",\"spec\":\"specs/alpha\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":103,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:04Z\",\"sessionId\":\"rr-a4\",\"spec\":\"specs/alpha\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":104,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:05Z\",\"sessionId\":\"rr-a5\",\"spec\":\"specs/alpha\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":105,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:06Z\",\"sessionId\":\"rr-b1\",\"spec\":\"specs/beta\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/beta\",\"exitCode\":0,\"durationMs\":106,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:07Z\",\"sessionId\":\"rr-b2\",\"spec\":\"specs/beta\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/beta\",\"exitCode\":0,\"durationMs\":107,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:08Z\",\"sessionId\":\"rr-b3\",\"spec\":\"specs/beta\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/beta\",\"exitCode\":0,\"durationMs\":108,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:00:09Z\",\"sessionId\":\"rr-b4\",\"spec\":\"specs/beta\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/beta\",\"exitCode\":0,\"durationMs\":109,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"

rerun_out="$(analyze "$rerun_log")"
if [[ "$(clone_count "$rerun_out")" == "0" ]]; then
  pass "facet 1: 9 honest re-runs of one validator over 2 targets are not reported as cloned evidence"
else
  fail "facet 1: honest re-runs reported as clones ($(clone_count "$rerun_out") group(s)) — target distinctness is measured per receipt"
  printf '  analysis: %s\n' "$rerun_out"
fi
if [[ "$(sibling_count "$rerun_out")" == "1" ]]; then
  pass "facet 1: the re-run group is accepted through the deterministic-sibling path, not by an empty analysis"
else
  fail "facet 1: expected exactly 1 accepted sibling group, observed $(sibling_count "$rerun_out")"
fi

# ADVERSARIAL BOUND for facet 1. Two DIFFERENT command identities over ONE
# target sharing one substantive stdout. Grouping targets by identity must not
# turn this into a pass: one identity's target cannot vouch for the other's.
onetarget_log="$TMP_DIR/facet1-onetarget.jsonl"
write_log "$onetarget_log" \
  "{\"ts\":\"2026-08-16T09:10:01Z\",\"sessionId\":\"ot-a\",\"spec\":\"specs/alpha\",\"cmd\":\"npm run lint\",\"exitCode\":0,\"durationMs\":201,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:10:03Z\",\"sessionId\":\"ot-b\",\"spec\":\"specs/alpha\",\"cmd\":\"npm run test\",\"exitCode\":0,\"durationMs\":203,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}"

onetarget_out="$(analyze "$onetarget_log")"
if [[ "$(clone_count "$onetarget_out")" == "1" ]]; then
  pass "facet 1 bound: two identities sharing ONE target and one stdout are still refused"
else
  fail "facet 1 bound: expected 1 clone group for two identities over one target, observed $(clone_count "$onetarget_out")"
  printf '  analysis: %s\n' "$onetarget_out"
fi

# ---------------------------------------------------------------------------
# BUG-033 facet 2 — `cmd_parts` unwraps only a bare leading `bash`/`sh`.
#
# One command spelled three ordinary ways resolves to three different families,
# so the group becomes a multi-identity collision that never should have been
# one. `bash -c <script>` is worse: it strips `bash` and leaves `-c` as the
# family, so the family is a flag.
# ---------------------------------------------------------------------------
family_of() {
  printf '%s' "$1" | jq -Rr "$DEFS"' command_family' 2>&1
}

for probe in \
  "node scripts/check-page.mjs alpha" \
  "env PAGE=alpha node scripts/check-page.mjs alpha" \
  "zsh -c node scripts/check-page.mjs alpha" \
  "PAGE=alpha node scripts/check-page.mjs alpha" \
  "bash -c node scripts/check-page.mjs alpha" \
  "sh -c node scripts/check-page.mjs alpha"; do
  observed="$(family_of "$probe")"
  if [[ "$observed" == "node" ]]; then
    pass "facet 2: '$probe' normalizes to command_family=node"
  else
    fail "facet 2: '$probe' normalizes to command_family='$observed' (expected node)"
  fi
done

wrapper_log="$TMP_DIR/facet2-wrappers.jsonl"
write_log "$wrapper_log" \
  "{\"ts\":\"2026-08-16T09:20:01Z\",\"sessionId\":\"wr-a\",\"spec\":\"specs/alpha\",\"cmd\":\"node scripts/check-page.mjs alpha\",\"exitCode\":0,\"durationMs\":301,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"validate\"]}" \
  "{\"ts\":\"2026-08-16T09:20:02Z\",\"sessionId\":\"wr-b\",\"spec\":\"specs/alpha\",\"cmd\":\"env PAGE=alpha node scripts/check-page.mjs alpha\",\"exitCode\":0,\"durationMs\":302,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"validate\"]}" \
  "{\"ts\":\"2026-08-16T09:20:03Z\",\"sessionId\":\"wr-c\",\"spec\":\"specs/alpha\",\"cmd\":\"zsh -c node scripts/check-page.mjs alpha\",\"exitCode\":0,\"durationMs\":303,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"validate\"]}" \
  "{\"ts\":\"2026-08-16T09:20:04Z\",\"sessionId\":\"wr-d\",\"spec\":\"specs/alpha\",\"cmd\":\"PAGE=alpha node scripts/check-page.mjs alpha\",\"exitCode\":0,\"durationMs\":304,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"validate\"]}" \
  "{\"ts\":\"2026-08-16T09:20:05Z\",\"sessionId\":\"wr-e\",\"spec\":\"specs/alpha\",\"cmd\":\"bash -c node scripts/check-page.mjs alpha\",\"exitCode\":0,\"durationMs\":305,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"validate\"]}"

wrapper_out="$(analyze "$wrapper_log")"
if [[ "$(clone_count "$wrapper_out")" == "0" ]]; then
  pass "facet 2: five wrapper spellings of one command over one target are not reported as cloned evidence"
else
  fail "facet 2: wrapper spellings reported as clones ($(clone_count "$wrapper_out") group(s))"
  printf '  analysis: %s\n' "$wrapper_out"
fi

# ADVERSARIAL BOUND for facet 2. The SAME wrappers over two genuinely different
# programs. Unwrapping must REVEAL the difference, not hide it.
wrapper_adv_log="$TMP_DIR/facet2-adversarial.jsonl"
write_log "$wrapper_adv_log" \
  "{\"ts\":\"2026-08-16T09:30:01Z\",\"sessionId\":\"wa-a\",\"spec\":\"specs/alpha\",\"cmd\":\"zsh -c cargo test\",\"exitCode\":0,\"durationMs\":401,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T09:30:03Z\",\"sessionId\":\"wa-b\",\"spec\":\"specs/beta\",\"cmd\":\"env CI=1 npm run lint\",\"exitCode\":0,\"durationMs\":403,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"

wrapper_adv_out="$(analyze "$wrapper_adv_log")"
if [[ "$(clone_count "$wrapper_adv_out")" == "1" ]]; then
  pass "facet 2 bound: two different programs behind identical wrappers are still refused"
else
  fail "facet 2 bound: expected 1 clone group for cargo-vs-npm behind wrappers, observed $(clone_count "$wrapper_adv_out")"
  printf '  analysis: %s\n' "$wrapper_adv_out"
fi
if printf '%s' "$wrapper_adv_out" | grep -qF 'family=cargo' &&
  printf '%s' "$wrapper_adv_out" | grep -qF 'family=npm'; then
  pass "facet 2 bound: the diagnostic names the unwrapped cargo and npm identities"
else
  fail "facet 2 bound: the diagnostic did not name both unwrapped identities"
  printf '  analysis: %s\n' "$wrapper_adv_out"
fi

# ---------------------------------------------------------------------------
# BUG-007 / BUG-032 pins. The BUG-033 relaxation must not disturb the two
# properties earlier defects were fixed to establish.
# ---------------------------------------------------------------------------
empty_log="$TMP_DIR/pin-empty.jsonl"
write_log "$empty_log" \
  "{\"ts\":\"2026-08-16T09:40:01Z\",\"sessionId\":\"pe-a\",\"cmd\":\"grep -rn TODO src/\",\"exitCode\":1,\"stdoutHash\":\"$EMPTY_SHA\",\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T09:40:03Z\",\"sessionId\":\"pe-b\",\"cmd\":\"node scripts/validate.mjs\",\"exitCode\":0,\"stdoutHash\":\"$EMPTY_SHA\",\"tags\":[\"validate\"]}"
empty_out="$(analyze "$empty_log")"
if [[ "$(clone_count "$empty_out")" == "0" ]]; then
  pass "BUG-007 pin: empty stdout stays exempt after the BUG-033 relaxation"
else
  fail "BUG-007 pin: empty stdout reported as a clone ($(clone_count "$empty_out") group(s))"
fi

provenance_log="$TMP_DIR/pin-provenance.jsonl"
write_log "$provenance_log" \
  "{\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/alpha\",\"exitCode\":0,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"cmd\":\"bash bubbles/scripts/artifact-lint.sh specs/beta\",\"exitCode\":0,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"
provenance_out="$(analyze "$provenance_log")"
if [[ "$(clone_count "$provenance_out")" == "1" ]]; then
  pass "BUG-032 pin: a collision with no independent execution provenance is still refused"
else
  fail "BUG-032 pin: provenance-poor collision no longer refused (observed $(clone_count "$provenance_out") clone group(s))"
fi

incompatible_log="$TMP_DIR/pin-incompatible.jsonl"
write_log "$incompatible_log" \
  "{\"ts\":\"2026-08-16T09:50:01Z\",\"sessionId\":\"pi-a\",\"spec\":\"specs/alpha\",\"cmd\":\"cargo test\",\"exitCode\":0,\"durationMs\":501,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T09:50:03Z\",\"sessionId\":\"pi-b\",\"spec\":\"specs/beta\",\"cmd\":\"npm run lint\",\"exitCode\":0,\"durationMs\":503,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"
incompatible_out="$(analyze "$incompatible_log")"
if [[ "$(clone_count "$incompatible_out")" == "1" ]]; then
  pass "BUG-032 pin: incompatible command families sharing one stdout are still refused"
else
  fail "BUG-032 pin: incompatible families no longer refused (observed $(clone_count "$incompatible_out") clone group(s))"
fi

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
