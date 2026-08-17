#!/usr/bin/env bash
# bubbles/scripts/generate-gate-enforcement-selftest.sh
#
# Hermetic selftest for generate-gate-enforcement.sh (IMP-047 S-A).
#
# Every case builds its own fixture repository under mktemp. The load-bearing
# property is that the generated region is DERIVED: a hand edit inside it must
# fail `--check`, an evidence change must move the derived value, and a gate
# whose blocking status cannot be derived must say `unknown` rather than pick a
# plausible answer.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GENERATOR="$SCRIPT_DIR/generate-gate-enforcement.sh"
GREP_TOOL="$SCRIPT_DIR/gate-id-grep.sh"
NAME="generate-gate-enforcement-selftest"

for required in "$GENERATOR" "$GREP_TOOL"; do
  if [[ ! -f "$required" ]]; then
    printf '%s: required surface missing: %s\n' "$NAME" "$required" >&2
    exit 2
  fi
done
if ! command -v python3 >/dev/null 2>&1; then
  printf '%s: SKIP (python3 not installed)\n' "$NAME"
  exit 0
fi

failures=0
checks=0

ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d 2>/dev/null)" || {
  printf '%s: cannot create temp dir\n' "$NAME" >&2
  exit 1
}
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# A fixture repo carrying only the surfaces the generator reads: a gate
# registry, a workflow registry, the scanner, and the generator itself.
new_fixture() {
  local root="$WORK/$1"
  mkdir -p "$root/bubbles/registry" "$root/bubbles/scripts" "$root/bubbles/workflows" "$root/agents"
  cp "$GENERATOR" "$root/bubbles/scripts/generate-gate-enforcement.sh"
  cp "$GREP_TOOL" "$root/bubbles/scripts/gate-id-grep.sh"

  cat >"$root/bubbles/registry/gates.yaml" <<'EOF'
gates:
  G001:
    since: "5.2.0"
    name: mode_bound_gate
    classification: businessInvariant
    enforcedBy: [ mode-required ]
    description: Bound to a mode requiredGates list.
  G002:
    since: "5.2.0"
    name: script_bound_gate
    classification: businessInvariant
    enforcedBy: [ script:bubbles/scripts/refusing-guard.sh ]
    description: Bound to a refusing script.
  G003:
    since: "5.2.0"
    name: advisory_gate
    classification: businessInvariant
    enforcedBy: [ script:bubbles/scripts/advisory-report.sh ]
    description: Bound to a script that only ever reports.
  G004:
    since: "5.2.0"
    name: behavioral_only_gate
    classification: modelCompensation
    enforcedBy: [ unbound ]
    description: Declared unbound while an agent surface names it.
EOF

  cat >"$root/bubbles/workflows.yaml" <<'EOF'
version: 1
defaultMode: full-delivery
EOF
  cat >"$root/bubbles/workflows/modes.yaml" <<'EOF'
modes:
  full-delivery:
    requiredGates: [ G001 ]
EOF

  cat >"$root/bubbles/scripts/refusing-guard.sh" <<'EOF'
#!/usr/bin/env bash
# Enforces G002.
if [[ -n "${FINDING:-}" ]]; then
  printf 'G002 violated\n' >&2
  exit 1
fi
exit 0
EOF

  cat >"$root/bubbles/scripts/advisory-report.sh" <<'EOF'
#!/usr/bin/env bash
printf 'G003 advisory: reporting only, never refusing\n'
exit 0
EOF

  cat >"$root/agents/bubbles.validate.agent.md" <<'EOF'
# bubbles.validate

G004 is enforced behaviorally by this agent during validation review.
EOF

  printf '%s' "$root"
}

gen() {
  local root="$1"
  shift
  set +e
  bash "$root/bubbles/scripts/generate-gate-enforcement.sh" --repo-root "$root" "$@" \
    >"$WORK/out" 2>"$WORK/err"
  GEN_RC=$?
  set -e
  return "$GEN_RC"
}

fixture="$(new_fixture base)"

# --- 1. first run creates the generated region -------------------------------
if gen "$fixture" && grep -q 'GENERATED:GATE_ENFORCEMENT_START' "$fixture/bubbles/registry/gates.yaml"; then
  ok "the generator creates the generated region on first run"
else
  bad "generator creates the region" "rc=$GEN_RC out=$(cat "$WORK/out" "$WORK/err")"
fi

registry="$fixture/bubbles/registry/gates.yaml"

# --- 2. mode-required and script bindings are derived, not copied ------------
if grep -qE '^    G001: \{ enforcedBy: \[ mode-required \], blocking: blocking, blockingBasis: mode-required-transition-refusal' "$registry" &&
  grep -qE '^    G002: \{ enforcedBy: \[ script:bubbles/scripts/refusing-guard.sh \], blocking: blocking, blockingBasis: script-exit-nonzero' "$registry"; then
  ok "mode-required and refusing-script bindings derive a blocking status"
else
  bad "bindings derive blocking status" "$(grep -E '^    G00[12]:' "$registry")"
fi

# --- 3. a script that never refuses derives advisory, not blocking -----------
# Without this the answer to "how many gates block?" is just "all of them".
if grep -qE '^    G003: \{ enforcedBy: \[ script:bubbles/scripts/advisory-report.sh \], blocking: advisory, blockingBasis: script-exit-zero-only' "$registry"; then
  ok "a report-only script derives advisory rather than blocking"
else
  bad "report-only script derives advisory" "$(grep -E '^    G003:' "$registry")"
fi

