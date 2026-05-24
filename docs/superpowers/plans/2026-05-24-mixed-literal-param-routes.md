# Mixed Literal + Param Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow literal route segments to coexist with `:param` segments at the same slot within a single HTTP method's radix tree, with lookups preferring the most specific match (literal > param), regardless of registration order.

**Architecture:** Replace the `wildChild bool` flag + "first-child-is-the-wildcard" coupling with an explicit `wildChild *Node` field separate from the static `children` slice. Insertion permits both kinds of children to coexist. Lookup tries static children via the existing indices table first and backtracks to `wildChild` if the static subtree returns no handler.

**Tech Stack:** Go 1.25, standard library only, package `github.com/m1z23r/drift/internal/router`. Tests use the standard `testing` package.

**Spec:** `docs/superpowers/specs/2026-05-24-mixed-literal-param-routes-design.md`

---

## File Structure

- **Modify:** `internal/router/tree.go` — rewrite `Node` struct, `AddRoute`, `insertChild`, and `GetValue`.
- **Create:** `internal/router/tree_test.go` — new test file covering insertion order, lookup priority, backtracking, and panics. Lives in `package router` to exercise internal API directly.

No public API in `pkg/drift` changes. The `Engine.addRoute` call site in `pkg/drift/drift.go:70` continues to call `root.AddRoute(path, routerHandlers)` unchanged.

---

## Task 1: Lock current behavior with characterization tests

Before changing anything, write tests for the behaviors that must continue working: pure-static routes, pure-param routes, catch-all routes, same-name param continuation, and duplicate-registration panic. These act as a safety net during the rewrite.

**Files:**
- Create: `internal/router/tree_test.go`

- [ ] **Step 1: Write the characterization test file**

```go
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
		t.Fatalf("GetValue(%q): got handler %v, want miss", path, handlers[0])
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

func TestSameNameParamContinuation(t *testing.T) {
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
```

- [ ] **Step 2: Run the characterization tests to see which pass today**

Run: `go test ./internal/router/ -run 'TestStaticOnly|TestParamOnly|TestCatchAll|TestSameNameParamContinuation|TestDuplicateRegistrationPanics' -v`

Expected: All five pass — these are behaviors the current implementation supports.

If any fail, stop and investigate before changing tree.go. The point of this task is to establish a verified baseline.

- [ ] **Step 3: Commit the baseline tests**

```bash
git add internal/router/tree_test.go
git commit -m "test: characterize current radix tree behavior"
```

---

## Task 2: Add a failing test for mixed literal + param routes (param-first order)

This is the first new-behavior test. Drives the core change.

**Files:**
- Modify: `internal/router/tree_test.go` — append new test.

- [ ] **Step 1: Append the failing test**

Add this test function at the end of `internal/router/tree_test.go`:

```go
func TestMixedLiteralAndParam_ParamFirst(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})
	n.AddRoute("/list/login", []HandlerFunc{recordingHandler("login")})

	mustGet(t, n, "/list/login", "login", nil)
	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/router/ -run TestMixedLiteralAndParam_ParamFirst -v`

Expected: panic with message containing `conflict with wildcard route` (from `tree.go:86`).

- [ ] **Step 3: Commit the failing test**

```bash
git add internal/router/tree_test.go
git commit -m "test: add failing test for mixed literal+param routes (param-first)"
```

---

## Task 3: Rewrite the `Node` struct and `AddRoute` / `insertChild` / `GetValue` to allow mixed children

This task does the full tree rewrite in one commit because the struct change cascades across all three functions — leaving any of them on the old layout breaks compilation. Tests run after to verify both the new behavior and all baseline behaviors still pass.

**Files:**
- Modify: `internal/router/tree.go` — replace `Node` struct and the three functions.

- [ ] **Step 1: Replace `internal/router/tree.go` with the new implementation**

Overwrite the entire file with the contents below.

