#!/usr/bin/env bash
# Hermetic adversarial selftest for release-train metadata assignment.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
HELPER="$SCRIPT_DIR/release-train-metadata-assign.sh"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
BINDING="$SCRIPT_DIR/repository-binding.sh"
MODES_FILE="$REPO_ROOT/bubbles/workflows/modes.yaml"
CAPABILITIES_FILE="$REPO_ROOT/bubbles/agent-capabilities.yaml"

for dependency in jq yq; do
  if ! command -v "$dependency" >/dev/null 2>&1; then
    echo "release-train-metadata-assign-selftest: SKIP ($dependency not installed)"
    exit 0
  fi
done

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
WORK_DIR="$(mktemp -d "$selftest_tmp_base/bubbles-train-metadata.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

file_digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

run_expect_failure() {
  local label="$1"
  local marker="$2"
  shift 2
  local output_file="$WORK_DIR/failure-output"
  local exit_code=0

  set +e
  "$@" >"$output_file" 2>&1
  exit_code=$?
  set -e

  if [[ "$exit_code" -ne 0 ]]; then
    pass "$label refuses"
  else
    fail "$label unexpectedly succeeded"
  fi
  if grep -Fq -- "$marker" "$output_file"; then
    pass "$label names '$marker'"
  else
    fail "$label did not name '$marker'"
  fi
}

resolved="$(bash "$RESOLVER" --resolve-v6 ship action:assign target:train-metadata 2>/dev/null || true)"
if [[ "$resolved" == "release-train-assign-metadata" ]]; then
  pass "exact v7 tuple resolves to release-train-assign-metadata"
else
  fail "exact v7 tuple did not resolve (got '$resolved')"
fi

if [[ "$(yq -r '.modes."release-train-assign-metadata".statusCeiling // ""' "$MODES_FILE")" == "train_metadata_assigned" ]] &&
   [[ "$(yq -r '.modes."release-train-assign-metadata".terminalAliases[0] // ""' "$MODES_FILE")" == "train_metadata_assigned" ]]; then
  pass "mode has the bounded train_metadata_assigned terminal token"
else
  fail "mode terminal contract is absent or incorrect"
fi

mode_phases="$(yq -r '.modes."release-train-assign-metadata".phaseOrder[]? // ""' "$MODES_FILE")"
if ! grep -Eq '^(devops|deploy|build|cut|promote|rollback|retire)$' <<<"$mode_phases"; then
  pass "mode has no lifecycle, build, or deployment phase"
else
  fail "mode contains a forbidden lifecycle, build, or deployment phase"
fi

owners="$(yq -r '.workflowModeGrants.agents | to_entries[] | select(.value.modes[]? == "release-train-assign-metadata") | .key' "$CAPABILITIES_FILE")"
if [[ "$owners" == "bubbles.train" ]]; then
  pass "bubbles.train is the sole direct runner grant"
else
  fail "assignment grants are not owner-exact (got '$owners')"
fi

if [[ ! -x "$HELPER" ]]; then
  fail "production helper is absent or not executable: $HELPER"
  echo "release-train-metadata-assign-selftest: FAIL ($failures assertion(s))" >&2
  exit 1
fi

FIXTURE="$WORK_DIR/repo"
STATE_DIR="$FIXTURE/specs/001-fixture"
CONTROL_DIR="$WORK_DIR/external-control"
CONTROL_FILE="$CONTROL_DIR/repository-binding.json"
PACKET_FILE="$WORK_DIR/binding-packet.json"
SESSION_ID="bug-038-security-selftest"
mkdir -p "$STATE_DIR" "$FIXTURE/config" "$FIXTURE/generated" "$FIXTURE/deploy/demo" "$FIXTURE/bin" "$FIXTURE/agents" "$FIXTURE/bubbles/scripts" "$FIXTURE/bubbles/workflows" "$CONTROL_DIR"
chmod 700 "$CONTROL_DIR"
touch "$FIXTURE/VERSION" "$FIXTURE/install.sh" "$FIXTURE/bubbles/scripts/cli.sh"
git init -q "$FIXTURE"
cat >"$FIXTURE/config/release-trains.yaml" <<'EOF'
version: 1
trains:
  - id: alpha
    phase: active
    target_slot: staging
    flags_bundle: config/feature-flags.alpha.yaml
  - id: beta
    phase: maintained
    target_slot: none
    flags_bundle: config/feature-flags.beta.yaml
