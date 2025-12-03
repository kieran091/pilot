package pilot

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type NodeType uint8

const (
	Static NodeType = iota
	Param
	Wildcard
)

var strPool = struct {
	sync.RWMutex
	pool map[string]string
}{
	pool: make(map[string]string),
}

func internStr(s string) string {
	if len(s) == 0 {
		return s
	}

	strPool.RLock()
	if interned, exists := strPool.pool[s]; exists {
		strPool.RUnlock()
		return interned
	}
	strPool.RUnlock()

	strPool.Lock()
	defer strPool.Unlock()

	// double check
	if interned, exists := strPool.pool[s]; exists {
		return interned
	}

	strPool.pool[s] = s
	return s
}

type node[T any] struct {
	segment    string
	typeOf     NodeType
	children   *map[string]*node[T]
	paramName  string
	paramChild *node[T]
	wildName   string
	wildChild  *node[T]

	valueMu  sync.RWMutex
	value    *T
	fullPath string
}

func (n *node[T]) getChildren() map[string]*node[T] {
	if n.children == nil {
		children := make(map[string]*node[T], 2)
		n.children = &children
	}
	return *n.children
}

func (n *node[T]) hasChildren() bool {
	return n.children != nil && len(*n.children) > 0
}

type RouteTree[T any] struct {
	root   atomic.Value // *node[T]
	freeze sync.RWMutex

	cacheMu  sync.RWMutex
	cache    map[string]*cacheEntry[T]
	cacheTTL time.Duration
}

type cacheEntry[T any] struct {
	value  T
	params map[string]string
	found  bool
	expire time.Time
}

func NewRouteTree[T any]() *RouteTree[T] {
	var r RouteTree[T]
	root := &node[T]{
		segment: "",
		typeOf:  Static,
	}
	r.root.Store(root)
	r.cache = make(map[string]*cacheEntry[T])
	r.cacheTTL = 10 * time.Minute
	return &r
}

func (r *RouteTree[T]) Insert(path string, val T) error {
	if path == "" || path[0] != '/' {
		return errors.New("path must start with '/'")
	}
	segs := splitPath(path)

	r.freeze.Lock()
	defer r.freeze.Unlock()

	root := r.root.Load().(*node[T])
	cur := root
	for i := range segs {
		seg := segs[i]
		if len(seg) == 0 {
			continue
		}

		if seg[0] == ':' {
			name := seg[1:]
			if name == "" {
				return errors.New("param name cannot be empty")
			}
			if cur.wildChild != nil {
				return errors.New("conflict: wildcard and param at same level")
			}
			if cur.paramChild == nil {
				cur.paramChild = &node[T]{
					segment:   internStr(seg),
					typeOf:    Param,
					paramName: internStr(name),
				}
			}
			if cur.paramChild.paramName != name {
				return errors.New("conflict: different param names at same level")
			}
			cur = cur.paramChild
			continue
		}

		if seg[0] == '*' {
			name := seg[1:]
			if name == "" {
				return errors.New("wildcard name cannot be empty")
			}
			if i != len(segs)-1 {
				return errors.New("wildcard must be the last segment")
			}
			if cur.hasChildren() || cur.paramChild != nil {
				return errors.New("conflict: wildcard cannot coexist with other children")
			}
			if cur.wildChild == nil {
				cur.wildChild = &node[T]{
					segment:  internStr(seg),
					typeOf:   Wildcard,
					wildName: internStr(name),
				}
			}
			cur = cur.wildChild
			continue
		}

		children := cur.getChildren()
		child := children[seg]
		if child == nil {
			child = &node[T]{
				segment: internStr(seg),
				typeOf:  Static,
			}
			children[seg] = child
		}
		cur = child
	}

	cur.valueMu.Lock()
	defer cur.valueMu.Unlock()
	if cur.value != nil {
		*cur.value = val
		cur.fullPath = path
	} else {
		v := val
		cur.value = &v
		cur.fullPath = path
	}

	r.root.Store(root)

	r.clearCacheForPath(path)
	return nil
}

func (r *RouteTree[T]) Lookup(path string) (T, map[string]string, bool) {
	var zero T
	if path == "" || path[0] != '/' {
		return zero, nil, false
	}

	r.cacheMu.RLock()
	if entry, exists := r.cache[path]; exists {
		if time.Now().Before(entry.expire) {
			r.cacheMu.RUnlock()
			return entry.value, entry.params, entry.found
		}
		r.cacheMu.RUnlock()
		r.cacheMu.Lock()
		delete(r.cache, path)
		r.cacheMu.Unlock()
	} else {
		r.cacheMu.RUnlock()
	}

	segs := splitPath(path)
	params := make(map[string]string)

	r.freeze.RLock()
	defer r.freeze.RUnlock()

	root := r.root.Load().(*node[T])
	cur := root
	for i := range segs {
		seg := segs[i]
		if seg == "" {
			continue
		}
		if cur.children != nil {
			if child := (*cur.children)[seg]; child != nil {
				cur = child
				continue
			}
		}
		if cur.paramChild != nil {
			params[cur.paramChild.paramName] = seg
			cur = cur.paramChild
			continue
		}
		if cur.wildChild != nil {
			var builder strings.Builder
			for j := i; j < len(segs); j++ {
				if j > i {
					builder.WriteByte('/')
				}
				builder.WriteString(segs[j])
			}
			params[cur.wildChild.wildName] = builder.String()
			cur = cur.wildChild
			break
		}

		r.cacheResult(path, zero, nil, false)
		return zero, nil, false
	}

	cur.valueMu.RLock()
	defer cur.valueMu.RUnlock()
	if cur.value == nil {
		r.cacheResult(path, zero, nil, false)
		return zero, nil, false
	}

	result := *cur.value
	r.cacheResult(path, result, params, true)
	return result, params, true
}

