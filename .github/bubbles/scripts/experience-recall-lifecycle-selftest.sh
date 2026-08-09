#!/usr/bin/env bash
# Hermetic adversarial acceptance tests for the experience-recall lifecycle
# ledger, the derived-state projection, and the bounded export surface.
#
# The ledger is the durable authority for derived recall state; the JSONL index
# is only a projection of it. These cases attack that claim directly: they
# rebuild over tombstones, corrupt the ledger, race concurrent mutations,
# aim the export at material outside the repository, and try to walk the closed
# state machine backwards.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORK="$(mktemp -d)"
MAIN="$WORK/main-repo"
GUARD="$WORK/guard-repo"
CONC="$WORK/conc-repo"
LAST_OUT="$WORK/stdout"
LAST_ERR="$WORK/stderr"
LAST_RC=0
PASS=0
FAIL=0

SOURCE_BODY_SENTINEL='SOURCE_BODY_SENTINEL'
TRANSCRIPT_BODY_SENTINEL='TRANSCRIPT_BODY_SENTINEL'

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

report_last() {
  echo "  exit: $LAST_RC"
  echo "  stdout: $(cat "$LAST_OUT")"
  echo "  stderr: $(cat "$LAST_ERR")"
}

# ---------------------------------------------------------------------------
# harness
# ---------------------------------------------------------------------------

run_case() {
  LAST_RC=0
  set +e
  "$@" >"$LAST_OUT" 2>"$LAST_ERR"
  LAST_RC=$?
  set -e
}

recall() {
  local root="$1"
  shift
  run_case bash "$root/bubbles/scripts/experience-recall.sh" "$@"
}

assert_rc() {
  local label="$1"
  local expected="$2"
  if [[ "$LAST_RC" -eq "$expected" ]]; then
    pass "$label"
  else
    fail "$label (expected exit $expected)"
    report_last
  fi
}

assert_json_out() {
  local label="$1"
  local expression="$2"
  if python3 -c 'import json, sys; data = json.load(open(sys.argv[1], encoding="utf-8")); assert eval(sys.argv[2], {"data": data})' \
    "$LAST_OUT" "$expression"; then
    pass "$label"
  else
    fail "$label"
    report_last
  fi
}

assert_stdout_not_contains() {
  local label="$1"
  local unexpected="$2"
  if grep -qF "$unexpected" "$LAST_OUT"; then
    fail "$label"
    report_last
  else
    pass "$label"
  fi
}

assert_no_traceback() {
  local label="$1"
  if grep -qF 'Traceback' "$LAST_ERR" || grep -qF 'Traceback' "$LAST_OUT"; then
    fail "$label"
    report_last
  else
    pass "$label"
  fi
}

# A refusal is a bounded, machine-readable decline: exit 6 plus a refusal
# envelope on stdout carrying the closed refusal code.
assert_refusal() {
  local label="$1"
  local code="$2"
  if [[ "$LAST_RC" -ne 6 ]]; then
    fail "$label (expected refusal exit 6)"
    report_last
    return
  fi
  if grep -qF 'Traceback' "$LAST_ERR"; then
    fail "$label (refusal leaked a traceback)"
    report_last
    return
  fi
  if python3 -c 'import json, sys; data = json.load(open(sys.argv[1], encoding="utf-8")); assert data["contractType"] == "refusal" and data["code"] == sys.argv[2]' \
    "$LAST_OUT" "$code"; then
    pass "$label"
  else
    fail "$label (expected refusal code $code)"
    report_last
  fi
}

# A malformed-derived-state failure is an engine failure, not a refusal: it must
# still be a single structured line, never a Python traceback.
assert_engine_failure() {
  local label="$1"
  local fragment="$2"
  if [[ "$LAST_RC" -eq 1 ]] \
    && grep -qF "$fragment" "$LAST_ERR" \
    && ! grep -qF 'Traceback' "$LAST_ERR"; then
    pass "$label"
  else
    fail "$label (expected clean exit 1 containing '$fragment')"
    report_last
  fi
}

# ---------------------------------------------------------------------------
# fixtures
# ---------------------------------------------------------------------------

