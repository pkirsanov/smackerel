#!/usr/bin/env bash
# mutable-dispatch-gateway.sh — the canonical final-edge gateway for a
# repository-mediated mutable child launch (IMP-056 SCOPE-4).
#
# WHY THIS EXISTS
#
# `reference-broker.sh` (IMP-056 SCOPE-2) is a hardened, mechanical launcher:
# given a permit and an action, it snapshots the bytes once, consumes the
# permit, and launches from an immutable copy. It has no idea what repository,
# boundary, or goal the caller is acting under -- it only knows permits and
# bytes. `mutable-dispatch-authorization.sh` (SCOPE-3) knows how to compose
# repository authority, exact boundary classification, and the G134 receipt
# into one signed artifact, but it never launches anything itself.
#
# Neither of those, alone or called separately by whatever orchestrator feels
# like it, is "no orchestrator, adapter, or command class receives an implicit
# exemption" (SCOPE-4's own requirement). This script is the one place that
# composes both and is the ONLY sanctioned way to reach the broker for a
# mutable dispatch. It resolves `dispatchAdmission.adapter` before anything
# else: absence or `none` refuses (GF-16/GF-17's own lesson generalized -- no
# fallback path exists for "adapter not configured"), and any adapter other
# than `reference-broker` refuses too, because IMP-057's native-host adapter
# is not built and this proposal does not pretend otherwise.
#
# THE TOCTOU CONSTRAINT THAT SHAPES THIS SCRIPT
#
# Authorization Invariant #5 binds "the immutable executable and interpreter
# identities SELECTED BY THE BROKER" -- meaning the authorization must be
# minted against the SAME snapshot bytes that are later launched, not a
# separately-derived copy. Snapshotting the action twice (once here to mint
# against, once inside the broker to launch) would reopen exactly the gap
# broker-owned snapshotting (SCOPE-2) exists to close: the executable could be
# swapped at its original mutable path between the two copies. So this script
# creates the snapshot ITSELF, mints and verifies the authorization against
# it, and then hands that SAME immutable directory to
# `reference-broker.sh dispatch --existing-snapshot-dir`, which reuses it
# rather than re-deriving it.
#
# WHAT THIS DOES NOT DO
#
# It does not remove `phase-coordinator.sh`'s direct broker calls or migrate
# the four mutable orchestrators -- that is SCOPE-6. Until SCOPE-6 lands,
# calling `reference-broker.sh` directly still works and still gets its own
# full snapshot/permit hardening; it just does not get this composed
# authorization. This script existing does not retroactively close that door.
#
# Usage:
#   mutable-dispatch-gateway.sh dispatch \
#     --repo-root PATH --store-root PATH --authority-file PATH \
#     --session-id ID --session-control-file PATH --packet-file PATH \
#     --feature-dir DIR --candidate-repo SLUG \
#     --receipt-file PATH --session-file PATH \
#     --permit-id ID --action FILE --permit-consumption FILE \
#     [--lifetime-seconds N] [--now ISO8601]
#
# Exit codes:
#   0  dispatched (the broker's own result is printed; its exit code is
#      propagated verbatim for the child's actual outcome)
#   1  REFUSED — adapter is not reference-broker, or an authorization
#      invariant did not hold. Nothing was launched.
#   2  usage or runtime error
#
# There is no --force / --skip / --assume-passed. A bypass-shaped flag on the
# one place authorization is composed is the exact failure this exists to
# prevent.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ADAPTER_RESOLVE="${BUBBLES_DISPATCH_ADAPTER_RESOLVE:-$SCRIPT_DIR/dispatch-adapter-resolve.sh}"
AUTHORIZATION="${BUBBLES_MUTABLE_DISPATCH_AUTHORIZATION:-$SCRIPT_DIR/mutable-dispatch-authorization.sh}"
SNAPSHOT_HELPER="${BUBBLES_REFERENCE_BROKER_SNAPSHOT:-$SCRIPT_DIR/reference-broker-snapshot.py}"
BROKER="${BUBBLES_REFERENCE_BROKER:-$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh}"
ENGINE="${BUBBLES_MEASURED_BUDGET_RUNTIME:-$SCRIPT_DIR/measured-budget-runtime.py}"

