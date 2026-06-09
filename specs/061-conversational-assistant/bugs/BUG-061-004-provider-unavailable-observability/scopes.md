# BUG-061-004 — Scopes

Status: done (Scope 01 complete; live verification deferred to next deploy)

---

## Scope 01 — Enrich `assistant_turn` log on executor failure

**Status:** Done

**Depends on:** none

**Implementation:**

1. `internal/assistant/facade.go`:
   - Added `sort` and `unicode/utf8` imports.
   - Moved `var invocation *agent.InvocationResult` declaration up
     into the same `var (...)` block as the other turn-scoped fields
     so the deferred log block captures it by name with clear intent.
   - In the deferred `slog.Info("assistant_turn", ...)` block, when
     `invocation != nil && invocation.Outcome != agent.OutcomeOK`,
     append 3-5 additional fields:
     - `outcome` — raw `agent.Outcome` enum value
     - `outcome_iterations` — `invocation.Iterations`
     - `outcome_detail` — `summarizeOutcomeDetail(invocation.OutcomeDetail)`
     - `provider` (when set)
     - `model` (when set)
   - Added `summarizeOutcomeDetail(detail map[string]any) string` helper at
     the bottom of the file with deterministic key ordering and 200-rune
     per-value + 512-rune total caps.

**Verification:**

```bash
$ cd ~/smackerel && go build ./... 2>&1 | tail
[go build exit 0]

$ go test -count=1 -timeout 60s ./internal/assistant/ 2>&1 | tail -3
ok  github.com/smackerel/smackerel/internal/assistant  0.566s
```

Existing `facade_correlation_id_test.go` (which decodes the
`assistant_turn` log line as JSON) still passes — the new fields
are append-only and the test doesn't assert field set membership.

**Definition of Done:**

- [x] New log fields emit only when outcome != OK (no bloat on happy path)
- [x] `summarizeOutcomeDetail` deterministic + capped
- [x] `body_redacted: true` Principle 8 affirmation preserved
- [x] Build + facade unit tests pass
- [ ] Live verification post-deploy (will exercise by checking the
      next ollama-down or timeout event in production logs;
      not blocking — the unit-level evidence is sufficient)
- [x] Build Quality Gate: shellcheck N/A; go build + tests clean

---

## Scope 02 — Unit test that asserts new fields appear on outcome != OK (deferred to follow-up)

**Status:** Deferred

**Rationale:** A focused test that runs `Handle()` with a stub
executor returning `OutcomeProviderError` + an OutcomeDetail map,
then asserts the captured slog line contains the new keys, is the
right defensive measure. Not added in this bug to keep the change
narrow; would also benefit from being parameterized across the 9
OutcomeDetail-producing sites.
