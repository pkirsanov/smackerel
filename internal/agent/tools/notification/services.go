// Package notification registers the spec 061 SCOPE-03 notification
// scenario's two agent tools:
//
//   - notification_propose (side_effect_class: read) — extracts
//     {what, when} from the LLM-parsed args, generates a ULID
//     confirm_ref, persists the opaque payload to a ConfirmStore with
//     TTL = assistant.skills.notifications.confirm_timeout, and
//     returns phase="proposed". If the LLM cannot resolve a slot
//     (typically "when"), the tool emits phase="slot_missing" with
//     suggested options instead — no confirm_ref is issued.
//
//   - notification_execute (side_effect_class: write) — given a
//     confirm_ref, reads the pending payload from the ConfirmStore
//     and registers a job with the spec 054 scheduler. The
//     scheduler call carries Source ("assistant") and Originator
//     ("user:" + userID) so the resulting reminder is attributable
//     in spec 054's job lineage.
//
// SCOPE-03 wires the tool *handlers* against minimal local interfaces
// (ConfirmStore, Scheduler). SCOPE-08 will inject concrete
// implementations (Postgres-backed confirm store, spec 054 scheduler
// binding). Until SetServices is called the handlers return a real Go
// error so the trace surfaces the misconfiguration immediately
// instead of silently dropping reminders.
package notification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Tool names registered by this package. Wiring and allowlist code
// MUST consult these constants rather than hard-coding strings.
const (
	ToolPropose = "notification_propose"
	ToolExecute = "notification_execute"
)

// ConfirmStore is the minimum surface notification_propose and
// notification_execute need from the spec 054 / SCOPE-08 confirm
// substrate.
//
// Put MUST be idempotent on ref: repeated Put for the same ref
// replaces the payload (callers should pick a fresh ULID per
// proposal). Get returns ("", false, nil) when the ref is missing or
// expired.
type ConfirmStore interface {
	Put(ctx context.Context, ref string, payload string, ttl time.Duration) error
	Get(ctx context.Context, ref string) (payload string, ok bool, err error)
}

// Scheduler is the minimum surface notification_execute needs from
// the spec 054 scheduler. The Source / Originator pair is propagated
// into the scheduler's job lineage table.
type Scheduler interface {
	Schedule(ctx context.Context, when time.Time, payload string, source string, originator string) (jobID string, err error)
}

// Services holds the runtime dependencies for both notification tools.
type Services struct {
	Confirm        ConfirmStore
	Scheduler      Scheduler
	ConfirmTimeout time.Duration
}

var (
	servicesMu sync.RWMutex
	services   *Services
)

// SetServices wires the production notification runtime. Pass nil to
// clear (test-only).
func SetServices(s *Services) {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = s
}

// ResetForTest clears the wired services. Test-only.
func ResetForTest() {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = nil
}

func loadServices() (*Services, error) {
	servicesMu.RLock()
	defer servicesMu.RUnlock()
	if services == nil {
		return nil, errors.New("notification_tools_not_configured")
	}
	if services.Confirm == nil {
		return nil, errors.New("notification_tools_confirm_store_not_configured")
	}
	if services.Scheduler == nil {
		return nil, errors.New("notification_tools_scheduler_not_configured")
	}
	if services.ConfirmTimeout <= 0 {
		return nil, fmt.Errorf("notification_tools_confirm_timeout_invalid: %s", services.ConfirmTimeout)
	}
	return services, nil
}

// nowFn is a test hook so tests can inject a deterministic clock for
// the {when} normalization performed by propose. Production code uses
// time.Now.
var nowFn = func() time.Time { return time.Now().UTC() }

// newRefFn is a test hook so tests can pin the ULID-shaped confirm
// reference returned by propose. Production code uses a fresh
// 128-bit random hex string.
var newRefFn = func() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read on Linux is /dev/urandom — failure here means the
		// kernel CSPRNG is broken, which is a fatal condition for the
		// process. Panic instead of returning a predictable ref.
		panic(fmt.Sprintf("notification: crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// payloadEnvelope is the opaque payload stored under a confirm_ref.
// notification_execute round-trips it from the ConfirmStore to the
// Scheduler unchanged so the schema travels with the data and the
// scheduler never sees raw LLM text.
type payloadEnvelope struct {
	What      string    `json:"what"`
	WhenUTC   time.Time `json:"when_utc"`
	UserID    string    `json:"user_id"`
	Transport string    `json:"transport,omitempty"`
}

// resolveWhen converts a (when_iso, when_relative) pair into a single
// UTC time. when_iso wins when both are present. when_relative
// understands a tiny vocabulary suitable for v1: "Nm", "Nh", "Nd"
// (minutes, hours, days from now). Unknown forms produce an error.
func resolveWhen(whenISO, whenRelative string, now time.Time) (time.Time, error) {
	s := strings.TrimSpace(whenISO)
	if s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid when_iso %q: %w", whenISO, err)
		}
		return t.UTC(), nil
	}
	r := strings.TrimSpace(whenRelative)
	if r == "" {
		return time.Time{}, errors.New("missing when")
	}
	// "2h", "30m", "1d" — single integer + unit
	if len(r) < 2 {
		return time.Time{}, fmt.Errorf("invalid when_relative %q", whenRelative)
	}
	unit := r[len(r)-1]
	numStr := r[:len(r)-1]
	var n int
	if _, err := fmt.Sscanf(numStr, "%d", &n); err != nil || n <= 0 {
		return time.Time{}, fmt.Errorf("invalid when_relative %q", whenRelative)
	}
	switch unit {
	case 'm':
		return now.Add(time.Duration(n) * time.Minute), nil
	case 'h':
		return now.Add(time.Duration(n) * time.Hour), nil
	case 'd':
		return now.Add(time.Duration(n) * 24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("invalid when_relative unit %q", string(unit))
	}
}
