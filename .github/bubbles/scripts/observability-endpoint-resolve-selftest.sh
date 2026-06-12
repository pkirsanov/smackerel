#!/usr/bin/env bash
set -uo pipefail

# observability-endpoint-resolve-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/observability-endpoint-resolve.sh`
# (IMP-001 SCOPE-3 — T3.3 resolution, T3.7 prod-block, T3.8 profile env binding).
#
# Like the posture-guard selftest, the hermetic workspace is staged UNDER $HOME
# so a snap-confined `yq` can read the fixture config (strict confinement cannot
# read /tmp). Staging files inside a throwaway $HOME workspace never becomes part
# of the working tree.
#
# Cases:
#   T3.3  validate/sloBurn → adapter=prometheus profile=test, validate env
#         materialized to PROMETHEUS_BASE_URL
#   T3.3  operate/alerts   → adapter=prometheus profile=prod
#   T3.3  validate/alerts (configured none) → adapter=none, exit 0
#   T3.7  PROD-BLOCK: validate/sloBurn with ONLY operate env set → exit 1
#         (validate plane never falls back to BUBBLES_OBS_OPERATE_* env) and the
#         operate URL never appears on stdout
#   T3.8  profile env binding: with BOTH planes' env set, validate resolves the
#         VALIDATE url and NEVER the operate url
#   T3.8  missing required profile env → exit 1, fail loud
#   usage: missing --plane / --signal / bad value / unknown flag / bypass flag → 2
#   --help → 0
#   missing-parser (yq) → exit 0, neutral adapter=none, WARN
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
RESOLVER="$SCRIPT_DIR/observability-endpoint-resolve.sh"
BASH_BIN="$(command -v bash)"

if [[ ! -x "$RESOLVER" ]]; then
  echo "observability-endpoint-resolve-selftest: resolver not executable: $RESOLVER" >&2
  exit 2
fi

if ! command -v yq >/dev/null 2>&1; then
  echo "SKIP: observability-endpoint-resolve-selftest (yq not installed)"
  exit 0
fi

# Defend against inherited plane env leaking into the resolver under test.
while IFS= read -r _leak_var; do
  [[ -n "$_leak_var" ]] && unset "$_leak_var"
done < <(compgen -v | grep -E '^BUBBLES_OBS_(VALIDATE|OPERATE)_' || true)

