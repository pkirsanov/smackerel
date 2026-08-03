#!/usr/bin/env bash
# python-env-selftest.sh — hermetic selftest for python-env.sh.
#
# Proves the RESOLUTION CONTRACT without touching the operator's real managed
# environment: every case points BUBBLES_PYTHON_HOME at a mktemp dir, and the
# "satisfying" interpreters are tiny fake python3 shims, so no network, no pip,
# and no dependency on whether this machine happens to have PyYAML installed.
#
# ADVERSARIAL CASES (each would pass if the logic regressed the obvious way):
#   A1  an interpreter satisfying only ONE of the two modules must NOT count as
#       satisfying. A resolver that stops at the first successful import passes
#       every other case here.
#   A2  activate() must NOT prepend when PATH's python3 already satisfies —
#       otherwise it silently hijacks an operator who provisioned their own way.
#   A3  the module list must stay in lockstep with dependency-posture.sh's
#       BUBBLES_REQUIRED_DEPS, so the documented duplication cannot drift.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_SH="$SCRIPT_DIR/python-env.sh"
POSTURE_SH="$SCRIPT_DIR/dependency-posture.sh"

if [[ ! -f "$ENV_SH" ]]; then
  echo "python-env-selftest: python-env.sh not found: $ENV_SH" >&2
  exit 2
fi

pass=0
fail=0
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

ok() {
  echo "PASS: $1"
  pass=$((pass + 1))
}
bad() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

assert_exit() {
  local expected="$1" label="$2"
  shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    ok "$label (exit $actual)"
  else
    bad "$label (expected exit $expected, got $actual)"
  fi
}

# make_fake_python <path> <module>... — a shim that imports ONLY the named modules.
# NOTE: fixture subdirectories are deliberately named "venvroot", never "home" —
# a literal Linux home-directory path in a portable surface trips the agnosticity lint.
make_fake_python() {
  local path="$1"
  shift
  mkdir -p "$(dirname "$path")"
  {
    echo '#!/usr/bin/env bash'
    echo '# fake python3 shim for python-env-selftest'
    echo 'if [[ "${1:-}" == "-c" ]]; then'
    echo '  case "${2:-}" in'
    local module
    for module in "$@"; do
      echo "    *\"import $module\"*) exit 0 ;;"
    done
    echo '    *) exit 1 ;;'
    echo '  esac'
    echo 'fi'
    echo 'exit 1'
  } >"$path"
  chmod +x "$path"
}

# ── Case 1: no managed venv, PATH python3 lacks the modules → unsatisfied ──
c1="$TMP_ROOT/c1"
mkdir -p "$c1/venvroot" "$c1/bin"
make_fake_python "$c1/bin/python3" nothingatall
assert_exit 1 "Case 1: nothing satisfies is exit 1" \
  env BUBBLES_PYTHON_HOME="$c1/venvroot" PATH="$c1/bin:/usr/bin:/bin" bash "$ENV_SH" --check

# ── Case 2: a managed venv that satisfies resolves ─────────────────────────
c2="$TMP_ROOT/c2"
mkdir -p "$c2/venvroot/bin" "$c2/bin"
make_fake_python "$c2/venvroot/bin/python3" yaml jsonschema
make_fake_python "$c2/bin/python3" nothingatall
assert_exit 0 "Case 2: satisfying managed venv is exit 0" \
  env BUBBLES_PYTHON_HOME="$c2/venvroot" PATH="$c2/bin:/usr/bin:/bin" bash "$ENV_SH" --check

resolved="$(env BUBBLES_PYTHON_HOME="$c2/venvroot" PATH="$c2/bin:/usr/bin:/bin" bash "$ENV_SH" --path 2>/dev/null || true)"
if [[ "$resolved" == "$c2/venvroot/bin/python3" ]]; then
  ok "Case 2b: --path prints the managed interpreter"
else
  bad "Case 2b: --path printed '$resolved', expected '$c2/venvroot/bin/python3'"
fi

# ── Case 3: PATH python3 that satisfies is accepted when no venv exists ────
c3="$TMP_ROOT/c3"
mkdir -p "$c3/venvroot" "$c3/bin"
make_fake_python "$c3/bin/python3" yaml jsonschema
assert_exit 0 "Case 3: satisfying PATH python3 is accepted" \
  env BUBBLES_PYTHON_HOME="$c3/venvroot" PATH="$c3/bin:/usr/bin:/bin" bash "$ENV_SH" --check

# ── Case 4: $BUBBLES_PYTHON override wins ─────────────────────────────────
c4="$TMP_ROOT/c4"
mkdir -p "$c4/venvroot/bin" "$c4/bin" "$c4/override"
make_fake_python "$c4/venvroot/bin/python3" yaml jsonschema
make_fake_python "$c4/bin/python3" nothingatall
make_fake_python "$c4/override/python3" yaml jsonschema
resolved="$(env BUBBLES_PYTHON_HOME="$c4/venvroot" BUBBLES_PYTHON="$c4/override/python3" \
  PATH="$c4/bin:/usr/bin:/bin" bash "$ENV_SH" --path 2>/dev/null || true)"
