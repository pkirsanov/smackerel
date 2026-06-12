#!/usr/bin/env bash
set -euo pipefail

# capability-consumer-freshness.sh
#
# Gate G127 — capability_consumer_freshness_gate.
#
# The framework stops exempting itself from the rule it enforces downstream.
# G029 (integration-completeness) requires every shipped artifact in a product
# repo to have a real consumer; this guard applies the SAME standard to the
# framework's OWN capability ledger (`bubbles/capability-ledger.yaml`):
#
#   Every capability with `state: shipped` MUST declare a non-empty
#   `consumers:` list, AND every path in that list MUST exist on disk.
#
# This closes the "orphan foundation" failure mode (IMP-001 R2-B): a capability
# can be shipped with mechanism + lint + skill + docs yet have nothing in code
# that actually invokes it. Declaring real, existing consumers makes the wiring
# auditable and mechanically enforced.
#
# SCOPE (state-aware):
#   * `state: shipped`     → ENFORCED (non-empty consumers + every path exists).
#   * `state: partial`     → no-op (foundation still maturing).
#   * `state: proposed`    → no-op (not yet built).
#   * `state: deprecated`  → no-op (on the way out).
#
# This is an EXISTENCE check (G029 standard), not a semantic-reference check:
# it proves the declared consumer file is present, not that the file textually
# references the owner surface. Authoring discipline (choose consumers that
# genuinely invoke the owner) is the human contract; a future semantic-grep
# enhancement (the G097-style "named mechanism → grep the code" check) can layer
# on top without changing this gate's contract.
#
# Parser dependency: `yq` (mikefarah v4) for the YAML ledger. This is a BLOCKING
# gate, so a MISSING parser FAILS CLOSED (exit 1) with an actionable "install
# yq" message rather than silently passing — but ONLY for a repo that actually
# HAS the ledger. A repo with no `bubbles/capability-ledger.yaml` (every
# downstream product repo — the ledger is a framework-source-only artifact)
# no-ops (exit 0) via a cheap parser-free pre-check that runs BEFORE the
# fail-closed parser gate, so the same binary stays safe whether it scans the
# framework source tree or a downstream checkout.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist and
# never will; an orphan shipped capability is a real finding — wire a consumer
# or downgrade the `state`, do not silence the gate.
#
# Exit codes:
#   0  no-op (no ledger present) OR every shipped capability has a non-empty
#      consumers list whose every path exists.
#   1  blocking: missing parser (fail closed) when a ledger IS present, a
#      shipped capability with an empty/absent consumers list (orphan), or a
#      shipped capability naming a consumer path that does not exist (dangling).
#   2  usage error (unknown flag / too many positionals).
#
# Usage:
#   bash bubbles/scripts/capability-consumer-freshness.sh [--repo-root <dir>] [--quiet]
#   bash bubbles/scripts/capability-consumer-freshness.sh <dir>   # positional repo root
#
# Reference: improvements/IMP-004-capability-consumer-freshness.md (SCOPE-2).

# --- Resolve own location ------------------------------------------------
SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

QUIET="false"
REPO_ROOT_ARG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/capability-consumer-freshness.sh [options] [<repo-root>]

Gate G127 — capability_consumer_freshness_gate.
Every capability with `state: shipped` in bubbles/capability-ledger.yaml MUST
declare a non-empty `consumers:` list, and every listed path MUST exist on disk.
partial / proposed / deprecated capabilities are exempt (no-op).

Options:
  --repo-root <dir>  Repo root to scan (default: the repo this guard lives in).
  <repo-root>        Same as --repo-root, positional.
  --quiet            Suppress informational PASS/no-op stdout (warnings and
                     errors are still emitted).
  -h, --help         Print this usage and exit 0.

Exit codes:
  0 = no-op (no ledger present) OR all shipped capabilities have a non-empty
      consumers list whose every path exists
  1 = blocking: missing parser (fail closed, when a ledger is present),
      orphan shipped capability (empty/absent consumers), or dangling
      consumer path (declared but missing on disk)
  2 = usage error

This is a BLOCKING gate: a MISSING yq parser FAILS CLOSED (exit 1) when a
ledger is present; it does NOT silently pass. There is NO --skip/--force/--ignore
bypass.
EOF
}

# --- Argument parsing (builtins only) ------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --repo-root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "capability-consumer-freshness: --repo-root requires a directory argument" >&2
        usage >&2
        exit 2
      fi
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --*)
      echo "capability-consumer-freshness: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$REPO_ROOT_ARG" ]]; then
        REPO_ROOT_ARG="$1"
      else
        echo "capability-consumer-freshness: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

info() { [[ "$QUIET" == "true" ]] || echo "capability-consumer-freshness: $*"; }
warn() { echo "capability-consumer-freshness: $*" >&2; }
err()  { echo "capability-consumer-freshness: $*" >&2; }

