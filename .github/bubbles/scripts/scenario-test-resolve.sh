#!/usr/bin/env bash
set -euo pipefail

# scenario-test-resolve.sh
#
# IMP-040 SCOPE-2 — resolve every scenario-manifest linked test to a REAL target.
#
# WHY THIS EXISTS
# Gate G057 promises that each scenario maps to real live-system coverage. What
# it actually did was count `"linkedTests"` FIELDS: `guards/control-plane-checks.sh`
# asserted only that the count was non-zero, so the string had to appear once
# anywhere in the file. BUG-030 reproduced the consequence — three Playwright
# titles that existed in no test file certified clean.
#
# Field presence and file existence do not satisfy that contract. This resolver
# opens the referenced file and resolves the referenced title.
#
# FOUR REFERENCE SHAPES ARE LIVE, and all four must keep working:
#   "tests/foo.spec.ts"                     plain path, NO title
#   "tests/foo.spec.ts#exact title"         path + title (the repository string form)
#   {"file": "tests/foo.spec.ts"}           object, NO title
#   {"file": "...", "testId": "..."}        object + testId (CONTROL_PLANE_SCHEMAS.md)
# A title is resolved ONLY when one is actually declared. A bare path is
# file-existence-only. Enforcing titles unconditionally would fail every
# existing packet and turn G057 into a false-block machine, which carries no
# more information than the dead gate it replaced.
#
# TWO FIELD SPELLINGS ARE LIVE for the scenario id. The JSON schema says `id`;
# CONTROL_PLANE_SCHEMAS.md and the current guard say `scenarioId`. Both are
# accepted here rather than picking a side, because a repo that follows either
# document is not wrong.
#
# WITHOUT AN INVENTORY ADAPTER (testDiscovery.adapter: none, the default) the
# title is resolved by a conservative literal scan of the referenced file. That
# still catches BUG-030's absent titles with no runner. The runner-category
# comparison is NOT APPLICABLE in that configuration — no runner was ever
# declared, so there is nothing to execute and nothing to be unknown about.
#
# IMP-047 PD-04 — THE FALSE-PASS THIS FILE USED TO SHIP.
# The two states below are not the same state, and collapsing them is what made
# a unit test offered as E2E certify clean:
#
#   adapter: none              -> category comparison NOT APPLICABLE  -> exit 0
#   adapter DECLARED, unable   -> category comparison UNKNOWN         -> exit 3
#     to execute (no timeout
#     implementation, command
#     failed, bad contract,
#     unparseable inventory)
#
# The repository that declares `testDiscovery.adapter: command` has asked for
# category enforcement. When that runner cannot be executed the answer is
# UNKNOWN, and an unknown result is never a pass. Previously the invocation used
# a bare `timeout`, which does not exist on a stock macOS PATH; the command
# substitution failed, the resolver silently degraded to the literal scan, and
# printed OK. The mechanism built to prevent category fraud reported success
# precisely when it could not run. It now routes through
# `bubbles_run_with_timeout` (guard-lib.sh), which prefers `timeout`, then
# `gtimeout`, then a bash watchdog — so the adapter actually runs — and fails
# loud on exit 3 when it still cannot.
#
# Title resolution still falls back to the literal scan even in the unknown
# case, so findings stay accurate: an inventory that failed must never be read
# as "no tests exist" and manufacture MISSING-TITLE against a title that is
# genuinely present in the file.
#
# Exit codes:
#   0  every reference resolved (or nothing to resolve)
#   1  one or more references failed to resolve
#   2  usage error / unreadable manifest
#   3  a declared inventory adapter could not be executed while at least one
#      checked reference declares a requiredTestType (category UNKNOWN)

SPEC_DIR=""
REPO_ROOT=""
QUIET=0

die_usage() {
  printf 'scenario-test-resolve: %s\n' "$1" >&2
  printf 'usage: scenario-test-resolve.sh <specDir> [--repo-root DIR] [--quiet]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root) shift; REPO_ROOT="${1:-}" ;;
    --quiet) QUIET=1 ;;
    -h|--help) sed -n '4,71p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported; fix the reference instead" ;;
    -*) die_usage "unknown option '$1'" ;;
    *) [[ -z "$SPEC_DIR" ]] || die_usage "unexpected argument '$1'"; SPEC_DIR="$1" ;;
  esac
  shift
done

[[ -n "$SPEC_DIR" ]] || die_usage "a spec directory is required"
[[ -d "$SPEC_DIR" ]] || die_usage "spec directory not found: $SPEC_DIR"
SPEC_DIR="$(cd "$SPEC_DIR" && pwd)"

