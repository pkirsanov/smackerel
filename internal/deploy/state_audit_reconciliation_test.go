// Package deploy — BUG-042-006 audit-history reconciliation contract.
//
// This file holds the persistent static-file invariant test that locks
// `specs/042-tailnet-edge-bind-pattern/state.json` against drift between
// its append-only audit history and the current Gate G028 NO-DEFAULTS /
// fail-loud SST policy.
//
// Background (per [`spec.md`](../../specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/spec.md)):
//
//   - Spec 042's original implementation (closed 2026-05-09) chose
//     `${HOST_BIND_ADDRESS:-127.0.0.1}:` as the compose host-bind
//     substitution form, and 9 narrative fields in spec-042 state.json
//     praised that decision as "preserving loopback default".
//   - BUG-029-003 (closed at HEAD `eec1437c` on 2026-05-14) reversed
//     that decision per Gate G028 and converted the form to
//     `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`.
//     The pre-Gate-G028 `${VAR:-default}` form is now FORBIDDEN, codified
//     in `.github/instructions/smackerel-no-defaults.instructions.md`
//     and `.github/copilot-instructions.md`.
//   - The audit history is append-only — historical substance MUST NOT
//     be deleted or rewritten. The fix prepends a SUPERSEDED marker to
//     each of the 9 stale fields and appends a single authoritative
//     reconciliation entry to `execution.completedPhaseClaims`.
//
// Contract (the validator in this file enforces it):
//
//   - A reconciliation entry exists at the tail of
//     `execution.completedPhaseClaims` with `phase ==
//     "spec_042_audit_reconciliation_post_BUG-029-003"`, `agent ==
//     "bubbles.implement"`, and a `notes` field containing the 12
//     required citation substrings (BUG-029-003, eec1437c, Gate G028,
//     the binding instruction file path, the binding workspace rule,
//     both substitution forms, the 4 `completedPhaseClaims[N].notes`
//     paths, the 5 `pendingTransitionRequests[*]@lineNNN` references,
//     and the compliance-test pointer
//     `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialDefaultFallback`).
//
//   - Each of the 9 affected fields (4 `completedPhaseClaims` notes +
//     5 `transitionRequests` reason/closeReason) begins with the
//     FROZEN SUPERSEDED marker literal.
//
// 3 sub-tests (A/B/C) — A is positive (live file), B and C are
// adversarial mutations on in-memory deep-copies that prove the
// validator is non-tautological per the bubbles-test-integrity skill.
package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// markerLiteral is the FROZEN SUPERSEDED marker prefix that BUG-042-006
// applies to every spec-042 audit narrative field that praised the
// now-FORBIDDEN `${HOST_BIND_ADDRESS:-127.0.0.1}:` form. Identical
// across all 9 affected fields. A single ASCII space follows the
// marker, then the original (untouched) substance.
const markerLiteral = "[SUPERSEDED by BUG-029-003 (HEAD eec1437c) — fail-loud form ${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}: now binding per Gate G028; the :-127.0.0.1 form below is RETAINED FOR AUDIT HISTORY ONLY and is now FORBIDDEN. See .github/instructions/smackerel-no-defaults.instructions.md]"

const (
	// reconciliationPhase is the FROZEN phase identifier of the
	// reconciliation entry the implement phase appends to
	// execution.completedPhaseClaims.
	reconciliationPhase = "spec_042_audit_reconciliation_post_BUG-029-003"

	// bugID is included in every validator error message so adversarial
	// sub-tests can pattern-match the failure mode per FROZEN DoD item C.
	bugID = "BUG-042-006"
)

// requiredCitationSubstrings names the substrings the FROZEN
// reconciliation-entry notes (per design.md DD-9 + Part 1 frozen JSON
// shape) MUST contain verbatim. A regression of the policy-reversal
// narrative would surface here.
var requiredCitationSubstrings = []string{
	"BUG-029-003",
	"eec1437c",
	"Gate G028",
	".github/instructions/smackerel-no-defaults.instructions.md",
	".github/copilot-instructions.md",
	"${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}",
	"${HOST_BIND_ADDRESS:-127.0.0.1}",
	"completedPhaseClaims[3].notes",
	"completedPhaseClaims[4].notes",
	"completedPhaseClaims[5].notes",
	"completedPhaseClaims[6].notes",
	"line212",
	"line222",
	"line226",
	"line232",
	"line234",
	"TestComposeContract_AdversarialDefaultFallback",
}

