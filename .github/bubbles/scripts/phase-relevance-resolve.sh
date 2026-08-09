#!/usr/bin/env bash
set -euo pipefail
umask 077

# phase-relevance-resolve.sh — the ONE executable phase-relevance resolver
# (IMP-038 SCOPE-5 / GF-4).
#
# `phaseRelevance` in bubbles/workflows/modes.yaml has been a REGISTRY that no
# executable consumed: each top-level runner decided for itself whether a phase
# was relevant, so four runners could reach four different verdicts for the same
# scope and the published "smart phase routing" claim covered whichever runner
# happened to implement it. This script makes the registry executable and gives
# every authorized runner ONE verdict to consume.
#
# It REDUCES IRRELEVANT WORK, NEVER ASSURANCE. Three properties enforce that:
#
#   1. neverSkip wins over every rule, unconditionally.
#   2. A rule whose `skipWhen` token has no evaluator here resolves to `run`,
#      and says so. An unrecognized classification is ambiguity, and ambiguity
#      must not silently delete a phase.
#   3. An evaluator that lacks the input it needs resolves to `run`. "I could
#      not tell" is not "not relevant".
#
# The rules are READ FROM THE REGISTRY, never restated here. Adding a rule to
# modes.yaml with no evaluator therefore degrades to `run` + a named reason
# rather than to an unenforced claim.
#
# Output (stdout), four lines:
#   verdict=<run|skip>
#   phase=<phase>
#   rule=<neverSkip|no-rule|unevaluated:<token>|<token>>
#   reason=<why>
#
# Exit codes (closed set):
#   0  decided — a verdict was printed
#   2  usage error, missing dependency, or an unreadable registry
#
# There is no --force / --skip-phase / --ignore. A caller that wants a phase to
# run always gets `run` by supplying nothing; a caller cannot ask this script
# to skip a phase it decided to run.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODES_FILE="${BUBBLES_MODES_FILE:-$REPO_ROOT/bubbles/workflows/modes.yaml}"

usage() {
  cat <<'EOF'
Usage: phase-relevance-resolve.sh --phase <phase> [inputs...]

Resolves whether a phase is relevant to the current scope, using the
`phaseRelevance` registry in bubbles/workflows/modes.yaml.

Required:
  --phase <name>                 The phase about to be dispatched.

Inputs (each omitted input makes its rule resolve to `run`):
  --changed-surface-file <path>  Newline-separated changed paths.
  --changed-lines <n>            Total changed lines across the scope.
  --spec-dir <path>              Feature dir; its spec/scope artifacts are read
                                 for SLA, ambiguity, and prior-spec signals.
  --session-file <path>          Session JSON; read for grillMode.
  --runner <agent>               Recorded in the reason for audit.

Output (stdout), four lines:
  verdict=<run|skip>
  phase=<phase>
  rule=<neverSkip|no-rule|unevaluated:<token>|<token>>
  reason=<why>

Exit codes:
  0  decided
  2  usage error, missing dependency, or unreadable registry

Fail-safe: unknown phase, unknown rule token, or missing input all resolve to
`run`. There is no flag that can force a skip.
EOF
}

fail_usage() { echo "phase-relevance-resolve: $*" >&2; exit 2; }

phase=""
changed_surface_file=""
changed_lines=""
spec_dir=""
session_file=""
runner=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase) phase="${2:-}"; shift 2 ;;
    --changed-surface-file) changed_surface_file="${2:-}"; shift 2 ;;
    --changed-lines) changed_lines="${2:-}"; shift 2 ;;
    --spec-dir) spec_dir="${2:-}"; shift 2 ;;
    --session-file) session_file="${2:-}"; shift 2 ;;
    --runner) runner="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail_usage "unknown option: $1" ;;
  esac
done

[[ -n "$phase" ]] || fail_usage "--phase is required"
[[ -f "$MODES_FILE" ]] || fail_usage "modes registry not found: $MODES_FILE"
command -v yq >/dev/null 2>&1 || fail_usage "yq (mikefarah, v4+) is required to read $MODES_FILE"

