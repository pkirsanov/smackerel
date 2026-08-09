#!/usr/bin/env bash
#
# bubbles result-envelope-validate.sh
#
# Scans every `agents/*.agent.md` for ```json fenced blocks tagged as
# `result_envelope:` (or `result-envelope:`) and validates each against
# bubbles/schemas/result-envelope.schema.json.
#
# Modes (history):
#   v5.2 / F5: full ADVISORY. Missing or malformed envelopes warn only.
#   v6.0 / B3: malformed envelopes BLOCK. Missing envelopes still WARN
#              (full coverage tracked as v6.1 follow-up — flipping all 40
#              agents at once would block every push without rolling
#              authoring work first).
#
# IMP-037 / SCOPE-6 adds a recall-authority refusal. An envelope that cites a
# recall record id, the recall index, or a recall export as EVIDENCE is refused
# in EVERY mode — including --advisory — because it is an authority breach, not
# a schema nit. It is handled like the repository-provenance class, which also
# blocks unconditionally.
#
# Usage:
#   result-envelope-validate.sh                  # v6.0 default: malformed
#                                                # blocks, missing warns
#   result-envelope-validate.sh --advisory       # v5.2 behavior: never block
#   result-envelope-validate.sh --strict         # block on missing OR malformed
#                                                # (v6.1+; opt-in until all
#                                                # agents are populated)
#   result-envelope-validate.sh [mode] \
#     --session-id <id> \
#     --session-control-file <path> \
#     --binding-packet-file <path> \
#     [--scenario-file <path> --node-id <id>]     # goal-node bindings derive
#                                                # their exact declaration
#
# Exit codes:
#   0  no blocking findings
#   1  at least one blocking finding for the active mode
#   2  usage error or missing schema

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCHEMA="$REPO_ROOT/bubbles/schemas/result-envelope.schema.json"
AGENTS_DIR="$REPO_ROOT/agents"

