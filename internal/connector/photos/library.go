package photos

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

type Capability string

type CapabilityStatus string

const (
	CapRead          Capability = "read"
	CapMonitor       Capability = "monitor"
	CapWriteAlbum    Capability = "write_album"
	CapWriteTag      Capability = "write_tag"
	CapWriteFavorite Capability = "write_favorite"
	CapArchive       Capability = "archive"
	CapDelete        Capability = "delete"
	CapUpload        Capability = "upload"
	CapFacesRead     Capability = "faces_read"
	CapFacesWrite    Capability = "faces_write"
	CapSensitivity   Capability = "sensitivity"

	CapabilitySupported   CapabilityStatus = "supported"
	CapabilityLimited     CapabilityStatus = "limited"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityUnknown     CapabilityStatus = "unknown"
)

type CapabilityEntry struct {
	Status         CapabilityStatus `json:"status"`
	LimitationCode string           `json:"limitation_code,omitempty"`
}

type CapabilityReport struct {
	Provider        string                         `json:"provider"`
	ProviderVersion string                         `json:"provider_version"`
	Capabilities    map[Capability]CapabilityEntry `json:"capabilities"`
	DetectedAt      time.Time                      `json:"detected_at"`
}

type PhotoOperation string

const (
	PhotoOpUpsert PhotoOperation = "upsert"
	PhotoOpDelete PhotoOperation = "delete"
)

type MediaRole string

const (
	MediaRoleUnknown        MediaRole = "unknown"
	MediaRoleRawOriginal    MediaRole = "raw_original"
	MediaRoleCameraOriginal MediaRole = "camera_original"
	MediaRoleEditedExport   MediaRole = "edited_export"
	MediaRoleDerivedExport  MediaRole = "derived_export"
	MediaRoleVideo          MediaRole = "video"
	MediaRoleDocumentScan   MediaRole = "document_scan"
	MediaRoleBurstMember    MediaRole = "burst_member"
	MediaRoleLivePhoto      MediaRole = "live_photo"
)

type SensitivityLevel string

const (
	SensitivityNone      SensitivityLevel = "none"
	SensitivitySensitive SensitivityLevel = "sensitive"
	SensitivityHidden    SensitivityLevel = "hidden"
)

type ProviderSensitivity struct {
	Level  SensitivityLevel `json:"level"`
	Source string           `json:"source"`
	Labels []string         `json:"labels,omitempty"`
}

type FaceClusterRef struct {
	Provider   string `json:"provider"`
	ClusterRef string `json:"cluster_ref"`
	Name       string `json:"name,omitempty"`
}

type FetchKind string

const (
	FetchThumbnail FetchKind = "thumbnail"
	FetchPreview   FetchKind = "preview"
	FetchOriginal  FetchKind = "original"
)

type Scope struct {
	Libraries      []string `json:"libraries"`
	IncludedAlbums []string `json:"included_albums"`
	ExcludedAlbums []string `json:"excluded_albums"`
	UseFaces       bool     `json:"use_faces"`
}

type FetchMeta struct {
	MIMEType string
	Size     int64
}

type UploadMeta struct {
	Filename string
	MIMEType string
	Albums   []string
	Tags     []string
}

type PhotoEvent struct {
	ProviderRef       string              `json:"provider_ref"`
	Operation         PhotoOperation      `json:"op"`
	ProviderMediaKind string              `json:"provider_media_kind"`
	MediaRole         MediaRole           `json:"media_role"`
	ContentHash       string              `json:"content_hash"`
	Bytes             *int64              `json:"bytes,omitempty"`
	BytesEstimated    bool                `json:"bytes_estimated"`
	MIMEType          string              `json:"mime_type"`
	Filename          string              `json:"filename"`
	CapturedAt        time.Time           `json:"captured_at"`
	UploadedAt        time.Time           `json:"uploaded_at"`
	GeoLat            *float64            `json:"geo_lat,omitempty"`
	GeoLon            *float64            `json:"geo_lon,omitempty"`
	EXIF              map[string]any      `json:"exif"`
	Albums            []string            `json:"albums"`
	Tags              []string            `json:"tags"`
	Faces             []FaceClusterRef    `json:"faces"`
	Sensitivity       ProviderSensitivity `json:"sensitivity"`
	RawProvider       map[string]any      `json:"raw_provider"`

	// Spec 040 Scope 4 — capture-channel fields. Provider scans omit
	// them (defaults persist as `source_channel='provider'`); the
	// upload pipeline (`POST /v1/photos/upload`) populates them with
	// the inbound channel + opaque source reference plus the document
	// group when running mobile document scan.
	SourceChannel     SourceChannel `json:"source_channel,omitempty"`
	SourceRef         string        `json:"source_ref,omitempty"`
	DocumentGroupRef  string        `json:"document_group_ref,omitempty"`
	DocumentPageIndex int           `json:"document_page_index,omitempty"`
}

// SourceChannel identifies the inbound transport that produced a photo
// event. The `provider` value is reserved for provider scans; uploads
// from the user-visible channels carry the matching transport tag so the
// store can answer "where did this photo come from" without parsing the
// provider payload.
type SourceChannel string

const (
	SourceChannelProvider SourceChannel = "provider"
	SourceChannelTelegram SourceChannel = "telegram"
	SourceChannelMobile   SourceChannel = "mobile"
	SourceChannelWeb      SourceChannel = "web"
	SourceChannelAgent    SourceChannel = "agent"
)

