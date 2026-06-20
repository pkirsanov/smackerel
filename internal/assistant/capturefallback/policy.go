// Package capturefallback implements the spec 074 SCOPE-1 transport-
// neutral capture-as-fallback policy.
//
// Capture-as-fallback is the runtime guarantee that when no
// user-facing scenario handles a turn, the user's thought is preserved
// as an Idea artifact without asking them to organize it. This package
// owns:
//
//   - the closed Cause and Provenance vocabularies (no out-of-band
//     suppression states),
//   - Policy.Decide — the trigger-eligibility contract,
//   - Policy.Capture — the dedup + write contract (consumers in
//     SCOPE-2..4 supply the store and writer implementations),
//   - the nfkc_casefold_ws_v1 normalization contract and HMAC-SHA256
//     hash derivation that scope dedup lookups.
//
// SCOPE-1 ships contracts and pure-functional building blocks only.
// Facade trigger execution lives in SCOPE-4; provenance metadata
// persistence lives in SCOPE-2; per-user dedup semantics live in
// SCOPE-3; telemetry/IntentTrace links live in SCOPE-5. The package
// MUST NOT be imported by facade trigger or capture-writer paths until
// the SCOPE-2..4 work lands.
//
// Inviolability invariant (SCN-074-A09): no public symbol in this
// package, no SST key in config/smackerel.yaml, and no env var in the
// generated bundle suppresses capture for an eligible turn. The
// inviolable_static_test.go file enforces this mechanically.
package capturefallback

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Cause is the closed vocabulary of fallback-eligible trigger causes.
// Adding a new value here is a deliberate spec change — the eligibility
// surface MUST stay closed so transports, the open-knowledge agent,
// the compiler, and the abandonment sweep cannot smuggle in suppression
// branches via "unknown" cause values.
type Cause string

const (
	CauseUnrouted              Cause = "unrouted"
	CauseOpenKnowledgeNoGround Cause = "open_knowledge_no_ground"
	CauseClarifyAbandoned      Cause = "clarify_abandoned"
	CauseCompilerError         Cause = "compiler_error"
)

// AllCauses is the closed Cause vocabulary used by the static
// inviolability guard (TP-074-02) and by Decide for membership checks.
var AllCauses = []Cause{
	CauseUnrouted,
	CauseOpenKnowledgeNoGround,
	CauseClarifyAbandoned,
	CauseCompilerError,
}

// validCauses indexes AllCauses for O(1) membership tests.
var validCauses = func() map[Cause]struct{} {
	m := make(map[Cause]struct{}, len(AllCauses))
	for _, c := range AllCauses {
		m[c] = struct{}{}
	}
	return m
}()

// Provenance is the closed vocabulary of capture provenances. Spec 008
// explicit captures use ProvenanceExplicit; capture-as-fallback writes
// use ProvenanceFallback. The two MUST NEVER dedup together (SCN-074-A02
// — enforced by SCOPE-2's dedup key composition).
type Provenance string

const (
	ProvenanceFallback Provenance = "capture-as-fallback"
	ProvenanceExplicit Provenance = "capture-explicit"
)

// AllProvenances is the closed Provenance vocabulary.
var AllProvenances = []Provenance{ProvenanceFallback, ProvenanceExplicit}

// SchemaVersion is the persisted capture-policy metadata schema
// version emitted into artifact_capture_policy.schema_version. v1.
const SchemaVersion = 1

// Request describes a fallback-eligible turn presented to Policy.Decide.
// All fields are required except IntentTraceID (empty when the spec 071
// IntentTrace recorder is disabled for this turn) and OccurredAt
// (defaults to time.Now in Decide when zero, captured here so callers
// pass an explicit time when replaying recorded traces).
type Request struct {
	UserID                 string
	Transport              string
	TransportMessageID     string
	OriginalText           string
	Cause                  Cause
	TraceID                string
	IntentTraceID          string
	AbandonedClarification bool
	OccurredAt             time.Time
}

// Decision is the output of Policy.Decide. It is intentionally minimal
// and contains NO suppression field: when Decide returns nil error,
// the caller MUST proceed to Capture. The Capture decision is recorded
// by the absence of a "skip" path — adding one would violate SCN-074-A09.
type Decision struct {
	Cause              Cause
	Provenance         Provenance
	NormalizedText     string
	NormalizedTextHash string
	DedupBucketStart   time.Time
	DedupWindow        time.Duration
	SchemaVersion      int
	OccurredAt         time.Time
	// Turn-context carried through from Request so Policy.Capture
	// can persist the artifact_capture_policy row without needing the
	// caller to thread the original Request a second time. These are
	// metadata fields, not suppression switches (SCN-074-A09).
	SourceTurnID           string
	IntentTraceID          string
	AbandonedClarification bool
}

