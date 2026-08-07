#!/usr/bin/env bash
# Hermetic acceptance tests for the local lexical experience-recall provider.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
INDEXER="$SCRIPT_DIR/experience-recall-index.py"
ADAPTER="$REPO_ROOT/bubbles/adapters/experience-recall/local-lexical.sh"
SCHEMA="$REPO_ROOT/bubbles/schemas/experience-recall.schema.json"
WORK="$(mktemp -d)"
FIXTURE="$WORK/fixture"
WRITER_FIXTURE="$WORK/writer"
OUTSIDE="$WORK/outside-result.json"
PASS=0
FAIL=0

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

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

assert_json() {
  local label="$1"
  local payload="$2"
  local expression="$3"
  if python3 -c 'import json, sys; data = json.loads(sys.argv[1]); assert eval(sys.argv[2], {"data": data})' \
      "$payload" "$expression"; then
    pass "$label"
  else
    fail "$label"
    echo "  payload: $payload"
  fi
}

assert_rejected() {
  local label="$1"
  local expected="$2"
  shift 2
  local case_id=$((PASS + FAIL + 1))
  local stdout_file="$WORK/rejected-$case_id.out"
  local stderr_file="$WORK/rejected-$case_id.err"
  local rc=0
  set +e
  "$@" >"$stdout_file" 2>"$stderr_file"
  rc=$?
  set -e
  if [[ "$rc" -eq 1 ]] && grep -qF "$expected" "$stderr_file"; then
    pass "$label"
  else
    fail "$label"
    echo "  exit: $rc"
    echo "  stderr: $(cat "$stderr_file")"
  fi
}

if [[ ! -f "$INDEXER" || ! -f "$ADAPTER" ]]; then
  echo "experience-recall-index-selftest: missing indexer or adapter" >&2
  exit 2
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "experience-recall-index-selftest: SKIP (python3 not installed)"
  exit 0
fi

mkdir -p "$FIXTURE/.specify/memory" "$FIXTURE/evidence" "$FIXTURE/improvements" "$FIXTURE/notes"

python3 - "$FIXTURE" "$OUTSIDE" <<'PY'
import hashlib
import json
import os
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
outside = pathlib.Path(sys.argv[2])
stamp = "2026-08-06T12:00:00Z"
epoch = 1786017600


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(value, (dict, list)):
        body = json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n"
    else:
        body = value
    path.write_text(body, encoding="utf-8")
    os.utime(path, (epoch, epoch))


def digest(path):
    return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()


resolution = {
    "sessionId": "fixture-session",
    "decisionId": "rb:fixture-session:1",
    "controlRevision": 1,
    "controlPathDigest": "sha256:" + "1" * 64,
    "authority": "explicit-repository-root",
    "transition": "confirmed",
    "scopeKind": "command",
    "scopeId": None,
    "targetKind": "repository-root",
    "pathVisibility": "local",
    "actionable": True,
}


def compacted(raw_pointer=None):
    value = {
        "agent": "bubbles.implement",
        "outcome": "completed_owned",
        "featureDir": "specs/100-fixture",
        "scopeIds": "SCOPE-1",
        "evidenceRefs": ["report.md#focused-proof"],
        "repositoryRoot": str(root),
        "repositoryAlias": "fixture",
        "repositoryResolution": dict(resolution),
        "timestamp": stamp,
    }
    if raw_pointer is not None:
        value["rawPointer"] = str(raw_pointer)
    return value


source = root / "evidence/source.md"
# portable-ok:domain fixture text, not a timeout utility invocation
write(source, "anchored database timeout evidence\n")

raw = root / "evidence/result-one.json"
write(
    raw,
    {
        "agent": "bubbles.implement",
        "outcome": "completed_owned",
        "summary": "Closed a bounded fixture result.",
        "evidenceRefs": ["report.md#focused-proof"],
        "findings": [
            {
                "id": "FIX-001",
                "severity": "warn",
                "owner": "bubbles.implement",
                "summary": "A bounded retry needed an explicit cap.",
                "scopeRef": "SCOPE-1",
                "scenarioRef": "SCN-001",
            }
        ],
        "addressedFindings": ["FIX-001"],
        "unresolvedFindings": [],
        "rawArtifactBody": "DO_NOT_COPY_RAW_SENTINEL",
    },
)

