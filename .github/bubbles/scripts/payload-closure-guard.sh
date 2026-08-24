#!/usr/bin/env bash
# payload-closure-guard.sh — assert the downstream payload is dependency-closed.
#
# IMP-042 SCOPE-12 / REG-11.
#
# The release manifest sorts every tracked file into one of two payload classes:
# `managed` (copied into a downstream install) and `sourceOnly` (kept in the
# framework source tree only). Nothing checked that the classes were CONSISTENT
# with each other, so a managed executable could reference a source-only script.
# Downstream that reference resolves to a file that is not there.
#
# The eval subsystem is where this actually happened. `eval-harness.sh` was
# source-only while three selftests that drive it, plus a `cli.sh` subcommand,
# shipped as managed. `eval-corpus-selftest.sh` was additionally scheduled
# self-only, so it shipped in every downstream install purely to skip.
#
# This guard reads the two classes out of the manifest and fails when a managed
# shell script references a source-only script without an existence guard. An
# existence guard is accepted because it degrades cleanly: `cli.sh eval` now
# tests for the harness and prints an explanation instead of dying on a missing
# file, which is the correct shape for a shipped entry point whose subsystem is
# deliberately not shipped.
#
# Usage:
#   payload-closure-guard.sh [repo_root]
#
# Exit codes:
#   0  payload is dependency-closed
#   1  one or more managed files reference an unshipped dependency
#   2  the manifest is missing or unreadable

set -euo pipefail

REPO_ROOT="${1:-}"
if [[ -z "$REPO_ROOT" ]]; then
  SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
  REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
fi

MANIFEST="$REPO_ROOT/bubbles/release-manifest.json"
LABEL="payload-closure-guard"

err() { printf '[%s][ERROR] %s\n' "$LABEL" "$*" >&2; }
info() { printf '[%s] %s\n' "$LABEL" "$*"; }

if [[ ! -f "$MANIFEST" ]]; then
  err "release manifest not found: bubbles/release-manifest.json"
  exit 2
fi

# Extract the two payload classes. Entries are one per line as
#   {"path": "<path>", "sha256": "<hex>"},
# so a single awk pass keyed on the section header is enough; this avoids a
# JSON dependency in a script that has to run under a minimal PATH.
extract_class() {
  awk -v section="\"$1\":" '
    index($0, section) { capture = 1; next }
    capture && /^[[:space:]]*\]/ { capture = 0 }
    capture {
      if (match($0, /"path": "[^"]+"/)) {
        entry = substr($0, RSTART + 9, RLENGTH - 10)
        print entry
      }
    }
  ' "$MANIFEST"
}

managed_list="$(extract_class managedFileChecksums)"
source_only_list="$(extract_class sourceOnlyFileChecksums)"

managed_count="$(printf '%s\n' "$managed_list" | grep -c . || true)"
source_only_count="$(printf '%s\n' "$source_only_list" | grep -c . || true)"

if [[ "$managed_count" -eq 0 || "$source_only_count" -eq 0 ]]; then
  err "could not read payload classes from the manifest (managed=$managed_count source-only=$source_only_count)"
  exit 2
fi

# Only shell scripts can carry a runtime reference we can detect mechanically.
source_only_scripts="$(printf '%s\n' "$source_only_list" | grep '\.sh$' || true)"

# Scripts that framework-validate schedules with run_check_self_only never
# execute in a downstream install, so a source-only dependency in one of them is
# not reachable there. Read that set from the scheduler rather than maintaining
# a second hand-written list -- the scheduler is the authority on what runs
# where, and a hand list would drift from it silently.
self_only_scheduled=""
scheduler="$REPO_ROOT/bubbles/scripts/framework-validate.sh"
if [[ -f "$scheduler" ]]; then
  self_only_scheduled="$(
    grep -v '^[[:space:]]*#' "$scheduler" |
      grep -E 'run_check_self_only' |
      grep -o -E '[a-z0-9][a-z0-9._-]*\.sh' |
      LC_ALL=C sort -u
  )"
fi

findings=0
scanned=0

