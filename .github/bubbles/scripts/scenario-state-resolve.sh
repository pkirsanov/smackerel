#!/usr/bin/env bash
# scenario-state-resolve.sh — derive every scenario state from receipts.
#
# Capability: scenario-outcome-derivation
#
# WHY THIS EXISTS
# A scope was "done" when its checkboxes were full. A checkbox proves that
# somebody ticked a box. What the framework needs to know is whether the named
# scenario went RED and then GREEN on the SAME test — that is the only shape
# that distinguishes "the change worked" from "the test always passed".
#
# THE RULE THIS SCRIPT ENFORCES ABOVE ALL OTHERS
# Every scenario state is DERIVED FROM RECEIPTS AND NEVER DECLARED. A state
# written by hand into scenario-manifest.json is REFUSED, by name, with
# SCS-DECLARED-STATE. This applies to all eight states with no exception.
#
# The reason is arithmetic, not purity. Eight states, per scenario, per scope,
# per spec, hand-maintained, is a LARGER bookkeeping tax than the checkbox
# accounting it replaces. The outcome model only reduces burden if the states
# are computed. So the one thing that would turn it into the biggest tax in the
# framework's history is the one thing this script refuses.
#
# The state vocabulary, the receipt binding contract, the ordering rules and the
# failure codes all live in bubbles/registry/scenario-states.yaml. This script
# READS that registry. It does not restate it.
#
# Usage:
#   bash bubbles/scripts/scenario-state-resolve.sh --spec-dir <dir> [options]
#
# Options:
#   --spec-dir <dir>        Feature/bug directory holding scenario-manifest.json
#   --log <path>            Receipt log (default <repo>/.specify/runtime/tool-calls.jsonl)
#   --source-revision <rev> Revision receipts must cite (default: git HEAD)
#   --changed-file <path>   Repeatable. A changed path; scenarios whose
#                           implementationRefs intersect it are marked AFFECTED.
#   --require <STATE>       Repeatable. A state every applicable scenario must
#                           reach for --certifiable to hold.
#   --certifiable           Exit 1 unless every required state holds for every
#                           applicable scenario.
#   --format text|json      Output format (default: text)
#   --registry <path>       Override the registry location (hermetic tests)
#
# There is no --skip, --force, --ignore or --assume flag, and there will not be
# one. A missing state is fixed by running the test, never by silencing the
# resolver.
#
# Exit codes:
#   0  resolved (and, under --certifiable, every required state holds)
#   1  a refusal was raised, or --certifiable was requested and is not satisfied
#   2  usage error, missing dependency, unreadable registry or unreadable manifest

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="scenario-state-resolve"
REGISTRY="$SCRIPT_DIR/../registry/scenario-states.yaml"

SPEC_DIR=""
LOG_PATH=""
SOURCE_REVISION=""
FORMAT="text"
CERTIFIABLE="false"
REQUIRED_STATES=()
CHANGED_FILES=()

usage() {
  sed -n '25,47p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

die_usage() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  usage >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --spec-dir) SPEC_DIR="${2:-}"; shift 2 ;;
    --log) LOG_PATH="${2:-}"; shift 2 ;;
    --source-revision) SOURCE_REVISION="${2:-}"; shift 2 ;;
    --changed-file) CHANGED_FILES+=("${2:-}"); shift 2 ;;
    --require) REQUIRED_STATES+=("${2:-}"); shift 2 ;;
    --certifiable) CERTIFIABLE="true"; shift ;;
    --format) FORMAT="${2:-}"; shift 2 ;;
    --registry) REGISTRY="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    --skip* | --force* | --ignore* | --assume* | --allow*)
      printf '%s: "%s" does not exist. A missing state is fixed by running the test.\n' "$NAME" "$1" >&2
      exit 2
      ;;
    *) die_usage "unknown argument: $1" ;;
  esac
done

[[ -n "$SPEC_DIR" ]] || die_usage "--spec-dir is required"
[[ -d "$SPEC_DIR" ]] || die_usage "spec dir not found: $SPEC_DIR"
case "$FORMAT" in
  text | json) ;;
  *) die_usage "--format must be text or json (got: $FORMAT)" ;;
esac
[[ -f "$REGISTRY" ]] || {
  printf '%s: registry not found: %s\n' "$NAME" "$REGISTRY" >&2
  exit 2
}

