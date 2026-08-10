#!/usr/bin/env bash
# gate-id-grep.sh
#
# Scans governance docs and scripts for gate ID references of the form
# G[0-9]{3} and detects two failure modes:
#
#   (a) duplicate-adjacent: the same gate ID appears twice in a row,
#       separated only by spaces, commas, or both. Examples:
#           "G028, G028"          -> finding
#           "G044, G044, G044"    -> finding
#           "G028 G028"           -> finding
#       This catches accidental copy-paste regressions in lists like
#       "Gates G024, G025, G028".
#
#   (b) unknown gate ID: any G\d{3} reference whose ID is not present in
#       the canonical set extracted from bubbles/workflows.yaml. The
#       canonical set is built from every line in that file that contains
#       either "requiredGates" or "delivery-gate-baseline" — those are the
#       authoritative sources of which gates exist as workflow contracts.
#       Gate-ID bands: the framework RESERVES G001–G199 (these MUST resolve
#       to a canonical workflow contract). Project-local custom gates use
#       G900+ — any ID >= G900 is treated as a custom/local gate and is
#       always allowed (never reported as unknown). The G200–G899 span is
#       an intentional reserved gap so the two bands can never overlap.
#
# Modes:
#   default   - only fail on duplicate-adjacent findings
#   --strict  - also fail on unknown-gate-id findings
#
# Portability: both scans are POSIX awk programs (2-arg match() plus
# RSTART/RLENGTH/substr) driven over a POSIX `find` file list. There is no
# `grep -P` / PCRE dependency and no gawk extension, so this gate runs on stock
# macOS/BSD userland without Homebrew. Word boundaries are emulated by testing
# the characters adjacent to a candidate against [A-Za-z0-9_] — neither GNU `\b`
# nor BSD `[[:<:]]` is used, since each is absent on the other platform.
#
# Scanned roots (relative to --repo-root):
#   - agents/
#   - instructions/
#   - docs/
#   - bubbles/scripts/
#
# Exit codes:
#   0 - no findings (in the active mode)
#   1 - one or more findings printed
#   2 - usage error / missing inputs
#
# Usage:
#   bash bubbles/scripts/gate-id-grep.sh [--repo-root <path>] [--strict]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"
strict="false"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/gate-id-grep.sh [--repo-root <path>] [--strict]

Scans agents/, instructions/, docs/, bubbles/scripts/ under <repo-root>
for gate ID patterns (G[0-9]{3}). Detects:
  - duplicate-adjacent IDs (e.g., "G028, G028")
  - unknown gate IDs not present in workflows.yaml requiredGates lists
    (only reported under --strict; project-local custom gates G900+ are
    always allowed; framework gates G001-G199 must resolve canonically)

Options:
  --repo-root <path>   Repo root to scan (default: script repo root)
  --strict             Also fail on unknown-gate-id findings
  -h, --help           Print this help

Exit 0 when no findings in the active mode, 1 when findings exist.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --strict)
      strict="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "gate-id-grep: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "gate-id-grep: repo root not found: $repo_root" >&2
  exit 2
fi

# --- awk capability guard --------------------------------------------------
#
# Both scans below run through awk. awk is POSIX-mandated, but if it is missing
# or cannot execute a trivial program the scans would emit ZERO rows and the
# gate would SILENTLY PASS — a false negative. Fail loud instead, exactly as the
# former PCRE guard did for the GNU-only `grep -P` this implementation removed.
# Override the interpreter with BUBBLES_AWK if a repo needs a specific awk.
AWK_BIN="${BUBBLES_AWK:-awk}"
if ! printf 'x\n' | "$AWK_BIN" '{ print }' >/dev/null 2>&1; then
  echo "gate-id-grep: requires a working POSIX awk; '$AWK_BIN' did not run." >&2
  echo "  This gate cannot scan reliably without awk and refuses to pass silently." >&2
  exit 2
fi

workflows_yaml="$repo_root/bubbles/workflows.yaml"
if [[ ! -f "$workflows_yaml" ]]; then
  echo "gate-id-grep: bubbles/workflows.yaml not found at $workflows_yaml" >&2
  exit 2
fi

# v6.1 (S2 true split): mode definitions (including each mode's requiredGates)
# live in bubbles/workflows/modes.yaml. Include it in the canonical-gate
# extraction so a gate referenced only by a mode is not flagged unknown.
gate_source_files=("$workflows_yaml")
modes_yaml="$repo_root/bubbles/workflows/modes.yaml"
[[ -f "$modes_yaml" ]] && gate_source_files+=("$modes_yaml")

