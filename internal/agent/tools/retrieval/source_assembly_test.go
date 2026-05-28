// Spec 061 SCOPE-06 — source-assembly invariant unit tests.
//
// These tests are the canonical proof of the design §5.1 contract:
//
//   - cited_artifact_ids → []contracts.Source with Title + CapturedAt
//     populated when the artifact exists in the graph;
//   - missing artifacts are dropped and increment
//     smackerel_assistant_source_assembly_drops_total{cause="missing_artifact"};
//   - lookup-callback errors are dropped and increment the same
//     counter with cause="lookup_error" instead of crashing the
//     response;
//   - ALL-missing case returns empty Sources[] so the provenance
//     gate (proven independently in
//     internal/assistant/provenance/gate_test.go) fires the canonical
//     refusal — this is the BS-007 end-to-end proof at the
//     capability-pure layer (the BS-007 e2e test on the full stack
//     is blocked on the SCOPE-04 facade post-processor hook; see
//     scopes.md SCOPE-06 evidence for the routed finding);
//   - sources_max cap is enforced and excess survivors become
//     SourcesOverflowCount, NOT silently dropped.
//
// Each test reads the counter delta with the same dto.Metric pattern
// internal/assistant/provenance/gate_test.go uses so the assertions
// are exact and resilient to other tests in the suite incrementing
// the global counter.
package retrieval

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
)

func TestAssembleSources_HappyPath_AllPresent(t *testing.T) {
	now := time.Date(2025, 3, 14, 9, 30, 0, 0, time.UTC)
	lookup := mapLookup(map[string]lookupRow{
		"a1": {title: "On Tailscale ACLs", capturedAt: now},
		"a2": {title: "Mullvad exit nodes", capturedAt: now.Add(-time.Hour)},
		"a3": {title: "Headscale 0.23 notes", capturedAt: now.Add(-2 * time.Hour)},
	})

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)
	beforeError := readDropsCounter(t, assistantmetrics.DropCauseLookupError)

	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"a1", "a2", "a3"}, lookup, 5)

	if overflow != 0 {
		t.Fatalf("overflow: want 0, got %d", overflow)
	}
	if got := len(sources); got != 3 {
		t.Fatalf("sources: want 3, got %d", got)
	}
	assertArtifactSource(t, sources[0], "a1", "On Tailscale ACLs", now)
	assertArtifactSource(t, sources[1], "a2", "Mullvad exit nodes", now.Add(-time.Hour))
	assertArtifactSource(t, sources[2], "a3", "Headscale 0.23 notes", now.Add(-2*time.Hour))

	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 0 {
		t.Fatalf("happy path must not increment missing_artifact: +%v", d)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseLookupError) - beforeError; d != 0 {
		t.Fatalf("happy path must not increment lookup_error: +%v", d)
	}
}

// TestAssembleSources_GraphDrift_PartialMissing proves the design §5.1
// invariant: missing IDs are dropped AND the counter increments by
// EXACTLY the number of missing IDs. Adversarial: if the assembler
// silently bypassed the increment (regressing BS-007 observability)
// or double-counted, this would fail.
func TestAssembleSources_GraphDrift_PartialMissing(t *testing.T) {
	now := time.Date(2025, 3, 14, 12, 0, 0, 0, time.UTC)
	lookup := mapLookup(map[string]lookupRow{
		"present-1": {title: "Present 1", capturedAt: now},
		"present-2": {title: "Present 2", capturedAt: now.Add(-30 * time.Minute)},
		// "missing-1" and "missing-2" are deliberately absent.
	})

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)
	beforeError := readDropsCounter(t, assistantmetrics.DropCauseLookupError)

	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"missing-1", "present-1", "missing-2", "present-2"},
		lookup, 10)

	if overflow != 0 {
		t.Fatalf("overflow: want 0, got %d", overflow)
	}
	if got := len(sources); got != 2 {
		t.Fatalf("sources: want 2 survivors, got %d", got)
	}
	assertArtifactSource(t, sources[0], "present-1", "Present 1", now)
	assertArtifactSource(t, sources[1], "present-2", "Present 2", now.Add(-30*time.Minute))

	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 2 {
		t.Fatalf("missing_artifact: want +2, got +%v", d)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseLookupError) - beforeError; d != 0 {
		t.Fatalf("partial-missing must not touch lookup_error: +%v", d)
	}
}

// TestAssembleSources_AllMissing_TriggersRefusalContract proves the
// BS-007 end-to-end contract at the source-assembly boundary: when
// every cited ID is missing, Sources[] is empty. The provenance gate
// (proven separately) sees the empty Sources and emits the canonical
// refusal + capture-route. This test is the SCOPE-06-owned half of
// the BS-007 proof; the full-stack half is blocked on the SCOPE-04
// facade post-processor hook.
func TestAssembleSources_AllMissing_TriggersRefusalContract(t *testing.T) {
	lookup := mapLookup(nil) // every lookup returns found=false

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)

	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"gone-1", "gone-2", "gone-3"}, lookup, 5)

	if sources == nil {
		// Empty slice and nil are both acceptable; the gate's check is
		// `len(resp.Sources) > 0`. Documented here so a future refactor
		// that picks nil over []contracts.Source{} does not surprise
		// the gate contract.
		t.Logf("sources is nil (also accepted: len()==0 is the contract)")
	}
	if got := len(sources); got != 0 {
		t.Fatalf("sources: want 0 (all-missing → empty), got %d", got)
	}
	if overflow != 0 {
		t.Fatalf("overflow: want 0, got %d", overflow)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 3 {
		t.Fatalf("missing_artifact: want +3, got +%v", d)
	}
}

