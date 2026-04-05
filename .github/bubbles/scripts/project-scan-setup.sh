#!/usr/bin/env bash
# =============================================================================
# project-scan-setup.sh
# =============================================================================
# Analyzes the project codebase and generates/updates the project-owned
# sections in .github/bubbles-project.yaml with project-appropriate patterns.
#
# This script is PROJECT-AGNOSTIC — it auto-detects languages, frameworks,
# auth patterns, serialization formats, and test env dependencies.
#
# Usage:
#   bash bubbles/scripts/project-scan-setup.sh [--dry-run] [--force] [--quiet]
#
# Flags:
#   --dry-run   Show what would be generated without writing
#   --force     Regenerate scans section even if one already exists
#   --quiet     Minimal output (for automated invocation from other scripts)
#
# Behavior:
#   - If .github/bubbles-project.yaml does not exist: creates it
#   - If it exists but has no managed Bubbles config sections: appends them
#   - If it exists with scans section: skip (unless --force)
#   - --dry-run: shows what would be generated without writing
#
# Exit codes:
#   0 = Success (file created/updated or already up-to-date)
#   1 = Error
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/fun-mode.sh" 2>/dev/null || true

DRY_RUN="false"
FORCE="false"
QUIET="false"
for arg in "$@"; do
  [[ "$arg" == "--dry-run" ]] && DRY_RUN="true"
  [[ "$arg" == "--force" ]] && FORCE="true"
  [[ "$arg" == "--quiet" ]] && QUIET="true"
done

PROJECT_YAML=".github/bubbles-project.yaml"

log() { [[ "$QUIET" == "false" ]] && echo "$@" || true; }

# =============================================================================
# Phase 1: Detect project tech stack
# =============================================================================
log "🔍 Analyzing project codebase..."
log ""

# Detect languages
HAS_RUST="false"
HAS_GO="false"
HAS_TS="false"
HAS_PYTHON="false"
HAS_JAVA="false"
HAS_SCALA="false"
HAS_BRIGHTSCRIPT="false"

[[ -f "Cargo.toml" ]] || find . -maxdepth 4 -name 'Cargo.toml' -not -path '*/target/*' 2>/dev/null | grep -q . && HAS_RUST="true"
[[ -f "go.mod" ]] || find . -maxdepth 4 -name 'go.mod' 2>/dev/null | grep -q . && HAS_GO="true"
find . -maxdepth 4 -name 'tsconfig.json' -not -path '*/node_modules/*' 2>/dev/null | grep -q . && HAS_TS="true"
find . -maxdepth 4 -name 'requirements*.txt' -o -name 'pyproject.toml' -o -name 'setup.py' 2>/dev/null | grep -q . && HAS_PYTHON="true"
find . -maxdepth 4 -name 'pom.xml' -o -name 'build.gradle' 2>/dev/null | grep -q . && HAS_JAVA="true"
find . -maxdepth 4 -name 'build.sbt' -not -path '*/target/*' 2>/dev/null | grep -q . && HAS_SCALA="true"
find . -maxdepth 4 -name '*.brs' -not -path '*/node_modules/*' 2>/dev/null | grep -q . && HAS_BRIGHTSCRIPT="true"

log "  Languages detected:"
[[ "$HAS_RUST" == "true" ]] && log "    ✓ Rust"
[[ "$HAS_GO" == "true" ]] && log "    ✓ Go"
[[ "$HAS_TS" == "true" ]] && log "    ✓ TypeScript"
[[ "$HAS_PYTHON" == "true" ]] && log "    ✓ Python"
[[ "$HAS_JAVA" == "true" ]] && log "    ✓ Java"
[[ "$HAS_SCALA" == "true" ]] && log "    ✓ Scala"
[[ "$HAS_BRIGHTSCRIPT" == "true" ]] && log "    ✓ BrightScript (Roku)"
log ""

# Detect serialization
HAS_PROTOBUF="false"
HAS_JSON="false"

find . -maxdepth 5 -name '*.proto' -not -path '*/target/*' -not -path '*/node_modules/*' 2>/dev/null | grep -q . && HAS_PROTOBUF="true"
# JSON is assumed if not protobuf-only
if [[ "$HAS_PROTOBUF" == "true" ]]; then
  log "  Serialization: Protobuf detected"
else
  HAS_JSON="true"
  log "  Serialization: JSON (default)"
fi