PYTHON_BIN="${BUBBLES_PYTHON:-}"
if [[ -z "$PYTHON_BIN" ]] && command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN="$(command -v python3)"
fi
[[ -n "$PYTHON_BIN" ]] || {
  printf '%s: python3 is required\n' "$NAME" >&2
  exit 2
}

MANIFEST="$SPEC_DIR/scenario-manifest.json"

if [[ -z "$LOG_PATH" ]]; then
  _repo_root="$(cd "$SPEC_DIR" && git rev-parse --show-toplevel 2>/dev/null || pwd)"
  LOG_PATH="$_repo_root/.specify/runtime/tool-calls.jsonl"
fi

# The source revision receipts are checked against. A receipt proves what the
# tree WAS; if the tree moved, the receipt is stale and the resolver says so
# rather than carrying it forward silently.
if [[ -z "$SOURCE_REVISION" ]]; then
  SOURCE_REVISION="$(git -C "$SPEC_DIR" rev-parse --verify HEAD 2>/dev/null || true)"
fi

# ROLLBACK (registry: migration.rollback). State advancement stops and the
# legacy phase state is used. Receipts and occurrence records are PRESERVED —
# rollback loses the derivation, never the evidence.
ROLLBACK="${BUBBLES_SCENARIO_STATE_ROLLBACK:-0}"

CHANGED_JOINED=""
if [[ "${#CHANGED_FILES[@]}" -gt 0 ]]; then
  CHANGED_JOINED="$(printf '%s\n' "${CHANGED_FILES[@]}")"
fi
REQUIRED_JOINED=""
if [[ "${#REQUIRED_STATES[@]}" -gt 0 ]]; then
  REQUIRED_JOINED="$(printf '%s\n' "${REQUIRED_STATES[@]}")"
fi

MANIFEST="$MANIFEST" \
  LOG_PATH="$LOG_PATH" \
  REGISTRY="$REGISTRY" \
  SOURCE_REVISION="$SOURCE_REVISION" \
  FORMAT="$FORMAT" \
  CERTIFIABLE="$CERTIFIABLE" \
  REQUIRED_JOINED="$REQUIRED_JOINED" \
  CHANGED_JOINED="$CHANGED_JOINED" \
  ROLLBACK="$ROLLBACK" \
  "$PYTHON_BIN" - <<'PY'
import hashlib, json, os, re, stat, sys

manifest_path = os.environ['MANIFEST']
log_path = os.environ['LOG_PATH']
registry_path = os.environ['REGISTRY']
source_revision = os.environ.get('SOURCE_REVISION', '').strip()
fmt = os.environ.get('FORMAT', 'text')
certifiable_mode = os.environ.get('CERTIFIABLE', 'false') == 'true'
rollback = os.environ.get('ROLLBACK', '0') == '1'
required_states = [s for s in os.environ.get('REQUIRED_JOINED', '').splitlines() if s.strip()]
changed_files = [c.strip() for c in os.environ.get('CHANGED_JOINED', '').splitlines() if c.strip()]

# --- registry read ---------------------------------------------------------
# Deliberately a small line reader rather than a YAML dependency. The registry
# has a fixed shallow shape, and making this resolver unavailable wherever
# PyYAML is absent would be the opposite of the point: a guard that cannot run
# is a guard that lies.
registry_text = open(registry_path, encoding='utf-8').read()

def registry_list(section):
    out, inside = [], False
    for line in registry_text.splitlines():
        if line.startswith(section + ':'):
            inside = True
            continue
        if inside and line and not line[0].isspace() and not line.startswith('#'):
            break
        if inside:
            m = re.match(r'^  - (\S+)\s*$', line)
            if m:
                out.append(m.group(1))
    return out

def registry_states():
    """(id, rank, receiptPhase, requiredOutcome, applicability) in declared order."""
    out, cur = [], None
    inside = False
    for line in registry_text.splitlines():
        if line.startswith('states:'):
            inside = True
            continue
        if inside and line and not line[0].isspace() and not line.startswith('#'):
            break
        if not inside:
            continue
        m = re.match(r'^  - id: (\S+)\s*$', line)
        if m:
            cur = {'id': m.group(1), 'rank': None, 'receiptPhase': None,
                   'requiredOutcome': None, 'applicability': None, 'derivedFrom': None}
            out.append(cur)
            continue
        if cur is None:
            continue
        for key in ('rank', 'receiptPhase', 'requiredOutcome', 'applicability', 'derivedFrom'):
            m = re.match(r'^    %s: (\S+)\s*$' % key, line)
            if m:
                cur[key] = int(m.group(1)) if key == 'rank' else m.group(1)
    return out

