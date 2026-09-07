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
# Authored linkedTests and plannedTests are distinct states. Four authored
# reference shapes are live, and all four must keep working:
#   "tests/foo.spec.ts"                     plain path, NO title
#   "tests/foo.spec.ts#exact title"         path + title (the repository string form)
#   {"file": "tests/foo.spec.ts"}           object, NO title
#   {"file": "...", "testId": "..."}        object + testId (CONTROL_PLANE_SCHEMAS.md)
# A title is resolved ONLY when one is actually declared. A bare path is
# file-existence-only. Enforcing titles unconditionally would fail every
# existing packet and turn G057 into a false-block machine, which carries no
# more information than the dead gate it replaced.
#
# New producers write plannedTests objects with path/title/type. Readers retain
# compatibility with legacy __FUTURE_TEST__ sentinels and linkedTests objects
# carrying testState:"planned-not-authored". Those forms are always planned,
# never authored. Planned paths are normalized and containment-checked
# lexically, but they need not exist. Authored paths must name an existing
# regular file reached by repository-rooted descriptor walking. Every authored
# intermediate or final symlink is refused, regardless of where it points.
#
# TWO FIELD SPELLINGS ARE LIVE for the scenario id. The JSON schema says `id`;
# CONTROL_PLANE_SCHEMAS.md and the current guard say `scenarioId`. Both are
# accepted here rather than picking a side, because a repo that follows either
# document is not wrong.
#
# WITHOUT AN INVENTORY ADAPTER (testDiscovery.adapter: none, the default) the
# title is resolved by a conservative structural scan of the referenced file.
# The bounded fallback grammar accepts only literal declarations: JS/TS
# test/it/describe (including .only/.skip/.todo), test.each(cases)("title"),
# Deno.test("title"), Python test_* functions with optional decorators, and
# Rust functions immediately governed by #[test] or #[tokio::test] with
# optional intervening attributes. Dynamic/computed titles and every other
# runner form require a test-discovery inventory. Comments, documentation,
# fixture strings, and fenced examples are never declarations. The runner-category
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
REFERENCE_READER="$SCRIPT_DIR/scenario-reference-reader.py"
[[ -f "$REFERENCE_READER" ]] || die_usage "scenario-reference-reader.py is required"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/scenario-test-resolve.XXXXXXXX")" || die_usage "cannot create temporary work directory"
WORK_DIR_IDENTITY="$(python3 - "$WORK_DIR" <<'PY'
import os, stat, sys
path = sys.argv[1]
metadata = os.lstat(path)
if not stat.S_ISDIR(metadata.st_mode) or stat.S_ISLNK(metadata.st_mode):
    raise SystemExit(1)
print("%s\t%s\t%s" % (os.path.realpath(os.path.dirname(path)), metadata.st_dev, metadata.st_ino))
PY
)" || die_usage "cannot establish temporary work directory identity"
cleanup() {
  python3 - "$WORK_DIR" "$WORK_DIR_IDENTITY" <<'PY'
import os, shutil, stat, sys
path, identity = sys.argv[1:]
expected_parent, expected_device, expected_inode = identity.split("\t")
try:
    metadata = os.lstat(path)
except FileNotFoundError:
    raise SystemExit(0)
valid = (os.path.realpath(os.path.dirname(path)) == expected_parent
         and stat.S_ISDIR(metadata.st_mode)
         and not stat.S_ISLNK(metadata.st_mode)
         and metadata.st_dev == int(expected_device)
         and metadata.st_ino == int(expected_inode))
if valid:
    shutil.rmtree(path)
PY
}
on_int() { trap - INT; cleanup; exit 130; }
on_term() { trap - TERM; cleanup; exit 143; }
trap cleanup EXIT
trap on_int INT
trap on_term TERM
REFERENCE_FILE="$WORK_DIR/reference.json"
INVENTORY_FILE="$WORK_DIR/inventory.json"
RESOLUTION_FILE="$WORK_DIR/inventory-resolution.txt"
RESOLUTION_ERROR_FILE="$WORK_DIR/inventory-resolution.err"
: >"$INVENTORY_FILE"
if ! BUBBLES_SCENARIO_TEST_RESOLVE_WORK_DIR="$WORK_DIR" \
    python3 "$REFERENCE_READER" "$MANIFEST" --repo-root "$REPO_ROOT" >"$REFERENCE_FILE"; then
    exit 2
