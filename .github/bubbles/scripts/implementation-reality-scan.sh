#!/usr/bin/env bash
# =============================================================================
# implementation-reality-scan.sh
# =============================================================================
# Scans implementation source files for stub, fake, hardcoded data, and
# default/fallback patterns that violate the "Real Implementation" and
# "No Defaults/No Fallbacks" policies.
#
# This script is PROJECT-AGNOSTIC — it works on any repo. It scans files
# referenced in scopes.md (or per-scope scope.md) for violations.
#
# Scans:
#   1. Backend stub patterns (hardcoded vecs, fake/mock/stub functions)
#   2. Handler/endpoint execution-depth failures (public surface, no real delegation)
#   3. Frontend fake data (getSimulationData, mock imports, hardcoded arrays)
#   4. Frontend API/client signal absence (hooks/services with zero fetch/query/client signals)
#   5. Prohibited simulation helpers in production (seeded_pick/seeded_range)
#   6. Default/fallback value patterns (unwrap_or, || default, ?? fallback)
#   7. Live-system tests using request interception/mocked backends
#   8. Sensitive client storage of auth/session/payment secrets
#   9. IDOR / auth bypass — user identity from request body instead of auth context
#   10. Silent decode failures — deserialization errors silently discarded
#
# Usage:
#   bash bubbles/scripts/implementation-reality-scan.sh <feature-dir> [--verbose]
#
# Exit codes:
#   0 = No violations found
#   1 = Violations detected (blocking)
#   2 = Usage error
#
# Called automatically by state-transition-guard.sh (Check 15).
# Can also be run standalone for pre-completion self-audit.
# =============================================================================
set -euo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

feature_dir="${1:-}"
verbose="false"

for arg in "$@"; do
  if [[ "$arg" == "--verbose" ]]; then
    verbose="true"
  fi
done

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/implementation-reality-scan.sh specs/<NNN-feature-name> [--verbose]"
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

violations=0
warnings=0
scanned_files=0

violation() {
  local file="$1"
  local line_num="$2"
  local pattern="$3"
  local context="$4"
  echo "🔴 VIOLATION [$pattern] $file:$line_num"
  if [[ "$verbose" == "true" ]]; then
    echo "   Context: $context"
  fi
  fun_fail
  violations=$((violations + 1))
}

vwarn() {
  local message="$1"
  echo "⚠️  WARN: $message"
  fun_warn
  warnings=$((warnings + 1))
}

pass() {
  local message="$1"
  echo "✅ PASS: $message"
}

info() {
  local message="$1"
  echo "ℹ️  INFO: $message"
}

# =============================================================================
# Detect scope layout and collect scope files
# =============================================================================
scope_files=()
if [[ -f "$feature_dir/scopes/_index.md" ]]; then
  while IFS= read -r scope_path; do
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' 2>/dev/null | sort)
elif [[ -f "$feature_dir/scopes.md" ]]; then
  scope_files=("$feature_dir/scopes.md")
fi

