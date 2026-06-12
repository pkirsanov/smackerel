#!/usr/bin/env bash
set -uo pipefail

# observability-check.sh
#
# Canonical bash twin for the MCP `check_observability` tool (IMP-001 SCOPE-6,
# T6.5). It wraps the three observability guards and returns ONE JSON verdict:
#   * posture (G098) — observability-posture-guard.sh --print-state (+ enforce)
#   * SLO     (G100) — observability-slo-guard.sh        (report mode)
#   * trace   (G080) — trace-contract-guard.sh           (vs captured evidence)
# plus an `endpoints` block that reports WHICH adapter is wired per
# (plane, signal) by invoking observability-endpoint-resolve.sh --names-only
# (read-only wiring query; never materializes or requires plane secret env, so
# it preserves INV-12 — no live operate-plane fetch happens here). This makes
# the endpoint resolver a real executable consumer, not just agent-prompt prose.
#
# Per the v6 MCP design rule, THIS script is the canonical logic; the MCP
# server is only a thin wrapper around it. Running the bash twin directly is
# always equivalent to calling the `check_observability` tool.
#
# READ-ONLY. It runs the guards in report mode and never fetches, mutates, or
# captures live telemetry. The actual operate-plane telemetry CAPTURE performed
# by ops agents (bubbles.stabilize / bubbles.upkeep / bubbles.train) is logged
# for provenance via the MCP `record_evidence` tool -> tool-log.sh ->
# .specify/runtime/tool-calls.jsonl (R2-F). This verdict wrapper is not a
# capture and writes no provenance row, exactly like the read-only check_gate
# twin.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist.
#
# Output (stdout): one JSON object. Diagnostic notes go to stderr.
#   {
#     "tool": "check_observability",
#     "schemaVersion": 1,
#     "repoRoot": "<abs path>",
#     "posture": { "guard": "G098", "state": "<token>", "exitCode": N, "verdict": "ok|blocking" },
#     "slo":     { "guard": "G100", "exitCode": N, "verdict": "ok|blocking", "detail": "<text>" },
#     "trace":   { "guard": "G080", "exitCode": N, "verdict": "ok|blocking|no-op", "evidence": "<path>|null", "detail": "<text>" },
#     "overall": "ok|blocking"
#   }
#
# Exit codes:
#   0  check completed; nothing blocking (posture ok/exempt/undeclared-warn, SLO
#      within target or no-op, trace satisfied or no-op).
#   1  check completed but a wrapped guard reported a BLOCKING verdict
#      (fake-wired / opted-out-malformed / unsupported-schema / invalid posture,
#      SLO breach / malformed evidence / missing parser fail-closed, or a
#      trace-contract violation). The JSON body carries the full per-guard
#      detail regardless.
#   2  usage error (unknown flag).
#
# Reference: improvements/IMP-001-observability-first-class.md (SCOPE-6, T6.5)

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

REPO_ROOT_ARG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/observability-check.sh [--repo-root <dir>] [<repo-root>]

Return the repo's observability posture (G098) plus the most recent SLO-evidence
verdict (G100) and trace-contract verdict (G080) as a single JSON object. The
canonical bash twin behind the MCP `check_observability` tool.

Options:
  --repo-root <dir>  Repo root to inspect (default: the repo this twin lives in).
  <repo-root>        Same as --repo-root, positional.
  -h, --help         Print this usage and exit 0.

Read-only: runs the three observability guards in report mode; never fetches,
mutates, or captures live telemetry. There is NO --skip/--force/--ignore bypass.

Exit codes:
  0 = nothing blocking (ok / exempt / no-op)
  1 = a wrapped guard reported a blocking verdict (detail in the JSON body)
  2 = usage error
EOF
}

# --- Argument parsing (builtins only) ------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || { echo "observability-check: --repo-root requires a directory argument" >&2; usage >&2; exit 2; }
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --*)
      echo "observability-check: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$REPO_ROOT_ARG" ]]; then
        REPO_ROOT_ARG="$1"
        shift
      else
        echo "observability-check: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      ;;
  esac
done

POSTURE_GUARD="$SCRIPT_DIR/observability-posture-guard.sh"
SLO_GUARD="$SCRIPT_DIR/observability-slo-guard.sh"
TRACE_GUARD="$SCRIPT_DIR/trace-contract-guard.sh"

# --- Repo-root resolution (mirrors the sibling guards) -------------------
resolve_repo_root() {
  if [[ -n "$REPO_ROOT_ARG" ]]; then
    ( cd "$REPO_ROOT_ARG" 2>/dev/null && pwd ) || printf '%s' "$REPO_ROOT_ARG"
    return 0
  fi
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  if [[ "$SCRIPT_DIR" == */.github/bubbles/scripts ]]; then
    ( cd "$SCRIPT_DIR/../../.." 2>/dev/null && pwd )
  else
    ( cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd )
  fi
}

REPO_ROOT_RESOLVED="$(resolve_repo_root)"

# Passthrough --repo-root only when one was supplied; otherwise let each guard
# auto-detect identically to this twin.
repo_args=()
[[ -n "$REPO_ROOT_ARG" ]] && repo_args=(--repo-root "$REPO_ROOT_ARG")

# --- JSON helpers (no jq dependency for envelope assembly) ---------------
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

sanitize_detail() {
  local s="$1"
  s="${s//$'\n'/ }"
  s="${s//$'\r'/ }"
  s="${s//$'\t'/ }"
  json_escape "$s"
}

verdict_for() { # exit_code -> ok|blocking
  [[ "$1" -eq 0 ]] && printf 'ok' || printf 'blocking'
}

