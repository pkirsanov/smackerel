package telegram

// Text markers used by the Telegram bot. No emoji allowed.
// Full set of 8 markers per spec SCN-001-004: . ? ! > - ~ # @
const (
	MarkerSuccess   = ". " // saved/confirmed
	MarkerUncertain = "? " // uncertainty/low confidence
	MarkerAction    = "! " // action needed
	MarkerInfo      = "> " // information/result
	MarkerListItem  = "- " // list item
	MarkerContinued = "~ " // continued/related
	MarkerHeading   = "# " // heading/topic
	MarkerMention   = "@ " // mention/entity reference
)
