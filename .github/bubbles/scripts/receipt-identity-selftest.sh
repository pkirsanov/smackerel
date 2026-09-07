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
BRIDGE="$SCRIPT_DIR/evidence-tool-log-bridge.sh"
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
[[ -f "$BRIDGE" ]] || {
  printf '%s: bridge not found: %s\n' "$NAME" "$BRIDGE" >&2
  exit 2
}

sha256_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$1" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$1" | shasum -a 256 | awk '{print $1}'
  else
    return 1
  fi
}

EMPTY_SHA="$(grep -oE 'c43_empty_stdout_sha256="[0-9a-f]{64}"' "$GUARD_SCRIPT" | grep -oE '[0-9a-f]{64}' | head -1 || true)"
EXPECTED_EMPTY_SHA="$(sha256_text '')" || {
  printf '%s: sha256sum or shasum is required\n' "$NAME" >&2
  exit 2
}
if [[ "$EMPTY_SHA" != "$EXPECTED_EMPTY_SHA" ]]; then
  printf '%s: empty-stdout constant not extractable from the guard (got %s)\n' "$NAME" "${EMPTY_SHA:-<none>}" >&2
  exit 2
fi

# The full Check 43 program: everything between the jq invocation line and its
# terminating quote. This is what the guard actually runs.
PROGRAM="$(awk '
  /c43_analysis="\$\(jq -rs/ { grab = 1; next }
  grab && /^[[:space:]]*'"'"' "\$c43_admitted_log"/ { exit }
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

NONEMPTY="$(sha256_text "$NAME-nonempty-fixture")"

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
# BUG-050 SCN-B050-003/004 — clone identity consumes the same transition-local
# admitted projection as freshness. The bridge executes real semantic
# admission; this selftest continues to execute the real extracted Check 43 jq.
# ---------------------------------------------------------------------------
admission_dir="$TMP_DIR/050-admission"
mkdir -p "$admission_dir"
cat > "$admission_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "spec": "050-admission",
  "scenarios": [
    {"scenarioId":"SCN-B050-003","title":"Unrelated clone groups are inert","requiredTestType":"functional"},
    {"scenarioId":"SCN-B050-004","title":"Admitted incompatible clone blocks","requiredTestType":"functional"}
  ]
}
EOF
admission_rev="$(printf '%040d' 1)"
active_hash="$(sha256_text 'bug050-active-output')"
unrelated_hash="$(sha256_text 'bug050-unrelated-clone-output')"

admit_log() {
  bash "$BRIDGE" "$admission_dir" --log "$1" --format=admitted-jsonl 2>&1
}

unrelated_clone_log="$TMP_DIR/bug050-unrelated-clone.jsonl"
write_log "$unrelated_clone_log" \
  "{\"schemaVersion\":3,\"ts\":\"2026-09-02T08:10:00Z\",\"sessionId\":\"b050-active\",\"spec\":\"050-admission\",\"scope\":\"SCOPE-01\",\"cmd\":\"bash active-check.sh\",\"exitCode\":0,\"durationMs\":10,\"stdoutHash\":\"$active_hash\",\"stdoutBytes\":64,\"tags\":[\"test\"],\"scenarioBinding\":{\"scenarioId\":\"SCN-B050-003\",\"phase\":\"green\",\"testIdentity\":\"BUG-050::active-clean\",\"sourceRevision\":\"$admission_rev\",\"negativeControl\":\"restore repository-global grouping\",\"claim\":\"unrelated clone groups are inert\"}}" \
  "{\"schemaVersion\":3,\"ts\":\"2026-09-02T08:10:01Z\",\"sessionId\":\"b050-unrelated-a\",\"spec\":\"999-unrelated\",\"scope\":\"SCOPE-X\",\"cmd\":\"cargo test\",\"exitCode\":0,\"durationMs\":11,\"stdoutHash\":\"$unrelated_hash\",\"stdoutBytes\":64,\"tags\":[\"test\"]}" \
  "{\"schemaVersion\":3,\"ts\":\"2026-09-02T08:10:02Z\",\"sessionId\":\"b050-unrelated-b\",\"spec\":\"999-unrelated\",\"scope\":\"SCOPE-X\",\"cmd\":\"npm run lint\",\"exitCode\":0,\"durationMs\":12,\"stdoutHash\":\"$unrelated_hash\",\"stdoutBytes\":64,\"tags\":[\"lint\"]}"
