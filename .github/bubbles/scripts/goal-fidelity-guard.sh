#!/usr/bin/env bash
# Gate G134: goal_fidelity_gate  (IMP-038 SCOPE-6 / GF-1, GF-2, GF-3, GF-6)
#
# Every other goal-fidelity control in IMP-038 is a CAPABILITY: goal-contract.sh
# can freeze and verify, work-boundary-resolve.sh can refuse an undeclared
# boundary, the envelope validator can refuse dishonest finding accounting. None
# of them is reached unless a runner chooses to call it. This guard is what makes
# the capabilities load-bearing — it checks the boundaries where drift actually
# enters, so a run that simply never called them fails here instead of passing.
#
# Six boundaries, each independently selectable so a caller checks the one it is
# standing on rather than paying for all six:
#
#   --boundary pre-planning     Contract complete, frozen, bound to a repository
#   --boundary post-planning    Every active requirement/scope traces to it
#   --boundary pre-dispatch     Candidate work in-boundary, carrying the digest
#   --boundary post-finding     No out-of-boundary path changed in this packet
#   --boundary post-compaction  Digest and boundary survived compaction/resume
#   --boundary pre-certification Evidence shows the success signal; constraints held
#   --boundary all              Run every boundary applicable to the inputs given
#
# G070 REPAIR (same scope). G070 declared `enforcedBy: [ unbound ]` while its
# description claimed artifact-lint.sh performed the presence check. It does not
# — grep finds no Outcome Contract logic there — so for ordinary feature work
# BOTH the goal-to-spec link and the spec-to-implementation link were unenforced.
# `--boundary pre-certification` (and `post-planning`) now performs that real
# presence check on spec.md, which is what lets G070's registry entry name a
# concrete enforcer instead of `unbound`.
#
# Exit codes (closed set)
#   0  every selected boundary held
#   1  a boundary failed — the findings are printed, one per line
#   2  usage error, missing jq, or an unreadable input
#
# There is NO --force / --skip / --ignore. A boundary that cannot be satisfied is
# a real refusal: narrow the work, or record an operator-approved expansion with
# `goal-contract.sh revise --approval-note`.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOAL_CONTRACT="$SCRIPT_DIR/goal-contract.sh"
BOUNDARY_RESOLVER="$SCRIPT_DIR/work-boundary-resolve.sh"

usage() {
  cat <<'EOF'
Usage: goal-fidelity-guard.sh --boundary <name> [inputs...]

Boundaries:
  pre-planning       --session-file <path>
                     Contract exists, is schema-valid, is frozen (approval.state
                     set), and names a repository via workBoundary.repositoryRoots.

  post-planning      --session-file <path> --spec-dir <dir>
                     spec.md carries a non-empty Outcome Contract, state.json
                     declares a workBoundary, and every scope traces to the goal.

  pre-dispatch       --session-file <path> --spec-dir <dir> --candidate-repo <slug>
                     [--candidate-spec <id>] [--candidate-path <path>]
                     [--ref-file <path>] [--mutable]
                     Candidate work resolves in-boundary; a supplied ref carries
                     the current digest. --mutable requires a declared allowedPaths.

  post-finding       --session-file <path> --spec-dir <dir>
                     --changed-path <path> [--changed-path <path>]...
                     No changed path fell outside the declared boundary.

  post-compaction    --session-file <path> --ref-file <path>
                     The record's goalRef still matches the frozen contract.

  pre-certification  --session-file <path> --spec-dir <dir>
                     spec.md has an Outcome Contract; report.md demonstrates the
                     success signal; every hard constraint is addressed; the
                     certification is not stale against a newer contract revision.

  all                Runs every boundary whose required inputs were supplied.

Common:
  --session-file <path>   .specify/memory/bubbles.session.json (or equivalent)
  --spec-dir <dir>        The feature directory under specs/
  -h, --help              Print this usage.

No --force / --skip / --ignore exists.
EOF
}

boundary=""
session_file=""
spec_dir=""
candidate_repo=""
candidate_spec=""
candidate_path=""
ref_file=""
mutable="false"
changed_paths=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --boundary) boundary="${2:-}"; shift 2 ;;
    --session-file) session_file="${2:-}"; shift 2 ;;
    --spec-dir) spec_dir="${2:-}"; shift 2 ;;
    --candidate-repo) candidate_repo="${2:-}"; shift 2 ;;
    --candidate-spec) candidate_spec="${2:-}"; shift 2 ;;
    --candidate-path) candidate_path="${2:-}"; shift 2 ;;
    --changed-path)
      [[ -n "${2:-}" ]] || { echo "goal-fidelity-guard: --changed-path requires a value" >&2; exit 2; }
      changed_paths+=("$2"); shift 2 ;;
    --ref-file) ref_file="${2:-}"; shift 2 ;;
    --mutable) mutable="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "goal-fidelity-guard: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

