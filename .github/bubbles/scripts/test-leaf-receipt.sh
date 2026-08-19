#!/usr/bin/env bash
# test-leaf-receipt.sh — occurrence identity, content addressing and resume for
# individual TEST LEAVES (IMP-048 SCOPE-3, PERF-9).
#
# Owner: bubbles.test
#
# WHY THIS EXISTS
# `phase-coordinator.sh` already guarantees occurrence identity, no-replay and
# resume-at-the-first-unresolved-occurrence -- but only for PHASES. Below a
# phase, an individual test command had no identity, no recorded outcome and no
# content addressing. So a timeout in ONE leaf forced re-running every sibling
# that had already passed on identical bytes, and a timed-out leaf was
# indistinguishable from a failing one.
#
# This extends those guarantees exactly one level down. It is a CONSUMER of the
# same occurrence rule, not a second copy of it: `occurrence-identity-lib.sh`
# owns what an occurrence id is and which outcomes resolve one, and both
# coordinators source it. Writing that rule twice is BUG-004, which this
# repository has already paid for once.
#
# WHAT IT GUARANTEES
#   earned receipts    THIS SCRIPT RUNS THE LEAF. The exit code and the output
#                      hash are observed, never accepted from a caller, so there
#                      is no argument with which to assert a pass. Digests come
#                      from bytes on disk; a declared owner path that does not
#                      exist is REFUSED rather than digested as absent.
#   no replay          A leaf that passed on an identical candidateDigest AND
#                      environmentFingerprint is reported ACCEPTED and its
#                      command is NOT run again.
#   precise invalidation  A changed production owner invalidates ONLY the leaves
#                      whose declared refs cover it. A sibling covering
#                      untouched owners stays ACCEPTED. That precision is the
#                      whole value: invalidating everything would be correct but
#                      would restore the waste this exists to remove.
#   unresolved is not a verdict  A timed-out leaf is UNRESOLVED. It is neither a
#                      pass nor a failure, and `assert` REFUSES to let it stand
#                      as either -- including as RED evidence, which is the
#                      dangerous direction: a test that never finished has not
#                      demonstrated a defect.
#   resume             Work resumes at the FIRST unresolved leaf, read from the
#                      store, never from a name or a guess.
#   stated reruns      The aggregate runs once all focused leaves pass on frozen
#                      bytes. A SECOND aggregate on the same digest is refused
#                      unless `--rerun-reason` states why. Reruns are not
#                      banned; `integration-order` is a legitimate reason where
#                      composition exercises what no leaf can. The requirement
#                      is that the reason be stated.
#
# ACCEPTANCE KEY (IMP-048 R2/R3). A leaf is ACCEPTED only when a stored receipt
# carries ALL of:
#   outcome RAN_PASS · candidateDigest == derived · environmentFingerprint ==
#   derived · a non-empty outputHash
# candidateDigest is derived from the leaf's id, its command, and the digests of
# every production owner it declares -- so owner-path digests are inside the
# acceptance key, not beside it. A receipt missing its output hash is a receipt
# nobody earned, and it is never honoured.
#
# DEFAULT OFF, per repo. With no `testLeafReceipts:` block, no config file, or
# an explicit `adapter: none`, the run behaves EXACTLY as it does today: every
# leaf executes, nothing is accepted, nothing is recorded, and no `.specify`
# directory is created. Note what default-off does NOT mean here -- it does not
# mean "skip the tests". A receipt capability that silently stopped running
# tests when unconfigured would be far worse than the waste it replaces.
#
# Store: append-only JSONL at <repo-root>/.specify/runtime/test-leaf-receipts.jsonl,
# schemaVersion `test-leaf-receipt/v1`, one object per EXECUTION. ACCEPTED is a
# report, never a record: re-recording work that was not run is how a green
# history gets manufactured by repetition.
#
# Usage:
#   test-leaf-receipt.sh resolve [--repo-root PATH] [--names-only]
#   test-leaf-receipt.sh run  --leaf <testId>=<command> [--leaf ...]
#                             [--leaf-ref <testId>=<path>]...
#                             [--env <key>=<value>]...
#                             [--timeout SECONDS]
#                             [--aggregate <testId>=<command>]
#                             [--rerun-reason TEXT]
#                             [--repo-root PATH]
#   test-leaf-receipt.sh assert --test-id ID --as pass|red-evidence [--repo-root PATH]
#   test-leaf-receipt.sh status [--test-id ID] [--repo-root PATH]
#
# Project config (project-owned, never framework-managed):
#
#   testLeafReceipts:
#     adapter: none | jsonl
#
# Exit codes (run):
#   0  every leaf and the aggregate are resolved
#   1  work remains: a leaf failed, a leaf is UNRESOLVED, or the aggregate is
#      BLOCKED_NOT_RUN
#   2  usage error, or a refusal (absent owner path, second aggregate with no
#      stated reason, bypass flag)
#
# Exit codes (assert): 0 the claim is supported by an earned receipt - 2 refused
# Exit codes (resolve/status): 0 ok - 1 configured-but-broken adapter - 2 usage
#
# There is no --skip, --force, --ignore, --replay or --assume flag.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="test-leaf-receipt"
SCHEMA_VERSION="test-leaf-receipt/v1"
STORE_REL=".specify/runtime/test-leaf-receipts.jsonl"

