#!/usr/bin/env bash
# bubbles/scripts/validation-closure-selftest.sh
#
# Hermetic selftest for the IMP-047 S-E declared-input-closure consumer.
#
# Capability: declared-input-closure-validation
#
# THE DEFECT THIS PINS CLOSED
# Change selection and result reuse used to be decided from a BASENAME PAIR:
# `X-selftest.sh` was assumed to own `X.sh` and nothing else. Editing
# `bubbles/registry/gates.yaml` therefore skipped every selftest that reads it,
# and editing `guard-lib.sh` served a cached PASS to every consumer that sources
# it, because in both cases NO BASENAME CHANGED. Both report success about work
# nothing re-examined.
#
# The three properties below are what make the replacement safe, and each case
# here is written so that reverting to the basename rule fails it:
#
#   1. A change to ANY declared input invalidates EVERY dependent check.
#   2. A dependent is found by its DECLARED INPUTS, not by its name.
#   3. An UNKNOWN closure always executes and can never be reused.
#
# The last case is about the word a reuse prints. REUSED and PASS are different
# claims — one cites an earlier verdict, the other says this run watched the work
# succeed — so printing the stronger word for the weaker claim is a suite
# reporting work it did not do.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="validation-closure-selftest"
CLOSURE="$SCRIPT_DIR/validation-closure.sh"
VALIDATOR="$SCRIPT_DIR/framework-validate.sh"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

if [[ ! -f "$CLOSURE" ]]; then
  printf '%s: consumer not found: %s\n' "$NAME" "$CLOSURE" >&2
  exit 2
fi

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

FIX="$WORK/repo"
mkdir -p "$FIX/bubbles/scripts" "$FIX/bubbles/registry"
printf '#!/usr/bin/env bash\nexit 0\n' >"$FIX/bubbles/scripts/alpha-selftest.sh"
# The name is deliberately unrelated to guard-lib: a basename rule can never
# connect these two, and the declared closure must.
printf '#!/usr/bin/env bash\nexit 0\n' >"$FIX/bubbles/scripts/zeta-name-selftest.sh"
printf '#!/usr/bin/env bash\nguard_noop() { return 0; }\n' >"$FIX/bubbles/scripts/guard-lib.sh"
printf '#!/usr/bin/env bash\nexit 0\n' >"$FIX/bubbles/scripts/live-guard.sh"
printf 'gates: []\n' >"$FIX/bubbles/registry/gates.yaml"

REG="$WORK/validation-checks.yaml"
cat >"$REG" <<'EOF'
schemaVersion: 1
generator: bubbles/scripts/generate-validation-checks.sh
source: bubbles/scripts/framework-validate.sh
checks:
  vc-alpha:
    script: bubbles/scripts/alpha-selftest.sh
    label: "Alpha selftest"
    closureComplete: true
    inputs:
    - bubbles/scripts/alpha-selftest.sh
    - bubbles/scripts/guard-lib.sh
    - bubbles/registry/gates.yaml
    commands:
    []
  vc-zeta:
    script: bubbles/scripts/zeta-name-selftest.sh
    label: "Zeta selftest"
    closureComplete: true
    inputs:
    - bubbles/scripts/zeta-name-selftest.sh
    - bubbles/scripts/guard-lib.sh
    commands:
    []
  vc-live:
    script: bubbles/scripts/live-guard.sh
    label: "Live guard"
    closureComplete: false
    inputs:
    - bubbles/scripts/live-guard.sh
    commands:
    []
derivedAt: fixture
EOF

vc() { bash "$CLOSURE" "$@" --registry "$REG" --repo-root "$FIX"; }

# --- P1. the map loads and reports what it carries --------------------------
list_out="$(vc list 2>&1)"
list_rc=$?
if [[ "$list_rc" -eq 0 ]] &&
  printf '%s\n' "$list_out" | grep -q '^vc-alpha	true	bubbles/scripts/alpha-selftest\.sh$' &&
  printf '%s\n' "$list_out" | grep -q '^vc-live	false	bubbles/scripts/live-guard\.sh$'; then
  ok "P1 the closure map loads and reports each check's completeness"
else
  bad "P1 map loads" "rc=$list_rc out=$(printf '%s' "$list_out" | tr '\n' '|')"
fi

id_out="$(vc id-for bubbles/scripts/alpha-selftest.sh 2>&1)"
if [[ "$id_out" == "vc-alpha" ]]; then
  ok "P2 a script path resolves to its stable check id"
else
  bad "P2 id-for" "out=$id_out"
fi

