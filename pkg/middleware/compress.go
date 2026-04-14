package middleware

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/m1z23r/drift/pkg/drift"
)

// CompressionConfig defines the config for compression middleware
type CompressionConfig struct {
	// Level is the compression level (0-9 for gzip, -2 to 9 for deflate)
	// -1 = default compression
	// 0 = no compression
	// 1 = best speed
	// 9 = best compression
	Level int

	// MinLength is the minimum response size to compress (in bytes)
	MinLength int

	// ExcludedExtensions are file extensions that should not be compressed
	ExcludedExtensions []string

	// ExcludedPaths are paths that should not be compressed
	ExcludedPaths []string
}

// DefaultCompressionConfig returns a default compression configuration
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{
		Level:     -1,   // default compression
		MinLength: 1024, // 1 KB
		ExcludedExtensions: []string{
			".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico",
			".mp3", ".mp4", ".avi", ".mov", ".webm",
			".zip", ".tar", ".gz", ".bz2", ".7z",
			".pdf", ".woff", ".woff2", ".ttf", ".eot",
		},
		ExcludedPaths: []string{},
	}
}

// Compress returns a compression middleware with default config
// Use this as a route-level middleware to enable compression for specific routes
func Compress() drift.HandlerFunc {
	return CompressWithConfig(DefaultCompressionConfig())
}

// CompressGlobal returns a compression middleware with default config for global use
// This can be used with app.Use() to compress all responses
func CompressGlobal() drift.HandlerFunc {
	return CompressWithConfig(DefaultCompressionConfig())
}

// CompressWithConfig returns a compression middleware with custom config
func CompressWithConfig(config CompressionConfig) drift.HandlerFunc {
	// Set defaults
	if config.Level < -2 || config.Level > 9 {
		config.Level = -1
	}
	if config.MinLength == 0 {
		config.MinLength = 1024
	}

	return func(c *drift.Context) {
		if skip, exists := c.Get("skip_compression"); exists && skip.(bool) {
			c.Next()
			return
		}

		path := c.Path()
		for _, excludedPath := range config.ExcludedPaths {
			if strings.HasPrefix(path, excludedPath) {
				c.Next()
				return
			}
		}

		for _, ext := range config.ExcludedExtensions {
			if strings.HasSuffix(path, ext) {
				c.Next()
				return
			}
		}

		// Select encoding from Accept-Encoding. Empty encoding means write
		// through uncompressed (no client support).
		var encoding string
		acceptEncoding := c.GetHeader("Accept-Encoding")
		if strings.Contains(acceptEncoding, "gzip") {
			encoding = "gzip"
		} else if strings.Contains(acceptEncoding, "deflate") {
			encoding = "deflate"
		}

		crw := &compressResponseWriter{
			ResponseWriter: c.Response,
			encoding:       encoding,
			minLength:      config.MinLength,
			level:          config.Level,
		}

		original := c.Response
		c.Response = crw

		c.Next()

		// Restore and flush. If the connection was hijacked (websocket), the
		// writer is marked passthrough and there's nothing to flush.
		c.Response = original
		crw.flush()
	}
}

// compressResponseWriter buffers the response body and compresses it on flush
// once the full size is known, so that Content-Encoding / Content-Length /
// Vary headers can be set before WriteHeader is propagated to the client.
type compressResponseWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	encoding    string
	minLength   int
	level       int
	statusCode  int
	passthrough bool // set when buffering is bypassed (e.g. Hijack)
}

// Write buffers the response body. No data reaches the underlying writer
// until flush() runs, after the handler returns.
func (w *compressResponseWriter) Write(data []byte) (int, error) {
	if w.passthrough {
		return w.ResponseWriter.Write(data)
	}
	return w.buf.Write(data)
}

// WriteHeader captures the status code without propagating it. Headers remain
// mutable until flush() commits them.
func (w *compressResponseWriter) WriteHeader(statusCode int) {
	if w.passthrough {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}
	w.statusCode = statusCode
}

// Header returns the response headers
func (w *compressResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// flush commits headers and body to the underlying ResponseWriter, compressing
// the buffered body if it meets the threshold and the client supports it.
func (w *compressResponseWriter) flush() {
	if w.passthrough {
		return
	}

	status := w.statusCode
	if status == 0 {
		status = http.StatusOK
	}

	body := w.buf.Bytes()
	h := w.ResponseWriter.Header()

	if w.encoding != "" && len(body) >= w.minLength {
		var compressed bytes.Buffer
		var writer io.WriteCloser
		var err error

		switch w.encoding {
		case "gzip":
			writer, err = gzip.NewWriterLevel(&compressed, w.level)
		case "deflate":
			writer, err = flate.NewWriter(&compressed, w.level)
		}

		if err == nil {
			if _, werr := writer.Write(body); werr == nil {
				if cerr := writer.Close(); cerr == nil {
					h.Set("Content-Encoding", w.encoding)
					h.Set("Vary", "Accept-Encoding")
					h.Set("Content-Length", strconv.Itoa(compressed.Len()))
					w.ResponseWriter.WriteHeader(status)
					_, _ = w.ResponseWriter.Write(compressed.Bytes())
					return
				}
			}
		}
		// Fall through to uncompressed on any compression error.
	}

	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.ResponseWriter.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.ResponseWriter.Write(body)
	}
}

// Flush flushes buffered data through as uncompressed, then flushes the
// underlying writer. Once flushed, subsequent writes go straight through.
// This preserves streaming semantics for callers that explicitly flush, at
// the cost of skipping compression for that response.
func (w *compressResponseWriter) Flush() {
	if !w.passthrough {
		status := w.statusCode
		if status == 0 {
			status = http.StatusOK
		}
		body := w.buf.Bytes()
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.WriteHeader(status)
		if len(body) > 0 {
			_, _ = w.ResponseWriter.Write(body)
		}
		w.buf.Reset()
		w.passthrough = true
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker to support WebSocket upgrades. Hijacking
// bypasses compression entirely — the caller takes over the raw connection.
func (w *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("websocket: response does not implement http.Hijacker")
	}
	w.passthrough = true
	return hj.Hijack()
}

// SkipCompression is a middleware that prevents compression for the current route
// Use this on routes that should not be compressed (like SSE endpoints)
// Must be used BEFORE the Compress middleware in the handler chain
func SkipCompression() drift.HandlerFunc {
	return func(c *drift.Context) {
		c.Set("skip_compression", true)
		c.Next()
	}
}
