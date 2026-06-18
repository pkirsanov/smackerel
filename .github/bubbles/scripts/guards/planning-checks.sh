# shellcheck shell=bash
# shellcheck disable=SC2154  # sourced fragment: all referenced vars are set in state-transition-guard.sh's scope before sourcing
# =============================================================================
# guards/planning-checks.sh  (M4 guard split)
# =============================================================================
# Checks 8A-8D: scenario-specific regression E2E coverage planning, consumer
# trace planning (G043), shared-infrastructure blast-radius planning (G067),
# and change-boundary containment (G069). Sourced by state-transition-guard.sh
# in the same shell scope: pass/fail/warn/info, the failures/warnings counters,
# $feature_dir, $scope_files[], $scope_analysis_files[], and
# scope_analysis_label are all in scope exactly as before extraction. Behavior
# is byte-identical to the previous inline blocks.
# =============================================================================

# CHECK 8A: Scenario-specific regression E2E coverage is planned
# =============================================================================
echo "--- Check 8A: Scenario-Specific Regression E2E Coverage ---"
missing_regression_e2e=0

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue
  scope_label="$(scope_analysis_label "$scope_index")"

  # v4.1.0: Scope-Kind opt-out. The default kind is `runtime-behavior`
  # which enforces the full 3 E2E DoD/Test-Plan rows. Other kinds
  # (contract-only, deploy-pointer, ci-config, docs-only, bootstrap)
  # legitimately do not produce live-runtime E2E evidence at ship time
  # and are exempted here. Authors opt in by adding either:
  #   `Scope-Kind: <kind>`         (plain markdown line near top)
  #   `**Scope-Kind:** <kind>`     (bold-key form — most common in templates)
  #   `**Scope-Kind**: <kind>`     (bold-then-colon form)
  # Default behavior (no header) = runtime-behavior = full E2E enforcement
  # (v4.0.x compatible).
  scope_kind="$(head -n 80 "$scope_path" \
    | grep -iE '^(\*\*)?Scope-Kind(\*\*)?[[:space:]]*:[[:space:]]*(\*\*)?[[:space:]]*' \
    | head -n 1 \
    | sed -E 's/^(\*\*)?Scope-Kind(\*\*)?[[:space:]]*:[[:space:]]*(\*\*)?[[:space:]]*//I' \
    | sed -E 's/[[:space:]]*(\*\*)?[[:space:]]*$//' \
    | sed -E 's/[[:space:]]+$//' \
    | tr '[:upper:]' '[:lower:]' || true)"
  if [[ -z "$scope_kind" ]]; then
    scope_kind="runtime-behavior"
  fi
  case "$scope_kind" in
    contract-only|deploy-pointer|ci-config|docs-only|bootstrap)
      info "Scope-Kind '$scope_kind' for $scope_label — E2E regression rows not required (v4.1.0 scopeKinds opt-out)"
      continue
      ;;
    runtime-behavior|"")
      # Fall through to full E2E enforcement (default).
      ;;
    *)
      warn "Scope-Kind '$scope_kind' for $scope_label is not a recognised v4.1.0 scopeKinds entry — enforcing default runtime-behavior E2E rules"
      ;;
  esac

  if grep -Eiq '^\- \[(x| )\] Scenario-specific E2E regression tests? for (EVERY|every) new/changed/fixed behavior' "$scope_path"; then
    pass "Scope DoD includes scenario-specific regression E2E requirement: $scope_label"
  else
    fail "Scope is missing DoD item for scenario-specific regression E2E coverage: $scope_label"
    missing_regression_e2e=$((missing_regression_e2e + 1))
  fi

  if grep -Eiq '^\- \[(x| )\] Broader E2E regression suite passes' "$scope_path"; then
    pass "Scope DoD includes broader E2E regression suite requirement: $scope_label"
  else
    fail "Scope is missing DoD item for broader E2E regression suite coverage: $scope_label"
    missing_regression_e2e=$((missing_regression_e2e + 1))
  fi

  if grep -Eiq '^\|.*Regression E2E' "$scope_path" || grep -Eiq '^\|.*e2e-(api|ui).*(\||`).*Regression:' "$scope_path"; then
    pass "Scope Test Plan includes explicit regression E2E row(s): $scope_label"
  else
    fail "Scope Test Plan is missing explicit scenario-specific regression E2E row(s): $scope_label"
    missing_regression_e2e=$((missing_regression_e2e + 1))
  fi
