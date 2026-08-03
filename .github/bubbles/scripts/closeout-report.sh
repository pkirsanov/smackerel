#!/usr/bin/env bash
# closeout-report.sh (IMP-033 / SCOPE-4 — gap WIP-3)
# ---------------------------------------------------------------------------
# The session-boundary reconciliation report: "what did this session leave
# behind in this repository, who owns the next move on each piece of it, and
# which of those pieces can be resolved safely right now".
#
# WHY THIS IS A SEPARATE COMMAND FROM `open-work`
# Recording open work and cleaning local state are different verbs with
# different risk profiles. `open-work` is pure reporting and safe to run
# constantly. `closeout` PROPOSES MUTATIONS and therefore needs a confirmation
# step. Fusing them would force the read-only case through the dangerous one's
# safety contract. `closeout` calls `open-work` and prints its result, so an
# operator who wants both still types one command.
#
# WHAT IT REPORTS, IN ORDER
#   1. The worktree/remote hygiene snapshot (worktree-hygiene-report.sh).
#   2. A disposition per non-trunk local branch and per stash: `merge-able`,
#      `has-unique-commits`, `dirty`, or `lease-held`.
#   3. Unrecorded residue — changed paths that map to no open spec, bug, or
#      improvement, offered as proposed `residue` rows for the register.
#   4. The exact commands the operator would run, PRINTED AND NOT EXECUTED.
#   5. The open-work register (open-work-report.sh), so the two halves of the
#      session boundary appear in one place.
#   6. One bounded invocation line per OTHER host-declared workspace root
#      (IMP-033 SCOPE-7), read from BUBBLES_WORKSPACE_ROOTS. Printed only — no
#      cross-root state is inspected, which is what keeps the one-repository-
#      per-command binding contract intact rather than merely well-intentioned.
#      Absent entirely when the host declared no other roots.
#
# SAFETY CONTRACT (IMP-033 SCOPE-4, mechanically asserted by the selftest)
#   * Report-only is the DEFAULT. `--apply` is the ONLY execution mechanism.
#     `--dry-run` is accepted as an explicit no-op synonym for the default so a
#     cautious operator can state the intent, but it is NOT the mechanism —
#     absence of `--apply` is. Naming both without saying which one governs is
#     how an implementer ends up building a third state.
#   * `--apply` refuses PER ITEM on unmerged branches, dirty worktrees, and any
#     stash. It never drops, never resets, never rebases, never force-pushes.
#     A refusal is printed with its remediation, because a situation this
#     command will not resolve is an operator decision.
#   * There is NO `--force`, NO `--skip`, and NO `--ignore` at any layer.
#   * `--apply` honours the IMP-023 writer-lease, RE-CHECKED AT ACTION TIME.
#     A second session can be working in this repository right now. The window
#     between reporting and applying is exactly where a concurrent session
#     lands a commit, so the snapshot the report opened with is not authority.
#     This mirrors worktree-reap.sh's still_lease_held() rather than inventing
#     a second lease reader. Report mode is unaffected: reading state under a
#     live lease is safe.
#
# WHAT `--apply` ACTUALLY EXECUTES — the provably safe subset only:
#   (a) `git branch -d` on a fully-merged non-trunk local branch that is not
#       checked out anywhere and is not covered by a live lease. `-d` is git's
#       own safe delete: git itself refuses a branch with unique commits, so
#       the safety does not rest on this script's classification being right.
#       `-D` is never used.
#   (b) Archiving the active session to .specify/memory/sessions/<id>.json,
#       which trajectory-inspector.sh already reads and which nothing else
#       currently writes.
#   Worktree removal is NOT reimplemented here. worktree-reap.sh already owns
#   that operation with this same safety contract; closeout prints its command.
#
# Args:
#   --repo-root <path>   Repository to reconcile (default: cwd walked upward)
#   --apply              Execute the provably safe subset (see above)
#   --dry-run            Explicit no-op synonym for the default; not a mechanism
#   -h | --help          Usage
#
# Exit codes: 0 report printed (or applied) | 2 usage error.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LEASES_SH="$SCRIPT_DIR/runtime-leases.sh"

REPO_ROOT=""
APPLY=false

