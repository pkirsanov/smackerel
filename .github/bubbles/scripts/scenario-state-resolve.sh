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
import json, os, re, sys

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
def refuse(code, scenario_id, detail):
    refusals.append({'code': code, 'scenarioId': scenario_id, 'detail': detail})

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
if os.path.isfile(log_path):
    for raw in open(log_path, encoding='utf-8', errors='replace'):
        raw = raw.strip()
        if not raw:
            continue
        try:
            entry = json.loads(raw)
        except Exception:
            continue
        if not isinstance(entry, dict):
            continue
        binding = entry.get('scenarioBinding')
        if not isinstance(binding, dict):
            continue
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
        refuse('SCS-REVISION-DRIFT', sid,
               'receipt cites source revision %s but the resolved revision is %s'
               % (binding['sourceRevision'][:12], source_revision[:12]))
        return False
    return True

by_scenario = {}
bound_receipts = []
for entry, binding in receipts:
    if not binding_ok(entry, binding):
        continue
    bound_receipts.append((entry, binding))
    by_scenario.setdefault(binding['scenarioId'], []).append((entry, binding))

def sort_key(pair):
    return pair[0].get('ts') or ''

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

    held = {'PLANNED'} if scenario.get('id') and scenario.get('requiredTestType') else set()
    blocked = set()

    red_pairs = by_phase.get('red') or []
    red_binding = None
    for entry, binding in red_pairs:
        if entry.get('exitCode') == 0:
            refuse('SCS-RED-NOT-FAILING', sid,
                   'a receipt claims the red phase but exited 0, so nothing was discriminated')
            continue
        red_binding = binding
        held.add('RED_VERIFIED')
        break

    # ORDERING RULE no-implementation-without-red. Without it, "the test passes"
    # is compatible with "the test always passed", and a test that always passed
    # proves nothing about the change.
    if 'RED_VERIFIED' in held:
        if by_phase.get('implement'):
            held.add('IMPLEMENTED')
    elif by_phase.get('implement'):
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

    if 'IMPLEMENTED' in held:
        for entry, binding in (by_phase.get('green') or []):
            if entry.get('exitCode') != 0:
                continue
            if binding['testIdentity'] != red_binding['testIdentity']:
                refuse('SCS-TEST-SUBSTITUTED', sid,
                       'green receipt cites test %r, red cited %r — replacing the test requires a planning revision and a new red'
                       % (binding['testIdentity'], red_binding['testIdentity']))
                continue
            if binding['negativeControl'] != red_binding['negativeControl']:
                refuse('SCS-CONTROL-SUBSTITUTED', sid,
                       'green receipt cites negative control %r, red cited %r'
                       % (binding['negativeControl'], red_binding['negativeControl']))
                continue
            held.add('GREEN_TARGETED')
            break
    elif by_phase.get('green'):
        if 'RED_VERIFIED' not in held:
            refuse('SCS-GREEN-WITHOUT-RED', sid,
                   'a green receipt exists with no expected-behavioral red for this scenario')
        blocked.add('GREEN_TARGETED')

    for phase, state_id in (('live', 'GREEN_LIVE'), ('regression', 'REGRESSION_GREEN'), ('observed', 'OBSERVED')):
        if state_id not in applicable:
            continue
        if 'GREEN_TARGETED' not in held:
            if by_phase.get(phase):
                blocked.add(state_id)
            continue
        for entry, _binding in (by_phase.get(phase) or []):
            if entry.get('exitCode') == 0:
                held.add(state_id)
                break

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

    results.append({
        'scenarioId': sid,
        'applicableStates': sorted(applicable, key=lambda s: BY_ID[s]['rank']),
        'derivedStates': ordered,
        'highestState': highest,
        'blockedNotRun': sorted(blocked, key=lambda s: BY_ID[s]['rank']),
        'missingStates': missing,
        'receiptCount': len(entries),
        'affectedBy': affected,
    })

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
blocking_refusals = [r for r in refusals if r['code'] != 'SCS-REVISION-DRIFT']
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
        print('  REFUSED %s [%s]: %s' % (r['code'], r['scenarioId'], r['detail']))
    if refusals and not blocking_refusals:
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
