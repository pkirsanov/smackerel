#!/usr/bin/env bash
#
# bubbles generate-modes-block.sh — modes-block strip + no-duplication guard
# (v6.1 / S2 true split).
#
# Canonical source: bubbles/workflows/modes.yaml owns the `modes:` registry
#                   (phaseRelevance sub-block + every mode definition). This is
#                   the ONLY place mode definitions live.
# workflows.yaml:   MUST NOT carry an inline `modes:` block. mode-resolver.sh
#                   composes workflows.yaml + modes.yaml at read time; the other
#                   consumers read modes.yaml directly. Keeping a copy in
#                   workflows.yaml would re-introduce the duplication S2 removed.
#
# Usage:
#   generate-modes-block.sh              # same as --check (read-only)
#   generate-modes-block.sh --check      # exit 0 if workflows.yaml has no
#                                        # inline modes block; exit 1 if it does
#                                        # (duplication regression)
#   generate-modes-block.sh --strip      # remove an inline modes block from
#                                        # workflows.yaml (one-shot migration /
#                                        # repair). No-op if already stripped.
#
# Exit codes:
#   0 — check passed / strip applied or no-op
#   1 — duplication detected in --check mode
#   2 — usage error or missing input

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
REGISTRY="$REPO_ROOT/bubbles/workflows/modes.yaml"
WORKFLOWS="$REPO_ROOT/bubbles/workflows.yaml"

MODE="check"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --strip) MODE="strip"; shift;;
    --check) MODE="check"; shift;;
    -h|--help)
      sed -n '2,30p' "$0"
      exit 0
      ;;
    *) echo "generate-modes-block: unknown arg: $1" >&2; exit 2;;
  esac
done

[[ -f "$WORKFLOWS" ]] || { echo "generate-modes-block: workflows.yaml missing at $WORKFLOWS" >&2; exit 2; }
[[ -f "$REGISTRY" ]] || {
  if [[ "$MODE" == "check" ]]; then
    # Downstream repos that installed a pre-S2 release won't have the registry
    # file. Emit SKIP rather than FAIL so framework-validate stays green until
    # the next install.sh run.
    echo "generate-modes-block: SKIP (bubbles/workflows/modes.yaml missing — re-run install.sh to upgrade past the modes split)"
    exit 0
  fi
  echo "generate-modes-block: registry missing at $REGISTRY" >&2
  exit 2
}

python3 - "$REGISTRY" "$WORKFLOWS" "$MODE" <<'PY'
import sys
from pathlib import Path

registry_path, workflows_path, mode = sys.argv[1], sys.argv[2], sys.argv[3]
current = Path(workflows_path).read_text()
lines = current.splitlines(keepends=True)

# Locate an inline `modes:` top-level key, if any. modes: is (was) the LAST
# top-level key, so an inline block runs from `start` to EOF.
start = None
for i, line in enumerate(lines):
    if line.rstrip('\n') == 'modes:':
        start = i
        break

reg_lines = len(Path(registry_path).read_text().splitlines())

if mode == 'check':
    # Post-split invariant: workflows.yaml MUST NOT carry an inline modes:
    # block — that would re-introduce the duplication S2 removed. The canonical
    # registry lives in bubbles/workflows/modes.yaml.
    if start is None:
        print("generate-modes-block: OK — workflows.yaml carries no inline modes "
              f"block; canonical registry is bubbles/workflows/modes.yaml ({reg_lines} lines)")
        sys.exit(0)
    print("generate-modes-block: DUPLICATION — workflows.yaml still contains an "
          "inline 'modes:' block.", file=sys.stderr)
    print("  Mode definitions are owned by bubbles/workflows/modes.yaml. Run:",
          file=sys.stderr)
    print("    bash bubbles/scripts/generate-modes-block.sh --strip", file=sys.stderr)
    sys.exit(1)

# strip mode: rewrite workflows.yaml as the preamble before `modes:`, leaving a
# pointer comment so readers know where the registry went.
if start is None:
    print("generate-modes-block: no change — workflows.yaml already carries no "
          "inline modes block")
    sys.exit(0)

pointer = (
    "# ── Workflow modes ─────────────────────────────────────────────────────────\n"
    "# The modes registry (mode definitions + phaseRelevance) lives in its own\n"
    "# canonical file: bubbles/workflows/modes.yaml (v6.1 / S2 true split).\n"
    "# mode-resolver.sh composes workflows.yaml + modes.yaml at read time; the\n"
    "# other consumers read modes.yaml directly. Do NOT re-inline a modes: block\n"
    "# here — generate-modes-block.sh --check (run by framework-validate) blocks\n"
    "# that duplication regression.\n"
)
preamble = ''.join(lines[:start]).rstrip('\n') + '\n' + pointer
removed = len(lines) - start
Path(workflows_path).write_text(preamble)
print(f"generate-modes-block: stripped inline modes block from workflows.yaml "
      f"({removed} lines removed; canonical registry has {reg_lines} lines)")
PY
