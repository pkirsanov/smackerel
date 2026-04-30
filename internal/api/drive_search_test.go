package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestDriveSearchResponseIncludesSnippetBreadcrumbSharingAndSensitivity
// proves SCN-038-010 at the API contract layer: when a search result is a
// drive_file artifact, the JSON response MUST expose snippet, folder
// breadcrumb, provider chip metadata, sharing badge, sensitivity badge,
// provider URL, and accessible action state so Screen 5 can render every
// required affordance without inferring missing data.
//
// Adversarial guards:
//   - the test fixture intentionally populates EVERY required drive field;
//     if SearchResult ever drops the Drive sub-object on serialization,
//     the JSON decode below will fail (Drive == nil) and the test will
//     fail loudly rather than silently weakening the contract;
//   - the snippet, breadcrumb, and provider URL assertions match exact
//     fixture values — a regression that returned empty strings would
//     fail (no tautology against zero-values);
//   - the availability assertion is "available" for the seeded fixture
//     and is checked together with tombstoned/permission_lost flags so
//     a future bug that flipped state never hides behind an unchecked
//     default.
func TestDriveSearchResponseIncludesSnippetBreadcrumbSharingAndSensitivity(t *testing.T) {
	driveResult := SearchResult{
		ArtifactID:   "drive:google:conn-038-scope4:air-fryer-manual",
		Title:        "Air Fryer Manual",
		ArtifactType: "drive_file",
		Summary:      "Owner manual for the air fryer model XL-2024.",
		SourceURL:    "https://drive.example/file/air-fryer-manual",
		Relevance:    "high",
		Explanation:  "Similarity: 0.82",
		CreatedAt:    "2026-04-30T12:00:00Z",
		Topics:       []string{"appliances", "kitchen"},
		Snippet:      "…air-fryer manual covers preheat steps, basket cleaning, and warranty…",
		Drive: &DriveSearchMetadata{
			ProviderID:       "google",
			ProviderURL:      "https://drive.example/file/air-fryer-manual",
			FolderBreadcrumb: []string{"Manuals", "Kitchen"},
			SharingState:     "private",
			Sensitivity:      "none",
			Availability:     "available",
			Tombstoned:       false,
			PermissionLost:   false,
			VersionChain:     []string{"rev-1"},
			OwnerLabel:       "fixture-owner@example.com",
			MimeType:         "application/pdf",
			ActionsEnabled:   true,
		},
	}

	se := &mockSearchEngine{
		results: []SearchResult{driveResult},
		total:   1,
		mode:    "semantic",
	}
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		SearchEngine: se,
	}

	body := `{"query": "air-fryer manual", "limit": 5}`
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.SearchHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var resp SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(resp.Results))
	}
	got := resp.Results[0]

	if got.Snippet == "" || got.Snippet != driveResult.Snippet {
		t.Fatalf("snippet = %q, want %q (snippet must round-trip on the wire)", got.Snippet, driveResult.Snippet)
	}
	if got.Drive == nil {
		t.Fatalf("drive metadata sub-object is nil — Screen 5 cannot render breadcrumb/sharing/sensitivity badges; raw=%s", rec.Body.String())
	}
	if got.Drive.ProviderID != "google" {
		t.Fatalf("drive.provider_id = %q, want google", got.Drive.ProviderID)
	}
	if got.Drive.ProviderURL == "" || got.Drive.ProviderURL != driveResult.Drive.ProviderURL {
		t.Fatalf("drive.provider_url = %q, want %q", got.Drive.ProviderURL, driveResult.Drive.ProviderURL)
	}
	if len(got.Drive.FolderBreadcrumb) != 2 || got.Drive.FolderBreadcrumb[0] != "Manuals" || got.Drive.FolderBreadcrumb[1] != "Kitchen" {
		t.Fatalf("drive.folder_breadcrumb = %v, want [Manuals Kitchen]", got.Drive.FolderBreadcrumb)
	}
	if got.Drive.SharingState != "private" {
		t.Fatalf("drive.sharing_state = %q, want private", got.Drive.SharingState)
	}
	if got.Drive.Sensitivity != "none" {
		t.Fatalf("drive.sensitivity = %q, want none", got.Drive.Sensitivity)
	}
	if got.Drive.Availability != "available" {
		t.Fatalf("drive.availability = %q, want available", got.Drive.Availability)
	}
	if got.Drive.Tombstoned || got.Drive.PermissionLost {
		t.Fatalf("drive.tombstoned=%v drive.permission_lost=%v, want both false for available file", got.Drive.Tombstoned, got.Drive.PermissionLost)
	}
	if !got.Drive.ActionsEnabled {
		t.Fatalf("drive.actions_enabled = false, want true so Screen 5 can render Open in Drive action; raw=%s", rec.Body.String())
	}
	if got.Drive.MimeType != "application/pdf" {
		t.Fatalf("drive.mime_type = %q, want application/pdf", got.Drive.MimeType)
	}
	if len(got.Drive.VersionChain) != 1 || got.Drive.VersionChain[0] != "rev-1" {
		t.Fatalf("drive.version_chain = %v, want [rev-1]", got.Drive.VersionChain)
	}

	// Adversarial: the on-the-wire JSON keys MUST match the documented
	// contract so Screen 5 can read them without speculative key probing.
	rawJSON := rec.Body.Bytes()
	for _, key := range []string{
		`"snippet":`,
		`"drive":`,
		`"folder_breadcrumb":`,
		`"sharing_state":`,
		`"sensitivity":`,
		`"availability":`,
		`"provider_url":`,
		`"actions_enabled":`,
	} {
		if !bytes.Contains(rawJSON, []byte(key)) {
			t.Fatalf("missing JSON key %s; payload=%s", key, string(rawJSON))
		}
	}
}

