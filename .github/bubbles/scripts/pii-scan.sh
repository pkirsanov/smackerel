#!/usr/bin/env bash
# pii-scan.sh — generic PII & secret scan for any Bubbles-equipped repo.
#
# WHAT IT DOES
#   1. Runs `gitleaks protect --staged` against the repo's `.gitleaks.toml`
#      (catches new secrets/PII in the staged diff — fast, hook-friendly).
#   2. If a machine-local token file exists at $BUBBLES_PII_TOKENS (default
#      ~/.config/bubbles/pii-tokens.txt), greps the staged diff against
#      every token in that file (one literal token per line, blank/# lines
#      skipped). Catches owner-specific identifiers (real hostname, real
#      personal name, etc.) that MUST NEVER appear in any committed text
#      but cannot be encoded in a generic regex without leaking the values
#      into the rule itself.
#
# WHAT IT IS NOT
#   This script contains ZERO project-specific values, ZERO personal data,
#   and ZERO secret patterns. It is portable: every Bubbles-using repo gets
#   the same script, and every consumer gets the same behavior.
#
# CONFIGURATION
#   .gitleaks.toml                 — required at repo root; defines patterns.
#   .gitleaksignore                — optional; per-fingerprint baseline.
#   ~/.config/bubbles/pii-tokens.txt — optional; one literal token per line.
#                                    Override with $BUBBLES_PII_TOKENS.
#
# BYPASS (use only in emergencies; commit message MUST justify)
#   SKIP_PII_SCAN=1 git commit ...
#
# EXIT CODES
#   0  — clean (no findings)
#   1  — findings detected (commit blocked)
#   2  — gitleaks not installed (commit blocked; see install hint)
#   3  — no .gitleaks.toml at repo root (commit blocked; misconfigured repo)

set -uo pipefail

if [[ "${SKIP_PII_SCAN:-0}" == "1" ]]; then
  echo "🫧 pii-scan: SKIP_PII_SCAN=1 — bypassing PII/secret scan." >&2
  echo "🫧 pii-scan: bypass MUST be justified in the commit message." >&2
  exit 0
fi

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)"
if [[ -z "$repo_root" ]]; then
  echo "❌ pii-scan: not inside a git repo." >&2
  exit 1
fi
cd "$repo_root"

# 1. Sanity: gitleaks must be installed.
if ! command -v gitleaks >/dev/null 2>&1; then
  cat >&2 <<'EOF'
❌ pii-scan: `gitleaks` not found in PATH.

Install (Linux/macOS):
  go install github.com/gitleaks/gitleaks/v8@latest
  export PATH="$HOME/go/bin:$PATH"

Or via Homebrew:
  brew install gitleaks

Or via package manager:
  apt install gitleaks   # debian/ubuntu (may be old version)

After install, retry the commit. To bypass in an emergency:
  SKIP_PII_SCAN=1 git commit ...   # MUST justify in commit message.
EOF
  exit 2
fi

# 2. Sanity: .gitleaks.toml must exist at repo root.
if [[ ! -f "$repo_root/.gitleaks.toml" ]]; then
  cat >&2 <<EOF
❌ pii-scan: no .gitleaks.toml at $repo_root.

This repo has not been configured for PII/secret prevention. Install the
canonical Bubbles config:

  cp \$BUBBLES_SOURCE/.gitleaks.toml $repo_root/.gitleaks.toml

Or fetch from the Bubbles framework distribution. Then retry the commit.
EOF
  exit 3
fi

# 3. Run gitleaks against staged content only.
#    Subcommand selection: gitleaks v8.18+ uses `gitleaks git --staged ...`,
#    older versions used `gitleaks protect --staged ...`. Detect which one the
#    installed binary supports so this script works across both eras without
#    requiring downstream consumers to pin a specific gitleaks version.
if gitleaks --help 2>&1 | grep -qE '^[[:space:]]+protect[[:space:]]'; then
  gitleaks_args=(
    protect
    --staged
    --config .gitleaks.toml
    --no-banner
    --redact
  )
else
  gitleaks_args=(
    git
    --staged
    --config .gitleaks.toml
    --no-banner
    --redact
  )
fi
if [[ -f "$repo_root/.gitleaksignore" ]]; then
  : # gitleaks auto-loads .gitleaksignore; no flag needed.
fi

if ! gitleaks "${gitleaks_args[@]}"; then
  cat >&2 <<'EOF'

❌ pii-scan: gitleaks detected secret(s) or PII in staged content.

WHAT TO DO
  1. Replace the secret/PII with a placeholder, env var, or template
     reference (e.g. ${MY_VAR}, <YOUR-VALUE>, example.test).
  2. If the finding is a false positive, add an inline allowlist comment
     `# gitleaks:allow` at end of line, OR add the fingerprint to
     .gitleaksignore (after committee review).
  3. Restage and retry: `git add -u && git commit ...`

EMERGENCY BYPASS (use sparingly, justify in commit message):
  SKIP_PII_SCAN=1 git commit ...

EOF
  exit 1
fi

# 4. Optional: machine-local token scan (catches owner-specific identifiers
#    that cannot be encoded in a portable rule without re-leaking them).
local_tokens="${BUBBLES_PII_TOKENS:-$HOME/.config/bubbles/pii-tokens.txt}"
if [[ -f "$local_tokens" && -r "$local_tokens" ]]; then
  # Build a temp filter file with comments/blanks stripped.
  filter_file="$(mktemp -t bubbles-pii-filter.XXXXXX)"
  trap 'rm -f "$filter_file"' EXIT
  grep -vE '^[[:space:]]*(#|$)' "$local_tokens" > "$filter_file"

  if [[ -s "$filter_file" ]]; then
    # Diff staged content (excluding deletions) and grep for any token.
    staged_diff="$(git diff --cached --no-color -U0 \
      -- ':!.gitleaks.toml' ':!.gitleaksignore' ':!.config/bubbles/' 2>/dev/null \
      | grep -E '^\+' \
      | grep -v '^+++' || true)"

    if [[ -n "$staged_diff" ]]; then
      # Case-insensitive literal-string match against the filter list.
      if echo "$staged_diff" | grep -F -i -f "$filter_file" -q; then
        cat >&2 <<EOF

❌ pii-scan: machine-local PII token detected in staged diff.

A token from $local_tokens appears in your staged changes. These are
owner-specific identifiers (e.g. real hostname, real personal name) that
MUST NEVER be committed.

WHAT TO DO
  1. Replace the value with a placeholder (e.g. <YOUR-DEVICE>, \${HOSTNAME}).
  2. Restage: \`git add -u && git commit ...\`

The token list is machine-local and never leaves this machine. It is NOT
a substitute for the .gitleaks.toml rules — it is a defense-in-depth
catch for values that cannot be expressed as a generic regex.

EMERGENCY BYPASS (use sparingly):
  SKIP_PII_SCAN=1 git commit ...

EOF
        exit 1
      fi
    fi
  fi
fi

echo "🫧 pii-scan: clean."
exit 0
