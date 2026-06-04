package graphapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// cursorVersion is the current wire-format version. Encoded into the
// cursor as the leading segment so future format changes (e.g. a new
// payload field, a different MAC algorithm) can be migrated without
// silently accepting old or forged tokens.
const cursorVersion = "v1"

// CursorPayload is the public, opaque-to-clients payload carried in
// every cursor. Server-only fields; clients never construct one.
//
// Per design.md §5 the payload is {resource, lastSortKey, lastID};
// Offset and Checksum are the spec 080 implementation choice for an
// HMAC-signed, drift-resistant cursor (the user task brief in
// SCOPE-080-01 names "HMAC-signed JSON with offset + checksum").
type CursorPayload struct {
	Resource    string `json:"r"`
	LastSortKey string `json:"k"`
	LastID      string `json:"i"`
	Offset      int64  `json:"o"`
	Checksum    string `json:"c,omitempty"`
}

// CursorCodec encodes and decodes opaque pagination cursors using an
// HMAC-SHA256 signature over the payload. The signing key is the raw
// bytes of the env var named by knowledge_graph_api.cursor_secret_env
// (SST, fail-loud). Construct one via NewCursorCodec; the zero value
// is not usable.
type CursorCodec struct {
	secret []byte
}

// NewCursorCodec returns a codec keyed by secret. An empty secret is
// a programming error and is rejected fail-loud — the caller should
// have already routed an SST-missing error from LoadConfig.
func NewCursorCodec(secret []byte) (*CursorCodec, error) {
	if len(secret) == 0 {
		return nil, errors.New("graphapi: cursor secret is empty (knowledge_graph_api.cursor_secret_env env var must be non-empty)")
	}
	return &CursorCodec{secret: append([]byte(nil), secret...)}, nil
}

// Encode produces the opaque cursor string of the form
//
//	v1.<base64url(payloadJSON)>.<base64url(hmac-sha256(payloadJSON))>
//
// The dot separator is safe because base64url uses only A-Za-z0-9_-
// (no '.'). Round-trips through Decode without information loss.
func (c *CursorCodec) Encode(p CursorPayload) (string, error) {
	if c == nil || len(c.secret) == 0 {
		return "", errors.New("graphapi: cursor codec is not initialized")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("graphapi: marshal cursor payload: %w", err)
	}
	mac := hmacSum(c.secret, raw)
	return strings.Join([]string{
		cursorVersion,
		base64.RawURLEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(mac),
	}, "."), nil
}

// Decode parses an opaque cursor produced by Encode. Any of the
// following fail with ErrMalformedCursor: empty input, wrong number
// of segments, unknown version, non-base64url segments, malformed
// JSON, or HMAC mismatch (tamper). The error is the typed
// ErrMalformedCursor singleton (or a clone via WithField) so handlers
// can route it directly through WriteAPIError.
func (c *CursorCodec) Decode(s string) (CursorPayload, error) {
	if c == nil || len(c.secret) == 0 {
		return CursorPayload{}, errors.New("graphapi: cursor codec is not initialized")
	}
	if s == "" {
		return CursorPayload{}, ErrMalformedCursor
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return CursorPayload{}, ErrMalformedCursor
	}
	if parts[0] != cursorVersion {
		return CursorPayload{}, ErrMalformedCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return CursorPayload{}, ErrMalformedCursor
	}
	mac, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return CursorPayload{}, ErrMalformedCursor
	}
	expected := hmacSum(c.secret, raw)
	if !hmac.Equal(mac, expected) {
		return CursorPayload{}, ErrMalformedCursor
	}
	var p CursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return CursorPayload{}, ErrMalformedCursor
	}
	return p, nil
}

func hmacSum(secret, payload []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write(payload)
	return m.Sum(nil)
}
