#!/usr/bin/env bash
# File: bubbles-drift-check.sh
#
# Fast, read-only "am I drifted from the framework?" check for a repo that has
# Bubbles installed. It recomputes the sha256 of every vendored managed file and
# compares it against the checksum recorded in the installed
# `bubbles/release-manifest.json` (the authoritative framework fingerprint),
# reporting per-file: IN-SYNC / DRIFTED (hash mismatch) / MISSING (manifest path
# absent on disk). It also reports ORPHAN framework scripts (a file in the
# wholly-framework-owned `bubbles/scripts[/guards]` dir that the manifest no
# longer lists — the IMP-008 install prune removes these; this surfaces them
# between upgrades).
#
# Unlike a full reinstall, this needs no network and mutates nothing — it gives a
# product pre-push or `doctor` a one-call drift signal (IMP-013).
#
# Usage: bubbles-drift-check.sh [--format text|json] [--root <path>] [-h]
#   --root <path>   install root to check (default: derived from this script's
#                   location — `<root>/.github/` downstream, `<root>/` in source).
#   --format text   human summary (default).
#   --format json   machine-readable summary.
#
# Exit: 0 = in-sync (no drift, no missing) OR no manifest present (nothing to
#           check) OR python3 unavailable (advisory skip);
#       1 = drift and/or missing managed files detected;
#       2 = usage error.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FORMAT="text"
ROOT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --format)
      FORMAT="${2:-}"
      shift 2 || {
        echo "bubbles-drift-check: --format needs a value" >&2
        exit 2
      }
      ;;
    --format=*)
      FORMAT="${1#*=}"
      shift
      ;;
    --root)
      ROOT="${2:-}"
      shift 2 || {
        echo "bubbles-drift-check: --root needs a value" >&2
        exit 2
      }
      ;;
    --root=*)
      ROOT="${1#*=}"
      shift
      ;;
    -h | --help)
      sed -n '2,28p' "$0"
      exit 0
      ;;
    *)
      echo "bubbles-drift-check: unknown argument '$1'." >&2
      exit 2
      ;;
  esac
done

case "$FORMAT" in
  text | json) ;;
  *)
    echo "bubbles-drift-check: --format must be 'text' or 'json' (got '$FORMAT')." >&2
    exit 2
    ;;
esac

# Resolve the install root. Downstream layout: this script lives at
# <root>/.github/bubbles/scripts → root is <root>/.github. Source layout:
# <root>/bubbles/scripts → root is <root>.
if [[ -z "$ROOT" ]]; then
  if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
    ROOT="${SCRIPT_DIR%/bubbles/scripts}"
  else
    ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  fi
fi

MANIFEST="$ROOT/bubbles/release-manifest.json"

if [[ ! -f "$MANIFEST" ]]; then
  [[ "$FORMAT" == "json" ]] \
    && echo '{"status":"no-manifest","root":"'"$ROOT"'"}' \
    || echo "bubbles-drift-check: no release manifest at $MANIFEST — nothing to check."
  exit 0
fi

if ! command -v python3 >/dev/null 2>&1; then
  [[ "$FORMAT" == "json" ]] \
    && echo '{"status":"skipped","reason":"python3 unavailable"}' \
    || echo "bubbles-drift-check: python3 unavailable — advisory skip (exit 0)."
  exit 0
fi

BUBBLES_DRIFT_ROOT="$ROOT" BUBBLES_DRIFT_FORMAT="$FORMAT" python3 - "$MANIFEST" <<'PYEOF'
import hashlib
import json
import os
import sys

manifest_path = sys.argv[1]
root = os.environ["BUBBLES_DRIFT_ROOT"]
fmt = os.environ["BUBBLES_DRIFT_FORMAT"]

try:
    with open(manifest_path, encoding="utf-8") as fh:
        manifest = json.load(fh)
except (ValueError, OSError) as exc:
    print(f"bubbles-drift-check: cannot read manifest: {exc}", file=sys.stderr)
    sys.exit(2)

entries = manifest.get("managedFileChecksums")
if not isinstance(entries, list):
    print("bubbles-drift-check: manifest has no managedFileChecksums list", file=sys.stderr)
    sys.exit(2)


