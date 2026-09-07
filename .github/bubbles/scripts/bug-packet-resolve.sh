#!/usr/bin/env bash
set -euo pipefail

# bug-packet-resolve.sh
#
# BUG-041 — the ONE reader of bubbles/registry/bug-packet.yaml.
#
# WHY THIS EXISTS
# `bug-packet.yaml` is the declared single bug-artifact authority and it had NO
# production reader. `artifact-lint.sh` and `state-transition-guard.sh` each
# carried a private hard-coded copy of the `full` form's artifact list and
# applied it to every packet, so the `compact` form — the DEFAULT route since
# IMP-047 S-D — could not pass either surface. A contract nobody reads cannot
# govern anything.
#
# The repair follows the IMP-047 S-B precedent (`report-sections-resolve.sh`)
# rather than adding a branch to each surface: a branch inside a surface is a
# third private copy of the contract, which is the same defect in a new place.
# `artifact-lint.sh` deliberately sources no sibling library, so this is invoked
# as a subprocess, not sourced.
#
# Output lines (stable, greppable, one fact per line):
#   form=<form>                          a declared form
#   default=<form>                       the form applied when no declaration is present
#   field=<name>                         the state.json field carrying the declaration
#   location=<file>                      the artifact carrying that field
#   vocab=<word>|<form>                  an accepted declaration word and its canonical form
#   alias=<word>|<form>                  a vocab entry that is a DEPRECATED alias
#   artifact=<form>|<id>|<conditional>   an artifact that form requires (conditional yes/no)
#   obligation=<form>|<id>|<dischargedIn>|<attestedIn>
#                                        an obligation that form retains, with
#                                        the artifact that DISCHARGES it and the
#                                        artifact that ATTESTS its completion
#
# `vocab=` covers canonical words AND deprecated aliases, so a caller performs
# ONE lookup. `alias=` additionally marks the deprecated subset, so retirement
# can be tracked without a second vocabulary.
#
# BUG-042 added `obligation=`. `obligationsRetained:` had a tidy structure and
# ZERO consumers — this reader set its artifact flag FALSE and skipped every
# entry — so a reduced form's obligations were documentation. The carrier fields
# are what a consumer needs: `dischargedIn` says where the work is done and
# `attestedIn` says where its completion is claimed. A form that needs neither
# (single-file has exactly one artifact, so the carrier is unambiguous) emits
# them EMPTY rather than forcing a schema change on a form that has no question
# to answer.
#
# There is NO fallback list. A missing or unparseable registry exits non-zero
# and prints nothing, so a caller cannot silently degrade to an empty
# requirement set — an empty requirement set is a false-PASS, which is the same
# defect class as IMP-047 PD-04. A declared form that resolves to ZERO artifacts
# is refused for the same reason, and so is a REDUCED form that resolves to zero
# obligations: fewer artifacts is the contract, fewer obligations is the
# loophole.
#
# python3, not awk: the registry needs nested-block parsing, and the portable way
# to express that in awk needs 3-argument match(), which BSD awk does not have
# (macos-portability-guard class-16). python3 is already a hard dependency of the
# sibling resolvers.
#
# Exit codes:
#   0  resolved
#   2  usage error / registry missing / registry unparseable / empty artifact set
#      / reduced form declaring zero obligations

REGISTRY=""

die_usage() {
  printf 'bug-packet-resolve: %s\n' "$1" >&2
  printf 'usage: bug-packet-resolve.sh [--registry FILE]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --registry)
      shift
      REGISTRY="${1:-}"
      ;;
    -h | --help)
      sed -n '4,64p' "${BASH_SOURCE[0]}"
      exit 0
      ;;
    --skip* | --force* | --ignore* | --no-verify*)
      die_usage "bypass-shaped flag '$1' is not supported"
      ;;
    *) die_usage "unknown option '$1'" ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -n "$REGISTRY" ]] || REGISTRY="$SCRIPT_DIR/../registry/bug-packet.yaml"
[[ -f "$REGISTRY" ]] || die_usage "registry not found: $REGISTRY"
command -v python3 >/dev/null 2>&1 || die_usage "python3 is required to read the registry"

REGISTRY="$REGISTRY" python3 - <<'PY'
import os, re, sys

path = os.environ["REGISTRY"]
with open(path, encoding="utf-8") as fh:
    lines = fh.read().split("\n")

group = None            # top-level block currently open
forms = []              # declared form names, in file order
artifacts = {}          # form -> [(id, conditional)]
obligations = {}        # form -> [(id, dischargedIn, attestedIn)]
cur_form = None
form_block = None       # "artifacts" | "obligationsRetained" | None
cur_artifact = None
cur_obligation = None

decl_field = None
decl_location = None
decl_default = None
vocab = []              # (word, form, is_alias)
decl_sub = None         # "vocabulary" | "deprecatedAliases"
cur_word = None
cur_word_form = None


def flush_artifact():
    global cur_artifact
    if cur_artifact and cur_form:
        artifacts.setdefault(cur_form, []).append(cur_artifact)
    cur_artifact = None


def flush_obligation():
    global cur_obligation
    if cur_obligation and cur_form:
        obligations.setdefault(cur_form, []).append(cur_obligation)
    cur_obligation = None


def flush_word():
    global cur_word, cur_word_form
    if cur_word is not None and cur_word_form is not None:
        vocab.append((cur_word, cur_word_form, decl_sub == "deprecatedAliases"))
    cur_word, cur_word_form = None, None


