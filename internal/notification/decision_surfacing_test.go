// Package notification: forward-looking test scaffolds for spec 054 Scope 9
// (Surfacing Controller Integration). These tests are scenario placeholders
// for SCN-054-027 and SCN-054-029. They remain skipped until spec 021 M1a
// unified surfacing controller is delivered and Scope 9 implementation begins.
package notification

import "testing"

// TestDecisionEnginePublishesSurfacingProposalInsteadOfDirectDispatch
// covers SCN-054-027.
func TestDecisionEnginePublishesSurfacingProposalInsteadOfDirectDispatch(t *testing.T) {
	t.Skip("SCN-054-027: awaits spec 021 M1a unified surfacing controller delivery; see specs/054-notification-intelligence-handler/scopes.md Scope 9")
}

// TestUrgentDecisionMarksProposalForBudgetBypass covers SCN-054-029.
func TestUrgentDecisionMarksProposalForBudgetBypass(t *testing.T) {
	t.Skip("SCN-054-029: awaits spec 021 M1a unified surfacing controller delivery; see specs/054-notification-intelligence-handler/scopes.md Scope 9")
}
