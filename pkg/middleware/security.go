package middleware

import (
	"fmt"

	"github.com/m1z23r/drift/pkg/drift"
)

// SecurityConfig defines the config for security headers middleware
type SecurityConfig struct {
	// XSSProtection provides protection against cross-site scripting attacks
	// Default: "1; mode=block"
	XSSProtection string

	// ContentTypeNosniff prevents the browser from MIME-sniffing
	// Default: "nosniff"
	ContentTypeNosniff string

	// XFrameOptions can be used to indicate whether a browser should be allowed
	// to render a page in a <frame>, <iframe>, <embed> or <object>
	// Default: "SAMEORIGIN"
	XFrameOptions string

	// HSTSMaxAge sets the Strict-Transport-Security header (in seconds)
	// Default: 31536000 (1 year)
	HSTSMaxAge int

	// HSTSIncludeSubdomains adds includeSubDomains to the HSTS header
	// Default: false
	HSTSIncludeSubdomains bool

	// HSTSPreload adds preload to the HSTS header
	// Default: false
	HSTSPreload bool

	// ContentSecurityPolicy sets the Content-Security-Policy header
	// Default: ""
	ContentSecurityPolicy string

	// ReferrerPolicy sets the Referrer-Policy header
	// Default: "strict-origin-when-cross-origin"
	ReferrerPolicy string

	// PermissionsPolicy sets the Permissions-Policy header
	// Default: ""
	PermissionsPolicy string
}

// DefaultSecurityConfig returns a default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		XSSProtection:      "1; mode=block",
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "SAMEORIGIN",
		HSTSMaxAge:         31536000, // 1 year
		ReferrerPolicy:     "strict-origin-when-cross-origin",
	}
}

// Secure returns a security middleware with default config
func Secure() drift.HandlerFunc {
	return SecureWithConfig(DefaultSecurityConfig())
}

// SecureWithConfig returns a security middleware with custom config
func SecureWithConfig(config SecurityConfig) drift.HandlerFunc {
	// Set defaults
	if config.XSSProtection == "" {
		config.XSSProtection = "1; mode=block"
	}
	if config.ContentTypeNosniff == "" {
		config.ContentTypeNosniff = "nosniff"
	}
	if config.XFrameOptions == "" {
		config.XFrameOptions = "SAMEORIGIN"
	}
	if config.HSTSMaxAge == 0 {
		config.HSTSMaxAge = 31536000
	}
	if config.ReferrerPolicy == "" {
		config.ReferrerPolicy = "strict-origin-when-cross-origin"
	}

	return func(c *drift.Context) {
		// X-XSS-Protection
		if config.XSSProtection != "" {
			c.Header("X-XSS-Protection", config.XSSProtection)
		}

		// X-Content-Type-Options
		if config.ContentTypeNosniff != "" {
			c.Header("X-Content-Type-Options", config.ContentTypeNosniff)
		}

		// X-Frame-Options
		if config.XFrameOptions != "" {
			c.Header("X-Frame-Options", config.XFrameOptions)
		}

		// Strict-Transport-Security
		if config.HSTSMaxAge > 0 {
			hsts := fmt.Sprintf("max-age=%d", config.HSTSMaxAge)
			if config.HSTSIncludeSubdomains {
				hsts += "; includeSubDomains"
			}
			if config.HSTSPreload {
				hsts += "; preload"
			}
			c.Header("Strict-Transport-Security", hsts)
		}

		// Content-Security-Policy
		if config.ContentSecurityPolicy != "" {
			c.Header("Content-Security-Policy", config.ContentSecurityPolicy)
		}

		// Referrer-Policy
		if config.ReferrerPolicy != "" {
			c.Header("Referrer-Policy", config.ReferrerPolicy)
		}

		// Permissions-Policy
		if config.PermissionsPolicy != "" {
			c.Header("Permissions-Policy", config.PermissionsPolicy)
		}

		c.Next()
	}
}

// StrictSecure returns a security middleware with strict settings
func StrictSecure() drift.HandlerFunc {
	return SecureWithConfig(SecurityConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            63072000, // 2 years
		HSTSIncludeSubdomains: true,
		HSTSPreload:           true,
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
		PermissionsPolicy:     "geolocation=(), microphone=(), camera=()",
	})
}
