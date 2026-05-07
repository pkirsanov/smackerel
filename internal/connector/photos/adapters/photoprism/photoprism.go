// Package photoprism is the second non-Immich provider adapter for
// Spec 040 Scope 5. It implements the provider-neutral
// `photolib.PhotoLibrary` contract on top of the PhotoPrism HTTP API
// (https://docs.photoprism.app/developer-guide/api/), with a focused
// surface that proves the capability matrix: read/scope/scan/monitor
// and the album/tag/favorite/archive/delete writers PhotoPrism does
// expose, while marking face-cluster rename UNSUPPORTED and
// sensitivity inference LIMITED — both with stable limitation codes
// from the shared capability-taxonomy registry.
//
// The adapter intentionally mirrors the Immich shape (single-file
// http.Client, capability probe via `/api/v1/server`, fixture-driven
// integration tests). The capability-taxonomy canary asserts the
// Go registry codes match the API + PWA strings.
package photoprism

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// providerName MUST stay lowercase + slug-style. The ingest pipeline
// stores it directly in `photos.provider` and the cross-provider
// duplicate signal expects no whitespace or casing drift.
const providerName = "photoprism"

// Client is the PhotoPrism `PhotoLibrary` adapter. It is constructed
// with a stdlib http.Client so tests can inject the
// `httptest.NewServer` client without changing call sites.
type Client struct {
	httpClient     *http.Client
	baseURL        string
	apiToken       string
	capability     photolib.CapabilityReport
	skipsMu        sync.Mutex
	skips          []photolib.SkipEntry
	uploadMaxBytes int64 // SST-injected via SetUploadMaxBytes; 0 = unlimited (test paths only)
}

// Photo is the on-the-wire shape returned by `/api/v1/photos` and
// `/api/v1/changes`. The adapter unwraps every field into the
// provider-neutral `photolib.PhotoEvent` shape — downstream code MUST
// NOT branch on this struct.
type Photo struct {
	UID            string         `json:"UID"`
	Type           string         `json:"Type"`
	OriginalName   string         `json:"OriginalName"`
	MIME           string         `json:"MIME"`
	Hash           string         `json:"Hash"`
	TakenAt        string         `json:"TakenAt"`
	CreatedAt      string         `json:"CreatedAt"`
	UpdatedAt      string         `json:"UpdatedAt"`
	FileSize       *int64         `json:"FileSize"`
	Lat            *float64       `json:"Lat"`
	Lng            *float64       `json:"Lng"`
	Albums         []AlbumRef     `json:"Albums"`
	Labels         []LabelRef     `json:"Labels"`
	Subjects       []SubjectRef   `json:"Subjects"`
	EXIFInfo       map[string]any `json:"EXIF"`
	Deleted        bool           `json:"Deleted"`
	SkipReason     string         `json:"SkipReason"`
	Classification *photolib.ClassificationDecision
}

type AlbumRef struct {
	UID  string `json:"UID"`
	Name string `json:"Name"`
}

type LabelRef struct {
	Name string `json:"Name"`
}

type SubjectRef struct {
	UID  string `json:"UID"`
	Name string `json:"Name"`
}

type photosResponse struct {
	Photos []Photo `json:"photos"`
	Cursor string  `json:"cursor"`
}

type serverResponse struct {
	Version string `json:"version"`
}

// NewClient constructs a PhotoPrism adapter. Pass `nil` to use
// `http.DefaultClient`.
func NewClient(client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{httpClient: client}
}

// SetUploadMaxBytes injects the SST-derived upload-body cap (MIT-040-S-006).
// Production wiring MUST set this from PhotosConfig.IOLimits.PhotoBinaryMaxBytes.
// Passing a value of 0 disables the cap and is intended for test paths only.
func (client *Client) SetUploadMaxBytes(max int64) {
	client.uploadMaxBytes = max
}

// ID is the provider identifier persisted in `photos.provider`.
func (client *Client) ID() string { return providerName }

