package photos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ProgressCounter struct {
	Done       int  `json:"done"`
	Total      int  `json:"total"`
	Empty      bool `json:"empty,omitempty"`
	ETASeconds *int `json:"eta_seconds,omitempty"`
}

type ScanProgress struct {
	Metadata    ProgressCounter `json:"metadata"`
	Thumbnails  ProgressCounter `json:"thumbnails"`
	Classify    ProgressCounter `json:"classify"`
	Embeddings  ProgressCounter `json:"embeddings"`
	OCR         ProgressCounter `json:"ocr"`
	Sensitivity ProgressCounter `json:"sensitivity"`
}

type SkipEntry struct {
	Reason            string    `json:"reason"`
	Count             int       `json:"count"`
	FileIdentities    []string  `json:"file_identities"`
	RetryToken        string    `json:"retry_token"`
	RecommendedAction string    `json:"recommended_action"`
	LastSeenAt        time.Time `json:"last_seen_at"`
}

type ConnectorState struct {
	ConnectorID          string           `json:"connector_id"`
	Provider             string           `json:"provider"`
	Status               string           `json:"status"`
	Scope                Scope            `json:"scope"`
	Progress             ScanProgress     `json:"progress"`
	Skips                []SkipEntry      `json:"skips"`
	Cursor               string           `json:"cursor,omitempty"`
	LastSyncAt           *time.Time       `json:"last_sync_at,omitempty"`
	MonitoringLagSeconds int              `json:"monitoring_lag_seconds"`
	UpdatedAt            time.Time        `json:"updated_at"`
	Capabilities         CapabilityReport `json:"capability_report,omitempty"`
}

type ScanResult struct {
	ConnectorID               string       `json:"connector_id"`
	Provider                  string       `json:"provider"`
	Progress                  ScanProgress `json:"progress"`
	Skips                     []SkipEntry  `json:"skips"`
	PersistedCount            int          `json:"persisted_count"`
	ClassifiedCount           int          `json:"classified_count"`
	ReusedClassificationCount int          `json:"reused_classification_count"`
	TombstonedCount           int          `json:"tombstoned_count"`
	Cursor                    string       `json:"cursor,omitempty"`
}

type ScannerConfig struct {
	MaxFileSizeBytes int64
}

type Scanner struct {
	store  *Store
	config ScannerConfig
	now    func() time.Time
}

type skipLedgerSource interface {
	PhotoSkips() []SkipEntry
}

func NewScanner(store *Store, config ScannerConfig) *Scanner {
	return &Scanner{store: store, config: config, now: time.Now}
}

func (scanner *Scanner) Scan(ctx context.Context, library PhotoLibrary, connectorID string, scope Scope) (*ScanResult, error) {
	if library == nil {
		return nil, fmt.Errorf("photos: photo library is required")
	}
	if scanner == nil || scanner.store == nil {
		return nil, fmt.Errorf("photos: scanner store is required")
	}
	events, errs := library.EnumerateScope(ctx, scope)
	result, err := scanner.consumeEvents(ctx, connectorID, library.Capabilities().Provider, events, errs)
	if err != nil {
		return nil, err
	}
	if skipSource, ok := library.(skipLedgerSource); ok {
		result.Skips = mergeSkips(result.Skips, skipSource.PhotoSkips(), scanner.now())
	}
	result.Provider = library.Capabilities().Provider
	result.Progress = progressForResult(result)
	state := connectorStateFromResult(connectorID, result.Provider, scope, result, scanner.now())
	state.Capabilities = library.Capabilities()
	if err := scanner.store.UpsertConnectorState(ctx, state); err != nil {
		return nil, err
	}
	if err := scanner.store.UpsertCapabilities(ctx, connectorID, library.Capabilities()); err != nil {
		return nil, err
	}
	return result, nil
}

func (scanner *Scanner) Monitor(ctx context.Context, library PhotoLibrary, connectorID string, cursor string) (*ScanResult, error) {
	if library == nil {
		return nil, fmt.Errorf("photos: photo library is required")
	}
	events, errs := library.Watch(ctx, cursor)
	result, err := scanner.consumeEvents(ctx, connectorID, library.Capabilities().Provider, events, errs)
	if err != nil {
		return nil, err
	}
	if skipSource, ok := library.(skipLedgerSource); ok {
		result.Skips = mergeSkips(result.Skips, skipSource.PhotoSkips(), scanner.now())
	}
	result.Provider = library.Capabilities().Provider
	result.Cursor = cursor
	result.Progress = progressForResult(result)
	state := connectorStateFromResult(connectorID, result.Provider, Scope{}, result, scanner.now())
	state.Cursor = cursor
	state.Capabilities = library.Capabilities()
	if err := scanner.store.UpsertConnectorState(ctx, state); err != nil {
		return nil, err
	}
	if err := scanner.store.UpsertCapabilities(ctx, connectorID, library.Capabilities()); err != nil {
		return nil, err
	}
	return result, nil
}

