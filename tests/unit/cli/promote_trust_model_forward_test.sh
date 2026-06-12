#!/usr/bin/env bash
# tests/unit/cli/promote_trust_model_forward_test.sh
#
# Spec 021 B1a — scripts/deploy/promote.sh (legacy C4 / PATH-2 promote) MUST
# forward the manifest-resolved --trust-model to the adapter invocation, so a CI
# ci-keyless manifest never misresolves to the adapter's stored local-operator
# default.
#
# Auto-discovered by `./smackerel.sh test unit` (tests/unit/cli/*.sh discovery).
# Also runnable directly: bash tests/unit/cli/promote_trust_model_forward_test.sh
#
# Adversarial / non-tautological: asserts BOTH
#   (1) a manifest that OMITS trustModel ⇒ --trust-model=ci-keyless (FR-6 inference)
#   (2) a manifest with explicit trustModel: local-operator ⇒ that value verbatim
# If promote.sh dropped --trust-model (pre-fix), (1)+(2) both fail. If it
# hardcoded ci-keyless, (2) fails. The pre-fix code (no forwarding) fails (1).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
PROMOTE="$REPO_ROOT/scripts/deploy/promote.sh"
PARSE="$REPO_ROOT/scripts/deploy/promote_manifest_parse.sh"
[[ -f "$PROMOTE" ]] || { echo "FAIL: not found: $PROMOTE" >&2; exit 1; }
[[ -f "$PARSE" ]] || { echo "FAIL: not found: $PARSE" >&2; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Isolated repo tree: real promote scripts + a fake smackerel.sh that records the
# argv promote.sh execs, + an in-tree deploy/home-lab adapter params.
mkdir -p "$TMP/scripts/deploy" "$TMP/deploy/home-lab"
cp "$PROMOTE" "$TMP/scripts/deploy/promote.sh"
cp "$PARSE" "$TMP/scripts/deploy/promote_manifest_parse.sh"
chmod +x "$TMP/scripts/deploy/promote.sh"
LOG="$TMP/invocation.log"
cat >"$TMP/smackerel.sh" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$LOG"
EOF
chmod +x "$TMP/smackerel.sh"
cat >"$TMP/deploy/home-lab/params.yaml" <<'EOF'
environment: home-lab
EOF

mk_manifest() {
  # $1 = optional trustModel line (empty ⇒ omitted, the real CI shape)
  local tm_line="$1" out="$2"
  {
    [[ -n "$tm_line" ]] && printf '%s\n' "$tm_line"
    cat <<'EOF'
sourceSha: ae0d540cae0d540cae0d540cae0d540cae0d540c
images:
  - name: smackerel-core
    ref: ghcr.io/pkirsanov/smackerel-core@sha256:eb0b0b7eaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  - name: smackerel-ml
    ref: ghcr.io/pkirsanov/smackerel-ml@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
configBundles:
  - env: home-lab
    ref: ghcr.io/pkirsanov/smackerel-config-bundles:home-lab-ae0d540cae0d540cae0d540cae0d540cae0d540c
    sha256: dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
EOF
  } >"$out"
}

run_promote() {
  : >"$LOG"
  (cd "$TMP" && bash scripts/deploy/promote.sh --target home-lab --build-manifest "$1") >/dev/null 2>&1 || true
}

# Case 1 — ci-keyless inference (manifest OMITS trustModel, like real CI).
mk_manifest "" "$TMP/ci.yaml"
run_promote "$TMP/ci.yaml"
grep -q -- '--trust-model=ci-keyless' "$LOG" \
  || fail "ci-keyless inference: adapter invocation missing --trust-model=ci-keyless; got: $(cat "$LOG")"
echo "PASS: ci-keyless inference forwards --trust-model=ci-keyless"

# Case 2 — explicit trustModel wins (proves the value is READ, not hardcoded).
mk_manifest "trustModel: local-operator" "$TMP/lo.yaml"
run_promote "$TMP/lo.yaml"
grep -q -- '--trust-model=local-operator' "$LOG" \
  || fail "explicit trustModel: adapter invocation missing --trust-model=local-operator; got: $(cat "$LOG")"
echo "PASS: explicit trustModel forwarded verbatim (local-operator)"

echo "ALL PASS: promote_trust_model_forward_test"
