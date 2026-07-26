#!/usr/bin/env bash
# skill-description-load.sh — IMP-021 SCOPE-5 (ID5): report the model-facing
# skill-description context load and each skill's invocation class.
#
# In this VS Code harness EVERY skill's `description:` frontmatter is auto-loaded
# into the model's <skills> block, so the aggregate byte length of all
# auto-discovery-required skill descriptions is a real, always-on context cost.
# This report surfaces that cost.
#
# REPORT-ONLY for the byte load: it prints the totals and exits 0. There is NO
# byte threshold and NO --skip/--force bypass — a blocking threshold is DEFERRED
# until calibration evidence justifies one (IMP-021 SCOPE-5 acceptance: expose
# the aggregate load "without ... an uncalibrated blocking threshold"). An
# optional advisory budget (skillDescriptionLoadMaxBytes in
# .github/bubbles-project.yaml) is printed but NEVER blocks.
#
# Invocation class is an AUTHORING JUDGMENT recorded in skills/INVENTORY.md — the
# VS Code harness has no per-skill auto-load toggle, so the class lives in the
# inventory, never in unsupported SKILL.md frontmatter. Description bytes are
# computed LIVE from each skills/<name>/SKILL.md so the aggregate is always real.
#
# STRUCTURAL COMPLETENESS is enforced (this is input validation, NOT a budget
# verdict): the inventory table MUST carry an `Invocation` and a `DescBytes`
# column, and every real skill row MUST record a valid Invocation class plus a
# numeric DescBytes. A row that omits either is a hard error (exit 1) — that is
# how a skill "missing its Invocation class" is flagged. This complements
# inventory-parity-check.sh (which enforces skill name-set parity). A recorded
# DescBytes that drifts from the live byte count is a NON-fatal advisory note.
#
# Usage:
#   skill-description-load.sh [--repo-root DIR] [--summary]
#     --repo-root DIR  repo root (default: inferred from this script's location)
#     --summary        print only the aggregate lines (used by framework-validate)
#
# Exit codes:
#   0  report printed (structurally complete inventory)
#   1  INVENTORY incomplete — missing Invocation/DescBytes column, OR a real
#      skill row omits its Invocation class or numeric DescBytes
#   2  usage / malformed input (missing skills dir or INVENTORY.md, -h)

set -euo pipefail

AUTO_CLASS="auto-discovery-required"
EXPLICIT_CLASS="explicit-invocation-sufficient"

usage() {
  echo "Usage: bash skill-description-load.sh [--repo-root DIR] [--summary]" >&2
}

REPO_ROOT_ARG=""
SUMMARY_ONLY="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 2
      ;;
    --summary)
      SUMMARY_ONLY="true"
      shift
      ;;
    --repo-root)
      REPO_ROOT_ARG="${2:-}"
      shift 2
      ;;
    --repo-root=*)
      REPO_ROOT_ARG="${1#--repo-root=}"
      shift
      ;;
    *)
      echo "[skill-description-load][USAGE] unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "$REPO_ROOT_ARG" ]]; then
  REPO_ROOT="$REPO_ROOT_ARG"
elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  # downstream install tree: .github/bubbles/scripts/ -> repo root
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  # framework source tree: bubbles/scripts/ -> repo root
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

SKILLS_DIR="$REPO_ROOT/skills"
INVENTORY="$SKILLS_DIR/INVENTORY.md"

if [[ ! -d "$SKILLS_DIR" ]]; then
  echo "[skill-description-load][USAGE] skills/ directory not found at: $SKILLS_DIR" >&2
  exit 2
fi
if [[ ! -f "$INVENTORY" ]]; then
  echo "[skill-description-load][USAGE] skills/INVENTORY.md not found at: $INVENTORY" >&2
  exit 2
fi