if [[ -n "$changed_lines" && ! "$changed_lines" =~ ^[0-9]+$ ]]; then
  fail_usage "--changed-lines must be a non-negative integer (got: $changed_lines)"
fi

decide() {
  local verdict="$1" rule="$2" reason="$3"
  if [[ -n "$runner" ]]; then
    reason="$reason (runner: $runner)"
  fi
  printf 'verdict=%s\nphase=%s\nrule=%s\nreason=%s\n' "$verdict" "$phase" "$rule" "$reason"
  exit 0
}

# --- registry reads ---------------------------------------------------------

registry_enabled="$(yq -r '.modes.phaseRelevance.enabled // "false"' "$MODES_FILE")"
if [[ "$registry_enabled" != "true" ]]; then
  decide "run" "no-rule" "phaseRelevance is disabled in the registry — every phase runs"
fi

# yq is mikefarah v4: parameters arrive through the environment (`strenv`), not
# through jq's `--arg`, which yq accepts and then silently ignores.
if [[ "$(BUBBLES_PR_PHASE="$phase" yq -r '.modes.phaseRelevance.neverSkip // [] | any_c(. == strenv(BUBBLES_PR_PHASE))' "$MODES_FILE" 2>/dev/null)" == "true" ]]; then
  decide "run" "neverSkip" "'$phase' is on the registry neverSkip list and is never subject to relevance rules"
fi

# Every skipWhen token declared for this phase, in registry order.
mapfile -t phase_rules < <(BUBBLES_PR_PHASE="$phase" yq -r \
  '.modes.phaseRelevance.rules // [] | map(select(.phase == strenv(BUBBLES_PR_PHASE))) | .[].skipWhen' \
  "$MODES_FILE" 2>/dev/null || true)

if [[ "${#phase_rules[@]}" -eq 0 ]]; then
  decide "run" "no-rule" "no phaseRelevance rule declares '$phase' — an undeclared phase always runs"
fi

rule_reason() {
  BUBBLES_PR_PHASE="$phase" BUBBLES_PR_TOKEN="$1" yq -r \
    '.modes.phaseRelevance.rules // [] | map(select(.phase == strenv(BUBBLES_PR_PHASE) and .skipWhen == strenv(BUBBLES_PR_TOKEN))) | .[0].reason // ""' \
    "$MODES_FILE" 2>/dev/null || true
}

# --- artifact helpers -------------------------------------------------------

# scope_artifacts — every spec/scope file the evaluators read. Empty when no
# --spec-dir was supplied, which makes each artifact-based rule resolve to run.
scope_artifacts() {
  [[ -n "$spec_dir" && -d "$spec_dir" ]] || return 0
  find "$spec_dir" -maxdepth 2 -type f \
    \( -name 'spec.md' -o -name 'scopes.md' -o -name 'scope.md' -o -name 'design.md' \) 2>/dev/null
}

changed_paths() {
  [[ -n "$changed_surface_file" && -f "$changed_surface_file" ]] || return 0
  grep -v '^[[:space:]]*$' "$changed_surface_file" 2>/dev/null || true
}

have_changed_surface() {
  [[ -n "$changed_surface_file" && -f "$changed_surface_file" ]] && [[ -n "$(changed_paths)" ]]
}

# --- evaluators -------------------------------------------------------------
#
# Each returns 0 when the skip condition HOLDS (so the phase may be skipped),
# 1 when it does not, and 2 when the input needed to decide is absent.

eval_scope_changed_fewer_than_50_lines() {
  [[ -n "$changed_lines" ]] || return 2
  [[ "$changed_lines" -lt 50 ]]
}