# One occurrence rule, two consumers. See phase-coordinator.sh.
# shellcheck source=occurrence-identity-lib.sh
. "$SCRIPT_DIR/occurrence-identity-lib.sh"
# Portable bounded execution: GNU timeout, gtimeout, or a bash watchdog, all
# normalising a timeout to 124.
# shellcheck source=guard-lib.sh
. "$SCRIPT_DIR/guard-lib.sh"

usage() {
  cat <<'EOF'
Usage:
  test-leaf-receipt.sh resolve [--repo-root PATH] [--names-only]
  test-leaf-receipt.sh run  --leaf <testId>=<command> [--leaf ...]
                            [--leaf-ref <testId>=<path>]...
                            [--env <key>=<value>]...
                            [--timeout SECONDS]
                            [--aggregate <testId>=<command>]
                            [--rerun-reason TEXT]
                            [--repo-root PATH]
  test-leaf-receipt.sh assert --test-id ID --as pass|red-evidence [--repo-root PATH]
  test-leaf-receipt.sh status [--test-id ID] [--repo-root PATH]

Leaf outcomes (closed set):
  RAN_PASS         executed and passed
  RAN_FAIL         executed and failed with a real exit code
  UNRESOLVED       the time budget expired; neither a pass nor a failure
  BLOCKED_NOT_RUN  the aggregate, held back because a leaf is outstanding
  ACCEPTED         a prior receipt covers these exact bytes; NOT re-run

A receipt is EARNED: this script runs the leaf, so the exit code and the output
hash are observed rather than supplied.

Project config (default OFF):

  testLeafReceipts:
    adapter: none | jsonl
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

# sha256 over a string supplied on stdin (callers use a herestring).
sha256_stdin() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    fail "no sha256 tool (sha256sum/shasum) available to earn a receipt" 2
  fi
}

# Minimal JSON string escaping. Ids, paths, commands and rerun reasons are the
# only untrusted inputs; a literal tab or newline inside one is escaped rather
# than emitted raw so a single record stays a single line.
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//	/\\t}"
  printf '%s' "$s"
}

