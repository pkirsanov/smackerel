#!/usr/bin/env bash
# Work-Boundary Resolver (IMP-100 Phase 4 R6 / work-boundary)
# ---------------------------------------------------------------------------
# Mechanical, FAIL-CLOSED classifier for the immutable per-task WORK BOUNDARY —
# the anti-"wandering" contract (a session that drifts to fix an unrelated repo,
# spec, or path instead of staying on the requested work; e.g. "works on repo A,
# starts fixing repo B"). It reads a feature's declared `workBoundary` from
# state.json and classifies a CANDIDATE change (repo / spec / path) as one of a
# closed disposition set:
#
#   in-boundary        inside the declared boundary → proceed inline
#   route-same-repo    same repo but OUTSIDE the declared specTargets/allowedPaths
#                      → file/route a finding; do NOT inline-fix (unrelated same-repo)
#   route-cross-repo   a DIFFERENT repo AND crossRepoPolicy=authorized
#                      → the user authorized a cross-repo scenario → route/handle
#   refuse-cross-repo  a DIFFERENT repo AND crossRepoPolicy=forbidden (the default)
#                      → REFUSE; never touch another repo unless explicitly authorized
#
# Backward-compatible: a feature with NO `workBoundary` block (or no state.json)
# resolves `in-boundary` — nothing declared to enforce — so existing specs are
# unaffected (default-off, opt-in).
# Fail-closed: a workBoundary that is PRESENT but malformed (missing/empty/non-array
# repositoryRoots, non-string entries, non-array optional lists, or a
# crossRepoPolicy outside {forbidden,authorized}) is a hard error (exit 2) — a
# declared-but-broken boundary MUST NOT silently pass.
#
# It is a RESOLVER (advisory decision on stdout), not a gate: exit 0 when it can
# decide, exit 2 on usage error / missing parser / malformed boundary. It composes
# with (does NOT replace) repo-binding-preflight.sh, which guards the separate
# agent-source-repo INSTALL binding; this adds the per-task ALLOWED-SCOPE decision.
#
# Output (stdout), three lines:
#   disposition=<in-boundary|route-same-repo|route-cross-repo|refuse-cross-repo>
#   repoMatch=<true|false>
#   reason=<why>
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: work-boundary-resolve.sh --feature-dir <dir> --candidate-repo <slug> \
                                [--candidate-spec <id>] [--candidate-path <path>]

Reads <feature-dir>/state.json ".workBoundary" and classifies the candidate:
  disposition=<in-boundary|route-same-repo|route-cross-repo|refuse-cross-repo>
  repoMatch=<true|false>
  reason=<why>

Backward-compatible: no workBoundary (or no state.json) → in-boundary.
Fail-closed: a present-but-malformed workBoundary exits 2.
EOF
}

feature_dir=""
candidate_repo=""
candidate_spec=""
candidate_path=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --feature-dir) feature_dir="${2:-}"; shift 2 ;;
    --candidate-repo) candidate_repo="${2:-}"; shift 2 ;;
    --candidate-spec) candidate_spec="${2:-}"; shift 2 ;;
    --candidate-path) candidate_path="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "work-boundary-resolve: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$feature_dir" || -z "$candidate_repo" ]]; then
  echo "work-boundary-resolve: --feature-dir and --candidate-repo are required" >&2
  usage >&2
  exit 2
fi

decide() {
  local disposition="$1" repo_match="$2" reason="$3"
  printf 'disposition=%s\n' "$disposition"
  printf 'repoMatch=%s\n' "$repo_match"
  printf 'reason=%s\n' "$reason"
  exit 0
}

state="$feature_dir/state.json"

# No state.json → nothing declared to enforce (backward-compatible permissive).
if [[ ! -f "$state" ]]; then
  decide "in-boundary" "unknown" "no state.json at $feature_dir — no boundary to enforce"
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "work-boundary-resolve: jq (required to parse state.json) is not installed" >&2
  exit 2
fi
if ! jq empty "$state" >/dev/null 2>&1; then
  echo "work-boundary-resolve: state.json is missing or is not valid JSON" >&2
  exit 2
fi

# No workBoundary block → backward-compatible permissive.
if [[ "$(jq -r 'has("workBoundary")' "$state")" != "true" ]]; then
  decide "in-boundary" "unknown" "no workBoundary declared — backward-compatible permissive"
fi

# ---------------------------------------------------------------------------
# Shape validation (FAIL-CLOSED). A declared-but-broken boundary MUST NOT pass.
# ---------------------------------------------------------------------------
if [[ "$(jq -r '.workBoundary | type' "$state")" != "object" ]]; then
  echo "work-boundary-resolve: workBoundary must be an object" >&2
  exit 2
fi
# repositoryRoots: required, non-empty array of strings.
if [[ "$(jq -r '.workBoundary.repositoryRoots | type' "$state" 2>/dev/null)" != "array" ]]; then
  echo "work-boundary-resolve: workBoundary.repositoryRoots must be an array" >&2
  exit 2
