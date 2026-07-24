package proactive

import "strings"

// NudgeAction is the closed set of actions a user can take on a proactive nudge
// card. The set is identical on every channel (web, Telegram, WhatsApp); only
// the surrounding markup differs (FR-107-005/006/007 cross-channel parity).
type NudgeAction int

const (
	// ActionUnknown is the zero value; it never encodes to a wire byte and is
	// only ever produced by a failed decode.
	ActionUnknown NudgeAction = iota
	// ActionAct performs the card's primary action and acknowledges it.
	ActionAct
	// ActionSnooze defers the card and acknowledges it (MVP reuses the
	// suppression window; there is no separate snooze store — design.md OQ6).
	ActionSnooze
	// ActionDismiss dismisses the card and acknowledges it.
	ActionDismiss
)

// String renders a stable, human-legible label. It is NOT the wire encoding;
// use WireByte for the transport byte.
func (a NudgeAction) String() string {
	switch a {
	case ActionAct:
		return "act"
	case ActionSnooze:
		return "snooze"
	case ActionDismiss:
		return "dismiss"
	default:
		return "unknown"
	}
}

// WireByte returns the single-character wire encoding for a in the
// a:n:<ref>:<a|s|d> callback form. ok is false for ActionUnknown so a caller
// never emits an ambiguous byte.
func (a NudgeAction) WireByte() (byte, bool) {
	switch a {
	case ActionAct:
		return 'a', true
	case ActionSnooze:
		return 's', true
	case ActionDismiss:
		return 'd', true
	default:
		return 0, false
	}
}

// ParseWireByte maps a wire byte ('a'/'s'/'d') back to a NudgeAction. ok is
// false for any other byte.
func ParseWireByte(b byte) (NudgeAction, bool) {
	switch b {
	case 'a':
		return ActionAct, true
	case 's':
		return ActionSnooze, true
	case 'd':
		return ActionDismiss, true
	default:
		return ActionUnknown, false
	}
}

// NudgeCallbackPrefix is the additive "assistant nudge" callback subprefix. It
// is parallel to the existing a:c: (confirm) and a:d: (disambiguation)
// assistant families in internal/telegram/assistant_adapter and shares the
// leading a: assistant namespace, so an existing IsAssistantCallback check
// already routes it to the assistant adapter. The Telegram callback_data and
// the WhatsApp interactive reply-id both carry this exact form, so one wire
// shape routes every channel to the one NudgeRegistry + ack path.
const NudgeCallbackPrefix = "a:n:"

// nudgeCallbackByteBudget is the maximum encoded length of a nudge callback:
// "a:n:" (4) + a 26-char ULID-shaped ref + ":" (1) + one action byte (1) = 32,
// comfortably inside Telegram's documented 64-byte callback_data bound and a
// WhatsApp reply-id. Exported as a compile-time-checkable constant for tests.
const nudgeCallbackByteBudget = 64

// EncodeNudgeCallback builds "a:n:<ref>:<a|s|d>".
//
// ref MUST be the opaque, ULID-shaped NudgeRef minted by the registry; the
// content_key is NEVER encoded — this function is the wire half of the
// anti-leak boundary (FR-107-028). It returns ok=false for an empty ref or an
// unknown action rather than emitting a malformed or leaking payload.
func EncodeNudgeCallback(ref NudgeRef, action NudgeAction) (string, bool) {
	if ref == "" {
		return "", false
	}
	if strings.ContainsAny(string(ref), ":") {
		// A ref must be an opaque token with no delimiter; refuse rather than
		// emit an ambiguous callback that could mis-parse.
		return "", false
	}
	b, ok := action.WireByte()
	if !ok {
		return "", false
	}
	out := NudgeCallbackPrefix + string(ref) + ":" + string(b)
	if len(out) > nudgeCallbackByteBudget {
		return "", false
	}
	return out, true
}

// DecodeNudgeCallback parses "a:n:<ref>:<a|s|d>" into its opaque ref and action.
//
// It accepts the full callback string (including the a:n: prefix). ok is false
// (never a panic) for any payload that is not a well-formed nudge callback,
// including the a:c:/a:d: families — so a decoder can fall through to other
// callback families without collision (FR-107-006 non-collision).
func DecodeNudgeCallback(data string) (ref NudgeRef, action NudgeAction, ok bool) {
	if !strings.HasPrefix(data, NudgeCallbackPrefix) {
		return "", ActionUnknown, false
	}
	payload := data[len(NudgeCallbackPrefix):]
	// payload is "<ref>:<action>"; the action is a single trailing byte.
	idx := strings.LastIndex(payload, ":")
	if idx <= 0 || idx != len(payload)-2 {
		return "", ActionUnknown, false
	}
	refPart := payload[:idx]
	if refPart == "" {
		return "", ActionUnknown, false
	}
	act, aok := ParseWireByte(payload[idx+1])
	if !aok {
		return "", ActionUnknown, false
	}
	return NudgeRef(refPart), act, true
}