# --- A1. ADVERSARIAL: a registry edit invalidates its readers ----------------
# The literal defect. `gates.yaml` is named by no selftest basename, so the
# retired rule found nothing to run when it changed.
gates_affected="$(vc affected --changed bubbles/registry/gates.yaml 2>&1)"
if printf '%s\n' "$gates_affected" | grep -qx 'vc-alpha'; then
  ok "A1 changing bubbles/registry/gates.yaml invalidates the check that reads it"
else
  bad "A1 gates.yaml invalidates readers" "out=$(printf '%s' "$gates_affected" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL non-vacuity twin of A1 ---------------------------------
# A1 would also pass against an implementation that returned every check for
# every change. A check that declares no dependence on gates.yaml must be absent.
if ! printf '%s\n' "$gates_affected" | grep -qx 'vc-zeta'; then
  ok "A2 a check that does not declare gates.yaml is NOT invalidated by it"
else
  bad "A2 selection is not universal" "out=$(printf '%s' "$gates_affected" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: a shared lib invalidates consumers REGARDLESS of name --
# `zeta-name-selftest.sh` shares no basename with `guard-lib.sh`. Under the
# retired rule this pairing was invisible; the declared closure is what makes it
# visible, so this case fails the moment the basename rule returns.
lib_affected="$(vc affected --changed bubbles/scripts/guard-lib.sh 2>&1)"
if printf '%s\n' "$lib_affected" | grep -qx 'vc-zeta' &&
  printf '%s\n' "$lib_affected" | grep -qx 'vc-alpha'; then
  ok "A3 changing guard-lib.sh invalidates both consumers, whatever they are named"
else
  bad "A3 lib invalidates by declaration" "out=$(printf '%s' "$lib_affected" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: an UNKNOWN closure always runs and never reuses --------
vc reusable vc-live >/dev/null 2>&1
live_reusable_rc=$?
vc digest vc-live >/dev/null 2>&1
live_digest_rc=$?
unrelated_affected="$(vc affected --changed bubbles/scripts/nothing-declares-this.sh 2>&1)"
if [[ "$live_reusable_rc" -eq 1 && "$live_digest_rc" -eq 1 ]] &&
  printf '%s\n' "$unrelated_affected" | grep -qx 'vc-live'; then
  ok "A4 an incomplete closure is never reusable, has no digest, and is affected by any change"
else
  bad "A4 unknown closure forces execution" \
    "reusable_rc=$live_reusable_rc digest_rc=$live_digest_rc affected=$(printf '%s' "$unrelated_affected" | tr '\n' '|')"
fi

# --- A5. the plan separates RUN from REUSABLE -------------------------------
plan_out="$(vc plan --changed bubbles/registry/gates.yaml 2>&1)"
if printf '%s\n' "$plan_out" | grep -qx 'RUN	vc-alpha' &&
  printf '%s\n' "$plan_out" | grep -qx 'RUN	vc-live' &&
  printf '%s\n' "$plan_out" | grep -qx 'REUSABLE	vc-zeta'; then
  ok "A5 the plan runs the affected and the unknown, and only reuses the untouched"
else
  bad "A5 plan separation" "out=$(printf '%s' "$plan_out" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL: the digest actually tracks declared input CONTENT ------
digest_before="$(vc digest vc-alpha 2>/dev/null)"
digest_repeat="$(vc digest vc-alpha 2>/dev/null)"
printf 'gates:\n  - G001\n' >"$FIX/bubbles/registry/gates.yaml"
digest_after="$(vc digest vc-alpha 2>/dev/null)"
rm -f "$FIX/bubbles/registry/gates.yaml"
digest_absent="$(vc digest vc-alpha 2>/dev/null)"
if [[ -n "$digest_before" && "$digest_before" == "$digest_repeat" &&
  "$digest_before" != "$digest_after" && "$digest_after" != "$digest_absent" ]]; then
  ok "A6 the digest is stable, changes with input content, and changes again when the input is deleted"
else
  bad "A6 digest tracks content" \
    "before=$digest_before repeat=$digest_repeat after=$digest_after absent=$digest_absent"
fi
printf 'gates: []\n' >"$FIX/bubbles/registry/gates.yaml"

# --- A7. ADVERSARIAL: an unreadable map REFUSES, it does not resolve empty ---
# Degrading to "no check depends on anything" would silently mark the whole
# suite reusable, which is the worst possible failure mode for this file.
missing_out="$(bash "$CLOSURE" list --registry "$WORK/absent.yaml" --repo-root "$FIX" 2>&1)"
missing_rc=$?
if [[ "$missing_rc" -eq 2 ]] && printf '%s' "$missing_out" | grep -q 'cannot read the closure map'; then
  ok "A7 an unreadable closure map exits 2 instead of resolving an empty contract"