// CaptureResult is the output of Policy.Capture (SCOPE-2..4 wire the
// real artifact-id / already-captured plumbing). Defined here so the
// contract is fixed at SCOPE-1 boundary and downstream scopes cannot
// reshape it.
type CaptureResult struct {
	IdeaArtifactID          string
	AlreadyCaptured         bool
	AlreadyCapturedSourceID string
	Decision                Decision
}

// Errors returned by Decide. These are the ONLY shapes Decide may
// return — none of them is a "suppress capture" signal.
var (
	// ErrInvalidCause means the caller passed a Cause outside the
	// closed vocabulary. The correct fix is to add the cause to
	// AllCauses with a spec amendment, not to swallow the error.
	ErrInvalidCause = errors.New("capturefallback: cause not in closed vocabulary")
	// ErrMissingUser means Request.UserID is empty. The capture
	// path is per-user; an empty user is a programming error in the
	// caller, not a suppression signal.
	ErrMissingUser = errors.New("capturefallback: request missing UserID")
	// ErrMissingText means Request.OriginalText is empty/whitespace.
	// Capture is only meaningful with real user text.
	ErrMissingText = errors.New("capturefallback: request missing OriginalText")
)

// Config bundles the SST values Policy needs. Mirrors
// internal/config.CaptureFallbackConfig but defined here so the
// package has no upward import of internal/config (avoids an import
// cycle when SCOPE-4 wires the facade).
type Config struct {
	DedupWindow         time.Duration
	NormalizationPolicy string
	DedupHashKey        string
}

// Policy is the foundation-owned interface consumed by the facade,
// compiler, open-knowledge agent, and abandoned-clarification sweep.
// The two methods MUST stay free of suppression branches.
type Policy interface {
	// Decide validates eligibility, normalizes text, computes the
	// dedup hash + bucket, and returns a Decision. There is no
	// "skip capture" return path: a non-nil error means the caller
	// presented a malformed Request, NOT that capture is disabled.
	Decide(ctx context.Context, req Request) (Decision, error)

	// Capture executes the dedup lookup, writes the Idea (or links
	// an existing one), and returns the CaptureResult. SCOPE-2..4
	// supply the concrete store/writer implementations.
	//
	// Capture without a user identity is invalid (SCN-074-A05 forbids
	// cross-user dedup); callers MUST use CaptureForUser. Capture
	// returns ErrMissingUser to surface programming errors.
	Capture(ctx context.Context, dec Decision) (CaptureResult, error)

	// CaptureForUser is the per-user dispatch path used by SCOPE-4's
	// facade. SCOPE-3 wired this so dedup is always per-user.
	CaptureForUser(ctx context.Context, userID string, dec Decision) (CaptureResult, error)
}

// DedupStore is the SCOPE-3-owned per-user dedup index. SCOPE-1
// defines the interface so the contract is fixed; the postgres-backed
// concrete implementation ships in SCOPE-3.
type DedupStore interface {
	Lookup(ctx context.Context, userID string, dec Decision) (existingArtifactID string, hit bool, err error)
	Record(ctx context.Context, userID, artifactID string, dec Decision) error
}

// IdeaWriter is the SCOPE-2/SCOPE-4 capture seam that creates the
// underlying Idea artifact. SCOPE-1 defines the interface; downstream
// scopes wire it to the existing capture/pipeline path so a fallback
// Idea is structurally identical to a spec 008 explicit-capture Idea
// (different only in provenance metadata).
type IdeaWriter interface {
	WriteIdea(ctx context.Context, userID, normalizedText string, dec Decision) (artifactID string, err error)
}

// IdeaCleaner is an optional interface that IdeaWriter implementations
// can implement to support cleanup of orphaned artifacts. When Record
// fails after WriteIdea succeeds, captureForUser will attempt cleanup
// if the writer implements this interface. This follows the compensating
// transaction pattern established in internal/pipeline/ingest.go.
type IdeaCleaner interface {
	DeleteIdea(ctx context.Context, artifactID string) error
}