usage() {
  sed -n '2,64p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      # An EMPTY value is a usage error, never a silent fall-through to the
      # walk-upward default. A caller that passes an unset variable here means
      # to name a repository; binding to whatever repository the process
      # happens to be standing in instead would point a mutating command at the
      # wrong checkout, which is the failure this command exists to prevent.
      REPO_ROOT="${2:-}"
      [[ -n "$REPO_ROOT" ]] || { echo "closeout: --repo-root requires a non-empty path" >&2; exit 2; }
      shift 2 ;;
    --apply)     APPLY=true; shift ;;
    --dry-run)   shift ;;
    -h|--help)   usage; exit 0 ;;
    *) echo "closeout: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# --- repo root resolution (same walk-upward rule as the sibling reports) ------
if [[ -z "$REPO_ROOT" ]]; then
  d="$PWD"
  while [[ "$d" != "/" ]]; do
    if [[ -d "$d/.specify/memory" || -d "$d/.git" ]]; then REPO_ROOT="$d"; break; fi
    d="$(dirname "$d")"
  done
  [[ -n "$REPO_ROOT" ]] || REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi
[[ -d "$REPO_ROOT" ]] || { echo "closeout: repo root not found: $REPO_ROOT" >&2; exit 2; }
REPO_ROOT="$(cd "$REPO_ROOT" && pwd -P)"

if ! git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  echo "Session closeout — $REPO_ROOT"
  echo "  not a git repository; nothing to reconcile."
  exit 0
fi

TRUNK="main"
git -C "$REPO_ROOT" show-ref --verify --quiet refs/heads/main || TRUNK="master"

# --- action-time safety re-checks (mirrors worktree-reap.sh) -----------------
still_lease_held() {
  local p="$1" active
  [[ -f "$p/.specify/runtime/resource-leases.json" ]] || return 1
  [[ -x "$LEASES_SH" ]] || return 1
  active="$(BUBBLES_REPO_ROOT="$p" bash "$LEASES_SH" summary 2>/dev/null | sed -nE 's/.*active=([0-9]+).*/\1/p' | head -1)"
  [[ "$active" =~ ^[0-9]+$ ]] || active=0
  [[ "$active" -gt 0 ]]
}

still_dirty() {
  local p="$1" n
  [[ -d "$p" ]] || return 1
  n="$(git -C "$p" status --porcelain 2>/dev/null | sed '/^$/d' | wc -l | tr -d ' ')"
  [[ "$n" =~ ^[0-9]+$ ]] || n=0
  [[ "$n" -gt 0 ]]
}

# Branch names that are checked out in SOME worktree — deleting one of these is
# not merely unsafe, git refuses it, and reporting it as reapable would be a lie.
checked_out_branches() {
  git -C "$REPO_ROOT" worktree list --porcelain 2>/dev/null \
    | sed -nE 's|^branch refs/heads/(.*)$|\1|p'
}

echo "Session closeout — $REPO_ROOT (trunk: $TRUNK)"
echo "Report only. Nothing below is executed unless --apply is passed."
[[ "$APPLY" == true ]] && echo "MODE: --apply — the provably safe subset WILL be executed."
echo

# --- 1. hygiene snapshot -----------------------------------------------------
echo "1. Worktree and remote hygiene"
hygiene_sh="$SCRIPT_DIR/worktree-hygiene-report.sh"
if [[ -f "$hygiene_sh" ]]; then
  BUBBLES_REPO_ROOT="$REPO_ROOT" bash "$hygiene_sh" 2>/dev/null \
    | grep -E '^worktree-hygiene' | sed 's/^/  /'
else
  echo "  worktree-hygiene-report.sh not found; hygiene snapshot unavailable."
fi
echo

# --- 2. disposition per branch and stash -------------------------------------
echo "2. Disposition proposal"
merged_list="$(git -C "$REPO_ROOT" branch --merged "$TRUNK" --format='%(refname:short)' 2>/dev/null)"
co_list="$(checked_out_branches)"