// affectedFieldDescriptor identifies one of the 9 stale audit fields by
// a stable navigation path through the parsed state.json. The validator
// asserts each field begins with markerLiteral; sub-test C strips the
// marker from one field at a time and asserts the validator surfaces
// the path AND bugID in its error.
type affectedFieldDescriptor struct {
	// Path is the human-readable identifier emitted in error messages.
	Path string
	// Get returns the current string value of the field from a parsed
	// state document. Returns ("", false) if the field cannot be located.
	Get func(state map[string]interface{}) (string, bool)
	// Set replaces the field's value in a parsed state document. Used
	// by sub-test C to strip the marker for adversarial mutation.
	Set func(state map[string]interface{}, value string)
}

// completedPhaseClaimNotesAccessors returns Get/Set closures for the
// `notes` field of the i-th entry in execution.completedPhaseClaims.
func completedPhaseClaimNotesAccessors(idx int) (func(map[string]interface{}) (string, bool), func(map[string]interface{}, string)) {
	get := func(state map[string]interface{}) (string, bool) {
		exec, ok := state["execution"].(map[string]interface{})
		if !ok {
			return "", false
		}
		claims, ok := exec["completedPhaseClaims"].([]interface{})
		if !ok || idx >= len(claims) {
			return "", false
		}
		entry, ok := claims[idx].(map[string]interface{})
		if !ok {
			return "", false
		}
		s, ok := entry["notes"].(string)
		return s, ok
	}
	set := func(state map[string]interface{}, value string) {
		exec := state["execution"].(map[string]interface{})
		claims := exec["completedPhaseClaims"].([]interface{})
		entry := claims[idx].(map[string]interface{})
		entry["notes"] = value
	}
	return get, set
}

// transitionRequestFieldAccessors returns Get/Set closures for the
// (requestedBy, targetAgent)-identified transition request's named
// field. The lookup is by content (requestedBy + targetAgent pair),
// NOT by array index — so insertions or reorderings in
// transitionRequests do not break the contract test.
func transitionRequestFieldAccessors(requestedBy, targetAgent, fieldName string) (func(map[string]interface{}) (string, bool), func(map[string]interface{}, string)) {
	find := func(state map[string]interface{}) (map[string]interface{}, bool) {
		reqs, ok := state["transitionRequests"].([]interface{})
		if !ok {
			return nil, false
		}
		for _, raw := range reqs {
			req, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if req["requestedBy"] == requestedBy && req["targetAgent"] == targetAgent {
				return req, true
			}
		}
		return nil, false
	}
	get := func(state map[string]interface{}) (string, bool) {
		req, ok := find(state)
		if !ok {
			return "", false
		}
		s, ok := req[fieldName].(string)
		return s, ok
	}
	set := func(state map[string]interface{}, value string) {
		req, ok := find(state)
		if !ok {
			return
		}
		req[fieldName] = value
	}
	return get, set
}

// affectedFields enumerates the 9 stale audit fields per BUG-042-006's
// design DD-9 + scopes.md FROZEN DoD item A. Sub-test A asserts each
// field begins with markerLiteral; sub-test C is table-driven over this
// list and strips the marker from one field at a time.
//
// The first 4 entries are the completedPhaseClaims notes for the 4
// downstream specialists (regression, simplify, stabilize, security)
// in the bugfix-fastlane chain. The last 5 are the transition-request
// reason/closeReason fields chained through simplify → stabilize →
// security. Each is identified by a stable lookup (array index for
// completedPhaseClaims; requestedBy + targetAgent pair for
// transitionRequests) so structural reorderings do not break the test.
func affectedFields() []affectedFieldDescriptor {
	out := []affectedFieldDescriptor{}

	// 4 completedPhaseClaims notes (regression / simplify / stabilize / security)
	for _, idx := range []int{3, 4, 5, 6} {
		i := idx
		g, s := completedPhaseClaimNotesAccessors(i)
		out = append(out, affectedFieldDescriptor{
			Path: fmt.Sprintf("execution.completedPhaseClaims[%d].notes", i),
			Get:  g,
			Set:  s,
		})
	}

	// 5 transition-request reason/closeReason fields (simplify chain)
	transitionFields := []struct {
		path        string
		requestedBy string
		targetAgent string
		fieldName   string
	}{
		{
			path:        "transitionRequests[regression->simplify].reason",
			requestedBy: "bubbles.regression",
			targetAgent: "bubbles.simplify",
			fieldName:   "reason",
		},
		{
			path:        "transitionRequests[simplify->stabilize].reason",
			requestedBy: "bubbles.simplify",
			targetAgent: "bubbles.stabilize",
			fieldName:   "reason",
		},
		{
			path:        "transitionRequests[simplify->stabilize].closeReason",
			requestedBy: "bubbles.simplify",
			targetAgent: "bubbles.stabilize",
			fieldName:   "closeReason",
		},
		{
			path:        "transitionRequests[stabilize->security].reason",
			requestedBy: "bubbles.stabilize",
			targetAgent: "bubbles.security",
			fieldName:   "reason",
		},
		{
			path:        "transitionRequests[stabilize->security].closeReason",
			requestedBy: "bubbles.stabilize",
			targetAgent: "bubbles.security",
			fieldName:   "closeReason",
		},
	}
	for _, tf := range transitionFields {
		g, s := transitionRequestFieldAccessors(tf.requestedBy, tf.targetAgent, tf.fieldName)
		out = append(out, affectedFieldDescriptor{
			Path: tf.path,
			Get:  g,
			Set:  s,
		})
	}

	return out
}

