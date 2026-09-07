#!/usr/bin/env bash
# bubbles/scripts/compact-obligation-basis-selftest.sh
#
# BUG-042 — the BEHAVIOURAL pin for the transition guard's third completion
# basis.
#
# WHY THIS IS A SEPARATE FILE
# BUG-041 recorded F-041-04 against its own coverage: it could pin that the
# guard REFERENCES bug-packet-resolve.sh, but not that the guard BEHAVES
# differently because of it. A wiring pin stays green while the wire carries
# nothing. The design for this change therefore required a behavioural pin, and
# named state-transition-guard-selftest.sh as the site.
#
# That file is 5,900+ lines and drives the guard dozens of times, and no session
# permitted to make this change has been permitted to run it end to end. A pin
# nobody can execute is the same failure as a pin that asserts nothing, arrived
# at from the other direction. So the pin lives here, where it is bounded (three
# guard invocations) and can be run and SHOWN to go red under mutation.
# framework-validate.sh's discovered-selftest sweep globs
# bubbles/scripts/*-selftest.sh, so this is executed with no wiring step —
# placement costs no coverage.
#
# WHAT IS PINNED
# Not "the guard mentions obligations". The guard's verdict must CHANGE with the
# packet's content, in both directions:
#   B1  four obligations attested        -> the basis PASSES
#   B2  one attestation removed          -> that obligation is REFUSED BY NAME
#   B3  one attestation left unchecked   -> refused, and distinguishably so
#   B4  the basis is actually SELECTED   -> without this, B2/B3 could be green
#                                           because the packet died earlier
#   B5  an attestation that does not cite its discharge site does NOT count
#
# B5 is the assertion that keeps `dischargedIn` load-bearing. Without it the
# registry could flatten every carrier to one artifact and nothing would notice.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/state-transition-guard.sh"
RESOLVER="$SCRIPT_DIR/bug-packet-resolve.sh"
NAME="compact-obligation-basis-selftest"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

[[ -f "$GUARD" ]] || {
  bad "the transition guard exists next to this selftest" "$GUARD"
  printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
  exit 1
}

# --- the obligation set under test, read from the registry -----------------
# Read, not restated. A pin carrying its own copy of the four ids would pass
# while the registry declared something else, which is the drift bug-packet.yaml
# exists to end.
facts="$(bash "$RESOLVER" 2>/dev/null)" || facts=""
compact_obligations="$(printf '%s\n' "$facts" | sed -n 's/^obligation=compact|//p')"
obligation_count="$(printf '%s\n' "$compact_obligations" | grep -c . || true)"

if [[ "$obligation_count" -ge 1 ]]; then
  ok "P0 the registry declares $obligation_count obligation(s) for the compact form"
else
  bad "P0 the registry declares compact obligations" "the fixtures below would assert nothing"
  printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
  exit 1
fi

# --- fixture base: a REAL compact packet -----------------------------------
# Synthesising one from scratch would mean synthesising eight admissible
# micro-fix answers in bug.md, and a fixture that drifts from the admission
# registry stops testing the real path. Copy a shipped compact packet instead:
# the copy is disposable and the original is never touched.
BUGS_ROOT="$SCRIPT_DIR/../../bugs"
source_packet=""
if [[ -d "$BUGS_ROOT" ]]; then
  for candidate in "$BUGS_ROOT"/BUG-*/; do
    [[ -f "$candidate/state.json" ]] || continue
    if grep -qE '"packet"[[:space:]]*:[[:space:]]*"(micro|compact)"' "$candidate/state.json" 2>/dev/null; then
      source_packet="${candidate%/}"
      break
    fi
  done
fi

if [[ -z "$source_packet" ]]; then
  bad "P1 a shipped compact packet is available as the fixture base" \
    "no bugs/BUG-*/state.json declares the compact form; the behavioural fixtures cannot be built"
  printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
  exit 1
fi
ok "P1 fixture base is the shipped compact packet $(basename "$source_packet")"