n_branches=0
n_reapable=0
reapable_branches=""
while IFS= read -r br; do
  [[ -n "$br" ]] || continue
  [[ "$br" == "$TRUNK" ]] && continue
  n_branches=$((n_branches + 1))

  disposition="has-unique-commits"
  printf '%s\n' "$merged_list" | grep -Fxq "$br" && disposition="merge-able"

  wt_path=""
  if printf '%s\n' "$co_list" | grep -Fxq "$br"; then
    wt_path="$(git -C "$REPO_ROOT" worktree list --porcelain 2>/dev/null \
      | awk -v b="branch refs/heads/$br" '/^worktree /{p=substr($0,10)} $0==b{print p; exit}')"
    if still_lease_held "$wt_path"; then
      disposition="lease-held"
    elif still_dirty "$wt_path"; then
      disposition="dirty"
    else
      disposition="checked-out"
    fi
  fi

  case "$disposition" in
    merge-able)
      n_reapable=$((n_reapable + 1))
      reapable_branches="${reapable_branches}${br}"$'\n'
      echo "  merge-able          $br — fully merged into $TRUNK; safe to delete"
      ;;
    has-unique-commits)
      echo "  has-unique-commits  $br — NOT merged into $TRUNK; closeout will not delete it"
      echo "                      remediation: merge or open a PR, or delete it yourself once you are certain"
      ;;
    lease-held)
      echo "  lease-held          $br — a live writer-lease covers $wt_path; left intact"
      ;;
    dirty)
      echo "  dirty               $br — uncommitted changes in $wt_path; commit or discard them yourself"
      ;;
    checked-out)
      echo "  checked-out         $br — checked out at $wt_path; git refuses to delete a checked-out branch"
      ;;
  esac
done < <(git -C "$REPO_ROOT" for-each-ref --format='%(refname:short)' refs/heads 2>/dev/null)
[[ "$n_branches" -eq 0 ]] && echo "  no non-trunk local branches."

n_stashes="$(git -C "$REPO_ROOT" stash list 2>/dev/null | sed '/^$/d' | wc -l | tr -d ' ')"
[[ "$n_stashes" =~ ^[0-9]+$ ]] || n_stashes=0
if [[ "$n_stashes" -gt 0 ]]; then
  echo "  stash               $n_stashes entr$([[ "$n_stashes" -eq 1 ]] && echo y || echo ies) — closeout NEVER drops a stash"
  echo "                      remediation: 'git stash list' then apply or drop each one deliberately"
fi
echo

# --- 3. unrecorded residue ---------------------------------------------------
echo "3. Unrecorded residue (changed paths that map to no open artifact)"
open_json=""
open_work_sh="$SCRIPT_DIR/open-work-report.sh"
if [[ -f "$open_work_sh" ]] && command -v jq >/dev/null 2>&1; then
  open_json="$(bash "$open_work_sh" --repo-root "$REPO_ROOT" --format json 2>/dev/null)"
fi

open_refs=""
if [[ -n "$open_json" ]]; then
  open_refs="$(printf '%s' "$open_json" | jq -r '.items[]? | .ref' 2>/dev/null | sed 's|/[^/]*$||' | sed '/^$/d' | LC_ALL=C sort -u)"
fi