markdown_raw = root / "evidence/result-markdown.md"
write(
    markdown_raw,
    """# Focused test output

Arbitrary raw body MARKDOWN_RAW_BODY_SENTINEL must not enter recall.

## RESULT-ENVELOPE

```yaml
agent: bubbles.test
outcome: route_required
summary: Canonical YAML result requires a bounded parser.
evidenceRefs:
  - report.md#yaml-envelope-proof
findings:
  - id: YAML-FINDING-001
    severity: blocker
    owner: bubbles.super
    summary: Canonical YAML findings must remain structured.
    scopeRef: SCOPE-2
    scenarioRef: SCN-YAML-001
addressedFindings:
  - YAML-ADDRESSED-001
unresolvedFindings:
  - YAML-FINDING-001
nextRequiredOwner: bubbles.super
rawArtifactBody: YAML_RAW_SENTINEL
```
""",
)

bad_command_raw = root / "evidence/result-bad-command.json"
write(
    bad_command_raw,
    {
        "agent": "bubbles.implement",
        "outcome": "completed_owned",
        "summary": "INCOHERENT_COMMAND_PACKET_SENTINEL",
        "evidenceRefs": ["report.md#bad-command-packet"],
        "findings": [],
        "addressedFindings": [],
        "unresolvedFindings": [],
    },
)

bad_goal_raw = root / "evidence/result-bad-goal.json"
write(
    bad_goal_raw,
    {
        "agent": "bubbles.implement",
        "outcome": "completed_owned",
        "summary": "INCOHERENT_GOAL_PACKET_SENTINEL",
        "evidenceRefs": ["report.md#bad-goal-packet"],
        "findings": [],
        "addressedFindings": [],
        "unresolvedFindings": [],
    },
)

transcript = root / "evidence/chat-transcript.json"
write(
    transcript,
    {
        "agent": "bubbles.implement",
        "outcome": "completed_owned",
        "evidenceRefs": ["report.md#not-admissible"],
        "chat": "TRANSCRIPT_SENTINEL",
    },
)
write(
    outside,
    {
        "agent": "bubbles.implement",
        "outcome": "completed_owned",
        "evidenceRefs": ["report.md#outside"],
    },
)
(root / "evidence/link-result.json").symlink_to(outside)

valid = compacted(raw)
valid["sourceDigest"] = digest(raw)
markdown_result = compacted(markdown_raw)
markdown_result["agent"] = "bubbles.test"
markdown_result["outcome"] = "route_required"
markdown_result["sourceDigest"] = digest(markdown_raw)
bad_command = compacted(bad_command_raw)
bad_command["sourceDigest"] = digest(bad_command_raw)
bad_command["repositoryResolution"]["decisionId"] = "rb:different-session:1"
bad_goal = compacted(bad_goal_raw)
bad_goal["sourceDigest"] = digest(bad_goal_raw)
bad_goal["repositoryResolution"] = {
    "sessionId": "fixture-session",
    "decisionId": "rb:fixture-session:1:node:different-node",
    "controlRevision": 1,
    "controlPathDigest": "sha256:" + "2" * 64,
    "authority": "scoped-scenario-node",
    "transition": "scoped-override",
    "scopeKind": "goal-node",
    "scopeId": "fixture-node",
    "targetKind": "goal-node",
    "pathVisibility": "local",
    "actionable": True,
}
session = {
    "compactedHistory": [
        valid,
        markdown_result,
        bad_command,
        bad_goal,
        compacted(),
        compacted(outside),
        compacted(root / "evidence/link-result.json"),
        compacted(transcript),
    ]
}
write(root / ".specify/memory/bubbles.session.json", session)

valid_meta = {
    "capturedAt": stamp,
    "lessonId": "lesson-" + "a" * 64,
    "repositoryAlias": "fixture",
    "reviewState": "anchored",
    "schemaVersion": 1,
    "sourceAnchor": {
        "contentDigest": digest(source),
        "observedAt": stamp,
        "relativePath": "evidence/source.md",
        "selector": "#database-timeout-proof",
    },
}
bad_meta = json.loads(json.dumps(valid_meta))
bad_meta["lessonId"] = "lesson-" + "b" * 64
bad_meta["sourceAnchor"]["contentDigest"] = "sha256:" + "0" * 64
lesson_prefix = (
    "- problem: database timeout; root cause: retry budget was missing; "
    "fix: bound every retry; applies when: database timeout repeats"
)
lessons = "\n".join(
    [
        "# Lessons",
        "",
        lesson_prefix + " <!-- bubbles-lesson-meta:" + json.dumps(valid_meta, sort_keys=True, separators=(",", ":")) + " -->",
        "- problem: old lesson; root cause: no anchor; fix: keep for clustering; applies when: legacy input remains",
        lesson_prefix + " <!-- bubbles-lesson-meta:" + json.dumps(bad_meta, sort_keys=True, separators=(",", ":")) + " -->",
        "",
    ]
)
write(root / ".specify/memory/lessons.md", lessons)