func (r *RouteTree[T]) Update(path string, updater func(old T) (T, bool)) bool {
	if path == "" || path[0] != '/' {
		return false
	}
	segs := splitPath(path)

	r.freeze.RLock()
	root := r.root.Load().(*node[T])
	cur := root
	found := true
	for i := 0; i < len(segs); i++ {
		seg := segs[i]
		if seg == "" {
			continue
		}
		if cur.children != nil {
			if child := (*cur.children)[seg]; child != nil {
				cur = child
				continue
			}
		}
		if cur.paramChild != nil {
			cur = cur.paramChild
			continue
		}
		if cur.wildChild != nil {
			cur = cur.wildChild
			break
		}
		found = false
		break
	}
	r.freeze.RUnlock()
	if !found {
		return false
	}

	cur.valueMu.Lock()
	defer cur.valueMu.Unlock()
	if cur.value == nil {
		return false
	}
	newVal, apply := updater(*cur.value)
	if !apply {
		return false
	}
	*cur.value = newVal
	return true
}

func (r *RouteTree[T]) Delete(path string) bool {
	if path == "" || path[0] != '/' {
		return false
	}
	segs := splitPath(path)

	r.freeze.Lock()
	defer r.freeze.Unlock()

	root := r.root.Load().(*node[T])
	cur := root
	stack := make([]*node[T], 0, len(segs)+1)
	keys := make([]string, 0, len(segs)+1)
	stack = append(stack, cur)
	keys = append(keys, "")

	for i := range segs {
		seg := segs[i]
		if seg == "" {
			continue
		}
		if cur.children != nil {
			if child := (*cur.children)[seg]; child != nil {
				cur = child
				stack = append(stack, cur)
				keys = append(keys, seg)
				continue
			}
		}
		if cur.paramChild != nil {
			cur = cur.paramChild
			stack = append(stack, cur)
			keys = append(keys, ":"+cur.paramName)
			continue
		}
		if cur.wildChild != nil {
			cur = cur.wildChild
			stack = append(stack, cur)
			keys = append(keys, "*"+cur.wildName)
			break
		}
		return false
	}

	cur.valueMu.Lock()
	had := cur.value != nil
	cur.value = nil
	cur.fullPath = ""
	cur.valueMu.Unlock()
	if !had {
		return false
	}

	for i := len(stack) - 1; i > 0; i-- {
		n := stack[i]
		p := stack[i-1]
		if n.value == nil && !n.hasChildren() && n.paramChild == nil && n.wildChild == nil {
			key := keys[i]
			if strings.HasPrefix(key, ":") {
				p.paramChild = nil
			} else if strings.HasPrefix(key, "*") {
				p.wildChild = nil
			} else {
				if p.children != nil {
					delete(*p.children, key)
				}
			}
		}
	}

	r.root.Store(root)
	return true
}

func (r *RouteTree[T]) cacheResult(path string, value T, params map[string]string, found bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	var cachedParams map[string]string
	if params != nil {
		cachedParams = make(map[string]string, len(params))
		for k, v := range params {
			cachedParams[k] = v
		}
	}

	r.cache[path] = &cacheEntry[T]{
		value:  value,
		params: cachedParams,
		found:  found,
		expire: time.Now().Add(r.cacheTTL),
	}
}

func (r *RouteTree[T]) clearCacheForPath(path string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	delete(r.cache, path)

	for cachedPath := range r.cache {
		if strings.HasPrefix(cachedPath, path) || strings.HasPrefix(path, cachedPath) {
			delete(r.cache, cachedPath)
		}
	}
}

func splitPath(path string) []string {
	p := path
	if p == "" {
		return []string{""}
	}

	p = strings.Trim(p, " ")

	if idx := strings.IndexByte(p, '?'); idx >= 0 {
		p = p[:idx]
	}
	if idx := strings.IndexByte(p, '#'); idx >= 0 {
		p = p[:idx]
	}

	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p == "/" {
		return []string{""}
	}
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}

	segs := strings.Split(p, "/")
	if len(segs) > 0 && segs[0] == "" {
		segs = segs[1:]
	}

	return segs
}