func (channel SourceChannel) Valid() bool {
	switch channel {
	case SourceChannelProvider, SourceChannelTelegram,
		SourceChannelMobile, SourceChannelWeb, SourceChannelAgent:
		return true
	}
	return false
}

type PhotoLibrary interface {
	connector.Connector
	Capabilities() CapabilityReport
	ProbeCapabilities(ctx context.Context, config connector.ConnectorConfig) (CapabilityReport, error)
	EnumerateScope(ctx context.Context, scope Scope) (<-chan PhotoEvent, <-chan error)
	Watch(ctx context.Context, cursor string) (<-chan PhotoEvent, <-chan error)
	Fetch(ctx context.Context, ref string, kind FetchKind) (io.ReadCloser, FetchMeta, error)
	Writer() ProviderWriter
}

type ProviderWriter interface {
	AddToAlbum(ctx context.Context, photo string, album string) error
	Tag(ctx context.Context, photo string, tag string) error
	Favorite(ctx context.Context, photo string, on bool) error
	Archive(ctx context.Context, photo string) error
	Delete(ctx context.Context, photo string) error
	Upload(ctx context.Context, src io.Reader, meta UploadMeta) (string, error)
	RenameFaceCluster(ctx context.Context, cluster string, name string) error
}

func (event PhotoEvent) Validate() error {
	var missing []string
	if strings.TrimSpace(event.ProviderRef) == "" {
		missing = append(missing, "provider_ref")
	}
	if event.Operation != PhotoOpUpsert && event.Operation != PhotoOpDelete {
		missing = append(missing, "op")
	}
	if event.Operation == PhotoOpUpsert {
		if strings.TrimSpace(event.ProviderMediaKind) == "" {
			missing = append(missing, "provider_media_kind")
		}
		if event.MediaRole == "" {
			missing = append(missing, "media_role")
		}
		if strings.TrimSpace(event.MIMEType) == "" {
			missing = append(missing, "mime_type")
		}
		if strings.TrimSpace(event.Filename) == "" {
			missing = append(missing, "filename")
		}
		if strings.TrimSpace(event.ContentHash) == "" {
			missing = append(missing, "content_hash")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("photos: missing required PhotoEvent fields: %s", strings.Join(missing, ", "))
	}
	if _, ok := event.RawProvider["provider_specific"]; ok {
		return fmt.Errorf("photos: raw_provider contains forbidden provider_specific marker")
	}
	return nil
}

func (event PhotoEvent) ProviderName() string {
	if provider, ok := event.RawProvider["provider"].(string); ok {
		return provider
	}
	if provider, ok := event.RawProvider["provider_id"].(string); ok {
		return provider
	}
	return ""
}

func (event PhotoEvent) RawArtifact(sourceID string) connector.RawArtifact {
	metadata := event.ProviderNeutralPayload()
	return connector.RawArtifact{
		SourceID:    sourceID,
		SourceRef:   event.ProviderRef,
		ContentType: event.MIMEType,
		Title:       event.Filename,
		RawContent:  event.Filename,
		URL:         "",
		Metadata:    metadata,
		CapturedAt:  event.CapturedAt,
	}
}

func (event PhotoEvent) ProviderNeutralPayload() map[string]any {
	return map[string]any{
		"provider_ref":        event.ProviderRef,
		"op":                  string(event.Operation),
		"provider_media_kind": event.ProviderMediaKind,
		"media_role":          string(event.MediaRole),
		"content_hash":        event.ContentHash,
		"bytes":               event.Bytes,
		"bytes_estimated":     event.BytesEstimated,
		"mime_type":           event.MIMEType,
		"filename":            event.Filename,
		"captured_at":         event.CapturedAt,
		"uploaded_at":         event.UploadedAt,
		"geo_lat":             event.GeoLat,
		"geo_lon":             event.GeoLon,
		"exif":                event.EXIF,
		"albums":              event.Albums,
		"tags":                event.Tags,
		"faces":               event.Faces,
		"sensitivity":         event.Sensitivity,
		"raw_provider":        event.RawProvider,
		"source_channel":      string(event.SourceChannel),
		"source_ref":          event.SourceRef,
		"document_group_ref":  event.DocumentGroupRef,
		"document_page_index": event.DocumentPageIndex,
	}
}

func SyntheticPhotoEvent() PhotoEvent {
	bytes := int64(1_048_576)
	lat := 38.7223
	lon := -9.1393
	return PhotoEvent{
		ProviderRef:       "synthetic-photo-040",
		Operation:         PhotoOpUpsert,
		ProviderMediaKind: "image",
		MediaRole:         MediaRoleCameraOriginal,
		ContentHash:       "sha256:synthetic-photo-040",
		Bytes:             &bytes,
		MIMEType:          "image/jpeg",
		Filename:          "synthetic-photo-040.jpg",
		CapturedAt:        time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		UploadedAt:        time.Date(2026, 4, 27, 12, 1, 0, 0, time.UTC),
		GeoLat:            &lat,
		GeoLon:            &lon,
		EXIF:              map[string]any{"camera": "Synthetic Camera", "software": "Fixture Generator"},
		Albums:            []string{"Synthetic Fixtures"},
		Tags:              []string{"scope-1", "fixture"},
		Faces:             []FaceClusterRef{{Provider: "synthetic", ClusterRef: "face-040", Name: "Fixture Person"}},
		Sensitivity:       ProviderSensitivity{Level: SensitivityNone, Source: "provider"},
		RawProvider:       map[string]any{"provider": "synthetic", "asset_id": "synthetic-photo-040"},
	}
}