improvement = """# IMP-100 - Fixture decision

**Status:** ACCEPTED 2026-08-06 - owner approved

### SCOPE-1 - First ordering rule (LRN-4)

**Decision:** Use deterministic lexical ordering.

Arbitrary proposal prose must not become a record.

### SCOPE-2 - Second ordering rule (EV-7)

**Decision:** Use deterministic lexical ordering.
"""
write(root / "improvements/IMP-100-fixture-decision.md", improvement)
current_style = """# IMP-102 - Current accepted style

**Status:** IN PROGRESS (SCOPE-1 owner-authorized 2026-08-06; SCOPE-2 remains)

### SCOPE-2 - Current proposal grammar (EV-7)

**Decision:** CURRENT_IMP_STYLE_DECISION_SENTINEL is admitted from a real proposal shape.
"""
write(root / "improvements/IMP-102-current-style.md", current_style)
undated = """# IMP-101 - Undated proposal

**Status:** ACCEPTED - no same-source date

### SCOPE-1 - Unsupported decision

**Decision:** UNDATED_DECISION_SENTINEL must not be indexed.
"""
write(root / "improvements/IMP-101-undated.md", undated)
write(root / "notes/random.md", "ARBITRARY_MARKDOWN_SENTINEL database timeout\n")
write(root / "notes/source.py", "SOURCE_CODE_SENTINEL = 'database timeout'\n")
PY

echo "experience-recall-index-selftest"

missing_status="$(bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture)"
assert_json "provider status distinguishes an unsynchronized index" "$missing_status" \
  'data["state"] == "missing" and data["recordCount"] == 0'

missing_freshness="$(bash "$ADAPTER" freshness --repo-root "$FIXTURE" --repository-alias fixture)"
assert_json "provider freshness reports unknown before the first synchronization" "$missing_freshness" \
  'data["contractType"] == "freshness" and data["state"] == "unknown" and data["sourceDigest"] is None and isinstance(data["checkedAt"], str) and data["checkedAt"] and data["reason"] == "index has not been synchronized"'

capabilities="$(bash "$ADAPTER" capabilities)"
assert_json "provider capabilities report derived verbs and unsupported lifecycle verbs" "$capabilities" \
  'data["adapter"] == "local-lexical" and data["capabilities"]["sync"] == "derived" and data["capabilities"]["export"] == "unsupported" and data["capabilities"]["delete"] == "unsupported" and data["networkAccess"] is False and data["automaticInstall"] is False'

sync_output="$(bash "$ADAPTER" sync --repo-root "$FIXTURE" --repository-alias fixture)"
if python3 - "$sync_output" <<'PY'
import json
import sys

status = json.loads(sys.argv[1])
exclusions = status["exclusions"]
assert status["synced"] is True
assert status["candidateCount"] == 15
assert status["excludedCount"] == sum(exclusions.values())
for reason, count in {
    "digest-mismatch": 1,
    "invalid-repository-packet": 2,
    "missing-anchor": 1,
    "transcript-like-input": 1,
    "unanchored-lesson": 1,
    "unsafe-anchor": 2,
}.items():
    assert exclusions.get(reason) == count, (reason, exclusions)
assert exclusions.get("owner-decision-unsupported") in {1, 2}
assert exclusions.get("result-envelope-unsupported", 0) in {0, 1}
allowed = {
    "digest-mismatch",
    "invalid-repository-packet",
    "missing-anchor",
    "owner-decision-unsupported",
    "result-envelope-unsupported",
    "transcript-like-input",
    "unanchored-lesson",
    "unsafe-anchor",
}
assert set(exclusions) <= allowed, exclusions
PY
then
  pass "sync accounts exactly for the complete closed corpus"
else
  fail "sync omitted, admitted, or misclassified a closed-corpus candidate"
fi

INDEX_FILE="$FIXTURE/.specify/runtime/experience-recall/index.jsonl"
STATUS_FILE="$FIXTURE/.specify/runtime/experience-recall/status.json"
index_hash_one="$(sha256_file "$INDEX_FILE")"
status_hash_one="$(sha256_file "$STATUS_FILE")"
bash "$ADAPTER" sync --repo-root "$FIXTURE" --repository-alias fixture >/dev/null
index_hash_two="$(sha256_file "$INDEX_FILE")"
status_hash_two="$(sha256_file "$STATUS_FILE")"
if [[ "$index_hash_one" == "$index_hash_two" && "$status_hash_one" == "$status_hash_two" ]]; then
  pass "rebuilding the same corpus produces byte-identical index and status files"
