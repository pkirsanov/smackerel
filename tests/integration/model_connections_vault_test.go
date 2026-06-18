//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
)

// Spec 096 SCOPE-02 (SCN-096-W05) — prove the reversible encrypted-at-rest
// credential vault persists to a REAL ephemeral Postgres and round-trips:
// Encrypt → INSERT model_provider_connections → re-read the at-rest columns →
// Decrypt recovers the original SYNTHETIC bundle. The row stores only
// ciphertext + nonce + key_version + last-4 redaction (no plaintext column);
// the stored ciphertext does not contain the plaintext. Hits the REAL DB — no
// query interception/mocking. Test isolation (env-pollution + test-environment
// policy): the disposable test stack's ephemeral Postgres only, a SYNTHETIC
// master key (never a real provider secret), and the row is cleaned up.

const modelProviderConnectionsTable = "model_provider_connections"

func TestVault_PersistRoundTripTestMasterKey_Spec096(t *testing.T) {
	pool := testPool(t) // skips when DATABASE_URL is unset (live stack not available)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Apply migration 061 idempotently to guarantee the table exists even if
	// this disposable DB predates the migration being embedded.
	migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "internal", "db", "migrations", "061_model_provider_connections.sql"))
	if err != nil {
		t.Fatalf("read migration 061: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply migration 061: %v", err)
	}
	if !tableExists(t, ctx, pool, modelProviderConnectionsTable) {
		t.Fatalf("expected table %s to exist after migrate", modelProviderConnectionsTable)
	}

	// SYNTHETIC 32-byte master key — NEVER a real provider secret.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("generate synthetic master key: %v", err)
	}
	vault, err := connvault.NewSecretVault(base64.StdEncoding.EncodeToString(raw), 1)
	if err != nil {
		t.Fatalf("build vault: %v", err)
	}

	connID := "anthropic-itest-" + time.Now().Format("20060102150405.000000000")
	const kind = "anthropic"
	const syntheticSecret = "sk-ant-synthetic-itest-DEADbeef0000WXYZ"
	bundle := map[string]string{"api_key": syntheticSecret}

	rec, err := vault.Encrypt(connID, kind, bundle)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Always clean up the disposable row.
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, err := pool.Exec(cctx, "DELETE FROM model_provider_connections WHERE connection_id = $1", connID); err != nil {
			t.Logf("cleanup model_provider_connections %s failed: %v", connID, err)
		}
	})

	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO model_provider_connections
			(connection_id, provider_kind, enabled,
			 secret_ciphertext, secret_nonce, secret_key_version, secret_redaction,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, connID, kind, true,
		rec.Ciphertext, rec.Nonce, rec.KeyVersion, rec.Redaction,
		now, now); err != nil {
		t.Fatalf("insert encrypted row: %v", err)
	}

	// Re-read ONLY the at-rest columns (there is no plaintext column).
	var (
		gotKind       string
		gotEnabled    bool
		gotCiphertext []byte
		gotNonce      []byte
		gotKeyVersion int
		gotRedaction  string
	)
	if err := pool.QueryRow(ctx, `
		SELECT provider_kind, enabled, secret_ciphertext, secret_nonce, secret_key_version, secret_redaction
		FROM model_provider_connections
		WHERE connection_id = $1
	`, connID).Scan(&gotKind, &gotEnabled, &gotCiphertext, &gotNonce, &gotKeyVersion, &gotRedaction); err != nil {
		t.Fatalf("re-read row: %v", err)
	}

	// At-rest protection: the stored ciphertext must NOT contain the plaintext.
	if bytes.Contains(gotCiphertext, []byte(syntheticSecret)) {
		t.Fatal("stored ciphertext contains the plaintext secret — not encrypted at rest")
	}
	// The persisted redaction is last-4 only.
	if gotRedaction != "…WXYZ" {
		t.Fatalf("persisted secret_redaction = %q, want last-4 hint …WXYZ", gotRedaction)
	}
	if gotKeyVersion != 1 {
		t.Fatalf("persisted secret_key_version = %d, want 1", gotKeyVersion)
	}
	if gotKind != kind || !gotEnabled {
		t.Fatalf("re-read row mismatch: kind=%q enabled=%v", gotKind, gotEnabled)
	}

	// Decrypt the re-read record in-core → recover the original bundle (reversible).
	recFromDB := connvault.VaultRecord{
		ConnectionID: connID,
		Kind:         gotKind,
		Ciphertext:   gotCiphertext,
		Nonce:        gotNonce,
		KeyVersion:   gotKeyVersion,
		Redaction:    gotRedaction,
	}
	got, err := vault.Decrypt(recFromDB)
	if err != nil {
		t.Fatalf("decrypt re-read record: %v", err)
	}
	if got["api_key"] != syntheticSecret {
		t.Fatalf("decrypted api_key = %q, want the original synthetic secret", got["api_key"])
	}
}
