#!/usr/bin/env bash
# Hermetic acceptance tests for the experience-recall CLI and bash twin.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TWIN="$SCRIPT_DIR/experience-recall.sh"
WORK="$(mktemp -d)"
PASS=0
FAIL=0
LAST_OUT="$WORK/stdout"
LAST_ERR="$WORK/stderr"
LAST_RC=0

cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

pass() {
  PASS=$((PASS + 1))
  echo "PASS: $1"
}

fail() {
  FAIL=$((FAIL + 1))
  echo "FAIL: $1"
}

run_case() {
  LAST_RC=0
  set +e
  "$@" >"$LAST_OUT" 2>"$LAST_ERR"
  LAST_RC=$?
  set -e
}

assert_rc() {
  local label="$1"
  local expected="$2"
  if [[ "$LAST_RC" -eq "$expected" ]]; then
    pass "$label (exit $LAST_RC)"
  else
    fail "$label (expected exit $expected, got $LAST_RC)"
    echo "  stdout: $(cat "$LAST_OUT")"
    echo "  stderr: $(cat "$LAST_ERR")"
  fi
}

assert_stdout_contains() {
  local label="$1"
  local expected="$2"
  if grep -qF "$expected" "$LAST_OUT"; then
    pass "$label"
  else
    fail "$label"
    echo "  stdout: $(cat "$LAST_OUT")"
  fi
}

assert_stderr_contains() {
  local label="$1"
  local expected="$2"
  if grep -qF "$expected" "$LAST_ERR"; then
    pass "$label"
  else
    fail "$label"
    echo "  stderr: $(cat "$LAST_ERR")"
  fi
}

assert_stdout_not_contains() {
  local label="$1"
  local unexpected="$2"
  if grep -qF "$unexpected" "$LAST_OUT"; then
    fail "$label"
    echo "  stdout: $(cat "$LAST_OUT")"
  else
    pass "$label"
  fi
}

assert_stderr_not_contains() {
  local label="$1"
  local unexpected="$2"
  if grep -qF "$unexpected" "$LAST_ERR"; then
    fail "$label"
    echo "  stderr: $(cat "$LAST_ERR")"
  else
    pass "$label"
  fi
}

assert_terminal_safe() {
  local label="$1"
  if python3 - "$LAST_OUT" "$LAST_ERR" 2>/dev/null <<'PY'
import pathlib
import sys

for name in sys.argv[1:]:
    payload = pathlib.Path(name).read_bytes()
    assert all(byte in (9, 10, 13) or 32 <= byte < 127 for byte in payload)
PY
  then
    pass "$label"
  else
    fail "$label"
  fi
}

assert_clean_provider_failure() {
  local label="$1"
  if [[ "$LAST_RC" -eq 1 ]] && ! grep -qF 'Traceback' "$LAST_ERR"; then
    pass "$label"
  else
    fail "$label (expected clean exit 1, got $LAST_RC)"
    echo "  stdout: $(cat "$LAST_OUT")"
    echo "  stderr: $(cat "$LAST_ERR")"
  fi
}

assert_safe_text_or_refusal() {
  local label="$1"
  local safe=0
  python3 - "$LAST_OUT" "$LAST_ERR" 2>/dev/null <<'PY' || safe=$?
import pathlib
import sys

for name in sys.argv[1:]:
    payload = pathlib.Path(name).read_bytes()
    assert all(byte in (9, 10, 13) or 32 <= byte < 127 for byte in payload)
PY
  if [[ "$safe" -eq 0 && ( "$LAST_RC" -eq 0 || "$LAST_RC" -eq 1 ) ]] \
    && ! grep -qF 'Traceback' "$LAST_ERR"; then
    pass "$label"
  else
    fail "$label (exit $LAST_RC, safe=$safe)"
    echo "  stdout: $(cat "$LAST_OUT")"
    echo "  stderr: $(cat "$LAST_ERR")"
  fi
}

assert_stdout_bounded() {
  local label="$1"
  local max_lines="$2"
  local max_bytes="$3"
  if python3 - "$LAST_OUT" "$max_lines" "$max_bytes" <<'PY'
import pathlib
import sys

payload = pathlib.Path(sys.argv[1]).read_bytes()
assert len(payload) <= int(sys.argv[3])
assert len(payload.splitlines()) <= int(sys.argv[2])
PY
  then
    pass "$label"
  else
    fail "$label"
    echo "  stdout bytes/lines exceed $max_bytes/$max_lines"
  fi
}

