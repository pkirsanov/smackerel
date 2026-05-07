package immich

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

type Client struct {
	httpClient     *http.Client
	baseURL        string
	apiKey         string
	capability     photolib.CapabilityReport
	skipsMu        sync.Mutex
	skips          []photolib.SkipEntry
	uploadMaxBytes int64 // SST-injected via SetUploadMaxBytes; 0 = unlimited (test paths only)
}

type Asset struct {
	ID               string                           `json:"id"`
	Type             string                           `json:"type"`
	OriginalFileName string                           `json:"originalFileName"`
	MIMEType         string                           `json:"mimeType"`
	Checksum         string                           `json:"checksum"`
	FileCreatedAt    string                           `json:"fileCreatedAt"`
	FileModifiedAt   string                           `json:"fileModifiedAt"`
	UpdatedAt        string                           `json:"updatedAt"`
	Size             *int64                           `json:"size"`
	EXIFInfo         map[string]any                   `json:"exifInfo"`
	Albums           []AlbumRef                       `json:"albums"`
	Tags             []string                         `json:"tags"`
	People           []PersonRef                      `json:"people"`
	Deleted          bool                             `json:"deleted"`
	SkipReason       string                           `json:"skipReason"`
	Classification   *photolib.ClassificationDecision `json:"classification"`
}

type AlbumRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PersonRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type versionResponse struct {
	Version string `json:"version"`
	Major   int    `json:"major"`
}

type assetsResponse struct {
	Assets []Asset `json:"assets"`
	Cursor string  `json:"cursor"`
}

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

func (client *Client) ID() string { return "immich" }

func (client *Client) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	baseURL, _ := config.SourceConfig["base_url"].(string)
	apiKey := config.Credentials["api_key"]
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("immich: base_url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("immich: api_key is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("immich: base_url must be an absolute URL")
	}
	client.baseURL = strings.TrimRight(baseURL, "/")
	client.apiKey = apiKey
	capabilities, err := client.ProbeCapabilities(ctx, config)
	if err != nil {
		return err
	}
	client.capability = capabilities
	return nil
}

func (client *Client) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	events, errs := client.Watch(ctx, cursor)
	var artifacts []connector.RawArtifact
	for event := range events {
		artifacts = append(artifacts, event.RawArtifact("photos:immich"))
	}
	for err := range errs {
		if err != nil {
			return nil, cursor, err
		}
	}
	return artifacts, cursor, nil
}

func (client *Client) Health(ctx context.Context) connector.HealthStatus {
	request, err := client.newRequest(ctx, http.MethodGet, "/api/server/version", nil)
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

func (client *Client) Close() error { return nil }

func (client *Client) Capabilities() photolib.CapabilityReport {
	if client.capability.Provider == "" {
		client.capability = defaultCapabilities("v1")
	}
	return client.capability
}

func (client *Client) ProbeCapabilities(ctx context.Context, config connector.ConnectorConfig) (photolib.CapabilityReport, error) {
	probeBaseURL := client.baseURL
	probeAPIKey := client.apiKey
	if probeBaseURL == "" {
		if baseURL, ok := config.SourceConfig["base_url"].(string); ok {
			probeBaseURL = strings.TrimRight(baseURL, "/")
		}
	}
	if probeAPIKey == "" {
		probeAPIKey = config.Credentials["api_key"]
	}
	httpClient := client.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	request, err := buildImmichRequest(ctx, probeBaseURL, probeAPIKey, http.MethodGet, "/api/server/version", nil)
	if err != nil {
		return photolib.CapabilityReport{}, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return photolib.CapabilityReport{}, fmt.Errorf("immich: capability probe failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return photolib.CapabilityReport{}, fmt.Errorf("immich: capability probe returned HTTP %d", response.StatusCode)
	}
	var parsed versionResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return photolib.CapabilityReport{}, fmt.Errorf("immich: decode capability probe: %w", err)
	}
	version := parsed.Version
	if version == "" && parsed.Major > 0 {
		version = fmt.Sprintf("v%d", parsed.Major)
	}
	if version == "" {
		version = "unknown"
	}
	return defaultCapabilities(version), nil
}

func (client *Client) EnumerateScope(ctx context.Context, scope photolib.Scope) (<-chan photolib.PhotoEvent, <-chan error) {
	return client.streamAssets(ctx, "/api/smackerel/assets", scope)
}

func (client *Client) Watch(ctx context.Context, cursor string) (<-chan photolib.PhotoEvent, <-chan error) {
	endpoint := "/api/smackerel/changes"
	if cursor != "" {
		endpoint += "?cursor=" + url.QueryEscape(cursor)
	}
	return client.streamAssets(ctx, endpoint, photolib.Scope{})
}

func (client *Client) Fetch(ctx context.Context, ref string, kind photolib.FetchKind) (io.ReadCloser, photolib.FetchMeta, error) {
	assetPath := path.Join("/api/assets", ref, string(kind))
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
		return nil, photolib.FetchMeta{}, fmt.Errorf("immich: fetch returned HTTP %d", response.StatusCode)
	}
	return response.Body, photolib.FetchMeta{MIMEType: response.Header.Get("Content-Type"), Size: response.ContentLength}, nil
}

