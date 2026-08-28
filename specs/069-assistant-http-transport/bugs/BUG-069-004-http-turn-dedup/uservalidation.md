# User Validation: BUG-069-004 - HTTP assistant turn deduplication

The packet remains in progress; validation-owned certification is not claimed.
Users may uncheck an item to report a regression after delivery.

## Checklist

- [x] Retrying the exact assistant HTTP turn with the same message ID returns one logical response and does not duplicate execution.
- [x] Different message IDs still create distinct assistant turns.
- [x] Two authenticated users can reuse the same opaque message ID without sharing responses.
- [x] Reusing one message ID with a changed request body is rejected rather than replayed or re-executed.


## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-28
- method: external-record
- record: Operator directive in the working session on 2026-08-28, verbatim "authorized, approved, update all user validations as approved".

### Scope of this acceptance, stated precisely

The items above describe behaviour observed against a running system. **No agent
performed those observations interactively**; the acceptor is the operator. What
automation contributed is this packet's own test evidence, recorded above.
