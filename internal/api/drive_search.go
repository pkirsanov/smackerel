// Drive-aware search response shape and enrichment for Spec 038 Scope 4.
//
// Search results carrying artifact_type="drive_file" are decorated with
// the drive metadata Screen 5 / Screen 6 need to render snippet, folder
// breadcrumb, provider chip, sharing/sensitivity badges, provider URL,
// accessible action state (tombstoned/permission-lost), and version chain.
//
// Enrichment runs as a single batched query against drive_files (no N+1)
// after the main search has produced its candidate list. Non-drive
// results are passed through unchanged so the search surface remains
// consumer-stable for callers that do not consume drive metadata.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DriveSearchMetadata is the per-result drive metadata returned by
// /api/search and consumed by the PWA Screen 5 search UI and Screen 6
// detail UI. The shape is intentionally provider-neutral: every field
// is derivable from the Provider interface or from drive_files row
// columns that exist for every drive provider.
//
// Availability values:
//   - "available"        — bytes are reachable through the provider
//   - "tombstoned"       — the provider trashed/deleted the file but
//     extracted knowledge remains queryable per retention policy
//   - "permission_lost"  — the provider revoked access; metadata stays
//     queryable but bytes are unavailable until reconnect
type DriveSearchMetadata struct {
	ProviderID       string   `json:"provider_id"`
	ProviderURL      string   `json:"provider_url"`
	FolderBreadcrumb []string `json:"folder_breadcrumb"`
	SharingState     string   `json:"sharing_state"`
	SharingAudience  string   `json:"sharing_audience,omitempty"`
	Sensitivity      string   `json:"sensitivity"`
	Availability     string   `json:"availability"`
	Tombstoned       bool     `json:"tombstoned"`
	PermissionLost   bool     `json:"permission_lost"`
	VersionChain     []string `json:"version_chain,omitempty"`
	OwnerLabel       string   `json:"owner_label,omitempty"`
	MimeType         string   `json:"mime_type,omitempty"`
	ActionsEnabled   bool     `json:"actions_enabled"`
}

// driveSearchRow is the raw scan target for the batched drive_files +
// artifacts join used by EnrichDriveResults.
type driveSearchRow struct {
	artifactID   string
	providerURL  string
	folderPath   []string
	sharingState []byte
	sensitivity  string
	tombstonedAt *string
	permissionAt *string
	versionChain []byte
	ownerLabel   string
	mimeType     string
	contentRaw   string
	summary      string
	metadata     []byte
}

// EnrichDriveResults batches a single drive_files JOIN artifacts query
// for every drive_file SearchResult, populates result.Snippet and
// result.Drive, and returns the same slice. Non-drive_file results pass
// through untouched. A nil pool, empty results slice, or zero
// drive_file results are all no-ops so this enrichment is safe to call
// from every code path that builds SearchResult values.
//
// query is the user-supplied search text used to extract a content
// snippet around the match position; it MAY be empty (snippet then
// falls back to the artifact summary).
func EnrichDriveResults(ctx context.Context, pool *pgxpool.Pool, query string, results []SearchResult) []SearchResult {
	if pool == nil || len(results) == 0 {
		return results
	}
	driveIndex := map[string]int{}
	driveIDs := make([]string, 0, len(results))
	for index, result := range results {
		if result.ArtifactType != "drive_file" {
			continue
		}
		if _, exists := driveIndex[result.ArtifactID]; exists {
			continue
		}
		driveIndex[result.ArtifactID] = index
		driveIDs = append(driveIDs, result.ArtifactID)
	}
	if len(driveIDs) == 0 {
		return results
	}

	rows, err := pool.Query(ctx, `
		SELECT f.artifact_id,
		       f.provider_url,
		       f.folder_path,
		       f.sharing_state,
		       f.sensitivity,
		       to_char(f.tombstoned_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       to_char(f.permission_lost_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       f.version_chain,
		       f.owner_label,
		       f.mime_type,
		       COALESCE(a.content_raw, ''),
		       COALESCE(a.summary, ''),
		       COALESCE(a.metadata, '{}'::jsonb)
		  FROM drive_files f
		  JOIN artifacts a ON a.id = f.artifact_id
		 WHERE f.artifact_id = ANY($1)
	`, driveIDs)
	if err != nil {
		slog.Warn("drive search enrichment query failed", "error", err, "drive_ids", len(driveIDs))
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var row driveSearchRow
		var tombstonedAt, permissionAt *string
		if err := rows.Scan(
			&row.artifactID,
			&row.providerURL,
			&row.folderPath,
			&row.sharingState,
			&row.sensitivity,
			&tombstonedAt,
			&permissionAt,
			&row.versionChain,
			&row.ownerLabel,
			&row.mimeType,
			&row.contentRaw,
			&row.summary,
			&row.metadata,
		); err != nil {
			slog.Warn("drive search enrichment scan failed", "error", err)
			continue
		}
		row.tombstonedAt = tombstonedAt
		row.permissionAt = permissionAt
		idx, ok := driveIndex[row.artifactID]
		if !ok {
			continue
		}
		results[idx].Drive = buildDriveSearchMetadata(row)
		results[idx].Snippet = buildDriveSnippet(query, row.summary, row.contentRaw)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("drive search enrichment row iteration failed", "error", err)
	}
	return results
}

