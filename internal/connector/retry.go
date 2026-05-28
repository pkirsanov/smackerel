// BUG-022-003 — Uniform 429/Retry-After handling across HTTP connectors.
//
// DoWithRetry is the shared HTTP execution helper that honors RFC 7231 §7.1.3
// Retry-After (both delta-seconds and HTTP-date forms) with a bounded retry
// budget. It replaces the per-connector ad-hoc "non-200 → error" pattern that
// caused brownouts to escalate into hard provider bans (see bug.md).
package connector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
)

// ErrRateLimitExhausted is returned by DoWithRetry when MaxAttempts is reached
// while the server is still answering with 429.
var ErrRateLimitExhausted = errors.New("rate limited: max retries exceeded")

// RetryOptions configures DoWithRetry. Zero values are NOT defaulted; callers
// should start from DefaultRetryOptions() and override fields as needed.
type RetryOptions struct {
	// MaxAttempts is the total number of HTTP attempts (including the first).
	// Must be >= 1; a value < 1 is treated as 1 (single attempt, no retry).
	MaxAttempts int
	// BaseDelay is the starting backoff delay used when Retry-After is absent.
	BaseDelay time.Duration
	// MaxDelay caps any individual sleep (Retry-After OR backoff-derived).
	MaxDelay time.Duration
	// Label is the connector identifier emitted as the `connector` metric label.
	Label string
}

// DefaultRetryOptions returns the production defaults: 5 attempts total,
// 1s base, 16s cap. Callers MUST set Label.
func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Second,
		MaxDelay:    16 * time.Second,
	}
}

// parseRetryAfter parses an HTTP Retry-After header value in either
// delta-seconds (RFC 7231 §7.1.3) or HTTP-date form. Returns (0, false) for
// empty or unparseable inputs. Past HTTP-dates return (0, true) so the
// caller treats them as "retry immediately".
func parseRetryAfter(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			return 0, true
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, true
		}
		return d, true
	}
	return 0, false
}

// DoWithRetry executes req through client, honoring 429 + Retry-After with a
// bounded retry budget defined by opts. On non-429 responses (including 5xx)
// the response is returned as-is; per-connector callers retain their existing
// error semantics. On retry exhaustion the final 429 body is drained and
// ErrRateLimitExhausted is returned.
//
// Concurrency: DoWithRetry does not mutate req. Callers that need to retry
// a request with a body MUST ensure req.GetBody is set (the request body is
// re-read via req.GetBody on every retry; if GetBody is nil, the request is
// re-sent with a nil body which is correct for GET/HEAD).
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, opts RetryOptions) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("DoWithRetry: nil client")
	}
	if req == nil {
		return nil, fmt.Errorf("DoWithRetry: nil request")
	}
	attempts := opts.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	bo := &Backoff{
		BaseDelay:  opts.BaseDelay,
		MaxDelay:   opts.MaxDelay,
		MaxRetries: attempts, // Next() will return ok for the first `attempts` calls
	}

	var lastResp *http.Response
	for attempt := 0; attempt < attempts; attempt++ {
		// Rebuild request body on retry if possible.
		curReq := req
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("DoWithRetry: GetBody: %w", err)
			}
			cloned := req.Clone(req.Context())
			cloned.Body = body
			curReq = cloned
		}

		resp, err := client.Do(curReq.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			// Success path: if we previously saw a 429 and now succeeded, count "recovered".
			if attempt > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				metrics.ConnectorRateLimit429Total.WithLabelValues(opts.Label, "recovered").Inc()
			}
			return resp, nil
		}

		// 429 path — decide whether to retry.
		retryAfterHdr := resp.Header.Get("Retry-After")
		// Drain + close body so connection can be reused.
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		lastResp = resp

		if attempt == attempts-1 {
			metrics.ConnectorRateLimit429Total.WithLabelValues(opts.Label, "exhausted").Inc()
			return nil, ErrRateLimitExhausted
		}

		// Determine sleep duration.
		var delay time.Duration
		if d, ok := parseRetryAfter(retryAfterHdr); ok {
			delay = d
		} else {
			d, _ := bo.Next()
			delay = d
		}
		if opts.MaxDelay > 0 && delay > opts.MaxDelay {
			delay = opts.MaxDelay
		}

		metrics.ConnectorRateLimit429Total.WithLabelValues(opts.Label, "retry").Inc()

		if delay <= 0 {
			// Yield to context cancellation but do not block.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	// Defensive: should be unreachable because the loop returns at attempt == attempts-1.
	_ = lastResp
	return nil, ErrRateLimitExhausted
}
