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
# ── --lingering mode (IMP-107 SCOPE-3, gap WT-EXPERIMENT-LINGER) ────────────
# The default mode above enforces the "no completion/certification LEAKAGE" half
# of the design-experiment contract. `--lingering` enforces the OTHER half —
# "capture then DELETE": a `.design-experiment`-marked worktree that STILL
# EXISTS after its run should have finalized is itself a finding (the mandated
# post-capture removal never happened), UNLESS a live IMP-023 writer-lease
# covers it (a lease-held experiment is still LIVE, not lingering — it reuses
# runtime-leases.sh exactly as worktree-reap.sh does):
#     design-experiment-guard.sh --lingering --worktree <dir>
# CONTRACT: advisory-first — it PRINTS the LINGERING finding and exits 0 by
# default so the SCOPE-1 hygiene report / reaper / doctor advisory can consume
# it without a hard gate. Pass `--strict` to make a lingering experiment a hard
# failure (exit 1) for a finalize/CI step. No marker -> no-op exit 0 (as always).
# The default (non-`--lingering`) leakage-REFUSE behavior is byte-unchanged.
#
# Advisory-until-adopted: a workflow MAY invoke this before merging or
# certifying. There is NO bypass flag — a design-experiment becomes deliverable
# only by being re-planned as a normal scope, never by skipping this check.
# Uses only grep/find (no jq/yq dependency) so it runs identically on WSL+macOS.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LEASES_SH="$SCRIPT_DIR/runtime-leases.sh"

usage() {
  cat <<'EOF'
Usage: design-experiment-guard.sh --worktree <dir> [--lingering [--strict]]

Default mode (no --lingering) — the no-LEAKAGE half of the design-experiment
contract. A `.design-experiment`-marked worktree MUST NOT leak completion/
certification state into durable artifacts. REFUSES (exit 1) on a terminal
certification/top-level status, a non-empty completedScopes, or a checked DoD
item (`- [x]`). PASS (exit 0) for a clean exploration or an unmarked directory.

--lingering mode (IMP-107 SCOPE-3) — the capture-then-DELETE half. Reports a
LINGERING finding when the marker is present (worktree not yet deleted) AND no
live writer-lease covers it. Advisory: PRINTS the finding and exits 0 by
default; add --strict to exit 1 on a lingering experiment. No marker -> no-op
exit 0. Reuses runtime-leases.sh to skip a LEASE-HELD (still-live) experiment.

Exit 2 = usage error (missing --worktree or path not found). No bypass flag.
EOF
}

# 0 (true) iff a live IMP-023 writer-lease covers the worktree. Reuses
# runtime-leases.sh's own effective status (its `summary` reports active=N) and
# only probes when the worktree already owns a lease registry file, so this
# stays READ-ONLY — identical to worktree-hygiene-report.sh / worktree-reap.sh.
lease_held() {
  local p="$1" active
  [[ -f "$p/.specify/runtime/resource-leases.json" ]] || return 1
  [[ -x "$LEASES_SH" ]] || return 1
  active="$(BUBBLES_REPO_ROOT="$p" bash "$LEASES_SH" summary 2>/dev/null | sed -nE 's/.*active=([0-9]+).*/\1/p' | head -1 || true)"
  [[ "$active" =~ ^[0-9]+$ ]] || active=0
  [[ "$active" -gt 0 ]]
}

worktree=""
lingering=false
strict=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --worktree) worktree="${2:-}"; shift 2 ;;
    --lingering) lingering=true; shift ;;
    --strict) strict=true; shift ;;
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

# Not a design-experiment (no marker) → no-op (both modes).
if [[ ! -f "$worktree/.design-experiment" ]]; then
  echo "[design-experiment-guard] OK — no .design-experiment marker; not a design-experiment (no-op)."
  exit 0
fi

# --lingering (IMP-107 SCOPE-3): the "capture then DELETE" half. A marked
# worktree that STILL EXISTS after its run finalized is itself a finding —
# UNLESS a live writer-lease covers it (still live, not lingering). This mode
# never inspects durable-artifact leakage; that is the default mode's job.
if [[ "$lingering" == true ]]; then
  if lease_held "$worktree"; then
    echo "[design-experiment-guard] OK — .design-experiment worktree is LEASE-HELD (live run); not lingering."
    exit 0
  fi
  echo "[design-experiment-guard] LINGERING: a .design-experiment worktree still exists after its run should have finalized:"
  echo "  - $worktree (.design-experiment marker present; no live writer-lease)"
  echo "  A design-experiment is disposable by construction: capture its findings into the durable spec/design,"
  echo "  then DELETE the worktree + its branch. Reap it with:"
  echo "    worktree-reap.sh --experiments --yes   (or: cli.sh doctor --heal)"
  if [[ "$strict" == true ]]; then
    exit 1
  fi
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
