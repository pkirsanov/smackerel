#!/usr/bin/env bash
# Spec 052 SCN-052-S02 / NFR Determinism — bundle self-hosted integration test.
#
# Asserts that two consecutive `smackerel config generate --env self-hosted
# --bundle` invocations with identical inputs produce byte-identical
# tar.gz bundles, AND that the bundle ships the new sibling
# secret-keys.yaml manifest enumerating the 4 managed secret keys.
#
# Sub-test A (determinism):
#   - Run loader twice into separate output dirs with same source-sha
#   - Compute sha256 of each tarball
#   - Assert hashes are byte-identical
#
# Sub-test B (sibling secret-keys.yaml):
#   - Extract bundle
#   - Assert tar contains secret-keys.yaml at top level
#   - Assert secret-keys.yaml lists exactly the 4 managed keys
#   - Assert app.env contains 4+ __SECRET_PLACEHOLDER__ markers
#   - Assert app.env contains ZERO ^POSTGRES_PASSWORD=smackerel$ lines
#   - Assert bundle-manifest.yaml lists secret-keys.yaml in files
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

SOURCE_SHA="0000000000000000000000000000000000000000"
BUNDLE_NAME="config-bundle-self-hosted-${SOURCE_SHA}.tar.gz"

# -----------------------------------------------------------------------------
# Sub-test A: two consecutive invocations produce byte-identical bundles.
# -----------------------------------------------------------------------------
echo "--- Sub-test A: bundle determinism (two invocations → identical sha256) ---"

cd "$REPO_ROOT"

OUT_A="$SCOPE_TMP/run-a"
OUT_B="$SCOPE_TMP/run-b"
mkdir -p "$OUT_A" "$OUT_B"

run_a_output="$(bash "$SMACKEREL_SH" config generate \
  --env self-hosted \
  --bundle \
  --output-dir "$OUT_A" \
  --source-sha "$SOURCE_SHA" 2>&1)"
run_a_exit=$?

if [[ "$run_a_exit" -ne 0 ]]; then
  echo "FAIL: run A returned exit $run_a_exit"
  echo "----- captured output -----"
  echo "$run_a_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: run A exited 0"
fi

# Sleep 1 second to ensure that any timestamp leakage would produce
# a different hash. Determinism MUST be timestamp-independent.
sleep 1

run_b_output="$(bash "$SMACKEREL_SH" config generate \
  --env self-hosted \
  --bundle \
  --output-dir "$OUT_B" \
  --source-sha "$SOURCE_SHA" 2>&1)"
run_b_exit=$?

if [[ "$run_b_exit" -ne 0 ]]; then
  echo "FAIL: run B returned exit $run_b_exit"
  echo "----- captured output -----"
  echo "$run_b_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: run B exited 0"
fi

TARBALL_A="$OUT_A/$BUNDLE_NAME"
TARBALL_B="$OUT_B/$BUNDLE_NAME"

if [[ ! -f "$TARBALL_A" ]] || [[ ! -f "$TARBALL_B" ]]; then
  echo "FAIL: tarball missing — A=$([[ -f "$TARBALL_A" ]] && echo present || echo MISSING) B=$([[ -f "$TARBALL_B" ]] && echo present || echo MISSING)"
  failures=$((failures + 1))
else
  HASH_A="$(sha256sum "$TARBALL_A" | awk '{print $1}')"
  HASH_B="$(sha256sum "$TARBALL_B" | awk '{print $1}')"
  echo "Run A sha256: $HASH_A"
  echo "Run B sha256: $HASH_B"

  if [[ "$HASH_A" == "$HASH_B" ]]; then
    echo "PASS: bundle sha256 hashes are byte-identical (NFR Determinism satisfied)"
  else
    echo "FAIL: bundle sha256 hashes DIFFER (NFR Determinism violated)"
    failures=$((failures + 1))
    echo "----- diff between extracted bundles: -----"
    EXTRACT_A="$SCOPE_TMP/extract-a"
    EXTRACT_B="$SCOPE_TMP/extract-b"
    mkdir -p "$EXTRACT_A" "$EXTRACT_B"
    tar xzf "$TARBALL_A" -C "$EXTRACT_A"
    tar xzf "$TARBALL_B" -C "$EXTRACT_B"
    diff -r "$EXTRACT_A" "$EXTRACT_B" || true
    echo "----- end diff -----"
  fi
