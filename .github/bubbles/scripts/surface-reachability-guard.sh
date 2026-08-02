#!/usr/bin/env bash
# surface-reachability-guard.sh — IMP-031 SCOPE-3: reconcile the surfaces a
# repository ACTUALLY exposes against the surfaces its specs CLAIM to expose,
# and report the orphans in both directions.
#
# The question this answers is "can a caller reach what we built?", and it has
# never been answerable before because only one half of the comparison existed.
# A spec could declare an Exposure Contract, and the framework could read it,
# but there was nothing to compare it against — so the check could only confirm
# that a declaration agreed with itself. The `surfaces:` block (IMP-031 SCOPE-2)
# supplies the missing half by naming a project-owned command that DERIVES each
# class of surface from source. This guard consumes both halves and reports:
#
#   ORPHANED SURFACE    a surface exists in source that no spec declares.
#                       Something is reachable that nobody committed to.
#   UNDELIVERED CLAIM   a spec declares a `delivered` surface that the
#                       derivation does not return. The delivery is a claim.
#
# REPORT-ONLY for the reconciliation. It prints findings and exits 0. There is
# NO orphan threshold and NO --skip/--force/--ignore bypass. A blocking
# threshold is DEFERRED until calibration evidence supports one — the precedent
# is skill-description-load.sh, which reports an aggregate load without an
# uncalibrated blocking verdict. Blocking on a denominator nobody has yet shown
# to be trustworthy would recreate the exact failure this guard exists to
# remove.
#
# DERIVATION INTEGRITY *is* enforced (this is input validation, not a
# reconciliation verdict, and it is the most important rule here). A declared
# class whose derive command is missing, errors, or returns ZERO records is a
# HARD ERROR (exit 1). An empty inventory must never read as agreement: a check
# that passes because it examined nothing is the precise defect being closed.
# If a product genuinely has no surfaces of some class, it declares no class —
# it does not declare one and derive nothing.
#
# Wire format between a derive command and this guard: one TAB-separated record
# per reachable surface on stdout —
#
#     <class>\t<id>\t<path>\t<sourceFile>
#
# Blank lines and `#` comment lines are ignored. A record whose class field
# disagrees with the declared class is a hard error, because a derivation that
# mislabels its own output cannot be reconciled against anything.
#
# `derive: codeIndex` routes through codeindex-resolve.sh and invokes the
# resolved adapter as `<adapter> surfaces --class <class>`. An adapter that
# resolves to `none` is a hard error for that class: routing a class to an
# unconfigured index is just another way to produce an empty denominator.
#
# NOT APPLICABLE (exit 0, silent success) when the repo declares no `surfaces:`
# block, so no existing repository is disturbed by this guard shipping. The
# Bubbles source checkout resolves EXEMPT for the same structural reason the
# observability gate exempts it: the framework is a governance payload, not a
# running product, and has no runtime surfaces to derive.
#
# Usage:
#   surface-reachability-guard.sh [--repo-root DIR] [--specs-dir DIR]
#     --repo-root DIR  repo root (default: inferred from this script's location)
#     --specs-dir DIR  specs root (default: <repo-root>/specs)
#
# Exit codes:
#   0  report printed (INCLUDING when orphans were found — report-only), or the
#      repo declares no surfaces: block, or the repo is EXEMPT
#   1  derivation integrity failure — a declared class is missing its command,
#      the command failed, it returned no records, or it mislabelled a record
#   2  usage / malformed input

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: surface-reachability-guard.sh [--repo-root DIR] [--specs-dir DIR]

Reconciles the derived surface inventory (the `surfaces:` block in
.github/bubbles-project.yaml) against the Exposure Contract tables declared in
specs/**/spec.md, and reports orphaned surfaces and undelivered claims.

Report-only: findings never change the exit code. A broken derivation does
(exit 1) — an empty inventory is a defect, never agreement. There is no
--skip/--force/--ignore bypass.
EOF
}