// Connect validates SourceConfig + Credentials, runs the capability
// probe, and caches the resolved capability report.
func (client *Client) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	baseURL, _ := config.SourceConfig["base_url"].(string)
	apiToken := config.Credentials["api_token"]
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("photoprism: base_url is required")
	}
	if strings.TrimSpace(apiToken) == "" {
		return fmt.Errorf("photoprism: api_token is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("photoprism: base_url must be an absolute URL")
	}
	client.baseURL = strings.TrimRight(baseURL, "/")
	client.apiToken = apiToken
	report, err := client.ProbeCapabilities(ctx, config)
	if err != nil {
		return err
	}
	client.capability = report
	return nil
}

// Sync exists to satisfy `connector.Connector`. The runtime ingest
// path uses `Watch`/`EnumerateScope` directly so this just adapts the
// channel-based interface to the artifact-list shape.
func (client *Client) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	events, errs := client.Watch(ctx, cursor)
	var artifacts []connector.RawArtifact
	for event := range events {
		artifacts = append(artifacts, event.RawArtifact("photos:"+providerName))
	}
	for err := range errs {
		if err != nil {
			return nil, cursor, err
		}
	}
	return artifacts, cursor, nil
}

// Health probes `/api/v1/server` so the operator dashboard can mark the
// connector degraded when PhotoPrism returns 5xx.
func (client *Client) Health(ctx context.Context) connector.HealthStatus {
	request, err := client.newRequest(ctx, http.MethodGet, "/api/v1/server", nil)
	if err != nil {
		return connector.HealthError
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return connector.HealthFailing
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return connector.HealthHealthy
	}
	return connector.HealthDegraded
}

// Close releases adapter resources. The HTTP client is owned by the
// caller so there's nothing to close here.
func (client *Client) Close() error { return nil }

// Capabilities returns the cached capability report. If `Connect` has
// not run yet (test paths), the static defaults are returned so
// downstream code never sees an empty Provider field.
func (client *Client) Capabilities() photolib.CapabilityReport {
	if client.capability.Provider == "" {
		client.capability = defaultCapabilities("unknown")
	}
	return client.capability
}

// ProbeCapabilities hits `/api/v1/server`, then fills in the static
// capability matrix for PhotoPrism. The matrix is intentionally
// limited compared to Immich to exercise the limitation governance
// surface (faces_write UNSUPPORTED, sensitivity LIMITED).
func (client *Client) ProbeCapabilities(ctx context.Context, config connector.ConnectorConfig) (photolib.CapabilityReport, error) {
	probeBaseURL := client.baseURL
	probeAPIToken := client.apiToken
	if probeBaseURL == "" {
		if baseURL, ok := config.SourceConfig["base_url"].(string); ok {
			probeBaseURL = strings.TrimRight(baseURL, "/")
		}
	}
	if probeAPIToken == "" {
		probeAPIToken = config.Credentials["api_token"]
	}
	httpClient := client.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	request, err := buildPhotoprismRequest(ctx, probeBaseURL, probeAPIToken, http.MethodGet, "/api/v1/server", nil)
	if err != nil {
		return photolib.CapabilityReport{}, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return photolib.CapabilityReport{}, fmt.Errorf("photoprism: capability probe failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return photolib.CapabilityReport{}, fmt.Errorf("photoprism: capability probe returned HTTP %d", response.StatusCode)
	}
	var parsed serverResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return photolib.CapabilityReport{}, fmt.Errorf("photoprism: decode capability probe: %w", err)
	}
	version := parsed.Version
	if strings.TrimSpace(version) == "" {
		version = "unknown"
	}
	return defaultCapabilities(version), nil
}

// EnumerateScope walks the provider library scoped to the user's
// album include/exclude list. Implementation mirrors Immich:
// stream-on-channel + skip-on-error with a per-asset SkipEntry.
func (client *Client) EnumerateScope(ctx context.Context, scope photolib.Scope) (<-chan photolib.PhotoEvent, <-chan error) {
	return client.streamPhotos(ctx, "/api/v1/photos", scope)
}

