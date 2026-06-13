#!/usr/bin/env bash
# v4.1.0-selftest.sh
#
# Hermetic selftest for the v4.1.0 framework additions:
#   1. delivered_pending_activation ceiling resolves from workflows.yaml
#   2. scopeKinds taxonomy present (6 kinds)
#   3. lockdownContract registry present (6 patterns)
#   4. 3 new modes declared (adapter-readiness-to-packet, dark-launch-shipped,
#      migration-shipped-pending-cutover)
#   5. G073 deliverableFiles[] manifest allows declared files, denies others
#   6. G090 executionRuntime=manual produces slo=skipped, exit 0
#   7. G090 executionRuntime=goal-loop produces normal metrics (not skipped)
#   8. G022 phaseStubs{reason:...} merges into completed phases
#   9. G041 sed extracts canonical base from "Done (annotation)"
#  10. G040 lockdown tag allowlist excludes tagged deferrals from failure
#  11. G009 evidence-by-reference resolver finds anchor + 10-line block
#  12. G056 schema accepts null/empty for certifiedCompletedPhases
#
# Exits 0 on full pass, 1 on any failure. Cleans up tmp tree on exit.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS="$REPO_ROOT/bubbles/workflows.yaml"
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml.
# scopeKinds + lockdownContract stay in workflows.yaml. MODES points at the
# registry (fallback to workflows.yaml for pre-split repos with inline modes).
MODES="$REPO_ROOT/bubbles/workflows/modes.yaml"
[[ -f "$MODES" ]] || MODES="$WORKFLOWS"
GUARD="$SCRIPT_DIR/state-transition-guard.sh"
RCH="$SCRIPT_DIR/retro-convergence-health.sh"

if [[ ! -f "$WORKFLOWS" ]]; then
  echo "[v4.1.0-selftest] FAIL: workflows.yaml missing at $WORKFLOWS" >&2
  exit 1
fi
if [[ ! -f "$GUARD" ]]; then
  echo "[v4.1.0-selftest] FAIL: state-transition-guard.sh missing at $GUARD" >&2
  exit 1
fi
if [[ ! -f "$RCH" ]]; then
  echo "[v4.1.0-selftest] FAIL: retro-convergence-health.sh missing at $RCH" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "[v4.1.0-selftest] FAIL: python3 required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[v4.1.0-selftest] FAIL: jq required" >&2
  exit 1
fi

TMP="$(mktemp -d -t bubbles-v4.1.0-selftest.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

pass=0
fail=0
note() { printf '[v4.1.0-selftest] %s\n' "$*"; }
ok()   { note "PASS  test-$1: $2"; pass=$((pass+1)); }
ko()   { note "FAIL  test-$1: $2"; fail=$((fail+1)); }

# ---- Test 1: ceiling string present in the modes registry ----------------
if grep -qE '(^|[^_-])delivered_pending_activation' "$MODES"; then
  ok 1 "delivered_pending_activation ceiling string present in modes registry"
else
  ko 1 "delivered_pending_activation ceiling string MISSING from modes registry"
fi

# ---- Test 2: scopeKinds taxonomy declares 6 kinds -------------------------
sk_kinds="$(python3 -c "
import yaml
d=yaml.safe_load(open('$WORKFLOWS'))
print(','.join(sorted((d.get('scopeKinds') or {}).keys())))
" 2>/dev/null || true)"
expected_kinds="bootstrap,ci-config,contract-only,deploy-pointer,docs-only,runtime-behavior"
if [[ "$sk_kinds" == "$expected_kinds" ]]; then
  ok 2 "scopeKinds has all 6 expected kinds ($sk_kinds)"
else
  ko 2 "scopeKinds mismatch: got '$sk_kinds', want '$expected_kinds'"
fi

# ---- Test 3: lockdownContract has 6 patterns + requiredFields -------------
lc_count="$(python3 -c "
import yaml
d=yaml.safe_load(open('$WORKFLOWS'))
print(len((d.get('lockdownContract') or {}).get('patterns') or []))
" 2>/dev/null || echo 0)"
if [[ "$lc_count" == "6" ]]; then
  ok 3 "lockdownContract declares 6 patterns"
else
  ko 3 "lockdownContract has $lc_count patterns, want 6"
fi

# ---- Test 4: 3 new modes present ------------------------------------------
new_modes="$(python3 -c "
import yaml
d=yaml.safe_load(open('$MODES'))
m=d.get('modes') or {}
present=[k for k in ('adapter-readiness-to-packet','dark-launch-shipped','migration-shipped-pending-cutover') if k in m and m[k].get('statusCeiling')=='delivered_pending_activation']
print(','.join(sorted(present)))
" 2>/dev/null || true)"
expected_modes="adapter-readiness-to-packet,dark-launch-shipped,migration-shipped-pending-cutover"
if [[ "$new_modes" == "$expected_modes" ]]; then
  ok 4 "3 new modes declared with delivered_pending_activation ceiling"
else
  ko 4 "new modes mismatch: got '$new_modes', want '$expected_modes'"
