#!/usr/bin/env bash
#
# runtime-concurrency-selftest.sh — IMP-102 SCOPE-8.
#
# Proves the concurrency-safety fixes for the runtime state surface:
#
#   (1) state-snapshot no-lost-update.
#       state-snapshot.sh now serializes its whole session-file interaction —
#       the mirror-session mirror (repository-binding.sh sets
#       `.repositoryBindingMirror`), the `turnSnapshots` append, and the
#       `convergenceLoops` update — under ONE exclusive lock. N concurrent
#       snapshots against the SAME session file therefore lose NO update:
#       `turnSnapshots` ends up with exactly N records and every parallel
#       `convergenceLoops` key is present with its value.
#
#       Non-tautology proof: the SAME parallel workload run against the
#       pre-fix state-snapshot.sh (`git show 650639b:...`, which has no lock)
#       LOSES updates in at least one round, while the fixed version NEVER
#       loses across the same rounds — demonstrating the race is real and the
#       lock closes it.
#
#   (2) runtime-leases stale-lock recovery.
#       runtime-leases.sh `acquire_registry_lock` used to `die` on any held
#       lock; a SIGKILLed holder (whose release trap never fired) left the lock
#       dir behind forever, permanently deadlocking every future acquire. The
#       fix records the holder pid + honours the lock dir mtime so a STALE lock
#       (dead holder pid, or mtime older than staleAfterMinutes) is broken and
#       the acquire succeeds, while a LIVE, fresh holder's lock is still
#       respected (acquire refuses without stealing it).
#
# Graceful-skip: jq absent -> SKIP (exit 0). The non-tautology old-baseline
# sub-proof additionally SKIPs (without failing the harness) if git or the
# 650639b blob is unavailable.
#
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SNAPSHOT="$SCRIPT_DIR/state-snapshot.sh"
BINDING="$SCRIPT_DIR/repository-binding.sh"
RUNTIME_SCRIPT="$SCRIPT_DIR/runtime-leases.sh"
OLD_REF="650639b"

pass_count=0
fail_count=0

pass() { pass_count=$((pass_count + 1)); echo "PASS: $1"; }
fail() { fail_count=$((fail_count + 1)); echo "FAIL: $1"; }

# --- Graceful skip ---------------------------------------------------------

if [[ ! -x "$SNAPSHOT" || ! -x "$BINDING" || ! -x "$RUNTIME_SCRIPT" ]]; then
  echo "runtime-concurrency-selftest: required scripts missing/not executable — SKIP"
  exit 0
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "runtime-concurrency-selftest: SKIP (jq not installed)"
  exit 0
fi

# macOS mktemp -d sits under the /var symlink; canonicalize the fixture root.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
BG_PIDS=()

