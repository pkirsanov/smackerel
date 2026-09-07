# shellcheck shell=bash
# shellcheck disable=SC2154  # sourced fragment: all referenced vars are set in state-transition-guard.sh's scope before sourcing
# shellcheck disable=SC2034  # CHECK8B_* result-contract vars are read by state-transition-guard.sh and its selftest in the same shell scope, not within this fragment
# =============================================================================
# guards/planning-checks.sh  (M4 guard split)
# =============================================================================
# Checks 8A-8D: scenario-specific regression E2E coverage planning, consumer
# trace planning (G043), shared-infrastructure blast-radius planning (G067),
# and change-boundary containment (G069). Sourced by state-transition-guard.sh
# in the same shell scope: pass/fail/warn/info, the failures/warnings counters,
# $feature_dir, $scope_files[], $scope_analysis_files[], and
# scope_analysis_label are all in scope exactly as before extraction. Each
# check owns its production behavior here while preserving the sourced boundary.
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

# CHECK 8B: Consumer trace planning for explicit interface mutations
# =============================================================================

# BEGIN CHECK8B FINITE CLASSIFIER
# BUG-032 / SCN-032-013: this private, sourceable block is the single grammar
# used by production and persistent regression tests. It accepts one physical
# line, examines at most 128 tokens and eight mutation candidates, and exposes
# stable CHECK8B_* outputs. Semantic uncertainty is a result, never an exemption.
_check8b_mutation_verb() {
  case "$1" in
    rename|renames|renamed|renaming) _CHECK8B_WORD_RESULT="rename" ;;
    remove|removes|removed|removing) _CHECK8B_WORD_RESULT="remove" ;;
    move|moves|moved|moving) _CHECK8B_WORD_RESULT="move" ;;
    deprecate|deprecates|deprecated|deprecating) _CHECK8B_WORD_RESULT="deprecate" ;;
    *) _CHECK8B_WORD_RESULT=""
      return 1
      ;;
  esac
  return 0
}

_check8b_is_surface() {
  case "$1" in
    route|path|endpoint|contract|api|url|slug|identifier|symbol|link|breadcrumb|navigation|redirect) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_modifier() {
  case "$1" in
    public|private|legacy|old|new|external|internal|canonical|consumer|client|api|ui|deep|navigation) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_determiner() {
  case "$1" in
    a|an|the|this|that) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_barrier() {
  case "$1" in
    from|to|into|onto|as|before|after|while|when|using|via|through|without|unless|during|because|and|or|but|yet|however|although) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_auxiliary() {
  case "$1" in
    is|are|was|were|be|been|being|gets|got) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_state_auxiliary() {
  case "$1" in
    is|are|was|were|be|been|being|gets|got|remain|remains|remained) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_preservation_verb() {
  case "$1" in
    preserve|preserves|preserved|preserving) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_preserved_state() {
  case "$1" in
    unchanged|preserved|stable) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_use_verb() {
  case "$1" in
    invoke|invokes|invoked|invoking|use|uses|used|using|via|through) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_is_change_verb() {
  case "$1" in
    change|changes|changed|changing) return 0 ;;
    *) return 1 ;;
  esac
}

_check8b_csv_add() {
  local value="$1"
  local current="$2"

  _CHECK8B_CSV_RESULT="$current"
  [[ -n "$value" ]] || return 0
  case ",$current," in
    *",$value,"*) return 0 ;;
  esac
  if [[ -n "$current" ]]; then
    _CHECK8B_CSV_RESULT="$current,$value"
  else
    _CHECK8B_CSV_RESULT="$value"
  fi
}

_check8b_is_surface_head_at() {
  local index="$1"
  local token_count="${#_CHECK8B_TOKENS[@]}"
  local token=""
  local next_index=$((index + 1))

  _CHECK8B_SURFACE_RESULT=""
  _CHECK8B_SURFACE_INDEX=-1
  [[ "$index" -ge 0 && "$index" -lt "$token_count" ]] || return 1
  token="${_CHECK8B_TOKENS[$index]}"
  _check8b_is_surface "$token" || return 1
  if [[ "$token" == "api" || "$token" == "navigation" ]] \
    && [[ "$next_index" -lt "$token_count" ]] \
    && [[ "${_CHECK8B_CLAUSE_IDS[$next_index]}" -eq "${_CHECK8B_CLAUSE_IDS[$index]}" ]] \
    && _check8b_is_surface "${_CHECK8B_TOKENS[$next_index]}"; then
    return 1
  fi
  _CHECK8B_SURFACE_RESULT="$token"
  _CHECK8B_SURFACE_INDEX="$index"
  return 0
}

