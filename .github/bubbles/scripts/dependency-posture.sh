#!/usr/bin/env bash
#
# dependency-posture.sh — shared fail-closed dependency guard
# (IMP-027 / SCOPE-4, SEC-2).
#
# WHY THIS EXISTS
# ---------------
# README.md claimed "No dependencies beyond curl and bash 4.0+". In practice ten
# non-selftest guards silently `exit 0` when a dependency is absent:
#
#   result-envelope-validate.sh   python3, jsonschema
#   yaml-schema-validate.sh       python3, PyYAML, jsonschema
#   diff-evidence-guard.sh        git, python3
#   evidence-tool-log-bridge.sh   python3   (the receipt bridge itself)
#   generate-gate-coverage-map.sh python3, PyYAML
#   generate-gates-block.sh       python3
#   mode-family-inventory.sh      python3
#   model-tier-advisory.sh        python3
#   parallel-fanout.sh            python3
#
# `jsonschema` and `PyYAML` are not stdlib and were not declared install
# requirements. A guard that skips is a guard that lies: the run goes green and
# the operator is told the check passed when it never executed.
#
# CONTRACT
#   bubbles_require_dep <label> <probe-description>
#     Call after a dependency probe fails. Exits 1 (fail closed) unless
#     BUBBLES_ALLOW_DEGRADED=1, in which case it emits a DEGRADED line and
#     returns non-zero so the caller can still take its skip path.
#
# Selftests may legitimately SKIP — they are hermetic and prove behavior, not
# posture. GUARDS may not: their whole purpose is to be the thing that fails.
#
# The opt-out is deliberately a single env var with a loud, greppable line so
# `cli.sh doctor` can report degraded posture rather than the operator
# discovering it from a green run that checked nothing.

# Resolve a satisfying interpreter BEFORE any probe runs. python-env.sh owns the
# managed virtualenv; activating it here means the scripts that source this
# module — and every bare `python3` they invoke — get the provisioned
# interpreter with no call-site change. Absent or unprovisioned, this is a
# no-op and the probes below correctly report MISSING.
_bubbles_posture_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$_bubbles_posture_dir/python-env.sh" ]]; then
  # shellcheck source=/dev/null
  . "$_bubbles_posture_dir/python-env.sh"
  bubbles_python_activate >/dev/null 2>&1 || true
fi
unset _bubbles_posture_dir

# bubbles_require_dep <script-label> <what-is-missing>
bubbles_require_dep() {
  local label="$1"
  local missing="$2"

  if [[ "${BUBBLES_ALLOW_DEGRADED:-0}" == "1" ]]; then
    echo "DEGRADED: $label — $missing (BUBBLES_ALLOW_DEGRADED=1; this check did NOT run)" >&2
    return 1
  fi

  echo "$label: FAIL — $missing" >&2
  echo "  This guard cannot verify anything without it, and reporting success" >&2
  echo "  for a check that never ran is the failure mode this refuses to have." >&2
  echo "  Provision the managed environment:" >&2
  echo "      bash bubbles/scripts/python-env.sh --provision" >&2
  echo "  Inspect posture with '--check'. Do NOT reach for a one-off" >&2
  echo "  'pip install --break-system-packages' or a /tmp virtualenv: both are" >&2
  echo "  undone by the next PATH change and leave no reproducible record." >&2
  echo "  Or set BUBBLES_ALLOW_DEGRADED=1 to proceed with an explicitly" >&2
  echo "  degraded posture that 'cli.sh doctor' will report." >&2
  exit 1
}

# The dependency set Bubbles actually requires, for doctor and README to share.
# Format: <probe-kind>:<name>:<why>
BUBBLES_REQUIRED_DEPS=(
  "command:bash:shell (>= 4.0)"
  "command:git:diff-evidence and drift surfaces"
  "command:python3:schema validation, generators, receipt bridge"
  "python-module:yaml:PyYAML — registry and workflow parsing"
  "python-module:jsonschema:envelope and schema validation"
)

# bubbles_dep_status — print one line per required dependency: "<name> <ok|MISSING> <why>"
bubbles_dep_status() {
  local entry kind name why
  for entry in "${BUBBLES_REQUIRED_DEPS[@]}"; do
    kind="${entry%%:*}"
    why="${entry#*:*:}"
    name="${entry#*:}"
    name="${name%%:*}"
    case "$kind" in
      command)
        if command -v "$name" >/dev/null 2>&1; then
          echo "$name ok $why"
        else
          echo "$name MISSING $why"
        fi
        ;;
      python-module)
        if command -v python3 >/dev/null 2>&1 && python3 -c "import $name" >/dev/null 2>&1; then
          echo "$name ok $why"
        else
          echo "$name MISSING $why"
        fi
        ;;
    esac
  done
}