if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "$SPEC_DIR" && git rev-parse --show-toplevel 2>/dev/null || true)"
  [[ -n "$REPO_ROOT" ]] || REPO_ROOT="$SPEC_DIR"
fi
[[ -d "$REPO_ROOT" ]] || die_usage "repo root not found: $REPO_ROOT"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

MANIFEST="$SPEC_DIR/scenario-manifest.json"
if [[ ! -f "$MANIFEST" ]]; then
  [[ "$QUIET" -eq 1 ]] || printf '[scenario-test-resolve] NA — no scenario-manifest.json in %s\n' "${SPEC_DIR#"$REPO_ROOT"/}"
  exit 0
fi

command -v python3 >/dev/null 2>&1 || die_usage "python3 is required to parse scenario-manifest.json"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# PD-04: the portable timeout helper. A bare `timeout` does not exist on a stock
# macOS PATH, and its absence used to silently disable category enforcement.
# shellcheck source=bubbles/scripts/guard-lib.sh
. "$SCRIPT_DIR/guard-lib.sh"

# Resolve the inventory adapter. Three distinct states, never collapsed:
#   CATEGORY_STATE=not-applicable  no adapter declared; nothing to execute
#   CATEGORY_STATE=available       the inventory ran and can report categories
#   CATEGORY_STATE=unexecutable    an adapter IS declared but could not run
INVENTORY_JSON=""
ADAPTER="none"
CATEGORY_STATE="not-applicable"
CATEGORY_REASON=""
if [[ -x "$SCRIPT_DIR/test-inventory-resolve.sh" ]]; then
  resolved="$(bash "$SCRIPT_DIR/test-inventory-resolve.sh" --repo-root "$REPO_ROOT" 2>/dev/null || true)"
  ADAPTER="$(printf '%s\n' "$resolved" | sed -n 's/^adapter=//p' | head -n 1)"
  [[ -n "$ADAPTER" ]] || ADAPTER="none"
  if [[ "$ADAPTER" == "command" ]]; then
    inv_cmd="$(printf '%s\n' "$resolved" | sed -n 's/^command=//p' | head -n 1)"
    inv_timeout="$(printf '%s\n' "$resolved" | sed -n 's/^timeoutSeconds=//p' | head -n 1)"
    if [[ -n "$inv_cmd" ]]; then
      # A failing or slow inventory must not silently become "no tests exist":
      # an empty inventory would fail every declared title. Title resolution
      # therefore still degrades to the literal scan. What does NOT degrade is
      # the category verdict — a declared adapter that cannot run leaves the
      # category UNKNOWN, and the Python stage below refuses on exit 3.
      CATEGORY_STATE="unexecutable"
      CATEGORY_REASON="the declared inventory adapter did not produce a usable inventory"
      inv_rc=0
      INVENTORY_JSON="$(cd "$REPO_ROOT" && bubbles_run_with_timeout "${inv_timeout:-120}" "$inv_cmd" 2>/dev/null)" || inv_rc=$?
      if [[ "$inv_rc" -eq 0 ]]; then
        CATEGORY_STATE="available"
        CATEGORY_REASON=""
      else
        INVENTORY_JSON=""
        ADAPTER="none"
        if [[ "$inv_rc" -eq 124 ]]; then
          CATEGORY_REASON="the declared inventory adapter timed out after ${inv_timeout:-120}s"
        else
          CATEGORY_REASON="the declared inventory adapter exited ${inv_rc}"
        fi
        printf 'scenario-test-resolve: WARN inventory adapter failed; falling back to literal scan\n' >&2
      fi
    else
      CATEGORY_STATE="unexecutable"
      CATEGORY_REASON="testDiscovery.adapter is 'command' but no command is configured"
    fi
  fi
fi

SPEC_DIR="$SPEC_DIR" REPO_ROOT="$REPO_ROOT" ADAPTER="$ADAPTER" \
  CATEGORY_STATE="$CATEGORY_STATE" CATEGORY_REASON="$CATEGORY_REASON" \
  INVENTORY_JSON="$INVENTORY_JSON" QUIET="$QUIET" python3 - <<'PY'
import json, os, sys

spec_dir = os.environ["SPEC_DIR"]
repo_root = os.environ["REPO_ROOT"]
adapter = os.environ["ADAPTER"]
inventory_raw = os.environ.get("INVENTORY_JSON", "")
quiet = os.environ.get("QUIET") == "1"
# PD-04: not-applicable | available | unexecutable. See the header.
category_state = os.environ.get("CATEGORY_STATE", "not-applicable")
category_reason = os.environ.get("CATEGORY_REASON", "")

