#!/usr/bin/env bash
# mutable-dispatch-authorization.sh — mint and verify `mutable-dispatch-authorization/v1`
# (IMP-056 SCOPE-3).
#
# WHY THIS EXISTS
#
# Repository ownership (`repository-binding.sh`), exact boundary classification
# (`work-boundary-resolve.sh`), and a `goal-boundary-receipt/v1` each answer a
# DIFFERENT question. None of them answers "may THIS repository-mediated
# mutable child launch, with THESE exact action/executable/interpreter bytes
# and THIS one-use permit, proceed right now?" — and none of them was designed
# to. This script composes all three plus the current IMP-055 permit into one
# signed artifact so a later consumer (the SCOPE-4 gateway) can demand ONE
# verified thing rather than re-deriving the composition itself, which is
# exactly how GF-16/GF-17 (resolver success and repository ownership treated
# as authorization) happened in the first place.
#
# It mints NOTHING unless the three prerequisite checks actually run and pass:
#   1. `repository-binding.sh validate-packet` — current repository authority.
#   2. `work-boundary-resolve.sh` — EXACT `disposition=in-boundary` (checking
#      exit 0 alone is the GF-16 mistake; disposition is parsed explicitly).
#   3. `goal-boundary-receipt.sh verify` — the unchanged G134 receipt.
# Each runs in order; the first failure aborts with nothing printed and that
# tool's own exit code, mirroring `goal-boundary-receipt.sh emit`'s "the guard
# runs first" discipline. A receipt that could be minted without its checks
# passing would be exactly the theatre this script exists to avoid.
#
# WHAT THIS DOES NOT DO
#
# It does not call the reference broker, consume a permit, or launch anything.
# Wiring `verify` in before `launch-pending` and requiring the current permit
# at every mutable vector is SCOPE-4 (the canonical final-edge gateway). This
# script is the standalone cryptographic contract SCOPE-3 owns: schema, mint,
# verify, and the negative-coverage list. Reuses `security-authority.py` with
# purpose `mutable-dispatch-authorization` — no second key-loading or signing
# implementation is added.
#
# SCHEMA (mutable-dispatch-authorization/v1) — CLOSED field set:
#   contractType         "mutable-dispatch-authorization"
#   schemaVersion        1
#   repositoryAlias      from the validated packet
#   controlRevision      from packet.repositoryResolution.controlRevision
#   controlDecisionId    from packet.repositoryResolution.decisionId
#   boundary             from the G134 receipt (expected "pre-dispatch")
#   receiptDigest        the G134 receipt's own tamper-evident digest
#   actionDigest         sha256:... — MUST agree across broker snapshot AND permit
#   executableDigest     sha256:... from the broker snapshot's executable identity
#   interpreterDigest    sha256:... , or "sha256:absent" when no interpreter is
#                        involved (native-elf launch form)
#   permitId             the current dispatch-permit's permitId
#   reservationId        the current dispatch-permit's reservationId
#   occurrenceId          the current dispatch-permit's occurrenceId
#   enforcementKind       the current dispatch-permit's enforcementKind — closed
#                        to "repository-reference"; "host-native" is refused
#   authorityId, trustRootId   from the loaded authority
#   issuedAt, expiresAt        ISO-8601 millisecond UTC, short lifetime
#   authenticator              hmac-sha256:... over every field above
#
# Usage:
#   mutable-dispatch-authorization.sh mint --authority-file <path> \
#     --session-id <id> --session-control-file <path> --packet-file <path> \
#     --feature-dir <dir> --candidate-repo <slug> \
#     --receipt-file <path> --session-file <path> \
#     --broker-snapshot-file <path> --permit-file <path> \
#     [--lifetime-seconds <n>] [--now <iso8601>]
#
#   mutable-dispatch-authorization.sh verify --authority-file <path> \
#     --authorization-file <path> \
#     --session-id <id> --session-control-file <path> --packet-file <path> \
#     --receipt-file <path> --broker-snapshot-file <path> --permit-file <path> \
#     [--now <iso8601>]
#
# Exit codes:
#   0  minted / verified
#   1  REFUSED — a binding does not match, or an upstream check failed
#   2  usage or runtime error
#
# There is no --force / --skip / --assume-passed. A bypass-shaped flag on a
# fail-closed authorization contract is the exact failure mode this exists to
# prevent.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_BINDING="${BUBBLES_REPOSITORY_BINDING:-$SCRIPT_DIR/repository-binding.sh}"
BOUNDARY_RESOLVE="${BUBBLES_WORK_BOUNDARY_RESOLVE:-$SCRIPT_DIR/work-boundary-resolve.sh}"
BOUNDARY_RECEIPT="${BUBBLES_GOAL_BOUNDARY_RECEIPT:-$SCRIPT_DIR/goal-boundary-receipt.sh}"
SECURITY_AUTHORITY="${BUBBLES_SECURITY_AUTHORITY:-$SCRIPT_DIR/security-authority.py}"
CONTRACT_VERSION="mutable-dispatch-authorization/v1"
PURPOSE="mutable-dispatch-authorization"
DEFAULT_LIFETIME_SECONDS=120

