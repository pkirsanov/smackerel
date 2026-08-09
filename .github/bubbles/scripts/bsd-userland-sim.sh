#!/usr/bin/env bash
# bsd-userland-sim.sh — BSD-userland (macOS) simulator for a GNU/Linux host.
#
# WHY THIS EXISTS
# ---------------
# `release-hygiene-macos` in .github/workflows/agnosticity.yml fails checks that
# are green on ubuntu, and the macOS job logs are not readable from a developer
# workstation. framework-validate.sh already carries a PATH shim that maps
# gsed -> sed and gtimeout -> timeout, but that shim only works in the
# macOS-to-GNU direction: it lets a Mac run GNU-shaped code. Nothing lets a Linux
# host run BSD-shaped userland, so a macOS-only failure could not be reproduced
# without a Mac.
#
# This simulator closes that direction. It builds a directory of shim executables
# that reject the GNU-only spelling of a flag and accept the portable or BSD
# spelling, plus an optional sealed PATH that hides the coreutils binaries stock
# macOS does not ship. Point PATH at it and a Linux host fails the way a Mac
# fails.
#
# It mirrors the approach framework-validate.sh already proved for the portable
# timeout fallback: force the platform-divergent branch on EVERY platform, so
# Linux continuous integration protects macOS.
#
# OPT-IN ONLY. Nothing here runs unless a caller invokes this script and assigns
# the result to PATH. A normal run is untouched.
#
# TWO ACTIVATION MODES, AND WHY BOTH EXIST
# ----------------------------------------
#   --prepend (default)  prints the shim DIRECTORY. Prepend it to PATH to get
#                        every behavioural shim in the divergence table below.
#   --sealed             prints a COMPLETE PATH value to ASSIGN (not prepend).
#                        Only this mode hides the absent-on-macOS binaries.
#
# A prepended directory cannot hide a binary, because the shell keeps searching
# later PATH elements until it finds an executable regular file. Making
# `command -v timeout` answer NO therefore requires replacing the PATH elements
# that carry the binary, which is what --sealed does: it mirrors every PATH
# directory holding a hidden name and leaves that name out of the mirror.
#
# SIMULATED DIVERGENCE TABLE
# --------------------------
#   sed       reject a bare -i with no suffix operand   accept -i '' and -i.bak
#   date      reject -d and --date                      accept -j -f, -v, +%s
#   stat      reject -c, --format, --printf             accept -f
#   readlink  reject -f, --canonicalize                 accept plain
#   grep      reject -P, --perl-regexp                  accept -E, -o
#   find      reject -printf                            accept -exec, -print
#   xargs     reject -r, --no-run-if-empty              accept plain
#   sort      reject -V, --version-sort                 accept plain
#   base64    reject -w, --wrap                         accept plain
#   mktemp    reject -p, --tmpdir, --suffix             accept -d, -t
#   head      reject a negative line count              accept -n N
#   cp        reject --parents                          accept plain
#   du        reject -b, --bytes                        accept -k
#
# HIDDEN (stock macOS ships none of these; --sealed makes them invisible):
#   timeout gtimeout sha256sum md5sum tac gsed ggrep gdate gstat
#
# PROVIDED (macOS ships these and a Linux host may not):
#   shasum md5
#
# ACCEPTED SPELLINGS ARE TRANSLATED, NOT MERELY WAVED THROUGH
# -----------------------------------------------------------
# A shim that only rejected the GNU spelling would make the CORRECT BSD branch
# fail too, because the binary underneath is still GNU. That would produce false
# attributions, which is worse than no simulator. So an accepted BSD spelling is
# rewritten into the GNU form before the real tool runs:
#   sed     -i '' and -i .bak      folded into the attached -i<suffix> form
#   date    -j -f <fmt> <value>    the VALUE is handed to GNU to parse
#   date    -r <seconds>           rewritten as the GNU epoch spelling
#   date    -v<adjustment>         rewritten as a GNU relative phrase
#   stat    -f <format>            field selectors mapped to the GNU spelling
#   mktemp  -t <prefix>            expanded to the TMPDIR template GNU wants
# Two limits are deliberate and visible rather than silent. The BSD input format
# is checked for presence but not replayed, so a format GNU cannot infer from the
# value fails; the ISO-shaped formats this framework uses are covered. The stat
# selector map covers the fields the framework reads, and an unmapped selector
# passes through for GNU to refuse.
#
# TWO DELIBERATE DEVIATIONS FROM A NAIVE "REJECT EVERY GNU FORM" RULE
# -------------------------------------------------------------------
# 1. A date format containing the nanosecond specifier is NOT rejected. BSD date
#    emits the specifier letter literally instead of failing, which is the exact
#    behaviour guard-lib.sh documents and numerically guards against. The shim
#    reproduces that literal output, so the caller sees the same corrupt value a
#    Mac produces. Rejecting instead would model a failure macOS does not have
#    and would hide the one it does.
# 2. Every simulated rejection exits 1, except grep and sort which exit 2. The
#    exact per-tool BSD exit number is not asserted, because no macOS host was
#    available to confirm it and inventing one would be a fabricated fact. What
#    is asserted is that the command FAILS, which is what a caller under
#    `set -e` or an `if` reacts to. grep and sort are the exception because grep
#    reserves exit 1 for "no match", so a rejection exiting 1 would be read as a
#    normal empty result and silently swallowed.
#
# EXIT CODES
#   0  success
#   2  usage error
#
# CLEANUP CONTRACT
#   bsd-userland-sim.sh --cleanup <value>
# accepts the shim directory, the simulation root, or a sealed PATH string, and
# removes the simulation root. It refuses any path that does not carry this
# simulator's marker file, so a mistyped argument cannot delete an unrelated tree.
#
# Self-contained: sources nothing, so it stays copyable and testable on its own.

