# User Validation: 021 Intelligence Delivery

**Feature:** 021-intelligence-delivery
**Created:** 2026-04-10

---

## Acceptance Checklist

### Alert Delivery Pipeline
- [x] Pending alerts are delivered via Telegram within 15 minutes of creation
- [x] The 2/day delivery cap is enforced — no more than 2 alerts delivered per calendar day
- [x] Snoozed alerts with expired snooze_until are picked up by the delivery sweep
- [x] Telegram delivery failures do not mark alerts as delivered; retried next cycle
- [x] Alert messages include type icon, title, body, and priority formatting

### Alert Producers
- [x] Bill alerts created for subscriptions with next billing ≤3 days
- [x] Trip prep alerts created for upcoming trips with departure ≤5 days
- [x] Return window alerts created for artifacts with return_deadline ≤5 days (priority 1)
- [x] Relationship cooling alerts created for contacts with >30 day gap + prior ≥1/week frequency
- [x] All 6 alert types have automated producers (including existing commitment_overdue and meeting_brief)
- [x] Each alert producer deduplicates — no duplicate alerts for the same condition

### Search Logging
- [x] Every search query is logged via LogSearch() after execution
- [x] LogSearch() failure does not affect the search response
- [x] DetectFrequentLookups returns results when the same query appears 3+ times in 14 days

### Intelligence Health Freshness
- [x] Health endpoint reports intelligence as "stale" when last synthesis > 48 hours
- [x] Stale intelligence contributes to overall "degraded" health status
- [x] Health reports "up" when synthesis is recent (< 48 hours)

## Notes

Baseline checklist — items checked by default. Uncheck any item that fails validation.
