#!/usr/bin/env bash
#
# security-gate.sh — mechanical enforcer for G034 (security_gate)
# (IMP-027 / SCOPE-4, SEC-3).
#
# WHY THIS EXISTS
# ---------------
# G034 was classified as one of only six businessInvariant gates, yet had NO
# enforcer script and was not referenced by any agent — including
# bubbles.security.agent.md. Its entire enforcement was "appears in a mode's
# requiredGates list", i.e. an LLM reading YAML and deciding whether it felt
# satisfied. A businessInvariant security gate with no mechanical surface is
# the weakest link in the invariant set: it carries the most authority and the
# least verification.
#
# This script gives it a surface. It checks the things a security gate can
# actually check about a repository from the outside, without pretending to be
# a vulnerability scanner:
#
#   1. secret-material    no private keys / keystores committed
#   2. inline-credentials no hardcoded password/token assignments in scripts
#   3. curl-pipe-shell    no `curl … | bash` remote-execution primitives
#   4. world-writable     no world-writable files in the tracked tree
#   5. eval-on-input      no `eval` on unsanitised command substitution
#
# Each finding names the file and line so it is actionable. This is a floor,
# not a ceiling: a project may wire additional scanners into its own gate set.
#
# Exit codes: 0 clean - 1 findings - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
quiet=0

usage() {
  cat <<'EOF'
security-gate.sh — mechanical enforcer for G034 (security_gate)

Usage:
  bash bubbles/scripts/security-gate.sh [--repo-root <path>] [--quiet]

Exit: 0 clean - 1 findings - 2 usage/environment error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "security-gate: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "security-gate: repo root not found: $repo_root" >&2
  exit 2
fi

findings=0
report() {
  printf 'FINDING: %s: %s\n' "$1" "$2"
  findings=$((findings + 1))
}

cd "$repo_root"

# Only inspect tracked files: the working tree may hold local scratch, and a
# gate that fails on an operator's untracked notes is a gate people disable.
if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
  mapfile -t tracked < <(git ls-files 2>/dev/null || true)
else
  mapfile -t tracked < <(find . -type f -not -path './.git/*' -printf '%P\n' 2>/dev/null || true)
fi

if [[ "${#tracked[@]}" -eq 0 ]]; then
  [[ "$quiet" -eq 1 ]] || echo "[security-gate] OK — no tracked files to inspect"
  exit 0
fi

# --- 1. Committed secret material ------------------------------------------
for f in "${tracked[@]}"; do
  case "$f" in
    *.pem | *.key | *.p12 | *.pfx | *.jks | *.keystore | *.mobileprovision | id_rsa | id_ed25519)
      report "secret-material" "$f is committed key/keystore material"
      ;;
  esac
done

while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  # A private-key MARKER inside documentation is the doc describing the pattern
  # (SECURITY-PII-POLICY.md does exactly that). Require the closing marker too,
  # so only an actual key block counts.
  case "$hit" in
    *.md | *security-gate*) continue ;;
  esac
  grep -q -- '-----END .*PRIVATE KEY-----' "$hit" 2>/dev/null || continue
  report "secret-material" "$hit contains a private-key block"
done < <(grep -rl -- '-----BEGIN .*PRIVATE KEY-----' "${tracked[@]}" 2>/dev/null || true)

# --- 2. Inline credentials in scripts --------------------------------------
#
# Matches an assignment of a non-empty literal to a secret-shaped name. Env
# indirection (VAR="$OTHER", VAR="${OTHER}") and empty placeholders are fine —
# those are the correct pattern, not the violation.
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  # Selftests build hermetic fixtures; a synthetic fixture token is not a
  # credential. Same carve-out the framework already grants selftests for
  # dependency SKIPs.
  case "$hit" in
    *-selftest.sh* | *dependency-posture.sh* | *security-gate* | *tests/*) continue ;;
  esac
  report "inline-credentials" "$hit"
done < <(
  grep -rnE '^[^#]*\b[A-Za-z_]*(PASSWORD|PASSWD|SECRET|TOKEN|APIKEY|API_KEY)[A-Za-z_]*=["'"'"']?[A-Za-z0-9/+_.-]{8,}["'"'"']?' \
    --include='*.sh' --include='*.yaml' --include='*.yml' . 2>/dev/null |
    grep -vE '=\$|=["'"'"']\$|=["'"'"']{2}|:-|placeholder|example|EXAMPLE|\bfake\b|redact' || true
)

# --- 3. curl-pipe-shell remote execution -----------------------------------
#
# Scoped to EXECUTABLE surfaces. Bubbles' own installation guide documents a
# `curl … | bash` one-liner — that is the product's published interface, and
# flagging it would be flagging the thing the user deliberately chose to run.
# The risk this catches is a SCRIPT silently fetching and executing remote code
# during a run the operator did not authorise.
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  case "$hit" in
    *security-gate*) continue ;;
  esac
  # A line that PRINTS the command is guidance, not execution. install.sh
  # echoes the one-liner so the operator can copy it; that is the opposite of
  # a silent fetch-and-run.
  line_body="${hit#*:*:}"
  case "$line_body" in
    *printf* | *echo*) continue ;;
  esac
  # Documented, operator-initiated exception: `cli.sh upgrade` re-invokes the
  # published installer on purpose. Recorded here rather than silently
  # excluded, so the one place the framework does this stays visible.
  case "$hit" in
    *"cli.sh"*"install.sh"*) continue ;;
  esac
  report "curl-pipe-shell" "$hit"
done < <(
  grep -rnE '(curl|wget)[^|]*\|[[:space:]]*(sudo[[:space:]]+)?(ba)?sh\b' \
    --include='*.sh' --include='*.yml' --include='*.yaml' . 2>/dev/null |
    grep -vE '^\s*[^:]+:[0-9]+:\s*#' || true
)

# --- 4. World-writable tracked files ---------------------------------------
for f in "${tracked[@]}"; do
  [[ -f "$f" ]] || continue
  perms="$(ls -l "$f" 2>/dev/null | cut -c1-10)"
  case "$perms" in
    *w-) report "world-writable" "$f is world-writable ($perms)" ;;
    *w*) [[ "${perms:8:1}" == "w" ]] && report "world-writable" "$f is world-writable ($perms)" ;;
  esac
done

# --- 5. eval on unsanitised substitution ------------------------------------
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  case "$hit" in
    *security-gate*) continue ;;
  esac
  report "eval-on-input" "$hit"
done < <(
  grep -rnE '^[^#]*\beval[[:space:]]+["'"'"']?\$\(' --include='*.sh' . 2>/dev/null || true
)

if [[ "$findings" -gt 0 ]]; then
  echo "[security-gate] FAIL — G034 findings: $findings"
  exit 1
fi

[[ "$quiet" -eq 1 ]] || echo "[security-gate] OK — ${#tracked[@]} tracked file(s), zero G034 findings"
exit 0