# --- Shared POSIX-awk gate-ID scanner --------------------------------------
#
# One program, three output modes (-v mode=...):
#   bare  - print each word-bounded G### token             -> "G024"
#   refs  - print every occurrence with its position       -> "file:line:G024"
#           (one row per OCCURRENCE, i.e. `grep -o` semantics)
#   dups  - print the whole line once when it contains the SAME id twice in a
#           row separated only by spaces/commas            -> "file:line:<line>"
#
# Word boundaries: a candidate G### is accepted only when the characters
# immediately before and after it are absent or non-[A-Za-z0-9_]. That is the
# ASCII \w rule PCRE's \b applies, expressed with portable string tests, so
# "G1234" never matches as "G123" and "XG123" never matches at all.
#
# Binary skip: a file whose FIRST record carries a C0 control byte (anything
# below 0x20 except tab/CR, or DEL) is treated as binary and skipped whole —
# the portable stand-in for `grep --binary-files=without-match`, which awk
# cannot reproduce exactly because several awks truncate records at NUL.
# shellcheck disable=SC2016  # awk program text: $0 must NOT be shell-expanded
AWK_PROG='
BEGIN {
  for (i = 1; i < 32; i++) {
    if (i != 9 && i != 13) ctrl = ctrl sprintf("%c", i)
  }
  ctrl = ctrl sprintf("%c", 127)
}
function looks_binary(s,   i, n, c) {
  n = length(s)
  if (n > 512) n = 512
  for (i = 1; i <= n; i++) {
    c = substr(s, i, 1)
    if (index(ctrl, c) > 0) return 1
  }
  return 0
}
function is_word(c) { return (c != "" && c ~ /[A-Za-z0-9_]/) }
FNR == 1 { skipfile = looks_binary($0) }
skipfile { next }
{
  line = $0
  base = 0
  rest = line
  n = 0
  while (match(rest, /G[0-9][0-9][0-9]/)) {
    s = base + RSTART
    if (!is_word((s > 1) ? substr(line, s - 1, 1) : "") \
        && !is_word(substr(line, s + 4, 1))) {
      tok = substr(line, s, 4)
      if (mode == "bare") print tok
      else if (mode == "refs") printf "%s:%d:%s\n", FILENAME, FNR, tok
      else { n++; tokv[n] = tok; posv[n] = s }
      base = s + 3
    } else {
      base = s
    }
    rest = substr(line, base + 1)
  }
  if (mode == "dups") {
    for (i = 2; i <= n; i++) {
      if (tokv[i] == tokv[i - 1] \
          && substr(line, posv[i - 1] + 4, posv[i] - posv[i - 1] - 4) ~ /^[ ,]+$/) {
        printf "%s:%d:%s\n", FILENAME, FNR, line
        break
      }
    }
  }
}
'

# --- Build canonical gate ID set from workflows.yaml -----------------------
#
# Canonical sources are lines mentioning either "requiredGates" or
# "delivery-gate-baseline". We extract every G[0-9]{3} occurrence on those
# lines. Project-local custom gates (>= G900) are also implicitly allowed at
# lookup time; framework gates (G001-G199) must resolve canonically.

declare -A canonical_set=()
while IFS= read -r gate_id; do
  [[ -z "$gate_id" ]] && continue
  canonical_set["$gate_id"]=1
done < <(
  grep -E 'requiredGates|delivery-gate-baseline' "${gate_source_files[@]}" \
    | LC_ALL=C "$AWK_BIN" -v mode=bare "$AWK_PROG" \
    | LC_ALL=C sort -u
)

if [[ "${#canonical_set[@]}" -eq 0 ]]; then
  echo "gate-id-grep: failed to extract any canonical gate IDs from $workflows_yaml" >&2
  exit 2
fi

# --- Discover scan targets -------------------------------------------------

declare -a scan_roots=()
for sub in agents instructions docs bubbles/scripts; do
  if [[ -d "$repo_root/$sub" ]]; then
    scan_roots+=("$repo_root/$sub")
  fi
done

# IMP-027 SCOPE-10: README.md is the framework's most-read surface and carried a
# reference to a RETIRED gate (former G039, absorbed by G038) that no scan could
# see because the roots above are directories only. Scan it explicitly.
if [[ -f "$repo_root/README.md" ]]; then
  scan_roots+=("$repo_root/README.md")
fi

if [[ "${#scan_roots[@]}" -eq 0 ]]; then
  echo "gate-id-grep: no scan roots present under $repo_root" >&2
  exit 2
fi

# --- Enumerate the concrete files to scan ----------------------------------
#
# `grep -r` followed symlinks named on the command line, never descended into
# .git, and skipped binary files. `find -H` + the -prune below + the awk binary
# rule reproduce that with POSIX-only flags. The list is sorted so finding order
# is stable across platforms (raw readdir order is not).

declare -a scan_files=()
while IFS= read -r scan_file; do
  [[ -z "$scan_file" ]] && continue
  [[ -r "$scan_file" ]] || continue
  scan_files+=("$scan_file")
