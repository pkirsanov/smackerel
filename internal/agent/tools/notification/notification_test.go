// Unit tests for the spec 061 SCOPE-03 notification_propose and
// notification_execute tool handlers.

package notification

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/auth"
)

// notifyCtx carries the authenticated caller the way production does
// (BUG-061-012): on the context, put there by the surface. These tests used to
// pass "user_id" as a tool argument — a value the model writes, which then
// reached the persisted envelope and the scheduler intact.
func notifyCtx(userID string) context.Context {
	return auth.WithSession(context.Background(), auth.Session{
		UserID: userID,
		Source: auth.SessionSourcePerUserToken,
		Scopes: []string{},
	})
}

type memConfirmStore struct {
	mu      sync.Mutex
	entries map[string]string
}

func newMemConfirmStore() *memConfirmStore {
	return &memConfirmStore{entries: map[string]string{}}
}
func (m *memConfirmStore) Put(_ context.Context, ref, payload string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[ref] = payload
	return nil
}
func (m *memConfirmStore) Get(_ context.Context, ref string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.entries[ref]
	return p, ok, nil
}

type fakeScheduler struct {
	lastWhen       time.Time
	lastPayload    string
	lastSource     string
	lastOriginator string
}

func (f *fakeScheduler) Schedule(_ context.Context, when time.Time, payload, source, originator string) (string, error) {
	f.lastWhen, f.lastPayload, f.lastSource, f.lastOriginator = when, payload, source, originator
	return "job-1", nil
}

func wireFakes(t *testing.T) (*memConfirmStore, *fakeScheduler) {
	t.Helper()
	store := newMemConfirmStore()
	sched := &fakeScheduler{}
	SetServices(&Services{Confirm: store, Scheduler: sched, ConfirmTimeout: 5 * time.Minute})
	t.Cleanup(ResetForTest)
	return store, sched
}

func TestNotification_BothToolsRegistered(t *testing.T) {
	for _, name := range []string{ToolPropose, ToolExecute} {
		if !agent.Has(name) {
			t.Fatalf("tool %q not registered", name)
		}
	}
	prop, _ := agent.ByName(ToolPropose)
	exec, _ := agent.ByName(ToolExecute)
	if prop.SideEffectClass != agent.SideEffectRead {
		t.Fatalf("propose side_effect_class = %q, want read", prop.SideEffectClass)
	}
	if exec.SideEffectClass != agent.SideEffectWrite {
		t.Fatalf("execute side_effect_class = %q, want write", exec.SideEffectClass)
	}
}