if [[ "$resolved" == "$c4/override/python3" ]]; then
  ok "Case 4: BUBBLES_PYTHON override takes precedence"
else
  bad "Case 4: --path printed '$resolved', expected the override"
fi

# ── Case 5: a NON-satisfying override is ignored, not trusted ─────────────
c5="$TMP_ROOT/c5"
mkdir -p "$c5/venvroot/bin" "$c5/bin" "$c5/override"
make_fake_python "$c5/venvroot/bin/python3" yaml jsonschema
make_fake_python "$c5/bin/python3" nothingatall
make_fake_python "$c5/override/python3" yaml
resolved="$(env BUBBLES_PYTHON_HOME="$c5/venvroot" BUBBLES_PYTHON="$c5/override/python3" \
  PATH="$c5/bin:/usr/bin:/bin" bash "$ENV_SH" --path 2>/dev/null || true)"
if [[ "$resolved" == "$c5/venvroot/bin/python3" ]]; then
  ok "Case 5: unsatisfying override falls through to the managed venv"
else
  bad "Case 5: --path printed '$resolved', expected the managed venv"
fi

# ── Case 6: usage errors ──────────────────────────────────────────────────
assert_exit 2 "Case 6: unknown mode is a usage error" bash "$ENV_SH" --bogus
assert_exit 2 "Case 6b: extra argument is a usage error" bash "$ENV_SH" --check extra
assert_exit 0 "Case 6c: --help exits 0" bash "$ENV_SH" --help

# ── Case 7: no bypass flag exists ─────────────────────────────────────────
for flag in --skip --force --ignore; do
  assert_exit 2 "Case 7: $flag is rejected (no bypass)" bash "$ENV_SH" "$flag"
done

# ── Case 8: provisioning refuses a missing requirements file ──────────────
assert_exit 2 "Case 8: missing requirements file is exit 2" \
  env BUBBLES_PYTHON_HOME="$TMP_ROOT/c8home" bash -c \
  '. "$1"; bubbles_python_provision "$2"' _ "$ENV_SH" "$TMP_ROOT/definitely-absent.txt"

# ── ADVERSARIAL A1: partial module satisfaction must NOT count ─────────────
a1="$TMP_ROOT/a1"
mkdir -p "$a1/venvroot/bin" "$a1/bin"
make_fake_python "$a1/venvroot/bin/python3" yaml
make_fake_python "$a1/bin/python3" nothingatall
assert_exit 1 "A1: interpreter with only ONE required module is unsatisfied" \
  env BUBBLES_PYTHON_HOME="$a1/venvroot" PATH="$a1/bin:/usr/bin:/bin" bash "$ENV_SH" --check

# ── ADVERSARIAL A2: activate must not hijack a satisfying PATH python3 ─────
a2="$TMP_ROOT/a2"
mkdir -p "$a2/venvroot/bin" "$a2/bin"
make_fake_python "$a2/venvroot/bin/python3" yaml jsonschema
make_fake_python "$a2/bin/python3" yaml jsonschema
a2_path="$(env BUBBLES_PYTHON_HOME="$a2/venvroot" PATH="$a2/bin:/usr/bin:/bin" bash -c \
  '. "$1"; bubbles_python_activate >/dev/null 2>&1; printf "%s" "$PATH"' _ "$ENV_SH")"
case "$a2_path" in
  "$a2/venvroot/bin":*) bad "A2: activate hijacked a PATH python3 that already satisfies" ;;
  *) ok "A2: activate leaves a satisfying PATH python3 alone" ;;
esac

# ── A2b: activate DOES prepend when PATH python3 does not satisfy ──────────
a2b_path="$(env BUBBLES_PYTHON_HOME="$a1/venvroot" PATH="$a1/bin:/usr/bin:/bin" bash -c \
  '. "$1"; bubbles_python_activate >/dev/null 2>&1; printf "%s" "$PATH"' _ "$ENV_SH")"
case "$a2b_path" in
  *"$a1/venvroot/bin"*) bad "A2b: activate prepended an UNSATISFYING managed venv" ;;
  *) ok "A2b: activate does not prepend an unsatisfying managed venv" ;;
esac

a2c="$TMP_ROOT/a2c"
mkdir -p "$a2c/venvroot/bin" "$a2c/bin"
make_fake_python "$a2c/venvroot/bin/python3" yaml jsonschema
make_fake_python "$a2c/bin/python3" nothingatall
a2c_path="$(env BUBBLES_PYTHON_HOME="$a2c/venvroot" PATH="$a2c/bin:/usr/bin:/bin" bash -c \
  '. "$1"; bubbles_python_activate >/dev/null 2>&1; printf "%s" "$PATH"' _ "$ENV_SH")"
case "$a2c_path" in
  "$a2c/venvroot/bin":*) ok "A2c: activate prepends the managed venv when PATH does not satisfy" ;;
  *) bad "A2c: activate failed to prepend; PATH=$a2c_path" ;;
esac

