package telegram

import (
	"strings"
	"unicode"
)

// Text markers used by the Telegram bot. No emoji allowed.
const (
	MarkerSuccess   = ". " // saved/confirmed
	MarkerUncertain = "? " // uncertainty/low confidence
	MarkerAction    = "! " // action needed
	MarkerInfo      = "> " // information/result
	MarkerListItem  = "- " // list item
	MarkerContinued = "~ " // continued/related
	MarkerHeading   = "# " // section heading
	MarkerMention   = "@ " // person/entity mention
)

// AllMarkers returns the complete set of text markers.
func AllMarkers() []string {
	return []string{
		MarkerSuccess,
		MarkerUncertain,
		MarkerAction,
		MarkerInfo,
		MarkerListItem,
		MarkerContinued,
		MarkerHeading,
		MarkerMention,
	}
}

// FormatSuccess formats a success message.
func FormatSuccess(msg string) string {
	return MarkerSuccess + msg
}

// FormatInfo formats an information message.
func FormatInfo(msg string) string {
	return MarkerInfo + msg
}

// FormatAction formats an action-needed message.
func FormatAction(msg string) string {
	return MarkerAction + msg
}

// FormatUncertain formats an uncertain/low-confidence message.
func FormatUncertain(msg string) string {
	return MarkerUncertain + msg
}

// FormatList formats a list of items with the list marker.
func FormatList(items []string) string {
	var lines []string
	for _, item := range items {
		lines = append(lines, MarkerListItem+item)
	}
	return strings.Join(lines, "\n")
}

// ContainsEmoji returns true if the text contains any emoji characters.
func ContainsEmoji(text string) bool {
	for _, r := range text {
		if isEmoji(r) {
			return true
		}
	}
	return false
}

// SanitizeOutput removes any emoji from output text.
func SanitizeOutput(text string) string {
	var b strings.Builder
	for _, r := range text {
		if !isEmoji(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isEmoji checks if a rune is an emoji character.
func isEmoji(r rune) bool {
	// Emoji ranges
	if r >= 0x1F600 && r <= 0x1F9FF {
		return true
	} // Emoticons, Supplemental
	if r >= 0x2600 && r <= 0x26FF {
		return true
	} // Misc symbols
	if r >= 0x2700 && r <= 0x27BF {
		return true
	} // Dingbats
	if r >= 0x1F300 && r <= 0x1F5FF {
		return true
	} // Misc symbols & pictographs
	if r >= 0x1FA00 && r <= 0x1FA6F {
		return true
	} // Chess, extended-A
	if r >= 0x1FA70 && r <= 0x1FAFF {
		return true
	} // Symbols extended-A
	if r >= 0xFE00 && r <= 0xFE0F {
		return true
	} // Variation selectors
	if r == 0x200D {
		return true
	} // ZWJ
	// Don't flag standard punctuation and symbols
	if unicode.IsPrint(r) && r < 0x2000 {
		return false
	}
	return false
}
