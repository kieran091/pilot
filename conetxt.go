package pilot

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Context struct {
	Request *http.Request
	Writer  *responseWriter

	handlers []HandlerFunc
	index    int8
	aborted  bool

	keys map[string]any
	mu   sync.RWMutex

	ctx context.Context

	service    string
	methodName string
	path       string
	params     map[string]string

	errors []error

	startTime time.Time
}

func NewContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{
		Request:   r,
		Writer:    withResponseWriter(w),
		handlers:  make([]HandlerFunc, 0),
		index:     -1,
		keys:      make(map[string]any),
		ctx:       r.Context(),
		path:      r.URL.Path,
		params:    make(map[string]string),
		errors:    make([]error, 0),
		startTime: time.Now(),
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
	c.keys[key] = value
}

func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.keys[key]
	return val, exists
}

func (c *Context) GetService() string {
	return c.service
}

func (c *Context) SetService(service string) {
	c.service = service
}

func (c *Context) GetMethodName() string {
	return c.methodName
}

func (c *Context) SetMethodName(methodName string) {
	c.methodName = methodName
}

func (c *Context) SetParams(key, value string) {
	c.params[key] = value
}

func (c *Context) Param(key string) string {
	return c.params[key]
}

func (c *Context) AddError(err error) {
	c.errors = append(c.errors, err)
}

func (c *Context) Duration() time.Duration {
	return time.Since(c.startTime)
}
