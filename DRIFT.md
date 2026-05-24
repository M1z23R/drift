# DRIFT.md

Quick, comprehensive guide to using the Drift web framework.

## Install

```bash
go get github.com/m1z23r/drift/pkg/drift
go get github.com/m1z23r/drift/pkg/middleware
go get github.com/m1z23r/drift/pkg/websocket
```

Requires Go 1.25+. Zero third-party runtime dependencies.

## Minimal Server

```go
package main

import (
    "github.com/m1z23r/drift/pkg/drift"
    "github.com/m1z23r/drift/pkg/middleware"
)

func main() {
    app := drift.New()
    app.Use(middleware.Recovery(), middleware.CORS(), middleware.BodyParser())

    app.Get("/users/:id", func(c *drift.Context) {
        c.JSON(200, map[string]string{"id": c.Param("id")})
    })

    app.Run(":8080")
}
```

## Routing

### HTTP methods

```go
app.Get(path, handlers...)
app.Post(path, handlers...)
app.Put(path, handlers...)
app.Patch(path, handlers...)
app.Delete(path, handlers...)
app.Options(path, handlers...)
app.Head(path, handlers...)
app.Any(path, handlers...) // all methods
```

Every method takes `path string, handlers ...HandlerFunc`. The last is the endpoint; earlier ones are per-route middleware.

### Path parameters

- `:name` — single segment, read with `c.Param("name")`
- `*name` — catch-all (must be the last segment), read with `c.Param("name")`

### Groups

```go
api := app.Group("/api/v1", authMiddleware) // optional group middleware
api.Use(loggerMiddleware)                    // more group middleware
api.Get("/status", statusHandler)

admin := api.Group("/admin", requireAdmin)   // groups nest
admin.Get("/dashboard", dashboardHandler)
```

### Router specificity

Drift uses a radix tree **per HTTP method**. Literal segments and `:param` segments can coexist at the same slot; lookups prefer the most specific match (literal wins on exact match), regardless of declaration order.

```go
// Both allowed in any order. /x/login → literal; /x/42 → :id with id=42.
app.Post("/x/login", loginHandler)
app.Post("/x/:id/y", byIDHandler)
```

What still panics at startup:

- Two `:param` siblings with different names at the same slot (e.g. `/x/:id` + `/x/:slug`) — ambiguous, almost always a user bug.
- A `*catchall` next to any sibling — catch-all must be the only child at its slot.

## Middleware

Signature: `type HandlerFunc func(*Context)`. Call `c.Next()` to continue; not calling it is fine if you write the response (chain auto-ends). Call `c.Abort()` to stop the chain.

```go
app.Use(mw1, mw2)                       // global
group := app.Group("/p", mw3); group.Use(mw4) // group
app.Get("/a", mw5, handler)             // per-route
```

### Built-in middleware (pkg/middleware)

| Middleware | Purpose |
|---|---|
| `Recovery()` / `RecoveryWithConfig` / `RecoveryWithHandler` | Catches panics, returns 500 with stack |
| `CORS()` / `CORSWithConfig` | CORS headers, preflight handling |
| `BodyParser()` | Parses JSON / form / URL-encoded into `c.Get("body")` |
| `RateLimiter()` / `PerRouteRateLimiter(n, dur)` | Token-bucket rate limiting per IP |
| `CSRF()` | Double-submit cookie CSRF; token at `c.GetString("csrf_token")` |
| `Secure()` / `StrictSecure()` / `SecureWithConfig` | HSTS, CSP, XFO, XSS headers |
| `Compress()` / `CompressWithConfig` | gzip/deflate response compression |
| `SkipCompression()` | Bypass compression (required for WebSocket routes when `Compress()` is global) |
| `Timeout()` / `TimeoutWithDuration(d)` / `TimeoutWithConfig` | Per-request timeout with custom handler |

## Context

### Request

```go
c.Param("id")                   // path param
c.QueryParam("q")               // ?q=
c.DefaultQuery("page", "1")
c.GetHeader("Authorization")
c.Cookie("session")             // (value, err)
c.PostForm("username")
c.DefaultPostForm("field", "x")
c.FormFile("file")              // *multipart.FileHeader
c.SaveUploadedFile(fh, dstPath)
c.BindJSON(&struct)
c.ClientIP()
```

### Response

