//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/m1z23r/drift/pkg/drift"
)

const addr = "127.0.0.1:8765"

func main() {
	app := drift.New()
	app.SetMode(drift.ReleaseMode)

	// ---- Users: literal vs :id, literal declared AFTER param ----
	app.Get("/users/:id", func(c *drift.Context) {
		c.String(http.StatusOK, "user:%s", c.Params["id"])
	})
	app.Get("/users/me", func(c *drift.Context) {
		c.String(http.StatusOK, "me")
	})
	app.Get("/users/special", func(c *drift.Context) {
		c.String(http.StatusOK, "special-user")
	})

	// ---- Posts: literal declared BEFORE param, with continuation ----
	app.Get("/posts/draft/comments", func(c *drift.Context) {
		c.String(http.StatusOK, "draft-comments")
	})
	app.Get("/posts/:id/comments", func(c *drift.Context) {
		c.String(http.StatusOK, "post-comments:%s", c.Params["id"])
	})
	app.Get("/posts/:id", func(c *drift.Context) {
		c.String(http.StatusOK, "post:%s", c.Params["id"])
	})

	// ---- Auth: literal POST + :provider GET (different methods, same prefix) ----
	app.Post("/auth/login", func(c *drift.Context) {
		c.String(http.StatusOK, "login-ok")
	})
	app.Get("/auth/:provider", func(c *drift.Context) {
		c.String(http.StatusOK, "oauth:%s", c.Params["provider"])
	})

	// ---- Deep nesting: mixed literal vs param at depth ----
	app.Get("/api/v1/resources/featured/items", func(c *drift.Context) {
		c.String(http.StatusOK, "featured-items")
	})
	app.Get("/api/v1/resources/:rid/items", func(c *drift.Context) {
		c.String(http.StatusOK, "items:%s", c.Params["rid"])
	})
	app.Get("/api/v1/resources/:rid", func(c *drift.Context) {
		c.String(http.StatusOK, "resource:%s", c.Params["rid"])
	})

	// ---- Catch-all on its own subtree (must be exclusive at its slot) ----
	app.Get("/files/*path", func(c *drift.Context) {
		c.String(http.StatusOK, "file:%s", c.Params["path"])
	})

	// ---- Backtracking edge case: static prefix partial match ----
	app.Get("/list/special", func(c *drift.Context) {
		c.String(http.StatusOK, "list-special")
	})
	app.Get("/list/:id", func(c *drift.Context) {
		c.String(http.StatusOK, "list:%s", c.Params["id"])
	})

	srv := &http.Server{Addr: addr, Handler: app}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	// Wait for server to bind.
	if err := waitReady(addr, 2*time.Second); err != nil {
		log.Fatalf("server did not become ready: %v", err)
	}

	type tc struct {
		method, path, wantBody string
		wantCode               int
	}
	cases := []tc{
		// Users (param-first declaration)
		{"GET", "/users/me", "me", 200},
		{"GET", "/users/special", "special-user", 200},
		{"GET", "/users/42", "user:42", 200},
		{"GET", "/users/somethingweird", "user:somethingweird", 200}, // backtrack from 's' static
		{"GET", "/users/m", "user:m", 200},                           // backtrack from 'm' static prefix

		// Posts (literal-first declaration, with continuation)
		{"GET", "/posts/draft/comments", "draft-comments", 200},
		{"GET", "/posts/42/comments", "post-comments:42", 200},
		{"GET", "/posts/draft", "post:draft", 200}, // literal "draft" as :id when no continuation
		{"GET", "/posts/99", "post:99", 200},

		// Auth (method differentiation)
		{"POST", "/auth/login", "login-ok", 200},
		{"GET", "/auth/google", "oauth:google", 200},
		{"GET", "/auth/login", "oauth:login", 200}, // GET /auth/login falls to :provider (no GET literal)

		// Deep nesting
		{"GET", "/api/v1/resources/featured/items", "featured-items", 200},
		{"GET", "/api/v1/resources/7/items", "items:7", 200},
		{"GET", "/api/v1/resources/featuredish/items", "items:featuredish", 200}, // backtrack
		{"GET", "/api/v1/resources/featured", "resource:featured", 200},
		{"GET", "/api/v1/resources/7", "resource:7", 200},

		// Catch-all
		{"GET", "/files/readme.md", "file:/readme.md", 200},
		{"GET", "/files/a/b/c.txt", "file:/a/b/c.txt", 200},

		// Backtracking edge
		{"GET", "/list/special", "list-special", 200},
		{"GET", "/list/somethingweird", "list:somethingweird", 200},
		{"GET", "/list/s", "list:s", 200},

		// Negative cases
		{"GET", "/users", "", 404},
		{"POST", "/users/me", "", 404},
		{"GET", "/no/such/route", "", 404},
	}

	client := &http.Client{Timeout: 1 * time.Second}
	pass, fail := 0, 0
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, "http://"+addr+c.path, nil)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("FAIL %-4s %-45s err=%v\n", c.method, c.path, err)
			fail++
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		gotBody := string(body)
		// 404 bodies are JSON; we only check the code for those.
		ok := resp.StatusCode == c.wantCode && (c.wantCode != 200 || gotBody == c.wantBody)

		mark := "PASS"
		if !ok {
			mark = "FAIL"
			fail++
		} else {
			pass++
		}
		fmt.Printf("%s %-4s %-45s status=%d body=%q\n", mark, c.method, c.path, resp.StatusCode, gotBody)
	}

	fmt.Printf("\n%d passed, %d failed\n", pass, fail)

	// Hard cap at 5s total: shut down cleanly.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
	}

	if fail > 0 {
		os.Exit(1)
	}
}

func waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := (&http.Client{Timeout: 100 * time.Millisecond}).Get("http://" + addr + "/__ping_does_not_exist")
		if err == nil {
			c.Body.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}
