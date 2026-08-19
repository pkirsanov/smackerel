#!/usr/bin/env bash
# mutation-receipt.sh — executed-mutation receipts for high-risk scenarios
# (IMP-048 SCOPE-4, EV-11).
#
# Owner: bubbles.test
#
# WHY THIS EXISTS
# `test-mechanism-lint.sh` checks that a DECLARATION exists: a `riskTier: high`
# scenario must declare `negativeControlMechanism: mutation`. Nothing checked
# that the declared mutation ever RAN. Measured across the eight workspace
# repositories, ZERO declare a `mutationExecution:` block, so every high-risk
# mutation control in the estate is a claim whose execution is unproven. A
# declaration nobody executes is the same unearned confidence the tier exists to
# remove -- it just costs a field instead of a sentence.
#
# This is the COMPANION check. It does not restate the declaration rules; it
# answers the one question they cannot: did the mutant get killed?
#
# WHAT A RECEIPT CARRIES
#   scenarioId · mutantId · sourceDigest · testId
#   expectedFailureSignature · observedFailure · restoredDigest
#
# WHAT IT GUARANTEES
#   earned receipts   THIS SCRIPT RUNS THE MUTATION. The mutant digest, the test
#                     exit code and the observed failure line are OBSERVED, never
#                     accepted from a caller, so there is no argument with which
#                     to assert a kill. A mutate command that changes no bytes
#                     produces no mutant and no receipt.
#   isolation         The mutation is applied to a COPIED FIXTURE under $TMPDIR,
#                     never to the working tree. In a shared tree the window
#                     between mutate and restore is a COMMIT WINDOW, and this
#                     estate has already shipped a neutralized check that way. A
#                     `--workspace` that resolves inside the repository is
#                     REFUSED rather than warned about.
#   restoration       `restoredDigest` is re-read from the REPOSITORY file after
#                     the run and must equal `sourceDigest`. A mismatch means a
#                     mutant was left behind; the run is refused and the event is
#                     recorded, because a dirty tree is the one thing that must
#                     never be lost to a silent exit.
#   honest absence    `negativeControlFallbackReason` remains the escape for a
#                     repository with no mutation tooling (IMP-048 R4). Only an
#                     UNDECLARED absence is a finding, so a deliberate fallback
#                     stays distinguishable from an unexecuted claim -- which is
#                     the entire point of the check.
#
# DEFAULT OFF, per repo. The enabling switch is the EXISTING project-owned
# `mutationExecution:` block, resolved by `mutation-resolve.sh`. There is no
# second resolver and no second registry: `adapter: none` (the default, and the
# measured state of every repository today) makes both `run` and `check` a
# no-op that writes nothing and creates no `.specify` directory. A repository
# that has not configured mutation tooling behaves EXACTLY as it does today.
#
# Store: append-only JSONL at <repo-root>/.specify/runtime/mutation-receipts.jsonl,
# schemaVersion `mutation-receipt/v1`, one object per EXECUTION.
#
# Usage:
#   mutation-receipt.sh resolve [--repo-root PATH] [--names-only]
#   mutation-receipt.sh run --scenario-id ID --test-id ID
#                           --source REPO_RELATIVE_PATH
#                           --mutate COMMAND --test COMMAND
#                           --expect TEXT
#                           [--workspace DIR] [--timeout SECONDS]
#                           [--repo-root PATH]
#   mutation-receipt.sh check --spec-dir DIR [--repo-root PATH] [--quiet]
#   mutation-receipt.sh status [--scenario-id ID] [--repo-root PATH]
#
# Project config (project-owned, never framework-managed; already shipped):
#
#   mutationExecution:
#     adapter: none | command
#     command: scripts/bubbles-mutation
#
# Exit codes (run):
#   0  the mutant was KILLED and the observed failure carried the expectation
#   1  a finding: the mutant SURVIVED, the signature did not match, or the test
#      never finished
#   2  usage error, or a refusal: absent source, a non-isolated workspace, a
#      mutate command that produced no mutant, or a violated restoration
# Exit codes (check):        0 clean or inert - 1 finding - 2 usage
# Exit codes (resolve/status): 0 ok - 1 configured-but-broken adapter - 2 usage
#
# There is no --skip, --force, --ignore, --assume or --no-verify flag.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="mutation-receipt"
SCHEMA_VERSION="mutation-receipt/v1"
STORE_REL=".specify/runtime/mutation-receipts.jsonl"
RESOLVER="$SCRIPT_DIR/mutation-resolve.sh"

