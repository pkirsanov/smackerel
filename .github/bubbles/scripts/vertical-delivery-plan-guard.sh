#!/usr/bin/env bash
# Vertical-Delivery Plan Guard (BFW-02 / IMP-022)
# ---------------------------------------------------------------------------
# Mechanizes the EXISTING `bubbles.plan` Phase-4 "Horizontal Plan Detection"
# behavioral rule (docs/guides/WORKFLOW_MODES.md § Horizontal Plan Detection):
# a plan whose first consumer-visible (usable) increment is deferred behind
# 3-or-more leading foundation-only scopes hides breakage until late and has no
# early runnable vertical slice. This does NOT invent a new planning concept —
# it makes the existing rule mechanically checkable (reuse-first).
#
# ADVISORY by default: it prints a warning and exits 0, so it never blocks an
# existing repo or a legitimate foundation-first plan. It exits non-zero ONLY
# when a project explicitly opts in via `.github/bubbles-project.yaml`
# (`verticalPlanGuard: block`), matching the advisory-until-configured posture of
# other coverage gates. There is no `--skip`/`--force` bypass.
#
# Classification is structural (does a scope reference a consumer-visible
# surface?), not keyword-counting: a scope is "consumer" when its body references
# a route/endpoint/UI/CLI/operator surface, else "foundation". Ambiguous scopes
# are treated as foundation (conservative — advisory only).
#
# It ALSO enforces a risk-adjusted scope BUDGET (IMP-022 SCOPE-2, bound to the
# Phase-1 tier): when the feature's state.json workflowMode is the low-risk
# `rapid-tool-delivery` fast lane, the plan must stay small (<= 5 active scopes /
# <= 3 increments) — the fast lane is for ONE usable increment, not a sprawling
# build. Every other mode is unbounded, exactly as before. Same advisory/block
# posture; conservative (unknown/absent mode is NOT treated as low-risk).
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: vertical-delivery-plan-guard.sh <feature-dir>

Flags a horizontal plan (>=3 leading foundation-only scopes before the first
consumer-visible increment) AND, for the low-risk rapid-tool-delivery tier, a
scope-budget breach (> 5 active scopes). Advisory (exit 0 + warning) by default;
blocks (exit 1) only when .github/bubbles-project.yaml sets verticalPlanGuard: block.
EOF
}

feature_dir="${1:-}"
if [[ -z "$feature_dir" ]]; then
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "vertical-delivery-plan-guard: feature dir not found: $feature_dir" >&2
  exit 2
fi

# ---------------------------------------------------------------------------
# Resolve enforcement mode (advisory | block). Advisory unless the repo opts in.
# ---------------------------------------------------------------------------
mode="advisory"
project_config=""
for candidate in \
  "$feature_dir/.github/bubbles-project.yaml" \
  ".github/bubbles-project.yaml" \
  "$(git -C "$feature_dir" rev-parse --show-toplevel 2>/dev/null)/.github/bubbles-project.yaml"; do
  if [[ -n "$candidate" && -f "$candidate" ]]; then
    project_config="$candidate"
    break
  fi
done
if [[ -n "$project_config" ]] && grep -qE '^[[:space:]]*verticalPlanGuard:[[:space:]]*block[[:space:]]*$' "$project_config"; then
  mode="block"
fi

# ---------------------------------------------------------------------------
# Collect ordered scope bodies. Supports both layouts:
#   1. single-file  scopes.md   (split on `^## Scope ` headers)
#   2. per-scope dir scopes/NN-*/scope.md  (ordered by directory name)
# ---------------------------------------------------------------------------
scope_bodies_dir="$(mktemp -d)"
trap 'rm -rf "$scope_bodies_dir"' EXIT INT TERM
scope_count=0

emit_scope() {
  local name="$1" body_file="$2"
  scope_count=$((scope_count + 1))
  printf '%s\n' "$name" >> "$scope_bodies_dir/names"
  cp "$body_file" "$scope_bodies_dir/scope-$scope_count.body"
}

if [[ -f "$feature_dir/scopes.md" ]]; then
  # Split scopes.md into per-scope bodies on `## Scope ` headers.
  current_body="$(mktemp)"
  current_name=""
  in_scope=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^##[[:space:]]+Scope[[:space:]] ]]; then
      if [[ "$in_scope" -eq 1 ]]; then
        emit_scope "$current_name" "$current_body"
      fi
      current_name="$(printf '%s' "$line" | sed -E 's/^##[[:space:]]+//')"
      : > "$current_body"
      in_scope=1
      continue
    fi
    if [[ "$in_scope" -eq 1 ]]; then
      printf '%s\n' "$line" >> "$current_body"
    fi
  done < "$feature_dir/scopes.md"
  if [[ "$in_scope" -eq 1 ]]; then
    emit_scope "$current_name" "$current_body"
  fi
  rm -f "$current_body"
elif [[ -d "$feature_dir/scopes" ]]; then
  while IFS= read -r scope_md; do
    [[ -n "$scope_md" ]] || continue
    emit_scope "$(basename "$(dirname "$scope_md")")" "$scope_md"
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -name 'scope.md' | LC_ALL=C sort)
fi

if [[ "$scope_count" -eq 0 ]]; then
  echo "[vertical-delivery-plan-guard] no scopes found in $feature_dir — nothing to check"
  exit 0
fi

