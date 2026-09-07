#!/usr/bin/env bash
# Hermetic repository-reference broker. This is not a native host interceptor.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$(cd "$SCRIPT_DIR/../../scripts" && pwd)"
ENGINE="$SCRIPTS_DIR/measured-budget-runtime.py"
FINALIZER="$SCRIPTS_DIR/measured-budget-broker-finalize.py"
SNAPSHOT_HELPER="$SCRIPTS_DIR/reference-broker-snapshot.py"
USAGE_ADAPTER="$SCRIPT_DIR/../usage/reference-test.sh"

usage() {
  cat >&2 <<'EOF'
Usage: reference-broker.sh capabilities
       reference-broker.sh dispatch --store-root PATH --permit-consumption FILE
         --action FILE [--existing-snapshot-dir PATH]

The action file is {"argv":[...],"actionDigest":"sha256:..."}. The digest is
sha256 over canonical JSON {"argv":[...]}. The consumption file carries every
permit binding required by MBE-1. Native VS Code, MCP, and ambient interception
are unsupported. This broker gates only the child argv routed through it.

--existing-snapshot-dir is for IMP-056 SCOPE-4's canonical gateway only: a
directory the gateway already populated via reference-broker-snapshot.py,
using the SAME immutable action/executable bytes it minted a
mutable-dispatch-authorization against. Passing it skips re-creating the
snapshot (which would otherwise reopen a TOCTOU window between the gateway's
mint and this broker's own launch) and reuses that directory as-is. Omit it
for a direct, unauthorized-by-a-gateway call, which still gets the full
broker-owned snapshot hardening on its own.
EOF
}

digest_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    echo "reference-broker: sha256 utility unavailable" >&2
    return 2
  fi
}

[[ $# -gt 0 ]] || { usage; exit 2; }
verb="$1"
shift
if [[ "$verb" == "capabilities" ]]; then
  printf '%s\n' '{"adapterId":"reference-broker","ambientInterception":"unsupported","descriptorExec":"unsupported","enforcementKind":"repository-reference","executableSnapshot":{"launchStrategy":"broker-owned-copy","permitted":["linux-regular-native-elf-without-existing-absolute-path-arguments","linux-dash-inline-c"],"refused":["explicit-interpreter-with-script-path","group-world-writable-executable","mach-o","script-pathname","shebang","symlink","unknown-format"]},"mcpInterception":"unsupported","nativeVsCodeInterception":"unsupported"}'
  exit 0
fi
[[ "$verb" == "dispatch" ]] || { echo "reference-broker: unsupported verb '$verb'" >&2; exit 2; }

store_root=""
consumption_file=""
action_file=""
existing_snapshot_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --store-root) store_root="${2:-}"; shift 2 ;;
    --permit-consumption) consumption_file="${2:-}"; shift 2 ;;
    --action) action_file="${2:-}"; shift 2 ;;
    --existing-snapshot-dir) existing_snapshot_dir="${2:-}"; shift 2 ;;
    *) echo "reference-broker: unknown option '$1'" >&2; exit 2 ;;
  esac
done
for required in "$store_root" "$consumption_file" "$action_file"; do
  [[ -n "$required" ]] || { usage; exit 2; }
done
command -v jq >/dev/null 2>&1 || { echo "reference-broker: jq is required" >&2; exit 2; }

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-reference-broker.XXXXXX")"
chmod 700 "$work_dir"
trap 'rm -rf "$work_dir"' EXIT INT TERM

if [[ -n "$existing_snapshot_dir" ]]; then
  # The gateway already built this snapshot and minted an authorization
  # against its exact bytes. Re-creating it here would copy the executable a
  # second time from its original mutable path -- exactly the TOCTOU gap
  # broker-owned snapshotting exists to close -- so it is used as-is.
  snapshot_dir="$existing_snapshot_dir"
  [[ -d "$snapshot_dir" ]] || { echo "reference-broker: existing snapshot directory not found: $snapshot_dir" >&2; exit 2; }
  for required_snapshot_file in action.json permit-consumption.json snapshot-metadata.json executable; do
    [[ -e "$snapshot_dir/$required_snapshot_file" ]] || {
      echo "reference-broker: existing snapshot directory is incomplete (missing $required_snapshot_file)" >&2
      exit 2
    }
  done