stage_framework() {
  local root="$1"
  local framework="$root/bubbles"
  local name=""
  mkdir -p "$framework/scripts" "$framework/adapters/experience-recall" \
    "$root/.github" "$root/.specify/memory" "$root/evidence"
  for name in experience-recall.sh experience-recall-resolve.sh repo-slug.sh; do
    cp "$SCRIPT_DIR/$name" "$framework/scripts/$name"
    chmod +x "$framework/scripts/$name"
  done
  for name in experience-recall-index.py experience-recall-lifecycle.py; do
    cp "$SCRIPT_DIR/$name" "$framework/scripts/$name"
    chmod +x "$framework/scripts/$name"
  done
  for name in local-lexical.sh none.sh; do
    cp "$REPO_ROOT/bubbles/adapters/experience-recall/$name" \
      "$framework/adapters/experience-recall/$name"
    chmod +x "$framework/adapters/experience-recall/$name"
  done
  printf 'experienceRecall:\n  adapter: local-lexical\n' >"$root/.github/bubbles-project.yaml"
}

# Five anchored lessons produce five recall records, which is the minimum that
# lets every lifecycle state hold at least one record simultaneously.
write_lessons() {
  local root="$1"
  local alias="$2"
  python3 - "$root" "$alias" "$SOURCE_BODY_SENTINEL" <<'PY'
import hashlib
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
alias = sys.argv[2]
sentinel = sys.argv[3]
stamp = "2026-08-06T12:00:00Z"

source = root / "evidence/source.md"
source.write_text(f"{sentinel} bounded database retry evidence\n", encoding="utf-8")
digest = "sha256:" + hashlib.sha256(source.read_bytes()).hexdigest()

lines = ["# Lessons", ""]
for offset in range(5):
    metadata = {
        "capturedAt": stamp,
        "lessonId": "lesson-" + str(offset + 1) * 64,
        "repositoryAlias": alias,
        "reviewState": "reviewed",
        "schemaVersion": 1,
        "sourceAnchor": {
            "contentDigest": digest,
            "observedAt": stamp,
            "relativePath": "evidence/source.md",
            "selector": f"#lesson-{offset + 1}",
        },
    }
    lines.append(
        f"- problem: bounded database retry {offset + 1}; "
        f"root cause: unbounded attempt {offset + 1}; "
        f"fix: cap retry {offset + 1}; applies when: database retry"
        + " <!-- bubbles-lesson-meta:"
        + json.dumps(metadata, ensure_ascii=True, sort_keys=True, separators=(",", ":"))
        + " -->"
    )
(root / ".specify/memory/lessons.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}

stage_repo() {
  local root="$1"
  stage_framework "$root"
  write_lessons "$root" "$(basename "$root")"
  bash "$root/bubbles/scripts/experience-recall.sh" sync >/dev/null
}

runtime_dir_of() { printf '%s' "$1/.specify/runtime/experience-recall"; }
ledger_of() { printf '%s' "$(runtime_dir_of "$1")/lifecycle.jsonl"; }

read_ids_into() {
  local root="$1"
  local line=""
  IDS=()
  while IFS= read -r line; do
    IDS+=("$line")
  done < <(python3 -c '
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1]) / ".specify/runtime/experience-recall/index.jsonl"
for line in path.read_text(encoding="utf-8").splitlines():
    print(json.loads(line)["recordId"])
' "$root")
}

if ! command -v python3 >/dev/null 2>&1; then
  echo "experience-recall-lifecycle-selftest: SKIP (python3 not installed)"
  exit 0
fi
if [[ ! -f "$SCRIPT_DIR/experience-recall-lifecycle.py" || ! -f "$SCRIPT_DIR/experience-recall.sh" ]]; then
  echo "experience-recall-lifecycle-selftest: missing lifecycle engine or twin" >&2
  exit 2
fi

echo "experience-recall-lifecycle-selftest"

stage_repo "$MAIN"
read_ids_into "$MAIN"
DELETED_ID="${IDS[0]}"
SUPERSEDED_ID="${IDS[1]}"
EXPIRED_ID="${IDS[2]}"
ADMITTED_ID="${IDS[3]}"

# ---------------------------------------------------------------------------
# case 1 + 2: a tombstone outlives a rebuild and never touches the source bytes
# ---------------------------------------------------------------------------

cp "$MAIN/evidence/source.md" "$WORK/source-before.md"

recall "$MAIN" delete "$DELETED_ID" --reason "tombstone survives rebuild"
assert_rc "delete records a tombstone transition" 0
assert_json_out "delete reports the admitted-to-deleted transition and preserves the source" \
  'data["contractType"] == "lifecycle-transition" and data["previousState"] == "admitted" and data["state"] == "deleted" and data["sourcePreserved"] is True and data["ledgerEntries"] == 1'

recall "$MAIN" sync
assert_rc "sync rebuilds the projection after a deletion" 0

recall "$MAIN" search "database retry" --limit 20
assert_rc "search succeeds after the rebuild" 0
if python3 -c 'import json, sys; data = json.load(open(sys.argv[1], encoding="utf-8")); assert len(data) == 4 and all(item["recordId"] != sys.argv[2] for item in data)' \
  "$LAST_OUT" "$DELETED_ID"; then
  pass "a deleted record does not return to search after a full rebuild"
else
  fail "a deleted record reappeared after the ledger projection was rebuilt"
  report_last
fi

recall "$MAIN" lifecycle list
assert_json_out "rebuilding the projection does not append or drop ledger entries" \
  'data["entryCount"] == 1 and data["effectiveCounts"]["deleted"] == 1'

if cmp -s "$WORK/source-before.md" "$MAIN/evidence/source.md"; then
  pass "the source artifact is byte-identical after deletion and rebuild"
else
  fail "deletion or rebuild rewrote the underlying source artifact"
fi

if grep -qF "$SOURCE_BODY_SENTINEL" "$MAIN/evidence/source.md"; then
  pass "the source artifact still carries its body sentinel after deletion"
else
  fail "the source body sentinel disappeared, so later leak checks would be vacuous"
fi

# ---------------------------------------------------------------------------
# case 3 + 4: non-admitted states leave search but stay visible to status/read
# ---------------------------------------------------------------------------

recall "$MAIN" lifecycle set superseded "$SUPERSEDED_ID" --reason "replaced by a newer lesson"
assert_rc "an admitted record may be superseded" 0
recall "$MAIN" lifecycle set expired "$EXPIRED_ID" --reason "retention window elapsed"
assert_rc "an admitted record may be expired" 0

recall "$MAIN" search "database retry" --limit 20
if python3 -c '
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
hidden = set(sys.argv[2:])
assert len(data) == 2, len(data)
assert all(item["recordId"] not in hidden for item in data)
assert all(item["lifecycle"]["state"] == "admitted" for item in data)
' "$LAST_OUT" "$DELETED_ID" "$SUPERSEDED_ID" "$EXPIRED_ID"; then
  pass "superseded, expired, and deleted records are absent from default search"
else
  fail "default search surfaced a record outside the admitted state"
  report_last
fi

recall "$MAIN" read "$SUPERSEDED_ID"
assert_rc "read reaches a superseded record" 0
assert_json_out "read reports the superseded state and its transition time" \
  'data["lifecycle"]["state"] == "superseded" and data["lifecycle"]["supersededAt"] is not None and data["lifecycle"]["deletedAt"] is None'

recall "$MAIN" read "$EXPIRED_ID"
assert_rc "read reaches an expired record" 0
assert_json_out "read reports the expired state and its transition time" \
  'data["lifecycle"]["state"] == "expired" and data["lifecycle"]["expiredAt"] is not None'

recall "$MAIN" read "$DELETED_ID"
assert_rc "read reaches a deleted record" 0
assert_json_out "read reports the deleted state and its transition time" \
  'data["lifecycle"]["state"] == "deleted" and data["lifecycle"]["deletedAt"] is not None'

recall "$MAIN" status
assert_rc "status succeeds with a mixed lifecycle population" 0
assert_json_out "status counts every lifecycle state and reconciles with the record total" \
  'data["lifecycleCounts"] == {"admitted": 2, "superseded": 1, "expired": 1, "deleted": 1} and sum(data["lifecycleCounts"].values()) == data["recordCount"] == 5'

recall "$MAIN" sync
assert_rc "sync succeeds with a mixed lifecycle population" 0
assert_json_out "a rebuild reprojects every lifecycle state from the ledger" \
  'data["lifecycleCounts"] == {"admitted": 2, "superseded": 1, "expired": 1, "deleted": 1}'

# ---------------------------------------------------------------------------
# case 5: export is bounded and carries no raw source body
# ---------------------------------------------------------------------------

recall "$MAIN" export --limit 2
assert_refusal "export refuses a selection wider than its explicit limit" selection-exceeds-limit

recall "$MAIN" export --limit 20
assert_rc "an export within its limit succeeds" 0
assert_json_out "an unfiltered export returns every record with anchors only" \
  'len(data) == 5 and all(set(item) == {"contractType", "schemaVersion", "recordId", "kind", "summary", "searchableFields", "repositoryAlias", "specRef", "scopeRef", "scenarioRefs", "sourceAnchor", "sourceTrust", "recallAuthority", "freshness", "lifecycle", "provenance"} for item in data)'
assert_stdout_not_contains "an export never carries the raw source body" "$SOURCE_BODY_SENTINEL"

recall "$MAIN" export --limit 2 --record-id "$ADMITTED_ID" --record-id "$SUPERSEDED_ID"
assert_rc "an explicit selection at the limit succeeds" 0
if python3 -c '
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
assert sorted(item["recordId"] for item in data) == sorted(sys.argv[2:])
' "$LAST_OUT" "$ADMITTED_ID" "$SUPERSEDED_ID"; then
  pass "an explicit selection honors the limit and returns only the named records"
else
  fail "an explicit export selection returned the wrong records"
  report_last
fi

recall "$MAIN" export --limit 20 --state deleted
assert_rc "a state-filtered export succeeds" 0
assert_json_out "a state-filtered export returns only that lifecycle state" \
  'len(data) == 1 and data[0]["lifecycle"]["state"] == "deleted"'

mkdir -p "$MAIN/out"
recall "$MAIN" export --limit 20 --output out/export.json
assert_rc "an export to a contained repository-relative path succeeds" 0
if [[ -f "$MAIN/out/export.json" ]] && ! grep -qF "$SOURCE_BODY_SENTINEL" "$MAIN/out/export.json"; then
  pass "a written export file carries no raw source body"
else
  fail "the written export file is missing or leaked the raw source body"
fi

recall "$MAIN" export --limit 0
assert_rc "the twin refuses a zero export limit" 2
recall "$MAIN" export --limit 21
assert_rc "the twin refuses an export limit above the contract bound" 2
recall "$MAIN" export
assert_rc "the twin refuses an export with no explicit limit" 2
recall "$MAIN" export --limit 20 --record-id "recall-$(printf '0%.0s' $(seq 1 64))"
assert_refusal "export refuses a well-formed but unknown record id" record-missing
recall "$MAIN" export --limit 20 --record-id "not-a-record-id"
assert_refusal "export refuses a malformed record id" invalid-selection
recall "$MAIN" export --limit 20 --state archived
assert_refusal "export refuses a lifecycle state outside the closed machine" invalid-selection

# ---------------------------------------------------------------------------
# case 9a: an export destination may not escape the repository
# ---------------------------------------------------------------------------

ln -s "$WORK/escaped-export.json" "$MAIN/out/link.json"
ln -s "$WORK" "$MAIN/out/linkdir"

recall "$MAIN" export --limit 20 --output out/link.json
assert_refusal "export refuses a destination that is a symlink" unsafe-output
recall "$MAIN" export --limit 20 --output out/linkdir/through-link.json
assert_refusal "export refuses a destination that traverses a symlink" unsafe-output
recall "$MAIN" export --limit 20 --output ../escaped-export.json
assert_refusal "export refuses a destination that walks out of the repository" unsafe-output
recall "$MAIN" export --limit 20 --output "$WORK/absolute-export.json"
assert_refusal "export refuses an absolute destination" unsafe-output

if [[ ! -e "$WORK/escaped-export.json" && ! -e "$WORK/through-link.json" && ! -e "$WORK/absolute-export.json" ]]; then
  pass "no refused export destination was written outside the repository"
else
  fail "a refused export still wrote a file outside the repository"
  ls -l "$WORK" || true
fi

# ---------------------------------------------------------------------------
# case 12: the closed state machine only leaves the admitted state
# ---------------------------------------------------------------------------

recall "$MAIN" lifecycle set superseded "$DELETED_ID"
assert_refusal "a deleted record may not be superseded" invalid-transition
recall "$MAIN" lifecycle set expired "$DELETED_ID"
assert_refusal "a deleted record may not be expired" invalid-transition
recall "$MAIN" delete "$DELETED_ID"
assert_refusal "a deleted record may not be deleted again" invalid-transition
recall "$MAIN" lifecycle set deleted "$SUPERSEDED_ID"
assert_refusal "a superseded record may not be deleted" invalid-transition
recall "$MAIN" lifecycle set expired "$SUPERSEDED_ID"
assert_refusal "a superseded record may not be expired" invalid-transition
recall "$MAIN" lifecycle set superseded "$EXPIRED_ID"
assert_refusal "an expired record may not be superseded" invalid-transition
recall "$MAIN" admit "$ADMITTED_ID"
assert_refusal "an admitted record may not be admitted again" invalid-transition
recall "$MAIN" lifecycle set archived "$ADMITTED_ID"
assert_rc "the twin refuses a state outside the closed machine" 2
recall "$MAIN" delete "recall-$(printf '0%.0s' $(seq 1 64))"
assert_refusal "a transition refuses a record that is not in the projection" record-missing

recall "$MAIN" lifecycle list
assert_json_out "refused transitions append nothing to the ledger" \
  'data["entryCount"] == 3'

# ---------------------------------------------------------------------------
# case 7: an explicit admit reverses a deletion
# ---------------------------------------------------------------------------

recall "$MAIN" admit "$DELETED_ID" --reason "re-admitted after review"
assert_rc "a deleted record may be re-admitted" 0
assert_json_out "re-admission reports the deleted-to-admitted transition" \
  'data["previousState"] == "deleted" and data["state"] == "admitted" and data["sourcePreserved"] is True'

recall "$MAIN" search "database retry" --limit 20
if python3 -c 'import json, sys; data = json.load(open(sys.argv[1], encoding="utf-8")); assert len(data) == 3 and any(item["recordId"] == sys.argv[2] for item in data)' \
  "$LAST_OUT" "$DELETED_ID"; then
  pass "a re-admitted record returns to default search"
else
  fail "a re-admitted record did not return to default search"
  report_last
fi

recall "$MAIN" sync
assert_json_out "a rebuild preserves the re-admission" \
  'data["lifecycleCounts"] == {"admitted": 3, "superseded": 1, "expired": 1, "deleted": 0}'

if cmp -s "$WORK/source-before.md" "$MAIN/evidence/source.md"; then
  pass "the source artifact is still byte-identical after the full lifecycle walk"
else
  fail "the lifecycle walk rewrote the underlying source artifact"
fi

# ---------------------------------------------------------------------------
# case 6 + 8 + 9b + 10: derived state is never trusted
# ---------------------------------------------------------------------------

stage_repo "$GUARD"
GUARD_RUNTIME="$(runtime_dir_of "$GUARD")"
GUARD_LEDGER="$(ledger_of "$GUARD")"
read_ids_into "$GUARD"
GUARD_ID="${IDS[0]}"

# The build-time corpus filter already refuses a transcript-family anchor, so
# the only way to prove export carries its own gate is to plant one directly in
# the derived index. Export must not trust the projection it reads.
python3 - "$GUARD" "$TRANSCRIPT_BODY_SENTINEL" <<'PY'
import importlib.util
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1]).resolve()
sentinel = sys.argv[2]
transcript = root / "evidence/session-transcript.md"
transcript.write_text(f"{sentinel} bounded database retry evidence\n", encoding="utf-8")