func (scanner *Scanner) ProcessEvent(ctx context.Context, connectorID string, event PhotoEvent) (*ScanResult, error) {
	if scanner == nil || scanner.store == nil {
		return nil, fmt.Errorf("photos: scanner store is required")
	}
	provider := event.ProviderName()
	if provider == "" {
		provider = "immich"
	}
	result := &ScanResult{ConnectorID: connectorID, Provider: provider}
	if err := scanner.processOne(ctx, connectorID, provider, event, result); err != nil {
		return nil, err
	}
	result.Progress = progressForResult(result)
	return result, nil
}

func (scanner *Scanner) consumeEvents(ctx context.Context, connectorID string, provider string, events <-chan PhotoEvent, errs <-chan error) (*ScanResult, error) {
	result := &ScanResult{ConnectorID: connectorID, Provider: provider}
	for event := range events {
		if err := scanner.processOne(ctx, connectorID, provider, event, result); err != nil {
			return nil, err
		}
	}
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (scanner *Scanner) processOne(ctx context.Context, connectorID string, provider string, event PhotoEvent, result *ScanResult) error {
	if provider == "" {
		provider = event.ProviderName()
	}
	if provider == "" {
		return fmt.Errorf("photos: provider is required")
	}
	if event.Operation == PhotoOpDelete {
		if err := scanner.store.MarkDeleted(ctx, connectorID, provider, event.ProviderRef); err != nil {
			return err
		}
		result.TombstonedCount++
		return nil
	}
	if skip := scanner.skipForEvent(event); skip != nil {
		result.Skips = mergeSkips(result.Skips, []SkipEntry{*skip}, scanner.now())
		return nil
	}
	var existing *PhotoRecord
	existing, existingErr := scanner.store.GetByProviderRef(ctx, provider, event.ProviderRef)
	if existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
		return existingErr
	}
	reuseClassification := existing != nil && existing.ContentHash == event.ContentHash && len(existing.Classification) > 0 && strings.TrimSpace(string(existing.Classification)) != "{}" && classificationFromEvent(event) == nil
	record, err := scanner.store.PublishPhotoEvent(ctx, connectorID, provider, event)
	if err != nil {
		return err
	}
	result.PersistedCount++
	if reuseClassification {
		result.ReusedClassificationCount++
		return nil
	}
	decision := classificationFromEvent(event)
	if decision == nil {
		return nil
	}
	if _, err := decision.Validate(); err != nil {
		return err
	}
	decision.Embedded = true
	if err := scanner.store.UpdateClassification(ctx, record.ID, *decision); err != nil {
		return err
	}
	result.ClassifiedCount++
	return nil
}

func (scanner *Scanner) skipForEvent(event PhotoEvent) *SkipEntry {
	if reason, ok := event.RawProvider["skip_reason"].(string); ok && strings.TrimSpace(reason) != "" {
		return newSkipEntry(reason, event.ProviderRef, scanner.now())
	}
	if scanner.config.MaxFileSizeBytes > 0 && event.Bytes != nil && *event.Bytes > scanner.config.MaxFileSizeBytes {
		return newSkipEntry("too_large", event.ProviderRef, scanner.now())
	}
	if !supportedPhotoMIME(event.MIMEType) {
		return newSkipEntry("unsupported_format", event.ProviderRef, scanner.now())
	}
	return nil
}

func supportedPhotoMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg", "image/heic", "image/heif", "image/png", "image/tiff", "image/webp", "video/mp4", "video/quicktime":
		return true
	default:
		return false
	}
}

func classificationFromEvent(event PhotoEvent) *ClassificationDecision {
	rawDecision, ok := event.RawProvider["classification"]
	if !ok || rawDecision == nil {
		return nil
	}
	encoded, err := json.Marshal(rawDecision)
	if err != nil {
		return nil
	}
	var decision ClassificationDecision
	if err := json.Unmarshal(encoded, &decision); err != nil {
		return nil
	}
	return &decision
}

