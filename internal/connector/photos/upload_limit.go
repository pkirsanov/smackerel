package photos

import "io"

// LimitedUploadReader returns an io.Reader that caps the number of
// bytes readable from src to maxBytes. When maxBytes <= 0 the source
// reader is returned unchanged so test paths that do not configure
// the SST cap retain the historical unbounded behavior.
//
// Production wiring is responsible for sourcing maxBytes from
// PhotosConfig.IOLimits.PhotoBinaryMaxBytes via each adapter's
// SetUploadMaxBytes hook. The cap is the MIT-040-S-006 defense in
// depth against an attacker-controlled io.Reader returning unbounded
// data after the API-edge MaxBytesReader has already been consumed.
func LimitedUploadReader(src io.Reader, maxBytes int64) io.Reader {
	if maxBytes <= 0 {
		return src
	}
	return io.LimitReader(src, maxBytes)
}
