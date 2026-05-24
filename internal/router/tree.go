package router

// nodeType represents the type of route node
type nodeType uint8

const (
	static   nodeType = iota // static route
	param                    // :param
	catchAll                 // *param
)

// HandlerFunc is a function that handles HTTP requests
type HandlerFunc any

// Node represents a node in the radix tree.
//
// Children layout:
//   - children:  static children only, indexed positionally by `indices`
//   - wildChild: optional wildcard child (param OR catchAll, never both)
//
// A node may hold both static children and a wildChild simultaneously;
// lookup prefers static, then falls back to wildChild.
type Node struct {
	path      string
	indices   string
	children  []*Node
	wildChild *Node
	handlers  []HandlerFunc
	priority  uint32
	nType     nodeType
	fullPath  string
}

// NewNode creates a new routing tree node
func NewNode() *Node {
	return &Node{}
}

// AddRoute adds a route to the tree
func (n *Node) AddRoute(path string, handlers []HandlerFunc) {
	fullPath := path
	n.priority++

	// Empty tree
	if len(n.path) == 0 && len(n.children) == 0 && n.wildChild == nil {
		n.insertChild(path, fullPath, handlers)
		n.nType = static
		return
	}

	parentFullPathIndex := 0

walk:
	for {
		// Find the longest common prefix between the incoming path and this node's path.
		i := longestCommonPrefix(path, n.path)

		// Split this node if the incoming path diverges inside n.path.
		if i < len(n.path) {
			child := &Node{
				path:      n.path[i:],
				nType:     static,
				indices:   n.indices,
				children:  n.children,
				wildChild: n.wildChild,
				handlers:  n.handlers,
				priority:  n.priority - 1,
				fullPath:  n.fullPath,
			}

			n.children = []*Node{child}
			n.wildChild = nil
			n.indices = string([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			n.fullPath = fullPath[:parentFullPathIndex+i]
		}

		// Descend into / create a child for the remaining suffix.
		if i < len(path) {
			path = path[i:]
			idxc := path[0]

			// Wildcard segment in the incoming path?
			if idxc == ':' || idxc == '*' {
				// If a wildChild already exists, it must agree with this one.
				if n.wildChild != nil {
					parentFullPathIndex += len(n.path)
					existing := n.wildChild

					// Catch-all is exclusive: either side being catch-all is a conflict.
					// Surface this before the generic same-name comparison so the panic
					// message names the actual cause.
					if existing.nType == catchAll || idxc == '*' {
						panic("catch-all conflicts with existing wildcard '" + existing.path +
							"' at the same path slot: " + fullPath +
							" vs " + existing.fullPath)
					}

					// Find end of incoming wildcard token.
					end := 1
					for end < len(path) && path[end] != '/' {
						end++
					}
					incomingToken := path[:end]

					if existing.path != incomingToken {
						panic("wildcard '" + incomingToken +
							"' conflicts with existing wildcard '" + existing.path +
							"' at the same path slot: " + fullPath +
							" vs " + existing.fullPath)
					}

					// Same-name wildcard: descend into the param node. Leave the
					// wildcard token on `path` so the next walk iteration consumes
					// it via the longest-common-prefix split against existing.path.
					n = existing
					continue walk
				}
				// No existing wildChild: insertChild handles creation.
				n.insertChild(path, fullPath, handlers)
				return
			}

			// Static segment in the incoming path.

			// Catch-all wildChild is exclusive — no static sibling allowed.
			if n.wildChild != nil && n.wildChild.nType == catchAll {
				panic("catch-all conflicts with static sibling at: " + fullPath)
			}

			// Check if a static child with the next path byte exists.
			for childIdx, c := range []byte(n.indices) {
				if c == idxc {
					parentFullPathIndex += len(n.path)
					childIdx = n.incrementChildPrio(childIdx)
					n = n.children[childIdx]
					continue walk
				}
			}

			// Otherwise insert a new static child.
			n.indices += string([]byte{idxc})
			child := &Node{
				fullPath: fullPath,
			}
			n.children = append(n.children, child)
			n.incrementChildPrio(len(n.indices) - 1)
			n = child

			n.insertChild(path, fullPath, handlers)
			return
		}

		// Path consumed exactly at this node — register handlers here.
		if n.handlers != nil {
			panic("handlers are already registered for path '" + fullPath + "'")
		}
		n.handlers = handlers
		n.fullPath = fullPath
		return
	}
}

// insertChild handles inserting the remaining suffix `path` under node n.
// On entry n is a freshly-positioned cursor: either a new empty node, or an
// existing node we've decided to grow from.
func (n *Node) insertChild(path, fullPath string, handlers []HandlerFunc) {
	for {
		wildcard, i, valid := findWildcard(path)
		if i < 0 {
			break
		}
		if !valid {
			panic("only one wildcard per path segment is allowed")
		}
		if len(wildcard) < 2 {
			panic("wildcards must be named with a non-empty name")
		}

		// param: ':name'
		if wildcard[0] == ':' {
			// Literal prefix before the wildcard (e.g. "users/" in "users/:id").
			if i > 0 {
				n.path = path[:i]
				path = path[i:]
			}

			// Attach param as wildChild.
			// Unreachable in practice: AddRoute only calls insertChild when n.wildChild is nil
			// (empty-tree path, the n.wildChild == nil guard before the insertChild call, and
			// freshly-created nodes). Kept as a defensive guard against future refactors.
			if n.wildChild != nil {
				panic("internal: wildChild already set on node for " + fullPath)
			}
			child := &Node{
				nType:    param,
				path:     wildcard,
				fullPath: fullPath,
				priority: 1,
			}
			n.wildChild = child
			n = child

			// If suffix remains after the param, recurse into a new static child.
			if len(wildcard) < len(path) {
				path = path[len(wildcard):]
				next := &Node{
					priority: 1,
					fullPath: fullPath,
				}
				n.indices = string(path[0])
				n.children = []*Node{next}
				n = next
				continue
			}

			n.handlers = handlers
			return
		}

		// catchAll: '*name'
		if i+len(wildcard) != len(path) {
			panic("catch-all routes are only allowed at the end of the path")
		}
		if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
			panic("catch-all conflicts with existing handle for the path segment root")
		}
		j := i - 1
		if path[j] != '/' {
			panic("no / before catch-all")
		}
		// Literal prefix up to and including the '/' lands on n.path.
		n.path = path[:j+1]

		if len(n.children) > 0 || n.wildChild != nil {
			panic("catch-all conflicts with existing children at: " + fullPath)
		}

		child := &Node{
			path:     path[j+1:],
			nType:    catchAll,
			handlers: handlers,
			priority: 1,
			fullPath: fullPath,
		}
		n.wildChild = child
		return
	}

	// No wildcard in the remaining path — pure static suffix.
	n.path = path
	n.handlers = handlers
	n.fullPath = fullPath
}

// GetValue retrieves handlers and params for a given path.
// At each slot, static children are tried first via the indices table;
// on a no-match-in-subtree result, lookup backtracks to wildChild.
func (n *Node) GetValue(path string) (handlers []HandlerFunc, params map[string]string, fullPath string) {
	params = make(map[string]string)
	if h, fp, ok := walkLookup(n, path, params); ok {
		return h, params, fp
	}
	return nil, nil, ""
}

// walkLookup is the recursive matcher. Returns ok=true iff a handler was found
// in this subtree. Params are mutated in place; on a non-match the caller must
// discard any param writes it made before recursing.
func walkLookup(n *Node, path string, params map[string]string) (handlers []HandlerFunc, fullPath string, ok bool) {
	prefix := n.path

	if len(path) > len(prefix) {
		if path[:len(prefix)] != prefix {
			return nil, "", false
		}
		rest := path[len(prefix):]
		idxc := rest[0]

		// 1. Try the matching static child (at most one due to indices uniqueness).
		for i, c := range []byte(n.indices) {
			if c == idxc {
				if h, fp, found := walkLookup(n.children[i], rest, params); found {
					return h, fp, true
				}
				break
			}
		}

		// 2. Fall back to the wildChild if present.
		if n.wildChild != nil {
			w := n.wildChild
			switch w.nType {
			case param:
				end := 0
				for end < len(rest) && rest[end] != '/' {
					end++
				}
				paramName := w.path[1:]
				prev, hadPrev := params[paramName]
				params[paramName] = rest[:end]

				if end == len(rest) {
					if w.handlers != nil {
						return w.handlers, w.fullPath, true
					}
					restoreParam(params, paramName, prev, hadPrev)
					return nil, "", false
				}
				suffix := rest[end:]
				idxc := suffix[0]
				for i, c := range []byte(w.indices) {
					if c == idxc {
						if h, fp, found := walkLookup(w.children[i], suffix, params); found {
							return h, fp, true
						}
						break
					}
				}
				restoreParam(params, paramName, prev, hadPrev)
				return nil, "", false

			case catchAll:
				params[w.path[1:]] = "/" + rest
				return w.handlers, w.fullPath, true
			default:
				panic("invalid node type in wildChild")
			}
		}

		return nil, "", false
	}

	if path == prefix {
		if n.handlers != nil {
			return n.handlers, n.fullPath, true
		}
		return nil, "", false
	}

	return nil, "", false
}

// restoreParam undoes a params[k] write so the caller can backtrack cleanly.
func restoreParam(params map[string]string, k, prev string, hadPrev bool) {
	if hadPrev {
		params[k] = prev
	} else {
		delete(params, k)
	}
}

// incrementChildPrio increments the priority of the child at the given index
// and bubbles it forward in the children slice to keep hot routes near the front.
func (n *Node) incrementChildPrio(pos int) int {
	cs := n.children
	cs[pos].priority++
	prio := cs[pos].priority

	newPos := pos
	for ; newPos > 0 && cs[newPos-1].priority < prio; newPos-- {
		cs[newPos-1], cs[newPos] = cs[newPos], cs[newPos-1]
	}

	if newPos != pos {
		n.indices = n.indices[:newPos] +
			n.indices[pos:pos+1] +
			n.indices[newPos:pos] + n.indices[pos+1:]
	}

	return newPos
}

// longestCommonPrefix returns the byte length of the common prefix of a and b.
func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

// findWildcard locates the first ':' or '*' wildcard segment in path.
// Returns the wildcard token (including the leading byte), its start index,
// and whether it is valid (no nested ':' or '*' inside the token).
func findWildcard(path string) (wildcard string, i int, valid bool) {
	for start, c := range []byte(path) {
		if c != ':' && c != '*' {
			continue
		}
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

