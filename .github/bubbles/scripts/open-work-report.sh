#!/usr/bin/env bash
# open-work-report.sh (IMP-033 / SCOPE-3 — gaps WIP-1, WIP-2)
# ---------------------------------------------------------------------------
# Answer one question for one repository: WHAT IS STILL OPEN, who owns the next
# move, and what that move is.
#
# The register this reads is `.specify/memory/open-work.md` — a COMMITTED
# markdown table. It is committed on purpose: a record of open work that lives
# only in a chat transcript or an uncommitted file is lost at exactly the moment
# it is needed, which is the failure this script exists to close.
#
# TWO ROW CLASSES, AND ONLY ONE OF THEM IS AUTHORED
# -------------------------------------------------
#   DERIVED  (kind = spec | bug | imp)
#       Never authored. Recomputed on every run from the artifacts that already
#       own the truth. Spec and bug rows come from `work-tracker-project.sh`,
#       the existing per-feature projection of `state.json` — this script does
#       NOT open `state.json` itself, because a second state reader is a second
#       source of truth, which is the status-mirror failure IMP-032 removed.
#       IMP rows come from `improvements/INDEX.md`, the register that already
#       owns improvement status.
#
#   RESIDUE  (kind = residue)
#       The only authored class. Residue is work that was NOTICED and NOT FILED:
#       it has no spec, no bug, no IMP, and therefore no artifact to derive from.
#       A residue row without both a next-owner and a next-action is a defect
#       (`--lint`), because a row that cannot be acted on is a to-do list entry
#       wearing a register's clothes.
#
# CLOSED ROWS ARE DELETED, NOT TOMBSTONED. A residue row disappears when the
# work is done or when it graduates into a spec, bug, or IMP — at which point
# the derived projection covers it. The register answers "what is still open",
# and a growing tail of closed rows destroys that answer within a month. Git
# history is the audit trail for what was removed and when.
#
# WHY MARKDOWN AND NOT JSON OR YAML
# ---------------------------------
# `doctor` reports PyYAML and jsonschema MISSING on ordinary developer machines,
# and the framework's guards fail closed on a missing dependency. A register
# that needs an optional parser to be readable disappears exactly when the
# environment is degraded — which is when carried-over work matters most.
# `improvements/INDEX.md` sets the precedent. Machine consumers get `--format
# json` by projection instead.
#
# TERMINAL STATUS IS NOT DECIDED HERE. `is-terminal-for-mode.sh` already owns
# that question, including the mode ceilings (`validated`, `docs_updated`,
# `delivered_pending_activation`, …) that are completed-for-mode and are NOT
# backlog. This script asks it rather than hardcoding a list that would drift.
#
# Usage:
#   open-work-report.sh [--repo-root <path>] [--format text|json]
#                       [--register <path>] [--lint] [--quiet-empty]
#
# Exit codes:
#   0 = report printed (this is a report, not a gate)
#   1 = --lint requested AND a register defect was found
#   2 = usage error
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'EOF'
Usage: open-work-report.sh [options]

  --repo-root <path>   Repository to inspect (default: walk up from cwd to a
                       directory containing .specify/memory, else the script's
                       own repo).
  --format text|json   Output format (default: text).
  --register <path>    Override the register path (default:
                       <repo-root>/.specify/memory/open-work.md).
  --lint               Exit 1 if the register holds a defective row. Without
                       this flag defects are reported and the exit stays 0.
  --quiet-empty        Print nothing when there is no open work at all
                       (used by embedding reports).
  -h, --help           This message.

Exit: 0 report printed | 1 --lint found a defect | 2 usage error.
EOF
}

repo_root=""
format="text"
register_override=""
do_lint=false
quiet_empty=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      # Empty is a usage error, not a silent fall-through to the walk-upward
      # default: a caller passing an unset variable means to name a repository,
      # and binding to the ambient one instead reports on the wrong repo.
      repo_root="${2:-}"
      [[ -n "$repo_root" ]] || { echo "open-work: --repo-root requires a non-empty path" >&2; exit 2; }
      shift 2 ;;
    --format) format="${2:-}"; shift 2 ;;
    --register) register_override="${2:-}"; shift 2 ;;
    --lint) do_lint=true; shift ;;
    --quiet-empty) quiet_empty=true; shift ;;
    -h | --help) usage; exit 0 ;;
    *) echo "open-work-report: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$format" in
  text | json) ;;
  *) echo "open-work-report: --format must be text or json (got: $format)" >&2; exit 2 ;;
esac