_check8b_surface_from() {
  local start_index="$1"
  local requested_end="$2"
  local token_count="${#_CHECK8B_TOKENS[@]}"
  local index="$start_index"
  local end_index="$requested_end"
  local modifier_count=0
  local token=""
  local next_index=0
  local clause_id=-1
  local phrase=""

  _CHECK8B_SURFACE_RESULT=""
  _CHECK8B_SURFACE_INDEX=-1
  _CHECK8B_REFERENCE_RESULT=""
  _CHECK8B_SURFACE_START_INDEX=-1
  _CHECK8B_SURFACE_END_INDEX=-1
  _CHECK8B_SURFACE_PHRASE=""
  [[ "$index" -ge 0 && "$index" -lt "$token_count" ]] || return 1
  if [[ "$end_index" -ge "$token_count" ]]; then
    end_index=$((token_count - 1))
  fi
  if [[ "$end_index" -gt $((start_index + 7)) ]]; then
    end_index=$((start_index + 7))
  fi
  clause_id="${_CHECK8B_CLAUSE_IDS[$index]}"

  token="${_CHECK8B_TOKENS[$index]}"
  if _check8b_is_determiner "$token"; then
    if [[ "$token" == "this" || "$token" == "that" ]]; then
      _CHECK8B_REFERENCE_RESULT="$token"
    fi
    index=$((index + 1))
  fi
  _CHECK8B_SURFACE_START_INDEX="$index"

  while [[ "$index" -le "$end_index" ]]; do
    [[ "${_CHECK8B_CLAUSE_IDS[$index]}" -eq "$clause_id" ]] || break
    token="${_CHECK8B_TOKENS[$index]}"
    _check8b_is_barrier "$token" && return 1
    if _check8b_is_surface "$token"; then
      phrase="${phrase}${phrase:+ }$token"
      _CHECK8B_SURFACE_RESULT="$token"
      _CHECK8B_SURFACE_INDEX="$index"
      _CHECK8B_SURFACE_END_INDEX="$index"
      next_index=$((index + 1))
      if [[ "$next_index" -gt "$end_index" ]] \
        || [[ "${_CHECK8B_CLAUSE_IDS[$next_index]}" -ne "$clause_id" ]] \
        || ! _check8b_is_surface "${_CHECK8B_TOKENS[$next_index]}"; then
        break
      fi
      index=$((index + 1))
      continue
    fi
    _check8b_is_modifier "$token" || break
    modifier_count=$((modifier_count + 1))
    [[ "$modifier_count" -le 4 ]] || return 1
    phrase="${phrase}${phrase:+ }$token"
    index=$((index + 1))
  done

  [[ -n "$_CHECK8B_SURFACE_RESULT" ]] || return 1
  _CHECK8B_SURFACE_PHRASE="$phrase"
  return 0
}

_check8b_passive_surface() {
  local mutation_index="$1"
  local token_count="${#_CHECK8B_TOKENS[@]}"
  local index=$((mutation_index - 1))
  local mutation_clause="${_CHECK8B_CLAUSE_IDS[$mutation_index]}"
  local lower_bound=$((mutation_index - 8))
  local subject_start=-1
  local subject_end=-1
  local target_start=-1
  local token=""
  local target_phrase=""
  local tail_phrase=""
  local next_index=0
  local scan_index=0

  _CHECK8B_SURFACE_RESULT=""
  _CHECK8B_SURFACE_INDEX=-1
  _CHECK8B_NEGATED_RESULT=0
  _CHECK8B_PASSIVE_RELATION=""
  _CHECK8B_PASSIVE_TARGET=""
  _CHECK8B_PASSIVE_UNRESOLVED=""
  _CHECK8B_PASSIVE_TAIL_SURFACE=""
  [[ "$index" -ge 0 ]] || return 1
  token="${_CHECK8B_TOKENS[$index]}"
  if [[ "$token" == "not" || "$token" == "never" ]]; then
    _CHECK8B_NEGATED_RESULT=1
    index=$((index - 1))
    [[ "$index" -ge 0 ]] || return 1
    token="${_CHECK8B_TOKENS[$index]}"
  fi
  _check8b_is_auxiliary "$token" || return 1
  if [[ "$token" == "be" && "$index" -ge 1 ]] \
    && { [[ "${_CHECK8B_TOKENS[$((index - 1))]}" == "not" ]] \
      || [[ "${_CHECK8B_TOKENS[$((index - 1))]}" == "never" ]]; }; then
    _CHECK8B_NEGATED_RESULT=1
    index=$((index - 1))
  fi
  if [[ "$token" == "be" && "$index" -ge 1 ]]; then
    case "${_CHECK8B_TOKENS[$((index - 1))]}" in
      will|shall|must|should|may|might|can|could) index=$((index - 1)) ;;
    esac
  fi
  subject_end=$((index - 1))
  [[ "$lower_bound" -ge 0 ]] || lower_bound=0
  [[ "$subject_end" -ge "$lower_bound" ]] || return 1
  [[ "${_CHECK8B_CLAUSE_IDS[$subject_end]}" -eq "$mutation_clause" ]] || return 1

  subject_start="$subject_end"
  for ((scan_index = subject_end; scan_index >= lower_bound; scan_index--)); do
    [[ "${_CHECK8B_CLAUSE_IDS[$scan_index]}" -eq "$mutation_clause" ]] || break
    token="${_CHECK8B_TOKENS[$scan_index]}"
    case "$token" in
      while|before|after|when|using|via|through|unless|during|because|and|or|but|yet|however|although|from|to|into|onto|as)
        break
        ;;
    esac
    if _check8b_mutation_verb "$token"; then
      break
    fi
    subject_start="$scan_index"
  done
  [[ "$subject_start" -ge "$lower_bound" ]] || return 1

  target_start="$subject_start"
  if [[ "${_CHECK8B_TOKENS[$target_start]}" == "no" ]]; then
    _CHECK8B_NEGATED_RESULT=1
    target_start=$((target_start + 1))
  fi

  if _check8b_surface_from "$target_start" "$subject_end"; then
    _CHECK8B_PASSIVE_TARGET="$_CHECK8B_SURFACE_RESULT"
    _CHECK8B_PASSIVE_TAIL_SURFACE="$_CHECK8B_SURFACE_RESULT"
    next_index=$((_CHECK8B_SURFACE_END_INDEX + 1))
    if [[ "$next_index" -le "$subject_end" ]] \
      && { [[ "${_CHECK8B_TOKENS[$next_index]}" == "identity" ]] \
        || [[ "${_CHECK8B_TOKENS[$next_index]}" == "identities" ]]; }; then
      next_index=$((next_index + 1))
    fi
    if [[ "$next_index" -le "$subject_end" ]] \
      && [[ "${_CHECK8B_TOKENS[$next_index]}" == "without" ]]; then
      next_index=$((next_index + 1))
      while [[ "$next_index" -le "$subject_end" ]]; do
        token="${_CHECK8B_TOKENS[$next_index]}"
        if _check8b_is_surface "$token" || _check8b_mutation_verb "$token"; then
          break
        fi
        next_index=$((next_index + 1))
      done
    fi
    if [[ "$next_index" -le "$subject_end" ]]; then
      tail_phrase="$_CHECK8B_SURFACE_RESULT"
      for ((scan_index = _CHECK8B_SURFACE_END_INDEX + 1; scan_index <= subject_end; scan_index++)); do
        token="${_CHECK8B_TOKENS[$scan_index]}"
        tail_phrase="$tail_phrase $token"
      done
      _CHECK8B_PASSIVE_TARGET="$tail_phrase"
      _CHECK8B_PASSIVE_UNRESOLVED="$tail_phrase"
      _CHECK8B_PASSIVE_RELATION="tail"
    elif [[ "$_CHECK8B_NEGATED_RESULT" -eq 1 ]]; then
      _CHECK8B_PASSIVE_RELATION="negated"
    else
      _CHECK8B_PASSIVE_RELATION="direct"
    fi
    return 0
  fi

  if [[ "$target_start" -le "$subject_end" ]] \
    && _check8b_is_determiner "${_CHECK8B_TOKENS[$target_start]}"; then
    target_start=$((target_start + 1))
  fi
  for ((scan_index = target_start; scan_index <= subject_end && scan_index < token_count; scan_index++)); do
    token="${_CHECK8B_TOKENS[$scan_index]}"
    target_phrase="${target_phrase}${target_phrase:+ }$token"
  done
  [[ -n "$target_phrase" ]] || return 1
  _CHECK8B_PASSIVE_TARGET="$target_phrase"
  _CHECK8B_PASSIVE_RELATION="other"
  return 0
}

