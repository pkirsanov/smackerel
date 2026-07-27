#!/usr/bin/env bash
# worktree-spawn.sh (IMP-107 / SCOPE-5 — gap WT-HARNESS)
# ---------------------------------------------------------------------------
# The CANONICAL, supported way to create a run/task git worktree — the
# first-class replacement for ad-hoc `git worktree add` when an operator or
# agent parallelizes many concurrent workflows on one repo (the observed
# `<repo>-<role>-<date>` debris pattern). It:
#   1. Runs `git worktree add -b <branch> <path> <base>` off the resolved base.
#   2. Stamps a `.bubbles-worktree` MARKER at the new worktree root recording
#      JSON { runId, mode, baseSha, createdAt, sessionId } — baseSha resolved
#      via `git rev-parse`, createdAt via `date -u +%Y-%m-%dT%H:%M:%SZ`.
#   3. With --experiment, ALSO stamps the existing `.design-experiment` marker
#      (composing with IMP-107 SCOPE-3's disposable-experiment path), so a
#      throwaway probe is BOTH framework-created AND declared disposable.
#
# THE MARKER IS THE SAFE IDENTITY SIGNAL (IMP-107 R2). A `.bubbles-worktree`-
# marked worktree is FRAMEWORK-CREATED; an UNMARKED worktree is HUMAN-OWNED.
# worktree-hygiene-report.sh surfaces this as a `framework-created=yes|no` tag,
# and the SCOPE-1 reaper (worktree-reap.sh) leaves every UNMARKED, un-merged,
# non-prunable worktree REPORT-ONLY — never touched. The marker REINFORCES the
# reaper's existing MERGED/PRUNABLE-only safety core; it is NOT a new reason to
# force-reap an un-merged worktree.
#
# Recommended default is still IN-TREE-ON-`main` (the zero-sprawl pattern the
# clean repos use); spawn an isolated worktree only when genuine isolation is
# needed. Every spawned (marked) worktree has a matching safe reap via
# worktree-reap.sh / `cli.sh doctor --heal`. See docs/recipes/parallel-worktrees.md.
#
# There is NO --skip / --force / bypass flag. Portable to bash 3.2 (macOS) +
# GNU/BSD git; uses only git + POSIX text tools (no jq/yq dependency).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# REPO_ROOT resolution mirrors worktree-hygiene-report.sh / worktree-reap.sh
# exactly (source-tree vs the downstream .github/bubbles/scripts install, with a
# BUBBLES_REPO_ROOT override so the hermetic selftest can point it at a
# synthesized repo).
if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
  REPO_ROOT="$BUBBLES_REPO_ROOT"
elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

usage() {
  cat <<'EOF'
Usage: worktree-spawn.sh --path <dir> --branch <name> [--base <sha|ref>]
                         [--mode <mode>] [--run-id <id>] [--session-id <id>]
                         [--experiment] [--help]

Create a run/task git worktree the SUPPORTED way and stamp a `.bubbles-worktree`
marker { runId, mode, baseSha, createdAt, sessionId } at its root. The marker is
the framework-created identity signal the hygiene report surfaces and the safe
reaper trusts (an UNMARKED worktree is human-owned -> report-only, never reaped).

  --path <dir>        REQUIRED. Directory for the new worktree (created by git).
  --branch <name>     REQUIRED. New branch name to create at <base>.
  --base <sha|ref>    Commit-ish to branch from. Default: HEAD.
  --mode <mode>       Recorded in the marker (e.g. full-delivery). Default: "".
  --run-id <id>       Recorded in the marker. Default: "".
  --session-id <id>   Recorded in the marker. Default: "".
  --experiment        ALSO stamp a `.design-experiment` marker (disposable probe).
  --help              Show this help and exit 0.

Exit: 0 success; 2 usage error / missing required arg / not a git repo;
1 base unresolvable or `git worktree add` failed. No --skip/--force/bypass flag.
EOF
}

# Minimal JSON string escaper (backslash + double-quote). bash 3.2 substitution.
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