unrelated_admitted_log="$TMP_DIR/bug050-unrelated-admitted.jsonl"
unrelated_admitted_out="$(admit_log "$unrelated_clone_log")"
unrelated_admitted_rc=$?
printf '%s\n' "$unrelated_admitted_out" > "$unrelated_admitted_log"
unrelated_admitted_count="$(awk 'NF { count++ } END { print count + 0 }' "$unrelated_admitted_log")"
unrelated_out="$(analyze "$unrelated_admitted_log")"
if [[ "$unrelated_admitted_rc" -eq 0 && "$unrelated_admitted_count" -eq 1 && "$(clone_count "$unrelated_out")" == "0" ]]; then
  pass "SCN-B050-003 unrelated incompatible clone history is excluded from the admitted projection"
else
  fail "SCN-B050-003 expected one active row and zero admitted clones (bridge=$unrelated_admitted_rc rows=$unrelated_admitted_count clones=$(clone_count "$unrelated_out"))"
fi

active_clone_log="$TMP_DIR/bug050-active-clone.jsonl"
write_log "$active_clone_log" \
  "{\"schemaVersion\":3,\"ts\":\"2026-09-02T08:20:00Z\",\"sessionId\":\"b050-clone-a\",\"spec\":\"050-admission\",\"scope\":\"SCOPE-01\",\"cmd\":\"cargo test\",\"exitCode\":0,\"durationMs\":20,\"stdoutHash\":\"$active_hash\",\"stdoutBytes\":64,\"tags\":[\"test\"],\"scenarioBinding\":{\"scenarioId\":\"SCN-B050-004\",\"phase\":\"green\",\"testIdentity\":\"BUG-050::clone-a\",\"sourceRevision\":\"$admission_rev\",\"negativeControl\":\"accept incompatible active programs\",\"claim\":\"admitted incompatible clone blocks\"}}" \
  "{\"schemaVersion\":3,\"ts\":\"2026-09-02T08:20:01Z\",\"sessionId\":\"b050-clone-b\",\"spec\":\"050-admission\",\"scope\":\"SCOPE-01\",\"cmd\":\"npm run lint\",\"exitCode\":0,\"durationMs\":21,\"stdoutHash\":\"$active_hash\",\"stdoutBytes\":64,\"tags\":[\"lint\"],\"scenarioBinding\":{\"scenarioId\":\"SCN-B050-004\",\"phase\":\"green\",\"testIdentity\":\"BUG-050::clone-b\",\"sourceRevision\":\"$admission_rev\",\"negativeControl\":\"accept incompatible active programs\",\"claim\":\"admitted incompatible clone blocks\"}}"
active_admitted_log="$TMP_DIR/bug050-active-admitted.jsonl"
active_admitted_out="$(admit_log "$active_clone_log")"
active_admitted_rc=$?
printf '%s\n' "$active_admitted_out" > "$active_admitted_log"
active_admitted_count="$(awk 'NF { count++ } END { print count + 0 }' "$active_admitted_log")"
active_out="$(analyze "$active_admitted_log")"
if [[ "$active_admitted_rc" -eq 0 && "$active_admitted_count" -eq 2 && "$(clone_count "$active_out")" == "1" ]]; then
  pass "SCN-B050-004 admitted incompatible clone remains refused by the BUG-033 identity program"
else
  fail "SCN-B050-004 expected two admitted rows and one clone (bridge=$active_admitted_rc rows=$active_admitted_count clones=$(clone_count "$active_out"))"
