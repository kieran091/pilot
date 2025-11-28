package pilot

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type HandlerFunc func(ctx *Context)

type Context struct {
	Request *http.Request
	Writer  http.ResponseWriter

	handlers []HandlerFunc
	index    int8
	aborted  bool

	Keys map[string]any
	mu   sync.RWMutex

	ctx context.Context

	Service string
	Method  string
	Path    string
	Params  map[string]string

	Errors []error

	StartTime time.Time
}

func NewContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{
		Request:   r,
		Writer:    w,
		handlers:  make([]HandlerFunc, 0),
		index:     -1,
		Keys:      make(map[string]any),
		ctx:       r.Context(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Params:    make(map[string]string),
		Errors:    make([]error, 0),
		StartTime: time.Now(),
	}
}

func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

func (c *Context) Abort() {
	c.aborted = true
}

func (c *Context) IsAborted() bool {
	return c.aborted
}

func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Keys[key] = value
}

func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.Keys[key]
	return val, exists
}

func (c *Context) GetService() string {
	return c.Service
}

func (c *Context) SetService(Service string) {
	c.Service = Service
}

func (c *Context) SetParams(key, value string) {
	c.Params[key] = value
}

func (c *Context) Param(key string) string {
	return c.Params[key]
}

func (c *Context) AddError(err error) {
	c.Errors = append(c.Errors, err)
}

func (c *Context) Duration() time.Duration {
	return time.Since(c.StartTime)
}
