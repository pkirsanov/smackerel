#!/usr/bin/env bash
# Hermetic adversarial tests for the IMP-037 / SCOPE-6 recall authority firewall
# enforced by result-envelope-validate.sh.
#
# The firewall exists because recalled experience is authority tier 4 and always
# `advisory`. READING recall is fine. CITING it as evidence is the breach: the
# moment a recall artifact lands in an evidence field it has acquired an
# authority it never had, backing a claim nobody independently re-read.
#
# Two obligations are tested here:
#
#   1. CONSTANT PARITY. result-envelope-validate.sh derives the refused shapes
#      from experience-recall-index.py at runtime, and falls back to literals
#      only when the indexer is absent. The validator's own comment names THIS
#      selftest as the reason that fallback cannot drift unseen. If nothing
#      pinned them, the fallback could quietly diverge from the real record-id
#      format and the guard would go blind on an indexer-less tree.
#
#   2. REFUSAL BEHAVIOR. Recall ids, recall index paths, and recall exports are
#      refused in EVERY mode -- including --advisory, which by design never
#      blocks on schema problems. An authority breach is not a schema nit.
#
# Exit: 0 all assertions pass, 1 any failure.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
VALIDATOR="$SCRIPT_DIR/result-envelope-validate.sh"
INDEXER="$SCRIPT_DIR/experience-recall-index.py"
SCHEMA="$REPO_ROOT/bubbles/schemas/result-envelope.schema.json"

PASS=0
FAIL=0
WORK="$(mktemp -d)"

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

echo "experience-recall-authority-selftest"

# --- Graceful degradation -------------------------------------------------
# The validator itself SKIPs (exit 0) without python3 + jsonschema. Asserting
# refusal behavior against a skipping validator would be a false green, so gate
# the ASSERTIONS, not the whole file, on the dependency actually being present.
for required in "$VALIDATOR" "$INDEXER" "$SCHEMA"; do
  if [[ ! -f "$required" ]]; then
    echo "experience-recall-authority-selftest: SKIP (missing $required)"
    exit 0
  fi
done
if ! command -v python3 >/dev/null 2>&1; then
  echo "experience-recall-authority-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  echo "experience-recall-authority-selftest: SKIP (python jsonschema not installed)"
  exit 0
fi

# --- Obligation 1: fallback constants match the indexer -------------------
# Compare the validator's FALLBACK_* literals against the indexer's real
# RECORD_ID_RE / RUNTIME_PARTS. Both are read as source text via AST, so this
# check never imports or executes either file.
constant_report="$(
  VALIDATOR="$VALIDATOR" INDEXER="$INDEXER" python3 - <<'PY'
import ast, os, re, sys
from pathlib import Path

validator = Path(os.environ['VALIDATOR']).read_text(encoding='utf-8')
indexer_src = Path(os.environ['INDEXER']).read_text(encoding='utf-8')

def literal_after(name, text):
    m = re.search(rf'^{name}\s*=\s*(.+)$', text, re.MULTILINE)
    if not m:
        return None
    try:
        return ast.literal_eval(m.group(1).strip())
    except Exception:
        return None

fallback_pattern = literal_after('FALLBACK_RECORD_ID_PATTERN', validator)
fallback_parts = literal_after('FALLBACK_RUNTIME_PARTS', validator)

indexer_parts = None
indexer_pattern = None
for node in ast.parse(indexer_src).body:
    if not isinstance(node, ast.Assign) or len(node.targets) != 1:
        continue
    target = node.targets[0]
    if not isinstance(target, ast.Name):
        continue
    if target.id == 'RUNTIME_PARTS':
        try:
            indexer_parts = tuple(ast.literal_eval(node.value))
        except Exception:
            pass
    elif target.id == 'RECORD_ID_RE' and isinstance(node.value, ast.Call) and node.value.args:
        try:
            indexer_pattern = ast.literal_eval(node.value.args[0])
        except Exception:
            pass

problems = []
for label, got, want in (
    ('record-id pattern', fallback_pattern, indexer_pattern),
    ('runtime parts', fallback_parts, tuple(indexer_parts) if indexer_parts else None),
):
    if got is None:
        problems.append(f'validator fallback for {label} not found or unparsable')
    elif want is None:
        problems.append(f'indexer constant for {label} not found or unparsable')
    elif got != want:
        problems.append(f'{label} drift: validator={got!r} indexer={want!r}')

