# Report: [BUG-015-003] Spec 015 forward pointer to spec 056

## Summary
Artifact-only documentation drift. Spec 015 declares `SyncMode = archive | api | hybrid` and devotes an entire "API Access Strategy" section to the hybrid strategy, but ships archive-only code. Spec 056 (`twitter-api-connector`, certified `specs_hardened` 2026-05-27) is the actual implementation of the API/hybrid path and names spec 015 as its predecessor. The reverse pointer from spec 015 → spec 056 was never authored. Fix scope: three files under `specs/015-twitter-connector/` (`state.json` additive `extensions[]` entry + `spec.md` top-of-document banner and API Access Strategy section note + `design.md` top-of-document banner). No runtime code touched.

## Completion Statement
Complete to status `specs_hardened`. Workflow mode `spec-scope-hardening` with `tdd.exempt` (artifact-only). Edits land in 3 files under `specs/015-twitter-connector/`: `spec.md` (top-of-document extension-pointer banner + API Access Strategy section note placing spec 056 within ±5 lines of the heading), `design.md` (top-of-document extension-pointer note), `state.json` (additive `extensions[]` entry naming `056-twitter-api-connector`). All 5 Gherkin scenarios pass; artifact-lint passes on both parent and bug folder; change boundary respected; JSON validity confirmed. `planningOnly: true` recorded per Gate G087 because the API/hybrid implementation already lives under spec 056 (no downstream implementation spec required).

## Bug Reproduction — Before Fix
```
$ for f in specs/015-twitter-connector/state.json specs/015-twitter-connector/spec.md specs/015-twitter-connector/design.md; do grep -q "056-twitter-api-connector" "$f"; echo "$f: exit $?"; done
specs/015-twitter-connector/state.json: exit 1
specs/015-twitter-connector/spec.md: exit 1
specs/015-twitter-connector/design.md: exit 1
```
All three greps exit 1 — spec 015 carries no pointer to spec 056. Bug reproduced.

## Test Evidence

### Pre-Fix Regression Test (FAILED as required)
Agent: `bubbles.implement` (artifact-shape regression, tdd.exempt)
Executed: YES
Command + output:
```
$ for f in specs/015-twitter-connector/state.json specs/015-twitter-connector/spec.md specs/015-twitter-connector/design.md; do echo "--- $f ---"; grep -c "056-twitter-api-connector" "$f"; rc=$?; echo "exit=$rc"; done
=== PRE-FIX GREP ===
--- specs/015-twitter-connector/state.json ---
0
exit=1
--- specs/015-twitter-connector/spec.md ---
0
exit=1
--- specs/015-twitter-connector/design.md ---
0
exit=1
```
All three exited 1 against `main` (no pointer present). Captured before any edits, satisfying the "pre-fix MUST FAIL" requirement.
**Claim Source:** executed
**Phase:** implement

### Post-Fix Regression Test (PASSED)
Agent: `bubbles.implement`
Executed: YES
Command + output:
```
$ for f in specs/015-twitter-connector/state.json specs/015-twitter-connector/spec.md specs/015-twitter-connector/design.md; do echo "--- $f ---"; n=$(grep -c "056-twitter-api-connector" "$f"); echo "matches=$n exit=$([ $n -gt 0 ] && echo 0 || echo 1)"; done
=== POST-FIX GREP ===
--- specs/015-twitter-connector/state.json ---
matches=2 exit=0
--- specs/015-twitter-connector/spec.md ---
matches=2 exit=0
--- specs/015-twitter-connector/design.md ---
matches=1 exit=0
```
All three exit 0. spec.md returns 2 matches (top-of-document banner + API Access Strategy section note), meeting the ≥ 2 requirement.
**Claim Source:** executed
**Phase:** implement

