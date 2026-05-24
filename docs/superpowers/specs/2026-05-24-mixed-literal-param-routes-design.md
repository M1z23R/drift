# Mixed Literal + Param Routes at Same Slot

## Problem

The Drift radix-tree router (`internal/router/tree.go`) panics at startup when a literal segment and a `:param` segment share a parent at the same slot within a single HTTP method's tree.

Example collisions today (all within one method tree):

- `POST /list/login` + `POST /list/:id/edit` → panic (`wildcard segment conflicts with existing children`)
- `POST /list/:id` + `POST /list/login` → panic (`conflict with wildcard route`)

The exact panic depends on insertion order, which forces users to either restructure their URLs or split routes across methods/prefixes.

## Goal

Allow literal and `:param` segments to coexist at the same slot inside a single method's tree, and match by **most specific** at lookup time — literals win over params on exact match, irrespective of insertion order.

## Non-Goals

- Multiple `:param` siblings with different names at the same slot remain a panic (ambiguous; almost always a user bug).
- Catch-all (`*name`) semantics are unchanged — still must be the final segment, still exclusive with siblings at its slot.
- Public router API (`Get`, `Post`, etc.) is unchanged.
- No new wildcard syntax (`{id}` style) — `:param` and `*catchAll` only.

## Rules (Source of Truth)

At any slot within a single method's tree:

| Sibling A | Sibling B | Behavior |
|---|---|---|
| literal `"login"` | `:id` | **Allowed.** Lookup tries literal first; param is fallback. Order-independent. |
| literal `"login"` | literal `"logout"` | Allowed (already works). |
| `:id` | `:slug` | **Panic** with a message naming both full routes. |
| `:id` | `:id` (continuation) | Allowed (same param name; already works). |
| `*filepath` | anything else | Panic (already works; unchanged). |

Lookup priority at each `/`-segment slot:

1. Try static children via the indices table.
2. If a static child matched and its subtree returned no handler, **backtrack** and try the `wildChild`.
3. If no static index matched at all, try the `wildChild`.
4. If nothing matched, return 404.

Backtracking in step 2 is required so that `GET /list/somethingweird` correctly falls through to `:id` when `/list/special` exists — the `s` index match should not steal the lookup.

## Design

### Tree node changes

Today a node is either "has static children" XOR "has a `wildChild`". Relax this so a node can hold **both** static children and a `wildChild` (param) simultaneously. The existing `wildChild bool` + `children[0]` convention is replaced by an explicit field:

```go
type Node struct {
    path      string
    indices   string
    children  []*Node  // static children only, indexed by indices
    wildChild *Node    // param OR catchAll child, nil if none
    handlers  []HandlerFunc
    priority  uint32
    nType     nodeType
    fullPath  string
}
```

Notes:
- `wildChild` is now a pointer to the wildcard child, separate from `children`. This removes the awkward "first child is the wildcard" coupling.
- Both `param` and `catchAll` nodes live under `wildChild` (they're mutually exclusive with each other — you can't have both at the same slot). The match logic in `GetValue` branches on `wildChild.nType` exactly as today.
- Catch-all retains its exclusivity rule: if `wildChild != nil && wildChild.nType == catchAll`, adding any sibling (static or param) panics. Conversely, attempting to add a catch-all when `len(children) > 0` or `wildChild != nil` panics.
- `priority` ordering on `children` is preserved for static lookup performance.

### Insertion (`AddRoute` / `insertChild`)

Both orderings produce the same final tree:

**Order A — param first, then literal:**
1. Insert `/list/:id` — `/list/` node gets `wildChild = {:id node}`.
2. Insert `/list/login` — walk to `/list/`, add `login` as a normal static child (does not touch `wildChild`).

**Order B — literal first, then param:**
1. Insert `/list/login` — `/list/` node gets static child `login`.
2. Insert `/list/:id` — walk to `/list/`, attach `:id` as `wildChild` next to existing static children.

When inserting a param at a node that already has a `wildChild`:
- Same name (`:id` + `:id`) → descend into the existing param node and continue as today.
- Different name (`:id` + `:slug`) → panic with: `"wildcard ':slug' conflicts with existing wildcard ':id' at the same path slot: <full path A> vs <full path B>"`.

Catch-all rules unchanged. A `*` segment still panics if siblings exist.

### Lookup (`GetValue`)

Replace the current "static then wildcard if no static index matched" logic with a try-static-then-backtrack-to-wildcard pattern. Sketch:

```
walk(node, path):
  consume node.path prefix from path
  if path empty:
    return node.handlers (or nil)
  for each static child whose indices entry matches path[0]:
    result = walk(child, path)
    if result != nil: return result
  if node.wildChild != nil:
    consume param segment, recurse into wildChild
    return that result (may be nil)
  return nil
```

The implementation can stay iterative for performance (no recursion), but the semantic is the above: static children are tried first, and a failed static subtree falls back to the param child.

Backtracking is bounded: at any slot there is at most one matching static index entry and at most one `wildChild`, so the cost is at most one extra traversal per slot.

### Param-value handling

When the lookup uses the `wildChild`, the captured param value runs from the current path cursor up to the next `/` (or end of path). This is identical to today. Param names are stored on the node (`n.path[1:]`), no change.

## Errors / Panics

All panics happen at route-registration time, never at request time.

- Two different-named param siblings → panic with both full route paths.
- Catch-all conflicts → unchanged messages.
- Existing "handlers already registered for path X" duplicate-registration panic → unchanged.

## Testing

New file: `internal/router/tree_test.go`. Cases:

1. **Insertion order independence**
   - `/list/:id` then `/list/login`: both routes resolve correctly.
   - `/list/login` then `/list/:id`: both routes resolve correctly.
2. **Most-specific wins**
   - With both registered, `GET /list/login` → literal handler; `GET /list/12345` → param handler with `id=12345`.
3. **Backtracking on partial static match**
   - With `/list/special` + `/list/:id` registered, `GET /list/somethingweird` → param handler with `id=somethingweird` (starts with `s` but is not `special`).
4. **Deeper nesting**
   - `/list/:id/items` + `/list/special/items` both resolve; `/list/special/items` hits the literal subtree, `/list/42/items` hits the param subtree.
5. **Mixed depths at same slot**
   - `/list/special` + `/list/:id/sub`: `GET /list/special` → literal; `GET /list/42/sub` → param.
6. **Same-name param continuation still works**
   - `/list/:id` + `/list/:id/edit` both resolve.
7. **Different-name param siblings panic**
   - Registering `/list/:id` then `/list/:slug` panics with a message naming both routes.
8. **Catch-all exclusivity unchanged**
   - `/files/*path` registered; attempting to add `/files/special` panics.
9. **Duplicate registration still panics**
   - Registering `/list/login` twice panics.

Tests live in `package router` so they exercise the internal API directly.

## Out of Scope

- `{name}` brace syntax for params.
- Regex-constrained params.
- Trailing-slash redirect behavior.
- Method-not-allowed (405) responses.
- Performance tuning beyond the existing priority-sort heuristic.

## Follow-up

After implementation:
- Remove the radix-tree limitation note from the user's global `CLAUDE.md` (the `Drift framework routing:` paragraph) so future sessions know the constraint is lifted.
- Update `DRIFT.md` if it documents the limitation.
