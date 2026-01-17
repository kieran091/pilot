package pilot

import (
	"context"
	"net"
	"net/http"
	"strings"
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
		if c.aborted {
			return
		}
		c.handlers[c.index](c)
		if c.aborted {
			return
		}
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

func (c *Context) ClientIP() string {
	// X-Forwarded-For
	if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// X-Real-IP
	if xri := c.Request.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	// X-Client-IP
	if xci := c.Request.Header.Get("X-Client-IP"); xci != "" {
		if net.ParseIP(xci) != nil {
			return xci
		}
	}

	// CF-Connecting-IP (Cloudflare)
	if cfip := c.Request.Header.Get("CF-Connecting-IP"); cfip != "" {
		if net.ParseIP(cfip) != nil {
			return cfip
		}
	}

	// True-Client-IP
	if tci := c.Request.Header.Get("True-Client-IP"); tci != "" {
		if net.ParseIP(tci) != nil {
			return tci
		}
	}

	// RemoteAddr
	if ip, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	return c.Request.RemoteAddr
}