usage() {
  cat <<'EOF'
Usage: mutable-dispatch-authorization.sh mint --authority-file <path> \
         --session-id <id> --session-control-file <path> --packet-file <path> \
         --feature-dir <dir> --candidate-repo <slug> \
         --receipt-file <path> --session-file <path> \
         --broker-snapshot-file <path> --permit-file <path> \
         [--lifetime-seconds <n>] [--now <iso8601>]

       mutable-dispatch-authorization.sh verify --authority-file <path> \
         --authorization-file <path> \
         --session-id <id> --session-control-file <path> --packet-file <path> \
         --receipt-file <path> --broker-snapshot-file <path> --permit-file <path> \
         [--now <iso8601>]

mint refuses (prints nothing, propagates the failing check's exit code) unless
--packet-file validates against --session-control-file, work-boundary-resolve
reports EXACTLY disposition=in-boundary for --candidate-repo under
--feature-dir, and --receipt-file verifies against --session-file. There is no
--force / --skip / --assume-passed.
EOF
}

fail_usage() {
  echo "mutable-dispatch-authorization: $*" >&2
  exit 2
}
fail_refuse() {
  echo "mutable-dispatch-authorization: REFUSED — $*" >&2
  exit 1
}

command -v jq >/dev/null 2>&1 || fail_usage "jq is required"
command -v python3 >/dev/null 2>&1 || fail_usage "python3 is required"

now_iso() {
  if [[ -n "${OVERRIDE_NOW:-}" ]]; then
    printf '%s' "$OVERRIDE_NOW"
    return
  fi
  date -u '+%Y-%m-%dT%H:%M:%S.000Z'
}

# Portable ISO-8601 (millisecond, Z-suffixed) -> epoch seconds. Neither GNU nor
# BSD `date -d` is assumed available on both platforms this framework targets,
# so the offset is computed once in Python, which every caller already needs.
epoch_of() {
  python3 -c '
import datetime as dt, sys
value = sys.argv[1]
try:
    parsed = dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc)
except ValueError:
    print("invalid", file=sys.stderr)
    raise SystemExit(1)
print(int(parsed.timestamp()))
' "$1"
}

add_seconds_iso() {
  local at="$1" seconds="$2"
  python3 -c '
import datetime as dt, sys
value, seconds = sys.argv[1], int(sys.argv[2])
parsed = dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc)
result = parsed + dt.timedelta(seconds=seconds)
print(result.strftime("%Y-%m-%dT%H:%M:%S.") + f"{result.microsecond // 1000:03d}Z")
' "$at" "$seconds"
}

# security-authority.py refuses any authority or payload path with a symlink
# component (its own hardening, not a quirk to route around). The default
# `mktemp` scratch area sits under a symlinked path on macOS ($TMPDIR ->
# /var/... -> /private/var/...), so every file this script hands to
# security-authority.py is created inside a directory canonicalized once via
# `cd -P`, the same fix repository-binding-selftest.sh applies to its own
# fixture root for the same reason.
canonical_scratch_file() {
  local dir
  dir="$(cd "$(mktemp -d)" && pwd -P)" || fail_usage "could not create a scratch directory"
  mktemp "$dir/tmp.XXXXXX"
}

