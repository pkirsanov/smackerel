# Bug: [BUG-039-005] Enabled With Zero Providers Reports False Ready

## Summary

`RECOMMENDATIONS_ENABLED=true` while the production provider registry is empty and Google/Yelp providers are false; UI/routes still mount but produce no providers, results, or watches.

## Severity

High (S2): a capability is advertised as enabled/ready although it cannot perform its core operation.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No config, provider registry, UI, or route execution occurred in this planning-only invocation.

## Reproduction Steps

1. Set `RECOMMENDATIONS_ENABLED=true` in the described production configuration.
2. Keep the production provider registry empty with Google and Yelp disabled.
3. Open mounted recommendation UI/routes.
4. Observe no usable providers, recommendation results, or watches while the capability appears enabled.

## Expected Behavior

An enabled recommendation capability has at least one configured healthy production provider. Otherwise startup fails when the capability is required, or capability readiness is explicitly unavailable and user actions are hidden/refused with setup guidance. Fixture providers never satisfy production readiness.

## Actual Behavior

The feature switch mounts user surfaces without an executable provider path and presents false readiness.

## Outcome Contract

**Intent:** Derive recommendation availability from healthy production providers rather than route/flag presence.

**Success Signal:** Readiness tests prove one healthy configured provider before enabling actions; zero/unhealthy providers produce unavailable/degraded states and no fabricated result/watch.

**Hard Constraints:** No fixture provider in production, no hidden fallback, no fake results, explicit provider provenance, no secret output, and no whole-product outage for intentionally optional capability.

**Failure Condition:** Enabled zero-provider state appears ready, actions silently do nothing, a fixture satisfies production readiness, or provider failure becomes no results without explanation.

## Impact And Dependencies

- Blocks recommendation/watches readiness in `specs/106-coherent-product-experience`.
- Blocks product journey acceptance in `BUG-102-001-product-journey-acceptance-gap`.
- Independent of Card Rewards recommendations in `BUG-083-002`.

## Root Cause Ownership

`bubbles.design` must confirm provider registry construction, capability-requiredness policy, readiness aggregation, route/UI mounting, watch lifecycle, and provider health before implementation.