// buildDriveSearchMetadata derives the Screen 5 / Screen 6 metadata from
// one drive_files JOIN artifacts row. It is the single place where
// drive_files columns are translated to the wire shape so future column
// additions land here, not at every caller.
func buildDriveSearchMetadata(row driveSearchRow) *DriveSearchMetadata {
	availability, tombstoned, permissionLost, actionsEnabled := availabilityFromRow(row)
	chain := decodeStringArray(row.versionChain)
	sharingState, sharingAudience := decodeSharingState(row.sharingState)
	providerID := decodeProviderID(row.metadata)

	return &DriveSearchMetadata{
		ProviderID:       providerID,
		ProviderURL:      row.providerURL,
		FolderBreadcrumb: append([]string{}, row.folderPath...),
		SharingState:     sharingState,
		SharingAudience:  sharingAudience,
		Sensitivity:      row.sensitivity,
		Availability:     availability,
		Tombstoned:       tombstoned,
		PermissionLost:   permissionLost,
		VersionChain:     chain,
		OwnerLabel:       row.ownerLabel,
		MimeType:         row.mimeType,
		ActionsEnabled:   actionsEnabled,
	}
}

// availabilityFromRow encodes the design.md §11 contract: tombstoned and
// permission-lost artifacts remain queryable but MUST NOT advertise
// downloadable bytes. ActionsEnabled is the explicit Screen 5 affordance
// flag — Open in Drive / preview / save-copy actions render disabled
// when this is false.
func availabilityFromRow(row driveSearchRow) (string, bool, bool, bool) {
	if row.tombstonedAt != nil && *row.tombstonedAt != "" {
		return "tombstoned", true, false, false
	}
	if row.permissionAt != nil && *row.permissionAt != "" {
		return "permission_lost", false, true, false
	}
	return "available", false, false, true
}

// decodeStringArray decodes a JSONB string-array column. Returns nil when
// the column is empty or invalid so callers can use len() == 0 as the
// "no chain" signal without disambiguating empty from missing.
func decodeStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// decodeSharingState collapses the provider-neutral sharing_state JSONB
// into Screen 5's compact label set. The PWA renders one of:
//   - "private"          — only the owner has access
//   - "shared"           — explicit shared list, no public link
//   - "shared_audience"  — shared with a domain/audience
//   - "public"           — link-shareable / public visibility
//
// Audience text is preserved verbatim when set so the badge can show
// "Shared with @kitchen-team" alongside the badge label.
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

// decodeProviderID reads metadata.provider_id from the artifacts row that
// was populated by the drive scan service. The column is the canonical
// place to recover the provider identifier from search results without
// re-joining drive_connections.
func decodeProviderID(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if id := stringField(payload, "provider_id"); id != "" {
		return id
	}
	return ""
}