EOF
cat >"$FIXTURE/config/feature-flags.alpha.yaml" <<'EOF'
flags:
  existingFlag: false
EOF
cat >"$FIXTURE/config/feature-flags.beta.yaml" <<'EOF'
flags:
  existingFlag: false
EOF
cat >"$FIXTURE/generated/config-bundle.sentinel" <<'EOF'
no generated config bundle mutation
EOF
cat >"$FIXTURE/deploy/demo/manifest.yaml" <<'EOF'
current: sha256:fixture
previousManifest: sha256:previous
EOF
cat >"$STATE_DIR/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "full-delivery",
  "releaseTrain": "alpha",
  "flagsIntroduced": ["existingFlag"],
  "execution": {"currentPhase": "implement", "unknownFuture": [1, 2, 3]},
  "certification": {"status": "in_progress", "completedScopes": []},
  "policySnapshot": {"tdd": {"mode": "scenario-first"}},
  "customFutureField": {"preserve": true}
}
EOF
chmod 640 "$STATE_DIR/state.json"

cat >"$FIXTURE/bubbles/agent-capabilities.yaml" <<'EOF'
workflowModeGrants:
  defaultAllowed: false
  agents:
    bubbles.train:
      modes: [ release-train-assign-metadata ]
    bubbles.workflow:
      modes: [ "*" ]
    bubbles.goal:
      modes: [ "*" ]
      excludedModes: [ release-train-assign-metadata ]
EOF
cat >"$FIXTURE/bubbles/workflows/modes.yaml" <<'EOF'
modes:
  release-train-assign-metadata:
    statusCeiling: train_metadata_assigned
    constraints:
      ownedStateFields: [ releaseTrain, flagsIntroduced ]
EOF
cat >"$FIXTURE/bubbles/agent-ownership.yaml" <<'EOF'
artifacts:
  release-train-state:
    owner:
      - bubbles.train
EOF

capture_packet() {
  local expected_revision="$1"
  local output
  output="$(bash "$BINDING" preflight \
    --session-id "$SESSION_ID" \
    --session-control-file "$CONTROL_FILE" \
    --request-class STRUCTURED \
    --workspace-root "$FIXTURE" \
    --repository-root "$FIXTURE" \
    --expected-control-revision "$expected_revision")" || return 1
  printf '%s\n' "$output" | awk 'found || /^\{/ { found=1; print }' >"$PACKET_FILE"
  jq -e '.repositoryResolution.actionable == true' "$PACKET_FILE" >/dev/null 2>&1
}

capture_packet 0 || {
  fail "fixture repository binding packet could not be established"
  echo "release-train-metadata-assign-selftest: FAIL ($failures assertion(s))" >&2
  exit 1
}

AUTH_ARGS=(
  --session-id "$SESSION_ID"
  --session-control-file "$CONTROL_FILE"
  --binding-packet-file "$PACKET_FILE"
  --workflow-mode release-train-assign-metadata
  --runner bubbles.train
)

STATE_FILE="$STATE_DIR/state.json"
NON_OWNED_BEFORE="$(jq -S 'del(.releaseTrain, .flagsIntroduced)' "$STATE_FILE")"
STATE_BEFORE_DRY="$(file_digest "$STATE_FILE")"
dry_output="$(bash "$HELPER" "$STATE_DIR" --train beta "${AUTH_ARGS[@]}")"
if [[ "$(jq -r '.releaseTrain' <<<"$dry_output")" == "beta" ]] &&
   [[ "$(jq -c '.flagsIntroduced' <<<"$dry_output")" == '["existingFlag"]' ]]; then
  pass "default dry-run changes train candidate and preserves omitted flags"
else
  fail "default dry-run candidate is incorrect"
fi
if [[ "$(file_digest "$STATE_FILE")" == "$STATE_BEFORE_DRY" ]]; then
  pass "default dry-run preserves destination bytes"
else
  fail "default dry-run changed destination bytes"
fi
explicit_dry_before="$(file_digest "$STATE_FILE")"
bash "$HELPER" "$STATE_DIR" --train beta --dry-run "${AUTH_ARGS[@]}" >/dev/null
if [[ "$(file_digest "$STATE_FILE")" == "$explicit_dry_before" ]] &&
   ! find "$STATE_DIR" -maxdepth 1 -name '.release-train-metadata.*' -print | grep -q .; then
  pass "explicit dry-run preserves destination bytes and creates no candidate"
