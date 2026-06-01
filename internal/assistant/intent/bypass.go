// Spec 068 SCOPE-1 — Operational-command bypass.
//
// spec.md Hard Constraint 1: only the carve-out set below bypasses the
// compiler. Every bypass is labelled in the trace so it is auditable
// and so the (later) policy guard from spec 067 can prove the carve-
// out is tiny and explicit.

package intent

import "strings"

// OperationalCommands is the closed, v1-frozen carve-out set defined
// in spec.md §"Hard Constraint 1". This package OWNS the canonical
// list; callers MUST NOT maintain a parallel list (Principle: one
// graph, many views).
var OperationalCommands = map[string]struct{}{
	"/help":   {},
	"/status": {},
	"/reset":  {},
	"/digest": {},
	"/recent": {},
	"/done":   {},
}

// BypassTraceLabel is the single trace label every operational-command
// bypass turn is stamped with (spec.md SCN-068-A07).
const BypassTraceLabel = "operational_command_bypass"

// IsOperationalCommand classifies an inbound message text as an
// operational-command bypass turn. Matching rules mirror the spec 061
// shortcut conventions: case-sensitive, first whitespace-delimited
// token, leading whitespace trimmed.
//
// Returns the matched command (e.g. "/status") and true on hit; empty
// string and false otherwise (including for the empty input).
func IsOperationalCommand(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	first := trimmed
	for i, r := range trimmed {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			first = trimmed[:i]
			goto check
		}
	}
check:
	if _, ok := OperationalCommands[first]; ok {
		return first, true
	}
	return "", false
}

// BypassTrace constructs the CompilerTrace recorded for an
// operational-command bypass turn.
func BypassTrace(rawText, command string) CompilerTrace {
	return CompilerTrace{
		RawText: rawText,
		Outcome: OutcomeBypass,
		Bypass: &BypassRecord{
			Command: command,
			Label:   BypassTraceLabel,
		},
	}
}
