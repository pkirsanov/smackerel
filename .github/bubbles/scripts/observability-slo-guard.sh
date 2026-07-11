#!/usr/bin/env bash
set -euo pipefail

# observability-slo-guard.sh
#
# Gate G100 — observability_slo_evidence_gate.
#
# The TEETH of the observability posture model. When a repo declares
# `traceContracts.observability.posture: wired` AND an *instrumented scope* (a
# Test Plan row declaring `observabilityWorkflow: <workflow>`) targets a
# workflow that carries an `slo:` link, this guard asserts the CAPTURED SLO
# evidence MEETS the env-agnostic contract target:
#
#   observed.latencyP99Ms  <=  target.latencyP99Ms
#   observed.errorRatePct  <=  target.errorRatePct
#   observed.availabilityPct >= target.availabilityPct
#
# A metric the CONTRACT declares (target.<metric>) but the captured `observed`
# block OMITS is a BREACH, not a silent skip: a dropped measurement is an
# instrumentation regression and the SLO cannot be PROVEN met (R2-G adversarial
# hardening).
# Targets come from `traceContracts.observability.slos.<sloKey>` in
# `bubbles-project.yaml` (or `.github/bubbles-project.yaml`). Captured evidence
# is the parsed metric artifact at
# `.specify/runtime/observability/<workflow>.slo.json` (shape defined by
# SCOPE-1: workflow, slo, sampleWindow, source, target, observed). That file is
# an OUTPUT of the captured run; the run's PROVENANCE lives in
# `.specify/runtime/tool-calls.jsonl` (MCP record_evidence) — the two stores are
# non-duplicative (R2-F).
#
# BLOCKING (exit 1) ONLY when posture==wired AND an instrumented workflow with
# an slo: link is in scope. A NO-OP (exit 0) when:
#   * posture != wired (opted-out / undeclared / no config), OR
#   * no `observabilityWorkflow` is declared in any scope artifact under
#     `specs/` (no instrumented scope), OR
#   * no instrumented workflow carries an `slo:` link.
#
# Malformed evidence JSON, or evidence captured for the WRONG workflow, is
# rejected (exit 1, fail loud) BEFORE any numeric comparison.
#
# Parser dependency: `yq` (mikefarah v4) for YAML + `jq` for the evidence JSON.
# Unlike the WARN-level posture guards (G098/G099), G100 is a BLOCKING gate, so
# a MISSING parser FAILS CLOSED (exit 1) with an actionable "install jq/yq"
# message rather than silently passing — but ONLY for a repo that has opted
# into observability. A repo with no `bubbles-project.yaml`, or no
# `traceContracts.observability` block, no-ops (exit 0) via a cheap parser-free
# opt-in pre-check that runs BEFORE the fail-closed parser gate, so wiring G100
# into the universal done-gate (state-transition-guard) never blocks a
# non-adopter repo that merely lacks jq/yq.
#
# The Bubbles framework SOURCE checkout is auto-exempt: it has no runtime to
# monitor and keeps no persistent `specs/` (G085), so it resolves to EXEMPT and
# no-ops (exit 0). This mirrors `is_framework_repo()` in `bubbles/scripts/cli.sh`
# (≈ line 181), replicated here keyed to the SCANNED repo root so the same
# binary stays correct whether it scans its own tree or a fixture.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist and
# never will; if the captured SLO evidence breaches the contract, fix the
# instrumentation or the system — not the gate.
#
# ── Gate G100 refinement: spec-attribution scoping ───────────────────────
# `--spec-dir <dir>` scopes instrumentation ATTRIBUTION to one transitioning
# spec's OWN scope artifacts (its scopes.md / scopes/*/scope.md Test-Plan rows)
# instead of the repo-wide specs/ scan. This mirrors the Gate G090
# spec-attribution guard in retro-convergence-health.sh: when a SPECIFIC spec is
# being promoted, instrumentation declared by OTHER specs must not be attributed
# to it. state-transition-guard.sh passes the transitioning spec dir (its
# feature_dir — the same value it hands G090) so a spec that declares NO
# observabilityWorkflow is not an instrumented scope and the SLO-evidence gate is
# NOT APPLICABLE to it → no-op (exit 0), even when an UNRELATED spec declares an
# instrumented workflow whose gitignored/live-captured evidence is absent in a
# fresh checkout. A spec that DOES declare an instrumented-with-slo workflow
# keeps FULL teeth: its own missing/malformed/wrong-workflow/breaching evidence
# still blocks (exit 1). `--spec-dir` is SCOPING, not a bypass — the NO-bypass
# rule is intact. When --spec-dir is ABSENT/empty the repo-wide behavior is
# preserved unchanged, so standalone/CI still enforces every instrumented
# workflow across specs/.
#
# Exit codes:
#   0  no-op (EXEMPT / no config / posture!=wired / no instrumented-with-slo
#      workflow), OR every instrumented SLO is within target.
#   1  blocking: missing parser (fail closed), unsupported schemaVersion,
#      malformed project YAML, missing SLO evidence for an instrumented
#      workflow, malformed evidence JSON, evidence for the wrong workflow, OR
#      an SLO breach.
#   2  usage error (unknown flag / too many positionals).
#
# Usage:
#   bash bubbles/scripts/observability-slo-guard.sh [--repo-root <dir>] [--spec-dir <dir>] [--quiet]
#   bash bubbles/scripts/observability-slo-guard.sh <dir>            # positional repo root
#
# Reference: improvements/IMP-001-observability-first-class.md (SCOPE-4, T4.1)