fi
if printf '%s' "$active_out" | grep -qF 'family=cargo category=test' &&
  printf '%s' "$active_out" | grep -qF 'family=npm category=lint'; then
  pass "SCN-B050-004 clone diagnostic preserves BUG-033 program and category identity detail"
else
  fail "SCN-B050-004 clone diagnostic lost BUG-033 identity detail"
  printf '  analysis: %s\n' "$active_out"
fi

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
program_of() {
  printf '%s' "$1" | jq -Rr "$DEFS"' program_identity' 2>&1
}
identity_of() {
  printf '%s' "$1" | jq -Rr "$DEFS"' cmd_identity' 2>&1
}

identity_of() {
  printf '%s' "$1" | jq -Rr "$DEFS"' cmd_identity' 2>&1
}

parts_of() {
  printf '%s' "$1" | jq -Rr "$DEFS"' cmd_parts | join(" ")' 2>&1
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
# BUG-033 facet 3 — bounded launchers expose the evidence-producing command.
# ---------------------------------------------------------------------------
facet3_direct="artifact-lint.sh bugs/BUG-033-receipt-target-grouping-and-wrapper-normalization"
facet3_direct_identity="$(identity_of "$facet3_direct")"

for launcher in timeout gtimeout; do
  observed="$(identity_of "$launcher 120 $facet3_direct")"
  if [[ "$observed" == "$facet3_direct_identity" ]]; then
    pass "SCN-B033-005: $launcher exposes the direct artifact-lint identity"
  else
    fail "SCN-B033-005: $launcher identity '$observed' differs from direct identity '$facet3_direct_identity'"
  fi
done

facet3_alarm="/usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120 $facet3_direct"
facet3_alarm_identity="$(identity_of "$facet3_alarm")"
if [[ "$facet3_alarm_identity" == "$facet3_direct_identity" ]]; then
  pass "SCN-B033-006: the exact portable Perl alarm launcher exposes the direct artifact-lint identity"
else
  fail "SCN-B033-006: portable alarm identity '$facet3_alarm_identity' differs from direct identity '$facet3_direct_identity'"
fi

for probe in \
  "timeout 120 env PAGE=alpha zsh -c node scripts/check-page.mjs alpha" \
  "env PAGE=alpha gtimeout 120 bash -c node scripts/check-page.mjs alpha" \
  "zsh -c /usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120 env PAGE=alpha node scripts/check-page.mjs alpha" \
  "PAGE=alpha timeout 120 sh -c node scripts/check-page.mjs alpha"; do
  observed_family="$(family_of "$probe")"
  observed_identity="$(identity_of "$probe")"
  if [[ "$observed_family" == "node" ]] && [[ "$observed_identity" == "node scripts/check-page.mjs" ]]; then
    pass "SCN-B033-007: composed spelling '$probe' exposes node scripts/check-page.mjs"
  else
    fail "SCN-B033-007: composed spelling '$probe' produced family='$observed_family' identity='$observed_identity'"
  fi
done

facet3_arbitrary_perl="/usr/bin/perl -e 'print 1' 120 artifact-lint.sh TARGET"
if [[ "$(parts_of "$facet3_arbitrary_perl")" == "$facet3_arbitrary_perl" ]] &&
  [[ "$(identity_of "$facet3_arbitrary_perl")" != "$(identity_of "artifact-lint.sh TARGET")" ]]; then
  pass "SCN-B033-008: arbitrary Perl remains unchanged and distinct from the direct command"
else
  fail "SCN-B033-008: arbitrary Perl was stripped or collapsed into the direct command"
fi

# "timeout --preserve-status 120 artifact-lint.sh TARGET" lived in this list
# until the Check 43 merge below taught the guard timeout's own closed
# option grammar: --preserve-status is a real, safe GNU option, and it now
# correctly strips to the direct identity. That positive case moved to the
# option-grammar coverage under "Check 43 timeout transparency" below;
# "--bogus-option" replaces it here as the still-genuinely-malformed case
# this loop exists to cover.
for malformed in \
  "timeout" \
  "timeout 120" \
  "gtimeout 120" \
  "timeout --bogus-option 120 artifact-lint.sh TARGET" \
  "/usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120" \
  "/usr/bin/perl -e 'alarm shift @ARGV; print @ARGV' 120 artifact-lint.sh TARGET"; do
  observed="$(parts_of "$malformed")"
  if [[ "$observed" == "$malformed" ]]; then
    pass "SCN-B033-009: malformed spelling '$malformed' remains unchanged"
  else
    fail "SCN-B033-009: malformed spelling '$malformed' normalized to '$observed'"
  fi
done

for launcher_kind in timeout gtimeout portable-perl-alarm; do
  case "$launcher_kind" in
    timeout)
      launcher_prefix="timeout 120"
      ;;
    gtimeout)
      launcher_prefix="gtimeout 120"
      ;;
    portable-perl-alarm)
      launcher_prefix="/usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120"
      ;;
  esac
  identity_a="$(identity_of "$launcher_prefix artifact-lint.sh TARGET")"
  identity_b="$(identity_of "$launcher_prefix state-transition-guard.sh TARGET")"
  if [[ "$identity_a" == "artifact-lint.sh TARGET" ]] &&
    [[ "$identity_b" == "state-transition-guard.sh TARGET" ]]; then
    pass "SCN-B033-010: $launcher_kind preserves both distinct underlying command identities"
  else
    fail "SCN-B033-010: $launcher_kind produced identity_a='$identity_a' identity_b='$identity_b'"
  fi