STATES = registry_states()
if not STATES:
    print('scenario-state-resolve: registry declares no states', file=sys.stderr)
    sys.exit(2)
FORBIDDEN_KEYS = registry_list('forbiddenDeclaredKeys')
if not FORBIDDEN_KEYS:
    print('scenario-state-resolve: registry declares no forbiddenDeclaredKeys', file=sys.stderr)
    sys.exit(2)
BY_ID = {s['id']: s for s in STATES}
PHASE_TO_STATE = {s['receiptPhase']: s['id'] for s in STATES if s['receiptPhase']}

# Which traits make the two trait-derived states applicable. Kept aligned with
# bubbles/registry/proof-obligations.yaml (IMP-047 S-D): a pure calculation owes
# no live route, and an SLA-sensitive behavior owes telemetry.
LIVE_TRAITS = {'user-visible-ui', 'api-contract', 'mutable-state', 'degraded-state',
               'shared-consumer', 'dependency-path', 'responsive-accessible', 'runtime-config'}
OBSERVED_TRAITS = {'sla-sensitive'}

refusals = []
def refuse(code, scenario_id, detail, receipt=None, disposition=None,
           blocking=None, superseded_by=None):
    row = {'code': code, 'scenarioId': scenario_id, 'detail': detail}
    if receipt is not None:
        row.update({
            'ledgerIdentity': dict(ledger_identities[id(receipt[0])]),
            'disposition': disposition,
            'blocking': bool(blocking),
            'supersededBy': superseded_by,
        })
    refusals.append(row)
    return row

# --- manifest --------------------------------------------------------------
if not os.path.isfile(manifest_path):
    out = {'specDir': os.path.dirname(manifest_path), 'manifestPresent': False,
           'sourceRevision': source_revision, 'rollback': rollback,
           'scenarios': [], 'refusals': [], 'certifiable': None,
           'note': 'no scenario-manifest.json; the legacy checkbox basis still applies'}
    if fmt == 'json':
        print(json.dumps(out, indent=2))
    else:
        print('scenario-state-resolve: no scenario-manifest.json at %s' % manifest_path)
        print('  legacy basis applies; no scenario state is derived')
    sys.exit(0)

try:
    manifest = json.load(open(manifest_path, encoding='utf-8'))
except Exception as exc:
    print('scenario-state-resolve: unreadable manifest %s: %s' % (manifest_path, exc), file=sys.stderr)
    sys.exit(2)

# The legacy bare-array envelope is tolerated for already-certified specs, the
# same way every other IMP-040 reader tolerates it.
scenarios = manifest if isinstance(manifest, list) else (manifest.get('scenarios') or [])

# --- receipts --------------------------------------------------------------
receipts = []
ledger_identities = {}

def read_log_snapshot(path):
    """Read one immutable regular-file prefix and retain physical row order."""
    if not os.path.exists(path):
        return []
    try:
        fd = os.open(path, os.O_RDONLY)
    except OSError as exc:
        print('scenario-state-resolve: unreadable receipt log %s: %s' % (path, exc), file=sys.stderr)
        sys.exit(2)
    try:
        opened = os.fstat(fd)
        if not stat.S_ISREG(opened.st_mode):
            print('scenario-state-resolve: receipt log is not a regular file: %s' % path, file=sys.stderr)
            sys.exit(2)
        remaining = opened.st_size
        chunks = []
        while remaining:
            chunk = os.read(fd, min(remaining, 1024 * 1024))
            if not chunk:
                print('scenario-state-resolve: receipt log short read: %s' % path, file=sys.stderr)
                sys.exit(2)
            chunks.append(chunk)
            remaining -= len(chunk)
        snapshot = b''.join(chunks)
        after = os.fstat(fd)
        try:
            current = os.stat(path)
        except OSError as exc:
            print('scenario-state-resolve: receipt log replaced during read: %s' % exc, file=sys.stderr)
            sys.exit(2)
        if ((opened.st_dev, opened.st_ino) != (after.st_dev, after.st_ino) or
                (opened.st_dev, opened.st_ino) != (current.st_dev, current.st_ino) or
                after.st_size < opened.st_size or current.st_size < opened.st_size):
            print('scenario-state-resolve: receipt log changed before snapshot completion: %s' % path,
                  file=sys.stderr)
            sys.exit(2)
    finally:
        os.close(fd)

    physical_rows = snapshot.split(b'\n')
    if snapshot.endswith(b'\n'):
        physical_rows.pop()
    return physical_rows

