// state.go — spec 075 SCOPE-1 window-state resolver seam.
package legacyretirement

import "context"

// WindowStateResolver combines the SST window_state with any
// durable runtime pause row (assistant_legacy_retirement_state) and
// returns the effective state the policy must act on. Precedence
// (design.md): SST "closed" wins; otherwise an active pause row
// makes the window paused; otherwise the window is open.
type WindowStateResolver interface {
	Resolve(ctx context.Context) (WindowState, StateReason, error)
}