usage() {
  cat <<'EOF'
Usage: mutable-dispatch-gateway.sh dispatch \
  --repo-root PATH --store-root PATH --authority-file PATH \
  --session-id ID --session-control-file PATH --packet-file PATH \
  --feature-dir DIR --candidate-repo SLUG \
  --receipt-file PATH --session-file PATH \
  --permit-id ID --action FILE --permit-consumption FILE \
  [--lifetime-seconds N] [--now ISO8601]

Resolves dispatchAdmission.adapter for --repo-root; refuses (exit 1) unless it
is exactly "reference-broker". Snapshots the action once, mints and verifies a
mutable-dispatch-authorization against that exact snapshot, and only then
invokes the broker. Refuses with nothing launched on the first failure.

There is no --force / --skip / --assume-passed.
EOF
}

fail_usage() {
  echo "mutable-dispatch-gateway: $*" >&2
  exit 2
}
fail_refuse() {
  echo "mutable-dispatch-gateway: REFUSED — $*" >&2
  exit 1
}

command -v jq >/dev/null 2>&1 || fail_usage "jq is required"
command -v python3 >/dev/null 2>&1 || fail_usage "python3 is required"

[[ "${1:-}" == "dispatch" ]] || { usage; exit 2; }
shift

repo_root="" store_root="" authority_file=""
session_id="" control_file="" packet_file=""
feature_dir="" candidate_repo=""
receipt_file="" session_file=""
permit_id="" action_file="" consumption_file=""
lifetime="" now_override=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root) repo_root="${2:-}"; shift 2 ;;
    --store-root) store_root="${2:-}"; shift 2 ;;
    --authority-file) authority_file="${2:-}"; shift 2 ;;
    --session-id) session_id="${2:-}"; shift 2 ;;
    --session-control-file) control_file="${2:-}"; shift 2 ;;
    --packet-file) packet_file="${2:-}"; shift 2 ;;
    --feature-dir) feature_dir="${2:-}"; shift 2 ;;
    --candidate-repo) candidate_repo="${2:-}"; shift 2 ;;
    --receipt-file) receipt_file="${2:-}"; shift 2 ;;
    --session-file) session_file="${2:-}"; shift 2 ;;
    --permit-id) permit_id="${2:-}"; shift 2 ;;
    --action) action_file="${2:-}"; shift 2 ;;
    --permit-consumption) consumption_file="${2:-}"; shift 2 ;;
    --lifetime-seconds) lifetime="${2:-}"; shift 2 ;;
    --now) now_override="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    --force|--skip|--assume-passed|--no-verify)
      fail_usage "bypass-shaped flag '$1' does not exist" ;;
    *) fail_usage "unknown option: $1" ;;
  esac
done
for pair in "repo-root:$repo_root" "store-root:$store_root" "authority-file:$authority_file" \
  "session-id:$session_id" "session-control-file:$control_file" "packet-file:$packet_file" \
  "feature-dir:$feature_dir" "candidate-repo:$candidate_repo" \
  "receipt-file:$receipt_file" "session-file:$session_file" \
  "permit-id:$permit_id" "action:$action_file" "permit-consumption:$consumption_file"; do
  [[ -n "${pair#*:}" ]] || fail_usage "dispatch requires --${pair%%:*}"
done

# --- adapter resolution: the FIRST gate, before anything else runs ----------
adapter_line="$(bash "$ADAPTER_RESOLVE" --repo-root "$repo_root" --names-only 2>&1)" || {
  fail_refuse "dispatchAdmission.adapter could not be resolved: $adapter_line"
}
adapter="${adapter_line#adapter=}"
if [[ "$adapter" == "none" ]]; then
  fail_refuse "dispatchAdmission.adapter is 'none' — absence or none admits no mutable child dispatch"
