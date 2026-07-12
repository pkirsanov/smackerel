#!/usr/bin/env bash
# Spec 052 SCN-052-S01 — SST loader placeholder emission contract test.
#
# Asserts that the SST loader (scripts/commands/config.sh) correctly emits
# placeholder markers for managed secret keys when the target is a
# production-class target (currently: self-hosted) and inline literal yaml
# values when the target is dev/test (per FR-052-011).
#
# Sub-test A (self-hosted target — placeholder mode):
#   - Loader exits 0
#   - Resulting app.env (staged in bundle) contains exactly 4 lines matching
#     ^<KEY>=__SECRET_PLACEHOLDER__<KEY>__$ for the 4 managed keys
#     (POSTGRES_PASSWORD, AUTH_SIGNING_ACTIVE_PRIVATE_KEY,
#     AUTH_AT_REST_HASHING_KEY, AUTH_BOOTSTRAP_TOKEN)
#   - Resulting app.env does NOT contain the literal dev-default value
#     'smackerel' as the POSTGRES_PASSWORD value
#
# Sub-test B (dev target — inline mode preserved):
#   - Loader exits 0
#   - Resulting dev.env contains POSTGRES_PASSWORD=smackerel literal
#   - Resulting dev.env contains ZERO __SECRET_PLACEHOLDER__ markers
#     (FR-052-011: dev/test never use placeholder mode)
#
# Exits 0 on full pass, 1 on any failure.

set -uo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
SMACKEREL_SH="$REPO_ROOT/smackerel.sh"

if [[ ! -f "$SMACKEREL_SH" ]]; then
  echo "FATAL: cannot locate smackerel.sh under $REPO_ROOT" >&2
  exit 1
fi

failures=0
SCOPE_TMP="$(mktemp -d)"
trap 'rm -rf "$SCOPE_TMP"' EXIT

MANAGED_KEYS=(
  POSTGRES_PASSWORD
  AUTH_SIGNING_ACTIVE_PRIVATE_KEY
  AUTH_AT_REST_HASHING_KEY
  AUTH_BOOTSTRAP_TOKEN
)

# -----------------------------------------------------------------------------
# Sub-test A: self-hosted target → placeholder mode.
# -----------------------------------------------------------------------------
echo "--- Sub-test A: self-hosted target emits placeholders for 4 managed keys ---"
SELF_HOSTED_BUNDLE_DIR="$SCOPE_TMP/self-hosted-bundle"
mkdir -p "$SELF_HOSTED_BUNDLE_DIR"

cd "$REPO_ROOT"
self_hosted_output="$(bash "$SMACKEREL_SH" config generate \
  --env self-hosted \
  --bundle \
  --output-dir "$SELF_HOSTED_BUNDLE_DIR" \
  --source-sha 0000000000000000000000000000000000000000 2>&1)"
self_hosted_exit=$?

if [[ "$self_hosted_exit" -ne 0 ]]; then
  echo "FAIL: smackerel config generate --env self-hosted --bundle returned exit $self_hosted_exit"
  echo "----- captured output -----"
  echo "$self_hosted_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: smackerel config generate --env self-hosted --bundle exited 0"
fi

SELF_HOSTED_TARBALL="$SELF_HOSTED_BUNDLE_DIR/config-bundle-self-hosted-0000000000000000000000000000000000000000.tar.gz"
if [[ ! -f "$SELF_HOSTED_TARBALL" ]]; then
  echo "FAIL: bundle tarball not produced at $SELF_HOSTED_TARBALL"
  failures=$((failures + 1))