for physical_ordinal, raw_bytes in enumerate(read_log_snapshot(log_path), 1):
    if not raw_bytes.strip():
        continue
    try:
        entry = json.loads(raw_bytes.decode('utf-8', errors='replace'))
    except Exception:
        continue
    if not isinstance(entry, dict):
        continue
    binding = entry.get('scenarioBinding')
    if not isinstance(binding, dict):
        continue
    append_ordinal = physical_ordinal
    ledger_identities[id(entry)] = {
        'appendOrdinal': append_ordinal,
        'rowSha256': 'sha256:' + hashlib.sha256(raw_bytes).hexdigest(),
    }
    receipts.append((entry, binding))

REQUIRED_BINDING = ['scenarioId', 'phase', 'testIdentity', 'sourceRevision', 'negativeControl']

def binding_ok(entry, binding):
    """A receipt that omits a required field is NOT weaker evidence for a state.
    It is not evidence for that state at all, and the missing field is named."""
    sid = binding.get('scenarioId') or '<unnamed>'
    ok = True
    for field in REQUIRED_BINDING:
        value = binding.get(field)
        if not (isinstance(value, str) and value.strip()):
            code = 'SCS-NO-NEGATIVE-CONTROL' if field == 'negativeControl' else 'SCS-MISSING-BINDING'
            refuse(code, sid, 'receipt for phase %r omits required binding field %r' %
                   (binding.get('phase', '<none>'), field))
            ok = False
    if not ok:
        return False
    if source_revision and binding['sourceRevision'] != source_revision:
        if binding['phase'] == 'red':
            return True
        refuse('SCS-REVISION-DRIFT', sid,
               'receipt cites source revision %s but the resolved revision is %s'
               % (binding['sourceRevision'][:12], source_revision[:12]))
        return False
    return True

candidate_receipts = []
for entry, binding in receipts:
    if not binding_ok(entry, binding):
        continue
    candidate_receipts.append((entry, binding))

def has_current_matching_implement(red_entry, red_binding):
    red_ts = red_entry.get('ts') or ''
    for entry, binding in candidate_receipts:
        if binding.get('phase') != 'implement' or entry.get('exitCode') != 0:
            continue
        if source_revision and binding.get('sourceRevision') != source_revision:
            continue
        if binding.get('scenarioId') != red_binding.get('scenarioId'):
            continue
        if binding.get('testIdentity') != red_binding.get('testIdentity'):
            continue
        if binding.get('negativeControl') != red_binding.get('negativeControl'):
            continue
        if (entry.get('ts') or '') <= red_ts:
            continue
        return True
    return False

by_scenario = {}
bound_receipts = []
for entry, binding in candidate_receipts:
    if (source_revision
            and binding.get('phase') == 'red'
            and binding.get('sourceRevision') != source_revision
            and not has_current_matching_implement(entry, binding)):
        refuse('SCS-REVISION-DRIFT', binding['scenarioId'],
               'receipt cites source revision %s but the resolved revision is %s'
               % (binding['sourceRevision'][:12], source_revision[:12]))
        continue
    bound_receipts.append((entry, binding))
    by_scenario.setdefault(binding['scenarioId'], []).append((entry, binding))

def sort_key(pair):
    return ledger_identities[id(pair[0])]['appendOrdinal']

def receipt_phase(pair):
    return pair[1].get('phase')

def receipt_succeeded(pair):
    return pair[0].get('exitCode') == 0

def proof_identity(pair):
    return (pair[1]['testIdentity'], pair[1]['negativeControl'])