### API Access Strategy Proximity Guard
Agent: `bubbles.implement`
Executed: YES
Command + output:
```
$ awk '/^## .*API Access Strategy/{h=NR} h && NR>=h-5 && NR<=h+5 && /056-twitter-api-connector/{print "match L"NR; found=1} END{exit found?0:1}' specs/015-twitter-connector/spec.md
match L76
```
"API Access Strategy" heading at L74; spec-056 reference at L76 (within ±5 lines). If the inline note were removed without leaving a 056 reference within ±5 lines of the heading, the guard would exit 1 and the regression would fail.
**Claim Source:** executed
**Phase:** implement

### Change-Boundary Guard
Agent: `bubbles.implement`
Executed: YES
Command + output:
```
$ git diff --name-only
specs/015-twitter-connector/design.md
specs/015-twitter-connector/spec.md
specs/015-twitter-connector/state.json
```
All 3 changed paths are under `specs/015-twitter-connector/`. No code, no other specs.
**Claim Source:** executed
**Phase:** implement

## Validation & Audit

### Validation Evidence
Agent: `bubbles.implement` (parent-expanded validate phase under spec-scope-hardening single-packet dispatch — same provenance model as BUG-020-007 recipe)
Executed: YES
Command + output:
```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector
... (full output captured in terminal session)
Artifact lint PASSED.

$ bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing
... (full output captured in terminal session)
Artifact lint PASSED.

$ python3 -c "import json; json.load(open('specs/015-twitter-connector/state.json')); print('OK')"
OK

$ git diff --name-only
specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/bug.md
specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/report.md
specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/scopes.md
specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/state.json
specs/015-twitter-connector/design.md
specs/015-twitter-connector/spec.md
specs/015-twitter-connector/state.json

$ bash .github/bubbles/scripts/state-transition-guard.sh specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing
... 🟢 TRANSITION PERMITTED at workflowMode=spec-scope-hardening / statusCeiling=specs_hardened
```
Both artifact-lints pass. state.json JSON validity confirmed. Change boundary respected: 3 spec-015 artifact files + 4 bug-folder artifact files; zero runtime code.
**Claim Source:** executed
**Phase:** validate

### Audit Evidence
Agent: `bubbles.implement` (parent-expanded audit phase)
Executed: YES
Command + output:
```
$ grep -c '^- \[ \]' specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/scopes.md
0
$ grep -ciE 'deferred|defer to|future scope|future work|follow-up|out of scope|will address later|postpone|skip for now|placeholder|temporary workaround' specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/scopes.md
0
$ grep '\*\*Status:\*\*' specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing/scopes.md
**Status:** Done
```
Zero unchecked DoD items. Zero deferral language. Scope status is canonical (`Done`). All 5 Gherkin scenarios map 1:1 to executed regression checks per Gate G068; all pass. The fix is real, the regression case is adversarial (would fail if any of the 3 forward pointers were removed), and zero runtime code was touched (tdd.exempt: artifact-only).
**Claim Source:** executed
**Phase:** audit

## Docs Evidence
Agent: `bubbles.implement` (parent-expanded docs phase)
Executed: YES
Documentation deltas land inside spec 015 itself (the extension-pointer notes ARE the docs update). No external doc (`docs/*.md`, README) references spec 015's API/hybrid strategy as a current implementation target, so no further sync is required. Reverse-pointer from spec 056 to spec 015 was already in place (see `specs/056-twitter-api-connector/spec.md` L11–13, which explicitly names spec 015 as predecessor).
**Claim Source:** interpreted
**Phase:** docs

## Bug Verification — After Fix
```
$ for f in specs/015-twitter-connector/state.json specs/015-twitter-connector/spec.md specs/015-twitter-connector/design.md; do n=$(grep -c "056-twitter-api-connector" "$f"); echo "$f: $n match(es)"; done
specs/015-twitter-connector/state.json: 2 match(es)
specs/015-twitter-connector/spec.md: 2 match(es)
specs/015-twitter-connector/design.md: 1 match(es)
```
All three files now carry the forward pointer. Bug verified fixed.
