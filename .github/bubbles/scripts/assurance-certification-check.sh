#!/usr/bin/env bash
# Assurance Certification Consistency Guard (IMP-100 Phase 3 choke point #1 — enforcement)
# ---------------------------------------------------------------------------
# `bubbles.validate` records the achieved assurance level at terminal
# certification under `state.json` `.certification.assurance = { level,
# missingForFull }` (derived by assurance-derive.sh). This guard REFUSES a
# recorded assurance block that is internally INCONSISTENT with the fail-closed
# derivation invariants — the anti-tamper teeth so a hand-edited or fabricated
# block can never claim a higher assurance than its own gap list supports:
#
#   level=full      → missingForFull MUST be empty (a full chain has no gaps)
#   level=fast      → missingForFull MUST list `independent-audit` (fast's only
#                     missing piece for full is the independent audit)
#   level=prototype → missingForFull MUST be non-empty (verification incomplete)
#
# BACKWARD-COMPATIBLE: a state.json with no `.certification.assurance` block (the
# vast majority of specs) is a no-op (exit 0). Reads JSON with `jq`; if `jq` is
# not installed it WARNs and skips (exit 0) rather than failing the run.
#
# Exit 0 = consistent / no block / no parser. Exit 1 = an inconsistent recorded
# block (REFUSED, naming the violation). Exit 2 = usage error.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: assurance-certification-check.sh --feature-dir <dir>

Validates that <dir>/state.json's `.certification.assurance` block (if present)
is internally consistent with the assurance-derive invariants:
  full → no gaps; fast → gaps list independent-audit; prototype → gaps non-empty.
Exit 0 = consistent / no block / jq missing. Exit 1 = inconsistent. Exit 2 = usage.
No bypass flag.
EOF
}

feature_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --feature-dir) feature_dir="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) echo "assurance-certification-check: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$feature_dir" ]]; then
  echo "assurance-certification-check: --feature-dir is required" >&2
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "assurance-certification-check: feature dir not found: $feature_dir" >&2
  exit 2
fi

state_file="$feature_dir/state.json"
if [[ ! -f "$state_file" ]]; then
  echo "[assurance-certification-check] OK — no state.json (no-op)."
  exit 0
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "[assurance-certification-check] WARN-and-skip — jq not installed; cannot parse $state_file (exit 0)."
  exit 0
fi

# No assurance block → backward-compatible no-op.
level="$(jq -r '(.certification.assurance.level // "")' "$state_file" 2>/dev/null || echo "")"
if [[ -z "$level" || "$level" == "null" ]]; then
  echo "[assurance-certification-check] OK — no .certification.assurance block (no-op)."
  exit 0
fi

case "$level" in
  full | fast | prototype) ;;
  *)
    echo "[assurance-certification-check] REFUSED: .certification.assurance.level='$level' is not one of full|fast|prototype." >&2
    exit 1
    ;;
esac

# missingForFull as a comma-joined string (empty when the array is absent/empty).
missing="$(jq -r '((.certification.assurance.missingForFull // []) | join(","))' "$state_file" 2>/dev/null || echo "")"

case "$level" in
  full)
    if [[ -n "$missing" ]]; then
      echo "[assurance-certification-check] REFUSED: level=full but missingForFull is non-empty ('$missing') — a full chain has no gaps." >&2
      exit 1
    fi
    ;;
  fast)
    if [[ ",$missing," != *",independent-audit,"* ]]; then
      echo "[assurance-certification-check] REFUSED: level=fast but missingForFull ('${missing:-<empty>}') does not list 'independent-audit' — fast is full minus the independent audit." >&2
      exit 1
    fi
    ;;
  prototype)
    if [[ -z "$missing" ]]; then
      echo "[assurance-certification-check] REFUSED: level=prototype but missingForFull is empty — a prototype has incomplete verification (at least one gap)." >&2
      exit 1
    fi
    ;;
esac

echo "[assurance-certification-check] OK — recorded assurance is internally consistent (level=$level, missingForFull='${missing:-<none>}')."
exit 0
