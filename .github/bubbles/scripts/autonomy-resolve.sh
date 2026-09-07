#!/usr/bin/env bash
#
# autonomy-resolve.sh — resolve the effective autonomy posture for a run.
#
# The posture governs INTERACTION (whether the run pauses to ask), never
# VERIFICATION. No posture waives a gate, an evidence standard, or a status
# ceiling; see agents/bubbles_shared/critical-requirements.md -> "Autonomy Floor".
#
# Precedence, highest first:
#   1. Per-run directive   (--autonomy <v>, or --directive "<str>" with autonomy:<v>)
#   2. Environment         (BUBBLES_AUTONOMY)
#   3. Project config      (.github/bubbles-project.yaml `autonomy:`)
#   4. Framework default   (full)
#
# Layers 2 and 3 are what make the posture survive a session restart: neither
# lives in the prompt, so an interrupted run resumes at the same posture without
# the operator re-asserting it.
#
# Parser dependency: `yq` (mikefarah v4) for the config layer ONLY. A missing yq
# WARNs and skips that layer; the directive, env, and default layers still
# resolve. This mirrors bubbles/scripts/adversarial-resolve.sh.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist.
#
# Exit codes:
#   0  resolved (including yq-missing config skip)
#   1  invalid value
#   2  usage error (unknown flag / missing required option value)

set -euo pipefail

DEFAULT_AUTONOMY="full"
VALID_AUTONOMY="full guarded interactive unattended"

REPO_ROOT_ARG=""
DIR_AUTONOMY=""
DIRECTIVE_STR=""
AUTONOMY_FLAG_SEEN=0
DIRECTIVE_FLAG_SEEN=0
FORMAT="env"
SESSION_BUDGET_ARG=""
SESSION_ID=""
SESSION_CONTROL_FILE=""
BINDING_PACKET_FILE=""
SCENARIO_FILE=""
NODE_ID=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/autonomy-resolve.sh [--autonomy <v>] [--directive "<str>"] [--repo-root <dir>] [--session-budget <s>] [--format env|json]

Resolves the effective autonomy posture. Precedence chain: per-run directive ->
BUBBLES_AUTONOMY env -> bubbles-project.yaml `autonomy:` -> framework default (full).

Options:
  --autonomy <full|guarded|interactive|unattended>
                            Per-run posture (directive layer, highest precedence).
  --directive "<str>"       Free-form per-run string; an `autonomy:<value>` token
                            is extracted from it. Typically $ADDITIONAL_CONTEXT.
                            An explicit --autonomy overrides the token inside
                            --directive.
  --repo-root <dir>         Repo root to scan for bubbles-project.yaml
                            (default: the repo containing this script).
  --session-budget <bounded|unbounded>
                            Override the detected session-budget state. Without
                            it the state is read from the exact session policy
                            head after packet validation.
  --session-id <id>         Exact host session for policy selection.
  --session-control-file <path>
                            Host-private authoritative session control record.
  --binding-packet-file <path>
                            Actionable packet whose session and root are used.
  --scenario-file <path>    Optional compiled scenario for a scoped packet.
  --node-id <id>            Optional scenario node. Supply both optional values.
  --format env|json         Output shape (default: env).
  -h, --help                Print this usage and exit 0.

Postures:
  full         grillMode off, socratic false; mechanical decisions auto-resolve,
               taste decisions still surface at each phase boundary, security
               decisions always escalate. This is the default.
  guarded      grillMode required-on-ambiguity plus a conditional clarify gate.
  interactive  grillMode on-demand plus socratic true; human in the loop.
  unattended   Opt-in, never the default. Adds to `full`: interactive questions
               are forbidden, taste-decision overflow auto-resolves and is
               logged, and a `blocked` on an agent-solvable cause requires a
               recorded remediation attempt first. An operator-only blocker
               stays a truthful terminal state. REQUIRES a non-null sessionBudget:
               a run that will not stop on its own forfeits being unbounded.

Exit codes: 0 resolved (incl. yq-missing) | 1 invalid value | 2 usage |
            3 posture/budget inconsistency (E039-UNATTENDED-UNBOUNDED).
There is NO --skip/--force/--ignore bypass. The posture never waives a gate.
EOF
}

die_usage() {
  echo "autonomy-resolve: $1" >&2
  exit 2
}