# Resolve a possibly-relative path to an absolute one (lexically; the leaf need
# not exist yet — git creates it). No GNU-only readlink -f (macOS-portable).
resolve_abs() {
  local p="$1"
  case "$p" in
    /*) printf '%s' "$p" ;;
    *)  printf '%s/%s' "$(pwd)" "$p" ;;
  esac
}

path_arg=""
branch_arg=""
base_arg=""
mode_arg=""
run_id_arg=""
session_id_arg=""
EXPERIMENT=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --path)
      shift; [[ $# -gt 0 ]] || { echo "worktree-spawn: --path requires a value" >&2; usage >&2; exit 2; }
      path_arg="$1"; shift ;;
    --branch)
      shift; [[ $# -gt 0 ]] || { echo "worktree-spawn: --branch requires a value" >&2; usage >&2; exit 2; }
      branch_arg="$1"; shift ;;
    --base)
      shift; [[ $# -gt 0 ]] || { echo "worktree-spawn: --base requires a value" >&2; usage >&2; exit 2; }
      base_arg="$1"; shift ;;
    --mode)
      shift; [[ $# -gt 0 ]] || { echo "worktree-spawn: --mode requires a value" >&2; usage >&2; exit 2; }
      mode_arg="$1"; shift ;;
    --run-id)
      shift; [[ $# -gt 0 ]] || { echo "worktree-spawn: --run-id requires a value" >&2; usage >&2; exit 2; }
      run_id_arg="$1"; shift ;;
    --session-id)
      shift; [[ $# -gt 0 ]] || { echo "worktree-spawn: --session-id requires a value" >&2; usage >&2; exit 2; }
      session_id_arg="$1"; shift ;;
    --experiment) EXPERIMENT=true; shift ;;
    -h | --help) usage; exit 0 ;;
    *) echo "worktree-spawn: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# Fail-loud on missing required args (no defaults for identity-bearing inputs).
if [[ -z "$path_arg" ]]; then
  echo "worktree-spawn: --path is required" >&2; usage >&2; exit 2
fi
if [[ -z "$branch_arg" ]]; then
  echo "worktree-spawn: --branch is required" >&2; usage >&2; exit 2
fi

if ! git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  echo "worktree-spawn: $REPO_ROOT is not a git repository" >&2; exit 2
fi

base_ref="${base_arg:-HEAD}"
base_sha="$(git -C "$REPO_ROOT" rev-parse --verify --quiet "$base_ref" 2>/dev/null || true)"
if [[ -z "$base_sha" ]]; then
  echo "worktree-spawn: base ref '$base_ref' is not resolvable in $REPO_ROOT" >&2; exit 1
fi

wt_path="$(resolve_abs "$path_arg")"

# Create the worktree + branch off the resolved base.
if ! git -C "$REPO_ROOT" worktree add -b "$branch_arg" "$wt_path" "$base_sha" >/dev/null 2>&1; then
  echo "worktree-spawn: 'git worktree add -b $branch_arg $wt_path $base_ref' failed" >&2
  echo "worktree-spawn: (does the branch already exist, or is the path non-empty?)" >&2
  exit 1
fi

# Canonicalize to the real worktree directory now that git created it, so the
# marker lands exactly where the hygiene report will look for it.
if wt_real="$(cd "$wt_path" 2>/dev/null && pwd -P)"; then
  wt_path="$wt_real"
fi

created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
marker_file="$wt_path/.bubbles-worktree"

# Stamp the framework-created identity marker (valid JSON, all five fields).
{
  printf '{\n'
  printf '  "runId": "%s",\n' "$(json_escape "$run_id_arg")"
  printf '  "mode": "%s",\n' "$(json_escape "$mode_arg")"
  printf '  "baseSha": "%s",\n' "$(json_escape "$base_sha")"
  printf '  "createdAt": "%s",\n' "$created_at"
  printf '  "sessionId": "%s"\n' "$(json_escape "$session_id_arg")"
  printf '}\n'
} > "$marker_file"

exp_note=""
if [[ "$EXPERIMENT" == true ]]; then
  # Compose with IMP-107 SCOPE-3: declare the worktree a disposable probe too.
  : > "$wt_path/.design-experiment"
  exp_note=" (+ .design-experiment)"
fi

echo "[worktree-spawn] created worktree $wt_path"
echo "  branch=$branch_arg  base=$base_ref  baseSha=${base_sha:0:12}"
echo "  marker=.bubbles-worktree${exp_note}  runId=${run_id_arg:-<none>}  mode=${mode_arg:-<none>}  sessionId=${session_id_arg:-<none>}"
echo "[worktree-spawn] reap when done: worktree-reap.sh (or 'cli.sh doctor --heal'). Unmarked human worktrees are never auto-reaped."
exit 0
