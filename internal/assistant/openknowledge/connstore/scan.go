package connstore

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
)

// ErrNoRow is returned by SetEnabled when no runtime row exists for the slot.
var ErrNoRow = errors.New("connstore: no runtime row for connection")

// errNoRow is the internal sentinel Get maps pgx.ErrNoRows to (so Get can
// translate it into found=false without leaking the pgx type).
var errNoRow = errors.New("connstore: row not found")

// scanRecord scans a single QueryRow result into a Record. A pgx.ErrNoRows is
// translated into the internal errNoRow sentinel.
func scanRecord(row pgx.Row) (Record, error) {
	return scanInto(row.Scan)
}

// scanRecordRows scans the current row of a multi-row result into a Record.
func scanRecordRows(rows pgx.Rows) (Record, error) {
	return scanInto(rows.Scan)
}

// scanInto reads the SELECT column list (selectColumns) into a Record. The
// nullable secret + last-test columns scan into pointer destinations so a SQL
// NULL maps to the zero value (nil secret, empty last-test state) rather than a
// scan error. The credential record is reconstructed ONLY when all three cipher
// columns are present.
func scanInto(scan func(...any) error) (Record, error) {
	var (
		connID       string
		kind         string
		enabled      bool
		ciphertext   []byte
		nonce        []byte
		keyVersion   *int
		redaction    *string
		lastTestedAt *time.Time
		lastOutcome  *string
		lastDetail   *string
		createdAt    time.Time
		updatedAt    time.Time
	)
	if err := scan(
		&connID, &kind, &enabled,
		&ciphertext, &nonce, &keyVersion, &redaction,
		&lastTestedAt, &lastOutcome, &lastDetail,
		&createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, errNoRow
		}
		return Record{}, err
	}

	rec := Record{
		ConnectionID: connID,
		ProviderKind: kind,
		Enabled:      enabled,
		LastTestedAt: lastTestedAt,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	if lastOutcome != nil {
		rec.LastTestOutcome = *lastOutcome
	}
	if lastDetail != nil {
		rec.LastTestDetail = *lastDetail
	}
	if len(ciphertext) > 0 && len(nonce) > 0 && keyVersion != nil {
		red := ""
		if redaction != nil {
			red = *redaction
		}
		rec.Secret = &connvault.VaultRecord{
			ConnectionID: connID,
			Kind:         kind,
			Ciphertext:   ciphertext,
			Nonce:        nonce,
			KeyVersion:   *keyVersion,
			Redaction:    red,
		}
	}
	return rec, nil
}