# Build one fixture. $1 = name, $2 = obligation id to damage, $3 = how
# ("omit" | "uncheck" | "no-discharge-site" | "" for none).
build_fixture() {
  local fixture_name="$1" damaged_id="${2:-}" damage="${3:-}"
  local dir="$WORK/$fixture_name"
  rm -rf "$dir"
  cp -R "$source_packet" "$dir"

  # Remove any attestation block the source packet already carries, so the
  # fixture's content is decided here and nowhere else. Scrubbing only the
  # marker line is NOT enough: a shipped compact packet carries its OWN
  # attestation lines, which have no marker, and those survived the copy and
  # silently re-satisfied every obligation this builder was asked to damage.
  # Every attestation-SHAPED line naming a declared obligation id goes.
  local report="$dir/report.md"
  [[ -f "$report" ]] || return 1
  local scrub_ids=""
  while IFS= read -r fact; do
    [[ -n "$fact" ]] || continue
    scrub_ids="${scrub_ids}${scrub_ids:+|}${fact%%|*}"
  done <<< "$compact_obligations"
  grep -v 'BUG-042 obligation attestation' "$report" \
    | grep -vE "^- \[[x ]\][^|]*(${scrub_ids})" >"$report.tmp" && mv "$report.tmp" "$report"

  {
    printf '\n## Obligation Attestation\n\n'
    printf 'BUG-042 obligation attestation fixture.\n\n'
    while IFS= read -r fact; do
      [[ -n "$fact" ]] || continue
      local id rest discharged
      id="${fact%%|*}"
      rest="${fact#*|}"
      discharged="${rest%%|*}"

      if [[ "$id" == "$damaged_id" ]]; then
        case "$damage" in
          omit) continue ;;
          uncheck) printf -- '- [ ] %s — discharged in %s\n' "$id" "$discharged"; continue ;;
          no-discharge-site) printf -- '- [x] %s — discharged somewhere\n' "$id"; continue ;;
        esac
      fi
      printf -- '- [x] %s — discharged in %s\n' "$id" "$discharged"
    done <<< "$compact_obligations"
    printf '\n'
  } >>"$report"
  printf '%s' "$dir"
}

run_guard() {
  local dir="$1" log="$2"
  bash "$GUARD" "$dir" >"$log" 2>&1
  return 0
}

PASS_MSG="registry-declared obligation(s) are attested"
BASIS_MSG="Completion basis: REGISTRY-DECLARED OBLIGATIONS"

# --- B1 / B4. every obligation attested -> the basis PASSES, and was USED ---
attested_dir="$(build_fixture attested)"
attested_log="$WORK/attested.log"
run_guard "$attested_dir" "$attested_log"

if grep -q "$BASIS_MSG" "$attested_log"; then
  ok "B4 the obligation basis is SELECTED for a compact packet (not skipped past)"
else
  bad "B4 the obligation basis is selected" "the guard never reported it; every assertion below would be vacuous"
fi

if grep -q "$PASS_MSG" "$attested_log" && ! grep -q "has NO attestation line" "$attested_log"; then
  ok "B1 a compact packet with every obligation attested PASSES the obligation basis"
else
  bad "B1 fully attested packet passes the basis" "$(grep -E "Obligation |$PASS_MSG" "$attested_log" | head -5 | tr '\n' '|')"
fi

# The old structural refusal must be GONE for this form. If it survives, the
# packet still cannot certify and the change bought nothing.
if ! grep -q "ZERO DoD checkbox items" "$attested_log"; then
  ok "B1b the 'ZERO DoD checkbox items' structural refusal no longer fires on a compact packet"
else
  bad "B1b structural refusal replaced" "the compact packet still dies at Check-4-structure"
fi

# --- B2. ONE obligation unattested -> refused, BY NAME ---------------------
target_id="$(printf '%s\n' "$compact_obligations" | head -1 | cut -d'|' -f1)"
omit_dir="$(build_fixture omitted "$target_id" omit)"
omit_log="$WORK/omitted.log"
run_guard "$omit_dir" "$omit_log"

if grep -q "Obligation '$target_id' has NO attestation line" "$omit_log" \
  && ! grep -q "$PASS_MSG" "$omit_log"; then
  ok "B2 removing the '$target_id' attestation REFUSES the packet, naming it"