# Mirrors state-transition-guard.sh Check 5A. The word boundaries on sla/slo are
# load-bearing: without them 'slot', 'slate', and 'Slack' all read as an SLA
# declaration and the stabilize phase stops skipping for unrelated scopes.
eval_scope_has_no_sla_or_perf_targets() {
  local artifacts
  artifacts="$(scope_artifacts)"
  [[ -n "$artifacts" ]] || return 2
  ! printf '%s\n' "$artifacts" | tr '\n' '\0' \
    | xargs -0 grep -Eliq 'latency|throughput|p95|p99|response time|\bsla\b|\bslo\b' 2>/dev/null
}

eval_scope_has_no_ci_deploy_or_infra_changes() {
  have_changed_surface || return 2
  ! changed_paths | grep -Eq '(^|/)(\.github/workflows|deploy|deployment|infra|infrastructure|docker|k8s|kubernetes|helm|terraform)(/|$)|(^|/)(Dockerfile|docker-compose[^/]*\.ya?ml|Makefile)$'
}

eval_scope_is_docs_only_or_config_only() {
  have_changed_surface || return 2
  ! changed_paths | grep -Evq '\.(md|markdown|txt|rst|adoc|ya?ml|json|toml|ini|cfg|conf|properties)$'
}

eval_scope_has_no_ambiguity_markers() {
  local artifacts
  artifacts="$(scope_artifacts)"
  [[ -n "$artifacts" ]] || return 2
  ! printf '%s\n' "$artifacts" | tr '\n' '\0' \
    | xargs -0 grep -Eliq 'TBD|TODO|\?\?\?|to be decided|to be determined|unclear|ambiguous|NEEDS CLARIFICATION|open question' 2>/dev/null
}

eval_grill_mode_off() {
  [[ -n "$session_file" && -f "$session_file" ]] || return 2
  command -v jq >/dev/null 2>&1 || return 2
  local mode
  mode="$(jq -r '.executionOptions.grillMode // .policySnapshot.grillMode // "off"' "$session_file" 2>/dev/null || echo "off")"
  [[ "$mode" == "off" ]]
}

eval_scope_has_no_auth_input_crypto_or_trust_boundary_changes() {
  have_changed_surface || return 2
  ! changed_paths | grep -Eiq 'auth|login|session|token|jwt|oauth|passw|credential|secret|crypt|cipher|hash|sign|cert|tls|ssl|permission|role|acl|policy|sanitiz|validat|escape|middleware'
}

eval_scope_is_new_feature_with_no_prior_specs() {
  [[ -n "$spec_dir" ]] || return 2
  local specs_root
  specs_root="$(dirname "$spec_dir")"
  [[ -d "$specs_root" ]] || return 2
  # A prior spec is any sibling feature directory carrying a state.json.
  local prior
  prior="$(find "$specs_root" -mindepth 2 -maxdepth 2 -name 'state.json' 2>/dev/null \
    | grep -v "^$spec_dir/" || true)"
  [[ -z "$prior" ]]
}

# --- resolution -------------------------------------------------------------
#
# Registry order decides: the FIRST rule whose skip condition holds wins. A
# token with no evaluator, or an evaluator with no input, ends the loop at
# `run` rather than falling through to a later rule — a phase must not be
# skipped by a rule the caller never supplied evidence for.

for token in "${phase_rules[@]}"; do
  [[ -n "$token" ]] || continue
  evaluator="eval_${token}"
  if ! declare -F "$evaluator" >/dev/null 2>&1; then
    decide "run" "unevaluated:$token" \
      "the registry declares skipWhen '$token' for '$phase' but no evaluator implements it — an unrecognized classification resolves to run, never to a silent skip"
  fi
  status=0
  "$evaluator" || status=$?
  case "$status" in
    0)
      registry_reason="$(rule_reason "$token")"
      decide "skip" "$token" "${registry_reason:-skip condition '$token' holds for '$phase'}"
      ;;
    1) : ;; # condition does not hold; try the next rule for this phase
    *)
      decide "run" "$token" \
        "skipWhen '$token' could not be evaluated — the required input was not supplied, and 'could not tell' is not 'not relevant'"
      ;;
  esac
done

decide "run" "no-rule" "no skip condition held for '$phase' — the phase is relevant to this scope"
