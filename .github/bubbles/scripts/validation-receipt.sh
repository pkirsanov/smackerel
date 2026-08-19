#!/usr/bin/env bash
#
# validation-receipt.sh — the run receipt framework-validate leaves behind, and
# the fail-closed predicate release-check uses to decide whether it may skip
# re-running the suite (IMP-049 / SCOPE-2).
#
# WHY THIS EXISTS
# ---------------
# release-check.sh runs the COMPLETE framework-validate.sh as its first check. A
# measured full run is 3743s across 338 checks, so certifying a release pays the
# entire suite again on a tree a validate run may have proven minutes earlier.
# The visible cost is recorded in BUGS.md: BUG-032's four guard repairs are
# implemented and landed, and the packet stays open purely because no session
# has spent the ~2 CPU-hours to certify the current tree.
#
# WHAT A RECEIPT IS
# -----------------
# A statement of the form "at time T, tier X of framework-validate reached
# verdict V about a tree whose content digest was D, on a toolchain whose
# fingerprint was F". It is NOT a claim about the tree in front of the consumer.
# The consumer RE-DERIVES D and F from the tree and environment it actually has
# and refuses unless both match, so a receipt cannot certify a tree it never
# saw. Nothing here is signed, and it does not need to be: forging a receipt
# requires write access to the working tree, and anyone with that can edit the
# validator itself.
#
# FAIL CLOSED, ALWAYS
# -------------------
# Every uncertainty resolves to "run the validation". Absent, unreadable,
# unparseable, wrong-schema, wrong-producer, wrong-version, failing, wrong-tier,
# partial, cache-assisted, expired, digest-mismatched, or toolchain-mismatched
# all return non-zero from validation_receipt_accept. There is no flag that
# forces acceptance. The failure mode this ordering protects against —
# certifying an unvalidated tree — is far worse than the 3743s it saves.
#
# CONSUMPTION IS OPT-IN (default OFF)
# -----------------------------------
# BUBBLES_RELEASE_CHECK_ACCEPT_RECEIPT=1 enables it. Unset or any other value
# means release-check behaves exactly as it does today. This follows the
# framework's own default-off resolver idiom (mutation-resolve.sh,
# test-inventory-resolve.sh) for the same reason: a reuse path that turns itself
# on changes what `release-check` MEANS for every existing caller without their
# consent. The measured beneficiary is the local operator loop, where the same
# person just ran framework-validate on the same tree; CI checks out fresh and
# has no receipt to consume either way.
#
# Environment:
#   BUBBLES_VALIDATION_RECEIPT_DIR         receipt directory
#                                          (default <repo>/.specify/runtime/validation-receipts)
#   BUBBLES_RELEASE_CHECK_ACCEPT_RECEIPT   1 enables consumption (default off)
#   BUBBLES_RELEASE_CHECK_RECEIPT_MAX_AGE_SECONDS
#                                          receipt expiry (default 86400)
#
# CLI (used by the selftest and for operator inspection):
#   validation-receipt.sh path      <repo-root>
#   validation-receipt.sh digest    <repo-root>
#   validation-receipt.sh toolchain
#   validation-receipt.sh write     <repo-root> <tier> <verdict> <checks> <duration> <changedOnly> <cacheEnabled>
#   validation-receipt.sh invalidate <repo-root>
#   validation-receipt.sh accept    <repo-root> <required-tier>
#
# Exit codes (accept): 0 accept - 1 refuse (reason printed) - 2 usage error

# NO `set` at file scope. This file is SOURCED by framework-validate.sh and
# release-check.sh, both of which run under `set -euo pipefail`; changing shell
# options here would silently reconfigure the caller. Options are set only on
# the CLI path at the bottom.

_VALIDATION_RECEIPT_SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! declare -F bubbles_sha256_stdin >/dev/null 2>&1; then
  # shellcheck source=bubbles/scripts/trust-metadata.sh
  source "$_VALIDATION_RECEIPT_SELF_DIR/trust-metadata.sh"
fi

