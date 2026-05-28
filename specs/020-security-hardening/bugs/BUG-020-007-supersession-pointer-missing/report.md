# Report: [BUG-020-007] Spec 020 supersession pointer to spec 042

## Summary
Artifact-only documentation drift. Spec 020 prescribes literal `127.0.0.1:HOST_PORT` host binds; spec 042 supersedes that prescription but the pointer was never back-propagated into spec 020. Code at `deploy/compose.deploy.yml` already matches spec 042, so no runtime change is required — only `state.json`, `spec.md`, and `design.md` under `specs/020-security-hardening/` were updated.

## Completion Statement
Complete. Workflow mode `bugfix-fastlane` with `tdd.exempt` (artifact-only). Edits land in 3 files under `specs/020-security-hardening/`: `spec.md` (inline supersession note above "Attack Surface: Network Exposure" + L279 success-metric row annotated), `design.md` (supersession note at top), `state.json` (additive `supersessions[]` entry pointing to `042-tailnet-edge-bind-pattern`). All 5 Gherkin scenarios pass; artifact-lint passes on both parent and bug folder; change boundary respected; JSON validity confirmed.

## Bug Reproduction — Before Fix
```
$ for f in specs/020-security-hardening/state.json specs/020-security-hardening/spec.md specs/020-security-hardening/design.md; do grep -q "042-tailnet-edge-bind-pattern" "$f"; echo "$f: exit $?"; done
specs/020-security-hardening/state.json: exit 1
specs/020-security-hardening/spec.md: exit 1
specs/020-security-hardening/design.md: exit 1
```
All three greps exit 1 — spec 020 carries no pointer to spec 042. Bug reproduced.

## Test Evidence

### Pre-Fix Regression Test (FAILED as required)
Agent: `bubbles.test` (artifact-shape regression, tdd.exempt)
Executed: YES
Command + output:
```
$ grep -q "042-tailnet-edge-bind-pattern" specs/020-security-hardening/state.json; echo "state.json: $?"
state.json: 1
$ grep -q "042-tailnet-edge-bind-pattern" specs/020-security-hardening/spec.md; echo "spec.md: $?"
spec.md: 1
$ grep -q "042-tailnet-edge-bind-pattern" specs/020-security-hardening/design.md; echo "design.md: $?"
design.md: 1
```
All three exited 1 against `main` (no pointer present). Captured before any edits, satisfying the "pre-fix MUST FAIL" requirement.

### Post-Fix Regression Test (PASSED)
Agent: `bubbles.test`
Executed: YES
Command + output:
```
$ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/state.json
8:      "supersededBy": "042-tailnet-edge-bind-pattern",
$ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/spec.md
48:> superseded by [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md).
62:**Mitigation:** ... see spec [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md) for the current host-bind contract.)
279:| Host port binding (SUPERSEDED by [042-tailnet-edge-bind-pattern]...) | ... |
$ grep -n "042-tailnet-edge-bind-pattern" specs/020-security-hardening/design.md
5:> been superseded by [specs/042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/design.md).
```
All three exit 0 with substantive supersession content.

### L269 Adversarial Proximity Guard
Agent: `bubbles.test`
Executed: YES
Command + output:
```
$ grep -n "100% of services bind to 127.0.0.1" specs/020-security-hardening/spec.md
$ echo "exit: $?"
exit: 1
$ grep -n "Host port binding" specs/020-security-hardening/spec.md
279:| Host port binding (SUPERSEDED by [042-tailnet-edge-bind-pattern](../042-tailnet-edge-bind-pattern/spec.md)) | Historical target was 100% of services bind to literal `127.0.0.1`; current contract is the fail-loud `${HOST_BIND_ADDRESS:?...}` form for `smackerel-core`/`smackerel-ml` and NO host `ports:` block for `postgres`/`nats` | Verify `deploy/compose.deploy.yml` invariants via `internal/deploy/compose_contract_test.go` (run as part of `./smackerel.sh test unit --go`); literal-`127.0.0.1` form is forbidden by gate G028 |
```
The bare phrase "100% of services bind to 127.0.0.1" no longer appears anywhere in spec.md. L279 row reads `Host port binding (SUPERSEDED by [042-tailnet-edge-bind-pattern]...)` with the spec-042 reference on the same line, satisfying the ±5-line proximity guard vacuously.

### Change-Boundary Guard
Agent: `bubbles.validate`
Executed: YES
Command + output:
```
$ git diff --name-only
specs/020-security-hardening/design.md
specs/020-security-hardening/spec.md
specs/020-security-hardening/state.json
```
All 3 changed paths are under `specs/020-security-hardening/`. No code, no other specs.

## Validation & Audit

### Validation Evidence
Agent: `bubbles.validate`
Executed: YES
Command + output:
```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening
... (full output captured during run)
Artifact lint PASSED.
EXIT=0

$ bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing
... (full output captured during run)
Artifact lint PASSED.
EXIT=0

$ python3 -c "import json; json.load(open('specs/020-security-hardening/state.json')); print('OK')"
OK

$ git diff --name-only
specs/020-security-hardening/design.md
specs/020-security-hardening/spec.md
specs/020-security-hardening/state.json
```
Both artifact-lints pass. state.json JSON validity confirmed. Change boundary respected (all 3 paths under `specs/020-security-hardening/`).

### Audit Evidence
Agent: `bubbles.audit`
Executed: YES
Command + output:
```
$ grep -c '^- \[ \]' specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/scopes.md
0
$ grep -ciE 'deferred|defer to|future scope|future work|follow-up|out of scope|will address later|postpone|skip for now|placeholder|temporary workaround' specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/scopes.md
0
$ grep '\*\*Status:\*\*' specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/scopes.md
**Status:** Done
```
Zero unchecked DoD items. Zero deferral language. Scope status is canonical (`Done`). All 5 Gherkin scenarios map 1:1 to executed regression checks; all pass. The fix is real, the regression case is adversarial (would fail if any of the 3 supersession pointers were removed), and zero runtime code was touched (tdd.exempt: artifact-only).

## Docs Evidence
Agent: `bubbles.docs`
Executed: YES
Documentation deltas land inside spec 020 itself (the supersession notes ARE the docs update). No external doc (`docs/*.md`, README) references the literal-`127.0.0.1` prescription from spec 020, so no further sync is required. Reverse-pointer from spec 042 to spec 020 was already in place (see `specs/042-tailnet-edge-bind-pattern/spec.md` L17, L104, L255).

## Bug Verification — After Fix
```
$ for f in specs/020-security-hardening/state.json specs/020-security-hardening/spec.md specs/020-security-hardening/design.md; do n=$(grep -c "042-tailnet-edge-bind-pattern" "$f"); echo "$f: $n match(es)"; done
specs/020-security-hardening/state.json: 1 match(es)
specs/020-security-hardening/spec.md: 3 match(es)
specs/020-security-hardening/design.md: 1 match(es)
```
All three files now carry the supersession pointer. Bug verified fixed.

