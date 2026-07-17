# shellcheck shell=bash
# shellcheck disable=SC2154  # sourced fragment: all referenced vars are set in state-transition-guard.sh's scope before sourcing (see header)
# =============================================================================
# guards/control-plane-checks.sh  (M4 guard split)
# =============================================================================
# Checks 3A, 3H, 3C, 3D, 3E, 3F: the v3 control-plane gates — policy snapshot
# provenance (G055), validate-owned certification state (G056), scenario
# manifest integrity (G057), lockdown/regression contracts (G058/G059),
# scenario-first TDD evidence (G060), and transition/rework packet closure
# (G061/G063). Sourced by state-transition-guard.sh in the same shell scope, so
# pass/fail/warn/info, the failures/warnings counters, and all computed vars
# ($state_file, $state_status, $state_workflow_mode, $scenario_manifest_file,
# $lockdown_approvals_file, $invalidation_ledger_file, $transition_requests_file,
# $rework_queue_file, $scope_files[], $report_files[], count_gherkin_scenarios,
# json_nested_string, relative_artifact_path) are all in scope exactly as
# before extraction. Behavior is byte-identical to the previous inline blocks.
# Check 3G (framework ownership / BUG-001 timeout) intentionally stays inline.
# =============================================================================

# Control-plane policy-activation fix: the cutoff date for the grandfather clause
# on newly-activated G060 enforcement. Specs that carry no policySnapshot and whose
# createdAt predates this date are not retro-broken (see policy_spec_grandfathered
# in guard-lib.sh). Mirrors the G094 grandfather pattern.
control_plane_policy_cutoff="2026-06-18"

# =============================================================================
# CHECK 3A: Policy Snapshot Provenance (Gate G055)
# =============================================================================
echo "--- Check 3A: Policy Snapshot Provenance (Gate G055) ---"
if grep -qE '"policySnapshot"[[:space:]]*:[[:space:]]*\{' "$state_file"; then
  pass "state.json contains policySnapshot"

  missing_policy_entries=0
  for policy_name in grill tdd autoCommit lockdown regression validation; do
    if grep -qE "\"$policy_name\"[[:space:]]*:[[:space:]]*\{" "$state_file"; then
      pass "policySnapshot records $policy_name"
    else
      fail "policySnapshot missing $policy_name entry (Gate G055)"
      missing_policy_entries=$((missing_policy_entries + 1))
    fi
  done

  source_hits="$(grep -cE '"source"[[:space:]]*:[[:space:]]*"(user-request|repo-default|workflow-forced|spec-lockdown)"' "$state_file" || true)"
  if [[ "$source_hits" -ge 3 ]]; then
    pass "policySnapshot records allowed provenance values"
  else
    fail "policySnapshot does not record enough valid provenance fields (Gate G055)"
  fi

  if [[ "$missing_policy_entries" -eq 0 ]]; then
    pass "policySnapshot covers the control-plane defaults required for this run"
  fi
else
  # Control-plane policy-activation fix: a missing policySnapshot is no longer a
  # hard fail. The repo SST config (.specify/memory/bubbles.config.json) IS the
  # provenance of record when a spec carries no per-spec snapshot, so resolve
  # provenance from it and PASS with an INFO note. Only a missing snapshot AND a
  # missing SST config leaves provenance genuinely unverifiable.
  if [[ -f "$guard_repo_root/.specify/memory/bubbles.config.json" ]]; then
    pass "policySnapshot absent — control-plane provenance resolved from the repo SST config (.specify/memory/bubbles.config.json)"
    cp_grill_source="$(resolve_effective_policy_source "$state_file" grill mode off "$guard_repo_root")"
    info "[G055] Effective control-plane provenance is 'repo-default' — the SST config is the provenance of record when a spec omits policySnapshot (e.g. grill source=${cp_grill_source})"
  else
    fail "state.json missing policySnapshot AND no repo SST config at .specify/memory/bubbles.config.json — control-plane provenance cannot be verified (Gate G055)"
  fi
fi
echo ""

