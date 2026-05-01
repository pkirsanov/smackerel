//go:build integration

// Spec 038 Scope 6 — Sensitivity policy + low-confidence confirmation
// integration test. Asserts the live Postgres schema enforces the new
// constraints and that the policy engine, confirmation store, and
// share-change alert table cooperate end-to-end against the same pool.
package drive

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/drive/confirm"
	"github.com/smackerel/smackerel/internal/drive/policy"
)

// TestSensitivityPolicyDowngradesOrRejectsUnsafeDelivery exercises Scope 6
// against the live Postgres test stack:
//
//  1. policy.Engine REFUSES SaveLinkShare for medical content (proves
//     the auto-link refusal path).
//  2. policy.Engine DOWNGRADES Retrieval delivery to SecureLink for
//     identity content (proves bytes never leave Drive for sensitive
//     tiers).
//  3. The drive_share_change_alerts table accepts a row when a
//     provider-side share state widens audience above the user's
//     baseline (proves the schema migration is live and the contract
//     for Screen 7 alerts holds).
//  4. The drive_confirmations table stores a pending row, accepts a
//     resolution write through confirm.Store, and Resolve is exactly-
//     once even when invoked twice in succession (proves the
//     persistence contract behind the HTTP /v1/drive/confirmations/{id}
//     endpoint).
//
// Anchors: SCN-038-016, SCN-038-017.
func TestSensitivityPolicyDowngradesOrRejectsUnsafeDelivery(t *testing.T) {
	pool := driveTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1 — REFUSE auto link-share for medical sensitivity.
	engine := policy.NewEngine()
	verdict, err := engine.Evaluate(policy.Action{
		Surface:         policy.SurfaceSaveLinkShare,
		Sensitivity:     policy.SensitivityMedical,
		WouldCreateLink: true,
		LinkAudience:    "anyone",
	})
	if err != nil {
		t.Fatalf("evaluate medical link-share: %v", err)
	}
	if verdict.Decision != policy.DecisionRefuse {
		t.Fatalf("medical link-share decision = %q, want Refuse", verdict.Decision)
	}
	if verdict.Reason == "" {
		t.Fatalf("Refuse verdict missing Reason (UI cannot explain to user)")
	}

	// Step 2 — DOWNGRADE retrieval to SecureLink for identity.
	verdict, err = engine.Evaluate(policy.Action{
		Surface:      policy.SurfaceRetrieval,
		Sensitivity:  policy.SensitivityIdentity,
		DeliveryMode: "bytes",
	})
	if err != nil {
		t.Fatalf("evaluate identity retrieval: %v", err)
	}
	if verdict.Decision != policy.DecisionDowngrade {
		t.Fatalf("identity retrieval decision = %q, want Downgrade", verdict.Decision)
	}
	if verdict.DowngradeMode != policy.DowngradeSecureLink {
		t.Fatalf("identity retrieval downgrade = %q, want SecureLink", verdict.DowngradeMode)
	}

	// Step 3 — Insert a drive_share_change_alerts row through the live
	// schema. Need a real drive_files PK first.
	connID := uuid.New()
	ownerUserID := uuid.New()
	uniqueLabel := "scope6-policy-" + uuid.NewString()[:8]
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

	// drive_files requires an artifact_id FK target.
	driveArtifactID := "test:scope6-policy-file:" + uuid.NewString()
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
		"https://drive.google.com/file/d/test", "patient-record.pdf",
		"application/pdf", int64(2048), "medical", "pending"); err != nil {
		t.Fatalf("insert drive_file: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_files WHERE id=$1`, driveFileID)
	})

	// Insert the share-change alert. Schema enforces alert_status CHECK
	// and sensitivity_after CHECK — bad values would error here.
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
		t.Fatalf("alert_id = 0 (sequence default broken)")
	}

	// Adversarial: insert with an invalid alert_status — must fail.
	_, badErr := pool.Exec(ctx, `
		INSERT INTO drive_share_change_alerts (
			drive_file_id, prior_audience, new_audience,
			sensitivity_after, alert_status, reason
		) VALUES ($1, 'private', 'public', 'medical', 'invalid_status', 'x')`,
		driveFileID)
	if badErr == nil {
		t.Fatalf("inserted alert with bogus alert_status — CHECK constraint missing")
	}

	// Step 4 — Insert artifact + confirmation via confirm.Store; verify
	// exactly-once Resolve.
	artifactID := "test:scope6-confirm:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw,
		                       content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'drive_file', 'patient-record.pdf', '<medical>',
		        $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE id=$1`, artifactID)
	})

	store := confirm.NewStore(pool, 1*time.Hour)
	created, err := store.Create(ctx, confirm.CreateInput{
		Kind:             confirm.KindClassification,
		SourceArtifactID: artifactID,
		Payload: confirm.Payload{
			Classification: "medical",
			Sensitivity:    "medical",
			Confidence:     0.42,
			Title:          "patient-record.pdf",
			ProviderID:     "google",
		},
	})
	if err != nil {
		t.Fatalf("confirm.Create: %v", err)
	}
	if created.Status != confirm.StatusPending {
		t.Fatalf("created.Status = %q, want pending", created.Status)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_confirmations WHERE id=$1`, created.ID)
	})

	resolved, err := store.Resolve(ctx, created.ID, confirm.ChannelWeb, confirm.Choice{
		Outcome:           confirm.OutcomeReroute,
		NewClassification: "personal",
		NewSensitivity:    "none",
	})
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	if resolved.Status != confirm.StatusRerouted {
		t.Fatalf("first Resolve status = %q, want rerouted", resolved.Status)
	}
	if resolved.Channel != confirm.ChannelWeb {
		t.Fatalf("first Resolve channel = %q, want web", resolved.Channel)
	}

	// Adversarial: a second Resolve must not silently overwrite.
	_, secondErr := store.Resolve(ctx, created.ID, confirm.ChannelTelegram, confirm.Choice{
		Outcome: confirm.OutcomeNoSave,
	})
	if secondErr == nil {
		t.Fatalf("second Resolve succeeded — exactly-once contract broken")
	}

	// Verify the persisted row is the FIRST decision, not the second.
	var status, channel string
	if err := pool.QueryRow(ctx, `
		SELECT status, channel FROM drive_confirmations WHERE id=$1`,
		created.ID).Scan(&status, &channel); err != nil {
		t.Fatalf("re-fetch confirmation: %v", err)
	}
	if status != string(confirm.StatusRerouted) {
		t.Fatalf("persisted status = %q, want rerouted (first writer wins)", status)
	}
	if channel != string(confirm.ChannelWeb) {
		t.Fatalf("persisted channel = %q, want web", channel)
	}
}
