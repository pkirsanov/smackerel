# Report: BUG-061-013 — Filing and root-cause evidence

**Packet status:** `in_progress` — filed, root-caused, **no fix implemented**.
**Phase:** discovery / documentation / analysis (`bubbles.bug`).
**Repository binding:** `PREFLIGHT_COMMITTED` decision
`rb:vscode-6af2178e10192363b0e52a46fb5e0950:40`, repository `smackerel`.

---

### Red-then-green ordering index

> Added by `bubbles.implement`. This is an INDEX, not a second set of observations: every line
> below is a pointer to a block recorded in full further down. It sits at the top of the file
> because the ordering itself is the claim — the proof of absence must be readable before the
> proof of presence.

**RED stage — the required behaviour was absent before the change.**
On the SCN-01 fixture the pre-fix `assertEnvsubstWrapperContract` returned `nil`: the contract
asserted nothing, so the scenario's required rejection did not happen. Failing proof captured by
the scratch probe described in *Red-then-green method* below, exit 1, sha256
`9e2e3abf9acc74bccc1750758bc462cd4c777ee465b9bab9e936014e688569fd`:

```
ERROR PROBEVERDICT SCN-01-unlocatable-invocation => GREEN-FALSE-PASS (returned nil)
ERROR PROBEVERDICT SCN-04-conditional-form-with-call-after-go-test => GREEN-FALSE-PASS (returned nil)
```

Then, still pre-fix in effect but measured against the fixed function, the SAME fixtures are
rejected — exit 1, sha256 `c049764422438dd910fddcff2f6d4ec20d9d9ccecac4309d3efc64945f9ac169`:

```
ERROR PROBEVERDICT SCN-01-unlocatable-invocation => RED-REJECTION err="SCN-01-unlocatable-invocation:
could not LOCATE a `go test` invocation. This wrapper is in envsubstTrackedWrappers precisely
because it runs `go test`, so a zero match means the matcher is blind — the wrapper is NOT
necessarily wrong…"
ERROR PROBEVERDICT SCN-04-conditional-form-with-call-after-go-test => RED-REJECTION err="…`go test`
invocation (offset 112) appears BEFORE the `ensure_envsubst` call (offset 195); envsubst must be
ensured BEFORE any go test runs…"
```

> **A vocabulary collision worth naming, because it inverts the usual reading.** The probe's own
> verdict words are about the *guard's* verdict on a bad fixture, not about the TDD stage.
> `GREEN-FALSE-PASS` is the DEFECT — the guard passing something it should reject — and is
> therefore the TDD **red**. `RED-REJECTION` is the FIX working — the guard rejecting what it
> should — and is part of the TDD **green**. Read the verdicts as the guard's output, not as the
> stage names.

**GREEN stage — the committed regressions now pass on the same behaviour.**
Post-fix the real suite runs 7 top-level tests plus 4 sub-tests and every one is PASS, exit 0,
sha256 `813701950a8c19adddffabc7ae0926b013eb89cf0e6126aa6eae48e732a21c30`; the full untagged unit
lane exits 0 at sha256 `b6b3a1afcb6a7d342e539e591c67b1c45fa5d63520a127032f0b53db96d8c3a1`. Both
blocks are reproduced in full under *Committed regressions execute (not vacuous)* and *Lane
results*.

**The probe was temporary and is not in the delivered diff.** It lived at
`internal/deploy/zz_bug061013_prefix_probe_test.go`, was run once per side, and was deleted before
commit. `git status --porcelain` is empty at the recorded HEAD and `git log --oneline` shows the
single fix commit `40a9e942`; no file matching `zz_bug061013*` exists in the tree or in that
commit. Verified by `git show --stat 40a9e942`, reproduced under *Code Diff Evidence*, whose file
list is exactly three artifacts and one source file — the probe is absent from it.

---

### Summary

A contract test stopped checking anything and continued to report green.

`TestEnvsubstWrapperContract_LiveWrappers` locates the `go test` invocation in each tracked wrapper
with an anchored regex (`internal/deploy/envsubst_wrapper_contract_test.go:82`) in order to assert
that `ensure_envsubst` precedes it. Commit `c7667d99` rewrote that invocation in
`scripts/runtime/go-integration.sh` to a conditional-and-piped form beginning `if ! `. The anchored
regex no longer matches, and because the ordering comparison at line 108 is guarded by
`goTestIdx != nil`, a zero match falls through to `return nil` — a pass.

**No production behaviour is asserted broken.** `ensure_envsubst` runs at line 14 and `go test` at
line 76, so the ordering the contract cares about genuinely holds at HEAD. The defect is in the
detector, not the subject.

Artifacts created: `bug.md`, `spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`,
`state.json`. Source files: none modified.

### Evidence provenance

Every claim below is tagged. Three provenance classes appear in this packet, and they are kept
distinct rather than merged into a single "verified":

| Class | Meaning |
|---|---|
| `executed-this-session` | The command was run by `bubbles.bug` in this session and the output below is its real output. |
| `reported-upstream` | Observed by the BUG-061-011 regression agent and/or the operator, and **independently re-executed here**. Recorded as corroboration, not as the sole basis. |
| `not-run` | Named explicitly so its absence is visible. |

Nothing in this report rests on `reported-upstream` alone. Every load-bearing claim carries an
`executed-this-session` block.

---

### Test Evidence

No test was written or modified in this session. The evidence below is **reproduction and
root-cause** evidence for a filing, not fix-verification evidence. The fix has not been implemented,
so there is no red-then-green pair to show; the adversarial red-then-green requirement is recorded
as an unchecked DoD item in `scopes.md`.

#### REPRO-1 — The regex the contract uses finds nothing

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `grep -nE '^\s*go\s+test\b' scripts/runtime/go-integration.sh`
**Exit Code:** 1

```
=== HEAD ===
4895d446

=== GREP A: anchored regex used by the contract test ===
GREP_A_EXIT=1
```

Exit 1 with no matching line. This is the exact pattern compiled at
`internal/deploy/envsubst_wrapper_contract_test.go:82`.

#### REPRO-2 — But the invocation is plainly present

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `grep -n 'go test' scripts/runtime/go-integration.sh`
**Exit Code:** 0

```
=== GREP B: any go test occurrence ===
76:if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
83:     echo "ERROR: go-integration: could not capture go test output (tee exit ${tee_rc}); the acceptance-gate assertion cannot be trusted." >&2
120:    echo "ERROR: go-integration: go test failed (exit ${go_test_rc})." >&2
GREP_B_EXIT=0

=== ensure_envsubst call sites ===
12:# shellcheck source=scripts/runtime/_ensure_envsubst.sh
13:source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
14:ensure_envsubst "go-integration"
```

Line 76 begins `if ! `, so `^\s*` cannot reach the token. The same block records
`ensure_envsubst` at line 14 — which is why the runtime ordering is **correct** and this is a
detector defect rather than a functional one.

REPRO-1 and REPRO-2 were first observed by the BUG-061-011 regression agent and separately by the
operator (`reported-upstream`). Both were re-executed here and reproduced with identical content;
no drift was found between the reported numbers and HEAD `4895d446`.

#### REPRO-3 — And the subtest reports PASS

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose`
**Exit Code:** 0

```
=== RUN   TestEnvsubstWrapperContract_LiveWrappers
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.115s
TARGETED_UNIT_EXIT=0
```

The `go-integration.sh` subtest passes while the locator it depends on matches nothing. The
operator's task described this as taken on the regression agent's report if it could not be
cheaply confirmed; it **was** cheaply confirmable, so it is `executed-this-session` and the
command is cited above.

#### RC-1 — The zero-match branch, in source

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `grep -n 'envsubstGoTestRE\|goTestIdx' internal/deploy/envsubst_wrapper_contract_test.go`
**Exit Code:** 0

```
79:// envsubstGoTestRE matches an actual `go test` invocation. This must
82:var envsubstGoTestRE = regexp.MustCompile(`(?m)^\s*go\s+test\b`)
107:    goTestIdx := envsubstGoTestRE.FindStringIndex(src)
108:    if goTestIdx != nil && goTestIdx[0] < callIdx[0] {
110:                    wrapperName, goTestIdx[0], callIdx[0])
```

The `goTestIdx != nil &&` short-circuit at line 108 is the silent pass. The two locators above it in
the same function — source line and `ensure_envsubst` call — both return an error on absence; only
this one returns success on absence.

#### RC-2 — Origin: the invocation matched before `c7667d99`

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `git show c7667d99^:scripts/runtime/go-integration.sh` piped to the same anchored grep
**Exit Code:** 0

```
=== commit c7667d99 touching go-integration.sh? ===
c7667d99 feat(stage-1): eval-gate lane wiring, router warm-up contract, corpus grant scopes 01-04
 scripts/runtime/go-integration.sh | 76 ++++++++++++++++++++++++++--
 1 file changed, 74 insertions(+), 2 deletions(-)

=== pre-commit form of the invocation ===
55:go test "${go_test_args[@]}"
PRE_GREP_A_EXIT=0
```

At the parent commit the invocation was bare and at line start, so the anchored regex matched and the
ordering assertion was live. The BUG-061-011 eval-gate wiring is what moved it out of reach.

#### RC-3 — Blast radius: one wrapper now, all four latently

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `grep -qE '^\s*go\s+test\b' scripts/runtime/<wrapper>` over the four tracked wrappers
**Exit Code:** 0

```
=== other three wrappers: does the anchored regex still match? ===
go-unit.sh         MATCH
go-e2e.sh          MATCH
go-stress.sh       MATCH

--- go test line in each wrapper ---
go-unit.sh: 67:go test "${go_test_args[@]}" ./...
go-e2e.sh: 89:go test "${go_test_args[@]}"
go-stress.sh: 50:go test -tags stress -v -count=1 -timeout 90s -run '^TestStressReadinessCanary_Live$' ./tests/stress/readiness
```

`go-integration.sh` is the only wrapper currently blind (REPRO-1). The other three share the same
zero-match branch, so any future prefix on their invocation degrades them the same way.

#### RC-4 — Why the existing adversarial trio missed it

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `grep -n 'go test \./\.\.\.' internal/deploy/envsubst_wrapper_contract_test.go`
**Exit Code:** 0

```
184:go test ./...
204:go test ./...
224:go test ./...
```

All three adversarial fixtures write the invocation bare and at line start, so the regex matches in
every fixture the suite owns. The zero-match branch has no test at all. The suite has teeth for two
of its three locators and none for the third.

#### PRECEDENT — The rule this guard is the outlier to

**Claim Source:** executed
**Executed:** YES (this session)
**Command:** `sed -n '28,36p' .github/bubbles/scripts/release-delivery-reconciliation-guard.sh`
**Exit Code:** 0

```
#   * A RECONCILED packet that binds nothing / has malformed annotations FAILS
#     LOUD (exit 1) — a missing field must never make the gate a silent no-op.
#   * A scanned root with no docs/releases/*/features.md (the Bubbles source
#     checkout) resolves EXEMPT (exit 0), mirroring the observability gates.
SEDEXIT=0
```

The phrasing the operator asked to be cited if it reproduced does reproduce, at line 32 of this
repository's own installed guard. A second instance of the same principle sits at
`.github/bubbles/scripts/surface-reachability-guard.sh:170-172`: dropping an unresolvable declaration
*"would turn a misconfiguration into a silent no-op — the failure mode this whole contract exists to
remove."*

### Not run in this session

Recorded so the gaps are visible rather than implied:

- **`not-run`** — the full unit lane, `integration`, `e2e`, `stress`, `lint`, and `format`. This is a
  filing task; no source changed, so there is nothing for those lanes to newly prove. They belong to
  the fix scope and appear as T-06 and T-07 in the `scopes.md` Test Plan.
- **`not-run`** — any red-then-green demonstration. It cannot exist yet: the fix is unimplemented, so
  there is no post-fix state to contrast. The two adversarial DoD items in `scopes.md` carry that
  obligation, both unchecked.
- **`not-run`** — no attempt was made to move `ensure_envsubst` after line 76 to observe the guard
  failing to catch it. That mutation would prove the blindness behaviourally, but it requires editing
  `scripts/runtime/go-integration.sh`, which this task's change boundary forbids. The static proof
  (REPRO-1 plus RC-1) is sufficient to establish the branch is unreachable, and the behavioural proof
  is folded into the SCN-04 DoD item.

### Uncertainty declaration

One thing is asserted with less than full confidence, and is flagged rather than smoothed over: the
claim that the *latent* radius is all four wrappers (RC-3) is an inference from shared code, not an
observation. The three matching wrappers are checked correctly **today**. The inference is that they
sit behind the same `goTestIdx != nil` branch and would degrade identically given a prefix — which
follows from RC-1, but no wrapper other than `go-integration.sh` has actually been observed to
degrade.

### Completion Statement

**Nothing is delivered by this packet, and no fix is claimed.** What is established:

- The defect reproduces at HEAD `4895d446` by three independently executed commands (REPRO-1,
  REPRO-2, REPRO-3), each with its own exit code recorded.
- The root cause is located in source at `internal/deploy/envsubst_wrapper_contract_test.go:82` and
  `:108` (RC-1), with its origin traced to `c7667d99` (RC-2).
- The blast radius is measured (RC-3) and the reason the existing adversarial suite did not catch it
  is measured (RC-4).
- Severity is **guard integrity, not user impact**. `ensure_envsubst` at line 14 precedes `go test`
  at line 76, so the ordering the contract exists to protect genuinely holds. The detector is what
  is defective.

The packet status is `in_progress` with every DoD item in `scopes.md` unchecked and every
human-acceptance item in `uservalidation.md` unchecked. No source file was modified; no commit and no
push was made.

---

# Implementation phase — `bubbles.implement`

**Phase:** implement (`bubbles.implement`).
**Repository binding:** `PREFLIGHT_COMMITTED` decision
`rb:vscode-6af2178e10192363b0e52a46fb5e0950:66`, revision 66, repository `smackerel`.
**HEAD at start of phase:** `d08013e6`.

> The filing section above is preserved verbatim. It was written at HEAD `4895d446`; this phase
> re-verified the defect at `d08013e6` before changing anything, and the reproduction was identical.

### Summary

Both composing changes from `design.md` landed together in the single allowed file, and nothing
else was touched.

**(a) Zero match is now a hard failure.** The `goTestIdx != nil &&` short-circuit was replaced by an
explicit absence branch, giving the `go test` locator the same shape the source-line and call
locators already had. This is the load-bearing half: it converts every future silent pass into a
loud one across all four tracked wrappers and any wrapper added later, not just the one wrapper that
is blind today.

**(b) The matcher now recognises the real shell forms.** `envsubstGoTestRE` accepts an enumerated,
repeatable set of leading tokens — `if`, `elif`, `then`, `while`, `until`, `&&`, `||`, `|`, `!` —
so `scripts/runtime/go-integration.sh:76` (`if ! go test … | tee …`) is located and its ordering is
genuinely compared. The enumeration is deliberately bounded, not open: `env VAR=x go test`,
`timeout N go test` and `x=$(go test …)` still do not match, and the updated comment says so. That
residual blindness is now safe precisely because (a) makes a miss loud — which is why (b) alone
would have been the wrong fix, and why the ordering of the two matters.

The stale comment at lines 79-81 (`Whitespace-leading is OK` as the full allowance) was replaced,
since leaving it would have made the file assert something false about its own regex.

### Pre-change re-verification

**Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

```
$ git rev-parse --short HEAD
d08013e6
$ grep -cE '^[[:space:]]*go[[:space:]]+test\b' scripts/runtime/go-integration.sh
0
$ for w in go-unit go-e2e go-stress; do grep -cE '^[[:space:]]*go[[:space:]]+test\b' scripts/runtime/$w.sh; done
go-unit: 1
go-e2e: 1
go-stress: 2
$ grep -n 'go test' scripts/runtime/go-integration.sh
76:if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
$ grep -n 'ensure_envsubst' scripts/runtime/go-integration.sh
13:source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
14:ensure_envsubst "go-integration"
```

Zero matches in `go-integration.sh` against the exact pattern the contract compiled, while the other
three wrappers match. `ensure_envsubst` at 14 precedes `go test` at 76, confirming the runtime
ordering is correct and the defect is in the detector — the severity claim in the filing holds.

### Red-then-green method, and why a scratch probe was used

The two adversarial DoD items require the verdict of `assertEnvsubstWrapperContract` on the SAME
fixture on both sides of the change. The committed regressions cannot supply the pre-fix half: they
assert the post-fix behaviour, so running them against the old function shows a failing assertion
rather than the silent pass itself, and "the test was red before" is a weaker claim than "the
function returned nil on this exact input".

A temporary file, `internal/deploy/zz_bug061013_prefix_probe_test.go`, therefore called the real
function in the real package and printed its verdict. It was run once before the change and once
after, with identical fixtures and an identical command, and **deleted before commit**. It appears
in no delivered diff. Both runs are recorded below with their full-output sha256, so either is
re-derivable.

#### RTG-1 — Zero-match branch: `nil` → error

**Claim Source:** executed · **Executed:** YES (both sides, this session)

PRE-FIX, exit 1, sha256 `9e2e3abf9acc74bccc1750758bc462cd4c777ee465b9bab9e936014e688569fd`:

```
$ timeout 900 ./smackerel.sh test unit --go --go-run TestZZBug061013PreFixProbe
ERROR PROBEVERDICT SCN-01-unlocatable-invocation => GREEN-FALSE-PASS (returned nil)
ERROR PROBEVERDICT SCN-04-conditional-form-with-call-after-go-test => GREEN-FALSE-PASS (returned nil)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.029s
```

POST-FIX, exit 1, sha256 `c049764422438dd910fddcff2f6d4ec20d9d9ccecac4309d3efc64945f9ac169`:

```
$ timeout 900 ./smackerel.sh test unit --go --go-run TestZZBug061013PreFixProbe
ERROR PROBEVERDICT SCN-01-unlocatable-invocation => RED-REJECTION err="SCN-01-unlocatable-invocation:
could not LOCATE a `go test` invocation. This wrapper is in envsubstTrackedWrappers precisely because
it runs `go test`, so a zero match means the matcher is blind — the wrapper is NOT necessarily
ERROR PROBEVERDICT SCN-04-conditional-form-with-call-after-go-test => RED-REJECTION err="SCN-04-conditional-form-with-call-after-go-test:
`go test` invocation (offset 112) appears BEFORE the `ensure_envsubst` call (offset 195); envsubst
must be ensured BEFORE any go test runs that may shell out to s
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.013s
```

Both probe runs exit 1 by construction — the probe ends in a deliberate `t.Errorf` so the package
output is printed. The exit code is not the signal; the verdict lines are.

#### RTG-2 — Restored ordering check: the error CHANGES CLASS, not just presence

The SCN-04 line above is the one that distinguishes a real fix from a cosmetic one. Post-fix it
returns the **ORDERING** error at offset 112 — a byte position inside the `if ! go test …` line that
the pre-fix pattern could not reach at all. Had the widening matched something inert, the same
fixture would have produced the *locator* error, and the committed test asserts the ordering
substring specifically, so that outcome would be red.

### Committed regressions execute (not vacuous)

**Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
=== RUN   TestEnvsubstWrapperContract_HelperExistsAndIsExecutable
--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
=== RUN   TestEnvsubstWrapperContract_LiveWrappers
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.014s
```

Nine sub-tests RUN and nine PASS. The `=== RUN` lines matter more than the PASS lines here: a
regression that is never selected passes vacuously, and a vacuous check is the exact defect this
packet exists to remove. Both new tests appear, so neither is inert. The same command was also run
unfiltered under `evidence-capture.sh`, exit 0, sha256
`813701950a8c19adddffabc7ae0926b013eb89cf0e6126aa6eae48e732a21c30`.

The three pre-existing adversarial sub-tests still pass with their substring assertions untouched in
the diff, so existing detection was not blunted. The third of them uses the bare `go test ./...`
form, which additionally confirms the widened pattern still matches the narrow shape it always did.

### Change boundary

**Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

```
$ git diff --stat
 internal/deploy/envsubst_wrapper_contract_test.go | 100 +++++++++++++++++++++-
 1 file changed, 96 insertions(+), 4 deletions(-)

$ git diff --stat -- scripts/runtime/
$ git diff -- scripts/runtime/ | wc -l
scripts/runtime diff lines: 0

$ git status --porcelain
 M internal/deploy/envsubst_wrapper_contract_test.go
```

`scripts/runtime/` is byte-identical to HEAD. This is recorded as evidence rather than assumed
because the cheapest route to a green lane was to revert `go-integration.sh:76` to a bare
`go test` — which would have satisfied the old regex, re-broken the eval gate the conditional form
serves, and left the zero-match hole open for the next wrapper. The detector was fixed; the subject
was not touched.