# Portable bounded execution: GNU timeout, gtimeout, or a bash watchdog, all
# normalising a timeout to 124.
# shellcheck source=guard-lib.sh
. "$SCRIPT_DIR/guard-lib.sh"

usage() {
  cat <<'EOF'
Usage:
  mutation-receipt.sh resolve [--repo-root PATH] [--names-only]
  mutation-receipt.sh run --scenario-id ID --test-id ID
                          --source REPO_RELATIVE_PATH
                          --mutate COMMAND --test COMMAND
                          --expect TEXT
                          [--workspace DIR] [--timeout SECONDS]
                          [--repo-root PATH]
  mutation-receipt.sh check --spec-dir DIR [--repo-root PATH] [--quiet]
  mutation-receipt.sh status [--scenario-id ID] [--repo-root PATH]

Outcomes (closed set):
  KILLED              the mutant made the test fail, carrying the expectation
  SURVIVED            the test still passed on the mutant; the control is vacuous
  SIGNATURE_MISMATCH  the test failed, but not for the stated reason
  UNRESOLVED          the time budget expired; neither a kill nor a survival
  ISOLATION_VIOLATED  the repository source changed; a mutant escaped the fixture

A receipt is EARNED: this script applies the mutation in an isolated copy and
runs the test, so the mutant digest and the failure are observed rather than
supplied.

Enabling switch (default OFF), read by mutation-resolve.sh:

  mutationExecution:
    adapter: none | command
EOF
}

fail() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  exit "${2:-1}"
}

die_usage() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  usage >&2
  exit 2
}

# sha256 over a file. macOS ships `shasum`, GNU ships `sha256sum`; neither is
# guaranteed, so both are probed and absence is loud rather than degrading to an
# unverified digest.
sha256_file() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    fail "no sha256 tool (sha256sum/shasum) available to earn a receipt" 2
  fi
}

# Minimal JSON string escaping. Ids, paths, commands and failure lines are the
# only untrusted inputs; a literal tab, newline or control byte inside one is
# escaped rather than emitted raw so a single record stays a single line.
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//	/\\t}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\n'/\\n}"
  printf '%s' "$s"
}

# The enabling switch is the EXISTING resolver. Duplicating it here would give
# the estate two answers to "is mutation execution configured", and a config
# surface with two readers drifts by construction (BUG-004).
# Called WITHOUT --names-only on purpose: the resolver only validates a
# configured command path on the full path, and a configured-but-broken adapter
# must fail loud rather than read as an opt-out.
resolve_adapter() {
  local repo_root="$1" out=''
  [ -f "$RESOLVER" ] || fail "mutation resolver missing at $RESOLVER" 2
  out="$(bash "$RESOLVER" --repo-root "$repo_root" 2>&1)" || {
    printf '%s\n' "$out" >&2
    fail "mutationExecution is configured but not resolvable; a typo must not read as an opt-out" 1
  }
  awk -F= '$1 == "adapter" { print $2; exit }' <<< "$out"
}

store_record_count() {
  if [ -f "$1" ]; then
    awk 'END { print NR + 0 }' "$1"
  else
    printf '0'
  fi
}

# --------------------------------------------------------------------------
# resolve
# --------------------------------------------------------------------------
cmd_resolve() {
  local repo_root="$PWD" names_only=0
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --names-only)
        names_only=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  printf 'adapter=%s\n' "$adapter"
  [ "$names_only" = "1" ] && return 0
  printf 'schemaVersion=%s\n' "$SCHEMA_VERSION"
  printf 'store=%s/%s\n' "$repo_root" "$STORE_REL"
  printf 'repoRoot=%s\n' "$repo_root"
  return 0
}

