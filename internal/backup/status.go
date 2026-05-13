// Status file contract for spec 048 backup automation.
//
// `scripts/commands/backup.sh` writes this JSON shape to the configured
// BACKUP_STATUS_FILE path after every run (success and failure). The Go
// core's metrics watcher reads the file and exposes the most recent
// outcome through `smackerel_backup_*` Prometheus metrics so spec 049
// alert rules can fire on missed/failed backups.
//
// The status file is local to the operator's host. The deploy adapter
// is responsible for shipping the actual backup artifact to off-host
// storage; only the LOCAL status record lives here.
//
// Secret hygiene: the script writes ONLY the fields below. No POSTGRES_PASSWORD,
// no SMACKEREL_AUTH_TOKEN, no API keys, ever. The status-file shape is
// asserted by `internal/backup/status_test.go` to keep this guarantee
// mechanical.

package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"
)

// Status is the on-disk shape backup.sh writes after each run.
//
// All timestamps are stored as Unix seconds (UTC). Strings carry only
// non-sensitive diagnostic context. The schema version is included so
// future evolution can be detected without breaking the watcher loop.
type Status struct {
	SchemaVersion       int    `json:"schema_version"`
	LastRunUnixtime     int64  `json:"last_run_unixtime"`
	LastSuccessUnixtime int64  `json:"last_success_unixtime"`
	LastStatus          string `json:"last_status"` // "success" | "failed"
	LastDurationMS      int64  `json:"last_duration_ms"`
	LastSizeBytes       int64  `json:"last_size_bytes"`
	LastArtifactName    string `json:"last_artifact_name"`
	LastError           string `json:"last_error,omitempty"`
}

// CurrentSchemaVersion is bumped when the on-disk JSON shape changes
// incompatibly. Backup.sh writes this constant; the watcher accepts
// any version >=1 but logs a warning when the version is newer than it
// knows how to read.
const CurrentSchemaVersion = 1

// AllowedStatuses is the closed set of values LastStatus may take.
// The watcher rejects any other value to keep the metric label cardinality
// bounded.
var AllowedStatuses = []string{"success", "failed"}

// LoadStatus reads and parses the status file at path. Returns
// (nil, os.ErrNotExist) when the file does not exist so callers can
// distinguish "no backup has run yet" from a parse error.
//
// Validation enforces:
//   - schema_version >= 1
//   - last_status is one of AllowedStatuses
//   - timestamps are not negative (zero is allowed — means "never")
//   - duration/size are not negative
//
// LoadStatus performs no other IO and never writes the file.
func LoadStatus(path string) (*Status, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is from SST config (BACKUP_STATUS_FILE)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fs.ErrNotExist
		}
		return nil, fmt.Errorf("read backup status file %q: %w", path, err)
	}
	var s Status
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse backup status file %q: %w", path, err)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid backup status file %q: %w", path, err)
	}
	return &s, nil
}

func (s *Status) validate() error {
	if s.SchemaVersion < 1 {
		return fmt.Errorf("schema_version must be >= 1; got %d", s.SchemaVersion)
	}
	if s.LastStatus != "" && !statusAllowed(s.LastStatus) {
		return fmt.Errorf("last_status %q is not in allowed set %v", s.LastStatus, AllowedStatuses)
	}
	if s.LastRunUnixtime < 0 {
		return fmt.Errorf("last_run_unixtime must be >= 0; got %d", s.LastRunUnixtime)
	}
	if s.LastSuccessUnixtime < 0 {
		return fmt.Errorf("last_success_unixtime must be >= 0; got %d", s.LastSuccessUnixtime)
	}
	if s.LastDurationMS < 0 {
		return fmt.Errorf("last_duration_ms must be >= 0; got %d", s.LastDurationMS)
	}
	if s.LastSizeBytes < 0 {
		return fmt.Errorf("last_size_bytes must be >= 0; got %d", s.LastSizeBytes)
	}
	// Secret-shape rejection: the status file MUST NEVER contain a value
	// that smells like a Postgres password or auth token. We reject any
	// substring that matches one of the forbidden key names — the script
	// would have to invent a field that doesn't exist in our struct to
	// trigger this, but defense in depth is cheap.
	for _, lit := range forbiddenSecretSubstrings {
		if strings.Contains(s.LastError, lit) || strings.Contains(s.LastArtifactName, lit) {
			return fmt.Errorf("backup status file appears to contain secret material (substring %q present) — refusing to load", lit)
		}
	}
	return nil
}

// forbiddenSecretSubstrings is the closed set of fragments that MUST
// NOT appear in any free-text field of the status file. Used as a
// defense-in-depth check; the script's own redaction is the primary
// guard.
var forbiddenSecretSubstrings = []string{
	"POSTGRES_PASSWORD=",
	"SMACKEREL_AUTH_TOKEN=",
	"TELEGRAM_BOT_TOKEN=",
	"AUTH_SIGNING_ACTIVE_PRIVATE_KEY=",
	"AUTH_AT_REST_HASHING_KEY=",
	"AUTH_BOOTSTRAP_TOKEN=",
	"LLM_API_KEY=",
}

func statusAllowed(s string) bool {
	for _, ok := range AllowedStatuses {
		if s == ok {
			return true
		}
	}
	return false
}

// MarshalStatus serializes a Status for the script side of the contract.
// Exposed so the integration test can build a representative payload
// without re-implementing the JSON shape.
func MarshalStatus(s Status) ([]byte, error) {
	s.SchemaVersion = CurrentSchemaVersion
	return json.MarshalIndent(s, "", "  ")
}

// Now is a package-level clock indirection so unit tests can pin time.
var Now = time.Now
