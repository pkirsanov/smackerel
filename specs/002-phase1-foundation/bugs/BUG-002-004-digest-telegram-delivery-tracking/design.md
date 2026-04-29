# Bug Fix Design: BUG-002-004

## Root Cause Analysis

### Investigation Summary
The broad E2E suite reports `tests/e2e/test_digest_telegram.sh` failing `SCN-002-032` with `Digest delivery not tracked`. Existing spec 002 artifacts define `SCN-002-032` as the digest-via-Telegram contract and map it to `tests/e2e/test_digest_telegram.sh`. No matching bug packet existed before this classification pass.

### Root Cause
Unproven at packetization time. The fix owner must capture the targeted red-stage output and identify whether digest generation, Telegram send routing, delivery tracking persistence, fixture chat configuration, or the E2E tracking query is the broken contract.

### Impact Analysis
- Affected components: digest generation, Telegram delivery path, delivery tracking persistence or observability, E2E digest fixtures.
- Affected data: disposable E2E digest and delivery records.
- Affected users: users relying on Telegram digest delivery may receive no delivery, or operators may lack evidence that delivery occurred.

## Fix Design

### Solution Approach
Start with a targeted run of `test_digest_telegram.sh` that records generated digest identity, configured channel/chat, send attempt result, and delivery tracking query output. Repair the first confirmed broken production or harness contract while preserving the requirement that delivery is tracked, not merely generated.

### Alternative Approaches Considered
1. Accepting digest generation without delivery tracking - rejected because `SCN-002-032` requires Telegram delivery proof.
2. Skipping Telegram delivery in broad E2E - rejected because it removes protected Phase 1 coverage.
3. Replacing delivery tracking with a canned test marker - rejected because live-stack proof must exercise real persistence or operator-visible tracking.

## Regression Test Design
- Targeted E2E: `test_digest_telegram.sh` passes with a generated digest and matching delivery tracking record.
- Adversarial E2E: a generated digest without tracking fails loudly and cannot satisfy the scenario.
- Broad E2E: `./smackerel.sh test e2e` no longer reports the digest delivery tracking failure.