done < <(
  find -H "${scan_roots[@]}" -name '.git' -prune -o -type f -print 2>/dev/null \
    | LC_ALL=C sort
)

if [[ "${#scan_files[@]}" -eq 0 ]]; then
  echo "gate-id-grep: no readable files under the scan roots of $repo_root" >&2
  exit 2
fi

# Run AWK_PROG over every scan file in ARG_MAX-safe batches. Returns non-zero if
# any batch fails so the caller can fail loud instead of scanning nothing.
run_scan() {
  local mode="$1"
  local -a batch=()
  local f
  for f in "${scan_files[@]}"; do
    batch+=("$f")
    if [[ "${#batch[@]}" -ge 400 ]]; then
      LC_ALL=C "$AWK_BIN" -v mode="$mode" "$AWK_PROG" "${batch[@]}" || return 1
      batch=()
    fi
  done
  if [[ "${#batch[@]}" -gt 0 ]]; then
    LC_ALL=C "$AWK_BIN" -v mode="$mode" "$AWK_PROG" "${batch[@]}" || return 1
  fi
  return 0
}

# This scanner and its selftest necessarily CONTAIN illustrative duplicate-ID
# examples ("G028, G028") in their documentation and fixtures. Scanning them
# reports the examples as findings, which is a false positive that would make
# the live scan permanently red. Exclude exactly those two files by name.
self_exclude_re='/(gate-id-grep|gate-id-grep-selftest)\.sh:'

dup_findings_file="$(mktemp)"
ref_findings_file="$(mktemp)"
unknown_findings_file="$(mktemp)"
trap 'rm -f "$dup_findings_file" "$ref_findings_file" "$unknown_findings_file"' EXIT INT TERM

# --- Detect duplicate-adjacent findings ------------------------------------
#
# A duplicate-adjacent match is the same G\d{3} ID separated only by
# spaces and/or commas. Example match: "G028, G028" or "G044 G044".
# One row per matching LINE, matching the old `grep -n` output contract.

if ! dup_raw="$(run_scan dups)"; then
  echo "gate-id-grep: duplicate-adjacent scan failed (awk returned non-zero)." >&2
  echo "  Refusing to report zero findings from a scan that did not complete." >&2
  exit 2
fi
if [[ -n "$dup_raw" ]]; then
  printf '%s\n' "$dup_raw" | grep -vE "$self_exclude_re" > "$dup_findings_file" || true
fi
unset dup_raw

dup_count="$(wc -l < "$dup_findings_file" | tr -d ' ')"

# --- Detect unknown gate ID findings (used under --strict) -----------------
#
# Collect every <file>:<line>:<G\d{3}> reference, then for each ID check
# canonical membership. Project-local custom gates (>= G900) are always allowed.

if ! run_scan refs > "$ref_findings_file"; then
  echo "gate-id-grep: reference scan failed (awk returned non-zero)." >&2
  echo "  Refusing to report zero findings from a scan that did not complete." >&2
  exit 2
fi

while IFS=: read -r file line id; do
  [[ -z "$id" ]] && continue
  # Project-local custom gates (>= G900) are always allowed; the framework
  # band G001-G199 must resolve canonically (G200-G899 is the reserved gap).
  numeric="${id#G}"
  numeric="${numeric#0}"
  numeric="${numeric#0}"
  if [[ -z "$numeric" ]]; then
    numeric=0
  fi
  if (( 10#$numeric >= 900 )); then
    continue
  fi
  if [[ -z "${canonical_set[$id]:-}" ]]; then
    printf '%s:%s:%s\n' "$file" "$line" "$id" >> "$unknown_findings_file"
  fi
done < "$ref_findings_file"

unknown_count="$(wc -l < "$unknown_findings_file" | tr -d ' ')"

# --- Report ---------------------------------------------------------------

active_failures=0

if [[ "$dup_count" -gt 0 ]]; then
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf 'FINDING: duplicate-adjacent: %s\n' "$line"
  done < "$dup_findings_file"
  active_failures=$((active_failures + dup_count))
fi

if [[ "$strict" == "true" && "$unknown_count" -gt 0 ]]; then
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf 'FINDING: unknown-gate-id: %s\n' "$line"
  done < "$unknown_findings_file"
  active_failures=$((active_failures + unknown_count))
fi

if [[ "$active_failures" -eq 0 ]]; then
  echo "[gate-id-grep] OK — zero findings"
  exit 0
fi

# Print a brief summary footer to make pipeline logs easier to scan.
if [[ "$strict" == "true" ]]; then
  echo "[gate-id-grep] FAIL — duplicate-adjacent: $dup_count, unknown: $unknown_count (strict)" >&2
else
  echo "[gate-id-grep] FAIL — duplicate-adjacent: $dup_count" >&2
fi
exit 1
