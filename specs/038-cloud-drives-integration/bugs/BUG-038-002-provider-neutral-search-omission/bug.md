# Bug: BUG-038-002 Provider-neutral Drive rows omitted from live search

## Summary

After successful Google and memdrive scan/extract passes, the authenticated live `/api/search` endpoint omitted both newly persisted provider rows from a `tomato salad` query.

## Severity

- [ ] Critical - System unusable or data loss
- [ ] High - Provider-neutral Drive recall is broken without a reliable workaround
- [x] Medium - Broad validation is nondeterministic under shared search state
- [ ] Low - Minor issue

## Status

- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Start the real disposable `smackerel-test` stack through `./smackerel.sh test e2e`.
2. Run `TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers` in the serialized Drive E2E package.
3. Let the test scan and extract one Google file and one memdrive file containing `tomato salad`.
4. Confirm both provider-neutral artifacts can be loaded directly from the canonical artifact store.
5. POST `{"query":"tomato salad","limit":20}` to the authenticated live `/api/search` endpoint.
6. Observe whether both exact artifact IDs are returned with their provider metadata.

## Expected Behavior

The live search response returns both newly indexed artifacts, preserving exact Google and memdrive identities and provider-neutral Drive metadata. Earlier-package rows must not make the regression depend on shared search-corpus ordering.

## Actual Behavior

The synthesis closeout broad run logged `seen=1 indexed=1` for both providers, then returned neither exact artifact ID: `google=false mem=false`.

## Environment

- Service: `smackerel-core` search plus Drive scan/extract
- Source baseline: `a6d2fb3ffd03e7b09e294f2cdac14816fb2f5d4f`
- Test category: live `e2e-api`, disposable Docker stack, serialized Go packages
- Platform: Linux

## Error Output

```text
=== RUN   TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers
drive scan: completed provider=google seen=1 indexed=1 skipped=0
drive scan: completed provider=memdrive seen=1 indexed=1 skipped=0
drive_cross_feature_e2e_test.go:147: /api/search must return BOTH provider rows; google=false mem=false
--- FAIL: TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers (3.87s)
```

This output is inherited from the synthesis packet and is routing provenance, not current-session RED certification. Current-session reproduction is recorded in `report.md` before implementation changes.

## Root Cause

The live regression used the fixed, generic query `tomato salad` against a bounded result set on a database shared by every earlier Go E2E package. Twenty pre-existing exact-title contenders fill the `limit: 20` text-search window and deterministically reproduce the broad failure even though both Drive rows are synchronously persisted and directly loadable. The test mistook shared search-corpus ordering for provider-index failure. The fix gives both provider fixtures a collision-resistant per-run term and queries that term; the twenty generic contenders remain in the test as an adversarial guard. Exact provider IDs and metadata assertions are unchanged.

## Related

- Owning feature: `specs/038-cloud-drives-integration/`
- Owning requirements: FR-001, FR-003, FR-011, UC-003, UC-008
- Parent synthesis evidence: `specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair/report.md#independent-broad-findings-routed-out-of-packet`
- Independent health packet: `../BUG-038-003-drive-e2e-core-health-collapse/`
