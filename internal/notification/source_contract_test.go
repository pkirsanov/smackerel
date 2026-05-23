package notification

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSourceRegistryRegistersMultipleInstancesWithoutNtfyDependency(t *testing.T) {
	registry := NewSourceRegistry()
	forms := []SourceForm{SourceFormStream, SourceFormWebhook, SourceFormPolling, SourceFormQueue, SourceFormFileDrop, SourceFormAPIPull, SourceFormManual}
	for index, form := range forms {
		adapter := testSourceAdapter{sourceType: "fixture_source", sourceForm: form, instanceID: "fixture-instance-" + string(rune('a'+index))}
		if err := registry.Register(adapter); err != nil {
			t.Fatalf("register %s: %v", form, err)
		}
	}
	if registry.Len() != len(forms) {
		t.Fatalf("registry length = %d, want %d", registry.Len(), len(forms))
	}
	registered := registry.List()
	seenForms := map[SourceForm]bool{}
	for _, item := range registered {
		if item.Key.SourceType != "fixture_source" {
			t.Fatalf("unexpected source type in registry: %+v", item.Key)
		}
		seenForms[item.Key.SourceForm] = true
	}
	for _, form := range forms {
		if !seenForms[form] {
			t.Fatalf("registry missing source form %s", form)
		}
	}
	duplicate := testSourceAdapter{sourceType: "another_source", sourceForm: SourceFormManual, instanceID: "fixture-instance-a"}
	if err := registry.Register(duplicate); err == nil {
		t.Fatal("expected duplicate source instance id rejection")
	}
	assertCorePackageHasNoFutureAdapterDependency(t)
}

func TestSourceAdapterConformanceSubmitsOnlyThroughSink(t *testing.T) {
	now := time.Date(2026, 5, 22, 6, 0, 0, 0, time.UTC)
	adapter := testSourceAdapter{
		sourceType: "webhook_fixture",
		sourceForm: SourceFormWebhook,
		instanceID: "webhook-fixture-a",
		envelope: SourceEventEnvelope{
			SourceType:           "webhook_fixture",
			SourceInstanceID:     "webhook-fixture-a",
			SourceForm:           SourceFormWebhook,
			SourceEventID:        "event-123",
			ObservedAt:           now,
			RawPayloadKind:       "json",
			RawPayload:           []byte(`{"title":"disk space"}`),
			DeliveryMetadata:     map[string]string{"request_id": "req-1"},
			SourceSpecificFields: map[string]string{"priority": "high"},
		},
	}
	sink := &recordingSink{receipt: IngestReceipt{SourceType: "webhook_fixture", SourceInstanceID: "webhook-fixture-a", SourceForm: SourceFormWebhook, RawEventID: "raw-1", Accepted: true, Status: "accepted"}}
	if err := adapter.Start(context.Background(), sink); err != nil {
		t.Fatalf("start adapter: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("sink event count = %d, want 1", len(sink.events))
	}
	got := sink.events[0]
	if got.SourceType != adapter.sourceType || got.SourceInstanceID != adapter.instanceID || got.SourceForm != adapter.sourceForm {
		t.Fatalf("adapter submitted wrong source identity: %+v", got)
	}
	if !sink.receipts[0].Accepted || sink.receipts[0].RawEventID != "raw-1" {
		t.Fatalf("sink receipt did not preserve raw acceptance: %+v", sink.receipts[0])
	}
}

type testSourceAdapter struct {
	sourceType string
	sourceForm SourceForm
	instanceID string
	envelope   SourceEventEnvelope
}

func (a testSourceAdapter) SourceType() string                                          { return a.sourceType }
func (a testSourceAdapter) SourceForm() SourceForm                                      { return a.sourceForm }
func (a testSourceAdapter) InstanceID() string                                          { return a.instanceID }
func (a testSourceAdapter) Connect(ctx context.Context, cfg SourceInstanceConfig) error { return nil }
func (a testSourceAdapter) Start(ctx context.Context, sink SourceEventSink) error {
	receipt, err := sink.SubmitSourceEvent(ctx, a.envelope)
	if err != nil {
		return err
	}
	if !receipt.Accepted {
		return nil
	}
	return sink.ReportSourceHealth(ctx, SourceHealthReport{SourceType: a.sourceType, SourceInstanceID: a.instanceID, SourceForm: a.sourceForm, State: SourceHealthConnected, ObservedAt: a.envelope.ObservedAt})
}
func (a testSourceAdapter) Health(ctx context.Context) SourceHealthReport {
	return SourceHealthReport{SourceType: a.sourceType, SourceInstanceID: a.instanceID, SourceForm: a.sourceForm, State: SourceHealthConnected, ObservedAt: time.Date(2026, 5, 22, 6, 1, 0, 0, time.UTC)}
}
func (a testSourceAdapter) Stop(ctx context.Context) error { return nil }

type recordingSink struct {
	receipt       IngestReceipt
	events        []SourceEventEnvelope
	receipts      []IngestReceipt
	healthReports []SourceHealthReport
}

func (s *recordingSink) SubmitSourceEvent(ctx context.Context, envelope SourceEventEnvelope) (IngestReceipt, error) {
	s.events = append(s.events, envelope)
	s.receipts = append(s.receipts, s.receipt)
	return s.receipt, nil
}

func (s *recordingSink) ReportSourceHealth(ctx context.Context, report SourceHealthReport) error {
	s.healthReports = append(s.healthReports, report)
	return nil
}

func assertCorePackageHasNoFutureAdapterDependency(t *testing.T) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test file")
	}
	packageDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(packageDir, name))
		if err != nil {
			t.Fatalf("read production source %s: %v", name, err)
		}
		body := strings.ToLower(string(contents))
		for _, forbidden := range []string{"n" + "tfy", "tele" + "gram"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("production notification source %s contains forbidden source-specific token %q", name, forbidden)
			}
		}
	}
}