else
  bad "B2 one unattested obligation is refused" "$(grep -E "Obligation |$PASS_MSG" "$omit_log" | head -5 | tr '\n' '|')"
fi

# The refusal must be SPECIFIC. A basis that refuses everything the moment one
# item is missing tells the author nothing, and would also stay green if the
# check were replaced by an unconditional failure.
other_refusals="$(grep -c "has NO attestation line" "$omit_log" || true)"
if [[ "$other_refusals" -eq 1 ]]; then
  ok "B2b exactly ONE obligation is refused — the other $((obligation_count - 1)) still pass"
else
  bad "B2b the refusal is specific" "$other_refusals obligations were refused; expected exactly 1"
fi

# --- B3. an UNCHECKED attestation is refused, and distinguishably ----------
uncheck_dir="$(build_fixture unchecked "$target_id" uncheck)"
uncheck_log="$WORK/unchecked.log"
run_guard "$uncheck_dir" "$uncheck_log"

if grep -q "Obligation '$target_id' is declared by bug-packet.yaml but its attestation line" "$uncheck_log" \
  && grep -q "is UNCHECKED" "$uncheck_log"; then
  ok "B3 an UNCHECKED attestation is refused, and is reported differently from a missing one"
else
  bad "B3 unchecked attestation refused distinguishably" "$(grep -E "Obligation " "$uncheck_log" | head -5 | tr '\n' '|')"
fi

# --- B5. an attestation that does not cite its discharge site does NOT count -
nosite_dir="$(build_fixture nosite "$target_id" no-discharge-site)"
nosite_log="$WORK/nosite.log"
run_guard "$nosite_dir" "$nosite_log"

if grep -q "Obligation '$target_id' has NO attestation line" "$nosite_log"; then
  ok "B5 a ticked attestation that does not name its dischargedIn artifact does NOT satisfy the obligation"
else
  bad "B5 dischargedIn is load-bearing" "a bare tick satisfied '$target_id'; the carrier field is decorative"
fi

# --- B6 / B7. artifact-lint requires the line to EXIST, not to be ticked ----
# The two surfaces must agree, or a packet passes lint on a line the guard then
# rejects. B7 is the half that matters: lint runs long before `done`, so an
# UNCHECKED attestation must satisfy lint and be refused only by the guard.
LINT="$SCRIPT_DIR/artifact-lint.sh"
if [[ ! -f "$LINT" ]]; then
  bad "B6 artifact-lint.sh exists next to this selftest" "$LINT"
else
  omit_lint_log="$WORK/omitted-lint.log"
  bash "$LINT" "$omit_dir" >"$omit_lint_log" 2>&1
  if grep -q "Obligation '$target_id' has NO attestation line" "$omit_lint_log"; then
    ok "B6 artifact-lint refuses a compact packet missing an obligation attestation line"
  else
    bad "B6 lint refuses a missing attestation" "$(grep -iE "obligation" "$omit_lint_log" | head -5 | tr '\n' '|')"
  fi

  uncheck_lint_log="$WORK/unchecked-lint.log"
  bash "$LINT" "$uncheck_dir" >"$uncheck_lint_log" 2>&1
  if grep -q "Obligation '$target_id' has an attestation line" "$uncheck_lint_log"; then
    ok "B7 artifact-lint accepts an UNCHECKED attestation — existence is lint's question, ticking is the guard's"
  else
    bad "B7 lint accepts an unchecked attestation" "$(grep -iE "obligation '$target_id'" "$uncheck_lint_log" | head -3 | tr '\n' '|')"
  fi
fi

# --- B8. a scopes.md-less form claiming completed scopes is a contradiction --
# Check 5 becomes NOT_APPLICABLE on this form, which is a waiver unless it
# substitutes the assertion that IS meaningful. This pins the substitution.
claim_dir="$(build_fixture claims-scopes)"
python3 - "$claim_dir/state.json" <<'PY' 2>/dev/null || true
import json, sys
path = sys.argv[1]
with open(path, encoding="utf-8") as fh:
    data = json.load(fh)
