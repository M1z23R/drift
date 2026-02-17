package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

// WebSocket GUID defined by RFC 6455
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

var (
	ErrNotWebSocket     = errors.New("not a websocket handshake")
	ErrMissingKey       = errors.New("missing Sec-WebSocket-Key header")
	ErrBadMethod        = errors.New("websocket: request method must be GET")
	ErrHijackNotSupport = errors.New("websocket: response does not implement http.Hijacker")
)

// validateHandshake validates the WebSocket upgrade request
func validateHandshake(r *http.Request) error {
	if r.Method != http.MethodGet {
		return ErrBadMethod
	}

	// Check for upgrade header
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return ErrNotWebSocket
	}

	// Check for connection upgrade
	connection := r.Header.Get("Connection")
	if !containsIgnoreCase(connection, "upgrade") {
		return ErrNotWebSocket
	}

	// Check WebSocket version
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return errors.New("websocket: unsupported version")
	}

	// Check for key
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		return ErrMissingKey
	}

	return nil
}

// computeAcceptKey computes the Sec-WebSocket-Accept value from the client key
func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// containsIgnoreCase checks if haystack contains needle (case-insensitive)
func containsIgnoreCase(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