### Code Diff Evidence

**Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

The change landed as a single commit, `40a9e942`.

```
$ git show --stat --oneline 40a9e942
40a9e942 fix(BUG-061-013): a wrapper-contract zero match is now a hard failure
 internal/deploy/envsubst_wrapper_contract_test.go  | 100 ++++++++-
 .../report.md                                      | 242 +++++++++++++++++++++
 .../scopes.md                                      | 236 +++++++++++++++++++-
 3 files changed, 563 insertions(+), 15 deletions(-)
```

`--stat` elides the two long packet paths. Unelided, so the delivered set is unambiguous — one
source file and two packet artifacts, and nothing else:

```
$ git show --name-only --format='' 40a9e942
internal/deploy/envsubst_wrapper_contract_test.go
specs/061-conversational-assistant/bugs/BUG-061-013-wrapper-contract-zero-match-silent-pass/report.md
specs/061-conversational-assistant/bugs/BUG-061-013-wrapper-contract-zero-match-silent-pass/scopes.md
```

No path under `scripts/runtime/` appears, and no `zz_bug061013*` probe file appears. Both absences
are load-bearing and are the reason the full file list is quoted rather than summarised.

#### Change 1 — the matcher was widened onto the real invocation form

```diff
 // envsubstGoTestRE matches an actual `go test` invocation. This must
-// appear AFTER the ensure_envsubst call. Whitespace-leading is OK; a
-// trailing `\` to indicate continuation is OK.
-var envsubstGoTestRE = regexp.MustCompile(`(?m)^\s*go\s+test\b`)
+// appear AFTER the ensure_envsubst call.
+//
+// The token is anchored to a line start, but whitespace is NOT the only
+// permitted prefix. Real wrappers put shell syntax in front of the command:
+// scripts/runtime/go-integration.sh runs it as `if ! go test … | tee …`, a
+// form the eval gate requires. A pattern that allowed only leading
+// whitespace matched nothing there, and before BUG-061-013 a zero match
+// returned nil — so the ordering check silently asserted nothing.
+//
+// The permitted prefixes are therefore enumerated: `if`, `elif`, `then`,
+// `while`, `until`, `&&`, `||`, `|`, and `!`, repeatable in any order. The
+// enumeration is deliberately bounded rather than open. Forms outside it
+// (`env VAR=x go test`, `timeout N go test`, `x=$(go test …)`) still do not
+// match — and that is safe now only because the zero-match branch below
+// makes such a miss LOUD. Widening a blind matcher buys one round; the
+// absence branch is what makes the next miss visible.
+var envsubstGoTestRE = regexp.MustCompile(`(?m)^[^\S\n]*(?:(?:if|elif|then|while|until|&&|\|\||\||!)[^\S\n]+)*go[^\S\n]+test\b`)
```

The stale comment was replaced in the same hunk. Leaving `Whitespace-leading is OK` in place would
have made the file assert something false about its own regex.

#### Change 2 — a zero match became a hard failure

```diff
        goTestIdx := envsubstGoTestRE.FindStringIndex(src)
-       if goTestIdx != nil && goTestIdx[0] < callIdx[0] {
+       if goTestIdx == nil {
+               // Absence is a LOCATOR failure, not compliance. Every wrapper reaching
+               // here is in envsubstTrackedWrappers, and the documented entry
+               // condition for that list is "runs `go test`" — so the list and the
+               // matcher contradict each other, and it is the matcher that is
+               // fallible. Returning nil here is the BUG-061-013 silent pass.
+               return fmt.Errorf("%s: could not LOCATE a `go test` invocation. This wrapper is in envsubstTrackedWrappers precisely because it runs `go test`, so a zero match means the matcher is blind — the wrapper is NOT necessarily wrong and MUST NOT be rewritten to satisfy the pattern. The matcher may need widening: extend envsubstGoTestRE to recognise the invocation form actually in use",
+                       wrapperName)
+       }
+       if goTestIdx[0] < callIdx[0] {
                return fmt.Errorf("%s: `go test` invocation (offset %d) appears BEFORE the `ensure_envsubst` call (offset %d); envsubst must be ensured BEFORE any go test runs that may shell out to scripts/commands/config.sh",
                        wrapperName, goTestIdx[0], callIdx[0])
        }
```

This is the load-bearing half. Splitting the compound condition does not merely reorder it: the
`!= nil` conjunct was the silent pass, because absence took the same path as compliance. The
locator now behaves like the two locators above it in the same function, both of which already
errored on absence — so the change removes an internal inconsistency rather than adding a rule.

The remaining hunks are additive: the two new adversarial sub-tests
(`…AdversarialRejectsUnlocatableInvocation`, `…AdversarialRejectsConditionalCallAfterGoTest`) and
the two new bullets in the file's header contract comment. No existing assertion or error substring
was removed or relaxed — verifiable in the diff above, which shows deletions only on the regex line
and its stale comment, and on the compound `if`.

### Lane results

| Lane | Command | Exit | sha256 of full output |
|---|---|---|---|
| Contract suite | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose` | 0 | `813701950a8c19adddffabc7ae0926b013eb89cf0e6126aa6eae48e732a21c30` |
| Full untagged unit lane (T-07) | `./smackerel.sh test unit --go` | 0 | `b6b3a1afcb6a7d342e539e591c67b1c45fa5d63520a127032f0b53db96d8c3a1` |
| Lint | `./smackerel.sh lint` | 0 | `99b1b3b48cd86513f07beda05d11a96c8498ace02b972caa6c8c0f057ec77367` |
| Format check | `./smackerel.sh format --check` | 0 | `67a766acc6e1887340721dbace5ce85cb3216d6f68ba380859df43635914630d` |
| Integration (T-06) | `./smackerel.sh test integration` | <!-- T06-EXIT --> | <!-- T06-SHA --> |

```
$ timeout 1800 ./smackerel.sh test unit --go
exit: 0
lines: 209
ok      github.com/smackerel/smackerel/tests/observability      (cached)
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
ok      github.com/smackerel/smackerel/tests/unit/clients       (cached)
?       github.com/smackerel/smackerel/web/pwa  [no test files]
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

<!-- T06-EVIDENCE-BLOCK -->

### Not run in this phase

Recorded so the gaps stay visible:

- **`not-run`** — `e2e`, `stress`, and `load`. Stated in the `scopes.md` Test Plan with its reason:
  the change is confined to a `_test.go` file in `internal/deploy` with no production consumer, so
  there is no runtime surface for those categories to exercise.
- **`not-run`** — no mutation of `scripts/runtime/go-integration.sh` to observe the live subtest
  failing. The change boundary forbids it. The equivalent behavioural proof is RTG-2, which applies
  the same mutation to a fixture carrying line 76's exact shape.

### Deviation from terminal discipline, disclosed

One supplementary readout piped `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract'
--verbose` through `grep` to surface the per-sub-test `=== RUN` / `--- PASS` lines, because the
wrapper always runs `./...` and the per-package output buried them. That is a filtering pipe, which
terminal discipline forbids. It is disclosed rather than omitted. The same command was ALSO run
unfiltered under `evidence-capture.sh` (exit 0, sha256 `813701950a8c…`), so no claim in this report
rests on the filtered invocation alone — the filtered output is a view of a run that is separately
recorded in full.

### Uncertainty declaration

The bounded prefix enumeration in `envsubstGoTestRE` is a judgement, not a proof. It covers the
forms present in the four wrappers today and the conditional/pipeline family generally, but it
cannot cover shapes nobody has written yet. `design.md` option (c) — semantic parsing — remains the
strongest long-term answer and was deliberately not taken. What has changed is that this residual
blindness is no longer dangerous: a miss now fails loudly instead of passing silently, so the next
unanticipated form will announce itself rather than quietly disabling the check.

### Completion Statement

Delivered: the two composing changes from `design.md`, both regressions, and the updated comment —
in one file, with `scripts/runtime/` untouched. Red-then-green is recorded on both sides for both
adversarial properties, from real execution in this session.

**Not claimed:** validation, audit, and human acceptance. `uservalidation.md` items remain unchecked
and no `## Human Acceptance Record` was authored — G136 is operator-only, and an agent checking
those items would fabricate the acceptance the gate exists to require. The packet status is not set
to `done` by this phase.

---

# Test phase — `bubbles.test`

**Phase:** test (`bubbles.test`).
**Repository binding:** `PREFLIGHT_COMMITTED` decision
`rb:vscode-6af2178e10192363b0e52a46fb5e0950:6`, revision 6, repository `smackerel`.
**HEAD at start of phase:** `6a837a89`. Fix under test: commit `40a9e942`.

> The two sections above are preserved verbatim. This phase added no source change: the only
> non-`specs/` path in the working tree at start and finish is unchanged, and the phase's job was to
> EXECUTE the Test Plan rather than to author code. Every lane below was run in THIS session; no
> figure is carried over from the implement phase, and where a lane was run before, it was re-run
> rather than cited.

> **Attribution split — read this before any lane figure below.** "This session" is not the same
> claim as "this agent". Rows **T-01 through T-05 and T-07** were executed by `bubbles.test`. Rows
> **T-06 and T-08** were executed by the ORCHESTRATOR (`bubbles.goal`) in this same session, and
> `bubbles.test` OBSERVED the captures rather than producing them: each time this agent returned,
> its process ended and took the running lane with it, so the two multi-thousand-line Docker lanes
> could only be carried by the orchestrator. Every T-06/T-08 block below is labelled accordingly.
> The distinction is recorded rather than smoothed over, because a report that says "I ran it" when
> another process ran it is the same species of small untruth this packet exists to remove — and
> unlike the lane figures, that one would be invisible to any gate.

### Test Plan row fixed before execution — T-08 was unresolvable

The guard blocked with `Test Plan references non-existent or non-resolvable file: go-e2e.sh` and
`1 of 8 test files from Test Plan DO NOT EXIST`. The file **does** exist. The failure was a wording
defect in the row, not a missing file, and the distinction matters because the two have opposite
remedies — one is fixed by writing a path, the other by writing a test.

`state-transition-guard.sh` Check 8 walks each Test Plan row's backticked spans in order and takes
the **first** one that looks like a path (`guards`-side helper `_check8_candidate_from_block`, then
`break`). T-08's Description column opened with a bare `` `go-e2e.sh` ``, so that span won before the
File column's correct `` `scripts/runtime/go-e2e.sh` `` was ever reached. A basename-only token is
then resolved by searching `$feature_dir/../..` — the *spec* folder, not the repo root — where a
runtime wrapper cannot be found, so it resolved to nothing.

The fix was to write the repo-relative path in the Description column so the first span already
resolves. The row was **not** weakened and **not** removed; its category, command, and rationale are
untouched. Before → after on the same guard invocation:

```
--- Check 8: Test File Existence ---            (before)
🔴 BLOCK: Test Plan references non-existent or non-resolvable file: go-e2e.sh
🔴 BLOCK: 1 of 8 test files from Test Plan DO NOT EXIST

--- Check 8: Test File Existence ---            (after)
✅ PASS: Test file exists: scripts/runtime/go-e2e.sh
failureCount: 14 → 12   failedChecks: [Check-8-contract,Check-8-file-existence] → []
```

### Lane results — every row of the Test Plan, executed this session

`Ran by` is a column rather than a footnote so no row can be read without its provenance.

| Row | Command | Ran by | Exit | Verdict | sha256 of full output |
|---|---|---|---|---|---|
| T-01 / T-02 | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation' --verbose` | `bubbles.test` | 0 | PASS | `08330fe1df5faa18eab3c3039d5b0f1b7f66bf5bda4ed357b028005f1487c6be` |
| T-03 | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_LiveWrappers' --verbose` | `bubbles.test` | 0 | PASS (4/4 subtests) | `f63fb2090fde1c79fbf11711784f72e3f43ecbd007a3b881910df773a8c72355` |
| T-04 | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest' --verbose` | `bubbles.test` | 0 | PASS | (direct run; verdict lines below) |
| T-05 | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_Adversarial' --verbose` | `bubbles.test` | 0 | PASS (5/5 fixtures) | (direct run; verdict lines below) |
| T-06 | `./smackerel.sh test integration` | **orchestrator (`bubbles.goal`)** | 0 | PASS | `a3230783b2c46e98b21c9dd5d93aa26bdac0ae0fbe8b196a8c24cf4f611c531a` |
| T-07 | `./smackerel.sh test unit` | `bubbles.test` | 0 | PASS | `3f8dd248bd37fc7cf28065c8d479aae84e047af0e4c84b866fe4f0ba65fe9d35` |
| T-08 | `./smackerel.sh test e2e` | **orchestrator (`bubbles.goal`)** | 0 | PASS | `eb5e8fa3c316b1f2212bf96e721d54779b3dc07a2b159513da4f1ab084f6cea4` |

#### T-01 / T-02 — the regression EXECUTES, it does not pass vacuously

**Claim Source:** executed · **Exit Code:** 0

The `=== RUN` line is the load-bearing half, and it is why the raw output was read rather than the
exit code alone. `--go-run` narrows an otherwise repo-wide `go test ./...`, so exit 0 is also what a
run that selected *nothing* would produce — which is the same silent-pass shape this bug is about.
One `=== RUN` for the target name, and exactly one across the whole run, settles it:

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation' --verbose
total lines: 553
=== RUN count: 1
272:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
273:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
...
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
T01_T02_EXIT=0
```

The same command was re-run unfiltered under `evidence-capture.sh` so nothing here rests on a read
of a filtered view: exit 0, 520 lines, sha256 `08330fe1df5f…`.

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify 08330fe1df5faa18eab3c3039d5b0f1b7f66bf5bda4ed357b028005f1487c6be -- ./smackerel.sh test unit --go --go-run TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation --verbose -->

#### T-03 — all four tracked wrappers, each subtest named

**Claim Source:** executed · **Exit Code:** 0

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_LiveWrappers' --verbose
T03_EXIT=0
302:=== RUN   TestEnvsubstWrapperContract_LiveWrappers
303:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
304:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
305:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
306:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
307:--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
308:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
309:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
310:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
311:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
313:ok          github.com/smackerel/smackerel/internal/deploy  0.023s
```

Four named subtests, four PASS lines. A green `go-integration.sh` subtest is *by itself* worthless
here — that is precisely the reading the defect produced — so it is T-04 below, not this block, that
makes it evidence.

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify f63fb2090fde1c79fbf11711784f72e3f43ecbd007a3b881910df773a8c72355 -- ./smackerel.sh test unit --go --go-run TestEnvsubstWrapperContract_LiveWrappers --verbose -->

#### T-04 — the widened matcher landed on the real invocation, not on something inert

**Claim Source:** executed · **Exit Code:** 0

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest' --verbose
lines=553
271:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
273:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
276:ok      github.com/smackerel/smackerel/internal/deploy  0.079s
554:T04_EXIT=0
```

This fixture carries line 76's exact `if ! go test … | tee …` shape with `ensure_envsubst` moved
*after* it, and the committed test asserts the **ordering** substring specifically. A widening that
had matched some inert span would surface the *locator* error instead and go red. That is what
upgrades T-03's green `go-integration.sh` subtest from a bare assertion into a checked one.

#### T-05 — the pre-existing trio was not blunted

**Claim Source:** executed · **Exit Code:** 0

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_Adversarial' --verbose
256:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
257:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
258:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
259:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
261:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
262:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
263:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
264:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
266:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
268:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
271:ok      github.com/smackerel/smackerel/internal/deploy  0.037s
549:T05_EXIT=0
```

Five fixtures under one prefix: the three that predate this fix and the two it added. The third,
`RejectsCallAfterGoTest`, is the one worth naming — its fixture uses the bare `go test ./...` form,
so its pass confirms the widened pattern still matches the narrow shape it always matched.

#### T-07 — the full unit lane, which executes `go-unit.sh`

**Claim Source:** executed · **Exit Code:** 0

**Redaction disclosed:** three lines in the tail window named an absolute checkout path under the
operator's home directory, which this repository forbids committing; `<repo-root>` is substituted.
The `sha256` covers the true unedited output, so the `verify` line re-derives it.

```
# BUG-061-013 T-07: full unit lane (executes go-unit.sh)
$ ./smackerel.sh test unit
exit: 0
lines: 441
sha256: 3f8dd248bd37fc7cf28065c8d479aae84e047af0e4c84b866fe4f0ba65fe9d35
--- first 20 ---
oom-preflight: OK — 35069 MB available (need 6000 MB; swap used 67 MB).
disk-preflight: OK — C: 68 GB free (need 40 GB), WSL / 462 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
[go-unit] envsubst missing — installing gettext-base
+ apt-get install -y --no-install-recommends gettext-base
--- omitted 401 line(s); sha256 above covers the full output ---
--- last 20 ---
1..2
# tests 2
# pass 2
# fail 0
# skipped 0
PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)
[test unit] -> bash <repo-root>/tests/unit/web/spec_077_discovery_convention_test.sh
PASS: spec_077_discovery_convention_test (TP-077-02-01 / SCN-077-A02)
[test unit] -> bash <repo-root>/tests/unit/web/spec_077_no_stub_bodies_test.sh
PASS: spec_077_no_stub_bodies_test (TP-077-03-06 / SCN-077-A08)
[test unit] shell unit tests in tests/unit/web/ finished OK
[test unit] running 1 shell unit test(s) from tests/unit/docs/
[test unit] -> bash <repo-root>/tests/unit/docs/spec_077_test_category_parity_test.sh
PASS: spec_077_test_category_parity_test (TP-077-02-03 / SCN-077-A06)
[test unit] shell unit tests in tests/unit/docs/ finished OK
```

The first-20 window is not filler: `ensure_envsubst go-unit` appears at line 5, **before** any
`go test` line, which is the live wrapper exhibiting at runtime the ordering the contract asserts
statically. `# fail 0` and exit 0 carry the lane verdict.

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify 3f8dd248bd37fc7cf28065c8d479aae84e047af0e4c84b866fe4f0ba65fe9d35 -- ./smackerel.sh test unit -->

#### T-06 — the integration lane, which executes `go-integration.sh`

**Claim Source:** executed — by the ORCHESTRATOR (`bubbles.goal`) in this session; OBSERVED, not
produced, by `bubbles.test` · **Exit Code:** 0

**Redaction disclosed:** the `config-validate:` line named an absolute checkout path under the
operator's home directory, which this repository forbids committing; `<repo>` is substituted. The
`sha256` covers the true unedited output, so the `verify` line re-derives it from a real run.

```
# BUG-061-013 T-06: integration lane (go-integration.sh)
$ timeout 2100 ./smackerel.sh test integration
exit: 0
lines: 10252
sha256: a3230783b2c46e98b21c9dd5d93aa26bdac0ae0fbe8b196a8c24cf4f611c531a
--- first 20 ---
oom-preflight: OK — 33169 MB available (need 6000 MB; swap used 182 MB).
disk-preflight: OK — C: 71 GB free (need 40 GB), WSL / 460 GB free (need 25 GB).
config-validate: <repo>/config/generated/test.env.tmp.1929798 OK
Smackerel pre-flight resource check: OK
  RAM  available: 32893 MB (required >= 6000 MB)
  Disk available: 471218 MB / 460.2 GB (required >= 15 GB)
Preparing disposable test stack...
Waiting for configured test host ports to be released after project-scoped cleanup (timeout 180s)...
Configured test host ports became free after 42.3s.
Building disposable test stack images before up (freshness convention)...
--- failure-shaped lines from the omitted region ---
        ERROR: model envelope validation failed (spec 045 FR-045-002): model envelope exceeded: LLM_MODEL="bug-045-fixture-llm-20gib" requires 20480 MiB ... but OLLAMA_MEMORY_LIMIT="8G" resolves to 8192 MiB
        ERROR: config-generate-time validation failed for env=test (see above)
--- omitted 10212 line(s); sha256 above covers the full output ---
--- last 20 ---
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-intent-compiler-provider-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-nats-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
```

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify a3230783b2c46e98b21c9dd5d93aa26bdac0ae0fbe8b196a8c24cf4f611c531a -- timeout 2100 ./smackerel.sh test integration -->

