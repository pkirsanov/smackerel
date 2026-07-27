#!/usr/bin/env bash
# Gate G130: domain_invariant_correspondence_gate
#
# Advisory-until-configured sibling of G097 (requirement_mechanism_gate),
# applying the SAME proven "warn-and-require-justification" design to product
# DOMAIN INVARIANTS instead of named security/contract mechanisms. Where G097
# asks "a requirement names PKCE/OAuth2/HMAC — does the code implement it?",
# G130 asks "a domainModel declares a business invariant (e.g. Order.status ∈
# {created,paid,shipped,refunded}) — is it mechanically ANCHORED, or is it just
# prose?" A declared-but-unanchored invariant is exactly the "looks specified
# but is unenforceable" shape the existing certification gates miss:
#   - G021 verifies a command RAN, not that any invariant is enforced.
#   - G028 verifies a real call is MADE, not that a violating value is rejected.
#   - traceability-guard verifies a test EXISTS, not that it asserts rejection.
#
# OPT-IN (advisory-until-configured): this gate is INERT unless a repo declares
# an OPTIONAL `domainModel:` block in `.github/bubbles-project.yaml` (sibling of
# `traceContracts:`), inline OR as `domainModel: { $ref: config/domain-model.yaml }`.
# A repo with no `domainModel:` block is a CLEAN NO-OP (exit 0). The Bubbles
# source repo declares none, so this guard no-ops there.
#
# Opt-in schema:
#   domainModel:
#     entities:
#       Order: { states: [created, paid, shipped, refunded], terminal: [refunded] }
#     invariants:
#       - id: INV-ORDER-STATUS-ENUM
#         rule: "Order.status ∈ {created, paid, shipped, refunded}"
#         kind: enumeration
#         enforcedBy: [db-constraint, type]   # where the product enforces it
#         provedBy: ["tests/order_status_test.rs::rejects_unknown_status"]
#
# DESIGN INTENT — warn-and-require-justification, NOT blind hard-block (G097):
#   For each declared invariant, it is ANCHORED (cleared) by ANY of:
#     (a) code evidence of an `enforcedBy` mechanism token in the scope's
#         declared implementation files (same backtick-path extraction as
#         G028/G097), OR
#     (b) at least one linked `provedBy` test that is a genuine ADVERSARIAL
#         assertion — one that rejects the violating input (a reject/invalid/
#         rejected/`must fail`/`second_..._rejected`/negative-assertion pattern
#         in the named test or its file), OR
#     (c) an explicit disclosure — a `## Domain-Invariant Justifications`
#         section (in spec.md or report.md) or an
#         `Invariant-Justification: <INV-id> — <reason>` line.
#   Only an invariant with NEITHER enforcement NOR an adversarial proving test
#   NOR a justification is a BLOCKING finding. Honest disclosure over mechanical
#   green: a legitimately-external or deferred invariant is cleared by one line.
#
# Grandfather: specs whose state.json createdAt is absent or earlier than the
# cutoff are WARN-only, so adopting this gate never retroactively blocks
# already-closed work. Only new specs get blocking enforcement.
#
# Parsing: mikefarah `yq` (v4) when present; otherwise a bounded awk/grep
# fallback that handles the canonical flow-style and simple block-style
# invariant declarations and degrades to an advisory WARN (exit 0) on a shape
# it cannot confidently parse. There is NO --skip/--force/bypass.
#
# Usage:
#   bash domain-invariant-guard.sh <feature-dir> [--quiet]
#
# Exit codes:
#   0  clean, not applicable (no domainModel), or grandfathered
#   1  G130 finding (declared invariant with no anchor and no justification)
#   2  runtime error / malformed input
#
set -uo pipefail

GRANDFATHER_CUTOFF="2026-07-27"

quiet="false"
feature_dir=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) quiet="true"; shift ;;
    -h|--help)
      sed -n '1,65p' "$0"
      exit 0
      ;;
    --*) echo "domain-invariant-guard: unknown option: $1" >&2; exit 2 ;;
    *)
      if [[ -n "$feature_dir" ]]; then
        echo "domain-invariant-guard: only one feature dir may be supplied" >&2
        exit 2
      fi
      feature_dir="$1"
      shift
      ;;
  esac
done