assert_json_file() {
  local label="$1"
  local expression="$2"
    if python3 -c 'import json,sys; data=json.load(open(sys.argv[1], encoding="utf-8")); assert eval(sys.argv[2], {"data": data})' \
      "$LAST_OUT" "$expression" 2>/dev/null; then
    pass "$label"
  else
    fail "$label"
    echo "  payload: $(cat "$LAST_OUT")"
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

file_fingerprint() {
  python3 - "$1" <<'PY'
import hashlib
import os
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
print(hashlib.sha256(path.read_bytes()).hexdigest() + ":" + str(os.stat(path).st_mtime_ns))
PY
}

stage_framework() {
  local root="$1"
  local layout="${2:-source}"
  local framework="$root/bubbles"
  if [[ "$layout" == "downstream" ]]; then
    framework="$root/.github/bubbles"
  fi
  mkdir -p "$framework/scripts" "$framework/adapters/experience-recall"
  cp "$REPO_ROOT/bubbles/scripts/cli.sh" "$framework/scripts/cli.sh"
  cp "$REPO_ROOT/bubbles/scripts/fun-mode.sh" "$framework/scripts/fun-mode.sh"
  cp "$REPO_ROOT/bubbles/scripts/aliases.sh" "$framework/scripts/aliases.sh"
  cp "$REPO_ROOT/bubbles/scripts/trust-metadata.sh" "$framework/scripts/trust-metadata.sh"
  cp "$REPO_ROOT/bubbles/scripts/experience-recall.sh" "$framework/scripts/experience-recall.sh"
  cp "$REPO_ROOT/bubbles/scripts/experience-recall-resolve.sh" "$framework/scripts/experience-recall-resolve.sh"
  cp "$REPO_ROOT/bubbles/scripts/experience-recall-index.py" "$framework/scripts/experience-recall-index.py"
  cp "$REPO_ROOT/bubbles/scripts/repo-slug.sh" "$framework/scripts/repo-slug.sh"
  cp "$REPO_ROOT/bubbles/adapters/experience-recall/none.sh" "$framework/adapters/experience-recall/none.sh"
  cp "$REPO_ROOT/bubbles/adapters/experience-recall/local-lexical.sh" "$framework/adapters/experience-recall/local-lexical.sh"
  cp "$REPO_ROOT/bubbles/action-risk-registry.yaml" "$framework/action-risk-registry.yaml"
  cp "$REPO_ROOT/bubbles/workflows.yaml" "$framework/workflows.yaml"
  chmod +x "$framework/scripts/experience-recall.sh" \
    "$framework/scripts/experience-recall-resolve.sh" \
    "$framework/scripts/experience-recall-index.py" \
    "$framework/adapters/experience-recall/none.sh" \
    "$framework/adapters/experience-recall/local-lexical.sh"
}

write_local_fixture() {
  local root="$1"
  local repository_alias="${2:-my-repo-name}"
  mkdir -p "$root/.github" "$root/.specify/memory" "$root/evidence" "$root/improvements"
  printf 'experienceRecall:\n  adapter: local-lexical\n' >"$root/.github/bubbles-project.yaml"
  python3 - "$root" "$repository_alias" <<'PY'
import hashlib
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
repository_alias = sys.argv[2]
source = root / "evidence/source.md"
source.write_text("SOURCE_BODY_SENTINEL bounded database retry evidence\n", encoding="utf-8")
digest = "sha256:" + hashlib.sha256(source.read_bytes()).hexdigest()
stamp = "2026-08-06T12:00:00Z"
lines = ["# Lessons", "", "- problem: legacy entry; root cause: no anchor; fix: preserve legacy; applies when: clustering"]
for index in range(3):
    lesson_id = "lesson-" + str(index + 1) * 64
    metadata = {
        "capturedAt": stamp,
        "lessonId": lesson_id,
        "repositoryAlias": repository_alias,
        "reviewState": "reviewed",
        "schemaVersion": 1,
        "sourceAnchor": {
            "contentDigest": digest,
            "observedAt": stamp,
            "relativePath": "evidence/source.md",
            "selector": f"#lesson-{index + 1}",
        },
    }
    visible = (
        f"- problem: bounded database retry {index + 1}; root cause: unbounded attempt; "
        f"fix: cap retry {index + 1}; applies when: database retry"
    )
    lines.append(
        visible
        + " <!-- bubbles-lesson-meta:"
        + json.dumps(metadata, ensure_ascii=True, separators=(",", ":"), sort_keys=True)
        + " -->"
    )
(root / ".specify/memory/lessons.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
(root / "improvements/IMP-900-recall-fixture.md").write_text(
    "# IMP-900 - Recall Fixture\n\n"
    "**Status:** ACCEPTED 2026-08-06 - fixture approval\n\n"
    "### SCOPE-3 - Bounded database retry decision\n\n"
    "**Decision:** Keep the bounded database retry under explicit operator control.\n",
    encoding="utf-8",
)
PY
}

adapter_dir_for() {
  local root="$1"
  if [[ -d "$root/.github/bubbles/adapters/experience-recall" ]]; then
    printf '%s' "$root/.github/bubbles/adapters/experience-recall"
  else
    printf '%s' "$root/bubbles/adapters/experience-recall"
  fi
}

select_adapter() {
  local root="$1"
  local adapter="$2"
  printf 'experienceRecall:\n  adapter: %s\n' "$adapter" >"$root/.github/bubbles-project.yaml"
}

write_adversarial_adapters() {
  local root="$1"
  local adapter_dir=""
  adapter_dir="$(adapter_dir_for "$root")"

  cat >"$adapter_dir/malformed.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '{not-json\n'
EOF

  cat >"$adapter_dir/wrong-shape.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  search) printf '{}\n' ;;
  read|status|freshness|sync) printf '[]\n' ;;
  *) exit 8 ;;
esac
EOF

  cat >"$adapter_dir/unsafe-text.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  search)
    printf '%s\n' '[{"recordId":"rec-unsafe","kind":"lesson","sourceTrust":"reviewed-lesson","score":{"total":1},"sourceAnchor":{"relativePath":"evidence/source.md","selector":"#unsafe"},"snippet":"safe\u001b[31munsafe"}]'
    ;;
  read)
    printf '%s\n' '{"recordId":"rec-unsafe","kind":"lesson","sourceTrust":"reviewed-lesson","recallAuthority":"advisory","freshness":{"state":"fresh"},"lifecycle":{"state":"admitted"},"sourceAnchor":{"relativePath":"evidence/source.md","selector":"#unsafe"},"summary":"safe\u001b[31munsafe"}'
    ;;
  status)
    printf '%s\n' '{"adapter":"unsafe-text","state":"ready\u001b[31munsafe","repositoryAlias":"my-repo-name","indexPath":"index.jsonl","recordCount":1,"candidateCount":1,"excludedCount":0,"lifecycleCounts":{"admitted":1,"superseded":0,"expired":0,"deleted":0},"exclusions":{},"freshness":{"state":"fresh","reason":"safe\u001b[31munsafe"}}'
    ;;
  freshness)
    printf '%s\n' '{"state":"fresh","reason":"safe\u001b[31munsafe"}'
    ;;
  sync)
    printf '%s\n' '{"adapter":"unsafe-text","synced":true,"repositoryAlias":"my-repo-name","indexPath":"index.jsonl\u001b[31munsafe","recordCount":1,"candidateCount":1,"excludedCount":0}'
    ;;
  *) exit 8 ;;
