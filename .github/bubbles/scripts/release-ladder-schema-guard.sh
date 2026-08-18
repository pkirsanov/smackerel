#!/usr/bin/env bash
# release-ladder-schema-guard.sh — Gate G137.
#
# Enforces the SHAPE of a repository's release ladder. G101
# (release-delivery-reconciliation-guard.sh) asks "is every promised required
# feature actually delivered?"; this gate asks the prior question "is the ladder
# itself well-formed, ordered, and stably identified?" — the question that must
# be answerable before a delivery assertion means anything.
#
# The two gates are deliberately separate. A repo can have a perfectly ordered
# ladder full of undelivered work (structurally clean, honestly red), or a
# fully delivered set of packets whose headers disagree about what phase they
# are (delivered, but unprovable). Collapsing them would let either failure
# hide the other.
#
# WHO DECLARES WHAT (the joint split this gate exists to support):
#   the PRODUCT declares its ladder, phase headers, dependsOn, and per-feature
#   bindings; the FRAMEWORK enforces that those declarations are internally
#   consistent. The framework never invents a ladder — a repo that declares
#   none is EXEMPT rather than wrong, because most repos have no release
#   ladder and inventing one for them would be a false finding.
#
# Ladder declaration (authored once, in docs/releases/README.md):
#   <!-- bubbles:release-ladder schemaVersion=1 phases=alpha,beta,ga -->
#
# Per-phase packet header (docs/releases/<phase>/*.md):
#   <!-- bubbles:reconciled-packet schemaVersion=1 phase=<phase> dependsOn=<csv|none> -->
#
# Per-feature binding (shared with G101):
#   <!-- bubbles:feature id=<id> spec=<spec-dir|none>
#        delivery=required|optional|carried|deferred-to:<phase>
#        [assurance=implemented|planned] [carried-regression=blocking|non-blocking] -->
#
# Checks:
#   header      exactly one packet header per phase; schemaVersion=1; the header's
#               declared phase equals its directory; the phase is on the ladder
#   dependsOn   equals the FULL transitive predecessor set in ladder order, so a
#               phase can never silently skip a prerequisite
#   binding     id/spec/delivery are mandatory; delivery is a closed vocabulary;
#               deferred-to resolves to a known LATER phase (never itself, never
#               backward — a backward defer is an unreachable promise)
#   assurance   only on required bindings, only implemented|planned
#   carried     delivery=carried MUST declare carried-regression=blocking|non-blocking
#   identity    one capability id <-> one spec across every phase (bijection);
#               drift is what makes "the same capability" unprovable across phases
#
# Exit: 0 clean or exempt ; 1 violation ; 2 usage/runtime
#
# There is NO --skip / --force / --ignore bypass by design. A new trusted state
# is reached by fixing the declaration, never by silencing the check.

set -euo pipefail

REPO_ROOT=""
LADDER_OVERRIDE=""

usage() {
  cat <<'EOF'
Usage: release-ladder-schema-guard.sh --repo-root <dir> [--ladder "<p1 p2 ...>"]

  --repo-root <dir>     repo to scan (required)
  --ladder "<p1 p2>"    space-separated ladder, overriding the repo declaration.
                        Intended for hermetic selftests and for a repo that
                        keeps its ladder outside docs/releases/README.md. This
                        cannot relax any check: a bogus ladder makes the guard
                        fail on missing phase directories.

A repo that declares no ladder and passes no --ladder is EXEMPT (exit 0).

Exit: 0 clean/exempt ; 1 violation ; 2 usage/runtime
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      REPO_ROOT="${2:-}"
      shift 2
      ;;
    --ladder)
      LADDER_OVERRIDE="${2:-}"
      shift 2
      ;;
    -h | --help)
      usage
      exit 2
      ;;
    *)
      if [[ -z "$REPO_ROOT" && -d "$1" ]]; then
        REPO_ROOT="$1"
        shift
      else
        echo "[release-ladder-schema-guard][ERROR] unknown arg: $1" >&2
        usage >&2
        exit 2
      fi
      ;;
  esac
done

if [[ -z "$REPO_ROOT" ]]; then
  echo "[release-ladder-schema-guard][ERROR] --repo-root is required" >&2
  usage >&2
  exit 2
fi
if [[ ! -d "$REPO_ROOT" ]]; then
  echo "[release-ladder-schema-guard][ERROR] repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi

RELEASES_DIR="$REPO_ROOT/docs/releases"

# ---- ladder resolution ------------------------------------------------------
# Precedence: explicit --ladder, else the repo's own declaration, else EXEMPT.
LADDER=""
if [[ -n "$LADDER_OVERRIDE" ]]; then
  LADDER="$LADDER_OVERRIDE"