func newSkipEntry(reason string, fileIdentity string, seenAt time.Time) *SkipEntry {
	return &SkipEntry{
		Reason:            normalizedSkipReason(reason),
		Count:             1,
		FileIdentities:    []string{fileIdentity},
		RetryToken:        "retry:" + normalizedSkipReason(reason) + ":" + fileIdentity,
		RecommendedAction: recommendedActionForSkip(reason),
		LastSeenAt:        seenAt.UTC(),
	}
}

func normalizedSkipReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "too_large", "unsupported_format", "permission_denied", "provider_error", "provider_5xx", "extraction_failed", "private_scope":
		return strings.ToLower(strings.TrimSpace(reason))
	default:
		return "extraction_failed"
	}
}

func recommendedActionForSkip(reason string) string {
	switch normalizedSkipReason(reason) {
	case "too_large":
		return "Reduce the file size or raise the configured photo scan size limit."
	case "unsupported_format":
		return "Convert the file to a supported photo or video format."
	case "permission_denied":
		return "Update provider permissions and retry this batch."
	case "provider_error", "provider_5xx":
		return "Retry the batch after the provider recovers."
	case "private_scope":
		return "Update the selected scan scope if this album should be included."
	default:
		return "Retry extraction after checking the provider file."
	}
}

func mergeSkips(existing []SkipEntry, additions []SkipEntry, seenAt time.Time) []SkipEntry {
	merged := make([]SkipEntry, 0, len(existing)+len(additions))
	byReason := map[string]int{}
	for _, entry := range existing {
		entry.Reason = normalizedSkipReason(entry.Reason)
		byReason[entry.Reason] = len(merged)
		merged = append(merged, entry)
	}
	for _, entry := range additions {
		entry.Reason = normalizedSkipReason(entry.Reason)
		if entry.Count == 0 {
			entry.Count = len(entry.FileIdentities)
		}
		if entry.Count == 0 {
			entry.Count = 1
		}
		if entry.LastSeenAt.IsZero() {
			entry.LastSeenAt = seenAt.UTC()
		}
		if entry.RetryToken == "" && len(entry.FileIdentities) > 0 {
			entry.RetryToken = "retry:" + entry.Reason + ":" + entry.FileIdentities[0]
		}
		if entry.RecommendedAction == "" {
			entry.RecommendedAction = recommendedActionForSkip(entry.Reason)
		}
		if index, ok := byReason[entry.Reason]; ok {
			merged[index].Count += entry.Count
			merged[index].FileIdentities = append(merged[index].FileIdentities, entry.FileIdentities...)
			merged[index].LastSeenAt = entry.LastSeenAt
			continue
		}
		byReason[entry.Reason] = len(merged)
		merged = append(merged, entry)
	}
	return merged
}

func progressForResult(result *ScanResult) ScanProgress {
	total := result.PersistedCount + result.TombstonedCount + skipTotal(result.Skips)
	processed := result.PersistedCount + result.TombstonedCount
	classifyDone := result.ClassifiedCount + result.ReusedClassificationCount
	progress := ScanProgress{
		Metadata:    ProgressCounter{Done: processed, Total: total, Empty: total == 0},
		Thumbnails:  ProgressCounter{Done: result.PersistedCount, Total: result.PersistedCount, Empty: total == 0},
		Classify:    ProgressCounter{Done: classifyDone, Total: result.PersistedCount, Empty: total == 0},
		Embeddings:  ProgressCounter{Done: classifyDone, Total: result.PersistedCount, Empty: total == 0},
		OCR:         ProgressCounter{Done: classifyDone, Total: result.PersistedCount, Empty: total == 0},
		Sensitivity: ProgressCounter{Done: classifyDone, Total: result.PersistedCount, Empty: total == 0},
	}
	return progress
}

func skipTotal(skips []SkipEntry) int {
	total := 0
	for _, entry := range skips {
		total += entry.Count
	}
	return total
}

func connectorStateFromResult(connectorID string, provider string, scope Scope, result *ScanResult, now time.Time) ConnectorState {
	status := "healthy"
	if len(result.Skips) > 0 {
		status = "degraded"
	}
	now = now.UTC()
	return ConnectorState{
		ConnectorID: connectorID,
		Provider:    provider,
		Status:      status,
		Scope:       scope,
		Progress:    result.Progress,
		Skips:       result.Skips,
		Cursor:      result.Cursor,
		LastSyncAt:  &now,
		UpdatedAt:   now,
	}
}
