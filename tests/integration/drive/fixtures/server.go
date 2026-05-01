// Package fixtures provides an owned HTTP fixture server that stands in
// for the Google OAuth and Google Drive REST APIs during Spec 038 Scope 1
// integration tests. The fixture exposes only the endpoints that the
// real GoogleDriveProvider calls during the OAuth authorization +
// connect-and-health round trip:
//
//   - GET  /oauth2/auth     — mints a code bound to the provided state
//     and returns it in a minimal JSON payload.
//   - POST /oauth2/token    — exchanges a code for an access+refresh
//     token with a 1-hour expires_in.
//   - GET  /drive/v3/about  — returns the bound user identity, gated by
//     a Bearer access token from /oauth2/token.
//   - GET  /drive/v3/files  — empty-drive listing returning {"files":[]}.
//
// The server is deterministic: state is in-memory, scoped to the Server
// instance, and reset by constructing a new Server. Tests SHOULD treat
// every Server as disposable per-test to avoid cross-test bleed.
//
// Programmatic helper IssueAuthCode lets tests skip the user-agent
// browser step and obtain a code bound to a server-issued state token
// directly. The /oauth2/auth endpoint also issues codes (via the same
// helper) so that interactive PWA tests can drive the redirect leg
// without extra fixture wiring.
package fixtures

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// File is a synthetic Google Drive file exposed by the fixture API.
type File struct {
	ID         string
	Name       string
	MimeType   string
	SizeBytes  int64
	FolderPath []string
	RevisionID string
	Owner      string
	URL        string
	Content    []byte
	Shared     bool
	Trashed    bool
}

// Change is a synthetic provider delta exposed by /drive/v3/changes.
type Change struct {
	Kind   string
	FileID string
}

// Server is an httptest.Server pre-loaded with the OAuth + Drive
// endpoint handlers. The embedded *httptest.Server exposes URL/Close.
type Server struct {
	*httptest.Server

	mu sync.Mutex
	// codes maps a one-shot authorization code to the state token
	// it was bound to at issuance. Consumed by /oauth2/token.
	codes map[string]string
	// tokens maps an access token to the user email returned by
	// /drive/v3/about. Tokens are minted by /oauth2/token.
	tokens        map[string]string
	files         map[string]File
	folders       map[string]string // folder_path -> provider folder id
	folderCreated map[string]int    // folder_path -> create count (Spec 038 Scope 5 BS-016 audit)
	uploads       []Upload
	changes       []Change
	requestCounts map[string]int
	outageStatus  int
	// uploadDelay simulates concurrent-upload latency. When > 0, every
	// folder-create call sleeps for the configured duration before
	// inserting into the folders map. Tests that exercise BS-016
	// (concurrent missing-folder saves) set this to widen the race
	// window so the unique-constraint guard is exercised.
	folderCreateDelay time.Duration
}

// Upload is a record of a successful POST /upload/drive/v3/files (or POST
// /drive/v3/files?uploadType=resumable) call. Tests assert provider-side
// write activity through Server.Uploads().
type Upload struct {
	ProviderFileID string
	FolderID       string
	Title          string
	MimeType       string
	SizeBytes      int64
	Bytes          []byte
}

// NewServer constructs and starts a fixture server. Callers MUST defer
// Close() to release the underlying httptest listener.
func NewServer() *Server {
	s := &Server{
		codes:         make(map[string]string),
		tokens:        make(map[string]string),
		files:         make(map[string]File),
		folders:       make(map[string]string),
		folderCreated: make(map[string]int),
		requestCounts: make(map[string]int),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/auth", s.handleAuth)
	mux.HandleFunc("/oauth2/token", s.handleToken)
	mux.HandleFunc("/drive/v3/about", s.handleAbout)
	mux.HandleFunc("/drive/v3/files", s.handleFiles)
	mux.HandleFunc("/drive/v3/files/", s.handleFileBytes)
	mux.HandleFunc("/drive/v3/changes", s.handleChanges)
	mux.HandleFunc("/upload/drive/v3/files", s.handleUpload)
	mux.HandleFunc("/drive/v3/folders", s.handleFolders)
	s.Server = httptest.NewServer(mux)
	return s
}

// AddFile adds or replaces one synthetic file and records an upsert change.
func (s *Server) AddFile(file File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if file.URL == "" {
		file.URL = "https://drive.example/" + file.ID
	}
	if file.Owner == "" {
		file.Owner = "fixture-user@example.com"
	}
	if file.RevisionID == "" {
		file.RevisionID = "rev-" + file.ID
	}
	s.files[file.ID] = file
}

// AddFiles adds a batch of synthetic files.
func (s *Server) AddFiles(files []File) {
	for _, file := range files {
		s.AddFile(file)
	}
}

// AddChange appends one synthetic change-feed entry.
func (s *Server) AddChange(change Change) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changes = append(s.changes, change)
}

// RequestCount returns how many times the fixture served a path.
func (s *Server) RequestCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestCounts[path]
}