# ---------------------------------------------------------------------------
# Classify each scope: consumer-visible surface vs foundation-only.
# Only STRONG, specific consumer signals count, so standard plan boilerplate
# ("Components/files", "service layer", "DB schema") never false-positives a
# foundation scope. Ambiguous → foundation (conservative; advisory only).
# ---------------------------------------------------------------------------
consumer_re='(/api/|GET /|POST /|PUT /|DELETE /|PATCH /|\.route\(|dashboard|frontend|web page|webpage|navigation|breadcrumb|deep link|WebSocket|CLI command|operator surface|user interface|admin portal)'

first_consumer=0
i=0
classification=""
while IFS= read -r name; do
  i=$((i + 1))
  body_file="$scope_bodies_dir/scope-$i.body"
  if grep -qiE "$consumer_re" "$body_file" 2>/dev/null; then
    classification="${classification}${i}:consumer:${name}"$'\n'
    if [[ "$first_consumer" -eq 0 ]]; then
      first_consumer=$i
    fi
  else
    classification="${classification}${i}:foundation:${name}"$'\n'
  fi
done < "$scope_bodies_dir/names"

# ---------------------------------------------------------------------------
# Horizontal detection: the first consumer-visible increment is deferred behind
# 3+ leading foundation-only scopes (or there is NO consumer scope in a
# multi-scope plan). Fewer than 3 leading foundation scopes = fine.
# ---------------------------------------------------------------------------
LEADING_FOUNDATION_THRESHOLD=3
verdict="ok"
if [[ "$first_consumer" -eq 0 ]]; then
  if [[ "$scope_count" -ge "$LEADING_FOUNDATION_THRESHOLD" ]]; then
    verdict="no-consumer"
  fi
elif [[ "$first_consumer" -gt "$LEADING_FOUNDATION_THRESHOLD" ]]; then
  verdict="deferred-consumer"
fi

# ---------------------------------------------------------------------------
# Risk-adjusted scope budget (IMP-022 SCOPE-2, bound to the Phase-1 tier).
# The rapid-tool-delivery fast lane is for ONE low-risk, build-free usable
# increment — not a sprawling multi-scope build. When state.json's workflowMode
# is that low-risk tier, cap active scopes at LOW_RISK_SCOPE_BUDGET. Every other
# mode stays unbounded (today's behavior). Conservative: an unknown/absent mode
# is NOT treated as low-risk, so no budget is imposed.
# ---------------------------------------------------------------------------
LOW_RISK_SCOPE_BUDGET=5
budget_verdict="ok"
state_file="$feature_dir/state.json"
if [[ -f "$state_file" ]]; then
  wf_mode="$(grep -oE '"workflowMode"[[:space:]]*:[[:space:]]*"[^"]*"' "$state_file" 2>/dev/null | head -n1 | sed -E 's/.*:[[:space:]]*"([^"]*)"$/\1/' || true)"
  if [[ "$wf_mode" == "rapid-tool-delivery" && "$scope_count" -gt "$LOW_RISK_SCOPE_BUDGET" ]]; then
    budget_verdict="over-budget"
  fi
fi

if [[ "$verdict" == "ok" && "$budget_verdict" == "ok" ]]; then
  if [[ "$first_consumer" -gt 0 ]]; then
    echo "[vertical-delivery-plan-guard] OK — first usable increment is early (scope $first_consumer of $scope_count); no horizontal chain; within scope budget."
  else
    echo "[vertical-delivery-plan-guard] OK — $scope_count scope(s), below the horizontal-chain threshold ($LEADING_FOUNDATION_THRESHOLD); within scope budget."
  fi
  exit 0
fi

# ---------------------------------------------------------------------------
# Emit the finding(s) + concrete remediation.
# ---------------------------------------------------------------------------
lead="$((first_consumer > 0 ? first_consumer - 1 : scope_count))"
{
  if [[ "$verdict" != "ok" ]]; then
    echo "[vertical-delivery-plan-guard] HORIZONTAL PLAN in $feature_dir:"
    if [[ "$verdict" == "no-consumer" ]]; then
      echo "  All $scope_count scopes are foundation-only — no scope delivers a consumer-visible (usable) increment."
    else
      echo "  Scopes 1..$lead are foundation-only; the first consumer-visible increment is scope $first_consumer of $scope_count."
    fi
    echo "  Remediation: restructure so an EARLY scope delivers a runnable vertical slice"
    echo "  (a consumer surface — route/UI/CLI/operator — plus its minimum backing path"
    echo "  and an end-to-end scenario), instead of stacking foundations first. See"
    echo "  docs/guides/WORKFLOW_MODES.md § Horizontal Plan Detection. A genuine"
    echo "  high-risk foundation-first rationale can opt out per scope."
  fi
  if [[ "$budget_verdict" == "over-budget" ]]; then
    echo "[vertical-delivery-plan-guard] SCOPE BUDGET (low-risk tier) in $feature_dir:"
    echo "  workflowMode=rapid-tool-delivery is the low-risk fast lane, but this plan has"
    echo "  $scope_count scopes (budget: <= $LOW_RISK_SCOPE_BUDGET active scopes / <= 3 increments)."
    echo "  Remediation: keep the fast lane to a single low-risk, build-free usable increment —"
    echo "  split the surplus scopes into a separate feature, or re-plan as full-delivery if the"
    echo "  work is genuinely large. risk-tier-resolve.sh escalates any high-risk trigger anyway."
  fi
} >&2

if [[ "$mode" == "block" ]]; then
  echo "[vertical-delivery-plan-guard] verticalPlanGuard: block — failing." >&2
  exit 1
fi
echo "[vertical-delivery-plan-guard] advisory (set verticalPlanGuard: block in .github/bubbles-project.yaml to enforce)." >&2
exit 0
