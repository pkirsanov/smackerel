//go:build integration

// BUG-080-001 SCOPE-03 — graph telemetry value safety (SCN-080-001-07).
//
// This file proves that the REAL graph telemetry adapter
// (internal/graphsynthetic.TelemetryObserver, constructed through the
// REAL graphsynthetic.NewTelemetryObserver) emits PLAIN spans whose
// every attribute is drawn from a CLOSED, CONTENT-FREE vocabulary, and
// that those spans are STRUCTURALLY DISJOINT from the ONLY trace
// workflow this repository registers: `core.health`
// (.github/bubbles-project.yaml → traceContracts.workflows), which
// covers `/api/health` liveness and is unrelated to the Knowledge
// Graph.
//
// SCOPE BOUNDARY — what this file deliberately does NOT do:
//
//   - It does NOT invent, declare, register, or reference an
//     `observabilityWorkflow` for the graph. Graph spans are PLAIN
//     spans, NOT a registered trace workflow.
//   - It does NOT claim a graph-specific G080 or G100 trace/SLO
//     contract. No graph SLO is asserted here, because none is
//     registered.
//   - It does NOT emit into, reuse, or assert a graph outcome against
//     `core.health`. That workflow name appears in this file for
//     exactly ONE purpose: to prove the graph span names are disjoint
//     from it.
//
// Spans are captured through the REAL generic tracer
// (internal/assistant/tracing) driving an OpenTelemetry SDK
// TracerProvider wired to an in-memory exporter — the same capture
// pattern internal/assistant/tracing/tracer_test.go already uses. There
// is NO mock of the code under test, NO NopObserver, NO t.Skip, and NO
// early return: every required assertion fails loudly when the behavior
// is absent.

package graphapi_integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// gtelRegisteredWorkflow is the ONLY trace workflow this repository
// registers. It is referenced here solely to prove disjointness; no
// graph span is ever emitted into it and no graph SLO is asserted
// against it.
const gtelRegisteredWorkflow = "core.health"

// gtelIdentityAttrs is the canonical 5-attribute identity set the
// generic tracer stamps on EVERY span it starts. The graph adapter
// passes all five as the empty string on purpose: a synthetic
// observation belongs to no user session, no assistant turn, no
// scenario, and no correlation.
var gtelIdentityAttrs = []string{
	"transport",
	"user_id_hashed",
	"assistant_turn_id",
	"scenario_id",
	"correlation_id",
}

// gtelOutcomeAttrs are the two attributes tracing.EndSpan stamps.
var gtelOutcomeAttrs = []string{"status", "error_cause"}

// gtelActivationKeys is the closed graph-owned key set of the
// `graph.activation` span.
var gtelActivationKeys = []string{
	"graph.activation.mode",
	"graph.activation.outcome",
	"graph.activation.code",
	"graph.activation.secret_presence",
}

// gtelFamilyReadKeys is the closed graph-owned key set of the
// `graph.family_read` span.
var gtelFamilyReadKeys = []string{
	"graph.read.family",
	"graph.read.outcome",
	"graph.read.code",
	"graph.read.evidence_ref",
	"graph.read.duration_ms",
}

// gtelAggregateKeys is the closed graph-owned key set of the
// `graph.synthetic_aggregate` span.
var gtelAggregateKeys = []string{
	"graph.synthetic.activation",
	"graph.synthetic.state",
	"graph.synthetic.code",
	"graph.synthetic.evidence_ref",
	"graph.synthetic.duration_ms",
	"graph.synthetic.family_count",
}

// gtelReadStates mirrors the closed ReadState vocabulary declared in
// internal/graphsynthetic/result.go. The package keeps its own copy
// unexported, so this test rebuilds it from the EXPORTED constants
// rather than hardcoding literals.
var gtelReadStates = []graphsynthetic.ReadState{
	graphsynthetic.StatePopulated,
	graphsynthetic.StateTrueEmpty,
	graphsynthetic.StateFailed,
	graphsynthetic.StateDisabled,
}

// gtelSyntheticCodes mirrors the closed synthetic diagnostic-code
// vocabulary, rebuilt from the EXPORTED constants.
var gtelSyntheticCodes = []string{
	graphsynthetic.CodeOK,
	graphsynthetic.CodeEmptyPermitted,
	graphsynthetic.CodeEmptyNotPermitted,
	graphsynthetic.CodeUnauthenticated,
	graphsynthetic.CodeForbidden,
	graphsynthetic.CodeRouteAbsent,
	graphsynthetic.CodeCapabilityDisabled,
	graphsynthetic.CodeStoreUnavailable,
	graphsynthetic.CodeServerError,
	graphsynthetic.CodeSchemaInvalid,
	graphsynthetic.CodeCursorInvalid,
	graphsynthetic.CodeRowMissing,
	graphsynthetic.CodeTransport,
	graphsynthetic.CodeUnexpectedStatus,
	graphsynthetic.CodePolicyDisabled,
	graphsynthetic.CodeFamilyMissing,
	graphsynthetic.CodeOptionalOmitted,
}

