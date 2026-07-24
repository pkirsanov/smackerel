# Bug: [BUG-080-001] Graph API Fails Soft Into Runtime Disablement

## Summary

`KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV` points to a secret whose value is empty; core logs a warning and leaves Graph API handlers nil, so topics, people, places, time, and edges return 404 while static Wiki pages ship and strict deployment verification still passes.

## Severity

Critical (S1): a required deployed capability is silently absent while release acceptance reports success.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No config generation, startup, route probe, secret read, host mutation, or deployment was executed here.

## Reproduction Steps

1. Configure `KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV` to name the required secret while leaving that secret value empty.
2. Start core in the reported production configuration.
3. Observe a warning and nil Graph API handlers rather than startup failure.
4. Request authenticated topics, people, places, time, and edges endpoints; observe 404 responses.
5. Observe that static Wiki pages are still present and strict deployment verification passes.

## Expected Behavior

When Graph API is required for production, missing or empty required secret material fails configuration/startup/deployment before serving. The product exposes no secret value. The operator-owned adapter injects the value. Strict acceptance includes route existence and an authenticated read-only synthetic over topics, people, places, time, and edges.

## Actual Behavior

The runtime starts without handlers, mounted static surfaces imply availability, all Graph API reads 404, and deployment acceptance misses the product failure.

## Outcome Contract

**Intent:** Make Graph API activation fail loud and make deployment readiness prove an authenticated graph read rather than static-route presence.

**Success Signal:** Empty/missing required cursor-secret configuration refuses startup/deploy with a typed non-secret code; valid injected configuration mounts every route and a read-only synthetic confirms authenticated graph reads.

**Hard Constraints:** Secret values never appear in output; Smackerel remains target-agnostic; operator deploy-adapter injection is devops-owned and not edited by this packet; synthetics perform no production writes; explicit disabled mode cannot be advertised as ready.

**Failure Condition:** Core can serve with required handlers nil, any graph endpoint 404s under a ready claim, strict acceptance passes without authenticated reads, or diagnostics expose the secret.

## Impact And Dependencies

- Blocks `specs/105-connected-knowledge-graph-explorer`.
- Blocks Wiki/Graph journeys in `specs/106-coherent-product-experience`.
- Blocks `BUG-102-001-product-journey-acceptance-gap`.
- Product-side contract is independent; full browser proof also depends on `BUG-070-001`.

## Ownership Boundary

Smackerel owns fail-loud config, route registration, readiness contract, and product synthetic. `bubbles.devops` owns the separate operator deploy-adapter secret-injection and strict-acceptance consumption change. This invocation does not edit the operator deploy repository.
