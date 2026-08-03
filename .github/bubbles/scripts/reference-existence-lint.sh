#!/usr/bin/env bash
# reference-existence-lint.sh — mechanical phantom-reference detector (gate G132).
#
# Gate G132 (reference_existence_gate) forbids citing a path that does not
# exist. A markdown link is a promise that its target is real, so a link whose
# target resolves to nothing on disk is a phantom reference: it reads as
# verified evidence, it propagates, and a later agent builds on it.
#
# The policy this enforces lives in agents/bubbles_shared/claim-grounding.md
# (No Phantom References). Gate ID references are covered separately by
# bubbles/scripts/gate-id-grep.sh; this lint owns PATH references.
#
# REUSABLE BY DESIGN — the scan surface is an argument (files and/or
# directories). There is NO default surface, following the
# macos-portability-guard.sh and technical-prose-lint.sh precedent: the tool is
# never implicitly pointed at a whole repository.
#
# ---------------------------------------------------------------------------
# WHAT IS CHECKED
#
#   Relative markdown link targets — [text](some/relative/path.md) — resolved
#   against the directory of the file that carries the link. A target that does
#   not exist as a file or a directory is a finding.
#
# WHAT IS DELIBERATELY *NOT* CHECKED
#
#   external schemes     http:, https:, mailto:, and any other scheme. This
#                        lint reads the local filesystem; it never makes a
#                        network call.
#   absolute paths       a leading / cannot be resolved reliably across a
#                        source checkout and a downstream install root.
#   bare anchors         #section links are intra-document, not paths.
#   placeholder targets  any target containing < > { } $ * — these are
#                        template slots such as specs/<NNN-feature-name>/spec.md
#                        and are not claims that a concrete path exists.
#   fenced code blocks   examples routinely name hypothetical paths.
#   inline code spans    `[a](b)` inside backticks is sample text, not a link.
#   bare inline paths    a backticked path such as `.github/bubbles/scripts/cli.sh`
#                        is frequently a DOWNSTREAM projection that correctly
#                        does not exist in the source checkout. Flagging those
#                        would bury real findings in false positives, and a lint
#                        that cries wolf gets ignored.
#
# ESCAPE HATCH
#
#   A genuinely intentional dangling link is exempted inline by placing
#   `ref-ok:<reason>` anywhere on the same line. There is no other bypass, no
#   --skip, no --force, and no allowlist file.
#
# ---------------------------------------------------------------------------
# ADVISORY UNTIL OPT-IN
#
#   By DEFAULT this prints findings and exits 0, so it can never break a
#   currently-green tree or reject historical artifacts. Set
#   `referenceExistenceGuard: block` in .github/bubbles-project.yaml to make
#   findings fail (exit 1).
#
# Usage:
#   reference-existence-lint.sh <path> [<path>...]
#
# Exit 0 = clean, OR advisory mode. Exit 1 = findings AND block mode enabled.
# Exit 2 = usage / environment error.

set -euo pipefail

err() { echo "[reference-existence-lint][ERROR] $*" >&2; }
info() { echo "[reference-existence-lint] $*"; }

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/reference-existence-lint.sh <path> [<path>...]

Reports relative markdown link targets that do not exist on disk.
There is NO default scan surface — name the files or directories to scan.

Exit 0 = clean, or findings in advisory mode (the default).
Exit 1 = findings while `referenceExistenceGuard: block` is set in
         .github/bubbles-project.yaml.
Exit 2 = usage or environment error.

Exempt a deliberately dangling link by putting `ref-ok:<reason>` on the
same line. There is no --skip, --force, or allowlist bypass.
EOF
}

targets=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    -*)
      err "unknown argument: $1"
      usage >&2
      exit 2
      ;;
    *)
      targets+=("$1")
      shift
      ;;
  esac
done

if [[ "${#targets[@]}" -eq 0 ]]; then
  err "no scan surface supplied — this tool has no default surface"
  usage >&2
  exit 2
fi

for t in "${targets[@]}"; do
  if [[ ! -e "$t" ]]; then
    err "scan path not found: $t"
    exit 2
  fi
