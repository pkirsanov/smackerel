// Unit tests for spec 048 backup status loader and watcher.
//
// Test plan:
//
//   T-048-002a Valid status file round-trips through MarshalStatus +
//              LoadStatus.
//   T-048-002b Missing file returns os.ErrNotExist (not a generic error)
//              so callers can distinguish "no backup yet" from corruption.
//   T-048-002c Schema_version < 1 is rejected.
//   T-048-002d Unknown last_status is rejected (label cardinality guard).
//   T-048-002e Secret-shaped substrings in free-text fields are rejected
//              (defense in depth — the script's own redaction is primary).
//   T-048-002f Watcher.Poll is idempotent for gauge updates and strictly
//              monotonic for the counter — repeat polls on an unchanged
//              file increment runs_total exactly once.
//   T-048-002g Watcher.Poll on a missing file leaves prior gauge values
//              alone and returns (nil, nil).

package backup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSink struct {
	lastSuccess float64
	lastSize    float64
	runsByStat  map[string]int
}

func newFakeSink() *fakeSink {
	return &fakeSink{runsByStat: map[string]int{}}
}

func (f *fakeSink) SetLastSuccessUnixtime(v float64) { f.lastSuccess = v }
func (f *fakeSink) SetLastSizeBytes(v float64)       { f.lastSize = v }
func (f *fakeSink) IncRun(status string)             { f.runsByStat[status]++ }

func writeStatus(t *testing.T, dir string, s Status) string {
	t.Helper()
	raw, err := MarshalStatus(s)
	if err != nil {
		t.Fatalf("MarshalStatus: %v", err)
	}
	path := filepath.Join(dir, "status.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// T-048-002a — valid file round-trips.
func TestLoadStatus_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Status{
		SchemaVersion:       CurrentSchemaVersion,
		LastRunUnixtime:     1747140000,
		LastSuccessUnixtime: 1747140000,
		LastStatus:          "success",
		LastDurationMS:      4321,
		LastSizeBytes:       1024 * 1024,
		LastArtifactName:    "smackerel-2026-05-13-120000.sql.gz",
	}
	path := writeStatus(t, dir, want)
	got, err := LoadStatus(path)
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if got.LastRunUnixtime != want.LastRunUnixtime ||
		got.LastSuccessUnixtime != want.LastSuccessUnixtime ||
		got.LastStatus != want.LastStatus ||
		got.LastSizeBytes != want.LastSizeBytes ||
		got.LastArtifactName != want.LastArtifactName {
		t.Fatalf("round-trip mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// T-048-002b — missing file → os.ErrNotExist.
func TestLoadStatus_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	_, err := LoadStatus(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist; got %v", err)
	}
}

// T-048-002c — schema_version < 1 is rejected.
func TestLoadStatus_RejectsZeroSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":0,"last_status":"success"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadStatus(path)
	if err == nil {
		t.Fatal("expected error for schema_version=0")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error should name schema_version; got: %v", err)
	}
}

// T-048-002d — unknown last_status is rejected.
func TestLoadStatus_RejectsUnknownStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"last_status":"mysterious"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadStatus(path)
	if err == nil {
		t.Fatal("expected error for unknown last_status")
	}
	if !strings.Contains(err.Error(), "mysterious") {
		t.Fatalf("error should name offending status; got: %v", err)
	}
}

// T-048-002e — secret-shaped substrings in any free-text field are
// rejected. This is the spec 048 FR-048-003 redaction defense in depth.
func TestLoadStatus_RejectsSecretSubstrings(t *testing.T) {
	cases := []struct {
		name  string
		field string
		bad   string
	}{
		{"password in error", "last_error", "POSTGRES_PASSWORD=hunter2"},
		{"token in error", "last_error", "SMACKEREL_AUTH_TOKEN=deadbeef"},
		{"auth-key in artifact name", "last_artifact_name", "smackerel-AUTH_SIGNING_ACTIVE_PRIVATE_KEY=foo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			s := Status{
				SchemaVersion: CurrentSchemaVersion,
				LastStatus:    "failed",
			}
			switch c.field {
			case "last_error":
				s.LastError = c.bad
			case "last_artifact_name":
				s.LastArtifactName = c.bad
			}
			path := writeStatus(t, dir, s)
			_, err := LoadStatus(path)
			if err == nil {
				t.Fatalf("expected error for secret-shaped value in %s", c.field)
			}
			if !strings.Contains(err.Error(), "secret material") {
				t.Fatalf("error should name secret material rejection; got: %v", err)
			}
		})
	}
}

