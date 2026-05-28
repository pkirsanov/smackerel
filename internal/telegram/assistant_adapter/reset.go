package assistant_adapter

// reset.go documents the /reset handling contract.
//
// /reset is the user-facing slash command that drops the user's
// pending confirm-card and disambiguation state for the current
// (UserID, Transport) pair. The adapter translates the inbound
// "/reset" message into an AssistantMessage{Kind: KindReset} and
// delegates everything else to the capability layer:
//
//   - The facade calls contextStore.DeleteByKey(userID, transport).
//   - The facade returns an AssistantResponse with
//     Status=StatusSavedAsIdea (per design — the user-visible ack)
//     OR an explicit "reset" status if the capability layer ships
//     one in v2.
//   - The adapter renders the resulting AssistantResponse normally.
//
// The translation logic lives in translate_inbound.go::isResetCommand
// + the corresponding switch arm; this file is documentation only.
//
// Test surface: reset_test.go in this package asserts that the
// translation produces Kind=KindReset and that the renderer surfaces
// the facade's ack body verbatim.