// validateAuditReconciliation parses state-json bytes and returns a
// non-nil error if either of the following invariants is violated:
//
//   - The reconciliation entry (phase == reconciliationPhase, agent ==
//     "bubbles.implement") is missing from
//     execution.completedPhaseClaims, OR any of the required citation
//     substrings is absent from its notes field. The error message
//     names reconciliationPhase AND bugID so sub-test B can
//     pattern-match.
//
//   - Any of the 9 affected fields (per affectedFields) does NOT begin
//     with markerLiteral. The error message names the affected field's
//     path AND bugID so sub-test C can pattern-match per row.
//
// All three sub-tests use the SAME validator function — a regression
// in the validator surfaces in all sub-tests simultaneously. This is
// the non-tautology proof per the bubbles-test-integrity skill.
func validateAuditReconciliation(data []byte) error {
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("validateAuditReconciliation (%s): json.Unmarshal failed: %w", bugID, err)
	}

	exec, ok := state["execution"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("validateAuditReconciliation (%s): execution block missing or not an object — reconciliation entry %q cannot be located", bugID, reconciliationPhase)
	}
	claimsRaw, ok := exec["completedPhaseClaims"].([]interface{})
	if !ok {
		return fmt.Errorf("validateAuditReconciliation (%s): execution.completedPhaseClaims missing or not an array — reconciliation entry %q cannot be located", bugID, reconciliationPhase)
	}

	var reconciliation map[string]interface{}
	for _, raw := range claimsRaw {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if entry["phase"] == reconciliationPhase {
			reconciliation = entry
			break
		}
	}
	if reconciliation == nil {
		return fmt.Errorf("validateAuditReconciliation (%s): reconciliation entry with phase %q is missing from execution.completedPhaseClaims; bugfix is required to append the FROZEN entry per design.md Part 1", bugID, reconciliationPhase)
	}

	if reconciliation["agent"] != "bubbles.implement" {
		return fmt.Errorf("validateAuditReconciliation (%s): reconciliation entry %q has agent=%v, expected %q", bugID, reconciliationPhase, reconciliation["agent"], "bubbles.implement")
	}

	notes, ok := reconciliation["notes"].(string)
	if !ok {
		return fmt.Errorf("validateAuditReconciliation (%s): reconciliation entry %q has no notes field (or notes is not a string)", bugID, reconciliationPhase)
	}
	for _, sub := range requiredCitationSubstrings {
		if !strings.Contains(notes, sub) {
			return fmt.Errorf("validateAuditReconciliation (%s): reconciliation entry %q notes is missing required citation substring %q", bugID, reconciliationPhase, sub)
		}
	}

	for _, f := range affectedFields() {
		got, ok := f.Get(state)
		if !ok {
			return fmt.Errorf("validateAuditReconciliation (%s): affected field %q cannot be located in the parsed state document — structural drift?", bugID, f.Path)
		}
		if !strings.HasPrefix(got, markerLiteral) {
			return fmt.Errorf("validateAuditReconciliation (%s): affected field %q does NOT begin with the FROZEN SUPERSEDED marker literal — spec 042 audit history is out of sync with current Gate G028 fail-loud policy. First 80 chars of field: %q", bugID, f.Path, truncateForError(got, 80))
		}
	}

	return nil
}

