#!/usr/bin/env bash
# goal-contract.sh — the immutable Goal Contract (IMP-038 SCOPE-1 / GF-1).
# ---------------------------------------------------------------------------
# One versioned, frozen contract per mutable top-level run (bubbles.goal,
# bubbles.sprint, bubbles.iterate, and mutable bubbles.workflow). It records
# what the operator actually asked for, so a run that keeps perfect PROCESS
# fidelity can still be caught losing OUTCOME fidelity.
#
# The contract lives at `.goalContract` in the session file
# (`.specify/memory/bubbles.session.json`). Only its stable id, revision, and
# digest are mirrored into a spec's `state.json.execution.goalContractRef`
# (IMP-038 R5: contract fields must not inflate every prompt).
#
# Shape authority: bubbles/schemas/goal-contract.schema.json. The validation
# below mirrors that schema mechanically so `verify` needs nothing but jq.
#
# Subcommands
#   freeze         Create revision 1 and freeze it. REFUSES if one already exists.
#   read           Print the stored contract, or one field.
#   verify         Validate the stored contract and check caller expectations.
#   revise         Create revision N+1. REFUSES without an operator approval note.
#   mirror         Write only {goalId, revision, sourceRequestDigest} into a state.json.
#   sync-boundary  Write .workBoundary into a state.json from the contract, so the
#                  spec and the contract cannot disagree (IMP-038 SCOPE-2 / GF-2).
#                  REFUSES a widening.
#
# Exit codes (closed set)
#   0  success / match
#   1  contract invalid, or a supplied expectation did not match
#   2  usage or runtime error (missing input, missing jq, unreadable file)
#   3  REFUSED — re-freezing an existing contract, revising without approval, or a
#      sync-boundary that would WIDEN a spec's already-declared boundary
#   4  no contract present in the session file
#
# There is no --force / --skip / --ignore. A second freeze is the silent-revision
# defect this script exists to prevent; the only way forward is `revise`.
set -euo pipefail

SCHEMA_VERSION="goal-contract/v1"
SCHEMA_VERSION_V2="goal-contract/v2"

# --- v2 semantic boundary (IMP-041 SCOPE-1) ---------------------------------
# The v1 boundary is PATH-shaped: it answers "may this goal touch that file?".
# It cannot answer "may this goal build a new runner?", so a goal could expand
# from a bounded test into a platform while every path stayed in-boundary. The
# semantic boundary is the second, shape-shaped layer. Both are closed enums so
# an undeclared expansion is a refusal rather than an unrecognised free string.
EXECUTION_SHAPES="one-off existing-capability-change reusable-capability"
CHANGE_CLASSES="existing-config existing-test new-product-code new-shared-library new-workflow new-runner new-virtual-machine new-daemon new-init-unit new-datastore new-cache new-approval-authority new-network-topology new-deployment-target"
DELTA_BUDGET_KEYS="maxNewScopes maxNewFiles maxNewWorkflows maxNewServices maxNewRunners maxNewVirtualMachines"

usage() {
  cat <<'EOF'
Usage: goal-contract.sh <subcommand> [options]

  freeze  --session-file <path> --source-request-file <path>
          --intent <s> --success-signal <s>
          [--failure-condition <s>]
          [--hard-constraint <s>]... [--non-goal <s>]...
          --target <kind>=<value> [--target <kind>=<value>]...
          --repository-root <slug> [--repository-root <slug>]...
          [--spec-target <s>]... [--allowed-path <s>]...
          [--cross-repo-policy <forbidden|authorized>]
          --runner <agent> --session-id <id> --repository-alias <alias>

          Freezes revision 1 (approval.state=auto-frozen, supersedes=null) and
          prints it. REFUSES (exit 3) if a contract already exists — use revise.
          <kind> is one of: repository, spec, path, release-phase, ops-packet.

  read    --session-file <path> [--field <jq-path>]
          Prints the whole contract, or one field (e.g. --field .intent).
          Exit 4 when no contract is present.

  verify  --session-file <path> [--expect-goal-id <id>]
          [--expect-revision <n>] [--expect-digest <sha256:...>]
          Validates the stored contract against the schema constraints and any
          supplied expectation. Exit 1 names the mismatched field.

  revise  --session-file <path> --approval-note <s>
          [--source-request-file <path>] [same content flags as freeze]
          [--runner <agent>] [--repository-alias <alias>]

          Creates revision N+1, sets supersedes to the prior goalId, and sets
          approval.state=operator-approved. REFUSES (exit 3) without
          --approval-note. Any content flag not supplied is carried forward
          from the prior revision. Supplying ANY --repository-root /
          --spec-target / --allowed-path REPLACES that whole list.
          The stored note is prefixed `widened: `, `narrowed: `, or
          `unchanged: ` according to the boundary comparison.

  mirror  --session-file <path> --state-file <path>
          Additively writes .execution.goalContractRef = {goalId, revision,
          sourceRequestDigest}. Never copies intent/successSignal/constraints,
          and never drops an existing .execution key.

  sync-boundary --session-file <path> --state-file <path>
          Additively writes .workBoundary from the contract's workBoundary, so
          the enforced boundary and the frozen contract cannot disagree. Seeds an
          absent boundary; NARROWING an existing one is free (a planner may
          narrow without approval); WIDENING is REFUSED (exit 3) — revise the
          Goal Contract instead. A refused sync leaves state.json untouched.

  ref     --session-file <path>
          Prints the canonical goalRef block for a transition payload:
          {goalId, revision, sourceRequestDigest, workBoundary}. This is the
          ONLY sanctioned producer — never hand-author a goalRef.

  verify-ref --session-file <path> --ref-file <path> [--require-boundary]
          Validates a goalRef carried by a dispatch packet, RESULT-ENVELOPE,
          route-required payload, ledger entry, snapshot, compacted record, or
          resume packet. Exit 1 when the ref omits a required field, when any
          field differs from the stored contract (digest/revision/goalId
          substitution), or when --require-boundary is given and the ref's
          workBoundary is absent or WIDER than the contract's.

  -h, --help   Print this usage.
EOF
}

# --- diagnostics -----------------------------------------------------------

fail_usage() { echo "goal-contract: $*" >&2; exit 2; }
fail_refuse() { echo "goal-contract: REFUSED — $*" >&2; exit 3; }
fail_absent() { echo "goal-contract: $*" >&2; exit 4; }
fail_invalid() { echo "goal-contract: $*" >&2; exit 1; }

require_jq() {
  if ! command -v jq >/dev/null 2>&1; then
    echo "goal-contract: jq is required but not found in PATH." >&2
    exit 2
  fi
}