manifest_path = os.path.join(spec_dir, "scenario-manifest.json")
try:
    with open(manifest_path, encoding="utf-8") as fh:
        manifest = json.load(fh)
except (OSError, ValueError) as exc:
    print(f"scenario-test-resolve: cannot parse {manifest_path}: {exc}", file=sys.stderr)
    sys.exit(2)

# The canonical taxonomy. A scenario whose required type is LIVE cannot be
# satisfied by a test the runner classifies as mocked — that substitution is
# precisely what makes a green suite meaningless.
LIVE = {"integration", "e2e-api", "e2e-ui", "stress", "load"}
MOCKED = {"unit", "ui-unit", "functional"}

inventory = []
if adapter == "command" and inventory_raw.strip():
    try:
        doc = json.loads(inventory_raw)
        if isinstance(doc, dict):
            version = str(doc.get("contractVersion", ""))
            if not version.startswith("bubbles-test-inventory/"):
                print(f"scenario-test-resolve: WARN inventory contractVersion '{version}' "
                      "is not bubbles-test-inventory/*; falling back to literal scan", file=sys.stderr)
                category_state = "unexecutable"
                category_reason = (f"the declared inventory adapter returned contractVersion "
                                   f"'{version}', which is not bubbles-test-inventory/*")
            else:
                got = doc.get("tests")
                inventory = got if isinstance(got, list) else []
        else:
            category_state = "unexecutable"
            category_reason = "the declared inventory adapter did not return an inventory object"
    except ValueError as exc:
        print(f"scenario-test-resolve: WARN inventory is not valid JSON ({exc}); "
              "falling back to literal scan", file=sys.stderr)
        category_state = "unexecutable"
        category_reason = f"the declared inventory adapter returned invalid JSON ({exc})"

use_inventory = bool(inventory)
if category_state == "available" and not use_inventory:
    # A declared adapter that ran but reported no tests cannot answer a category
    # question either. Unknown, not clean.
    category_state = "unexecutable"
    category_reason = category_reason or "the declared inventory adapter reported no tests"

def normalize(ref):
    """Return (file, title) for any of the four live reference shapes."""
    if isinstance(ref, str):
        text = ref.strip()
        if not text:
            return (None, None)
        if "#" in text:
            path, title = text.split("#", 1)
            return (path.strip(), title.strip() or None)
        return (text, None)
    if isinstance(ref, dict):
        path = ref.get("file") or ref.get("path")
        title = ref.get("title") or ref.get("testId") or ref.get("name")
        path = path.strip() if isinstance(path, str) else None
        title = title.strip() if isinstance(title, str) and title.strip() else None
        return (path, title)
    return (None, None)

def is_sentinel(value):
    # Packets use __FUTURE_TEST__ to mean "planned, not yet written". Treating a
    # declared placeholder as a missing file would block planning packets.
    return bool(value) and value.startswith("__") and value.endswith("__")

findings = []
unresolved_category = []
checked = 0
scanned_titles = 0
skipped_category = 0

scenarios = manifest.get("scenarios") if isinstance(manifest, dict) else manifest
if not isinstance(scenarios, list):
    scenarios = []

