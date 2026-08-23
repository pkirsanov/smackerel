# User Validation: BUG-069-006 Atomic confirm redemption

The packet is `in_progress` and nothing has been implemented. Every item below
is therefore **unchecked**, and that is the honest state rather than a formality.

The Bubbles convention checks acceptance items by default when an agent has just
validated the behaviour they describe. That convention does not apply here. The
behaviour these items describe - one execution under concurrent redemption -
does not exist in the current implementation. Checking them would assert exactly
the guarantee this packet was opened to report as absent.

These items become the operator acceptance contract once the implement phase
delivers. An item is checked only when it has been observed to hold.

## Checklist

- [ ] Two people, or one person double-tapping, cannot make a single confirmation execute a gated action twice.
- [ ] A confirm that loses a race is refused with the same message a user sees when confirming something already handled, revealing nothing about the race.
- [ ] The audit trail shows exactly one terminal outcome per confirmation, so it never records both an acceptance and a timeout for one card.
- [ ] Confirming a card normally still executes the action exactly once and returns the expected result.
- [ ] Re-submitting an already-used confirmation still declines to execute the action a second time.
- [ ] Cancelling a confirm card behaves correctly when a cancellation and an acceptance arrive at the same moment.
- [ ] A confirm card that expires while being accepted resolves to one outcome, not two.
- [ ] Nothing else in the conversation - working context, pending disambiguation, pending clarification - is disturbed when a confirmation is redeemed.

## Verification Steps

1. Start the disposable test stack through the repository CLI.
2. Propose a gated action so one confirm card is live for the test identity.
3. Issue two confirmations for that same card reference concurrently, each with
   a distinct transport message id, released together rather than in sequence.
4. Count the side effects of the gated action and confirm exactly one occurred.
5. Confirm one caller succeeded and the other was refused with the standard
   already-handled response.
6. Query the audit rows for that card reference and confirm exactly one terminal
   row exists.
7. Repeat steps 3 to 6 with an acceptance racing the expiry sweep, and confirm a
   single terminal outcome.
8. Run the existing sequential confirm scenario unmodified and confirm it still
   passes.