else
  fail "explicit dry-run changed destination bytes or created a candidate"
fi

run_expect_failure "environment identity declaration alone" "runner is required" /usr/bin/env BUBBLES_AGENT_NAME=bubbles.train bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-assign-metadata

missing_runner_before="$(file_digest "$STATE_FILE")"
run_expect_failure "missing authenticated runner" "runner is required" bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-assign-metadata
[[ "$(file_digest "$STATE_FILE")" == "$missing_runner_before" ]] && pass "missing authenticated runner preserves destination bytes" || fail "missing authenticated runner changed destination bytes"

wildcard_before="$(file_digest "$STATE_FILE")"
run_expect_failure "wildcard-admitted non-train runner" "admitted by wildcard" /usr/bin/env BUBBLES_AGENT_NAME=bubbles.train bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-assign-metadata --runner bubbles.workflow
[[ "$(file_digest "$STATE_FILE")" == "$wildcard_before" ]] && pass "wildcard-admitted refusal preserves destination bytes" || fail "wildcard-admitted refusal changed destination bytes"

excluded_before="$(file_digest "$STATE_FILE")"
run_expect_failure "explicit exclusion over wildcard" "explicitly excluded" bash "$HELPER" "$STATE_DIR" --train beta --dry-run \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-assign-metadata --runner bubbles.goal
[[ "$(file_digest "$STATE_FILE")" == "$excluded_before" ]] && pass "explicit exclusion preserves destination bytes" || fail "explicit exclusion changed destination bytes"

default_deny_before="$(file_digest "$STATE_FILE")"
run_expect_failure "unregistered runner default deny" "denied by default" bash "$HELPER" "$STATE_DIR" --train beta --dry-run \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-assign-metadata --runner bubbles.unknown
[[ "$(file_digest "$STATE_FILE")" == "$default_deny_before" ]] && pass "default-deny refusal preserves destination bytes" || fail "default-deny refusal changed destination bytes"

wrong_runner_before="$(file_digest "$STATE_FILE")"
run_expect_failure "wrong runner despite environment impersonation" "direct authenticated runner bubbles.train" /usr/bin/env BUBBLES_AGENT_NAME=bubbles.train bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-assign-metadata --runner bubbles.workflow
[[ "$(file_digest "$STATE_FILE")" == "$wrong_runner_before" ]] && pass "environment impersonation refusal preserves destination bytes" || fail "environment impersonation refusal changed destination bytes"

cp "$FIXTURE/bubbles/agent-ownership.yaml" "$WORK_DIR/ownership.valid.yaml"
cat >"$FIXTURE/bubbles/agent-ownership.yaml" <<'EOF'
artifacts: {}
EOF
owner_missing_before="$(file_digest "$STATE_FILE")"
run_expect_failure "missing release-train-state owner" "sole release-train-state owner" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
[[ "$(file_digest "$STATE_FILE")" == "$owner_missing_before" ]] && pass "missing ownership preserves destination bytes" || fail "missing ownership changed destination bytes"
cat >"$FIXTURE/bubbles/agent-ownership.yaml" <<'EOF'
artifacts:
  release-train-state:
    owner:
      - bubbles.train
      - bubbles.train
EOF
owner_duplicate_before="$(file_digest "$STATE_FILE")"
run_expect_failure "duplicate release-train-state owner" "sole release-train-state owner" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
[[ "$(file_digest "$STATE_FILE")" == "$owner_duplicate_before" ]] && pass "duplicate ownership preserves destination bytes" || fail "duplicate ownership changed destination bytes"
cat >"$FIXTURE/bubbles/agent-ownership.yaml" <<'EOF'
artifacts:
  release-train-state:
    owner:
      - bubbles.workflow
EOF
owner_before="$(file_digest "$STATE_FILE")"
run_expect_failure "wrong release-train-state owner" "sole release-train-state owner" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
[[ "$(file_digest "$STATE_FILE")" == "$owner_before" ]] && pass "wrong ownership preserves destination bytes" || fail "wrong ownership changed destination bytes"
cp "$WORK_DIR/ownership.valid.yaml" "$FIXTURE/bubbles/agent-ownership.yaml"

cp "$FIXTURE/bubbles/workflows/modes.yaml" "$WORK_DIR/modes.valid.yaml"
cat >"$FIXTURE/bubbles/workflows/modes.yaml" <<'EOF'
modes:
  release-train-assign-metadata:
    statusCeiling: train_metadata_assigned
    constraints:
      ownedStateFields: [ releaseTrain ]