// T-048-002f — Watcher.Poll is idempotent for gauges and monotonic for
// counters. This is the contract that backs the alert rule: the
// `smackerel_backup_runs_total` counter MUST NOT inflate when the file
// is unchanged.
func TestWatcher_PollIdempotentAndMonotonic(t *testing.T) {
	dir := t.TempDir()
	path := writeStatus(t, dir, Status{
		SchemaVersion:       CurrentSchemaVersion,
		LastRunUnixtime:     1000,
		LastSuccessUnixtime: 1000,
		LastStatus:          "success",
		LastSizeBytes:       42,
	})
	sink := newFakeSink()
	w := NewWatcher(path, time.Second, sink)

	// Three polls on an unchanged file.
	for i := 0; i < 3; i++ {
		if _, err := w.Poll(); err != nil {
			t.Fatalf("Poll #%d: %v", i, err)
		}
	}
	if got, want := sink.runsByStat["success"], 1; got != want {
		t.Fatalf("expected runs_total{success}=1 after 3 polls on unchanged file; got %d", got)
	}
	if sink.lastSuccess != 1000 {
		t.Fatalf("expected lastSuccess=1000; got %v", sink.lastSuccess)
	}
	if sink.lastSize != 42 {
		t.Fatalf("expected lastSize=42; got %v", sink.lastSize)
	}

	// Now advance the file — a new backup ran and FAILED.
	writeStatus(t, dir, Status{
		SchemaVersion:       CurrentSchemaVersion,
		LastRunUnixtime:     2000,
		LastSuccessUnixtime: 1000, // still last success
		LastStatus:          "failed",
		LastSizeBytes:       0,
		LastError:           "pg_dump exited 1",
	})
	if _, err := w.Poll(); err != nil {
		t.Fatalf("Poll after failure: %v", err)
	}
	if got, want := sink.runsByStat["failed"], 1; got != want {
		t.Fatalf("expected runs_total{failed}=1 after one failed run; got %d", got)
	}
	if got, want := sink.runsByStat["success"], 1; got != want {
		t.Fatalf("expected runs_total{success}=1 still; got %d", got)
	}
	// LastSuccessUnixtime gauge holds the prior success.
	if sink.lastSuccess != 1000 {
		t.Fatalf("expected lastSuccess to hold prior success=1000; got %v", sink.lastSuccess)
	}
	if sink.lastSize != 0 {
		t.Fatalf("expected lastSize to reflect the failed run size=0; got %v", sink.lastSize)
	}
}

// T-048-002g — Watcher.Poll on a missing file returns (nil, nil) and
// does not touch the sink.
func TestWatcher_PollMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	sink := newFakeSink()
	w := NewWatcher(path, time.Second, sink)
	s, err := w.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if s != nil {
		t.Fatalf("expected nil status for missing file; got %+v", s)
	}
	if sink.lastSuccess != 0 || sink.lastSize != 0 {
		t.Fatalf("expected sink untouched; got lastSuccess=%v lastSize=%v", sink.lastSuccess, sink.lastSize)
	}
	if len(sink.runsByStat) != 0 {
		t.Fatalf("expected no counter increments; got %v", sink.runsByStat)
	}
}

// T-048-002h — NewWatcher panics on nil sink. A silent watcher would
// defeat the spec 049 alert contract; fail loud at construction.
func TestWatcher_NilSinkPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil sink")
		}
	}()
	_ = NewWatcher("/tmp/foo", time.Second, nil)
}

