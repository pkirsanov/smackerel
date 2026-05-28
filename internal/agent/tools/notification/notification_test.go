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
)

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

	raw, err := handleNotificationPropose(context.Background(),
		[]byte(`{"user_id":"u1","what":"take out trash","when_relative":"2h"}`))
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
	raw, err := handleNotificationPropose(context.Background(),
		[]byte(`{"user_id":"u1","what":"do it"}`))
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

func TestPropose_MissingUserID_Errors(t *testing.T) {
	wireFakes(t)
	_, err := handleNotificationPropose(context.Background(),
		[]byte(`{"user_id":"","what":"x","when_relative":"1h"}`))
	if err == nil || !strings.Contains(err.Error(), "missing_user_id") {
		t.Fatalf("err = %v, want missing_user_id", err)
	}
}

func TestExecute_RoundTrip_CallsScheduler(t *testing.T) {
	store, sched := wireFakes(t)

	rawP, err := handleNotificationPropose(context.Background(),
		[]byte(`{"user_id":"u42","what":"call mom","when_relative":"1h","transport":"telegram"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	var pout proposeOutput
	_ = json.Unmarshal(rawP, &pout)

	rawE, err := handleNotificationExecute(context.Background(),
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
	_, err := handleNotificationExecute(context.Background(),
		[]byte(`{"confirm_ref":"deadbeef"}`))
	if err == nil || !strings.Contains(err.Error(), "confirm_ref_unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotification_NotConfigured_FailsLoud(t *testing.T) {
	ResetForTest()
	if _, err := handleNotificationPropose(context.Background(), []byte(`{"user_id":"u","what":"x","when_relative":"1h"}`)); err == nil || !strings.Contains(err.Error(), "not_configured") {
		t.Fatalf("propose err = %v", err)
	}
	if _, err := handleNotificationExecute(context.Background(), []byte(`{"confirm_ref":"abc"}`)); err == nil || !strings.Contains(err.Error(), "not_configured") {
		t.Fatalf("execute err = %v", err)
	}
}
