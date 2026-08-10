#!/usr/bin/env bash
#
# security-gate.sh — mechanical enforcer for G034 (security_gate)
# (IMP-027 / SCOPE-4, SEC-3).
#
# WHY THIS EXISTS
# ---------------
# G034 was classified as one of only six businessInvariant gates, yet had NO
# enforcer script and was not referenced by any agent — including
# bubbles.security.agent.md. Its entire enforcement was "appears in a mode's
# requiredGates list", i.e. an LLM reading YAML and deciding whether it felt
# satisfied. A businessInvariant security gate with no mechanical surface is
# the weakest link in the invariant set: it carries the most authority and the
# least verification.
#
# This script gives it a surface. It checks the things a security gate can
# actually check about a repository from the outside, without pretending to be
# a vulnerability scanner:
#
#   1. secret-material    no private keys / keystores committed
#   2. inline-credentials no hardcoded password/token assignments in scripts
#   3. curl-pipe-shell    no `curl … | bash` remote-execution primitives
#   4. world-writable     no world-writable files in the tracked tree
#   5. eval-on-input      no `eval` on unsanitised command substitution
#
# Each finding names the file and line so it is actionable. This is a floor,
# not a ceiling: a project may wire additional scanners into its own gate set.
#
# Exit codes: 0 clean - 1 findings - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
quiet=0

usage() {
  cat <<'EOF'
security-gate.sh — mechanical enforcer for G034 (security_gate)

Usage:
  bash bubbles/scripts/security-gate.sh [--repo-root <path>] [--quiet]

Exit: 0 clean - 1 findings - 2 usage/environment error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "security-gate: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "security-gate: repo root not found: $repo_root" >&2
  exit 2
fi

findings=0
report() {
  printf 'FINDING: %s: %s\n' "$1" "$2"
  findings=$((findings + 1))
}

# An inline `security-gate:allow` / `gitleaks:allow` on the offending LINE is
# an explicit operator acknowledgement, and the same standard this gate
# already applies to the highest-severity class: a committed key block.
#
# It cannot be inferred, it has to be typed on the exact line, and it lands in
# the diff a reviewer is already reading. Without it a consuming repo cannot
# resolve a legitimate finding — an official installer one-liner, a hermetic
# test fixture — except by patching this framework file or switching the gate
# off, and a gate that gets switched off enforces nothing.
#
# Every acknowledgement stays auditable:
#   grep -rn 'security-gate:allow' .
acknowledged() {
  case "$1" in
    *security-gate:allow* | *gitleaks:allow*) return 0 ;;
  esac
  return 1
}

cd "$repo_root"

# Only inspect tracked files: the working tree may hold local scratch, and a
# gate that fails on an operator's untracked notes is a gate people disable.
if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
  mapfile -t tracked < <(git ls-files 2>/dev/null || true)
else
  # NOT `find -printf '%P\n'`: -printf is a GNU extension that BSD/macOS find
  # rejects with "unknown primary or operator". That failure emptied the
  # denominator and made this gate report OK on every non-git tree on macOS —
  # a green gate that had inspected nothing. Strip the leading './' instead.
  mapfile -t tracked < <(find . -type f -not -path './.git/*' 2>/dev/null | sed 's|^\./||' || true)
fi

if [[ "${#tracked[@]}" -eq 0 ]]; then
  [[ "$quiet" -eq 1 ]] || echo "[security-gate] OK — no tracked files to inspect"
  exit 0
fi

# --- 1. Committed secret material ------------------------------------------
for f in "${tracked[@]}"; do
  case "$f" in
    *.pem | *.key | *.p12 | *.pfx | *.jks | *.keystore | *.mobileprovision | id_rsa | id_ed25519)
      report "secret-material" "$f is committed key/keystore material"
      ;;
  esac
done

