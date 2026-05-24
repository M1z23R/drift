package router

import (
	"strings"
	"testing"
)

// recordingHandler returns a handler that records its identity for lookup assertions.
func recordingHandler(id string) HandlerFunc {
	return id
}

// mustGet asserts a path resolves to the expected handler id and params.
func mustGet(t *testing.T, n *Node, path, wantID string, wantParams map[string]string) {
	t.Helper()
	handlers, params, _ := n.GetValue(path)
	if handlers == nil {
		t.Fatalf("GetValue(%q): got nil handlers, want %q", path, wantID)
	}
	got, ok := handlers[0].(string)
	if !ok || got != wantID {
		t.Fatalf("GetValue(%q): got handler %v, want %q", path, handlers[0], wantID)
	}
	if len(params) != len(wantParams) {
		t.Fatalf("GetValue(%q): got %d params, want %d (got=%v want=%v)", path, len(params), len(wantParams), params, wantParams)
	}
	for k, v := range wantParams {
		if params[k] != v {
			t.Fatalf("GetValue(%q): param %q = %q, want %q", path, k, params[k], v)
		}
	}
}

// mustMiss asserts a path resolves to no handler.
func mustMiss(t *testing.T, n *Node, path string) {
	t.Helper()
	handlers, _, _ := n.GetValue(path)
	if handlers != nil {
		t.Fatalf("GetValue(%q): got handlers %v, want miss", path, handlers)
	}
}

// mustPanic asserts that fn panics with a message containing want.
func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", want)
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, want) {
			t.Fatalf("expected panic containing %q, got %q", want, msg)
		}
	}()
	fn()
}

func TestStaticOnly(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/login", []HandlerFunc{recordingHandler("login")})
	n.AddRoute("/list/logout", []HandlerFunc{recordingHandler("logout")})

	mustGet(t, n, "/list/login", "login", nil)
	mustGet(t, n, "/list/logout", "logout", nil)
	mustMiss(t, n, "/list/other")
}

func TestParamOnly(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})

	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
	mustGet(t, n, "/list/abc", "byID", map[string]string{"id": "abc"})
	mustMiss(t, n, "/list/")
	mustMiss(t, n, "/list/42/extra")
}

func TestCatchAll(t *testing.T) {
	n := NewNode()
	n.AddRoute("/files/*path", []HandlerFunc{recordingHandler("files")})

	mustGet(t, n, "/files/a", "files", map[string]string{"path": "/a"})
	mustGet(t, n, "/files/a/b/c", "files", map[string]string{"path": "/a/b/c"})
}

func TestParamWithLiteralContinuation(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})
	n.AddRoute("/list/:id/edit", []HandlerFunc{recordingHandler("edit")})

	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
	mustGet(t, n, "/list/42/edit", "edit", map[string]string{"id": "42"})
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/login", []HandlerFunc{recordingHandler("a")})
	mustPanic(t, "already registered", func() {
		n.AddRoute("/list/login", []HandlerFunc{recordingHandler("b")})
	})
}

func TestMixedLiteralAndParam_ParamFirst(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})
	n.AddRoute("/list/login", []HandlerFunc{recordingHandler("login")})

	mustGet(t, n, "/list/login", "login", nil)
	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
}
