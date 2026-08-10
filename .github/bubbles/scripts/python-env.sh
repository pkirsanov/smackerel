#!/usr/bin/env bash
# python-env.sh — managed Python interpreter resolution and dependency
# provisioning for Bubbles.
#
# WHY THIS EXISTS
# ---------------
# dependency-posture.sh declares python3 + PyYAML + jsonschema REQUIRED and
# refuses to let a guard skip silently ("a guard that skips is a guard that
# lies"). But the framework shipped no supported way to OBTAIN those modules, so
# the declaration had no reachable remedy. Observed consequences on a real
# operator machine:
#
#   * `pip install --user --break-system-packages` — forces past PEP 668 on a
#     Homebrew interpreter, and lands in whichever python3 PATH resolved that
#     day.
#   * a virtualenv under /tmp, referenced by nothing in the repo, wiped on
#     reboot.
#
# Both are shortcuts, and both are silently undone by the next PATH change. A
# machine can carry several python3 installs (Homebrew, python.org, Xcode,
# conda); `command -v python3` is therefore NOT a stable identity, and a
# successful install can stop applying without anything being edited.
#
# THE CONTRACT
#   The managed virtualenv owns its own interpreter, so once provisioned it
#   keeps satisfying regardless of later PATH or conda changes. Resolution is
#   explicit and ordered, never ambient:
#
#     1. $BUBBLES_PYTHON            — operator override, honored only if it satisfies
#     2. the managed venv           — bubbles_python_home()
#     3. python3 from PATH          — honored only if it already satisfies
#
#   Nothing is auto-installed. Provisioning is an explicit operator act:
#     bash bubbles/scripts/python-env.sh --provision
#
# SOURCEABLE. Sourcing defines functions and changes nothing else — it does NOT
# set shell options, because callers source this from scripts with their own
# `set` posture.
#
# Usage (executed):
#   python-env.sh --check       posture report; exit 0 satisfied, 1 not
#   python-env.sh --provision   create/repair the managed venv from requirements.txt
#   python-env.sh --path        print the resolved interpreter; exit 1 if none
#   python-env.sh --help
#
# Exit codes: 0 ok · 1 unsatisfied · 2 usage or provisioning error.

# The python modules Bubbles requires. Kept in lockstep with the
# `python-module:` entries of BUBBLES_REQUIRED_DEPS in dependency-posture.sh;
# python-env-selftest.sh asserts the two agree, so the duplication cannot drift.
BUBBLES_PYTHON_MODULES=(yaml jsonschema)

# bubbles_python_home — durable root of the managed virtualenv.
# NOT under /tmp: a temp-dir venv is erased on reboot, which is exactly the
# shortcut this module replaces.
bubbles_python_home() {
  if [[ -n "${BUBBLES_PYTHON_HOME:-}" ]]; then
    printf '%s\n' "${BUBBLES_PYTHON_HOME%/}"
    return 0
  fi
  printf '%s\n' "${XDG_CACHE_HOME:-$HOME/.cache}/bubbles/python"
}

bubbles_python_venv_python() {
  printf '%s\n' "$(bubbles_python_home)/bin/python3"
}

# bubbles_python_satisfies <interpreter> — true when it imports every required module.
bubbles_python_satisfies() {
  local py="${1:-}" module
  [[ -n "$py" && -x "$py" ]] || return 1
  for module in "${BUBBLES_PYTHON_MODULES[@]}"; do
    "$py" -c "import $module" >/dev/null 2>&1 || return 1
  done
  return 0
}

# bubbles_python_resolve — print the first interpreter that satisfies, in the
# documented order. Prints nothing and returns 1 when none does.
bubbles_python_resolve() {
  local candidate
  if [[ -n "${BUBBLES_PYTHON:-}" ]] && bubbles_python_satisfies "$BUBBLES_PYTHON"; then
    printf '%s\n' "$BUBBLES_PYTHON"
    return 0
  fi
  candidate="$(bubbles_python_venv_python)"
  if bubbles_python_satisfies "$candidate"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  candidate="$(command -v python3 2>/dev/null || true)"
  if bubbles_python_satisfies "$candidate"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  return 1
}

# bubbles_python_activate — make a bare `python3` call resolve to a satisfying
# interpreter, so the ~77 scripts that invoke `python3` directly need no edit.
# Only prepends when PATH's python3 does NOT already satisfy, so an operator who
# provisioned deps their own way keeps their interpreter. Idempotent.
bubbles_python_activate() {
  local venv_python bin_dir path_python
  venv_python="$(bubbles_python_venv_python)"
  bubbles_python_satisfies "$venv_python" || return 1
  path_python="$(command -v python3 2>/dev/null || true)"
  bubbles_python_satisfies "$path_python" && return 0
  bin_dir="${venv_python%/python3}"
  case ":${PATH:-}:" in
    *":$bin_dir:"*) ;;
    *)
      PATH="$bin_dir:${PATH:-}"
      export PATH
      ;;
  esac
  return 0
}

# bubbles_python_base — an interpreter able to CREATE the venv. Distinct from
# bubbles_python_resolve: the base need not have the modules yet.
bubbles_python_base() {
  local candidate resolved
  for candidate in "${BUBBLES_PYTHON_BASE:-}" python3 python3.13 python3.12 python3.11; do
    [[ -n "$candidate" ]] || continue
    resolved="$(command -v "$candidate" 2>/dev/null || true)"
    [[ -n "$resolved" ]] || continue
    if "$resolved" -c 'import sys, venv; sys.exit(0 if sys.version_info[:2] >= (3, 9) else 1)' >/dev/null 2>&1; then
      printf '%s\n' "$resolved"
      return 0
    fi
  done
  return 1
}