// defaultPolicy is the SCOPE-1 reference implementation of Policy.
// Decide is fully implemented (pure-functional, no IO). Capture
// requires SCOPE-2/SCOPE-3 collaborators; SCOPE-1 ships the wiring
// shape so SCOPE-4 has a stable seam.
type defaultPolicy struct {
	cfg    Config
	store  DedupStore
	writer IdeaWriter
	now    func() time.Time
}

// New constructs a Policy with the given SST config and downstream
// collaborators. store and writer may be nil in SCOPE-1 unit tests
// that only exercise Decide; Capture returns ErrNotWired in that case.
func New(cfg Config, store DedupStore, writer IdeaWriter) (Policy, error) {
	if cfg.DedupWindow <= 0 {
		return nil, fmt.Errorf("capturefallback: invalid cfg.DedupWindow %s", cfg.DedupWindow)
	}
	if cfg.NormalizationPolicy != NormalizationPolicyV1 {
		return nil, fmt.Errorf("capturefallback: unsupported normalization policy %q (only %q is implemented in v1)", cfg.NormalizationPolicy, NormalizationPolicyV1)
	}
	if strings.TrimSpace(cfg.DedupHashKey) == "" {
		return nil, errors.New("capturefallback: cfg.DedupHashKey is empty")
	}
	return &defaultPolicy{cfg: cfg, store: store, writer: writer, now: time.Now}, nil
}

// ErrNotWired is returned by Capture when no DedupStore/IdeaWriter is
// configured. Returning a typed error keeps SCOPE-1 honest: it is NOT
// a suppression signal — SCOPE-2/SCOPE-3/SCOPE-4 must supply real
// collaborators before Capture is invoked from the facade.
var ErrNotWired = errors.New("capturefallback: dedup store / idea writer not wired (SCOPE-2..4 must supply implementations)")

// NormalizationPolicyV1 mirrors internal/config.NormalizationPolicyV1.
// Duplicated here so the capturefallback package has no upward import
// of internal/config (avoids cycle with SCOPE-4 facade wiring).
const NormalizationPolicyV1 = "nfkc_casefold_ws_v1"

// Decide implements Policy.Decide.
func (p *defaultPolicy) Decide(_ context.Context, req Request) (Decision, error) {
	if _, ok := validCauses[req.Cause]; !ok {
		return Decision{}, fmt.Errorf("%w: %q", ErrInvalidCause, req.Cause)
	}
	if strings.TrimSpace(req.UserID) == "" {
		return Decision{}, ErrMissingUser
	}
	if strings.TrimSpace(req.OriginalText) == "" {
		return Decision{}, ErrMissingText
	}
	occurred := req.OccurredAt
	if occurred.IsZero() {
		occurred = p.now()
	}
	normalized := NormalizeV1(req.OriginalText)
	hash := HashNormalized(normalized, p.cfg.DedupHashKey)
	bucket := BucketStart(occurred, p.cfg.DedupWindow)
	return Decision{
		Cause:                  req.Cause,
		Provenance:             ProvenanceFallback,
		NormalizedText:         normalized,
		NormalizedTextHash:     hash,
		DedupBucketStart:       bucket,
		DedupWindow:            p.cfg.DedupWindow,
		SchemaVersion:          SchemaVersion,
		OccurredAt:             occurred,
		SourceTurnID:           req.TransportMessageID,
		IntentTraceID:          req.IntentTraceID,
		AbandonedClarification: req.AbandonedClarification,
	}, nil
}

// CaptureForUser executes Policy.Capture for a specific user. Capture
// itself takes only a Decision (which does not carry UserID), so the
// per-user dispatch lives on this helper. SCOPE-3 ships this so the
// facade in SCOPE-4 has a single entry point.
func (p *defaultPolicy) CaptureForUser(ctx context.Context, userID string, dec Decision) (CaptureResult, error) {
	return p.captureForUser(ctx, userID, dec)
}

// Capture implements Policy.Capture. Without a per-user identity it
// cannot enforce SCN-074-A05 (cross-user isolation); callers MUST go
// through CaptureForUser. SCOPE-4's facade wires the per-user path.
func (p *defaultPolicy) Capture(_ context.Context, _ Decision) (CaptureResult, error) {
	return CaptureResult{}, ErrMissingUser
}