func TestPropose_Happy_StagesPayloadAndIssuesRef(t *testing.T) {
	store, _ := wireFakes(t)

	raw, err := handleNotificationPropose(notifyCtx("u1"),
		[]byte(`{"what":"take out trash","when_relative":"2h"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	var out proposeOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Phase != "proposed" {
		t.Fatalf("phase = %q, want proposed", out.Phase)
	}
	if out.ConfirmRef == "" {
		t.Fatalf("missing confirm_ref")
	}
	if _, ok, _ := store.Get(context.Background(), out.ConfirmRef); !ok {
		t.Fatalf("payload not staged under ref %q", out.ConfirmRef)
	}
}

func TestPropose_MissingWhen_SlotMissing(t *testing.T) {
	wireFakes(t)
	raw, err := handleNotificationPropose(notifyCtx("u1"),
		[]byte(`{"what":"do it"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	var out proposeOutput
	_ = json.Unmarshal(raw, &out)
	if out.Phase != "slot_missing" {
		t.Fatalf("phase = %q, want slot_missing", out.Phase)
	}
	if len(out.SlotMissingOptions) == 0 {
		t.Fatalf("expected slot_missing_options, got none")
	}
	if out.ConfirmRef != "" {
		t.Fatalf("confirm_ref must NOT be issued on slot_missing, got %q", out.ConfirmRef)
	}
}

func TestPropose_UnidentifiedCaller_Errors(t *testing.T) {
	// BUG-061-012 SCN-02. Successor to TestPropose_MissingUserID_Errors:
	// identity is no longer an argument, so the equivalent refusal is "no
	// principal on the context". This is the only assertion that propose
	// refuses an unnamed caller rather than staging a payload for nobody, so
	// it is converted rather than deleted.
	wireFakes(t)

	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"no principal", context.Background(), "notification_propose_no_principal"},
		{"principal identifies nobody", notifyCtx("  "), "notification_propose_principal_without_user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handleNotificationPropose(tc.ctx, []byte(`{"what":"x","when_relative":"1h"}`))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestPropose_IgnoresModelSuppliedUserID(t *testing.T) {
	// Adversarial: a model that still writes "user_id" must not be able to
	// pick whose reminder is staged. The session user is what lands in the
	// persisted envelope, which is what notification_execute schedules against.
	store, _ := wireFakes(t)

	raw, err := handleNotificationPropose(notifyCtx("caller"),
		[]byte(`{"user_id":"victim","what":"x","when_relative":"1h"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	var out proposeOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	staged, ok, _ := store.Get(context.Background(), out.ConfirmRef)
	if !ok {
		t.Fatalf("payload not staged under ref %q", out.ConfirmRef)
	}
	var env payloadEnvelope
	if err := json.Unmarshal([]byte(staged), &env); err != nil {
		t.Fatalf("unmarshal staged envelope: %v", err)
	}
	if env.UserID != "caller" {
		t.Fatalf("staged envelope user_id = %q, want the session user \"caller\" — a model argument steered the recipient", env.UserID)
	}
}

func TestExecute_RoundTrip_CallsScheduler(t *testing.T) {
	store, sched := wireFakes(t)

	rawP, err := handleNotificationPropose(notifyCtx("u42"),
		[]byte(`{"what":"call mom","when_relative":"1h","transport":"telegram"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	var pout proposeOutput
	_ = json.Unmarshal(rawP, &pout)

	rawE, err := handleNotificationExecute(notifyCtx("u42"),
		[]byte(`{"confirm_ref":"`+pout.ConfirmRef+`"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var eout executeOutput
	_ = json.Unmarshal(rawE, &eout)
	if eout.Phase != "confirmed" || eout.ScheduledJobID != "job-1" {
		t.Fatalf("execute out = %+v", eout)
	}
	if sched.lastSource != "assistant" {
		t.Fatalf("source = %q, want assistant", sched.lastSource)
	}
	if sched.lastOriginator != "user:u42" {
		t.Fatalf("originator = %q, want user:u42", sched.lastOriginator)
	}
	if got, _, _ := store.Get(context.Background(), pout.ConfirmRef); got != sched.lastPayload {
		t.Fatalf("payload mismatch:\n  staged:    %s\n  scheduled: %s", got, sched.lastPayload)
	}
}

func TestExecute_UnknownConfirmRef_Errors(t *testing.T) {
	wireFakes(t)
	_, err := handleNotificationExecute(notifyCtx("u1"),
		[]byte(`{"confirm_ref":"deadbeef"}`))
	if err == nil || !strings.Contains(err.Error(), "confirm_ref_unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestExecute_PrincipalMustMatchEnvelope(t *testing.T) {
	// SEC-02. notification_propose returns confirm_ref TO THE MODEL, so the ref
	// is a bearer capability naming a user. execute used to schedule as
	// "user:"+envelope.UserID with no principal check, so naming another user's
	// ref confirmed their pending action.
	stage := func(t *testing.T, owner string) (*fakeScheduler, string) {
		t.Helper()
		_, sched := wireFakes(t)
		raw, err := handleNotificationPropose(notifyCtx(owner),
			[]byte(`{"what":"call mom","when_relative":"1h"}`))
		if err != nil {
			t.Fatalf("propose: %v", err)
		}
		var out proposeOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal propose: %v", err)
		}
		if out.ConfirmRef == "" {
			t.Fatalf("propose issued no confirm_ref")
		}
		return sched, out.ConfirmRef
	}

	t.Run("no principal", func(t *testing.T) {
		sched, ref := stage(t, "owner")
		_, err := handleNotificationExecute(context.Background(),
			[]byte(`{"confirm_ref":"`+ref+`"}`))
		if err == nil || !strings.Contains(err.Error(), "notification_execute_no_principal") {
			t.Fatalf("err = %v, want notification_execute_no_principal", err)
		}
		if sched.lastOriginator != "" {
			t.Fatalf("scheduler ran for an unauthenticated caller: originator = %q", sched.lastOriginator)
		}
	})

	t.Run("foreign principal", func(t *testing.T) {
		// Adversarial: with the mismatch check removed this call succeeds and
		// schedules "user:owner" at the request of "attacker", so BOTH the
		// error assertion and the scheduler assertion flip to failing.
		sched, ref := stage(t, "owner")
		_, err := handleNotificationExecute(notifyCtx("attacker"),
			[]byte(`{"confirm_ref":"`+ref+`"}`))
		if err == nil || !strings.Contains(err.Error(), "notification_execute_principal_mismatch") {
			t.Fatalf("err = %v, want notification_execute_principal_mismatch", err)
		}
		if sched.lastOriginator != "" {
			t.Fatalf("a foreign principal confirmed the owner's pending action: originator = %q", sched.lastOriginator)
		}
	})

	t.Run("owning principal proceeds", func(t *testing.T) {
		sched, ref := stage(t, "owner")
		raw, err := handleNotificationExecute(notifyCtx("owner"),
			[]byte(`{"confirm_ref":"`+ref+`"}`))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		var out executeOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal execute: %v", err)
		}
		if out.Phase != "confirmed" {
			t.Fatalf("phase = %q, want confirmed", out.Phase)
		}
		if sched.lastOriginator != "user:owner" {
			t.Fatalf("originator = %q, want user:owner", sched.lastOriginator)
		}
	})
}

func TestNotification_NotConfigured_FailsLoud(t *testing.T) {
	ResetForTest()
	if _, err := handleNotificationPropose(notifyCtx("u"), []byte(`{"what":"x","when_relative":"1h"}`)); err == nil || !strings.Contains(err.Error(), "not_configured") {
		t.Fatalf("propose err = %v", err)
	}
	if _, err := handleNotificationExecute(notifyCtx("u"), []byte(`{"confirm_ref":"abc"}`)); err == nil || !strings.Contains(err.Error(), "not_configured") {
		t.Fatalf("execute err = %v", err)
	}
}
