package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/m1z23r/drift/pkg/drift"
)

// RateLimiterConfig defines the config for rate limiter middleware
type RateLimiterConfig struct {
	// Max is the maximum number of requests allowed within the window
	Max int

	// Window is the time window for the rate limit
	Window time.Duration

	// KeyFunc is a function to generate a key for rate limiting
	// Default: uses client IP
	KeyFunc func(*drift.Context) string

	// Handler is called when rate limit is exceeded
	Handler drift.HandlerFunc
}

// tokenBucket represents a token bucket for rate limiting
type tokenBucket struct {
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

// rateLimiter manages rate limiting
type rateLimiter struct {
	buckets sync.Map
	config  RateLimiterConfig
}

// DefaultRateLimiterConfig returns a default rate limiter configuration
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		Max:    100,
		Window: time.Minute,
		KeyFunc: func(c *drift.Context) string {
			return c.ClientIP()
		},
		Handler: func(c *drift.Context) {
			c.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "Rate limit exceeded",
			})
		},
	}
}

// RateLimiter returns a rate limiter middleware with default config
func RateLimiter() drift.HandlerFunc {
	return RateLimiterWithConfig(DefaultRateLimiterConfig())
}

// RateLimiterWithConfig returns a rate limiter middleware with custom config
func RateLimiterWithConfig(config RateLimiterConfig) drift.HandlerFunc {
	// Set defaults
	if config.Max == 0 {
		config.Max = 100
	}
	if config.Window == 0 {
		config.Window = time.Minute
	}
	if config.KeyFunc == nil {
		config.KeyFunc = func(c *drift.Context) string {
			return c.ClientIP()
		}
	}
	if config.Handler == nil {
		config.Handler = func(c *drift.Context) {
			c.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "Rate limit exceeded",
			})
		}
	}

	limiter := &rateLimiter{
		config: config,
	}

	// Start cleanup routine
	go limiter.cleanup()

	return func(c *drift.Context) {
		key := config.KeyFunc(c)

		if !limiter.allow(key) {
			config.Handler(c)
			c.Abort()
			return
		}

		c.Next()
	}
}

// allow checks if a request is allowed based on the rate limit
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()

	// Get or create bucket
	value, _ := rl.buckets.LoadOrStore(key, &tokenBucket{
		tokens:     rl.config.Max,
		lastRefill: now,
	})

	bucket := value.(*tokenBucket)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Calculate time since last refill
	elapsed := now.Sub(bucket.lastRefill)

	// Refill tokens based on elapsed time
	if elapsed >= rl.config.Window {
		bucket.tokens = rl.config.Max
		bucket.lastRefill = now
	} else {
		// Partial refill based on elapsed time
		tokensToAdd := int(float64(rl.config.Max) * (elapsed.Seconds() / rl.config.Window.Seconds()))
		bucket.tokens += tokensToAdd
		if bucket.tokens > rl.config.Max {
			bucket.tokens = rl.config.Max
		}
		if tokensToAdd > 0 {
			bucket.lastRefill = now
		}
	}

	// Check if tokens are available
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}

	return false
}

// cleanup periodically removes old buckets
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.Window * 2)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		rl.buckets.Range(func(key, value any) bool {
			bucket := value.(*tokenBucket)
			bucket.mu.Lock()
			if now.Sub(bucket.lastRefill) > rl.config.Window*3 {
				rl.buckets.Delete(key)
			}
			bucket.mu.Unlock()
			return true
		})
	}
}

// PerRouteRateLimiter creates a rate limiter that can be applied per route
func PerRouteRateLimiter(max int, window time.Duration) drift.HandlerFunc {
	return RateLimiterWithConfig(RateLimiterConfig{
		Max:    max,
		Window: window,
	})
}