fi

# PD-04: the portable timeout helper. A bare `timeout` does not exist on a stock
# macOS PATH, and its absence used to silently disable category enforcement.
# shellcheck source=bubbles/scripts/guard-lib.sh
. "$SCRIPT_DIR/guard-lib.sh"

# Resolve the inventory adapter. Three distinct states, never collapsed:
#   CATEGORY_STATE=not-applicable  no adapter declared; nothing to execute
#   CATEGORY_STATE=available       the inventory ran and can report categories
#   CATEGORY_STATE=unexecutable    an adapter IS declared but could not run
ADAPTER="none"
CATEGORY_STATE="not-applicable"
CATEGORY_REASON=""
if [[ -x "$SCRIPT_DIR/test-inventory-resolve.sh" ]]; then
    resolution_rc=0
    bash "$SCRIPT_DIR/test-inventory-resolve.sh" --repo-root "$REPO_ROOT" \
        >"$RESOLUTION_FILE" 2>"$RESOLUTION_ERROR_FILE" || resolution_rc=$?
    if [[ "$resolution_rc" -ne 0 ]]; then
        # The resolver can emit a prefix such as `adapter=command` before it
        # discovers a missing command, unsafe path, or malformed timeout. Its
        # stdout is therefore a transaction: consume all of it only after exit 0,
        # and consume none of it after failure.
        ADAPTER="unresolved"
        CATEGORY_STATE="unexecutable"
        CATEGORY_REASON="test-inventory-resolve.sh exited ${resolution_rc}; its partial output was discarded"
        printf 'scenario-test-resolve: WARN test inventory configuration could not be resolved; falling back to literal scan for titles\n' >&2
    else
        resolved="$(cat "$RESOLUTION_FILE")"
        ADAPTER="$(printf '%s\n' "$resolved" | sed -n 's/^adapter=//p' | head -n 1)"
        [[ -n "$ADAPTER" ]] || ADAPTER="none"
    fi
    if [[ "$resolution_rc" -eq 0 && "$ADAPTER" == "command" ]]; then
        inv_cmd="$(printf '%s\n' "$resolved" | sed -n 's/^command=//p' | head -n 1)"
        inv_relative="$(printf '%s\n' "$resolved" | sed -n 's/^commandRelative=//p' | head -n 1)"
        inv_device="$(printf '%s\n' "$resolved" | sed -n 's/^commandDevice=//p' | head -n 1)"
        inv_inode="$(printf '%s\n' "$resolved" | sed -n 's/^commandInode=//p' | head -n 1)"
        inv_mode="$(printf '%s\n' "$resolved" | sed -n 's/^commandMode=//p' | head -n 1)"
        inv_size="$(printf '%s\n' "$resolved" | sed -n 's/^commandSize=//p' | head -n 1)"
        inv_mtime_ns="$(printf '%s\n' "$resolved" | sed -n 's/^commandMtimeNs=//p' | head -n 1)"
        inv_sha256="$(printf '%s\n' "$resolved" | sed -n 's/^commandSha256=//p' | head -n 1)"
        inv_timeout="$(printf '%s\n' "$resolved" | sed -n 's/^timeoutSeconds=//p' | head -n 1)"
    if [[ -n "$inv_cmd" && -n "$inv_relative" ]]; then
      # A failing or slow inventory must not silently become "no tests exist":
      # an empty inventory would fail every declared title. Title resolution
      # therefore still degrades to the literal scan. What does NOT degrade is
      # the category verdict — a declared adapter that cannot run leaves the
      # category UNKNOWN, and the Python stage below refuses on exit 3.
      CATEGORY_STATE="unexecutable"
      CATEGORY_REASON="the declared inventory adapter did not produce a usable inventory"
      inv_rc=0
            (cd "$REPO_ROOT" && bubbles_run_with_timeout "${inv_timeout:-120}" python3 -c '
import hashlib, os, stat, sys

root, relative, display_path, device, inode, mode, size, mtime_ns, expected_digest = sys.argv[1:]
parts = relative.split("/")
opened = []
try:
    if not parts or any(part in ("", ".", "..") for part in parts):
        raise OSError("inventory executable has an invalid repository-relative path")
    if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_DIRECTORY"):
        raise OSError("no-follow descriptor walking is unavailable")
    parent = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    opened.append(parent)
    for component in parts[:-1]:
        child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                        dir_fd=parent)
        opened.append(child)
        if not stat.S_ISDIR(os.fstat(child).st_mode):
            raise OSError("inventory executable intermediate component is not a directory")
        parent = child
    descriptor = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW, dir_fd=parent)
    opened.append(descriptor)
    metadata = os.fstat(descriptor)
    observed = (metadata.st_dev, metadata.st_ino, metadata.st_mode & 0o7777,
                metadata.st_size, metadata.st_mtime_ns)
    expected = tuple(int(value) for value in (device, inode, mode, size, mtime_ns))
    if not stat.S_ISREG(metadata.st_mode) or observed != expected:
        raise OSError("inventory executable identity changed after approval")
    digest = hashlib.sha256()
    while True:
        chunk = os.read(descriptor, 1024 * 1024)
        if not chunk:
            break
        digest.update(chunk)
    confirmed = os.fstat(descriptor)
    confirmed_identity = (confirmed.st_dev, confirmed.st_ino,
                          confirmed.st_mode & 0o7777, confirmed.st_size,
                          confirmed.st_mtime_ns)
    if confirmed_identity != expected:
        raise OSError("inventory executable identity changed while hashing")
    if len(expected_digest) != 64 or digest.hexdigest() != expected_digest:
        raise OSError("inventory executable content changed after approval")
    os.lseek(descriptor, 0, os.SEEK_SET)
    os.set_inheritable(descriptor, True)
    stable_path = f"/dev/fd/{descriptor}"
    os.execve(stable_path, [display_path], os.environ.copy())