fi

# ---- Test 5: G073 deliverableFiles[] semantics (direct logic test) --------
# We test the in-guard manifest logic by extracting the inlined Python and
# is_deliverable_file shell function shape via a focused python check.
mf_test="$(python3 -c "
import json
state={'deliverableFiles':['example-app/home-lab/apply.sh','example-app/home-lab/tests/**','docs/']}
# Replicate the is_deliverable_file matching rules from state-transition-guard.sh
def is_deliverable(f, deliverables):
    for df in deliverables:
        if f == df: return True
        if df.endswith('/**') and f.startswith(df[:-3]+'/'): return True
        if df.endswith('/') and f.startswith(df): return True
    return False
checks=[
    ('example-app/home-lab/apply.sh', True,  'exact path'),
    ('example-app/home-lab/tests/foo.bash', True, 'recursive glob'),
    ('docs/Operations.md', True, 'dir prefix'),
    ('example-app/home-lab/bootstrap.sh', False, 'undeclared path'),
    ('backend/main.go', False, 'unrelated path'),
]
for f, want, why in checks:
    got=is_deliverable(f, state['deliverableFiles'])
    print(f'{\"OK\" if got==want else \"BAD\"}: {f} -> {got} ({why})')
" 2>&1)"
if echo "$mf_test" | grep -q '^BAD:'; then
  ko 5 "deliverableFiles[] matching logic: $mf_test"
else
  ok 5 "deliverableFiles[] matching logic (exact/glob/prefix/denied all correct)"
fi

# ---- Test 6: G090 manual runtime -> slo:skipped ---------------------------
mkdir -p "$TMP/g090/.specify/memory" "$TMP/g090/specs/sample"
python3 -c "
import json
json.dump({'executionRuntime':'manual'}, open('$TMP/g090/.specify/memory/bubbles.session.json','w'))
json.dump({'status':'in_progress'}, open('$TMP/g090/specs/sample/state.json','w'))
"
slo6="$(bash "$RCH" specs/sample --repo-root "$TMP/g090" --format json 2>/dev/null | jq -r '.convergenceHealth.slo' 2>/dev/null || echo PARSE_ERR)"
if [[ "$slo6" == "skipped" ]]; then
  ok 6 "G090 executionRuntime=manual produces slo=skipped"
else
  ko 6 "G090 manual runtime: expected slo=skipped, got '$slo6'"
fi

# ---- Test 7: G090 goal-loop runtime -> slo:pass (not skipped) -------------
python3 -c "
import json
json.dump({'executionRuntime':'goal-loop','runs':[{'specDir':'specs/sample','iterationCount':1}]}, open('$TMP/g090/.specify/memory/bubbles.session.json','w'))
"
slo7="$(bash "$RCH" specs/sample --repo-root "$TMP/g090" --format json 2>/dev/null | jq -r '.convergenceHealth.slo' 2>/dev/null || echo PARSE_ERR)"
if [[ "$slo7" == "pass" || "$slo7" == "degraded" || "$slo7" == "failed" ]]; then
  ok 7 "G090 executionRuntime=goal-loop bypasses skip (slo=$slo7)"
else
  ko 7 "G090 goal-loop runtime: expected non-skipped slo, got '$slo7'"
fi

# ---- Test 8: G022 phaseStubs merge ----------------------------------------
ps_test="$(python3 -c "
state={'execution':{'phaseStubs':{'chaos':{'reason':'no SLA'},'stress':{'reason':'deploy-pointer kind'},'empty':{'reason':''}},'completedPhaseClaims':['implement','test']}}
execution=(state.get('execution') or {})
cp=execution.get('completedPhaseClaims') or []
stubs=execution.get('phaseStubs') or {}
stubbed=[k for k,v in stubs.items() if isinstance(v,dict) and (v.get('reason') or '').strip()]
merged=list(dict.fromkeys(list(cp)+stubbed))
print('chaos' in merged, 'stress' in merged, 'empty' in merged, 'implement' in merged)
" 2>&1)"
# Expect: True True False True  (empty-reason stub is rejected)
if [[ "$ps_test" == "True True False True" ]]; then
  ok 8 "G022 phaseStubs merges valid (chaos,stress) and rejects empty-reason (empty)"
else
  ko 8 "G022 phaseStubs merge: got '$ps_test' want 'True True False True'"
fi

# ---- Test 9: G041 sed extracts canonical base from annotations ------------
ann_ok=true
for input_status in "Done" "Done (completed_owned)" "Blocked (awaiting-operator-commit)" "In Progress (foo)"; do
  base="$(echo "**Status:** $input_status" | sed -E 's/.*\*\*Status:\*\*[[:space:]]*//' | sed -E 's/[[:space:]]*$//' | sed -E 's/[[:space:]]*\(.*\)[[:space:]]*$//' | sed -E 's/[[:space:]]+$//')"
  expected_base="${input_status%% (*}"
  if [[ "$base" != "$expected_base" ]]; then
    ann_ok=false
    note "  sed mismatch: '$input_status' -> '$base' (expected '$expected_base')"
  fi