```go
package router

// nodeType represents the type of route node
type nodeType uint8

const (
	static   nodeType = iota // static route
	param                    // :param
	catchAll                 // *param
)

// HandlerFunc is a function that handles HTTP requests
type HandlerFunc interface{}

// Node represents a node in the radix tree.
//
// Children layout:
//   - children:  static children only, indexed positionally by `indices`
//   - wildChild: optional wildcard child (param OR catchAll, never both)
//
// A node may hold both static children and a wildChild simultaneously;
// lookup prefers static, then falls back to wildChild.
type Node struct {
	path     string
	indices  string
	children []*Node
	wildChild *Node
	handlers []HandlerFunc
	priority uint32
	nType    nodeType
	fullPath string
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
					existing.priority++

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

					// Same-name wildcard: descend and continue walking.
					n = existing
					if end == len(path) {
						// No suffix after the wildcard — register handlers here.
						if n.handlers != nil {
							panic("handlers are already registered for path '" + fullPath + "'")
						}
						n.handlers = handlers
						n.fullPath = fullPath
						return
					}
					// Suffix remains; continue walking into this param node.
					path = path[end:]
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
				// Wildcard cannot displace existing static children at this slot.
				if len(n.children) > 0 || n.wildChild != nil {
					// We're about to set n.path to the prefix, but if n already
					// holds children we'd lose them — this only happens on a
					// fresh node, so it's safe. AddRoute never calls insertChild
					// on a node that already has children unless n.path is empty.
				}
				n.path = path[:i]
				path = path[i:]
			}

			// Attach param as wildChild.
			if n.wildChild != nil {
				// Caller (AddRoute) handles same-vs-different-name dispatch;
				// reaching here means we tried to create a wildChild where one
				// already exists, which is a programming error in this package.
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
		// catchAll requires a '/' immediately before it.
		j := i - 1
		if path[j] != '/' {
			panic("no / before catch-all")
		}
		// Literal prefix up to the '/' lands on n.path; the '/' belongs to the wildChild boundary.
		n.path = path[:j+1] // include the trailing '/'

		// catchAll cannot coexist with siblings.
		if len(n.children) > 0 || n.wildChild != nil {
			panic("catch-all conflicts with existing children at: " + fullPath)
		}

		child := &Node{
			path:     path[j+1:], // e.g. "*filepath"
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
				break // only one possible static match per byte; no other static to try
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
					// No handlers at this exact slot.
					restoreParam(params, paramName, prev, hadPrev)
					return nil, "", false
				}
				// More path after the param — descend through w's static children.
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
				// catchAll node lives under wildChild; rest is the entire remainder
				// including the leading '/' that prefix consumed. But prefix here
				// includes that '/' (see insertChild), so rest is what comes after.
				params[w.path[1:]] = "/" + rest
				return w.handlers, w.fullPath, true
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

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Run all existing router tests to verify baseline + new behavior**

Run: `go test ./internal/router/ -v`

Expected: All six tests from Tasks 1 and 2 pass.

If `TestCatchAll` fails because the catchAll param now omits or duplicates the leading `/`, inspect the test expectation (`{"path": "/a"}`) against actual output and fix the implementation, not the test — the historical behavior is "catchAll captures the slash and everything after." The new lookup in `walkLookup` builds the value as `"/" + rest`, which should match.

- [ ] **Step 3: Run the full project test suite to verify nothing else broke**

Run: `go test ./...`

Expected: All packages pass.

- [ ] **Step 4: Commit the rewrite**

```bash
git add internal/router/tree.go
git commit -m "feat(router): allow literal+param sibling routes, prefer literal at lookup"
```

---

## Task 4: Add literal-first insertion order test

Confirm the symmetric case works too: declare the literal route first, then the param route.

**Files:**
- Modify: `internal/router/tree_test.go`

- [ ] **Step 1: Append the test**

```go
func TestMixedLiteralAndParam_LiteralFirst(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/login", []HandlerFunc{recordingHandler("login")})
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})

	mustGet(t, n, "/list/login", "login", nil)
	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/router/ -run TestMixedLiteralAndParam_LiteralFirst -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/router/tree_test.go
