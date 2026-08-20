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



