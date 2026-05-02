// Package consumers exposes the spec 038 Scope 8 provider-neutral
// downstream-consumer surface. Downstream features (recipes, expenses,
// lists, annotations, meal planning, digest, domain extraction, agent
// tools) MUST consume drive artifacts through this package — never by
// importing internal/drive/google or any other provider-specific
// package directly.
//
// The package intentionally exports just two things:
//
//   - LoadDriveArtifact: a single read helper that returns the
//     provider-neutral drive metadata + extracted text + classification
//     a downstream consumer needs (sensitivity tier, sharing audience,
//     folder breadcrumb, provider URL, summary, content_raw). The shape
//     is derivable from drive_files JOIN artifacts so it works for any
//     drive provider that landed through the scan service.
//
//   - DriveArtifactSummary: the result struct, with stable JSON-tagged
//     fields downstream consumers can serialize without re-deriving
//     the wire shape per call site.
//
// The package contract is enforced mechanically by
// consumer_contract_test.go which scans the source files of every
// downstream package and fails the build if any of them imports a
// drive-provider-specific package.
package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotDriveArtifact is returned by LoadDriveArtifact when the requested
// artifactID exists but its artifact_type is not "drive_file". Callers
// SHOULD treat this as a routing error rather than a fatal failure: the
// artifact was real, it was just sourced through a non-drive ingestion
// path. Downstream features that branch on artifact source SHOULD use
// errors.Is to detect this case.
var ErrNotDriveArtifact = errors.New("consumers: artifact is not a drive_file")

// ErrDriveArtifactNotFound is returned by LoadDriveArtifact when no
// matching artifact row exists. Callers SHOULD treat this as a 404 in
// HTTP surfaces.
var ErrDriveArtifactNotFound = errors.New("consumers: drive artifact not found")

