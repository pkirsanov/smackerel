// Spec 071 SCOPE-02 — Deterministic sampling.
//
// Sampler.ShouldSample is a pure function of a stable per-turn id
// (trace_id) and the SST-resolved ratio. The same trace_id always
// produces the same decision so retries and replays remain consistent.
// Ratio 1.0 forces sampled; ratio 0.0 forces sampled-out; intermediate
// values bucket on sha256(trace_id) modulo 1<<32 / 2^32.

package intenttrace

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// Sampler returns the deterministic sampling decision for one turn.
type Sampler interface {
	ShouldSample(traceID string) bool
}

// RatioSampler is the deterministic sampler.
type RatioSampler struct {
	ratio float64
}

// NewRatioSampler validates and returns a RatioSampler. Ratio must be
// in the closed interval [0,1]; anything else fails loud per the
// smackerel-no-defaults policy.
func NewRatioSampler(ratio float64) (*RatioSampler, error) {
	if ratio < 0 || ratio > 1 {
		return nil, fmt.Errorf("intenttrace: sampling ratio must be in [0,1], got %v", ratio)
	}
	return &RatioSampler{ratio: ratio}, nil
}

// Ratio returns the configured ratio.
func (s *RatioSampler) Ratio() float64 { return s.ratio }

// ShouldSample implements Sampler.
func (s *RatioSampler) ShouldSample(traceID string) bool {
	if s == nil {
		return false
	}
	if s.ratio >= 1.0 {
		return true
	}
	if s.ratio <= 0.0 {
		return false
	}
	if traceID == "" {
		return false
	}
	sum := sha256.Sum256([]byte(traceID))
	bucket := binary.BigEndian.Uint32(sum[:4])
	threshold := uint32(s.ratio * float64(^uint32(0)))
	return bucket < threshold
}

// ErrSamplerNotConfigured is returned when callers ask for a decision
// without a configured sampler.
var ErrSamplerNotConfigured = errors.New("intenttrace: sampler not configured")
