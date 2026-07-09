---
name: drift-framework
description: Use when writing or modifying Go code that imports github.com/m1z23r/drift, working inside the drift repo itself, or building HTTP handlers, middleware, SSE endpoints, or WebSocket routes with drift. Triggers on imports of pkg/drift, pkg/middleware, pkg/websocket; on uses of drift.New, drift.Context, drift.HandlerFunc; on questions about drift routing/groups/middleware order.
---

# Drift Framework

> **Built for drift `v1.1.0`** (module `github.com/m1z23r/drift`, Go 1.25.2). Verify the target's version with `go list -m github.com/m1z23r/drift` (or `git describe --tags` inside the repo). If it differs, re-check this skill against the source before trusting the routing/panic details below.

Lightweight Go web framework (Gin-style API, zero third-party runtime deps, Go 1.25+). Public packages: `pkg/drift`, `pkg/middleware`, `pkg/websocket`. Routing is a radix tree **per HTTP method**.

If `DRIFT.md` is present at the repo root (i.e. you are inside the framework repo), read it first — it is the authoritative quick reference. Otherwise treat this skill as the cheat sheet and consult github.com/m1z23r/drift for canonical docs.

## Minimal app shape

```go
app := drift.New()
app.Use(middleware.Recovery(), middleware.CORS(), middleware.BodyParser())
app.Get("/users/:id", func(c *drift.Context) {
    c.JSON(200, map[string]string{"id": c.Param("id")})
})
app.Run(":8080")
```

Handler signature is always `func(*drift.Context)` (alias `drift.HandlerFunc`). Route registration takes `path string, handlers ...HandlerFunc` — the **last** handler is the endpoint; earlier ones are per-route middleware.

## Routing rules — get these wrong and startup panics

- One radix tree per HTTP method. `GET /x/:id` is fine alongside `POST /x/login` because they live in different trees.
- **Literal and `:param` siblings coexist** at the same slot within a single method tree; lookup prefers the literal on exact match. `app.Post("/x/login", …)` + `app.Post("/x/:id/y", …)` works.
- Panics at startup:
  - Two `:param` siblings with **different names** at the same slot in one method (e.g. `/x/:id` + `/x/:slug`).
  - A `*catchall` next to **any** sibling — catch-all must be the only child at its slot.
- `:name` matches a single segment; `*name` is a catch-all and must be the last segment. Read both via `c.Param("name")`.

## Middleware

- `c.Next()` advances the chain; `c.Abort()` (or any `AbortWith…` / `c.BadRequest`-style helper) stops it. Not calling `Next()` is fine if you write the response — the chain auto-ends.
- Scope: `app.Use(...)` global, `group.Use(...)` group, trailing args on a route registration are per-route.
- Built-ins live in `pkg/middleware`: `Recovery`, `CORS`, `BodyParser`, `RateLimiter` / `PerRouteRateLimiter`, `CSRF`, `Secure` / `StrictSecure`, `Compress`, `SkipCompression`, `Timeout` (+ each has a `…WithConfig` variant where applicable).
- **WebSocket + global `Compress()` → always put `middleware.SkipCompression()` on the WS route**, otherwise the HTTP upgrade gets corrupted.
- `BodyParser()` stores the parsed body at `c.Get("body")` — there is no automatic struct binding from it; use `c.BindJSON(&v)` if you want a typed struct.
- `CSRF()` exposes the token at `c.GetString("csrf_token")`.

## Context cheat sheet

Request: `c.Param`, `c.QueryParam`, `c.DefaultQuery`, `c.GetHeader`, `c.Cookie`, `c.PostForm`, `c.DefaultPostForm`, `c.FormFile`, `c.SaveUploadedFile`, `c.BindJSON`, `c.ClientIP`.

Response: `c.JSON`, `c.String`, `c.HTML`, `c.Redirect`, `c.Status`, `c.Header`, `c.SetCookie`, `c.Data`, `c.File`, `c.FileAttachment`, `c.Stream`, `c.StreamReader`, `c.StreamBytes`.

State passing: `c.Set(key, val)`, `c.Get(key) (any, bool)`, typed helpers `c.GetString` / `c.GetInt` / `c.GetBool`.