bubbles_python_requirements_default() {
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  printf '%s\n' "$here/../requirements.txt"
}

# bubbles_python_provision <requirements-file> — idempotent create-or-repair.
bubbles_python_provision() {
  local requirements="${1:-}" venv_dir venv_python base
  venv_dir="$(bubbles_python_home)"
  venv_python="$venv_dir/bin/python3"

  if [[ ! -f "$requirements" ]]; then
    echo "python-env: requirements file not found: $requirements" >&2
    return 2
  fi

  # A half-built or interpreter-less venv is repaired, not worked around.
  if [[ ! -x "$venv_python" ]]; then
    if ! base="$(bubbles_python_base)"; then
      echo "python-env: no python3 (>= 3.9, with venv) available to build the managed environment" >&2
      echo "  set BUBBLES_PYTHON_BASE to an interpreter, or install python3." >&2
      return 2
    fi
    echo "python-env: creating managed environment at $venv_dir (base: $base)"
    rm -rf "$venv_dir"
    if ! "$base" -m venv "$venv_dir"; then
      echo "python-env: failed to create the virtualenv at $venv_dir" >&2
      return 2
    fi
  else
    echo "python-env: reusing managed environment at $venv_dir"
  fi

  # Record the toolchain identity on EVERY run, pass or fail. A provisioning
  # failure that reports only "it failed" leaves the next reader guessing, and
  # WHICH interpreter ran — on WHICH platform — is the first fact any diagnosis
  # needs. Asked of the interpreter itself, never inferred from the host.
  local venv_identity pip_status=0
  venv_identity="$("$venv_python" -c 'import sys, platform; print(sys.version.split()[0], platform.system() + "/" + platform.machine())' 2>/dev/null)" ||
    venv_identity="unreported (the interpreter did not answer)"

  echo "python-env: interpreter $venv_python ($venv_identity)"
  echo "python-env: installing pinned requirements from $requirements"
  "$venv_python" -m pip install \
    --disable-pip-version-check \
    --no-input \
    --requirement "$requirements" || pip_status=$?
  if [[ "$pip_status" -ne 0 ]]; then
    # Report what was OBSERVED. The previous message asserted a cause
    # ("network unavailable, or a pin no longer resolves") that nothing here
    # measured, which is how a CI leg goes dark: the log named a diagnosis
    # instead of the data needed to reach one.
    {
      echo "python-env: dependency installation FAILED (pip rc=$pip_status)"
      echo "  interpreter  : $venv_python"
      echo "  python       : $venv_identity"
      echo "  requirements : $requirements"
      echo "  The cause is NOT determined here; pip's own output above is the record."
      echo "  Common causes: no route to the package index, an index that requires"
      echo "  auth, or a pin with no distribution for this python/platform."
    } >&2
    return 2
  fi

  if ! bubbles_python_satisfies "$venv_python"; then
    echo "python-env: environment still does not import every required module after install" >&2
    return 1
  fi
  return 0
}

bubbles_python_report() {
  local resolved module status_line satisfied=0
  echo "Bubbles managed Python posture"
  echo "  managed venv : $(bubbles_python_home)"
  if resolved="$(bubbles_python_resolve)"; then
    satisfied=1
    echo "  resolved     : $resolved"
  else
    echo "  resolved     : NONE — no interpreter imports every required module"
  fi
  for module in "${BUBBLES_PYTHON_MODULES[@]}"; do
    if [[ "$satisfied" -eq 1 ]] && "$resolved" -c "import $module" >/dev/null 2>&1; then
      status_line="ok"
    else
      status_line="MISSING"
    fi
    printf '  module %-12s %s\n' "$module" "$status_line"
  done
  [[ "$satisfied" -eq 1 ]] || return 1
  return 0
}

# ── executed (not sourced) ────────────────────────────────────────────────
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  set -euo pipefail

  bubbles_python_usage() {
    cat <<'EOF'
Usage: bash bubbles/scripts/python-env.sh [--check | --provision | --path]

  --check       Report interpreter/module posture. Exit 0 satisfied, 1 not.
  --provision   Create or repair the managed virtualenv and install the pinned
                requirements from bubbles/requirements.txt.
  --path        Print the resolved interpreter. Exit 1 when none satisfies.
  --help        This text.

Resolution order: $BUBBLES_PYTHON, then the managed venv, then python3 on PATH.
Override the venv location with BUBBLES_PYTHON_HOME.
There is no --skip/--force: an unsatisfied posture is reported, never bypassed.
EOF
  }

  mode="--check"
  if [[ $# -gt 0 ]]; then
    mode="$1"
    shift
  fi
  if [[ $# -gt 0 ]]; then
    echo "python-env: unexpected argument: $1" >&2
    bubbles_python_usage >&2
    exit 2
  fi

  case "$mode" in
    --check)
      if bubbles_python_report; then
        exit 0
      fi
      echo >&2
      echo "Remediate with: bash bubbles/scripts/python-env.sh --provision" >&2
      exit 1
      ;;
    --provision)
      requirements="$(bubbles_python_requirements_default)"
      bubbles_python_provision "$requirements"
      provision_status=$?
      if [[ "$provision_status" -ne 0 ]]; then
        exit "$provision_status"
      fi
      echo
      bubbles_python_report
      ;;
    --path)
      if resolved_path="$(bubbles_python_resolve)"; then
        printf '%s\n' "$resolved_path"
        exit 0
      fi
      echo "python-env: no interpreter satisfies the required modules" >&2
      exit 1
      ;;
    --help | -h)
      bubbles_python_usage
      exit 0
      ;;
    *)
      echo "python-env: unknown mode: $mode" >&2
      bubbles_python_usage >&2
      exit 2
      ;;
  esac
fi