# --- portable sha256 -------------------------------------------------------
# macOS ships shasum, not sha256sum. Fail loudly rather than degrading to a
# weaker digest: a Goal Contract with no trustworthy digest is worthless.
sha256_file() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$path" | awk '{print $NF}'
  else
    echo "goal-contract: sha256sum, shasum, or openssl is required to digest the source request" >&2
    exit 2
  fi
}

# --- JSON helpers ----------------------------------------------------------

# json_array <values...> — a JSON array of strings; [] when no values.
json_array() {
  jq -n '$ARGS.positional' --args ${1+"$@"}
}

read_json_file() {
  local path="$1" label="$2"
  [[ -f "$path" ]] || fail_usage "$label not found: $path"
  jq empty "$path" >/dev/null 2>&1 || fail_usage "$label is not valid JSON: $path"
}

# atomic_session_write <destination-file> <new-full-json>
# mktemp + mv inside the destination directory, matching state-snapshot.sh.
# Path-generic despite the name: sync-boundary uses it for a spec state.json.
atomic_session_write() {
  local session_file="$1" payload="$2" dir tmp
  dir="$(cd "$(dirname "$session_file")" && pwd)"
  tmp="$(mktemp "$dir/.goal-contract.update.XXXXXX")"
  printf '%s\n' "$payload" > "$tmp"
  mv "$tmp" "$session_file"
}

get_contract() {
  local session_file="$1"
  read_json_file "$session_file" "session file"
  if [[ "$(jq -r 'has("goalContract") and (.goalContract != null)' "$session_file")" != "true" ]]; then
    fail_absent "no Goal Contract at .goalContract in $session_file"
  fi
  jq -c '.goalContract' "$session_file"
}

