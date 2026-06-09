// Package webcreds implements the spec 070 web operator credential
// layer: argon2id password hashing + a Postgres-backed repo for the
// web_user_credentials table.
//
// This package is NOT used for machine clients / Telegram / PASETO
// bearer auth. Those continue to use internal/auth/{bearer,oauth,
// scope_middleware}. Web credentials are a UX layer for the human
// operator surface only; on successful verification the cookie value
// is still the existing shared AuthToken.
package webcreds

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ErrInvalidCredentials is returned by Repo.VerifyAndTouch and by
// Verify when the supplied password does not match the stored hash,
// when the user does not exist, or when the PHC string is malformed.
// All three cases collapse into one error to prevent user-enumeration
// via response shape.
var ErrInvalidCredentials = errors.New("webcreds: invalid credentials")

// Parameters for argon2id. These are baked at compile-time, not
// configurable per-call; rotating costs requires bumping these and
// re-hashing existing rows (acceptable for the operator-only scale
// this surface targets).
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024 // 64 MB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	saltLen      int    = 16
)

// MinPasswordLength is the minimum length accepted by Hash and by
// the CLI bootstrap. argon2id itself handles arbitrary lengths; this
// floor exists so the operator-facing CLI can refuse trivially-weak
// inputs before they ever reach the DB.
const MinPasswordLength = 12

// Hash returns an argon2id PHC string for the supplied password.
// Returns an error when password is shorter than MinPasswordLength
// or when the underlying CSPRNG read fails.
func Hash(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", fmt.Errorf("webcreds: password must be at least %d characters", MinPasswordLength)
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("webcreds: read salt: %w", err)
	}
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		argonKeyLen,
	)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// Verify checks that the supplied password produces the same hash
// as the encoded PHC string. Returns nil on match, ErrInvalidCredentials
// on mismatch or malformed input. Constant-time compare on the hash
// suffix prevents timing leaks across mismatches.
func Verify(phc, password string) error {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return ErrInvalidCredentials
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return ErrInvalidCredentials
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return ErrInvalidCredentials
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return ErrInvalidCredentials
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return ErrInvalidCredentials
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

// dummyHash is computed once at package init from a fixed sentinel
// password. It is used by Repo.VerifyAndTouch for unknown-user
// requests so the wall-clock cost matches a known-user-wrong-password
// path, preventing timing-based user enumeration.
//
// The sentinel value is intentionally not derived from any secret —
// the only goal is to make Verify perform a real argon2id evaluation
// for unknown users. Verify against this hash with the user-supplied
// password is GUARANTEED to fail (the user cannot know the sentinel),
// so leaking the sentinel reveals nothing.
var dummyHash string

func init() {
	h, err := Hash("smackerel-webcreds-timing-parity-sentinel-do-not-use")
	if err != nil {
		// Should be impossible: sentinel is well-formed and long enough.
		panic(fmt.Sprintf("webcreds: dummy hash init failed: %v", err))
	}
	dummyHash = h
}

// DummyHash returns the package-level dummy hash for timing parity
// during unknown-user verification. Exposed for tests.
func DummyHash() string { return dummyHash }