fi
if [[ "$(jq -r '(.workBoundary.repositoryRoots | length) >= 1 and (.workBoundary.repositoryRoots | all(type == "string" and length > 0))' "$state")" != "true" ]]; then
  echo "work-boundary-resolve: workBoundary.repositoryRoots must be a non-empty array of non-empty strings" >&2
  exit 2
fi
# Optional specTargets / allowedPaths: arrays of strings when present.
for opt in specTargets allowedPaths; do
  if [[ "$(jq -r --arg k "$opt" 'if (.workBoundary | has($k)) then (.workBoundary[$k] | type) else "absent" end' "$state")" == "array" ]]; then
    if [[ "$(jq -r --arg k "$opt" '.workBoundary[$k] | all(type == "string" and length > 0)' "$state")" != "true" ]]; then
      echo "work-boundary-resolve: workBoundary.$opt must be an array of non-empty strings" >&2
      exit 2
    fi
  elif [[ "$(jq -r --arg k "$opt" 'if (.workBoundary | has($k)) then (.workBoundary[$k] | type) else "absent" end' "$state")" != "absent" ]]; then
    echo "work-boundary-resolve: workBoundary.$opt must be an array when present" >&2
    exit 2
  fi
done
# crossRepoPolicy: optional, default 'forbidden'; closed enum.
cross_policy="$(jq -r '.workBoundary.crossRepoPolicy // "forbidden"' "$state")"
case "$cross_policy" in
  forbidden|authorized) ;;
  *) echo "work-boundary-resolve: workBoundary.crossRepoPolicy must be 'forbidden' or 'authorized'" >&2; exit 2 ;;
esac

# ---------------------------------------------------------------------------
# Classification.
# ---------------------------------------------------------------------------
# 1) Repo dimension: is the candidate repo one of the declared roots?
repo_match="$(jq -r --arg r "$candidate_repo" '.workBoundary.repositoryRoots | index($r) != null' "$state")"
if [[ "$repo_match" != "true" ]]; then
  if [[ "$cross_policy" == "authorized" ]]; then
    decide "route-cross-repo" "false" "candidate repo '$candidate_repo' is outside repositoryRoots but crossRepoPolicy=authorized — handle as an authorized cross-repo scenario"
  fi
  decide "refuse-cross-repo" "false" "candidate repo '$candidate_repo' is outside repositoryRoots and crossRepoPolicy=forbidden — refuse (route-only; never touch another repo unless authorized)"
fi

# 2) Same repo. Check the optional spec dimension (only when both declared + given).
spec_targets_declared="$(jq -r 'if (.workBoundary | has("specTargets")) then ((.workBoundary.specTargets | length) > 0) else false end' "$state")"
if [[ "$spec_targets_declared" == "true" && -n "$candidate_spec" ]]; then
  cand_spec_base="${candidate_spec##*/}"
  spec_in="$(jq -r --arg s "$candidate_spec" --arg b "$cand_spec_base" '.workBoundary.specTargets | any((. == $s) or ((. | sub("/$"; "") | split("/") | last) == $b))' "$state")"
  if [[ "$spec_in" != "true" ]]; then
    decide "route-same-repo" "true" "candidate spec '$candidate_spec' is in-repo but outside the declared specTargets — file/route a finding rather than inline-fixing unrelated work"
  fi
fi

# 3) Same repo. Check the optional path dimension (only when both declared + given).
if [[ "$(jq -r 'if (.workBoundary | has("allowedPaths")) then ((.workBoundary.allowedPaths | length) > 0) else false end' "$state")" == "true" && -n "$candidate_path" ]]; then
  allowed_list="$(jq -r '.workBoundary.allowedPaths[]' "$state")"
  path_ok="false"
  while IFS= read -r pat; do
    [[ -z "$pat" ]] && continue
    case "$pat" in
      *"/**")
        prefix="${pat%/**}"
        if [[ "$candidate_path" == "$prefix" || "$candidate_path" == "$prefix/"* ]]; then path_ok="true"; break; fi
        ;;
      */)
        if [[ "$candidate_path" == "$pat"* ]]; then path_ok="true"; break; fi
        ;;
      *)
        if [[ "$candidate_path" == "$pat" ]]; then path_ok="true"; break; fi
        ;;
    esac
  done <<< "$allowed_list"
  if [[ "$path_ok" != "true" ]]; then
    decide "route-same-repo" "true" "candidate path '$candidate_path' is in-repo but outside the declared allowedPaths — file/route a finding rather than inline-fixing unrelated work"
  fi
fi

# 4) Inside the repo, and within any declared spec/path scope → in-boundary.
decide "in-boundary" "true" "candidate repo '$candidate_repo' is within repositoryRoots and within any declared spec/path scope"