else
  snapshot_dir="$work_dir"
  python3 "$SNAPSHOT_HELPER" --action "$action_file" --permit-consumption "$consumption_file" --output-dir "$snapshot_dir"
fi
snapshot_action="$snapshot_dir/action.json"
snapshot_consumption="$snapshot_dir/permit-consumption.json"

permit_id="$(jq -r '.permit_id // .permitId // ""' "$snapshot_consumption")"
nonce="$(jq -r '.nonce // ""' "$snapshot_consumption")"
at="$(jq -r '.consumed_at // .consumedAt // ""' "$snapshot_consumption")"
[[ -n "$permit_id" && -n "$nonce" && -n "$at" ]] || {
  echo "reference-broker: permit identity or dispatch time unavailable" >&2
  exit 2
}

# IMP-056 SCOPE-2 (Launch State And Settlement). launch-pending is durable
# BEFORE the permit is consumed: if the broker dies anywhere between here and
# a terminal record, that pending record is the only evidence process
# creation was ever attempted, and launch-reconcile is what later resolves it
# to launch-ambiguous rather than leaving the question unanswerable.
jq -n --arg pid "$permit_id" --arg nonce "$nonce" --arg at "$at" \
  '{permit_id:$pid,nonce:$nonce,recorded_at:$at}' >"$work_dir/launch-pending-input.json"
chmod 600 "$work_dir/launch-pending-input.json"
python3 "$ENGINE" launch-pending --store-root "$store_root" --input "$work_dir/launch-pending-input.json" >/dev/null

# MBE validates every permit binding, expiry, and one-use state under the ECF
# lock. A failed consumption exits before the child can run.
python3 "$ENGINE" permit-consume --store-root "$store_root" --input "$snapshot_consumption" >/dev/null

argv_index=0
while IFS= read -r item; do
  : "$item"
  argv_index=$((argv_index + 1))
done < <(jq -r '.argv[1:][]' "$snapshot_action")
expected_argc="$(jq -r '.argv | length - 1' "$snapshot_action")"
[[ "$argv_index" -eq "$expected_argc" ]] || {
  echo "reference-broker: action argv could not be loaded exactly" >&2
  exit 2
}

jq -n --arg pid "$permit_id" --arg at "$at" '{permit_id:$pid,recorded_at:$at}' \
  >"$work_dir/launch-terminal-input.json"
chmod 600 "$work_dir/launch-terminal-input.json"

launch_rc=0
python3 "$SNAPSHOT_HELPER" launch --output-dir "$snapshot_dir" || launch_rc=$?
if [[ "$launch_rc" -ne 0 ]] || [[ ! -f "$snapshot_dir/child-status" ]]; then
  # Launch preparation failed strictly AFTER the permit was consumed but
  # BEFORE process creation could be confirmed. This is launch-denied, not
  # launch-ambiguous: the failure is observed here, in this process, not
  # inferred later by a recovery pass over an unresolved pending record.
  python3 "$ENGINE" launch-deny --store-root "$store_root" --input "$work_dir/launch-terminal-input.json" >/dev/null
  echo "reference-broker: launch preparation failed after permit consumption" >&2
  exit 4
fi
launch_confirm_record="$(python3 "$ENGINE" launch-confirm --store-root "$store_root" --input "$work_dir/launch-terminal-input.json")"

child_status="$(<"$snapshot_dir/child-status")"
[[ "$child_status" =~ ^[0-9]+$ ]] || {
  echo "reference-broker: child exit status is invalid" >&2
  exit 4
}