# --------------------------------------------------------------------------
# run
# --------------------------------------------------------------------------
cmd_run() {
  local repo_root="$PWD" scenario_id='' test_id='' source_rel=''
  local mutate_cmd='' test_cmd='' expect='' workspace='' timeout_secs=0

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --scenario-id)
        [ "$#" -ge 2 ] || die_usage "--scenario-id requires a value"
        scenario_id="$2"
        shift 2
        ;;
      --test-id)
        [ "$#" -ge 2 ] || die_usage "--test-id requires a value"
        test_id="$2"
        shift 2
        ;;
      --source)
        [ "$#" -ge 2 ] || die_usage "--source requires a repo-relative path"
        source_rel="$2"
        shift 2
        ;;
      --mutate)
        [ "$#" -ge 2 ] || die_usage "--mutate requires a command"
        mutate_cmd="$2"
        shift 2
        ;;
      --test)
        [ "$#" -ge 2 ] || die_usage "--test requires a command"
        test_cmd="$2"
        shift 2
        ;;
      --expect)
        [ "$#" -ge 2 ] || die_usage "--expect requires the failure signature the mutant must produce"
        expect="$2"
        shift 2
        ;;
      --workspace)
        [ "$#" -ge 2 ] || die_usage "--workspace requires a directory OUTSIDE the repository"
        workspace="$2"
        shift 2
        ;;
      --timeout)
        [ "$#" -ge 2 ] || die_usage "--timeout requires a value"
        timeout_secs="$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --assume* | --no-verify*)
        printf '%s: "%s" does not exist. A mutant is killed by running the test, never by asserting it.\n' "$NAME" "$1" >&2
        exit 2
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"
  [ -n "$scenario_id" ] || die_usage "--scenario-id is required"
  [ -n "$test_id" ] || die_usage "--test-id is required"
  [ -n "$source_rel" ] || die_usage "--source is required"
  [ -n "$mutate_cmd" ] || die_usage "--mutate is required"
  [ -n "$test_cmd" ] || die_usage "--test is required"
  # An expectation is not optional. A receipt that records only "the test
  # failed" cannot distinguish a killed mutant from a broken harness, so it is
  # unfalsifiable and therefore not evidence.
  [ -n "$expect" ] || die_usage "--expect is required: a receipt with no expected failure signature cannot be falsified"
  case "$timeout_secs" in
    '' | *[!0-9]*) die_usage "--timeout must be a non-negative integer (got: $timeout_secs)" ;;
  esac

  local adapter
  adapter="$(resolve_adapter "$repo_root")"

  # ---- DEFAULT OFF --------------------------------------------------------
  # Nothing runs, nothing is recorded, no directory is created. A repository
  # with no mutation tooling is not asked to invent one (IMP-048 R4).
  if [ "$adapter" = "none" ]; then
    printf 'adapter=none\n'
    printf 'receipts=skipped\n'
    printf 'records=0\n'
    return 0
  fi

  # ---- the source under mutation -----------------------------------------
  case "$source_rel" in
    /* | *..*) die_usage "--source must be a repo-relative path without '..' (got '$source_rel')" ;;
  esac
  local source_abs="$repo_root/$source_rel"
  [ -f "$source_abs" ] ||
    fail "--source '$source_rel' not found at $source_abs; a mutant cannot be derived from bytes that do not exist" 2

  local source_digest
  source_digest="$(sha256_file "$source_abs")"

  # ---- ISOLATION ----------------------------------------------------------
  # Enforced, not documented. A workspace inside the repository is refused
  # BEFORE anything is mutated, because the refusal is worthless afterwards.
  local isolation='' owned_workspace=0 ws=''
  if [ -n "$workspace" ]; then
    [ -d "$workspace" ] || fail "--workspace '$workspace' is not a directory" 2
    ws="$(cd "$workspace" && pwd)"
    case "$ws/" in
      "$repo_root"/*)
        fail "--workspace '$ws' is inside the repository at $repo_root. A mutation in a shared working tree opens a commit window between mutate and restore; use a directory outside the repository, or omit --workspace to get an isolated copy" 2
        ;;
    esac
    isolation='declared-workspace'
  else
    ws="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-mutation.XXXXXX")" ||
      fail "cannot create an isolated fixture directory" 2
    owned_workspace=1
    isolation='copied-fixture'
  fi
  if [ "$owned_workspace" -eq 1 ]; then
    # shellcheck disable=SC2064  # expand ws now: the trap must survive it
    trap "rm -rf '$ws'" EXIT INT TERM
  fi

  # A faithful copy of the repository, minus its history. `.git` is excluded so
  # a mutation can never reach the object store of the tree it is isolated from.
  cp -R "$repo_root/." "$ws/" || fail "cannot populate the isolated fixture at $ws" 2
  rm -rf "$ws/.git"

  local ws_source="$ws/$source_rel"
  [ -f "$ws_source" ] || fail "the isolated fixture is missing '$source_rel'" 2
  local copy_digest
  copy_digest="$(sha256_file "$ws_source")"
  [ "$copy_digest" = "$source_digest" ] ||
    fail "the isolated fixture does not carry the source bytes ($copy_digest != $source_digest)" 2

  local work_dir
  work_dir="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-mutation-out.XXXXXX")"
  local mutate_out="$work_dir/mutate.out" test_out="$work_dir/test.out"

  local started_at finished_at rc=0
  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # ---- apply the mutation IN THE FIXTURE ---------------------------------
  export BUBBLES_MUTATION_SOURCE="$source_rel"
  export BUBBLES_MUTATION_WORKSPACE="$ws"
  rc=0
  if [ "$timeout_secs" -gt 0 ]; then
    (cd "$ws" && bubbles_run_with_timeout "$timeout_secs" bash -c "$mutate_cmd") > "$mutate_out" 2>&1 || rc=$?
  else
    (cd "$ws" && bash -c "$mutate_cmd") > "$mutate_out" 2>&1 || rc=$?
  fi
  cat "$mutate_out"
  if [ "$rc" -ne 0 ]; then
    rm -rf "$work_dir"
    fail "the mutate command exited $rc; no mutant was produced" 2
  fi

  local mutant_id
  mutant_id="$(sha256_file "$ws_source")"
  if [ "$mutant_id" = "$source_digest" ]; then
    rm -rf "$work_dir"
    fail "the mutate command changed no bytes of '$source_rel'; a file identical to the source is not a mutant and cannot be killed" 2
  fi

  # ---- run the test AGAINST the mutant ------------------------------------
  rc=0
  if [ "$timeout_secs" -gt 0 ]; then
    (cd "$ws" && bubbles_run_with_timeout "$timeout_secs" bash -c "$test_cmd") > "$test_out" 2>&1 || rc=$?
  else
    (cd "$ws" && bash -c "$test_cmd") > "$test_out" 2>&1 || rc=$?
  fi
  cat "$test_out"
  finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # The observed failure is read from the captured output file, never from a
  # pipe: `... | grep -q` on unbounded output is the BUG-009 SIGPIPE race.
  local matched observed outcome
  matched="$(awk -v pat="$expect" 'index($0, pat) > 0 { print "yes"; exit }' "$test_out")"
  observed="$(awk -v pat="$expect" 'index($0, pat) > 0 { print substr($0, 1, 200); exit }' "$test_out")"
  if [ -z "$observed" ]; then
    observed="$(awk '/FAIL|Fail|error|Error|assert|Assert|not ok|Traceback/ { print substr($0, 1, 200); exit }' "$test_out")"
  fi
  [ -n "$observed" ] || observed="exit:$rc"

  if [ "$timeout_secs" -gt 0 ] && [ "$rc" -eq 124 ]; then
    # Neither a kill nor a survival. A test that never finished has demonstrated
    # nothing about the mutant in either direction.
    outcome="UNRESOLVED"
    observed=''
  elif [ "$rc" -eq 0 ]; then
    # The mutant survived. The negative control is vacuous: the test does not
    # notice when the owning code is wrong, which is exactly what the tier asked.
    outcome="SURVIVED"
    observed=''
  elif [ "$matched" != "yes" ]; then
    outcome="SIGNATURE_MISMATCH"
  else
    outcome="KILLED"
  fi

  # ---- RESTORATION --------------------------------------------------------
  # Re-read from the REPOSITORY, not from the fixture. This is the check that
  # catches a mutant left behind in the shared tree.
  local restored_digest
  restored_digest="$(sha256_file "$source_abs")"
  if [ "$restored_digest" != "$source_digest" ]; then
    outcome="ISOLATION_VIOLATED"
  fi

  # ---- append the receipt -------------------------------------------------
  local store="$repo_root/$STORE_REL"
  mkdir -p "$(dirname "$store")" || fail "cannot create the receipt store directory $(dirname "$store")"
  printf '{"schemaVersion":"%s","scenarioId":"%s","testId":"%s","sourcePath":"%s","sourceDigest":"%s","mutantId":"%s","expectedFailureSignature":"%s","observedFailure":"%s","restoredDigest":"%s","isolation":"%s","workspace":"%s","startedAt":"%s","finishedAt":"%s","exitCode":%s,"outcome":"%s"}\n' \
    "$SCHEMA_VERSION" \
    "$(json_escape "$scenario_id")" \
    "$(json_escape "$test_id")" \
    "$(json_escape "$source_rel")" \
    "$source_digest" \
    "$mutant_id" \
    "$(json_escape "$expect")" \
    "$(json_escape "$observed")" \
    "$restored_digest" \
    "$isolation" \
    "$(json_escape "$ws")" \
    "$started_at" \
    "$finished_at" \
    "$rc" \
    "$outcome" \
    >> "$store" || fail "cannot append to the receipt store $store"

  rm -rf "$work_dir"

  printf 'adapter=%s\n' "$adapter"
  printf 'store=%s\n' "$store"
  printf 'scenarioId=%s\n' "$scenario_id"
  printf 'testId=%s\n' "$test_id"
  printf 'sourceDigest=%s\n' "$source_digest"
  printf 'mutantId=%s\n' "$mutant_id"
  printf 'restoredDigest=%s\n' "$restored_digest"
  printf 'isolation=%s\n' "$isolation"
  printf 'expectedFailureSignature=%s\n' "$expect"
  printf 'observedFailure=%s\n' "${observed:-<none>}"
  printf 'outcome=%s\n' "$outcome"
  printf 'records=%s\n' "$(store_record_count "$store")"

  case "$outcome" in
    KILLED) return 0 ;;
    ISOLATION_VIOLATED)
      printf '%s: the repository source changed during the run (%s != %s). A mutant was left behind in the shared tree.\n' \
        "$NAME" "$restored_digest" "$source_digest" >&2
      return 2
      ;;
    *) return 1 ;;
  esac
}

# --------------------------------------------------------------------------
# check — the companion to the declaration lint
# --------------------------------------------------------------------------
cmd_check() {
  local repo_root="$PWD" spec_dir='' quiet=0
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --spec-dir)
        [ "$#" -ge 2 ] || die_usage "--spec-dir requires a value"
        spec_dir="$2"
        shift 2
        ;;
      --quiet)
        quiet=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --assume* | --no-verify*)
        printf '%s: "%s" does not exist. Declare a negativeControlFallbackReason instead.\n' "$NAME" "$1" >&2
        exit 2
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -n "$spec_dir" ] || die_usage "--spec-dir is required"
  [ -d "$spec_dir" ] || die_usage "spec directory not found: $spec_dir"
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local manifest="$spec_dir/scenario-manifest.json"
  if [ ! -f "$manifest" ]; then
    [ "$quiet" -eq 1 ] || printf '[%s] NA — no scenario-manifest.json\n' "$NAME"
    return 0
  fi

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  if [ "$adapter" = "none" ]; then
    # DEFAULT OFF. Declaration rules still apply (test-mechanism-lint.sh owns
    # them); the EXECUTION obligation only exists where tooling is configured.
    [ "$quiet" -eq 1 ] || printf '[%s] OK — mutationExecution adapter is none (inert)\n' "$NAME"
    return 0
  fi

  command -v python3 > /dev/null 2>&1 || die_usage "python3 is required"

  MANIFEST="$manifest" STORE="$repo_root/$STORE_REL" QUIET="$quiet" python3 - <<'PY'
import json, os, sys

manifest_path = os.environ["MANIFEST"]
store_path = os.environ["STORE"]
quiet = os.environ.get("QUIET") == "1"

try:
    with open(manifest_path, encoding="utf-8") as fh:
        manifest = json.load(fh)
except (OSError, ValueError) as exc:
    print(f"mutation-receipt: cannot parse {manifest_path}: {exc}", file=sys.stderr)
    sys.exit(2)

# Append-only means the NEWEST record for a scenario wins. Nothing rewrites a
# past record, so a later run correcting an earlier one is an addition.
latest = {}
malformed = 0
try:
    with open(store_path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except ValueError:
                malformed += 1
                continue
            if isinstance(rec, dict) and isinstance(rec.get("scenarioId"), str):
                latest[rec["scenarioId"]] = rec
except OSError:
    pass

findings = []
owed = 0
covered = 0

# Both manifest envelopes ship in the wild: {"scenarios": [...]} and a bare
# top-level list of the same objects.
scenarios = manifest.get("scenarios") if isinstance(manifest, dict) else manifest
if not isinstance(scenarios, list):
    scenarios = []

VALID_ISOLATION = {"copied-fixture", "declared-workspace"}


def nonempty(value):
    return isinstance(value, str) and value.strip() != ""


for scenario in scenarios:
    if not isinstance(scenario, dict):
        continue
    sid = scenario.get("id") or scenario.get("scenarioId") or "<unidentified-scenario>"
    if scenario.get("riskTier") != "high":
        continue
    mech = scenario.get("testMechanism")
    if not isinstance(mech, dict):
        continue
    if mech.get("negativeControlMechanism") != "mutation":
        continue

    # THE HONEST ESCAPE (IMP-048 R4). A repository without mutation tooling
    # stays shippable by SAYING it has none. Only an UNDECLARED absence is a
    # finding, so a deliberate fallback never looks like an unexecuted claim.
    if nonempty(mech.get("negativeControlFallbackReason")):
        continue

    owed += 1
    receipt = latest.get(sid)
    if receipt is None:
        findings.append((sid, "MUTATION-UNEXECUTED",
                         "declares negativeControlMechanism 'mutation' at riskTier 'high' but "
                         "no execution receipt names it. Run the mutation, or state a "
                         "negativeControlFallbackReason naming why it cannot be run"))
        continue

    outcome = receipt.get("outcome")
    source_digest = receipt.get("sourceDigest")
    mutant_id = receipt.get("mutantId")
    restored = receipt.get("restoredDigest")
    observed = receipt.get("observedFailure")
    expected = receipt.get("expectedFailureSignature")
    isolation = receipt.get("isolation")

    # A LEFT-BEHIND MUTANT. Checked before the outcome, because a dirty tree is
    # a worse fact than whatever the run concluded.
    if not nonempty(source_digest) or not nonempty(restored) or restored != source_digest:
        findings.append((sid, "MUTATION-NOT-RESTORED",
                         f"the receipt records restoredDigest '{restored}' against sourceDigest "
                         f"'{source_digest}'. The source was not restored, so a mutant was left "
                         "behind in the tree; the receipt is refused"))
        continue

    if isolation not in VALID_ISOLATION:
        findings.append((sid, "MUTATION-NOT-ISOLATED",
                         f"the receipt records isolation '{isolation}'. A mutation run in a "
                         "shared working tree opens a commit window between mutate and restore; "
                         f"only {', '.join(sorted(VALID_ISOLATION))} are accepted"))
        continue

    # A CLAIM THAT IS NOT A KILL. Diagnosed before the forgery checks so a
    # genuine survival is reported as a survival: SURVIVED legitimately records
    # no observed failure, and calling that a forgery would misname the one
    # outcome the tier most needs to read correctly.
    if outcome != "KILLED":
        findings.append((sid, "MUTATION-NOT-KILLED",
                         f"the receipt records outcome '{outcome}'. Only KILLED shows the test is "
                         "sensitive to the owning code being wrong; SURVIVED shows the opposite, "
                         "and UNRESOLVED shows neither"))
        continue

    # AN ASSERTED KILL. Every field below can only come from a real run: the
    # mutant digest from bytes that differ, the observed failure from output, the
    # expectation from the caller's falsifiable claim.
    if not nonempty(mutant_id) or mutant_id == source_digest:
        findings.append((sid, "MUTATION-FORGED",
                         f"the receipt names mutantId '{mutant_id}', which is empty or identical "
                         "to the source. A file that differs from the source in no byte is not a "
                         "mutant, so nothing was killed"))
        continue
    if not nonempty(expected):
        findings.append((sid, "MUTATION-FORGED",
                         "the receipt states no expectedFailureSignature. A receipt recording only "
                         "that the test failed cannot distinguish a killed mutant from a broken "
                         "harness, so it is unfalsifiable"))
        continue
    if not nonempty(observed):
        findings.append((sid, "MUTATION-FORGED",
                         "the receipt records no observedFailure. A kill with no observed failure "
                         "was asserted, not executed"))
        continue

    covered += 1

if malformed:
    print(f"mutation-receipt: {malformed} unparseable record(s) in {store_path} were ignored",
          file=sys.stderr)

if findings:
    print("mutation-receipt: FAIL — a declared mutation control was never executed (EV-11)",
          file=sys.stderr)
    for sid, code, detail in findings:
        print(f"  {code}: {sid}", file=sys.stderr)
        print(f"    {detail}", file=sys.stderr)
    print("", file=sys.stderr)
    print(f"mutation-receipt: {len(findings)} finding(s).", file=sys.stderr)
    sys.exit(1)

if not quiet:
    if owed:
        print(f"[mutation-receipt] OK — {covered}/{owed} high-risk mutation control(s) carry a killed-mutant receipt")
    else:
        print("[mutation-receipt] OK — no high-risk scenario owes a mutation receipt (inert)")
sys.exit(0)
PY
}

# --------------------------------------------------------------------------
# status
# --------------------------------------------------------------------------
cmd_status() {
  local repo_root="$PWD" scenario_id=''
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --scenario-id)
        [ "$#" -ge 2 ] || die_usage "--scenario-id requires a value"
        scenario_id="$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter store
  adapter="$(resolve_adapter "$repo_root")"
  store="$repo_root/$STORE_REL"
  printf 'adapter=%s\n' "$adapter"
  printf 'store=%s\n' "$store"
  if [ "$adapter" = "none" ]; then
    printf 'records=0\n'
    return 0
  fi
  printf 'records=%s\n' "$(store_record_count "$store")"

  if [ -n "$scenario_id" ]; then
    local field
    printf 'scenarioId=%s\n' "$scenario_id"
    for field in outcome mutantId sourceDigest restoredDigest observedFailure isolation; do
      printf '%s=%s\n' "$field" \
        "$(store_last_field "$store" "$scenario_id" "$field")"
    done
  fi
  return 0
}

# The LAST recorded value of one field for one scenario. Append-only means the
# newest line wins; nothing rewrites a past record.
store_last_field() {
  local store="$1" sid="$2" field="$3"
  [ -f "$store" ] || {
    printf 'none'
    return 0
  }
  local value
  value="$(awk -v sid="\"scenarioId\":\"$sid\"" -v pat="\"$field\":" '
    index($0, sid) > 0 { line = $0 }
    END {
      if (line == "") exit 0
      p = index(line, pat)
      if (p == 0) exit 0
      rest = substr(line, p + length(pat))
      if (substr(rest, 1, 1) == "\"") {
        rest = substr(rest, 2)
        q = index(rest, "\"")
        if (q > 0) print substr(rest, 1, q - 1)
      } else {
        q = index(rest, ",")
        if (q == 0) q = index(rest, "}")
        if (q > 0) print substr(rest, 1, q - 1)
      }
    }
  ' "$store")"
  printf '%s' "${value:-none}"
}

main() {
  local sub="${1:-}"
  if [ "$#" -gt 0 ]; then
    shift
  fi
  case "$sub" in
    resolve) cmd_resolve "$@" ;;
    run) cmd_run "$@" ;;
    check) cmd_check "$@" ;;
    status) cmd_status "$@" ;;
    -h | --help)
      usage
      exit 0
      ;;
    '')
      usage >&2
      exit 2
      ;;
    *) die_usage "unknown subcommand: $sub" ;;
  esac
}

main "$@"