esac
EOF

  cat >"$adapter_dir/failure-matrix.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  search) code=6 ;;
  read) code=7 ;;
  status) code=8 ;;
  freshness) code=9 ;;
  sync) code=10 ;;
  *) code=11 ;;
esac
printf '{"operation":"%s","reason":"controlled failure"}\n' "${1:-unknown}"
printf '[experience-recall:failure-matrix][ERROR] controlled %s failure\n' "${1:-unknown}" >&2
exit "$code"
EOF

  cat >"$adapter_dir/unknown-freshness.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' '{"state":"future-state","reason":"not in the closed enum"}'
EOF

  chmod +x "$adapter_dir/malformed.sh" "$adapter_dir/wrong-shape.sh" \
    "$adapter_dir/unsafe-text.sh" "$adapter_dir/failure-matrix.sh" \
    "$adapter_dir/unknown-freshness.sh"
}

last_cli_risk() {
  local root="$1"
  local arguments="$2"
  python3 - "$root/.specify/runtime/framework-events.jsonl" "$arguments" <<'PY'
import json
import pathlib
import sys

events = [json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text(encoding="utf-8").splitlines()]
for event in reversed(events):
    if event.get("command") == "recall" and event.get("details") == "args=" + sys.argv[2]:
        print(event.get("riskClass", ""))
        break
PY
}

assert_last_cli_risk() {
  local label="$1"
  local root="$2"
  local arguments="$3"
  local expected="$4"
  local actual=""
  actual="$(last_cli_risk "$root" "$arguments")"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label"
  else
    fail "$label (expected $expected, got ${actual:-empty})"
  fi
}

write_failing_adapter() {
  local root="$1"
  cat >"$root/bubbles/adapters/experience-recall/atomic-fail.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  sync)
    echo '{"reason":"controlled-sync-failure","synced":false}'
    echo '[experience-recall:atomic-fail][ERROR] controlled sync failure' >&2
    exit 7
    ;;
  *)
    echo '[experience-recall:atomic-fail][ERROR] unexpected verb' >&2
    exit 8
    ;;
esac
EOF
  chmod +x "$root/bubbles/adapters/experience-recall/atomic-fail.sh"
}

echo "experience-recall-cli-selftest"

for required in \
  "$TWIN" \
  "$SCRIPT_DIR/repo-slug.sh" \
  "$SCRIPT_DIR/experience-recall-resolve.sh" \
  "$SCRIPT_DIR/experience-recall-index.py" \
  "$REPO_ROOT/bubbles/adapters/experience-recall/none.sh" \
  "$REPO_ROOT/bubbles/adapters/experience-recall/local-lexical.sh"; do
  if [[ ! -f "$required" ]]; then
    echo "experience-recall-cli-selftest: missing required surface: $required" >&2
    exit 2
  fi
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "experience-recall-cli-selftest: SKIP (python3 not installed)"
  exit 0
fi

NONE_REPO="$WORK/None.Repo"
stage_framework "$NONE_REPO"
NONE_TWIN="$NONE_REPO/bubbles/scripts/experience-recall.sh"
NONE_CLI="$NONE_REPO/bubbles/scripts/cli.sh"

run_case bash "$NONE_TWIN" status --format json
assert_rc "default-none status succeeds explicitly" 0
assert_json_file "default-none status identifies the disabled adapter" \
  'data["adapter"] == "none" and data["state"] == "disabled" and data["recordCount"] == 0'

none_twin_status="$(cat "$LAST_OUT")"
run_case bash "$NONE_CLI" recall status --format json
assert_rc "CLI default-none status succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$none_twin_status" ]]; then
  pass "CLI and twin default-none status are byte-equivalent"
else
  fail "CLI and twin default-none status differ"
fi

run_case bash "$NONE_CLI" help
assert_rc "CLI help succeeds" 0
assert_stdout_contains "CLI help inventories the recall family" "recall <subcommand>"
assert_stdout_contains "CLI help documents closed freshness exits" "0 fresh, 3 stale, 4 unknown, 5 disabled"