# Resolve the repo root the same way trajectory-inspector.sh does, so both
# surfaces agree on which repository they are describing.
if [[ -z "$repo_root" ]]; then
  d="$PWD"
  while [[ "$d" != "/" ]]; do
    if [[ -d "$d/.specify/memory" ]]; then repo_root="$d"; break; fi
    d="$(dirname "$d")"
  done
  [[ -n "$repo_root" ]] || repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi
if [[ ! -d "$repo_root" ]]; then
  echo "open-work-report: repo root not found: $repo_root" >&2
  exit 2
fi
repo_root="$(cd "$repo_root" && pwd)"

REGISTER="${register_override:-$repo_root/.specify/memory/open-work.md}"
SPECS_DIR="$repo_root/specs"
IMP_INDEX="$repo_root/improvements/INDEX.md"
PROJECT_SH="$SCRIPT_DIR/work-tracker-project.sh"
TERMINAL_SH="$SCRIPT_DIR/is-terminal-for-mode.sh"
TODAY="$(date -u +%Y-%m-%d)"

have_jq=false
command -v jq >/dev/null 2>&1 && have_jq=true

# Rows accumulate as tab-separated records so bash 3.2 can carry them without
# associative arrays: id \t title \t kind \t ref \t state \t owner \t action \t opened \t lastSeen \t derived
ROWS=""
DEFECTS=""

add_row() {
  ROWS="${ROWS}${1}"$'\t'"${2}"$'\t'"${3}"$'\t'"${4}"$'\t'"${5}"$'\t'"${6}"$'\t'"${7}"$'\t'"${8}"$'\t'"${9}"$'\t'"${10}"$'\n'
}

add_defect() {
  DEFECTS="${DEFECTS}${1}"$'\n'
}

# A field is "unset" when it is empty or one of the placeholder glyphs a human
# reaches for when they have nothing to say. Those are exactly the rows the
# lint exists to reject, so treat them as absent rather than as content.
is_placeholder() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')" in
    "" | "-" | "—" | "--" | "?" | "n/a" | "na" | "tbd" | "todo" | "none") return 0 ;;
  esac
  return 1
}