// TestAssembleSources_LookupError_DroppedAndCounted proves transient
// lookup failures do NOT crash the response; they are dropped and
// counted under cause=lookup_error so dashboards can distinguish
// "graph drift" from "PG outage hiding sources".
func TestAssembleSources_LookupError_DroppedAndCounted(t *testing.T) {
	now := time.Date(2025, 3, 14, 15, 0, 0, 0, time.UTC)
	transient := errors.New("connection reset by peer")
	lookup := func(_ context.Context, id string) (string, time.Time, bool, error) {
		switch id {
		case "ok":
			return "Survivor", now, true, nil
		case "errored":
			return "", time.Time{}, false, transient
		default:
			return "", time.Time{}, false, nil
		}
	}

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)
	beforeError := readDropsCounter(t, assistantmetrics.DropCauseLookupError)

	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"errored", "ok", "errored"}, lookup, 10)

	// "errored" appears twice but dedup collapses to one lookup; the
	// surviving "ok" gives us exactly one Source.
	if got := len(sources); got != 1 {
		t.Fatalf("sources: want 1, got %d", got)
	}
	if overflow != 0 {
		t.Fatalf("overflow: want 0, got %d", overflow)
	}
	assertArtifactSource(t, sources[0], "ok", "Survivor", now)

	if d := readDropsCounter(t, assistantmetrics.DropCauseLookupError) - beforeError; d != 1 {
		t.Fatalf("lookup_error: want +1 (after dedup), got +%v", d)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 0 {
		t.Fatalf("lookup-error case must not touch missing_artifact: +%v", d)
	}
}

// TestAssembleSources_NotFoundSentinel_TreatedAsMissingArtifact proves
// callers can return ErrArtifactNotFound alongside found=false for
// caller-side diagnostics without changing counter cardinality.
func TestAssembleSources_NotFoundSentinel_TreatedAsMissingArtifact(t *testing.T) {
	lookup := func(_ context.Context, _ string) (string, time.Time, bool, error) {
		return "", time.Time{}, false, ErrArtifactNotFound
	}

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)
	beforeError := readDropsCounter(t, assistantmetrics.DropCauseLookupError)

	sources, _ := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"x", "y"}, lookup, 5)
	if got := len(sources); got != 0 {
		t.Fatalf("sources: want 0, got %d", got)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 2 {
		t.Fatalf("missing_artifact: want +2 (ErrArtifactNotFound is a missing-artifact drop), got +%v", d)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseLookupError) - beforeError; d != 0 {
		t.Fatalf("ErrArtifactNotFound must NOT route to lookup_error: +%v", d)
	}
}

// TestAssembleSources_CapAndOverflow proves the SST cap is honored
// AND that the overflow count is exact so the facade can populate
// contracts.AssistantResponse.SourcesOverflowCount per the response
// contract (internal/assistant/contracts/response.go).
func TestAssembleSources_CapAndOverflow(t *testing.T) {
	now := time.Date(2025, 3, 14, 18, 0, 0, 0, time.UTC)
	rows := map[string]lookupRow{}
	for _, id := range []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7"} {
		rows[id] = lookupRow{title: "title-" + id, capturedAt: now}
	}
	lookup := mapLookup(rows)

	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"s1", "s2", "s3", "s4", "s5", "s6", "s7"}, lookup, 5)

	if got := len(sources); got != 5 {
		t.Fatalf("sources: want 5 (cap), got %d", got)
	}
	if overflow != 2 {
		t.Fatalf("overflow: want 2 (=7−5), got %d", overflow)
	}
	for _, want := range []string{"s1", "s2", "s3", "s4", "s5"} {
		var present bool
		for _, s := range sources {
			if s.ID == want {
				present = true
				break
			}
		}
		if !present {
			t.Fatalf("first-5 invariant violated: want %s in sources", want)
		}
	}
}

// TestAssembleSources_DuplicateCitations_Collapsed proves the LLM
// citing the same artifact twice does not double-spend the cap or
// double-count drops. Adversarial: if dedup were silently removed,
// this test would fail because overflow would become 1 and the
// counter increment would double.
func TestAssembleSources_DuplicateCitations_Collapsed(t *testing.T) {
	now := time.Date(2025, 3, 14, 21, 0, 0, 0, time.UTC)
	lookup := mapLookup(map[string]lookupRow{
		"same": {title: "Cited Twice", capturedAt: now},
	})

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)

	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"same", "same", "same"}, lookup, 5)

	if got := len(sources); got != 1 {
		t.Fatalf("sources: want 1 (deduplicated), got %d", got)
	}
	if overflow != 0 {
		t.Fatalf("overflow: want 0, got %d", overflow)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 0 {
		t.Fatalf("dedup must not touch missing_artifact: +%v", d)
	}
}

