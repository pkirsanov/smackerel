// Package retrieve implements the Spec 038 Scope 7 Retrieval Service.
//
// The Retrieval Service answers user queries that ask for a previously
// captured drive file ("send me the Lisbon boarding pass") through a
// channel adapter (Telegram bot, scenario-agent tool). It is the single
// point that:
//
//   - Searches the artifact store for drive_file candidates.
//   - Applies sensitivity policy via internal/drive/policy: sensitive
//     content NEVER leaves Smackerel as bytes through Telegram (BS-025);
//     the engine downgrades to a secure_link, refuses outright, or — for
//     non-sensitive content — falls through to bytes/provider-link.
//   - Downgrades non-sensitive bytes to provider_link when the file is
//     larger than the configured Telegram inline cap.
//   - Returns disambiguation candidates when more than one drive file
//     matches the query, so the channel adapter can prompt the user to
//     pick the intended file.
//   - Sources every refusal/downgrade reason from a single localized
//     ReasonTable; channel adapters do not invent prose.
//
// Design anchors:
//   - SCN-038-019 — Telegram retrieves a policy-allowed Drive file.
//   - SCN-038-020 — Sensitive retrieval never sends bytes over Telegram.
//   - design.md §6 Retrieval Service.
package retrieve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	driveobs "github.com/smackerel/smackerel/internal/drive/observability"
	"github.com/smackerel/smackerel/internal/drive/policy"
)

// Channel names the surface a retrieval request originated from. The
// Telegram channel applies the Telegram-specific size cap and the
// SurfaceRetrieval policy decision table; future channels (web,
// in-app PWA) reuse the same machinery with their own cap and any
// channel-specific policy overrides.
type Channel string

const (
	// ChannelTelegram identifies retrieval over the Telegram bot.
	ChannelTelegram Channel = "telegram"
)

// Validate reports whether the channel value is recognized.
func (c Channel) Validate() error {
	switch c {
	case ChannelTelegram:
		return nil
	}
	return fmt.Errorf("retrieve: unknown channel %q", c)
}

// Mode is the delivery instruction the channel adapter MUST honor.
type Mode string

const (
	// ModeBytes — the channel may send the raw artifact bytes to the user.
	ModeBytes Mode = "bytes"
	// ModeSecureLink — sensitive content; the channel MUST send a
	// secure link the user opens from a trusted device. Never bytes.
	ModeSecureLink Mode = "secure_link"
	// ModeProviderLink — the channel MUST send the provider's deep
	// link. Used when the file is too large for inline byte delivery
	// or when the policy engine downgrades non-sensitive content.
	ModeProviderLink Mode = "provider_link"
	// ModeRefused — retrieval is forbidden; the channel MUST surface
	// PolicyReason / Hint without inventing alternative prose.
	ModeRefused Mode = "refused"
	// ModeDisambiguate — multiple candidates match; the channel MUST
	// present the candidate list and route the user's pick back into
	// Service.Retrieve.
	ModeDisambiguate Mode = "disambiguate"
)

// RetrieveRequest is the input to Service.Retrieve.
type RetrieveRequest struct {
	// Channel identifies the surface invoking retrieval; required.
	Channel Channel
	// UserID is the surface-specific user identifier (Telegram chat
	// id, web session subject, etc.). Audited, never trusted to make
	// authorization decisions on its own.
	UserID string
	// Query is the user-supplied free-text request.
	Query string
	// AllowedClassif optionally narrows the search to classifications
	// the caller is willing to accept (e.g. ["receipt", "boarding_pass"]).
	AllowedClassif []string
	// Limit caps the candidate count returned for disambiguation.
	// 0 falls back to DefaultLimit.
	Limit int
	// SelectedArtifactID short-circuits the search step when the
	// caller has already picked a candidate via a previous
	// ModeDisambiguate response. The service still re-applies the
	// policy and size checks against the named candidate.
	SelectedArtifactID string
}

// RetrieveCandidate is one drive-file match returned to the channel.
// Provider-neutral identifiers ONLY; channel adapters MUST NOT branch
// on Provider.
type RetrieveCandidate struct {
	ArtifactID  string
	Title       string
	Folder      string
	Sensitivity string
	SizeBytes   int64
	Provider    string
	ProviderURL string
}

// RetrieveDelivery is the structured outcome the channel adapter
// renders to the user. The PolicyReason and Hint fields source from
// ReasonTable so the channel does not invent prose (design.md §6
// "Refusal text is sourced from a single localized table").
type RetrieveDelivery struct {
	Mode         Mode
	URL          string
	Bytes        []byte
	MimeType     string
	Title        string
	Sensitivity  string
	PolicyReason string
	Hint         string
	Candidates   []RetrieveCandidate
}

// DefaultLimit caps disambiguation candidates returned to the channel.
const DefaultLimit = 5