// gtelActivationStates is the closed fail-soft activation vocabulary.
var gtelActivationStates = []graphapi.ActivationState{
	graphapi.ActivationEnabled,
	graphapi.ActivationDisabled,
}

// gtelActivationOutcomes is the closed derived-outcome vocabulary the
// adapter computes from Activation.Disabled().
var gtelActivationOutcomes = []string{"enabled", "disabled"}

// gtelSecretPresences is the closed, value-safe secret-presence
// classification vocabulary. It names the presence CLASS only — never a
// secret value, its length, or a hash of it.
var gtelSecretPresences = []graphapi.SecretPresence{
	graphapi.SecretPresent,
	graphapi.SecretEmpty,
	graphapi.SecretMissing,
}

// gtelActivationCodes is the closed activation diagnostic-code
// vocabulary produced by graphapi.ResolveActivation.
var gtelActivationCodes = []string{
	graphapi.CodeActivationOK,
	graphapi.CodeCursorSecretEmpty,
	graphapi.CodeCursorSecretMissing,
}

// gtelUUIDPattern matches a UUID anywhere inside a value. A UUID in a
// span attribute is an identifier leak regardless of which id it is.
var gtelUUIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// gtelForbiddenValue is a content-bearing value that MUST NEVER reach a
// span attribute. label is safe to print; value is NEVER printed.
type gtelForbiddenValue struct {
	label string
	value string
}

// gtelForbiddenValues builds the content-bearing value set that flows
// near this code path in this test. Every one of these is genuinely
// passed INTO the observer (see gtelPoisonedFamilyRows) so the scan in
// the "no span attribute carries content" sub-test is a real leak probe
// and not a tautology.
func gtelForbiddenValues(t *testing.T) []gtelForbiddenValue {
	t.Helper()
	return []gtelForbiddenValue{
		// A fabricated-but-realistically-shaped credential. It is an
		// obvious placeholder on purpose so no secret scanner treats
		// this test fixture as a real leak.
		{label: "fabricated-credential", value: "PLACEHOLDER-NOT-A-REAL-CREDENTIAL-cursor-hmac-v1"}, //gitleaks:allow
		// A freshly generated (crypto/rand) RFC-4122 v4 identifier.
		{label: "generated-uuid", value: gtelNewUUID(t)},
		// A human-readable topic label — the exact class of graph
		// content a leaky span would expose.
		{label: "topic-label", value: "Quarterly Board Compensation Review"},
		// A target base URL.
		{label: "base-url", value: "http://example.invalid:8080"},
		// A host:port authority.
		{label: "host-port-authority", value: "graph-store.internal.invalid:5432"},
	}
}