fail_usage() { echo "goal-fidelity-guard: $*" >&2; exit 2; }

[[ -n "$boundary" ]] || fail_usage "--boundary is required"
case "$boundary" in
  pre-planning|post-planning|pre-dispatch|post-finding|post-compaction|pre-certification|all) ;;
  *) fail_usage "unknown boundary: $boundary" ;;
esac
command -v jq >/dev/null 2>&1 || fail_usage "jq is required"
[[ -f "$GOAL_CONTRACT" ]] || fail_usage "goal-contract.sh not found at $GOAL_CONTRACT"

FINDINGS=0
finding() { echo "GOAL-FIDELITY[$1] $2"; FINDINGS=$((FINDINGS + 1)); }

selected() { [[ "$boundary" == "all" || "$boundary" == "$1" ]]; }

require_session() {
  [[ -n "$session_file" ]] || fail_usage "$boundary requires --session-file"
  [[ -f "$session_file" ]] || fail_usage "session file not found: $session_file"
}

require_spec_dir() {
  [[ -n "$spec_dir" ]] || fail_usage "$boundary requires --spec-dir"
  [[ -d "$spec_dir" ]] || fail_usage "spec dir not found: $spec_dir"
}

# contract_json — the frozen contract, or empty when absent/invalid.
contract_json() {
  bash "$GOAL_CONTRACT" read --session-file "$session_file" 2>/dev/null
}

# ---------------------------------------------------------------------------
# section_body <file> <heading-regex>
# Prints the lines under a markdown heading, stopping at the next heading of the
# same or higher level. Used for Outcome Contract presence, where "present" must
# mean "has content" — an empty section is the exact shape that let a spec claim
# an outcome it never stated.
# ---------------------------------------------------------------------------
section_body() {
  local file="$1" pattern="$2"
  [[ -f "$file" ]] || return 0
  awk -v pat="$pattern" '
    $0 ~ pat { insec = 1; next }
    insec && /^#{1,3}[[:space:]]/ { insec = 0 }
    insec { print }
  ' "$file"
}

outcome_contract_findings() {
  local spec_md="$spec_dir/spec.md" body
  if [[ ! -f "$spec_md" ]]; then
    finding "G070" "no spec.md in $spec_dir — the Outcome Contract cannot exist"
    return
  fi
  body="$(section_body "$spec_md" '^#{1,3}[[:space:]]+Outcome Contract')"
  if [[ -z "$(tr -d '[:space:]' <<< "$body")" ]]; then
    finding "G070" "$spec_md has no non-empty '## Outcome Contract' section. G070 requires Intent, Success Signal, Hard Constraints, and Failure Condition BEFORE bootstrap completes; without it there is no statement of what this feature was for."
    return
  fi
  local field
  for field in "Intent" "Success Signal"; do
    if ! grep -qiE "^[[:space:]]*[-*]?[[:space:]]*(\*\*)?${field}(\*\*)?[[:space:]]*:" <<< "$body"; then
      finding "G070" "$spec_md Outcome Contract declares no '$field'. A contract missing its $field cannot be demonstrated at certification."
    fi
  done
}

# ---------------------------------------------------------------------------
# Boundary 1 — before planning.
# ---------------------------------------------------------------------------
check_pre_planning() {
  require_session
  local contract rc=0
  contract="$(contract_json)" || rc=$?
  if [[ -z "$contract" ]]; then
    finding "GF-1" "no Goal Contract at .goalContract in $session_file. Planning MUST NOT start against an unfrozen goal — freeze it with 'goal-contract.sh freeze'."
    return
  fi
  if ! bash "$GOAL_CONTRACT" verify --session-file "$session_file" >/dev/null 2>&1; then
    finding "GF-1" "the stored Goal Contract is invalid: $(bash "$GOAL_CONTRACT" verify --session-file "$session_file" 2>&1 | tail -3 | tr '\n' ' ')"
    return
  fi
  local approval
  approval="$(jq -r '.approval.state // empty' <<< "$contract")"
  if [[ -z "$approval" ]]; then
    finding "GF-1" "the Goal Contract records no approval.state, so it was never frozen. An unfrozen contract can be edited to match whatever the run did."
  fi
  local roots
  roots="$(jq -r '(.workBoundary.repositoryRoots // []) | length' <<< "$contract")"
  if [[ "$roots" == "0" ]]; then
    finding "GF-2" "the Goal Contract declares no repositoryRoots, so the run is not bound to any repository decision."
  fi
}