HTTP errors (each sets status, aborts chain, writes `{code, message}` JSON; empty msg uses default status text): `c.BadRequest`, `c.Unauthorized`, `c.Forbidden`, `c.NotFound`, `c.MethodNotAllowed`, `c.Conflict`, `c.UnprocessableEntity`, `c.TooManyRequests`, `c.InternalServerError`, `c.NotImplemented`, `c.BadGateway`, `c.ServiceUnavailable`, `c.GatewayTimeout`. For any other code: `c.Error(code, msg)` or `c.ErrorWithData(code, payload)`. Reusable error value: `drift.NewHTTPError(code, msg)`.

## SSE

```go
sse := c.SSE()
sse.Send(payload, event, id)
sse.SendJSON(v, event, id)
sse.SendComment("keepalive")
```

`Send` / `SendJSON` return an error when the client disconnects — break the loop on error. Pair with a ticker + `select` + timeout for periodic pushes.

## WebSockets

```go
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

Conn API: `ReadMessage` / `WriteMessage`, `WriteText` / `WriteBinary`, `ReadJSON` / `WriteJSON`, `Ping`, `SetReadLimit`, `SetReadDeadline` / `SetWriteDeadline`, `RemoteAddr` / `LocalAddr`, `Close(code, reason)`, `CloseCode()` / `CloseText()` (after disconnect). Message types: `TextMessage`, `BinaryMessage`. Standard close codes are exported as `websocket.CloseNormalClosure`, `CloseGoingAway`, `CloseProtocolError`, `CloseAbnormalClosure`, `ClosePolicyViolation`, `CloseMessageTooBig`, `CloseInternalServerErr`, etc.

Custom upgrader for non-default buffer sizes, read limits, `CheckOrigin`, or `Subprotocols`:

```go
upgrader := &websocket.Upgrader{ReadBufferSize: 4096, ReadLimit: 32 << 20, CheckOrigin: …, Subprotocols: []string{"graphql-ws"}}
conn, err := upgrader.Upgrade(c)
```

## Modes

```go
app.SetMode(drift.DebugMode)    // default; logs routes, requests, startup
app.SetMode(drift.ReleaseMode)  // silent; use in production
app.IsDebug()
```

## Project layout (importable boundary)

- `pkg/drift` — engine, context, router, errors. **Import this.**
- `pkg/middleware` — built-in middleware.
- `pkg/websocket` — RFC 6455 implementation.
- `internal/router` — radix tree implementation. **Not importable from outside.** When modifying routing semantics, edit here; expose changes via `pkg/drift`.
- `examples/*.go` — runnable demos (basic, SSE, WS, errors, mixed routes).

## Common mistakes

| Symptom | Cause | Fix |
|---|---|---|
| Panic: `wildcard '…' conflicts with existing wildcard '…'` | Two `:param` siblings with **different names** at the same slot in one method tree | Rename so the param name matches, or change method/prefix to separate them |
| Panic: `catch-all conflicts with …` (existing wildcard / static sibling / existing children) | A `*catchall` sharing a slot with any sibling | Give the catch-all its own slot, or change method/prefix so it has no siblings |
| WS handshake fails / mangled headers when `Compress()` is global | Compression middleware running on the upgrade response | Add `middleware.SkipCompression()` before the WS handler |
| `c.Get("body")` returns `nil, false` | `BodyParser()` not registered or running after the handler | Register `middleware.BodyParser()` globally before the route |
| Handler runs but response is empty / written twice | Missing `c.Next()` in middleware that also wrote a response, or both middleware and handler writing | Pick one writer; use `c.Abort()` to stop the chain when short-circuiting |
| Logs spamming in production | Default mode is debug | `app.SetMode(drift.ReleaseMode)` |

## When editing the framework itself

- Router-tree changes go in `internal/router/tree.go`; tests in `tree_test.go` + `pkg/drift/router_mixed_test.go`.
- `DRIFT.md` and `README.md` are the user-facing docs — keep them in sync with any public-API change.
- `examples/` should still build after API changes; `go run examples/main.go` is the smoke test.