// gtelNewUUID generates an RFC-4122 v4 UUID from crypto/rand without
// pulling in a UUID dependency.
func gtelNewUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("generate a UUID for the forbidden-value set: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// gtelPoisonedFamilyRows builds one family row per canonical family in
// which EVERY string-typed field carries a content-bearing forbidden
// value. These rows are handed to the REAL ObserveAggregate as
// AggregateResult.Families.
//
// This is the adversarial core of the content scan: the forbidden
// values are genuinely INSIDE the observer's input. The aggregate span
// is contractually allowed to emit only len(Families) — a bare count —
// so if ObserveAggregate ever widened to emit a joined family list,
// a per-family code, or any other row detail, the scan goes RED.
func gtelPoisonedFamilyRows(t *testing.T, forbidden []gtelForbiddenValue) []graphsynthetic.GraphFamilyResult {
	t.Helper()
	if len(forbidden) == 0 {
		t.Fatalf("test setup: the forbidden-value set is empty; the content scan would be a tautology")
	}
	families := graphapi.RequiredGraphFamilies()
	if len(families) == 0 {
		t.Fatalf("test setup: graphapi.RequiredGraphFamilies() returned no families")
	}
	rows := make([]graphsynthetic.GraphFamilyResult, 0, len(families))
	for i := range families {
		f := forbidden[i%len(forbidden)]
		row := graphsynthetic.GraphFamilyResult{
			Family:      graphapi.GraphRouteFamily(f.value),
			State:       graphsynthetic.ReadState(f.value),
			DurationMs:  int64(i),
			Code:        f.value,
			EvidenceRef: f.value,
		}
		// Non-tautology guard: prove the poison is genuinely OUTSIDE
		// every closed vocabulary. If Validate() accepted it, the
		// "content-free" claim would be meaningless.
		if err := row.Validate(); err == nil {
			t.Fatalf("test setup: poisoned family row %d (forbidden %q) passed Validate(); "+
				"the closed vocabulary would then admit content and this scan proves nothing",
				i, f.label)
		}
		rows = append(rows, row)
	}
	return rows
}

// gtelAttrIndex is a span's attribute set indexed by key.
type gtelAttrIndex map[string]attribute.KeyValue

// gtelIndexAttrs indexes a span's attributes and fails loudly on a
// duplicate key (a span attribute set MUST be a map, not a bag).
func gtelIndexAttrs(t *testing.T, span tracetest.SpanStub) gtelAttrIndex {
	t.Helper()
	idx := make(gtelAttrIndex, len(span.Attributes))
	for _, kv := range span.Attributes {
		key := string(kv.Key)
		if _, dup := idx[key]; dup {
			t.Fatalf("span %q carries duplicate attribute key %q", span.Name, key)
		}
		idx[key] = kv
	}
	return idx
}

// gtelSortedKeys returns the index's keys in stable order for
// deterministic failure messages.
func gtelSortedKeys(idx gtelAttrIndex) []string {
	keys := make([]string, 0, len(idx))
	for k := range idx {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// gtelExpectedKeys returns the exact closed key set a graph span of the
// supplied name MUST carry: its graph-owned keys plus the tracer-owned
// identity and outcome keys, and nothing else.
func gtelExpectedKeys(t *testing.T, spanName string) []string {
	t.Helper()
	var owned []string
	switch spanName {
	case graphsynthetic.SpanActivation:
		owned = gtelActivationKeys
	case graphsynthetic.SpanFamilyRead:
		owned = gtelFamilyReadKeys
	case graphsynthetic.SpanAggregate:
		owned = gtelAggregateKeys
	default:
		t.Fatalf("span %q is not a graph-owned span name; the graph adapter emits only %q, %q, %q",
			spanName, graphsynthetic.SpanActivation, graphsynthetic.SpanFamilyRead, graphsynthetic.SpanAggregate)
	}
	keys := make([]string, 0, len(owned)+len(gtelIdentityAttrs)+len(gtelOutcomeAttrs))
	keys = append(keys, owned...)
	keys = append(keys, gtelIdentityAttrs...)
	keys = append(keys, gtelOutcomeAttrs...)
	sort.Strings(keys)
	return keys
}

// gtelAssertExactKeySet proves the span carries EXACTLY the closed key
// set — no missing key and, critically, no extra key. An undeclared key
// is an open vocabulary and therefore a leak vector.
func gtelAssertExactKeySet(t *testing.T, span tracetest.SpanStub, idx gtelAttrIndex) {
	t.Helper()
	want := gtelExpectedKeys(t, span.Name)
	got := gtelSortedKeys(idx)
	if !slices.Equal(got, want) {
		for _, k := range got {
			if !slices.Contains(want, k) {
				t.Errorf("span %q carries UNDECLARED attribute key %q; the key set MUST be closed", span.Name, k)
			}
		}
		for _, k := range want {
			if !slices.Contains(got, k) {
				t.Errorf("span %q is MISSING required attribute key %q", span.Name, k)
			}
		}
		t.Fatalf("span %q attribute key set = %v; want exactly %v", span.Name, got, want)
	}
}

// gtelRequireString fetches a STRING-typed attribute, failing loudly if
// it is absent or carries a different OTel value type. The type check
// matters: attribute.Value.AsString() silently returns "" for a
// non-string, which would let a missing/mistyped attribute masquerade
// as a legitimately empty one.
func gtelRequireString(t *testing.T, spanName string, idx gtelAttrIndex, key string) string {
	t.Helper()
	kv, ok := idx[key]
	if !ok {
		t.Fatalf("span %q is missing required attribute %q; present keys=%v", spanName, key, gtelSortedKeys(idx))
	}
	if kv.Value.Type() != attribute.STRING {
		t.Fatalf("span %q attribute %q has OTel value type %s; want STRING", spanName, key, kv.Value.Type())
	}
	return kv.Value.AsString()
}

// gtelRequireInt64 fetches an INT64-typed attribute (attribute.Int and
// attribute.Int64 both land on INT64 in the OTel Go SDK).
func gtelRequireInt64(t *testing.T, spanName string, idx gtelAttrIndex, key string) int64 {
	t.Helper()
	kv, ok := idx[key]
	if !ok {
		t.Fatalf("span %q is missing required attribute %q; present keys=%v", spanName, key, gtelSortedKeys(idx))
	}
	if kv.Value.Type() != attribute.INT64 {
		t.Fatalf("span %q attribute %q has OTel value type %s; want INT64", spanName, key, kv.Value.Type())
	}
	return kv.Value.AsInt64()
}

// gtelDriveObserver runs the REAL graphsynthetic.TelemetryObserver
// against an OpenTelemetry SDK provider wired to an in-memory exporter
// and returns every recorded span.
//
// It drives, in order:
//
//	3 activation observations   — every closed (state, presence, code) outcome
//	8x4 = 32 family reads       — EVERY canonical family x EVERY ReadState
//	4 aggregate observations    — every closed AggregateState, each carrying
//	                              the POISONED family rows
func gtelDriveObserver(t *testing.T, forbidden []gtelForbiddenValue) tracetest.SpanStubs {
	t.Helper()

	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("tracer provider shutdown: %v", err)
		}
	}()
	tr := tracing.NewTracerFromProvider(provider, "smackerel-core")

	// The REAL constructor and the REAL adapter. NOT a NopObserver,
	// NOT a stub, NOT a fake.
	obs := graphsynthetic.NewTelemetryObserver(tr, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if obs == nil {
		t.Fatalf("graphsynthetic.NewTelemetryObserver returned nil; the code under test is unavailable")
	}

	// --- activation observations -------------------------------------
	activations := []graphapi.Activation{
		{State: graphapi.ActivationEnabled, SecretPresence: graphapi.SecretPresent, Code: graphapi.CodeActivationOK},
		{State: graphapi.ActivationDisabled, SecretPresence: graphapi.SecretEmpty, Code: graphapi.CodeCursorSecretEmpty},
		{State: graphapi.ActivationDisabled, SecretPresence: graphapi.SecretMissing, Code: graphapi.CodeCursorSecretMissing},
	}
	for _, activation := range activations {
		obs.ObserveActivation(activation)
	}

	// --- family-read observations ------------------------------------
	// One distinct failure code per family so the closed code
	// vocabulary is exercised broadly rather than on one lucky row.
	failureCodes := []string{
		graphsynthetic.CodeEmptyNotPermitted,
		graphsynthetic.CodeUnauthenticated,
		graphsynthetic.CodeForbidden,
		graphsynthetic.CodeRouteAbsent,
		graphsynthetic.CodeCapabilityDisabled,
		graphsynthetic.CodeStoreUnavailable,
		graphsynthetic.CodeServerError,
		graphsynthetic.CodeSchemaInvalid,
	}
	families := graphapi.RequiredGraphFamilies()
	if len(families) != 8 {
		t.Fatalf("graphapi.RequiredGraphFamilies() returned %d families; the canonical taxonomy is 8", len(families))
	}
	for i, family := range families {
		for j, state := range gtelReadStates {
			code := ""
			switch state {
			case graphsynthetic.StatePopulated:
				code = graphsynthetic.CodeOK
			case graphsynthetic.StateTrueEmpty:
				code = graphsynthetic.CodeEmptyPermitted
			case graphsynthetic.StateFailed:
				code = failureCodes[i%len(failureCodes)]
			case graphsynthetic.StateDisabled:
				code = graphsynthetic.CodePolicyDisabled
			default:
				t.Fatalf("test setup: read state %q has no mapped diagnostic code", state)
			}
			row := graphsynthetic.GraphFamilyResult{
				Family:     family,
				State:      state,
				DurationMs: int64(i*7 + j), // includes 0 for the first row
				Code:       code,
				// Derived from the family alone — never caller-supplied.
				EvidenceRef: graphsynthetic.EvidenceRef(family),
			}
			// Non-tautology guard in the other direction: the rows we
			// feed the observer are genuinely production-shaped, so a
			// "value is in the closed vocabulary" assertion downstream
			// is meaningful rather than trivially satisfied by garbage.
			if err := row.Validate(); err != nil {
				t.Fatalf("test setup: contract-valid family row for %q/%q failed Validate(): %v", family, state, err)
			}
			obs.ObserveFamilyRead(row)
		}
	}

	// --- aggregate observations --------------------------------------
	poisoned := gtelPoisonedFamilyRows(t, forbidden)
	aggStates := graphsynthetic.AggregateStates()
	if len(aggStates) == 0 {
		t.Fatalf("graphsynthetic.AggregateStates() returned an empty vocabulary")
	}
	for i, state := range aggStates {
		activation := graphapi.ActivationEnabled
		code := graphsynthetic.CodeOK
		switch state {
		case graphsynthetic.AggregateAvailable:
			code = graphsynthetic.CodeOK
		case graphsynthetic.AggregateDegraded:
			code = graphsynthetic.CodeOptionalOmitted
		case graphsynthetic.AggregateUnavailable:
			code = graphsynthetic.CodeFamilyMissing
		case graphsynthetic.AggregatePolicyDisabled:
			activation = graphapi.ActivationDisabled
			code = graphsynthetic.CodePolicyDisabled
		default:
			t.Fatalf("test setup: aggregate state %q has no mapped diagnostic code", state)
		}
		obs.ObserveAggregate(graphsynthetic.AggregateResult{
			Activation:  activation,
			State:       state,
			DurationMs:  int64(i * 11),
			Code:        code,
			EvidenceRef: graphsynthetic.AggregateEvidenceRef,
			// Content-bearing rows go IN; only a bare count may come OUT.
			Families: poisoned,
		})
	}

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatalf("the real TelemetryObserver recorded ZERO spans after %d activation, %d family-read, and %d aggregate observations; "+
			"graph telemetry is not emitting", len(activations), len(families)*len(gtelReadStates), len(aggStates))
	}
	return spans
}

// TestGraphActivationAndFamilyReadTelemetryUsesClosedContentFreeAttributes
// proves the graph telemetry adapter emits plain, graph-owned spans
// whose attribute keys and values are closed and content-free, and
// whose names are structurally disjoint from `core.health` — the ONLY
// trace workflow this repository registers.
func TestGraphActivationAndFamilyReadTelemetryUsesClosedContentFreeAttributes(t *testing.T) {
	forbidden := gtelForbiddenValues(t)
	spans := gtelDriveObserver(t, forbidden)

	wantActivation := 3
	wantFamilyRead := len(graphapi.RequiredGraphFamilies()) * len(gtelReadStates)
	wantAggregate := len(graphsynthetic.AggregateStates())
	wantTotal := wantActivation + wantFamilyRead + wantAggregate
	if len(spans) != wantTotal {
		t.Fatalf("recorded %d spans; want exactly %d (%d activation + %d family_read + %d aggregate)",
			len(spans), wantTotal, wantActivation, wantFamilyRead, wantAggregate)
	}

	totalAttrs := 0
	for _, span := range spans {
		totalAttrs += len(span.Attributes)
	}
	t.Logf("inspected %d spans carrying %d attributes in total (%d activation, %d family_read, %d aggregate)",
		len(spans), totalAttrs, wantActivation, wantFamilyRead, wantAggregate)

	t.Run("span_names_are_graph_owned_and_disjoint_from_core_health_workflow", func(t *testing.T) {
		graphSpanNames := []string{
			graphsynthetic.SpanActivation,
			graphsynthetic.SpanFamilyRead,
			graphsynthetic.SpanAggregate,
		}
		counts := map[string]int{}
		for _, span := range spans {
			if !slices.Contains(graphSpanNames, span.Name) {
				t.Errorf("recorded span name %q is not one of the graph-owned span names %v", span.Name, graphSpanNames)
				continue
			}
			counts[span.Name]++
		}
		if t.Failed() {
			t.Fatalf("the graph adapter emitted a span outside its own closed span-name set")
		}
		if got := counts[graphsynthetic.SpanActivation]; got != wantActivation {
			t.Errorf("%q span count = %d; want %d", graphsynthetic.SpanActivation, got, wantActivation)
		}
		if got := counts[graphsynthetic.SpanFamilyRead]; got != wantFamilyRead {
			t.Errorf("%q span count = %d; want %d", graphsynthetic.SpanFamilyRead, got, wantFamilyRead)
		}
		if got := counts[graphsynthetic.SpanAggregate]; got != wantAggregate {
			t.Errorf("%q span count = %d; want %d", graphsynthetic.SpanAggregate, got, wantAggregate)
		}
		if len(counts) != len(graphSpanNames) {
			t.Errorf("recorded %d distinct span names %v; want exactly the %d graph span names %v",
				len(counts), gtelSortedNames(counts), len(graphSpanNames), graphSpanNames)
		}

		// Disjointness from the ONE registered workflow. `core.health`
		// is referenced ONLY here, and only to prove separation: no
		// graph span is emitted into it and no graph SLO is asserted
		// against it.
		for _, span := range spans {
			if span.Name == gtelRegisteredWorkflow {
				t.Errorf("span name %q collides with the registered %q workflow; graph spans are PLAIN spans and MUST NOT reuse it",
					span.Name, gtelRegisteredWorkflow)
			}
			if strings.HasPrefix(span.Name, gtelRegisteredWorkflow+".") {
				t.Errorf("span name %q is nested under the registered %q workflow namespace; graph spans MUST be disjoint from it",
					span.Name, gtelRegisteredWorkflow)
			}
			if strings.Contains(span.Name, "health") {
				t.Errorf("span name %q contains %q; a graph span MUST NOT present itself as part of the health workflow",
					span.Name, "health")
			}
		}
	})

	t.Run("activation_telemetry_attributes_are_closed", func(t *testing.T) {
		seenModes := map[string]bool{}
		seenPresences := map[string]bool{}
		inspected := 0
		attrsInspected := 0
		for _, span := range spans {
			if span.Name != graphsynthetic.SpanActivation {
				continue
			}
			inspected++
			idx := gtelIndexAttrs(t, span)
			attrsInspected += len(idx)
			gtelAssertExactKeySet(t, span, idx)

			mode := gtelRequireString(t, span.Name, idx, "graph.activation.mode")
			if !slices.Contains(gtelActivationStates, graphapi.ActivationState(mode)) {
				t.Errorf("attribute %q = %q is outside the closed activation-state vocabulary %v",
					"graph.activation.mode", mode, gtelActivationStates)
			}
			seenModes[mode] = true

			outcome := gtelRequireString(t, span.Name, idx, "graph.activation.outcome")
			if !slices.Contains(gtelActivationOutcomes, outcome) {
				t.Errorf("attribute %q = %q is outside the closed outcome vocabulary %v",
					"graph.activation.outcome", outcome, gtelActivationOutcomes)
			}

			code := gtelRequireString(t, span.Name, idx, "graph.activation.code")
			if !slices.Contains(gtelActivationCodes, code) {
				t.Errorf("attribute %q = %q is outside the closed activation-code vocabulary %v",
					"graph.activation.code", code, gtelActivationCodes)
			}

			presence := gtelRequireString(t, span.Name, idx, "graph.activation.secret_presence")
			if !slices.Contains(gtelSecretPresences, graphapi.SecretPresence(presence)) {
				t.Errorf("attribute %q = %q is outside the closed secret-presence vocabulary %v",
					"graph.activation.secret_presence", presence, gtelSecretPresences)
			}
			seenPresences[presence] = true
		}
		if inspected != wantActivation {
			t.Fatalf("inspected %d %q spans; want %d", inspected, graphsynthetic.SpanActivation, wantActivation)
		}
		// Coverage guard: the closed-vocabulary check must have run
		// across BOTH activation states and ALL THREE presence classes,
		// not on one lucky row.
		for _, state := range gtelActivationStates {
			if !seenModes[string(state)] {
				t.Errorf("activation state %q never appeared in a recorded span; the vocabulary check is under-exercised", state)
			}
		}
		for _, presence := range gtelSecretPresences {
			if !seenPresences[string(presence)] {
				t.Errorf("secret presence %q never appeared in a recorded span; the vocabulary check is under-exercised", presence)
			}
		}
		t.Logf("activation: inspected %d spans / %d attributes", inspected, attrsInspected)
	})

	t.Run("family_read_telemetry_attributes_are_closed", func(t *testing.T) {
		canonical := graphapi.RequiredGraphFamilies()
		seenFamilies := map[graphapi.GraphRouteFamily]bool{}
		seenStates := map[graphsynthetic.ReadState]bool{}
		seenCodes := map[string]bool{}
		inspected := 0
		attrsInspected := 0
		for _, span := range spans {
			if span.Name != graphsynthetic.SpanFamilyRead {
				continue
			}
			inspected++
			idx := gtelIndexAttrs(t, span)
			attrsInspected += len(idx)
			gtelAssertExactKeySet(t, span, idx)

			family := gtelRequireString(t, span.Name, idx, "graph.read.family")
			if !slices.Contains(canonical, graphapi.GraphRouteFamily(family)) {
				t.Errorf("attribute %q = %q is not one of the 8 canonical graph route families %v",
					"graph.read.family", family, canonical)
			}
			seenFamilies[graphapi.GraphRouteFamily(family)] = true

			outcome := gtelRequireString(t, span.Name, idx, "graph.read.outcome")
			if !slices.Contains(gtelReadStates, graphsynthetic.ReadState(outcome)) {
				t.Errorf("attribute %q = %q is outside the closed read-state vocabulary %v",
					"graph.read.outcome", outcome, gtelReadStates)
			}
			seenStates[graphsynthetic.ReadState(outcome)] = true

			code := gtelRequireString(t, span.Name, idx, "graph.read.code")
			if !slices.Contains(gtelSyntheticCodes, code) {
				t.Errorf("attribute %q = %q is outside the closed diagnostic-code vocabulary %v",
					"graph.read.code", code, gtelSyntheticCodes)
			}
			seenCodes[code] = true

			evidence := gtelRequireString(t, span.Name, idx, "graph.read.evidence_ref")
			if want := graphsynthetic.EvidenceRef(graphapi.GraphRouteFamily(family)); evidence != want {
				t.Errorf("attribute %q = %q; it MUST be derived from the family name alone as %q",
					"graph.read.evidence_ref", evidence, want)
			}

			duration := gtelRequireInt64(t, span.Name, idx, "graph.read.duration_ms")
			if duration < 0 {
				t.Errorf("attribute %q = %d; a duration MUST be non-negative", "graph.read.duration_ms", duration)
			}
		}
		if inspected != wantFamilyRead {
			t.Fatalf("inspected %d %q spans; want %d (8 canonical families x %d read states)",
				inspected, graphsynthetic.SpanFamilyRead, wantFamilyRead, len(gtelReadStates))
		}
		// Coverage guards: EVERY canonical family and EVERY ReadState
		// must have been driven, so the vocabulary check is broad.
		for _, family := range canonical {
			if !seenFamilies[family] {
				t.Errorf("canonical family %q was never observed; the family-read vocabulary check is incomplete", family)
			}
		}
		for _, state := range gtelReadStates {
			if !seenStates[state] {
				t.Errorf("read state %q was never observed; the family-read vocabulary check is incomplete", state)
			}
		}
		if len(seenCodes) < 4 {
			t.Errorf("only %d distinct diagnostic codes were observed (%v); the code vocabulary check is under-exercised",
				len(seenCodes), gtelSortedSet(seenCodes))
		}
		t.Logf("family_read: inspected %d spans / %d attributes across %d families, %d read states, %d distinct codes",
			inspected, attrsInspected, len(seenFamilies), len(seenStates), len(seenCodes))
	})

	t.Run("identity_attributes_are_present_but_empty_on_every_graph_span", func(t *testing.T) {
		inspected := 0
		attrsInspected := 0
		for _, span := range spans {
			idx := gtelIndexAttrs(t, span)
			inspected++
			// A synthetic observation belongs to no user session, no
			// assistant turn, no scenario, and no correlation: each of
			// the 5 canonical identity attributes MUST be PRESENT (so
			// the span shape stays uniform) and MUST be EMPTY (so no
			// session context is carried).
			for _, key := range gtelIdentityAttrs {
				value := gtelRequireString(t, span.Name, idx, key)
				attrsInspected++
				if value != "" {
					t.Errorf("span %q identity attribute %q is non-empty; a graph synthetic observation MUST carry no user session, "+
						"no assistant turn, no scenario, and no correlation (value redacted: %d bytes)",
						span.Name, key, len(value))
				}
			}
			status := gtelRequireString(t, span.Name, idx, "status")
			attrsInspected++
			if status != "ok" {
				t.Errorf("span %q attribute %q = %q; want %q", span.Name, "status", status, "ok")
			}
			cause := gtelRequireString(t, span.Name, idx, "error_cause")
			attrsInspected++
			if cause != "" {
				t.Errorf("span %q attribute %q = %q; want empty when status is ok", span.Name, "error_cause", cause)
			}
		}
		if inspected != wantTotal {
			t.Fatalf("inspected %d spans for identity attributes; want %d", inspected, wantTotal)
		}
		t.Logf("identity: inspected %d spans / %d tracer-owned attributes (5 identity + status + error_cause per span)",
			inspected, attrsInspected)
	})

	t.Run("no_span_attribute_carries_content", func(t *testing.T) {
		// Proof that the probe is real, not a tautology: the poisoned
		// family rows genuinely reached ObserveAggregate. The aggregate
		// span is contractually allowed to emit only their COUNT.
		wantFamilyCount := int64(len(graphapi.RequiredGraphFamilies()))
		aggregatesChecked := 0
		for _, span := range spans {
			if span.Name != graphsynthetic.SpanAggregate {
				continue
			}
			aggregatesChecked++
			idx := gtelIndexAttrs(t, span)
			gtelAssertExactKeySet(t, span, idx)

			if got := gtelRequireInt64(t, span.Name, idx, "graph.synthetic.family_count"); got != wantFamilyCount {
				t.Fatalf("attribute %q = %d; want %d — the content-bearing family rows did NOT reach ObserveAggregate, "+
					"so this leak probe would prove nothing", "graph.synthetic.family_count", got, wantFamilyCount)
			}
			// The aggregate's own attributes must still be closed.
			activation := gtelRequireString(t, span.Name, idx, "graph.synthetic.activation")
			if !slices.Contains(gtelActivationStates, graphapi.ActivationState(activation)) {
				t.Errorf("attribute %q = %q is outside the closed activation vocabulary %v",
					"graph.synthetic.activation", activation, gtelActivationStates)
			}
			state := gtelRequireString(t, span.Name, idx, "graph.synthetic.state")
			if !slices.Contains(graphsynthetic.AggregateStates(), graphsynthetic.AggregateState(state)) {
				t.Errorf("attribute %q = %q is outside the closed aggregate-state vocabulary %v",
					"graph.synthetic.state", state, graphsynthetic.AggregateStates())
			}
			code := gtelRequireString(t, span.Name, idx, "graph.synthetic.code")
			if !slices.Contains(gtelSyntheticCodes, code) {
				t.Errorf("attribute %q = %q is outside the closed diagnostic-code vocabulary", "graph.synthetic.code", code)
			}
			evidence := gtelRequireString(t, span.Name, idx, "graph.synthetic.evidence_ref")
			if evidence != graphsynthetic.AggregateEvidenceRef {
				t.Errorf("attribute %q = %q; it MUST be the constant %q",
					"graph.synthetic.evidence_ref", evidence, graphsynthetic.AggregateEvidenceRef)
			}
			if got := gtelRequireInt64(t, span.Name, idx, "graph.synthetic.duration_ms"); got < 0 {
				t.Errorf("attribute %q = %d; a duration MUST be non-negative", "graph.synthetic.duration_ms", got)
			}
		}
		if aggregatesChecked != wantAggregate {
			t.Fatalf("inspected %d %q spans; want %d", aggregatesChecked, graphsynthetic.SpanAggregate, wantAggregate)
		}

		// The scan itself. Every attribute of every span, key AND
		// rendered value. Failure messages print the attribute key and
		// a REDACTED marker naming the forbidden value's LABEL — never
		// the value itself.
		scanned := 0
		for _, span := range spans {
			for _, kv := range span.Attributes {
				scanned++
				key := string(kv.Key)
				// Emit() renders any OTel value type (string, int64,
				// bool, slices) to a string, so no attribute escapes
				// the scan by being non-string.
				rendered := kv.Value.Emit()

				for _, f := range forbidden {
					if strings.Contains(key, f.value) {
						t.Errorf("span %q attribute KEY %q contains forbidden content [REDACTED: %s]",
							span.Name, key, f.label)
					}
					if strings.Contains(rendered, f.value) {
						t.Errorf("span %q attribute %q VALUE leaks forbidden content [REDACTED: %s]",
							span.Name, key, f.label)
					}
					lowerKey := strings.ToLower(key)
					lowerRendered := strings.ToLower(rendered)
					lowerValue := strings.ToLower(f.value)
					if strings.Contains(lowerKey, lowerValue) || strings.Contains(lowerRendered, lowerValue) {
						t.Errorf("span %q attribute %q leaks forbidden content case-insensitively [REDACTED: %s]",
							span.Name, key, f.label)
					}
				}

				// Structural leak classes, independent of the explicit
				// forbidden list: an identifier or a target endpoint in
				// ANY attribute value is a leak whatever its origin.
				if gtelUUIDPattern.MatchString(rendered) {
					t.Errorf("span %q attribute %q VALUE is UUID-shaped; a span attribute MUST NOT carry an identifier [REDACTED: %d bytes]",
						span.Name, key, len(rendered))
				}
				if strings.Contains(rendered, "http://") || strings.Contains(rendered, "https://") {
					t.Errorf("span %q attribute %q VALUE carries a URL scheme; a span attribute MUST NOT carry a target endpoint [REDACTED: %d bytes]",
						span.Name, key, len(rendered))
				}
			}
		}
		if scanned != totalAttrs {
			t.Fatalf("content scan covered %d attributes; want %d (every attribute of every span)", scanned, totalAttrs)
		}
		t.Logf("content scan: %d attributes across %d spans checked against %d forbidden values, the UUID shape, and URL schemes",
			scanned, len(spans), len(forbidden))
	})
}

// gtelSortedNames returns a count map's keys in stable order.
func gtelSortedNames(counts map[string]int) []string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// gtelSortedSet returns a set's members in stable order.
func gtelSortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