except (OSError, ValueError) as exc:
    print(f"test inventory execution refused: {exc}", file=sys.stderr)
    for descriptor in reversed(opened):
        try:
            os.close(descriptor)
        except OSError:
            pass
    raise SystemExit(126)
' "$REPO_ROOT" "$inv_relative" "$inv_cmd" "$inv_device" "$inv_inode" "$inv_mode" "$inv_size" "$inv_mtime_ns" "$inv_sha256" 2>/dev/null) >"$INVENTORY_FILE" || inv_rc=$?
      if [[ "$inv_rc" -eq 0 ]]; then
        CATEGORY_STATE="available"
        CATEGORY_REASON=""
      else
        : >"$INVENTORY_FILE"
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

python3 - "$SPEC_DIR" "$REPO_ROOT" "$ADAPTER" "$CATEGORY_STATE" "$CATEGORY_REASON" \
    "$QUIET" "$REFERENCE_FILE" "$INVENTORY_FILE" <<'PY'
import json, os, re, stat, sys

spec_dir, repo_root, adapter, category_state, category_reason, quiet_arg, reference_file, inventory_file = sys.argv[1:]
quiet = quiet_arg == "1"

def reject_duplicate_members(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON member {key!r}")
        result[key] = value
    return result

# PD-04: not-applicable | available | unexecutable. See the header.
try:
    with open(reference_file, encoding="utf-8") as fh:
        reference_document = json.load(fh, object_pairs_hook=reject_duplicate_members)
except (OSError, ValueError) as exc:
    print(f"scenario-test-resolve: cannot parse shared reference projection: {exc}", file=sys.stderr)
    sys.exit(2)

# The canonical taxonomy. A scenario whose required type is LIVE cannot be
# satisfied by a test the runner classifies as mocked — that substitution is
# precisely what makes a green suite meaningless.
LIVE = {"integration", "e2e-api", "e2e-ui", "stress", "load"}
MOCKED = {"unit", "ui-unit", "functional"}

inventory = []
if adapter == "command" and os.path.getsize(inventory_file):
    try:
        with open(inventory_file, encoding="utf-8") as fh:
            doc = json.load(fh, object_pairs_hook=reject_duplicate_members)
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
    except (OSError, ValueError) as exc:
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

findings = []
unresolved_category = []
checked = 0
planned = 0
scanned_titles = 0
skipped_category = 0

reference_scenarios = reference_document.get("scenarios", [])
if not isinstance(reference_scenarios, list):
    print("scenario-test-resolve: shared reference projection has no scenarios array", file=sys.stderr)
    sys.exit(2)

def open_validated(ref):
    relative = ref.get("path")
    identity = ref.get("identity")
    if not relative or not isinstance(identity, dict):
        return None, "shared projection omitted repository-relative path identity"
    parts = relative.split("/")
    opened = []
    try:
        if not parts or any(part in ("", ".", "..") for part in parts):
            raise OSError("authored path is not a normalized repository-relative path")
        if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_DIRECTORY"):
            raise OSError("no-follow descriptor walking is unavailable")
        parent = os.open(repo_root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        opened.append(parent)
        for component in parts[:-1]:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                            dir_fd=parent)
            opened.append(child)
            if not stat.S_ISDIR(os.fstat(child).st_mode):
                raise OSError("authored path intermediate component is not a directory")
            parent = child
        descriptor = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW, dir_fd=parent)
        opened.append(descriptor)
        metadata = os.fstat(descriptor)
        if not stat.S_ISREG(metadata.st_mode):
            raise OSError("authored target is no longer a regular file")
    except (OSError, ValueError) as exc:
        for opened_descriptor in reversed(opened):
            try:
                os.close(opened_descriptor)
            except OSError:
                pass
        return None, f"authored target cannot be opened safely without symlinks: {exc}"
    observed = (metadata.st_mode & 0o7777, metadata.st_dev, metadata.st_ino,
                metadata.st_size, metadata.st_mtime_ns)
    expected = (identity.get("mode"), identity.get("device"), identity.get("inode"),
                identity.get("size"), identity.get("modifiedNanoseconds"))
    if observed != expected:
        for opened_descriptor in reversed(opened):
            os.close(opened_descriptor)
        return None, "authored target identity changed after shared-reader projection"
    try:
        with os.fdopen(os.dup(descriptor), encoding="utf-8", errors="replace") as fh:
            content = fh.read()
        confirmed = os.fstat(descriptor)
        confirmed_identity = (confirmed.st_mode & 0o7777, confirmed.st_dev,
                              confirmed.st_ino, confirmed.st_size,
                              confirmed.st_mtime_ns)
        if confirmed_identity != expected:
            return None, "authored target identity changed while reading validated content"
        return content, ""
    except OSError as exc:
        return None, f"authored target cannot be read from its validated descriptor: {exc}"
    finally:
        for opened_descriptor in reversed(opened):
            try:
                os.close(opened_descriptor)
            except OSError:
                pass

