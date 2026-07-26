#!/usr/bin/env bash
set -u
set -o pipefail

# Durable work-repository boundary for one host-supplied Bubbles session.
# Repository selection is committed outside candidate repositories before any
# repository-local state, target expansion, specs discovery, or dispatch.

umask 077
export LC_ALL=C

usage() {
  cat <<'EOF'
Usage: repository-binding.sh <subcommand> [options]

Subcommands:
  preflight        Resolve and atomically commit a command repository binding
  validate-packet  Validate an actionable packet against current session state
  discover-specs   Enumerate only <validated repositoryRoot>/specs
  mirror-session   Mirror a validated decision into ignored repo-local state

Run repository-binding.sh <subcommand> --help for details. No bypass exists.
EOF
}

preflight_usage() {
  cat <<'EOF'
Usage: repository-binding.sh preflight \
  --session-id <id> --session-control-file <external-path> \
  --request-class <class> --workspace-root <root> [--workspace-root <root> ...] \
  [--repository-root <root> | --target <exact-path> | \
   --resolved-natural-language-root <declared-root> | --binding-packet-file <file>] \
  [--diagnostic-chat-cwd <path>] [--diagnostic-host-repository <path>] \
  [--diagnostic-active-editor <path>] [--diagnostic-tool-cwd <path>] \
  [--expected-control-revision <N>]
EOF
}

packet_usage() {
  cat <<'EOF'
Usage: repository-binding.sh validate-packet \
  --session-id <id> --session-control-file <path> --packet-file <path> \
  [--scenario-file <compiled-scenario.json> --node-id <node-id>] \
  [--emit-redacted-projection]
EOF
}

discover_usage() {
  cat <<'EOF'
Usage: repository-binding.sh discover-specs \
  --session-id <id> --session-control-file <path> --packet-file <path> --mode <mode>
EOF
}

mirror_usage() {
  cat <<'EOF'
Usage: repository-binding.sh mirror-session \
  --session-id <id> --session-control-file <path> --packet-file <path>
EOF
}

fail_usage() {
  printf 'repository-binding: %s\n' "$1" >&2
  return 2
}

sha256_text() {
  local value="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$value" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$value" | shasum -a 256 | awk '{print $1}'
  else
    return 1
  fi
}

control_path_digest() {
  local digest

  digest="$(sha256_text "$1")" || return 1
  printf 'sha256:%s' "$digest"
}

physical_directory() {
  (cd -P -- "$1" 2>/dev/null && pwd -P)
}

canonical_existing_path() {
  local path="$1"
  local candidate="$path"
  local parent
  local base
  local physical_parent
  local link_target
  local link_hops=0

  [[ "$path" == /* && ( -e "$path" || -L "$path" ) ]] || return 1
  while [[ -L "$candidate" ]]; do
    link_hops=$((link_hops + 1))
    [[ "$link_hops" -le 40 ]] || return 1
    link_target="$(readlink "$candidate")" || return 1
    if [[ "$link_target" == /* ]]; then
      candidate="$link_target"
    else
      candidate="$(dirname "$candidate")/$link_target"
    fi
    parent="$(dirname "$candidate")"
    base="$(basename "$candidate")"
    physical_parent="$(physical_directory "$parent")" || return 1
    if [[ "$physical_parent" == "/" ]]; then
      candidate="/$base"
    else
      candidate="$physical_parent/$base"
    fi
  done

  if [[ -d "$candidate" ]]; then
    physical_directory "$candidate"
    return $?
  fi
  [[ -e "$candidate" ]] || return 1
  parent="$(dirname "$candidate")"
  base="$(basename "$candidate")"
  physical_parent="$(physical_directory "$parent")" || return 1
  if [[ "$physical_parent" == "/" ]]; then
    printf '/%s' "$base"
  else
    printf '%s/%s' "$physical_parent" "$base"
  fi
}

physical_path_is_contained() {
  local path="$1"
  local root="$2"
  local parent

  while true; do
    [[ "$path" == "$root" ]] && return 0
    [[ "$path" != "/" ]] || return 1
    parent="$(dirname "$path")"
    [[ "$parent" != "$path" ]] || return 1
    path="$parent"
  done
}

relative_target_has_parent_traversal() {
  case "/$1/" in
    */../*) return 0 ;;
    *) return 1 ;;
  esac
}

canonical_git_root() {
  local candidate="$1"
  local physical_candidate
  local git_root

  physical_candidate="$(physical_directory "$candidate")" || return 1
  git_root="$(git -C "$physical_candidate" rev-parse --show-toplevel 2>/dev/null)" || return 1
  physical_directory "$git_root"
}

canonical_file_location() {
  local path="$1"
  local parent
  local base
  local suffix=""
  local segment
  local next_parent
  local physical_parent

  [[ "$path" == /* && ! -L "$path" ]] || return 1
  parent="$(dirname "$path")"
  base="$(basename "$path")"
  [[ -n "$base" && "$base" != "." && "$base" != ".." ]] || return 1

  while [[ ! -d "$parent" ]]; do
    segment="$(basename "$parent")"
    [[ -n "$segment" && "$segment" != "." && "$segment" != ".." ]] || return 1
    suffix="/$segment$suffix"
    next_parent="$(dirname "$parent")"
    [[ "$next_parent" != "$parent" ]] || return 1
    parent="$next_parent"
  done

  physical_parent="$(physical_directory "$parent")" || return 1
  if [[ "$physical_parent" == "/" ]]; then
    printf '%s/%s' "$suffix" "$base"
  else
    printf '%s%s/%s' "$physical_parent" "$suffix" "$base"
  fi
}

path_has_symlink_component() {
  local path="$1"
  local remainder
  local component
  local cursor=""

  [[ "$path" == /* ]] || return 0
  remainder="${path#/}"
  while [[ -n "$remainder" ]]; do
    if [[ "$remainder" == */* ]]; then
      component="${remainder%%/*}"
      remainder="${remainder#*/}"
    else
      component="$remainder"
      remainder=""
    fi
    [[ -n "$component" && "$component" != "." && "$component" != ".." ]] || return 0
    cursor="$cursor/$component"
    [[ ! -L "$cursor" ]] || return 0
    [[ -e "$cursor" ]] || return 1
  done
  return 1
}

foundation_eligible() {
  local root="$1"

  if [[ -f "$root/VERSION" && -f "$root/install.sh" && \
        -f "$root/bubbles/scripts/cli.sh" && -d "$root/agents" ]]; then
    return 0
  fi

  [[ -f "$root/.github/bubbles/release-manifest.json" && \
     -d "$root/.github/agents" ]]
}

repository_alias() {
  basename "$1"
}

contains_control_char() {
  case "$1" in
    *[[:cntrl:]]*) return 0 ;;
  esac
  return 1
}

workspace_candidates_have_control() {
  local candidates="$1"
  local candidate

  while IFS= read -r candidate; do
    [[ -n "$candidate" ]] || continue
    contains_control_char "$candidate" && return 0
  done <<< "$candidates"
  return 1
}

contains_line() {
  local lines="$1"
  local expected="$2"
  local line

  while IFS= read -r line; do
    [[ "$line" == "$expected" ]] && return 0
  done <<< "$lines"
  return 1
}

append_unique_line() {
  local lines="$1"
  local value="$2"

  if [[ -n "$lines" ]] && contains_line "$lines" "$value"; then
    printf '%s' "$lines"
  elif [[ -n "$lines" ]]; then
    printf '%s\n%s' "$lines" "$value"
  else
    printf '%s' "$value"
  fi
}

line_count() {
  local lines="$1"
  local count=0
  local line

  [[ -n "$lines" ]] || {
    printf '0\n'
    return 0
  }
  while IFS= read -r line; do
    [[ -n "$line" ]] && count=$((count + 1))
  done <<< "$lines"
  printf '%s\n' "$count"
}

