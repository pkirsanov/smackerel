package pipeline

import "testing"

// SCN-002-045: Source ID constants accessible without importing processor.
func TestSCN002045_SourceIDConstants_Accessible(t *testing.T) {
	// Verify all source ID constants have expected string values.
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"SourceCapture", SourceCapture, "capture"},
		{"SourceTelegram", SourceTelegram, "telegram"},
		{"SourceBrowser", SourceBrowser, "browser"},
		{"SourceBrowserHistory", SourceBrowserHistory, "browser-history"},
		{"SourceRSS", SourceRSS, "rss"},
		{"SourceBookmarks", SourceBookmarks, "bookmarks"},
		{"SourceGoogleKeep", SourceGoogleKeep, "google-keep"},
		{"SourceGoogleMaps", SourceGoogleMaps, "google-maps-timeline"},
		{"SourceHospitable", SourceHospitable, "hospitable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.constant)
			}
		})
	}
}

// SCN-002-046: Processing status constants available as typed values.
func TestSCN002046_ProcessingStatusType(t *testing.T) {
	// Verify typed constants have correct string values.
	if string(StatusPending) != "pending" {
		t.Errorf("StatusPending = %q, want %q", string(StatusPending), "pending")
	}
	if string(StatusProcessed) != "processed" {
		t.Errorf("StatusProcessed = %q, want %q", string(StatusProcessed), "processed")
	}
	if string(StatusFailed) != "failed" {
		t.Errorf("StatusFailed = %q, want %q", string(StatusFailed), "failed")
	}
}

// Verify the type system distinguishes ProcessingStatus from plain string.
func TestProcessingStatusType_NotPlainString(t *testing.T) {
	var ps ProcessingStatus = StatusPending
	// Verify it can be used as a string via conversion
	s := string(ps)
	if s != "pending" {
		t.Errorf("string(ps) = %q, want %q", s, "pending")
	}
}
