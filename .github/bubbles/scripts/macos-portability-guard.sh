#!/usr/bin/env bash
# macos-portability-guard.sh — reusable GNU/BSD (Linux + macOS) portability lint.
#
# Fails (exit 1) if any GNU-coreutils / bash-4.x-only shell construct that breaks
# on macOS (bash 3.2 + BSD userland) appears in the CALLER-SUPPLIED script
# surface. This is the canonical mechanical enforcement of
#   instructions/wsl-macos-compatibility.instructions.md
# and the "Pitfall -> Portable Form" table in
#   skills/bubbles-cross-platform-shell/SKILL.md
#
# REUSABLE BY DESIGN — this tool takes the scan surface as an argument (files
# and/or directories) and/or the PORTABILITY_SCAN_PATHS env var. It has NO
# default surface and MUST NOT be pointed at the framework's own bubbles/scripts/
# (those intentionally use raw timeout/sed -i/date -d, mediated by the guard-lib
# helpers + the framework-validate PATH shim). framework-validate runs this
# tool's SELFTEST, never a scan of the framework's own scripts. Downstream repos
# wire it into their pre-push against their OWN operator script surface.
#
# Helper-aware: a line routed through a portable helper (bubbles_run_with_timeout,
# bubbles_sed_inplace, bubbles_iso_to_epoch, bubbles_file_mtime_epoch,
# bubbles_now_ms, run_with_timeout, gtimeout, command -v timeout) or already
# carrying a BSD fallback (|| date -j, date -v, stat -f) is NOT a violation.
#
# Inline allowlist: a line carrying "# portable-ok:<reason>" (or that pragma on
# the line immediately above) is exempt — for genuinely intentional raw usage
# (e.g. a Docker-internal entrypoint, curl --connect-timeout). Full-line comments
# are stripped before scanning so explanatory prose never trips the guard.
#
# Portable BY DESIGN: runs unchanged on macOS bash 3.2 + BSD coreutils and on
# Linux/WSL. It uses no GNU-only construct itself — the class-detection patterns
# are written with [[:space:]] / alternation so this file's own source is clean
# when scanned by itself (proven by the selftest running the guard on the guard).
#
# Exit codes:
#   0  surface is WSL+macOS portable (or empty surface of real files)
#   1  one or more portability violation class(es) found (listed on stdout)
#   2  usage error (no surface given / -h / bad path)
#
# There is NO bypass flag. Fix the construct (use the portable helper/form) or,
# for a genuinely intentional raw usage, annotate the line with # portable-ok:.
#
# Self-contained (does NOT source guard-lib.sh) so it can be copied verbatim into
# a downstream repo's lint surface. Origin: bubbles-cross-platform-shell.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: macos-portability-guard.sh [PATH ...]

Scan the given shell-script surface (files and/or directories) for GNU/bash-4.x
constructs that break on macOS (bash 3.2 + BSD userland) and fail loud if any
are found. Directories are searched for *.sh files; explicit file arguments are
scanned as-is regardless of extension.

Surface resolution (at least one path is REQUIRED):
  1. PATH arguments on the command line, OR
  2. the PORTABILITY_SCAN_PATHS env var (whitespace-separated paths).
There is NO built-in default surface.

Options:
  -h, --help   Show this help and exit 0.

Exit codes:
  0  surface is WSL+macOS portable
  1  portability violation(s) found
  2  usage error (no surface / bad path)

Inline exemption:  add  # portable-ok:<reason>  to a line (or the line above it)
for a genuinely intentional raw usage. There is no other bypass.
EOF
}

# ---------------------------------------------------------------------------
# Argument / surface resolution
# ---------------------------------------------------------------------------
scan_args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do
        scan_args+=("$1")
        shift
      done
      ;;
    -*)
      echo "[macos-portability-guard][USAGE] unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      scan_args+=("$1")
      shift
      ;;
  esac
done

# Fall back to PORTABILITY_SCAN_PATHS (whitespace-separated) when no path args.
if [[ "${#scan_args[@]}" -eq 0 && -n "${PORTABILITY_SCAN_PATHS:-}" ]]; then
  # Word-split the env var on whitespace (intentional; paths with spaces are not
  # supported via the env var — pass them as explicit arguments instead).
  # shellcheck disable=SC2206
  scan_args=(${PORTABILITY_SCAN_PATHS})