// TestAssembleSources_EmptyStringID_TreatedAsMissingArtifact proves an
// empty cited ID is a graph-drift case, not a panic or a silent skip.
func TestAssembleSources_EmptyStringID_TreatedAsMissingArtifact(t *testing.T) {
	now := time.Date(2025, 3, 14, 23, 0, 0, 0, time.UTC)
	lookup := mapLookup(map[string]lookupRow{
		"keep": {title: "Keep", capturedAt: now},
	})

	beforeMissing := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact)

	sources, _ := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"", "keep", ""}, lookup, 5)
	if got := len(sources); got != 1 {
		t.Fatalf("sources: want 1 (only \"keep\"), got %d", got)
	}
	if d := readDropsCounter(t, assistantmetrics.DropCauseMissingArtifact) - beforeMissing; d != 2 {
		t.Fatalf("missing_artifact: want +2 (both empty IDs), got +%v", d)
	}
}

// TestAssembleSources_ZeroOrNegativeSourcesMax_ReturnsEmpty proves the
// SST-misconfiguration guardrail: a typo that lands sources_max <= 0
// must NOT attempt lookups (avoiding wasted PG calls) AND must
// return an empty Sources[] so the provenance gate fires downstream
// instead of silently emitting unsourced bodies.
func TestAssembleSources_ZeroOrNegativeSourcesMax_ReturnsEmpty(t *testing.T) {
	called := 0
	lookup := func(_ context.Context, _ string) (string, time.Time, bool, error) {
		called++
		return "x", time.Time{}, true, nil
	}

	for _, max := range []int{0, -1, -100} {
		sources, overflow := AssembleSources(context.Background(),
			"retrieval_qa",
			[]string{"a", "b", "c"}, lookup, max)
		if sources != nil {
			t.Fatalf("max=%d: want nil sources, got %#v", max, sources)
		}
		if overflow != 0 {
			t.Fatalf("max=%d: want 0 overflow, got %d", max, overflow)
		}
	}
	if called != 0 {
		t.Fatalf("guardrail must short-circuit before any lookup; got %d calls", called)
	}
}

// TestAssembleSources_NilLookup_ReturnsEmpty proves a misconfigured
// caller (forgot to wire the lookup) does NOT panic.
func TestAssembleSources_NilLookup_ReturnsEmpty(t *testing.T) {
	sources, overflow := AssembleSources(context.Background(),
		"retrieval_qa",
		[]string{"a"}, nil, 5)
	if sources != nil {
		t.Fatalf("nil lookup: want nil sources, got %#v", sources)
	}
	if overflow != 0 {
		t.Fatalf("nil lookup: want 0 overflow, got %d", overflow)
	}
}

// -------------------- helpers --------------------

type lookupRow struct {
	title      string
	capturedAt time.Time
}

func mapLookup(rows map[string]lookupRow) ArtifactLookupFn {
	return func(_ context.Context, id string) (string, time.Time, bool, error) {
		row, ok := rows[id]
		if !ok {
			return "", time.Time{}, false, nil
		}
		return row.title, row.capturedAt, true, nil
	}
}

func assertArtifactSource(t *testing.T, s contracts.Source, wantID, wantTitle string, wantCapturedAt time.Time) {
	t.Helper()
	if s.ID != wantID {
		t.Fatalf("Source.ID: want %q, got %q", wantID, s.ID)
	}
	if s.Title != wantTitle {
		t.Fatalf("Source.Title: want %q, got %q", wantTitle, s.Title)
	}
	if s.Kind != contracts.SourceArtifact {
		t.Fatalf("Source.Kind: want SourceArtifact, got %q", s.Kind)
	}
	ref, ok := s.Ref.(contracts.ArtifactRef)
	if !ok {
		t.Fatalf("Source.Ref: want ArtifactRef, got %T", s.Ref)
	}
	if ref.ArtifactID != wantID {
		t.Fatalf("ArtifactRef.ArtifactID: want %q, got %q", wantID, ref.ArtifactID)
	}
	if !ref.CapturedAt.Equal(wantCapturedAt) {
		t.Fatalf("ArtifactRef.CapturedAt: want %v, got %v", wantCapturedAt, ref.CapturedAt)
	}
}

func readDropsCounter(t *testing.T, cause assistantmetrics.SourceAssemblyDropCause) float64 {
	t.Helper()
	c := assistantmetrics.SourceAssemblyDropsCounter.WithLabelValues("retrieval_qa", string(cause))
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter.Write(%q): %v", cause, err)
	}
	if m.Counter == nil {
		return 0
	}
	return m.Counter.GetValue()
}
