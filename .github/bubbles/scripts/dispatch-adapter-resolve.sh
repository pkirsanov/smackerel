#!/usr/bin/env bash
# Resolve optional dispatchAdmission.adapter. Absence is deliberately none.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADAPTER_DIR="$FRAMEWORK_ROOT/adapters/dispatch"
repo_root="$PWD"
names_only=0

usage() {
  cat >&2 <<'EOF'
Usage: dispatch-adapter-resolve.sh [--repo-root PATH] [--names-only]
Reads project-owned dispatchAdmission.adapter. Absence resolves to none.
EOF
}
fail() { echo "dispatch-adapter-resolve: $1" >&2; exit "${2:-1}"; }
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root) [[ $# -ge 2 ]] || fail "--repo-root requires a value" 2; repo_root="$2"; shift 2 ;;
    --names-only) names_only=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1" 2 ;;
  esac
done
[[ -d "$repo_root" ]] || fail "repo root not found: $repo_root" 2
repo_root="$(cd "$repo_root" && pwd)"
config_file=""
[[ -f "$repo_root/.github/bubbles-project.yaml" ]] && config_file="$repo_root/.github/bubbles-project.yaml"
[[ -z "$config_file" && -f "$repo_root/bubbles-project.yaml" ]] && config_file="$repo_root/bubbles-project.yaml"
adapter=""
if [[ -n "$config_file" ]]; then
  adapter="$(awk '
    /^[[:space:]]*#/ { next }
    /^dispatchAdmission:[[:space:]]*$/ { inblock=1; next }
    inblock && /^[^[:space:]]/ { inblock=0 }
    inblock && $1 == "adapter:" { value=$2; gsub(/["\047]/,"",value); print value; exit }
  ' "$config_file" 2>/dev/null || true)"
fi
[[ -n "$adapter" ]] || adapter="none"
case "$adapter" in *[!a-z0-9-]*|''|-*) fail "invalid dispatchAdmission.adapter '$adapter'" ;; esac
adapter_path="$ADAPTER_DIR/$adapter.sh"
[[ -f "$adapter_path" ]] || fail "configured dispatchAdmission.adapter '$adapter' has no adapter at $adapter_path"
printf 'adapter=%s\n' "$adapter"
[[ "$names_only" == 1 ]] && exit 0
printf 'adapterPath=%s\nrepoRoot=%s\n' "$adapter_path" "$repo_root"