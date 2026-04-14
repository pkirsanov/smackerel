package connector

import (
	"math"
	"math/rand"
	"time"
)

// Backoff implements exponential backoff with jitter for rate-limit handling.
type Backoff struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
	attempt    int
}

// DefaultBackoff returns a backoff policy suitable for API rate limits.
func DefaultBackoff() *Backoff {
	return &Backoff{
		BaseDelay:  1 * time.Second,
		MaxDelay:   16 * time.Second,
		MaxRetries: 5,
	}
}

// Next returns the next delay duration and whether retries remain.
func (b *Backoff) Next() (time.Duration, bool) {
	if b.attempt >= b.MaxRetries {
		return 0, false
	}

	// Exponential: base * 2^attempt, clamped to MaxDelay before int64 conversion
	// to prevent overflow (math.Pow can exceed int64 range for large attempts).
	exp := math.Pow(2, float64(b.attempt))
	delayFloat := float64(b.BaseDelay) * exp
	maxFloat := float64(b.MaxDelay)
	if delayFloat > maxFloat || math.IsInf(delayFloat, 0) || math.IsNaN(delayFloat) || delayFloat < 0 {
		delayFloat = maxFloat
	}

	delay := time.Duration(delayFloat)
	if delay > b.MaxDelay {
		delay = b.MaxDelay
	}
	if delay <= 0 {
		delay = b.BaseDelay
	}

	// Add jitter: ±25% (guard against tiny delays where int division yields 0)
	if half := int64(delay) / 2; half > 0 {
		jitter := time.Duration(rand.Int63n(half)) - time.Duration(int64(delay)/4)
		delay += jitter
	}

	b.attempt++
	return delay, true
}

// Reset resets the backoff counter.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt number.
func (b *Backoff) Attempt() int {
	return b.attempt
}