while IFS= read -r managed_file; do
  [[ -n "$managed_file" ]] || continue
  case "$managed_file" in
    *.sh) ;;
    *) continue ;;
  esac
  managed_path="$REPO_ROOT/$managed_file"
  [[ -f "$managed_path" ]] || continue
  scanned=$((scanned + 1))

  managed_base="${managed_file##*/}"
  if printf '%s\n' "$self_only_scheduled" | grep -Fxq "$managed_base"; then
    continue
  fi

  while IFS= read -r dep_file; do
    [[ -n "$dep_file" ]] || continue
    dep_base="${dep_file##*/}"

    # A runtime dependency is a VARIABLE-ROOTED path: "$SCRIPT_DIR/x.sh",
    # "$REPO_ROOT/tests/.../x.sh". A registry or classification entry is a
    # LITERAL repo-relative path sitting in an array. That distinction is the
    # whole discriminator -- matching on the bare basename instead reported 21
    # violations where only 4 were real, because it counted the manifest
    # generator's own source-only classification list as a dependency on the
    # files it was classifying.
    #
    # Comments are stripped first so a "# Invoked by x.sh" header never counts.
    # Writes are stripped too: a selftest that materialises a fixture with
    # `cat > "$repo/.../x.sh"` or `touch "$d/install.sh"` CREATES that path
    # inside its own scratch tree, so it does not depend on the real file.
    variable_path_pattern="\\\$\{?[A-Za-z_][A-Za-z0-9_]*\}?/[^\"';|&[:space:]]*$dep_base"
    runtime_refs="$(
      grep -v '^[[:space:]]*#' "$managed_path" |
      grep -v -E ">>?[[:space:]]*\"[^\"]*$dep_base" |
      grep -v -E "(touch|mkdir|rm|cp|mv)[[:space:]][^|;&]*$dep_base" |
      grep -o -E "$variable_path_pattern" |
      LC_ALL=C sort -u || true
    )"
    [[ -n "$runtime_refs" ]] || continue

    # A selftest can materialize a source-shaped marker inside its disposable
    # fixture, then chmod, stage, or execute that generated file. Exempt only
    # the EXACT variable-rooted path that was written. Creating
    # `$fixture/x.sh` must never excuse a separate `$SCRIPT_DIR/x.sh` runtime
    # dependency merely because both share one basename.
    materialized_refs="$(
      grep -v '^[[:space:]]*#' "$managed_path" |
      grep -E "((printf|cat)[^|;&]*>>?|touch[[:space:]])[^|;&]*$dep_base" |
      grep -o -E "$variable_path_pattern" |
      LC_ALL=C sort -u || true
    )"
    external_ref_found=false
    while IFS= read -r runtime_ref; do
      [[ -n "$runtime_ref" ]] || continue
      if ! printf '%s\n' "$materialized_refs" | grep -Fx "$runtime_ref" >/dev/null; then
        external_ref_found=true
        break
      fi
    done <<<"$runtime_refs"
    if [[ "$external_ref_found" != "true" ]]; then
      continue
    fi

    # Two guard shapes are accepted, because both degrade cleanly downstream.
    #
    # 1. An existence test. `cli.sh eval` tests for the harness and prints an
    #    explanation instead of dying on a missing file, which is the correct
    #    shape for a shipped entry point whose subsystem is deliberately not
    #    shipped. The test may sit anywhere in a multi-line condition, so this
    #    matches the operator rather than a `[[` prefix.
    # 2. run_check_self_only. This is the framework's own established wrapper
    #    for "source tree only"; framework-validate schedules the regression
    #    suite through it, so downstream those checks are skipped, not broken.
    if grep -E -q -- "-[fxde][[:space:]]+\"[^\"]*$dep_base\"" "$managed_path"; then
      continue
    fi
    if grep -v '^[[:space:]]*#' "$managed_path" |
      grep -E "run_check_self_only.*$dep_base" >/dev/null; then
      continue
    fi

    # 3. An explicit, reasoned allowance. Some references resolve OUTSIDE the
    #    payload entirely -- `cli.sh upgrade` runs install.sh from a source
    #    checkout the operator supplies or from a release tarball it fetches,
    #    never from the installed tree. No amount of pattern matching can infer
    #    that, so it has to be stated. The marker must carry a reason, which
    #    keeps the exemption reviewable instead of invisible.
    if grep -F -q 'payload-closure-allow' "$managed_path"; then
      continue
    fi

    err "managed file references unshipped dependency: $managed_file -> $dep_file"
    findings=$((findings + 1))
  done <<<"$source_only_scripts"
done <<<"$managed_list"

if [[ "$findings" -gt 0 ]]; then
  err "found $findings payload closure violation(s)"
  err "either ship the dependency, move the referencing file to the same class, or add an existence guard"
  exit 1
fi

info "OK — $scanned managed shell script(s) scanned against $source_only_count source-only entry(ies), payload is dependency-closed"
exit 0
