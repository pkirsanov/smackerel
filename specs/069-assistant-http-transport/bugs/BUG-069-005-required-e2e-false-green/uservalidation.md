# User Validation: BUG-069-005 Required assistant E2E false-green

The packet is `in_progress`; these checked baseline expectations become the
post-fix user/operator verification contract. A user may uncheck an item to
report a regression after delivery.

## Checklist

- [x] Natural-language assistant turns use the configured compiler before any user-facing route decision.
- [x] Ambiguous Springfield weather requests present persistent choices and do not call weather before selection.
- [x] List and annotation writes present a confirmation control and do not mutate state before acceptance.
- [x] Accepting one issued confirm reference executes the proposed action once; replay does not execute it again.
- [x] The five manifest-required E2E tests fail when required behavior is absent and report zero skips when healthy.
- [x] Test determinism uses the real authenticated HTTP/core/ML/persistence path with only the external LLM provider substituted in the disposable test stack.

## Verification Steps

1. Start the disposable test stack through the Smackerel repo CLI.
2. Run the exact five-test selector recorded by the implementation/test owner.
3. Confirm five tests pass and zero tests skip.
4. Inspect the exact pending conversation row during disambiguation and confirm
   flows, then verify it clears after resolution.
5. Verify no list/annotation state changes before acceptance and exactly one
   change occurs after acceptance.
6. Run the propagated regression-quality guard against the five files and
   confirm Go skip-family calls are rejected.