# =============================================================================
# CHECK 3H: Validate-owned certification state (Gate G056)
# =============================================================================
echo "--- Check 3H: Validate Certification State (Gate G056) ---"
if grep -qE '"certification"[[:space:]]*:[[:space:]]*\{' "$state_file"; then
  pass "state.json contains certification block"

  certification_status="$(json_nested_string "certification" "status" "$state_file" || true)"

  if [[ -n "$certification_status" ]]; then
    if [[ -n "$state_status" && "$certification_status" != "$state_status" ]]; then
      fail "Top-level status ('$state_status') does not match certification.status ('$certification_status') (Gate G056)"
    else
      pass "Top-level status matches certification.status ($certification_status)"
    fi
  else
    fail "certification block is missing status field (Gate G056)"
  fi

  # v4.1.0: G056 schema loosening. Accept presence of the field with any
  # value type (array, object, null, empty). Pre-v4.1.0 the grep patterns
  # required `: [` or `: {` literal starts, which fired false positives
  # whenever the certifying agent (bubbles.validate) emitted `null` or
  # `[]` / `{}` placeholders before the first scope landed. Field
  # presence is what the gate must enforce; the field's structural
  # content is checked by other gates (G024, G026, G027, etc.).
  if grep -qE '"certifiedCompletedPhases"[[:space:]]*:' "$state_file"; then
    pass "certification block records certifiedCompletedPhases (any value type)"
  else
    fail "certification block missing certifiedCompletedPhases (Gate G056)"
  fi

  if grep -qE '"scopeProgress"[[:space:]]*:' "$state_file"; then
    pass "certification block records scopeProgress (any value type)"
  else
    fail "certification block missing scopeProgress (Gate G056)"
  fi

  if grep -qE '"lockdownState"[[:space:]]*:' "$state_file"; then
    pass "certification block records lockdownState (any value type)"
  else
    fail "certification block missing lockdownState (Gate G056)"
  fi
else
  fail "state.json missing certification block — validate-owned promotion state cannot be verified (Gate G056)"
fi
echo ""

# =============================================================================
# CHECK 3C: Scenario contract manifest (Gate G057)
# =============================================================================
echo "--- Check 3C: Scenario Manifest Integrity (Gate G057) ---"
gherkin_scenario_count="$(count_gherkin_scenarios)"
if [[ "$gherkin_scenario_count" -gt 0 ]]; then
  if [[ -f "$scenario_manifest_file" ]]; then
    pass "Scenario manifest exists: $(relative_artifact_path "$scenario_manifest_file")"

    manifest_scenario_count="$(grep -cE '"scenarioId"[[:space:]]*:' "$scenario_manifest_file" || true)"
    manifest_test_type_count="$(grep -cE '"requiredTestType"[[:space:]]*:' "$scenario_manifest_file" || true)"
    manifest_linked_test_count="$(grep -cE '"linkedTests"[[:space:]]*:' "$scenario_manifest_file" || true)"
    manifest_evidence_count="$(grep -cE '"evidenceRefs"[[:space:]]*:' "$scenario_manifest_file" || true)"

    if [[ "$manifest_scenario_count" -lt "$gherkin_scenario_count" ]]; then
      fail "scenario-manifest.json only tracks $manifest_scenario_count scenarios but resolved scopes define $gherkin_scenario_count Gherkin scenarios (Gate G057)"
    else
      pass "scenario-manifest.json covers at least as many scenarios as the scope artifacts ($manifest_scenario_count >= $gherkin_scenario_count)"
    fi

    if [[ "$manifest_test_type_count" -lt "$gherkin_scenario_count" ]]; then
      fail "scenario-manifest.json is missing requiredTestType entries for one or more scenarios (Gate G057)"
    else
      pass "scenario-manifest.json records required live test types"
    fi

    if [[ "$manifest_linked_test_count" -eq 0 ]]; then
      fail "scenario-manifest.json is missing linkedTests entries (Gate G057)"
    else
      pass "scenario-manifest.json records linkedTests"
    fi

    if [[ "$manifest_evidence_count" -eq 0 ]]; then
      fail "scenario-manifest.json is missing evidenceRefs entries (Gate G057)"
    else
      pass "scenario-manifest.json records evidenceRefs"
    fi
  else
    fail "Resolved scopes define Gherkin scenarios but scenario-manifest.json is missing (Gate G057)"
  fi
