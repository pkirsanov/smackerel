# Bug: BUG-002-004 Digest Telegram delivery tracking

## Summary
SCN-002-032 reports `test_digest_telegram.sh` failing because digest delivery is not tracked, blocking broad E2E certification for the Phase 1 digest delivery contract.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Broad E2E certification blocked for a protected digest delivery scenario
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [ ] Confirmed (targeted red-stage output must be captured by the fix owner)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run the broad E2E command through `./smackerel.sh test e2e`.
2. Allow `tests/e2e/test_digest_telegram.sh` to execute scenario `SCN-002-032`.
3. Generate or request a digest with Telegram delivery configured.
4. Observe whether the system records the delivery as tracked for the configured chat/channel.

## Expected Behavior
The digest should be generated and delivered to the configured Telegram chat, and the live-stack proof should observe the delivery tracking signal required by `SCN-002-032`.

## Actual Behavior
The broad E2E failure reports `SCN-002-032`: digest delivery not tracked.

## Environment
- Service: Go core digest generator, Telegram delivery path, PostgreSQL, E2E stack
- Version: Workspace state on 2026-04-28 during 039 broad E2E failure classification
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Broad E2E classification input:
tests/e2e/test_digest_telegram.sh SCN-002-032 digest delivery not tracked.
```

## Root Cause (initial analysis)
Root cause is unproven at packetization time. Candidate surfaces include digest delivery event persistence, Telegram delivery adapter instrumentation, configured chat fixture setup, test polling/query criteria, or broad-suite state isolation around generated digests.

## Related
- Feature: `specs/002-phase1-foundation/`
- Scenario: `SCN-002-032 Digest via Telegram`
- Scenario contract: `specs/002-phase1-foundation/scenario-manifest.json`
- E2E test: `tests/e2e/test_digest_telegram.sh`
