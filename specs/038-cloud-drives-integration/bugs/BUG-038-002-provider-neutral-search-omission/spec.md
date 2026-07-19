# Specification: BUG-038-002 Provider-neutral Drive search omission

## Release Train

Target train: `mvp`. This bug introduces no feature flag and does not modify release-train bundles.

## Expected Behavior

Once a Drive scan and extraction transaction completes, the canonical authenticated search surface must make the resulting artifacts discoverable by their content, title, folder context, and provider metadata. Provider-neutral consumers must receive Google and memdrive rows through the same response contract.

## Requirements

### BR-038-002-001 Synchronous visibility

After `InitialScan` and `ProcessPending` return success, an authenticated `/api/search` request with matching lexical terms must be able to return the persisted artifacts without arbitrary delay.

### BR-038-002-002 Collision-resistant regression query

The regression must use a collision-resistant per-run query term shared by both provider fixtures. Earlier-package generic matches may fill the bounded result window without invalidating the provider-neutral assertion.

### BR-038-002-003 Provider and audience fidelity

Search must preserve exact `artifact_id`, `drive.provider_id`, folder, sharing, sensitivity, and audience behavior. The fix must not weaken provider or audience assertions and must not expose an artifact outside its existing policy contract.

### BR-038-002-004 Isolation and cleanup

Regression tests must use uniquely identified Google and memdrive connections on the disposable test database and remove only their own rows. They must not depend on data from another package or test invocation.

### BR-038-002-005 Deterministic verification

The regression must prove database visibility and live API visibility through deterministic state checks or bounded polling tied to a concrete persistence state. Arbitrary sleeps, interception, mocks, skip/only selectors committed to source, and weakened assertions are forbidden.

## Acceptance Criteria

1. A current-session RED run reproduces omission of both exact provider rows.
2. At least one adversarial case fills the result limit with generic fixed-term contenders and still requires both exact Drive rows by the per-run term.
3. Google and memdrive rows appear in one authenticated live response with exact IDs and provider metadata.
4. Focused unit/integration/E2E, the serialized Drive package, check, lint, format, and packet guards pass.
5. Certification remains `in_progress`; no validate-owned completion is claimed.

## Deployment Boundary

This branch delivers source, tests, and packet evidence only. It does not deploy, modify a target adapter, touch `knb`, or mutate any release-train configuration.
