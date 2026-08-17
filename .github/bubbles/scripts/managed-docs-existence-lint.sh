#!/usr/bin/env bash
# managed-docs-existence-lint.sh — required managed docs must actually exist.
#
# IMP-042 SCOPE-13 / REG-12.
#
# docs-registry.yaml declares which docs a repository maintains and which are
# required. Nothing checked that a required doc's path resolves to a real file,
# so the registry could promise a document that was never written and every
# projection built from it inherited the claim.
#
# Scope: this contract is for PRODUCT repositories. The Bubbles source tree owns
# no docs/Architecture.md or docs/API.md and correctly should not -- its own
# document set is governed by the governance index and release-check's
# required-files list. Running the check here would report guaranteed failures
# that say nothing about the framework, so a framework source tree is skipped
# explicitly rather than silently.
#
# Usage:
#   managed-docs-existence-lint.sh [repo_root]
#
# Exit codes:
#   0  every required managed doc exists, or this is a framework source tree
#   1  a required managed doc path does not exist
#   2  the registry or resolver is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${1:-$(cd -- "$SCRIPT_DIR/../.." && pwd)}"
RESOLVER="$SCRIPT_DIR/docs-registry-resolve.sh"
LABEL="managed-docs-existence-lint"

err() { printf '[%s][ERROR] %s\n' "$LABEL" "$*" >&2; }
info() { printf '[%s] %s\n' "$LABEL" "$*"; }

if [[ -f "$REPO_ROOT/install.sh" && -f "$REPO_ROOT/VERSION" && -d "$REPO_ROOT/bubbles/scripts" ]]; then
  info "SKIP — framework source tree; the managed-doc contract governs product repositories"
  exit 0
fi

if [[ ! -x "$RESOLVER" ]]; then
  err "docs-registry-resolve.sh not available at $RESOLVER"
  exit 2
fi

effective="$(cd "$REPO_ROOT" && bash "$RESOLVER" --effective 2>/dev/null)"
if [[ -z "$effective" ]]; then
  err "docs registry resolved to nothing"
  exit 2
fi

# Pair each managed doc key with its resolved path and required flag. The
# resolver emits a stable two-space/four-space shape, so a single awk pass is
# enough and avoids a YAML dependency in a check that must run downstream.
required_docs="$(printf '%s\n' "$effective" | awk '
  /^  [a-zA-Z][a-zA-Z0-9_-]*:[[:space:]]*$/ {
    key = $1; sub(/:$/, "", key); path = ""; next
  }
  /^    path:[[:space:]]/ { path = $2; next }
  /^    required:[[:space:]]/ {
    if ($2 == "true" && key != "" && path != "") print key "\t" path
    next
  }
')"

checked=0
findings=0
while IFS=$'\t' read -r key path; do
  [[ -n "$key" && -n "$path" ]] || continue
  checked=$((checked + 1))
  if [[ ! -e "$REPO_ROOT/$path" ]]; then
    err "required managed doc '$key' declares $path, which does not exist"
    findings=$((findings + 1))
  fi
done <<<"$required_docs"

if [[ "$checked" -eq 0 ]]; then
  err "no required managed docs were parsed from the effective registry"
  exit 2
fi

if [[ "$findings" -gt 0 ]]; then
  err "found $findings required managed doc(s) with no file"
  err "either write the document, repoint it with docsRegistryOverrides, or stop declaring it required"
  exit 1
fi

info "OK — all $checked required managed doc(s) exist"
exit 0
