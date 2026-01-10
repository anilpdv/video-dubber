// Package http provides HTTP client utilities with connection pooling and retry logic.
package http

import (
	"net/http"
	"time"

	"video-translator/internal/config"
)

// ClientConfig configures the HTTP client behavior.
type ClientConfig struct {
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

// DefaultClientConfig returns the default HTTP client configuration.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:             config.HTTPTimeout,
		MaxIdleConns:        config.HTTPMaxIdleConns,
		MaxIdleConnsPerHost: config.HTTPMaxIdleConnsPerHost,
		IdleConnTimeout:     config.HTTPIdleConnTimeout,
	}
}

// NewPooledClient creates an HTTP client with connection pooling.
// This should be reused across requests to the same host for efficiency.
func NewPooledClient(cfg ClientConfig) *http.Client {
	return &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
			IdleConnTimeout:     cfg.IdleConnTimeout,
		},
	}
}

// NewDefaultClient creates an HTTP client with default pooling settings.
func NewDefaultClient() *http.Client {
	return NewPooledClient(DefaultClientConfig())
}

// Shared clients for different API endpoints
var (
	// OpenAIClient is a shared HTTP client for OpenAI API calls.
	OpenAIClient = NewDefaultClient()

	// DeepSeekClient is a shared HTTP client for DeepSeek API calls.
	DeepSeekClient = NewDefaultClient()

	// CosyVoiceClient is a shared HTTP client for CosyVoice API calls.
	// Uses a longer timeout for GPU-intensive synthesis.
	CosyVoiceClient = NewPooledClient(ClientConfig{
		Timeout:             2 * time.Minute,
		MaxIdleConns:        config.HTTPMaxIdleConns,
		MaxIdleConnsPerHost: config.HTTPMaxIdleConnsPerHost,
		IdleConnTimeout:     config.HTTPIdleConnTimeout,
	})
)
