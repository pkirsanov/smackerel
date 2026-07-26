#!/usr/bin/env bash
set -uo pipefail

# workflow-yaml-validity-selftest.sh
#
# IMP-102 / SCOPE-3. Proof that every GitHub Actions workflow under
# .github/workflows/ is valid YAML that GitHub can actually load, and that the
# check has real teeth against the exact defect this scope fixed: an inline
# `run: |` block-scalar whose embedded `python3 -c` continuation lines sit at
# COLUMN 0 (below the block-scalar indent). YAML terminates the block scalar at
# the col-0 line and then mis-parses `try:` / `except:` as mapping keys, so
# GitHub silently fails to load the workflow and its enforcement job never runs.
#
# Two layers:
#   (positive, LIVE)      every real .github/workflows/*.yml|*.yaml MUST parse
#                         via python3 + PyYAML. The file path is passed by ARGV
#                         (sys.argv[1]) and its contents are NEVER interpolated
#                         into the Python source (consistent with the SCOPE-4
#                         RCE-hardening posture).
#   (adversarial, HERMETIC) a temp workflow reproducing the col-0-continuation
#                         anti-pattern MUST be REJECTED by the same parser, and
#                         its correct single-line equivalent MUST parse. A
#                         tautological check (where the broken fixture still
#                         "passes") is forbidden.
#
# Graceful-skip (exit 0 + explicit SKIP) when python3 / PyYAML is unavailable.
# Hermetic fixtures live under a mktemp dir cleaned on exit; the positive scan
# is read-only and never mutates the repo.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Repo root: prefer git (robust in the framework source tree, in a downstream
# .github/bubbles/scripts install, AND inside a detached worktree); fall back
# to the scripts/../.. layout when git is unavailable.
if ! REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null)"; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

pass=0
fail=0

ok() {
  pass=$((pass + 1))
}

bad() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

# ── Graceful skip when the parser stack is unavailable ──────────────────────
if ! command -v python3 >/dev/null 2>&1; then
  echo "workflow-yaml-validity-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import yaml' >/dev/null 2>&1; then
  echo "workflow-yaml-validity-selftest: SKIP (PyYAML not installed)"
  exit 0
fi

# Parse helper: the workflow file arrives via ARGV (sys.argv[1]); its contents
# are NEVER interpolated into the Python source. Returns 0 iff valid YAML.
yaml_parses() { # file
  python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))' "$1" >/dev/null 2>&1
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-workflow-yaml-validity.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "=== workflow YAML validity selftest (IMP-102 / SCOPE-3) ==="
echo "repo root: $REPO_ROOT"

# ── Layer 1 (positive, LIVE): every real workflow MUST parse ────────────────
workflows_dir="$REPO_ROOT/.github/workflows"
scanned=0
if [[ -d "$workflows_dir" ]]; then
  shopt -s nullglob
  for wf in "$workflows_dir"/*.yml "$workflows_dir"/*.yaml; do
    scanned=$((scanned + 1))
    if yaml_parses "$wf"; then
      ok
    else
      bad "workflow does not parse (GitHub cannot load it): ${wf#"$REPO_ROOT"/}"
    fi
  done
  shopt -u nullglob
fi
echo "positive scan: $scanned workflow(s) checked under ${workflows_dir#"$REPO_ROOT"/}"

# ── Layer 2a (adversarial, HERMETIC): col-0 continuation MUST be rejected ────
# Faithful replica of the exact defect: an inline `python3 -c` whose try/except
# continuation lines sit at COLUMN 0, below the `run: |` block-scalar indent, so
# YAML ends the block scalar early and the following step's real `name:` mapping
# key collides with the corrupted top-level parse (mirroring the original's
# line-89 "mapping values are not allowed here" failure). A single col-0 block
# with nothing after it folds into a weird-but-valid mapping and would HIDE the
# bug — the trailing second step is what gives this fixture teeth.
red_fixture="$tmp/red-col0-continuation.yml"
cat > "$red_fixture" <<'RED'
name: red-col0-continuation
on:
  push:
jobs:
  guard:
    runs-on: ubuntu-latest
    steps:
      - name: broken inline python continuation
        run: |
          set -euo pipefail
          status="$(echo "$payload" | python3 -c 'import json,sys
try:
  d=json.loads(sys.stdin.read())
  print(d.get("status",""))
except Exception:
  pass' 2>/dev/null || true)"
          echo "$status"
      - name: following step whose name key collides with the corrupted parse
        run: echo done
RED

if yaml_parses "$red_fixture"; then
  bad "adversarial col-0-continuation workflow unexpectedly PARSED — the check has NO teeth (tautological)"
else
  ok
fi

# ── Layer 2b (control, HERMETIC): the corrected single-line form MUST parse ──
# Proves the red result above is specific to the defect, not a broken parser.
green_fixture="$tmp/green-singleline.yml"
cat > "$green_fixture" <<'GREEN'
name: green-singleline
on:
  push:
jobs:
  guard:
    runs-on: ubuntu-latest
    steps:
      - name: correct inline python one-liner
        run: |
          set -euo pipefail
          status="$(echo "$payload" | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); print(d.get("status",""))' 2>/dev/null || true)"
          echo "$status"
GREEN

if yaml_parses "$green_fixture"; then
  ok
else
  bad "green single-line-equivalent workflow failed to parse (control fixture should be valid)"
fi

echo ""
echo "workflow-yaml-validity-selftest: $pass passed / $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