REPO_ROOT_ARG=""
SPECS_DIR_ARG=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 2
      ;;
    --repo-root)
      REPO_ROOT_ARG="${2:-}"
      shift 2
      ;;
    --repo-root=*)
      REPO_ROOT_ARG="${1#--repo-root=}"
      shift
      ;;
    --specs-dir)
      SPECS_DIR_ARG="${2:-}"
      shift 2
      ;;
    --specs-dir=*)
      SPECS_DIR_ARG="${1#--specs-dir=}"
      shift
      ;;
    *)
      echo "[surface-reachability][USAGE] unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "$REPO_ROOT_ARG" ]]; then
  [[ -d "$REPO_ROOT_ARG" ]] || {
    echo "[surface-reachability][USAGE] --repo-root is not a directory: $REPO_ROOT_ARG" >&2
    exit 2
  }
  REPO_ROOT="$(cd "$REPO_ROOT_ARG" && pwd)"
elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  # downstream install tree: .github/bubbles/scripts/ -> repo root
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  # framework source tree: bubbles/scripts/ -> repo root
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

SPECS_DIR="${SPECS_DIR_ARG:-$REPO_ROOT/specs}"

# ---------------------------------------------------------------------------
# EXEMPT: the Bubbles framework source checkout has no runtime to expose.
# Detected structurally (its own payload layout), not by directory name, so a
# consumer repo that happens to be called "bubbles" is still checked.
# ---------------------------------------------------------------------------
if [[ -f "$REPO_ROOT/VERSION" && -f "$REPO_ROOT/install.sh" && -d "$REPO_ROOT/bubbles/scripts" && -d "$REPO_ROOT/agents/bubbles_shared" ]]; then
  echo "[surface-reachability] EXEMPT: Bubbles framework source checkout has no product runtime to derive surfaces from."
  exit 0
fi

CONFIG=""
for candidate in "$REPO_ROOT/.github/bubbles-project.yaml" "$REPO_ROOT/bubbles-project.yaml"; do
  if [[ -f "$candidate" ]]; then
    CONFIG="$candidate"
    break
  fi
done

if [[ -z "$CONFIG" ]]; then
  echo "[surface-reachability] not applicable: no bubbles-project.yaml under $REPO_ROOT."
  exit 0
fi

# ---------------------------------------------------------------------------
# Parse the `surfaces:` block. Deliberately awk-only: framework validation runs
# on a minimal PATH, and a guard that needs yq installed is a guard that
# silently does not run.
#
# Accepts both the inline flow form and the expanded form:
#     httpRoute:  { derive: "scripts/inventory/http-routes.sh" }
#     httpRoute:
#       derive: scripts/inventory/http-routes.sh
# Emits one `<class>\t<derive>` line per declared class.
# ---------------------------------------------------------------------------
declared="$(
  awk '
    function flush() {
      # A class declared with no derive command still counts as declared. It is
      # reported later as an integrity failure, because dropping it here would
      # turn a misconfiguration into a silent no-op — the failure mode this
      # whole contract exists to remove.
      if (pending != "") { print pending "\t"; pending = "" }
    }
    /^[^[:space:]#]/ { flush(); in_surfaces = 0; in_classes = 0 }
    /^surfaces:[[:space:]]*$/ { in_surfaces = 1; next }
    in_surfaces == 0 { next }
    /^[[:space:]]{2}classes:[[:space:]]*$/ { flush(); in_classes = 1; next }
    /^[[:space:]]{2}[A-Za-z0-9_-]+:/ && !/^[[:space:]]{2}classes:/ { flush(); in_classes = 0 }
    in_classes == 0 { next }
    /^[[:space:]]{4}[A-Za-z0-9_-]+:/ {
      flush()
      line = $0
      name = line
      sub(/^[[:space:]]+/, "", name)
      sub(/:.*$/, "", name)
      rest = line
      sub(/^[[:space:]]*[A-Za-z0-9_-]+:[[:space:]]*/, "", rest)
      if (rest ~ /derive/) {
        sub(/^.*derive[[:space:]]*:[[:space:]]*/, "", rest)
        sub(/[[:space:]]*}[[:space:]]*$/, "", rest)
        gsub(/^["'"'"']|["'"'"']$/, "", rest)
        sub(/[[:space:]]+$/, "", rest)
        print name "\t" rest
      } else {
        pending = name
      }
      next
    }
    /^[[:space:]]{6}derive[[:space:]]*:/ && pending != "" {
      rest = $0
      sub(/^.*derive[[:space:]]*:[[:space:]]*/, "", rest)
      gsub(/^["'"'"']|["'"'"']$/, "", rest)
      sub(/[[:space:]]+$/, "", rest)
      print pending "\t" rest
      pending = ""
      next
    }
    END { flush() }
  ' "$CONFIG"
)"