// Watch streams incremental changes from the cursor-anchored endpoint.
func (client *Client) Watch(ctx context.Context, cursor string) (<-chan photolib.PhotoEvent, <-chan error) {
	endpoint := "/api/v1/changes"
	if cursor != "" {
		endpoint += "?cursor=" + url.QueryEscape(cursor)
	}
	return client.streamPhotos(ctx, endpoint, photolib.Scope{})
}

// Fetch streams the binary representation requested by `kind`.
// PhotoPrism exposes thumbnails at `/api/v1/photos/<uid>/<kind>`.
func (client *Client) Fetch(ctx context.Context, ref string, kind photolib.FetchKind) (io.ReadCloser, photolib.FetchMeta, error) {
	assetPath := path.Join("/api/v1/photos", ref, string(kind))
	request, err := client.newRequest(ctx, http.MethodGet, assetPath, nil)
	if err != nil {
		return nil, photolib.FetchMeta{}, err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, photolib.FetchMeta{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, photolib.FetchMeta{}, fmt.Errorf("photoprism: fetch returned HTTP %d", response.StatusCode)
	}
	return response.Body, photolib.FetchMeta{MIMEType: response.Header.Get("Content-Type"), Size: response.ContentLength}, nil
}

// Writer returns the writer surface. The writer enforces capability
// status — destructive or unsupported operations return a typed error
// the API layer translates to `409 PROVIDER_LIMITATION`.
func (client *Client) Writer() photolib.ProviderWriter { return writer{client: client} }

// PhotoSkips returns the buffered skip entries. The scanner reads this
// after each enumerate pass to populate `photo_sync_state.skipped`.
func (client *Client) PhotoSkips() []photolib.SkipEntry {
	client.skipsMu.Lock()
	defer client.skipsMu.Unlock()
	out := make([]photolib.SkipEntry, len(client.skips))
	copy(out, client.skips)
	return out
}

func (client *Client) streamPhotos(ctx context.Context, endpoint string, scope photolib.Scope) (<-chan photolib.PhotoEvent, <-chan error) {
	events := make(chan photolib.PhotoEvent)
	errs := make(chan error, 1)
	client.resetSkips()
	go func() {
		defer close(events)
		defer close(errs)
		request, err := client.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			errs <- err
			return
		}
		response, err := client.httpClient.Do(request)
		if err != nil {
			errs <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			errs <- fmt.Errorf("photoprism: photo enumeration returned HTTP %d", response.StatusCode)
			return
		}
		var parsed photosResponse
		if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
			errs <- fmt.Errorf("photoprism: decode photos response: %w", err)
			return
		}
		for _, photo := range parsed.Photos {
			if !photoInScope(photo, scope) {
				continue
			}
			event, skip, err := MapPhoto(photo)
			if err != nil {
				errs <- err
				return
			}
			if skip != nil {
				client.appendSkip(*skip)
				continue
			}
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case events <- event:
			}
		}
	}()
	return events, errs
}