data.setdefault("certification", {})["completedScopes"] = ["01-invented-scope"]
with open(path, "w", encoding="utf-8") as fh:
    json.dump(data, fh, indent=2)
PY
claim_log="$WORK/claims-scopes.log"
run_guard "$claim_dir" "$claim_log"
if grep -q "yet state.json claims 1 completed scope" "$claim_log"; then
  ok "B8 a compact packet claiming a completed scope is REFUSED — Check 5 substitutes, it does not waive"
else
  bad "B8 scopeless form cannot claim completed scopes" "$(grep -E "Check-5|completedScopes|completed scope" "$claim_log" | head -5 | tr '\n' '|')"
fi

# --- B9. Check 4A follows the DoD content to the attestation artifact -------
# If the relocation moved DoD-shaped content out of scopes.md and the format
# checks kept scanning only scopes.md, the G041 reformatting bypass reopens in
# the new location.
bypass_dir="$(build_fixture format-bypass)"
{
  printf '\n## Definition of Done\n\n'
  printf -- '- (deferred) an item reformatted out of checkbox shape\n\n'
} >>"$bypass_dir/report.md"
bypass_log="$WORK/format-bypass.log"
run_guard "$bypass_dir" "$bypass_log"
if grep -q "DoD format manipulation detected in report.md" "$bypass_log"; then
  ok "B9 Check 4A scans the attestation artifact, so the G041 bypass does not reopen there"
else
  bad "B9 Check 4A follows the relocation" "$(grep -E "format manipulation" "$bypass_log" | head -3 | tr '\n' '|')"
fi

# --- B10 / B11. Gate G027 is FORM-AWARE (BUG-042, DI-038-04) ----------------
# G027 asked "implement/test claimed => scopes completed?". A form declaring no
# scopes.md cannot answer yes, and Check 5 refuses it if it tries — so the
# DEFAULT bug route had NO satisfying value and was unfalsifiable. G027 now asks
# the equivalent question this form CAN answer: are the registry-declared
# obligations attested? These reuse the fixtures already built above, so they
# cost no additional guard invocation.
#
# Both halves are pinned. B10 is worthless alone: a G027 that simply skipped
# this form would pass it. B11 is the half that keeps the gate a gate.
G027_PASS="Phase-obligation coherence verified"
G027_SCOPE_PROXY_A="completedScopes is EMPTY — FABRICATION (Gate G027)"
G027_SCOPE_PROXY_B="ZERO scopes are marked 'Done' — FABRICATION (Gate G027)"

fixture_phases="$(grep -oE '"phase"[[:space:]]*:[[:space:]]*"(implement|test)"' "$attested_dir/state.json" | head -2 | tr '\n' ' ')"
if [[ -n "$fixture_phases" ]]; then
  ok "B10a the fixture base records an implement/test phase claim, so G027's implementation branch is entered"
else
  bad "B10a the fixture base claims implement/test" \
    "no implement/test phase claim in the fixture state.json; B10/B11 would assert nothing"
fi

if grep -q "$G027_PASS" "$attested_log" \
  && ! grep -qF "$G027_SCOPE_PROXY_A" "$attested_log" \
  && ! grep -qF "$G027_SCOPE_PROXY_B" "$attested_log"; then
  ok "B10 a fully attested compact packet claiming implement/test CLEARS G027 — the scope-count proxy no longer contradicts Check 5"
else
  bad "B10 G027 is form-aware for the compact form" \
    "$(grep -E "Gate G027|$G027_PASS" "$attested_log" | head -4 | tr '\n' '|')"
fi

if grep -qE "registry-declared obligation '$target_id' is NOT attested — FABRICATION \(Gate G027\)" "$omit_log" \
  && ! grep -q "$G027_PASS" "$omit_log"; then
  ok "B11 a compact packet claiming implement/test with '$target_id' UNATTESTED still FAILS G027 — anti-fabrication survives the form-awareness"
else
  bad "B11 G027 still refuses an unevidenced compact phase claim" \
    "$(grep -E "Gate G027|$G027_PASS" "$omit_log" | head -4 | tr '\n' '|')"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