cleanup() {
  local p
  for p in "${BG_PIDS[@]:-}"; do
    [[ -n "$p" ]] || continue
    kill "$p" 2>/dev/null || true
    wait "$p" 2>/dev/null || true
  done
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

echo "Running runtime concurrency selftest..."

# ===========================================================================
# Case 1: state-snapshot no-lost-update (+ non-tautology old-baseline proof)
# ===========================================================================

N=8
ROUNDS=10

REPO="$TMP_ROOT/snapshot-repo"
mkdir -p "$REPO/.specify/memory" "$REPO/bubbles/scripts" "$REPO/agents"
printf 'test-version\n' > "$REPO/VERSION"
printf '#!/usr/bin/env bash\n' > "$REPO/install.sh"
printf '#!/usr/bin/env bash\n' > "$REPO/bubbles/scripts/cli.sh"
git init -q "$REPO"

SESSION_FILE="$REPO/.specify/memory/bubbles.session.json"

# Pre-generate one bound (control-file, packet) pair per parallel worker. Each
# uses a DISTINCT session id + DISTINCT (external) control file but the SAME
# repository root, so every worker writes the SAME session file (the shared
# resource under test) while never contending on the control file.
CTRLS=()
PKTS=()
prepared=true
for (( i = 0; i < N; i++ )); do
  sid="conc-$i"
  control_dir="$TMP_ROOT/controls/$sid"
  mkdir -p "$control_dir"
  chmod 700 "$control_dir"
  ctrl="$control_dir/repository-binding.json"
  pkt="$TMP_ROOT/packets/$sid.packet.json"
  mkdir -p "$TMP_ROOT/packets"
  preflight_out="$(bash "$BINDING" preflight \
    --session-id "$sid" \
    --session-control-file "$ctrl" \
    --expected-control-revision 0 \
    --request-class TARGETLESS_MODE \
    --repository-root "$REPO" \
    --workspace-root "$REPO" 2>/dev/null)"
  printf '%s\n' "$preflight_out" \
    | awk '/^\{.*"repositoryRoot"/ { packet = $0 } END { print packet }' > "$pkt"
  if ! jq -e '.repositoryResolution.actionable == true' "$pkt" >/dev/null 2>&1; then
    prepared=false
    break
  fi
  CTRLS+=("$ctrl")
  PKTS+=("$pkt")
done

reset_session() {
  rm -f "$SESSION_FILE" 2>/dev/null || true
  rm -rf "$SESSION_FILE.lock" 2>/dev/null || true
}

# Launch N snapshots in parallel against $1 (a state-snapshot.sh path) and wait.
run_parallel_snapshots() {
  local snapshot_bin="$1"
  local pids=()
  local i
  for (( i = 0; i < N; i++ )); do
    BUBBLES_AGENT_NAME="conc-agent-$i" bash "$snapshot_bin" \
      --phase "phase-$i" --mode start \
      --session-id "conc-$i" \
      --session-control-file "${CTRLS[$i]}" \
      --binding-packet-file "${PKTS[$i]}" \
      --convergence-iteration "$((100 + i))" \
      --spec-dir "specs/000-p$i" \
      >/dev/null 2>&1 &
    pids+=("$!")
  done
  local pid
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
  done
}

# turnSnapshots length in the current session file (0 if missing/invalid).
session_turn_count() {
  jq '(.turnSnapshots // []) | length' "$SESSION_FILE" 2>/dev/null || echo 0
}

# 0 = intact (no lost update), 1 = a lost update was detected.
session_intact() {
  jq -e . "$SESSION_FILE" >/dev/null 2>&1 || return 1
  local turns i want got
  turns="$(session_turn_count)"
  [[ "$turns" == "$N" ]] || return 1
  for (( i = 0; i < N; i++ )); do
    want=$((100 + i))
    got="$(jq --arg s "specs/000-p$i" \
      '((.convergenceLoops // []) | map(select(.specDir == $s)) | .[0].iterationCount) // empty' \
      "$SESSION_FILE" 2>/dev/null)"
    [[ "$got" == "$want" ]] || return 1
  done
  return 0
}

# Prepare the pre-fix (650639b) baseline for the non-tautology proof. Its sibling
# repository-binding.sh (unchanged at 650639b) is copied alongside it so the old
# state-snapshot resolves the same validator.
OLD_SNAPSHOT=""
if command -v git >/dev/null 2>&1 \
  && git -C "$SOURCE_ROOT" cat-file -e "$OLD_REF:bubbles/scripts/state-snapshot.sh" 2>/dev/null; then
  OLD_DIR="$TMP_ROOT/old-baseline"
  mkdir -p "$OLD_DIR"
  if git -C "$SOURCE_ROOT" show "$OLD_REF:bubbles/scripts/state-snapshot.sh" > "$OLD_DIR/state-snapshot.sh" 2>/dev/null \
    && git -C "$SOURCE_ROOT" show "$OLD_REF:bubbles/scripts/repository-binding.sh" > "$OLD_DIR/repository-binding.sh" 2>/dev/null; then
    chmod +x "$OLD_DIR/state-snapshot.sh" "$OLD_DIR/repository-binding.sh"
    OLD_SNAPSHOT="$OLD_DIR/state-snapshot.sh"
  fi
fi

if [[ "$prepared" != true ]]; then
  fail "state-snapshot no-lost-update: could not prepare bound repository fixtures"
else
  fixed_losses=0
  old_losses=0
  old_ran=0
  round=1
  while (( round <= ROUNDS )); do
    # --- fixed version: must NEVER lose ---
    reset_session
    run_parallel_snapshots "$SNAPSHOT"
    fixed_turns="$(session_turn_count)"
    if session_intact; then
      fixed_msg="intact ($fixed_turns/$N)"
    else
      fixed_losses=$((fixed_losses + 1))
      fixed_msg="LOST ($fixed_turns/$N)"
    fi

    # --- pre-fix baseline: expected to lose in >=1 round ---
    old_turns="n/a"
    if [[ -n "$OLD_SNAPSHOT" ]]; then
      old_ran=1
      reset_session
      run_parallel_snapshots "$OLD_SNAPSHOT"
      old_turns="$(session_turn_count)"
      if ! session_intact; then
        old_losses=$((old_losses + 1))
      fi
    fi

    echo "  round $round: fixed=$fixed_msg  pre-fix(650639b) turnSnapshots=$old_turns/$N"
    round=$((round + 1))
  done

  if [[ "$fixed_losses" -eq 0 ]]; then
    pass "fixed state-snapshot loses NO update across $ROUNDS parallel rounds of $N (turnSnapshots==$N, all convergence keys present)"
  else
    fail "fixed state-snapshot lost an update in $fixed_losses/$ROUNDS rounds"
  fi

  if [[ "$old_ran" -eq 1 ]]; then
    if [[ "$old_losses" -ge 1 ]]; then
      pass "NON-TAUTOLOGY: pre-fix state-snapshot (650639b, lock-free) lost updates in $old_losses/$ROUNDS rounds — the race is real and the lock closes it"
    else
      fail "NON-TAUTOLOGY expected the pre-fix state-snapshot to lose at least once across $ROUNDS rounds but it never did (test may be insufficiently contended)"
    fi
  else
    echo "SKIP: non-tautology old-baseline proof (git or $OLD_REF blob unavailable)"
  fi
  reset_session
fi

# ===========================================================================
# Case 2: runtime-leases stale-lock recovery
# ===========================================================================

RT_ROOT="$TMP_ROOT/runtime-repo"
mkdir -p "$RT_ROOT/.specify/memory" "$RT_ROOT/.specify/runtime"
cat > "$RT_ROOT/.specify/memory/bubbles.config.json" <<'EOF'
{
  "version": 2,
  "defaults": {
    "runtime": { "leaseTtlMinutes": 20, "staleAfterMinutes": 60, "reusePolicy": "fingerprint-match-only", "source": "repo-default" }
  },
  "modeOverrides": {},
  "metrics": { "enabled": false, "activityTrackingEnabled": false }
}
EOF
printf '{\n  "sessionId": "runtime-conc"\n}\n' > "$RT_ROOT/.specify/memory/bubbles.session.json"
RT_LOCK="$RT_ROOT/.specify/runtime/.locks/resource-leases.lock"

run_acquire() {
  local resource="$1"
  BUBBLES_REPO_ROOT="$RT_ROOT" BUBBLES_SESSION_ID="runtime-conc" BUBBLES_AGENT_NAME="bubbles.validate" \
    bash "$RUNTIME_SCRIPT" acquire \
    --purpose validation --environment dev --share-mode shared-compatible \
    --fingerprint-input "schema:v1" --resource "$resource" >/dev/null 2>&1
}

plant_lock() {
  local pid="$1"
  rm -rf "$RT_LOCK" 2>/dev/null || true
  mkdir -p "$RT_LOCK"
  printf '%s\n' "$pid" > "$RT_LOCK/holder.pid"
}

# --- Sub-case A: dead holder pid -> stale -> broken -> acquire succeeds -----
bash -c 'exit 0' & dead_pid=$!
wait "$dead_pid" 2>/dev/null || true
if kill -0 "$dead_pid" 2>/dev/null; then
  echo "SKIP: stale-lock dead-pid sub-case (reaped pid $dead_pid unexpectedly still alive — pid reuse)"
else
  plant_lock "$dead_pid"
  if run_acquire "container:stale-dead"; then
    pass "stale lock with a DEAD holder pid is broken and acquire succeeds (no deadlock)"
  else
    fail "stale lock with a DEAD holder pid should be broken; acquire refused/deadlocked instead"
  fi
fi

# --- Sub-case B: live, fresh holder -> respected -> acquire refuses ---------
sleep 60 & live_pid=$!
BG_PIDS+=("$live_pid")
plant_lock "$live_pid"
touch "$RT_LOCK" 2>/dev/null || true
if run_acquire "container:live-fresh"; then
  fail "live, fresh holder lock should be respected; acquire wrongly SUCCEEDED (stole a live lock)"
else
  if [[ -d "$RT_LOCK" ]]; then
    pass "live, fresh holder lock is respected (acquire refuses and does NOT steal the held lock)"
  else
    fail "acquire refused but the live holder's lock dir was removed (should not be broken)"
  fi
fi
kill "$live_pid" 2>/dev/null || true
wait "$live_pid" 2>/dev/null || true
rm -rf "$RT_LOCK" 2>/dev/null || true

# --- Sub-case C: live holder but ancient mtime -> stale -> broken -----------
sleep 60 & live_pid2=$!
BG_PIDS+=("$live_pid2")
plant_lock "$live_pid2"
# Age the lock dir well past staleAfterMinutes (portable BSD/GNU touch -t form).
touch -t 202001010000 "$RT_LOCK" 2>/dev/null || true
if run_acquire "container:stale-age"; then
  pass "stale lock with a live holder but mtime older than staleAfterMinutes is broken and acquire succeeds"
else
  fail "stale-by-age lock should be broken; acquire refused/deadlocked instead"
fi
kill "$live_pid2" 2>/dev/null || true
wait "$live_pid2" 2>/dev/null || true
rm -rf "$RT_LOCK" 2>/dev/null || true

# ===========================================================================
# Case 3: state-snapshot mkdir-fallback lock hygiene (the no-flock path)
# ===========================================================================
#
# Case 1 exercises whichever lock strategy the host happens to provide, which on
# Linux and CI is always flock. The mkdir mutex is the strategy stock macOS
# actually runs (no flock in the base install, so GitHub's macOS runners take
# it), and its stale-break is the step that can lose an update — so it needs
# direct coverage on every host, not only on macOS. A PATH sandbox mirroring the
# real PATH minus `flock` reproduces the macOS condition here.

NOFLOCK_BIN="$TMP_ROOT/noflock-bin"
mkdir -p "$NOFLOCK_BIN"
while IFS= read -r path_dir; do
  [[ -n "$path_dir" && -d "$path_dir" ]] || continue
  for path_exe in "$path_dir"/*; do
    [[ -e "$path_exe" ]] || continue
    exe_name="${path_exe##*/}"
    [[ "$exe_name" != "flock" ]] || continue
    [[ -e "$NOFLOCK_BIN/$exe_name" ]] || ln -s "$path_exe" "$NOFLOCK_BIN/$exe_name" 2>/dev/null || true
  done
