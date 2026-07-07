#!/usr/bin/env bash
# Spec 061 SCOPE-06a (BS-002-OPTION2-INCOMPLETE-MULTI-PATH-MODEL-LEAK) —
# post-`up` test-stack pre-warm hook. After `docker compose up --wait`
# returns green for `--env test`, this script asks Ollama to (1) pull
# the SST-resolved default chat model + embedding model, then (2) warm
# the chat model with a single num_predict=1 /api/generate call so the
# first BS-002 / BS-007 /ask invocation does not eat the cold-load and
# blow the 5s retrieval-qa-v1 timeout.
#
# Fail-loud contract (smackerel-no-defaults):
#   * AGENT_PROVIDER_DEFAULT_MODEL MUST be set and non-empty.
#   * OLLAMA_HOST_PORT MUST be set and non-empty (host-routable port
#     published by docker-compose for the test stack). The prewarm runs
#     on the host, so OLLAMA_URL's in-container hostname is not usable.
#   * EMBEDDING_MODEL, when present and non-empty, MUST also pull.
#   * Non-2xx response from /api/pull or /api/generate is a hard failure.
#   * Any /api/pull stream that does not terminate with status="success"
#     (or that contains an "error" object) is a hard failure.
#
# Skipped silently when SMACKEREL_PREWARM_ENABLED is "false". Used by
# the dev / home-lab smackerel_run_up path which does not pre-warm.

set -euo pipefail

if [[ "${SMACKEREL_PREWARM_ENABLED:-true}" == "false" ]]; then
  exit 0
fi

prewarm_require_env() {
  local name="$1"
  local val="${!name-}"
  if [[ -z "$val" ]]; then
    echo "stack.sh prewarm: required env var $name is missing or empty (spec 061 SCOPE-06a; check config/generated/test.env)" >&2
    exit 1
  fi
  printf '%s' "$val"
}

prewarm_optional_env() {
  local name="$1"
  printf '%s' "${!name-}"
}

chat_model="$(prewarm_require_env AGENT_PROVIDER_DEFAULT_MODEL)"
host_port="$(prewarm_require_env OLLAMA_HOST_PORT)"
embedding_model="$(prewarm_optional_env EMBEDDING_MODEL)"
# Spec 061 SCOPE-06a Round 65 (D4 hybrid fix). Both values originate from
# infrastructure.ollama.test.* in config/smackerel.yaml — no shell-side defaults.
warmup_num_predict="$(prewarm_require_env OLLAMA_TEST_PREWARM_WARMUP_NUM_PREDICT)"
warmup_second_call_max_ms="$(prewarm_require_env OLLAMA_TEST_PREWARM_SECOND_CALL_MAX_MS)"

url="http://127.0.0.1:${host_port}"

prewarm_pull_model() {
  local model="$1"
  local body_file status_file curl_rc http_status
  body_file="$(mktemp)"
  status_file="$(mktemp)"

  echo "stack.sh prewarm: pulling $model via $url/api/pull"

  set +e
  curl \
    --silent \
    --show-error \
    --output "$body_file" \
    --write-out '%{http_code}' \
    --max-time 900 \
    --request POST \
    --header 'Content-Type: application/json' \
    --data "$(printf '{"model":"%s","stream":true}' "$model")" \
    "${url}/api/pull" \
    > "$status_file" 2>&1
  curl_rc=$?
  set -e

  http_status="$(cat "$status_file" 2>/dev/null || true)"

  if [[ "$curl_rc" -ne 0 ]]; then
    echo "stack.sh prewarm: curl failed (rc=$curl_rc) pulling $model at $url/api/pull" >&2
    echo "stack.sh prewarm: status capture: $http_status" >&2
    sed -n '1,40p' "$body_file" >&2 || true
    rm -f "$body_file" "$status_file"
    exit 2
  fi

  if [[ ! "$http_status" =~ ^2[0-9][0-9]$ ]]; then
    echo "stack.sh prewarm: HTTP $http_status from ${url}/api/pull pulling $model (expected 2xx)" >&2
    sed -n '1,40p' "$body_file" >&2 || true
    rm -f "$body_file" "$status_file"
    exit 2
  fi

  if grep -q '"error"' "$body_file"; then
    echo "stack.sh prewarm: /api/pull stream reported error for $model:" >&2
    grep '"error"' "$body_file" | head -5 >&2
    rm -f "$body_file" "$status_file"
    exit 2
  fi

  if ! tail -n 5 "$body_file" | grep -q '"status":"success"'; then
    echo "stack.sh prewarm: /api/pull did not terminate with status=success for $model. Last lines:" >&2
    tail -n 5 "$body_file" >&2
    rm -f "$body_file" "$status_file"
    exit 2
  fi

  echo "stack.sh prewarm: $model pulled (HTTP $http_status, status=success)"
  rm -f "$body_file" "$status_file"
}

