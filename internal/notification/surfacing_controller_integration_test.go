//go:build integration

// Package notification: forward-looking integration test scaffolds for spec 054
// Scope 9 (Surfacing Controller Integration). These cover SCN-054-028 and
// SCN-054-030 and remain skipped until spec 021 M1a unified surfacing
// controller is delivered and Scope 9 implementation begins.
package notification

import "testing"

// TestControllerSuppressesNonUrgentProposalWhenGlobalBudgetExhausted
// covers SCN-054-028.
func TestControllerSuppressesNonUrgentProposalWhenGlobalBudgetExhausted(t *testing.T) {
	t.Skip("SCN-054-028: awaits spec 021 M1a unified surfacing controller delivery; see specs/054-notification-intelligence-handler/scopes.md Scope 9")
}

// TestAcknowledgmentOnOneSurfaceCancelsSiblingProposals covers SCN-054-030.
func TestAcknowledgmentOnOneSurfaceCancelsSiblingProposals(t *testing.T) {
	t.Skip("SCN-054-030: awaits spec 021 M1a unified surfacing controller delivery; see specs/054-notification-intelligence-handler/scopes.md Scope 9")
}
