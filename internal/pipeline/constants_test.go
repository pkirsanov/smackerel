package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/auth"
)

// SCN-002-045: Source ID constants accessible without importing processor.
func TestSCN002045_SourceIDConstants_Accessible(t *testing.T) {
	// Verify all source ID constants have expected string values.
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"SourceCapture", SourceCapture, "capture"},
		{"SourceTelegram", SourceTelegram, "telegram"},
		{"SourceBrowser", SourceBrowser, "browser"},
		{"SourceBrowserHistory", SourceBrowserHistory, "browser-history"},
		{"SourceRSS", SourceRSS, "rss"},
		{"SourceBookmarks", SourceBookmarks, "bookmarks"},
		{"SourceGoogleKeep", SourceGoogleKeep, "google-keep"},
		{"SourceGoogleMaps", SourceGoogleMaps, "google-maps-timeline"},
		{"SourceHospitable", SourceHospitable, "hospitable"},
		{"SourceGmail", SourceGmail, "gmail"},
		{"SourceGoogleCalendar", SourceGoogleCalendar, "google-calendar"},
		{"SourceYouTube", SourceYouTube, "youtube"},
		{"SourceDiscord", SourceDiscord, "discord"},
		{"SourceTwitter", SourceTwitter, "twitter"},
		{"SourceWeather", SourceWeather, "weather"},
		{"SourceGovAlerts", SourceGovAlerts, "gov-alerts"},
		{"SourceFinancialMarkets", SourceFinancialMarkets, "financial-markets"},
		{"SourceGuestHost", SourceGuestHost, "guesthost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.constant)
			}
		})
	}
}

// SCN-002-046: Processing status constants available as typed values.
func TestSCN002046_ProcessingStatusType(t *testing.T) {
	// Verify typed constants have correct string values.
	if string(StatusPending) != "pending" {
		t.Errorf("StatusPending = %q, want %q", string(StatusPending), "pending")
	}
	if string(StatusProcessed) != "processed" {
		t.Errorf("StatusProcessed = %q, want %q", string(StatusProcessed), "processed")
	}
	if string(StatusFailed) != "failed" {
		t.Errorf("StatusFailed = %q, want %q", string(StatusFailed), "failed")
	}
}

// Verify the type system distinguishes ProcessingStatus from plain string.
func TestProcessingStatusType_NotPlainString(t *testing.T) {
	var ps ProcessingStatus = StatusPending
	// Verify it can be used as a string via conversion
	s := string(ps)
	if s != "pending" {
		t.Errorf("string(ps) = %q, want %q", s, "pending")
	}
}

// Sentinel errors: verify errors.Is works through fmt.Errorf wrapping.
func TestSentinelErrors_ExtractionFailed_Unwrappable(t *testing.T) {
	inner := fmt.Errorf("DNS resolution failed for example.com")
	wrapped := fmt.Errorf("%w: %w", ErrExtractionFailed, inner)

	if !errors.Is(wrapped, ErrExtractionFailed) {
		t.Error("errors.Is(wrapped, ErrExtractionFailed) should be true")
	}
	if !errors.Is(wrapped, inner) {
		t.Error("errors.Is(wrapped, inner) should be true — original cause must be accessible")
	}
}

func TestSentinelErrors_NATSPublish_Unwrappable(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	wrapped := fmt.Errorf("%w: %w", ErrNATSPublish, inner)

	if !errors.Is(wrapped, ErrNATSPublish) {
		t.Error("errors.Is(wrapped, ErrNATSPublish) should be true")
	}
	if !errors.Is(wrapped, inner) {
		t.Error("errors.Is(wrapped, inner) should be true — original cause must be accessible")
	}
}

// ctxCapturingAgentRunner records the context FireScenario actually built. That
// context is the only place the injected principal is observable, and reading it
// here avoids adding production code purely to expose it.
type ctxCapturingAgentRunner struct {
	gotCtx context.Context
	gotEnv agent.IntentEnvelope
}

func (r *ctxCapturingAgentRunner) Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	r.gotCtx = ctx
	r.gotEnv = env
	return &agent.InvocationResult{Outcome: agent.OutcomeOK}, &agent.RoutingDecision{}
}

func (r *ctxCapturingAgentRunner) KnownIntents() []string { return nil }

// BUG-061-012 R3.3 / SCN-07 — pipeline.FireScenario invokes as an explicit
// system principal holding no grants.
//
// Until this test existed the injection was unasserted: deleting
// auth.WithSession from agent_bridge.go left the suite green, because an
// invocation with no session also happens to fail closed today. That equivalence
// is the whole risk — the day a default session appears upstream, an unasserted
// call site silently inherits whatever it grants.
//
// Scopes is checked non-nil AND empty deliberately. nil is Session.Scopes'
// "legacy non-scoped session" sentinel; an empty slice is the opposite claim.
func TestPipelineFireScenario_InvokesAsSystemPrincipalWithNoGrants(t *testing.T) {
	runner := &ctxCapturingAgentRunner{}

	FireScenario(context.Background(), runner, "any_scenario", nil)

	if runner.gotCtx == nil {
		t.Fatal("FireScenario never invoked the runner")
	}
	sess, ok := auth.SessionFromContext(runner.gotCtx)
	if !ok {
		t.Fatal(`invoked context carries no session; pipeline.FireScenario must inject auth.SystemSession("pipeline")`)
	}
	if sess.Source != auth.SessionSourceSystem {
		t.Errorf("session Source = %q, want %q", sess.Source, auth.SessionSourceSystem)
	}
	if sess.UserID != "system:pipeline" {
		t.Errorf("session UserID = %q, want %q", sess.UserID, "system:pipeline")
	}
	if sess.Scopes == nil {
		t.Error("session Scopes is nil (the legacy non-scoped sentinel); the system principal must declare an explicitly empty grant set")
	}
	if len(sess.Scopes) != 0 {
		t.Errorf("session Scopes = %v, want empty; a pipeline stage holds no corpus grant", sess.Scopes)
	}
}