VALIDATION_RECEIPT_SCHEMA_VERSION=1
VALIDATION_RECEIPT_PRODUCER="framework-validate.sh"
VALIDATION_RECEIPT_DIGEST_ALGORITHM="manifest-inventory-content-v1"

# validation_receipt_dir <repo-root>
validation_receipt_dir() {
  local repo_root="${1:-}"
  printf '%s' "${BUBBLES_VALIDATION_RECEIPT_DIR:-$repo_root/.specify/runtime/validation-receipts}"
}

# validation_receipt_path <repo-root>
validation_receipt_path() {
  printf '%s/framework-validate.json' "$(validation_receipt_dir "${1:-}")"
}

# validation_receipt_tree_digest <repo-root>
#
# WHAT THIS COVERS. The release manifest already tracks the framework's own file
# set with a per-file sha256 (bubbles/release-manifest.json — 919 managed plus
# the source-only inventory). This re-hashes every one of those paths FROM DISK
# and folds the manifest's own hash in, so the digest describes the bytes that
# are there NOW rather than the bytes the manifest remembers. Using the manifest
# only as the FILE LIST is the point: a stale manifest can misstate a hash, but
# it cannot make this function read a file that is not on disk.
#
# WHAT THIS DOES NOT COVER, stated plainly:
#   - any file absent from both manifest inventories: specs/, bugs/,
#     improvements/, .specify/runtime/**, and every untracked or newly added
#     file that has not been folded into the manifest yet. A new file changes
#     the manifest when it is regenerated, and release-check runs the manifest
#     freshness gate separately, but between those two moments a brand-new
#     untracked file is invisible here.
#   - the environment. That is why validation_receipt_toolchain_digest exists
#     and is compared as a separate precondition.
#   - anything non-deterministic the suite touches: the clock, the network, git
#     history depth, and the contents of .specify/runtime.
# A missing inventory file is folded in as the literal token MISSING rather than
# skipped, so deleting a tracked file changes the digest instead of vanishing.
validation_receipt_tree_digest() {
  local repo_root="${1:-}"
  [[ -n "$repo_root" ]] || return 1
  # Both layouts, in source-first order. The manifest is at bubbles/ in a source
  # checkout and .github/bubbles/ in an installed one, and framework-validate --
  # which is shipped and sources this file -- runs in both. Resolving relative to
  # repo_root rather than to this script keeps synthetic fixture roots working.
  local manifest="$repo_root/bubbles/release-manifest.json"
  [[ -r "$manifest" ]] || manifest="$repo_root/.github/bubbles/release-manifest.json"
  [[ -r "$manifest" ]] || return 1

  local -a entries=()
  mapfile -t entries < <(
    grep -oE '"path"[[:space:]]*:[[:space:]]*"[^"]*"' "$manifest" 2>/dev/null \
      | sed -E 's/.*"([^"]*)"$/\1/' \
      | LC_ALL=C sort -u
  )
  [[ "${#entries[@]}" -gt 0 ]] || return 1

  # One batched hash pass over the whole inventory. The batch tool drops entries
  # it cannot read, so its output is indexed by path rather than trusted
  # positionally — pairing a path with another file's hash is exactly the silent
  # corruption this digest exists to detect.
  local -A hashes=()
  local hash path
  while IFS=$'\t' read -r hash path; do
    [[ -n "$path" ]] || continue
    hashes["$path"]="$hash"
  done < <(printf '%s\n' "${entries[@]}" | bubbles_sha256_batch "$repo_root" 2>/dev/null || true)

  {
    printf '#algorithm\t%s\n' "$VALIDATION_RECEIPT_DIGEST_ALGORITHM"
    printf '#entries\t%s\n' "${#entries[@]}"
    printf '#manifest\t%s\n' "$(bubbles_sha256_file "$manifest")"
    for path in "${entries[@]}"; do
      if [[ -n "${hashes[$path]:-}" ]]; then
        printf '%s\t%s\n' "$path" "${hashes[$path]}"
      elif [[ -f "$repo_root/$path" ]]; then
        printf '%s\t%s\n' "$path" "$(bubbles_sha256_file "$repo_root/$path")"
      else
        printf '%s\tMISSING\n' "$path"
      fi
    done
  } | bubbles_sha256_stdin
}