fi

if [[ "${#scan_args[@]}" -eq 0 ]]; then
  echo "[macos-portability-guard][USAGE] no scan surface given." >&2
  echo "  pass one or more files/dirs, or set PORTABILITY_SCAN_PATHS." >&2
  usage >&2
  exit 2
fi

# Expand the surface into a concrete .sh file list. A file argument is taken
# verbatim; a directory argument contributes its *.sh files (recursively).
targets=()
for p in "${scan_args[@]}"; do
  if [[ -f "$p" ]]; then
    targets+=("$p")
  elif [[ -d "$p" ]]; then
    while IFS= read -r f; do
      [[ -n "$f" ]] && targets+=("$f")
    done < <(find "$p" -type f -name '*.sh' 2>/dev/null | LC_ALL=C sort)
  else
    echo "[macos-portability-guard][USAGE] path not found: $p" >&2
    exit 2
  fi
done

if [[ "${#targets[@]}" -eq 0 ]]; then
  echo "[macos-portability-guard] no *.sh files in the given surface; nothing to scan."
  exit 0
fi

# ---------------------------------------------------------------------------
# Preprocess: emit "file:line:code" for every SCANNABLE line, i.e. drop full-line
# comments and lines exempted by a # portable-ok: pragma (same line, or on the
# line immediately above). Portable POSIX awk (printf / ~ / string ops only).
# ---------------------------------------------------------------------------
build_scan_stream() {
  local f
  for f in "${targets[@]}"; do
    awk -v fname="$f" '
      function is_pragma(s) { return (s ~ /#[[:space:]]*portable-ok:/) }
      {
        line = $0
        full_comment = (line ~ /^[[:space:]]*#/)
        pragma_here  = is_pragma(line)
        if (full_comment) {
          # a full-line pragma comment exempts the NEXT code line
          prev_pragma = (pragma_here ? 1 : 0)
          next
        }
        if (pragma_here) {          # trailing pragma exempts THIS line only
          prev_pragma = 0
          next
        }
        if (prev_pragma) {          # exempted by the pragma on the line above
          prev_pragma = 0
          next
        }
        printf "%s:%d:%s\n", fname, FNR, line
        prev_pragma = 0
      }
    ' "$f"
  done
}

SCAN_STREAM="$(build_scan_stream)"

# ---------------------------------------------------------------------------
# Class detection. Each pattern names the offending construct with [[:space:]] /
# alternation so THIS file's own source never self-matches (verified by the
# selftest running the guard on the guard). scan_class prints the offending
# "file:line:code" rows for one class, minus any helper-routed / BSD-fallback
# rows the class exempts.
# ---------------------------------------------------------------------------
scan_class() {
  local pat="$1" skip="${2:-}" out
  out="$(printf '%s\n' "$SCAN_STREAM" | grep -E "$pat" 2>/dev/null || true)"
  if [[ -n "$skip" && -n "$out" ]]; then
    out="$(printf '%s\n' "$out" | grep -vE "$skip" 2>/dev/null || true)"
  fi
  printf '%s' "$out"
}

violations=0
report() {
  local label="$1" hits="$2" remedy="$3"
  if [[ -n "$hits" ]]; then
    echo "FAIL macOS-portability violation -- $label"
    while IFS= read -r _row; do
      [[ -n "$_row" ]] && echo "   $_row"
    done <<< "$hits"
    echo "   remedy: $remedy"
    violations=$((violations + 1))
  else
    echo "ok   $label: none"
  fi
}

echo "== macOS portability guard -- scanning ${#targets[@]} file(s) =="

# 1. raw timeout(1) — absent on macOS by default (only gtimeout, if installed).
report "class-1 raw-timeout" \
  "$(scan_class '(^|[^_./[:alnum:]-])timeout[[:space:]]' \
      'command -v timeout|run_with_timeout|gtimeout|bubbles_run_with_timeout|TIMEOUT_BIN')" \
  "route through bubbles_run_with_timeout (guard-lib.sh); preserve exit 124"

# 2. in-place sed(2) — GNU 'sed -i prog' vs BSD 'sed -i "" prog' are incompatible.
report "class-2 in-place-sed" \
  "$(scan_class '(^|[^_[:alnum:]-])sed[[:space:]]+-i' \
      'bubbles_sed_inplace')" \
  "use bubbles_sed_inplace (temp-file rewrite)"

# 3. date(3) -d — GNU-only relative/parse; BSD date uses -j -f / -v.
report "class-3 date-d-parse" \
  "$(scan_class '(^|[^_[:alnum:]-])date[[:space:]]+-d' \
      'bubbles_iso_to_epoch|date[[:space:]]+-v|date[[:space:]]+-j')" \
  "use bubbles_iso_to_epoch, or add a BSD fallback ( ... || date -j / date -v )"

# 4. stat(3) -c — GNU format flag; BSD stat uses -f.
report "class-4 stat-c-mtime" \
  "$(scan_class '(^|[^_[:alnum:]-])stat[[:space:]]+-c' \
      'bubbles_file_mtime_epoch|stat[[:space:]]+-f')" \
  "use bubbles_file_mtime_epoch (GNU/BSD stat mtime helper)"

# 5. readlink -f used to absolutize — BSD readlink -f canonicalizes symlinks.
report "class-5 readlink-f-absolutize" \
  "$(scan_class 'readlink[[:space:]]+-f')" \
  "preserve the already-absolute path; do not canonicalize on BSD"

# 6. grep -P (PCRE) — not implemented by BSD grep.
report "class-6 grep-pcre" \
  "$(scan_class 'grep[[:space:]]+-[[:alnum:]]*P')" \
  "rewrite as ERE (grep -E), or detect a PCRE grep (ggrep)"

# 7. [[ -v VAR ]] set-test — requires bash >= 4.2; macOS ships bash 3.2.
report "class-7 bracket-v-isset" \
  "$(scan_class '\[\[[[:space:]]+-v[[:space:]]')" \
  "use [[ -n \"\${VAR+set}\" ]] / [[ -n \"\${!ref+set}\" ]]"

# 8. mapfile / readarray — bash >= 4 builtins; macOS bash is 3.2.
report "class-8 mapfile-readarray" \
  "$(scan_class '(^|[^_[:alnum:]-])(mapfile|readarray)[[:space:]]' \
      'BASH_VERSINFO')" \
  "use a while IFS= read -r loop, or guard with BASH_VERSINFO >= 4"

# 9. mktemp --suffix — GNU-only long option; BSD mktemp lacks it.
report "class-9 mktemp-suffix" \
  "$(scan_class 'mktemp[[:space:]].*--suffix')" \
  "f=\$(mktemp); mv \"\$f\" \"\$f.EXT\"; f=\"\$f.EXT\""

# 10. df --output — GNU coreutils only; BSD df has no --output.
report "class-10 df-output" \
  "$(scan_class '(^|[^_[:alnum:]-])df[[:space:]].*--output')" \
  "parse POSIX df -P output via awk (NR==2)"

# 11. /bin/true and /bin/false — absent on macOS (they live at /usr/bin).
report "class-11 bin-true-false" \
  "$(scan_class '/bin/(true|false)')" \
  "use bare true / false (avoid the absolute /bin path)"

# 12. paste -s -d without an explicit '-' stdin operand — BSD paste needs it.
report "class-12 paste-no-stdin-operand" \
  "$(scan_class '(^|[^_[:alnum:]-])paste[[:space:]]+-[a-z]*s' \
      '[[:space:]]-([[:space:]]|\||;|\)|&|$)')" \
  "append an explicit '-' stdin operand so BSD paste reads stdin"

# 13. date +%s%N (nanoseconds) — BSD date may echo a literal N.
report "class-13 date-nanoseconds" \
  "$(scan_class '(^|[^_[:alnum:]-])date[[:space:]]+[+]%s%N' \
      'bubbles_now_ms')" \
  "use bubbles_now_ms (numeric-guards the %N and falls back to seconds)"

echo
if [[ "$violations" -gt 0 ]]; then
  echo "FAIL: $violations macOS-portability construct class(es) found in the scanned surface."
  echo "See instructions/wsl-macos-compatibility.instructions.md (and skill bubbles-cross-platform-shell)."
  exit 1
fi
echo "PASS: the scanned surface is WSL+macOS portable."
exit 0
