package drift

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/m1z23r/drift/internal/router"
)

// Mode represents the environment mode
type Mode string

const (
	DebugMode   Mode = "debug"
	ReleaseMode Mode = "release"
)

// Engine is the main framework instance
type Engine struct {
	RouterGroup
	pool  sync.Pool
	trees map[string]*router.Node // method -> radix tree
	mode  Mode
}

// New creates a new Engine instance in debug mode
func New() *Engine {
	engine := &Engine{
		RouterGroup: RouterGroup{
			handlers: nil,
			basePath: "/",
			engine:   nil,
		},
		trees: make(map[string]*router.Node),
		mode:  DebugMode,
	}
	engine.RouterGroup.engine = engine
	engine.pool.New = func() any {
		return &Context{}
	}
	return engine
}

// Default creates an Engine with default middleware (Logger, Recovery)
func Default() *Engine {
	engine := New()
	// Add default middleware here if needed
	return engine
}

// SetMode sets the engine mode (debug or release)
func (engine *Engine) SetMode(mode Mode) {
	engine.mode = mode
}

// GetMode returns the current engine mode
func (engine *Engine) GetMode() Mode {
	return engine.mode
}

// IsDebug returns true if the engine is in debug mode
func (engine *Engine) IsDebug() bool {
	return engine.mode == DebugMode
}

// addRoute adds a route to the engine
func (engine *Engine) addRoute(method, path string, handlers []HandlerFunc) {
	if path[0] != '/' {
		panic("path must begin with '/'")
	}

	root := engine.trees[method]
	if root == nil {
		root = router.NewNode()
		engine.trees[method] = root
	}

	// Convert HandlerFunc to router.HandlerFunc (interface{})
	routerHandlers := make([]router.HandlerFunc, len(handlers))
	for i, h := range handlers {
		routerHandlers[i] = h
	}
	root.AddRoute(path, routerHandlers)

	// Log route registration in debug mode
	if engine.IsDebug() {
		log.Printf("[DRIFT] %-7s %s", method, path)
	}
}

// ServeHTTP implements the http.Handler interface
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := engine.pool.Get().(*Context)
	c.Response = w
	c.Request = req
	c.Params = make(map[string]string)
	c.Query = req.URL.Query()
	c.store = make(map[string]any)
	c.index = -1
	c.aborted = false
	c.statusCode = http.StatusOK

	// Log request in debug mode
	var start time.Time
	if engine.IsDebug() {
		start = time.Now()
	}

	engine.handleRequest(c)

	// Log response in debug mode
	if engine.IsDebug() {
		duration := time.Since(start)
		log.Printf("[DRIFT] %s %s - %d - %v", req.Method, req.URL.Path, c.statusCode, duration)
	}

	engine.pool.Put(c)
}

// handleRequest handles the HTTP request
func (engine *Engine) handleRequest(c *Context) {
	httpMethod := c.Request.Method
	path := c.Request.URL.Path

	// Find route
	if root := engine.trees[httpMethod]; root != nil {
		routerHandlers, params, fullPath := root.GetValue(path)
		if routerHandlers != nil {
			// Convert router.HandlerFunc back to HandlerFunc
			handlers := make([]HandlerFunc, len(routerHandlers))
			for i, h := range routerHandlers {
				handlers[i] = h.(HandlerFunc)
			}
			c.handlers = handlers
			c.Params = params
			c.Set("_fullPath", fullPath)
			c.Next()
			return
		}
	}

	// No route found
	c.handlers = []HandlerFunc{func(c *Context) {
		c.JSON(http.StatusNotFound, map[string]string{
			"error": "Not Found",
		})
	}}
	c.Next()
}

// Run starts the HTTP server
func (engine *Engine) Run(addr string) error {
	if engine.IsDebug() {
		log.Printf("[DRIFT] Starting server in %s mode on %s", engine.mode, addr)
		log.Printf("[DRIFT] Use engine.SetMode(drift.ReleaseMode) to disable debug logs")
	}
	return http.ListenAndServe(addr, engine)
}

// RunTLS starts the HTTPS server
func (engine *Engine) RunTLS(addr, certFile, keyFile string) error {
	return http.ListenAndServeTLS(addr, certFile, keyFile, engine)
}

// NoRoute registers handlers for when no route is matched
func (engine *Engine) NoRoute(handlers ...HandlerFunc) {
	// This would require storing and using custom 404 handlers
	// Simplified version - could be enhanced
}

// NoMethod registers handlers for when the method is not allowed
func (engine *Engine) NoMethod(handlers ...HandlerFunc) {
	// This would require storing and using custom 405 handlers
	// Simplified version - could be enhanced
}
