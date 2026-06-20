# User Validation — Spec 097 Card-Rewards Google Calendar Delivery

> Items default to checked `[x]` (validated by the delivering agent). The
> operator unchecks an item to report it is not working as expected.

## Checklist

- [x] A production Google Calendar write client exists (idempotent create/update).
- [x] Card-rewards recommendations are delivered to the configured Google Calendar when calendar_sync is enabled.
- [x] Re-running the sync updates existing events rather than duplicating them.
- [x] When calendar_sync is disabled, recommendations stay in the Web UI and nothing is written to the calendar.
- [x] The Google credential is a managed secret (sops-encrypted via the deploy adapter), never committed or logged.
- [ ] A real card-rewards event is visible on the operator's "Credit cards" Google Calendar (live home-lab proof — SCOPE-02).