git commit -m "test: verify literal-first insertion of mixed routes"
```

---

## Task 5: Add backtracking test (partial static prefix match)

This is the most important correctness test for the new lookup logic. With `/list/special` and `/list/:id` registered, `GET /list/somethingweird` shares the leading `s` byte with `special`, so the indices table will steer the lookup into the `special` subtree. The lookup must backtrack and try `:id`.

**Files:**
- Modify: `internal/router/tree_test.go`

- [ ] **Step 1: Append the test**

```go
func TestBacktrackingOnPartialStaticMatch(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/special", []HandlerFunc{recordingHandler("special")})
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})

	mustGet(t, n, "/list/special", "special", nil)
	mustGet(t, n, "/list/somethingweird", "byID", map[string]string{"id": "somethingweird"})
	mustGet(t, n, "/list/s", "byID", map[string]string{"id": "s"})
	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/router/ -run TestBacktrackingOnPartialStaticMatch -v`

Expected: PASS. If it fails on `/list/somethingweird` returning nil, the backtracking path in `walkLookup` is broken — debug by checking the static-child loop falls through to the `if n.wildChild != nil` block after the recursive call returns `found=false`.

- [ ] **Step 3: Commit**

```bash
git add internal/router/tree_test.go
git commit -m "test: backtrack to wildChild when static prefix match fails deeper"
```

---

## Task 6: Add deeper-nesting and mixed-depth tests

Confirm the behavior holds when the conflict is one level deeper, and when the literal and param subtrees have different depths.

**Files:**
- Modify: `internal/router/tree_test.go`

- [ ] **Step 1: Append both tests**

```go
func TestDeeperNesting(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/:id/items", []HandlerFunc{recordingHandler("paramItems")})
	n.AddRoute("/list/special/items", []HandlerFunc{recordingHandler("specialItems")})

	mustGet(t, n, "/list/special/items", "specialItems", nil)
	mustGet(t, n, "/list/42/items", "paramItems", map[string]string{"id": "42"})
	mustGet(t, n, "/list/somethingweird/items", "paramItems", map[string]string{"id": "somethingweird"})
}

func TestMixedDepthsAtSameSlot(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/special", []HandlerFunc{recordingHandler("special")})
	n.AddRoute("/list/:id/sub", []HandlerFunc{recordingHandler("paramSub")})

	mustGet(t, n, "/list/special", "special", nil)
	mustGet(t, n, "/list/42/sub", "paramSub", map[string]string{"id": "42"})

	// `/list/special/sub`: static branch is taken first (indices match `s`),
	// but `special` is a leaf with no `/sub` child — lookup backtracks to
	// `:id` which matches with id=special.
	mustGet(t, n, "/list/special/sub", "paramSub", map[string]string{"id": "special"})

	// No handler for `/list/42` alone — `:id` only has a continuation route.
	mustMiss(t, n, "/list/42")
}
```

- [ ] **Step 2: Run both tests**

Run: `go test ./internal/router/ -run 'TestDeeperNesting|TestMixedDepthsAtSameSlot' -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/router/tree_test.go
git commit -m "test: deeper nesting and mixed-depth literal/param siblings"
```

---

## Task 7: Add different-named param siblings panic test

Verify that two `:param` siblings with different names at the same slot still panic, with a message naming both routes.

**Files:**
- Modify: `internal/router/tree_test.go`

- [ ] **Step 1: Append the test**

```go
func TestDifferentNamedParamSiblingsPanic(t *testing.T) {
	n := NewNode()
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})

	mustPanic(t, ":slug", func() {
		n.AddRoute("/list/:slug", []HandlerFunc{recordingHandler("bySlug")})
	})
}