EOF
fields_before="$(file_digest "$STATE_FILE")"
run_expect_failure "missing flagsIntroduced ownership" "ownedStateFields" bash "$HELPER" "$STATE_DIR" --train beta --flags-json '[]' --apply "${AUTH_ARGS[@]}"
[[ "$(file_digest "$STATE_FILE")" == "$fields_before" ]] && pass "owned-field mismatch preserves destination bytes" || fail "owned-field mismatch changed destination bytes"
cp "$WORK_DIR/modes.valid.yaml" "$FIXTURE/bubbles/workflows/modes.yaml"

cp "$FIXTURE/bubbles/agent-capabilities.yaml" "$WORK_DIR/capabilities.valid.yaml"
cat >"$FIXTURE/bubbles/agent-capabilities.yaml" <<'EOF'
workflowModeGrants:
  defaultAllowed: false
  agents:
    bubbles.train:
      modes: release-train-assign-metadata
EOF
run_expect_failure "malformed runner grant" "workflow mode grant authority is malformed" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
cat >"$FIXTURE/bubbles/agent-capabilities.yaml" <<'EOF'
workflowModeGrants:
  defaultAllowed: false
  agents:
    bubbles.train:
      modes: [ release-train-assign-metadata ]
      unsupportedAuthority: true
EOF
run_expect_failure "unsupported runner grant field" "unsupported field" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
cp "$WORK_DIR/capabilities.valid.yaml" "$FIXTURE/bubbles/agent-capabilities.yaml"

cat >"$FIXTURE/bubbles/workflows/modes.yaml" <<'EOF'
modes:
  release-train-assign-metadata:
    constraints:
      ownedStateFields: releaseTrain
EOF
run_expect_failure "malformed assignment mode" "assignment mode authority is malformed" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
cp "$WORK_DIR/modes.valid.yaml" "$FIXTURE/bubbles/workflows/modes.yaml"

MISMATCH_PACKET="$WORK_DIR/binding-mismatch.json"
jq '.repositoryRoot = "/outside-the-authorized-repository"' "$PACKET_FILE" >"$MISMATCH_PACKET"
run_expect_failure "repository binding mismatch" "PACKET" bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$MISMATCH_PACKET" --workflow-mode release-train-assign-metadata --runner bubbles.train

NONACTIONABLE_PACKET="$WORK_DIR/binding-nonactionable.json"
jq '.repositoryRoot = "<redacted-local-root>" | .repositoryResolution.pathVisibility = "redacted" | .repositoryResolution.actionable = false' "$PACKET_FILE" >"$NONACTIONABLE_PACKET"
run_expect_failure "nonactionable repository binding" "NONACTIONABLE" bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$NONACTIONABLE_PACKET" --workflow-mode release-train-assign-metadata --runner bubbles.train

STALE_PACKET="$WORK_DIR/binding-stale.json"
cp "$PACKET_FILE" "$STALE_PACKET"
capture_packet 1 || fail "fixture repository binding packet could not advance"
run_expect_failure "stale repository binding" "BOUNDARY_CONFLICT" bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$STALE_PACKET" --workflow-mode release-train-assign-metadata --runner bubbles.train

run_expect_failure "wrong workflow mode" "workflow mode" bash "$HELPER" "$STATE_DIR" --train beta --apply \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --binding-packet-file "$PACKET_FILE" --workflow-mode release-train-cut

OUTSIDE_DIR="$WORK_DIR/outside/specs/001-external"
mkdir -p "$OUTSIDE_DIR"
cp "$STATE_FILE" "$OUTSIDE_DIR/state.json"
run_expect_failure "external state target" "authorized repository" bash "$HELPER" "$OUTSIDE_DIR" --train beta --apply "${AUTH_ARGS[@]}"

ln -s "$STATE_DIR" "$FIXTURE/specs/002-symlinked"
run_expect_failure "symlinked state path" "symlink" bash "$HELPER" "$FIXTURE/specs/002-symlinked" --train beta --apply "${AUTH_ARGS[@]}"

chmod 666 "$STATE_FILE"
run_expect_failure "writable-by-others state mode" "mode" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
chmod 640 "$STATE_FILE"