SUPPORTED_SCHEMA_VERSION="1"
EVIDENCE_SIGNAL="slo"

# --- Resolve own location ------------------------------------------------
SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

QUIET="false"
REPO_ROOT_ARG=""
SPEC_DIR_ARG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/observability-slo-guard.sh [options] [<repo-root>]

Gate G100 — observability_slo_evidence_gate.
When traceContracts.observability.posture is `wired` and an instrumented scope
(a Test Plan row declaring `observabilityWorkflow`) targets a workflow with an
`slo:` link, assert the captured SLO evidence in
`.specify/runtime/observability/<workflow>.slo.json` MEETS the contract target
in `traceContracts.observability.slos.<sloKey>`.

Options:
  --repo-root <dir>  Repo root to scan (default: the repo this guard lives in).
  <repo-root>        Same as --repo-root, positional.
  --spec-dir <dir>   Scope instrumentation attribution to THIS spec's own scope
                     artifacts (Gate G100 refinement, mirrors G090). A spec that
                     declares no observabilityWorkflow no-ops; a spec declaring
                     an instrumented-with-slo workflow keeps full teeth. Absent
                     = repo-wide behavior (backward-compatible).
  --quiet            Suppress informational PASS/no-op stdout (warnings and
                     errors are still emitted).
  -h, --help         Print this usage and exit 0.

Exit codes:
  0 = no-op (EXEMPT / no config / posture!=wired / no instrumented-with-slo
      workflow) OR all instrumented SLOs within target
  1 = blocking: missing parser (fail closed) / unsupported schemaVersion /
      malformed YAML / missing evidence / malformed evidence / wrong-workflow
      evidence / SLO breach
  2 = usage error

This is a BLOCKING gate: a MISSING jq/yq parser FAILS CLOSED (exit 1), it does
NOT silently pass. There is NO --skip/--force/--ignore bypass.
EOF
}

# --- Argument parsing (builtins only) ------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --repo-root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "observability-slo-guard: --repo-root requires a directory argument" >&2
        usage >&2
        exit 2
      fi
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --spec-dir)
      shift
      if [[ $# -eq 0 ]]; then
        echo "observability-slo-guard: --spec-dir requires a directory argument" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_ARG="$1"
      shift
      ;;
    --*)
      echo "observability-slo-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$REPO_ROOT_ARG" ]]; then
        REPO_ROOT_ARG="$1"
      else
        echo "observability-slo-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

info() { [[ "$QUIET" == "true" ]] || echo "observability-slo-guard: $*"; }
warn() { echo "observability-slo-guard: $*" >&2; }
err()  { echo "observability-slo-guard: $*" >&2; }

# --- Repo-root resolution (builtins only) --------------------------------
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

# --- Framework SOURCE detection (builtins only) --------------------------
# Replicates the INTENT of is_framework_repo() from cli.sh (≈ line 181), keyed
# to the SCANNED root: the framework SOURCE checkout keeps scripts at
# <root>/bubbles/scripts and carries the canonical VERSION + install.sh, while a
# downstream install only ever carries <root>/.github/bubbles/scripts.
repo_is_framework_source() {
  local root="$1"
  [[ -f "$root/VERSION" \
     && -f "$root/install.sh" \
     && -d "$root/bubbles/scripts" \
     && ! -d "$root/.github/bubbles/scripts" ]]
}