else
  fail "deterministic rebuild changed derived bytes"
fi

if python3 -c 'import jsonschema' >/dev/null 2>&1; then
  if python3 - "$SCHEMA" "$INDEX_FILE" "$capabilities" "$missing_freshness" <<'PY'
import json
import sys

import jsonschema

schema = json.load(open(sys.argv[1], encoding="utf-8"))
validator = jsonschema.Draft202012Validator(
    schema,
    format_checker=jsonschema.Draft202012Validator.FORMAT_CHECKER,
)
for line in open(sys.argv[2], encoding="utf-8"):
    validator.validate(json.loads(line))
validator.validate(json.loads(sys.argv[3]))
validator.validate(json.loads(sys.argv[4]))
PY
  then
    pass "all indexed records and provider contracts satisfy the SCOPE-1 schema"
  else
    fail "an indexed record or provider contract violates the SCOPE-1 schema"
  fi
else
  echo "SKIP: generated-record schema validation (python jsonschema not installed)"
fi

if python3 - "$FIXTURE" <<'PY'
import pathlib
import sys

runtime = pathlib.Path(sys.argv[1]) / ".specify/runtime"
actual = sorted(path.relative_to(runtime).as_posix() for path in runtime.rglob("*") if path.is_file())
assert actual == ["experience-recall/index.jsonl", "experience-recall/status.json"], actual
assert not any(path.name.startswith(".index.jsonl.") or path.name.startswith(".status.json.") for path in runtime.rglob("*"))
PY
then
  pass "derived state is contained and atomic writes leave no temporary files"
else
  fail "derived state escaped its runtime directory or left a temporary file"
fi

if ! grep -Eq 'DO_NOT_COPY_RAW_SENTINEL|YAML_RAW_SENTINEL|MARKDOWN_RAW_BODY_SENTINEL|TRANSCRIPT_SENTINEL|ARBITRARY_MARKDOWN_SENTINEL|SOURCE_CODE_SENTINEL|UNDATED_DECISION_SENTINEL' "$INDEX_FILE"; then
  pass "index omits raw bodies, transcripts, arbitrary markdown, and source code"
else
  fail "excluded or raw artifact content leaked into the index"
fi

if python3 - "$INDEX_FILE" "$STATUS_FILE" <<'PY'
import json
import sys

records = [json.loads(line) for line in open(sys.argv[1], encoding="utf-8") if line.strip()]
status = json.load(open(sys.argv[2], encoding="utf-8"))
yaml_records = [
    record for record in records
    if record["sourceAnchor"]["relativePath"] == "evidence/result-markdown.md"
]
full_admission = (
    sorted(record["kind"] for record in yaml_records)
    == ["compacted-result", "finding", "outcome"]
    and any(
        record["kind"] == "finding"
        and record["summary"] == "Canonical YAML findings must remain structured."
        for record in yaml_records
    )
    and status["exclusions"].get("result-envelope-unsupported", 0) == 0
)
honest_unsupported = (
  sorted(record["kind"] for record in yaml_records) in (
    [],
    ["compacted-result", "outcome"],
  )
  and status["exclusions"].get("result-envelope-unsupported") == 1
)
assert full_admission or honest_unsupported, (yaml_records, status["exclusions"])
PY
then
  pass "canonical Markdown/YAML envelope is fully admitted or explicitly unsupported"
else
  fail "canonical Markdown/YAML envelope was partially admitted without its finding or an unsupported-shape count"
fi

current_decision="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text 'CURRENT_IMP_STYLE_DECISION_SENTINEL' --kind owner-decision --limit 5)"
if python3 - "$current_decision" "$sync_output" <<'PY'
import json
import sys

results = json.loads(sys.argv[1])
status = json.loads(sys.argv[2])
admitted = (
    len(results) == 1
    and results[0]["scopeRef"] == "SCOPE-2"
    and results[0]["snippet"].startswith("CURRENT_IMP_STYLE_DECISION_SENTINEL")
)
honest_exclusion = not results and status["exclusions"].get("owner-decision-unsupported") == 2
assert admitted or honest_exclusion, (results, status["exclusions"])
PY
then
  pass "current IMP-037 status grammar is admitted or counted as owner-decision-unsupported"
else
  fail "current IMP-037 status grammar was silently omitted"
fi

if ! grep -Eq 'INCOHERENT_COMMAND_PACKET_SENTINEL|INCOHERENT_GOAL_PACKET_SENTINEL' "$INDEX_FILE"; then
  pass "repository packets require decision-id coherence for command and goal-node scopes"