set -euo pipefail

MARKER_NAME=".bubbles-bsd-userland-sim"

# Names stock macOS does not ship. `gtimeout` sits on this line so the framework
# portability guard's own raw-timeout class treats the line as helper-aware.
HIDDEN_TOOLS="timeout gtimeout sha256sum md5sum tac gsed ggrep gdate gstat" # portable-ok: data list, not an invocation

# Names whose flag handling diverges between GNU coreutils and BSD userland.
SHIMMED_TOOLS="sed date stat readlink grep find xargs sort base64 mktemp head cp du"

usage() {
  cat <<'EOF'
Usage: bsd-userland-sim.sh [--prepend | --sealed] [--root DIR]
       bsd-userland-sim.sh --cleanup <shim-dir | root | sealed-PATH>
       bsd-userland-sim.sh --help

Build a BSD-userland (macOS) simulation on a GNU/Linux host and print the value
the caller should put on PATH.

Modes:
  --prepend   (default) print the shim DIRECTORY; prepend it to PATH. Activates
              every behavioural shim. Does NOT hide the absent-on-macOS binaries.
  --sealed    print a COMPLETE PATH value to ASSIGN. Additionally hides the
              binaries stock macOS does not ship, so `command -v timeout` fails
              the way it fails on a Mac.
  --root DIR  build into DIR instead of a fresh temporary directory.
  --cleanup   remove a simulation built by an earlier call.

Typical use:
  SIM_PATH="$(bash bsd-userland-sim.sh --sealed)"
  trap 'bash bsd-userland-sim.sh --cleanup "$SIM_PATH"' EXIT
  PATH="$SIM_PATH" bash some-selftest.sh

Exit codes:
  0  success
  2  usage error
EOF
}