done

facet3_exit_log="$TMP_DIR/facet3-exit-mismatch.jsonl"
write_log "$facet3_exit_log" \
  "{\"ts\":\"2026-08-23T12:00:01Z\",\"sessionId\":\"exit-a\",\"spec\":\"specs/alpha\",\"cmd\":\"timeout 120 artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":901,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-23T12:00:03Z\",\"sessionId\":\"exit-b\",\"spec\":\"specs/beta\",\"cmd\":\"gtimeout 120 artifact-lint.sh specs/beta\",\"exitCode\":1,\"durationMs\":903,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"
facet3_exit_out="$(analyze "$facet3_exit_log")"
if [[ "$(identity_of "timeout 120 artifact-lint.sh specs/alpha")" == "artifact-lint.sh specs/alpha" ]] &&
  [[ "$(identity_of "gtimeout 120 artifact-lint.sh specs/beta")" == "artifact-lint.sh specs/beta" ]] &&
  [[ "$(clone_count "$facet3_exit_out")" == "1" ]] &&
  [[ "$(sibling_count "$facet3_exit_out")" == "0" ]]; then
  pass "SCN-B033-011: normalized commands with different exits remain incompatible"
else
  fail "SCN-B033-011: launcher identity or independent exit incompatibility was not preserved"
  printf '  analysis: %s\n' "$facet3_exit_out"
fi

facet3_equal_exit_log="$TMP_DIR/facet3-equal-exit.jsonl"
write_log "$facet3_equal_exit_log" \
  "{\"ts\":\"2026-08-23T12:10:01Z\",\"sessionId\":\"equal-a\",\"spec\":\"specs/alpha\",\"cmd\":\"timeout 120 artifact-lint.sh specs/alpha\",\"exitCode\":0,\"durationMs\":911,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-23T12:10:03Z\",\"sessionId\":\"equal-b\",\"spec\":\"specs/beta\",\"cmd\":\"gtimeout 120 artifact-lint.sh specs/beta\",\"exitCode\":0,\"durationMs\":913,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"
facet3_equal_exit_out="$(analyze "$facet3_equal_exit_log")"
if [[ "$(clone_count "$facet3_equal_exit_out")" == "0" ]] &&
  [[ "$(sibling_count "$facet3_equal_exit_out")" == "1" ]]; then
  pass "SCN-B033-011 negative control: equal exits remove the exit-result incompatibility"
