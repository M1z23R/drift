# Drift Web Framework

A fast, lightweight, and expressive web framework for Go, inspired by Gin and Express.js.

## Features

- **Intuitive API**: Familiar `.Get()`, `.Post()`, `.Put()`, `.Delete()`, `.Patch()` methods
- **Middleware Chain**: Global, router-level, and per-route middleware support
- **Dynamic Routing**: Support for URL parameters (`:id`) and catch-all routes (`*filepath`)
- **Context Data**: Pass data between middleware and handlers with `.Set()` and `.Get()`
- **Sub-routers**: Organize routes with `.Group()` for better code structure
- **Built-in Middleware**:
  - CORS with configurable origins, methods, and headers
  - Body parsing (JSON, form-data, URL-encoded)
  - Rate limiting with token bucket algorithm
  - CSRF protection using double-submit cookie pattern
  - Security headers (HSTS, XSS Protection, CSP, etc.)
  - Recovery/panic handling with stack traces
  - Response compression (gzip/deflate)
  - Request timeouts
- **Performance**: Radix tree-based routing for O(log n) route matching
- **Response Helpers**: `.JSON()`, `.String()`, `.HTML()`, `.Redirect()`, and more
- **File Streaming**: Efficient file serving without loading into memory
- **Server-Sent Events (SSE)**: Real-time server-to-client streaming
- **Environment Modes**: Debug and release modes with automatic logging control

## Installation

```bash
go get github.com/m1z23r/drift
```

## Quick Start

```go
package main

import (
    "github.com/m1z23r/drift"
    "github.com/m1z23r/drift/middleware"
)

func main() {
    // Create a new engine
    app := drift.New()

    // Global middleware
    app.Use(middleware.CORS())
    app.Use(middleware.BodyParser())

    // Routes
    app.Get("/", func(c *drift.Context) {
        c.JSON(200, map[string]string{
            "message": "Hello, World!",
        })
    })

    app.Get("/users/:id", func(c *drift.Context) {
        id := c.Param("id")
        c.JSON(200, map[string]string{
            "user_id": id,
        })
    })

    // Start server
    app.Run(":8080")
}
```

## HTTP Methods

Drift supports all standard HTTP methods:

```go
app.Get("/resource", handler)
app.Post("/resource", handler)
app.Put("/resource/:id", handler)
app.Patch("/resource/:id", handler)
app.Delete("/resource/:id", handler)
app.Options("/resource", handler)
app.Head("/resource", handler)
app.Any("/resource", handler) // Matches all methods
```

## Middleware

### Global Middleware

Apply middleware to all routes:

```go
app.Use(middleware.CORS())
app.Use(middleware.BodyParser())
app.Use(middleware.RateLimiter())
```

### Router Middleware

Apply middleware to a group of routes:

```go
api := app.Group("/api")
api.Use(authMiddleware)
{
    api.Get("/users", getUsers)
    api.Post("/users", createUser)
}
```

### Per-Route Middleware

Apply middleware to a specific route:

```go
app.Get("/admin", authMiddleware, adminHandler)
```

## Dynamic Parameters

### URL Parameters

```go
app.Get("/users/:id/posts/:postId", func(c *drift.Context) {
    userId := c.Param("id")
    postId := c.Param("postId")
    c.JSON(200, map[string]string{
        "user_id": userId,
        "post_id": postId,
    })
})
```

### Catch-all Parameters

```go
app.Get("/files/*filepath", func(c *drift.Context) {
    path := c.Param("filepath")
    // Serve file at path
})
```

## Context Data (Set/Get)

Pass data between middleware and handlers:

```go
// Middleware sets data
app.Use(func(c *drift.Context) {
    c.Set("user_id", "12345")
    c.Set("role", "admin")
    c.Next()
})

// Handler gets data
app.Get("/profile", func(c *drift.Context) {
    userId := c.GetString("user_id")
    role := c.GetString("role")
    c.JSON(200, map[string]any{
        "user_id": userId,
        "role": role,
    })
})
```

## Router Groups

Organize routes with prefixes and shared middleware:

