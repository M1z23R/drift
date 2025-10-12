package drift

// nodeType represents the type of route node
type nodeType uint8

const (
	static   nodeType = iota // static route
	param                    // :param
	catchAll                 // *param
)

// node represents a node in the radix tree
type node struct {
	path      string
	indices   string
	children  []*node
	handlers  []HandlerFunc
	priority  uint32
	nType     nodeType
	wildChild bool
	fullPath  string
}

// addRoute adds a route to the tree
func (n *node) addRoute(path string, handlers []HandlerFunc) {
	fullPath := path
	n.priority++

	// Empty tree
	if len(n.path) == 0 && len(n.children) == 0 {
		n.insertChild(path, fullPath, handlers)
		n.nType = static
		return
	}

	parentFullPathIndex := 0

walk:
	for {
		// Find the longest common prefix
		i := longestCommonPrefix(path, n.path)

		// Split edge
		if i < len(n.path) {
			child := &node{
				path:      n.path[i:],
				wildChild: n.wildChild,
				nType:     static,
				indices:   n.indices,
				children:  n.children,
				handlers:  n.handlers,
				priority:  n.priority - 1,
				fullPath:  n.fullPath,
			}

			n.children = []*node{child}
			n.indices = string([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			n.wildChild = false
			n.fullPath = fullPath[:parentFullPathIndex+i]
		}

		// Make new node a child of this node
		if i < len(path) {
			path = path[i:]

			if n.wildChild {
				parentFullPathIndex += len(n.path)
				n = n.children[0]
				n.priority++

				// Check if the wildcard matches
				if len(path) >= len(n.path) && n.path == path[:len(n.path)] &&
					(len(n.path) >= len(path) || path[len(n.path)] == '/') {
					continue walk
				} else {
					panic("conflict with wildcard route")
				}
			}

			idxc := path[0]

			// Check if a child with the next path byte exists
			for i, c := range []byte(n.indices) {
				if c == idxc {
					parentFullPathIndex += len(n.path)
					i = n.incrementChildPrio(i)
					n = n.children[i]
					continue walk
				}
			}

			// Otherwise insert it
			if idxc != ':' && idxc != '*' {
				n.indices += string([]byte{idxc})
				child := &node{
					fullPath: fullPath,
				}
				n.children = append(n.children, child)
				n.incrementChildPrio(len(n.indices) - 1)
				n = child
			}

			n.insertChild(path, fullPath, handlers)
			return
		}

		// Otherwise add handlers to current node
		if n.handlers != nil {
			panic("handlers are already registered for path '" + fullPath + "'")
		}
		n.handlers = handlers
		n.fullPath = fullPath
		return
	}
}

// insertChild inserts a child node
func (n *node) insertChild(path, fullPath string, handlers []HandlerFunc) {
	for {
		// Find prefix until first wildcard
		wildcard, i, valid := findWildcard(path)
		if i < 0 {
			break
		}

		if !valid {
			panic("only one wildcard per path segment is allowed")
		}

		// Check if the wildcard has a name
		if len(wildcard) < 2 {
			panic("wildcards must be named with a non-empty name")
		}

		// Check if this node has existing children
		if len(n.children) > 0 {
			panic("wildcard segment conflicts with existing children")
		}

		// param
		if wildcard[0] == ':' {
			if i > 0 {
				n.path = path[:i]
				path = path[i:]
			}

			n.wildChild = true
			child := &node{
				nType:    param,
				path:     wildcard,
				fullPath: fullPath,
			}
			n.children = []*node{child}
			n = child
			n.priority++

			// If the path doesn't end with the wildcard, then there
			// will be another non-wildcard subpath starting with '/'
			if len(wildcard) < len(path) {
				path = path[len(wildcard):]
				child := &node{
					priority: 1,
					fullPath: fullPath,
				}
				n.children = []*node{child}
				n = child
				continue
			}

			// Otherwise we're done
			n.handlers = handlers
			return
		}

		// catchAll
		if i+len(wildcard) != len(path) {
			panic("catch-all routes are only allowed at the end of the path")
		}

		if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
			panic("catch-all conflicts with existing handle for the path segment root")
		}

		// currently fixed width 1 for '/'
		i--
		if path[i] != '/' {
			panic("no / before catch-all")
		}

		n.path = path[:i]

		// First node: catchAll node with empty path
		child := &node{
			wildChild: true,
			nType:     catchAll,
			fullPath:  fullPath,
		}
		n.children = []*node{child}
		n.indices = string('/')
		n = child
		n.priority++

		// Second node: node holding the variable
		child = &node{
			path:     path[i:],
			nType:    catchAll,
			handlers: handlers,
			priority: 1,
			fullPath: fullPath,
		}
		n.children = []*node{child}

		return
	}

	// If no wildcard was found, simply insert the path and handlers
	n.path = path
	n.handlers = handlers
	n.fullPath = fullPath
}

// getValue retrieves handlers and params for a given path
func (n *node) getValue(path string) (handlers []HandlerFunc, params map[string]string, fullPath string) {
	params = make(map[string]string)

walk:
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				// Try all the non-wildcard children first
				idxc := path[0]
				for i, c := range []byte(n.indices) {
					if c == idxc {
						n = n.children[i]
						continue walk
					}
				}

				// If there is a wildcard child, use it
				if n.wildChild {
					n = n.children[0]
					switch n.nType {
					case param:
						// Find end of param (either '/' or end of path)
						end := 0
						for end < len(path) && path[end] != '/' {
							end++
						}

						// Save param value
						params[n.path[1:]] = path[:end]

						// We need to go deeper!
						if end < len(path) {
							if len(n.children) > 0 {
								path = path[end:]
								n = n.children[0]
								continue walk
							}

							// ... but we can't
							return nil, nil, ""
						}

						handlers = n.handlers
						fullPath = n.fullPath
						return

					case catchAll:
						// Save param value
						params[n.path[2:]] = path

						handlers = n.handlers
						fullPath = n.fullPath
						return

					default:
						panic("invalid node type")
					}
				}

				// Nothing found
				return nil, nil, ""
			}
		} else if path == prefix {
			// We should have reached the node containing the handlers
			if handlers = n.handlers; handlers != nil {
				fullPath = n.fullPath
				return
			}

			// No handlers registered for this path
			return nil, nil, ""
		}

		// Nothing found
		return nil, nil, ""
	}
}