fi

# -----------------------------------------------------------------------------
# Sub-test B: bundle contains secret-keys.yaml and shields literals.
# -----------------------------------------------------------------------------
echo ""
echo "--- Sub-test B: bundle ships sibling secret-keys.yaml + shields literals ---"

if [[ ! -f "$TARBALL_A" ]]; then
  echo "SKIP: tarball A not present, cannot run sibling-yaml assertions"
else
  EXTRACT="$SCOPE_TMP/extract"
  mkdir -p "$EXTRACT"
  tar xzf "$TARBALL_A" -C "$EXTRACT"

  if tar tzf "$TARBALL_A" | grep -q '^secret-keys\.yaml$'; then
    echo "PASS: tarball contains secret-keys.yaml at top level"
  else
    echo "FAIL: tarball does NOT contain secret-keys.yaml at top level"
    echo "----- tar tzf output: -----"
    tar tzf "$TARBALL_A"
    echo "----- end -----"
    failures=$((failures + 1))
  fi

  SECRET_KEYS_FILE="$EXTRACT/secret-keys.yaml"
  if [[ ! -f "$SECRET_KEYS_FILE" ]]; then
    echo "FAIL: extracted bundle missing secret-keys.yaml"
    failures=$((failures + 1))
  else
    echo "----- secret-keys.yaml contents: -----"
    cat "$SECRET_KEYS_FILE"
    echo "----- end -----"

    for key in "${MANAGED_KEYS[@]}"; do
      if grep -qE "^[[:space:]]*-[[:space:]]+${key}$" "$SECRET_KEYS_FILE"; then
        echo "PASS: secret-keys.yaml lists $key"
      else
        echo "FAIL: secret-keys.yaml does NOT list $key"
        failures=$((failures + 1))
      fi
    done

    listed_count="$(grep -cE '^[[:space:]]*-[[:space:]]+[A-Z_]+$' "$SECRET_KEYS_FILE" || true)"
    if [[ "$listed_count" -eq 4 ]]; then
      echo "PASS: secret-keys.yaml lists exactly 4 keys"
    else
      echo "FAIL: secret-keys.yaml lists $listed_count keys (expected 4)"
      failures=$((failures + 1))
    fi
  fi

  APPENV="$EXTRACT/app.env"
  if [[ ! -f "$APPENV" ]]; then
    echo "FAIL: extracted bundle missing app.env"
    failures=$((failures + 1))
  else
    placeholder_count="$(grep -c '__SECRET_PLACEHOLDER__' "$APPENV" || true)"
    if [[ "$placeholder_count" -ge 4 ]]; then
      echo "PASS: app.env contains $placeholder_count __SECRET_PLACEHOLDER__ markers (>= 4)"
    else
      echo "FAIL: app.env contains only $placeholder_count __SECRET_PLACEHOLDER__ markers (expected >= 4)"
      failures=$((failures + 1))
    fi

    literal_count="$(grep -cE '^POSTGRES_PASSWORD=smackerel$' "$APPENV" || true)"
    if [[ "$literal_count" -eq 0 ]]; then
      echo "PASS: app.env contains ZERO ^POSTGRES_PASSWORD=smackerel$ lines (placeholder shields literal)"
    else
      echo "FAIL: app.env contains $literal_count ^POSTGRES_PASSWORD=smackerel$ lines (placeholder failed to shield)"
      failures=$((failures + 1))
    fi
  fi

  MANIFEST="$EXTRACT/bundle-manifest.yaml"
  if [[ ! -f "$MANIFEST" ]]; then
    echo "FAIL: extracted bundle missing bundle-manifest.yaml"
    failures=$((failures + 1))
  else
    if grep -qE '^[[:space:]]*-[[:space:]]+secret-keys\.yaml$' "$MANIFEST"; then
      echo "PASS: bundle-manifest.yaml lists secret-keys.yaml in files"
    else
      echo "FAIL: bundle-manifest.yaml does NOT list secret-keys.yaml in files"
      echo "----- bundle-manifest.yaml contents: -----"
      cat "$MANIFEST"
      echo "----- end -----"
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
