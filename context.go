package drift

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// Context represents the context of the current HTTP request
type Context struct {
	Request  *http.Request
	Response http.ResponseWriter
	Params   map[string]string // URL parameters (:id, etc.)
	Query    url.Values        // Query string parameters

	// Middleware chain management
	handlers []HandlerFunc
	index    int8

	// Context data storage (like gin's Set/Get)
	mu     sync.RWMutex
	store  map[string]any
	aborted bool

	// Response status
	statusCode int
}

// HandlerFunc defines the handler function type
type HandlerFunc func(*Context)

// newContext creates a new Context instance
func newContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{
		Request:    r,
		Response:   w,
		Params:     make(map[string]string),
		Query:      r.URL.Query(),
		store:      make(map[string]any),
		index:      -1,
		statusCode: http.StatusOK,
	}
}

// Next executes the next handler in the middleware chain
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		if c.aborted {
			return
		}
		c.handlers[c.index](c)
		c.index++
	}
}

// Abort prevents pending handlers from being called
func (c *Context) Abort() {
	c.aborted = true
}

// AbortWithStatus aborts the chain and writes the HTTP status code
func (c *Context) AbortWithStatus(code int) {
	c.Status(code)
	c.Abort()
}

// AbortWithStatusJSON aborts the chain and writes JSON response
func (c *Context) AbortWithStatusJSON(code int, data any) {
	c.Abort()
	c.JSON(code, data)
}

// IsAborted returns true if the current context was aborted
func (c *Context) IsAborted() bool {
	return c.aborted
}

// Set stores a new key/value pair in the context
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = value
}

// Get retrieves a value from the context by key
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists := c.store[key]
	return value, exists
}

// MustGet retrieves a value from the context or panics if it doesn't exist
func (c *Context) MustGet(key string) any {
	if value, exists := c.Get(key); exists {
		return value
	}
	panic("Key \"" + key + "\" does not exist")
}

// GetString retrieves a string value from the context
func (c *Context) GetString(key string) string {
	if val, ok := c.Get(key); ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt retrieves an int value from the context
func (c *Context) GetInt(key string) int {
	if val, ok := c.Get(key); ok {
		if i, ok := val.(int); ok {
			return i
		}
	}
	return 0
}

// GetBool retrieves a bool value from the context
func (c *Context) GetBool(key string) bool {
	if val, ok := c.Get(key); ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// Param returns the value of the URL parameter
func (c *Context) Param(key string) string {
	return c.Params[key]
}

// QueryParam returns the query string parameter
func (c *Context) QueryParam(key string) string {
	return c.Query.Get(key)
}

// DefaultQuery returns the query parameter or a default value
func (c *Context) DefaultQuery(key, defaultValue string) string {
	if value := c.Query.Get(key); value != "" {
		return value
	}
	return defaultValue
}

// Status sets the HTTP status code
func (c *Context) Status(code int) {
	c.statusCode = code
	c.Response.WriteHeader(code)
}

// Header sets a response header
func (c *Context) Header(key, value string) {
	c.Response.Header().Set(key, value)
}

// GetHeader retrieves a request header
func (c *Context) GetHeader(key string) string {
	return c.Request.Header.Get(key)
}

// JSON sends a JSON response
func (c *Context) JSON(code int, data any) error {
	c.Header("Content-Type", "application/json")
	c.statusCode = code
	c.Response.WriteHeader(code)
	encoder := json.NewEncoder(c.Response)
	return encoder.Encode(data)
}

// String sends a plain text response
func (c *Context) String(code int, format string, values ...any) error {
	c.Header("Content-Type", "text/plain")
	c.statusCode = code
	c.Response.WriteHeader(code)
	if len(values) > 0 {
		_, err := fmt.Fprintf(c.Response, format, values...)
		return err
	}
	_, err := c.Response.Write([]byte(format))
	return err
}

// HTML sends an HTML response
func (c *Context) HTML(code int, html string) error {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.statusCode = code
	c.Response.WriteHeader(code)
	_, err := c.Response.Write([]byte(html))
	return err
}

// Data writes raw bytes to the response
func (c *Context) Data(code int, contentType string, data []byte) error {
	c.Header("Content-Type", contentType)
	c.statusCode = code
	c.Response.WriteHeader(code)
	_, err := c.Response.Write(data)
	return err
}

// Redirect sends an HTTP redirect
func (c *Context) Redirect(code int, location string) {
	if code < http.StatusMultipleChoices || code > http.StatusPermanentRedirect {
		code = http.StatusFound
	}
	http.Redirect(c.Response, c.Request, location, code)
}

// BindJSON binds the request body to a struct using JSON
func (c *Context) BindJSON(obj any) error {
	decoder := json.NewDecoder(c.Request.Body)
	return decoder.Decode(obj)
}

// PostForm returns the form value for the given key
func (c *Context) PostForm(key string) string {
	return c.Request.FormValue(key)
}

// DefaultPostForm returns the form value or a default value
func (c *Context) DefaultPostForm(key, defaultValue string) string {
	if value := c.Request.FormValue(key); value != "" {
		return value
	}
	return defaultValue
}

// FormFile retrieves a file from a multipart form
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	_, fh, err := c.Request.FormFile(name)
	return fh, err
}

// MultipartForm returns the multipart form
func (c *Context) MultipartForm() (*multipart.Form, error) {
	err := c.Request.ParseMultipartForm(32 << 20) // 32 MB
	return c.Request.MultipartForm, err
}

// SaveUploadedFile saves an uploaded file to dst
func (c *Context) SaveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := createFile(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// Cookie retrieves a cookie by name
func (c *Context) Cookie(name string) (string, error) {
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// SetCookie adds a Set-Cookie header
func (c *Context) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		Path:     path,
		Domain:   domain,
		Secure:   secure,
		HttpOnly: httpOnly,
	}
	http.SetCookie(c.Response, cookie)
}

// ClientIP returns the client's IP address
func (c *Context) ClientIP() string {
	// Check X-Forwarded-For header
	if ip := c.GetHeader("X-Forwarded-For"); ip != "" {
		return ip
	}
	// Check X-Real-IP header
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return ip
	}
	// Fall back to RemoteAddr
	return c.Request.RemoteAddr
}

