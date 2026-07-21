#!/usr/bin/env bash
# Isolated Design-Experiment Guard (IMP-100 Phase 4 / IMP-026 SCOPE-8)
# ---------------------------------------------------------------------------
# A design-experiment is a DISPOSABLE worktree for throwaway exploration (a
# spike / proof-of-concept / "what if" probe). Its purpose is LEARNING, not
# delivery: its outputs can NEVER satisfy DoD, tests, integration, or
# certification, and the worktree is DELETED after its findings are captured
# into the durable spec/design. This guard mechanically REFUSES a
# `.design-experiment`-marked worktree that has leaked completion/certification
# state into durable artifacts, so throwaway work can never masquerade as done.
#
# A worktree is a design-experiment IFF a `.design-experiment` marker file
# exists at <worktree>. Absent the marker this guard is a no-op (exit 0) — it
# never touches a normal delivery worktree.
#
# REFUSE (exit 1) when a marked worktree contains ANY of:
#   - a state.json with a terminal certification/top-level status
#     (done | delivered_fast | delivered_prototype | specs_hardened)
#   - a state.json with a non-empty completedScopes array
#   - a checked DoD item (`- [x]`) in any scope.md / scopes.md
# PASS (exit 0) for a clean exploration or an unmarked directory.
# Exit 2 = usage error (missing --worktree or path not found).
#
# Advisory-until-adopted: a workflow MAY invoke this before merging or
# certifying. There is NO bypass flag — a design-experiment becomes deliverable
# only by being re-planned as a normal scope, never by skipping this check.
# Uses only grep/find (no jq/yq dependency) so it runs identically on WSL+macOS.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: design-experiment-guard.sh --worktree <dir>

A `.design-experiment`-marked worktree MUST NOT leak completion/certification
state into durable artifacts. REFUSES (exit 1) on a terminal certification/top-
level status, a non-empty completedScopes, or a checked DoD item (`- [x]`).
PASS (exit 0) for a clean exploration or an unmarked directory. Exit 2 = usage.
No bypass flag.
EOF
}

worktree=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --worktree) worktree="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) echo "design-experiment-guard: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$worktree" ]]; then
  echo "design-experiment-guard: --worktree is required" >&2
  usage >&2
  exit 2
fi
if [[ ! -d "$worktree" ]]; then
  echo "design-experiment-guard: worktree not found: $worktree" >&2
  exit 2
fi

# Not a design-experiment (no marker) → no-op.
if [[ ! -f "$worktree/.design-experiment" ]]; then
  echo "[design-experiment-guard] OK — no .design-experiment marker; not a design-experiment (no-op)."
  exit 0
fi

leaks=()

# 1) Terminal certification/top-level status + non-empty completedScopes in any state.json.
while IFS= read -r sf; do
  [[ -n "$sf" ]] || continue
  if grep -qE '"status"[[:space:]]*:[[:space:]]*"(done|delivered_fast|delivered_prototype|specs_hardened)"' "$sf" 2>/dev/null; then
    leaks+=("terminal status in ${sf#"$worktree"/}")
  fi
  # completedScopes with at least one entry (a quoted string inside the array).
  if grep -qE '"completedScopes"[[:space:]]*:[[:space:]]*\[[[:space:]]*"' "$sf" 2>/dev/null; then
    leaks+=("non-empty completedScopes in ${sf#"$worktree"/}")
  fi
done < <(find "$worktree" -type f -name 'state.json' -not -path '*/.git/*' 2>/dev/null)

# 2) Checked DoD items in any scope.md / scopes.md.
while IFS= read -r df; do
  [[ -n "$df" ]] || continue
  if grep -qiE '^[[:space:]]*-[[:space:]]*\[x\]' "$df" 2>/dev/null; then
    leaks+=("checked DoD item in ${df#"$worktree"/}")
  fi
done < <(find "$worktree" -type f \( -name 'scope.md' -o -name 'scopes.md' \) -not -path '*/.git/*' 2>/dev/null)

if [[ "${#leaks[@]}" -gt 0 ]]; then
  echo "[design-experiment-guard] REFUSED: a .design-experiment worktree leaked completion/certification state:" >&2
  for l in "${leaks[@]}"; do
    echo "  - $l" >&2
  done
  echo "  A design-experiment is throwaway learning — it can never satisfy DoD/test/certification." >&2
  echo "  Capture its findings into the durable spec/design, delete the worktree, and re-plan real work as a normal scope." >&2
  exit 1
fi

echo "[design-experiment-guard] OK — .design-experiment worktree is a clean exploration (no DoD/certification leakage)."
exit 0