run_case bash "$NONE_TWIN" search database --format json
assert_rc "default-none JSON search returns the neutral result" 0
assert_json_file "default-none JSON search is an empty result array" 'data == []'

run_case bash "$NONE_TWIN" search database --format text
assert_rc "default-none text search is non-blocking" 0
assert_stdout_contains "default-none text search says disabled rather than zero matches" "disabled (adapter=none)"

run_case bash "$NONE_TWIN" read recall-missing --format json
assert_rc "default-none read refuses" 5
assert_json_file "default-none read refusal is structured" \
  'data["adapter"] == "none" and data["state"] == "disabled" and data["operation"] == "read"'

run_case bash "$NONE_TWIN" freshness --format json
assert_rc "default-none freshness has the disabled exit" 5
assert_json_file "default-none freshness is distinct from unknown" \
  'data["contractType"] == "freshness" and data["state"] == "disabled"'

run_case bash "$NONE_TWIN" sync --format text
assert_rc "default-none sync refuses" 5
assert_stdout_contains "default-none sync names the disabled adapter" "disabled (adapter=none)"

run_case bash "$NONE_TWIN" status --repo-root "$WORK"
assert_rc "the twin rejects a repository-root override" 2
assert_stderr_contains "the twin explains repository rooting" "repository root is derived"

run_case bash "$NONE_CLI" recall status --repository-alias other
assert_rc "the CLI rejects a repository-alias override" 2
assert_stderr_contains "the CLI keeps repository alias derived" "repository alias is derived"

run_case bash "$NONE_TWIN" status --repo-root="$WORK"
assert_rc "the twin rejects an equals-form repository-root override" 2
run_case bash "$NONE_TWIN" status --repository-alias=other
assert_rc "the twin rejects an equals-form repository-alias override" 2
run_case bash "$NONE_TWIN" status --adapter none
assert_rc "the twin rejects a separated adapter override" 2
run_case bash "$NONE_CLI" recall status --adapter=none
assert_rc "the CLI rejects an equals-form adapter override" 2

for unavailable_subcommand in export delete admit lifecycle; do
  run_case bash "$NONE_CLI" recall "$unavailable_subcommand"
  assert_rc "the CLI does not expose $unavailable_subcommand before its owning scope" 2
  assert_stderr_contains "$unavailable_subcommand refusal names the scope boundary" "not available in this scope"
done

if [[ ! -e "$NONE_REPO/.specify/runtime/experience-recall" ]]; then
  pass "disabled read-only recall commands create no recall runtime state"
else
  fail "disabled read-only recall commands created recall runtime state"
fi

for risk_case in \
  'search database --format json|read_only' \
  'read recall-missing --format json|read_only' \
  'status --format json|read_only' \
  'freshness --format json|read_only' \
  'sync --format json|owned_mutation'; do
  risk_args="${risk_case%%|*}"
  risk_expected="${risk_case##*|}"
  # shellcheck disable=SC2086
  run_case bash "$NONE_CLI" recall $risk_args
  assert_last_cli_risk "CLI effective risk is $risk_expected for recall $risk_args" \
    "$NONE_REPO" "$risk_args" "$risk_expected"
done

run_case bash "$NONE_CLI" recall future-mutation
assert_rc "an unknown recall subcommand reaches usage refusal" 2
assert_last_cli_risk "unknown recall subcommands are conservatively classified before refusal" \
  "$NONE_REPO" "future-mutation" "owned_mutation"

FALLBACK_REPO="$WORK/!!!"
stage_framework "$FALLBACK_REPO"
run_case bash "$FALLBACK_REPO/bubbles/scripts/experience-recall.sh" status --format json
assert_rc "a punctuation-only repository basename uses installer fallback semantics" 0
assert_json_file "the fallback repository alias matches the installer marker" \
  'data["repositoryAlias"] == "repo" and data["state"] == "disabled"'

DOWNSTREAM_REPO="$WORK/Installed Product"
stage_framework "$DOWNSTREAM_REPO" downstream
write_local_fixture "$DOWNSTREAM_REPO" "installed-product"
DOWNSTREAM_TWIN="$DOWNSTREAM_REPO/.github/bubbles/scripts/experience-recall.sh"
run_case bash "$DOWNSTREAM_TWIN" sync --format json
assert_rc "installed-downstream layout derives its product root" 0
assert_json_file "installed-downstream layout binds the intended repository alias" \
  'data["repositoryAlias"] == "installed-product" and data["synced"] is True'

LOCAL_REPO="$WORK/My.Repo_Name"
stage_framework "$LOCAL_REPO"
write_local_fixture "$LOCAL_REPO"
LOCAL_TWIN="$LOCAL_REPO/bubbles/scripts/experience-recall.sh"
LOCAL_CLI="$LOCAL_REPO/bubbles/scripts/cli.sh"

run_case bash "$LOCAL_TWIN" freshness --format json
assert_rc "unsynchronized local freshness exits unknown" 4
assert_json_file "unsynchronized local freshness is not fresh" \
  'data["state"] == "unknown" and data["reason"] == "index has not been synchronized"'

run_case bash "$LOCAL_TWIN" search "bounded database retry" --format json
assert_rc "unsynchronized local search preserves provider refusal" 1
assert_stderr_contains "unsynchronized search keeps the provider code" "index-missing"

run_case bash "$LOCAL_TWIN" read recall-missing --format json
assert_rc "unsynchronized local read preserves provider refusal" 1
assert_stderr_contains "unsynchronized read keeps the provider code" "index-missing"

