#!/usr/bin/env bash
set -euo pipefail

# strict-terminal-status-guard.sh
#
# Gate G092 - strict_terminal_status_gate.
#
# New certification writes may use only the terminal delivery statuses `done`
# and `blocked`. Legacy `done_with_concerns` remains readable only for old,
# untouched specs until recertification migrates the status to `done` plus
# observations, or to `blocked` when required work remains.
#
# Usage:
#   bash bubbles/scripts/strict-terminal-status-guard.sh <specDir> [--quiet] [--repo-root <root>]
#
# Exit codes:
#   0  clean
#   1  one or more G092 strict terminal status violations
#   2  runtime error, invalid arguments, missing state, or malformed JSON

QUIET="false"
SPEC_DIR_INPUT=""
REPO_ROOT_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/strict-terminal-status-guard.sh <specDir> [--quiet] [--repo-root <root>]

Arguments:
  <specDir>          Spec directory containing state.json.

Optional:
  --quiet            Suppress success output.
  --repo-root <root> Repository root used to scan workflow and agent contracts.
  -h, --help         Print this usage and exit.

Exit codes:
  0 = clean
  1 = G092 strict terminal status violation
  2 = runtime error, invalid arguments, missing state, or malformed JSON
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || { echo "strict-terminal-status-guard: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_INPUT="$2"
      shift 2
      ;;
    --*)
      echo "strict-terminal-status-guard: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$SPEC_DIR_INPUT" ]]; then
        echo "strict-terminal-status-guard: only one specDir may be supplied" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR_INPUT" ]]; then
  echo "strict-terminal-status-guard: missing required specDir" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "strict-terminal-status-guard: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR_INPUT" ]]; then
  echo "strict-terminal-status-guard: specDir does not exist: $SPEC_DIR_INPUT" >&2
  exit 2
fi

SPEC_DIR_ABS="$(cd "$SPEC_DIR_INPUT" && pwd -P)"
STATE_FILE="$SPEC_DIR_ABS/state.json"