done

if [[ "$missing_regression_e2e" -gt 0 ]]; then
  fail "$missing_regression_e2e regression E2E planning requirement(s) missing — every runtime-behavior feature/fix/change needs persistent scenario-specific E2E regression coverage"
fi
echo ""

# CHECK 8B: Consumer trace planning for renames/removals
# =============================================================================
echo "--- Check 8B: Consumer Trace Planning For Renames/Removals ---"
rename_scope_hits=0
missing_consumer_trace=0

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue
  scope_label="$(scope_analysis_label "$scope_index")"

  if grep -Eiq '\b(rename|renamed|remove|removed|move|moved|replace|replaced|deprecat(e|ed)|migration)\b.*\b(route|path|endpoint|contract|api|url|slug|identifier|symbol|link|breadcrumb|navigation|redirect)\b|\b(route|path|endpoint|contract|api|url|slug|identifier|symbol|link|breadcrumb|navigation|redirect)\b.*\b(rename|renamed|remove|removed|move|moved|replace|replaced|deprecat(e|ed)|migration)\b' "$scope_path"; then
    rename_scope_hits=$((rename_scope_hits + 1))

    if grep -Eiq 'Consumer Impact Sweep' "$scope_path"; then
      pass "Scope includes Consumer Impact Sweep section: $scope_label"
    else
      fail "Scope renames/removes interfaces but has no Consumer Impact Sweep section: $scope_label"
      missing_consumer_trace=$((missing_consumer_trace + 1))
    fi

    if grep -Eiq '^\- \[(x| )\] .*consumer impact sweep.*zero stale first-party references remain' "$scope_path"; then
      pass "Scope DoD includes consumer impact sweep completion item: $scope_label"
    else
      fail "Scope renames/removes interfaces but is missing DoD item for consumer impact sweep: $scope_label"
      missing_consumer_trace=$((missing_consumer_trace + 1))
    fi

    if grep -Eiq 'navigation|breadcrumb|redirect|API client|generated client|deep link|stale-reference' "$scope_path"; then
      pass "Scope lists affected consumer surfaces for rename/removal work: $scope_label"
    else
      fail "Scope renames/removes interfaces but does not enumerate affected consumer surfaces: $scope_label"
      missing_consumer_trace=$((missing_consumer_trace + 1))
    fi
  fi
done

if [[ "$rename_scope_hits" -eq 0 ]]; then
  info "No rename/removal scope patterns detected — consumer trace planning check not applicable"
elif [[ "$missing_consumer_trace" -gt 0 ]]; then
  fail "$missing_consumer_trace consumer-trace planning requirement(s) missing for rename/removal scope(s)"
fi
echo ""