location = root / "bubbles/scripts/experience-recall-index.py"
spec = importlib.util.spec_from_file_location("bubbles_recall_index_fixture", str(location))
index = importlib.util.module_from_spec(spec)
spec.loader.exec_module(index)

runtime = root / ".specify/runtime/experience-recall"
records = [
    json.loads(line)
    for line in (runtime / "index.jsonl").read_text(encoding="utf-8").splitlines()
]
digest = index.digest_file(transcript)
poisoned = records[0]
poisoned["sourceAnchor"]["relativePath"] = "evidence/session-transcript.md"
poisoned["sourceAnchor"]["contentDigest"] = digest
poisoned["freshness"]["sourceDigest"] = digest

payload = index.serialize_index(records)
status = json.loads((runtime / "status.json").read_text(encoding="utf-8"))
status["indexDigest"] = index.digest_bytes(payload)
status["sourceDigest"] = index.source_digest_for_records(records)
(runtime / "index.jsonl").write_bytes(payload)
(runtime / "status.json").write_text(index.canonical_json(status) + "\n", encoding="utf-8")
PY

recall "$GUARD" export --limit 20
assert_refusal "export refuses a transcript-shaped anchor planted in the index" transcript-like-export
assert_stdout_not_contains "a refused transcript export leaks no transcript body" "$TRANSCRIPT_BODY_SENTINEL"

