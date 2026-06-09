#!/usr/bin/env bash
# Gate G097: requirement_mechanism_correspondence_gate
#
# Closes the "shape-not-semantics" hole that lets a requirement naming a
# concrete mechanism (PKCE, OAuth2, CSRF, HMAC, mTLS, ...) ship green even
# when the implementation never implements that mechanism. The existing
# certification gates each verify shape/existence, not requirement-to-code
# correspondence:
#   - G021 anti-fabrication verifies a command RAN, not that the claim matches
#     the requirement (tests can genuinely pass against a fake).
#   - G028 implementation-reality-scan verifies a real call is made, not that
#     it uses the named mechanism (a real call with the WRONG auth passes).
#   - traceability-guard verifies a test EXISTS for a scenario, not that it
#     asserts the CORRECT behavior.
#
# This guard pulls the one mechanical check that the reconcile/gaps sweep
# already proved works ("a requirement names a mechanism -> grep the code for
# it") forward from a later sweep into certification.
#
# DESIGN INTENT — warn-and-require-justification, NOT blind hard-block:
#   A requirement may legitimately be satisfied by a differently-named
#   mechanism. So a named-but-absent mechanism is cleared two ways:
#     (a) code evidence of the mechanism (or a known synonym) in the scope's
#         implementation files, OR
#     (b) an explicit justification disclosing the naming/scope difference.
#   Only a mechanism that is named in requirements with NEITHER code evidence
#   NOR a justification produces a blocking finding. That is honest disclosure
#   over mechanical green — the divergence is surfaced, never silently blessed,
#   and a legitimate naming difference is cleared by one disclosure line.
#
# Two additional advisory NUDGES (never change the exit code) surface the
# adjacent failure modes from the same incident:
#   #4 adversarial-assertion nudge: a security mechanism named in requirements
#      with no negative/rejection assertion in the scope's tests.
#   #3 fake-server-as-oracle nudge: a live-tier (integration/e2e) test whose
#      only server is an in-process fake (httptest.Server / MockWebServer /
#      WireMockServer) — it does not exercise the real external contract.
#
# Grandfather: specs whose state.json createdAt is absent or earlier than the
# cutoff are WARN-only, so a framework upgrade never retroactively blocks
# already-closed downstream work. Only new specs get blocking enforcement.
#
# Usage:
#   bash requirement-mechanism-guard.sh <feature-dir> [--quiet]
#
# Exit codes:
#   0  clean, not applicable, or grandfathered (advisory nudges may print)
#   1  G097 finding (named mechanism with no code evidence AND no justification)
#   2  runtime error / malformed input
#
set -uo pipefail

GRANDFATHER_CUTOFF="2026-06-08"

quiet="false"
feature_dir=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) quiet="true"; shift ;;
    -h|--help)
      sed -n '1,52p' "$0"
      exit 0
      ;;
    --*) echo "requirement-mechanism-guard: unknown option: $1" >&2; exit 2 ;;
    *)
      if [[ -n "$feature_dir" ]]; then
        echo "requirement-mechanism-guard: only one feature dir may be supplied" >&2
        exit 2
      fi
      feature_dir="$1"
      shift
      ;;
  esac
done

if [[ -z "$feature_dir" ]]; then
  echo "requirement-mechanism-guard: missing feature directory argument" >&2
  echo "Usage: bash requirement-mechanism-guard.sh <feature-dir> [--quiet]" >&2
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "requirement-mechanism-guard: feature directory not found: $feature_dir" >&2
  exit 2
fi

say() {
  [[ "$quiet" == "true" ]] && return 0
  echo "$1"
}