control_path_is_external() {
  local control_file="$1"
  local roots="$2"
  local root

  while IFS= read -r root; do
    [[ -n "$root" ]] || continue
    case "$control_file" in
      "$root"|"$root"/*) return 1 ;;
    esac
  done <<< "$roots"
  return 0
}

ensure_private_control_parent() {
  local control_file="$1"
  local control_dir

  control_dir="$(dirname "$control_file")"
  if [[ ! -e "$control_dir" ]]; then
    mkdir -p "$control_dir" || return 1
    chmod 700 "$control_dir" || return 1
  fi
  private_control_parent_is_valid "$control_file"
}

private_control_parent_is_valid() {
  local control_file="$1"
  local control_dir
  local permissions

  control_dir="$(dirname "$control_file")"
  path_has_symlink_component "$control_dir" && return 1
  [[ -d "$control_dir" && -O "$control_dir" && -w "$control_dir" && -x "$control_dir" ]] || return 1
  permissions="$(ls -ld "$control_dir" 2>/dev/null)" || return 1
  permissions="${permissions%% *}"
  case "$permissions" in
    drwx------*) return 0 ;;
    *) return 1 ;;
  esac
}

control_file_is_private() {
  local control_file="$1"
  local permissions

  [[ ! -e "$control_file" ]] && return 0
  [[ -f "$control_file" && -O "$control_file" && ! -L "$control_file" ]] || return 1
  permissions="$(ls -l "$control_file" 2>/dev/null)" || return 1
  permissions="${permissions%% *}"
  case "$permissions" in
    -rw-------*) return 0 ;;
    *) return 1 ;;
  esac
}

control_record_matches_path() {
  local control_file="$1"
  local expected_digest

  expected_digest="$(control_path_digest "$control_file")" || return 1
  [[ "$(jq -r '.controlPathDigest // empty' "$control_file" 2>/dev/null)" == "$expected_digest" ]]
}

valid_session_id() {
  [[ -n "$1" && "$1" =~ ^[A-Za-z0-9._:-]+$ ]]
}

valid_request_class() {
  case "$1" in
    STRUCTURED|TARGETLESS_MODE|CONTINUATION|VAGUE|CONTINUE|FRAMEWORK) return 0 ;;
    *) return 1 ;;
  esac
}

control_is_valid() {
  local control_file="$1"

  [[ -f "$control_file" ]] || return 1
  jq -e '
    def exact_keys($expected): (keys | sort) == ($expected | sort);
    def nonempty_string: type == "string" and length > 0;
    def absolute_path: nonempty_string and startswith("/");
    def safe_session: nonempty_string and test("^[A-Za-z0-9._:-]+$");
    def command_authority:
      . == "explicit-repository-root"
      or . == "concrete-target"
      or . == "resolved-natural-language"
      or . == "durable-work-boundary"
      or . == "single-eligible-repository";
    def establishing_authority:
      . == "explicit-repository-root"
      or . == "concrete-target"
      or . == "resolved-natural-language"
      or . == "single-eligible-repository";
    def command_transition:
      . == "established" or . == "continued" or . == "confirmed" or . == "switched";
    def command_target_kind:
      . == "repository-root"
      or . == "absolute-target"
      or . == "relative-target"
      or . == "natural-language"
      or . == "inherited-boundary"
      or . == "sole-eligible-repository";
    def timestamp: nonempty_string and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$");
    def transition_entry($session):
      type == "object"
      and exact_keys(["revision", "decisionId", "fromRepositoryRoot", "toRepositoryRoot",
                      "fromRepositoryAlias", "toRepositoryAlias", "authority", "transition",
                      "targetKind", "timestamp"])
      and (.revision | type == "number" and . >= 1 and floor == .)
      and (.decisionId == ("rb:" + $session + ":" + (.revision | tostring)))
      and ((.fromRepositoryRoot == null) or (.fromRepositoryRoot | absolute_path))
      and (.toRepositoryRoot | absolute_path)
      and ((.fromRepositoryAlias == null) or (.fromRepositoryAlias | nonempty_string))
      and (.toRepositoryAlias | nonempty_string)
      and (.authority | command_authority)
      and (.transition | command_transition)
      and (.targetKind | command_target_kind)
      and (.timestamp | timestamp);
    . as $record
    | type == "object"
    and exact_keys(["schemaVersion", "sessionId", "controlPathDigest", "revision", "currentBinding", "transitionHistory"])
    and .schemaVersion == 1
    and (.sessionId | safe_session)
    and (.controlPathDigest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
    and (.revision | type == "number" and . >= 1 and floor == .)
    and (.currentBinding | type == "object"
         and exact_keys(["repositoryRoot", "repositoryAlias", "establishedDecisionId",
                         "establishedAuthority", "establishedAt", "lastDecisionId"]))
    and (.currentBinding.repositoryRoot | absolute_path)
    and (.currentBinding.repositoryAlias | nonempty_string)
    and (.currentBinding.establishedDecisionId | nonempty_string)
    and (.currentBinding.establishedAuthority | establishing_authority)
    and (.currentBinding.establishedAt | timestamp)
    and (.currentBinding.lastDecisionId == ("rb:" + .sessionId + ":" + (.revision | tostring)))
    and (.transitionHistory | type == "array" and length == $record.revision)
    and all($record.transitionHistory[]; transition_entry($record.sessionId))
    and all(range(0; ($record.transitionHistory | length));
      . as $index
      | $record.transitionHistory[$index].revision == ($index + 1))
    and ($record.transitionHistory[0].fromRepositoryRoot == null)
    and ($record.transitionHistory[0].fromRepositoryAlias == null)
    and all(range(1; ($record.transitionHistory | length));
      . as $index
      | $record.transitionHistory[$index].fromRepositoryRoot == $record.transitionHistory[$index - 1].toRepositoryRoot
      and $record.transitionHistory[$index].fromRepositoryAlias == $record.transitionHistory[$index - 1].toRepositoryAlias)
    and ($record.transitionHistory[-1].revision == $record.revision)
    and ($record.transitionHistory[-1].decisionId == $record.currentBinding.lastDecisionId)
    and ($record.transitionHistory[-1].toRepositoryRoot == $record.currentBinding.repositoryRoot)
    and ($record.transitionHistory[-1].toRepositoryAlias == $record.currentBinding.repositoryAlias)
    and any($record.transitionHistory[];
      .decisionId == $record.currentBinding.establishedDecisionId
      and (.transition == "established" or .transition == "switched")
      and .authority == $record.currentBinding.establishedAuthority
      and .timestamp == $record.currentBinding.establishedAt)
  ' "$control_file" >/dev/null 2>&1
}

packet_json_is_valid() {
  local packet_json="$1"

  jq -e '
    def exact_keys($expected): (keys | sort) == ($expected | sort);
    def nonempty_string: type == "string" and length > 0;
    def absolute_path: nonempty_string and startswith("/");
    def safe_session: nonempty_string and test("^[A-Za-z0-9._:-]+$");
    def command_authority:
      . == "explicit-repository-root"
      or . == "concrete-target"
      or . == "resolved-natural-language"
      or . == "durable-work-boundary"
      or . == "single-eligible-repository";
    def command_transition:
      . == "established" or . == "continued" or . == "confirmed" or . == "switched";
    def command_target_kind:
      . == "repository-root"
      or . == "absolute-target"
      or . == "relative-target"
      or . == "natural-language"
      or . == "inherited-boundary"
      or . == "sole-eligible-repository";
    def command_semantics:
      (.authority == "explicit-repository-root" and .targetKind == "repository-root"
       and (.transition == "established" or .transition == "confirmed" or .transition == "switched"))
      or (.authority == "concrete-target"
          and (.targetKind == "absolute-target" or .targetKind == "relative-target")
          and (.transition == "established" or .transition == "confirmed" or .transition == "switched"))
      or (.authority == "resolved-natural-language" and .targetKind == "natural-language"
          and (.transition == "established" or .transition == "confirmed" or .transition == "switched"))
      or (.authority == "durable-work-boundary" and .targetKind == "inherited-boundary"
          and .transition == "continued")
      or (.authority == "single-eligible-repository" and .targetKind == "sole-eligible-repository"
          and .transition == "established");
    . as $packet
    | $packet.repositoryResolution as $resolution
    | type == "object"
    and exact_keys(["repositoryRoot", "repositoryAlias", "repositoryResolution"])
    and ($packet.repositoryAlias | nonempty_string)
    and ($resolution | type == "object"
         and exact_keys(["sessionId", "decisionId", "controlRevision", "controlPathDigest", "authority", "transition",
                         "scopeKind", "scopeId", "targetKind", "pathVisibility", "actionable"]))
    and ($resolution.sessionId | safe_session)
    and ($resolution.controlRevision | type == "number" and . >= 1 and floor == .)
    and ($resolution.controlPathDigest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
    and ($resolution.actionable | type == "boolean")
    and (
      (($packet.repositoryRoot | absolute_path)
       and $resolution.pathVisibility == "local"
       and $resolution.actionable == true
       and (
         ($resolution.scopeKind == "command"
          and $resolution.scopeId == null
          and ($resolution.authority | command_authority)
          and ($resolution.transition | command_transition)
          and ($resolution.targetKind | command_target_kind)
          and ($resolution | command_semantics)
          and $resolution.decisionId == ("rb:" + $resolution.sessionId + ":" + ($resolution.controlRevision | tostring)))
         or
         ($resolution.scopeKind == "goal-node"
          and ($resolution.scopeId | nonempty_string)
          and $resolution.authority == "scoped-scenario-node"
          and $resolution.transition == "scoped-override"
          and $resolution.targetKind == "goal-node"
          and $resolution.decisionId == ("rb:" + $resolution.sessionId + ":" + ($resolution.controlRevision | tostring) + ":node:" + $resolution.scopeId))
       ))
      or
      ($packet.repositoryRoot == "<redacted-local-root>"
       and $resolution.pathVisibility == "redacted"
       and $resolution.actionable == false
       and (
         ($resolution.scopeKind == "command"
          and $resolution.scopeId == null
          and ($resolution.authority | command_authority)
          and ($resolution.transition | command_transition)
          and ($resolution.targetKind | command_target_kind)
          and ($resolution | command_semantics)
          and $resolution.decisionId == ("rb:" + $resolution.sessionId + ":" + ($resolution.controlRevision | tostring)))
         or
         ($resolution.scopeKind == "goal-node"
          and ($resolution.scopeId | nonempty_string)
          and $resolution.authority == "scoped-scenario-node"
          and $resolution.transition == "scoped-override"
          and $resolution.targetKind == "goal-node"
          and $resolution.decisionId == ("rb:" + $resolution.sessionId + ":" + ($resolution.controlRevision | tostring) + ":node:" + $resolution.scopeId))
       ))
    )
  ' <<< "$packet_json" >/dev/null 2>&1
}

packet_shape_is_valid() {
  local packet_file="$1"
  local packet_json

  [[ -f "$packet_file" ]] || return 1
  packet_json="$(cat -- "$packet_file")" || return 1
  packet_json_is_valid "$packet_json"
}

emit_refusal() {
  local reason="$1"
  local trusted_status="$2"
  local trusted_root="$3"
  local signal

  printf 'REPOSITORY PREFLIGHT REFUSED reason=%s affinity=unchanged repoLocalSideEffects=zero\n' "$reason"
  printf 'REPOSITORY-REFUSAL\n'
  printf 'outcome: refused\n'
  printf 'reasonCode: %s\n' "$reason"
  if [[ "$(jq -r 'length' <<< "$ACTIVE_OBSERVED_SIGNALS_JSON")" == "0" ]]; then
    printf 'observedSignals: []\n'
  else
    printf 'observedSignals:\n'
    while IFS= read -r signal; do
      printf 'observedSignals[].kind: %s\n' "$(jq -r '.kind' <<< "$signal")"
      printf 'observedSignals[].repository: %s\n' "$(jq -r '.repository' <<< "$signal")"
      printf 'observedSignals[].authority: diagnostic-only\n'
    done < <(jq -c '.[]' <<< "$ACTIVE_OBSERVED_SIGNALS_JSON")
  fi
  printf 'trustedBoundaryState.status: %s\n' "$trusted_status"
  printf 'trustedBoundaryState.repository: %s\n' "$trusted_root"
  printf 'requiredInput.field: repositoryRoot\n'
  printf 'requiredInput.requirement: one eligible canonical repository root\n'
  printf 'remediation.input.repositoryRoot: <canonical-repository-root>\n'
  printf 'affinity: unchanged\n'
  printf 'repoLocalSideEffects: zero\n'
  return 1
}

release_lock() {
  if [[ -n "$ACTIVE_LOCK_DIR" ]]; then
    rmdir "$ACTIVE_LOCK_DIR" 2>/dev/null || true
    ACTIVE_LOCK_DIR=""
  fi
}

acquire_lock() {
  local control_file="$1"
  local trusted_status="$2"
  local trusted_root="$3"
  ACTIVE_LOCK_DIR="$control_file.lock"
  if ! mkdir "$ACTIVE_LOCK_DIR" 2>/dev/null; then
    emit_refusal CONTROL_LOCK_BUSY "$trusted_status" "$trusted_root"
    return $?
  fi
  trap release_lock EXIT INT TERM
}

write_control_record() {
  local control_file="$1"
  local session_id="$2"
  local old_control="$3"
  local selected_root="$4"
  local selected_alias="$5"
  local authority="$6"
  local transition="$7"
  local target_kind="$8"
  local old_revision=0
  local new_revision
  local decision_id
  local timestamp
  local old_root=""
  local old_alias=""
  local established_decision
  local established_authority
  local established_at
  local control_dir
  local control_digest
  local temporary_file

  control_digest="$(control_path_digest "$control_file")" || return 1

  if [[ -n "$old_control" ]]; then
    old_revision="$(jq -r '.revision' "$old_control")"
    old_root="$(jq -r '.currentBinding.repositoryRoot' "$old_control")"
    old_alias="$(jq -r '.currentBinding.repositoryAlias' "$old_control")"
  fi
  new_revision=$((old_revision + 1))
  decision_id="rb:$session_id:$new_revision"
  timestamp="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

  if [[ -z "$old_control" || "$transition" == "switched" ]]; then
    established_decision="$decision_id"
    established_authority="$authority"
    established_at="$timestamp"
  else
    established_decision="$(jq -r '.currentBinding.establishedDecisionId' "$old_control")"
    established_authority="$(jq -r '.currentBinding.establishedAuthority' "$old_control")"
    established_at="$(jq -r '.currentBinding.establishedAt' "$old_control")"
  fi

  control_dir="$(dirname "$control_file")"
  temporary_file="$(mktemp "$control_dir/.repository-binding.XXXXXX")" || return 1

  if [[ -n "$old_control" ]]; then
    jq \
      --argjson revision "$new_revision" \
      --arg controlPathDigest "$control_digest" \
      --arg decision "$decision_id" \
      --arg root "$selected_root" \
      --arg alias "$selected_alias" \
      --arg establishedDecision "$established_decision" \
      --arg establishedAuthority "$established_authority" \
      --arg establishedAt "$established_at" \
      --arg oldRoot "$old_root" \
      --arg oldAlias "$old_alias" \
      --arg authority "$authority" \
      --arg transition "$transition" \
      --arg targetKind "$target_kind" \
      --arg timestamp "$timestamp" \
      '.controlPathDigest = $controlPathDigest
       | .revision = $revision
       | .currentBinding = {
           repositoryRoot: $root,
           repositoryAlias: $alias,
           establishedDecisionId: $establishedDecision,
           establishedAuthority: $establishedAuthority,
           establishedAt: $establishedAt,
           lastDecisionId: $decision
         }
       | .transitionHistory += [{
           revision: $revision,
           decisionId: $decision,
           fromRepositoryRoot: $oldRoot,
           toRepositoryRoot: $root,
           fromRepositoryAlias: $oldAlias,
           toRepositoryAlias: $alias,
           authority: $authority,
           transition: $transition,
           targetKind: $targetKind,
           timestamp: $timestamp
        }]' "$old_control" > "$temporary_file" || {
        rm -f "$temporary_file"
        return 1
      }
  else
    jq -n \
      --arg session "$session_id" \
      --arg controlPathDigest "$control_digest" \
      --argjson revision "$new_revision" \
      --arg decision "$decision_id" \
      --arg root "$selected_root" \
      --arg alias "$selected_alias" \
      --arg authority "$authority" \
      --arg transition "$transition" \
      --arg targetKind "$target_kind" \
      --arg timestamp "$timestamp" \
      '{
        schemaVersion: 1,
        sessionId: $session,
        controlPathDigest: $controlPathDigest,
        revision: $revision,
        currentBinding: {
          repositoryRoot: $root,
          repositoryAlias: $alias,
          establishedDecisionId: $decision,
          establishedAuthority: $authority,
          establishedAt: $timestamp,
          lastDecisionId: $decision
        },
        transitionHistory: [{
          revision: $revision,
          decisionId: $decision,
          fromRepositoryRoot: null,
          toRepositoryRoot: $root,
          fromRepositoryAlias: null,
          toRepositoryAlias: $alias,
          authority: $authority,
          transition: $transition,
          targetKind: $targetKind,
          timestamp: $timestamp
        }]
      }' > "$temporary_file" || {
        rm -f "$temporary_file"
        return 1
      }
  fi

  control_is_valid "$temporary_file" || {
    rm -f "$temporary_file"
    return 1
  }
  chmod 600 "$temporary_file" || {
    rm -f "$temporary_file"
    return 1
  }
  mv "$temporary_file" "$control_file" || {
    rm -f "$temporary_file"
    return 1
  }
}

emit_decision() {
  local control_file="$1"
  local authority="$2"
  local transition="$3"
  local target_kind="$4"
  local source="$5"
  local compatibility="$6"
  local root
  local alias
  local session_id
  local revision
  local decision_id
  local control_digest
  local headline="BOUND"
  local previous_alias=""

  root="$(jq -r '.currentBinding.repositoryRoot' "$control_file")"
  alias="$(jq -r '.currentBinding.repositoryAlias' "$control_file")"
  session_id="$(jq -r '.sessionId' "$control_file")"
  revision="$(jq -r '.revision' "$control_file")"
  decision_id="$(jq -r '.currentBinding.lastDecisionId' "$control_file")"
  control_digest="$(jq -r '.controlPathDigest' "$control_file")"
  case "$transition" in
    switched)
      headline="SWITCHED"
      previous_alias="$(jq -r '.transitionHistory[-1].fromRepositoryAlias' "$control_file")"
      ;;
    confirmed) headline="CONFIRMED" ;;
  esac

  printf 'REPOSITORY PREFLIGHT %s repository=%s root=%s' "$headline" "$alias" "$root"
  [[ -n "$previous_alias" && "$previous_alias" != "null" ]] && printf ' previous=%s' "$previous_alias"
  printf ' source=%s affinity=%s' "$source" "$transition"
  [[ -n "$compatibility" ]] && printf ' compatibility=%s' "$compatibility"
  printf '\n'
  printf 'PREFLIGHT_COMMITTED decision=%s revision=%s repository=%s root=%s\n' \
    "$decision_id" "$revision" "$alias" "$root"

  jq -cn \
    --arg root "$root" \
    --arg alias "$alias" \
    --arg session "$session_id" \
    --arg decision "$decision_id" \
    --argjson revision "$revision" \
    --arg controlPathDigest "$control_digest" \
    --arg authority "$authority" \
    --arg transition "$transition" \
    --arg targetKind "$target_kind" \
    '{
      repositoryRoot: $root,
      repositoryAlias: $alias,
      repositoryResolution: {
        sessionId: $session,
        decisionId: $decision,
        controlRevision: $revision,
        controlPathDigest: $controlPathDigest,
        authority: $authority,
        transition: $transition,
        scopeKind: "command",
        scopeId: null,
        targetKind: $targetKind,
        pathVisibility: "local",
        actionable: true
      }
    }'
}

emit_redacted_projection_json() {
  local packet_json="$1"

  jq -c '
    .repositoryRoot = "<redacted-local-root>"
    | .repositoryResolution.pathVisibility = "redacted"
    | .repositoryResolution.actionable = false
  ' <<< "$packet_json"
}

collect_workspace_roots() {
  local candidates="$1"
  local canonical_roots=""
  local eligible_roots=""
  local candidate
  local canonical

  while IFS= read -r candidate; do
    [[ -n "$candidate" ]] || continue
    canonical="$(canonical_git_root "$candidate")" || continue
    canonical_roots="$(append_unique_line "$canonical_roots" "$canonical")"
    if foundation_eligible "$canonical"; then
      eligible_roots="$(append_unique_line "$eligible_roots" "$canonical")"
    fi
  done <<< "$candidates"
  printf '%s\n---ELIGIBLE---\n%s' "$canonical_roots" "$eligible_roots"
}

resolve_target_root() {
  local target="$1"
  local eligible_roots="$2"
  local matches=""
  local root
  local canonical
  local physical_target
  local target_path
  local target_probe

  if [[ "$target" == /* ]]; then
    physical_target="$(canonical_existing_path "$target")" || return 1
    if [[ -d "$physical_target" ]]; then
      target_probe="$physical_target"
    else
      target_probe="$(dirname "$physical_target")"
    fi
    canonical="$(canonical_git_root "$target_probe" 2>/dev/null || true)"
    [[ -n "$canonical" ]] || return 1
    physical_path_is_contained "$physical_target" "$canonical" || return 1
    foundation_eligible "$canonical" || return 2
    printf '%s' "$canonical"
    return 0
  fi

  relative_target_has_parent_traversal "$target" && return 1

  while IFS= read -r root; do
    [[ -n "$root" ]] || continue
    target_path="$root/$target"
    [[ -e "$target_path" || -L "$target_path" ]] || continue
    physical_target="$(canonical_existing_path "$target_path")" || continue
    physical_path_is_contained "$physical_target" "$root" || continue
    matches="$(append_unique_line "$matches" "$root")"
  done <<< "$eligible_roots"
  case "$(line_count "$matches")" in
    0) return 1 ;;
    1) printf '%s' "$matches" ; return 0 ;;
    *) return 3 ;;
  esac
}

packet_json_conflicts_with_control() {
  local packet_json="$1"
  local control_file="$2"
  local expected_session="$3"
  local packet_revision

  packet_json_is_valid "$packet_json" || return 2
  [[ "$(jq -r '.repositoryResolution.actionable' <<< "$packet_json")" == "true" ]] || return 3
  [[ "$(jq -r '.repositoryResolution.pathVisibility' <<< "$packet_json")" == "local" ]] || return 3
  [[ "$(jq -r '.repositoryResolution.sessionId' <<< "$packet_json")" == "$expected_session" ]] || return 1
  [[ "$(jq -r '.repositoryRoot' <<< "$packet_json")" == "$(jq -r '.currentBinding.repositoryRoot' "$control_file")" ]] || return 1
  packet_revision="$(jq -r '.repositoryResolution.controlRevision' <<< "$packet_json")"
  [[ "$packet_revision" == "$(jq -r '.revision' "$control_file")" ]] || return 1
  [[ "$(jq -r '.repositoryResolution.controlPathDigest' <<< "$packet_json")" == "$(jq -r '.controlPathDigest' "$control_file")" ]] || return 1
  [[ "$(jq -r '.repositoryResolution.decisionId' <<< "$packet_json")" == "$(jq -r '.currentBinding.lastDecisionId' "$control_file")" ]] || return 1
  [[ "$(jq -r '.repositoryAlias' <<< "$packet_json")" == "$(jq -r '.currentBinding.repositoryAlias' "$control_file")" ]] || return 1
  [[ "$(jq -r '.repositoryResolution.authority' <<< "$packet_json")" == "$(jq -r --argjson revision "$packet_revision" '.transitionHistory[] | select(.revision == $revision) | .authority' "$control_file")" ]] || return 1
  [[ "$(jq -r '.repositoryResolution.transition' <<< "$packet_json")" == "$(jq -r --argjson revision "$packet_revision" '.transitionHistory[] | select(.revision == $revision) | .transition' "$control_file")" ]] || return 1
  [[ "$(jq -r '.repositoryResolution.targetKind' <<< "$packet_json")" == "$(jq -r --argjson revision "$packet_revision" '.transitionHistory[] | select(.revision == $revision) | .targetKind' "$control_file")" ]] || return 1
  return 0
}

packet_conflicts_with_control() {
  local packet_file="$1"
  local control_file="$2"
  local expected_session="$3"
  local packet_json

  [[ -f "$packet_file" ]] || return 2
  packet_json="$(cat -- "$packet_file")" || return 2
  packet_json_conflicts_with_control "$packet_json" "$control_file" "$expected_session"
}

preflight() {
  local session_id=""
  local control_file=""
  local request_class=""
  local workspace_candidates=""
  local explicit_root=""
  local target=""
  local resolved_natural_language_root=""
  local binding_packet=""
  local expected_revision=""
  local explicit_intent_count=0
  local workspace_result
  local canonical_roots
  local eligible_roots
  local old_control=""
  local old_root=""
  local old_alias=""
  local selected_root=""
  local selected_alias=""
  local authority=""
  local transition=""
  local target_kind=""
  local source=""
  local compatibility=""
  local diagnostic_chat_cwd=""
  local diagnostic_host_repository=""
  local diagnostic_active_editor=""
  local diagnostic_tool_cwd=""
  local target_rc=0
  local canonical_control_file=""
  local packet_rc=0
  local trusted_status="absent"
  local trusted_root="none"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session-id) shift; [[ $# -gt 0 ]] || { fail_usage '--session-id requires a value'; return 2; }; session_id="$1" ;;
      --session-control-file) shift; [[ $# -gt 0 ]] || { fail_usage '--session-control-file requires a value'; return 2; }; control_file="$1" ;;
      --request-class) shift; [[ $# -gt 0 ]] || { fail_usage '--request-class requires a value'; return 2; }; request_class="$1" ;;
      --workspace-root) shift; [[ $# -gt 0 ]] || { fail_usage '--workspace-root requires a value'; return 2; }; workspace_candidates="$(append_unique_line "$workspace_candidates" "$1")" ;;
      --repository-root) shift; [[ $# -gt 0 ]] || { fail_usage '--repository-root requires a value'; return 2; }; explicit_root="$1" ;;
      --target) shift; [[ $# -gt 0 ]] || { fail_usage '--target requires a value'; return 2; }; target="$1" ;;
      --resolved-natural-language-root) shift; [[ $# -gt 0 ]] || { fail_usage '--resolved-natural-language-root requires a value'; return 2; }; resolved_natural_language_root="$1" ;;
      --binding-packet-file) shift; [[ $# -gt 0 ]] || { fail_usage '--binding-packet-file requires a value'; return 2; }; binding_packet="$1" ;;
      --diagnostic-chat-cwd) shift; [[ $# -gt 0 ]] || { fail_usage '--diagnostic-chat-cwd requires a value'; return 2; }; diagnostic_chat_cwd="$1" ;;
      --diagnostic-host-repository) shift; [[ $# -gt 0 ]] || { fail_usage '--diagnostic-host-repository requires a value'; return 2; }; diagnostic_host_repository="$1" ;;
      --diagnostic-active-editor) shift; [[ $# -gt 0 ]] || { fail_usage '--diagnostic-active-editor requires a value'; return 2; }; diagnostic_active_editor="$1" ;;
      --diagnostic-tool-cwd) shift; [[ $# -gt 0 ]] || { fail_usage '--diagnostic-tool-cwd requires a value'; return 2; }; diagnostic_tool_cwd="$1" ;;
      --expected-control-revision) shift; [[ $# -gt 0 ]] || { fail_usage '--expected-control-revision requires a value'; return 2; }; expected_revision="$1" ;;
      -h|--help) preflight_usage; return 0 ;;
      *) fail_usage "unknown preflight option: $1"; return 2 ;;
    esac
    shift
  done

  # Reject C0 control characters in any repository, alias-bearing, or diagnostic
  # value before emitting any line-oriented output or creating control state. A
  # control byte cannot name a real repository root or alias, and reflecting it
  # could smuggle control sequences into downstream output. Workspace roots are
  # checked per entry so the newline joiner is never treated as adversarial.
  if contains_control_char "$explicit_root" || \
     contains_control_char "$target" || \
     contains_control_char "$resolved_natural_language_root" || \
     contains_control_char "$diagnostic_chat_cwd" || \
     contains_control_char "$diagnostic_host_repository" || \
     contains_control_char "$diagnostic_active_editor" || \
     contains_control_char "$diagnostic_tool_cwd" || \
     workspace_candidates_have_control "$workspace_candidates"; then
    printf 'REPOSITORY PREFLIGHT REFUSED reason=CONTROL_CHARACTER_INPUT affinity=unchanged repoLocalSideEffects=zero\n'
    return 2
  fi

  valid_session_id "$session_id" || { fail_usage 'preflight requires a safe --session-id'; return 2; }
  [[ -n "$control_file" ]] || { fail_usage 'preflight requires --session-control-file'; return 2; }
  valid_request_class "$request_class" || { fail_usage 'preflight requires a supported --request-class'; return 2; }
  [[ -n "$workspace_candidates" ]] || { fail_usage 'preflight requires at least one --workspace-root'; return 2; }
  [[ -z "$expected_revision" || "$expected_revision" =~ ^[0-9]+$ ]] || { fail_usage '--expected-control-revision must be an integer'; return 2; }
  [[ -n "$explicit_root" ]] && explicit_intent_count=$((explicit_intent_count + 1))
  [[ -n "$target" ]] && explicit_intent_count=$((explicit_intent_count + 1))
  [[ -n "$resolved_natural_language_root" ]] && explicit_intent_count=$((explicit_intent_count + 1))
  if [[ "$explicit_intent_count" -gt 1 ]]; then
    fail_usage '--repository-root, --target, and --resolved-natural-language-root are mutually exclusive'
    return 2
  fi
  ACTIVE_OBSERVED_SIGNALS_JSON="$(jq -cn \
    --arg chatCwd "$diagnostic_chat_cwd" \
    --arg hostRepository "$diagnostic_host_repository" \
    --arg activeEditor "$diagnostic_active_editor" \
    --arg toolCwd "$diagnostic_tool_cwd" \
    '[
      {kind: "chat-cwd", repository: $chatCwd},
      {kind: "host-repository", repository: $hostRepository},
      {kind: "active-editor", repository: $activeEditor},
      {kind: "tool-cwd", repository: $toolCwd}
    ] | map(select(.repository != "") | . + {authority: "diagnostic-only"})')" || return 3

  workspace_result="$(collect_workspace_roots "$workspace_candidates")"
  canonical_roots="${workspace_result%%$'\n---ELIGIBLE---\n'*}"
  eligible_roots="${workspace_result#*$'\n---ELIGIBLE---\n'}"
  canonical_control_file="$(canonical_file_location "$control_file")" || {
    fail_usage '--session-control-file must be an absolute, non-symlink external path'
    return 2
  }
  if path_has_symlink_component "$control_file"; then
    fail_usage '--session-control-file must not traverse symlink components'
    return 2
  fi
  control_path_is_external "$canonical_control_file" "$canonical_roots" || {
    fail_usage '--session-control-file must be external to workspace repositories'
    return 2
  }
  control_file="$canonical_control_file"
  control_file_is_private "$control_file" || {
    fail_usage '--session-control-file must be an owner-private regular file when it exists'
    return 2
  }
  if [[ -f "$control_file" ]]; then
    if control_is_valid "$control_file" && control_record_matches_path "$control_file"; then
      trusted_root="$(jq -r '.currentBinding.repositoryRoot' "$control_file")"
      if [[ "$(jq -r '.sessionId' "$control_file")" != "$session_id" ]]; then
        trusted_status="conflicting"
      elif [[ ! -d "$trusted_root" ]] || ! foundation_eligible "$trusted_root" || \
           [[ "$(canonical_git_root "$trusted_root" 2>/dev/null || true)" != "$trusted_root" ]] || \
           ! contains_line "$eligible_roots" "$trusted_root"; then
        trusted_status="stale"
      else
        trusted_status="valid"
      fi
    else
      trusted_status="malformed"
    fi
  fi

  # Explicit resolution failures occur before lock acquisition and never mutate.
  if [[ -n "$explicit_root" ]]; then
    if [[ ! -d "$explicit_root" ]]; then
      emit_refusal EXPLICIT_REPOSITORY_ROOT_NOT_FOUND "$trusted_status" "$trusted_root"
      return $?
    fi
    selected_root="$(canonical_git_root "$explicit_root" || true)"
    if [[ -z "$selected_root" ]] || ! foundation_eligible "$selected_root"; then
      emit_refusal EXPLICIT_REPOSITORY_ROOT_INELIGIBLE "$trusted_status" "$trusted_root"
      return $?
    fi
    authority="explicit-repository-root"
    target_kind="repository-root"
    source="explicit-repositoryRoot"
  elif [[ -n "$resolved_natural_language_root" ]]; then
    if [[ ! -d "$resolved_natural_language_root" ]]; then
      emit_refusal EXPLICIT_REPOSITORY_ROOT_NOT_FOUND "$trusted_status" "$trusted_root"
      return $?
    fi
    selected_root="$(canonical_git_root "$resolved_natural_language_root" || true)"
    if [[ -z "$selected_root" ]] || ! foundation_eligible "$selected_root" || \
       ! contains_line "$eligible_roots" "$selected_root"; then
      emit_refusal EXPLICIT_REPOSITORY_ROOT_INELIGIBLE "$trusted_status" "$trusted_root"
      return $?
    fi
    authority="resolved-natural-language"
    target_kind="natural-language"
    source="concrete-target"
  elif [[ -n "$target" ]]; then
    selected_root="$(resolve_target_root "$target" "$eligible_roots")" || target_rc=$?
    case "$target_rc" in
      0) ;;
      3) emit_refusal TARGET_ALIAS_AMBIGUOUS "$trusted_status" "$trusted_root"; return $? ;;
      *) emit_refusal EXPLICIT_REPOSITORY_ROOT_NOT_FOUND "$trusted_status" "$trusted_root"; return $? ;;
    esac
    authority="concrete-target"
    if [[ "$target" == /* ]]; then
      target_kind="absolute-target"
    else
      target_kind="relative-target"
    fi
    source="concrete-target"
  fi

  if [[ -n "$selected_root" ]]; then
    canonical_roots="$(append_unique_line "$canonical_roots" "$selected_root")"
    eligible_roots="$(append_unique_line "$eligible_roots" "$selected_root")"
  fi
  control_path_is_external "$control_file" "$canonical_roots" || {
    fail_usage '--session-control-file must be external to workspace repositories'
    return 2
  }
  if [[ -z "$expected_revision" ]] && {
       [[ -n "$selected_root" ]] ||
       [[ "$trusted_status" == "valid" ]] ||
       { [[ "$trusted_status" == "absent" ]] && [[ "$(line_count "$eligible_roots")" == "1" ]]; }
     }; then
    emit_refusal BOUNDARY_CONFLICT "$trusted_status" "$trusted_root"
    return $?
  fi
  ensure_private_control_parent "$control_file" || {
    printf 'repository-binding: session control directory must be private, writable, and searchable\n' >&2
    return 3
  }

  if [[ -n "$binding_packet" && -z "$selected_root" ]] && ! packet_shape_is_valid "$binding_packet"; then
    emit_refusal BOUNDARY_MALFORMED "$trusted_status" "$trusted_root"
    return $?
  fi

  acquire_lock "$control_file" "$trusted_status" "$trusted_root" || return 1

  if [[ -f "$control_file" ]]; then
    if ! control_is_valid "$control_file" || ! control_record_matches_path "$control_file"; then
      emit_refusal BOUNDARY_MALFORMED malformed none
      return $?
    else
      if [[ "$(jq -r '.sessionId' "$control_file")" != "$session_id" ]]; then
        emit_refusal BOUNDARY_CONFLICT conflicting "$(jq -r '.currentBinding.repositoryRoot' "$control_file")"
        return $?
      fi
      old_control="$control_file"
      old_root="$(jq -r '.currentBinding.repositoryRoot' "$control_file")"
      old_alias="$(jq -r '.currentBinding.repositoryAlias' "$control_file")"
      if [[ -n "$expected_revision" && "$(jq -r '.revision' "$control_file")" != "$expected_revision" ]]; then
        emit_refusal BOUNDARY_CONFLICT valid "$old_root"
        return $?
      fi
      if [[ ! -d "$old_root" ]] || ! foundation_eligible "$old_root" || \
         [[ "$(canonical_git_root "$old_root" 2>/dev/null || true)" != "$old_root" ]] || \
         ! contains_line "$eligible_roots" "$old_root"; then
        if [[ -z "$selected_root" ]]; then
          emit_refusal BOUNDARY_STALE stale "$old_root"
          return $?
        fi
      fi
      if [[ -n "$binding_packet" && -z "$selected_root" ]]; then
        packet_conflicts_with_control "$binding_packet" "$control_file" "$session_id"
        packet_rc=$?
        if [[ "$packet_rc" -ne 0 ]]; then
          emit_refusal BOUNDARY_CONFLICT valid "$old_root"
          return $?
        fi
      fi
    fi
  elif [[ -n "$binding_packet" && -z "$selected_root" ]]; then
    emit_refusal BOUNDARY_CONFLICT absent none
    return $?
  elif [[ -n "$expected_revision" && "$expected_revision" != "0" ]]; then
    emit_refusal BOUNDARY_CONFLICT absent none
    return $?
  fi

  if [[ -z "$selected_root" ]]; then
    if [[ -n "$old_control" ]]; then
      selected_root="$old_root"
      selected_alias="$old_alias"
      authority="durable-work-boundary"
      transition="continued"
      target_kind="inherited-boundary"
      source="session-work-boundary"
    else
      case "$(line_count "$eligible_roots")" in
        0) emit_refusal NO_ELIGIBLE_REPOSITORY absent none; return $? ;;
        1)
          selected_root="$eligible_roots"
          authority="single-eligible-repository"
          transition="established"
          target_kind="sole-eligible-repository"
          source="sole-eligible-repo"
          compatibility="single-repository"
          ;;
        *) emit_refusal TARGETLESS_MULTI_ROOT_UNBOUND absent none; return $? ;;
      esac
    fi
  else
    selected_alias="$(repository_alias "$selected_root")"
    if [[ -z "$old_control" ]]; then
      transition="established"
    elif [[ "$selected_root" == "$old_root" ]]; then
      transition="confirmed"
    else
      transition="switched"
    fi
  fi

  [[ -n "$selected_alias" ]] || selected_alias="$(repository_alias "$selected_root")"
  if [[ ! -d "$selected_root" ]] || ! foundation_eligible "$selected_root" || \
     [[ "$(canonical_git_root "$selected_root" 2>/dev/null || true)" != "$selected_root" ]]; then
    emit_refusal EXPLICIT_REPOSITORY_ROOT_INELIGIBLE absent none
    return $?
  fi
  if ! write_control_record "$control_file" "$session_id" "$old_control" \
      "$selected_root" "$selected_alias" "$authority" "$transition" "$target_kind"; then
    printf 'repository-binding: failed to atomically commit session control record\n' >&2
    return 3
  fi
  release_lock
  trap - EXIT INT TERM
  emit_decision "$control_file" "$authority" "$transition" "$target_kind" "$source" "$compatibility"
}

goal_node_declaration_json() {
  local scenario_file="$1"
  local node_id="$2"

  [[ -f "$scenario_file" ]] || return 1
  jq -ce --arg nodeId "$node_id" '
    (.nodes // [] | map(select(.id == $nodeId))) as $nodes
    | select(($nodes | length) == 1)
    | $nodes[0] as $node
    | (.repos // [] | map(select(.id == $node.repo))) as $repos
    | select(($repos | length) == 1)
    | $repos[0] as $repo
    | select($repo.repositoryRoot | type == "string" and length > 0)
    | select($repo.repositoryAlias | type == "string" and test("^[A-Za-z0-9][A-Za-z0-9._-]*$"))
    | select($node.repositoryResolution | type == "object")
    | select($node.repositoryResolution.controlPathDigest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
    | select($node.repositoryResolution.authority == "scoped-scenario-node")
    | select($node.repositoryResolution.transition == "scoped-override")
    | select($node.repositoryResolution.scopeKind == "goal-node")
    | select($node.repositoryResolution.scopeId == $node.id)
    | select($node.repositoryResolution.targetKind == "goal-node")
    | select($node.repositoryResolution.pathVisibility == "local")
    | select($node.repositoryResolution.actionable == true)
    | {
        repositoryRoot: $repo.repositoryRoot,
        repositoryAlias: $repo.repositoryAlias,
        repositoryResolution: $node.repositoryResolution
      }
  ' "$scenario_file"
}

validate_packet_internal() {
  local session_id="$1"
  local control_file="$2"
  local packet_file="$3"
  local output_mode="$4"
  local expected_goal_node_declaration="${5:-}"
  local packet_rc
  local root
  local scope_kind
  local scope_id
  local canonical_control_file
  local packet_json

  canonical_control_file="$(canonical_file_location "$control_file" 2>/dev/null || true)"
  if [[ -z "$canonical_control_file" ]] || path_has_symlink_component "$control_file" || \
     ! private_control_parent_is_valid "$canonical_control_file" || \
     ! control_file_is_private "$canonical_control_file"; then
    printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_MALFORMED actionable=false\n'
    return 1
  fi
  control_file="$canonical_control_file"
  if ! control_is_valid "$control_file" || ! control_record_matches_path "$control_file"; then
    printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_MALFORMED actionable=false\n'
    return 1
  fi
  packet_json="$(cat -- "$packet_file" 2>/dev/null)" || packet_json=""
  if ! packet_json_is_valid "$packet_json"; then
    printf 'REPOSITORY PACKET REFUSED reason=PACKET_MALFORMED actionable=false\n'
    return 1
  fi
  if [[ "$(jq -r '.repositoryResolution.actionable' <<< "$packet_json")" != "true" || \
        "$(jq -r '.repositoryResolution.pathVisibility' <<< "$packet_json")" != "local" ]]; then
    printf 'REPOSITORY PACKET REFUSED reason=PACKET_NONACTIONABLE actionable=false pathVisibility=%s redacted=%s\n' \
      "$(jq -r '.repositoryResolution.pathVisibility' <<< "$packet_json")" \
      "$(jq -r '.repositoryRoot == "<redacted-local-root>"' <<< "$packet_json")"
    return 1
  fi
  root="$(jq -r '.repositoryRoot' <<< "$packet_json")"
  scope_kind="$(jq -r '.repositoryResolution.scopeKind' <<< "$packet_json")"
  scope_id="$(jq -r '.repositoryResolution.scopeId // empty' <<< "$packet_json")"
  if [[ "$scope_kind" == "goal-node" && -z "$expected_goal_node_declaration" ]]; then
    printf 'REPOSITORY PACKET REFUSED reason=GOAL_NODE_DECLARATION_REQUIRED actionable=false scopeId=%s\n' \
      "$scope_id"
    return 1
  fi
  if [[ -n "$expected_goal_node_declaration" ]] && ! jq -e \
      --argjson expected "$expected_goal_node_declaration" \
      '.repositoryRoot == $expected.repositoryRoot and
       .repositoryAlias == $expected.repositoryAlias and
       .repositoryResolution == $expected.repositoryResolution' \
      <<< "$packet_json" >/dev/null 2>&1; then
    printf 'REPOSITORY PACKET REFUSED reason=GOAL_NODE_REPOSITORY_MISMATCH actionable=false scopeId=%s\n' \
      "$scope_id"
    return 1
  fi
  if ! control_path_is_external "$control_file" "$root"; then
    printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_MALFORMED actionable=false\n'
    return 1
  fi
  if [[ ! -d "$root" ]] || ! foundation_eligible "$root" || \
     [[ "$(canonical_git_root "$root" 2>/dev/null || true)" != "$root" ]]; then
    if [[ "$scope_kind" == "goal-node" ]]; then
      printf 'REPOSITORY PACKET REFUSED reason=GOAL_NODE_REPOSITORY_UNRESOLVED actionable=false scopeId=%s\n' "$scope_id"
    else
      printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_STALE actionable=false\n'
    fi
    return 1
  fi
  if [[ "$scope_kind" == "goal-node" ]]; then
    if [[ "$(jq -r '.repositoryResolution.sessionId' <<< "$packet_json")" != "$session_id" || \
          "$(jq -r '.repositoryResolution.controlRevision' <<< "$packet_json")" != "$(jq -r '.revision' "$control_file")" || \
          "$(jq -r '.repositoryResolution.controlPathDigest' <<< "$packet_json")" != "$(jq -r '.controlPathDigest' "$control_file")" ]]; then
      printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_CONFLICT actionable=false scopeId=%s\n' "$scope_id"
      return 1
    fi
    if [[ "$output_mode" == "visible" ]]; then
      printf 'REPOSITORY PACKET SCOPED actionable=true repository=%s root=%s decision=%s revision=%s scopeKind=goal-node scopeId=%s\n' \
        "$(jq -r '.repositoryAlias' <<< "$packet_json")" \
        "$root" \
        "$(jq -r '.repositoryResolution.decisionId' <<< "$packet_json")" \
        "$(jq -r '.repositoryResolution.controlRevision' <<< "$packet_json")" \
        "$scope_id"
    elif [[ "$output_mode" != "silent" ]]; then
      printf 'repository-binding: invalid packet validation output mode\n' >&2
      return 2
    fi
    VALIDATED_PACKET_JSON="$packet_json"
    return 0
  fi
  packet_json_conflicts_with_control "$packet_json" "$control_file" "$session_id"
  packet_rc=$?
  if [[ "$packet_rc" -ne 0 ]]; then
    printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_CONFLICT actionable=false\n'
    return 1
  fi
  if [[ "$output_mode" == "visible" ]]; then
    printf 'REPOSITORY PACKET VALID actionable=true repository=%s root=%s decision=%s revision=%s\n' \
      "$(jq -r '.repositoryAlias' <<< "$packet_json")" \
      "$(jq -r '.repositoryRoot' <<< "$packet_json")" \
      "$(jq -r '.repositoryResolution.decisionId' <<< "$packet_json")" \
      "$(jq -r '.repositoryResolution.controlRevision' <<< "$packet_json")"
  elif [[ "$output_mode" != "silent" ]]; then
    printf 'repository-binding: invalid packet validation output mode\n' >&2
    return 2
  fi
  VALIDATED_PACKET_JSON="$packet_json"
  return 0
}

validate_packet() {
  local session_id=""
  local control_file=""
  local packet_file=""
  local scenario_file=""
  local node_id=""
  local goal_node_declaration=""
  local emit_redacted=false
  local output_mode="visible"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session-id) shift; [[ $# -gt 0 ]] || { fail_usage '--session-id requires a value'; return 2; }; session_id="$1" ;;
      --session-control-file) shift; [[ $# -gt 0 ]] || { fail_usage '--session-control-file requires a value'; return 2; }; control_file="$1" ;;
      --packet-file) shift; [[ $# -gt 0 ]] || { fail_usage '--packet-file requires a value'; return 2; }; packet_file="$1" ;;
      --scenario-file) shift; [[ $# -gt 0 ]] || { fail_usage '--scenario-file requires a value'; return 2; }; scenario_file="$1" ;;
      --node-id) shift; [[ $# -gt 0 ]] || { fail_usage '--node-id requires a value'; return 2; }; node_id="$1" ;;
      --emit-redacted-projection) emit_redacted=true ;;
      -h|--help) packet_usage; return 0 ;;
      *) fail_usage "unknown validate-packet option: $1"; return 2 ;;
    esac
    shift
  done
  valid_session_id "$session_id" || { fail_usage 'validate-packet requires a safe --session-id'; return 2; }
  [[ -n "$control_file" && -n "${packet_file:-}" ]] || { fail_usage 'validate-packet requires control and packet files'; return 2; }
  if [[ -n "$scenario_file" && -z "$node_id" ]] || [[ -z "$scenario_file" && -n "$node_id" ]]; then
    fail_usage 'goal-node validation requires both --scenario-file and --node-id'
    return 2
  fi
  if [[ -n "$scenario_file" ]]; then
    goal_node_declaration="$(goal_node_declaration_json "$scenario_file" "$node_id" 2>/dev/null || true)"
    if [[ -z "$goal_node_declaration" ]]; then
      printf 'REPOSITORY PACKET REFUSED reason=GOAL_NODE_DECLARATION_INVALID actionable=false scopeId=%s\n' \
        "$node_id"
      return 1
    fi
  fi
  [[ "$emit_redacted" == true ]] && output_mode="silent"
  validate_packet_internal "$session_id" "$control_file" "$packet_file" "$output_mode" \
    "$goal_node_declaration" || return $?
  if [[ "$emit_redacted" == true ]]; then
    emit_redacted_projection_json "$VALIDATED_PACKET_JSON"
  fi
}

parse_packet_command_args() {
  local command_kind="$1"
  shift
  PARSED_SESSION_ID=""
  PARSED_CONTROL_FILE=""
  PARSED_PACKET_FILE=""
  PARSED_MODE=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session-id) shift; [[ $# -gt 0 ]] || return 2; PARSED_SESSION_ID="$1" ;;
      --session-control-file) shift; [[ $# -gt 0 ]] || return 2; PARSED_CONTROL_FILE="$1" ;;
      --packet-file) shift; [[ $# -gt 0 ]] || return 2; PARSED_PACKET_FILE="$1" ;;
      --mode)
        [[ "$command_kind" == "discover-specs" ]] || return 2
        shift
        [[ $# -gt 0 ]] || return 2
        PARSED_MODE="$1"
        ;;
      *) return 2 ;;
    esac
    shift
  done
  [[ -n "$PARSED_SESSION_ID" && -n "$PARSED_CONTROL_FILE" && -n "$PARSED_PACKET_FILE" ]]
}

discover_specs() {
  if [[ $# -eq 1 && ( "$1" == "-h" || "$1" == "--help" ) ]]; then
    discover_usage
    return 0
  fi
  parse_packet_command_args discover-specs "$@" || { discover_usage >&2; return 2; }
  valid_session_id "$PARSED_SESSION_ID" || { discover_usage >&2; return 2; }
  [[ -n "$PARSED_MODE" ]] || { discover_usage >&2; return 2; }
  validate_packet_internal "$PARSED_SESSION_ID" "$PARSED_CONTROL_FILE" "$PARSED_PACKET_FILE" visible || return 1
  local root
  local specs_root
  local candidate
  local canonical_candidate
  root="$(jq -r '.repositoryRoot' <<< "$VALIDATED_PACKET_JSON")"
  specs_root="$root/specs"
  # A specs root that is itself a symlink can redirect the entire discovery
  # scope outside the committed canonical repository. Refuse fail-closed and
  # emit no discovery scope or spec entries.
  if [[ -L "$specs_root" ]]; then
    printf 'REPOSITORY DISCOVERY REFUSED reason=SPECS_ROOT_SYMLINK affinity=unchanged repoLocalSideEffects=zero\n'
    return 1
  fi
  printf 'DISCOVERY SCOPE mode=%s root=%s\n' "$PARSED_MODE" "$specs_root"
  [[ -d "$specs_root" ]] || return 0
  for candidate in "$specs_root"/*; do
    [[ -d "$candidate" ]] || continue
    # Exclude any child whose canonical target escapes the selected repository
    # root (for example a child symlinked outside the repository); contained
    # siblings remain discoverable and the escaped path is never emitted.
    canonical_candidate="$(canonical_existing_path "$candidate" 2>/dev/null || true)"
    if [[ -z "$canonical_candidate" ]] || \
       ! physical_path_is_contained "$canonical_candidate" "$root"; then
      continue
    fi
    printf '%s\n' "$candidate"
  done
}

mirror_session() {
  if [[ $# -eq 1 && ( "$1" == "-h" || "$1" == "--help" ) ]]; then
    mirror_usage
    return 0
  fi
  parse_packet_command_args mirror-session "$@" || { mirror_usage >&2; return 2; }
  valid_session_id "$PARSED_SESSION_ID" || { mirror_usage >&2; return 2; }
  validate_packet_internal "$PARSED_SESSION_ID" "$PARSED_CONTROL_FILE" "$PARSED_PACKET_FILE" visible || return 1
  local root
  local session_dir
  local session_file
  local temp_file
  local timestamp
  root="$(jq -r '.repositoryRoot' <<< "$VALIDATED_PACKET_JSON")"
  session_dir="$root/.specify/memory"
  session_file="$session_dir/bubbles.session.json"
  if path_has_symlink_component "$session_file"; then
    printf 'REPOSITORY MIRROR REFUSED reason=SESSION_MIRROR_SYMLINK repoLocalSideEffects=zero\n'
    return 1
  fi
  mkdir -p "$session_dir" || return 3
  if path_has_symlink_component "$session_file" || \
     [[ "$(physical_directory "$session_dir" 2>/dev/null || true)" != "$session_dir" ]]; then
    printf 'REPOSITORY MIRROR REFUSED reason=SESSION_MIRROR_SYMLINK repoLocalSideEffects=zero\n'
    return 1
  fi
  if [[ -f "$session_file" ]] && ! jq -e 'type == "object"' "$session_file" >/dev/null 2>&1; then
    printf 'REPOSITORY MIRROR REFUSED reason=SESSION_MIRROR_MALFORMED repoLocalSideEffects=zero\n'
    return 1
  fi
  if [[ -f "$session_file" ]] && jq -e \
    --arg session_id "$PARSED_SESSION_ID" \
    --arg repository_root "$root" \
    '.repositoryBindingMirror.repositoryResolution.sessionId == $session_id and
     .repositoryBindingMirror.repositoryRoot != $repository_root' \
    "$session_file" >/dev/null 2>&1; then
    printf 'REPOSITORY MIRROR DRIFT authority=external-control repair=overwrite\n'
  fi
  timestamp="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  temp_file="$(mktemp "$session_dir/.bubbles-session.XXXXXX")" || return 3
  if [[ -f "$session_file" ]]; then
    jq --argjson binding "$VALIDATED_PACKET_JSON" --arg timestamp "$timestamp" \
      '.repositoryBindingMirror = ($binding + {
         mirroredControlRevision: $binding.repositoryResolution.controlRevision,
         mirroredAt: $timestamp
       })' "$session_file" > "$temp_file" || { rm -f "$temp_file"; return 3; }
  else
    jq -n --argjson binding "$VALIDATED_PACKET_JSON" --arg timestamp "$timestamp" \
      '{repositoryBindingMirror: ($binding + {
         mirroredControlRevision: $binding.repositoryResolution.controlRevision,
         mirroredAt: $timestamp
       })}' > "$temp_file" || { rm -f "$temp_file"; return 3; }
  fi
  mv "$temp_file" "$session_file" || { rm -f "$temp_file"; return 3; }
  printf 'REPOSITORY MIRROR UPDATED repository=%s revision=%s\n' \
    "$(jq -r '.repositoryAlias' <<< "$VALIDATED_PACKET_JSON")" \
    "$(jq -r '.repositoryResolution.controlRevision' <<< "$VALIDATED_PACKET_JSON")"
}

main() {
  [[ $# -gt 0 ]] || { usage >&2; return 2; }
  local subcommand="$1"
  shift
  case "$subcommand" in
    preflight) preflight "$@" ;;
    validate-packet) validate_packet "$@" ;;
    discover-specs) discover_specs "$@" ;;
    mirror-session) mirror_session "$@" ;;
    -h|--help|help) usage ;;
    *) fail_usage "unknown subcommand: $subcommand" ;;
  esac
}

ACTIVE_LOCK_DIR=""
ACTIVE_OBSERVED_SIGNALS_JSON="[]"
VALIDATED_PACKET_JSON=""
PARSED_SESSION_ID=""
PARSED_CONTROL_FILE=""
PARSED_PACKET_FILE=""
PARSED_MODE=""
main "$@"
exit $?