else
  fail "a regex-valid but incoherent repository packet entered the index"
fi

overlong_query="$(python3 -c 'print("q" * 1001)')"
assert_rejected "empty queries are rejected" "invalid-query" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text ''
assert_rejected "overlong queries are rejected" "invalid-query" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text "$overlong_query"
assert_rejected "query limit zero is rejected" "invalid-query" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text database --limit 0
assert_rejected "query limit twenty-one is rejected" "invalid-query" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text database --limit 21
assert_rejected "unknown query kinds are rejected" "invalid-query" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text database --kind unknown-kind
assert_rejected "unknown query trust classes are rejected" "invalid-query" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text database --trust unknown-trust
assert_rejected "repository alias mismatch is rejected" "index-invalid" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias other --text database

filtered="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture \
  --text 'deterministic lexical ordering' --kind owner-decision --trust owner-approved \
  --scope-ref SCOPE-1 --limit 5)"
assert_json "kind, trust, and scope filters are applied before scoring" "$filtered" \
  'len(data) == 1 and data[0]["kind"] == "owner-decision" and data[0]["sourceTrust"] == "owner-approved" and data[0]["scopeRef"] == "SCOPE-1"'

spec_filtered="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture \
  --text 'bounded fixture' --spec-ref specs/100-fixture --limit 5)"
assert_json "spec filters retain only exact spec references" "$spec_filtered" \
  'len(data) >= 1 and all(item["specRef"] == "specs/100-fixture" for item in data)'

scope_miss="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture \
  --text 'deterministic lexical ordering' --scope-ref SCOPE-404 --limit 5)"
assert_json "scope filters do not fall back to unscoped results" "$scope_miss" \
  'data == []'

default_bound="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text SCOPE-1)"
assert_json "default search results remain bounded to five" "$default_bound" \
  'len(data) <= 5'

relevance="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text 'database timeout' --limit 5)"
assert_json "labeled relevance fixture returns the anchored lesson first" "$relevance" \
  'len(data) >= 1 and data[0]["kind"] == "lesson" and data[0]["sourceAnchor"]["relativePath"] == "evidence/source.md"'

ordering="$(bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text 'deterministic lexical ordering' --kind owner-decision --limit 5)"
assert_json "equal-score results use deterministic record-id ordering" "$ordering" \
  'len(data) == 2 and [item["recordId"] for item in data] == sorted(item["recordId"] for item in data)'

lesson_id="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])[0]["recordId"])' "$relevance")"
read_output="$(bash "$ADAPTER" read --repo-root "$FIXTURE" --repository-alias fixture --record-id "$lesson_id")"
assert_json "read returns the anchored record while its digest is current" "$read_output" \
  'data["recordId"] == "'"$lesson_id"'" and data["recallAuthority"] == "advisory" and data["freshness"]["state"] == "fresh"'

ready_status="$(bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture)"
assert_json "provider status reports exclusions and fresh derived state" "$ready_status" \
  'data["state"] == "ready" and data["excludedCount"] == sum(data["exclusions"].values()) and data["freshness"]["state"] == "fresh" and data["lifecycleCounts"] == {"admitted": data["recordCount"], "deleted": 0, "expired": 0, "superseded": 0}'

freshness="$(bash "$ADAPTER" freshness --repo-root "$FIXTURE" --repository-alias fixture)"
assert_json "provider freshness reports a current aggregate source digest" "$freshness" \
  'data["contractType"] == "freshness" and data["state"] == "fresh" and data["sourceDigest"].startswith("sha256:")'

INDEX_BACKUP="$WORK/index.backup"
STATUS_BACKUP="$WORK/status.backup"
cp "$INDEX_FILE" "$INDEX_BACKUP"
cp "$STATUS_FILE" "$STATUS_BACKUP"

restore_derived_state() {
  cp "$INDEX_BACKUP" "$INDEX_FILE"
  cp "$STATUS_BACKUP" "$STATUS_FILE"
}