# ---------------------------------------------------------------------------
# Resolve a repo root so implementation paths declared in scope files (which
# are repo-relative) can be located. Prefer git; fall back to the path segment
# before /specs/; fall back to CWD.
# ---------------------------------------------------------------------------
repo_root="$(git -C "$feature_dir" rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "$repo_root" ]]; then
  if [[ "$feature_dir" == */specs/* ]]; then
    repo_root="${feature_dir%%/specs/*}"
  else
    repo_root="$(pwd)"
  fi
fi

resolve_path() {
  local candidate="$1"
  if [[ -f "$candidate" ]]; then
    printf '%s\n' "$candidate"
    return
  fi
  if [[ -f "$repo_root/$candidate" ]]; then
    printf '%s\n' "$repo_root/$candidate"
    return
  fi
  printf '%s\n' ""
}

# ---------------------------------------------------------------------------
# Collect scope files (single-file or per-scope-directory layout).
# ---------------------------------------------------------------------------
scope_files=()
if [[ -f "$feature_dir/scopes/_index.md" ]]; then
  while IFS= read -r scope_path; do
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' 2>/dev/null | sort)
elif [[ -f "$feature_dir/scopes.md" ]]; then
  scope_files=("$feature_dir/scopes.md")
fi

spec_md="$feature_dir/spec.md"
design_md="$feature_dir/design.md"
report_md="$feature_dir/report.md"
state_json="$feature_dir/state.json"

# Not applicable when there is no requirement surface to read.
if [[ ! -f "$spec_md" && ${#scope_files[@]} -eq 0 ]]; then
  say "ℹ️  G097: no spec.md or scope files in $feature_dir — requirement-mechanism check not applicable"
  exit 0
fi

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
else
  # ISO-8601 dates sort lexicographically; compare the YYYY-MM-DD prefix.
  if [[ "${created_at:0:10}" < "$GRANDFATHER_CUTOFF" ]]; then
    grandfathered="true"
  fi
fi

# ---------------------------------------------------------------------------
# Requirement corpus (what the spec/design/scopes PROMISE) and justification
# corpus (where a naming/scope difference may be disclosed).
# ---------------------------------------------------------------------------
req_corpus=""
[[ -f "$spec_md" ]] && req_corpus+="$(cat "$spec_md")"$'\n'
[[ -f "$design_md" ]] && req_corpus+="$(cat "$design_md")"$'\n'
for sf in "${scope_files[@]+"${scope_files[@]}"}"; do
  req_corpus+="$(cat "$sf")"$'\n'
done

# Justification corpus: the dedicated section in spec.md/report.md plus any
# inline "Mechanism-Justification:" lines anywhere in spec/design/report.
extract_justification_section() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  awk '
    /^##[[:space:]]+Requirement-Mechanism Justifications/ { in_sec=1; next }
    /^##[[:space:]]/ && in_sec { in_sec=0 }
    in_sec { print }
  ' "$file"
}
just_corpus=""
just_corpus+="$(extract_justification_section "$spec_md")"$'\n'
just_corpus+="$(extract_justification_section "$report_md")"$'\n'
# Inline single-line justifications (case-insensitive marker).
inline_just="$(grep -hiE 'mechanism-justification:' "$spec_md" "$design_md" "$report_md" 2>/dev/null || true)"
just_corpus+="$inline_just"$'\n'

# ---------------------------------------------------------------------------
# Implementation + test file discovery (same project-agnostic approach as
# implementation-reality-scan.sh: backtick-wrapped paths in scope files).
# ---------------------------------------------------------------------------
IMPL_DISCOVERY='`[^`]+\.(rs|ts|tsx|js|jsx|py|go|java|dart|scala|brs|sh|kt|rb|cs)\b[^`]*`'
TEST_DISCOVERY='`[^`]+(spec|test)[^`]*\.(rs|ts|tsx|js|jsx|py|go|java|dart|scala|brs|sh|kt|rb|cs)\b[^`]*`'

impl_files=()
test_files=()

add_unique() {
  # add_unique <array-name> <value>
  local -n arr="$1"
  local val="$2"
  local existing
  for existing in "${arr[@]+"${arr[@]}"}"; do
    [[ "$existing" == "$val" ]] && return 0
  done
  arr+=("$val")
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

for sf in "${scope_files[@]+"${scope_files[@]}"}"; do
  impl_section="$(awk '
    /^###[[:space:]]+Implementation Files$/ { in_impl=1; next }
    /^##[[:space:]]/ { in_impl=0 }
    /^###[[:space:]]/ && in_impl { in_impl=0 }
    in_impl { print }
  ' "$sf" 2>/dev/null || true)"
  while IFS= read -r p; do
    [[ -n "$p" ]] && add_unique impl_files "$p"
  done < <(collect_paths "$impl_section" "$IMPL_DISCOVERY")

  sf_text="$(cat "$sf")"
  while IFS= read -r tp; do
    [[ -n "$tp" ]] && add_unique test_files "$tp"
  done < <(collect_paths "$sf_text" "$TEST_DISCOVERY")
done

# Fallback: if no Implementation Files section yielded paths, mine the whole
# scope text for source-looking backtick paths (still bounded to declared files).
if [[ ${#impl_files[@]} -eq 0 ]]; then
  for sf in "${scope_files[@]+"${scope_files[@]}"}"; do
    while IFS= read -r p; do
      [[ -n "$p" ]] && add_unique impl_files "$p"
    done < <(collect_paths "$(cat "$sf")" "$IMPL_DISCOVERY")
  done
fi

# ---------------------------------------------------------------------------
# Mechanism catalog as four index-aligned arrays. Parallel arrays (not a
# delimiter-joined row) are used DELIBERATELY: every requirement/code regex
# contains its own `|` alternations, so any single-char field delimiter would
# mis-split them. All regexes are ERE, matched case-insensitively via grep -i.
# mech_sec[i]=1 marks a security mechanism (enables the #4 negative-assertion
# nudge).
# ---------------------------------------------------------------------------
mech_labels=(
  'PKCE'
  'OAuth2'
  'refresh_token'
  'CSRF'
  'HMAC'
  'mTLS'
  'SAML'
  'WebAuthn'
  'TOTP'
  'Content-Security-Policy'
  'HSTS'
  'Idempotency-Key'
)
mech_req=(
  '\bpkce\b|code[ _-]?verifier|code[ _-]?challenge'
  'oauth[ _-]?2|\boauth2\b'
  'refresh[ _-]?token'
  '\bcsrf\b|\bxsrf\b'
  '\bhmac\b'
  '\bmtls\b|mutual[ _-]?tls'
  '\bsaml\b'
  'webauthn|fido2'
  '\btotp\b|time-based one-time'
  'content[ _-]security[ _-]policy|\bcsp\b'
  '\bhsts\b|strict-transport-security'
  'idempotenc[ye][ _-]?key'
)
mech_code=(
  'pkce|code[ _-]?verifier|code[ _-]?challenge'
  'oauth2|oauth[ _-]?2|authorization[ _-]?code|\boauth\b'
  'refresh[ _-]?token'
  'csrf|xsrf|anti[ _-]?forgery|samesite'
  'hmac'
  'mtls|mutual[ _-]?tls|client[ _-]?cert'
  'saml'
  'webauthn|fido2'
  'totp|otpauth|pyotp'
  'content[ _-]security[ _-]policy'
  'strict-transport-security|hsts'
  'idempotenc[ye][ _-]?key'
)
mech_sec=(
  '1'
  '1'
  '1'
  '1'
  '1'
  '1'
  '1'
  '1'
  '1'
  '0'
  '0'
  '0'
)

# Negative/rejection-assertion patterns for the #4 nudge.
NEG_ASSERT='reject|rejected|\b401\b|\b403\b|unauthor|forbidden|invalid[ _-]?(token|signature|verifier|challenge|credential|key)|should[ _-]?(fail|error|reject)|must[ _-]?fail|expect[^A-Za-z]*(throw|error|reject)|assert[^A-Za-z]*(err|fail|false)|denied|tamper|wrong[ _-]?(token|key|secret|credential)'

# In-process fake HTTP server constructors for the #3 nudge.
FAKE_SERVER='httptest\.New(TLS)?Server|httptest\.NewUnstartedServer|net/http/httptest|MockWebServer|WireMockServer|new[[:space:]]+WireMock'

# code_evidence_present <code-regex>  -> 0 if any impl file matches.
# Pure comment lines are stripped before matching so a comment that merely
# NAMES the mechanism ("// TODO: PKCE", "// no PKCE yet", "/// uses HMAC") does
# not count as implementing it — that comment-as-evidence illusion is exactly
# the "looks done but isn't" shape this gate targets. Real implementations
# still match on their actual code lines (calls, identifiers, string literals).
code_evidence_present() {
  local rx="$1"
  local f
  for f in "${impl_files[@]+"${impl_files[@]}"}"; do
    [[ -f "$f" ]] || continue
    if grep -vE '^[[:space:]]*(//|#|\*|/\*|--|<!--|;;)' "$f" 2>/dev/null | grep -qiE "$rx"; then
      return 0
    fi
  done
  return 1
}

scope_tests_have_negative_assertion() {
  local f
  for f in "${test_files[@]+"${test_files[@]}"}"; do
    [[ -f "$f" ]] || continue
    if grep -qiE "$NEG_ASSERT" "$f" 2>/dev/null; then
      return 0
    fi
  done
  return 1
}

is_live_tier_test() {
  local test_path="$1"
  local base; base="$(basename "$test_path")"
  local sf matched
  for sf in "${scope_files[@]+"${scope_files[@]}"}"; do
    matched="$(grep -F "$test_path" "$sf" 2>/dev/null || true)"
    [[ -z "$matched" ]] && matched="$(grep -F "$base" "$sf" 2>/dev/null || true)"
    if printf '%s' "$matched" | grep -Eiq 'integration|e2e-api|e2e-ui|live-stack|live stack|live-system|live system|real-stack|real stack'; then
      return 0
    fi
  done
  return 1
}

# ---------------------------------------------------------------------------
# Evaluate each mechanism.
# ---------------------------------------------------------------------------
findings=0
nudges=0
named_count=0

for i in "${!mech_labels[@]}"; do
  label="${mech_labels[$i]}"
  req_rx="${mech_req[$i]}"
  code_rx="${mech_code[$i]}"
  security="${mech_sec[$i]}"

  # Is this mechanism NAMED in the requirement corpus?
  if ! printf '%s' "$req_corpus" | grep -qiE "$req_rx"; then
    continue
  fi
  named_count=$((named_count + 1))

  # Justification disclosed for this mechanism?
  justified="false"
  if printf '%s' "$just_corpus" | grep -qiE "$req_rx"; then
    justified="true"
  fi

  # Code evidence present?
  has_code="false"
  if [[ ${#impl_files[@]} -gt 0 ]] && code_evidence_present "$code_rx"; then
    has_code="true"
  fi

  if [[ "$has_code" == "true" ]]; then
    say "✅ G097: requirement names '$label' and implementation files show matching mechanism evidence"
  elif [[ "$justified" == "true" ]]; then
    say "✅ G097: requirement names '$label' without direct code evidence, but a Requirement-Mechanism justification discloses the difference"
  elif [[ ${#impl_files[@]} -eq 0 ]]; then
    # No declared implementation files to scan — G028 owns "zero impl files".
    say "ℹ️  G097: requirement names '$label' but no implementation files are declared in scope files; deferring to G028 (implementation-reality-scan) for file-presence enforcement"
  else
    echo "🔴 G097 BLOCK: requirement names mechanism '$label' but NO implementation file shows it (searched ${#impl_files[@]} file(s)) and NO Requirement-Mechanism justification discloses the difference"
    findings=$((findings + 1))
  fi

  # #4 advisory nudge — security mechanism without a negative assertion.
  if [[ "$security" == "1" && ${#test_files[@]} -gt 0 ]]; then
    if ! scope_tests_have_negative_assertion; then
      say "⚠️  G097 NUDGE: security mechanism '$label' is named but no negative/rejection assertion was found in the scope's tests — add an adversarial test that asserts the WRONG credential/token is rejected (the environment-independent case that fails if the bug is reintroduced)"
      nudges=$((nudges + 1))
    fi
  fi
done

# #3 advisory nudge — live-tier external-contract test backed only by an
# in-process fake server.
for tf in "${test_files[@]+"${test_files[@]}"}"; do
  [[ -f "$tf" ]] || continue
  if grep -qiE "$FAKE_SERVER" "$tf" 2>/dev/null && is_live_tier_test "$tf"; then
    say "⚠️  G097 NUDGE: live-tier test '$tf' is backed by an in-process fake server (httptest.Server / MockWebServer / WireMock) — it does not exercise the real external contract; mark the real path as genuinely-unverified or push the auth-tier enforcement into the test"
    nudges=$((nudges + 1))
  fi
done

# ---------------------------------------------------------------------------
# Verdict.
# ---------------------------------------------------------------------------
if [[ "$named_count" -eq 0 ]]; then
  say "✅ G097: no concrete security/contract mechanism named in requirements — not applicable"
  exit 0
fi

if [[ "$findings" -gt 0 ]]; then
  if [[ "$grandfathered" == "true" ]]; then
    say ""
    say "⚠️  G097: $findings requirement-mechanism correspondence gap(s) — DOWNGRADED to warning (spec createdAt '$created_at' is before cutoff $GRANDFATHER_CUTOFF or absent; grandfathered)."
    say "Remediation when this spec is next touched: add the mechanism to the implementation, OR add a '## Requirement-Mechanism Justifications' entry disclosing the naming/scope difference."
    exit 0
  fi
  echo ""
  echo "G097: $findings requirement-mechanism correspondence finding(s)."
  echo "Each finding is a mechanism named in requirements with NEITHER code evidence NOR a disclosed justification."
  echo "Remediate by ONE of:"
  echo "  (a) implement the named mechanism in the scope's implementation files, OR"
  echo "  (b) add a '## Requirement-Mechanism Justifications' section (in spec.md or report.md) — or a 'Mechanism-Justification: <mechanism> — <reason>' line — disclosing the differently-named mechanism or out-of-scope decision."
  echo "This is honest disclosure over mechanical green: a legitimate naming difference is cleared by one disclosure line; a silent gap is not."
  exit 1
fi

if [[ "$nudges" -gt 0 ]]; then
  say "✅ G097: requirement-mechanism correspondence satisfied ($named_count named mechanism(s)); $nudges advisory nudge(s) above are non-blocking."
else
  say "✅ G097: requirement-mechanism correspondence satisfied for $named_count named mechanism(s)."
fi
exit 0