# ---------------------------------------------------------------------------
# Boundary 2 — after planning.
# ---------------------------------------------------------------------------
check_post_planning() {
  require_session
  require_spec_dir
  outcome_contract_findings

  local state="$spec_dir/state.json"
  if [[ ! -f "$state" ]]; then
    finding "GF-2" "no state.json in $spec_dir — planning produced no enforceable work boundary."
    return
  fi
  if [[ "$(jq -r 'has("workBoundary") and (.workBoundary != null)' "$state" 2>/dev/null)" != "true" ]]; then
    finding "GF-2" "$state declares no workBoundary. Derive it from the frozen contract with 'goal-contract.sh sync-boundary' — planning that leaves reach undeclared is unbounded."
  fi
  if [[ "$(jq -r '.execution.goalContractRef // empty' "$state" 2>/dev/null)" == "" ]]; then
    finding "GF-1" "$state carries no execution.goalContractRef. Run 'goal-contract.sh mirror' so the spec points back at the goal it came from."
  else
    local spec_goal contract_goal
    spec_goal="$(jq -r '.execution.goalContractRef.goalId // empty' "$state")"
    contract_goal="$(bash "$GOAL_CONTRACT" read --session-file "$session_file" --field .goalId 2>/dev/null)"
    if [[ -n "$contract_goal" && "$spec_goal" != "$contract_goal" ]]; then
      finding "GF-1" "$state points at goal '$spec_goal' but the session holds '$contract_goal'. Stale planning must be routed back to its owner before mutable work resumes."
    fi
  fi
}

