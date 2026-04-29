//go:build integration

// Spec 038 Scope 1 — Shared Infrastructure Impact Sweep canary. This is the
// "Canary" row in scopes.md Test Plan. It runs against the live test stack
// and asserts that the three shared protected surfaces this scope depends
// on are simultaneously consistent:
//
//  1. Configuration SST — every required DRIVE_* env var is present in
//     the orchestrator-resolved environment (proves config generate
//     emitted them and the Docker test stack inherits them).
//  2. NATS contract — the live JetStream has the DRIVE stream with the
//     drive.> subjects pattern (proves Go EnsureStreams ran on startup
//     against the live NATS).
//  3. Migration 021 — drive_connections exists in the live test database
//     (proves migrations applied on the disposable test stack).
//
// If any of those three surfaces drift from the SST contract, the canary
// fails before the broader scope-1 integration tests run, giving a fast
// signal that the foundation is broken.
package drive

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestDriveFoundationCanary_ConfigNATSAndMigrationContracts is the canary
// row from scopes.md Scope 1 Test Plan.
func TestDriveFoundationCanary_ConfigNATSAndMigrationContracts(t *testing.T) {
	t.Run("config_DRIVE_env_vars_present", canaryConfigDriveEnvVars)
	t.Run("nats_DRIVE_stream_in_jetstream", canaryNATSDriveStream)
	t.Run("migration_021_drive_connections_present", canaryMigration021)
}

// canaryConfigDriveEnvVars asserts that the SST-resolved test env file
// (config/generated/test.env) declares every required DRIVE_* key. The
// orchestrator (./smackerel.sh test integration) regenerates this file
// before launching the test container; the canary reads it directly from
// the mounted /workspace so the assertion does not depend on whether the
// container also forwards DRIVE_* into its environment.
func canaryConfigDriveEnvVars(t *testing.T) {
	required := []string{
		"DRIVE_ENABLED",
		"DRIVE_CLASSIFICATION_ENABLED",
		"DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD",
		"DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION",
		"DRIVE_SCAN_PARALLELISM",
		"DRIVE_SCAN_BATCH_SIZE",
		"DRIVE_MONITOR_POLL_INTERVAL_SECONDS",
		"DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES",
		"DRIVE_POLICY_SENSITIVITY_DEFAULT",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE",
		"DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET",
		"DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES",
		"DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY",
		"DRIVE_LIMITS_MAX_FILE_SIZE_BYTES",
		"DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE",
		"DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL",
		"DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL",
		"DRIVE_PROVIDER_GOOGLE_API_BASE_URL",
		"DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS",
	}
	envPath := envFilePath(t)
	keys := loadEnvFileKeys(t, envPath)
	for _, key := range required {
		v, ok := keys[key]
		if !ok {
			t.Errorf("required key %q is not present in %s — config generate did not emit it", key, envPath)
			continue
		}
		if v == "" && !allowEmpty(key) {
			t.Errorf("required key %q is empty in %s", key, envPath)
		}
	}
}

// allowEmpty reports whether a DRIVE_* key may legitimately be empty in
// dev/test (OAuth secrets are placeholder strings until the operator
// configures real credentials, gated by drive.enabled=false at runtime).
func allowEmpty(key string) bool {
	switch key {
	case "DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID",
		"DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET":
		return true
	}
	return false
}

// envFilePath resolves the path to config/generated/test.env from inside
// the integration test container, where /workspace is the repo root.
func envFilePath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/workspace/config/generated/test.env",
		"config/generated/test.env",
		"../../../config/generated/test.env",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatalf("config/generated/test.env not found in any of: %v (./smackerel.sh test integration must regenerate it before tests run)", candidates)
	return ""
}

// loadEnvFileKeys parses a KEY=VALUE env file into a map. Lines starting
// with # or blank are skipped.
func loadEnvFileKeys(t *testing.T, path string) map[string]string {
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

// canaryNATSDriveStream asserts that the live JetStream has the DRIVE
// stream with the expected drive.> subjects pattern. This proves Go
// EnsureStreams ran during core startup against the live test NATS.
func canaryNATSDriveStream(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set — live test stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")

	opts := []natsgo.Option{natsgo.Name("smackerel-drive-canary")}
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

	stream, err := js.Stream(ctx, "DRIVE")
	if err != nil {
		t.Fatalf("DRIVE stream lookup: %v (must be created by Go EnsureStreams on startup)", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("DRIVE stream info: %v", err)
	}
	if info.Config.Name != "DRIVE" {
		t.Errorf("stream name = %q, want %q", info.Config.Name, "DRIVE")
	}
	foundPattern := false
	for _, s := range info.Config.Subjects {
		if s == "drive.>" {
			foundPattern = true
			break
		}
	}
	if !foundPattern {
		t.Errorf("DRIVE stream subjects = %v, want one of them to be %q", info.Config.Subjects, "drive.>")
	}

	// The four scope-1 drive subjects MUST be reachable through the
	// stream's pattern. JetStream does not pre-publish subjects, so we
	// verify by issuing a publish to each (workqueue retention drops
	// the message after the next pull; the canary does not pull). Each
	// publish failure indicates the stream pattern doesn't match.
	driveSubjects := []string{
		"drive.scan.request",
		"drive.scan.result",
		"drive.change.notify",
		"drive.health.report",
	}
	for _, subj := range driveSubjects {
		_, err := js.Publish(ctx, subj, []byte(`{"canary":true}`))
		if err != nil {
			t.Errorf("publish %q against DRIVE stream: %v", subj, err)
		}
	}

	// Negative assertion — a subject NOT under drive.> must not match
	// the DRIVE stream. If a future change widens the pattern, this
	// catches the regression.
	_, err = js.Publish(ctx, "not-drive.canary", []byte(`{}`))
	if err == nil {
		t.Error(`publish to "not-drive.canary" succeeded against DRIVE stream — pattern is too wide`)
	} else if !errors.Is(err, natsgo.ErrNoStreamResponse) && !errors.Is(err, natsgo.ErrNoResponders) {
		// Acceptable: any "no stream matched" error. Just log if it's
		// a different shape so we know the test is still meaningful.
		t.Logf("not-drive.canary publish failed as expected: %v", err)
	}
}

// canaryMigration021 asserts that drive_connections exists in the live
// test database, proving migration 021 applied during stack startup.
func canaryMigration021(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack not available")
	}
	pool := driveTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !tableExists(t, pool, ctx, "drive_connections") {
		t.Fatal("drive_connections table missing — migration 021 did not apply on the test stack")
	}
}
