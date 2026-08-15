#!/usr/bin/env bash
# bubbles/scripts/mutation-adapter-contract-selftest.sh
#
# Hermetic contract selftest for the mutation-execution adapter (IMP-040
# SCOPE-7).
#
# The load-bearing half is adversarial. A resolver that quietly degrades a
# BROKEN configuration to `none` makes a typo indistinguishable from a
# deliberate opt-out — and since `none` is what licenses a high-risk scenario to
# accept a weaker negative control, that silent degrade would let a misspelling
# buy a permanent exemption from the strongest proof COV-11 asks for. Cases
# A1-A8 all assert loud refusal rather than degrade.
#
# N1-N3 pin the neutral adapter's shapes. `run` must return the neutral MAP and
# never a survival count: a zero would be a positive claim that no mutant
# survived, which is precisely the unearned confidence this gate removes.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/mutation-resolve.sh"
NONE_ADAPTER="$(cd "$SCRIPT_DIR/.." && pwd)/adapters/mutation/none.sh"
NAME="mutation-adapter-contract-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

# $1 root, $2... lines of the mutationExecution block (omit for no block at all)
make_repo() {
  local root="$1"; shift
  mkdir -p "$root/.github"
  {
    printf 'projectName: fixture\n'
    if [[ $# -gt 0 ]]; then
      printf 'mutationExecution:\n'
      local line
      for line in "$@"; do printf '  %s\n' "$line"; done
    fi
    # A trailing sibling key proves block scoping: keys after the block must
    # not leak into it.
    printf 'codeIndex:\n  adapter: none\n'
  } >"$root/.github/bubbles-project.yaml"
}

run_resolve() {
  set +e
  OUT="$(bash "$RESOLVE" --repo-root "$1" 2>&1)"
  RC=$?
  set -e
}

# --- P1. no mutationExecution block at all resolves neutrally ---------------
R="$WORK/p1"; make_repo "$R"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=none'; then
  ok "P1 absent mutationExecution block resolves to none"
else
  bad "P1 absent block" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P2. no config file at all resolves neutrally --------------------------
R="$WORK/p2"; mkdir -p "$R"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=none'; then
  ok "P2 absent config file resolves to none"
else
  bad "P2 absent config" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. explicit none resolves and names the framework adapter ------------
R="$WORK/p3"; make_repo "$R" 'adapter: none'
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'adapterPath=.*adapters/mutation/none.sh'; then
  ok "P3 explicit none names the framework adapter path"
else
  bad "P3 explicit none" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. a valid command adapter resolves ----------------------------------
R="$WORK/p4"; make_repo "$R" 'adapter: command' 'command: scripts/mut' 'timeoutSeconds: 900'
mkdir -p "$R/scripts"; printf '#!/bin/sh\necho "{}"\n' >"$R/scripts/mut"; chmod +x "$R/scripts/mut"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'command=.*/scripts/mut' \
  && printf '%s' "$OUT" | grep -qx 'timeoutSeconds=900'; then
  ok "P4 a valid command adapter resolves with its timeout"
else
  bad "P4 command adapter" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P5. a command adapter with no timeout takes the default ---------------
R="$WORK/p5"; make_repo "$R" 'adapter: command' 'command: scripts/mut'
mkdir -p "$R/scripts"; printf '#!/bin/sh\necho "{}"\n' >"$R/scripts/mut"; chmod +x "$R/scripts/mut"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'timeoutSeconds=600'; then
  ok "P5 an omitted timeout takes the documented default"
else
  bad "P5 default timeout" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- ADVERSARIAL: every broken config must FAIL LOUD, never degrade --------
adversarial() {
  local label="$1" root="$2"; shift 2
  run_resolve "$root"
  if [[ "$RC" -eq 1 ]] && ! printf '%s' "$OUT" | grep -qx 'adapter=none'; then
    ok "$label"
  else
    bad "$label" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
}

R="$WORK/a1"; make_repo "$R" 'adapter: comand'
adversarial "A1 a misspelled adapter fails loud, it does not degrade to none" "$R"

R="$WORK/a2"; make_repo "$R" 'adapter: command'
adversarial "A2 command adapter with no command: fails loud" "$R"

R="$WORK/a3"; make_repo "$R" 'adapter: command' 'command: scripts/absent'
adversarial "A3 a command that does not exist fails loud" "$R"

R="$WORK/a4"; make_repo "$R" 'adapter: command' 'command: scripts/mut'
mkdir -p "$R/scripts"; printf '#!/bin/sh\n' >"$R/scripts/mut"; chmod -x "$R/scripts/mut"
adversarial "A4 a non-executable command fails loud" "$R"

R="$WORK/a5"; make_repo "$R" 'adapter: command' 'command: /etc/passwd'
adversarial "A5 an absolute command path is refused" "$R"

R="$WORK/a6"; make_repo "$R" 'adapter: command' 'command: ../outside/mut'
adversarial "A6 a parent-traversal command path is refused" "$R"

R="$WORK/a7"; make_repo "$R" 'adapter: command' 'command: scripts/mut' 'timeoutSeconds: soon'
mkdir -p "$R/scripts"; printf '#!/bin/sh\n' >"$R/scripts/mut"; chmod +x "$R/scripts/mut"
adversarial "A7 a non-numeric timeout is refused" "$R"

R="$WORK/a8"; make_repo "$R" 'adapter: command' 'command: scripts/mut' 'timeoutSeconds: 0'
mkdir -p "$R/scripts"; printf '#!/bin/sh\n' >"$R/scripts/mut"; chmod +x "$R/scripts/mut"
adversarial "A8 a zero timeout is refused" "$R"

# --- P6. block scoping: a sibling key must not leak into the block ---------
# codeIndex.adapter is `none` in every fixture; if scoping were broken the
# command fixtures would still resolve, so this asserts the inverse direction:
# a block with ONLY a sibling present must not pick the sibling's adapter.
R="$WORK/p6"; mkdir -p "$R/.github"
printf 'projectName: fixture\ncodeIndex:\n  adapter: command\n  command: scripts/ci\n' >"$R/.github/bubbles-project.yaml"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=none'; then
  ok "P6 a sibling block's adapter does not leak into mutationExecution"
else
  bad "P6 block scoping" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- N1-N3. neutral adapter shapes -----------------------------------------
neutral() {
  local verb="$1" want="$2" label="$3"
  set +e
  local direct sub
  direct="$(bash "$NONE_ADAPTER" "$verb" 2>&1)"
  sub="$(bash "$NONE_ADAPTER" selftest "$verb" 2>&1)"
  set -e
  if [[ "$direct" == "$want" && "$sub" == "$want" ]]; then
    ok "$label"
  else
    bad "$label" "direct=$direct sub=$sub want=$want"
  fi
}

neutral run '{}' "N1 run returns the neutral map through both entry points"
neutral capabilities '{}' "N2 capabilities returns the neutral map"
neutral status '{"measured":false,"adapter":"none","reason":"no mutation adapter configured"}' \
  "N3 status reports unmeasured, distinguishable from measured zero"

# N4. ADVERSARIAL: the neutral adapter must never report a survival count. A
# `"survived":0` would be a positive claim that nothing survived.
set +e
run_out="$(bash "$NONE_ADAPTER" run 2>&1)"
set -e
if ! printf '%s' "$run_out" | grep -q 'survived\|killed\|mutants'; then
  ok "N4 the neutral run output carries no survival claim"
else
  bad "N4 neutral run leaks a survival claim" "out=$run_out"
fi

# --- U1. usage --------------------------------------------------------------
set +e
bash "$RESOLVE" --repo-root "$WORK/absent" >/dev/null 2>&1; u1=$?
bash "$RESOLVE" --bogus >/dev/null 2>&1; u2=$?
bash "$NONE_ADAPTER" wat >/dev/null 2>&1; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]]; then
  ok "U1 absent root, unknown option and unknown verb all exit 2"
else
  bad "U1 usage" "absent=$u1 opt=$u2 verb=$u3"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
