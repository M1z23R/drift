package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/m1z23r/drift"
)

// TimeoutConfig defines the config for timeout middleware
type TimeoutConfig struct {
	// Timeout is the maximum duration before timing out
	Timeout time.Duration

	// Handler is called when a timeout occurs
	// If nil, a default 408 Request Timeout response is sent
	Handler drift.HandlerFunc
}

// DefaultTimeoutConfig returns a default timeout configuration
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Timeout: 30 * time.Second,
		Handler: nil,
	}
}

// Timeout returns a timeout middleware with default config (30 seconds)
func Timeout() drift.HandlerFunc {
	return TimeoutWithConfig(DefaultTimeoutConfig())
}

// TimeoutWithDuration returns a timeout middleware with specified duration
func TimeoutWithDuration(timeout time.Duration) drift.HandlerFunc {
	return TimeoutWithConfig(TimeoutConfig{
		Timeout: timeout,
		Handler: nil,
	})
}

// TimeoutWithConfig returns a timeout middleware with custom config
func TimeoutWithConfig(config TimeoutConfig) drift.HandlerFunc {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.Handler == nil {
		config.Handler = func(c *drift.Context) {
			c.AbortWithStatusJSON(http.StatusRequestTimeout, map[string]string{
				"error": "Request Timeout",
			})
		}
	}

	return func(c *drift.Context) {
		// Create a context with timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), config.Timeout)
		defer cancel()

		// Replace the request context
		c.Request = c.Request.WithContext(ctx)

		// Channel to signal completion
		done := make(chan struct{})

		// Run the handler in a goroutine
		go func() {
			c.Next()
			close(done)
		}()

		// Wait for either completion or timeout
		select {
		case <-done:
			// Handler completed successfully
			return
		case <-ctx.Done():
			// Timeout occurred
			if ctx.Err() == context.DeadlineExceeded {
				config.Handler(c)
			}
		}
	}
}