else
  fail "SCN-B033-011 negative control: equal exits did not produce deterministic siblings"
  printf '  analysis: %s\n' "$facet3_equal_exit_out"
fi

# ---------------------------------------------------------------------------
# Check 43 timeout transparency — GNU `timeout` and macOS coreutils
# `gtimeout` are execution bounds, not underlying programs. Only bare canonical
# wrapper tokens are transparent after their known options and mandatory
# duration are consumed. A receipt cannot authenticate the executable behind a
# path-qualified token, so system paths and attacker-controlled paths remain
# opaque alongside unknown options, malformed durations, and near-miss names.
# ---------------------------------------------------------------------------
timeout_program="scenario-test-resolve-selftest.sh"
timeout_identity="scenario-test-resolve-selftest.sh"
for probe in \
  "bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout -k 5 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --kill-after=5 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --kill-after 5 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout -s TERM 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --signal=TERM 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --signal TERM 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout -v 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --verbose 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --foreground 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --preserve-status 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --kill-after=inf 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout --signal TERM --kill-after=5 --foreground --verbose --preserve-status 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "gtimeout -k 5 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "gtimeout -- 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "timeout 540 /usr/bin/env bash bubbles/scripts/scenario-test-resolve-selftest.sh" \
  "env CHECK=1 bash -c gtimeout --verbose 150 sh bubbles/scripts/scenario-test-resolve-selftest.sh"; do
  observed_family="$(family_of "$probe")"
  observed_program="$(program_of "$probe")"
  observed_identity="$(identity_of "$probe")"
  if [[ "$observed_family" == "$timeout_program" &&
    "$observed_program" == "$timeout_program" &&
    "$observed_identity" == "$timeout_identity" ]]; then
    pass "timeout transparency: '$probe' has the bare script family/program/identity"
  else
    fail "timeout transparency: '$probe' resolved family='$observed_family' program='$observed_program' identity='$observed_identity'"
  fi
done

for probe in \
  "mytimeout 150 cargo test" \
  "timeout-wrapper 150 cargo test" \
  "/tmp/timeout 150 cargo test" \
  "/usr/bin/timeout 150 cargo test" \
  "/usr/local/bin/gtimeout 150 cargo test" \
  "timeout --unknown 150 cargo test" \
  "timeout -x 150 cargo test" \
  "timeout --help 150 cargo test" \
  "timeout --version 150 cargo test" \
  "timeout -f 150 cargo test" \
  "timeout -p 150 cargo test" \
  "timeout -vfp 150 cargo test" \
  "timeout -k.5 150 cargo test" \
  "timeout -sTERM 150 cargo test" \
  "timeout -s9 150 cargo test" \
  "timeout -sv 150 cargo test" \
  "timeout -k --verbose 150 cargo test" \
  "timeout -k invalid 150 cargo test" \
  "timeout --kill-after=invalid 150 cargo test" \
  "timeout -s --verbose 150 cargo test" \
  "timeout -s BOGUS 150 cargo test" \
  "timeout --signal= 150 cargo test" \
  "timeout --signal 150" \
  "timeout -k" \
  "timeout -s" \
  "timeout -v" \
  "timeout 1S cargo test" \
  "timeout not-a-duration cargo test" \
  "timeout 150"; do
  observed_family="$(family_of "$probe")"
  if [[ "$observed_family" != "cargo" ]]; then
    pass "timeout transparency bound: '$probe' remains opaque"
  else
    fail "timeout transparency bound: '$probe' was incorrectly unwrapped as cargo"
  fi
done

