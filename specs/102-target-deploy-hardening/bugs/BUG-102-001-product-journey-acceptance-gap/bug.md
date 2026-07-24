# Bug: [BUG-102-001] Product Journey Acceptance Gap

## Summary

Strict deploy-adapter acceptance can pass image/config/Caddy/health checks while primary product journeys remain broken: Wiki APIs return 404, modern auth fails, Search enhancement is blocked, Digest renders false empty, and Assistant leaves a blank response.

## Severity

Critical (S1): deployment reports success without proving that the shipped product is usable.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No deployment, host, browser, HTTP, production, or adapter mutation occurred in this planning-only invocation.

## Reproduction Steps

1. Run the reported strict acceptance against the production deployment target.
2. Observe acceptance passing image identity, configuration, reverse proxy, and health checks.
3. Use the deployed product as an authenticated user.
4. Observe Wiki/Graph API 404s, modern auth rejection, blocked Search submission, Digest false empty, and a blank Assistant failure response.

## Expected Behavior

The Smackerel repository owns a deterministic authenticated read-only product-journey synthetic contract. The operator-owned deploy adapter consumes that contract during strict acceptance. Required journey failures produce closed failure codes and reject acceptance. The synthetic performs no production data writes, does not expose credentials, and includes real-browser Playwright plus deployment-target validation.

## Actual Behavior

Infrastructure and health signals pass while end-user behavior remains unverified and materially broken.

## Outcome Contract

**Intent:** Make deployment acceptance prove that the current release can complete its required authenticated read-only product journeys.

**Success Signal:** One versioned product-owned command/contract validates login/session, Search, Digest, Assistant, Wiki/Graph, Recommendations, Cards, and required status/read surfaces; adapter acceptance consumes its machine-readable result and rejects any required failure.

**Hard Constraints:** No production writes; no test user/data creation on production; no secret output; no internal request interception; optional capabilities are evaluated against explicit policy; product logic stays in Smackerel and adapter logic stays in the operator-owned deploy repository.

**Failure Condition:** Acceptance can pass on health/routes alone, ignores a required journey, accepts 404/blank/false-empty/auth split, mutates production data, or emits ambiguous/non-actionable failures.

## Dependency Contract

This packet depends on the product behaviors owned by:

- `BUG-070-001` production browser session.
- `BUG-002-006` Search submission.
- `BUG-002-007` Digest typed truthful read.
- `BUG-073-006` Assistant terminal errors.
- `BUG-080-001` Graph API activation/read synthetic.
- `BUG-004-004` synthesis persistence/health truth.
- `BUG-039-005` provider-backed recommendations readiness.
- `BUG-083-002` Card Rewards parity.
- `specs/104-universal-ask-self-knowledge` Scope 8.
- Specs 105 and 106 for final Graph/coherent-product journeys after their owner phases.

## Ownership Boundary

Smackerel owns journey definitions, expected states, Playwright/API probes, failure codes, and machine-readable result schema. `bubbles.devops` owns deploy-adapter invocation/consumption in the separate operator repository. This packet changes neither that repository nor production.
