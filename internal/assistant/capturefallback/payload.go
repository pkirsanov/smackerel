// Spec 074 SCOPE-1 — capture-as-fallback payload.
//
// CapturePayload is the closed shape persisted alongside a fallback
// Idea (artifact_capture_policy row in the SCOPE-2 migration). The
// SCOPE-1 contract is that NO interpretation happens at capture time:
// the payload carries normalized text, provenance, cause, dedup
// inputs, trace ids, and the abandoned-clarification flag, and
// NOTHING ELSE. No inferred tags, topics, categories, or lifecycle
// guesses may appear in this struct or in any sibling persistence
// shape that the capture path writes.
//
// SCN-074-A10 is enforced both behaviorally (BuildCapturePayload
// constructs the struct directly with no analysis hooks) and
// structurally (payload_test.go reflects the struct and rejects any
// field whose name matches the forbidden vocabulary).
package capturefallback

import "time"

// CapturePayload is the persisted metadata shape for a fallback Idea
// capture. Field names are part of the public contract — adding a
// "tags", "topics", "categories", or "lifecycle*" field is a SCN-074-A10
// regression and is rejected by payload_test.go.
type CapturePayload struct {
	// ArtifactID is the Idea artifact id; set by the IdeaWriter
	// in SCOPE-2/SCOPE-4.
	ArtifactID string
	// UserID scopes the capture per-user (SCN-074-A05 forbids
	// cross-user dedup).
	UserID string
	// Provenance is the closed-vocabulary capture provenance.
	// SCOPE-1 only constructs ProvenanceFallback payloads; spec 008
	// explicit-capture writes (provenance="capture-explicit") flow
	// through their own seam in SCOPE-2.
	Provenance Provenance
	// FallbackCause is the trigger cause (empty for explicit
	// captures; SCOPE-1 always sets it because it only builds
	// fallback payloads).
	FallbackCause Cause
	// NormalizedText is the normalized form of the user's original
	// text. Stored explicitly so dedup queries do not have to
	// re-derive it.
	NormalizedText string
	// NormalizedTextHash is the HMAC-SHA256 of NormalizedText under
	// the SST dedup hash key.
	NormalizedTextHash string
	// DedupBucketStart is the UTC start of the dedup window bucket
	// the request occurred in.
	DedupBucketStart time.Time
	// DedupWindowSeconds is the configured dedup window in seconds
	// (denormalized for query convenience).
	DedupWindowSeconds int
	// SourceTurnID is the upstream transport message id (Telegram
	// update id, WhatsApp wamid, HTTP request id, etc.) that
	// produced this capture.
	SourceTurnID string
	// IntentTraceID joins the capture to the spec 071 IntentTrace
	// row for this turn (empty when tracing is sampled out).
	IntentTraceID string
	// AbandonedClarification is true when the capture came from a
	// SCOPE-4 abandonment sweep (clarify_abandoned cause) — kept
	// distinct from FallbackCause for query convenience even though
	// they covary.
	AbandonedClarification bool
	// AlreadyCapturedSourceID is set on dedup-hit rows to the
	// artifact id of the previously-captured Idea.
	AlreadyCapturedSourceID string
	// SchemaVersion is the persisted metadata schema version.
	SchemaVersion int
	// CreatedAt is the UTC capture timestamp.
	CreatedAt time.Time
}

// BuildCapturePayload constructs a CapturePayload from a Decision and
// the originating Request. By construction the payload carries no
// inferred organization metadata — there is no parameter, hook, or
// return-shape extension point for tags/topics/categories at this
// boundary.
func BuildCapturePayload(req Request, dec Decision, artifactID string) CapturePayload {
	return CapturePayload{
		ArtifactID:             artifactID,
		UserID:                 req.UserID,
		Provenance:             dec.Provenance,
		FallbackCause:          dec.Cause,
		NormalizedText:         dec.NormalizedText,
		NormalizedTextHash:     dec.NormalizedTextHash,
		DedupBucketStart:       dec.DedupBucketStart,
		DedupWindowSeconds:     int(dec.DedupWindow / time.Second),
		SourceTurnID:           req.TransportMessageID,
		IntentTraceID:          req.IntentTraceID,
		AbandonedClarification: req.AbandonedClarification,
		SchemaVersion:          dec.SchemaVersion,
		CreatedAt:              dec.OccurredAt.UTC(),
	}
}