elif [[ -f "$RELEASES_DIR/README.md" ]]; then
  decl="$(grep -hoE '<!-- *bubbles:release-ladder [^>]*-->' "$RELEASES_DIR/README.md" 2>/dev/null | head -1 || true)"
  if [[ -n "$decl" ]]; then
    sv="$(printf '%s' "$decl" | grep -oE '(^| )schemaVersion=[^ ]+' | head -1 | sed -E 's/^ ?schemaVersion=//' || true)"
    if [[ "$sv" != "1" ]]; then
      echo "[release-ladder-schema-guard][ERROR] release-ladder declares schemaVersion='$sv' — must be 1 and refused, not best-effort parsed" >&2
      exit 1
    fi
    phases_csv="$(printf '%s' "$decl" | grep -oE '(^| )phases=[^ ]+' | head -1 | sed -E 's/^ ?phases=//' || true)"
    if [[ -z "$phases_csv" ]]; then
      echo "[release-ladder-schema-guard][ERROR] release-ladder declaration carries no phases= list — a declaration that names nothing would make this gate a silent no-op" >&2
      exit 1
    fi
    LADDER="$(printf '%s' "$phases_csv" | tr ',' ' ')"
  fi
fi

if [[ -z "$LADDER" ]]; then
  echo "[release-ladder-schema-guard] EXEMPT: no bubbles:release-ladder declaration in docs/releases/README.md and no --ladder given (repo does not use a release ladder)"
  exit 0
fi

VIOLATIONS=0
fail() {
  echo "[release-ladder-schema-guard][ERROR] $1" >&2
  VIOLATIONS=$((VIOLATIONS + 1))
}

# Duplicate phase in the declared ladder makes ordering ambiguous.
dupe="$(printf '%s\n' $LADDER | sort | uniq -d | tr '\n' ' ')"
if [[ -n "${dupe// /}" ]]; then
  echo "[release-ladder-schema-guard][ERROR] ladder declares duplicate phase(s): $dupe" >&2
  exit 1
fi

if [[ ! -d "$RELEASES_DIR" ]]; then
  echo "[release-ladder-schema-guard][ERROR] ladder is declared but $RELEASES_DIR does not exist" >&2
  exit 1
fi

phase_index() {
  target="$1"
  i=0
  for p in $LADDER; do
    i=$((i + 1))
    if [[ "$p" == "$target" ]]; then
      printf '%s' "$i"
      return 0
    fi
  done
  printf '0'
}

expected_depends_on() {
  target="$1"
  acc=""
  for p in $LADDER; do
    [[ "$p" == "$target" ]] && break
    if [[ -z "$acc" ]]; then acc="$p"; else acc="$acc,$p"; fi
  done
  if [[ -z "$acc" ]]; then printf 'none'; else printf '%s' "$acc"; fi
}

attr() { printf '%s' "$1" | grep -oE "(^| )$2=[^ ]+" | head -1 | sed -E "s/^ ?$2=//" || true; }

WORK="$(mktemp -d -t bubbles-ladder-schema-XXXXXXXX)"
trap 'rm -rf "$WORK"' EXIT INT TERM
IDENTITY_PAIRS="$WORK/identity"
: >"$IDENTITY_PAIRS"

# ---- per-phase header checks ------------------------------------------------
for phase in $LADDER; do
  dir="$RELEASES_DIR/$phase"
  if [[ ! -d "$dir" ]]; then
    fail "phase directory missing: docs/releases/$phase (declared on the ladder)"
    continue
  fi

  header_count="$(grep -rhc 'bubbles:reconciled-packet' "$dir"/*.md 2>/dev/null | awk '{s+=$1} END {print s+0}')"
  if [[ "$header_count" -ne 1 ]]; then
    fail "$phase: expected exactly 1 packet header, found $header_count"
    continue
  fi

  header="$(grep -rh 'bubbles:reconciled-packet' "$dir"/*.md 2>/dev/null | head -1)"

  sv="$(attr "$header" schemaVersion)"
  if [[ "$sv" != "1" ]]; then
    fail "$phase: schemaVersion='$sv' — must be 1 and refused, not best-effort parsed"
  fi

  declared_phase="$(attr "$header" phase)"
  if [[ "$declared_phase" != "$phase" ]]; then
    fail "$phase: header declares phase='$declared_phase' but the directory is '$phase'"
  fi

  want="$(expected_depends_on "$phase")"
  got="$(attr "$header" dependsOn)"
  if [[ -n "$got" && "$got" != "$want" ]]; then
    fail "$phase: dependsOn='$got' but the full transitive predecessor set in ladder order is '$want'"
  fi