// SetOutage makes Drive endpoints return the supplied status.
func (s *Server) SetOutage(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outageStatus = status
}

// ClearOutage restores healthy fixture responses.
func (s *Server) ClearOutage() {
	s.SetOutage(0)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should not fail on supported platforms; tests
		// would surface this as a fixture-internal error.
		panic("fixtures: rand.Read: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// IssueAuthCode mints a one-shot authorization code bound to the given
// state token. Tests use this helper to drive FinalizeConnect without
// performing a real browser redirect through /oauth2/auth.
func (s *Server) IssueAuthCode(state string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := "code-" + randHex(8)
	s.codes[code] = state
	return code
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, `{"error":"missing_state"}`, http.StatusBadRequest)
		return
	}
	code := s.IssueAuthCode(state)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":  code,
		"state": state,
	})
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad_form"}`, http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing_code"}`, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if _, ok := s.codes[code]; !ok {
		s.mu.Unlock()
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	delete(s.codes, code)
	access := "tok-" + randHex(16)
	s.tokens[access] = "fixture-user@example.com"
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  access,
		"refresh_token": "refresh-" + randHex(8),
		"expires_in":    3600,
		"token_type":    "Bearer",
	})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	if s.maybeOutage(w, r) {
		return
	}
	s.count(r.URL.Path)
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	s.mu.Lock()
	email, ok := s.tokens[token]
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user": map[string]any{
			"emailAddress": email,
			"displayName":  "Fixture User",
		},
	})
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if s.maybeOutage(w, r) {
		return
	}
	s.count("/drive/v3/files")
	s.mu.Lock()
	files := make([]File, 0, len(s.files))
	for _, file := range s.files {
		if !file.Trashed {
			files = append(files, file)
		}
	}
	s.mu.Unlock()
	sort.Slice(files, func(leftIndex int, rightIndex int) bool { return files[leftIndex].ID < files[rightIndex].ID })
	pageSize := 100
	if rawPageSize := r.URL.Query().Get("pageSize"); rawPageSize != "" {
		if parsed, err := strconv.Atoi(rawPageSize); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	startIndex := 0
	if token := r.URL.Query().Get("pageToken"); token != "" {
		parsed, err := strconv.Atoi(token)
		if err == nil && parsed >= 0 {
			startIndex = parsed
		}
	}
	endIndex := startIndex + pageSize
	if endIndex > len(files) {
		endIndex = len(files)
	}
	nextPageToken := ""
	if endIndex < len(files) {
		nextPageToken = strconv.Itoa(endIndex)
	}
	items := make([]map[string]any, 0, endIndex-startIndex)
	for _, file := range files[startIndex:endIndex] {
		items = append(items, file.toGoogleJSON())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"nextPageToken": nextPageToken,
		"files":         items,
	})
}

func (s *Server) handleFileBytes(w http.ResponseWriter, r *http.Request) {
	if s.maybeOutage(w, r) {
		return
	}
	s.count("/drive/v3/files")
	fileID := strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")
	s.mu.Lock()
	file, ok := s.files[fileID]
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", file.MimeType)
	_, _ = w.Write(file.Content)
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	if s.maybeOutage(w, r) {
		return
	}
	s.count("/drive/v3/changes")
	startIndex := 0
	if token := r.URL.Query().Get("pageToken"); token != "" {
		parsed, err := strconv.Atoi(token)
		if err == nil && parsed >= 0 {
			startIndex = parsed
		}
	}
	s.mu.Lock()
	changes := append([]Change(nil), s.changes...)
	files := make(map[string]File, len(s.files))
	for key, value := range s.files {
		files[key] = value
	}
	s.mu.Unlock()
	if startIndex > len(changes) {
		startIndex = len(changes)
	}
	items := make([]map[string]any, 0, len(changes)-startIndex)
	for _, change := range changes[startIndex:] {
		entry := map[string]any{"fileId": change.FileID, "kind": change.Kind}
		if change.Kind == "delete" {
			entry["removed"] = true
		}
		if file, ok := files[change.FileID]; ok && change.Kind != "delete" {
			entry["file"] = file.toGoogleJSON()
		}
		items = append(items, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"newStartPageToken": strconv.Itoa(len(changes)),
		"changes":           items,
	})
}

func (s *Server) count(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestCounts[path] = s.requestCounts[path] + 1
}

func (s *Server) maybeOutage(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	status := s.outageStatus
	s.mu.Unlock()
	if status == 0 {
		return false
	}
	if r != nil {
		s.count(r.URL.Path)
	}
	http.Error(w, fmt.Sprintf(`{"error":"fixture_outage","status":%d}`, status), status)
	return true
}

func (file File) toGoogleJSON() map[string]any {
	modified := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	return map[string]any{
		"id":             file.ID,
		"name":           file.Name,
		"mimeType":       file.MimeType,
		"size":           strconv.FormatInt(file.SizeBytes, 10),
		"parents":        []string{"root"},
		"webViewLink":    file.URL,
		"modifiedTime":   modified,
		"headRevisionId": file.RevisionID,
		"owners": []map[string]string{{
			"emailAddress": file.Owner,
			"displayName":  file.Owner,
		}},
		"sharingUser": map[string]string{
			"emailAddress": file.Owner,
			"displayName":  file.Owner,
		},
		"shared":  file.Shared,
		"trashed": file.Trashed,
		"appProperties": map[string]string{
			"smackerel_folder_path": strings.Join(file.FolderPath, "/"),
		},
	}
}

// SetFolderCreateDelay slows folder-create responses to widen the BS-016
// race window in tests that exercise concurrent missing-folder saves.
func (s *Server) SetFolderCreateDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.folderCreateDelay = d
}

// FolderCreateCount returns how many distinct create attempts were issued
// for the supplied folder path. The fixture honors the first writer, so
// values >1 prove the BS-016 contract has been enforced cross-process by
// drive_folder_resolutions (the unique constraint coalesces them to one).
func (s *Server) FolderCreateCount(folderPath string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.folderCreated[folderPath]
}

// Uploads returns a copy of the recorded upload audit log.
func (s *Server) Uploads() []Upload {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Upload, len(s.uploads))
	copy(out, s.uploads)
	return out
}

// FolderID returns the provider folder id assigned to the supplied path.
// Tests use this to assert "exactly one folder created" cross-call.
func (s *Server) FolderID(folderPath string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.folders[folderPath]
	return id, ok
}

// handleFolders services Spec 038 Scope 5 folder lookup + create. The real
// Google Drive API uses /drive/v3/files with a metadata query; the fixture
// exposes a dedicated /drive/v3/folders surface so the client can probe and
// create without conflating with file uploads. Contract:
//
//	GET  /drive/v3/folders?path={folder_path}
//	     → 200 {"id": "<folder-id>"} when the path exists, 404 otherwise.
//	POST /drive/v3/folders          (JSON body {"path": "..."})
//	     → 200 {"id": "<folder-id>"} after recording the create attempt.
//	     → If the path already exists, returns the existing folder id and
//	       increments the create counter (proves the BS-016 audit trail
//	       even when our resolver loses the race).
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	if s.maybeOutage(w, r) {
		return
	}
	s.count("/drive/v3/folders")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, `{"error":"missing_path"}`, http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		id, ok := s.folders[path]
		s.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"bad_body"}`, http.StatusBadRequest)
			return
		}
		var req struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Path == "" {
			http.Error(w, `{"error":"bad_json"}`, http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		delay := s.folderCreateDelay
		s.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		s.mu.Lock()
		s.folderCreated[req.Path] = s.folderCreated[req.Path] + 1
		if existing, ok := s.folders[req.Path]; ok {
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": existing})
			return
		}
		newID := "folder-" + randHex(8)
		s.folders[req.Path] = newID
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": newID})
	default:
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleUpload services Spec 038 Scope 5 file uploads. The real Google
// Drive API uses POST /upload/drive/v3/files?uploadType=multipart with a
// related-multipart payload. The fixture accepts a simpler JSON body
// because the runtime client (internal/drive/google) wraps the bytes in a
// JSON envelope when talking to the fixture, which keeps the integration
// test surface narrow and inspectable.
//
// Body:
//
//	{"folder_id": "...", "title": "...", "mime_type": "...",
//	 "data_b64": "..."}
//
// On success the fixture stores the file in `files`, records an Upload
// audit row, and returns:
//
//	{"id": "<file-id>", "webViewLink": "...", "headRevisionId": "..."}
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if s.maybeOutage(w, r) {
		return
	}
	s.count("/upload/drive/v3/files")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"bad_body"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		FolderID string `json:"folder_id"`
		Title    string `json:"title"`
		MimeType string `json:"mime_type"`
		DataB64  string `json:"data_b64"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"bad_json"}`, http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.FolderID == "" {
		http.Error(w, `{"error":"missing_fields"}`, http.StatusBadRequest)
		return
	}
	data, err := decodeBase64(req.DataB64)
	if err != nil {
		http.Error(w, `{"error":"bad_b64"}`, http.StatusBadRequest)
		return
	}
	fileID := "file-" + randHex(8)
	revisionID := "rev-" + randHex(6)
	url := "https://drive.example/" + fileID
	s.mu.Lock()
	s.files[fileID] = File{
		ID:         fileID,
		Name:       req.Title,
		MimeType:   req.MimeType,
		SizeBytes:  int64(len(data)),
		FolderPath: []string{req.FolderID},
		RevisionID: revisionID,
		Owner:      "fixture-user@example.com",
		URL:        url,
		Content:    data,
	}
	s.uploads = append(s.uploads, Upload{
		ProviderFileID: fileID,
		FolderID:       req.FolderID,
		Title:          req.Title,
		MimeType:       req.MimeType,
		SizeBytes:      int64(len(data)),
		Bytes:          append([]byte(nil), data...),
	})
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":             fileID,
		"webViewLink":    url,
		"headRevisionId": revisionID,
	})
}

func decodeBase64(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}