# --- Config file location ------------------------------------------------
locate_config() {
  local root="$1"
  if [[ -f "$root/.github/bubbles-project.yaml" ]]; then
    printf '%s' "$root/.github/bubbles-project.yaml"
  elif [[ -f "$root/bubbles-project.yaml" ]]; then
    printf '%s' "$root/bubbles-project.yaml"
  else
    printf '%s' ''
  fi
}

# --- Instrumented-workflow detection -------------------------------------
# An instrumented workflow is one named by `observabilityWorkflow: <key>` in a
# scope artifact under the SCAN DIR (Markdown Test Plan rows OR test-plan.json).
# The scan dir is repo-wide (<repo>/specs) by default, OR — under the Gate G100
# spec-attribution refinement — one transitioning spec's own subtree (see
# --spec-dir / INSTRUMENTATION_SCAN_DIR). The source repo has no specs/ (G085) so
# this is empty there. The match is whole-key anchored so `booking.create` never
# matches `booking.created`.
workflow_is_instrumented() {
  local scan_dir="$1" wf="$2"
  [[ -d "$scan_dir" ]] || return 1
  local wf_re
  wf_re="$(printf '%s' "$wf" | sed -E 's/[][(){}.^$*+?|\\]/\\&/g')"
  grep -rhE "observabilityWorkflow[\"\`[:space:]:=]+${wf_re}([^A-Za-z0-9._-]|$)" \
    "$scan_dir" >/dev/null 2>&1
}