if [[ -z "$feature_dir" ]]; then
  echo "domain-invariant-guard: missing feature directory argument" >&2
  echo "Usage: bash domain-invariant-guard.sh <feature-dir> [--quiet]" >&2
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "domain-invariant-guard: feature directory not found: $feature_dir" >&2
  exit 2
fi

# Canonicalize to an absolute path so path resolution is cwd-independent.
feature_dir_abs="$(cd "$feature_dir" 2>/dev/null && pwd -P)" || {
  echo "domain-invariant-guard: cannot resolve feature directory: $feature_dir" >&2
  exit 2
}
feature_dir="$feature_dir_abs"

say() {
  [[ "$quiet" == "true" ]] && return 0
  echo "$1"
}

# BUBBLES_DOMAIN_INVARIANT_DISABLE_YQ is a TEST-ONLY seam: it forces the awk/grep
# fallback parser so the fallback path is genuinely exercised by the selftest.
# It changes only the PARSER, never the enforcement — both parsers apply the
# identical anchor/justification/grandfather checks. It is not a gate bypass.
have_yq() {
  [[ -n "${BUBBLES_DOMAIN_INVARIANT_DISABLE_YQ:-}" ]] && return 1
  command -v yq >/dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Resolve the nearest .github/bubbles-project.yaml (or bubbles-project.yaml)
# walking up from the feature dir. The directory that CONTAINS it is the
# project root used to resolve repo-relative implementation/test paths.
# Emits "<project_root>\t<config_file>" on success.
# ---------------------------------------------------------------------------
find_project_config() {
  local dir="$feature_dir"
  while :; do
    if [[ -f "$dir/.github/bubbles-project.yaml" ]]; then
      printf '%s\t%s\n' "$dir" "$dir/.github/bubbles-project.yaml"
      return 0
    fi
    if [[ -f "$dir/bubbles-project.yaml" ]]; then
      printf '%s\t%s\n' "$dir" "$dir/bubbles-project.yaml"
      return 0
    fi
    [[ "$dir" == "/" ]] && break
    dir="$(dirname "$dir")"
  done
  return 1
}

pc="$(find_project_config || true)"
if [[ -z "$pc" ]]; then
  say "ℹ️  G130: no .github/bubbles-project.yaml above $feature_dir — no domainModel declared; domain-invariant check not applicable (advisory-until-configured no-op)"
  exit 0
fi
project_root="${pc%%$'\t'*}"
config_file="${pc#*$'\t'}"

config_has_domain_model() {
  if have_yq; then
    [[ "$(yq 'has("domainModel")' "$config_file" 2>/dev/null || echo false)" == "true" ]]
  else
    grep -qE '^domainModel:' "$config_file"
  fi
}

if ! config_has_domain_model; then
  say "ℹ️  G130: no domainModel block in $config_file — domain-invariant check not applicable (advisory-until-configured no-op)"
  exit 0
fi

# ---------------------------------------------------------------------------
# Resolve inline vs `$ref: config/domain-model.yaml`. Both forms resolve to a
# model_file whose invariants are read identically.
# ---------------------------------------------------------------------------
detect_ref() {
  if have_yq; then
    yq '.domainModel["$ref"] // ""' "$config_file" 2>/dev/null || true
  else
    awk '
      /^domainModel:/ { d = 1 }
      d && /\$ref:/ {
        line = $0
        sub(/.*\$ref:[[:space:]]*/, "", line)
        gsub(/[}"{[:space:]]/, "", line)
        gsub(/\047/, "", line)
        print line
        exit
      }
    ' "$config_file" 2>/dev/null || true
  fi
}

model_file="$config_file"
inv_root="domain"          # invariants at .domainModel.invariants
ref_path="$(detect_ref | tr -d '[:space:]')"
if [[ -n "$ref_path" ]]; then
  resolved_ref=""
  if [[ -f "$project_root/$ref_path" ]]; then
    resolved_ref="$project_root/$ref_path"
  elif [[ -f "$(dirname "$config_file")/$ref_path" ]]; then
    resolved_ref="$(dirname "$config_file")/$ref_path"
  elif [[ -f "$ref_path" ]]; then
    resolved_ref="$ref_path"
  fi
  if [[ -z "$resolved_ref" ]]; then
    say "⚠️  G130: domainModel.\$ref -> '$ref_path' not found (searched relative to $project_root and $(dirname "$config_file")); domain-invariant check cannot proceed — treating as not applicable (advisory)"
    exit 0
  fi
  model_file="$resolved_ref"
  inv_root="ref"
fi

spec_md="$feature_dir/spec.md"
report_md="$feature_dir/report.md"
design_md="$feature_dir/design.md"
state_json="$feature_dir/state.json"

# ---------------------------------------------------------------------------
# Grandfather: only specs created on/after the cutoff get blocking enforcement.
# ---------------------------------------------------------------------------
created_at=""
if [[ -f "$state_json" ]]; then
  created_at="$(grep -Eo '"createdAt"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_json" \
    | head -n 1 \
    | sed -E 's/.*"createdAt"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/' || true)"
fi
grandfathered="false"
if [[ -z "$created_at" ]]; then
  grandfathered="true"
elif [[ "${created_at:0:10}" < "$GRANDFATHER_CUTOFF" ]]; then
  grandfathered="true"
fi

# ---------------------------------------------------------------------------
# Justification corpus: the dedicated section in spec.md/report.md plus any
# inline "Invariant-Justification:" lines. An invariant is disclosed when its
# INV-id appears in this corpus.
# ---------------------------------------------------------------------------
extract_justification_section() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  awk '
    /^##[[:space:]]+Domain-Invariant Justifications/ { in_sec = 1; next }
    /^##[[:space:]]/ && in_sec { in_sec = 0 }
    in_sec { print }
  ' "$file"
}
just_corpus=""
just_corpus+="$(extract_justification_section "$spec_md")"$'\n'
just_corpus+="$(extract_justification_section "$report_md")"$'\n'
inline_just="$(grep -hiE 'invariant-justification:' "$spec_md" "$design_md" "$report_md" 2>/dev/null || true)"
just_corpus+="$inline_just"$'\n'