WORKSPACE="$(mktemp -d "${HOME}/.bubbles-selftest-obs-resolve.XXXXXX")"
cleanup() { rm -rf "$WORKSPACE"; }
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
ok() { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko() { printf '[selftest] FAIL: %s\n' "$*" >&2; FAIL_COUNT=$((FAIL_COUNT + 1)); }

REPO="$WORKSPACE/repo"
mkdir -p "$REPO/.github"
cat > "$REPO/.github/bubbles-project.yaml" <<'EOF'
traceContracts:
  observability:
    schemaVersion: 1
    posture: wired
    endpoints:
      validate:
        alerts: { adapter: none }
        sloBurn: { adapter: prometheus, profile: test }
        errorRate: { adapter: prometheus, profile: test }
        deployImpact: { adapter: none }
      operate:
        alerts: { adapter: prometheus, profile: prod }
        sloBurn: { adapter: prometheus, profile: prod }
        errorRate: { adapter: prometheus, profile: prod }
        deployImpact: { adapter: prometheus, profile: prod }
EOF

RC=""; OUT=""; ERROUT=""
run_resolve() {
  "$BASH_BIN" "$RESOLVER" "$@" >"$WORKSPACE/out" 2>"$WORKSPACE/err"
  RC=$?
  OUT="$(cat "$WORKSPACE/out")"
  ERROUT="$(cat "$WORKSPACE/err")"
}
run_resolve_no_parser() {
  local empty="$WORKSPACE/emptybin"
  mkdir -p "$empty"
  PATH="$empty" "$BASH_BIN" "$RESOLVER" "$@" >"$WORKSPACE/out" 2>"$WORKSPACE/err"
  RC=$?
  OUT="$(cat "$WORKSPACE/out")"
  ERROUT="$(cat "$WORKSPACE/err")"
}

assert_exit()    { local w="$1" l="$2"; if [[ "$RC" == "$w" ]]; then ok "$l (exit $RC)"; else ko "$l: expected exit $w, got $RC"; printf '  --- stdout ---\n%s\n  --- stderr ---\n%s\n' "$OUT" "$ERROUT" >&2; fi; }
assert_out()     { local n="$1" l="$2"; if grep -qF -- "$n" <<<"$OUT"; then ok "$l (stdout has '$n')"; else ko "$l: stdout missing '$n'"; printf '  --- stdout ---\n%s\n' "$OUT" >&2; fi; }
assert_out_not() { local n="$1" l="$2"; if grep -qF -- "$n" <<<"$OUT"; then ko "$l: stdout unexpectedly has '$n'"; printf '  --- stdout ---\n%s\n' "$OUT" >&2; else ok "$l (stdout absent '$n')"; fi; }
assert_err()     { local n="$1" l="$2"; if grep -qiF -- "$n" <<<"$ERROUT"; then ok "$l (stderr has '$n')"; else ko "$l: stderr missing '$n'"; printf '  --- stderr ---\n%s\n' "$ERROUT" >&2; fi; }

VAL_URL="http://test-stack-prometheus:9090"
PROD_URL="http://prod-prometheus:9090"

# --- T3.3: validate/sloBurn with validate env → prometheus/test ----------
BUBBLES_OBS_VALIDATE_PROMETHEUS_BASE_URL="$VAL_URL" \
BUBBLES_OBS_VALIDATE_PROMETHEUS_CURL_MAX_TIME="10" \
BUBBLES_OBS_VALIDATE_PROMETHEUS_QUERY_SLO_BURN="slo:burn_rate" \
  run_resolve --plane validate --signal sloBurn --repo-root "$REPO"
assert_exit 0 "validate/sloBurn resolves"
assert_out "adapter=prometheus" "validate/sloBurn adapter"
assert_out "profile=test" "validate/sloBurn profile"
assert_out "PROMETHEUS_BASE_URL=$VAL_URL" "validate/sloBurn materializes validate base url"
assert_out "PROMETHEUS_QUERY_SLO_BURN=slo:burn_rate" "validate/sloBurn materializes query"

# --- T3.3: operate/alerts with operate env → prometheus/prod -------------
BUBBLES_OBS_OPERATE_PROMETHEUS_BASE_URL="$PROD_URL" \
BUBBLES_OBS_OPERATE_PROMETHEUS_CURL_MAX_TIME="10" \
  run_resolve --plane operate --signal alerts --repo-root "$REPO"
assert_exit 0 "operate/alerts resolves"
assert_out "adapter=prometheus" "operate/alerts adapter"
assert_out "profile=prod" "operate/alerts profile"
assert_out "PROMETHEUS_BASE_URL=$PROD_URL" "operate/alerts materializes operate base url"

# --- T3.3: validate/alerts configured as none → adapter=none -------------
run_resolve --plane validate --signal alerts --repo-root "$REPO"
assert_exit 0 "validate/alerts (none) resolves"
assert_out "adapter=none" "validate/alerts is none"

# --- T3.7: PROD-BLOCK — validate cannot use operate env ------------------
# Set ONLY operate env; request the validate plane. The resolver must NOT fall
# back to BUBBLES_OBS_OPERATE_* and must fail loud for the missing validate env,
# and the prod URL must never reach stdout.
BUBBLES_OBS_OPERATE_PROMETHEUS_BASE_URL="$PROD_URL" \
BUBBLES_OBS_OPERATE_PROMETHEUS_CURL_MAX_TIME="10" \
BUBBLES_OBS_OPERATE_PROMETHEUS_QUERY_SLO_BURN="slo:burn_rate" \
  run_resolve --plane validate --signal sloBurn --repo-root "$REPO"
assert_exit 1 "prod-block: validate cannot resolve with only operate env"
assert_err "BUBBLES_OBS_VALIDATE_PROMETHEUS_BASE_URL" "prod-block names the missing VALIDATE var"
assert_out_not "$PROD_URL" "prod-block: operate URL never reaches stdout"

# --- T3.8: profile env binding — both planes set, validate wins ----------
BUBBLES_OBS_VALIDATE_PROMETHEUS_BASE_URL="$VAL_URL" \
BUBBLES_OBS_VALIDATE_PROMETHEUS_CURL_MAX_TIME="10" \
BUBBLES_OBS_VALIDATE_PROMETHEUS_QUERY_SLO_BURN="slo:burn_rate" \
BUBBLES_OBS_OPERATE_PROMETHEUS_BASE_URL="$PROD_URL" \
BUBBLES_OBS_OPERATE_PROMETHEUS_CURL_MAX_TIME="10" \
BUBBLES_OBS_OPERATE_PROMETHEUS_QUERY_SLO_BURN="slo:burn_rate" \
  run_resolve --plane validate --signal sloBurn --repo-root "$REPO"
assert_exit 0 "profile-binding: validate resolves with both planes set"
assert_out "PROMETHEUS_BASE_URL=$VAL_URL" "profile-binding: validate URL materialized"
assert_out_not "$PROD_URL" "profile-binding: operate URL never materialized for validate"

# --- T3.8: missing required profile env fails loud -----------------------
run_resolve --plane validate --signal sloBurn --repo-root "$REPO"
assert_exit 1 "missing profile env fails loud"
assert_err "missing required profile env" "missing-env message is loud"

# --- --names-only: report wiring WITHOUT any secret env (read-only query) ---
# The health-check consumer (observability-check.sh) calls this. With NO
# BUBBLES_OBS_*_ env set, a wired prometheus adapter must still resolve its
# NAME (exit 0) and must NOT fail-loud on missing secrets.
run_resolve --plane validate --signal sloBurn --names-only --repo-root "$REPO"
assert_exit 0 "names-only resolves wired adapter without secret env"
assert_out "adapter=prometheus" "names-only reports adapter name"
assert_out "profile=test" "names-only reports profile"
assert_out_not "PROMETHEUS_BASE_URL" "names-only does NOT materialize env"

# --- --names-only: still honors prod-block (validate never reads operate) ---
run_resolve --plane operate --signal alerts --names-only --repo-root "$REPO"
assert_exit 0 "names-only operate/alerts resolves"
assert_out "adapter=prometheus" "names-only operate/alerts adapter"
assert_out "profile=prod" "names-only operate/alerts profile"

# --- --names-only: unconfigured signal → neutral none --------------------
run_resolve --plane validate --signal alerts --names-only --repo-root "$REPO"
assert_exit 0 "names-only unconfigured signal no-ops"
assert_out "adapter=none" "names-only none for unconfigured signal"

# --- usage errors --------------------------------------------------------
run_resolve --signal sloBurn --repo-root "$REPO"
assert_exit 2 "missing --plane is usage error"
run_resolve --plane validate --repo-root "$REPO"
assert_exit 2 "missing --signal is usage error"
run_resolve --plane bogus --signal sloBurn --repo-root "$REPO"
assert_exit 2 "invalid --plane is usage error"
run_resolve --plane validate --signal bogus --repo-root "$REPO"
assert_exit 2 "invalid --signal is usage error"

# --- NO bypass flag ------------------------------------------------------
for flag in --skip --force --ignore; do
  run_resolve "$flag" --plane validate --signal alerts --repo-root "$REPO"
  assert_exit 2 "bypass flag $flag rejected"
done

# --- --help --------------------------------------------------------------
run_resolve --help
assert_exit 0 "--help exits 0"

# --- missing-parser → WARN-and-skip, neutral none ------------------------
run_resolve_no_parser --plane validate --signal sloBurn --repo-root "$REPO"
assert_exit 0 "missing-parser WARN-and-skip"
assert_out "adapter=none" "missing-parser emits neutral none"

echo ""
echo "observability-endpoint-resolve-selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if (( FAIL_COUNT == 0 )); then
  echo "observability-endpoint-resolve selftest passed."
  exit 0
else
  echo "observability-endpoint-resolve selftest FAILED." >&2
  exit 1
fi
