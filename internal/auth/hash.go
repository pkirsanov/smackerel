package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
)

// HashToken returns the HMAC-SHA-256 of token using key, hex-encoded.
// Spec 044 OQ-8 — bearer tokens are stored at rest as their HMAC under a
// separate key (auth.at_rest_hashing_key) so that database compromise
// alone does not yield usable tokens. Returns an error when key is empty
// to refuse persistence with a missing secret rather than silently
// hashing under a zero key.
func HashToken(token, key string) (string, error) {
	if key == "" {
		return "", errors.New("auth: cannot hash token with empty hashing key")
	}
	if token == "" {
		return "", errors.New("auth: cannot hash empty token")
	}
	mac := hmac.New(sha256.New, []byte(key))
	if _, err := mac.Write([]byte(token)); err != nil {
		return "", fmt.Errorf("auth: hmac write: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// CompareTokenHash returns true when the supplied token hashes to
// expectedHexHash under the supplied key. Comparison is constant-time
// (subtle.ConstantTimeCompare) to refuse the timing oracle that a naive
// string-equality check would create.
func CompareTokenHash(token, key, expectedHexHash string) (bool, error) {
	got, err := HashToken(token, key)
	if err != nil {
		return false, err
	}
	if len(got) != len(expectedHexHash) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expectedHexHash)) == 1, nil
}