_check8b_mutation_is_negated() {
  local mutation_index="$1"
  local previous_index=$((mutation_index - 1))
  local previous_token=""

  _CHECK8B_NEGATED_RESULT=0
  [[ "$previous_index" -ge 0 ]] || return 1
  previous_token="${_CHECK8B_TOKENS[$previous_index]}"
  case "$previous_token" in
    not|never|without) :
      _CHECK8B_NEGATED_RESULT=1
      return 0
      ;;
  esac
  return 1
}

_check8b_named_surface_before() {
  local before_index="$1"
  local surface="$2"
  local lower_bound=$((before_index - 8))
  local index=0
  local clause_id=-1

  _CHECK8B_REFERENCE_RESULT=""
  [[ "$lower_bound" -ge 0 ]] || lower_bound=0
  [[ "$before_index" -gt 0 ]] || return 1
  clause_id="${_CHECK8B_CLAUSE_IDS[$before_index]}"
  for ((index = before_index - 1; index >= lower_bound; index--)); do
    [[ "${_CHECK8B_CLAUSE_IDS[$index]}" -eq "$clause_id" ]] || break
    if [[ "${_CHECK8B_TOKENS[$index]}" == "$surface" ]] \
      && _check8b_is_surface_head_at "$index"; then
      _CHECK8B_REFERENCE_RESULT="$surface"
      return 0
    fi
  done
  return 1
}

_check8b_target_after() {
  local mutation_index="$1"
  local verb="$2"
  local token_count="${#_CHECK8B_TOKENS[@]}"
  local end_index=$((mutation_index + 8))
  local index=$((mutation_index + 1))
  local mutation_clause="${_CHECK8B_CLAUSE_IDS[$mutation_index]}"
  local token=""
  local next_index=0
  local tail_index=0
  local target_phrase=""
  local tail_phrase=""

  _CHECK8B_TARGET_RESULT=""
  _CHECK8B_RELATION_KIND="irrelevant"
  _CHECK8B_UNRESOLVED_RESULT=""
  _CHECK8B_TAIL_SURFACE=""
  _CHECK8B_NEGATED_RESULT=0
  if [[ "$end_index" -ge "$token_count" ]]; then
    end_index=$((token_count - 1))
  fi

  if _check8b_passive_surface "$mutation_index"; then
    _CHECK8B_TARGET_RESULT="$_CHECK8B_PASSIVE_TARGET"
    _CHECK8B_RELATION_KIND="$_CHECK8B_PASSIVE_RELATION"
    _CHECK8B_UNRESOLVED_RESULT="$_CHECK8B_PASSIVE_UNRESOLVED"
    _CHECK8B_TAIL_SURFACE="$_CHECK8B_PASSIVE_TAIL_SURFACE"
    return 0
  fi

  if _check8b_surface_from "$index" "$end_index"; then
    _CHECK8B_TARGET_RESULT="$_CHECK8B_SURFACE_RESULT"
    _CHECK8B_TAIL_SURFACE="$_CHECK8B_SURFACE_RESULT"
    if _check8b_mutation_is_negated "$mutation_index"; then
      _CHECK8B_RELATION_KIND="negated"
      return 0
    fi
    next_index=$((_CHECK8B_SURFACE_END_INDEX + 1))
    if [[ "$next_index" -le "$end_index" ]] \
      && [[ "${_CHECK8B_CLAUSE_IDS[$next_index]}" -eq "$mutation_clause" ]] \
      && { [[ "${_CHECK8B_TOKENS[$next_index]}" == "identity" ]] \
        || [[ "${_CHECK8B_TOKENS[$next_index]}" == "identities" ]]; }; then
      next_index=$((next_index + 1))
    fi
    if [[ "$next_index" -gt "$end_index" ]] \
      || [[ "${_CHECK8B_CLAUSE_IDS[$next_index]}" -ne "$mutation_clause" ]]; then
      _CHECK8B_RELATION_KIND="direct"
      return 0
    fi
    token="${_CHECK8B_TOKENS[$next_index]}"
    if _check8b_is_barrier "$token" || _check8b_mutation_verb "$token"; then
      _CHECK8B_RELATION_KIND="direct"
      return 0
    fi

    tail_phrase="$_CHECK8B_SURFACE_RESULT"
    for ((tail_index = next_index; tail_index <= end_index; tail_index++)); do
      [[ "${_CHECK8B_CLAUSE_IDS[$tail_index]}" -eq "$mutation_clause" ]] || break
      token="${_CHECK8B_TOKENS[$tail_index]}"
      if _check8b_is_barrier "$token" || _check8b_mutation_verb "$token"; then
        break
      fi
      tail_phrase="$tail_phrase $token"
    done
    _CHECK8B_TARGET_RESULT="$tail_phrase"
    _CHECK8B_UNRESOLVED_RESULT="$tail_phrase"
    _CHECK8B_RELATION_KIND="tail"
    return 0
  fi

  if [[ "$index" -le "$end_index" ]] \
    && _check8b_is_determiner "${_CHECK8B_TOKENS[$index]}"; then
    index=$((index + 1))
  fi
  while [[ "$index" -le "$end_index" ]]; do
    [[ "${_CHECK8B_CLAUSE_IDS[$index]}" -eq "$mutation_clause" ]] || break
    token="${_CHECK8B_TOKENS[$index]}"
    _check8b_is_barrier "$token" && break
    _check8b_mutation_verb "$token" && break
    if _check8b_surface_from "$index" "$end_index"; then
      _CHECK8B_UNRESOLVED_RESULT="$_CHECK8B_SURFACE_PHRASE"
      _CHECK8B_RELATION_KIND="unresolved"
      return 0
    fi
    target_phrase="${target_phrase}${target_phrase:+ }$token"
    index=$((index + 1))
  done
  if [[ -n "$target_phrase" ]]; then
    _CHECK8B_TARGET_RESULT="$target_phrase"
    _CHECK8B_RELATION_KIND="other"
  fi
  [[ -n "$verb" ]] || _CHECK8B_RELATION_KIND="irrelevant"
  return 0
}