mutate_derived_state() {
  local mode="$1"
  python3 - "$INDEX_FILE" "$STATUS_FILE" "$mode" <<'PY'
import hashlib
import json
import sys

index_path, status_path, mode = sys.argv[1:]
if mode in {"status-alias", "status-count"}:
    status = json.load(open(status_path, encoding="utf-8"))
    if mode == "status-alias":
        status["repositoryAlias"] = "tampered"
    else:
        status["recordCount"] += 1
    open(status_path, "w", encoding="utf-8").write(
        json.dumps(status, ensure_ascii=True, separators=(",", ":"), sort_keys=True) + "\n"
    )
    raise SystemExit(0)

payload = open(index_path, "rb").read()
if mode == "digest-mismatch":
    payload += b"\n"
elif mode == "invalid-utf8":
    payload += b"\xff\n"
elif mode == "malformed-jsonl":
    payload += b"{not-json}\n"
elif mode == "duplicate-id":
    lines = payload.decode("utf-8").splitlines()
    lines.insert(1, lines[0])
    payload = ("\n".join(lines) + "\n").encode("utf-8")
elif mode == "out-of-order":
    lines = payload.decode("utf-8").splitlines()
    lines.reverse()
    payload = ("\n".join(lines) + "\n").encode("utf-8")
else:
    raise AssertionError(mode)
open(index_path, "wb").write(payload)
if mode != "digest-mismatch":
    status = json.load(open(status_path, encoding="utf-8"))
    status["indexDigest"] = "sha256:" + hashlib.sha256(payload).hexdigest()
    open(status_path, "w", encoding="utf-8").write(
        json.dumps(status, ensure_ascii=True, separators=(",", ":"), sort_keys=True) + "\n"
    )
PY
}

mutate_derived_state status-alias
assert_rejected "status repository tampering is rejected" "index-invalid" \
  bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state
mutate_derived_state status-count
assert_rejected "status count tampering is rejected against index contents" "index-invalid" \
  bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state
mutate_derived_state digest-mismatch
assert_rejected "index byte tampering is rejected by status digest" "index-invalid" \
  bash "$ADAPTER" freshness --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state
mutate_derived_state malformed-jsonl
assert_rejected "malformed JSONL is rejected after digest verification" "index-invalid" \
  bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state
mutate_derived_state invalid-utf8
assert_rejected "non-UTF-8 JSONL is rejected with a structured index error" "index-invalid" \
  bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state
mutate_derived_state duplicate-id
assert_rejected "duplicate record ids are rejected" "index-invalid" \
  bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state
mutate_derived_state out-of-order
assert_rejected "out-of-order record ids are rejected" "index-invalid" \
  bash "$ADAPTER" status --repo-root "$FIXTURE" --repository-alias fixture
restore_derived_state

mv "$FIXTURE/evidence/source.md" "$WORK/unavailable-source.md"
unknown_freshness="$(bash "$ADAPTER" freshness --repo-root "$FIXTURE" --repository-alias fixture)"
assert_json "unavailable source anchors produce unknown freshness" "$unknown_freshness" \
  'data["state"] == "unknown" and data["sourceDigest"] is None and "unavailable" in data["reason"]'
mv "$WORK/unavailable-source.md" "$FIXTURE/evidence/source.md"

# portable-ok:domain fixture text, not a timeout utility invocation
printf '%s\n' 'changed anchored database timeout evidence' > "$FIXTURE/evidence/source.md"
stale="$(bash "$ADAPTER" freshness --repo-root "$FIXTURE" --repository-alias fixture)"
assert_json "provider freshness detects a changed source digest" "$stale" \
  'data["state"] == "stale" and "digest" in data["reason"]'
set +e
bash "$ADAPTER" read --repo-root "$FIXTURE" --repository-alias fixture --record-id "$lesson_id" \
  >"$WORK/stale-read.out" 2>"$WORK/stale-read.err"
stale_read_rc=$?
set -e
if [[ "$stale_read_rc" -eq 1 ]] && grep -q 'record-stale' "$WORK/stale-read.err"; then
  pass "read refuses a digest-mismatched source anchor"
else
  fail "read did not refuse a digest-mismatched source anchor"
fi
assert_rejected "search refuses stale source anchors" "index-stale" \
  bash "$ADAPTER" search --repo-root "$FIXTURE" --repository-alias fixture --text database

set +e
export_output="$(bash "$ADAPTER" export 2>"$WORK/export.err")"
export_rc=$?
delete_output="$(bash "$ADAPTER" delete 2>"$WORK/delete.err")"
delete_rc=$?
set -e
if [[ "$export_rc" -eq 1 && "$export_output" == '[]' && "$delete_rc" -eq 1 ]] \
  && [[ "$delete_output" == '{"reason":"unsupported","supported":false}' ]]; then
  pass "unsupported export and delete verbs reject with canonical JSON shapes"
else
  fail "unsupported provider verbs did not match their declared capabilities"
fi

if grep -Eq '(^|[^[:alnum:]_])(curl|wget|pip|npm|npx|brew|apt|git[[:space:]]+clone|requests|urllib|socket)([^[:alnum:]_]|$)' \
    "$INDEXER" "$ADAPTER"; then
  fail "provider or indexer contains a disallowed dependency reference"
