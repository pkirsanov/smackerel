package contracts

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// fakeAdapter is the package-internal stub used to prove TransportAdapter
// is satisfiable AND to exercise every method's signature. The fake is
// deliberately minimal — it does NOT implement any business logic.
type fakeAdapter struct {
	name           string
	started        atomic.Bool
	stopped        atomic.Bool
	translateCalls atomic.Int64
	renderCalls    atomic.Int64
	identityCalls  atomic.Int64
}

func (f *fakeAdapter) Name() string { return f.name }

func (f *fakeAdapter) Translate(_ context.Context, _ TransportPayload) (AssistantMessage, error) {
	f.translateCalls.Add(1)
	return AssistantMessage{UserID: "u-fake", Transport: f.name, Text: "hello", Kind: KindText}, nil
}

func (f *fakeAdapter) Render(_ context.Context, identity TransportIdentity, resp AssistantResponse) error {
	f.renderCalls.Add(1)
	if identity.Transport != f.name {
		return errors.New("identity transport mismatch")
	}
	if resp.Status == "" {
		return errors.New("Render called with empty Status")
	}
	return nil
}

func (f *fakeAdapter) Identity(_ context.Context, _ TransportPayload) (TransportIdentity, error) {
	f.identityCalls.Add(1)
	return TransportIdentity{UserID: "u-fake", Transport: f.name}, nil
}

func (f *fakeAdapter) Start(_ context.Context, _ Assistant) error {
	f.started.Store(true)
	return nil
}

func (f *fakeAdapter) Stop(_ context.Context) error {
	f.stopped.Store(true)
	return nil
}

// Compile-time assertion.
var _ TransportAdapter = (*fakeAdapter)(nil)

// TestTransportAdapter_FakeImplementsEveryMethod — exercises every
// method on the package-internal fake. Adversarial: if any
// TransportAdapter method gains or loses a parameter without the
// fake being updated, this file fails to compile, which is the
// strongest possible regression signal.
func TestTransportAdapter_FakeImplementsEveryMethod(t *testing.T) {
	ctx := context.Background()
	f := &fakeAdapter{name: "telegram"}

	if got, want := f.Name(), "telegram"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}

	if got := f.Name(); got != "telegram" && got != "whatsapp" && got != "web" && got != "mobile" {
		t.Errorf("Name() returned non-closed-vocab value %q", got)
	}

	msg, err := f.Translate(ctx, struct{}{})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if msg.Transport != "telegram" {
		t.Errorf("Translate produced wrong transport: %q", msg.Transport)
	}

	id, err := f.Identity(ctx, struct{}{})
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if id.UserID == "" {
		t.Error("Identity returned empty UserID")
	}

	if err := f.Render(ctx, id, AssistantResponse{Status: StatusThinking}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if err := f.Start(ctx, _staticFake{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !f.started.Load() {
		t.Error("Start did not flip started flag")
	}

	if err := f.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !f.stopped.Load() {
		t.Error("Stop did not flip stopped flag")
	}

	// Adversarial: assert the fake observed exactly the calls we
	// drove — proves the test exercised every method (not just
	// compiled them).
	if got := f.translateCalls.Load(); got != 1 {
		t.Errorf("Translate call count = %d, want 1", got)
	}
	if got := f.renderCalls.Load(); got != 1 {
		t.Errorf("Render call count = %d, want 1", got)
	}
	if got := f.identityCalls.Load(); got != 1 {
		t.Errorf("Identity call count = %d, want 1", got)
	}
}
