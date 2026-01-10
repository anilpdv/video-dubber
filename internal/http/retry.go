package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"video-translator/internal/config"
)

// RetryConfig configures retry behavior for HTTP requests.
type RetryConfig struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	BackoffFactor   float64
	RetryableStatus []int // HTTP status codes that should trigger a retry
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   config.DefaultMaxRetries,
		InitialDelay:  config.DefaultRetryDelayBase,
		BackoffFactor: 2.0,
		RetryableStatus: []int{
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

// isRetryableStatus checks if a status code should trigger a retry.
func isRetryableStatus(status int, retryable []int) bool {
	for _, s := range retryable {
		if s == status {
			return true
		}
	}
	return false
}

// DoWithRetry executes an HTTP request with exponential backoff retry.
// The request body must be resettable (use bytes.NewReader or similar).
func DoWithRetry(client *http.Client, req *http.Request, cfg RetryConfig) (*http.Response, error) {
	return DoWithRetryContext(context.Background(), client, req, cfg)
}

// DoWithRetryContext executes an HTTP request with retry and context support.
func DoWithRetryContext(ctx context.Context, client *http.Client, req *http.Request, cfg RetryConfig) (*http.Response, error) {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check context before making request
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Clone the request to allow retries
		reqClone := req.Clone(ctx)

		// Reset body if it's a seeker (like bytes.Reader)
		if req.Body != nil {
			if seeker, ok := req.Body.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart)
			}
		}

		resp, err := client.Do(reqClone)
		if err != nil {
			lastErr = err
			// Wait before retry
			if attempt < cfg.MaxAttempts {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				delay = time.Duration(float64(delay) * cfg.BackoffFactor)
			}
			continue
		}

		// Check if we should retry based on status code
		if isRetryableStatus(resp.StatusCode, cfg.RetryableStatus) && attempt < cfg.MaxAttempts {
			resp.Body.Close() // Close the response before retry
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * cfg.BackoffFactor)
			continue
		}

		// Success or non-retryable error
		return resp, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", cfg.MaxAttempts, lastErr)
}

// RetryFunc is a helper for retrying any function with exponential backoff.
type RetryFunc[T any] func() (T, error)

// Retry executes a function with exponential backoff retry.
func Retry[T any](fn RetryFunc[T], maxAttempts int, initialDelay time.Duration) (T, error) {
	return RetryWithContext(context.Background(), fn, maxAttempts, initialDelay)
}

// RetryWithContext executes a function with retry and context support.
func RetryWithContext[T any](ctx context.Context, fn RetryFunc[T], maxAttempts int, initialDelay time.Duration) (T, error) {
	var zero T
	var lastErr error
	delay := initialDelay

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check context before attempt
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Wait before retry
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * 2.0) // exponential backoff
		}
	}

	return zero, fmt.Errorf("failed after %d retries: %w", maxAttempts, lastErr)
}