// captureForUser is the SCOPE-3 implementation: lookup → write → record.
// If Record fails after WriteIdea succeeds, attempts to clean up the
// orphaned artifact following the compensating transaction pattern
// established in internal/pipeline/ingest.go (lines 124-127).
func (p *defaultPolicy) captureForUser(ctx context.Context, userID string, dec Decision) (CaptureResult, error) {
	if p.store == nil || p.writer == nil {
		return CaptureResult{}, ErrNotWired
	}
	if strings.TrimSpace(userID) == "" {
		return CaptureResult{}, ErrMissingUser
	}
	if existingID, hit, err := p.store.Lookup(ctx, userID, dec); err != nil {
		return CaptureResult{}, fmt.Errorf("capturefallback: dedup lookup: %w", err)
	} else if hit {
		return CaptureResult{
			IdeaArtifactID:          existingID,
			AlreadyCaptured:         true,
			AlreadyCapturedSourceID: existingID,
			Decision:                dec,
		}, nil
	}
	artifactID, err := p.writer.WriteIdea(ctx, userID, dec.NormalizedText, dec)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("capturefallback: write idea: %w", err)
	}
	if err := p.store.Record(ctx, userID, artifactID, dec); err != nil {
		// Clean up orphaned artifact on Record failure following the
		// compensating transaction pattern from internal/pipeline/ingest.go.
		// This prevents orphan Ideas without dedup metadata that would
		// cause duplicate captures on subsequent calls with the same text.
		if cleaner, ok := p.writer.(IdeaCleaner); ok {
			if cleanupErr := cleaner.DeleteIdea(ctx, artifactID); cleanupErr != nil {
				slog.Error("capturefallback: failed to clean up orphaned artifact after record failure",
					slog.String("artifact_id", artifactID),
					slog.String("user_id", userID),
					slog.String("cleanup_error", cleanupErr.Error()),
					slog.String("record_error", err.Error()),
				)
			} else {
				slog.Warn("capturefallback: cleaned up orphaned artifact after record failure",
					slog.String("artifact_id", artifactID),
					slog.String("user_id", userID),
				)
			}
		} else {
			// Writer does not implement IdeaCleaner; log the orphan for
			// manual cleanup/auditing.
			slog.Error("capturefallback: orphaned artifact created (writer does not support cleanup)",
				slog.String("artifact_id", artifactID),
				slog.String("user_id", userID),
				slog.String("record_error", err.Error()),
			)
		}
		return CaptureResult{}, fmt.Errorf("capturefallback: record dedup: %w", err)
	}
	return CaptureResult{
		IdeaArtifactID:  artifactID,
		AlreadyCaptured: false,
		Decision:        dec,
	}, nil
}

// NormalizeV1 implements the nfkc_casefold_ws_v1 normalization policy.
// Steps:
//  1. NFKC compatibility composition (collapses width/compatibility
//     variants e.g. fullwidth digits → ascii digits).
//  2. Unicode case folding (Go's strings.ToLower is a best-effort
//     casefold for the BMP; sufficient for v1).
//  3. Whitespace collapse: any run of unicode.IsSpace runes → single
//     ASCII space; leading/trailing whitespace trimmed.
//
// Versioned in the function name AND in the policy constant so a
// future v2 (e.g. punctuation-stripping) is an additive, opt-in change
// that does not retroactively merge existing dedup history.
func NormalizeV1(s string) string {
	if s == "" {
		return ""
	}
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// HashNormalized derives the hex-encoded HMAC-SHA256 of the normalized
// text under the configured dedup hash key. The key is required to be
// non-empty by Config validation; an empty key here is a programmer
// error and produces a deliberately-distinguishable hash so test
// failures point at the wiring bug rather than at a silent collision.
func HashNormalized(normalized, hashKey string) string {
	if hashKey == "" {
		// Distinguishable sentinel; surfaces in test failures
		// instead of silently colliding with a real hash.
		return "ERR_EMPTY_HASH_KEY"
	}
	mac := hmac.New(sha256.New, []byte(hashKey))
	mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// BucketStart returns the UTC start of the dedup bucket containing t.
// Buckets are aligned to the Unix epoch with width = window. Aligning
// to a fixed epoch (rather than per-user) is what gives SCN-074-A03
// its "same window" semantics across two messages from the same user.
func BucketStart(t time.Time, window time.Duration) time.Time {
	if window <= 0 {
		return t.UTC()
	}
	utc := t.UTC()
	w := window.Nanoseconds()
	bucket := utc.UnixNano() / w * w
	return time.Unix(0, bucket).UTC()
}