else
  pass "provider and indexer contain no external fetch or package command references"
fi

mkdir -p "$WRITER_FIXTURE/bubbles/scripts" "$WRITER_FIXTURE/bubbles" "$WRITER_FIXTURE/evidence"
cp "$REPO_ROOT/bubbles/scripts/cli.sh" "$WRITER_FIXTURE/bubbles/scripts/cli.sh"
cp "$REPO_ROOT/bubbles/scripts/fun-mode.sh" "$WRITER_FIXTURE/bubbles/scripts/fun-mode.sh"
cp "$REPO_ROOT/bubbles/scripts/aliases.sh" "$WRITER_FIXTURE/bubbles/scripts/aliases.sh"
cp "$REPO_ROOT/bubbles/scripts/trust-metadata.sh" "$WRITER_FIXTURE/bubbles/scripts/trust-metadata.sh"
cp "$REPO_ROOT/bubbles/action-risk-registry.yaml" "$WRITER_FIXTURE/bubbles/action-risk-registry.yaml"
cp "$REPO_ROOT/bubbles/workflows.yaml" "$WRITER_FIXTURE/bubbles/workflows.yaml"
printf '%s\n' 'writer anchor' > "$WRITER_FIXTURE/evidence/source.md"

legacy_rc=0
bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
  --problem 'legacy writer' --root-cause 'four fields only' \
  --fix 'retain the command shape' --applies-when 'no recall anchor exists' \
  >"$WORK/legacy-writer.out" 2>"$WORK/legacy-writer.err" || legacy_rc=$?
if [[ "$legacy_rc" -eq 0 ]] && python3 - "$WRITER_FIXTURE/.specify/memory/lessons.md" <<'PY'
import json
import re
import sys

line = open(sys.argv[1], encoding="utf-8").read().splitlines()[-1]
assert line.startswith("- problem: legacy writer; root cause: four fields only; fix: retain the command shape; applies when: no recall anchor exists ")
match = re.search(r"<!-- bubbles-lesson-meta:(\{.*\}) -->$", line)
assert match
metadata = json.loads(match.group(1))
assert re.fullmatch(r"lesson-[0-9a-f]{64}", metadata["lessonId"])
assert metadata["reviewState"] == "unanchored"
assert metadata["sourceAnchor"] is None
assert metadata["repositoryAlias"] is None
PY
then
  pass "legacy four-field lesson invocation remains valid and does not invent an anchor"
else
  fail "legacy four-field lesson invocation failed or fabricated metadata"
fi

for run in 1 2; do
  bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
    --problem 'anchored writer' --root-cause 'source proof exists' \
    --fix 'persist validated metadata' --applies-when 'recall admission is requested' \
    --repository-alias writer --source-path evidence/source.md \
    --source-selector '#writer-proof' --review-state reviewed \
    >"$WORK/anchored-writer-$run.out" 2>"$WORK/anchored-writer-$run.err"
done

if python3 - "$WRITER_FIXTURE/.specify/memory/lessons.md" "$WRITER_FIXTURE/evidence/source.md" <<'PY'
import hashlib
import json
import re
import sys

lines = open(sys.argv[1], encoding="utf-8").read().splitlines()[-2:]
metadata = []
for line in lines:
    match = re.search(r"<!-- bubbles-lesson-meta:(\{.*\}) -->$", line)
    assert match
    metadata.append(json.loads(match.group(1)))
expected = "sha256:" + hashlib.sha256(open(sys.argv[2], "rb").read()).hexdigest()
assert metadata[0]["lessonId"] != metadata[1]["lessonId"]
assert all(re.fullmatch(r"lesson-[0-9a-f]{64}", item["lessonId"]) for item in metadata)
assert metadata[0]["repositoryAlias"] == "writer"
assert metadata[0]["reviewState"] == "reviewed"
assert metadata[0]["sourceAnchor"]["relativePath"] == "evidence/source.md"
assert metadata[0]["sourceAnchor"]["selector"] == "#writer-proof"
assert metadata[0]["sourceAnchor"]["contentDigest"] == expected
PY
then
  pass "anchored lesson writer emits stable per-entry identity and current source digest"
else
  fail "anchored lesson writer metadata is unstable or invalid"
fi

writer_sync="$(bash "$ADAPTER" sync --repo-root "$WRITER_FIXTURE" --repository-alias writer)"
assert_json "index admits anchored entries written by the supported lesson command" "$writer_sync" \
  'data["recordCount"] == 2 and data["countsByKind"] == {"lesson": 2} and data["candidateCount"] == 3 and data["exclusions"] == {"unanchored-lesson": 1}'

