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

### Single-Capability Justification

- **Classification:** This is a regression repair inside the existing provider-neutral Drive search capability, not a new capability, provider, or shared contract. Google and memdrive already use the same Drive scan, extraction, persistence, and search surfaces.
- **Existing foundation and reuse path:** `internal/drive/scan.Service.InitialScan` and `internal/drive/extract.Service.ProcessPending` persist artifacts carrying `drive_connections.provider_id`; `internal/drive/consumers.LoadDriveArtifact` and the canonical `internal/api/search.go` `/api/search` handler consume those artifacts. The repair reuses that path and changes only the collision resistance of the regression query.
- **Consumer set:** The authenticated core search API, provider-neutral Drive artifact consumers, and the Google-plus-memdrive cross-feature E2E in `tests/e2e/drive/drive_cross_feature_e2e_test.go` continue to consume the same contract and metadata.
- **Why no new abstraction or provider registry is needed:** Provider identity, provider selection, and the shared artifact contract already exist. No provider behavior or extension point changes, so another registry or adapter layer would duplicate the established Drive foundation without serving a new variant.
- **Residual risk:** The packet evidence records the opt-in Ollama agent E2E as not executed. That coverage remains unproven and is routed to `bubbles.test` by this packet's existing `state.json.routing`; this requirement reconciliation does not convert it into execution evidence or a completion claim.

## Acceptance Criteria

1. A current-session RED run reproduces omission of both exact provider rows.
2. At least one adversarial case fills the result limit with generic fixed-term contenders and still requires both exact Drive rows by the per-run term.
3. Google and memdrive rows appear in one authenticated live response with exact IDs and provider metadata.
4. Focused unit/integration/E2E, the serialized Drive package, check, lint, format, and packet guards pass.
5. Certification remains `in_progress`; no validate-owned completion is claimed.

## Deployment Boundary

This branch delivers source, tests, and packet evidence only. It does not deploy, modify a target adapter, touch `knb`, or mutate any release-train configuration.
