#!/usr/bin/env bash
# state-consistency-scan.sh — read-only advisory scan for state.json bookkeeping drift.
#
# WHY THIS EXISTS (IMP-032 / EV-4, COV-4)
# --------------------------------------
# The framework gates OVER-claiming hard: state-transition-guard.sh Check-5-all-done
# refuses promotion while any scope is not Done. Nothing looked at the inverse.
#
# Two drift classes were invisible until an operator happened to run a guard by hand:
#
#   1. mirror-divergence — top-level `status` and `certification.status` disagree.
#      transition-contract-resolver.sh refuses such a spec with E009-TARGET-MISMATCH
#      BEFORE it reads the mode registry, so the spec is unresolvable and every later
#      guard run reports targetStatus UNRESOLVED. This is produced by advancing
#      `status` alone, which is easy to do because `certification.*` is
#      bubbles.validate-owned and a non-validate actor cannot legally write it.
#
#   2. status-behind-evidence — `status` is still `not_started` while the spec's own
#      scopes record Done work or its execution record lists completed phases.
#      Under-claiming asserts nothing false, so no anti-fabrication gate fires. It is
#      still a defect: work-pickers select by status, so the spec is silently skipped
#      or re-picked.
#
# REPORT-ONLY BY DESIGN — never blocking. Under-claiming is the truthful side of the
# ledger, and making it blocking would push agents toward premature promotion, which
# is the failure the anti-fabrication gates exist to prevent. This scan informs; it
# never forces a status write. It also never writes: repairing a divergence is
# bubbles.validate-owned (see skills/bubbles-status-transition/SKILL.md).
#
# Exit codes:
#   0 — scan completed (with or without findings)
#   2 — usage / environment error
#
# Usage:
#   bash bubbles/scripts/state-consistency-scan.sh [--repo-root <path>] [--quiet]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"
quiet=0

usage() {
  cat <<'EOF'
state-consistency-scan.sh — read-only advisory scan for state.json bookkeeping drift

Usage:
  bash bubbles/scripts/state-consistency-scan.sh [options]

Options:
  --repo-root <path>   Repo root to inspect (default: script repo root)
  --quiet              Suppress the OK sentinel; still prints findings
  -h, --help           Show this help

Reports two classes:
  mirror-divergence       status != certification.status (spec is unresolvable)
  status-behind-evidence  status=not_started but scopes/phases record real work

Exit: 0 scan completed - 2 usage/environment error. Never blocks.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "state-consistency-scan: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "state-consistency-scan: repo root not found: $repo_root" >&2
  exit 2
fi

specs_dir="$repo_root/specs"

# An absent specs/ directory is legitimate (the framework source repo has none).
if [[ ! -d "$specs_dir" ]]; then
  [[ "$quiet" -eq 1 ]] || echo "[state-consistency-scan] OK — no specs/ directory (nothing to scan)"
  exit 0
fi

# jq is optional here: this is an advisory surface, so a missing parser degrades to a
# clean skip rather than a hard failure that would break an otherwise-green doctor run.
if ! command -v jq >/dev/null 2>&1; then
  [[ "$quiet" -eq 1 ]] || echo "[state-consistency-scan] SKIP (jq not installed)"
  exit 0
fi

mirror_findings=0
underclaim_findings=0
scanned=0

# Bash 3.2 on macOS has no mapfile, so read the find stream directly.
while IFS= read -r state_file; do
  [[ -n "$state_file" ]] || continue
  feature_dir="$(dirname "$state_file")"
  rel="${feature_dir#"$repo_root"/}"

  # A malformed state.json is another guard's finding, not this scan's.
  jq -e . "$state_file" >/dev/null 2>&1 || continue
  scanned=$((scanned + 1))

  status="$(jq -r 'if (.status | type) == "string" then .status else "" end' "$state_file")"
  cert_status="$(jq -r 'if ((.certification // {}) | .status | type) == "string" then .certification.status else "" end' "$state_file")"
  [[ -n "$status" ]] || continue

  if [[ -n "$cert_status" && "$cert_status" != "$status" ]]; then
    printf 'FINDING: mirror-divergence: %s: status=%s certification.status=%s — spec is unresolvable (E009-TARGET-MISMATCH); route to bubbles.validate\n' \
      "$rel" "$status" "$cert_status"
    mirror_findings=$((mirror_findings + 1))
  fi

  [[ "$status" == "not_started" ]] || continue

  # A spec parked with a stated reason is a deliberate hold, not silent drift.
  blocked_reason="$(jq -r '.blockedReason // ""' "$state_file")"
  [[ -z "$blocked_reason" ]] || continue

  # `grep` exits 1 on no-match, which `pipefail` would turn into a scan abort.
  done_scopes="$({ find "$feature_dir" -maxdepth 3 \( -name 'scopes.md' -o -name 'scope.md' \) -type f -exec grep -hE '^\*\*Status:\*\*.*Done' {} + 2>/dev/null || true; } | wc -l | tr -d ' ')"
  done_scopes="${done_scopes:-0}"
  phase_claims="$(jq -r '((.execution // {}) .completedPhaseClaims // []) | length' "$state_file" 2>/dev/null || echo 0)"
  phase_claims="${phase_claims:-0}"

  if [[ "$done_scopes" -gt 0 || "$phase_claims" -gt 0 ]]; then
    printf 'FINDING: status-behind-evidence: %s: status=not_started but doneScopes=%s completedPhaseClaims=%s — work-pickers select by status, so this spec is skipped\n' \
      "$rel" "$done_scopes" "$phase_claims"
    underclaim_findings=$((underclaim_findings + 1))
  fi
done < <(find "$specs_dir" -name 'state.json' -type f 2>/dev/null | LC_ALL=C sort)

total=$((mirror_findings + underclaim_findings))

if [[ "$total" -gt 0 ]]; then
  printf '[state-consistency-scan] %s finding(s) across %s spec(s): %s mirror-divergence, %s status-behind-evidence — advisory, exit 0\n' \
    "$total" "$scanned" "$mirror_findings" "$underclaim_findings"
  exit 0
fi

[[ "$quiet" -eq 1 ]] || printf '[state-consistency-scan] OK — zero findings across %s spec(s)\n' "$scanned"
exit 0
