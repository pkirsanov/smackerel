// Spec 038 Scope 7 — Telegram bridge for the Retrieval Service.
//
// DriveRetrieveBridge wraps retrieve.Service so the Telegram bot can
// answer file-retrieval prompts ("send me the Lisbon boarding pass")
// against the user's drive corpus and reply with the file bytes,
// secure link, provider link, refusal, or disambiguation list — all
// under the same policy contract the Save Rules use (BS-025: sensitive
// content NEVER leaves Smackerel as raw bytes through Telegram).
//
// The bridge owns no state, holds no timers, and delegates every
// behavioural decision to retrieve.Service. Construction is dependency-
// injected so unit tests can pass in fakes; production wiring is in
// cmd/core/wiring.go.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/drive/retrieve"
)

// DriveRetrieveBridge is the Telegram-facing wrapper for the Retrieval
// Service. Use SetDriveRetrieveBridge on the Bot to attach.
type DriveRetrieveBridge struct {
	svc *retrieve.Service
}

// NewDriveRetrieveBridge constructs the bridge. svc is required; passing
// nil produces an explicit panic at startup so the runtime fails loud.
func NewDriveRetrieveBridge(svc *retrieve.Service) *DriveRetrieveBridge {
	if svc == nil {
		panic("telegram: NewDriveRetrieveBridge requires retrieve.Service")
	}
	return &DriveRetrieveBridge{svc: svc}
}

// RetrieveDriveFile runs a Telegram-channel retrieval for the supplied
// query (and optional selected_artifact_id when the user picks from a
// disambiguation list). The returned RetrieveDelivery carries the
// channel-appropriate payload: bytes (only when policy + size cap
// allow), secure_link/provider_link, refusal, or disambiguation
// candidates.
func (b *DriveRetrieveBridge) RetrieveDriveFile(
	ctx context.Context,
	userID string,
	query string,
	selectedArtifactID string,
) (retrieve.RetrieveDelivery, error) {
	if b == nil || b.svc == nil {
		return retrieve.RetrieveDelivery{}, errors.New("telegram: drive retrieve bridge not configured")
	}
	if strings.TrimSpace(query) == "" {
		return retrieve.RetrieveDelivery{}, errors.New("telegram: RetrieveDriveFile: query required")
	}
	return b.svc.Retrieve(ctx, retrieve.RetrieveRequest{
		Channel:            retrieve.ChannelTelegram,
		UserID:             userID,
		Query:              query,
		SelectedArtifactID: selectedArtifactID,
	})
}

// FormatRetrieveReply renders the Telegram reply text for a delivery.
// Public so unit tests can pin wording without instantiating a real bot.
//
// The reply is intentionally plain text:
//   - bytes      → caller sends the bytes; reply text describes the file.
//   - secure_link/provider_link → reply text contains the URL + reason.
//   - refused    → reply text explains why the file cannot be sent.
//   - disambig   → reply text lists each candidate with title, folder,
//     provider, and sensitivity (the four labels users need to pick).
func FormatRetrieveReply(delivery retrieve.RetrieveDelivery) string {
	switch delivery.Mode {
	case retrieve.ModeBytes:
		title := delivery.Title
		if title == "" {
			title = "drive file"
		}
		return fmt.Sprintf("📎 Sending %s", title)
	case retrieve.ModeSecureLink:
		base := "🔒 This file is sensitive — opening through a secure link instead of sending it as bytes."
		if delivery.Title != "" {
			base = fmt.Sprintf("🔒 %s is sensitive — opening through a secure link instead of sending it as bytes.", delivery.Title)
		}
		if delivery.URL != "" {
			base += "\n" + delivery.URL
		}
		if delivery.PolicyReason != "" {
			base += "\nReason: " + delivery.PolicyReason
		}
		return base
	case retrieve.ModeProviderLink:
		base := "🔗 The file is too large to send through Telegram — opening it on the provider instead."
		if delivery.Title != "" {
			base = fmt.Sprintf("🔗 %s is too large to send through Telegram — opening it on the provider instead.", delivery.Title)
		}
		if delivery.URL != "" {
			base += "\n" + delivery.URL
		}
		return base
	case retrieve.ModeRefused:
		base := "⚠️ Refused: the requested file cannot be sent through Telegram."
		if delivery.PolicyReason != "" {
			base = "⚠️ " + delivery.PolicyReason
		}
		if delivery.Hint != "" {
			base += "\n" + delivery.Hint
		}
		return base
	case retrieve.ModeDisambiguate:
		var sb strings.Builder
		sb.WriteString("🔍 Multiple matches — pick one:\n")
		for i, c := range delivery.Candidates {
			fmt.Fprintf(&sb, "%d. %s — folder: %s — provider: %s — sensitivity: %s\n",
				i+1,
				c.Title,
				c.Folder,
				c.Provider,
				c.Sensitivity,
			)
		}
		return strings.TrimRight(sb.String(), "\n")
	default:
		return "⚠️ The retrieval service returned no result."
	}
}
