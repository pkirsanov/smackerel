#!/usr/bin/env bash
# Evidence Receipt Check — targeted invalidation (IMP-100 Phase 2 / IMP-024 SCOPE-2)
# ---------------------------------------------------------------------------
# Consumes the tool-call evidence log (JSONL written by tool-log.sh) and reports,
# for each CURRENT receipt that declared an `inputClosure` (the input files the
# evidence depended on, hashed at record time), whether it is STILL VALID or
# STALE. A later receipt supersedes an earlier receipt with the same evidence
# identity: cwd + spec + scope + command + scenario binding. This is what makes
# the prescribed "re-run to refresh" remediation real rather than permanent
# stale history. A run from another scope or scenario remains a distinct receipt.
#
# A receipt is STALE iff any file in its input closure has CHANGED — either it is
# named in --changed, or its CURRENT sha256 differs from the recorded one (or the
# file is now unreadable). A receipt whose inputs are all unchanged is VALID and
# may be REUSED without re-running the command (targeted invalidation: an
# unrelated / formatter-only change invalidates NOTHING; a behavior change
# invalidates exactly the receipts whose input closure intersects it).
#
# A receipt with NO inputClosure is UNKNOWN — it cannot be proven fresh, so it is
# reported separately and conservatively (never silently treated as valid).
#
# Output: a JSON summary on stdout { total, current, superseded, withClosure,
# valid, stale, unknown, staleReceipts: [{ts, cmd, reason}] }. `total` remains
# the raw append count for compatibility; `current` is the post-supersession
# count evaluated for freshness.
#
# Usage:
#   bash bubbles/scripts/evidence-receipt-check.sh --log <jsonl> \
#        [--repo-root <dir>] [--changed <f1,f2,...>] [--transition-admitted] [--strict]
#
# Exit codes:
#   0  report produced (default) — or clean under --strict (no stale receipts)
#   1  under --strict: at least one stale receipt
#   2  usage / runtime error
set -euo pipefail

LOG_FILE=""
REPO_ROOT="."
CHANGED_CSV=""
TRANSITION_ADMITTED="false"
STRICT="false"

usage() {
  cat <<'EOF'
Usage: evidence-receipt-check.sh --log <jsonl> [--repo-root <dir>] [--changed <f1,f2,...>] [--transition-admitted] [--strict]

Reports which tool-log receipts (with an inputClosure) are still valid vs stale
(an input changed). --strict exits 1 when any receipt is stale.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --log)
      [[ $# -ge 2 ]] || { echo "evidence-receipt-check: --log requires a value" >&2; exit 2; }
      LOG_FILE="$2"
      shift 2
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || { echo "evidence-receipt-check: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT="$2"
      shift 2
      ;;
    --changed)
      [[ $# -ge 2 ]] || { echo "evidence-receipt-check: --changed requires a value" >&2; exit 2; }
      CHANGED_CSV="$2"
      shift 2
      ;;
    --transition-admitted)
      TRANSITION_ADMITTED="true"
      shift
      ;;
    --strict)
      STRICT="true"
      shift
      ;;
    *)
      echo "evidence-receipt-check: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$LOG_FILE" ]]; then
  echo "evidence-receipt-check: missing required --log" >&2
  usage >&2
  exit 2
fi
if [[ ! -f "$LOG_FILE" ]]; then
  echo "evidence-receipt-check: log not found: $LOG_FILE" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "evidence-receipt-check: jq is required but not found in PATH" >&2
  exit 2
fi

# Normalize a path: strip a single leading ./
norm_path() { printf '%s' "${1#./}"; }

# Build a newline-delimited set of changed paths (normalized).
CHANGED_SET=""
if [[ -n "$CHANGED_CSV" ]]; then
  while IFS= read -r c || [[ -n "$c" ]]; do
    [[ -n "$c" ]] || continue
    CHANGED_SET="${CHANGED_SET}$(norm_path "$c")"$'\n'
  done < <(printf '%s' "$CHANGED_CSV" | tr ',' '\n')
fi
changed_contains() {
  local p
  p="$(norm_path "$1")"
  [[ -n "$CHANGED_SET" ]] || return 1
  printf '%s' "$CHANGED_SET" | grep -qxF "$p"
}

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" 2>/dev/null | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" 2>/dev/null | awk '{print $1}'
  else
    printf ''
  fi
}

raw_total=0
current_total=0
with_closure=0
valid=0
stale=0
unknown=0
historical=0
stale_json="[]"

# Select the latest append for each evidence identity before evaluating
# freshness. The log is append-only; supersession changes which receipt is
# current without deleting the historical record. Build the compact current
# view through temporary files so malformed JSONL rows can still be skipped
# individually, matching the checker's previous conservative behavior.
identity_rows="$(mktemp)"
current_receipts="$(mktemp)"
# shellcheck disable=SC2317,SC2329 # Invoked indirectly by the EXIT/INT/TERM trap.
cleanup_receipt_views() {
  rm -f "$identity_rows" "$current_receipts"
}
trap cleanup_receipt_views EXIT INT TERM

