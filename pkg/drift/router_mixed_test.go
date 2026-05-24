package drift

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEngine_MixedLiteralAndParamRoutes(t *testing.T) {
	cases := []struct {
		name     string
		register func(e *Engine)
		path     string
		wantCode int
		wantBody string
	}{
		{
			name: "literal_wins_over_param_param_first",
			register: func(e *Engine) {
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:%s", c.Params["id"]) })
				e.Get("/list/login", func(c *Context) { c.String(200, "login") })
			},
			path:     "/list/login",
			wantCode: 200,
			wantBody: "login",
		},
		{
			name: "param_matches_when_literal_doesnt_match_exactly",
			register: func(e *Engine) {
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:%s", c.Params["id"]) })
				e.Get("/list/login", func(c *Context) { c.String(200, "login") })
			},
			path:     "/list/42",
			wantCode: 200,
			wantBody: "byID:42",
		},
		{
			name: "literal_first_then_param_still_resolves_correctly",
			register: func(e *Engine) {
				e.Get("/list/login", func(c *Context) { c.String(200, "login") })
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:%s", c.Params["id"]) })
			},
			path:     "/list/42",
			wantCode: 200,
			wantBody: "byID:42",
		},
		{
			name: "backtracks_to_param_when_static_prefix_partial",
			register: func(e *Engine) {
				e.Get("/list/special", func(c *Context) { c.String(200, "special") })
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:%s", c.Params["id"]) })
			},
			path:     "/list/somethingweird",
			wantCode: 200,
			wantBody: "byID:somethingweird",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()
			e.SetMode(ReleaseMode)
			tc.register(e)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d want %d", rec.Code, tc.wantCode)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("body: got %q want %q", got, tc.wantBody)
			}
		})
	}
}