resolve_adapter() {
  local repo_root="$1" config_file='' adapter=''
  if [ -f "$repo_root/.github/bubbles-project.yaml" ]; then
    config_file="$repo_root/.github/bubbles-project.yaml"
  elif [ -f "$repo_root/bubbles-project.yaml" ]; then
    config_file="$repo_root/bubbles-project.yaml"
  fi

  if [ -n "$config_file" ]; then
    adapter="$(awk '
      /^[[:space:]]*#/ { next }
      /^testLeafReceipts:[[:space:]]*$/ { inblock = 1; next }
      inblock && /^[^[:space:]]/ { inblock = 0 }
      inblock && $1 == "adapter:" {
        value = $2
        gsub(/["\047]/, "", value)
        print value
        exit
      }
    ' "$config_file" 2> /dev/null || true)"
  fi

  [ -n "$adapter" ] || adapter='none'

  case "$adapter" in
    *[!a-z0-9-]* | '' | -*)
      fail "invalid testLeafReceipts.adapter '$adapter' (expected none or jsonl)"
      ;;
  esac

  # A configured-but-unknown adapter fails LOUD instead of degrading to `none`.
  # A typo that silently produced "unenforced" would be indistinguishable from a
  # deliberate opt-out, and every leaf would then re-run forever while the repo
  # believed it had receipts.
  case "$adapter" in
    none | jsonl) ;;
    *) fail "unknown testLeafReceipts.adapter '$adapter' (expected none or jsonl)" ;;
  esac

  printf '%s' "$adapter"
}

# The LAST recorded value of one field for one occurrence. Append-only means the
# newest line wins; nothing rewrites a past record. Handles both quoted and bare
# (numeric / null) values.
store_last_field() {
  local store="$1" occ="$2" field="$3"
  [ -f "$store" ] || return 0
  awk -v occ="\"testOccurrenceId\":\"$occ\"" -v pat="\"$field\":" '
    index($0, occ) > 0 { line = $0 }
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
  ' "$store"
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
  if [ "$names_only" = "1" ]; then
    return 0
  fi
  printf 'schemaVersion=%s\n' "$SCHEMA_VERSION"
  printf 'store=%s/%s\n' "$repo_root" "$STORE_REL"
  printf 'repoRoot=%s\n' "$repo_root"
  return 0
}

