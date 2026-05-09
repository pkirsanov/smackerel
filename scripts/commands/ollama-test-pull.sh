#!/usr/bin/env bash
# Spec 043 Scope 2 — Ollama test model pull.
#
# Pulls the SST-pinned test model into the running Ollama container so
# the spec 043 e2e happy-path test (tests/e2e/agent/happy_path_test.go)
# can exercise the production NATS+sidecar+litellm+Ollama path against a
# real model without per-test cold-pull latency.
#
# This is FAIL-LOUD by design (FR-OLLAMA-002, FR-OLLAMA-005):
#
#   * Every required env var MUST be set; an empty value is fatal.
#     There are NO Go-style fallback defaults.
#   * Non-2xx responses from the Ollama HTTP API are fatal.
#   * The pull must complete within OLLAMA_TEST_PULL_TIMEOUT_SECONDS;
#     timeout is fatal (does NOT t.Skip equivalent).
#
# Required env vars (sourced from config/generated/test.env):
#
#   OLLAMA_URL                            base URL for the Ollama HTTP API
#   OLLAMA_TEST_MODEL                     model tag to pull (SST-pinned;
#                                         see infrastructure.ollama.test.model
#                                         in config/smackerel.yaml)
#   OLLAMA_TEST_PULL_TIMEOUT_SECONDS      ceiling on the pull wall-clock
#
# Exit codes:
#
#   0   pull completed and the model is present in the daemon's catalog
#   1   missing/empty required env var
#   2   HTTP error from /api/pull (non-2xx)
#   3   pull timed out before the daemon reported "success"
#   4   model still missing from /api/tags after the pull reported success

set -euo pipefail

require_env() {
  local name="$1"
  local val="${!name-}"
  if [[ -z "$val" ]]; then
    echo "ollama-test-pull: required env var $name is missing or empty (SST violation; check config/generated/test.env)" >&2
    exit 1
  fi
  printf '%s' "$val"
}

ollama_url="$(require_env OLLAMA_URL)"
test_model="$(require_env OLLAMA_TEST_MODEL)"
timeout_seconds="$(require_env OLLAMA_TEST_PULL_TIMEOUT_SECONDS)"

# strip trailing slashes so URL composition is unambiguous
ollama_url="${ollama_url%/}"

echo "ollama-test-pull: pulling $test_model from $ollama_url (timeout=${timeout_seconds}s)"

# /api/pull streams NDJSON status updates; we POST {"name":"<model>","stream":false}
# so the daemon blocks until the pull finishes (or the daemon errors).
# The host-side timeout(1) command provides the wall-clock ceiling.
pull_payload=$(printf '{"name":"%s","stream":false}' "$test_model")

http_status_file="$(mktemp)"
trap 'rm -f "$http_status_file"' EXIT

set +e
timeout "${timeout_seconds}s" curl \
  --silent \
  --show-error \
  --fail-with-body \
  --output /dev/null \
  --write-out '%{http_code}' \
  --max-time "${timeout_seconds}" \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "$pull_payload" \
  "${ollama_url}/api/pull" \
  > "$http_status_file" 2>&1
curl_rc=$?
set -e

http_status="$(cat "$http_status_file" 2>/dev/null || true)"

if [[ "$curl_rc" -eq 124 ]]; then
  echo "ollama-test-pull: timeout after ${timeout_seconds}s pulling $test_model from $ollama_url" >&2
  exit 3
fi

if [[ "$curl_rc" -ne 0 ]]; then
  echo "ollama-test-pull: curl failed (rc=$curl_rc) pulling $test_model from $ollama_url" >&2
  echo "ollama-test-pull: response: $http_status" >&2
  exit 2
fi

if [[ ! "$http_status" =~ ^2[0-9][0-9]$ ]]; then
  echo "ollama-test-pull: HTTP $http_status from ${ollama_url}/api/pull (expected 2xx)" >&2
  exit 2
fi

# Verify the model is actually present in the daemon's catalog. /api/pull
# may return 200 even if the underlying registry pull silently dropped a
# layer (unlikely, but the verification keeps the contract honest).
if ! curl --silent --show-error --fail --max-time 30 "${ollama_url}/api/tags" \
    | grep -q "\"name\":\"${test_model}\""; then
  echo "ollama-test-pull: model $test_model missing from ${ollama_url}/api/tags after successful pull" >&2
  exit 4
fi

echo "ollama-test-pull: $test_model present in ${ollama_url}/api/tags"