timeout_sibling_log="$TMP_DIR/timeout-siblings.jsonl"
write_log "$timeout_sibling_log" \
  "{\"ts\":\"2026-08-16T09:34:01Z\",\"sessionId\":\"tn-a\",\"cmd\":\"bash bubbles/scripts/scenario-test-resolve-selftest.sh alpha\",\"exitCode\":0,\"durationMs\":441,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"],\"inputClosure\":[{\"path\":\"alpha\",\"sha256\":\"aaa\"}]}" \
  "{\"ts\":\"2026-08-16T09:34:03Z\",\"sessionId\":\"tn-b\",\"cmd\":\"gtimeout --kill-after 5 150 /usr/bin/env CHECK=1 sh bubbles/scripts/scenario-test-resolve-selftest.sh beta\",\"exitCode\":0,\"durationMs\":443,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"],\"inputClosure\":[{\"path\":\"beta\",\"sha256\":\"bbb\"}]}"

timeout_sibling_out="$(analyze "$timeout_sibling_log")"
if [[ "$(clone_count "$timeout_sibling_out")" == "0" &&
  "$(sibling_count "$timeout_sibling_out")" == "1" ]]; then
  pass "timeout transparency: bare child and bare canonical gtimeout wrapper normalize to deterministic siblings"
else
  fail "timeout transparency: expected 0 clones and 1 sibling group for bare-versus-wrapped executions"
  printf '  analysis: %s\n' "$timeout_sibling_out"
fi

timeout_path_impersonation_log="$TMP_DIR/timeout-path-impersonation.jsonl"
write_log "$timeout_path_impersonation_log" \
  "{\"ts\":\"2026-08-16T09:34:11Z\",\"sessionId\":\"tp-a\",\"spec\":\"specs/alpha\",\"cmd\":\"bash bubbles/scripts/scenario-test-resolve-selftest.sh alpha\",\"exitCode\":0,\"durationMs\":445,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T09:34:13Z\",\"sessionId\":\"tp-b\",\"spec\":\"specs/beta\",\"cmd\":\"/tmp/timeout 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh beta\",\"exitCode\":0,\"durationMs\":447,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T09:34:15Z\",\"sessionId\":\"tp-c\",\"spec\":\"specs/gamma\",\"cmd\":\"/usr/bin/timeout 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh gamma\",\"exitCode\":0,\"durationMs\":449,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}"

timeout_path_impersonation_out="$(analyze "$timeout_path_impersonation_log")"
if [[ "$(clone_count "$timeout_path_impersonation_out")" == "1" ]]; then
  pass "timeout trust bound: path-qualified timeout tokens do not collapse to the nested child identity"
else
  fail "timeout trust bound: expected path-qualified timeout tokens to remain distinct from the child"
  printf '  analysis: %s\n' "$timeout_path_impersonation_out"
fi
if printf '%s' "$timeout_path_impersonation_out" | grep -qF 'family=timeout' &&
  printf '%s' "$timeout_path_impersonation_out" | grep -qF "family=$timeout_program"; then
  pass "timeout trust bound: diagnostics preserve wrapper and child families"
else
  fail "timeout trust bound: diagnostics did not preserve both wrapper and child families"
  printf '  analysis: %s\n' "$timeout_path_impersonation_out"
fi

timeout_adv_log="$TMP_DIR/timeout-adversarial.jsonl"
write_log "$timeout_adv_log" \
  "{\"ts\":\"2026-08-16T09:35:01Z\",\"sessionId\":\"tw-a\",\"spec\":\"specs/alpha\",\"cmd\":\"timeout 150 cargo test\",\"exitCode\":0,\"durationMs\":451,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T09:35:03Z\",\"sessionId\":\"tw-b\",\"spec\":\"specs/beta\",\"cmd\":\"gtimeout --preserve-status 150 npm run test\",\"exitCode\":0,\"durationMs\":453,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}"

timeout_adv_out="$(analyze "$timeout_adv_log")"
if [[ "$(clone_count "$timeout_adv_out")" == "1" ]]; then
  pass "timeout transparency bound: timeout-wrapped cargo and npm sharing stdout are still refused"
else
  fail "timeout transparency bound: expected 1 clone group for timeout-wrapped cargo-vs-npm, observed $(clone_count "$timeout_adv_out")"
  printf '  analysis: %s\n' "$timeout_adv_out"