func (client *Client) Writer() photolib.ProviderWriter { return writer{client: client} }

func (client *Client) PhotoSkips() []photolib.SkipEntry {
	client.skipsMu.Lock()
	defer client.skipsMu.Unlock()
	copySkips := make([]photolib.SkipEntry, len(client.skips))
	copy(copySkips, client.skips)
	return copySkips
}

func (client *Client) streamAssets(ctx context.Context, endpoint string, scope photolib.Scope) (<-chan photolib.PhotoEvent, <-chan error) {
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
			errs <- fmt.Errorf("immich: asset enumeration returned HTTP %d", response.StatusCode)
			return
		}
		var parsed assetsResponse
		if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
			errs <- fmt.Errorf("immich: decode asset enumeration: %w", err)
			return
		}
		for _, asset := range parsed.Assets {
			if !assetInScope(asset, scope) {
				continue
			}
			event, skipEntry, err := MapAsset(asset)
			if err != nil {
				errs <- err
				return
			}
			if skipEntry != nil {
				client.appendSkip(*skipEntry)
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

func MapAsset(asset Asset) (photolib.PhotoEvent, *photolib.SkipEntry, error) {
	if strings.TrimSpace(asset.ID) == "" {
		return photolib.PhotoEvent{}, nil, fmt.Errorf("immich: asset id is required")
	}
	if asset.Deleted {
		return photolib.PhotoEvent{ProviderRef: asset.ID, Operation: photolib.PhotoOpDelete, RawProvider: map[string]any{"provider": "immich", "asset_id": asset.ID}}, nil, nil
	}
	if strings.TrimSpace(asset.SkipReason) != "" {
		skipEntry := photolib.SkipEntry{
			Reason:            asset.SkipReason,
			Count:             1,
			FileIdentities:    []string{asset.ID},
			RetryToken:        "retry:" + asset.SkipReason + ":" + asset.ID,
			RecommendedAction: "Retry extraction after checking the provider file.",
			LastSeenAt:        time.Now().UTC(),
		}
		return photolib.PhotoEvent{}, &skipEntry, nil
	}
	capturedAt, err := parseImmichTime(asset.FileCreatedAt)
	if err != nil {
		return photolib.PhotoEvent{}, nil, err
	}
	uploadedAt, _ := parseImmichTime(firstNonEmpty(asset.FileModifiedAt, asset.UpdatedAt, asset.FileCreatedAt))
	rawProvider := map[string]any{
		"provider": "immich",
		"asset_id": asset.ID,
	}
	if asset.Classification != nil {
		rawProvider["classification"] = asset.Classification
	}
	event := photolib.PhotoEvent{
		ProviderRef:       asset.ID,
		Operation:         photolib.PhotoOpUpsert,
		ProviderMediaKind: strings.ToLower(firstNonEmpty(asset.Type, "image")),
		MediaRole:         mediaRoleForAsset(asset),
		ContentHash:       asset.Checksum,
		Bytes:             asset.Size,
		MIMEType:          asset.MIMEType,
		Filename:          asset.OriginalFileName,
		CapturedAt:        capturedAt,
		UploadedAt:        uploadedAt,
		EXIF:              nonNilMap(asset.EXIFInfo),
		Albums:            albumNames(asset.Albums),
		Tags:              nonNilStrings(asset.Tags),
		Faces:             faceRefs(asset.People),
		Sensitivity:       photolib.ProviderSensitivity{Level: photolib.SensitivityNone, Source: "provider"},
		RawProvider:       rawProvider,
	}
	return event, nil, nil
}

func (client *Client) newRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	return buildImmichRequest(ctx, client.baseURL, client.apiKey, method, endpoint, body)
}

func buildImmichRequest(ctx context.Context, baseURL string, apiKey string, method string, endpoint string, body any) (*http.Request, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("immich: base_url is required")
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
	if apiKey != "" {
		request.Header.Set("x-api-key", apiKey)
	}
	return request, nil
}

func defaultCapabilities(version string) photolib.CapabilityReport {
	capabilities := map[photolib.Capability]photolib.CapabilityEntry{}
	for _, capability := range []photolib.Capability{
		photolib.CapRead,
		photolib.CapMonitor,
		photolib.CapWriteAlbum,
		photolib.CapWriteTag,
		photolib.CapWriteFavorite,
		photolib.CapArchive,
		photolib.CapDelete,
		photolib.CapUpload,
		photolib.CapFacesRead,
		photolib.CapFacesWrite,
		photolib.CapSensitivity,
	} {
		capabilities[capability] = photolib.CapabilityEntry{Status: photolib.CapabilitySupported}
	}
	return photolib.CapabilityReport{Provider: "immich", ProviderVersion: version, Capabilities: capabilities, DetectedAt: time.Now().UTC()}
}

func assetInScope(asset Asset, scope photolib.Scope) bool {
	if len(scope.IncludedAlbums) > 0 && !assetMatchesAnyAlbum(asset, scope.IncludedAlbums) {
		return false
	}
	if len(scope.ExcludedAlbums) > 0 && assetMatchesAnyAlbum(asset, scope.ExcludedAlbums) {
		return false
	}
	return true
}

func assetMatchesAnyAlbum(asset Asset, candidates []string) bool {
	for _, album := range asset.Albums {
		for _, candidate := range candidates {
			if album.ID == candidate || album.Name == candidate {
				return true
			}
		}
	}
	return false
}

func mediaRoleForAsset(asset Asset) photolib.MediaRole {
	if strings.HasPrefix(strings.ToLower(asset.MIMEType), "video/") || strings.EqualFold(asset.Type, "VIDEO") {
		return photolib.MediaRoleVideo
	}
	filename := strings.ToLower(asset.OriginalFileName)
	for _, suffix := range []string{".dng", ".cr2", ".nef", ".arw", ".raf", ".orf", ".rw2"} {
		if strings.HasSuffix(filename, suffix) {
			return photolib.MediaRoleRawOriginal
		}
	}
	return photolib.MediaRoleCameraOriginal
}

func parseImmichTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("immich: parse time %q: %w", value, err)
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
	values := make([]string, 0, len(albums))
	for _, album := range albums {
		if strings.TrimSpace(album.Name) != "" {
			values = append(values, album.Name)
			continue
		}
		if strings.TrimSpace(album.ID) != "" {
			values = append(values, album.ID)
		}
	}
	return values
}

