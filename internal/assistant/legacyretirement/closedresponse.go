// closedresponse.go — spec 075 SCOPE-5 canonical unknown-command
// response renderer and retired-handler invocation guard.
//
// SCN-075-A07: after the operator flips window_state to "closed",
// the policy returns RetirementDecision{ServeNL=false, ...}; this
// file is the renderer transport adapters call to obtain the
// canonical response body (loaded from
// legacy_retirement.post_window_unknown_response_copy via the
// configCatalog) plus the structural guard that increments the
// RetiredHandlerInvocationCounter if a regression bypasses the
// closed-state branch and reaches a retired handler.
package legacyretirement

import (
	"errors"
	"fmt"
	"strings"
)

// ClosedResponse is the structured payload transport adapters render
// after a closed-window retired-command turn. Mirrors design.md §
// "API/Contracts" Closed window block.
type ClosedResponse struct {
	Status        string `json:"status"`
	ErrorCause    string `json:"error_cause"`
	Body          string `json:"body"`
	FacadeInvoked bool   `json:"facade_invoked"`
}

// ErrDecisionNotClosed is returned by ClosedResponseFor when the
// caller asks for a closed response for a decision whose effective
// state is not WindowClosed. It is a programmer error, not a user
// error.
var ErrDecisionNotClosed = errors.New("legacyretirement: ClosedResponseFor: decision EffectiveState is not closed")

// ClosedResponseFor returns the canonical unknown-command response
// for a RetirementDecision whose EffectiveState is WindowClosed.
// The body is the SST-loaded copy attached to the catalog entry
// (configCatalog stores PostWindowUnknownResponseCopy[token] in
// RetiredCommand.ReplacementExample). Empty body → fail-loud, since
// the catalog constructor already rejects empty copy values.
func ClosedResponseFor(decision RetirementDecision) (ClosedResponse, error) {
	if decision.EffectiveState != WindowClosed {
		return ClosedResponse{}, fmt.Errorf("%w: state=%q", ErrDecisionNotClosed, decision.EffectiveState)
	}
	body := strings.TrimSpace(decision.Command.ReplacementExample)
	if body == "" {
		return ClosedResponse{}, fmt.Errorf("legacyretirement: ClosedResponseFor: empty unknown-command body for %q (SST coverage gap)", decision.Command.Command)
	}
	return ClosedResponse{
		Status:        "unavailable",
		ErrorCause:    "retired_command_closed",
		Body:          body,
		FacadeInvoked: false,
	}, nil
}

// RecordRetiredHandlerInvocation is the structural safety hook the
// (now-deleted) retired handlers MUST call if any code path ever
// reaches them after window_state=closed. It increments the
// RetiredHandlerInvocationCounter metric so the observation report
// can detect the regression and refuse to advance the deletion
// gate. The counter is labeled by command only; raw user ids and
// raw turn text are NEVER passed in.
func RecordRetiredHandlerInvocation(command string) {
	c := strings.TrimSpace(command)
	if c == "" {
		// Defensive: still record under a sentinel so a
		// regression that drops the command label is visible.
		c = "_unknown"
	}
	RetiredHandlerInvocationCounter.WithLabelValues(c).Inc()
}