sha256_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    echo "mutable-dispatch-authorization: sha256sum or shasum is required" >&2
    exit 2
  fi
}

# --- shared field derivation --------------------------------------------------
# Both mint and verify derive the SAME bindings from the SAME current-state
# files, so a divergence between mint-time and verify-time inputs is exactly
# what verify's field-by-field comparison is built to catch.

derive_snapshot_fields() {
  local snapshot_file="$1"
  [[ -f "$snapshot_file" ]] || fail_usage "broker snapshot file not found: $snapshot_file"
  jq empty "$snapshot_file" 2>/dev/null || fail_usage "broker snapshot file is not valid JSON: $snapshot_file"
  local expected_fields='["actionDigest","executable","executableFormat","launchForm","launchStrategy","platform","schemaVersion"]'
  jq -e --argjson expected "$expected_fields" \
    '(keys | sort) == ($expected | sort)' "$snapshot_file" >/dev/null 2>&1 ||
    fail_usage "broker snapshot file fields are not closed"
  SNAP_ACTION_DIGEST="$(jq -r '.actionDigest' "$snapshot_file")"
  SNAP_EXECUTABLE_DIGEST="$(jq -r '.executable.sha256' "$snapshot_file")"
  local launch_form
  launch_form="$(jq -r '.launchForm' "$snapshot_file")"
  case "$launch_form" in
    dash-inline-c) SNAP_INTERPRETER_DIGEST="$SNAP_EXECUTABLE_DIGEST" ;;
    native-elf) SNAP_INTERPRETER_DIGEST="sha256:absent" ;;
    *) fail_usage "broker snapshot launchForm '$launch_form' is not recognised" ;;
  esac
}

derive_permit_fields() {
  local permit_file="$1"
  [[ -f "$permit_file" ]] || fail_usage "permit file not found: $permit_file"
  jq empty "$permit_file" 2>/dev/null || fail_usage "permit file is not valid JSON: $permit_file"
  [[ "$(jq -r '.contractType // ""' "$permit_file")" == "dispatch-permit" ]] ||
    fail_usage "permit file is not a dispatch-permit record"
  PERMIT_ID="$(jq -r '.permitId // ""' "$permit_file")"
  PERMIT_RESERVATION_ID="$(jq -r '.reservationId // ""' "$permit_file")"
  PERMIT_OCCURRENCE_ID="$(jq -r '.occurrenceId // ""' "$permit_file")"
  PERMIT_ACTION_DIGEST="$(jq -r '.actionDigest // ""' "$permit_file")"
  PERMIT_ENFORCEMENT_KIND="$(jq -r '.enforcementKind // ""' "$permit_file")"
  [[ -n "$PERMIT_ID" && -n "$PERMIT_RESERVATION_ID" && -n "$PERMIT_OCCURRENCE_ID" && -n "$PERMIT_ACTION_DIGEST" ]] ||
    fail_usage "permit file is missing required fields"
  [[ "$PERMIT_ENFORCEMENT_KIND" == "repository-reference" ]] ||
    fail_refuse "permit enforcementKind '$PERMIT_ENFORCEMENT_KIND' is not repository-reference — host-native enforcement is outside IMP-056"
}

derive_receipt_fields() {
  local receipt_file="$1"
  [[ -f "$receipt_file" ]] || fail_usage "receipt file not found: $receipt_file"
  jq empty "$receipt_file" 2>/dev/null || fail_usage "receipt file is not valid JSON: $receipt_file"
  [[ "$(jq -r '.receiptVersion // ""' "$receipt_file")" == "goal-boundary-receipt/v1" ]] ||
    fail_usage "receipt file is not a goal-boundary-receipt/v1"
  RECEIPT_DIGEST="$(jq -r '.receiptDigest // ""' "$receipt_file")"
  RECEIPT_BOUNDARY="$(jq -r '.boundary // ""' "$receipt_file")"
  [[ -n "$RECEIPT_DIGEST" && -n "$RECEIPT_BOUNDARY" ]] || fail_usage "receipt file is missing required fields"
}