**The two `ERROR:` lines are a negative fixture, and that is checked here rather than asserted.**
The capture lifts them out of the omitted region precisely so they cannot be skimmed past, so this
phase read the source instead of trusting the shape. In `tests/integration/config_validate_test.go`,
`buildOversizedEnvFile` rewrites `LLM_MODEL` and `OLLAMA_MODEL` to `bug-045-fixture-llm-20gib`,
declares that model at `weights_mib: 20480` in `ML_MODEL_MEMORY_PROFILES_JSON`, and **pins**
`OLLAMA_MEMORY_LIMIT=8G` so the fixture stays oversized regardless of the live limit. The test that
consumes it, `TestConfigValidate_AC5c_BinaryRejectsOversizedModel`, then runs the built binary and
asserts `exitCode == 1` — `t.Fatalf` on anything else — plus three required substrings:
`OLLAMA_MEMORY_LIMIT`, `bug-045-fixture-llm-20gib`, and `20480`. Those are exactly the tokens in the
lifted lines. The inference runs the *opposite* way from the intuitive one: a run in which these
`ERROR:` lines were ABSENT would mean the rejection stopped firing and the lane would go **red**.
Their presence is the assertion passing. The lane verdict is the `exit: 0` above, and the teardown
window shows the disposable stack removed rather than leaked.

#### T-08 — the e2e lane, which executes `scripts/runtime/go-e2e.sh`

**Claim Source:** executed — by the ORCHESTRATOR (`bubbles.goal`) in this session; OBSERVED, not
produced, by `bubbles.test` · **Exit Code:** 0

```
# BUG-061-013 T-08: e2e lane (scripts/runtime/go-e2e.sh)
$ timeout 3300 ./smackerel.sh test e2e
exit: 0
lines: 4528
sha256: eb5e8fa3c316b1f2212bf96e721d54779b3dc07a2b159513da4f1ab084f6cea4
--- first 20 ---
Running isolated lifecycle shell E2E: test_timeout_process_cleanup.sh
=== BUG-031-004-SCN-002: regression detects surviving child work ===
Detector reported surviving child work: ...
PASS: BUG-031-004-SCN-002
=== BUG-031-004-SCN-001: E2E interruption terminates child processes ===
Nested E2E runner returned nonzero after interruption: -1
PASS: BUG-031-004-SCN-001
=== BUG-031-009-SCN-001/002: interrupted Docker Go runner is reaped before teardown ===
--- failure-shaped lines from the omitted region ---
FAIL: Services did not become healthy within 8s
--- omitted 4488 line(s); sha256 above covers the full output ---
--- last 20 ---
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
PASS: go-e2e-corpus-enforce
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
```

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify eb5e8fa3c316b1f2212bf96e721d54779b3dc07a2b159513da4f1ab084f6cea4 -- timeout 3300 ./smackerel.sh test e2e -->

**The `FAIL:` line belongs to a deliberately-broken stack — with one correction to how it was
described to this phase.** It was handed over as appearing "inside an adversarial lifecycle
scenario". It is adversarial, but it is **not** one of the lifecycle scenarios visible in the
first-20 window (`BUG-031-004-SCN-001/002`, `BUG-031-009-SCN-001/002`); attributing it to those
would have been a plausible guess that the source does not support. The string is emitted by
`e2e_wait_healthy` at `tests/e2e/lib/helpers.sh:103`, whose timeout is a parameter — and the literal
`8s` is what pins it, because every other call site in the tree passes `120`
(`helpers.sh:47`, `helpers.sh:53`, `run_all.sh:81`, `test_persistence.sh:25`, `:53`). The sole
`e2e_wait_healthy 8` is `tests/e2e/test_postgres_readiness_gate.sh:24`, the readiness-gate canary
for scenario `SCN-002-BUG-002-001`. That script starts the stack, then runs
`smackerel_compose "$TEST_ENV" stop postgres` **on purpose**, and asserts the gate refuses:

```
if [ "$READINESS_EXIT" -eq 0 ]; then
    e2e_fail "Readiness gate passed even though postgres was stopped"
fi
e2e_assert_contains "$READINESS_OUTPUT" "postgres readiness" ...
echo "PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=$READINESS_EXIT)"
```

So the `FAIL:` line is the required output of a stack that was broken on purpose, and — as with
T-06 — its ABSENCE is what would fail the canary. The short 8-second budget is itself the tell: a
gate that is *expected* to fail is not given 120 seconds to do it.

**Neither lane is suspect.** Both exited 0, both tore their disposable stacks down, and T-08's tail
carries `PASS: go-e2e-corpus-enforce`. Had either failure-shaped line turned out to be a real
failure on inspection, the correct action would have been to say so and treat the lane as suspect
rather than lean on the exit code — the exit code is the weaker signal precisely because a lane can
exit 0 while a check inside it silently declines to run, which is this bug's own failure mode.

### Completion Statement — test phase

All eight Test Plan rows ran to a verdict and all eight exited 0. Five rows (T-01/T-02, T-03, T-04,
T-05, T-07) were executed by `bubbles.test`; the two multi-thousand-line Docker lanes (T-06, T-08)
were executed by the orchestrator (`bubbles.goal`) in this session and observed here. No lane was
cited from the implement phase, and no row is recorded as agent-run that was not.

Two DoD items were added to `scopes.md` for the regression-E2E coverage the guard requires, and both
are checked against the T-08 capture above: the scenario-specific item covers `go-e2e.sh`, the
tracked wrapper this contract governs, and the broader item covers the full e2e lane.

**What this phase did NOT do, recorded so the gaps stay visible:**

- **`go-stress.sh` remains statically asserted only.** It is the one tracked wrapper with no lane
  executing it here. The reason is in the `scopes.md` Test Plan: it was never a blind wrapper, so a
  stress lane would add cost without signal. Recorded, not closed.
- **The implement-phase `<!-- T06-EXIT -->` / `<!-- T06-SHA -->` / `<!-- T06-EVIDENCE-BLOCK -->`
  placeholders were left in place.** They sit in the implement phase's own section, and that phase's
  T-06 run is a *different* run from this one — sha `579e9a14…`, 10,355 lines, versus this phase's
  `a3230783…`, 10,252 lines. Filling another phase's record with this phase's figures would misdate
  the evidence, so they are flagged for their owner instead of quietly overwritten.
- **`certification.pendingGates` in `state.json` still reads "7 of the 8 … phases remain
  unexecuted: test, regression, …".** That sentence is now false — `test` has executed. The
  `certification.*` block is owned by `bubbles.validate`; this phase writes execution progress only,
  so the discrepancy is ROUTED rather than edited. Next owner: reconcile to six.
- **Not claimed:** regression, simplify, stabilize, security, validate, audit, and human acceptance.
  `uservalidation.md` is untouched and no `## Human Acceptance Record` was authored — G136 is
  operator-only. Packet status and `certification.status` remain `in_progress`.

---

# Regression phase — `bubbles.regression`

**Phase:** regression (`bubbles.regression`).
**Repository binding:** `PREFLIGHT_COMMITTED` decision
`rb:vscode-6af2178e10192363b0e52a46fb5e0950:11`, revision 11, repository `smackerel`.
**HEAD throughout this phase:** `9af5e309` (unchanged at start and finish; `git status --porcelain`
empty at both ends). **Fix under review:** commit `40a9e942`. **Baseline:** `d08013e6`, which is
`40a9e942^`.

> **The risk this phase was asked to characterise is not "do the new tests pass".** The test phase
> already settled that. This change *widened a regex that other assertions depend on*, and a
> widening fails in two directions that a green suite does not distinguish: it can bind something it
> should not, and it can blunt an existing detector so that a fixture still fails but for the wrong
> reason. Both of those produce green. So every question below is answered by a **delta** — old
> matcher versus new, pre-fix tree versus HEAD — and never by the post-fix state alone.

### Verdict

**🟢 REGRESSION_FREE.** No regression was found, and the numbers that decide it are below rather
than asserted. Three independent deltas were measured: the matcher's binding set changed by exactly
`+1` line and that line is a real invocation; the three pre-existing adversarial fixtures bind at
**byte-identical** offsets on both sides; and the full unit lane exits 0 at the pre-fix baseline and
at HEAD with an identical 441-line transcript.

### The change surface, measured before anything was run

Bounding the blast radius first is what makes the later "nothing else broke" claim cheap to trust,
so it was measured rather than inferred from the commit message.

**Claim Source:** executed · **Exit Code:** 0

```
$ git diff --name-only 40a9e942^ 40a9e942 -- '*.go' '*.sh' '*.py' '*.ts' '*.yaml' '*.yml' '*.json'
internal/deploy/envsubst_wrapper_contract_test.go
count=1

$ git diff --name-status 40a9e942^ HEAD -- '*.go' '*.sh' '*.py' '*.ts' '*.tsx' '*.js' '*.yaml' '*.yml' 'Dockerfile*' 'go.mod' 'go.sum'
M       .github/bubbles/...            (11 paths — framework-managed, unrelated upgrade)
M       internal/deploy/envsubst_wrapper_contract_test.go

$ git diff --stat HEAD -- scripts/runtime/      # AC-5
(no output)
$ git status --porcelain -- scripts/runtime/
(no output)
```

Across the **entire** pre-fix→HEAD span, exactly **one** product-code file changed, and it is a
`_test.go`. The other 11 code paths are `.github/bubbles/` framework files from an unrelated
framework upgrade; they carry no Go build input. `scripts/runtime/` is byte-identical to `HEAD` on
both checks, so **AC-5 holds** — the subject of the detector was not edited to satisfy its own
detector.

### Q1 — Did the widening blunt the three pre-existing adversarial tests?

**Answer: No.** Two independent lines of evidence, because the interesting failure mode here is a
test that still fails *for the wrong reason*, which a PASS/FAIL column cannot see.

**(a) The blunting mode is structurally detectable, by construction.** All three pre-existing tests
assert a *specific error substring*, not merely `err != nil`:

| Test | Asserted substring | Which locator produces it |
|---|---|---|
| `AdversarialRejectsMissingSource` | `missing source line` | `envsubstSourceLineRE` — reached **before** the go-test matcher |
| `AdversarialRejectsSourceWithoutCall` | `never calls` | `envsubstCallRE` — reached **before** the go-test matcher |
| `AdversarialRejectsCallAfterGoTest` | ``BEFORE the `ensure_envsubst` call`` | the **ordering** branch, downstream of the widened matcher |

The first two short-circuit before `envsubstGoTestRE` is consulted at all, so the widening is
unreachable for them. The third is the one that matters, and its substring is precisely what
discriminates the **ordering** error from the new **locator** error (`could not LOCATE`). Had the
widening blunted the matcher on that fixture, the post-fix function would have returned the locator
error and the test would have gone **red** on the substring check — not passed quietly. The
substring assertions are what convert "still fails" into "still fails for the right reason".

**(b) The widening is a strict no-op on all three fixtures — measured, not argued.** Each fixture
body was run through the pre-fix and post-fix patterns and the first-match **byte offset** compared:

**Claim Source:** executed · **Exit Code:** 0

```
---- fixture 1 (missing source)      ----   OLD 52:go test    NEW 52:go test
---- fixture 2 (source without call) ----   OLD 112:go test   NEW 112:go test
---- fixture 3 (call after go test)  ----   OLD 112:go test   NEW 112:go test
```

Identical offsets on all three. The ordering comparison `goTestIdx[0] < callIdx[0]` therefore
receives the *same integer* it received before the change, so its verdict cannot have moved.

**(c) The lane confirms it.** The trio was run in isolation. The `=== RUN` lines are load-bearing
and are the reason the transcript was read rather than the exit code trusted: `--go-run` narrows an
otherwise repo-wide `go test ./...`, so **exit 0 is also what a selection that matched nothing
would produce** — the same silent-pass shape this packet exists to remove. Reading only the exit
code here would have reproduced the bug inside its own regression check.

**Claim Source:** executed · **Exit Code:** 0

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsMissingSource|TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall|TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest' --verbose
exit: 0    lines: 524    sha256: 4d194a16149dea50f662f0dd3edf7cc16d1286d749d1c2519898bf2b95a556b9

403:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
404:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
405:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
406:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
408:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
409:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
411:ok      github.com/smackerel/smackerel/internal/deploy  0.038s
```

Three `=== RUN` lines for three requested names, and `ok …/internal/deploy 0.038s` rather than
`[no tests to run]` — so the selection was non-empty and the greens are real. The verdict lines were
read from the full unfiltered transcript emitted under `--diagnostic` (exit 0, 524 lines, sha256
`b39f401a1f72f52da68433aa1e7c6096a4660758e8ec30dac0e996e269d39ea0`), not from a filtering pipe.

The `--go-run` regex is **quoted** in the verify hint below. `evidence-capture.sh:218` renders the
replayed command with `"$*"`, which drops the original quoting; for a single-name selector that is
harmless, but this selector contains `|`, so the emitted form would parse as a three-stage shell
pipeline with `TestEnvsubst…SourceWithoutCall` as a command name. The quoting is restored so the
hint is actually runnable. Digest and command semantics are unchanged.

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify 4d194a16149dea50f662f0dd3edf7cc16d1286d749d1c2519898bf2b95a556b9 -- ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsMissingSource|TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall|TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest' --verbose -->

### Q2 — Does the widened matcher bind anything it should NOT?

**Answer: No. It binds exactly one line more than the pre-fix pattern, and that line is a real
invocation.** Both patterns were run over the four live wrappers and the match sets compared
line-by-line.

| Wrapper | Pre-fix matches | Post-fix matches | Delta |
|---|---|---|---|
| `scripts/runtime/go-unit.sh` | 1 — line 67 | 1 — line 67 | **0** — identical |
| `scripts/runtime/go-integration.sh` | **0** | 1 — line 76 (`if ! go test … \| tee …`) | **+1** — the defect |
| `scripts/runtime/go-e2e.sh` | 1 — line 89 | 1 — line 89 | **0** — identical |
| `scripts/runtime/go-stress.sh` | 2 — lines 50, 90 | 2 — lines 50, 90 | **0** — identical |
| **total** | **4** | **5** | **+1** |

The widening changed behaviour on **exactly the one file that was blind and nowhere else**. That is
the shape a correct narrow fix should have, and it is the reason the delta table is more informative
than the post-fix match list on its own.

**The negative control is what makes that mean something.** A widened matcher that had degraded into
a substring search would also "locate" every wrapper — and would be worthless. The four wrappers
contain **12** lines carrying the words `go test`; the widened matcher binds **5**. These are the
**7** it correctly refuses:

**Claim Source:** executed · **Exit Code:** 0

```
scripts/runtime/go-unit.sh:4:   # externally. No secrets pass through this script — only apt + go test
scripts/runtime/go-unit.sh:25:  # bypassing into raw `go test`.
scripts/runtime/go-unit.sh:66:  echo "[go-unit] starting go test ./..."
scripts/runtime/go-unit.sh:68:  echo "[go-unit] go test ./... finished OK"
scripts/runtime/go-integration.sh:83:   echo "ERROR: go-integration: could not capture go test output (tee exit ${tee_rc}); …"
scripts/runtime/go-integration.sh:122:  echo "ERROR: go-integration: go test failed (exit ${go_test_rc})." >&2
scripts/runtime/go-stress.sh:76:        done < <(go test -tags stress -list "$go_run_selector" "$package_path")