done < <(printf '%s\n' "$PATH" | tr ':' '\n')

noflock_usable=true
( PATH="$NOFLOCK_BIN"; command -v flock >/dev/null 2>&1 ) && noflock_usable=false
( PATH="$NOFLOCK_BIN"; command -v jq >/dev/null 2>&1 ) || noflock_usable=false

SNAP_TRACE="$TMP_ROOT/snapshot-lock-trace.txt"

if [[ "$prepared" != true || "$noflock_usable" != true ]]; then
  echo "SKIP: state-snapshot mkdir-fallback sub-cases (no-flock PATH sandbox unavailable)"
else
  # --- Sub-case D: dead holder -> stale -> broken -> the write completes ----
  bash -c 'exit 0' & snap_dead_pid=$!
  wait "$snap_dead_pid" 2>/dev/null || true
  if kill -0 "$snap_dead_pid" 2>/dev/null; then
    echo "SKIP: state-snapshot dead-holder break sub-case (reaped pid $snap_dead_pid still alive — pid reuse)"
  else
    reset_session
    rm -f "$SNAP_TRACE" 2>/dev/null || true
    mkdir -p "$SESSION_FILE.lock"
    printf '%s\n' "$snap_dead_pid" > "$SESSION_FILE.lock/holder.pid"
    PATH="$NOFLOCK_BIN" BUBBLES_LOCK_TRACE="$SNAP_TRACE" BUBBLES_AGENT_NAME="conc-agent-0" \
      bash "$SNAPSHOT" --phase phase-dead-holder --mode start \
      --session-id "conc-0" \
      --session-control-file "${CTRLS[0]}" \
      --binding-packet-file "${PKTS[0]}" \
      >/dev/null 2>&1
    snap_dead_rc=$?
    snap_dead_turns="$(session_turn_count)"
    if [[ "$snap_dead_rc" -eq 0 && "$snap_dead_turns" == "1" ]] \
      && grep -q 'BREAK-STALE' "$SNAP_TRACE" 2>/dev/null; then
      pass "state-snapshot mkdir fallback BREAKS a lock whose holder pid is dead and completes its write (no deadlock)"
    else
      fail "state-snapshot mkdir fallback should break a DEAD-holder lock (exit=$snap_dead_rc turnSnapshots=$snap_dead_turns/1 broke=$(grep -c 'BREAK-STALE' "$SNAP_TRACE" 2>/dev/null || echo 0))"
    fi
    rm -rf "$SESSION_FILE.lock" 2>/dev/null || true
  fi

  # --- Sub-case E: live, fresh holder -> respected (lock is NOT stolen) -----
  # This is the property the lost-update defect violated: a waiter must never
  # destroy a lock that a live holder currently owns. Case 1 proves it
  # statistically across parallel rounds; this proves it deterministically.
  reset_session
  rm -f "$SNAP_TRACE" 2>/dev/null || true
  sleep 30 & snap_live_pid=$!
  BG_PIDS+=("$snap_live_pid")
  mkdir -p "$SESSION_FILE.lock"
  printf '%s\n' "$snap_live_pid" > "$SESSION_FILE.lock/holder.pid"
  # Backgrounded WITHOUT a wrapper function on purpose: backgrounding a function
  # forks a subshell, so $! would name the subshell and the kill below would
  # leave the real snapshot alive to write into TMP_ROOT during teardown.
  PATH="$NOFLOCK_BIN" BUBBLES_LOCK_TRACE="$SNAP_TRACE" BUBBLES_AGENT_NAME="conc-agent-0" \
    bash "$SNAPSHOT" --phase phase-live-holder --mode start \
    --session-id "conc-0" \
    --session-control-file "${CTRLS[0]}" \
    --binding-packet-file "${PKTS[0]}" \
    >/dev/null 2>&1 &
  snap_waiter_pid=$!
  BG_PIDS+=("$snap_waiter_pid")
  sleep 3
  snap_live_turns="$(session_turn_count)"
  if [[ -d "$SESSION_FILE.lock" && "$snap_live_turns" == "0" ]] \
    && ! grep -q 'BREAK' "$SNAP_TRACE" 2>/dev/null; then
    pass "state-snapshot mkdir fallback does NOT steal a live, fresh holder's lock (it waits)"
  else
    fail "state-snapshot mkdir fallback stole a live holder's lock (lockDirPresent=$([[ -d "$SESSION_FILE.lock" ]] && echo yes || echo no) turnSnapshots=$snap_live_turns/0 breaks=$(grep -c 'BREAK' "$SNAP_TRACE" 2>/dev/null || echo 0))"
  fi
  kill "$snap_waiter_pid" 2>/dev/null || true
  wait "$snap_waiter_pid" 2>/dev/null || true
  kill "$snap_live_pid" 2>/dev/null || true
  wait "$snap_live_pid" 2>/dev/null || true
  rm -rf "$SESSION_FILE.lock" 2>/dev/null || true
  reset_session
fi

# ===========================================================================

echo
echo "runtime concurrency selftest: $pass_count passed / $fail_count failed"
if [[ "$fail_count" -ne 0 ]]; then
  exit 1
fi
echo "runtime concurrency selftest passed."
exit 0
