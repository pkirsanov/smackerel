package telegram

// Text markers used by the Telegram bot. No emoji allowed.
const (
	MarkerSuccess   = ". " // saved/confirmed
	MarkerUncertain = "? " // uncertainty/low confidence
	MarkerAction    = "! " // action needed
	MarkerInfo      = "> " // information/result
	MarkerListItem  = "- " // list item
	MarkerContinued = "~ " // continued/related
)