// Method returns the HTTP method
func (c *Context) Method() string {
	return c.Request.Method
}

// Path returns the request path
func (c *Context) Path() string {
	return c.Request.URL.Path
}

// FullPath returns the matched route full path
func (c *Context) FullPath() string {
	// This will be set during routing
	if path, ok := c.Get("_fullPath"); ok {
		return path.(string)
	}
	return c.Request.URL.Path
}

// Stream writes data from an io.Reader to the response
// This allows streaming large files without loading them into memory
func (c *Context) Stream(code int, contentType string, reader io.Reader) error {
	c.Header("Content-Type", contentType)
	c.statusCode = code
	c.Response.WriteHeader(code)
	_, err := io.Copy(c.Response, reader)
	return err
}

// File streams a file from the filesystem to the response
// The file is streamed directly without loading it into memory
func (c *Context) File(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get file info for content type detection
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Detect content type from file extension
	contentType := detectContentType(filepath)

	// Set headers
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	c.statusCode = http.StatusOK
	c.Response.WriteHeader(http.StatusOK)

	// Stream the file
	_, err = io.Copy(c.Response, file)
	return err
}

// FileAttachment streams a file as a downloadable attachment
func (c *Context) FileAttachment(filepath, filename string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Use provided filename or extract from path
	if filename == "" {
		filename = fileInfo.Name()
	}

	// Detect content type
	contentType := detectContentType(filepath)

	// Set headers for download
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	c.statusCode = http.StatusOK
	c.Response.WriteHeader(http.StatusOK)

	// Stream the file
	_, err = io.Copy(c.Response, file)
	return err
}

// StreamReader streams data from an io.Reader with the specified content type
// This is useful for streaming data from any source (databases, APIs, etc.)
func (c *Context) StreamReader(reader io.Reader, contentType string) error {
	c.Header("Content-Type", contentType)
	c.statusCode = http.StatusOK
	c.Response.WriteHeader(http.StatusOK)
	_, err := io.Copy(c.Response, reader)
	return err
}

// StreamBytes streams bytes with the specified content type
// Unlike Data(), this uses io.Copy which is more efficient for large data
func (c *Context) StreamBytes(code int, contentType string, data []byte) error {
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", len(data)))
	c.statusCode = code
	c.Response.WriteHeader(code)
	_, err := c.Response.Write(data)
	return err
}

// SSEWriter represents a Server-Sent Events writer
type SSEWriter struct {
	ctx    *Context
	writer http.ResponseWriter
}

// SSE initializes Server-Sent Events for this response
// Returns an SSEWriter that can be used to send events
func (c *Context) SSE() *SSEWriter {
	// Set headers for SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable buffering in nginx

	c.statusCode = http.StatusOK
	c.Response.WriteHeader(http.StatusOK)

	// Flush if the writer supports it
	if flusher, ok := c.Response.(http.Flusher); ok {
		flusher.Flush()
	}

	return &SSEWriter{
		ctx:    c,
		writer: c.Response,
	}
}

// Send sends an SSE event with optional event type and ID
func (s *SSEWriter) Send(data string, event string, id string) error {
	if id != "" {
		if _, err := fmt.Fprintf(s.writer, "id: %s\n", id); err != nil {
			return err
		}
	}

	if event != "" {
		if _, err := fmt.Fprintf(s.writer, "event: %s\n", event); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(s.writer, "data: %s\n\n", data); err != nil {
		return err
	}

	// Flush the data
	if flusher, ok := s.writer.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// SendJSON sends JSON data as an SSE event
func (s *SSEWriter) SendJSON(data any, event string, id string) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.Send(string(jsonData), event, id)
}

// SendComment sends an SSE comment (keeps connection alive)
func (s *SSEWriter) SendComment(comment string) error {
	if _, err := fmt.Fprintf(s.writer, ": %s\n\n", comment); err != nil {
		return err
	}

	if flusher, ok := s.writer.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// detectContentType detects the content type from file extension
func detectContentType(filePath string) string {
	ext := filepath.Ext(filePath)

	// Common content types
	contentTypes := map[string]string{
		".html": "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".tar":  "application/x-tar",
		".gz":   "application/gzip",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".mp3":  "audio/mpeg",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".txt":  "text/plain",
		".csv":  "text/csv",
	}

	if contentType, ok := contentTypes[ext]; ok {
		return contentType
	}

	return "application/octet-stream"
}
