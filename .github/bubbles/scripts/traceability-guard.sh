#!/usr/bin/env bash
# Capability: dod-gherkin-fidelity-threshold
set -euo pipefail

script_source="${BASH_SOURCE[0]}"
script_source_hops=0
while [[ -L "$script_source" ]]; do
  script_source_hops=$((script_source_hops + 1))
  if [[ "$script_source_hops" -gt 40 ]]; then
    echo "ERROR: traceability guard script path exceeds the symlink limit" >&2
    exit 2
  fi
  if ! script_link="$(readlink "$script_source")"; then
    echo "ERROR: traceability guard script path cannot be resolved" >&2
    exit 2
  fi
  if [[ "$script_link" == /* ]]; then
    script_source="$script_link"
  else
    script_source="$(dirname "$script_source")/$script_link"
  fi
done
SCRIPT_DIR="$(cd -P "$(dirname "$script_source")" && pwd -P)"

# shellcheck source=/dev/null
source "$SCRIPT_DIR/dod-section-lib.sh"

# Shared scenario->DoD matcher (BUG-004): ONE implementation of the G068 rule,
# consumed here and by state-transition-guard.sh Check 22. This guard passes the
# `id-hint-lenient` id policy; see scenario-match-lib.sh for what that means.
# shellcheck source=/dev/null
source "$SCRIPT_DIR/scenario-match-lib.sh"

if [[ "${BASH_VERSINFO[0]}" -ge 4 ]]; then
  # shellcheck source=/dev/null
  source "$SCRIPT_DIR/fun-mode.sh"
else
  fun_fail() { :; }
  fun_warn() { :; }
  fun_banner() { :; }
fi

feature_dir=""
scope_mode="--all-scopes"
scope_mode_seen=0
coverage_policy="planning"
coverage_policy_seen=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all-scopes|--current-scope)
      if [[ "$scope_mode_seen" -eq 1 ]]; then
        echo "ERROR: scope selector may be declared exactly once" >&2
        exit 2
      fi
      scope_mode="$1"
      scope_mode_seen=1
      ;;
    --coverage-policy=planning|--coverage-policy=authored)
      if [[ "$coverage_policy_seen" -eq 1 ]]; then
        echo "ERROR: --coverage-policy may be declared exactly once" >&2
        exit 2
      fi
      coverage_policy="${1#*=}"
      coverage_policy_seen=1
      ;;
    --coverage-policy|--coverage-policy=*)
      echo "ERROR: --coverage-policy requires =planning or =authored" >&2
      exit 2
      ;;
    --*)
      echo "ERROR: unrecognized option: $1" >&2
      exit 2
      ;;
    *)
      if [[ -n "$feature_dir" ]]; then
        echo "ERROR: unexpected argument: $1" >&2
        exit 2
      fi
      feature_dir="$1"
      ;;
  esac
  shift
done

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/traceability-guard.sh specs/<NNN-feature-name> [--all-scopes|--current-scope] [--coverage-policy=planning|authored]"
  exit 2
fi

detect_repo_root() {
  if [[ "$SCRIPT_DIR" == */.github/bubbles/scripts ]]; then
    (cd -P "$SCRIPT_DIR/../../.." && pwd -P)
  else
    (cd -P "$SCRIPT_DIR/../.." && pwd -P)
  fi
}

