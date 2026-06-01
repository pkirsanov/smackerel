// privacy_test.go — spec 075 SCOPE-1 TP-075-03 / SCN-075-A11.
//
// Asserts the privacy invariants the rest of the system depends on:
//
//  1. The HMAC user-bucket helper rejects an empty key (the SST
//     validator catches this at startup; the constructor is the
//     defense-in-depth).
//  2. UserBucket() returns a stable HMAC-SHA256 hex digest, never
//     the raw user id, and is collision-resistant for distinct
//     ids (two distinct ids produce distinct buckets).
//  3. The residual telemetry counter is registered with the closed
//     label set {command, user_bucket} — and specifically does NOT
//     carry a raw-user-id-shaped label (e.g. "user_id", "user",
//     "telegram_chat_id"). A regression that adds such a label
//     would trip this assertion BEFORE Scope 3 wires the metric.
//  4. Metric collection on a populated ResidualUsageCounter never
//     surfaces the raw user id as a label value: the only path the
//     Scope 3 wiring is permitted to take goes through
//     UserBucketHasher.UserBucket. We prove this by registering a
//     sample using the bucket and asserting Prometheus' label
//     enumeration only returns the {command, user_bucket} keys.
package legacyretirement

import (
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewUserBucketHasher_RejectsEmptyKey(t *testing.T) {
	if _, err := NewUserBucketHasher(""); err == nil {
		t.Fatal("NewUserBucketHasher must reject empty key; got nil error")
	}
	if _, err := NewUserBucketHasher("   "); err == nil {
		t.Fatal("NewUserBucketHasher must reject whitespace-only key; got nil error")
	}
	if _, err := NewUserBucketHasher("real-key"); err != nil {
		t.Fatalf("NewUserBucketHasher must accept a non-empty key; got %v", err)
	}
}

func TestUserBucket_HMACDigestNotRawID(t *testing.T) {
	h, err := NewUserBucketHasher("spec-075-privacy-test-key")
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	const rawID = "telegram-chat-123456789"
	bucket := h.UserBucket(rawID)

	if bucket == rawID {
		t.Fatalf("UserBucket(%q) returned the raw id; privacy invariant violated", rawID)
	}
	if strings.Contains(bucket, rawID) {
		t.Fatalf("UserBucket output %q contains the raw id substring %q; privacy invariant violated", bucket, rawID)
	}
	// HMAC-SHA256 hex digest is always 64 lowercase hex chars.
	if matched, _ := regexp.MatchString(`^[0-9a-f]{64}$`, bucket); !matched {
		t.Fatalf("UserBucket output %q must be 64-char lowercase hex (HMAC-SHA256)", bucket)
	}

	// Stability — same id, same key → same digest.
	if h.UserBucket(rawID) != bucket {
		t.Fatal("UserBucket must be deterministic for a given (key, userID)")
	}

	// Distinct ids → distinct buckets (collision resistance sanity).
	if h.UserBucket("telegram-chat-987654321") == bucket {
		t.Fatal("distinct user ids must produce distinct buckets")
	}
}

func TestUserBucket_EmptyUserIDReturnsEmpty(t *testing.T) {
	h, err := NewUserBucketHasher("spec-075-privacy-test-key")
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	if got := h.UserBucket(""); got != "" {
		t.Errorf("UserBucket(\"\") must return empty string, got %q", got)
	}
}

// TestResidualUsageCounter_LabelSetRejectsRawIdShapedLabels asserts
// the metric's label set is exactly {command, user_bucket}. A
// regression that adds "user_id" / "user" / "telegram_chat_id" /
// "raw_id" as a label here would trip the test BEFORE Scope 3
// starts emitting samples — the SCN-075-A11 privacy invariant says
// "no raw user identifiers in telemetry", and label NAMES are the
// first thing an operator sees on the /metrics endpoint.
func TestResidualUsageCounter_LabelSetRejectsRawIdShapedLabels(t *testing.T) {
	// Drive a single observation through the public Record path so
	// the label set is materialised. We use the package's HMAC
	// helper to compute the bucket — never the raw id.
	h, err := NewUserBucketHasher("spec-075-privacy-test-key")
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	const rawID = "raw-user-must-not-appear-in-metric-label-1234"
	bucket := h.UserBucket(rawID)
	ResidualUsageCounter.WithLabelValues("/weather", bucket).Inc()

	// Enumerate every series the counter exposes and verify the
	// label keys are exactly {command, user_bucket}, and no label
	// VALUE equals the raw user id.
	ch := make(chan prometheus.Metric, 16)
	ResidualUsageCounter.Collect(ch)
	close(ch)

	forbiddenLabelNames := map[string]bool{
		"user_id":          true,
		"user":             true,
		"telegram_chat_id": true,
		"raw_id":           true,
		"chat_id":          true,
		"username":         true,
	}
	requiredLabelNames := map[string]bool{
		LabelCommand:    false,
		LabelUserBucket: false,
	}

	sawAny := false
	for m := range ch {
		sawAny = true
		var dtoMetric dto.Metric
		if err := m.Write(&dtoMetric); err != nil {
			t.Fatalf("metric.Write: %v", err)
		}
		for _, lp := range dtoMetric.GetLabel() {
			name := lp.GetName()
			val := lp.GetValue()
			if forbiddenLabelNames[name] {
				t.Errorf("ResidualUsageCounter exposes forbidden raw-id-shaped label %q (value=%q); privacy invariant violated", name, val)
			}
			if _, ok := requiredLabelNames[name]; ok {
				requiredLabelNames[name] = true
			} else if name != LabelCommand && name != LabelUserBucket {
				t.Errorf("ResidualUsageCounter exposes unexpected label %q (value=%q); label set must be exactly {%q, %q}", name, val, LabelCommand, LabelUserBucket)
			}
			if val == rawID {
				t.Errorf("ResidualUsageCounter label %q carries the raw user id %q as a value; privacy invariant violated", name, val)
			}
			if strings.Contains(val, rawID) {
				t.Errorf("ResidualUsageCounter label %q value %q contains the raw user id %q substring; privacy invariant violated", name, val, rawID)
			}
		}
	}
	if !sawAny {
		t.Fatal("ResidualUsageCounter.Collect produced zero series; the Inc above should have materialised at least one")
	}
	for name, seen := range requiredLabelNames {
		if !seen {
			t.Errorf("ResidualUsageCounter is missing required label %q", name)
		}
	}
}