// Searcher returns drive-file candidates that match the request. The
// production implementation queries Postgres; tests substitute a fake.
// Implementations MUST honor RetrieveRequest.AllowedClassif when
// non-empty and MUST return at most RetrieveRequest.Limit results.
type Searcher interface {
	SearchDrive(ctx context.Context, req RetrieveRequest) ([]RetrieveCandidate, error)
}

// BytesFetcher fetches the raw artifact bytes for a successful
// non-sensitive retrieval. Implementations MUST consult the configured
// drive provider via the existing drive.Provider abstraction so this
// package never branches on provider type.
type BytesFetcher interface {
	GetArtifactBytes(ctx context.Context, artifactID string) (data []byte, mime string, err error)
}

// ReasonTable owns every user-facing refusal/downgrade reason.
// Channel adapters render Hint verbatim and pair PolicyReason with
// the localized text in their UX layer.
type ReasonTable struct {
	NoMatch          string
	SensitiveRefusal string
	SensitiveLink    string
	OversizeLink     string
	Disambiguate     string
}

// DefaultReasonTable returns the english reason text used by the
// Telegram channel. Translations land here, not in the channel layer.
func DefaultReasonTable() ReasonTable {
	return ReasonTable{
		NoMatch:          "No drive files matched that request. Try a folder name or a phrase from the file.",
		SensitiveRefusal: "That file is sensitive — Smackerel will not deliver it through Telegram.",
		SensitiveLink:    "That file is sensitive — opening it requires a secure link from a trusted device.",
		OversizeLink:     "That file is too large to send inline; opening via the provider link instead.",
		Disambiguate:     "Multiple drive files matched. Reply with the number of the file you want.",
	}
}

// ErrServiceNotConfigured is returned when Retrieve is called against a
// zero-value Service. Callers MUST construct via NewService.
var ErrServiceNotConfigured = errors.New("retrieve: service not configured")

// Service coordinates search, policy, and bytes delivery for the
// retrieval flow. Construct one per process; safe for concurrent use.
type Service struct {
	searcher  Searcher
	fetcher   BytesFetcher
	policy    *policy.Engine
	maxInline int64
	reasons   ReasonTable
}

// NewService constructs the Retrieval Service.
//
//   - searcher    — drive-file candidate source (required).
//   - fetcher     — bytes source for non-sensitive deliveries (required).
//   - engine      — sensitivity policy engine (required); use
//     policy.NewEngine() in tests.
//   - maxInline   — Telegram inline byte cap (DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES).
//   - reasons     — localized reason table; pass DefaultReasonTable()
//     when you have no localization layer.
func NewService(searcher Searcher, fetcher BytesFetcher, engine *policy.Engine, maxInline int64, reasons ReasonTable) *Service {
	if searcher == nil || fetcher == nil || engine == nil {
		panic("retrieve.NewService: searcher, fetcher, and policy engine are required")
	}
	if maxInline <= 0 {
		panic("retrieve.NewService: maxInline must be positive")
	}
	return &Service{
		searcher:  searcher,
		fetcher:   fetcher,
		policy:    engine,
		maxInline: maxInline,
		reasons:   reasons,
	}
}

// Retrieve executes the §6 flow against the supplied request.
//
//  1. Search() runs against the configured Searcher.
//  2. Zero candidates → ModeRefused with NoMatch reason.
//  3. Multiple candidates → ModeDisambiguate with the candidate list.
//  4. Single candidate → policy + size check, then bytes / link / refusal.
func (s *Service) Retrieve(ctx context.Context, req RetrieveRequest) (RetrieveDelivery, error) {
	if s == nil || s.searcher == nil || s.fetcher == nil || s.policy == nil {
		return RetrieveDelivery{}, ErrServiceNotConfigured
	}
	if err := req.Channel.Validate(); err != nil {
		return RetrieveDelivery{}, err
	}
	if strings.TrimSpace(req.Query) == "" && req.SelectedArtifactID == "" {
		return RetrieveDelivery{}, errors.New("retrieve: query or selected_artifact_id required")
	}
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}

	candidates, err := s.searcher.SearchDrive(ctx, req)
	if err != nil {
		return RetrieveDelivery{}, fmt.Errorf("retrieve: search: %w", err)
	}

	if req.SelectedArtifactID != "" {
		picked, ok := findCandidate(candidates, req.SelectedArtifactID)
		if !ok {
			return RetrieveDelivery{
				Mode:         ModeRefused,
				PolicyReason: "selection_not_found",
				Hint:         s.reasons.NoMatch,
			}, nil
		}
		return s.deliverOne(ctx, picked)
	}

	switch len(candidates) {
	case 0:
		return RetrieveDelivery{
			Mode:         ModeRefused,
			PolicyReason: "no_match",
			Hint:         s.reasons.NoMatch,
		}, nil
	case 1:
		return s.deliverOne(ctx, candidates[0])
	default:
		return RetrieveDelivery{
			Mode:       ModeDisambiguate,
			Hint:       s.reasons.Disambiguate,
			Candidates: candidates,
		}, nil
	}
}

