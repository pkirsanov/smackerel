package nats

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	natsio "github.com/nats-io/nats.go"
)

// TestRequest_NilClient verifies Request fails fast when the receiver is nil.
func TestRequest_NilClient(t *testing.T) {
	var c *Client
	_, err := c.Request(context.Background(), "any.subject", []byte("x"), time.Second)
	if err == nil {
		t.Fatal("expected error from nil client")
	}
	if !strings.Contains(err.Error(), "client is nil") {
		t.Errorf("expected 'client is nil' error, got: %v", err)
	}
}

// TestRequest_NilConn verifies Request fails fast when the underlying conn
// is nil (defensive guard in addition to the nil-receiver case).
func TestRequest_NilConn(t *testing.T) {
	c := &Client{Conn: nil}
	_, err := c.Request(context.Background(), "any.subject", []byte("x"), time.Second)
	if err == nil {
		t.Fatal("expected error from nil conn")
	}
	if !strings.Contains(err.Error(), "client is nil") {
		t.Errorf("expected 'client is nil' error, got: %v", err)
	}
}

// TestRequest_ZeroTimeoutRejected enforces the SST policy: callers MUST pass
// an explicit positive timeout. There is no hidden default.
func TestRequest_ZeroTimeoutRejected(t *testing.T) {
	c := &Client{Conn: &natsio.Conn{}}
	_, err := c.Request(context.Background(), "any.subject", []byte("x"), 0)
	if err == nil {
		t.Fatal("expected error for zero timeout")
	}
	if !strings.Contains(err.Error(), "timeout must be > 0") {
		t.Errorf("expected timeout-must-be-positive error, got: %v", err)
	}
}

// TestRequest_NegativeTimeoutRejected mirrors the zero-timeout rule.
func TestRequest_NegativeTimeoutRejected(t *testing.T) {
	c := &Client{Conn: &natsio.Conn{}}
	_, err := c.Request(context.Background(), "any.subject", []byte("x"), -1*time.Second)
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
	if !strings.Contains(err.Error(), "timeout must be > 0") {
		t.Errorf("expected timeout-must-be-positive error, got: %v", err)
	}
}

// natsTestURL returns a usable NATS URL or skips the test. Real-server
// integration tests run only when SMACKEREL_NATS_TEST_URL is exported (CI or
// the smackerel.sh integration harness sets it). Following the existing
// pattern in this package: connection failure paths are unit-testable; the
// happy-path round-trip needs a live broker.
func natsTestURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("SMACKEREL_NATS_TEST_URL")
	if url == "" {
		t.Skip("SMACKEREL_NATS_TEST_URL not set; skipping live NATS request/reply test")
	}
	return url
}

// TestRequest_HappyPath verifies a request gets the responder's reply.
// Requires a live NATS broker; skipped otherwise.
func TestRequest_HappyPath(t *testing.T) {
	url := natsTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	subj := "smk.test.req." + randSuffix()
	sub, err := c.Conn.Subscribe(subj, func(m *natsio.Msg) {
		_ = m.Respond(append([]byte("echo:"), m.Data...))
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	reply, err := c.Request(ctx, subj, []byte("ping"), 2*time.Second)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if string(reply) != "echo:ping" {
		t.Errorf("unexpected reply: %q", reply)
	}
}

// TestRequest_TimeoutNoResponder verifies Request returns an error when no
// responder is subscribed. Requires a live broker.
func TestRequest_TimeoutNoResponder(t *testing.T) {
	url := natsTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	subj := "smk.test.noresp." + randSuffix()
	_, err = c.Request(ctx, subj, []byte("ping"), 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when no responder is subscribed")
	}
	// Either NoResponders (server-issued) or a context deadline error is
	// acceptable; both indicate the caller should fall back.
	if !errors.Is(err, natsio.ErrNoResponders) &&
		!strings.Contains(err.Error(), "deadline") &&
		!strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected no-responders or timeout error, got: %v", err)
	}
}

// randSuffix returns a short pseudo-random subject suffix to avoid collisions
// when tests run in parallel against a shared broker.
func randSuffix() string {
	return time.Now().Format("150405.000000000")
}
