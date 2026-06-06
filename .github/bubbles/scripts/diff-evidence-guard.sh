#!/usr/bin/env bash
#
# bubbles diff-evidence-guard.sh (v5.1 / M5).
#
# Verifies that DoD items claiming "X file added", "Y test created", or
# "Z handler wired" actually appear in the spec's git diff range.
#
# Catches the "claimed done, didn't change code" failure mode at gate time,
# even when the agent's evidence prose is convincing.
#
# How it works:
#   1. Resolve the spec's baseSha from state.json (executionHistory[0]) or
#      fall back to the spec's first commit.
#   2. For each DoD item in scopes.md (or per-scope scope.md), look for
#      claims about file/test/endpoint creation.
#   3. Cross-reference against `git diff --name-status <baseSha>..HEAD`.
#   4. Fail if a DoD item claims "test added at <path>" but <path> is not
#      in the diff with an A (added) status.
#
# Exit: 0 = all DoD claims match diff; 1 = at least one claim doesn't match.
# Advisory by default in v5.1 — flip to blocking in v5.2 via env var
# BUBBLES_DIFF_EVIDENCE_GUARD_STRICT=1.

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: diff-evidence-guard.sh <spec-dir> [--strict] [--base-sha <sha>]

Verifies DoD claims in scopes.md match the git diff for the spec.

Arguments:
  <spec-dir>   path to specs/<feature>/
  --strict     fail non-zero on mismatch (default: advisory; exit 0 even on mismatch)
  --base-sha   override the baseline SHA used for diffing (default: state.json executionHistory[0])
USAGE
}

if [[ $# -lt 1 ]]; then usage; exit 2; fi

SPEC_DIR="$1"
shift
STRICT="${BUBBLES_DIFF_EVIDENCE_GUARD_STRICT:-0}"
BASE_SHA_OVERRIDE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) STRICT=1; shift;;
    --base-sha) BASE_SHA_OVERRIDE="$2"; shift 2;;
    -h|--help) usage; exit 0;;
    *) usage; exit 2;;
  esac
done

[[ -d "$SPEC_DIR" ]] || { echo "diff-evidence-guard: spec dir missing: $SPEC_DIR" >&2; exit 2; }

if ! command -v git >/dev/null 2>&1; then
  echo "diff-evidence-guard: SKIP (git not installed)"
  exit 0
fi

REPO_ROOT="$(cd "$SPEC_DIR" && git rev-parse --show-toplevel 2>/dev/null || true)"
[[ -n "$REPO_ROOT" ]] || { echo "diff-evidence-guard: SKIP (not a git repo)"; exit 0; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "diff-evidence-guard: SKIP (python3 not installed)"
  exit 0
fi

# v6.0 / B2: diff-evidence-guard is default-on for ALL specs.
#
# Promotion rules (auto-strict when ALL of these are true):
#   - state.json.modernization.diffEvidence is NOT "advisory"
#
# v5.2 / F2 (superseded): auto-strict only for specs created on/after the
#   2026-06-04 cutoff. Kept here as a fallback when state.json has no
#   modernization block AND the spec predates the cutoff — those specs
#   stay advisory until their state.json is touched (grandfather clause).
#
# v6.0 / B2 (current): explicit opt-out only.
#   - state.json.modernization.diffEvidence == "advisory" → advisory mode
#   - state.json.modernization.diffEvidence == "enforce"  → strict mode
#   - state.json.modernization MISSING or empty           → strict mode
#                                                          (was advisory in v5.x
#                                                          unless post-cutoff)
# Manual --strict / BUBBLES_DIFF_EVIDENCE_GUARD_STRICT=1 still works for
# operators who want to force the gate on an otherwise-advisory spec.
#
# Grandfather clause: specs created before the v5.2 cutoff that have NO
# state.json.modernization block keep their v5 advisory behavior, because
# they were authored under the older policy and may legitimately lack the
# diff-traceable path-claims that B2 expects. Touching state.json (any
# write) demotes them to the v6 policy automatically.
DIFF_EVIDENCE_CUTOFF="2026-06-04"
if [[ "$STRICT" != "1" ]] && [[ -f "$SPEC_DIR/state.json" ]]; then
  AUTO_DECISION="$(python3 - <<PY
import json, sys, subprocess
try:
    d = json.load(open("$SPEC_DIR/state.json"))
except Exception:
    print("unknown"); sys.exit(0)
mod = (d.get('modernization') or {})
choice = (mod.get('diffEvidence') or '').strip().lower()
if choice == 'advisory':
    print('advisory'); sys.exit(0)
if choice == 'enforce':
    print('enforce'); sys.exit(0)
# v6.0 / B2: default-on. v5.x grandfather clause: pre-cutoff specs with
# no explicit choice AND no modernization block stay advisory.
has_mod_block = bool(mod)
try:
    first = subprocess.check_output(
        ['git', '-C', "$REPO_ROOT", 'log', '--diff-filter=A', '--format=%cI', '--', "$SPEC_DIR"],
        stderr=subprocess.DEVNULL, text=True,
    ).strip().splitlines()
    first_date = first[-1][:10] if first else ""
except Exception:
    first_date = ""
if first_date and first_date < "$DIFF_EVIDENCE_CUTOFF" and not has_mod_block:
    # Pre-cutoff spec with no modernization block — v5 grandfather.
    print('advisory'); sys.exit(0)
# Everything else: v6 default-on.
print('enforce')
PY
)"
  case "$AUTO_DECISION" in
    enforce)
      STRICT=1
      echo "diff-evidence-guard: enforcing (v6.0 / B2 default; opt out via state.json.modernization.diffEvidence=\"advisory\")"
      ;;
    advisory)
      echo "diff-evidence-guard: advisory mode (state.json.modernization.diffEvidence=\"advisory\" OR v5 grandfather)"
      ;;
    unknown)
      : # leave STRICT=0 — state.json unparseable; do not promote
      ;;
  esac