check8b_classify_line() {
  local LC_ALL=C
  local line=""
  local line_byte_count=0
  local character=""
  local character_code=0
  local token=""
  local token_byte_count=0
  local max_token_byte_count=0
  local token_count=0
  local token_overflow=0
  local clause_id=0
  local index=0
  local candidate_index=0
  local candidate_count=0
  local first_verb=""
  local verb=""
  local surface=""
  local direct_surfaces=""
  local preserved_surfaces=""
  local used_surfaces=""
  local other_target=""
  local first_negated_target=""
  local tail_phrase=""
  local tail_surface=""
  local unresolved_phrase=""
  local unresolved_reason=""
  local unresolved_reference=0
  local surface_count=0
  local negated_count=0
  local conflict=0
  local remaining_pairs=""
  local direct_pair=""
  local state_index=0
  local state_lower_bound=0
  local reference_index=0
  local -a candidate_verbs=()

  CHECK8B_CLASSIFICATION="irrelevant"
  CHECK8B_VERB="none"
  CHECK8B_MUTATION_TARGET="none"
  CHECK8B_DIRECT_SURFACES="none"
  CHECK8B_PRESERVED_SURFACES="none"
  CHECK8B_REASON="none"
  CHECK8B_UNRESOLVED_PHRASE="none"
  CHECK8B_BOUNDARY="not-applicable"
  CHECK8B_LINE_BYTE_COUNT=0
  CHECK8B_TOKEN_BYTE_COUNT=0
  CHECK8B_TOKEN_COUNT=0
  CHECK8B_CANDIDATE_COUNT=0
  _CHECK8B_TOKENS=()
  _CHECK8B_CLAUSE_IDS=()
  _CHECK8B_CANDIDATE_INDEXES=()

  if [[ "$#" -ne 1 ]]; then
    CHECK8B_CLASSIFICATION="error"
    CHECK8B_MUTATION_TARGET="unavailable"
    CHECK8B_REASON="invalid-arguments"
    return 2
  fi
  line="$1"

  for ((index = 0; index <= 4096; index++)); do
    character="${line:$index:1}"
    if [[ -z "$character" ]]; then
      line_byte_count="$index"
      break
    fi
    if [[ "$index" -eq 4096 ]]; then
      CHECK8B_CLASSIFICATION="ambiguous"
      CHECK8B_MUTATION_TARGET="unresolved"
      CHECK8B_REASON="line-byte-limit"
      CHECK8B_BOUNDARY="line-bytes=4097/4096:first-overflow"
      CHECK8B_LINE_BYTE_COUNT=4097
      return 0
    fi
  done
  CHECK8B_LINE_BYTE_COUNT="$line_byte_count"

  for ((index = 0; index < line_byte_count; index++)); do
    character="${line:$index:1}"
    printf -v character_code '%d' "'$character"
    if (( (character_code >= 48 && character_code <= 57) || (character_code >= 65 && character_code <= 90) || (character_code >= 97 && character_code <= 122) )); then
      if [[ -z "$token" && "${#_CHECK8B_TOKENS[@]}" -ge 128 ]]; then
        token_overflow=1
        break
      fi
      token_byte_count=$((token_byte_count + 1))
      if [[ "$token_byte_count" -gt 256 ]]; then
        CHECK8B_CLASSIFICATION="ambiguous"
        CHECK8B_MUTATION_TARGET="unresolved"
        CHECK8B_REASON="token-byte-limit"
        CHECK8B_BOUNDARY="token-bytes=257/256:first-overflow"
        CHECK8B_TOKEN_BYTE_COUNT=257
        CHECK8B_TOKEN_COUNT=0
        CHECK8B_CANDIDATE_COUNT=0
        _CHECK8B_TOKENS=()
        _CHECK8B_CLAUSE_IDS=()
        _CHECK8B_CANDIDATE_INDEXES=()
        return 0
      fi
      if [[ "$token_byte_count" -gt "$max_token_byte_count" ]]; then
        max_token_byte_count="$token_byte_count"
      fi
      token="$token$character"
      continue
    fi

    if (( (character_code < 32 || character_code == 127) && character_code != 9 && character_code != 13 )); then
      CHECK8B_CLASSIFICATION="error"
      CHECK8B_MUTATION_TARGET="unavailable"
      CHECK8B_REASON="normalization-error"
      CHECK8B_TOKEN_COUNT=0
      CHECK8B_CANDIDATE_COUNT=0
      _CHECK8B_TOKENS=()
      _CHECK8B_CLAUSE_IDS=()
      _CHECK8B_CANDIDATE_INDEXES=()
      return 2
    fi

    if [[ -n "$token" ]]; then
      token="${token,,}"
      _CHECK8B_TOKENS+=("$token")
      _CHECK8B_CLAUSE_IDS+=("$clause_id")
      case "$token" in
        but|however|although|yet) clause_id=$((clause_id + 1)) ;;
      esac
      token=""
      token_byte_count=0
    fi
    case "$character" in
      '.'|','|':'|';'|'!'|'?') clause_id=$((clause_id + 1)) ;;
    esac
  done

  if [[ "$token_overflow" -eq 0 && -n "$token" ]]; then
    token="${token,,}"
    _CHECK8B_TOKENS+=("$token")
    _CHECK8B_CLAUSE_IDS+=("$clause_id")
  fi
  CHECK8B_TOKEN_BYTE_COUNT="$max_token_byte_count"
  token_count="${#_CHECK8B_TOKENS[@]}"
  CHECK8B_TOKEN_COUNT="$token_count"

  if [[ "$token_overflow" -eq 1 ]]; then
    CHECK8B_CLASSIFICATION="ambiguous"
    CHECK8B_MUTATION_TARGET="unresolved"
    CHECK8B_REASON="token-limit"
    CHECK8B_BOUNDARY="tokens=129/128:first-overflow"
    return 0
  fi
  if [[ "$line_byte_count" -eq 4096 ]]; then
    CHECK8B_BOUNDARY="line-bytes=4096/4096"
  elif [[ "$max_token_byte_count" -eq 256 ]]; then
    CHECK8B_BOUNDARY="token-bytes=256/256"
  elif [[ "$token_count" -eq 128 ]]; then
    CHECK8B_BOUNDARY="tokens=128/128"
  fi
  [[ "$token_count" -gt 0 ]] || return 0

  for ((index = 0; index < token_count; index++)); do
    token="${_CHECK8B_TOKENS[$index]}"
    if _check8b_mutation_verb "$token"; then
      candidate_count=$((candidate_count + 1))
      [[ -n "$first_verb" ]] || first_verb="$_CHECK8B_WORD_RESULT"
      if [[ "$candidate_count" -gt 8 ]]; then
        CHECK8B_CLASSIFICATION="ambiguous"
        CHECK8B_VERB="$first_verb"
        CHECK8B_MUTATION_TARGET="unresolved"
        CHECK8B_REASON="candidate-limit"
        CHECK8B_BOUNDARY="candidates=9/8:first-overflow"
        CHECK8B_CANDIDATE_COUNT=9
        return 0
      fi
      _CHECK8B_CANDIDATE_INDEXES+=("$index")
      candidate_verbs+=("$_CHECK8B_WORD_RESULT")
    fi
  done

  CHECK8B_CANDIDATE_COUNT="$candidate_count"
  [[ "$candidate_count" -gt 0 ]] || return 0
  CHECK8B_VERB="$first_verb"
  if [[ "$CHECK8B_BOUNDARY" == "not-applicable" && "$candidate_count" -eq 8 ]]; then
    CHECK8B_BOUNDARY="candidates=8/8"
  fi

  for ((candidate_index = 0; candidate_index < candidate_count; candidate_index++)); do
    index="${_CHECK8B_CANDIDATE_INDEXES[$candidate_index]}"
    verb="${candidate_verbs[$candidate_index]}"
    _check8b_target_after "$index" "$verb"
    case "$_CHECK8B_RELATION_KIND" in
      direct) :
        surface="$_CHECK8B_TARGET_RESULT"
        _check8b_csv_add "$verb:$surface" "$direct_surfaces"
        direct_surfaces="$_CHECK8B_CSV_RESULT"
        [[ "$CHECK8B_MUTATION_TARGET" != "none" ]] \
          || CHECK8B_MUTATION_TARGET="$surface"
        ;;
      negated) :
        negated_count=$((negated_count + 1))
        surface="$_CHECK8B_TARGET_RESULT"
        _check8b_csv_add "$surface" "$preserved_surfaces"
        preserved_surfaces="$_CHECK8B_CSV_RESULT"
        [[ -n "$first_negated_target" ]] || first_negated_target="$surface"
        ;;
      tail) :
        [[ -n "$tail_phrase" ]] || tail_phrase="$_CHECK8B_UNRESOLVED_RESULT"
        [[ -n "$tail_surface" ]] || tail_surface="$_CHECK8B_TAIL_SURFACE"
        ;;
      unresolved) :
        [[ -n "$unresolved_phrase" ]] || unresolved_phrase="$_CHECK8B_UNRESOLVED_RESULT"
        [[ -n "$unresolved_reason" ]] || unresolved_reason="unresolved"
        ;;
      other) :
        [[ -n "$other_target" ]] || other_target="$_CHECK8B_TARGET_RESULT"
        ;;
    esac
  done

  for ((index = 0; index < token_count; index++)); do
    token="${_CHECK8B_TOKENS[$index]}"
    if _check8b_is_surface "$token"; then
      surface_count=$((surface_count + 1))
    fi

    if _check8b_is_preservation_verb "$token" \
      && _check8b_surface_from "$((index + 1))" "$((index + 8))"; then
      surface="$_CHECK8B_SURFACE_RESULT"
      reference_index="$_CHECK8B_SURFACE_START_INDEX"
      if [[ -n "$_CHECK8B_REFERENCE_RESULT" ]] \
        && ! _check8b_named_surface_before "$reference_index" "$surface"; then
        unresolved_reference=1
        [[ -n "$unresolved_phrase" ]] \
          || unresolved_phrase="$_CHECK8B_REFERENCE_RESULT $surface"
      else
        _check8b_csv_add "$surface" "$preserved_surfaces"
        preserved_surfaces="$_CHECK8B_CSV_RESULT"
      fi
    fi

    if [[ "$token" == "without" && "$((index + 2))" -lt "$token_count" ]] \
      && _check8b_is_change_verb "${_CHECK8B_TOKENS[$((index + 1))]}" \
      && _check8b_surface_from "$((index + 2))" "$((index + 9))"; then
      surface="$_CHECK8B_SURFACE_RESULT"
      reference_index="$_CHECK8B_SURFACE_START_INDEX"
      if [[ -n "$_CHECK8B_REFERENCE_RESULT" ]] \
        && ! _check8b_named_surface_before "$reference_index" "$surface"; then
        unresolved_reference=1
        [[ -n "$unresolved_phrase" ]] \
          || unresolved_phrase="$_CHECK8B_REFERENCE_RESULT $surface"
      else
        _check8b_csv_add "$surface" "$preserved_surfaces"
        preserved_surfaces="$_CHECK8B_CSV_RESULT"
      fi
    fi

    if _check8b_is_preserved_state "$token"; then
      state_index=$((index - 1))
      if [[ "$state_index" -ge 0 ]] \
        && _check8b_is_state_auxiliary "${_CHECK8B_TOKENS[$state_index]}"; then
        state_lower_bound=$((index - 8))
        [[ "$state_lower_bound" -ge 0 ]] || state_lower_bound=0
        state_index=$((state_index - 1))
        while [[ "$state_index" -ge "$state_lower_bound" ]] \
          && [[ "${_CHECK8B_CLAUSE_IDS[$state_index]}" -eq "${_CHECK8B_CLAUSE_IDS[$index]}" ]]; do
          if _check8b_is_surface_head_at "$state_index"; then
            _check8b_csv_add "$_CHECK8B_SURFACE_RESULT" "$preserved_surfaces"
            preserved_surfaces="$_CHECK8B_CSV_RESULT"
            break
          fi
          state_index=$((state_index - 1))
        done
      fi
    fi

    if _check8b_is_use_verb "$token"; then
      if _check8b_surface_from "$((index + 1))" "$((index + 8))"; then
        _check8b_csv_add "$_CHECK8B_SURFACE_RESULT" "$used_surfaces"
        used_surfaces="$_CHECK8B_CSV_RESULT"
      elif _check8b_passive_surface "$index" \
        && _check8b_is_surface "$_CHECK8B_PASSIVE_TARGET"; then
        _check8b_csv_add "$_CHECK8B_PASSIVE_TARGET" "$used_surfaces"
        used_surfaces="$_CHECK8B_CSV_RESULT"
      fi
    fi
  done

  if [[ -n "$other_target" && -n "$used_surfaces" ]]; then
    remaining_pairs="$used_surfaces"
    while [[ -n "$remaining_pairs" ]]; do
      surface="${remaining_pairs%%,*}"
      if [[ "$remaining_pairs" == *,* ]]; then
        remaining_pairs="${remaining_pairs#*,}"
      else
        remaining_pairs=""
      fi
      _check8b_csv_add "$surface" "$preserved_surfaces"
      preserved_surfaces="$_CHECK8B_CSV_RESULT"
    done
  fi

  if [[ -n "$tail_phrase" ]]; then
    if [[ ",$preserved_surfaces," == *",$tail_surface,"* ]]; then
      [[ -n "$other_target" ]] || other_target="$tail_phrase"
    else
      CHECK8B_CLASSIFICATION="ambiguous"
      CHECK8B_MUTATION_TARGET="unresolved"
      CHECK8B_DIRECT_SURFACES="none"
      CHECK8B_PRESERVED_SURFACES="none"
      CHECK8B_REASON="surface-tail"
      CHECK8B_UNRESOLVED_PHRASE="$tail_phrase"
      return 0
    fi
  fi

  if [[ -n "$direct_surfaces" && -n "$preserved_surfaces" ]]; then
    remaining_pairs="$direct_surfaces"
    while [[ -n "$remaining_pairs" ]]; do
      direct_pair="${remaining_pairs%%,*}"
      if [[ "$remaining_pairs" == *,* ]]; then
        remaining_pairs="${remaining_pairs#*,}"
      else
        remaining_pairs=""
      fi
      surface="${direct_pair#*:}"
      case ",$preserved_surfaces," in
        *",$surface,"*) conflict=1 ;;
      esac
    done
  fi

  if [[ "$conflict" -eq 1 ]]; then
    CHECK8B_CLASSIFICATION="ambiguous"
    CHECK8B_MUTATION_TARGET="unresolved"
    CHECK8B_DIRECT_SURFACES="none"
    CHECK8B_PRESERVED_SURFACES="$preserved_surfaces"
    CHECK8B_REASON="conflict"
  elif [[ "$unresolved_reference" -eq 1 ]]; then
    CHECK8B_CLASSIFICATION="ambiguous"
    CHECK8B_MUTATION_TARGET="unresolved"
    CHECK8B_DIRECT_SURFACES="none"
    CHECK8B_PRESERVED_SURFACES="none"
    CHECK8B_REASON="unresolved-reference"
    CHECK8B_UNRESOLVED_PHRASE="${unresolved_phrase:-none}"
  elif [[ -n "$unresolved_reason" ]]; then
    CHECK8B_CLASSIFICATION="ambiguous"
    CHECK8B_MUTATION_TARGET="unresolved"
    CHECK8B_DIRECT_SURFACES="none"
    CHECK8B_PRESERVED_SURFACES="none"
    CHECK8B_REASON="$unresolved_reason"
    CHECK8B_UNRESOLVED_PHRASE="${unresolved_phrase:-none}"
  elif [[ -n "$direct_surfaces" && -n "$preserved_surfaces" ]]; then
    CHECK8B_CLASSIFICATION="mixed-surface"
    CHECK8B_DIRECT_SURFACES="$direct_surfaces"
    CHECK8B_PRESERVED_SURFACES="$preserved_surfaces"
    CHECK8B_REASON="mixed"
  elif [[ -n "$direct_surfaces" ]]; then
    CHECK8B_CLASSIFICATION="direct-positive"
    CHECK8B_DIRECT_SURFACES="$direct_surfaces"
    CHECK8B_REASON="direct"
  elif [[ -n "$preserved_surfaces" ]] \
    && { [[ -n "$other_target" ]] || [[ "$negated_count" -gt 0 ]]; }; then
    CHECK8B_CLASSIFICATION="negative"
    CHECK8B_MUTATION_TARGET="${other_target:-$first_negated_target}"
    CHECK8B_PRESERVED_SURFACES="$preserved_surfaces"
    CHECK8B_REASON="preserved-surface"
  elif [[ -n "$other_target" && -n "$used_surfaces" ]]; then
    CHECK8B_CLASSIFICATION="negative"
    CHECK8B_MUTATION_TARGET="$other_target"
    CHECK8B_REASON="preserved-surface"
  elif [[ "$surface_count" -gt 0 ]]; then
    CHECK8B_CLASSIFICATION="ambiguous"
    CHECK8B_MUTATION_TARGET="unresolved"
    CHECK8B_REASON="unresolved"
    CHECK8B_UNRESOLVED_PHRASE="${unresolved_phrase:-none}"
  fi
  return 0
}
# END CHECK8B FINITE CLASSIFIER

