package drift

import (
	"reflect"
	"testing"
)

func TestEngine_Routes(t *testing.T) {
	e := New()
	e.SetMode(ReleaseMode)

	e.Get("/health", func(c *Context) {})

	api := e.Group("/api/v1")
	api.Get("/status", func(c *Context) {})
	api.Get("/", func(c *Context) {})

	empty := api.Group("")
	empty.Post("/things", func(c *Context) {})

	admin := api.Group("/admin")
	admin.Delete("/users/:id", func(c *Context) {})

	want := []RouteInfo{
		{Method: "GET", Path: "/health"},
		{Method: "GET", Path: "/api/v1/status"},
		{Method: "GET", Path: "/api/v1/"},
		{Method: "POST", Path: "/api/v1/things"},
		{Method: "DELETE", Path: "/api/v1/admin/users/:id"},
	}

	got := e.Routes()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Routes() = %+v, want %+v", got, want)
	}
}

func TestEngine_Routes_Any(t *testing.T) {
	e := New()
	e.SetMode(ReleaseMode)

	e.Any("/ping", func(c *Context) {})

	want := []RouteInfo{
		{Method: "GET", Path: "/ping"},
		{Method: "POST", Path: "/ping"},
		{Method: "PUT", Path: "/ping"},
		{Method: "DELETE", Path: "/ping"},
		{Method: "PATCH", Path: "/ping"},
		{Method: "OPTIONS", Path: "/ping"},
		{Method: "HEAD", Path: "/ping"},
	}

	got := e.Routes()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Routes() = %+v, want %+v", got, want)
	}
}

func TestEngine_Routes_CopySemantics(t *testing.T) {
	e := New()
	e.SetMode(ReleaseMode)
	e.Get("/a", func(c *Context) {})

	got := e.Routes()
	got[0].Path = "/mutated"

	again := e.Routes()
	if again[0].Path != "/a" {
		t.Fatalf("Routes() returned a slice sharing storage with the engine: got %q want %q", again[0].Path, "/a")
	}
}