# --- posture (G098) ------------------------------------------------------
# --print-state is read-only and always exits 0; the enforcing run gives the
# health exit code (undeclared+warn stays exit 0, fake-wired/malformed = 1).
posture_state="$(bash "$POSTURE_GUARD" --print-state "${repo_args[@]}" 2>/dev/null)" || posture_state=""
[[ -n "$posture_state" ]] || posture_state="UNAVAILABLE"
bash "$POSTURE_GUARD" --quiet "${repo_args[@]}" >/dev/null 2>&1
posture_exit=$?

# --- slo (G100) ----------------------------------------------------------
slo_out="$(bash "$SLO_GUARD" --quiet "${repo_args[@]}" 2>&1)"
slo_exit=$?
slo_detail="$(printf '%s' "$slo_out" | tr '\n' ' ')"
slo_detail="${slo_detail#"${slo_detail%%[![:space:]]*}"}"
slo_detail="${slo_detail%"${slo_detail##*[![:space:]]}"}"
[[ -n "$slo_detail" ]] || slo_detail="no SLO breach (within target or no-op)"

# --- trace (G080) --------------------------------------------------------
# The trace guard requires captured evidence; with none present the verdict is
# a clean no-op (nothing to validate), never a usage error.
trace_evidence=""
obs_dir="$REPO_ROOT_RESOLVED/.specify/runtime/observability"
if [[ -d "$obs_dir" ]]; then
  trace_evidence="$(find "$obs_dir" -maxdepth 1 -type f \( -name '*.trace.json' -o -name '*.trace.txt' \) 2>/dev/null | LC_ALL=C sort | tail -1)"
fi

if [[ -n "$trace_evidence" ]]; then
  trace_base="$(basename "$trace_evidence")"
  trace_workflow="${trace_base%.trace.*}"
  trace_out="$(bash "$TRACE_GUARD" --trace-output "$trace_evidence" --workflow "$trace_workflow" "${repo_args[@]}" 2>&1)"
  trace_exit=$?
  trace_verdict="$(verdict_for "$trace_exit")"
  trace_detail="$(printf '%s' "$trace_out" | tr '\n' ' ')"
  trace_detail="${trace_detail#"${trace_detail%%[![:space:]]*}"}"
  trace_detail="${trace_detail%"${trace_detail##*[![:space:]]}"}"
  [[ -n "$trace_detail" ]] || trace_detail="trace evidence validated"
  trace_evidence_json="\"$(json_escape "$trace_evidence")\""
else
  trace_exit=0
  trace_verdict="no-op"
  trace_detail="no trace evidence captured under .specify/runtime/observability/"
  trace_evidence_json="null"
fi

posture_verdict="$(verdict_for "$posture_exit")"
slo_verdict="$(verdict_for "$slo_exit")"

# --- endpoints (resolver consumer) ---------------------------------------
# Make observability-endpoint-resolve.sh a REAL executable consumer (not just
# agent-prompt prose): for each (plane, signal) report WHICH adapter is wired,
# read-only, via the resolver's --names-only mode. This never materializes or
# requires plane-scoped secret env (so it is safe in a health-check context and
# preserves INV-12 — no live operate-plane fetch happens here, only a config
# read of the wired adapter name). The resolver auto-no-ops to `none` when the
# repo has no observability config, so this is silent for non-adopters.
RESOLVER="$SCRIPT_DIR/observability-endpoint-resolve.sh"
resolve_adapter() { # $1=plane $2=signal -> adapter name (or "none"/"unknown")
  local plane="$1" signal="$2" out adapter
  if [[ ! -x "$RESOLVER" ]]; then printf 'unknown'; return 0; fi
  out="$(bash "$RESOLVER" --plane "$plane" --signal "$signal" --names-only "${repo_args[@]}" 2>/dev/null)" || { printf 'unknown'; return 0; }
  adapter="$(printf '%s\n' "$out" | sed -n 's/^adapter=//p' | head -1)"
  [[ -n "$adapter" ]] || adapter="none"
  printf '%s' "$adapter"
}

endpoints_json=""
for _plane in validate operate; do
  plane_pairs=""
  for _signal in alerts sloBurn errorRate deployImpact; do
    _adapter="$(resolve_adapter "$_plane" "$_signal")"
    plane_pairs="${plane_pairs}      \"${_signal}\": \"$(json_escape "$_adapter")\",\n"
  done
  # strip trailing comma+newline of the last pair
  plane_pairs="${plane_pairs%,\\n}"
  endpoints_json="${endpoints_json}    \"${_plane}\": {\n${plane_pairs}\n    },\n"
done
endpoints_json="${endpoints_json%,\\n}"

overall_exit=0
if [[ "$posture_exit" -ne 0 || "$slo_exit" -ne 0 || "$trace_exit" -ne 0 ]]; then
  overall_exit=1
fi
overall_verdict="$(verdict_for "$overall_exit")"

cat <<JSON
{
  "tool": "check_observability",
  "schemaVersion": 1,
  "repoRoot": "$(json_escape "$REPO_ROOT_RESOLVED")",
  "posture": {
    "guard": "G098",
    "state": "$(json_escape "$posture_state")",
    "exitCode": $posture_exit,
    "verdict": "$posture_verdict"
  },
  "slo": {
    "guard": "G100",
    "exitCode": $slo_exit,
    "verdict": "$slo_verdict",
    "detail": "$(sanitize_detail "$slo_detail")"
  },
  "trace": {
    "guard": "G080",
    "exitCode": $trace_exit,
    "verdict": "$trace_verdict",
    "evidence": $trace_evidence_json,
    "detail": "$(sanitize_detail "$trace_detail")"
  },
  "endpoints": {
$(printf '%b' "$endpoints_json")
  },
  "overall": "$overall_verdict"
}
JSON

exit "$overall_exit"
