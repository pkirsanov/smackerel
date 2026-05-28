// Spec 058 Scope 1 — SST validation tests for ExtensionIngestConfig.
package config

import (
	"strings"
	"testing"
)

// TestExtensionIngestConfig_Validate_RejectsEachMissingField asserts
// that Validate() names every required field that is zero-valued or
// empty. The test iterates one field at a time so the operator-facing
// error message remains specific to the offending field.
func TestExtensionIngestConfig_Validate_RejectsEachMissingField(t *testing.T) {
	base := ExtensionIngestConfig{
		Enabled:                   true,
		MaxBatchItems:             256,
		MaxBodyBytes:              1 << 20,
		DefaultDedupWindowSeconds: 1800,
		AcceptedContentTypes:      []string{"bookmark", "browser_history_visit"},
		RequiredTokenScope:        "extension:bookmarks,history",
	}

	cases := []struct {
		name     string
		mutate   func(c *ExtensionIngestConfig)
		wantText string
	}{
		{"MaxBatchItems_zero", func(c *ExtensionIngestConfig) { c.MaxBatchItems = 0 }, "EXTENSION_INGEST_MAX_BATCH_ITEMS"},
		{"MaxBodyBytes_zero", func(c *ExtensionIngestConfig) { c.MaxBodyBytes = 0 }, "EXTENSION_INGEST_MAX_BODY_BYTES"},
		{"DefaultDedupWindowSeconds_zero", func(c *ExtensionIngestConfig) { c.DefaultDedupWindowSeconds = 0 }, "EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS"},
		{"AcceptedContentTypes_empty", func(c *ExtensionIngestConfig) { c.AcceptedContentTypes = nil }, "EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES"},
		{"AcceptedContentTypes_blank_entry", func(c *ExtensionIngestConfig) { c.AcceptedContentTypes = []string{"bookmark", ""} }, "EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES"},
		{"RequiredTokenScope_empty", func(c *ExtensionIngestConfig) { c.RequiredTokenScope = "" }, "EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE"},
		{"RequiredTokenScope_whitespace", func(c *ExtensionIngestConfig) { c.RequiredTokenScope = "   " }, "EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			// Defensive deep copy of the slice so case mutations do not bleed.
			cfg.AcceptedContentTypes = append([]string(nil), base.AcceptedContentTypes...)
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected Validate() to reject; got nil error")
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("expected error to name %q; got %v", tc.wantText, err)
			}
		})
	}
}

// TestExtensionIngestConfig_Validate_AcceptsFullyPopulated proves the
// positive path: a populated config returns nil.
func TestExtensionIngestConfig_Validate_AcceptsFullyPopulated(t *testing.T) {
	cfg := ExtensionIngestConfig{
		Enabled:                   true,
		MaxBatchItems:             256,
		MaxBodyBytes:              1 << 20,
		DefaultDedupWindowSeconds: 1800,
		AcceptedContentTypes:      []string{"bookmark", "browser_history_visit"},
		RequiredTokenScope:        "extension:bookmarks,history",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned unexpected error: %v", err)
	}
}
