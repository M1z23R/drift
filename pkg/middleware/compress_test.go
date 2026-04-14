package middleware_test

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/m1z23r/drift/pkg/drift"
	"github.com/m1z23r/drift/pkg/middleware"
)

func newApp() *drift.Engine {
	app := drift.New()
	app.Use(middleware.Compress())
	return app
}

func TestCompress_setsContentEncodingForLargeJSON(t *testing.T) {
	app := newApp()
	app.Get("/big", func(c *drift.Context) {
		body := strings.Repeat("hello world ", 200) // ~2400 bytes
		c.JSON(200, map[string]string{"data": body})
	})

	req := httptest.NewRequest("GET", "/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected Content-Encoding: gzip, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("expected Vary: Accept-Encoding, got %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got == "" {
		t.Fatalf("expected Content-Length to be set")
	}

	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	decoded, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("failed to read gzip body: %v", err)
	}
	if !strings.Contains(string(decoded), "hello world") {
		t.Fatalf("decoded body missing expected content: %s", string(decoded))
	}
}

func TestCompress_doesNotCompressSmallResponses(t *testing.T) {
	app := newApp()
	app.Get("/small", func(c *drift.Context) {
		c.JSON(200, map[string]string{"ok": "yes"})
	})

	req := httptest.NewRequest("GET", "/small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("small response should not be compressed, got Content-Encoding=%q", got)
	}
	if !strings.Contains(rec.Body.String(), `"ok":"yes"`) {
		t.Fatalf("expected plain JSON body, got %q", rec.Body.String())
	}
}

func TestCompress_skipsWhenAcceptEncodingMissing(t *testing.T) {
	app := newApp()
	app.Get("/big", func(c *drift.Context) {
		body := strings.Repeat("x", 2048)
		c.JSON(200, map[string]string{"data": body})
	})

	req := httptest.NewRequest("GET", "/big", nil)
	// No Accept-Encoding header
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("should not compress without Accept-Encoding, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "xxxx") {
		t.Fatalf("expected uncompressed body")
	}
}

func TestCompress_handlesEmptyBody(t *testing.T) {
	app := newApp()
	app.Get("/empty", func(c *drift.Context) {
		c.Response.WriteHeader(204)
	})

	req := httptest.NewRequest("GET", "/empty", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("empty body should not be compressed, got %q", got)
	}
}

func TestCompress_preservesStatusCode(t *testing.T) {
	app := newApp()
	app.Get("/missing", func(c *drift.Context) {
		c.NotFound("")
	})

	req := httptest.NewRequest("GET", "/missing", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCompress_contentLengthMatchesBody(t *testing.T) {
	app := newApp()
	app.Get("/big", func(c *drift.Context) {
		body := strings.Repeat("hello world ", 200)
		c.JSON(200, map[string]string{"data": body})
	})

	req := httptest.NewRequest("GET", "/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	cl := rec.Header().Get("Content-Length")
	if cl == "" {
		t.Fatal("Content-Length missing")
	}
	if got, want := rec.Body.Len(), len(rec.Body.Bytes()); got != want {
		t.Fatalf("body length mismatch: %d vs %d", got, want)
	}
	// Content-Length should equal actual bytes on the wire.
	if cl != strconv.Itoa(rec.Body.Len()) {
		t.Fatalf("Content-Length %s != actual body len %d", cl, rec.Body.Len())
	}
}