for raw in lines:
    if re.match(r"^[A-Za-z]", raw):
        flush_artifact()
        flush_obligation()
        flush_word()
        head = raw.split(":", 1)[0]
        group = {"forms": "forms", "declaration": "decl"}.get(head)
        cur_form, form_block, decl_sub = None, None, None
        continue

    if group is None:
        continue

    if group == "forms":
        m = re.match(r"^  - form:\s*(\S+)\s*$", raw)
        if m:
            flush_artifact()
            flush_obligation()
            cur_form = m.group(1)
            forms.append(cur_form)
            form_block = None
            continue
        if cur_form is None:
            continue
        m = re.match(r"^    ([A-Za-z][A-Za-z0-9_]*):", raw)
        if m:
            flush_artifact()
            flush_obligation()
            key = m.group(1)
            form_block = key if key in ("artifacts", "obligationsRetained") else None
            continue
        if form_block == "artifacts":
            m = re.match(r"^      -\s+id:\s*(.+?)\s*$", raw)
            if m:
                flush_artifact()
                cur_artifact = (m.group(1), False)
                continue
            if cur_artifact and re.match(r"^        conditional:\s*true\s*$", raw):
                cur_artifact = (cur_artifact[0], True)
            continue
        if form_block == "obligationsRetained":
            m = re.match(r"^      -\s+id:\s*(.+?)\s*$", raw)
            if m:
                flush_obligation()
                cur_obligation = (m.group(1), "", "")
                continue
            if cur_obligation:
                m = re.match(r"^        dischargedIn:\s*(\S+)\s*$", raw)
                if m:
                    cur_obligation = (cur_obligation[0], m.group(1), cur_obligation[2])
                    continue
                m = re.match(r"^        attestedIn:\s*(\S+)\s*$", raw)
                if m:
                    cur_obligation = (cur_obligation[0], cur_obligation[1], m.group(1))
            continue
        continue

    if group == "decl":
        m = re.match(r"^  ([A-Za-z][A-Za-z0-9_]*):\s*(\S*)\s*$", raw)
        if m:
            flush_word()
            key, val = m.group(1), m.group(2)
            if key in ("vocabulary", "deprecatedAliases"):
                decl_sub = key
            else:
                decl_sub = None
                if key == "field":
                    decl_field = val
                elif key == "location":
                    decl_location = val
                elif key == "absent":
                    decl_default = val
            continue
        if decl_sub is None:
            continue
        m = re.match(r"^    -\s+word:\s*(\S+)\s*$", raw)
        if m:
            flush_word()
            cur_word = m.group(1)
            continue
        m = re.match(r"^      form:\s*(\S+)\s*$", raw)
        if m:
            cur_word_form = m.group(1)
        continue

flush_artifact()
flush_obligation()
flush_word()


def refuse(msg):
    print(f"bug-packet-resolve: {path} {msg}", file=sys.stderr)
    sys.exit(2)


if not forms:
    refuse("declares no forms:")

# An empty requirement set is a false-PASS. Refuse rather than emit it.
for form in forms:
    if not artifacts.get(form):
        refuse(f"declares form '{form}' with zero artifacts")

if not decl_field or not decl_location or not decl_default:
    refuse("has no complete declaration: block (field / location / absent)")

if decl_default not in forms:
    refuse(f"declaration absent-default '{decl_default}' is not a declared form")

if not vocab:
    refuse("declaration: block declares no vocabulary")

unknown = sorted({f for _, f, _ in vocab if f not in forms})
if unknown:
    refuse(f"declaration vocabulary maps to undeclared form(s): {unknown}")

seen = {}
for word, form, _ in vocab:
    if word in seen and seen[word] != form:
        refuse(f"declaration word '{word}' maps to both '{seen[word]}' and '{form}'")
    seen[word] = form

# BUG-042. A REDUCED form — one requiring fewer artifacts than the absent-default
# — buys its proportionality by keeping every obligation the unreduced form
# carries. If it declares none, nothing downstream can check that it kept them,
# and the guard's obligation basis would certify it on an EMPTY required set.
# That is the same false-PASS class as the zero-artifact refusal above, so it is
# refused the same way rather than emitted.
default_artifact_count = len(artifacts.get(decl_default, []))
for form in forms:
    if len(artifacts.get(form, [])) < default_artifact_count and not obligations.get(form):
        refuse(
            f"declares reduced form '{form}' "
            f"({len(artifacts.get(form, []))} artifact(s) vs the '{decl_default}' default's "
            f"{default_artifact_count}) with ZERO obligationsRetained"
        )

out = []
for form in forms:
    out.append(f"form={form}")
out.append(f"default={decl_default}")
out.append(f"field={decl_field}")
out.append(f"location={decl_location}")
for word, form, is_alias in vocab:
    out.append(f"vocab={word}|{form}")
    if is_alias:
        out.append(f"alias={word}|{form}")
for form in forms:
    for artifact_id, conditional in artifacts[form]:
        out.append(f"artifact={form}|{artifact_id}|{'yes' if conditional else 'no'}")
for form in forms:
    for obligation_id, discharged_in, attested_in in obligations.get(form, []):
        out.append(f"obligation={form}|{obligation_id}|{discharged_in}|{attested_in}")

print("\n".join(out))
PY