```go
// API v1
v1 := app.Group("/api/v1")
v1.Use(func(c *drift.Context) {
    c.Header("X-API-Version", "1.0")
    c.Next()
})

v1.Get("/status", statusHandler)

// Admin routes (nested group)
admin := v1.Group("/admin", authMiddleware)
admin.Get("/dashboard", dashboardHandler)
admin.Get("/users", listUsersHandler)
```

## Built-in Middleware

### CORS

```go
app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
    AllowOrigins:     []string{"http://localhost:3000"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
    AllowCredentials: true,
    MaxAge:           3600,
}))
```

### Body Parser

Automatically parses JSON, form-data, and URL-encoded bodies:

```go
app.Use(middleware.BodyParser())

app.Post("/users", func(c *drift.Context) {
    body, _ := c.Get("body")
    c.JSON(201, body)
})
```

### Rate Limiting

```go
// Global rate limit: 100 requests per minute per IP
app.Use(middleware.RateLimiter())

// Per-route rate limit
app.Get("/expensive",
    middleware.PerRouteRateLimiter(10, time.Minute),
    handler,
)
```

### CSRF Protection

```go
protected := app.Group("/admin")
protected.Use(middleware.CSRF())

protected.Get("/form", func(c *drift.Context) {
    token := c.GetString("csrf_token")
    // Include token in form
})

protected.Post("/submit", handler)
```

### Security Headers

```go
// Default security headers
app.Use(middleware.Secure())

// Strict security headers
app.Use(middleware.StrictSecure())

// Custom configuration
app.Use(middleware.SecureWithConfig(middleware.SecurityConfig{
    XFrameOptions:         "DENY",
    ContentSecurityPolicy: "default-src 'self'",
    HSTSMaxAge:            31536000,
}))
```

### Recovery (Panic Handling)

```go
// Default recovery middleware
app.Use(middleware.Recovery())

// Custom recovery handler
app.Use(middleware.RecoveryWithHandler(func(c *drift.Context, err any) {
    log.Printf("Panic: %v", err)
    c.JSON(500, map[string]string{
        "error": "Internal Server Error",
    })
}))

// Custom configuration
app.Use(middleware.RecoveryWithConfig(middleware.RecoveryConfig{
    StackSize:       8 << 10, // 8 KB stack trace
    DisableStackAll: false,
    PrintStack:      true,
}))
```

### Compression

```go
// Default compression (gzip/deflate)
app.Use(middleware.Compress())

// Custom configuration
app.Use(middleware.CompressWithConfig(middleware.CompressionConfig{
    Level:     6, // 0-9, higher = better compression but slower
    MinLength: 1024, // Only compress responses > 1KB
    ExcludedExtensions: []string{".jpg", ".png", ".mp4"},
    ExcludedPaths: []string{"/api/no-compress"},
}))
```

### Timeout

```go
// Default timeout (30 seconds)
app.Use(middleware.Timeout())

// Custom timeout duration
app.Use(middleware.TimeoutWithDuration(5 * time.Second))

// Per-route timeout
app.Get("/slow", middleware.TimeoutWithDuration(10*time.Second), handler)

// Custom timeout handler
app.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
    Timeout: 5 * time.Second,
    Handler: func(c *drift.Context) {
        c.JSON(408, map[string]string{"error": "Too slow!"})
    },
}))
```

## Response Helpers

```go
// JSON response
c.JSON(200, map[string]string{"key": "value"})

// String response
c.String(200, "Hello, %s!", name)

// HTML response
c.HTML(200, "<h1>Hello</h1>")

// Redirect
c.Redirect(302, "/login")

// Status only
c.Status(204)

// File download (loads into memory)
c.Data(200, "application/pdf", pdfBytes)

// Stream file (efficient, no memory loading)
c.File("/path/to/file.pdf")

// Stream file as download
c.FileAttachment("/path/to/file.pdf", "custom-name.pdf")

// Stream from any io.Reader
c.Stream(200, "video/mp4", videoReader)
c.StreamReader(dataReader, "application/json")

// Stream bytes efficiently
c.StreamBytes(200, "image/png", imageBytes)
```

## Request Helpers