else
  info "No Gherkin scenarios found in resolved scope artifacts — scenario manifest check skipped"
fi
echo ""

# =============================================================================
# CHECK 3D: Lockdown and regression contract protection (G058/G059)
# =============================================================================
echo "--- Check 3D: Lockdown And Regression Contracts (G058/G059) ---"
# Control-plane policy activation: anchor the regression-immutability reasoning to
# the SST by resolving it through the snapshot -> SST config -> framework-default
# chain (the lockdown/regression triggers below still key off scenario-manifest
# content; this only surfaces the effective default so it is no longer inert).
cp_regression_immutability="$(resolve_effective_policy "$state_file" regression immutability protected-scenarios "$guard_repo_root")"
info "[G058/G059] Effective regression immutability policy: ${cp_regression_immutability} (resolved via policySnapshot -> SST config -> framework default)"
locked_scenario_count=0
changed_contract_count=0
if [[ -f "$scenario_manifest_file" ]]; then
  locked_scenario_count="$(grep -cE '"lockdown"[[:space:]]*:[[:space:]]*true' "$scenario_manifest_file" || true)"
  changed_contract_count="$(grep -cE '"changeType"[[:space:]]*:[[:space:]]*"(changed|replacement|removed)"' "$scenario_manifest_file" || true)"
  regression_required_count="$(grep -cE '"regressionRequired"[[:space:]]*:[[:space:]]*true' "$scenario_manifest_file" || true)"

  if [[ "$regression_required_count" -gt 0 ]]; then
    pass "scenario-manifest.json marks $regression_required_count regression-protected scenario contract(s)"
  else
    info "No regression-protected scenarios marked in scenario-manifest.json"
  fi

  if [[ "$locked_scenario_count" -gt 0 && "$changed_contract_count" -gt 0 ]]; then
    if [[ -f "$lockdown_approvals_file" ]]; then
      pass "Lockdown approvals artifact exists: $(relative_artifact_path "$lockdown_approvals_file")"
    else
      fail "Locked scenario changes require lockdown-approvals.json (Gate G058)"
    fi

    if [[ -f "$invalidation_ledger_file" ]]; then
      pass "Invalidation ledger exists: $(relative_artifact_path "$invalidation_ledger_file")"
    else
      fail "Locked scenario changes require invalidation-ledger.json (Gate G058)"
    fi

    if [[ -f "$lockdown_approvals_file" ]]; then
      if grep -qE '"approvedVia"[[:space:]]*:[[:space:]]*"bubbles\.grill"' "$lockdown_approvals_file"; then
        pass "Lockdown approval was captured through bubbles.grill"
      else
        fail "lockdown-approvals.json is missing approvedVia=bubbles.grill (Gate G058)"
      fi
    fi

    if [[ -f "$invalidation_ledger_file" ]]; then
      if grep -qE '"invalidatedBy"[[:space:]]*:[[:space:]]*"bubbles\.validate"' "$invalidation_ledger_file"; then
        pass "Invalidation ledger records validate-owned invalidation"
      else
        fail "invalidation-ledger.json is missing invalidatedBy=bubbles.validate (Gate G058/G059)"
      fi
    fi
  else
    info "No locked scenario replacements detected — lockdown approval and invalidation artifacts not required"
  fi
else
  info "Scenario manifest not present — lockdown/regression contract checks depend on Gate G057"
fi
echo ""

# =============================================================================
# CHECK 3E: Scenario-first TDD evidence (Gate G060)
# =============================================================================
echo "--- Check 3E: Scenario-first TDD Evidence (Gate G060) ---"
# Layer 1 (control-plane policy activation): resolve the effective TDD mode from
# the per-spec policySnapshot, then the repo SST config defaults, then the
# framework default (scenario-first). Before this fix the mode was read ONLY from
# policySnapshot.tdd.mode, so a missing snapshot (empirically ~93% of downstream
# specs) left the SST default INERT and this gate silently skipped.
effective_tdd_mode="$(resolve_effective_policy "$state_file" tdd mode scenario-first "$guard_repo_root")"
effective_tdd_source="$(resolve_effective_policy_source "$state_file" tdd mode scenario-first "$guard_repo_root")"