# validation_receipt_toolchain_digest
#
# The tree digest says nothing about the machine, and framework-validate's
# verdict depends on the machine: shellcheck and shfmt versions change what
# lints report, and a missing optional tool turns checks into skips. Comparing
# this is what stops a receipt written on the Linux CI runner from being
# consumed on the macOS runner — two DIFFERENT jobs on two fresh checkouts,
# which is what the "CI pays it twice" cost actually is.
#
# `sed` and `timeout` are deliberately EXCLUDED. framework-validate prepends a
# mktemp compat directory that aliases gsed/gtimeout onto those names, so their
# resolved identity differs between a run inside the validator and a run outside
# it. Including them would make every receipt mismatch on macOS for a reason
# that has nothing to do with correctness.
validation_receipt_toolchain_digest() {
  {
    printf 'os\t%s\n' "$(uname -s 2>/dev/null || echo unknown)"
    printf 'arch\t%s\n' "$(uname -m 2>/dev/null || echo unknown)"
    printf 'bash\t%s\n' "${BASH_VERSION:-unknown}"
    local tool version
    for tool in shellcheck shfmt yq jq python3 node git; do
      if command -v "$tool" >/dev/null 2>&1; then
        version="$("$tool" --version 2>/dev/null | head -1 | tr -d '\r')"
        printf '%s\t%s\n' "$tool" "${version:-present}"
      else
        printf '%s\tabsent\n' "$tool"
      fi
    done
  } | bubbles_sha256_stdin
}

# validation_receipt_invalidate <repo-root>
#
# Called by framework-validate BEFORE its first check. A run that is in flight,
# or one that dies partway, must not leave the PREVIOUS receipt standing: the
# tree can be byte-identical to the one that passed last time while this run is
# discovering a failure, and a surviving pass-receipt would be consumed on a
# tree whose verdict has just changed. Removing first makes the receipt describe
# only runs that reached an end.
validation_receipt_invalidate() {
  local repo_root="${1:-}"
  [[ -n "$repo_root" ]] || return 1
  local receipt
  receipt="$(validation_receipt_path "$repo_root")"
  [[ -e "$receipt" ]] || return 0
  rm -f "$receipt" 2>/dev/null || true
  [[ ! -e "$receipt" ]]
}

# validation_receipt_write <repo-root> <tier> <verdict> <checks> <duration> <changedOnly> <cacheEnabled>
validation_receipt_write() {
  local repo_root="${1:-}" tier="${2:-}" verdict="${3:-}"
  local checks="${4:-0}" duration="${5:-0}"
  local changed_only="${6:-false}" cache_enabled="${7:-false}"
  [[ -n "$repo_root" && -n "$tier" && -n "$verdict" ]] || return 2

  local dir receipt tree_digest toolchain_digest version git_sha
  dir="$(validation_receipt_dir "$repo_root")"
  receipt="$(validation_receipt_path "$repo_root")"
  mkdir -p "$dir" 2>/dev/null || return 1

  tree_digest="$(validation_receipt_tree_digest "$repo_root" 2>/dev/null || true)"
  [[ -n "$tree_digest" ]] || return 1
  toolchain_digest="$(validation_receipt_toolchain_digest 2>/dev/null || true)"
  [[ -n "$toolchain_digest" ]] || return 1

  version="unknown"
  [[ -f "$repo_root/VERSION" ]] && version="$(tr -d '[:space:]' <"$repo_root/VERSION" 2>/dev/null || echo unknown)"
  git_sha="$(git -C "$repo_root" rev-parse HEAD 2>/dev/null || echo unknown)"

  local tmp
  tmp="$(mktemp "$dir/.receipt.XXXXXX")" || return 1
  {
    printf '{\n'
    printf '  "schemaVersion": %s,\n' "$VALIDATION_RECEIPT_SCHEMA_VERSION"
    printf '  "producer": "%s",\n' "$VALIDATION_RECEIPT_PRODUCER"
    printf '  "frameworkVersion": "%s",\n' "$version"
    printf '  "tier": "%s",\n' "$tier"
    printf '  "verdict": "%s",\n' "$verdict"
    printf '  "checksExecuted": %s,\n' "$checks"
    printf '  "durationSeconds": %s,\n' "$duration"
    printf '  "changedOnly": %s,\n' "$changed_only"
    printf '  "cacheEnabled": %s,\n' "$cache_enabled"
    printf '  "recordedAt": "%s",\n' "$(bubbles_current_timestamp)"
    printf '  "recordedAtEpoch": %s,\n' "$(date -u +%s)"
    printf '  "gitSha": "%s",\n' "$git_sha"
    printf '  "treeDigestAlgorithm": "%s",\n' "$VALIDATION_RECEIPT_DIGEST_ALGORITHM"
    printf '  "treeDigest": "%s",\n' "$tree_digest"
    printf '  "toolchainDigest": "%s"\n' "$toolchain_digest"
    printf '}\n'
  } >"$tmp" || {
    rm -f "$tmp"
    return 1
  }
  mv -f "$tmp" "$receipt" 2>/dev/null || {
    rm -f "$tmp"
    return 1
  }
}

