#!/usr/bin/env bash
# validation-closure.sh — the declared-input closure consumer.
#
# Capability: declared-input-closure-validation
#
# WHAT THIS REPLACES
# Two decisions used to be made from a BASENAME PAIR: "did the change touch this
# check?" and "may this check reuse its last result?". Both assumed a selftest
# `X-selftest.sh` owns `X.sh` and nothing else. Changing
# `bubbles/registry/gates.yaml` therefore skipped 97 checks that read it, and
# changing `guard-lib.sh` served a cached PASS to 100 checks that source it.
#
# Both decisions now come from the GENERATED closure map,
# `bubbles/registry/validation-checks.yaml`, whose contents were derived by
# tracing real references rather than inferred from names.
#
# THE TWO RULES THAT MAKE THIS SAFE
#   1. A change to ANY declared input invalidates EVERY dependent check.
#   2. An UNKNOWN closure (`closureComplete: false`) is always executed and can
#      never be reused. Not knowing the inputs is not the same as knowing there
#      are none, and only one of those permits reuse.
#
# The runtime digest covers more than the file contents: the framework VERSION,
# the platform (`uname -s`), and the VERSION STRING of every external command in
# the closure. A `yq` upgrade changes verdicts, so it must change the key.
#
# Usage (CLI):
#   validation-closure.sh list [--incomplete|--complete]
#   validation-closure.sh id-for <script-path>
#   validation-closure.sh inputs <check-id>
#   validation-closure.sh digest <check-id>
#   validation-closure.sh reusable <check-id>
#   validation-closure.sh affected --changed <path> [--changed <path> ...]
#   validation-closure.sh plan [--changed <path> ...] [--format text|ids]
#
# Usage (library):  source validation-closure.sh; vclosure_load
#
# There is no --skip, --force or --assume-unchanged flag. A check is excluded by
# proving nothing it depends on moved, never by asserting it.
#
# Exit codes:
#   0  the query succeeded (for `reusable`: the check may be reused)
#   1  the query answered NO (unknown closure, unknown check, not reusable)
#   2  usage error, or the closure map could not be read

set -uo pipefail

VCLOSURE_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VCLOSURE_REPO_ROOT="$(cd "$VCLOSURE_SCRIPT_DIR/../.." && pwd)"
VCLOSURE_REGISTRY_DEFAULT="$VCLOSURE_REPO_ROOT/bubbles/registry/validation-checks.yaml"

declare -A VCLOSURE_INPUTS=()
declare -A VCLOSURE_COMMANDS=()
declare -A VCLOSURE_COMPLETE=()
declare -A VCLOSURE_SCRIPT=()
declare -A VCLOSURE_ID_BY_SCRIPT=()
declare -a VCLOSURE_IDS=()
VCLOSURE_LOADED=""

vclosure_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | cut -d' ' -f1
  else
    return 1
  fi
}

# vclosure_load [registry-path] [repo-root]
vclosure_load() {
  local registry="${1:-$VCLOSURE_REGISTRY_DEFAULT}"
  local root="${2:-$VCLOSURE_REPO_ROOT}"
  [[ -f "$registry" ]] || return 2

  VCLOSURE_INPUTS=()
  VCLOSURE_COMMANDS=()
  VCLOSURE_COMPLETE=()
  VCLOSURE_SCRIPT=()
  VCLOSURE_ID_BY_SCRIPT=()
  VCLOSURE_IDS=()

  local line id="" section=""
  while IFS= read -r line; do
    case "$line" in
      '  vc-'*:)
        id="${line#  }"
        id="${id%:}"
        VCLOSURE_IDS+=("$id")
        VCLOSURE_INPUTS["$id"]=""
        VCLOSURE_COMMANDS["$id"]=""
        VCLOSURE_COMPLETE["$id"]="false"
        section=""
        ;;
      '    script: '*)
        [[ -n "$id" ]] || continue
        VCLOSURE_SCRIPT["$id"]="${line#    script: }"
        VCLOSURE_ID_BY_SCRIPT["${VCLOSURE_SCRIPT[$id]}"]="$id"
        ;;
      '    closureComplete: '*)
        [[ -n "$id" ]] || continue
        VCLOSURE_COMPLETE["$id"]="${line#    closureComplete: }"
        ;;
      '    inputs:') section="inputs" ;;
      '    commands:') section="commands" ;;
      '    - '*)
        [[ -n "$id" ]] || continue
        case "$section" in
          inputs) VCLOSURE_INPUTS["$id"]+="${line#    - }"$'\n' ;;
          commands) VCLOSURE_COMMANDS["$id"]+="${line#    - }"$'\n' ;;
        esac
        ;;
      *) ;;
    esac
  done <"$registry"

  VCLOSURE_LOADED="$root"
  [[ "${#VCLOSURE_IDS[@]}" -gt 0 ]] || return 2
  return 0
}

vclosure_id_for_script() {
  local script="$1"
  printf '%s' "${VCLOSURE_ID_BY_SCRIPT[$script]:-}"
}

# A closure the generator could not enumerate is UNKNOWN. Unknown executes and
# never reuses.
vclosure_reusable() {
  local id="$1"
  [[ -n "${VCLOSURE_COMPLETE[$id]:-}" ]] || return 1
  [[ "${VCLOSURE_COMPLETE[$id]}" == "true" ]] || return 1
  return 0
}

