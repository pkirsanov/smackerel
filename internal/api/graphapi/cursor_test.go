package graphapi

import (
	"errors"
	"strings"
	"testing"
)

// TestEncodeDecodeCursor_Roundtrip — SCN-080-11 regression: a cursor
// produced by Encode must decode back to the same payload.
func TestEncodeDecodeCursor_Roundtrip(t *testing.T) {
	codec := mustCodec(t, "test-secret-roundtrip-32-bytes!!")
	in := CursorPayload{
		Resource:    "topics",
		LastSortKey: "2026-06-03T12:34:56Z",
		LastID:      "topic-42",
		Offset:      100,
		Checksum:    "abc123",
	}
	enc, err := codec.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(enc, cursorVersion+".") {
		t.Errorf("encoded cursor missing %q prefix: %q", cursorVersion+".", enc)
	}
	out, err := codec.Decode(enc)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch:\n  want %+v\n  got  %+v", in, out)
	}
}

// TestDecodeCursor_RejectsGarbage — SCN-080-11: malformed cursors of
// every shape must return ErrMalformedCursor. Each case is the
// adversarial form a real client (or attacker) might produce.
func TestDecodeCursor_RejectsGarbage(t *testing.T) {
	codec := mustCodec(t, "test-secret-garbage-cases-32!!")
	cases := map[string]string{
		"empty":               "",
		"plain garbage":       "not-a-real-cursor",
		"wrong segment count": "v1.onlytwosegments",
		"too many segments":   "v1.a.b.c.d",
		"unknown version":     "v9.aaaa.bbbb",
		"non-base64 payload":  "v1.@@@@.bbbb",
		"non-base64 mac":      "v1.aaaa.@@@@",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := codec.Decode(in)
			if !errors.Is(err, ErrMalformedCursor) {
				t.Errorf("Decode(%q) err = %v; want ErrMalformedCursor", in, err)
			}
		})
	}
}

// TestDecodeCursor_RejectsTamper — adversarial: flip one byte of the
// HMAC and verify Decode rejects via hmac.Equal constant-time compare.
func TestDecodeCursor_RejectsTamper(t *testing.T) {
	codec := mustCodec(t, "tamper-secret-bytes-32-chars-!!aa")
	enc, err := codec.Encode(CursorPayload{Resource: "people", LastID: "p1"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parts := strings.Split(enc, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(parts))
	}
	// Flip the FIRST byte of the base64url MAC segment. Flipping a
	// trailing byte can land entirely in base64 padding bits and
	// silently round-trip to the same decoded bytes; flipping a
	// leading byte guarantees the decoded MAC differs.
	mac := []byte(parts[2])
	mac[0] ^= 0x01
	// Ensure the flipped byte stays in the base64url alphabet so the
	// failure is "HMAC mismatch", not "base64 decode error".
	if mac[0] < 'A' || (mac[0] > 'Z' && mac[0] < 'a') || mac[0] > 'z' {
		mac[0] = 'A'
	}
	parts[2] = string(mac)
	tampered := strings.Join(parts, ".")
	if tampered == enc {
		t.Fatal("tamper produced identical cursor; test is degenerate")
	}
	if _, err := codec.Decode(tampered); !errors.Is(err, ErrMalformedCursor) {
		t.Errorf("Decode(tampered) err = %v; want ErrMalformedCursor", err)
	}
}

// TestDecodeCursor_RejectsCrossKeyForgery — a cursor signed with key A
// must not verify under key B. Guards against the "swap signing key,
// forget to invalidate old cursors" misconfiguration mode.
func TestDecodeCursor_RejectsCrossKeyForgery(t *testing.T) {
	codecA := mustCodec(t, "secret-A-secret-A-32-bytes!!aaaa")
	codecB := mustCodec(t, "secret-B-secret-B-32-bytes!!bbbb")
	enc, err := codecA.Encode(CursorPayload{Resource: "places", LastID: "pl1"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := codecB.Decode(enc); !errors.Is(err, ErrMalformedCursor) {
		t.Errorf("Decode under wrong key err = %v; want ErrMalformedCursor", err)
	}
}

// TestNewCursorCodec_RejectsEmptySecret — fail-loud guard: a codec
// constructed with an empty secret is unusable; the SST loader must
// catch this upstream, but the constructor double-checks.
func TestNewCursorCodec_RejectsEmptySecret(t *testing.T) {
	if _, err := NewCursorCodec(nil); err == nil {
		t.Error("NewCursorCodec(nil) returned no error; want fail-loud")
	}
	if _, err := NewCursorCodec([]byte{}); err == nil {
		t.Error("NewCursorCodec([]byte{}) returned no error; want fail-loud")
	}
}

func mustCodec(t *testing.T, secret string) *CursorCodec {
	t.Helper()
	c, err := NewCursorCodec([]byte(secret))
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return c
}