func faceRefs(people []PersonRef) []photolib.FaceClusterRef {
	values := make([]photolib.FaceClusterRef, 0, len(people))
	for _, person := range people {
		if strings.TrimSpace(person.ID) == "" {
			continue
		}
		values = append(values, photolib.FaceClusterRef{Provider: "immich", ClusterRef: person.ID, Name: person.Name})
	}
	return values
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
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

type writer struct {
	client *Client
}

func (writer writer) AddToAlbum(ctx context.Context, photo string, album string) error {
	return writer.do(ctx, http.MethodPost, path.Join("/api/albums", album, "assets"), map[string]any{"ids": []string{photo}})
}

func (writer writer) Tag(ctx context.Context, photo string, tag string) error {
	return writer.do(ctx, http.MethodPost, path.Join("/api/assets", photo, "tags"), map[string]any{"tag": tag})
}

func (writer writer) Favorite(ctx context.Context, photo string, on bool) error {
	return writer.do(ctx, http.MethodPut, path.Join("/api/assets", photo, "favorite"), map[string]any{"favorite": on})
}

func (writer writer) Archive(ctx context.Context, photo string) error {
	return writer.do(ctx, http.MethodPost, path.Join("/api/assets", photo, "archive"), map[string]any{})
}

func (writer writer) Delete(ctx context.Context, photo string) error {
	return writer.do(ctx, http.MethodDelete, path.Join("/api/assets", photo), nil)
}

func (writer writer) Upload(ctx context.Context, src io.Reader, meta photolib.UploadMeta) (string, error) {
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
	requestBody := map[string]any{"filename": meta.Filename, "mime_type": meta.MIMEType, "albums": meta.Albums, "tags": meta.Tags, "bytes": data}
	request, err := writer.client.newRequest(ctx, http.MethodPost, "/api/assets", requestBody)
	if err != nil {
		return "", err
	}
	response, err := writer.client.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("immich: upload returned HTTP %d", response.StatusCode)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("immich: upload response missing id")
	}
	return parsed.ID, nil
}

func (writer writer) RenameFaceCluster(ctx context.Context, cluster string, name string) error {
	return writer.do(ctx, http.MethodPatch, path.Join("/api/people", cluster), map[string]any{"name": name})
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
		return fmt.Errorf("immich: writer endpoint %s returned HTTP %d", endpoint, response.StatusCode)
	}
	return nil
}