fi

timeout_script_adv_log="$TMP_DIR/timeout-distinct-scripts.jsonl"
write_log "$timeout_script_adv_log" \
  "{\"ts\":\"2026-08-16T09:36:01Z\",\"sessionId\":\"ts-a\",\"spec\":\"specs/alpha\",\"cmd\":\"timeout 150 bash bubbles/scripts/alpha-selftest.sh\",\"exitCode\":0,\"durationMs\":461,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T09:36:03Z\",\"sessionId\":\"ts-b\",\"spec\":\"specs/beta\",\"cmd\":\"gtimeout -s TERM 150 sh bubbles/scripts/beta-selftest.sh\",\"exitCode\":0,\"durationMs\":463,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}"

timeout_script_adv_out="$(analyze "$timeout_script_adv_log")"
if [[ "$(clone_count "$timeout_script_adv_out")" == "1" ]]; then
  pass "timeout transparency bound: two distinct timeout-wrapped scripts sharing stdout are still refused"
else
  fail "timeout transparency bound: expected 1 clone group for distinct scripts, observed $(clone_count "$timeout_script_adv_out")"
  printf '  analysis: %s\n' "$timeout_script_adv_out"
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

# ---------------------------------------------------------------------------
# BUG-028 defect 1 — a differing CATEGORY is not a forgery trigger.
#
# `evidence_category` comes from operator-supplied tags, which describe a run
# rather than identify it. These two receipts are the SAME command: identical
# cmd, family, and identity. They were executed two days apart in two sessions
# and tagged differently, and the old disjunction alleged forgery for that
# alone.
# ---------------------------------------------------------------------------
category_log="$TMP_DIR/bug028-category.jsonl"
write_log "$category_log" \
  "{\"ts\":\"2026-07-13T22:40:55Z\",\"sessionId\":\"b28-test\",\"spec\":\"specs/alpha\",\"scope\":\"01-fail-closed\",\"cmd\":\"bash bubbles/scripts/macos-portability-guard-selftest.sh\",\"exitCode\":0,\"durationMs\":1213,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-07-15T22:05:13Z\",\"sessionId\":\"b28-validate\",\"spec\":\"specs/alpha\",\"scope\":\"Scope-1\",\"cmd\":\"bash bubbles/scripts/macos-portability-guard-selftest.sh\",\"exitCode\":0,\"durationMs\":991,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"validate\"]}"

category_out="$(analyze "$category_log")"
if [[ "$(clone_count "$category_out")" == "0" ]]; then
  pass "BUG-028 defect 1: one command tagged test and validate is not reported as cloned evidence"
else
  fail "BUG-028 defect 1: a tag difference alone still alleges forgery ($(clone_count "$category_out") group(s))"
  printf '  analysis: %s\n' "$category_out"
fi

# ADVERSARIAL BOUND for defect 1. Two genuinely DIFFERENT command identities
# sharing ONE target and one substantive stdout. Removing category as a TRIGGER
# must not remove this: neither identity's target can vouch for the other's.
category_adv_log="$TMP_DIR/bug028-category-adversarial.jsonl"
write_log "$category_adv_log" \
  "{\"ts\":\"2026-08-16T10:00:01Z\",\"sessionId\":\"b28c-a\",\"spec\":\"specs/alpha\",\"cmd\":\"cargo test\",\"exitCode\":0,\"durationMs\":601,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T10:00:03Z\",\"sessionId\":\"b28c-b\",\"spec\":\"specs/alpha\",\"cmd\":\"npm run lint\",\"exitCode\":0,\"durationMs\":603,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"

category_adv_out="$(analyze "$category_adv_log")"
if [[ "$(clone_count "$category_adv_out")" == "1" ]]; then
  pass "BUG-028 defect 1 bound: two identities sharing ONE target and one stdout are still refused"
