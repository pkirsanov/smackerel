#!/usr/bin/env bash
# Risk-Tier Resolver (BFW-01 / IMP-021)
# ---------------------------------------------------------------------------
# Mechanical, FAIL-CLOSED eligibility resolver for the risk-tiered rapid delivery
# path (IMP-021 SCOPE-1). It classifies a requested change as `rapid-tool-delivery`
# ONLY when it shows a positive low-risk signal (build-free/static/isolated tool)
# AND carries NONE of the high-risk triggers (auth, payments, secrets, PII, DB
# migration, deployment/infra, production mutation, host-singleton, cross-product
# contract). Any high-risk trigger, or the absence of a positive low-risk signal,
# resolves to `full-delivery` — so a user can NEVER self-label risky work low-risk
# to shed gates, and unknown/ambiguous work gets the maximum-assurance chain.
#
# Reuse-first: this is the eligibility RESOLVER (IMP-021 SCOPE-1, extended for
# IMP-100 Phase 1). It does NOT add a budget mechanism (Gate G128 already provides
# aggregate session budgets) and does NOT itself register a mode. Exit 0 always
# (it is a resolver, not a gate). The decision is on stdout as four lines:
#   tier=<rapid-tool-delivery|full-delivery>   the mode to route to
#   riskClass=<low|high|unknown>               classification (unknown = fail-closed)
#   minimumAssurance=<fast|full>               the assurance FLOOR before cert/deploy
#   reason=<why>
# riskClass/minimumAssurance are re-evaluatable at intake / post-plan / pre-cert
# by re-running with the then-current surface + changed paths.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: risk-tier-resolve.sh [--surface <text>] [--changed-paths <newline-list>]

Prints:  tier=<rapid-tool-delivery|full-delivery>
         riskClass=<low|high|unknown>
         minimumAssurance=<fast|full>
         reason=<why>
Fail-closed: defaults to full-delivery (riskClass=unknown, minimumAssurance=full)
unless a positive low-risk signal is present AND no high-risk trigger is found.
EOF
}

surface=""
changed_paths=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --surface) surface="$2"; shift 2 ;;
    --changed-paths) changed_paths="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "risk-tier-resolve: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

haystack="$(printf '%s\n%s\n' "$surface" "$changed_paths")"

# High-risk trigger classes (any match → full-delivery). Text + path signals.
high_risk_re='(auth|login|oauth|jwt|\brbac\b|authoriz|permission|payment|billing|invoice|stripe|paypal|checkout|charge card|secret|credential|api[_ -]?key|password|private key|access token|\bpii\b|personal data|gdpr|social security|\bssn\b|migration|schema change|alter table|drop table|create table|deploy|kubernetes|\bk8s\b|terraform|helm|production|infrastructure|dockerfile|systemd|daemon|host port|singleton|proto|protobuf|api contract|cross-product|shared contract|breaking change)'
high_risk_path_re='(^|/)(migrations?|auth|secrets?|k8s|deploy|terraform|helm|\.env|Dockerfile)(/|$)|\.proto$'

# Positive low-risk signals (required to be eligible for rapid).
low_risk_re='(build-free|buildless|static site|static html|single[- ]file|self[- ]contained html|\.html\b|no backend|no server|isolated tool|frontend[- ]only|client[- ]only|docs?[- ]only|research[- ]lab tool|offline[- ]browsable)'

decide() {
  local tier="$1" risk_class="$2" min_assurance="$3" reason="$4"
  printf 'tier=%s\n' "$tier"
  printf 'riskClass=%s\n' "$risk_class"
  printf 'minimumAssurance=%s\n' "$min_assurance"
  printf 'reason=%s\n' "$reason"
  exit 0
}

# 1) Any high-risk trigger (text or path) escalates — cannot be self-labeled away.
if printf '%s' "$haystack" | grep -qiE "$high_risk_re"; then
  match="$(printf '%s' "$haystack" | grep -oiE "$high_risk_re" | head -n1)"
  decide "full-delivery" "high" "full" "high-risk trigger present (\"$match\") — maximum assurance required"
fi
if [[ -n "$changed_paths" ]] && printf '%s' "$changed_paths" | grep -qiE "$high_risk_path_re"; then
  match="$(printf '%s' "$changed_paths" | grep -iE "$high_risk_path_re" | head -n1)"
  decide "full-delivery" "high" "full" "high-risk changed path present (\"$match\") — maximum assurance required"
fi

# 2) No trigger AND a positive low-risk signal → eligible for the rapid path.
if printf '%s' "$haystack" | grep -qiE "$low_risk_re"; then
  decide "rapid-tool-delivery" "low" "fast" "positive low-risk signal and no high-risk trigger"
fi

# 3) Fail closed: no positive low-risk signal → full-delivery at full assurance.
decide "full-delivery" "unknown" "full" "no positive low-risk signal — fail-closed to maximum assurance"