func (s *Service) deliverOne(ctx context.Context, cand RetrieveCandidate) (RetrieveDelivery, error) {
	sensitivity := policy.Sensitivity(cand.Sensitivity)
	if !policy.IsKnownSensitivity(string(sensitivity)) {
		sensitivity = policy.SensitivityNone
	}

	deliveryMode := "bytes"
	if cand.SizeBytes > 0 && cand.SizeBytes > s.maxInline {
		deliveryMode = "provider_link"
	}

	verdict, err := s.policy.Evaluate(policy.Action{
		Surface:      policy.SurfaceRetrieval,
		Sensitivity:  sensitivity,
		DeliveryMode: deliveryMode,
	})
	if err != nil {
		return RetrieveDelivery{}, fmt.Errorf("retrieve: policy evaluate: %w", err)
	}

	base := RetrieveDelivery{
		Title:        cand.Title,
		Sensitivity:  cand.Sensitivity,
		Candidates:   []RetrieveCandidate{cand},
		PolicyReason: verdict.Reason,
	}

	switch verdict.Decision {
	case policy.DecisionRefuse:
		base.Mode = ModeRefused
		base.Hint = s.reasons.SensitiveRefusal
		recordRetrieveDecision(cand, base.Mode)
		return base, nil

	case policy.DecisionDowngrade:
		switch verdict.DowngradeMode {
		case policy.DowngradeSecureLink:
			base.Mode = ModeSecureLink
			base.URL = cand.ProviderURL
			base.Hint = s.reasons.SensitiveLink
			recordRetrieveDecision(cand, base.Mode)
			return base, nil
		case policy.DowngradeProviderLink:
			base.Mode = ModeProviderLink
			base.URL = cand.ProviderURL
			base.Hint = s.reasons.OversizeLink
			recordRetrieveDecision(cand, base.Mode)
			return base, nil
		default:
			// Defensive: unknown downgrade mode falls through to refusal
			// rather than silently delivering bytes.
			base.Mode = ModeRefused
			base.PolicyReason = "unknown_downgrade_mode"
			base.Hint = s.reasons.SensitiveRefusal
			recordRetrieveDecision(cand, base.Mode)
			return base, nil
		}

	case policy.DecisionAllow:
		// Non-sensitive content past the inline cap was pre-downgraded
		// to provider_link before evaluation; honor that downgrade now.
		if deliveryMode == "provider_link" {
			base.Mode = ModeProviderLink
			base.URL = cand.ProviderURL
			base.PolicyReason = "size_exceeds_inline_limit"
			base.Hint = s.reasons.OversizeLink
			recordRetrieveDecision(cand, base.Mode)
			return base, nil
		}
		bytes, mime, err := s.fetcher.GetArtifactBytes(ctx, cand.ArtifactID)
		if err != nil {
			driveobs.DriveProviderErrors.WithLabelValues(providerLabel(cand.Provider), "retrieve").Inc()
			slog.Warn("drive retrieve: fetcher failed",
				"provider", cand.Provider, "artifact_id", cand.ArtifactID, "error", err,
			)
			return RetrieveDelivery{}, fmt.Errorf("retrieve: fetch bytes: %w", err)
		}
		base.Mode = ModeBytes
		base.Bytes = bytes
		base.MimeType = mime
		base.PolicyReason = "allowed"
		recordRetrieveDecision(cand, base.Mode)
		return base, nil

	default:
		base.Mode = ModeRefused
		base.PolicyReason = "policy_unspecified"
		base.Hint = s.reasons.SensitiveRefusal
		return base, nil
	}
}

func findCandidate(list []RetrieveCandidate, id string) (RetrieveCandidate, bool) {
	for _, c := range list {
		if c.ArtifactID == id {
			return c, true
		}
	}
	return RetrieveCandidate{}, false
}

// recordRetrieveDecision increments the provider-neutral retrieval-decision
// counter (label provider + mode). The provider label collapses to "unknown"
// when the candidate row lacks a provider tag.
func recordRetrieveDecision(cand RetrieveCandidate, mode Mode) {
	driveobs.DriveRetrieveDecisions.WithLabelValues(providerLabel(cand.Provider), string(mode)).Inc()
	slog.Info("drive retrieve: decision",
		"provider", providerLabel(cand.Provider),
		"artifact_id", cand.ArtifactID,
		"mode", string(mode),
		"sensitivity", cand.Sensitivity,
	)
}

func providerLabel(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "unknown"
	}
	return p
}
