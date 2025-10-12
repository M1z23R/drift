package middleware

import (
	"strings"

	"github.com/m1z23r/drift"
)

// CORSConfig defines the config for CORS middleware
type CORSConfig struct {
	// AllowOrigins defines a list of origins that may access the resource
	AllowOrigins []string

	// AllowMethods defines a list methods allowed when accessing the resource
	AllowMethods []string

	// AllowHeaders defines a list of request headers that can be used
	AllowHeaders []string

	// ExposeHeaders defines a whitelist headers that clients are allowed to access
	ExposeHeaders []string

	// AllowCredentials indicates whether the request can include user credentials
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results can be cached
	MaxAge int
}

// DefaultCORSConfig returns a default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders: []string{},
		AllowCredentials: false,
		MaxAge: 3600,
	}
}

// CORS returns a CORS middleware with default config
func CORS() drift.HandlerFunc {
	return CORSWithConfig(DefaultCORSConfig())
}

// CORSWithConfig returns a CORS middleware with custom config
func CORSWithConfig(config CORSConfig) drift.HandlerFunc {
	// Set defaults if not provided
	if len(config.AllowOrigins) == 0 {
		config.AllowOrigins = []string{"*"}
	}
	if len(config.AllowMethods) == 0 {
		config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}
	if len(config.AllowHeaders) == 0 {
		config.AllowHeaders = []string{"Origin", "Content-Type", "Accept"}
	}

	allowMethods := strings.Join(config.AllowMethods, ", ")
	allowHeaders := strings.Join(config.AllowHeaders, ", ")
	exposeHeaders := strings.Join(config.ExposeHeaders, ", ")

	return func(c *drift.Context) {
		origin := c.GetHeader("Origin")

		// Check if origin is allowed
		allowOrigin := "*"
		if len(config.AllowOrigins) > 0 && config.AllowOrigins[0] != "*" {
			allowOrigin = ""
			for _, o := range config.AllowOrigins {
				if o == origin || o == "*" {
					allowOrigin = origin
					break
				}
			}
			if allowOrigin == "" {
				allowOrigin = config.AllowOrigins[0]
			}
		}

		// Set CORS headers
		c.Header("Access-Control-Allow-Origin", allowOrigin)
		c.Header("Access-Control-Allow-Methods", allowMethods)
		c.Header("Access-Control-Allow-Headers", allowHeaders)

		if len(config.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", exposeHeaders)
		}

		if config.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if config.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", string(rune(config.MaxAge)))
		}

		// Handle preflight requests
		if c.Method() == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