else
  echo "PASS: bundle tarball produced at $SELF_HOSTED_TARBALL"
  SELF_HOSTED_EXTRACT="$SCOPE_TMP/self-hosted-extract"
  mkdir -p "$SELF_HOSTED_EXTRACT"
  tar xzf "$SELF_HOSTED_TARBALL" -C "$SELF_HOSTED_EXTRACT"

  SELF_HOSTED_APPENV="$SELF_HOSTED_EXTRACT/app.env"
  if [[ ! -f "$SELF_HOSTED_APPENV" ]]; then
    echo "FAIL: extracted bundle does not contain app.env at $SELF_HOSTED_APPENV"
    failures=$((failures + 1))
  else
    placeholder_hits=0
    for key in "${MANAGED_KEYS[@]}"; do
      if grep -qE "^${key}=__SECRET_PLACEHOLDER__${key}__$" "$SELF_HOSTED_APPENV"; then
        echo "PASS: $key emitted as placeholder marker"
        placeholder_hits=$((placeholder_hits + 1))
      else
        echo "FAIL: $key NOT emitted as placeholder marker (expected ^${key}=__SECRET_PLACEHOLDER__${key}__$)"
        echo "----- matching lines in app.env: -----"
        grep "^${key}=" "$SELF_HOSTED_APPENV" || echo "(no matching key)"
        echo "----- end -----"
        failures=$((failures + 1))
      fi
    done

    if [[ "$placeholder_hits" -ne 4 ]]; then
      echo "FAIL: expected exactly 4 placeholder hits, got $placeholder_hits"
      failures=$((failures + 1))
    else
      echo "PASS: exactly 4 placeholder markers emitted (one per managed key)"
    fi

    if grep -qE '^POSTGRES_PASSWORD=smackerel$' "$SELF_HOSTED_APPENV"; then
      echo "FAIL: self-hosted app.env contains literal POSTGRES_PASSWORD=smackerel (placeholder mode failed to shield)"
      failures=$((failures + 1))
    else
      echo "PASS: self-hosted app.env does NOT contain literal POSTGRES_PASSWORD=smackerel"
    fi
  fi
fi

# -----------------------------------------------------------------------------
# Sub-test B: dev target → inline mode preserved (FR-052-011).
# -----------------------------------------------------------------------------
echo ""
echo "--- Sub-test B: dev target preserves inline yaml values ---"
DEV_BUNDLE_DIR="$SCOPE_TMP/dev-bundle"
mkdir -p "$DEV_BUNDLE_DIR"

dev_output="$(bash "$SMACKEREL_SH" config generate \
  --env dev \
  --bundle \
  --output-dir "$DEV_BUNDLE_DIR" \
  --source-sha 0000000000000000000000000000000000000000 2>&1)"
dev_exit=$?

if [[ "$dev_exit" -ne 0 ]]; then
  echo "FAIL: smackerel config generate --env dev --bundle returned exit $dev_exit"
  echo "----- captured output -----"
  echo "$dev_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: smackerel config generate --env dev --bundle exited 0"
fi

DEV_TARBALL="$DEV_BUNDLE_DIR/config-bundle-dev-0000000000000000000000000000000000000000.tar.gz"
if [[ ! -f "$DEV_TARBALL" ]]; then
  echo "FAIL: dev bundle tarball not produced at $DEV_TARBALL"
  failures=$((failures + 1))
else
  echo "PASS: dev bundle tarball produced at $DEV_TARBALL"
  DEV_EXTRACT="$SCOPE_TMP/dev-extract"
  mkdir -p "$DEV_EXTRACT"
  tar xzf "$DEV_TARBALL" -C "$DEV_EXTRACT"

  DEV_APPENV="$DEV_EXTRACT/app.env"
  if [[ ! -f "$DEV_APPENV" ]]; then
    echo "FAIL: extracted dev bundle does not contain app.env at $DEV_APPENV"
    failures=$((failures + 1))
  else
    if grep -qE '^POSTGRES_PASSWORD=smackerel$' "$DEV_APPENV"; then
      echo "PASS: dev app.env preserves literal POSTGRES_PASSWORD=smackerel (FR-052-011)"
    else
      echo "FAIL: dev app.env does NOT contain literal POSTGRES_PASSWORD=smackerel (FR-052-011 broken)"
      echo "----- POSTGRES_PASSWORD lines in dev app.env: -----"
      grep '^POSTGRES_PASSWORD=' "$DEV_APPENV" || echo "(no matching key)"
      echo "----- end -----"
      failures=$((failures + 1))
    fi

    placeholder_count="$(grep -c '__SECRET_PLACEHOLDER__' "$DEV_APPENV" || true)"
    if [[ "$placeholder_count" -eq 0 ]]; then
      echo "PASS: dev app.env contains ZERO __SECRET_PLACEHOLDER__ markers (FR-052-011)"
    else
      echo "FAIL: dev app.env contains $placeholder_count __SECRET_PLACEHOLDER__ markers (expected 0 — dev should be inline)"
      failures=$((failures + 1))
    fi
  fi
fi

# -----------------------------------------------------------------------------
echo ""
if [[ "$failures" -gt 0 ]]; then
  echo "FAILURES: $failures sub-test(s) failed"
  exit 1
fi
echo "All sub-tests passed"
exit 0