substring 'go test' lines : 12
widened-anchored matches  : 5
pre-fix anchored matches  : 4
```

The three prose lines named in the phase brief — `go-unit.sh:4`, `:25`, `:66` — are all present in
the refused set, and the scan surfaced **four more** that the brief did not name (`go-unit.sh:68`
and both `go-integration.sh` `echo "ERROR: …"` lines), plus `go-stress.sh:76`. The `(?m)^` anchor
still holds: a match must begin at a line start, and none of these lines *begins* with an enumerated
prefix or with `go`.

**One property moved in the safe direction and is worth recording, because it is easy to read the
other way.** The pre-fix pattern used `\s`, which in Go's regexp **includes `\n`**; the post-fix
pattern uses `[^\S\n]`, which does not. On the newline axis the new pattern is therefore *stricter*,
not looser — it can no longer span a line boundary between `go` and `test`. The widening is confined
entirely to the bounded prefix enumeration.

### Q3 — Did any previously-green test elsewhere break?

**Answer: No — and this was established against a real baseline that was executed, not inferred.**

**Method.** A detached worktree was created at `40a9e942^` and the *identical* lane was run there.
This is a supported path in this repository rather than a hack: `smackerel_prepare_tooling_git_mount_args`
(`scripts/lib/runtime.sh:10`) explicitly detects a linked worktree's `.git` **file** and mounts the
git common dir read-only for the tooling container. The main checkout was never modified — verified
clean by `git status --porcelain` before, during, and after — and the worktree was removed at the
end. The baseline tree was confirmed to carry the **pre-fix** pattern before any lane was run:

```
$ git -C <baseline-worktree> rev-parse HEAD
d08013e6fb7089cadef873240d0d0ee3a6f48894
$ grep -n 'envsubstGoTestRE = regexp' <baseline-worktree>/internal/deploy/envsubst_wrapper_contract_test.go
82:var envsubstGoTestRE = regexp.MustCompile(`(?m)^\s*go\s+test\b`)
```

**The full-lane comparison.** Both sides, same command, same tool:

| | Commit | Command | Exit | Lines | sha256 of full output |
|---|---|---|---|---|---|
| **BASELINE** | `d08013e6` (pre-fix) | `./smackerel.sh test unit` | **0** | **441** | `90c060e034b6ab7a889e984c3a13e0e7d25277de5aff9b55d41f8d5daddb3964` |
| **HEAD** | `9af5e309` | `./smackerel.sh test unit` | **0** | **441** | `9c8e86a9cd2fa63a13cccba78a841e6b5ab8a3551e5440b22cff89fa1769fb41` |

Green on both sides, with an **identical 441-line transcript length**. The hashes differ only
because the transcript embeds run-varying values — preflight RAM/disk figures, per-package timings,
and the checkout path — which is why the line count and the exit code carry the comparison and the
hash carries re-derivability. `evidence-capture.sh` surfaced **no failure-shaped lines** from either
omitted region.

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify 9c8e86a9cd2fa63a13cccba78a841e6b5ab8a3551e5440b22cff89fa1769fb41 -- ./smackerel.sh test unit -->

**Coverage delta — did a test disappear?** A suite can stay green by losing a test, so the inventory
was enumerated on both sides. Only package `deploy` changed, so it is the only package where the
inventory *could* move:

| | Exit | Lines | Top-level tests | Subtests | Failures |
|---|---|---|---|---|---|
| **BASELINE** `d08013e6` | 0 | 536 | **5** | 4 (`LiveWrappers/*`) | 0 |
| **HEAD** `9af5e309` | 0 | 540 | **7** | 4 (`LiveWrappers/*`) | 0 |

```
BASELINE (sha256 5f3dcb0970d0be07d527d2949b9f0ad9bf0eca222589ce99b1c89924345aee94)
  HelperExistsAndIsExecutable · LiveWrappers (+4 subtests) ·
  AdversarialRejectsMissingSource · AdversarialRejectsSourceWithoutCall · AdversarialRejectsCallAfterGoTest

HEAD     (sha256 7f30160c12647a94fa6d706db7dba34a9fade1a91eb5d6cce5798b5bfc7bdfc0)
  …the same five, byte-identical names and PASS verdicts, plus
  + AdversarialRejectsUnlocatableInvocation
  + AdversarialRejectsConditionalCallAfterGoTest
```

**+2 added, 0 removed, 0 renamed, 0 subtests lost.** The coverage delta is strictly additive.

**The baseline run independently reproduced the defect, which is the strongest single datum here.**
At `d08013e6` the subtest `TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh` reports
`--- PASS` — while the pre-fix matcher binds **zero** lines in that file (measured in the Q2 table).
That is BUG-061-013 caught in the act on this phase's own authority: a green subtest that compared
nothing. The same subtest is green at HEAD, but now with a located invocation behind it.

### Cross-spec impact scan

**Symbol reachability.** Every symbol the commit touches is package-private and referenced from a
single file:

```
$ grep -rn 'envsubstGoTestRE|assertEnvsubstWrapperContract|envsubstTrackedWrappers|envsubstCallRE|envsubstSourceLineRE'
81 matches in 8 files — of which the ONLY .go file is
    internal/deploy/envsubst_wrapper_contract_test.go
(the other 7 files are spec .md / .json inside this and the BUG-061-011 packet)
```

No other Go file can observe the widened matcher, so there is no "elsewhere" inside the codebase for
its behaviour to leak into.

**The other guard over the same wrapper — checked, because this is where a real conflict would
live.** `internal/deploy/eval_lane_contract_test.go` also reads
`scripts/runtime/go-integration.sh` (BUG-061-011's invariant), and both guards compile into the same
package test binary. They share **no literals**: the eval-lane guard keys on
`gate_marker_prefix="ASSISTANT_ACCEPTANCE_GATE_V1"`, on
`if [[ -z "$go_run_selector" ]]; then # full-lane run: …`, and on the
`"$gate_executed_assertions"` comparisons — none of which the `go test` matcher can reach. The
"column zero" phrase in `tests/eval/assistant/harness.go:340` refers to the gate **marker** in the
lane's *output stream*, not to the invocation line. No interaction, no contradiction.

**A second `go test` matcher exists in the same package and was left alone, correctly.**
`internal/deploy/ci_integration_topology_contract_test.go:82` compiles
`` `go\s+test\b[^\n]*-tags[=\s]+integration\b[^\n]*\./tests/integration` `` — deliberately
*unanchored*, because its job is the opposite one: to **forbid** a raw `go test` anywhere in a CI
workflow step, not to locate one at a line start. It scans `.github/workflows` YAML, never
`scripts/runtime/`. Divergent semantics here are correct rather than drift, and the commit did not
touch it.

### Test baseline comparison

| Category | Baseline `d08013e6` | HEAD `9af5e309` | Delta | Status |
|---|---|---|---|---|
| Full unit lane (Go + Python + shell, all packages) | exit 0, 441 lines, 0 failures | exit 0, 441 lines, 0 failures | 0 | 🟢 CLEAN |
| `internal/deploy` contract suite | 5 tests + 4 subtests, all PASS | 7 tests + 4 subtests, all PASS | **+2 tests** | 🟢 CLEAN (additive) |
| Wrapper matcher binding set | 4 matches | 5 matches | **+1** (`go-integration.sh:76`) | 🟢 INTENDED |
| Pre-existing adversarial fixture offsets | 52 / 112 / 112 | 52 / 112 / 112 | 0 | 🟢 CLEAN |
| `scripts/runtime/` bytes | — | identical to `HEAD` | 0 | 🟢 AC-5 HOLDS |

### Deployment regression scan

**Not applicable, and measured rather than waved off.** The Build-Once Deploy-Many surfaces —
`deploy/`, `.github/workflows/build.yml`, `config/smackerel.yaml`, `scripts/deploy/` — appear
nowhere in `git diff --name-only 40a9e942^ HEAD`. No manifest pointer, image digest, bundle hash,
cosign step, attestation, Trivy gate, or adapter script was touched, so none of the Gate G081
detection patterns has an input to fire on.

### Why the integration and e2e lanes were not re-run

This is a deliberate scoping decision with a stronger justification than a re-run would have
produced, so it is recorded rather than left as an absence. The two heavyweight lanes **cannot
compile the changed file**, by their own package selection:

```
scripts/runtime/go-integration.sh:53:  go_test_args+=(./tests/integration/... ./internal/notification/... \
                                          ./internal/assistant/... ./internal/cardrewards/... ./tests/eval/...)
scripts/runtime/go-e2e.sh:78:          go_test_packages=(./tests/e2e/...)
```

`./internal/deploy/...` is in **neither** allow-list. The changed file is compiled only by the unit
lane's `go test ./...`. That is a structural proof that those lanes are unaffected, which is
strictly better evidence than a green re-run — a re-run shows they *did not* break, while the
allow-list shows they *could not*. Both lanes were in any case already executed green at this exact
HEAD in this session by the orchestrator (T-06 `a3230783…`, T-08 `eb5e8fa3…`), and re-running them
without a matching baseline-side run would have added ~75 minutes and no delta.

### Baseline method, disclosed

Three things were done to the scratch worktree that a reader should be able to audit:

1. **The first baseline attempt failed, and it was not a regression.** `./smackerel.sh test unit` at
   `d08013e6` exited **1** (214 lines, sha256
   `21b0ddb4ab41f6c2853c936a2a02d7a5a385b51394e46e1e97238a139575dd32`) with
   `docker_security_test.go:231: read config/generated/nats.conf: … no such file or directory`.
   `config/generated/` is gitignored, so a fresh worktree has none. This is a **worktree artifact**,
   not a finding — it is recorded because discarding a red run without saying why is exactly how a
   baseline gets quietly shopped for. It was repaired by generating the config, after which the
   lane exited 0 (R-04 above).
2. **A gitignored operator-local env file was copied into the worktree** for environment parity
   (`config generate` refused without it). Its contents were never printed. It was **deleted first**
   during cleanup and its absence confirmed, before anything else was removed.
3. **The worktree was fully removed.** `git worktree list` reports only the main checkout, the
   scratch directory no longer exists, and `git status --porcelain` on the main checkout is empty at
   `9af5e309`. Some generated files were root-owned by the tooling container and were removed via a
   container running as root, since host-side removal returned `Permission denied` and privilege
   escalation is not available.

### Deviation from terminal discipline, disclosed

One command in this phase — a `./smackerel.sh config generate` in the scratch worktree — was
invoked with `>/dev/null 2>&1`, which discards output and is forbidden. It was caught immediately
and the command was **re-run unfiltered**, which is how the real cause (`Permission denied` writing
`nats.conf`, not the hardware-tier error the suppressed run implied) was found. No claim in this
section rests on the suppressed invocation. It is recorded rather than omitted, because a discarded
stream is the same species of self-inflicted blindness this packet exists to remove.

### Not run in this phase

- **`./smackerel.sh test integration` and `./smackerel.sh test e2e`** — not re-run; reason and the
  allow-list proof are in the section above. Already green at this HEAD in this session.
- **`./smackerel.sh test stress`** — not run. `go-stress.sh` is a tracked wrapper, but it was never
  blind (2 matches on both sides of the change) and the stress lane does not compile
  `internal/deploy` either.
- **No mutation test against the live `scripts/runtime/go-integration.sh`.** AC-5 forbids touching
  it. The equivalent behavioural proof is the `AdversarialRejectsConditionalCallAfterGoTest`
  fixture, which carries line 76's exact shape.
- **No baseline-side integration/e2e run**, which is what a full two-sided comparison of those lanes
  would have required. Recorded as a scope boundary, not closed.

### Uncertainty declaration

Two limits are worth stating plainly rather than leaving for a reader to discover.

**The prefix enumeration remains a judgement.** `envsubstGoTestRE` still cannot match
`env VAR=x go test`, `timeout N go test`, or `x=$(go test …)`. This phase did not remove that
blindness and did not try to. What it verified is that the blindness is no longer *dangerous*: the
zero-match branch now converts any future miss into a loud locator failure, and
`AdversarialRejectsUnlocatableInvocation` is the regression that keeps that branch honest.

**"No regression elsewhere" is scoped to what the change can reach.** That scope was measured — one
`_test.go`, package-private symbols, absent from the integration/e2e/stress allow-lists — rather
than assumed, and the full unit lane was run on both sides of it. It is not a claim that every lane
in the repository was executed twice.

### Independent re-derivation before recording

The analysis above was produced by an earlier `bubbles.regression` invocation in this session that
returned before writing the phase to `state.json`. The recording invocation did **not** rubber-stamp
it. The two load-bearing claims were re-derived from the tree, and one defect was found and fixed.

**Re-derivation 1 — the fixture offsets, from absolute file offsets rather than retyped fixtures.**
Retyping a fixture to re-measure it would test the transcription, not the claim, so the offsets were
taken directly out of the source file and subtracted.

**Claim Source:** executed · **Exit Code:** 0

```
$ grep -abo '`#!/usr/bin/env bash' internal/deploy/envsubst_wrapper_contract_test.go
9864  10720  11572  13023  14765          ← opening backtick; fixture[0] is backtick+1

$ grep -abo -E '^go[[:blank:]]+test' internal/deploy/envsubst_wrapper_contract_test.go
9917:go test    10833:go test    11685:go test     ← the ONLY line-starting tokens in the file

fixture1  9917 − 9865 =  52
fixture2 10833 − 10721 = 112
fixture3 11685 − 11573 = 112
```

`52 / 112 / 112`, matching the figures recorded in Q1(b).

**This upgrades the claim from "measured equal" to "structurally forced equal", which is stronger.**
Both patterns are `(?m)^`-anchored, so a match can only *begin* at a line start; and both require a
literal `go`, then whitespace, then `test`. The file contains exactly **three** line-starting
`go<blank>test` tokens — one per fixture — so within any one fixture there is exactly **one**
position at which either pattern can match at all. Two patterns that can each only match in one
place, and do both match there, cannot disagree about the offset. The equality is therefore not a
lucky measurement that a future edit might quietly break.

The one way the old pattern could have matched somewhere else is its `\s`, which in Go **includes
`\n`** and so could have spanned a line boundary between `go` and `test`. That was checked and does
not occur inside any fixture: the only end-of-line `go` in the file is at lines 25–26, inside a
comment naming `…_test.go` filenames, at byte offsets far below the first fixture at 9865.

**Re-derivation 2 — the Q2 binding delta, reproduced independently.** The three counts and both
match sets were regenerated with a fresh scan of the four live wrappers.

**Claim Source:** executed · **Exit Code:** 0

```
substring 'go test' lines : 12   ← identical 12 lines to those listed in Q2
pre-fix anchored matches  :  4   ← go-unit:67, go-e2e:89, go-stress:50, go-stress:90
post-fix anchored matches :  5   ← the same 4, plus go-integration.sh:76
delta                     : +1   ← if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
```

The `+1` line is a real invocation, and the seven refused lines are line-for-line the seven listed
in Q2. Q2 is reproduced exactly.

**Re-derivation 3 — the lane allow-lists that justify not re-running integration and e2e.** This is
the claim that buys ~75 minutes, so it was checked rather than accepted.

**Claim Source:** executed · **Exit Codes:** 0 (allow-list read), 1 (`internal/deploy` absent — the
desired result)

```
$ grep -n 'go_test_args+=(\./|go_test_packages=(' scripts/runtime/go-{integration,e2e,stress}.sh
go-integration.sh:53: ./tests/integration/... ./internal/notification/... ./internal/assistant/... \
                      ./internal/cardrewards/... ./tests/eval/...
go-e2e.sh:78:         ./tests/e2e/...
go-e2e.sh:81:         ./tests/e2e/assistant

$ grep -n 'internal/deploy' scripts/runtime/go-{integration,e2e,stress}.sh
(no output — exit 1)

$ grep -n 'go test' scripts/runtime/go-unit.sh
67: go test "${go_test_args[@]}" ./...      ← repo-wide; the only lane that compiles the changed file
```

Neither heavyweight lane can compile `internal/deploy`. The structural argument holds.

**Defect found and fixed: an unrunnable verify hint.** The Q1(c) `<!-- verify: -->` comment carried
its `--go-run` alternation **unquoted**, because `evidence-capture.sh:218` renders the replayed
command with `"$*"` and that drops the caller's quoting. For the single-name selectors elsewhere in
this report that is harmless; for this one it is not, because the selector contains `|` and the
emitted line parses as a three-stage shell pipeline. A re-derivation hint that cannot be run is the
same species of unchecked assertion this packet exists to remove, so the quoting was restored. The
digest and the command's semantics are unchanged.

**Nothing else was altered, and no claim was weakened.** No further correction was required: the
`--go-run` / `--verbose` flags cited in Q1(c) are real `test unit` flags (`smackerel.sh:58`, gated
behind `--go`, which the recorded command passes), `--diagnostic` is `evidence-capture.sh`'s
retention escalation rather than a CLI flag, and the two 524-line transcripts differing in sha256 is
the run-varying-content pattern the phase already documents for the 441-line pair.

### Completion Statement — regression phase

The regression phase executed and found **no regression**. All three questions were answered by a
measured delta: the widened matcher binds exactly one line more than its predecessor and that line
is `go-integration.sh:76`, a real invocation; it refuses all seven prose and `echo` lines that
merely contain the words `go test`; the three pre-existing adversarial fixtures bind at identical
byte offsets on both sides and all three still reject with their original error substrings; and the
full unit lane exits 0 at both the pre-fix baseline `d08013e6` and at HEAD `9af5e309` with identical
441-line transcripts and a strictly additive `+2 / -0` test inventory.

The baseline was **executed**, not inferred — and it independently reproduced BUG-061-013, showing
`LiveWrappers/go-integration.sh` green at a commit where the matcher bound nothing in that file.

The analysis was performed by an earlier `bubbles.regression` invocation in this session which
returned before recording the phase; the recording invocation re-derived the two load-bearing claims
from the tree rather than accepting them, and the offset claim came back **stronger** than recorded
(forced by there being exactly one line-starting `go<blank>test` token per fixture, not merely
observed equal). One defect was found and fixed — an unrunnable `<!-- verify: -->` hint whose
alternation regex had lost its quoting. No claim was weakened and no verdict changed.

No source file was modified by this phase. `scripts/runtime/` and `internal/` are untouched, the
working tree is clean at `9af5e309` apart from this packet's `report.md` and `state.json`, and
`uservalidation.md` was not opened. Only `regression` is claimed; `simplify`, `stabilize`,
`security`, `validate`, and `audit` remain unexecuted and are left to their own specialists. Packet
`status` and `certification.status` remain `in_progress` — certification belongs to
`bubbles.validate`.

**Routed, not edited:** `certification.pendingGates` still reads "7 of the 8 … phases remain
unexecuted: test, regression, …". Two of those have now executed. The `certification.*` block is
owned by `bubbles.validate`; this phase writes execution progress only, so the count is flagged for
its owner rather than corrected here.

---

# Simplify phase — `bubbles.simplify`

### Verdict

**NO CHANGE. `internal/deploy/envsubst_wrapper_contract_test.go` is unmodified, and the working tree
carries no source diff.** The review ran all three passes — reuse, quality, efficiency — and each
one's headline recommendation was already satisfied or was measured to be net-negative for this
file. One genuine inaccuracy was found; it was **proven inert** and therefore flagged rather than
fixed, because rewriting a hardened, adversarially-tested contract file for a change that cannot
alter behaviour is the churn this phase is supposed to refuse.

A no-op simplify phase is a result, not an absence of one. What follows is the measurement behind it.

### Review surface

One file, 326 lines, byte-identical to the fix commit `40a9e942`
(`git diff --quiet 40a9e942 HEAD -- <file>` → identical). That commit's only source path is this same
file, so the packet's change surface and this phase's review surface are the same object.

```
=== incomplete-work markers (TODO|FIXME|HACK|XXX|nolint|t.Skip) ===
marker-grep-exit=1        # grep found nothing
```

### Pass 1 — reuse: the extraction is already done

| Measurement | Result |
|---|---|
| Files in `internal/deploy` containing the wrapper-scan logic | **1 of 38** — no cross-file duplication |
| Call sites of `assertEnvsubstWrapperContract` | **6** — 1 live loop + 5 adversarial |
| `TestEnvsubstWrapperContract_LiveWrappers` shape | already table-driven over `envsubstTrackedWrappers` |

The reuse pass exists to recommend extracting a shared helper. That helper exists, is the single
point of truth for the contract, and every test routes through it. There is nothing to extract.

### Pass 1 — reuse: why the five adversarial tests are NOT table-driven

This was the primary question, so it was answered with a line census rather than an impression:

```
total lines in adversarial block : 116
  mechanically repeated skeleton : 42
  doc-comment lines (rationale)  : 29
  blank                          :  4
  unique payload (fixtures+msgs) : 41
```

A table can remove **only the 42 skeleton lines**. It must reintroduce a struct definition (~7), a
runner loop (~12), and per-row field labels plus row braces (5 rows × 5 fields + 10 ≈ 35) — roughly
**54 lines to remove 42**, before any table-integrity guard. The 41 unique payload lines and 29
rationale lines survive in either shape. **A table here relocates the content; it does not reduce
it, and on raw count it grows the file.**

Three costs that do not appear in the line count, and that matter more than it:

1. **One row would have to carry two assertions.** `AdversarialRejectsUnlocatableInvocation` asserts
   *both* `could not LOCATE` and `matcher may need widening` — EB-2 and AC-4 are distinct claims, one
   about locating and one about blame attribution. A single `wantSubstr` field forces merging them,
   which is weakening; a `[]string` field adds exactly the complexity the table was meant to remove.

2. **The names are load-bearing recorded evidence.** Any table conversion turns five top-level
   functions into subtests with new identifiers. Those five names are referenced **10–12× each in
   `report.md`, 2–4× in `scopes.md`**, plus `design.md` and `state.json` — roughly fifty references
   in artifacts this phase must not rewrite.

3. **The decisive one — a table makes this file's own failure mode easier.** This packet exists
   because an assertion silently stopped asserting. Five top-level `func`s are maximally visible in
   review: deleting one deletes a named function and removes a line from the test output. Blanking a
   `wantSubstrings` field in a table row **still compiles and still passes**. Converting independent
   named assertions into rows of a shared table enlarges the surface for precisely the defect
   BUG-061-013 removes. That is the opposite of a simplification here.

### Pass 2 — quality: the line-104 matcher comment is accurate and complete

Every claim the comment makes was checked against the compiled pattern rather than read
sympathetically:

| Comment claim | Verified against `envsubstGoTestRE` |
|---|---|
| prefixes `if`, `elif`, `then`, `while`, `until`, `&&`, `\|\|`, `\|`, `!`, repeatable in any order | matches the pattern's `(?:(?:…)[^\S\n]+)*` group exactly |
| `env VAR=x go test` does not match | correct — `env` is not an enumerated token and the anchor forbids it |
| `timeout N go test` does not match | correct — same reason |
| `x=$(go test …)` does not match | correct, **and executed**: this is fixture 4, which the lane below proves returns the locator error |

One thing deliberately **not** added: a note about alternation order. `\|\|` precedes `\|`, which
looks load-bearing but is not — Go's `regexp` is RE2, which finds a match if *any* alternative
matches, so ordering affects preference, not existence. A comment warning about a non-hazard would
mislead the next reader, which is the failure this pass is trying to prevent.

### Pass 2 — quality: the one real inaccuracy, and why it was flagged instead of fixed

Line 75 describes `envsubstSourceLineRE`'s prefix as `(?m)^\s*`. Line 80 compiles
`(?m)^[^\S\n]*`. Those are **not** interchangeable, and the difference is not theoretical — measured
on a blank-line-before-token shape:

```
claimed  \s*: start=45  matched='\nensure_envsubst '
compiled [^\S\n]*: start=46  matched='ensure_envsubst '
blank-line offset=45  true-call-line offset=46  delta=1
```

`\s` includes `\n`, so under `(?m)` it can begin a match at an earlier blank line and report an
offset that is not the token's line. Offsets are exactly what this function's ordering comparisons
consume, so this looked like a live mis-widening hazard.

**It is not.** Substituting `\s` for `[^\S\n]` in all three matchers changes **0 of 11 verdicts**,
across the five real fixtures, two probes built specifically to trigger the offset shift, and all
four live wrappers:

```
fx1 missing-source         compiled=missing source line    claimed=missing source line    same
fx2 source-only            compiled=never calls            claimed=never calls            same
fx3 call-after-gotest      compiled=go test BEFORE call    claimed=go test BEFORE call    same
fx4 unlocatable            compiled=could not LOCATE       claimed=could not LOCATE       same
fx5 conditional-late       compiled=go test BEFORE call    claimed=go test BEFORE call    same
probe blank+violation      compiled=go test BEFORE call    claimed=go test BEFORE call    same
probe blank+compliant      compiled=PASS                   claimed=PASS                   same
live go-unit.sh            compiled=PASS                   claimed=PASS                   same
live go-integration.sh     compiled=PASS                   claimed=PASS                   same
live go-e2e.sh             compiled=PASS                   claimed=PASS                   same
live go-stress.sh          compiled=PASS                   claimed=PASS                   same

verdict divergences across 11 inputs: 0
```

The zero is **structural, not luck**, and that is what makes it safe to rely on: `\s*` can only
extend a match start backward through *contiguous whitespace*, and every offset pair the function
compares (`callIdx` vs `srcIdx`, `goTestIdx` vs `callIdx`) is separated by a line containing
non-whitespace text that `\s*` cannot span. The pull-back is bounded by the preceding blank run and
can never cross the other token's line. So no substitution of this kind can flip a comparison in
this file — including in `envsubstGoTestRE` itself.

The comment's operative claim — "anchors to line start so the rationale comment above does NOT
match" — is true of both forms (a `//` line begins with non-whitespace either way). The imprecision
is in a paraphrase that cannot reach a comparison. **Fixing it would mean editing a hardened file
and re-running lanes to change a character class in prose. Flagged here, with the proof, so the next
reader gets the analysis without the file being churned.**

**Related, also left alone:** `envsubstCallRE` (line 85) is the one matcher that genuinely uses
`\s*`, which makes the file's convention inconsistent. The same proof covers it — it cannot produce
a false pass or a false failure. It is also a **matcher**, and changing a matcher changes bindings;
the regression phase established the binding set as exactly 4 → 5 lines, so touching it would be a
regression to re-measure, not a simplification.

### Pass 2 — quality: function size

```
assertEnvsubstWrapperContract                              lines= 37 <-- OVER 30
envsubstWrapperRepoRoot                                    lines= 13
TestEnvsubstWrapperContract_HelperExistsAndIsExecutable    lines= 29
TestEnvsubstWrapperContract_LiveWrappers                   lines= 19
TestEnvsubstWrapperContract_AdversarialRejectsMissingSource lines= 18
TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall lines= 19
TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest lines= 27
TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation lines= 30
TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest lines= 18
```

One function exceeds the 30-line guideline, at 37. Its excess is **entirely error prose**: five
guard clauses, each a two-line `fmt.Errorf`, plus the five-line rationale comment on the zero-match
branch and five blank separators. Maximum nesting is 1 and every branch is an early return. Those
messages are themselves pinned by tests (EB-2 / AC-4 assert their substrings), so splitting the
function would separate each guard from the message its test asserts — worse on every axis the
guideline is trying to protect. Not a defect; not changed.

### Pass 3 — efficiency

| Candidate | Disposition |
|---|---|
| `src := string(raw)` copies the file bytes; `FindIndex` on `[]byte` would avoid it | **Rejected.** 9 call sites over ~100-line shell scripts; the whole package runs in 0.025s. Trading a clear `string` API for a `[]byte` one buys an unmeasurable amount. |
| Three regexes recompiled per call | **Already correct** — all three are package-level `MustCompile`, compiled once. |
| `os.Stat` + `os.ReadFile` in the helper test are two syscalls | **Rejected.** `ReadFile` does not return the mode the test asserts; `Open`+`Stat`+`ReadAll` is more code for one fewer syscall in a test. |

No efficiency change is warranted.

### Lane evidence — review surface green, and the selector proven to bind

```
# BUG-061-013 simplify: review-surface lane, unchanged file
$ ./smackerel.sh test unit --go --go-run TestEnvsubstWrapperContract --verbose
exit: 0
lines: 540
sha256: 2a51e6d2bb79ba9a49785f364f88091fb4bfd5bfda5f83f2b49cc4f653e7aa86
--- first 20 ---
oom-preflight: OK — 26407 MB available (need 6000 MB; swap used 672 MB).
disk-preflight: OK — C: 69 GB free (need 40 GB), WSL / 490 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
[go-unit] envsubst missing — installing gettext-base
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
  gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 500 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.017s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/observability      0.003s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.014s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/unit/clients       0.006s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/web/pwa/tests    0.189s [no tests to run]
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

<!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify 2a51e6d2bb79ba9a49785f364f88091fb4bfd5bfda5f83f2b49cc4f653e7aa86 -- ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose -->

**Exit 0 was not accepted as proof.** The tail above shows repeated `testing: warning: no tests to
run` / `PASS` pairs — a `--go-run` selector that binds *nothing* also exits 0. Treating that as a
pass would be this packet's own defect, committed while recording its cleanup phase. The lane was
therefore re-run plainly and the binding read out of the full transcript:

```
232:=== RUN   TestEnvsubstWrapperContract_HelperExistsAndIsExecutable
233:--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
234:=== RUN   TestEnvsubstWrapperContract_LiveWrappers
235:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
236:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
237:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
238:=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
239:--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
240:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
241:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
242:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
243:    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
244:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
245:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
246:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
247:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
249:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
250:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
251:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
252:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
254:=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
256:--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)

259:ok      github.com/smackerel/smackerel/internal/deploy  0.025s
537:LANE_EXIT=0

count of bound top-level tests: 7
```

Seven top-level tests bound and passed, four `LiveWrappers` subtests passed, zero `FAIL`, and the
package reports `ok`. That is what makes the exit code mean something.

**Replay-hint correction, disclosed.** `evidence-capture.sh` emitted its hint with the source-repo
path `bubbles/scripts/evidence-capture.sh`, which does not exist in this downstream checkout; the
hint above uses the installed path `.github/bubbles/scripts/evidence-capture.sh` and quotes the
`--go-run` selector. Same correction class the regression phase applied to its own hint — an
un-runnable re-derivation hint is an unchecked assertion.

### Not run in this phase

`./smackerel.sh lint` and `./smackerel.sh format --check` were **not** run, and no result for them is
claimed. Both are gates on *changed* source; this phase changed no source, and the file is
byte-identical to `40a9e942`, whose implement phase already recorded them. Running them would
re-derive a property of an unmodified file. The integration, e2e, and stress lanes were likewise not
re-run for the same reason — see the test and regression phases for their recorded results.

### Uncertainty declaration

The eleven-input verdict comparison was executed with Python's `re`, not Go's `regexp`. The two
engines differ in backtracking strategy, so this is a **transcription**, not a run of the compiled
patterns. It is sound for the specific question asked — whether `\s` consuming `\n` moves a reported
offset — because that turns on what `\s` matches, which both engines define identically, and because
RE2 reports the leftmost start at which any match exists, so it would report the same earlier start.
The structural argument above does not depend on the engine at all. The Go-side facts are covered by
the executed lane: fixture 4 (`x=$(go test …)`) really does produce the locator error under the
compiled pattern, because `AdversarialRejectsUnlocatableInvocation` passed.

The claim that a table-driven form would be **~54 lines to remove 42** is an estimate of code not
written; the 116 / 42 / 29 / 41 / 4 census of the existing block is measured. The three non-line-count
objections stand independently of that estimate.

### Completion Statement — simplify phase

The simplify phase executed and **changed nothing**, which is the correct outcome here rather than an
absent one. Reuse: the shared helper already exists with six call sites and the logic appears in one
file of thirty-eight. Duplication: the five adversarial tests share 42 skeleton lines that a table
would replace with roughly 54, while relocating rather than removing the 41 unique and 29 rationale
lines — and would convert five independently-visible assertions into rows where a blanked field still
compiles and still passes, which is this packet's own failure mode. Clarity: the line-104 comment's
claims were each checked against the compiled pattern and hold. Efficiency: the only candidates trade
clarity for an unmeasurable gain in a 0.025s package.

The one genuine inaccuracy — line 75 describing a `[^\S\n]*` prefix as `\s*` — was measured rather
than assumed, found to change offsets but **zero of eleven verdicts**, and shown to be structurally
incapable of flipping any comparison because every compared offset pair is separated by
non-whitespace text. It is recorded here for its owner instead of being fixed, because editing a
hardened, adversarially-tested contract file for an inert paraphrase is churn, not simplification.

No source file was modified. `internal/deploy/envsubst_wrapper_contract_test.go` and
`scripts/runtime/` are byte-identical to HEAD, so **AC-5 is preserved by this phase as well** — the
subject still was not edited to satisfy its own detector. The only working-tree changes are this
packet's `report.md` and `state.json`. `uservalidation.md` was not opened and no
`## Human Acceptance Record` section was authored. Only `simplify` is claimed; `stabilize`,
`security`, `validate`, and `audit` remain unexecuted and belong to their own specialists. Packet
`status` and `certification.status` remain `in_progress`.

**Routed, not edited (unchanged from the regression phase's routing):**
`certification.pendingGates` still reads "7 of the 8 … phases remain unexecuted: test, regression,
simplify, …". **Four** have now executed — `implement`, `test`, `regression`, `simplify`. The
`certification.*` block is owned by `bubbles.validate`; this phase writes execution progress only, so
the count is flagged for its owner rather than corrected here.


# Stabilize phase — `bubbles.stabilize`

### Verdict

🟢 **STABLE** for the packet-attributable surface. The change introduces **no** determinism, latency,
resource, or failure-mode risk, and the four lanes it touches show **zero** instability markers.

One **real** stability weakness was found, but it is **pre-existing** and **foreign-owned**:
`ensure_envsubst` installs `gettext-base` over the network on every fresh container with no retry, no
timeout, and no offline fallback. It is recorded below as `OBS-061-013-STAB-01` and routed to
`bubbles.devops`. It is **not** a defect of this packet, it does **not** gate this packet, and it was
deliberately not fixed here — AC-5 forbids touching `scripts/runtime/`, and the durable fix is a
container-image change outside this lane.

### Scope of this phase

The change is test-only: one file, `internal/deploy/envsubst_wrapper_contract_test.go`. So the
stability question is not "does the product degrade" but "is this guard itself dependable" — does it
run the same way every time, fail usefully when it fails, and cost nothing meaningful.

Where a property examined below is **pre-existing** rather than introduced by `40a9e942`, it is
labelled as such. The packet's own diff touched the matcher (line 104), the zero-match branch (line
130), and two adversarial fixtures; `envsubstWrapperRepoRoot`, the exec-bit check, and the tracked-
wrapper slice all predate it.

### Area 1 — Flakiness and determinism: CLEAN

Four independent nondeterminism sources were checked and all are absent.

```
# BUG-061-013 stabilize: nondeterminism sweep over the contract test
$ grep -nE 'range .*map|os\.ReadDir|filepath\.Walk|filepath\.Glob|time\.Now|math/rand|t\.Parallel' \
    internal/deploy/envsubst_wrapper_contract_test.go
NONDET_EXIT=1 (1=none found)
```

No map iteration (Go randomises map order), no directory listing (filesystem order), no wall-clock,
no RNG, no `t.Parallel` interleaving. The only iteration is over `envsubstTrackedWrappers`, a fixed
slice literal, so subtest order is fixed — confirmed by the transcript, which emits `go-unit.sh`,
`go-integration.sh`, `go-e2e.sh`, `go-stress.sh` in declaration order.

**Working-directory and checkout-layout independence.** `envsubstWrapperRepoRoot` derives the repo
root from `runtime.Caller(0)` plus `../..`, never from `os.Getwd()` or an env var. `runtime.Caller`
returns the path recorded at *compile* time, so the decisive question is whether the build trims it —
a trimmed path would be module-relative and therefore resolved against CWD at runtime.

```
# Is -trimpath in play anywhere? (tracked files, repo-wide)
$ git grep -n 'trimpath'
GITGREP_EXIT=1 (1=absent)

# Could a binary be built in one place and run in another?
$ grep -rnE 'go test -c|go build[^|]*\.test' scripts/
GOTESTC_EXIT=1 (1=absent)
```

`-trimpath` is absent, so the recorded path is absolute; and no wrapper builds a detached test binary,
so compile and run always happen in the same container against the same mount. Root resolution is
therefore CWD-independent and survives any checkout root. It breaks only if the test file itself is
moved out of `internal/deploy/` — and then `os.Stat` fails and `t.Fatalf` fires, which is loud, not
silent.

An earlier form of this check returned `TRIMPATH_EXIT=2`. That is grep's *error* code, not "no match"
(which is 1) — a non-existent glob had poisoned the argument list. It was re-run against paths that
exist; the exit 1 above is the trustworthy result. Recorded because accepting the first result would
have been the same class of mistake this packet exists to remove.

**The exec-bit check is reproducible**, because the mode is versioned rather than filesystem-derived:

```
$ git ls-files -s scripts/runtime/_ensure_envsubst.sh
100755 3e7a6af135f9b3b5251163b8177b7ec026df52a3 0       scripts/runtime/_ensure_envsubst.sh
```

**The test cache cannot mask a stale pass.** `scripts/runtime/go-unit.sh:63` appends `-count=1`
alongside the `-run` selector, so a scoped re-run genuinely re-executes instead of replaying a cached
`(cached)` verdict. Both runs below are real executions.

**Empirical:** two runs, both `exit 0`, both 540 lines, identical eleven-PASS set.

### Area 2 — Failure-mode quality: CLEAN, and proven by executed test

The zero-match branch returns an `error` that `TestEnvsubstWrapperContract_LiveWrappers` hands to
`t.Fatalf`. It **cannot hang**: it is a pure in-memory regex over an already-read `[]byte`, with no
I/O, no network, no loop, and no sleep on that path. It fails on the first tracked wrapper that
cannot be located.

The message is actionable *and correctly directed* — it names the wrapper, states the invocation
`could not LOCATE`, says explicitly that the wrapper is **not** necessarily wrong and must not be
rewritten to satisfy the pattern, and names the remediation target (`envsubstGoTestRE`). That matters
because the tempting wrong fix is to edit a correct wrapper until the blind matcher matches it.

This is not an assertion of mine; it is executed. `AdversarialRejectsUnlocatableInvocation` asserts
the message contains both `could not LOCATE` and `matcher may need widening`, and it **passed**.
`AdversarialRejectsConditionalCallAfterGoTest` demands the *ordering* error specifically — obtainable
only if the widened matcher located the real `if ! go test … | tee …` line — and it **passed** too.
So the widening landed on the real invocation rather than on something inert.

### Area 3 — Resource and time behaviour: no measurable cost

`internal/deploy` completed in **0.025s**, the floor of the 0.025–0.075s band recorded by prior lanes.
Every one of the four `LiveWrappers` subtests reports 0.00s. Total input scanned is 12,799 bytes
(go-unit 2,181 + go-integration 5,365 + go-e2e 2,533 + go-stress 2,720). All three patterns are
package-level `regexp.MustCompile` vars (lines 80, 85, 104), so they compile **once per test binary**,
not once per wrapper.

**Catastrophic backtracking cannot occur here** — not "is unlikely". Go's `regexp` is RE2-based and is
documented to run in time linear in the size of the input; it simulates an NFA and does not backtrack.
The nested quantifier `(?:(?:if|elif|then|while|until|&&|\|\||\||!)[^\S\n]+)*` is the classic
catastrophic-backtracking shape in a PCRE-family engine, and would deserve a warning there. In RE2 it
is linear, so the widening carries no ReDoS risk by construction. The measured 0.025s corroborates it.

### Area 4 — Operational stability of the touched lanes: CLEAN in transcript

Signal sweep over this packet's 104 KB `report.md`, which contains the recorded integration (10,252
lines, exit 0) and e2e (4,528 lines, exit 0) transcripts:

```
# BUG-061-013 stabilize: instability-signal counts in report.md
retry          0
retrying       0
timed out      0
unhealthy      0
restarting     0
flaky          0
panic:         0
DATA RACE      0
health         3
```

No retries, no timeouts, no unhealthy containers, no restarts, no panics, no data races. The three
`health` hits are ordinary readiness references, not failures. Those long lanes were **not** re-run —
there was no reason to, and the regression phase already recorded why.

### OBS-061-013-STAB-01 — network-dependent `apt-get` in all four Go lanes (pre-existing, routed)

**Severity:** medium. **Provenance:** pre-existing (spec-045 / 047 / 052); **not** introduced by this
packet. **Owner:** `bubbles.devops`.

`ensure_envsubst` runs `apt-get update -qq` followed by `apt-get install` against the Debian mirror
whenever `envsubst` is absent, which is every fresh container. Observed live in both runs this phase:
`[go-unit] envsubst missing — installing gettext-base` → `Need to get 160 kB of archives`.

```
# Resilience around that install
$ grep -nE 'retry|--retries|timeout|\|\||set \+e|until |for i in' scripts/runtime/_ensure_envsubst.sh
RESILIENCE_EXIT=1 (1=none present)
```

There is no retry, no timeout, and no offline fallback. Callers run `set -euo pipefail`, so a
transient mirror failure aborts the wrapper **before any test executes**, across all four Go lanes
(unit, integration, e2e, stress).

The failure mode is fail-fast and loud, so this degrades **availability, not correctness** — it cannot
produce a false green. The durable fix is to bake `gettext-base` into the test image so no per-run
network install is needed, which is a `Dockerfile` change owned by `bubbles.devops`. It was not
attempted here: AC-5 requires `scripts/runtime/` stay byte-identical, and image changes are outside a
diagnostic lane.

Worth stating plainly: this packet's contract test is precisely what guarantees that helper is invoked
in the right place. The packet **strengthens** the guard around a dependency whose own resilience is
weak. Fixing the dependency would make the guard cheaper, not redundant.

**Checked and dismissed, so it is not recorded as a finding:** `go-e2e.sh` and
`go-integration.sh` are git mode `100644` while `go-unit.sh` and `go-stress.sh` are `100755`. This is
not a contract violation — the test checks the exec bit on the *helper* only — and it is empirically
harmless, since both lanes ran to `exit 0` this session and are therefore invoked as `bash <script>`.

### Lane evidence — the non-vacuity proof

`--go-run` narrows an otherwise repo-wide run, so `exit 0` is **also** what a zero-match selection
produces. The first capture was therefore rejected as insufficient: its bounded window omitted 500
middle lines and the visible tail was entirely `[no tests to run]`. The window was widened until the
selected package was visible.

```
# BUG-061-013 stabilize run2: NON-VACUITY proof (named PASS lines) + internal/deploy timing
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
exit: 0
lines: 540
sha256: 976c1d16437b2f71207a72dd402e7db05c5aead45658b68725f1843fa803b6ce

ok      github.com/smackerel/smackerel/internal/db      0.020s [no tests to run]
=== RUN   TestEnvsubstWrapperContract_HelperExistsAndIsExecutable
--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
=== RUN   TestEnvsubstWrapperContract_LiveWrappers
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.025s
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/internal/digest  0.028s [no tests to run]
```

Eleven named `--- PASS:` lines — seven top-level plus four subtests — including both fixtures this
packet added. The decisive discriminator is the **absence** of a suffix: `internal/deploy 0.025s`
carries no `[no tests to run]`, while its immediate neighbours `internal/db` and `internal/digest`
both do. A zero-match selection would have rendered `internal/deploy` exactly like its neighbours.
That contrast, not the exit code, is what proves the selector bound real tests.

Replay (selector quoted; it contains no `|`, but quoting it keeps the hint copy-pasteable):

```
bash .github/bubbles/scripts/evidence-capture.sh --lines 280 --label "…" -- \
  ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
```

### Determinism across runs, with an honest caveat

| | run 1 | run 2 |
|---|---|---|
| exit | 0 | 0 |
| lines | 540 | 540 |
| sha256 | `9053b2a826ed0401836d70019198d3a295601790ed892f48b6105e960ac21b58` | `976c1d16437b2f71207a72dd402e7db05c5aead45658b68725f1843fa803b6ce` |
| eleven-PASS set | identical | identical |

The lane is **result-deterministic but not byte-deterministic**: the digests differ because per-package
timings (`0.020s`, `0.028s`, …) and `apt-get` transfer rates vary between runs. The practical
consequence is worth recording for later phases — `evidence-capture.sh --verify <sha>` against this
lane will return **exit 3 (mismatch)** on a clean re-run with nothing regressed. A `--verify` mismatch
here must not be read as a regression; compare the named PASS set instead.

### Not run in this phase

`./smackerel.sh lint` and `./smackerel.sh format --check` were **not** run and no result for them is
claimed: both gate *changed* source, and this phase changed none. The integration, e2e, and stress
lanes were **not** re-run — their recorded transcripts carry zero instability markers and re-running
them would re-derive a property of unmodified files. No security review was performed; that is
`bubbles.security`'s lane. No benchmark was added, because that would require writing into
`internal/`, which AC-5 forbids.

### Uncertainty declaration

`OBS-061-013-STAB-01` is derived from reading the helper's source and observing two live installs; the
mirror-failure path itself was **not** induced, so its abort behaviour is inferred from `set -euo
pipefail` semantics plus the measured absence of retry/timeout/fallback, not observed. Inducing a
network partition inside the test container is a fault-injection exercise that a read-only diagnostic
phase does not perform.

The RE2 linear-time claim is a property of Go's `regexp` package documented by its authors, not
something re-measured here; the 0.025s figure corroborates but does not prove it. The 12,799-byte
input total is measured.

### Completion Statement — stabilize phase

The stabilize phase executed. On the packet's own surface it is a **clean pass, and that conclusion
rests on measurement rather than on the change being small**: nondeterminism sweep empty, `-trimpath`
absent so root resolution is CWD-independent, no detached test binary to desync compile-time paths,
exec bit versioned at `100755`, `-count=1` defeating the test cache, 0.025s at the floor of the prior
band, and backtracking structurally impossible under RE2 rather than merely unobserved.

Failure-mode quality is proven by execution, not argued: the two fixtures this packet added both
passed, and they assert the *content* of the diagnostic — that it says the invocation could not be
located and that the **matcher**, not the wrapper, may need widening.

The one first capture that exited 0 was **rejected** rather than recorded, because its visible tail
was entirely `[no tests to run]` and could not distinguish a real selection from a zero-match one.
Accepting it would have reproduced, inside this packet's own evidence, the exact silent-pass shape the
packet exists to remove.

One real stability weakness exists — the unconditional, unretried network `apt-get` in all four Go
lanes — but it is pre-existing, foreign-owned, degrades availability rather than correctness, and is
routed to `bubbles.devops` rather than fixed here.

No source file was modified. `internal/deploy/envsubst_wrapper_contract_test.go` and
`scripts/runtime/` remain byte-identical to HEAD, so **AC-5 is preserved by this phase**. The only
working-tree changes are this packet's `report.md` and `state.json`. `uservalidation.md` was not
opened and no `## Human Acceptance Record` section was authored (G136 is operator-only). Only
`stabilize` is claimed; `security`, `validate`, and `audit` remain unexecuted and belong to their own
specialists. Packet `status` and `certification.status` remain `in_progress`.

**Routed, not edited (unchanged from the regression and simplify phases' routing):**
`certification.pendingGates` still claims phases are unexecuted that have now run. **Five** have now
executed — `implement`, `test`, `regression`, `simplify`, `stabilize`. The `certification.*` block is
owned by `bubbles.validate`; this phase writes execution progress only, so the count is flagged for
its owner rather than corrected here.

---

# Security phase — `bubbles.security`

### Verdict

🔒 **SECURE** — no security finding is attributable to this packet.

That verdict is recorded with its limits stated. It rests on four executed checks, not on the change
being test-only: the guarded property was traced to its single call site and shown to carry no secret;
the widened matcher was re-derived independently and shown to bind the real invocation in all four
wrappers; the changed file was shown to have no exec, network, or deserialization surface and to run
on RE2; and this packet's artifacts were scanned on disk rather than through the staged-diff path that
would have inspected nothing.

One pre-existing, foreign-owned observation is recorded below (`OBS-061-013-SEC-01`). It is
informational, it is not introduced by this packet, and it does not change the verdict.

### Scope of this phase

Read-only. `internal/deploy/envsubst_wrapper_contract_test.go` and `scripts/runtime/` were **read but
not modified**, so AC-5 is preserved. The only working-tree changes are this packet's `report.md` and
`state.json`.

### What the guarantee protects, and the precise security consequence if it silently breaks

The contract asserts that `ensure_envsubst` is sourced and called BEFORE `go test` in each tracked
wrapper. The obvious hypothesis is that a missing or mis-ordered substitution could leak, blank, or
mis-resolve a secret-bearing value. **That hypothesis is false here, and the reason is worth stating
precisely rather than assumed in either direction.**

`envsubst` has exactly **one** call site in the entire repository:

```
$ grep -rn envsubst scripts/ --include='*.sh'    # invocation sites only
scripts/commands/config.sh:3163:  envsubst "$PROM_SUBST_VARS" < "$PROM_TMPL_FILE" > "$PROM_OUT_FILE"
```

Read at `scripts/commands/config.sh:3155-3167`:

```bash
# Use envsubst to substitute ONLY the named variables. This avoids
# accidental expansion of '$' characters in the template that happen
# to look like env vars but aren't.
PROM_SUBST_VARS='${PROMETHEUS_SCRAPE_INTERVAL_S} ${PROMETHEUS_EVALUATION_INTERVAL_S} ${CORE_CONTAINER_PORT} ${ML_CONTAINER_PORT}'
...
  envsubst "$PROM_SUBST_VARS" < "$PROM_TMPL_FILE" > "$PROM_OUT_FILE"
chmod 0644 "$PROM_OUT_FILE"
```

Three properties follow, each load-bearing for the verdict:

1. **The call uses envsubst's allow-list form.** `envsubst "$PROM_SUBST_VARS"` substitutes ONLY the
   four named variables; every other `$TOKEN` in the template is emitted literally. This is the
   opposite of a leak primitive — it is the control that prevents ambient environment values from
   being interpolated into a rendered artifact. The unrestricted form (`envsubst` with no argument),
   which WOULD expand every variable in the environment, is not used anywhere.
2. **All four substituted variables are non-secret.** Two scrape intervals and two container ports —
   integers. No token, password, or key is in the set.
3. **The secret-bearing artifact does not go through envsubst at all.** Thirty lines earlier, the NATS
   config — the one carrying `${NATS_AUTH_SECTION}` — is rendered by bash and locked down separately:

   ```
   scripts/commands/config.sh:3101:NATS_AUTH_SECTION=""
   scripts/commands/config.sh:3130:printf '%s\n' "$NATS_CONF_CONTENT" > "$NATS_CONF_FILE"
   scripts/commands/config.sh:3131:chmod 0600 "$NATS_CONF_FILE"
   ```

   Different renderer, `0600` rather than `0644`. The envsubst path never handles it.

`scripts/runtime/go-unit.sh:4` states the same boundary in the wrapper itself: *"No secrets pass
through this script — only apt + go test."* That comment is corroborated by the call-site reading, not
taken on trust.

**So the consequence of this guard silently breaking is availability and guard integrity, not
confidentiality.** The failure shape is `envsubst: command not found`, exit 127. Whether that is loud
or silent is the question that actually decides severity, so it was measured rather than assumed:

```
$ grep -rln 'config\.sh' --include='*_test.go' .        # 35 files shell out to config.sh
$ # for each: count exit-ignoring patterns vs error checks
cmd/core/connectors_startup_gate_test.go                    ignored_exit_patterns=0  error_checks=7
internal/config/validate_test.go                            ignored_exit_patterns=0  error_checks=116
internal/deploy/bundle_secret_contract_test.go              ignored_exit_patterns=0  error_checks=105
internal/deploy/envsubst_wrapper_contract_test.go           ignored_exit_patterns=0  error_checks=22
tests/integration/config_validate_test.go                   ignored_exit_patterns=0  error_checks=19
tests/e2e/drive/drive_foundation_e2e_test.go                ignored_exit_patterns=0  error_checks=24
   … all 35 files: ignored_exit_patterns=0
```

**Every one of the 35 callers checks the exit status; none ignores it.** `config.sh` itself opens with
`set -euo pipefail` (line 2), so a 127 aborts generation immediately. And a bad render cannot pass
unnoticed either — `internal/deploy/alertmanager_bundle_contract_test.go:48-64` parses the rendered
`prometheus.yml` as YAML and fails if it lacks an `alerting.alertmanagers` block, which a zero-byte
render necessarily does.

The conclusion is therefore bounded and honest: **this contract is a second line of defence over an
already-loud failure.** Its security value is that it catches the mis-ordering at build time instead of
at lane runtime. It is not the only thing standing between the repository and a silent compromise, and
this phase does not claim it is.

**What the zero-match hole actually meant.** Before the fix, `go-integration.sh` was **silently
unguarded** — the assertion ran, compared nothing, and reported PASS. That is the real security story:
not a leak, but a control that had become decorative on one of four surfaces while continuing to report
success. A guard that cannot fail provides assurance it has not earned, and the assurance is what other
decisions are then built on.

### Does the widened matcher weaken the guarantee?

This was the sharpest risk to check. A locator that binds the **wrong** line would assert ordering
against something inert and pass while the true invocation stayed unguarded — trading a silent pass for
a subtler silent pass. The regression phase measured the binding set as 4 → 5; that was re-derived here
independently, by extracting the literal regex from line 104 and running both the old and new patterns
against the four wrappers, with the literal `go test` occurrences as ground truth.

```
$ OLD='^\s*go\s+test\b'
$ NEW='^[^\S\n]*(?:(?:if|elif|then|while|until|&&|\|\||\||!)[^\S\n]+)*go[^\S\n]+test\b'
$ for w in go-unit go-integration go-e2e go-stress; do grep -Pn "$OLD" / "$NEW" scripts/runtime/$w.sh; done
```

| Wrapper | literal `go test` lines | OLD matches | NEW matches | `ensure_envsubst` at | ordering asserted? |
|---|---|---|---|---|---|
| `go-unit.sh` | 4, 25, 66, 67, 68 | **67** | **67** | 17 | yes (17 < 67) — unchanged |
| `go-integration.sh` | 76, 83, 122 | *(none)* | **76** | 14 | **yes (14 < 76) — was vacuous** |
| `go-e2e.sh` | 89 | **89** | **89** | 14 | yes (14 < 89) — unchanged |
| `go-stress.sh` | 50, 76, 90 | **50**, 90 | **50**, 90 | 11 | yes (11 < 50) — unchanged |
| **total matched lines** | | **4** | **5** | | |

4 → 5 confirmed independently. Three findings follow, and the third is the one that answers the
question:

1. **The single added line is the real invocation.** `go-integration.sh:76` is
   `if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then` — the actual command, not a
   mention of it.
2. **The other three wrappers bind byte-identically before and after.** Same line numbers. The widening
   could not have blunted them, because it did not move them.
3. **The widening opened no path into inert text.** This is the decisive one. Every wrapper contains
   `go test` inside comments or `echo` strings — `go-unit.sh:4,25,66,68` and `go-integration.sh:83,122`
   — and the new pattern matches **none** of them. It cannot: the prefix enumeration is a closed set of
   shell keywords and operators (`if|elif|then|while|until|&&|\|\||\||!`), each requiring trailing
   whitespace, anchored at line start with only horizontal whitespace ahead of it. `#` and `echo` are
   not members. `go-stress.sh:76`'s process substitution `done < <(go test …)` is likewise unmatched,
   which is harmless because line 50 already binds first.

The guard uses `FindStringIndex`, i.e. the **leftmost** match, so the assertion is "the first locatable
invocation follows the first `ensure_envsubst` call". For a false pass the matcher would have to miss
the real invocation **and** bind an inert line sitting after the call. Neither half occurs in any
tracked wrapper, as measured above. **The widened matcher strengthens the guarantee: one vacuous
assertion became live, and the other three are unchanged.**

The residual risk of a bounded enumeration is real — a future `env VAR=x go test` or `timeout N go
test` would not match — but it is now **loud** rather than silent, which is the half of the fix that
does the durable work. That was proven by execution, not argued:
`TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest` asserts the **ORDERING**
error specifically on the `if ! go test … | tee …` shape. Had the matcher bound something inert, that
fixture would surface the *locator* error instead and the test would fail.

One property is unchanged by this packet and is recorded so it is not mistaken for a new guarantee: the
contract compares **textual** order, and shell textual order is not execution order (a function defined
early and invoked late would read as "before"). That asymmetry errs toward a false **failure**, which is
the safe direction, and it predates this change.

### Injection and ReDoS surface of the changed file

The test reads wrapper files and regex-matches them, so path traversal, unsanitised input, and command
execution were each checked. The accurate answer is that none is present, and the reason is structural:

```
$ sed -n '/^import (/,/^)/p' internal/deploy/envsubst_wrapper_contract_test.go
import ( "fmt"  "os"  "path/filepath"  "regexp"  "runtime"  "strings"  "testing" )

$ grep -nE 'os/exec|exec\.Command|syscall|net/http|os\.Getenv|encoding/json|yaml\.|template\.|unsafe' <file>
   exit=1   → no match (1 is clean; 2 would be a grep error)
```

- **No command execution.** No `os/exec`, no `syscall`, no shell. The wrapper bytes are matched
  in-memory and never executed or evaluated. There is no injection sink for the file content to reach.
- **No attacker-controlled path.** Every path component is fixed at compile time:
  `filepath.Join(root, "scripts", "runtime", name)` where `root` derives from `runtime.Caller(0)` and
  `name` iterates the hardcoded `envsubstTrackedWrappers` slice literal. No env var, flag, file
  content, or network input contributes. `os.Getenv` does not appear in the file.
- **No deserialization.** No `encoding/*`, no YAML, no `text/template`, no `unsafe`.

So the honest statement is that this is a pure in-memory regex over an already-read `[]byte`, with no
exec and no external input — not a hazard requiring mitigation.

**ReDoS.** The new pattern contains a nested quantifier, `(?:(?:if|elif|…)[^\S\n]+)*`, which under a
backtracking engine such as PCRE would be a catastrophic-backtracking candidate. It is not one here,
and the reason is the engine rather than the pattern: Go's `regexp` is RE2, which simulates an NFA in
time linear in the input and does not backtrack. That the module uses stdlib RE2 and not a PCRE binding
was verified rather than assumed:

```
$ grep -rnE 'dlclark/regexp2|GRegexp|gijit/pcre|glenn-brown/golang-pkg-pcre' --include='*.go' --include='go.mod' .
   exit=1   → no PCRE binding anywhere in the module; stdlib RE2 only
```

Input size is bounded by the four wrapper files, and the package runs in 0.025s.

### Secret hygiene of this packet's own artifacts

The packet's transcripts are large (`report.md` 2,123 lines before this section; `scopes.md` 516),
which is exactly the condition under which a pasted absolute path or credential slips in. Three
independent checks were run.

**First, the limitation was demonstrated rather than described.** `pii-scan.sh` runs
`gitleaks protect --staged` and greps `git diff --cached`. With a clean tree it inspects nothing:

```
$ git status --porcelain      → (empty)
$ git diff --cached --name-only | wc -l   → 0
$ bash .github/bubbles/scripts/pii-scan.sh
4:06AM INF 0 commits scanned.
4:06AM INF scan completed in 31.9ms
4:06AM INF no leaks found
🫧 pii-scan: clean.
   PII_SCAN_EXIT=0
```

**`0 commits scanned` — that "clean" is vacuous.** It is worth naming the shape: a check reporting
success having examined nothing is precisely the defect class this packet exists to remove. Accepting
it as the secret-hygiene result would have reproduced BUG-061-013 inside the security phase's own
evidence. It is recorded here as a demonstration, not as a pass.

**Second, the files were scanned on disk**, bypassing the staged-diff path entirely:

```
$ gitleaks detect --no-git --source <packet> --config .gitleaks.toml --redact --verbose
4:06AM INF scan completed in 187ms
4:06AM INF no leaks found
GITLEAKS_DETECT_EXIT=0
```

187ms over real file content, against the repo's own rule set — a genuine scan, unlike the 31.9ms
zero-commit run above.

**Third, the machine-local token list and explicit deployment-identity patterns**, reported as counts
only so no value is echoed:

```
   token list: present, 56 non-empty entries
   non-empty non-comment = 23      tokens_checked = 23      → complete coverage, none skipped
   token#1 … token#23              → matches=0 for every token

   home-dir path                      matches=0
   PRIVATE KEY block                  matches=0
   tailnet FQDN (.ts.net)             matches=0
   CGNAT 100.64/10                    matches=0
   public-ish IPv4                    matches=0   (grep_exit=1, re-run under -P)
   AWS-style key id                   matches=0
   bearer/authz header                matches=0
   ssh-rsa/ed25519 pubkey             matches=0
   assigned secret literal            matches=0
```

Two of those lines were nearly wrong, and the corrections are the reason the result can be trusted:

- The token-list sweep checked 23 of 56 non-empty lines. Rather than assume the other 33 were comments,
  the arithmetic was closed: `56 non-empty − 33 comment lines = 23`. Coverage is complete.
- The IPv4 pattern uses a negative lookahead, which **ERE does not support**, so the first run's `0`
  could have been a broken pattern rather than a clean sweep. It was re-run under `grep -P` with the
  exit code read explicitly (**1** = no match, not **2** = error), and then given a positive control:

  ```
  $ printf '203.0.113.7\n' > /tmp/pii-positive-control.txt
  $ grep -PIn '<same pattern>' /tmp/pii-positive-control.txt
  1:203.0.113.7
     control_exit=0   → the harness fires, so the 1 above is a REAL clean sweep
  ```

  Without that control, "no matches" would have been indistinguishable from "the check cannot match
  anything" — the same silent-pass shape again.

**Result: no secret, token, credential, private key, real username, home-directory path, tailnet
identity, or concrete deploy-target name is present in this packet's artifacts.**

### G034 — mechanical security gate

```
# G034 security-gate.sh --repo-root . (BUG-061-013 security phase)
$ bash .github/bubbles/scripts/security-gate.sh --repo-root .
exit: 1
lines: 13
sha256: 2cd11a85cbf5f98814e79f92906b0a9fffedcd39805ac5a881d10b3f4a27a75a
--- output ---
FINDING: inline-credentials: ./scripts/commands/config.sh:866:  POSTGRES_PASSWORD="__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__"
FINDING: inline-credentials: ./scripts/commands/config.sh:1070:  LLM_PROVIDER_SECRET_MASTER_KEY="__SECRET_PLACEHOLDER__LLM_PROVIDER_SECRET_MASTER_KEY__"
FINDING: inline-credentials: ./scripts/commands/config.sh:1236:  TELEGRAM_BOT_TOKEN="__SECRET_PLACEHOLDER__TELEGRAM_BOT_TOKEN__"
FINDING: inline-credentials: ./scripts/commands/config.sh:1542:  KEEP_GOOGLE_APP_PASSWORD="__SECRET_PLACEHOLDER__KEEP_GOOGLE_APP_PASSWORD__"
FINDING: inline-credentials: ./scripts/commands/config.sh:1841:  AUTH_BOOTSTRAP_TOKEN="__SECRET_PLACEHOLDER__AUTH_BOOTSTRAP_TOKEN__"
FINDING: inline-credentials: ./scripts/commands/config.sh:1851:  WEB_REGISTRATION_INVITE_TOKEN="__SECRET_PLACEHOLDER__WEB_REGISTRATION_INVITE_TOKEN__"
FINDING: inline-credentials: ./scripts/commands/config.sh:2239:  ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF="ASSISTANT_TELEGRAM_WEBHOOK_SECRET"
FINDING: inline-credentials: ./scripts/commands/config.sh:2240:  ASSISTANT_TELEGRAM_WEBHOOK_SECRET="test-webhook-secret-061-scope-05-bs001-fixture"
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:56:  echo "FAIL: SST loader returned exit 0 for POSTGRES_PASSWORD=smackerel + TARGET_ENV=self-hosted (expected non-zero)"
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:111:  if grep -qE '^POSTGRES_PASSWORD=__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__$' "$SELF_HOSTED_ENV"; then
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:118:  if grep -qE '^POSTGRES_PASSWORD=smackerel$' "$SELF_HOSTED_ENV"; then
FINDING: inline-credentials: ./scripts/commands/config_secret_rejection_test.sh:122:    echo "PASS: self-hosted.env does NOT contain literal POSTGRES_PASSWORD=smackerel"
[security-gate] FAIL — G034 findings: 12
```

The gate exits 1. **None of the 12 is attributable to this packet, and on inspection none is a true
positive.** Both halves were checked rather than asserted.

**Attribution.** All 12 sit in `scripts/commands/`. This packet's entire source change is one file:

```
$ git diff --stat 40a9e942~1..HEAD -- scripts/ cmd/ internal/
 internal/deploy/envsubst_wrapper_contract_test.go | 100 +++++++++++++++++++++-
 1 file changed, 96 insertions(+), 4 deletions(-)
```

`scripts/` is byte-identical — AC-5 confirmed independently here. Per-line blame puts every flagged
line months before this packet (2026-08-19):

| line | last touched | commit |
|---|---|---|
| `config.sh:866`, `:1841` | 2026-05-15 | `6ead0b27` spec-052 bundle secret injection contract |
| `config.sh:1236`, `:1542` | 2026-05-28 | `4d63905a`, `e22a2cb0` |
| `config.sh:2239`, `:2240` | 2026-05-28 | `6f0b80db` BS-001 Telegram webhook e2e |
| `config.sh:1851` | 2026-06-14 | `b9171571` invite-token-gated registration |
| `config.sh:1070` | 2026-06-18 | `48cfb6fc` multi-provider model registry |

**True-positive assessment.** The gate matches `NAME="value"` lexically and cannot tell a sentinel from
a credential. Each class was read at source:

- **Six `__SECRET_PLACEHOLDER__<KEY>__` hits are a deliberate sentinel**, documented at `config.sh:497`,
  `:853`, `:1812`, `:3062`, `:3299`: the SST loader emits these markers *instead of* literal values for
  production-class targets, so the deploy adapter injects the real secret. This is the mechanism that
  **prevents** a hardcoded credential, and `internal/config/placeholder_runtime_test.go` plus
  `config_secret_rejection_test.sh` assert it holds.
- **`:2239` is a reference name, not a value** — `…_SECRET_REF="ASSISTANT_TELEGRAM_WEBHOOK_SECRET"` names
  the variable to resolve.
- **`:2240` is a test-target-only fixture.** Read in context (`config.sh:2234-2240`) it sits inside
  `if [[ "$TARGET_ENV" == "test" ]]`, with the surrounding comment stating that
  production/self-hosted/dev retain their SST-resolved values. It is self-labelled
  (`…-061-scope-05-bs001-fixture`) and cannot reach a production render.
- **The four `config_secret_rejection_test.sh` hits are negative assertions** — a test proving a weak
  password is REJECTED and does not appear in the rendered env. The inverse of a leak.

So G034's exit 1 reflects a **pre-existing repository-wide baseline of 12 benign lexical matches**, not
a defect in this packet. The gate's inability to distinguish a placeholder sentinel from a credential is
a precision property of the enforcer, which lives under `.github/bubbles/` and is **owned by the
framework**; this phase does not edit it, and the repository's own `gitleaks` configuration — the tool
that gates commits — passes cleanly on the same tree.

### Non-vacuity of this phase's own lane evidence

The guard lane was executed through the repo CLI. The first capture exited 0 over 540 lines, and was
**rejected as proof**, because `--go-run` narrows an otherwise repo-wide run and exit 0 is also what a
zero-match selection produces — the visible window was entirely `[no tests to run]`, with the
`internal/deploy` block in the omitted region (measured: 88 package directories sort before it, putting
it near line 317 of 540). Widening the window far enough to show it would have pasted essentially the
whole transcript, so a **differential control** was used instead — the identical lane, with a selector
that matches no test:

| | selector `TestEnvsubstWrapperContract` | negative control `…_NoSuchTestZZZ_NegativeControl` |
|---|---|---|
| exit | **0** | **0** |
| lines | **540** | **519** |
| sha256 | `ded98abca5f83cc5883fea0b0e8c7db6d6c578ab479fe9dc367ff39c900b88fc` | `7858a32ec9c2dc070acfd63ef89389d2f88147dde74fb45f45324e4b4bbbaa9c` |

Both exit 0, which demonstrates rather than merely asserts that **the exit code carries no information
here**. The 21-line delta does, and it reconciles exactly with the test inventory:

```
   top-level tests matching the selector = 7   (lines 161, 191, 211, 230, 250, 278, 309)
   LiveWrappers subtests                 = 4   (one per tracked wrapper)
   total test entities                   = 11

   predicted: 11 '=== RUN' + 11 '--- PASS:' − 1 'no tests to run' = 21
   observed delta (540 − 519)                                     = 21
```

Predicted equals observed with nothing left over, so all 11 entities — the four live wrapper subtests
and the five adversarial rejections, including both added by this packet — executed and passed. The
`internal/deploy` package's `ok` line necessarily carries no `[no tests to run]` suffix, because those
21 lines exist only if tests ran.

Replay hints (the selector contains no `|`; both are quoted for safety):

```
bash .github/bubbles/scripts/evidence-capture.sh --verify ded98abca5f83cc5883fea0b0e8c7db6d6c578ab479fe9dc367ff39c900b88fc -- ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
bash .github/bubbles/scripts/evidence-capture.sh --verify 7858a32ec9c2dc070acfd63ef89389d2f88147dde74fb45f45324e4b4bbbaa9c -- ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_NoSuchTestZZZ_NegativeControl' --verbose
```

Per the stabilize phase's recorded caveat, this lane is result-deterministic but not byte-deterministic
(per-package timings and `apt-get` transfer rates vary), so `--verify` will return **exit 3** on a clean
re-run. That mismatch is not a regression; compare the 21-line delta and the named PASS set instead.

### OBS-061-013-SEC-01 — the rendered monitoring config is truncated before the failed exec (pre-existing)

**Severity: LOW / informational. Availability and observability, not confidentiality. Not introduced by
this packet. Owner: `bubbles.devops`, jointly with `OBS-061-013-STAB-01` — both concern
`scripts/commands/` and `scripts/runtime/`, which AC-5 puts beyond this packet's reach.**

While tracing the guarded property, one behaviour was measured that sharpens why the ordering matters.
At `config.sh:3163` the shell sets up `> "$PROM_OUT_FILE"` **before** attempting to exec `envsubst`, so
a missing binary truncates the previously-rendered `config/generated/prometheus.yml` to zero bytes and
only then fails. Verified outside the repository tree:

```
$ printf 'PRE-EXISTING RENDERED CONFIG\n' > out.yml
$ ( set -euo pipefail; definitely_not_a_real_binary_envsubst '${X}' < tmpl > out.yml )
before: size=29 content=[PRE-EXISTING RENDERED CONFIG]
definitely_not_a_real_binary_envsubst: command not found
subshell_exit=127
after:  size=0  content=[]
```

The exposure is tightly bounded, which is why this is informational rather than a finding, and each
bound was checked rather than assumed:

- `config.sh` runs under `set -euo pipefail`, so generation aborts at that line — the failure is loud.
- The artifact is **gitignored** (`.gitignore:17`) and untracked, so a truncated render cannot
  propagate through version control.
- Its only consumer is `docker-compose.yml:317`, a read-only bind mount into Prometheus.
- `internal/deploy/alertmanager_bundle_contract_test.go` parses the rendered file and requires an
  `alerting.alertmanagers` block, so a zero-byte render fails a contract test.

The residual path requires an operator to ignore a failed `config generate` and start the stack anyway,
which would leave Prometheus with no scrape configuration — a monitoring blind spot (OWASP A09) rather
than a disclosure. A durable remedy would render to a temporary file and move it into place only on
success, so a failed generation leaves the previous config intact. That work belongs to the owner named
above; this phase records it and modifies nothing.

### Not run in this phase

`./smackerel.sh lint` and `./smackerel.sh format --check` were **not** run and no result is claimed for
them: both gate changed source, and this phase changed none. The integration, e2e, and stress lanes were
**not** re-run; the security-relevant property of those wrappers is their `ensure_envsubst` ordering,
which was verified statically above against the wrapper files themselves. No dependency-vulnerability
scan (`govulncheck`, `nancy`) was run: this packet adds no dependency, `go.mod` is untouched, and a
module-wide CVE sweep would report the repository's standing posture rather than anything about this
change. No penetration test or fault injection was performed — inducing a missing `envsubst` inside the
container is fault injection, which a read-only diagnostic phase does not perform.

### Uncertainty declaration

The `OBS-061-013-SEC-01` truncation mechanism was reproduced directly, but the **consequence** of a
zero-byte `prometheus.yml` reaching a running Prometheus was **not** observed — no stack was started
against a truncated render. The four bounds listed above were each verified; the residual operator path
is reasoned from them, not measured.

The claim that no test tolerates a `config.sh` failure rests on a pattern sweep for exit-ignoring idioms
across all 35 caller files (`ignored_exit_patterns=0` in every one) plus the presence of substantial
error checking in each. It is a strong signal, not an exhaustive proof that no caller anywhere swallows
a specific error branch.

The RE2 linear-time property is documented by Go's authors and corroborated by the absence of any PCRE
binding in the module; it was not re-measured under adversarial input here.

The G034 true-positive assessment rests on reading each flagged line in its source context. That reading
is reproducible from the file/line references given, and the sentinel mechanism is independently
asserted by the repository's own placeholder and secret-rejection tests.

### Completion Statement — security phase

The security phase executed, and its verdict is 🔒 **SECURE** with no finding attributable to this
packet.

The guarded property was traced to its single call site rather than assumed. The tempting hypothesis —
that a broken substitution could leak or blank a secret — is **false here**, because `envsubst` is
invoked in its allow-list form over four non-secret numeric variables, while the one secret-bearing
artifact is rendered by a different mechanism at `0600`. The real security story is the one the packet
already names: `go-integration.sh` was silently unguarded, and a control that reports success without
comparing anything supplies assurance it has not earned.

The widened matcher does **not** weaken the guarantee. Re-derived independently, the binding set goes
4 → 5; the single addition is the genuine invocation at `go-integration.sh:76`; the other three wrappers
bind byte-identically; and the closed prefix enumeration provably admits no comment or `echo` line, so
the widening opened no route onto inert text.

The changed file has no exec, network, or deserialization surface, and every path component is fixed at
compile time — so path traversal and command injection are absent by construction rather than mitigated.
Catastrophic backtracking is impossible under RE2, verified as the engine actually in use.

Secret hygiene is clean, and clean by a check that could have failed: the staged-diff scan was shown to
inspect **0 commits** and its "clean" was rejected as vacuous, the artifacts were scanned on disk
instead, the token-list arithmetic was closed at 23 of 23, and the IPv4 pattern was re-run under a
PCRE-capable grep with a positive control after ERE silently could not honour its lookahead.

The one lane capture that exited 0 with an all-`[no tests to run]` window was **rejected** rather than
recorded, and replaced with a differential negative control whose 21-line delta reconciles exactly with
the 11-entity test inventory — so the assertion that the guard ran and passed is arithmetic, not
eyeballing.

G034's mechanical enforcer exits 1 on a pre-existing baseline of 12 lexical matches in
`scripts/commands/`, every one last touched months before this packet, and every one a placeholder
sentinel, a reference name, a labelled test fixture, or a negative assertion in a secret-rejection test.
One informational observation, `OBS-061-013-SEC-01`, is recorded with its owner named.

No source file was modified. `internal/deploy/envsubst_wrapper_contract_test.go` and `scripts/runtime/`
remain byte-identical to HEAD, so **AC-5 is preserved by this phase**. The only working-tree changes are
this packet's `report.md` and `state.json`. `uservalidation.md` was not opened and no
`## Human Acceptance Record` section was authored (G136 is operator-only). Only `security` is claimed;
`validate` and `audit` remain unexecuted and belong to their own specialists. Packet `status` and
`certification.status` remain `in_progress`.

**Routed, not edited (unchanged from the regression, simplify, and stabilize phases' routing):**
`certification.pendingGates` still claims phases are unexecuted that have now run. **Six** have now
executed — `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`. The `certification.*`
block is owned by `bubbles.validate`; this phase writes execution progress only, so the count is flagged
for its owner rather than corrected here.


# Audit phase — `bubbles.audit`

### Verdict

**AUDIT CLEAN — no defect found.** The fix genuinely closes the defect it claims; it does not merely
appear to. Every central claim below was **re-derived by execution in this session**, not read from a
prior phase's transcript.

This verdict is bounded, and the bound is stated rather than implied: it certifies **evidence integrity
and completion honesty** for the audit phase. It is **not** a delivery certification. `validate` remains
unexecuted and belongs to its own specialist; `status` and `certification.status` stay `in_progress`.

### Scope of this phase

Read-only adversarial review of commit `40a9e942` and the packet's ten commits. No source file, no
wrapper, and no foreign artifact was modified. The prior phases were **not** re-run; they were attacked.

### The trap this packet is about, demonstrated before relying on any selector

`--go-run` narrows the selection, so **exit 0 is also what a zero match produces**. Before trusting any
focused run, the audit established the discriminator with a selector that cannot match anything:

```
$ timeout 900 ./smackerel.sh test unit --go --go-run 'TestNoSuchTestExistsAnywhere_AuditControl' --verbose
ok      github.com/smackerel/smackerel/internal/deploy  0.021s [no tests to run]
LANE_EXIT=0
```

Zero `=== RUN` lines, green lane, exit 0. The discriminator is therefore the `[no tests to run]` suffix
and the presence of named `=== RUN` / `--- PASS` lines — never the exit code.

### Audit Evidence

The audit phase's findings are recorded as AUDIT-01 through AUDIT-09 below. They are grouped
under this heading because `bugfix-fastlane` requires a section by this name; the content was
already present and is unchanged.

### AUDIT-01 — the tests actually execute (exit 0 was not accepted as proof)

```
$ timeout 900 ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
=== RUN   TestEnvsubstWrapperContract_HelperExistsAndIsExecutable
--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
=== RUN   TestEnvsubstWrapperContract_LiveWrappers
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.01s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.048s
LANE_EXIT=0
```

Five functions and four subtests genuinely ran. The package line reads `ok … 0.048s` **without** the
`[no tests to run]` suffix that the control run carried — the two runs differ exactly where they must.

**Note the property the fix creates:** `LiveWrappers/go-integration.sh` passing is now *load-bearing*.
Before the fix that subtest passed vacuously; after it, a zero match is fatal, so its green is a
positive assertion that the invocation was located **and** the ordering compared.

### AUDIT-02 — the defect and the fix, re-derived directly against the live wrappers

Both regexes were lifted from the source and applied to the four wrappers independently of the test:

```
=== OLD regex (pre-fix)  ^\s*go\s+test\b ===
go-unit.sh       1
go-integration.sh 0          <-- the defect: matched NOTHING, yet reported GREEN
go-e2e.sh        1
go-stress.sh     2

=== NEW regex (post-fix) ===
go-unit.sh       count=1  lines=67
go-integration.sh count=1  lines=76
go-e2e.sh        count=1  lines=89
go-stress.sh     count=2  lines=50,90
```

This reproduces the packet's measured claim **exactly**: before-counts `1 / 0 / 1 / 2`, after-lines
`67 / 76 / 89 / 50`. The binding set moves **4 → 5**, the single addition being the genuine invocation
at `go-integration.sh:76`, which at HEAD reads:

```
if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
```

The security phase's independently-stated "4 → 5, single addition" is therefore corroborated by
measurement rather than accepted on assertion.

### AUDIT-03 — the widening landed on real invocations, not inert text

`FindStringIndex` returns only the **first** match, so `go-stress.sh` having two matches is a place a
widening could silently bind to the wrong thing. Both are real invocations, and every wrapper orders
the helper first:

```
go-stress.sh:50  go test -tags stress -v -count=1 -timeout 90s -run '^TestStressReadinessCanary_Live$' ./tests/stress/readiness
go-stress.sh:90          go test "${go_test_args[@]}" "$package_path"

go-unit.sh         ensure_envsubst@17   firstGoTest@67   ordered=YES
go-integration.sh  ensure_envsubst@14   firstGoTest@76   ordered=YES
go-e2e.sh          ensure_envsubst@14   firstGoTest@89   ordered=YES
go-stress.sh       ensure_envsubst@11   firstGoTest@50   ordered=YES
```

### AUDIT-04 — the regression tests are ADVERSARIAL, not tautological

This is the check that decides whether the fix is real or cosmetic. A regression that passes whether or
not the bug returns is worthless. Each new fixture was tested against **both** regexes:

```
FIXTURE A (unlocatable):  output=$(go test ./... 2>&1)
  NEW regex: NO MATCH  -> genuinely exercises the hard-failure branch

FIXTURE B (conditional):  if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
  OLD regex: NO MATCH  -> reverting the widening breaks this test (it would see a LOCATOR error,
                          not the asserted ORDERING error)
  NEW regex: MATCH     -> the widening is what makes the ordering assertion reachable
```

Both halves of the fix are **independently** guarded, which is the property that matters:

- Revert the hard-failure to `return nil` → fixture A returns nil → `AdversarialRejectsUnlocatableInvocation` fails.
- Revert the widening → fixture B yields the locator error → `AdversarialRejectsConditionalCallAfterGoTest` asserts the *ordering* substring and fails.

Neither test can pass through the bug. The packet's decision to assert the **specific** error substring
rather than "some error" is what buys the second property, and it is load-bearing.

### AUDIT-05 — AC-5 verified across all ten packet commits, not just the fix

AC-5 requires `scripts/runtime/` to stay byte-identical — the wrapper must not be edited to satisfy its
own detector. Checked per commit, not merely at the tip:

```
f48b6642 : offending=0      7acbaaf3 : offending=0
40a9e942 : offending=0      9af5e309 : offending=0
ffc7f60d : offending=0      65ec19b9 : offending=0
d0b42dfe : offending=0      b316e027 : offending=0
                            cc43f087 : offending=0
                            8cdc31e8 : offending=0

$ git status --porcelain -- scripts/runtime/ internal/
(empty — clean)

$ git show --stat 40a9e942
 internal/deploy/envsubst_wrapper_contract_test.go  | 100 ++++++++-
 .../report.md                                      | 242 +++++++++++++++++++++
 .../scopes.md                                      | 236 +++++++++++++++++++-
 3 files changed, 563 insertions(+), 15 deletions(-)
```

One naive check deserves recording because it is misleading and a future reader will hit it: a diff over
the raw linear range `f48b6642^..HEAD -- scripts/runtime/` **does** report a change to
`go-integration.sh`. That change belongs to commit `fa61daa0` (**BUG-061-011**, dated `05:41:57`,
*sixteen hours before* this packet's fix at `21:58:27`) — a different packet's simplify phase, which
explicitly recorded that it left line 76 alone so this bug would stay unmasked. It is **interleaved
foreign work, not this packet's**. Confirmed decisively:

```
$ git log --format='%h %s' 40a9e942..HEAD -- scripts/runtime/
(empty — nothing after the fix touched scripts/runtime/)
```

**AC-5 is intact.**

### AUDIT-06 — phase attribution is truthful

Seven `executionHistory` entries, one per agent, spans non-uniform (9, 11, 17, 48, 24, 15, 24 minutes) —
no fabrication-indicating uniform spacing. The cited pre-fix baseline is real and is exactly the right
commit:

```
d08013e6 2026-08-19T21:15:57+00:00  validate(BUG-061-011): …
parent of 40a9e942 = d08013e6
```

The red-then-green probe `TestZZBug061013PreFixProbe` is **not** in the tree (`grep` → 0 hits), which is
correct: a probe that exercises pre-fix code cannot be kept. Its captures are therefore not replayable —
but the property they establish **is** independently re-derivable, and AUDIT-04 re-derived it.

### AUDIT-07 — replay hints are runnable (the defect class a prior phase already caught once)

The regression phase found and fixed an unquoted `--go-run` alternation that would parse as a shell
pipeline. Swept for recurrence across the packet:

```
=== UNQUOTED --go-run containing a shell metachar (would misparse) ===
--- end (empty = clean) ---

quoted-with-pipe   : 2
UNquoted-with-pipe : 0
```

Not merely well-formed — **executed verbatim**:

```
$ timeout 900 ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsMissingSource|TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall|TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest' --verbose
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.080s
LANE_EXIT=0
```

Three real tests selected and run — not a zero match. T-05 holds.

### AUDIT-08 — DoD honesty: 14 of 14 checked, none on weaker evidence than it asserts

Two items were spot-checked because they are the ones most likely to be asserted rather than run:

- **The two ADVERSARIAL red-then-green items** claim `nil → error` on identical input across the fix
  boundary. Independently corroborated by AUDIT-04 without relying on the vanished probe capture.
- **The full e2e lane item (T-08)** is the most expensive claim in the packet. Its evidence is a real
  capture block (`exit: 0`, `lines: 4528`, `sha256 eb5e8fa3…`) and it explicitly declines to lean on
  exit 0 alone, naming the terminal check `PASS: go-e2e-corpus-enforce` and the stack-teardown lines as
  the load-bearing signals — *because a lane exiting 0 while a check inside it quietly declines to run
  is this bug's own failure mode*. Its provenance is attributed honestly ("by the ORCHESTRATOR
  (`bubbles.goal`) … OBSERVED, not produced, by `bubbles.test`") rather than claimed as own work.

Evidence provenance across the report is **27 `Claim Source: executed` and 0 `interpreted`**, with six
explicit uncertainty declarations. There are no interpreted claims requiring reviewer spot-check.

### AUDIT-09 — overclaim review: none found

- **Security verdict.** States `SECURE` *with its limits attached*, not as a blanket clean bill: an
  explicit "Not run in this phase" section, an uncertainty declaration conceding the truncation
  *consequence* was "reasoned from them, not measured", the caller sweep called "a strong signal, not an
  exhaustive proof", and RE2 linearity "not re-measured under adversarial input here". Its `4 → 5`
  binding-set claim is corroborated by AUDIT-02. Its one observation is marked pre-existing and
  foreign-owned rather than attributed to this packet.
- **Simplify no-op.** Honest and *quantified*, not lazy: the table-driven refactor was measured as
  net-negative (42 skeleton lines → roughly 54) and rejected on a semantic ground that is precisely this
  packet's failure mode — a blanked table field still compiles and still passes. It also flagged a
  genuine residual inaccuracy and routed it instead of silently fixing it. **Verified still present:**
  line 75's comment says `(?m)^\s*` while line 80's pattern is `(?m)^[^\S\n]*`. It is inert (both match
  leading horizontal whitespace) and correctly left to its owner.

### Observations (non-blocking; for `bubbles.validate` and the operator)

1. `OBS-061-013-AUDIT-01` — **cosmetic execution-block drift.** `execution.completedPhaseClaims` lists
   four phases while top-level `completedPhases` lists six, and `execution.activeAgent` /
   `execution.currentPhase` still read `simplify` although `stabilize`, `security` and now `audit` have
   run. This is **not** a functional gap, and the guard's own output proves it: `stabilize` and
   `security` are absent from `completedPhaseClaims` yet the guard does **not** report them missing, so
   it reads the **union** of both records. Left uncorrected here deliberately — the same treatment
   `stabilize` and `security` gave it — and routed to `bubbles.validate`, which owns reconciliation.
2. `OBS-061-013-AUDIT-02` — the guard warns "20 of 68 evidence blocks lack terminal output signals
   (potentially fabricated)". Assessed as **benign**: the blocks in question are source excerpts, regex
   listings and file dumps rather than terminal claims, and the provenance discipline is 27 executed /
   0 interpreted. Recorded so the warning is not silently absorbed.
3. `certification.pendingGates` prose remains stale — already routed by the regression, simplify,
   stabilize, and security phases to `bubbles.validate`. Unchanged here; audit does not write
   `certification.*`.

### Not run in this phase

`./smackerel.sh lint` and `./smackerel.sh format --check` were **not** re-run and no result is claimed
for them: both gate changed source, and this phase changed none. The integration, e2e and stress lanes
were **not** re-run — re-running them would re-execute the regression phase's work rather than audit it,
and the audit's contribution is independent re-derivation of the central claims, which was performed
against the wrapper files and the committed tests directly. No fixture was mutated in-tree to force the
hard-failure branch, because AC-5 forbids editing `internal/`; the equivalent property was established
analytically in AUDIT-04 instead.

### Uncertainty declaration

The pre-fix probe captures (`9e2e3abf…`, `c049764…`) and the e2e capture (`eb5e8fa3…`) cite sha256
digests over evidence-capture output whose temporary directories no longer exist, so those specific
hashes cannot be re-verified now. This is a property of the capture tool, **not** a packet defect — the
packet mitigated it correctly by copying the decisive windows into the artifacts. Where it mattered, the
underlying property was re-derived by other means rather than trusted (AUDIT-04).

The regex comparison in AUDIT-02 and AUDIT-04 was performed with PCRE `grep -P` against patterns Go
compiles under RE2. The constructs used here (anchors, character classes, non-capturing groups,
alternation, `\b`) carry identical semantics in both engines, and the results agree with the committed
test's live behaviour in AUDIT-01, which is the corroborating control. It was not verified under a Go
harness, because building one would require creating a package outside the permitted change surface.

### Completion Statement — audit phase

The audit phase executed. Verdict: **AUDIT CLEAN — no defect found.** No finding is attributable to this
packet, and the verdict is recorded with its limits rather than as a blanket pass.

The packet's central claim survives adversarial re-derivation. The defect was real (`go-integration.sh`
matched **nothing** while reporting green), the fix locates it at line 76, the binding set moves 4 → 5,
and — the part that decides soundness — **both** halves of the fix are independently guarded by
regression tests that cannot pass through the bug. The subject was never edited to satisfy its own
detector: AC-5 holds across all ten packet commits and the working tree, and the one apparent
counter-example is interleaved foreign work from BUG-061-011 that predates the fix by sixteen hours.

Exit 0 was never accepted as proof. The zero-match trap this packet documents was first demonstrated on
a control selector, and every subsequent claim of execution rests on named `=== RUN` / `--- PASS` lines.

No source file was modified. `internal/deploy/envsubst_wrapper_contract_test.go` and `scripts/runtime/`
remain byte-identical to HEAD, so **AC-5 is preserved by this phase**. The only working-tree changes are
this packet's `report.md` and `state.json`. `uservalidation.md` was not opened and no
`## Human Acceptance Record` section was authored (G136 is operator-only). Only `audit` is claimed;
`validate` remains unexecuted and belongs to its own specialist. Packet `status` and
`certification.status` remain `in_progress` — audit reports evidence integrity; it does not certify.

---

## Validate phase — `bubbles.validate`

### Verdict

**CERTIFIED on the technical surface; packet status `blocked` on G136 alone.** Scope
`BUG-061-013-SCOPE-01` is Done, all 14 DoD items are backed by evidence that demonstrates what each item
actually asserts, and no finding is attributable to this packet. `status` is **not** set to `done`,
because human acceptance has not occurred and cannot be manufactured — see *The G136 boundary* below.

This phase did **not** rubber-stamp seven green phases. Every load-bearing claim below was re-derived by
execution or by direct source inspection in this session. Where a prior phase's conclusion was
interpretive rather than mechanical, the interpretation was checked against the source it rests on.

### Validation Evidence

#### V-01 — The trap first: the zero-match discriminator, established in THIS session

`--go-run` narrows selection, so exit 0 is also what a zero match produces. Before trusting any focused
run, a selector that cannot match anything was executed and its signature recorded.

**Claim Source:** executed · **Exit Code:** 0 (both runs)

```
# NEGATIVE CONTROL — selector matches nothing
$ ./smackerel.sh test unit --go --go-run 'TestZZZValidateNegativeControlMatchesNothing' --verbose
NEGCTL_LANE_EXIT=0
TOTAL_LINES=519
=== RUN lines: 0
ok      github.com/smackerel/smackerel/internal/deploy  0.078s [no tests to run]
```

Both the control and the real run exit 0. The exit code therefore carries no information here, and every
execution claim below rests on named `=== RUN` / `--- PASS` lines and on the **absence** of the
`[no tests to run]` suffix.

#### V-02 — The contract family genuinely executes

**Claim Source:** executed · **Exit Code:** 0 · 540 lines ·
sha256 `fdc36c2a71924c50a8b37ec78eb2bce2def87cff384bdcbac1dcc8afaa0f95d9`

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
=== RUN   TestEnvsubstWrapperContract_HelperExistsAndIsExecutable
--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
=== RUN   TestEnvsubstWrapperContract_LiveWrappers
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
=== RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.028s
```

Seven functions and four subtests, each named. The package line carries **no** `[no tests to run]`
suffix, which is exactly where this run differs from V-01's control. Covers DoD SCN-01, SCN-02, SCN-03,
SCN-05, and the two adversarial red-then-green items on their green side.

#### V-03 — The defect and the fix, re-derived against the live wrappers, independently of the test

Both regexes were lifted from the committed source and applied directly to the four tracked wrappers
with PCRE, so this measurement does not depend on the test that is under validation.

**Claim Source:** executed

```
OLD  ^\s*go\s+test\b
NEW  ^[^\S\n]*(?:(?:if|elif|then|while|until|&&|\|\||\||!)[^\S\n]+)*go[^\S\n]+test\b

scripts/runtime/go-unit.sh         OLD 67          NEW 67
scripts/runtime/go-integration.sh  OLD (NONE)      NEW 76
scripts/runtime/go-e2e.sh          OLD 89          NEW 89
scripts/runtime/go-stress.sh       OLD 50, 90      NEW 50, 90

go-integration.sh:76 = if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
```

This reproduces the packet's measured claim exactly: the binding set moves 4 → 5, and the single
addition is the genuine invocation the old pattern could not reach. The defect was real.

#### V-04 — The ordering comparison is genuinely performed, so `go-integration.sh` PASS is load-bearing

**Claim Source:** executed

```
wrapper                 ensure_envsubst      go test (first match)
go-unit.sh              17                   67
go-integration.sh       14                   76
go-e2e.sh               14                   89
go-stress.sh            11                   50
```

`ensure_envsubst` precedes the located invocation in all four. Combined with the zero-match branch now
being fatal, the green `LiveWrappers/go-integration.sh` subtest is a positive assertion that the
invocation was **located** and the ordering **compared** — where before the fix it passed vacuously.

#### V-05 — The two new adversarial tests are NON-VACUOUS

This is the check that separates a real fix from a decorative one, and it was made against the fixtures
themselves rather than assumed. Both fixtures are raw string literals in the test file, so the committed
matcher can be applied to them directly.

**Claim Source:** executed

```
NEW regex applied to internal/deploy/envsubst_wrapper_contract_test.go:
  215: go test ./...
  235: go test ./...
  255: go test ./...
  314: if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then

fixture lines, verbatim:
  284: output=$(go test ./... 2>&1)      <- NOT in the match list
  314: if ! go test ... | tee ...        <- IS in the match list
```

Line 284 is the `RejectsUnlocatableInvocation` fixture: it genuinely **evades** the widened matcher, so
that test exercises the real zero-match branch rather than passing for an unrelated reason. Line 314 is
the `RejectsConditionalCallAfterGoTest` fixture: it genuinely **matches**, so its assertion on the
*ordering* substring proves the widening landed on a real invocation. Had the widening landed on
something inert, that test would have surfaced the locator error and gone red. Neither test is
tautological.

#### V-06 — Replay hints are runnable, including the alternation class a prior phase already fixed

Every `--go-run` selector in the packet was enumerated and checked for shell-parse hazards.

**Claim Source:** executed · **Exit Code:** 0

```
hazard class — --go-run with an UNQUOTED value containing | ( ) ^ $ * :
  (no matches)

the one alternation selector, executed end-to-end:
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsMissingSource|TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall|TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest' --verbose
ALT3_LANE_EXIT=0
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
=== RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.044s
```

The alternation is single-quoted at every occurrence, so it reaches `go test` as one argument instead of
parsing as a shell pipeline. The hint is runnable, and running it also discharges the DoD item asserting
the three pre-existing adversarial sub-tests were not blunted (T-05).

#### V-07 — AC-5 verified across the packet's whole commit range, not just the fix

**Claim Source:** executed

```
$ git status --porcelain -- scripts/runtime/ internal/
(empty — working tree clean on both paths)

$ git diff --stat 40a9e942^..HEAD -- scripts/runtime/
(empty — scripts/runtime BYTE-IDENTICAL from the fix's parent through HEAD)

$ git diff --stat 40a9e942^..HEAD -- scripts/ cmd/ internal/
 internal/deploy/envsubst_wrapper_contract_test.go | 100 +++++++++++++++++++++-
 1 file changed, 96 insertions(+), 4 deletions(-)

$ git show --name-only 40a9e942
internal/deploy/envsubst_wrapper_contract_test.go
<packet>/report.md
<packet>/scopes.md
```

Across every BUG-061-013 commit, the only non-artifact file touched is the one test file. Two commits in
the same log window do touch `internal/deploy/eval_lane_contract_test.go` and
`scripts/runtime/go-integration.sh`, but both belong to sibling BUG-061-011 and both **precede**
`40a9e942`; the `40a9e942^..HEAD` diffs above are bounded to this packet and are the decisive form.

The scratch probe used for the pre-fix red-then-green was confirmed genuinely disposed of: absent from
the working tree, and `git log --all -- internal/deploy/zz_bug061013_prefix_probe_test.go` returns
nothing, so it never entered a commit and cannot have widened the change surface.

#### V-08 — The prior phases' interpretive claims were checked against source, not accepted

The test phase explained two failure-shaped lines inside otherwise-green live lanes as required output of
deliberate negative fixtures. That is an interpretation, and interpretations are where a green lane can
hide a real failure, so each was verified at its source.

**Claim Source:** executed

```
tests/e2e/lib/helpers.sh:103
  echo "FAIL: Services did not become healthy within ${timeout}s"

all e2e_wait_healthy call sites:
  tests/e2e/lib/helpers.sh:47              e2e_wait_healthy 120
  tests/e2e/lib/helpers.sh:53              e2e_wait_healthy 120
  tests/e2e/run_all.sh:81                  e2e_wait_healthy 120
  tests/e2e/test_persistence.sh:25         e2e_wait_healthy 120
  tests/e2e/test_persistence.sh:53         e2e_wait_healthy 120
  tests/e2e/test_postgres_readiness_gate.sh:24   e2e_wait_healthy 8   <- sole 8s call site

tests/integration/config_validate_test.go
  74  func buildOversizedEnvFile(...)
  106 ML_MODEL_MEMORY_PROFILES_JSON ... "bug-045-fixture-llm-20gib","weights_mib":20480
  138 func TestConfigValidate_AC5c_BinaryRejectsOversizedModel(...)
```

Both interpretations hold. The `8s` timeout is unique to the readiness-gate canary, which stops postgres
on purpose, and the oversized-model fixture exists exactly as described. In both lanes the
failure-shaped line is the assertion **passing**; its absence is what would turn the lane red. The
T-06 and T-08 DoD items therefore rest on evidence that means what they claim.

#### V-09 — Build Quality Gate, re-run rather than inherited

**Claim Source:** executed

```
$ ./smackerel.sh lint
exit: 0 · 157 lines · sha256 9cef6098a51e7e1f2321a9d65bf994d1a511488880bb13a790242bf58c0d393e
  Web validation passed

$ ./smackerel.sh format --check
exit: 0 · 136 lines · sha256 655526ae031b729a88f829c04ffb4b614d9163b5cd25529d72a7d76aaea2d999
  78 files already formatted

$ bash .github/bubbles/scripts/artifact-lint.sh <packet>
  Artifact lint PASSED.        ARTIFACT_LINT_EXIT=0

$ bash .github/bubbles/scripts/pii-scan.sh
  no leaks found · pii-scan: clean.     PII_SCAN_EXIT=0
```

The audit phase correctly declined to claim `lint` and `format` because it changed no source. This phase
re-ran both rather than inheriting the test phase's result, and additionally confirmed no packet artifact
contains an absolute home-directory path.

### DoD versus evidence — the judgement

All 14 checked items in `scopes.md` are backed by evidence that demonstrates what the item asserts, not
merely something adjacent to it. The items that could most easily have been over-claimed were checked
specifically:

| DoD item | What it asserts | Why the evidence actually demonstrates it |
|---|---|---|
| SCN-01 / SCN-02 | absence is rejected, and the message blames the *matcher* | V-05 proves the fixture evades the matcher, so the branch is genuinely reached; V-02 shows the test named and passing |
| SCN-03 / SCN-05 | `go-integration.sh:76` is located and its ordering compared | V-03 locates it at 76 independently; V-04 shows ordering holds; the zero-match branch makes the green load-bearing |
| adversarial red-then-green (both) | verdict differs across the change on the SAME input | executed via a scratch probe on both sides with recorded verdict lines, and the probe was proven never committed (V-07) |
| T-05 trio not blunted | three pre-existing fixtures still reject | re-executed in V-06 with named RUN/PASS |
| T-06 / T-08 live lanes | the wrappers the contract governs still run | exit 0 plus V-08's source-level confirmation that the failure-shaped lines are passing negative fixtures |
| AC-5 / change boundary | one file, `scripts/runtime/` untouched | V-07, in three independent diff forms |
| Build Quality Gate | lint, format, artifact-lint clean | V-09, re-run this session |

Phase attributions are truthful. Each of the seven recorded phases has exactly one `executionHistory`
entry written by the matching specialist, and the git log carries one commit per phase in the same
order. No agent claims a phase it did not execute. T-06 and T-08 are explicitly attributed to the
orchestrator as **observed, not produced** — an under-claim, which is the safe direction.

The stated bounds are accurate rather than decorative. Security states its limits and its read-only
scope; simplify is an honest no-op, confirmed by `git diff --quiet 40a9e942 HEAD -- <file>` returning
identical; audit bounds itself to evidence integrity and completion honesty and says in terms that it is
**not** a delivery certification.

### Finding

**No blocking finding.** No defect is attributable to this packet.

One non-blocking observation was carried into this phase and is now reconciled, because it was the only
statement in the packet that was actually **false** rather than merely incomplete:

- `OBS-061-013-AUDIT-01` / stale `certification.pendingGates`. The pending-gate prose still asserted that
  seven specialist phases remained unexecuted and that `## Validation Evidence` was "absent by design",
  both of which this session made untrue. `execution.activeAgent` and `execution.currentPhase` still read
  `simplify` after three further phases had run. Certification reconciliation is this phase's ownership,
  so those fields are rewritten to the truth and `validate` is claimed with first-hand provenance. The
  residual under-claim in `execution.completedPhaseClaims` for `stabilize`, `security` and `audit` is
  **left alone**: those are other agents' claims to make, the guard reads the union of both records so
  nothing is gated on it, and an under-claim cannot fabricate.

### The G136 boundary, and why this packet is `blocked` rather than `done`

`uservalidation.md` `## Checklist` is `writer: human` in
`.github/bubbles/registry/acceptance-authority.yaml` (`acceptance-checklist` → `writer: human`;
`acceptance-record` → `writer: human`). Its four `[Bug Fix]` items ship unchecked and only the operator
may check them. This phase did not open them, did not check them, and authored no
`## Human Acceptance Record`.

Gate G136 therefore **remains red**, and that is the correct outcome rather than a shortfall. Technical
verification is not human acceptance: everything above establishes that the fix does what it claims, and
none of it establishes that a human looked at the resulting behaviour and accepted it. An agent checking
those boxes would manufacture precisely the signal the gate exists to require.

Status is therefore set to `blocked` on G136 alone, with the operator action named in `blockedReason`.
Sibling `BUG-061-011` was resolved the same way, and for the same reason.

### Not run in this phase

The integration (`T-06`), e2e (`T-08`) and stress lanes were **not** re-executed, and no fresh result is
claimed for them. Their DoD items were validated by assessing the recorded evidence and by verifying at
source that the failure-shaped lines inside them are passing negative fixtures (V-08) — which is the
check that could actually have falsified them. Re-running them would have re-done the test phase's work
rather than validated it, at roughly forty minutes per lane.

No fixture was mutated in-tree to force the hard-failure branch, because AC-5 forbids editing
`internal/`; the equivalent property was established in V-05 by applying the committed matcher to the
committed fixtures instead.

### Uncertainty declaration

The pre-fix probe captures (`9e2e3abf…`, `c049764…`), the T-06 capture (`a3230783…`) and the T-08 capture
(`eb5e8fa3…`) cite sha256 digests over evidence-capture output whose temporary directories no longer
exist, so those specific hashes cannot be re-derived now. This phase's own captures
(`fdc36c2a…`, `9cef6098…`, `655526ae…`) were taken this session, but they cover output containing
timings and package-install lines, so `--verify` would not reproduce them byte-for-byte either. This is a
property of capturing non-deterministic output, **not** a packet defect. Where it mattered, the
underlying property was re-derived by other means rather than trusted: V-03, V-04, V-05 and V-08 depend
on no prior hash at all.

The regex re-derivation in V-03 and V-05 used PCRE `grep -P` against a pattern Go compiles under RE2. The
constructs involved (anchors, negated character classes, non-capturing groups, alternation, `\b`) carry
identical semantics in both engines, and the result agrees with the committed test's live behaviour in
V-02, which is the corroborating control.

### Completion Statement — validate phase

The validate phase executed. Verdict: **technically certified, `blocked` on human acceptance.**

Scope `BUG-061-013-SCOPE-01` is Done and all 14 DoD items are backed by evidence demonstrating what they
assert. The defect was real and is closed: `go-integration.sh` matched **nothing** while reporting green,
the widened matcher now locates it at line 76, and — the part that decides durability — a zero match is
now fatal, so the next syntax shift produces a loud failure rather than another silent pass. Both halves
are guarded by regression tests that were shown to be non-vacuous rather than assumed to be.

Exit 0 was never accepted as proof. The zero-match trap was demonstrated on a control selector before any
focused run was trusted, and every execution claim rests on named `=== RUN` / `--- PASS` lines.

No source file was modified by this phase. `internal/deploy/envsubst_wrapper_contract_test.go` and
`scripts/runtime/` remain byte-identical to HEAD, so **AC-5 is preserved**. The only working-tree changes
are this packet's `report.md` and `state.json`. `uservalidation.md` was not modified, no box was checked,
and no `## Human Acceptance Record` was authored.

`status` and `certification.status` are set to `blocked`, not `done`. G136 human acceptance is the sole
outstanding item, and it belongs to the operator.

<!-- bubbles:certifying-window-begin -->

## Certifying window — 2026-08-27

Everything above this marker was authored and validated in prior specialist rounds, the last of
which closed on 2026-08-20. Everything below it is the fresh evidence of the round that actually
certifies this packet, and is held to the strict evidence standard.

The marker is placed here, and not earlier, because this is the real start of the current window.
The append-only audit rule forbids retroactively rewriting the historical blocks above, and they
are retained unedited rather than trimmed.

### Human acceptance, and what was verified before recording it

G136 was the sole outstanding gate. The operator cleared it on 2026-08-27 with the directive
"human gates approved, check all uservalidations, continue". That decision is recorded in
`uservalidation.md` under `## Human Acceptance Record` with `method: external-record`.

The directive authorises acceptance; it does not establish that the underlying claims are true.
So each of the four `[Bug Fix]` items was re-executed today before its box was checked. Named
`--- PASS` lines were read rather than the lane's exit status, because this packet exists
precisely because a green exit can mean the selector matched nothing:

```text
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.019s
Exit Code: 0
```

Every name the four items depend on appears in that list, so the selector bound what it was
meant to bind. The `--- FAIL` count across the run was `0`.

### Change confinement, re-checked against history

A working-tree diff would be trivially empty now the fix is committed, so confinement was
re-checked over the whole range since the fix instead:

```text
$ git diff 40a9e942^..HEAD -- scripts/runtime/ | wc -l
0
$ git show --name-only --format='' 40a9e942 | grep -v '^specs/'
internal/deploy/envsubst_wrapper_contract_test.go
Exit Code: 0
```

`scripts/runtime/` is byte-identical across every commit since the fix, and the fix commit
touched exactly one file outside this packet's own artifacts: the detector. That is stronger
than the original check, which held only at the moment its diff was taken.




