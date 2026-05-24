package ntfy

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNtfyScanSubscriptionStatesRejectsMalformedRedactionState(t *testing.T) {
	rows := &ntfyScanRows{rows: []ntfyScanRow{malformedSubscriptionStateRow([]byte(`{"status":`))}}
	states, err := scanSubscriptionStates(rows)
	if err == nil {
		t.Fatalf("expected malformed subscription redaction_state decode error, got states=%+v", states)
	}
	for _, required := range []string{"decode ntfy subscription redaction state", "ntfy-source", "home-lab-alerts"} {
		if !strings.Contains(err.Error(), required) {
			t.Fatalf("subscription redaction_state error missing %q context: %v", required, err)
		}
	}
}

func TestNtfyScanDeadLettersRejectsMalformedRedactionState(t *testing.T) {
	rows := &ntfyScanRows{rows: []ntfyScanRow{malformedDeadLetterRow([]byte(`{"status":`))}}
	records, err := scanDeadLetters(rows)
	if err == nil {
		t.Fatalf("expected malformed dead-letter redaction_state decode error, got records=%+v", records)
	}
	for _, required := range []string{"decode ntfy dead-letter redaction state", "ntfy-dlq-1", "ntfy-source"} {
		if !strings.Contains(err.Error(), required) {
			t.Fatalf("dead-letter redaction_state error missing %q context: %v", required, err)
		}
	}
}

type ntfyScanRows struct {
	rows  []ntfyScanRow
	index int
	err   error
}

type ntfyScanRow struct {
	scan func(dest ...any) error
}

func (rows *ntfyScanRows) Close() {}

func (rows *ntfyScanRows) Err() error {
	return rows.err
}

func (rows *ntfyScanRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (rows *ntfyScanRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (rows *ntfyScanRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.index++
	return true
}

func (rows *ntfyScanRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.rows) {
		return fmt.Errorf("ntfy scan test row index out of range")
	}
	return rows.rows[rows.index-1].scan(dest...)
}

func (rows *ntfyScanRows) Values() ([]any, error) {
	return nil, nil
}

func (rows *ntfyScanRows) RawValues() [][]byte {
	return nil
}

func (rows *ntfyScanRows) Conn() *pgx.Conn {
	return nil
}

func malformedSubscriptionStateRow(redactionJSON []byte) ntfyScanRow {
	now := time.Date(2026, 5, 24, 23, 59, 0, 0, time.UTC)
	return ntfyScanRow{scan: func(dest ...any) error {
		*dest[0].(*string) = "ntfy-source"
		*dest[1].(*string) = "home-lab-alerts"
		*dest[2].(*string) = "webhook"
		*dest[3].(*string) = "webhook"
		*dest[4].(*string) = SubscriptionConnected
		*dest[5].(*string) = "evt-redaction-state"
		*dest[6].(**time.Time) = &now
		*dest[7].(**time.Time) = nil
		*dest[8].(**time.Time) = nil
		*dest[9].(**time.Time) = &now
		*dest[10].(*int) = 0
		*dest[11].(*bool) = false
		*dest[12].(*int) = 0
		*dest[13].(*int) = 3
		*dest[14].(*string) = ""
		*dest[15].(*string) = ""
		*dest[16].(*[]byte) = append([]byte(nil), redactionJSON...)
		*dest[17].(*time.Time) = now
		*dest[18].(*time.Time) = now
		return nil
	}}
}

func malformedDeadLetterRow(redactionJSON []byte) ntfyScanRow {
	now := time.Date(2026, 5, 24, 23, 59, 0, 0, time.UTC)
	return ntfyScanRow{scan: func(dest ...any) error {
		*dest[0].(*string) = "ntfy-dlq-1"
		*dest[1].(*string) = "ntfy-source"
		*dest[2].(*string) = "home-lab-alerts"
		*dest[3].(*string) = "evt-redaction-state"
		*dest[4].(*string) = "message"
		*dest[5].(*time.Time) = now
		*dest[6].(*string) = "sha256:redaction-state"
		*dest[7].(*int) = 128
		*dest[8].(*string) = PayloadRefHashOnly
		*dest[9].(*[]byte) = nil
		*dest[10].(*string) = ""
		*dest[11].(*string) = "safe preview"
		*dest[12].(*string) = DeadLetterSinkUnavailable
		*dest[13].(*string) = "source sink unavailable"
		*dest[14].(*bool) = true
		*dest[15].(*string) = ReplayStatusPending
		*dest[16].(*int) = 0
		*dest[17].(**time.Time) = nil
		*dest[18].(*[]byte) = append([]byte(nil), redactionJSON...)
		*dest[19].(*time.Time) = now
		*dest[20].(*time.Time) = now
		return nil
	}}
}
