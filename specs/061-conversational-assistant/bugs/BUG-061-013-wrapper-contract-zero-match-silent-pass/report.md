# Report: BUG-061-013 — Filing and root-cause evidence

**Packet status:** `in_progress` — filed, root-caused, **no fix implemented**.
**Phase:** discovery / documentation / analysis (`bubbles.bug`).
**Repository binding:** `PREFLIGHT_COMMITTED` decision
`rb:vscode-6af2178e10192363b0e52a46fb5e0950:40`, repository `smackerel`.

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

