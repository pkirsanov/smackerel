//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestPhotosFoundation_ConfigNATSAndSchemaLiveStack(t *testing.T) {
	t.Run("config_PHOTOS_env_vars_present", canaryConfigPhotosEnvVars)
	t.Run("nats_PHOTOS_stream_in_jetstream", canaryNATSPhotosStream)
	t.Run("migration_025_photos_present", canaryMigration025Photos)
}

func TestPhotosFoundation_SyntheticPhotoPersistsProviderNeutralShape(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = testID(t)
	event.ContentHash = "sha256:" + strings.ReplaceAll(event.ProviderRef, "/", "-")
	store := photolib.NewStore(pool)
	record, err := store.PublishPhotoEvent(ctx, "connector-040", "synthetic", event)
	if err != nil {
		t.Fatalf("PublishPhotoEvent: %v", err)
	}
	cleanupPhoto(t, record.ArtifactID)

	if record.Provider != "synthetic" {
		t.Fatalf("provider = %q, want synthetic", record.Provider)
	}
	if record.ProviderRef != event.ProviderRef {
		t.Fatalf("provider_ref = %q, want %q", record.ProviderRef, event.ProviderRef)
	}
	if record.MediaRole != photolib.MediaRoleCameraOriginal {
		t.Fatalf("media_role = %q, want %q", record.MediaRole, photolib.MediaRoleCameraOriginal)
	}
	if record.Sensitivity != photolib.SensitivityNone {
		t.Fatalf("sensitivity = %q, want none", record.Sensitivity)
	}
	if strings.Contains(string(record.RawProvider), "provider_specific") {
		t.Fatalf("raw_provider leaked forbidden provider_specific marker: %s", string(record.RawProvider))
	}

	var artifactType string
	var sourceID string
	if err := pool.QueryRow(ctx, `SELECT artifact_type, source_id FROM artifacts WHERE id=$1`, record.ArtifactID).Scan(&artifactType, &sourceID); err != nil {
		t.Fatalf("read linked artifact: %v", err)
	}
	if artifactType != "photo" || sourceID != "photos:synthetic" {
		t.Fatalf("artifact shape = type=%q source_id=%q, want photo/photos:synthetic", artifactType, sourceID)
	}
}

func canaryConfigPhotosEnvVars(t *testing.T) {
	required := []string{
		"PHOTOS_ENABLED",
		"PHOTOS_SCAN_PARALLELISM",
		"PHOTOS_SCAN_BATCH_SIZE",
		"PHOTOS_SCAN_MAX_FILE_SIZE_BYTES",
		"PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS",
		"PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD",
		"PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD",
		"PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD",
		"PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS",
		"PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS",
		"PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS",
		"PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES",
		"PHOTOS_POLICY_ACTIONS_MAX_SCOPE_SIZE",
		"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL",
		"PHOTOS_INTELLIGENCE_EMBED_MODEL",
		"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL",
		"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL",
		"PHOTOS_INTELLIGENCE_OCR_MODEL",
		"PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR",
		"PHOTOS_PROVIDER_IMMICH_ENABLED",
		"PHOTOS_PROVIDER_IMMICH_BASE_URL",
		"PHOTOS_PROVIDER_IMMICH_API_KEY",
		"PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS",
		"PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS",
	}
	envPath := integrationEnvFilePath(t)
	keys := loadIntegrationEnvFileKeys(t, envPath)
	for _, key := range required {
		value, ok := keys[key]
		if !ok {
			t.Errorf("required key %q is not present in %s", key, envPath)
			continue
		}
		if value == "" && !allowEmptyPhotoKey(key) {
			t.Errorf("required key %q is empty in %s", key, envPath)
		}
	}
}

func allowEmptyPhotoKey(key string) bool {
	switch key {
	case "PHOTOS_PROVIDER_IMMICH_BASE_URL", "PHOTOS_PROVIDER_IMMICH_API_KEY":
		return true
	}
	return false
}

func integrationEnvFilePath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/workspace/config/generated/test.env",
		"config/generated/test.env",
		"../config/generated/test.env",
		"../../config/generated/test.env",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatalf("config/generated/test.env not found in any of: %v", candidates)
	return ""
}

func loadIntegrationEnvFileKeys(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file %s: %v", path, err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		out[line[:eq]] = line[eq+1:]
	}
	return out
}

func canaryNATSPhotosStream(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set — live test stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	opts := []natsgo.Option{natsgo.Name("smackerel-photos-canary")}
	if authToken != "" {
		opts = append(opts, natsgo.Token(authToken))
	}
	nc, err := natsgo.Connect(natsURL, opts...)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := js.Stream(ctx, "PHOTOS")
	if err != nil {
		t.Fatalf("PHOTOS stream lookup: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("PHOTOS stream info: %v", err)
	}
	if info.Config.Name != "PHOTOS" {
		t.Fatalf("stream name = %q, want PHOTOS", info.Config.Name)
	}
	if !containsString(info.Config.Subjects, "photos.>") {
		t.Fatalf("PHOTOS stream subjects = %v, want photos.>", info.Config.Subjects)
	}
	for _, subject := range []string{"photos.classify", "photos.classified", "photos.embed", "photos.embedded"} {
		_, err := js.Publish(ctx, subject, []byte(`{"canary":true}`))
		if err != nil {
			t.Errorf("publish %q against PHOTOS stream: %v", subject, err)
		}
	}
	_, err = js.Publish(ctx, "photoz.classify", []byte(`{}`))
	if err == nil {
		t.Error(`publish to "photoz.classify" succeeded — PHOTOS pattern is too wide`)
	} else if !errors.Is(err, natsgo.ErrNoStreamResponse) && !errors.Is(err, natsgo.ErrNoResponders) {
		t.Logf("photoz.classify publish failed as expected: %v", err)
	}
}

func canaryMigration025Photos(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, table := range []string{"photos", "photo_capabilities", "photo_sync_state", "photo_audit_events"} {
		if !integrationTableExists(t, pool, ctx, table) {
			t.Fatalf("%s table missing — migration 025 did not apply on the test stack", table)
		}
	}
}

func integrationTableExists(t *testing.T, pool *pgxpool.Pool, ctx context.Context, table string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists); err != nil {
		t.Fatalf("table exists query for %s: %v", table, err)
	}
	return exists
}

func cleanupPhoto(t *testing.T, artifactID string) {
	t.Helper()
	pool := testPool(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE id=$1`, artifactID); err != nil {
			t.Logf("cleanup photo artifact %s failed: %v", artifactID, err)
		}
	})
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
