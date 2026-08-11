// Package auth — delegation ceiling for bridged (credential-less) principals.
//
// Spec 108 design.md §10.7. A bridge mints a token for a principal who never
// presented a credential: the Telegram bridge synthesizes authority from a
// chat→user mapping, which is a weaker binding than token possession. The
// ceiling below bounds what such a synthesized token may carry.
//
// This lives beside the grant vocabulary rather than inside internal/telegram
// on purpose. Spec 108 §18 decision 3 makes "authority is defined at the
// principal, never at the minter" permanent, and the structural way to hold
// that line is for the bridge package to contain no scope literal at all —
// asserted by the grep guard in internal/telegram/scope_literal_guard_test.go.
package auth

import "slices"

// GrantAnnotationEdit authorizes writing an annotation. Gated at
// internal/api/router.go on the /api/artifacts/{id}/annotations group.
const GrantAnnotationEdit = "annotation:edit"

// TelegramBridgeDelegableGrants is the closed set of grants a Telegram bridge
// token may carry. Membership is derived from the gated internal routes the
// bridge actually calls (spec 108 design.md §10.7), enumerated rather than
// sampled:
//
//	corpus:read      → /api/search, /api/digest, /api/recent, /api/knowledge
//	annotation:edit  → /api/artifacts/{id}/annotations
//
// The bridge's other internal calls (/api/capture, /api/health, /api/lists,
// /api/expenses, /api/internal/telegram-message-artifact, /v1/photos/upload)
// carry no RequireScope, and the assistant is reached in-process rather than
// over HTTP, so no assistant grant is required.
//
// This is a CEILING, not a grant list. It only ever WITHHOLDS authority the
// principal already holds; it can never confer authority the principal lacks.
// That distinction is what separates it from the minter-side list §18
// decision 3 rejected, and DeriveTelegramBridgeGrants makes it mechanical:
// derived ⊆ recorded for every input, so an ungranted principal stays
// ungranted no matter what this set contains.
//
// A capability missing from this set fails closed — a new bridge command
// surfaces as a 403, which is a visible failure rather than a silent one.
var TelegramBridgeDelegableGrants = []string{
	GrantGlobalCorpusRead,
	GrantAnnotationEdit,
}

// DeriveTelegramBridgeGrants returns recorded ∩ TelegramBridgeDelegableGrants.
//
// Two properties hold by construction rather than by review:
//
//   - derived ⊆ recorded — every returned element passed a membership test
//     against recorded, so no scope is ever conferred.
//   - derived ⊆ TelegramBridgeDelegableGrants — iteration is over the ceiling,
//     so output order is deterministic regardless of the order the database
//     returned the recorded set in.
//
// The wildcard sentinel is never honored, mirroring the RequireScope invariant:
// a recorded "*" grants nothing here either.
//
// An empty result is returned as an empty slice, never as a permissive one.
// Callers MUST treat it as "no delegable authority" and refuse the mint; a
// zero-scope bridge token could reach nothing anyway.
func DeriveTelegramBridgeGrants(recorded []string) []string {
	derived := make([]string, 0, len(TelegramBridgeDelegableGrants))
	for _, grant := range TelegramBridgeDelegableGrants {
		if grant == wildcardGrant {
			continue
		}
		if slices.Contains(recorded, grant) {
			derived = append(derived, grant)
		}
	}
	return derived
}