def make_chain_id(scenario_id, red, implement, green):
    digest = hashlib.sha256()
    fields = [
        scenario_id,
        green[1]['sourceRevision'],
        green[1]['testIdentity'],
        green[1]['negativeControl'],
    ]
    for pair in (red, implement, green):
        identity = ledger_identities[id(pair[0])]
        fields.extend([str(identity['appendOrdinal']), identity['rowSha256']])
    for field in fields:
        encoded = field.encode('utf-8')
        digest.update(len(encoded).to_bytes(8, 'big'))
        digest.update(encoded)
    return 'sha256:' + digest.hexdigest()

# Scenario ids that anchor a proof chain of their own: they have a RED receipt
# for a given test identity. Used to tell a LEGITIMATE parallel chain (two
# scenarios that each red-then-green the same shared test) apart from a
# SUBSTITUTION (an unanchored green filed under another id and offered as proof
# for a scenario that never turned green itself).
red_anchored = set()
for entry, binding in bound_receipts:
    if binding.get('phase') == 'red' and entry.get('exitCode') != 0:
        red_anchored.add((binding['scenarioId'], binding['testIdentity']))

# --- derivation ------------------------------------------------------------
results = []
for scenario in scenarios:
    if not isinstance(scenario, dict):
        continue
    sid = scenario.get('id') or '<unnamed>'

    # THE RULE. A hand-written state is refused before anything else is read,
    # because if it were tolerated once it would be tolerated always, and the
    # model would become the bookkeeping tax it exists to remove.
    declared = [k for k in FORBIDDEN_KEYS if k in scenario]
    if declared:
        refuse('SCS-DECLARED-STATE', sid,
               'scenario declares %s; every scenario state is derived from receipts and never written by hand'
               % ', '.join(sorted(declared)))

    traits = set(scenario.get('behaviorTraits') or [])
    impl_refs = [r for r in (scenario.get('implementationRefs') or []) if isinstance(r, str)]

    applicable = {s['id'] for s in STATES if s['id'] != 'CERTIFIED'}
    if not (traits & LIVE_TRAITS):
        applicable.discard('GREEN_LIVE')
    if not (traits & OBSERVED_TRAITS):
        applicable.discard('OBSERVED')

    entries = sorted(by_scenario.get(sid, []), key=sort_key)
    by_phase = {}
    for entry, binding in entries:
        by_phase.setdefault(binding.get('phase'), []).append((entry, binding))

    disposition_rows = []
    disposition_by_entry = {}
    for pair in entries:
        row = {
            'ledgerIdentity': dict(ledger_identities[id(pair[0])]),
            'phase': receipt_phase(pair),
            'disposition': 'UNRESOLVED',
            'diagnosticCode': None,
            'blocking': False,
            'supersededBy': None,
        }
        disposition_rows.append(row)
        disposition_by_entry[id(pair[0])] = row

    def set_disposition(pair, disposition, diagnostic_code=None, blocking=False,
                        superseded_by=None):
        row = disposition_by_entry[id(pair[0])]
        row.update({
            'disposition': disposition,
            'diagnosticCode': diagnostic_code,
            'blocking': blocking,
            'supersededBy': superseded_by,
        })

    complete_candidates = []
    for green in by_phase.get('green') or []:
        if not receipt_succeeded(green):
            continue
        matching_reds = [
            red for red in (by_phase.get('red') or [])
            if not receipt_succeeded(red)
            and sort_key(red) < sort_key(green)
            and proof_identity(red) == proof_identity(green)
        ]
        for red in reversed(matching_reds):
            implementations = [
                implement for implement in (by_phase.get('implement') or [])
                if sort_key(red) < sort_key(implement) < sort_key(green)
            ]
            if implementations:
                complete_candidates.append({
                    'red': red,
                    'implement': implementations[-1],
                    'green': green,
                })
                break

    model_ordinals = [sort_key(pair) for pair in entries]
    order_conflict = (
        any(value <= 0 for value in model_ordinals)
        or len(model_ordinals) != len(set(model_ordinals))
        or any(right <= left for left, right in zip(model_ordinals, model_ordinals[1:]))
    )
    selected = None
    selected_chain = None
    selection_status = 'NO_COMPLETE_CHAIN'
    if order_conflict and entries:
        selection_status = 'ORDER_REFUSED'
        conflict_receipt = entries[-1]
        refuse(
            'SCS-APPEND-ORDER-CONFLICT', sid,
            'receipt append ordinals are missing, duplicated, nonpositive, or nonmonotonic',
            conflict_receipt, 'UNRESOLVED', True, None)
        set_disposition(conflict_receipt, 'UNRESOLVED',
                        'SCS-APPEND-ORDER-CONFLICT', True, None)
    elif complete_candidates:
        selected = max(complete_candidates, key=lambda candidate: sort_key(candidate['green']))
        selected_chain = {
            'chainId': make_chain_id(sid, selected['red'], selected['implement'], selected['green']),
            'scenarioId': sid,
            'sourceRevision': selected['green'][1]['sourceRevision'],
            'testIdentity': selected['green'][1]['testIdentity'],
            'negativeControl': selected['green'][1]['negativeControl'],
            'red': dict(ledger_identities[id(selected['red'][0])]),
            'implement': dict(ledger_identities[id(selected['implement'][0])]),
            'green': dict(ledger_identities[id(selected['green'][0])]),
        }
        selection_status = 'SELECTED'
        selected_entries = {id(selected[name][0]) for name in ('red', 'implement', 'green')}
        for pair in entries:
            if id(pair[0]) in selected_entries:
                set_disposition(pair, 'SELECTED')
            elif sort_key(pair) < sort_key(selected['green']):
                set_disposition(pair, 'SUPERSEDED', superseded_by=selected_chain['chainId'])

    def receipt_refusal(code, pair, detail, supersedable=False, direct_replacement=False):
        is_superseded = bool(
            selected_chain and supersedable and direct_replacement
            and sort_key(pair) < selected_chain['green']['appendOrdinal']
        )
        disposition = 'SUPERSEDED' if is_superseded else 'UNRESOLVED'
        blocking = not is_superseded
        superseded_by = selected_chain['chainId'] if is_superseded else None
        refuse(code, sid, detail, pair, disposition, blocking, superseded_by)
        set_disposition(pair, disposition, code, blocking, superseded_by)

    held = {'PLANNED'} if scenario.get('id') and scenario.get('requiredTestType') else set()
    blocked = set()

    red_pairs = by_phase.get('red') or []
    red_binding = None
    if not order_conflict:
        for pair in red_pairs:
            if receipt_succeeded(pair):
                directly_replaced = bool(
                    selected
                    and proof_identity(pair) == proof_identity(selected['red'])
                    and sort_key(pair) < sort_key(selected['red'])
                )
                receipt_refusal(
                    'SCS-RED-NOT-FAILING', pair,
                    'a receipt claims the red phase but exited 0, so nothing was discriminated',
                    supersedable=True, direct_replacement=directly_replaced)

        if selected is not None:
            red_binding = selected['red'][1]
            held.update({'RED_VERIFIED', 'IMPLEMENTED', 'GREEN_TARGETED'})
        else:
            failing_reds = [pair for pair in red_pairs if not receipt_succeeded(pair)]
            if failing_reds:
                active_red = failing_reds[-1]
                red_binding = active_red[1]
                held.add('RED_VERIFIED')

    # ORDERING RULE no-implementation-without-red. Without it, "the test passes"
    # is compatible with "the test always passed", and a test that always passed
    # proves nothing about the change.
    if selected is None and 'RED_VERIFIED' in held:
        eligible_implementations = [
            pair for pair in (by_phase.get('implement') or [])
            if sort_key(pair) > sort_key(active_red)
        ]
        if eligible_implementations:
            held.add('IMPLEMENTED')
            set_disposition(eligible_implementations[-1], 'SELECTED')
    elif selected is None and by_phase.get('implement'):
        blocked.add('IMPLEMENTED')

    # CROSS-SCENARIO SUBSTITUTION. A green receipt that runs THIS scenario's
    # discriminator — same test identity, same negative control — but is filed
    # under a DIFFERENT scenario id is a substitution, not a proof. It has to be
    # detected here and not inside this scenario's own receipt bucket, because
    # bucketing by scenarioId is exactly what hides the substituted id.
    #
    # A foreign green whose own scenario red-anchored the same test is a
    # legitimate parallel chain, not a substitution, so it is left alone.
    if red_binding is not None:
        for entry, binding in bound_receipts:
            if binding.get('phase') != 'green' or entry.get('exitCode') != 0:
                continue
            if binding['scenarioId'] == sid:
                continue
            if binding['testIdentity'] != red_binding['testIdentity']:
                continue
            if binding['negativeControl'] != red_binding['negativeControl']:
                continue
            if (binding['scenarioId'], binding['testIdentity']) in red_anchored:
                continue
            refuse('SCS-CROSS-SCENARIO', sid,
                   'a green receipt over this scenario\'s discriminator %r is filed under scenario %r, but the red cited %r'
                   % (red_binding['testIdentity'], binding['scenarioId'], sid))

    candidate_green_entries = {id(candidate['green'][0]) for candidate in complete_candidates}
    if not order_conflict:
        for pair in (by_phase.get('green') or []):
            if not receipt_succeeded(pair) or id(pair[0]) in candidate_green_entries:
                continue
            prior_failing_reds = [
                red for red in red_pairs
                if not receipt_succeeded(red) and sort_key(red) < sort_key(pair)
            ]
            if not prior_failing_reds:
                receipt_refusal(
                    'SCS-GREEN-WITHOUT-RED', pair,
                    'a green receipt exists with no expected-behavioral red for this scenario')
                continue
            anchor = prior_failing_reds[-1]
            if pair[1]['testIdentity'] != anchor[1]['testIdentity']:
                code = 'SCS-TEST-SUBSTITUTED'
                detail = (
                    'green receipt cites test %r, red cited %r — replacing the test requires a planning revision and a new red'
                    % (pair[1]['testIdentity'], anchor[1]['testIdentity'])
                )
            elif pair[1]['negativeControl'] != anchor[1]['negativeControl']:
                code = 'SCS-CONTROL-SUBSTITUTED'
                detail = (
                    'green receipt cites negative control %r, red cited %r'
                    % (pair[1]['negativeControl'], anchor[1]['negativeControl'])
                )
            else:
                continue
            directly_replaced = bool(
                selected
                and proof_identity(selected['red']) == proof_identity(anchor)
                and sort_key(pair) < sort_key(selected['green'])
            )
            receipt_refusal(code, pair, detail, supersedable=True,
                            direct_replacement=directly_replaced)

        if selected is None and by_phase.get('green'):
            blocked.add('GREEN_TARGETED')

    if selected is not None:
        later_failing_reds = [
            pair for pair in red_pairs
            if not receipt_succeeded(pair) and sort_key(pair) > sort_key(selected['green'])
        ]
        if later_failing_reds:
            campaign_red = later_failing_reds[0]
            later_implementations = [
                pair for pair in (by_phase.get('implement') or [])
                if sort_key(pair) > sort_key(campaign_red)
            ]
            missing_phase = 'GREEN' if later_implementations else 'IMPLEMENT'
            receipt_refusal(
                'SCS-CHAIN-PARTIAL', campaign_red,
                'campaign beginning at append ordinal %d is missing %s'
                % (sort_key(campaign_red), missing_phase))
        else:
            later_implementations = [
                pair for pair in (by_phase.get('implement') or [])
                if sort_key(pair) > sort_key(selected['green'])
            ]
            if later_implementations:
                receipt_refusal(
                    'SCS-CHAIN-PARTIAL', later_implementations[0],
                    'campaign beginning at append ordinal %d is missing RED'
                    % sort_key(later_implementations[0]))

    for phase, state_id in (('live', 'GREEN_LIVE'), ('regression', 'REGRESSION_GREEN'), ('observed', 'OBSERVED')):
        if state_id not in applicable:
            continue
        if 'GREEN_TARGETED' not in held:
            if by_phase.get(phase):
                blocked.add(state_id)
            continue
        for pair in (by_phase.get(phase) or []):
            if receipt_succeeded(pair) and (selected is None or sort_key(pair) > sort_key(selected['green'])):
                held.add(state_id)
                set_disposition(pair, 'SELECTED')
                break

    if selected is not None and any(row['blocking'] for row in disposition_rows):
        selection_status = 'SELECTED_WITH_UNRESOLVED'

    # A CHANGED implementation ref marks the scenario AFFECTED. That is what
    # makes targeted revalidation possible instead of re-certifying everything.
    affected = sorted({ref for ref in impl_refs for changed in changed_files
                       if ref == changed or changed.startswith(ref.rstrip('/') + '/')})
    if affected:
        refuse('SCS-IMPL-REF-CHANGED', sid,
               'implementation ref(s) %s changed; this scenario is AFFECTED and its green is stale'
               % ', '.join(affected))
        for state_id in ('GREEN_TARGETED', 'GREEN_LIVE', 'REGRESSION_GREEN', 'OBSERVED'):
            if state_id in held:
                held.discard(state_id)
                blocked.add(state_id)

    if rollback:
        # Advancement stops. Receipts are untouched and still reported.
        held = {'PLANNED'} & held
        blocked = set()

    ordered = [s['id'] for s in STATES if s['id'] in held]
    highest = ordered[-1] if ordered else None
    missing = sorted(applicable - held, key=lambda s: BY_ID[s]['rank'])

    result = {
        'scenarioId': sid,
        'applicableStates': sorted(applicable, key=lambda s: BY_ID[s]['rank']),
        'derivedStates': ordered,
        'highestState': highest,
        'blockedNotRun': sorted(blocked, key=lambda s: BY_ID[s]['rank']),
        'missingStates': missing,
        'receiptCount': len(entries),
        'affectedBy': affected,
    }
    if entries:
        result.update({
            'selectionStatus': selection_status,
            'selectedChain': selected_chain,
            'receiptDispositions': disposition_rows,
        })
    results.append(result)