if [[ -z "$declared" ]]; then
  echo "[surface-reachability] not applicable: no surfaces: block declared in $CONFIG."
  exit 0
fi

# ---------------------------------------------------------------------------
# Derive the inventory, one class at a time. Every failure mode here is loud.
# ---------------------------------------------------------------------------
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/surface-reachability.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
inventory="$work_dir/inventory.tsv"
: > "$inventory"

integrity_errors=""
record_integrity_error() {
  integrity_errors="${integrity_errors}  - $1"$'\n'
}

while IFS=$'\t' read -r class derive_cmd; do
  [[ -n "$class" ]] || continue
  if [[ -z "$derive_cmd" ]]; then
    record_integrity_error "class '$class' declares no derive command."
    continue
  fi

  if [[ "$derive_cmd" == "codeIndex" ]]; then
    resolver="$SCRIPT_DIR/codeindex-resolve.sh"
    if [[ ! -x "$resolver" ]]; then
      record_integrity_error "class '$class' routes to codeIndex but codeindex-resolve.sh is not executable at $resolver."
      continue
    fi
    resolved="$("$resolver" --repo-root "$REPO_ROOT" 2>/dev/null || true)"
    adapter_name="$(printf '%s\n' "$resolved" | awk -F= '$1 == "adapter" { print $2 }')"
    adapter_path="$(printf '%s\n' "$resolved" | awk -F= '$1 == "adapterPath" { print $2 }')"
    if [[ -z "$adapter_name" || "$adapter_name" == "none" ]]; then
      record_integrity_error "class '$class' routes to codeIndex, but this repo resolves adapter='none' — an unconfigured index derives nothing, which is an empty denominator, not a clean pass."
      continue
    fi
    derive_cmd="$adapter_path surfaces --class $class"
  fi

  raw="$work_dir/raw-$class.out"
  if ! (cd "$REPO_ROOT" && bash -c "$derive_cmd") > "$raw" 2> "$work_dir/raw-$class.err"; then
    detail="$(head -n1 "$work_dir/raw-$class.err" 2>/dev/null || true)"
    record_integrity_error "class '$class' derive command failed: $derive_cmd${detail:+ — $detail}"
    continue
  fi

  count=0
  mislabelled=0
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    case "$line" in '#'*) continue ;; esac
    rec_class="${line%%$'\t'*}"
    if [[ "$rec_class" != "$class" ]]; then
      mislabelled=$((mislabelled + 1))
      continue
    fi
    printf '%s\n' "$line" >> "$inventory"
    count=$((count + 1))
  done < "$raw"

  if [[ "$mislabelled" -gt 0 ]]; then
    record_integrity_error "class '$class' emitted $mislabelled record(s) labelled with a different class — a derivation that mislabels its own output cannot be reconciled."
    continue
  fi
  if [[ "$count" -eq 0 ]]; then
    record_integrity_error "class '$class' derived ZERO surfaces. Declaring a class asserts the product exposes it; deriving nothing is a broken command, not an empty product."
    continue
  fi
done <<< "$declared"

if [[ -n "$integrity_errors" ]]; then
  {
    echo "[surface-reachability] DERIVATION INTEGRITY FAILURE in $CONFIG:"
    printf '%s' "$integrity_errors"
    echo "  Remediation: fix or remove the declaration. A class that derives nothing"
    echo "  makes every downstream reachability answer meaningless, because the"
    echo "  comparison then runs against an empty set and agrees with everything."
  } >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Collect declared exposure from every spec's Exposure Contract table.