prewarm_warm_chat_model() {
  local model="$1"
  local num_predict="$2"
  local call_label="$3"
  local payload status_file curl_rc http_status start_ns end_ns elapsed_ms
  payload=$(printf '{"model":"%s","prompt":"warm","stream":false,"options":{"num_predict":%s}}' "$model" "$num_predict")
  status_file="$(mktemp)"

  echo "stack.sh prewarm: priming $model via $url/api/generate (call=$call_label num_predict=$num_predict)" >&2

  start_ns=$(date +%s%N); [[ "$start_ns" == *[!0-9]* ]] && start_ns=$(( ${start_ns%%[!0-9]*} * 1000000000 ))  # portable-ok: BSD/macOS date lacks %N (emits a literal N); fall back to whole seconds so the ms math stays numeric
  set +e
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --max-time 180 \
    --request POST \
    --header 'Content-Type: application/json' \
    --data "$payload" \
    "${url}/api/generate" \
    > "$status_file" 2>&1
  curl_rc=$?
  set -e
  end_ns=$(date +%s%N); [[ "$end_ns" == *[!0-9]* ]] && end_ns=$(( ${end_ns%%[!0-9]*} * 1000000000 ))  # portable-ok: BSD/macOS date lacks %N (emits a literal N); fall back to whole seconds so the ms math stays numeric
  elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))

  http_status="$(cat "$status_file" 2>/dev/null || true)"

  if [[ "$curl_rc" -ne 0 ]]; then
    echo "stack.sh prewarm: curl failed (rc=$curl_rc) warming $model at $url (call=$call_label)" >&2
    echo "stack.sh prewarm: response: $http_status" >&2
    rm -f "$status_file"
    exit 2
  fi

  if [[ ! "$http_status" =~ ^2[0-9][0-9]$ ]]; then
    echo "stack.sh prewarm: HTTP $http_status from ${url}/api/generate (expected 2xx, call=$call_label)" >&2
    rm -f "$status_file"
    exit 2
  fi

  echo "stack.sh prewarm: $model warmed (call=$call_label HTTP $http_status latency_ms=$elapsed_ms)" >&2
  rm -f "$status_file"
  printf '%s' "$elapsed_ms"
}

prewarm_pull_model "$chat_model"

if [[ -n "$embedding_model" ]]; then
  prewarm_pull_model "$embedding_model"
fi

# Spec 061 SCOPE-06a Round 65 (D4 hybrid fix): TWO consecutive warmup calls.
# Call 1 pays the cold-context load cost. Call 2 must execute on the warm
# resident model — its latency is the sustained-warmth proof gate. Fail loud
# if call 2 exceeds infrastructure.ollama.test.prewarm_warmup_second_call_max_ms.
# Spec 061 SCOPE-06b Round 68: budget raised to 55000 ms to match the Round 66
# retrieval-qa-v1 timeout_ms=60000 (safety margin 5000).
first_latency_ms="$(prewarm_warm_chat_model "$chat_model" "$warmup_num_predict" "first")"
second_latency_ms="$(prewarm_warm_chat_model "$chat_model" "$warmup_num_predict" "second")"

echo "stack.sh prewarm: warmup summary first_call_ms=$first_latency_ms second_call_ms=$second_latency_ms threshold_ms=$warmup_second_call_max_ms"

if (( second_latency_ms > warmup_second_call_max_ms )); then
  echo "stack.sh prewarm: SECOND warmup call latency_ms=$second_latency_ms exceeds threshold_ms=$warmup_second_call_max_ms — model is not sustainably warm; BS-002 against retrieval-qa-v1 timeout_ms=60000 will regress (spec 061 SCOPE-06b Round 68; threshold = budget − safety_margin)" >&2
  exit 3
fi