# --- certifiability --------------------------------------------------------
# The resolver NEVER emits CERTIFIED. Certification is validate-owned, and a
# resolver that could certify would be a second certifying authority.
unsatisfied = []
for row in results:
    for want in required_states:
        if want not in row['applicableStates']:
            continue
        if want not in row['derivedStates']:
            unsatisfied.append({'scenarioId': row['scenarioId'], 'missing': want})

# A stale receipt is EXCLUDED from derivation, so it can only withhold evidence,
# never contradict it — and a scenario left without fresh evidence already lands
# in `unsatisfied`. Counting drift here would block every spec whose append-only
# log outlived a commit, which is every spec eventually.
blocking_refusals = [
    r for r in refusals
    if r.get('blocking', r['code'] != 'SCS-REVISION-DRIFT')
]
certifiable = (not blocking_refusals) and (not unsatisfied) if required_states or certifiable_mode else None

out = {
    'specDir': os.path.dirname(manifest_path),
    'manifestPresent': True,
    'sourceRevision': source_revision,
    'rollback': rollback,
    'requiredStates': required_states,
    'scenarioCount': len(results),
    'scenarios': results,
    'refusals': refusals,
    'blockingRefusalCount': len(blocking_refusals),
    'unsatisfied': unsatisfied,
    'certifiable': certifiable,
}

