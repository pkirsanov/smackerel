#!/usr/bin/env bash
# bubbles/scripts/generate-telemetry-reader-map-selftest.sh
#
# Hermetic selftest for generate-telemetry-reader-map.sh (IMP-047 S-A).
#
# The map's whole value is that it distinguishes a store somebody produces from
# a store nobody does. Case 3 is the one that matters: a declared store with no
# producer and no reader must appear as `unmeasured` rather than be absent, and
# it is the shape that made framework health report fields nobody wrote.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GENERATOR="$SCRIPT_DIR/generate-telemetry-reader-map.sh"
NAME="generate-telemetry-reader-map-selftest"

if [[ ! -f "$GENERATOR" ]]; then
  printf '%s: required surface missing: %s\n' "$NAME" "$GENERATOR" >&2
  exit 2
fi
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

new_fixture() {
  local root="$WORK/$1"
  mkdir -p "$root/bubbles/scripts" "$root/docs/generated"
  cp "$GENERATOR" "$root/bubbles/scripts/generate-telemetry-reader-map.sh"

  cat >"$root/bubbles/workflows.yaml" <<'EOF'
version: 1
metrics:
  enabled: false
  storageFile: .specify/metrics/events.jsonl
  gateTelemetry:
    storageFile: .specify/runtime/gate-hits.jsonl
    producer: bubbles/scripts/gate-hit-log.sh
    reportCommand: "bubbles/scripts/gate-hit-log.sh report"
  derivedFromState:
    source: state.json executionHistory
EOF

  cat >"$root/bubbles/scripts/gate-hit-log.sh" <<'EOF'
#!/usr/bin/env bash
log_path() { printf '%s/.specify/runtime/gate-hits.jsonl' "$1"; }
EOF

  cat >"$root/bubbles/scripts/health-report.sh" <<'EOF'
#!/usr/bin/env bash
STORE=".specify/runtime/gate-hits.jsonl"
wc -l "$STORE"
EOF

  printf '%s' "$root"
}

gen() {
  local root="$1"
  shift
  set +e
  bash "$root/bubbles/scripts/generate-telemetry-reader-map.sh" --repo-root "$root" "$@" \
    >"$WORK/out" 2>"$WORK/err"
  GEN_RC=$?
  set -e
  return "$GEN_RC"
}

fixture="$(new_fixture base)"
MAP="$fixture/docs/generated/telemetry-reader-map.md"

# --- 1. the document is created and carries the generated markers ------------
if gen "$fixture" && [[ -f "$MAP" ]] &&
  grep -q 'GENERATED:TELEMETRY_READER_MAP_START' "$MAP" &&
  grep -q 'GENERATED:TELEMETRY_READER_MAP_END' "$MAP"; then
  ok "the map is generated between markers"
else
  bad "map is generated between markers" "rc=$GEN_RC out=$(cat "$WORK/out" "$WORK/err")"
fi

# --- 2. a confirmed producer and its readers are separated -------------------
# The declared producer must actually reference its own store, or the
# declaration is a claim rather than a fact.
if grep -q '| `.specify/runtime/gate-hits.jsonl` | `bubbles/scripts/gate-hit-log.sh` | `bubbles/scripts/health-report.sh` | direct |' "$MAP"; then
  ok "a confirmed producer is separated from its readers"
else
  bad "producer and readers are separated" "$(grep 'gate-hits' "$MAP")"
fi

# --- 3. ADVERSARIAL: a declared store nobody touches is `unmeasured` ---------
# Omitting this row is how a store with no producer stays invisible while a
# report keeps claiming values from it.
if grep -q '| `.specify/metrics/events.jsonl` | — | — | unmeasured |' "$MAP"; then
  ok "a declared store with no producer and no reader is reported as unmeasured"
else
  bad "unmeasured stores are reported" "$(grep 'events.jsonl' "$MAP")"
fi

# --- 4. a non-file source is `derived`, not silently dropped ----------------
if grep -q '| `(derived) state.json executionHistory` | `state.json executionHistory` | — | derived |' "$MAP"; then
  ok "a registry source that is not a file is reported as derived"
else
  bad "derived sources are reported" "$(grep 'derived' "$MAP" | head -3)"
fi

# --- 5. --check passes on a freshly generated document -----------------------
if gen "$fixture" --check; then
  ok "--check passes immediately after generation"
else
  bad "--check passes when current" "rc=$GEN_RC out=$(cat "$WORK/out")"
fi

# --- 6. ADVERSARIAL: a hand edit fails --check ------------------------------
python3 - "$MAP" <<'PY'
import sys
from pathlib import Path
p = Path(sys.argv[1])
p.write_text(p.read_text(encoding="utf-8").replace("unmeasured", "direct"), encoding="utf-8")
PY
if ! gen "$fixture" --check; then
  if [[ "$GEN_RC" -eq 1 ]] && grep -q 'DRIFT' "$WORK/out"; then
    ok "a hand edit to the generated map fails --check with exit 1"
  else
    bad "hand edit fails --check" "rc=$GEN_RC out=$(cat "$WORK/out")"
  fi
else
  bad "hand edit fails --check" "--check accepted a hand-edited map"
fi

# --- 7. ADVERSARIAL: the map tracks the evidence ----------------------------
# Give the unmeasured store a real reader and its class must move. A row that
# did not move would mean the table is asserted rather than derived.
cat >"$fixture/bubbles/scripts/events-reader.sh" <<'EOF'
#!/usr/bin/env bash
wc -l ".specify/metrics/events.jsonl"
EOF
gen "$fixture" >/dev/null 2>&1
if grep -q '| `.specify/metrics/events.jsonl` | `undeclared` | `bubbles/scripts/events-reader.sh` | direct |' "$MAP"; then
  ok "adding a real reader moves the store out of unmeasured"
else
  bad "map tracks the evidence" "$(grep 'events.jsonl' "$MAP")"
fi

# --- 8. ADVERSARIAL: a store named in prose is not invented as a second store
# A path at the end of a sentence must not be captured with its full stop.
cat >"$fixture/bubbles/scripts/prose-mention.sh" <<'EOF'
#!/usr/bin/env bash
# Outcomes are appended to .specify/runtime/gate-hits.jsonl.
exit 0
EOF
gen "$fixture" >/dev/null 2>&1
if ! grep -q 'gate-hits.jsonl\.`' "$MAP"; then
  ok "a store named at the end of a sentence does not create a phantom store"
else
  bad "no phantom store from prose" "$(grep 'gate-hits' "$MAP")"
fi

# --- 9. no bypass flag exists ------------------------------------------------
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