```go
// URL parameters
id := c.Param("id")

// Query parameters
name := c.QueryParam("name")
page := c.DefaultQuery("page", "1")

// Headers
auth := c.GetHeader("Authorization")
c.Header("X-Custom", "value")

// Cookies
token, _ := c.Cookie("session")
c.SetCookie("session", "abc123", 3600, "/", "", false, true)

// Form data
username := c.PostForm("username")
password := c.DefaultPostForm("password", "")

// File upload
file, _ := c.FormFile("file")
c.SaveUploadedFile(file, "/uploads/"+file.Filename)

// Bind JSON
var user User
c.BindJSON(&user)

// Client IP
ip := c.ClientIP()
```

## Middleware Chain Control

```go
// Continue to next handler
c.Next()

// Abort the chain
c.Abort()

// Abort with status
c.AbortWithStatus(401)

// Abort with JSON
c.AbortWithStatusJSON(403, map[string]string{
    "error": "Forbidden",
})
```

## Environment Modes

Control logging and debug output with environment modes:

```go
// Create engine (defaults to debug mode)
app := drift.New()

// Set to release mode (disables debug logs)
app.SetMode(drift.ReleaseMode)

// Set to debug mode (enables route registration and request logs)
app.SetMode(drift.DebugMode)

// Check current mode
if app.IsDebug() {
    // Do something only in debug mode
}
```

Debug mode automatically logs:
- Route registration when routes are added
- HTTP requests with method, path, status code, and duration
- Server startup information

Release mode disables all framework logs for production use.

## Server-Sent Events (SSE)

Stream real-time updates to clients:

```go
app.Get("/events", func(c *drift.Context) {
    sse := c.SSE()

    // Send simple text events
    sse.Send("Hello, World!", "", "")

    // Send events with event type and ID
    sse.Send("User logged in", "user-event", "123")

    // Send JSON data
    sse.SendJSON(map[string]any{
        "user": "john",
        "action": "login",
    }, "user-event", "124")

    // Send keepalive comments (keeps connection open)
    sse.SendComment("keepalive")
})
```

Real-world example with ticker:

```go
app.Get("/sse/time", func(c *drift.Context) {
    sse := c.SSE()
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    timeout := time.After(30 * time.Second)

    for {
        select {
        case <-timeout:
            return
        case t := <-ticker.C:
            err := sse.SendJSON(map[string]any{
                "timestamp": t.Unix(),
                "time": t.Format(time.RFC3339),
            }, "time-update", "")
            if err != nil {
                return // Client disconnected
            }
        }
    }
})
```

Client-side JavaScript:

```javascript
const eventSource = new EventSource('/events');

// Listen for specific event types
eventSource.addEventListener('time-update', (e) => {
    const data = JSON.parse(e.data);
    console.log('Time:', data.time);
});

// Handle errors
eventSource.onerror = () => {
    console.log('Connection lost');
    eventSource.close();
};
```

## Example Applications

### Basic Example

See [examples/main.go](examples/main.go) for a comprehensive example with routing, middleware, and context features.

```bash
go run examples/main.go
```

Then visit http://localhost:8080

### Advanced Features Example

See [examples/sse_example.go](examples/sse_example.go) for SSE, Recovery, Compression, and Timeout examples.

```bash
go run examples/sse_example.go
```

Then visit http://localhost:8080/sse for an interactive SSE demo

## Project Structure

```
drift/
├── drift.go           # Main engine with environment modes
├── context.go         # Request context with SSE support
├── router.go          # Router and groups
├── tree.go            # Radix tree for routing
├── middleware/
│   ├── cors.go        # CORS middleware
│   ├── bodyparser.go  # Body parsing middleware
│   ├── ratelimit.go   # Rate limiting middleware
│   ├── csrf.go        # CSRF protection
│   ├── security.go    # Security headers
│   ├── recovery.go    # Panic recovery
│   ├── compress.go    # Response compression
│   └── timeout.go     # Request timeouts
└── examples/
    ├── main.go        # Basic example
    └── sse_example.go # Advanced features example
```

## Performance

- **Radix Tree Routing**: O(log n) route matching
- **Context Pooling**: Reduces memory allocations
- **Zero Allocations**: For common operations
- **Minimal Dependencies**: Only standard library

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
