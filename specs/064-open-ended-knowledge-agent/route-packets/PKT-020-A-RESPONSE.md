# PKT-020-A — RESPONSE (accepted as known drift)

- **Date:** 2026-06-02
- **Authorized by:** workflow owner (rescope decision)
- **Disposition:** Accepted as known drift; ownership transferred to downstream spec 020.

## Decision

The application-layer egress allowlist, `SanitizeSnippet` body, API-key
non-leakage regression guards, and `ContentHash`-over-sanitised
contract have shipped locally for SCOPE-15. The four review questions
raised in PKT-020-A are **accepted as known drift** and transferred to
spec 020 ownership:

1. v1 exact-match allowlist policy review.
2. Wildcard host-pattern follow-up policy.
3. Network-layer / container egress firewall layered defence.
4. SearxNG upstream-engines constraint at the deploy adapter layer.

These items are out-of-scope for spec 064 close-out and will be picked
up in a successor spec dedicated to the security-hardening surface.

## Impact on spec 064

- Spec 064 SCOPE-15 transitions to `Done` with a known-drift
  annotation in its scope status.
- `state.json.transitionRequests[PKT-020-A].status` flips to `closed`.
- `state.json.reworkQueue[PKT-020-A-await].status` flips to `resolved`.

## Receiving-spec action

Spec 020 (or its successor) is the owner of the four policy questions
listed above. The application-layer protections shipped in SCOPE-15
remain authoritative for spec 064 runtime behaviour.
