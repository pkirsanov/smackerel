package pipeline

import (
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestResolveTierFromMetadata_Nil(t *testing.T) {
	tier := resolveTierFromMetadata(nil)
	if tier != string(TierStandard) {
		t.Errorf("nil metadata should return standard, got %q", tier)
	}
}

func TestResolveTierFromMetadata_Empty(t *testing.T) {
	tier := resolveTierFromMetadata(map[string]interface{}{})
	if tier != string(TierStandard) {
		t.Errorf("empty metadata should return standard, got %q", tier)
	}
}

func TestResolveTierFromMetadata_WithTier(t *testing.T) {
	tests := []struct {
		name     string
		tier     string
		expected string
	}{
		{"full", "full", "full"},
		{"standard", "standard", "standard"},
		{"light", "light", "light"},
		{"metadata", "metadata", "metadata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]interface{}{"processing_tier": tt.tier}
			got := resolveTierFromMetadata(m)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestResolveTierFromMetadata_EmptyTierString(t *testing.T) {
	m := map[string]interface{}{"processing_tier": ""}
	tier := resolveTierFromMetadata(m)
	if tier != string(TierStandard) {
		t.Errorf("empty tier string should return standard, got %q", tier)
	}
}

func TestResolveTierFromMetadata_WrongType(t *testing.T) {
	m := map[string]interface{}{"processing_tier": 42}
	tier := resolveTierFromMetadata(m)
	if tier != string(TierStandard) {
		t.Errorf("non-string tier should return standard, got %q", tier)
	}
}

func TestNewRawArtifactPublisher(t *testing.T) {
	pub := NewRawArtifactPublisher(nil, nil)
	if pub == nil {
		t.Fatal("expected non-nil publisher")
	}
	if pub.DB != nil {
		t.Error("expected nil DB")
	}
	if pub.NATS != nil {
		t.Error("expected nil NATS")
	}
}

func TestRawArtifactPublisher_ImplementsInterface(t *testing.T) {
	// Compile-time check that RawArtifactPublisher satisfies ArtifactPublisher.
	var _ connector.ArtifactPublisher = (*RawArtifactPublisher)(nil)
}

func TestNATSProcessPayload_MetadataField(t *testing.T) {
	payload := NATSProcessPayload{
		ArtifactID:  "test-1",
		ContentType: "email",
		RawText:     "some text",
		SourceID:    "gmail",
		Metadata: map[string]interface{}{
			"action_items": []string{"Review Q3 budget"},
			"from":         "boss@company.com",
		},
	}

	if payload.Metadata == nil {
		t.Fatal("metadata should not be nil")
	}
	if _, ok := payload.Metadata["action_items"]; !ok {
		t.Error("metadata should contain action_items")
	}
	if _, ok := payload.Metadata["from"]; !ok {
		t.Error("metadata should contain from")
	}
}