is_valid() {
  case " $VALID_AUTONOMY " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --autonomy)
      [[ $AUTONOMY_FLAG_SEEN -eq 0 ]] || die_usage "duplicate --autonomy flag is ambiguous"
      AUTONOMY_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || die_usage "--autonomy requires a value"
      DIR_AUTONOMY="$1"
      shift
      ;;
    --directive)
      [[ $DIRECTIVE_FLAG_SEEN -eq 0 ]] || die_usage "duplicate --directive flag is ambiguous"
      DIRECTIVE_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || die_usage "--directive requires a value"
      DIRECTIVE_STR="$1"
      shift
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || die_usage "--repo-root requires a value"
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --session-budget)
      shift
      [[ $# -gt 0 ]] || die_usage "--session-budget requires a value"
      SESSION_BUDGET_ARG="$1"
      shift
      ;;
    --session-id)
      shift
      [[ $# -gt 0 ]] || die_usage "--session-id requires a value"
      SESSION_ID="$1"
      shift
      ;;
    --session-control-file)
      shift
      [[ $# -gt 0 ]] || die_usage "--session-control-file requires a value"
      SESSION_CONTROL_FILE="$1"
      shift
      ;;
    --binding-packet-file)
      shift
      [[ $# -gt 0 ]] || die_usage "--binding-packet-file requires a value"
      BINDING_PACKET_FILE="$1"
      shift
      ;;
    --scenario-file)
      shift
      [[ $# -gt 0 ]] || die_usage "--scenario-file requires a value"
      SCENARIO_FILE="$1"
      shift
      ;;
    --node-id)
      shift
      [[ $# -gt 0 ]] || die_usage "--node-id requires a value"
      NODE_ID="$1"
      shift
      ;;
    --format)
      shift
      [[ $# -gt 0 ]] || die_usage "--format requires a value"
      FORMAT="$1"
      shift
      ;;
    --skip | --force | --ignore | --no-verify)
      echo "autonomy-resolve: $1 is not a supported flag. The posture governs interaction, never verification; there is no bypass." >&2
      exit 2
      ;;
    *)
      die_usage "unknown flag: $1"
      ;;
  esac
done

case "$FORMAT" in
  env | json) ;;
  *) die_usage "--format must be env or json" ;;
esac

case "$SESSION_BUDGET_ARG" in
  "" | bounded | unbounded) ;;
  *) die_usage "--session-budget must be bounded or unbounded" ;;
esac

if [[ -n "$SCENARIO_FILE" && -z "$NODE_ID" ]]; then
  die_usage "--scenario-file requires --node-id"
fi
if [[ -n "$NODE_ID" && -z "$SCENARIO_FILE" ]]; then
  die_usage "--node-id requires --scenario-file"
fi

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"
if [[ -n "$REPO_ROOT_ARG" ]]; then
  REPO_ROOT="$REPO_ROOT_ARG"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd)"
fi

RESOLVED=""
SOURCE_LAYER=""

# --- Layer 1: per-run directive ---
if [[ -z "$RESOLVED" && -n "$DIR_AUTONOMY" ]]; then
  if ! is_valid "$DIR_AUTONOMY"; then
    echo "autonomy-resolve: invalid --autonomy value '$DIR_AUTONOMY' (expected one of: $VALID_AUTONOMY)" >&2
    exit 1
  fi
  RESOLVED="$DIR_AUTONOMY"
  SOURCE_LAYER="directive"
fi

if [[ -z "$RESOLVED" && -n "$DIRECTIVE_STR" ]]; then
  token="$(printf '%s' "$DIRECTIVE_STR" | tr '[:space:]' '\n' | grep -E '^autonomy:' | head -1 || true)"
  if [[ -n "$token" ]]; then
    token="${token#autonomy:}"
    if ! is_valid "$token"; then
      echo "autonomy-resolve: invalid autonomy token '$token' in --directive (expected one of: $VALID_AUTONOMY)" >&2
      exit 1
    fi
    RESOLVED="$token"
    SOURCE_LAYER="directive"
  fi
fi

# --- Layer 2: environment ---
if [[ -z "$RESOLVED" && -n "${BUBBLES_AUTONOMY:-}" ]]; then
  if ! is_valid "$BUBBLES_AUTONOMY"; then
    echo "autonomy-resolve: invalid BUBBLES_AUTONOMY value '$BUBBLES_AUTONOMY' (expected one of: $VALID_AUTONOMY)" >&2
    exit 1
  fi
  RESOLVED="$BUBBLES_AUTONOMY"
  SOURCE_LAYER="env"
fi

# --- Layer 3: project config ---
if [[ -z "$RESOLVED" ]]; then
  config_file=""
  for c in "$REPO_ROOT/.github/bubbles-project.yaml" "$REPO_ROOT/bubbles-project.yaml"; do
    if [[ -f "$c" ]]; then
      config_file="$c"
      break
    fi
  done
  if [[ -n "$config_file" ]]; then
    if command -v yq >/dev/null 2>&1; then
      raw="$(yq '.autonomy' "$config_file" 2>/dev/null || true)"
      if [[ -n "$raw" && "$raw" != "null" ]]; then
        if ! is_valid "$raw"; then
          echo "autonomy-resolve: invalid autonomy value '$raw' in $config_file (expected one of: $VALID_AUTONOMY)" >&2
          exit 1
        fi
        RESOLVED="$raw"
        SOURCE_LAYER="project-config"
      fi
    else
      echo "autonomy-resolve: WARN yq not found — skipping the $config_file config layer." >&2
    fi
  fi
fi

# --- Layer 4: framework default ---
if [[ -z "$RESOLVED" ]]; then
  RESOLVED="$DEFAULT_AUTONOMY"
  SOURCE_LAYER="framework-default"
fi

# `unattended` will not stop on its own, so it forfeits the right to be unbounded.
if [[ "$RESOLVED" == "unattended" ]]; then
  budget_state="$SESSION_BUDGET_ARG"
  if [[ -z "$budget_state" ]]; then
    [[ -n "$SESSION_ID" ]] || die_usage "--session-id is required when unattended boundedness reads session policy"
    [[ -n "$SESSION_CONTROL_FILE" ]] || die_usage "--session-control-file is required when unattended boundedness reads session policy"
    [[ -n "$BINDING_PACKET_FILE" ]] || die_usage "--binding-packet-file is required when unattended boundedness reads session policy"
    REPOSITORY_BINDING="$SCRIPT_DIR/repository-binding.sh"
    [[ -f "$REPOSITORY_BINDING" ]] || {
      echo "autonomy-resolve: repository binding validator is unavailable" >&2
      exit 3
    }
    command -v jq >/dev/null 2>&1 || {
      echo "autonomy-resolve: jq is required for exact-session policy selection" >&2
      exit 3
    }
    VALIDATE_ARGS=(
      validate-packet
      --session-id "$SESSION_ID"
      --session-control-file "$SESSION_CONTROL_FILE"
      --packet-file "$BINDING_PACKET_FILE"
    )
    if [[ -n "$SCENARIO_FILE" ]]; then
      VALIDATE_ARGS+=(--scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID")
    fi
    if ! bash "$REPOSITORY_BINDING" ${VALIDATE_ARGS[@]+"${VALIDATE_ARGS[@]}"} >/dev/null 2>&1; then
      echo "autonomy-resolve: exact-session packet authority is invalid" >&2
      exit 3
    fi
    PACKET_SESSION_ID="$(jq -r '.repositoryResolution.sessionId' "$BINDING_PACKET_FILE" 2>/dev/null || true)"
    PACKET_ROOT="$(jq -r '.repositoryRoot' "$BINDING_PACKET_FILE" 2>/dev/null || true)"
    if [[ "$PACKET_SESSION_ID" != "$SESSION_ID" || "$PACKET_ROOT" != "$REPO_ROOT" ]]; then
      echo "autonomy-resolve: exact-session packet authority does not match the requested root and session" >&2
      exit 3
    fi
    SESSION_STATE_IO="$SCRIPT_DIR/session-state-io.py"
    [[ -f "$SESSION_STATE_IO" ]] || {
      echo "autonomy-resolve: session state I/O helper is unavailable" >&2
      exit 3
    }
    command -v python3 >/dev/null 2>&1 || {
      echo "autonomy-resolve: python3 is required for exact-session policy capture" >&2
      exit 3
    }
    policy_capture_dir="$(mktemp -d)"
    policy_capture="$policy_capture_dir/bubbles.session.json"
    set +e
    python3 "$SESSION_STATE_IO" capture \
      --root "$REPO_ROOT" \
      --relative-path '.specify/memory/bubbles.session.json' \
      --destination "$policy_capture" >/dev/null 2>&1
    capture_rc=$?
    set -e
    if [[ "$capture_rc" -eq 4 ]]; then
      rm -rf "$policy_capture_dir"
      budget_state="unbounded"
    elif [[ "$capture_rc" -ne 0 ]]; then
      rm -rf "$policy_capture_dir"
      echo "autonomy-resolve: E039-SESSION-POLICY-INVALID — exact-session state capture is unsafe or unstable." >&2
      exit 3
    else
      set +e
      policy_state="$(jq -r --arg session "$SESSION_ID" '
        def cap_keys:
          ["schemaVersion", "maxTotalConvergenceIterations", "maxWallClockMinutes",
           "maxToolCalls", "maxSingleToolResultBytes", "maxCumulativeToolResultBytes",
           "maxPromptTokensPerRequest", "maxCumulativePromptTokens"];
        def outer_keys:
          ["recordSchemaVersion", "hostSessionId", "revision", "supersedesRevision",
           "recordedAt", "budget"];
        def valid_timestamp:
          type == "string"
          and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")
          and (try (fromdateiso8601 | type == "number") catch false);
        def valid_budget:
          type == "object"
          and ((keys - cap_keys) | length) == 0
          and .schemaVersion == 1
          and ([cap_keys[1:][] as $key
            | ((has($key) | not) or .[$key] == null
               or ((.[$key] | type) == "number" and (.[$key] | floor) == .[$key] and .[$key] >= 0))]
            | all);
        def valid_record:
          type == "object"
          and ((keys | sort) == (outer_keys | sort))
          and .recordSchemaVersion == 1
          and (.hostSessionId | type) == "string" and (.hostSessionId | length) > 0
          and (.revision | type) == "number" and (.revision | floor) == .revision and .revision > 0
          and (.supersedesRevision == null
               or ((.supersedesRevision | type) == "number"
                   and (.supersedesRevision | floor) == .supersedesRevision
                   and .supersedesRevision > 0
                   and .supersedesRevision < .revision))
          and (.recordedAt | valid_timestamp)
          and (.budget | valid_budget);
        if (.sessionBudgetHistory? == null) then "unbounded"
        elif (.sessionBudgetHistory | type) != "array" then error("invalid-policy-history")
        else
          ([.sessionBudgetHistory[] | select(.hostSessionId == $session)]) as $records
          | if ($records | length) == 0 then "unbounded"
            elif (all($records[]; valid_record) | not) then error("invalid-policy-record")
            elif (($records | map(.revision) | unique | length) != ($records | length)) then error("duplicate-policy-revision")
            elif (($records | map(select(.revision == 1 and .supersedesRevision == null)) | length) != 1) then error("missing-policy-root")
            elif ([$records[] | select(.revision > 1) as $record
                     | (([$records[] | select(.revision == ($record.revision - 1))] | length) != 1
                        or $record.supersedesRevision != ($record.revision - 1))]
                    | any) then error("branching-policy-chain")
            else
              ($records | max_by(.revision) | .budget) as $head
              | if ([cap_keys[1:][] as $key | $head[$key] != null] | any) then "bounded" else "unbounded" end
            end
        end
      ' "$policy_capture" 2>/dev/null)"
      policy_rc=$?
      set -e
      rm -rf "$policy_capture_dir"
      if [[ "$policy_rc" -ne 0 || ( "$policy_state" != "bounded" && "$policy_state" != "unbounded" ) ]]; then
        echo "autonomy-resolve: E039-SESSION-POLICY-INVALID — exact-session policy history is malformed, duplicated, or branched." >&2
        exit 3
      fi
      budget_state="$policy_state"
    fi
  fi
  if [[ "$budget_state" != "bounded" ]]; then
    echo "autonomy-resolve: E039-UNATTENDED-UNBOUNDED — autonomy 'unattended' requires one validated exact-session policy head with a non-null cap. A run that will not stop on its own must be bounded." >&2
    exit 3
  fi
fi

if [[ "$FORMAT" == "json" ]]; then
  printf '{"autonomy":"%s","source":"%s"}\n' "$RESOLVED" "$SOURCE_LAYER"
else
  printf 'BUBBLES_RESOLVED_AUTONOMY=%s\n' "$RESOLVED"
  printf 'BUBBLES_RESOLVED_AUTONOMY_SOURCE=%s\n' "$SOURCE_LAYER"
fi