print('OK' if not problems else 'DRIFT: ' + '; '.join(problems))
PY
)"
if [[ "$constant_report" == "OK" ]]; then
  pass "validator fallback constants match the indexer's own constants"
else
  fail "fallback constants drifted from the indexer ($constant_report)"
fi

# --- Fixture repository ---------------------------------------------------
# repo_root inside the validator is agents_dir.parent, so cited export paths
# resolve relative to this fixture root -- never the real repository.
FIXTURE="$WORK/repo"
mkdir -p "$FIXTURE/agents" "$FIXTURE/bubbles/scripts" "$FIXTURE/bubbles/schemas"
cp "$VALIDATOR" "$FIXTURE/bubbles/scripts/result-envelope-validate.sh"
cp "$INDEXER" "$FIXTURE/bubbles/scripts/experience-recall-index.py"
cp "$SCHEMA" "$FIXTURE/bubbles/schemas/result-envelope.schema.json"
for helper in dependency-posture.sh python-env.sh; do
  [[ -f "$SCRIPT_DIR/$helper" ]] && cp "$SCRIPT_DIR/$helper" "$FIXTURE/bubbles/scripts/$helper"
done
FIXTURE_VALIDATOR="$FIXTURE/bubbles/scripts/result-envelope-validate.sh"

HEX64="$(printf 'a%.0s' $(seq 1 64))"
RECALL_ID="recall-$HEX64"

