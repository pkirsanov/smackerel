#!/usr/bin/env bash
set -euo pipefail

# post-cert-spec-edit-guard.sh
#
# Gate G088 - post_certification_spec_edit_gate.
#
# Certified specs must not silently change planning truth after certification.
# For specs whose top-level state.status is done or legacy read-only
# done_with_concerns compatibility, this guard checks git history for commits after
# top-level certifiedAt touching spec.md, design.md, scopes.md,
# scopes/_index.md, or per-scope scope.md files. G092 makes
# done_with_concerns read-only compatibility only; touched or recertified
# specs migrate to done plus observations or blocked.
#
# Usage:
#   bash bubbles/scripts/post-cert-spec-edit-guard.sh <specDir> [--quiet]
#
# Exit codes:
#   0  clean
#   1  one or more G088 post-certification edit violations
#   2  runtime error, invalid arguments, missing state, malformed JSON/date, or
#      git inspection error

QUIET="false"
SPEC_DIR_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/post-cert-spec-edit-guard.sh <specDir> [--quiet]

Arguments:
  <specDir>  Spec directory containing state.json.

Optional:
  --quiet    Suppress success output.
  -h, --help Print this usage and exit.

Exit codes:
  0 = clean
  1 = G088 post-certification planning edit violation
  2 = runtime error, invalid arguments, missing state, malformed JSON/date, or git inspection error
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
    --*)
      echo "post-cert-spec-edit-guard: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$SPEC_DIR_INPUT" ]]; then
        echo "post-cert-spec-edit-guard: only one specDir may be supplied" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR_INPUT" ]]; then
  echo "post-cert-spec-edit-guard: missing required specDir" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "post-cert-spec-edit-guard: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR_INPUT" ]]; then
  echo "post-cert-spec-edit-guard: specDir does not exist: $SPEC_DIR_INPUT" >&2
  exit 2
fi

SPEC_DIR_ABS="$(cd "$SPEC_DIR_INPUT" && pwd -P)"
STATE_FILE="$SPEC_DIR_ABS/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
  echo "post-cert-spec-edit-guard: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "post-cert-spec-edit-guard: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

rfc3339_epoch() {
  local value="$1"
  jq -nr --arg d "$value" 'try ($d | fromdateiso8601 | floor) catch empty'
}

state_type_for() {
  local expr="$1"
  jq -r "$expr" "$STATE_FILE"
}

status_type="$(state_type_for 'if has("status") then (.status | type) else "missing" end')"
if [[ "$status_type" != "string" ]]; then
  echo "post-cert-spec-edit-guard: state.status must be a string in $STATE_FILE" >&2
  exit 2
fi

status="$(state_type_for '.status')"
spec_rel="$SPEC_DIR_INPUT"

legacy_status_compatible() {
  jq -e '(.legacyStatusCompatibility == true) or (.certification.legacyStatusCompatibility == true)' "$STATE_FILE" >/dev/null 2>&1
}

recertification_active() {
  jq -e '
    (.requiresRevalidation == true)
    or (.touchedForRecertification == true)
    or (.recertifying == true)
    or (.certification.touched == true)
    or (.certification.recertifying == true)
  ' "$STATE_FILE" >/dev/null 2>&1
}

if [[ "$status" != "done" && "$status" != "done_with_concerns" ]]; then
  if [[ "$QUIET" != "true" ]]; then
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status is not certified done"
  fi
  exit 0
fi

if [[ "$status" == "done_with_concerns" ]]; then
  if ! legacy_status_compatible || recertification_active; then
    echo "G092 strict_terminal_status_gate violation: $spec_rel uses new or recertified done_with_concerns; migrate to status done with observations[] or status blocked" >&2
    exit 1
  fi
fi

if ! command -v git >/dev/null 2>&1; then
  echo "post-cert-spec-edit-guard: git is required but not found in PATH" >&2
  exit 2
fi

if ! REPO_ROOT="$(git -C "$SPEC_DIR_ABS" rev-parse --show-toplevel 2>/dev/null)"; then
  echo "post-cert-spec-edit-guard: specDir is not inside a git worktree: $SPEC_DIR_INPUT" >&2
  exit 2
fi

REPO_ROOT="$(cd "$REPO_ROOT" && pwd -P)"

