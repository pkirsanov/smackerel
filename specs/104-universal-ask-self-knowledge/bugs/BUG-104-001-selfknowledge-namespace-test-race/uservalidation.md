# User Validation — BUG-104-001

Items are CHECKED `[x]` by default because each was verified by execution in the session
that fixed the defect. Uncheck an item to report that the behaviour is broken; an unchecked
item is a blocking regression.

- [x] `./smackerel.sh test integration` completes without the three `openknowledge`
      failures (`TestSelfKnowledge_TrustPerimeter`, `TestSelfKnowledgeTool_CitesOnlySmackerelSelf`,
      `TestPgxSemanticSearcher_NamespaceScopedCosine`)
- [x] A test's result no longer depends on whether it ran alone or in a full parallel run
- [x] The integration suite is stable across repeated runs, not green only once
- [x] `tests/integration/selfknowledge` still verifies the ingestor with namespace-wide
      counts — the fix did not weaken what that test asserts
- [x] `tests/integration/openknowledge` still exercises the literal `smackerel_self`
      namespace the shipped tool uses — the fix did not relocate the tests to a
      synthetic namespace that would stop testing real behaviour
- [x] The suite does not hang: a failing or panicking test releases the namespace lock

## Known limitations (not defects in this fix)

- `tests/integration/knowledge_stats_test.go` issues a `TRUNCATE`. It is a latent hazard of
  the same class but is not currently implicated in any failure, and is deliberately not
  addressed here.
- The production `internal/assistant/selfknowledge` stale sweep still issues a
  namespace-wide delete against the real corpus. That is its designed behaviour and is
  unchanged.