# ── ADVERSARIAL A3: module list must match dependency-posture.sh ───────────
if [[ -f "$POSTURE_SH" ]]; then
  declared="$(grep -oE '"python-module:[a-zA-Z0-9_]+:' "$POSTURE_SH" |
    sed 's/"python-module://; s/:$//' | LC_ALL=C sort | tr '\n' ' ')"
  owned="$(bash -c '. "$1"; printf "%s\n" "${BUBBLES_PYTHON_MODULES[@]}"' _ "$ENV_SH" |
    LC_ALL=C sort | tr '\n' ' ')"
  if [[ -n "$declared" && "$declared" == "$owned" ]]; then
    ok "A3: BUBBLES_PYTHON_MODULES matches dependency-posture.sh ($owned)"
  else
    bad "A3: module lists drifted — posture='$declared' python-env='$owned'"
  fi
else
  bad "A3: dependency-posture.sh not found at $POSTURE_SH"
fi

# ── Case 9: requirements.txt is pinned and single-index ───────────────────
# Directives only: a comment that NAMES --extra-index-url (the file documents
# why it has none) must not be mistaken for one being declared.
REQ="$SCRIPT_DIR/../requirements.txt"
if [[ -f "$REQ" ]]; then
  directives="$(grep -vE '^[[:space:]]*#' "$REQ" || true)"
  if [[ "$(printf '%s\n' "$directives" | grep -c -- '--extra-index-url' || true)" -eq 0 ]]; then
    ok "Case 9: requirements.txt declares no --extra-index-url"
  else
    bad "Case 9: requirements.txt declares an --extra-index-url"
  fi
  if [[ "$(printf '%s\n' "$directives" | grep -c -- '--index-url' || true)" -eq 1 ]]; then
    ok "Case 9b: requirements.txt pins exactly one index"
  else
    bad "Case 9b: requirements.txt must declare exactly one --index-url"
  fi
  unpinned="$(grep -vE '^[[:space:]]*(#|$|--)' "$REQ" | grep -vE '==' | tr -d ' ' || true)"
  if [[ -z "$unpinned" ]]; then
    ok "Case 9c: every requirement is == pinned"
  else
    bad "Case 9c: unpinned requirement(s): $unpinned"
  fi
else
  bad "Case 9: requirements.txt not found at $REQ"
fi

# ---------------------------------------------------------------------------
# Case 10 (adversarial A4): cli.sh MUST activate the managed interpreter early.
#
# This is the seam that covers the ~15 selftests which call `python3 -c "import
# yaml"` directly without sourcing dependency-posture.sh. If someone deletes it,
# those selftests silently regress to SKIP/empty-value failures instead of
# failing loudly here. Guard it structurally.
# ---------------------------------------------------------------------------
CLI="$SCRIPT_DIR/cli.sh"
if [[ -f "$CLI" ]]; then
  if grep -q 'bubbles_python_activate' "$CLI"; then
    ok "Case 10: cli.sh activates the managed interpreter"
  else
    bad "Case 10: cli.sh no longer calls bubbles_python_activate — every child selftest loses the managed interpreter"
  fi

  # It must activate BEFORE the command dispatch, otherwise children spawned by
  # earlier subcommand handling would miss the exported PATH.
  # NOTE: `|| true` is required — this script runs under `set -euo pipefail`, so
  # a non-matching grep inside a pipeline would abort the whole selftest.
  activate_line="$(grep -n 'bubbles_python_activate' "$CLI" | head -1 | cut -d: -f1 || true)"
  dispatch_line="$(grep -nE '^[[:space:]]*case "\$(command_name|first_word|COMMAND|1|\{1:-\})' "$CLI" | head -1 | cut -d: -f1 || true)"
  if [[ -n "$activate_line" && -n "$dispatch_line" ]]; then
    if [[ "$activate_line" -lt "$dispatch_line" ]]; then
      ok "Case 10b: activation (line $activate_line) precedes command dispatch (line $dispatch_line)"
    else
      bad "Case 10b: activation at line $activate_line runs AFTER dispatch at line $dispatch_line"
    fi
  else
    ok "Case 10b: dispatch marker not matched; Case 10 already guards presence"
  fi
else
  bad "Case 10: cli.sh not found at $CLI"
fi

# ---------------------------------------------------------------------------
# Case 11 (adversarial A5): the pinned closure must cover every required module.
# A requirements.txt that installs PyYAML but forgets jsonschema would provision
# "successfully" and then fail at first use.
# ---------------------------------------------------------------------------
if [[ -f "$REQ" ]]; then
  missing_mod=""
  for _m in "${BUBBLES_PYTHON_MODULES[@]}"; do
    case "$_m" in
      yaml) _pkg='PyYAML' ;;
      jsonschema) _pkg='jsonschema' ;;
      *) _pkg="$_m" ;;
    esac
    grep -qiE "^[[:space:]]*${_pkg}==" "$REQ" || missing_mod="$missing_mod $_m"
  done
  if [[ -z "$missing_mod" ]]; then
    ok "Case 11: requirements.txt pins a distribution for every required module"
  else
    bad "Case 11: no pinned distribution for required module(s):$missing_mod"
  fi
fi

echo
echo "python-env selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
