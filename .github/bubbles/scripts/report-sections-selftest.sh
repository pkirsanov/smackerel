#!/usr/bin/env bash
# bubbles/scripts/report-sections-selftest.sh
#
# Hermetic selftest for the IMP-047 S-B report-section contract.
#
# The measured defect this pins closed: `feature-templates.md` contained ZERO
# occurrences of `Completion Statement`, `Validation Evidence`, `Audit Evidence`,
# or `Chaos Evidence`, while `artifact-lint.sh` required all four. The framework
# shipped a template that failed its own lint plus `report-section-autofix.sh` to
# inject the missing headings after the fact.
#
# The load-bearing case is A1: a `report.md` authored VERBATIM from the canonical
# generated template must satisfy every section check on FIRST WRITE with no
# autofix. A2 is its non-vacuity twin — delete one section and the same lint must
# name it — because a section check that passes on everything proves nothing.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="report-sections-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

TEMPLATE="$REPO_ROOT/agents/bubbles_shared/feature-templates.md"
REGISTRY="$REPO_ROOT/bubbles/registry/report-sections.yaml"

# Extract the report.md body from the GENERATED block, exactly as an author
# copying the canonical template would.
author_report_from_template() {
  TEMPLATE="$TEMPLATE" python3 - "$1" <<'PY'
import os, sys
src = open(os.environ["TEMPLATE"], encoding="utf-8").read()
start = "<!-- GENERATED:REPORT_TEMPLATE_START"
end = "<!-- GENERATED:REPORT_TEMPLATE_END"
if start not in src or end not in src:
    sys.exit(3)
block = src.split(start, 1)[1].split(end, 1)[0]
body = block.split("```markdown\n", 1)[1].rsplit("\n```", 1)[0]
open(sys.argv[1], "w", encoding="utf-8").write(body + "\n")
PY
}

# --- P1. the resolver reads the registry and reproduces its facts ----------
RES="$(bash "$SCRIPT_DIR/report-sections-resolve.sh" 2>&1)"; rc=$?
if [[ "$rc" -eq 0 ]] &&
  printf '%s' "$RES" | grep -q '^always=Completion Statement|yes' &&
  printf '%s' "$RES" | grep -q '^mode=full-delivery|Validation Evidence;Audit Evidence;Chaos Evidence' &&
  printf '%s' "$RES" | grep -q '^mode=audit-only|Audit Evidence'; then
  ok "P1 the registry resolver reports the always-required and per-mode sections"
else
  bad "P1 registry resolver" "rc=$rc out=$(printf '%s' "$RES" | tr '\n' '|')"
fi

# --- A0. ADVERSARIAL: no fallback list when the registry is unreadable ------
# Degrading to "no sections required" would be a false-PASS of exactly the PD-04
# shape: a check that reports clean because it could not run.
missing_out="$(bash "$SCRIPT_DIR/report-sections-resolve.sh" --registry "$WORK/absent.yaml" 2>&1)"; rc=$?
if [[ "$rc" -eq 2 ]] && printf '%s' "$missing_out" | grep -q 'registry not found'; then
  ok "A0 an unreadable registry exits 2 instead of resolving an empty contract"
else
  bad "A0 unreadable registry refused" "rc=$rc out=$(printf '%s' "$missing_out" | tr '\n' '|')"
fi

# --- A0b. ADVERSARIAL: a registry naming an undeclared section is refused ---
cat >"$WORK/bad-registry.yaml" <<'EOF'
schemaVersion: report-sections/v1
alwaysRequired:
  - id: summary
    heading: Summary
    acceptShallow: true
promotionSections:
  - id: audit-evidence
    heading: Audit Evidence
    owner: bubbles.audit
modeRequired:
  enforceWhenStatusIn: [done]
  groups:
    - id: g1
      sections: [audit-evidence, does-not-exist]
      modes:
        - audit-only
EOF
badreg_out="$(bash "$SCRIPT_DIR/report-sections-resolve.sh" --registry "$WORK/bad-registry.yaml" 2>&1)"; rc=$?
if [[ "$rc" -eq 2 ]] && printf '%s' "$badreg_out" | grep -q 'undeclared section id'; then
  ok "A0b a registry referencing an undeclared section id is refused"
else
  bad "A0b undeclared section id refused" "rc=$rc out=$(printf '%s' "$badreg_out" | tr '\n' '|')"
fi

# --- P2. the shipped template is in sync with the registry ------------------
gen_out="$(bash "$SCRIPT_DIR/generate-report-template.sh" --check 2>&1)"; rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "P2 the shipped report template matches the registry (generator --check)"
else
  bad "P2 template in sync" "rc=$rc out=$(printf '%s' "$gen_out" | tr '\n' '|')"
fi