run_case bash "$LOCAL_TWIN" sync --format json
assert_rc "local sync succeeds" 0
assert_json_file "local sync uses the canonical repository alias" \
  'data["adapter"] == "local-lexical" and data["repositoryAlias"] == "my-repo-name" and data["synced"] is True and data["recordCount"] >= 3'
sync_json="$(cat "$LAST_OUT")"

SYMLINK_REPO="$WORK/Symlink.Switch"
mkdir -p "$SYMLINK_REPO"
ln -s "$LOCAL_REPO/bubbles" "$SYMLINK_REPO/bubbles"
run_case bash "$SYMLINK_REPO/bubbles/scripts/experience-recall.sh" status --format json
assert_rc "a symlinked framework path resolves to its physical repository" 0
assert_json_file "a symlinked invocation cannot switch the derived repository alias" \
  'data["repositoryAlias"] == "my-repo-name" and data["indexPath"].startswith(".specify/runtime/")'

FILE_SYMLINK_REPO="$WORK/File.Symlink.Switch"
stage_framework "$FILE_SYMLINK_REPO"
rm "$FILE_SYMLINK_REPO/bubbles/scripts/experience-recall.sh"
ln -s "$LOCAL_TWIN" "$FILE_SYMLINK_REPO/bubbles/scripts/experience-recall.sh"
run_case bash "$FILE_SYMLINK_REPO/bubbles/scripts/experience-recall.sh" status --format json
assert_rc "a file-symlinked twin resolves without switching repositories" 0
assert_json_file "a file-symlinked twin remains rooted to its physical source repository" \
  'data["repositoryAlias"] == "my-repo-name" and data["state"] == "ready"'

run_case bash "$LOCAL_CLI" recall sync --format json
assert_rc "CLI local sync succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$sync_json" ]]; then
  pass "CLI and twin sync JSON are byte-equivalent"
else
  fail "CLI and twin sync JSON differ"
fi

run_case bash "$LOCAL_TWIN" sync --format text
assert_rc "local text sync succeeds" 0
assert_stdout_contains "local text sync reports the provider and mutation result" "adapter=local-lexical synced=true"
sync_text="$(cat "$LAST_OUT")"
run_case bash "$LOCAL_CLI" recall sync --format text
assert_rc "CLI local text sync succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$sync_text" ]]; then
  pass "CLI and twin sync text are byte-equivalent"
else
  fail "CLI and twin sync text differ"
fi

run_case bash "$LOCAL_TWIN" search "bounded database retry" --limit 2 --format json
assert_rc "bounded JSON search succeeds" 0
assert_json_file "bounded search obeys limit and repository alias" \
  'len(data) == 2 and all(item["repositoryAlias"] == "my-repo-name" for item in data)'
search_json="$(cat "$LAST_OUT")"
record_id="$(python3 -c 'import json,sys; print(next(item["recordId"] for item in json.load(open(sys.argv[1], encoding="utf-8")) if item["kind"] == "lesson"))' "$LAST_OUT")"

run_case bash "$LOCAL_CLI" recall search "bounded database retry" --limit 2 --format json
assert_rc "CLI bounded JSON search succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$search_json" ]]; then
  pass "CLI and twin search JSON are byte-equivalent"
else
  fail "CLI and twin search JSON differ"
fi

run_case bash "$LOCAL_TWIN" search "bounded database retry" \
  --kind lesson --trust reviewed-lesson --limit 5 --format json
assert_rc "kind and trust filters succeed" 0
assert_json_file "kind and trust filters are forwarded exactly" \
  'len(data) == 3 and all(item["kind"] == "lesson" and item["sourceTrust"] == "reviewed-lesson" for item in data)'

run_case bash "$LOCAL_TWIN" search "bounded database retry" \
  --kind lesson --kind owner-decision --limit 5 --format json
assert_rc "repeatable kind filters succeed" 0
assert_json_file "repeatable kind filters retain both requested families" \
  'len(data) == 4 and {item["kind"] for item in data} == {"lesson", "owner-decision"}'

run_case bash "$LOCAL_TWIN" search "bounded database retry" \
  --trust reviewed-lesson --trust owner-approved --limit 5 --format json
assert_rc "repeatable trust filters succeed" 0
assert_json_file "repeatable trust filters retain both requested trust classes" \
  'len(data) == 4 and {item["sourceTrust"] for item in data} == {"reviewed-lesson", "owner-approved"}'

run_case bash "$LOCAL_TWIN" search "-bounded database retry" --limit 2 --format json
assert_rc "a query beginning with a hyphen is data rather than an option" 0
assert_json_file "a hyphen-leading query remains bounded" 'len(data) == 2'

run_case bash "$LOCAL_TWIN" search $'bounded\ndatabase retry' --limit 2 --format json
assert_rc "a newline-bearing query is handled without argument splitting" 0
assert_json_file "a newline-bearing query remains bounded" 'len(data) == 2'
assert_terminal_safe "a newline-bearing query emits no terminal control bytes"

run_case bash "$LOCAL_TWIN" search $'bounded\033database retry' --limit 2 --format text
assert_rc "a control-bearing query is handled deterministically" 0
assert_terminal_safe "a control-bearing query cannot inject terminal controls"

