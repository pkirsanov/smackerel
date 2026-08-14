// System principal for server-initiated agent invocations (BUG-061-012 R3.3).
//
// Scheduler, pipeline, judgment, and annotation-classifier invocations have no
// human caller. Before this constructor they passed a context carrying no
// session at all, which failed closed — but only by accident. Nothing recorded
// that "no principal" was the intent, so the day a default session appeared
// upstream those surfaces would have silently gained whatever it granted.
//
// An explicit principal with an explicitly empty grant set states the intent
// and, unlike an absent session, can be asserted against: a test can prove the
// system principal does NOT hold auth.GrantGlobalCorpusRead. Absence cannot be
// distinguished from an oversight; a declaration can.
package auth

import "strings"

// SessionSourceSystem — request originated inside the process (scheduler tick,
// pipeline stage, judgment evaluation, annotation classification) rather than
// from any external caller.
//
// This value MUST stay non-empty. WithSession returns the context UNCHANGED
// when sess.Source == "", so an empty source would make every system injection
// a silent no-op and quietly restore the accidental-fail-closed this exists to
// replace. TestSystemSession_SourceIsNonEmptySoInjectionIsNotANoOp guards it.
const SessionSourceSystem SessionSource = "system"

// SystemSession builds the principal for a server-initiated invocation.
//
// The grant set is explicitly empty. A system trigger has no corpus authority:
// it acts on its own behalf, not on behalf of a user, so there is no user whose
// grant it could be exercising. Corpus tools therefore fail closed for these
// surfaces by construction. Widening this set is a security decision and must
// be made deliberately in this one place rather than at a call site.
//
// component identifies the originating subsystem ("scheduler", "pipeline",
// "judgment", "annotation") and lands in UserID so the audit trail names which
// surface acted. It is prefixed to keep a system actor from ever colliding
// with a real auth_users.user_id.
func SystemSession(component string) Session {
	component = strings.TrimSpace(component)
	if component == "" {
		component = "unspecified"
	}
	return Session{
		UserID: "system:" + component,
		Source: SessionSourceSystem,
		// Explicitly empty, NOT nil: nil is Session.Scopes' "legacy
		// non-scoped session" sentinel, and this is the opposite — a
		// session that was scoped, deliberately, to nothing.
		Scopes: []string{},
	}
}

// IsSystem reports whether the session was produced by SystemSession. Callers
// that must refuse server-initiated invocations (rather than merely deny them a
// grant) test this instead of string-matching UserID.
func IsSystem(sess Session) bool {
	return sess.Source == SessionSourceSystem
}
