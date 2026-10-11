package utils

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

// maxRetryAfter caps the Retry-After header value so a single response
// cannot stall the client for an unreasonable amount of time.
const maxRetryAfter = 30 * time.Second

// RetryTransport wraps an http.RoundTripper and retries requests that
// receive transient server errors (HTTP 5xx) or rate-limit responses
// (HTTP 429). It uses exponential backoff between retries, and respects
// the Retry-After header on 429 responses when present.
type RetryTransport struct {
	Base       http.RoundTripper
	MaxRetries int
	BaseDelay  time.Duration
}

// isRetryable returns true for status codes that should be retried.
func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		// Clone the request body for retries (body is consumed on each attempt)
		if req.Body != nil && req.GetBody != nil {
			bodyClone, _ := req.GetBody()
			req.Body = bodyClone
		}

		resp, err = t.Base.RoundTrip(req)
		if err != nil {
			return resp, err
		}

		if !isRetryable(resp.StatusCode) || attempt == t.MaxRetries {
			return resp, nil
		}

		// Retryable status -- drain body and retry
		delay := t.retryDelay(resp, attempt)

		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		fmt.Printf("[http-retry] %s %s: attempt %d/%d got HTTP %d, retrying in %s\n",
			req.Method, req.URL.Path, attempt+1, t.MaxRetries, resp.StatusCode, delay)
		time.Sleep(delay)
	}

	return resp, err
}

// retryDelay determines the backoff duration for the given attempt.
// For 429 responses with a Retry-After header (in seconds), it uses
// that value (capped at maxRetryAfter). Otherwise it falls back to
// exponential backoff: BaseDelay * 2^attempt.
func (t *RetryTransport) retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil && seconds > 0 {
				d := time.Duration(seconds) * time.Second
				d = min(d, maxRetryAfter)
				return d
			}
		}
	}
	return t.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
}

// NewRetryTransport creates a RetryTransport that wraps the given base transport
// with 5 retries and 2s base delay (exponential: 2s, 4s, 8s, 16s, 32s).
func NewRetryTransport(base http.RoundTripper) *RetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &RetryTransport{
		Base:       base,
		MaxRetries: 5,
		BaseDelay:  2 * time.Second,
	}
}