while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  # A private-key MARKER inside documentation is the doc describing the pattern
  # (SECURITY-PII-POLICY.md does exactly that).
  case "$hit" in
    *.md | *security-gate*) continue ;;
  esac
  # Requiring only that the closing marker ALSO appears somewhere in the file
  # asks the wrong question. A test that asserts a PEM prefix on one line and
  # the matching suffix on the next holds both markers and zero key bytes, and
  # was reported as committed key material. Co-occurrence of two strings is not
  # a key block. (The literal markers are deliberately NOT quoted in this
  # comment: writing them here would make this file trip the very scanners it
  # exists to complement.)
  #
  # Require a base64 payload BETWEEN the markers, so the finding means "this
  # file carries key material" rather than "this file mentions the words".
  #
  # "Payload" has to mean a CONTIGUOUS base64 run, not "enough base64-ish
  # characters". Deleting punctuation from ordinary prose leaves base64:
  #   pytest.fail("sidecar did not return a PEM PRIVATE KEY")
  # strips to 62 such characters and was reported as key material. A real PEM
  # body line has no interior spaces or punctuation, so the test is whether the
  # line CONSISTS of base64 once its surrounding quoting is removed.
  #
  # The escape handling is load-bearing, not cosmetic. A PEM embedded in a
  # source string literal carries \n escapes, so its body reads ...bvBRQ\n\ —
  # not contiguous, and a genuine committed key went UNDETECTED. Each source
  # line is therefore split on the escape before the segments are judged.
  #
  # Splitting the FILE (rather than each line) would be wrong: it moves a
  # trailing `// gitleaks:allow` off the BEGIN line and revokes the operator's
  # acknowledgement. The annotation is matched against the ORIGINAL line and
  # the payload against the segments.
  #
  # The block must also CLOSE. A file asserting on a prefix marker has no
  # closing marker at all; without this the scan ran to EOF and judged
  # unrelated text as the payload — the real trigger was a banner comment of
  # '=' characters, contiguous base64 because '=' is padding.
  #
  # An inline `gitleaks:allow` / `security-gate:allow` on the BEGIN line is an
  # explicit operator acknowledgement that the block is a throwaway fixture —
  # a structurally valid PEM is often the only way to test a PEM parser. It
  # must be written deliberately and is visible in review; nothing infers it,
  # so an unannotated real key still fails.
  awk '
    {
      orig = $0
      segments = split(orig, seg, /\\n/)
      for (i = 1; i <= segments; i++) {
        s = seg[i]
        if (s ~ /-----BEGIN [A-Z ]*PRIVATE KEY-----/) {
          if (orig ~ /gitleaks:allow|security-gate:allow/) {
            inblock = 0
          } else {
            inblock = 1
            candidate = 0
          }
          continue
        }
        if (inblock && s ~ /-----END [A-Z ]*PRIVATE KEY-----/) {
          if (candidate) { found = 1; exit }
          inblock = 0
          continue
        }
        if (inblock) {
          payload = s
          sub(/^[^A-Za-z0-9+\/=]+/, "", payload)
          sub(/[^A-Za-z0-9+\/=]+$/, "", payload)
          if (payload ~ /^[A-Za-z0-9+\/=]{32,}$/) { candidate = 1 }
        }
      }
    }
    END { exit(found ? 0 : 1) }
  ' "$hit" 2>/dev/null || continue
  report "secret-material" "$hit contains a private-key block"
done < <(grep -rl -- '-----BEGIN .*PRIVATE KEY-----' "${tracked[@]}" 2>/dev/null || true)