# _validation_receipt_tier_satisfies <recorded-tier> <required-tier>
# Tier order is core < full. An unrecognised tier satisfies nothing.
_validation_receipt_tier_satisfies() {
  local recorded="${1:-}" required="${2:-}"
  local recorded_rank required_rank
  case "$recorded" in
    core) recorded_rank=1 ;;
    full) recorded_rank=2 ;;
    *) return 1 ;;
  esac
  case "$required" in
    core) required_rank=1 ;;
    full) required_rank=2 ;;
    *) return 1 ;;
  esac
  [[ "$recorded_rank" -ge "$required_rank" ]]
}

# validation_receipt_accept <repo-root> <required-tier>
#
# Prints ONE line describing the decision on stdout, so the caller can echo it
# verbatim and the operator can see WHY the suite was or was not re-run.
# Returns 0 only when every precondition holds.
validation_receipt_accept() {
  local repo_root="${1:-}" required_tier="${2:-full}"
  [[ -n "$repo_root" ]] || {
    echo "receipt: refused (no repository root given)"
    return 1
  }

  if [[ "${BUBBLES_RELEASE_CHECK_ACCEPT_RECEIPT:-}" != "1" ]]; then
    echo "receipt: not consulted (BUBBLES_RELEASE_CHECK_ACCEPT_RECEIPT is not 1 — reuse is opt-in)"
    return 1
  fi

  local receipt
  receipt="$(validation_receipt_path "$repo_root")"
  if [[ ! -r "$receipt" ]]; then
    echo "receipt: refused (no readable receipt at $receipt)"
    return 1
  fi

  local schema producer version tier verdict changed_only cache_enabled
  local recorded_epoch recorded_at recorded_tree recorded_toolchain
  schema="$(bubbles_json_number_field "$receipt" schemaVersion)"
  producer="$(bubbles_json_string_field "$receipt" producer)"
  version="$(bubbles_json_string_field "$receipt" frameworkVersion)"
  tier="$(bubbles_json_string_field "$receipt" tier)"
  verdict="$(bubbles_json_string_field "$receipt" verdict)"
  changed_only="$(bubbles_json_bool_field "$receipt" changedOnly)"
  cache_enabled="$(bubbles_json_bool_field "$receipt" cacheEnabled)"
  recorded_epoch="$(bubbles_json_number_field "$receipt" recordedAtEpoch)"
  recorded_at="$(bubbles_json_string_field "$receipt" recordedAt)"
  recorded_tree="$(bubbles_json_string_field "$receipt" treeDigest)"
  recorded_toolchain="$(bubbles_json_string_field "$receipt" toolchainDigest)"

  # One combined emptiness test. Any missing field means the file did not parse
  # as the receipt shape, and an unparseable receipt is indistinguishable from a
  # hand-written one.
  if [[ -z "$schema" || -z "$producer" || -z "$tier" || -z "$verdict" ||
    -z "$changed_only" || -z "$cache_enabled" || -z "$recorded_epoch" ||
    -z "$recorded_tree" || -z "$recorded_toolchain" ]]; then
    echo "receipt: refused (unparseable — a required field is missing)"
    return 1
  fi
  if [[ "$schema" != "$VALIDATION_RECEIPT_SCHEMA_VERSION" ]]; then
    echo "receipt: refused (schemaVersion $schema, this consumer understands $VALIDATION_RECEIPT_SCHEMA_VERSION)"
    return 1
  fi
  if [[ "$producer" != "$VALIDATION_RECEIPT_PRODUCER" ]]; then
    echo "receipt: refused (producer '$producer', expected '$VALIDATION_RECEIPT_PRODUCER')"
    return 1
  fi
  if [[ "$verdict" != "pass" ]]; then
    echo "receipt: refused (verdict '$verdict')"
    return 1
  fi
  if ! _validation_receipt_tier_satisfies "$tier" "$required_tier"; then
    echo "receipt: refused (tier '$tier' does not satisfy required tier '$required_tier')"
    return 1
  fi
  if [[ "$changed_only" != "false" ]]; then
    echo "receipt: refused (recorded run used --changed-only, so it did not execute every check)"
    return 1
  fi
  if [[ "$cache_enabled" != "false" ]]; then
    echo "receipt: refused (recorded run used the result cache, so some verdicts were reused)"
    return 1
  fi

  local current_version="unknown"
  [[ -f "$repo_root/VERSION" ]] && current_version="$(tr -d '[:space:]' <"$repo_root/VERSION" 2>/dev/null || echo unknown)"
  if [[ "$version" != "$current_version" ]]; then
    echo "receipt: refused (framework version '$version', tree is '$current_version')"
    return 1
  fi

  local max_age now age
  max_age="${BUBBLES_RELEASE_CHECK_RECEIPT_MAX_AGE_SECONDS:-86400}"
  now="$(date -u +%s)"
  age=$((now - recorded_epoch))
  if [[ "$age" -lt 0 || "$age" -gt "$max_age" ]]; then
    echo "receipt: refused (recorded $recorded_at, age ${age}s exceeds the ${max_age}s limit)"
    return 1
  fi

  local current_tree current_toolchain
  current_tree="$(validation_receipt_tree_digest "$repo_root" 2>/dev/null || true)"
  if [[ -z "$current_tree" ]]; then
    echo "receipt: refused (cannot derive a tree digest from this repository)"
    return 1
  fi
  if [[ "$current_tree" != "$recorded_tree" ]]; then
    echo "receipt: refused (tree digest ${current_tree:0:16}… does not match the validated ${recorded_tree:0:16}…)"
    return 1
  fi
  current_toolchain="$(validation_receipt_toolchain_digest 2>/dev/null || true)"
  if [[ -z "$current_toolchain" || "$current_toolchain" != "$recorded_toolchain" ]]; then
    echo "receipt: refused (toolchain fingerprint differs from the one that ran the validation)"
    return 1
  fi

  echo "receipt: accepted (tier=$tier verdict=pass recorded=$recorded_at age=${age}s tree=${recorded_tree:0:16}…) — not re-running framework-validate"
  return 0
}

# --- CLI -------------------------------------------------------------------
# Present so the selftest and an operator can exercise every function without
# reproducing its logic. Sourcing this file runs none of it.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  set -uo pipefail
  case "${1:-}" in
    path) validation_receipt_path "${2:?repo-root required}" && echo ;;
    digest) validation_receipt_tree_digest "${2:?repo-root required}" ;;
    toolchain) validation_receipt_toolchain_digest ;;
    invalidate) validation_receipt_invalidate "${2:?repo-root required}" ;;
    write)
      validation_receipt_write "${2:?repo-root required}" "${3:?tier required}" \
        "${4:?verdict required}" "${5:-0}" "${6:-0}" "${7:-false}" "${8:-false}"
      ;;
    accept) validation_receipt_accept "${2:?repo-root required}" "${3:-full}" ;;
    -h | --help | "")
      sed -n '3,60p' "${BASH_SOURCE[0]}"
      exit 0
      ;;
    *)
      echo "validation-receipt: unknown subcommand '$1'." >&2
      exit 2
      ;;
  esac
fi