die_usage() {
  printf '[bsd-userland-sim][USAGE] %s\n' "$1" >&2
  usage >&2
  exit 2
}

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
mode="prepend"
root=""
cleanup_target=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --prepend) mode="prepend" ;;
    --sealed) mode="sealed" ;;
    --root)
      [[ $# -ge 2 ]] || die_usage "--root requires a directory argument"
      root="$2"
      shift
      ;;
    --cleanup)
      [[ $# -ge 2 ]] || die_usage "--cleanup requires a path or PATH value"
      mode="cleanup"
      cleanup_target="$2"
      shift
      ;;
    *) die_usage "unknown argument: $1" ;;
  esac
  shift
done

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
if [[ "$mode" == "cleanup" ]]; then
  # A sealed PATH is colon-separated and its first element is the shim directory.
  first="${cleanup_target%%:*}"
  [[ -n "$first" ]] || die_usage "--cleanup was given an empty value"

  candidate="$first"
  [[ "$(basename "$candidate")" == "bin" ]] && candidate="$(dirname "$candidate")"

  if [[ ! -f "$candidate/$MARKER_NAME" ]]; then
    printf '[bsd-userland-sim][USAGE] refusing to remove %s: no %s marker.\n' \
      "$candidate" "$MARKER_NAME" >&2
    exit 2
  fi
  rm -rf "$candidate"
  exit 0
fi

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
if [[ -z "$root" ]]; then
  root="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-bsd-sim.XXXXXX")"
else
  mkdir -p "$root"
fi
root="$(cd "$root" && pwd -P)"

bindir="$root/bin"
realdir="$root/real"
mkdir -p "$bindir" "$realdir"
printf 'bubbles bsd-userland simulation root\n' >"$root/$MARKER_NAME"

# resolve_real <name> — absolute path of the genuine tool on the INHERITED PATH,
# skipping anything inside this simulation. Prints nothing when absent.
resolve_real() {
  local name="$1" dir
  local IFS=':'
  for dir in $PATH; do
    [[ -n "$dir" ]] || continue
    case "$dir" in "$root" | "$root"/*) continue ;; esac
    if [[ -f "$dir/$name" && -x "$dir/$name" ]]; then
      printf '%s' "$dir/$name"
      return 0
    fi
  done
  return 1
}

# ---------------------------------------------------------------------------
# The dispatcher. One script; every shimmed tool is a link to it, and it decides
# which divergence rule to apply from its own invocation name. Keeping the rules
# in one file is what makes them reviewable side by side.
# ---------------------------------------------------------------------------
dispatcher="$bindir/.bsd-shim"
cat >"$dispatcher" <<'DISPATCHER_EOF'
#!/usr/bin/env bash
# Generated by bsd-userland-sim.sh. Do not edit; regenerate instead.
#
# `set -e` is deliberately absent: this process must return the real tool's exit
# status untouched, and an early abort would replace it.
set -uo pipefail

tool="$(basename "$0")"
shim_dir="$(cd "$(dirname "$0")" && pwd -P)"
sim_root="$(dirname "$shim_dir")"
real_file="$sim_root/real/$tool"

# reject <message> <exit-code> — emulate a BSD tool refusing a GNU-only flag.
reject() {
  printf '%s\n' "$1" >&2
  exit "$2"
}

# is_short_cluster <arg> <letter> — true when arg is a single-dash short-option
# cluster containing letter. `--long` forms never match.
is_short_cluster() {
  local arg="$1" letter="$2"
  case "$arg" in
    --*) return 1 ;;
    -*) [[ "$arg" == *"$letter"* ]] && return 0 ;;
  esac
  return 1
}

# translate_stat_format <bsd-format> — rewrite the BSD field selectors this
# framework actually uses into their GNU equivalents. Unknown selectors pass
# through so GNU can reject them itself rather than being silently dropped.
translate_stat_format() {
  local in="$1" out="" idx=0 len="${#1}" nxt three
  while [[ $idx -lt $len ]]; do
    if [[ "${in:$idx:1}" != "%" ]]; then
      out="$out${in:$idx:1}"
      idx=$((idx + 1))
      continue
    fi
    three="${in:$idx:3}"
    case "$three" in
      '%Su')
        out="$out%U"
        idx=$((idx + 3))
        continue
        ;;
      '%Sg')
        out="$out%G"
        idx=$((idx + 3))
        continue
        ;;
      '%Sm')
        out="$out%y"
        idx=$((idx + 3))
        continue
        ;;
      '%Sp')
        out="$out%A"
        idx=$((idx + 3))
        continue
        ;;
      '%Op' | '%Lp')
        # BSD spells octal permission bits with an interpretation modifier.
        out="$out%a"
        idx=$((idx + 3))
        continue
        ;;
    esac
    nxt="${in:$((idx + 1)):1}"
    case "$nxt" in
      m) out="$out%Y" ;;
      a) out="$out%X" ;;
      c) out="$out%Z" ;;
      z) out="$out%s" ;;
      N) out="$out%n" ;;
      *) out="$out%$nxt" ;;
    esac
    idx=$((idx + 2))
  done
  printf '%s' "$out"
}

# translate_date_shift <bsd-adjustment> — rewrite a BSD relative adjustment such
# as -7d into the GNU relative phrase "-7 days". Prints nothing on an unknown
# unit so the caller can refuse.
translate_date_shift() {
  local adj="$1" sign num unit
  sign="${adj:0:1}"
  case "$sign" in
    + | -) adj="${adj:1}" ;;
    *) sign="+" ;;
  esac
  num="${adj%%[A-Za-z]*}"
  unit="${adj##*[0-9]}"
  [[ "$num" =~ ^[0-9]+$ ]] || return 1
  case "$unit" in
    y) unit="years" ;;
    m) unit="months" ;;
    w) unit="weeks" ;;
    d) unit="days" ;;
    H) unit="hours" ;;
    M) unit="minutes" ;;
    S) unit="seconds" ;;
    *) return 1 ;;
  esac
  printf '%s%s %s' "$sign" "$num" "$unit"
}

case "$tool" in
  sed)
    # BSD consumes the argument after a bare -i as the backup suffix, so the GNU
    # spelling silently eats the program. Accept an attached suffix, an empty
    # suffix, or a plain dot-suffix operand; refuse anything else. An accepted
    # BSD spelling is then folded into the attached form GNU understands, so the
    # correct branch keeps working under simulation.
    sed_args=()
    idx=1
    while [[ $idx -le $# ]]; do
      arg="${!idx}"
      if [[ "$arg" == "--" ]]; then
        while [[ $idx -le $# ]]; do
          sed_args+=("${!idx}")
          idx=$((idx + 1))
        done
        break
      fi
      if [[ "$arg" == "-i" ]]; then
        if [[ $idx -ge $# ]]; then
          reject "sed: [bsd-userland-sim] BSD requires an explicit suffix operand after the in-place flag" 1
        fi
        nxt=$((idx + 1))
        suffix="${!nxt}"
        if [[ -n "$suffix" && ! "$suffix" =~ ^\.[A-Za-z0-9._-]*$ ]]; then
          reject "sed: [bsd-userland-sim] BSD requires an explicit suffix operand after the in-place flag; the GNU form consumes the program as the suffix" 1
        fi
        sed_args+=("-i$suffix")
        idx=$((idx + 2))
        continue
      fi
      sed_args+=("$arg")
      idx=$((idx + 1))
    done
    set -- ${sed_args[@]+"${sed_args[@]}"}
    ;;
  date)
    date_args=()
    date_spec=""
    date_shift=""
    for arg in "$@"; do
      case "$arg" in
        -d | --date | --date=*)
          reject "date: [bsd-userland-sim] illegal option; BSD parses timestamps with -j -f and shifts them with -v" 1
          ;;
      esac
    done
    idx=1
    while [[ $idx -le $# ]]; do
      arg="${!idx}"
      case "$arg" in
        -j)
          # "do not set the clock". GNU never sets it from a format read, so the
          # flag has no counterpart and is simply consumed.
          idx=$((idx + 1))
          continue
          ;;
        -f)
          nxt=$((idx + 1))
          fmt_arg="${!nxt:-}"
          str_idx=$((idx + 2))
          [[ -n "$fmt_arg" && $str_idx -le $# ]] ||
            reject "date: [bsd-userland-sim] the BSD parse flag needs an input format and a value" 1
          # The input format is validated as present but not replayed: GNU is
          # asked to parse the VALUE directly, which covers the ISO-shaped
          # formats this framework uses. An exotic format GNU cannot infer will
          # fail here, and that limitation is intentional rather than silent.
          date_spec="${!str_idx}"
          idx=$((idx + 3))
          continue
          ;;
        -r)
          nxt=$((idx + 1))
          [[ $nxt -le $# ]] ||
            reject "date: [bsd-userland-sim] the BSD epoch flag needs a seconds operand" 1
          [[ "${!nxt}" =~ ^[0-9]+$ ]] ||
            reject "date: [bsd-userland-sim] the BSD epoch flag takes seconds, not a file" 1
          date_spec="@${!nxt}"
          idx=$((idx + 2))
          continue
          ;;
        -v*)
          shift_phrase="$(translate_date_shift "${arg#-v}")" ||
            reject "date: [bsd-userland-sim] unsupported adjustment unit in ${arg}" 1
          date_shift="$date_shift $shift_phrase"
          idx=$((idx + 1))
          continue
          ;;
        +*)
          # BSD has no nanosecond selector and emits the letter literally.
          date_args+=("${arg//%N/N}")
          idx=$((idx + 1))
          continue
          ;;
        *)
          date_args+=("$arg")
          idx=$((idx + 1))
          ;;
      esac
    done
    if [[ -n "$date_spec" || -n "$date_shift" ]]; then
      [[ -n "$date_spec" ]] || date_spec="now"
      set -- -d "${date_spec}${date_shift}" ${date_args[@]+"${date_args[@]}"}
    else
      set -- ${date_args[@]+"${date_args[@]}"}
    fi
    ;;
  stat)
    stat_args=()
    idx=1
    while [[ $idx -le $# ]]; do
      arg="${!idx}"
      if [[ "$arg" == "--" ]]; then
        while [[ $idx -le $# ]]; do
          stat_args+=("${!idx}")
          idx=$((idx + 1))
        done
        break
      fi
      case "$arg" in
        --format | --format=* | --printf | --printf=*)
          reject "stat: [bsd-userland-sim] illegal option; BSD selects fields with -f" 1
          ;;
        -f)
          nxt=$((idx + 1))
          [[ $nxt -le $# ]] ||
            reject "stat: [bsd-userland-sim] the BSD field flag needs a format operand" 1
          stat_args+=("-c" "$(translate_stat_format "${!nxt}")")
          idx=$((idx + 2))
          continue
          ;;
        -f?*)
          stat_args+=("-c" "$(translate_stat_format "${arg#-f}")")
          idx=$((idx + 1))
          continue
          ;;
      esac
      if is_short_cluster "$arg" c; then
        reject "stat: [bsd-userland-sim] illegal option -- c; BSD selects fields with -f" 1
      fi
      stat_args+=("$arg")
      idx=$((idx + 1))
    done
    set -- ${stat_args[@]+"${stat_args[@]}"}
    ;;
  readlink)
    for arg in "$@"; do
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        --canonicalize | --canonicalize-*)
          reject "readlink: [bsd-userland-sim] illegal option; stock BSD cannot canonicalize" 1
          ;;
      esac
      if is_short_cluster "$arg" f; then
        reject "readlink: [bsd-userland-sim] illegal option -- f; stock BSD cannot canonicalize" 1
      fi
    done
    ;;
  grep)
    # Exit 2, not 1: grep reserves 1 for "no match", so a rejection exiting 1
    # would be read as a normal empty result and swallowed.
    skip_next=0
    for arg in "$@"; do
      if [[ $skip_next -eq 1 ]]; then
        skip_next=0
        continue
      fi
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        -e | -f | -m | --regexp | --file)
          skip_next=1
          continue
          ;;
        --perl-regexp)
          reject "grep: [bsd-userland-sim] unknown option; BSD has no Perl-regexp engine" 2
          ;;
      esac
      if is_short_cluster "$arg" P; then
        reject "grep: [bsd-userland-sim] unknown option -- P; BSD has no Perl-regexp engine" 2
      fi
    done
    ;;
  find)
    for arg in "$@"; do
      if [[ "$arg" == "-printf" ]]; then
        reject "find: [bsd-userland-sim] -printf: unknown primary or operator" 1
      fi
    done
    ;;
  xargs)
    # Scan only the option run. The first non-option argument is the command,
    # and its own flags belong to it, not to xargs.
    skip_next=0
    for arg in "$@"; do
      if [[ $skip_next -eq 1 ]]; then
        skip_next=0
        continue
      fi
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        -[nPILsE])
          skip_next=1
          continue
          ;;
        --no-run-if-empty)
          reject "xargs: [bsd-userland-sim] illegal option; BSD has no empty-input guard flag" 1
          ;;
        -*) ;;
        *) break ;;
      esac
      if is_short_cluster "$arg" r; then
        reject "xargs: [bsd-userland-sim] illegal option -- r; BSD has no empty-input guard flag" 1
      fi
    done
    ;;
  sort)
    # Exit 2 to match sort's own error convention.
    for arg in "$@"; do
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        --version-sort)
          reject "sort: [bsd-userland-sim] unknown option; BSD has no version-ordered collation" 2
          ;;
      esac
      if is_short_cluster "$arg" V; then
        reject "sort: [bsd-userland-sim] unknown option -- V; BSD has no version-ordered collation" 2
      fi
    done
    ;;
  base64)
    for arg in "$@"; do
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        --wrap | --wrap=*)
          reject "base64: [bsd-userland-sim] invalid option; BSD spells the line-break control -b" 1
          ;;
        -w | -w*)
          reject "base64: [bsd-userland-sim] invalid option -- w; BSD spells the line-break control -b" 1
          ;;
      esac
    done
    ;;
  mktemp)
    mktemp_args=()
    mktemp_prefix=""
    idx=1
    while [[ $idx -le $# ]]; do
      arg="${!idx}"
      case "$arg" in
        -p | -p* | --tmpdir | --tmpdir=*)
          reject "mktemp: [bsd-userland-sim] illegal option; BSD takes the directory from TMPDIR or a template" 1
          ;;
        --suffix | --suffix=*)
          reject "mktemp: [bsd-userland-sim] illegal option; BSD has no suffix flag" 1
          ;;
        -t)
          nxt=$((idx + 1))
          [[ $nxt -le $# ]] ||
            reject "mktemp: [bsd-userland-sim] the BSD prefix flag needs an operand" 1
          mktemp_prefix="${!nxt}"
          idx=$((idx + 2))
          continue
          ;;
        *)
          mktemp_args+=("$arg")
          idx=$((idx + 1))
          ;;
      esac
    done
    if [[ -n "$mktemp_prefix" ]]; then
      # BSD turns a prefix into TMPDIR/prefix.XXXXXXXX; GNU wants that template
      # spelled out.
      mktemp_args+=("${TMPDIR:-/tmp}/${mktemp_prefix}.XXXXXXXX")
    fi
    set -- ${mktemp_args[@]+"${mktemp_args[@]}"}
    ;;
  head)
    # A negative line count means "all but the last N" in GNU and is not a BSD
    # concept. Catch the separated, attached, and long spellings.
    idx=1
    while [[ $idx -le $# ]]; do
      arg="${!idx}"
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        -n)
          nxt=$((idx + 1))
          if [[ $nxt -le $# && "${!nxt}" =~ ^-[0-9]+$ ]]; then
            reject "head: [bsd-userland-sim] illegal line count; BSD has no all-but-last form" 1
          fi
          idx=$nxt
          ;;
        -n-[0-9]* | --lines=-[0-9]*)
          reject "head: [bsd-userland-sim] illegal line count; BSD has no all-but-last form" 1
          ;;
      esac
      idx=$((idx + 1))
    done
    ;;
  cp)
    for arg in "$@"; do
      [[ "$arg" == "--" ]] && break
      if [[ "$arg" == "--parents" ]]; then
        reject "cp: [bsd-userland-sim] illegal option; BSD has no path-preserving copy flag" 1
      fi
    done
    ;;
  du)
    for arg in "$@"; do
      [[ "$arg" == "--" ]] && break
      case "$arg" in
        --bytes)
          reject "du: [bsd-userland-sim] illegal option; BSD reports in blocks, so use -k" 1
          ;;
      esac
      if is_short_cluster "$arg" b; then
        reject "du: [bsd-userland-sim] illegal option -- b; BSD reports in blocks, so use -k" 1
      fi
    done
    ;;
esac

if [[ ! -f "$real_file" ]]; then
  printf '%s: [bsd-userland-sim] no real implementation recorded for this tool\n' "$tool" >&2
  exit 127
fi
real="$(<"$real_file")"
if [[ -z "$real" || ! -x "$real" ]]; then
  printf '%s: [bsd-userland-sim] recorded implementation is not executable\n' "$tool" >&2
  exit 127
fi
exec "$real" "$@"
DISPATCHER_EOF
chmod +x "$dispatcher"

for tool in $SHIMMED_TOOLS; do
  if real_path="$(resolve_real "$tool")"; then
    printf '%s' "$real_path" >"$realdir/$tool"
    ln -sf ".bsd-shim" "$bindir/$tool"
  fi
done

# ---------------------------------------------------------------------------
# macOS-only equivalents. Providing these keeps the simulation faithful rather
# than merely destructive: a script that legitimately reaches for the macOS
# spelling still works.
# ---------------------------------------------------------------------------
if ! resolve_real shasum >/dev/null 2>&1; then
  sha1_real="$(resolve_real sha1sum || true)"
  sha256_real="$(resolve_real sha256sum || true)"
  if [[ -n "$sha1_real" || -n "$sha256_real" ]]; then
    cat >"$bindir/shasum" <<SHASUM_EOF
#!/usr/bin/env bash
# Generated by bsd-userland-sim.sh: macOS shasum over GNU digest binaries.
set -uo pipefail
algo=1
args=()
while [[ \$# -gt 0 ]]; do
  case "\$1" in
    -a)
      algo="\${2:-1}"
      shift 2
      ;;
    -a*)
      algo="\${1#-a}"
      shift
      ;;
    *)
      args+=("\$1")
      shift
      ;;
  esac
done
case "\$algo" in
  1) impl="$sha1_real" ;;
  256) impl="$sha256_real" ;;
  *)
    printf 'shasum: [bsd-userland-sim] unsupported algorithm %s\n' "\$algo" >&2
    exit 1
    ;;
esac
if [[ -z "\$impl" ]]; then
  printf 'shasum: [bsd-userland-sim] no digest implementation for algorithm %s\n' "\$algo" >&2
  exit 1
fi
exec "\$impl" \${args[@]+"\${args[@]}"}
SHASUM_EOF
    chmod +x "$bindir/shasum"
  fi
fi

if ! resolve_real md5 >/dev/null 2>&1; then
  md5_real="$(resolve_real md5sum || true)"
  if [[ -n "$md5_real" ]]; then
    cat >"$bindir/md5" <<MD5_EOF
#!/usr/bin/env bash
# Generated by bsd-userland-sim.sh: macOS md5 output shape over GNU md5sum.
set -uo pipefail
if [[ \$# -eq 0 ]]; then
  "$md5_real" | { read -r digest _rest; printf '%s\n' "\$digest"; }
  exit \${PIPESTATUS[0]}
fi
status=0
for f in "\$@"; do
  if ! digest="\$("$md5_real" "\$f")"; then
    status=1
    continue
  fi
  printf 'MD5 (%s) = %s\n' "\$f" "\${digest%% *}"
done
exit "\$status"
MD5_EOF
    chmod +x "$bindir/md5"
  fi
fi

# ---------------------------------------------------------------------------
# Emit the PATH value.
# ---------------------------------------------------------------------------
if [[ "$mode" == "prepend" ]]; then
  printf '%s\n' "$bindir"
  exit 0
fi

# Sealed mode. Mirror only the PATH directories that actually carry a hidden
# name, and leave that name out of the mirror. Directories with no hidden name
# pass through untouched, which keeps the cost proportional to the problem
# rather than to the size of PATH.
mirror_index=0
sealed="$bindir"
seen_dirs=""
while IFS= read -r dir; do
  [[ -n "$dir" ]] || continue
  case "$dir" in "$root" | "$root"/*) continue ;; esac
  [[ -d "$dir" ]] || continue
  case ":$seen_dirs:" in *":$dir:"*) continue ;; esac
  seen_dirs="$seen_dirs:$dir"

  carries_hidden=0
  for tool in $HIDDEN_TOOLS; do
    if [[ -f "$dir/$tool" && -x "$dir/$tool" ]]; then
      carries_hidden=1
      break
    fi
  done

  if [[ "$carries_hidden" -eq 0 ]]; then
    sealed="$sealed:$dir"
    continue
  fi

  mirror_index=$((mirror_index + 1))
  mirror="$root/mirror/$mirror_index"
  mkdir -p "$mirror"
  # One exec for the whole directory; a per-entry loop over a large bin
  # directory measured two orders of magnitude slower.
  ln -s "$dir"/* "$mirror/" 2>/dev/null || true
  for tool in $HIDDEN_TOOLS; do
    rm -f "$mirror/$tool"
  done
  sealed="$sealed:$mirror"
done < <(printf '%s\n' "$PATH" | tr ':' '\n')

printf '%s\n' "$sealed"
