// BUG-061-012 R3.3 — contract tests for the system principal.
//
// system_session.go names TestSystemSession_SourceIsNonEmptySoInjectionIsNotANoOp
// as the guard on its most load-bearing invariant. That reference is only worth
// anything if the test exists, so it lives here.

package auth

import (
	"context"
	"slices"
	"testing"
)

// WithSession returns the context UNCHANGED when Source is empty. If
// SessionSourceSystem were ever emptied, every scheduler/pipeline/judgment/
// annotation injection would become a silent no-op and the surfaces would go
// back to failing closed by accident — the exact state SystemSession replaced.
func TestSystemSession_SourceIsNonEmptySoInjectionIsNotANoOp(t *testing.T) {
	if SessionSourceSystem == "" {
		t.Fatal("SessionSourceSystem is empty; WithSession would drop every system injection on the floor")
	}

	ctx := WithSession(context.Background(), SystemSession("scheduler"))
	sess, ok := SessionFromContext(ctx)
	if !ok {
		t.Fatal("system session did not survive WithSession; the injection is a no-op")
	}
	if !IsSystem(sess) {
		t.Errorf("IsSystem(system session) = false, source = %q", sess.Source)
	}
}

// The grant set is empty ON PURPOSE, and empty is not nil: nil is Session.Scopes'
// "legacy non-scoped session" sentinel, which is a different claim.
func TestSystemSession_HoldsNoCorpusGrantAndIsScopedNotLegacy(t *testing.T) {
	for _, component := range []string{"scheduler", "pipeline", "judgment", "annotation"} {
		t.Run(component, func(t *testing.T) {
			sess := SystemSession(component)
			if sess.Scopes == nil {
				t.Fatal("Scopes is nil; that reads as a legacy non-scoped session rather than one deliberately scoped to nothing")
			}
			if len(sess.Scopes) != 0 {
				t.Errorf("Scopes = %v, want empty; a server trigger acts on its own behalf and has no user grant to exercise", sess.Scopes)
			}
			if slices.Contains(sess.Scopes, GrantGlobalCorpusRead) {
				t.Errorf("system principal holds %q", GrantGlobalCorpusRead)
			}
			if GateGlobalCorpusRead(sess).Allowed {
				t.Error("system principal is authorized to read the corpus")
			}
			if sess.UserID != "system:"+component {
				t.Errorf("UserID = %q, want %q so the audit trail names the acting surface", sess.UserID, "system:"+component)
			}
		})
	}
}

// A system UserID must never be mistakable for a real auth_users.user_id, and a
// blank component must not erase the prefix that guarantees it.
func TestSystemSession_BlankComponentStillNamespaced(t *testing.T) {
	for _, in := range []string{"", "   ", "\t"} {
		sess := SystemSession(in)
		if sess.UserID != "system:unspecified" {
			t.Errorf("SystemSession(%q).UserID = %q, want system:unspecified", in, sess.UserID)
		}
		if !IsSystem(sess) {
			t.Errorf("SystemSession(%q) is not identifiable as a system session", in)
		}
	}
}

// IsSystem must key off Source, not the UserID string. A user who somehow held
// a "system:"-prefixed id must not be granted system identity, and — the
// direction that actually matters — a real user session must never test true.
func TestSystemSession_IsSystemIsNotVacuous(t *testing.T) {
	if IsSystem(Session{UserID: "system:scheduler", Source: SessionSourcePerUserToken}) {
		t.Error("IsSystem matched on the UserID string; a real token could impersonate a system actor")
	}
	if IsSystem(Session{}) {
		t.Error("IsSystem(zero session) = true; an absent session is not a system session")
	}
	if !IsSystem(SystemSession("pipeline")) {
		t.Error("IsSystem(SystemSession) = false; the checker cannot recognize its own constructor")
	}
}