for scenario in scenarios:
    if not isinstance(scenario, dict):
        continue
    # Both spellings are live: the JSON schema says `id`, the schema guide and
    # the current guard say `scenarioId`.
    sid = scenario.get("id") or scenario.get("scenarioId") or "<unidentified-scenario>"
    required = scenario.get("requiredTestType")
    refs = scenario.get("linkedTests")
    if not isinstance(refs, list):
        continue

    for ref in refs:
        rel, title = normalize(ref)
        if rel is None or is_sentinel(rel):
            continue
        checked += 1
        abs_path = os.path.normpath(os.path.join(repo_root, rel))
        # A reference must not escape the bound repository root.
        if not abs_path.startswith(repo_root + os.sep) and abs_path != repo_root:
            findings.append((sid, rel, "OUTSIDE-REPO",
                             "linked test path escapes the repository root"))
            continue
        if not os.path.isfile(abs_path):
            findings.append((sid, rel, "MISSING-FILE",
                             "no such file under the repository root"))
            continue
        if title is None or is_sentinel(title):
            continue

        if use_inventory:
            matches = [t for t in inventory
                       if isinstance(t, dict)
                       and str(t.get("title", "")) == title
                       and (not t.get("file") or os.path.normpath(str(t["file"])) == os.path.normpath(rel))]
            if not matches:
                findings.append((sid, f"{rel}#{title}", "MISSING-TITLE",
                                 "the inventory declares no test with this exact title"))
                continue
            if len(matches) > 1:
                findings.append((sid, f"{rel}#{title}", "AMBIGUOUS-TITLE",
                                 f"{len(matches)} tests share this title; a reference must resolve to exactly one"))
                continue
            category = str(matches[0].get("category", "")) or None
            if required and not category:
                # PD-04: the inventory ran but declares no category for this
                # test. The comparison is applicable and unperformed, which is
                # UNKNOWN — the same false-PASS shape as a missing runner.
                unresolved_category.append((sid, f"{rel}#{title}", required))
                if not category_reason:
                    category_reason = ("the inventory resolved this test but declares no "
                                       "category for it")
                continue
            if required and category:
                if required in LIVE and category in MOCKED:
                    findings.append((sid, f"{rel}#{title}", "CATEGORY-MISMATCH",
                                     f"requiredTestType '{required}' is live-system but the runner "
                                     f"classifies this test as '{category}'"))
                    continue
                if required != category and required in LIVE and category in LIVE:
                    findings.append((sid, f"{rel}#{title}", "CATEGORY-MISMATCH",
                                     f"requiredTestType '{required}' does not match the runner category '{category}'"))
                    continue
        else:
            # Conservative literal scan. It cannot report a category, so the
            # category comparison is skipped rather than guessed.
            try:
                with open(abs_path, encoding="utf-8", errors="replace") as fh:
                    body = fh.read()
            except OSError as exc:
                findings.append((sid, f"{rel}#{title}", "UNREADABLE-FILE", str(exc)))
                continue
            occurrences = body.count(title)
            if occurrences == 0:
                findings.append((sid, f"{rel}#{title}", "MISSING-TITLE",
                                 "the referenced file contains no test with this exact title"))
                continue
            if occurrences > 1:
                findings.append((sid, f"{rel}#{title}", "AMBIGUOUS-TITLE",
                                 f"the title appears {occurrences} times; a reference must resolve to exactly one"))
                continue
            scanned_titles += 1
            if required:
                skipped_category += 1
                # PD-04: this reference WOULD have been category-compared had the
                # inventory been usable. Record it only where the comparison was
                # actually applicable — a declared requiredTestType plus a
                # declared title — so the refusal mirrors the check it replaces
                # and never over-reaches to bare paths.
                if category_state == "unexecutable":
                    unresolved_category.append((sid, f"{rel}#{title}", required))

if unresolved_category:
    # An applicable category comparison that could not be executed is UNKNOWN.
    # Reporting clean here is the exact false-PASS PD-04 exists to remove. This
    # block is printed even when `findings` is non-empty, so an unknown category
    # is never silently swallowed by a louder failure.
    print("scenario-test-resolve: FAIL — an applicable test category could not be "
          "executed (Gate G057 / IMP-047 PD-04)", file=sys.stderr)
    print(f"  reason: {category_reason or 'the declared inventory adapter could not be executed'}",
          file=sys.stderr)
    for sid, ref, required in unresolved_category:
        print(f"  CATEGORY-UNRESOLVED: {sid} -> {ref}", file=sys.stderr)
        print(f"    requiredTestType '{required}' is declared but no runner category could be "
              "obtained; an unmeasured category is UNKNOWN, never a pass", file=sys.stderr)
    print("", file=sys.stderr)
    print("scenario-test-resolve: repair the declared testDiscovery adapter, or set "
          "testDiscovery.adapter to 'none' to declare that this repository does not "
          "enforce runner categories.", file=sys.stderr)
    print(f"scenario-test-resolve: {len(unresolved_category)} unresolved categor(y/ies) "
          f"of {checked} checked.", file=sys.stderr)

if findings:
    print("scenario-test-resolve: FAIL — linked tests that do not resolve (Gate G057)", file=sys.stderr)
    for sid, ref, code, detail in findings:
        print(f"  {code}: {sid} -> {ref}", file=sys.stderr)
        print(f"    {detail}", file=sys.stderr)
    print("", file=sys.stderr)
    print(f"scenario-test-resolve: {len(findings)} unresolved reference(s) of {checked} checked.", file=sys.stderr)
    sys.exit(1)

if unresolved_category:
    sys.exit(3)

if not quiet:
    mode = "inventory" if use_inventory else "literal-scan"
    extra = ""
    if skipped_category:
        # Reachable only when NO adapter was declared. A declared-but-unusable
        # adapter exits 3 above; this line can no longer describe a false pass.
        extra = (f"; {skipped_category} category comparison(s) not applicable "
                 "(no test-discovery adapter declared)")
    print(f"[scenario-test-resolve] OK — {checked} reference(s) resolved via {mode}{extra}")
sys.exit(0)
PY
