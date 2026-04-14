# Bug: Compress middleware sends gzipped body without `Content-Encoding` header

## Repro

Server with default `app.Use(middleware.Compress())`. Any response ≥ `MinLength` (default 1024 bytes) returned via `c.JSON()`.

```bash
curl -i -H 'Accept-Encoding: gzip' http://localhost:8080/some/endpoint --output /tmp/r.bin
head -10 /tmp/r.bin
```

Observed response:
```
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 405

<gzipped binary garbage>
```

Expected:
```
HTTP/1.1 200 OK
Content-Type: application/json
Content-Encoding: gzip
Vary: Accept-Encoding

<gzipped body>
```

The body **is** gzipped, but neither `Content-Encoding: gzip` nor `Vary: Accept-Encoding` is set. Browsers receive the gzipped bytes as `application/json` and render unreadable garbage.

## Root cause

`pkg/middleware/compress.go`, `compressResponseWriter.Write()`:

```go
func (w *compressResponseWriter) Write(data []byte) (int, error) {
    if !w.headerSet {
        w.headerSet = true
        if len(data) < w.minLength {
            return w.ResponseWriter.Write(data)
        }
        w.compressed = true
        w.ResponseWriter.Header().Set("Content-Encoding", w.encoding) // ← too late
        w.ResponseWriter.Header().Del("Content-Length")
        w.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
    }
    // ... gzip writer.Write(data) ...
}
```

`drift.Context.JSON()` (and similar response helpers) call the underlying `http.ResponseWriter.WriteHeader(status)` **before** they call `Write(body)`. Once `WriteHeader` fires, the Go `net/http` server flushes the response headers to the wire — any `w.Header().Set(...)` calls after that are silently ignored. By the time the compress middleware tries to set `Content-Encoding`, the headers are already gone.

Additionally, `Content-Length` was set by drift before the body was written (to the original uncompressed size) and was sent to the client unchanged. The actual body delivered is the compressed one — so `Content-Length` is wrong too (it happens to match the compressed length in some test outputs only by accident, depending on what `c.JSON` does internally).

The middleware also overrides `WriteHeader` but doesn't intercept it to make the compression decision there:

```go
func (w *compressResponseWriter) WriteHeader(statusCode int) {
    w.ResponseWriter.WriteHeader(statusCode) // headers committed here, body decision deferred to Write()
}
```

## Fix

Make the compression decision **before** `WriteHeader` is allowed through. Two approaches:

### Option A: Buffer-then-decide (simplest, correct)

Buffer the response body in memory. After the handler returns, decide whether to compress based on total buffered length, then set headers and write to the underlying response.

```go
type compressResponseWriter struct {
    http.ResponseWriter
    buf        bytes.Buffer
    statusCode int
    encoding   string
    minLength  int
    config     CompressionConfig
}

func (w *compressResponseWriter) Write(data []byte) (int, error) {
    return w.buf.Write(data)
}

func (w *compressResponseWriter) WriteHeader(statusCode int) {
    w.statusCode = statusCode // defer the actual call
}

func (w *compressResponseWriter) flush() error {
    body := w.buf.Bytes()
    h := w.ResponseWriter.Header()

    if len(body) >= w.minLength && w.encoding != "" {
        // Compress.
        var compressed bytes.Buffer
        var writer io.WriteCloser
        switch w.encoding {
        case "gzip":
            gw, _ := gzip.NewWriterLevel(&compressed, w.config.Level)
            writer = gw
        case "deflate":
            dw, _ := flate.NewWriter(&compressed, w.config.Level)
            writer = dw
        }
        if _, err := writer.Write(body); err != nil { return err }
        if err := writer.Close(); err != nil { return err }

        h.Set("Content-Encoding", w.encoding)
        h.Set("Vary", "Accept-Encoding")
        h.Set("Content-Length", strconv.Itoa(compressed.Len()))

        if w.statusCode == 0 { w.statusCode = 200 }
        w.ResponseWriter.WriteHeader(w.statusCode)
        _, err := w.ResponseWriter.Write(compressed.Bytes())
        return err
    }

    // Below threshold or no encoding: write through uncompressed.
    h.Set("Content-Length", strconv.Itoa(len(body)))
    if w.statusCode == 0 { w.statusCode = 200 }
    w.ResponseWriter.WriteHeader(w.statusCode)
    _, err := w.ResponseWriter.Write(body)
    return err
}
```