// incrementChildPrio increments the priority of the child at the given index
func (n *node) incrementChildPrio(pos int) int {
	cs := n.children
	cs[pos].priority++
	prio := cs[pos].priority

	// Adjust position (move to front)
	newPos := pos
	for ; newPos > 0 && cs[newPos-1].priority < prio; newPos-- {
		// Swap node positions
		cs[newPos-1], cs[newPos] = cs[newPos], cs[newPos-1]
	}

	// Build new index char string
	if newPos != pos {
		n.indices = n.indices[:newPos] + // Unchanged prefix
			n.indices[pos:pos+1] + // The index char we move
			n.indices[newPos:pos] + n.indices[pos+1:] // Rest without char at 'pos'
	}

	return newPos
}

// Helper functions

// longestCommonPrefix finds the longest common prefix
func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

// findWildcard finds a wildcard segment and checks its validity
func findWildcard(path string) (wildcard string, i int, valid bool) {
	// Find start
	for start, c := range []byte(path) {
		// A wildcard starts with ':' (param) or '*' (catch-all)
		if c != ':' && c != '*' {
			continue
		}

		// Find end and check for invalid characters
		valid = true
		for end, c := range []byte(path[start+1:]) {
			switch c {
			case '/':
				return path[start : start+1+end], start, valid
			case ':', '*':
				valid = false
			}
		}
		return path[start:], start, valid
	}
	return "", -1, false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
