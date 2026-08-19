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

## Principle this restores

The repository already applies this rule to its guards and states it explicitly. Verified in-repo at
HEAD:

- `.github/bubbles/scripts/release-delivery-reconciliation-guard.sh:32` —
  *"a missing field must never make the gate a silent no-op."*
- `.github/bubbles/scripts/surface-reachability-guard.sh:170–172` — a declared class with no derive
  command is reported as an integrity failure, because *"dropping it here would turn a
  misconfiguration into a silent no-op — the failure mode this whole contract exists to remove."*

The envsubst wrapper contract is the outlier. This bug brings it into line.
