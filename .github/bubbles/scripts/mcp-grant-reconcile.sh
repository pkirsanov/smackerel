#!/usr/bin/env bash
# File: mcp-grant-reconcile.sh
#
# Shared, sourceable library for operator-managed MCP tool grants on the five
# framework-managed orchestrator agents that ship with a RESTRICTIVE `tools:`
# allowlist (bubbles.goal/sprint/iterate/bug/workflow).
#
# Problem this solves: those agents are checksum-pinned framework-managed files.
# An operator who wants one of them to also use an MCP tool (e.g. playwright,
# github) cannot edit the allowlist without triggering "Framework-managed file
# drift detected" and having the edit wiped on the next refresh.
#
# Model (project-owned intent, framework-trusted verification):
#   - Operators DECLARE grants in the project-owned `.github/bubbles-project.yaml`
#     under `mcp.grants`. Keys are agent names OR the reserved group alias
#     `restricted-orchestrators`; values are lists of tool names.
#   - `mcp-grant-sync.sh` INJECTS the declared grants into the agent files as a
#     deterministic, append-only suffix on the canonical single-line `tools:`
#     array.
#   - The integrity guard (downstream-framework-write-guard.sh) becomes
#     grant-aware via `bubbles_mcp_reconcile_to_stdout`: it STRIPS only the
#     operator-declared grant tokens and exact-matches the result against the
#     UNCHANGED canonical `.checksums` hash. A declared grant is accepted; any
#     undeclared edit (body tamper, undeclared tool, missing core token) still
#     fails as drift.
#
# Trust boundary: the canonical core + restricted-agent set below are framework
# constants in this checksum-pinned file (trusted). The grant list comes from the
# project-owned config (untrusted) and is used ONLY as a strip-allowlist for
# reconstruction — never as a source of canonical bytes. The trust anchor stays
# on the framework-managed `.checksums`.
#
# This file is SOURCEABLE (defines functions, runs nothing on source). It is also
# safe to execute directly (prints a one-line self-description) for doctor's
# executable-script convention.

# --- Framework constants (trusted; checksum-pinned) -----------------------

# Canonical core allowlist shared by all five restricted orchestrators. Verified
# byte-identical across bubbles.{goal,sprint,iterate,bug,workflow}.agent.md:
#   tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright]
# `bubbles` (the framework's own MCP server) and `playwright` ship as framework
# defaults so the autonomous orchestrators can drive framework + browser MCP
# tools out of the box. Per-downstream additions layer on via mcp.grants.
#
# `bubbles` is the CANONICAL placeholder for the framework MCP server token. On
# a downstream install the server registers in .vscode/mcp.json under a UNIQUE
# per-repo id (bubbles-<repo-slug>) so VS Code 1.118+ does not dedup-disable it
# in a multi-root workspace. The placeholder is therefore MATERIALIZED to that
# per-repo id in the installed agents (see bubbles_mcp_server_token) so the
# orchestrators actually bind to the running server. The canonical source — and
# therefore .checksums — always stores `bubbles`; reconcile normalizes the
# materialized token back to `bubbles` before hashing, exactly like a stripped
# grant. In the Bubbles SOURCE repo the token stays canonical `bubbles`.
BUBBLES_MCP_CORE_TOOLS=(read search edit agent todo web execute bubbles playwright)

# The canonical placeholder token that names the framework MCP server in the
# core allowlist. Materialized per-repo on downstream installs.
BUBBLES_MCP_SERVER_PLACEHOLDER="bubbles"

# The five framework-managed orchestrators that ship the restrictive allowlist.
# Only these are eligible for grants; every other agent inherits all tools.
BUBBLES_MCP_RESTRICTED_AGENTS=(
  bubbles.goal
  bubbles.sprint
  bubbles.iterate
  bubbles.bug
  bubbles.workflow
)

# Reserved group-alias key in mcp.grants that fans out to all restricted agents.
BUBBLES_MCP_GROUP_ALIAS="restricted-orchestrators"

# --- MCP server token materialization (per-repo unique id) ----------------