// MapPhoto translates a PhotoPrism `Photo` into a provider-neutral
// `PhotoEvent`. The resulting event has the same shape as the Immich
// adapter so the cross-provider duplicate signal works without
// per-provider branches.
func MapPhoto(photo Photo) (photolib.PhotoEvent, *photolib.SkipEntry, error) {
	if strings.TrimSpace(photo.UID) == "" {
		return photolib.PhotoEvent{}, nil, fmt.Errorf("photoprism: photo UID is required")
	}
	if photo.Deleted {
		return photolib.PhotoEvent{
			ProviderRef: photo.UID,
			Operation:   photolib.PhotoOpDelete,
			RawProvider: map[string]any{"provider": providerName, "uid": photo.UID},
		}, nil, nil
	}
	if strings.TrimSpace(photo.SkipReason) != "" {
		skip := photolib.SkipEntry{
			Reason:            photo.SkipReason,
			Count:             1,
			FileIdentities:    []string{photo.UID},
			RetryToken:        "retry:" + photo.SkipReason + ":" + photo.UID,
			RecommendedAction: "Retry extraction after checking the provider file.",
			LastSeenAt:        time.Now().UTC(),
		}
		return photolib.PhotoEvent{}, &skip, nil
	}
	captured, err := parsePhotoprismTime(photo.TakenAt)
	if err != nil {
		return photolib.PhotoEvent{}, nil, err
	}
	uploaded, _ := parsePhotoprismTime(firstNonEmpty(photo.UpdatedAt, photo.CreatedAt, photo.TakenAt))
	rawProvider := map[string]any{
		"provider": providerName,
		"uid":      photo.UID,
	}
	if photo.Classification != nil {
		rawProvider["classification"] = photo.Classification
	}
	contentHash := strings.TrimSpace(photo.Hash)
	if contentHash != "" && !strings.Contains(contentHash, ":") {
		// PhotoPrism returns the raw SHA-1 hex digest. Normalise to
		// the same `algo:digest` shape the cross-provider signal
		// expects so a SHA-1 collision with Immich's SHA-256 cannot
		// silently match.
		contentHash = "sha1:" + contentHash
	}
	return photolib.PhotoEvent{
		ProviderRef:       photo.UID,
		Operation:         photolib.PhotoOpUpsert,
		ProviderMediaKind: strings.ToLower(firstNonEmpty(photo.Type, "image")),
		MediaRole:         mediaRoleForPhoto(photo),
		ContentHash:       contentHash,
		Bytes:             photo.FileSize,
		MIMEType:          photo.MIME,
		Filename:          photo.OriginalName,
		CapturedAt:        captured,
		UploadedAt:        uploaded,
		GeoLat:            photo.Lat,
		GeoLon:            photo.Lng,
		EXIF:              nonNilMap(photo.EXIFInfo),
		Albums:            albumNames(photo.Albums),
		Tags:              labelNames(photo.Labels),
		Faces:             subjectRefs(photo.Subjects),
		Sensitivity:       photolib.ProviderSensitivity{Level: photolib.SensitivityNone, Source: providerName + ":inferred-locally"},
		RawProvider:       rawProvider,
	}, nil, nil
}

func (client *Client) newRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	return buildPhotoprismRequest(ctx, client.baseURL, client.apiToken, method, endpoint, body)
}

func buildPhotoprismRequest(ctx context.Context, baseURL string, apiToken string, method string, endpoint string, body any) (*http.Request, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("photoprism: base_url is required")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if apiToken != "" {
		request.Header.Set("X-Session-ID", apiToken)
	}
	return request, nil
}

func defaultCapabilities(version string) photolib.CapabilityReport {
	caps := map[photolib.Capability]photolib.CapabilityEntry{
		photolib.CapRead:          {Status: photolib.CapabilitySupported},
		photolib.CapMonitor:       {Status: photolib.CapabilitySupported},
		photolib.CapWriteAlbum:    {Status: photolib.CapabilitySupported},
		photolib.CapWriteTag:      {Status: photolib.CapabilitySupported},
		photolib.CapWriteFavorite: {Status: photolib.CapabilitySupported},
		photolib.CapArchive:       {Status: photolib.CapabilitySupported},
		photolib.CapDelete:        {Status: photolib.CapabilitySupported},
		photolib.CapUpload:        {Status: photolib.CapabilitySupported},
		photolib.CapFacesRead:     {Status: photolib.CapabilitySupported},
		photolib.CapFacesWrite: {
			Status:         photolib.CapabilityUnsupported,
			LimitationCode: string(photolib.LimitationFacesWriteNotSupported),
		},
		photolib.CapSensitivity: {
			Status:         photolib.CapabilityLimited,
			LimitationCode: string(photolib.LimitationSensitivityNotInferred),
		},
	}
	return photolib.CapabilityReport{
		Provider:        providerName,
		ProviderVersion: version,
		Capabilities:    caps,
		DetectedAt:      time.Now().UTC(),
	}
}