// T-102-C4-03 — cross-repo schema parity (spec 102 SCOPE-102-04).
//
// The knb self-hosted backup adapter (<deployment-owner>/<product>/<target>/backup.sh,
// write_backup_status) is the ONLY writer of the on-disk backup-status file
// this watcher reads. This test embeds a verbatim replica of that writer's
// heredoc (schema_version 1, the full 8-field set) and proves:
//
//  1. The knb-written JSON parses cleanly through LoadStatus at
//     CurrentSchemaVersion — the watcher can consume what the adapter emits.
//  2. The key SET the adapter writes is exactly the json-tag set backup.Status
//     models — neither repo can silently add, drop, or rename a field without
//     failing this test (the drift-lock the SCOPE-102-04 DoD requires).
//
// If backup.sh changes its heredoc, update the `knbWritten` literal here in the
// same change; if backup.Status changes its json tags, this test fails until
// the adapter and this literal are reconciled.
func TestLoadStatus_KnbAdapterSchemaParity(t *testing.T) {
	// Verbatim replica of <deployment-owner>/<product>/<target>/backup.sh::write_backup_status.
	// Field order + key names MUST match that heredoc byte-for-byte (values
	// are concrete stand-ins for the shell-expanded ${...} placeholders).
	const knbWritten = `{
  "schema_version": 1,
  "last_run_unixtime": 1783669740,
  "last_success_unixtime": 1783669740,
  "last_status": "success",
  "last_duration_ms": 4321,
  "last_size_bytes": 1048576,
  "last_artifact_name": "postgres-smackerel.sql.gz",
  "last_error": ""
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	if err := os.WriteFile(path, []byte(knbWritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// (1) The adapter-written shape parses through the watcher's loader.
	got, err := LoadStatus(path)
	if err != nil {
		t.Fatalf("knb-adapter-written status MUST parse via LoadStatus; got: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema parity: SchemaVersion=%d, want CurrentSchemaVersion=%d", got.SchemaVersion, CurrentSchemaVersion)
	}
	if got.LastRunUnixtime != 1783669740 || got.LastSuccessUnixtime != 1783669740 {
		t.Fatalf("timestamps mis-parsed: run=%d success=%d", got.LastRunUnixtime, got.LastSuccessUnixtime)
	}
	if got.LastStatus != "success" {
		t.Fatalf("last_status mis-parsed: %q", got.LastStatus)
	}
	if got.LastDurationMS != 4321 || got.LastSizeBytes != 1048576 {
		t.Fatalf("duration/size mis-parsed: dur=%d size=%d", got.LastDurationMS, got.LastSizeBytes)
	}
	if got.LastArtifactName != "postgres-smackerel.sql.gz" {
		t.Fatalf("last_artifact_name mis-parsed: %q", got.LastArtifactName)
	}

	// (2) Adversarial key-set parity. The knb writer's key set MUST equal the
	// json-tag set of backup.Status. LastError is forced non-empty so the
	// struct's `omitempty` tag still emits the key for the comparison.
	var knbKeys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(knbWritten), &knbKeys); err != nil {
		t.Fatalf("unmarshal knb keys: %v", err)
	}
	structJSON, err := json.Marshal(Status{
		SchemaVersion:       1,
		LastRunUnixtime:     1,
		LastSuccessUnixtime: 1,
		LastStatus:          "success",
		LastDurationMS:      1,
		LastSizeBytes:       1,
		LastArtifactName:    "x",
		LastError:           "forced-non-empty",
	})
	if err != nil {
		t.Fatalf("marshal struct: %v", err)
	}
	var structKeys map[string]json.RawMessage
	if err := json.Unmarshal(structJSON, &structKeys); err != nil {
		t.Fatalf("unmarshal struct keys: %v", err)
	}
	for k := range knbKeys {
		if _, ok := structKeys[k]; !ok {
			t.Fatalf("knb writer emits key %q that backup.Status does not model (cross-repo schema drift)", k)
		}
	}
	for k := range structKeys {
		if _, ok := knbKeys[k]; !ok {
			t.Fatalf("backup.Status models key %q that the knb writer does not emit (cross-repo schema drift)", k)
		}
	}
}
