// Spec 074 SCOPE-1 TP-074-03 — capture payload no-interpretation test.
//
// SCN-074-A10 — when a fallback Idea is captured, the persisted
// payload MUST contain only the normalized text + provenance + dedup
// inputs + trace ids. No inferred tags, topics, categories, or
// lifecycle guesses may appear in the payload struct or be populated
// by BuildCapturePayload.
//
// The test reflects the CapturePayload struct and rejects any field
// whose lowercased name contains a member of the forbidden vocabulary.
// This is a structural guard that survives refactors: even an
// "innocent" addition like InferredTopics or LifecycleHint will fail
// the test.
package capturefallback

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// forbiddenPayloadFieldSubstrings is the closed list of field-name
// fragments that signal capture-time interpretation. Lowercased
// CapturePayload field names are checked against each fragment.
var forbiddenPayloadFieldSubstrings = []string{
	"tag",
	"topic",
	"category",
	"categories",
	"lifecycle",
	"sentiment",
	"emotion",
	"classification",
	"summary",
	"label",
	"theme",
	"inferred",
}

func TestCapturePayload_StructHasNoInterpretationFields(t *testing.T) {
	rt := reflect.TypeOf(CapturePayload{})
	for i := 0; i < rt.NumField(); i++ {
		name := strings.ToLower(rt.Field(i).Name)
		for _, bad := range forbiddenPayloadFieldSubstrings {
			if strings.Contains(name, bad) {
				t.Errorf("CapturePayload field %q contains forbidden substring %q (SCN-074-A10: no interpretation at capture time)", rt.Field(i).Name, bad)
			}
		}
	}
}

func TestBuildCapturePayload_OmitsInterpretationMetadata(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	req := Request{
		UserID:                 "user-1",
		Transport:              "telegram",
		TransportMessageID:     "msg-42",
		OriginalText:           "  Try the new bakery on 4th!  ",
		Cause:                  CauseUnrouted,
		TraceID:                "trace-1",
		IntentTraceID:          "intent-1",
		AbandonedClarification: false,
		OccurredAt:             now,
	}
	dec := Decision{
		Cause:              CauseUnrouted,
		Provenance:         ProvenanceFallback,
		NormalizedText:     NormalizeV1(req.OriginalText),
		NormalizedTextHash: HashNormalized(NormalizeV1(req.OriginalText), "k"),
		DedupBucketStart:   BucketStart(now, 24*time.Hour),
		DedupWindow:        24 * time.Hour,
		SchemaVersion:      SchemaVersion,
		OccurredAt:         now,
	}
	payload := BuildCapturePayload(req, dec, "artifact-1")

	if payload.NormalizedText == "" {
		t.Errorf("payload.NormalizedText empty; expected normalized form of OriginalText")
	}
	if payload.Provenance != ProvenanceFallback {
		t.Errorf("payload.Provenance = %q, want %q", payload.Provenance, ProvenanceFallback)
	}
	if payload.FallbackCause != CauseUnrouted {
		t.Errorf("payload.FallbackCause = %q, want %q", payload.FallbackCause, CauseUnrouted)
	}
	if payload.DedupWindowSeconds != int(24*time.Hour/time.Second) {
		t.Errorf("payload.DedupWindowSeconds = %d, want %d", payload.DedupWindowSeconds, int(24*time.Hour/time.Second))
	}
	if payload.ArtifactID != "artifact-1" {
		t.Errorf("payload.ArtifactID = %q, want %q", payload.ArtifactID, "artifact-1")
	}
	if payload.SchemaVersion != SchemaVersion {
		t.Errorf("payload.SchemaVersion = %d, want %d", payload.SchemaVersion, SchemaVersion)
	}
	if payload.CreatedAt != now.UTC() {
		t.Errorf("payload.CreatedAt = %s, want %s", payload.CreatedAt, now.UTC())
	}
}

func TestNormalizeV1_NFKCAndCaseAndWhitespace(t *testing.T) {
	// Fullwidth digits + uppercase + tabs + double spaces → ascii lower single-space.
	in := "  Try\tthe  NEW Bakery on ４th!  "
	got := NormalizeV1(in)
	want := "try the new bakery on 4th!"
	if got != want {
		t.Errorf("NormalizeV1(%q) = %q, want %q", in, got, want)
	}
}

func TestHashNormalized_DeterministicAndKeyed(t *testing.T) {
	h1 := HashNormalized("hello world", "k1")
	h2 := HashNormalized("hello world", "k1")
	h3 := HashNormalized("hello world", "k2")
	if h1 != h2 {
		t.Errorf("HashNormalized non-deterministic: %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("HashNormalized must vary with key: %q == %q", h1, h3)
	}
	if HashNormalized("hello world", "") != "ERR_EMPTY_HASH_KEY" {
		t.Errorf("empty hash key must produce ERR_EMPTY_HASH_KEY sentinel")
	}
}

func TestBucketStart_AlignedAndStable(t *testing.T) {
	window := time.Hour
	t1 := time.Date(2026, 6, 1, 10, 15, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 10, 59, 59, 0, time.UTC)
	t3 := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	b1 := BucketStart(t1, window)
	b2 := BucketStart(t2, window)
	b3 := BucketStart(t3, window)
	if !b1.Equal(b2) {
		t.Errorf("same-window timestamps mapped to different buckets: %s vs %s", b1, b2)
	}
	if b1.Equal(b3) {
		t.Errorf("cross-window timestamps mapped to same bucket: %s == %s", b1, b3)
	}
}