assert_rejected "lesson fields reject reserved metadata marker injection" "reserved lesson metadata marker" \
  bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
    --problem 'marker <!-- bubbles-lesson-meta:{"injected":true} -->' \
    --root-cause injection --fix reject --applies-when metadata-is-written
assert_rejected "partial lesson source metadata is rejected" "requires --repository-alias" \
  bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
    --problem partial --root-cause missing-fields --fix reject --applies-when metadata-is-written \
    --repository-alias writer
assert_rejected "invalid lesson review state is rejected" "must be anchored or reviewed" \
  bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
    --problem invalid-review --root-cause invalid-state --fix reject --applies-when metadata-is-written \
    --repository-alias writer --source-path evidence/source.md --source-selector '#proof' --review-state pending
ln -s source.md "$WRITER_FIXTURE/evidence/source-link.md"
assert_rejected "lesson source symlinks are rejected" "contained regular file without symlink components" \
  bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
    --problem symlink --root-cause unsafe-source --fix reject --applies-when metadata-is-written \
    --repository-alias writer --source-path evidence/source-link.md --source-selector '#proof' --review-state reviewed
assert_rejected "lessons cannot anchor to the lessons file" "cannot anchor a lesson to the lessons file" \
  bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
    --problem self-anchor --root-cause recursive-source --fix reject --applies-when metadata-is-written \
    --repository-alias writer --source-path .specify/memory/lessons.md --source-selector '#proof' --review-state reviewed

set +e
bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
  --problem 'concurrent writer' --root-cause 'simultaneous append' \
  --fix 'preserve one complete line' --applies-when 'two writers append together' \
  --repository-alias writer --source-path evidence/source.md \
  --source-selector '#concurrent-proof' --review-state reviewed \
  >"$WORK/concurrent-writer-1.out" 2>"$WORK/concurrent-writer-1.err" &
concurrent_pid_one=$!
bash "$WRITER_FIXTURE/bubbles/scripts/cli.sh" lessons add \
  --problem 'concurrent writer' --root-cause 'simultaneous append' \
  --fix 'preserve one complete line' --applies-when 'two writers append together' \
  --repository-alias writer --source-path evidence/source.md \
  --source-selector '#concurrent-proof' --review-state reviewed \
  >"$WORK/concurrent-writer-2.out" 2>"$WORK/concurrent-writer-2.err" &
concurrent_pid_two=$!
wait "$concurrent_pid_one"
concurrent_rc_one=$?
wait "$concurrent_pid_two"
concurrent_rc_two=$?
set -e

concurrent_unique_ids=0
if [[ "$concurrent_rc_one" -eq 0 && "$concurrent_rc_two" -eq 0 ]] \
  && concurrent_unique_ids="$(python3 - "$WRITER_FIXTURE/.specify/memory/lessons.md" <<'PY'
import json
import re
import sys

lines = open(sys.argv[1], encoding="utf-8").read().splitlines()[-2:]
expected = (
    "- problem: concurrent writer; root cause: simultaneous append; "
    "fix: preserve one complete line; applies when: two writers append together"
)
ids = []
for line in lines:
    assert line.startswith(expected + " "), line
    match = re.search(r"<!-- bubbles-lesson-meta:(\{.*\}) -->$", line)
    assert match, line
    metadata = json.loads(match.group(1))
    assert metadata["repositoryAlias"] == "writer"
    assert metadata["reviewState"] == "reviewed"
    assert metadata["sourceAnchor"]["relativePath"] == "evidence/source.md"
    ids.append(metadata["lessonId"])
print(len(set(ids)))
PY
)"; then
  pass "concurrent lesson appends produce two complete metadata-bearing lines"
else
  fail "concurrent lesson appends failed or corrupted a line"
fi

concurrent_sync="$(bash "$ADAPTER" sync --repo-root "$WRITER_FIXTURE" --repository-alias writer)"
assert_json "concurrent lesson identity and exact-duplicate collapse are reflected in index counts" "$concurrent_sync" \
  'data["candidateCount"] == 5 and data["recordCount"] == 2 + int("'"$concurrent_unique_ids"'") and data["countsByKind"] == {"lesson": data["recordCount"]} and data["exclusions"] == {"unanchored-lesson": 1}'

if [[ "$FAIL" -gt 0 ]]; then
  echo "experience-recall-index-selftest: $PASS passed, $FAIL failed"
  exit 1
fi

echo "experience-recall-index-selftest: $PASS passed, $FAIL failed"