# --- schema-mirroring validation ------------------------------------------
# Emits one line per violation on stdout; empty output means valid.
contract_violations() {
  local contract="$1"
  # The three enum expansions below are deliberately unquoted: json_array takes
  # one argument per member, and these constants are fixed literals in this file.
  # shellcheck disable=SC2086
  jq -n -r --argjson c "$contract" --arg sv "$SCHEMA_VERSION" --arg sv2 "$SCHEMA_VERSION_V2" \
    --argjson shapes "$(json_array $EXECUTION_SHAPES)" \
    --argjson classes "$(json_array $CHANGE_CLASSES)" \
    --argjson budgetkeys "$(json_array $DELTA_BUDGET_KEYS)" '
    def nes: type == "string" and length > 0;
    def nesarr: type == "array" and all(.[]; nes);
    def show: if . == null then "null" else tostring end;

    [
      (if ($c | type) != "object" then "contract must be an object" else empty end),

      (if ($c.schemaVersion != $sv) and ($c.schemaVersion != $sv2)
       then "schemaVersion must be \"\($sv)\" or \"\($sv2)\" (observed: \($c.schemaVersion | show))"
       else empty end),

      (if ($c.goalId | nes | not) or (($c.goalId // "") | test("^gc:[A-Za-z0-9._-]+:[0-9]+$") | not)
       then "goalId must match ^gc:<sessionId>:<revision>$ (observed: \($c.goalId | show))"
       else empty end),

      (if ($c.revision | type) != "number" or ($c.revision != ($c.revision | floor)) or ($c.revision < 1)
       then "revision must be an integer >= 1 (observed: \($c.revision | show))"
       else empty end),

      (if (($c.goalId // "") | test("^gc:[A-Za-z0-9._-]+:[0-9]+$"))
          and (($c.goalId | split(":") | last) != ($c.revision | show))
       then "goalId revision segment must equal revision (observed goalId: \($c.goalId | show), revision: \($c.revision | show))"
       else empty end),

      (if (($c.goalId // "") | test("^gc:[A-Za-z0-9._-]+:[0-9]+$"))
          and (($c.provenance.sessionId // "") | nes)
          and (($c.goalId | split(":"))[1] != $c.provenance.sessionId)
       then "goalId session segment must equal provenance.sessionId (observed goalId: \($c.goalId | show), sessionId: \($c.provenance.sessionId | show))"
       else empty end),

      (if ($c.sourceRequestDigest | nes | not) or (($c.sourceRequestDigest // "") | test("^sha256:[0-9a-f]{64}$") | not)
       then "sourceRequestDigest must match ^sha256:<64 hex>$ (observed: \($c.sourceRequestDigest | show))"
       else empty end),

      (if ($c.intent | nes | not) then "intent must be a non-empty string" else empty end),
      (if ($c.successSignal | nes | not) then "successSignal must be a non-empty string" else empty end),
      (if ($c | has("failureCondition")) and ($c.failureCondition | nes | not)
       then "failureCondition must be a non-empty string when present" else empty end),

      (if ($c.hardConstraints | nesarr | not)
       then "hardConstraints must be an array of non-empty strings" else empty end),
      (if ($c.nonGoals | nesarr | not)
       then "nonGoals must be an array of non-empty strings" else empty end),

      (if ($c.targetReferences | type) != "array" or (($c.targetReferences // []) | length) < 1
       then "targetReferences must be a non-empty array"
       else ( $c.targetReferences
              | to_entries[]
              | select(
                  (.value | type) != "object"
                  or ((.value | keys_unsorted | sort) != ["kind","value"])
                  or ((.value.kind // "") | IN("repository","spec","path","release-phase","ops-packet") | not)
                  or ((.value.value // null) | nes | not))
              | "targetReferences[\(.key)] must be {kind:<repository|spec|path|release-phase|ops-packet>, value:<non-empty string>} (observed: \(.value | tojson))" )
       end),

      (if ($c.workBoundary | type) != "object"
       then "workBoundary must be an object"
       else (
         (if ($c.workBoundary.repositoryRoots | nesarr | not) or (($c.workBoundary.repositoryRoots // []) | length) < 1
          then "workBoundary.repositoryRoots must be a non-empty array of non-empty strings" else empty end),
         (if ($c.workBoundary | has("specTargets")) and ($c.workBoundary.specTargets | nesarr | not)
          then "workBoundary.specTargets must be an array of non-empty strings" else empty end),
         (if ($c.workBoundary | has("allowedPaths")) and ($c.workBoundary.allowedPaths | nesarr | not)
          then "workBoundary.allowedPaths must be an array of non-empty strings" else empty end),
         (if ($c.workBoundary | has("crossRepoPolicy")) and (($c.workBoundary.crossRepoPolicy // "") | IN("forbidden","authorized") | not)
          then "workBoundary.crossRepoPolicy must be \"forbidden\" or \"authorized\" (observed: \($c.workBoundary.crossRepoPolicy | show))" else empty end),
         (($c.workBoundary | keys_unsorted[]
           | select(IN("repositoryRoots","specTargets","allowedPaths","crossRepoPolicy") | not)
           | "workBoundary has an unknown key: \(.)"))
       ) end),

      (if $c.schemaVersion == $sv2
       then (
         if ($c.semanticBoundary | type) != "object"
         then "semanticBoundary must be an object in \($sv2)"
         else (
           (if ($c.semanticBoundary.executionShape // "") | IN($shapes[]) | not
            then "semanticBoundary.executionShape must be one of \($shapes | join(", ")) (observed: \($c.semanticBoundary.executionShape | show))"
            else empty end),
           (if ($c.semanticBoundary.allowedChangeClasses | type) != "array"
            then "semanticBoundary.allowedChangeClasses must be an array"
            else ($c.semanticBoundary.allowedChangeClasses[]
                  | select(IN($classes[]) | not)
                  | "semanticBoundary.allowedChangeClasses has an unknown change class: \(. | show)") end),
           (if ($c.semanticBoundary.approvalRequiredChangeClasses | type) != "array"
            then "semanticBoundary.approvalRequiredChangeClasses must be an array"
            else ($c.semanticBoundary.approvalRequiredChangeClasses[]
                  | select(IN($classes[]) | not)
                  | "semanticBoundary.approvalRequiredChangeClasses has an unknown change class: \(. | show)") end),
           ( (($c.semanticBoundary.allowedChangeClasses // []) as $a
              | ($c.semanticBoundary.approvalRequiredChangeClasses // []) as $r
              | ($a - ($a - $r))) as $overlap
             | if ($overlap | type) == "array" and ($overlap | length) > 0
               then "semanticBoundary.allowedChangeClasses and approvalRequiredChangeClasses must not overlap (both: \($overlap | join(", ")))"
               else empty end),
           (if ($c.semanticBoundary.deltaBudget | type) != "object"
            then "semanticBoundary.deltaBudget must be an object"
            else (
              ($c.semanticBoundary.deltaBudget | to_entries[]
               | select(.key | IN($budgetkeys[]) | not)
               | "semanticBoundary.deltaBudget has an unknown key: \(.key)"),
              ($c.semanticBoundary.deltaBudget | to_entries[]
               | select(((.value | type) != "number") or (.value != (.value | floor)) or (.value < 0))
               | "semanticBoundary.deltaBudget.\(.key) must be a non-negative integer (observed: \(.value | show))")
            ) end),
           (($c.semanticBoundary | keys_unsorted[]
             | select(IN("executionShape","allowedChangeClasses","approvalRequiredChangeClasses","deltaBudget") | not)
             | "semanticBoundary has an unknown key: \(.)"))
         ) end )
       elif ($c | has("semanticBoundary"))
       then "semanticBoundary requires schemaVersion \"\($sv2)\" (observed: \($c.schemaVersion | show))"
       else empty end),

      (if ($c.createdAt // "") | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$") | not
       then "createdAt must be RFC3339 UTC (YYYY-MM-DDThh:mm:ssZ) (observed: \($c.createdAt | show))"
       else empty end),

      (if ($c.provenance | type) != "object"
       then "provenance must be an object"
       else (
         (($c.provenance | keys_unsorted | sort) as $k
          | if $k != ["repositoryAlias","runner","sessionId"]
            then "provenance must have exactly runner, sessionId, repositoryAlias (observed: \($k | tojson))" else empty end),
         (if ($c.provenance.runner | nes | not) then "provenance.runner must be a non-empty string" else empty end),
         (if ($c.provenance.sessionId | nes | not) then "provenance.sessionId must be a non-empty string" else empty end),
         (if ($c.provenance.repositoryAlias | nes | not) then "provenance.repositoryAlias must be a non-empty string" else empty end)
       ) end),

      (if ($c.approval | type) != "object"
       then "approval must be an object"
       else (
         (($c.approval | keys_unsorted | sort) as $k
          | if $k != ["approvalNote","approvedAt","state"]
            then "approval must have exactly state, approvedAt, approvalNote (observed: \($k | tojson))" else empty end),
         (if ($c.approval.state // "") | IN("auto-frozen","operator-approved","pending-expansion") | not
          then "approval.state must be auto-frozen, operator-approved, or pending-expansion (observed: \($c.approval.state | show))" else empty end),
         (if ($c.approval.approvedAt != null) and ($c.approval.approvedAt | type) != "string"
          then "approval.approvedAt must be a string or null" else empty end),
         (if ($c.approval.approvalNote != null) and ($c.approval.approvalNote | type) != "string"
          then "approval.approvalNote must be a string or null" else empty end)
       ) end),

      (if ($c | has("supersedes") | not)
       then "supersedes is required (null for revision 1)"
       elif ($c.supersedes != null) and (($c.supersedes | type) != "string" or (($c.supersedes // "") | test("^gc:[A-Za-z0-9._-]+:[0-9]+$") | not))
       then "supersedes must be a goalId or null (observed: \($c.supersedes | show))"
       elif ($c.revision == 1) and ($c.supersedes != null)
       then "revision 1 must have supersedes=null (observed: \($c.supersedes | show))"
       elif (($c.revision // 0) > 1) and ($c.supersedes == null)
       then "revision \($c.revision | show) must name the prior goalId in supersedes"
       else empty end),

      ( ["schemaVersion","goalId","revision","sourceRequestDigest","intent","successSignal",
         "hardConstraints","failureCondition","nonGoals","targetReferences","workBoundary",
         "semanticBoundary",
         "createdAt","provenance","approval","supersedes"] as $known
        | $c | keys_unsorted[] | select(. as $k | $known | index($k) | not)
        | "contract has an unknown key: \(.)" )
    ] | .[]
  '
}

assert_valid_contract() {
  local contract="$1" violations
  # The validator dereferences object fields, so a non-object must be rejected
  # here rather than thrown from inside jq.
  if [[ "$(jq -r 'type' <<< "$contract")" != "object" ]]; then
    echo "goal-contract: stored contract is invalid:" >&2
    echo "  - contract must be an object" >&2
    exit 1
  fi
  violations="$(contract_violations "$contract")"
  if [[ -n "$violations" ]]; then
    echo "goal-contract: stored contract is invalid:" >&2
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      echo "  - $line" >&2
    done <<< "$violations"
    exit 1
  fi
}

# --- shared content-flag parsing ------------------------------------------

reset_content_flags() {
  session_file=""
  source_request_file=""
  intent=""
  success_signal=""
  failure_condition=""
  approval_note=""
  runner=""
  session_id=""
  repository_alias=""
  cross_repo_policy=""
  state_file=""
  ref_file=""
  require_boundary="false"
  field=""
  expect_goal_id=""
  expect_revision=""
  expect_digest=""
  hard_constraints=()
  non_goals=()
  target_kinds=()
  target_values=()
  repository_roots=()
  spec_targets=()
  allowed_paths=()
  execution_shape=""
  allowed_change_classes=()
  approval_change_classes=()
  delta_budget_keys=()
  delta_budget_values=()
}

# A list entry that is empty, or a target kind outside the enum, is a caller
# error (exit 2) — catch it at the flag rather than as an invalid contract.
require_nonempty() {
  [[ -n "${2:-}" ]] || fail_usage "$1 requires a non-empty value"
}

# Word-boundary membership against a space-separated enum. Written with a case
# glob rather than an array scan so it stays bash-3.2 clean on macOS.
in_enum() {
  case " $2 " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
  esac
}

# Emits the v2 semanticBoundary object, or nothing when no semantic flag was
# supplied (which is what keeps a v1 freeze byte-identical to today's).
build_semantic_json() {
  [[ -n "$execution_shape" ]] || return 0
  local allowed approval budget i
  allowed="$(json_array ${allowed_change_classes[@]+"${allowed_change_classes[@]}"})"
  approval="$(json_array ${approval_change_classes[@]+"${approval_change_classes[@]}"})"
  budget='{}'
  i=0
  while [[ "$i" -lt "${#delta_budget_keys[@]}" ]]; do
    budget="$(jq -c --arg k "${delta_budget_keys[$i]}" --argjson v "${delta_budget_values[$i]}" \
      '. + {($k): $v}' <<< "$budget")"
    i=$((i + 1))
  done
  jq -n --arg shape "$execution_shape" --argjson a "$allowed" --argjson r "$approval" --argjson b "$budget" \
    '{ executionShape: $shape, allowedChangeClasses: ($a | unique), approvalRequiredChangeClasses: ($r | unique), deltaBudget: $b }'
}

# Classifies a semantic boundary transition the way classify_boundary_change
# classifies a path one, so the stored approval note records WHICH direction the
# operator approved. Shape rank is ordered because promoting one-off work to a
# reusable capability is the exact expansion IMP-041 exists to surface.
classify_semantic_change() {
  local prior="$1" new="$2"
  jq -n -r --argjson o "${prior:-null}" --argjson n "${new:-null}" '
    def rank: { "one-off": 0, "existing-capability-change": 1, "reusable-capability": 2 }[.] // -1;
    def budget_grew($a; $b): [ $b | to_entries[] | select(.value > (($a[.key]) // 0)) ] | length > 0;
    if $o == null and $n == null then "semantic-absent"
    elif $o == null then "semantic-added"
    elif $n == null then "semantic-removed"
    elif $o == $n then "semantic-unchanged"
    else
      ((($n.allowedChangeClasses // []) - ($o.allowedChangeClasses // [])) | length > 0) as $classes_added
      | (($n.executionShape | rank) > ($o.executionShape | rank)) as $shape_promoted
      | (budget_grew(($o.deltaBudget // {}); ($n.deltaBudget // {}))) as $budget_up
      | if $classes_added or $shape_promoted or $budget_up then "semantic-widened"
        else "semantic-narrowed" end
    end'
}

parse_flags() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session-file) session_file="${2:-}"; shift 2 ;;
      --source-request-file) source_request_file="${2:-}"; shift 2 ;;
      --intent) intent="${2:-}"; shift 2 ;;
      --success-signal) success_signal="${2:-}"; shift 2 ;;
      --failure-condition) failure_condition="${2:-}"; shift 2 ;;
      --hard-constraint) require_nonempty "$1" "${2:-}"; hard_constraints+=("$2"); shift 2 ;;
      --non-goal) require_nonempty "$1" "${2:-}"; non_goals+=("$2"); shift 2 ;;
      --target)
        local spec="${2:-}"
        [[ "$spec" == *"="* ]] || fail_usage "--target must be <kind>=<value> (observed: $spec)"
        case "${spec%%=*}" in
          repository|spec|path|release-phase|ops-packet) ;;
          *) fail_usage "--target kind must be repository, spec, path, release-phase, or ops-packet (observed: ${spec%%=*})" ;;
        esac
        [[ -n "${spec#*=}" ]] || fail_usage "--target requires a non-empty value (observed: $spec)"
        target_kinds+=("${spec%%=*}")
        target_values+=("${spec#*=}")
        shift 2 ;;
      --repository-root) require_nonempty "$1" "${2:-}"; repository_roots+=("$2"); shift 2 ;;
      --spec-target) require_nonempty "$1" "${2:-}"; spec_targets+=("$2"); shift 2 ;;
      --allowed-path) require_nonempty "$1" "${2:-}"; allowed_paths+=("$2"); shift 2 ;;
      --cross-repo-policy) cross_repo_policy="${2:-}"; shift 2 ;;
      --execution-shape)
        require_nonempty "$1" "${2:-}"
        in_enum "$2" "$EXECUTION_SHAPES" ||
          fail_usage "--execution-shape must be one of: $EXECUTION_SHAPES (observed: $2)"
        execution_shape="$2"; shift 2 ;;
      --allow-change-class)
        require_nonempty "$1" "${2:-}"
        in_enum "$2" "$CHANGE_CLASSES" ||
          fail_usage "--allow-change-class must be one of: $CHANGE_CLASSES (observed: $2)"
        allowed_change_classes+=("$2"); shift 2 ;;
      --approval-change-class)
        require_nonempty "$1" "${2:-}"
        in_enum "$2" "$CHANGE_CLASSES" ||
          fail_usage "--approval-change-class must be one of: $CHANGE_CLASSES (observed: $2)"
        approval_change_classes+=("$2"); shift 2 ;;
      --delta-budget)
        local budget="${2:-}"
        [[ "$budget" == *"="* ]] || fail_usage "--delta-budget must be <key>=<count> (observed: $budget)"
        in_enum "${budget%%=*}" "$DELTA_BUDGET_KEYS" ||
          fail_usage "--delta-budget key must be one of: $DELTA_BUDGET_KEYS (observed: ${budget%%=*})"
        case "${budget#*=}" in
          ''|*[!0-9]*) fail_usage "--delta-budget count must be a non-negative integer (observed: ${budget#*=})" ;;
        esac
        delta_budget_keys+=("${budget%%=*}")
        delta_budget_values+=("${budget#*=}")
        shift 2 ;;
      --runner) runner="${2:-}"; shift 2 ;;
      --session-id) session_id="${2:-}"; shift 2 ;;
      --repository-alias) repository_alias="${2:-}"; shift 2 ;;
      --approval-note) approval_note="${2:-}"; shift 2 ;;
      --state-file) state_file="${2:-}"; shift 2 ;;
      --ref-file) ref_file="${2:-}"; shift 2 ;;
      --require-boundary) require_boundary="true"; shift ;;
      --field) field="${2:-}"; shift 2 ;;
      --expect-goal-id) expect_goal_id="${2:-}"; shift 2 ;;
      --expect-revision) expect_revision="${2:-}"; shift 2 ;;
      --expect-digest) expect_digest="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) fail_usage "unknown option: $1" ;;
    esac
  done
}

build_targets_json() {
  local kinds values
  kinds="$(json_array ${target_kinds[@]+"${target_kinds[@]}"})"
  values="$(json_array ${target_values[@]+"${target_values[@]}"})"
  jq -n --argjson k "$kinds" --argjson v "$values" \
    '[range(0; $k | length) | { kind: $k[.], value: $v[.] }]'
}

build_boundary_json() {
  local roots specs paths policy="$1"
  roots="$(json_array ${repository_roots[@]+"${repository_roots[@]}"})"
  specs="$(json_array ${spec_targets[@]+"${spec_targets[@]}"})"
  paths="$(json_array ${allowed_paths[@]+"${allowed_paths[@]}"})"
  jq -n --argjson r "$roots" --argjson s "$specs" --argjson p "$paths" --arg policy "$policy" '
    { repositoryRoots: $r, crossRepoPolicy: $policy }
    + (if ($s | length) > 0 then { specTargets: $s } else {} end)
    + (if ($p | length) > 0 then { allowedPaths: $p } else {} end)
  '
}

# classify_boundary_change <old-boundary-json> <new-boundary-json>
#
# "Widened" means the new boundary grants reach the old one did not. That is a
# COVERAGE question, not a set-difference one, because two dimensions carry
# patterns rather than exact identifiers:
#
#   allowedPaths  `prefix/**`, `dir/`, or an exact path
#   specTargets   an exact target, or any target with the same basename
#
# Naive set difference reads `bubbles/scripts/**` -> `bubbles/scripts/x.sh` as
# an ADDITION and refuses it as a widening, even though it is strictly less
# reach. That false refusal blocks the narrowing a planner is explicitly
# allowed to perform without approval, so coverage is the correct test.
#
# The matching rules below MIRROR work-boundary-resolve.sh. The selftest drives
# one shared table through BOTH implementations, so neither can drift alone.
#
# Absence is deliberately NOT read as "unrestricted". A boundary with no
# allowedPaths cannot authorize a mutation at all (`--require-allowed-paths`
# refuses it), so declaring the first path list GRANTS concrete mutation reach
# where none was usable — that stays a widening.
#
# An addition on any dimension (or forbidden -> authorized) outranks a
# simultaneous removal: a mixed change still grants new reach.
classify_boundary_change() {
  jq -n -r --argjson o "$1" --argjson n "$2" '
    def basename_of: sub("/$"; "") | split("/") | last;

    # covers_path($pat; $cand) — work-boundary-resolve.sh path dimension.
    def covers_path($pat; $cand):
      if ($pat | endswith("/**")) then
        ($pat | rtrimstr("/**")) as $prefix
        | ($cand == $prefix) or ($cand | startswith($prefix + "/"))
      elif ($pat | endswith("/")) then
        $cand | startswith($pat)
      else
        $cand == $pat
      end;

    # covers_spec($pat; $cand) — work-boundary-resolve.sh spec dimension.
    def covers_spec($pat; $cand):
      ($pat == $cand) or (($pat | basename_of) == ($cand | basename_of));

    def old(k): ($o[k] // []) | unique;
    def new(k): ($n[k] // []) | unique;

    # uncovered(candidates; patterns; rule) — entries the other side cannot reach.
    def uncovered($cands; $pats; $kind):
      [ $cands[]
        | . as $c
        | select(
            [ $pats[] | . as $p
              | if $kind == "path" then covers_path($p; $c)
                elif $kind == "spec" then covers_spec($p; $c)
                else ($p == $c) end
            ] | any | not)
      ] | length;

    (uncovered(new("repositoryRoots"); old("repositoryRoots"); "exact")
      + uncovered(new("specTargets"); old("specTargets"); "spec")
      + uncovered(new("allowedPaths"); old("allowedPaths"); "path")) as $add
    | (uncovered(old("repositoryRoots"); new("repositoryRoots"); "exact")
      + uncovered(old("specTargets"); new("specTargets"); "spec")
      + uncovered(old("allowedPaths"); new("allowedPaths"); "path")) as $rem
    | (($o.crossRepoPolicy // "forbidden")) as $op
    | (($n.crossRepoPolicy // "forbidden")) as $np
    | (if $op == "forbidden" and $np == "authorized" then 1 else 0 end) as $polWiden
    | (if $op == "authorized" and $np == "forbidden" then 1 else 0 end) as $polNarrow
    | if ($add + $polWiden) > 0 then "widened"
      elif ($rem + $polNarrow) > 0 then "narrowed"
      else "unchanged" end
  '
}

# --- subcommands -----------------------------------------------------------

cmd_freeze() {
  parse_flags "$@"

  [[ -n "$session_file" ]] || fail_usage "freeze requires --session-file"
  [[ -n "$source_request_file" ]] || fail_usage "freeze requires --source-request-file"
  [[ -f "$source_request_file" ]] || fail_usage "source request file not found: $source_request_file"
  [[ -n "$intent" ]] || fail_usage "freeze requires --intent"
  [[ -n "$success_signal" ]] || fail_usage "freeze requires --success-signal"
  [[ -n "$runner" ]] || fail_usage "freeze requires --runner"
  [[ -n "$session_id" ]] || fail_usage "freeze requires --session-id"
  [[ -n "$repository_alias" ]] || fail_usage "freeze requires --repository-alias"
  [[ "${#target_kinds[@]}" -gt 0 ]] || fail_usage "freeze requires at least one --target <kind>=<value>"
  [[ "${#repository_roots[@]}" -gt 0 ]] || fail_usage "freeze requires at least one --repository-root"

  # goalId embeds the session id between colons, so the id itself must not
  # carry one. Refuse rather than emit an unparseable goalId.
  case "$session_id" in
    *[!A-Za-z0-9._-]*) fail_usage "--session-id must match ^[A-Za-z0-9._-]+$ (a ':' would make goalId ambiguous): $session_id" ;;
  esac

  local policy="${cross_repo_policy:-forbidden}"
  case "$policy" in
    forbidden|authorized) ;;
    *) fail_usage "--cross-repo-policy must be 'forbidden' or 'authorized' (observed: $policy)" ;;
  esac

  local session_dir
  session_dir="$(dirname "$session_file")"
  mkdir -p "$session_dir"
  if [[ ! -f "$session_file" ]]; then
    printf '{}\n' > "$session_file"
  fi
  read_json_file "$session_file" "session file"

  if [[ "$(jq -r 'has("goalContract") and (.goalContract != null)' "$session_file")" == "true" ]]; then
    fail_refuse "a Goal Contract is already frozen at .goalContract in $session_file ($(jq -r '.goalContract.goalId // "unknown"' "$session_file")). Re-freezing would silently replace the operator's outcome — use 'revise --approval-note' instead."
  fi

  local digest targets boundary contract created_at semantic schema
  digest="sha256:$(sha256_file "$source_request_file")"
  targets="$(build_targets_json)"
  boundary="$(build_boundary_json "$policy")"
  semantic="$(build_semantic_json)"
  created_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

  # A semantic detail with no --execution-shape would be silently dropped, which
  # is the failure mode this scope exists to remove. Refuse instead.
  if [[ -z "$semantic" ]] &&
    { [[ "${#allowed_change_classes[@]}" -gt 0 ]] ||
      [[ "${#approval_change_classes[@]}" -gt 0 ]] ||
      [[ "${#delta_budget_keys[@]}" -gt 0 ]]; }; then
    fail_usage "--allow-change-class, --approval-change-class and --delta-budget require --execution-shape (they describe a semantic boundary that would otherwise be dropped)"
  fi

  if [[ -n "$semantic" ]]; then schema="$SCHEMA_VERSION_V2"; else schema="$SCHEMA_VERSION"; fi

  contract="$(jq -n \
    --arg sv "$schema" \
    --arg goal_id "gc:${session_id}:1" \
    --arg digest "$digest" \
    --arg intent "$intent" \
    --arg success "$success_signal" \
    --arg failure "$failure_condition" \
    --argjson hard "$(json_array ${hard_constraints[@]+"${hard_constraints[@]}"})" \
    --argjson nongoals "$(json_array ${non_goals[@]+"${non_goals[@]}"})" \
    --argjson targets "$targets" \
    --argjson boundary "$boundary" \
    --argjson semantic "${semantic:-null}" \
    --arg created "$created_at" \
    --arg runner "$runner" \
    --arg session_id "$session_id" \
    --arg alias "$repository_alias" \
    '{
      schemaVersion: $sv,
      goalId: $goal_id,
      revision: 1,
      sourceRequestDigest: $digest,
      intent: $intent,
      successSignal: $success,
      hardConstraints: $hard,
      nonGoals: $nongoals,
      targetReferences: $targets,
      workBoundary: $boundary,
      createdAt: $created,
      provenance: { runner: $runner, sessionId: $session_id, repositoryAlias: $alias },
      approval: { state: "auto-frozen", approvedAt: null, approvalNote: null },
      supersedes: null
    }
    + (if $failure == "" then {} else { failureCondition: $failure } end)
    + (if $semantic == null then {} else { semanticBoundary: $semantic } end)')"

  assert_valid_contract "$contract"

  atomic_session_write "$session_file" \
    "$(jq --argjson gc "$contract" '. + { goalContract: $gc }' "$session_file")"

  jq -n --argjson gc "$contract" '$gc'
}

cmd_read() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "read requires --session-file"

  local contract
  contract="$(get_contract "$session_file")"

  if [[ -n "$field" ]]; then
    jq -n -r --argjson gc "$contract" "\$gc | $field" 2>/dev/null \
      || fail_usage "--field is not a valid jq path: $field"
    return
  fi
  jq -n --argjson gc "$contract" '$gc'
}

cmd_verify() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "verify requires --session-file"

  local contract
  contract="$(get_contract "$session_file")"
  assert_valid_contract "$contract"

  local observed
  if [[ -n "$expect_goal_id" ]]; then
    observed="$(jq -r '.goalId' <<< "$contract")"
    [[ "$observed" == "$expect_goal_id" ]] \
      || fail_invalid "goalId mismatch: observed '$observed', expected '$expect_goal_id'"
  fi
  if [[ -n "$expect_revision" ]]; then
    observed="$(jq -r '.revision' <<< "$contract")"
    [[ "$observed" == "$expect_revision" ]] \
      || fail_invalid "revision mismatch: observed '$observed', expected '$expect_revision'"
  fi
  if [[ -n "$expect_digest" ]]; then
    observed="$(jq -r '.sourceRequestDigest' <<< "$contract")"
    [[ "$observed" == "$expect_digest" ]] \
      || fail_invalid "sourceRequestDigest mismatch: observed '$observed', expected '$expect_digest'"
  fi

  echo "goal-contract: verified $(jq -r '.goalId' <<< "$contract") revision $(jq -r '.revision' <<< "$contract")"
}

cmd_revise() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "revise requires --session-file"

  local prior
  prior="$(get_contract "$session_file")"
  assert_valid_contract "$prior"

  if [[ -z "$approval_note" ]]; then
    fail_refuse "revise requires --approval-note. An expansion of intent, success criteria, targets, or boundary must not mutate the frozen contract without recorded operator approval."
  fi

  # Content carries forward unless a flag explicitly replaces it. sessionId is
  # the identity anchor goalId encodes, so it is never overridable here.
  local prior_session_id prior_digest new_digest new_revision
  prior_session_id="$(jq -r '.provenance.sessionId' <<< "$prior")"
  prior_digest="$(jq -r '.sourceRequestDigest' <<< "$prior")"
  new_revision="$(jq -r '.revision + 1' <<< "$prior")"

  if [[ -n "$source_request_file" ]]; then
    [[ -f "$source_request_file" ]] || fail_usage "source request file not found: $source_request_file"
    new_digest="sha256:$(sha256_file "$source_request_file")"
  else
    new_digest="$prior_digest"
  fi

  local new_intent new_success new_failure new_runner new_alias
  new_intent="${intent:-$(jq -r '.intent' <<< "$prior")}"
  new_success="${success_signal:-$(jq -r '.successSignal' <<< "$prior")}"
  new_failure="${failure_condition:-$(jq -r '.failureCondition // ""' <<< "$prior")}"
  new_runner="${runner:-$(jq -r '.provenance.runner' <<< "$prior")}"
  new_alias="${repository_alias:-$(jq -r '.provenance.repositoryAlias' <<< "$prior")}"

  local new_hard new_nongoals new_targets
  if [[ "${#hard_constraints[@]}" -gt 0 ]]; then
    new_hard="$(json_array "${hard_constraints[@]}")"
  else
    new_hard="$(jq -c '.hardConstraints' <<< "$prior")"
  fi
  if [[ "${#non_goals[@]}" -gt 0 ]]; then
    new_nongoals="$(json_array "${non_goals[@]}")"
  else
    new_nongoals="$(jq -c '.nonGoals' <<< "$prior")"
  fi
  if [[ "${#target_kinds[@]}" -gt 0 ]]; then
    new_targets="$(build_targets_json)"
  else
    new_targets="$(jq -c '.targetReferences' <<< "$prior")"
  fi

  # Boundary: any dimension with no supplied flag carries forward unchanged.
  local prior_boundary new_boundary policy
  prior_boundary="$(jq -c '.workBoundary' <<< "$prior")"
  policy="${cross_repo_policy:-$(jq -r '.workBoundary.crossRepoPolicy // "forbidden"' <<< "$prior")}"
  case "$policy" in
    forbidden|authorized) ;;
    *) fail_usage "--cross-repo-policy must be 'forbidden' or 'authorized' (observed: $policy)" ;;
  esac
  if [[ "${#repository_roots[@]}" -eq 0 ]]; then
    while IFS= read -r entry; do
      [[ -n "$entry" ]] && repository_roots+=("$entry")
    done < <(jq -r '.workBoundary.repositoryRoots[]' <<< "$prior")
  fi
  if [[ "${#spec_targets[@]}" -eq 0 ]]; then
    while IFS= read -r entry; do
      [[ -n "$entry" ]] && spec_targets+=("$entry")
    done < <(jq -r '.workBoundary.specTargets // [] | .[]' <<< "$prior")
  fi
  if [[ "${#allowed_paths[@]}" -eq 0 ]]; then
    while IFS= read -r entry; do
      [[ -n "$entry" ]] && allowed_paths+=("$entry")
    done < <(jq -r '.workBoundary.allowedPaths // [] | .[]' <<< "$prior")
  fi
  new_boundary="$(build_boundary_json "$policy")"

  # Semantic boundary: replaced only when --execution-shape is supplied, else it
  # carries forward untouched. A revise that forgot the flag must not silently
  # drop the operator's declared shape.
  local prior_semantic new_semantic semantic_change schema
  prior_semantic="$(jq -c '.semanticBoundary // null' <<< "$prior")"
  if [[ -n "$execution_shape" ]]; then
    new_semantic="$(build_semantic_json)"
  elif [[ "$prior_semantic" != "null" ]]; then
    new_semantic="$prior_semantic"
  else
    new_semantic=""
  fi
  semantic_change="$(classify_semantic_change "$prior_semantic" "${new_semantic:-null}")"
  if [[ -n "$new_semantic" ]]; then schema="$SCHEMA_VERSION_V2"; else schema="$SCHEMA_VERSION"; fi

  local change stored_note created_at contract
  change="$(classify_boundary_change "$prior_boundary" "$new_boundary")"
  # v1 contracts keep the exact historical "<change>: <note>" prefix; the
  # semantic term is appended only once a semantic boundary actually exists.
  if [[ "$semantic_change" == "semantic-absent" ]]; then
    stored_note="${change}: ${approval_note}"
  else
    stored_note="${change}/${semantic_change}: ${approval_note}"
  fi
  created_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

  contract="$(jq -n \
    --arg sv "$schema" \
    --arg goal_id "gc:${prior_session_id}:${new_revision}" \
    --argjson revision "$new_revision" \
    --arg digest "$new_digest" \
    --arg intent "$new_intent" \
    --arg success "$new_success" \
    --arg failure "$new_failure" \
    --argjson hard "$new_hard" \
    --argjson nongoals "$new_nongoals" \
    --argjson targets "$new_targets" \
    --argjson boundary "$new_boundary" \
    --argjson semantic "${new_semantic:-null}" \
    --arg created "$created_at" \
    --arg runner "$new_runner" \
    --arg session_id "$prior_session_id" \
    --arg alias "$new_alias" \
    --arg note "$stored_note" \
    --arg supersedes "$(jq -r '.goalId' <<< "$prior")" \
    '{
      schemaVersion: $sv,
      goalId: $goal_id,
      revision: $revision,
      sourceRequestDigest: $digest,
      intent: $intent,
      successSignal: $success,
      hardConstraints: $hard,
      nonGoals: $nongoals,
      targetReferences: $targets,
      workBoundary: $boundary,
      createdAt: $created,
      provenance: { runner: $runner, sessionId: $session_id, repositoryAlias: $alias },
      approval: { state: "operator-approved", approvedAt: $created, approvalNote: $note },
      supersedes: $supersedes
    }
    + (if $failure == "" then {} else { failureCondition: $failure } end)
    + (if $semantic == null then {} else { semanticBoundary: $semantic } end)')"

  assert_valid_contract "$contract"

  atomic_session_write "$session_file" \
    "$(jq --argjson gc "$contract" '. + { goalContract: $gc }' "$session_file")"

  jq -n --argjson gc "$contract" '$gc'
}

cmd_mirror() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "mirror requires --session-file"
  [[ -n "$state_file" ]] || fail_usage "mirror requires --state-file"

  local contract
  contract="$(get_contract "$session_file")"
  assert_valid_contract "$contract"

  read_json_file "$state_file" "state file"

  local ref state_dir tmp
  ref="$(jq -c '{ goalId, revision, sourceRequestDigest }' <<< "$contract")"
  state_dir="$(cd "$(dirname "$state_file")" && pwd)"
  tmp="$(mktemp "$state_dir/.goal-contract.state.XXXXXX")"
  jq --argjson ref "$ref" \
    '. + { execution: ((.execution // {}) + { goalContractRef: $ref }) }' \
    "$state_file" > "$tmp"
  mv "$tmp" "$state_file"

  jq -n --argjson ref "$ref" '$ref'
}

# cmd_sync_boundary — carry the frozen contract's workBoundary into a spec's
# state.json (IMP-038 SCOPE-2 / GF-2), so the boundary the resolver enforces and
# the boundary the operator approved are one fact, not two.
#
# Direction rule, decided by the SHARED classify_boundary_change() so there is
# exactly one comparator in this file:
#   absent    seed it
#   unchanged idempotent rewrite
#   narrowed  write \u2014 a planner may shrink reach without approval
#   widened   REFUSE (exit 3) \u2014 granting a spec MORE reach than it already
#             declares is an expansion, and an expansion belongs in `revise`
#             where it carries a recorded operator approval note.
cmd_sync_boundary() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "sync-boundary requires --session-file"
  [[ -n "$state_file" ]] || fail_usage "sync-boundary requires --state-file"

  local contract
  contract="$(get_contract "$session_file")"
  assert_valid_contract "$contract"

  read_json_file "$state_file" "state file"

  local boundary existing change
  boundary="$(jq -c '.workBoundary' <<< "$contract")"

  if [[ "$(jq -r 'has("workBoundary") and (.workBoundary != null)' "$state_file")" == "true" ]]; then
    if [[ "$(jq -r '.workBoundary | type' "$state_file")" != "object" ]]; then
      fail_usage "existing .workBoundary in $state_file is not an object — repair it before syncing"
    fi
    existing="$(jq -c '.workBoundary' "$state_file")"
    change="$(classify_boundary_change "$existing" "$boundary")"
    if [[ "$change" == "widened" ]]; then
      fail_refuse "the Goal Contract's workBoundary grants MORE reach than $state_file already declares. Narrowing is free; widening a spec's declared reach is an expansion — record it with 'goal-contract.sh revise --approval-note ...' and then widen $state_file's workBoundary deliberately. sync-boundary will not widen a spec for you. state.json was NOT modified."
    fi
  fi

  atomic_session_write "$state_file" \
    "$(jq --argjson wb "$boundary" '. + { workBoundary: $wb }' "$state_file")"

  jq -n --argjson wb "$boundary" '$wb'
}

# cmd_ref — the ONE sanctioned producer of a transition goalRef (IMP-038
# SCOPE-3 / GF-1, GF-5). Every surface that must carry goal identity —
# dispatch packets, RESULT-ENVELOPE and route-required payloads, invocation
# ledgers, turn and convergence snapshots, compacted history, continuation and
# resume packets, validation and audit attempts — emits THIS block rather than
# hand-authoring one. A hand-authored ref is exactly how a substituted digest
# enters the system unnoticed.
#
# The boundary IS included here, unlike `mirror`. The two serve different
# readers: `mirror` writes a durable spec-local pointer and deliberately stays
# minimal (R5 — contract fields must not inflate every prompt), while a
# transition ref is consumed by a verifier that must also confirm the work
# stayed inside the declared reach.
cmd_ref() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "ref requires --session-file"

  local contract
  contract="$(get_contract "$session_file")"
  assert_valid_contract "$contract"

  jq -c '{
    goalId: .goalId,
    revision: .revision,
    sourceRequestDigest: .sourceRequestDigest,
    workBoundary: .workBoundary
  }
  + (if has("semanticBoundary") then { semanticBoundary: .semanticBoundary } else {} end)' <<< "$contract"
}

# cmd_verify_ref — the ONE comparator for a goalRef arriving from any
# transition. It refuses three distinct failures that all look like success to
# a reader who only eyeballs the payload:
#
#   omission     a field is absent, so nothing was actually asserted
#   substitution a field is present but disagrees with the frozen contract
#   widening     the ref claims MORE reach than the contract grants
#
# Narrowing is accepted: a specialist legitimately reports back a subset of the
# boundary it was given. Widening is not, because a specialist cannot grant
# itself reach the operator never approved.
cmd_verify_ref() {
  parse_flags "$@"
  [[ -n "$session_file" ]] || fail_usage "verify-ref requires --session-file"
  [[ -n "$ref_file" ]] || fail_usage "verify-ref requires --ref-file"

  local contract
  contract="$(get_contract "$session_file")"
  assert_valid_contract "$contract"

  read_json_file "$ref_file" "ref file"
  local ref
  ref="$(jq -c '.' "$ref_file")"
  [[ "$(jq -r 'type' <<< "$ref")" == "object" ]] \
    || fail_invalid "goalRef must be an object (observed $(jq -r 'type' <<< "$ref"))"

  local key observed expected
  for key in goalId revision sourceRequestDigest; do
    if [[ "$(jq -r --arg k "$key" 'has($k) and (.[$k] != null)' <<< "$ref")" != "true" ]]; then
      fail_invalid "goalRef omits $key. A transition that carries no goal identity cannot be traced to the frozen contract — emit it with 'goal-contract.sh ref'."
    fi
    observed="$(jq -r --arg k "$key" '.[$k] | tostring' <<< "$ref")"
    expected="$(jq -r --arg k "$key" '.[$k] | tostring' <<< "$contract")"
    if [[ "$observed" != "$expected" ]]; then
      fail_invalid "goalRef $key mismatch: observed '$observed', frozen contract holds '$expected'. A substituted $key silently re-points the work at a different goal."
    fi
  done

  if [[ "$require_boundary" == "true" ]]; then
    if [[ "$(jq -r 'has("workBoundary") and (.workBoundary != null)' <<< "$ref")" != "true" ]]; then
      fail_invalid "goalRef omits workBoundary, but --require-boundary was given. A mutable transition must state the reach it operated within."
    fi
    if [[ "$(jq -r '.workBoundary | type' <<< "$ref")" != "object" ]]; then
      fail_invalid "goalRef.workBoundary must be an object"
    fi
    local change
    change="$(classify_boundary_change "$(jq -c '.workBoundary' <<< "$contract")" "$(jq -c '.workBoundary' <<< "$ref")")"
    if [[ "$change" == "widened" ]]; then
      fail_invalid "goalRef.workBoundary claims MORE reach than the frozen contract grants. A specialist may report a narrowed boundary; it may not grant itself an expansion — that requires 'goal-contract.sh revise --approval-note'."
    fi
  fi

  echo "goal-contract: goalRef verified against $(jq -r '.goalId' <<< "$contract") revision $(jq -r '.revision' <<< "$contract")"
}

# --- dispatch --------------------------------------------------------------

main() {
  local subcommand="${1:-}"
  if [[ -z "$subcommand" ]]; then
    usage >&2
    exit 2
  fi
  if [[ "$subcommand" == "-h" || "$subcommand" == "--help" ]]; then
    usage
    exit 0
  fi
  shift

  require_jq
  reset_content_flags

  case "$subcommand" in
    freeze) cmd_freeze "$@" ;;
    read) cmd_read "$@" ;;
    verify) cmd_verify "$@" ;;
    revise) cmd_revise "$@" ;;
    mirror) cmd_mirror "$@" ;;
    sync-boundary) cmd_sync_boundary "$@" ;;
    ref) cmd_ref "$@" ;;
    verify-ref) cmd_verify_ref "$@" ;;
    *) fail_usage "unknown subcommand: $subcommand" ;;
  esac
}

main "$@"
