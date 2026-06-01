// sststate.go — spec 075 SCOPE-2 WindowStateResolver wiring SST
// window_state to runtime pause rows.
//
// Precedence (design.md §"Effective Window State"):
//
//	SST "closed"       → WindowClosed (highest priority)
//	active pause row   → WindowPaused (SST "open" combined with a
//	                     row in assistant_legacy_retirement_state
//	                     whose effective_state="paused")
//	otherwise          → WindowOpen
//
// Scope 2 wires the resolver into the policy and the in-process
// catalog/ledger so the per-user notice path can run end-to-end in
// unit tests. The runtime pause row source is abstracted as
// PauseStateReader so Scope 4 can swap in the threshold-driven
// writer without touching this file.
package legacyretirement

import (
	"context"
	"fmt"
	"strings"
)

const (
	stateReasonSSTClosed     StateReason = "sst_closed"
	stateReasonRuntimePaused StateReason = "runtime_pause_active"
	stateReasonOpenNoPause   StateReason = "sst_open_no_runtime_pause"
	stateReasonUnknownSST    StateReason = "sst_unknown_value"
)

// PauseStateReader exposes the durable runtime pause-state row
// (assistant_legacy_retirement_state). A return of (active=false,
// nil) means "no active pause row"; any error is propagated.
type PauseStateReader interface {
	IsPaused(ctx context.Context, windowID string) (active bool, err error)
}

// staticPauseReader is the trivial PauseStateReader used until
// Scope 4 wires the SQL implementation. Always returns "not paused".
type staticPauseReader struct{ paused bool }

// NewStaticPauseStateReader returns a PauseStateReader that always
// reports the same boolean. Tests use this to exercise the
// open→paused branch without a real DB row.
func NewStaticPauseStateReader(paused bool) PauseStateReader {
	return &staticPauseReader{paused: paused}
}

func (s *staticPauseReader) IsPaused(context.Context, string) (bool, error) {
	return s.paused, nil
}

// SSTStateConfig is the minimal SST projection the resolver needs.
type SSTStateConfig struct {
	WindowID    string
	WindowState string
}

type sstStateResolver struct {
	cfg   SSTStateConfig
	pause PauseStateReader
}

// NewWindowStateResolver returns a WindowStateResolver wired to the
// SST window_state and the runtime pause-state reader.
func NewWindowStateResolver(cfg SSTStateConfig, pause PauseStateReader) (WindowStateResolver, error) {
	if strings.TrimSpace(cfg.WindowID) == "" {
		return nil, fmt.Errorf("legacyretirement: SSTStateConfig.WindowID empty; refuse to construct resolver")
	}
	if pause == nil {
		return nil, fmt.Errorf("legacyretirement: PauseStateReader is nil; refuse to construct resolver (use NewStaticPauseStateReader(false) for the default)")
	}
	switch cfg.WindowState {
	case "open", "closed":
	default:
		return nil, fmt.Errorf("legacyretirement: SSTStateConfig.WindowState=%q invalid; must be \"open\" or \"closed\"", cfg.WindowState)
	}
	return &sstStateResolver{cfg: cfg, pause: pause}, nil
}

func (r *sstStateResolver) Resolve(ctx context.Context) (WindowState, StateReason, error) {
	if r.cfg.WindowState == "closed" {
		return WindowClosed, stateReasonSSTClosed, nil
	}
	if r.cfg.WindowState != "open" {
		return "", stateReasonUnknownSST, fmt.Errorf("legacyretirement: SST window_state=%q is neither open nor closed", r.cfg.WindowState)
	}
	paused, err := r.pause.IsPaused(ctx, r.cfg.WindowID)
	if err != nil {
		return "", "", fmt.Errorf("legacyretirement: pause-state read for window %q: %w", r.cfg.WindowID, err)
	}
	if paused {
		return WindowPaused, stateReasonRuntimePaused, nil
	}
	return WindowOpen, stateReasonOpenNoPause, nil
}