# Detect handler file locations
log ""
log "  Handler file patterns detected:"
HANDLER_DIRS=()
for candidate in \
  "services/gateway/src/handlers" \
  "backend/internal/api/handlers" \
  "backend/rust-services/*/src" \
  "src/handlers" \
  "src/controllers" \
  "src/api" \
  "app/controllers" \
  "app/handlers" \
  "internal/api/handlers" \
  "cmd/*/handlers" \
  "roku-app/source" \
  "roku-app/components"; do
  found_dirs="$(find . -maxdepth 5 -path "./$candidate" -type d 2>/dev/null || true)"
  if [[ -n "$found_dirs" ]]; then
    while IFS= read -r d; do
      HANDLER_DIRS+=("$d")
      log "    ✓ $d"
    done <<< "$found_dirs"
  fi
done
log ""

# =============================================================================
# Phase 2: Detect auth patterns
# =============================================================================
log "  Auth patterns detected:"

AUTH_PATTERNS=()

# Rust auth patterns
if [[ "$HAS_RUST" == "true" ]]; then
  grep -rl 'JwtClaims\|jwt_claims\|auth_middleware\|security_middleware' --include='*.rs' . 2>/dev/null | grep -v target | head -3 | while read -r f; do
    log "    ✓ Rust JWT/auth middleware ($f)"
  done
  # Check for header-based identity
  if grep -rl 'x-user-id\|x-organization-id' --include='*.rs' . 2>/dev/null | grep -v target | head -1 | grep -q .; then
    AUTH_PATTERNS+=('x-user-id\|x-organization-id\|x-api-key')
    log "    ✓ Header-based identity propagation (x-user-id)"
  fi
  # Check for claims-based
  if grep -rl 'claims\.\|Claims\b' --include='*.rs' . 2>/dev/null | grep -v target | head -1 | grep -q .; then
    AUTH_PATTERNS+=('claims\.\|JwtClaims\|token_data')
  fi
  AUTH_PATTERNS+=('auth_middleware\|security_middleware\|enhanced_auth\|api_key_middleware')
fi

# Go auth patterns
if [[ "$HAS_GO" == "true" ]]; then
  if grep -rl 'GetUserID\|GetTenantID\|GetClaims' --include='*.go' . 2>/dev/null | head -1 | grep -q .; then
    AUTH_PATTERNS+=('GetUserID\|GetTenantID\|GetClaims\|GetManagerID')
    log "    ✓ Go context-based auth (GetUserID/GetTenantID)"
  fi
  if grep -rl 'ctx\.Value.*user_id\|ctx\.Value.*claims' --include='*.go' . 2>/dev/null | head -1 | grep -q .; then
    AUTH_PATTERNS+=('ctx\.Value\|UserIDKey\|TenantIDKey\|ClaimsKey')
  fi
  if grep -rl 'RequireRole\|PropertyAccess' --include='*.go' . 2>/dev/null | head -1 | grep -q .; then
    AUTH_PATTERNS+=('RequireRole\|PropertyAccessMiddleware')
    log "    ✓ Go role-based access control"
  fi
fi

# Generic auth patterns
AUTH_PATTERNS+=('auth_user\|authenticated_user\|CurrentUser\|AuthUser\|FromRequest\|from_request_parts\|Authorization\|Bearer')

log ""

# =============================================================================
# Phase 3: Detect test env dependencies
# =============================================================================
log "  Test environment dependencies detected:"

ENV_DEPS=()

# Search for env vars with .expect() or panic-on-missing in Rust
if [[ "$HAS_RUST" == "true" ]]; then
  while IFS= read -r match; do
    var_name="$(echo "$match" | grep -oE '"[A-Z_]+"' | head -1 | sed 's/"//g')"
    if [[ -n "$var_name" ]] && [[ "$var_name" != "PATH" ]] && [[ "$var_name" != "HOME" ]]; then
      ENV_DEPS+=("$var_name")
      log "    ✓ $var_name (Rust .expect() on env var)"
    fi
  done < <(grep -rn 'env::var.*\.expect\|env::var.*\.unwrap()' --include='*.rs' . 2>/dev/null | grep -v target | grep -v '#\[cfg(test)\]' | head -10)
fi

