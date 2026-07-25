// Package auth — browser session role/grant model and global-corpus gate
// (BUG-070-001 SCOPE-01 / SCOPE-06, AUTH-012 / AUTH-014). Authority is granted
// explicitly, never inferred from possession of a valid credential. There is no
// wildcard grant and no implicit default grant: a capability not present in the
// session's persisted grant snapshot is not held.
//
// Product decision reconciled by BUG-070-001: there is ONE operator-owned
// global corpus accessed via explicit roles/grants — no tenant and no per-user
// row isolation. The corpus read gate therefore turns on a single explicit
// grant and asserts no isolation claim.
package auth

import "slices"

// Role is a named authority tier for a browser session. Each role maps to an
// EXPLICIT persisted grant snapshot; the role name alone confers nothing —
// authorization always evaluates the grant set.
type Role string

const (
	// RoleDailyUser is the ordinary product user. It holds the daily
	// capability grants and no operator grant.
	RoleDailyUser Role = "daily-user"
	// RoleOperator is the elevated user. Operator authority is granted, never
	// implied by holding a valid credential.
	RoleOperator Role = "operator"
)

// Grant identifiers are well-formed <surface>:<capability> scope strings
// (validated by ValidateScopeName). None is a wildcard; the vocabulary is
// closed and explicit.
const (
	// GrantAssistantTurn authorizes an Assistant turn (daily capability).
	GrantAssistantTurn = "assistant:turn"
	// GrantKnowledgeGraphRead authorizes reading the Knowledge Graph (daily).
	GrantKnowledgeGraphRead = "knowledge-graph:read"
	// GrantGlobalCorpusRead authorizes reading the single operator-owned global
	// private corpus (Graph / Digest / Synthesis). Held by the operator and by
	// a specifically-granted daily user; NOT part of the daily default set.
	GrantGlobalCorpusRead = "corpus:read"
	// GrantOperatorAdmin authorizes operator/admin surfaces (operator only).
	GrantOperatorAdmin = "operator:admin"
	// GrantOperatorModelPicker authorizes the model-picker/admin surface
	// (operator only).
	GrantOperatorModelPicker = "operator:model-picker"
)

// wildcardGrant is a sentinel that MUST NEVER be honored. Mirrors the spec 060
// BS-002 invariant enforced by RequireScope: a wildcard is never a grant.
const wildcardGrant = "*"

// dailyUserGrants is the explicit default grant snapshot for RoleDailyUser.
// No wildcard, no operator grant, no corpus grant.
var dailyUserGrants = []string{GrantAssistantTurn, GrantKnowledgeGraphRead}

// operatorGrants is the explicit default grant snapshot for RoleOperator. It is
// NOT computed as "daily + extra"; it is an independent explicit set so a
// change to the daily set cannot silently widen operator authority.
var operatorGrants = []string{
	GrantAssistantTurn,
	GrantKnowledgeGraphRead,
	GrantGlobalCorpusRead,
	GrantOperatorAdmin,
	GrantOperatorModelPicker,
}

// GrantsForRole returns a copy of the explicit default grant snapshot for a
// role. Unknown roles return an empty set (no implicit default grant).
func GrantsForRole(role Role) []string {
	switch role {
	case RoleDailyUser:
		return append([]string(nil), dailyUserGrants...)
	case RoleOperator:
		return append([]string(nil), operatorGrants...)
	default:
		return nil
	}
}

// SessionWithRole builds a browser-purpose Session carrying the role's default
// grant snapshot plus any explicit per-principal extra grants (e.g. a daily
// user specifically granted corpus:read). Used by issuance and by tests; the
// persisted grant snapshot lands in Session.Scopes, which is the single
// authority source consulted by AuthorizeGrant and GateGlobalCorpusRead.
func SessionWithRole(userID, tokenID string, role Role, extraGrants ...string) Session {
	grants := GrantsForRole(role)
	for _, g := range extraGrants {
		if g != "" && !slices.Contains(grants, g) {
			grants = append(grants, g)
		}
	}
	return Session{
		UserID:  userID,
		TokenID: tokenID,
		Source:  SessionSourcePerUserToken,
		Scopes:  grants,
	}
}

// GrantDecision is the outcome of an authorization check. Reason is a bounded,
// non-sensitive label for telemetry/tests; it never carries identity or token
// material.
type GrantDecision struct {
	Allowed bool
	Reason  string
}

// AuthorizeGrant decides whether the authenticated session holds `required`.
// Authority comes ONLY from the session's persisted grant snapshot
// (Session.Scopes). A wildcard is never honored, and a bare valid session (nil
// or empty Scopes) implies no grant.
func AuthorizeGrant(sess Session, required string) GrantDecision {
	if slices.Contains(sess.Scopes, wildcardGrant) {
		// Defense-in-depth: a wildcard sentinel MUST NEVER widen authority.
		return GrantDecision{Allowed: false, Reason: "wildcard_grant_forbidden"}
	}
	if slices.Contains(sess.Scopes, required) {
		return GrantDecision{Allowed: true}
	}
	return GrantDecision{Allowed: false, Reason: "grant_absent"}
}

// CorpusDecision is the outcome of the single operator-owned global-corpus read
// gate. A denied decision deliberately carries NO content, count, label, or
// existence hint — the caller renders a bare 403. The model is one global
// corpus, so the decision asserts no tenant or per-user row isolation.
type CorpusDecision struct {
	Allowed bool
}

// GateGlobalCorpusRead gates a read of the single global private corpus on the
// explicit GrantGlobalCorpusRead grant. The operator and a specifically-granted
// daily user pass; an ungranted daily user is denied. A wildcard is never
// honored. There is no tenant or per-user isolation parameter — the corpus is
// one global store and access is grant-gated, not row-partitioned.
func GateGlobalCorpusRead(sess Session) CorpusDecision {
	if slices.Contains(sess.Scopes, wildcardGrant) {
		return CorpusDecision{Allowed: false}
	}
	return CorpusDecision{Allowed: slices.Contains(sess.Scopes, GrantGlobalCorpusRead)}
}