trim() {
  printf '%s' "$1" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

# Count the COLUMN DELIMITERS in a markdown table row. A pipe written as `\|` is
# cell content, not a column break, so it is removed before counting — that is
# what lets the check reject an unescaped pipe without banning pipes outright.
count_delimiters() {
  local stripped="${1//\\|/}"
  local pipes="${stripped//[!|]/}"
  printf '%s' "${#pipes}"
}

# --- DERIVED: spec and bug rows, via the existing per-feature projection ------
# work-tracker-project.sh is CONSUMED unchanged. Its header states the
# projection is a pure function of state.json with byte-identical output for
# identical input, which is the property being relied on here. Any future edit
# to it is a contract change for every consumer, not just this one.
derive_artifact_rows() {
  [[ -d "$SPECS_DIR" ]] || return 0
  if ! $have_jq; then
    add_defect "jq is not installed, so spec and bug rows could not be derived. The authored register below is still complete; the derived section is not."
    return 0
  fi
  [[ -f "$PROJECT_SH" ]] || { add_defect "work-tracker-project.sh not found at $PROJECT_SH; derived rows unavailable."; return 0; }

  local sf feature_dir proj epic_id epic_title epic_status mode kind owner action opened rel
  while IFS= read -r sf; do
    [[ -n "$sf" ]] || continue
    feature_dir="$(dirname "$sf")"
    proj="$(bash "$PROJECT_SH" --feature-dir "$feature_dir" 2>/dev/null || true)"
    [[ -n "$proj" ]] || continue

    epic_id="$(printf '%s' "$proj" | jq -r '(.items[]? | select(.type=="epic") | .id) // empty' 2>/dev/null | head -1)"
    [[ -n "$epic_id" ]] || continue
    epic_title="$(printf '%s' "$proj" | jq -r '(.items[]? | select(.type=="epic") | .title) // empty' 2>/dev/null | head -1)"
    epic_status="$(printf '%s' "$proj" | jq -r '(.items[]? | select(.type=="epic") | .status) // empty' 2>/dev/null | head -1)"

    # The mode, the next owner, and the opened date are metadata the projection
    # deliberately does not carry (it is provider-neutral). Read them from the
    # same state.json the projection just consumed — this is presentation
    # metadata, not a second status reader; status still comes from the
    # projection above and is never recomputed here.
    mode="$(jq -r '(.workflowMode // .mode // "") | tostring' "$sf" 2>/dev/null || echo "")"
    owner="$(jq -r '(.nextRequiredOwner // .nextOwner // "") | tostring' "$sf" 2>/dev/null || echo "")"
    opened="$(jq -r '(.createdAt // .created // .startedAt // "") | tostring' "$sf" 2>/dev/null | cut -c1-10)"

    # Terminal-for-mode is not open work. Ask the script that owns the answer.
    if [[ -n "$epic_status" && -f "$TERMINAL_SH" ]]; then
      if bash "$TERMINAL_SH" "$epic_status" "${mode:-full-delivery}" >/dev/null 2>&1; then
        continue
      fi
    elif [[ "$epic_status" == "done" ]]; then
      continue
    fi

    case "$feature_dir" in
      */bugs/*) kind="bug" ;;
      *) kind="spec" ;;
    esac
    rel="${feature_dir#"$repo_root"/}"
    action="resume ${mode:-the declared workflow mode} at the recorded phase"
    add_row "$epic_id" "${epic_title:-$epic_id}" "$kind" "$rel" "${epic_status:-unknown}" \
      "${owner:-—}" "$action" "${opened:-—}" "$TODAY" "derived"
  done < <(find "$SPECS_DIR" -name 'state.json' -type f 2>/dev/null | sort)
}

# --- DERIVED: improvement rows, from the register that already owns them ------
# INDEX.md rows are `| <id or link> | <title> | <STATUS> | …`. Only a PROPOSED
# improvement is open work; APPLIED and WITHDRAWN are closed by definition.
derive_improvement_rows() {
  [[ -f "$IMP_INDEX" ]] || return 0
  local line id title status
  while IFS= read -r line; do
    case "$line" in
      \|*IMP-[0-9][0-9][0-9]*) ;;
      *) continue ;;
    esac
    id="$(printf '%s' "$line" | grep -oE 'IMP-[0-9]{3}' | head -1)"
    [[ -n "$id" ]] || continue
    status="$(printf '%s' "$line" | awk -F'|' '{print $4}')"
    status="$(trim "$status")"
    case "$status" in
      PROPOSED*) ;;
      *) continue ;;
    esac
    title="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $3}')")"
    add_row "$id" "${title:-$id}" "imp" "improvements/INDEX.md" "proposed" \
      "—" "approve, land, or withdraw the remaining scopes" "—" "$TODAY" "derived"
  done < "$IMP_INDEX"
}

# --- AUTHORED: residue rows, read from the committed register -----------------
read_register_rows() {
  if [[ ! -f "$REGISTER" ]]; then
    add_defect "register not found at ${REGISTER#"$repo_root"/} — run 'closeout' or create it from templates/open-work.md.tmpl so carried-over work has somewhere durable to live."
    return 0
  fi

  # The register has to travel with the repository. If it is ignored, every
  # carried-over item is invisible to the next clone, which is the exact
  # failure this file exists to prevent.
  if git -C "$repo_root" rev-parse --git-dir >/dev/null 2>&1; then
    if git -C "$repo_root" check-ignore -q "$REGISTER" 2>/dev/null; then
      add_defect "register ${REGISTER#"$repo_root"/} is git-ignored; a register that does not travel with the repository cannot carry work across sessions."
    fi
  fi

  local line id title kind ref state owner action opened lastseen seen_ids=""
  local header_cols="" row_cols=""
  while IFS= read -r line; do
    case "$line" in
      \|*) ;;
      *) continue ;;
    esac
    # Skip the header row and the separator row.
    case "$line" in
      *'---'*) continue ;;
    esac
    id="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $2}')")"
    [[ -n "$id" ]] || continue
    if [[ "$id" == "id" ]]; then
      # The header defines the shape every data row must match.
      header_cols="$(count_delimiters "$line")"
      continue
    fi

    # An unescaped `|` inside a cell silently splits that cell, so every value
    # after it shifts one column left and the row renders with the wrong text
    # under the wrong heading. Nothing else here would notice: the fields still
    # parse, they are just the wrong fields.
    if [[ -n "$header_cols" ]]; then
      row_cols="$(count_delimiters "$line")"
      if [[ "$row_cols" != "$header_cols" ]]; then
        add_defect "row '$id' has $row_cols column delimiters but the table header declares $header_cols; an unescaped '|' inside a cell splits that cell and shifts every column after it. Escape each literal pipe in the row as '\\|'."
      fi
    fi

    title="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $3}')")"
    kind="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $4}')")"
    ref="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $5}')")"
    state="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $6}')")"
    owner="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $7}')")"
    action="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $8}')")"
    opened="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $9}')")"
    lastseen="$(trim "$(printf '%s' "$line" | awk -F'|' '{print $10}')")"

    if [[ "$kind" != "residue" ]]; then
      add_defect "row '$id' declares kind='$kind'; only 'residue' rows may be authored. spec, bug, and imp rows are derived on every run, and authoring one creates a second source of truth for status."
      continue
    fi
    if is_placeholder "$owner"; then
      add_defect "residue row '$id' has no next-owner; an unowned row is not actionable and will not survive the next session."
    fi
    if is_placeholder "$action"; then
      add_defect "residue row '$id' has no next-action; 'finish the thing' is not a next action."
    fi
    case "$seen_ids" in
      *"|$id|"*) add_defect "residue row '$id' is declared more than once; ids must be unique so a row can be removed unambiguously when it closes." ;;
      *) seen_ids="${seen_ids}|$id|" ;;
    esac

    add_row "$id" "${title:-$id}" "residue" "${ref:-—}" "${state:-open}" \
      "${owner:-—}" "${action:-—}" "${opened:-—}" "${lastseen:-$TODAY}" "authored"
  done < "$REGISTER"
}

derive_artifact_rows
derive_improvement_rows
read_register_rows

row_count=0
[[ -n "$ROWS" ]] && row_count="$(printf '%s' "$ROWS" | grep -c '' || true)"
defect_count=0
[[ -n "$DEFECTS" ]] && defect_count="$(printf '%s' "$DEFECTS" | grep -c '' || true)"

if [[ "$format" == "json" ]]; then
  if $have_jq; then
    # Build the whole document in ONE jq call. An earlier shape staged the rows
    # through a temp file inside the repo, which made a read-only report dirty
    # the very working tree it is reporting on.
    printf '%s' "$ROWS" | jq -R -s \
      --arg root "$repo_root" \
      --arg gen "$TODAY" \
      --arg reg "${REGISTER#"$repo_root"/}" \
      --arg defects "$DEFECTS" '
      {
        source: "bubbles",
        repoRoot: $root,
        generatedAt: $gen,
        register: $reg,
        items: (
          split("\n") | map(select(length > 0)) | map(split("\t")) |
          map({
            id: .[0], title: .[1], kind: .[2], ref: .[3], state: .[4],
            nextOwner: .[5], nextAction: .[6], opened: .[7], lastSeen: .[8],
            derived: (.[9] == "derived")
          })
        ),
        defects: ($defects | split("\n") | map(select(length > 0)))
      }'
  else
    printf '{"source":"bubbles","repoRoot":"%s","items":[],"defects":["jq is not installed; --format json requires it"]}\n' "$repo_root"
  fi
else
  if [[ "$row_count" -eq 0 && "$defect_count" -eq 0 ]] && $quiet_empty; then
    exit 0
  fi
  echo "Open work — $repo_root"
  echo "Register: ${REGISTER#"$repo_root"/} (committed; derived rows are recomputed on every run)"
  echo
  if [[ "$row_count" -eq 0 ]]; then
    echo "  (nothing open: no non-terminal spec or bug, no PROPOSED improvement, no authored residue)"
  else
    printf '  %-30s %-8s %-26s %-22s %s\n' "ID" "KIND" "STATE" "NEXT-OWNER" "NEXT-ACTION"
    printf '  %-30s %-8s %-26s %-22s %s\n' "------------------------------" "--------" "--------------------------" "----------------------" "-----------"
    while IFS=$'\t' read -r r_id r_title r_kind r_ref r_state r_owner r_action _r_opened _r_seen _r_src; do
      [[ -n "$r_id" ]] || continue
      printf '  %-30s %-8s %-26s %-22s %s\n' \
        "$(printf '%.30s' "$r_id")" "$r_kind" "$(printf '%.26s' "$r_state")" \
        "$(printf '%.22s' "$r_owner")" "$r_action"
      [[ -n "$r_title" ]] && printf '  %-30s %s\n' "" "↳ $r_title  [$r_ref]"
    done < <(printf '%s' "$ROWS")
  fi
  echo
  if [[ "$defect_count" -gt 0 ]]; then
    echo "  Register defects ($defect_count):"
    while IFS= read -r d; do
      [[ -n "$d" ]] || continue
      echo "    ✗ $d"
    done < <(printf '%s' "$DEFECTS")
    echo
  fi
fi

if $do_lint && [[ "$defect_count" -gt 0 ]]; then
  exit 1
fi
exit 0