// truncateForError shortens a long string to at most n runes for error
// messages. Pure helper, no side effects.
func truncateForError(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003 is the
// FROZEN parent function name for the BUG-042-006 regression contract
// (per scopes.md FROZEN DoD item C). The 3 sub-tests below (A/B/C) are
// FROZEN names. Renaming any of them invalidates the contract.
func TestSpec042_StateJsonHasReconciliationEntry_PostBUG029003(t *testing.T) {
	statePath := filepath.Join(repoRoot(t), "specs", "042-tailnet-edge-bind-pattern", "state.json")
	liveBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read live spec 042 state.json at %q: %v", statePath, err)
	}

	t.Run("A_live_file_has_reconciliation_entry_and_all_9_markers", func(t *testing.T) {
		if err := validateAuditReconciliation(liveBytes); err != nil {
			t.Fatalf("live spec 042 state.json fails reconciliation contract: %v", err)
		}
	})

	t.Run("B_adversarial_reconciliation_entry_stripped_fails_red", func(t *testing.T) {
		var state map[string]interface{}
		if err := json.Unmarshal(liveBytes, &state); err != nil {
			t.Fatalf("adversarial parse failed: %v", err)
		}
		exec := state["execution"].(map[string]interface{})
		claims := exec["completedPhaseClaims"].([]interface{})
		filtered := make([]interface{}, 0, len(claims))
		stripped := false
		for _, raw := range claims {
			entry, ok := raw.(map[string]interface{})
			if ok && entry["phase"] == reconciliationPhase {
				stripped = true
				continue
			}
			filtered = append(filtered, raw)
		}
		if !stripped {
			t.Fatalf("adversarial setup: live state had no reconciliation entry to strip — sub-test A should have surfaced this first")
		}
		exec["completedPhaseClaims"] = filtered

		mutated, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("adversarial re-serialize failed: %v", err)
		}

		err = validateAuditReconciliation(mutated)
		if err == nil {
			t.Fatalf("adversarial mutation (reconciliation entry stripped) was NOT detected by the validator — this regression test is tautological and would not catch the bug")
		}
		if !strings.Contains(err.Error(), reconciliationPhase) {
			t.Errorf("adversarial error %q does not name the missing phase identifier %q", err.Error(), reconciliationPhase)
		}
		if !strings.Contains(err.Error(), bugID) {
			t.Errorf("adversarial error %q does not name the bug ID %q", err.Error(), bugID)
		}
	})

	t.Run("C_adversarial_marker_stripped_fails_red", func(t *testing.T) {
		for _, fd := range affectedFields() {
			fd := fd
			t.Run(fd.Path, func(t *testing.T) {
				var state map[string]interface{}
				if err := json.Unmarshal(liveBytes, &state); err != nil {
					t.Fatalf("adversarial parse failed: %v", err)
				}
				original, ok := fd.Get(state)
				if !ok {
					t.Fatalf("adversarial setup: affected field %q cannot be located in the parsed state", fd.Path)
				}
				if !strings.HasPrefix(original, markerLiteral) {
					t.Fatalf("adversarial setup: live field %q does not begin with markerLiteral; cannot strip — sub-test A should have surfaced this", fd.Path)
				}
				stripped := strings.TrimPrefix(original, markerLiteral+" ")
				if stripped == original {
					t.Fatalf("adversarial setup: marker prefix not followed by ASCII space in field %q; cannot perform clean strip", fd.Path)
				}
				fd.Set(state, stripped)

				mutated, err := json.Marshal(state)
				if err != nil {
					t.Fatalf("adversarial re-serialize failed: %v", err)
				}

				err = validateAuditReconciliation(mutated)
				if err == nil {
					t.Fatalf("adversarial mutation (marker stripped from %q) was NOT detected by the validator — this regression test is tautological and would not catch the bug", fd.Path)
				}
				if !strings.Contains(err.Error(), fd.Path) {
					t.Errorf("adversarial error %q does not name the affected field path %q", err.Error(), fd.Path)
				}
				if !strings.Contains(err.Error(), bugID) {
					t.Errorf("adversarial error %q does not name the bug ID %q", err.Error(), bugID)
				}
			})
		}
	})
}
