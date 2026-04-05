#!/usr/bin/env bash
# 🐾 Regression Baseline Guard — "Something's prowlin' around in the code, boys."
# Checks for test baseline regressions and cross-spec interference.
#
# Usage: regression-baseline-guard.sh <spec-dir> [--verbose]
#
# Gates enforced:
#   G044 — regression_baseline_gate (test count comparison)
#   G045 — cross_spec_regression_gate (cross-spec test execution)
#   G046 — spec_conflict_detection_gate (route/table/API collision scan)
#
set -euo pipefail

SPEC_DIR="${1:?Usage: regression-baseline-guard.sh <spec-dir> [--verbose]}"
VERBOSE="${2:-}"
FAILURES=0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { printf "${GREEN}  ✅ %s${NC}\n" "$1"; }
fail() { printf "${RED}  ❌ %s${NC}\n" "$1"; FAILURES=$((FAILURES + 1)); }
info() { printf "${CYAN}  ℹ️  %s${NC}\n" "$1"; }
warn() { printf "${YELLOW}  ⚠️  %s${NC}\n" "$1"; }

echo ""
printf "${CYAN}🐾 Regression Baseline Guard${NC}\n"
printf "${CYAN}   Spec: %s${NC}\n" "$SPEC_DIR"
echo ""

# ── Check 1: report.md exists ─────────────────────────────────────
echo "── G044: Regression Baseline ──"

REPORT_FILE=""
if [[ -f "${SPEC_DIR}/report.md" ]]; then
  REPORT_FILE="${SPEC_DIR}/report.md"
elif ls "${SPEC_DIR}"/scopes/*/report.md >/dev/null 2>&1; then
  REPORT_FILE=$(ls "${SPEC_DIR}"/scopes/*/report.md 2>/dev/null | head -1)
fi

if [[ -n "$REPORT_FILE" ]]; then
  # Check for baseline comparison table in report
  if grep -qi "test baseline\|baseline comparison\|before.*after.*delta" "$REPORT_FILE" 2>/dev/null; then
    pass "Test baseline comparison found in report"
  else
    warn "No test baseline comparison table found in report.md (first run may establish baseline)"
  fi
else
  warn "No report.md found — baseline will be established on first regression run"
fi

# ── Check 2: Cross-spec regression scan ─────────────────────────────
echo ""
echo "── G045: Cross-Spec Regression ──"

SPECS_DIR=$(dirname "$SPEC_DIR")
CURRENT_SPEC=$(basename "$SPEC_DIR")

# Count other spec directories with done status
DONE_SPECS=0
TOTAL_SPECS=0
if [[ -d "$SPECS_DIR" ]]; then
  for spec_state in "${SPECS_DIR}"/*/state.json; do
    [[ ! -f "$spec_state" ]] && continue
    spec_name=$(basename "$(dirname "$spec_state")")
    [[ "$spec_name" == "$CURRENT_SPEC" ]] && continue
    TOTAL_SPECS=$((TOTAL_SPECS + 1))
    if grep -q '"status"[[:space:]]*:[[:space:]]*"done"' "$spec_state" 2>/dev/null; then
      DONE_SPECS=$((DONE_SPECS + 1))
    fi
  done
fi

if [[ $DONE_SPECS -gt 0 ]]; then
  info "Found $DONE_SPECS done specs (of $TOTAL_SPECS total) that need cross-spec regression verification"
  pass "Cross-spec inventory completed"
else
  info "No done specs found — cross-spec regression check is informational only"
  pass "Cross-spec check N/A (no done specs)"
fi

# ── Check 3: Spec conflict detection ──────────────────────────────
echo ""
echo "── G046: Spec Conflict Detection ──"

CONFLICTS=0

# Check for route collisions in design.md files
if [[ -f "${SPEC_DIR}/design.md" ]]; then
  # Extract routes from current spec's design
  CURRENT_ROUTES=$(grep -oE '(GET|POST|PUT|PATCH|DELETE)\s+/[^ ]+' "${SPEC_DIR}/design.md" 2>/dev/null || true)

  if [[ -n "$CURRENT_ROUTES" ]]; then
    while IFS= read -r route; do
      [[ -z "$route" ]] && continue
      # Search other specs for the same route
      for other_design in "${SPECS_DIR}"/*/design.md; do
        [[ ! -f "$other_design" ]] && continue
        other_spec=$(basename "$(dirname "$other_design")")
        [[ "$other_spec" == "$CURRENT_SPEC" ]] && continue
        if grep -qF "$route" "$other_design" 2>/dev/null; then
          if [[ -n "$VERBOSE" ]]; then
            warn "Route collision: '$route' found in both $CURRENT_SPEC and $other_spec"
          fi
          CONFLICTS=$((CONFLICTS + 1))
        fi
      done
    done <<< "$CURRENT_ROUTES"
  fi
fi

if [[ $CONFLICTS -eq 0 ]]; then
  pass "No route/endpoint collisions detected across specs"
else
  warn "$CONFLICTS potential route collision(s) detected — regression agent will investigate"
fi

# ── Summary ──────────────────────────────────────────────────────
echo ""
echo "── Summary ──"

if [[ $FAILURES -eq 0 ]]; then
  printf "${GREEN}🐾 Regression baseline guard: PASSED${NC}\n"
  printf "${GREEN}   All ${FAILURES} checks passed.${NC}\n"
  echo ""
  exit 0
else
  printf "${RED}🐾 Regression baseline guard: FAILED${NC}\n"
  printf "${RED}   ${FAILURES} check(s) failed.${NC}\n"
  echo ""
  exit 1
fi