done

# ---- per-feature binding checks ---------------------------------------------
for phase in $LADDER; do
  dir="$RELEASES_DIR/$phase"
  [[ -d "$dir" ]] || continue

  # Bash 3.2 has no process-substitution arrays; collect to a file first so the
  # loop body runs in THIS shell and violation counting is not lost to a subshell.
  grep -rhoE '<!-- *bubbles:feature [^>]*-->' "$dir"/*.md 2>/dev/null >"$WORK/ann.$phase" || true
  [[ -s "$WORK/ann.$phase" ]] || continue

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    fid="$(attr "$line" id)"
    fspec="$(attr "$line" spec)"
    fdelivery="$(attr "$line" delivery)"
    fassurance="$(attr "$line" assurance)"
    fcarried="$(attr "$line" 'carried-regression')"

    if [[ -z "$fid" || -z "$fspec" || -z "$fdelivery" ]]; then
      fail "$phase: malformed binding (id/spec/delivery all required): $line"
      continue
    fi

    case "$fdelivery" in
      required | optional | carried) ;;
      deferred-to:*)
        target="${fdelivery#deferred-to:}"
        if [[ "$(phase_index "$target")" == "0" ]]; then
          fail "$phase: binding '$fid' defers to unknown phase '$target'"
        elif [[ "$target" == "$phase" ]]; then
          fail "$phase: binding '$fid' defers to itself"
        elif [[ "$(phase_index "$target")" -le "$(phase_index "$phase")" ]]; then
          fail "$phase: binding '$fid' defers BACKWARD to '$target' (an unreachable promise)"
        fi
        ;;
      *)
        fail "$phase: binding '$fid' has delivery='$fdelivery' outside the closed vocabulary (required|optional|carried|deferred-to:<phase>)"
        ;;
    esac

    if [[ "$fdelivery" == "carried" && -z "$fcarried" ]]; then
      fail "$phase: binding '$fid' is delivery=carried but omits carried-regression"
    fi
    if [[ -n "$fcarried" && "$fcarried" != "blocking" && "$fcarried" != "non-blocking" ]]; then
      fail "$phase: binding '$fid' has carried-regression='$fcarried' outside {blocking,non-blocking}"
    fi

    if [[ -n "$fassurance" ]]; then
      if [[ "$fassurance" != "implemented" && "$fassurance" != "planned" ]]; then
        fail "$phase: binding '$fid' has assurance='$fassurance' outside {implemented,planned}"
      fi
      if [[ "$fdelivery" != "required" ]]; then
        fail "$phase: binding '$fid' carries assurance on a non-required binding (meaningless there)"
      fi
    fi

    printf '%s %s %s\n' "$fid" "$fspec" "$phase" >>"$IDENTITY_PAIRS"
  done <"$WORK/ann.$phase"
done

# ---- stable capability identity (bijection across phases) -------------------
if [[ -s "$IDENTITY_PAIRS" ]]; then
  for bad_id in $(awk '{print $1" "$2}' "$IDENTITY_PAIRS" | sort -u | awk '{print $1}' | sort | uniq -d); do
    specs="$(awk -v k="$bad_id" '$1==k {print $2}' "$IDENTITY_PAIRS" | sort -u | tr '\n' ' ')"
    fail "stable-id drift: capability id '$bad_id' is bound to multiple specs: $specs"
  done
  for bad_spec in $(awk '{print $2" "$1}' "$IDENTITY_PAIRS" | sort -u | awk '{print $1}' | sort | uniq -d); do
    [[ "$bad_spec" == "none" ]] && continue
    ids="$(awk -v k="$bad_spec" '$2==k {print $1}' "$IDENTITY_PAIRS" | sort -u | tr '\n' ' ')"
    fail "stable-id drift: spec '$bad_spec' is bound to multiple capability ids: $ids"
  done
fi

phase_count="$(printf '%s\n' $LADDER | grep -c . || true)"
if [[ "$VIOLATIONS" -eq 0 ]]; then
  echo "[release-ladder-schema-guard] OK (G137: $phase_count-phase ladder, headers, dependsOn, bindings, and capability identity conform)"
  exit 0
fi

echo "" >&2
echo "[release-ladder-schema-guard][ERROR] G137: the release ladder declaration is not internally consistent ($VIOLATIONS violation(s) above). A delivery assertion over a malformed ladder cannot mean anything." >&2
exit 1