# ---------------------------------------------------------------------------
# Boundary 3 — before dispatch.
# ---------------------------------------------------------------------------
check_pre_dispatch() {
  require_session
  require_spec_dir
  [[ -n "$candidate_repo" ]] || fail_usage "pre-dispatch requires --candidate-repo"
  [[ -f "$BOUNDARY_RESOLVER" ]] || fail_usage "work-boundary-resolve.sh not found"

  local args=(--feature-dir "$spec_dir" --candidate-repo "$candidate_repo" --strict)
  [[ -n "$candidate_spec" ]] && args+=(--candidate-spec "$candidate_spec")
  [[ -n "$candidate_path" ]] && args+=(--candidate-path "$candidate_path")
  [[ "$mutable" == "true" ]] && args+=(--require-allowed-paths)

  local out rc=0
  out="$(bash "$BOUNDARY_RESOLVER" "${args[@]}" 2>&1)" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    finding "GF-2" "the candidate work was refused by the boundary resolver (exit $rc): $(tr '\n' ' ' <<< "$out")"
    return
  fi
  local disposition
  disposition="$(sed -n 's/^disposition=//p' <<< "$out")"
  if [[ "$disposition" != "in-boundary" ]]; then
    finding "GF-2" "candidate work resolves '$disposition', not in-boundary. Route it to its owner instead of dispatching it inside this packet."
  fi

  if [[ -n "$ref_file" ]]; then
    local vargs=(verify-ref --session-file "$session_file" --ref-file "$ref_file")
    [[ "$mutable" == "true" ]] && vargs+=(--require-boundary)
    local vout vrc=0
    vout="$(bash "$GOAL_CONTRACT" "${vargs[@]}" 2>&1)" || vrc=$?
    if [[ "$vrc" -ne 0 ]]; then
      finding "GF-1" "the dispatch packet's goalRef did not verify (exit $vrc): $(tr '\n' ' ' <<< "$vout")"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Boundary 4 — after finding handling.
# ---------------------------------------------------------------------------
check_post_finding() {
  require_session
  require_spec_dir
  [[ "${#changed_paths[@]}" -gt 0 ]] || fail_usage "post-finding requires at least one --changed-path"
  [[ -f "$BOUNDARY_RESOLVER" ]] || fail_usage "work-boundary-resolve.sh not found"

  local repo
  repo="$(bash "$GOAL_CONTRACT" read --session-file "$session_file" --field '.workBoundary.repositoryRoots[0]' 2>/dev/null)"
  [[ -n "$repo" ]] || fail_usage "the Goal Contract declares no repositoryRoots"

  local p out rc disposition
  for p in "${changed_paths[@]}"; do
    rc=0
    out="$(bash "$BOUNDARY_RESOLVER" --feature-dir "$spec_dir" --candidate-repo "$repo" \
      --candidate-path "$p" --strict 2>&1)" || rc=$?
    if [[ "$rc" -ne 0 ]]; then
      finding "GF-2" "changed path '$p' could not be classified (exit $rc): $(tr '\n' ' ' <<< "$out")"
      continue
    fi
    disposition="$(sed -n 's/^disposition=//p' <<< "$out")"
    if [[ "$disposition" != "in-boundary" ]]; then
      finding "GF-3" "changed path '$p' resolves '$disposition' — out-of-boundary work was performed inside the parent packet. Handling a finding never authorizes editing outside the approved reach; that is an expansion."
    fi
  done
}

# ---------------------------------------------------------------------------
# Boundary 5 — after compaction or resume.
# ---------------------------------------------------------------------------
check_post_compaction() {
  require_session
  [[ -n "$ref_file" ]] || fail_usage "post-compaction requires --ref-file"
  [[ -f "$ref_file" ]] || fail_usage "ref file not found: $ref_file"

  local out rc=0
  out="$(bash "$GOAL_CONTRACT" verify-ref --session-file "$session_file" --ref-file "$ref_file" --require-boundary 2>&1)" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    finding "GF-5" "the resumed goalRef no longer matches the frozen contract (exit $rc): $(tr '\n' ' ' <<< "$out"). Compaction and resume MUST preserve goal identity verbatim; a mismatch means the resumed run is pointed at a different goal or a superseded revision."
  fi
}

# ---------------------------------------------------------------------------
# Boundary 6 — before final certification.
# ---------------------------------------------------------------------------
check_pre_certification() {
  require_session
  require_spec_dir
  outcome_contract_findings

  local spec_md="$spec_dir/spec.md" report_md="$spec_dir/report.md"
  local body signal
  body="$(section_body "$spec_md" '^#{1,3}[[:space:]]+Outcome Contract')"
  signal="$(grep -iE "^[[:space:]]*[-*]?[[:space:]]*(\*\*)?Success Signal(\*\*)?[[:space:]]*:" <<< "$body" | head -1 | sed -E 's/^[^:]*:[[:space:]]*//')"

  if [[ ! -f "$report_md" ]]; then
    finding "GF-6" "no report.md in $spec_dir — certification cannot demonstrate the success signal without evidence."
  elif [[ -n "$signal" ]]; then
    # Presence, not semantics: the substance check stays with bubbles.validate.
    # What is mechanical is whether the report ever REFERS to the declared
    # signal. A report that never mentions it cannot have demonstrated it.
    if ! grep -qiF "Success Signal" "$report_md"; then
      finding "GF-6" "$report_md never references the declared Success Signal. G070 requires the signal to be DEMONSTRATED in evidence, not merely declared in spec.md."
    fi
  fi

  local constraints_line
  constraints_line="$(grep -icE "^[[:space:]]*[-*]?[[:space:]]*(\*\*)?Hard Constraints?(\*\*)?[[:space:]]*:" <<< "$body")"
  if [[ "$constraints_line" == "0" ]]; then
    finding "G070" "$spec_md Outcome Contract declares no 'Hard Constraints'. Certification cannot claim constraints were preserved when none were stated."
  fi

  local state="$spec_dir/state.json"
  if [[ -f "$state" ]]; then
    local spec_rev contract_rev
    spec_rev="$(jq -r '.execution.goalContractRef.revision // empty' "$state" 2>/dev/null)"
    contract_rev="$(bash "$GOAL_CONTRACT" read --session-file "$session_file" --field .revision 2>/dev/null)"
    if [[ -n "$spec_rev" && -n "$contract_rev" && "$spec_rev" != "$contract_rev" ]]; then
      finding "GF-1" "$state certifies against Goal Contract revision $spec_rev while the session holds revision $contract_rev. A contract revision invalidates planning and certification claims that depended on the prior digest — route those artifacts back to their owners before certifying."
    fi
  fi
}

selected pre-planning      && check_pre_planning
selected post-planning     && [[ -n "$spec_dir" ]] && check_post_planning
selected pre-dispatch      && [[ -n "$candidate_repo" ]] && check_pre_dispatch
selected post-finding      && [[ "${#changed_paths[@]}" -gt 0 ]] && check_post_finding
selected post-compaction   && [[ -n "$ref_file" ]] && check_post_compaction
selected pre-certification && [[ -n "$spec_dir" ]] && check_pre_certification

if [[ "$FINDINGS" -gt 0 ]]; then
  echo "goal-fidelity-guard: FAIL boundary=$boundary findings=$FINDINGS"
  exit 1
fi
echo "goal-fidelity-guard: PASS boundary=$boundary"
exit 0