done

# ── advisory-until-opt-in: walk up from the first target for the config ──
mode="advisory"
probe="${targets[0]}"
if [[ -f "$probe" ]]; then
  probe="$(cd "$(dirname "$probe")" && pwd)"
else
  probe="$(cd "$probe" && pwd)"
fi
dir="$probe"
while :; do
  if [[ -f "$dir/.github/bubbles-project.yaml" ]]; then
    if grep -qE '^[[:space:]]*referenceExistenceGuard:[[:space:]]*block[[:space:]]*$' "$dir/.github/bubbles-project.yaml"; then
      mode="block"
    fi
    break
  fi
  [[ "$dir" == "/" || -z "$dir" ]] && break
  dir="$(dirname "$dir")"
done

# ── collect markdown files from the supplied surface ──────────────────────
files=()
while IFS= read -r f; do
  [[ -n "$f" ]] || continue
  files+=("$f")
done < <(
  for t in "${targets[@]}"; do
    if [[ -f "$t" ]]; then
      case "$t" in
        *.md) printf '%s\n' "$t" ;;
      esac
    else
      find "$t" -type f -name '*.md' \
        -not -path '*/node_modules/*' \
        -not -path '*/.git/*' 2>/dev/null || true
    fi
  done | LC_ALL=C sort -u
)

if [[ "${#files[@]}" -eq 0 ]]; then
  info "OK — no markdown files in the supplied surface"
  exit 0
fi

# ── extract candidate relative link targets ───────────────────────────────
# Emits: <lineno><TAB><target> for each checkable link.
extract_links() {
  awk '
    BEGIN { fenced = 0 }
    {
      line = $0

      # fenced code blocks (``` or ~~~) — toggle and skip the fence line itself
      if (line ~ /^[[:space:]]*(```|~~~)/) { fenced = 1 - fenced; next }
      if (fenced) next

      # inline exemption
      if (line ~ /ref-ok:/) next

      # strip inline code spans so `[a](b)` samples are not treated as links
      gsub(/`[^`]*`/, "", line)

      rest = line
      while (match(rest, /\]\(([^()[:space:]]*)/)) {
        raw = substr(rest, RSTART + 2, RLENGTH - 2)
        rest = substr(rest, RSTART + RLENGTH)

        # trim a trailing ) left by the non-greedy capture
        sub(/\)+$/, "", raw)
        if (raw == "") continue

        # angle-bracket wrapped target
        sub(/^</, "", raw)
        sub(/>$/, "", raw)

        # drop the anchor
        sub(/#.*$/, "", raw)
        if (raw == "") continue

        # placeholder / template slot — not a claim that a path exists
        if (raw ~ /[<>{}$*]/) continue

        # external scheme (http:, https:, mailto:, file:, vscode:, ...)
        if (raw ~ /^[a-zA-Z][a-zA-Z0-9+.-]*:/) continue

        # absolute path — unresolvable across source vs installed roots
        if (raw ~ /^\//) continue

        # percent-encoded space is the only encoding worth decoding here
        gsub(/%20/, " ", raw)

        printf "%d\t%s\n", NR, raw
      }
    }
  ' "$1"
}

findings=0
scanned=0

for f in "${files[@]}"; do
  scanned=$((scanned + 1))
  base_dir="$(dirname "$f")"
  while IFS=$'\t' read -r lineno target; do
    [[ -n "$lineno" ]] || continue
    [[ -e "$base_dir/$target" ]] && continue
    err "$f:$lineno broken relative link target: $target"
    findings=$((findings + 1))
  done < <(extract_links "$f")
done

if [[ "$findings" -eq 0 ]]; then
  info "OK — $scanned markdown file(s) scanned, every relative link target resolves"
  exit 0
fi

if [[ "$mode" == "block" ]]; then
  err "$findings phantom reference(s) across $scanned file(s) — failing (referenceExistenceGuard: block)"
  exit 1
fi
info "$findings phantom reference(s) across $scanned file(s) — advisory only (exit 0). Set referenceExistenceGuard: block in .github/bubbles-project.yaml to enforce."
exit 0