```go
c.JSON(200, any)
c.String(200, "Hi %s", name)
c.HTML(200, "<h1>...</h1>")
c.Redirect(302, "/login")
c.Status(204)
c.Header("X-Custom", "v")
c.SetCookie(name, val, maxAge, path, domain, secure, httpOnly)

// Bytes / files / streams
c.Data(200, "application/pdf", bytes)
c.File("/abs/path.pdf")
c.FileAttachment("/abs/path.pdf", "download.pdf")
c.Stream(200, "video/mp4", reader)
c.StreamReader(reader, "application/json")
c.StreamBytes(200, "image/png", imgBytes)
```

### Flow control

```go
c.Next()
c.Abort()
c.AbortWithStatus(401)
c.AbortWithStatusJSON(403, payload)
```

### Passing data through the chain

```go
c.Set("userID", 42)
c.Get("userID")          // (any, bool)
c.GetString("role")      // typed helpers: GetString/GetInt/GetBool
```

## HTTP Error Helpers

All set status, abort the chain, and write `{code, message}` JSON. Empty message uses the default status text.

```go
c.BadRequest("")          // 400
c.Unauthorized("")        // 401
c.Forbidden("")           // 403
c.NotFound("")            // 404
c.MethodNotAllowed("")    // 405
c.Conflict("")            // 409
c.UnprocessableEntity("") // 422
c.TooManyRequests("")     // 429
c.InternalServerError("") // 500
c.NotImplemented("")      // 501
c.BadGateway("")          // 502
c.ServiceUnavailable("")  // 503
c.GatewayTimeout("")      // 504

c.Error(418, "I'm a teapot")           // any status
c.ErrorWithData(422, validationMap)    // custom body
drift.NewHTTPError(503, "DB down")     // reusable HTTPError
```

## Modes

```go
app.SetMode(drift.DebugMode)    // default: logs routes + requests
app.SetMode(drift.ReleaseMode)  // silent
app.IsDebug()
```

Debug mode logs route registration, request method/path/status/duration, and startup info. Release mode disables all framework logs.

## Server-Sent Events

```go
app.Get("/events", func(c *drift.Context) {
    sse := c.SSE()
    sse.Send("text payload", "eventName", "id-1")
    sse.SendJSON(map[string]any{"x": 1}, "update", "id-2")
    sse.SendComment("keepalive")
})
```

`Send`/`SendJSON` return an error when the client disconnects — break the loop on error. Pair with a ticker and a timeout/`select` for periodic pushes.

## WebSockets

```go
import "github.com/m1z23r/drift/pkg/websocket"

app.Get("/ws", middleware.SkipCompression(), func(c *drift.Context) {
    conn, err := websocket.Upgrade(c)
    if err != nil { return }
    defer conn.Close(websocket.CloseNormalClosure, "bye")

    for {
        t, data, err := conn.ReadMessage()
        if err != nil { break }
        conn.WriteMessage(t, data)
    }
})
```

**Always** use `middleware.SkipCompression()` on WebSocket routes when `Compress()` is registered globally — it corrupts the HTTP upgrade otherwise.

### Connection API

```go
conn.ReadMessage()                    // (type, data, err)
conn.WriteMessage(type, data)
conn.WriteText(s) / WriteBinary(b)
conn.ReadJSON(&v) / WriteJSON(v)
conn.Ping(payload)
conn.SetReadLimit(bytes)
conn.SetReadDeadline(t) / SetWriteDeadline(t)
conn.RemoteAddr() / LocalAddr()
conn.Close(code, reason)
conn.CloseCode() / CloseText()        // after disconnect
```

### Custom upgrader

```go
upgrader := &websocket.Upgrader{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    ReadLimit:       32 << 20,
    CheckOrigin:     func(r *http.Request) bool { return r.Header.Get("Origin") == "https://x.com" },
    Subprotocols:    []string{"graphql-ws"},
}
conn, err := upgrader.Upgrade(c)
```

Message types: `TextMessage`, `BinaryMessage`. Close codes: `CloseNormalClosure` (1000), `CloseGoingAway` (1001), `CloseProtocolError` (1002), `CloseUnsupportedData` (1003), `CloseAbnormalClosure` (1006), `ClosePolicyViolation` (1008), `CloseMessageTooBig` (1009), `CloseInternalServerErr` (1011), and more.

## Project Layout

```
pkg/
  drift/       # engine, context, router, errors — import this
  middleware/  # built-in middlewares
  websocket/   # RFC 6455 implementation
internal/
  router/      # radix tree (not importable)
examples/
  main.go
  sse_example.go
  websocket_example.go
  errors_example.go
```

## Running Examples

```bash
go run examples/main.go              # :8080 — routing + middleware demo
go run examples/sse_example.go       # :8080/sse — live SSE demo
go run examples/websocket_example.go # :8080/ws — echo server
go run examples/errors_example.go    # error helpers
```