Then in the middleware, call `crw.flush()` after `c.Next()` returns instead of relying on `writer.Close()`.

**Tradeoff:** buffers the entire response body in memory. Fine for typical JSON APIs (responses are small kB). Bad for large file streams — but those should use `c.Stream()`/`c.File()` and get `SkipCompression()` anyway.

### Option B: Streaming with header-decision-on-first-write

Keep streaming behaviour but make sure headers are committed correctly:

1. Override `WriteHeader(status)` to **defer** the call (don't propagate immediately).
2. On first `Write(data)`:
   - If `len(data) < minLength`: pass through uncompressed, propagate status + Content-Length, then propagate the body.
   - Otherwise: set `Content-Encoding`, `Vary`, **delete** `Content-Length` (chunked encoding takes over), propagate status, then write to gzip writer.
3. On subsequent writes: write through to whichever writer was selected.
4. After handler returns, if compressed, flush+close the gzip writer.
5. If `Write` was never called (empty body), still propagate the deferred `WriteHeader` call.

This requires more bookkeeping but keeps streaming responses streaming.

## Tests to add

```go
func TestCompress_setsContentEncodingForLargeJSON(t *testing.T) {
    app := drift.New()
    app.Use(middleware.Compress())
    app.Get("/big", func(c *drift.Context) {
        body := strings.Repeat("hello world ", 200) // ~2400 bytes
        c.JSON(200, map[string]string{"data": body})
    })

    req := httptest.NewRequest("GET", "/big", nil)
    req.Header.Set("Accept-Encoding", "gzip")
    rec := httptest.NewRecorder()
    app.ServeHTTP(rec, req)

    if rec.Header().Get("Content-Encoding") != "gzip" {
        t.Fatalf("expected Content-Encoding: gzip, got %q", rec.Header().Get("Content-Encoding"))
    }
    if rec.Header().Get("Vary") != "Accept-Encoding" {
        t.Fatalf("expected Vary: Accept-Encoding")
    }

    // Body must be valid gzip and decode to expected JSON
    gz, err := gzip.NewReader(rec.Body)
    if err != nil { t.Fatal(err) }
    decoded, _ := io.ReadAll(gz)
    if !strings.Contains(string(decoded), "hello world") {
        t.Fatalf("decoded body missing expected content")
    }
}

func TestCompress_doesNotCompressSmallResponses(t *testing.T) {
    app := drift.New()
    app.Use(middleware.Compress())
    app.Get("/small", func(c *drift.Context) {
        c.JSON(200, map[string]string{"ok": "yes"})
    })

    req := httptest.NewRequest("GET", "/small", nil)
    req.Header.Set("Accept-Encoding", "gzip")
    rec := httptest.NewRecorder()
    app.ServeHTTP(rec, req)

    if rec.Header().Get("Content-Encoding") != "" {
        t.Fatalf("small response should not be compressed")
    }
}

func TestCompress_skipsWhenAcceptEncodingMissing(t *testing.T) { ... }
func TestCompress_handlesEmptyBody(t *testing.T) { ... }
func TestCompress_preservesStatusCode(t *testing.T) {
    // Verify that c.NotFound() etc. still send the right status code through.
}
```

## Acceptance criteria

- Browsers receive `Content-Encoding: gzip` with gzipped bodies and decompress them transparently.
- `Content-Length` matches the actual bytes on the wire (compressed length when compressed, uncompressed otherwise).
- `Vary: Accept-Encoding` is set when a response was conditionally compressed.
- Status codes (200, 4xx, 5xx) are preserved correctly.
- Small responses still pass through uncompressed.
- All existing `SkipCompression()` semantics still work.
- Curl without `--compressed` shows raw gzipped bytes; curl with `--compressed` shows decoded JSON.
