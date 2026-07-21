#!/usr/bin/env bash
# Held-Out Eval Isolation Guard (IMP-100 Phase 6 / IMP-020 S4 — AF-004)
# ---------------------------------------------------------------------------
# A held-out benchmark is only meaningful if its tasks were NOT visible while the
# harness (and the framework) were built — otherwise the score is overfit. This
# guard enforces the isolation invariant mechanically:
#   1. ISOLATION — no held-out taskId may also appear in the development-visible
#      task/fixture corpus (bubbles/eval/tasks + bubbles/eval/fixtures). A leaked
#      task is a hard failure.
#   2. SUBSTANTIVE — every held-out task MUST be a v2 task (schemaVersion 2) with
#      at least one REQUIRED executable-oracle or semantic-evaluator check, so a
#      held-out result certifies real outcome, never structure alone (AF-001).
# It also reports the held-out set stratified by an optional per-task `stratum`
# field, so a benchmark can report per-stratum results.
#
# The held-out tasks themselves are operator-supplied and kept out of the
# development corpus (like the semantic/judge adapters, which are operator
# configuration). This repo ships only the CONVENTION + this guard; an empty or
# README-only held-out dir is a NO-OP.
#
# Cost + provenance: run the held-out suite through the evidence recorder, e.g.
#   BUBBLES_TOOL_LOG_TAGS=eval,held-out bash bubbles/scripts/tool-log.sh -- \
#     bash bubbles/scripts/eval-harness.sh run --suite <held-out> --output <out>
# The tool-log receipt carries durationMs (cost) + framework version/sha
# (provenance); eval-harness emits the per-task outcome.
#
# Usage:
#   bash bubbles/scripts/eval-heldout-guard.sh [--held-out <dir>] [--contract <dir>]...
#
# Exit codes:
#   0  clean / not-applicable (no held-out tasks)
#   1  isolation leak or a non-substantive held-out task
#   2  usage / runtime error
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EVAL_DIR="$(dirname "$SCRIPT_DIR")/eval"

HELD_OUT_DIR="$EVAL_DIR/held-out"
CONTRACT_DIRS=()

usage() {
  cat <<'EOF'
Usage: eval-heldout-guard.sh [--held-out <dir>] [--contract <dir>]...

Enforces held-out benchmark isolation: no held-out taskId may appear in the
development-visible corpus, and every held-out task must be a substantive v2
task. Defaults: held-out=bubbles/eval/held-out, contract=bubbles/eval/{tasks,fixtures}.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --held-out)
      [[ $# -ge 2 ]] || { echo "eval-heldout-guard: --held-out requires a value" >&2; exit 2; }
      HELD_OUT_DIR="$2"
      shift 2
      ;;
    --contract)
      [[ $# -ge 2 ]] || { echo "eval-heldout-guard: --contract requires a value" >&2; exit 2; }
      CONTRACT_DIRS+=("$2")
      shift 2
      ;;
    *)
      echo "eval-heldout-guard: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${#CONTRACT_DIRS[@]}" -eq 0 ]]; then
  CONTRACT_DIRS=("$EVAL_DIR/tasks" "$EVAL_DIR/fixtures")
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "[eval-heldout-guard] WARN-and-skip — jq not installed (exit 0)." >&2
  exit 0
fi

# No held-out directory (or no task JSON in it) → nothing to check.
if [[ ! -d "$HELD_OUT_DIR" ]]; then
  echo "[eval-heldout-guard] no held-out dir at $HELD_OUT_DIR — no-op"
  exit 0
fi

# Collect held-out task files (top-level JSON files that declare a taskId).
held_out_files=()
while IFS= read -r f; do
  [[ -n "$f" ]] || continue
  if jq -e 'type == "object" and has("taskId")' "$f" >/dev/null 2>&1; then
    held_out_files+=("$f")
  fi
done < <(find "$HELD_OUT_DIR" -type f -name '*.json' 2>/dev/null | LC_ALL=C sort)

if [[ "${#held_out_files[@]}" -eq 0 ]]; then
  echo "[eval-heldout-guard] no held-out task JSON under $HELD_OUT_DIR — no-op (convention present, tasks operator-supplied)"
  exit 0
fi

# Build the development-visible taskId set.
contract_ids_file="$(mktemp)"
trap 'rm -f "$contract_ids_file"' EXIT INT TERM
for cdir in "${CONTRACT_DIRS[@]}"; do
  [[ -d "$cdir" ]] || continue
  while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    jq -r '(.taskId // empty)' "$f" 2>/dev/null || true
  done < <(find "$cdir" -type f -name '*.json' 2>/dev/null) >> "$contract_ids_file"
done

findings=0
violation() {
  echo "❌ eval-heldout-guard: $*" >&2
  findings=$((findings + 1))
}

# Per-stratum counting.
strata_file="$(mktemp)"
trap 'rm -f "$contract_ids_file" "$strata_file"' EXIT INT TERM

for f in "${held_out_files[@]}"; do
  tid="$(jq -r '.taskId' "$f")"

  # 1. ISOLATION — held-out id must not appear in the development corpus.
  if grep -qxF "$tid" "$contract_ids_file" 2>/dev/null; then
    violation "held-out task '$tid' ($f) also appears in the development-visible corpus — overfit leak; held-out tasks MUST be disjoint"
  fi

  # 2. SUBSTANTIVE — must be v2 with a required executable-oracle / semantic-evaluator.
  schema_v="$(jq -r '(.schemaVersion // 1)' "$f")"
  if [[ "$schema_v" != "2" ]]; then
    violation "held-out task '$tid' ($f) is not schemaVersion 2 — a held-out benchmark cannot certify on the legacy structural contract"
  else
    has_required_substantive="$(jq -r '[.checks[]? | select(.required == true and (.type == "executable-oracle" or .type == "semantic-evaluator"))] | length' "$f" 2>/dev/null || echo 0)"
    if [[ "${has_required_substantive:-0}" -eq 0 ]]; then
      violation "held-out task '$tid' ($f) has no REQUIRED executable-oracle/semantic-evaluator check — a held-out result would certify structure alone (AF-001)"
    fi
  fi

  stratum="$(jq -r '(.stratum // "unstratified")' "$f")"
  printf '%s\n' "$stratum" >> "$strata_file"
done

echo "[eval-heldout-guard] held-out tasks: ${#held_out_files[@]} — per-stratum:"
LC_ALL=C sort "$strata_file" | uniq -c | while IFS= read -r line; do
  echo "  $line"
done

if [[ "$findings" -gt 0 ]]; then
  echo "eval-heldout-guard: $findings held-out isolation/substance violation(s)" >&2
  exit 1
fi

echo "[eval-heldout-guard] OK — ${#held_out_files[@]} held-out task(s) are isolated from the development corpus and substantive."
exit 0
