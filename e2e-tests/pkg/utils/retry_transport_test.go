package utils

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryTransport_Retries429(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewRetryTransport(http.DefaultTransport)
	transport.BaseDelay = 10 * time.Millisecond // fast for tests
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestRetryTransport_Retries5xx(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewRetryTransport(http.DefaultTransport)
	transport.BaseDelay = 10 * time.Millisecond
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestRetryTransport_DoesNotRetry4xx(t *testing.T) {
	codes := []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
	}

	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(code)
			}))
			defer server.Close()

			transport := NewRetryTransport(http.DefaultTransport)
			transport.BaseDelay = 10 * time.Millisecond
			client := &http.Client{Transport: transport}

			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != code {
				t.Fatalf("expected %d, got %d", code, resp.StatusCode)
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("expected 1 attempt (no retry), got %d", got)
			}
		})
	}
}

func TestRetryTransport_RespectsRetryAfterHeader(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewRetryTransport(http.DefaultTransport)
	transport.BaseDelay = 10 * time.Millisecond
	client := &http.Client{Transport: transport}

	start := time.Now()
	resp, err := client.Get(server.URL)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// Should have waited ~1 second per the Retry-After header, not 10ms
	if elapsed < 900*time.Millisecond {
		t.Fatalf("expected at least ~1s delay from Retry-After, got %s", elapsed)
	}
}

func TestRetryTransport_CapsRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"120"}},
	}
	transport := NewRetryTransport(http.DefaultTransport)

	delay := transport.retryDelay(resp, 0)
	if delay != maxRetryAfter {
		t.Fatalf("expected capped delay of %s, got %s", maxRetryAfter, delay)
	}
}

func TestRetryTransport_ExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	transport := NewRetryTransport(http.DefaultTransport)
	transport.MaxRetries = 2
	transport.BaseDelay = 10 * time.Millisecond
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	// 1 initial + 2 retries = 3 total attempts
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestRetryTransport_FallsBackToExponentialWithout429RetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
	}
	transport := &RetryTransport{
		BaseDelay: 100 * time.Millisecond,
	}

	// attempt 0: 100ms * 2^0 = 100ms
	delay := transport.retryDelay(resp, 0)
	if delay != 100*time.Millisecond {
		t.Fatalf("expected 100ms, got %s", delay)
	}
	// attempt 2: 100ms * 2^2 = 400ms
	delay = transport.retryDelay(resp, 2)
	if delay != 400*time.Millisecond {
		t.Fatalf("expected 400ms, got %s", delay)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{301, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}
	for _, tt := range tests {
		if got := isRetryable(tt.code); got != tt.want {
			t.Errorf("isRetryable(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}
