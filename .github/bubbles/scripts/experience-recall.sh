#!/usr/bin/env bash
# Repository-rooted bash twin for evidence-backed experience recall.

set -euo pipefail

resolve_script_file() {
  local source="$1"
  local directory=""
  local target=""
  local hops=0

  case "$source" in
    /*) ;;
    *) source="$(pwd -P)/$source" ;;
  esac

  while :; do
    directory="$(cd -P -- "$(dirname "$source")" 2>/dev/null && pwd -P)" || return 1
    source="$directory/$(basename "$source")"
    if [[ ! -L "$source" ]]; then
      [[ -f "$source" ]] || return 1
      printf '%s\n' "$source"
      return 0
    fi
    hops=$((hops + 1))
    [[ "$hops" -le 40 ]] || return 1
    target="$(readlink "$source" 2>/dev/null)" || return 1
    [[ -n "$target" ]] || return 1
    case "$target" in
      /*) source="$target" ;;
      *) source="$directory/$target" ;;
    esac
  done
}

SCRIPT_FILE="$(resolve_script_file "${BASH_SOURCE[0]}")" || {
  echo "experience-recall: cannot resolve the physical script file" >&2
  exit 1
}
SCRIPT_DIR="$(cd -P -- "$(dirname "$SCRIPT_FILE")" && pwd -P)"
FRAMEWORK_ROOT="$(cd -P -- "$SCRIPT_DIR/.." && pwd -P)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" \
  && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd -P -- "$SCRIPT_DIR/../../.." && pwd -P)"
else
  REPO_ROOT="$(cd -P -- "$SCRIPT_DIR/../.." && pwd -P)"
fi

# shellcheck source=bubbles/scripts/repo-slug.sh
source "$SCRIPT_DIR/repo-slug.sh"

ADAPTER=""
ADAPTER_PATH=""
REPOSITORY_ALIAS=""
PROVIDER_OUTPUT=""
PROVIDER_RC=0
FORMAT="json"
FORMAT_SEEN=0
TRANSITION_REASON=""
TRANSITION_REASON_SEEN=0

usage() {
  cat <<'EOF'
Usage: experience-recall.sh <subcommand> [args...]

Repository-rooted subcommands:
  search <query> [--limit N] [--kind KIND ...] [--trust TRUST ...]
                 [--spec-ref REF] [--scope-ref REF] [--format json|text]
  read <record-id> [--format json|text]
  status [--format json|text]
  freshness [--format json|text]
  sync [--format json|text]
  delete <record-id> [--reason TEXT] [--format json|text]
  admit <record-id> [--reason TEXT] [--format json|text]
  lifecycle list [--format json|text]
  lifecycle set <state> <record-id> [--reason TEXT] [--format json|text]
  export --limit N [--record-id ID ...] [--kind KIND ...] [--state STATE ...]
         [--output REPO-RELATIVE-PATH] [--format json|text]

The repository root and repository alias are derived from this installed twin.
Public --repo-root, --repository-alias, and --adapter overrides are refused.

Lifecycle states form the closed machine
admitted -> superseded | expired | deleted. Only an explicit admit reverses a
transition. Deletion changes derived recall state and the lifecycle ledger only;
it never deletes or rewrites the source artifact.

Export requires an explicit --limit. It emits normalized records and source
anchors, never raw source bodies and never transcript data.

Exit codes:
  0 success (freshness: fresh)
  1 provider, engine, or malformed-response failure
  2 usage error
  3 freshness: stale
  4 freshness: unknown
  5 experience recall adapter is disabled
  6 lifecycle or export refusal
EOF
}

sanitize_text_value() {
  python3 -c '
import sys
import unicodedata

value = sys.argv[1]
safe = "".join(
    "?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character
    for character in value
)
print(" ".join(safe.split()), end="")
' "$1"
}

fail_usage() {
  local message=""
  message="$(sanitize_text_value "$1")" || message="invalid usage"
  printf 'experience-recall: %s\n' "$message" >&2
  exit 2
}

set_format() {
  [[ "$FORMAT_SEEN" -eq 0 ]] || fail_usage "--format may be specified once"
  FORMAT_SEEN=1
  case "$1" in
    json | text) FORMAT="$1" ;;
    *) fail_usage "--format must be json or text" ;;
  esac
}

validate_public_value() {
  local label="$1"
  local value="$2"
  if python3 -c '
import sys
import unicodedata

value = sys.argv[1]
unsafe = any(
    unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"}
    for character in value
)
raise SystemExit(1 if unsafe else 0)
' "$value"; then
    return 0
  fi
  printf 'experience-recall: %s contains unsafe control characters\n' "$label" >&2
  return 1
}

reject_derived_control() {
  case "$1" in
    --repo-root | --repo-root=*) fail_usage "repository root is derived; --repo-root is not accepted" ;;
    --repository-alias | --repository-alias=*) fail_usage "repository alias is derived; --repository-alias is not accepted" ;;
    --adapter | --adapter=*) fail_usage "adapter selection comes from repository config; --adapter is not accepted" ;;
  esac
}

resolve_provider() {
  local resolution=""
  local resolution_rc=0
  local line=""
  local key=""
  local value=""
  local resolved_root=""
  local expected_path=""

  set +e
  resolution="$(bash "$SCRIPT_DIR/experience-recall-resolve.sh" --repo-root "$REPO_ROOT")"
  resolution_rc=$?
  set -e
  [[ "$resolution_rc" -eq 0 ]] || exit "$resolution_rc"

  while IFS= read -r line; do
    key="${line%%=*}"
    value="${line#*=}"
    case "$key" in
      adapter) ADAPTER="$value" ;;
      adapterPath) ADAPTER_PATH="$value" ;;
      repoRoot) resolved_root="$value" ;;
    esac
  done <<< "$resolution"

  [[ -n "$ADAPTER" && -n "$ADAPTER_PATH" && "$resolved_root" == "$REPO_ROOT" ]] || {
    echo "experience-recall: resolver returned an incomplete or mismatched result" >&2
    exit 1
  }
  expected_path="$FRAMEWORK_ROOT/adapters/experience-recall/$ADAPTER.sh"
  [[ "$ADAPTER_PATH" == "$expected_path" && -f "$ADAPTER_PATH" ]] || {
    echo "experience-recall: resolver returned a non-canonical adapter path" >&2
    exit 1
  }
  REPOSITORY_ALIAS="$(bubbles_repository_alias_from_root "$REPO_ROOT")" || {
    echo "experience-recall: repository basename cannot form a safe alias" >&2
    exit 1
  }
}

run_provider() {
  PROVIDER_RC=0
  set +e
  PROVIDER_OUTPUT="$(bash "$ADAPTER_PATH" "$@")"
  PROVIDER_RC=$?
  set -e
}

run_engine() {
  local engine="$SCRIPT_DIR/experience-recall-lifecycle.py"
  command -v python3 >/dev/null 2>&1 || {
    echo "experience-recall: python3 is required for lifecycle and export operations" >&2
    exit 1
  }
  [[ -f "$engine" ]] || {
    echo "experience-recall: lifecycle engine not found: $engine" >&2
    exit 1
  }
  PROVIDER_RC=0
  set +e
  PROVIDER_OUTPUT="$(python3 "$engine" "$@" --repo-root "$REPO_ROOT" --repository-alias "$REPOSITORY_ALIAS")"
  PROVIDER_RC=$?
  set -e
}

propagate_provider_failure() {
  if [[ "$PROVIDER_RC" -ne 0 ]]; then
    [[ -z "$PROVIDER_OUTPUT" ]] || printf '%s\n' "$PROVIDER_OUTPUT"
    return "$PROVIDER_RC"
  fi
  return 0
}

  validate_provider_response() {
    local operation="$1"
    python3 -c '
if True:
  import json
  import math
  import re
  import sys

  operation = sys.argv[1]
  raw = sys.argv[2]

  def invalid(code):
    print(f"experience-recall: invalid provider response operation={operation} code={code}", file=sys.stderr)
    raise SystemExit(1)

  try:
    if len(raw.encode("utf-8")) > 1_048_576:
      invalid("response-too-large")
  except UnicodeError:
    invalid("malformed-json")

  def reject_constant(_value):
    raise ValueError("non-finite-number")

  def unique_object(pairs):
    value = {}
    for key, item in pairs:
      if key in value:
        raise ValueError("duplicate-key")
      value[key] = item
    return value

  try:
    payload = json.loads(
      raw,
      object_pairs_hook=unique_object,
      parse_constant=reject_constant,
    )
  except (json.JSONDecodeError, UnicodeError, ValueError, RecursionError):
    invalid("malformed-json")

  KINDS = {"compacted-result", "lesson", "owner-decision", "finding", "outcome"}
  TRUST = {
    "executed-result", "historical-result", "reviewed-lesson", "anchored-lesson",
    "owner-approved", "historical-finding", "historical-outcome",
  }
  FRESHNESS_STATES = {"fresh", "stale", "unknown", "disabled"}
  LIFECYCLE_STATES = {"admitted", "superseded", "expired", "deleted"}
  DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
  ALIAS = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")

  def text(value, minimum=1, maximum=1024):
    return isinstance(value, str) and minimum <= len(value) <= maximum

  def nullable_text(value, maximum=512):
    return value is None or text(value, 1, maximum)

  def nonnegative_integer(value):
    return type(value) is int and value >= 0

  def finite_number(value):
    return type(value) in {int, float} and math.isfinite(value) and value >= 0

  def exact_keys(value, required, allowed=None):
    if not isinstance(value, dict) or not required.issubset(value):
      return False
    return allowed is None or set(value).issubset(allowed)

  def valid_freshness(value):
    required = {"contractType", "state", "sourceDigest", "checkedAt"}
    allowed = required | {"reason"}
    if not exact_keys(value, required, allowed):
      return False
    state = value["state"]
    digest = value["sourceDigest"]
    if value["contractType"] != "freshness" or state not in FRESHNESS_STATES:
      return False
    if not text(value["checkedAt"], 1, 64) or not nullable_text(value.get("reason"), 512):
      return False
    if digest is not None and (not isinstance(digest, str) or DIGEST.fullmatch(digest) is None):
      return False
    return state not in {"fresh", "stale"} or isinstance(digest, str)

  def valid_lifecycle(value):
    keys = {"contractType", "state", "admittedAt", "supersededAt", "expiredAt", "deletedAt"}
    if not exact_keys(value, keys, keys):
      return False
    if value["contractType"] != "lifecycle" or value["state"] not in LIFECYCLE_STATES:
      return False
    return text(value["admittedAt"], 1, 64) and all(
      nullable_text(value[key], 64) for key in ("supersededAt", "expiredAt", "deletedAt")
    )

  def valid_anchor(value):
    keys = {"relativePath", "selector", "contentDigest", "observedAt"}
    return (
      exact_keys(value, keys, keys)
      and text(value["relativePath"], 1, 1024)
      and not value["relativePath"].startswith("/")
      and ".." not in value["relativePath"].split("/")
      and text(value["selector"], 1, 512)
      and isinstance(value["contentDigest"], str)
      and DIGEST.fullmatch(value["contentDigest"]) is not None
      and text(value["observedAt"], 1, 64)
    )

  def valid_provenance(value):
    keys = {"extractor", "extractorVersion", "provider", "providerVersion", "derivedAt"}
    return exact_keys(value, keys, keys) and all(text(value[key], 1, 128) for key in keys)

  def valid_metadata(value):
    scenarios = value.get("scenarioRefs")
    return (
      text(value.get("recordId"), 1, 128)
      and value.get("kind") in KINDS
      and isinstance(value.get("repositoryAlias"), str)
      and ALIAS.fullmatch(value["repositoryAlias"]) is not None
      and nullable_text(value.get("specRef"), 512)
      and nullable_text(value.get("scopeRef"), 512)
      and isinstance(scenarios, list)
      and len(scenarios) <= 100
      and len(scenarios) == len(set(scenarios))
      and all(text(item, 1, 128) for item in scenarios)
      and valid_anchor(value.get("sourceAnchor"))
      and value.get("sourceTrust") in TRUST
      and value.get("recallAuthority") == "advisory"
      and valid_freshness(value.get("freshness"))
      and valid_lifecycle(value.get("lifecycle"))
      and valid_provenance(value.get("provenance"))
    )

  def valid_score(value):
    keys = {"exactIdentifier", "exactPhrase", "tokenOverlap", "tagOverlap", "total"}
    return exact_keys(value, keys, keys) and all(finite_number(value[key]) for key in keys)

  def valid_searchable_fields(value):
    keys = {"identifiers", "phrases", "tags"}
    if not exact_keys(value, keys, keys):
      return False
    return all(
      isinstance(value[key], list)
      and len(value[key]) <= 100
      and len(value[key]) == len(set(value[key]))
      and all(text(item, 1, 512) for item in value[key])
      for key in keys
    )

  METADATA_KEYS = {
    "recordId", "kind", "repositoryAlias", "specRef", "scopeRef", "scenarioRefs",
    "sourceAnchor", "sourceTrust", "recallAuthority", "freshness", "lifecycle", "provenance",
  }
  RESULT_KEYS = METADATA_KEYS | {"contractType", "schemaVersion", "snippet", "score"}
  RECORD_KEYS = METADATA_KEYS | {"contractType", "schemaVersion", "summary", "searchableFields"}

  def valid_result(value):
    return (
      exact_keys(value, RESULT_KEYS, RESULT_KEYS)
      and value["contractType"] == "result"
      and value["schemaVersion"] == 1
      and text(value["snippet"], 1, 1024)
      and valid_metadata(value)
      and valid_score(value["score"])
    )

  def valid_record(value):
    return (
      exact_keys(value, RECORD_KEYS, RECORD_KEYS)
      and value["contractType"] == "record"
      and value["schemaVersion"] == 1
      and text(value["summary"], 1, 2000)
      and valid_searchable_fields(value["searchableFields"])
      and valid_metadata(value)
    )

  def valid_count_map(value):
    return isinstance(value, dict) and len(value) <= 100 and all(
      text(key, 1, 128) and nonnegative_integer(item) for key, item in value.items()
    )

  def valid_status(value, require_freshness, require_synced):
    required = {
      "contractType", "schemaVersion", "adapter", "providerVersion", "repositoryAlias",
      "state", "indexPath", "recordCount", "candidateCount", "countsByKind",
      "lifecycleCounts", "excludedCount", "exclusions",
    }
    if not exact_keys(value, required):
      return False
    lifecycle = value["lifecycleCounts"]
    if not (
      value["contractType"] == "status"
      and value["schemaVersion"] == 1
      and text(value["adapter"], 1, 64)
      and text(value["providerVersion"], 1, 64)
      and isinstance(value["repositoryAlias"], str)
      and ALIAS.fullmatch(value["repositoryAlias"]) is not None
      and text(value["state"], 1, 64)
      and nullable_text(value["indexPath"], 1024)
      and all(nonnegative_integer(value[key]) for key in ("recordCount", "candidateCount", "excludedCount"))
      and valid_count_map(value["countsByKind"])
      and isinstance(lifecycle, dict)
      and set(lifecycle) == LIFECYCLE_STATES
      and all(nonnegative_integer(lifecycle[key]) for key in LIFECYCLE_STATES)
      and valid_count_map(value["exclusions"])
    ):
      return False
    if require_freshness and not valid_freshness(value.get("freshness")):
      return False
    return not require_synced or value.get("synced") is True

  def valid_lifecycle_transition(value):
    keys = {
      "contractType", "schemaVersion", "adapter", "repositoryAlias", "recordId",
      "previousState", "state", "transitionedAt", "reason", "sourceAnchor",
      "sourcePreserved", "ledgerPath", "ledgerEntries", "lifecycleCounts",
    }
    if not exact_keys(value, keys, keys):
      return False
    counts = value["lifecycleCounts"]
    return (
      value["contractType"] == "lifecycle-transition"
      and value["schemaVersion"] == 1
      and text(value["adapter"], 1, 64)
      and isinstance(value["repositoryAlias"], str)
      and ALIAS.fullmatch(value["repositoryAlias"]) is not None
      and text(value["recordId"], 1, 128)
      and value["previousState"] in LIFECYCLE_STATES
      and value["state"] in LIFECYCLE_STATES
      and text(value["transitionedAt"], 1, 64)
      and nullable_text(value["reason"], 512)
      and valid_anchor(value["sourceAnchor"])
      # A transition that did not preserve its source artifact is refused.
      and value["sourcePreserved"] in (True, None)
      and text(value["ledgerPath"], 1, 1024)
      and nonnegative_integer(value["ledgerEntries"])
      and isinstance(counts, dict)
      and set(counts) == LIFECYCLE_STATES
      and all(nonnegative_integer(counts[key]) for key in LIFECYCLE_STATES)
    )

  def valid_ledger_entry(value):
    keys = {
      "contractType", "schemaVersion", "sequence", "repositoryAlias", "recordId",
      "anchorKey", "sourceAnchor", "state", "transitionedAt", "reason",
    }
    if not exact_keys(value, keys, keys):
      return False
    return (
      value["contractType"] == "lifecycle-entry"
      and value["schemaVersion"] == 1
      and type(value["sequence"]) is int
      and value["sequence"] >= 1
      and isinstance(value["repositoryAlias"], str)
      and ALIAS.fullmatch(value["repositoryAlias"]) is not None
      and text(value["recordId"], 1, 128)
      and isinstance(value["anchorKey"], str)
      and DIGEST.fullmatch(value["anchorKey"]) is not None
      and valid_anchor(value["sourceAnchor"])
      and value["state"] in LIFECYCLE_STATES
      and text(value["transitionedAt"], 1, 64)
      and nullable_text(value["reason"], 512)
    )

  def valid_ledger(value):
    keys = {
      "contractType", "schemaVersion", "adapter", "repositoryAlias", "ledgerPath",
      "entryCount", "effectiveCounts", "entries",
    }
    if not exact_keys(value, keys, keys):
      return False
    counts = value["effectiveCounts"]
    entries = value["entries"]
    return (
      value["contractType"] == "lifecycle-ledger"
      and value["schemaVersion"] == 1
      and text(value["adapter"], 1, 64)
      and isinstance(value["repositoryAlias"], str)
      and ALIAS.fullmatch(value["repositoryAlias"]) is not None
      and text(value["ledgerPath"], 1, 1024)
      and nonnegative_integer(value["entryCount"])
      and isinstance(counts, dict)
      and set(counts) == LIFECYCLE_STATES
      and all(nonnegative_integer(counts[key]) for key in LIFECYCLE_STATES)
      and isinstance(entries, list)
      and value["entryCount"] == len(entries)
      and all(valid_ledger_entry(item) for item in entries)
    )

  valid = False
  if operation == "search":
    valid = isinstance(payload, list) and len(payload) <= 20 and all(valid_result(item) for item in payload)
  elif operation == "read":
    valid = valid_record(payload)
  elif operation == "status":
    valid = valid_status(payload, require_freshness=True, require_synced=False)
  elif operation == "freshness":
    valid = valid_freshness(payload)
  elif operation == "sync":
    valid = valid_status(payload, require_freshness=False, require_synced=True)
  elif operation == "lifecycle":
    valid = valid_lifecycle_transition(payload)
  elif operation == "ledger":
    valid = valid_ledger(payload)
  elif operation == "export":
    valid = isinstance(payload, list) and len(payload) <= 20 and all(valid_record(item) for item in payload)
  else:
    invalid("unknown-operation")

  if not valid:
    invalid("wrong-shape")
  ' "$operation" "$PROVIDER_OUTPUT"
  }

disabled_status_json() {
  printf '{"adapter":"none","candidateCount":0,"contractType":"status","countsByKind":{},"excludedCount":0,"exclusions":{},"freshness":{"reason":"experience recall adapter is disabled","state":"disabled"},"indexPath":null,"lifecycleCounts":{"admitted":0,"deleted":0,"expired":0,"superseded":0},"providerVersion":"1","recordCount":0,"repositoryAlias":"%s","schemaVersion":1,"state":"disabled"}\n' "$REPOSITORY_ALIAS"
}

disabled_freshness_json() {
  local checked_at=""
  checked_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf '{"checkedAt":"%s","contractType":"freshness","reason":"experience recall adapter is disabled","sourceDigest":null,"state":"disabled"}\n' "$checked_at"
}

disabled_refusal_json() {
  local operation="$1"
  printf '{"adapter":"none","contractType":"refusal","operation":"%s","reason":"experience recall adapter is disabled","state":"disabled"}\n' "$operation"
}

render_search_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
  safe = "".join(
    "?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character
    for character in str(value)
  )
  return " ".join(safe.split())

results = json.loads(sys.argv[1])
if not isinstance(results, list):
    raise SystemExit("experience-recall: provider search response is not an array")
if not results:
    print("No recall matches.")
for item in results:
    score = item.get("score", {}).get("total", 0)
    source = item.get("sourceAnchor", {}).get("relativePath", "unknown")
    print(
        "record={record} kind={kind} trust={trust} score={score:g} source={source} snippet={snippet}".format(
            record=one_line(item.get("recordId", "")),
            kind=one_line(item.get("kind", "")),
            trust=one_line(item.get("sourceTrust", "")),
            score=float(score),
            source=one_line(source),
            snippet=one_line(item.get("snippet", "")),
        )
    )
' "$PROVIDER_OUTPUT"
}

render_read_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

record = json.loads(sys.argv[1])
if not isinstance(record, dict) or not record.get("recordId"):
    raise SystemExit("experience-recall: provider read response is not a record")
anchor = record.get("sourceAnchor", {})
print("record=" + one_line(record.get("recordId")))
print(
    "kind={kind} trust={trust} authority={authority}".format(
        kind=one_line(record.get("kind")),
        trust=one_line(record.get("sourceTrust")),
        authority=one_line(record.get("recallAuthority")),
    )
)
print(
    "freshness={freshness} lifecycle={lifecycle}".format(
        freshness=one_line(record.get("freshness", {}).get("state")),
        lifecycle=one_line(record.get("lifecycle", {}).get("state")),
    )
)
print(
    "source={path} selector={selector}".format(
        path=one_line(anchor.get("relativePath")),
        selector=one_line(anchor.get("selector")),
    )
)
print("summary=" + one_line(record.get("summary")))
' "$PROVIDER_OUTPUT"
}

render_status_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

status = json.loads(sys.argv[1])
adapter = sys.argv[2]
if not isinstance(status, dict):
    raise SystemExit("experience-recall: provider status response is not an object")
record_count = int(status.get("recordCount", 0))
lifecycle = status.get("lifecycleCounts")
if not isinstance(lifecycle, dict):
    lifecycle = {"admitted": record_count, "superseded": 0, "expired": 0, "deleted": 0}
freshness = status.get("freshness", {})
print(
    "adapter={adapter} state={state} repository={repository}".format(
        adapter=one_line(status.get("adapter", adapter)),
        state=one_line(status.get("state")),
        repository=one_line(status.get("repositoryAlias")),
    )
)
print(
    "index={index} records={records} candidates={candidates} excluded={excluded}".format(
        index=one_line(status.get("indexPath")),
        records=record_count,
        candidates=int(status.get("candidateCount", 0)),
        excluded=int(status.get("excludedCount", 0)),
    )
)
print(
    "lifecycle admitted={admitted} superseded={superseded} expired={expired} deleted={deleted}".format(
        admitted=int(lifecycle.get("admitted", 0)),
        superseded=int(lifecycle.get("superseded", 0)),
        expired=int(lifecycle.get("expired", 0)),
        deleted=int(lifecycle.get("deleted", 0)),
    )
)
exclusions = status.get("exclusions", {})
if exclusions:
  print("exclusions " + " ".join(f"{one_line(key)}={one_line(exclusions[key])}" for key in sorted(exclusions)))
else:
    print("exclusions none")
print(
    "freshness={state} reason={reason}".format(
        state=one_line(freshness.get("state", "unknown")),
        reason=one_line(freshness.get("reason")),
    )
)
' "$PROVIDER_OUTPUT" "$ADAPTER"
}

render_freshness_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

freshness = json.loads(sys.argv[1])
if not isinstance(freshness, dict):
    raise SystemExit("experience-recall: provider freshness response is not an object")
print(
    "adapter={adapter} freshness={state} reason={reason}".format(
        adapter=one_line(sys.argv[2]),
        state=one_line(freshness.get("state")),
        reason=one_line(freshness.get("reason")),
    )
)
' "$PROVIDER_OUTPUT" "$ADAPTER"
}

render_sync_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

status = json.loads(sys.argv[1])
if not isinstance(status, dict):
    raise SystemExit("experience-recall: provider sync response is not an object")
print(
    "adapter={adapter} synced={synced} repository={repository} index={index} records={records} candidates={candidates} excluded={excluded}".format(
        adapter=one_line(status.get("adapter", sys.argv[2])),
        synced=str(bool(status.get("synced", False))).lower(),
        repository=one_line(status.get("repositoryAlias")),
        index=one_line(status.get("indexPath")),
        records=int(status.get("recordCount", 0)),
        candidates=int(status.get("candidateCount", 0)),
        excluded=int(status.get("excludedCount", 0)),
    )
)
' "$PROVIDER_OUTPUT" "$ADAPTER"
}

freshness_exit() {
  local state=""
  state="$(python3 -c 'import json,sys; value=json.loads(sys.argv[1]); print(value.get("state", "")) if isinstance(value, dict) else sys.exit(1)' "$PROVIDER_OUTPUT")" || return 1
  case "$state" in
    fresh) return 0 ;;
    stale) return 3 ;;
    unknown) return 4 ;;
    disabled) return 5 ;;
    *)
      echo "experience-recall: provider returned unknown freshness state '$state'" >&2
      return 1
      ;;
  esac
}

render_lifecycle_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    if value is True:
        return "true"
    if value is False:
        return "false"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

transition = json.loads(sys.argv[1])
if not isinstance(transition, dict):
    raise SystemExit("experience-recall: lifecycle response is not an object")
counts = transition.get("lifecycleCounts", {})
anchor = transition.get("sourceAnchor", {})
print(
    "record={record} previous={previous} state={state} at={at}".format(
        record=one_line(transition.get("recordId")),
        previous=one_line(transition.get("previousState")),
        state=one_line(transition.get("state")),
        at=one_line(transition.get("transitionedAt")),
    )
)
print(
    "source={path} selector={selector} sourcePreserved={preserved}".format(
        path=one_line(anchor.get("relativePath")),
        selector=one_line(anchor.get("selector")),
        preserved=one_line(transition.get("sourcePreserved")),
    )
)
print(
    "ledger={ledger} entries={entries} reason={reason}".format(
        ledger=one_line(transition.get("ledgerPath")),
        entries=int(transition.get("ledgerEntries", 0)),
        reason=one_line(transition.get("reason")),
    )
)
print(
    "lifecycle admitted={admitted} superseded={superseded} expired={expired} deleted={deleted}".format(
        admitted=int(counts.get("admitted", 0)),
        superseded=int(counts.get("superseded", 0)),
        expired=int(counts.get("expired", 0)),
        deleted=int(counts.get("deleted", 0)),
    )
)
' "$PROVIDER_OUTPUT"
}

render_ledger_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

ledger = json.loads(sys.argv[1])
if not isinstance(ledger, dict):
    raise SystemExit("experience-recall: lifecycle ledger response is not an object")
counts = ledger.get("effectiveCounts", {})
print(
    "ledger={ledger} repository={repository} entries={entries}".format(
        ledger=one_line(ledger.get("ledgerPath")),
        repository=one_line(ledger.get("repositoryAlias")),
        entries=int(ledger.get("entryCount", 0)),
    )
)
print(
    "effective admitted={admitted} superseded={superseded} expired={expired} deleted={deleted}".format(
        admitted=int(counts.get("admitted", 0)),
        superseded=int(counts.get("superseded", 0)),
        expired=int(counts.get("expired", 0)),
        deleted=int(counts.get("deleted", 0)),
    )
)
entries = ledger.get("entries", [])
if not entries:
    print("No lifecycle transitions recorded.")
for entry in entries:
    anchor = entry.get("sourceAnchor", {})
    print(
        "seq={sequence} record={record} state={state} at={at} source={source} reason={reason}".format(
            sequence=int(entry.get("sequence", 0)),
            record=one_line(entry.get("recordId")),
            state=one_line(entry.get("state")),
            at=one_line(entry.get("transitionedAt")),
            source=one_line(anchor.get("relativePath")),
            reason=one_line(entry.get("reason")),
        )
    )
' "$PROVIDER_OUTPUT"
}

render_export_text() {
  python3 -c '
import json
import sys
import unicodedata

def one_line(value):
    if value is None:
        return "none"
    safe = "".join("?" if unicodedata.category(character).startswith("C") or unicodedata.category(character) in {"Zl", "Zp"} else character for character in str(value))
    return " ".join(safe.split())

records = json.loads(sys.argv[1])
if not isinstance(records, list):
    raise SystemExit("experience-recall: export response is not an array")
print("exported={count} bodies=excluded transcripts=excluded".format(count=len(records)))
if not records:
    print("No records matched the bounded export selection.")
for record in records:
    anchor = record.get("sourceAnchor", {})
    print(
        "record={record} kind={kind} trust={trust} lifecycle={lifecycle} source={source} selector={selector}".format(
            record=one_line(record.get("recordId")),
            kind=one_line(record.get("kind")),
            trust=one_line(record.get("sourceTrust")),
            lifecycle=one_line(record.get("lifecycle", {}).get("state")),
            source=one_line(anchor.get("relativePath")),
            selector=one_line(anchor.get("selector")),
        )
    )
' "$PROVIDER_OUTPUT"
}

cmd_search() {
  local query="${1:-}"
  local limit="5"
  local spec_ref=""
  local scope_ref=""
  local option=""
  local -a kinds=()
  local -a trusts=()
  local -a provider_args=()
  local limit_seen=0
  local spec_ref_seen=0
  local scope_ref_seen=0

  [[ -n "$query" && "$query" != --* ]] || fail_usage "search requires one quoted query"
  shift
  FORMAT="json"
  while [[ $# -gt 0 ]]; do
    option="$1"
    reject_derived_control "$option"
    case "$option" in
      --limit)
        [[ $# -ge 2 ]] || fail_usage "--limit requires a value"
        [[ "$limit_seen" -eq 0 ]] || fail_usage "--limit may be specified once"
        limit_seen=1
        limit="$2"
        shift 2
        ;;
      --kind)
        [[ $# -ge 2 ]] || fail_usage "--kind requires a value"
        validate_public_value "--kind" "$2" || return 1
        kinds+=("$2")
        shift 2
        ;;
      --trust)
        [[ $# -ge 2 ]] || fail_usage "--trust requires a value"
        validate_public_value "--trust" "$2" || return 1
        trusts+=("$2")
        shift 2
        ;;
      --spec-ref)
        [[ $# -ge 2 ]] || fail_usage "--spec-ref requires a value"
        [[ "$spec_ref_seen" -eq 0 ]] || fail_usage "--spec-ref may be specified once"
        spec_ref_seen=1
        validate_public_value "--spec-ref" "$2" || return 1
        spec_ref="$2"
        shift 2
        ;;
      --scope-ref)
        [[ $# -ge 2 ]] || fail_usage "--scope-ref requires a value"
        [[ "$scope_ref_seen" -eq 0 ]] || fail_usage "--scope-ref may be specified once"
        scope_ref_seen=1
        validate_public_value "--scope-ref" "$2" || return 1
        scope_ref="$2"
        shift 2
        ;;
      --format)
        [[ $# -ge 2 ]] || fail_usage "--format requires a value"
        set_format "$2"
        shift 2
        ;;
      *) fail_usage "unknown search option: $option" ;;
    esac
  done
  [[ "$limit" =~ ^[0-9]+$ && "$limit" -ge 1 && "$limit" -le 20 ]] ||
    fail_usage "--limit must be an integer from 1 through 20"

  provider_args=(search --repo-root "$REPO_ROOT" --repository-alias "$REPOSITORY_ALIAS" --text "$query" --limit "$limit")
  for option in "${kinds[@]}"; do provider_args+=(--kind "$option"); done
  for option in "${trusts[@]}"; do provider_args+=(--trust "$option"); done
  [[ -z "$spec_ref" ]] || provider_args+=(--spec-ref "$spec_ref")
  [[ -z "$scope_ref" ]] || provider_args+=(--scope-ref "$scope_ref")

  run_provider "${provider_args[@]}"
  propagate_provider_failure || return $?
  validate_provider_response search || return 1
  if [[ "$ADAPTER" == "none" ]]; then
    if [[ "$FORMAT" == "json" ]]; then
      printf '%s\n' "$PROVIDER_OUTPUT"
    else
      echo "Experience recall disabled (adapter=none)."
    fi
  elif [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_search_text
  fi
}

cmd_read() {
  local record_id="${1:-}"
  local option=""
  [[ -n "$record_id" && "$record_id" != --* ]] || fail_usage "read requires a record id"
  validate_public_value "record id" "$record_id" || return 1
  shift
  FORMAT="json"
  while [[ $# -gt 0 ]]; do
    option="$1"
    reject_derived_control "$option"
    case "$option" in
      --format)
        [[ $# -ge 2 ]] || fail_usage "--format requires a value"
        set_format "$2"
        shift 2
        ;;
      *) fail_usage "unknown read option: $option" ;;
    esac
  done

  run_provider read --repo-root "$REPO_ROOT" --repository-alias "$REPOSITORY_ALIAS" --record-id "$record_id"
  propagate_provider_failure || return $?
  if [[ "$ADAPTER" == "none" ]]; then
    if [[ "$FORMAT" == "json" ]]; then
      disabled_refusal_json read
    else
      echo "Experience recall disabled (adapter=none); read is unavailable."
    fi
    return 5
  fi
  validate_provider_response read || return 1
  if [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_read_text
  fi
}

parse_simple_format() {
  local operation="$1"
  shift
  local option=""
  FORMAT="json"
  while [[ $# -gt 0 ]]; do
    option="$1"
    reject_derived_control "$option"
    case "$option" in
      --format)
        [[ $# -ge 2 ]] || fail_usage "--format requires a value"
        set_format "$2"
        shift 2
        ;;
      *) fail_usage "unexpected $operation argument: $option" ;;
    esac
  done
}

cmd_status() {
  parse_simple_format status "$@"
  run_provider status --repo-root "$REPO_ROOT" --repository-alias "$REPOSITORY_ALIAS"
  propagate_provider_failure || return $?
  if [[ "$ADAPTER" == "none" ]]; then
    if [[ "$FORMAT" == "json" ]]; then
      disabled_status_json
    else
      echo "adapter=none state=disabled repository=$REPOSITORY_ALIAS"
      echo "index=none records=0 candidates=0 excluded=0"
      echo "lifecycle admitted=0 superseded=0 expired=0 deleted=0"
      echo "exclusions none"
      echo "freshness=disabled reason=experience recall adapter is disabled"
    fi
    return 0
  fi
  validate_provider_response status || return 1
  if [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_status_text
  fi
}

cmd_freshness() {
  parse_simple_format freshness "$@"
  run_provider freshness --repo-root "$REPO_ROOT" --repository-alias "$REPOSITORY_ALIAS"
  propagate_provider_failure || return $?
  if [[ "$ADAPTER" == "none" ]]; then
    if [[ "$FORMAT" == "json" ]]; then
      disabled_freshness_json
    else
      echo "adapter=none freshness=disabled reason=experience recall adapter is disabled"
    fi
    return 5
  fi
  validate_provider_response freshness || return 1
  if [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_freshness_text
  fi
  freshness_exit
}

cmd_sync() {
  parse_simple_format sync "$@"
  run_provider sync --repo-root "$REPO_ROOT" --repository-alias "$REPOSITORY_ALIAS"
  propagate_provider_failure || return $?
  if [[ "$ADAPTER" == "none" ]]; then
    if [[ "$FORMAT" == "json" ]]; then
      disabled_refusal_json sync
    else
      echo "Experience recall disabled (adapter=none); sync is unavailable."
    fi
    return 5
  fi
  validate_provider_response sync || return 1
  if [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_sync_text
  fi
}

refuse_disabled() {
  local operation="$1"
  local message="$2"
  if [[ "$FORMAT" == "json" ]]; then
    disabled_refusal_json "$operation"
  else
    echo "Experience recall disabled (adapter=none); $message is unavailable."
  fi
  return 5
}

parse_transition_options() {
  TRANSITION_REASON=""
  TRANSITION_REASON_SEEN=0
  local operation="$1"
  shift
  local option=""
  FORMAT="json"
  while [[ $# -gt 0 ]]; do
    option="$1"
    reject_derived_control "$option"
    case "$option" in
      --reason)
        [[ $# -ge 2 ]] || fail_usage "--reason requires a value"
        [[ "$TRANSITION_REASON_SEEN" -eq 0 ]] || fail_usage "--reason may be specified once"
        TRANSITION_REASON_SEEN=1
        validate_public_value "--reason" "$2" || return 1
        TRANSITION_REASON="$2"
        shift 2
        ;;
      --format)
        [[ $# -ge 2 ]] || fail_usage "--format requires a value"
        set_format "$2"
        shift 2
        ;;
      *) fail_usage "unknown $operation option: $option" ;;
    esac
  done
}

run_transition() {
  local operation="$1"
  local state="$2"
  local record_id="$3"
  shift 3
  [[ -n "$record_id" && "$record_id" != --* ]] || fail_usage "$operation requires a record id"
  validate_public_value "record id" "$record_id" || return 1
  parse_transition_options "$operation" "$@" || return 1

  if [[ "$ADAPTER" == "none" ]]; then
    refuse_disabled "$operation" "$operation"
    return $?
  fi

  local -a engine_args=(set --record-id "$record_id" --state "$state")
  [[ "$TRANSITION_REASON_SEEN" -eq 0 ]] || engine_args+=(--reason "$TRANSITION_REASON")
  run_engine "${engine_args[@]}"
  propagate_provider_failure || return $?
  validate_provider_response lifecycle || return 1
  if [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_lifecycle_text
  fi
}

cmd_delete() {
  run_transition delete deleted "${1:-}" "${@:2}"
}

cmd_admit() {
  run_transition admit admitted "${1:-}" "${@:2}"
}

cmd_lifecycle() {
  local action="${1:-}"
  case "$action" in
    list)
      shift
      parse_simple_format "lifecycle list" "$@"
      if [[ "$ADAPTER" == "none" ]]; then
        refuse_disabled lifecycle "lifecycle list"
        return $?
      fi
      run_engine ledger
      propagate_provider_failure || return $?
      validate_provider_response ledger || return 1
      if [[ "$FORMAT" == "json" ]]; then
        printf '%s\n' "$PROVIDER_OUTPUT"
      else
        render_ledger_text
      fi
      ;;
    set)
      [[ $# -ge 3 ]] || fail_usage "lifecycle set requires a state and a record id"
      local state="$2"
      case "$state" in
        admitted | superseded | expired | deleted) ;;
        *) fail_usage "lifecycle set state must be admitted, superseded, expired, or deleted" ;;
      esac
      run_transition "lifecycle set" "$state" "$3" "${@:4}"
      ;;
    '' | --*) fail_usage "lifecycle requires the action 'list' or 'set'" ;;
    *) fail_usage "unknown lifecycle action: $action" ;;
  esac
}

cmd_export() {
  local limit=""
  local output=""
  local option=""
  local -a record_ids=()
  local -a kinds=()
  local -a states=()
  local limit_seen=0
  local output_seen=0

  FORMAT="json"
  while [[ $# -gt 0 ]]; do
    option="$1"
    reject_derived_control "$option"
    case "$option" in
      --limit)
        [[ $# -ge 2 ]] || fail_usage "--limit requires a value"
        [[ "$limit_seen" -eq 0 ]] || fail_usage "--limit may be specified once"
        limit_seen=1
        limit="$2"
        shift 2
        ;;
      --record-id)
        [[ $# -ge 2 ]] || fail_usage "--record-id requires a value"
        validate_public_value "--record-id" "$2" || return 1
        record_ids+=("$2")
        shift 2
        ;;
      --kind)
        [[ $# -ge 2 ]] || fail_usage "--kind requires a value"
        validate_public_value "--kind" "$2" || return 1
        kinds+=("$2")
        shift 2
        ;;
      --state)
        [[ $# -ge 2 ]] || fail_usage "--state requires a value"
        validate_public_value "--state" "$2" || return 1
        states+=("$2")
        shift 2
        ;;
      --output)
        [[ $# -ge 2 ]] || fail_usage "--output requires a value"
        [[ "$output_seen" -eq 0 ]] || fail_usage "--output may be specified once"
        output_seen=1
        validate_public_value "--output" "$2" || return 1
        output="$2"
        shift 2
        ;;
      --format)
        [[ $# -ge 2 ]] || fail_usage "--format requires a value"
        set_format "$2"
        shift 2
        ;;
      *) fail_usage "unknown export option: $option" ;;
    esac
  done
  [[ "$limit_seen" -eq 1 ]] || fail_usage "export requires an explicit --limit"
  [[ "$limit" =~ ^[0-9]+$ && "$limit" -ge 1 && "$limit" -le 20 ]] ||
    fail_usage "--limit must be an integer from 1 through 20"

  if [[ "$ADAPTER" == "none" ]]; then
    refuse_disabled export export
    return $?
  fi

  local -a engine_args=(export --limit "$limit")
  for option in ${record_ids[@]+"${record_ids[@]}"}; do engine_args+=(--record-id "$option"); done
  for option in ${kinds[@]+"${kinds[@]}"}; do engine_args+=(--kind "$option"); done
  for option in ${states[@]+"${states[@]}"}; do engine_args+=(--state "$option"); done
  [[ "$output_seen" -eq 0 ]] || engine_args+=(--output "$output")

  run_engine "${engine_args[@]}"
  propagate_provider_failure || return $?
  validate_provider_response export || return 1
  if [[ "$FORMAT" == "json" ]]; then
    printf '%s\n' "$PROVIDER_OUTPUT"
  else
    render_export_text
  fi
}

main() {
  local subcommand="${1:-}"
  case "$subcommand" in
    -h | --help | help | '')
      usage
      return 0
      ;;
    search | read | status | freshness | sync | delete | admit | lifecycle | export) ;;
    *) fail_usage "unknown subcommand: $subcommand" ;;
  esac
  shift

  resolve_provider
  case "$subcommand" in
    search) cmd_search "$@" ;;
    read) cmd_read "$@" ;;
    status) cmd_status "$@" ;;
    freshness) cmd_freshness "$@" ;;
    sync) cmd_sync "$@" ;;
    delete) cmd_delete "$@" ;;
    admit) cmd_admit "$@" ;;
    lifecycle) cmd_lifecycle "$@" ;;
    export) cmd_export "$@" ;;
  esac
}

main "$@"
