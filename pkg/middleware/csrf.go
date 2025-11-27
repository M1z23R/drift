package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/m1z23r/drift/pkg/drift"
)

// CSRFConfig defines the config for CSRF middleware
type CSRFConfig struct {
	// TokenLength defines the length of the CSRF token
	TokenLength int

	// TokenLookup is a string in the form of "<source>:<key>" that is used
	// to extract the token. Possible values:
	// - "header:<name>"
	// - "form:<name>"
	// - "query:<name>"
	// - "cookie:<name>"
	// Default: "header:X-CSRF-Token"
	TokenLookup string

	// CookieName is the name of the CSRF cookie
	CookieName string

	// CookiePath is the path of the CSRF cookie
	CookiePath string

	// CookieDomain is the domain of the CSRF cookie
	CookieDomain string

	// CookieSecure indicates if the CSRF cookie should only be sent over HTTPS
	CookieSecure bool

	// CookieHTTPOnly indicates if the CSRF cookie should be HTTP only
	CookieHTTPOnly bool

	// CookieSameSite defines the SameSite attribute of the CSRF cookie
	CookieSameSite http.SameSite

	// ErrorHandler defines a function which is executed for an invalid CSRF token
	ErrorHandler drift.HandlerFunc
}

// DefaultCSRFConfig returns a default CSRF configuration
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		TokenLength:    32,
		TokenLookup:    "header:X-CSRF-Token",
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieSecure:   false,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteStrictMode,
		ErrorHandler: func(c *drift.Context) {
			c.JSON(http.StatusForbidden, map[string]string{
				"error": "Invalid CSRF token",
			})
		},
	}
}

// CSRF returns a CSRF middleware with default config
func CSRF() drift.HandlerFunc {
	return CSRFWithConfig(DefaultCSRFConfig())
}

// CSRFWithConfig returns a CSRF middleware with custom config
func CSRFWithConfig(config CSRFConfig) drift.HandlerFunc {
	// Set defaults
	if config.TokenLength == 0 {
		config.TokenLength = 32
	}
	if config.TokenLookup == "" {
		config.TokenLookup = "header:X-CSRF-Token"
	}
	if config.CookieName == "" {
		config.CookieName = "_csrf"
	}
	if config.CookiePath == "" {
		config.CookiePath = "/"
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = func(c *drift.Context) {
			c.JSON(http.StatusForbidden, map[string]string{
				"error": "Invalid CSRF token",
			})
		}
	}

	return func(c *drift.Context) {
		// Skip CSRF check for safe methods
		method := c.Method()
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			// Generate and set token for safe methods
			token, err := generateToken(config.TokenLength)
			if err != nil {
				c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "Failed to generate CSRF token",
				})
				c.Abort()
				return
			}

			// Set cookie
			c.SetCookie(
				config.CookieName,
				token,
				3600, // 1 hour
				config.CookiePath,
				config.CookieDomain,
				config.CookieSecure,
				config.CookieHTTPOnly,
			)

			// Store token in context for access by handlers
			c.Set("csrf_token", token)
			c.Next()
			return
		}

		// For unsafe methods, validate the token
		cookieToken, err := c.Cookie(config.CookieName)
		if err != nil || cookieToken == "" {
			config.ErrorHandler(c)
			c.Abort()
			return
		}

		// Extract token from request based on TokenLookup
		requestToken := extractToken(c, config.TokenLookup)
		if requestToken == "" {
			config.ErrorHandler(c)
			c.Abort()
			return
		}

		// Compare tokens
		if !compareTokens(cookieToken, requestToken) {
			config.ErrorHandler(c)
			c.Abort()
			return
		}

		c.Next()
	}
}

// generateToken generates a random CSRF token
func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// extractToken extracts the CSRF token from the request
func extractToken(c *drift.Context, lookup string) string {
	// Parse lookup format: "source:key"
	parts := splitLookup(lookup)
	if len(parts) != 2 {
		return ""
	}

	source := parts[0]
	key := parts[1]

	switch source {
	case "header":
		return c.GetHeader(key)
	case "form":
		return c.PostForm(key)
	case "query":
		return c.QueryParam(key)
	case "cookie":
		token, _ := c.Cookie(key)
		return token
	default:
		return ""
	}
}

// compareTokens compares two CSRF tokens in constant time
func compareTokens(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	result := 0
	for i := 0; i < len(a); i++ {
		result |= int(a[i]) ^ int(b[i])
	}

	return result == 0
}

// splitLookup splits the lookup string
func splitLookup(lookup string) []string {
	for i := 0; i < len(lookup); i++ {
		if lookup[i] == ':' {
			return []string{lookup[:i], lookup[i+1:]}
		}
	}
	return []string{lookup}
}