def javascript_titles(source):
    """Return literal JS/TS suite/test titles without trusting comments or strings."""
    titles = []
    limit = len(source)
    index = 0

    def skip_space(position):
        while position < limit and source[position].isspace():
            position += 1
        return position

    def quoted(position):
        quote = source[position]
        position += 1
        value = []
        while position < limit:
            char = source[position]
            if char == "\\" and position + 1 < limit:
                value.append(source[position + 1])
                position += 2
                continue
            if char == quote:
                return "".join(value), position + 1
            if char in "\r\n":
                return None, position
            value.append(char)
            position += 1
        return None, position

    while index < limit:
        if source.startswith("//", index):
            newline = source.find("\n", index + 2)
            index = limit if newline < 0 else newline + 1
            continue
        if source.startswith("/*", index):
            end = source.find("*/", index + 2)
            index = limit if end < 0 else end + 2
            continue
        if source[index] in "'\"`":
            quote = source[index]
            if quote == "`":
                index += 1
                while index < limit:
                    if source[index] == "\\": index += 2; continue
                    if source[index] == "`": index += 1; break
                    index += 1
            else:
                _, index = quoted(index)
            continue
        name = re.match(r"(?:(?:Deno\.)?test|it|describe)(?:\.(?:only|skip|todo))?\b", source[index:])
        if name and (index == 0 or not (source[index - 1].isalnum() or source[index - 1] in "_$")):
            cursor = skip_space(index + len(name.group(0)))
            each = re.match(r"\.each\s*\(", source[cursor:])
            if each:
                cursor += len(each.group(0))
                nesting = 1
                while cursor < limit and nesting:
                    if source.startswith("//", cursor):
                        newline = source.find("\n", cursor + 2)
                        cursor = limit if newline < 0 else newline + 1
                        continue
                    if source.startswith("/*", cursor):
                        end = source.find("*/", cursor + 2)
                        cursor = limit if end < 0 else end + 2
                        continue
                    if source[cursor] in "'\"`":
                        quote = source[cursor]
                        cursor += 1
                        while cursor < limit:
                            if source[cursor] == "\\": cursor += 2; continue
                            if source[cursor] == quote: cursor += 1; break
                            cursor += 1
                        continue
                    if source[cursor] == "(": nesting += 1
                    elif source[cursor] == ")": nesting -= 1
                    cursor += 1
                cursor = skip_space(cursor)
            if cursor < limit and source[cursor] == "(":
                cursor = skip_space(cursor + 1)
                if cursor < limit and source[cursor] in "'\"":
                    title, end = quoted(cursor)
                    if title is not None:
                        titles.append(title)
                        index = end
                        continue
        index += 1
    return titles

