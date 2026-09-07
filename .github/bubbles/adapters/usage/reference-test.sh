#!/usr/bin/env bash
# Hermetic MBE usage adapter for repository-reference lifecycle tests.
# It models exact usage from broker-observed test inputs. It makes no claim
# about a provider API or native host interception and is disabled by default.
set -euo pipefail
umask 077

OPERATION="${1:-}"
INPUT_FILE="${2:-}"
if [[ "$OPERATION" == "v2" ]]; then
  OPERATION="${2:-}"
  INPUT_FILE="${3:-}"
fi
MAPPING_DIGEST="sha256:2fe7fb1720b77f0cb15b77df8cd7dbc79aaee2c4682061767e7f09a0faaa2f6f"
AUTHORITY_TOOL="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../scripts" && pwd)/security-authority.py"
CAPABILITIES='[{"dimension":"modelRequestCount","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"inputTokens","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"outputTokens","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"cacheWriteTokens","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"cacheReadTokens","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"providerCredits","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"monetaryMinorUnits","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"subagentDispatches","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"webCalls","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"browserCalls","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"toolCalls","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"retainedResultBytes","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"wallTimeMs","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"retries","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true},{"dimension":"concurrency","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":true}]'

fail() { printf '{"code":"%s","message":"%s"}\n' "$1" "$2" >&2; exit "${3:-3}"; }
require_input() {
  [[ "${BUBBLES_USAGE_REFERENCE_TEST:-}" == "enabled" ]] || fail "MBE-USAGE-REFERENCE-DISABLED" "reference-test usage is available only to explicit hermetic tests"
  [[ -n "$INPUT_FILE" && -f "$INPUT_FILE" ]] || fail "MBE-USAGE-INPUT" "operation requires an input JSON file" 2
  command -v jq >/dev/null 2>&1 || fail "MBE-USAGE-DEPENDENCY" "jq is required" 2
}
digest_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    fail "MBE-USAGE-DEPENDENCY" "sha256 utility is required" 2
  fi
}
typed_digest() {
  local type="$1" payload="$2"
  printf '%s' "${type}:1:${payload}" | digest_text
}