echo "--- Check 8B: Consumer Trace Planning For Renames/Removals ---"
rename_scope_hits=0
check8b_relevant_scope_hits=0
missing_consumer_trace=0

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue
  scope_label="$(scope_analysis_label "$scope_index")"
  check8b_scope_has_direct=0
  check8b_scope_direct_union=""
  check8b_record_count=0
  check8b_record_classifications=()
  check8b_record_targets=()
  check8b_record_direct=()
  check8b_record_preserved=()
  check8b_record_reasons=()
  check8b_record_unresolved=()
  check8b_record_boundaries=()
  check8b_record_locations=()

  if ! _scope_context_consume "$scope_index" "Check 8B"; then
    continue
  fi
  for check8b_context_index in "${!SCOPE_CONTEXT_ACTIVE_LINES[@]}"; do
    check8b_scope_line="${SCOPE_CONTEXT_ACTIVE_LINES[$check8b_context_index]}"
    if check8b_classify_line "$check8b_scope_line"; then
      check8b_helper_status=0
    else
      check8b_helper_status=$?
    fi

    case "$CHECK8B_CLASSIFICATION" in
      irrelevant)
        continue
        ;;
      direct-positive|mixed-surface)
        check8b_scope_has_direct=1
        check8b_remaining_direct="$CHECK8B_DIRECT_SURFACES"
        while [[ -n "$check8b_remaining_direct" ]]; do
          check8b_direct_pair="${check8b_remaining_direct%%,*}"
          if [[ "$check8b_remaining_direct" == *,* ]]; then
            check8b_remaining_direct="${check8b_remaining_direct#*,}"
          else
            check8b_remaining_direct=""
          fi
          _check8b_csv_add "$check8b_direct_pair" "$check8b_scope_direct_union"
          check8b_scope_direct_union="$_CHECK8B_CSV_RESULT"
        done
        ;;
      negative|ambiguous|error) ;;
      *)
        CHECK8B_CLASSIFICATION="error"
        CHECK8B_MUTATION_TARGET="unavailable"
        CHECK8B_DIRECT_SURFACES="none"
        CHECK8B_PRESERVED_SURFACES="none"
        CHECK8B_REASON="normalization-error"
        CHECK8B_UNRESOLVED_PHRASE="none"
        CHECK8B_BOUNDARY="not-applicable"
        ;;
    esac
    if [[ "$check8b_helper_status" -ne 0 && "$CHECK8B_CLASSIFICATION" != "error" ]]; then
      CHECK8B_CLASSIFICATION="error"
      CHECK8B_MUTATION_TARGET="unavailable"
      CHECK8B_DIRECT_SURFACES="none"
      CHECK8B_PRESERVED_SURFACES="none"
      CHECK8B_REASON="normalization-error"
      CHECK8B_UNRESOLVED_PHRASE="none"
      CHECK8B_BOUNDARY="not-applicable"
    fi
    check8b_record_classifications[$check8b_record_count]="$CHECK8B_CLASSIFICATION"
    check8b_record_targets[$check8b_record_count]="$CHECK8B_MUTATION_TARGET"
    check8b_record_direct[$check8b_record_count]="$CHECK8B_DIRECT_SURFACES"
    check8b_record_preserved[$check8b_record_count]="$CHECK8B_PRESERVED_SURFACES"
    check8b_record_reasons[$check8b_record_count]="$CHECK8B_REASON"
    check8b_record_unresolved[$check8b_record_count]="$CHECK8B_UNRESOLVED_PHRASE"
    check8b_record_boundaries[$check8b_record_count]="$CHECK8B_BOUNDARY"
    check8b_record_locations[$check8b_record_count]="${SCOPE_CONTEXT_ACTIVE_LOCATIONS[$check8b_context_index]}"
    check8b_record_count=$((check8b_record_count + 1))
    check8b_relevant_scope_hits=$((check8b_relevant_scope_hits + 1))
  done

  check8b_impact_sweep="skipped"
  check8b_impact_completion="skipped"
  check8b_impact_inventory="skipped"
  check8b_scope_impact_missing=0
  if [[ "$check8b_scope_has_direct" -eq 1 ]]; then
    rename_scope_hits=$((rename_scope_hits + 1))
    if grep -Eiq 'Consumer Impact Sweep' "$scope_path"; then
      check8b_impact_sweep="pass"
      pass "Scope includes Consumer Impact Sweep section: $scope_label"
    else
      check8b_impact_sweep="missing"
      check8b_scope_impact_missing=1
      fail "Scope renames/removes interfaces but has no Consumer Impact Sweep section: $scope_label"
      missing_consumer_trace=$((missing_consumer_trace + 1))
    fi

    if grep -Eiq '^\- \[(x| )\] .*consumer impact sweep.*zero stale first-party references remain' "$scope_path"; then
      check8b_impact_completion="pass"
      pass "Scope DoD includes consumer impact sweep completion item: $scope_label"
    else
      check8b_impact_completion="missing"
      check8b_scope_impact_missing=1
      fail "Scope renames/removes interfaces but is missing DoD item for consumer impact sweep: $scope_label"
      missing_consumer_trace=$((missing_consumer_trace + 1))
    fi

    if awk '
      {
        normalized = tolower($0)
        if (normalized ~ /^[[:space:]]*#+[[:space:]]+/) {
          in_inventory_section = normalized ~ /(consumer impact sweep|affected consumer surfaces)/
          next
        }
        if (in_inventory_section && normalized ~ /(navigation|breadcrumb|redirect|api client|generated client|deep link|stale-reference)/) {
          found = 1
        }
      }
      END { exit found ? 0 : 1 }
    ' "$scope_path"; then
      check8b_impact_inventory="pass"
      pass "Scope lists affected consumer surfaces for rename/removal work: $scope_label"
    else
      check8b_impact_inventory="missing"
      check8b_scope_impact_missing=1
      fail "Scope renames/removes interfaces but does not enumerate affected consumer surfaces: $scope_label"
      missing_consumer_trace=$((missing_consumer_trace + 1))
    fi
  fi

  for ((check8b_record_index = 0; check8b_record_index < check8b_record_count; check8b_record_index++)); do
    check8b_record_classification="${check8b_record_classifications[$check8b_record_index]}"
    check8b_record_target="${check8b_record_targets[$check8b_record_index]}"
    check8b_record_direct_value="${check8b_record_direct[$check8b_record_index]}"
    check8b_record_preserved_value="${check8b_record_preserved[$check8b_record_index]}"
    check8b_record_reason="${check8b_record_reasons[$check8b_record_index]}"
    check8b_record_unresolved_value="${check8b_record_unresolved[$check8b_record_index]}"
    check8b_record_boundary="${check8b_record_boundaries[$check8b_record_index]}"
    check8b_record_location="${check8b_record_locations[$check8b_record_index]}"
    check8b_record_impact="skipped"
    check8b_record_sweep="skipped"
    check8b_record_completion="skipped"
    check8b_record_inventory="skipped"
    check8b_record_result="continue"
    check8b_record_correction="none"

    case "$check8b_record_classification" in
      direct-positive|mixed-surface)
        check8b_record_impact="run"
        check8b_record_sweep="$check8b_impact_sweep"
        check8b_record_completion="$check8b_impact_completion"
        check8b_record_inventory="$check8b_impact_inventory"
        if [[ "$check8b_scope_impact_missing" -ne 0 ]]; then
          check8b_record_result="blocked"
          check8b_record_correction="Add only the missing Consumer Impact Sweep section, completion item, and affected-consumer inventory for direct surfaces: $check8b_record_direct_value."
        fi
        ;;
      ambiguous)
        check8b_record_result="blocked"
        case "$check8b_record_reason" in
          conflict)
            check8b_conflict_surface="${check8b_record_preserved_value%%,*}"
            check8b_record_correction="Rewrite only this declaration so $check8b_conflict_surface is either removed or preserved, not both."
            ;;
          unresolved)
            check8b_record_correction="Rewrite only this declaration to state whether the public API route changes or remains unchanged."
            ;;
          unresolved-reference)
            check8b_record_correction="Rewrite only this declaration to name the referenced consumer surface explicitly."
            ;;
          surface-tail)
            check8b_tail_record_surface="${check8b_record_unresolved_value%% *}"
            check8b_record_correction="Rewrite only this declaration to distinguish artifact '$check8b_record_unresolved_value' from consumer surface '$check8b_tail_record_surface'."
            ;;
          token-limit)
            check8b_record_correction="Split only this declaration before token 129 while preserving its meaning."
            ;;
          candidate-limit)
            check8b_record_correction="Split only this declaration so each declaration has at most eight candidates."
            ;;
          line-byte-limit)
            check8b_record_correction="Shorten only this declaration to at most 4096 bytes while preserving its meaning."
            ;;
          token-byte-limit)
            check8b_record_correction="Shorten only this token to at most 256 bytes while preserving its meaning."
            ;;
        esac
        ;;
      error)
        check8b_record_result="blocked"
        check8b_record_correction="Rewrite only this declaration as one bounded plain-text sentence. Preserve its meaning, then rerun."
        ;;
    esac

    printf '%s\n' \
      'check: Check 8B' \
      "scope: $scope_label" \
      "source-location: $scope_label:$check8b_record_location" \
      "classification: $check8b_record_classification" \
      "mutation-target: $check8b_record_target" \
      "direct-surfaces: $check8b_record_direct_value" \
      "preserved-surfaces: $check8b_record_preserved_value" \
      "reason: $check8b_record_reason" \
      "boundary: $check8b_record_boundary" \
      "impact-checks: $check8b_record_impact" \
      "impact-sweep-section: $check8b_record_sweep" \
      "impact-completion-item: $check8b_record_completion" \
      "impact-consumer-inventory: $check8b_record_inventory" \
      "result: $check8b_record_result" \
      "correction: $check8b_record_correction"
    if [[ "$check8b_record_classification" == "ambiguous" ]]; then
      fail "Check 8B blocked an ambiguous declaration in $scope_label with reason $check8b_record_reason"
    elif [[ "$check8b_record_classification" == "error" ]]; then
      fail "Check 8B classifier failed for a declaration in $scope_label with reason $check8b_record_reason"
    fi
  done
done

if [[ "$check8b_relevant_scope_hits" -eq 0 ]]; then
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

for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