# ---------------------------------------------------------------------------
# Scope files + implementation-file discovery (same project-agnostic approach
# as implementation-reality-scan.sh / G097: backtick-wrapped paths in scope
# files). Implementation files back the enforcedBy code-evidence check.
# ---------------------------------------------------------------------------
scope_files=()
if [[ -f "$feature_dir/scopes/_index.md" ]]; then
  while IFS= read -r scope_path; do
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' 2>/dev/null | sort)
elif [[ -f "$feature_dir/scopes.md" ]]; then
  scope_files=("$feature_dir/scopes.md")
fi

IMPL_DISCOVERY='`[^`]+\.(rs|ts|tsx|js|jsx|py|go|java|dart|scala|brs|sh|kt|rb|cs|sql)\b[^`]*`'

resolve_path() {
  local candidate="$1"
  if [[ -f "$candidate" ]]; then
    printf '%s\n' "$candidate"
    return
  fi
  if [[ -f "$project_root/$candidate" ]]; then
    printf '%s\n' "$project_root/$candidate"
    return
  fi
  printf '%s\n' ""
}

collect_paths() {
  local text="$1"
  local pattern="$2"
  local raw norm resolved
  while IFS= read -r raw; do
    norm="${raw//\`/}"
    norm="${norm%%::*}"
    resolved="$(resolve_path "$norm")"
    [[ -n "$resolved" ]] && printf '%s\n' "$resolved"
  done < <(printf '%s\n' "$text" | grep -oE "$pattern" 2>/dev/null | sort -u || true)
}

impl_files=()
impl_add() {
  local val="$1" existing
  for existing in "${impl_files[@]+"${impl_files[@]}"}"; do
    [[ "$existing" == "$val" ]] && return 0
  done
  impl_files+=("$val")
}

for sf in "${scope_files[@]+"${scope_files[@]}"}"; do
  impl_section="$(awk '
    /^###[[:space:]]+Implementation Files$/ { in_impl = 1; next }
    /^##[[:space:]]/ { in_impl = 0 }
    /^###[[:space:]]/ && in_impl { in_impl = 0 }
    in_impl { print }
  ' "$sf" 2>/dev/null || true)"
  while IFS= read -r p; do
    [[ -n "$p" ]] && impl_add "$p"
  done < <(collect_paths "$impl_section" "$IMPL_DISCOVERY")
done