mkdir "$STATE_DIR/.release-train-metadata.lock"
run_expect_failure "concurrent helper writer" "concurrent metadata writer lock" bash "$HELPER" "$STATE_DIR" --train beta --apply "${AUTH_ARGS[@]}"
rmdir "$STATE_DIR/.release-train-metadata.lock"

for command_name in docker cosign oras; do
  cat >"$FIXTURE/bin/$command_name" <<EOF
#!/usr/bin/env bash
printf '%s\n' '$command_name' >>'$WORK_DIR/forbidden-invocations'
exit 97
EOF
  chmod +x "$FIXTURE/bin/$command_name"
done

sentinel_paths="$FIXTURE/config/release-trains.yaml $FIXTURE/config/feature-flags.alpha.yaml $FIXTURE/config/feature-flags.beta.yaml $FIXTURE/generated/config-bundle.sentinel $FIXTURE/deploy/demo/manifest.yaml"
sentinel_before=""
for sentinel_path in $sentinel_paths; do
  sentinel_before="${sentinel_before}$(file_digest "$sentinel_path")  $sentinel_path
"
done

PATH="$FIXTURE/bin:$PATH" bash "$HELPER" "$STATE_DIR" --train beta --flags-json '["flagOne","flagTwo"]' --apply "${AUTH_ARGS[@]}"
if [[ "$(jq -r '.releaseTrain' "$STATE_FILE")" == "beta" ]] &&
   [[ "$(jq -c '.flagsIntroduced' "$STATE_FILE")" == '["flagOne","flagTwo"]' ]]; then
  pass "exact owner applies existing train and explicit flags"
else
  fail "exact owner assignment did not persist owned values"
fi
if [[ "$(jq -S 'del(.releaseTrain, .flagsIntroduced)' "$STATE_FILE")" == "$NON_OWNED_BEFORE" ]]; then
  pass "apply preserves every non-owned state value semantically"
else
  fail "apply changed a non-owned state value"
fi
if [[ "$(stat -c '%a' "$STATE_FILE" 2>/dev/null || stat -f '%Lp' "$STATE_FILE" 2>/dev/null)" == "640" ]]; then
  pass "atomic replacement preserves the destination file mode"
else
  fail "atomic replacement changed destination file mode"
fi

REAL_SHA256SUM="$(command -v sha256sum || true)"
if [[ -n "$REAL_SHA256SUM" ]]; then
  SHA_COUNTER="$WORK_DIR/sha-counter"
  cat >"$FIXTURE/bin/sha256sum" <<EOF
#!/usr/bin/env bash
count=0
if [[ -f "$SHA_COUNTER" ]]; then
  count="\$(cat "$SHA_COUNTER")"
fi
count=\$((count + 1))
printf '%s\n' "\$count" >"$SHA_COUNTER"
if [[ "\$count" -eq 3 ]]; then
  printf '%s\n' '{"replacementDrift":true}' >"$STATE_FILE"
  chmod 640 "$STATE_FILE"
fi
exec "$REAL_SHA256SUM" "\$@"
EOF
  chmod +x "$FIXTURE/bin/sha256sum"
  run_expect_failure "replacement-time state drift" "changed concurrently" /usr/bin/env PATH="$FIXTURE/bin:$PATH" bash "$HELPER" "$STATE_DIR" --train alpha --apply "${AUTH_ARGS[@]}"
  if [[ "$(jq -r '.replacementDrift // false' "$STATE_FILE")" == "true" ]]; then
    pass "replacement-time drift is preserved instead of overwritten"
  else
    fail "replacement-time drift was overwritten by the stale candidate"
  fi
  rm -f "$FIXTURE/bin/sha256sum" "$SHA_COUNTER"
  cat >"$STATE_FILE" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "full-delivery",
  "releaseTrain": "beta",
  "flagsIntroduced": ["flagOne", "flagTwo"],
  "execution": {"currentPhase": "implement", "unknownFuture": [1, 2, 3]},
  "certification": {"status": "in_progress", "completedScopes": []},
  "policySnapshot": {"tdd": {"mode": "scenario-first"}},
  "customFutureField": {"preserve": true}
}
EOF
  chmod 640 "$STATE_FILE"
else
  pass "replacement-time drift adversary skipped because sha256sum is unavailable"
fi

sentinel_after=""
for sentinel_path in $sentinel_paths; do
  sentinel_after="${sentinel_after}$(file_digest "$sentinel_path")  $sentinel_path