# --- 4. ADVERSARIAL: the unbound/behavioral contradiction is resolved --------
# This is the G071 shape: the field says `unbound` while a real surface names
# the gate. The derived value must name the surface, the status must be
# `unknown` (behavioral exit behaviour is not derivable), and the disagreement
# must be reported rather than silently overwritten.
if grep -qE '^    G004: \{ enforcedBy: \[ behavioral:bubbles.validate \], blocking: unknown, blockingBasis: behavioral-only-no-derivable-exit, agreement: contradiction, declaredEnforcedBy: \[ unbound \] \}' "$registry"; then
  ok "a declared-unbound gate with a real surface is reported as a contradiction"
else
  bad "unbound/behavioral contradiction is resolved from evidence" "$(grep -E '^    G004:' "$registry")"
fi

# --- 5. --check passes on a freshly generated region -------------------------
if gen "$fixture" --check; then
  ok "--check passes immediately after generation"
else
  bad "--check passes when current" "rc=$GEN_RC out=$(cat "$WORK/out" "$WORK/err")"
fi

# --- 6. ADVERSARIAL: a hand edit inside the region fails --check -------------
# This is the property that makes the block an authority instead of a copy.
python3 - "$registry" <<'PY'
import sys
from pathlib import Path
p = Path(sys.argv[1])
text = p.read_text(encoding="utf-8")
p.write_text(text.replace("blocking: advisory", "blocking: blocking"), encoding="utf-8")
PY
if ! gen "$fixture" --check; then
  if [[ "$GEN_RC" -eq 1 ]] && grep -q 'DRIFT' "$WORK/out"; then
    ok "a hand edit inside the generated region fails --check with exit 1"
  else
    bad "hand edit fails --check" "rc=$GEN_RC out=$(cat "$WORK/out")"
  fi
else
  bad "hand edit fails --check" "--check accepted a hand-edited region"
fi

# --- 7. regeneration restores the derived value ------------------------------
gen "$fixture" >/dev/null 2>&1
if grep -qE '^    G003: .*blocking: advisory' "$registry"; then
  ok "regeneration overwrites the hand edit with the derived value"
else
  bad "regeneration restores derived value" "$(grep -E '^    G003:' "$registry")"
fi

# --- 8. ADVERSARIAL: the derived value tracks the evidence -------------------
# Teach the advisory script to refuse and the status must move. A block that did
# not move here would be a hand-written list wearing a GENERATED marker.
cat >"$fixture/bubbles/scripts/advisory-report.sh" <<'EOF'
#!/usr/bin/env bash
if [[ -n "${FINDING:-}" ]]; then
  printf 'G003 violated\n' >&2
  exit 1
fi
exit 0
EOF
gen "$fixture" >/dev/null 2>&1
if grep -qE '^    G003: .*blocking: blocking, blockingBasis: script-exit-nonzero' "$registry"; then
  ok "changing the enforcing script's exit behaviour moves the derived status"
else
  bad "derived status tracks the evidence" "$(grep -E '^    G003:' "$registry")"
fi

# --- 9. ADVERSARIAL: usage text is not a binding -----------------------------
# gate-hit-log.sh documents `--passed "G001 G002"` in its usage heredoc. Reading
# that as enforcement made three registry tools "enforce" whatever their help
# text quoted.
usage_fixture="$(new_fixture usage-text)"
cat >"$usage_fixture/bubbles/scripts/telemetry-writer.sh" <<'SCRIPT'
#!/usr/bin/env bash
usage() {
  cat <<'USAGE'
usage: telemetry-writer.sh --passed "G002 G003"
USAGE
}
usage
exit 0
SCRIPT
gen "$usage_fixture" >/dev/null 2>&1
if ! grep -qE '^    G00[23]: .*telemetry-writer\.sh' "$usage_fixture/bubbles/registry/gates.yaml"; then
  ok "a gate id quoted in usage text is not recorded as an enforcement binding"
else
  bad "usage text is not a binding" "$(grep -E 'telemetry-writer' "$usage_fixture/bubbles/registry/gates.yaml")"
fi

# --- 10. ADVERSARIAL: a doc that describes a gate does not enforce it --------
doc_fixture="$(new_fixture doc-only)"
mkdir -p "$doc_fixture/docs"
cat >"$doc_fixture/docs/gate-notes.md" <<'DOC'
# Gate notes

G002 refuses a transition when the guarded condition fails.
DOC
gen "$doc_fixture" >/dev/null 2>&1
if ! grep -qE '^    G002: .*docs/gate-notes\.md' "$doc_fixture/bubbles/registry/gates.yaml"; then
  ok "a documentation reference is not recorded as an enforcement binding"
else
  bad "documentation is not a binding" "$(grep -E '^    G002:' "$doc_fixture/bubbles/registry/gates.yaml")"
fi

# --- 11. the generated entries never collide with the registry's own keys ----
# A sibling block at two-space indent doubled every gate count in the framework;
# the entries must stay one level deeper.
if [[ "$(grep -cE '^  G[0-9]{3}:' "$registry")" == "4" ]]; then
  ok "the generated entries do not add two-space-indented gate keys"
else
  bad "generated entries stay out of the registry key namespace" \
    "$(grep -cE '^  G[0-9]{3}:' "$registry") two-space gate keys"
fi

# --- 12. no bypass flag exists -----------------------------------------------
bypass_ok=1
for flag in --skip --force --ignore; do
  set +e
  bash "$GENERATOR" --repo-root "$fixture" "$flag" >/dev/null 2>&1
  rc=$?
  set -e
  if [[ "$rc" -ne 2 ]]; then
    bad "generator refuses the bypass flag $flag" "exit was $rc"
    bypass_ok=0
  fi
done
[[ "$bypass_ok" -eq 1 ]] && ok "every bypass-shaped flag is refused with a usage error"

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
