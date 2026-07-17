#!/usr/bin/env bash
# Repo-Binding Preflight (BFW-05 / IMP-025)
# ---------------------------------------------------------------------------
# Fail-loud guard against a multi-root workspace hazard: a session editing one
# repository while running a Bubbles agent installed under a DIFFERENT workspace
# root (the failure mode where an agent bound to one repo silently mutates
# another). It asserts, before mutable work, that the agent's source repository
# matches the repository being edited — or that canonical framework-source work
# is explicitly selected.
#
# Reuse-first: it reuses the SAME repo-slug derivation the installer already uses
# for the per-repo MCP server id (install.sh: basename -> lowercase -> sanitize),
# and it reads the existing `.github/bubbles/.install-source.json` install
# metadata when a `targetRepoSlug` marker is present (forward-compatible with the
# installer/agent stamping that writes it). No per-machine absolute path is used
# — only the repo-relative slug.
#
# Exit codes: 0 = bound OK (or advisory: no marker to check), 1 = binding
# mismatch (refuse), 2 = usage error. No --skip/--force bypass.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: repo-binding-preflight.sh [--repo-root <dir>] [--agent-source <slug>] [--canonical-source]

  --repo-root <dir>      Target repository root (default: git toplevel of CWD, else CWD).
  --agent-source <slug>  The repo slug the ACTIVE agent was installed for (its embedded
                         marker). If it differs from the target repo, the agent belongs to
                         another workspace root — refuse.
  --canonical-source     The active agent IS the canonical Bubbles framework source
                         (framework-health work). Always passes.

With neither --agent-source nor --canonical-source, the expected slug is read from
<repo-root>/.github/bubbles/.install-source.json "targetRepoSlug" when present;
absent that marker, the check is advisory (exit 0) with remediation.
EOF
}

# Shared repo-slug derivation — MUST match install.sh's mcp_repo_slug logic.
repo_slug_of() {
  local name="$1"
  printf '%s' "$name" \
    | LC_ALL=C tr '[:upper:]' '[:lower:]' \
    | LC_ALL=C sed -e 's/[^a-z0-9]/-/g' -e 's/--*/-/g' -e 's/^-//' -e 's/-$//'
}

repo_root=""
agent_source=""
canonical_source="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root) repo_root="$2"; shift 2 ;;
    --agent-source) agent_source="$2"; shift 2 ;;
    --canonical-source) canonical_source="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "repo-binding-preflight: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# Canonical framework-source work is always allowed (the agent IS the source).
if [[ "$canonical_source" == "true" ]]; then
  echo "[repo-binding-preflight] OK — canonical Bubbles framework-source work (no downstream binding required)."
  exit 0
fi

# Resolve the target repository root.
if [[ -z "$repo_root" ]]; then
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  [[ -n "$repo_root" ]] || repo_root="$PWD"
fi
if [[ ! -d "$repo_root" ]]; then
  echo "repo-binding-preflight: repo root not found: $repo_root" >&2
  exit 2
fi
target_slug="$(repo_slug_of "$(basename "$repo_root")")"
[[ -n "$target_slug" ]] || target_slug="repo"

# Resolve the expected (agent-source) slug.
expected_slug="$agent_source"
marker_source="--agent-source"
if [[ -z "$expected_slug" ]]; then
  marker_file="$repo_root/.github/bubbles/.install-source.json"
  if [[ -f "$marker_file" ]]; then
    # Extract "targetRepoSlug": "<value>" without requiring jq.
    expected_slug="$(sed -nE 's/.*"targetRepoSlug"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' "$marker_file" | head -n1)"
    marker_source="$marker_file targetRepoSlug"
  fi
fi

# No expected identity available → advisory (do not block what we cannot verify).
if [[ -z "$expected_slug" ]]; then
  {
    echo "[repo-binding-preflight] ADVISORY — no repo-binding marker for '$target_slug'."
    echo "  Pass --agent-source <slug> (the active agent's install repo), --canonical-source"
    echo "  for framework work, or (re)install so .github/bubbles/.install-source.json records"
    echo "  targetRepoSlug. Cannot verify the agent-source binding without one."
  } >&2
  exit 0
fi

if [[ "$expected_slug" == "$target_slug" ]]; then
  echo "[repo-binding-preflight] OK — agent source '$expected_slug' matches target repo '$target_slug'."
  exit 0
fi

{
  echo "[repo-binding-preflight] BINDING MISMATCH — refusing before mutable work:"
  echo "  target repository : $target_slug ($repo_root)"
  echo "  agent source      : $expected_slug (from $marker_source)"
  echo "  The active agent belongs to a DIFFERENT workspace root. Select the '$target_slug'"
  echo "  repository's own installed agent, or pass --canonical-source for framework-source"
  echo "  work. Do NOT edit '$target_slug' with '$expected_slug''s agent."
} >&2
exit 1