derive_packet_fields() {
  local packet_file="$1"
  [[ -f "$packet_file" ]] || fail_usage "packet file not found: $packet_file"
  jq empty "$packet_file" 2>/dev/null || fail_usage "packet file is not valid JSON: $packet_file"
  PACKET_REPOSITORY_ALIAS="$(jq -r '.repositoryAlias // ""' "$packet_file")"
  PACKET_CONTROL_REVISION="$(jq -r '.repositoryResolution.controlRevision // ""' "$packet_file")"
  PACKET_DECISION_ID="$(jq -r '.repositoryResolution.decisionId // ""' "$packet_file")"
  [[ -n "$PACKET_REPOSITORY_ALIAS" && -n "$PACKET_CONTROL_REVISION" && -n "$PACKET_DECISION_ID" ]] ||
    fail_usage "packet file is missing required fields"
}

# The two mutable-invariant cross-checks that are NOT per-field comparisons
# against the authorization: the action digest must agree across the broker
# snapshot AND the permit BEFORE anything is bound to it, because an
# authorization that faithfully bound two disagreeing sources would still be
# internally consistent and still wrong.
require_action_digest_agreement() {
  [[ "$SNAP_ACTION_DIGEST" == "$PERMIT_ACTION_DIGEST" ]] ||
    fail_refuse "broker snapshot actionDigest does not match the permit's actionDigest — they must authorize the same bytes"
}

