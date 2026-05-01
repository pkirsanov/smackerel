//go:build e2e

// Spec 038 Scope 6 — Sensitivity policy E2E test.
//
// Anchor: SCN-038-017 — sensitivity policy blocks unsafe auto-link
// sharing and bytes-mode delivery for sensitive Drive content.
//
// The test asserts the live runtime decision boundary across two
// surfaces:
//
//  1. policy.Engine refuses SaveLinkShare for medical sensitivity even
//     when the action requests an "anyone" audience link. This is
//     evaluated against the same engine constructor the runtime uses,
//     and through a metrics observer that mirrors the production wire-
//     up so the prometheus counter increments are exercised end-to-end.
//
//  2. policy.Engine downgrades Retrieval to SecureLink for identity
//     sensitivity, blocking byte transport (Telegram bytes path).
//
//  3. The drive_share_change_alerts table — backing Screen 7 alerts —
//     accepts a row representing a provider widening the audience of a
//     sensitive file, and the schema rejects rows with bogus
//     alert_status (CHECK constraint live).
package drive

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/drive/policy"
)

func TestDrivePolicyE2E_SensitiveFileNeverReturnsTelegramBytesOrPublicShare(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine := policy.NewEngineWithObserver(policy.NewMetricsObserver())

	// Step 1 — Refuse public link-share for medical sensitivity.
	v, err := engine.Evaluate(policy.Action{
		Surface:         policy.SurfaceSaveLinkShare,
		Sensitivity:     policy.SensitivityMedical,
		WouldCreateLink: true,
		LinkAudience:    "anyone",
	})
	if err != nil {
		t.Fatalf("evaluate medical link-share: %v", err)
	}
	if v.Decision != policy.DecisionRefuse {
		t.Fatalf("medical link-share decision = %q, want Refuse", v.Decision)
	}

	// Step 2 — Downgrade identity retrieval to SecureLink.
	v, err = engine.Evaluate(policy.Action{
		Surface:      policy.SurfaceRetrieval,
		Sensitivity:  policy.SensitivityIdentity,
		DeliveryMode: "bytes",
	})
	if err != nil {
		t.Fatalf("evaluate identity retrieval: %v", err)
	}
	if v.Decision != policy.DecisionDowngrade {
		t.Fatalf("identity retrieval decision = %q, want Downgrade", v.Decision)
	}
	if v.DowngradeMode != policy.DowngradeSecureLink {
		t.Fatalf("identity retrieval downgrade = %q, want SecureLink", v.DowngradeMode)
	}

	// Step 3 — drive_share_change_alerts row insertion via the live
	// schema. We need a real drive_files PK first.
	connID := uuid.New()
	ownerUserID := uuid.New()
	uniqueLabel := "scope6-e2e-policy-" + uuid.NewString()[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO drive_connections (
			id, provider_id, owner_user_id, account_label, access_mode, status
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		connID, "google", ownerUserID, uniqueLabel, "read_save", "healthy"); err != nil {
		t.Fatalf("insert drive_connection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_connections WHERE id=$1`, connID)
	})

	driveArtifactID := "test:scope6-e2e-policy-file:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw,
		                       content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'drive_file', 'med-record.pdf', '<bytes>',
		        $1, $1, NOW(), NOW())`, driveArtifactID); err != nil {
		t.Fatalf("insert drive_file artifact: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE id=$1`, driveArtifactID)
	})

	driveFileID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO drive_files (
			id, artifact_id, connection_id, provider_file_id, provider_url,
			title, mime_type, size_bytes, sensitivity, extraction_state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		driveFileID, driveArtifactID, connID, "p-"+uuid.NewString(),
		"https://drive.google.com/file/d/test", "medical-record.pdf",
		"application/pdf", int64(2048), "medical", "pending"); err != nil {
		t.Fatalf("insert drive_file: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_files WHERE id=$1`, driveFileID)
	})

	var alertID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO drive_share_change_alerts (
			drive_file_id, prior_audience, new_audience,
			sensitivity_after, alert_status, reason
		) VALUES ($1, 'private', 'public', 'medical', 'open',
			'provider widened audience above sensitivity baseline')
		RETURNING id`, driveFileID).Scan(&alertID); err != nil {
		t.Fatalf("insert drive_share_change_alerts: %v", err)
	}
	if alertID == 0 {
		t.Fatalf("alert_id = 0 (sequence missing)")
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_share_change_alerts WHERE id=$1`, alertID)
	})

	// Adversarial: bogus alert_status must be rejected by CHECK.
	_, badErr := pool.Exec(ctx, `
		INSERT INTO drive_share_change_alerts (
			drive_file_id, prior_audience, new_audience,
			sensitivity_after, alert_status, reason
		) VALUES ($1, 'private', 'public', 'medical', 'invalid_status', 'x')`,
		driveFileID)
	if badErr == nil {
		t.Fatalf("inserted alert with bogus alert_status — CHECK constraint missing")
	}
}