func stringField(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// buildDriveSnippet returns a Screen 5-friendly excerpt from the
// extracted content (or the summary as fallback). If the query terms
// appear in content_raw, the snippet is centered on the first match;
// otherwise the leading window of either content_raw or summary is
// returned. Length is capped so Screen 5 layout is predictable.
const driveSnippetMax = 280

func buildDriveSnippet(query, summary, contentRaw string) string {
	source := contentRaw
	if strings.TrimSpace(source) == "" {
		source = summary
	}
	if strings.TrimSpace(source) == "" {
		return ""
	}
	source = collapseWhitespace(source)
	if query != "" {
		if start := indexOfQueryToken(source, query); start >= 0 {
			return centeredSnippet(source, start, driveSnippetMax)
		}
	}
	if len(source) <= driveSnippetMax {
		return source
	}
	return source[:driveSnippetMax] + "…"
}

func collapseWhitespace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func indexOfQueryToken(source, query string) int {
	lowerSource := strings.ToLower(source)
	for _, token := range strings.Fields(query) {
		token = strings.Trim(token, ".,!?;:\"'")
		if len(token) < 3 {
			continue
		}
		if idx := strings.Index(lowerSource, strings.ToLower(token)); idx >= 0 {
			return idx
		}
	}
	return -1
}

func centeredSnippet(source string, start, maxLen int) string {
	half := maxLen / 2
	leading := start - half
	if leading < 0 {
		leading = 0
	}
	end := leading + maxLen
	if end > len(source) {
		end = len(source)
		leading = end - maxLen
		if leading < 0 {
			leading = 0
		}
	}
	prefix := ""
	if leading > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(source) {
		suffix = "…"
	}
	return prefix + source[leading:end] + suffix
}

// errDriveDetailNotFound is returned by LoadDriveArtifactDetail when no
// drive_files row exists for the requested artifact. It is exported as a
// sentinel so the HTTP handler can map it to 404 without leaking driver
// errors.
var errDriveDetailNotFound = errors.New("drive: artifact detail not found")

// DriveArtifactDetailResponse is the JSON body served by
// GET /v1/drive/artifacts/{id}. It stitches together the artifact row
// and the drive_files row so Screen 6 can render preview/text/metadata/
// versions tabs in a single round trip.
type DriveArtifactDetailResponse struct {
	ArtifactID      string              `json:"artifact_id"`
	Title           string              `json:"title"`
	ArtifactType    string              `json:"artifact_type"`
	Summary         string              `json:"summary"`
	ExtractedText   string              `json:"extracted_text"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	Drive           DriveSearchMetadata `json:"drive"`
	Versions        []DriveVersionEntry `json:"versions"`
	BannerMessage   string              `json:"banner_message,omitempty"`
	BannerSeverity  string              `json:"banner_severity,omitempty"`
	ExtractionState string              `json:"extraction_state"`
}

// DriveVersionEntry is one row of the Screen 6 Versions tab. Order is
// most-recent-first; Index 0 is the head revision (current artifact).
type DriveVersionEntry struct {
	RevisionID string `json:"revision_id"`
	IsHead     bool   `json:"is_head"`
}

// LoadDriveArtifactDetail returns the full Screen 6 detail payload for
// the given drive artifact. It joins drive_files + artifacts in a single
// query and returns errDriveDetailNotFound when the row is missing so
// the caller can map cleanly to 404.
func LoadDriveArtifactDetail(ctx context.Context, pool *pgxpool.Pool, artifactID string) (*DriveArtifactDetailResponse, error) {
	if pool == nil {
		return nil, fmt.Errorf("drive detail: nil pool")
	}
	row := pool.QueryRow(ctx, `
		SELECT f.artifact_id,
		       a.title,
		       a.artifact_type,
		       COALESCE(a.summary, ''),
		       COALESCE(a.content_raw, ''),
		       a.created_at::text,
		       a.updated_at::text,
		       f.provider_url,
		       f.folder_path,
		       f.sharing_state,
		       f.sensitivity,
		       to_char(f.tombstoned_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       to_char(f.permission_lost_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       f.version_chain,
		       f.owner_label,
		       f.mime_type,
		       COALESCE(a.metadata, '{}'::jsonb),
		       f.extraction_state
		  FROM drive_files f
		  JOIN artifacts a ON a.id = f.artifact_id
		 WHERE f.artifact_id = $1
		 LIMIT 1`, artifactID)

	var (
		title, artifactType, summary, content string
		createdAt, updatedAt                  string
		providerURL                           string
		folderPath                            []string
		sharingState                          []byte
		sensitivity                           string
		tombstonedAt, permissionAt            *string
		versionChain                          []byte
		ownerLabel, mimeType                  string
		metadata                              []byte
		extractionState                       string
		gotArtifactID                         string
	)
	if err := row.Scan(
		&gotArtifactID,
		&title,
		&artifactType,
		&summary,
		&content,
		&createdAt,
		&updatedAt,
		&providerURL,
		&folderPath,
		&sharingState,
		&sensitivity,
		&tombstonedAt,
		&permissionAt,
		&versionChain,
		&ownerLabel,
		&mimeType,
		&metadata,
		&extractionState,
	); err != nil {
		if errors.Is(err, errDriveDetailNotFound) {
			return nil, errDriveDetailNotFound
		}
		// pgx returns ErrNoRows for missing detail; map both cases to the
		// sentinel via string match because the api package keeps its
		// driver-import surface narrow.
		if strings.Contains(err.Error(), "no rows") {
			return nil, errDriveDetailNotFound
		}
		return nil, err
	}

	driveRow := driveSearchRow{
		artifactID:   gotArtifactID,
		providerURL:  providerURL,
		folderPath:   folderPath,
		sharingState: sharingState,
		sensitivity:  sensitivity,
		tombstonedAt: tombstonedAt,
		permissionAt: permissionAt,
		versionChain: versionChain,
		ownerLabel:   ownerLabel,
		mimeType:     mimeType,
		summary:      summary,
		contentRaw:   content,
		metadata:     metadata,
	}
	meta := buildDriveSearchMetadata(driveRow)

	versions := buildDriveVersions(meta.VersionChain)
	banner, severity := buildAvailabilityBanner(meta.Availability)

	// Screen 6 hides the extracted text panel when bytes are unavailable;
	// the Versions tab still renders so users can confirm the historical
	// revisions even without current bytes.
	visibleText := content
	if !meta.ActionsEnabled {
		visibleText = ""
	}

	return &DriveArtifactDetailResponse{
		ArtifactID:      gotArtifactID,
		Title:           title,
		ArtifactType:    artifactType,
		Summary:         summary,
		ExtractedText:   visibleText,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		Drive:           *meta,
		Versions:        versions,
		BannerMessage:   banner,
		BannerSeverity:  severity,
		ExtractionState: extractionState,
	}, nil
}

// buildDriveVersions reverses the chronological version_chain so the
// most recent revision is first; the head revision (last appended) is
// flagged so Screen 6 can highlight it and disable diff actions against
// itself.
func buildDriveVersions(chain []string) []DriveVersionEntry {
	if len(chain) == 0 {
		return []DriveVersionEntry{}
	}
	entries := make([]DriveVersionEntry, 0, len(chain))
	headIndex := len(chain) - 1
	for i := len(chain) - 1; i >= 0; i-- {
		entries = append(entries, DriveVersionEntry{
			RevisionID: chain[i],
			IsHead:     i == headIndex,
		})
	}
	return entries
}

// buildAvailabilityBanner returns the Screen 6 banner copy and severity
// for an artifact whose provider bytes are no longer reachable. Banner
// strings are intentionally specific so the user understands why actions
// like "Open in Drive" are disabled and what next step recovers access.
func buildAvailabilityBanner(availability string) (string, string) {
	switch availability {
	case "tombstoned":
		return "This file was trashed in the source drive. Smackerel still indexes the extracted knowledge so you can search and link to it, but the original bytes are no longer downloadable.", "warning"
	case "permission_lost":
		return "Smackerel no longer has permission to read this file in the source drive. Reconnect the drive to restore access; the extracted knowledge remains queryable.", "warning"
	default:
		return "", ""
	}
}

// ApplyDriveSearchFilters drops drive_file results that do not match the
// optional drive-specific filters (provider/folder/sharing/audience/
// sensitivity). Non-drive results are returned untouched. Empty filter
// values are no-ops so callers can pass SearchFilters{} freely. Spec 038
// Scope 8 — multi-provider search returns one unified ranked list with
// provider-neutral filters; this is the post-enrichment gate that the
// SearchEngine applies before returning to the caller.
func ApplyDriveSearchFilters(filters SearchFilters, results []SearchResult) []SearchResult {
	if !hasDriveFilters(filters) {
		return results
	}
	out := results[:0]
	for _, result := range results {
		if result.ArtifactType != "drive_file" {
			out = append(out, result)
			continue
		}
		if result.Drive == nil {
			// Drive enrichment failed; refuse to leak the row past
			// a filter we cannot apply. Honest filtering > silent passthrough.
			continue
		}
		if !driveResultMatches(filters, *result.Drive) {
			continue
		}
		out = append(out, result)
	}
	return out
}

func hasDriveFilters(filters SearchFilters) bool {
	return filters.DriveProvider != "" ||
		filters.DriveFolder != "" ||
		filters.DriveSharing != "" ||
		filters.DriveAudience != "" ||
		filters.DriveSensitivity != ""
}

func driveResultMatches(filters SearchFilters, meta DriveSearchMetadata) bool {
	if filters.DriveProvider != "" && !strings.EqualFold(meta.ProviderID, filters.DriveProvider) {
		return false
	}
	if filters.DriveSharing != "" && !strings.EqualFold(meta.SharingState, filters.DriveSharing) {
		return false
	}
	if filters.DriveAudience != "" && !strings.EqualFold(meta.SharingAudience, filters.DriveAudience) {
		return false
	}
	if filters.DriveSensitivity != "" && !strings.EqualFold(meta.Sensitivity, filters.DriveSensitivity) {
		return false
	}
	if filters.DriveFolder != "" {
		needle := strings.ToLower(filters.DriveFolder)
		match := false
		for _, segment := range meta.FolderBreadcrumb {
			if strings.Contains(strings.ToLower(segment), needle) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}