python3 "$FINALIZER" context --store-root "$store_root" --permit-id "$permit_id" --child-exit-code "$child_status" --at "$at" >"$work_dir/receipt-input.json"
BUBBLES_USAGE_REFERENCE_TEST=enabled bash "$USAGE_ADAPTER" receipt "$work_dir/receipt-input.json" >"$work_dir/receipt.json"
python3 "$ENGINE" usage-record --store-root "$store_root" --input "$work_dir/receipt.json" >/dev/null
[[ -n "${BUBBLES_USAGE_REFERENCE_AUTHORITY:-}" ]] || { echo "reference-broker: receipt authority is not configured" >&2; exit 4; }
jq -cS --arg at "$at" '{receipt:.,verifiedAt:$at}' "$work_dir/receipt.json" >"$work_dir/verification-payload.json"
chmod 600 "$work_dir/verification-payload.json"
python3 "$SCRIPTS_DIR/security-authority.py" sign usage-receipt "$BUBBLES_USAGE_REFERENCE_AUTHORITY" "$work_dir/verification-payload.json" >"$work_dir/authenticator.json"
jq -cS --slurpfile auth "$work_dir/authenticator.json" '. + {authenticator:$auth[0].authenticator}' "$work_dir/verification-payload.json" >"$work_dir/verification-input.json"
chmod 600 "$work_dir/verification-input.json"
BUBBLES_USAGE_REFERENCE_TEST=enabled bash "$USAGE_ADAPTER" verify-receipt "$work_dir/verification-input.json" >"$work_dir/verification.json"
python3 "$ENGINE" usage-record --store-root "$store_root" --input "$work_dir/verification.json" >/dev/null
receipt_id="$(jq -r '.usageReceiptId' "$work_dir/receipt.json")"
verification_id="$(jq -r '.verificationId' "$work_dir/verification.json")"
settlement_json="$(python3 "$FINALIZER" settle --store-root "$store_root" --permit-id "$permit_id" --usage-receipt-id "$receipt_id" --receipt-verification-id "$verification_id" --at "$at")"
outcome="$(printf '%s' "$settlement_json" | jq -r '.terminalState')"
stdout_bytes="$(wc -c <"$snapshot_dir/stdout" | tr -d ' ')"
stderr_bytes="$(wc -c <"$snapshot_dir/stderr" | tr -d ' ')"

# IMP-056 SCOPE-5 (EV-17). This block is audit evidence ONLY: everything in it
# is read from durable records the ledger already accepted BEFORE this line
# runs, after the launch it describes already happened. Nothing here is
# consulted by any authorization decision -- the gateway (SCOPE-4) already
# made every gating decision before the broker was ever invoked, from the
# SAME durable records this just re-reads. A caller that fabricated this
# object could not use it to open the gateway, because nothing reads a result
# envelope as an input; it can only misrepresent history after the fact,
# which is a provenance problem for whoever consumes the report, not an
# authorization bypass.
#
# CONDITIONAL, per SCOPE-5's own wording: present only when this dispatch was
# reached through the SCOPE-4 gateway (--existing-snapshot-dir left an
# authorization.json in the shared snapshot directory). A direct broker call
# has no authorization to report and gets no audit block, exactly as before.
mutable_dispatch_audit="null"
if [[ -f "$snapshot_dir/authorization.json" ]]; then
  authorization_authenticator="$(jq -r '.authenticator // ""' "$snapshot_dir/authorization.json")"
  launch_state_id="$(jq -r '.launchStateId // ""' <<<"$launch_confirm_record")"
  launch_state="$(jq -r '.state // ""' <<<"$launch_confirm_record")"
  mutable_dispatch_audit="$(jq -n --arg pid "$permit_id" --arg aa "$authorization_authenticator" \
    --arg lsid "$launch_state_id" --arg ls "$launch_state" \
    '{permitId:$pid,authorizationAuthenticator:$aa,launchStateId:$lsid,launchState:$ls}')"
fi

jq -nc --argjson childExitCode "$child_status" --arg settlement "$outcome" \
  --argjson stderrBytes "$stderr_bytes" --argjson stdoutBytes "$stdout_bytes" \
  --argjson mutableDispatchAudit "$mutable_dispatch_audit" \
  '{childExitCode:$childExitCode,contractType:"reference-dispatch-result",
    mutableDispatchAudit:$mutableDispatchAudit,nativeHostInterception:"unsupported",
    schemaVersion:1,settlement:$settlement,stderrBytes:$stderrBytes,stdoutBytes:$stdoutBytes}'
exit "$child_status"