fi

# Resolve baseSha.
BASE_SHA="$BASE_SHA_OVERRIDE"
if [[ -z "$BASE_SHA" ]] && [[ -f "$SPEC_DIR/state.json" ]]; then
  BASE_SHA="$(python3 -c "
import json, sys
try:
    d = json.load(open('$SPEC_DIR/state.json'))
    eh = d.get('executionHistory', [])
    if eh:
        print(eh[0].get('baseSha', ''))
except Exception:
    pass
" 2>/dev/null)"
fi

if [[ -z "$BASE_SHA" ]]; then
  # Fall back to the first commit that touched the spec directory.
  BASE_SHA="$(git -C "$REPO_ROOT" log --diff-filter=A --format=%H -- "$SPEC_DIR" 2>/dev/null | tail -1)"
fi

if [[ -z "$BASE_SHA" ]]; then
  echo "diff-evidence-guard: SKIP (no baseSha resolvable for $SPEC_DIR)"
  exit 0
fi

# Collect spec scope files.
SCOPE_FILES=()
if [[ -f "$SPEC_DIR/scopes.md" ]]; then
  SCOPE_FILES+=("$SPEC_DIR/scopes.md")
fi
if [[ -d "$SPEC_DIR/scopes" ]]; then
  while IFS= read -r f; do SCOPE_FILES+=("$f"); done < <(find "$SPEC_DIR/scopes" -type f -name 'scope.md' 2>/dev/null)
fi

if [[ "${#SCOPE_FILES[@]}" -eq 0 ]]; then
  echo "diff-evidence-guard: SKIP (no scope files in $SPEC_DIR)"
  exit 0
fi

# Get changed files since baseSha.
CHANGED_FILES_RAW="$(git -C "$REPO_ROOT" diff --name-status "$BASE_SHA"..HEAD 2>/dev/null || true)"

# Python does the heavy lifting: parse DoD lines, extract claimed paths,
# cross-reference against the diff.
SCOPE_FILES_JOINED="$(printf '%s\n' "${SCOPE_FILES[@]}")"

python3 - <<PY
import os, re, sys
from pathlib import Path

repo_root = "$REPO_ROOT"
base_sha = "$BASE_SHA"
strict = $STRICT
scope_files = """$SCOPE_FILES_JOINED""".strip().splitlines()
changed_raw = """$CHANGED_FILES_RAW""".strip().splitlines()

# Parse diff: status -> set of paths.
added_paths = set()
modified_paths = set()
all_changed = set()
for line in changed_raw:
    if not line.strip():
        continue
    parts = line.split('\t', 2)
    if len(parts) < 2:
        continue
    status = parts[0].strip()
    path = parts[-1].strip()
    all_changed.add(path)
    if status.startswith('A'):
        added_paths.add(path)
    if status.startswith('M') or status.startswith('A'):
        modified_paths.add(path)

# Detect claims in checked DoD items: "- [x] ... at \`<path>\`" or "added: <path>".
# Heuristics — kept conservative to avoid false positives.
CHECKED_DOD_RE = re.compile(r'^- \[x\] (.*)$')
PATH_CLAIM_RE = re.compile(r'\`([A-Za-z0-9_/.\-]+\.(?:rs|go|ts|tsx|js|jsx|py|sh|md|yaml|yml|json|sql|proto))\`')
ACTION_RE = re.compile(r'\b(add(?:ed)?|creat(?:ed)?|new|wire(?:d)?|introduc(?:e|ed))\b', re.IGNORECASE)

claims = []  # (scope_file, line_no, dod_text, claimed_paths)

for sf in scope_files:
    try:
        text = Path(sf).read_text(errors='replace')
    except Exception:
        continue
    for i, raw in enumerate(text.splitlines(), start=1):
        m = CHECKED_DOD_RE.match(raw)
        if not m:
            continue
        body = m.group(1)
        if not ACTION_RE.search(body):
            continue
        paths = PATH_CLAIM_RE.findall(body)
        if not paths:
            continue
        claims.append((str(Path(sf).resolve()).removeprefix(repo_root.rstrip('/') + '/'), i, body, paths))

if not claims:
    print(f"diff-evidence-guard: PASS (no DoD path-claims to verify in $(echo "${SCOPE_FILES[@]}" | wc -w) scope file(s); baseSha={base_sha[:12]})")
    sys.exit(0)

mismatches = []
for sf, ln, body, paths in claims:
    for p in paths:
        if p not in all_changed:
            mismatches.append((sf, ln, body, p))

if not mismatches:
    print(f"diff-evidence-guard: PASS ({len(claims)} DoD path-claim(s) verified against diff baseSha={base_sha[:12]})")
    sys.exit(0)

print(f"diff-evidence-guard: {'FAIL' if strict else 'WARN'}")
print(f"  baseSha: {base_sha[:12]}..HEAD")
print(f"  {len(mismatches)} DoD item(s) claim path changes NOT present in diff:")
for sf, ln, body, p in mismatches[:20]:
    body_short = (body[:80] + '…') if len(body) > 80 else body
    print(f"    {sf}:{ln}: claims '{p}' but path is not in git diff")
    print(f"      → {body_short}")
if len(mismatches) > 20:
    print(f"    … {len(mismatches) - 20} more")
print("")
print("  Either:")
print("    1. The work was claimed done but not committed — finish + commit, OR")
print("    2. The DoD path was misspelled — fix the DoD text, OR")
print("    3. The baseSha is wrong — pass --base-sha <correct-sha>")

if strict:
    sys.exit(1)
sys.exit(0)
PY
