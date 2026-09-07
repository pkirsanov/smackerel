#!/usr/bin/env bash
# bubbles/scripts/scenario-impact-resolve.sh
#
# Source-to-scenario impact resolution (IMP-040 SCOPE-9 / REG-8).
#
# WHY THIS EXISTS
# Changed-spec validation asks "which specs did this diff touch". That question
# is answered by spec-folder paths, so a diff that changes only SOURCE looks
# like it touched no spec at all — and every scenario certified against that
# source stays certified on evidence that no longer describes the code. The
# blind spot is worst exactly where it matters most: a shared consumer, whose
# single edit invalidates scenarios across many specs at once.
#
# `implementationRefs` closes it. Each scenario records the code path that owns
# its asserted result plus the consumer surfaces that render it. A diff that
# intersects those refs marks the scenario for revalidation regardless of which
# spec folder the diff touched.
#
# MATCHING IS PREFIX-AWARE, DELIBERATELY.
# A ref may name a file (`src/pricing/total.ts`), a file plus a symbol
# (`src/pricing/total.ts#computeTotal`), or a directory (`src/pricing/`). A
# directory ref matches everything beneath it. Symbol suffixes are stripped
# before comparison: the framework cannot verify which symbol a diff touched
# without a code index, and claiming otherwise would UNDER-report — the failure
# direction that leaves stale certification standing.
#
# OVER-REPORTING IS THE SAFE DIRECTION HERE and this script prefers it. A
# scenario flagged that did not need revalidation costs a re-run; a scenario
# missed keeps a false certification.
#
# Exit codes:
#   0  no certified scenario is impacted
#   1  at least one certified scenario is impacted and needs revalidation
#   2  usage error / unparseable manifest

set -uo pipefail

SPEC_DIR=""
CHANGED_FILES=()
FORMAT="human"
QUIET=0
OWNED_TEMP_DIRS=()

cleanup_owned_temp_dirs() {
  local owned_dir
  for owned_dir in "${OWNED_TEMP_DIRS[@]+"${OWNED_TEMP_DIRS[@]}"}"; do
    [ -n "$owned_dir" ] && rm -rf -- "$owned_dir"
  done
}
trap cleanup_owned_temp_dirs EXIT INT TERM

usage() {
  cat <<'EOF'
Usage: scenario-impact-resolve.sh <spec-dir> [--changed <path>]... [options]

Report which scenarios a source diff invalidates, via implementationRefs.

Options:
  --changed PATH     A repository-relative changed path (repeatable)
  --changed-from -   Read changed paths from stdin, one per line
  --format ids       Print only impacted scenario ids, one per line
  --quiet            Suppress the clean-run summary line
  -h, --help         Show this help

Exit: 0 nothing impacted | 1 impacted scenarios need revalidation | 2 usage
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --changed)
      [ "$#" -ge 2 ] || { echo "scenario-impact-resolve: --changed requires a value" >&2; exit 2; }
      CHANGED_FILES+=("$2"); shift 2 ;;
    --changed-from)
      [ "$#" -ge 2 ] || { echo "scenario-impact-resolve: --changed-from requires a value" >&2; exit 2; }
      if [ "$2" = "-" ]; then
        while IFS= read -r line; do
          [ -n "$line" ] && CHANGED_FILES+=("$line")
        done
      else
        echo "scenario-impact-resolve: --changed-from currently supports only '-' (stdin)" >&2
        exit 2
      fi
      shift 2 ;;
    --format)
      [ "$#" -ge 2 ] || { echo "scenario-impact-resolve: --format requires a value" >&2; exit 2; }
      FORMAT="$2"; shift 2 ;;
    --quiet) QUIET=1; shift ;;
    -h | --help) usage; exit 0 ;;
    --skip* | --force | --ignore* | --no-verify)
      echo "scenario-impact-resolve: '$1' is bypass-shaped and is not supported." >&2
      echo "  Stale certification is the exact failure this resolves; there is no opt-out." >&2
      exit 2 ;;
    -*)
      echo "scenario-impact-resolve: unknown option: $1" >&2; exit 2 ;;
    *)
      if [ -n "$SPEC_DIR" ]; then
        echo "scenario-impact-resolve: unexpected argument: $1" >&2; exit 2
      fi
      SPEC_DIR="$1"; shift ;;
  esac
done

[ -n "$SPEC_DIR" ] || { usage >&2; exit 2; }
[ -d "$SPEC_DIR" ] || { echo "scenario-impact-resolve: spec dir not found: $SPEC_DIR" >&2; exit 2; }