# Resolve the framework MCP server token to materialize in the restricted
# orchestrators' `tools:` allowlist.
#
# - Downstream install layout (this lib at <repo>/.github/bubbles/scripts):
#   returns `bubbles-<repo-slug>`, matching the id install.sh registers in
#   .vscode/mcp.json. The slug algorithm MUST stay byte-identical to install.sh
#   ("Register the Bubbles MCP server" step) so the agent token matches the
#   registered server and the IDE binds it.
# - Bubbles SOURCE layout (this lib at <repo>/bubbles/scripts): returns the
#   canonical placeholder `bubbles` — NO per-repo materialization, so the source
#   agents and their .checksums stay canonical.
# - BUBBLES_MCP_FORCE_SERVER_TOKEN overrides the derivation (hermetic selftests).
#
# Layout is detected from THIS lib's own path (BASH_SOURCE), so every caller
# (mcp-grant-sync.sh, downstream-framework-write-guard.sh, selftest) derives the
# identical token regardless of which script sourced the lib.
bubbles_mcp_server_token() {
  if [[ -n "${BUBBLES_MCP_FORCE_SERVER_TOKEN:-}" ]]; then
    printf '%s' "$BUBBLES_MCP_FORCE_SERVER_TOKEN"
    return 0
  fi
  local lib_dir project_root base slug
  lib_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if [[ "$lib_dir" != *"/.github/bubbles/scripts" ]]; then
    printf '%s' "$BUBBLES_MCP_SERVER_PLACEHOLDER"
    return 0
  fi
  project_root="${lib_dir%/.github/bubbles/scripts}"
  base="$(basename "$project_root")"
  slug="$(printf '%s' "$base" | LC_ALL=C tr '[:upper:]' '[:lower:]' | LC_ALL=C sed -e 's/[^a-z0-9]/-/g' -e 's/--*/-/g' -e 's/^-//' -e 's/-$//')"
  [[ -n "$slug" ]] || slug="repo"
  printf '%s' "bubbles-${slug}"
}

# --- Predicates -----------------------------------------------------------

bubbles_mcp_is_restricted_agent() {
  local name="$1"
  local candidate
  for candidate in "${BUBBLES_MCP_RESTRICTED_AGENTS[@]}"; do
    [[ "$candidate" == "$name" ]] && return 0
  done
  return 1
}

bubbles_mcp_is_core_tool() {
  local name="$1"
  local candidate
  for candidate in "${BUBBLES_MCP_CORE_TOOLS[@]}"; do
    [[ "$candidate" == "$name" ]] && return 0
  done
  # The materialized per-repo server token stands in for the canonical `bubbles`
  # core token, so treat it as core too (prevents a redundant grant suffix).
  [[ "$name" == "$(bubbles_mcp_server_token)" ]] && return 0
  return 1
}

# Resolve the project-owned config path for a repo root (downstream first).
# Prints nothing if neither location exists.
bubbles_mcp_config_path() {
  local project_root="$1"
  if [[ -f "$project_root/.github/bubbles-project.yaml" ]]; then
    printf '%s' "$project_root/.github/bubbles-project.yaml"
  elif [[ -f "$project_root/bubbles-project.yaml" ]]; then
    printf '%s' "$project_root/bubbles-project.yaml"
  fi
}

# --- Grant resolution -----------------------------------------------------

# Print the EFFECTIVE grant tokens for an agent: (agent-specific ∪ group-alias),
# minus core tokens, validated for charset, de-duplicated, lexically sorted.
# One token per line. Empty when: no config, yq absent, or no grants declared.
#
# Args: <config_file> <agent_name>
bubbles_mcp_effective_grants() {
  local config="$1"
  local agent="$2"

  [[ -n "$config" && -f "$config" ]] || return 0
  command -v yq >/dev/null 2>&1 || return 0

  # Read config via stdin redirect (the shell opens the file) so this works
  # even with a snap-confined yq that cannot access caller-created temp paths.
  local raw=''
  local agent_grants=''
  local alias_grants=''
  agent_grants="$(yq -r ".mcp.grants.\"${agent}\"[]?" - <"$config" 2>/dev/null || true)"
  alias_grants="$(yq -r ".mcp.grants.\"${BUBBLES_MCP_GROUP_ALIAS}\"[]?" - <"$config" 2>/dev/null || true)"
  raw="$(printf '%s\n%s\n' "$agent_grants" "$alias_grants")"

  local token
  printf '%s\n' "$raw" | while IFS= read -r token; do
    [[ -n "$token" ]] || continue
    if [[ ! "$token" =~ ^[A-Za-z0-9_.-]+$ ]]; then
      echo "bubbles mcp: ignoring invalid grant token '${token}' in ${config} (allowed: A-Za-z0-9_.-)" >&2
      continue
    fi
    bubbles_mcp_is_core_tool "$token" && continue
    printf '%s\n' "$token"
  done | LC_ALL=C sort -u
}