# --- mint ---------------------------------------------------------------------
cmd_mint() {
  local authority_file="" session_id="" control_file="" packet_file=""
  local feature_dir="" candidate_repo="" receipt_file="" session_file=""
  local snapshot_file="" permit_file="" lifetime="$DEFAULT_LIFETIME_SECONDS"
  OVERRIDE_NOW=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --authority-file) authority_file="${2:-}"; shift 2 ;;
      --session-id) session_id="${2:-}"; shift 2 ;;
      --session-control-file) control_file="${2:-}"; shift 2 ;;
      --packet-file) packet_file="${2:-}"; shift 2 ;;
      --feature-dir) feature_dir="${2:-}"; shift 2 ;;
      --candidate-repo) candidate_repo="${2:-}"; shift 2 ;;
      --receipt-file) receipt_file="${2:-}"; shift 2 ;;
      --session-file) session_file="${2:-}"; shift 2 ;;
      --broker-snapshot-file) snapshot_file="${2:-}"; shift 2 ;;
      --permit-file) permit_file="${2:-}"; shift 2 ;;
      --lifetime-seconds) lifetime="${2:-}"; shift 2 ;;
      --now) OVERRIDE_NOW="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      --force|--skip|--assume-passed|--no-verify)
        fail_usage "bypass-shaped flag '$1' does not exist" ;;
      *) fail_usage "unknown mint option: $1" ;;
    esac
  done
  for pair in "authority-file:$authority_file" "session-id:$session_id" \
    "session-control-file:$control_file" "packet-file:$packet_file" \
    "feature-dir:$feature_dir" "candidate-repo:$candidate_repo" \
    "receipt-file:$receipt_file" "session-file:$session_file" \
    "broker-snapshot-file:$snapshot_file" "permit-file:$permit_file"; do
    [[ -n "${pair#*:}" ]] || fail_usage "mint requires --${pair%%:*}"
  done
  [[ "$lifetime" =~ ^[1-9][0-9]*$ ]] || fail_usage "--lifetime-seconds must be a positive integer"

  # THE ORDERING THAT MATTERS. Each prerequisite runs BEFORE anything is built,
  # and a failure propagates that check's own exit code with nothing printed —
  # exactly goal-boundary-receipt.sh's discipline, applied to three checks
  # instead of one.
  local rc=0
  bash "$REPO_BINDING" validate-packet --session-id "$session_id" \
    --session-control-file "$control_file" --packet-file "$packet_file" >&2 || rc=$?
  [[ "$rc" -eq 0 ]] || { echo "mutable-dispatch-authorization: no authorization minted — repository packet did not validate" >&2; exit "$rc"; }

  local boundary_output
  rc=0
  boundary_output="$(bash "$BOUNDARY_RESOLVE" --feature-dir "$feature_dir" --candidate-repo "$candidate_repo" --strict 2>&2)" || rc=$?
  [[ "$rc" -eq 0 ]] || { echo "mutable-dispatch-authorization: no authorization minted — boundary resolution failed" >&2; exit "$rc"; }
  # GF-16's own lesson: exit 0 is necessary but not sufficient. The
  # disposition must be parsed and be EXACTLY in-boundary.
  if [[ "$boundary_output" != *"disposition=in-boundary"* ]]; then
    fail_refuse "boundary disposition is not exactly in-boundary: $boundary_output"
  fi

  rc=0
  bash "$BOUNDARY_RECEIPT" verify --receipt-file "$receipt_file" --session-file "$session_file" \
    --expect-boundary pre-dispatch >&2 || rc=$?
  [[ "$rc" -eq 0 ]] || { echo "mutable-dispatch-authorization: no authorization minted — G134 receipt did not verify" >&2; exit "$rc"; }

  derive_packet_fields "$packet_file"
  derive_receipt_fields "$receipt_file"
  derive_snapshot_fields "$snapshot_file"
  derive_permit_fields "$permit_file"
  require_action_digest_agreement

  local issued_at expires_at payload_file authenticator_json authenticator
  issued_at="$(now_iso)"
  expires_at="$(add_seconds_iso "$issued_at" "$lifetime")"

  # security-authority.py has no "identify only" mode, so authorityId and
  # trustRootId are read the same way every other caller reads them: sign an
  # arbitrary closed payload and take the identity fields from the result,
  # never from a caller-supplied value. The probe payload itself is discarded.
  local probe_file probe_result authority_id trust_root_id
  probe_file="$(canonical_scratch_file)"
  printf '{}' >"$probe_file"
  chmod 600 "$probe_file"
  probe_result="$(python3 "$SECURITY_AUTHORITY" sign "$PURPOSE" "$authority_file" "$probe_file")" ||
    { rm -rf "$(dirname "$probe_file")"; fail_usage "authority file is invalid for purpose $PURPOSE"; }
  rm -rf "$(dirname "$probe_file")"
  authority_id="$(jq -r '.authorityId' <<< "$probe_result")"
  trust_root_id="$(jq -r '.trustRootId' <<< "$probe_result")"

  local body
  body="$(jq -n \
    --arg ct "$CONTRACT_VERSION" \
    --arg alias "$PACKET_REPOSITORY_ALIAS" \
    --argjson rev "$PACKET_CONTROL_REVISION" \
    --arg dec "$PACKET_DECISION_ID" \
    --arg boundary "$RECEIPT_BOUNDARY" \
    --arg rd "$RECEIPT_DIGEST" \
    --arg ad "$SNAP_ACTION_DIGEST" \
    --arg ed "$SNAP_EXECUTABLE_DIGEST" \
    --arg id "$SNAP_INTERPRETER_DIGEST" \
    --arg pid "$PERMIT_ID" \
    --arg res "$PERMIT_RESERVATION_ID" \
    --arg occ "$PERMIT_OCCURRENCE_ID" \
    --arg ek "$PERMIT_ENFORCEMENT_KIND" \
    --arg aid "$authority_id" \
    --arg trid "$trust_root_id" \
    --arg iat "$issued_at" \
    --arg exp "$expires_at" \
    '{
      contractType: $ct,
      schemaVersion: 1,
      repositoryAlias: $alias,
      controlRevision: $rev,
      controlDecisionId: $dec,
      boundary: $boundary,
      receiptDigest: $rd,
      actionDigest: $ad,
      executableDigest: $ed,
      interpreterDigest: $id,
      permitId: $pid,
      reservationId: $res,
      occurrenceId: $occ,
      enforcementKind: $ek,
      authorityId: $aid,
      trustRootId: $trid,
      issuedAt: $iat,
      expiresAt: $exp
    }')"

  payload_file="$(canonical_scratch_file)"
  trap 'rm -rf "$(dirname "$payload_file")"' RETURN
  printf '%s' "$body" >"$payload_file"
  chmod 600 "$payload_file"
  authenticator_json="$(python3 "$SECURITY_AUTHORITY" sign "$PURPOSE" "$authority_file" "$payload_file")" ||
    fail_usage "signing failed"
  authenticator="$(jq -r '.authenticator' <<< "$authenticator_json")"
  jq -n --argjson body "$body" --arg auth "$authenticator" '$body + { authenticator: $auth }'
}