n_residue=0
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  p="${line:3}"
  accounted=false
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    case "$p" in "$ref"/*|"$ref") accounted=true; break ;; esac
  done <<< "$open_refs"
  if [[ "$accounted" == false ]]; then
    n_residue=$((n_residue + 1))
    echo "  | residue-$(printf '%02d' "$n_residue") | <one line: what this change is for> | residue | $p | open | <who> | <the next concrete move> | $(date +%Y-%m-%d) | $(date +%Y-%m-%d) |"
  fi
done < <(git -C "$REPO_ROOT" status --porcelain 2>/dev/null)

if [[ "$n_residue" -eq 0 ]]; then
  echo "  none — every changed path maps to an open artifact."
else
  echo
  echo "  Paste the rows you still care about into .specify/memory/open-work.md and fill in"
  echo "  the owner and the next action. A row with a placeholder in either field fails"
  echo "  'cli.sh open-work --lint', which is the point: \"finish the thing\" is not a next action."
fi
echo

# --- 4. the commands, printed not executed -----------------------------------
echo "4. Commands (printed, not executed)"
if [[ "$n_reapable" -gt 0 ]]; then
  while IFS= read -r br; do
    [[ -n "$br" ]] || continue
    echo "  git -C $REPO_ROOT branch -d $br"
  done <<< "$reapable_branches"
else
  echo "  (no branch is safely deletable right now)"
fi
echo "  bash bubbles/scripts/worktree-reap.sh --yes        # merged/prunable worktrees, same safety contract"
echo "  bash bubbles/scripts/cli.sh open-work --lint       # verify the register before you walk away"
echo

# --- apply: the provably safe subset only ------------------------------------
if [[ "$APPLY" == true ]]; then
  echo "Applying the provably safe subset"
  if [[ "$n_reapable" -eq 0 ]]; then
    echo "  nothing to apply."
  fi
  while IFS= read -r br; do
    [[ -n "$br" ]] || continue
    # Re-check at ACTION TIME. The report above is a snapshot, and the window
    # between reporting and applying is exactly where a concurrent session
    # lands a commit or takes a lease.
    if still_lease_held "$REPO_ROOT"; then
      echo "  SKIP  $br — a writer-lease became active on $REPO_ROOT; left intact"
      continue
    fi
    if printf '%s\n' "$(checked_out_branches)" | grep -Fxq "$br"; then
      echo "  SKIP  $br — became checked out; left intact"
      continue
    fi
    if git -C "$REPO_ROOT" branch -d "$br" >/dev/null 2>&1; then
      echo "  deleted branch $br"
    else
      # git itself declined — it saw unique commits this snapshot did not.
      echo "  SKIP  $br — git declined the safe delete; left intact"
    fi
  done <<< "$reapable_branches"

  # Archive the active session where trajectory-inspector.sh already looks.
  session_file="$REPO_ROOT/.specify/memory/bubbles.session.json"
  if [[ -f "$session_file" ]] && command -v jq >/dev/null 2>&1; then
    sid="$(jq -r '.sessionId // empty' "$session_file" 2>/dev/null)"
    if [[ -n "$sid" ]]; then
      mkdir -p "$REPO_ROOT/.specify/memory/sessions"
      archive="$REPO_ROOT/.specify/memory/sessions/${sid}.json"
      if jq --arg t "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '. + {closedOutAt: $t}' \
           "$session_file" > "$archive.tmp" 2>/dev/null; then
        mv "$archive.tmp" "$archive"
        echo "  archived session $sid to .specify/memory/sessions/${sid}.json"
      else
        rm -f "$archive.tmp"
        echo "  session archive skipped — could not read $session_file"
      fi
    fi
  fi
  echo
fi

# --- 5. open work ------------------------------------------------------------
echo "5. Open work"
if [[ -f "$open_work_sh" ]]; then
  bash "$open_work_sh" --repo-root "$REPO_ROOT" 2>/dev/null | sed 's/^/  /'
else
  echo "  open-work-report.sh not found at $open_work_sh"
fi

# --- 6. the other declared roots ---------------------------------------------
#
# IMP-033 / SCOPE-7. The operator's real workspace spans several repositories,
# but the repository-binding contract is deliberately one repository per
# command. Sweeping all of them from here would be the easy answer and the
# wrong one: it would make a MUTATING command's blast radius depend on host
# workspace configuration rather than on an argument the operator typed.
#
# So this prints, and only prints, the exact invocation for each of the OTHER
# declared roots. The operator runs N bounded closeouts instead of one
# unbounded sweep. NO cross-root state is read here — not a git status, not a
# register, not a branch list. The roots are echoed back from what the host
# declared and nothing else, which is what keeps the one-repository-per-command
# binding contract intact rather than merely well-intentioned.
if [[ -n "${BUBBLES_WORKSPACE_ROOTS:-}" ]]; then
  other_roots=""
  while IFS= read -r ws_root; do
    [[ -n "$ws_root" ]] || continue
    # Compare canonically so a symlinked or trailing-slash duplicate of the
    # current repo is not offered back to the operator as a separate one.
    ws_canon="$(cd "$ws_root" 2>/dev/null && pwd -P || printf '%s' "$ws_root")"
    [[ "$ws_canon" != "$REPO_ROOT" ]] || continue
    other_roots="${other_roots}${other_roots:+$'\n'}$ws_canon"
  done <<< "${BUBBLES_WORKSPACE_ROOTS//:/$'\n'}"

  if [[ -n "$other_roots" ]]; then
    echo
    echo "6. Other declared roots (not inspected — one repository per command)"
    while IFS= read -r ws_root; do
      [[ -n "$ws_root" ]] || continue
      printf '  bash bubbles/scripts/cli.sh closeout --repo-root %s\n' "$ws_root"
    done <<< "$other_roots"
  fi
fi

exit 0