# CHECK 8C: Shared infrastructure blast-radius planning
# =============================================================================
echo "--- Check 8C: Shared Infrastructure Blast-Radius Planning ---"
shared_scope_hits=0
missing_shared_infra_requirements=0

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue
  scope_label="$(scope_analysis_label "$scope_index")"

  # BUG-007: the middle alternation's second arm previously allowed the generic
  # words (setup|contract|flow), so benign prose like a Test Plan row describing a
  # "regression session" that re-runs a "user flow" matched (session + flow) and
  # the scope was wrongly required to carry a Shared Infrastructure Impact Sweep.
  # Require a real test-infrastructure noun (fixture|fixtures|harness|bootstrap) to
  # co-occur with the infra subject. The shared/global qualifier arm and the
  # specific multi-word-phrase arm (which signal GENUINE shared infra) are
  # unchanged, so real shared fixture/bootstrap work is still caught.
  if grep -Eiq '\b(shared|global|common|core)\b.*\b(fixture|fixtures|harness|setup|bootstrap|test helper|test infrastructure)\b|\b(auth|login|session|password reset|token refresh|tenant context|role detection|storage injection|init script|addinitscript)\b.*\b(fixture|fixtures|harness|bootstrap)\b|\b(auth fixture|login fixture|global setup|playwright setup|bootstrap helper|shared test helper)\b' "$scope_path"; then
    shared_scope_hits=$((shared_scope_hits + 1))

    if grep -Eiq 'Shared Infrastructure Impact Sweep' "$scope_path"; then
      pass "Scope includes Shared Infrastructure Impact Sweep section: $scope_label"
    else
      fail "Scope touches shared fixture/bootstrap infrastructure but has no Shared Infrastructure Impact Sweep section: $scope_label"
      missing_shared_infra_requirements=$((missing_shared_infra_requirements + 1))
    fi

    if grep -Eiq '^\- \[(x| )\] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns' "$scope_path"; then
      pass "Scope DoD includes shared-infrastructure canary item: $scope_label"
    else
      fail "Scope touches shared fixture/bootstrap infrastructure but is missing the canary DoD item: $scope_label"
      missing_shared_infra_requirements=$((missing_shared_infra_requirements + 1))
    fi

    if grep -Eiq '^\- \[(x| )\] Rollback or restore path for shared infrastructure changes is documented and verified' "$scope_path"; then
      pass "Scope DoD includes rollback/restore item for shared infrastructure: $scope_label"
    else
      fail "Scope touches shared fixture/bootstrap infrastructure but is missing the rollback/restore DoD item: $scope_label"
      missing_shared_infra_requirements=$((missing_shared_infra_requirements + 1))
    fi

    if grep -Eiq '^\|.*Canary:' "$scope_path" || grep -Eiq '^\|.*Fixture Canary' "$scope_path"; then
      pass "Scope Test Plan includes explicit canary row(s): $scope_label"
    else
      fail "Scope touches shared fixture/bootstrap infrastructure but lacks an explicit canary Test Plan row: $scope_label"
      missing_shared_infra_requirements=$((missing_shared_infra_requirements + 1))
    fi

    if grep -Eiq 'ordering|timing|storage|session|context|role|bootstrap contract|downstream contract|blast radius' "$scope_path"; then
      pass "Scope enumerates downstream contract surfaces for shared infrastructure work: $scope_label"
    else
      fail "Scope touches shared fixture/bootstrap infrastructure but does not enumerate downstream contract surfaces: $scope_label"
      missing_shared_infra_requirements=$((missing_shared_infra_requirements + 1))
    fi
  fi
done

if [[ "$shared_scope_hits" -eq 0 ]]; then
  info "No shared fixture/bootstrap scope patterns detected — blast-radius planning check not applicable"
elif [[ "$missing_shared_infra_requirements" -gt 0 ]]; then
  fail "$missing_shared_infra_requirements shared-infrastructure planning requirement(s) missing"
fi
echo ""

# CHECK 8D: Change boundary containment for risky refactors
# =============================================================================
echo "--- Check 8D: Change Boundary Containment ---"
boundary_scope_hits=0
missing_change_boundary=0

for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  if grep -Eiq '\b(refactor|refactoring|simplify|simplification|cleanup|repair|hotspot)\b|Shared Infrastructure Impact Sweep' "$scope_path"; then
    boundary_scope_hits=$((boundary_scope_hits + 1))

    if grep -Eiq 'Change Boundary' "$scope_path"; then
      pass "Scope includes Change Boundary section: ${scope_path#$feature_dir/}"
    else
      fail "Scope is a refactor/repair but has no Change Boundary section: ${scope_path#$feature_dir/}"
      missing_change_boundary=$((missing_change_boundary + 1))
    fi

    if grep -Eiq '^\- \[(x| )\] Change Boundary is respected and zero excluded file families were changed' "$scope_path"; then
      pass "Scope DoD includes change-boundary containment item: ${scope_path#$feature_dir/}"
    else
      fail "Scope is a refactor/repair but is missing the change-boundary DoD item: ${scope_path#$feature_dir/}"
      missing_change_boundary=$((missing_change_boundary + 1))
    fi

    if grep -Eiq 'Allowed file families|Included file families|Excluded surfaces|Untouched surfaces' "$scope_path"; then
      pass "Scope enumerates allowed and excluded surfaces for the change boundary: ${scope_path#$feature_dir/}"
    else
      fail "Scope is a refactor/repair but does not enumerate allowed and excluded surfaces: ${scope_path#$feature_dir/}"
      missing_change_boundary=$((missing_change_boundary + 1))
    fi
  fi
done

if [[ "$boundary_scope_hits" -eq 0 ]]; then
  info "No refactor/repair scope patterns detected — change-boundary check not applicable"
elif [[ "$missing_change_boundary" -gt 0 ]]; then
  fail "$missing_change_boundary change-boundary containment requirement(s) missing"
fi
echo ""