write_agent() {
  # $1 = agent basename, $2 = envelope JSON body
  rm -f "$FIXTURE"/agents/*.agent.md
  {
    printf '# Fixture Agent\n\n'
    printf '```json result_envelope:\n'
    printf '%s\n' "$2"
    printf '```\n'
  } >"$FIXTURE/agents/$1.agent.md"
}

# Output goes to a file and the exit code is the function's own return value.
# Capturing both through `$(...)` would run the assignment in a subshell, so the
# caller would never see RC -- the harness would silently assert nothing.
RUN_OUT="$WORK/validator-output.txt"
run_validator() {
  # $1 = mode flag ("" for v6-default). Writes output to $RUN_OUT, returns rc.
  if [[ -n "${1:-}" ]]; then
    bash "$FIXTURE_VALIDATOR" "$1" >"$RUN_OUT" 2>&1
  else
    bash "$FIXTURE_VALIDATOR" >"$RUN_OUT" 2>&1
  fi
}

assert_refused() {
  # $1 = label, $2 = mode flag, $3 = envelope JSON
  write_agent fixture "$3"
  local rc out
  run_validator "$2"
  rc=$?
  out="$(cat "$RUN_OUT")"
  if [[ "$rc" -ne 1 ]]; then
    fail "$1 (expected exit 1, got $rc; output: $out)"
    return
  fi
  case "$out" in
    *RECALL-AUTHORITY*) pass "$1" ;;
    *) fail "$1 (exit 1 but no RECALL-AUTHORITY finding; output: $out)" ;;
  esac
}

assert_allowed() {
  # $1 = label, $2 = envelope JSON
  write_agent fixture "$2"
  local rc out
  run_validator ""
  rc=$?
  out="$(cat "$RUN_OUT")"
  case "$out" in
    *RECALL-AUTHORITY*) fail "$1 (unexpected RECALL-AUTHORITY refusal; output: $out)" ;;
    *)
      if [[ "$rc" -eq 0 ]]; then
        pass "$1"
      else
        fail "$1 (expected exit 0, got $rc; output: $out)"
      fi
      ;;
  esac
}

# --- Obligation 2: refusal in every mode ----------------------------------
ID_ENVELOPE="{\"agent\":\"bubbles.test\",\"outcome\":\"completed_owned\",\"evidenceRefs\":[\"$RECALL_ID\"]}"

assert_refused "recall record id in evidenceRefs is refused (v6-default)" "" "$ID_ENVELOPE"
assert_refused "recall record id in evidenceRefs is refused (--strict)" "--strict" "$ID_ENVELOPE"
# The load-bearing case: --advisory never blocks on schema problems, but an
# authority breach is not a schema problem.
assert_refused "recall record id in evidenceRefs is refused even in --advisory" "--advisory" "$ID_ENVELOPE"

assert_refused "recall id embedded in a longer citation string is refused" "" \
  "{\"agent\":\"bubbles.test\",\"outcome\":\"completed_owned\",\"evidenceRefs\":[\"see $RECALL_ID for prior context\"]}"

assert_refused "recall index path in evidenceRefs is refused" "" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":[".specify/runtime/experience-recall/index.json"]}'

assert_refused "recall index path with an anchor fragment is refused" "" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":[".specify/runtime/experience-recall/index.json#hit-3"]}'

assert_refused "recall id in toolCalls is refused" "" \
  "{\"agent\":\"bubbles.test\",\"outcome\":\"completed_owned\",\"toolCalls\":[{\"tool\":\"recall\",\"result\":\"$RECALL_ID\"}]}"

assert_refused "recall id in a nested dodRef is refused" "" \
  "{\"agent\":\"bubbles.test\",\"outcome\":\"completed_owned\",\"findings\":[{\"id\":\"F1\",\"dodRef\":\"$RECALL_ID\"}]}"

# --- Anti-evasion: exports are classified by CONTENT, not name ------------
# `recall export --output` takes a caller-named path, so a name-based rule
# would be defeated by `mv`. These two files differ only in content.
mkdir -p "$FIXTURE/docs"
printf '[{"contractType":"record","recallAuthority":"advisory","recordId":"%s"}]\n' \
  "$RECALL_ID" >"$FIXTURE/docs/quarterly-notes.json"
assert_refused "recall export renamed to an innocent path is still refused" "" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":["docs/quarterly-notes.json"]}'

printf '[{"contractType":"record","note":"not a recall export"}]\n' \
  >"$FIXTURE/docs/similar-shape.json"
assert_allowed "a non-recall JSON file with a similar shape is allowed" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":["docs/similar-shape.json"]}'

# A symlink is refused classification outright rather than followed, so a
# symlink pointing at a real export cannot be used to smuggle it in either --
# but it also must not crash the validator.
ln -s "$FIXTURE/docs/quarterly-notes.json" "$FIXTURE/docs/linked-export.json" 2>/dev/null || true
write_agent fixture '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":["docs/linked-export.json"]}'
run_validator ""
symlink_rc=$?
if [[ "$symlink_rc" -eq 0 || "$symlink_rc" -eq 1 ]]; then
  pass "a symlinked export is handled without crashing the validator"
else
  fail "symlinked export crashed the validator (exit $symlink_rc; output: $(cat "$RUN_OUT"))"
fi

assert_allowed "a path-traversal citation does not crash or false-refuse" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":["../../etc/passwd"]}'

# --- Legitimate citations must keep working -------------------------------
assert_allowed "a normal spec evidence anchor is allowed" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":["specs/001-thing/report.md#test-evidence"]}'

# Narrative disclosure is deliberately NOT scanned: an agent SHOULD be able to
# say it consulted advisory recall. Refusing that would punish honesty and push
# consultation underground.
assert_allowed "naming recall in the narrative summary is allowed" \
  "{\"agent\":\"bubbles.test\",\"outcome\":\"completed_owned\",\"summary\":\"Consulted advisory recall ($RECALL_ID) then re-read the current source.\",\"evidenceRefs\":[\"specs/001-thing/report.md#test-evidence\"]}"

assert_allowed "an http URL citation is not treated as a repository path" \
  '{"agent":"bubbles.test","outcome":"completed_owned","evidenceRefs":["https://example.invalid/experience-recall/index.json"]}'

# --- Fallback path: validator shipped without the indexer -----------------
# This is the exact tree the fallback literals exist for. Refusal must survive.
rm -f "$FIXTURE/bubbles/scripts/experience-recall-index.py"
assert_refused "recall id is still refused when the indexer is absent (fallback literals)" "" "$ID_ENVELOPE"
cp "$INDEXER" "$FIXTURE/bubbles/scripts/experience-recall-index.py"

# --- The real repository must not false-positive --------------------------
real_out="$(bash "$VALIDATOR" --advisory 2>&1)"
real_rc=$?
case "$real_out" in
  *RECALL-AUTHORITY*)
    fail "the real agents/ tree reports a recall-authority violation (output: $real_out)"
    ;;
  *)
    if [[ "$real_rc" -eq 0 ]]; then
      pass "the real agents/ tree has no recall-authority violation"
    else
      fail "real-tree advisory run exited $real_rc (output: $real_out)"
    fi
    ;;
esac

echo
echo "experience-recall-authority-selftest: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]] || exit 1
exit 0