run_case bash "$LOCAL_TWIN" search "bounded database retry" --kind $'lesson\nowner' --format json
assert_rc "a control-bearing filter value is rejected" 1
assert_terminal_safe "a control-bearing filter refusal emits no terminal controls"

run_case bash "$LOCAL_TWIN" search "bounded database retry" \
  --kind owner-decision --scope-ref SCOPE-3 --limit 5 --format json
assert_rc "scope filter succeeds" 0
assert_json_file "scope filter retains the accepted decision only" \
  'len(data) == 1 and data[0]["kind"] == "owner-decision" and data[0]["scopeRef"] == "SCOPE-3"'

run_case bash "$LOCAL_TWIN" search "bounded database retry" \
  --spec-ref specs/404-missing --format json
assert_rc "spec filter with no exact match succeeds" 0
assert_json_file "spec filter does not fall back to unscoped records" 'data == []'

run_case bash "$LOCAL_TWIN" search "bounded database retry" --limit 1 --format text
assert_rc "concise text search succeeds" 0
assert_stdout_contains "text search includes the bounded record identity" "record="
assert_stdout_bounded "text search is line- and byte-bounded" 1 2048
if grep -qF 'SOURCE_BODY_SENTINEL' "$LAST_OUT"; then
  fail "text search expanded source body content"
else
  pass "text search does not expand source bodies"
fi
search_text="$(cat "$LAST_OUT")"

run_case bash "$LOCAL_CLI" recall search "bounded database retry" --limit 1 --format text
assert_rc "CLI concise text search succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$search_text" ]]; then
  pass "CLI and twin search text are byte-equivalent"
else
  fail "CLI and twin search text differ"
fi

run_case bash "$LOCAL_TWIN" read "$record_id" --format json
assert_rc "JSON read succeeds while the anchor is current" 0
assert_json_file "JSON read returns the selected advisory record" \
  'data["recordId"] == "'"$record_id"'" and data["recallAuthority"] == "advisory"'
read_json="$(cat "$LAST_OUT")"

run_case bash "$LOCAL_CLI" recall read "$record_id" --format json
assert_rc "CLI JSON read succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$read_json" ]]; then
  pass "CLI and twin read JSON are byte-equivalent"
else
  fail "CLI and twin read JSON differ"
fi

run_case bash "$LOCAL_TWIN" read "$record_id" --format text
assert_rc "concise text read succeeds" 0
assert_stdout_contains "text read reports advisory authority" "authority=advisory"
assert_stdout_bounded "text read is line- and byte-bounded" 5 4096
if grep -qF 'SOURCE_BODY_SENTINEL' "$LAST_OUT"; then
  fail "text read expanded source body content"
else
  pass "text read does not expand source bodies"
fi

run_case bash "$LOCAL_TWIN" status --format json
assert_rc "local status succeeds" 0
assert_json_file "local status reports provider, exclusions, and index" \
  'data["adapter"] == "local-lexical" and data["state"] == "ready" and data["excludedCount"] == 1 and data["indexPath"].endswith("index.jsonl") and data["lifecycleCounts"] == {"admitted": data["recordCount"], "deleted": 0, "expired": 0, "superseded": 0}'

run_case bash "$LOCAL_TWIN" status --format text
assert_rc "local text status succeeds" 0
assert_stdout_contains "text status reports lifecycle counts" "lifecycle admitted="
assert_stdout_contains "text status reports excluded sources" "excluded=1"
assert_stdout_bounded "text status is line- and byte-bounded" 5 4096
status_text="$(cat "$LAST_OUT")"
run_case bash "$LOCAL_CLI" recall status --format text
assert_rc "CLI local text status succeeds" 0
if [[ "$(cat "$LAST_OUT")" == "$status_text" ]]; then
  pass "CLI and twin status text are byte-equivalent"
else
  fail "CLI and twin status text differ"
fi

run_case bash "$LOCAL_TWIN" freshness --format json
assert_rc "current local freshness exits fresh" 0
assert_json_file "current local freshness is fresh" 'data["state"] == "fresh"'

run_case bash "$LOCAL_TWIN" freshness --format text
assert_rc "current local text freshness exits fresh" 0
assert_stdout_contains "local text freshness reports fresh" "adapter=local-lexical freshness=fresh"
freshness_text="$(cat "$LAST_OUT")"
run_case bash "$LOCAL_CLI" recall freshness --format text
assert_rc "CLI local text freshness exits fresh" 0
if [[ "$(cat "$LAST_OUT")" == "$freshness_text" ]]; then
  pass "CLI and twin freshness text are byte-equivalent"
else
  fail "CLI and twin freshness text differ"
fi

index_file="$LOCAL_REPO/.specify/runtime/experience-recall/index.jsonl"
status_file="$LOCAL_REPO/.specify/runtime/experience-recall/status.json"
index_read_only_before="$(file_fingerprint "$index_file")"
status_read_only_before="$(file_fingerprint "$status_file")"
run_case bash "$LOCAL_TWIN" search "bounded database retry" --limit 1 --format json
assert_rc "read-only immutability probe search succeeds" 0
run_case bash "$LOCAL_TWIN" read "$record_id" --format json
assert_rc "read-only immutability probe read succeeds" 0
run_case bash "$LOCAL_TWIN" status --format json
assert_rc "read-only immutability probe status succeeds" 0
run_case bash "$LOCAL_TWIN" freshness --format json
assert_rc "read-only immutability probe freshness succeeds" 0
index_read_only_after="$(file_fingerprint "$index_file")"
status_read_only_after="$(file_fingerprint "$status_file")"
if [[ "$index_read_only_before" == "$index_read_only_after" \
  && "$status_read_only_before" == "$status_read_only_after" ]]; then
  pass "search/read/status/freshness leave recall runtime bytes and mtimes unchanged"