# Search for Go os.Getenv without fallback in test-adjacent code
if [[ "$HAS_GO" == "true" ]]; then
  while IFS= read -r match; do
    var_name="$(echo "$match" | grep -oE '"[A-Z_]+"' | head -1 | sed 's/"//g')"
    if [[ -n "$var_name" ]] && [[ ${#var_name} -gt 3 ]]; then
      ENV_DEPS+=("$var_name")
      log "    ✓ $var_name (Go os.Getenv in test setup)"
    fi
  done < <(grep -rn 'os\.Getenv' --include='*_test.go' --include='*test*.go' . 2>/dev/null | head -10)
fi

log ""

# =============================================================================
# Phase 4: Build IDOR body identity patterns
# =============================================================================
IDOR_BODY=()

if [[ "$HAS_RUST" == "true" ]]; then
  IDOR_BODY+=("body\.user_id\|payload\.user_id\|input\.user_id\|request\.user_id")
  IDOR_BODY+=("body\.owner_id\|body\.org_id\|body\.tenant_id\|body\.manager_id")
fi

if [[ "$HAS_GO" == "true" ]]; then
  IDOR_BODY+=("body\.UserID\|body\.OwnerID\|body\.TenantID\|body\.ManagerID\|body\.PropertyID")
  IDOR_BODY+=("req\.UserID\|req\.OwnerID\|req\.TenantID\|input\.UserID")
fi

if [[ "$HAS_TS" == "true" ]]; then
  IDOR_BODY+=("req\.body\.userId\|req\.body\.ownerId\|req\.body\.orgId\|req\.body\.tenantId")
fi

if [[ "$HAS_PYTHON" == "true" ]]; then
  IDOR_BODY+=("data\[.user_id.\]\|data\[.owner_id.\]\|request\.json\[.user_id.\]")
fi

if [[ "$HAS_SCALA" == "true" ]] || [[ "$HAS_JAVA" == "true" ]]; then
  IDOR_BODY+=("body\.userId\|body\.ownerId\|body\.orgId\|body\.tenantId")
  IDOR_BODY+=("request\.userId\|request\.ownerId\|payload\.userId\|payload\.tenantId")
fi

# =============================================================================
# Phase 5: Build silent decode patterns
# =============================================================================
DECODE_PATTERNS=()
DECODE_ERROR_HANDLING=()

if [[ "$HAS_RUST" == "true" ]]; then
  DECODE_PATTERNS+=("if let Ok.*decode\|if let Ok.*deserialize\|if let Ok.*from_bytes\|if let Ok.*parse_from")
  if [[ "$HAS_PROTOBUF" == "true" ]]; then
    DECODE_PATTERNS+=("if let Ok.*prost::Message\|if let Ok.*protobuf")
  fi
  DECODE_PATTERNS+=("filter_map.*\.ok()\|flat_map.*\.ok()")
  DECODE_PATTERNS+=("decode.*unwrap_or_default\|deserialize.*unwrap_or_default\|from_bytes.*unwrap_or_default")
  DECODE_ERROR_HANDLING+=("log::error\|tracing::error\|error!\(|warn!\(|return Err\(|\.map_err\(")
fi

if [[ "$HAS_GO" == "true" ]]; then
  DECODE_PATTERNS+=("proto\.Unmarshal.*_\b\|json\.Unmarshal.*_\b")
  DECODE_PATTERNS+=("Decode.*_\s*:=\|Deserialize.*_\s*:=")
  DECODE_ERROR_HANDLING+=("log\.Error\|log\.Warn\|logger\.Error\|return.*err\|respondError")
fi

if [[ "$HAS_TS" == "true" ]]; then
  DECODE_PATTERNS+=("JSON\.parse.*catch\|protobuf.*catch\|decode.*catch")
  DECODE_ERROR_HANDLING+=("console\.error\|logger\.error")
fi

if [[ "$HAS_PYTHON" == "true" ]]; then
  DECODE_PATTERNS+=("except.*pass\s*$\|except.*continue\s*$")
  DECODE_ERROR_HANDLING+=("logging\.error\|logger\.error")
fi

if [[ "$HAS_SCALA" == "true" ]] || [[ "$HAS_JAVA" == "true" ]]; then
  DECODE_PATTERNS+=("case Success\|Try\(.*\.get\b\|catch.*case.*NonFatal")
  DECODE_PATTERNS+=("\.recover\s*{\|getOrElse\|orElse")
  DECODE_ERROR_HANDLING+=("logger\.error\|log\.error\|Logger\.error")
fi

# =============================================================================
# Phase 6: Generate YAML
# =============================================================================

# Detect project name
PROJECT_NAME="$(basename "$(pwd)")"
PROJECT_NAME="$(echo "$PROJECT_NAME" | sed 's/[_-]/ /g' | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2)} 1' | sed 's/ //g')"

HANDLER_FILTER='handler|controller|route|endpoint|api'
if [[ "${#HANDLER_DIRS[@]}" -gt 0 ]]; then
  # Extract last path components as filter hints
  for d in "${HANDLER_DIRS[@]}"; do
    base="$(basename "$d")"
    if ! echo "$HANDLER_FILTER" | grep -qi "$base"; then
      HANDLER_FILTER="${HANDLER_FILTER}|${base}"
    fi
  done
fi

# Combine auth patterns
AUTH_COMBINED=""
for p in "${AUTH_PATTERNS[@]}"; do
  if [[ -z "$AUTH_COMBINED" ]]; then
    AUTH_COMBINED="$p"
  else
    AUTH_COMBINED="${AUTH_COMBINED}\|${p}"
  fi
done

# Combine env deps
ENV_COMBINED=""
for e in "${ENV_DEPS[@]}"; do
  if [[ -z "$ENV_COMBINED" ]]; then
    ENV_COMBINED="$e"
  else
    ENV_COMBINED="${ENV_COMBINED}\|${e}"
  fi
done

# Combine decode error handling
DECODE_ERR_COMBINED=""
for p in "${DECODE_ERROR_HANDLING[@]}"; do
  if [[ -z "$DECODE_ERR_COMBINED" ]]; then
    DECODE_ERR_COMBINED="$p"
  else
    DECODE_ERR_COMBINED="${DECODE_ERR_COMBINED}\|${p}"
  fi
done

# Detect managed-doc path overrides when the repo does not follow the
# framework default docs layout. These overrides remain project-owned.
log "  Managed docs path overrides detected:"
DOCS_OVERRIDE_LINES=""
DOCS_OVERRIDE_COUNT=0

detect_doc_path_override() {
  local key="$1"
  local default_path="$2"
  shift 2

  local candidate=""
  if [[ -f "$default_path" ]]; then
    return 0
  fi

  for candidate in "$@"; do
    if [[ -f "$candidate" ]]; then
      DOCS_OVERRIDE_LINES="${DOCS_OVERRIDE_LINES}
    ${key}:
      path: ${candidate}"
      DOCS_OVERRIDE_COUNT=$((DOCS_OVERRIDE_COUNT + 1))
      log "    ✓ ${key} -> ${candidate}"
      return 0
    fi
  done
}

detect_doc_path_override architecture docs/Architecture.md \
  docs/Platform_Architecture_Design.md \
  docs/ARCHITECTURE.md \
  README.md
detect_doc_path_override api docs/API.md \
  docs/api/Rhai_Script_Engine.md
detect_doc_path_override development docs/Development.md \
  README.md
detect_doc_path_override testing docs/Testing.md \
  docs/Test_Architecture.md \
  docs/Test_Storage_Isolation_Design.md
detect_doc_path_override deployment docs/Deployment.md \
  DOCKER.md
detect_doc_path_override operations docs/Operations.md \
  docs/Troubleshooting.md \
  docs/runbooks/integration-test-failures.md

if [[ "$DOCS_OVERRIDE_COUNT" -eq 0 ]]; then
  log "    (none)"
fi

# Build the YAML content
YAML_CONTENT="# =============================================================================
# bubbles-project.yaml — ${PROJECT_NAME}
# =============================================================================
# Project-specific Bubbles extensions.
# Auto-generated by: bubbles project setup
# This file is project-owned and never overwritten by Bubbles upgrades.
# Re-run 'bubbles project setup' to regenerate (only updates if no scans exist).
#
# See: agents/bubbles_shared/project-config-contract.md for full schema.
# =============================================================================

# Custom quality gates (G100+ range, auto-assigned IDs)
# gates:
#   example-gate:
#     script: scripts/my-check.sh
#     blocking: true
#     description: Example custom quality gate

# Scan pattern extensions for built-in gates
scans:
  # G047: IDOR / Auth Bypass Detection
  idor:
    handlerFilePatterns: '${HANDLER_FILTER}'
    bodyIdentityPatterns:"

for pat in "${IDOR_BODY[@]}"; do
  YAML_CONTENT="${YAML_CONTENT}
        - '${pat}'"
done

YAML_CONTENT="${YAML_CONTENT}
    authContextPatterns: '${AUTH_COMBINED}'

  # G048: Silent Decode Failure Detection
  silentDecode:
    patterns:"

for pat in "${DECODE_PATTERNS[@]}"; do
  YAML_CONTENT="${YAML_CONTENT}
        - '${pat}'"
done

YAML_CONTENT="${YAML_CONTENT}
    errorHandling: '${DECODE_ERR_COMBINED}'"

if [[ -n "$ENV_COMBINED" ]]; then
  YAML_CONTENT="${YAML_CONTENT}

  # G051: Test Environment Dependency Detection
  testEnvDependency:
    patterns: '${ENV_COMBINED}'"
fi

if [[ "$DOCS_OVERRIDE_COUNT" -gt 0 ]]; then
  YAML_CONTENT="${YAML_CONTENT}

# Managed-doc registry overrides
docsRegistryOverrides:
  managedDocs:${DOCS_OVERRIDE_LINES}

  # Add classification: or policy: overrides here only when your repo needs
  # a managed-doc shape that differs from the framework default registry."
fi

log ""
log "============================================================"
log "  PROJECT SCAN SETUP RESULT"
log "============================================================"
log ""

if [[ "$DRY_RUN" == "true" ]]; then
  echo "--- DRY RUN: Would generate .github/bubbles-project.yaml ---"
  echo ""
  echo "$YAML_CONTENT"
  echo ""
  echo "Run without --dry-run to write the file."
  exit 0
fi

# Check if file exists and already has scans section
if [[ -f "$PROJECT_YAML" ]]; then
  if grep -q '^scans:' "$PROJECT_YAML" 2>/dev/null; then
    if [[ "$FORCE" == "true" ]]; then
      # Remove existing scans section and regenerate
      # Preserve everything before 'scans:' (gates, comments, etc.)
      tmp_file="$(mktemp)"
      sed '/^scans:/,$d' "$PROJECT_YAML" > "$tmp_file"
      # Append new scans section
      echo "$YAML_CONTENT" | sed -n '/^# Scan pattern/,$p' >> "$tmp_file"
      mv "$tmp_file" "$PROJECT_YAML"
      log "✅ Regenerated scans section in .github/bubbles-project.yaml (--force)"
    elif [[ "$DOCS_OVERRIDE_COUNT" -gt 0 ]] && ! grep -q '^docsRegistryOverrides:' "$PROJECT_YAML" 2>/dev/null; then
      echo "" >> "$PROJECT_YAML"
      echo "$YAML_CONTENT" | sed -n '/^# Managed-doc registry overrides/,$p' >> "$PROJECT_YAML"
      log "✅ Appended docsRegistryOverrides section to existing .github/bubbles-project.yaml"
    else
      log "✅ .github/bubbles-project.yaml already configured (scans section present)"
      exit 0
    fi
  else
    # File exists but no scans section — append
    echo "" >> "$PROJECT_YAML"
    echo "$YAML_CONTENT" | sed -n '/^# Scan pattern/,$p' >> "$PROJECT_YAML"
    log "✅ Appended scans section to existing .github/bubbles-project.yaml"
  fi
else
  mkdir -p .github
  echo "$YAML_CONTENT" > "$PROJECT_YAML"
  log "✅ Created .github/bubbles-project.yaml with project-specific scan patterns"
fi

log ""
log "Languages: $(
  parts=""
  [[ "$HAS_RUST" == "true" ]] && parts="${parts}Rust, "
  [[ "$HAS_GO" == "true" ]] && parts="${parts}Go, "
  [[ "$HAS_TS" == "true" ]] && parts="${parts}TypeScript, "
  [[ "$HAS_PYTHON" == "true" ]] && parts="${parts}Python, "
  [[ "$HAS_JAVA" == "true" ]] && parts="${parts}Java, "
  [[ "$HAS_SCALA" == "true" ]] && parts="${parts}Scala, "
  [[ "$HAS_BRIGHTSCRIPT" == "true" ]] && parts="${parts}BrightScript, "
  echo "${parts%, }"
)"
log "Serialization: $([[ "$HAS_PROTOBUF" == "true" ]] && echo "Protobuf" || echo "JSON")"
log "Handler dirs: ${#HANDLER_DIRS[@]} found"
log "Auth patterns: ${#AUTH_PATTERNS[@]} detected"
log "Env deps: ${#ENV_DEPS[@]} found"
log ""
log "Customize: edit .github/bubbles-project.yaml to add project-specific patterns, managed-doc overrides, or custom gates."