# --------------------------------------------------------------------------
# run
# --------------------------------------------------------------------------
cmd_run() {
  local repo_root="$PWD" timeout_secs=0 agg_raw='' rerun_reason=''
  local leaf_ids=() leaf_cmds=() ref_tests=() ref_paths=() env_pairs=()

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --leaf)
        [ "$#" -ge 2 ] || die_usage "--leaf requires <testId>=<command>"
        case "$2" in
          *=*) ;;
          *) die_usage "--leaf expects <testId>=<command> (got: $2)" ;;
        esac
        leaf_ids+=("${2%%=*}")
        leaf_cmds+=("${2#*=}")
        shift 2
        ;;
      --leaf-ref)
        [ "$#" -ge 2 ] || die_usage "--leaf-ref requires <testId>=<path>"
        case "$2" in
          *=*) ;;
          *) die_usage "--leaf-ref expects <testId>=<path> (got: $2)" ;;
        esac
        ref_tests+=("${2%%=*}")
        ref_paths+=("${2#*=}")
        shift 2
        ;;
      --env)
        [ "$#" -ge 2 ] || die_usage "--env requires <key>=<value>"
        case "$2" in
          *=*) ;;
          *) die_usage "--env expects <key>=<value> (got: $2)" ;;
        esac
        env_pairs+=("$2")
        shift 2
        ;;
      --timeout)
        [ "$#" -ge 2 ] || die_usage "--timeout requires a value"
        timeout_secs="$2"
        shift 2
        ;;
      --aggregate)
        [ "$#" -ge 2 ] || die_usage "--aggregate requires <testId>=<command>"
        case "$2" in
          *=*) ;;
          *) die_usage "--aggregate expects <testId>=<command> (got: $2)" ;;
        esac
        agg_raw="$2"
        shift 2
        ;;
      --rerun-reason)
        [ "$#" -ge 2 ] || die_usage "--rerun-reason requires a value"
        rerun_reason="$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --replay* | --assume*)
        printf '%s: "%s" does not exist. A leaf is resolved by running it, never by asserting it.\n' "$NAME" "$1" >&2
        exit 2
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"
  # `${arr[*]+x}` rather than `${#arr[@]}`: an empty array under `set -u` is an
  # unbound reference on bash 4.0-4.3, and the framework baseline is bash 4.
  [ -n "${leaf_ids[*]+x}" ] || die_usage "at least one --leaf is required"
  case "$timeout_secs" in
    '' | *[!0-9]*) die_usage "--timeout must be a non-negative integer (got: $timeout_secs)" ;;
  esac

  local adapter
  adapter="$(resolve_adapter "$repo_root")"

  local work_dir
  work_dir="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-test-leaf.XXXXXX")"
  # shellcheck disable=SC2064  # expand work_dir now: the trap must survive it
  trap "rm -rf '$work_dir'" EXIT INT TERM

  local agg_id='' agg_cmd=''
  if [ -n "$agg_raw" ]; then
    agg_id="${agg_raw%%=*}"
    agg_cmd="${agg_raw#*=}"
  fi

  # ---- occurrence identity (shared rule, leaves then the aggregate) --------
  local all_names=("${leaf_ids[@]}")
  [ -n "$agg_id" ] && all_names+=("$agg_id")
  local occ_ids=() _oid
  while IFS= read -r _oid; do
    [ -n "$_oid" ] || continue
    occ_ids+=("$_oid")
  done < <(occurrence_ids_for "${all_names[@]}")

  local agg_occ=''
  if [ -n "$agg_id" ]; then
    agg_occ="${occ_ids[$((${#occ_ids[@]} - 1))]}"
  fi

  # ---- DEFAULT OFF --------------------------------------------------------
  # Every leaf executes and nothing is recorded: exactly today's behaviour. The
  # capability being unconfigured must never be a reason a test does not run.
  if [ "$adapter" = "none" ]; then
    printf 'adapter=none\n'
    printf 'receipts=skipped\n'
    local i rc any_fail=0
    for i in "${!leaf_ids[@]}"; do
      rc=0
      bash -c "${leaf_cmds[$i]}" || rc=$?
      printf 'leaf.%s.occurrenceId=%s\n' "$((i + 1))" "${occ_ids[$i]}"
      printf 'leaf.%s.outcome=%s\n' "$((i + 1))" "$([ "$rc" -eq 0 ] && printf 'RAN_PASS' || printf 'RAN_FAIL')"
      [ "$rc" -eq 0 ] || any_fail=1
    done
    if [ -n "$agg_id" ]; then
      rc=0
      bash -c "$agg_cmd" || rc=$?
      printf 'aggregate.occurrenceId=%s\n' "$agg_occ"
      printf 'aggregate.outcome=%s\n' "$([ "$rc" -eq 0 ] && printf 'RAN_PASS' || printf 'RAN_FAIL')"
      [ "$rc" -eq 0 ] || any_fail=1
    fi
    printf 'records=0\n'
    printf 'complete=%s\n' "$([ "$any_fail" -eq 0 ] && printf 'true' || printf 'false')"
    return "$any_fail"
  fi

  # ---- environment fingerprint -------------------------------------------
  # Sorted so declaration order cannot change the identity, and carrying the
  # platform so "same env" is not merely "same declared env".
  local env_material env_fp
  env_material="platform:$(uname -s)/$(uname -m)"
  if [ -n "${env_pairs[*]+x}" ]; then
    env_material="${env_material}
$(printf '%s\n' "${env_pairs[@]}" | sort)"
  fi
  env_fp="$(sha256_stdin <<< "$env_material")"

  # ---- per-leaf owner digests and candidate digests -----------------------
  local leaf_refjson=() leaf_cand=()
  local i j refs sorted_refs path abs digest refjson material union_refs=''
  for i in "${!leaf_ids[@]}"; do
    refs=''
    for j in ${ref_tests[@]+"${!ref_tests[@]}"}; do
      if [ "${ref_tests[$j]}" = "${leaf_ids[$i]}" ]; then
        refs="${refs}${refs:+
}${ref_paths[$j]}"
      fi
    done
    sorted_refs=''
    if [ -n "$refs" ]; then
      sorted_refs="$(sort -u <<< "$refs")"
    fi

    material="${leaf_ids[$i]}
