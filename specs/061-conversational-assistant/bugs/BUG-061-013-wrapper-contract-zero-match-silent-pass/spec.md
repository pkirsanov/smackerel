# Spec: BUG-061-013 — A wrapper-ordering contract must fail when it cannot locate the invocation it checks

## Problem statement

`assertEnvsubstWrapperContract` in `internal/deploy/envsubst_wrapper_contract_test.go` locates three
things in each tracked wrapper: the `source` line for `_ensure_envsubst.sh`, the `ensure_envsubst`
call, and the `go test` invocation. Two of those three locators treat *absence* as a violation. The
third treats absence as compliance.

That asymmetry is the defect. It is visible in the function itself:

| Locator | Absence is treated as | Line |
|---|---|---|
| `envsubstSourceLineRE` | **error** — "missing source line" | 92–96 |
| `envsubstCallRE` | **error** — "never calls `ensure_envsubst`" | 98–101 |
| `envsubstGoTestRE` | **pass** — falls through to `return nil` | 107–111 |

## Expected behaviour

**A wrapper-ordering contract MUST FAIL when it cannot locate the invocation it is contracted to
check. A zero match MUST be an error, not a silent pass.**

Precisely:

- **EB-1 — Zero match is a hard failure.** For any wrapper in `envsubstTrackedWrappers`, if the
  `go test` locator returns no match, `assertEnvsubstWrapperContract` MUST return a non-nil error.
  The wrapper is on that list *because it runs `go test`*; a wrapper on the list with no locatable
  invocation is by construction a **locator failure**, never a compliant wrapper.

- **EB-2 — The error must name the real problem.** The message MUST say that the invocation could
  not be located and that the matcher needs widening — not that the wrapper is malformed. A future
  reader must not spend time auditing a correct wrapper.

- **EB-3 — The matcher must recognise the forms actually in use.** At minimum it must locate a
  `go test` invocation that is prefixed by a conditional (`if`, `if !`), a list operator (`&&`,
  `||`), or is part of a pipeline. `scripts/runtime/go-integration.sh:76` is the live example:

  ```
  if ! go test "${go_test_args[@]}" 2>&1 | tee "$gate_output_file"; then
  ```

- **EB-4 — The ordering guarantee is unchanged.** `ensure_envsubst` must still be proven to precede
  the invocation. This bug narrows *when the check is allowed to be silent*; it does not relax what
  the check asserts.

- **EB-5 — The blindness must itself be tested.** A fixture whose invocation the matcher cannot
  locate MUST make the test go RED. Without this, EB-1 could regress exactly as EB-4's ordering
  check did — invisibly.

## Acceptance criteria

| # | Criterion | How it is proven |
|---|---|---|
| AC-1 | A tracked wrapper with no locatable `go test` produces an error | Adversarial fixture, RED before fix |
| AC-2 | `go-integration.sh` at HEAD is located by the matcher and its ordering is genuinely asserted | The live subtest fails if `ensure_envsubst` is moved after line 76 |
| AC-3 | All four tracked wrappers still pass on their true ordering | Live subtest green with no weakened assertion |
| AC-4 | The error text distinguishes "matcher could not see it" from "wrapper is wrong" | Assertion on the error substring |
| AC-5 | No production/runtime file is modified | `git diff` over `scripts/`, `cmd/`, `internal/` excluding the one test file |

## Non-goals

- Rewriting `scripts/runtime/go-integration.sh` back to a bare `go test`. The conditional-and-piped
  form is required by the BUG-061-011 eval gate and is correct. **Changing the subject to satisfy
  the detector is the wrong direction** — it would make the wrapper serve the test rather than the
  test serve the wrapper, and it would leave the zero-match hole open for the next wrapper.
- Any change to `_ensure_envsubst.sh`, the four wrappers, or the envsubst install path.
- Broadening the *scope* of what the contract asserts (e.g. adding new invariants).

### Single-Capability Justification

Gate G094's proportionality trigger fires on this packet, so this section records why one capability
is the correct shape and why a domain-capability model would be fiction here.

**What actually triggered the gate.** Both hits are in `scopes.md`, and neither is a design
statement. Both sit inside a single block of captured verbatim terminal output, sealed by an
`evidence-capture.sh` sha256:

| `scopes.md` line | Literal text | What it actually is |
|---|---|---|
| 291 | `#0 building with "default" instance using docker driver` | a Docker buildx status line |
| 302 | `Container smackerel-test-intent-compiler-provider-1  Removed` | a Compose container name |

The gate matched `driver` and `provider` as substrings of machine output. This file and `design.md`
contain zero trigger words. That evidence is deliberately left byte-identical: captured output
records what a command printed, and editing it to clear a trigger would corrupt the record to buy a
green gate — the same inversion the first non-goal above rejects.

**The single capability.** *Locate the `go test` invocation inside a tracked wrapper, and assert that
`ensure_envsubst` precedes it.* EB-1 through EB-5 are five requirements on that one capability: what
a zero match means (EB-1), what the error must say (EB-2), which invocation forms must be locatable
(EB-3), that the ordering assertion is unchanged (EB-4), and that the blindness is itself tested
(EB-5). Five requirements on one capability, not five capabilities.

**Why no variation axis exists.** An axis needs two or more members a caller can choose between.
There is one contract, one locator, and one ordering rule, applied uniformly to one list of wrappers.
Every member of `envsubstTrackedWrappers` presents an invocation that the single locator reaches —
three bare at line start (`go-unit.sh:67`, `go-e2e.sh:89`, `go-stress.sh:50`) and one behind shell
syntax (`go-integration.sh:76`, `if ! go test … | tee …`). That last one is the nearest candidate for
an axis and is not one: it is a different *spelling* of the same command, which is why EB-3 widens
the one pattern instead of adding a second locator.

**What would make a second implementation correct.** A tracked wrapper that does not share the
`go test` grammar: one that runs its suite through a different runner, or that builds the invocation
where no lexical pattern can reach it (`env VAR=x go test`, `timeout N go test`, a `$( … )` capture).
*How the invocation is located* would then have two real members, and a locator abstraction would be
warranted. No such wrapper exists today, and EB-3's bounded enumeration would have to be reopened
first. Modelling that family now would be abstraction ahead of its second member — and EB-1 is what
will announce the day that member arrives, because it converts an unlocatable wrapper from a silent
pass into a named failure.

## Principle this restores

The repository already applies this rule to its guards and states it explicitly. Verified in-repo at
HEAD:

- `.github/bubbles/scripts/release-delivery-reconciliation-guard.sh:32` —
  *"a missing field must never make the gate a silent no-op."*
- `.github/bubbles/scripts/surface-reachability-guard.sh:170–172` — a declared class with no derive
  command is reported as an integrity failure, because *"dropping it here would turn a
  misconfiguration into a silent no-op — the failure mode this whole contract exists to remove."*

The envsubst wrapper contract is the outlier. This bug brings it into line.
