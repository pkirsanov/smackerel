//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosUpload_TelegramMobileWebEnterSamePipeline covers
// SCN-040-010: every capture channel (Telegram, mobile, web) MUST land
// in the unified photo pipeline. The integration check exercises
// Store.PublishPhotoEvent directly with the channel metadata that the
// API handler forwards (the API surface is owned by the unit test in
// internal/api/photos_upload_test.go and the e2e test in
// tests/e2e/photos_telegram_test.go). The strict assertion here is:
// rows persist with the inbound channel + ref preserved AND the
// cross-channel query helper returns each row.
func TestPhotosUpload_TelegramMobileWebEnterSamePipeline(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uniqueRef := testID(t)
	channels := []struct {
		channel   photolib.SourceChannel
		sourceRef string
	}{
		{photolib.SourceChannelTelegram, "tg:1001:" + uniqueRef},
		{photolib.SourceChannelMobile, "device:" + uniqueRef},
		{photolib.SourceChannelWeb, "session:" + uniqueRef},
	}
	persisted := make(map[photolib.SourceChannel]string, len(channels))
	for _, c := range channels {
		event := photolib.SyntheticPhotoEvent()
		event.ProviderRef = string(c.channel) + ":upload:" + uniqueRef + ":" + uuid.NewString()
		event.ContentHash = "sha256:" + strings.ReplaceAll(event.ProviderRef, "/", "-")
		event.MediaRole = photolib.MediaRoleCameraOriginal
		event.SourceChannel = c.channel
		event.SourceRef = c.sourceRef
		record, err := store.PublishPhotoEvent(ctx, "test-upload-"+string(c.channel), string(c.channel), event)
		if err != nil {
			t.Fatalf("publish %s upload: %v", c.channel, err)
		}
		cleanupPhoto(t, record.ArtifactID)
		persisted[c.channel] = record.ArtifactID

		if record.SourceChannel != c.channel {
			t.Fatalf("channel %s persisted as %q", c.channel, record.SourceChannel)
		}
		if record.SourceRef != c.sourceRef {
			t.Fatalf("source_ref for %s mismatched: got %q want %q", c.channel, record.SourceRef, c.sourceRef)
		}

		rows, err := store.ListPhotosBySource(ctx, c.channel, c.sourceRef)
		if err != nil {
			t.Fatalf("list by source for %s: %v", c.channel, err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row for %s/%s, got %d", c.channel, c.sourceRef, len(rows))
		}
		if rows[0].ID != record.ID {
			t.Fatalf("list-by-source returned wrong row for %s: %s vs %s", c.channel, rows[0].ID, record.ID)
		}
	}

	if len(persisted) != 3 {
		t.Fatalf("expected 3 channels persisted, got %d", len(persisted))
	}
}