# --- verify ---------------------------------------------------------------------
cmd_verify() {
  local authority_file="" authorization_file="" session_id="" control_file="" packet_file=""
  local receipt_file="" snapshot_file="" permit_file=""
  OVERRIDE_NOW=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --authority-file) authority_file="${2:-}"; shift 2 ;;
      --authorization-file) authorization_file="${2:-}"; shift 2 ;;
      --session-id) session_id="${2:-}"; shift 2 ;;
      --session-control-file) control_file="${2:-}"; shift 2 ;;
      --packet-file) packet_file="${2:-}"; shift 2 ;;
      --receipt-file) receipt_file="${2:-}"; shift 2 ;;
      --broker-snapshot-file) snapshot_file="${2:-}"; shift 2 ;;
      --permit-file) permit_file="${2:-}"; shift 2 ;;
      --now) OVERRIDE_NOW="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      --force|--skip|--assume-passed|--no-verify)
        fail_usage "bypass-shaped flag '$1' does not exist" ;;
      *) fail_usage "unknown verify option: $1" ;;
    esac
  done
  for pair in "authority-file:$authority_file" "authorization-file:$authorization_file" \
    "session-id:$session_id" "session-control-file:$control_file" "packet-file:$packet_file" \
    "receipt-file:$receipt_file" "broker-snapshot-file:$snapshot_file" "permit-file:$permit_file"; do
    [[ -n "${pair#*:}" ]] || fail_usage "verify requires --${pair%%:*}"
  done
  [[ -f "$authorization_file" ]] || fail_usage "authorization file not found: $authorization_file"
  jq empty "$authorization_file" 2>/dev/null || fail_usage "authorization file is not valid JSON: $authorization_file"

  local closed_fields='["contractType","schemaVersion","repositoryAlias","controlRevision","controlDecisionId","boundary","receiptDigest","actionDigest","executableDigest","interpreterDigest","permitId","reservationId","occurrenceId","enforcementKind","authorityId","trustRootId","issuedAt","expiresAt","authenticator"]'
  jq -e --argjson expected "$closed_fields" '(keys | sort) == ($expected | sort)' "$authorization_file" >/dev/null 2>&1 ||
    fail_refuse "authorization fields are not closed — an altered or substituted authorization is rejected on shape alone"

  local a_version
  a_version="$(jq -r '.contractType' "$authorization_file")"
  [[ "$a_version" == "$CONTRACT_VERSION" ]] || fail_refuse "authorization contractType '$a_version' is not '$CONTRACT_VERSION'"
  [[ "$(jq -r '.schemaVersion' "$authorization_file")" == "1" ]] || fail_refuse "authorization schemaVersion is unsupported"

  # Cryptographic check FIRST, exactly as goal-boundary-receipt.sh checks tamper
  # before comparing to current state: a binding comparison against an already-
  # forged payload proves nothing.
  python3 "$SECURITY_AUTHORITY" verify "$PURPOSE" "$authority_file" "$authorization_file" >/dev/null 2>&1 ||
    fail_refuse "authenticator is invalid, or authority/trust-root/purpose is wrong"

  derive_packet_fields "$packet_file"
  derive_receipt_fields "$receipt_file"
  derive_snapshot_fields "$snapshot_file"
  derive_permit_fields "$permit_file"
  require_action_digest_agreement

  # Expiry BEFORE binding checks — an authorization that is otherwise perfectly
  # bound but time-expired must still refuse.
  local issued_at expires_at now issued_epoch expires_epoch now_epoch
  issued_at="$(jq -r '.issuedAt' "$authorization_file")"
  expires_at="$(jq -r '.expiresAt' "$authorization_file")"
  now="$(now_iso)"
  issued_epoch="$(epoch_of "$issued_at")" || fail_refuse "authorization issuedAt is invalid"
  expires_epoch="$(epoch_of "$expires_at")" || fail_refuse "authorization expiresAt is invalid"
  now_epoch="$(epoch_of "$now")" || fail_usage "current time could not be computed"
  [[ "$expires_epoch" -gt "$issued_epoch" ]] || fail_refuse "authorization expiresAt is not after issuedAt"
  [[ "$now_epoch" -le "$expires_epoch" ]] || fail_refuse "authorization is expired"
  [[ "$now_epoch" -ge "$issued_epoch" ]] || fail_refuse "authorization issuedAt is in the future"

  local a_alias a_rev a_dec a_boundary a_rd a_ad a_ed a_id a_pid a_res a_occ a_ek
  a_alias="$(jq -r '.repositoryAlias' "$authorization_file")"
  a_rev="$(jq -r '.controlRevision' "$authorization_file")"
  a_dec="$(jq -r '.controlDecisionId' "$authorization_file")"
  a_boundary="$(jq -r '.boundary' "$authorization_file")"
  a_rd="$(jq -r '.receiptDigest' "$authorization_file")"
  a_ad="$(jq -r '.actionDigest' "$authorization_file")"
  a_ed="$(jq -r '.executableDigest' "$authorization_file")"
  a_id="$(jq -r '.interpreterDigest' "$authorization_file")"
  a_pid="$(jq -r '.permitId' "$authorization_file")"
  a_res="$(jq -r '.reservationId' "$authorization_file")"
  a_occ="$(jq -r '.occurrenceId' "$authorization_file")"
  a_ek="$(jq -r '.enforcementKind' "$authorization_file")"

  [[ "$a_alias" == "$PACKET_REPOSITORY_ALIAS" ]] ||
    fail_refuse "authorization repositoryAlias does not match the current packet — wrong-repository"
  [[ "$a_rev" == "$PACKET_CONTROL_REVISION" ]] ||
    fail_refuse "authorization controlRevision $a_rev does not match the current packet revision $PACKET_CONTROL_REVISION — wrong-revision"
  [[ "$a_dec" == "$PACKET_DECISION_ID" ]] ||
    fail_refuse "authorization controlDecisionId does not match the current packet — wrong-repository"
  [[ "$a_boundary" == "pre-dispatch" && "$a_boundary" == "$RECEIPT_BOUNDARY" ]] ||
    fail_refuse "authorization boundary does not match the current receipt's boundary — wrong-receipt"
  [[ "$a_rd" == "$RECEIPT_DIGEST" ]] ||
    fail_refuse "authorization receiptDigest does not match the current receipt — wrong-receipt"
  [[ "$a_ad" == "$SNAP_ACTION_DIGEST" && "$a_ad" == "$PERMIT_ACTION_DIGEST" ]] ||
    fail_refuse "authorization actionDigest does not match the current broker snapshot and permit — wrong-action"
  [[ "$a_ed" == "$SNAP_EXECUTABLE_DIGEST" ]] ||
    fail_refuse "authorization executableDigest does not match the current broker snapshot — wrong-executable"
  [[ "$a_id" == "$SNAP_INTERPRETER_DIGEST" ]] ||
    fail_refuse "authorization interpreterDigest does not match the current broker snapshot — wrong-interpreter"
  [[ "$a_pid" == "$PERMIT_ID" ]] ||
    fail_refuse "authorization permitId does not match the current permit"
  [[ "$a_res" == "$PERMIT_RESERVATION_ID" ]] ||
    fail_refuse "authorization reservationId does not match the current permit"
  [[ "$a_occ" == "$PERMIT_OCCURRENCE_ID" ]] ||
    fail_refuse "authorization occurrenceId does not match the current permit"
  [[ "$a_ek" == "$PERMIT_ENFORCEMENT_KIND" ]] ||
    fail_refuse "authorization enforcementKind does not match the current permit"

  echo "mutable-dispatch-authorization: OK (repository '$a_alias' revision $a_rev, permit $a_pid)"
}

case "${1:-}" in
  mint) shift; cmd_mint "$@" ;;
  verify) shift; cmd_verify "$@" ;;
  -h|--help|"") usage; [[ -n "${1:-}" ]] && exit 0 || exit 2 ;;
  *) fail_usage "unknown subcommand: $1" ;;
esac
