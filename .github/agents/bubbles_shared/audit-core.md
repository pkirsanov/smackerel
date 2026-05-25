# Audit Core

Purpose: mandatory audit/validation rules for `bubbles.audit` and `bubbles.validate`.

## Load By Default
- `critical-requirements.md`
- `audit-core.md`
- `evidence-rules.md`
- `state-gates.md`
- Scope entrypoint, `report.md`, `state.json`, and `uservalidation.md`

## Audit Responsibilities
- Verify evidence is real and phase claims match artifact reality.
- Fail completion when planned-behavior fidelity, regression permanence, or consumer-trace coverage is missing.
- Treat state-transition and reality scans as mechanical blockers, not advisory checks.
- **Generate Spot-Check Recommendations** to mitigate automation bias (see below).

## Required Audit Checks
- State transition guard passes.
- DoD evidence is inline and legitimate.
- Required specialist phases actually executed.
- Rename/removal work has Consumer Impact Sweep coverage and zero stale first-party references.
- Scenario-specific E2E regression coverage exists for changed behavior.
- **Evidence provenance review** — all `interpreted` claim-source blocks must be verified (see evidence-rules.md).
- **Uncertainty Declaration review** — all unchecked DoD items with Uncertainty Declarations must be assessed for resolvability.

## Spot-Check Recommendations (Automation Bias Mitigation)

**Purpose:** When all gates pass and all DoD items are `[x]`, the user is presented with a "green wall" of checkmarks. Without explicit guidance on what to verify, automation bias causes humans to trust more and check less as AI confidence increases. Spot-Check Recommendations break this loop.

**`bubbles.audit` MUST include a `## Spot-Check Recommendations` section in every audit verdict**, even when the verdict is `🚀 SHIP_IT`. This section highlights items that passed all gates but have characteristics that warrant human review.

### Trigger Conditions (Include item if ANY condition is true)

| Condition | Why It Warrants Review |
|-----------|----------------------|
| Evidence block has `**Claim Source:** interpreted` | Agent had to reason about output — interpretation may be wrong |
| Evidence block is exactly 10 lines (minimum threshold) | Minimum-viable evidence is more likely to omit signals |
| First time this test type was used in this spec | Novel test patterns have higher error rates |
| DoD item was unchecked `[ ]` and then checked `[x]` in a later edit (re-check) | Late completion may indicate initial difficulty worth verifying |
| Evidence shows warnings in output (even if exit code was 0) | Warnings may indicate non-critical but real issues |
| Test passed on first attempt for a non-trivial change | "Too clean" execution pattern — legitimate but worth a glance |
| Scope has `Done with Concerns` status | Concerns exist that the user should be aware of |
| Any Uncertainty Declaration was resolved by this audit pass | Resolution of uncertainty is inherently higher-risk |

### Output Format

```markdown
## Spot-Check Recommendations

These items passed all gates but have characteristics that warrant human review:

1. **Scope 03, DoD item 4** — Evidence was `interpreted` (not directly `executed`).
   Interpretation: "Test passed but assertion only checks HTTP status, not retry count."
   Recommendation: Verify that retry count is actually correct.

2. **Scope 05, DoD item 2** — Evidence block is exactly 10 lines (minimum threshold).
   Recommendation: Verify the output captures the critical verification signal.

3. **Scope 01, DoD item 7** — First time `stress` test type was used in this spec.
   Recommendation: Verify stress test methodology is appropriate.

If no items trigger spot-check conditions: "No spot-check items identified. All evidence is `executed` with comfortable margins."
```

### Rules
- Spot-Check Recommendations are INFORMATIONAL — they do not block completion.
- The section MUST appear in every audit verdict, even if empty ("No spot-check items identified").
- Items are ordered by review priority: `interpreted` evidence first, then minimum-threshold evidence, then other triggers.
- Each recommendation includes a one-sentence explanation of WHY it warrants review and WHAT to verify.

## References
- `test-fidelity.md`
- `consumer-trace.md`
- `e2e-regression.md`
- `evidence-rules.md` — Evidence Provenance Taxonomy, Uncertainty Declaration Protocol