# description_bytes <skill-dir> — byte length of the SKILL.md `description:`
# frontmatter value (single line; one optional leading space stripped). 0 if
# the file or the description line is absent.
description_bytes() {
  local md="$1/SKILL.md"
  [[ -f "$md" ]] || {
    echo 0
    return 0
  }
  local val
  val="$(awk '/^description:/{sub(/^description:[ ]?/, ""); print; exit}' "$md")"
  printf '%s' "$val" | wc -c | tr -d ' '
}

# --- Parse the inventory table into: name<TAB>class<TAB>recordedBytes ---
# The header row is the first `| ... |` line carrying Skill + Invocation +
# DescBytes; column positions are resolved from it (robust to reordering). The
# separator row and prose lines are ignored (no backtick name token).
inv_rows="$(mktemp)"
trap 'rm -f "$inv_rows"' EXIT

awk -F'|' '
  BEGIN { name_col = 0; inv_col = 0; db_col = 0; header_seen = 0 }
  !header_seen && /^\|/ && /Skill/ && /Invocation/ && /DescBytes/ {
    for (i = 1; i <= NF; i++) {
      c = $i
      gsub(/^[ \t]+|[ \t]+$/, "", c)
      if (c == "Skill") name_col = i
      if (c == "Invocation") inv_col = i
      if (c == "DescBytes") db_col = i
    }
    header_seen = 1
    next
  }
  header_seen && /^\|/ {
    ncell = $name_col
    if (match(ncell, /`[^`]+`/)) {
      nm = substr(ncell, RSTART + 1, RLENGTH - 2)
    } else {
      next
    }
    icell = $inv_col
    gsub(/^[ \t]+|[ \t]+$/, "", icell)
    dcell = $db_col
    gsub(/^[ \t]+|[ \t]+$/, "", dcell)
    print nm "\t" icell "\t" dcell
  }
  END { if (!header_seen) exit 3 }
' "$INVENTORY" >"$inv_rows" || {
  echo "[skill-description-load][ERROR] skills/INVENTORY.md has no table header with Skill + Invocation + DescBytes columns." >&2
  echo "[skill-description-load] FAIL — upgrade the inventory table with Invocation and DescBytes columns." >&2
  exit 1
}

declare -A REC_CLASS=()
declare -A REC_BYTES=()
while IFS=$'\t' read -r nm cls db; do
  [[ -n "$nm" ]] || continue
  REC_CLASS["$nm"]="$cls"
  REC_BYTES["$nm"]="$db"
done <"$inv_rows"

# --- Real skill dirs on disk (have SKILL.md, exclude `__*` probe dirs) ---
disk_skills="$(mktemp)"
trap 'rm -f "$inv_rows" "$disk_skills"' EXIT
shopt -s nullglob
for d in "$SKILLS_DIR"/*/; do
  name="$(basename "$d")"
  case "$name" in
    __*) continue ;;
  esac
  if [[ -f "$d/SKILL.md" ]]; then
    echo "$name"
  fi
done | LC_ALL=C sort -u >"$disk_skills"

auto_total=0
explicit_total=0
grand_total=0
auto_count=0
explicit_count=0
skill_count=0
declare -a missing_class=()
declare -a missing_bytes=()
declare -a drift_notes=()
declare -a table_rows=()

while IFS= read -r name; do
  [[ -n "$name" ]] || continue
  skill_count=$((skill_count + 1))
  live_bytes="$(description_bytes "$SKILLS_DIR/$name")"
  grand_total=$((grand_total + live_bytes))

  cls="${REC_CLASS[$name]:-}"
  rec_bytes="${REC_BYTES[$name]:-}"

  if [[ "$cls" == "$AUTO_CLASS" ]]; then
    auto_total=$((auto_total + live_bytes))
    auto_count=$((auto_count + 1))
  elif [[ "$cls" == "$EXPLICIT_CLASS" ]]; then
    explicit_total=$((explicit_total + live_bytes))
    explicit_count=$((explicit_count + 1))
  else
    missing_class+=("$name")
  fi

  if [[ ! "$rec_bytes" =~ ^[0-9]+$ ]]; then
    missing_bytes+=("$name")
  elif [[ "$rec_bytes" -ne "$live_bytes" ]]; then
    drift_notes+=("$name: recorded DescBytes $rec_bytes != live $live_bytes")
  fi

  table_rows+=("$(printf '%-42s %-32s %6s' "$name" "${cls:-<MISSING>}" "$live_bytes")")
done <"$disk_skills"

# --- Structural completeness (input validation, not a budget verdict) ---
if [[ ${#missing_class[@]} -gt 0 || ${#missing_bytes[@]} -gt 0 ]]; then
  if [[ ${#missing_class[@]} -gt 0 ]]; then
    echo "[skill-description-load][ERROR] skill(s) with NO valid Invocation class in skills/INVENTORY.md:" >&2
    for s in "${missing_class[@]}"; do echo "  - $s" >&2; done
  fi
  if [[ ${#missing_bytes[@]} -gt 0 ]]; then
    echo "[skill-description-load][ERROR] skill(s) with NO numeric DescBytes in skills/INVENTORY.md:" >&2
    for s in "${missing_bytes[@]}"; do echo "  - $s" >&2; done
  fi
  echo "[skill-description-load] FAIL — record an Invocation class ($AUTO_CLASS | $EXPLICIT_CLASS) and a numeric DescBytes for every skill row." >&2
  exit 1
fi

# --- Report (byte load is report-only; exit 0) ---
if [[ "$SUMMARY_ONLY" != "true" ]]; then
  printf '\n%-42s %-32s %6s\n' "Skill" "Invocation" "DescB"
  printf '%-42s %-32s %6s\n' "-----" "----------" "-----"
  for r in "${table_rows[@]}"; do echo "$r"; done
fi

echo
echo "=== Skill description-load report (IMP-021 SCOPE-5) ==="
printf 'skills total:                     %d\n' "$skill_count"
printf 'auto-discovery-required:          %d skill(s), %d bytes  (always-loaded context cost)\n' "$auto_count" "$auto_total"
printf 'explicit-invocation-sufficient:   %d skill(s), %d bytes\n' "$explicit_count" "$explicit_total"
printf 'all skill descriptions:           %d bytes\n' "$grand_total"

if [[ ${#drift_notes[@]} -gt 0 ]]; then
  echo "advisory (non-fatal) — recorded DescBytes drift vs live SKILL.md:"
  for n in "${drift_notes[@]}"; do echo "  - $n"; done
fi

# --- Optional advisory budget (report-only; NEVER blocks in this MVP) ---
max_bytes=""
dir="$REPO_ROOT"
project_config=""
while :; do
  if [[ -f "$dir/.github/bubbles-project.yaml" ]]; then
    project_config="$dir/.github/bubbles-project.yaml"
    break
  fi
  [[ "$dir" == "/" || -z "$dir" ]] && break
  dir="$(dirname "$dir")"
done
if [[ -n "$project_config" ]]; then
  max_bytes="$({ grep -oE '^[[:space:]]*skillDescriptionLoadMaxBytes:[[:space:]]*[0-9]+' "$project_config" || true; } | grep -oE '[0-9]+' | head -1 || true)"
fi
if [[ -n "$max_bytes" ]]; then
  if [[ "$auto_total" -gt "$max_bytes" ]]; then
    printf 'advisory: auto-discovery load %d bytes is OVER skillDescriptionLoadMaxBytes (%d) — advisory only, MVP is report-only (no block).\n' "$auto_total" "$max_bytes"
  else
    printf 'advisory: auto-discovery load %d bytes within skillDescriptionLoadMaxBytes (%d).\n' "$auto_total" "$max_bytes"
  fi
else
  printf 'no skillDescriptionLoadMaxBytes configured — report-only (no threshold).\n'
fi

echo "[skill-description-load] OK — report-only, exit 0."
exit 0
