package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/m1z23r/drift"
)

// BodyParserConfig defines the config for body parser middleware
type BodyParserConfig struct {
	// MaxBodySize defines the maximum allowed body size (in bytes)
	MaxBodySize int64
}

// DefaultBodyParserConfig returns a default body parser configuration
func DefaultBodyParserConfig() BodyParserConfig {
	return BodyParserConfig{
		MaxBodySize: 32 << 20, // 32 MB
	}
}

// BodyParser returns a body parser middleware with default config
func BodyParser() drift.HandlerFunc {
	return BodyParserWithConfig(DefaultBodyParserConfig())
}

// BodyParserWithConfig returns a body parser middleware with custom config
func BodyParserWithConfig(config BodyParserConfig) drift.HandlerFunc {
	if config.MaxBodySize == 0 {
		config.MaxBodySize = 32 << 20 // 32 MB
	}

	return func(c *drift.Context) {
		// Limit the request body size
		c.Request.Body = http.MaxBytesReader(c.Response, c.Request.Body, config.MaxBodySize)

		contentType := c.GetHeader("Content-Type")
		if contentType == "" {
			c.Next()
			return
		}

		// Parse media type
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			c.Next()
			return
		}

		switch {
		case mediaType == "application/json":
			parseJSON(c)
		case mediaType == "application/x-www-form-urlencoded":
			parseForm(c)
		case strings.HasPrefix(mediaType, "multipart/"):
			parseMultipart(c)
		}

		c.Next()
	}
}

// parseJSON parses JSON body
func parseJSON(c *drift.Context) {
	// Read the body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Set("_bodyParserError", err)
		return
	}

	// Restore the body for later use
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse JSON into a map
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		// Try parsing as array
		var arrayData []any
		if err2 := json.Unmarshal(body, &arrayData); err2 != nil {
			c.Set("_bodyParserError", err)
			return
		}
		c.Set("body", arrayData)
		c.Set("_bodyRaw", string(body))
		return
	}

	c.Set("body", data)
	c.Set("_bodyRaw", string(body))
}

// parseForm parses URL-encoded form data
func parseForm(c *drift.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.Set("_bodyParserError", err)
		return
	}

	// Convert form data to map
	formData := make(map[string]any)
	for key, values := range c.Request.PostForm {
		if len(values) == 1 {
			formData[key] = values[0]
		} else {
			formData[key] = values
		}
	}

	c.Set("body", formData)
}

// parseMultipart parses multipart form data
func parseMultipart(c *drift.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.Set("_bodyParserError", err)
		return
	}

	// Convert form data to map
	formData := make(map[string]any)

	// Add form values
	if c.Request.MultipartForm != nil {
		for key, values := range c.Request.MultipartForm.Value {
			if len(values) == 1 {
				formData[key] = values[0]
			} else {
				formData[key] = values
			}
		}

		// Add file information
		if len(c.Request.MultipartForm.File) > 0 {
			files := make(map[string]any)
			for key, fileHeaders := range c.Request.MultipartForm.File {
				if len(fileHeaders) == 1 {
					files[key] = map[string]any{
						"filename": fileHeaders[0].Filename,
						"size":     fileHeaders[0].Size,
						"header":   fileHeaders[0].Header,
					}
				} else {
					fileList := make([]map[string]any, len(fileHeaders))
					for i, fh := range fileHeaders {
						fileList[i] = map[string]any{
							"filename": fh.Filename,
							"size":     fh.Size,
							"header":   fh.Header,
						}
					}
					files[key] = fileList
				}
			}
			formData["_files"] = files
		}
	}

	c.Set("body", formData)
}
