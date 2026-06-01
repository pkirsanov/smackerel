// catalog.go — spec 075 SCOPE-1 retired-command catalog seam.
//
// The actual finite list of retired commands is owned by spec 066.
// This file declares only the interface the policy consumes, so the
// foundation package never copies or expands the retired-command
// list. Scope 2 wires a concrete implementation that delegates to
// spec 066.
package legacyretirement

// RetiredCommandCatalog is the read-only seam the policy uses to
// classify an inbound token as a retired command. Implementations
// MUST return the canonical RetiredCommand for the token; the
// catalog is the only place that defines NoticeCopy and
// ReplacementExample, so notice rendering cannot drift from the SST.
type RetiredCommandCatalog interface {
	// Lookup returns the catalog entry for token (e.g. "/weather"),
	// or ok=false if the token is not a retired command. Lookup MUST
	// be a pure, side-effect-free read.
	Lookup(token string) (RetiredCommand, bool)
	// All returns every retired command. The policy uses this at
	// startup to verify that legacy_retirement.notice_copy_per_command
	// and legacy_retirement.post_window_unknown_response_copy cover
	// every catalog entry (fail-loud if any are missing).
	All() []RetiredCommand
}
