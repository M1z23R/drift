package drift

// RouterGroup is used internally to configure router groups
type RouterGroup struct {
	handlers []HandlerFunc
	basePath string
	engine   *Engine
}

// Group creates a new router group with the given path prefix
func (group *RouterGroup) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		handlers: group.combineHandlers(handlers),
		basePath: group.calculateAbsolutePath(relativePath),
		engine:   group.engine,
	}
}

// Use adds middleware to the router group
func (group *RouterGroup) Use(middleware ...HandlerFunc) {
	group.handlers = append(group.handlers, middleware...)
}

// Get registers a GET route
func (group *RouterGroup) Get(relativePath string, handlers ...HandlerFunc) {
	group.handle("GET", relativePath, handlers)
}

// Post registers a POST route
func (group *RouterGroup) Post(relativePath string, handlers ...HandlerFunc) {
	group.handle("POST", relativePath, handlers)
}

// Put registers a PUT route
func (group *RouterGroup) Put(relativePath string, handlers ...HandlerFunc) {
	group.handle("PUT", relativePath, handlers)
}

// Delete registers a DELETE route
func (group *RouterGroup) Delete(relativePath string, handlers ...HandlerFunc) {
	group.handle("DELETE", relativePath, handlers)
}

// Patch registers a PATCH route
func (group *RouterGroup) Patch(relativePath string, handlers ...HandlerFunc) {
	group.handle("PATCH", relativePath, handlers)
}

// Options registers an OPTIONS route
func (group *RouterGroup) Options(relativePath string, handlers ...HandlerFunc) {
	group.handle("OPTIONS", relativePath, handlers)
}

// Head registers a HEAD route
func (group *RouterGroup) Head(relativePath string, handlers ...HandlerFunc) {
	group.handle("HEAD", relativePath, handlers)
}

// Any registers a route that matches all HTTP methods
func (group *RouterGroup) Any(relativePath string, handlers ...HandlerFunc) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, method := range methods {
		group.handle(method, relativePath, handlers)
	}
}

// Static serves files from the given file system root
func (group *RouterGroup) Static(relativePath, root string) {
	handler := func(c *Context) {
		// Simple static file serving
		// In production, you'd want to use http.FileServer
		c.String(200, "Static file serving for: "+root)
	}
	urlPattern := joinPaths(relativePath, "/*filepath")
	group.Get(urlPattern, handler)
}

// handle registers a new request handle and middleware with the given path and method
func (group *RouterGroup) handle(httpMethod, relativePath string, handlers []HandlerFunc) {
	absolutePath := group.calculateAbsolutePath(relativePath)
	handlers = group.combineHandlers(handlers)
	group.engine.addRoute(httpMethod, absolutePath, handlers)
}

// calculateAbsolutePath calculates the absolute path for a relative path
func (group *RouterGroup) calculateAbsolutePath(relativePath string) string {
	return joinPaths(group.basePath, relativePath)
}

// combineHandlers merges the group's handlers with the route handlers
func (group *RouterGroup) combineHandlers(handlers []HandlerFunc) []HandlerFunc {
	finalSize := len(group.handlers) + len(handlers)
	mergedHandlers := make([]HandlerFunc, finalSize)
	copy(mergedHandlers, group.handlers)
	copy(mergedHandlers[len(group.handlers):], handlers)
	return mergedHandlers
}

// joinPaths joins two paths
func joinPaths(absolutePath, relativePath string) string {
	if relativePath == "" {
		return absolutePath
	}

	finalPath := absolutePath
	if finalPath == "" {
		finalPath = "/"
	}

	// If relativePath starts with '/', use it directly
	if relativePath[0] == '/' {
		if finalPath == "/" {
			return relativePath
		}
		return finalPath + relativePath
	}

	// Otherwise add a separator
	if finalPath[len(finalPath)-1] == '/' {
		return finalPath + relativePath
	}

	return finalPath + "/" + relativePath
}