func photoInScope(photo Photo, scope photolib.Scope) bool {
	if len(scope.IncludedAlbums) > 0 && !photoMatchesAnyAlbum(photo, scope.IncludedAlbums) {
		return false
	}
	if len(scope.ExcludedAlbums) > 0 && photoMatchesAnyAlbum(photo, scope.ExcludedAlbums) {
		return false
	}
	return true
}

func photoMatchesAnyAlbum(photo Photo, candidates []string) bool {
	for _, album := range photo.Albums {
		for _, candidate := range candidates {
			if album.UID == candidate || album.Name == candidate {
				return true
			}
		}
	}
	return false
}

func mediaRoleForPhoto(photo Photo) photolib.MediaRole {
	if strings.HasPrefix(strings.ToLower(photo.MIME), "video/") || strings.EqualFold(photo.Type, "video") {
		return photolib.MediaRoleVideo
	}
	filename := strings.ToLower(photo.OriginalName)
	for _, suffix := range []string{".dng", ".cr2", ".nef", ".arw", ".raf", ".orf", ".rw2"} {
		if strings.HasSuffix(filename, suffix) {
			return photolib.MediaRoleRawOriginal
		}
	}
	return photolib.MediaRoleCameraOriginal
}

func parsePhotoprismTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("photoprism: parse time %q: %w", value, err)
	}
	return parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func albumNames(albums []AlbumRef) []string {
	out := make([]string, 0, len(albums))
	for _, album := range albums {
		if strings.TrimSpace(album.Name) != "" {
			out = append(out, album.Name)
			continue
		}
		if strings.TrimSpace(album.UID) != "" {
			out = append(out, album.UID)
		}
	}
	return out
}

func labelNames(labels []LabelRef) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		if strings.TrimSpace(label.Name) != "" {
			out = append(out, label.Name)
		}
	}
	return out
}