# --- 2. Inline credentials in scripts --------------------------------------
#
# Matches an assignment of a non-empty literal to a secret-shaped name. Env
# indirection (VAR="$OTHER", VAR="${OTHER}") and empty placeholders are fine —
# those are the correct pattern, not the violation.
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  # Selftests build hermetic fixtures; a synthetic fixture token is not a
  # credential. Same carve-out the framework already grants selftests for
  # dependency SKIPs.
  case "$hit" in
    *-selftest.sh* | *dependency-posture.sh* | *security-gate* | *tests/*) continue ;;
  esac
  acknowledged "$hit" && continue
  report "inline-credentials" "$hit"
done < <(
  grep -rnE '^[^#]*\b[A-Za-z_]*(PASSWORD|PASSWD|SECRET|TOKEN|APIKEY|API_KEY)[A-Za-z_]*=["'"'"']?[A-Za-z0-9/+_.-]{8,}["'"'"']?' \
    --include='*.sh' --include='*.yaml' --include='*.yml' . 2>/dev/null |
    grep -vE '=\$|=["'"'"']\$|=["'"'"']{2}|:-|placeholder|example|EXAMPLE|\bfake\b|redact' |
    # A value that embeds a substitution is COMPOSED at runtime, so there is no
    # literal to leak: RUN_TOKEN="depqual-$$-$(date +%s)" is a run identifier,
    # not a credential. The exclusions above only catch a value that STARTS
    # with `$`, so any generated value carrying a literal prefix was reported.
    grep -vE '[$][({]|[$][$]' |
    # `TOKEN_CHARS='ABC…xyz0123'` is the ALPHABET a random token is drawn from.
    # The name matches because `[A-Za-z_]*` after TOKEN absorbs the suffix, but
    # publishing a character set leaks nothing.
    grep -vE '(_CHARS|_CHARSET|_ALPHABET)=' || true
)

# --- 3. curl-pipe-shell remote execution -----------------------------------
#
# Scoped to EXECUTABLE surfaces. Bubbles' own installation guide documents a
# `curl … | bash` one-liner — that is the product's published interface, and
# flagging it would be flagging the thing the user deliberately chose to run.
# The risk this catches is a SCRIPT silently fetching and executing remote code
# during a run the operator did not authorise.
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  case "$hit" in
    *security-gate*) continue ;;
  esac
  # A line that PRINTS the command is guidance, not execution. install.sh
  # echoes the one-liner so the operator can copy it; that is the opposite of
  # a silent fetch-and-run.
  line_body="${hit#*:*:}"
  case "$line_body" in
    *printf* | *echo*) continue ;;
  esac
  # Documented, operator-initiated exception: `cli.sh upgrade` re-invokes the
  # published installer on purpose. Recorded here rather than silently
  # excluded, so the one place the framework does this stays visible.
  case "$hit" in
    *"cli.sh"*"install.sh"*) continue ;;
  esac
  acknowledged "$hit" && continue
  report "curl-pipe-shell" "$hit"
done < <(
  grep -rnE '(curl|wget)[^|]*\|[[:space:]]*(sudo[[:space:]]+)?(ba)?sh\b' \
    --include='*.sh' --include='*.yml' --include='*.yaml' . 2>/dev/null |
    grep -vE '^\s*[^:]+:[0-9]+:\s*#' || true
)

# --- 4. World-writable tracked files ---------------------------------------
for f in "${tracked[@]}"; do
  [[ -f "$f" ]] || continue
  perms="$(ls -l "$f" 2>/dev/null | cut -c1-10)"
  case "$perms" in
    *w-) report "world-writable" "$f is world-writable ($perms)" ;;
    *w*) [[ "${perms:8:1}" == "w" ]] && report "world-writable" "$f is world-writable ($perms)" ;;
  esac
done

# --- 5. eval on unsanitised substitution ------------------------------------
while IFS= read -r hit; do
  [[ -n "$hit" ]] || continue
  case "$hit" in
    *security-gate*) continue ;;
  esac
  acknowledged "$hit" && continue
  report "eval-on-input" "$hit"
done < <(
  grep -rnE '^[^#]*\beval[[:space:]]+["'"'"']?\$\(' --include='*.sh' . 2>/dev/null || true
)

if [[ "$findings" -gt 0 ]]; then
  echo "[security-gate] FAIL — G034 findings: $findings"
  exit 1
fi

[[ "$quiet" -eq 1 ]] || echo "[security-gate] OK — ${#tracked[@]} tracked file(s), zero G034 findings"
exit 0