MANIFEST="$SPEC_DIR/scenario-manifest.json"
if [ ! -f "$MANIFEST" ]; then
  [ "$QUIET" = "1" ] || echo "[scenario-impact-resolve] OK — no scenario-manifest.json (inert)"
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REFERENCE_READER="$SCRIPT_DIR/scenario-reference-reader.py"
[ -f "$REFERENCE_READER" ] || {
  echo "scenario-impact-resolve: scenario-reference-reader.py is required" >&2
  exit 2
}
REPO_ROOT="$(cd "$SPEC_DIR" && git rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$REPO_ROOT" ] || REPO_ROOT="$SPEC_DIR"
PROJECTION_TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/scenario-impact-resolve.XXXXXX")" || {
  echo "scenario-impact-resolve: could not create private projection directory" >&2
  exit 2
}
OWNED_TEMP_DIRS+=("$PROJECTION_TEMP_DIR")
chmod 700 "$PROJECTION_TEMP_DIR" || exit 2
REFERENCE_PROJECTION_FILE="$PROJECTION_TEMP_DIR/reference-projection.json"
if ! (umask 077 && python3 "$REFERENCE_READER" "$MANIFEST" --repo-root "$REPO_ROOT" >"$REFERENCE_PROJECTION_FILE"); then
  exit 2
fi

CHANGED_JOINED="$(printf '%s\n' "${CHANGED_FILES[@]+"${CHANGED_FILES[@]}"}")"

CHANGED="$CHANGED_JOINED" FORMAT="$FORMAT" QUIET="$QUIET" \
  python3 - "$REFERENCE_PROJECTION_FILE" <<'PY'
import json, os, sys

fmt = os.environ.get("FORMAT", "human")
quiet = os.environ.get("QUIET") == "1"
changed = [c.strip() for c in os.environ.get("CHANGED", "").splitlines() if c.strip()]

try:
  with open(sys.argv[1], "r", encoding="utf-8") as projection_file:
    projection = json.load(projection_file)
except (IndexError, OSError, ValueError) as exc:
    print(f"scenario-impact-resolve: invalid scenario-reference-reader projection: {exc}", file=sys.stderr)
    sys.exit(2)

def normalise(ref):
    """Strip a symbol suffix and any leading ./ so refs and diff paths compare."""
    ref = ref.split("#", 1)[0].strip()
    while ref.startswith("./"):
        ref = ref[2:]
    return ref

def intersects(ref, path):
    ref = normalise(ref)
    if not ref:
        return False
    if ref.endswith("/"):
        return path.startswith(ref)
    # An exact file match, or a directory ref written without its slash.
    return path == ref or path.startswith(ref + "/")

# A scenario is CERTIFIED when it carries evidence or an explicit lockdown.
# Both are claims that the behavior was proven at a point in time, and both are
# what a source change can silently invalidate.
def is_certified(scenario):
  metadata = scenario.get("metadata")
  if isinstance(metadata, dict) and metadata.get("lockdown"):
    return True
  refs = scenario.get("evidenceRefs")
  return isinstance(refs, list) and len(refs) > 0

# The canonical reader owns envelopes, versions, aliases, identities,
# references, duplicate members, and strict-v2 closed-object validation.
scenarios = projection.get("scenarios")
if not isinstance(scenarios, list):
    print("scenario-impact-resolve: invalid scenario-reference-reader projection", file=sys.stderr)
    sys.exit(2)

impacted = []
declared = 0

for scenario in scenarios:
    if not isinstance(scenario, dict):
        print("scenario-impact-resolve: invalid scenario-reference-reader projection", file=sys.stderr)
        sys.exit(2)
    refs = scenario.get("implementationRefs")
    if not isinstance(refs, list) or not refs:
        continue
    declared += 1
    sid = scenario.get("scenarioId")
    hits = sorted({
        path
        for path in changed
        for ref in refs
        if isinstance(ref, str) and intersects(ref, path)
    })
    if hits and is_certified(scenario):
        impacted.append((sid, hits))

if fmt == "ids":
    for sid, _ in impacted:
        print(sid)
    sys.exit(1 if impacted else 0)

if impacted:
    print("scenario-impact-resolve: certified scenarios intersect this diff (REG-8)",
          file=sys.stderr)
    for sid, hits in impacted:
        print(f"  REVALIDATE: {sid}", file=sys.stderr)
        for hit in hits:
            print(f"    changed: {hit}", file=sys.stderr)
    print("", file=sys.stderr)
    print(f"scenario-impact-resolve: {len(impacted)} scenario(s) need revalidation.",
          file=sys.stderr)
    sys.exit(1)

if not quiet:
    if declared:
        print(f"[scenario-impact-resolve] OK — {declared} scenario(s) carry "
              "implementationRefs, none intersect this diff")
    else:
        print("[scenario-impact-resolve] OK — no implementationRefs declared (inert)")
sys.exit(0)
PY