canonical_path_is_within() {
  local candidate_path="$1"
  local canonical_root="$2"

  if [[ "$canonical_root" == "/" ]]; then
    [[ "$candidate_path" == /* ]]
  else
    [[ "$candidate_path" == "$canonical_root" || "$candidate_path" == "$canonical_root/"* ]]
  fi
}

if ! repo_root="$(detect_repo_root)"; then
  echo "ERROR: repository root cannot be canonicalized" >&2
  exit 2
fi

if [[ "$feature_dir" != /* ]]; then
  if [[ -d "$PWD/$feature_dir" ]]; then
    if ! repo_root="$(cd -P "$PWD" && pwd -P)"; then
      echo "ERROR: caller repository root cannot be canonicalized" >&2
      exit 2
    fi
  fi
  feature_dir="$repo_root/$feature_dir"
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

if ! feature_dir="$(cd -P "$feature_dir" && pwd -P)"; then
  echo "ERROR: feature directory cannot be canonicalized" >&2
  exit 2
fi
if ! canonical_path_is_within "$feature_dir" "$repo_root"; then
  echo "ERROR: feature directory resolves outside the repository root" >&2
  exit 2
fi

failures=0
warnings=0
scenario_total=0
row_total=0
mapped_total=0
file_reference_total=0
edge_declared=0
edge_inferred=0
edge_ambiguous=0
report_reference_total=0
deferred_evidence_total=0
scenario_manifest_total=0
scenario_manifest_file="$feature_dir/scenario-manifest.json"

fail() {
  local message="$1"
  echo "❌ $message"
  fun_fail
  failures=$((failures + 1))
}

warn() {
  local message="$1"
  echo "⚠️  $message"
  fun_warn
  warnings=$((warnings + 1))
}

pass() {
  local message="$1"
  echo "✅ $message"
}

info() {
  local message="$1"
  echo "ℹ️  $message"
}

json_first_string() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -Eo '"'"$key"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
}

detect_scope_layout() {
  local state_layout=""
  state_layout="$(json_first_string "scopeLayout" "$feature_dir/state.json" || true)"
  if [[ "$state_layout" == "per-scope-directory" ]] || [[ -f "$feature_dir/scopes/_index.md" ]]; then
    echo "per-scope-directory"
  else
    echo "single-file"
  fi
}

scope_section_tmp_files=()

cleanup_tmp_artifacts() {
  if [[ ${#scope_section_tmp_files[@]} -gt 0 ]]; then
    rm -f "${scope_section_tmp_files[@]}"
  fi
}

trap cleanup_tmp_artifacts EXIT

build_scope_analysis_units() {
  local scope_path="$1"
  local current_tmp=""
  local current_label=""
  local line=""

  if [[ "$scope_layout" != "single-file" ]] || [[ "$(basename "$scope_path")" != "scopes.md" ]]; then
    scope_analysis_files+=("$scope_path")
    scope_analysis_labels+=("${scope_path#$feature_dir/}")
    return
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^##[[:space:]]+Scope[[:space:]]+[0-9]+: ]]; then
      if [[ -n "$current_tmp" ]]; then
        scope_analysis_files+=("$current_tmp")
        scope_analysis_labels+=("$current_label")
      fi

      current_tmp="$(mktemp)"
      scope_section_tmp_files+=("$current_tmp")
      current_label="$(printf '%s' "$line" | sed -E 's/^##[[:space:]]+//')"
      printf '%s\n' "$line" > "$current_tmp"
      continue
    fi

    if [[ -n "$current_tmp" ]]; then
      if [[ "$line" =~ ^##[[:space:]]+Shared[[:space:]]+Planning[[:space:]]+Expectations ]]; then
        scope_analysis_files+=("$current_tmp")
        scope_analysis_labels+=("$current_label")
        current_tmp=""
        current_label=""
        continue
      fi

      printf '%s\n' "$line" >> "$current_tmp"
    fi
  done < "$scope_path"

  if [[ -n "$current_tmp" ]]; then
    scope_analysis_files+=("$current_tmp")
    scope_analysis_labels+=("$current_label")
  fi
}

scope_analysis_label() {
  local index="$1"
  if [[ "$index" -lt ${#scope_analysis_labels[@]} ]]; then
    printf '%s\n' "${scope_analysis_labels[$index]}"
  else
    printf '%s\n' "${scope_analysis_files[$index]#$feature_dir/}"
  fi
}

extract_test_rows() {
  local scope_path="$1"
  [[ -f "$scope_path" && -r "$scope_path" ]] || return 2
  python3 - "$scope_path" <<'PY'
import re, sys

lines = open(sys.argv[1], encoding="utf-8", errors="replace").read().splitlines()
visible = []
comment = False
fence = None
for raw in lines:
    line, pos = "", 0
    while pos < len(raw):
        if comment:
            end = raw.find("-->", pos)
            if end < 0: break
            comment, pos = False, end + 3
        else:
            start = raw.find("<!--", pos)
            if start < 0: line += raw[pos:]; break
            line += raw[pos:start]; comment, pos = True, start + 4
    marker = re.match(r"^\s*(`{3,}|~{3,})", line)
    if fence:
        if marker and marker.group(1)[0] == fence: fence = None
        continue
    if marker:
        fence = marker.group(1)[0]
        continue
    visible.append(line.rstrip())

sections = []
for index, line in enumerate(visible):
    match = re.match(r"^(#{2,3}) Test Plan$", line)
    if match:
        sections.append((index + 1, len(match.group(1))))
if not sections: raise SystemExit(3)
if len(sections) != 1: raise SystemExit(5)
start, depth = sections[0]

def cells(line):
    text = line.strip()
    if not text.startswith("|"): return None
    out, cell, index, code_ticks = [], [], 1, 0
    while index < len(text):
        ch = text[index]
        if ch == "\\" and index + 1 < len(text):
            cell.extend((ch, text[index + 1])); index += 2; continue
        if ch == "`":
            end = index
            while end < len(text) and text[end] == "`": end += 1
            run = end - index
            if code_ticks == 0: code_ticks = run
            elif run == code_ticks: code_ticks = 0
            cell.extend(text[index:end]); index = end; continue
        if ch == "|" and code_ticks == 0:
            out.append("".join(cell).strip()); cell = []; index += 1; continue
        cell.append(ch); index += 1
    if cell: out.append("".join(cell).strip())
    return out

header = None
path_header = None
table_kind = None
state = "SEEK_HEADER"
row_count = 0
path_headers = ("file/location", "file/surface", "persistentfileandexacttitle")

def fail_structure(message, line_number):
    print(f"ERROR: {message} at visible line {line_number}", file=sys.stderr)
    raise SystemExit(4)

def is_separator(row):
    return bool(row) and all(
        bool(value) and re.fullmatch(r":?-{3,}:?", value.replace(" ", ""))
        for value in row
    )

section = []
section_end_line = len(visible) + 1
for line_number, line in enumerate(visible[start:], start=start + 1):
    heading = re.match(r"^(#{1,6})(?:\s|$)", line)
    if heading and len(heading.group(1)) <= depth: section_end_line = line_number; break
    section.append((line_number, line, heading))

index = 0
while index < len(section):
    line_number, line, heading = section[index]
    row = cells(line)

    if state == "DONE":
        next_row = cells(section[index + 1][1]) if index + 1 < len(section) else None
        if row is not None and next_row is not None and len(next_row) == len(row) and is_separator(next_row):
            fail_structure("second Markdown table in Test Plan section", line_number)
        index += 1
        continue

    if state == "SEEK_HEADER":
        if row is None:
            index += 1
            continue
        normalized = [re.sub(r"\s+", "", value).lower() for value in row]
        matched_paths = [value for value in path_headers if value in normalized]
        reserved_candidate = any(value in normalized for value in ("id", "type", "testtype") + path_headers)
        if not reserved_candidate:
            index += 1
            continue
        if normalized.count("testtype") == 1 and len(matched_paths) == 1:
            if (normalized.count("id") != 0 or normalized.count("type") != 0
              or any(normalized.count(value) != 1 for value in matched_paths)): fail_structure("malformed Test Plan header", line_number)
            table_kind = "legacy"
        elif normalized.count("id") == 1 and normalized.count("type") == 1 and len(matched_paths) == 1:
            if (normalized.count("testtype") != 0
              or any(normalized.count(value) != 1 for value in matched_paths)): fail_structure("malformed Test Plan header", line_number)
            table_kind = "canonical"
        else: fail_structure("malformed Test Plan header", line_number)
        header = normalized
        path_header = matched_paths[0]
        state = "EXPECT_SEPARATOR"
        index += 1
        continue

    if state == "EXPECT_SEPARATOR":
        if row is None:
            fail_structure("missing or delayed Test Plan separator", line_number)
        if len(row) != len(header):
            fail_structure(
                f"wrong-width Test Plan separator: expected {len(header)} cells, got {len(row)}",
                line_number,
            )
        if not is_separator(row):
            fail_structure("invalid or empty Test Plan separator cell", line_number)
        state = "READ_ROWS"
        index += 1
        continue

    if row is None or heading:
        if row_count == 0:
            fail_structure("rowless recognized Test Plan table", line_number)
        state = "DONE"
        index += 1
        continue

    next_row = cells(section[index + 1][1]) if index + 1 < len(section) else None
    if next_row is not None and len(next_row) == len(row) and is_separator(next_row):
        fail_structure("second Markdown table in Test Plan section", line_number)
    if is_separator(row):
        fail_structure("unexpected separator inside Test Plan data rows", line_number)
    if len(row) != len(header):
        fail_structure(
            f"malformed Test Plan row: expected {len(header)} cells, got {len(row)}",
            line_number,
        )
    required_headers = ("testtype", path_header) if table_kind == "legacy" else ("id", "type", path_header)
    if any(not row[header.index(required)] for required in required_headers):
        fail_structure("required Test Plan cell is empty", line_number)
    file_value = row[header.index(path_header)]
    semantic = [value for position, value in enumerate(row)
                if header[position] not in path_headers + ("command",) and value]
    print(file_value + "\t" + " | ".join(semantic))
    row_count += 1
    index += 1

if state == "EXPECT_SEPARATOR":
    fail_structure("missing or delayed Test Plan separator", section_end_line)
if state == "READ_ROWS" and row_count == 0:
    fail_structure("rowless recognized Test Plan table", section_end_line)
PY
}

test_row_file_location() {
  local row_record="$1"
  printf '%s\n' "${row_record%%$'\t'*}"
}

test_row_raw() {
  local row_record="$1"
  if [[ "$row_record" == *$'\t'* ]]; then
    printf '%s\n' "${row_record#*$'\t'}"
  else
    printf '%s\n' "$row_record"
  fi
}

test_row_has_path() {
  local row_record="$1"
  [[ -n "$(extract_path_candidates "$(test_row_file_location "$row_record")")" ]]
}

extract_dod_items() {
  local scope_path="$1"
  # BUG-026: route through the shared DoD section parser so the tiered-DoD
  # boundary is correct (nested tier subheadings are retained through depth 6
  # and the section ends only at a same-or-shallower heading) and identical to
  # state-transition Check 4A/22. Emits the checkbox item text after the marker,
  # one per line — the same shape the previous inline awk produced, without the
  # depth-4-tier false boundary that made valid DoDs look rowless (BUG026-F002).
  dod_section_parse "$scope_path" | awk -F'\t' '
    $1 == "CHECKBOX" { out = $4; for (i = 5; i <= NF; i++) out = out "\t" $i; print out }
  '
}

extract_scenarios() {
  local scope_path="$1"
  grep -E '^[[:space:]]*Scenario( Outline)?:' "$scope_path" | sed -E 's/^[[:space:]]*Scenario( Outline)?:[[:space:]]*//'
}

# Emits "TRACEID<TAB>TITLE" per scenario, where TRACEID is the SCN/AC/FR/UC id on
# the nearest preceding heading (empty when the heading carries none). The TITLE
# field is byte-identical to what extract_scenarios emits, deliberately: the id is
# carried BESIDE the title rather than prefixed onto it, so the word-overlap
# scorer sees exactly the same word set it saw before and no existing match can
# change outcome. Without this the id never reached the comparison at all —
# extract_scenarios strips everything before "Scenario:", while the id lives on
# the heading above — which is why the guard reported declared=0 on every packet.
extract_scenarios_with_ids() {
  local scope_path="$1"
  awk '
    /^[[:space:]]*#{1,6}[[:space:]]/ {
      heading_id = ""
      if (match($0, /(SCN|AC|FR|UC)-[A-Za-z0-9_-]+/)) {
        heading_id = substr($0, RSTART, RLENGTH)
      }
      next
    }
    /^[[:space:]]*Scenario( Outline)?:/ {
      title = $0
      sub(/^[[:space:]]*Scenario( Outline)?:[[:space:]]*/, "", title)
      scenario_id = heading_id
      if (scenario_id == "" && match(title, /(SCN|AC|FR|UC)-[A-Za-z0-9_-]+/)) {
        scenario_id = substr(title, RSTART, RLENGTH)
      }
      print scenario_id "\t" title
    }
  ' "$scope_path"
}

# An explicit shared trace id is stronger evidence of an intended mapping than any
# similarity score, so it ESTABLISHES a match instead of only grading one that
# word overlap already found. Previously this comparison existed only inside
# classify_match_kind, which runs after a match succeeds — so a DoD item naming
# its scenario outright was still reported as having no faithful DoD item.
# Returns 1 on an empty id so a heading without an id can never blanket-match.
trace_id_declared() {
  local scenario_id="$1"
  local target="$2"
  local tid

  [[ -n "$scenario_id" ]] || return 1

  while IFS= read -r tid; do
    [[ -n "$tid" ]] || continue
    if [[ "$tid" == "$scenario_id" ]]; then
      return 0
    fi
  done < <(bubbles_scenario_extract_trace_ids "$target")

  return 1
}

# classify_match_kind — IMP-015 Scope B (informational only).
# Re-derives the confidence of an already-confirmed scenario→target match
# READ-ONLY: 'declared' iff the scenario's first trace ID also appears in the
# target; otherwise 'inferred'. Never touches failures/warnings/exit.
classify_match_kind() {
  local scenario="$1"
  local target="$2"
  local explicit_id="${3:-}"
  local sid tid
  # The caller may pass the id explicitly because the scenario title never
  # contains one. Falling back to extraction keeps older callers working.
  sid="$explicit_id"
  [[ -n "$sid" ]] || sid="$(bubbles_scenario_extract_trace_ids "$scenario" | head -n 1 || true)"
  if [[ -n "$sid" ]]; then
    while IFS= read -r tid; do
      [[ -n "$tid" ]] || continue
      if [[ "$tid" == "$sid" ]]; then
        printf 'declared\n'
        return 0
      fi
    done < <(bubbles_scenario_extract_trace_ids "$target")
  fi
  printf 'inferred\n'
}

extract_path_candidates() {
  local value="$1"
  printf '%s\n' "$value" | grep -Eo '([A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+' || true
}

resolve_test_path() {
  local candidate="$1"
  local scope_dir="$2"
  python3 - "$repo_root" "$scope_dir" "$candidate" <<'PY'
import os, stat, sys
root, scope, value = sys.argv[1:]
if "\\" in value or value.startswith(("/", "//")) or any(part in ("", ".", "..") for part in value.split("/")):
    raise SystemExit(1)
root = os.path.realpath(root)
for base in (root, os.path.realpath(scope)):
    lexical = os.path.join(base, *value.split("/"))
    canonical = os.path.realpath(lexical)
    try:
        if os.path.commonpath((root, canonical)) != root: continue
        metadata = os.stat(canonical)
    except (OSError, ValueError): continue
    if stat.S_ISREG(metadata.st_mode):
        relative = os.path.relpath(canonical, root).replace(os.sep, "/")
        print("%s\t%s\t%s\t%s\t%s\t%s\t%s" % (
          canonical, relative, metadata.st_mode & 0o7777, metadata.st_dev, metadata.st_ino,
          metadata.st_size, metadata.st_mtime_ns))
        raise SystemExit(0)
raise SystemExit(1)
PY
}

revalidate_test_path() {
  local resolved="$1"
  python3 - "$repo_root" "$resolved" <<'PY'
import os, stat, sys
root, record = sys.argv[1:]
path, relative, mode, device, inode, size, modified_ns = record.split("\t")
try:
    metadata = os.stat(path)
    valid = (os.path.commonpath((os.path.realpath(root), os.path.realpath(path))) == os.path.realpath(root)
       and stat.S_ISREG(metadata.st_mode)
    and (metadata.st_mode & 0o7777, metadata.st_dev, metadata.st_ino,
      metadata.st_size, metadata.st_mtime_ns)
    == (int(mode), int(device), int(inode), int(size), int(modified_ns)))
except (OSError, ValueError): valid = False
raise SystemExit(0 if valid else 1)
PY
}

revalidate_manifest_test_path() {
  local record="$1"
  python3 - "$repo_root" "$record" <<'PY'
import os, stat, sys
root, record = sys.argv[1:]
path, mode, device, inode, size, modified_ns = record.split("\t")
try:
    metadata = os.stat(path)
    valid = (os.path.commonpath((os.path.realpath(root), os.path.realpath(path))) == os.path.realpath(root)
    and stat.S_ISREG(metadata.st_mode)
    and (metadata.st_mode & 0o7777, metadata.st_dev, metadata.st_ino,
      metadata.st_size, metadata.st_mtime_ns)
    == (int(mode), int(device), int(inode), int(size), int(modified_ns)))
except (OSError, ValueError): valid = False
raise SystemExit(0 if valid else 1)
PY
}

normalize_absolute_path_lexically() {
  local input_path="$1"
  local component
  local last_index
  local normalized=""
  local -a input_components=()
  local -a normalized_components=()

  IFS='/' read -r -a input_components <<< "$input_path"
  if [[ ${#input_components[@]} -gt 0 ]]; then
    for component in "${input_components[@]}"; do
      case "$component" in
        ""|.) continue ;;
        ..)
          if [[ ${#normalized_components[@]} -gt 0 ]]; then
            last_index=$((${#normalized_components[@]} - 1))
            unset "normalized_components[$last_index]"
          fi
          ;;
        *) normalized_components+=("$component") ;;
      esac
    done
  fi

  if [[ ${#normalized_components[@]} -gt 0 ]]; then
    for component in "${normalized_components[@]}"; do
      normalized="$normalized/$component"
    done
  fi
  normalized_absolute_path="${normalized:-/}"
}

resolve_linked_test_candidate_once() {
  local canonical_base="$1"
  local relative_path="$2"
  local resolved_path="$canonical_base"
  local component
  local candidate_path
  local link_target
  local target_relative
  local symlink_hops=0
  local -a pending_components=()
  local -a link_components=()

  resolved_candidate_path=""
  resolved_candidate_status="missing-target"
  IFS='/' read -r -a pending_components <<< "$relative_path"

  while [[ ${#pending_components[@]} -gt 0 ]]; do
    component="${pending_components[0]}"
    if [[ ${#pending_components[@]} -gt 1 ]]; then
      pending_components=("${pending_components[@]:1}")
    else
      pending_components=()
    fi

    case "$component" in
      ""|.) continue ;;
      ..)
        resolved_path="$(dirname "$resolved_path")"
        if ! canonical_path_is_within "$resolved_path" "$repo_root"; then
          resolved_candidate_status="outside-repository"
          return 1
        fi
        continue
        ;;
    esac

    candidate_path="$resolved_path/$component"
    if [[ -L "$candidate_path" ]]; then
      symlink_hops=$((symlink_hops + 1))
      if [[ "$symlink_hops" -gt 40 ]]; then
        resolved_candidate_status="unstable-target"
        return 1
      fi
      if ! link_target="$(readlink "$candidate_path")"; then
        resolved_candidate_status="unstable-target"
        return 1
      fi
      link_components=()

      if [[ "$link_target" == /* ]]; then
        normalize_absolute_path_lexically "$link_target"
        if ! canonical_path_is_within "$normalized_absolute_path" "$repo_root"; then
          resolved_candidate_status="outside-repository"
          return 1
        fi
        resolved_path="$repo_root"
        if [[ "$normalized_absolute_path" == "$repo_root" ]]; then
          link_components=()
        else
          target_relative="${normalized_absolute_path#"$repo_root"/}"
          IFS='/' read -r -a link_components <<< "$target_relative"
        fi
      else
        IFS='/' read -r -a link_components <<< "$link_target"
      fi
      if [[ ${#link_components[@]} -gt 0 ]]; then
        if [[ ${#pending_components[@]} -gt 0 ]]; then
          pending_components=("${link_components[@]}" "${pending_components[@]}")
        else
          pending_components=("${link_components[@]}")
        fi
      fi
      continue
    fi

    if [[ ${#pending_components[@]} -gt 0 ]]; then
      if [[ -d "$candidate_path" ]]; then
        if ! resolved_path="$(cd -P "$candidate_path" && pwd -P)"; then
          resolved_candidate_status="unstable-target"
          return 1
        fi
        if ! canonical_path_is_within "$resolved_path" "$repo_root"; then
          resolved_candidate_status="outside-repository"
          return 1
        fi
      elif [[ -e "$candidate_path" ]]; then
        resolved_candidate_status="non-regular-target"
        return 1
      else
        resolved_candidate_status="missing-target"
        return 1
      fi
      continue
    fi

    if ! canonical_path_is_within "$candidate_path" "$repo_root"; then
      resolved_candidate_status="outside-repository"
      return 1
    fi
    if [[ -f "$candidate_path" ]]; then
      resolved_candidate_path="$candidate_path"
      resolved_candidate_status="ok"
      return 0
    fi
    if [[ -e "$candidate_path" ]]; then
      resolved_candidate_status="non-regular-target"
    else
      resolved_candidate_status="missing-target"
    fi
    return 1
  done

  resolved_candidate_status="non-regular-target"
  return 1
}

resolve_linked_test_path() {
  local relative_path="$1"
  local scope_dir="$2"
  local base
  local canonical_base
  local first_path
  local first_status
  local second_path
  local second_status
  local duplicate
  local first_seen_base=""
  local saw_unstable=0
  local saw_outside=0
  local saw_nonregular=0
  local -a candidate_bases=("$repo_root" "$scope_dir")

  linked_test_resolution_status="missing-target"
  linked_test_resolution_base=""

  for base in "${candidate_bases[@]}"; do
    if ! canonical_base="$(cd -P "$base" && pwd -P)"; then
      continue
    fi
    if ! canonical_path_is_within "$canonical_base" "$repo_root"; then
      saw_outside=1
      continue
    fi

    duplicate=0
    if [[ -n "$first_seen_base" && "$canonical_base" == "$first_seen_base" ]]; then
      duplicate=1
    fi
    if [[ "$duplicate" -eq 1 ]]; then
      continue
    fi
    if [[ -z "$first_seen_base" ]]; then
      first_seen_base="$canonical_base"
    fi

    if resolve_linked_test_candidate_once "$canonical_base" "$relative_path"; then
      first_path="$resolved_candidate_path"
      first_status="$resolved_candidate_status"
    else
      first_path="$resolved_candidate_path"
      first_status="$resolved_candidate_status"
    fi

    if [[ "$first_status" == "ok" ]]; then
      if resolve_linked_test_candidate_once "$canonical_base" "$relative_path"; then
        second_path="$resolved_candidate_path"
        second_status="$resolved_candidate_status"
      else
        second_path="$resolved_candidate_path"
        second_status="$resolved_candidate_status"
      fi
      if [[ "$second_status" == "ok" && "$second_path" == "$first_path" && -f "$second_path" ]]; then
        linked_test_resolution_status="ok"
        if [[ "$canonical_base" == "$repo_root" ]]; then
          linked_test_resolution_base="repository"
        else
          linked_test_resolution_base="feature"
        fi
        return 0
      fi
      saw_unstable=1
      continue
    fi

    case "$first_status" in
      unstable-target) saw_unstable=1 ;;
      outside-repository) saw_outside=1 ;;
      non-regular-target) saw_nonregular=1 ;;
    esac
  done

  if [[ "$saw_unstable" -eq 1 ]]; then
    linked_test_resolution_status="unstable-target"
  elif [[ "$saw_outside" -eq 1 ]]; then
    linked_test_resolution_status="outside-repository"
  elif [[ "$saw_nonregular" -eq 1 ]]; then
    linked_test_resolution_status="non-regular-target"
  else
    linked_test_resolution_status="missing-target"
  fi
  return 1
}

classify_linked_test_record() {
  local record="$1"

  jq -r '
    if ((.ordinal | type) != "number") or ((.path | type) != "string") then
      error("invalid linked-test projection record")
    else
      .path as $path
      | if ($path | explode | any(. < 32 or . == 127)) then "control-character"
        elif ($path | test("^[[:space:]]*$")) then "empty-reference"
        elif ($path | startswith("/"))
          or ($path | startswith("\\\\"))
          or ($path | test("^[A-Za-z]:")) then "absolute-reference"
        elif ($path | split("/") | any(. == "..")) then "parent-traversal"
        else "lexically-safe-relative"
        end
    end
  ' <<< "$record" 2>/dev/null
}

linked_test_reference_is_acceptable() {
  local record="$1"
  local scope_dir="$2"
  local lexical_status
  local candidate_path

  linked_test_reference_ordinal=""
  linked_test_reference_path=""
  linked_test_reference_status="projection-error"

  if ! linked_test_reference_ordinal="$(jq -r '.ordinal | select(type == "number")' <<< "$record" 2>/dev/null)" \
    || [[ -z "$linked_test_reference_ordinal" ]]; then
    return 1
  fi
  if ! lexical_status="$(classify_linked_test_record "$record")" || [[ -z "$lexical_status" ]]; then
    return 1
  fi
  if [[ "$lexical_status" != "lexically-safe-relative" ]]; then
    linked_test_reference_status="$lexical_status"
    return 1
  fi
  if ! candidate_path="$(jq -er '.path | select(type == "string")' <<< "$record" 2>/dev/null)"; then
    return 1
  fi

  linked_test_reference_path="$candidate_path"
  if resolve_linked_test_path "$candidate_path" "$scope_dir"; then
    linked_test_reference_status="ok"
    return 0
  fi
  linked_test_reference_status="$linked_test_resolution_status"
  return 1
}

concrete_test_path_is_acceptable() {
  local candidate_path="$1"
  local scope_dir="$2"
  local record

  if ! record="$(jq -cn --arg path "$candidate_path" \
    '{ordinal: 0, field: "testPlan", form: "path", path: $path}')"; then
    return 1
  fi
  linked_test_reference_is_acceptable "$record" "$scope_dir"
}

legacy_manifest_projection() {
  jq -c '
    def scenarios:
      if type == "array" then .
      elif type == "object" and (.scenarios | type == "array") then .scenarios
      else error("expected object.scenarios[] or legacy top-level array")
      end;
    def trimmed:
      gsub("^[[:space:]]+|[[:space:]]+$"; "");
    def authored_reference($field; $index; $reference):
      if ($reference | type) == "string" then
        {field: $field, index: $index, kind: "authored", path: (($reference | split("#") | .[0] // "") | trimmed), identity: null}
      elif (($reference | type) == "object") and (($reference.file? | type) == "string") then
        {field: $field, index: $index, kind: "authored", path: ($reference.file | trimmed), identity: null}
      elif (($reference | type) == "object") and ($reference.file? == null)
        and (($reference.path? | type) == "string") then
        {field: $field, index: $index, kind: "authored", path: ($reference.path | trimmed), identity: null}
      else error("linked-test reference must be a string or object with file/path")
      end;
    def planned_reference($index; $reference):
      if (($reference | type) == "object") and (($reference.path? | type) == "string") then
        {field: "plannedTests", index: $index, kind: "planned", path: ($reference.path | trimmed), identity: null}
      else error("planned-test reference must be an object with path")
      end;
    {
      scenarios: [
        scenarios[] as $scenario
        | (($scenario.id // $scenario.scenarioId) // error("scenario has no id or scenarioId")) as $scenario_id
        | {
            scenarioId: $scenario_id,
            scopeRef: ($scenario.scopeRef // $scenario.scope // $scenario.scopeId // null),
            title: ($scenario.title // null),
            requiredTestType: ($scenario.requiredTestType // null),
            evidenceRefs: ($scenario.evidenceRefs // []),
            implementationRefs: ($scenario.implementationRefs // []),
            invariantRefs: ($scenario.invariantRefs // []),
            obligations: ($scenario.obligations // null),
            testMechanism: ($scenario.testMechanism // null),
            references: (
              [
                ["linkedTests", "linkedTestContracts"][] as $field
                | (($scenario[$field] // []) | to_entries[]) as $entry
                | authored_reference($field; $entry.key; $entry.value)
              ] + [
                (($scenario.plannedTests // []) | to_entries[]) as $entry
                | planned_reference($entry.key; $entry.value)
              ]
            )
          }
      ]
    }
  ' "$scenario_manifest_file"
}

legacy_projection_uses_feature_root() {
  local projection_file="$1"
  local reference=""
  local ordinal=0
  local saw_feature_root=0

  while IFS= read -r reference; do
    [[ -n "$reference" ]] || continue
    ordinal=$((ordinal + 1))
    reference="$(jq -c --argjson ordinal "$ordinal" '. + {ordinal: $ordinal}' <<< "$reference")"
    if linked_test_reference_is_acceptable "$reference" "$feature_dir" \
      && [[ "$linked_test_resolution_base" == "feature" ]]; then
      saw_feature_root=1
      break
    fi
  done < <(jq -c '.scenarios[].references[] | select(.kind == "authored")' "$projection_file")

  [[ "$saw_feature_root" -eq 1 ]]
}

report_mentions_path() {
  local report_path="$1"
  local candidate="$2"

  if [[ ! -f "$report_path" ]]; then
    return 1
  fi

  grep -Fq "$candidate" "$report_path"
}

scenario_matches_row() {
  local scenario="$1"
  local row="$2"
  local scenario_id
  local row_id
  local words
  local word
  local row_norm
  local score=0
  local threshold=3
  local word_count=0

  scenario_id="$(bubbles_scenario_extract_trace_ids "$scenario" | head -n 1 || true)"
  if [[ -n "$scenario_id" ]]; then
    while IFS= read -r row_id; do
      if [[ -n "$row_id" ]] && [[ "$row_id" == "$scenario_id" ]]; then
        return 0
      fi
    done < <(bubbles_scenario_extract_trace_ids "$row")
  fi

  row_norm="$(bubbles_scenario_normalize_text "$row")"
  words="$(bubbles_scenario_significant_words "$scenario")"
  if [[ -z "$words" ]]; then
    [[ "$row_norm" == *"$(bubbles_scenario_normalize_text "$scenario")"* ]]
    return
  fi

  while IFS= read -r word; do
    [[ -n "$word" ]] || continue
    word_count=$((word_count + 1))
    if [[ " $row_norm " == *" $word "* ]]; then
      score=$((score + 1))
    fi
  done <<< "$words"

  [[ "$word_count" -ge 3 && "$score" -ge "$threshold" ]]
}

scope_layout="$(detect_scope_layout)"
scope_files=()
scope_analysis_files=()
scope_analysis_labels=()

# BUG-026 C2: in --current-scope mode, resolve the immutable applicable universe
# from state.json (fail-closed) and keep only the applicable scope directories.
# A not_started descendant of the current scope is omitted; the current scope,
# its transitive prerequisites, and applicable siblings are retained.
applicable_scope_dirs=""
scope_resolver_file=""
if [[ "$scope_mode" == "--current-scope" ]]; then
  if [[ "$scope_layout" != "per-scope-directory" ]]; then
    echo "ERROR: --current-scope requires per-scope-directory layout (single-file scopes.md carries no scopeDir bijection)" >&2
    exit 2
  fi
  scope_resolver="$SCRIPT_DIR/scope-universe-resolver.py"
  if [[ ! -f "$scope_resolver" ]]; then
    echo "ERROR: scope-universe-resolver.py not found (required for --current-scope): $scope_resolver" >&2
    exit 2
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo "ERROR: python3 not found (required for --current-scope)" >&2
    exit 2
  fi
  if ! scope_resolver_out="$(python3 "$scope_resolver" "$feature_dir" current-scope 2>&1)"; then
    echo "ERROR: scope-universe resolution refused (--current-scope):" >&2
    printf '%s\n' "$scope_resolver_out" >&2
    exit 2
  fi
  scope_resolver_file="$(mktemp)"
  scope_section_tmp_files+=("$scope_resolver_file")
  printf '%s\n' "$scope_resolver_out" >"$scope_resolver_file"
  if ! applicable_scope_dirs="$(python3 - "$scope_resolver_file" "$feature_dir/scopes" <<'PY'
import json
import os
import sys

records = []
with open(sys.argv[1], encoding="utf-8") as handle:
  for line_number, line in enumerate(handle, 1):
    fields = line.rstrip("\n").split("\t")
    if len(fields) != 8 or fields[0] != "RECORD":
      sys.stderr.write("invalid scope-universe RECORD protocol at line %d\n" % line_number)
      raise SystemExit(2)
    try:
      aliases = json.loads(fields[7])
    except ValueError:
      sys.stderr.write("invalid scope-universe alias JSON at line %d\n" % line_number)
      raise SystemExit(2)
    if (not isinstance(aliases, list) or aliases != sorted(set(aliases))
        or any(not isinstance(alias, str) or not alias for alias in aliases)):
      sys.stderr.write("invalid scope-universe alias array at line %d\n" % line_number)
      raise SystemExit(2)
    if fields[1] not in aliases:
      sys.stderr.write("canonical scope identity absent from aliases at line %d\n" % line_number)
      raise SystemExit(2)
    records.append((fields, aliases))

physical = {}
for entry in os.listdir(sys.argv[2]):
  scope_file = os.path.join(sys.argv[2], entry, "scope.md")
  if os.path.isfile(scope_file):
    physical.setdefault(entry, []).append(scope_file)

selected = []
claimed = {}
for fields, _aliases in records:
  if fields[6]:
    scope_dir = fields[6].rstrip("/").rsplit("/", 1)[-1]
    matches = physical.get(scope_dir, [])
    if len(matches) != 1:
      sys.stderr.write("scope-universe record %s resolves to %d physical scopes\n" % (fields[1], len(matches)))
      raise SystemExit(2)
    claimed.setdefault(scope_dir, []).append(fields[1])
    if fields[5] == "true":
      selected.append(scope_dir)
for scope_dir, owners in claimed.items():
  if len(owners) != 1:
    sys.stderr.write("physical scope %s is claimed by %d scope-universe records\n" % (scope_dir, len(owners)))
    raise SystemExit(2)
for scope_dir in selected:
  print(scope_dir)
PY
)"; then
    echo "ERROR: --current-scope could not project resolver records onto unique physical scopes" >&2
    exit 2
  fi
  if [[ -z "$applicable_scope_dirs" ]]; then
    echo "ERROR: --current-scope resolved an empty applicable universe (no scopeDir on any applicable state record)" >&2
    exit 2
  fi
fi

if [[ "$scope_layout" == "per-scope-directory" ]]; then
  while IFS= read -r scope_path; do
    if [[ "$scope_mode" == "--current-scope" ]]; then
      scope_local_dir="$(basename "$(dirname "$scope_path")")"
      if ! printf '%s\n' "$applicable_scope_dirs" | grep -qxF "$scope_local_dir"; then
        continue
      fi
    fi
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | LC_ALL=C sort)
else
  scope_files+=("$feature_dir/scopes.md")
fi

if [[ "$scope_mode" == "--current-scope" && ${#scope_files[@]} -eq 0 ]]; then
  echo "ERROR: --current-scope matched no physical scope directory (state scopeDir vs disk name mismatch)" >&2
  exit 2
fi

for scope_path in "${scope_files[@]}"; do
  build_scope_analysis_units "$scope_path"
done

if [[ ${#scope_analysis_files[@]} -eq 0 ]]; then
  scope_analysis_files=("${scope_files[@]}")
  for scope_path in "${scope_files[@]}"; do
    scope_analysis_labels+=("${scope_path#$feature_dir/}")
  done
fi

# Project manifest-linked test files onto the same immutable scope universe as
# the scope files above. Without this projection, --current-scope omitted a Not
# Started descendant's scope.md but still failed on that descendant's planned
# test file from scenario-manifest.json. Sequential execution then deadlocked:
# the current scope could not become Done until a later scope authored its test,
# while the later scope could not start until the current scope was Done.
#
# Default --all-scopes selection remains strict. The structured
# projection exists only in --current-scope, which already requires Python for
# the fail-closed scope-universe resolver. Every scenario must resolve to one
# physical scope directory; an unknown/ambiguous scope reference is a refusal,
# never a silently skipped manifest row.
manifest_linked_test_projection() {
  local reference_file=""
  local applicable_scope_file=""
  local fallback_file=""
  local manifest_shape=""
  local reader_error_file=""
  local reference_reader="$SCRIPT_DIR/scenario-reference-reader.py"
  reference_file="$(mktemp)"
  applicable_scope_file="$(mktemp)"
  fallback_file="$(mktemp)"
  reader_error_file="$(mktemp)"
  scope_section_tmp_files+=("$reference_file" "$applicable_scope_file" "$fallback_file" "$reader_error_file")

  if ! manifest_shape="$(jq -er '
    if type == "array" then "legacy-array"
    elif type == "object" and (.scenarios | type == "array") and (has("schemaVersion") | not) then "legacy-object"
    elif type == "object" and .schemaVersion == 1 then "schema-1"
    elif type == "object" and .schemaVersion == 2 then "schema-2"
    else error("unsupported scenario manifest envelope")
    end
  ' "$scenario_manifest_file")"; then
    return 2
  fi

  if [[ "$manifest_shape" == "legacy-object" ]]; then
    if ! legacy_manifest_projection >"$reference_file"; then
      return 2
    fi
  else
    [[ -f "$reference_reader" ]] || {
      printf 'scenario-reference-reader.py is required\n' >&2
      return 2
    }
    if ! python3 "$reference_reader" "$scenario_manifest_file" --repo-root "$repo_root" \
      >"$reference_file" 2>"$reader_error_file"; then
      if [[ "$manifest_shape" != "schema-2" ]] \
        && grep -Fq 'authored path is not an existing stable regular file' "$reader_error_file" \
        && legacy_manifest_projection >"$fallback_file" \
        && legacy_projection_uses_feature_root "$fallback_file"; then
        cat "$fallback_file" >"$reference_file"
      else
        cat "$reader_error_file" >&2
        return 2
      fi
    fi
  fi
  if [[ "$scope_mode" != "--current-scope" ]]; then
    cat "$reference_file"
    return 0
  fi
  printf '%s\n' "$applicable_scope_dirs" >"$applicable_scope_file"
  python3 - "$scope_mode" "$reference_file" "$scope_resolver_file" "$applicable_scope_file" "$feature_dir/scopes" <<'PY'
import json
import os
import sys

try:
  with open(sys.argv[2], encoding="utf-8") as handle:
    document = json.load(handle)
except (OSError, ValueError) as exc:
  sys.stderr.write("cannot parse scenario reference projection (%s)\n" % exc.__class__.__name__)
  raise SystemExit(2)

with open(sys.argv[4], encoding="utf-8") as handle:
  applicable_dirs = set(line.strip() for line in handle if line.strip())
scope_mode = sys.argv[1]
if scope_mode == "--current-scope" and not applicable_dirs:
  sys.stderr.write("scope-universe projection is empty\n")
  raise SystemExit(2)

alias_to_dirs = {}
if scope_mode == "--current-scope":
  with open(sys.argv[3], encoding="utf-8") as handle:
    for line_number, line in enumerate(handle, 1):
      fields = line.rstrip("\n").split("\t")
      if len(fields) != 8 or fields[0] != "RECORD":
        sys.stderr.write("invalid scope-universe RECORD protocol at line %d\n" % line_number)
        raise SystemExit(2)
      try:
        aliases = json.loads(fields[7])
      except ValueError:
        sys.stderr.write("invalid scope-universe alias JSON at line %d\n" % line_number)
        raise SystemExit(2)
      if (not isinstance(aliases, list) or aliases != sorted(set(aliases))
          or any(not isinstance(alias, str) or not alias for alias in aliases)
          or fields[1] not in aliases):
        sys.stderr.write("invalid scope-universe alias array at line %d\n" % line_number)
        raise SystemExit(2)
      scope_dir = fields[6].rstrip("/").rsplit("/", 1)[-1] if fields[6] else ""
      physical = os.path.join(sys.argv[5], scope_dir, "scope.md")
      if not scope_dir or not os.path.isfile(physical):
        sys.stderr.write("scope-universe record %s has no unique physical scope\n" % fields[1])
        raise SystemExit(2)
      for alias in aliases:
        alias_to_dirs.setdefault(alias, set()).add(scope_dir)
  physical_claims = {}
  with open(sys.argv[3], encoding="utf-8") as handle:
    for line in handle:
      fields = line.rstrip("\n").split("\t")
      scope_dir = fields[6].rstrip("/").rsplit("/", 1)[-1]
      physical_claims.setdefault(scope_dir, []).append(fields[1])
  for scope_dir, owners in physical_claims.items():
    if len(owners) != 1:
      sys.stderr.write("physical scope %s is claimed by %d scope-universe records\n" % (scope_dir, len(owners)))
      raise SystemExit(2)

def resolve_scope(scenario, scenario_id):
  scope_ref = scenario.get("scopeRef")
  if not isinstance(scope_ref, str) or not scope_ref:
    sys.stderr.write("scenario %s scope reference is not a normalized string\n" % scenario_id)
    raise SystemExit(2)
  matches = alias_to_dirs.get(scope_ref, set())
  if len(matches) != 1:
    sys.stderr.write("scenario %s scope reference resolves to %d physical scopes\n" % (scenario_id, len(matches)))
    raise SystemExit(2)
  return next(iter(matches))

scenarios = document.get("scenarios")
if not isinstance(scenarios, list):
  sys.stderr.write("scenario reference projection has no scenarios array\n")
  raise SystemExit(2)

selected = []
for scenario in scenarios:
  scenario_id = scenario["scenarioId"]
  if scope_mode == "--current-scope":
    scope_dir = resolve_scope(scenario, scenario_id)
    if scope_dir not in applicable_dirs:
      continue
  selected.append(scenario)
json.dump({"scenarios": selected}, sys.stdout, ensure_ascii=False, separators=(",", ":"))
sys.stdout.write("\n")
PY
}

# Reconcile the projected manifest against the physical scenario universe using
# authoritative option B. Every identified scope scenario must match its stable
# ID with exact multiplicity. The remaining manifest records are not assigned an
# inferred identity: their cardinality must equal the unidentified legacy scope
# residual. Comparing after current-scope alias projection is essential because
# two aliases of one physical scope must not manufacture scenario coverage.
scenario_manifest_reconciliation() {
  local projection_file="$1"
  shift
  python3 - "$projection_file" "$@" <<'PY'
import json
import re
import sys
from collections import Counter

with open(sys.argv[1], encoding="utf-8") as handle:
  document = json.load(handle)

manifest_ids = [scenario["scenarioId"] for scenario in document["scenarios"]]
scope_ids = []
scope_count = 0
heading_id = ""
id_pattern = re.compile(r"(SCN|AC|FR|UC)-[A-Za-z0-9_-]+")
heading_pattern = re.compile(r"^\s*#{1,6}\s")
scenario_pattern = re.compile(r"^\s*Scenario(?: Outline)?:\s*(.*)$")

for path in sys.argv[2:]:
  with open(path, encoding="utf-8", errors="replace") as handle:
    for line in handle:
      if heading_pattern.match(line):
        match = id_pattern.search(line)
        heading_id = match.group(0) if match else ""
        continue
      match = scenario_pattern.match(line)
      if not match:
        continue
      scope_count += 1
      scenario_id = heading_id
      if not scenario_id:
        inline = id_pattern.search(match.group(1))
        scenario_id = inline.group(0) if inline else ""
      if scenario_id:
        scope_ids.append(scenario_id)

manifest_counts = Counter(manifest_ids)
scope_counts = Counter(scope_ids)
for scenario_id in sorted(identifier for identifier, count in scope_counts.items() if count != 1):
  print("DUPLICATE_SCOPE\t%s\t%d" % (scenario_id, scope_counts[scenario_id]))
for scenario_id in sorted(scope_counts):
  expected = scope_counts[scenario_id]
  actual = manifest_counts.get(scenario_id, 0)
  if actual != expected:
    finding = "DUPLICATE_MANIFEST" if actual > 1 else "STABLE_ID"
    print("%s\t%s\t%d\t%d" % (finding, scenario_id, expected, actual))

identified_manifest_count = sum(manifest_counts.get(identifier, 0) for identifier in scope_counts)
legacy_scope_residual = scope_count - len(scope_ids)
legacy_manifest_residual = len(manifest_ids) - identified_manifest_count
if legacy_manifest_residual != legacy_scope_residual:
  print("LEGACY_RESIDUAL\tlegacy-residual\t%d\t%d" % (
      legacy_scope_residual, legacy_manifest_residual))
PY
}

echo "============================================================"
echo "  BUBBLES TRACEABILITY GUARD"
echo "  Feature: $feature_dir"
echo "  Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "============================================================"
fun_banner
echo ""

echo "--- Scenario Manifest Cross-Check (G057/G059) ---"
scope_defined_scenarios=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  scope_defined_scenarios=$((scope_defined_scenarios + $(grep -cE '^[[:space:]]*Scenario( Outline)?:' "$scope_path" || true)))
done

if [[ "$scope_defined_scenarios" -gt 0 ]]; then
  if [[ ! -f "$scenario_manifest_file" ]]; then
    fail "Resolved scopes define $scope_defined_scenarios Gherkin scenarios but scenario-manifest.json is missing"
  else
    manifest_parseable=true
    manifest_projection_file="$(mktemp)"
    manifest_projection_error_file="$(mktemp)"
    scope_section_tmp_files+=("$manifest_projection_file" "$manifest_projection_error_file")
    if ! command -v jq >/dev/null 2>&1; then
      fail "scenario reference projection requires jq for structured validation"
      manifest_parseable=false
    elif ! manifest_linked_test_projection >"$manifest_projection_file" 2>"$manifest_projection_error_file"; then
      projection_error="$(cat "$manifest_projection_error_file")"
      fail "scenario-manifest.json linked-test scope projection failed: $projection_error"
      manifest_parseable=false
    fi

    if [[ "$manifest_parseable" == "true" ]]; then
      scenario_manifest_total="$(jq -r '.scenarios | length' "$manifest_projection_file")"
      reconciliation_file="$(mktemp)"
      scope_section_tmp_files+=("$reconciliation_file")
      if ! scenario_manifest_reconciliation "$manifest_projection_file" "${scope_files[@]}" >"$reconciliation_file"; then
        fail "scenario-manifest.json exact scenario reconciliation could not be evaluated"
        manifest_parseable=false
      fi
      reconciliation_findings=0
      while IFS=$'\t' read -r reconciliation_kind reconciliation_id reconciliation_expected reconciliation_actual; do
        [[ -n "$reconciliation_kind" ]] || continue
        reconciliation_findings=$((reconciliation_findings + 1))
        case "$reconciliation_kind" in
          DUPLICATE_MANIFEST)
            fail "identified-subset exact matching failed: duplicate known stable scenario id in scenario-manifest.json: $reconciliation_id expected=$reconciliation_expected actual=$reconciliation_actual"
            ;;
          DUPLICATE_SCOPE)
            fail "identified-subset exact matching failed: duplicate known stable scenario id in resolved scope scenarios: $reconciliation_id (scenarios=$reconciliation_expected)"
            ;;
          LEGACY_RESIDUAL)
            fail "legacy residual cardinality differs after identified-subset exact matching: expected=$reconciliation_expected actual=$reconciliation_actual"
            ;;
          STABLE_ID)
            fail "identified-subset exact matching failed for known stable scenario id: $reconciliation_id expected=$reconciliation_expected actual=$reconciliation_actual"
            ;;
          *)
            fail "scenario-manifest.json exact scenario reconciliation returned an unknown finding: $reconciliation_kind"
            ;;
        esac
      done <"$reconciliation_file"
      if [[ "$manifest_parseable" == "true" && "$reconciliation_findings" -eq 0 ]]; then
        pass "scenario-manifest.json covers $scenario_manifest_total scenario contract(s)"
      fi

      manifest_authored_links=0
      manifest_planned_links=0
      manifest_missing_files=0
      policy_gap_total=0
      if [[ "$coverage_policy" == "authored" ]]; then
        policy_gap_filter='.scenarios[] | select(any(.references[]?; .kind == "authored") | not) | .scenarioId'
      else
        policy_gap_filter='.scenarios[] | select((.references | length) == 0) | .scenarioId'
      fi
      while IFS= read -r policy_gap_scenario; do
        [[ -n "$policy_gap_scenario" ]] || continue
        policy_gap_total=$((policy_gap_total + 1))
        if [[ "$coverage_policy" == "authored" ]]; then
          fail "scenario-manifest.json scenario lacks authored coverage required by --coverage-policy=authored: $policy_gap_scenario"
        else
          fail "scenario-manifest.json scenario lacks authored or planned coverage required by --coverage-policy=planning: $policy_gap_scenario"
        fi
      done < <(jq -r "$policy_gap_filter" "$manifest_projection_file")

      manifest_test_ordinal=0
      while IFS= read -r manifest_test_record; do
        [[ -n "$manifest_test_record" ]] || continue
        manifest_test_ordinal=$((manifest_test_ordinal + 1))
        manifest_test_kind="$(jq -r '.kind // empty' <<< "$manifest_test_record")"
        if [[ "$manifest_test_kind" == "planned" ]]; then
          manifest_planned_links=$((manifest_planned_links + 1))
          continue
        fi
        [[ "$manifest_test_kind" == "authored" ]] || continue
        manifest_authored_links=$((manifest_authored_links + 1))
        manifest_test_record="$(jq -c --argjson ordinal "$manifest_test_ordinal" '. + {ordinal: $ordinal}' <<< "$manifest_test_record")"
        if ! linked_test_reference_is_acceptable "$manifest_test_record" "$feature_dir"; then
          case "$linked_test_reference_status" in
            control-character|empty-reference|absolute-reference|parent-traversal)
              fail "scenario-manifest.json rejects linked test reference #$linked_test_reference_ordinal: $linked_test_reference_status"
              ;;
            outside-repository|non-regular-target|unstable-target)
              fail "scenario-manifest.json rejects linked test file: $linked_test_reference_path ($linked_test_reference_status)"
              ;;
            missing-target)
              fail "scenario-manifest.json references missing linked test file: $linked_test_reference_path (missing-target)"
              ;;
            *)
              fail "scenario-manifest.json linked-test projection record is invalid"
              ;;
          esac
          manifest_missing_files=$((manifest_missing_files + 1))
          continue
        fi

        manifest_test_identity="$(jq -r '
          if (.identity | type) == "object" then
            [(.canonicalPath // ""), (.identity.mode // ""), (.identity.device // ""),
             (.identity.inode // ""), (.identity.size // ""),
             (.identity.modifiedNanoseconds // "")] | @tsv
          else empty
          end
        ' <<< "$manifest_test_record")"
        if [[ -n "$manifest_test_identity" ]] && ! revalidate_manifest_test_path "$manifest_test_identity"; then
          fail "scenario-manifest.json linked test identity changed before content-sensitive use: $linked_test_reference_path"
          manifest_missing_files=$((manifest_missing_files + 1))
          continue
        fi
        pass "scenario-manifest.json linked test exists: $linked_test_reference_path"
      done < <(jq -c '.scenarios[].references[]' "$manifest_projection_file")

      manifest_evidence_refs="$(jq -r '[.scenarios[] | select((.evidenceRefs | type) == "array" and (.evidenceRefs | length) > 0)] | length' "$manifest_projection_file")"
      if [[ "$manifest_evidence_refs" -eq "$scenario_manifest_total" ]]; then
        pass "scenario-manifest.json records evidenceRefs for all $scenario_manifest_total scenario contract(s)"
      else
        fail "scenario-manifest.json records evidenceRefs for only $manifest_evidence_refs of $scenario_manifest_total scenario contract(s)"
      fi

      if [[ "$manifest_authored_links" -gt 0 && "$manifest_missing_files" -eq 0 ]]; then
        pass "All $manifest_authored_links authored linked test reference(s) from scenario-manifest.json exist"
      elif [[ "$manifest_authored_links" -eq 0 && "$manifest_planned_links" -gt 0 ]]; then
        info "Authored linked-test resolution NOT_APPLICABLE: 0 authored reference(s); $manifest_planned_links planned test reference(s) remain unauthored"
      elif [[ "$manifest_authored_links" -eq 0 && "$manifest_planned_links" -eq 0 ]]; then
        info "Linked-test classification NOT_APPLICABLE: scenario-manifest.json has no authored or planned test references"
      fi
    fi

    # IMP-106 SCOPE-3 (DOM-LINEAGE) — advisory scenario→invariant lineage edges.
    # A scenario carrying invariantRefs:["INV-..."] DECLARES an edge to the
    # domain invariant(s) it exercises. This reuses the existing declared/
    # inferred/ambiguous edge-confidence vocabulary; an invariantRefs edge is
    # always 'declared' because the reference is explicit (there is no inference
    # or ambiguity path for an invariant reference, unlike scenario↔Test-Plan/DoD
    # prose matching). Advisory only (info) — it never fails and never changes the
    # exit code — and a strict no-op when no invariantRefs are present, so a repo
    # that has not opted into a domainModel: block is unaffected. Full per-scope
    # integration into the Test Plan / DoD edge loops is intentionally NOT
    # attempted here to avoid destabilizing that logic; see
    # improvements/IMP-106-product-domain-ontology-and-invariant-anchoring.md
    # (SCOPE-3) and bubbles/schemas/scenario-manifest.schema.json.
    invariant_edge_total=0
    if [[ "$manifest_parseable" == "true" ]]; then
      invariant_edge_total="$(jq -r '[.scenarios[].invariantRefs[]? | select(test("^INV-[A-Z0-9-]+$"))] | length' "$manifest_projection_file")"
    fi
    if [[ -n "$invariant_edge_total" && "$invariant_edge_total" -gt 0 ]]; then
      info "scenario→invariant lineage edges (IMP-106 DOM-LINEAGE, advisory): declared=$invariant_edge_total"
    fi
  fi
else
  info "No scope-defined Gherkin scenarios found — scenario manifest cross-check skipped"
fi
echo ""

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  if [[ ! -f "$scope_path" ]]; then
    fail "Missing scope file: $(scope_analysis_label "$scope_index")"
    continue
  fi

  scope_label="$(scope_analysis_label "$scope_index")"
  scope_dir="$(dirname "$scope_path")"
  if [[ "$scope_layout" == "per-scope-directory" ]]; then
    report_path="$scope_dir/report.md"
  else
    report_path="$feature_dir/report.md"
  fi

  # A Not Started scope has, by definition, produced no execution evidence yet.
  # Demanding a report evidence reference from it reports the sequential
  # execution model the framework itself prescribes as a defect, and buries the
  # findings that belong to scopes actually under way. This CANNOT weaken the
  # done gate: promotion to done requires every scope to be Done, so no scope is
  # Not Started at that point and every deferral has already expired.
  scope_not_started=0
  if grep -qE '^\*\*Status:\*\*[[:space:]]*Not Started[[:space:]]*$' "$scope_path" 2>/dev/null; then
    scope_not_started=1
  fi

  info "Checking traceability for $scope_label"

  scenarios=""
  scenario_status=0
  if scenarios="$(extract_scenarios_with_ids "$scope_path")"; then
    scenario_status=0
  else
    scenario_status=$?
  fi

  if [[ "$scenario_status" -eq 1 ]] || [[ -z "$scenarios" ]]; then
    fail "$scope_label has no Gherkin scenarios to trace"
    continue
  elif [[ "$scenario_status" -ne 0 ]]; then
    fail "$scope_label Gherkin scenario extraction failed"
    continue
  fi

  test_rows=""
  test_rows_status=0
  if test_rows="$(extract_test_rows "$scope_path")"; then
    test_rows_status=0
  else
    test_rows_status=$?
  fi

  if [[ "$test_rows_status" -eq 3 ]]; then
    fail "$scope_label has no recognized Test Plan section (expected exact ## Test Plan or ### Test Plan)"
    continue
  elif [[ "$test_rows_status" -eq 5 ]]; then
    fail "$scope_label has multiple visible exact Test Plan sections; exactly one ## Test Plan or ### Test Plan is applicable"
    continue
  elif [[ "$test_rows_status" -ne 0 ]]; then
    fail "$scope_label Test Plan extraction failed"
    continue
  elif [[ -z "$test_rows" ]]; then
    fail "$scope_label has no concrete Test Plan rows to trace"
    continue
  fi

  scope_scenario_count=0
  scope_row_count=0
  while IFS= read -r _row_record; do
    [[ -n "$_row_record" ]] || continue
    scope_row_count=$((scope_row_count + 1))
  done <<< "$test_rows"

  row_total=$((row_total + scope_row_count))

  while IFS= read -r scenario_record; do
    [[ -n "$scenario_record" ]] || continue
    scenario_id="${scenario_record%%$'\t'*}"
    scenario="${scenario_record#*$'\t'}"
    [[ -n "$scenario" ]] || continue
    scope_scenario_count=$((scope_scenario_count + 1))
    scenario_total=$((scenario_total + 1))

    matched_row=""
    declared_match_count=0
    if [[ -n "$scenario_id" ]]; then
      while IFS= read -r row_record; do
        [[ -n "$row_record" ]] || continue
        row="$(test_row_raw "$row_record")"
        if trace_id_declared "$scenario_id" "$row"; then
          declared_match_count=$((declared_match_count + 1))
          [[ -n "$matched_row" ]] || matched_row="$row_record"
          if test_row_has_path "$row_record"; then
            matched_row="$row_record"
          fi
        fi
      done <<< "$test_rows"
      if [[ "$declared_match_count" -gt 1 ]]; then
        fail "$scope_label scenario has ambiguous explicit Test Plan bindings: $scenario"
        continue
      fi
    fi

    if [[ -z "$matched_row" ]]; then
      inferred_match_count=0
      while IFS= read -r row_record; do
        [[ -n "$row_record" ]] || continue
        row="$(test_row_raw "$row_record")"
        if scenario_matches_row "$scenario" "$row"; then
          inferred_match_count=$((inferred_match_count + 1))
          [[ -n "$matched_row" ]] || matched_row="$row_record"
          if test_row_has_path "$row_record"; then
            matched_row="$row_record"
          fi
        fi
      done <<< "$test_rows"
      if [[ "$inferred_match_count" -gt 1 ]]; then
        fail "$scope_label scenario has ambiguous inferred Test Plan bindings: $scenario"
        continue
      fi
    fi

    if [[ -z "$matched_row" ]]; then
      fail "$scope_label scenario has no traceable Test Plan row: $scenario"
      continue
    fi

    mapped_total=$((mapped_total + 1))
    pass "$scope_label scenario mapped to Test Plan row: $scenario"

    matched_row_raw="$(test_row_raw "$matched_row")"
    edge_kind="$(classify_match_kind "$scenario" "$matched_row_raw" "$scenario_id")"
    if [[ "$edge_kind" == "inferred" ]]; then
      _amb=0
      while IFS= read -r _r_record; do
        [[ -n "$_r_record" ]] || continue
        _r="$(test_row_raw "$_r_record")"
        if scenario_matches_row "$scenario" "$_r"; then
          _amb=$((_amb + 1))
        fi
      done <<< "$test_rows"
      [[ "$_amb" -gt 1 ]] && edge_kind="ambiguous"
    fi
    case "$edge_kind" in
      declared) edge_declared=$((edge_declared + 1)) ;;
      ambiguous) edge_ambiguous=$((edge_ambiguous + 1)) ;;
      *) edge_inferred=$((edge_inferred + 1)) ;;
    esac
    info "$scope_label scenario→row match confidence: $edge_kind"

    path_candidates="$(extract_path_candidates "$(test_row_file_location "$matched_row")")"
    if [[ -z "$path_candidates" ]]; then
      fail "$scope_label mapped row has no concrete test file path: $scenario"
      continue
    fi

    existing_path=""
    existing_path_record=""
    while IFS= read -r candidate; do
      [[ -n "$candidate" ]] || continue
      if concrete_test_path_is_acceptable "$candidate" "$scope_dir" \
        && existing_path_record="$(resolve_test_path "$candidate" "$scope_dir")"; then
        existing_path="${existing_path_record#*$'\t'}"
        existing_path="${existing_path%%$'\t'*}"
        break
      fi
    done <<< "$path_candidates"

    if [[ -z "$existing_path" ]]; then
      fail "$scope_label mapped row references no existing concrete test file: $scenario"
      continue
    fi

    file_reference_total=$((file_reference_total + 1))
    pass "$scope_label scenario maps to concrete test file: $existing_path"

    if ! revalidate_test_path "$existing_path_record"; then
      fail "$scope_label mapped test file identity changed before evidence use: $scenario"
      continue
    elif report_mentions_path "$report_path" "$existing_path" || report_mentions_path "$report_path" "$candidate"; then
      report_reference_total=$((report_reference_total + 1))
      pass "$scope_label report references concrete test evidence: $existing_path"
    elif [[ "$scope_not_started" -eq 1 ]]; then
      deferred_evidence_total=$((deferred_evidence_total + 1))
      info "$scope_label report evidence DEFERRED (scope is Not Started, so no run has produced evidence yet): $existing_path"
    else
      fail "$scope_label report is missing evidence reference for concrete test file: $existing_path"
    fi
  done <<< "$scenarios"

  info "$scope_label summary: scenarios=$scope_scenario_count test_rows=$scope_row_count"
  echo ""
done

# =============================================================================
# PASS 2: Gherkin → DoD Content Fidelity (Gate G068)
# =============================================================================
# Verifies that every Gherkin scenario's behavioral claim is faithfully
# represented by at least one DoD item. Detects the failure mode where DoD
# items are silently rewritten to match delivery instead of the spec.
# =============================================================================
echo "--- Gherkin → DoD Content Fidelity (Gate G068) ---"
dod_fidelity_total=0
dod_fidelity_mapped=0
dod_fidelity_unmapped=0

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue

  scope_label="$(scope_analysis_label "$scope_index")"
  scenarios=""
  scenario_status=0
  if scenarios="$(extract_scenarios_with_ids "$scope_path")"; then
    scenario_status=0
  else
    scenario_status=$?
  fi
  dod_items="$(extract_dod_items "$scope_path")"

  if [[ "$scenario_status" -eq 1 ]] || [[ -z "$scenarios" ]]; then
    continue
  elif [[ "$scenario_status" -ne 0 ]]; then
    fail "$scope_label Gherkin scenario extraction failed"
    continue
  fi

  if [[ -z "$dod_items" ]]; then
    fail "$scope_label has Gherkin scenarios but no DoD items — cannot verify content fidelity"
    continue
  fi

  while IFS= read -r scenario_record; do
    [[ -n "$scenario_record" ]] || continue
    scenario_id="${scenario_record%%$'\t'*}"
    scenario="${scenario_record#*$'\t'}"
    [[ -n "$scenario" ]] || continue
    dod_fidelity_total=$((dod_fidelity_total + 1))

    matched_dod=""
    declared_dod_match_count=0
    if [[ -n "$scenario_id" ]]; then
      while IFS= read -r dod_item; do
        [[ -n "$dod_item" ]] || continue
        if trace_id_declared "$scenario_id" "$dod_item"; then
          declared_dod_match_count=$((declared_dod_match_count + 1))
          [[ -n "$matched_dod" ]] || matched_dod="$dod_item"
        fi
      done <<< "$dod_items"
      if [[ "$declared_dod_match_count" -gt 1 ]]; then
        fail "$scope_label Gherkin scenario has ambiguous explicit DoD bindings: $scenario"
        dod_fidelity_unmapped=$((dod_fidelity_unmapped + 1))
        continue
      fi
    fi
    if [[ -z "$matched_dod" ]]; then
      inferred_dod_match_count=0
      while IFS= read -r dod_item; do
        [[ -n "$dod_item" ]] || continue
        if bubbles_scenario_matches_dod "$scenario" "$dod_item" id-hint-lenient; then
          inferred_dod_match_count=$((inferred_dod_match_count + 1))
          [[ -n "$matched_dod" ]] || matched_dod="$dod_item"
        fi
      done <<< "$dod_items"
      if [[ "$inferred_dod_match_count" -gt 1 ]]; then
        fail "$scope_label Gherkin scenario has ambiguous inferred DoD bindings: $scenario"
        dod_fidelity_unmapped=$((dod_fidelity_unmapped + 1))
        continue
      fi
    fi

    if [[ -z "$matched_dod" ]]; then
      fail "$scope_label Gherkin scenario has no faithful DoD item preserving its behavioral claim: $scenario"
      dod_fidelity_unmapped=$((dod_fidelity_unmapped + 1))
    else
      dod_fidelity_mapped=$((dod_fidelity_mapped + 1))
      pass "$scope_label scenario maps to DoD item: $scenario"
      edge_kind="$(classify_match_kind "$scenario" "$matched_dod" "$scenario_id")"
      if [[ "$edge_kind" == "inferred" ]]; then
        _amb=0
        while IFS= read -r _d; do
          [[ -n "$_d" ]] || continue
          if bubbles_scenario_matches_dod "$scenario" "$_d" id-hint-lenient; then
            _amb=$((_amb + 1))
          fi
        done <<< "$dod_items"
        [[ "$_amb" -gt 1 ]] && edge_kind="ambiguous"
      fi
      case "$edge_kind" in
        declared) edge_declared=$((edge_declared + 1)) ;;
        ambiguous) edge_ambiguous=$((edge_ambiguous + 1)) ;;
        *) edge_inferred=$((edge_inferred + 1)) ;;
      esac
      info "$scope_label scenario→DoD match confidence: $edge_kind"
    fi
  done <<< "$scenarios"
done

if [[ "$dod_fidelity_total" -gt 0 ]]; then
  info "DoD fidelity: $dod_fidelity_total scenarios checked, $dod_fidelity_mapped mapped to DoD, $dod_fidelity_unmapped unmapped"
  if [[ "$dod_fidelity_unmapped" -gt 0 ]]; then
    fail "DoD content fidelity gap: $dod_fidelity_unmapped Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)"
  fi
else
  info "No scenarios to check for DoD content fidelity"
fi
echo ""

echo "--- Traceability Summary ---"
info "Scenarios checked: $scenario_total"
info "Test rows checked: $row_total"
info "Scenario-to-row mappings: $mapped_total"
info "Concrete test file references: $file_reference_total"
info "Report evidence references: $report_reference_total"
if [[ "$deferred_evidence_total" -gt 0 ]]; then
  info "Report evidence DEFERRED to their own execution (Not Started scopes): $deferred_evidence_total"
fi
info "DoD fidelity scenarios: $dod_fidelity_total (mapped: $dod_fidelity_mapped, unmapped: $dod_fidelity_unmapped)"
info "Edge confidence (IMP-015 Scope B): declared=$edge_declared inferred=$edge_inferred ambiguous=$edge_ambiguous"

if [[ "$failures" -gt 0 ]]; then
  echo ""
  echo "RESULT: FAILED ($failures failures, $warnings warnings)"
  exit 1
fi

echo ""
echo "RESULT: PASSED ($warnings warnings)"
exit 0