else
  fail "a read-only recall command changed recall runtime bytes or mtimes"
fi

printf '%s\n' 'changed bounded database retry evidence' >"$LOCAL_REPO/evidence/source.md"
run_case bash "$LOCAL_TWIN" freshness --format json
assert_rc "digest mismatch freshness exits stale" 3
assert_json_file "stale freshness remains machine-readable" 'data["state"] == "stale"'

run_case bash "$LOCAL_TWIN" search "bounded database retry" --format json
assert_rc "stale search preserves provider refusal" 1
assert_stderr_contains "stale search preserves the provider code" "index-stale"

run_case bash "$LOCAL_TWIN" read "$record_id" --format json
assert_rc "stale read preserves provider refusal" 1
assert_stderr_contains "stale read preserves the provider code" "record-stale"

mv "$LOCAL_REPO/evidence/source.md" "$WORK/unavailable-source.md"
run_case bash "$LOCAL_TWIN" freshness --format json
assert_rc "unavailable anchor freshness exits unknown" 4
assert_json_file "unavailable anchor is unknown rather than fresh" 'data["state"] == "unknown"'

run_case bash "$LOCAL_TWIN" search "bounded database retry" --format json
assert_rc "unknown-state search preserves provider refusal" 1
assert_stderr_contains "unknown-state search keeps the provider code" "index-unknown"

run_case bash "$LOCAL_TWIN" read "$record_id" --format json
assert_rc "unknown-state read preserves provider refusal" 1
assert_stderr_contains "unknown-state read keeps the provider code" "missing-anchor"
mv "$WORK/unavailable-source.md" "$LOCAL_REPO/evidence/source.md"

# Restore the indexed source bytes before checking delegated sync failure.
printf '%s\n' 'SOURCE_BODY_SENTINEL bounded database retry evidence' >"$LOCAL_REPO/evidence/source.md"
run_case bash "$LOCAL_TWIN" sync --format json
assert_rc "restored local corpus resynchronizes" 0
index_before="$(sha256_file "$index_file")"
status_before="$(sha256_file "$status_file")"
write_failing_adapter "$LOCAL_REPO"
printf 'experienceRecall:\n  adapter: atomic-fail\n' >"$LOCAL_REPO/.github/bubbles-project.yaml"
run_case bash "$LOCAL_TWIN" sync --format json
assert_rc "controlled provider sync failure is preserved" 7
assert_stderr_contains "controlled provider refusal remains visible" "controlled sync failure"
index_after="$(sha256_file "$index_file")"
status_after="$(sha256_file "$status_file")"
if [[ "$index_before" == "$index_after" && "$status_before" == "$status_after" ]]; then
  pass "failed delegated sync leaves the prior index and status unchanged"
else
  fail "failed delegated sync changed prior derived state"
fi
printf 'experienceRecall:\n  adapter: local-lexical\n' >"$LOCAL_REPO/.github/bubbles-project.yaml"

write_adversarial_adapters "$LOCAL_REPO"

select_adapter "$LOCAL_REPO" failure-matrix
for failure_case in \
  'search database --format json|6' \
  'read recall-record --format json|7' \
  'status --format json|8' \
  'freshness --format json|9' \
  'sync --format json|10'; do
  failure_args="${failure_case%%|*}"
  failure_rc="${failure_case##*|}"
  # shellcheck disable=SC2086
  run_case bash "$LOCAL_TWIN" $failure_args
  assert_rc "provider failure exit is preserved for $failure_args" "$failure_rc"
  assert_stderr_contains "provider failure remains visible for $failure_args" "controlled"
done

select_adapter "$LOCAL_REPO" malformed
run_case bash "$LOCAL_TWIN" search database --format json
assert_clean_provider_failure "malformed search JSON is refused cleanly"
run_case bash "$LOCAL_TWIN" search database --format text
assert_clean_provider_failure "malformed text-rendered search JSON has no traceback"
run_case bash "$LOCAL_TWIN" read recall-record --format json
assert_clean_provider_failure "malformed read JSON is refused cleanly"
run_case bash "$LOCAL_TWIN" status --format json
assert_clean_provider_failure "malformed status JSON is refused cleanly"
run_case bash "$LOCAL_TWIN" freshness --format json
assert_clean_provider_failure "malformed freshness JSON is refused cleanly"
run_case bash "$LOCAL_TWIN" sync --format json
assert_clean_provider_failure "malformed sync JSON is refused cleanly"

select_adapter "$LOCAL_REPO" wrong-shape
run_case bash "$LOCAL_TWIN" search database --format json
assert_clean_provider_failure "object-shaped search response is refused cleanly"
run_case bash "$LOCAL_TWIN" read recall-record --format json
assert_clean_provider_failure "array-shaped read response is refused cleanly"
run_case bash "$LOCAL_TWIN" status --format json
assert_clean_provider_failure "array-shaped status response is refused cleanly"
run_case bash "$LOCAL_TWIN" freshness --format json
assert_clean_provider_failure "array-shaped freshness response is refused cleanly"
run_case bash "$LOCAL_TWIN" sync --format json
assert_clean_provider_failure "array-shaped sync response is refused cleanly"

