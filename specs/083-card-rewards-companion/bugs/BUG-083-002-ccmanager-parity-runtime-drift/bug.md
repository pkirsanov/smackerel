# Bug: [BUG-083-002] CCManager Parity Runtime Drift

## Summary

Smackerel's Card Rewards capability has strong PostgreSQL, provenance, lifecycle, and UI foundations, but it does not yet demonstrate parity-or-better with the independently inspected CCManager workflows across all 16 required product areas.

## Severity

High (S2): Card Rewards is a major product surface whose completeness and coherence are prerequisites for spec 106.

## Status And Provenance

Reported from the spec 105/106 analyst finding ledger and operator request. **Claim Source:** interpreted historical input plus read-only repository inspection in this invocation. No CCManager or Smackerel runtime, test, mutation, commit, push, or deployment was executed.

## Read-Only Baseline Sources

- CCManager: `README.md`, `docs/ARCHITECTURE.md`, `web/app.py`, `web/templates/`, `data/pending-selections.json`, `.github/workflows/`.
- Smackerel: specs 083 and 092, `internal/cardrewards/`, `internal/web/cardrewards*.go`, and `web/pwa/tests/cardrewards*.spec.ts`.
- Analyst dependency framing: read-only untracked `specs/105-connected-knowledge-graph-explorer/spec.md` and `specs/106-coherent-product-experience/spec.md`; neither file was edited.

## Reproduction / Gap Observation

1. Compare the 16 areas in [spec.md](spec.md) against CCManager's implemented routes/data workflows and Smackerel's current Card Rewards routes/contracts.
2. Observe that Smackerel already exceeds CCManager in several foundations but has unproven or missing parity in multi-category offer representation, complete selection/bonus lifecycle, editable historical recommendations, versioned import/export, pending-selection workflow, and coherent product-level UX/error behavior.
3. Observe that spec 106 treats Card Rewards as a required primary journey, so partial parity or a visually separate sub-application blocks a coherent product claim.

## Expected Behavior

Smackerel delivers parity-or-better across all 16 measurable areas without losing its existing advantages: PostgreSQL integrity, source attribution/provenance, confidence and multi-source reconciliation, lifecycle/manual verification, fail-loud SST, auditable scheduling, strict CSP/PASETO security, and real-stack accessible UI coverage.

## Actual Behavior

The current implementation has strong isolated capabilities, but the complete 16-area parity contract has not been planned, executed, or verified end to end.

## Outcome Contract

**Intent:** Absorb every useful CCManager card-management workflow into Smackerel as a coherent, secure, source-qualified Card Rewards experience while preserving or improving Smackerel's stronger architecture.

**Success Signal:** An authenticated user completes wallet, offers, selections, bonuses, calendar, historical optimization, source operations, audit, config, import/export, pending actions, report, schedule/manual, UX/error, responsive/a11y/theme, and security journeys with round-trip proof and parity ledger evidence.

**Hard Constraints:** No regression of PostgreSQL/SST/provenance/lifecycle/security/accessibility advantages; no JSON files as runtime source of truth; no fixture provider/data in production; no financial advice/execution; no secret/card-sensitive leakage; no internal mocks in live tests.

**Failure Condition:** Any of the 16 areas remains below the measured baseline, any workflow is route-only or non-persistent, any Smackerel advantage is removed, or parity is claimed without adversarial real-stack evidence.

## Impact And Dependencies

- Major dependency of `specs/106-coherent-product-experience` Card surface.
- Authenticated full-product Playwright depends on `BUG-070-001`.
- Product acceptance depends on this packet through `BUG-102-001`.
- Scope decomposition must preserve independent delivery and avoid editing active spec 104.

## Root Cause Ownership

This is capability/runtime drift, not one certified code defect. `bubbles.design` must reconcile domain/API/UI/data boundaries; `bubbles.plan` must split the 16 areas into bounded dependent delivery scopes before implementation.