recall "$GUARD" export --limit 20 --output out-of-corpus.json
assert_refusal "a transcript-shaped selection is refused before any file is written" transcript-like-export
if [[ ! -e "$GUARD/out-of-corpus.json" ]]; then
  pass "a refused transcript export writes no destination file"
else
  fail "a refused transcript export still wrote its destination file"
fi

# The build-time filter is the paired defense; prove it holds too.
python3 - "$GUARD" "$(basename "$GUARD")" <<'PY'
import hashlib
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
alias = sys.argv[2]
transcript = root / "evidence/session-transcript.md"
metadata = {
    "capturedAt": "2026-08-06T12:00:00Z",
    "lessonId": "lesson-" + "9" * 64,
    "repositoryAlias": alias,
    "reviewState": "reviewed",
    "schemaVersion": 1,
    "sourceAnchor": {
        "contentDigest": "sha256:" + hashlib.sha256(transcript.read_bytes()).hexdigest(),
        "observedAt": "2026-08-06T12:00:00Z",
        "relativePath": "evidence/session-transcript.md",
        "selector": "#transcript-anchored-lesson",
    },
}
lessons = root / ".specify/memory/lessons.md"
lessons.write_text(
    lessons.read_text(encoding="utf-8")
    + "- problem: transcript anchored lesson; root cause: excluded input family; "
    + "fix: refuse the anchor; applies when: database retry"
    + " <!-- bubbles-lesson-meta:"
    + json.dumps(metadata, ensure_ascii=True, sort_keys=True, separators=(",", ":"))
    + " -->\n",
    encoding="utf-8",
)
PY