while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue
  printf '%s' "$entry" | jq -e 'type == "object"' >/dev/null 2>&1 || continue
  raw_total=$((raw_total + 1))
  identity="$(printf '%s' "$entry" | jq -r '
    [
      (.cwd // ""),
      (.spec // ""),
      (.scope // ""),
      (.cmd // ""),
      (.scenarioBinding.scenarioId // ""),
      (.scenarioBinding.phase // ""),
      (.scenarioBinding.testIdentity // "")
    ] | @base64
  ')"
  compact="$(printf '%s' "$entry" | jq -c '.')"
  printf '%s\t%s\n' "$identity" "$compact" >> "$identity_rows"
done < "$LOG_FILE"

awk -F '\t' '
  {
    latest_at[$1] = NR
    latest[$1] = $2
  }
  END {
    for (key in latest) current[latest_at[key]] = latest[key]
    for (line = 1; line <= NR; line++) {
      if (line in current) print current[line]
    }
  }
' "$identity_rows" > "$current_receipts"

historical_red_indexes="[]"
if [[ "$TRANSITION_ADMITTED" == "true" ]]; then
  historical_red_indexes="$(jq -s '
    to_entries as $receipts
    | [
        $receipts[] as $red
        | select(($red.value.scenarioBinding.phase // "") == "red")
        | ($red.value.cwd // "") as $cwd
        | ($red.value.spec // "") as $spec
        | ($red.value.scope // "") as $scope
        | ($red.value.scenarioBinding.scenarioId // "") as $scenario_id
        | ($red.value.scenarioBinding.testIdentity // "") as $test_identity
        | select($scenario_id != "" and $test_identity != "")
        | select(any(
            $receipts[];
            .key > $red.key
            and (.value.cwd // "") == $cwd
            and (.value.spec // "") == $spec
            and (.value.scope // "") == $scope
            and (.value.scenarioBinding.scenarioId // "") == $scenario_id
            and (.value.scenarioBinding.testIdentity // "") == $test_identity
            and (.value.scenarioBinding.phase // "") == "implement"
            and (.value.exitCode // 1) == 0
          ))
        | $red.key
      ]
  ' "$current_receipts")"
fi

receipt_index=-1
while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue
  receipt_index=$((receipt_index + 1))
  current_total=$((current_total + 1))

  has_closure="$(printf '%s' "$entry" | jq -r 'if (has("inputClosure") and ((.inputClosure // []) | length) > 0) then "yes" else "no" end')"
  if [[ "$has_closure" != "yes" ]]; then
    unknown=$((unknown + 1))
    continue
  fi
  with_closure=$((with_closure + 1))

  ts="$(printf '%s' "$entry" | jq -r '.ts // ""')"
  cmd="$(printf '%s' "$entry" | jq -r '.cmd // ""')"

  reason=""
  while IFS=$'\t' read -r ipath irec; do
    [[ -n "$ipath" ]] || continue
    if changed_contains "$ipath"; then
      reason="input changed (named): $ipath"
      break
    fi
    local_path="$REPO_ROOT/$ipath"
    if [[ ! -f "$local_path" ]]; then
      reason="input missing: $ipath"
      break
    fi
    cur="$(hash_file "$local_path")"
    if [[ "$irec" == "null" || -z "$irec" ]]; then
      reason="input had no recorded hash: $ipath"
      break
    fi
    if [[ "$cur" != "$irec" ]]; then
      reason="input hash differs: $ipath"
      break
    fi
  done < <(printf '%s' "$entry" | jq -r '.inputClosure[] | [.path, (.sha256 // "null")] | @tsv')

  is_historical="false"
  if [[ "$TRANSITION_ADMITTED" == "true" && -n "$reason" ]]; then
    is_historical="$(printf '%s' "$historical_red_indexes" | jq -r --argjson receiptIndex "$receipt_index" 'index($receiptIndex) != null')"
  fi

  if [[ "$is_historical" == "true" ]]; then
    historical=$((historical + 1))
  elif [[ -n "$reason" ]]; then
    stale=$((stale + 1))
    stale_json="$(printf '%s' "$stale_json" | jq --arg ts "$ts" --arg cmd "$cmd" --arg r "$reason" '. + [{ts: $ts, cmd: $cmd, reason: $r}]')"
  else
    valid=$((valid + 1))
  fi
done < "$current_receipts"

superseded=$((raw_total - current_total))

if [[ "$TRANSITION_ADMITTED" == "true" ]]; then
  jq -n \
    --argjson total "$raw_total" \
    --argjson current "$current_total" \
    --argjson superseded "$superseded" \
    --argjson withClosure "$with_closure" \
    --argjson valid "$valid" \
    --argjson historical "$historical" \
    --argjson stale "$stale" \
    --argjson unknown "$unknown" \
    --argjson staleReceipts "$stale_json" \
    '{total: $total, current: $current, superseded: $superseded, withClosure: $withClosure, valid: $valid, historical: $historical, stale: $stale, unknown: $unknown, staleReceipts: $staleReceipts}'
else
  jq -n \
    --argjson total "$raw_total" \
    --argjson current "$current_total" \
    --argjson superseded "$superseded" \
    --argjson withClosure "$with_closure" \
    --argjson valid "$valid" \
    --argjson stale "$stale" \
    --argjson unknown "$unknown" \
    --argjson staleReceipts "$stale_json" \
    '{total: $total, current: $current, superseded: $superseded, withClosure: $withClosure, valid: $valid, stale: $stale, unknown: $unknown, staleReceipts: $staleReceipts}'
fi

if [[ "$STRICT" == "true" && "$stale" -gt 0 ]]; then
  exit 1
fi
exit 0