if [[ -z "$effective_tdd_mode" && ( "$state_workflow_mode" == "bugfix-fastlane" || "$state_workflow_mode" == "chaos-hardening" ) ]]; then
  effective_tdd_mode="scenario-first"
  effective_tdd_source="workflow-forced"
fi

# Per-packet exemption support (per upstream fix proposal — artifact-only fastlanes)
# Read policySnapshot.tdd.exempt + exemptReason from state.json.
tdd_exempt="$({
  python3 -c "
import json
try:
    with open('$state_file') as f:
        data = json.load(f)
    tdd = (data.get('policySnapshot', {}) or {}).get('tdd', {}) or {}
    print('true' if tdd.get('exempt') is True else 'false')
except Exception:
    print('false')
" 2>/dev/null
} || echo "false")"

tdd_exempt_reason="$({
  python3 -c "
import json
try:
    with open('$state_file') as f:
        data = json.load(f)
    tdd = (data.get('policySnapshot', {}) or {}).get('tdd', {}) or {}
    r = tdd.get('exemptReason', '') or ''
    print(r.strip())
except Exception:
    print('')
" 2>/dev/null
} || echo "")"

# Eligible modes for opt-in exemption (artifact-only fastlanes + always-exempt docs/reconcile)
tdd_exemption_eligible_modes="docs-only reconcile-to-doc validate-to-doc gaps-to-doc devops-to-doc bugfix-fastlane chaos-hardening stabilize-to-doc audit-to-doc"
tdd_forbidden_reasons="n/a none exempt no tests skip skipped tbd todo"

if [[ "$effective_tdd_mode" == "scenario-first" ]]; then
  if [[ "$tdd_exempt" == "true" ]]; then
    # Validate exemption: mode eligible, reason present, reason substantive
    mode_eligible="false"
    for m in $tdd_exemption_eligible_modes; do
      if [[ "$state_workflow_mode" == "$m" ]]; then
        mode_eligible="true"
        break
      fi
    done

    if [[ "$mode_eligible" != "true" ]]; then
      fail "policySnapshot.tdd.exempt=true is not allowed for workflow mode '$state_workflow_mode' — exemption only permitted for: $tdd_exemption_eligible_modes (Gate G060)"
    elif [[ -z "$tdd_exempt_reason" ]]; then
      fail "policySnapshot.tdd.exempt=true requires a non-empty exemptReason (Gate G060)"
    elif [[ "${#tdd_exempt_reason}" -lt 20 ]]; then
      fail "policySnapshot.tdd.exemptReason must be at least 20 characters describing why no runtime test surface exists (Gate G060). Got: '$tdd_exempt_reason'"
    else
      # Reject stop-word reasons
      reason_lc="$(echo "$tdd_exempt_reason" | tr '[:upper:]' '[:lower:]' | tr -d '[:punct:]' | xargs)"
      is_stop_word="false"
      for sw in $tdd_forbidden_reasons; do
        if [[ "$reason_lc" == "$sw" ]]; then
          is_stop_word="true"
          break
        fi
      done
      if [[ "$is_stop_word" == "true" ]]; then
        fail "policySnapshot.tdd.exemptReason is a stop-word ('$tdd_exempt_reason') — provide a substantive explanation (Gate G060)"
      else
        pass "Scenario-first TDD exempted under mode '$state_workflow_mode' — INFO[G060-EXEMPT] reason: $tdd_exempt_reason"
      fi
    fi
  else
    # Layer 2 (evidence integrity): require a real RED->GREEN ordering, not a
    # keyword rubber-stamp. The previous grep passed merely because a report or
    # template contained the word "tdd"/"scenario-first" — proving nothing about
    # actual red-before-green ordering. detect_red_green_ordering passes ONLY when
    # a failing-proof (RED) marker precedes a passing-proof (GREEN) marker in the
    # SAME report.
    if detect_red_green_ordering ${scope_files[@]+"${scope_files[@]}"} ${report_files[@]+"${report_files[@]}"}; then
      pass "Scenario-first TDD red→green ordering is recorded in the scope/report artifacts (mode source: ${effective_tdd_source:-framework-default})"
    elif policy_spec_grandfathered "$state_file" "$control_plane_policy_cutoff"; then
      # Grandfather clause: a historical spec (createdAt before the cutoff, or
      # missing) that never carried a policySnapshot is NOT retro-broken by the
      # newly-activated enforcement — downgrade to INFO instead of a blocking fail.
      info "[G060-GRANDFATHERED] Effective TDD mode is scenario-first (source: ${effective_tdd_source:-repo-default}) but this spec predates the ${control_plane_policy_cutoff} activation cutoff and carries no policySnapshot — red→green ordering not enforced retroactively (new specs and snapshot-bearing specs get full enforcement)"
    else
      fail "Effective TDD mode is scenario-first (source: ${effective_tdd_source:-repo-default}) but no RED→GREEN ordering was found in the scope/report artifacts — a failing-proof (red) marker MUST appear on an earlier line than a passing-proof (green) marker in the same report; the word 'tdd' alone is not evidence (Gate G060)"
    fi
  fi
