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



