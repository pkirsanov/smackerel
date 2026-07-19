# User Validation: BUG-069-004 - HTTP assistant turn deduplication

The packet remains in progress; validation-owned certification is not claimed.
Users may uncheck an item to report a regression after delivery.

## Checklist

- [x] Retrying the exact assistant HTTP turn with the same message ID returns one logical response and does not duplicate execution.
- [x] Different message IDs still create distinct assistant turns.
- [x] Two authenticated users can reuse the same opaque message ID without sharing responses.
- [x] Reusing one message ID with a changed request body is rejected rather than replayed or re-executed.