${leaf_cmds[$i]}"
    refjson=''
    while IFS= read -r path; do
      [ -n "$path" ] || continue
      case "$path" in
        /*) abs="$path" ;;
        *) abs="$repo_root/$path" ;;
      esac
      # A digest can only be earned from bytes that exist. Digesting an absent
      # owner as "" would let a deleted file look like an unchanged one.
      [ -f "$abs" ] || fail "declared owner path not found: $path (a leaf receipt cannot be earned from bytes that do not exist)" 2
      digest="$(sha256_file "$abs")"
      material="${material}
${path}:${digest}"
      refjson="${refjson}${refjson:+,}\"$(json_escape "${path}:${digest}")\""
      union_refs="${union_refs}${union_refs:+
}${path}"
    done <<< "$sorted_refs"

    leaf_refjson+=("$refjson")
    leaf_cand+=("$(sha256_stdin <<< "$material")")
  done

  # ---- aggregate digest: the union of every declared owner ---------------
  local agg_cand='' agg_refjson=''
  if [ -n "$agg_id" ]; then
    sorted_refs=''
    if [ -n "$union_refs" ]; then
      sorted_refs="$(sort -u <<< "$union_refs")"
    fi
    material="${agg_id}
${agg_cmd}"
    while IFS= read -r path; do
      [ -n "$path" ] || continue
      case "$path" in
        /*) abs="$path" ;;
        *) abs="$repo_root/$path" ;;
      esac
      [ -f "$abs" ] || fail "declared owner path not found: $path" 2
      digest="$(sha256_file "$abs")"
      material="${material}
${path}:${digest}"
      agg_refjson="${agg_refjson}${agg_refjson:+,}\"$(json_escape "${path}:${digest}")\""
    done <<< "$sorted_refs"
    agg_cand="$(sha256_stdin <<< "$material")"
  fi

  local store="$repo_root/$STORE_REL"

  # ---- STATED RERUNS ------------------------------------------------------
  # Checked BEFORE anything executes, so a refusal writes nothing at all.
  if [ -n "$agg_id" ] && [ -z "$rerun_reason" ] && [ -f "$store" ]; then
    local prior_outcome prior_cand
    prior_outcome="$(store_last_field "$store" "$agg_occ" outcome)"
    prior_cand="$(store_last_field "$store" "$agg_occ" candidateDigest)"
    if [ "$prior_outcome" = "RAN_PASS" ] && [ "$prior_cand" = "$agg_cand" ]; then
      fail "the aggregate $agg_occ already passed on candidateDigest $agg_cand; a second aggregate on the same digest requires --rerun-reason (for example: integration-order), or omit --aggregate to resume the leaves only" 2
    fi
  fi

  # ---- acceptance ---------------------------------------------------------
  # ACCEPTED requires all four: the recorded outcome, the candidate digest, the
  # environment fingerprint, AND an output hash. The last one is what makes a
  # receipt earned rather than asserted.
  local leaf_outcome=() leaf_exit=() leaf_hash=() leaf_started=() leaf_finished=()
  local prior_outcome prior_cand prior_env prior_hash
  local resume_index=-1
  for i in "${!leaf_ids[@]}"; do
    prior_outcome="$(store_last_field "$store" "${occ_ids[$i]}" outcome)"
    prior_cand="$(store_last_field "$store" "${occ_ids[$i]}" candidateDigest)"
    prior_env="$(store_last_field "$store" "${occ_ids[$i]}" environmentFingerprint)"
    prior_hash="$(store_last_field "$store" "${occ_ids[$i]}" outputHash)"
    if [ "$prior_outcome" = "RAN_PASS" ] &&
      [ "$prior_cand" = "${leaf_cand[$i]}" ] &&
      [ "$prior_env" = "$env_fp" ] &&
      [ -n "$prior_hash" ]; then
      leaf_outcome+=("ACCEPTED")
    else
      leaf_outcome+=("PENDING")
      [ "$resume_index" -ge 0 ] || resume_index="$i"
    fi
    leaf_exit+=("")
    leaf_hash+=("")
    leaf_started+=("")
    leaf_finished+=("")
  done

  mkdir -p "$(dirname "$store")" || fail "cannot create the receipt store directory $(dirname "$store")"

  # ---- execution ----------------------------------------------------------
  # Leaves are INDEPENDENT of one another. One outstanding leaf must not delete
  # the diagnostics of its siblings -- that is the same principle
  # phase-coordinator.sh applies to independent phases. Only the AGGREGATE is
  # gated, because "the whole suite on frozen bytes" is a claim about all of
  # them at once.
  local rc out_file now outstanding=0
  for i in "${!leaf_ids[@]}"; do
    if [ "${leaf_outcome[$i]}" = "ACCEPTED" ]; then
      continue
    fi
    out_file="$work_dir/leaf.$i.out"
    leaf_started[$i]="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    rc=0
    if [ "$timeout_secs" -gt 0 ]; then
      bubbles_run_with_timeout "$timeout_secs" bash -c "${leaf_cmds[$i]}" > "$out_file" 2>&1 || rc=$?
    else
      bash -c "${leaf_cmds[$i]}" > "$out_file" 2>&1 || rc=$?
    fi
    leaf_finished[$i]="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    # Captured, never discarded: the output is hashed for the receipt and then
    # replayed in full so no line is lost to the caller.
    cat "$out_file"
    leaf_hash[$i]="$(sha256_file "$out_file")"
    if [ "$timeout_secs" -gt 0 ] && [ "$rc" -eq 124 ]; then
      # UNRESOLVED. Not a pass, not a failure. A command that never finished has
      # demonstrated nothing in either direction.
      leaf_outcome[$i]="UNRESOLVED"
      leaf_exit[$i]=""
      outstanding=1
    elif [ "$rc" -eq 0 ]; then
      leaf_outcome[$i]="RAN_PASS"
      leaf_exit[$i]="$rc"
    else
      leaf_outcome[$i]="RAN_FAIL"
      leaf_exit[$i]="$rc"
      outstanding=1
    fi
  done

  # ---- aggregate ----------------------------------------------------------
  local agg_outcome='' agg_exit='' agg_hash='' agg_started='' agg_finished=''
  if [ -n "$agg_id" ]; then
    local all_resolved=1
    for i in "${!leaf_ids[@]}"; do
      occurrence_resolving_outcome "${leaf_outcome[$i]}" || all_resolved=0
    done
    if [ "$all_resolved" -eq 0 ]; then
      agg_outcome="BLOCKED_NOT_RUN"
      outstanding=1
    else
      out_file="$work_dir/aggregate.out"
      agg_started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      rc=0
      if [ "$timeout_secs" -gt 0 ]; then
        bubbles_run_with_timeout "$timeout_secs" bash -c "$agg_cmd" > "$out_file" 2>&1 || rc=$?
      else
        bash -c "$agg_cmd" > "$out_file" 2>&1 || rc=$?
      fi
      agg_finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      cat "$out_file"
      agg_hash="$(sha256_file "$out_file")"
      if [ "$timeout_secs" -gt 0 ] && [ "$rc" -eq 124 ]; then
        agg_outcome="UNRESOLVED"
        outstanding=1
      elif [ "$rc" -eq 0 ]; then
        agg_outcome="RAN_PASS"
        agg_exit="$rc"
      else
        agg_outcome="RAN_FAIL"
        agg_exit="$rc"
        outstanding=1
      fi
    fi
  fi

  # ---- append receipts ----------------------------------------------------
  # Only for work that ACTUALLY RAN. ACCEPTED is reported, never recorded:
  # re-recording an execution that did not happen is how a green history is
  # manufactured by repetition.
  append_receipt() {
    local occ="$1" tid="$2" kind="$3" cand="$4" refs="$5" outcome="$6"
    local exit_code="$7" out_hash="$8" started="$9" finished="${10}"
    local exit_json='null'
    [ -n "$exit_code" ] && exit_json="$exit_code"
    local reason_json='null'
    [ -n "$rerun_reason" ] && reason_json="\"$(json_escape "$rerun_reason")\""
    printf '{"schemaVersion":"%s","testOccurrenceId":"%s","testId":"%s","kind":"%s","candidateDigest":"%s","inputPathDigests":[%s],"environmentFingerprint":"%s","timeout":%s,"startedAt":"%s","finishedAt":"%s","exitCode":%s,"outputHash":"%s","outcome":"%s","rerunReason":%s}\n' \
      "$SCHEMA_VERSION" \
      "$(json_escape "$occ")" \
      "$(json_escape "$tid")" \
      "$kind" \
      "$(json_escape "$cand")" \
      "$refs" \
      "$env_fp" \
      "$timeout_secs" \
      "$(json_escape "$started")" \
      "$(json_escape "$finished")" \
      "$exit_json" \
      "$(json_escape "$out_hash")" \
      "$outcome" \
      "$reason_json" \
      >> "$store" || fail "cannot append to the receipt store $store"
  }

  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  for i in "${!leaf_ids[@]}"; do
    [ "${leaf_outcome[$i]}" = "ACCEPTED" ] && continue
    append_receipt "${occ_ids[$i]}" "${leaf_ids[$i]}" leaf "${leaf_cand[$i]}" \
      "${leaf_refjson[$i]}" "${leaf_outcome[$i]}" "${leaf_exit[$i]}" \
      "${leaf_hash[$i]}" "${leaf_started[$i]:-$now}" "${leaf_finished[$i]:-$now}"
  done
  if [ -n "$agg_id" ]; then
    append_receipt "$agg_occ" "$agg_id" aggregate "$agg_cand" "$agg_refjson" \
      "$agg_outcome" "$agg_exit" "$agg_hash" "${agg_started:-$now}" "${agg_finished:-$now}"
  fi

  # ---- report -------------------------------------------------------------
  printf 'adapter=jsonl\n'
  printf 'store=%s\n' "$store"
  printf 'environmentFingerprint=%s\n' "$env_fp"
  printf 'timeout=%s\n' "$timeout_secs"
  if [ "$resume_index" -ge 0 ]; then
    printf 'resumedAt=%s\n' "${occ_ids[$resume_index]}"
  else
    printf 'resumedAt=%s\n' '<nothing outstanding>'
  fi
  for i in "${!leaf_ids[@]}"; do
    printf 'leaf.%s.occurrenceId=%s\n' "$((i + 1))" "${occ_ids[$i]}"
    printf 'leaf.%s.testId=%s\n' "$((i + 1))" "${leaf_ids[$i]}"
    printf 'leaf.%s.outcome=%s\n' "$((i + 1))" "${leaf_outcome[$i]}"
    printf 'leaf.%s.candidateDigest=%s\n' "$((i + 1))" "${leaf_cand[$i]}"
    printf 'leaf.%s.replayed=%s\n' "$((i + 1))" \
      "$([ "${leaf_outcome[$i]}" = "ACCEPTED" ] && printf 'false' || printf 'true')"
  done
  if [ -n "$agg_id" ]; then
    printf 'aggregate.occurrenceId=%s\n' "$agg_occ"
    printf 'aggregate.outcome=%s\n' "$agg_outcome"
    printf 'aggregate.candidateDigest=%s\n' "$agg_cand"
    printf 'aggregate.rerunReason=%s\n' "${rerun_reason:-none}"
  fi
  printf 'records=%s\n' "$(store_record_count "$store")"
  printf 'complete=%s\n' "$([ "$outstanding" -eq 0 ] && printf 'true' || printf 'false')"

  [ "$outstanding" -eq 0 ] || return 1
  return 0
}

# --------------------------------------------------------------------------
# assert
# --------------------------------------------------------------------------
# The surface on which an UNRESOLVED leaf is refused as evidence. RED evidence
# is the dangerous direction and is refused just as hard as a claimed pass: a
# test that never finished has not demonstrated a defect either.
cmd_assert() {
  local repo_root="$PWD" test_id='' claim=''
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --test-id)
        [ "$#" -ge 2 ] || die_usage "--test-id requires a value"
        test_id="$2"
        shift 2
        ;;
      --as)
        [ "$#" -ge 2 ] || die_usage "--as requires pass or red-evidence"
        claim="$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --replay* | --assume*)
        printf '%s: "%s" does not exist. A leaf is resolved by running it, never by asserting it.\n' "$NAME" "$1" >&2
        exit 2
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"
  [ -n "$test_id" ] || die_usage "--test-id is required"
  case "$claim" in
    pass | red-evidence) ;;
    *) die_usage "--as must be pass or red-evidence (got: '${claim}')" ;;
  esac

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  printf 'adapter=%s\n' "$adapter"
  if [ "$adapter" = "none" ]; then
    printf 'assert=skipped\n'
    return 0
  fi

  local store occ outcome cand hash
  store="$repo_root/$STORE_REL"
  case "$test_id" in
    *'#'*) occ="$test_id" ;;
    *) occ="${test_id}#1" ;;
  esac
  outcome="$(store_last_field "$store" "$occ" outcome)"
  cand="$(store_last_field "$store" "$occ" candidateDigest)"
  hash="$(store_last_field "$store" "$occ" outputHash)"

  printf 'testOccurrenceId=%s\n' "$occ"
  printf 'claim=%s\n' "$claim"
  printf 'recordedOutcome=%s\n' "${outcome:-none}"

  if [ -z "$outcome" ]; then
    fail "no receipt for $occ: a claim about a leaf that was never executed is an assertion, not evidence" 2
  fi
  if [ "$outcome" = "BLOCKED_NOT_RUN" ]; then
    fail "$occ was BLOCKED_NOT_RUN and never executed; it cannot support '$claim'" 2
  fi
  # An unearned receipt is one whose execution left no trace. It is refused
  # before its outcome is consulted further, because the outcome of an execution
  # that produced no output hash is not a finding about production.
  if [ -z "$hash" ] || [ -z "$cand" ]; then
    fail "the receipt for $occ carries no output hash or no candidate digest; it was not earned by running the leaf" 2
  fi
  if [ "$outcome" = "UNRESOLVED" ]; then
    fail "$occ is UNRESOLVED (the time budget expired): it is neither a pass nor valid RED evidence, so it cannot support '$claim'" 2
  fi
  if [ "$claim" = "pass" ] && [ "$outcome" != "RAN_PASS" ]; then
    fail "$occ recorded $outcome; it cannot support a pass" 2
  fi
  if [ "$claim" = "red-evidence" ] && [ "$outcome" != "RAN_FAIL" ]; then
    fail "$occ recorded $outcome; it cannot support RED evidence" 2
  fi

  printf 'assert=supported\n'
  return 0
}

# --------------------------------------------------------------------------
# status
# --------------------------------------------------------------------------
cmd_status() {
  local repo_root="$PWD" test_id=''
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --test-id)
        [ "$#" -ge 2 ] || die_usage "--test-id requires a value"
        test_id="$2"
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

  if [ -n "$test_id" ]; then
    local occ outcome cand env_fp hash earned='false'
    case "$test_id" in
      *'#'*) occ="$test_id" ;;
      *) occ="${test_id}#1" ;;
    esac
    outcome="$(store_last_field "$store" "$occ" outcome)"
    cand="$(store_last_field "$store" "$occ" candidateDigest)"
    env_fp="$(store_last_field "$store" "$occ" environmentFingerprint)"
    hash="$(store_last_field "$store" "$occ" outputHash)"
    [ -n "$hash" ] && [ -n "$cand" ] && earned='true'
    printf 'testOccurrenceId=%s\n' "$occ"
    printf 'outcome=%s\n' "${outcome:-none}"
    printf 'candidateDigest=%s\n' "${cand:-none}"
    printf 'environmentFingerprint=%s\n' "${env_fp:-none}"
    printf 'outputHash=%s\n' "${hash:-none}"
    printf 'earned=%s\n' "$earned"
    if occurrence_resolving_outcome "${outcome:-}"; then
      printf 'resolved=true\n'
    else
      printf 'resolved=false\n'
    fi
  fi
  return 0
}

main() {
  local sub="${1:-}"
  if [ "$#" -gt 0 ]; then
    shift
  fi
  case "$sub" in
    resolve) cmd_resolve "$@" ;;
    run) cmd_run "$@" ;;
    assert) cmd_assert "$@" ;;
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
