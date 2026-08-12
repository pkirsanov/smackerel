# Spec — BUG-104-001 expected behaviour

## Problem statement

Integration test packages that write to a shared artifact namespace in the shared test
database must not be able to corrupt each other's data, regardless of the order or
concurrency with which `go test` schedules them.

## Expected behaviour

1. `./smackerel.sh test integration` MUST exit 0 deterministically. A test's result MUST
   NOT depend on whether it ran alone or alongside a sibling package.

2. `tests/integration/selfknowledge` MUST retain the ability to wipe and count the
   `smackerel_self` namespace, because its assertions about the ingestor are inherently
   namespace-wide and the ingestor generates ids the test does not choose.

3. `tests/integration/openknowledge` MUST retain the ability to insert into the literal
   `smackerel_self` namespace, because the self-knowledge tool hardcodes that namespace
   and testing any other namespace would not exercise the shipped behaviour.

4. Requirements 2 and 3 are simultaneously satisfiable ONLY under explicit mutual
   exclusion. That exclusion MUST be enforced mechanically, not by convention or by a
   comment asking future authors to be careful.

5. Removing the mutual exclusion MUST cause a test to fail. A fix for a race that carries
   no guard silently regresses the first time someone simplifies the cleanup.

6. The exclusion MUST NOT serialise unrelated namespaces, and MUST NOT be able to leak a
   held lock if a test panics or is killed — a leaked lock would convert a flaky suite
   into a hung suite, which is worse than the defect being fixed.

## Out of scope

- The production `internal/assistant/selfknowledge` stale-sweep, which legitimately issues
  a namespace-wide delete against the real corpus. That is the ingestor's designed
  behaviour and is unchanged here.
- `tests/integration/knowledge_stats_test.go`'s `TRUNCATE`, which is not currently
  implicated in a failure. Recorded as a latent hazard of the same class, not fixed here.
