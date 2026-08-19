# BUG-061-013 — The envsubst wrapper contract passes when it cannot find what it is contracted to check

**Status:** Reported (root cause confirmed; no fix implemented)
**Severity:** S2 — guard integrity / test-effectiveness. **Not user-facing.**
**Discovered:** 2026-08-19, during the regression phase of BUG-061-011
**Discovered at:** HEAD `4895d446`
**Affected surface:** `internal/deploy/envsubst_wrapper_contract_test.go` (the detector).
`scripts/runtime/go-integration.sh` is the wrapper the detector has gone blind to, but it is
**not defective** — see "What is NOT broken".

---

## Summary

`TestEnvsubstWrapperContract_LiveWrappers` asserts, for each of four tracked test wrappers, that
`ensure_envsubst` is called *before* `go test` runs. It locates the `go test` invocation with an
anchored lexical regex:

```go
// internal/deploy/envsubst_wrapper_contract_test.go:82
var envsubstGoTestRE = regexp.MustCompile(`(?m)^\s*go\s+test\b`)
```

Commit `c7667d99` rewrote that invocation in `scripts/runtime/go-integration.sh` from a bare
`go test …` to a conditional-and-piped form beginning `if ! `. The anchored regex no longer
matches. The subtest for that wrapper **still reports PASS** — because the ordering comparison is
guarded by `goTestIdx != nil`, so a zero match falls through to `return nil`.

The assertion now has nothing to assert against, and reports success. It is a check that cannot
fail.

## What is NOT broken

State this plainly, because the severity depends on it:

- **No production behaviour is asserted broken.** `ensure_envsubst "go-integration"` is called at
  `scripts/runtime/go-integration.sh:14`; `go test` runs at line 76. The ordering the contract cares
  about **genuinely holds today**.
- **No user is affected.** This is a test-infrastructure defect. Nothing in the assistant, the
  corpus, the connectors, or any delivered surface changes behaviour.
- **`go-integration.sh` is not the defect.** Its rewrite is legitimate — it needed to capture output
  for the BUG-061-011 eval gate. The detector's inability to read the new form is the detector's
  problem.

What IS broken is the **detector**. If a future change moved `ensure_envsubst` after `go test` in
`go-integration.sh`, this test would not notice, and the integration lane would fail at runtime with
the exact `exit 127 / envsubst: command not found` that this contract was written to prevent
(see the spec-052 chaos-finding rationale in the test file's own header).

## Reproduction

At HEAD `4895d446`, in the repository root:

**Step 1 — the regex the contract uses finds nothing:**

```
$ grep -nE '^\s*go\s+test\b' scripts/runtime/go-integration.sh
GREP_A_EXIT=1
```

Exit 1, no output — zero matches.

**Step 2 — but `go test` is plainly there:**

```
$ grep -n 'go test' scripts/runtime/go-integration.sh
76:if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
GREP_B_EXIT=0
```

The line begins `if ! `, so the `^\s*` anchor cannot reach the `go test` token.

**Step 3 — and the subtest passes anyway:**

```
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.115s
TARGETED_UNIT_EXIT=0
```

Full transcript and the other three subtests are in [report.md](./report.md).

## Blast radius

`go-integration.sh` is the **only** one of the four tracked wrappers currently affected. The other
three still match the anchored regex, so their ordering assertion is still live:

| Wrapper | `^\s*go\s+test\b` matches? | Ordering actually checked? |
|---|---|---|
| `go-unit.sh` | yes (line 67) | yes |
| `go-integration.sh` | **NO** | **NO — silently passes** |
| `go-e2e.sh` | yes (line 89) | yes |
| `go-stress.sh` | yes (line 50) | yes |

The *latent* radius is all four: any of the remaining three could acquire a prefix tomorrow and
degrade the same way, silently, because the zero-match branch is shared.

## Why the existing adversarial tests did not catch it

The test file carries three adversarial sub-tests, and they are genuinely good — they prove the
source-line locator and the call locator both have teeth. But all three fixtures write the
invocation as a bare `go test ./...` at line start (lines 184, 204, 224). The regex therefore
matches in **every** fixture the suite owns. The zero-match branch is never exercised by any test,
adversarial or otherwise. The suite is complete with respect to two of its three locators and blind
with respect to the third.

## Origin

`c7667d99` — *"feat(stage-1): eval-gate lane wiring, router warm-up contract, corpus grant scopes
01-04"* — the BUG-061-011 fix. It changed `scripts/runtime/go-integration.sh` by 74 insertions and
2 deletions. Before it, line 55 read `go test "${go_test_args[@]}"` and matched the anchored regex.

This is worth recording as motivation rather than as blame: **BUG-061-011 existed precisely because
an evaluation gate ran in no automated lane — a gate that could not fail.** Its fix reintroduced the
same defect class in a neighbouring guard. The class is not "someone was careless"; it is that a
guard's own blindness is invisible by construction unless the guard is built to report it.

## Related

- `BUG-061-011` — the packet whose fix introduced this. **Do not modify it**; its regression phase is
  in flight.
- Precedent for the principle this violates, in this repo's own guard corpus:
  `.github/bubbles/scripts/release-delivery-reconciliation-guard.sh:32` — *"a missing field must
  never make the gate a silent no-op."*