select_adapter "$LOCAL_REPO" unknown-freshness
run_case bash "$LOCAL_TWIN" freshness --format json
assert_clean_provider_failure "unknown freshness state is refused with exit 1"
assert_stderr_not_contains "unknown freshness refusal has no traceback" "Traceback"

select_adapter "$LOCAL_REPO" unsafe-text
run_case bash "$LOCAL_TWIN" search database --format text
assert_safe_text_or_refusal "search text sanitizes or refuses terminal escapes"
run_case bash "$LOCAL_TWIN" read recall-record --format text
assert_safe_text_or_refusal "read text sanitizes or refuses terminal escapes"
run_case bash "$LOCAL_TWIN" status --format text
assert_safe_text_or_refusal "status text sanitizes or refuses terminal escapes"
run_case bash "$LOCAL_TWIN" freshness --format text
assert_safe_text_or_refusal "freshness text sanitizes or refuses terminal escapes"
run_case bash "$LOCAL_TWIN" sync --format text
assert_safe_text_or_refusal "sync text sanitizes or refuses terminal escapes"

select_adapter "$LOCAL_REPO" local-lexical

run_case bash "$LOCAL_TWIN" search database --limit 0
assert_rc "limit zero is rejected at the public interface" 2
run_case bash "$LOCAL_TWIN" search database --limit 21
assert_rc "limit above twenty is rejected at the public interface" 2
run_case bash "$LOCAL_TWIN" search database --format yaml
assert_rc "unknown output format is rejected" 2
run_case bash "$LOCAL_TWIN" search database --unknown-option
assert_rc "unknown search option is rejected" 2
run_case bash "$LOCAL_TWIN" search
assert_rc "missing search query is rejected" 2
run_case bash "$LOCAL_TWIN" search database --limit
assert_rc "missing limit value is rejected" 2
run_case bash "$LOCAL_TWIN" search database --kind
assert_rc "missing kind value is rejected" 2
run_case bash "$LOCAL_TWIN" search database --trust
assert_rc "missing trust value is rejected" 2
run_case bash "$LOCAL_TWIN" search database --spec-ref
assert_rc "missing spec-ref value is rejected" 2
run_case bash "$LOCAL_TWIN" search database --scope-ref
assert_rc "missing scope-ref value is rejected" 2
run_case bash "$LOCAL_TWIN" search database --format
assert_rc "missing format value is rejected" 2
run_case bash "$LOCAL_TWIN" search database --limit 1 --limit 2
assert_rc "duplicate limit is rejected" 2
run_case bash "$LOCAL_TWIN" search database --spec-ref specs/one --spec-ref specs/two
assert_rc "duplicate spec-ref is rejected" 2
run_case bash "$LOCAL_TWIN" search database --scope-ref SCOPE-1 --scope-ref SCOPE-2
assert_rc "duplicate scope-ref is rejected" 2
run_case bash "$LOCAL_TWIN" search database --format json --format text
assert_rc "duplicate format is rejected" 2
run_case bash "$LOCAL_TWIN" read "$record_id" --format json --format text
assert_rc "duplicate read format is rejected" 2
run_case bash "$LOCAL_TWIN" status --format json --format text
assert_rc "duplicate status format is rejected" 2
run_case bash "$LOCAL_TWIN" freshness --format json --format text
assert_rc "duplicate freshness format is rejected" 2
run_case bash "$LOCAL_TWIN" sync --format json --format text
assert_rc "duplicate sync format is rejected" 2
run_case bash "$LOCAL_TWIN" read
assert_rc "missing read record id is rejected" 2
run_case bash "$LOCAL_TWIN" read "$record_id" trailing
assert_rc "unexpected read arguments are rejected" 2
run_case bash "$LOCAL_TWIN" read $'record\033id' --format json
assert_rc "a control-bearing record id is rejected" 1
assert_terminal_safe "a control-bearing record-id refusal emits no terminal controls"
run_case bash "$LOCAL_TWIN" status trailing
assert_rc "unexpected status arguments are rejected" 2
run_case bash "$LOCAL_TWIN" freshness trailing
assert_rc "unexpected freshness arguments are rejected" 2
run_case bash "$LOCAL_TWIN" sync trailing
assert_rc "unexpected sync arguments are rejected" 2
run_case bash "$LOCAL_TWIN" search database --kind unknown-kind --format json
assert_rc "provider rejects an unknown kind without wrapper fallback" 1
assert_stderr_contains "unknown kind keeps the provider's structured code" "invalid-query"

if grep -Eq '(^|[^[:alnum:]_])(curl|wget|pip|npm|npx|brew|apt|git[[:space:]]+clone|requests|urllib|socket)([^[:alnum:]_]|$)' \
  "$TWIN" "$SCRIPT_DIR/repo-slug.sh"; then
  fail "public recall twin or slug helper introduced a network/package command"
else
  pass "public recall twin and slug helper contain no network/package command"
fi

if [[ "$FAIL" -gt 0 ]]; then
  echo "experience-recall-cli-selftest: $PASS passed, $FAIL failed"
  exit 1
fi

echo "experience-recall-cli-selftest: $PASS passed, $FAIL failed"