fi
[[ "$adapter" == "reference-broker" ]] || fail_refuse "dispatchAdmission.adapter '$adapter' is not reference-broker — no other enforcement kind is authorized by this gateway"

# --- one snapshot, created here, minted against, and reused by the broker --
snapshot_dir="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-mutable-dispatch-gateway.XXXXXX")"
chmod 700 "$snapshot_dir"
trap 'rm -rf "$snapshot_dir"' EXIT INT TERM

if ! python3 "$SNAPSHOT_HELPER" --action "$action_file" --permit-consumption "$consumption_file" --output-dir "$snapshot_dir" 2>"$snapshot_dir/.snapshot-error"; then
  cat "$snapshot_dir/.snapshot-error" >&2
  fail_refuse "the broker's own snapshot validation refused this action before any authorization was attempted"
fi

permit_probe="$snapshot_dir/.permit-get-input.json"
jq -n --arg pid "$permit_id" '{permit_id:$pid}' >"$permit_probe"
chmod 600 "$permit_probe"
permit_file="$snapshot_dir/.permit.json"
python3 "$ENGINE" permit-get --store-root "$store_root" --input "$permit_probe" >"$permit_file" || \
  fail_refuse "permit $permit_id could not be read from the store"
chmod 600 "$permit_file"

mint_args=(mint --authority-file "$authority_file" \
  --session-id "$session_id" --session-control-file "$control_file" --packet-file "$packet_file" \
  --feature-dir "$feature_dir" --candidate-repo "$candidate_repo" \
  --receipt-file "$receipt_file" --session-file "$session_file" \
  --broker-snapshot-file "$snapshot_dir/snapshot-metadata.json" --permit-file "$permit_file")
[[ -n "$lifetime" ]] && mint_args+=(--lifetime-seconds "$lifetime")
[[ -n "$now_override" ]] && mint_args+=(--now "$now_override")

# Named, not dot-prefixed: IMP-056 SCOPE-5's post-execution audit contract
# reads this file, when present, to report which authorization the launch it
# is describing actually used. It stays PRIVATE (0600, inside the 0700
# snapshot_dir) either way; the name is discoverability, not exposure.
authorization_file="$snapshot_dir/authorization.json"
if ! bash "$AUTHORIZATION" "${mint_args[@]}" >"$authorization_file" 2>"$snapshot_dir/.mint-error"; then
  cat "$snapshot_dir/.mint-error" >&2
  fail_refuse "authorization could not be minted — repository, boundary, or receipt invariant did not hold"
fi
chmod 600 "$authorization_file"

verify_args=(verify --authority-file "$authority_file" --authorization-file "$authorization_file" \
  --session-id "$session_id" --session-control-file "$control_file" --packet-file "$packet_file" \
  --receipt-file "$receipt_file" --broker-snapshot-file "$snapshot_dir/snapshot-metadata.json" --permit-file "$permit_file")
[[ -n "$now_override" ]] && verify_args+=(--now "$now_override")
if ! bash "$AUTHORIZATION" "${verify_args[@]}" >/dev/null 2>"$snapshot_dir/.verify-error"; then
  cat "$snapshot_dir/.verify-error" >&2
  fail_refuse "the just-minted authorization did not verify — refusing rather than trusting mint alone"
fi

# --- every invariant held. Only now does the broker ever run. --------------
# Not `exec`: the EXIT trap above must still fire to clean up snapshot_dir,
# which `exec` would skip by replacing this process instead of letting it
# return from its own script.
broker_rc=0
bash "$BROKER" dispatch --store-root "$store_root" --permit-consumption "$consumption_file" \
  --action "$action_file" --existing-snapshot-dir "$snapshot_dir" || broker_rc=$?
exit "$broker_rc"