# --- Repo-root resolution (builtins only) --------------------------------
if [[ -n "$REPO_ROOT_ARG" ]]; then
  REPO_ROOT="$REPO_ROOT_ARG"
elif [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
  REPO_ROOT="$BUBBLES_REPO_ROOT"
else
  # The guard lives at <repo-root>/bubbles/scripts/; climb two levels.
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd || true)"
fi

if [[ -z "$REPO_ROOT" || ! -d "$REPO_ROOT" ]]; then
  err "repo root does not resolve to a directory: '${REPO_ROOT:-<empty>}'"
  exit 1
fi

LEDGER="$REPO_ROOT/bubbles/capability-ledger.yaml"

# --- Parser-free non-adopter pre-check -----------------------------------
# The capability ledger is a framework-source-only artifact. A repo without it
# (every downstream product checkout) is a clean no-op — BEFORE the fail-closed
# parser gate, so wiring this guard anywhere never penalizes a repo that simply
# has no ledger and no yq.
if [[ ! -f "$LEDGER" ]]; then
  info "no capability ledger at bubbles/capability-ledger.yaml — no-op (exit 0)"
  exit 0
fi

# --- Fail-closed parser gate (blocking gate; ledger IS present) ----------
if ! command -v yq >/dev/null 2>&1; then
  err "yq (mikefarah v4) is REQUIRED to validate the capability ledger but was not found."
  err "This is a BLOCKING gate — it fails closed rather than skip. Install yq:"
  err "  https://github.com/mikefarah/yq#install"
  exit 1
fi

# --- Enforce: every shipped capability has real consumers ----------------
# Validate the .capabilities map parses FIRST — distinguish a MALFORMED ledger
# (missing/non-map .capabilities → hard error) from a ledger that simply
# declares zero shipped capabilities (a clean no-op, e.g. a proposal-only
# ledger). Without this split, a legitimate proposed-only ledger would falsely
# fail.
if ! yq -e '.capabilities | (tag == "!!map")' "$LEDGER" >/dev/null 2>&1; then
  err "capability ledger .capabilities is missing or not a map: $LEDGER"
  exit 1
fi

shipped_keys=()
mapfile -t shipped_keys < <(
  yq -r '.capabilities | to_entries[] | select(.value.state == "shipped") | .key' \
    "$LEDGER" 2>/dev/null || true
)

if [[ "${#shipped_keys[@]}" -eq 0 ]]; then
  info "ledger parses but declares no shipped capabilities — nothing to enforce (exit 0)"
  exit 0
fi

findings=0
checked_paths=0

for key in "${shipped_keys[@]}"; do
  consumers=()
  mapfile -t consumers < <(
    yq -r '.capabilities["'"$key"'"].consumers[]?' "$LEDGER" 2>/dev/null || true
  )

  # Count NON-EMPTY (substance) consumers. A list of only blank/whitespace
  # entries is an ORPHAN just like an absent list — "declares a consumer" means
  # a real path, not a blank line. Counting array size alone would let
  # `consumers: ["", ""]` pass with zero real consumers (shape-not-substance
  # hole — the exact FAILURE CONDITION this gate exists to close).
  real_consumers=0
  for consumer in "${consumers[@]}"; do
    [[ -n "${consumer//[[:space:]]/}" ]] && real_consumers=$((real_consumers + 1))
  done

  if [[ "$real_consumers" -eq 0 ]]; then
    err "ORPHAN: shipped capability '$key' declares no real consumers (empty/absent/blank-only consumers list)."
    err "        A shipped capability MUST name >=1 existing executable surface that uses it."
    err "        Wire a real consumer, or downgrade state to partial/proposed."
    findings=$((findings + 1))
    continue
  fi

  for consumer in "${consumers[@]}"; do
    # A blank/whitespace-only entry is MALFORMED — neither a real path nor a
    # deliberate omission. Fail loud rather than silently skipping it, so a
    # stray blank line can never dilute the consumer list undetected.
    if [[ -z "${consumer//[[:space:]]/}" ]]; then
      err "MALFORMED: shipped capability '$key' has a blank/empty consumer entry; remove it or replace with a real path."
      findings=$((findings + 1))
      continue
    fi
    if [[ -e "$REPO_ROOT/$consumer" ]]; then
      checked_paths=$((checked_paths + 1))
    else
      err "DANGLING: shipped capability '$key' names a consumer that does not exist: $consumer"
      findings=$((findings + 1))
    fi
  done
done

if [[ "$findings" -gt 0 ]]; then
  err "FAIL: $findings capability-consumer finding(s) across ${#shipped_keys[@]} shipped capabilities."
  exit 1
fi

info "OK: all ${#shipped_keys[@]} shipped capabilities declare consumers; $checked_paths consumer path(s) verified present."
exit 0