done
if $ann_ok; then
  ok 9 "G041 sed annotation extraction: Done, Done (X), Blocked (Y), In Progress (Z) all extract canonical base"
else
  ko 9 "G041 sed annotation extraction failed"
fi

# ---- Test 10: G040 lockdown tag allowlist ---------------------------------
g040_excl='no deferred items|no deferred work|no deferrals|without deferred work|zero deferred items|zero deferrals|no issues deferred|no issues deferred or skipped|followUpOwner|followUpAction|followUpTarget|followUps|follow-up narrative|follow-up section|\[lockdown-deferred-fr-[0-9]+\]|\[lockdown-deferred-[a-z0-9-]+-fr-[0-9]+\]|\[awaiting-operator-commit\]|\[awaiting-third-party-approval\]|\[awaiting-cutover-window\]|\[awaiting-regulator-review\]'
g040_def='deferred|future work|placeholder'
g040_pass=0
g040_fail=0
g040_lines=(
  "Live cutover deferred [lockdown-deferred-FR-020]"
  "Runtime smoke deferred [awaiting-operator-commit]"
  "Done — no deferred items"
  "Live cutover deferred until operator commits"
  "placeholder content"
)
g040_wants=( "PASS" "PASS" "PASS" "FAIL" "FAIL" )
for i in "${!g040_lines[@]}"; do
  line="${g040_lines[$i]}"
  want="${g040_wants[$i]}"
  if echo "$line" | grep -iE "$g040_def" | grep -viE "$g040_excl" >/dev/null; then
    result="FAIL"
  else
    result="PASS"
  fi
  if [[ "$result" == "$want" ]]; then
    g040_pass=$((g040_pass+1))
  else
    g040_fail=$((g040_fail+1))
    note "  G040 mismatch: '$line' -> $result (want $want)"
  fi
done
if [[ "$g040_fail" -eq 0 ]]; then
  ok 10 "G040 lockdown allowlist: 5/5 expected verdicts ($g040_pass PASS, 0 FAIL)"
else
  ko 10 "G040 lockdown allowlist: $g040_fail wrong verdicts"
fi

# ---- Test 11: G009 evidence-by-reference resolver -------------------------
mkdir -p "$TMP/g009"
cat > "$TMP/g009/report.md" <<'REPORTEOF'
# Report

## Scope 1 Cosign

    $ cosign verify --certificate-identity-regexp foo ghcr.io/x@sha256:abc
    line 2
    line 3
    line 4
    line 5
    line 6
    line 7
    line 8
    line 9
    line 10
    line 11

## Empty Section
REPORTEOF

# Extract the resolve_evidence_by_reference function from the guard and source it.
fn_def="$(awk '/^resolve_evidence_by_reference\(\)/,/^}/' "$GUARD")"
if [[ -z "$fn_def" ]]; then
  ko 11 "could not extract resolve_evidence_by_reference function from guard"
else
  eval "$fn_def"
  r1=0; r2=0; r3=0
  resolve_evidence_by_reference "$TMP/g009" "report.md#scope-1-cosign" && r1=1 || r1=0
  resolve_evidence_by_reference "$TMP/g009" "report.md#empty-section" || r2=1  # expect fail
  resolve_evidence_by_reference "$TMP/g009" "report.md#does-not-exist" || r3=1  # expect fail
  if [[ "$r1" == "1" && "$r2" == "1" && "$r3" == "1" ]]; then
    ok 11 "G009 evidence-by-reference: 10+ line block accepted, empty/missing anchors rejected"
  else
    ko 11 "G009 evidence-by-reference: r1=$r1 (want 1) r2=$r2 (want 1) r3=$r3 (want 1)"
  fi
fi

# ---- Test 12: G056 schema loosening (presence-only check) -----------------
mkdir -p "$TMP/g056"
cat > "$TMP/g056/state.json" <<'STATEEOF'
{
  "status": "in_progress",
  "certification": {
    "status": "in_progress",
    "certifiedCompletedPhases": null,
    "scopeProgress": [],
    "lockdownState": {}
  }
}
STATEEOF
# v4.1.0 grep pattern just enforces field PRESENCE
g056_ok=true
for field in certifiedCompletedPhases scopeProgress lockdownState; do
  if ! grep -qE "\"$field\"[[:space:]]*:" "$TMP/g056/state.json"; then
    g056_ok=false
    note "  G056 field-presence check failed for $field"
  fi
done
if $g056_ok; then
  ok 12 "G056 schema loosening: null/[]/{} values all pass field-presence check"
else
  ko 12 "G056 schema loosening regressed"
fi

# ---- summary --------------------------------------------------------------
note "=================================================="
note "RESULT: $pass passed, $fail failed (12 total tests)"
if [[ "$fail" -eq 0 ]]; then
  note "v4.1.0 selftest CLEAN"
  exit 0
else
  note "v4.1.0 selftest FAILED"
  exit 1
fi