resolve_repo_file() {
  local rel="$1"
  local candidate

  for candidate in "$REPO_ROOT/$rel" "$REPO_ROOT/.github/$rel"; do
    if [[ -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done

  return 1
}

if [[ ! -f "$STATE_FILE" ]]; then
  echo "strict-terminal-status-guard: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "strict-terminal-status-guard: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

resolve_repo_root() {
  if [[ -n "$REPO_ROOT_INPUT" ]]; then
    (cd "$REPO_ROOT_INPUT" && pwd -P)
    return 0
  fi

  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    (cd "$BUBBLES_REPO_ROOT" && pwd -P)
    return 0
  fi

  local parent
  parent="$(dirname "$SPEC_DIR_ABS")"
  if [[ "$(basename "$parent")" == "specs" ]]; then
    (cd "$(dirname "$parent")" && pwd -P)
    return 0
  fi

  local dir
  dir="$SPEC_DIR_ABS"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/specs" && ( -f "$dir/bubbles/workflows.yaml" || -f "$dir/.github/bubbles/workflows.yaml" || -d "$dir/.specify/memory" ) ]]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done

  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" || ! -d "$REPO_ROOT" ]]; then
  echo "strict-terminal-status-guard: unable to resolve repository root from specDir: $SPEC_DIR_INPUT" >&2
  exit 2
fi

WORKFLOWS_FILE="$(resolve_repo_file "bubbles/workflows.yaml" || true)"
if [[ -z "$WORKFLOWS_FILE" ]]; then
  echo "strict-terminal-status-guard: workflows.yaml not found under $REPO_ROOT/bubbles or $REPO_ROOT/.github/bubbles" >&2
  exit 2
fi

relative_path() {
  local path="$1"
  if [[ "$path" == "$REPO_ROOT" ]]; then
    printf '.'
  elif [[ "$path" == "$REPO_ROOT"/* ]]; then
    printf '%s' "${path#$REPO_ROOT/}"
  else
    printf '%s' "$path"
  fi
}

spec_rel="$(relative_path "$SPEC_DIR_ABS")"
finding_count=0
legacy_read_only_count=0
text_files_scanned=0

violation() {
  echo "G092 strict_terminal_status_gate violation: $*" >&2
  finding_count=$((finding_count + 1))
}

state_string_type_or_missing() {
  local expr="$1"
  jq -r "$expr" "$STATE_FILE"
}

validate_state_shape() {
  local status_type certification_status_type observations_type certification_observations_type

  status_type="$(state_string_type_or_missing 'if has("status") then (.status | type) else "missing" end')"
  if [[ "$status_type" != "string" && "$status_type" != "missing" ]]; then
    echo "strict-terminal-status-guard: state.status must be a string when present in $STATE_FILE" >&2
    exit 2
  fi

  certification_status_type="$(state_string_type_or_missing 'if (.certification? | type) == "object" and (.certification | has("status")) then (.certification.status | type) else "missing" end')"
  if [[ "$certification_status_type" != "string" && "$certification_status_type" != "missing" ]]; then
    echo "strict-terminal-status-guard: certification.status must be a string when present in $STATE_FILE" >&2
    exit 2
  fi

  observations_type="$(state_string_type_or_missing 'if has("observations") then (.observations | type) else "array" end')"
  if [[ "$observations_type" != "array" ]]; then
    echo "strict-terminal-status-guard: observations must be an array when present in $STATE_FILE" >&2
    exit 2
  fi

  certification_observations_type="$(state_string_type_or_missing 'if (.certification? | type) == "object" and (.certification | has("observations")) then (.certification.observations | type) else "array" end')"
  if [[ "$certification_observations_type" != "array" ]]; then
    echo "strict-terminal-status-guard: certification.observations must be an array when present in $STATE_FILE" >&2
    exit 2
  fi
}

legacy_done_with_concerns_read_only() {
  local file="$1"
  jq -e '
    (.status == "done_with_concerns" or .certification.status? == "done_with_concerns")
    and ((.legacyStatusCompatibility == true) or (.certification.legacyStatusCompatibility == true))
    and ((.certifiedAt? // "") | type == "string" and length > 0)
    and (.requiresRevalidation != true)
    and (.touchedForRecertification != true)
    and (.recertifying != true)
    and (.certification.touched != true)
    and (.certification.recertifying != true)
    and ((.proposedStatus? // "") != "done_with_concerns")
    and ((.certification.proposedStatus? // "") != "done_with_concerns")
    and ((.certification.targetStatus? // "") != "done_with_concerns")
    and ((.execution.proposedStatus? // "") != "done_with_concerns")
    and ((.resultEnvelope.outcome? // "") != "done_with_concerns")
    and ([.transitionRequests[]? | select(((.status? // .toStatus? // .targetStatus? // .outcome? // .proposedStatus? // "") == "done_with_concerns"))] | length == 0)
  ' "$file" >/dev/null
}

check_current_state() {
  validate_state_shape

  local status certification_status
  status="$(jq -r '.status // ""' "$STATE_FILE")"
  certification_status="$(jq -r '.certification.status // ""' "$STATE_FILE")"

  if [[ "$status" == "done_with_concerns" || "$certification_status" == "done_with_concerns" ]]; then
    if legacy_done_with_concerns_read_only "$STATE_FILE"; then
      legacy_read_only_count=$((legacy_read_only_count + 1))
    else
      violation "$spec_rel uses done_with_concerns in an active or recertified state; migrate to status done with observations[] when gates pass, or blocked when required work remains"
    fi
  fi

  while IFS= read -r path_label; do
    [[ -n "$path_label" ]] || continue
    violation "$spec_rel proposes forbidden new terminal status done_with_concerns at $path_label; valid new terminal statuses are done and blocked"
  done < <(jq -r '
    def scalar_path_labels:
      [
        ["proposedStatus"],
        ["targetStatus"],
        ["terminalStatus"],
        ["terminalOutcome"],
        ["outcome"],
        ["resultEnvelope", "outcome"],
        ["certification", "proposedStatus"],
        ["certification", "targetStatus"],
        ["certification", "statusWrite"],
        ["execution", "proposedStatus"],
        ["execution", "terminalOutcome"]
      ];
    [
      scalar_path_labels[] as $p
      | select((try getpath($p) catch null) == "done_with_concerns")
      | $p | join(".")
    ] +
    [
      .transitionRequests[]?
      | select(((.status? // .toStatus? // .targetStatus? // .outcome? // .proposedStatus? // "") == "done_with_concerns"))
      | "transitionRequests[]"
    ]
    | .[]
  ' "$STATE_FILE")

  local done_requested
  done_requested="$(jq -r '
    if (.status == "done")
      or (.certification.status? == "done")
      or (.proposedStatus? == "done")
      or (.certification.proposedStatus? == "done")
      or (.resultEnvelope.outcome? == "done")
    then "true" else "false" end
  ' "$STATE_FILE")"

  if [[ "$done_requested" == "true" ]]; then
    while IFS= read -r observation_label; do
      [[ -n "$observation_label" ]] || continue
      violation "$spec_rel records status done with a high/remediation-required observation at $observation_label; such observations must block certification"
    done < <(jq -r '
      def obs_stream:
        (.observations[]? | {where: "observations[]", item: .}),
        (.certification.observations[]? | {where: "certification.observations[]", item: .}),
        (.resultEnvelope.observations[]? | {where: "resultEnvelope.observations[]", item: .});
      obs_stream
      | select(.item | type == "object")
      | select(
          ((.item.severity? // "" | tostring | ascii_downcase) == "high")
          or (.item.remediationRequired == true)
          or (.item.remediation_required == true)
          or ((.item.kind? // "" | tostring | ascii_downcase) == "remediation-required")
          or ((.item.classification? // "" | tostring | ascii_downcase) == "remediation-required")
        )
      | .where
    ' "$STATE_FILE")
  fi
}

outcome_state_keys() {
  awk '
    /^outcomeStates:/ { in_outcomes=1; next }
    in_outcomes && /^[A-Za-z0-9_-]+:/ { exit }
    in_outcomes && /^  [A-Za-z0-9_-]+:/ {
      key=$1
      sub(":$", "", key)
      print key
    }
  ' "$WORKFLOWS_FILE"
}

check_workflow_outcome_states() {
  local keys key has_done has_blocked
  keys="$(outcome_state_keys)"
  has_done="false"
  has_blocked="false"

  if [[ -z "$keys" ]]; then
    violation "bubbles/workflows.yaml outcomeStates exposes no terminal delivery states"
    return 0
  fi

  while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    case "$key" in
      done) has_done="true" ;;
      blocked) has_blocked="true" ;;
      done_with_concerns)
        violation "bubbles/workflows.yaml outcomeStates still exposes done_with_concerns as a new terminal delivery status; move it to legacy read-only metadata"
        ;;
      *)
        violation "bubbles/workflows.yaml outcomeStates exposes unexpected terminal delivery status '$key' (allowed: done, blocked)"
        ;;
    esac
  done <<< "$keys"

  [[ "$has_done" == "true" ]] || violation "bubbles/workflows.yaml outcomeStates is missing required terminal status done"
  [[ "$has_blocked" == "true" ]] || violation "bubbles/workflows.yaml outcomeStates is missing required terminal status blocked"
}

line_is_legacy_or_forbidden() {
  local line_lc="$1"
  case "$line_lc" in
    *legacy*|*read-only*|*read\ only*|*migrat*|*forbidden*|*deprecated*|*not\ valid*|*not\ permitted*|*must\ not*|*cannot*|*no\ new*|*old\ specs*|*until\ touched*|*compatib*|*reject*|*block*|*g092*) return 0 ;;
    *) return 1 ;;
  esac
}

line_is_detection_code() {
  local line_lc="$1"
  case "$line_lc" in
    *'"done_with_concerns"'*) return 0 ;;
    *"'done_with_concerns'"*) return 0 ;;
    *proposedstatus*|*targetstatus*|*transitionrequests*|*dependency_status*|*assert_*|*write_state*) return 0 ;;
    *) return 1 ;;
  esac
}

check_text_file() {
  local file="$1"
  local rel detection_context findings finding
  [[ -f "$file" ]] || return 0
  text_files_scanned=$((text_files_scanned + 1))
  rel="$(relative_path "$file")"
  detection_context="false"
  if [[ "$rel" == bubbles/scripts/* || "$rel" == .github/bubbles/scripts/* || "$rel" == tests/regression/* ]]; then
    detection_context="true"
  fi

  findings="$(awk -v rel="$rel" -v detection_context="$detection_context" '
    BEGIN { quote = sprintf("%c", 39) }
    {
      line = tolower($0)
      if (index(line, "done_with_concerns") == 0 && index(line, "done with concerns") == 0) {
        next
      }

      if (line ~ /legacy|read-only|read only|migrat|forbidden|deprecated|not valid|not permitted|must not|cannot|no new|new writes are forbidden|old specs|until touched|compatib|reject|block|g092/) {
        next
      }

      if (detection_context == "true") {
        if (index(line, "\"done_with_concerns\"") > 0 || index(line, quote "done_with_concerns" quote) > 0) {
          next
        }
        if (line ~ /proposedstatus|targetstatus|transitionrequests|dependency_status|assert_|write_state/) {
          next
        }
      }

      printf "%s:%d permits or describes active done_with_concerns behavior without marking it legacy-read-only/FORBIDDEN\n", rel, NR
    }
  ' "$file")"

  while IFS= read -r finding; do
    [[ -n "$finding" ]] || continue
    violation "$finding"
  done <<< "$findings"
}

check_active_text_contracts() {
  local target_rels=(
    "agents/bubbles_shared/completion-governance.md"
    "agents/bubbles.validate.agent.md"
    "agents/bubbles.workflow.agent.md"
    "agents/bubbles.audit.agent.md"
    "agents/bubbles.harden.agent.md"
    "agents/bubbles.gaps.agent.md"
    "agents/bubbles.retro.agent.md"
    "agents/bubbles.spec-review.agent.md"
    "bubbles/scripts/post-cert-spec-edit-guard.sh"
    "bubbles/scripts/post-cert-spec-edit-guard-selftest.sh"
    "bubbles/scripts/inter-spec-dependency-guard.sh"
    "bubbles/scripts/inter-spec-dependency-guard-selftest.sh"
    "tests/regression/test_11_post_cert_spec_edit.sh"
    "tests/regression/test_12_inter_spec_dependency.sh"
  )

  local rel target
  for rel in "${target_rels[@]}"; do
    target="$(resolve_repo_file "$rel" || true)"
    [[ -n "$target" ]] || continue
    check_text_file "$target"
  done
}

check_current_state
check_workflow_outcome_states
check_active_text_contracts

if [[ "$finding_count" -gt 0 ]]; then
  echo "G092 strict_terminal_status_gate blocked: findings=$finding_count spec=$spec_rel legacyReadOnlyStatuses=$legacy_read_only_count textFilesScanned=$text_files_scanned" >&2
  exit 1
fi

if [[ "$QUIET" != "true" ]]; then
  if [[ "$legacy_read_only_count" -gt 0 ]]; then
    echo "strict-terminal-status-guard: PASS Gate G092 (strict_terminal_status_gate) - spec=$spec_rel terminalStatuses=done,blocked legacyReadOnlyStatus=present textFilesScanned=$text_files_scanned"
  else
    echo "strict-terminal-status-guard: PASS Gate G092 (strict_terminal_status_gate) - spec=$spec_rel terminalStatuses=done,blocked observations=non-status textFilesScanned=$text_files_scanned"
  fi
fi

exit 0