func subjectRefs(subjects []SubjectRef) []photolib.FaceClusterRef {
	out := make([]photolib.FaceClusterRef, 0, len(subjects))
	for _, subject := range subjects {
		if strings.TrimSpace(subject.UID) == "" {
			continue
		}
		out = append(out, photolib.FaceClusterRef{Provider: providerName, ClusterRef: subject.UID, Name: subject.Name})
	}
	return out
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func (client *Client) resetSkips() {
	client.skipsMu.Lock()
	defer client.skipsMu.Unlock()
	client.skips = nil
}

func (client *Client) appendSkip(entry photolib.SkipEntry) {
	client.skipsMu.Lock()
	defer client.skipsMu.Unlock()
	client.skips = append(client.skips, entry)
}

// writer satisfies `photolib.ProviderWriter` and enforces capability
// status. Operations the provider does not support return a typed
// `*ProviderLimitationError` so the API layer can translate it into
// the `409 PROVIDER_LIMITATION` envelope.
type writer struct {
	client *Client
}

// ProviderLimitationError is the typed error every writer returns
// when the requested action is not supported. It carries the stable
// limitation code so the API/PWA can both render the same banner
// without recomputing the taxonomy.
type ProviderLimitationError struct {
	Capability     photolib.Capability
	Status         photolib.CapabilityStatus
	LimitationCode photolib.LimitationCode
	Provider       string
}

func (err *ProviderLimitationError) Error() string {
	return fmt.Sprintf("photoprism: capability %q is %q (%s)", err.Capability, err.Status, err.LimitationCode)
}

func (writer writer) checkCapability(capability photolib.Capability) error {
	if writer.client == nil {
		return fmt.Errorf("photoprism: writer has no client")
	}
	descriptor := photolib.CheckCapability(writer.client.Capabilities(), capability)
	if descriptor == nil {
		return nil
	}
	return &ProviderLimitationError{
		Capability:     capability,
		Status:         descriptor.Status,
		LimitationCode: descriptor.Code,
		Provider:       providerName,
	}
}

func (writer writer) AddToAlbum(ctx context.Context, photo string, album string) error {
	if err := writer.checkCapability(photolib.CapWriteAlbum); err != nil {
		return err
	}
	return writer.do(ctx, http.MethodPost, path.Join("/api/v1/albums", album, "photos"), map[string]any{"photos": []string{photo}})
}

func (writer writer) Tag(ctx context.Context, photo string, tag string) error {
	if err := writer.checkCapability(photolib.CapWriteTag); err != nil {
		return err
	}
	return writer.do(ctx, http.MethodPost, path.Join("/api/v1/photos", photo, "labels"), map[string]any{"label": tag})
}

func (writer writer) Favorite(ctx context.Context, photo string, on bool) error {
	if err := writer.checkCapability(photolib.CapWriteFavorite); err != nil {
		return err
	}
	return writer.do(ctx, http.MethodPut, path.Join("/api/v1/photos", photo, "favorite"), map[string]any{"favorite": on})
}

func (writer writer) Archive(ctx context.Context, photo string) error {
	if err := writer.checkCapability(photolib.CapArchive); err != nil {
		return err
	}
	return writer.do(ctx, http.MethodPost, path.Join("/api/v1/photos", photo, "archive"), map[string]any{})
}

func (writer writer) Delete(ctx context.Context, photo string) error {
	if err := writer.checkCapability(photolib.CapDelete); err != nil {
		return err
	}
	return writer.do(ctx, http.MethodDelete, path.Join("/api/v1/photos", photo), nil)
}

func (writer writer) Upload(ctx context.Context, src io.Reader, meta photolib.UploadMeta) (string, error) {
	if err := writer.checkCapability(photolib.CapUpload); err != nil {
		return "", err
	}
	// MIT-040-S-006: cap the upload body to the SST-configured photo
	// binary max (defense-in-depth against attacker-controlled io.Reader
	// returning unbounded data). Production wiring sets the cap from
	// `PhotosConfig.IOLimits.PhotoBinaryMaxBytes`; tests that do not set
	// the limit retain the historical unbounded behavior.
	var reader io.Reader = src
	if writer.client.uploadMaxBytes > 0 {
		reader = io.LimitReader(src, writer.client.uploadMaxBytes)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	requestBody := map[string]any{"filename": meta.Filename, "mime_type": meta.MIMEType, "albums": meta.Albums, "labels": meta.Tags, "bytes": data}
	request, err := writer.client.newRequest(ctx, http.MethodPost, "/api/v1/upload", requestBody)
	if err != nil {
		return "", err
	}
	response, err := writer.client.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("photoprism: upload returned HTTP %d", response.StatusCode)
	}
	var parsed struct {
		UID string `json:"UID"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.UID == "" {
		return "", fmt.Errorf("photoprism: upload response missing UID")
	}
	return parsed.UID, nil
}

func (writer writer) RenameFaceCluster(ctx context.Context, cluster string, name string) error {
	// PhotoPrism's `/subjects` endpoint is reserved for the human
	// curator workflow; the public API does not expose a stable
	// rename surface. Returning a `ProviderLimitationError` here is
	// the contract that lets the API layer produce 409
	// PROVIDER_LIMITATION with the shared limitation code.
	if err := writer.checkCapability(photolib.CapFacesWrite); err != nil {
		return err
	}
	return writer.do(ctx, http.MethodPatch, path.Join("/api/v1/subjects", cluster), map[string]any{"name": name})
}

func (writer writer) do(ctx context.Context, method string, endpoint string, body any) error {
	request, err := writer.client.newRequest(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	response, err := writer.client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("photoprism: writer endpoint %s returned HTTP %d", endpoint, response.StatusCode)
	}
	return nil
}