else
  fail "BUG-028 defect 1 bound: expected 1 clone group for cargo-vs-npm over one target, observed $(clone_count "$category_adv_out")"
  printf '  analysis: %s\n' "$category_adv_out"
fi

# ---------------------------------------------------------------------------
# ADVERSARIAL BOUND, cross-spec reuse. The receipt log is repository-wide, and
# grouping deliberately spans every spec: that is what catches one captured
# result reused for an UNRELATED claim. Two incompatible identities colliding
# on one stdout across two specs must still be refused.
# ---------------------------------------------------------------------------
cross_log="$TMP_DIR/bug028-cross-spec.jsonl"
write_log "$cross_log" \
  "{\"ts\":\"2026-08-16T10:20:01Z\",\"sessionId\":\"b28x-a\",\"spec\":\"specs/alpha\",\"cmd\":\"cargo test\",\"exitCode\":0,\"durationMs\":801,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T10:20:03Z\",\"sessionId\":\"b28x-b\",\"spec\":\"specs/gamma\",\"cmd\":\"npm run lint\",\"exitCode\":0,\"durationMs\":803,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"

cross_out="$(analyze "$cross_log")"
if [[ "$(clone_count "$cross_out")" == "1" ]]; then
  pass "BUG-028 bound: a group mixing two specs on one stdout is still refused"
else
  fail "BUG-028 bound: cross-spec reuse no longer blocks (observed $(clone_count "$cross_out"))"
  printf '  analysis: %s\n' "$cross_out"
fi

# ---------------------------------------------------------------------------
# BUG-028 canonical case — ONE deterministic validator over THREE subjects.
#
# This is the reported defect. `artifact-lint.sh` never prints its subject, so
# three honest runs over three different improvement directories share one
# substantive stdout. They were refused because the operator tagged them
# differently, which a category-uniqueness constraint read as incompatible
# evidence. Distinct subjects and distinct executions are what make these
# independent; the label an agent chose is not part of the program's identity.
# ---------------------------------------------------------------------------
canonical_log="$TMP_DIR/bug028-canonical-three-subjects.jsonl"
write_log "$canonical_log" \
  "{\"ts\":\"2026-08-16T11:00:01Z\",\"sessionId\":\"b28k-1\",\"spec\":\"specs/alpha\",\"scope\":\"SCOPE-1\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh improvements/BUG-013-alpha\",\"exitCode\":0,\"durationMs\":701,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}" \
  "{\"ts\":\"2026-08-16T11:00:05Z\",\"sessionId\":\"b28k-2\",\"spec\":\"specs/alpha\",\"scope\":\"SCOPE-1\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh improvements/BUG-018-beta\",\"exitCode\":0,\"durationMs\":702,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"test\"]}" \
  "{\"ts\":\"2026-08-16T11:00:09Z\",\"sessionId\":\"b28k-3\",\"spec\":\"specs/alpha\",\"scope\":\"SCOPE-1\",\"cmd\":\"bash bubbles/scripts/artifact-lint.sh improvements/BUG-019-gamma\",\"exitCode\":0,\"durationMs\":703,\"stdoutHash\":\"$NONEMPTY\",\"stdoutBytes\":128,\"tags\":[\"lint\"]}"

canonical_out="$(analyze "$canonical_log")"
if [[ "$(clone_count "$canonical_out")" == "0" ]]; then
  pass "BUG-028 canonical: one validator over three subjects with differing tags is not reported as cloned evidence"
else
  fail "BUG-028 canonical: differing tags across one validator's three subjects still allege forgery ($(clone_count "$canonical_out") group(s))"
  printf '  analysis: %s\n' "$canonical_out"
fi
if [[ "$(sibling_count "$canonical_out")" == "1" ]]; then
  pass "BUG-028 canonical: the three-subject group is accepted through the deterministic-sibling path"
else
  fail "BUG-028 canonical: expected exactly 1 accepted sibling group, observed $(sibling_count "$canonical_out")"
  printf '  analysis: %s\n' "$canonical_out"
fi

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