"
done
if [[ "$sentinel_after" == "$sentinel_before" ]] && [[ ! -f "$WORK_DIR/forbidden-invocations" ]]; then
  pass "assignment preserves config, bundles, generated output, manifest, and invokes no lifecycle tool"
else
  fail "assignment crossed the closed side-effect boundary"
fi

idempotent_digest="$(file_digest "$STATE_FILE")"
if source "$SCRIPT_DIR/guard-lib.sh" && idempotent_mtime="$(bubbles_file_mtime_epoch "$STATE_FILE")"; then
  bash "$HELPER" "$STATE_FILE" --train beta --flags-json '["flagOne","flagTwo"]' --apply "${AUTH_ARGS[@]}"
  if [[ "$(file_digest "$STATE_FILE")" == "$idempotent_digest" ]] &&
     [[ "$(bubbles_file_mtime_epoch "$STATE_FILE")" == "$idempotent_mtime" ]]; then
    pass "identical assignment is a byte-and-mtime preserving no-op"
  else
    fail "identical assignment replaced or changed state.json"
  fi
else
  fail "portable file mtime helper was unavailable"
fi

bash "$HELPER" "$STATE_DIR" --train alpha --apply "${AUTH_ARGS[@]}"
if [[ "$(jq -c '.flagsIntroduced' "$STATE_FILE")" == '["flagOne","flagTwo"]' ]]; then
  pass "omitted flags preserve flagsIntroduced during apply"
else
  fail "omitted flags changed flagsIntroduced"
fi
bash "$HELPER" "$STATE_DIR" --train alpha --flags-json '[]' --apply "${AUTH_ARGS[@]}"
if [[ "$(jq -c '.flagsIntroduced' "$STATE_FILE")" == '[]' ]]; then
  pass "explicit empty flags array clears flagsIntroduced"
else
  fail "explicit empty flags array did not clear flagsIntroduced"
fi

for invalid_flags in '{"flag":true}' '["duplicate","duplicate"]' '[""]' '[1]'; do
  before_invalid="$(file_digest "$STATE_FILE")"
  run_expect_failure "invalid flags $invalid_flags" "flags-json" bash "$HELPER" "$STATE_DIR" --train alpha --flags-json "$invalid_flags" --apply "${AUTH_ARGS[@]}"
  if [[ "$(file_digest "$STATE_FILE")" == "$before_invalid" ]]; then
    pass "invalid flags $invalid_flags preserve destination bytes"
  else
    fail "invalid flags $invalid_flags changed destination bytes"
  fi
done

unknown_before="$(file_digest "$STATE_FILE")"
run_expect_failure "unknown train" "missing-train" bash "$HELPER" "$STATE_DIR" --train missing-train --apply "${AUTH_ARGS[@]}"
if [[ "$(file_digest "$STATE_FILE")" == "$unknown_before" ]] &&
   ! find "$STATE_DIR" -maxdepth 1 -name '.release-train-metadata.*' -print | grep -q .; then
  pass "unknown train preserves bytes and leaves no candidate residue"
else
  fail "unknown train changed bytes or left candidate residue"
fi

run_expect_failure "duplicate train option" "exactly once" bash "$HELPER" "$STATE_DIR" --train alpha --train beta "${AUTH_ARGS[@]}"
run_expect_failure "duplicate flags option" "at most once" bash "$HELPER" "$STATE_DIR" --train alpha --flags-json '[]' --flags-json '[]' "${AUTH_ARGS[@]}"
run_expect_failure "duplicate runner option" "only once" bash "$HELPER" "$STATE_DIR" --train alpha --runner bubbles.train "${AUTH_ARGS[@]}"
run_expect_failure "conflicting mode options" "mutually exclusive" bash "$HELPER" "$STATE_DIR" --train alpha --dry-run --apply "${AUTH_ARGS[@]}"
run_expect_failure "caller-controlled agent option" "unknown option" bash "$HELPER" "$STATE_DIR" --train alpha --agent bubbles.train "${AUTH_ARGS[@]}"
run_expect_failure "extra positional target" "one target" bash "$HELPER" "$STATE_DIR" "$STATE_DIR" --train alpha "${AUTH_ARGS[@]}"

if [[ "$failures" -gt 0 ]]; then
  echo "release-train-metadata-assign-selftest: FAIL ($failures assertion(s))" >&2
  exit 1
fi

echo "release-train-metadata-assign-selftest: PASS"