def structural_titles(path, source):
    titles = []
    patterns = (
        re.compile(r'^\s*(?:async\s+)?def\s+(test_[A-Za-z0-9_]+)\s*\('),
        re.compile(r'^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*\('),
    )
    if os.path.splitext(path)[1].lower() in (".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"):
        titles.extend(javascript_titles(source))
    pending_test_attribute = False
    for line in source.splitlines():
        stripped = line.lstrip()
        if stripped.startswith(("//", "# ", "/*", "*", "<!--")):
            continue
        if re.match(r"^#\[(?:test|tokio::test(?:\([^]]*\))?)\]\s*$", stripped):
            pending_test_attribute = True
            continue
        if pending_test_attribute and re.match(r"^#\[[^]]+\]\s*$", stripped):
            continue
        if stripped.startswith("@"):
            continue
        match = patterns[0].match(line)
        if match:
            titles.append(match.group(1))
            pending_test_attribute = False
            continue
        match = patterns[1].match(line)
        if match and pending_test_attribute:
            titles.append(match.group(1))
        pending_test_attribute = False
    return titles

for parsed_scenario in reference_scenarios:
    if not isinstance(parsed_scenario, dict):
        print("scenario-test-resolve: malformed scenario in shared reference projection", file=sys.stderr)
        sys.exit(2)
    sid = parsed_scenario.get("scenarioId", "<unidentified-scenario>")
    required = parsed_scenario.get("requiredTestType")
    for ref in parsed_scenario.get("references", []):
        rel = ref["path"]
        title = ref.get("title")
        if ref["kind"] == "planned":
            planned += 1
            continue
        checked += 1
        abs_path = ref["canonicalPath"]
        if not ref["exists"]:
            findings.append((sid, rel, "MISSING-FILE",
                             "no such file under the repository root"))
            continue
        source, identity_error = open_validated(ref)
        if source is None:
            findings.append((sid, rel, "STALE-FILE-IDENTITY", identity_error))
            continue
        if title is None:
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
            try:
                declarations = structural_titles(abs_path, source)
            except OSError as exc:
                findings.append((sid, f"{rel}#{title}", "UNREADABLE-FILE", str(exc)))
                continue
            occurrences = declarations.count(title)
            if occurrences == 0:
                findings.append((sid, f"{rel}#{title}", "MISSING-TITLE",
                                 "the referenced file declares no structural test with this exact title"))
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
    if checked == 0:
        if planned:
            print(f"[scenario-test-resolve] NA — 0 authored linked-test reference(s) checked; "
                  f"{planned} planned test reference(s) remain unauthored")
        else:
            print("[scenario-test-resolve] NA — 0 authored linked-test reference(s) checked; "
                  "manifest is unclassified (no authored or planned test references)")
        sys.exit(0)
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