if [[ ${#impl_files[@]} -eq 0 ]]; then
  for sf in "${scope_files[@]+"${scope_files[@]}"}"; do
    while IFS= read -r p; do
      [[ -n "$p" ]] && impl_add "$p"
    done < <(collect_paths "$(cat "$sf")" "$IMPL_DISCOVERY")
  done
fi

# ---------------------------------------------------------------------------
# enforcedBy token -> code-search ERE. Well-known enforcement kinds get a small
# synonym expansion; an unknown token falls back to the literal token with any
# non-alphanumeric run treated as an interchangeable [ _-]? separator (which is
# also injection-safe: no input metachar survives). Matched case-insensitively
# against NON-comment lines of the impl files (a comment that merely NAMES the
# mechanism must not count as enforcing it — the same illusion G097 guards).
# ---------------------------------------------------------------------------
enforced_code_regex() {
  local token lower
  token="$1"
  lower="$(printf '%s' "$token" | tr '[:upper:]' '[:lower:]')"
  case "$lower" in
    *db-constraint*|*db_constraint*|*database-constraint*|*sql-constraint*|*constraint*)
      printf '%s' 'constraint|check[[:space:]]*\(|not[ _-]?null|foreign[ _-]?key|\bunique\b|references|alter[ _-]?table' ;;
    *check*)
      printf '%s' 'check[[:space:]]*\(|check[ _-]?constraint' ;;
    *enum*)
      printf '%s' '\benum\b|enumeration|#\[.*enum' ;;
    *not-null*|*not_null*|*notnull*)
      printf '%s' 'not[ _-]?null' ;;
    *unique*)
      printf '%s' '\bunique\b' ;;
    *foreign-key*|*foreign_key*|*fk*)
      printf '%s' 'foreign[ _-]?key|references' ;;
    *state-machine*|*state_machine*|*statemachine*|*fsm*|*transition*)
      printf '%s' 'transition|state[ _-]?machine|\bfsm\b|next[ _-]?state|can[ _-]?transition' ;;
    *validat*)
      printf '%s' 'validat' ;;
    *type-state*|*typestate*|*type-system*|*type_system*)
      printf '%s' 'newtype|type[ _-]?state|sealed|\benum\b' ;;
    *type*)
      printf '%s' '\benum\b|newtype|sealed|:[[:space:]]*[a-z0-9_]*(status|state|kind)' ;;
    *invariant*|*assert*)
      printf '%s' 'assert|invariant|\bensure\b|\brequire\b' ;;
    *range*|*bound*|*clamp*)
      printf '%s' 'clamp|\bmin\(|\bmax\(|bounds|within[ _-]?range' ;;
    *regex*|*pattern*|*format*)
      printf '%s' 'regex|regexp|\bpattern\b|matches\(' ;;
    *)
      printf '%s' "$lower" | sed -E 's/[^a-z0-9]+/[ _-]?/g' ;;
  esac
}

code_evidence_present() {
  local rx="$1" f
  for f in "${impl_files[@]+"${impl_files[@]}"}"; do
    [[ -f "$f" ]] || continue
    if grep -vE '^[[:space:]]*(//|#|\*|/\*|--|<!--|;;)' "$f" 2>/dev/null | grep -qiE "$rx"; then
      return 0
    fi
  done
  return 1
}

