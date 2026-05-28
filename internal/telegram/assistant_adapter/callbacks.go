package assistant_adapter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// CallbackData encoding scheme (Telegram callback_data is bounded at
// 64 bytes; the prefix + ULID-shaped ref + suffix fit well within it):
//
//	"a:c:<confirmRef>:<pos|neg>"      — confirm card
//	"a:d:<disambiguationRef>:<num>"   — disambiguation choice
//
// The "a:" namespace prefix lets the bot's safeHandleCallback route
// assistant callbacks distinctly from spec 028 list callbacks
// (handleListCallback uses a different scheme — see internal/telegram/list.go).
//
// Any callback_data not matching either of the two assistant shapes
// returns ErrNotAssistantMessage from translateCallback so the bot
// falls through to its existing handler.
const (
	callbackPrefix         = "a:"
	callbackConfirmPrefix  = "a:c:"
	callbackDisambigPrefix = "a:d:"
)

// callbackKind discriminates the two assistant callback shapes.
type callbackKind int

const (
	callbackKindUnknown callbackKind = iota
	callbackKindConfirm
	callbackKindDisambig
)

// decodedCallback is the shape produced by decodeCallbackData.
type decodedCallback struct {
	kind   callbackKind
	ref    string
	choice contracts.ConfirmChoice // confirm only
	number int                     // disambig only
}

// encodeConfirmCallback builds the callback_data string for a
// confirm-card button. The (ref, choice) pair round-trips through
// decodeCallbackData losslessly. The function does NOT enforce the
// 64-byte Telegram cap — callers are expected to keep ConfirmRef
// short (capability layer emits ULIDs, 26 chars).
func encodeConfirmCallback(ref string, choice contracts.ConfirmChoice) string {
	suffix := "pos"
	if choice == contracts.ConfirmNegative {
		suffix = "neg"
	}
	return fmt.Sprintf("%s%s:%s", callbackConfirmPrefix, ref, suffix)
}

// encodeDisambigCallback builds the callback_data string for a
// disambiguation-choice button. (ref, number) round-trips through
// decodeCallbackData losslessly.
func encodeDisambigCallback(ref string, number int) string {
	return fmt.Sprintf("%s%s:%d", callbackDisambigPrefix, ref, number)
}

// IsAssistantCallback reports whether the supplied callback_data
// targets the assistant adapter. The bot's safeHandleCallback uses
// this to decide between assistant routing and the existing
// handleListCallback path. Strict prefix match — never claims a
// non-assistant callback.
func IsAssistantCallback(data string) bool {
	return strings.HasPrefix(data, callbackPrefix)
}

// decodeCallbackData parses an assistant callback_data string into a
// decodedCallback. Returns ErrNotAssistantMessage when the prefix is
// not "a:" (non-assistant callback) and a descriptive error when the
// payload is malformed.
func decodeCallbackData(data string) (decodedCallback, error) {
	if !strings.HasPrefix(data, callbackPrefix) {
		return decodedCallback{}, ErrNotAssistantMessage
	}
	switch {
	case strings.HasPrefix(data, callbackConfirmPrefix):
		return decodeConfirm(data[len(callbackConfirmPrefix):])
	case strings.HasPrefix(data, callbackDisambigPrefix):
		return decodeDisambig(data[len(callbackDisambigPrefix):])
	default:
		return decodedCallback{}, fmt.Errorf("assistant_adapter: callback_data %q has unknown assistant subprefix", data)
	}
}

// decodeConfirm parses "<ref>:<pos|neg>" → decodedCallback.
func decodeConfirm(payload string) (decodedCallback, error) {
	idx := strings.LastIndex(payload, ":")
	if idx <= 0 || idx == len(payload)-1 {
		return decodedCallback{}, fmt.Errorf("assistant_adapter: malformed confirm callback %q", payload)
	}
	ref := payload[:idx]
	suffix := payload[idx+1:]
	var choice contracts.ConfirmChoice
	switch suffix {
	case "pos":
		choice = contracts.ConfirmPositive
	case "neg":
		choice = contracts.ConfirmNegative
	default:
		return decodedCallback{}, fmt.Errorf("assistant_adapter: confirm callback choice %q is not pos/neg", suffix)
	}
	return decodedCallback{
		kind:   callbackKindConfirm,
		ref:    ref,
		choice: choice,
	}, nil
}

// decodeDisambig parses "<ref>:<number>" → decodedCallback.
func decodeDisambig(payload string) (decodedCallback, error) {
	idx := strings.LastIndex(payload, ":")
	if idx <= 0 || idx == len(payload)-1 {
		return decodedCallback{}, fmt.Errorf("assistant_adapter: malformed disambig callback %q", payload)
	}
	ref := payload[:idx]
	num, err := strconv.Atoi(payload[idx+1:])
	if err != nil {
		return decodedCallback{}, fmt.Errorf("assistant_adapter: disambig callback number %q not numeric: %w", payload[idx+1:], err)
	}
	if num <= 0 {
		return decodedCallback{}, fmt.Errorf("assistant_adapter: disambig callback number %d not 1-indexed", num)
	}
	return decodedCallback{
		kind:   callbackKindDisambig,
		ref:    ref,
		number: num,
	}, nil
}