# --- Spec-attribution scoping helpers (Gate G100 refinement) -------------
# resolve_spec_scan_dir: resolve the transitioning spec's OWN artifact subtree.
# --spec-dir may be absolute, relative to the resolved repo root, or (fallback)
# relative to CWD — mirroring how state-transition-guard.sh hands its feature_dir
# to G090's retro-convergence-health.sh.
resolve_spec_scan_dir() {
  local sd="$1"
  sd="${sd#./}"
  sd="${sd%/}"
  if [[ "$sd" == /* ]]; then
    printf '%s' "$sd"
  elif [[ -d "$REPO_ROOT_RESOLVED/$sd" ]]; then
    printf '%s' "$REPO_ROOT_RESOLVED/$sd"
  elif [[ -d "$sd" ]]; then
    ( cd "$sd" 2>/dev/null && pwd ) || printf '%s' "$REPO_ROOT_RESOLVED/$sd"
  else
    printf '%s' "$REPO_ROOT_RESOLVED/$sd"
  fi
}

# spec_declares_any_observability_workflow: true when the spec's own scope
# artifacts declare ANY observabilityWorkflow (the spec IS an instrumented
# scope). A spec that declares none is not instrumented → G100 not applicable.
spec_declares_any_observability_workflow() {
  local scan_dir="$1"
  [[ -d "$scan_dir" ]] || return 1
  grep -rhE "observabilityWorkflow[\"\`[:space:]:=]+" "$scan_dir" >/dev/null 2>&1
}

# --- EXEMPT short-circuit (before parser check) --------------------------
if repo_is_framework_source "$REPO_ROOT_RESOLVED"; then
  info "Observability SLO gate: EXEMPT (no-runtime) — Bubbles framework source repo; nothing to monitor. (G100 OK)"
  exit 0
fi

# --- Spec-attribution scoping (Gate G100 refinement) ---------------------
# Default: repo-wide instrumentation scan (backward-compatible; --spec-dir
# absent). When state-transition-guard promotes a SPECIFIC spec it passes
# --spec-dir, scoping attribution to THAT spec's own scope artifacts (mirrors the
# G090 spec-attribution guard). A spec that declares NO observabilityWorkflow is
# not an instrumented scope, so the SLO-evidence gate is not applicable to it —
# no-op here (exit 0), BEFORE the adopter/parser gates, so an UNRELATED spec's
# absent live-captured evidence can never false-block this promotion. A spec that
# DOES declare instrumentation continues into the full flow with attribution
# scoped to its own subtree, keeping full teeth for its own workflows.
INSTRUMENTATION_SCAN_DIR="$REPO_ROOT_RESOLVED/specs"
if [[ -n "$SPEC_DIR_ARG" ]]; then
  SPEC_SCAN_DIR="$(resolve_spec_scan_dir "$SPEC_DIR_ARG")"
  if ! spec_declares_any_observability_workflow "$SPEC_SCAN_DIR"; then
    info "Observability SLO gate: no instrumented observabilityWorkflow attributed to ${SPEC_DIR_ARG} — SLO evidence gate not applicable to this spec; G100 no-op. (G100 OK)"
    exit 0
  fi
  INSTRUMENTATION_SCAN_DIR="$SPEC_SCAN_DIR"
fi

# --- Non-adopter opt-in pre-check (NO external tools needed) -------------
# A repo that never adopted observability MUST no-op even when jq/yq are
# absent — the fail-closed parser requirement applies ONLY to repos that HAVE
# opted in. This pre-check runs BEFORE the fail-closed parser gate so that
# wiring G100 into the universal done-gate (state-transition-guard) never
# blocks a non-adopter repo that simply lacks jq/yq. Determining posture==wired
# needs yq, so this resolves the chicken-and-egg with a parser-free scan:
# require a literal top-level/parent `traceContracts:` key followed by an
# indented child `observability:` key. It ignores blank/comment lines and
# unrelated keys such as `not_observability:` or `# observability:`. It is
# implemented with bash builtins ONLY (`while read`, `[[ =~ ]]`, and string
# length) so it stays correct even when PATH is fully stripped of grep/jq/yq.
CFG="$(locate_config "$REPO_ROOT_RESOLVED")"
if [[ -z "$CFG" ]]; then
  info "Observability SLO gate: no bubbles-project.yaml — posture undeclared; G100 no-op (G098 owns the posture nag). (G100 OK)"
  exit 0
fi
obs_optin="false"
if [[ -r "$CFG" ]]; then
  in_trace_contracts="false"
  trace_indent=-1
  while IFS= read -r _obs_line || [[ -n "$_obs_line" ]]; do
    # Ignore blank and full-line comment lines without requiring grep/sed/awk.
    if [[ "$_obs_line" =~ ^[[:space:]]*$ || "$_obs_line" =~ ^[[:space:]]*# ]]; then
      continue
    fi

    leading_spaces="${_obs_line%%[! ]*}"
    indent=${#leading_spaces}

    if [[ "$_obs_line" =~ ^[[:space:]]*traceContracts:[[:space:]]*($|#) ]]; then
      in_trace_contracts="true"
      trace_indent=$indent
      continue
    fi

    if [[ "$in_trace_contracts" == "true" && "$indent" -le "$trace_indent" ]]; then
      in_trace_contracts="false"
    fi

    if [[ "$in_trace_contracts" == "true" && "$indent" -gt "$trace_indent" && "$_obs_line" =~ ^[[:space:]]*observability:[[:space:]]*($|#) ]]; then
      obs_optin="true"
      break
    fi
  done < "$CFG"
fi
if [[ "$obs_optin" != "true" ]]; then
  info "Observability SLO gate: no traceContracts.observability block — posture undeclared; G100 no-op (G098 owns the posture nag). (G100 OK)"
  exit 0
fi

# --- Parser dependency: FAIL CLOSED on missing jq/yq (adopters only) -----
# Reached ONLY when the repo has opted into observability (config present with
# an `observability:` key). G100 is a BLOCKING gate: for a repo that committed
# to observability, a missing parser must NOT silently pass.
MISSING_PARSERS=()
command -v yq >/dev/null 2>&1 || MISSING_PARSERS+=("yq (mikefarah v4)")
command -v jq >/dev/null 2>&1 || MISSING_PARSERS+=("jq")
if [[ ${#MISSING_PARSERS[@]} -gt 0 ]]; then
  err "G100 (observability_slo_evidence_gate): required parser(s) missing: ${MISSING_PARSERS[*]}. This repo HAS opted into observability (traceContracts.observability present) and this is a BLOCKING gate that FAILS CLOSED — it will not silently pass. Install jq and yq (mikefarah v4) so the captured SLO evidence can be parsed and asserted."
  exit 1
fi

if ! yq '.' "$CFG" >/dev/null 2>&1; then
  err "G100 (observability_slo_evidence_gate): project config is not valid YAML; cannot resolve observability SLO contract. Fix the YAML."
  exit 1
fi

OBS_PRESENT="$(yq '.traceContracts.observability != null' "$CFG" 2>/dev/null || printf 'false')"
if [[ "$OBS_PRESENT" != "true" ]]; then
  info "Observability SLO gate: no traceContracts.observability block — posture undeclared; G100 no-op. (G100 OK)"
  exit 0
fi

POSTURE="$(yq -r '.traceContracts.observability.posture // ""' "$CFG" 2>/dev/null || printf '')"
if [[ "$POSTURE" != "wired" ]]; then
  info "Observability SLO gate: posture is '${POSTURE:-undeclared}' (not wired); G100 no-op. (G100 OK)"
  exit 0
fi

# Posture is wired — enforce schemaVersion BEFORE any SLO semantics (INV-13).
SCHEMA_VERSION="$(yq -r '.traceContracts.observability.schemaVersion // ""' "$CFG" 2>/dev/null || printf '')"
if [[ "$SCHEMA_VERSION" != "$SUPPORTED_SCHEMA_VERSION" ]]; then
  err "G100 (observability_slo_evidence_gate): unsupported traceContracts.observability.schemaVersion '${SCHEMA_VERSION:-<absent>}' (supported: ${SUPPORTED_SCHEMA_VERSION}). Failing loud BEFORE applying SLO semantics (INV-13)."
  exit 1
fi

# --- Resolve the workflows that carry an slo: link -----------------------
mapfile -t SLO_LINKED_WORKFLOWS < <(
  yq -r '.traceContracts.workflows // {} | to_entries | map(select(.value.slo != null)) | .[].key' "$CFG" 2>/dev/null || true
)

if [[ ${#SLO_LINKED_WORKFLOWS[@]} -eq 0 ]]; then
  info "Observability SLO gate: wired, but no traceContracts.workflows entry carries an slo: link; G100 no-op. (G100 OK)"
  exit 0
fi

EVIDENCE_DIR="$REPO_ROOT_RESOLVED/.specify/runtime/observability"
ENFORCED=0
BREACHES=()

for WF in "${SLO_LINKED_WORKFLOWS[@]}"; do
  # Only enforce workflows that are ACTUALLY instrumented by a scope. A wired
  # repo with an slo: link but no observabilityWorkflow declaration is NOT
  # blocked (the gate never infers workflow applicability from changed paths).
  # Attribution is scoped to INSTRUMENTATION_SCAN_DIR (repo-wide specs/ by
  # default; one spec's own subtree under --spec-dir per the G100 refinement).
  if ! workflow_is_instrumented "$INSTRUMENTATION_SCAN_DIR" "$WF"; then
    continue
  fi
  ENFORCED=$((ENFORCED + 1))

  SLO_KEY="$(W="$WF" yq -r '.traceContracts.workflows[strenv(W)].slo // ""' "$CFG" 2>/dev/null || printf '')"
  if [[ -z "$SLO_KEY" ]]; then
    # Defensive: should not happen (WF came from the slo-linked set).
    continue
  fi

  TARGET_JSON="$(S="$SLO_KEY" yq -o=json -I=0 '.traceContracts.observability.slos[strenv(S)] // {}' "$CFG" 2>/dev/null || printf '{}')"
  if [[ -z "$TARGET_JSON" || "$TARGET_JSON" == "null" || "$TARGET_JSON" == "{}" ]]; then
    err "G100: instrumented workflow '$WF' links slo '$SLO_KEY' but traceContracts.observability.slos['$SLO_KEY'] declares no target. Add latencyP99Ms/errorRatePct/availabilityPct to the slo contract."
    BREACHES+=("$WF: no target for slo '$SLO_KEY'")
    continue
  fi

  EVIDENCE_FILE="$EVIDENCE_DIR/${WF}.${EVIDENCE_SIGNAL}.json"

  # (1) Missing evidence for an instrumented workflow → BLOCK (the gap case).
  if [[ ! -f "$EVIDENCE_FILE" ]]; then
    err "G100: instrumented workflow '$WF' (slo '$SLO_KEY') has NO captured SLO evidence at ${EVIDENCE_FILE#"$REPO_ROOT_RESOLVED"/}. A wired+instrumented scope MUST capture SLO evidence under load (integration/e2e/stress) before it can be done."
    BREACHES+=("$WF: missing evidence file")
    continue
  fi

  # (2) Malformed evidence JSON → BLOCK (fail loud, before numeric compare).
  if ! jq -e 'type == "object"' "$EVIDENCE_FILE" >/dev/null 2>&1; then
    err "G100: SLO evidence for workflow '$WF' is malformed JSON (not a parseable object): ${EVIDENCE_FILE#"$REPO_ROOT_RESOLVED"/}. Rejecting before any numeric comparison."
    BREACHES+=("$WF: malformed evidence JSON")
    continue
  fi

  # (3) Required `observed` block present → else malformed → BLOCK (fail loud).
  if ! jq -e '.observed != null and (.observed | type == "object")' "$EVIDENCE_FILE" >/dev/null 2>&1; then
    err "G100: SLO evidence for workflow '$WF' is missing the required 'observed' block: ${EVIDENCE_FILE#"$REPO_ROOT_RESOLVED"/}. Rejecting as malformed before any numeric comparison."
    BREACHES+=("$WF: evidence missing 'observed'")
    continue
  fi

  # (4) Evidence captured for the WRONG workflow → BLOCK (fail loud).
  EV_WORKFLOW="$(jq -r '.workflow // ""' "$EVIDENCE_FILE" 2>/dev/null || printf '')"
  if [[ "$EV_WORKFLOW" != "$WF" ]]; then
    err "G100: SLO evidence at ${EVIDENCE_FILE#"$REPO_ROOT_RESOLVED"/} is for workflow '${EV_WORKFLOW:-<absent>}' but is being consumed for instrumented workflow '$WF'. Wrong-workflow evidence is rejected before any numeric comparison."
    BREACHES+=("$WF: evidence workflow mismatch ('${EV_WORKFLOW:-<absent>}')")
    continue
  fi

  # (5) Numeric assertion against the CONTRACT target (the authority, not the
  # evidence's self-reported target). jq does the float comparison and emits one
  # line per breached metric.
  #
  # ADVERSARIAL HARDENING (R2-G): a metric the CONTRACT declares but the captured
  # `observed` block OMITS is a BREACH, not a silent skip. A regression that
  # drops the p99/error-rate/availability measurement is an instrumentation
  # regression — the SLO cannot be PROVEN met, so the gate must fail. This is
  # what gives the "removed required attribute" adversarial case real teeth: the
  # check is only trusted if it fails when instrumentation regresses.
  WF_BREACHES="$(
    jq -r --argjson tgt "$TARGET_JSON" '
      .observed as $o
      | [ (if $tgt.latencyP99Ms != null then
              (if $o.latencyP99Ms == null
                 then "latencyP99Ms observed MISSING (target=\($tgt.latencyP99Ms); instrumentation regression — cannot prove SLO met)"
               elif $o.latencyP99Ms > $tgt.latencyP99Ms
                 then "latencyP99Ms observed=\($o.latencyP99Ms) > target=\($tgt.latencyP99Ms)"
               else empty end)
            else empty end),
          (if $tgt.errorRatePct != null then
              (if $o.errorRatePct == null
                 then "errorRatePct observed MISSING (target=\($tgt.errorRatePct); instrumentation regression — cannot prove SLO met)"
               elif $o.errorRatePct > $tgt.errorRatePct
                 then "errorRatePct observed=\($o.errorRatePct) > target=\($tgt.errorRatePct)"
               else empty end)
            else empty end),
          (if $tgt.availabilityPct != null then
              (if $o.availabilityPct == null
                 then "availabilityPct observed MISSING (target=\($tgt.availabilityPct); instrumentation regression — cannot prove SLO met)"
               elif $o.availabilityPct < $tgt.availabilityPct
                 then "availabilityPct observed=\($o.availabilityPct) < target=\($tgt.availabilityPct)"
               else empty end)
            else empty end)
        ]
      | .[]
    ' "$EVIDENCE_FILE" 2>/dev/null || printf ''
  )"

  if [[ -n "$WF_BREACHES" ]]; then
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      err "G100 SLO BREACH [$WF → $SLO_KEY]: $line"
      BREACHES+=("$WF: $line")
    done <<< "$WF_BREACHES"
  else
    info "Observability SLO gate: workflow '$WF' (slo '$SLO_KEY') — captured evidence within target. (G100 OK)"
  fi
done

# --- Verdict --------------------------------------------------------------
if [[ "$ENFORCED" -eq 0 ]]; then
  info "Observability SLO gate: wired, but no instrumented scope declares an observabilityWorkflow with an slo: link; G100 no-op. (G100 OK)"
  exit 0
fi

if [[ ${#BREACHES[@]} -gt 0 ]]; then
  err "G100 (observability_slo_evidence_gate): ${#BREACHES[@]} SLO evidence failure(s) across $ENFORCED instrumented workflow(s). Captured telemetry must MEET the traceContracts.observability.slos contract before a wired instrumented scope is done. There is NO bypass — fix the instrumentation or the system."
  exit 1
fi

info "Observability SLO gate: $ENFORCED instrumented workflow(s) — all captured SLO evidence within target. (G100 OK)"
exit 0
