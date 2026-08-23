# Specification: BUG-074-002 No-ground E2E must fail, not skip, on a canonical-ack regression

## Scope Of This Specification

This specification governs the **detection behaviour of one live E2E test**, not
the behaviour of the assistant facade. It changes no production contract. Spec
074 SCOPE-074-04B's product rule is unchanged and is restated here only as the
thing the test must be able to detect.

## Restated Product Contract (unchanged, for reference only)

Spec 074 **SCOPE-074-04B** (`specs/074-capture-as-fallback-policy/scopes.md`
line 365): when `open_knowledge` returns `status="refused"` (no-ground) and the
turn is fallback-eligible, the facade invokes the capture-fallback policy with
`cause=open_knowledge_no_ground`, and the user MUST see the canonical
saved-as-idea acknowledgement on the wire:

- `status == saved_as_idea`
- `capture_route == true`
- `confirm_card == nil`
- `disambiguation_prompt == nil`
- body contains the canonical `saved as an idea` acknowledgement

**This packet does not modify, reinterpret, or weaken that rule.**

## Expected Behavior (the actual subject of this bug)

The required regression test for TP-074-14 / SCN-074-A01 MUST report **FAIL**
when the observed envelope violates the SCOPE-074-04B canonical-ack rule,
including — and especially — when `status` is not `saved_as_idea`.

A status other than `saved_as_idea` on a no-ground open-knowledge turn IS the
hunted regression. It MUST NOT be reported as "path not exercised".

## Acceptance Criteria

1. **AC-1 — No outcome-conditional skip.** The test contains no `t.Skip` /
   `t.Skipf` whose condition is an *outcome of the contract under test*. A
   status other than `saved_as_idea` MUST take an assertion path, never a skip
   path.
2. **AC-2 — Canonical-ack assertions are reachable on every non-infrastructure
   run.** The four SCOPE-074-04B assertions (`capture_route`, nil
   `confirm_card`, nil `disambiguation_prompt`, canonical body) execute whenever
   the adapter answered HTTP 200 with a decodable envelope.
3. **AC-3 — Nondeterminism is absorbed legitimately or not at all.** Any
   remaining tolerance for live-model variation is expressed either (a) at the
   input layer, by pinning the branch deterministically, or (b) in the outcome
   space, by asserting an invariant that holds across every legitimate outcome —
   never by bailing out of the assertions.
4. **AC-4 — Infrastructure unavailability remains a legitimate skip.** A skip is
   permitted **only** for the adapter never binding (HTTP 503
   `assistant_http_not_ready`), which is availability of the route, not an
   outcome of the contract. The existing line-71 skip may remain in that role
   and MUST NOT be broadened.
5. **AC-5 — The header comment matches the code.** The `Adversarial coverage:`
   claim (lines 16–20) is either made true by the fix or corrected. No surviving
   comment may promise a failure mode the code cannot produce.
6. **AC-6 — The change is proven adversarially.** A pre-fix demonstration shows
   the current test SKIPPING (green run, nothing reported) on an
   off-contract status, and the post-fix demonstration shows the same condition
   FAILING. A regression that cannot be shown to flip the outcome does not
   satisfy this criterion.
7. **AC-7 — Policy compliance.** The resulting test satisfies
   `.github/copilot-instructions.md` line 331 (no failure-condition early
   exits).

### Single-Capability Justification

Gate G094 asks whether this work introduces a reusable foundation or a single
concrete capability. It is a **single capability**, and the proportionality
argument is the packet's own change boundary: exactly one test function in
exactly one file, `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go`,
with `internal/` explicitly forbidden.

The gate's trigger words fire here because the shipped fix is a five-branch
switch, and a branch set can look like a provider or strategy family. It is not
one. The branches are a **total partition of one response envelope's outcome
space**, not interchangeable implementations of a shared contract: they are
selected by reading `error_cause` and `sources` on a single `TurnResponse`, they
share no interface, nothing dispatches between them at runtime, and no second
caller can select one. Extracting a "branch strategy" abstraction over them
would add indirection with exactly one consumer.

There is a real reuse question here, and it is honestly out of scope rather than
absent: `tests/e2e/assistant/http_capture_test.go:71` carries the same
branch-on-the-thing-you-assert defect. That is recorded as DI-6 in `report.md`
and routed to its own packet. If and when a second instance is fixed, the shared
**shape** — classify on fields disjoint from the asserted fields, close the
outcome space totally, keep skips keyed only on infrastructure availability — is
what would become a documented pattern. Two instances is the point at which that
becomes evidence rather than speculation; one is not.

## Non-Goals

- Changing `internal/assistant/` production code. This packet asserts **no**
  production defect.
- Re-litigating whether SCOPE-074-04B's rule is correct.
- Resolving which branch the live stack takes for the fabricated-city prompt.
  That is an open unknown recorded in `bug.md`; the fix must be correct
  regardless of the answer, and pinning the branch (option b) is one admissible
  way to make the answer irrelevant.
- Fixing the analogous pattern anywhere else in the repository. If the fix
  surfaces siblings, they are routed, not silently swept in.

## Release Train

Target: `mvp`. No feature flag introduced. Test-only change boundary.

## Test Isolation

Live E2E runs against the disposable test stack with unique transport message
IDs, per the existing `tests/e2e/assistant/` conventions. No persistent state is
introduced.

## Deployment Boundary

None. No deployment, release-train, host, secret, image, or config-bundle change
is implied by this packet.