if [[ ${#scope_files[@]} -eq 0 ]]; then
  echo "ERROR: No scope files found in $feature_dir"
  exit 2
fi

# =============================================================================
# Extract implementation source file paths from scope files
# =============================================================================

# resolve_path: given a potentially bare filename or relative path, attempt to
# find the actual file on disk. Returns resolved path or empty string.
resolve_path() {
  local candidate="$1"
  # If it already exists as given, use it directly
  if [[ -f "$candidate" ]]; then
    echo "$candidate"
    return
  fi
  # If the path has no directory component (bare filename like "ml.rs"),
  # search common source directories for it.
  if [[ "$candidate" != */* ]]; then
    local found
    found="$(find services/gateway/src/handlers \
                  services/gateway/src/scripting/api \
                  services/ \
                  libs/ \
             -maxdepth 5 -name "$candidate" -type f 2>/dev/null | head -1 || true)"
    if [[ -n "$found" ]]; then
      echo "$found"
      return
    fi
  fi
  echo ""
}

# add_impl_file: deduplicate and add to impl_files array
add_impl_file() {
  local path="$1"
  for existing in "${impl_files[@]+"${impl_files[@]}"}"; do
    if [[ "$existing" == "$path" ]]; then
      return
    fi
  done
  impl_files+=("$path")
}

impl_files=()
test_files=()

IMPL_DISCOVERY_PATTERN='`[^`]+\.(rs|ts|tsx|js|jsx|py|go|java|dart|scala|brs|sh|yaml|yml|json|md)\b[^`]*`'
TEST_DISCOVERY_PATTERN='`[^`]+(spec|test)[^`]*\.(rs|ts|tsx|js|jsx|py|go|java|dart|scala|brs|sh)\b[^`]*`'

normalize_declared_path() {
  local raw_path="$1"

  raw_path="${raw_path//\`/}"
  printf '%s\n' "${raw_path%%::*}"
}

resolve_declared_paths_from_text() {
  local source_text="$1"
  local discovery_pattern="$2"
  local raw_path=""
  local normalized_path=""
  local resolved_path=""

  while IFS= read -r raw_path; do
    normalized_path="$(normalize_declared_path "$raw_path")"
    resolved_path="$(resolve_path "$normalized_path")"
    if [[ -n "$resolved_path" ]]; then
      printf '%s\n' "$resolved_path"
    fi
  done < <(printf '%s\n' "$source_text" | grep -oE "$discovery_pattern" 2>/dev/null | sort -u || true)
}

add_test_file() {
  local path="$1"
  for existing in "${test_files[@]+"${test_files[@]}"}"; do
    if [[ "$existing" == "$path" ]]; then
      return
    fi
  done
  test_files+=("$path")
}

# Batch-extract all backtick-wrapped file paths from each scope file using
# a single grep pass per file (avoids per-line subprocess overhead on large
# scopes.md files).
for scope_file in "${scope_files[@]}"; do
  implementation_section="$({ awk '
    /^### Implementation Files$/ { in_impl=1; next }
    /^## / { in_impl=0 }
    /^### / && in_impl { in_impl=0 }
    in_impl { print }
  ' "$scope_file"; } || true)"

  while IFS= read -r resolved; do
    add_impl_file "$resolved"
  done < <(resolve_declared_paths_from_text "$implementation_section" "$IMPL_DISCOVERY_PATTERN")

  scope_text="$(<"$scope_file")"
  while IFS= read -r resolved_test; do
    add_test_file "$resolved_test"
  done < <(resolve_declared_paths_from_text "$scope_text" "$TEST_DISCOVERY_PATTERN")
done

is_live_system_test_file() {
  local test_path="$1"
  local base_name
  base_name="$(basename "$test_path")"

  for scope_file in "${scope_files[@]}"; do
    matched_lines="$(grep -F "$test_path" "$scope_file" 2>/dev/null || true)"
    if [[ -z "$matched_lines" ]]; then
      matched_lines="$(grep -F "$base_name" "$scope_file" 2>/dev/null || true)"
    fi

    if echo "$matched_lines" | grep -Eiq 'integration|e2e-api|e2e-ui|live-stack|live stack|live-system|live system|real-stack|real stack'; then
      return 0
    fi
  done

  return 1
}

# =============================================================================
# Fallback: If scopes yielded zero files, try design.md
# =============================================================================
if [[ ${#impl_files[@]} -eq 0 ]] && [[ -f "$feature_dir/design.md" ]]; then
  info "Scopes yielded 0 files — falling back to design.md for file discovery"
  design_text="$(<"$feature_dir/design.md")"
  while IFS= read -r resolved; do
    add_impl_file "$resolved"
  done < <(resolve_declared_paths_from_text "$design_text" "$IMPL_DISCOVERY_PATTERN")
  if [[ ${#impl_files[@]} -gt 0 ]]; then
    vwarn "Resolved ${#impl_files[@]} file(s) from design.md fallback — scopes.md should reference these directly"
  fi
fi

if [[ ${#impl_files[@]} -eq 0 ]]; then
  echo "🔴 VIOLATION [ZERO_FILES_RESOLVED] No implementation file paths resolved from scope files"
  echo ""
  echo "  This means scopes.md / scope.md files either:"
  echo "    1. Do not reference implementation files in backtick-wrapped paths, or"
  echo "    2. Reference files that do not exist on disk"
  echo ""
  echo "  Scanning nothing = no assurance. This is a blocking failure."
  echo "  Fix: Ensure scopes.md lists implementation files as \`path/to/file.ext\`"
  echo ""
  echo "============================================================"
  echo "  REALITY SCAN RESULT: 1 violation(s), 0 warning(s)"
  echo "  Files scanned: 0"
  echo "============================================================"
  exit 1
fi

info "Resolved ${#impl_files[@]} implementation file(s) to scan"
echo ""

# =============================================================================
# SCAN 1: Gateway/Backend Stub Detection
# =============================================================================
# Detects handlers that return hardcoded/seeded data instead of real
# backend service calls. Common patterns:
#   - vec![StructName { field: "literal" }]  (Rust hardcoded vec)
#   - return Ok(vec![...])  with inline struct construction
#   - generate_*, simulate_*, seed_*, fake_* function calls in handlers
#   - Static arrays/slices returned directly from handler functions
# =============================================================================
echo "--- Scan 1: Gateway/Backend Stub Patterns ---"

BACKEND_STUB_PATTERNS=(
  # Rust: hardcoded vec construction in handler-like contexts
  'vec!\[.*{.*:.*"'
  # Rust: returning seeded/simulated/fake data function calls
  'generate_fake\|generate_mock\|generate_stub\|generate_sample'
  'simulate_.*data\|simulation_data\|get_simulation\|getSimulation'
  'seed_data\|seeded_data\|fake_data\|mock_data\|sample_data\|dummy_data'
  'hardcoded_\|HARDCODED_\|SAMPLE_DATA\|MOCK_DATA\|FAKE_DATA\|DUMMY_DATA'
  # Any language: static/const arrays used as response data
  'static.*RESPONSES\|static.*ITEMS\|static.*RECORDS\|static.*ENTRIES'
  'const.*RESPONSES\|const.*ITEMS\|const.*RECORDS\|const.*ENTRIES'
  # Functions explicitly named as stubs/fakes
  'fn fake_\|fn mock_\|fn stub_\|fn placeholder_'
  'function fake\|function mock\|function stub\|function placeholder'
  'def fake_\|def mock_\|def stub_\|def placeholder_'
)

for impl_file in "${impl_files[@]}"; do
  scanned_files=$((scanned_files + 1))
  file_ext="${impl_file##*.}"

  # Only scan backend files (rs, go, py, java) for backend stub patterns
  if [[ "$file_ext" == "rs" || "$file_ext" == "go" || "$file_ext" == "py" || "$file_ext" == "java" || "$file_ext" == "scala" ]]; then
    # Skip test files — stubs/mocks in test files are a separate concern
    if echo "$impl_file" | grep -qE '(test|spec|_test\.rs|_test\.go|test_)'; then
      continue
    fi

    # Find the #[cfg(test)] boundary — lines after this are test code (Rust)
    cfg_test_line=999999
    if [[ "$file_ext" == "rs" ]]; then
      local_cfg="$(grep -n '#\[cfg(test)\]' "$impl_file" 2>/dev/null | head -1 | cut -d: -f1 || true)"
      if [[ -n "$local_cfg" ]]; then cfg_test_line=$local_cfg; fi
    fi

    for pattern in "${BACKEND_STUB_PATTERNS[@]}"; do
      while IFS=: read -r line_num matched_line; do
        # Skip lines in #[cfg(test)] module
        if [[ "$line_num" -ge "$cfg_test_line" ]]; then
          continue
        fi
        # Skip comment lines
        if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*)'; then
          continue
        fi
        violation "$impl_file" "$line_num" "BACKEND_STUB" "$matched_line"
      done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
    done

    # Multi-line vec! detection (Rust only): vec![ on one line, struct or
    # json macro with string literals on subsequent lines.
    #
    # Approved patterns that are NOT violations:
    #   1. Lines annotated with "// @catalog" — static API metadata
    #   2. vec! within ±30 lines of SimPrng/OrnsteinUhlenbeck/MarkovChain
    #      usage — approved stochastic simulation, not hardcoded data
    if [[ "$file_ext" == "rs" ]]; then
      while IFS=: read -r line_num matched_line; do
        # Skip lines in #[cfg(test)] module
        if [[ "$line_num" -ge "$cfg_test_line" ]]; then
          continue
        fi
        if echo "$matched_line" | grep -qE '^\s*(//|/\*|\*)'; then
          continue
        fi
        # Skip lines annotated as approved static metadata
        if echo "$matched_line" | grep -qE '@catalog'; then
          continue
        fi
        # Skip vec! within ±30 lines of approved simulation model usage
        file_len=$(wc -l < "$impl_file")
        sim_start=$((line_num - 30))
        if [[ $sim_start -lt 1 ]]; then sim_start=1; fi
        sim_end=$((line_num + 30))
        if [[ $sim_end -gt $file_len ]]; then sim_end=$file_len; fi
        has_sim="$(sed -n "${sim_start},${sim_end}p" "$impl_file" | grep -cE 'SimPrng|OrnsteinUhlenbeck|MarkovChain|from_seed_str' || true)"
        if [[ "$has_sim" -gt 0 ]]; then
          continue
        fi
        # Check if any of the next 5 lines contain struct field: "literal"
        # or serde_json key-value pairs with string values (need 3+ to be
        # considered hardcoded data, not just an ID field)
        end_line=$((line_num + 5))
        if [[ $end_line -gt $file_len ]]; then end_line=$file_len; fi
        has_struct_literal="$(sed -n "$((line_num + 1)),${end_line}p" "$impl_file" | grep -cE '[a-z_]+:\s*"|"[a-z_]+":\s*"' || true)"
        if [[ "$has_struct_literal" -ge 3 ]]; then
          violation "$impl_file" "$line_num" "HARDCODED_VEC_STRUCT" "$matched_line (struct with string literals follows)"
        fi
      done < <(grep -nE 'vec!\[' "$impl_file" 2>/dev/null | grep -vE '^\s*(//|/\*|\*)' | grep -vE 'vec!\[\s*\]' || true)
    fi
  fi
done
echo ""

# =============================================================================
# SCAN 1B: Handler / Endpoint Execution Depth
# =============================================================================
# Detects public handler/endpoint/controller files that return shaped success
# payloads but show no evidence of real delegation/query/execution.
# =============================================================================
echo "--- Scan 1B: Handler / Endpoint Execution Depth ---"

HANDLER_FILE_PATTERNS='handler|handlers|controller|controllers|route|routes|endpoint|endpoints|api|grpc'
HANDLER_SKIP_PATTERNS='health|metrics|readiness|liveness'
DELEGATION_PATTERNS='service\.|repo\.|repository\.|client\.|query\(|execute\(|dispatch\(|fetch\(|send\(|call\(|invoke\(|workflow\.|usecase\.|domain\.|storage\.|db\.|sql|grpc|httpClient|apiClient|axios\.|requests\.'
STATUS_ONLY_PATTERNS='["'"'']status["'"''][[:space:]]*:[[:space:]]*["'"''](ready|ok|success)["'"'']|status[[:space:]]*[:=][[:space:]]*["'"''](ready|ok|success)["'"'']|json!\(|serde_json::json|return[[:space:]]+.*(Ok|Some)\('
UNUSED_PARAM_PATTERNS='fn[[:space:]].*[(,][[:space:]]*_[A-Za-z0-9_]+|function[[:space:]].*[(,][[:space:]]*_[A-Za-z0-9_]+|def[[:space:]].*[(,][[:space:]]*_[A-Za-z0-9_]+'

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  if [[ "$file_ext" == "rs" || "$file_ext" == "go" || "$file_ext" == "py" || "$file_ext" == "java" || "$file_ext" == "scala" || "$file_ext" == "ts" || "$file_ext" == "tsx" || "$file_ext" == "js" || "$file_ext" == "jsx" ]]; then
    if echo "$impl_file" | grep -qE '(test|spec|_test\.|\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
      continue
    fi

    if ! echo "$impl_file" | grep -qiE "$HANDLER_FILE_PATTERNS"; then
      continue
    fi

    if echo "$impl_file" | grep -qiE "$HANDLER_SKIP_PATTERNS"; then
      continue
    fi

    delegation_count="$(grep -cE "$DELEGATION_PATTERNS" "$impl_file" 2>/dev/null || true)"
    status_only_count="$(grep -cE "$STATUS_ONLY_PATTERNS" "$impl_file" 2>/dev/null || true)"

    if [[ "$delegation_count" -eq 0 && "$status_only_count" -gt 0 ]]; then
      violation "$impl_file" "0" "EXECUTION_DEPTH" "Handler-like file exposes success/response construction but shows zero delegation/query/execution signals"
    fi

    while IFS=: read -r line_num matched_line; do
      if [[ -z "$line_num" ]]; then
        continue
      fi
      if [[ "$delegation_count" -eq 0 ]] && [[ "$status_only_count" -gt 0 ]]; then
        violation "$impl_file" "$line_num" "UNUSED_HANDLER_INPUT" "$matched_line"
      fi
    done < <(grep -nE "$UNUSED_PARAM_PATTERNS" "$impl_file" 2>/dev/null || true)
  fi
done
echo ""

# =============================================================================
# SCAN 1C: Endpoint Not-Implemented / Placeholder Responses
# =============================================================================
# Detects handler-like files that still return 501/not-implemented style
# placeholders while a scope claims the behavior is delivered.
# =============================================================================
echo "--- Scan 1C: Endpoint Not-Implemented / Placeholder Responses ---"

ENDPOINT_NOT_IMPLEMENTED_PATTERNS=(
  'StatusCode::NOT_IMPLEMENTED'
  'http\.StatusNotImplemented'
  'status\.StatusNotImplemented'
  '501([^0-9]|$)'
  'Not Implemented'
  'throw new Error\('
  'not implemented'
  'return .*not implemented'
  'unimplemented!\('
  'todo!\('
)

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  if [[ "$file_ext" == "rs" || "$file_ext" == "go" || "$file_ext" == "py" || "$file_ext" == "java" || "$file_ext" == "scala" || "$file_ext" == "ts" || "$file_ext" == "tsx" || "$file_ext" == "js" || "$file_ext" == "jsx" ]]; then
    if echo "$impl_file" | grep -qE '(test|spec|_test\.|\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
      continue
    fi

    if ! echo "$impl_file" | grep -qiE "$HANDLER_FILE_PATTERNS"; then
      continue
    fi

    if echo "$impl_file" | grep -qiE "$HANDLER_SKIP_PATTERNS"; then
      continue
    fi

    for pattern in "${ENDPOINT_NOT_IMPLEMENTED_PATTERNS[@]}"; do
      while IFS=: read -r line_num matched_line; do
        [[ -z "$line_num" ]] && continue
        if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*)'; then
          continue
        fi
        violation "$impl_file" "$line_num" "ENDPOINT_NOT_IMPLEMENTED" "$matched_line"
      done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
    done
  fi
done
echo ""

# =============================================================================
# SCAN 1D: External Integration Authenticity
# =============================================================================
# Detects provider/adapter/integration files that show suspicious fake/no-op
# behavior without any sign of real upstream calls or SDK/client delegation.
# =============================================================================
echo "--- Scan 1D: External Integration Authenticity ---"

INTEGRATION_FILE_PATTERNS='provider|providers|adapter|adapters|integration|integrations|connector|connectors|client|clients'
INTEGRATION_EXTERNAL_CALL_PATTERNS='fetch\(|axios\.|requests\.|httpClient|apiClient|client\.|send\(|post\(|get\(|put\(|delete\(|patch\(|request\(|invoke\(|grpc|sdk|smtp|webhook|oauth|mail|email|sms'
INTEGRATION_SUSPICIOUS_PATTERNS=(
  'Math\.random'
  'randomUUID'
  'uuid4'
  'rand::'
  'generate_.*code'
  'noop'
  'no-op'
  'mock'
  'fake'
  'sample'
  'dummy'
  'return[[:space:]]+Ok\(\(\)\)'
  'return[[:space:]]+nil'
  'return[[:space:]]+None'
  'return[[:space:]]+null'
  'Promise\.resolve\('
)

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  if [[ "$file_ext" == "rs" || "$file_ext" == "go" || "$file_ext" == "py" || "$file_ext" == "java" || "$file_ext" == "scala" || "$file_ext" == "ts" || "$file_ext" == "tsx" || "$file_ext" == "js" || "$file_ext" == "jsx" ]]; then
    if echo "$impl_file" | grep -qE '(test|spec|_test\.|\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
      continue
    fi

    if ! echo "$impl_file" | grep -qiE "$INTEGRATION_FILE_PATTERNS"; then
      continue
    fi

    external_call_count="$(grep -cE "$INTEGRATION_EXTERNAL_CALL_PATTERNS" "$impl_file" 2>/dev/null || true)"
    if [[ "$external_call_count" -gt 0 ]]; then
      continue
    fi

    for pattern in "${INTEGRATION_SUSPICIOUS_PATTERNS[@]}"; do
      while IFS=: read -r line_num matched_line; do
        [[ -z "$line_num" ]] && continue
        if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*)'; then
          continue
        fi
        violation "$impl_file" "$line_num" "FAKE_INTEGRATION" "$matched_line"
      done < <(grep -nEi "$pattern" "$impl_file" 2>/dev/null || true)
    done
  fi
done
echo ""

# =============================================================================
# SCAN 2: Frontend Hardcoded Data Detection
# =============================================================================
# Detects frontend code using hardcoded/simulation data instead of real
# API calls. Common patterns:
#   - getSimulationData() calls
#   - Hooks with zero API/query/client signals that return static data
#   - Hardcoded arrays/objects used as component data sources
#   - Import of simulation/mock data modules in production code
# =============================================================================
echo "--- Scan 2: Frontend Hardcoded Data Patterns ---"

FRONTEND_FAKE_PATTERNS=(
  # Simulation data function calls
  'getSimulationData\|getSimulation\|useSimulationData\|useSimulation'
  'getMockData\|useMockData\|getFakeData\|useFakeData'
  'getSampleData\|useSampleData\|getDummyData\|useDummyData'
  # Import of simulation/mock/fake data modules
  'import.*simulat\|import.*mockData\|import.*fakeData\|import.*sampleData'
  'from.*simulat.*import\|from.*mock_data\|from.*fake_data\|from.*sample_data'
  'require.*simulat\|require.*mockData\|require.*fakeData'
  # Hardcoded data constants used as data sources
  'MOCK_.*=\s*\[\|FAKE_.*=\s*\[\|SAMPLE_.*=\s*\[\|DUMMY_.*=\s*\['
  'SIMULATION_.*=\s*\[\|HARDCODED_.*=\s*\['
  'mockData\s*=\s*\[\|fakeData\s*=\s*\[\|sampleData\s*=\s*\['
  'simulationData\s*=\s*\[\|dummyData\s*=\s*\['
  # Static data generation in hooks
  'generate_fake\|generate_mock\|generate_stub\|generate_sample'
)

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  # Only scan frontend files (ts, tsx, js, jsx, dart) for frontend patterns
  if [[ "$file_ext" == "ts" || "$file_ext" == "tsx" || "$file_ext" == "js" || "$file_ext" == "jsx" || "$file_ext" == "dart" ]]; then
    # Skip test files
    if echo "$impl_file" | grep -qE '(\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
      continue
    fi

    for pattern in "${FRONTEND_FAKE_PATTERNS[@]}"; do
      while IFS=: read -r line_num matched_line; do
        # Skip comment lines
        if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*|{/\*)'; then
          continue
        fi
        violation "$impl_file" "$line_num" "FRONTEND_FAKE_DATA" "$matched_line"
      done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
    done
  fi
done
echo ""

# =============================================================================
# SCAN 2B: Sensitive Client Storage
# =============================================================================
# Detects auth/session/payment/secret material persisted in client-side storage.
# =============================================================================
echo "--- Scan 2B: Sensitive Client Storage ---"

SENSITIVE_CLIENT_STORAGE_PATTERNS=(
  'localStorage\.(setItem|getItem).*(token|auth|session|jwt|refresh|bearer|secret|api[_-]?key|card|payment|cvv|cvc|ssn)'
  'sessionStorage\.(setItem|getItem).*(token|auth|session|jwt|refresh|bearer|secret|api[_-]?key|card|payment|cvv|cvc|ssn)'
  'AsyncStorage\.(setItem|getItem).*(token|auth|session|jwt|refresh|bearer|secret|api[_-]?key|card|payment|cvv|cvc|ssn)'
  'SharedPreferences.*(token|auth|session|jwt|refresh|bearer|secret|api[_-]?key|card|payment|cvv|cvc|ssn)'
  'indexedDB.*(token|auth|session|jwt|refresh|bearer|secret|api[_-]?key|card|payment|cvv|cvc|ssn)'
  '(token|auth|session|jwt|refresh|bearer|secret|api[_-]?key|card|paymentMethod|cvv|cvc|ssn).*(localStorage|sessionStorage|AsyncStorage|SharedPreferences|indexedDB)'
)

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  if [[ "$file_ext" == "ts" || "$file_ext" == "tsx" || "$file_ext" == "js" || "$file_ext" == "jsx" || "$file_ext" == "dart" ]]; then
    if echo "$impl_file" | grep -qE '(\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
      continue
    fi

    for pattern in "${SENSITIVE_CLIENT_STORAGE_PATTERNS[@]}"; do
      while IFS=: read -r line_num matched_line; do
        [[ -z "$line_num" ]] && continue
        if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*|{/\*)'; then
          continue
        fi
        violation "$impl_file" "$line_num" "SENSITIVE_CLIENT_STORAGE" "$matched_line"
      done < <(grep -nEi "$pattern" "$impl_file" 2>/dev/null || true)
    done
  fi
done
echo ""

# =============================================================================
# SCAN 3: Frontend API Call Absence Detection
# =============================================================================
# Detects frontend hook/service files that should make API calls but don't.
# A "data hook" or "service" file that has zero network/query/client
# signals is likely returning hardcoded or simulated data.
#
# Heuristic: files matching *hook*.ts, *service*.ts, use*.ts, *api*.ts
# that contain zero occurrences of:
#   - direct calls (fetch, axios, .get/.post/.request)
#   - query hooks (useQuery/useMutation/useSWR variants)
#   - client transports/imports (apiClient/httpClient/*Client/*Api/*Transport)
# =============================================================================
echo "--- Scan 3: Frontend API Call Absence ---"

API_CALL_PATTERNS='fetch\(|axios(\.|\b)|\.(get|post|put|delete|patch|request)\(|use(Query|Mutation|SuspenseQuery|InfiniteQuery|SWR)\b|mutateAsync\(|httpClient\b|apiClient\b|queryClient\b|grpc\b|protobuf\b|create(Api|Http)Client\b|requestClient\b|transport\b|client\.(get|post|put|delete|patch|request|query|mutate)\('
API_IMPORT_PATTERNS='^import[[:space:]].*((api|client|transport|query)[A-Za-z0-9_]*|[A-Za-z0-9_]*(Api|Client|Transport|Query))[[:space:]]+from[[:space:]]+["\x27][^"\x27]+["\x27]|^import[[:space:]].*from[[:space:]]+["\x27][^"\x27]*(api|client|transport|query)[^"\x27]*["\x27]|require\(["\x27][^"\x27]*(api|client|transport|query)[^"\x27]*["\x27]\)'

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"
  file_basename="$(basename "$impl_file")"

  # Only check frontend data-fetching files
  if [[ "$file_ext" == "ts" || "$file_ext" == "tsx" || "$file_ext" == "js" || "$file_ext" == "jsx" ]]; then
    # Skip test files
    if echo "$impl_file" | grep -qE '(\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
      continue
    fi

    # Check if this looks like a data-fetching file (hook, service, api)
    is_data_file="false"
    if echo "$file_basename" | grep -qiE '(hook|service|api|fetch|data|store|query|use[A-Z])'; then
      is_data_file="true"
    fi
    # Also check if file contains "export function use" or "export const use" (custom hook pattern)
    if grep -qE 'export\s+(function|const)\s+use[A-Z]' "$impl_file" 2>/dev/null; then
      is_data_file="true"
    fi

    if [[ "$is_data_file" == "true" ]]; then
      api_call_count="$(grep -cE "$API_CALL_PATTERNS" "$impl_file" 2>/dev/null || true)"
      api_import_count="$(grep -cEi "$API_IMPORT_PATTERNS" "$impl_file" 2>/dev/null || true)"
      if [[ "$api_call_count" -eq 0 && "$api_import_count" -eq 0 ]]; then
        violation "$impl_file" "0" "NO_API_CALLS" "Data hook/service file has ZERO API-call or client-transport signals — likely returning hardcoded data"
      fi
    fi
  fi
done
echo ""

# =============================================================================
# SCAN 4: seeded_pick / seeded_range in production Rust code
# =============================================================================
# Some projects prohibit deterministic simulation helpers in production code
# while still allowing them in test code.
# This scan is only active if the patterns are found in scope-referenced files.
# =============================================================================
echo "--- Scan 4: Prohibited Simulation Helpers in Production ---"

PROHIBITED_SIM_PATTERNS=(
  'seeded_pick\|seeded_range\|seed_from_str'
)

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  # Only scan Rust production files
  if [[ "$file_ext" == "rs" ]]; then
    if echo "$impl_file" | grep -qE '(test|spec|_test\.rs|tests/)'; then
      continue
    fi
    for pattern in "${PROHIBITED_SIM_PATTERNS[@]}"; do
      while IFS=: read -r line_num matched_line; do
        if echo "$matched_line" | grep -qE '^\s*(//|/\*|\*)'; then
          continue
        fi
        violation "$impl_file" "$line_num" "PROHIBITED_SIM_HELPER" "$matched_line"
      done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
    done
  fi
done
echo ""

# =============================================================================
# SCAN 5: Default/Fallback Value Detection in Production Code
# =============================================================================
# Detects production code that uses default values, fallbacks, or silent
# recovery instead of failing fast on missing config/data.
# These patterns hide failures and violate the "No Defaults" policy.
#
# Language-specific patterns:
#   Rust:    unwrap_or(), unwrap_or_default(), unwrap_or_else(|| default)
#   Go:      getEnv("K", "fallback"), os.Getenv with || fallback
#   Python:  os.getenv("K", "default"), .get("k", default)
#   TS/JS:   || "default", ?? "fallback", || 'fallback'
#   Shell:   ${VAR:-default}
# =============================================================================
echo "--- Scan 5: Default/Fallback Value Patterns ---"

RUST_DEFAULT_PATTERNS=(
  'unwrap_or\b\|unwrap_or_default\b\|unwrap_or_else'
  'or_else.*Ok\|or_else.*Some'
)

GO_DEFAULT_PATTERNS=(
  'getEnv.*,.*"[^"]\+'
  'Getenv.*\|\|'
  'LookupEnv.*default\|LookupEnv.*fallback'
)

TS_JS_DEFAULT_PATTERNS=(
  'process\.env\.\w\+\s*[|][|]'
  'import\.meta\.env\.\w\+\s*[|][|]'
  'import\.meta\.env\.\w\+\s*\?\?'
  'env\.\w\+\s*\?\?'
  'env\.\w\+\s*[|][|]'
)

PYTHON_DEFAULT_PATTERNS=(
  'os\.getenv.*,\s*[^)]+'
  'os\.environ\.get.*,\s*[^)]+'
  '\.get\(.*,\s*.*default'
)

for impl_file in "${impl_files[@]}"; do
  file_ext="${impl_file##*.}"

  # Skip test files
  if echo "$impl_file" | grep -qE '(test|spec|_test\.|\.test\.|\.spec\.|__tests__|__mocks__|e2e|playwright)'; then
    continue
  fi

  case "$file_ext" in
    rs)
      for pattern in "${RUST_DEFAULT_PATTERNS[@]}"; do
        while IFS=: read -r line_num matched_line; do
          if echo "$matched_line" | grep -qE '^\s*(//|/\*|\*)'; then continue; fi
          violation "$impl_file" "$line_num" "DEFAULT_FALLBACK" "$matched_line"
        done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
      done
      ;;
    go)
      for pattern in "${GO_DEFAULT_PATTERNS[@]}"; do
        while IFS=: read -r line_num matched_line; do
          if echo "$matched_line" | grep -qE '^\s*(//|/\*|\*)'; then continue; fi
          violation "$impl_file" "$line_num" "DEFAULT_FALLBACK" "$matched_line"
        done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
      done
      ;;
    ts|tsx|js|jsx)
      for pattern in "${TS_JS_DEFAULT_PATTERNS[@]}"; do
        while IFS=: read -r line_num matched_line; do
          if echo "$matched_line" | grep -qE '^\s*(//|/\*|\*|{/\*)'; then continue; fi
          violation "$impl_file" "$line_num" "DEFAULT_FALLBACK" "$matched_line"
        done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
      done
      ;;
    py)
      for pattern in "${PYTHON_DEFAULT_PATTERNS[@]}"; do
        while IFS=: read -r line_num matched_line; do
          if echo "$matched_line" | grep -qE '^\s*#'; then continue; fi
          violation "$impl_file" "$line_num" "DEFAULT_FALLBACK" "$matched_line"
        done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
      done
      ;;
  esac
done
echo ""

# =============================================================================
# SCAN 6: Live-System Test Interception Detection
# =============================================================================
# Detects tests labeled as integration/e2e/live-stack that intercept requests
# or inject mocked backend responses, which reclassifies them out of live
# system categories.
# =============================================================================
echo "--- Scan 6: Live-System Test Interception ---"

LIVE_TEST_INTERCEPT_PATTERNS=(
  'page\.route'
  'context\.route'
  'cy\.intercept'
  'intercept\('
  'msw'
  'nock'
  'wiremock'
  'responses\.'
  'httpretty'
)

live_test_files_found=0
for test_file in "${test_files[@]}"; do
  if [[ ! -f "$test_file" ]]; then
    continue
  fi

  if ! is_live_system_test_file "$test_file"; then
    continue
  fi

  live_test_files_found=$((live_test_files_found + 1))
  for pattern in "${LIVE_TEST_INTERCEPT_PATTERNS[@]}"; do
    while IFS=: read -r line_num matched_line; do
      if [[ -z "$line_num" ]]; then
        continue
      fi
      if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*)'; then
        continue
      fi
      violation "$test_file" "$line_num" "LIVE_TEST_INTERCEPT" "$matched_line"
    done < <(grep -nE "$pattern" "$test_file" 2>/dev/null || true)
  done
done

if [[ "$live_test_files_found" -eq 0 ]]; then
  info "No live-system test files referenced in scope artifacts for interception scan"
fi
echo ""

# =============================================================================
# SCAN 7: IDOR / Auth Bypass Detection (Gate G047)
# =============================================================================
# Detects handlers that extract user/org/tenant identity from request body
# or query params instead of from authenticated context (auth middleware,
# JWT claims, session). This is an IDOR vulnerability — callers can
# impersonate other users by changing the ID in the request body.
#
# CORRECT:   userId := ctx.Value("authenticated_user_id")
# INCORRECT: userId := body.UserId  (caller-controlled!)
#
# Patterns are loaded from .github/bubbles-project.yaml if available,
# otherwise generic defaults are used. Projects can override:
#   scans.idor.bodyIdentityPatterns   — regex patterns for body identity extraction
#   scans.idor.authContextPatterns    — regex patterns for correct auth context usage
#   scans.idor.handlerFilePatterns    — how to identify handler/controller files
# =============================================================================
echo "--- Scan 7: IDOR / Auth Bypass Detection (Gate G047) ---"

# Auto-generate project config if missing (just-in-time, fully automatic)
PROJECT_CONFIG=".github/bubbles-project.yaml"
SETUP_SCRIPT="$(dirname "${BASH_SOURCE[0]}")/project-scan-setup.sh"
if [[ ! -f "$PROJECT_CONFIG" ]] || ! grep -q '^scans:' "$PROJECT_CONFIG" 2>/dev/null; then
  if [[ -f "$SETUP_SCRIPT" ]]; then
    info "Auto-generating .github/bubbles-project.yaml (first-time project scan setup)..."
    bash "$SETUP_SCRIPT" --quiet 2>/dev/null || true
  fi
fi
IDOR_BODY_PATTERNS=()
IDOR_AUTH_PATTERNS=""
IDOR_HANDLER_FILTER=""

if [[ -f "$PROJECT_CONFIG" ]]; then
  # Extract bodyIdentityPatterns from YAML (simple line-based parsing)
  while IFS= read -r pat; do
    pat="$(echo "$pat" | sed 's/^[[:space:]]*-[[:space:]]*//' | sed 's/^"//' | sed 's/"$//' | sed "s/^'//" | sed "s/'$//")"
    [[ -n "$pat" ]] && IDOR_BODY_PATTERNS+=("$pat")
  done < <(sed -n '/scans:/,/^[^ ]/{ /idor:/,/^    [^ ]/{/bodyIdentityPatterns:/,/^      [^ ]/{/^        -/p}}}' "$PROJECT_CONFIG" 2>/dev/null || true)

  # Extract authContextPatterns
  local_auth="$(sed -n '/scans:/,/^[^ ]/{ /idor:/,/^    [^ ]/{/authContextPatterns:/s/.*authContextPatterns:[[:space:]]*//p}}' "$PROJECT_CONFIG" 2>/dev/null || true)"
  [[ -n "$local_auth" ]] && IDOR_AUTH_PATTERNS="$local_auth"

  # Extract handlerFilePatterns
  local_handler="$(sed -n '/scans:/,/^[^ ]/{ /idor:/,/^    [^ ]/{/handlerFilePatterns:/s/.*handlerFilePatterns:[[:space:]]*//p}}' "$PROJECT_CONFIG" 2>/dev/null || true)"
  [[ -n "$local_handler" ]] && IDOR_HANDLER_FILTER="$local_handler"
fi

# Generic defaults — catch the most common IDOR anti-patterns across languages
if [[ ${#IDOR_BODY_PATTERNS[@]} -eq 0 ]]; then
  IDOR_BODY_PATTERNS=(
    # Generic: identity fields extracted from request body/payload/input structs
    'body\.\(user_id\|owner_id\|org_id\|tenant_id\|manager_id\|UserID\|OwnerID\|OrgID\|TenantID\|ManagerID\|userId\|ownerId\|orgId\|tenantId\)'
    'payload\.\(user_id\|owner_id\|org_id\|tenant_id\|UserID\|OwnerID\|OrgID\|TenantID\|userId\|ownerId\|orgId\|tenantId\)'
    'input\.\(user_id\|owner_id\|org_id\|tenant_id\|UserID\|OwnerID\|OrgID\|TenantID\|userId\|ownerId\|orgId\|tenantId\)'
    'req\.body\.\(userId\|ownerId\|orgId\|tenantId\|user_id\|owner_id\)'
    'request\.body\.\(userId\|ownerId\|orgId\|tenantId\|user_id\|owner_id\)'
    'request\.json\.\(user_id\|owner_id\|org_id\|tenant_id\)'
    'data\[.user_id.\]\|data\[.owner_id.\]\|data\[.org_id.\]\|data\[.tenant_id.\]'
  )
fi

if [[ -z "$IDOR_AUTH_PATTERNS" ]]; then
  IDOR_AUTH_PATTERNS='auth_user\|authenticated_user\|claims\.\|token\.\|session\.\|ctx\.user\|middleware\.\|get_user_id_from_token\|extract_user_id\|CurrentUser\|AuthUser\|get_authenticated\|FromRequest\|from_request_parts'
fi

if [[ -z "$IDOR_HANDLER_FILTER" ]]; then
  IDOR_HANDLER_FILTER='handler|controller|route|endpoint|api|grpc|server'
fi

for impl_file in "${impl_files[@]}"; do
  # Skip test files
  if echo "$impl_file" | grep -qE '(test|spec|_test\.|\.test\.|\.spec\.|__tests__|__mocks__)'; then
    continue
  fi

  # Only scan handler/route/controller-like files
  if ! echo "$impl_file" | grep -qiE "$IDOR_HANDLER_FILTER"; then
    continue
  fi

  for pattern in "${IDOR_BODY_PATTERNS[@]}"; do
    while IFS=: read -r line_num matched_line; do
      # Skip comment lines (multi-language)
      if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*|{/\*)'; then continue; fi
      # Check if auth context is also used in this file
      auth_usage="$(grep -cE "$IDOR_AUTH_PATTERNS" "$impl_file" 2>/dev/null || true)"
      if [[ "$auth_usage" -eq 0 ]]; then
        violation "$impl_file" "$line_num" "IDOR_BODY_IDENTITY" "User/org identity extracted from request body instead of auth context: $matched_line"
      else
        vwarn "IDOR risk in $impl_file:$line_num — body identity field present alongside auth context. Manual review: ensure body ID is NOT used for authorization decisions."
      fi
    done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
  done
done
echo ""

# =============================================================================
# SCAN 8: Silent Decode Failure Detection (Gate G048)
# =============================================================================
# Detects code that silently discards deserialization/decode errors instead
# of logging or propagating them. This allows data corruption to go
# undetected — corrupted database rows are silently dropped from results.
#
# Patterns are loaded from .github/bubbles-project.yaml if available,
# otherwise generic defaults are used. Projects can override:
#   scans.silentDecode.patterns       — regex patterns for silent decode detection
#   scans.silentDecode.errorHandling  — regex patterns for acceptable error handling
# =============================================================================
echo "--- Scan 8: Silent Decode Failure Detection (Gate G048) ---"

# Load project-specific patterns or use generic defaults
SILENT_DECODE_PATTERNS=()
DECODE_ERROR_HANDLING_PATTERNS=""

if [[ -f "$PROJECT_CONFIG" ]]; then
  while IFS= read -r pat; do
    pat="$(echo "$pat" | sed 's/^[[:space:]]*-[[:space:]]*//' | sed 's/^"//' | sed 's/"$//' | sed "s/^'//" | sed "s/'$//")"
    [[ -n "$pat" ]] && SILENT_DECODE_PATTERNS+=("$pat")
  done < <(sed -n '/scans:/,/^[^ ]/{ /silentDecode:/,/^    [^ ]/{/patterns:/,/^      [^ ]/{/^        -/p}}}' "$PROJECT_CONFIG" 2>/dev/null || true)

  local_err="$(sed -n '/scans:/,/^[^ ]/{ /silentDecode:/,/^    [^ ]/{/errorHandling:/s/.*errorHandling:[[:space:]]*//p}}' "$PROJECT_CONFIG" 2>/dev/null || true)"
  [[ -n "$local_err" ]] && DECODE_ERROR_HANDLING_PATTERNS="$local_err"
fi

if [[ ${#SILENT_DECODE_PATTERNS[@]} -eq 0 ]]; then
  SILENT_DECODE_PATTERNS=(
    # Silent Ok extraction on decode/deserialize operations
    'if let Ok.*decode\|if let Ok.*deserialize\|if let Ok.*from_bytes\|if let Ok.*parse_from'
    'if let Ok.*prost::Message\|if let Ok.*serde.*from\|if let Ok.*protobuf'
    # Dropping Err results via filter_map/flat_map
    'filter_map.*\.ok()\|flat_map.*\.ok()'
    # unwrap_or_default on decode results
    'decode.*unwrap_or_default\|deserialize.*unwrap_or_default\|from_bytes.*unwrap_or_default'
    'parse_from.*unwrap_or_default\|from_slice.*unwrap_or_default'
    # Go: ignoring decode error
    'proto\.Unmarshal.*_\b\|json\.Unmarshal.*_\b'
    # TS/JS: swallowing parse errors
    'JSON\.parse.*catch\|protobuf.*catch\|decode.*catch'
    # Python: swallowing decode errors
    'except.*pass\s*$\|except.*continue\s*$'
  )
fi

if [[ -z "$DECODE_ERROR_HANDLING_PATTERNS" ]]; then
  DECODE_ERROR_HANDLING_PATTERNS='log::error\|tracing::error\|error!\(|warn!\(|eprintln!\(|return Err\(|\.map_err\(|log\.Error\|log\.Warn\|logger\.error\|logger\.warn\|console\.error\|logging\.error'
fi

for impl_file in "${impl_files[@]}"; do
  # Skip test files — test code may legitimately test error handling
  if echo "$impl_file" | grep -qE '(test|spec|_test\.|\.test\.|\.spec\.|__tests__|__mocks__)'; then
    continue
  fi

  # Find #[cfg(test)] boundary for Rust files
  file_ext="${impl_file##*.}"
  cfg_test_line=999999
  if [[ "$file_ext" == "rs" ]]; then
    local_cfg="$(grep -n '#\[cfg(test)\]' "$impl_file" 2>/dev/null | head -1 | cut -d: -f1 || true)"
    if [[ -n "$local_cfg" ]]; then cfg_test_line=$local_cfg; fi
  fi

  for pattern in "${SILENT_DECODE_PATTERNS[@]}"; do
    while IFS=: read -r line_num matched_line; do
      [[ -z "$line_num" ]] && continue
      # Skip lines in #[cfg(test)] module
      if [[ "$line_num" -ge "$cfg_test_line" ]]; then continue; fi
      # Skip comment lines
      if echo "$matched_line" | grep -qE '^\s*(//|#|/\*|\*|{/\*)'; then continue; fi
      # Check if there's error handling within ±5 lines
      file_len=$(wc -l < "$impl_file")
      check_start=$((line_num - 2))
      if [[ $check_start -lt 1 ]]; then check_start=1; fi
      check_end=$((line_num + 5))
      if [[ $check_end -gt $file_len ]]; then check_end=$file_len; fi
      has_error_handling="$(sed -n "${check_start},${check_end}p" "$impl_file" | grep -cE "$DECODE_ERROR_HANDLING_PATTERNS" || true)"
      if [[ "$has_error_handling" -eq 0 ]]; then
        violation "$impl_file" "$line_num" "SILENT_DECODE" "Decode/deserialize error silently discarded — corrupted data will be invisible: $matched_line"
      fi
    done < <(grep -nE "$pattern" "$impl_file" 2>/dev/null || true)
  done
done
echo ""

# =============================================================================
# FINAL VERDICT
# =============================================================================
echo "============================================================"
echo "  IMPLEMENTATION REALITY SCAN RESULT"
echo "============================================================"
echo ""
echo "  Files scanned:  $scanned_files"
echo "  Violations:     $violations"
echo "  Warnings:       $warnings"
echo ""

if [[ "$violations" -gt 0 ]]; then
  echo "🔴 BLOCKED: $violations source code reality violation(s) found"
  fun_message scan_dirty
  echo ""
  echo "These violations indicate stub, fake, or hardcoded data patterns"
  echo "in implementation files. ALL violations must be resolved before"
  echo "the spec/scope can be marked 'done'."
  echo ""
  echo "Common fixes:"
  echo "  - Replace hardcoded Vec/array returns with real DB queries"
  echo "  - Replace status-only handlers with real service/domain/store delegation"
  echo "  - Replace getSimulationData() with real API fetch() calls"
  echo "  - Replace simulate_*() in handlers with real service calls"
  echo "  - Replace 501/not-implemented handlers with real delegated behavior before claiming delivery"
  echo "  - Replace random/no-op provider adapters with real upstream/API/SDK integration paths"
  echo "  - Add real fetch/axios/grpc calls to data hooks"
  echo "  - Remove auth/session/payment secrets from localStorage/sessionStorage/IndexedDB/AsyncStorage/SharedPreferences"
  echo "  - Replace unwrap_or()/unwrap_or_default() with ? and fail-fast"
  echo "  - Replace || 'default' / ?? 'fallback' with explicit missing-config errors"
  echo "  - Replace os.getenv('K', 'default') with fail-fast on missing env"
  echo "  - Reclassify intercepted tests out of live-stack categories and add real integration/E2E coverage"
  echo "  - Extract user identity from auth context (JWT/session/middleware), NOT from request body (IDOR fix)"
  echo "  - Replace 'if let Ok(x) = decode()' with match/? and log errors with row ID (silent decode fix)"
  echo "  - Replace filter_map(|r| r.ok()) on decode results with explicit error logging (silent decode fix)"
  exit 1
else
  if [[ "$warnings" -gt 0 ]]; then
    echo "🟡 PASSED with $warnings warning(s) — manual review advised"
  else
    echo "🟢 PASSED: No source code reality violations detected"
    fun_message scan_clean
  fi
  exit 0
fi