# Join the canonical core tools with ", " (the exact canonical separator),
# materializing the `bubbles` placeholder to the per-repo MCP server token.
bubbles_mcp_join_core() {
  local out='' token emit server_token
  server_token="$(bubbles_mcp_server_token)"
  for token in "${BUBBLES_MCP_CORE_TOOLS[@]}"; do
    emit="$token"
    [[ "$token" == "$BUBBLES_MCP_SERVER_PLACEHOLDER" ]] && emit="$server_token"
    out="${out:+$out, }$emit"
  done
  printf '%s' "$out"
}

# --- File transforms (stdout only; no in-place writes here) ---------------

# RECONCILE: print <file> with the operator-declared grant tokens stripped from
# its single-line `tools:` array, re-emitted in canonical "tools: [a, b, c]"
# format. Removing the declared grants from an injected line reproduces the
# canonical bytes exactly; the caller hashes stdout and compares to .checksums.
#
# Args: <file> <config_file> <agent_name>
bubbles_mcp_reconcile_to_stdout() {
  local file="$1"
  local config="$2"
  local agent="$3"

  local strip server_token
  strip="$(bubbles_mcp_effective_grants "$config" "$agent")"
  server_token="$(bubbles_mcp_server_token)"

  # Fast path: no grant suffix to strip AND no per-repo token to normalize
  # (source layout) — the file is already canonical, emit byte-identical.
  if [[ -z "$strip" && "$server_token" == "$BUBBLES_MCP_SERVER_PLACEHOLDER" ]]; then
    cat "$file"
    return 0
  fi

  awk -v strip="$strip" -v server_token="$server_token" -v placeholder="$BUBBLES_MCP_SERVER_PLACEHOLDER" '
    BEGIN {
      n = split(strip, arr, "\n")
      for (i = 1; i <= n; i++) if (arr[i] != "") drop[arr[i]] = 1
    }
    /^tools: \[.*\]$/ {
      inner = $0
      sub(/^tools: \[/, "", inner)
      sub(/\]$/, "", inner)
      m = split(inner, toks, ", ")
      out = ""
      for (i = 1; i <= m; i++) {
        t = toks[i]
        if (t in drop) continue
        if (t == server_token) t = placeholder   # normalize materialized token -> canonical
        out = (out == "") ? t : out ", " t
      }
      print "tools: [" out "]"
      next
    }
    { print }
  ' "$file"
}

# INJECT: print <file> with its single-line `tools:` array rewritten to
# canonical core + the agent's effective declared grants (sorted suffix). Pure
# function of (core constant, declared grants): idempotent, and a re-sync after
# a config change resets to the new desired state (handles add AND remove).
#
# Args: <file> <config_file> <agent_name>
bubbles_mcp_inject_to_stdout() {
  local file="$1"
  local config="$2"
  local agent="$3"

  local grants
  grants="$(bubbles_mcp_effective_grants "$config" "$agent")"

  local core_joined
  core_joined="$(bubbles_mcp_join_core)"

  local repl
  if [[ -n "$grants" ]]; then
    local grants_joined
    grants_joined="$(printf '%s\n' "$grants" | awk 'NR>1{printf ", "}{printf "%s",$0}')"
    repl="tools: [${core_joined}, ${grants_joined}]"
  else
    repl="tools: [${core_joined}]"
  fi

  awk -v repl="$repl" '
    /^tools: \[.*\]$/ { print repl; next }
    { print }
  ' "$file"
}

# When executed directly (not sourced), print a one-line self-description so the
# file behaves like the other executable scripts under doctor Check 5.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "mcp-grant-reconcile.sh — sourceable library for operator-managed MCP tool grants (v7.1)"
  echo "Restricted agents: ${BUBBLES_MCP_RESTRICTED_AGENTS[*]}"
  echo "Canonical core: ${BUBBLES_MCP_CORE_TOOLS[*]}"
fi
