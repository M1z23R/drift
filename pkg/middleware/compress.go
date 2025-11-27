package middleware

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
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
		Level:     -1, // default compression
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
func Compress() drift.HandlerFunc {
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
		// Check if path is excluded
		path := c.Path()
		for _, excludedPath := range config.ExcludedPaths {
			if strings.HasPrefix(path, excludedPath) {
				c.Next()
				return
			}
		}

		// Check if extension is excluded
		for _, ext := range config.ExcludedExtensions {
			if strings.HasSuffix(path, ext) {
				c.Next()
				return
			}
		}

		// Get accepted encodings
		acceptEncoding := c.GetHeader("Accept-Encoding")
		if acceptEncoding == "" {
			c.Next()
			return
		}

		// Create a custom response writer
		var writer io.WriteCloser
		var encoding string

		if strings.Contains(acceptEncoding, "gzip") {
			encoding = "gzip"
			gzipWriter, err := gzip.NewWriterLevel(c.Response, config.Level)
			if err != nil {
				c.Next()
				return
			}
			writer = gzipWriter
		} else if strings.Contains(acceptEncoding, "deflate") {
			encoding = "deflate"
			deflateWriter, err := flate.NewWriter(c.Response, config.Level)
			if err != nil {
				c.Next()
				return
			}
			writer = deflateWriter
		} else {
			c.Next()
			return
		}

		// Wrap the response writer
		crw := &compressResponseWriter{
			ResponseWriter: c.Response,
			writer:         writer,
			encoding:       encoding,
			minLength:      config.MinLength,
		}

		// Replace the response writer
		c.Response = crw

		// Execute the next handler
		c.Next()

		// Close the writer to flush any remaining data
		writer.Close()
	}
}

// compressResponseWriter wraps http.ResponseWriter with compression
type compressResponseWriter struct {
	http.ResponseWriter
	writer    io.WriteCloser
	encoding  string
	minLength int
	written   int
	headerSet bool
}

// Write compresses and writes data to the response
func (w *compressResponseWriter) Write(data []byte) (int, error) {
	// Set headers on first write
	if !w.headerSet {
		w.headerSet = true

		// Check if we should compress based on content length
		if len(data) < w.minLength {
			// Don't compress, write directly
			return w.ResponseWriter.Write(data)
		}

		// Set compression header
		w.ResponseWriter.Header().Set("Content-Encoding", w.encoding)
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
	}

	// Write compressed data
	n, err := w.writer.Write(data)
	if err != nil {
		return n, err
	}

	w.written += n
	return len(data), nil // Return original length
}

// WriteHeader writes the status code
func (w *compressResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

// Header returns the response headers
func (w *compressResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// Flush flushes the compressed data
func (w *compressResponseWriter) Flush() {
	if flusher, ok := w.writer.(interface{ Flush() error }); ok {
		flusher.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