if fmt == 'json':
    print(json.dumps(out, indent=2))
else:
    print('scenario-state-resolve: %s' % out['specDir'])
    print('  source revision: %s' % (source_revision[:12] if source_revision else '<unresolved>'))
    if rollback:
        print('  ROLLBACK ACTIVE: state advancement stopped; receipts preserved')
    for row in results:
        print('  %s  state=%s  derived=[%s]' % (
            row['scenarioId'], row['highestState'] or 'NONE', ' '.join(row['derivedStates'])))
        if row['blockedNotRun']:
            print('      BLOCKED_NOT_RUN: %s' % ' '.join(row['blockedNotRun']))
        if row['affectedBy']:
            print('      AFFECTED by: %s' % ' '.join(row['affectedBy']))
    for r in refusals:
        if r.get('disposition') == 'SUPERSEDED':
            print('  SUPERSEDED %s [%s] receipt=%d supersededBy=%s: %s' % (
                r['code'], r['scenarioId'], r['ledgerIdentity']['appendOrdinal'],
                r['supersededBy'], r['detail']))
        else:
            print('  REFUSED %s [%s]: %s' % (r['code'], r['scenarioId'], r['detail']))
    if (refusals and not blocking_refusals
            and all(r['code'] == 'SCS-REVISION-DRIFT' for r in refusals)):
        print('  (all %d refusals are SCS-REVISION-DRIFT: superseded receipts, excluded from derivation, not blocking)' % len(refusals))
    for u in unsatisfied:
        print('  UNSATISFIED %s does not hold for %s' % (u['missing'], u['scenarioId']))
    if certifiable is not None:
        print('  certifiable: %s' % ('yes' if certifiable else 'no'))

if blocking_refusals:
    sys.exit(1)
if certifiable_mode and not certifiable:
    sys.exit(1)
sys.exit(0)
PY
exit $?