// TestDriveSearchResponseSurfacesTombstoneAndPermissionLossState proves the
// API never silently hides revoked or trashed drive artifacts: search
// results MUST keep them queryable but mark availability so Screen 5 can
// disable byte-delivery actions (SCN-038-012, design.md §8 / §11).
//
// Adversarial: if a future change defaulted tombstoned/permission_lost to
// false even when the upstream record is unavailable, the explicit
// availability+flag pair below would catch the regression.
func TestDriveSearchResponseSurfacesTombstoneAndPermissionLossState(t *testing.T) {
	results := []SearchResult{
		{
			ArtifactID:   "drive:google:conn-038-scope4:trashed-file",
			Title:        "Trashed Receipt",
			ArtifactType: "drive_file",
			Summary:      "Receipt that was trashed in the provider.",
			SourceURL:    "https://drive.example/file/trashed-file",
			Snippet:      "…receipt total $42.13 for airport lunch…",
			Drive: &DriveSearchMetadata{
				ProviderID:       "google",
				ProviderURL:      "https://drive.example/file/trashed-file",
				FolderBreadcrumb: []string{"Receipts", "Travel"},
				SharingState:     "private",
				Sensitivity:      "financial",
				Availability:     "tombstoned",
				Tombstoned:       true,
				ActionsEnabled:   false,
				MimeType:         "application/pdf",
			},
		},
		{
			ArtifactID:   "drive:google:conn-038-scope4:permission-lost-file",
			Title:        "Permission-lost Brief",
			ArtifactType: "drive_file",
			Summary:      "Internal brief; provider revoked access.",
			SourceURL:    "https://drive.example/file/permission-lost-file",
			Snippet:      "…internal Q3 launch brief outline…",
			Drive: &DriveSearchMetadata{
				ProviderID:       "google",
				ProviderURL:      "https://drive.example/file/permission-lost-file",
				FolderBreadcrumb: []string{"Strategy"},
				SharingState:     "shared",
				Sensitivity:      "none",
				Availability:     "permission_lost",
				PermissionLost:   true,
				ActionsEnabled:   false,
				MimeType:         "application/vnd.google-apps.document",
			},
		},
	}
	se := &mockSearchEngine{results: results, total: 2, mode: "semantic"}
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		SearchEngine: se,
	}
	body := `{"query": "receipt brief", "limit": 5}`
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.SearchHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
	for i, want := range []struct {
		availability   string
		tombstoned     bool
		permissionLost bool
		actionsEnabled bool
	}{
		{availability: "tombstoned", tombstoned: true, permissionLost: false, actionsEnabled: false},
		{availability: "permission_lost", tombstoned: false, permissionLost: true, actionsEnabled: false},
	} {
		got := resp.Results[i]
		if got.Drive == nil {
			t.Fatalf("result %d missing drive metadata", i)
		}
		if got.Drive.Availability != want.availability ||
			got.Drive.Tombstoned != want.tombstoned ||
			got.Drive.PermissionLost != want.permissionLost ||
			got.Drive.ActionsEnabled != want.actionsEnabled {
			t.Fatalf("result %d state mismatch: got availability=%q tombstoned=%v permission_lost=%v actions_enabled=%v; want %+v",
				i, got.Drive.Availability, got.Drive.Tombstoned, got.Drive.PermissionLost, got.Drive.ActionsEnabled, want)
		}
	}
}

// Compile-time guard: ensure the deprecated SearchResult zero value does
// not accidentally become the documented contract by referencing the new
// fields explicitly. Unused tests guard against a refactor silently
// dropping fields from the struct.
var _ = func() any {
	r := SearchResult{Snippet: "guard", Drive: &DriveSearchMetadata{ActionsEnabled: true}}
	_ = context.Background()
	return r
}