def _read_designlanguages_block(root_dir):
    # Operator-owned project config; the .github/ form is preferred, with
    # repo-root fallbacks for both the source-repo and downstream layouts.
    candidates = [
        os.path.join(root_dir, "bubbles-project.yaml"),
        os.path.join(root_dir, ".github", "bubbles-project.yaml"),
        os.path.join(os.path.dirname(root_dir), "bubbles-project.yaml"),
    ]
    for cfg in candidates:
        if not os.path.isfile(cfg):
            continue
        try:
            with open(cfg, encoding="utf-8") as fh:
                lines = fh.read().splitlines()
        except OSError:
            continue
        block, capturing = [], False
        for line in lines:
            if not capturing:
                if line.startswith("designLanguages:"):
                    capturing = True
                continue
            if line and not line[0].isspace():
                break  # a new top-level key ends the block
            block.append(line)
        return "\n".join(block).lower()
    return ""


def _opted_out_skill_dirs(root_dir):
    # An optional skill (listed in the vendored optional-skills registry) is
    # "opted out" when its enablement token is absent from the designLanguages
    # block — its files are then legitimately absent on disk (not a drift).
    reg = os.path.join(root_dir, "bubbles", "registry", "optional-skills.txt")
    out = set()
    if not os.path.isfile(reg):
        return out
    dl_block = _read_designlanguages_block(root_dir)
    try:
        with open(reg, encoding="utf-8") as fh:
            reg_lines = fh.read().splitlines()
    except OSError:
        return out
    for raw in reg_lines:
        item = raw.split("#", 1)[0].strip()
        if not item:
            continue
        parts = item.split()
        name = parts[0]
        token = parts[1] if len(parts) > 1 else name
        if token.lower() not in dl_block:
            out.add(name)
    return out


_OPTED_OUT_SKILL_DIRS = _opted_out_skill_dirs(root)


def _rel_is_opted_out(rel):
    return any(rel.startswith(f"skills/{name}/") for name in _OPTED_OUT_SKILL_DIRS)


opted_out = []


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


in_sync = 0
drifted = []
missing = []
managed_paths = set()

for entry in entries:
    if not isinstance(entry, dict):
        continue
    rel = entry.get("path")
    want = entry.get("sha256")
    if not rel or not want:
        continue
    managed_paths.add(rel)
    abspath = os.path.join(root, rel)
    if not os.path.isfile(abspath):
        (opted_out if _rel_is_opted_out(rel) else missing).append(rel)
        continue
    try:
        got = sha256_file(abspath)
    except OSError as exc:
        (opted_out if _rel_is_opted_out(rel) else missing).append(rel)
        continue
    if got == want:
        in_sync += 1
    else:
        drifted.append(rel)

# Orphan detection scoped to the wholly-framework-owned script dirs (no operator
# files live there). A *.sh present on disk but absent from the manifest is an
# orphan the IMP-008 install prune would remove on the next upgrade.
orphans = []
for sub in ("bubbles/scripts", "bubbles/scripts/guards"):
    d = os.path.join(root, sub)
    if not os.path.isdir(d):
        continue
    for name in os.listdir(d):
        if not name.endswith(".sh"):
            continue
        rel = f"{sub}/{name}"
        if rel not in managed_paths and os.path.isfile(os.path.join(d, name)):
            orphans.append(rel)

drifted.sort()
missing.sort()
opted_out.sort()
orphans.sort()

if fmt == "json":
    print(json.dumps({
        "status": "drift" if (drifted or missing) else "in-sync",
        "inSync": in_sync,
        "drifted": drifted,
        "missing": missing,
        "optedOut": opted_out,
        "orphans": orphans,
        "version": manifest.get("version"),
    }, indent=2))
else:
    print(f"Bubbles drift check (manifest version {manifest.get('version')})")
    print(f"  in-sync : {in_sync}")
    print(f"  drifted : {len(drifted)}")
    print(f"  missing : {len(missing)}")
    print(f"  opt-out : {len(opted_out)} (optional skills not enabled in designLanguages; absence is expected)")
    print(f"  orphans : {len(orphans)} (framework scripts not in manifest; pruned on next upgrade)")
    for rel in drifted:
        print(f"  DRIFTED  {rel}")
    for rel in missing:
        print(f"  MISSING  {rel}")
    for rel in opted_out:
        print(f"  OPT-OUT  {rel}")
    for rel in orphans:
        print(f"  ORPHAN   {rel}")
    if not drifted and not missing:
        print("  result  : IN-SYNC")
    else:
        print("  result  : DRIFT DETECTED")

sys.exit(1 if (drifted or missing) else 0)
PYEOF