# --- A1. THE LOAD-BEARING CASE ---------------------------------------------
# A report.md authored verbatim from the canonical template must satisfy every
# section check on first write, with no autofix. Before S-B this was impossible:
# the template omitted four of the sections the lint required.
FIX="$WORK/fixture"
mkdir -p "$FIX/specs/001-x"
author_report_from_template "$FIX/specs/001-x/report.md" || bad "A1 could not author from template"
cat >"$FIX/specs/001-x/state.json" <<'EOF'
{"status":"done","workflowMode":"full-delivery"}
EOF
lint_out="$(cd "$REPO_ROOT" && bash "$SCRIPT_DIR/artifact-lint.sh" "$FIX/specs/001-x" 2>&1 || true)"
section_failures="$(printf '%s\n' "$lint_out" | grep -E 'missing required section|requires report.md section' || true)"
# Non-vacuity: the section checks must actually have RUN, not been skipped.
section_passes="$(printf '%s\n' "$lint_out" | grep -cE 'report.md contains section matching' || true)"
if [[ -z "$section_failures" ]] && [[ "$section_passes" -ge 3 ]]; then
  ok "A1 a report.md authored from the canonical template passes every section check on first write"
else
  bad "A1 first-write section pass" \
    "passes=$section_passes failures=$(printf '%s' "$section_failures" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL non-vacuity twin of A1 --------------------------------
# Delete one required section from the authored report. The SAME lint must name
# it. Without this, A1 would also pass against a lint that checks nothing.
python3 - "$FIX/specs/001-x/report.md" <<'PY'
import sys
path = sys.argv[1]
out, drop = [], False
for line in open(path, encoding="utf-8"):
    if line.startswith("### Completion Statement"):
        drop = True
        continue
    if drop and line.startswith("### "):
        drop = False
    if not drop:
        out.append(line)
open(path, "w", encoding="utf-8").write("".join(out))
PY
lint_out2="$(cd "$REPO_ROOT" && bash "$SCRIPT_DIR/artifact-lint.sh" "$FIX/specs/001-x" 2>&1 || true)"
if printf '%s' "$lint_out2" | grep -q 'missing required section matching.*Completion Statement'; then
  ok "A2 removing a required section makes the same lint name it"
else
  bad "A2 missing section named" "out=$(printf '%s' "$lint_out2" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: the autofix script is absent and uncalled -------------
# The Retirement Rule half of S-B. A generated template makes injection
# unnecessary, and an injected EMPTY heading was always a way to satisfy a
# section check while carrying no evidence.
#
# The scan looks for INVOCATIONS, not mentions. Several surviving comments name
# the retired script to explain why it is gone; a grep for the bare name would
# report those as callers and make this case unfixable by construction.
autofix_present="no"
[[ -e "$SCRIPT_DIR/report-section-autofix.sh" ]] && autofix_present="yes"
autofix_callers="$(cd "$REPO_ROOT" && grep -rnE '(bash|sh|exec|source)[[:space:]]+[^[:space:]]*report-section-autofix\.sh' \
  bubbles/scripts agents instructions prompts templates 2>/dev/null \
  | grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' || true)"
if [[ "$autofix_present" == "no" ]] && [[ -z "$autofix_callers" ]]; then
  ok "A3 report-section-autofix.sh is absent and has no invocation"
else
  bad "A3 autofix retired" "present=$autofix_present callers=$(printf '%s' "$autofix_callers" | tr '\n' ' ')"
fi

# --- A4. ADVERSARIAL: --autofix is refused, not silently ignored ------------
# Silently accepting a removed flag would let a caller believe injection still
# happened. The flag must refuse and say what replaced it.
flag_out="$(cd "$REPO_ROOT" && bash "$SCRIPT_DIR/artifact-lint.sh" "$FIX/specs/001-x" --autofix 2>&1 || true)"
if printf '%s' "$flag_out" | grep -q 'was removed by IMP-047 S-B'; then
  ok "A4 the removed --autofix flag refuses and names its replacement"
else
  bad "A4 --autofix refused" "out=$(printf '%s' "$flag_out" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: template drift is detected, not tolerated -------------
# Hand-editing inside the GENERATED markers is the drift S-B exists to prevent.
DRIFT="$WORK/drift"
mkdir -p "$DRIFT/bubbles/registry" "$DRIFT/agents/bubbles_shared"
cp "$REGISTRY" "$DRIFT/bubbles/registry/report-sections.yaml"
python3 - "$TEMPLATE" "$DRIFT/agents/bubbles_shared/feature-templates.md" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
src = src.replace("### Chaos Evidence", "### Chaos Evidence (hand-edited)", 1)
open(sys.argv[2], "w", encoding="utf-8").write(src)
PY
drift_out="$(bash "$SCRIPT_DIR/generate-report-template.sh" --check --repo-root "$DRIFT" 2>&1)"; rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$drift_out" | grep -q 'DRIFT'; then
  ok "A5 a hand edit inside the GENERATED markers is reported as drift"
else
  bad "A5 template drift detected" "rc=$rc out=$(printf '%s' "$drift_out" | tr '\n' '|')"
fi

# --- P3. the four contested headings are present in the shipped template ----
# The literal measurement from the IMP-047 problem statement, inverted.
missing_headings=""
for heading in "Completion Statement" "Validation Evidence" "Audit Evidence" "Chaos Evidence"; do
  grep -q "^### $heading" "$TEMPLATE" || missing_headings="$missing_headings $heading"
done
if [[ -z "$missing_headings" ]]; then
  ok "P3 the template now contains all four previously-absent required headings"
else
  bad "P3 contested headings present" "missing:$missing_headings"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