if [[ "$SPEC_DIR_ABS" != "$REPO_ROOT"/* ]]; then
  echo "post-cert-spec-edit-guard: specDir is outside the git repository root: $SPEC_DIR_ABS" >&2
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

certified_type="$(state_type_for 'if has("certifiedAt") then (.certifiedAt | type) else "missing" end')"
if [[ "$certified_type" != "string" ]]; then
  echo "post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec $spec_rel (status=$status)" >&2
  exit 2
fi

certified_at="$(state_type_for '.certifiedAt')"
if [[ "$certified_at" =~ ^[[:space:]]*$ ]]; then
  echo "post-cert-spec-edit-guard: certifiedAt is empty for certified spec $spec_rel" >&2
  exit 2
fi

certified_epoch="$(rfc3339_epoch "$certified_at")"
if [[ -z "$certified_epoch" ]]; then
  echo "post-cert-spec-edit-guard: malformed certifiedAt timestamp for $spec_rel: $certified_at" >&2
  exit 2
fi

# ---------------------------------------------------------------------------
# IMP-106 SCOPE-3 (DOM-LINEAGE) — advisory domain-invariant re-verification flag.
#
# When a domain invariant's `rule` text in the domainModel: block changes AFTER
# certification, every scenario in this spec's scenario-manifest.json whose
# `invariantRefs` includes that invariant id is FLAGGED for re-verification.
# This realizes "a source changed -> dependent facts must be re-examined" on the
# artifacts Bubbles already owns (see improvements/IMP-106-...md SCOPE-3 and
# bubbles/schemas/scenario-manifest.schema.json).
#
# ADVISORY ONLY: it prints notes to stdout and NEVER changes the exit code. It
# is a strict no-op unless BOTH (a) a domainModel: block exists
# (.github/bubbles-project.yaml or bubbles-project.yaml, inline or via `$ref:`)
# AND (b) at least one scenario in this spec carries invariantRefs. It reuses
# G088's post-certifiedAt edit-detection shape: compare the domain-model source
# as-of-certification (git) against the current working tree. YAML invariant
# parsing is best-effort (advisory); the JSON manifest is parsed with jq.
# ---------------------------------------------------------------------------
emit_invariant_reverification_advisory() {
  local manifest="$SPEC_DIR_ABS/scenario-manifest.json"
  [[ -f "$manifest" ]] || return 0

  # (b) advisory posture: at least one scenario must carry a non-empty
  # invariantRefs array, else this spec has not opted into invariant lineage.
  if ! jq -e '[.scenarios[]? | select((.invariantRefs // []) | type == "array" and length > 0)] | length > 0' "$manifest" >/dev/null 2>&1; then
    return 0
  fi

  # (a) locate the domainModel: source — the project yaml (inline) or its $ref.
  local dm_src="" candidate
  for candidate in ".github/bubbles-project.yaml" "bubbles-project.yaml"; do
    if [[ -f "$REPO_ROOT/$candidate" ]] && grep -qE '^[[:space:]]*domainModel:' "$REPO_ROOT/$candidate"; then
      dm_src="$candidate"
      break
    fi
  done
  [[ -n "$dm_src" ]] || return 0

  # Best-effort `$ref:` indirection: if the domainModel block points at another
  # file that actually carries invariant declarations, parse that file instead.
  local dm_ref
  # shellcheck disable=SC2016  # literal `$ref` is a YAML key, not a shell expansion
  dm_ref="$(grep -E '\$ref:' "$REPO_ROOT/$dm_src" 2>/dev/null | head -n 1 | sed -E 's/.*\$ref:[[:space:]]*//; s/[[:space:]}].*$//' || true)"
  if [[ -n "$dm_ref" && -f "$REPO_ROOT/$dm_ref" ]] && grep -qE '(^|[[:space:]])id:[[:space:]]*INV-|invariants:' "$REPO_ROOT/$dm_ref" 2>/dev/null; then
    dm_src="$dm_ref"
  fi

  # Best-effort YAML parser: emit "<INV-id>\t<rule text>" for each invariant.
  # Tracks the current `id: INV-*` and pairs it with the next `rule:` value.
  # shellcheck disable=SC2016  # awk program is a literal; $0/$1 are awk fields
  local parse_awk='
    /^[[:space:]]*-?[[:space:]]*id:[[:space:]]*INV-/ {
      line=$0
      sub(/^[[:space:]]*-?[[:space:]]*id:[[:space:]]*/, "", line)
      sub(/[[:space:]]*(#.*)?$/, "", line)
      cur=line
      next
    }
    /^[[:space:]]*rule:[[:space:]]*/ {
      if (cur != "") {
        r=$0
        sub(/^[[:space:]]*rule:[[:space:]]*/, "", r)
        print cur "\t" r
        cur=""
      }
      next
    }
  '

  # Current (working-tree) invariant rules — captures uncommitted edits too.
  local cur_pairs=""
  cur_pairs="$(awk "$parse_awk" "$REPO_ROOT/$dm_src" 2>/dev/null || true)"

  # Certified (as-of-certifiedAt) invariant rules from git history.
  local cert_commit="" cert_pairs=""
  cert_commit="$(git -C "$REPO_ROOT" rev-list -1 --before="$certified_at" HEAD -- "$dm_src" 2>/dev/null || true)"
  if [[ -n "$cert_commit" ]]; then
    cert_pairs="$(git -C "$REPO_ROOT" show "$cert_commit:$dm_src" 2>/dev/null | awk "$parse_awk" 2>/dev/null || true)"
  fi
  # No pre-certification snapshot => no prior rule text to diff => nothing to
  # flag (a newly-introduced model is not a post-cert rule EDIT).
  [[ -n "$cert_pairs" ]] || return 0

  # Domain-invariant ids referenced by this spec's scenarios.
  local ref_ids=""
  ref_ids="$(jq -r '[.scenarios[]? | (.invariantRefs // []) | .[]?] | unique | .[]?' "$manifest" 2>/dev/null || true)"
  [[ -n "$ref_ids" ]] || return 0

  local flagged_any=0 rid cur_rule cert_rule scenarios
  while IFS= read -r rid; do
    [[ -n "$rid" ]] || continue
    cur_rule="$(printf '%s\n' "$cur_pairs"  | awk -F'\t' -v id="$rid" '$1==id{sub(/^[^\t]*\t/,""); print; exit}' || true)"
    cert_rule="$(printf '%s\n' "$cert_pairs" | awk -F'\t' -v id="$rid" '$1==id{sub(/^[^\t]*\t/,""); print; exit}' || true)"
    # Flag ONLY when an existing invariant's rule text was edited: both sides
    # present and different. An unchanged rule is never flagged, so editing an
    # UNRELATED invariant cannot flag a scenario that does not reference it.
    if [[ -n "$cur_rule" && -n "$cert_rule" && "$cur_rule" != "$cert_rule" ]]; then
      scenarios="$(jq -r --arg inv "$rid" '.scenarios[]? | select((.invariantRefs // []) | index($inv)) | (.id // .scenarioId // "unknown")' "$manifest" 2>/dev/null | tr '\n' ' ' | sed -E 's/[[:space:]]+$//' || true)"
      if [[ "$flagged_any" -eq 0 ]]; then
        echo "post-cert-spec-edit-guard: ADVISORY (IMP-106 DOM-LINEAGE) — domainModel invariant rule(s) changed after certification; re-verify dependent scenarios/scopes for $spec_rel"
        flagged_any=1
      fi
      echo "  - invariant $rid rule edited after certifiedAt=$certified_at; flagged scenarios: ${scenarios:-<none-mapped>}"
    fi
  done <<< "$ref_ids"

  return 0
}

emit_invariant_reverification_advisory

requires_type="$(state_type_for 'if has("requiresRevalidation") then (.requiresRevalidation | type) else "missing" end')"
requires_revalidation="false"
if [[ "$requires_type" == "boolean" ]]; then
  requires_revalidation="$(state_type_for 'if .requiresRevalidation == true then "true" else "false" end')"
elif [[ "$requires_type" != "missing" ]]; then
  echo "post-cert-spec-edit-guard: requiresRevalidation must be boolean when present in $STATE_FILE" >&2
  exit 2
fi

latest_current_review="$(jq -r '
  [
    .executionHistory[]?
    | select((.agent? // "") == "bubbles.spec-review")
    | select(((.reviewStatus? // .reviewVerdict? // .verdict? // "") | ascii_upcase) == "CURRENT")
    | (.runCompletedAt? // .completedAt? // .reviewedAt? // empty)
    | select(type == "string" and length > 0)
  ]
  | sort
  | last // ""
' "$STATE_FILE")"

latest_current_review_epoch=""
if [[ -n "$latest_current_review" ]]; then
  latest_current_review_epoch="$(rfc3339_epoch "$latest_current_review")"
  if [[ -z "$latest_current_review_epoch" ]]; then
    echo "post-cert-spec-edit-guard: malformed bubbles.spec-review CURRENT timestamp for $spec_rel: $latest_current_review" >&2
    exit 2
  fi
fi

tracked_paths=()

add_tracked_file() {
  local rel="$1"
  if [[ -f "$REPO_ROOT/$rel" ]]; then
    tracked_paths+=("$rel")
  fi
}

add_tracked_file "$spec_rel/spec.md"
add_tracked_file "$spec_rel/design.md"
add_tracked_file "$spec_rel/scopes.md"
add_tracked_file "$spec_rel/scopes/_index.md"

if [[ -d "$SPEC_DIR_ABS/scopes" ]]; then
  while IFS= read -r scope_file; do
    tracked_paths+=("$(relative_path "$scope_file")")
  done < <(find "$SPEC_DIR_ABS/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)
fi

if [[ "${#tracked_paths[@]}" -eq 0 ]]; then
  echo "post-cert-spec-edit-guard: no planning truth files found under $spec_rel" >&2
  exit 2
fi

LOG_FILE="$(mktemp -t g088-git-log-XXXXXXXX)"
DIFF_FILE="$(mktemp -t g088-git-diff-XXXXXXXX)"
cleanup() {
  rm -f "$LOG_FILE" "$DIFF_FILE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# IMP-049 SCOPE-1 Option C — placeholder-aware hunk classification.
#
# Path-level detection above cannot distinguish a mandated PII/genericization
# redaction from a requirements change, because every inspection is
# --name-only. A consumer under a policy that forbids real hostnames, IPs,
# operator usernames, or tailnet identifiers in committed files must edit
# certified planning files to obey it, and therefore accumulates G088 findings
# with no planning-truth drift at all. The two policies were individually
# correct and jointly unsatisfiable.
#
# This classifier reads the actual hunks and clears an edit ONLY when every
# changed line is a concrete-value -> placeholder substitution. It is
# FAIL-CLOSED by construction (IMP-049 R1: a filter bug that silently
# suppresses a real requirements change is a worse outcome than a noisy gate):
#   - unreadable or empty diff            -> NOT redaction (stays a finding)
#   - added/removed line counts differ    -> NOT redaction
#   - any pair not equal after masking    -> NOT redaction
#   - a pair where the old line carried no concrete value, or the new line
#     gained no placeholder                -> NOT redaction
# Only a positively-proven substitution is cleared; silence never clears.
#
# This is deliberately NOT a declared-marker channel (Option B). A marker the
# guard does not verify is indistinguishable from --skip, which this framework
# does not ship, and G088 already carries one unvalidated escape in
# requiresRevalidation. Nothing here is self-asserted: the diff is the evidence.
# ---------------------------------------------------------------------------

# Concrete environment-specific values a redaction policy compels scrubbing.
_G088_CONCRETE_RE='([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})|(/(Users|home)/[A-Za-z0-9_.-]+)|([A-Za-z0-9][A-Za-z0-9-]*(\.[A-Za-z0-9-]+)+)'
# Placeholder shapes a redaction substitutes in.
_G088_PLACEHOLDER_RE='(<[A-Za-z0-9_. -]+>)|(\$\{[A-Za-z0-9_]+\})|(REDACTED)|(redacted)|(example\.(com|org|net))|(placeholder)'
# A file reference such as design.md or state.json satisfies the hostname shape
# above. Neutralize those FIRST so swapping a doc reference can never be
# mistaken for scrubbing a hostname.
#
# No pattern here uses \b: that is a GNU extension, and BSD sed on macOS does
# not honour it, which would silently leave file references looking like
# hostnames. Over-neutralizing is the safe direction — it can only withhold a
# clear, never grant one.
_G088_FILENAME_RE='[A-Za-z0-9_-]+\.(md|json|sh|ya?ml|txt|py|rs|ts|js|toml|lock|cfg|ini)'

# The delimiter must appear in NEITHER regex: both carry '|' alternation and
# the concrete pattern carries '/' in its /Users|/home branch.
_g088_norm() {
  printf '%s' "$1" | sed -E "s%${_G088_FILENAME_RE}%#%g"
}

_g088_mask() {
  _g088_norm "$1" | sed -E -e "s%${_G088_PLACEHOLDER_RE}%@%g" -e "s%${_G088_CONCRETE_RE}%@%g"
}

_g088_has() {
  _g088_norm "$1" | grep -Eq "$2"
}

# _g088_is_redaction_only <commit-or-WORKTREE> <file> -> 0 when every changed
# line in that edit is a proven concrete-value -> placeholder substitution.
_g088_is_redaction_only() {
  local commit="$1" file="$2" raw="" old_lines="" new_lines="" line
  if [[ "$commit" == "WORKTREE" ]]; then
    raw="$(git -C "$REPO_ROOT" diff --unified=0 -- "$file" 2>/dev/null || true)"
    raw="$raw
$(git -C "$REPO_ROOT" diff --cached --unified=0 -- "$file" 2>/dev/null || true)"
  else
    raw="$(git -C "$REPO_ROOT" show --unified=0 --format='' "$commit" -- "$file" 2>/dev/null || true)"
  fi

  # Fail closed on an unreadable or empty diff.
  [[ -n "${raw//[[:space:]]/}" ]] || return 1

  while IFS= read -r line; do
    case "$line" in
      ---*|+++*) continue ;;
      -*) old_lines="${old_lines}${line:1}"$'\n' ;;
      +*) new_lines="${new_lines}${line:1}"$'\n' ;;
    esac
  done <<< "$raw"

  local old_count new_count
  old_count="$(printf '%s' "$old_lines" | grep -c '' 2>/dev/null || echo 0)"
  new_count="$(printf '%s' "$new_lines" | grep -c '' 2>/dev/null || echo 0)"
  # A pure substitution rewrites lines in place; an insertion or deletion is not
  # a redaction and must remain a finding.
  [[ "$old_count" -gt 0 && "$old_count" -eq "$new_count" ]] || return 1

  local i=1 o n
  while [[ "$i" -le "$old_count" ]]; do
    o="$(printf '%s' "$old_lines" | sed -n "${i}p")"
    n="$(printf '%s' "$new_lines" | sed -n "${i}p")"
    # The old line must actually carry a concrete value and the new line must
    # actually gain a placeholder, otherwise this is some other edit that
    # merely happens to normalize equal.
    _g088_has "$o" "$_G088_CONCRETE_RE" || return 1
    _g088_has "$n" "$_G088_PLACEHOLDER_RE" || return 1
    [[ "$(_g088_mask "$o")" == "$(_g088_mask "$n")" ]] || return 1
    i=$((i + 1))
  done
  return 0
}

if ! git -C "$REPO_ROOT" log --format='@@G088@@%x09%H%x09%cI%x09%s' --name-only --since="$certified_at" -- "${tracked_paths[@]}" > "$LOG_FILE"; then
  echo "post-cert-spec-edit-guard: git log inspection failed for $spec_rel" >&2
  exit 2
fi

if ! git -C "$REPO_ROOT" diff --name-only -- "${tracked_paths[@]}" > "$DIFF_FILE"; then
  echo "post-cert-spec-edit-guard: git diff inspection failed for $spec_rel" >&2
  exit 2
fi

if ! git -C "$REPO_ROOT" diff --cached --name-only -- "${tracked_paths[@]}" >> "$DIFF_FILE"; then
  echo "post-cert-spec-edit-guard: staged git diff inspection failed for $spec_rel" >&2
  exit 2
fi

post_cert_entries=()
post_cert_commits=()
post_cert_files=()
current_hash=""
current_date=""
current_subject=""

while IFS= read -r line; do
  if [[ "$line" == @@G088@@$'\t'* ]]; then
    IFS=$'\t' read -r _ current_hash current_date current_subject <<< "$line"
    continue
  fi

  if [[ -n "$line" && -n "$current_hash" ]]; then
    post_cert_entries+=("commit=$current_hash date=$current_date file=$line subject=$current_subject")
    post_cert_commits+=("$current_hash")
    post_cert_files+=("$line")
  fi
done < "$LOG_FILE"

while IFS= read -r dirty_path; do
  [[ -n "$dirty_path" ]] || continue
  post_cert_entries+=("commit=WORKTREE date=uncommitted file=$dirty_path subject=uncommitted planning truth edit")
  post_cert_commits+=("WORKTREE")
  post_cert_files+=("$dirty_path")
done < "$DIFF_FILE"

# Partition the path-level detections into proven redactions and real findings.
# The redaction set is REPORTED, never hidden: a cleared edit that nobody can
# see would be the same silence this gate exists to interrupt.
drift_entries=()
redaction_entries=()
_idx=0
while [[ "$_idx" -lt "${#post_cert_entries[@]}" ]]; do
  if _g088_is_redaction_only "${post_cert_commits[$_idx]}" "${post_cert_files[$_idx]}"; then
    redaction_entries+=("${post_cert_entries[$_idx]}")
  else
    drift_entries+=("${post_cert_entries[$_idx]}")
  fi
  _idx=$((_idx + 1))
done
post_cert_entries=("${drift_entries[@]+"${drift_entries[@]}"}")

# ---------------------------------------------------------------------------
# IMP-049 SCOPE-3 — carried-finding ledger.
#
# The measured downstream state was 40 certified specs whose findings were being
# carried with no record anywhere. Carrying is the option actually taken and the
# one the framework never recorded, so the erosion was invisible.
#
# A carry is a RECORD, never an EXEMPTION. Declaring one does NOT change the
# exit code: the finding still fails. That is deliberate — a ledger entry that
# cleared a blocking gate would be exactly the unvalidated self-assertion this
# proposal refuses (see requiresRevalidation, and Option B). What the ledger
# buys is attribution: who decided, why, and when, plus an aggregate that can be
# argued about instead of a silence that becomes the norm.
#
# Shape, in state.json:
#   "g088Carry": [ { "file": "...", "reason": "...", "owner": "...", "date": "..." } ]
# An entry missing any of the four fields is malformed and does NOT count as
# declared, so a half-filled ledger cannot launder a finding into looking owned.
# ---------------------------------------------------------------------------
carry_files=""
if jq -e 'has("g088Carry") and (.g088Carry | type == "array")' "$STATE_FILE" >/dev/null 2>&1; then
  carry_files="$(jq -r '
    [ .g088Carry[]?
      | select(((.file // "") | tostring | length) > 0
            and ((.reason // "") | tostring | length) > 0
            and ((.owner // "") | tostring | length) > 0
            and ((.date // "") | tostring | length) > 0)
      | .file ] | .[]?' "$STATE_FILE" 2>/dev/null || true)"
fi

carry_declared=0
carry_undeclared=0
_i=0
while [[ "$_i" -lt "${#post_cert_entries[@]}" ]]; do
  _entry_file="${post_cert_entries[$_i]#*file=}"
  _entry_file="${_entry_file%% subject=*}"
  if [[ -n "$carry_files" ]] && printf '%s\n' "$carry_files" | grep -Fxq "$_entry_file"; then
    carry_declared=$((carry_declared + 1))
  else
    carry_undeclared=$((carry_undeclared + 1))
  fi
  _i=$((_i + 1))
done

if [[ "${#post_cert_entries[@]}" -gt 0 && "$requires_revalidation" == "true" ]]; then
  if [[ "$QUIET" != "true" ]]; then
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status requiresRevalidation=true postCertEdits=${#post_cert_entries[@]}"
  fi
  exit 0
fi

if [[ "${#post_cert_entries[@]}" -gt 0 ]]; then
  echo "G088 post_certification_spec_edit_gate violation: certified planning truth changed after certifiedAt" >&2
  echo "  spec: $spec_rel" >&2
  echo "  status: $status" >&2
  echo "  certifiedAt: $certified_at" >&2
  echo "  trackedFiles: ${#tracked_paths[@]}" >&2
  echo "  postCertEdits: ${#post_cert_entries[@]}" >&2
  echo "  clearedAsRedaction: ${#redaction_entries[@]}" >&2
  echo "  carriedDeclared: $carry_declared" >&2
  echo "  carriedUndeclared: $carry_undeclared" >&2
  echo "  remediation: demote status out of done, set requiresRevalidation:true, or complete a current bubbles.spec-review recertification and update certifiedAt after the edit" >&2
  echo "  G092: legacy done_with_concerns is read-only compatibility only; touched or recertified specs must migrate to done plus observations or blocked" >&2
  echo "  commits/files:" >&2
  for entry in "${post_cert_entries[@]}"; do
    echo "    - $entry" >&2
  done
  if [[ "${#redaction_entries[@]}" -gt 0 ]]; then
    echo "  cleared as concrete-value->placeholder redaction (not planning drift):" >&2
    for entry in "${redaction_entries[@]}"; do
      echo "    ~ $entry" >&2
    done
  fi
  exit 1
fi

if [[ "$QUIET" != "true" ]]; then
  _redaction_note=""
  if [[ "${#redaction_entries[@]}" -gt 0 ]]; then
    _redaction_note=" clearedAsRedaction=${#redaction_entries[@]}"
  fi
  if [[ -n "$latest_current_review" && "$latest_current_review_epoch" -le "$certified_epoch" ]]; then
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status certifiedAt=$certified_at currentSpecReview=$latest_current_review trackedFiles=${#tracked_paths[@]}${_redaction_note}"
  else
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status certifiedAt=$certified_at trackedFiles=${#tracked_paths[@]}${_redaction_note}"
  fi
fi

exit 0