enf_has_code_evidence() {
  local csv="$1" tok rx
  [[ ${#impl_files[@]} -gt 0 ]] || return 1
  [[ -n "$csv" ]] || return 1
  local tokens=()
  IFS=',' read -r -a tokens <<< "$csv"
  for tok in "${tokens[@]+"${tokens[@]}"}"; do
    tok="${tok#"${tok%%[![:space:]]*}"}"
    tok="${tok%"${tok##*[![:space:]]}"}"
    tok="${tok//\"/}"
    tok="${tok//\'/}"
    [[ -n "$tok" ]] || continue
    rx="$(enforced_code_regex "$tok")"
    [[ -n "$rx" ]] || continue
    if code_evidence_present "$rx"; then
      return 0
    fi
  done
  return 1
}

# A genuine adversarial assertion: rejects a violating input.
ADVERSARIAL_RX='reject|rejected|invalid|refuse|refused|deny|denied|disallow|forbidden|not[ _-]?allow|must[ _-]?fail|should[ _-]?(fail|reject|error|deny)|unknown[ _-]?(status|state|value|variant)|violat|out[ _-]?of[ _-]?range|second_[a-z0-9_]*_rejected|duplicate|conflict|\b40[13]\b|assert[^a-z]*(err|fail|false|throw)|expect[^a-z]*(err|throw|reject|fail)|panic|is_err|with_?err|to[ _-]?throw|tothrow'

prv_has_adversarial() {
  local csv="$1" entry name path resolved
  [[ -n "$csv" ]] || return 1
  local entries=()
  IFS=',' read -r -a entries <<< "$csv"
  for entry in "${entries[@]+"${entries[@]}"}"; do
    entry="${entry#"${entry%%[![:space:]]*}"}"
    entry="${entry%"${entry##*[![:space:]]}"}"
    entry="${entry//\"/}"
    entry="${entry//\'/}"
    [[ -n "$entry" ]] || continue
    # (1) the named test function itself signals rejection.
    name="${entry##*::}"
    if [[ "$name" != "$entry" ]] && printf '%s' "$name" | grep -qiE "$ADVERSARIAL_RX"; then
      return 0
    fi
    # (2) the resolved test file contains an adversarial assertion.
    path="${entry%%::*}"
    resolved="$(resolve_path "$path")"
    if [[ -n "$resolved" && -f "$resolved" ]] && grep -qiE "$ADVERSARIAL_RX" "$resolved" 2>/dev/null; then
      return 0
    fi
  done
  return 1
}

# ---------------------------------------------------------------------------
# Extract invariants as a stream: "<id>|||<enforcedBy-csv>|||<provedBy-csv>".
# ---------------------------------------------------------------------------
extract_invariants_yq() {
  local base
  if [[ "$inv_root" == "ref" ]]; then
    if [[ "$(yq 'has("invariants")' "$model_file" 2>/dev/null || echo false)" == "true" ]]; then
      base='(.invariants // [])'
    elif [[ "$(yq '.domainModel | has("invariants")' "$model_file" 2>/dev/null || echo false)" == "true" ]]; then
      base='(.domainModel.invariants // [])'
    else
      return 0
    fi
  else
    base='(.domainModel.invariants // [])'
  fi
  yq "${base}[] | ((.id // \"\") + \"|||\" + ((.enforcedBy // []) | join(\",\")) + \"|||\" + ((.provedBy // []) | join(\",\")))" "$model_file" 2>/dev/null || true
}

extract_invariants_awk() {
  awk '
    function flush() {
      if (id != "") { print id "|||" enf "|||" prv }
      id = ""; enf = ""; prv = ""; curkey = ""
    }
    BEGIN { inv = 0; id = ""; enf = ""; prv = ""; curkey = "" }
    /^[[:space:]]*invariants:[[:space:]]*$/ { inv = 1; next }
    inv && /^[^[:space:]#]/ { flush(); inv = 0 }
    inv {
      if ($0 ~ /^[[:space:]]*-[[:space:]]+id:/) {
        flush()
        line = $0; sub(/.*id:[[:space:]]*/, "", line)
        gsub(/["\047[:space:]]/, "", line)
        id = line; curkey = ""; next
      }
      if ($0 ~ /enforcedBy:[[:space:]]*\[/) {
        line = $0; sub(/.*enforcedBy:[[:space:]]*\[/, "", line); sub(/\].*/, "", line)
        gsub(/[[:space:]]/, "", line)
        enf = line; curkey = ""; next
      }
      if ($0 ~ /provedBy:[[:space:]]*\[/) {
        line = $0; sub(/.*provedBy:[[:space:]]*\[/, "", line); sub(/\].*/, "", line)
        gsub(/[[:space:]]*,[[:space:]]*/, ",", line)
        sub(/^[[:space:]]+/, "", line); sub(/[[:space:]]+$/, "", line)
        prv = line; curkey = ""; next
      }
      if ($0 ~ /^[[:space:]]*enforcedBy:[[:space:]]*$/) { curkey = "enf"; next }
      if ($0 ~ /^[[:space:]]*provedBy:[[:space:]]*$/) { curkey = "prv"; next }
      if (curkey != "" && $0 ~ /^[[:space:]]*-[[:space:]]+/) {
        line = $0; sub(/^[[:space:]]*-[[:space:]]+/, "", line)
        sub(/^[[:space:]]+/, "", line); sub(/[[:space:]]+$/, "", line)
        if (curkey == "enf") { enf = (enf == "" ? line : enf "," line) }
        else if (curkey == "prv") { prv = (prv == "" ? line : prv "," line) }
        next
      }
      if ($0 ~ /^[[:space:]]*[A-Za-z_]+:/) { curkey = "" }
    }
    END { flush() }
  ' "$model_file" 2>/dev/null || true
}

extract_invariants() {
  if have_yq; then
    extract_invariants_yq
  else
    extract_invariants_awk
  fi
}

inv_stream="$(extract_invariants)"

if [[ -z "$(printf '%s' "$inv_stream" | tr -d '[:space:]')" ]]; then
  if ! have_yq; then
    say "⚠️  G130: a domainModel block is declared but the awk/grep fallback parsed no invariants (install mikefarah yq v4 for full parsing) — treating as not applicable (advisory)"
  else
    say "✅ G130: domainModel declares no invariants — not applicable"
  fi
  exit 0
fi

# ---------------------------------------------------------------------------
# Evaluate each declared invariant.
# ---------------------------------------------------------------------------
findings=0
named_count=0

while IFS= read -r inv_line; do
  [[ -n "$inv_line" ]] || continue
  inv_id="${inv_line%%|||*}"
  rest="${inv_line#*|||}"
  enf_csv="${rest%%|||*}"
  prv_csv="${rest#*|||}"
  inv_id="${inv_id//\"/}"
  inv_id="${inv_id//\'/}"
  inv_id="${inv_id#"${inv_id%%[![:space:]]*}"}"
  inv_id="${inv_id%"${inv_id##*[![:space:]]}"}"
  [[ -n "$inv_id" ]] || continue
  named_count=$((named_count + 1))

  justified="false"
  if printf '%s' "$just_corpus" | grep -Fq "$inv_id"; then
    justified="true"
  fi

  if enf_has_code_evidence "$enf_csv"; then
    say "✅ G130: invariant '$inv_id' is anchored by enforcedBy code evidence in the scope's implementation files"
  elif prv_has_adversarial "$prv_csv"; then
    say "✅ G130: invariant '$inv_id' is anchored by an adversarial provedBy test (rejects a violating input)"
  elif [[ "$justified" == "true" ]]; then
    say "✅ G130: invariant '$inv_id' has no code/adversarial-test anchor, but a Domain-Invariant justification discloses the decision"
  else
    echo "🔴 G130 BLOCK: domainModel invariant '$inv_id' is NOT mechanically anchored — NO enforcedBy code evidence in the scope's implementation files, NO adversarial provedBy test that rejects a violating input, and NO Domain-Invariant justification"
    findings=$((findings + 1))
  fi
done <<< "$inv_stream"

# ---------------------------------------------------------------------------
# Verdict.
# ---------------------------------------------------------------------------
if [[ "$named_count" -eq 0 ]]; then
  say "✅ G130: domainModel declares no parseable invariants — not applicable"
  exit 0
fi

if [[ "$findings" -gt 0 ]]; then
  if [[ "$grandfathered" == "true" ]]; then
    say ""
    say "⚠️  G130: $findings domain-invariant correspondence gap(s) — DOWNGRADED to warning (spec createdAt '$created_at' is before cutoff $GRANDFATHER_CUTOFF or absent; grandfathered)."
    say "Remediation when this spec is next touched: enforce the invariant in code (an enforcedBy mechanism), add an adversarial provedBy test that rejects a violating input, OR add a '## Domain-Invariant Justifications' entry."
    exit 0
  fi
  echo ""
  echo "G130: $findings domain-invariant correspondence finding(s)."
  echo "Each finding is a declared domainModel invariant with NEITHER enforcedBy code evidence, NOR an adversarial provedBy test that rejects a violating input, NOR a disclosed justification."
  echo "Remediate by ONE of:"
  echo "  (a) enforce the invariant in the scope's implementation files (an enforcedBy mechanism the guard can grep), OR"
  echo "  (b) add at least one provedBy test that ADVERSARIALLY asserts a violating input is rejected, OR"
  echo "  (c) add a '## Domain-Invariant Justifications' section (in spec.md or report.md) — or an 'Invariant-Justification: <INV-id> — <reason>' line — disclosing why the invariant is not mechanically anchored."
  echo "This is honest disclosure over mechanical green: a declared invariant that is prose-only is surfaced, never silently blessed."
  exit 1
fi

say "✅ G130: domain-invariant correspondence satisfied for $named_count declared invariant(s)."
exit 0