#
# Rows inside fenced code blocks are IGNORED. The framework ships the Exposure
# Contract table as a TEMPLATE inside a ``` fence, and a scanner that cannot
# tell an example from a declaration would read the framework's own
# documentation as a product commitment. That imprecision — matching text in
# comments, fences, and generated docs — is the detector defect this proposal
# documents; this guard must not reproduce it.
# ---------------------------------------------------------------------------
declared_exposure="$work_dir/declared.tsv"
: > "$declared_exposure"

if [[ -d "$SPECS_DIR" ]]; then
  while IFS= read -r spec_file; do
    [[ -n "$spec_file" ]] || continue
    awk -v specfile="$spec_file" '
      /^[[:space:]]*```/ { fenced = !fenced; next }
      fenced { next }
      /^##[[:space:]]+Exposure Contract[[:space:]]*$/ { in_table = 1; next }
      /^##[[:space:]]/ { in_table = 0 }
      in_table == 0 { next }
      /^[[:space:]]*\|/ {
        line = $0
        if (line ~ /^[[:space:]]*\|[[:space:]]*-*[-: |]*\|[[:space:]]*$/) next
        n = split(line, cell, "|")
        if (n < 6) next
        for (i = 1; i <= n; i++) {
          gsub(/^[[:space:]]+|[[:space:]]+$/, "", cell[i])
          gsub(/`/, "", cell[i])
        }
        if (tolower(cell[2]) == "capability") next
        if (cell[3] == "" || cell[4] == "") next
        print cell[3] "\t" cell[4] "\t" tolower(cell[5]) "\t" cell[2] "\t" specfile
      }
    ' "$spec_file" >> "$declared_exposure"
  done < <(find "$SPECS_DIR" -type f -name 'spec.md' 2>/dev/null | sort)
fi

# ---------------------------------------------------------------------------
# Reconcile.
# ---------------------------------------------------------------------------
orphans=""
while IFS=$'\t' read -r class id path source_file; do
  [[ -n "$class" ]] || continue
  if ! awk -F'\t' -v c="$class" -v i="$id" '$1 == c && $2 == i { found = 1 } END { exit found ? 0 : 1 }' "$declared_exposure"; then
    orphans="${orphans}    $class  $id  (${source_file:-${path:-source unknown}})"$'\n'
  fi
done < "$inventory"

undelivered=""
while IFS=$'\t' read -r class id status capability spec_file; do
  [[ "$status" == "delivered" ]] || continue
  if ! awk -F'\t' -v c="$class" -v i="$id" '$1 == c && $2 == i { found = 1 } END { exit found ? 0 : 1 }' "$inventory"; then
    undelivered="${undelivered}    $class  $id  (capability ${capability:-unnamed}, declared in ${spec_file})"$'\n'
  fi
done < "$declared_exposure"

derived_total="$(wc -l < "$inventory" | tr -d '[:space:]')"
declared_total="$(wc -l < "$declared_exposure" | tr -d '[:space:]')"

echo "[surface-reachability] report for $REPO_ROOT"
echo "  derived surfaces:   $derived_total"
echo "  declared exposures: $declared_total"

if [[ -z "$orphans" && -z "$undelivered" ]]; then
  echo "  reconciliation: every derived surface is declared, and every delivered claim exists in source."
  exit 0
fi

if [[ -n "$orphans" ]]; then
  echo "  ORPHANED SURFACES — reachable in source, declared by no spec:"
  printf '%s' "$orphans"
fi
if [[ -n "$undelivered" ]]; then
  echo "  UNDELIVERED CLAIMS — declared 'delivered' but absent from the derived inventory:"
  printf '%s' "$undelivered"
fi
echo "  These are findings, not a verdict: this guard is report-only while the"
echo "  derivation commands earn trust. Record each surface in the owning spec's"
echo "  Exposure Contract, or correct the claim."
exit 0