else
  bad "A7 unreadable map refused" "rc=$missing_rc out=$(printf '%s' "$missing_out" | tr '\n' '|')"
fi

# --- A8. ADVERSARIAL: no bypass flag exists ---------------------------------
bypass_out="$(bash "$CLOSURE" list --force --registry "$REG" --repo-root "$FIX" 2>&1)"
bypass_rc=$?
if [[ "$bypass_rc" -eq 2 ]] && printf '%s' "$bypass_out" | grep -q 'no bypass'; then
  ok "A8 a bypass-shaped flag is refused with exit 2"
else
  bad "A8 no bypass" "rc=$bypass_rc out=$(printf '%s' "$bypass_out" | tr '\n' '|')"
fi

# --- A9. ADVERSARIAL: a reuse prints REUSED with a receipt, never PASS -------
# Reuse is the whole product of this closure map, and the word it prints is the
# claim it makes. This reads the reuse branch of framework-validate.sh directly.
reuse_branch() {
  LC_ALL=C awk '
    /validate_cache_get/ && /then/ { inblock = 1 }
    inblock { print }
    inblock && /return 0/ { exit }
  ' "$1"
}
branch="$(reuse_branch "$VALIDATOR")"
if [[ -n "$branch" ]] &&
  printf '%s\n' "$branch" | grep -q 'REUSED:' &&
  printf '%s\n' "$branch" | grep -q '_receipt' &&
  ! printf '%s\n' "$branch" | grep -q 'PASS:'; then
  ok "A9 the reuse branch prints REUSED with a receipt id and never prints PASS"
else
  bad "A9 REUSED is not PASS" "branch=$(printf '%s' "$branch" | tr '\n' '|')"
fi

# --- A10. ADVERSARIAL non-vacuity twin of A9 --------------------------------
# A9 would pass against an extractor that matched nothing. Mutate a copy so the
# reuse branch prints PASS, and require the SAME assertion to reject it.
LC_ALL=C sed 's/REUSED: /PASS: /' "$VALIDATOR" >"$WORK/mutated-validate.sh"
mutated_branch="$(reuse_branch "$WORK/mutated-validate.sh")"
if [[ -n "$mutated_branch" ]] && printf '%s\n' "$mutated_branch" | grep -q 'PASS:'; then
  ok "A10 the same assertion rejects a reuse branch mutated to print PASS"
else
  bad "A10 REUSED assertion is non-vacuous" "mutated=$(printf '%s' "$mutated_branch" | tr '\n' '|')"
fi

# A11 the generated registry must be readable by a YAML parser.
#
# It never was. `commands:` followed by a bare `[]` on the NEXT line at the SAME
# indent is a sibling key, not a value, so every consumer that actually parses
# this file refused the whole document -- 60 occurrences. Bubbles never noticed
# because nothing here parses it; it is read with grep and awk. A downstream
# consumer's YAML check found it. A generated file nobody parses is a file whose
# syntax nobody checks, so the check belongs next to the generator.
if command -v yq >/dev/null 2>&1; then
  a11_registry="$SCRIPT_DIR/../registry/validation-checks.yaml"
  if yq -o=json '.' "$a11_registry" >/dev/null 2>&1; then
    ok "A11 generated validation-checks.yaml parses as YAML"
  else
    bad "A11 generated validation-checks.yaml parses as YAML" \
      "yq refused the document: $(yq -o=json '.' "$a11_registry" 2>&1 | head -1)"
  fi

  # Non-vacuity: the assertion must reject the exact shape that shipped.
  # Written as a heredoc rather than a printf carrying escaped newlines, because
  # the agnosticity lint's Windows drive-letter rule matches any letter followed
  # by a colon and a backslash -- which an escaped newline after a YAML key
  # produces. The rule is right; the printf was the wrong way to spell this.
  cat >"$WORK/a11-bad.yaml" <<'A11_BAD_YAML'
checks:
  a:
    commands:
    []
A11_BAD_YAML
  if yq -o=json '.' "$WORK/a11-bad.yaml" >/dev/null 2>&1; then
    bad "A11 parser rejects the shipped defect" "yq accepted 'commands:' with a bare [] on the next line"
  else
    ok "A11 parser rejects the shipped defect (non-vacuous)"
  fi
else
  ok "A11 skipped — yq not installed, no YAML parser available"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
