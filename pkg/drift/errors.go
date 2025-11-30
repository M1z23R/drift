package drift

import "net/http"

// HTTPError represents a custom HTTP error
type HTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface
func (e *HTTPError) Error() string {
	return e.Message
}

// NewHTTPError creates a new HTTPError with the given status code and message
func NewHTTPError(code int, message string) *HTTPError {
	return &HTTPError{
		Code:    code,
		Message: message,
	}
}

// Common HTTP error helpers

// BadRequest returns a 400 Bad Request error
func (c *Context) BadRequest(message string) {
	if message == "" {
		message = "Bad Request"
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, HTTPError{
		Code:    http.StatusBadRequest,
		Message: message,
	})
}

// Unauthorized returns a 401 Unauthorized error
func (c *Context) Unauthorized(message string) {
	if message == "" {
		message = "Unauthorized"
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, HTTPError{
		Code:    http.StatusUnauthorized,
		Message: message,
	})
}

// Forbidden returns a 403 Forbidden error
func (c *Context) Forbidden(message string) {
	if message == "" {
		message = "Forbidden"
	}
	c.AbortWithStatusJSON(http.StatusForbidden, HTTPError{
		Code:    http.StatusForbidden,
		Message: message,
	})
}

// NotFound returns a 404 Not Found error
func (c *Context) NotFound(message string) {
	if message == "" {
		message = "Not Found"
	}
	c.AbortWithStatusJSON(http.StatusNotFound, HTTPError{
		Code:    http.StatusNotFound,
		Message: message,
	})
}

// MethodNotAllowed returns a 405 Method Not Allowed error
func (c *Context) MethodNotAllowed(message string) {
	if message == "" {
		message = "Method Not Allowed"
	}
	c.AbortWithStatusJSON(http.StatusMethodNotAllowed, HTTPError{
		Code:    http.StatusMethodNotAllowed,
		Message: message,
	})
}

// Conflict returns a 409 Conflict error
func (c *Context) Conflict(message string) {
	if message == "" {
		message = "Conflict"
	}
	c.AbortWithStatusJSON(http.StatusConflict, HTTPError{
		Code:    http.StatusConflict,
		Message: message,
	})
}

// UnprocessableEntity returns a 422 Unprocessable Entity error
func (c *Context) UnprocessableEntity(message string) {
	if message == "" {
		message = "Unprocessable Entity"
	}
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, HTTPError{
		Code:    http.StatusUnprocessableEntity,
		Message: message,
	})
}

// TooManyRequests returns a 429 Too Many Requests error
func (c *Context) TooManyRequests(message string) {
	if message == "" {
		message = "Too Many Requests"
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, HTTPError{
		Code:    http.StatusTooManyRequests,
		Message: message,
	})
}

// InternalServerError returns a 500 Internal Server Error
func (c *Context) InternalServerError(message string) {
	if message == "" {
		message = "Internal Server Error"
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, HTTPError{
		Code:    http.StatusInternalServerError,
		Message: message,
	})
}

// NotImplemented returns a 501 Not Implemented error
func (c *Context) NotImplemented(message string) {
	if message == "" {
		message = "Not Implemented"
	}
	c.AbortWithStatusJSON(http.StatusNotImplemented, HTTPError{
		Code:    http.StatusNotImplemented,
		Message: message,
	})
}

// BadGateway returns a 502 Bad Gateway error
func (c *Context) BadGateway(message string) {
	if message == "" {
		message = "Bad Gateway"
	}
	c.AbortWithStatusJSON(http.StatusBadGateway, HTTPError{
		Code:    http.StatusBadGateway,
		Message: message,
	})
}

// ServiceUnavailable returns a 503 Service Unavailable error
func (c *Context) ServiceUnavailable(message string) {
	if message == "" {
		message = "Service Unavailable"
	}
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, HTTPError{
		Code:    http.StatusServiceUnavailable,
		Message: message,
	})
}

// GatewayTimeout returns a 504 Gateway Timeout error
func (c *Context) GatewayTimeout(message string) {
	if message == "" {
		message = "Gateway Timeout"
	}
	c.AbortWithStatusJSON(http.StatusGatewayTimeout, HTTPError{
		Code:    http.StatusGatewayTimeout,
		Message: message,
	})
}

// Error returns a custom HTTP error with the given status code and message
// This allows users to create custom errors for any HTTP status code
func (c *Context) Error(code int, message string) {
	if message == "" {
		message = http.StatusText(code)
	}
	c.AbortWithStatusJSON(code, HTTPError{
		Code:    code,
		Message: message,
	})
}

// ErrorWithData returns a custom HTTP error with the given status code and custom data
// This allows users to send custom error responses with any structure
func (c *Context) ErrorWithData(code int, data any) {
	c.AbortWithStatusJSON(code, data)
}