// DriveArtifactSummary is the provider-neutral view downstream consumers
// see when they call LoadDriveArtifact. Every field is derivable from
// drive_files JOIN artifacts and works identically for every registered
// drive provider.
//
// Tombstoned/PermissionLost artifacts are still surfaced with their
// metadata so consumers can render "knowledge available, bytes
// unavailable" badges; the IsAvailable boolean flips off and ProviderURL
// stays populated for "Open in Drive" links.
type DriveArtifactSummary struct {
	ArtifactID       string   `json:"artifact_id"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	ExtractedText    string   `json:"extracted_text"`
	ProviderID       string   `json:"provider_id"`
	ProviderURL      string   `json:"provider_url"`
	FolderBreadcrumb []string `json:"folder_breadcrumb"`
	SharingState     string   `json:"sharing_state"`
	SharingAudience  string   `json:"sharing_audience,omitempty"`
	Sensitivity      string   `json:"sensitivity"`
	Classification   string   `json:"classification,omitempty"`
	OwnerLabel       string   `json:"owner_label,omitempty"`
	MimeType         string   `json:"mime_type,omitempty"`
	IsAvailable      bool     `json:"is_available"`
	Tombstoned       bool     `json:"tombstoned"`
	PermissionLost   bool     `json:"permission_lost"`
}

// LoadDriveArtifact returns a provider-neutral DriveArtifactSummary for
// the given artifactID. Downstream features call this helper to avoid
// re-implementing the drive_files JOIN artifacts SQL and to keep their
// import surface free of provider-specific packages.
//
// Errors:
//
//	ErrDriveArtifactNotFound — no row in artifacts matched artifactID.
//	ErrNotDriveArtifact      — artifact exists but artifact_type is
//	                           not "drive_file".
//	other errors             — propagated verbatim from the database
//	                           driver.
func LoadDriveArtifact(ctx context.Context, pool *pgxpool.Pool, artifactID string) (DriveArtifactSummary, error) {
	if pool == nil {
		return DriveArtifactSummary{}, errors.New("consumers: nil pool")
	}
	if strings.TrimSpace(artifactID) == "" {
		return DriveArtifactSummary{}, errors.New("consumers: artifactID required")
	}

	row := pool.QueryRow(ctx, `
		SELECT a.id, a.artifact_type, a.title, COALESCE(a.summary, ''),
		       COALESCE(a.content_raw, ''), COALESCE(a.metadata, '{}'::jsonb),
		       f.provider_url, f.folder_path, f.sharing_state, f.sensitivity,
		       f.owner_label, f.mime_type,
		       (f.tombstoned_at IS NOT NULL) AS tombstoned,
		       (f.permission_lost_at IS NOT NULL) AS permission_lost
		  FROM artifacts a
		  LEFT JOIN drive_files f ON f.artifact_id = a.id
		 WHERE a.id = $1
		 LIMIT 1
	`, artifactID)

	var (
		gotID, gotType, title, summary, content string
		metadata                                []byte
		providerURL                             *string
		folderPath                              []string
		sharingState                            []byte
		sensitivity                             *string
		ownerLabel, mimeType                    *string
		tombstoned, permissionLost              bool
	)
	if err := row.Scan(&gotID, &gotType, &title, &summary, &content, &metadata,
		&providerURL, &folderPath, &sharingState, &sensitivity,
		&ownerLabel, &mimeType, &tombstoned, &permissionLost); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DriveArtifactSummary{}, ErrDriveArtifactNotFound
		}
		return DriveArtifactSummary{}, fmt.Errorf("consumers: load drive artifact: %w", err)
	}
	if gotType != "drive_file" {
		return DriveArtifactSummary{}, ErrNotDriveArtifact
	}
	if providerURL == nil {
		// drive_files row missing — drive_file artifact_type without
		// drive_files is a schema invariant violation, but we surface
		// it as ErrDriveArtifactNotFound so consumers don't render a
		// half-initialised summary.
		return DriveArtifactSummary{}, ErrDriveArtifactNotFound
	}

	sharingLabel, audience := decodeSharingState(sharingState)
	classification := decodeClassification(metadata)
	providerID := decodeProviderID(metadata)

	out := DriveArtifactSummary{
		ArtifactID:       gotID,
		Title:            title,
		Summary:          summary,
		ExtractedText:    content,
		ProviderID:       providerID,
		ProviderURL:      stringPtr(providerURL),
		FolderBreadcrumb: append([]string{}, folderPath...),
		SharingState:     sharingLabel,
		SharingAudience:  audience,
		Sensitivity:      stringPtr(sensitivity),
		Classification:   classification,
		OwnerLabel:       stringPtr(ownerLabel),
		MimeType:         stringPtr(mimeType),
		IsAvailable:      !tombstoned && !permissionLost,
		Tombstoned:       tombstoned,
		PermissionLost:   permissionLost,
	}
	return out, nil
}

func stringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// decodeSharingState collapses the provider-neutral sharing_state JSONB
// into the same compact label set the API search surface uses (private,
// shared, shared_audience, public). Centralizing this here keeps every
// downstream consumer using the identical labels regardless of the
// underlying provider.
func decodeSharingState(raw []byte) (string, string) {
	if len(raw) == 0 {
		return "private", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "private", ""
	}
	audience := stringField(payload, "audience")
	if visibility := stringField(payload, "visibility"); visibility != "" {
		switch visibility {
		case "public", "anyone_with_link", "anyone":
			return "public", audience
		case "domain", "audience":
			return "shared_audience", audience
		case "shared", "users":
			return "shared", audience
		}
	}
	if shared, ok := payload["shared"].(bool); ok && shared {
		if audience != "" {
			return "shared_audience", audience
		}
		return "shared", ""
	}
	return "private", audience
}

func decodeClassification(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if cls := stringField(payload, "classification"); cls != "" {
		return cls
	}
	if domain, ok := payload["domain_data"].(map[string]any); ok {
		if cls := stringField(domain, "classification"); cls != "" {
			return cls
		}
	}
	return ""
}

func decodeProviderID(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return stringField(payload, "provider_id")
}

func stringField(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