# vclosure_digest <id> <framework-version>
#
# Content of every declared input, plus the framework version, the platform and
# the version string of every external command. A check whose closure is unknown
# has NO digest: returning one would imply the inputs are known.
vclosure_digest() {
  local id="$1" version="${2:-unknown}"
  vclosure_reusable "$id" || return 1
  local root="${VCLOSURE_LOADED:-$VCLOSURE_REPO_ROOT}"

  local material="" path abs cmd cmd_version
  material+="closure-v1"$'\n'
  material+="version=$version"$'\n'
  material+="platform=$(uname -s 2>/dev/null || echo unknown)"$'\n'
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    abs="$root/$path"
    if [[ -f "$abs" ]]; then
      material+="$path $(vclosure_sha256 <"$abs")"$'\n'
    else
      # A declared input that is gone changes the answer as surely as one that
      # changed, so absence is recorded rather than skipped.
      material+="$path absent"$'\n'
    fi
  done <<<"${VCLOSURE_INPUTS[$id]}"
  while IFS= read -r cmd; do
    [[ -n "$cmd" ]] || continue
    if command -v "$cmd" >/dev/null 2>&1; then
      cmd_version="$("$cmd" --version 2>/dev/null | head -1 || true)"
      material+="cmd:$cmd ${cmd_version:-present}"$'\n'
    else
      material+="cmd:$cmd absent"$'\n'
    fi
  done <<<"${VCLOSURE_COMMANDS[$id]}"

  printf '%s' "$material" | vclosure_sha256
}

# vclosure_affected <id> <changed-paths-newline-separated>
#
# An unknown closure is affected by EVERY change, which is what forces it to run.
vclosure_affected() {
  local id="$1" changed="$2" path
  vclosure_reusable "$id" || return 0
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    case $'\n'"${VCLOSURE_INPUTS[$id]}" in
      *$'\n'"$path"$'\n'*) return 0 ;;
    esac
  done <<<"$changed"
  return 1
}

# --- CLI ---------------------------------------------------------------------
# Sourcing must not execute the CLI, or every consumer would run a subcommand.
(return 0 2>/dev/null) && return 0

NAME="validation-closure"
REGISTRY="$VCLOSURE_REGISTRY_DEFAULT"
FRAMEWORK_VERSION="unknown"
[[ -f "$VCLOSURE_REPO_ROOT/VERSION" ]] &&
  FRAMEWORK_VERSION="$(tr -d '[:space:]' <"$VCLOSURE_REPO_ROOT/VERSION" 2>/dev/null || echo unknown)"

SUBCOMMAND="${1:-}"
[[ $# -gt 0 ]] && shift

declare -a CHANGED=()
FILTER=""
ARG=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --changed)
      shift
      CHANGED+=("${1:-}")
      ;;
    --registry)
      shift
      REGISTRY="${1:-}"
      ;;
    --repo-root)
      shift
      VCLOSURE_REPO_ROOT="${1:-}"
      ;;
    --complete) FILTER="complete" ;;
    --incomplete) FILTER="incomplete" ;;
    --*)
      printf '%s: unsupported flag "%s". This tool has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
    *) ARG="$1" ;;
  esac
  shift || true
done

if ! vclosure_load "$REGISTRY" "$VCLOSURE_REPO_ROOT"; then
  printf '%s: cannot read the closure map: %s\n' "$NAME" "$REGISTRY" >&2
  printf '%s: regenerate it with bubbles/scripts/generate-validation-checks.sh\n' "$NAME" >&2
  exit 2
fi

changed_text=""
for c in "${CHANGED[@]:-}"; do
  [[ -n "$c" ]] && changed_text+="$c"$'\n'
done

case "$SUBCOMMAND" in
  list)
    for id in "${VCLOSURE_IDS[@]}"; do
      case "$FILTER" in
        complete) vclosure_reusable "$id" || continue ;;
        incomplete) vclosure_reusable "$id" && continue ;;
      esac
      printf '%s\t%s\t%s\n' "$id" "${VCLOSURE_COMPLETE[$id]}" "${VCLOSURE_SCRIPT[$id]:-}"
    done
    ;;
  id-for)
    [[ -n "$ARG" ]] || {
      printf '%s: id-for requires a script path\n' "$NAME" >&2
      exit 2
    }
    id="$(vclosure_id_for_script "$ARG")"
    [[ -n "$id" ]] || exit 1
    printf '%s\n' "$id"
    ;;
  inputs)
    [[ -n "${VCLOSURE_INPUTS[$ARG]:-}" ]] || exit 1
    printf '%s' "${VCLOSURE_INPUTS[$ARG]}"
    ;;
  digest)
    d="$(vclosure_digest "$ARG" "$FRAMEWORK_VERSION")" || exit 1
    printf '%s\n' "$d"
    ;;
  reusable)
    vclosure_reusable "$ARG" || exit 1
    ;;
  affected)
    [[ -n "$changed_text" ]] || {
      printf '%s: affected requires at least one --changed <path>\n' "$NAME" >&2
      exit 2
    }
    for id in "${VCLOSURE_IDS[@]}"; do
      vclosure_affected "$id" "$changed_text" && printf '%s\n' "$id"
    done
    ;;
  plan)
    for id in "${VCLOSURE_IDS[@]}"; do
      if [[ -z "$changed_text" ]]; then
        printf 'RUN\t%s\n' "$id"
        continue
      fi
      if vclosure_affected "$id" "$changed_text"; then
        printf 'RUN\t%s\n' "$id"
      else
        printf 'REUSABLE\t%s\n' "$id"
      fi
    done
    ;;
  "" | -h | --help)
    printf 'usage: %s.sh <list|id-for|inputs|digest|reusable|affected|plan> [options]\n' "$NAME"
    exit 0
    ;;
  *)
    printf '%s: unknown subcommand "%s"\n' "$NAME" "$SUBCOMMAND" >&2
    exit 2
    ;;
esac
exit 0
