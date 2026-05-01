package save

import (
	"bytes"
	"io"
)

// newBytesReadCloser wraps a byte slice in an io.ReadCloser so the save
// service can pass artifact bytes to drive.Provider.PutFile without leaking
// readers. Close is a no-op.
func newBytesReadCloser(data []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(data))
}

// nullableUUID returns nil for empty strings so pgx writes SQL NULL into
// nullable UUID columns rather than failing on an empty-string cast.
func nullableUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