recall "$GUARD" sync
assert_rc "sync succeeds with a transcript-anchored lesson present" 0
assert_json_out "the build-time corpus filter excludes a transcript-anchored lesson" \
  'data["recordCount"] == 5 and data["candidateCount"] == 6 and data["exclusions"]["transcript-like-input"] == 1'

recall "$GUARD" export --limit 20
assert_rc "export recovers once the transcript-family anchor is out of the corpus" 0
assert_stdout_not_contains "the recovered export carries no transcript body" "$TRANSCRIPT_BODY_SENTINEL"

recall "$GUARD" delete "$GUARD_ID" --reason "seed a ledger entry"
assert_rc "the guard fixture records one ledger entry" 0
cp "$GUARD_LEDGER" "$WORK/guard-ledger.bak"

corrupt_ledger() {
  python3 - "$GUARD_LEDGER" "$WORK/guard-ledger.bak" "$1" <<'PY'
import json
import pathlib
import sys

ledger = pathlib.Path(sys.argv[1])
backup = pathlib.Path(sys.argv[2])
mode = sys.argv[3]
payload = backup.read_bytes()

if mode == "truncated":
    ledger.write_bytes(payload.rstrip(b"\n"))
elif mode == "malformed-json":
    ledger.write_bytes(payload + b'{"contractType": "lifecycle-entry"\n')
elif mode == "sequence-gap":
    entry = json.loads(payload.decode("utf-8").splitlines()[0])
    entry["sequence"] = 7
    ledger.write_bytes((json.dumps(entry, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8"))
elif mode == "foreign-repository":
    entry = json.loads(payload.decode("utf-8").splitlines()[0])
    entry["repositoryAlias"] = "some-other-repository"
    ledger.write_bytes((json.dumps(entry, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8"))
elif mode == "blank-entry":
    ledger.write_bytes(b"\n" + payload)
elif mode == "invalid-utf8":
    ledger.write_bytes(b'{"contractType": "lifecycle-entry", "reason": "\xff\xfe"}\n')
elif mode == "restore":
    ledger.write_bytes(payload)
else:
    raise SystemExit(f"unknown corruption mode: {mode}")
PY
}

corrupt_ledger truncated
recall "$GUARD" lifecycle list
assert_engine_failure "a ledger truncated at its final entry is refused" "lifecycle ledger is truncated"
assert_no_traceback "the truncated-ledger refusal emits no traceback"

corrupt_ledger malformed-json
recall "$GUARD" lifecycle list
assert_engine_failure "a malformed ledger entry is refused" "is malformed"
assert_no_traceback "the malformed-ledger refusal emits no traceback"

corrupt_ledger sequence-gap
recall "$GUARD" lifecycle list
assert_engine_failure "a discontinuous ledger sequence is refused" "sequence is not continuous"

corrupt_ledger foreign-repository
recall "$GUARD" lifecycle list
assert_engine_failure "a ledger entry bound to another repository is refused" "belongs to another repository"

corrupt_ledger blank-entry
recall "$GUARD" lifecycle list
assert_engine_failure "a blank ledger entry is refused" "blank entry"

corrupt_ledger invalid-utf8
recall "$GUARD" lifecycle list
assert_engine_failure "a ledger that is not valid UTF-8 is refused" "not valid UTF-8"
assert_no_traceback "the invalid-UTF-8 refusal emits no traceback"

recall "$GUARD" delete "${IDS[1]}"
assert_engine_failure "a transition over a corrupt ledger is refused" "ledger-invalid"
if cmp -s "$WORK/guard-ledger.bak" "$GUARD_LEDGER"; then
  fail "the corrupted ledger was silently repaired instead of refused"
else
  pass "a refused transition never rewrites a corrupt ledger"
fi

corrupt_ledger restore
recall "$GUARD" lifecycle list
assert_json_out "the ledger is readable again once the corruption is reverted" \
  'data["entryCount"] == 1 and data["effectiveCounts"]["deleted"] == 1'

mv "$GUARD_LEDGER" "$WORK/relocated-ledger.jsonl"
ln -s "$WORK/relocated-ledger.jsonl" "$GUARD_LEDGER"
recall "$GUARD" lifecycle list
assert_engine_failure "a ledger symlinked out of the repository is refused" "not a contained regular file"
recall "$GUARD" delete "${IDS[1]}"
assert_engine_failure "a transition against a symlinked ledger is refused" "not a contained regular file"
rm -f "$GUARD_LEDGER"
mv "$WORK/relocated-ledger.jsonl" "$GUARD_LEDGER"

mv "$GUARD_RUNTIME" "$WORK/relocated-runtime"
ln -s "$WORK/relocated-runtime" "$GUARD_RUNTIME"
recall "$GUARD" lifecycle list
assert_engine_failure "derived state symlinked out of the repository is refused" "not a contained directory"
rm -f "$GUARD_RUNTIME"
mv "$WORK/relocated-runtime" "$GUARD_RUNTIME"

recall "$GUARD" lifecycle list
assert_json_out "the ledger survives every containment refusal intact" \
  'data["entryCount"] == 1'

# ---------------------------------------------------------------------------
# case 11: concurrent mutations lose no update and corrupt no ledger
# ---------------------------------------------------------------------------

stage_repo "$CONC"
read_ids_into "$CONC"
CONC_IDS=("${IDS[@]}")

for id in "${CONC_IDS[@]}"; do
  bash "$CONC/bubbles/scripts/experience-recall.sh" delete "$id" --reason "concurrent tombstone" \
    >"$WORK/conc-delete-$id.out" 2>&1 &
done
wait

recall "$CONC" lifecycle list
assert_json_out "five concurrent deletions append five continuous, distinct ledger entries" \
  'data["entryCount"] == 5 and [entry["sequence"] for entry in data["entries"]] == [1, 2, 3, 4, 5] and len({entry["recordId"] for entry in data["entries"]}) == 5 and data["effectiveCounts"] == {"admitted": 0, "superseded": 0, "expired": 0, "deleted": 5}'

for id in "${CONC_IDS[@]}"; do
  bash "$CONC/bubbles/scripts/experience-recall.sh" admit "$id" --reason "concurrent re-admission" \
    >"$WORK/conc-admit-$id.out" 2>&1 &
done
wait

recall "$CONC" lifecycle list
assert_json_out "five concurrent re-admissions extend the ledger without a gap" \
  'data["entryCount"] == 10 and [entry["sequence"] for entry in data["entries"]] == list(range(1, 11)) and data["effectiveCounts"] == {"admitted": 5, "superseded": 0, "expired": 0, "deleted": 0}'

CONTENDED_ID="${CONC_IDS[0]}"
for attempt in 1 2 3 4 5 6; do
  (
    set +e
    bash "$CONC/bubbles/scripts/experience-recall.sh" delete "$CONTENDED_ID" --reason "contended tombstone" \
      >"$WORK/conc-race-$attempt.out" 2>"$WORK/conc-race-$attempt.err"
    echo "$?" >"$WORK/conc-race-$attempt.rc"
  ) &
done
wait

RACE_WON=0
RACE_REFUSED=0
for attempt in 1 2 3 4 5 6; do
  case "$(cat "$WORK/conc-race-$attempt.rc")" in
    0) RACE_WON=$((RACE_WON + 1)) ;;
    6) RACE_REFUSED=$((RACE_REFUSED + 1)) ;;
    *) : ;;
  esac
done
if [[ "$RACE_WON" -eq 1 && "$RACE_REFUSED" -eq 5 ]]; then
  pass "exactly one contender wins a same-record race and the rest are refused"
else
  fail "a same-record race produced $RACE_WON winners and $RACE_REFUSED refusals"
  cat "$WORK"/conc-race-*.err || true
fi

recall "$CONC" lifecycle list
assert_json_out "a same-record race appends exactly one ledger entry" \
  'data["entryCount"] == 11 and [entry["sequence"] for entry in data["entries"]] == list(range(1, 12)) and data["effectiveCounts"] == {"admitted": 4, "superseded": 0, "expired": 0, "deleted": 1}'

recall "$CONC" status
assert_rc "the projection is self-consistent after concurrent mutation" 0
assert_json_out "concurrent mutation leaves the projection reconciled with the ledger" \
  'data["lifecycleCounts"] == {"admitted": 4, "superseded": 0, "expired": 0, "deleted": 1} and data["recordCount"] == 5'

recall "$CONC" sync
assert_rc "a rebuild after concurrent mutation succeeds" 0
assert_json_out "a rebuild reproduces the raced lifecycle state exactly" \
  'data["lifecycleCounts"] == {"admitted": 4, "superseded": 0, "expired": 0, "deleted": 1}'

if python3 -c '
import pathlib
import sys

runtime = pathlib.Path(sys.argv[1]) / ".specify/runtime/experience-recall"
names = sorted(path.name for path in runtime.iterdir() if path.is_file())
assert names == ["index.jsonl", "lifecycle.jsonl", "lifecycle.lock", "status.json"], names
' "$CONC"; then
  pass "concurrent mutation leaves no temporary derived files behind"
else
  fail "concurrent mutation left temporary or unexpected files in derived state"
  ls -la "$(runtime_dir_of "$CONC")" || true
fi

# ---------------------------------------------------------------------------

if [[ "$FAIL" -gt 0 ]]; then
  echo "experience-recall-lifecycle-selftest: $PASS passed, $FAIL failed"
  exit 1
fi

echo "experience-recall-lifecycle-selftest: $PASS passed, $FAIL failed"