resolve_source_script_dir() {
    local source_path="${BASH_SOURCE[0]}"
    local link_target=""
    while [[ -L "$source_path" ]]; do
        link_target="$(readlink "$source_path")" || return 1
        if [[ "$link_target" = /* ]]; then
            source_path="$link_target"
        else
            source_path="$(dirname "$source_path")/$link_target"
        fi
    done
    cd "$(dirname "$source_path")" && pwd
}

SOURCE_SCRIPT_DIR="$(resolve_source_script_dir)"
REPOSITORY_BINDING="$SOURCE_SCRIPT_DIR/repository-binding.sh"
# Recall shapes are derived from the real indexer, never restated. See the
# load_recall_constants() comment below for why the fallback cannot drift.
RECALL_INDEX_SOURCE="$SCRIPT_DIR/experience-recall-index.py"
[[ -f "$RECALL_INDEX_SOURCE" ]] || RECALL_INDEX_SOURCE="$SOURCE_SCRIPT_DIR/experience-recall-index.py"

MODE="v6-default"  # v6.0 / B3 default: malformed blocks, missing warns.
SESSION_ID=""
SESSION_CONTROL_FILE=""
BINDING_PACKET_FILE=""
VALIDATED_PACKET_FILE=""
VALIDATED_PACKET=""
SCENARIO_FILE=""
NODE_ID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) MODE="strict"; shift;;
    --advisory) MODE="advisory"; shift;;
        --session-id)
            [[ $# -ge 2 ]] || { echo "result-envelope-validate: --session-id requires a value" >&2; exit 2; }
            SESSION_ID="$2"
            shift 2
            ;;
        --session-control-file)
            [[ $# -ge 2 ]] || { echo "result-envelope-validate: --session-control-file requires a value" >&2; exit 2; }
            SESSION_CONTROL_FILE="$2"
            shift 2
            ;;
        --binding-packet-file)
            [[ $# -ge 2 ]] || { echo "result-envelope-validate: --binding-packet-file requires a value" >&2; exit 2; }
            BINDING_PACKET_FILE="$2"
            shift 2
            ;;
        --scenario-file)
            [[ $# -ge 2 ]] || { echo "result-envelope-validate: --scenario-file requires a value" >&2; exit 2; }
            SCENARIO_FILE="$2"
            shift 2
            ;;
        --node-id)
            [[ $# -ge 2 ]] || { echo "result-envelope-validate: --node-id requires a value" >&2; exit 2; }
            NODE_ID="$2"
            shift 2
            ;;
    -h|--help)
      sed -n '1,30p' "$0" >&2
      exit 0
      ;;
    *) echo "result-envelope-validate: unknown arg: $1" >&2; exit 2;;
  esac
done

BINDING_REQUIRED=false
if [[ -n "$SESSION_ID" || -n "$SESSION_CONTROL_FILE" || -n "$BINDING_PACKET_FILE" ]]; then
    BINDING_REQUIRED=true
    [[ -n "$SESSION_ID" ]] || { echo "result-envelope-validate: --session-id is required with repository binding" >&2; exit 2; }
    [[ -n "$SESSION_CONTROL_FILE" ]] || { echo "result-envelope-validate: --session-control-file is required with repository binding" >&2; exit 2; }
    [[ -n "$BINDING_PACKET_FILE" ]] || { echo "result-envelope-validate: --binding-packet-file is required with repository binding" >&2; exit 2; }
    if [[ -n "$SCENARIO_FILE" && -z "$NODE_ID" ]] || [[ -z "$SCENARIO_FILE" && -n "$NODE_ID" ]]; then
        echo "result-envelope-validate: goal-node validation requires both --scenario-file and --node-id" >&2
        exit 2
    fi
    [[ -f "$REPOSITORY_BINDING" ]] || { echo "result-envelope-validate: repository binding validator missing at $REPOSITORY_BINDING" >&2; exit 2; }
    VALIDATED_PACKET_FILE="$(mktemp)" || { echo "result-envelope-validate: unable to create immutable packet snapshot" >&2; exit 2; }
    trap 'rm -f "$VALIDATED_PACKET_FILE"' EXIT INT TERM
    cp -- "$BINDING_PACKET_FILE" "$VALIDATED_PACKET_FILE" || {
        echo "result-envelope-validate: unable to capture binding packet" >&2
        exit 2
    }
    chmod 600 "$VALIDATED_PACKET_FILE"
    set +e
    if [[ -n "$SCENARIO_FILE" ]]; then
        BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" validate-packet \
            --session-id "$SESSION_ID" \
            --session-control-file "$SESSION_CONTROL_FILE" \
            --packet-file "$VALIDATED_PACKET_FILE" \
            --scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID" 2>&1)"
    else
        BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" validate-packet \
            --session-id "$SESSION_ID" \
            --session-control-file "$SESSION_CONTROL_FILE" \
            --packet-file "$VALIDATED_PACKET_FILE" 2>&1)"
    fi
    BINDING_RC=$?
    set -e
    if [[ "$BINDING_RC" -ne 0 ]]; then
        printf '%s\n' "$BINDING_OUTPUT" >&2
        exit "$BINDING_RC"
    fi
    VALIDATED_PACKET="$(cat -- "$VALIDATED_PACKET_FILE")" || exit 2
fi

[[ -f "$SCHEMA" ]] || { echo "result-envelope-validate: schema missing at $SCHEMA" >&2; exit 2; }
[[ -d "$AGENTS_DIR" ]] || { echo "result-envelope-validate: agents/ missing at $AGENTS_DIR" >&2; exit 2; }

# Dependency posture (IMP-027 / SCOPE-4). dependency-posture.sh's own header
# lists this script among the ten that silently skipped on a missing dependency,
# but it never actually sourced the module. Sourcing it here honors that
# contract AND activates the managed interpreter from python-env.sh, so a
# provisioned environment satisfies the import without any PATH ceremony.
_rev_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$_rev_script_dir/dependency-posture.sh" ]]; then
  # shellcheck source=/dev/null
  . "$_rev_script_dir/dependency-posture.sh"
fi
unset _rev_script_dir

if ! command -v python3 >/dev/null 2>&1; then
  if declare -f bubbles_require_dep >/dev/null 2>&1; then
    bubbles_require_dep "result-envelope-validate" \
      "python3 — result envelopes cannot be schema-validated without it" || exit 0
  fi
  echo "result-envelope-validate: SKIP (python3 not installed)"
  exit 0
fi

if ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  if declare -f bubbles_require_dep >/dev/null 2>&1; then
    bubbles_require_dep "result-envelope-validate" \
      "the python 'jsonschema' module — result envelopes cannot be schema-validated without it" || exit 0
  fi
  echo "result-envelope-validate: SKIP (python jsonschema not installed)"
  exit 0
fi

AGENTS_DIR="$AGENTS_DIR" \
SCHEMA="$SCHEMA" \
MODE="$MODE" \
BINDING_REQUIRED="$BINDING_REQUIRED" \
VALIDATED_PACKET="$VALIDATED_PACKET" \
RECALL_INDEX_SOURCE="$RECALL_INDEX_SOURCE" \
python3 - <<'PY'
import ast, json, os, re, sys
from pathlib import Path, PurePosixPath

agents_dir = Path(os.environ['AGENTS_DIR'])
schema_path = Path(os.environ['SCHEMA'])
mode = os.environ.get('MODE', 'v6-default')  # advisory | v6-default | strict
binding_required = os.environ.get('BINDING_REQUIRED', 'false') == 'true'
validated_packet = os.environ.get('VALIDATED_PACKET', '')

try:
    import jsonschema
except Exception:
    # Unreachable after the bash preflight above. Kept as a hard failure rather
    # than a skip so no path through this script can report success for a
    # validation that never ran.
    print("result-envelope-validate: FAIL - jsonschema missing after preflight", file=sys.stderr)
    sys.exit(2)

schema = json.loads(schema_path.read_text())
binding_packet = json.loads(validated_packet) if binding_required else None
binding_schema_path = schema_path.parent / 'repository-binding.schema.json'
schema_store = {}
if binding_schema_path.is_file():
    binding_schema = json.loads(binding_schema_path.read_text())
    schema_store[binding_schema_path.resolve().as_uri()] = binding_schema
    schema_store['https://github.com/pkirsanov/bubbles/schemas/repository-binding.schema.json'] = binding_schema
    binding_schema_id = binding_schema.get('$id')
    if binding_schema_id:
        schema_store[binding_schema_id] = binding_schema
schema_resolver = jsonschema.RefResolver.from_schema(schema, store=schema_store)

PROVENANCE_FIELDS = (
    'sessionId',
    'decisionId',
    'controlRevision',
    'controlPathDigest',
    'authority',
    'transition',
    'scopeKind',
    'scopeId',
    'targetKind',
    'pathVisibility',
    'actionable',
)

def repository_provenance_error(doc):
    missing = [
        field for field in ('repositoryRoot', 'repositoryAlias', 'repositoryResolution')
        if field not in doc
    ]
    if missing:
        return f"repository provenance missing fields: {', '.join(missing)}"
    resolution = doc.get('repositoryResolution')
    if not isinstance(resolution, dict):
        return 'repositoryResolution must be an object'
    missing_resolution = [field for field in PROVENANCE_FIELDS if field not in resolution]
    if missing_resolution:
        return f"repositoryResolution missing fields: {', '.join(missing_resolution)}"
    if doc['repositoryRoot'] != binding_packet['repositoryRoot']:
        return 'repositoryRoot does not match the current binding packet'
    if doc['repositoryAlias'] != binding_packet['repositoryAlias']:
        return 'repositoryAlias does not match the current binding packet'
    expected_resolution = binding_packet['repositoryResolution']
    mismatched = [
        field for field in PROVENANCE_FIELDS
        if resolution[field] != expected_resolution[field]
    ]
    if mismatched:
        return f"repositoryResolution does not match current packet: {', '.join(mismatched)}"
    return None

def repository_projection_error(doc):
    resolution = doc.get('repositoryResolution')
    if not isinstance(resolution, dict):
        return None
    visibility = resolution.get('pathVisibility')
    actionable = resolution.get('actionable')
    repository_root = doc.get('repositoryRoot')
    if visibility == 'redacted':
        if repository_root != '<redacted-local-root>':
            return 'redacted repository result must not retain a canonical local root'
        if actionable is not False:
            return 'redacted repository result must be non-actionable'
    if visibility == 'local':
        if not isinstance(repository_root, str) or not repository_root.startswith('/'):
            return 'local repository result requires a canonical absolute root'
        if actionable is not True:
            return 'local repository result must be actionable'
        if not binding_required:
            return (
                'local actionable result requires --session-id, '
                '--session-control-file, and --binding-packet-file'
            )
    return None

def finding_accounting_error(doc):
    """IMP-038 SCOPE-4 / GF-3: routing is not resolution.

    A routed finding was handed to another owner, not fixed. Reporting it in
    addressedFindings is how a bounded run claims closure it never performed,
    and it reads as a clean result to anyone scanning the envelope. The same
    applies to `blocking-external`, which by definition the parent could not
    close. `independent` permits the parent to CONTINUE; it never permits the
    parent to claim the work is done, and it still requires the disposition
    artifact to exist.
    """
    findings = doc.get('findings')
    if not isinstance(findings, list):
        return None
    addressed = doc.get('addressedFindings')
    addressed_ids = set(addressed) if isinstance(addressed, list) else set()
    for entry in findings:
        if not isinstance(entry, dict):
            continue
        fid = entry.get('id')
        impact = entry.get('goalImpact')
        disposition = entry.get('disposition')
        filed = entry.get('filedArtifact')
        if fid in addressed_ids:
            if disposition == 'routed':
                return (
                    f"finding '{fid}' is disposition=routed but reported in "
                    'addressedFindings; routing hands work to an owner, it does not close it'
                )
            if impact == 'blocking-external':
                return (
                    f"finding '{fid}' is goalImpact=blocking-external but reported in "
                    'addressedFindings; the parent blocks on it rather than closing it'
                )
        needs_artifact = disposition == 'routed' or impact == 'independent'
        if needs_artifact and not (isinstance(filed, str) and filed.strip()):
            return (
                f"finding '{fid}' is {'disposition=routed' if disposition == 'routed' else 'goalImpact=independent'} "
                'but names no filedArtifact; an undischarged finding is not closed'
            )
    return None

# ---------------------------------------------------------------------------
# IMP-037 / SCOPE-6: recall artifacts can never become evidence.
#
# Recalled experience is authority tier 4 and always `advisory`. Reading it is
# fine. CITING it is the breach: the moment a recall artifact appears in an
# evidence field it has acquired an authority it does not have, and the claim it
# backs was never independently re-read.
#
# The three refused shapes are DERIVED from the real indexer
# (experience-recall-index.py), not restated here, so a change to the record-id
# format or the derived-state directory cannot silently outrun this guard. The
# literals below are only the fallback for a tree that ships the validator
# without the indexer; experience-recall-authority-selftest.sh asserts they
# still equal the indexer's own constants, so the fallback cannot drift unseen.
FALLBACK_RECORD_ID_PATTERN = r'^recall-[0-9a-f]{64}$'
FALLBACK_RUNTIME_PARTS = ('.specify', 'runtime', 'experience-recall')
RECALL_EXPORT_SCAN_BYTES = 256 * 1024
# Closed set. Deliberately NOT `summary` or `blocker.reason`: an agent SHOULD be
# able to say in prose that it consulted advisory recall. Scanning narrative
# fields would refuse the honest disclosure this contract wants to encourage.
EVIDENCE_FIELDS = ('evidenceRefs', 'toolCalls', 'evidence', 'dodRef')

def load_recall_constants(source):
    """Read the indexer's own RECORD_ID_RE and RUNTIME_PARTS without importing it."""
    pattern, parts = FALLBACK_RECORD_ID_PATTERN, FALLBACK_RUNTIME_PARTS
    try:
        tree = ast.parse(Path(source).read_text(encoding='utf-8'))
    except Exception:
        return pattern, parts
    for node in tree.body:
        if not isinstance(node, ast.Assign) or len(node.targets) != 1:
            continue
        target = node.targets[0]
        if not isinstance(target, ast.Name):
            continue
        if target.id == 'RUNTIME_PARTS':
            try:
                value = ast.literal_eval(node.value)
            except Exception:
                continue
            if isinstance(value, (tuple, list)) and value and all(isinstance(p, str) for p in value):
                parts = tuple(value)
        elif target.id == 'RECORD_ID_RE' and isinstance(node.value, ast.Call) and node.value.args:
            try:
                literal = ast.literal_eval(node.value.args[0])
            except Exception:
                continue
            if isinstance(literal, str) and literal:
                pattern = literal
    return pattern, parts

recall_pattern, recall_runtime_parts = load_recall_constants(
    os.environ.get('RECALL_INDEX_SOURCE', '')
)
# The indexer anchors its regex to match a whole value. An evidence citation
# embeds the id inside a longer string, so search the unanchored body.
RECALL_RECORD_ID_RE = re.compile(recall_pattern.strip('^$'))
RECALL_RUNTIME_DIR = '/'.join(recall_runtime_parts)
repo_root = agents_dir.parent

def iter_citation_strings(node, path):
    if isinstance(node, str):
        yield path, node
    elif isinstance(node, list):
        for index, item in enumerate(node):
            yield from iter_citation_strings(item, f'{path}[{index}]')
    elif isinstance(node, dict):
        for key, value in node.items():
            yield from iter_citation_strings(value, f'{path}.{key}')

def iter_evidence_citations(node, path=''):
    if isinstance(node, dict):
        for key, value in node.items():
            child = f'{path}.{key}' if path else key
            if key in EVIDENCE_FIELDS:
                yield from iter_citation_strings(value, child)
            else:
                yield from iter_evidence_citations(value, child)
    elif isinstance(node, list):
        for index, item in enumerate(node):
            yield from iter_evidence_citations(item, f'{path}[{index}]')

def citation_path(citation):
    """Strip a `#anchor` fragment and reject non-path citations."""
    candidate = citation.split('#', 1)[0].strip().replace('\\', '/')
    if not candidate or candidate.startswith(('http://', 'https://')):
        return None
    while candidate.startswith('./'):
        candidate = candidate[2:]
    return candidate or None

def is_recall_export_file(relative):
    """Classify by CONTENT, because `export --output` takes a caller-named path.

    A name-based rule would be evaded by renaming the file, and would also
    over-block an innocent path that merely looks recall-ish. An unreadable,
    absent, or non-export file is not a violation.
    """
    try:
        pure = PurePosixPath(relative)
        if pure.is_absolute() or any(part == '..' for part in pure.parts):
            return False
        target = repo_root / pure
        if target.is_symlink() or not target.is_file():
            return False
        if target.stat().st_size > RECALL_EXPORT_SCAN_BYTES:
            return False
        payload = json.loads(target.read_text(encoding='utf-8'))
    except Exception:
        return False
    for entry in payload if isinstance(payload, list) else [payload]:
        if not isinstance(entry, dict) or entry.get('contractType') != 'record':
            continue
        if 'recallAuthority' not in entry:
            continue
        record_id = entry.get('recordId')
        if isinstance(record_id, str) and RECALL_RECORD_ID_RE.fullmatch(record_id):
            return True
    return False

def recall_refusal(field, kind, citation):
    return (
        f"{field} cites {kind} ('{citation}'). Recalled experience is advisory "
        "(authority tier 4): it can never satisfy a DoD item or serve as execution "
        "evidence. Re-read the current source and cite that anchor instead - the "
        "spec/scope path, the report.md evidence anchor, or the decision artifact."
    )

def recall_authority_error(doc):
    for field, citation in iter_evidence_citations(doc):
        if RECALL_RECORD_ID_RE.search(citation):
            return recall_refusal(field, 'a recall record id', citation)
        relative = citation_path(citation)
        if relative is None:
            continue
        if RECALL_RUNTIME_DIR in relative:
            return recall_refusal(field, 'a recall index path', citation)
        if is_recall_export_file(relative):
            return recall_refusal(field, 'a recall export path', citation)
    return None

# Match a fenced block that looks like an envelope. Two acceptable shapes:
#   1. ```json result_envelope:        ```  ... ```
#   2. <!-- result_envelope --> ```json ... ```
#   3. ```jsonc with first non-blank line "// result_envelope"
# Cheap regex over option (1) (most common); future shapes can be added.
ENVELOPE_RE = re.compile(
    r'```(?:json[c5]?|jsonc)\s+(?:result[_-]envelope:?)\s*\n(.*?)\n```',
    re.DOTALL | re.IGNORECASE,
)
# Bare ```json fenced block under a heading "## Result Envelope" or
# "## RESULT-ENVELOPE" inside the prose. We accept a small lookbehind window.
BARE_ENVELOPE_RE = re.compile(
    r'(?:^|\n)#{1,4}\s+result[_\-\s]?envelope\b[^\n]*\n+```(?:json[c5]?|jsonc)\s*\n(.*?)\n```',
    re.DOTALL | re.IGNORECASE,
)

total_agents = 0
agents_with_envelope = 0
agents_missing_envelope = []
malformed_envelopes = []  # list of (path, error_text)
repository_binding_errors = []
recall_authority_errors = []

for p in sorted(agents_dir.glob('*.agent.md')):
    total_agents += 1
    text = p.read_text(errors='replace')
    matches = []
    for m in ENVELOPE_RE.finditer(text):
        matches.append(m.group(1))
    for m in BARE_ENVELOPE_RE.finditer(text):
        matches.append(m.group(1))
    if not matches:
        agents_missing_envelope.append(p.name)
        continue
    agents_with_envelope += 1
    for raw in matches:
        try:
            doc = json.loads(raw)
        except json.JSONDecodeError as e:
            malformed_envelopes.append((p.name, f"JSON parse error: {e}"))
            continue
        recall_error = recall_authority_error(doc)
        if recall_error:
            recall_authority_errors.append((p.name, recall_error))
            continue
        projection_error = repository_projection_error(doc)
        if projection_error:
            error_text = f"Repository provenance error: {projection_error}"
            malformed_envelopes.append((p.name, error_text))
            repository_binding_errors.append((p.name, error_text))
            continue
        try:
            jsonschema.validate(doc, schema, resolver=schema_resolver)
        except jsonschema.ValidationError as e:
            malformed_envelopes.append((p.name, f"Schema error: {e.message} at {list(e.path)}"))
            continue
        if binding_required:
            provenance_error = repository_provenance_error(doc)
            if provenance_error:
                error_text = f"Repository provenance error: {provenance_error}"
                malformed_envelopes.append((p.name, error_text))
                repository_binding_errors.append((p.name, error_text))
        accounting_error = finding_accounting_error(doc)
        if accounting_error:
            malformed_envelopes.append((p.name, f"Finding accounting error: {accounting_error}"))

# Report.
print(f"result-envelope-validate: scanned {total_agents} agent file(s)")
print(f"  with valid envelope: {agents_with_envelope}")
print(f"  missing envelope: {len(agents_missing_envelope)}")
print(f"  malformed envelope(s): {len(malformed_envelopes)}")
print(f"  recall-authority violation(s): {len(recall_authority_errors)}")
print(f"  mode: {mode}")

if agents_missing_envelope and mode != "strict":
    print("  Advisory: the following agents do not yet emit a result_envelope JSON block:")
    for name in agents_missing_envelope[:10]:
        print(f"    - {name}")
    if len(agents_missing_envelope) > 10:
        print(f"    ... and {len(agents_missing_envelope) - 10} more")
    if mode == "v6-default":
        print("  Missing envelopes are advisory by default in the current contract; use --strict to make missing envelopes blocking.")

for name, err in malformed_envelopes[:10]:
    print(f"  MALFORMED: {name}: {err}")

for name, err in recall_authority_errors[:10]:
    print(f"  RECALL-AUTHORITY: {name}: {err}")

# Exit policy:
#   advisory     -> always 0
#   v6-default   -> 1 iff any malformed; missing warns only
#   strict       -> 1 iff any malformed OR missing
# Repository-provenance and recall-authority breaches are authority failures,
# not schema nits, so they block in EVERY mode including advisory.
if repository_binding_errors or recall_authority_errors:
    sys.exit(1)
if mode == "advisory":
    sys.exit(0)
if mode == "v6-default":
    sys.exit(1 if malformed_envelopes else 0)
# strict
sys.exit(1 if (agents_missing_envelope or malformed_envelopes) else 0)
PY