case "$OPERATION" in
  describe)
    printf '{"adapterId":"reference-test","capabilities":%s,"contractType":"usage-adapter-description","hostSchemaIds":["repository-reference-test-v2"],"mappingDigest":"%s","schemaVersion":2,"sessionIdentity":"exact","supportedMajors":[2]}\n' "$CAPABILITIES" "$MAPPING_DIGEST"
    ;;
  identify-session)
    require_input
    jq -cS --arg adapter reference-test '{adapterId:$adapter,artifactSessionId:.artifactSessionId,contractType:"host-session-identity",hostInstanceId:.hostInstanceId,hostSchemaId:"repository-reference-test-v2",hostSessionId:.hostSessionId,proofDigest:.proofDigest,repositoryDecisionId:.repositoryDecisionId,schemaVersion:2,sessionIdentityId:.sessionIdentityId,startedAt:.startedAt,workspaceIdentity:.workspaceIdentity}' "$INPUT_FILE"
    ;;
  quote)
    require_input
    jq -cS --arg adapter reference-test --arg mapping "$MAPPING_DIGEST" '{actionDigest:.actionDigest,adapterId:$adapter,attemptId:.attemptId,budgetId:.budgetId,contractType:"usage-quote",dimensionSetDigest:.dimensionSetDigest,epochId:.epochId,expiresAt:.expiresAt,goalId:.goalId,intentId:.intentId,mappingDigest:$mapping,maximums:.maximums,negotiationId:.negotiationId,occurrenceId:.occurrenceId,quoteId:.quoteId,quotedAt:.quotedAt,ruleDigest:.ruleDigest,schemaVersion:2,sessionIdentityId:.sessionIdentityId}' "$INPUT_FILE"
    ;;
  receipt)
    require_input
    child_exit="$(jq -r '.childExitCode' "$INPUT_FILE")"
    result_payload="$(jq -cS '{childExitCode:.childExitCode}' "$INPUT_FILE")"
    result_digest="sha256:$(typed_digest reference-test-result "$result_payload")"
    proof_payload="$(jq -cS --arg providerReceiptDigest "$result_digest" '{consumptionId:.consumptionId,permitId:.permitId,providerReceiptDigest:$providerReceiptDigest}' "$INPUT_FILE")"
    proof_digest="sha256:$(typed_digest reference-test-proof "$proof_payload")"
    [[ "$child_exit" =~ ^[0-9]+$ ]] || fail "MBE-USAGE-RECEIPT-INVALID" "child exit code is invalid"
    status=measured
    receipt_payload="$(jq -cS --arg adapter reference-test --arg status "$status" --arg provider "$result_digest" --arg proof "$proof_digest" '{actionDigest:.actionDigest,adapterContractVersion:2,adapterId:$adapter,attemptId:.attemptId,budgetId:.budgetId,contractType:"usage-receipt",epochId:.epochId,finishedAt:.finishedAt,goalId:.goalId,hostSchemaId:"repository-reference-test-v2",intentId:.intentId,measurement:(if $status == "measured" then .measurement else [] end),measurementStatus:$status,monotonicFinishedNs:.monotonicFinishedNs,monotonicStartedNs:.monotonicStartedNs,occurrenceId:.occurrenceId,permitId:.permitId,providerReceiptDigest:$provider,retainedProjectionBytes:0,retainedProjectionDigest:null,revision:1,schemaVersion:2,sessionIdentityId:.sessionIdentityId,sourceProofDigest:$proof,startedAt:.startedAt,supersedesReceiptId:null}' "$INPUT_FILE")"
    receipt_digest="$(typed_digest usage-receipt "$receipt_payload")"
    printf '%s' "$receipt_payload" | jq -cS --arg id "urc:$receipt_digest" '. + {usageReceiptId:$id}'
    ;;
  verify-receipt)
    require_input
    jq -e '.receipt.adapterId == "reference-test" and .receipt.hostSchemaId == "repository-reference-test-v2" and .receipt.sourceProofDigest != null' "$INPUT_FILE" >/dev/null || fail "MBE-USAGE-RECEIPT-INVALID" "receipt is not reference-test evidence"
    [[ -n "${BUBBLES_USAGE_REFERENCE_AUTHORITY:-}" ]] || fail "MBE-USAGE-AUTHORITY-UNCONFIGURED" "receipt verification requires an independent configured authority" 4
    authority_result="$(python3 "$AUTHORITY_TOOL" verify usage-receipt "$BUBBLES_USAGE_REFERENCE_AUTHORITY" "$INPUT_FILE")" || fail "MBE-USAGE-RECEIPT-INVALID" "receipt authenticator is invalid" 4
    authority_id="$(printf '%s' "$authority_result" | jq -r '.authorityId')"
    trust_root_id="$(printf '%s' "$authority_result" | jq -r '.trustRootId')"
    proof_digest="sha256:$(typed_digest reference-test-authenticator "$(jq -r '.authenticator' "$INPUT_FILE")")"
    verification_payload="$(jq -cS --arg authority "$authority_id" --arg trustRoot "$trust_root_id" --arg proof "$proof_digest" '{authorityId:$authority,proofDigest:$proof,revision:1,supersedesVerificationId:null,trustRootId:$trustRoot,usageReceiptId:.receipt.usageReceiptId,verdict:"valid",verifiedAt:.verifiedAt}' "$INPUT_FILE")"
    verification_digest="$(typed_digest usage-receipt-verification "$verification_payload")"
    printf '%s' "$verification_payload" | jq -cS --arg id "urv:$verification_digest" '. + {contractType:"usage-receipt-verification",schemaVersion:2,verificationId:$id}'
    ;;
  snapshot) fail "MBE-USAGE-VERB" "reference-test snapshot is not part of the dispatch lifecycle" 2 ;;
  *) fail "MBE-USAGE-VERB" "unknown reference-test operation" 2 ;;
esac