else
  info "Effective TDD mode is '${effective_tdd_mode:-off}' — scenario-first evidence check not required"
fi
echo ""

# =============================================================================
# CHECK 3F: Transition and rework packet closure (Gate G061)
# =============================================================================
echo "--- Check 3F: Transition And Rework Packets (Gate G061) ---"
pending_transition_failures=0

# Use python to inspect transitionRequests properly: allow status=="open" entries
# ONLY when they carry routedTo + (routedToCommit|routedToSpec|routedToTicket) + productAction=="none"
# (and crossRepoFollowUp:true when routed to an external/upstream owner).
tr_analysis="$({
  python3 -c "
import json, re, sys
try:
    with open('$state_file') as f:
        data = json.load(f)
    trs = data.get('transitionRequests', []) or []
    if not isinstance(trs, list):
        trs = []
    blocking = []
    routed_open = []
    for tr in trs:
        if not isinstance(tr, dict):
            continue
        status = (tr.get('status') or '').strip()
        tr_id = tr.get('id') or tr.get('transitionRequestId') or '<unknown>'
        if status in ('', 'closed', 'resolved', 'done', 'cancelled', 'rejected'):
            continue
        if status == 'open':
            routed_to = (tr.get('routedTo') or '').strip()
            routed_commit = (tr.get('routedToCommit') or '').strip()
            routed_spec = (tr.get('routedToSpec') or '').strip()
            routed_ticket = (tr.get('routedToTicket') or '').strip()
            product_action = (tr.get('productAction') or '').strip()
            cross_repo = bool(tr.get('crossRepoFollowUp'))
            problems = []
            if not routed_to:
                problems.append('missing routedTo')
            if not (routed_commit or routed_spec or routed_ticket):
                problems.append('missing routedToCommit/Spec/Ticket')
            if product_action != 'none':
                problems.append(f'productAction is \"{product_action}\" not \"none\"')
            if routed_commit and not re.fullmatch(r'[0-9a-f]{7,40}', routed_commit):
                problems.append(f'routedToCommit not a hex SHA: {routed_commit}')
            if routed_ticket and not re.match(r'https?://', routed_ticket):
                problems.append('routedToTicket not a URL')
            looks_external = bool(re.search(r'upstream|external|bubbles\\.', routed_to, re.I))
            if looks_external and not cross_repo:
                problems.append('routed externally but crossRepoFollowUp is not true')
            if problems:
                blocking.append((tr_id, status, problems))
            else:
                routed_open.append((tr_id, routed_to))
        else:
            blocking.append((tr_id, status, ['status is not open/closed/resolved']))
    for tr_id, status, probs in blocking:
        print(f'BLOCK\\t{tr_id}\\t{status}\\t{\"; \".join(probs)}')
    for tr_id, routed_to in routed_open:
        print(f'OK\\t{tr_id}\\t{routed_to}')