func TestSameNamedParamSiblingsAllowed(t *testing.T) {
	n := NewNode()
	// Two routes that share the :id wildcard at the same slot — must not panic.
	n.AddRoute("/list/:id", []HandlerFunc{recordingHandler("byID")})
	n.AddRoute("/list/:id/edit", []HandlerFunc{recordingHandler("edit")})

	mustGet(t, n, "/list/42", "byID", map[string]string{"id": "42"})
	mustGet(t, n, "/list/42/edit", "edit", map[string]string{"id": "42"})
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./internal/router/ -run 'TestDifferentNamedParamSiblingsPanic|TestSameNamedParamSiblingsAllowed' -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/router/tree_test.go
git commit -m "test: panic on different-named :param siblings, allow same-name"
```

---

## Task 8: Add catch-all exclusivity test

Verify catch-all still cannot coexist with static or param siblings.

**Files:**
- Modify: `internal/router/tree_test.go`

- [ ] **Step 1: Append the test**

```go
func TestCatchAllExclusivity(t *testing.T) {
	t.Run("static_then_catchall_panics", func(t *testing.T) {
		n := NewNode()
		n.AddRoute("/files/special", []HandlerFunc{recordingHandler("special")})
		mustPanic(t, "catch-all", func() {
			n.AddRoute("/files/*path", []HandlerFunc{recordingHandler("files")})
		})
	})

	t.Run("catchall_then_static_panics", func(t *testing.T) {
		n := NewNode()
		n.AddRoute("/files/*path", []HandlerFunc{recordingHandler("files")})
		mustPanic(t, "catch-all", func() {
			n.AddRoute("/files/special", []HandlerFunc{recordingHandler("special")})
		})
	})

	t.Run("catchall_then_param_panics", func(t *testing.T) {
		n := NewNode()
		n.AddRoute("/files/*path", []HandlerFunc{recordingHandler("files")})
		mustPanic(t, "catch-all", func() {
			n.AddRoute("/files/:id", []HandlerFunc{recordingHandler("byID")})
		})
	})
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/router/ -run TestCatchAllExclusivity -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/router/tree_test.go
git commit -m "test: catch-all stays exclusive with static and param siblings"
```

---

## Task 9: Integration smoke test through the public `drift.Engine`

Verify the change works end-to-end via the public router API (`engine.Get`, etc.), so we know the integration in `pkg/drift/drift.go` works without changes.

**Files:**
- Create: `pkg/drift/router_mixed_test.go`

- [ ] **Step 1: Create the integration test file**

```go
package drift

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEngine_MixedLiteralAndParamRoutes(t *testing.T) {
	cases := []struct {
		name      string
		register  func(e *Engine)
		path      string
		wantCode  int
		wantBody  string
	}{
		{
			name: "literal_wins_over_param_param_first",
			register: func(e *Engine) {
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:"+c.Params["id"]) })
				e.Get("/list/login", func(c *Context) { c.String(200, "login") })
			},
			path:     "/list/login",
			wantCode: 200,
			wantBody: "login",
		},
		{
			name: "param_matches_when_literal_doesnt_match_exactly",
			register: func(e *Engine) {
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:"+c.Params["id"]) })
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
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:"+c.Params["id"]) })
			},
			path:     "/list/42",
			wantCode: 200,
			wantBody: "byID:42",
		},
		{
			name: "backtracks_to_param_when_static_prefix_partial",
			register: func(e *Engine) {
				e.Get("/list/special", func(c *Context) { c.String(200, "special") })
				e.Get("/list/:id", func(c *Context) { c.String(200, "byID:"+c.Params["id"]) })
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
```

- [ ] **Step 2: Run it**

Run: `go test ./pkg/drift/ -run TestEngine_MixedLiteralAndParamRoutes -v`

Expected: All four subtests PASS.

If `c.String` or `c.Params` use a different shape than assumed, check `pkg/drift/context.go` and adjust the test (not the production code).

- [ ] **Step 3: Run the full test suite one more time**

Run: `go test ./...`

Expected: All packages pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/drift/router_mixed_test.go
git commit -m "test: end-to-end coverage for mixed literal/param routes via Engine"
```

---

## Task 10: Update DRIFT.md if it documents the old limitation

**Files:**
- Possibly modify: `DRIFT.md`

- [ ] **Step 1: Check whether DRIFT.md mentions the old limitation**

Run: `grep -n "wildcard\|literal\|:param\|same slot" DRIFT.md || echo "no matches"`

If no matches, skip steps 2 and 3 of this task — there's nothing to update.

- [ ] **Step 2: If matches were found, update DRIFT.md**

Open `DRIFT.md`, locate the paragraph describing the radix-tree limitation (likely starting with "The radix-tree router panics..." or similar), and replace it with a brief note that literal and `:param` segments can now coexist at the same slot, with the literal winning at lookup. Different-named `:param` siblings still panic.

- [ ] **Step 3: Commit (only if DRIFT.md changed)**

```bash
git add DRIFT.md
git commit -m "docs: note that mixed literal/param routes are now supported"
```

---

## Final verification

- [ ] **Run the full suite one last time**

Run: `go test ./...`

Expected: every package green. If anything else in `pkg/drift` (handlers, websocket, etc.) exercises routes, those routes should resolve identically — the new tree is a strict superset of the old one's behavior.

- [ ] **Build check**

Run: `go build ./...`

Expected: clean build, no warnings.

---

## Notes for the executor

- The Task 3 rewrite is the only large diff. Every other task is a few lines of test code plus a commit.
- The most likely failure mode in Task 3 is the `walkLookup` catch-all branch — historical behavior produces `"/"+rest` for the param value. The new lookup builds it the same way, but only `TestCatchAll` from Task 1 will catch a regression here.
- If a baseline test from Task 1 starts failing after Task 3, do not weaken the test — investigate the new `tree.go` first. The baseline encodes the public contract.
- `findWildcard` and `incrementChildPrio` are unchanged from the original. They're kept intact in the rewrite because they have no dependency on the struct layout change.
