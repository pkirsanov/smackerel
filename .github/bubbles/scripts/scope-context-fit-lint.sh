#!/usr/bin/env bash
# Scope Context-Fit Lint (IMP-100 Phase 4 / IMP-026 SCOPE-6 — contextFit)
# ---------------------------------------------------------------------------
# Extends the G037 scope-size-discipline dimension with CONTEXT FIT: a scope must
# be executable by a FRESH specialist from the durable artifacts alone (spec.md /
# design.md / scope body / referenced files), WITHOUT replaying the chat/session
# that produced it. It is NOT a token-count check — it detects the clear
# anti-pattern where a scope's instructions depend on ephemeral conversation
# context ("as discussed above", "per our chat", "earlier in this session", …),
# which a fresh single-specialist context cannot recover.
#
# Reuse-first: this mechanizes the EXISTING single-specialist-context expectation
# (a scope is a durable, self-contained unit of work) rather than inventing a new
# planning concept.
#
# ADVISORY by default (prints a warning, exits 0) so it never blocks an existing
# repo. It exits non-zero ONLY when a project opts in via
# `.github/bubbles-project.yaml` (`scopeContextFitGuard: block`), matching the
# advisory-until-configured posture of vertical-delivery-plan-guard. There is no
# `--skip`/`--force` bypass. Detection is conservative (a curated set of
# unambiguous chat-replay phrases), so ordinary requirement language
# ("the user asked for", "the operator selects") never false-positives.
#
# Usage:
#   bash bubbles/scripts/scope-context-fit-lint.sh <feature-dir>
#
# Exit codes:
#   0  clean, or advisory finding (default posture)
#   1  a context-fit violation under block posture
#   2  usage / runtime error
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scope-context-fit-lint.sh <feature-dir>

Flags any scope whose body depends on ephemeral chat/session context (e.g.
"as discussed above", "per our chat", "earlier in this session") instead of the
durable artifacts a fresh specialist can read. Advisory (exit 0 + warning) by
default; blocks (exit 1) only when .github/bubbles-project.yaml sets
scopeContextFitGuard: block.
EOF
}

feature_dir="${1:-}"
if [[ -z "$feature_dir" ]]; then
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "scope-context-fit-lint: feature dir not found: $feature_dir" >&2
  exit 2
fi

# ---------------------------------------------------------------------------
# Resolve enforcement mode (advisory | block). Advisory unless the repo opts in.
# ---------------------------------------------------------------------------
mode="advisory"
project_config=""
for candidate in \
  "$feature_dir/.github/bubbles-project.yaml" \
  ".github/bubbles-project.yaml" \
  "$(git -C "$feature_dir" rev-parse --show-toplevel 2>/dev/null)/.github/bubbles-project.yaml"; do
  if [[ -n "$candidate" && -f "$candidate" ]]; then
    project_config="$candidate"
    break
  fi
done
if [[ -n "$project_config" ]] && grep -qE '^[[:space:]]*scopeContextFitGuard:[[:space:]]*block[[:space:]]*$' "$project_config"; then
  mode="block"
fi

# ---------------------------------------------------------------------------
# Collect ordered scope bodies. Supports both layouts:
#   1. single-file  scopes.md   (split on `^## Scope ` headers)
#   2. per-scope dir scopes/NN-*/scope.md  (ordered by directory name)
# ---------------------------------------------------------------------------
scope_bodies_dir="$(mktemp -d)"
trap 'rm -rf "$scope_bodies_dir"' EXIT INT TERM
scope_count=0

emit_scope() {
  local name="$1" body_file="$2"
  scope_count=$((scope_count + 1))
  printf '%s\n' "$name" >> "$scope_bodies_dir/names"
  cp "$body_file" "$scope_bodies_dir/scope-$scope_count.body"
}

if [[ -f "$feature_dir/scopes.md" ]]; then
  current_body="$(mktemp)"
  current_name=""
  in_scope=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^##[[:space:]]+Scope[[:space:]] ]]; then
      if [[ "$in_scope" -eq 1 ]]; then
        emit_scope "$current_name" "$current_body"
      fi
      current_name="$(printf '%s' "$line" | sed -E 's/^##[[:space:]]+//')"
      : > "$current_body"
      in_scope=1
      continue
    fi
    if [[ "$in_scope" -eq 1 ]]; then
      printf '%s\n' "$line" >> "$current_body"
    fi
  done < "$feature_dir/scopes.md"
  if [[ "$in_scope" -eq 1 ]]; then
    emit_scope "$current_name" "$current_body"
  fi
  rm -f "$current_body"
elif [[ -d "$feature_dir/scopes" ]]; then
  while IFS= read -r scope_md; do
    [[ -n "$scope_md" ]] || continue
    emit_scope "$(basename "$(dirname "$scope_md")")" "$scope_md"
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -name 'scope.md' | LC_ALL=C sort)
fi

if [[ "$scope_count" -eq 0 ]]; then
  echo "[scope-context-fit-lint] no scopes found in $feature_dir — nothing to check"
  exit 0
fi

# ---------------------------------------------------------------------------
# Chat/session-replay dependency signals. Curated to be UNAMBIGUOUS: each phrase
# points at recovering ephemeral conversation context, which a fresh specialist
# cannot do. Ordinary requirement language ("the user asked for X", "the operator
# selects") is deliberately NOT included, so it never false-positives.
# ---------------------------------------------------------------------------
chat_dep_re='(as discussed above|as we discussed|as we agreed (earlier|above)|per our (conversation|chat|discussion)|in the chat|in our conversation|the conversation above|the (above|earlier) discussion|earlier in (this session|the chat|the conversation)|replay the chat|see (the )?chat|per the thread|from the chat history|as noted in the (chat|conversation)|discussed in (the )?(chat|conversation|thread)|as mentioned (above in|in the) (the )?(chat|conversation)|the discussion we had|our earlier (chat|conversation|discussion))'

findings=0
i=0
finding_lines=""
while IFS= read -r name; do
  i=$((i + 1))
  body_file="$scope_bodies_dir/scope-$i.body"
  if match="$(grep -inE "$chat_dep_re" "$body_file" 2>/dev/null | head -n1)"; then
    findings=$((findings + 1))
    phrase="$(printf '%s' "$match" | sed -E 's/^[0-9]+://')"
    finding_lines="${finding_lines}  - scope \"${name}\": depends on chat/session context: ${phrase}"$'\n'
  fi
done < "$scope_bodies_dir/names"

if [[ "$findings" -eq 0 ]]; then
  echo "[scope-context-fit-lint] OK — all $scope_count scope(s) are self-contained (no chat/session-replay dependency); a fresh specialist can execute from the durable artifacts."
  exit 0
fi

{
  echo "[scope-context-fit-lint] CONTEXT-FIT in $feature_dir:"
  echo "  $findings of $scope_count scope(s) depend on ephemeral chat/session context a fresh specialist cannot recover:"
  printf '%s' "$finding_lines"
  echo "  Remediation: rewrite the flagged scope so its instructions are self-contained —"
  echo "  restate the decision inline or reference a DURABLE artifact (spec.md / design.md /"
  echo "  a scenario ID / a file path), not the conversation. See G037 (contextFit dimension)."
} >&2

if [[ "$mode" == "block" ]]; then
  exit 1
fi
echo "[scope-context-fit-lint] advisory only (exit 0). Set scopeContextFitGuard: block in .github/bubbles-project.yaml to enforce." >&2
exit 0