except Exception as e:
    print(f'ERR\\t{e}')
" 2>/dev/null
} || true)"

if echo "$tr_analysis" | grep -q '^ERR'; then
  # Fall back to legacy check if state.json is malformed
  if grep -A6 '"transitionRequests"' "$state_file" | grep -qE '"TR-|"transitionRequestId"'; then
    fail "state.json still contains non-empty transitionRequests — validation routing is not complete (Gate G061)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "state.json transitionRequests queue is empty"
  fi
else
  if echo "$tr_analysis" | grep -q '^BLOCK'; then
    while IFS=$'\t' read -r marker tr_id status probs; do
      [[ "$marker" == "BLOCK" ]] || continue
      fail "transitionRequest $tr_id (status=$status) lacks routing fields: $probs (Gate G061)"
      pending_transition_failures=$((pending_transition_failures + 1))
    done <<< "$tr_analysis"
  fi
  if echo "$tr_analysis" | grep -q '^OK'; then
    while IFS=$'\t' read -r marker tr_id routed_to; do
      [[ "$marker" == "OK" ]] || continue
      pass "transitionRequest $tr_id is open-but-routed to '$routed_to' (Gate G061 allowance)"
    done <<< "$tr_analysis"
  fi
  if [[ -z "$tr_analysis" ]]; then
    pass "state.json transitionRequests queue is empty"
  fi
fi

rework_nonempty="$({
  python3 -c "
import json
try:
    with open('$state_file') as f:
        data = json.load(f)
    rq = data.get('reworkQueue', []) or []
    print('true' if isinstance(rq, list) and len(rq) > 0 else 'false')
except Exception:
    print('false')
" 2>/dev/null
} || echo "false")"
if [[ "$rework_nonempty" == "true" ]]; then
  fail "state.json still contains non-empty reworkQueue entries — open rework remains (Gate G061)"
  pending_transition_failures=$((pending_transition_failures + 1))
else
  pass "state.json reworkQueue is empty"
fi

if [[ -f "$transition_requests_file" ]]; then
  if grep -qE '"status"[[:space:]]*:[[:space:]]*"(pending-validation|route_required|blocked|open)"' "$transition_requests_file"; then
    fail "transition-requests.json contains unresolved transition packets (Gate G061)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "transition-requests.json contains no unresolved packets"
  fi

  if grep -qE '"evidenceRefs"[[:space:]]*:[[:space:]]*\[[[:space:]]*\]' "$transition_requests_file"; then
    fail "transition-requests.json contains a packet without evidenceRefs (Gate G061)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "transition packets include evidenceRefs"
  fi
fi

if [[ -f "$rework_queue_file" ]]; then
  if grep -qE '"status"[[:space:]]*:[[:space:]]*"(open|pending|route_required|blocked)"' "$rework_queue_file"; then
    fail "rework-queue.json contains unresolved rework packets (Gate G061)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "rework-queue.json contains no unresolved rework packets"
  fi

  if ! grep -qE '"owner"[[:space:]]*:[[:space:]]*"bubbles\.[A-Za-z0-9.-]+"' "$rework_queue_file"; then
    fail "rework-queue.json is missing a concrete owning specialist for one or more packets (Gate G063)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "rework packets record a concrete owning specialist"
  fi

  if ! grep -qE '"reason"[[:space:]]*:[[:space:]]*"[^"]+"' "$rework_queue_file"; then
    fail "rework-queue.json is missing packet reasons (Gate G063)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "rework packets record concrete reasons"
  fi

  if ! grep -qE '"(scenarioIds|dodItems)"[[:space:]]*:[[:space:]]*\[' "$rework_queue_file"; then
    fail "rework-queue.json is missing scenarioIds or dodItems references (Gate G063)"
    pending_transition_failures=$((pending_transition_failures + 1))
  else
    pass "rework packets record scenario or DoD references"
  fi
fi

if [[ "$pending_transition_failures" -eq 0 ]]; then
  pass "Transition and rework routing is closed"
fi
echo ""
